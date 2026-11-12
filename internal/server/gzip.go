package server

import (
	"compress/gzip"
	"net/http"
	"strconv"
	"strings"
)

// --- 响应压缩 ---
//
// 控制台的 /admin/api/accounts 一页默认 50 个账号（约 20~40KB JSON），
// /admin/api/overview 带着模型目录也有十几 KB；局域网 / 远程部署时这些
// 都是纯文本，gzip 通常能压到 1/5~1/10。内嵌的 JS / CSS 同理。
//
// 只压「明确可压」的类型，并且**跳过 SSE**：流式响应压缩后必须逐帧 flush，
// 很多客户端不认，收益也接近于零。

// compressibleType 判断响应该不该压缩。
func compressibleType(ct string) bool {
	ct = strings.ToLower(strings.TrimSpace(ct))
	if ct == "" {
		return false
	}
	// SSE 必须跳过：逐帧 flush + gzip 会让客户端拿不到增量。
	if strings.HasPrefix(ct, "text/event-stream") {
		return false
	}
	switch {
	case strings.HasPrefix(ct, "application/json"),
		strings.HasPrefix(ct, "text/"),
		strings.HasPrefix(ct, "application/javascript"),
		strings.HasPrefix(ct, "application/xml"),
		strings.HasPrefix(ct, "image/svg+xml"):
		return true
	}
	return false
}

// gzipWriter 按 Content-Type 决定是否真的压缩。
//
// 之所以延迟到 WriteHeader 才决定：handler 通常先 Set("Content-Type") 再
// WriteHeader，此时类型已知；提前决定就得让每个 handler 自己声明。
type gzipWriter struct {
	http.ResponseWriter
	gz        *gzip.Writer
	decided   bool
	compress  bool
	wroteHead bool
}

func (w *gzipWriter) decide() {
	if w.decided {
		return
	}
	w.decided = true
	ct := w.Header().Get("Content-Type")
	// 已有 Content-Encoding（例如上游透传）就不再套一层。
	if w.Header().Get("Content-Encoding") != "" || !compressibleType(ct) {
		return
	}
	w.compress = true
	w.Header().Del("Content-Length")
	w.Header().Add("Vary", "Accept-Encoding")
	w.Header().Set("Content-Encoding", "gzip")
	w.gz = gzip.NewWriter(w.ResponseWriter)
}

func (w *gzipWriter) WriteHeader(code int) {
	if w.wroteHead {
		return
	}
	w.wroteHead = true
	// 1xx / 204 / 304 没有响应体：套上 Content-Encoding 会让代理与客户端
	// 把一个空体当成压缩流（304 撞 ETag 时最容易命中）。
	if code >= 200 && code != http.StatusNoContent && code != http.StatusNotModified {
		w.decide()
	} else {
		w.decided = true
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *gzipWriter) Write(p []byte) (int, error) {
	if !w.wroteHead {
		w.WriteHeader(http.StatusOK)
	}
	if w.compress {
		return w.gz.Write(p)
	}
	return w.ResponseWriter.Write(p)
}

// Flush 在压缩模式下先 flush gzip（否则压缩缓冲会让增量卡住）。
func (w *gzipWriter) Flush() {
	if w.compress {
		_ = w.gz.Flush()
	}
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (w *gzipWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

// close 结束压缩流；只在真的压缩过时才有意义。
func (w *gzipWriter) close() {
	if w.compress && w.gz != nil {
		_ = w.gz.Close()
	}
}

// withGzip 对声明支持 gzip 的客户端压缩文本响应。
func withGzip(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !acceptsGzip(r) {
			next.ServeHTTP(w, r)
			return
		}
		gw := &gzipWriter{ResponseWriter: w}
		defer gw.close()
		next.ServeHTTP(gw, r)
	})
}

// acceptsGzip 判断客户端是否接受 gzip（支持 q 权重，q=0 表示拒绝）。
//
// 不能简单地判断 `strings.Contains(params, "q=0")`：那会把 q=0.5（接受）
// 也当成拒绝。这里按 "q=" 后面的数字取值比较。
func acceptsGzip(r *http.Request) bool {
	for _, part := range strings.Split(r.Header.Get("Accept-Encoding"), ",") {
		enc, params, _ := strings.Cut(strings.TrimSpace(part), ";")
		if !strings.EqualFold(strings.TrimSpace(enc), "gzip") {
			continue
		}
		return encodingWeight(params) > 0
	}
	return false
}

// encodingWeight 从 ";q=0.5" 这类参数里取权重，缺省为 1。
func encodingWeight(params string) float64 {
	for _, p := range strings.Split(params, ";") {
		k, v, ok := strings.Cut(strings.TrimSpace(p), "=")
		if !ok || !strings.EqualFold(strings.TrimSpace(k), "q") {
			continue
		}
		if w, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil {
			return w
		}
		return 0
	}
	return 1
}
