package deepai

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
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

// checkInAttemptTimeout 是补签到的**默认**超时预算：签到只是上下文，失败不影响推理。
//
// 线上实测（2026-11-13，v0.1.3，82 分钟）签到阻塞推理 292 次、均值 4.8s、
// 53 次顶到 10s，是首包延迟的主要来源。预算可以通过
// Client.SetCheckInBudget（控制台「签到阻塞预算」）下调；默认值保持不变，
// 避免升级即改变行为。
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

	// ackedNonce 是已经回传给上游的 nonce（诊断 / 测试用）。
	//
	// 「拿到 nonce」和「把 nonce 回传」是两件事：实测只有后者才算签到完成，
	// 未完成的客户端会被上游换到替身模型分支（docs/recon.md §5.1）。
	ackedNonce string

	// ackedAt 是最近一次回传完成的时刻，签到节奏以它为准。
	//
	// 2026-11-13 线上事故：旧实现用「最新 nonce != 已回传 nonce」判断是否需要
	// 补签，而上游每次 /theos 都会下发**新** nonce，于是每回传一次条件又成立，
	// 桌面端的 seconds/2（默认 300s）节奏被完全绕过，实际退化成
	// checkInMinInterval（30s）一次，线上因此多出 292 次同步阻塞。
	ackedAt time.Time

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

// markAcked 记录已回传的 nonce 与回传时刻（只有 /theos 真正带上了它才调用）。
func (c *Client) markAcked(acct *Account, nonce string) {
	if nonce == "" {
		return
	}
	st := c.stateFor(acct)
	c.checkIns.mu.Lock()
	st.ackedNonce = nonce
	st.ackedAt = time.Now()
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

// CheckInOutcome 是一次 EnsureCheckIn 的结果，供调用方记指标 / 日志。
type CheckInOutcome struct {
	// Attempted 报告这次是否真的打了 /theos。
	Attempted bool
	// Waited 是同步等待耗时（Attempted 为 false 时为 0）。
	Waited time.Duration
	// TimedOut 报告这次等待是否被预算截断（上游没在预算内给出响应）。
	TimedOut bool
	// Err 是 /theos 的原始错误（403 签退、超时等；调用方不应据此拒绝推理）。
	Err error
}

// checkInDueAt 判断账号此刻是否需要补签到（纯读，调用方持锁）。
//
// 三条规则与桌面端一致：被 check_in_refused 锁住 → 不签；已经回传过 →
// 按半衰期 seconds/2 节流；最小间隔兜底（秒数很长时不被连续触发）。
func checkInDueAt(st *checkInState, now time.Time) bool {
	if st == nil {
		return false
	}
	if !st.refusedUntil.IsZero() && now.Before(st.refusedUntil) {
		return false
	}
	if st.checkIn != nil && st.checkIn.Valid() && !st.ackedAt.IsZero() {
		ttl := time.Duration(st.checkIn.Seconds) * time.Second
		if now.Sub(st.ackedAt) < ttl/2 {
			return false
		}
	}
	if !st.attemptAt.IsZero() && now.Sub(st.attemptAt) < checkInMinInterval {
		return false
	}
	return true
}

// CheckInDue 报告账号此刻是否需要补签到（只读，不发请求）。
//
// 热路径（EnsureCheckIn）与后台预热（Server.WarmCheckIns）共用这一判据：
// 预热只会让热路径更早看到「刚回传过」，不改变签到节奏，也不会造成额外风暴。
func (c *Client) CheckInDue(acct *Account) bool {
	if acct == nil {
		return false
	}
	st := c.stateFor(acct)
	c.checkIns.mu.Lock()
	defer c.checkIns.mu.Unlock()
	return checkInDueAt(st, time.Now())
}

// EnsureCheckIn 在推理前补一次签到（对应桌面端的 ensureCheckedIn）。
//
// 节奏与桌面端一致：以「上次回传完成」的时刻为基准，seconds/2（默认 300s）
// 内不再打扰上游；从未回传过（冷启动 / 新号）则立即补一次，并用
// attemptAt 限制最小间隔。任何失败都不阻塞推理：签到是附带条件，不是准入。
func (c *Client) EnsureCheckIn(ctx context.Context, acct *Account) CheckInOutcome {
	return c.EnsureCheckInWithin(ctx, acct, c.checkInBudget())
}

// EnsureCheckInWithin 与 EnsureCheckIn 同义，但用显式预算。
//
// 两个调用方对预算的诉求相反：
//   - 热路径（推理前）要短：超过预算就先带旧 nonce 放行推理（默认 10s，可调到 2.5s）；
//   - 后台预热（WarmCheckIns）可以长：它不在任何请求的关键路径上，
//     给足 10s 才容易真正把 nonce 回传成功（线上住宅代理 p95 约 5s）。
func (c *Client) EnsureCheckInWithin(ctx context.Context, acct *Account, budget time.Duration) CheckInOutcome {
	var out CheckInOutcome
	if acct == nil {
		return out
	}
	st := c.stateFor(acct)
	now := time.Now()
	c.checkIns.mu.Lock()
	due := checkInDueAt(st, now)
	if due {
		st.attemptAt = now
	}
	c.checkIns.mu.Unlock()
	if !due {
		return out
	}

	out.Attempted = true
	start := time.Now()
	cctx, cancel := context.WithTimeout(ctx, budget)
	defer cancel()
	// 结果（包括 403）都不影响推理；挑战在 captureCheckIn 里已经吃到。
	_, out.Err = c.Theos(cctx, acct)
	out.Waited = time.Since(start)
	out.TimedOut = errors.Is(out.Err, context.DeadlineExceeded)
	return out
}

// checkInBudget 返回同步签到的等待预算。
//
// 优先级：Runtime 拉取值 → 实例值（SetCheckInBudget）→ 默认 10s。
func (c *Client) checkInBudget() time.Duration {
	if c.Runtime != nil {
		if d := c.Runtime().CheckInBudget; d > 0 {
			return d
		}
	}
	if c.CheckInBudget > 0 {
		return c.CheckInBudget
	}
	return checkInAttemptTimeout
}

// SetCheckInBudget 设置推理前同步签到的等待预算（<=0 表示用默认值 10s）。
//
// 调小不会让签到失败：超过预算就带着旧 nonce 先放行推理，下一轮请求继续补。
func (c *Client) SetCheckInBudget(d time.Duration) {
	if d < 0 {
		d = 0
	}
	c.CheckInBudget = d
}
