package server

import (
	"context"
	"sync"
	"time"

	"deepaicode2api/internal/deepai"
	"deepaicode2api/internal/logx"
)

// warmCheckInBudget 是后台预热的单次预算。
//
// 比热路径长（热路径线上设 2.5s）：预热不在任何请求的关键路径上，给足时间
// 才能真正把 nonce 回传成功。2026-11-24 线上从 10s 调到 20s：住宅代理
// **冷启动**实测 6–8s（见 proxyhealth.go 里 proxyProbeTimeout 的注释），再加
// TLS 与上游 RTT，10s 预算下预热每一轮都恰好把预算打满（线上 18 分钟里
// 16/16 全超时，ms≈10002）；而热路径 / 手动探测用的是已建立的连接，0.5–1s
// 就回来了。宁可让预热单次多等，也别让它每 30s 永远失败一次。
const warmCheckInBudget = 20 * time.Second

// warmCheckInDefault 是每次预热最多处理的账号数。
//
// 16 个号、半衰期 seconds/2（默认 300s）意味着稳态下每分钟只有 1~2 个号到期，
// 上限 4 已经足够把「到期」提前消化掉，又不会在启动瞬间把代理打满。
const warmCheckInDefault = 4

// 预热失败后的退避参数：连续失败 1/2/3/4/5+ 次 → 2m / 4m / 8m / 16m / 20m。
//
// 为什么需要退避：「签到到期」不是「上游不可用」，一条冷启动慢的出口如果每
// 30s 重试一次，等于每分钟白打一次 /theos 并刷一条日志。退避只影响**预热**——
// 热路径的 EnsureCheckIn 照旧按自己的预算尝试，语义不变。
const (
	warmCooldownBase       = 2 * time.Minute
	warmCooldownMax        = 20 * time.Minute
	warmCooldownMaxEntries = 4096
)

// warmCooldown 是「预热失败退避表」（按账号 ID）。
//
// 零值可用（map 懒创建）。只记连续失败次数，退避窗口由次数推出来；
// 成功一次即删条目，下次到期照常预热。
type warmCooldown struct {
	mu sync.Mutex
	m  map[string]warmCooldownEntry
}

type warmCooldownEntry struct {
	fails int
	until time.Time
}

// allow 报告该账号此刻是否允许预热（不在退避窗口内）。
func (c *warmCooldown) allow(id string, now time.Time) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.m[id]
	return !ok || !now.Before(e.until)
}

// note 记录一次预热结果：成功清零，失败按 warmCooldownBase×2^(n-1) 退避。
func (c *warmCooldown) note(id string, ok bool, now time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if ok {
		delete(c.m, id) // nil map 上 delete 是空操作
		return
	}
	if c.m == nil {
		c.m = make(map[string]warmCooldownEntry)
	}
	if len(c.m) >= warmCooldownMaxEntries {
		c.evictLocked(now)
	}
	e := c.m[id]
	e.fails++
	d := warmCooldownBase
	for i := 1; i < e.fails && d < warmCooldownMax; i++ {
		d *= 2
	}
	if d > warmCooldownMax {
		d = warmCooldownMax
	}
	e.until = now.Add(d)
	c.m[id] = e
}

// fails 返回连续失败次数（0 = 健康）。
func (c *warmCooldown) fails(id string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.m[id].fails
}

// evictLocked 清理已过期的条目；清完仍然塞满说明号池规模异常，直接重置。
func (c *warmCooldown) evictLocked(now time.Time) {
	for k, e := range c.m {
		if !now.Before(e.until) {
			delete(c.m, k)
		}
	}
	if len(c.m) >= warmCooldownMaxEntries {
		c.m = make(map[string]warmCooldownEntry)
	}
}

// WarmCheckIns 在后台为「签到到期」的账号补一次 /theos（热路径预热）。
//
// 为什么值得做：签到是推理前的同步调用（nonce 回传是上游判定真实客户端的
// 依据，不能挪到推理之后），线上 2026-11-22 的 37 小时里有 940 次请求被它
// 阻塞、最长吃满 10s 预算。把「到期」提前在后台消化掉之后，热路径仍然调用
// EnsureCheckIn（保证语义不变），但绝大多数情况会因为在半衰期内直接返回。
//
// 语义边界：
//   - 只对已经启用的账号做，不碰 blocked / 未验证的号（预热不该制造额外风控）；
//   - 失败静默：预热是纯优化，不写账号错误标记，也不影响任何请求；
//   - 与热路径共用 EnsureCheckIn 的判据与幂等锁，不会重复打 /theos；
//   - 连续失败（典型是出口冷启动吃满预算）后按 s.warm 退避，不再每轮重试。
func (s *Server) WarmCheckIns(ctx context.Context, limit, workers int) int {
	if s == nil || s.Store == nil {
		return 0
	}
	if limit <= 0 {
		limit = warmCheckInDefault
	}
	if workers <= 0 {
		workers = 2
	}
	if workers > limit {
		workers = limit
	}

	type task struct {
		acct   *deepai.Account
		client *deepai.Client
	}
	var pending []task
	now := time.Now()
	for _, a := range s.Store.Accounts() {
		if a == nil || !a.Enabled {
			continue
		}
		// 退避中：这一轮不碰上游，省下一次注定超时的 /theos 和一条日志。
		if !s.warm.allow(a.ID, now) {
			continue
		}
		c := s.clientFor(a)
		if !c.CheckInDue(a) {
			continue
		}
		pending = append(pending, task{acct: a, client: c})
		if len(pending) >= limit {
			break
		}
	}
	if len(pending) == 0 {
		return 0
	}

	sem := make(chan struct{}, workers)
	var wg sync.WaitGroup
	done := 0
	var mu sync.Mutex
	for _, t := range pending {
		wg.Add(1)
		go func(t task) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				return
			}
			defer func() { <-sem }()
			out := t.client.EnsureCheckInWithin(ctx, t.acct, warmCheckInBudget)
			if !out.Attempted {
				return
			}
			s.warm.note(t.acct.ID, out.Err == nil, time.Now())
			mu.Lock()
			done++
			mu.Unlock()
			if out.Err != nil {
				// 带上 out.Err：只打 timed_out 时看不出是「我方预算」还是
				// CONNECT 被重置 / 上游 5xx，线上 2026-11-24 就卡在这里。
				logx.Info("签到预热未完成（不影响推理）",
					"account", acctLabel(t.acct),
					"ms", out.Waited.Milliseconds(),
					"timed_out", out.TimedOut,
					"fails", s.warm.fails(t.acct.ID),
					"err", out.Err)
			}
		}(t)
	}
	wg.Wait()
	return done
}
