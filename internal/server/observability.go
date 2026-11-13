package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"deepaicode2api/internal/deepai"
	"deepaicode2api/internal/logx"
)

// itoa64 / ftoa 是 Prometheus 文本格式的必填转换。
func itoa64(v int64) string { return strconv.FormatInt(v, 10) }
func ftoa(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }

// --- 可观测性：request id / 访问日志 / /metrics ---
//
// 三件事放在一层中间件里完成，而不是散在各 handler：
//   - request id：生成并回写 X-Request-Id（CORS 早已在 Expose-Headers 里
//     声明了它，但此前从未真正生成过），日志与响应因此可以互相对上；
//   - 访问日志：一行 JSON（method/path/status/ms/bytes/remote/request_id），
//     静态资源除外，避免刷屏；
//   - 指标：把状态码分类、耗时总计、在途请求数记进原子计数器，供 /metrics 导出。

type requestIDKey struct{}

// requestID 从上下文取 request id（没有则返回空串）。
func requestID(ctx context.Context) string {
	if v, ok := ctx.Value(requestIDKey{}).(string); ok {
		return v
	}
	return ""
}

// newRequestID 生成 16 位 hex 的请求号。
func newRequestID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		// 随机源不可用时退化为时间戳：request id 只用于关联日志，不需要密码学强度。
		return fmt.Sprintf("%016x", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}

// sanitizeRequestID 只接受长度可控、字符集安全的传入 id（便于跨服务串联）。
func sanitizeRequestID(s string) string {
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

// metrics 是进程内的轻量指标计数（不持久化、重启清零）。
type metrics struct {
	requests atomic.Int64
	c2xx     atomic.Int64
	c3xx     atomic.Int64
	c4xx     atomic.Int64
	c5xx     atomic.Int64
	durSumNS atomic.Int64
	durCount atomic.Int64
	inflight atomic.Int64

	// 扩展指标（见 metrics.go）：签到 / 上游错误 / 兜底 / 分端点延迟。
	checkinAttempts atomic.Int64
	checkinBlocked  atomic.Int64
	checkinTimeouts atomic.Int64
	fallbackTotal   atomic.Int64
	extra           *extraMetrics
}

func newMetrics() *metrics { return &metrics{extra: newExtraMetrics()} }

// observe 记录一次请求（status + 耗时 + 端点分布）。
func (m *metrics) observe(endpoint string, status int, d time.Duration) {
	m.observeBasic(status, d)
	m.observeDuration(endpoint, d.Seconds())
}

// observeBasic 只记状态码与总耗时，不进分端点 / 分位直方图。
//
// /admin/* 用它：控制台自己的请求如果也进延迟直方图，/admin/api/overview
// 的响应体就会因为「上一次 overview 的耗时」而每次变化，ETag 永远命中不了
// （TestOverviewETag 就是这么暴露的）。管理接口不是延迟 SLI。
func (m *metrics) observeBasic(status int, d time.Duration) {
	m.requests.Add(1)
	switch {
	case status >= 500:
		m.c5xx.Add(1)
	case status >= 400:
		m.c4xx.Add(1)
	case status >= 300:
		m.c3xx.Add(1)
	default:
		m.c2xx.Add(1)
	}
	m.durSumNS.Add(int64(d))
	m.durCount.Add(1)
}

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

// withObservability 装上 request id、访问日志与指标。
func (s *Server) withObservability(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := sanitizeRequestID(r.Header.Get("X-Request-Id"))
		if id == "" {
			id = newRequestID()
		}
		w.Header().Set("X-Request-Id", id)

		sw := &statusWriter{ResponseWriter: w}
		m := s.metrics
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
			m.observeBasic(status, d)
		} else {
			m.observe(endpointLabel(r.URL.Path), status, d)
		}

		if !isStaticAsset(r.URL.Path) {
			logx.Info("access",
				"request_id", id,
				"method", r.Method,
				"path", r.URL.Path,
				"status", status,
				"ms", d.Milliseconds(),
				"bytes", sw.bytes,
				"remote", clientIP(r.RemoteAddr),
			)
		}
	})
}

// poolSnapshot 是号池健康度的一次快照（控制台与 /metrics 共用口径）。
type poolSnapshot struct {
	Total, Ready, Degraded, Blocked int
	QuotaFresh, QuotaDue            int
}

// poolSnapshot 计算号池健康度。
//
// 口径与 /health 完全一致（AccountReady / AccountDegraded / 额度时效），
// 抽出来是为了避免控制台徽章与 Prometheus 指标给出互相矛盾的数。
func (s *Server) poolSnapshot() poolSnapshot {
	var out poolSnapshot
	now := time.Now()
	for _, a := range s.Store.Accounts() {
		out.Total++
		if deepai.AccountReady(a, now) {
			out.Ready++
		}
		if deepai.AccountDegraded(a, now) {
			out.Degraded++
		}
		if a.Enabled && a.Status == deepai.StatusBlocked {
			out.Blocked++
		}
		if a.Theos != nil && a.Theos.CheckedAt > 0 &&
			now.Sub(time.Unix(a.Theos.CheckedAt, 0)) < quotaStaleAfter {
			out.QuotaFresh++
		}
		if quotaNeedsRefresh(a, now, quotaGuardStaleAfter) {
			out.QuotaDue++
		}
	}
	return out
}

// handleMetrics 输出 Prometheus 文本格式指标。
//
// 挂 withAdminAuth：里面含号池规模与请求量，和 /admin/api/* 同级。
func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErr(w, http.StatusMethodNotAllowed, "仅支持 GET", "invalid_request_error")
		return
	}
	m := s.metrics
	pool := s.poolSnapshot()
	stats := s.Store.Stats()

	var b strings.Builder
	metric := func(name, typ, help string, samples ...[2]string) {
		fmt.Fprintf(&b, "# HELP %s %s\n# TYPE %s %s\n", name, help, name, typ)
		for _, s := range samples {
			fmt.Fprintf(&b, "%s %s\n", s[0], s[1])
		}
	}
	counter := func(name, help string, samples ...[2]string) { metric(name, "counter", help, samples...) }
	gauge := func(name, help string, samples ...[2]string) { metric(name, "gauge", help, samples...) }

	counter("gateway_requests_total", "网关处理的请求总数（按状态码分类）",
		[2]string{`gateway_requests_total{class="2xx"}`, itoa64(m.c2xx.Load())},
		[2]string{`gateway_requests_total{class="3xx"}`, itoa64(m.c3xx.Load())},
		[2]string{`gateway_requests_total{class="4xx"}`, itoa64(m.c4xx.Load())},
		[2]string{`gateway_requests_total{class="5xx"}`, itoa64(m.c5xx.Load())},
	)
	// 耗时用 sum + count 表示：不保留分位数，避免进程内维护直方图的开销。
	counter("gateway_request_duration_seconds_sum", "请求耗时总和（秒）",
		[2]string{"gateway_request_duration_seconds_sum", ftoa(float64(m.durSumNS.Load()) / float64(time.Second))})
	counter("gateway_request_duration_seconds_count", "请求耗时样本数",
		[2]string{"gateway_request_duration_seconds_count", itoa64(m.durCount.Load())})
	gauge("gateway_inflight_requests", "当前在途请求数",
		[2]string{"gateway_inflight_requests", itoa64(m.inflight.Load())})
	gauge("gateway_accounts", "号池账号数量（按状态）",
		[2]string{`gateway_accounts{state="total"}`, itoa64(int64(pool.Total))},
		[2]string{`gateway_accounts{state="ready"}`, itoa64(int64(pool.Ready))},
		[2]string{`gateway_accounts{state="degraded"}`, itoa64(int64(pool.Degraded))},
		[2]string{`gateway_accounts{state="blocked"}`, itoa64(int64(pool.Blocked))},
		[2]string{`gateway_accounts{state="quota_fresh"}`, itoa64(int64(pool.QuotaFresh))},
		[2]string{`gateway_accounts{state="quota_due"}`, itoa64(int64(pool.QuotaDue))},
	)
	gauge("gateway_log_requests_total", "请求日志累计条数（stats 口径）",
		[2]string{"gateway_log_requests_total", itoa64(stats.TotalRequests)})
	gauge("gateway_log_failed_total", "失败请求累计条数（stats 口径）",
		[2]string{"gateway_log_failed_total", itoa64(stats.FailedRequests)})
	gauge("gateway_keys", "有效 API 密钥数量",
		[2]string{"gateway_keys", itoa64(int64(len(s.Store.Keys())))})
	gauge("go_goroutines", "当前 goroutine 数",
		[2]string{"go_goroutines", itoa64(int64(runtime.NumGoroutine()))})
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	gauge("go_memstats_heap_alloc_bytes", "堆上已分配字节数",
		[2]string{"go_memstats_heap_alloc_bytes", itoa64(int64(ms.HeapAlloc))})

	// 扩展指标（metrics.go）：分端点延迟、签到、兜底、上游错误码。
	snap := m.snapshot()
	endpoints := make([]string, 0, len(snap.ByEndpoint))
	for k := range snap.ByEndpoint {
		endpoints = append(endpoints, k)
	}
	sort.Strings(endpoints)
	for _, ep := range endpoints {
		appendHistogram(&b, "gateway_request_duration_seconds",
			"HTTP 请求耗时（按端点）", "endpoint", ep, snap.ByEndpoint[ep])
	}
	appendHistogram(&b, "gateway_checkin_seconds", "推理前同步签到的等待耗时", "", "", snap.CheckIn)
	appendHistogram(&b, "gateway_admission_wait_seconds", "账号全忙时的排队等待耗时", "", "", snap.Admission)

	counter("gateway_checkin_total", "推理前的签到尝试（attempt / 慢阻塞 / 超预算）",
		[2]string{`gateway_checkin_total{result="attempt"}`, itoa64(m.checkinAttempts.Load())},
		[2]string{`gateway_checkin_total{result="blocked"}`, itoa64(m.checkinBlocked.Load())},
		[2]string{`gateway_checkin_total{result="timeout"}`, itoa64(m.checkinTimeouts.Load())},
	)
	counter("gateway_fallback_total", "请求体形状被拒后按桌面端形状重试的次数",
		[2]string{`gateway_fallback_total{reason="body_shape"}`, itoa64(m.fallbackTotal.Load())},
	)
	errSamples := make([][2]string, 0, len(snap.UpstreamErrs))
	for _, e := range snap.UpstreamErrs {
		labels := fmt.Sprintf(`code=%q,status="%d",path=%q`, e.Code, e.Status, e.Path)
		errSamples = append(errSamples, [2]string{"gateway_upstream_errors_total{" + labels + "}", itoa64(e.Count)})
	}
	if len(errSamples) > 0 {
		counter("gateway_upstream_errors_total", "上游拒绝次数（按路径 / 错误码 / 状态码）", errSamples...)
	}

	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte(b.String()))
}
