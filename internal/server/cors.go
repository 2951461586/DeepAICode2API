package server

import (
	"net"
	"net/http"
	"net/url"
	"strings"
)

// --- CORS 与来源校验 ---

// withCORS 处理跨域。
//
// 默认只放行本机来源（localhost / 127.0.0.1 / [::1]，任意端口）；
// 需局域网访问时在设置里显式列出逗号分隔的来源。不提供通配符。
func (s *Server) withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		allowed := s.corsOrigin(r.Header.Get("Origin"))
		writeCORSHeaders(w, allowed)
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// writeCORSHeaders 仅在来源通过校验时写入跨域响应头。
func writeCORSHeaders(w http.ResponseWriter, allowed string) {
	if allowed == "" || hasWildcard(allowed) {
		return
	}
	h := w.Header()
	h.Set("Access-Control-Allow-Origin", allowed)
	h.Set("Vary", "Origin")
	h.Set("Access-Control-Allow-Headers", "Authorization, Content-Type, x-api-key, anthropic-version, anthropic-beta, X-Admin-Password, X-Session-Affinity")
	h.Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
	h.Set("Access-Control-Expose-Headers", "X-Request-Id, Retry-After")
}

// corsOrigin 返回应当回给浏览器的 Allow-Origin 值，空串表示不放行。
func (s *Server) corsOrigin(raw string) string {
	if raw == "" || raw == "null" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return ""
	}

	// 1) 显式允许列表（逗号分隔，值来自控制台设置）
	if cfg := strings.TrimSpace(s.Store.Settings().CORSOrigin); cfg != "" {
		for _, entry := range strings.Split(cfg, ",") {
			entry = strings.TrimSpace(entry)
			if entry != "" && !hasWildcard(entry) && strings.EqualFold(entry, raw) {
				return entry
			}
		}
		return ""
	}

	// 2) 默认仅本机：主机名必须在环回集合内
	if !isLoopbackHost(u.Hostname()) {
		return ""
	}
	if port := u.Port(); port != "" {
		return u.Scheme + "://" + net.JoinHostPort(u.Hostname(), port)
	}
	return u.Scheme + "://" + u.Hostname()
}

func hasWildcard(s string) bool { return strings.IndexByte(s, '*') >= 0 }

func isLoopbackHost(host string) bool {
	switch strings.ToLower(host) {
	case "localhost", "127.0.0.1", "::1":
		return true
	}
	return false
}

func isLoopbackAddr(remoteAddr string) bool {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
