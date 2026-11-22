package respstate

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"deepaicode2api/internal/jsonx"
)

// msgItem 造一則標準 Responses message item（測試輔助）。
func msgItem(role, content string) map[string]any {
	return map[string]any{"type": "message", "role": role, "content": content}
}

// itemCostsOf 算一組 item 的 token 估算（測試輔助）。
func itemCostsOf(items []any) []int {
	costs := make([]int, len(items))
	for i, it := range items {
		costs[i] = ItemApproxTokens(it)
	}
	return costs
}

// hasItemText 判斷某段文字是否出現在任一 item 的 JSON 裡（測試輔助）。
func hasItemText(items []any, want string) bool {
	for _, it := range items {
		raw, err := json.Marshal(it)
		if err == nil && strings.Contains(string(raw), want) {
			return true
		}
	}
	return false
}

// hasEntry 判断条目是否还在（顺带触发 TTL 清理，等价于旧的 get 语义）。
func hasEntry(h *Store, id string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.purgeExpiredLocked()
	return h.entries[id] != nil
}

// historyDrop 模擬容量淘汰：直接把條目從暫存裡摘掉。
func historyDrop(h *Store, ids ...string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, id := range ids {
		delete(h.entries, id)
	}
	kept := h.order[:0]
	for _, id := range h.order {
		if _, ok := h.entries[id]; ok {
			kept = append(kept, id)
		}
	}
	h.order = kept
}

// TestHistoryOverlap 鎖住「in 前綴與 base 後綴重疊」的判定：
// 完整重送、部分重送、新回合、以及「文字重複但不是重送」。
func TestHistoryOverlap(t *testing.T) {
	base := []any{
		msgItem("user", "q1"), msgItem("assistant", "r1"),
		msgItem("user", "q2"), msgItem("assistant", "r2"),
	}
	cases := []struct {
		name string
		in   []any
		want int
	}{
		{"新回合", []any{msgItem("user", "q3")}, 0},
		{"完整重送", []any{
			msgItem("user", "q1"), msgItem("assistant", "r1"),
			msgItem("user", "q2"), msgItem("assistant", "r2"),
			msgItem("user", "q3"),
		}, 4},
		{"部分重送（最後一輪）", []any{
			msgItem("user", "q2"), msgItem("assistant", "r2"), msgItem("user", "q3"),
		}, 2},
		{"只重送最後一則", []any{msgItem("assistant", "r2"), msgItem("user", "q3")}, 1},
		// 重複同一句提問但 base 末項是助手輸出：不能當成重送，
		// 否則會把上一輪的回答誤刪。
		{"重複文字但不是重送", []any{msgItem("user", "q1")}, 0},
		{"空輸入", nil, 0},
	}
	for _, c := range cases {
		if got := historyOverlap(c.in, base); got != c.want {
			t.Fatalf("%s: historyOverlap = %d，期望 %d", c.name, got, c.want)
		}
	}
	if got := historyOverlap(base, nil); got != 0 {
		t.Fatalf("base 為空時應為 0，實際 %d", got)
	}
}

// TestHistoryReplayDedupesResentHistory 驗證重放會按重疊去重：
// 完整重送、部分重送都不會把同一段歷史拼兩次。
func TestHistoryReplayDedupesResentHistory(t *testing.T) {
	h := New(time.Hour, 10)
	h.Remember(&Turn{Input: []any{msgItem("user", "q1")}}, "resp_0",
		[]any{msgItem("assistant", "r1")})
	h.Remember(&Turn{Parent: "resp_0", Input: []any{msgItem("user", "q2")}}, "resp_1",
		[]any{msgItem("assistant", "r2")})

	turns := []struct {
		name string
		in   []any
		want int
	}{
		{"新回合", []any{msgItem("user", "q3")}, 5},
		{"部分重送", []any{
			msgItem("user", "q2"), msgItem("assistant", "r2"), msgItem("user", "q3"),
		}, 5},
		{"完整重送", []any{
			msgItem("user", "q1"), msgItem("assistant", "r1"),
			msgItem("user", "q2"), msgItem("assistant", "r2"),
			msgItem("user", "q3"),
		}, 5},
	}
	for _, c := range turns {
		payload := map[string]any{"input": c.in}
		outcome, msg := h.Replay(payload, "resp_1", "glm-5.3-flash", 320000)
		if outcome != OK {
			t.Fatalf("%s: Replay = %v（%s）", c.name, outcome, msg)
		}
		items, _ := payload["input"].([]any)
		if len(items) != c.want {
			t.Fatalf("%s: 重放後 item 數 = %d，期望 %d（重疊被拼了兩次）", c.name, len(items), c.want)
		}
		if !hasItemText(items, "q1") || !hasItemText(items, "q3") {
			t.Fatalf("%s: 應同時保留開頭與本輪輸入：%v", c.name, items)
		}
		if _, ok := payload["previous_response_id"]; ok {
			t.Fatalf("%s: 重放後應移除 previous_response_id", c.name)
		}
	}
}

// TestTruncateHistoryRejectsOversizedItems 驗證單則超大訊息裁不掉時，
// 裁剪結果仍超限（由呼叫方回明確錯誤）。
func TestTruncateHistoryRejectsOversizedItems(t *testing.T) {
	items := []any{
		msgItem("user", strings.Repeat("x", 500)),
		msgItem("assistant", "ok"),
		msgItem("user", "latest"),
	}
	got := truncateHistoryWithCosts(items, itemCostsOf(items), 100)
	total := 0
	for _, item := range got {
		total += ItemApproxTokens(item)
	}
	if total <= 100 {
		t.Fatalf("期望单条消息仍超限以便 replay 返回错误，估算=%d", total)
	}
}

// TestTruncateHistoryDropsOrphanToolOutputs 驗證裁剪點落在工具調用配對中間時，
// 不會留下孤立的 function_call_output：gateway 會把 output 翻成 role=tool 訊息，
// 而上游要求 tool 訊息緊跟帶 tool_calls 的 assistant，否則整輪 400。
func TestTruncateHistoryDropsOrphanToolOutputs(t *testing.T) {
	filler := strings.Repeat("x", 2000)

	// 情況一：function_call 落在被裁掉的中段，它的 output 落在尾段 → output 必須被丟棄。
	orphaned := []any{
		msgItem("system", "sys"),
		msgItem("user", "開頭暗號 ZEBRA42"),
		map[string]any{"type": "function_call", "call_id": "c0", "name": "lookup", "arguments": "{}"},
		msgItem("user", filler),
		map[string]any{"type": "function_call_output", "call_id": "c0", "output": "結果"},
		msgItem("user", "最新提問"),
	}
	costs := itemCostsOf(orphaned)
	// 只容納 head + 尾段兩則（output 與最新提問），call 留在中段被裁掉。
	budget := costs[0] + costs[1] + costs[4] + costs[5] + 60
	out := truncateHistoryWithCosts(orphaned, costs, budget)

	if !hasItemText(out, "最新提問") {
		t.Fatalf("尾段最新提問應保留：%#v", out)
	}
	for _, it := range out {
		m, _ := it.(map[string]any)
		if jsonx.StrOf(m["type"]) == "function_call_output" {
			t.Fatalf("配對的 function_call 已被裁掉，仍留下孤立 output（上游會 400）：%#v", out)
		}
	}

	// 情況二：配對兩則都落在尾段 → 必須一起保留，不能誤刪。
	paired := []any{
		msgItem("system", "sys"),
		msgItem("user", "開頭"),
		msgItem("user", filler),
		map[string]any{"type": "function_call", "call_id": "c1", "name": "lookup", "arguments": "{}"},
		map[string]any{"type": "function_call_output", "call_id": "c1", "output": "結果"},
		msgItem("user", "最新"),
	}
	pcosts := itemCostsOf(paired)
	pbudget := pcosts[0] + pcosts[1] + pcosts[3] + pcosts[4] + pcosts[5] + 60
	pout := truncateHistoryWithCosts(paired, pcosts, pbudget)

	var calls, outputs int
	for _, it := range pout {
		m, _ := it.(map[string]any)
		switch jsonx.StrOf(m["type"]) {
		case "function_call":
			calls++
		case "function_call_output":
			outputs++
		}
	}
	if calls != 1 || outputs != 1 {
		t.Fatalf("成對的工具調用應完整保留，實際 call=%d output=%d：%#v", calls, outputs, pout)
	}
}

// TestTruncateHistoryMarkerFitsBudget 驗證插入的說明訊息本身也計入預算。
// 舊行為只在尾段選取時用滿額預算，裁剪結果再加說明訊息就超限，
// replay 於是把「本來壓得下去」的請求回成 400。
func TestTruncateHistoryMarkerFitsBudget(t *testing.T) {
	items := []any{msgItem("system", "sys"), msgItem("user", "開頭")}
	for i := 0; i < 5; i++ {
		items = append(items, msgItem("user", strings.Repeat("y", 400)))
	}
	costs := itemCostsOf(items)
	budget := costs[0] + costs[1] + 2*costs[2] // 留兩則尾段就會超限的預算
	out := truncateHistoryWithCosts(items, costs, budget)

	total := 0
	for _, it := range out {
		total += ItemApproxTokens(it)
	}
	if total > budget {
		t.Fatalf("裁剪結果 %d tokens 超過預算 %d（說明訊息未計入）：%#v", total, budget, out)
	}
}

// TestItemApproxTokensCalibration 驗證估算與上游 input_tokens 偏差在 ±30% 內。
// 壓力測試實測：160k ASCII bytes → 上游 input_tokens=100,761（比值 1.59）。
func TestItemApproxTokensCalibration(t *testing.T) {
	ascii := strings.Repeat("a", 160000)
	got := ItemApproxTokens(ascii)
	want := 100761 // 上游實測值
	if diff := float64(got-want) / float64(want); diff > 0.30 || diff < -0.30 {
		t.Fatalf("ASCII token 估算 = %d，上游實測 %d，偏差 %.1f%% 超出 ±30%%", got, want, diff*100)
	}
	// 中文：一個字元約 1 token。
	zh := ItemApproxTokens(strings.Repeat("中", 1000))
	if zh < 800 || zh > 1200 {
		t.Fatalf("中文 token 估算 = %d，期望約 1000", zh)
	}
}

// TestEffectiveContextWindow 驗證按模型查表的有效窗口。
func TestEffectiveContextWindow(t *testing.T) {
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
		if got := EffectiveContextWindow(id, 320000); got != exp {
			t.Fatalf("%s 有效窗口 = %d，期望 %d", id, got, exp)
		}
	}
	// 未知/付費模型：min(標示, 320k)，不再硬壓 120k。
	if got := EffectiveContextWindow("some-paid-model", 320000); got != 320000 {
		t.Fatalf("未知模型 320k 標示應為 320000，實際 %d", got)
	}
	if got := EffectiveContextWindow("some-paid-model", 200); got != 200 {
		t.Fatalf("小於上限的標示應原樣保留，實際 %d", got)
	}

	// 全域環境覆寫（舊行為保留）。
	t.Setenv("DEEPAI2API_EFFECTIVE_CONTEXT_WINDOW", "64000")
	if got := EffectiveContextWindow("glm-5.3-flash", 320000); got != 64000 {
		t.Fatalf("全域環境覆寫應為 64000，實際 %d", got)
	}
	t.Setenv("DEEPAI2API_EFFECTIVE_CONTEXT_WINDOW", "0")
	if got := EffectiveContextWindow("glm-5.3-flash", 320000); got != 0 {
		t.Fatalf("全域設 0 應關閉上限，實際 %d", got)
	}
}

// TestEffectiveContextWindowPerModelOverride 驗證 DEEPAI2API_CONTEXT_WINDOWS
// 按模型 JSON 覆寫：只改指定模型，不影響其他模型；全域覆寫優先於按模型。
func TestEffectiveContextWindowPerModelOverride(t *testing.T) {
	t.Setenv("DEEPAI2API_EFFECTIVE_CONTEXT_WINDOW", "")
	t.Setenv("DEEPAI2API_CONTEXT_WINDOWS", `{"glm-5.3-flash":64000,"mimo-2.6-flash":0}`)
	if got := EffectiveContextWindow("glm-5.3-flash", 320000); got != 64000 {
		t.Fatalf("按模型覆寫 glm 應為 64000，實際 %d", got)
	}
	if got := EffectiveContextWindow("mimo-2.6-flash", 320000); got != 0 {
		t.Fatalf("按模型覆寫 mimo 設 0 應關閉上限，實際 %d", got)
	}
	// 未列入 JSON 的模型仍走表格/標示值。
	if got := EffectiveContextWindow("glyph-cluster", 320000); got != 256000 {
		t.Fatalf("未覆寫的 glyph 應為 256000，實際 %d", got)
	}
	// 全域覆寫優先於按模型。
	t.Setenv("DEEPAI2API_EFFECTIVE_CONTEXT_WINDOW", "100000")
	if got := EffectiveContextWindow("glm-5.3-flash", 320000); got != 100000 {
		t.Fatalf("全域覆寫應優先，實際 %d", got)
	}
	// 格式錯誤的 JSON 應忽略。
	t.Setenv("DEEPAI2API_EFFECTIVE_CONTEXT_WINDOW", "")
	t.Setenv("DEEPAI2API_CONTEXT_WINDOWS", `not-json`)
	if got := EffectiveContextWindow("glm-5.3-flash", 320000); got != 320000 {
		t.Fatalf("錯誤 JSON 應忽略，實際 %d", got)
	}
}

// TestTruncationPointFollowsModelWindow 驗證 truncation:auto 的裁剪點跟著
// 各模型上限走：同一段約 20 萬 token 的歷史，mimo-flash（140k）會被裁，
// glm（320k）不會被裁——證明不再一律在 12 萬就提前裁剪。
func TestTruncationPointFollowsModelWindow(t *testing.T) {
	// 用 ItemApproxTokens 反推：產生約 200000 tokens 的 ASCII（每 token ~1.6 bytes）。
	bigFill := strings.Repeat("a", 200000*8/5) // 320000 bytes → 約 200000 tokens
	items := []any{
		msgItem("user", "開頭暗號 ZEBRA42"),
		msgItem("user", bigFill),
		msgItem("user", "最新提問"),
	}
	total := 0
	for _, it := range items {
		total += ItemApproxTokens(it)
	}
	if total < 150000 || total > 250000 {
		t.Fatalf("測試資料 token 估算 = %d，期望約 20 萬", total)
	}

	// 模擬 replay 的判斷：只有 total > limit 才裁剪。
	maybeTruncate := func(modelID string) ([]any, int) {
		limit := EffectiveContextWindow(modelID, 320000)
		if total > limit {
			return truncateHistoryWithCosts(cloneAnySlice(items), itemCostsOf(items), limit), limit
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

// TestContextWindowOverridesCached 驗證窗口覆寫的 JSON 只解析一次，
// 且環境變數一變就失效（t.Setenv 在測試裡改值必須立刻生效）。
func TestContextWindowOverridesCached(t *testing.T) {
	ptr := func(m map[string]int) uintptr { return uintptr(reflect.ValueOf(m).UnsafePointer()) }

	t.Setenv("DEEPAI2API_CONTEXT_WINDOWS", `{"a":1,"b":2}`)
	first := contextWindowOverrides()
	if first["a"] != 1 || first["b"] != 2 {
		t.Fatalf("解析結果錯誤：%v", first)
	}
	if ptr(contextWindowOverrides()) != ptr(first) {
		t.Fatal("同一份環境變數應命中緩存，不應每次重新解析")
	}

	t.Setenv("DEEPAI2API_CONTEXT_WINDOWS", `{"a":3}`)
	next := contextWindowOverrides()
	if next["a"] != 3 {
		t.Fatalf("環境變數變更後應重新解析，實際 %v", next)
	}
	if ptr(next) == ptr(first) {
		t.Fatal("環境變數變更後應換成新的解析結果")
	}

	t.Setenv("DEEPAI2API_CONTEXT_WINDOWS", `not-json`)
	if got := contextWindowOverrides(); got != nil {
		t.Fatalf("格式錯誤應忽略整項（nil），實際 %v", got)
	}
}

// TestHistoryByteLimit 验证单条过大时不写入，且不会挤掉已有历史。
func TestHistoryByteLimit(t *testing.T) {
	h := New(time.Hour, 10)
	h.maxBytes = 128
	h.put("small", &historyEntry{input: []any{"ok"}})
	h.put("large", &historyEntry{input: []any{strings.Repeat("x", 256)}})
	if hasEntry(h, "large") || !hasEntry(h, "small") {
		t.Fatal("应拒绝过大的单条历史，并保留已有小条目")
	}
	if h.usedBytes > h.maxBytes {
		t.Fatalf("缓存占用 %d 超过上限 %d", h.usedBytes, h.maxBytes)
	}
}

// TestHistoryTTLAndMax 驗證 TTL 與容量上限。
func TestHistoryTTLAndMax(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	h := New(time.Hour, 2)
	h.now = func() time.Time { return base }

	h.put("a", &historyEntry{input: []any{"a"}})
	h.now = func() time.Time { return base.Add(30 * time.Minute) }
	h.put("b", &historyEntry{input: []any{"b"}})
	h.now = func() time.Time { return base.Add(90 * time.Minute) } // a 已過期
	h.put("c", &historyEntry{input: []any{"c"}})
	if hasEntry(h, "a") {
		t.Fatal("過期條目 a 應被清除")
	}
	if !hasEntry(h, "b") || !hasEntry(h, "c") {
		t.Fatal("未過期條目應保留")
	}

	// 容量上限：再加兩筆後只留最新 2 筆。
	h.now = func() time.Time { return base.Add(95 * time.Minute) }
	h.put("d", &historyEntry{input: []any{"d"}})
	h.put("e", &historyEntry{input: []any{"e"}})
	if h.Len() != 2 {
		t.Fatalf("容量上限後條目 = %d，期望 2", h.Len())
	}
	if hasEntry(h, "b") || hasEntry(h, "c") {
		t.Fatal("最舊的 b/c 應被淘汰")
	}
	if !hasEntry(h, "d") || !hasEntry(h, "e") {
		t.Fatal("最新的 d/e 應保留")
	}
}

// TestHistoryChainCompact 驗證暫存只存增量而不是每輪存整條歷史：
// 5 輪各帶 2000 字元 filler 時，全量存會佔約 30k，增量存約 10k。
func TestHistoryChainCompact(t *testing.T) {
	h := New(time.Hour, 100)
	filler := strings.Repeat("f", 2000)
	prev := ""
	for i := 0; i < 5; i++ {
		id := "resp_" + itoaApprox(i)
		h.Remember(&Turn{
			Parent: prev,
			Input:  []any{msgItem("user", filler)},
		}, id, []any{msgItem("assistant", "ok")})
		prev = id
	}
	if h.Len() != 5 {
		t.Fatalf("暫存條目 = %d，期望 5", h.Len())
	}
	// 全量存：1+2+3+4+5 = 15 份 filler ≈ 30k 位元組；增量存 5 份 ≈ 10k。
	if used := h.usedBytes; used > 16000 {
		t.Fatalf("暫存佔用 %d bytes，增量存儲應遠低於全量的約 30k（增量存、非全量）", used)
	}
}

// TestHistoryCheckpointSurvivesEviction 驗證檢查點的作用：
// 父條目被淘汰後，只要最近一份快照還在，續接仍可還原完整歷史。
func TestHistoryCheckpointSurvivesEviction(t *testing.T) {
	h := New(time.Hour, 100)
	h.checkpointEvery = 2 // 5 輪內造出兩份檢查點

	prev := ""
	ids := make([]string, 0, 5)
	for i := 0; i < 5; i++ {
		id := "resp_" + itoaApprox(i)
		text := "turn " + itoaApprox(i)
		if i == 0 {
			text = "Remember codeword ZEBRA42"
		}
		h.Remember(&Turn{Parent: prev, Input: []any{msgItem("user", text)}}, id,
			[]any{msgItem("assistant", "ok")})
		prev = id
		ids = append(ids, id)
	}

	// 淘汰最舊三條（含深度 0 的根）：最後一條的檢查點應讓它仍可續接。
	historyDrop(h, ids[:3]...)

	payload := map[string]any{"input": "What is the codeword?"}
	outcome, msg := h.Replay(payload, ids[4], "glm-5.3-flash", 320000)
	if outcome != OK {
		t.Fatalf("有檢查點時應可續接，實際 outcome=%v（%s）", outcome, msg)
	}
	if !hasItemText(payload["input"].([]any), "ZEBRA42") {
		t.Fatalf("續接後應保留開頭暗號：%v", payload["input"])
	}
}

// TestHistoryBrokenChainIsExplicit 驗證鏈斷（父條目被淘汰且無檢查點）時回
// NotFound，而不是送半截歷史上去（那就是靜默失憶）。
func TestHistoryBrokenChainIsExplicit(t *testing.T) {
	h := New(time.Hour, 100) // 預設間隔 8：3 輪鏈沒有檢查點
	prev := ""
	ids := make([]string, 0, 3)
	for i := 0; i < 3; i++ {
		id := "resp_" + itoaApprox(i)
		h.Remember(&Turn{Parent: prev, Input: []any{msgItem("user", "turn "+itoaApprox(i))}}, id,
			[]any{msgItem("assistant", "ok")})
		prev = id
		ids = append(ids, id)
	}

	historyDrop(h, ids[0])
	payload := map[string]any{"input": "What is the codeword?"}
	if outcome, _ := h.Replay(payload, ids[2], "glm-5.3-flash", 320000); outcome != NotFound {
		t.Fatalf("鏈斷應回 NotFound，實際 %v", outcome)
	}
	if _, ok := payload["input"].(string); !ok {
		t.Fatalf("NotFound 時不應改寫 payload：%v", payload["input"])
	}
}

// TestReplayBodyBytesCeiling 驗證 issue #7 的字節閘門：
// 上游在請求體約 196 KB 處靜默砍掉開頭，純英文下 token 估算（1.6 B/token）
// 要到 320k 才會觸發，攔不住；所以 replay 必須按字節提前拒絕 / 裁剪。
func TestReplayBodyBytesCeiling(t *testing.T) {
	newChain := func(t *testing.T) (*Store, string) {
		t.Helper()
		h := New(time.Hour, 10)
		h.Remember(&Turn{Input: []any{msgItem("user", "開頭暗號 ZEBRA42")}}, "resp_0",
			[]any{msgItem("assistant", "ok")})
		return h, "resp_0"
	}
	// 250 KB ASCII：token 估算約 156k（遠低於 320k 標示值），字節已超 196,608。
	big := strings.Repeat("a", 250000)

	t.Run("默认拒绝并说明字节上限", func(t *testing.T) {
		h, prev := newChain(t)
		payload := map[string]any{"input": []any{msgItem("user", big)}}
		outcome, msg := h.Replay(payload, prev, "glm-5.3-flash", 320000)
		if outcome != TooLong {
			t.Fatalf("250 KB 請求體應回 TooLong，實際 %v", outcome)
		}
		if !strings.Contains(msg, "位元組") || !strings.Contains(msg, "issue #7") {
			t.Fatalf("錯誤訊息應說明字節閘門與 issue #7，實際：%s", msg)
		}
		if !strings.Contains(msg, "contextWindow") {
			t.Fatalf("錯誤訊息仍應帶 contextWindow 供客戶端定位：%s", msg)
		}
	})

	t.Run("truncation auto 会裁掉超长中段", func(t *testing.T) {
		// 60 輪、每輪 5 KB：總請求體 ~300 KB，但歷史可分段丟棄，
		// 與「單則訊息本身就超限」不同（後者只能回 TooLong）。
		h := New(time.Hour, 100)
		h.Remember(&Turn{Input: []any{msgItem("user", "開頭暗號 ZEBRA42")}}, "resp_0",
			[]any{msgItem("assistant", "ok")})
		prev := "resp_0"
		chunk := strings.Repeat("b", 5000)
		for i := range 60 {
			id := "resp_" + itoaApprox(i+1)
			h.Remember(&Turn{Parent: prev, Input: []any{msgItem("user", chunk)}}, id,
				[]any{msgItem("assistant", "ok")})
			prev = id
		}
		payload := map[string]any{
			"input":      []any{msgItem("user", "最新提問")},
			"truncation": "auto",
		}
		outcome, msg := h.Replay(payload, prev, "glm-5.3-flash", 320000)
		if outcome != OK {
			t.Fatalf("truncation:auto 應成功，實際 %v（%s）", outcome, msg)
		}
		items, _ := payload["input"].([]any)
		if !hasItemText(items, "ZEBRA42") {
			t.Fatalf("裁剪後仍應保留開頭：%v", items)
		}
		if !hasItemText(items, "自動省略") {
			t.Fatalf("裁剪後應插入省略標記：%v", items)
		}
		raw, err := json.Marshal(payload["input"])
		if err != nil {
			t.Fatalf("序列化裁剪結果失敗: %v", err)
		}
		if limit := ContextBodyBytes(); limit > 0 && len(raw) > limit {
			t.Fatalf("裁剪後 input 仍有 %d 位元組，超過上限 %d", len(raw), limit)
		}
	})

	t.Run("环境变量可关闭字节闸门", func(t *testing.T) {
		t.Setenv("DEEPAI2API_CONTEXT_BODY_BYTES", "0")
		h, prev := newChain(t)
		payload := map[string]any{"input": []any{msgItem("user", big)}}
		if outcome, msg := h.Replay(payload, prev, "glm-5.3-flash", 320000); outcome != OK {
			t.Fatalf("關閉字節閘門後應放行（token 未超限），實際 %v（%s）", outcome, msg)
		}
	})
}
