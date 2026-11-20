package poolsvc

import (
	"log"
	"maps"
	"time"

	"deepaicode2api/internal/store"
)

// 会话粘性：把 affinity 绑定到具体账号。
//
// 背景：affinity（`X-Session-Affinity` / body 里的 `prompt_cache_key`）此前只作为
// 请求头透传给上游，选号仍然只看分组。池子里有几百个号时，同一会话的连续请求
// 会落到不同账号上；而上游的前缀缓存是**按前缀分区**的（实测：同账号同一段
// 2687 token 前缀，第二次 `cached_tokens=2560`；换一个 `prompt_cache_key` 立刻
// miss）。账号一换，缓存与上游会话上下文就等于每次都冷启动。
//
// 策略：命中绑定且那个号仍然可调度 → 先试它；绑定失效（号被拦 / 额度用尽 /
// 并发已满 / 超过 TTL）→ 回退常规轮询并重新绑定。只影响「先试谁」，
// 不影响可用性：绑定永远不是硬约束。
//
// 落盘：绑定表放独立的旁路文件（store.SaveSticky），随重启保留；进程内改动
// 先只标脏，由后台 30s 周期与退出流程统一 flush，避免每个请求一次写盘。
const (
	// StickyTTL 是绑定的闲置有效期。
	//
	// 比前缀缓存的实际存活时间保守：会话超过 10 分钟没有请求，多半已经换了
	// 上下文，继续钉在旧号上只会让负载集中。
	StickyTTL = 10 * time.Minute
	// StickyMax 是绑定表规模上限（按 affinity 计）。
	//
	// 超出后先回收过期项；仍然超限就整表清空 —— 绑定只是优化，
	// 清空等价于「重新学一遍」，不会造成正确性问题。
	StickyMax = 2048
)

// LoadSticky 从旁路文件恢复绑定表（启动时调用一次）。
//
// 文件不存在 / 解析失败都只记日志：绑定是缓存，丢了不影响服务。
// 过期项在这里就剔掉，避免重启后把陈旧会话钉回旧账号。
func (p *Service) LoadSticky() {
	m, err := p.store.LoadSticky()
	if err != nil {
		log.Printf("[sticky] 读取绑定表失败（按空表启动）: %v", err)
		return
	}
	now := time.Now()
	alive := make(map[string]store.StickyBinding, len(m))
	for k, b := range m {
		if k == "" || b.AccountID == "" {
			continue
		}
		if now.Sub(time.Unix(b.At, 0)) > StickyTTL {
			continue
		}
		alive[k] = b
	}
	p.stickyMu.Lock()
	p.sticky = alive
	p.stickyMu.Unlock()
	if len(alive) > 0 {
		log.Printf("[sticky] 已恢复 %d 条会话绑定（丢弃过期 %d 条）", len(alive), len(m)-len(alive))
	}
}

// StickyAccount 返回 affinity 当前绑定的账号 ID（无绑定 / 已过期时返回空串）。
func (p *Service) StickyAccount(affinity string) string {
	if affinity == "" {
		return ""
	}
	p.stickyMu.Lock()
	defer p.stickyMu.Unlock()
	b, ok := p.sticky[affinity]
	if !ok {
		return ""
	}
	if time.Since(time.Unix(b.At, 0)) > StickyTTL {
		delete(p.sticky, affinity)
		p.stickyDirty.Store(true)
		return ""
	}
	return b.AccountID
}

// BindSticky 把 affinity 绑定到刚成功的账号。
func (p *Service) BindSticky(affinity, accountID string) {
	if affinity == "" || accountID == "" {
		return
	}
	p.stickyMu.Lock()
	defer p.stickyMu.Unlock()
	if p.sticky == nil {
		p.sticky = map[string]store.StickyBinding{}
	}
	if _, exists := p.sticky[affinity]; !exists && len(p.sticky) >= StickyMax {
		p.gcStickyLocked()
	}
	p.sticky[affinity] = store.StickyBinding{AccountID: accountID, At: time.Now().Unix()}
	p.stickyDirty.Store(true)
}

// DropSticky 在账号被判死（拦截 / 额度用尽）时解除绑定。
//
// 只在绑定确实指向这个号时才删：重试链上可能已经把会话改绑到别的号，
// 那时删掉会误伤新的绑定。
func (p *Service) DropSticky(affinity, accountID string) {
	if affinity == "" {
		return
	}
	p.stickyMu.Lock()
	defer p.stickyMu.Unlock()
	if b, ok := p.sticky[affinity]; ok && b.AccountID == accountID {
		delete(p.sticky, affinity)
		p.stickyDirty.Store(true)
	}
}

// gcStickyLocked 回收过期绑定；仍然超限就整表清空。调用方必须持有 stickyMu。
func (p *Service) gcStickyLocked() {
	now := time.Now()
	for k, b := range p.sticky {
		if now.Sub(time.Unix(b.At, 0)) > StickyTTL {
			delete(p.sticky, k)
		}
	}
	if len(p.sticky) >= StickyMax {
		clear(p.sticky)
	}
}

// FlushSticky 把绑定表写进旁路文件（无改动时跳过）。
//
// 由后台 30s 周期与 Close 调用：绑定变化频率远低于请求频率，攒批写盘即可。
func (p *Service) FlushSticky() {
	if !p.stickyDirty.Swap(false) {
		return
	}
	p.stickyMu.Lock()
	snap := maps.Clone(p.sticky)
	p.stickyMu.Unlock()
	if err := p.store.SaveSticky(snap); err != nil {
		// 写失败不改变服务语义，但要留痕并让下一轮重试。
		p.stickyDirty.Store(true)
		log.Printf("[sticky] 保存绑定表失败（下轮重试）: %v", err)
	}
}

// Bindings 返回当前绑定数量（控制台 / 测试用）。
func (p *Service) Bindings() int {
	p.stickyMu.Lock()
	defer p.stickyMu.Unlock()
	return len(p.sticky)
}
