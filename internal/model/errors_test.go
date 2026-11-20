package model

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

// mustErr 构造一个带指定状态码与响应体的上游错误。
func mustErr(t *testing.T, status int, body string) *HTTPError {
	t.Helper()
	return &HTTPError{Op: "上游推理", Status: status, Body: []byte(body)}
}

func TestHTTPErrorRetryable(t *testing.T) {
	cases := []struct {
		status int
		want   bool
		why    string
	}{
		{400, false, "请求本身的问题"},
		{404, false, "模型不存在，重试无意义"},
		{422, false, "参数非法"},
		{401, true, "鉴权失败可换号"},
		{403, true, ""},
		{409, true, "容量拒绝"},
		{429, true, "限流"},
		{500, true, ""},
		{502, true, ""},
		{503, true, ""},
		{504, true, ""},
		{418, false, "未知 4xx 不重试"},
	}
	for _, c := range cases {
		if got := mustErr(t, c.status, "{}").Retryable(); got != c.want {
			t.Errorf("status=%d Retryable()=%v，期望 %v（%s）", c.status, got, c.want, c.why)
		}
	}
}

func TestHTTPErrorAuthFailed(t *testing.T) {
	cases := []struct {
		status int
		body   string
		want   bool
	}{
		{401, `{}`, true},
		{403, `{"error":{"code":"auth_required"}}`, true},
		{403, `{"error":{"code":"invalid_api_key"}}`, true},
		{403, `{"error":{"code":"email_unverified"}}`, true},
		{403, `{"error":{"code":"session_busy"}}`, false},
		{500, `{}`, false},
	}
	for _, c := range cases {
		if got := mustErr(t, c.status, c.body).AuthFailed(); got != c.want {
			t.Errorf("status=%d body=%s AuthFailed()=%v，期望 %v", c.status, c.body, got, c.want)
		}
	}
}

func TestHTTPErrorCapacityWaiting(t *testing.T) {
	busy := mustErr(t, http.StatusConflict, `{"error":{"code":"session_busy"}}`)
	if !busy.CapacityWaiting() {
		t.Fatal("409 + session_busy 应判定为容量等待")
	}
	if !busy.Retryable() {
		t.Fatal("容量等待必须可重试")
	}

	conflict := mustErr(t, http.StatusConflict, `{"error":{"code":"duplicate"}}`)
	if conflict.CapacityWaiting() {
		t.Fatal("非 session_busy 的 409 不应判定为容量等待")
	}
	if got := mustErr(t, 429, `{"error":{"code":"session_busy"}}`).CapacityWaiting(); got {
		t.Fatal("非 409 即使 code=session_busy 也不算容量等待")
	}
}

func TestHTTPErrorCodeAndMessage(t *testing.T) {
	e := mustErr(t, 409, `{"error":{"message":"模型正忙","code":"session_busy"}}`)
	if e.Code() != "session_busy" {
		t.Fatalf("Code() = %q", e.Code())
	}
	if got := e.Error(); !strings.Contains(got, "模型正忙") {
		t.Fatalf("Error() 应优先使用上游 message，得到 %q", got)
	}

	if got := mustErr(t, 500, "not json").Code(); got != "" {
		t.Fatalf("非 JSON body 的 Code() 应为空，得到 %q", got)
	}
}

func TestHTTPErrorTruncatesLongBody(t *testing.T) {
	// 上游异常时会回吐 HTML / 堆栈，不能整段塞进错误串再写进日志。
	long := strings.Repeat("A", 1000)
	got := mustErr(t, 502, long).Error()
	if !strings.HasSuffix(got, "…") {
		t.Fatalf("长 body 应被截断并以省略号结尾: %q", got)
	}
	if strings.Count(got, "A") > 301 {
		t.Fatalf("截断长度不合理：%d 个 A", strings.Count(got, "A"))
	}

	if got := mustErr(t, 500, "   ").Error(); !strings.Contains(got, "HTTP 500") {
		t.Fatalf("空 body 应回退到状态码: %q", got)
	}
}

// TestBodyRejectedRequires400 固化「只有 400 且文案匹配才算请求体形状拒绝」。
func TestBodyRejectedRequires400(t *testing.T) {
	msg := `{"error":{"message":"This request is not one DeepAI Code sends.","code":"invalid_request_error"}}`
	if !mustErr(t, http.StatusBadRequest, msg).BodyRejected() {
		t.Fatal("400 + 固定文案应判定为请求体拒绝")
	}
	if mustErr(t, http.StatusInternalServerError, msg).BodyRejected() {
		t.Fatal("非 400 不应判定为请求体拒绝")
	}
	if mustErr(t, http.StatusBadRequest, `{"error":{"message":"bad param"}}`).BodyRejected() {
		t.Fatal("文案不匹配不应判定为请求体拒绝")
	}
	if mustErr(t, http.StatusBadRequest, ``).BodyRejected() {
		t.Fatal("空 body 不应判定为请求体拒绝")
	}
}

// TestBodyRejectedAcceptsNewMessage 覆盖 2026-11-13 实测到的新文案：
// 上游把同一个门槛的 message 换成了 “This request cannot be accepted.”，
// 漏掉它 ChatBodyFallback 就不会重试，调用方直接吃 400。
func TestBodyRejectedAcceptsNewMessage(t *testing.T) {
	msg := `{"error":{"message":"This request cannot be accepted.","type":"invalid_request_error","code":"invalid_request_error"}}`
	if !mustErr(t, http.StatusBadRequest, msg).BodyRejected() {
		t.Fatal("400 + 新文案（cannot be accepted）应判定为请求体拒绝")
	}
	if mustErr(t, http.StatusForbidden, msg).BodyRejected() {
		t.Fatal("非 400 不应判定为请求体拒绝")
	}
}

// TestQuotaExhaustedDetectionAndReset 覆盖 403 theos_exhausted 的识别与恢复时间。
func TestQuotaExhaustedDetectionAndReset(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	// refresh_at 必须是**相对** now 的时间：写死日期会让这条用例变成定时炸弹
	// （2026-11-14T00:05:52 一过，QuotaResetAt 按“已过期”回退到 now+6h，CI 就红）。
	resetAt := now.Add(2 * time.Hour)
	stamp := resetAt.Format(time.RFC3339)
	body := `{"error":{"message":"You've used today's Theos. They refresh at ` + stamp + `.",` +
		`"type":"theos_exhausted","code":"theos_exhausted","refresh_at":"` + stamp + `"}}`
	e := mustErr(t, http.StatusForbidden, body)
	if !e.QuotaExhausted() {
		t.Fatal("应识别为 theos_exhausted")
	}
	if e.SocialLoginRequired() || e.EmailUnverified() || e.MachineRequired() {
		t.Fatal("不应误判为其它账号终态")
	}
	if got := e.QuotaResetAt(6*time.Hour, now); !got.Equal(resetAt) {
		t.Fatalf("恢复时间 = %v，期望 %v", got, resetAt)
	}
	// refresh_at 已经过期（上游给的是过去时间）时用 fallback，而不是立刻重试。
	past := now.Add(-time.Hour).UTC().Format(time.RFC3339)
	stale := mustErr(t, http.StatusForbidden,
		`{"error":{"code":"theos_exhausted","refresh_at":"`+past+`"}}`)
	if got, want := stale.QuotaResetAt(6*time.Hour, now), now.Add(6*time.Hour); !got.Equal(want) {
		t.Fatalf("过期 refresh_at 应回退到 now+6h，实际 %v", got)
	}
	// 其它错误码不应命中。
	if mustErr(t, http.StatusForbidden, `{"error":{"code":"check_in_refused"}}`).QuotaExhausted() {
		t.Fatal("check_in_refused 不应识别为额度用完")
	}
}
