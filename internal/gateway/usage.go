package gateway

import "deepaicode2api/internal/jsonx"

// Usage 是归一化后的 token / 成本用量。
type Usage struct {
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
	ReasoningTokens  int
	// CachedTokens 是「命中前缀缓存」的 prompt token 数。
	//
	// 上游形态不一致：OpenAI 风格的 prompt_tokens_details.cached_tokens、
	// Responses 风格的 input_tokens_details.cached_tokens、
	// Anthropic 风格的 cache_read_input_tokens。三个都收。
	CachedTokens int
	Cost         float64
}

// ParseUsage 兼容 chat/completions 与 responses 两种用量字段。
func ParseUsage(v any) Usage {
	m, ok := v.(map[string]any)
	if !ok {
		return Usage{}
	}
	u := Usage{
		PromptTokens:     jsonx.IntOf(m["prompt_tokens"]),
		CompletionTokens: jsonx.IntOf(m["completion_tokens"]),
		TotalTokens:      jsonx.IntOf(m["total_tokens"]),
		Cost:             jsonx.FloatOf(m["cost"]),
	}
	if u.PromptTokens == 0 {
		u.PromptTokens = jsonx.IntOf(m["input_tokens"])
	}
	if u.CompletionTokens == 0 {
		u.CompletionTokens = jsonx.IntOf(m["output_tokens"])
	}
	if u.TotalTokens == 0 {
		u.TotalTokens = u.PromptTokens + u.CompletionTokens
	}
	if d, ok := m["completion_tokens_details"].(map[string]any); ok {
		u.ReasoningTokens = jsonx.IntOf(d["reasoning_tokens"])
	}
	if d, ok := m["output_tokens_details"].(map[string]any); ok && u.ReasoningTokens == 0 {
		u.ReasoningTokens = jsonx.IntOf(d["reasoning_tokens"])
	}
	u.CachedTokens = cachedTokensOf(m)
	if c, ok := m["cost_details"].(map[string]any); ok && u.Cost == 0 {
		u.Cost = jsonx.FloatOf(c["upstream_inference_cost"])
	}
	return u
}

// cachedTokensOf 从三种上游形态里取缓存命中的 prompt token 数。
func cachedTokensOf(m map[string]any) int {
	for _, key := range []string{"prompt_tokens_details", "input_tokens_details"} {
		if d, ok := m[key].(map[string]any); ok {
			if n := jsonx.IntOf(d["cached_tokens"]); n > 0 {
				return n
			}
		}
	}
	if n := jsonx.IntOf(m["cache_read_input_tokens"]); n > 0 {
		return n
	}
	return 0
}
