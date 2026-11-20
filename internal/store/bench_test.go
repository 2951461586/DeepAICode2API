package store

import (
	"path/filepath"
	"strconv"
	"testing"

	"deepaicode2api/internal/model"
)

// benchStore 构造一个装着 n 个账号的 Store（不落盘）。
func benchStore(b *testing.B, n int) *Store {
	b.Helper()
	st, err := Open(filepath.Join(b.TempDir(), "state.json"))
	if err != nil {
		b.Fatalf("store.Open: %v", err)
	}
	for i := 0; i < n; i++ {
		st.UpsertAccount(&model.Account{
			ID:      "acc-" + strconv.Itoa(i),
			Name:    "acc-" + strconv.Itoa(i),
			APIKey:  "key-" + strconv.Itoa(i),
			Email:   "user" + strconv.Itoa(i) + "@example.com",
			Enabled: true,
			Status:  "ok",
			Theos:   &model.Allowance{Balance: "10", Enabled: true},
		})
	}
	return st
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
