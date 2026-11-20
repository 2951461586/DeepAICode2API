package poolsvc

import (
	"context"
	"net/http"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"deepaicode2api/internal/deepai"
)

// --- 配额探测 ---
//
// 探测链路的三个不变量：
//   1. 去重：同一个号同时只能有一次探测（三条路径会撞车）；
//   2. 限流：全局在途探测不超过 probeWorkers（否则探针本身变成风控特征）；
//   3. 失败语义：自动复探失败只推进退避，不把正在服务的号标成 error。

// TestNeedsRefresh 表驱动覆盖额度新鲜度判定的每一档。
func TestNeedsRefresh(t *testing.T) {
	now := time.Now()
	cases := []struct {
		name       string
		acct       *deepai.Account
		staleAfter time.Duration
		want       bool
	}{
		{"nil 账号", nil, time.Hour, false},
		{"停用账号", &deepai.Account{Enabled: false}, time.Hour, false},
		{"从未探测", &deepai.Account{Enabled: true}, time.Hour, true},
		{
			"刚刚探测过（严格模式不重探）",
			&deepai.Account{Enabled: true, Theos: &deepai.Allowance{CheckedAt: now.Unix()}},
			0, false,
		},
		{
			"超出 staleAfter",
			&deepai.Account{Enabled: true, Theos: &deepai.Allowance{CheckedAt: now.Add(-2 * time.Hour).Unix()}},
			time.Hour, true,
		},
		{
			"未超 staleAfter",
			&deepai.Account{Enabled: true, Theos: &deepai.Allowance{CheckedAt: now.Add(-30 * time.Minute).Unix()}},
			time.Hour, false,
		},
		{
			"refresh_at 已过（额度刚刷新）",
			&deepai.Account{Enabled: true, Theos: &deepai.Allowance{
				CheckedAt: now.Unix(),
				RefreshAt: now.Add(-time.Minute).Format(time.RFC3339),
			}},
			time.Hour, true,
		},
		{
			"refresh_at 未到",
			&deepai.Account{Enabled: true, Theos: &deepai.Allowance{
				CheckedAt: now.Unix(),
				RefreshAt: now.Add(time.Hour).Format(time.RFC3339),
			}},
			time.Hour, false,
		},
		{
			"探测冷却中",
			&deepai.Account{Enabled: true, ProbeAfter: now.Add(time.Minute).Unix()},
			time.Hour, false,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := NeedsRefresh(c.acct, now, c.staleAfter); got != c.want {
				t.Fatalf("NeedsRefresh = %v，期望 %v", got, c.want)
			}
		})
	}
}

// TestSortByDue 验证探测优先级：refresh_at 已过 → 从未探测 → 单纯过旧。
func TestSortByDue(t *testing.T) {
	now := time.Now()
	stale := &deepai.Account{ID: "stale", Enabled: true, Theos: &deepai.Allowance{CheckedAt: now.Add(-2 * time.Hour).Unix()}}
	never := &deepai.Account{ID: "never", Enabled: true}
	due := &deepai.Account{ID: "due", Enabled: true, Theos: &deepai.Allowance{
		CheckedAt: now.Unix(),
		RefreshAt: now.Add(-time.Minute).Format(time.RFC3339),
	}}

	// 同级按 checked_at 升序：两个「从未探测」中 checked_at 都是 0，保持稳定顺序。
	got := []*deepai.Account{stale, never, due}
	SortByDue(got, now)
	want := []string{"due", "never", "stale"}
	for i, id := range want {
		if got[i].ID != id {
			t.Fatalf("第 %d 位 = %s，期望 %s（顺序 %v）", i, got[i].ID, id, idsOf(got))
		}
	}
}

func idsOf(accts []*deepai.Account) []string {
	out := make([]string, len(accts))
	for i, a := range accts {
		out[i] = a.ID
	}
	return out
}

// TestClaimProbeDedup 验证同一账号的并发探测被去重。
func TestClaimProbeDedup(t *testing.T) {
	p := New(Config{})

	if !p.ClaimProbe("acc-0") {
		t.Fatal("首次 ClaimProbe 应成功")
	}
	if p.ClaimProbe("acc-0") {
		t.Fatal("同一账号第二次 ClaimProbe 应失败（去重）")
	}
	if n := p.Probing(); n != 1 {
		t.Fatalf("Probing = %d，期望 1", n)
	}
	if !p.ClaimProbe("acc-1") {
		t.Fatal("不同账号应能同时探测")
	}

	p.ReleaseProbe("acc-0")
	if !p.ClaimProbe("acc-0") {
		t.Fatal("ReleaseProbe 后应能再次占用")
	}
	if p.ClaimProbe("") {
		t.Fatal("空 ID 不应占用探测槽位")
	}
}

// TestProbeBatchBoundedConcurrency 验证批量探测的并发上限。
func TestProbeBatchBoundedConcurrency(t *testing.T) {
	meter := &concurrencyMeter{}
	g := newGate(probeWorkers * 2)
	upstreamStub(t, meter.track(func(w http.ResponseWriter, r *http.Request) {
		g.wait(t)
		theosOK(w, r)
	}))
	p := newService(t, newTestStore(t), func(*deepai.Account) *deepai.Client { return newStubClient(t) })
	seedAccounts(t, p.store, probeWorkers*2)

	accts := p.store.Accounts()
	go func() {
		// 等并发打满再一次性放行，避免用例挂住。
		for i := 0; i < probeWorkers; i++ {
			<-g.entered
		}
		g.open()
	}()
	out := p.ProbeBatch(context.Background(), accts, 5*time.Second, false)

	if len(out) != len(accts) {
		t.Fatalf("结果条数 = %d，期望 %d（按下标一一对应）", len(out), len(accts))
	}
	for i, o := range out {
		if o.Account == nil || o.Account.ID != accts[i].ID {
			t.Fatalf("第 %d 项账号 = %+v，期望 %s", i, o.Account, accts[i].ID)
		}
		if o.Skipped || o.Err != nil {
			t.Fatalf("第 %d 项不应跳过/失败: skipped=%v err=%v", i, o.Skipped, o.Err)
		}
	}
	if peak := meter.peak.Load(); peak > int64(probeWorkers) {
		t.Fatalf("探测并发峰值 = %d，超过上限 %d", peak, probeWorkers)
	}
}

// TestProbeBatchSkipsInFlightAccount 验证已有探测在跑的账号被标记 skipped。
func TestProbeBatchSkipsInFlightAccount(t *testing.T) {
	upstreamStub(t, theosOK)
	p := newService(t, newTestStore(t), func(*deepai.Account) *deepai.Client { return newStubClient(t) })
	seedAccounts(t, p.store, 2)
	accts := p.store.Accounts()

	if !p.ClaimProbe(accts[0].ID) {
		t.Fatal("预占用失败")
	}
	defer p.ReleaseProbe(accts[0].ID)

	out := p.ProbeBatch(context.Background(), accts, 5*time.Second, false)
	if !out[0].Skipped {
		t.Fatal("已在探测中的账号应被跳过")
	}
	if out[1].Skipped {
		t.Fatal("未在探测中的账号不应被跳过")
	}
}

// TestProbeBatchAutoFailureOnlyBacksOff 验证「自动复探失败只推进退避」。
//
// 这是线上踩过的坑（docs/changes.md §61：同一账号 323 次/小时 /theos 风暴）：
// 失败若既不改状态也不记退避，NeedsRefresh 会永远为 true，热路径每个请求都再探一次。
func TestProbeBatchAutoFailureOnlyBacksOff(t *testing.T) {
	upstreamStub(t, theosFail)
	p := newService(t, newTestStore(t), func(*deepai.Account) *deepai.Client { return newStubClient(t) })
	seedAccounts(t, p.store, 1)
	accts := p.store.Accounts()

	out := p.ProbeBatch(context.Background(), accts, 5*time.Second, false)
	if out[0].Err == nil {
		t.Fatal("假上游返回 500，应记录错误")
	}

	got := p.store.Account(accts[0].ID)
	if got.Status == deepai.StatusError {
		t.Fatal("自动复探的瞬时失败不应把账号标成 error")
	}
	if got.ProbeFailures == 0 {
		t.Fatal("自动复探失败必须推进 ProbeFailures")
	}
	if got.ProbeAfter <= time.Now().Unix() {
		t.Fatal("自动复探失败必须留下冷却（否则热路径会反复重探）")
	}
}

// TestProbeBatchStrictFailureMarksStatus 验证手动探测（strict）会落状态。
func TestProbeBatchStrictFailureMarksStatus(t *testing.T) {
	upstreamStub(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest) // 非瞬时错误
		_, _ = w.Write([]byte(`{"error":{"message":"bad request"}}`))
	})
	p := newService(t, newTestStore(t), func(*deepai.Account) *deepai.Client { return newStubClient(t) })
	seedAccounts(t, p.store, 1)

	p.ProbeBatch(context.Background(), p.store.Accounts(), 5*time.Second, true)
	got := p.store.Account("acc-0")
	if got.Status != deepai.StatusError {
		t.Fatalf("手动探测的非瞬时失败应标 error，实际 %q", got.Status)
	}
	if got.LastErr == "" {
		t.Fatal("应记录 last_err")
	}
}

// TestProbeBatchSocialLoginBlocksAccount 验证策略拦截落 blocked。
func TestProbeBatchSocialLoginBlocksAccount(t *testing.T) {
	upstreamStub(t, theosSocial)
	p := newService(t, newTestStore(t), func(*deepai.Account) *deepai.Client { return newStubClient(t) })
	seedAccounts(t, p.store, 1)

	p.ProbeBatch(context.Background(), p.store.Accounts(), 5*time.Second, false)
	got := p.store.Account("acc-0")
	if got.Status != deepai.StatusBlocked {
		t.Fatalf("social_login_required 应把账号标 blocked，实际 %q", got.Status)
	}
	if a, _ := p.store.PickAccount("", nil); a != nil {
		t.Fatalf("被拦截的账号不应再被调度: %s", a.ID)
	}
}

// TestProbeAsyncRespectsSemaphore 验证热路径按需复探不会堆成风暴。
func TestProbeAsyncRespectsSemaphore(t *testing.T) {
	g := newGate(probeWorkers * 4)
	upstreamStub(t, func(w http.ResponseWriter, r *http.Request) {
		g.wait(t)
		theosOK(w, r)
	})
	p := newService(t, newTestStore(t), func(*deepai.Account) *deepai.Client { return newStubClient(t) })
	const n = probeWorkers * 3
	seedAccounts(t, p.store, n)
	accts := p.store.Accounts()

	started := 0
	for _, a := range accts {
		if p.ProbeAsync(a) {
			started++
		} else {
			break // 槽位已满，后面的必然也失败
		}
	}
	g.open()
	if started > probeWorkers {
		t.Fatalf("按需复探同时发起 %d 个，超过全局上限 %d", started, probeWorkers)
	}
	if started == 0 {
		t.Fatal("至少应发起一个按需复探")
	}
}

// TestProbeAsyncDeduplicates 验证同一账号在途时不再重复发起。
func TestProbeAsyncDeduplicates(t *testing.T) {
	g := newGate(4)
	upstreamStub(t, func(w http.ResponseWriter, r *http.Request) {
		g.wait(t)
		theosOK(w, r)
	})
	p := newService(t, newTestStore(t), func(*deepai.Account) *deepai.Client { return newStubClient(t) })
	seedAccounts(t, p.store, 1)
	acct := p.store.Account("acc-0")

	if !p.ProbeAsync(acct) {
		t.Fatal("首次按需复探应发起")
	}
	if p.ProbeAsync(acct) {
		t.Fatal("同一账号在途时不应重复发起")
	}
	if p.ProbeAsync(nil) {
		t.Fatal("nil 账号不应发起")
	}
	g.open()
}

// TestReprobeStaleLimit 验证后台守护的分批上限。
func TestReprobeStaleLimit(t *testing.T) {
	meter := &concurrencyMeter{}
	upstreamStub(t, meter.track(theosOK))
	p := newService(t, newTestStore(t), func(*deepai.Account) *deepai.Client { return newStubClient(t) })
	seedAccounts(t, p.store, 6)

	if got := p.ReprobeStale(2); got != 2 {
		t.Fatalf("ReprobeStale(2) = %d，期望 2", got)
	}
	if hits := meter.hits.Load(); hits != 2 {
		t.Fatalf("上游命中 %d 次，期望 2（只探到期且受 limit 限制）", hits)
	}
	// 探测成功的账号不再到期。
	if got := p.ReprobeStale(0); got != 4 {
		t.Fatalf("第二轮应只剩 4 个到期账号，实际 %d", got)
	}
}

// TestScheduleQuotaReprobeStopsOnClose 验证入池复探可被关停打断。
func TestScheduleQuotaReprobeStopsOnClose(t *testing.T) {
	var hits atomic.Int64
	upstreamStub(t, func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		theosOK(w, r)
	})
	st := newTestStore(t)
	p := New(Config{Store: st, ClientFor: func(*deepai.Account) *deepai.Client { return newStubClient(t) }, Save: st.Save})
	seedAccounts(t, st, 1)
	// 每个号入池时都是 0 额度：确保复探真的会跑（PositiveQuota 为 false）。
	st.UpdateAccount("acc-0", func(a *deepai.Account) {
		a.Theos = &deepai.Allowance{Enabled: true, Balance: "0"}
	})
	p.SetQuotaReprobeDelays([]time.Duration{time.Hour})

	p.ScheduleQuotaReprobe("acc-0")
	p.ScheduleQuotaReprobe("   ") // 空 ID：直接返回

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	begin := time.Now()
	p.Close(ctx)
	if elapsed := time.Since(begin); elapsed > 2*time.Second {
		t.Fatalf("Close 用了 %s，退避睡眠没有被关停信号打断", elapsed)
	}
	if hits.Load() != 0 {
		t.Fatalf("关停后不应再打上游，实际 %d 次", hits.Load())
	}
}

// TestWarmUpDedupesEgressAndCountsReachable 验证预热按出口去重且 401 算通。
func TestWarmUpDedupesEgressAndCountsReachable(t *testing.T) {
	meter := &concurrencyMeter{}
	upstreamStub(t, meter.track(func(w http.ResponseWriter, r *http.Request) {
		// 上游 /models 要求 Api-Key；401 证明链路已通（预热目的达成）。
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"auth_required"}}`))
	}))
	p := newService(t, newTestStore(t), func(a *deepai.Account) *deepai.Client {
		// 按出口建客户端：预热的目标就是「每个出口都把链路跑一遍」，
		// 共用直连客户端会把「代理出口」也当成直连，测不到去重与代理分支。
		return deepai.NewClientWithProxy(a.Proxy)
	})
	ids := seedAccounts(t, p.store, 5)
	// 制造两个出口：直连（""）与一个不可达代理。
	// 可达出口只应打一次 /models；不可达出口连不上，不会命中 stub。
	p.store.UpdateAccount(ids[0], func(a *deepai.Account) { a.Proxy = "" })
	p.store.UpdateAccount(ids[1], func(a *deepai.Account) { a.Proxy = "" })
	p.store.UpdateAccount(ids[2], func(a *deepai.Account) { a.Proxy = "http://127.0.0.1:1" })

	n := p.WarmUp(context.Background())
	if n == 0 {
		t.Fatal("至少应有一个出口预热成功")
	}
	if hits := meter.hits.Load(); hits != 1 {
		t.Fatalf("直连出口只应打一次 /models，实际 %d 次（出口去重失效）", hits)
	}
}

// TestWarmUpSkipsDisabledAccounts 验证停用账号不参与预热。
func TestWarmUpSkipsDisabledAccounts(t *testing.T) {
	meter := &concurrencyMeter{}
	upstreamStub(t, meter.track(theosOK))
	st := newTestStore(t)
	p := newService(t, st, func(*deepai.Account) *deepai.Client { return newStubClient(t) })
	ids := seedAccounts(t, st, 2)
	st.UpdateAccount(ids[0], func(a *deepai.Account) { a.Enabled = false })
	st.UpdateAccount(ids[1], func(a *deepai.Account) { a.Enabled = false })

	if n := p.WarmUp(context.Background()); n != 0 {
		t.Fatalf("没有启用账号时预热数 = %d，期望 0", n)
	}
	if hits := meter.hits.Load(); hits != 0 {
		t.Fatalf("停用账号不应产生预热请求，实际 %d 次", hits)
	}
}

// TestEnrollProxyPoolNilWhenUnconfigured 验证未配置代理池时返回 nil（跳过自动分配）。
func TestEnrollProxyPoolNilWhenUnconfigured(t *testing.T) {
	t.Setenv("PROXY_POOL", "")
	t.Setenv("PROXY_POOL_FILE", "")
	p := New(Config{})
	if pool := p.EnrollProxyPool(); pool != nil {
		t.Fatalf("未配置时应返回 nil，实际 %+v", pool)
	}
	// 单例：第二次调用复用同一结果（nil）。
	if pool := p.EnrollProxyPool(); pool != nil {
		t.Fatalf("第二次调用也应返回 nil，实际 %+v", pool)
	}
}

// TestQuotaRefreshAtParsesBothFormats 覆盖 refresh_at 的两种上游格式。
func TestQuotaRefreshAtParsesBothFormats(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	a := &deepai.Account{Theos: &deepai.Allowance{RefreshAt: now.Format(time.RFC3339)}}
	if got := quotaRefreshAt(a); !got.Equal(now) {
		t.Fatalf("RFC3339 解析 = %v，期望 %v", got, now)
	}
	noZone := now.Format("2006-01-02T15:04:05")
	a.Theos.RefreshAt = noZone
	if got := quotaRefreshAt(a); got.IsZero() {
		t.Fatal("不带时区的变体也应能解析")
	}
	for _, raw := range []string{"", "not-a-time"} {
		a.Theos.RefreshAt = raw
		if got := quotaRefreshAt(a); !got.IsZero() {
			t.Fatalf("非法值 %q 应返回零值，实际 %v", raw, got)
		}
	}
	if got := quotaRefreshAt(nil); !got.IsZero() {
		t.Fatal("nil 账号应返回零值")
	}
	if got := quotaRefreshAt(&deepai.Account{}); !got.IsZero() {
		t.Fatal("无 Theos 应返回零值")
	}
}

// TestMaxCheckinBatchSanity 锁定分批常量（控制台按 remaining 续批的契约）。
func TestMaxCheckinBatchSanity(t *testing.T) {
	if MaxCheckinBatch <= 0 || MaxCheckinBatch > 1000 {
		t.Fatalf("MaxCheckinBatch = %d，超出合理区间", MaxCheckinBatch)
	}
	if QuotaGuardBatch <= 0 || QuotaGuardBatch > MaxCheckinBatch {
		t.Fatalf("QuotaGuardBatch = %d，应小于一次手动批次", QuotaGuardBatch)
	}
	_ = strconv.Itoa(QuotaGuardBatch)
}
