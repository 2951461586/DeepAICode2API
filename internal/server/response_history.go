package server

// Responses 伺服端對話歷史暫存 + 重放。
//
// 背景：宗恩透過反代（含 magpie 插件）使用 /v1/responses 時，同一視窗
// 對話到一半模型「失憶」。根因是用戶端攜帶 previous_response_id 續接，
// 但網關的 chat/completions 合成路徑是無狀態的，舊版直接回 400
// unsupported_parameter；客戶端收到 400 後靜默重開對話，上下文被截斷。
//
// 解法：網關自己記住每一輪的 input 與 output（按 response id 索引），
// 收到 previous_response_id 時沿鏈重組完整 input 再送上游，取代 400。
// 進程內存储，TTL 與上限可調；重啟後歷史清空（客戶端應重新帶完整 input）。

import (
	"encoding/json"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// historyEntry 是一輪已完成的 Responses 交互。
//
// input 是「本輪送出前的完整對話 input」（含之前所有輪次的 input 與 output），
// 因此重放是 O(1)：取上一輪的 input+output 接上本輪 input 即可，
// 不必每次都遍歷整條鏈。
type historyEntry struct {
	id           string
	input        []any // 完整對話 input（Responses item 格式）
	instructions string
	output       []any // 本輪助手輸出 item
	createdAt    time.Time
}

// responseTurn 是單輪請求送出前的輸入快照，回應完成後連同輸出寫入暫存。
type responseTurn struct {
	input        []any
	instructions string
}

// remember 把這輪的輸入快照與輸出寫入暫存。
func (h *responseHistory) remember(turn *responseTurn, respID string, output []any) {
	if h == nil || turn == nil || respID == "" {
		return
	}
	h.put(respID, &historyEntry{
		input:        cloneAnySlice(turn.input),
		instructions: turn.instructions,
		output:       cloneAnySlice(output),
	})
}

// responseHistory 是執行緒安全的對話歷史暫存。
type responseHistory struct {
	mu      sync.RWMutex
	entries map[string]*historyEntry
	order   []string // 插入順序，供容量淘汰
	ttl     time.Duration
	max     int
	now     func() time.Time // 測試可注入
}

// newResponseHistoryFromEnv 從環境變數構造暫存。
//
// DEEPAI2API_HISTORY_TTL_HOURS  歷史存活小時數（預設 24，0/負數用預設）。
// DEEPAI2API_HISTORY_MAX_ENTRIES 最多保留輪次（預設 500，0/負數用預設）。
func newResponseHistoryFromEnv() *responseHistory {
	ttlH := envPositiveInt("DEEPAI2API_HISTORY_TTL_HOURS", 24)
	max := envPositiveInt("DEEPAI2API_HISTORY_MAX_ENTRIES", 500)
	return newResponseHistory(time.Duration(ttlH)*time.Hour, max)
}

func newResponseHistory(ttl time.Duration, max int) *responseHistory {
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	if max <= 0 {
		max = 500
	}
	return &responseHistory{
		entries: map[string]*historyEntry{},
		ttl:     ttl,
		max:     max,
		now:     time.Now,
	}
}

func envPositiveInt(key string, def int) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return def
	}
	return n
}

// put 存入一輪交互；同 id 重複存入為覆蓋。
func (h *responseHistory) put(id string, e *historyEntry) {
	if h == nil || id == "" {
		return
	}
	e.id = id
	if e.createdAt.IsZero() {
		e.createdAt = h.now()
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.purgeExpiredLocked()
	if _, exists := h.entries[id]; !exists {
		h.order = append(h.order, id)
	}
	h.entries[id] = e
	for len(h.order) > h.max {
		oldest := h.order[0]
		h.order = h.order[1:]
		delete(h.entries, oldest)
	}
}

// get 取出一輪交互；過期視為不存在。
func (h *responseHistory) get(id string) *historyEntry {
	if h == nil || id == "" {
		return nil
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.purgeExpiredLocked()
	e := h.entries[id]
	if e == nil {
		return nil
	}
	cp := *e
	cp.input = cloneAnySlice(e.input)
	cp.output = cloneAnySlice(e.output)
	return &cp
}

// replayOutcome 是重放結果的語意分類。
type replayOutcome int

const (
	replayOK       replayOutcome = iota // 已重放（可能已自動裁減）
	replayNotFound                      // id 找不到 / 過期 → 400 unsupported_parameter
	replayTooLong                       // 超過 contextWindow 且未開 truncation:auto → 400
)

// replay 把 previous_response_id 指向的歷史重組進目前請求。
//
// 成功時：payload.input 被換成完整歷史、previous_response_id 被移除、
// 必要時補回最初的 instructions；回傳 replayOK。
// 找不到 / 已過期 → replayNotFound（呼叫方回 400 unsupported_parameter）。
//
// 若有效窗口 > 0 且重放後的鏈超過上限：
//   - payload["truncation"] == "auto"：裁掉最舊的中段輪次（保留開頭系統/首則訊息
//     與最近可容納範圍），照常送上游，回 replayOK。
//   - 否則：不更動 payload，回 replayTooLong 與明確訊息（呼叫方回 400），
//     绝不靜默砍歷史——那正是「失憶」的根源。
func (h *responseHistory) replay(payload map[string]any, prevID, modelID string, contextWindow int) (replayOutcome, string) {
	e := h.get(prevID)
	if e == nil {
		return replayNotFound, ""
	}
	merged := make([]any, 0, len(e.input)+len(e.output)+4)
	merged = append(merged, e.input...)
	merged = append(merged, e.output...)
	merged = append(merged, responsesInputArray(payload)...)

	// 有效窗口按模型實測值查表（見 effectiveContextWindow）；少數模型上限低於
	// 目錄標示（mimo-flash 140k、glyph 256k），其餘 9 個免費模型可吃滿 320k。
	// 讓網關先裁剪並留下標記，也不要讓上游偷偷砍歷史。
	limit := effectiveContextWindow(modelID, contextWindow)
	if limit > 0 {
		total := 0
		for _, it := range merged {
			total += itemApproxTokens(it)
		}
		if total > limit {
			if strOf(payload["truncation"]) != "auto" {
				return replayTooLong, "重放後的對話歷史約 " + itoaApprox(total) +
					" tokens，超過模型有效上下文 contextWindow " + itoaApprox(limit) +
					"（目錄標示 " + itoaApprox(contextWindow) +
					"）；請由客戶端壓縮歷史，或於請求設定 \"truncation\":\"auto\" 由網關自動裁減最舊中段。"
			}
			merged = truncateHistory(merged, limit)
		}
	}

	payload["input"] = merged
	if strOf(payload["instructions"]) == "" && e.instructions != "" {
		payload["instructions"] = e.instructions
	}
	delete(payload, "previous_response_id")
	return replayOK, ""
}

// truncateHistory 裁掉最舊的中段輪次：保留開頭（系統訊息 + 第一則使用者訊息，
// 通常是重要背景/暗號）與最近可容納的尾段；中間插入一則說明訊息。
func truncateHistory(items []any, contextWindow int) []any {
	if contextWindow <= 0 || len(items) <= 2 {
		return items
	}
	// head = 開頭連續的 system 訊息，再加上第一則非 system 訊息（開頭訊息）。
	headEnd := 0
	for headEnd < len(items) {
		m, ok := items[headEnd].(map[string]any)
		if !ok || strOf(m["role"]) != "system" {
			break
		}
		headEnd++
	}
	if headEnd < len(items) {
		headEnd++
	}
	used := 0
	for _, it := range items[:headEnd] {
		used += itemApproxTokens(it)
	}

	// tail = 由最新往回，尽可能容納。
	tailStart := len(items)
	for tailStart > headEnd {
		t := itemApproxTokens(items[tailStart-1])
		if used+t > contextWindow {
			break
		}
		used += t
		tailStart--
	}
	if tailStart >= len(items) {
		tailStart = len(items) - 1 // 至少保留最新一則
	}
	if tailStart < headEnd {
		tailStart = headEnd
	}

	// 標記必須用 user role：上游會拒絕「system 出現在非首則」的請求體
	// （實測 Chat Completions：system 在中間 → 400 This request cannot be accepted）。
	out := make([]any, 0, headEnd+(len(items)-tailStart)+1)
	out = append(out, items[:headEnd]...)
	out = append(out, map[string]any{
		"type": "message", "role": "user",
		"content": "（中間的對話歷史因超過上下文長度已由網關自動省略）",
	})
	out = append(out, items[tailStart:]...)
	return out
}

// perModelContextWindow 是實測得出的「例外」上限：少數模型的可靠窗口
// 低於目錄標示值，不能跟其他模型一樣一律用 320k。
//
// 2026-11-20 11 個免費模型實測（純透傳 + 唯一隨機 ASCII 階梯，附多輪對照）：
//   - mimo-2.6-flash：140k 完整，170k 起上游「靜默截斷」（prompt_tokens 掉到
//     5,296 ≈ 3%）。真正上限落在 140k–170k，保守取 140000，避免上游偷砍歷史。
//   - glyph-cluster：260k 完整，320k 硬拒絕（400 provider rejected）；目錄標
//     256k 與實測一致，取 256000。
//   - 其餘 9 個免費模型：320k 完整（glm/deepseek/qwen/minimax/muse/solar/
//     gpt-6-luna/mimo-pro/deepai-cyber），等於目錄標示值，不再人為砍到 120k。
//
// 未列在此表的模型（含付費模型）走 min(標示值, catalogContextCap)。
var perModelContextWindow = map[string]int{
	"mimo-2.6-flash": 140000,
	"glyph-cluster":  256000,
}

// catalogContextCap 是目錄標示值的通用上限；目前所有模型標示值皆 ≤ 320k。
const catalogContextCap = 320000

// effectiveContextWindow 回傳重放時真正可用的上下文上限（按模型）。
//
// 優先順序（高 → 低）：
//  1. DEEPAI2API_EFFECTIVE_CONTEXT_WINDOW：全域覆寫（0/負數 = 不設上限）。
//     保留舊行為，作為排查/臨時降上限用。
//  2. DEEPAI2API_CONTEXT_WINDOWS：JSON 物件，按 model id 覆寫，例如
//     {"mimo-2.6-flash":140000,"glm-5.3-flash":320000}
//     （值 0/負數 = 該模型不設上限；格式錯誤則忽略整項）。
//  3. perModelContextWindow：上面的實測例外表。
//  4. min(標示值, catalogContextCap)；標示值 ≤ 0 時用 catalogContextCap。
func effectiveContextWindow(modelID string, catalog int) int {
	if raw := strings.TrimSpace(os.Getenv("DEEPAI2API_EFFECTIVE_CONTEXT_WINDOW")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil {
			if n <= 0 {
				return 0
			}
			return n
		}
	}
	if raw := strings.TrimSpace(os.Getenv("DEEPAI2API_CONTEXT_WINDOWS")); raw != "" {
		var overrides map[string]int
		if err := json.Unmarshal([]byte(raw), &overrides); err == nil {
			if n, ok := overrides[modelID]; ok {
				if n <= 0 {
					return 0
				}
				return n
			}
		}
	}
	if n, ok := perModelContextWindow[modelID]; ok {
		return n
	}
	if catalog <= 0 || catalog > catalogContextCap {
		return catalogContextCap
	}
	return catalog
}

// itemApproxTokens 粗估 item 的 token 數。中文等全形字元約 1 token/字，
// ASCII 約 1.6 bytes/token（實測 160k 英文隨機字串 → 100,761 input_tokens，
// 比值 1.59；舊的 len/3 對程式碼場景低估約一倍，會讓自動壓縮太晚觸發）。
// 只在上限判斷時呼叫，正常情況零額外開銷。
func itemApproxTokens(item any) int {
	raw, err := json.Marshal(item)
	if err != nil {
		return 0
	}
	tokens := 0
	asciiRun := 0
	flush := func() {
		if asciiRun > 0 {
			tokens += (asciiRun*10 + 15) / 16 // ceil(run / 1.6)
			asciiRun = 0
		}
	}
	for _, r := range string(raw) {
		if r < 0x80 {
			asciiRun++
			continue
		}
		flush()
		tokens++ // 一個非 ASCII 字元約 1 token（中文、emoji 等同量級）
	}
	flush()
	return tokens
}

func itoaApprox(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

func (h *responseHistory) purgeExpiredLocked() {
	now := h.now()
	kept := h.order[:0]
	for _, id := range h.order {
		e := h.entries[id]
		if e == nil || now.Sub(e.createdAt) > h.ttl {
			delete(h.entries, id)
			continue
		}
		kept = append(kept, id)
	}
	h.order = kept
}

// len 供測試與監控使用。
func (h *responseHistory) len() int {
	if h == nil {
		return 0
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.entries)
}

// --- 工具函式 ---

// responsesInputArray 把 payload.input 正規化成 Responses item 切片（深拷貝）。
// 字串 input 轉成單一 user message item，與 gateway 的轉換保持一致。
func responsesInputArray(payload map[string]any) []any {
	switch v := payload["input"].(type) {
	case []any:
		return cloneAnySlice(v)
	case string:
		return []any{map[string]any{"type": "message", "role": "user", "content": v}}
	}
	return nil
}

// cloneAnySlice 用 JSON 往返做深拷貝；input/output item 都是純 JSON 值。
func cloneAnySlice(in []any) []any {
	if in == nil {
		return nil
	}
	raw, err := json.Marshal(in)
	if err != nil {
		return append([]any(nil), in...)
	}
	var out []any
	if err := json.Unmarshal(raw, &out); err != nil {
		return append([]any(nil), in...)
	}
	return out
}
