package gateway

import "encoding/json"

// Usage 是归一化后的 token / 成本用量。
type Usage struct {
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
	ReasoningTokens  int
	Cost             float64
}

// ParseUsage 兼容 chat/completions 与 responses 两种用量字段。
func ParseUsage(v any) Usage {
	m, ok := v.(map[string]any)
	if !ok {
		return Usage{}
	}
	u := Usage{
		PromptTokens:     IntOf(m["prompt_tokens"]),
		CompletionTokens: IntOf(m["completion_tokens"]),
		TotalTokens:      IntOf(m["total_tokens"]),
		Cost:             FloatOf(m["cost"]),
	}
	if u.PromptTokens == 0 {
		u.PromptTokens = IntOf(m["input_tokens"])
	}
	if u.CompletionTokens == 0 {
		u.CompletionTokens = IntOf(m["output_tokens"])
	}
	if u.TotalTokens == 0 {
		u.TotalTokens = u.PromptTokens + u.CompletionTokens
	}
	if d, ok := m["completion_tokens_details"].(map[string]any); ok {
		u.ReasoningTokens = IntOf(d["reasoning_tokens"])
	}
	if d, ok := m["output_tokens_details"].(map[string]any); ok && u.ReasoningTokens == 0 {
		u.ReasoningTokens = IntOf(d["reasoning_tokens"])
	}
	if c, ok := m["cost_details"].(map[string]any); ok && u.Cost == 0 {
		u.Cost = FloatOf(c["upstream_inference_cost"])
	}
	return u
}

func IntOf(v any) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case json.Number:
		i, _ := n.Int64()
		return int(i)
	}
	return 0
}

func FloatOf(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case int:
		return float64(n)
	case json.Number:
		f, _ := n.Float64()
		return f
	}
	return 0
}

func LenOfString(v any) int {
	s, _ := v.(string)
	return len([]rune(s))
}
