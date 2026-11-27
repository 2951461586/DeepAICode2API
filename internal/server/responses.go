package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"deepaicode2api/internal/deepai"
	"deepaicode2api/internal/gateway"
	"deepaicode2api/internal/httpx"
	"deepaicode2api/internal/jsonx"
	"deepaicode2api/internal/logx"
	"deepaicode2api/internal/respstate"
)

// handleResponses 是 /v1/responses 的入口。
//
// 两条路径：
//   - 目录里 api=openai-responses 的模型：原样透传到上游 /responses；
//   - 其余模型：把 Responses 请求翻译成 Chat Completions，再把结果
//     合成为 Responses 对象 / 事件流。
func (s *Server) handleResponses(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpx.WriteErr(w, http.StatusMethodNotAllowed, "仅支持 POST", "invalid_request_error")
		return
	}
	start := time.Now()

	payload, ok := httpx.DecodeJSONRequest[map[string]any](w, r, func(code int, msg string) {
		httpx.WriteErr(w, code, msg, "invalid_request_error")
	})
	if !ok {
		return
	}

	model, modelOK := s.resolveModelOK(requestModelName(payload))
	if !modelOK {
		httpx.WriteErr(w, http.StatusNotFound, s.unknownModelMessage(requestModelName(payload)), "invalid_request_error")
		return
	}
	payload["model"] = model.ID
	stream, _ := payload["stream"].(bool)
	affinity := sessionAffinity(r, payload)
	key := extractKey(r)
	promptLen := estimatePromptLen(payload)

	rls := s.newReqLog(key, nil, model.ID, "responses", stream, start, promptLen).withHistory(payload).withClient(r)
	// 先記下「本輪新增的 input」與續接父 id：重放會把 payload["input"] 換成完整
	// 歷史，那之後就取不到增量了（暫存只存增量 + 檢查點，完整歷史由鏈還原）。
	turn := &respstate.Turn{
		Input:        respstate.InputItems(payload),
		Instructions: jsonx.StrOf(payload["instructions"]),
		Parent:       jsonx.StrOf(payload["previous_response_id"]),
	}
	if p := gateway.StatefulParam(payload); p != "" {
		// previous_response_id：合成路徑現在透過伺服端歷史暫存重放上下文，
		// 不再直接回 400（那是「失憶」的根因）。重放失敗才退回明確 400。
		// conversation / item_reference 需要上游保存的服務端會話/條目，
		// 網關無從重建，保留明確 400 並在文件寫清楚。
		if !model.IsResponses() && p == "previous_response_id" {
			prevID := jsonx.StrOf(payload["previous_response_id"])
			outcome, msg := s.history.Replay(payload, prevID, model.ID, model.ContextWindow)
			switch outcome {
			case respstate.NotFound:
				s.metrics.AddStateful(p, true)
				httpx.WriteErr(w, http.StatusBadRequest,
					(&gateway.StatefulParamError{Param: p}).Error(), "unsupported_parameter")
				return
			case respstate.TooLong:
				s.metrics.AddStateful(p, true)
				httpx.WriteErr(w, http.StatusBadRequest, msg, "invalid_request_error")
				return
			}
			s.metrics.AddStateful(p, false) // 已重放，視為成功續接
			logx.Info("Responses 歷史重放成功", "model", model.ID, "prev", prevID)
		} else if !model.IsResponses() {
			s.metrics.AddStateful(p, true)
			httpx.WriteErr(w, http.StatusBadRequest,
				(&gateway.StatefulParamError{Param: p}).Error(), "unsupported_parameter")
			return
		} else {
			s.metrics.AddStateful(p, false)
			logx.Warn("上游 /responses 透傳帶狀態字段：續接能力取決於上游",
				"param", p, "model", model.ID)
		}
	}
	if model.IsResponses() {
		s.passthroughResponses(w, r, payload, affinity, rls, turn)
		return
	}

	chatPayload, err := gateway.ResponsesToChatPayload(payload, model.ID)
	if err != nil {
		httpx.WriteErr(w, http.StatusBadRequest, err.Error(), "invalid_request_error")
		return
	}
	if stream {
		s.streamResponsesAsChat(w, r, chatPayload, affinity, rls, turn)
		return
	}
	s.blockingResponsesAsChat(w, r, chatPayload, affinity, rls, turn)
}

// passthroughResponses 透传上游 /responses。
func (s *Server) passthroughResponses(w http.ResponseWriter, r *http.Request, payload map[string]any, affinity string, rl reqLog, turn *respstate.Turn) {
	ctx, cancel := httpx.ContextWithTimeout(r, s.requestTimeout())
	defer cancel()

	// 同上：上游缺省 stream 时按 SSE 返回，必须显式下发。
	payload["stream"] = rl.stream
	body, err := json.Marshal(payload)
	if err != nil {
		rl.doneStatus(http.StatusBadRequest, 0, gateway.Usage{}, err)
		httpx.WriteErr(w, http.StatusBadRequest, "请求序列化失败", "invalid_request_error")
		return
	}
	rl = rl.withOutboundBytes(len(body))
	resp, acct, err := s.openModel(ctx, deepai.PathResponses, body, affinity, s.requestGroup(r), rl.apiKey(), rl.stream, s.requireQuotaFor(rl.model))
	rl.acct = acct
	if err != nil {
		rl.done(0, gateway.Usage{}, err)
		writeUpstreamErr(w, err)
		return
	}
	defer resp.Body.Close()

	if !rl.stream {
		rawResp, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
		if err != nil {
			rl.done(0, gateway.Usage{}, err)
			httpx.WriteErr(w, http.StatusBadGateway, "读取上游响应失败: "+err.Error(), "upstream_error")
			return
		}
		var out map[string]any
		if err := json.Unmarshal(rawResp, &out); err != nil {
			// 上游回的不是 JSON（HTML 登录页 / SSE 帧 / 纯文本限流提示）。
			// 保持原样透传 + 200（客户端拿到的与上游一致），但**必须留日志**：
			// 旧实现在这里直接 return，请求会从控制台日志与统计里彻底消失。
			httpx.WriteRawJSON(w, http.StatusOK, rawResp)
			rl.doneStatus(http.StatusOK, 0, gateway.Usage{},
				fmt.Errorf("上游响应不是合法 JSON（已原样透传）: %w", err))
			return
		}
		rewriteResponsesModel(out, rl.model)
		usage := gateway.ParseUsage(out["usage"])
		outLen := responsesTextLen(out)
		if jsonx.StrOf(out["status"]) == "completed" {
			s.rememberResponse(turn, out)
		}
		encoded, _ := json.Marshal(out)
		rl.done(outLen, usage, nil)
		httpx.WriteRawJSON(w, http.StatusOK, encoded)
		return
	}

	sw := newSSEWriter(w)
	var (
		usage     gateway.Usage
		outLen    int
		finalResp map[string]any
	)
	streamErr := forEachSSEFrame(resp.Body, func(f sseFrame) bool {
		if f.Event != "" {
			_, _ = w.Write([]byte("event: " + f.Event + "\n"))
			return true
		}
		if f.Data == "" {
			return true
		}
		if f.Data == sseDone {
			sw.done()
			return false
		}
		obj := f.Object
		if obj == nil {
			_, _ = w.Write([]byte("data: " + f.Data + "\n\n"))
			sw.flush()
			return true
		}
		rewriteResponsesModel(obj, rl.model)
		// 記住最終 response 物件（含 id / output）：串流重放的依據。
		if jsonx.StrOf(obj["type"]) == "response.completed" {
			if respObj, ok := obj["response"].(map[string]any); ok {
				finalResp = respObj
			}
		}
		if respObj, ok := obj["response"].(map[string]any); ok {
			if u := gateway.ParseUsage(respObj["usage"]); u.TotalTokens > 0 || u.Cost > 0 {
				usage = u
			}
		}
		outLen += responsesEventTextLen(obj)
		// 必须写出改写后的对象：直接写原始 data 会把 model 改写丢掉。
		if raw := jsonEvent(obj); raw != nil {
			sw.data(raw)
			return true
		}
		_, _ = w.Write([]byte("data: " + f.Data + "\n\n"))
		sw.flush()
		return true
	})
	if streamErr != nil {
		payload := jsonEvent(map[string]any{
			"type": "error", "message": streamErr.Error(),
		})
		if payload != nil {
			sw.event("error", payload)
		}
	}
	if streamErr == nil {
		s.rememberResponse(turn, finalResp)
	}
	rl.done(outLen, usage, streamErr)
}

// blockingResponsesAsChat 用 Chat Completions 合成 Responses 响应。
func (s *Server) blockingResponsesAsChat(w http.ResponseWriter, r *http.Request, chatPayload map[string]any, affinity string, rl reqLog, turn *respstate.Turn) {
	ctx, cancel := httpx.ContextWithTimeout(r, s.requestTimeout())
	defer cancel()

	out, acct, outBytes, err := s.callJSON(ctx, deepai.PathChat, chatPayload, affinity, s.requestGroup(r), rl.apiKey(), s.requireQuotaFor(rl.model))
	if outBytes > 0 {
		rl = rl.withOutboundBytes(outBytes)
	}
	rl.acct = acct
	if err != nil {
		rl.done(0, gateway.Usage{}, err)
		writeUpstreamErr(w, err)
		return
	}
	respObj := gateway.ChatToResponsesObject(out, rl.model)
	usage := gateway.ParseUsage(out["usage"])
	outLen := responsesTextLen(respObj)

	s.rememberResponse(turn, respObj)
	rl.done(outLen, usage, nil)
	httpx.WriteJSON(w, http.StatusOK, respObj)
}

// streamResponsesAsChat 用 Chat Completions 流合成 Responses 事件流。
func (s *Server) streamResponsesAsChat(w http.ResponseWriter, r *http.Request, chatPayload map[string]any, affinity string, rl reqLog, turn *respstate.Turn) {
	ctx, cancel := httpx.ContextWithTimeout(r, s.requestTimeout())
	defer cancel()

	body, err := json.Marshal(chatPayload)
	if err != nil {
		rl.doneStatus(http.StatusBadRequest, 0, gateway.Usage{}, err)
		httpx.WriteErr(w, http.StatusBadRequest, "请求序列化失败", "invalid_request_error")
		return
	}
	rl = rl.withOutboundBytes(len(body))
	resp, acct, err := s.openModel(ctx, deepai.PathChat, body, affinity, s.requestGroup(r), rl.apiKey(), true, s.requireQuotaFor(rl.model))
	rl.acct = acct
	if err != nil {
		rl.done(0, gateway.Usage{}, err)
		writeUpstreamErr(w, err)
		return
	}
	defer resp.Body.Close()

	sw := newSSEWriter(w)
	gen := newResponsesStreamGen(sw, rl.model)
	gen.created()

	var (
		usage  gateway.Usage
		outLen int
	)
	streamErr := forEachChatChunk(resp.Body, func(chunk map[string]any) bool {
		if u := gateway.ParseUsage(chunk["usage"]); u.TotalTokens > 0 || u.Cost > 0 {
			usage = u
		}
		d, finish := chatChunkDelta(chunk)
		if d.Reasoning != "" {
			gen.reasoning(d.Reasoning)
			outLen += len([]rune(d.Reasoning))
		}
		if d.Content != "" {
			gen.textDelta(d.Content)
			outLen += len([]rune(d.Content))
		}
		for _, tc := range d.ToolCalls {
			gen.toolCall(tc.Index, tc.ID, tc.Name, tc.Args)
		}
		if finish != "" {
			gen.finishReason = finish
		}
		return true
	})

	if streamErr != nil {
		gen.fail(streamErr)
	} else {
		gen.complete(usage)
		s.rememberResponse(turn, gen.finalResponse)
	}
	rl.done(outLen, usage, streamErr)
}

// --- 工具函数 ---

// rememberResponse 從最終 Responses 物件取出 id / output 寫入歷史暫存。
// 只有成功完成（有 id 與 output）的回應才會被記住。
func (s *Server) rememberResponse(turn *respstate.Turn, resp map[string]any) {
	if resp == nil || s.history == nil || jsonx.StrOf(resp["status"]) != "completed" {
		return
	}
	id := jsonx.StrOf(resp["id"])
	output, _ := resp["output"].([]any)
	if id == "" || len(output) == 0 {
		return
	}
	s.history.Remember(turn, id, output)
}

// rewriteResponsesModel 把 Responses 对象 / 事件里的 model 改为对外模型名。
func rewriteResponsesModel(obj map[string]any, publicModel string) {
	if publicModel == "" {
		return
	}
	if _, ok := obj["model"]; ok {
		obj["model"] = publicModel
	}
	if resp, ok := obj["response"].(map[string]any); ok {
		rewriteResponsesModel(resp, publicModel)
	}
}

// responsesTextLen 统计 Responses 对象中的输出文本长度。
func responsesTextLen(obj map[string]any) int {
	total := 0
	if s, ok := obj["output_text"].(string); ok {
		total += len([]rune(s))
	}
	output, _ := obj["output"].([]any)
	for _, itemv := range output {
		item, ok := itemv.(map[string]any)
		if !ok {
			continue
		}
		switch jsonx.StrOf(item["type"]) {
		case "message":
			content, _ := item["content"].([]any)
			for _, cv := range content {
				c, ok := cv.(map[string]any)
				if ok {
					total += jsonx.LenOfString(c["text"])
				}
			}
		case "reasoning":
			summary, _ := item["summary"].([]any)
			for _, sv := range summary {
				sm, ok := sv.(map[string]any)
				if ok {
					total += jsonx.LenOfString(sm["text"])
				}
			}
		}
	}
	return total
}

// responsesEventTextLen 统计单个流事件的文本增量长度。
func responsesEventTextLen(obj map[string]any) int {
	if jsonx.StrOf(obj["type"]) == "response.output_text.delta" {
		return len([]rune(jsonx.StrOf(obj["delta"])))
	}
	if jsonx.StrOf(obj["type"]) == "response.reasoning_summary_text.delta" {
		return len([]rune(jsonx.StrOf(obj["delta"])))
	}
	return 0
}
