package gateway

import (
	"sort"
	"strings"
)

// --- Anthropic 流式内容块管理器 ---

// anthropicToolBlock 是缓冲中的工具调用。
//
// OpenAI 的流式 tool_calls 以 index 区分，多个工具调用的参数分片**可能交错**
// 到达（尤其是并行工具调用）。Anthropic 的 content_block 语义是顺序块：
// 一个块 close 之后就不能再往里追加。因此这里先按 index 把参数攒齐，
// 在 Finish / 下一次文本块之前按 index 顺序一次性发出，避免把 tool#1 的
// 参数分片写进 tool#0 的块里。
type anthropicToolBlock struct {
	toolIdx int
	id      string
	name    string
	args    strings.Builder
}

type AnthropicBlockWriter struct {
	send  func(event string, v any)
	kind  string // "" / thinking / text / tool
	index int
	open  bool
	tools map[int]*anthropicToolBlock
}

func NewAnthropicBlockWriter(send func(string, any)) *AnthropicBlockWriter {
	return &AnthropicBlockWriter{send: send, index: -1, tools: map[int]*anthropicToolBlock{}}
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

// ToolCall 缓冲一个工具调用分片；真正的内容块在 Finish 时按 index 顺序发出。
func (b *AnthropicBlockWriter) ToolCall(toolIdx int, id, name, args string) {
	if b.tools == nil {
		b.tools = map[int]*anthropicToolBlock{}
	}
	t := b.tools[toolIdx]
	if t == nil {
		t = &anthropicToolBlock{toolIdx: toolIdx}
		b.tools[toolIdx] = t
	}
	if id != "" {
		t.id = id
	}
	if name != "" {
		t.name = name
	}
	t.args.WriteString(args)
}

// Finish 结束响应：关闭当前块，把缓冲的工具调用按 index 顺序发出，再关闭最后一个。
func (b *AnthropicBlockWriter) Finish() {
	b.Close()
	b.flushTools()
	b.Close()
}

// flushTools 按 tool index 升序发出工具块。
//
// 每个工具的参数作为**单个** input_json_delta 发出：上游分片已经攒齐，
// 再拆成多个 delta 只会增加客户端解析负担，而且无法保证与 OpenAI 的
// 分片边界一致。
func (b *AnthropicBlockWriter) flushTools() {
	if len(b.tools) == 0 {
		return
	}
	idxs := make([]int, 0, len(b.tools))
	for idx := range b.tools {
		idxs = append(idxs, idx)
	}
	sort.Ints(idxs)
	for _, idx := range idxs {
		t := b.tools[idx]
		b.OpenBlock("tool", map[string]any{
			"type": "tool_use", "id": t.id, "name": t.name, "input": map[string]any{},
		})
		if t.args.Len() > 0 {
			b.send("content_block_delta", map[string]any{
				"type": "content_block_delta", "index": b.index,
				"delta": map[string]any{"type": "input_json_delta", "partial_json": t.args.String()},
			})
		}
	}
	b.tools = map[int]*anthropicToolBlock{}
}
