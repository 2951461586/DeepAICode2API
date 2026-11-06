package register

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"deepaicode2api/internal/deepai"
	"deepaicode2api/internal/retry"
)

// applyVerify 处理一次验证结果（点链接 + 打点），返回是否验证成功。
func (p *Pipeline) applyVerify(ctx context.Context, rec *Record, web *WebClient, out verifyOutcome) bool {
	vt := time.Now()
	switch {
	case errors.Is(out.err, ErrNoInbox):
		rec.Mark("verify", "skipped (no inbox)", 0)
		p.logf("[%s] 邮箱后端无收件箱，跳过邮件验证", rec.Email)
	case out.err != nil:
		rec.Mark("verify", "warning", out.waited)
		p.logf("[%s] 等待验证邮件失败：%v", rec.Email, out.err)
	default:
		if err := verifyEmailThrottled(ctx, web, out.link); err != nil {
			rec.Mark("verify", "warning", out.waited+time.Since(vt))
			p.logf("[%s] 验证链接访问失败：%v", rec.Email, err)
			return false
		}
		rec.EmailVerified = true
		if StatusRank(rec.Status) < StatusRank(StatusVerified) {
			rec.Status = StatusVerified
		}
		rec.Mark("verify", "ok", out.waited+time.Since(vt))
		p.logf("[%s] 邮箱验证完成", rec.Email)
		return true
	}
	return false
}

// verifyBackfill 在入池之后继续等验证邮件，成功后回填 email_verified。
//
// 用脱离调用方 ctx 的独立超时：批次的 ctx 在 RunBatch 返回时就会被取消，
// 而验证往往在这之后才完成。所有写入都作用在记录副本上，避免与
// finish()/notify() 的读取竞争。
func (p *Pipeline) verifyBackfill(rec *Record, web *WebClient, ch chan verifyOutcome) {
	cp := cloneRecord(rec)
	if cp == nil {
		return
	}
	defer func() {
		p.verifyPending.Add(-1)
		p.verifyWG.Done()
		if r := recover(); r != nil {
			p.logf("[%s] 后台验证异常：%v", cp.Email, r)
		}
	}()

	wait := p.opts.VerifyTimeout
	if wait <= 0 {
		wait = DefaultVerifyWait
	}
	ctx, cancel := context.WithTimeout(context.Background(), wait+2*time.Minute)
	defer cancel()

	var out verifyOutcome
	select {
	case out = <-ch:
	case <-ctx.Done():
		return
	}
	if !p.applyVerify(ctx, cp, web, out) {
		return
	}

	if vm, ok := p.opts.Enroller.(VerifiedMarker); ok {
		if err := vm.MarkVerified(ctx, cp.AccountID, cp.Email); err != nil {
			p.logf("[%s] 回填验证状态失败：%v", cp.Email, err)
		}
	}

	// 再写一条台账：Merge 保留更高等级，并把 verify 从 pending 更新为 ok。
	if p.opts.Ledger != nil {
		_ = p.opts.Ledger.Append(cp)
	}
	p.notify(cp)
}

// theosRetryDelays 是 /theos 验活遇到瞬时错误时的退避序列。
//
// 实测批次里 4/50 的失败全是这一类：No exit node / TLS handshake timeout /
// unexpected EOF —— 全是出口抖动，重试一两次就能救回。
var theosRetryDelays = []time.Duration{time.Second, 3 * time.Second}

// theosWithRetry 查询配额，瞬时错误退避重试；鉴权/参数错误直接返回。
func theosWithRetry(ctx context.Context, up *deepai.Client, acct *deepai.Account) (*deepai.Allowance, error) {
	var al *deepai.Allowance
	err := retry.Do(ctx, theosRetryDelays, deepai.IsTransient, func(c context.Context) error {
		got, err := up.Theos(c, acct)
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

// isUnverifiedError 报告错误是否因为“邮箱还没验证”。
func isUnverifiedError(err error) bool {
	var he *deepai.HTTPError
	if !errors.As(err, &he) {
		return false
	}
	return he.Code() == "email_unverified"
}

// verifyOutcome 是并行验证等待的结果。
type verifyOutcome struct {
	link   string
	err    error
	waited time.Duration
}

// validate 用 /theos 验证 API Key（可选再做一次最小对话探针）。
func (p *Pipeline) validate(ctx context.Context, rec *Record, up *deepai.Client) error {
	if strings.TrimSpace(rec.APIKey) == "" {
		return errors.New("缺少 API Key")
	}
	if up == nil {
		up = deepai.NewClientWithFingerprint(rec.Proxy, "")
	}
	vctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	acct := &deepai.Account{
		APIKey: rec.APIKey, Proxy: rec.Proxy, MachineID: rec.MachineID,
		UserAgent: rec.UserAgent, AcceptLanguage: rec.AcceptLanguage,
		SecCHUA: rec.SecCHUA, Platform: rec.Platform,
	}
	al, err := theosWithRetry(vctx, up, acct)
	if err != nil {
		return fmt.Errorf("Theos 验活失败: %w", err)
	}
	rec.TheosBalance = al.Balance
	rec.TheosGranted = al.Granted
	rec.TheosRefreshAt = al.RefreshAt

	if p.opts.ProbeChat {
		payload := map[string]any{
			"model":      firstNonEmpty(p.opts.Config.ProbeModel, DefaultProbeModel),
			"stream":     false,
			"max_tokens": 8,
			"messages": []any{
				map[string]any{"role": "user", "content": "Reply with exactly: OK"},
			},
		}
		body, _ := json.Marshal(payload)
		resp, err := up.OpenModel(vctx, acct, deepai.PathChat, body, "")
		if err != nil {
			return fmt.Errorf("对话验活失败: %w", err)
		}
		defer resp.Body.Close()
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		if resp.StatusCode/100 != 2 {
			return fmt.Errorf("对话验活 HTTP %d: %s", resp.StatusCode, truncate(string(raw), 200))
		}
	}
	return nil
}
