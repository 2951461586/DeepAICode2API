package server

import (
	"encoding/json"
	"net/http"
	"time"

	"deepaicode2api/internal/deepai"
	"deepaicode2api/internal/gateway"
	"deepaicode2api/internal/httpx"
)

// --- Anthropic 请求模型 ---

// handleAnthropicMessages 是 POST /v1/messages 的入口。
//
// 把 Anthropic Messages 请求转换为 OpenAI Chat Completions，调用上游后再
// 映射回 Anthropic 的 message / SSE 事件模型（含 tool_use、thinking）。
func (s *Server) handleAnthropicMessages(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeAnthropicErr(w, http.StatusMethodNotAllowed, "invalid_request_error", "仅支持 POST")
		return
	}
	start := time.Now()

	req, ok := httpx.DecodeJSONRequest[gateway.AnthropicRequest](w, r, func(code int, msg string) {
		writeAnthropicErr(w, code, "invalid_request_error", msg)
	})
	if !ok {
		return
	}
	if len(req.Messages) == 0 {
		writeAnthropicErr(w, http.StatusBadRequest, "invalid_request_error", "messages 不能为空")
		return
	}

	model, modelOK := s.resolveModelOK(req.Model)
	if !modelOK {
		writeAnthropicErr(w, http.StatusNotFound, "invalid_request_error", s.unknownModelMessage(req.Model))
		return
	}
	payload, err := gateway.AnthropicToChatPayload(&req, model.ID)
	if err != nil {
		writeAnthropicErr(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	// 会话级 affinity：客户端给了就用，没给就用稳定前缀派生（见 request.go）。
	// 既保上游前缀缓存分区，也让选号把同一会话钉在同一个账号上。
	affinity := sessionAffinity(r, payload)
	key := extractKey(r)
	promptLen := estimatePromptLen(payload)

	rl := s.newReqLog(key, nil, model.ID, "messages", req.Stream, start, promptLen).withHistory(payload)
	if req.Stream {
		s.streamAnthropic(w, r, payload, affinity, rl)
		return
	}
	s.blockingAnthropic(w, r, payload, affinity, rl)
}

// blockingAnthropic 收集完整回复后按 Anthropic message 结构返回。
func (s *Server) blockingAnthropic(w http.ResponseWriter, r *http.Request, payload map[string]any, affinity string, rl reqLog) {
	ctx, cancel := httpx.ContextWithTimeout(r, s.requestTimeout())
	defer cancel()

	out, acct, outBytes, err := s.callJSON(ctx, deepai.PathChat, payload, affinity, s.requestGroup(r), rl.apiKey(), s.requireQuotaFor(rl.model))
	if outBytes > 0 {
		rl = rl.withOutboundBytes(outBytes)
	}
	rl.acct = acct
	if err != nil {
		rl.done(0, gateway.Usage{}, err)
		writeAnthropicUpstreamErr(w, err)
		return
	}
	usage := gateway.ParseUsage(out["usage"])
	msg := gateway.ChatToAnthropicMessage(out, rl.model)
	outLen := gateway.AnthropicTextLen(msg)

	rl.done(outLen, usage, nil)
	httpx.WriteJSON(w, http.StatusOK, msg)
}

// streamAnthropic 以 Anthropic SSE 事件模型流式返回。
func (s *Server) streamAnthropic(w http.ResponseWriter, r *http.Request, payload map[string]any, affinity string, rl reqLog) {
	ctx, cancel := httpx.ContextWithTimeout(r, s.requestTimeout())
	defer cancel()

	body, err := json.Marshal(payload)
	if err != nil {
		rl.doneStatus(http.StatusBadRequest, 0, gateway.Usage{}, err)
		writeAnthropicErr(w, http.StatusBadRequest, "invalid_request_error", "请求序列化失败")
		return
	}
	rl = rl.withOutboundBytes(len(body))
	resp, acct, err := s.openModel(ctx, deepai.PathChat, body, affinity, s.requestGroup(r), rl.apiKey(), true, s.requireQuotaFor(rl.model))
	rl.acct = acct
	if err != nil {
		rl.done(0, gateway.Usage{}, err)
		writeAnthropicUpstreamErr(w, err)
		return
	}
	defer resp.Body.Close()

	sw := newSSEWriter(w)

	msgID := "msg_" + deepai.RandomHex(12)
	send := func(event string, v any) {
		if raw := jsonEvent(v); raw != nil {
			sw.event(event, raw)
		}
	}

	send("message_start", map[string]any{
		"type": "message_start",
		"message": map[string]any{
			"id": msgID, "type": "message", "role": "assistant", "model": rl.model,
			"content": []any{}, "stop_reason": nil, "stop_sequence": nil,
			"usage": map[string]any{"input_tokens": 0, "output_tokens": 0},
		},
	})

	block := gateway.NewAnthropicBlockWriter(send)
	var (
		usage      gateway.Usage
		outLen     int
		stopReason = "end_turn"
		streamErr  error
	)

	streamErr = forEachChatChunk(resp.Body, func(chunk map[string]any) bool {
		if u := gateway.ParseUsage(chunk["usage"]); u.TotalTokens > 0 || u.Cost > 0 {
			usage = u
		}
		d, finish := chatChunkDelta(chunk)
		if finish != "" {
			stopReason = gateway.AnthropicStopReason(finish)
		}
		if d.Reasoning != "" {
			block.Thinking(d.Reasoning)
			outLen += len([]rune(d.Reasoning))
		}
		if d.Content != "" {
			block.Text(d.Content)
			outLen += len([]rune(d.Content))
		}
		for _, tc := range d.ToolCalls {
			block.ToolCall(tc.Index, tc.ID, tc.Name, tc.Args)
		}
		return true
	})

	if streamErr != nil {
		// 出错时只关闭当前已打开的块，不 flush 工具缓冲：
		// 参数 JSON 可能只到了一半，当作完整 tool_use 发出去会让客户端解析失败。
		block.Close()
		send("error", map[string]any{
			"type":  "error",
			"error": map[string]any{"type": "api_error", "message": streamErr.Error()},
		})
		// Anthropic 的事件模型里 error 是终止事件：后面不能再跟 message_delta /
		// message_stop。旧实现在 error 之后照样补一条 message_delta
		// （stop_reason=end_turn），把“出错的流”宣告成正常结束，客户端可能把
		// 半截工具调用当成功。
		rl.done(outLen, usage, streamErr)
		return
	}
	block.Finish()
	outputTokens := usage.CompletionTokens
	if outputTokens == 0 {
		outputTokens = estTokens(outLen)
	}
	usageDelta := gateway.AnthropicUsage(usage)
	usageDelta["output_tokens"] = outputTokens
	send("message_delta", map[string]any{
		"type":  "message_delta",
		"delta": map[string]any{"stop_reason": stopReason, "stop_sequence": nil},
		"usage": usageDelta,
	})
	send("message_stop", map[string]any{"type": "message_stop"})

	rl.done(outLen, usage, nil)
}

// handleAnthropicCountTokens 返回粗略的输入 token 估算。
//
// Anthropic 客户端（如 Claude Code）会在发送消息前调用它做预算控制；
// 上游不提供该能力，这里用字符数近似，只需单调合理即可。
func (s *Server) handleAnthropicCountTokens(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeAnthropicErr(w, http.StatusMethodNotAllowed, "invalid_request_error", "仅支持 POST")
		return
	}
	req, ok := httpx.DecodeJSONRequest[gateway.AnthropicRequest](w, r, func(code int, msg string) {
		writeAnthropicErr(w, code, "invalid_request_error", msg)
	})
	if !ok {
		return
	}
	payload, err := gateway.AnthropicToChatPayload(&req, s.resolveModel(req.Model).ID)
	if err != nil {
		writeAnthropicErr(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	rawPayload, _ := json.Marshal(payload)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"input_tokens": estTokens(len(rawPayload)),
	})
}

// --- 工具函数 ---

// estTokens 用字符数粗略估算 token（1 token ≈ 2 字符）。
func estTokens(chars int) int {
	if chars <= 0 {
		return 0
	}
	return chars/2 + 1
}

// writeAnthropicErr 按 Anthropic 的错误结构返回。
func writeAnthropicErr(w http.ResponseWriter, code int, typ, msg string) {
	httpx.WriteJSON(w, code, map[string]any{
		"type":  "error",
		"error": map[string]any{"type": typ, "message": msg},
	})
}

// requestTimeout 返回设置里的请求超时。
// requestTimeout 返回单请求整体预算。
//
// 默认值与范围校验在 store.Settings.Normalize / Validate 里统一定义，
// 这里不再重复「<=0 就取 600」——那种兜底曾经散在三处。
func (s *Server) requestTimeout() time.Duration {
	return s.Store.Settings().RuntimeView().RequestTimeout
}
