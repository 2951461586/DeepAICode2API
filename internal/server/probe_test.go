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
