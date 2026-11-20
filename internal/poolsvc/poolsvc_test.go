package poolsvc

import (
	"context"
	"testing"
	"time"

	"deepaicode2api/internal/store"
)

// TestCloseWaitsForProbes 验证 Close 会等已登记的后台落盘工作。
//
// 白盒用例（直接用 quotaWG）：这正是「先写内存再 Save()」的 goroutine 与
// 进程退出之间的契约，测试要能模拟一笔已登记、尚未完成的工作。
func TestCloseWaitsForProbes(t *testing.T) {
	p := New(Config{})

	started := make(chan struct{})
	p.quotaWG.Add(1)
	go func() {
		defer p.quotaWG.Done()
		close(started)
		time.Sleep(100 * time.Millisecond)
	}()
	<-started

	begin := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	p.Close(ctx)
	if elapsed := time.Since(begin); elapsed < 90*time.Millisecond {
		t.Fatalf("Close 提前返回（%s）：没有等待后台任务", elapsed)
	}
}

// TestCloseInterruptsBackoff 验证 Close 能打断复探退避睡眠。
//
// 退避最长 12 分钟；不打断就会把进程关停拖到超时。
func TestCloseInterruptsBackoff(t *testing.T) {
	p := New(Config{})
	p.quotaReprobeDelays = []time.Duration{time.Hour}
	p.ScheduleQuotaReprobe("acc-1")

	done := make(chan struct{})
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		p.Close(ctx)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Close 未能在 2s 内返回：退避睡眠没有被关停信号打断")
	}
}

// TestStickyExpired 验证过期绑定按未绑定处理，且会被顺手清掉。
func TestStickyExpired(t *testing.T) {
	p := New(Config{})
	p.sticky["sess"] = store.StickyBinding{AccountID: "acc-0", At: time.Now().Add(-2 * StickyTTL).Unix()}
	if got := p.StickyAccount("sess"); got != "" {
		t.Fatalf("过期绑定应失效，实际 %q", got)
	}
	if p.Bindings() != 0 {
		t.Fatalf("过期绑定应被清除，实际仍剩 %d 条", p.Bindings())
	}
}
