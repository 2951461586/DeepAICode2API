package observe

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"time"

	"deepaicode2api/internal/guard"
	"deepaicode2api/internal/logx"
)

// --- request id / 访问日志 / 指标中间件 ---
//
// 三件事放在一层中间件里完成，而不是散在各 handler：
//   - request id：生成并回写 X-Request-Id（CORS 早已在 Expose-Headers 里
//     声明了它，但此前从未真正生成过），日志与响应因此可以互相对上；
//   - 访问日志：一行 JSON（method/path/status/ms/bytes/remote/request_id），
//     静态资源除外，避免刷屏；
//   - 指标：把状态码分类、耗时总计、在途请求数记进原子计数器，供 /metrics 导出。

type requestIDKey struct{}

// NewRequestID 生成 16 位 hex 的请求号。
func NewRequestID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		// 随机源不可用时退化为时间戳：request id 只用于关联日志，不需要密码学强度。
		return fmt.Sprintf("%016x", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}

// SanitizeRequestID 只接受长度可控、字符集安全的传入 id（便于跨服务串联）。
func SanitizeRequestID(s string) string {
	s = strings.TrimSpace(s)
	if s == "" || len(s) > 64 {
		return ""
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '-', r == '_', r == '.':
		default:
			return ""
		}
	}
	return s
}

// statusWriter 记录状态码与响应字节数，并透传 Flush / Unwrap。
//
// Unwrap 是必须的：http.ResponseController 与部分中间件靠它拿到底层
// ResponseWriter（本项目虽未用 ResponseController，但保留以免将来踩坑）。
type statusWriter struct {
	http.ResponseWriter
	status int
	bytes  int64
}

func (w *statusWriter) WriteHeader(code int) {
	if w.status == 0 {
		w.status = code
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Write(p []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	n, err := w.ResponseWriter.Write(p)
	w.bytes += int64(n)
	return n, err
}

func (w *statusWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (w *statusWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

// isStaticAsset 判断是否属于无需访问日志的静态资源。
func isStaticAsset(path string) bool {
	switch {
	// favicon / apple-touch-icon 也在这里：浏览器每次打开页面都会自动请求它们，
	// 不进这个白名单就变成每条控制台访问都多两行日志。
	case path == "/style.css", path == "/ping",
		path == "/favicon.ico", path == "/apple-touch-icon.png":
		return true
	case strings.HasPrefix(path, "/js/"):
		return true
	}
	return false
}

// IsStaticAsset 报告某个路径是否属于「不记访问日志」的静态资源。
func IsStaticAsset(path string) bool { return isStaticAsset(path) }

// Middleware 装上 request id、访问日志与指标。
//
// 只依赖 *Metrics：号池/车道等业务数字不参与请求计数，保持这层与 store 无关。
func Middleware(m *Metrics, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := SanitizeRequestID(r.Header.Get("X-Request-Id"))
		if id == "" {
			id = NewRequestID()
		}
		w.Header().Set("X-Request-Id", id)

		sw := &statusWriter{ResponseWriter: w}
		m.inflight.Add(1)
		start := time.Now()

		next.ServeHTTP(sw, r.WithContext(context.WithValue(r.Context(), requestIDKey{}, id)))

		d := time.Since(start)
		m.inflight.Add(-1)
		status := sw.status
		if status == 0 {
			status = http.StatusOK
		}
		if strings.HasPrefix(r.URL.Path, "/admin/") {
			m.ObserveBasic(status, d)
		} else {
			m.Observe(EndpointLabel(r.URL.Path), status, d)
		}

		if !isStaticAsset(r.URL.Path) {
			logx.Info("access",
				"request_id", id,
				"method", r.Method,
				"path", r.URL.Path,
				"status", status,
				"ms", d.Milliseconds(),
				"bytes", sw.bytes,
				"remote", guard.ClientIP(r.RemoteAddr),
			)
		}
	})
}
