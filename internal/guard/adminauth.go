package guard

import (
	"net"
	"strings"
	"sync"
	"time"
)

// --- 管理面：口令暴力破解防护 ---
//
// 管理口令是控制台与服务端状态的唯一防线，而 /login 是公开页面。此前
// withAdminAuth 只做常数时间比较，失败没有代价——局域网里可以无上限地猜。
//
// 策略（按来源 IP）：
//   - 前 free 次失败照常回 401；
//   - 之后进入指数退避（base×2^n，上限 max），退避期内直接 429 + Retry-After；
//   - 成功一次即清零；
//   - 表达到上限时清掉已过期的条目，避免被伪造来源撑爆内存。

const (
	// FreeAttempts 是进入退避前允许的连续失败次数。
	FreeAttempts = 5
	// backoffBase 是首次退避时长（Server 会把设置里的值注入成 policy.Base）。
	backoffBase = time.Second
	// backoffMax 是退避上限：太长会让运维自己也进不来。
	backoffMax = 30 * time.Second
	// adminGuardMaxEntries 是表规模上限，超过时先清理过期条目。
	adminGuardMaxEntries = 4096
)

// BackoffPolicy 是退避策略（服务器从设置/测试注入，零值走默认）。
type BackoffPolicy struct {
	Free int           // 进入退避前的连续失败次数（<=0 用 FreeAttempts）
	Base time.Duration // 首次退避时长（<=0 用 backoffBase）
	Max  time.Duration // 退避上限（<=0 用 backoffMax）
}

// DefaultBackoffPolicy 返回默认策略。
func DefaultBackoffPolicy() BackoffPolicy {
	return BackoffPolicy{Free: FreeAttempts, Base: backoffBase, Max: backoffMax}
}

func (p BackoffPolicy) normalized() BackoffPolicy {
	if p.Free <= 0 {
		p.Free = FreeAttempts
	}
	if p.Base <= 0 {
		p.Base = backoffBase
	}
	if p.Max <= 0 {
		p.Max = backoffMax
	}
	return p
}

// adminGuardEntry 是一个来源 IP 的失败计数与退避截止时间。
type adminGuardEntry struct {
	fails int
	until time.Time
}

// AdminGuard 记录管理口令的失败情况。
type AdminGuard struct {
	mu      sync.Mutex
	entries map[string]*adminGuardEntry
}

// NewAdminGuard 返回一个空的失败表。
func NewAdminGuard() *AdminGuard {
	return &AdminGuard{entries: map[string]*adminGuardEntry{}}
}

// ClientIP 取出来源 IP（不含端口）；解析失败时退回原串。
func ClientIP(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		return strings.TrimSpace(remoteAddr)
	}
	return host
}

// Blocked 报告该来源当前是否处于退避期，返回剩余时长。
func (g *AdminGuard) Blocked(ip string, now time.Time) (time.Duration, bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	e := g.entries[ip]
	if e == nil || e.until.IsZero() {
		return 0, false
	}
	if now.Before(e.until) {
		return e.until.Sub(now), true
	}
	return 0, false
}

// Failure 记录一次失败，返回本次判定的退避时长（0 表示尚未进入退避）。
func (g *AdminGuard) Failure(ip string, now time.Time, policy BackoffPolicy) time.Duration {
	policy = policy.normalized()
	g.mu.Lock()
	defer g.mu.Unlock()

	if len(g.entries) >= adminGuardMaxEntries {
		g.pruneLocked(now)
	}
	e := g.entries[ip]
	if e == nil {
		e = &adminGuardEntry{}
		g.entries[ip] = e
	}
	e.fails++
	if e.fails <= policy.Free {
		return 0
	}
	// 第一次进入退避时 backoff = base，之后每次翻倍。
	shift := e.fails - policy.Free - 1
	backoff := policy.Base
	for i := 0; i < shift && backoff < policy.Max; i++ {
		backoff *= 2
	}
	if backoff > policy.Max {
		backoff = policy.Max
	}
	e.until = now.Add(backoff)
	return backoff
}

// Success 清掉该来源的失败记录。
func (g *AdminGuard) Success(ip string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.entries, ip)
}

// pruneLocked 清理已过退避期且不再有意义的条目（调用方持锁）。
func (g *AdminGuard) pruneLocked(now time.Time) {
	for ip, e := range g.entries {
		if e.until.IsZero() || now.After(e.until) {
			delete(g.entries, ip)
		}
	}
}
