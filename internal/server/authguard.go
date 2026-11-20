package server

import (
	"log"
	"net/http"
	"strconv"
	"time"

	"deepaicode2api/internal/guard"
	"deepaicode2api/internal/httpx"
)

// --- 管理面口令退避的装配 ---
//
// 失败表与退避算法在 internal/guard.AdminGuard；这里负责 HTTP 语义（429 +
// Retry-After）与日志。adminGuardBase 保留在 Server 上：测试用它注入更短的
// 首次退避时长，避免真的等 1 秒。

// adminGuardPolicy 返回生效的退避策略。
func (s *Server) adminGuardPolicy() guard.BackoffPolicy {
	p := guard.DefaultBackoffPolicy()
	if s.adminGuardBase > 0 {
		p.Base = s.adminGuardBase
	}
	return p
}

// checkAdminThrottle 在比对口令前调用：处于退避期就直接 429。
func (s *Server) checkAdminThrottle(w http.ResponseWriter, r *http.Request) bool {
	delay, blocked := s.adminGuard.Blocked(guard.ClientIP(r.RemoteAddr), time.Now())
	if !blocked {
		return true
	}
	secs := int(delay.Round(time.Second) / time.Second)
	if secs < 1 {
		secs = 1
	}
	w.Header().Set("Retry-After", strconv.Itoa(secs))
	httpx.WriteErr(w, http.StatusTooManyRequests,
		"管理口令尝试过于频繁，请 "+strconv.Itoa(secs)+" 秒后再试", "invalid_request_error")
	return false
}

// noteAdminAuthFailure 记录失败并在进入退避时打日志。
func (s *Server) noteAdminAuthFailure(r *http.Request) {
	ip := guard.ClientIP(r.RemoteAddr)
	backoff := s.adminGuard.Failure(ip, time.Now(), s.adminGuardPolicy())
	if backoff > 0 {
		log.Printf("[admin-auth] 管理口令连续失败，来源 %s 退避 %s（%s %s）",
			ip, backoff, r.Method, r.URL.Path)
	}
}

// noteAdminAuthSuccess 口令正确时清零计数。
func (s *Server) noteAdminAuthSuccess(r *http.Request) {
	s.adminGuard.Success(guard.ClientIP(r.RemoteAddr))
}
