// Package store 负责网关状态的持久化（账号 / 密钥 / 设置 / 日志）。
//
// 选号策略在 internal/pool；本包只在持锁状态下调用它，并把结果作为快照交付。
package store

import (
	"sync"
	"sync/atomic"

	"deepaicode2api/internal/model"
	"deepaicode2api/internal/pool"
)

// APIKey 是网关对外签发的调用密钥。
type APIKey struct {
	Key       string `json:"key"`
	Name      string `json:"name"`
	Group     string `json:"group,omitempty"`
	Enabled   bool   `json:"enabled"`
	CreatedAt int64  `json:"created_at"`
	LastUsed  int64  `json:"last_used"`
	Requests  int64  `json:"requests"`
}

// Settings 是运行期可调参数。
type Settings struct {
	ModelAliases   map[string]string `json:"model_aliases"`
	RequestTimeout int               `json:"request_timeout_sec"`
	MaxRetries     int               `json:"max_retries"`
	LogKeep        int               `json:"log_keep"`
	CORSOrigin     string            `json:"cors_origin"`
	AdminPassword  string            `json:"admin_password"`
	FallbackModel  string            `json:"fallback_model"`
	// StrictModel 关闭「解析不到就回落」的行为：模型名不在目录里时返回 404，
	// 而不是静默换成默认模型。默认关闭，保持开箱即用。
	StrictModel bool `json:"strict_model"`

	// Lane 是探测到的 Theos 车道版本（空 = 用编译期默认值）。
	//
	// 网关启动时会借账号凭据探测一次 /models，命中就写在这里，升级不用改代码。
	// 它存的是「探测命中的车道」，**不保证等于上游当前车道**：2026-11-19 上游
	// 放宽门槛后 <=7 全部回 200，探测只能证明那一档能用；上游实际车道见
	// Server.UpstreamLane（来自 /theos 的 lane_version）。
	Lane string `json:"lane,omitempty"`

	// LaneBuild 是写入 Lane 时的编译期车道（model.LaneVersion）。
	//
	// 它把「本次构建探测到的结果」和「旧版本残留」区分开：正常情况下车道
	// 只向前走，但上游也可能只保留旧车道（或探测结果比编译值旧）。只有
	// LaneBuild 与当前编译值一致时，CurrentLane 才无条件信任 Lane；否则仍按
	// max(Lane, 编译值) 取较新值，避免升级后还跑旧车道。
	LaneBuild string `json:"lane_build,omitempty"`

	// RateLimitRPS / RateLimitBurst 是每把密钥的令牌桶（0 = 不限速）。
	// RateLimitBurst 为 0 时按 max(2, RPS) 取容量。
	RateLimitRPS   float64 `json:"rate_limit_rps,omitempty"`
	RateLimitBurst int     `json:"rate_limit_burst,omitempty"`
	// KeyConcurrency 是每把密钥的并发在途请求上限（0 = 不限）。
	KeyConcurrency int `json:"key_concurrency,omitempty"`

	// CheckinBlockMS 是推理前同步签到的等待预算（毫秒，0 = 默认 10000）。
	//
	// 2026-11-13 线上实测签到阻塞推理 292 次、均值 4.8s：调小它可以把首包
	// 延迟压下来，代价是这次请求可能带着旧 nonce 发出（上一轮已回传过，
	// 下一轮继续补），不会让推理失败。
	CheckinBlockMS int `json:"checkin_block_ms,omitempty"`

	// PresetDesktopShape 让网关对「缺 system 消息」的聊天请求预置桌面端形状，
	// 不再先吃一次 400 再重试。
	//
	// 只对已经确认过需要该形状的调用方生效（按 API Key 记忆 10 分钟），
	// 首次仍走「透传 → 400 → 补形状重试」的学习路径。
	PresetDesktopShape bool `json:"preset_desktop_shape,omitempty"`

	// ContextBodyBytes 是发往上游的请求体上限（字节，0 = 用默认 196608）。
	//
	// issue #7：上游约 196 KB 起会对请求体「静默砍头」（丢开头、保留尾部，
	// usage.prompt_tokens 钉在 ~40k）。这个值是网关上唯一的量化闸门，
	// 之前只能靠环境变量 DEEPAI2API_CONTEXT_BODY_BYTES 改，控制台看不到。
	ContextBodyBytes int `json:"context_body_bytes,omitempty"`

	// BodyCapMode 决定请求体超过 ContextBodyBytes 时的处置：
	//
	//	warn   只告警（默认；保持既有「透传成功但可能已被上游截断」的行为）
	//	reject 立刻回 400，不把已知会被截断的请求送上去
	BodyCapMode string `json:"body_cap_mode,omitempty"`

	// NonStreamBudget 是非流式请求的整体预算（秒，0 = 默认 180）。
	//
	// 2026-11-25 线上 6 h 内 60 次 /v1/chat/completions 的 504 全部落在旧的
	// 硬编码 90 s 上（ms=90000、字节=0），是当时 5xx 的绝对主因。
	NonStreamBudget int `json:"nonstream_budget_sec,omitempty"`

	// StreamIdleSec 是上游响应「多久没有新数据就判死」的预算（秒，0 = 默认 120）。
	// FirstByteSec 是非流式等首字节的预算（秒，0 = 默认 300）：长思考模型
	// 首字节前可能超过 2 分钟，不能与帧间空闲用同一个值。
	StreamIdleSec int `json:"stream_idle_sec,omitempty"`
	FirstByteSec  int `json:"first_byte_sec,omitempty"`
}

// LogEntry 是一条请求日志（只记元信息，不落对话内容）。
type LogEntry struct {
	Time      int64  `json:"time"`
	Key       string `json:"key"`
	Account   string `json:"account"`
	Model     string `json:"model"`
	Endpoint  string `json:"endpoint"`
	Stream    bool   `json:"stream"`
	Status    int    `json:"status"`
	LatencyMS int64  `json:"latency_ms"`
	PromptLen int    `json:"prompt_len"`
	OutputLen int    `json:"output_len"`
	TokensIn  int    `json:"tokens_in"`
	TokensOut int    `json:"tokens_out"`
	// CachedTokens 是上游报告的前缀缓存命中 token 数（无此字段时为 0）。
	CachedTokens int `json:"cached_tokens,omitempty"`
	// CacheWriteTokens 是本次「写入前缀缓存」的 prompt token 数。
	//
	// 没有它，控制台上的命中率分不清两种完全不同的 0 命中：
	//   - 新会话首轮 / 空闲超过上游 TTL 后的第一笔：这一段必须写缓存，下一笔才能命中；
	//   - 本该命中却 miss（换号、前缀被改）。
	// 前者是命中率的结构性上限，后者才是要修的问题。
	CacheWriteTokens int     `json:"cache_write_tokens,omitempty"`
	Cost             float64 `json:"cost"`
	// Messages / Sys 是本次请求回喂给上游的历史规模（不含任何正文）。
	//
	// 排障时第一眼要回答的问题就是「是客户端没给历史，还是网关把历史弄丢了」：
	// Messages=1 通常说明调用方只发了本轮（典型是依赖 previous_response_id 续接
	// 却走了无状态的 chat 合成路径），Sys=false 则说明连系统提示都没发。
	Messages int  `json:"messages,omitempty"`
	Sys      bool `json:"system,omitempty"`
	// BodyBytes 是實際發往上游的請求體大小（位元組）。
	//
	// issue #5 的自查建議：messages / system 只能說明「有沒有帶歷史」，帶了很長
	// 的歷史仍可能被上游靜默截斷（issue #7：請求體約 196 KB 起砍掉開頭）。
	// 把出站字節數和 messages 放一起，才能區分「客戶端沒發歷史」「網關弄丟歷史」
	// 與「上游截斷」。
	BodyBytes int `json:"body_bytes,omitempty"`
	// BodyNearCap 表示本次請求體已達安全上限的 90% 以上（僅提示，不阻斷）。
	BodyNearCap bool `json:"body_near_cap,omitempty"`
	// UA / IP 是調用方標識，只留脫敏視圖（UA 截前 120 字符，IP 取真實客戶端地址）。
	//
	// 為什麼需要：BodyBytes + Messages 只說明「發了多大、帶了多少歷史」，說不清
	// 「哪個客戶端在發」。2026-11-27 線上 /responses 約 10% 的 400 全部是上游
	// 「conversation exceeds the context window」，網關側無法從體積反查調用方，
	// 只能請下游挨個自查。有了這兩個字段，下一次同類問題直接在日誌裡歸因。
	// 注意 UA 可偽造、也常被 SDK 固定成一個值：它是線索，不是身份。
	UA  string `json:"ua,omitempty"`
	IP  string `json:"ip,omitempty"`
	Err string `json:"err,omitempty"`
}

// Stats 是累计统计。
type Stats struct {
	TotalRequests  int64 `json:"total_requests"`
	FailedRequests int64 `json:"failed_requests"`
	TotalCharsOut  int64 `json:"total_chars_out"`
	TotalTokensIn  int64 `json:"total_tokens_in"`
	TotalTokensOut int64 `json:"total_tokens_out"`
	// TotalCachedTokens 是上游报告「命中前缀缓存」的 prompt token 总量，
	// 用于算缓存命中率（cached / prompt）。
	//
	// 注意它是**累计值**：跟 DayTokensIn（当日）组合算比率是错的（会得出一个
	// 既不等于累计也不等于当日的数）。控制台因此同时存一份 DayCachedTokens。
	TotalCachedTokens int64 `json:"total_cached_tokens,omitempty"`
	// TotalCacheWriteTokens 是「写入前缀缓存」的 prompt token 总量。
	//
	// 命中率的完整分解：读命中 / 写缓存 / 全新（prompt − 读 − 写）。
	// 三者里的「写」是投资而非损失：它买的是后续请求的读命中。
	TotalCacheWriteTokens int64   `json:"total_cache_write_tokens,omitempty"`
	TotalCost             float64 `json:"total_cost"`
	// Day / DayTokensIn / DayTokensOut / DayCachedTokens 是「按本地日期」的当日用量。
	//
	// 跨天时由 Record 自动清零（不需要定时任务）；Date 用 YYYY-MM-DD。
	Day             string `json:"day,omitempty"`
	DayTokensIn     int64  `json:"day_tokens_in,omitempty"`
	DayTokensOut    int64  `json:"day_tokens_out,omitempty"`
	DayCachedTokens int64  `json:"day_cached_tokens,omitempty"`
	// DayCacheWriteTokens 是当日写入缓存的 prompt token 量（与 DayCachedTokens 同窗口）。
	DayCacheWriteTokens int64 `json:"day_cache_write_tokens,omitempty"`
	// StartedAt 是统计开始时间（首次启动），ResetAt 是最近一次手动重置的时间。
	//
	// 两者分开：控制台需要能解释「统计数比日志少」——重置只清计数，
	// 日志按设计保留。只保留 StartedAt 时，重置后它变成"启动时间"，
	// 语义就含混了。
	StartedAt int64 `json:"started_at"`
	ResetAt   int64 `json:"reset_at,omitempty"`
}

// State 是全部持久化状态。
//
// Logs 与 Stats 刻意不写进主文件：它们随每个请求变动，留在里面会让 Save 的
// “内容未变则跳过”永远失效。日志走 append-only 的 <base>-logs.jsonl，
// 统计走 <base>-stats.json。
type State struct {
	Accounts      []*model.Account `json:"accounts"`
	Keys          []*APIKey        `json:"keys"`
	Settings      Settings         `json:"settings"`
	Stats         Stats            `json:"-"`
	Logs          []LogEntry       `json:"-"`
	Models        []model.Model    `json:"models"`
	DefaultModel  string           `json:"default_model"`
	ModelsUpdated int64            `json:"models_updated"`
	MachineID     string           `json:"machine_id"`
}

// legacyState 是拆分前的状态文件格式（含 logs / stats），仅用于读取迁移。
type legacyState struct {
	Accounts      []*model.Account `json:"accounts"`
	Keys          []*APIKey        `json:"keys"`
	Settings      Settings         `json:"settings"`
	Stats         Stats            `json:"stats"`
	Logs          []LogEntry       `json:"logs"`
	Models        []model.Model    `json:"models"`
	DefaultModel  string           `json:"default_model"`
	ModelsUpdated int64            `json:"models_updated"`
	MachineID     string           `json:"machine_id"`
}

// Store 是带锁的状态容器。
type Store struct {
	mu    sync.RWMutex
	path  string
	st    *State
	sched *pool.Scheduler

	// statsPath / logsPath / stickyPath 是由主路径派生的旁路文件。
	statsPath  string
	logsPath   string
	stickyPath string
	logs       *logStore
	// stickyLast 是上次写入绑定表的明文，用于跳过无变化的重复写盘。
	stickyLast []byte

	// masterKey 非空时状态文件整体加密（AES-256-GCM）。
	masterKey []byte
	// encrypted 记录上次读写时文件是否已加密（用于启动日志 / 控制台提示）。
	encrypted bool
	// lastSaved 是上次落盘的明文快照，用于跳过无变化的重复写盘。
	lastSaved []byte
	// statsDirty 标记统计数据有变化（单独小文件，每次请求都会变）。
	statsDirty bool

	// savesTotal / savedBytes 是主文件的实际写盘次数与明文字节数。
	//
	// 号池涨到 653 个账号后，每个请求都会改账号运行态，很容易让
	// 「内容未变则跳过」失效、退化成 30s 一次约 950KB 的全量重写。
	// 这两个计数（不落盘）用来在压测里直接观测写放大，而不是靠猜。
	savesTotal atomic.Int64
	savedBytes atomic.Int64

	// saveMu 串行化落盘。
	//
	// Save 会被多个 goroutine 调用：后台 30s 周期保存、每个管理操作，
	// 以及批量入池后「每账号一个」的配额复探 goroutine。没有它时（实测）：
	//   - 两个 Save 共用 <path>.tmp，互相截断，rename 后可能落下坏文件；
	//     Windows 上更直接：报「文件被另一个进程占用」而失败；
	//   - 先到者的 rename 晚到者覆盖，磁盘可能比内存旧；
	//   - lastSaved 底层数组读写竞争（-race 可复现）。
	saveMu sync.Mutex
}

// DefaultSettings 返回默认设置。
func DefaultSettings() Settings {
	return Settings{
		ModelAliases:   map[string]string{},
		RequestTimeout: DefaultRequestTimeout,
		MaxRetries:     DefaultMaxRetries,
		LogKeep:        DefaultLogKeep,
		// 默认仅放行本机来源；如需局域网访问，在控制台显式配置。
		CORSOrigin:    "",
		FallbackModel: "",
		// issue #7 / 长尾预算：显式写出默认值，与控制台回显一致。
		ContextBodyBytes: DefaultContextBodyBytes,
		BodyCapMode:      BodyCapWarn,
		NonStreamBudget:  DefaultNonStreamBudget,
		StreamIdleSec:    DefaultStreamIdle,
		FirstByteSec:     DefaultFirstByte,
	}
}
