package server

import (
	"context"
	"errors"
	"net/http"
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
// p90 是 4129ms，热连接约 2000ms；`/models` 不烧 Theos，是唯一「免费但会
// 走完整链路」的端点。
//
// 2026-11-14 起 /models 要求 Api-Key，所以每个出口取**该出口上的一个真实
// 账号**去预热（而不是只带 Proxy 的空账号）；401 仍算预热成功 —— 那已经
// 证明 DNS/TCP/TLS/代理 CONNECT 全部通了，只是这个出口上的号没 Key。
//
// 语义：只预热启用中的账号用到的出口（含「直连」这一档），同一出口只打一次；
// 失败静默（预热是纯优化，不能影响启动）；返回成功的出口数。
func (s *Server) WarmUpstream(ctx context.Context) int {
	byEgress := map[string]*deepai.Account{}
	for _, a := range s.Store.Accounts() {
		if a == nil || !a.Enabled {
			continue
		}
		// 空串代表直连（走 s.Up），这里作为一个独立出口参与预热。
		if _, seen := byEgress[a.Proxy]; !seen {
			byEgress[a.Proxy] = a
		}
	}

	targets := make([]string, 0, len(byEgress))
	for proxy := range byEgress {
		targets = append(targets, proxy)
	}
	// 固定顺序：日志与测试才对得上，也避免每次启动的预热顺序抖动。
	sort.Strings(targets)

	var ok atomic.Int64
	parallel.For(ctx, WarmUpstreamWorkers, len(targets), func(ctx context.Context, i int) {
		attempt, cancel := context.WithTimeout(ctx, WarmUpstreamTimeout)
		defer cancel()
		acct := byEgress[targets[i]]
		client := s.clientFor(acct)
		if _, _, err := client.FetchModels(attempt, acct); err != nil && !warmReachable(err) {
			return
		}
		ok.Add(1)
	})
	return int(ok.Load())
}

// warmReachable 判断预热失败是否属于「链路其实已经通了」。
//
// 401 = 上游要 Key 而这个出口上的号没给（或 Key 失效）：DNS/TCP/TLS 与代理
// CONNECT 都已经成功，预热的目的一样达到。5xx / 超时才是真的没通。
func warmReachable(err error) bool {
	var he *deepai.HTTPError
	return errors.As(err, &he) && he.Status == http.StatusUnauthorized
}
