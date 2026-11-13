package deepai

import (
	"context"
	"io"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// nonceServer 每次 /theos 都下发一个**新** nonce（与上游真实行为一致）。
//
// 这个细节是旧实现出错的根因：用「最新 nonce != 已回传 nonce」判断是否补签，
// 会在每次回传后又变成 true，把 seconds/2 的节奏绕过成 checkInMinInterval。
func nonceServer(t *testing.T, hits *int) *Client {
	t.Helper()
	var mu sync.Mutex
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		*hits++
		mu.Unlock()
		if r.Header.Get(HeaderCheckIn) == "" {
			_, _ = io.WriteString(w, `{"enabled":true,"check_in":{"nonce":"`+repeatHex("a")+`","seconds":600}}`)
			return
		}
		_, _ = io.WriteString(w, `{"enabled":true,"check_in":{"nonce":"`+repeatHex("b")+`","seconds":600}}`)
	})
	return c
}

func repeatHex(ch string) string {
	out := ""
	for i := 0; i < 32; i++ {
		out += ch
	}
	return out
}

// TestEnsureCheckInFollowsHalfLife 固化桌面端节奏：
// 回传完成后的 seconds/2（600s → 300s）内不再打扰上游；
// 超过半衰期才补签一次。旧实现会每 30s 撞一次。
func TestEnsureCheckInFollowsHalfLife(t *testing.T) {
	hits := 0
	c := nonceServer(t, &hits)
	acct := &Account{APIKey: "k-cadence"}
	ctx := context.Background()

	// 冷启动：拿到挑战但还没回传。
	if _, err := c.Theos(ctx, acct); err != nil {
		t.Fatalf("Theos: %v", err)
	}
	if hits != 1 {
		t.Fatalf("首次 /theos 应调用 1 次，实际 %d", hits)
	}

	// 第一次补签：立刻回传（不能等半衰期，否则冷启动 5 分钟内都没签到）。
	if out := c.EnsureCheckIn(ctx, acct); !out.Attempted {
		t.Fatal("冷启动后的第一次推理应补签")
	}
	if hits != 2 {
		t.Fatalf("补签应调用 /theos，实际 %d 次", hits)
	}

	// 半衰期内：把 attemptAt 拨到 1 分钟前（避开 30s 兜底），仍不应补签。
	st := c.stateFor(acct)
	c.checkIns.mu.Lock()
	st.attemptAt = time.Now().Add(-time.Minute)
	c.checkIns.mu.Unlock()
	if out := c.EnsureCheckIn(ctx, acct); out.Attempted {
		t.Fatal("半衰期内不应补签")
	}
	if hits != 2 {
		t.Fatalf("半衰期内不应请求上游，实际 %d 次", hits)
	}

	// 超过半衰期（600s/2=300s）：应补签一次。
	c.checkIns.mu.Lock()
	st.ackedAt = time.Now().Add(-301 * time.Second)
	st.attemptAt = time.Now().Add(-time.Minute)
	c.checkIns.mu.Unlock()
	if out := c.EnsureCheckIn(ctx, acct); !out.Attempted {
		t.Fatal("超过半衰期应补签")
	}
	if hits != 3 {
		t.Fatalf("超过半衰期应请求上游，实际 %d 次", hits)
	}
}

// TestEnsureCheckInBudget 固化「签到预算可调」：上游超时后立即放行推理，
// 结果标记 TimedOut，且不影响后续请求继续补签。
func TestEnsureCheckInBudget(t *testing.T) {
	var hits atomic.Int64
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		time.Sleep(300 * time.Millisecond)
		_, _ = io.WriteString(w, `{"enabled":true}`)
	})
	c.SetCheckInBudget(40 * time.Millisecond)

	start := time.Now()
	out := c.EnsureCheckIn(context.Background(), &Account{APIKey: "k-budget"})
	elapsed := time.Since(start)

	if !out.Attempted {
		t.Fatal("应尝试签到")
	}
	if !out.TimedOut {
		t.Fatalf("应报告 TimedOut，实际 err=%v", out.Err)
	}
	if elapsed > 250*time.Millisecond {
		t.Fatalf("预算 40ms 应尽快返回，实际等了 %v", elapsed)
	}
	if got := hits.Load(); got != 1 {
		t.Fatalf("上游调用次数 = %d，期望 1", got)
	}
}

// TestSetCheckInBudgetNegative 覆盖越界值：负数归零（= 用默认 10s）。
func TestSetCheckInBudgetNegative(t *testing.T) {
	c := NewClient()
	c.SetCheckInBudget(-time.Second)
	if c.CheckInBudget != 0 {
		t.Fatalf("负数预算应归零，实际 %v", c.CheckInBudget)
	}
	if got := c.checkInBudget(); got != checkInAttemptTimeout {
		t.Fatalf("默认预算 = %v，期望 %v", got, checkInAttemptTimeout)
	}
}
