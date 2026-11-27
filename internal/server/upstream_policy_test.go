package server

import (
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"deepaicode2api/internal/deepai"
)

func mustParseTime(t *testing.T, s string) time.Time {
	t.Helper()
	ts, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatalf("解析时间 %q: %v", s, err)
	}
	return ts
}

// TestClassifyUpstreamError 固化错误 → 动作映射（表驱动，新增错误码时先改这里）。
func TestClassifyUpstreamError(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
		want   upstreamAction
	}{
		{"邮箱未验证", http.StatusForbidden, `{"error":{"code":"email_unverified"}}`, actBlockAndRetry},
		{"策略拦截", http.StatusForbidden, `{"error":{"code":"social_login_required"}}`, actBlockAndReturn},
		{"缺机器指纹", http.StatusBadRequest, `{"error":{"code":"machine_required"}}`, actReturn},
		{"车道不对", http.StatusUpgradeRequired, `{"error":{"code":"lane_version_unsupported"}}`, actProbeLane},
		{"额度用完", http.StatusForbidden, `{"error":{"code":"theos_exhausted"}}`, actQuotaExhausted},
		{"容量拒绝", http.StatusConflict, `{"error":{"code":"session_busy"}}`, actWaitCapacity},
		{"形状被拒（旧文案）", http.StatusBadRequest, `{"error":{"message":"This request is not one DeepAI Code sends."}}`, actShapeFallback},
		{"形状被拒（新文案）", http.StatusBadRequest, `{"error":{"message":"This request cannot be accepted."}}`, actShapeFallback},
		{"普通 403", http.StatusForbidden, `{"error":{"code":"forbidden"}}`, actRetryOther},
		{"普通 429", http.StatusTooManyRequests, `{"error":{"code":"rate_limited"}}`, actRetryOther},
		{"形状被拒（只认 code）", http.StatusBadRequest, `{"error":{"code":"invalid_request_error","message":"unknown wording"}}`, actShapeFallback},
		{"普通 400", http.StatusBadRequest, `{"error":{"code":"bad_param","message":"bad param"}}`, actReturn},
		{"200 不应进入本函数", http.StatusOK, `{}`, actReturn},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := &deepai.HTTPError{Status: c.status, Body: []byte(c.body)}
			if got := classifyUpstreamError(e); got != c.want {
				t.Fatalf("classify = %v，期望 %v", got, c.want)
			}
		})
	}
	if got := classifyUpstreamError(nil); got != actReturn {
		t.Fatalf("nil 错误应直接回传，实际 %v", got)
	}
}

// TestQuotaResetAt 覆盖 refresh_at 解析与兜底。
func TestQuotaResetAt(t *testing.T) {
	now := mustParseTime(t, "2026-11-13T12:00:00Z")

	withRefresh := &deepai.HTTPError{Status: http.StatusForbidden,
		Body: []byte(`{"error":{"code":"theos_exhausted","refresh_at":"2026-11-14T00:05:52+00:00"}}`)}
	got := withRefresh.QuotaResetAt(quotaExhaustedFallback, now)
	want := mustParseTime(t, "2026-11-14T00:05:52Z")
	if !got.Equal(want) {
		t.Fatalf("refresh_at = %v，期望 %v", got, want)
	}

	noRefresh := &deepai.HTTPError{Status: http.StatusForbidden, Body: []byte(`{"error":{"code":"theos_exhausted"}}`)}
	got = noRefresh.QuotaResetAt(quotaExhaustedFallback, now)
	if !got.Equal(now.Add(quotaExhaustedFallback)) {
		t.Fatalf("缺少 refresh_at 应回落到 6h，得到 %v", got)
	}
}

// TestSameAccountRetryable 锁住「同号重试」的边界。
//
// 换号的隐性代价是上游前缀缓存整段冷启动（命中率直接归零），所以凡是
// 「号本身没坏、只是链路抖了一下」的失败都值得同号再来一次；而 429 / 403 /
// 400 / 426 是账号级或请求级语义，同号重试只会白等一次往返。
func TestSameAccountRetryable(t *testing.T) {
	cases := []struct {
		name   string
		err    *deepai.HTTPError
		expect bool
	}{
		{"nil", nil, false},
		{"502 断流", &deepai.HTTPError{Status: 502}, true},
		{"503", &deepai.HTTPError{Status: 503}, true},
		{"504", &deepai.HTTPError{Status: 504}, true},
		{"500", &deepai.HTTPError{Status: 500}, true},
		{"429 账号级限流", &deepai.HTTPError{Status: 429}, false},
		{"403 策略", &deepai.HTTPError{Status: 403}, false},
		{"400 语义", &deepai.HTTPError{Status: 400}, false},
		{"426 车道", &deepai.HTTPError{Status: 426}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := sameAccountRetryable(c.err); got != c.expect {
				t.Fatalf("sameAccountRetryable = %v，期望 %v", got, c.expect)
			}
		})
	}
}

// TestOpenModelRetries5xxOnSameAccount 是端到端证据：上游第一次 502、
// 第二次成功时，两次请求必须打到**同一个账号**（同一个上游 Api-Key）。
//
// 这条断言直接对应线上观测：同一段 130+ 条消息的会话命中率 98~100%，
// 只有换号那两次掉到 0%。换号重试会把这行数字变成常态。
func TestOpenModelRetries5xxOnSameAccount(t *testing.T) {
	var calls atomic.Int32
	u := newUpstream(t, func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, deepai.PathChat) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"enabled":true,"balance":"100.00000","lane_version":8}`)
			return
		}
		if calls.Add(1) == 1 {
			w.WriteHeader(http.StatusBadGateway)
			_, _ = io.WriteString(w, `{"error":{"code":"upstream_error","message":"unexpected EOF"}}`)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"x","object":"chat.completion","model":"glm-5.3-flash",
		  "choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],
		  "usage":{"prompt_tokens":10,"completion_tokens":2,"total_tokens":12}}`)
	})

	s, key := newTestServer(t, u)
	// 第二个号：如果重试换了号，第二次请求的 Api-Key 就会变成它。
	s.Store.UpsertAccount(&deepai.Account{
		ID: "acc-2", Name: "test2", APIKey: "upstream-key-2", Enabled: true, Status: "ok",
	})

	body := `{"model":"glm-5.3-flash","messages":[{"role":"user","content":"hi"}]}`
	rec := do(t, s, http.MethodPost, "/v1/chat/completions", key, body)
	if rec.Code != http.StatusOK {
		t.Fatalf("状态码 = %d，期望 200（body=%s）", rec.Code, rec.Body.String())
	}

	u.mu.Lock()
	var keys []string
	for _, r := range u.requests {
		if strings.HasSuffix(r.Path, deepai.PathChat) {
			keys = append(keys, r.Header.Get("Api-Key"))
		}
	}
	u.mu.Unlock()
	if len(keys) != 2 {
		t.Fatalf("上行请求数 = %d，期望 2（一次 502 + 一次同号重试）", len(keys))
	}
	if keys[0] != keys[1] {
		t.Fatalf("重试换了号：%v —— 5xx 应在同一个号上重试以保住前缀缓存", keys)
	}
}
