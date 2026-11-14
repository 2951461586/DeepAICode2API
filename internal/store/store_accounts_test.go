package store

import (
	"errors"
	"strings"
	"testing"
	"time"

	"deepaicode2api/internal/deepai"
)

// LockAccountProbe 只加探测冷却与备注：不改状态、不累加失败次数。
//
// check_in_refused 这类错误下账号仍可推理（只是落在替身模型分支），
// 标成 error 虽然不影响调度（调度器只跳过 blocked），但会污染健康度展示；
// 真正要防的是后台守护每 5 分钟又来磕一次 403 —— 即复探风暴
// （docs/performance.md §14）。
func TestLockAccountProbeKeepsStatus(t *testing.T) {
	st := newStore(t)
	st.UpsertAccount(&deepai.Account{ID: "a1", APIKey: "k1", Enabled: true, Status: "ok"})
	st.SetAccountAllowance("a1", &deepai.Allowance{Enabled: true}, nil)
	if err := st.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	until := time.Now().Add(6 * time.Hour).Truncate(time.Second)
	st.LockAccountProbe("a1", until, "check_in_refused: too many restarts today")

	a := st.Accounts()[0]
	if a.Status != "ok" {
		t.Fatalf("状态不应变化，实际 %q", a.Status)
	}
	if a.Failures != 0 {
		t.Fatalf("失败计数不应增加，实际 %d", a.Failures)
	}
	if a.ProbeAfter != until.Unix() {
		t.Fatalf("探测冷却 = %d，期望 %d", a.ProbeAfter, until.Unix())
	}
	if !strings.Contains(a.LastErr, "check_in_refused") {
		t.Fatalf("备注未写入: %q", a.LastErr)
	}
	if a.Theos == nil {
		t.Fatal("额度快照不应被清掉")
	}
}

// 已有更长的冷却时，短锁不能把它缩短。
func TestLockAccountProbeKeepsLongerCooldown(t *testing.T) {
	st := newStore(t)
	st.UpsertAccount(&deepai.Account{ID: "a1", APIKey: "k1", Enabled: true})
	far := time.Now().Add(12 * time.Hour).Truncate(time.Second)
	st.LockAccountProbe("a1", far, "")
	st.LockAccountProbe("a1", time.Now().Add(time.Minute), "")
	if got := st.Accounts()[0].ProbeAfter; got != far.Unix() {
		t.Fatalf("冷却被缩短: %d，期望 %d", got, far.Unix())
	}
}

// 未知账号不 panic、不产生副作用。
func TestLockAccountProbeUnknown(t *testing.T) {
	st := newStore(t)
	st.LockAccountProbe("nope", time.Now().Add(time.Hour), "x")
	if got := st.Accounts(); len(got) != 0 {
		t.Fatalf("不应新建账号: %+v", got)
	}
}

// 探测失败按次退避 2m → 10m → 1h，成功即清零。
func TestProbeCooldownBackoff(t *testing.T) {
	st := newStore(t)
	st.UpsertAccount(&deepai.Account{ID: "a1", APIKey: "k1", Enabled: true})

	steps := []time.Duration{2 * time.Minute, 10 * time.Minute, time.Hour, time.Hour}
	for i, want := range steps {
		st.SetAccountAllowance("a1", nil, errors.New("查询 Theos: HTTP 502"))
		a := st.Accounts()[0]
		if a.ProbeFailures != i+1 {
			t.Fatalf("第 %d 次失败后 ProbeFailures = %d", i+1, a.ProbeFailures)
		}
		got := time.Until(time.Unix(a.ProbeAfter, 0))
		if got < want-time.Minute || got > want+time.Minute {
			t.Fatalf("第 %d 次失败后冷却 = %v，期望约 %v", i+1, got, want)
		}
	}

	st.SetAccountAllowance("a1", &deepai.Allowance{Enabled: true}, nil)
	a := st.Accounts()[0]
	if a.ProbeAfter != 0 || a.ProbeFailures != 0 {
		t.Fatalf("成功应清零冷却，实际 after=%d failures=%d", a.ProbeAfter, a.ProbeFailures)
	}
}

// RecordProbeFailure 只推进探测退避：线上热路径的按需复探失败时，
// 不能把号标坏（Status/Failures 原样），但必须让 ProbeAfter 前进，
// 否则 quotaNeedsRefresh 永远 true，每个选中该号的请求都会再探一次
// （docs/changes.md §61：同一账号 323 次/小时）。
func TestRecordProbeFailureBacksOffOnlyProbe(t *testing.T) {
	st := newStore(t)
	st.UpsertAccount(&deepai.Account{
		ID: "a1", APIKey: "k1", Enabled: true, Status: "ok",
		Theos: &deepai.Allowance{Enabled: true, CheckedAt: time.Now().Add(-time.Hour).Unix()},
	})

	st.RecordProbeFailure("a1")

	a := st.Accounts()[0]
	if a.ProbeFailures != 1 {
		t.Fatalf("ProbeFailures = %d，期望 1", a.ProbeFailures)
	}
	left := time.Until(time.Unix(a.ProbeAfter, 0))
	if left < 90*time.Second || left > 3*time.Minute {
		t.Fatalf("首次失败冷却 = %v，期望约 2 分钟", left)
	}
	if a.Status != "ok" || a.Failures != 0 {
		t.Fatalf("不应改调度状态: status=%q failures=%d", a.Status, a.Failures)
	}
	if a.Theos == nil {
		t.Fatal("额度快照不应被清掉")
	}

	// 第二次失败要拉长到 10 分钟，第三次封顶 1 小时。
	for i, want := range []time.Duration{10 * time.Minute, time.Hour} {
		st.RecordProbeFailure("a1")
		a = st.Accounts()[0]
		left = time.Until(time.Unix(a.ProbeAfter, 0))
		if left < want-time.Minute || left > want+time.Minute {
			t.Fatalf("第 %d 次失败冷却 = %v，期望约 %v", i+2, left, want)
		}
	}

	// 探测成功（任何写入额度的路径）都要把退避清零。
	st.SetAccountAllowance("a1", &deepai.Allowance{Enabled: true}, nil)
	a = st.Accounts()[0]
	if a.ProbeAfter != 0 || a.ProbeFailures != 0 {
		t.Fatalf("成功应清零退避，实际 after=%d failures=%d", a.ProbeAfter, a.ProbeFailures)
	}
}

// 未知账号不 panic、不影响其它账号。
func TestRecordProbeFailureUnknownAccount(t *testing.T) {
	st := newStore(t)
	st.UpsertAccount(&deepai.Account{ID: "a1", APIKey: "k1", Enabled: true})
	st.RecordProbeFailure("nope")
	if a := st.Accounts()[0]; a.ProbeFailures != 0 || a.ProbeAfter != 0 {
		t.Fatalf("不应影响其它账号: %+v", a)
	}
}
