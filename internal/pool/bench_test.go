package pool

import (
	"strconv"
	"testing"

	"deepaicode2api/internal/deepai"
)

// benchAccounts 构造 n 个健康账号（含一个 balance 字段，贴近真实快照）。
func benchAccounts(n int) []*deepai.Account {
	out := make([]*deepai.Account, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, &deepai.Account{
			ID:       "acc-" + strconv.Itoa(i),
			Name:     "acc-" + strconv.Itoa(i),
			APIKey:   "key-" + strconv.Itoa(i),
			Enabled:  true,
			Status:   "ok",
			Failures: i % 3,
			Theos:    &deepai.Allowance{Balance: "10", Granted: "20", Enabled: true},
		})
	}
	return out
}

// BenchmarkPick500 是选号热路径的基线。
//
// 目标：500 个号的池子里单次选号保持在纳秒级、零额外分配。
// 若哪天有人把「深拷贝整个池」塞回来，这里的 ns/op 与 allocs/op 会立刻变形。
func BenchmarkPick500(b *testing.B) {
	s := New()
	accounts := benchAccounts(500)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if s.Pick(accounts, "") == nil {
			b.Fatal("应选到账号")
		}
	}
}

// BenchmarkPickFiltered500 覆盖「带并发槽位过滤」的路径（服务端实际用法）。
func BenchmarkPickFiltered500(b *testing.B) {
	s := New()
	accounts := benchAccounts(500)
	skip := func(a *deepai.Account) bool { return a.ID == "acc-7" }
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if s.PickFiltered(accounts, "", skip) == nil {
			b.Fatal("应选到账号")
		}
	}
}

// BenchmarkHasEligible500 覆盖全忙判定路径。
func BenchmarkHasEligible500(b *testing.B) {
	s := New()
	accounts := benchAccounts(500)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if !s.HasEligible(accounts, "") {
			b.Fatal("应有可调度账号")
		}
	}
}
