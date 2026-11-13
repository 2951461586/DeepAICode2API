// Package store 负责网关状态的持久化（账号 / 密钥 / 设置 / 日志）。
//
// 选号策略在 internal/pool；本包只在持锁状态下调用它，并把结果作为快照交付。
package store

import (
	"sync"
	"sync/atomic"

	"deepaicode2api/internal/deepai"
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
	// 上游抬车道时旧值一律回 426；网关启动时用免鉴权的 /models 探测一次，
	// 命中就写在这里，升级不用改代码。
	Lane string `json:"lane,omitempty"`

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
	CachedTokens int     `json:"cached_tokens,omitempty"`
	Cost         float64 `json:"cost"`
	Err          string  `json:"err,omitempty"`
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
	TotalCachedTokens int64   `json:"total_cached_tokens,omitempty"`
	TotalCost         float64 `json:"total_cost"`
	// Day / DayTokensIn / DayTokensOut / DayCachedTokens 是「按本地日期」的当日用量。
	//
	// 跨天时由 Record 自动清零（不需要定时任务）；Date 用 YYYY-MM-DD。
	Day             string `json:"day,omitempty"`
	DayTokensIn     int64  `json:"day_tokens_in,omitempty"`
	DayTokensOut    int64  `json:"day_tokens_out,omitempty"`
	DayCachedTokens int64  `json:"day_cached_tokens,omitempty"`
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
	Accounts      []*deepai.Account `json:"accounts"`
	Keys          []*APIKey         `json:"keys"`
	Settings      Settings          `json:"settings"`
	Stats         Stats             `json:"-"`
	Logs          []LogEntry        `json:"-"`
	Models        []deepai.Model    `json:"models"`
	DefaultModel  string            `json:"default_model"`
	ModelsUpdated int64             `json:"models_updated"`
	MachineID     string            `json:"machine_id"`
}

// legacyState 是拆分前的状态文件格式（含 logs / stats），仅用于读取迁移。
type legacyState struct {
	Accounts      []*deepai.Account `json:"accounts"`
	Keys          []*APIKey         `json:"keys"`
	Settings      Settings          `json:"settings"`
	Stats         Stats             `json:"stats"`
	Logs          []LogEntry        `json:"logs"`
	Models        []deepai.Model    `json:"models"`
	DefaultModel  string            `json:"default_model"`
	ModelsUpdated int64             `json:"models_updated"`
	MachineID     string            `json:"machine_id"`
}

// Store 是带锁的状态容器。
type Store struct {
	mu    sync.RWMutex
	path  string
	st    *State
	sched *pool.Scheduler

	// statsPath / logsPath 是由主路径派生的旁路文件。
	statsPath string
	logsPath  string
	logs      *logStore

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
		RequestTimeout: 600,
		MaxRetries:     3,
		LogKeep:        500,
		// 默认仅放行本机来源；如需局域网访问，在控制台显式配置。
		CORSOrigin:    "",
		FallbackModel: "",
	}
}
