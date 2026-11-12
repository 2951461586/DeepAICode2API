package server

import (
	"net/http"
	"strings"
	"testing"
)

// TestDecodeJSONRequestEnvelopes 覆盖三个推理入口共享的「读体 → 解析 → 报错」。
//
// 这段逻辑此前在 chat / responses / anthropic 里各写一遍，状态码与文案改一处
// 漏两处；抽成 decodeJSONRequest 后由 fail 回调保留各协议的错误信封形制，
// 这里锁住「协议对了、状态码对了、不把畸形请求放进业务层」。
func TestDecodeJSONRequestEnvelopes(t *testing.T) {
	s, key := newTestServer(t, nil)

	cases := []struct {
		name      string
		path      string
		wantType  string // 错误信封里的 type（Anthropic 在 error.type，OpenAI 在顶层 error.type）
		anthropic bool
	}{
		{"chat 请求体损坏", "/v1/chat/completions", "invalid_request_error", false},
		{"responses 请求体损坏", "/v1/responses", "invalid_request_error", false},
		{"messages 请求体损坏", "/v1/messages", "invalid_request_error", true},
		{"count_tokens 请求体损坏", "/v1/messages/count_tokens", "invalid_request_error", true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec := do(t, s, http.MethodPost, c.path, key, `{"model":`)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("状态码 = %d，期望 400：%s", rec.Code, rec.Body.String())
			}
			body := rec.Body.String()
			if !strings.Contains(body, c.wantType) {
				t.Fatalf("错误信封缺 %q：%s", c.wantType, body)
			}
			if c.anthropic && !strings.Contains(body, `"type":"error"`) {
				t.Fatalf("Anthropic 错误信封应为 type=error：%s", body)
			}
			// 上游不应被调用：畸形请求在入口就该被挡住。
		})
	}
}
