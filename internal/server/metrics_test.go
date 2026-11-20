package server

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

// TestMetricsEndpointExposition 断言 /metrics 暴露了新增系列。
func TestMetricsEndpointExposition(t *testing.T) {
	s, key := newTestServer(t, nil)
	s.metrics.Observe("chat", 200, 120*time.Millisecond)
	s.metrics.AddFallback()
	s.metrics.ObserveUpstreamError("/chat/completions", "lane_version_unsupported", 426)
	s.metrics.ObserveCheckIn(true, 0, checkInSlowWarn, false)
	s.metrics.ObserveCheckIn(true, 3*time.Second, checkInSlowWarn, true)
	s.noteUpstreamLane(9)

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
		`gateway_upstream_lane 9`,
		`gateway_lane{source="compiled"}`,
	} {
		if !strings.Contains(body, needle) {
			t.Fatalf("/metrics 缺少 %q\n%s", needle, body[:min(len(body), 4000)])
		}
	}
}

// TestOverviewIncludesRuntimeHealth 断言控制台概览带上运行健康字段。
func TestOverviewIncludesRuntimeHealth(t *testing.T) {
	s, key := newTestServer(t, nil)
	s.metrics.Observe("chat", 200, 100*time.Millisecond)
	s.metrics.AddFallback()

	rec := do(t, s, http.MethodGet, "/admin/api/overview", key, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("状态码 = %d", rec.Code)
	}
	body := rec.Body.String()
	for _, needle := range []string{`"latency"`, `"checkin"`, `"upstream_errors"`, `"fallback_total"`, `"lane_drift"`, `"lane_upstream"`} {
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
