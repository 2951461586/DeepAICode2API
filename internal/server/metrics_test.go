package server

import (
	"net/http"
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
	m := newMetrics()
	m.observe("chat", 200, 100*time.Millisecond)
	m.observe("responses", 502, 3*time.Second)
	m.observeUpstreamError("/chat/completions", "session_busy", 409)
	m.observeUpstreamError("/chat/completions", "session_busy", 409)
	m.addFallback()
	m.observeAdmissionWait(250 * time.Millisecond)

	snap := m.snapshot()
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

// TestMetricsEndpointExposition 断言 /metrics 暴露了新增系列。
func TestMetricsEndpointExposition(t *testing.T) {
	s, key := newTestServer(t, nil)
	s.metrics.observe("chat", 200, 120*time.Millisecond)
	s.metrics.addFallback()
	s.metrics.observeUpstreamError("/chat/completions", "lane_version_unsupported", 426)
	s.metrics.checkinAttempts.Add(2)
	s.metrics.checkinTimeouts.Add(1)

	rec := do(t, s, http.MethodGet, "/metrics", key, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("状态码 = %d", rec.Code)
	}
	body := rec.Body.String()
	for _, needle := range []string{
		"gateway_request_duration_seconds_bucket{endpoint=\"chat\"",
		"gateway_checkin_seconds_bucket",
		"gateway_admission_wait_seconds_bucket",
		`gateway_checkin_total{result="timeout"} 1`,
		`gateway_fallback_total{reason="body_shape"} 1`,
		`gateway_upstream_errors_total{code="lane_version_unsupported",status="426",path="/chat/completions"} 1`,
	} {
		if !strings.Contains(body, needle) {
			t.Fatalf("/metrics 缺少 %q\n%s", needle, body[:min(len(body), 4000)])
		}
	}
}

// TestOverviewIncludesRuntimeHealth 断言控制台概览带上运行健康字段。
func TestOverviewIncludesRuntimeHealth(t *testing.T) {
	s, key := newTestServer(t, nil)
	s.metrics.observe("chat", 200, 100*time.Millisecond)
	s.metrics.addFallback()

	rec := do(t, s, http.MethodGet, "/admin/api/overview", key, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("状态码 = %d", rec.Code)
	}
	body := rec.Body.String()
	for _, needle := range []string{`"latency"`, `"checkin"`, `"upstream_errors"`, `"fallback_total"`} {
		if !strings.Contains(body, needle) {
			t.Fatalf("overview 缺少 %s", needle)
		}
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
