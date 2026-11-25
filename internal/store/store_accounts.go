package store

import (
	"strings"
	"time"

	"deepaicode2api/internal/model"
)

// --- 账号 ---

// Accounts 返回账号快照列表（每项都是深拷贝）。
func (s *Store) Accounts() []*model.Account {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*model.Account, len(s.st.Accounts))
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
func (s *Store) Account(id string) *model.Account {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, a := range s.st.Accounts {
		if a.ID == id {
			return a.Clone()
		}
	}
	return nil
}

// PreferredAccount 返回 id 对应账号的副本，前提是它在 group 下仍然可调度。
//
// 用于会话粘性：先把请求送回同一把 Key 上次用的号（上游前缀缓存与会话上下文
// 都在那个号上）。不可调度时返回 nil，调用方回退常规选号。
// 与 PickAccount 同量级开销：一次读锁 + 一个账号的拷贝，不做全池复制。
func (s *Store) PreferredAccount(id, group string) *model.Account {
	if id == "" {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, a := range s.st.Accounts {
		if a == nil || a.ID != id {
			continue
		}
		if !s.sched.Eligible(a, group) {
			return nil
		}
		return a.Clone()
	}
	return nil
}

// AccountCounts 返回账号总数与就绪数，不解包快照。
//
// 控制台首屏需要在不同步整个账号列表的情况下展示“就绪/总”，
// 而 Accounts() 会给每个账号做深拷贝。
//
// “就绪”的口径见 model.AccountReady：error 只在错误还算新鲜时算不健康，
// blocked 永不算就绪（它不会自愈）。旧实现用的 `status != "error"` 会把
// 一次概率性 EOF 永久计成不健康，也会把 blocked 算成就绪。
func (s *Store) AccountCounts() (total, ready int) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	now := time.Now()
	for _, a := range s.st.Accounts {
		if model.AccountReady(a, now) {
			ready++
		}
	}
	return len(s.st.Accounts), ready
}

// UpdateAccount 在写锁内就地修改一个账号；返回账号是否存在。
func (s *Store) UpdateAccount(id string, fn func(*model.Account)) bool {
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
func (s *Store) UpdateAccounts(fn func(*model.Account) bool) int {
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
func (s *Store) UpsertAccount(a *model.Account) bool {
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

// PickAccount 在指定分组内以轮询方式挑一个可用账号。
//
// 调度规则（分组 / 额度 / 失败加权 / 轮询）仍在 internal/pool；这里只负责
// 「一次读锁 + 一次扫描」。
//
// 每个请求一次的热路径：这里只做一次读锁 + 一次扫描，**只深拷贝被选中的账号**。
// 旧版由调用方循环 "Accounts()（全池深拷贝）+ NextAccountGroup（写锁 + 再拷贝）"
// 实现，653 个号时是 O(n²) 次拷贝、每轮一次写锁。
//
// skip 为可选的临时排除谓词（如“并发槽位已满”），必须是纯函数。
// 返回值第二个标记“池里是否有可调度账号（忽略 skip）”，
// 供调用方区分“全忙”与“根本没号”。
func (s *Store) PickAccount(group string, skip func(*model.Account) bool) (*model.Account, bool) {
	return s.PickAccountFor(group, skip, false)
}

// PickAccountFor 是 PickAccount 的严格版：requireQuota 为 true 时不接受
// 「额度已确认非正」的账号（付费模型专用，见 pool.PickFilteredFor）。
func (s *Store) PickAccountFor(group string, skip func(*model.Account) bool, requireQuota bool) (*model.Account, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	acct := s.sched.PickFilteredFor(s.st.Accounts, group, skip, requireQuota)
	if acct == nil {
		return nil, s.sched.HasEligibleFor(s.st.Accounts, group, requireQuota)
	}
	return acct.Clone(), true
}

// AnySchedulable 报告是否存在满足 ok 的可调度账号（启用、未被拦截），
// 不解包快照、不深拷贝。
func (s *Store) AnySchedulable(ok func(*model.Account) bool) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, a := range s.st.Accounts {
		if a == nil || !a.Enabled || a.Status == model.StatusBlocked {
			continue
		}
		if ok == nil || ok(a) {
			return true
		}
	}
	return false
}

// NextAccountGroup 兼容旧调用点：等价于 PickAccount(group, nil) 的成功分支。
func (s *Store) NextAccountGroup(group string) *model.Account {
	s.mu.RLock()
	defer s.mu.RUnlock()
	acct := s.sched.Pick(s.st.Accounts, group)
	if acct == nil {
		return nil
	}
	return acct.Clone()
}
