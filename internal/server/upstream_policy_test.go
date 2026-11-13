package server

import (
	"net/http"
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
		{"普通 400", http.StatusBadRequest, `{"error":{"code":"invalid_request_error","message":"bad param"}}`, actReturn},
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
