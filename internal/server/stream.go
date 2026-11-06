package server

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"strings"
)

// --- 上游 SSE 读帧 ---

// sseFrame 是一条上游 SSE 帧里与业务相关的内容。
//
// 网关同时存在两类消费场景：
//   - 透传（chat / responses passthrough）：需要事件名、注释帧、原始 data，
//     以便原样转发上游扩展事件与心跳；
//   - 协议转换（anthropic / responses-as-chat）：只关心解析后的 JSON。
//
// 用一个结构体同时覆盖两者，避免每个 handler 各写一遍读帧循环。
type sseFrame struct {
	// Comment 非空表示注释 / 心跳帧（以 ":" 开头），透传时用于维持客户端连接。
	Comment string
	// Event 是 "event:" 字段的值（Anthropic / Responses 事件流需要）。
	Event string
	// Data 是 "data:" 之后的原始内容（已去首尾空白）。
	Data string
	// Object 是 Data 的 JSON 解析结果；解析失败或非对象时为 nil。
	Object map[string]any
}

// sseDone 是 SSE 流的显式结束标记。
const sseDone = "[DONE]"

// forEachSSEFrame 逐帧解析上游 SSE，把每一帧交给 fn；fn 返回 false 时停止。
//
// 统一了四个流式 handler 的读帧逻辑：data 前缀、[DONE] 终止、JSON 解析结果，
// 以及 event: / 注释帧的透传信息。fn 返回 false 或读到 EOF 都算正常结束。
func forEachSSEFrame(body io.Reader, fn func(f sseFrame) bool) error {
	return readSSELines(body, func(line string) bool {
		trimmed := strings.TrimSpace(line)
		switch {
		case trimmed == "":
			return true
		case strings.HasPrefix(trimmed, ":"):
			return fn(sseFrame{Comment: trimmed})
		}
		if rest, ok := strings.CutPrefix(trimmed, "event:"); ok {
			return fn(sseFrame{Event: strings.TrimSpace(rest)})
		}
		if !strings.HasPrefix(trimmed, "data:") {
			return true
		}
		data := strings.TrimSpace(strings.TrimPrefix(trimmed, "data:"))
		if data == "" {
			return true
		}
		if data == sseDone {
			fn(sseFrame{Data: sseDone})
			return false
		}
		var obj map[string]any
		_ = json.Unmarshal([]byte(data), &obj) // 解析失败时 Object 保持 nil
		return fn(sseFrame{Data: data, Object: obj})
	})
}

// forEachChatChunk 是 forEachSSEFrame 在 chat/completions 语义下的薄封装：
// 只交付已解析的 JSON chunk，自动跳过心跳 / event: / [DONE] / 非法帧。
func forEachChatChunk(body io.Reader, fn func(chunk map[string]any) bool) error {
	return forEachSSEFrame(body, func(f sseFrame) bool {
		if f.Object == nil {
			return true
		}
		return fn(f.Object)
	})
}

// readSSELines 逐行读取上游 SSE；回调返回 false 时停止。
//
// 刻意不复用 deepai.ReadSSE：那个实现按 SSE 规范把多行 data 合并后再分发，
// 会丢失注释帧与 event: 行的原始形态，而网关的透传路径需要它们。
func readSSELines(body io.Reader, fn func(line string) bool) error {
	br := bufio.NewReaderSize(body, 64*1024)
	for {
		line, err := br.ReadString('\n')
		if line != "" {
			if !fn(line) {
				return nil
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
	}
}

// --- chat completion 增量提取 ---

// chatDelta 是一次 chat completion chunk 中与协议适配相关的增量内容。
//
// Anthropic 与 Responses 两条适配链路此前各自解析一遍
// choices[0] → delta → reasoning/content/tool_calls，逻辑逐字相同；
// 统一到这里后，适配器只负责把增量渲染成自己的事件格式。
type chatDelta struct {
	Reasoning string
	Content   string
	ToolCalls []chatToolDelta
}

// chatToolDelta 是一次工具调用的增量片段。
type chatToolDelta struct {
	Index int
	ID    string
	Name  string
	Args  string
}

// chatChunkDelta 取出 chunk 首个 choice 的增量内容与 finish_reason。
//
// finish 为空表示本帧没有结束原因；delta 字段可能全为空（例如仅带 usage 的尾帧）。
func chatChunkDelta(chunk map[string]any) (chatDelta, string) {
	var d chatDelta
	choices, _ := chunk["choices"].([]any)
	if len(choices) == 0 {
		return d, ""
	}
	cm, _ := choices[0].(map[string]any)
	if cm == nil {
		return d, ""
	}

	finish, _ := cm["finish_reason"].(string)
	delta, _ := cm["delta"].(map[string]any)
	if delta == nil {
		return d, finish
	}

	d.Reasoning, _ = delta["reasoning"].(string)
	d.Content, _ = delta["content"].(string)
	tcs, ok := delta["tool_calls"].([]any)
	if !ok {
		return d, finish
	}
	for _, tcv := range tcs {
		tc, ok := tcv.(map[string]any)
		if !ok {
			continue
		}
		td := chatToolDelta{Index: intOf(tc["index"])}
		td.ID, _ = tc["id"].(string)
		if fn, ok := tc["function"].(map[string]any); ok {
			td.Name, _ = fn["name"].(string)
			td.Args, _ = fn["arguments"].(string)
		}
		d.ToolCalls = append(d.ToolCalls, td)
	}
	return d, finish
}
