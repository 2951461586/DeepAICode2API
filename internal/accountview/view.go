// Package accountview 是控制台账号列表的「查询视图」：过滤 / 排序 / 分页
// 以及展示用的字段投影（label / plan / 额度数值）。
//
// 为什么从 internal/store 拆出来：这些是**展示层**规则 ——
//   - 排序键 "account" 用的是「邮箱优先、其次名称」的展示名；
//   - 套餐标签要与前端一致（pro / free）；
//   - 额度要按数值比较，而不是按字符串。
//
// 它们与「怎么落盘、怎么加锁」无关，却和持久化代码混在同一个 622 行的文件里，
// 于是改一个排序键也要动存储层。现在 store 只出快照，本包只做投影。
package accountview

import (
	"sort"
	"strconv"
	"strings"

	"deepaicode2api/internal/model"
)

// Query 描述一次账号列表查询。
type Query struct {
	Offset int
	Limit  int
	Group  string
	Status string
	Query  string
	// Sort 取 account / group / source / plan / theos / status / failures / last_used；
	// 空串表示保持号池原顺序。
	Sort string
	// Desc 为 true 时降序。
	Desc bool
}

// Result 是一次账号分页查询的结果。
type Result struct {
	Items []*model.Account
	Total int // 过滤后的总数（未分页）
}

// Accounts 按条件过滤、排序后分页返回账号快照（纯函数，不持锁）。
//
// 控制台一屏只渲染几十行，而全量账号快照动辄几百 KB；
// 分页把传输量从 O(全部账号) 降到 O(单页)。
//
// 排序必须在这里做而不是前端：列表是服务端分页的，只排当前页会得到
// 错误结果（第 2 页的第一条可能比第 1 页的最后一条还小）。
// 相等值统一按 ID 兜底，保证翻页时条目不会在页之间跳动。
//
// offset<0 视为 0；limit<=0 用默认 50；limit>200 封顶 200。
func Accounts(all []*model.Account, q Query) Result {
	offset := q.Offset
	limit := q.Limit
	if offset < 0 {
		offset = 0
	}
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	group := strings.TrimSpace(q.Group)
	status := strings.ToLower(strings.TrimSpace(q.Status))
	query := strings.ToLower(strings.TrimSpace(q.Query))

	matched := make([]*model.Account, 0, len(all))
	for _, a := range all {
		if group != "" && !strings.EqualFold(a.GroupName(), group) {
			continue
		}
		if status != "" && !strings.EqualFold(a.Status, status) {
			continue
		}
		if query != "" && !matches(a, query) {
			continue
		}
		matched = append(matched, a)
	}

	sortBy(matched, q.Sort, q.Desc)

	total := len(matched)
	if offset >= total {
		return Result{Items: nil, Total: total}
	}
	end := offset + limit
	if end > total {
		end = total
	}
	items := make([]*model.Account, 0, end-offset)
	for _, a := range matched[offset:end] {
		items = append(items, a.Clone())
	}
	return Result{Items: items, Total: total}
}

// sortBy 按指定字段排序；未知字段保持原顺序。
func sortBy(xs []*model.Account, key string, desc bool) {
	key = strings.ToLower(strings.TrimSpace(key))
	if key == "" || len(xs) < 2 {
		return
	}
	cmp, ok := comparator(key)
	if !ok {
		return
	}
	sort.SliceStable(xs, func(i, j int) bool {
		c := cmp(xs[i], xs[j])
		if c == 0 {
			return xs[i].ID < xs[j].ID // 稳定兜底
		}
		if desc {
			return c > 0
		}
		return c < 0
	})
}

// accountComparator 返回某字段的升序比较函数；未知字段返回 ok=false。
func comparator(key string) (func(a, b *model.Account) int, bool) {
	cmpStr := func(a, b string) int { return strings.Compare(a, b) }
	cmpInt := func(a, b int64) int {
		switch {
		case a < b:
			return -1
		case a > b:
			return 1
		default:
			return 0
		}
	}
	switch key {
	case "account", "name", "email":
		return func(a, b *model.Account) int { return cmpStr(accountLabel(a), accountLabel(b)) }, true
	case "group":
		return func(a, b *model.Account) int { return cmpStr(a.GroupName(), b.GroupName()) }, true
	case "source":
		return func(a, b *model.Account) int { return cmpStr(a.Source, b.Source) }, true
	case "plan":
		return func(a, b *model.Account) int { return cmpStr(accountPlanLabel(a), accountPlanLabel(b)) }, true
	case "theos":
		return func(a, b *model.Account) int { return cmpInt(quotaValue(a), quotaValue(b)) }, true
	case "status":
		return func(a, b *model.Account) int { return cmpStr(a.Status, b.Status) }, true
	case "failures":
		return func(a, b *model.Account) int {
			return cmpInt(int64(a.Failures), int64(b.Failures))
		}, true
	case "last_used":
		return func(a, b *model.Account) int { return cmpInt(a.LastUsed, b.LastUsed) }, true
	}
	return nil, false
}

// accountLabel 是账号的展示名（优先邮箱，其次名称，最后 ID）。
func accountLabel(a *model.Account) string {
	if a == nil {
		return ""
	}
	switch {
	case a.Email != "":
		return a.Email
	case a.Name != "":
		return a.Name
	default:
		return a.ID
	}
}

// accountPlanLabel 是套餐的展示名（与前端一致）。
func accountPlanLabel(a *model.Account) string {
	if a == nil {
		return ""
	}
	if a.IsPro {
		return "pro"
	}
	if a.Plan != "" {
		return a.Plan
	}
	return "free"
}

// quotaValue 把 Theos 余额解析成数值；未知/解析不了按 -1（排在最前，升序）。
func quotaValue(a *model.Account) int64 {
	if a == nil || a.Theos == nil {
		return -1
	}
	v, err := strconv.ParseFloat(strings.TrimSpace(a.Theos.Balance), 64)
	if err != nil {
		return -1
	}
	return int64(v * 1000) // 保留 3 位小数，避免浮点在比较里的抖动
}

// accountMatches 在 ID / 名称 / 邮箱 / 用户名 / 分组 / 代理上做子串匹配。
func matches(a *model.Account, kw string) bool {
	for _, field := range []string{a.ID, a.Name, a.Email, a.Username, a.GroupName(), a.Proxy, a.Fingerprint} {
		if strings.Contains(strings.ToLower(field), kw) {
			return true
		}
	}
	return false
}
