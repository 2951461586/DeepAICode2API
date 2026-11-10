package deepai

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestNewInstanceIDLayout(t *testing.T) {
	a, b := newInstanceID(), newInstanceID()
	if len(a) != 32 || strings.Trim(a, "0123456789abcdef") != "" {
		t.Fatalf("实例号 = %q，期望 32 位小写 hex", a)
	}
	if a == b {
		t.Fatal("每次调用都应是新的实例号")
	}
}

func TestCheckInKey(t *testing.T) {
	if got := checkInKey(nil); got != "" {
		t.Fatalf("nil 账号的 key = %q，期望空串", got)
	}
	if got := checkInKey(&Account{APIKey: "sk-a"}); got != "sk-a" {
		t.Fatalf("key = %q，期望 api key 本身", got)
	}
	// 同一账号不同字段不应影响 key（只有 API Key 参与隔离）。
	if checkInKey(&Account{ID: "x", APIKey: "sk-a"}) != checkInKey(&Account{ID: "y", APIKey: "sk-a"}) {
		t.Fatal("签到状态应按 API Key 隔离")
	}
}

// TestStateForLazilyInitializes 覆盖零值 Client（未经过构造器）的兜底路径。
func TestStateForLazilyInitializes(t *testing.T) {
	c := &Client{}
	st := c.stateFor(nil)
	if st == nil {
		t.Fatal("stateFor 不应返回 nil")
	}
	if c.checkIns == nil {
		t.Fatal("stateFor 应惰性初始化签到存储")
	}
	if c.stateFor(nil) != st {
		t.Fatal("同一 key 的签到状态应稳定复用")
	}

	other := c.stateFor(&Account{APIKey: "sk-other"})
	if other == st {
		t.Fatal("不同账号不应共享签到状态")
	}
}

func TestMarkAckedIgnoresEmptyNonce(t *testing.T) {
	c := NewClient()
	acct := &Account{APIKey: "sk-a"}

	c.markAcked(acct, "")
	if got := c.stateFor(acct).ackedNonce; got != "" {
		t.Fatalf("空 nonce 不应记录，得到 %q", got)
	}
	c.markAcked(acct, "nonce-1")
	if got := c.stateFor(acct).ackedNonce; got != "nonce-1" {
		t.Fatalf("ackedNonce = %q", got)
	}
}

func TestLockCheckInGuards(t *testing.T) {
	c := NewClient()
	acct := &Account{APIKey: "sk-lock"}

	c.lockCheckIn(nil, time.Hour) // nil 账号不应 panic
	c.lockCheckIn(acct, 0)        // 非正数不应加锁
	c.lockCheckIn(acct, -time.Minute)
	if got := c.CheckInRefusedUntil(acct); !got.IsZero() {
		t.Fatalf("无效调用不应加锁，得到 %v", got)
	}

	c.lockCheckIn(acct, time.Hour)
	until := c.CheckInRefusedUntil(acct)
	if until.Before(time.Now().Add(59 * time.Minute)) {
		t.Fatalf("锁到期时间应在 1h 后，得到 %v", until)
	}
	// 锁是按账号隔离的。
	if got := c.CheckInRefusedUntil(&Account{APIKey: "sk-other"}); !got.IsZero() {
		t.Fatalf("锁污染了其它账号: %v", got)
	}
}

// TestCheckInReturnsDefensiveCopy 确认调用方改返回值不会污染缓存。
func TestCheckInReturnsDefensiveCopy(t *testing.T) {
	c := NewClient()
	acct := &Account{APIKey: "sk-ci"}
	if got := c.CheckIn(acct); got != nil {
		t.Fatalf("尚未拿到挑战时应返回 nil，得到 %+v", got)
	}

	c.captureCheckIn(acct, []byte(`{"check_in":{"nonce":"`+strings.Repeat("a", 32)+`","seconds":600}}`))
	got := c.CheckIn(acct)
	if got == nil || got.Seconds != 600 {
		t.Fatalf("挑战未缓存: %+v", got)
	}
	got.Nonce = "mutated"
	if c.CheckIn(acct).Nonce == "mutated" {
		t.Fatal("CheckIn 应返回副本")
	}
}

// TestCaptureCheckInRejectsInvalid 覆盖非法/缺失 check_in 的静默忽略。
func TestCaptureCheckInRejectsInvalid(t *testing.T) {
	c := NewClient()
	acct := &Account{APIKey: "sk-bad"}
	for _, body := range []string{
		`not json`,
		`{}`,
		`{"check_in":null}`,
		`{"check_in":{"nonce":"short","seconds":600}}`,                           // nonce 长度不对
		`{"check_in":{"nonce":"` + strings.Repeat("g", 32) + `","seconds":600}}`, // 非 hex
		`{"check_in":{"nonce":"` + strings.Repeat("a", 32) + `","seconds":0}}`,   // seconds 必须为正
	} {
		c.captureCheckIn(acct, []byte(body))
		if got := c.CheckIn(acct); got != nil {
			t.Fatalf("body=%s 不应缓存挑战，得到 %+v", body, got)
		}
	}
}

// TestEnsureCheckInSkipsWhileRefused 确认静默锁内完全不打上游。
func TestEnsureCheckInSkipsWhileRefused(t *testing.T) {
	called := 0
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		called++
		_, _ = w.Write([]byte(`{}`))
	})
	acct := &Account{APIKey: "sk-refused"}

	c.lockCheckIn(acct, time.Hour)
	c.EnsureCheckIn(context.Background(), acct)
	if called != 0 {
		t.Fatalf("静默锁内不应请求 /theos，实际调用了 %d 次", called)
	}

	// 解锁后（把锁过期时间拨到过去）应立即补签一次。
	c.stateFor(acct).refusedUntil = time.Now().Add(-time.Second)
	c.EnsureCheckIn(context.Background(), acct)
	if called != 1 {
		t.Fatalf("解锁后应补签一次，实际 %d 次", called)
	}

	// checkInMinInterval 内的第二次调用应被节流。
	c.EnsureCheckIn(context.Background(), acct)
	if called != 1 {
		t.Fatalf("节流窗口内不应重复签到，实际 %d 次", called)
	}
}

// TestEnsureCheckInNilAccount 确认 nil 账号直接返回（推理前调用必须无副作用）。
func TestEnsureCheckInNilAccount(t *testing.T) {
	called := 0
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		called++
	})
	c.EnsureCheckIn(context.Background(), nil)
	if called != 0 {
		t.Fatalf("nil 账号不应请求上游，实际 %d 次", called)
	}
}

// TestApplyCheckInHeadersBeacon 覆盖心跳的 pi 字段与 nonce 回传。
func TestApplyCheckInHeadersBeacon(t *testing.T) {
	c := NewClient()
	acct := &Account{APIKey: "sk-beacon"}

	// 冷启动：没有 nonce、没有转发记录 → 只有心跳，pi=0。
	req := mustReq(t, c)
	c.applyCheckInHeaders(req, acct)
	beacon := req.Header.Get(HeaderBeacon)
	if !strings.HasPrefix(beacon, "v="+AppVersion+";up=") || !strings.HasSuffix(beacon, ";pi=0") {
		t.Fatalf("心跳 = %q", beacon)
	}
	if beacon != "v="+AppVersion+";up="+strconv.Itoa(UptimeSeconds())+";pi=0" {
		t.Fatalf("心跳格式错误: %q", beacon)
	}
	if got := req.Header.Get(HeaderCheckIn); got != "" {
		t.Fatalf("无挑战时不应带 %s，得到 %q", HeaderCheckIn, got)
	}

	// 有挑战 + 刚转发过 → 带 nonce 且 pi=1。
	nonce := strings.Repeat("b", 32)
	c.captureCheckIn(acct, []byte(`{"check_in":{"nonce":"`+nonce+`","seconds":600}}`))
	c.markRelay(acct)
	req = mustReq(t, c)
	c.applyCheckInHeaders(req, acct)
	if got := req.Header.Get(HeaderCheckIn); got != nonce {
		t.Fatalf("%s = %q，期望 %q", HeaderCheckIn, got, nonce)
	}
	if !strings.HasSuffix(req.Header.Get(HeaderBeacon), ";pi=1") {
		t.Fatalf("刚转发过时 pi 应为 1: %q", req.Header.Get(HeaderBeacon))
	}

	// 转发记录早于挑战 TTL → pi 回落 0。
	c.stateFor(acct).lastRelay = time.Now().Add(-time.Hour)
	req = mustReq(t, c)
	c.applyCheckInHeaders(req, acct)
	if !strings.HasSuffix(req.Header.Get(HeaderBeacon), ";pi=0") {
		t.Fatalf("过期转发记录后 pi 应为 0: %q", req.Header.Get(HeaderBeacon))
	}
}

// mustReq 构造一个用于检查请求头的最小请求（httptest 可避免真实网络与 handler 规则）。
func mustReq(t *testing.T, c *Client) *http.Request {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, c.ModelURL(PathTheos), nil)
	if req == nil {
		t.Fatal("构造请求失败")
	}
	return req
}

func TestUptimeSecondsMonotonic(t *testing.T) {
	a := UptimeSeconds()
	if a < 0 {
		t.Fatalf("UptimeSeconds() = %d，不应为负", a)
	}
	if b := UptimeSeconds(); b < a {
		t.Fatalf("运行时长不应回退: %d → %d", a, b)
	}
}
