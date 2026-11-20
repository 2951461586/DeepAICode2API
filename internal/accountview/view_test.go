package accountview

import (
	"strconv"
	"testing"

	"deepaicode2api/internal/model"
)

// 这一组是纯函数用例：输入账号快照列表，断言过滤 / 排序 / 分页结果。
// 不需要 Store（那正是把它从 internal/store 拆出来的收益之一）。

func seed(n int) []*model.Account {
	out := make([]*model.Account, 0, n+1)
	for i := 0; i < n; i++ {
		out = append(out, &model.Account{
			ID:      "id" + strconv.Itoa(i),
			APIKey:  "sk-" + strconv.Itoa(i),
			Email:   "u" + strconv.Itoa(i) + "@x.com",
			Group:   "g1",
			Enabled: true,
			Status:  "ok",
		})
	}
	out = append(out, &model.Account{ID: "other", APIKey: "sk-other", Group: "g2", Enabled: true, Status: "ok"})
	return out
}

func TestAccountsPaginationAndFilter(t *testing.T) {
	all := seed(25)

	page := Accounts(all, Query{Limit: 10})
	if page.Total != 26 || len(page.Items) != 10 {
		t.Fatalf("首页: total=%d items=%d", page.Total, len(page.Items))
	}
	page = Accounts(all, Query{Offset: 20, Limit: 10})
	if len(page.Items) != 6 {
		t.Fatalf("末页应剩 6，实际 %d", len(page.Items))
	}
	page = Accounts(all, Query{Limit: 10, Group: "g2"})
	if page.Total != 1 {
		t.Fatalf("分组过滤: %d", page.Total)
	}
	page = Accounts(all, Query{Limit: 10, Query: "u3@"})
	if page.Total != 1 {
		t.Fatalf("关键词过滤: %d", page.Total)
	}
	// 越界 offset 不应 panic，返回空页但 total 仍准确。
	page = Accounts(all, Query{Offset: 999, Limit: 10})
	if len(page.Items) != 0 || page.Total != 26 {
		t.Fatalf("越界: items=%d total=%d", len(page.Items), page.Total)
	}
	// limit 封顶 200、offset<0 视为 0。
	if page := Accounts(all, Query{Offset: -5, Limit: 999}); len(page.Items) != 26 {
		t.Fatalf("limit 封顶后应返回全部 26 条，实际 %d", len(page.Items))
	}
}

// TestAccountsSortUsesDisplayFields 锁定「排序键用的是展示名」这条展示层规则。
func TestAccountsSortUsesDisplayFields(t *testing.T) {
	all := []*model.Account{
		{ID: "c", Name: "zeta", Email: "", Group: "g", Status: "ok"},
		{ID: "a", Name: "alpha", Email: "zzz@x.com", Group: "g", Status: "ok"},
		{ID: "b", Name: "beta", Email: "aaa@x.com", Group: "g", Status: "ok"},
	}
	page := Accounts(all, Query{Sort: "account", Limit: 10})
	// 展示名优先邮箱：aaa@x.com < zeta < zzz@x.com
	want := []string{"b", "c", "a"}
	for i, id := range want {
		if page.Items[i].ID != id {
			t.Fatalf("排序第 %d 位 = %s，期望 %s", i, page.Items[i].ID, id)
		}
	}
	// 降序与末位 ID 兜底不应让条目在翻页间跳动。
	page = Accounts(all, Query{Sort: "account", Desc: true, Limit: 10})
	if page.Items[0].ID != "a" {
		t.Fatalf("降序首位 = %s，期望 a", page.Items[0].ID)
	}
	// 未知排序键保持原顺序。
	page = Accounts(all, Query{Sort: "nope", Limit: 10})
	if page.Items[0].ID != "c" {
		t.Fatalf("未知排序键应保序，首位 = %s", page.Items[0].ID)
	}
}

// TestAccountsSortByQuotaIsNumeric 验证额度按数值比较而不是字符串。
func TestAccountsSortByQuotaIsNumeric(t *testing.T) {
	all := []*model.Account{
		{ID: "a", Status: "ok", Theos: &model.Allowance{Balance: "10"}},
		{ID: "b", Status: "ok", Theos: &model.Allowance{Balance: "9"}},
		{ID: "c", Status: "ok", Theos: &model.Allowance{Balance: "unknown"}},
	}
	page := Accounts(all, Query{Sort: "theos", Limit: 10})
	// "unknown" → -1 排最前；9 < 10（字符串比较会得到 "10" < "9"）。
	want := []string{"c", "b", "a"}
	for i, id := range want {
		if page.Items[i].ID != id {
			t.Fatalf("第 %d 位 = %s，期望 %s（按数值排序）", i, page.Items[i].ID, id)
		}
	}
}

// BenchmarkAccountsPage600 是控制台账号分页的基线。
//
// 600 个号、每页 50 行：只应克隆 50 个账号，而不是整池。
func BenchmarkAccountsPage600(b *testing.B) {
	all := make([]*model.Account, 0, 600)
	for i := 0; i < 600; i++ {
		all = append(all, &model.Account{
			ID: "acc-" + strconv.Itoa(i), Name: "n" + strconv.Itoa(i),
			Status: "ok", LastUsed: int64(i),
			Theos: &model.Allowance{Balance: "10"},
		})
	}
	q := Query{Offset: 100, Limit: 50, Sort: "last_used", Desc: true}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		page := Accounts(all, q)
		if len(page.Items) != 50 {
			b.Fatalf("应返回 50 条，实际 %d", len(page.Items))
		}
	}
}

func TestAccountPageSorting(t *testing.T) {
	quota := []string{"5", "0", "9", ""}
	all := make([]*model.Account, 0, 4)
	for i := 0; i < 4; i++ {
		a := &model.Account{
			ID: "id" + strconv.Itoa(i), APIKey: "sk-" + strconv.Itoa(i),
			Email: "u" + strconv.Itoa(i) + "@x.com", Group: "g1", Enabled: true,
			Failures: i, Status: "ok",
		}
		if quota[i] != "" {
			a.Theos = &model.Allowance{Enabled: true, Balance: quota[i]}
		}
		all = append(all, a)
	}

	// theos 升序：未知(-1) 在最前，然后 0、5、9。
	page := Accounts(all, Query{Sort: "theos"})
	got := []string{}
	for _, a := range page.Items {
		got = append(got, a.ID)
	}
	want := []string{"id3", "id1", "id0", "id2"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("theos 升序 = %v，期望 %v", got, want)
		}
	}

	// failures 降序。
	page = Accounts(all, Query{Sort: "failures", Desc: true})
	if page.Items[0].ID != "id3" {
		t.Fatalf("failures 降序首条应为 id3，实际 %s", page.Items[0].ID)
	}

	// 分页稳定：逐页取回拼起来必须等于一次性取回。
	full := Accounts(all, Query{Sort: "account"})
	var paged []string
	for off := 0; off < full.Total; off += 2 {
		p := Accounts(all, Query{Offset: off, Limit: 2, Sort: "account"})
		for _, a := range p.Items {
			paged = append(paged, a.ID)
		}
	}
	for i, a := range full.Items {
		if paged[i] != a.ID {
			t.Fatalf("分页结果不一致：分页 %v", paged)
		}
	}

	// 未知排序字段不应改变结果，也不应 panic。
	if p := Accounts(all, Query{Sort: "nope"}); p.Total != 4 {
		t.Fatalf("未知排序字段：total=%d", p.Total)
	}
}
