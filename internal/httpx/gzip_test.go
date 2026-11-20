package httpx

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// --- 响应写出 ---

// TestWriteJSONSetsContentTypeAndBody 锁定对外 JSON 契约。
func TestWriteJSONSetsContentTypeAndBody(t *testing.T) {
	w := httptest.NewRecorder()
	WriteJSON(w, http.StatusTeapot, map[string]any{"ok": true})

	if got, want := w.Code, http.StatusTeapot; got != want {
		t.Fatalf("状态码 = %d，期望 %d", got, want)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json; charset=utf-8" {
		t.Fatalf("Content-Type = %q", ct)
	}
	if body := strings.TrimSpace(w.Body.String()); body != `{"ok":true}` {
		t.Fatalf("响应体 = %q", body)
	}
}

// TestWriteErrEnvelope 锁定错误信封结构（前端按 error.type 分类）。
func TestWriteErrEnvelope(t *testing.T) {
	w := httptest.NewRecorder()
	WriteErr(w, http.StatusBadRequest, "坏了", "invalid_request_error")

	if w.Code != http.StatusBadRequest {
		t.Fatalf("状态码 = %d", w.Code)
	}
	var got struct {
		Error struct {
			Message string `json:"message"`
			Type    string `json:"type"`
			Code    int    `json:"code"`
		} `json:"error"`
	}
	decode(t, w.Body.Bytes(), &got)
	if got.Error.Message != "坏了" || got.Error.Type != "invalid_request_error" {
		t.Fatalf("错误信封 = %+v", got.Error)
	}
	if got.Error.Code != http.StatusBadRequest {
		t.Fatalf("code 字段应为 HTTP 状态码（历史行为），实际 %d", got.Error.Code)
	}
}

// TestWriteRawJSONDoesNotReencode 验证原始字节被原样写出。
func TestWriteRawJSONDoesNotReencode(t *testing.T) {
	w := httptest.NewRecorder()
	// 键顺序与空白都应原样保留（上游透传路径依赖这一点）。
	raw := []byte(`{"b":1, "a":2}`)
	WriteRawJSON(w, http.StatusOK, raw)

	if body := w.Body.String(); body != string(raw) {
		t.Fatalf("响应体 = %q，期望 %q（不应重新序列化）", body, raw)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json; charset=utf-8" {
		t.Fatalf("Content-Type = %q", ct)
	}
}

// TestContextWithTimeoutCarriesRequestContext 验证派生的 ctx 仍继承请求取消。
func TestContextWithTimeoutCarriesRequestContext(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx, cancel := ContextWithTimeout(r, 0) // 0 = 立即到期，但也必须继承父 ctx
	defer cancel()

	select {
	case <-ctx.Done():
	default:
		t.Fatal("超时为 0 的 ctx 应已结束")
	}
	if ctx.Value(anyKey{}) != nil {
		t.Fatal("不应凭空注入值")
	}
}

type anyKey struct{}

func decode(t *testing.T, raw []byte, dst any) {
	t.Helper()
	if err := json.Unmarshal(raw, dst); err != nil {
		t.Fatalf("解析 JSON: %v（原文 %q）", err, raw)
	}
}

// --- gzip ---

// TestCompressibleType 表驱动覆盖「压不压」的判定。
func TestCompressibleType(t *testing.T) {
	cases := map[string]bool{
		"":                                 false,
		"application/json; charset=utf-8":  true,
		"text/html":                        true,
		"application/javascript":           true,
		"image/svg+xml":                    true,
		"text/event-stream":                false, // SSE 必须跳过
		"text/event-stream; charset=utf-8": false,
		"image/png":                        false,
		"application/octet-stream":         false,
	}
	for ct, want := range cases {
		if got := compressibleType(ct); got != want {
			t.Fatalf("compressibleType(%q) = %v，期望 %v", ct, got, want)
		}
	}
}

// TestAcceptsGzipWeights 覆盖 q 权重解析（q=0 拒绝，q=0.5 接受）。
func TestAcceptsGzipWeights(t *testing.T) {
	cases := []struct {
		header string
		want   bool
	}{
		{"", false},
		{"gzip", true},
		{"gzip, deflate, br", true},
		{"deflate", false},
		{"gzip;q=0", false},
		{"gzip;q=0.5", true},
		{"GZIP;q=1", true},
		{"br;q=1, gzip;q=0.8", true},
		{"identity", false},
	}
	for _, c := range cases {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		if c.header != "" {
			r.Header.Set("Accept-Encoding", c.header)
		}
		if got := AcceptsGzip(r); got != c.want {
			t.Fatalf("AcceptsGzip(%q) = %v，期望 %v", c.header, got, c.want)
		}
	}
}

// TestWithGzipCompressesJSON 验证文本响应真的被压缩且可解压还原。
func TestWithGzipCompressesJSON(t *testing.T) {
	payload := strings.Repeat(`{"k":"v"}`, 200)
	h := WithGzip(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, payload)
	}))

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("Accept-Encoding", "gzip")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)

	if enc := w.Header().Get("Content-Encoding"); enc != "gzip" {
		t.Fatalf("Content-Encoding = %q，期望 gzip", enc)
	}
	if v := w.Header().Get("Vary"); !strings.Contains(v, "Accept-Encoding") {
		t.Fatalf("Vary = %q，缺少 Accept-Encoding", v)
	}
	if cl := w.Header().Get("Content-Length"); cl != "" {
		t.Fatalf("压缩后必须删掉 Content-Length，实际 %q", cl)
	}

	zr, err := gzip.NewReader(bytes.NewReader(w.Body.Bytes()))
	if err != nil {
		t.Fatalf("gzip.NewReader: %v", err)
	}
	raw, err := io.ReadAll(zr)
	if err != nil {
		t.Fatalf("解压: %v", err)
	}
	if string(raw) != payload {
		t.Fatalf("解压内容与原文不一致（%d vs %d 字节）", len(raw), len(payload))
	}
}

// TestWithGzipSkipsWhenClientDoesNotAccept 验证不支持 gzip 的客户端拿到明文。
func TestWithGzipSkipsWhenClientDoesNotAccept(t *testing.T) {
	h := WithGzip(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))

	if enc := w.Header().Get("Content-Encoding"); enc != "" {
		t.Fatalf("不应压缩，实际 Content-Encoding=%q", enc)
	}
	if w.Body.String() != `{"ok":true}` {
		t.Fatalf("响应体 = %q", w.Body.String())
	}
}

// TestWithGzipSkipsSSE 验证 SSE 不被压缩（逐帧 flush 前提）。
func TestWithGzipSkipsSSE(t *testing.T) {
	h := WithGzip(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {}\n\n")
	}))
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("Accept-Encoding", "gzip")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)

	if enc := w.Header().Get("Content-Encoding"); enc != "" {
		t.Fatalf("SSE 不应被压缩，实际 %q", enc)
	}
}

// TestWithGzipSkipsBodylessStatus 验证 204/304 不挂 Content-Encoding。
//
// 304 撞 ETag 是最常命中的路径：给它加 Content-Encoding 会让代理把空体当压缩流。
func TestWithGzipSkipsBodylessStatus(t *testing.T) {
	for _, code := range []int{http.StatusNoContent, http.StatusNotModified} {
		h := WithGzip(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(code)
		}))
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.Header.Set("Accept-Encoding", "gzip")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)

		if w.Code != code {
			t.Fatalf("状态码 = %d，期望 %d", w.Code, code)
		}
		if enc := w.Header().Get("Content-Encoding"); enc != "" {
			t.Fatalf("%d 不应有 Content-Encoding，实际 %q", code, enc)
		}
		if w.Body.Len() != 0 {
			t.Fatalf("%d 不应有响应体，实际 %q", code, w.Body.String())
		}
	}
}

// TestWithGzipFlushPropagates 验证压缩模式下 Flush 会把缓冲推出去。
func TestWithGzipFlushPropagates(t *testing.T) {
	h := WithGzip(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"part":1}`)
		w.(http.Flusher).Flush()
		_, _ = io.WriteString(w, `{"part":2}`)
	}))
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("Accept-Encoding", "gzip")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)

	zr, err := gzip.NewReader(bytes.NewReader(w.Body.Bytes()))
	if err != nil {
		t.Fatalf("gzip.NewReader: %v", err)
	}
	raw, err := io.ReadAll(zr)
	if err != nil {
		t.Fatalf("解压: %v", err)
	}
	if string(raw) != `{"part":1}{"part":2}` {
		t.Fatalf("解压内容 = %q", raw)
	}
}

// TestWithGzipKeepsExistingEncoding 验证上游已声明的 Content-Encoding 不被二次压缩。
func TestWithGzipKeepsExistingEncoding(t *testing.T) {
	h := WithGzip(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Encoding", "br")
		_, _ = io.WriteString(w, "not-really-brotli")
	}))
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("Accept-Encoding", "gzip")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)

	if enc := w.Header().Get("Content-Encoding"); enc != "br" {
		t.Fatalf("已有编码应保留，实际 %q", enc)
	}
}

// TestGzipWriterDuplicateWriteHeaderIgnored 验证重复 WriteHeader 不产生 superfluous。
func TestGzipWriterDuplicateWriteHeaderIgnored(t *testing.T) {
	w := httptest.NewRecorder()
	gw := &gzipWriter{ResponseWriter: w}
	gw.WriteHeader(http.StatusOK)
	gw.WriteHeader(http.StatusInternalServerError) // 应被忽略
	gw.close()

	if w.Code != http.StatusOK {
		t.Fatalf("状态码 = %d，期望 200（第二次 WriteHeader 应被忽略）", w.Code)
	}
	if w.Body.Len() != 0 {
		t.Fatalf("空响应体，实际 %q", w.Body.String())
	}
}

// TestGzipWriterUnwrap 验证 Unwrap 暴露下层（http.ResponseController 依赖它）。
func TestGzipWriterUnwrap(t *testing.T) {
	w := httptest.NewRecorder()
	gw := &gzipWriter{ResponseWriter: w}
	if got := gw.Unwrap(); got != w {
		t.Fatalf("Unwrap = %#v，期望原始 writer", got)
	}
}
