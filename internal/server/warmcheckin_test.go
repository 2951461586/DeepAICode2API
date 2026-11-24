package server

import (
	"context"
	"net/http"
	"testing"
	"time"

	"deepaicode2api/internal/deepai"
)

// TestWarmCheckInsConsumesDueOnce 固化「后台预热只消化到期的账号，且幂等」：
// 第一次预热真的打了一次 /theos，紧接着的第二次必须什么都不做
// （attemptAt 的最小间隔在兜底），否则后台就会变成新的签到风暴源。
func TestWarmCheckInsConsumesDueOnce(t *testing.T) {
	u := newUpstream(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"enabled":true,"balance":"10","granted":"20","check_in":{"nonce":"n-1","seconds":600}}`))
	})
	s, _ := newTestServer(t, u)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if got := s.WarmCheckIns(ctx, 4, 2); got != 1 {
		t.Fatalf("第一次预热处理了 %d 个账号，期望 1", got)
	}
	if got := s.WarmCheckIns(ctx, 4, 2); got != 0 {
		t.Fatalf("第二次预热处理了 %d 个账号，期望 0（最小间隔内不再打扰上游）", got)
	}
}

// TestWarmCheckInsSkipsDisabled 固化「不给停用的号打 /theos」：
// 预热是优化，不该替运维去碰已停用的账号。
func TestWarmCheckInsSkipsDisabled(t *testing.T) {
	u := newUpstream(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"enabled":true,"balance":"10","granted":"20"}`))
	})
	s, _ := newTestServer(t, u)
	disabled := func(a *deepai.Account) bool {
		a.Enabled = false
		return true
	}
	if n := s.Store.UpdateAccounts(disabled); n != 1 {
		t.Fatalf("停用账号失败（改动 %d 个）", n)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if got := s.WarmCheckIns(ctx, 4, 2); got != 0 {
		t.Fatalf("停用账号不应被预热，实际处理 %d 个", got)
	}
	u.mu.Lock()
	n := len(u.requests)
	u.mu.Unlock()
	if n != 0 {
		t.Fatalf("上游收到了 %d 个请求，期望 0", n)
	}
}

// TestCheckInDueAfterAttemptThrottled 固化深层的判据本身：
// 没回传过 nonce 时，半衰期节流不生效，但最小间隔（30s）会拦住连打。
func TestCheckInDueAfterAttemptThrottled(t *testing.T) {
	u := newUpstream(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"enabled":true,"balance":"10","granted":"20","check_in":{"nonce":"n-1","seconds":600}}`))
	})
	s, _ := newTestServer(t, u)
	acct := s.Store.Accounts()[0]
	client := s.clientFor(acct)

	if !client.CheckInDue(acct) {
		t.Fatal("新号应当签到到期")
	}
	out := client.EnsureCheckIn(context.Background(), acct)
	if !out.Attempted || out.Err != nil {
		t.Fatalf("签到失败：attempted=%v err=%v", out.Attempted, out.Err)
	}
	if client.CheckInDue(acct) {
		t.Fatal("最小间隔内不应再次到期")
	}
}

// TestWarmCooldownBackoffCurve 固化退避曲线：2m / 4m / 8m / 16m / 20m（封顶），
// 并且「成功一次」即清零。曲线本身是策略，写死在这里防止以后调参改坏。
func TestWarmCooldownBackoffCurve(t *testing.T) {
	var c warmCooldown
	now := time.Unix(1795480000, 0)
	want := []time.Duration{
		2 * time.Minute,
		4 * time.Minute,
		8 * time.Minute,
		16 * time.Minute,
		20 * time.Minute, // 封顶
		20 * time.Minute,
	}
	for i, d := range want {
		c.note("acc-1", false, now)
		if got := c.fails("acc-1"); got != i+1 {
			t.Fatalf("第 %d 次失败后 fails = %d，期望 %d", i+1, got, i+1)
		}
		// 窗口内不放行，窗口外放行。
		if c.allow("acc-1", now.Add(d-time.Second)) {
			t.Fatalf("第 %d 次失败：退避窗口内不应放行（窗口 %v）", i+1, d)
		}
		if !c.allow("acc-1", now.Add(d)) {
			t.Fatalf("第 %d 次失败：退避窗口过后应放行（窗口 %v）", i+1, d)
		}
	}

	// 成功一次清零：既不退避，也不留失败计数。
	c.note("acc-1", true, now)
	if got := c.fails("acc-1"); got != 0 {
		t.Fatalf("成功后 fails = %d，期望 0", got)
	}
	if !c.allow("acc-1", now) {
		t.Fatal("成功后应立即放行")
	}
}

// TestWarmCooldownZeroValue 固化零值可用：没失败过的账号直接放行。
func TestWarmCooldownZeroValue(t *testing.T) {
	var c warmCooldown
	if !c.allow("never-seen", time.Now()) {
		t.Fatal("没有失败记录的账号应当放行")
	}
	if got := c.fails("never-seen"); got != 0 {
		t.Fatalf("fails = %d，期望 0", got)
	}
}

// TestWarmCheckInsHonoursCooldown 固化接线：退避中的账号这一轮根本不打上游。
//
// 线上症状是「30s 一轮、每轮都恰好超时 10s」，退避就是为了掐掉这条死循环。
func TestWarmCheckInsHonoursCooldown(t *testing.T) {
	u := newUpstream(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"enabled":true,"balance":"10","granted":"20"}`))
	})
	s, _ := newTestServer(t, u)
	acct := s.Store.Accounts()[0]

	s.warm.note(acct.ID, false, time.Now()) // 等价于「刚失败过一轮」

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if got := s.WarmCheckIns(ctx, 4, 2); got != 0 {
		t.Fatalf("退避中的账号不应被预热，实际处理 %d 个", got)
	}
	u.mu.Lock()
	n := len(u.requests)
	u.mu.Unlock()
	if n != 0 {
		t.Fatalf("退避中仍向上游发了 %d 个请求", n)
	}
}
