package server

import (
	"context"
	"strconv"
	"testing"
	"time"

	"deepaicode2api/internal/deepai"
)

// fillPool 把号池替换成 n 个可用账号（先清空 newTestServer 自带的那个）。
func fillPool(t *testing.T, s *Server, n int) {
	t.Helper()
	for _, a := range s.Store.Accounts() {
		s.Store.RemoveAccount(a.ID)
	}
	for i := 0; i < n; i++ {
		id := "acc-" + strconv.Itoa(i)
		s.Store.UpsertAccount(&deepai.Account{
			ID: id, Name: id, APIKey: "key-" + strconv.Itoa(i),
			Enabled: true, Status: "ok",
		})
	}
}

// TestPickFreeAccountAllocationsBounded 是 P0-1 的回归防线。
//
// 旧实现是「循环 n 次，每次 Accounts() 全池深拷贝 + NextAccountGroup 写锁」，
// 分配数随池子规模线性增长（500 个号时单次选号上千次分配）。现在应当
// 只有「被选中的那一个账号 + 少量固定开销」。
func TestPickFreeAccountAllocationsBounded(t *testing.T) {
	s, _ := newTestServer(t, nil)
	fillPool(t, s, 500)

	const budget = 40 // 远小于 500（旧实现至少 500×2 次分配）
	got := testing.AllocsPerRun(50, func() {
		a, ok, _ := s.pickFreeAccount("")
		if !ok || a == nil {
			t.Fatalf("应当选到账号")
		}
		s.releaseAccount(a.ID)
	})
	if got > budget {
		t.Fatalf("单次选号分配 %.1f 次，基线 %d：又在全池拷贝了？", got, budget)
	}
}

// TestPickFreeAccountSkipsBusy 验证并发槽位在选号阶段就被排除，
// 并且「全忙」与「没有可调度账号」能被区分开。
func TestPickFreeAccountSkipsBusy(t *testing.T) {
	s, _ := newTestServer(t, nil)
	fillPool(t, s, 2)

	// 每个账号最多 maxInFlightPerAccount 个并发；先占满全部。
	var held []string
	for i := 0; i < 2*maxInFlightPerAccount; i++ {
		a, ok, candidates := s.pickFreeAccount("")
		if !ok {
			t.Fatalf("第 %d 次应仍有空槽（candidates=%v）", i, candidates)
		}
		held = append(held, a.ID)
	}
	if len(held) != 2*maxInFlightPerAccount {
		t.Fatalf("应占满 2 个账号 × %d 个槽位，实际 %d", maxInFlightPerAccount, len(held))
	}
	// 槽位全满：拿不到账号，但 candidates 必须为 true（池里有号，只是忙）。
	if a, ok, candidates := s.pickFreeAccount(""); ok || a != nil || !candidates {
		t.Fatalf("全忙时不应选中账号：a=%v ok=%v candidates=%v", a, ok, candidates)
	}
	for _, id := range held {
		s.releaseAccount(id)
	}
	if a, ok, _ := s.pickFreeAccount(""); !ok || a == nil {
		t.Fatal("槽位释放后应能重新选到账号")
	}
}

// TestPickFreeAccountEmptyPool 区分「没号」与「全忙」。
func TestPickFreeAccountEmptyPool(t *testing.T) {
	s, _ := newTestServer(t, nil)
	for _, a := range s.Store.Accounts() {
		s.Store.RemoveAccount(a.ID)
	}
	if a, ok, candidates := s.pickFreeAccount(""); ok || a != nil || candidates {
		t.Fatalf("空池应返回 (nil,false,false)，实际 (%v,%v,%v)", a, ok, candidates)
	}
}

// TestCloseInterruptsReprobeBackoff 是 P0-3 的回归防线。
//
// 配额复探的退避最长 12 分钟；Close 必须能打断睡眠并立即返回，
// 否则进程关停会挂到超时。
func TestCloseInterruptsReprobeBackoff(t *testing.T) {
	s, _ := newTestServer(t, nil)
	s.quotaReprobeDelays = []time.Duration{time.Hour}

	s.scheduleQuotaReprobe("acc-1")

	done := make(chan struct{})
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		s.Close(ctx)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Close 未能在 2s 内返回：退避睡眠没有被关停信号打断")
	}
}

// TestCloseWaitsForProbes 验证 Close 会等已登记的探测落盘。
func TestCloseWaitsForProbes(t *testing.T) {
	s, _ := newTestServer(t, nil)

	started := make(chan struct{})
	s.quotaWG.Add(1)
	go func() {
		defer s.quotaWG.Done()
		close(started)
		time.Sleep(100 * time.Millisecond)
	}()
	<-started

	begin := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	s.Close(ctx)
	if elapsed := time.Since(begin); elapsed < 90*time.Millisecond {
		t.Fatalf("Close 提前返回（%s）：没有等待后台任务", elapsed)
	}
}
