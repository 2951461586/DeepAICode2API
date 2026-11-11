package server

import (
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
)

var hex16 = regexp.MustCompile(`^[0-9a-f]{16}$`)

// TestRequestIDGeneratedAndEchoed 守住 request id 中间件。
//
// 这条能力此前只存在于 CORS 的 Expose-Headers 声明里（声明了却从不生成），
// 日志与响应因此无法互相对上。
func TestRequestIDGeneratedAndEchoed(t *testing.T) {
	s, _ := newTestServer(t, nil)

	rec := do(t, s, http.MethodGet, "/ping", "", "")
	if got := rec.Header().Get("X-Request-Id"); !hex16.MatchString(got) {
		t.Fatalf("应生成 16 位 hex request id，实际 %q", got)
	}

	// 合法传入值原样回显（便于跨服务串联）。
	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	req.Header.Set("X-Request-Id", "trace-abc_123")
	rec2 := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec2, req)
	if got := rec2.Header().Get("X-Request-Id"); got != "trace-abc_123" {
		t.Fatalf("合法 request id 应回显，实际 %q", got)
	}

	// 非法传入值必须被替换（避免日志注入 / 头污染）。
	req3 := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req3.RemoteAddr = "127.0.0.1:1234"
	req3.Header.Set("X-Request-Id", "bad id\nwith newline")
	rec3 := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec3, req3)
	if got := rec3.Header().Get("X-Request-Id"); !hex16.MatchString(got) {
		t.Fatalf("非法 request id 应被替换，实际 %q", got)
	}
}

// TestMetricsEndpoint 验证 /metrics 的格式与鉴权。
func TestMetricsEndpoint(t *testing.T) {
	s, _ := newTestServer(t, nil)
	do(t, s, http.MethodGet, "/health", "", "") // 造一条请求计数

	rec := do(t, s, http.MethodGet, "/metrics", "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("/metrics = %d：%s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/plain") {
		t.Fatalf("Content-Type = %q，应为 Prometheus 文本格式", ct)
	}
	body := rec.Body.String()
	for _, want := range []string{
		"# TYPE gateway_requests_total counter",
		`gateway_requests_total{class="2xx"}`,
		"gateway_request_duration_seconds_sum",
		"gateway_inflight_requests",
		`gateway_accounts{state="total"}`,
		"go_goroutines",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("指标输出缺少 %q：\n%s", want, body)
		}
	}
	if rec := do(t, s, http.MethodPost, "/metrics", "", ""); rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST /metrics 应回 405，实际 %d", rec.Code)
	}
}

// TestGzipCompressesJSON 验证 JSON 响应会被压缩且内容可还原。
func TestGzipCompressesJSON(t *testing.T) {
	s, _ := newTestServer(t, nil)

	req := httptest.NewRequest(http.MethodGet, "/admin/api/overview", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	req.Header.Set("Accept-Encoding", "gzip")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("状态码 = %d：%s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Encoding"); got != "gzip" {
		t.Fatalf("Content-Encoding = %q，应为 gzip", got)
	}
	if !strings.Contains(rec.Header().Get("Vary"), "Accept-Encoding") {
		t.Fatalf("缺少 Vary: Accept-Encoding，代理可能缓存错版本")
	}
	zr, err := gzip.NewReader(rec.Body)
	if err != nil {
		t.Fatalf("解压失败: %v", err)
	}
	raw, err := io.ReadAll(zr)
	if err != nil {
		t.Fatalf("读取解压内容失败: %v", err)
	}
	if !strings.HasPrefix(strings.TrimSpace(string(raw)), "{") {
		t.Fatalf("解压后不是 JSON：%s", raw)
	}
}

// TestGzipSkipsSSE 验证流式响应不压缩（逐帧 flush 语义不能破坏）。
func TestGzipSkipsSSE(t *testing.T) {
	h := withGzip(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("data: {\"a\":1}\n\n"))
	}))
	req := httptest.NewRequest(http.MethodGet, "/v1/chat/completions", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if enc := rec.Header().Get("Content-Encoding"); enc != "" {
		t.Fatalf("SSE 不应被压缩，Content-Encoding = %q", enc)
	}
	if body := rec.Body.String(); !strings.Contains(body, "data:") {
		t.Fatalf("SSE 原文应原样输出：%q", body)
	}
}

// TestGzipSkipsBodylessStatus 验证 304/204 不会被套上 Content-Encoding。
//
// 资产命中 ETag 回 304 时最容易踩：304 没有响应体，带上 gzip 头会让代理
// 与部分客户端把空体当成压缩流解析。
func TestGzipSkipsBodylessStatus(t *testing.T) {
	for _, code := range []int{http.StatusNotModified, http.StatusNoContent} {
		h := withGzip(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(code)
		}))
		req := httptest.NewRequest(http.MethodGet, "/js/app.js", nil)
		req.Header.Set("Accept-Encoding", "gzip")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if enc := rec.Header().Get("Content-Encoding"); enc != "" {
			t.Fatalf("状态 %d 不应有 Content-Encoding，实际 %q", code, enc)
		}
		if rec.Body.Len() != 0 {
			t.Fatalf("状态 %d 不应有响应体，实际 %d 字节", code, rec.Body.Len())
		}
	}
}

// TestGzipHonorsQZero 验证 gzip;q=0 被尊重（客户端明确拒绝）。
func TestGzipHonorsQZero(t *testing.T) {
	if acceptsGzip(reqWithEncoding("gzip;q=0")) {
		t.Fatal("gzip;q=0 应视为拒绝")
	}
	if !acceptsGzip(reqWithEncoding("deflate, gzip;q=0.5")) {
		t.Fatal("gzip;q=0.5 应视为接受")
	}
	if acceptsGzip(reqWithEncoding("")) {
		t.Fatal("空 Accept-Encoding 不应压缩")
	}
}

func reqWithEncoding(enc string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	if enc != "" {
		r.Header.Set("Accept-Encoding", enc)
	}
	return r
}
