package server

import (
	"io"
	"net/http"
	"testing"
	"time"

	"deepaicode2api/internal/deepai"
)

// 探测失败后的冷却必须真的挡住后台守护：否则每 5 分钟会把同一批失败号
// 全部重探一遍（失败往往成片），变成持续的 /theos 风暴。
func TestQuotaNeedsRefreshRespectsProbeCooldown(t *testing.T) {
	now := time.Now()

	cooling := &deepai.Account{ID: "a1", Enabled: true, ProbeAfter: now.Add(time.Hour).Unix()}
	if quotaNeedsRefresh(cooling, now, 6*time.Hour) {
		t.Fatal("冷却期内不应再探测")
	}
	if !quotaNeedsRefresh(cooling, now.Add(2*time.Hour), 6*time.Hour) {
		t.Fatal("冷却到期且从未探到额度，应重新探测")
	}

	fresh := &deepai.Account{
		ID: "a2", Enabled: true,
		Theos: &deepai.Allowance{Enabled: true, CheckedAt: now.Unix()},
	}
	if quotaNeedsRefresh(fresh, now, 6*time.Hour) {
		t.Fatal("额度快照新鲜时不应重探")
	}
}

// check_in_refused 要落 6h 探测锁，且不能把账号标成 error
// （推理照常可用，只是当天落在替身分支）。
func TestProbeCheckInRefusedLocksProbe(t *testing.T) {
	u := newUpstream(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = io.WriteString(w, `{"error":{"type":"check_in_refused","code":"check_in_refused"},`+
			`"check_in":{"nonce":"0123456789abcdef0123456789abcdef","seconds":600}}`)
	})
	s, _ := newTestServer(t, u)

	s.probeAccounts(s.Store.Accounts())

	a := s.Store.Account("acc-1")
	if a == nil {
		t.Fatal("账号不应消失")
	}
	if a.Status == "error" {
		t.Fatalf("签到限流不是账号坏，不应标 error: %q", a.LastErr)
	}
	if a.ProbeAfter <= time.Now().Unix() {
		t.Fatal("应落探测锁")
	}
	if left := time.Until(time.Unix(a.ProbeAfter, 0)); left < 5*time.Hour {
		t.Fatalf("探测锁应约 6h，实际剩余 %v", left)
	}
}

// 推理热路径的按需复探失败后必须留退避：线上（docs/changes.md §61）同一个号
// 1 小时被重探 323 次，就是因为这里只写了日志、没写 ProbeAfter。
func TestProbeAsyncFailureRecordsBackoff(t *testing.T) {
	u := newUpstream(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"error":{"message":"models temporarily unavailable"}}`)
	})
	s, _ := newTestServer(t, u)

	stale := time.Now().Add(-time.Hour).Unix()
	s.Store.UpsertAccount(&deepai.Account{
		ID: "stale-1", Name: "stale", APIKey: "sk-stale", Enabled: true, Status: "ok",
		Theos: &deepai.Allowance{Enabled: true, CheckedAt: stale},
	})
	a := s.Store.Account("stale-1")
	if a == nil {
		t.Fatal("账号不存在")
	}
	if !quotaNeedsRefresh(a, time.Now(), quotaStaleAfter) {
		t.Fatal("前置条件：额度快照过旧，应需要复探")
	}

	if !s.probeAsync(a) {
		t.Fatal("应发起一次探测")
	}
	s.quotaWG.Wait()

	got := s.Store.Account("stale-1")
	if got.ProbeFailures != 1 {
		t.Fatalf("探测失败应记一次退避，实际 ProbeFailures = %d", got.ProbeFailures)
	}
	if got.ProbeAfter <= time.Now().Unix() {
		t.Fatalf("应落探测冷却，实际 ProbeAfter = %d", got.ProbeAfter)
	}
	if quotaNeedsRefresh(got, time.Now(), quotaStaleAfter) {
		t.Fatal("冷却期内不应再被判为需要复探（否则每个请求都会重探一次）")
	}
	// 热路径失败不能污染调度：状态与失败计数都不动。
	if got.Status != "ok" || got.Failures != 0 {
		t.Fatalf("不应改调度状态: status=%q failures=%d", got.Status, got.Failures)
	}
	if got.Theos == nil || got.Theos.CheckedAt != stale {
		t.Fatal("失败不应改写额度快照")
	}
}
