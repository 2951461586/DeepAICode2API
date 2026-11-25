package poolsvc

import (
	"io"
	"sync"

	"deepaicode2api/internal/deepai"
	"deepaicode2api/internal/pool"
)

// MaxInFlightPerAccount 是单账号同时进行的上游请求上限。
//
// 调度只决定「这一轮选到谁」，不保证并发不会集中到少数账号；
// 实测 12 并发时可能全落在 1~2 个号上，一碰上上游限流就是长尾。
const MaxInFlightPerAccount = 2

// Pick 扇一圈号池，找一个还有在途槽位的账号。
//
// prefer 非空时先试那个账号（会话粘性，见 sticky.go）：它仍然可调度且有
// 空槽就用它。失败（被拦 / 额度用尽 / 并发已满）静默回退常规轮询 ——
// 粘性永远不是硬约束。
//
// 不能用 max_retries 次贪心选号：饱和时几次尝试大概率全撞在忙号上，
// 于是对外 429 而池里其实还有空槽（docs/performance.md §14）。
// 返回值第三个标记池里是否有可选账号：区分「全忙」与「没有可调度账号」，
// 后者要交给调用方给出可行动的原因（social_login / 未验证…）。
//
// 实现要点：选号在 store 里一次读完（只拷贝被选中的那个），忙号通过纯谓词
// 排除；Acquire 只处理「检查到占用」的窄竞态，失败就排除后重试。
func (p *Service) Pick(group, prefer string) (*deepai.Account, bool, bool) {
	return p.pickWith(group, prefer, nil, false)
}

// PickAvoiding 先在没有被 avoid 排除的候选里选号；若这样会挖空整个池
// （或者干脆没有候选），退回到「不考虑 avoid」的常规调度。
//
// 典型用途：某个出口代理正在冷却（见 proxy.Health），优先换到别的号；
// 但号池只剩那一个可用号时仍然要用它 —— 宁可走一个已知慢的出口，
// 也不要对外回 429（线上 2026-11-22：16 个号各绑一个住宅代理，
// 经常同时有两三个出口在超时）。
func (p *Service) PickAvoiding(group, prefer string, avoid func(*deepai.Account) bool) (*deepai.Account, bool, bool) {
	return p.PickAvoidingFor(group, prefer, avoid, false)
}

// PickAvoidingFor 是 PickAvoiding 的严格版：requireQuota 为 true 时
// 余额已确认非正的账号既不参与常规调度，也不接受粘性命中。
//
// 付费模型（theosPerRequest > 0）必须走这条路：否则一个负余额账号仍会被
// 选上（旧版本会“先选有额度的、都没有就全集兜底”），每个请求白撞一次
// 上游 403 theos_exhausted。
func (p *Service) PickAvoidingFor(group, prefer string, avoid func(*deepai.Account) bool, requireQuota bool) (*deepai.Account, bool, bool) {
	if avoid != nil {
		if a, ok, candidates := p.pickWith(group, prefer, avoid, requireQuota); ok || !candidates {
			return a, ok, candidates
		}
	}
	return p.pickWith(group, prefer, nil, requireQuota)
}

// pickWith 是 Pick 的实现体，avoid 非 nil 时额外排除一批账号。
func (p *Service) pickWith(group, prefer string, avoid func(*deepai.Account) bool, requireQuota bool) (*deepai.Account, bool, bool) {
	if prefer != "" {
		if a := p.store.PreferredAccount(prefer, group); a != nil && (avoid == nil || !avoid(a)) &&
			(!requireQuota || pool.HasQuota(a)) && p.Acquire(a.ID) {
			return a, true, true
		}
	}
	// excluded 用切片而不是 map：重试只在「检查与占用撞车」时发生，通常 0~1 次，
	// 切片比 map 少一次分配，而选号是每个请求一次的热路径。
	var excluded []string
	skip := func(a *deepai.Account) bool {
		for _, id := range excluded {
			if id == a.ID {
				return true
			}
		}
		if avoid != nil && avoid(a) {
			return true
		}
		return p.Busy(a.ID)
	}
	for {
		a, candidates := p.store.PickAccountFor(group, skip, requireQuota)
		if a == nil {
			return nil, false, candidates
		}
		if p.Acquire(a.ID) {
			return a, true, true
		}
		// 竞态：谓词判定空闲后、占用前被别人抢满。排除它再看下一个。
		excluded = append(excluded, a.ID)
	}
}

// Busy 报告账号的在途槽位是否已满（纯读，不占用）。
func (p *Service) Busy(id string) bool {
	p.inflightMu.Lock()
	defer p.inflightMu.Unlock()
	return p.inflight[id] >= MaxInFlightPerAccount
}

// Acquire 尝试占用一个账号的在途槽位；已满时返回 false。
func (p *Service) Acquire(id string) bool {
	p.inflightMu.Lock()
	defer p.inflightMu.Unlock()
	if p.inflight == nil {
		p.inflight = map[string]int{}
	}
	if p.inflight[id] >= MaxInFlightPerAccount {
		return false
	}
	p.inflight[id]++
	return true
}

// Release 归还一个在途槽位。
func (p *Service) Release(id string) {
	p.inflightMu.Lock()
	defer p.inflightMu.Unlock()
	if p.inflight[id] <= 0 {
		delete(p.inflight, id)
		return
	}
	p.inflight[id]--
	if p.inflight[id] == 0 {
		delete(p.inflight, id)
	}
}

// InFlight 返回当前占用在途槽位的账号数（控制台 / 测试观测用）。
func (p *Service) InFlight() int {
	p.inflightMu.Lock()
	defer p.inflightMu.Unlock()
	return len(p.inflight)
}

// HasFree 报告池里是否还有账号有空在途槽位。
//
// 409 之后用它区分「立即换号」与「整池都满，等一等」：以前一律先
// sleep(Retry-After) 再换号，最坏白等 30s，还把非流式预算（90s）吃掉。
// 只做一次指针扫描 + map 查询，不复制整个号池。
func (p *Service) HasFree() bool {
	return p.store.AnySchedulable(func(a *deepai.Account) bool {
		return !p.Busy(a.ID)
	})
}

// releaseBody 在响应体关闭时归还账号的在途配额。
//
// 所有上游调用点都以 defer resp.Body.Close() 收尾，所以槽位会随 Body 释放：
// 流式响应可能持续数分钟，不能在建连成功时就归还。
type releaseBody struct {
	io.ReadCloser
	once    sync.Once
	release func()
}

func (b *releaseBody) Close() error {
	b.once.Do(b.release)
	return b.ReadCloser.Close()
}

// WrapBody 把上游响应体包一层：Close 时归还该账号的在途槽位。
func (p *Service) WrapBody(rc io.ReadCloser, accountID string) io.ReadCloser {
	if rc == nil {
		return nil
	}
	return &releaseBody{ReadCloser: rc, release: func() { p.Release(accountID) }}
}
