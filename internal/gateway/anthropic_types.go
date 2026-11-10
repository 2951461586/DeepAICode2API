package gateway

import (
	"encoding/json"
)

type AnthropicRequest struct {
	Model         string             `json:"model"`
	MaxTokens     int                `json:"max_tokens"`
	System        json.RawMessage    `json:"system"`
	Messages      []AnthropicMessage `json:"messages"`
	Stream        bool               `json:"stream"`
	Temperature   *float64           `json:"temperature"`
	TopP          *float64           `json:"top_p"`
	TopK          *int               `json:"top_k"`
	StopSequences []string           `json:"stop_sequences"`
	Tools         []AnthropicTool    `json:"tools"`
	ToolChoice    json.RawMessage    `json:"tool_choice"`
	Thinking      json.RawMessage    `json:"thinking"`
	// OutputConfig 是 Anthropic 自适应思考的强度旋钮（{"effort":"low"}）。
	OutputConfig *struct {
		Effort string `json:"effort"`
	} `json:"output_config"`
	Metadata json.RawMessage `json:"metadata"`
}

type AnthropicMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

type AnthropicTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema"`
}

func FirstString(vals ...any) string {
	for _, v := range vals {
		if s, ok := v.(string); ok && s != "" {
			return s
		}
	}
	return ""
}
