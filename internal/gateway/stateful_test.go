package gateway

import (
	"errors"
	"testing"

	"deepaicode2api/internal/jsonx"
)

// TestStatefulParam 覆盖「需要上游状态」的三种形态与它们的空值变体。
//
// 判据必须只认真正携带状态的写法：`previous_response_id: ""` 与不传等价，
// 不能因为客户端习惯性填空串就 400。
func TestStatefulParam(t *testing.T) {
	cases := []struct {
		name    string
		payload map[string]any
		want    string
	}{
		{"无状态字段", map[string]any{"input": "hi"}, ""},
		{"空的 previous_response_id", map[string]any{"previous_response_id": ""}, ""},
		{"null 的 previous_response_id", map[string]any{"previous_response_id": nil}, ""},
		{"previous_response_id", map[string]any{"previous_response_id": "resp_1"}, "previous_response_id"},
		{"空 conversation 字符串", map[string]any{"conversation": ""}, ""},
		{"空 conversation 对象", map[string]any{"conversation": map[string]any{}}, ""},
		{"conversation 字符串", map[string]any{"conversation": "conv_1"}, "conversation"},
		{"conversation 对象", map[string]any{"conversation": map[string]any{"id": "conv_1"}}, "conversation"},
		{"item_reference", map[string]any{"input": []any{
			map[string]any{"type": "item_reference", "id": "msg_1"},
		}}, "item_reference"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := StatefulParam(c.payload); got != c.want {
				t.Fatalf("StatefulParam = %q，期望 %q", got, c.want)
			}
		})
	}
}

// TestResponsesToChatRejectsStateful 验证合成路径不会静默忽略续接字段。
//
// 静默忽略的后果是「一问一答」：客户端以为续接成功，模型其实只看得到本轮
// input（issue #5 的 amnesia）。
func TestResponsesToChatRejectsStateful(t *testing.T) {
	payload := map[string]any{
		"model":                "glm-5.3-flash",
		"previous_response_id": "resp_abc",
		"input":                "What is CODE?",
	}
	_, err := ResponsesToChatPayload(payload, "glm-5.3-flash")
	var perr *StatefulParamError
	if !errors.As(err, &perr) || perr.Param != "previous_response_id" {
		t.Fatalf("err = %v，期望 StatefulParamError(previous_response_id)", err)
	}

	// 不带状态字段时正常翻译。
	out, err := ResponsesToChatPayload(map[string]any{
		"model": "glm-5.3-flash", "input": "hi",
	}, "glm-5.3-flash")
	if err != nil {
		t.Fatalf("无状态字段不应报错: %v", err)
	}
	if msgs, _ := out["messages"].([]any); len(msgs) != 1 {
		t.Fatalf("messages = %d 条，期望 1", len(msgs))
	}
}

// TestResponsesInputMessagesPreservesHistory 锁住多轮 / 工具历史的逐项保留。
//
// 这是「历史被翻译层弄丢」的回归防线：message / function_call /
// function_call_output 必须按原顺序落到 chat messages；reasoning 与输出侧
// 条目（web_search_call 等）按设计跳过。
func TestResponsesInputMessagesPreservesHistory(t *testing.T) {
	payload := map[string]any{
		"instructions": "你是助手",
		"input": []any{
			map[string]any{"type": "message", "role": "user", "content": "第一轮"},
			map[string]any{"type": "reasoning", "summary": []any{map[string]any{"type": "summary_text", "text": "想过"}}},
			map[string]any{"type": "function_call", "call_id": "call_1", "name": "lookup", "arguments": `{"q":"x"}`},
			map[string]any{"type": "function_call_output", "call_id": "call_1", "output": "结果"},
			map[string]any{"type": "web_search_call", "id": "ws_1", "status": "completed"},
			map[string]any{"type": "message", "role": "assistant", "content": "回答"},
			map[string]any{"type": "message", "role": "user", "content": "第二轮"},
		},
	}
	msgs := responsesInputMessages(payload)
	if len(msgs) != 6 {
		t.Fatalf("messages = %d 条，期望 6", len(msgs))
	}
	roles := make([]string, 0, len(msgs))
	for _, m := range msgs {
		mm, _ := m.(map[string]any)
		roles = append(roles, jsonx.StrOf(mm["role"]))
	}
	want := []string{"system", "user", "assistant", "tool", "assistant", "user"}
	for i := range want {
		if roles[i] != want[i] {
			t.Fatalf("第 %d 条角色 = %q，期望 %q（全部：%v）", i, roles[i], want[i], roles)
		}
	}
	// tool 条目要带 tool_call_id，否则上游会把它当普通消息。
	tool, _ := msgs[3].(map[string]any)
	if jsonx.StrOf(tool["tool_call_id"]) != "call_1" {
		t.Fatalf("tool_call_id = %v，期望 call_1", tool["tool_call_id"])
	}
}
