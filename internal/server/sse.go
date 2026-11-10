package server

import (
	"deepaicode2api/internal/deepai"
	"encoding/json"
	"net/http"
)

// writeRawJSON 直接写出已生成的 JSON 字节。
func writeRawJSON(w http.ResponseWriter, code int, raw []byte) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_, _ = w.Write(raw)
}

// sseWriter 统一 SSE 响应头与 flush。
type sseWriter struct {
	w       http.ResponseWriter
	flusher http.Flusher
}

func newSSEWriter(w http.ResponseWriter) *sseWriter {
	flusher, _ := w.(http.Flusher)
	h := w.Header()
	h.Set("Content-Type", "text/event-stream; charset=utf-8")
	h.Set("Cache-Control", "no-cache")
	h.Set("Connection", "keep-alive")
	h.Set("X-Accel-Buffering", "no")
	// 在这里提交 200：调用方随后再写 header 会被 net/http 判为
	// superfluous（旧实现正是如此，只在日志里制造噪音）。
	w.WriteHeader(http.StatusOK)
	if flusher != nil {
		flusher.Flush()
	}
	return &sseWriter{w: w, flusher: flusher}
}

func (s *sseWriter) data(raw []byte) {
	_ = deepai.WriteSSE(s.w, raw)
	s.flush()
}

func (s *sseWriter) event(name string, raw []byte) {
	_ = deepai.WriteSSEEvent(s.w, name, raw)
	s.flush()
}

func (s *sseWriter) done() {
	_, _ = s.w.Write([]byte("data: [DONE]\n\n"))
	s.flush()
}

func (s *sseWriter) flush() {
	if s.flusher != nil {
		s.flusher.Flush()
	}
}

// jsonEvent 序列化事件载荷，失败时返回 nil。
func jsonEvent(v any) []byte {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	return raw
}
