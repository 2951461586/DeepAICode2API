package poolsvc

import (
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"deepaicode2api/internal/deepai"
)

// --- 在途槽位 ---
//
// 槽位是「调度选到谁」与「上游允许几个在途」之间的唯一约束。它出错的表现是
// 账号被永久钉死（泄漏）或上限形同虚设（超额），两者都不会立刻报错，
// 只会在压测里变成成功率下降 —— 所以这里用并发用例直接压不变量。

// TestAcquireReleaseNoLeak 验证并发占用 / 归还之后不泄漏任何槽位。
func TestAcquireReleaseNoLeak(t *testing.T) {
	p := newService(t, newTestStore(t), nil)

	const (
		workers = 64
		rounds  = 50
	)
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		id := "acc-0"
		if w%2 == 0 {
			id = "acc-1"
		}
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			for i := 0; i < rounds; i++ {
				if !p.Acquire(id) {
					continue // 已满，下一次再试
				}
				p.Release(id)
			}
		}(id)
	}
	wg.Wait()

	if n := p.InFlight(); n != 0 {
		t.Fatalf("并发 Acquire/Release 后仍有 %d 个账号占着槽位（泄漏）", n)
	}
	// 泄漏的另一种形态：计数卡在非零但账号已不在表里。
	if got := p.inflight["acc-0"]; got != 0 {
		t.Fatalf("acc-0 计数 = %d，期望 0", got)
	}
}

// TestAcquireEnforcesPerAccountCap 验证单账号上限被真正执行。
func TestAcquireEnforcesPerAccountCap(t *testing.T) {
	p := newService(t, newTestStore(t), nil)

	for i := 0; i < MaxInFlightPerAccount; i++ {
		if !p.Acquire("acc-0") {
			t.Fatalf("第 %d 次占用应成功", i+1)
		}
	}
	if p.Acquire("acc-0") {
		t.Fatalf("超过上限 %d 仍占用成功", MaxInFlightPerAccount)
	}
	if !p.Busy("acc-0") {
		t.Fatal("占满后 Busy 应为 true")
	}

	p.Release("acc-0")
	if p.Busy("acc-0") {
		t.Fatal("归还一个槽位后 Busy 应为 false")
	}
	if !p.Acquire("acc-0") {
		t.Fatal("归还后应能再次占用")
	}
}

// TestReleaseUnknownAccountIsNoop 验证归还不存在的账号不会把计数打到负数。
func TestReleaseUnknownAccountIsNoop(t *testing.T) {
	p := newService(t, newTestStore(t), nil)

	p.Release("never-acquired")
	if n := p.InFlight(); n != 0 {
		t.Fatalf("InFlight = %d，期望 0", n)
	}
	if v, ok := p.inflight["never-acquired"]; ok {
		t.Fatalf("不存在的账号不应留在表里: %v", v)
	}

	// 归还不存在的账号之后，该账号的额度仍应是满的（没被负计数污染）。
	for i := 0; i < MaxInFlightPerAccount; i++ {
		if !p.Acquire("never-acquired") {
			t.Fatalf("第 %d 次占用应成功", i+1)
		}
	}
}

// TestWrapBodyReleasesOnClose 验证响应体关闭时归还槽位，且只归还一次。
func TestWrapBodyReleasesOnClose(t *testing.T) {
	p := newService(t, newTestStore(t), nil)

	if !p.Acquire("acc-0") {
		t.Fatal("占用应成功")
	}
	body := p.WrapBody(io.NopCloser(strings.NewReader("hello")), "acc-0")
	if body == nil {
		t.Fatal("WrapBody 返回 nil")
	}
	raw, err := io.ReadAll(body)
	if err != nil || string(raw) != "hello" {
		t.Fatalf("读体失败: %q %v", raw, err)
	}
	// 读完之后槽位尚未归还：流式响应可能持续数分钟。
	if !p.Busy("acc-0") && p.InFlight() != 1 {
		t.Fatalf("读体不应归还槽位，InFlight = %d", p.InFlight())
	}

	if err := body.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if n := p.InFlight(); n != 0 {
		t.Fatalf("Close 后 InFlight = %d，期望 0", n)
	}

	// 重复 Close 不应再归还一个（否则计数会变成负值 / 抢占别人的槽）。
	_ = body.Close()
	if n := p.InFlight(); n != 0 {
		t.Fatalf("重复 Close 后 InFlight = %d，期望 0", n)
	}
}

// TestWrapBodyNil 验证 nil 响应体原样返回 nil（上游 204 等）。
func TestWrapBodyNil(t *testing.T) {
	p := newService(t, newTestStore(t), nil)
	if got := p.WrapBody(nil, "acc-0"); got != nil {
		t.Fatalf("WrapBody(nil) = %v，期望 nil", got)
	}
}

// TestPickReportsBusyVersusEmpty 覆盖 Pick 第三个返回值的两种语义。
//
// 这个标记决定对外是「稍后重试（429）」还是「没有可用账号（503 + 原因）」，
// 两者混淆会让运维拿着 429 去查一个已经全被拦截的号池。
func TestPickReportsBusyVersusEmpty(t *testing.T) {
	st := newTestStore(t)
	p := newService(t, st, nil)

	// 池为空：没有可调度账号。
	if a, ok, candidates := p.Pick("", ""); a != nil || ok || candidates {
		t.Fatalf("空池 Pick = (%v,%v,%v)，期望 (nil,false,false)", a, ok, candidates)
	}

	seedAccounts(t, st, 1)
	for i := 0; i < MaxInFlightPerAccount; i++ {
		if !p.Acquire("acc-0") {
			t.Fatalf("第 %d 次占用应成功", i+1)
		}
	}
	a, ok, candidates := p.Pick("", "")
	if a != nil || ok {
		t.Fatalf("全忙时不应选到账号，实际 %+v ok=%v", a, ok)
	}
	if !candidates {
		t.Fatal("全忙时 candidates 应为 true（池里有号，只是没空槽）")
	}
	if p.HasFree() {
		t.Fatal("全忙时 HasFree 应为 false")
	}

	p.Release("acc-0")
	if !p.HasFree() {
		t.Fatal("归还后 HasFree 应为 true")
	}
	if a, ok, _ := p.Pick("", ""); !ok || a == nil {
		t.Fatalf("有空槽时应选到账号，实际 %+v ok=%v", a, ok)
	}
}

// TestPickPrefersStickyThenFallsBack 验证粘性只影响「先试谁」。
func TestPickPrefersStickyThenFallsBack(t *testing.T) {
	st := newTestStore(t)
	p := newService(t, st, nil)
	seedAccounts(t, st, 3)

	p.BindSticky("sess", "acc-2")
	a, ok, _ := p.Pick("", p.StickyAccount("sess"))
	if !ok || a == nil || a.ID != "acc-2" {
		t.Fatalf("应优先选到绑定的 acc-2，实际 %+v ok=%v", a, ok)
	}
	p.Release(a.ID)

	// 绑定的号满槽时静默回退，不阻塞请求。
	for i := 0; i < MaxInFlightPerAccount; i++ {
		if !p.Acquire("acc-2") {
			t.Fatalf("第 %d 次占用应成功", i+1)
		}
	}
	a2, ok, _ := p.Pick("", p.StickyAccount("sess"))
	if !ok || a2 == nil {
		t.Fatal("绑定的号满槽时应回退到别的号")
	}
	if a2.ID == "acc-2" {
		t.Fatal("绑定的号已满，不应被选中")
	}
}

// TestPickSkipsUnschedulableAccount 验证被拦截的号不会被 Pick 选中。
func TestPickSkipsUnschedulableAccount(t *testing.T) {
	st := newTestStore(t)
	p := newService(t, st, nil)
	seedAccounts(t, st, 2)
	p.Block("acc-0", "social_login_required: use google")

	for i := 0; i < 10; i++ {
		a, ok, _ := p.Pick("", "")
		if !ok || a == nil {
			t.Fatal("还有可用账号，Pick 不应失败")
		}
		if a.ID == "acc-0" {
			t.Fatal("被拦截的账号不应被选中")
		}
		p.Release(a.ID)
	}
}

// TestPickExcludesBusyPreferred 验证「优先号忙」时回退而不是死循环。
func TestPickExcludesBusyPreferred(t *testing.T) {
	st := newTestStore(t)
	p := newService(t, st, nil)
	seedAccounts(t, st, 2)
	for i := 0; i < MaxInFlightPerAccount; i++ {
		if !p.Acquire("acc-0") {
			t.Fatalf("第 %d 次占用应成功", i+1)
		}
	}

	done := make(chan *deepai.Account, 1)
	go func() {
		a, _, _ := p.Pick("", "acc-0")
		done <- a
	}()
	select {
	case a := <-done:
		if a == nil || a.ID != "acc-1" {
			t.Fatalf("优先号满槽时应回退到 acc-1，实际 %+v", a)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Pick 在满槽的优先号上卡住（疑似死循环）")
	}
}
