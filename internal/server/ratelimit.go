package server

import (
	"net/http"
	"time"

	"deepaicode2api/internal/guard"
)

// --- 数据面限流的装配 ---
//
// 状态机（令牌桶 / 在途计数 / 闲置回收）在 internal/guard；这里只负责把
// 「设置」翻译成一次判定。放在 withAuth 里（校验密钥之后）而不是全局中间件：
// 限流是按密钥隔离的，没有密钥的请求（未签发密钥的本机自举）不该计入任何人的配额。
func (s *Server) rateLimitGate(r *http.Request, key string) (release func(), ok bool, retryAfter time.Duration) {
	st := s.settings(r)
	return s.limiter.Gate(guard.KeyID(key), guard.RateConfig{
		RPS:         st.RateLimitRPS,
		Burst:       st.RateLimitBurst,
		Concurrency: st.KeyConcurrency,
	})
}
