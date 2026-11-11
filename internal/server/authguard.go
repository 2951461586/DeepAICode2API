package server

import (
	"log"
	"net"
	"net/http"
	"strconv"
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
	// adminAuthFreeAttempts 是进入退避前允许的连续失败次数。
	adminAuthFreeAttempts = 5
	// adminAuthBackoffBase 是首次退避时长（测试可注入更短的值）。
	adminAuthBackoffBase = time.Second
	// adminAuthBackoffMax 是退避上限：太长会让运维自己也进不来。
	adminAuthBackoffMax = 30 * time.Second
	// adminAuthGuardMaxEntries 是表规模上限，超过时先清理过期条目。
	adminAuthGuardMaxEntries = 4096
)

// adminGuardEntry 是一个来源 IP 的失败计数与退避截止时间。
type adminGuardEntry struct {
	fails int
	until time.Time
}

// adminGuard 记录管理口令的失败情况。
type adminGuard struct {
	mu      sync.Mutex
	entries map[string]*adminGuardEntry
}

func newAdminGuard() *adminGuard {
	return &adminGuard{entries: map[string]*adminGuardEntry{}}
}

// clientIP 取出来源 IP（不含端口）；解析失败时退回原串。
func clientIP(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		return strings.TrimSpace(remoteAddr)
	}
	return host
}

// blocked 报告该来源当前是否处于退避期，返回剩余时长。
func (g *adminGuard) blocked(ip string, now time.Time) (time.Duration, bool) {
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

// failure 记录一次失败，返回本次判定的退避时长（0 表示尚未进入退避）。
func (g *adminGuard) failure(ip string, now time.Time, free int, base, max time.Duration) time.Duration {
	g.mu.Lock()
	defer g.mu.Unlock()

	if len(g.entries) >= adminAuthGuardMaxEntries {
		g.pruneLocked(now)
	}
	e := g.entries[ip]
	if e == nil {
		e = &adminGuardEntry{}
		g.entries[ip] = e
	}
	e.fails++
	if e.fails <= free {
		return 0
	}
	// 第一次进入退避时 backoff = base，之后每次翻倍。
	shift := e.fails - free - 1
	backoff := base
	for i := 0; i < shift && backoff < max; i++ {
		backoff *= 2
	}
	if backoff > max {
		backoff = max
	}
	e.until = now.Add(backoff)
	return backoff
}

// success 清掉该来源的失败记录。
func (g *adminGuard) success(ip string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.entries, ip)
}

// pruneLocked 清理已过退避期且不再有意义的条目（调用方持锁）。
func (g *adminGuard) pruneLocked(now time.Time) {
	for ip, e := range g.entries {
		if e.until.IsZero() || now.After(e.until) {
			delete(g.entries, ip)
		}
	}
}

// adminGuardParams 返回生效的 free/base/max（允许测试注入更短的值）。
func (s *Server) adminGuardParams() (free int, base, max time.Duration) {
	free, base, max = adminAuthFreeAttempts, s.adminGuardBase, adminAuthBackoffMax
	if base <= 0 {
		base = adminAuthBackoffBase
	}
	return free, base, max
}

// checkAdminThrottle 在比对口令前调用：处于退避期就直接 429。
func (s *Server) checkAdminThrottle(w http.ResponseWriter, r *http.Request) bool {
	delay, blocked := s.adminGuard.blocked(clientIP(r.RemoteAddr), time.Now())
	if !blocked {
		return true
	}
	secs := int(delay.Round(time.Second) / time.Second)
	if secs < 1 {
		secs = 1
	}
	w.Header().Set("Retry-After", strconv.Itoa(secs))
	writeErr(w, http.StatusTooManyRequests,
		"管理口令尝试过于频繁，请 "+strconv.Itoa(secs)+" 秒后再试", "invalid_request_error")
	return false
}

// noteAdminAuthFailure 记录失败并在进入退避时打日志。
func (s *Server) noteAdminAuthFailure(r *http.Request) {
	ip := clientIP(r.RemoteAddr)
	free, base, max := s.adminGuardParams()
	backoff := s.adminGuard.failure(ip, time.Now(), free, base, max)
	if backoff > 0 {
		log.Printf("[admin-auth] 管理口令连续失败，来源 %s 退避 %s（%s %s）",
			ip, backoff, r.Method, r.URL.Path)
	}
}

// noteAdminAuthSuccess 口令正确时清零计数。
func (s *Server) noteAdminAuthSuccess(r *http.Request) {
	s.adminGuard.success(clientIP(r.RemoteAddr))
}
