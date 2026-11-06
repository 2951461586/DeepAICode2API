package register

import (
	"context"
	"crypto/rand"
	"errors"
	"log"
	"math/big"
	"strings"
	"time"

	"deepaicode2api/internal/deepai"
	"deepaicode2api/internal/parallel"
)

// RunBatch 并发跑 N 个账号。
func (p *Pipeline) RunBatch(ctx context.Context, count, concurrency int, email, password string) []*Record {
	if count <= 0 {
		count = 1
	}
	results := make([]*Record, count)
	parallel.For(ctx, concurrency, count, func(ctx context.Context, i int) {
		p.waitTurn(ctx)
		e, pw := "", ""
		if count == 1 {
			e, pw = email, password
		}
		results[i] = p.RunOne(ctx, e, pw)
	})
	return results
}

// RunRecords 对已有台账记录补跑（幂等；已入池的直接跳过）。
func (p *Pipeline) RunRecords(ctx context.Context, records []*Record, concurrency int) []*Record {
	results := make([]*Record, len(records))
	parallel.For(ctx, concurrency, len(records), func(ctx context.Context, i int) {
		old := records[i]
		if old == nil {
			return
		}
		if old.Status == StatusEnrolled && old.APIKey != "" {
			results[i] = old
			return
		}
		p.waitTurn(ctx)
		var rec *Record
		if old.APIKey != "" && strings.TrimSpace(old.Password) == "" {
			rec = p.resumeWithKey(ctx, old)
		} else {
			rec = p.runOne(ctx, old.Email, old.Password, old.Proxy, old.Fingerprint, old.MachineID)
			rec = Merge(old, rec)
			_ = p.opts.Ledger.Append(rec)
			p.notify(rec)
		}
		results[i] = rec
	})
	return results
}

// resumeWithKey 在只有 API Key 的情况下补验活 / 补入池。
func (p *Pipeline) resumeWithKey(ctx context.Context, old *Record) *Record {
	rec := *old
	rec.Group = firstNonEmpty(rec.Group, p.effectiveGroup())
	if strings.TrimSpace(rec.Proxy) == "" && p.opts.ProxyPool != nil {
		rec.Proxy = p.opts.ProxyPool.Next()
		rec.ProxyLabel = ProxyLabel(rec.Proxy)
	}
	t := time.Now()
	fp := FingerprintByID(rec.Fingerprint)
	up := deepai.NewClientWithFingerprint(rec.Proxy, fp.UserAgent)
	// 历史台账里的记录可能没有机器指纹（改动之前生成的），补一个。
	if strings.TrimSpace(rec.MachineID) == "" {
		rec.MachineID = deepai.NewMachineUUID()
	}
	if err := p.validate(ctx, &rec, up); err != nil {
		rec.Mark("validate", "failed", time.Since(t))
		rec.Error = err.Error()
		rec.Status = StatusFailed
		_ = p.opts.Ledger.Append(&rec)
		p.notify(&rec)
		return &rec
	}
	rec.Status = StatusKeyed
	rec.Mark("validate", "ok", time.Since(t))
	if p.opts.Enroller != nil {
		t = time.Now()
		id, err := p.opts.Enroller.Enroll(ctx, &rec)
		if err != nil {
			rec.Mark("enroll", "failed", time.Since(t))
			rec.Error = "入池失败: " + err.Error()
		} else {
			rec.AccountID = id
			rec.Status = StatusEnrolled
			rec.Mark("enroll", "ok", time.Since(t))
		}
	}
	if rec.FinishedAt == 0 {
		rec.FinishedAt = float64(time.Now().UnixNano()) / 1e9
	}
	merged := Merge(old, &rec)
	_ = p.opts.Ledger.Append(merged)
	p.notify(merged)
	return merged
}

// finish 归一化、写台账、回调并返回记录。
func (p *Pipeline) finish(rec *Record) *Record {
	rec.Normalize()
	if rec.FinishedAt == 0 {
		rec.FinishedAt = float64(time.Now().UnixNano()) / 1e9
	}
	if p.opts.Ledger != nil {
		if err := p.opts.Ledger.Append(rec); err != nil {
			p.logf("[%s] 写台账失败：%v", rec.Email, err)
		}
	}
	p.notify(rec)
	p.logf("[%s] 结果：%s%s", rec.Email, rec.Status, errSuffix(rec))
	return rec
}

func (p *Pipeline) notify(rec *Record) {
	if p.opts.OnUpdate != nil && rec != nil {
		p.opts.OnUpdate(rec)
	}
}

// mailboxFor 为指定邮箱准备收件箱；未指定邮箱时创建一个新的。
func (p *Pipeline) mailboxFor(ctx context.Context, email string) (Mailbox, error) {
	email = strings.TrimSpace(email)
	if email != "" {
		if ip, ok := p.opts.Provider.(InboxProvider); ok {
			if mb, ok := ip.Inbox(email); ok {
				return mb, nil
			}
		}
		name := "none"
		if p.opts.Provider != nil {
			name = p.opts.Provider.Name()
		}
		return &noInboxMailbox{email: email, provider: name}, nil
	}
	if p.opts.Provider == nil {
		return nil, errors.New("未配置邮箱后端且未指定邮箱")
	}
	return p.opts.Provider.Create(ctx)
}

func (p *Pipeline) effectiveGroup() string {
	g := strings.TrimSpace(p.opts.Group)
	if g == "" {
		g = strings.TrimSpace(p.opts.Config.DefaultGroup)
	}
	if g == "" {
		g = "default"
	}
	return g
}

// waitTurn 控制注册请求的最小间隔（默认 800ms）。
func (p *Pipeline) waitTurn(ctx context.Context) {
	if p.opts.MinInterval <= 0 {
		return
	}
	p.mu.Lock()
	wait := time.Until(p.lastStart.Add(p.opts.MinInterval))
	p.mu.Unlock()
	if wait > 0 {
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
		case <-timer.C:
		}
	}
	p.mu.Lock()
	p.lastStart = time.Now()
	p.mu.Unlock()
}

func (p *Pipeline) logf(format string, args ...any) {
	if p.opts.Log != nil {
		p.opts.Log(format, args...)
		return
	}
	log.Printf(format, args...)
}

func boolText(v bool) string {
	if v {
		return "exists"
	}
	return "new"
}

func errSuffix(rec *Record) string {
	if strings.TrimSpace(rec.Error) == "" {
		return ""
	}
	return "（" + rec.Error + "）"
}

// RandomPassword 生成满足常见强度要求的随机密码。
func RandomPassword(n int) string {
	const upper = "ABCDEFGHJKLMNPQRSTUVWXYZ"
	const lower = "abcdefghijkmnopqrstuvwxyz"
	const digits = "23456789"
	const symbols = "!@#$%^&*"
	all := upper + lower + digits + symbols
	if n < 8 {
		n = 8
	}
	buf := make([]byte, 0, n)
	pick := func(set string) byte {
		i, err := rand.Int(rand.Reader, big.NewInt(int64(len(set))))
		if err != nil {
			return set[0]
		}
		return set[i.Int64()]
	}
	buf = append(buf, pick(upper), pick(lower), pick(digits), pick(symbols))
	for len(buf) < n {
		buf = append(buf, pick(all))
	}
	// 洗牌
	for i := len(buf) - 1; i > 0; i-- {
		j, err := rand.Int(rand.Reader, big.NewInt(int64(i+1)))
		if err != nil {
			break
		}
		buf[i], buf[j.Int64()] = buf[j.Int64()], buf[i]
	}
	return string(buf)
}

// isAlreadyRegistered 判断注册错误是否表示邮箱已存在。
func isAlreadyRegistered(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "already has an account") ||
		strings.Contains(msg, "already exists") ||
		strings.Contains(msg, "already registered")
}
