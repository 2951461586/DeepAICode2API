package gateway

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

// 本文件直接覆盖转换层本体。
//
// 这些函数此前只在 internal/server 的端到端用例里被间接执行（单包覆盖率 13%），
// 某个分支改坏时只有「恰好走到那条分支的集成测试」才会报警。这里按函数补齐：
// 约定是「非法/缺失输入返回零值或明确错误，绝不 panic」——上游字段类型不稳定，
// 这一层不能因为一个形状异常就把整个请求打成 500。

// raw 把任意值编成 json.RawMessage，省掉手写转义。
func raw(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("序列化: %v", err)
	}
	return b
}

func f64(v float64) *float64 { return &v }

// --- Anthropic → Chat ---

func TestAnthropicToChatPayload(t *testing.T) {
	req := &AnthropicRequest{
		Model:         "claude-x",
		MaxTokens:     256,
		Temperature:   f64(0.3),
		StopSequences: []string{"END"},
		System:        raw(t, []any{map[string]any{"type": "text", "text": "你是助手"}}),
		Messages: []AnthropicMessage{
			{Role: "user", Content: raw(t, []any{
				map[string]any{"type": "text", "text": "看图"},
				map[string]any{"type": "image", "source": map[string]any{
					"type": "base64", "media_type": "image/jpeg", "data": "AAAA",
				}},
			})},
			{Role: "assistant", Content: raw(t, []any{
				map[string]any{"type": "text", "text": "查一下"},
				map[string]any{"type": "tool_use", "id": "tu_1", "name": "lookup",
					"input": map[string]any{"q": "x"}},
			})},
			{Role: "user", Content: raw(t, []any{
				map[string]any{"type": "tool_result", "tool_use_id": "tu_1", "content": "结果"},
			})},
		},
		Thinking:   raw(t, map[string]any{"type": "enabled", "budget_tokens": 8000}),
		ToolChoice: raw(t, map[string]any{"type": "auto"}),
		Tools: []AnthropicTool{{
			Name: "lookup", Description: "查询",
			InputSchema: raw(t, map[string]any{"type": "object"}),
		}},
	}

	out, err := AnthropicToChatPayload(req, "glm-5.3-flash")
	if err != nil {
		t.Fatalf("转换失败: %v", err)
	}
	if out["model"] != "glm-5.3-flash" || out["max_tokens"] != 256 {
		t.Fatalf("基础字段未映射: %#v", out)
	}
	if out["temperature"] != 0.3 {
		t.Fatalf("temperature = %v", out["temperature"])
	}
	if got := out["reasoning_effort"]; got != "medium" {
		t.Fatalf("reasoning_effort = %v，期望 medium", got)
	}
	if got := out["tool_choice"]; !equalJSON(got, "auto") {
		t.Fatalf("tool_choice = %#v", got)
	}

	msgs, _ := out["messages"].([]any)
	if len(msgs) != 4 {
		t.Fatalf("消息数 = %d，期望 4（system + user + assistant + tool）: %#v", len(msgs), msgs)
	}
	if m0 := msgs[0].(map[string]any); m0["role"] != "system" || m0["content"] != "你是助手" {
		t.Fatalf("system 消息不对: %#v", m0)
	}
	// user 的文本 + 图片合并成同一条多模态消息。
	u := msgs[1].(map[string]any)
	parts, _ := u["content"].([]any)
	if len(parts) != 2 {
		t.Fatalf("user 多模态片段数 = %d: %#v", len(parts), u)
	}
	img := parts[1].(map[string]any)
	if img["type"] != "image_url" {
		t.Fatalf("图片片段类型 = %v", img["type"])
	}
	if url := img["image_url"].(map[string]any)["url"]; url != "data:image/jpeg;base64,AAAA" {
		t.Fatalf("图片 data URL = %v", url)
	}
	// assistant 的 tool_use 变成 tool_calls。
	a := msgs[2].(map[string]any)
	calls, _ := a["tool_calls"].([]any)
	if len(calls) != 1 {
		t.Fatalf("assistant tool_calls = %#v", a)
	}
	fn := calls[0].(map[string]any)["function"].(map[string]any)
	if fn["name"] != "lookup" || fn["arguments"] != `{"q":"x"}` {
		t.Fatalf("tool_call 参数 = %#v", fn)
	}
	// tool_result 变成 role=tool。
	tr := msgs[3].(map[string]any)
	if tr["role"] != "tool" || tr["tool_call_id"] != "tu_1" || tr["content"] != "结果" {
		t.Fatalf("tool 消息 = %#v", tr)
	}
}

func TestAnthropicToChatPayloadRejectsEmpty(t *testing.T) {
	_, err := AnthropicToChatPayload(&AnthropicRequest{Model: "m"}, "m")
	if err == nil {
		t.Fatal("没有任何消息时应报错，而不是把空 messages 发给上游")
	}
}

func TestAnthropicEffort(t *testing.T) {
	cases := []struct {
		name string
		req  *AnthropicRequest
		want string
	}{
		{"nil 请求", nil, ""},
		{"无 thinking", &AnthropicRequest{}, ""},
		{"output_config 优先", &AnthropicRequest{
			OutputConfig: &struct {
				Effort string `json:"effort"`
			}{Effort: "HIGH"},
			Thinking: raw(t, map[string]any{"type": "disabled"}),
		}, "high"},
		{"disabled → none", &AnthropicRequest{
			Thinking: raw(t, map[string]any{"type": "disabled"}),
		}, "none"},
		{"enabled 无预算 → 空", &AnthropicRequest{
			Thinking: raw(t, map[string]any{"type": "enabled"}),
		}, ""},
		{"预算 < 4096 → low", &AnthropicRequest{
			Thinking: raw(t, map[string]any{"type": "enabled", "budget_tokens": 1024}),
		}, "low"},
		{"预算 4096~16383 → medium", &AnthropicRequest{
			Thinking: raw(t, map[string]any{"type": "enabled", "budget_tokens": 8000}),
		}, "medium"},
		{"预算 ≥ 16384 → high", &AnthropicRequest{
			Thinking: raw(t, map[string]any{"type": "enabled", "budget_tokens": 32000}),
		}, "high"},
		{"thinking 不是对象", &AnthropicRequest{Thinking: raw(t, "enabled")}, ""},
		{"未知 type", &AnthropicRequest{
			Thinking: raw(t, map[string]any{"type": "weird"}),
		}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := AnthropicEffort(c.req); got != c.want {
				t.Fatalf("AnthropicEffort = %q，期望 %q", got, c.want)
			}
		})
	}
}

func TestAppendAnthropicMessage(t *testing.T) {
	t.Run("字符串内容", func(t *testing.T) {
		var msgs []any
		if err := AppendAnthropicMessage(&msgs, AnthropicMessage{Role: "user", Content: raw(t, "hi")}); err != nil {
			t.Fatalf("转换失败: %v", err)
		}
		if len(msgs) != 1 || msgs[0].(map[string]any)["content"] != "hi" {
			t.Fatalf("结果 = %#v", msgs)
		}
	})

	t.Run("空 role 默认 user", func(t *testing.T) {
		var msgs []any
		_ = AppendAnthropicMessage(&msgs, AnthropicMessage{Content: raw(t, "hi")})
		if msgs[0].(map[string]any)["role"] != "user" {
			t.Fatalf("role = %v", msgs[0].(map[string]any)["role"])
		}
	})

	t.Run("assistant 只有 tool_use", func(t *testing.T) {
		var msgs []any
		err := AppendAnthropicMessage(&msgs, AnthropicMessage{Role: "assistant", Content: raw(t, []any{
			map[string]any{"type": "tool_use", "id": "t1", "name": "f", "input": map[string]any{}},
		})})
		if err != nil {
			t.Fatalf("转换失败: %v", err)
		}
		if len(msgs) != 1 {
			t.Fatalf("应产出 1 条消息: %#v", msgs)
		}
	})

	t.Run("assistant 全空不产出消息", func(t *testing.T) {
		var msgs []any
		_ = AppendAnthropicMessage(&msgs, AnthropicMessage{Role: "assistant", Content: raw(t, []any{
			map[string]any{"type": "text", "text": ""},
		})})
		if len(msgs) != 0 {
			t.Fatalf("空 assistant 不应产出消息: %#v", msgs)
		}
	})

	t.Run("document 无图片数据时降级为文本", func(t *testing.T) {
		var msgs []any
		_ = AppendAnthropicMessage(&msgs, AnthropicMessage{Role: "user", Content: raw(t, []any{
			map[string]any{"type": "document", "title": "合同.pdf", "source": map[string]any{
				"type": "base64", "media_type": "application/pdf", "data": "AAAA",
			}},
		})})
		parts := msgs[0].(map[string]any)["content"].([]any)
		if got := parts[0].(map[string]any)["text"]; got != "[document] 合同.pdf" {
			t.Fatalf("降级文本 = %v", got)
		}
	})

	t.Run("content 非字符串也非数组", func(t *testing.T) {
		var msgs []any
		if err := AppendAnthropicMessage(&msgs, AnthropicMessage{Role: "user", Content: raw(t, 42)}); err == nil {
			t.Fatal("应返回错误")
		}
	})
}

func TestAnthropicSystemText(t *testing.T) {
	cases := []struct {
		name string
		in   json.RawMessage
		want string
	}{
		{"空", nil, ""},
		{"字符串", raw(t, "  sys  "), "sys"},
		{"文本块数组", raw(t, []any{
			map[string]any{"type": "text", "text": "A"},
			map[string]any{"type": "image", "source": map[string]any{}},
			map[string]any{"type": "text", "text": "B"},
		}), "A\nB"},
		{"非法 JSON", json.RawMessage(`{bad`), ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := AnthropicSystemText(c.in); got != c.want {
				t.Fatalf("AnthropicSystemText = %q，期望 %q", got, c.want)
			}
		})
	}
}

func TestAnthropicToolResultText(t *testing.T) {
	cases := []struct {
		name string
		in   map[string]any
		want string
	}{
		{"字符串", map[string]any{"content": "ok"}, "ok"},
		{"文本块", map[string]any{"content": []any{
			map[string]any{"type": "text", "text": "A"},
			map[string]any{"type": "text", "text": "B"},
		}}, "A\nB"},
		{"图片块降级", map[string]any{"content": []any{
			map[string]any{"type": "image"},
		}}, "[image omitted]"},
		{"非预期结构", map[string]any{"content": 42}, ""},
		{"缺失", map[string]any{}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := AnthropicToolResultText(c.in); got != c.want {
				t.Fatalf("= %q，期望 %q", got, c.want)
			}
		})
	}
}

func TestAnthropicToolChoice(t *testing.T) {
	cases := []struct {
		name string
		in   json.RawMessage
		want any
	}{
		{"空", nil, nil},
		{"auto", raw(t, map[string]any{"type": "auto"}), "auto"},
		{"any → required", raw(t, map[string]any{"type": "any"}), "required"},
		{"none", raw(t, map[string]any{"type": "none"}), "none"},
		{"tool 带名字", raw(t, map[string]any{"type": "tool", "name": "f"}),
			map[string]any{"type": "function", "function": map[string]any{"name": "f"}}},
		{"tool 缺名字", raw(t, map[string]any{"type": "tool"}), nil},
		{"未知 type", raw(t, map[string]any{"type": "weird"}), nil},
		{"非法 JSON", json.RawMessage(`{bad`), nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := AnthropicToolChoice(c.in); !equalJSON(got, c.want) {
				t.Fatalf("= %#v，期望 %#v", got, c.want)
			}
		})
	}
}

// --- Chat → Anthropic ---

func TestChatToAnthropicMessage(t *testing.T) {
	out := map[string]any{
		"choices": []any{map[string]any{
			"finish_reason": "tool_calls",
			"message": map[string]any{
				"reasoning": "先想一下",
				"content":   "好的",
				"tool_calls": []any{
					map[string]any{"id": "c1", "function": map[string]any{
						"name": "lookup", "arguments": `{"q":"x"}`,
					}},
					map[string]any{"id": "c2", "function": map[string]any{
						"name": "broken", "arguments": `{not json`,
					}},
				},
			},
		}},
		"usage": map[string]any{"prompt_tokens": 3, "completion_tokens": 4},
	}

	msg := ChatToAnthropicMessage(out, "glm-5.3-flash")
	if msg["type"] != "message" || msg["role"] != "assistant" || msg["model"] != "glm-5.3-flash" {
		t.Fatalf("信封字段不对: %#v", msg)
	}
	if msg["stop_reason"] != "tool_use" {
		t.Fatalf("stop_reason = %v", msg["stop_reason"])
	}
	content := msg["content"].([]any)
	// 顺序：thinking → text → tool_use ×2
	if len(content) != 4 {
		t.Fatalf("内容块数 = %d: %#v", len(content), content)
	}
	if content[0].(map[string]any)["type"] != "thinking" {
		t.Fatalf("第一块应为 thinking: %#v", content[0])
	}
	if content[1].(map[string]any)["text"] != "好的" {
		t.Fatalf("文本块 = %#v", content[1])
	}
	tu := content[2].(map[string]any)
	if !equalJSON(tu["input"], map[string]any{"q": "x"}) {
		t.Fatalf("tool_use input = %#v", tu["input"])
	}
	// 非法 JSON 参数必须保留原文，不能静默丢掉。
	broken := content[3].(map[string]any)
	if !equalJSON(broken["input"], map[string]any{"_raw": `{not json`}) {
		t.Fatalf("非法参数未保留原文: %#v", broken["input"])
	}
	usage := msg["usage"].(map[string]any)
	if usage["input_tokens"] != 3 || usage["output_tokens"] != 4 {
		t.Fatalf("usage = %#v", usage)
	}
}

func TestChatToAnthropicMessageEmpty(t *testing.T) {
	msg := ChatToAnthropicMessage(map[string]any{}, "m")
	content := msg["content"].([]any)
	if len(content) != 1 || content[0].(map[string]any)["type"] != "text" {
		t.Fatalf("空响应也应有一个 text 块（客户端按它渲染）: %#v", content)
	}
	if msg["stop_reason"] != "end_turn" {
		t.Fatalf("stop_reason = %v", msg["stop_reason"])
	}
}

func TestAnthropicStopReason(t *testing.T) {
	cases := map[string]string{
		"tool_calls":     "tool_use",
		"function_call":  "tool_use",
		"length":         "max_tokens",
		"content_filter": "end_turn",
		"stop":           "end_turn",
		"":               "end_turn",
	}
	for in, want := range cases {
		if got := AnthropicStopReason(in); got != want {
			t.Fatalf("AnthropicStopReason(%q) = %q，期望 %q", in, got, want)
		}
	}
}

func TestAnthropicTextLen(t *testing.T) {
	msg := map[string]any{"content": []any{
		map[string]any{"type": "text", "text": "中文"},
		map[string]any{"type": "thinking", "thinking": "abc"},
		map[string]any{"type": "tool_use", "input": map[string]any{}},
	}}
	if got := AnthropicTextLen(msg); got != 5 {
		t.Fatalf("AnthropicTextLen = %d，期望 5（按 rune 计）", got)
	}
}

// --- Responses → Chat ---

func TestResponsesToChatPayload(t *testing.T) {
	payload := map[string]any{
		"instructions":      "你是助手",
		"max_output_tokens": float64(128),
		"temperature":       0.2,
		"reasoning":         map[string]any{"effort": "LOW"},
		"input": []any{
			map[string]any{"type": "message", "role": "user", "content": []any{
				map[string]any{"type": "input_text", "text": "帮我查"},
				map[string]any{"type": "input_image", "image_url": "https://x/y.png"},
			}},
			map[string]any{"type": "function_call", "call_id": "c0", "name": "lookup", "arguments": `{"q":"a"}`},
			map[string]any{"type": "function_call_output", "call_id": "c0", "output": map[string]any{"ok": true}},
			map[string]any{"type": "reasoning", "summary": "不回喂"},
		},
		"tools":       []any{map[string]any{"type": "function", "name": "lookup", "parameters": map[string]any{"type": "object"}}},
		"tool_choice": map[string]any{"type": "function", "name": "lookup"},
	}

	out, err := ResponsesToChatPayload(payload, "glm-5.3-flash")
	if err != nil {
		t.Fatalf("转换失败: %v", err)
	}
	if out["model"] != "glm-5.3-flash" || out["max_tokens"] != 128 {
		t.Fatalf("基础字段 = %#v", out)
	}
	if out["reasoning_effort"] != "low" {
		t.Fatalf("reasoning_effort = %v（应小写化）", out["reasoning_effort"])
	}
	msgs := out["messages"].([]any)
	// instructions → system；input 里的 reasoning 项被丢弃。
	if len(msgs) != 4 {
		t.Fatalf("消息数 = %d: %#v", len(msgs), msgs)
	}
	if msgs[0].(map[string]any)["role"] != "system" {
		t.Fatalf("首条应为 system: %#v", msgs[0])
	}
	parts := msgs[1].(map[string]any)["content"].([]any)
	if len(parts) != 2 || parts[1].(map[string]any)["type"] != "image_url" {
		t.Fatalf("多模态内容 = %#v", parts)
	}
	tool := msgs[3].(map[string]any)
	if tool["role"] != "tool" || tool["tool_call_id"] != "c0" {
		t.Fatalf("function_call_output 未转成 tool 消息: %#v", tool)
	}
	if got := tool["content"]; got != `{"ok":true}` {
		t.Fatalf("tool 输出 = %v（应 JSON 序列化）", got)
	}
	if tc := out["tool_choice"]; !equalJSON(tc, map[string]any{
		"type": "function", "function": map[string]any{"name": "lookup"},
	}) {
		t.Fatalf("tool_choice = %#v", tc)
	}
}

func TestResponsesToChatPayloadStringInput(t *testing.T) {
	out, err := ResponsesToChatPayload(map[string]any{"input": "hi"}, "m")
	if err != nil {
		t.Fatalf("转换失败: %v", err)
	}
	msgs := out["messages"].([]any)
	if len(msgs) != 1 || msgs[0].(map[string]any)["content"] != "hi" {
		t.Fatalf("字符串 input 应变成一条 user 消息: %#v", msgs)
	}
}

func TestResponsesToChatPayloadNoInput(t *testing.T) {
	_, err := ResponsesToChatPayload(map[string]any{}, "m")
	if !errors.Is(err, ErrNoMessages) {
		t.Fatalf("err = %v，期望 ErrNoMessages", err)
	}
}

func TestResponsesInputContent(t *testing.T) {
	cases := []struct {
		name string
		in   any
		want any
	}{
		{"字符串原样", "hi", "hi"},
		{"nil", nil, ""},
		{"数字", 3, ""},
		{"文本块", []any{map[string]any{"type": "input_text", "text": "A"}},
			[]any{map[string]any{"type": "text", "text": "A"}}},
		{"refusal 也当文本", []any{map[string]any{"type": "refusal", "refusal": "不行"}},
			[]any{map[string]any{"type": "text", "text": "不行"}}},
		{"空数组 → 空串", []any{}, ""},
		{"无法识别的块被忽略", []any{map[string]any{"type": "weird"}}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ResponsesInputContent(c.in); !equalJSON(got, c.want) {
				t.Fatalf("= %#v，期望 %#v", got, c.want)
			}
		})
	}
}

// --- Chat → Responses ---

func TestChatToResponsesObject(t *testing.T) {
	chat := map[string]any{
		"choices": []any{map[string]any{"message": map[string]any{
			"reasoning": "想想",
			"content":   "答案",
			"tool_calls": []any{map[string]any{"id": "c1", "function": map[string]any{
				"name": "lookup", "arguments": `{"q":1}`,
			}}},
		}}},
		"usage": map[string]any{
			"prompt_tokens": 10, "completion_tokens": 5, "total_tokens": 15,
			"completion_tokens_details": map[string]any{"reasoning_tokens": 2},
		},
	}
	resp := ChatToResponsesObject(chat, "glm-5.3-flash")
	if resp["object"] != "response" || resp["status"] != "completed" || resp["model"] != "glm-5.3-flash" {
		t.Fatalf("信封字段 = %#v", resp)
	}
	if resp["output_text"] != "答案" {
		t.Fatalf("output_text = %v", resp["output_text"])
	}
	if !strings.HasPrefix(resp["id"].(string), "resp_") {
		t.Fatalf("id = %v", resp["id"])
	}
	output := resp["output"].([]any)
	if len(output) != 3 {
		t.Fatalf("output 项数 = %d: %#v", len(output), output)
	}
	if output[0].(map[string]any)["type"] != "reasoning" {
		t.Fatalf("第一项应为 reasoning: %#v", output[0])
	}
	if output[2].(map[string]any)["type"] != "function_call" {
		t.Fatalf("第三项应为 function_call: %#v", output[2])
	}
	usage := resp["usage"].(map[string]any)
	if usage["input_tokens"] != 10 || usage["output_tokens"] != 5 || usage["total_tokens"] != 15 {
		t.Fatalf("usage = %#v", usage)
	}
	if d := usage["output_tokens_details"].(map[string]any); d["reasoning_tokens"] != 2 {
		t.Fatalf("reasoning_tokens = %#v", d)
	}
}

func TestChatToResponsesObjectEmpty(t *testing.T) {
	resp := ChatToResponsesObject(map[string]any{}, "m")
	output := resp["output"].([]any)
	if len(output) != 0 {
		t.Fatalf("空响应不应造出 output: %#v", output)
	}
	if resp["output_text"] != "" {
		t.Fatalf("output_text = %v", resp["output_text"])
	}
}

// --- 用量归一 ---

func TestParseUsage(t *testing.T) {
	cases := []struct {
		name string
		in   any
		want Usage
	}{
		{"nil", nil, Usage{}},
		{"非对象", "x", Usage{}},
		{"chat 形态", map[string]any{
			"prompt_tokens": 10, "completion_tokens": 4, "total_tokens": 14,
		}, Usage{PromptTokens: 10, CompletionTokens: 4, TotalTokens: 14}},
		{"responses 形态", map[string]any{
			"input_tokens": 8, "output_tokens": 2,
		}, Usage{PromptTokens: 8, CompletionTokens: 2, TotalTokens: 10}},
		{"总量缺失时相加", map[string]any{
			"prompt_tokens": 3, "completion_tokens": 7,
		}, Usage{PromptTokens: 3, CompletionTokens: 7, TotalTokens: 10}},
		{"缓存命中（openai）", map[string]any{
			"prompt_tokens": 10, "completion_tokens": 1,
			"prompt_tokens_details": map[string]any{"cached_tokens": 6},
		}, Usage{PromptTokens: 10, CompletionTokens: 1, TotalTokens: 11, CachedTokens: 6}},
		{"缓存命中（anthropic）", map[string]any{
			"input_tokens": 10, "output_tokens": 1, "cache_read_input_tokens": 9,
		}, Usage{PromptTokens: 10, CompletionTokens: 1, TotalTokens: 11, CachedTokens: 9}},
		{"推理 token", map[string]any{
			"prompt_tokens": 1, "completion_tokens": 5,
			"completion_tokens_details": map[string]any{"reasoning_tokens": 4},
		}, Usage{PromptTokens: 1, CompletionTokens: 5, TotalTokens: 6, ReasoningTokens: 4}},
		{"成本", map[string]any{
			"prompt_tokens": 1, "completion_tokens": 1,
			"cost_details": map[string]any{"upstream_inference_cost": 0.25},
		}, Usage{PromptTokens: 1, CompletionTokens: 1, TotalTokens: 2, Cost: 0.25}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ParseUsage(c.in); got != c.want {
				t.Fatalf("ParseUsage = %+v，期望 %+v", got, c.want)
			}
		})
	}
}

// --- 流式块管理 ---

func TestAnthropicBlockWriter(t *testing.T) {
	type ev struct {
		name string
		body map[string]any
	}
	var events []ev
	b := NewAnthropicBlockWriter(func(name string, v any) {
		events = append(events, ev{name: name, body: v.(map[string]any)})
	})

	b.Thinking("想")
	b.Thinking("更多") // 同一块内不重复 open
	b.Text("答")      // 切换块会先 close 上一个
	b.ToolCall(0, "t1", "lookup", `{"q":1}`)
	b.ToolCall(0, "t1", "lookup", "") // 同一工具块不重复 open
	b.ToolCall(1, "t2", "other", `{}`)
	b.Close()
	b.Close() // 幂等：已关闭不再发事件

	var starts, stops, deltas int
	for _, e := range events {
		switch e.name {
		case "content_block_start":
			starts++
		case "content_block_stop":
			stops++
		case "content_block_delta":
			deltas++
		}
	}
	// thinking / text / tool / tool = 4 块
	if starts != 4 || stops != 4 {
		t.Fatalf("块数 start=%d stop=%d，期望 4/4：%#v", starts, stops, events)
	}
	// thinking 2 次 delta + text 1 + tool json 2 = 5
	if deltas != 5 {
		t.Fatalf("delta 数 = %d，期望 5", deltas)
	}
	// index 必须递增且与 stop 对得上。
	last := -1
	for _, e := range events {
		if e.name != "content_block_start" {
			continue
		}
		if idx := IntOf(e.body["index"]); idx <= last {
			t.Fatalf("块 index 未递增: %d → %d", last, idx)
		} else {
			last = idx
		}
	}
}
