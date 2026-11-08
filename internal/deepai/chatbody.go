package deepai

import "encoding/json"

// DefaultSystemPrompt 是补齐 system 消息时使用的兜底提示词。
//
// 上游 0.3.19+ 的请求体校验只认“DeepAI Code 会发的那种形状”，桌面端每个
// 请求都带 system 提示词与 tools；纯 user-only 的请求偶尔会被 400
// invalid_request_error 拒绝。这里只补 system 消息，不改写用户内容；
// 请求里已经有 system / developer 消息时原样保留，绝不替换。
const DefaultSystemPrompt = "You are a helpful AI assistant. Answer the user's request directly."

// desktopChatKeys 是桌面端 chat/completions 实际会发送的字段白名单。
//
// 仅在上游以 “This request is not one DeepAI Code sends.” 拒绝后作为兜底
// 重试使用；正常路径仍然透传客户端的原始字段。
//
// 采样参数必须在白名单里：形状回退是为了让请求被接受，不是为了把它改成
// 另一个请求。丢掉 seed / service_tier / stream_options 会改变采样条件与
// 用量上报，指纹类检测甚至会因此换一个候选模型。
var desktopChatKeys = map[string]bool{
	"model":                 true,
	"messages":              true,
	"tools":                 true,
	"tool_choice":           true,
	"parallel_tool_calls":   true,
	"max_tokens":            true,
	"max_completion_tokens": true,
	"temperature":           true,
	"top_p":                 true,
	"stop":                  true,
	"stream":                true,
	"stream_options":        true,
	"seed":                  true,
	"service_tier":          true,
	"reasoning_effort":      true,
	"prompt_cache_key":      true,
}

// ChatBodyFallback 把 chat 请求体收敛成桌面端形状，供 400 拒绝后的重试使用。
//
// 规则：
//   - 只保留白名单字段；
//   - 确保第一条消息是 system（缺失时插入默认提示词）。
//
// 返回 ok=false 表示请求体无法解析 / 没有 messages，不值得重试。
func ChatBodyFallback(body []byte, systemPrompt string) ([]byte, bool) {
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, false
	}
	msgs, ok := payload["messages"].([]any)
	if !ok || len(msgs) == 0 {
		return nil, false
	}
	if systemPrompt == "" {
		systemPrompt = DefaultSystemPrompt
	}

	strict := make(map[string]any, len(desktopChatKeys))
	for k, v := range payload {
		if desktopChatKeys[k] {
			strict[k] = v
		}
	}

	if !hasSystemMessage(msgs) {
		withSystem := make([]any, 0, len(msgs)+1)
		withSystem = append(withSystem, map[string]any{"role": "system", "content": systemPrompt})
		withSystem = append(withSystem, msgs...)
		strict["messages"] = withSystem
	}

	out, err := json.Marshal(strict)
	if err != nil {
		return nil, false
	}
	return out, true
}

func hasSystemMessage(msgs []any) bool {
	for _, m := range msgs {
		msg, ok := m.(map[string]any)
		if !ok {
			continue
		}
		role, _ := msg["role"].(string)
		if role == "system" || role == "developer" {
			return true
		}
	}
	return false
}
