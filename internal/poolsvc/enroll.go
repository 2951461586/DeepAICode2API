package poolsvc

import (
	"context"
	"errors"
	"log"
	"strconv"
	"strings"
	"time"

	"deepaicode2api/internal/credimport"
	"deepaicode2api/internal/deepai"
)

// MarkVerified 把账号标记为邮箱已验证（按 ID 或邮箱定位）。
func (p *Service) MarkVerified(id, email string) error {
	// 幂等：重复置 true 无害。
	mark := func(a *deepai.Account) { a.EmailVerified = true }

	// 先按 ID 定位；补跑时台账里的 ID 可能已失效，再按邮箱回退。
	if id = strings.TrimSpace(id); id != "" && p.store.UpdateAccount(id, mark) {
		return p.store.Save()
	}
	if want := strings.TrimSpace(email); want != "" {
		for _, a := range p.store.Accounts() {
			if strings.EqualFold(a.Email, want) && p.store.UpdateAccount(a.ID, mark) {
				return p.store.Save()
			}
		}
	}
	return errors.New("账号不存在")
}

// EnrollRegistration 是入池的唯一写入口（管理接口与浏览器登录共用）。
func (p *Service) EnrollRegistration(reg deepai.Registration, al *deepai.Allowance) (string, bool, error) {
	acct, err := credimport.AccountFromRegistration(reg)
	if err != nil {
		return "", false, err
	}
	now := time.Now().Unix()
	acct.Status = "ok"
	acct.LastUsed = now
	acct.LastVerified = now
	acct.Failures = 0
	acct.LastErr = ""
	if al != nil {
		acct.Theos = al
	}

	isNew := p.store.UpsertAccount(acct)
	// Upsert 会保留旧运行态；把本次验证结果显式写回。
	p.store.UpdateAccount(acct.ID, func(stored *deepai.Account) {
		stored.Status = "ok"
		stored.Failures = 0
		stored.LastErr = ""
		stored.LastUsed = now
		stored.LastVerified = now
		stored.EmailVerified = reg.EmailVerified
		stored.Plan = reg.Plan
		if al != nil {
			stored.Theos = al.Clone()
		}
	})
	if err := p.store.Save(); err != nil {
		return "", isNew, err
	}

	// 刚注册完的账号，上游往往还没把当日额度发下来：此时 /theos 会返回
	// balance=0，但几分钟后同一个 Key 再查就是 200。若不复探，这个 0 会被
	// 当成事实缓存下来，调度侧就会长期跳过该账号。
	if al == nil || !PositiveQuota(al) {
		p.ScheduleQuotaReprobe(acct.ID)
	}
	return acct.ID, isNew, nil
}

// PositiveQuota 报告配额是否明确大于 0。
//
// 解析不了 / 字段缺失都按“未知”（返回 false）处理：宁可多探一次，
// 也不要把可用账号判死。
func PositiveQuota(al *deepai.Allowance) bool {
	if al == nil || !al.Enabled {
		return false
	}
	v, err := strconv.ParseFloat(strings.TrimSpace(al.Balance), 64)
	return err == nil && v > 0
}

// AllowanceFromEnroll 构造入池时携带的配额快照。
//
// 上游在账号刚创建时常常返回 balance=0（当日额度随后才下发）。这个 0 不是
// 「确认耗尽」而是「未知」：按确认值缓存会让调度第一轮把它排除，最长 15 分钟
// （R2 实测：新批入池后立刻压测，50 个号只有 12~15 个可被选中，
// 数据面仅覆盖 19/50）。所以 0 额度时把 CheckedAt 留 0，让 pool.HasQuota
// 按「未知」放行；真没额度时上游会拒，失败的账号会被自动降权。
func AllowanceFromEnroll(balance, granted, refreshAt string) *deepai.Allowance {
	al := &deepai.Allowance{Enabled: true, Balance: balance, Granted: granted, RefreshAt: refreshAt}
	if !PositiveQuota(al) {
		return al
	}
	al.CheckedAt = time.Now().Unix()
	return al
}

// quotaReprobeDelays 是入池后 0 额度的复探退避序列。
//
// 实测（R2 压测）：入池瞬间 50 个新号里只有 12~15 个拿到额度；
// 原 3/6/12 分钟序列要到第 9 分钟才集齐 47/50。改成 45s/2min/6min 后，
// 「可调度」的空窗从 9 分钟压到 1 分钟以内；拿到正额度即停。
var quotaReprobeDelays = []time.Duration{45 * time.Second, 2 * time.Minute, 6 * time.Minute}

// ScheduleQuotaReprobe 异步复探某个账号的配额（仅当它仍是 0 时）。
//
// 每个账号一个 goroutine，次数有上限，不会自我放大。
func (p *Service) ScheduleQuotaReprobe(id string) {
	if strings.TrimSpace(id) == "" {
		return
	}
	delays := p.quotaReprobeDelays
	if len(delays) == 0 {
		return
	}
	p.quotaWG.Add(1)
	go func() {
		defer p.quotaWG.Done()
		for _, d := range delays {
			// 可被关停打断：退避最长 12 分钟，不能把进程退出拖住。
			if !p.sleepOrStop(d) {
				return
			}
			a := p.store.Account(id)
			if a == nil {
				return // 已被删除
			}
			if PositiveQuota(a.Theos) {
				return // 期间被别的路径探到了
			}
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			al, err := p.clientFor(a).Theos(ctx, a)
			cancel()
			p.SetAllowance(id, al, err)
			if err := p.store.Save(); err != nil {
				log.Printf("[quota] 保存复探结果失败: %v", err)
			}
			if err == nil && PositiveQuota(al) {
				log.Printf("[quota] %s 复探拿到额度: balance=%s granted=%s", a.Name, al.Balance, al.Granted)
				return
			}
		}
	}()
}

// SetQuotaReprobeDelays 覆盖入池后的复探退避序列（nil / 空 = 关闭复探）。
//
// 生产用不到：留着是为了测试能注入毫秒级退避，以及排查时可以临时关掉
// 「入池后自动复探」这条后台链路。
func (p *Service) SetQuotaReprobeDelays(d []time.Duration) {
	p.quotaReprobeDelays = d
}
