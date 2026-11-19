package server

import (
	"log"
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
	// stickyTTL 是绑定的闲置有效期。
	//
	// 比前缀缓存的实际存活时间保守：会话超过 10 分钟没有请求，多半已经换了
	// 上下文，继续钉在旧号上只会让负载集中。
	stickyTTL = 10 * time.Minute
	// stickyMax 是绑定表规模上限（按 affinity 计）。
	//
	// 超出后先回收过期项；仍然超限就整表清空 —— 绑定只是优化，
	// 清空等价于「重新学一遍」，不会造成正确性问题。
	stickyMax = 2048
)

// loadSticky 从旁路文件恢复绑定表（启动时调用一次）。
//
// 文件不存在 / 解析失败都只记日志：绑定是缓存，丢了不影响服务。
// 过期项在这里就剔掉，避免重启后把陈旧会话钉回旧账号。
func (s *Server) loadSticky() {
	m, err := s.Store.LoadSticky()
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
		if now.Sub(time.Unix(b.At, 0)) > stickyTTL {
			continue
		}
		alive[k] = b
	}
	s.stickyMu.Lock()
	s.sticky = alive
	s.stickyMu.Unlock()
	if len(alive) > 0 {
		log.Printf("[sticky] 已恢复 %d 条会话绑定（丢弃过期 %d 条）", len(alive), len(m)-len(alive))
	}
}

// stickyAccount 返回 affinity 当前绑定的账号 ID（无绑定 / 已过期时返回空串）。
func (s *Server) stickyAccount(affinity string) string {
	if affinity == "" {
		return ""
	}
	s.stickyMu.Lock()
	defer s.stickyMu.Unlock()
	b, ok := s.sticky[affinity]
	if !ok {
		return ""
	}
	if time.Since(time.Unix(b.At, 0)) > stickyTTL {
		delete(s.sticky, affinity)
		s.stickyDirty.Store(true)
		return ""
	}
	return b.AccountID
}

// bindSticky 把 affinity 绑定到刚成功的账号。
func (s *Server) bindSticky(affinity, accountID string) {
	if affinity == "" || accountID == "" {
		return
	}
	s.stickyMu.Lock()
	defer s.stickyMu.Unlock()
	if s.sticky == nil {
		s.sticky = map[string]store.StickyBinding{}
	}
	if _, exists := s.sticky[affinity]; !exists && len(s.sticky) >= stickyMax {
		s.gcStickyLocked()
	}
	s.sticky[affinity] = store.StickyBinding{AccountID: accountID, At: time.Now().Unix()}
	s.stickyDirty.Store(true)
}

// dropSticky 在账号被判死（拦截 / 额度用尽）时解除绑定。
//
// 只在绑定确实指向这个号时才删：重试链上可能已经把会话改绑到别的号，
// 那时删掉会误伤新的绑定。
func (s *Server) dropSticky(affinity, accountID string) {
	if affinity == "" {
		return
	}
	s.stickyMu.Lock()
	defer s.stickyMu.Unlock()
	if b, ok := s.sticky[affinity]; ok && b.AccountID == accountID {
		delete(s.sticky, affinity)
		s.stickyDirty.Store(true)
	}
}

// gcStickyLocked 回收过期绑定；仍然超限就整表清空。调用方必须持有 stickyMu。
func (s *Server) gcStickyLocked() {
	now := time.Now()
	for k, b := range s.sticky {
		if now.Sub(time.Unix(b.At, 0)) > stickyTTL {
			delete(s.sticky, k)
		}
	}
	if len(s.sticky) >= stickyMax {
		clear(s.sticky)
	}
}

// FlushSticky 把绑定表写进旁路文件（无改动时跳过）。
//
// 由后台 30s 周期与 Close 调用：绑定变化频率远低于请求频率，攒批写盘即可。
func (s *Server) FlushSticky() {
	if !s.stickyDirty.Swap(false) {
		return
	}
	s.stickyMu.Lock()
	snap := make(map[string]store.StickyBinding, len(s.sticky))
	for k, v := range s.sticky {
		snap[k] = v
	}
	s.stickyMu.Unlock()
	if err := s.Store.SaveSticky(snap); err != nil {
		// 写失败不改变服务语义，但要留痕并让下一轮重试。
		s.stickyDirty.Store(true)
		log.Printf("[sticky] 保存绑定表失败（下轮重试）: %v", err)
	}
}

// StickyBindings 返回当前绑定数量（控制台 / 测试用）。
func (s *Server) StickyBindings() int {
	s.stickyMu.Lock()
	defer s.stickyMu.Unlock()
	return len(s.sticky)
}
