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
	if got := s.history.len(); got != 1 {
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
	if n := s.metrics.statefulReject.Load(); n != 0 {
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
	if n := s.metrics.statefulReject.Load(); n != 0 {
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
	if got := s.history.len(); got != 1 {
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

// TestResponsesHistoryTTLAndMax 驗證 TTL 與容量上限。
func TestResponsesHistoryTTLAndMax(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	h := newResponseHistory(time.Hour, 2)
	h.now = func() time.Time { return base }

	h.put("a", &historyEntry{input: []any{"a"}})
	h.now = func() time.Time { return base.Add(30 * time.Minute) }
	h.put("b", &historyEntry{input: []any{"b"}})
	h.now = func() time.Time { return base.Add(90 * time.Minute) } // a 已過期
	h.put("c", &historyEntry{input: []any{"c"}})
	if h.get("a") != nil {
		t.Fatal("過期條目 a 應被清除")
	}
	if h.get("b") == nil || h.get("c") == nil {
		t.Fatal("未過期條目應保留")
	}

	// 容量上限：再加兩筆後只留最新 2 筆。
	h.now = func() time.Time { return base.Add(95 * time.Minute) }
	h.put("d", &historyEntry{input: []any{"d"}})
	h.put("e", &historyEntry{input: []any{"e"}})
	if h.len() != 2 {
		t.Fatalf("容量上限後條目 = %d，期望 2", h.len())
	}
	if h.get("b") != nil || h.get("c") != nil {
		t.Fatal("最舊的 b/c 應被淘汰")
	}
	if h.get("d") == nil || h.get("e") == nil {
		t.Fatal("最新的 d/e 應保留")
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
	// 用全域環境覆寫把有效窗口壓到很小（約 200 tokens → 600 bytes），
	// 讓「開頭暗號 + 數輪填充 + 最新提問」必然超限；不影響按模型查表。
	t.Setenv("DEEPAI2API_EFFECTIVE_CONTEXT_WINDOW", "200")

	// 第 1 輪：暗號。
	r1 := do(t, s, http.MethodPost, "/v1/responses", key,
		`{"model":"glm-5.3-flash","input":"Remember codeword ZEBRA42"}`)
	if r1.Code != http.StatusOK {
		t.Fatalf("第 1 輪 = %d：%s", r1.Code, r1.Body.String())
	}
	id, _ := respIDAndText(t, r1)

	// 第 2 輪：帶長填充讓鏈變很長，truncation:auto。
	longFill := strings.Repeat("filler word ", 120) // ~1440 bytes → 远超 600
	body2 := `{"model":"glm-5.3-flash","previous_response_id":"` + id +
		`","truncation":"auto","input":"What is the codeword? ` + longFill + `"}`
	r2 := do(t, s, http.MethodPost, "/v1/responses", key, body2)
	if r2.Code != http.StatusOK {
		t.Fatalf("truncation:auto 應回 200，實際 %d：%s", r2.Code, r2.Body.String())
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

// TestItemApproxTokensCalibration 驗證估算與上游 input_tokens 偏差在 ±30% 內。
// 壓力測試實測：160k ASCII bytes → 上游 input_tokens=100,761（比值 1.59）。
func TestItemApproxTokensCalibration(t *testing.T) {
	ascii := strings.Repeat("a", 160000)
	got := itemApproxTokens(ascii)
	want := 100761 // 上游實測值
	if diff := float64(got-want) / float64(want); diff > 0.30 || diff < -0.30 {
		t.Fatalf("ASCII token 估算 = %d，上游實測 %d，偏差 %.1f%% 超出 ±30%%", got, want, diff*100)
	}
	// 中文：一個字元約 1 token。
	zh := itemApproxTokens(strings.Repeat("中", 1000))
	if zh < 800 || zh > 1200 {
		t.Fatalf("中文 token 估算 = %d，期望約 1000", zh)
	}
}

// TestEffectiveContextWindow 驗證有效窗口按模型實測值查表、其餘模型用目錄標示、
// 以及兩種環境覆寫（全域 + 按模型）都生效。
func TestEffectiveContextWindow(t *testing.T) {
	// 11 個免費模型的實測上限。
	want := map[string]int{
		"mimo-2.6-flash":      140000, // 140k 完整，170k 起靜默截斷 → 保守取 140k
		"glyph-cluster":       256000, // 260k 完整，320k 硬拒絕 → 目錄標 256k
		"glm-5.3-flash":       320000,
		"deepseek-v4.1-flash": 320000,
		"qwen-3.8-flash":      320000,
		"minimax-m3.1-flash":  320000,
		"muse-ember":          320000,
		"solar-mini-4":        320000,
		"gpt-6-luna":          320000,
		"mimo-2.6-pro":        320000,
		"deepai-cyber":        320000,
	}
	for id, exp := range want {
		if got := effectiveContextWindow(id, 320000); got != exp {
			t.Fatalf("%s 有效窗口 = %d，期望 %d", id, got, exp)
		}
	}
	// 未知/付費模型：min(標示, 320k)，不再硬壓 120k。
	if got := effectiveContextWindow("some-paid-model", 320000); got != 320000 {
		t.Fatalf("未知模型 320k 標示應為 320000，實際 %d", got)
	}
	if got := effectiveContextWindow("some-paid-model", 200); got != 200 {
		t.Fatalf("小於上限的標示應原樣保留，實際 %d", got)
	}

	// 全域環境覆寫（舊行為保留）。
	t.Setenv("DEEPAI2API_EFFECTIVE_CONTEXT_WINDOW", "64000")
	if got := effectiveContextWindow("glm-5.3-flash", 320000); got != 64000 {
		t.Fatalf("全域環境覆寫應為 64000，實際 %d", got)
	}
	t.Setenv("DEEPAI2API_EFFECTIVE_CONTEXT_WINDOW", "0")
	if got := effectiveContextWindow("glm-5.3-flash", 320000); got != 0 {
		t.Fatalf("全域設 0 應關閉上限，實際 %d", got)
	}
}

// TestEffectiveContextWindowPerModelOverride 驗證 DEEPAI2API_CONTEXT_WINDOWS
// 按模型 JSON 覆寫：只改指定模型，不影響其他模型；全域覆寫優先於按模型。
func TestEffectiveContextWindowPerModelOverride(t *testing.T) {
	t.Setenv("DEEPAI2API_EFFECTIVE_CONTEXT_WINDOW", "")
	t.Setenv("DEEPAI2API_CONTEXT_WINDOWS", `{"glm-5.3-flash":64000,"mimo-2.6-flash":0}`)
	if got := effectiveContextWindow("glm-5.3-flash", 320000); got != 64000 {
		t.Fatalf("按模型覆寫 glm 應為 64000，實際 %d", got)
	}
	if got := effectiveContextWindow("mimo-2.6-flash", 320000); got != 0 {
		t.Fatalf("按模型覆寫 mimo 設 0 應關閉上限，實際 %d", got)
	}
	// 未列入 JSON 的模型仍走表格/標示值。
	if got := effectiveContextWindow("glyph-cluster", 320000); got != 256000 {
		t.Fatalf("未覆寫的 glyph 應為 256000，實際 %d", got)
	}
	// 全域覆寫優先於按模型。
	t.Setenv("DEEPAI2API_EFFECTIVE_CONTEXT_WINDOW", "100000")
	if got := effectiveContextWindow("glm-5.3-flash", 320000); got != 100000 {
		t.Fatalf("全域覆寫應優先，實際 %d", got)
	}
	// 格式錯誤的 JSON 應忽略。
	t.Setenv("DEEPAI2API_EFFECTIVE_CONTEXT_WINDOW", "")
	t.Setenv("DEEPAI2API_CONTEXT_WINDOWS", `not-json`)
	if got := effectiveContextWindow("glm-5.3-flash", 320000); got != 320000 {
		t.Fatalf("錯誤 JSON 應忽略，實際 %d", got)
	}
}

// TestTruncationPointFollowsModelWindow 驗證 truncation:auto 的裁剪點跟著
// 各模型上限走：同一段約 20 萬 token 的歷史，mimo-flash（140k）會被裁，
// glm（320k）不會被裁——證明不再一律在 12 萬就提前裁剪。
func TestTruncationPointFollowsModelWindow(t *testing.T) {
	// 用 itemApproxTokens 反推：產生約 200000 tokens 的 ASCII（每 token ~1.6 bytes）。
	bigFill := strings.Repeat("a", 200000*8/5) // 320000 bytes → 約 200000 tokens
	items := []any{
		map[string]any{"type": "message", "role": "user", "content": "開頭暗號 ZEBRA42"},
		map[string]any{"type": "message", "role": "user", "content": bigFill},
		map[string]any{"type": "message", "role": "user", "content": "最新提問"},
	}
	total := 0
	for _, it := range items {
		total += itemApproxTokens(it)
	}
	if total < 150000 || total > 250000 {
		t.Fatalf("測試資料 token 估算 = %d，期望約 20 萬", total)
	}

	// 模擬 replay 的判斷：只有 total > limit 才裁剪。
	maybeTruncate := func(modelID string) ([]any, int) {
		limit := effectiveContextWindow(modelID, 320000)
		if total > limit {
			return truncateHistory(cloneAnySlice(items), limit), limit
		}
		return cloneAnySlice(items), limit
	}

	// mimo-2.6-flash：上限 140k，20 萬歷史超限 → 應裁剪並留標記。
	mimoOut, mimoLimit := maybeTruncate("mimo-2.6-flash")
	if mimoLimit != 140000 {
		t.Fatalf("mimo 上限 = %d，期望 140000", mimoLimit)
	}
	mimoBlob, _ := json.Marshal(mimoOut)
	if !strings.Contains(string(mimoBlob), "自動省略") {
		t.Fatalf("mimo 在 140k 上限應裁剪並留標記，實際未裁")
	}

	// glm：上限 320k，同一段 20 萬歷史不超限 → 不應裁剪。
	glmOut, glmLimit := maybeTruncate("glm-5.3-flash")
	if glmLimit != 320000 {
		t.Fatalf("glm 上限 = %d，期望 320000", glmLimit)
	}
	glmBlob, _ := json.Marshal(glmOut)
	if strings.Contains(string(glmBlob), "自動省略") {
		t.Fatalf("glm 在 320k 上限不應裁剪 20 萬歷史，卻被裁了")
	}
}

// 確保 deepai import 被使用（模型的 IsResponses 路徑在別的測試覆蓋）。
var _ = deepai.PathChat
