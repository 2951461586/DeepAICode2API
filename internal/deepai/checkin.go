package deepai

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// processStartedAt 是进程启动时间，用于心跳里的 up=<秒>。
var processStartedAt = time.Now()

// checkInMinInterval 是两次签到尝试之间的最小间隔。
//
// 桌面端在拿到 check_in 之前不会主动签到（它靠账号状态刷新触发）；
// 网关没有独立的“状态轮询”，所以给首次签到加一个下限，避免每次热路径
// 请求都去撞一次 403。
const checkInMinInterval = 30 * time.Second

// checkInAttemptTimeout 是补签到的超时预算：签到只是上下文，失败不影响推理。
const checkInAttemptTimeout = 10 * time.Second

// CheckInRefusedLock 是命中 check_in_refused 后的静默锁时长。
//
// 上游当天的「重启签到」次数用尽后，403 里仍然会下发新的 nonce；若不停手，
// needAck 会让 EnsureCheckIn 每 30s（checkInMinInterval）再撞一次，变成一个
// 自己制造的签到风暴。锁一段时间比“识别错误码后仍然重试”克制得多。
// 上游的拒绝文案是 “Try again tomorrow”，这里取 6h（与客户端的日额度节奏一致），
// 到期后允许再试一次；如果仍被拒，锁会重新续上。
const CheckInRefusedLock = 6 * time.Hour

// newInstanceID 生成进程级实例号（16 字节随机数的 hex，32 个字符）。
func newInstanceID() string {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return ""
	}
	return hex.EncodeToString(buf[:])
}

// UptimeSeconds 返回进程已运行的秒数。
func UptimeSeconds() int { return int(time.Since(processStartedAt).Seconds()) }

type checkInState struct {
	checkIn   *CheckIn  // 最近一次 /theos 下发的签到挑战
	noteAt    time.Time // 拿到挑战的时间
	attemptAt time.Time // 最近一次主动签到的时间
	lastRelay time.Time // 最近一次推理转发时间（心跳 pi 字段）

	// ackedNonce 是已经回传给上游的 nonce。
	//
	// 「拿到 nonce」和「把 nonce 回传」是两件事：实测只有后者才算签到完成，
	// 未完成的客户端会被上游换到替身模型分支（docs/recon.md §5.1）。
	// 没有这个字段时，EnsureCheckIn 会在 seconds/2 内直接返回，导致冷启动后
	// 前 300s 的推理都还没签到。
	ackedNonce string

	// refusedUntil 是 check_in_refused 的静默锁（零值 = 未锁）。
	//
	// 被上游按天限流的账号当天拿不到真模型分支，但推理仍可用；
	// 继续打 /theos 只会浪费请求并可能加深风控。
	refusedUntil time.Time
}

type checkInStore struct {
	mu sync.Mutex
	m  map[string]*checkInState
}

func newCheckInStore() *checkInStore {
	return &checkInStore{m: map[string]*checkInState{}}
}

// checkInKey 以 API Key 区分签到状态：同一个 Client 会被多个账号复用
// （只按代理分桶），签到挑战必须按账号隔离。
func checkInKey(acct *Account) string {
	if acct == nil {
		return ""
	}
	return acct.APIKey
}

func (s *checkInStore) state(key string) *checkInState {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.m[key]
	if st == nil {
		st = &checkInState{}
		s.m[key] = st
	}
	return st
}

func (c *Client) stateFor(acct *Account) *checkInState {
	if c.checkIns == nil {
		c.checkIns = newCheckInStore()
	}
	return c.checkIns.state(checkInKey(acct))
}

// captureCheckIn 从 /theos 的响应体里取出 check_in（成功与拒绝都会带）。
func (c *Client) captureCheckIn(acct *Account, body []byte) {
	var payload struct {
		CheckIn *CheckIn `json:"check_in"`
	}
	if json.Unmarshal(body, &payload) != nil || !payload.CheckIn.Valid() {
		return
	}
	st := c.stateFor(acct)
	c.checkIns.mu.Lock()
	defer c.checkIns.mu.Unlock()
	cp := *payload.CheckIn
	st.checkIn = &cp
	st.noteAt = time.Now()
}

// CheckIn 返回缓存的签到挑战（测试 / 控制台诊断用）。
func (c *Client) CheckIn(acct *Account) *CheckIn {
	st := c.stateFor(acct)
	c.checkIns.mu.Lock()
	defer c.checkIns.mu.Unlock()
	if st.checkIn == nil {
		return nil
	}
	cp := *st.checkIn
	return &cp
}

// markAcked 记录已回传的 nonce（只有 /theos 真正带上了它才调用）。
func (c *Client) markAcked(acct *Account, nonce string) {
	if nonce == "" {
		return
	}
	st := c.stateFor(acct)
	c.checkIns.mu.Lock()
	st.ackedNonce = nonce
	c.checkIns.mu.Unlock()
}

// markRelay 记录一次推理转发时间；心跳里的 pi 字段据此判断“最近有没有在干活”。
func (c *Client) markRelay(acct *Account) {
	st := c.stateFor(acct)
	c.checkIns.mu.Lock()
	st.lastRelay = time.Now()
	c.checkIns.mu.Unlock()
}

// lockCheckIn 给某个账号的签到加静默锁（命中 check_in_refused 时使用）。
func (c *Client) lockCheckIn(acct *Account, d time.Duration) {
	if acct == nil || d <= 0 {
		return
	}
	st := c.stateFor(acct)
	c.checkIns.mu.Lock()
	st.refusedUntil = time.Now().Add(d)
	c.checkIns.mu.Unlock()
}

// CheckInRefusedUntil 返回账号的签到静默锁到期时间（零值 = 未锁）。
//
// 控制台诊断 / 测试用：被锁的账号当天不会再打 /theos，但推理照常。
func (c *Client) CheckInRefusedUntil(acct *Account) time.Time {
	st := c.stateFor(acct)
	c.checkIns.mu.Lock()
	defer c.checkIns.mu.Unlock()
	return st.refusedUntil
}

// applyCheckInHeaders 写入签到 nonce 与心跳（仅 /theos 使用）。
func (c *Client) applyCheckInHeaders(req *http.Request, acct *Account) {
	st := c.stateFor(acct)
	c.checkIns.mu.Lock()
	ci := st.checkIn
	relayed := st.lastRelay
	c.checkIns.mu.Unlock()

	ttl := 600
	if ci != nil && ci.Seconds > 0 {
		ttl = ci.Seconds
	}
	if ci != nil && ci.Nonce != "" {
		req.Header.Set(HeaderCheckIn, ci.Nonce)
	}
	pi := 0
	if !relayed.IsZero() && time.Since(relayed) < time.Duration(ttl)*time.Second {
		pi = 1
	}
	// 与桌面端 deepAIBeacon 同形：v=<版本>;up=<秒>;pi=<0|1>。
	//
	// 桌面端只在焦点状态已知时追加 `;focus=<0|1>`（deepAIBeacon 把未知事实
	// 直接省略）。网关是服务端进程，没有窗口焦点，所以不伪造这个字段。
	req.Header.Set(HeaderBeacon, fmt.Sprintf("v=%s;up=%d;pi=%d", AppVersion, UptimeSeconds(), pi))
}

// EnsureCheckIn 在推理前补一次签到（对应桌面端的 ensureCheckedIn）。
//
// 语义与桌面端一致：只在“已有挑战且已过半衰期”时补签；网关额外允许
// 在没有挑战时补一次（相当于账号状态刷新），并用 attemptAt 限制频率。
// 任何失败都不阻塞推理：签到是附带的条件，不是准入。
func (c *Client) EnsureCheckIn(ctx context.Context, acct *Account) {
	if acct == nil {
		return
	}
	st := c.stateFor(acct)
	c.checkIns.mu.Lock()
	ci, noteAt, attemptAt, acked := st.checkIn, st.noteAt, st.attemptAt, st.ackedNonce
	c.checkIns.mu.Unlock()

	now := time.Now()
	// 被 check_in_refused 锁住的账号当天不再签到：推理仍可走替身分支，
	// 继续打 /theos 只会浪费时间与风控预算（见 CheckInRefusedLock）。
	if !st.refusedUntil.IsZero() && now.Before(st.refusedUntil) {
		return
	}
	// 有挑战但没回传过 → 先补一次（这是上游判定“真实客户端”的依据）。
	// 否则才退回半衰期节奏（seconds/2，默认 300s）。
	needAck := ci != nil && ci.Valid() && ci.Nonce != acked
	if ci != nil && ci.Valid() && !needAck && now.Sub(noteAt) < time.Duration(ci.Seconds)*time.Second/2 {
		return
	}
	if !attemptAt.IsZero() && now.Sub(attemptAt) < checkInMinInterval {
		return
	}
	c.checkIns.mu.Lock()
	st.attemptAt = now
	c.checkIns.mu.Unlock()

	cctx, cancel := context.WithTimeout(ctx, checkInAttemptTimeout)
	defer cancel()
	// 结果（包括 403）都不影响推理；挑战在 captureCheckIn 里已经吃到。
	_, _ = c.Theos(cctx, acct)
}
