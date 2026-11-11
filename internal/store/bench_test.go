package store

import (
	"path/filepath"
	"strconv"
	"testing"

	"deepaicode2api/internal/deepai"
)

// benchStore 构造一个装着 n 个账号的 Store（不落盘）。
func benchStore(b *testing.B, n int) *Store {
	b.Helper()
	st, err := Open(filepath.Join(b.TempDir(), "state.json"))
	if err != nil {
		b.Fatalf("store.Open: %v", err)
	}
	for i := 0; i < n; i++ {
		st.UpsertAccount(&deepai.Account{
			ID:      "acc-" + strconv.Itoa(i),
			Name:    "acc-" + strconv.Itoa(i),
			APIKey:  "key-" + strconv.Itoa(i),
			Email:   "user" + strconv.Itoa(i) + "@example.com",
			Enabled: true,
			Status:  "ok",
			Theos:   &deepai.Allowance{Balance: "10", Enabled: true},
		})
	}
	return st
}

// BenchmarkAccountPage600 是控制台账号分页的基线。
//
// 600 个号、每页 50 行：应当只克隆 50 个账号，而不是整池。
func BenchmarkAccountPage600(b *testing.B) {
	st := benchStore(b, 600)
	q := AccountQuery{Offset: 100, Limit: 50, Sort: "last_used", Desc: true}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		page := st.AccountPage(q)
		if len(page.Items) != 50 {
			b.Fatalf("应返回 50 条，实际 %d", len(page.Items))
		}
	}
}

// BenchmarkPickAccount600 覆盖「一次读锁 + 只拷贝选中账号」的选号路径。
func BenchmarkPickAccount600(b *testing.B) {
	st := benchStore(b, 600)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		a, ok := st.PickAccount("", nil)
		if !ok || a == nil {
			b.Fatal("应选到账号")
		}
	}
}

// BenchmarkAnySchedulable600 覆盖全忙判定（不拷贝）。
func BenchmarkAnySchedulable600(b *testing.B) {
	st := benchStore(b, 600)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if !st.AnySchedulable(nil) {
			b.Fatal("应有可调度账号")
		}
	}
}
