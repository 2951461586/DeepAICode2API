package poolsvc

import (
	"testing"

	"deepaicode2api/internal/deepai"
)

// TestPickAvoidingFallsBackWhenAllAvoided 固化「换出口优先、弃号其次」：
// 有别的候选就绕开被 avoid 排除的号；一个不剩时必须退回常规调度，
// 而不是对外报「没有可调度账号」（那会变成 429）。
func TestPickAvoidingFallsBackWhenAllAvoided(t *testing.T) {
	st := newTestStore(t)
	seedAccounts(t, st, 3)
	for i, id := range []string{"acc-0", "acc-1", "acc-2"} {
		proxy := []string{"http://e0:1", "http://e1:1", "http://e2:1"}[i]
		if !st.UpdateAccount(id, func(a *deepai.Account) { a.Proxy = proxy }) {
			t.Fatalf("写回账号 %s 失败", id)
		}
	}
	svc := newService(t, st, nil)

	// 只排除第一个出口：应选到另外两个之一。
	avoided := "http://e0:1"
	var picked []string
	for i := 0; i < 2; i++ {
		a, ok, candidates := svc.PickAvoiding("", "", func(a *deepai.Account) bool { return a.Proxy == avoided })
		if !ok || !candidates {
			t.Fatalf("第 %d 次选号失败 ok=%v candidates=%v", i, ok, candidates)
		}
		picked = append(picked, a.Proxy)
		svc.Release(a.ID)
	}
	for _, p := range picked {
		if p == avoided {
			t.Fatalf("选到了被排除的出口 %s（实际 %v）", p, picked)
		}
	}

	// 排除全部出口：必须退化回常规调度，仍然给出一个号。
	all := func(*deepai.Account) bool { return true }
	a, ok, candidates := svc.PickAvoiding("", "", all)
	if !ok || !candidates || a == nil {
		t.Fatal("全部被排除时应回退常规调度，而不是报无可用账号")
	}
	svc.Release(a.ID)

	// 全忙（槽位用尽）：ok=false；candidates 仍为 true，表示「池里还有可选账号，只是都在忙」——
	// 调用方据此排队而不是报「没有可调度账号」。
	for _, id := range []string{"acc-0", "acc-1", "acc-2"} {
		svc.Acquire(id)
		svc.Acquire(id)
	}
	if _, ok, candidates := svc.PickAvoiding("", "", func(*deepai.Account) bool { return false }); ok || !candidates {
		t.Fatalf("全忙时应 ok=false candidates=true，实际 ok=%v candidates=%v", ok, candidates)
	}
}

// TestPickAvoidingHonoursStickyUnlessAvoided 固化「会话粘性让位于出口冷却」：
// 上次用的号如果出口正在冷却，就不必再优先它。
func TestPickAvoidingHonoursStickyUnlessAvoided(t *testing.T) {
	st := newTestStore(t)
	seedAccounts(t, st, 2)
	svc := newService(t, st, nil)

	sticky := "acc-0"
	a, ok, _ := svc.PickAvoiding("", sticky, nil)
	if !ok {
		t.Fatal("首次选号失败")
	}
	svc.Release(a.ID)

	// 不排除：粘性生效，仍选 acc-0。
	a, ok, _ = svc.PickAvoiding("", sticky, nil)
	if !ok || a.ID != sticky {
		t.Fatalf("粘性未生效：%v", a)
	}
	svc.Release(a.ID)

	// 排除粘性号对应的出口：应换到另一个号。
	a, ok, _ = svc.PickAvoiding("", sticky, func(x *deepai.Account) bool { return x.ID == sticky })
	if !ok || a.ID == sticky {
		t.Fatalf("粘性号被排除后仍选了它：%v", a)
	}
	svc.Release(a.ID)
}
