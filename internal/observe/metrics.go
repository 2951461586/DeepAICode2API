// Package observe 是可观测性层：进程内指标（计数器 + 定桶直方图）、
// /metrics 的 Prometheus 文本输出，以及 request id / 访问日志中间件。
//
// 边界：本包只依赖标准库、internal/logx（一行 JSON 日志）与 internal/guard
// （取来源 IP）。它不认识 store / 号池 / 协议层——/metrics 里那些「号池与车道」
// 的数字由调用方通过 PromExtra 传进来。因此可以脱离网关单测与压测。
package observe

import (
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

// 本文件是指标的**扩展**层（/metrics 与 /admin/api/overview 共用）。
//
// 设计取舍：
//   - 零依赖：自己维护定桶直方图，不引入 Prometheus 客户端；
//   - 低基数：端点只分 chat / responses / messages / other，上游错误按
//     (path, code, status) 聚合并限制条目数，避免被异常流量打爆内存；
//   - 与旧指标共存：原来的 requests/2xx..5xx/durSum 保留，控制台不破。

// defaultBuckets 是延迟桶（秒）。覆盖免费模型的百毫秒级到深度思考的分钟级。
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

// HistogramView 是直方图的 JSON / Prometheus 视图。
type HistogramView struct {
	Bounds []float64 `json:"bounds"`
	Counts []uint64  `json:"counts"`
	Sum    float64   `json:"sum"`
	Total  uint64    `json:"total"`
}

func (h *hist) view() HistogramView {
	if h == nil {
		return HistogramView{}
	}
	bounds := append([]float64(nil), h.bounds...)
	counts := append([]uint64(nil), h.counts...)
	return HistogramView{Bounds: bounds, Counts: counts, Sum: h.sum, Total: h.total}
}

// EndpointLabel 把 URL 路径收敛成低基数标签。
func EndpointLabel(path string) string {
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

// Metrics 是进程内的轻量指标计数（不持久化、重启清零）。
type Metrics struct {
	requests atomic.Int64
	c2xx     atomic.Int64
	c3xx     atomic.Int64
	c4xx     atomic.Int64
	c5xx     atomic.Int64
	durSumNS atomic.Int64
	durCount atomic.Int64
	inflight atomic.Int64

	// 扩展指标：签到 / 上游错误 / 兜底 / 分端点延迟。
	checkinAttempts atomic.Int64
	checkinBlocked  atomic.Int64
	checkinTimeouts atomic.Int64
	fallbackTotal   atomic.Int64

	// stateful* 统计依赖上游响应状态的请求（previous_response_id /
	// conversation / item_reference）。rejected 只统计网关拒绝的那部分。
	statefulPrevID  atomic.Int64
	statefulConv    atomic.Int64
	statefulItemRef atomic.Int64
	statefulReject  atomic.Int64

	extra *extraMetrics
}

// NewMetrics 返回一套归零的指标。
func NewMetrics() *Metrics { return &Metrics{extra: newExtraMetrics()} }

// Observe 记录一次请求（status + 耗时 + 端点分布）。
func (m *Metrics) Observe(endpoint string, status int, d time.Duration) {
	m.ObserveBasic(status, d)
	m.ObserveDuration(endpoint, d.Seconds())
}

// ObserveBasic 只记状态码与总耗时，不进分端点 / 分位直方图。
//
// /admin/* 用它：控制台自己的请求如果也进延迟直方图，/admin/api/overview
// 的响应体就会因为「上一次 overview 的耗时」而每次变化，ETag 永远命中不了
// （TestOverviewETag 就是这么暴露的）。管理接口不是延迟 SLI。
func (m *Metrics) ObserveBasic(status int, d time.Duration) {
	m.requests.Add(1)
	switch {
	case status >= 500:
		m.c5xx.Add(1)
	case status >= 400:
		m.c4xx.Add(1)
	case status >= 300:
		m.c3xx.Add(1)
	default:
		m.c2xx.Add(1)
	}
	m.durSumNS.Add(int64(d))
	m.durCount.Add(1)
}

// ObserveDuration 记录一次请求耗时（秒）。
func (m *Metrics) ObserveDuration(endpoint string, sec float64) {
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

// ObserveUpstreamError 记录一次上游拒绝（含错误码）。
func (m *Metrics) ObserveUpstreamError(path, code string, status int) {
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

// ObserveCheckIn 记录一次推理前的签到尝试。
//
// 参数都是原始值而不是 deepai 的结果类型：本包不依赖协议层。
// slowAfter 是「签到阻塞推理」的告警阈值（由调用方的策略决定）。
func (m *Metrics) ObserveCheckIn(attempted bool, waited, slowAfter time.Duration, timedOut bool) {
	if !attempted {
		return
	}
	m.checkinAttempts.Add(1)
	if slowAfter > 0 && waited >= slowAfter {
		m.checkinBlocked.Add(1)
	}
	if timedOut {
		m.checkinTimeouts.Add(1)
	}
	sec := waited.Seconds()
	x := m.extra
	x.mu.Lock()
	x.checkinDur.observe(sec)
	x.mu.Unlock()
}

// ObserveAdmissionWait 记录一次「账号全忙」的排队等待（秒）。
func (m *Metrics) ObserveAdmissionWait(d time.Duration) {
	if d <= 0 {
		return
	}
	x := m.extra
	x.mu.Lock()
	x.admissionDur.observe(d.Seconds())
	x.mu.Unlock()
}

// AddFallback 记录一次「请求体形状被拒 → 补形状重试」。
func (m *Metrics) AddFallback() { m.fallbackTotal.Add(1) }

// AddStateful 记录一次依赖上游响应状态的请求。
//
// rejected=true 表示网关没有转发，而是回 400：合成路径（chat/completions）
// 只有 input 里的内容，静默忽略续接字段就等于把上下文截断。
func (m *Metrics) AddStateful(param string, rejected bool) {
	switch param {
	case "previous_response_id":
		m.statefulPrevID.Add(1)
	case "conversation":
		m.statefulConv.Add(1)
	case "item_reference":
		m.statefulItemRef.Add(1)
	}
	if rejected {
		m.statefulReject.Add(1)
	}
}

// MetricsSnapshot 是给控制台概览的聚合视图。
type MetricsSnapshot struct {
	Latency      HistogramView            `json:"latency"`
	ByEndpoint   map[string]HistogramView `json:"by_endpoint"`
	CheckIn      HistogramView            `json:"checkin"`
	Admission    HistogramView            `json:"admission"`
	UpstreamErrs []UpstreamErrView        `json:"upstream_errors"`
	CheckInStats map[string]int64         `json:"checkin_stats"`
	Stateful     map[string]int64         `json:"stateful"`
	Fallback     int64                    `json:"fallback_total"`
}

// UpstreamErrView 是一条聚合后的上游错误。
type UpstreamErrView struct {
	Path   string `json:"path"`
	Code   string `json:"code"`
	Status int    `json:"status"`
	Count  int64  `json:"count"`
}

// Snapshot 取一次一致性快照（按数量排序，最多 8 条上游错误）。
func (m *Metrics) Snapshot() MetricsSnapshot {
	x := m.extra
	x.mu.Lock()
	byEndpoint := make(map[string]HistogramView, len(x.endpointDur))
	for k, h := range x.endpointDur {
		byEndpoint[k] = h.view()
	}
	errs := make([]UpstreamErrView, 0, len(x.upstreamErr))
	for k, n := range x.upstreamErr {
		path, code, status := splitErrKey(k)
		errs = append(errs, UpstreamErrView{Path: path, Code: code, Status: status, Count: n})
	}
	latency := x.durHist.view()
	checkin := x.checkinDur.view()
	admission := x.admissionDur.view()
	x.mu.Unlock()

	sort.Slice(errs, func(i, j int) bool { return errs[i].Count > errs[j].Count })
	if len(errs) > 8 {
		errs = errs[:8]
	}
	return MetricsSnapshot{
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
		Stateful: map[string]int64{
			"previous_response_id": m.statefulPrevID.Load(),
			"conversation":         m.statefulConv.Load(),
			"item_reference":       m.statefulItemRef.Load(),
			"rejected":             m.statefulReject.Load(),
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

// QuantileMs 返回毫秒分位（0 表示没有样本）。
func QuantileMs(view HistogramView, q float64) float64 {
	h := &hist{bounds: view.Bounds, counts: view.Counts, sum: view.Sum, total: view.Total}
	return h.quantile(q) * 1000
}

// LatencySummary 把直方图收敛成控制台要的三个分位（毫秒）。
func LatencySummary(v HistogramView) map[string]float64 {
	return map[string]float64{
		"p50_ms": QuantileMs(v, 0.50),
		"p95_ms": QuantileMs(v, 0.95),
		"p99_ms": QuantileMs(v, 0.99),
	}
}
