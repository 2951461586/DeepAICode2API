package server

import (
	"bytes"
	"errors"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"deepaicode2api/internal/respstate"
	"deepaicode2api/internal/store"
)

// TestBodyCapReached 覆盖 issue #7 的两种处置。
func TestBodyCapReached(t *testing.T) {
	base := store.Settings{}.Normalize()
	big := bytes.Repeat([]byte("x"), base.ContextBodyBytes+1)
	small := bytes.Repeat([]byte("x"), 1024)

	// warn（默认）：超过上限也照常透传，由 reqlog 打 body_near_cap 标记。
	if err := bodyCapReached(base, big); err != nil {
		t.Fatalf("warn 模式不应拒绝，实际 %v", err)
	}

	reject := base
	reject.BodyCapMode = store.BodyCapReject
	if err := bodyCapReached(reject, small); err != nil {
		t.Fatalf("未超限不应拒绝，实际 %v", err)
	}
	err := bodyCapReached(reject, big)
	if err == nil {
		t.Fatal("reject 模式应拒绝超限请求体")
	}
	var bce *bodyCapError
	if !errors.As(err, &bce) {
		t.Fatalf("错误类型应是 *bodyCapError，实际 %T", err)
	}
	if bce.Bytes != len(big) || bce.Limit != base.ContextBodyBytes {
		t.Fatalf("错误里应带字节数与上限，实际 bytes=%d limit=%d", bce.Bytes, bce.Limit)
	}
	// 日志状态码必须是 400：记成 502 会让「调用方请求过大」和「代理故障」混在一起。
	if got := StatusOf(err); got != http.StatusBadRequest {
		t.Fatalf("StatusOf = %d，期望 400", got)
	}
	if !strings.Contains(err.Error(), "issue #7") {
		t.Fatalf("错误文案应指出 issue #7，实际 %q", err.Error())
	}
}

// TestChatRejectsOversizedBodyWhenConfigured 端到端确认 reject 模式：
// 请求在选号之前就被拒（上游一次都没被调用），回 400 invalid_request_error。
func TestChatRejectsOversizedBodyWhenConfigured(t *testing.T) {
	var upstreamCalls atomic.Int32
	u := newUpstream(t, func(w http.ResponseWriter, r *http.Request) {
		upstreamCalls.Add(1)
		http.Error(w, "不该被调用", http.StatusInternalServerError)
	})
	s, key := newTestServer(t, u)

	// 上限压到最小合法值，处置设为 reject。
	cap := store.MinContextBodyBytes
	reject := store.BodyCapReject
	s.Store.UpdateSettings(func(st *store.Settings) {
		st.ContextBodyBytes = cap
		st.BodyCapMode = reject
	})
	respstate.SetBodyLimit(cap)
	t.Cleanup(func() { respstate.SetBodyLimit(0) })

	padding := strings.Repeat("x", cap+1024)
	body := `{"model":"glm-5.3-flash","messages":[{"role":"user","content":"` + padding + `"}]}`
	rec := do(t, s, http.MethodPost, "/v1/chat/completions", key, body)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("状态码 = %d，期望 400；body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "invalid_request_error") {
		t.Fatalf("错误类型应为 invalid_request_error，实际 %s", rec.Body.String())
	}
	if n := upstreamCalls.Load(); n != 0 {
		t.Fatalf("请求体超限应在选号前被拒，实际上游被调用 %d 次", n)
	}
}
