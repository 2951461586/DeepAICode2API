package observe

import (
	"fmt"
	"io"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

// itoa64 / ftoa 是 Prometheus 文本格式的必填转换。
func itoa64(v int64) string { return strconv.FormatInt(v, 10) }
func ftoa(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }

// PoolSnapshot 是号池健康度的一次快照（控制台 / /health / /metrics 共用口径）。
//
// 数值由调用方算好传进来：本包不认识账号与额度策略。
type PoolSnapshot struct {
	Total, Ready, Degraded, Blocked int
	QuotaFresh, QuotaDue            int
}

// PromExtra 是 /metrics 里那些与请求计数无关、必须由调用方提供的数字。
type PromExtra struct {
	Pool         PoolSnapshot
	LogRequests  int64 // 请求日志累计条数
	LogFailed    int64 // 失败请求累计条数
	Keys         int   // 有效 API 密钥数量
	Lane         int   // 网关当前发出的 Theos 车道
	CompiledLane int   // 编译期车道
	UpstreamLane int64 // 上游 /theos 回报的车道（0 = 尚未观测）
}

// WritePrometheus 输出 Prometheus 文本格式指标。
//
// 指标名 / 标签 / 顺序都是外部契约（看板与告警按名字取值），改动等于破坏兼容。
func (m *Metrics) WritePrometheus(w io.Writer, extra PromExtra) {
	var b strings.Builder
	metric := func(name, typ, help string, samples ...[2]string) {
		fmt.Fprintf(&b, "# HELP %s %s\n# TYPE %s %s\n", name, help, name, typ)
		for _, s := range samples {
			fmt.Fprintf(&b, "%s %s\n", s[0], s[1])
		}
	}
	counter := func(name, help string, samples ...[2]string) { metric(name, "counter", help, samples...) }
	gauge := func(name, help string, samples ...[2]string) { metric(name, "gauge", help, samples...) }

	counter("gateway_requests_total", "网关处理的请求总数（按状态码分类）",
		[2]string{`gateway_requests_total{class="2xx"}`, itoa64(m.c2xx.Load())},
		[2]string{`gateway_requests_total{class="3xx"}`, itoa64(m.c3xx.Load())},
		[2]string{`gateway_requests_total{class="4xx"}`, itoa64(m.c4xx.Load())},
		[2]string{`gateway_requests_total{class="5xx"}`, itoa64(m.c5xx.Load())},
	)
	// 耗时用 sum + count 表示：不保留分位数，避免进程内维护直方图的开销。
	counter("gateway_request_duration_seconds_sum", "请求耗时总和（秒）",
		[2]string{"gateway_request_duration_seconds_sum", ftoa(float64(m.durSumNS.Load()) / float64(time.Second))})
	counter("gateway_request_duration_seconds_count", "请求耗时样本数",
		[2]string{"gateway_request_duration_seconds_count", itoa64(m.durCount.Load())})
	gauge("gateway_inflight_requests", "当前在途请求数",
		[2]string{"gateway_inflight_requests", itoa64(m.inflight.Load())})
	gauge("gateway_accounts", "号池账号数量（按状态）",
		[2]string{`gateway_accounts{state="total"}`, itoa64(int64(extra.Pool.Total))},
		[2]string{`gateway_accounts{state="ready"}`, itoa64(int64(extra.Pool.Ready))},
		[2]string{`gateway_accounts{state="degraded"}`, itoa64(int64(extra.Pool.Degraded))},
		[2]string{`gateway_accounts{state="blocked"}`, itoa64(int64(extra.Pool.Blocked))},
		[2]string{`gateway_accounts{state="quota_fresh"}`, itoa64(int64(extra.Pool.QuotaFresh))},
		[2]string{`gateway_accounts{state="quota_due"}`, itoa64(int64(extra.Pool.QuotaDue))},
	)
	gauge("gateway_log_requests_total", "请求日志累计条数（stats 口径）",
		[2]string{"gateway_log_requests_total", itoa64(extra.LogRequests)})
	gauge("gateway_log_failed_total", "失败请求累计条数（stats 口径）",
		[2]string{"gateway_log_failed_total", itoa64(extra.LogFailed)})
	gauge("gateway_keys", "有效 API 密钥数量",
		[2]string{"gateway_keys", itoa64(int64(extra.Keys))})
	// 上游车道：与 gateway_lane 对比即可发现协议漂移（上游 > 网关 = 该升级了）。
	gauge("gateway_lane", "网关当前发出的 Theos 车道",
		[2]string{"gateway_lane", itoa64(int64(extra.Lane))},
		[2]string{`gateway_lane{source="compiled"}`, itoa64(int64(extra.CompiledLane))},
	)
	gauge("gateway_upstream_lane", "上游 /theos 回报的车道（0 = 尚未观测）",
		[2]string{"gateway_upstream_lane", itoa64(extra.UpstreamLane)})
	gauge("go_goroutines", "当前 goroutine 数",
		[2]string{"go_goroutines", itoa64(int64(runtime.NumGoroutine()))})
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	gauge("go_memstats_heap_alloc_bytes", "堆上已分配字节数",
		[2]string{"go_memstats_heap_alloc_bytes", itoa64(int64(ms.HeapAlloc))})

	// 扩展指标：分端点延迟、签到、兜底、上游错误码。
	snap := m.Snapshot()
	endpoints := make([]string, 0, len(snap.ByEndpoint))
	for k := range snap.ByEndpoint {
		endpoints = append(endpoints, k)
	}
	sort.Strings(endpoints)
	for _, ep := range endpoints {
		appendHistogram(&b, "gateway_request_duration_seconds",
			"HTTP 请求耗时（按端点）", "endpoint", ep, snap.ByEndpoint[ep])
	}
	appendHistogram(&b, "gateway_checkin_seconds", "推理前同步签到的等待耗时", "", "", snap.CheckIn)
	appendHistogram(&b, "gateway_admission_wait_seconds", "账号全忙时的排队等待耗时", "", "", snap.Admission)

	counter("gateway_checkin_total", "推理前的签到尝试（attempt / 慢阻塞 / 超预算）",
		[2]string{`gateway_checkin_total{result="attempt"}`, itoa64(m.checkinAttempts.Load())},
		[2]string{`gateway_checkin_total{result="blocked"}`, itoa64(m.checkinBlocked.Load())},
		[2]string{`gateway_checkin_total{result="timeout"}`, itoa64(m.checkinTimeouts.Load())},
	)
	counter("gateway_fallback_total", "请求体形状被拒后按桌面端形状重试的次数",
		[2]string{`gateway_fallback_total{reason="body_shape"}`, itoa64(m.fallbackTotal.Load())},
	)
	// 依赖上游响应状态的请求：这些是「上下文看起来丢了」的第一现场。
	// 合成路径（chat）会直接拒绝，透传路径交给上游（其能否续接尚未验证）。
	counter("gateway_stateful_requests_total", "依赖上游保存响应状态的请求（按字段）",
		[2]string{`gateway_stateful_requests_total{param="previous_response_id"}`, itoa64(m.statefulPrevID.Load())},
		[2]string{`gateway_stateful_requests_total{param="conversation"}`, itoa64(m.statefulConv.Load())},
		[2]string{`gateway_stateful_requests_total{param="item_reference"}`, itoa64(m.statefulItemRef.Load())},
	)
	counter("gateway_stateful_rejected_total", "因合成路径无状态而拒绝的续接请求",
		[2]string{`gateway_stateful_rejected_total{reason="stateless_translation"}`, itoa64(m.statefulReject.Load())},
	)
	errSamples := make([][2]string, 0, len(snap.UpstreamErrs))
	for _, e := range snap.UpstreamErrs {
		labels := fmt.Sprintf(`code=%q,status="%d",path=%q`, e.Code, e.Status, e.Path)
		errSamples = append(errSamples, [2]string{"gateway_upstream_errors_total{" + labels + "}", itoa64(e.Count)})
	}
	if len(errSamples) > 0 {
		counter("gateway_upstream_errors_total", "上游拒绝次数（按路径 / 错误码 / 状态码）", errSamples...)
	}

	_, _ = io.WriteString(w, b.String())
}

// appendHistogram 输出一个 Prometheus 直方图（含 +Inf 桶与 sum/count）。
//
// labelName 为空表示无标签；否则按 labelName="labelValue" 输出。
// 输出顺序固定（桶升序），便于 diff 与抓取端做稳定性检查。
func appendHistogram(b *strings.Builder, name, help, labelName, labelValue string, v HistogramView) {
	label := ""
	if labelName != "" {
		label = labelName + "=" + strconv.Quote(labelValue) + ","
	}
	fmt.Fprintf(b, "# HELP %s %s\n# TYPE %s histogram\n", name, help, name)
	var cum uint64
	for i, bound := range v.Bounds {
		cum += v.Counts[i]
		le := strconv.Quote(strconv.FormatFloat(bound, 'f', -1, 64))
		fmt.Fprintf(b, "%s_bucket{%sle=%s} %d\n", name, label, le, cum)
	}
	cum += v.Counts[len(v.Bounds)]
	fmt.Fprintf(b, "%s_bucket{%sle=\"+Inf\"} %d\n", name, label, cum)
	fmt.Fprintf(b, "%s_sum %s\n", name, ftoa(v.Sum))
	fmt.Fprintf(b, "%s_count %d\n", name, v.Total)
}
