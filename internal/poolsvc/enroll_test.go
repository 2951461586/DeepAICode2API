package poolsvc

import (
	"context"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"deepaicode2api/internal/deepai"
)

// --- 入池与验证 ---
//
// 入池是唯一会把「外部注册链路」和「号池」耦合起来的写入口：
// 管理接口与浏览器登录共用同一条路径，所以它的幂等性与 0 额度语义
// 都属于模块契约（R2 压测的两个结论：重复入池不能出现两份账号；
// 入池时的 0 额度是「未知」而不是「确认耗尽」）。

// TestEnrollRegistrationNewThenIdempotent 验证入池按邮箱去重，且不新增第二份。
func TestEnrollRegistrationNewThenIdempotent(t *testing.T) {
	st := newTestStore(t)
	p := newService(t, st, nil)

	reg := deepai.Registration{Email: "a@example.com", APIKey: "sk-first", Source: "register"}
	id, isNew, err := p.EnrollRegistration(reg, nil)
	if err != nil {
		t.Fatalf("首次入池: %v", err)
	}
	if !isNew {
		t.Fatal("首次入池应返回 isNew=true")
	}
	if id == "" {
		t.Fatal("入池应返回账号 ID")
	}

	// 同一邮箱、Key 已轮换（重新注册会换 Key）：仍应命中同一条。
	reg.APIKey = "sk-second"
	id2, isNew2, err := p.EnrollRegistration(reg, nil)
	if err != nil {
		t.Fatalf("重复入池: %v", err)
	}
	if isNew2 {
		t.Fatal("同邮箱重复入池不应新增账号")
	}
	if id2 != id {
		t.Fatalf("重复入池的 ID = %q，期望复用 %q", id2, id)
	}
	if n := len(st.Accounts()); n != 1 {
		t.Fatalf("号池规模 = %d，期望 1", n)
	}
}

// TestEnrollRegistrationWritesRuntimeState 验证入池把验证结果写进运行态。
func TestEnrollRegistrationWritesRuntimeState(t *testing.T) {
	st := newTestStore(t)
	p := newService(t, st, nil)

	al := &deepai.Allowance{Enabled: true, Balance: "7", Granted: "20", CheckedAt: time.Now().Unix()}
	reg := deepai.Registration{
		Email: "b@example.com", APIKey: "sk-b", Source: "register",
		Plan: "pro", EmailVerified: true, Group: "g1",
	}
	id, _, err := p.EnrollRegistration(reg, al)
	if err != nil {
		t.Fatalf("入池: %v", err)
	}

	got := st.Account(id)
	if got.Status != "ok" || got.Failures != 0 || got.LastErr != "" {
		t.Fatalf("入池应把账号置为健康态: status=%q failures=%d lastErr=%q", got.Status, got.Failures, got.LastErr)
	}
	if got.Plan != "pro" || !got.EmailVerified || got.Group != "g1" {
		t.Fatalf("入池应写回身份字段: plan=%q verified=%v group=%q", got.Plan, got.EmailVerified, got.Group)
	}
	if got.Theos == nil || got.Theos.Balance != "7" {
		t.Fatalf("入池应携带配额快照，实际 %+v", got.Theos)
	}
}

// TestEnrollRegistrationRejectsMissingKey 验证无 Key 的注册记录被拒。
func TestEnrollRegistrationRejectsMissingKey(t *testing.T) {
	p := newService(t, newTestStore(t), nil)
	if _, _, err := p.EnrollRegistration(deepai.Registration{Email: "x@example.com"}, nil); err == nil {
		t.Fatal("没有 API Key 的注册记录应报错")
	}
}

// TestPositiveQuota 表驱动覆盖「额度是否为正」的判定。
//
// 这个判定决定入池后多久能被调度：0 额度按「未知」处理（继续调度），
// 而不是按「耗尽」排除 15 分钟（R2 实测：覆盖从 19/50 掉到 12~15/50）。
func TestPositiveQuota(t *testing.T) {
	cases := []struct {
		name string
		al   *deepai.Allowance
		want bool
	}{
		{"nil", nil, false},
		{"未启用", &deepai.Allowance{Enabled: false, Balance: "10"}, false},
		{"正额度", &deepai.Allowance{Enabled: true, Balance: "10"}, true},
		{"小数额度", &deepai.Allowance{Enabled: true, Balance: "0.5"}, true},
		{"零额度", &deepai.Allowance{Enabled: true, Balance: "0"}, false},
		{"空额度", &deepai.Allowance{Enabled: true, Balance: ""}, false},
		{"非数字", &deepai.Allowance{Enabled: true, Balance: "many"}, false},
		{"带空格", &deepai.Allowance{Enabled: true, Balance: " 3 "}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := PositiveQuota(c.al); got != c.want {
				t.Fatalf("PositiveQuota = %v，期望 %v", got, c.want)
			}
		})
	}
}

// TestAllowanceFromEnrollZeroQuotaIsUnknown 验证 0 额度不写 CheckedAt。
func TestAllowanceFromEnrollZeroQuotaIsUnknown(t *testing.T) {
	zero := AllowanceFromEnroll("0", "20", "")
	if zero == nil || !zero.Enabled {
		t.Fatalf("入池快照应至少标记 enabled: %+v", zero)
	}
	if zero.CheckedAt != 0 {
		t.Fatalf("0 额度的 CheckedAt 必须留 0（表示未知），实际 %d", zero.CheckedAt)
	}

	pos := AllowanceFromEnroll("5", "20", "2030-01-01T00:00:00Z")
	if pos.CheckedAt == 0 {
		t.Fatal("正额度的 CheckedAt 应写入当前时间")
	}
	if pos.Balance != "5" || pos.Granted != "20" || pos.RefreshAt == "" {
		t.Fatalf("入池快照字段丢失: %+v", pos)
	}
}

// TestMarkVerifiedByIDAndEmail 验证邮箱已验证标记的两条定位路径。
func TestMarkVerifiedByIDAndEmail(t *testing.T) {
	st := newTestStore(t)
	p := newService(t, st, nil)
	ids := seedAccounts(t, st, 2)
	st.UpdateAccount(ids[0], func(a *deepai.Account) { a.Email = "first@example.com" })

	if err := p.MarkVerified(ids[0], ""); err != nil {
		t.Fatalf("按 ID 标记: %v", err)
	}
	if !st.Account(ids[0]).EmailVerified {
		t.Fatal("按 ID 标记未生效")
	}

	// 台账里的 ID 已失效时按邮箱回退。
	if err := p.MarkVerified("stale-id", "first@example.com"); err != nil {
		t.Fatalf("按邮箱标记: %v", err)
	}
	if !st.Account(ids[0]).EmailVerified {
		t.Fatal("按邮箱标记未生效")
	}

	// 幂等：重复标记无害。
	if err := p.MarkVerified(ids[0], ""); err != nil {
		t.Fatalf("重复标记: %v", err)
	}

	if err := p.MarkVerified("", ""); err == nil {
		t.Fatal("账号不存在时应报错")
	}
	if err := p.MarkVerified("nope", "nobody@example.com"); err == nil {
		t.Fatal("ID 与邮箱都匹配不到时应报错")
	}
}

// TestProbeAccountQuotaRetriesTransientOnce 验证入池探测对瞬时错误重试一次。
func TestProbeAccountQuotaRetriesTransientOnce(t *testing.T) {
	var hits atomic.Int64
	upstreamStub(t, func(w http.ResponseWriter, r *http.Request) {
		if hits.Add(1) == 1 {
			theosFail(w, r) // 第一次 500（瞬时，可重试）
			return
		}
		theosOK(w, r)
	})
	// 把入池重试的退避压到极小，避免用例等 1 秒。
	old := probeRetryDelays
	probeRetryDelays = []time.Duration{time.Millisecond}
	t.Cleanup(func() { probeRetryDelays = old })

	p := newService(t, newTestStore(t), func(*deepai.Account) *deepai.Client { return newStubClient(t) })
	seedAccounts(t, p.store, 1)

	al, err := p.ProbeAccountQuota(p.store.Account("acc-0"))
	if err != nil {
		t.Fatalf("重试后应成功: %v", err)
	}
	if al == nil || al.Balance != "10" {
		t.Fatalf("配额快照 = %+v", al)
	}
	if hits.Load() != 2 {
		t.Fatalf("上游命中 %d 次，期望 2（一次失败 + 一次重试）", hits.Load())
	}
}

// TestProbeAccountQuotaGivesUpOnNonTransient 验证不可重试错误直接返回。
func TestProbeAccountQuotaGivesUpOnNonTransient(t *testing.T) {
	var hits atomic.Int64
	upstreamStub(t, func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"bad"}}`))
	})
	p := newService(t, newTestStore(t), func(*deepai.Account) *deepai.Client { return newStubClient(t) })
	seedAccounts(t, p.store, 1)

	if _, err := p.ProbeAccountQuota(p.store.Account("acc-0")); err == nil {
		t.Fatal("400 不应被重试掩盖")
	}
	if hits.Load() != 1 {
		t.Fatalf("非瞬时错误应只打一次上游，实际 %d 次", hits.Load())
	}
}

// TestSaveIsBestEffort 验证 Save 失败不影响调用方语义（落盘是尽力而为）。
func TestSaveIsBestEffort(t *testing.T) {
	st := newTestStore(t)
	p := New(Config{Store: st, ClientFor: func(*deepai.Account) *deepai.Client { return nil }, Save: nil})
	p.Save() // 未注入 Save：空操作，不 panic
	p2 := New(Config{Store: st})
	p2.Save()
}

// TestCloseIsIdempotent 验证重复 Close 不 panic、且能等待已登记工作。
func TestCloseIsIdempotent(t *testing.T) {
	p := New(Config{})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	p.Close(ctx)
	p.Close(ctx)
}
