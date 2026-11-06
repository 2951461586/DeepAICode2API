// Package parallel 提供有界并发的批量执行。
//
// 网关里有 6 处「固定并发跑一批任务」的逻辑：批量注册、台账补跑、
// 配额探测、控制台巡检、代理连通性检查、邮箱重新验证。它们各自手写
// jobs channel + WaitGroup，取消语义与结果收集方式还各不相同。
// 统一到 For 之后，并发上限、ctx 取消、结果按下标回填只有一处实现。
package parallel

import (
	"context"
	"sync"
)

// For 以最多 workers 个并发执行 fn(0)…fn(count-1)。
//
// 语义：
//   - workers<=0 视为 1；workers>count 会被收敛到 count；
//   - ctx 取消后不再派发新任务，已开始的任务会跑完；
//   - For 返回时所有已派发的任务都已结束，调用方可安全读取结果切片；
//   - fn 的写入必须落在各自下标上（或使用独立同步），避免结果竞争。
func For(ctx context.Context, workers, count int, fn func(ctx context.Context, i int)) {
	if count <= 0 || fn == nil {
		return
	}
	if workers <= 0 {
		workers = 1
	}
	if workers > count {
		workers = count
	}
	// 已取消时一个都不派发。
	//
	// 只靠派发循环里的 select 不够：ctx.Done() 与 jobs<-i 可能同时就绪，
	// Go 会等概率二选一，于是“已取消的 ctx”仍会跑掉部分任务。
	if ctx.Err() != nil {
		return
	}

	jobs := make(chan int)
	var wg sync.WaitGroup
	wg.Add(workers)
	for w := 0; w < workers; w++ {
		go func() {
			defer wg.Done()
			for i := range jobs {
				fn(ctx, i)
			}
		}()
	}

	for i := 0; i < count; i++ {
		select {
		case <-ctx.Done():
			close(jobs)
			wg.Wait()
			return
		case jobs <- i:
		}
	}
	close(jobs)
	wg.Wait()
}
