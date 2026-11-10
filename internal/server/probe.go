package server

import (
	"context"
	"log"
	"sort"
	"strings"
	"time"

	"deepaicode2api/internal/deepai"
	"deepaicode2api/internal/parallel"
	"deepaicode2api/internal/pool"
	"deepaicode2api/internal/retry"
)

// --- 账号配额探测 ---

const (
	// probeWorkers 是探测批量账号的并发上限。
	//
	// 串行探测 20 个账号最坏要 10 分钟（每个 30s 超时），全并发又容易
	// 把探针本身变成上游风控特征，4 是实测下来兼顾速度与克制的值。
	probeWorkers = 4

	// quotaStaleAfter 是「按需复探」的触发阈值：使用中的账号额度信息
	// 超过这个时间就异步刷新一次。与调度侧的 pool.QuotaStaleAfter 对齐 ——
	// 调度在同一阈值之后不再信任 balance=0，两边判断必须一致。
	quotaStaleAfter = pool.QuotaStaleAfter

	// QuotaGuardInterval / QuotaGuardBatch 是后台额度守护的节奏。
	//
	// 只挑「refresh_at 已过 / 从未探测 / 超过 6 小时未探」的账号，稳态下
	// 几乎不产生请求；每天额度刷新那一刻会有整池集中到期，按 24 个 / 5 分钟
	// 排队消化（653 号约 2.3 小时铺完）。不搞全量并发扫描 —— 那是明显的
	// 风控特征，也会把探针变成流量。
	QuotaGuardInterval = 5 * time.Minute
	QuotaGuardBatch    = 24

	// quotaGuardStaleAfter 是后台守护的「过旧」阈值。
	// 比按需复探宽得多：活跃账号由推理路径顺带刷新，后台只负责兜底。
	quotaGuardStaleAfter = 6 * time.Hour

	// maxCheckinBatch 是单次 /admin/api/checkin 最多探测的账号数。
	//
	// 653 号全量串起来要 11 分钟，超过浏览器 / 反代的耐心；改为分批 +
	// remaining，控制台按 remaining 继续下一批（每批探测过的 checked_at
	// 会推到现在，下一批自然取到没探过的）。
	maxCheckinBatch = 200
)

// probeRetryDelays 是入池前探测配额的退避序列。
//
// 入池是唯一会阻塞调用方的探测点，而 /theos 的失败几乎都是出口抖动
// （No exit node / TLS handshake / unexpected EOF），重试一次能救回绝大多数。
var probeRetryDelays = []time.Duration{time.Second}

// quotaRefreshAt 解析上游返回的 refresh_at；无法解析时返回零值。
func quotaRefreshAt(a *deepai.Account) time.Time {
	if a == nil || a.Theos == nil {
		return time.Time{}
	}
	raw := strings.TrimSpace(a.Theos.RefreshAt)
	if raw == "" {
		return time.Time{}
	}
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		return t
	}
	// 容忍不带时区的变体，按 UTC 处理（上游目前都是 RFC3339）。
	if t, err := time.Parse("2006-01-02T15:04:05", raw); err == nil {
		return t
	}
	return time.Time{}
}

// quotaNeedsRefresh 报告账号的额度信息是否已经不可信。
//
// staleAfter<=0 表示严格模式：只认「从未探测」与「refresh_at 已过」，
// 用于手动 checkin 的 stale_only 过滤（不想把刚探过的号再打一遍）。
func quotaNeedsRefresh(a *deepai.Account, now time.Time, staleAfter time.Duration) bool {
	if a == nil || !a.Enabled {
		return false
	}
	// 探测失败后的冷却：失败往往是成片的（出口故障 / 上游抖动），
	// 不缓存失败结果就会让后台守护每 5 分钟把同一批失败号全部重探一遍。
	if a.ProbeAfter > now.Unix() {
		return false
	}
	if a.Theos == nil || a.Theos.CheckedAt <= 0 {
		return true
	}
	if rt := quotaRefreshAt(a); !rt.IsZero() && !now.Before(rt) {
		return true
	}
	if staleAfter > 0 && now.Sub(time.Unix(a.Theos.CheckedAt, 0)) >= staleAfter {
		return true
	}
	return false
}

// quotaDueRank 给出探测优先级：0 = refresh_at 已过（额度刚刷新，最该探），
// 1 = 从未探测，2 = 单纯过旧。
func quotaDueRank(a *deepai.Account, now time.Time) int {
	if rt := quotaRefreshAt(a); !rt.IsZero() && !now.Before(rt) {
		return 0
	}
	if a.Theos == nil || a.Theos.CheckedAt <= 0 {
		return 1
	}
	return 2
}

// sortByQuotaDue 把最值得探测的账号排在前面（同级按 checked_at 升序）。
//
// 分批探测依赖这个顺序：只探前 N 个时，先探「额度刚刷新」和「从没探过」的，
// 才不会把配额花在已经最新的号上。
func sortByQuotaDue(accts []*deepai.Account, now time.Time) {
	sort.SliceStable(accts, func(i, j int) bool {
		ri, rj := quotaDueRank(accts[i], now), quotaDueRank(accts[j], now)
		if ri != rj {
			return ri < rj
		}
		return checkedAt(accts[i]) < checkedAt(accts[j])
	})
}

func checkedAt(a *deepai.Account) int64 {
	if a == nil || a.Theos == nil {
		return 0
	}
	return a.Theos.CheckedAt
}

// claimProbe 占用某账号的探测槽位；已有探测在跑时返回 false。
//
// 三条路径会同时探同一批账号：手动 checkin、入池后延迟复探、推理时的
// 按需复探。没有去重时它们会互相叠加，把同一个号连打三次。
func (s *Server) claimProbe(id string) bool {
	if strings.TrimSpace(id) == "" {
		return false
	}
	s.probeMu.Lock()
	defer s.probeMu.Unlock()
	if s.probing == nil {
		s.probing = map[string]struct{}{}
	}
	if _, busy := s.probing[id]; busy {
		return false
	}
	s.probing[id] = struct{}{}
	return true
}

func (s *Server) releaseProbe(id string) {
	s.probeMu.Lock()
	delete(s.probing, id)
	s.probeMu.Unlock()
}

// probeOutcome 是单个账号的探测结果。
type probeOutcome struct {
	Account   *deepai.Account
	Allowance *deepai.Allowance
	Err       error
	// Skipped 表示该账号已有探测在跑，本次没有真正请求上游。
	Skipped bool
}

// probeBatch 以有界并发探测一批账号，结果按下标与入参一一对应。
//
// strict=true 时失败会记入账号状态（手动探测：用户想知道哪个号不可用）；
// strict=false 时只采信成功结果（自动复探：上游抖一下不应该把正在服务
// 推理的账号标成 error，更不能累加 Failures 把调度带偏）。
func (s *Server) probeBatch(ctx context.Context, accts []*deepai.Account, timeout time.Duration, strict bool) []probeOutcome {
	out := make([]probeOutcome, len(accts))
	indexes := make([]int, 0, len(accts))
	for i, a := range accts {
		if a == nil {
			continue
		}
		out[i].Account = a
		if !s.claimProbe(a.ID) {
			out[i].Skipped = true
			continue
		}
		indexes = append(indexes, i)
	}
	if len(indexes) == 0 {
		return out
	}
	defer func() {
		for _, i := range indexes {
			s.releaseProbe(accts[i].ID)
		}
	}()

	parallel.For(ctx, probeWorkers, len(indexes), func(ctx context.Context, k int) {
		i := indexes[k]
		a := accts[i]
		cctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		// 全局在途上限：手动批次、后台守护、按需复探共享 4 个槽位。
		select {
		case s.probeSem <- struct{}{}:
			defer func() { <-s.probeSem }()
		case <-cctx.Done():
			return
		}
		al, err := s.clientFor(a).Theos(cctx, a)
		out[i].Allowance, out[i].Err = al, err
		if err != nil && !strict {
			return
		}
		// 策略拦截不是“额度探测失败”：单独落 blocked，调度器会跳过它。
		if deepai.IsSocialLoginRequired(err) {
			s.Store.BlockAccount(a.ID, err.Error())
			return
		}
		// 签到被限流也不是账号坏：推理照常，只是当天别想再签到。
		// 锁住额度探测，避免后台守护每 5 分钟再来磕一次 403。
		if deepai.IsCheckInRefused(err) {
			s.Store.LockAccountProbe(a.ID, time.Now().Add(deepai.CheckInRefusedLock), err.Error())
			return
		}
		s.Store.SetAccountAllowance(a.ID, al, err)
	})

	// 全部被去重跳过时无事可存。
	if len(indexes) > 0 {
		_ = s.Store.Save()
	}
	return out
}

// probeAccounts 探测一批账号并落盘（不关心逐项结果）。
func (s *Server) probeAccounts(accts []*deepai.Account) {
	s.probeBatch(context.Background(), accts, 20*time.Second, true)
}

// probeAndSave 异步探测单个账号并落盘。
func (s *Server) probeAndSave(a *deepai.Account) {
	s.probeAccounts([]*deepai.Account{a})
}

// probeAndSaveAsync 在后台探测一个账号并落盘，把这次异步工作登记到 quotaWG。
//
// 登记（Add）必须在起 goroutine 之前：探测会先写内存再 Save()，测试若只等
// 接口返回就中途退出，后台的落盘会与 t.TempDir() 清理撞上，在 CI 上表现为
// “TempDir RemoveAll cleanup: directory not empty”。
func (s *Server) probeAndSaveAsync(a *deepai.Account) {
	if a == nil {
		return
	}
	s.quotaWG.Add(1)
	go func() {
		defer s.quotaWG.Done()
		s.probeAndSave(a)
	}()
}

// probeAsync 异步刷新单个账号的额度；返回是否真正发起。
//
// 先同步占用槽位再起 goroutine：否则每个请求都会起一个立刻退出的 goroutine，
// 高并发下就是无意义的调度风暴。
func (s *Server) probeAsync(a *deepai.Account) bool {
	if a == nil || !s.claimProbe(a.ID) {
		return false
	}
	// 拿不到全局槽位就放弃：这批过期额度留给下一轮（后台守护或下一次请求），
	// 不在热点路径上排队等，也不把探针堆成风控特征。
	select {
	case s.probeSem <- struct{}{}:
	default:
		s.releaseProbe(a.ID)
		return false
	}
	go func() {
		defer func() { <-s.probeSem }()
		defer s.releaseProbe(a.ID)
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		al, err := s.clientFor(a).Theos(ctx, a)
		if err != nil {
			// 自动复探失败不改账号状态：推理路径不依赖这次结果，
			// 把 transient 错误写进 Status/Failures 只会让调度误判。
			// 但 social_login_required 是策略终态，必须落盘让调度器停下。
			if deepai.IsSocialLoginRequired(err) {
				s.Store.BlockAccount(a.ID, err.Error())
				if saveErr := s.Store.Save(); saveErr != nil {
					log.Printf("[quota] 保存策略拦截状态失败: %v", saveErr)
				}
				log.Printf("[quota] %s 已被上游要求改用 Google/Apple 登录（social_login_required）", a.Name)
				return
			}
			log.Printf("[quota] %s 额度复探失败（不影响调度）: %v", a.Name, err)
			return
		}
		s.Store.SetAccountAllowance(a.ID, al, nil)
		if err := s.Store.Save(); err != nil {
			log.Printf("[quota] 保存复探结果失败: %v", err)
		}
	}()
	return true
}

// ReprobeStale 探测一批额度信息过旧的账号，返回实际发起的探测数。
//
// limit<=0 表示不限（测试用）；生产由 QuotaGuardBatch 限流。
func (s *Server) ReprobeStale(limit int) int {
	now := time.Now()
	accts := s.Store.Accounts()
	due := make([]*deepai.Account, 0, len(accts))
	for _, a := range accts {
		if quotaNeedsRefresh(a, now, quotaGuardStaleAfter) {
			due = append(due, a)
		}
	}
	if len(due) == 0 {
		return 0
	}
	sortByQuotaDue(due, now)
	if limit > 0 && len(due) > limit {
		due = due[:limit]
	}
	out := s.probeBatch(context.Background(), due, 20*time.Second, false)
	probed := 0
	for _, o := range out {
		if o.Account != nil && !o.Skipped {
			probed++
		}
	}
	return probed
}

// probeAccountQuota 探测一个账号的配额；瞬时错误重试一次（入池专用，阻塞）。
func (s *Server) probeAccountQuota(acct *deepai.Account) (*deepai.Allowance, error) {
	var al *deepai.Allowance
	err := retry.Do(context.Background(), probeRetryDelays, deepai.IsTransient, func(parent context.Context) error {
		ctx, cancel := context.WithTimeout(parent, 20*time.Second)
		defer cancel()
		got, err := s.clientFor(acct).Theos(ctx, acct)
		if err != nil {
			return err
		}
		al = got
		return nil
	})
	if err != nil {
		return nil, err
	}
	return al, nil
}
