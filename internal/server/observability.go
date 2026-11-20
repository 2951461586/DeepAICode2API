package server

import (
	"net/http"
	"time"

	"deepaicode2api/internal/deepai"
	"deepaicode2api/internal/httpx"
	"deepaicode2api/internal/observe"
	"deepaicode2api/internal/poolsvc"
)

// --- 可观测性的装配 ---
//
// 指标状态机、request id / 访问日志中间件与 Prometheus 文本格式都在
// internal/observe（不依赖 store / 号池 / 协议层）。这里只补两件它不该知道的事：
//
//  1. 号池健康度快照：需要 store + 额度策略，/health 与 /metrics 共用口径；
//  2. 与请求计数无关的「业务数字」（日志条数 / 密钥数 / 车道），经 PromExtra 传入。

// poolSnapshot 计算号池健康度。
//
// 口径与 /health 完全一致（AccountReady / AccountDegraded / 额度时效），
// 抽出来是为了避免控制台徽章与 Prometheus 指标给出互相矛盾的数。
func (s *Server) poolSnapshot() observe.PoolSnapshot {
	var out observe.PoolSnapshot
	now := time.Now()
	for _, a := range s.Store.Accounts() {
		out.Total++
		if deepai.AccountReady(a, now) {
			out.Ready++
		}
		if deepai.AccountDegraded(a, now) {
			out.Degraded++
		}
		if a.Enabled && a.Status == deepai.StatusBlocked {
			out.Blocked++
		}
		if a.Theos != nil && a.Theos.CheckedAt > 0 &&
			now.Sub(time.Unix(a.Theos.CheckedAt, 0)) < quotaStaleAfter {
			out.QuotaFresh++
		}
		if poolsvc.NeedsRefresh(a, now, poolsvc.GuardStaleAfter) {
			out.QuotaDue++
		}
	}
	return out
}

// handleMetrics 输出 Prometheus 文本格式指标。
//
// 挂 withAdminAuth：里面含号池规模与请求量，和 /admin/api/* 同级。
func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httpx.WriteErr(w, http.StatusMethodNotAllowed, "仅支持 GET", "invalid_request_error")
		return
	}
	stats := s.Store.Stats()
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	s.metrics.WritePrometheus(w, observe.PromExtra{
		Pool:         s.poolSnapshot(),
		LogRequests:  stats.TotalRequests,
		LogFailed:    stats.FailedRequests,
		Keys:         len(s.Store.Keys()),
		Lane:         atoiOr(s.CurrentLane(), 0),
		CompiledLane: atoiOr(deepai.LaneVersion, 0),
		UpstreamLane: int64(s.UpstreamLane()),
	})
}
