package poolsvc

import (
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"deepaicode2api/internal/model"
)

// --- 账号状态转移 ---
//
// 从 internal/store 迁入：这些用例锁的是「一次调用结果/一次探测结果该
// 怎么记到账号上」的业务判定（瞬时失败不标 error、策略拦截要 blocked、
// 探测失败要退避），代码已搬到 internal/poolsvc/accountstate.go。

func transientErr() error {
	return &model.HTTPError{Op: "上游", Status: http.StatusTooManyRequests, Body: []byte(`{}`)}
}

func hardErr() error {
	return &model.HTTPError{Op: "上游", Status: http.StatusUnauthorized,
		Body: []byte(`{"error":{"code":"invalid_api_key"}}`)}
}

func TestMarkAccountFailureTransientKeepsStatus(t *testing.T) {
	st := newTestStore(t)
	p := newService(t, st, nil)
	st.UpsertAccount(&model.Account{ID: "a1", APIKey: "k1", Enabled: true, Status: "ok"})

	p.MarkFailure("a1", transientErr())

	a := st.Accounts()[0]
	if a.Status != "ok" {
		t.Fatalf("瞬时失败不应改状态，实际 %q", a.Status)
	}
	if a.Failures != 1 || a.LastErr == "" || a.LastErrAt == 0 {
		t.Fatalf("失败历史仍应记录：failures=%d lastErr=%q lastErrAt=%d", a.Failures, a.LastErr, a.LastErrAt)
	}
}

func TestMarkAccountFailureHardMarksError(t *testing.T) {
	st := newTestStore(t)
	p := newService(t, st, nil)
	st.UpsertAccount(&model.Account{ID: "a1", APIKey: "k1", Enabled: true, Status: "ok"})

	p.MarkFailure("a1", hardErr())

	a := st.Accounts()[0]
	if a.Status != model.StatusError {
		t.Fatalf("账号级失败应标 error，实际 %q", a.Status)
	}
	if a.LastErrAt == 0 {
		t.Fatal("标记 error 时必须写 last_err_at，否则无法衰减")
	}
}

func TestMarkAccountSuccessClearsError(t *testing.T) {
	st := newTestStore(t)
	p := newService(t, st, nil)
	st.UpsertAccount(&model.Account{ID: "a1", APIKey: "k1", Enabled: true,
		Status: model.StatusError, LastErr: "旧错误", LastErrAt: time.Now().Unix()})

	p.MarkResult("a1", true, "")

	a := st.Accounts()[0]
	if a.Status != "ok" || a.LastErr != "" || a.LastErrAt != 0 || a.Failures != 0 {
		t.Fatalf("成功应彻底清掉错误痕迹：status=%q lastErr=%q lastErrAt=%d failures=%d",
			a.Status, a.LastErr, a.LastErrAt, a.Failures)
	}
}

func TestRecoverStaleErrors(t *testing.T) {
	st := newTestStore(t)
	p := newService(t, st, nil)
	stale := time.Now().Add(-model.ErrorStaleAfter - time.Minute).Unix()
	fresh := time.Now().Unix()
	st.UpsertAccount(&model.Account{ID: "stale", APIKey: "k-stale", Enabled: true, Status: model.StatusError, LastErrAt: stale})
	st.UpsertAccount(&model.Account{ID: "fresh", APIKey: "k-fresh", Enabled: true, Status: model.StatusError, LastErrAt: fresh})
	st.UpsertAccount(&model.Account{ID: "blocked", APIKey: "k-blocked", Enabled: true, Status: model.StatusBlocked, LastErrAt: stale})
	// 老状态文件没有 last_err_at：也要能自愈。
	st.UpsertAccount(&model.Account{ID: "legacy", APIKey: "k-legacy", Enabled: true, Status: model.StatusError})

	if n := p.RecoverStaleErrors(time.Now()); n != 2 {
		t.Fatalf("应自愈 2 个（stale + legacy），实际 %d", n)
	}
	byID := map[string]*model.Account{}
	for _, a := range st.Accounts() {
		byID[a.ID] = a
	}
	if byID["stale"].Status != "ok" || byID["legacy"].Status != "ok" {
		t.Fatalf("陈旧错误应置回 ok：stale=%q legacy=%q", byID["stale"].Status, byID["legacy"].Status)
	}
	if byID["fresh"].Status != model.StatusError {
		t.Fatalf("新鲜错误不该被动：%q", byID["fresh"].Status)
	}
	if byID["blocked"].Status != model.StatusBlocked {
		t.Fatalf("blocked 不会自愈，不该被改：%q", byID["blocked"].Status)
	}
	// 幂等：再跑一次没有可改的。
	if n := p.RecoverStaleErrors(time.Now()); n != 0 {
		t.Fatalf("第二次应为 0，实际 %d", n)
	}
}

func TestSetAccountAllowanceTransientKeepsStatus(t *testing.T) {
	st := newTestStore(t)
	p := newService(t, st, nil)
	st.UpsertAccount(&model.Account{ID: "a1", APIKey: "k1", Enabled: true, Status: "ok"})

	// 探测失败（EOF）：不该把好号标成 error，但要退避。
	p.SetAllowance("a1", nil, errors.New("Get \"https://api.deepai.org\": unexpected EOF"))

	a := st.Accounts()[0]
	if a.Status != "ok" {
		t.Fatalf("瞬时探测失败不应改状态，实际 %q", a.Status)
	}
	if a.ProbeFailures != 1 || a.ProbeAfter == 0 {
		t.Fatalf("仍应累加探测退避：probeFailures=%d probeAfter=%d", a.ProbeFailures, a.ProbeAfter)
	}

	// 账号级失败要标 error。
	p.SetAllowance("a1", nil, hardErr())
	if got := st.Accounts()[0].Status; got != model.StatusError {
		t.Fatalf("账号级探测失败应标 error，实际 %q", got)
	}
}

// LockAccountProbe 只加探测冷却与备注：不改状态、不累加失败次数。
//
// check_in_refused 这类错误下账号仍可推理（只是落在替身模型分支），
// 标成 error 虽然不影响调度（调度器只跳过 blocked），但会污染健康度展示；
// 真正要防的是后台守护每 5 分钟又来磕一次 403 —— 即复探风暴
// （docs/performance.md §14）。
func TestLockAccountProbeKeepsStatus(t *testing.T) {
	st := newTestStore(t)
	p := newService(t, st, nil)
	st.UpsertAccount(&model.Account{ID: "a1", APIKey: "k1", Enabled: true, Status: "ok"})
	p.SetAllowance("a1", &model.Allowance{Enabled: true}, nil)
	if err := st.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	until := time.Now().Add(6 * time.Hour).Truncate(time.Second)
	p.LockProbe("a1", until, "check_in_refused: too many restarts today")

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
	st := newTestStore(t)
	p := newService(t, st, nil)
	st.UpsertAccount(&model.Account{ID: "a1", APIKey: "k1", Enabled: true})
	far := time.Now().Add(12 * time.Hour).Truncate(time.Second)
	p.LockProbe("a1", far, "")
	p.LockProbe("a1", time.Now().Add(time.Minute), "")
	if got := st.Accounts()[0].ProbeAfter; got != far.Unix() {
		t.Fatalf("冷却被缩短: %d，期望 %d", got, far.Unix())
	}
}

// 未知账号不 panic、不产生副作用。
func TestLockAccountProbeUnknown(t *testing.T) {
	st := newTestStore(t)
	p := newService(t, st, nil)
	p.LockProbe("nope", time.Now().Add(time.Hour), "x")
	if got := st.Accounts(); len(got) != 0 {
		t.Fatalf("不应新建账号: %+v", got)
	}
}

// 探测失败按次退避 2m → 10m → 1h，成功即清零。
func TestProbeCooldownBackoff(t *testing.T) {
	st := newTestStore(t)
	p := newService(t, st, nil)
	st.UpsertAccount(&model.Account{ID: "a1", APIKey: "k1", Enabled: true})

	steps := []time.Duration{2 * time.Minute, 10 * time.Minute, time.Hour, time.Hour}
	for i, want := range steps {
		p.SetAllowance("a1", nil, errors.New("查询 Theos: HTTP 502"))
		a := st.Accounts()[0]
		if a.ProbeFailures != i+1 {
			t.Fatalf("第 %d 次失败后 ProbeFailures = %d", i+1, a.ProbeFailures)
		}
		got := time.Until(time.Unix(a.ProbeAfter, 0))
		if got < want-time.Minute || got > want+time.Minute {
			t.Fatalf("第 %d 次失败后冷却 = %v，期望约 %v", i+1, got, want)
		}
	}

	p.SetAllowance("a1", &model.Allowance{Enabled: true}, nil)
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
	st := newTestStore(t)
	p := newService(t, st, nil)
	st.UpsertAccount(&model.Account{
		ID: "a1", APIKey: "k1", Enabled: true, Status: "ok",
		Theos: &model.Allowance{Enabled: true, CheckedAt: time.Now().Add(-time.Hour).Unix()},
	})

	p.RecordProbeFailure("a1")

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
		p.RecordProbeFailure("a1")
		a = st.Accounts()[0]
		left = time.Until(time.Unix(a.ProbeAfter, 0))
		if left < want-time.Minute || left > want+time.Minute {
			t.Fatalf("第 %d 次失败冷却 = %v，期望约 %v", i+2, left, want)
		}
	}

	// 探测成功（任何写入额度的路径）都要把退避清零。
	p.SetAllowance("a1", &model.Allowance{Enabled: true}, nil)
	a = st.Accounts()[0]
	if a.ProbeAfter != 0 || a.ProbeFailures != 0 {
		t.Fatalf("成功应清零退避，实际 after=%d failures=%d", a.ProbeAfter, a.ProbeFailures)
	}
}

// 未知账号不 panic、不影响其它账号。
func TestRecordProbeFailureUnknownAccount(t *testing.T) {
	st := newTestStore(t)
	p := newService(t, st, nil)
	st.UpsertAccount(&model.Account{ID: "a1", APIKey: "k1", Enabled: true})
	p.RecordProbeFailure("nope")
	if a := st.Accounts()[0]; a.ProbeFailures != 0 || a.ProbeAfter != 0 {
		t.Fatalf("不应影响其它账号: %+v", a)
	}
}

func TestMarkAccountResultResetsFailures(t *testing.T) {
	st := newTestStore(t)
	p := newService(t, st, nil)
	seedAccounts(t, st, 1)
	a := st.Account("acc-0")
	p.MarkResult(a.ID, false, "boom")
	p.MarkResult(a.ID, false, "boom2")
	if got := st.Account(a.ID); got.Failures != 2 || got.Status != "error" || got.LastErr != "boom2" {
		t.Fatalf("失败计数错误: %#v", got)
	}

	p.MarkResult(a.ID, true, "")
	if got := st.Account(a.ID); got.Failures != 0 || got.Status != "ok" || got.LastErr != "" {
		t.Fatalf("成功后应清零: %#v", got)
	}
}
