package register

import (
	"context"
	"errors"
	"sync"
	"time"

	"deepaicode2api/internal/deepai"
)

// errNilWebClient 表示调用补验证时没给 web 客户端。
var errNilWebClient = errors.New("web 客户端为空")

// 验证链接点击的上游特征（来自 HAR 实测）：
//
//	GET https://api.deepai.org/verify_email?key=…  →  200 text/html
//	服务端 wait ≈ 13.2s（占总耗时 95%）
//
// 一个请求就要占上游 Passenger worker 13 秒。10 路并发同时点，worker 池会被打满，
// 表现就是 502「Incomplete response received from application」/ unexpected EOF /
// 60s 客户端超时 —— 第八批 0/50 的验证失败正是这么来的。
//
// 所以：验证点击必须限并发（默认 2），失败按 2s/10s/60s 退避。
var (
	// verifyClickSlots 限制同时打到上游的验证请求数（全局，非按 Pipeline 实例）。
	verifyClickSlots = make(chan struct{}, 2)

	// verifyRetryDelays 是点击验证链接的退避序列。
	verifyRetryDelays = []time.Duration{2 * time.Second, 10 * time.Second, 60 * time.Second}
)

// verifyEmailThrottled 在并发闸门内点击验证链接，并对瞬时错误退避重试。
//
// 闸门按“每次尝试”获取，退避等待期间不占坑，避免一个慢请求把名额攥死。
func verifyEmailThrottled(ctx context.Context, web *WebClient, link string) error {
	if web == nil {
		return errNilWebClient
	}
	var lastErr error
	for attempt := 0; ; attempt++ {
		if err := withVerifySlot(ctx, func() error { return web.VerifyEmail(ctx, link) }); err == nil {
			return nil
		} else {
			lastErr = err
			// 域名不合法 / 4xx 这类问题重试没意义。
			if !verifyRetryable(err) {
				return err
			}
		}
		if attempt >= len(verifyRetryDelays) {
			return lastErr
		}
		if !sleepCtx(ctx, verifyRetryDelays[attempt]) {
			return lastErr
		}
	}
}

// withVerifySlot 在拿到并发名额后执行 fn。
func withVerifySlot(ctx context.Context, fn func() error) error {
	select {
	case verifyClickSlots <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}
	defer func() { <-verifyClickSlots }()
	return fn()
}

// verifyRetryable 判断验证失败是否值得重试。
//
// 除网络/5xx 之外，上游在 worker 被打满时还会回 502 或直接断连，
// 这些都是瞬时状态，等一会儿再试就能过。
func verifyRetryable(err error) bool {
	if err == nil {
		return false
	}
	var we *WebError
	if errors.As(err, &we) {
		return we.Status >= 500 || we.Status == 408 || we.Status == 429
	}
	return deepai.IsTransient(err)
}

// VerifyTarget 是一个待补验证的账号。
type VerifyTarget struct {
	Email       string
	Group       string
	Proxy       string
	Fingerprint string
}

// ReverifyOptions 是补验证的调度参数。
type ReverifyOptions struct {
	Config    *Config
	Provider  Provider
	Marker    VerifiedMarker
	Log       func(format string, args ...any)
	WaitFor   time.Duration // 单账号找信的最长等待，默认 20s
	PollEvery time.Duration // 收信轮询间隔，默认 1s
	Workers   int           // 并发账号数，默认 2
}

// ReverifyResult 是单个账号的补验证结果。
type ReverifyResult struct {
	Email string
	OK    bool
	Err   string
}

// Reverify 对一批账号重新收信、点击验证链接，成功后回填 email_verified。
//
// 用于上游抖动导致「拿到链接但点不动」的批次补跑：链接本身没有时效问题，
// 重新从收件箱取一次再点即可。整体并发默认 2，配合 verifyEmailThrottled 的
// 闸门，避免再一次把上游 worker 打满。
func Reverify(ctx context.Context, opts ReverifyOptions, targets []VerifyTarget) []ReverifyResult {
	results := make([]ReverifyResult, 0, len(targets))
	if len(targets) == 0 {
		return results
	}
	cfg := opts.Config
	if cfg == nil {
		cfg = LoadConfig()
	}
	inbox, ok := opts.Provider.(InboxProvider)
	if !ok {
		return append(results, ReverifyResult{Err: "当前邮箱后端不支持打开已有收件箱"})
	}
	wait := opts.WaitFor
	if wait <= 0 {
		wait = 20 * time.Second
	}
	poll := opts.PollEvery
	if poll <= 0 {
		poll = time.Second
	}
	workers := opts.Workers
	if workers <= 0 {
		workers = 2
	}
	logf := opts.Log
	if logf == nil {
		logf = func(string, ...any) {}
	}

	jobs := make(chan VerifyTarget)
	out := make(chan ReverifyResult, len(targets))
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for t := range jobs {
				out <- reverifyOne(ctx, cfg, inbox, opts.Marker, logf, t, wait, poll)
			}
		}()
	}
	go func() {
		defer close(jobs)
		for _, t := range targets {
			select {
			case jobs <- t:
			case <-ctx.Done():
				return
			}
		}
	}()
	wg.Wait()
	close(out)
	for r := range out {
		results = append(results, r)
	}
	return results
}

func reverifyOne(ctx context.Context, cfg *Config, inbox InboxProvider, marker VerifiedMarker,
	logf func(string, ...any), t VerifyTarget, wait, poll time.Duration) ReverifyResult {

	res := ReverifyResult{Email: t.Email}
	mb, ok := inbox.Inbox(t.Email)
	if !ok {
		res.Err = "收件箱不可用"
		return res
	}
	// since 传零值表示“接受收件箱里的任意邮件”（忽略时间过滤）。
	link, err := mb.WaitForVerification(ctx, time.Time{}, wait, poll)
	if err != nil {
		res.Err = "取验证链接失败: " + err.Error()
		return res
	}

	web := NewWebClientWithFingerprint(cfg, FingerprintByID(t.Fingerprint), t.Proxy)
	if err := verifyEmailThrottled(ctx, web, link); err != nil {
		res.Err = "点击验证链接失败: " + err.Error()
		return res
	}
	if marker != nil {
		if err := marker.MarkVerified(ctx, "", t.Email); err != nil {
			res.Err = "回填失败: " + err.Error()
			return res
		}
	}
	logf("[%s] 补验证成功", t.Email)
	res.OK = true
	return res
}
