package server

import (
	"context"
	"time"

	"deepaicode2api/internal/deepai"
	"deepaicode2api/internal/parallel"
	"deepaicode2api/internal/retry"
)

// --- 账号配额探测 ---

// probeRetryDelays 是入池前探测配额的退避序列。
//
// 入池是唯一会阻塞调用方的探测点，而 /theos 的失败几乎都是出口抖动
// （No exit node / TLS handshake / unexpected EOF），重试一次能救回绝大多数。
var probeRetryDelays = []time.Duration{time.Second}

// probeAccountQuota 探测一个账号的配额；瞬时错误重试一次。
func (s *Server) probeAccountQuota(acct *deepai.Account) (*deepai.Allowance, error) {
	var al *deepai.Allowance
	err := retry.Do(context.Background(), probeRetryDelays, deepai.IsTransient, func(parent context.Context) error {
		ctx, cancel := context.WithTimeout(parent, 20*time.Second)
		defer cancel()
		got, err := s.clientFor(acct).Theos(ctx, acct)
		if err != nil {
			return err
		}
		al = got
		return nil
	})
	if err != nil {
		return nil, err
	}
	return al, nil
}

// probeAndSave 异步探测单个账号并落盘。
func (s *Server) probeAndSave(a *deepai.Account) {
	s.probeAccounts([]*deepai.Account{a})
}

// probeAccounts 以有界并发探测一批账号的配额并落盘。
//
// 串行探测 20 个账号最坏要 10 分钟（每个 30s 超时），所以限制到 4 并发；
// 再高容易把探针本身变成风控特征。
func (s *Server) probeAccounts(accts []*deepai.Account) {
	const workers = 4
	parallel.For(context.Background(), workers, len(accts), func(ctx context.Context, i int) {
		a := accts[i]
		if a == nil {
			return
		}
		ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()
		al, err := s.clientFor(a).Theos(ctx, a)
		s.Store.SetAccountAllowance(a.ID, al, err)
	})
	_ = s.Store.Save()
}
