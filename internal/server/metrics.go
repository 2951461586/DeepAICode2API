package server

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"deepaicode2api/internal/deepai"
)

// 本文件是 /metrics 与 /admin/api/overview 的**扩展**指标层。
//
// 设计取舍：
//   - 零依赖：自己维护定桶直方图，不引入 Prometheus 客户端；
//   - 低基数：端点只分 chat / responses / messages / other，上游错误按
//     (path, code, status) 聚合并限制条目数，避免被异常流量打爆内存；
//   - 与旧指标共存：原来的 requests/2xx..5xx/durSum 保留，控制台不破。

// 延迟桶（秒）。覆盖免费模型的百毫秒级到深度思考的分钟级。
var defaultBuckets = []float64{0.05, 0.1, 0.25, 0.5, 1, 2, 5, 10, 30, 60, 120, 300, 600}

// hist 是定桶直方图：只保留每桶计数 + sum/count，够算 p50/p95/p99。
type hist struct {
	bounds []float64
	counts []uint64 // len(bounds)+1，最后一桶是 +Inf
	sum    float64
	total  uint64
}

func newHist(bounds []float64) *hist {
	return &hist{bounds: bounds, counts: make([]uint64, len(bounds)+1)}
}

func (h *hist) observe(v float64) {
	idx := sort.SearchFloat64s(h.bounds, v)
	h.counts[idx]++
	h.sum += v
	h.total++
}

// quantile 返回 q 分位所在桶的上界（q 取 0~1）。
//
// 返回桶上界而不是线性插值：调用方要的是"p95 落在 2s 这一档"，插值反而
// 会给出一个没有样本支撑的精确假象。
func (h *hist) quantile(q float64) float64 {
	if h == nil || h.total == 0 {
		return 0
	}
	if q <= 0 {
		return 0
	}
	if q > 1 {
		q = 1
	}
	target := uint64(float64(h.total) * q)
	if target == 0 {
		target = 1
	}
	var cum uint64
	for i, c := range h.counts {
		cum += c
		if cum >= target {
			if i < len(h.bounds) {
				return h.bounds[i]
			}
			return h.bounds[len(h.bounds)-1]
		}
	}
	return h.bounds[len(h.bounds)-1]
}

// histogramView 是直方图的 JSON / Prometheus 视图。
type histogramView struct {
	Bounds []float64 `json:"bounds"`
	Counts []uint64  `json:"counts"`
	Sum    float64   `json:"sum"`
	Total  uint64    `json:"total"`
}

func (h *hist) view() histogramView {
	if h == nil {
		return histogramView{}
	}
	bounds := append([]float64(nil), h.bounds...)
	counts := append([]uint64(nil), h.counts...)
	return histogramView{Bounds: bounds, Counts: counts, Sum: h.sum, Total: h.total}
}

// endpointLabel 把 URL 路径收敛成低基数标签。
func endpointLabel(path string) string {
	switch path {
	case "/v1/chat/completions":
		return "chat"
	case "/v1/responses":
		return "responses"
	case "/v1/messages":
		return "messages"
	}
	return "other"
}

// extraMetrics 是扩展指标（需要互斥锁的那部分）。
type extraMetrics struct {
	mu sync.Mutex
	// endpointDur 按端点分桶，用于回答"哪个端点在变慢"。
	endpointDur map[string]*hist
	// upstreamErr 按 (path, code, status) 聚合上游错误。
	upstreamErr map[string]int64
	// upstreamErrCap 限制条目数；溢出后全部记到 overflow 键。
	upstreamErrCap int
	// checkinDur 是同步签到等待的耗时分布。
	checkinDur *hist
	// admissionDur 是账号全忙时的排队等待分布。
	admissionDur *hist
	// durHist 是全部请求的耗时分布（与 endpointDur 之和一致）。
	durHist *hist
}

func newExtraMetrics() *extraMetrics {
	return &extraMetrics{
		endpointDur:    map[string]*hist{},
		upstreamErr:    map[string]int64{},
		upstreamErrCap: 200,
		checkinDur:     newHist(defaultBuckets),
		admissionDur:   newHist(defaultBuckets),
		durHist:        newHist(defaultBuckets),
	}
}

// observeDuration 记录一次请求耗时（秒）。
func (m *metrics) observeDuration(endpoint string, sec float64) {
	x := m.extra
	x.mu.Lock()
	defer x.mu.Unlock()
	x.durHist.observe(sec)
	h := x.endpointDur[endpoint]
	if h == nil {
		h = newHist(defaultBuckets)
		x.endpointDur[endpoint] = h
	}
	h.observe(sec)
}

// observeUpstreamError 记录一次上游拒绝（含错误码）。
func (m *metrics) observeUpstreamError(path, code string, status int) {
	if code == "" {
		code = "unknown"
	}
	key := path + "\x00" + code + "\x00" + itoa64(int64(status))
	x := m.extra
	x.mu.Lock()
	defer x.mu.Unlock()
	if _, ok := x.upstreamErr[key]; !ok && len(x.upstreamErr) >= x.upstreamErrCap {
		key = "overflow\x00overflow\x000"
	}
	x.upstreamErr[key]++
}

// observeCheckIn 记录一次推理前的签到尝试。
func (m *metrics) observeCheckIn(outcome deepai.CheckInOutcome) {
	if !outcome.Attempted {
		return
	}
	m.checkinAttempts.Add(1)
	if outcome.Waited >= checkInSlowWarn {
		m.checkinBlocked.Add(1)
	}
	if outcome.TimedOut {
		m.checkinTimeouts.Add(1)
	}
	sec := outcome.Waited.Seconds()
	x := m.extra
	x.mu.Lock()
	x.checkinDur.observe(sec)
	x.mu.Unlock()
}

// observeAdmissionWait 记录一次"账号全忙"的排队等待（秒）。
func (m *metrics) observeAdmissionWait(d time.Duration) {
	if d <= 0 {
		return
	}
	x := m.extra
	x.mu.Lock()
	x.admissionDur.observe(d.Seconds())
	x.mu.Unlock()
}

// addFallback 记录一次「请求体形状被拒 → 补形状重试」。
func (m *metrics) addFallback() { m.fallbackTotal.Add(1) }

// metricsSnapshot 是给控制台概览的聚合视图。
type metricsSnapshot struct {
	Latency      histogramView            `json:"latency"`
	ByEndpoint   map[string]histogramView `json:"by_endpoint"`
	CheckIn      histogramView            `json:"checkin"`
	Admission    histogramView            `json:"admission"`
	UpstreamErrs []upstreamErrView        `json:"upstream_errors"`
	CheckInStats map[string]int64         `json:"checkin_stats"`
	Fallback     int64                    `json:"fallback_total"`
}

type upstreamErrView struct {
	Path   string `json:"path"`
	Code   string `json:"code"`
	Status int    `json:"status"`
	Count  int64  `json:"count"`
}

// snapshot 取一次一致性快照（按数量排序，最多 8 条上游错误）。
func (m *metrics) snapshot() metricsSnapshot {
	x := m.extra
	x.mu.Lock()
	byEndpoint := make(map[string]histogramView, len(x.endpointDur))
	for k, h := range x.endpointDur {
		byEndpoint[k] = h.view()
	}
	errs := make([]upstreamErrView, 0, len(x.upstreamErr))
	for k, n := range x.upstreamErr {
		path, code, status := splitErrKey(k)
		errs = append(errs, upstreamErrView{Path: path, Code: code, Status: status, Count: n})
	}
	latency := x.durHist.view()
	checkin := x.checkinDur.view()
	admission := x.admissionDur.view()
	x.mu.Unlock()

	sort.Slice(errs, func(i, j int) bool { return errs[i].Count > errs[j].Count })
	if len(errs) > 8 {
		errs = errs[:8]
	}
	return metricsSnapshot{
		Latency:      latency,
		ByEndpoint:   byEndpoint,
		CheckIn:      checkin,
		Admission:    admission,
		UpstreamErrs: errs,
		CheckInStats: map[string]int64{
			"attempts": m.checkinAttempts.Load(),
			"blocked":  m.checkinBlocked.Load(),
			"timeouts": m.checkinTimeouts.Load(),
		},
		Fallback: m.fallbackTotal.Load(),
	}
}

func splitErrKey(key string) (string, string, int) {
	parts := splitNUL(key)
	if len(parts) < 2 {
		return key, "", 0
	}
	status := 0
	for _, r := range parts[2] {
		if r < '0' || r > '9' {
			status = 0
			break
		}
		status = status*10 + int(r-'0')
	}
	return parts[0], parts[1], status
}

// splitNUL 避免在热路径引入 strings.Split 的额外语义（这里只按 \x00 切）。
func splitNUL(s string) []string {
	out := make([]string, 0, 3)
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == 0 {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	return append(out, s[start:])
}

// quantileMs 返回毫秒分位（0 表示没有样本）。
func quantileMs(view histogramView, q float64) float64 {
	h := &hist{bounds: view.Bounds, counts: view.Counts, sum: view.Sum, total: view.Total}
	return h.quantile(q) * 1000
}

// appendHistogram 输出一个 Prometheus 直方图（含 +Inf 桶与 sum/count）。
//
// labelName 为空表示无标签；否则按 labelName="labelValue" 输出。
// 输出顺序固定（桶升序），便于 diff 与抓取端做稳定性检查。
func appendHistogram(b *strings.Builder, name, help, labelName, labelValue string, v histogramView) {
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

// latencySummary 把直方图收敛成控制台要的三个分位（毫秒）。
func latencySummary(v histogramView) map[string]float64 {
	return map[string]float64{
		"p50_ms": quantileMs(v, 0.50),
		"p95_ms": quantileMs(v, 0.95),
		"p99_ms": quantileMs(v, 0.99),
	}
}
