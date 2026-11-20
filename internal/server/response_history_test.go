package server

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"deepaicode2api/internal/deepai"
	"deepaicode2api/internal/respstate"
)

// codewordUpstream 回一個假上游：它會把所有 chat messages 的字串化內容
// 拿去比對暗號。只有當「問題」與「暗號」同時出現在訊息裡才回答暗號，
// 否則回答 UNKNOWN —— 用來斷言歷史是否真的被重放到上游。
//
// 注意：自己讀 r.Body 並記錄（newUpstream 的包裝會先讀走 body），
// 因此这里直接建 httptest.Server，不走 newUpstream。
func codewordUpstream(t *testing.T, codeword string) *upstream {
	t.Helper()
	u := &upstream{}
	u.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var payload map[string]any
		_ = json.Unmarshal(raw, &payload)
		u.mu.Lock()
		u.requests = append(u.requests, recordedRequest{
			Path: r.URL.Path, Header: r.Header.Clone(), Payload: payload, Body: raw,
		})
		u.mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		blob, _ := json.Marshal(payload["messages"])
		text := string(blob)
		answer := "UNKNOWN"
		if strings.Contains(text, "What is the codeword") {
			if strings.Contains(text, codeword) {
				answer = "CODEWORD=" + codeword
			}
		} else {
			answer = "STORED"
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []any{map[string]any{
				"index": 0, "message": map[string]any{
					"role": "assistant", "content": answer,
				},
				"finish_reason": "stop",
			}},
			"usage": map[string]any{"prompt_tokens": 10, "completion_tokens": 3},
		})
	}))
	t.Cleanup(u.srv.Close)
	return u
}

func respIDAndText(t *testing.T, rec *httptest.ResponseRecorder) (string, string) {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("回應不是 JSON: %v\n%s", err, rec.Body.String())
	}
	id, _ := out["id"].(string)
	text, _ := out["output_text"].(string)
	return id, text
}

// TestResponsesChainMemory3Rounds 鎖住「鏈式記憶」：3 輪以上，
// 第一輪給暗號，第三輪用 previous_response_id 續接問暗號，必须答對。
func TestResponsesChainMemory3Rounds(t *testing.T) {
	const codeword = "ZEBRA42"
	u := codewordUpstream(t, codeword)
	s, key := newTestServer(t, u)

	// 第 1 輪：給暗號。
	r1 := do(t, s, http.MethodPost, "/v1/responses", key,
		`{"model":"glm-5.3-flash","input":"Remember codeword ZEBRA42"}`)
	if r1.Code != http.StatusOK {
		t.Fatalf("第 1 輪狀態 = %d：%s", r1.Code, r1.Body.String())
	}
	id1, _ := respIDAndText(t, r1)
	if id1 == "" {
		t.Fatal("第 1 輪沒有 id")
	}
	if got := s.history.Len(); got != 1 {
		t.Fatalf("歷史條目 = %d，期望 1", got)
	}

	// 第 2 輪：帶上一輪 id，問不相關問題（延長鏈）。
	r2 := do(t, s, http.MethodPost, "/v1/responses", key,
		`{"model":"glm-5.3-flash","previous_response_id":"`+id1+`","input":"Still there?"}`)
	if r2.Code != http.StatusOK {
		t.Fatalf("第 2 輪狀態 = %d：%s", r2.Code, r2.Body.String())
	}
	id2, _ := respIDAndText(t, r2)
	if id2 == "" || id2 == id1 {
		t.Fatalf("第 2 輪 id 應為新的：id1=%s id2=%s", id1, id2)
	}

	// 第 3 輪：帶第 2 輪 id 問暗號，必须答對。
	r3 := do(t, s, http.MethodPost, "/v1/responses", key,
		`{"model":"glm-5.3-flash","previous_response_id":"`+id2+`","input":"What is the codeword?"}`)
	if r3.Code != http.StatusOK {
		t.Fatalf("第 3 輪狀態 = %d：%s", r3.Code, r3.Body.String())
	}
	_, text := respIDAndText(t, r3)
	if !strings.Contains(text, "CODEWORD="+codeword) {
		t.Fatalf("第 3 輪失憶：output_text = %q，期望含 CODEWORD=%s", text, codeword)
	}

	// 斷言上游確實收到了被重放的暗號。
	last := u.last(t)
	blob, _ := json.Marshal(last.Payload["messages"])
	if !strings.Contains(string(blob), codeword) {
		t.Fatalf("上游未收到重放的暗號：%s", blob)
	}
	if n := s.metrics.Snapshot().Stateful["rejected"]; n != 0 {
		t.Fatalf("續接成功不應記拒絕，實際 = %d", n)
	}
}

// TestResponsesNonChainInputUnchanged 驗證非鏈式路徑（input 直接帶完整歷史）
// 不被改寫、不回歸：沒有 previous_response_id 时不應動 input。
func TestResponsesNonChainInputUnchanged(t *testing.T) {
	u := codewordUpstream(t, "ZEBRA42")
	s, key := newTestServer(t, u)

	body := `{"model":"glm-5.3-flash","input":[
		{"type":"message","role":"user","content":"Remember codeword ZEBRA42"},
		{"type":"message","role":"assistant","content":"STORED"},
		{"type":"message","role":"user","content":"What is the codeword?"}
	]}`
	rec := do(t, s, http.MethodPost, "/v1/responses", key, body)
	if rec.Code != http.StatusOK {
		t.Fatalf("狀態 = %d：%s", rec.Code, rec.Body.String())
	}
	last := u.last(t)
	msgs, ok := last.Payload["messages"].([]any)
	if !ok || len(msgs) != 3 {
		t.Fatalf("上游 messages 數 = %d，期望 3（原樣 3 條）：%#v", len(msgs), last.Payload["messages"])
	}
	if n := s.metrics.Snapshot().Stateful["rejected"]; n != 0 {
		t.Fatalf("非鏈式請求不應記 stateful 拒絕，實際 = %d", n)
	}
}

// TestResponsesStreamChainMemoryAndEvents 驗證串流路徑也存歷史、
// 能重放，且事件序列完整（created → deltas → completed）。
func TestResponsesStreamChainMemoryAndEvents(t *testing.T) {
	const codeword = "ZEBRA42"
	// 自建 httptest.Server 以便讀到 request body（newUpstream 會先讀走）。
	u := &upstream{}
	u.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		raw, _ := io.ReadAll(r.Body)
		var payload map[string]any
		_ = json.Unmarshal(raw, &payload)
		u.mu.Lock()
		u.requests = append(u.requests, recordedRequest{
			Path: r.URL.Path, Header: r.Header.Clone(), Payload: payload, Body: raw,
		})
		u.mu.Unlock()
		blob, _ := json.Marshal(payload["messages"])
		answer := "STORED"
		if strings.Contains(string(blob), "What is the codeword") {
			answer = "UNKNOWN"
			if strings.Contains(string(blob), codeword) {
				answer = "CODEWORD=" + codeword
			}
		}
		for _, c := range []string{
			`{"choices":[{"index":0,"delta":{"content":"` + answer + `"}}]}`,
			`{"choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
			`{"choices":[],"usage":{"prompt_tokens":10,"completion_tokens":3}}`,
			`[DONE]`,
		} {
			_, _ = io.WriteString(w, "data: "+c+"\n\n")
			w.(http.Flusher).Flush()
		}
	}))
	t.Cleanup(u.srv.Close)
	s, key := newTestServer(t, u)

	// 第 1 輪（串流）：給暗號，取出 response id。
	r1 := do(t, s, http.MethodPost, "/v1/responses", key,
		`{"model":"gpt-4o","stream":true,"input":"Remember codeword ZEBRA42"}`)
	if r1.Code != http.StatusOK {
		t.Fatalf("第 1 輪狀態 = %d：%s", r1.Code, r1.Body.String())
	}
	events1 := sseEvents(t, r1.Body.String())
	var id1 string
	for _, e := range events1 {
		if e.Event == "response.completed" {
			id1, _ = e.JSON(t)["response"].(map[string]any)["id"].(string)
		}
	}
	if id1 == "" {
		t.Fatal("串流第 1 輪 response.completed 缺少 id")
	}
	if got := s.history.Len(); got != 1 {
		t.Fatalf("串流後歷史條目 = %d，期望 1", got)
	}

	// 第 2 輪（串流）：帶 id 問暗號。
	r2 := do(t, s, http.MethodPost, "/v1/responses", key,
		`{"model":"gpt-4o","stream":true,"previous_response_id":"`+id1+`","input":"What is the codeword?"}`)
	if r2.Code != http.StatusOK {
		t.Fatalf("第 2 輪狀態 = %d：%s", r2.Code, r2.Body.String())
	}

	// 事件序列完整。
	events2 := sseEvents(t, r2.Body.String())
	names := eventNames(events2)
	if len(names) == 0 || names[0] != "response.created" {
		t.Fatalf("首事件 = %v，期望 response.created 開頭", names)
	}
	if last := names[len(names)-1]; last != "response.completed" {
		t.Fatalf("末事件 = %q，期望 response.completed（全部：%v）", last, names)
	}
	for _, want := range []string{
		"response.output_item.added", "response.output_text.delta",
		"response.output_text.done", "response.completed",
	} {
		found := false
		for _, n := range names {
			if n == want {
				found = true
			}
		}
		if !found {
			t.Fatalf("串流缺少事件 %q：%v", want, names)
		}
	}

	// 文字含暗號 → 證明串流路徑也重放了歷史。
	var text strings.Builder
	for _, e := range events2 {
		if e.Event == "response.output_text.delta" {
			if d, ok := e.JSON(t)["delta"].(string); ok {
				text.WriteString(d)
			}
		}
	}
	if !strings.Contains(text.String(), "CODEWORD="+codeword) {
		t.Fatalf("串流失憶：文字 = %q", text.String())
	}
}

func TestRememberResponseSkipsIncomplete(t *testing.T) {
	s := &Server{history: respstate.New(time.Hour, 10)}
	turn := &respstate.Turn{Input: []any{"hello"}}
	s.rememberResponse(turn, map[string]any{
		"id": "resp_incomplete", "status": "incomplete",
		"output": []any{map[string]any{"type": "message"}},
	})
	if n := s.history.Len(); n != 0 {
		t.Fatalf("incomplete response 不應進入歷史快取，實際 %d 條", n)
	}
}

// TestResponsesPartialInputWithPrevIDNotDuplicated 驗證「只重送歷史尾段」時
// 也不重複拼接：客戶端帶回上一輪的 user + 上一輪的 assistant output + 新問題，
// 網關按後綴重疊去重，開頭暗號仍保留、重送的內容只出現一次。
func TestResponsesPartialInputWithPrevIDNotDuplicated(t *testing.T) {
	u := codewordUpstream(t, "ZEBRA42")
	s, key := newTestServer(t, u)

	r1 := do(t, s, http.MethodPost, "/v1/responses", key,
		`{"model":"glm-5.3-flash","input":"Remember codeword ZEBRA42"}`)
	id1, _ := respIDAndText(t, r1)
	r2 := do(t, s, http.MethodPost, "/v1/responses", key,
		`{"model":"glm-5.3-flash","previous_response_id":"`+id1+`","input":"turn 2"}`)
	if r2.Code != http.StatusOK {
		t.Fatalf("第 2 輪 = %d：%s", r2.Code, r2.Body.String())
	}
	id2, _ := respIDAndText(t, r2)

	var out map[string]any
	if err := json.Unmarshal(r2.Body.Bytes(), &out); err != nil {
		t.Fatalf("第 2 輪回應不是 JSON: %v", err)
	}
	output, _ := out["output"].([]any)
	if len(output) == 0 {
		t.Fatalf("第 2 輪沒有 output：%s", r2.Body.String())
	}

	// 只重送最後一輪（user「turn 2」+ 該輪 output）+ 新問題。
	in := append([]any{map[string]any{"type": "message", "role": "user", "content": "turn 2"}}, output...)
	in = append(in, map[string]any{"type": "message", "role": "user", "content": "What is the codeword?"})
	r3 := do(t, s, http.MethodPost, "/v1/responses", key, mustMarshal(t, map[string]any{
		"model": "glm-5.3-flash", "previous_response_id": id2, "input": in,
	}))
	if r3.Code != http.StatusOK {
		t.Fatalf("第 3 輪 = %d：%s", r3.Code, r3.Body.String())
	}
	if _, text := respIDAndText(t, r3); !strings.Contains(text, "ZEBRA42") {
		t.Fatalf("重送尾段後仍應保留開頭暗號，實際回答 %q", text)
	}

	blob, _ := json.Marshal(u.last(t).Payload["messages"])
	if n := strings.Count(string(blob), `turn 2`); n != 1 {
		t.Fatalf("重送的尾段不應重複（\"turn 2\" 出現 %d 次）：%s", n, blob)
	}
}

// TestResponsesConversationStillRejected conversation / item_reference
// 需要上游服務端狀態，網關無從重建，保留明確 400。
func TestResponsesConversationStillRejected(t *testing.T) {
	u := newUpstream(t, func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("conversation 不應打到上游")
	})
	s, key := newTestServer(t, u)

	for _, body := range []string{
		`{"model":"glm-5.3-flash","conversation":"conv_abc","input":"hi"}`,
		`{"model":"glm-5.3-flash","input":[{"type":"item_reference","id":"item_1"}]}`,
	} {
		rec := do(t, s, http.MethodPost, "/v1/responses", key, body)
		if rec.Code != http.StatusBadRequest ||
			!strings.Contains(rec.Body.String(), "unsupported_parameter") {
			t.Fatalf("應回 400 unsupported_parameter：%s（%s）", rec.Body.String(), body)
		}
	}
}

// TestResponsesTruncationAuto 驗證 truncation:auto 在超限时裁剪最舊中段，
// 保留開頭暗號與最近訊息，照常送上游（200，暗號仍可見）。
func TestResponsesTruncationAuto(t *testing.T) {
	const codeword = "ZEBRA42"
	u := codewordUpstream(t, codeword)
	s, key := newTestServer(t, u)
	// 用全域環境覆寫把有效窗口壓到 400 tokens，便於測試多輪舊歷史裁剪。
	t.Setenv("DEEPAI2API_EFFECTIVE_CONTEXT_WINDOW", "400")

	// 第 1 輪：暗號。
	r1 := do(t, s, http.MethodPost, "/v1/responses", key,
		`{"model":"glm-5.3-flash","input":"Remember codeword ZEBRA42"}`)
	if r1.Code != http.StatusOK {
		t.Fatalf("第 1 輪 = %d：%s", r1.Code, r1.Body.String())
	}
	id, _ := respIDAndText(t, r1)

	// 累積多輪中段歷史，再以短問題觸發舊歷史裁剪。
	fill := strings.Repeat("filler ", 50)
	body2 := `{"model":"glm-5.3-flash","previous_response_id":"` + id +
		`","truncation":"auto","input":"` + fill + `"}`
	r2 := do(t, s, http.MethodPost, "/v1/responses", key, body2)
	if r2.Code != http.StatusOK {
		t.Fatalf("第 2 輪 = %d：%s", r2.Code, r2.Body.String())
	}
	id, _ = respIDAndText(t, r2)
	body3 := `{"model":"glm-5.3-flash","previous_response_id":"` + id +
		`","truncation":"auto","input":"What is the codeword?"}`
	r3 := do(t, s, http.MethodPost, "/v1/responses", key, body3)
	if r3.Code != http.StatusOK {
		t.Fatalf("truncation:auto 應回 200，實際 %d：%s", r3.Code, r3.Body.String())
	}

	// 上游收到的 messages 應含裁減標記、開頭暗號、以及最新提問。
	last := u.last(t)
	blob, _ := json.Marshal(last.Payload["messages"])
	text := string(blob)
	if !strings.Contains(text, "自動省略") {
		t.Fatalf("truncation:auto 應插入裁減說明：%s", text)
	}
	// 裁減標記必須是 user role：上游會拒絕 system 出現在非首則的請求體。
	msgs, _ := last.Payload["messages"].([]any)
	if len(msgs) > 1 {
		for i, m := range msgs {
			mm, _ := m.(map[string]any)
			if i > 0 && strOf(mm["role"]) == "system" {
				t.Fatalf("裁減後 system 出現在非首則（上游會 400）：%s", text)
			}
		}
	}
	if !strings.Contains(text, codeword) {
		t.Fatalf("裁減後應保留開頭暗號：%s", text)
	}
	if !strings.Contains(text, "What is the codeword") {
		t.Fatalf("裁減後應保留最新提問：%s", text)
	}
}

// TestResponsesTruncationRequired 驗證未設 truncation 時超限回明確 400，
// 不靜默砍歷史（那是失憶根源）。
func TestResponsesTruncationRequired(t *testing.T) {
	u := codewordUpstream(t, "ZEBRA42")
	s, key := newTestServer(t, u)
	t.Setenv("DEEPAI2API_EFFECTIVE_CONTEXT_WINDOW", "200")

	r1 := do(t, s, http.MethodPost, "/v1/responses", key,
		`{"model":"glm-5.3-flash","input":"Remember codeword ZEBRA42"}`)
	if r1.Code != http.StatusOK {
		t.Fatalf("第 1 輪 = %d：%s", r1.Code, r1.Body.String())
	}
	id, _ := respIDAndText(t, r1)

	longFill := strings.Repeat("filler word ", 120)
	body2 := `{"model":"glm-5.3-flash","previous_response_id":"` + id +
		`","input":"What is the codeword? ` + longFill + `"}`
	r2 := do(t, s, http.MethodPost, "/v1/responses", key, body2)
	if r2.Code != http.StatusBadRequest {
		t.Fatalf("未設 truncation 超限應回 400，實際 %d：%s", r2.Code, r2.Body.String())
	}
	if !strings.Contains(r2.Body.String(), "contextWindow") ||
		!strings.Contains(r2.Body.String(), "truncation") {
		t.Fatalf("400 訊息應指明 contextWindow 與 truncation：%s", r2.Body.String())
	}
}

// 確保 deepai import 被使用（模型的 IsResponses 路徑在別的測試覆蓋）。
var _ = deepai.PathChat

// TestResponsesReplayCountsTools 驗證重放時 tools 定義也計入上下文：
// tools schema 每輪都隨請求送出，不算進去會讓剛好超限的請求穿過檢查、
// 由上游靜默截斷歷史（正是 issue #5 的失憶形態）。
func TestResponsesReplayCountsTools(t *testing.T) {
	u := codewordUpstream(t, "ZEBRA42")
	s, key := newTestServer(t, u)
	t.Setenv("DEEPAI2API_EFFECTIVE_CONTEXT_WINDOW", "120")

	r1 := do(t, s, http.MethodPost, "/v1/responses", key,
		`{"model":"glm-5.3-flash","input":"hi"}`)
	if r1.Code != http.StatusOK {
		t.Fatalf("第 1 輪 = %d：%s", r1.Code, r1.Body.String())
	}
	id, _ := respIDAndText(t, r1)

	// 歷史只有十幾 token，tools 定義約 250 token：合計超過 120 的窗口。
	tools := `[{"type":"function","name":"lookup","description":"` +
		strings.Repeat("d", 400) + `","parameters":{"type":"object"}}]`
	body := `{"model":"glm-5.3-flash","previous_response_id":"` + id +
		`","input":"again","tools":` + tools + `}`
	r2 := do(t, s, http.MethodPost, "/v1/responses", key, body)
	if r2.Code != http.StatusBadRequest {
		t.Fatalf("tools 撑破窗口應回 400，實際 %d：%s", r2.Code, r2.Body.String())
	}
	if !strings.Contains(r2.Body.String(), "contextWindow") {
		t.Fatalf("400 訊息應指明 contextWindow：%s", r2.Body.String())
	}
}

// --- 增量存儲 / 前綴去重 / 輸出預留 / 窗口覆寫緩存 ---

// TestResponsesFullInputWithPrevIDNotDuplicated 驗證「同時送 previous_response_id
// 與完整 input」時不再把基底拼兩次：同一段歷史只出現一次。
func TestResponsesFullInputWithPrevIDNotDuplicated(t *testing.T) {
	u := codewordUpstream(t, "ZEBRA42")
	s, key := newTestServer(t, u)

	r1 := do(t, s, http.MethodPost, "/v1/responses", key,
		`{"model":"glm-5.3-flash","input":"Remember codeword ZEBRA42"}`)
	if r1.Code != http.StatusOK {
		t.Fatalf("第 1 輪 = %d：%s", r1.Code, r1.Body.String())
	}
	id, _ := respIDAndText(t, r1)

	var out map[string]any
	if err := json.Unmarshal(r1.Body.Bytes(), &out); err != nil {
		t.Fatalf("第 1 輪回應不是 JSON: %v", err)
	}
	output, _ := out["output"].([]any)
	if len(output) == 0 {
		t.Fatalf("第 1 輪沒有 output：%s", r1.Body.String())
	}

	// 客戶端把「首輪 input + 首輪 output + 新問題」整包送回來。
	in := []any{map[string]any{
		"type": "message", "role": "user", "content": "Remember codeword ZEBRA42",
	}}
	in = append(in, output...)
	in = append(in, map[string]any{
		"type": "message", "role": "user", "content": "What is the codeword?",
	})
	r2 := do(t, s, http.MethodPost, "/v1/responses", key, mustMarshal(t, map[string]any{
		"model": "glm-5.3-flash", "previous_response_id": id, "input": in,
	}))
	if r2.Code != http.StatusOK {
		t.Fatalf("第 2 輪 = %d：%s", r2.Code, r2.Body.String())
	}

	blob, _ := json.Marshal(u.last(t).Payload["messages"])
	if n := strings.Count(string(blob), "Remember codeword ZEBRA42"); n != 1 {
		t.Fatalf("完整 input 不應再拼一次基底（暗號出現 %d 次）：%s", n, blob)
	}
}

// TestResponsesReplayReservesMaxOutputTokens 驗證 max_output_tokens 會佔掉窗口：
// 歷史貼近上限時，沒有給輸出留預算就會被上游靜默截斷。
func TestResponsesReplayReservesMaxOutputTokens(t *testing.T) {
	u := codewordUpstream(t, "ZEBRA42")
	s, key := newTestServer(t, u)
	t.Setenv("DEEPAI2API_EFFECTIVE_CONTEXT_WINDOW", "200")

	r1 := do(t, s, http.MethodPost, "/v1/responses", key,
		`{"model":"glm-5.3-flash","input":"hi"}`)
	if r1.Code != http.StatusOK {
		t.Fatalf("第 1 輪 = %d：%s", r1.Code, r1.Body.String())
	}
	id, _ := respIDAndText(t, r1)

	// 歷史只有十幾 token，但 max_output_tokens=300 讓總需求超過 200。
	r2 := do(t, s, http.MethodPost, "/v1/responses", key,
		`{"model":"glm-5.3-flash","previous_response_id":"`+id+
			`","input":"again","max_output_tokens":300}`)
	if r2.Code != http.StatusBadRequest {
		t.Fatalf("輸出預算擠破窗口應回 400，實際 %d：%s", r2.Code, r2.Body.String())
	}
	if !strings.Contains(r2.Body.String(), "max_output_tokens") {
		t.Fatalf("400 訊息應說明 max_output_tokens 預留：%s", r2.Body.String())
	}

	// 對照：不宣告輸出預算就照常放行。
	r3 := do(t, s, http.MethodPost, "/v1/responses", key,
		`{"model":"glm-5.3-flash","previous_response_id":"`+id+`","input":"again"}`)
	if r3.Code != http.StatusOK {
		t.Fatalf("未宣告 max_output_tokens 應回 200，實際 %d：%s", r3.Code, r3.Body.String())
	}
}
