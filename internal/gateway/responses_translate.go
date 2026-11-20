package gateway

import (
	"encoding/json"
	"strings"
	"time"

	"deepaicode2api/internal/jsonx"
)

// --- Responses → Chat ---

// ResponsesReasoningEffort 提取 Responses 请求里的推理强度。
//
// 兼容两种写法：官方形状 {"reasoning":{"effort":"low"}}，以及直接给字符串。
func ResponsesReasoningEffort(payload map[string]any) string {
	switch v := payload["reasoning"].(type) {
	case map[string]any:
		return strings.ToLower(strings.TrimSpace(jsonx.StrOf(v["effort"])))
	case string:
		return strings.ToLower(strings.TrimSpace(v))
	}
	return ""
}

// ResponsesToChatPayload 把 Responses 请求翻译成 Chat Completions。
//
// 依赖上游响应状态的请求（`previous_response_id` / `conversation` /
// `item_reference`）在这里直接报错：合成路径只有 `input` 里的内容，
// 静默忽略这些字段会让客户端以为“续接成功”，实际上下文被截断。
func ResponsesToChatPayload(payload map[string]any, modelID string) (map[string]any, error) {
	if p := StatefulParam(payload); p != "" {
		return nil, &StatefulParamError{Param: p}
	}
	out := map[string]any{
		"model":  modelID,
		"stream": jsonx.BoolOf(payload["stream"]),
	}
	if v, ok := payload["temperature"]; ok {
		out["temperature"] = v
	}
	if v, ok := payload["top_p"]; ok {
		out["top_p"] = v
	}
	if v := jsonx.IntOf(payload["max_output_tokens"]); v > 0 {
		out["max_tokens"] = v
	}
	if v, ok := payload["prompt_cache_key"]; ok {
		out["prompt_cache_key"] = v
	}
	// Responses 的 reasoning.effort 与 chat 的 reasoning_effort 是同一个旋钮。
	// 不映射就是静默把调用方的推理强度丢掉（上游会按默认值跑）。
	if effort := ResponsesReasoningEffort(payload); effort != "" {
		out["reasoning_effort"] = effort
	}

	messages := responsesInputMessages(payload)
	if len(messages) == 0 {
		return nil, ErrNoMessages
	}
	out["messages"] = messages

	if tools, ok := payload["tools"].([]any); ok && len(tools) > 0 {
		converted := make([]any, 0, len(tools))
		for _, tv := range tools {
			t, ok := tv.(map[string]any)
			if !ok || jsonx.StrOf(t["type"]) != "function" {
				continue
			}
			fn := map[string]any{"name": jsonx.StrOf(t["name"])}
			if d := jsonx.StrOf(t["description"]); d != "" {
				fn["description"] = d
			}
			if p, ok := t["parameters"]; ok {
				fn["parameters"] = p
			}
			converted = append(converted, map[string]any{"type": "function", "function": fn})
		}
		if len(converted) > 0 {
			out["tools"] = converted
			if tc, ok := payload["tool_choice"]; ok {
				out["tool_choice"] = ConvertResponsesToolChoice(tc)
			}
		}
	}
	return out, nil
}

// responsesInputMessages 把 Responses 的 instructions + input 展开为 chat messages。
//
// 单独抽出来只为可读性与复杂度：这里是「历史会不会被弄丢」的核心，
// 每条分支都对应一种 item 形态（message / function_call / function_call_output）。
// 未识别的 item 类型（web_search_call、code_interpreter_call 等**输出侧**条目）
// 跳过：chat/completions 无法表达它们，而其中的用户可见结论通常已在
// message 项里；真正会导致上下文缺失的 item_reference 已在入口处报错。
func responsesInputMessages(payload map[string]any) []any {
	messages := make([]any, 0, 4)
	if instructions, ok := payload["instructions"].(string); ok && strings.TrimSpace(instructions) != "" {
		messages = append(messages, map[string]any{"role": "system", "content": instructions})
	}
	switch input := payload["input"].(type) {
	case string:
		messages = append(messages, map[string]any{"role": "user", "content": input})
	case []any:
		for _, itemv := range input {
			item, ok := itemv.(map[string]any)
			if !ok {
				if s, ok := itemv.(string); ok {
					messages = append(messages, map[string]any{"role": "user", "content": s})
				}
				continue
			}
			switch jsonx.StrOf(item["type"]) {
			case "message", "":
				role := jsonx.StrOf(item["role"])
				if role == "" {
					role = "user"
				}
				messages = append(messages, map[string]any{
					"role": role, "content": ResponsesInputContent(item["content"]),
				})
			case "function_call":
				appendResponsesFunctionCall(&messages, item)
			case "function_call_output":
				messages = append(messages, map[string]any{
					"role":         "tool",
					"tool_call_id": jsonx.FirstString(item["call_id"], item["id"]),
					"content":      ResponsesToolOutput(item["output"]),
				})
			case "reasoning":
				// 历史推理内容不再回喂上游（与客户端一致）。
			default:
				// 其余是输出侧条目（web_search_call / code_interpreter_call / …），
				// chat 无法表达；跳过不影响用户可见历史。
			}
		}
	}
	return messages
}

func ResponsesInputContent(v any) any {
	switch c := v.(type) {
	case string:
		return c
	case []any:
		parts := make([]any, 0, len(c))
		for _, pv := range c {
			p, ok := pv.(map[string]any)
			if !ok {
				continue
			}
			switch jsonx.StrOf(p["type"]) {
			case "input_text", "output_text", "text":
				if t := jsonx.StrOf(p["text"]); t != "" {
					parts = append(parts, map[string]any{"type": "text", "text": t})
				}
			case "input_image":
				if u := jsonx.StrOf(p["image_url"]); u != "" {
					parts = append(parts, map[string]any{
						"type": "image_url", "image_url": map[string]any{"url": u},
					})
				}
			case "refusal":
				if t := jsonx.StrOf(p["refusal"]); t != "" {
					parts = append(parts, map[string]any{"type": "text", "text": t})
				}
			}
		}
		if len(parts) == 0 {
			return ""
		}
		return parts
	}
	return ""
}

// appendResponsesFunctionCall 把 function_call 合并到相邻的 assistant 消息。
//
// Responses 的并行工具调用是连续多个 function_call item；若每个都单独成一条
// assistant 消息，就会变成 assistant(call1) → assistant(call2) → tool(call1)，
// 而 OpenAI 要求 role=tool 的消息紧跟在带 tool_calls 的 assistant 之后。
// 合并后一条 assistant 带多个 tool_calls，后面的 function_call_output 才能正确对应。
func appendResponsesFunctionCall(messages *[]any, item map[string]any) {
	call := map[string]any{
		"id": jsonx.FirstString(item["call_id"], item["id"]), "type": "function",
		"function": map[string]any{
			"name":      jsonx.StrOf(item["name"]),
			"arguments": jsonx.StrOf(item["arguments"]),
		},
	}
	if n := len(*messages); n > 0 {
		if last, ok := (*messages)[n-1].(map[string]any); ok && jsonx.StrOf(last["role"]) == "assistant" {
			if tcs, ok := last["tool_calls"].([]any); ok {
				last["tool_calls"] = append(tcs, call)
				return
			}
		}
	}
	*messages = append(*messages, map[string]any{
		"role": "assistant", "content": nil, "tool_calls": []any{call},
	})
}

func ResponsesToolOutput(v any) string {
	switch o := v.(type) {
	case string:
		return o
	case nil:
		return ""
	default:
		raw, err := json.Marshal(o)
		if err != nil {
			return ""
		}
		return string(raw)
	}
}

func ConvertResponsesToolChoice(v any) any {
	switch tc := v.(type) {
	case string:
		return tc
	case map[string]any:
		if jsonx.StrOf(tc["type"]) == "function" && jsonx.StrOf(tc["name"]) != "" {
			return map[string]any{"type": "function", "function": map[string]any{"name": jsonx.StrOf(tc["name"])}}
		}
	}
	return nil
}

// --- Chat → Responses ---

// ChatToResponsesObject 把 Chat Completions 响应合成为 Responses 对象。
func ChatToResponsesObject(chat map[string]any, publicModel string) map[string]any {
	output := make([]any, 0, 3)
	outputText := ""
	reasoningText := ""

	choices, _ := chat["choices"].([]any)
	if len(choices) > 0 {
		if cm, ok := choices[0].(map[string]any); ok {
			if msg, ok := cm["message"].(map[string]any); ok {
				if r := jsonx.StrOf(msg["reasoning"]); r != "" {
					reasoningText = r
					output = append(output, map[string]any{
						"type": "reasoning", "id": newID("rs"),
						"summary": []any{map[string]any{"type": "summary_text", "text": r}},
					})
				}
				if t := jsonx.StrOf(msg["content"]); t != "" {
					outputText = t
					output = append(output, map[string]any{
						"type": "message", "id": newID("msg"),
						"status": "completed", "role": "assistant",
						"content": []any{map[string]any{"type": "output_text", "text": t, "annotations": []any{}}},
					})
				}
				if tcs, ok := msg["tool_calls"].([]any); ok {
					for _, tcv := range tcs {
						tc, ok := tcv.(map[string]any)
						if !ok {
							continue
						}
						callID := jsonx.StrOf(tc["id"])
						name, args := "", ""
						if fn, ok := tc["function"].(map[string]any); ok {
							name = jsonx.StrOf(fn["name"])
							args = jsonx.StrOf(fn["arguments"])
						}
						output = append(output, map[string]any{
							"type": "function_call", "id": newID("fc"),
							"call_id": callID, "name": name, "arguments": args,
							"status": "completed",
						})
					}
				}
			}
		}
	}

	usage := ParseUsage(chat["usage"])
	resp := map[string]any{
		"id":                  newID("resp"),
		"object":              "response",
		"created_at":          time.Now().Unix(),
		"status":              "completed",
		"model":               publicModel,
		"output":              output,
		"output_text":         outputText,
		"parallel_tool_calls": true,
		"tool_choice":         "auto",
		"tools":               []any{},
		"usage": map[string]any{
			"input_tokens":          usage.PromptTokens,
			"output_tokens":         usage.CompletionTokens,
			"total_tokens":          usage.TotalTokens,
			"input_tokens_details":  map[string]any{"cached_tokens": usage.CachedTokens},
			"output_tokens_details": map[string]any{"reasoning_tokens": usage.ReasoningTokens},
		},
	}
	if reasoningText != "" {
		resp["reasoning"] = map[string]any{"summary": reasoningText}
	}
	return resp
}
