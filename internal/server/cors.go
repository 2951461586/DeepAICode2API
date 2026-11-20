package server

import (
	"net/http"

	"deepaicode2api/internal/guard"
)

// --- CORS 与来源校验（装配） ---
//
// 判定逻辑在 internal/guard（纯函数，可单测）。这里只做中间件接线。

// withCORS 处理跨域。
//
// 默认只放行本机来源（localhost / 127.0.0.1 / [::1]，任意端口）；
// 需局域网访问时在设置里显式列出逗号分隔的来源。不提供通配符。
func (s *Server) withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		guard.WriteHeaders(w, guard.Origin(r.Header.Get("Origin"), s.settings(r).CORSOrigin))
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
