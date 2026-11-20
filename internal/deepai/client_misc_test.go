package deepai

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// errWriter 累计写入预算耗尽后返回错误，用于覆盖 SSE 写出错误的分支。
type errWriter struct{ budget int }

func (w *errWriter) Write(p []byte) (int, error) {
	if w.budget <= 0 {
		return 0, errors.New("写入失败")
	}
	if len(p) > w.budget {
		n := w.budget
		w.budget = 0
		return n, errors.New("写入失败")
	}
	w.budget -= len(p)
	return len(p), nil
}

func TestWriteSSEFormatAndErrors(t *testing.T) {
	var buf strings.Builder
	if err := WriteSSE(&buf, []byte(`{"a":1}`)); err != nil {
		t.Fatalf("WriteSSE: %v", err)
	}
	if got := buf.String(); got != "data: {\"a\":1}\n\n" {
		t.Fatalf("WriteSSE 输出 = %q", got)
	}

	// 三段写入（"data: " 6 字节 / payload 7 字节 / "\n\n" 2 字节）
	// 任意一段失败都要把错误抛给调用方（否则流会静默截断）。
	for _, budget := range []int{0, 6, 13} {
		if err := WriteSSE(&errWriter{budget: budget}, []byte(`{"a":1}`)); err == nil {
			t.Fatalf("写入预算 %d 字节用尽时应报错", budget)
		}
	}
}

func TestWriteSSEEventFormatAndErrors(t *testing.T) {
	var buf strings.Builder
	if err := WriteSSEEvent(&buf, "content_block_delta", []byte(`{"x":1}`)); err != nil {
		t.Fatalf("WriteSSEEvent: %v", err)
	}
	want := "event: content_block_delta\ndata: {\"x\":1}\n\n"
	if got := buf.String(); got != want {
		t.Fatalf("WriteSSEEvent 输出 = %q，期望 %q", got, want)
	}

	// 空 event 名不写 event 行（OpenAI 兼容端点的形态）。
	buf.Reset()
	if err := WriteSSEEvent(&buf, "", []byte(`[DONE]`)); err != nil {
		t.Fatalf("WriteSSEEvent: %v", err)
	}
	if got := buf.String(); got != "data: [DONE]\n\n" {
		t.Fatalf("空 event 输出 = %q", got)
	}

	// "event: e\n" 9 + "data: " 6 + payload 7 + "\n\n" 2 = 24 字节。
	for _, budget := range []int{0, 5, 9, 15, 22} {
		if err := WriteSSEEvent(&errWriter{budget: budget}, "e", []byte(`{"x":1}`)); err == nil {
			t.Fatalf("写入预算 %d 字节用尽时应报错", budget)
		}
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

// TestDoTransportError 覆盖连接失败：错误必须原样上抛并被判定为瞬时故障。
func TestDoTransportError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := srv.URL
	srv.Close() // 端口已关闭，后续请求连接被拒

	c := NewClient()
	c.Origin = url

	_, _, err := c.FetchModels(context.Background(), nil)
	if err == nil {
		t.Fatal("连接失败应返回错误")
	}
	if !IsTransient(err) {
		t.Fatalf("连接被拒应判定为瞬时故障，得到 %v", err)
	}

	if _, err := c.OpenModel(context.Background(), &Account{APIKey: "k"}, PathChat, []byte(`{}`), "aff"); err == nil {
		t.Fatal("OpenModel 连接失败应返回错误")
	}
	if _, err := c.Theos(context.Background(), &Account{APIKey: "k"}); err == nil {
		t.Fatal("Theos 连接失败应返回错误")
	}
	if _, err := c.postJSON(context.Background(), nil, PathWebSearch, []byte(`{}`), "联网搜索"); err == nil {
		t.Fatal("postJSON 连接失败应返回错误")
	}
	if _, err := c.Transcribe(context.Background(), nil, "", "", []byte("x")); err == nil {
		t.Fatal("Transcribe 连接失败应返回错误")
	}
	if _, err := c.ExchangeCode(context.Background(), "code"); err == nil {
		t.Fatal("ExchangeCode 连接失败应返回错误")
	}
	if _, err := c.DetectLane(context.Background(), nil); err == nil {
		t.Fatal("DetectLane 连接失败应返回错误")
	}
	if c.Lane() != LaneVersion {
		t.Fatalf("探测失败不应改车道，实际 %q", c.Lane())
	}
}

// TestDetectLaneServerErrorFallback 覆盖「全是 5xx」：应给出兜底错误，
// 因为 5xx 说明上游抖动而不是车道不对（此时 lastErr 为空）。
func TestDetectLaneServerErrorFallback(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	})
	_, err := c.DetectLane(context.Background(), nil)
	if err == nil || !strings.Contains(err.Error(), "所有候选车道都被上游拒绝") {
		t.Fatalf("全 5xx 应给出兜底错误，得到 %v", err)
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
