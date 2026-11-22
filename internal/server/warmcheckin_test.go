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
