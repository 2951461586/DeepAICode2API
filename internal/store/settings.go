package store

import (
	"fmt"
	"math"
	"strings"
	"time"
)

// --- 设置的默认值、归一化与校验 ---
//
// 为什么集中在这里：Settings 是 15 个字段的扁平结构，运行期可由控制台任意
// 改写。此前每个消费点各自兜底 ——
//
//	anthropic.requestTimeout()  if sec <= 0 { sec = 600 }
//	upstream.openModel()        if maxTries <= 0 { maxTries = 3 }
//	admin.settingsPatch.apply() if *p.X > 0 { ... }
//
// 三处规则各写一份，改默认值要改三处，而且「0 是关闭」与「0 是未设置」
// 混在同一个判断里（限流参数 0 = 关闭，重试次数 0 = 未设置）。现在：
//
//	Normalize()  只填「0 没有语义」的字段，且必须幂等
//	Validate()   管理端写入前调用，返回可读原因（4xx 文案）
//	Runtime()    把常用项换算成 Duration / int，消费端不再自己算

// 设置的默认值（也是「归零」的目标值）。
const (
	DefaultRequestTimeout = 600   // 秒；上游长回复 + 重试的总预算
	DefaultMaxRetries     = 3     // 单请求最多换几个账号
	DefaultLogKeep        = 500   // 请求日志保留条数
	DefaultCheckinBlockMS = 0     // 0 = 用客户端默认预算（10s）
	MaxRequestTimeout     = 86400 // 一天；再长没有实际意义，只会掩盖挂死
	MaxRetriesCeiling     = 20
	MaxLogKeep            = 100000
	MaxCheckinBlockMS     = 600000 // 10 分钟；签到上限本身是 10s 级
)

// 非流式预算 / 上游响应超时 / 请求体上限的默认值与区间（issue #7、2026-11-25 长尾）。
const (
	// DefaultNonStreamBudget 是非流式请求的整体预算：比旧的硬编码 90 s 宽，
	// 又比 request_timeout（默认 600 s）短，避免慢请求长时间占号池。
	DefaultNonStreamBudget = 180
	MinNonStreamBudget     = 30
	MaxNonStreamBudget     = 600

	// DefaultStreamIdle 是流式帧间空闲预算；DefaultFirstByte 是非流式首字节预算。
	DefaultStreamIdle = 120
	MinStreamIdle     = 10
	MaxStreamIdle     = 600
	DefaultFirstByte  = 300
	MinFirstByte      = 30
	MaxFirstByte      = 600

	// DefaultContextBodyBytes 是上游请求体安全上限（issue #7 实测约 196 KB 起静默砍头）。
	DefaultContextBodyBytes = 196608
	MinContextBodyBytes     = 4096
	MaxContextBodyBytes     = 64 << 20

	// BodyCapWarn 只告警（既有行为）；BodyCapReject 直接 400。
	BodyCapWarn   = "warn"
	BodyCapReject = "reject"
)

// Runtime 是「换算好、可直接用」的设置视图。
//
// 只放热路径真正需要、且换算规则唯一的值；其他字段直接读 Settings。
type Runtime struct {
	// RequestTimeout 是单请求整体预算（已保证 > 0）。
	RequestTimeout time.Duration
	// MaxRetries 是单请求最多尝试的账号数（已保证 >= 1）。
	MaxRetries int
	// LogKeep 是日志保留条数（已保证 >= 1）。
	LogKeep int
	// CheckinBudget 是推理前同步签到的等待预算；0 = 用客户端默认值。
	CheckinBudget time.Duration
	// NonStreamBudget 是非流式请求的整体预算（已保证 > 0）。
	NonStreamBudget time.Duration
	// StreamIdle / FirstByte 是上游响应的帧间空闲 / 首字节预算（已保证 > 0）。
	StreamIdle time.Duration
	FirstByte  time.Duration
}

// Normalize 返回归一化后的设置副本（幂等）。
//
// 只处理「0 / 负数没有语义」的字段。以下 0 是**合法语义**，必须保留：
//   - CORSOrigin 空 = 只放行本机来源；
//   - RateLimitRPS / RateLimitBurst / KeyConcurrency 为 0 = 不限；
//   - CheckinBlockMS 为 0 = 用客户端默认 10s；
//   - StrictModel / PresetDesktopShape 的 false = 关闭。
func (s Settings) Normalize() Settings {
	if s.RequestTimeout <= 0 {
		s.RequestTimeout = DefaultRequestTimeout
	}
	if s.MaxRetries <= 0 {
		s.MaxRetries = DefaultMaxRetries
	}
	if s.LogKeep <= 0 {
		s.LogKeep = DefaultLogKeep
	}
	// 负数一律夹到 0（只可能来自手改状态文件）。
	if s.RateLimitRPS < 0 {
		s.RateLimitRPS = 0
	}
	if s.RateLimitBurst < 0 {
		s.RateLimitBurst = 0
	}
	if s.KeyConcurrency < 0 {
		s.KeyConcurrency = 0
	}
	if s.CheckinBlockMS < 0 {
		s.CheckinBlockMS = 0
	}
	if s.NonStreamBudget <= 0 {
		s.NonStreamBudget = DefaultNonStreamBudget
	}
	if s.StreamIdleSec <= 0 {
		s.StreamIdleSec = DefaultStreamIdle
	}
	if s.FirstByteSec <= 0 {
		s.FirstByteSec = DefaultFirstByte
	}
	if s.ContextBodyBytes <= 0 {
		s.ContextBodyBytes = DefaultContextBodyBytes
	}
	// BodyCapMode：空值（旧状态文件 / 首次启动）等同于 warn，保持既有语义。
	s.BodyCapMode = strings.ToLower(strings.TrimSpace(s.BodyCapMode))
	if s.BodyCapMode == "" {
		s.BodyCapMode = BodyCapWarn
	}
	s.CORSOrigin = strings.TrimSpace(s.CORSOrigin)
	s.FallbackModel = strings.TrimSpace(s.FallbackModel)
	s.Lane = strings.TrimSpace(s.Lane)
	if s.ModelAliases == nil {
		s.ModelAliases = map[string]string{}
	}
	return s
}

// Validate 检查设置是否可用；返回的错误直接作为 4xx 文案下发。
//
// 与 Normalize 的分工：Normalize 容忍「未设置」（补默认值），Validate 只拒绝
// 「明确写了但不可能成立」的值（超范围、NaN、通配符来源）。
func (s Settings) Validate() error {
	if s.RequestTimeout < 1 || s.RequestTimeout > MaxRequestTimeout {
		return fmt.Errorf("请求超时必须在 1~%d 秒之间（当前 %d）", MaxRequestTimeout, s.RequestTimeout)
	}
	if s.MaxRetries < 1 || s.MaxRetries > MaxRetriesCeiling {
		return fmt.Errorf("重试次数必须在 1~%d 之间（当前 %d）", MaxRetriesCeiling, s.MaxRetries)
	}
	if s.LogKeep < 1 || s.LogKeep > MaxLogKeep {
		return fmt.Errorf("日志保留条数必须在 1~%d 之间（当前 %d）", MaxLogKeep, s.LogKeep)
	}
	if s.CheckinBlockMS < 0 || s.CheckinBlockMS > MaxCheckinBlockMS {
		return fmt.Errorf("签到阻塞预算必须在 0~%d 毫秒之间（当前 %d）", MaxCheckinBlockMS, s.CheckinBlockMS)
	}
	if err := s.validateBudgets(); err != nil {
		return err
	}
	if math.IsNaN(s.RateLimitRPS) || math.IsInf(s.RateLimitRPS, 0) {
		return fmt.Errorf("每秒请求数必须是有限数值（当前 %v）", s.RateLimitRPS)
	}
	if s.RateLimitRPS < 0 {
		return fmt.Errorf("每秒请求数不能为负（当前 %v）", s.RateLimitRPS)
	}
	if s.RateLimitBurst < 0 {
		return fmt.Errorf("突发容量不能为负（当前 %d）", s.RateLimitBurst)
	}
	if s.KeyConcurrency < 0 {
		return fmt.Errorf("每密钥并发上限不能为负（当前 %d）", s.KeyConcurrency)
	}
	// 通配符来源等于对任何网站开放管理面（含 SameSite/CORS 预检），
	// 口径与 guard.Origin 的「不提供通配符」一致。
	for _, origin := range strings.Split(s.CORSOrigin, ",") {
		origin = strings.TrimSpace(origin)
		if origin == "" {
			continue
		}
		if strings.Contains(origin, "*") {
			return fmt.Errorf("跨域来源不支持通配符：%q（请逐个列出，或用空值仅放行本机）", origin)
		}
	}
	return nil
}

// validateBudgets 校验超时 / 请求体上限这一组「预算类」字段。
//
// 单独拆出来是因为它们只在数值范围上不同（且新增字段还在长），留在 Validate
// 里会把那个函数堆成一堆 if——复杂度高到看不出真正的业务规则（如 CORS 通配符
// 禁止）。两处不一致时以 Normalize 为准（0 = 未设置）。
func (s Settings) validateBudgets() error {
	check := func(name string, v, min, max int, unit string) error {
		if v < min || v > max {
			return fmt.Errorf("%s必须在 %d~%d %s之间（当前 %d）", name, min, max, unit, v)
		}
		return nil
	}
	if err := check("非流式预算", s.NonStreamBudget, MinNonStreamBudget, MaxNonStreamBudget, "秒"); err != nil {
		return err
	}
	if err := check("流式空闲超时", s.StreamIdleSec, MinStreamIdle, MaxStreamIdle, "秒"); err != nil {
		return err
	}
	if err := check("首字节超时", s.FirstByteSec, MinFirstByte, MaxFirstByte, "秒"); err != nil {
		return err
	}
	if err := check("请求体上限", s.ContextBodyBytes, MinContextBodyBytes, MaxContextBodyBytes, "字节"); err != nil {
		return err
	}
	if s.BodyCapMode != BodyCapWarn && s.BodyCapMode != BodyCapReject {
		return fmt.Errorf("请求体超限处置只能是 %q 或 %q（当前 %q）", BodyCapWarn, BodyCapReject, s.BodyCapMode)
	}
	return nil
}

// Runtime 返回换算好的运行期视图（内部先 Normalize，调用方无需预处理）。
func (s Settings) RuntimeView() Runtime {
	s = s.Normalize()
	return Runtime{
		RequestTimeout:  time.Duration(s.RequestTimeout) * time.Second,
		MaxRetries:      s.MaxRetries,
		LogKeep:         s.LogKeep,
		CheckinBudget:   time.Duration(s.CheckinBlockMS) * time.Millisecond,
		NonStreamBudget: time.Duration(s.NonStreamBudget) * time.Second,
		StreamIdle:      time.Duration(s.StreamIdleSec) * time.Second,
		FirstByte:       time.Duration(s.FirstByteSec) * time.Second,
	}
}
