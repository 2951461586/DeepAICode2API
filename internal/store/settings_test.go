package store

import (
	"math"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// --- 设置默认值 / 归一化 / 校验 ---
//
// 这三条规则此前散在三处（anthropic.requestTimeout 的 <=0→600、
// upstream.openModel 的 <=0→3、admin.settingsPatch 的 >0 判断）。
// 现在只在 store.Settings 上定义一次，用例同时守住「哪些 0 是合法语义」。

// TestNormalizeFillsUnsetDefaults 验证「0 没有语义」的字段被补默认值。
func TestNormalizeFillsUnsetDefaults(t *testing.T) {
	got := Settings{}.Normalize()
	if got.RequestTimeout != DefaultRequestTimeout {
		t.Fatalf("RequestTimeout = %d，期望 %d", got.RequestTimeout, DefaultRequestTimeout)
	}
	if got.MaxRetries != DefaultMaxRetries {
		t.Fatalf("MaxRetries = %d，期望 %d", got.MaxRetries, DefaultMaxRetries)
	}
	if got.LogKeep != DefaultLogKeep {
		t.Fatalf("LogKeep = %d，期望 %d", got.LogKeep, DefaultLogKeep)
	}
	if got.ModelAliases == nil {
		t.Fatal("ModelAliases 不应为 nil（调用方会直接 range）")
	}
}

// TestNormalizeKeepsIntentionalZero 锁定「0 是合法语义」的字段不被改写。
//
// 这些 0 分别表示：不限流 / 不限并发 / 用客户端默认签到预算 / 只放行本机来源。
// 一旦被归一化成非 0，控制台的「关闭限流」就会静默失效。
func TestNormalizeKeepsIntentionalZero(t *testing.T) {
	in := Settings{
		RateLimitRPS:   0,
		RateLimitBurst: 0,
		KeyConcurrency: 0,
		CheckinBlockMS: 0,
		CORSOrigin:     "",
		StrictModel:    false,
	}
	got := in.Normalize()
	if got.RateLimitRPS != 0 || got.RateLimitBurst != 0 || got.KeyConcurrency != 0 {
		t.Fatalf("限流参数的 0（= 关闭）被改写: %+v", got)
	}
	if got.CheckinBlockMS != DefaultCheckinBlockMS {
		t.Fatalf("CheckinBlockMS = %d，期望保持 %d（0 = 用客户端默认）", got.CheckinBlockMS, DefaultCheckinBlockMS)
	}
	if got.CORSOrigin != "" {
		t.Fatalf("空 CORSOrigin（= 仅本机）被改写为 %q", got.CORSOrigin)
	}
}

// TestNormalizeIsIdempotent 验证幂等（Settings() 每次读取都会跑一遍）。
func TestNormalizeIsIdempotent(t *testing.T) {
	first := Settings{RequestTimeout: -5, MaxRetries: -1, RateLimitRPS: -2, CORSOrigin: "  a.com  "}.Normalize()
	second := first.Normalize()
	if first.RequestTimeout != second.RequestTimeout ||
		first.MaxRetries != second.MaxRetries ||
		first.LogKeep != second.LogKeep ||
		first.RateLimitRPS != second.RateLimitRPS ||
		first.CORSOrigin != second.CORSOrigin ||
		len(first.ModelAliases) != len(second.ModelAliases) {
		t.Fatalf("Normalize 不幂等:\n%+v\n%+v", first, second)
	}
	if first.RequestTimeout != DefaultRequestTimeout || first.MaxRetries != DefaultMaxRetries {
		t.Fatalf("负值应归一到默认值: %+v", first)
	}
	if first.RateLimitRPS != 0 {
		t.Fatalf("负的 RPS 应夹到 0，实际 %v", first.RateLimitRPS)
	}
	if first.CORSOrigin != "a.com" {
		t.Fatalf("来源应被 trim，实际 %q", first.CORSOrigin)
	}
}

// TestValidateAcceptsDefaults 验证默认设置有理由被接受。
func TestValidateAcceptsDefaults(t *testing.T) {
	if err := DefaultSettings().Normalize().Validate(); err != nil {
		t.Fatalf("默认设置不应校验失败: %v", err)
	}
}

// TestValidateRejectsOutOfRange 表驱动覆盖每种越界。
func TestValidateRejectsOutOfRange(t *testing.T) {
	base := DefaultSettings()
	cases := []struct {
		name string
		mut  func(*Settings)
		want string // 期望错误里出现的关键词
	}{
		{"超时为 0", func(s *Settings) { s.RequestTimeout = 0 }, "请求超时"},
		{"超时过大", func(s *Settings) { s.RequestTimeout = MaxRequestTimeout + 1 }, "请求超时"},
		{"重试为 0", func(s *Settings) { s.MaxRetries = 0 }, "重试次数"},
		{"重试过大", func(s *Settings) { s.MaxRetries = MaxRetriesCeiling + 1 }, "重试次数"},
		{"日志为 0", func(s *Settings) { s.LogKeep = 0 }, "日志保留"},
		{"日志过大", func(s *Settings) { s.LogKeep = MaxLogKeep + 1 }, "日志保留"},
		{"签到预算过大", func(s *Settings) { s.CheckinBlockMS = MaxCheckinBlockMS + 1 }, "签到阻塞预算"},
		{"RPS 为负", func(s *Settings) { s.RateLimitRPS = -1 }, "每秒请求数"},
		{"RPS 为 NaN", func(s *Settings) { s.RateLimitRPS = math.NaN() }, "有限数值"},
		{"RPS 为 Inf", func(s *Settings) { s.RateLimitRPS = math.Inf(1) }, "有限数值"},
		{"突发为负", func(s *Settings) { s.RateLimitBurst = -1 }, "突发容量"},
		{"并发为负", func(s *Settings) { s.KeyConcurrency = -1 }, "并发上限"},
		{"来源通配符", func(s *Settings) { s.CORSOrigin = "https://*.example.com" }, "通配符"},
		{"来源星号", func(s *Settings) { s.CORSOrigin = "*" }, "通配符"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			st := base
			c.mut(&st)
			err := st.Validate()
			if err == nil {
				t.Fatalf("%s 应校验失败", c.name)
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Fatalf("错误 = %q，期望包含 %q", err.Error(), c.want)
			}
		})
	}
}

// TestValidateAllowsExplicitOriginList 验证逐个列出的来源可以通过。
func TestValidateAllowsExplicitOriginList(t *testing.T) {
	st := DefaultSettings()
	st.CORSOrigin = "http://a.example.com, https://b.example.com:8443"
	if err := st.Validate(); err != nil {
		t.Fatalf("显式来源列表不应被拒绝: %v", err)
	}
}

// TestRuntimeViewConvertsUnits 验证换算只在 RuntimeView 里做一次。
func TestRuntimeViewConvertsUnits(t *testing.T) {
	rt := Settings{RequestTimeout: 90, MaxRetries: 5, LogKeep: 1234, CheckinBlockMS: 2500}.RuntimeView()
	if rt.RequestTimeout != 90*time.Second {
		t.Fatalf("RequestTimeout = %v，期望 90s", rt.RequestTimeout)
	}
	if rt.MaxRetries != 5 {
		t.Fatalf("MaxRetries = %d，期望 5", rt.MaxRetries)
	}
	if rt.LogKeep != 1234 {
		t.Fatalf("LogKeep = %d，期望 1234", rt.LogKeep)
	}
	if rt.CheckinBudget != 2500*time.Millisecond {
		t.Fatalf("CheckinBudget = %v，期望 2.5s", rt.CheckinBudget)
	}

	// 未设置时 RuntimeView 自己归一化，调用方不需要先 Normalize。
	zero := Settings{}.RuntimeView()
	if zero.RequestTimeout != DefaultRequestTimeout*time.Second {
		t.Fatalf("零值 RuntimeView.RequestTimeout = %v", zero.RequestTimeout)
	}
	if zero.MaxRetries != DefaultMaxRetries {
		t.Fatalf("零值 RuntimeView.MaxRetries = %d", zero.MaxRetries)
	}
}

// TestStoreSettingsIsAlwaysNormalized 验证读取处就归一化（旧状态文件缺字段）。
func TestStoreSettingsIsAlwaysNormalized(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	st, err := Open(path)
	if err != nil {
		t.Fatalf("打开状态文件: %v", err)
	}
	// 模拟「手改过 / 旧版本落下」的状态：把三个字段清零。
	st.UpdateSettings(func(s *Settings) {
		s.RequestTimeout = 0
		s.MaxRetries = 0
		s.LogKeep = 0
		s.ModelAliases = nil
	})

	got := st.Settings()
	if got.RequestTimeout != DefaultRequestTimeout || got.MaxRetries != DefaultMaxRetries || got.LogKeep != DefaultLogKeep {
		t.Fatalf("Store.Settings() 未归一化: %+v", got)
	}
	if got.ModelAliases == nil {
		t.Fatal("ModelAliases 不应为 nil")
	}
}
