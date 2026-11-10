package gateway

// --- Anthropic 流式内容块管理器 ---

type AnthropicBlockWriter struct {
	send     func(event string, v any)
	kind     string // "" / thinking / text / tool
	index    int
	open     bool
	toolSeen map[int]bool
}

func NewAnthropicBlockWriter(send func(string, any)) *AnthropicBlockWriter {
	return &AnthropicBlockWriter{send: send, index: -1, toolSeen: map[int]bool{}}
}

func (b *AnthropicBlockWriter) OpenBlock(kind string, content map[string]any) {
	b.Close()
	b.index++
	b.kind = kind
	b.open = true
	b.send("content_block_start", map[string]any{
		"type": "content_block_start", "index": b.index, "content_block": content,
	})
}

func (b *AnthropicBlockWriter) Close() {
	if !b.open {
		return
	}
	b.send("content_block_stop", map[string]any{
		"type": "content_block_stop", "index": b.index,
	})
	b.open = false
	b.kind = ""
}

func (b *AnthropicBlockWriter) Thinking(text string) {
	if b.kind != "thinking" {
		b.OpenBlock("thinking", map[string]any{"type": "thinking", "thinking": "", "signature": ""})
	}
	b.send("content_block_delta", map[string]any{
		"type": "content_block_delta", "index": b.index,
		"delta": map[string]any{"type": "thinking_delta", "thinking": text},
	})
}

func (b *AnthropicBlockWriter) Text(text string) {
	if b.kind != "text" {
		b.OpenBlock("text", map[string]any{"type": "text", "text": ""})
	}
	b.send("content_block_delta", map[string]any{
		"type": "content_block_delta", "index": b.index,
		"delta": map[string]any{"type": "text_delta", "text": text},
	})
}

func (b *AnthropicBlockWriter) ToolCall(toolIdx int, id, name, args string) {
	if b.kind != "tool" || !b.toolSeen[toolIdx] {
		b.OpenBlock("tool", map[string]any{
			"type": "tool_use", "id": id, "name": name, "input": map[string]any{},
		})
		b.toolSeen[toolIdx] = true
	}
	if args != "" {
		b.send("content_block_delta", map[string]any{
			"type": "content_block_delta", "index": b.index,
			"delta": map[string]any{"type": "input_json_delta", "partial_json": args},
		})
	}
}
