package poolsvc

import (
	"context"
	"errors"
	"time"

	"deepaicode2api/internal/model"
)

// --- 账号状态转移 ---
//
// 这些函数原先住在 internal/store：持久化层同时承担了「一次调用结果该怎么
// 记到账号上」的业务策略（瞬时错误不标 error、策略拦截要 blocked、探测失败
// 要退避）。结果是读一个号池问题要在 store / pool / poolsvc 三个包之间跳。
//
// 现在: store 只提供带锁的原语（UpdateAccount / UpdateAccounts / Save），
// 状态转移的**判定**全在这里。落盘时机仍由调用方决定（这些函数不 Save），
// 与迁移前一致。

// MarkResult 记录一次账号调用结果。
//
// 失败时是否把账号标成 error 由 immediate 参数决定：调用方已经确定要记成
// 硬失败时传 true；否则用 MarkFailure（它会做瞬时判定）。
func (p *Service) MarkResult(id string, ok bool, note string) {
	p.markResult(id, ok, note, false)
}

// MarkFailure 记录一次带错误对象的失败。
//
// 瞬时失败（EOF / 429 / 5xx / 容量拒绝 / 签到限流）只累加 Failures 与
// last_err，**不动 status**：一次抖动不该让账号在控制台上长期显示不健康
// （docs/changes.md §45）。
func (p *Service) MarkFailure(id string, err error) {
	if err == nil {
		return
	}
	// 上下文被取消 = 我们自己放弃了这个请求（客户端断开 / 上层收尾），
	// 不是账号的问题。
	if errors.Is(err, context.Canceled) {
		return
	}
	p.markResult(id, false, err.Error(), model.Transient(err))
}

// markResult 是 MarkResult / MarkFailure 的共同实现。
func (p *Service) markResult(id string, ok bool, note string, transient bool) {
	p.store.UpdateAccount(id, func(a *model.Account) {
		now := time.Now().Unix()
		a.LastUsed = now
		if ok {
			a.Failures = 0
			a.Status = "ok"
			a.LastErr = ""
			a.LastErrAt = 0
			// 推理成功说明额度已恢复（或这条 403 是历史噪声）。
			a.QuotaUntil = 0
			// 注意：不在这里清 ProbeAfter —— 推理成功不等于 /theos 会成功。
			// 签到限流的账号正是「能推理、不能签到」，清掉冷却会让后台守护
			// 每分钟又去磕一次 403。探测成功后由 SetAllowance 清零。
			if note != "" {
				a.Note = note
			}
			return
		}
		a.Failures++
		a.LastErr = note
		a.LastErrAt = now
		if !transient {
			a.Status = model.StatusError
		}
	})
}

// RecoverStaleErrors 把「已经很久没再失败」的 error 号改回 ok，返回改动个数。
//
// status 是展示与排序的依据，光靠 AccountReady 的懒判定会让账号列表里的
// 红点与顶部徽章对不上。后台 30s 跳一次（O(n)，与账号数同阶）。
//
// blocked 不动：它不会自愈，必须等一次成功 /theos 或人工处理。
func (p *Service) RecoverStaleErrors(now time.Time) int {
	return p.store.UpdateAccounts(func(a *model.Account) bool {
		if a.Status != model.StatusError {
			return false
		}
		if now.Sub(time.Unix(a.LastErrAt, 0)) < model.ErrorStaleAfter {
			return false
		}
		// 保留 last_err / failures 作为历史，只把「现在是否健康」改回去。
		a.Status = "ok"
		return true
	})
}

// Block 把账号标记为「上游策略拦截」（当前来源：social_login_required）。
//
// 与普通失败的区别：这不是网络/额度问题，换号重试救不回来。调度器会跳过
// blocked，直到一次成功的 /theos 把它重新置回 ok（见 SetAllowance）。
func (p *Service) Block(id, reason string) {
	p.store.UpdateAccount(id, func(a *model.Account) {
		a.LastUsed = time.Now().Unix()
		a.Status = model.StatusBlocked
		a.LastErr = reason
		if a.Failures < 1 {
			a.Failures = 1
		}
	})
}

// MarkQuotaExhausted 标记账号「当日额度用完」，until 之前调度跳过它。
//
// 与 Block 的区别：这是自愈状态（到点自动恢复），不需要人工处理，
// 也不该让控制台把它显示成「被策略拦截」。
func (p *Service) MarkQuotaExhausted(id string, until time.Time) {
	p.store.UpdateAccount(id, func(a *model.Account) {
		a.QuotaUntil = until.Unix()
		a.LastErr = "当日额度用完（" + until.Format(time.RFC3339) + " 恢复）"
		a.LastErrAt = time.Now().Unix()
	})
}

// ClearQuotaExhausted 清除额度用完标记（探测到余额恢复时调用）。
func (p *Service) ClearQuotaExhausted(id string) {
	p.store.UpdateAccount(id, func(a *model.Account) { a.QuotaUntil = 0 })
}

// SetAllowance 缓存一次配额探测结果。
func (p *Service) SetAllowance(id string, al *model.Allowance, err error) {
	p.store.UpdateAccount(id, func(a *model.Account) {
		a.LastUsed = time.Now().Unix()
		if err != nil {
			// 瞬时错误（EOF / 429 / 签到限流）不该把账号打成 error：
			// 探测失败经常是成片的出口抖动，不是账号本身坏了。
			if !model.Transient(err) {
				a.Status = model.StatusError
			}
			a.LastErr = err.Error()
			a.LastErrAt = time.Now().Unix()
			a.Failures++
			bumpProbeBackoff(a)
			return
		}
		a.Theos = al
		a.Status = "ok"
		a.LastErr = ""
		a.LastErrAt = 0
		a.Failures = 0
		a.ProbeFailures = 0
		a.ProbeAfter = 0
	})
}

// RecordProbeFailure 记一次「自动复探」失败：只推进探测退避，不改调度状态。
//
// 与 SetAllowance 的错误分支的区别：这里来自推理路径上的按需复探，一次出口
// 抖动不该让调度器判号坏了，也不该覆盖控制台上更具体的原因。
//
// 但退避**必须**留下：不记 ProbeAfter 时 NeedsRefresh 永远为 true，
// 每个选中该账号的请求都会再探一次（线上实测 323 次/小时，docs/changes.md §61）。
func (p *Service) RecordProbeFailure(id string) {
	p.store.UpdateAccount(id, func(a *model.Account) { bumpProbeBackoff(a) })
}

// LockProbe 给账号的额度探测加静默锁（目前来源：check_in_refused）。
//
// 与普通失败的区别：上游没坏、账号也能推理，只是当天的「重启签到」次数用尽；
// 锁住探测可以避免后台守护与推理前签到反复撞 403。
func (p *Service) LockProbe(id string, until time.Time, note string) {
	p.store.UpdateAccount(id, func(a *model.Account) {
		a.LastUsed = time.Now().Unix()
		if until.Unix() > a.ProbeAfter {
			a.ProbeAfter = until.Unix()
		}
		if note != "" {
			a.LastErr = note
		}
	})
}

// bumpProbeBackoff 推进账号的探测退避（调用方已持锁）。
func bumpProbeBackoff(a *model.Account) {
	a.ProbeFailures++
	a.ProbeAfter = time.Now().Add(probeBackoff(a.ProbeFailures)).Unix()
}

// probeBackoff 给出连续探测失败后的冷却时长：2min → 10min → 1h（封顶）。
//
// 不缓存失败结果时，后台守护每 5 分钟会把同一批失败号全部重探一遍；
// 而失败往往是成片的（出口故障 / 上游抖动），653 号规模下就是持续的
// /theos 风暴。与 docs/recon.md 的「签到是有限资源」是同一条约束。
func probeBackoff(consecutive int) time.Duration {
	switch {
	case consecutive <= 1:
		return 2 * time.Minute
	case consecutive == 2:
		return 10 * time.Minute
	default:
		return time.Hour
	}
}
