package proxy

import (
	"sync"
	"time"
)

// Health 是出口代理的健康台账：连续失败按指数退避进入冷却窗口，
// 一次成功立刻清零。进程内状态（重启即清空），不做持久化——
// 代理好坏是分钟级的运行时事实，落盘反而会因为重启前的旧结论误伤。
//
// 用途：调度时把「正在冷却的出口」排到候选末尾（poolsvc.PickAvoiding），
// 而不是把账号本身标黑。线上 2026-11-22 的形态是 16 个账号各绑一个住宅
// 代理，故障表现为代理侧 504 / unexpected EOF（账号本身没问题），
// 所以需要「换出口优先、弃号其次」的次序。
type Health struct {
	mu      sync.Mutex
	entries map[string]*healthEntry

	// Base / Max / Now 可被测试注入；零值用默认。
	Base time.Duration
	Max  time.Duration
	Now  func() time.Time
}

type healthEntry struct {
	fails int
	until time.Time
}

const (
	// defaultHealthBase 是首次失败的冷却时长。
	defaultHealthBase = 30 * time.Second
	// defaultHealthMax 是连续失败退避的上限。
	defaultHealthMax = 10 * time.Minute
)

// NewHealth 返回空的健康台账。
func NewHealth() *Health { return &Health{entries: map[string]*healthEntry{}} }

func (h *Health) now() time.Time {
	if h != nil && h.Now != nil {
		return h.Now()
	}
	return time.Now()
}

func (h *Health) base() time.Duration {
	if h.Base > 0 {
		return h.Base
	}
	return defaultHealthBase
}

func (h *Health) max() time.Duration {
	if h.Max > 0 {
		return h.Max
	}
	return defaultHealthMax
}

// Report 记录一次出口结果：ok=false 累加失败并延长冷却，ok=true 清零。
//
// key 用原始代理串（含凭据），保证「同主机不同凭据」的口径不会互相污染。
func (h *Health) Report(key string, ok bool) {
	if h == nil || key == "" {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.entries == nil {
		h.entries = map[string]*healthEntry{}
	}
	e := h.entries[key]
	if e == nil {
		e = &healthEntry{}
		h.entries[key] = e
	}
	if ok {
		e.fails = 0
		e.until = time.Time{}
		return
	}
	e.fails++
	d := h.base() << (e.fails - 1)
	if d > h.max() || d <= 0 { // 溢出或超过上限
		d = h.max()
	}
	e.until = h.now().Add(d)
}

// Cooling 报告出口是否还在冷却窗口内。
func (h *Health) Cooling(key string) bool {
	if h == nil || key == "" {
		return false
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	e := h.entries[key]
	if e == nil || e.until.IsZero() {
		return false
	}
	return h.now().Before(e.until)
}

// Fails 返回出口当前的连续失败次数（诊断用）。
func (h *Health) Fails(key string) int {
	if h == nil || key == "" {
		return 0
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if e := h.entries[key]; e != nil {
		return e.fails
	}
	return 0
}

// CoolingCount 返回当前处于冷却中的出口数（概览用）。
func (h *Health) CoolingCount() int {
	if h == nil {
		return 0
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	now := h.now()
	n := 0
	for _, e := range h.entries {
		if !e.until.IsZero() && now.Before(e.until) {
			n++
		}
	}
	return n
}
