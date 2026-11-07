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

// markRelay 记录一次推理转发时间；心跳里的 pi 字段据此判断“最近有没有在干活”。
func (c *Client) markRelay(acct *Account) {
	st := c.stateFor(acct)
	c.checkIns.mu.Lock()
	st.lastRelay = time.Now()
	c.checkIns.mu.Unlock()
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
	// 与桌面端 deepAIBeacon 同形：v=<版本>;up=<秒>;pi=<0|1>;focus=<0|1>。
	req.Header.Set(HeaderBeacon, fmt.Sprintf("v=%s;up=%d;pi=%d;focus=1", AppVersion, UptimeSeconds(), pi))
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
	ci, noteAt, attemptAt := st.checkIn, st.noteAt, st.attemptAt
	c.checkIns.mu.Unlock()

	now := time.Now()
	if ci != nil && ci.Valid() && now.Sub(noteAt) < time.Duration(ci.Seconds)*time.Second/2 {
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
