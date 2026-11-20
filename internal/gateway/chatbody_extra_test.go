package gateway

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"

	"deepaicode2api/internal/deepai"
)

// mustErr 构造一个带指定状态码与响应体的上游错误。
//
// 这几个用例断言的是「网关在收到 400 形状拒绝后如何重写请求体」，
// 所以既用到 gateway.ChatBodyFallback，也用到 deepai.HTTPError 的判定方法；
// 测试专用地引入 deepai 不产生环（生产代码里 gateway 已不依赖 deepai）。
func mustErr(t *testing.T, status int, body string) *deepai.HTTPError {
	t.Helper()
	return &deepai.HTTPError{Op: "上游推理", Status: status, Body: []byte(body)}
}

// TestEnsureSystemMessageAddsOnlyWhenMissing 固化「预置形状只补 system」。
func TestEnsureSystemMessageAddsOnlyWhenMissing(t *testing.T) {
	body := []byte(`{"model":"m","temperature":0.3,"messages":[{"role":"user","content":"hi"}]}`)
	out, inserted := EnsureSystemMessage(body, "")
	if !inserted {
		t.Fatal("缺少 system 应插入")
	}
	var payload map[string]any
	if err := json.Unmarshal(out, &payload); err != nil {
		t.Fatalf("输出不是合法 JSON: %v", err)
	}
	msgs := payload["messages"].([]any)
	if len(msgs) != 2 {
		t.Fatalf("messages 长度 = %d，期望 2", len(msgs))
	}
	if msgs[0].(map[string]any)["role"] != "system" {
		t.Fatalf("第一条应为 system: %#v", msgs[0])
	}
	// 其它字段必须原样保留（不能像 ChatBodyFallback 那样裁剪）。
	if payload["temperature"].(float64) != 0.3 {
		t.Fatalf("temperature 被改写: %#v", payload["temperature"])
	}
	if !bytes.Contains(out, []byte(`"model":"m"`)) {
		t.Fatalf("model 丢失: %s", out)
	}

	// 已有 system（或 developer）时不动。
	for _, role := range []string{"system", "developer"} {
		with := []byte(`{"model":"m","messages":[{"role":"` + role + `","content":"s"},{"role":"user","content":"hi"}]}`)
		out2, inserted := EnsureSystemMessage(with, "")
		if inserted {
			t.Fatalf("已有 %s 时不应插入", role)
		}
		if !bytes.Equal(out2, with) {
			t.Fatalf("已有 %s 时不应改写请求体", role)
		}
	}

	// 非法 / 空 messages：不插入、返回 false。
	for _, bad := range []string{`not json`, `{}`, `{"messages":[]}`} {
		if _, ins := EnsureSystemMessage([]byte(bad), ""); ins {
			t.Fatalf("body=%q 不应插入", bad)
		}
	}
}

// 从 internal/deepai 迁入：形状推断与字段白名单属于协议适配层。

// 400 形状拒绝 -> 桌面端字段白名单 + 补 system 消息。
func TestChatBodyFallback(t *testing.T) {
	body := []byte(`{"model":"m","stream":true,"user":"u1","messages":[{"role":"user","content":"hi"}],"tools":[]}`)
	rejected := mustErr(t, http.StatusBadRequest,
		`{"error":{"message":"This request is not one DeepAI Code sends.","code":"invalid_request_error"}}`)
	if !rejected.BodyRejected() {
		t.Fatal("应识别为请求体形状拒绝")
	}

	out, ok := ChatBodyFallback(body, "")
	if !ok {
		t.Fatal("可解析的 messages 应能兜底")
	}
	var got map[string]any
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("兜底结果不是合法 JSON: %v", err)
	}
	if _, exists := got["user"]; exists {
		t.Fatal("白名单外的字段应被剔除")
	}
	if got["stream"] != true || got["model"] != "m" {
		t.Fatalf("白名单字段应保留: %#v", got)
	}
	msgs, _ := got["messages"].([]any)
	if len(msgs) != 2 {
		t.Fatalf("应插入一条 system 消息，实际 %d 条", len(msgs))
	}
	if role, _ := msgs[0].(map[string]any)["role"].(string); role != "system" {
		t.Fatalf("第一条应为 system，实际 %q", role)
	}

	if _, ok := ChatBodyFallback([]byte(`{"model":"m"}`), ""); ok {
		t.Fatal("没有 messages 时不应兜底")
	}
}

// 已带 system 的请求不应被重复插入。
func TestChatBodyFallbackKeepsExistingSystem(t *testing.T) {
	body := []byte(`{"model":"m","messages":[{"role":"system","content":"s"},{"role":"user","content":"hi"}]}`)
	out, ok := ChatBodyFallback(body, "")
	if !ok {
		t.Fatal("应能兜底")
	}
	var got map[string]any
	_ = json.Unmarshal(out, &got)
	if msgs, _ := got["messages"].([]any); len(msgs) != 2 {
		t.Fatalf("不应插入额外 system，实际 %d 条", len(msgs))
	}
}

// 形状回退不能顺手改掉采样条件：seed / service_tier / stream_options
// 会改变采样与用量上报，指纹类检测甚至会因此换一个候选模型。
// store 则是桌面端每次请求都带的字段，必须同样保留。
func TestChatBodyFallbackKeepsSamplingFields(t *testing.T) {
	body := []byte(`{"model":"m","messages":[{"role":"user","content":"hi"}],` +
		`"seed":7,"service_tier":"flex","stream":true,"store":false,` +
		`"stream_options":{"include_usage":true},"presence_penalty":0.5,"user":"u1"}`)
	out, ok := ChatBodyFallback(body, "")
	if !ok {
		t.Fatal("应能兜底")
	}
	var got map[string]any
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("兜底结果不是 JSON: %v", err)
	}
	for _, k := range []string{"seed", "service_tier", "stream_options", "store"} {
		if _, exists := got[k]; !exists {
			t.Errorf("白名单丢了 %s，会改变采样条件或用量上报", k)
		}
	}
	if v, exists := got["store"]; exists && v != false {
		t.Errorf("store 应原值保留，实际 %#v", v)
	}
	for _, k := range []string{"presence_penalty", "user"} {
		if _, exists := got[k]; exists {
			t.Errorf("非桌面字段 %s 仍应被剥离", k)
		}
	}
}

// TestHasSystemMessageIgnoresMalformed 覆盖 messages 里的脏数据。
func TestHasSystemMessageIgnoresMalformed(t *testing.T) {
	cases := []struct {
		name string
		msgs []any
		want bool
	}{
		{"空", nil, false},
		{"只有 user", []any{map[string]any{"role": "user"}}, false},
		{"system", []any{map[string]any{"role": "system"}}, true},
		{"developer 也算", []any{map[string]any{"role": "developer"}}, true},
		{"非对象条目被跳过", []any{"oops", 42, nil, map[string]any{"role": "system"}}, true},
		{"role 非字符串", []any{map[string]any{"role": 7}}, false},
		{"大小写敏感", []any{map[string]any{"role": "System"}}, false},
	}
	for _, c := range cases {
		if got := hasSystemMessage(c.msgs); got != c.want {
			t.Errorf("%s: hasSystemMessage()=%v，期望 %v", c.name, got, c.want)
		}
	}
}

// TestChatBodyFallbackDirtyMessages 覆盖 messages 里混入非对象条目时仍能兜底。
func TestChatBodyFallbackDirtyMessages(t *testing.T) {
	body := []byte(`{"model":"m","messages":["oops",{"role":"user","content":"hi"}]}`)
	out, ok := ChatBodyFallback(body, "自定义提示词")
	if !ok {
		t.Fatal("messages 非空即应兜底")
	}
	var got map[string]any
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("兜底结果不是合法 JSON: %v", err)
	}
	msgs, _ := got["messages"].([]any)
	if len(msgs) != 3 {
		t.Fatalf("应插入一条 system，实际 %d 条", len(msgs))
	}
	first, _ := msgs[0].(map[string]any)
	if first["role"] != "system" || first["content"] != "自定义提示词" {
		t.Fatalf("自定义提示词未生效: %#v", first)
	}

	if _, ok := ChatBodyFallback([]byte(`not json`), ""); ok {
		t.Fatal("非法 JSON 不应兜底")
	}
	if _, ok := ChatBodyFallback([]byte(`{"messages":{}}`), ""); ok {
		t.Fatal("messages 不是数组时不应兜底")
	}
	if _, ok := ChatBodyFallback([]byte(`{"messages":[]}`), ""); ok {
		t.Fatal("messages 为空时不应兜底")
	}
}
