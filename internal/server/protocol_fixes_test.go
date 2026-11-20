package server

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"deepaicode2api/internal/deepai"
	"deepaicode2api/internal/jsonx"
	"deepaicode2api/internal/poolsvc"
	"deepaicode2api/internal/store"
)

// ctxT 返回一个带超时的测试上下文。
func ctxT(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	return ctx
}

// timeAgo 返回 d 之前的时间点（用于制造过期绑定）。
func timeAgo(d time.Duration) time.Time { return time.Now().Add(-d) }

// httptestRequestWithHeader 构造一个只带指定请求头的请求。
func httptestRequestWithHeader(name, value string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	req.Header.Set(name, value)
	return req
}

// --- 协议修补的回归测试（v0.1.8）---

// TestAnthropicStreamErrorOmitsTerminalEvents 锁住「出错即终止」的语义。
//
// Anthropic 的事件模型里 `error` 是终止事件：后面不能再跟 message_delta /
// message_stop。旧实现在 error 之后照样补一条 message_delta（stop_reason
// 被写成 end_turn），等于把一次中途失败的流宣告成正常结束 —— 客户端会把
// 只到一半的工具调用当成功处理。
func TestAnthropicStreamErrorOmitsTerminalEvents(t *testing.T) {
	u := newUpstream(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		// 声明比实际写出的长度更长：客户端读到 io.ErrUnexpectedEOF，
		// 与上游中途断连时的形态一致。
		w.Header().Set("Content-Length", "4096")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"半截\"}}]}\n\n")
		w.(http.Flusher).Flush()
	})
	s, key := newTestServer(t, u)

	body := `{"model":"claude-sonnet-4","max_tokens":64,"stream":true,
	  "messages":[{"role":"user","content":"hi"}]}`
	rec := do(t, s, http.MethodPost, "/v1/messages", key, body)

	events := sseEvents(t, rec.Body.String())
	names := eventNames(events)
	sawError := false
	for _, n := range names {
		switch n {
		case "error":
			sawError = true
		case "message_delta", "message_stop":
			t.Fatalf("出错后不应再发 %q: %v", n, names)
		}
	}
	if !sawError {
		t.Fatalf("流中断应产生 error 事件: %v", names)
	}
}

// TestResponsesStreamCachedTokens 验证缓存命中量透传到 Responses 的 usage。
//
// 上游在末帧给 `prompt_tokens_details.cached_tokens`，旧实现把它硬编码成 0，
// 于是所有走 Responses 协议的客户端看到的缓存命中率永远是 0。
func TestResponsesStreamCachedTokens(t *testing.T) {
	u := newUpstream(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		chunks := []string{
			`{"choices":[{"index":0,"delta":{"content":"hi"}}]}`,
			`{"choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":1,"total_tokens":11,"prompt_tokens_details":{"cached_tokens":6}}}`,
			`[DONE]`,
		}
		for _, c := range chunks {
			_, _ = io.WriteString(w, "data: "+c+"\n\n")
			w.(http.Flusher).Flush()
		}
	})
	s, key := newTestServer(t, u)

	body := `{"model":"glm-5.3-flash","stream":true,"input":"hi"}`
	rec := do(t, s, http.MethodPost, "/v1/responses", key, body)
	if rec.Code != http.StatusOK {
		t.Fatalf("状态码 = %d：%s", rec.Code, rec.Body.String())
	}

	var cached float64 = -1
	for _, e := range sseEvents(t, rec.Body.String()) {
		m := e.JSON(t)
		// response.completed 把 usage 嵌在 response 对象里。
		resp, ok := m["response"].(map[string]any)
		if !ok {
			continue
		}
		usage, ok := resp["usage"].(map[string]any)
		if !ok {
			continue
		}
		details, ok := usage["input_tokens_details"].(map[string]any)
		if !ok {
			continue
		}
		if v, ok := details["cached_tokens"].(float64); ok {
			cached = v
		}
	}
	if cached != 6 {
		t.Fatalf("input_tokens_details.cached_tokens = %v，期望 6（上游已回传）", cached)
	}
}

// TestStickyPersistsAcrossRestart 验证绑定表走旁路文件随重启保留。
//
// 绑定是缓存：丢了不影响正确性，但重启就重新学一遍等于每次升级都把前缀缓存
// 冲掉一次，所以必须落盘。
func TestStickyPersistsAcrossRestart(t *testing.T) {
	s, _ := newTestServer(t, nil)
	fillPool(t, s, 2)
	s.Pool.BindSticky("sess-restart", "acc-1")
	s.Pool.FlushSticky()

	// 同一个状态文件重新打开一个 Server：绑定应当已经回来了。
	s2 := New(s.Store)
	defer s2.Close(ctxT(t))
	if got := s2.Pool.StickyAccount("sess-restart"); got != "acc-1" {
		t.Fatalf("重启后绑定 = %q，期望 acc-1", got)
	}
	if n := s2.Pool.Bindings(); n != 1 {
		t.Fatalf("重启后绑定数 = %d，期望 1", n)
	}
}

// TestStickySkipsExpiredOnLoad 验证载入时剔除过期绑定。
func TestStickySkipsExpiredOnLoad(t *testing.T) {
	s, _ := newTestServer(t, nil)
	fillPool(t, s, 2)
	if err := s.Store.SaveSticky(map[string]store.StickyBinding{
		"old": {AccountID: "acc-0", At: timeAgo(poolsvc.StickyTTL * 2).Unix()},
		"new": {AccountID: "acc-1", At: time.Now().Unix()},
	}); err != nil {
		t.Fatalf("SaveSticky: %v", err)
	}

	s2 := New(s.Store)
	defer s2.Close(ctxT(t))
	if got := s2.Pool.StickyAccount("old"); got != "" {
		t.Fatalf("过期绑定不应恢复，实际 %q", got)
	}
	if got := s2.Pool.StickyAccount("new"); got != "acc-1" {
		t.Fatalf("有效绑定未恢复，实际 %q", got)
	}
}

// TestUpstreamLaneDrift 验证 /theos 回传的 lane_version 变成可观测信号。
//
// 2026-11-19 起上游对 ≤7 全放行，`/models` 探测再也发现不了「客户端已抬车道」；
// lane_version 是唯一信号，必须在 /health 与指标里看得到。
func TestUpstreamLaneDrift(t *testing.T) {
	u := newUpstream(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != deepai.ModelBasePath+deepai.PathTheos {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"enabled":true,"balance":"1","lane_version":9}`)
	})
	s, _ := newTestServer(t, u)

	acct := s.Store.Account("acc-1")
	if _, err := s.Up.Theos(ctxT(t), acct); err != nil {
		t.Fatalf("Theos: %v", err)
	}
	if got := s.UpstreamLane(); got != 9 {
		t.Fatalf("UpstreamLane = %d，期望 9", got)
	}
	drift := s.LaneDrift()
	if drift <= 0 {
		t.Fatalf("上游 9 > 网关 %s，漂移应为正数，实际 %d", s.CurrentLane(), drift)
	}

	rec := do(t, s, http.MethodGet, "/health", "", "")
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("解析 /health: %v", err)
	}
	if out["lane_upstream"].(float64) != 9 || out["lane_drift"].(float64) != float64(drift) {
		t.Fatalf("/health 车道字段不对: %#v", out)
	}
}

// TestSessionAffinityDerivation 验证不带会话头的客户端也能得到稳定 affinity
// （前缀缓存分区 + 选号粘性都靠它）：Claude Code 与普通 OpenAI SDK 调用方
// 都不发会话头，但两条链路的请求形状不一样。
func TestSessionAffinityDerivation(t *testing.T) {
	base := map[string]any{
		"model":  "glm-5.3-flash",
		"system": "你是助手",
		"messages": []any{
			map[string]any{"role": "system", "content": "你是助手"},
			map[string]any{"role": "user", "content": "第一轮"},
		},
		"tools": []any{map[string]any{"name": "f"}},
	}
	next := map[string]any{
		"model":  "glm-5.3-flash",
		"system": "你是助手",
		"messages": []any{
			map[string]any{"role": "system", "content": "你是助手"},
			map[string]any{"role": "user", "content": "第一轮"},
			map[string]any{"role": "assistant", "content": "答"},
			map[string]any{"role": "user", "content": "第二轮"},
		},
		"tools": []any{map[string]any{"name": "f"}},
	}
	other := map[string]any{
		"model":  "glm-5.3-flash",
		"system": "另一个角色",
		"messages": []any{
			map[string]any{"role": "system", "content": "另一个角色"},
			map[string]any{"role": "user", "content": "第一轮"},
		},
		"tools": []any{map[string]any{"name": "f"}},
	}

	a1 := sessionAffinity(nil, base)
	a2 := sessionAffinity(nil, next)
	a3 := sessionAffinity(nil, other)
	if a1 == "" || !affinityPattern.MatchString(a1) {
		t.Fatalf("派生的 affinity 不合法: %q", a1)
	}
	if a1 != a2 {
		t.Fatalf("同一会话（仅增长 user 轮次）应得到同一 affinity: %q vs %q", a1, a2)
	}
	if a1 == a3 {
		t.Fatalf("不同 system 应得到不同 affinity: %q", a1)
	}

	// 调用方自己给了会话头时优先用它。
	req := httptestRequestWithHeader("X-Session-Affinity", "client-sess")
	if got := sessionAffinity(req, base); got != "client-sess" {
		t.Fatalf("应优先使用调用方的会话头，实际 %q", got)
	}

	// Responses 形状：系统提示在 instructions，正文在 input；
	// 只改用户轮次不应改变 affinity。
	resp1 := map[string]any{
		"model":        "glm-5.3-flash",
		"instructions": "你是助手",
		"input":        []any{map[string]any{"role": "user", "content": "第一轮"}},
		"tools":        []any{map[string]any{"name": "f"}},
	}
	resp2 := map[string]any{
		"model":        "glm-5.3-flash",
		"instructions": "你是助手",
		"input": []any{
			map[string]any{"role": "user", "content": "第一轮"},
			map[string]any{"role": "user", "content": "第二轮"},
		},
		"tools": []any{map[string]any{"name": "f"}},
	}
	r1, r2 := sessionAffinity(nil, resp1), sessionAffinity(nil, resp2)
	if r1 == "" || r1 != r2 {
		t.Fatalf("Responses 形状应得到稳定 affinity: %q vs %q", r1, r2)
	}
}

// TestSessionAffinitySkipsBareRequests 验证「没有可缓存前缀」的裸请求不派生。
//
// 否则一句 "hi"（无 system、无 tools）也会算出同一个 affinity，把大量互不相关
// 的短请求钉到同一个账号上——那是负载集中，不是缓存命中。
func TestSessionAffinitySkipsBareRequests(t *testing.T) {
	bare := map[string]any{
		"model":    "glm-5.3-flash",
		"messages": []any{map[string]any{"role": "user", "content": "hi"}},
	}
	if got := sessionAffinity(nil, bare); got != "" {
		t.Fatalf("裸请求不应派生 affinity，实际 %q", got)
	}
	// 有 tools 就是可缓存前缀，应当派生。
	withTools := map[string]any{
		"model":    "glm-5.3-flash",
		"messages": []any{map[string]any{"role": "user", "content": "hi"}},
		"tools":    []any{map[string]any{"name": "f"}},
	}
	if got := sessionAffinity(nil, withTools); got == "" {
		t.Fatal("带 tools 的请求应派生 affinity")
	}
}

// --- issue #5：上下文 / 失忆 ---

// TestResponsesStatefulRejectedOnTranslatedModel 锁住「不再静默丢历史」。
//
// 非 openai-responses 模型走 chat 合成路径，只有 input 里的内容；
// previous_response_id 若被忽略，客户端以为续接成功，实际模型只看得到本轮。
func TestResponsesStatefulRejectedWhenHistoryMissing(t *testing.T) {
	u := newUpstream(t, func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("找不到历史时不应打到上游，路径 %s", r.URL.Path)
		w.WriteHeader(http.StatusOK)
	})
	s, key := newTestServer(t, u)

	rec := do(t, s, http.MethodPost, "/v1/responses", key,
		`{"model":"glm-5.3-flash","previous_response_id":"resp_abc","input":"What is CODE?"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("状态码 = %d，期望 400：%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "unsupported_parameter") ||
		!strings.Contains(rec.Body.String(), "previous_response_id") {
		t.Fatalf("错误应点名字段与类型: %s", rec.Body.String())
	}
	if n := s.metrics.Snapshot().Stateful["rejected"]; n != 1 {
		t.Fatalf("拒绝计数 = %d，期望 1", n)
	}
}

// TestResponsesStatefulForwardedOnResponsesModel 验证透传分支不改写字段。
//
// gpt-6-astra 这类模型由上游 /responses 处理，网关原样转发、只记日志：
// 能否续接由上游决定（2026-11-19 该模型一直 429，尚未验证）。
func TestResponsesStatefulForwardedOnResponsesModel(t *testing.T) {
	u := newUpstream(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"resp_2","object":"response","status":"completed","output_text":"ok"}`)
	})
	s, key := newTestServer(t, u)
	s.Store.SetModels([]deepai.Model{{ID: "resp-model", API: "openai-responses"}}, "resp-model")

	rec := do(t, s, http.MethodPost, "/v1/responses", key,
		`{"model":"resp-model","previous_response_id":"resp_1","input":"and then?"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("状态码 = %d：%s", rec.Code, rec.Body.String())
	}
	last := u.last(t)
	if last.Path != deepai.ModelBasePath+deepai.PathResponses {
		t.Fatalf("应透传到上游 /responses，实际 %q", last.Path)
	}
	if got, _ := last.Payload["previous_response_id"].(string); got != "resp_1" {
		t.Fatalf("previous_response_id 被改写/丢弃: %#v", last.Payload["previous_response_id"])
	}
	if n := s.metrics.Snapshot().Stateful["previous_response_id"]; n != 1 {
		t.Fatalf("previous_response_id 计数 = %d，期望 1", n)
	}
}

// TestChatMultiTurnHistoryPreserved 验证多轮历史逐字透传（网关不改写、不截断）。
func TestChatMultiTurnHistoryPreserved(t *testing.T) {
	u := newUpstream(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"choices":[{"message":{"role":"assistant","content":"ALPHA-7"}}]}`)
	})
	s, key := newTestServer(t, u)

	body := `{"model":"glm-5.3-flash","stream":false,"messages":[
	  {"role":"system","content":"Remember CODE=ALPHA-7."},
	  {"role":"user","content":"What is CODE?"},
	  {"role":"assistant","content":"ALPHA-7"},
	  {"role":"user","content":"Again?"}]}`
	rec := do(t, s, http.MethodPost, "/v1/chat/completions", key, body)
	if rec.Code != http.StatusOK {
		t.Fatalf("状态码 = %d：%s", rec.Code, rec.Body.String())
	}
	msgs, _ := u.last(t).Payload["messages"].([]any)
	if len(msgs) != 4 {
		t.Fatalf("上游收到 %d 条消息，期望 4（历史被截断）：%s", len(msgs), string(u.last(t).Body))
	}
	first, _ := msgs[0].(map[string]any)
	if jsonx.StrOf(first["content"]) != "Remember CODE=ALPHA-7." {
		t.Fatalf("system 内容被改写: %v", first["content"])
	}
}

// TestAnthropicMultiTurnToolsPreserved 覆盖 Anthropic 多轮 + 工具结果的转换。
func TestAnthropicMultiTurnToolsPreserved(t *testing.T) {
	u := newUpstream(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"choices":[{"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`)
	})
	s, key := newTestServer(t, u)

	body := `{"model":"claude-sonnet-5-5","max_tokens":64,"messages":[
	  {"role":"user","content":"查天气"},
	  {"role":"assistant","content":[
	    {"type":"text","text":"我查一下"},
	    {"type":"tool_use","id":"call_1","name":"get_weather","input":{"city":"上海"}}]},
	  {"role":"user","content":[
	    {"type":"tool_result","tool_use_id":"call_1","content":"晴 26℃"},
	    {"type":"text","text":"那要带伞吗？"}]}]}`
	rec := do(t, s, http.MethodPost, "/v1/messages", key, body)
	if rec.Code != http.StatusOK {
		t.Fatalf("状态码 = %d：%s", rec.Code, rec.Body.String())
	}
	msgs, _ := u.last(t).Payload["messages"].([]any)
	if len(msgs) != 4 {
		t.Fatalf("上游收到 %d 条消息，期望 4（user/assistant(tool_use)/tool/user）：%s", len(msgs), string(u.last(t).Body))
	}
	assistant, _ := msgs[1].(map[string]any)
	if _, ok := assistant["tool_calls"]; !ok {
		t.Fatalf("assistant 消息缺少 tool_calls: %v", assistant)
	}
	tool, _ := msgs[2].(map[string]any)
	if jsonx.StrOf(tool["role"]) != "tool" || jsonx.StrOf(tool["tool_call_id"]) != "call_1" {
		t.Fatalf("tool 消息不合法: %v", tool)
	}
	last, _ := msgs[3].(map[string]any)
	if jsonx.StrOf(last["role"]) != "user" {
		t.Fatalf("tool_result 之后的文本应成为 user 消息: %v", last)
	}
}

// TestHistoryStats 覆盖日志里的历史规模统计（不含正文）。
func TestHistoryStats(t *testing.T) {
	msgs, sys := historyStats(map[string]any{
		"messages": []any{
			map[string]any{"role": "system", "content": "s"},
			map[string]any{"role": "user", "content": "u"},
		},
	})
	if msgs != 2 || !sys {
		t.Fatalf("messages 形态: msgs=%d sys=%v", msgs, sys)
	}

	msgs, sys = historyStats(map[string]any{
		"instructions": "s",
		"input": []any{
			map[string]any{"type": "message", "role": "user", "content": "u"},
			map[string]any{"type": "function_call", "call_id": "c", "name": "f"},
		},
	})
	if msgs != 1 || !sys {
		t.Fatalf("responses 形态: msgs=%d sys=%v", msgs, sys)
	}

	msgs, sys = historyStats(map[string]any{"input": "hi"})
	if msgs != 1 || sys {
		t.Fatalf("input 简写: msgs=%d sys=%v", msgs, sys)
	}
}

// TestShapeFallbackKeepsHistory 验证「补形状重试」不会吃掉历史。
//
// 兜底路径会裁剪白名单字段（user 之类会被丢掉），但 messages 必须原样保留 ——
// 否则一次 400 兜底就能把多轮对话截断成单轮，看起来又像“失忆”。
func TestShapeFallbackKeepsHistory(t *testing.T) {
	var calls atomic.Int64
	u := newUpstream(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, deepai.PathTheos) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"enabled":true,"lane_version":7}`)
			return
		}
		if calls.Add(1) == 1 {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, `{"error":{"message":"This request is not one DeepAI Code sends.",`+
				`"code":"invalid_request_error"}}`)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"choices":[{"message":{"role":"assistant","content":"ok"}}]}`)
	})
	s, key := newTestServer(t, u)

	body := `{"model":"glm-5.3-flash","stream":false,"messages":[
	  {"role":"user","content":"turn-1"},
	  {"role":"assistant","content":"reply-1"},
	  {"role":"user","content":"turn-2"}],"user":"u1"}`
	rec := do(t, s, http.MethodPost, "/v1/chat/completions", key, body)
	if rec.Code != http.StatusOK {
		t.Fatalf("状态码 = %d：%s", rec.Code, rec.Body.String())
	}
	if calls.Load() != 2 {
		t.Fatalf("应重试一次，实际 %d 次", calls.Load())
	}
	msgs, _ := u.last(t).Payload["messages"].([]any)
	if len(msgs) != 4 {
		t.Fatalf("兜底重试后 %d 条消息，期望 4（3 条历史 + 补的 system）：%s", len(msgs), string(u.last(t).Body))
	}
	// 顺序：system 在最前，其余原序（不能打乱轮次）。
	wantRoles := []string{"system", "user", "assistant", "user"}
	for i, mv := range msgs {
		m, _ := mv.(map[string]any)
		if got := jsonx.StrOf(m["role"]); got != wantRoles[i] {
			t.Fatalf("第 %d 条角色 = %q，期望 %q", i, got, wantRoles[i])
		}
	}
	if got := jsonx.StrOf(msgs[2].(map[string]any)["content"]); got != "reply-1" {
		t.Fatalf("历史正文被改写: %q", got)
	}
}
