package server

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"deepaicode2api/internal/store"
)

// 严格模式：未识别的模型名直接 404，不静默换成默认模型。
//
// 这条行为的存在意义是「测得准」：宽松回落会把 gpt-6-sol 这类拼写变成
// glm-5.3-flash，调用方以为在测 A，实际跑的是 B。
func TestStrictModelRejectsUnknown(t *testing.T) {
	u := newUpstream(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"ok"}}]}`)
	})
	s, key := newTestServer(t, u)
	s.Store.UpdateSettings(func(st *store.Settings) { st.StrictModel = true })

	rec := do(t, s, http.MethodPost, "/v1/chat/completions", key,
		`{"model":"gpt-6-sol","messages":[{"role":"user","content":"hi"}]}`)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("状态码 = %d，期望 404：%s", rec.Code, rec.Body.String())
	}
	if body := rec.Body.String(); !strings.Contains(body, "严格模式") || !strings.Contains(body, "invalid_request_error") {
		t.Fatalf("错误信息应说明严格模式与错误类型：%s", body)
	}
	u.mu.Lock()
	hits := len(u.requests)
	u.mu.Unlock()
	if hits != 0 {
		t.Fatalf("未识别的模型不应打到上游，实际 %d 次", hits)
	}

	// 目录内的模型照常放行。
	rec = do(t, s, http.MethodPost, "/v1/chat/completions", key,
		`{"model":"glm-5.3-flash","messages":[{"role":"user","content":"hi"}]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("目录内模型应通过，状态码 = %d：%s", rec.Code, rec.Body.String())
	}

	// Anthropic 与 Responses 两条协议同样生效，且保持各自的错误形状。
	rec = do(t, s, http.MethodPost, "/v1/messages", key,
		`{"model":"gpt-6-sol","max_tokens":8,"messages":[{"role":"user","content":"hi"}]}`)
	if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), `"type":"error"`) {
		t.Fatalf("Anthropic 严格模式错误形状不对: %d %s", rec.Code, rec.Body.String())
	}
	rec = do(t, s, http.MethodPost, "/v1/responses", key,
		`{"model":"gpt-6-sol","input":"hi"}`)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("Responses 严格模式状态码 = %d：%s", rec.Code, rec.Body.String())
	}
}

// 关闭严格模式时保持原有回落语义（开箱即用）。
func TestLooseModelStillFallsBack(t *testing.T) {
	u := newUpstream(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"ok"}}]}`)
	})
	s, key := newTestServer(t, u)

	rec := do(t, s, http.MethodPost, "/v1/chat/completions", key,
		`{"model":"gpt-6-sol","messages":[{"role":"user","content":"hi"}]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("宽松模式应回落而不是报错，状态码 = %d：%s", rec.Code, rec.Body.String())
	}
	if got := u.last(t).Payload["model"]; got == "gpt-6-sol" {
		t.Fatalf("上游 model 应被解析成目录模型，实际 %v", got)
	}
}

// 未知模型名要带上可执行的提示：相近候选 + 别名入口。
func TestUnknownModelMessageSuggests(t *testing.T) {
	s, _ := newTestServer(t, nil)
	s.Store.UpdateSettings(func(st *store.Settings) { st.StrictModel = true })
	msg := s.unknownModelMessage("gpt-6-sol")
	if !strings.Contains(msg, "gpt-6-sol") || !strings.Contains(msg, "/v1/models") {
		t.Fatalf("提示信息应包含模型名与可用入口: %s", msg)
	}
	if s.Store.Settings().StrictModel != true {
		t.Fatal("严格模式开关未生效")
	}
}
