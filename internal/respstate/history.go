package respstate

// Package respstate 是 /v1/responses 的伺服端對話歷史暫存與重放。
//
// 背景：宗恩透過反代（含 magpie 插件）使用 /v1/responses 時，同一視窗
// 對話到一半模型「失憶」。根因是用戶端攜帶 previous_response_id 續接，
// 但網關的 chat/completions 合成路徑是無狀態的，舊版直接回 400
// unsupported_parameter；客戶端收到 400 後靜默重開對話，上下文被截斷。
//
// 解法：網關自己記住每一輪的增量 input 與 output（按 response id 索引，另帶
// parent 鏈），收到 previous_response_id 時沿鏈重組完整 input 再送上游，取代 400。
// 進程內存储，TTL 與上限可調；重啟後歷史清空（客戶端應重新帶完整 input）。
//
// 記憶體：舊版每輪都存整條歷史（O(輪數²)）。現在只存本輪增量，每
// historyCheckpointEvery 輪存一份完整快照作為檢查點：記憶體降到 O(總量)
// 的最壞 O(h²/interval)，鏈上父條目被淘汰時也能從最近的檢查點還原。
//
// 本包不依賴 internal/server：只用到 internal/jsonx 的取值工具，因此可以
// 脫離 HTTP 層單獨測試。

import (
	"encoding/json"
	"os"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	"deepaicode2api/internal/jsonx"
)

// historyCheckpointEvery 是檢查點間隔（輪）。
//
// 太小則快照占的記憶體接近舊版全量存；太大則鏈走訪變長、父條目一被淘汰
// 就整個斷鏈。8 讓「最壞走訪 8 步」與「額外記憶體 ≈ 1/8 全量」同時成立。
const historyCheckpointEvery = 8

// historyEntry 是一輪已完成的 Responses 交互。
//
// input / output 只存「本輪增量」（本輪新增的 item 與本輪助手輸出），
// 之前的歷史由 parent 鏈還原；snapshot 是檢查點（本輪之前的完整 item 序列）。
type historyEntry struct {
	id           string
	parent       string // 上一輪的 response id；空表示這輪是鏈的起點
	input        []any  // 本輪新增的 input（Responses item 格式）
	instructions string
	output       []any // 本輪助手輸出 item
	createdAt    time.Time
	depth        int   // 鏈深（根為 0），決定是否造檢查點
	snapshot     []any // 檢查點：本輪之前的完整 item 序列（可能為 nil）
	bytes        int64 // 條目體積估算，供容量水位用（避免每次重新量）
}

// Turn 是單輪請求送出前的輸入快照，回應完成後連同輸出寫入暫存。
//
// Input 必須是「本輪新增」的項目：呼叫方（handleResponses）在重放改寫
// payload 之前就要把它取下來，否則存進去的是重放後的整條歷史（回到 O(輪數²)）。
type Turn struct {
	Input        []any
	Instructions string
	Parent       string // 請求裡的 previous_response_id（續接時）
}

// remember 把這輪的增量輸入與輸出寫入暫存。
//
// turn.Input 已經是本輪新增項目的獨立副本（InputItems 內含深拷貝），
// 這裡直接收下所有權，不再重覆拷貝；output 來自回應物件，仍拷一份隔開。
func (h *Store) Remember(turn *Turn, respID string, output []any) {
	if h == nil || turn == nil || respID == "" {
		return
	}
	h.put(respID, &historyEntry{
		parent:       turn.Parent,
		input:        turn.Input,
		instructions: turn.Instructions,
		output:       cloneAnySlice(output),
	})
}

// Store 是執行緒安全的對話歷史暫存。
type Store struct {
	mu        sync.RWMutex
	entries   map[string]*historyEntry
	order     []string // 插入順序，供容量淘汰
	ttl       time.Duration
	max       int
	maxBytes  int64
	usedBytes int64
	now       func() time.Time // 測試可注入

	// checkpointEvery 是檢查點間隔（0 = 不造檢查點，僅測試用）。
	checkpointEvery int
}

// NewFromEnv 從環境變數構造暫存。
//
// DEEPAI2API_HISTORY_TTL_HOURS  歷史存活小時數（預設 24，0/負數用預設）。
// DEEPAI2API_HISTORY_MAX_ENTRIES 最多保留輪次（預設 500，0/負數用預設）。
func NewFromEnv() *Store {
	ttlH := envPositiveInt("DEEPAI2API_HISTORY_TTL_HOURS", 24)
	max := envPositiveInt("DEEPAI2API_HISTORY_MAX_ENTRIES", 500)
	return New(time.Duration(ttlH)*time.Hour, max)
}

func New(ttl time.Duration, max int) *Store {
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	if max <= 0 {
		max = 500
	}
	return &Store{
		entries:         map[string]*historyEntry{},
		ttl:             ttl,
		max:             max,
		maxBytes:        64 << 20,
		now:             time.Now,
		checkpointEvery: historyCheckpointEvery,
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
func (h *Store) put(id string, e *historyEntry) {
	if h == nil || id == "" || e == nil {
		return
	}
	e.id = id
	if e.createdAt.IsZero() {
		e.createdAt = h.now()
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.purgeExpiredLocked()

	// 鏈深與檢查點都要看父條目，所以只能在鎖內算。
	// 父條目不在（淘汰/TTL/測試直接寫入）時按根處理。
	if parent := h.entries[e.parent]; parent != nil {
		e.depth = parent.depth + 1
		if e.snapshot == nil && h.checkpointEvery > 0 && e.depth%h.checkpointEvery == 0 {
			e.snapshot = h.itemsBefore(e)
		}
	}
	e.bytes = historyEntryBytes(e)
	if e.bytes > h.maxBytes {
		return // 單條過大：不寫入（不會把已有條目擠掉）
	}
	if old := h.entries[id]; old != nil {
		h.usedBytes -= old.bytes
	} else {
		h.order = append(h.order, id)
	}
	h.entries[id] = e
	h.usedBytes += e.bytes
	for len(h.order) > h.max || h.usedBytes > h.maxBytes {
		oldest := h.order[0]
		h.order = h.order[1:]
		if old := h.entries[oldest]; old != nil {
			h.usedBytes -= old.bytes
			delete(h.entries, oldest)
		}
	}
}

// replayOutcome 是重放結果的語意分類。
type replayOutcome int

const (
	OK       replayOutcome = iota // 已重放（可能已自動裁減）
	NotFound                      // id 找不到 / 過期 → 400 unsupported_parameter
	TooLong                       // 超過 contextWindow 且未開 truncation:auto → 400
)

// chain 沿 parent 鏈收集 prevID 及所有祖先（由新到舊）。
//
// 返回 false 表示鏈不完整：prevID 不存在、或路徑上有條目已被 TTL/容量淘汰。
// 這種情況寧可回明確 400，也不送半截歷史上去——後者就是「靜默失憶」。
func (h *Store) chain(prevID string) ([]*historyEntry, bool) {
	if h == nil || prevID == "" {
		return nil, false
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.purgeExpiredLocked()

	limit := h.max + 1 // 任一存活鏈都不可能比容量上限更長
	chain := make([]*historyEntry, 0, 8)
	seen := make(map[string]struct{}, 8)
	for id := prevID; id != ""; {
		if len(chain) >= limit {
			return nil, false
		}
		if _, dup := seen[id]; dup {
			return nil, false // 環：壞資料時明確失敗，不死循環
		}
		seen[id] = struct{}{}
		e := h.entries[id]
		if e == nil {
			return nil, false
		}
		chain = append(chain, e)
		// 檢查點自帶「本輪之前」的完整序列：再往上的祖先不需要，也不再因它們
		// 被淘汰而判定鏈斷（這正是檢查點的價值）。
		if e.snapshot != nil {
			return chain, true
		}
		id = e.parent
	}
	return chain, true
}

// materializeChain 把鏈還原成「本輪之前」的完整 item 序列與兜底 instructions。
//
// 快照（snapshot）會覆蓋之前累積的前綴，所以只要遇到快照就重置基底；
// 條目發布後不再被改寫，item 切片可以直接共用（只讀）。
func materializeChain(chain []*historyEntry) ([]any, string) {
	var base []any
	for i := len(chain) - 1; i >= 0; i-- { // 由舊到新
		e := chain[i]
		if e.snapshot != nil {
			base = append([]any(nil), e.snapshot...)
		}
		base = append(base, e.input...)
		base = append(base, e.output...)
	}
	var instructions string
	for _, e := range chain { // 由新到舊：取最近的非空 instructions
		if e.instructions != "" {
			instructions = e.instructions
			break
		}
	}
	return base, instructions
}

// historyOverlap 回傳「in 的前綴與 base 的後綴重疊」的項目數。
//
// 用於識別客戶端重送歷史：
//   - k == len(base)：完整重送（連開頭一起送）；
//   - 0 < k < len(base)：部分重送（只帶尾段）；
//   - k == 0：新回合，基底全部保留。
//
// 重疊必須**貼住 base 的末尾**（否則就不是「這一段我已經有了」）；因此只考慮
// j >= len(base)-len(in) 的位置，k 不做截斷。由前往後掃，第一個命中即最大重疊。
// base 的末項通常是上一輪的助手輸出、in 首項是 user 訊息，k=1 誤配很難發生。
//
// 成本按兩道上限處理：首項比對最多走一遍 base（每項一次深比較），整段驗証
// 最多 16 次——避免病態資料把熱路徑拖成 O(n×m)。
func historyOverlap(in, base []any) int {
	if len(in) == 0 || len(base) == 0 {
		return 0
	}
	const maxChecks = 16 // 整段驗証次數上限
	checks := 0
	for j := 0; j < len(base); j++ {
		k := len(base) - j
		if k > len(in) {
			continue // 這一段比 in 還長：不可能全落在 in 的前綴裡
		}
		if !reflect.DeepEqual(in[0], base[j]) {
			continue
		}
		if checks++; checks > maxChecks {
			break
		}
		match := true
		for i := 1; i < k; i++ {
			if !reflect.DeepEqual(in[i], base[j+i]) {
				match = false
				break
			}
		}
		if match {
			return k
		}
	}
	return 0
}

// Replay 把 previous_response_id 指向的歷史重組進目前請求。
//
// 成功時：payload.input 被換成完整歷史、previous_response_id 被移除、
// 必要時補回最初的 instructions；回傳 OK。
// 找不到 / 已過期 / 鏈上缺一環 → NotFound（呼叫方回 400 unsupported_parameter）。
//
// 若有效窗口 > 0 且重放後的鏈超過上限：
//   - payload["truncation"] == "auto"：裁掉最舊的中段輪次（保留開頭系統/首則訊息
//     與最近可容納範圍），照常送上游，回 OK。
//   - 否則：不更動 payload，回 TooLong 與明確訊息（呼叫方回 400），
//     绝不靜默砍歷史——那正是「失憶」的根源。
func (h *Store) Replay(payload map[string]any, prevID, modelID string, contextWindow int) (replayOutcome, string) {
	chain, ok := h.chain(prevID)
	if !ok {
		return NotFound, ""
	}
	base, inherited := materializeChain(chain)
	in := InputItems(payload)

	// 有些客戶端（多見於反代 / 封裝層）會把歷史重送一遍：完整重送、或只帶
	// 尾段。直接 base+in 會讓重疊部分重複（token 翻倍、更快撞上限）。
	// 用最大後綴重疊去重：k=0 等同不重疊（新回合），k=len(base) 等同完整重送。
	k := historyOverlap(in, base) // k=0 时 base[:len(base)] 即完整基底
	merged := make([]any, 0, len(base)+len(in)-k+4)
	merged = append(merged, base[:len(base)-k]...)
	merged = append(merged, in...)

	// 有效窗口按模型實測值查表（見 EffectiveContextWindow）；少數模型上限低於
	// 目錄標示（mimo-flash 140k、glyph 256k），其餘 9 個免費模型可吃滿 320k。
	// 讓網關先裁剪並留下標記，也不要讓上游偷偷砍歷史。
	limit := EffectiveContextWindow(modelID, contextWindow)
	instructions := jsonx.StrOf(payload["instructions"])
	if instructions == "" {
		instructions = inherited
	}
	// 固定開銷＝instructions + tools 定義：兩者每輪都隨請求送上去，都佔上下文。
	// 舊版只算 instructions，tools 大（幾十個 function schema）時會低估幾千
	// token，讓「剛好超限」的請求穿過檢查、由上游靜默截斷歷史。
	fixedTokens := ItemApproxTokens(instructions)
	if tools, ok := payload["tools"]; ok {
		fixedTokens += ItemApproxTokens(tools)
	}
	// 輸出預算：客戶端要求的 max_output_tokens 也要在窗口內。不預留的話，
	// 歷史貼著上限時上游只能二選一：截斷歷史（失憶）或超額。
	reserve := 0
	if n := jsonx.IntOf(payload["max_output_tokens"]); n > 0 {
		reserve = n
	}
	if limit > 0 {
		costs := make([]int, len(merged))
		total := fixedTokens
		for i, it := range merged {
			costs[i] = ItemApproxTokens(it)
			total += costs[i]
		}
		if total+reserve > limit {
			if jsonx.StrOf(payload["truncation"]) != "auto" {
				return TooLong, "重放後的請求（對話歷史 + 工具定義）約 " + itoaApprox(total) +
					" tokens" + reserveNote(reserve) +
					"，超過模型有效上下文 contextWindow " + itoaApprox(limit) +
					"（目錄標示 " + itoaApprox(contextWindow) +
					"）；請由客戶端壓縮歷史，或於請求設定 \"truncation\":\"auto\" 由網關自動裁減最舊中段。"
			}
			merged = truncateHistoryWithCosts(merged, costs, limit-fixedTokens-reserve)
			// 重算裁剪後的實際規模（含插入的說明訊息）。
			remaining := fixedTokens
			for _, it := range merged {
				remaining += ItemApproxTokens(it)
			}
			if remaining+reserve > limit {
				return TooLong, "truncation:auto 無法將對話壓至模型有效上下文 contextWindow " + itoaApprox(limit) + " tokens" + reserveNote(reserve) + "；請先縮短單則訊息或工具輸出。"
			}
		}
	}

	payload["input"] = merged
	if jsonx.StrOf(payload["instructions"]) == "" && inherited != "" {
		payload["instructions"] = inherited
	}
	delete(payload, "previous_response_id")
	return OK, ""
}

// reserveNote 把「為 max_output_tokens 預留」的說明拼進錯誤訊息（未預留時為空串）。
func reserveNote(reserve int) string {
	if reserve <= 0 {
		return ""
	}
	return "（已為 max_output_tokens 預留 " + itoaApprox(reserve) + " tokens）"
}

// truncateHistoryWithCosts 裁掉最舊的中段輪次：保留開頭（系統訊息 + 第一則
// 使用者訊息，通常是重要背景/暗號）與最近可容納的尾段；中間插入一則說明訊息。
//
// budget 是「可分配給對話 item 的 token 預算」，呼叫方已扣除 instructions /
// tools；說明訊息自身的 token 由本函式再扣。costs 由呼叫方一次算好，避免同一批
// item 被反覆 JSON 序列化（replay 路徑下每輪都要估算整條歷史）。
func truncateHistoryWithCosts(items []any, costs []int, budget int) []any {
	if budget <= 0 || len(items) <= 2 {
		return items
	}
	if len(costs) != len(items) { // 防禦：呼叫方長度不符時自行估算
		costs = make([]int, len(items))
		for i, it := range items {
			costs[i] = ItemApproxTokens(it)
		}
	}
	marker := historyTruncMarker()
	// 說明訊息也佔 token：不先扣掉，裁剪結果會剛好超出預算，讓 replay
	// 誤判「壓不下去」而回 400（issue #5 的續接在邊界上偶發失敗）。
	avail := budget - ItemApproxTokens(marker)
	if avail < 1 {
		avail = 1
	}
	// head = 開頭連續的 system 訊息，再加上第一則非 system 訊息（開頭訊息）。
	headEnd := 0
	for headEnd < len(items) {
		m, ok := items[headEnd].(map[string]any)
		if !ok || jsonx.StrOf(m["role"]) != "system" {
			break
		}
		headEnd++
	}
	if headEnd < len(items) {
		headEnd++
	}
	used := 0
	for _, c := range costs[:headEnd] {
		used += c
	}

	// tail = 由最新往回，尽可能容納。
	tailStart := len(items)
	for tailStart > headEnd {
		c := costs[tailStart-1]
		if used+c > avail {
			break
		}
		used += c
		tailStart--
	}
	if tailStart >= len(items) {
		tailStart = len(items) - 1 // 至少保留最新一則
	}
	if tailStart < headEnd {
		tailStart = headEnd
	}

	out := make([]any, 0, headEnd+(len(items)-tailStart)+1)
	out = append(out, items[:headEnd]...)
	out = append(out, marker)
	out = append(out, items[tailStart:]...)
	return dropOrphanToolOutputs(out)
}

// historyTruncMarker 是插入裁減處的說明訊息。
//
// 必須用 user role：上游會拒絕「system 出現在非首則」的請求體
// （實測 Chat Completions：system 在中間 → 400 This request cannot be accepted）。
func historyTruncMarker() map[string]any {
	return map[string]any{
		"type": "message", "role": "user",
		"content": "（中間的對話歷史因超過上下文長度已由網關自動省略）",
	}
}

// dropOrphanToolOutputs 丟掉「工具調用已被裁掉」的 function_call_output。
//
// Responses 的 function_call / function_call_output 以 call_id 成對；裁剪按
// item 邊界切，必然有機會把兩者分到不同側。gateway 把 output 翻成 role=tool
// 的消息，而上游要求 tool 訊息緊跟在帶 tool_calls 的 assistant 之後——孤立的
// tool 訊息會 400，也就是「歷史一被裁，之後每一輪都失敗」。
//
// 判斷用位置而非全局 call_id 集合：中間插入的說明訊息（user role）同樣會打斷
// assistant 的 tool_calls 鏈，僅靠「這個 call_id 在保留集合裡」會漏掉這種情況。
func dropOrphanToolOutputs(items []any) []any {
	if len(items) == 0 {
		return items
	}
	// active = 上一則 assistant 訊息（含被合併的連續 function_call）帶的 call_id。
	active := make(map[string]struct{}, 4)
	reset := func() { active = make(map[string]struct{}, 4) }
	out := items[:0]
	for _, it := range items {
		m, ok := it.(map[string]any)
		if !ok {
			reset() // 純字串 item 會變成 user 訊息
			out = append(out, it)
			continue
		}
		switch jsonx.StrOf(m["type"]) {
		case "function_call":
			if id := toolCallID(m); id != "" {
				active[id] = struct{}{}
			}
		case "function_call_output":
			if _, ok := active[toolCallID(m)]; !ok {
				continue // 配對的 function_call 已被裁掉：丟掉，否則上游 400
			}
		case "message", "":
			if jsonx.StrOf(m["role"]) != "system" {
				reset()
			}
		}
		out = append(out, it)
	}
	return out
}

// toolCallID 取 function_call / function_call_output 的配對 id；
// 與 gateway 的 FirstString(call_id, id) 保持同一套回退規則。
func toolCallID(m map[string]any) string {
	if id := jsonx.StrOf(m["call_id"]); id != "" {
		return id
	}
	return jsonx.StrOf(m["id"])
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

var (
	ctxOverridesMu  sync.Mutex
	ctxOverridesRaw string
	ctxOverridesMap map[string]int
)

// contextWindowOverrides 解析並緩存 DEEPAI2API_CONTEXT_WINDOWS。
//
// 這條鏈每輪重放都要跑；舊版每次都 json.Unmarshal 一次環境變數（每請求
// 一次解析）。緩存以「原始字串」為 key：改環境變數（包括測試裡的 t.Setenv）
// 會自動失效，不需要重啟也不需要顯式 flush。返回的 map 只讀，永不就地修改。
func contextWindowOverrides() map[string]int {
	raw := strings.TrimSpace(os.Getenv("DEEPAI2API_CONTEXT_WINDOWS"))
	if raw == "" {
		return nil
	}
	ctxOverridesMu.Lock()
	defer ctxOverridesMu.Unlock()
	if raw == ctxOverridesRaw {
		return ctxOverridesMap
	}
	ctxOverridesRaw = raw
	ctxOverridesMap = nil
	var m map[string]int
	if err := json.Unmarshal([]byte(raw), &m); err == nil {
		ctxOverridesMap = m
	}
	// 格式錯誤時保持 nil：等於忽略整項，與舊行為一致。
	return ctxOverridesMap
}

// EffectiveContextWindow 回傳重放時真正可用的上下文上限（按模型）。
//
// 優先順序（高 → 低）：
//  1. DEEPAI2API_EFFECTIVE_CONTEXT_WINDOW：全域覆寫（0/負數 = 不設上限）。
//     保留舊行為，作為排查/臨時降上限用。
//  2. DEEPAI2API_CONTEXT_WINDOWS：JSON 物件，按 model id 覆寫，例如
//     {"mimo-2.6-flash":140000,"glm-5.3-flash":320000}
//     （值 0/負數 = 該模型不設上限；格式錯誤則忽略整項）。
//  3. perModelContextWindow：上面的實測例外表。
//  4. min(標示值, catalogContextCap)；標示值 ≤ 0 時用 catalogContextCap。
func EffectiveContextWindow(modelID string, catalog int) int {
	if raw := strings.TrimSpace(os.Getenv("DEEPAI2API_EFFECTIVE_CONTEXT_WINDOW")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil {
			if n <= 0 {
				return 0
			}
			return n
		}
	}
	if raw := strings.TrimSpace(os.Getenv("DEEPAI2API_CONTEXT_WINDOWS")); raw != "" {
		if n, ok := contextWindowOverrides()[modelID]; ok {
			if n <= 0 {
				return 0
			}
			return n
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

// ItemApproxTokens 粗估 item 的 token 數。中文等全形字元約 1 token/字，
// ASCII 約 1.6 bytes/token（實測 160k 英文隨機字串 → 100,761 input_tokens，
// 比值 1.59；舊的 len/3 對程式碼場景低估約一倍，會讓自動壓縮太晚觸發）。
// 只在上限判斷時呼叫，正常情況零額外開銷。
func ItemApproxTokens(item any) int {
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

func (h *Store) purgeExpiredLocked() {
	now := h.now()
	kept := h.order[:0]
	for _, id := range h.order {
		e := h.entries[id]
		if e == nil || now.Sub(e.createdAt) > h.ttl {
			if e != nil {
				h.usedBytes -= e.bytes
			}
			delete(h.entries, id)
			continue
		}
		kept = append(kept, id)
	}
	h.order = kept
}

// itemsBefore 回傳 e 之前的完整 item 序列（供造檢查點用）。
//
// 必須在持有 h.mu 時呼叫：它要讀父條目。優先用最近的快照作為基底，
// 否則從最舊的祖先开始拼接。
func (h *Store) itemsBefore(e *historyEntry) []any {
	var newer []*historyEntry // 由新到舊，不含 e
	cur := h.entries[e.parent]
	for i := 0; cur != nil && i <= h.max; i++ { // 有界：鏈不可能比容量更長
		if cur.snapshot != nil {
			out := append([]any(nil), cur.snapshot...)
			out = append(out, cur.input...)
			out = append(out, cur.output...)
			for j := len(newer) - 1; j >= 0; j-- {
				out = append(out, newer[j].input...)
				out = append(out, newer[j].output...)
			}
			return out
		}
		newer = append(newer, cur)
		cur = h.entries[cur.parent]
	}
	out := make([]any, 0, 16)
	for j := len(newer) - 1; j >= 0; j-- {
		out = append(out, newer[j].input...)
		out = append(out, newer[j].output...)
	}
	return out
}

// historyEntryBytes 估算條目序列化後的位元組數。
//
// 用結構走訪而不是 json.Marshal：這只是容量水位（maxBytes 64 MiB）的量尺，
// 沒必要為每次寫入再複製一份完整 JSON（舊版每輪都要序列化整條歷史）。
func historyEntryBytes(e *historyEntry) int64 {
	if e == nil {
		return 0
	}
	return 64 + int64(len(e.id)+len(e.parent)+len(e.instructions)) +
		jsonBytesOf(e.input) + jsonBytesOf(e.output) + jsonBytesOf(e.snapshot)
}

// jsonBytesOf 估算 JSON 值序列化後的位元組數：只走一遍結構，不分配。
//
// 字串按 len 計（轉義字元會略低估，份量級仍然正確）；容器按固定開銷 + 遞迴。
func jsonBytesOf(v any) int64 {
	switch t := v.(type) {
	case nil:
		return 4 // null
	case string:
		return int64(len(t)) + 2
	case bool:
		return 5
	case float64, int, int64:
		return 8
	case []any:
		n := int64(2)
		for _, it := range t {
			n += jsonBytesOf(it) + 1
		}
		return n
	case map[string]any:
		n := int64(2)
		for k, val := range t {
			n += int64(len(k)) + 3 + jsonBytesOf(val) + 1
		}
		return n
	default:
		return 16
	}
}

// len 供測試與監控使用。
func (h *Store) Len() int {
	if h == nil {
		return 0
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.entries)
}

// --- 工具函式 ---

// InputItems 把 payload.input 正規化成 Responses item 切片（深拷貝）。
// 字串 input 轉成單一 user message item，與 gateway 的轉換保持一致。
func InputItems(payload map[string]any) []any {
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
