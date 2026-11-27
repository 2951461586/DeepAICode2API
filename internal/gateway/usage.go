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
	// CacheWriteTokens 是「本次写入前缀缓存」的 prompt token 数。
	//
	// 命中率看起来只有 60–70% 的主要构成项：一段新前缀（新会话的首轮、空闲超过
	// 上游 TTL 之后的重写）必须先把缓存写一遍，这次请求的这块 token 不是「没命中」，
	// 而是「只能靠后续请求才能兑现」。上游在 Anthropic 形状里把它单列为
	// cache_creation_input_tokens；不记这个量，「冷启动」与「该命中却没命中」
	// 在控制台上长得一模一样。
	CacheWriteTokens int
	Cost             float64
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
	u.CacheWriteTokens = cacheWriteTokensOf(m)
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

// cacheWriteTokensOf 取「写入前缀缓存」的 prompt token 数。
//
// 上游目前给的是 Anthropic 形状的 cache_creation_input_tokens（实测 0.3.52 的
// /chat/completions 响应里就带这个字段）；同时也兼容放在 *_tokens_details 里的
// 写法，两个都看，取第一个非零值。
func cacheWriteTokensOf(m map[string]any) int {
	if n := jsonx.IntOf(m["cache_creation_input_tokens"]); n > 0 {
		return n
	}
	for _, key := range []string{"prompt_tokens_details", "input_tokens_details"} {
		if d, ok := m[key].(map[string]any); ok {
			if n := jsonx.IntOf(d["cache_creation_tokens"]); n > 0 {
				return n
			}
		}
	}
	return 0
}
