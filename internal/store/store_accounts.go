package store

import (
	"sort"
	"strconv"
	"strings"
	"time"

	"deepaicode2api/internal/deepai"
)

// --- 账号 ---

// Accounts 返回账号快照列表（每项都是深拷贝）。
func (s *Store) Accounts() []*deepai.Account {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*deepai.Account, len(s.st.Accounts))
	for i, a := range s.st.Accounts {
		out[i] = a.Clone()
	}
	return out
}

// Account 按 ID 查找账号，返回深拷贝快照。
//
// 不返回号池里的活指针：账号状态会被调度、后台探测、管理接口并发改写，
// 把活指针交出去等于把加锁责任推给每个调用方。需要就地改字段时用
// UpdateAccount / UpdateAccounts（在写锁内完成）。
func (s *Store) Account(id string) *deepai.Account {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, a := range s.st.Accounts {
		if a.ID == id {
			return a.Clone()
		}
	}
	return nil
}

// AccountPage 是一次账号分页查询的结果。
type AccountPage struct {
	Items []*deepai.Account
	Total int // 过滤后的总数（未分页）
}

// AccountCounts 返回账号总数与就绪数，不解包快照。
//
// 控制台首屏需要在不同步整个账号列表的情况下展示“就绪/总”，
// 而 Accounts() 会给每个账号做深拷贝。
func (s *Store) AccountCounts() (total, ready int) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, a := range s.st.Accounts {
		if a.Enabled && a.Status != "error" {
			ready++
		}
	}
	return len(s.st.Accounts), ready
}

// AccountQuery 描述一次账号列表查询。
type AccountQuery struct {
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

// AccountPage 按条件过滤、排序后分页返回账号快照。
//
// 控制台一屏只渲染几十行，而全量账号快照动辄几百 KB；
// 分页把传输量从 O(全部账号) 降到 O(单页)。
//
// 排序必须在这里做而不是前端：列表是服务端分页的，只排当前页会得到
// 错误结果（第 2 页的第一条可能比第 1 页的最后一条还小）。
// 相等值统一按 ID 兜底，保证翻页时条目不会在页之间跳动。
//
// offset<0 视为 0；limit<=0 用默认 50；limit>200 封顶 200。
func (s *Store) AccountPage(q AccountQuery) AccountPage {
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

	s.mu.RLock()
	defer s.mu.RUnlock()

	matched := make([]*deepai.Account, 0, len(s.st.Accounts))
	for _, a := range s.st.Accounts {
		if group != "" && !strings.EqualFold(a.GroupName(), group) {
			continue
		}
		if status != "" && !strings.EqualFold(a.Status, status) {
			continue
		}
		if query != "" && !accountMatches(a, query) {
			continue
		}
		matched = append(matched, a)
	}

	sortAccounts(matched, q.Sort, q.Desc)

	total := len(matched)
	if offset >= total {
		return AccountPage{Items: nil, Total: total}
	}
	end := offset + limit
	if end > total {
		end = total
	}
	items := make([]*deepai.Account, 0, end-offset)
	for _, a := range matched[offset:end] {
		items = append(items, a.Clone())
	}
	return AccountPage{Items: items, Total: total}
}

// sortAccounts 按指定字段排序；未知字段保持原顺序。
func sortAccounts(xs []*deepai.Account, key string, desc bool) {
	key = strings.ToLower(strings.TrimSpace(key))
	if key == "" || len(xs) < 2 {
		return
	}
	cmp, ok := accountComparator(key)
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
func accountComparator(key string) (func(a, b *deepai.Account) int, bool) {
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
		return func(a, b *deepai.Account) int { return cmpStr(accountLabel(a), accountLabel(b)) }, true
	case "group":
		return func(a, b *deepai.Account) int { return cmpStr(a.GroupName(), b.GroupName()) }, true
	case "source":
		return func(a, b *deepai.Account) int { return cmpStr(a.Source, b.Source) }, true
	case "plan":
		return func(a, b *deepai.Account) int { return cmpStr(accountPlanLabel(a), accountPlanLabel(b)) }, true
	case "theos":
		return func(a, b *deepai.Account) int { return cmpInt(quotaValue(a), quotaValue(b)) }, true
	case "status":
		return func(a, b *deepai.Account) int { return cmpStr(a.Status, b.Status) }, true
	case "failures":
		return func(a, b *deepai.Account) int {
			return cmpInt(int64(a.Failures), int64(b.Failures))
		}, true
	case "last_used":
		return func(a, b *deepai.Account) int { return cmpInt(a.LastUsed, b.LastUsed) }, true
	}
	return nil, false
}

// accountLabel 是账号的展示名（优先邮箱，其次名称，最后 ID）。
func accountLabel(a *deepai.Account) string {
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
func accountPlanLabel(a *deepai.Account) string {
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
func quotaValue(a *deepai.Account) int64 {
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
func accountMatches(a *deepai.Account, kw string) bool {
	for _, field := range []string{a.ID, a.Name, a.Email, a.Username, a.GroupName(), a.Proxy, a.Fingerprint} {
		if strings.Contains(strings.ToLower(field), kw) {
			return true
		}
	}
	return false
}

// UpdateAccount 在写锁内就地修改一个账号；返回账号是否存在。
func (s *Store) UpdateAccount(id string, fn func(*deepai.Account)) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, a := range s.st.Accounts {
		if a.ID == id {
			fn(a)
			return true
		}
	}
	return false
}

// UpdateAccounts 在写锁内批量遍历全部账号。
//
// fn 返回 true 表示本次改动了该账号；返回值为改动个数。
func (s *Store) UpdateAccounts(fn func(*deepai.Account) bool) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, a := range s.st.Accounts {
		if fn(a) {
			n++
		}
	}
	return n
}

// UpsertAccount 按 API Key 指纹或邮箱去重后写入账号，返回是否为新增。
//
// 同一账号重新登录时 API Key 可能不变，但重新注册/补跑时邮箱相同、
// Key 可能已轮换；按邮箱二次匹配可以避免号池里出现同一账号两份。
func (s *Store) UpsertAccount(a *deepai.Account) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, ex := range s.st.Accounts {
		if ex.KeyID() != a.KeyID() && !sameEmail(ex.Email, a.Email) {
			continue
		}
		a.ID = ex.ID
		// 运行态以旧值优先，身份字段以新值优先（空值回落）——见 MergeFrom。
		a.MergeFrom(ex)
		s.st.Accounts[i] = a.Clone()
		return false
	}
	s.st.Accounts = append(s.st.Accounts, a.Clone())
	return true
}

func sameEmail(a, b string) bool {
	a, b = strings.TrimSpace(a), strings.TrimSpace(b)
	return a != "" && b != "" && strings.EqualFold(a, b)
}

// RemoveAccount 删除账号。
func (s *Store) RemoveAccount(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, a := range s.st.Accounts {
		if a.ID == id {
			s.st.Accounts = append(s.st.Accounts[:i], s.st.Accounts[i+1:]...)
			return true
		}
	}
	return false
}

// NextAccount 以轮询方式挑一个可用账号（全部分组）。
func (s *Store) NextAccount() *deepai.Account { return s.NextAccountGroup("") }

// NextAccountGroup 在指定分组内以轮询方式挑一个可用账号。
//
// group 为空表示全部分组。连续失败过多的账号会被跳过；全组不可用时
// 回退到失败最少的那个，保证不会硬阻塞请求。分组不存在时返回 nil，
// 由调用方给出“该分组没有账号”的明确错误。
//
// 调度分两轮：第一轮只考虑「额度未知或额度 > 0」的账号。上游按机器发放
// 每日免费额度，池里常有 balance=0 的账号（免费额度已耗尽），先打过去只会
// 被上游拒一次再换号；第二轮才在去重后的全集里选，保证不会硬阻塞。
func (s *Store) NextAccountGroup(group string) *deepai.Account {
	s.mu.Lock()
	defer s.mu.Unlock()
	acct := s.sched.Pick(s.st.Accounts, group)
	if acct == nil {
		return nil
	}
	return acct.Clone()
}

// MarkAccountResult 记录一次账号调用结果。
func (s *Store) MarkAccountResult(id string, ok bool, note string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, a := range s.st.Accounts {
		if a.ID != id {
			continue
		}
		a.LastUsed = time.Now().Unix()
		if ok {
			a.Failures = 0
			a.Status = "ok"
			a.LastErr = ""
			if note != "" {
				a.Note = note
			}
		} else {
			a.Failures++
			a.Status = "error"
			a.LastErr = note
		}
		return
	}
}

// SetAccountAllowance 缓存一次配额探测结果。
func (s *Store) SetAccountAllowance(id string, al *deepai.Allowance, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, a := range s.st.Accounts {
		if a.ID != id {
			continue
		}
		a.LastUsed = time.Now().Unix()
		if err != nil {
			a.Status = "error"
			a.LastErr = err.Error()
			a.Failures++
			return
		}
		a.Theos = al
		a.Status = "ok"
		a.LastErr = ""
		a.Failures = 0
		return
	}
}
