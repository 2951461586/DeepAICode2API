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
// 比热路径长得多（热路径线上设 2.5s）：预热不在任何请求的关键路径上，
// 给足时间才能真正把 nonce 回传成功（住宅代理 p95 约 5s，实测 2.5s 预算下
// 几乎全部超时，等于把「到期」白消耗掉并占满 30s 的最小间隔）。
const warmCheckInBudget = 10 * time.Second

// warmCheckInDefault 是每次预热最多处理的账号数。
//
// 16 个号、半衰期 seconds/2（默认 300s）意味着稳态下每分钟只有 1~2 个号到期，
// 上限 4 已经足够把「到期」提前消化掉，又不会在启动瞬间把代理打满。
const warmCheckInDefault = 4

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
//   - 与热路径共用 EnsureCheckIn 的判据与幂等锁，不会重复打 /theos。
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
	for _, a := range s.Store.Accounts() {
		if a == nil || !a.Enabled {
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
			mu.Lock()
			done++
			mu.Unlock()
			if out.Err != nil {
				logx.Info("签到预热未完成（不影响推理）", "account", acctLabel(t.acct), "ms", out.Waited.Milliseconds(), "timed_out", out.TimedOut)
			}
		}(t)
	}
	wg.Wait()
	return done
}
