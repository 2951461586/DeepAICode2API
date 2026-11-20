package observe

import (
	"strings"
	"testing"
	"time"
)

// TestHistQuantile 固化桶分位：返回所在桶的上界，样本不足时为 0。
func TestHistQuantile(t *testing.T) {
	h := newHist([]float64{0.1, 0.5, 1, 5})
	if got := h.quantile(0.5); got != 0 {
		t.Fatalf("空直方图分位 = %v，期望 0", got)
	}
	h.observe(0.05)
	h.observe(0.3)
	h.observe(0.9)
	h.observe(9)
	// 4 个样本，p50 的秩 = 2：第 1 个在 <=0.1 桶、第 2 个在 <=0.5 桶，
	// 累计到 2 时命中 0.5 桶，返回该桶上界。
	if got := h.quantile(0.5); got != 0.5 {
		t.Fatalf("p50 = %v，期望 0.5", got)
	}
	// 样本少时秩会截断：4*0.95=3.8 → 秩 3，落在 <=1 桶。
	// 这正是"返回桶上界"的取向：宁可给已知的保守值，不插值造假精度。
	if got := h.quantile(0.95); got != 1 {
		t.Fatalf("p95 = %v，期望 1", got)
	}
}

// TestMetricsRecordsExtras 覆盖扩展指标的累加口径。
func TestMetricsRecordsExtras(t *testing.T) {
	m := NewMetrics()
	m.Observe("chat", 200, 100*time.Millisecond)
	m.Observe("responses", 502, 3*time.Second)
	m.ObserveUpstreamError("/chat/completions", "session_busy", 409)
	m.ObserveUpstreamError("/chat/completions", "session_busy", 409)
	m.AddFallback()
	m.ObserveAdmissionWait(250 * time.Millisecond)

	snap := m.Snapshot()
	if snap.Fallback != 1 {
		t.Fatalf("fallback = %d，期望 1", snap.Fallback)
	}
	if len(snap.UpstreamErrs) != 1 || snap.UpstreamErrs[0].Code != "session_busy" || snap.UpstreamErrs[0].Count != 2 {
		t.Fatalf("上游错误聚合 = %+v", snap.UpstreamErrs)
	}
	if snap.Latency.Total != 2 {
		t.Fatalf("延迟样本 = %d，期望 2", snap.Latency.Total)
	}
	if _, ok := snap.ByEndpoint["chat"]; !ok {
		t.Fatalf("缺少 chat 端点分布: %+v", snap.ByEndpoint)
	}
	if snap.Admission.Total != 1 {
		t.Fatalf("排队样本 = %d，期望 1", snap.Admission.Total)
	}
}

// TestMetricsStatefulAndCheckIn 覆盖状态字段计数与签到口径。
func TestMetricsStatefulAndCheckIn(t *testing.T) {
	m := NewMetrics()
	m.AddStateful("previous_response_id", true)
	m.AddStateful("previous_response_id", false)
	m.AddStateful("conversation", false)
	m.AddStateful("item_reference", false)

	const slow = 2 * time.Second
	m.ObserveCheckIn(false, 0, slow, false)           // 未尝试：什么都不记
	m.ObserveCheckIn(true, 0, slow, false)            // attempt
	m.ObserveCheckIn(true, 3*time.Second, slow, true) // attempt + blocked + timeout

	snap := m.Snapshot()
	if got := snap.Stateful["previous_response_id"]; got != 2 {
		t.Fatalf("previous_response_id = %d，期望 2", got)
	}
	if got := snap.Stateful["rejected"]; got != 1 {
		t.Fatalf("rejected = %d，期望 1", got)
	}
	if got := snap.CheckInStats["attempts"]; got != 2 {
		t.Fatalf("attempts = %d，期望 2", got)
	}
	if got := snap.CheckInStats["blocked"]; got != 1 {
		t.Fatalf("blocked = %d，期望 1", got)
	}
	if got := snap.CheckInStats["timeouts"]; got != 1 {
		t.Fatalf("timeouts = %d，期望 1", got)
	}
}

// TestWritePrometheusContract 锁住 /metrics 的对外契约：指标名、标签与关键取值。
//
// 这些名字是看板 / 告警的输入，改名等于破坏兼容；顺序固定便于 diff。
func TestWritePrometheusContract(t *testing.T) {
	m := NewMetrics()
	m.Observe("chat", 200, 120*time.Millisecond)
	m.Observe("responses", 502, 3*time.Second)
	m.AddFallback()
	m.ObserveUpstreamError("/chat/completions", "lane_version_unsupported", 426)
	m.ObserveCheckIn(true, 0, 2*time.Second, false)
	m.ObserveCheckIn(true, 3*time.Second, 2*time.Second, true)

	var b strings.Builder
	m.WritePrometheus(&b, PromExtra{
		Pool:         PoolSnapshot{Total: 3, Ready: 2, Degraded: 1, Blocked: 0, QuotaFresh: 2, QuotaDue: 1},
		LogRequests:  10,
		LogFailed:    1,
		Keys:         2,
		Lane:         7,
		CompiledLane: 7,
		UpstreamLane: 9,
	})
	body := b.String()
	for _, needle := range []string{
		`gateway_requests_total{class="2xx"} 1`,
		`gateway_requests_total{class="5xx"} 1`,
		`gateway_accounts{state="total"} 3`,
		`gateway_accounts{state="quota_due"} 1`,
		`gateway_log_requests_total 10`,
		`gateway_keys 2`,
		`gateway_lane 7`,
		`gateway_lane{source="compiled"} 7`,
		`gateway_upstream_lane 9`,
		`gateway_request_duration_seconds_bucket{endpoint="chat",le="0.25"} 1`,
		"gateway_checkin_seconds_bucket",
		"gateway_admission_wait_seconds_bucket",
		`gateway_checkin_total{result="attempt"} 2`,
		`gateway_checkin_total{result="blocked"} 1`,
		`gateway_checkin_total{result="timeout"} 1`,
		`gateway_fallback_total{reason="body_shape"} 1`,
		`gateway_upstream_errors_total{code="lane_version_unsupported",status="426",path="/chat/completions"} 1`,
		"go_goroutines",
		"go_memstats_heap_alloc_bytes",
	} {
		if !strings.Contains(body, needle) {
			t.Fatalf("/metrics 缺少 %q\n%s", needle, body)
		}
	}
	// 直方图必须带 +Inf 桶与 sum/count 后缀。
	for _, needle := range []string{`le="+Inf"`, "gateway_request_duration_seconds_sum", "gateway_request_duration_seconds_count"} {
		if !strings.Contains(body, needle) {
			t.Fatalf("直方图缺少 %q", needle)
		}
	}
}

// TestEndpointLabel 验证低基数标签。
func TestEndpointLabel(t *testing.T) {
	cases := map[string]string{
		"/v1/chat/completions": "chat",
		"/v1/responses":        "responses",
		"/v1/messages":         "messages",
		"/v1/models":           "other",
		"/ping":                "other",
	}
	for in, want := range cases {
		if got := EndpointLabel(in); got != want {
			t.Fatalf("EndpointLabel(%q) = %q，期望 %q", in, got, want)
		}
	}
}

// TestIsStaticAsset 确认图标不会被写进访问日志。
//
// 浏览器每次打开页面都会自动请求 /favicon.ico（以及 Apple 的 touch icon），
// 它们和 /style.css、/js/* 一样是纯静态资源：进日志只会把 access 行刷成
// 「一半是图标」的噪声，还会让 /metrics 的 2xx 计数虚高。
func TestIsStaticAsset(t *testing.T) {
	for _, p := range []string{"/favicon.ico", "/apple-touch-icon.png", "/style.css", "/js/app.js", "/ping"} {
		if !IsStaticAsset(p) {
			t.Errorf("%s 应视为静态资源（不写访问日志）", p)
		}
	}
	for _, p := range []string{"/", "/login", "/admin/api/logs", "/v1/models"} {
		if IsStaticAsset(p) {
			t.Errorf("%s 不是静态资源，不应被跳过", p)
		}
	}
}
