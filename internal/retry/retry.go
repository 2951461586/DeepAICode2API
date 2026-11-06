// Package retry 提供基于「瞬时错误」判定的退避重试。
//
// 网关里存在多处「失败就再来一次」的逻辑（/theos 验活、入池配额探测、
// 代理连通性检查）。它们共享同一条判据（deepai.IsTransient）但各自
// 手写退避循环，容易在改动时漏掉某一处。统一到这里后，
// 「重试几次、每次等多久、什么错误不重试」只有一处定义。
package retry

import (
	"context"
	"time"
)

// Classifier 报告错误是否值得重试。
type Classifier func(error) bool

// Do 执行 fn；失败且 retryable 判定可重试时，按 delays 依次退避后重试。
//
// 语义：
//   - delays 为空表示只尝试一次；
//   - 首次失败即被 retryable 判为不可重试时，立刻返回该错误；
//   - 显式把 ctx 传进 fn，便于每次尝试各自设置超时；
//   - ctx 在退避期间到期时返回最后一次的错误（不再发起新尝试）。
//
// 返回 nil 表示某次尝试成功。
func Do(ctx context.Context, delays []time.Duration, retryable Classifier, fn func(context.Context) error) error {
	var lastErr error
	for attempt := 0; ; attempt++ {
		if attempt > 0 && !sleep(ctx, delays[attempt-1]) {
			return lastErr
		}
		err := fn(ctx)
		if err == nil {
			return nil
		}
		lastErr = err
		if retryable != nil && !retryable(err) {
			return err
		}
		if attempt >= len(delays) {
			return lastErr
		}
	}
}

// sleep 等待 d，ctx 到期时返回 false。
func sleep(ctx context.Context, d time.Duration) bool {
	if d <= 0 {
		return true
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
