package server

import (
	"context"
	"sort"
	"sync/atomic"
	"time"

	"deepaicode2api/internal/deepai"
	"deepaicode2api/internal/parallel"
)

const (
	// WarmUpstreamWorkers 是连接预热的并发上限：出口少时没必要开太多，
	// 出口多时也不该在启动瞬间把代理打满。
	WarmUpstreamWorkers = 4
	// WarmUpstreamTimeout 是单个出口的预热预算（含代理握手）。
	WarmUpstreamTimeout = 10 * time.Second
)

// WarmUpstream 按号池里实际出现过的出口各打一次 `/models`，把
// DNS / TCP / TLS（以及代理 CONNECT）提前做完。
//
// 为什么值得做：首个真实推理请求要串行付整条握手成本。r2 压测里冷启动
// p90 是 4129ms，热连接约 2000ms；`/models` 不需要 Api-Key、不烧 Theos，
// 是唯一「免费但会走完整链路」的端点。
//
// 语义：只预热启用中的账号用到的出口（含「直连」这一档），同一出口只打一次；
// 失败静默（预热是纯优化，不能影响启动）；返回成功的出口数。
func (s *Server) WarmUpstream(ctx context.Context) int {
	egress := map[string]bool{}
	for _, a := range s.Store.Accounts() {
		if !a.Enabled {
			continue
		}
		// 空串代表直连（走 s.Up），这里作为一个独立出口参与预热。
		egress[a.Proxy] = true
	}

	targets := make([]string, 0, len(egress))
	for proxy := range egress {
		targets = append(targets, proxy)
	}
	// 固定顺序：日志与测试才对得上，也避免每次启动的预热顺序抖动。
	sort.Strings(targets)

	var ok atomic.Int64
	parallel.For(ctx, WarmUpstreamWorkers, len(targets), func(ctx context.Context, i int) {
		attempt, cancel := context.WithTimeout(ctx, WarmUpstreamTimeout)
		defer cancel()
		client := s.clientFor(&deepai.Account{Proxy: targets[i]})
		if _, _, err := client.FetchModels(attempt); err != nil {
			return
		}
		ok.Add(1)
	})
	return int(ok.Load())
}
