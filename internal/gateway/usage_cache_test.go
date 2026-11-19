package gateway

import "testing"

// TestAnthropicUsageCarriesCacheTokens 验证缓存命中量按 Anthropic 语义拆分。
//
// Anthropic 的 input_tokens 只算「未命中缓存」的输入，命中量单列在
// cache_read_input_tokens；上游给的是 prompt_tokens 总数，所以这里必须做减法，
// 否则客户端把两个字段相加会得到双倍上下文。
func TestAnthropicUsageCarriesCacheTokens(t *testing.T) {
	got := AnthropicUsage(Usage{PromptTokens: 10, CompletionTokens: 3, TotalTokens: 13, CachedTokens: 6})
	if got["input_tokens"] != 4 {
		t.Fatalf("input_tokens = %v，期望 4（10 − 6）", got["input_tokens"])
	}
	if got["cache_read_input_tokens"] != 6 {
		t.Fatalf("cache_read_input_tokens = %v，期望 6", got["cache_read_input_tokens"])
	}
	if got["cache_creation_input_tokens"] != 0 {
		t.Fatalf("cache_creation_input_tokens = %v，期望 0", got["cache_creation_input_tokens"])
	}
	if got["output_tokens"] != 3 {
		t.Fatalf("output_tokens = %v，期望 3", got["output_tokens"])
	}

	// 没有缓存命中时 input_tokens 保持原值（不引入新的减法语义）。
	plain := AnthropicUsage(Usage{PromptTokens: 11, CompletionTokens: 2})
	if plain["input_tokens"] != 11 {
		t.Fatalf("无缓存时 input_tokens = %v，期望 11", plain["input_tokens"])
	}
	if _, ok := plain["cache_read_input_tokens"]; ok {
		t.Fatalf("无缓存时不应出现 cache_read_input_tokens: %#v", plain)
	}
}

// TestChatToAnthropicMessageCachedUsage 覆盖非流式路径的端到端映射。
func TestChatToAnthropicMessageCachedUsage(t *testing.T) {
	out := map[string]any{
		"choices": []any{map[string]any{
			"finish_reason": "stop",
			"message":       map[string]any{"role": "assistant", "content": "ok"},
		}},
		"usage": map[string]any{
			"prompt_tokens": 20, "completion_tokens": 4, "total_tokens": 24,
			"prompt_tokens_details": map[string]any{"cached_tokens": 16},
		},
	}
	msg := ChatToAnthropicMessage(out, "glm-5.3-flash")
	usage, _ := msg["usage"].(map[string]any)
	if usage["input_tokens"] != 4 || usage["cache_read_input_tokens"] != 16 {
		t.Fatalf("缓存用量映射错误: %#v", usage)
	}
}

// TestChatToResponsesObjectCachedTokens 验证 Responses 对象的 input_tokens_details。
func TestChatToResponsesObjectCachedTokens(t *testing.T) {
	out := map[string]any{
		"choices": []any{map[string]any{
			"finish_reason": "stop",
			"message":       map[string]any{"role": "assistant", "content": "ok"},
		}},
		"usage": map[string]any{
			"prompt_tokens": 20, "completion_tokens": 4, "total_tokens": 24,
			"prompt_tokens_details": map[string]any{"cached_tokens": 16},
		},
	}
	resp := ChatToResponsesObject(out, "glm-5.3-flash")
	usage, _ := resp["usage"].(map[string]any)
	details, _ := usage["input_tokens_details"].(map[string]any)
	if details["cached_tokens"] != 16 {
		t.Fatalf("cached_tokens = %v，期望 16（上游已回传）", details["cached_tokens"])
	}
}
