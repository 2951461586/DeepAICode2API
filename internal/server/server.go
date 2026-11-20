// Package server 暴露 OpenAI 兼容的 HTTP 接口。
package server

import (
	"context"
	"errors"
	"log"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"deepaicode2api/internal/deepai"
	"deepaicode2api/internal/guard"
	"deepaicode2api/internal/httpx"
	"deepaicode2api/internal/observe"
	"deepaicode2api/internal/poolsvc"
	"deepaicode2api/internal/registry"
	"deepaicode2api/internal/respstate"
	"deepaicode2api/internal/store"
)

// Server 是网关 HTTP 服务。
type Server struct {
	Store *store.Store
	Up    *deepai.Client
	// Login 保存进行中的浏览器登录会话（内存态）。
	Login *loginSessions

	// Pool 是号池的运行时状态（在途槽位 / 探测去重 / 会话粘性 / 入池代理）。
	// 并发原语全部在 internal/poolsvc；server 只注入两件它不该知道的事：
	// 按出口的客户端工厂（含代理缓存）与落盘入口。
	Pool *poolsvc.Service

	refreshMu    sync.Mutex
	proxyMu      sync.Mutex
	proxyClients map[string]*deepai.Client

	// laneMu / laneCheckedAt 是车道探测的节流：426 会成批出现，
	// 没有冷却就会把一次车道升级变成一轮探测风暴。
	laneMu        sync.Mutex
	laneCheckedAt time.Time

	// upstreamLane 是 /theos 回报的上游车道（0 = 尚未观测）。
	//
	// 2026-11-19 起上游对 ≤7 的车道全部放行，所以「车道探测命中」再也不能
	// 说明当前车道是哪一个，也看不出上游是否已经抬到 8。/theos 的
	// lane_version 是唯一信号，记下来给控制台 / 指标做升级提示。
	upstreamLane atomic.Int64

	// admissionWait 是「槽位全满」时的排队上限（测试可注入 0 立即 429）。
	admissionWait time.Duration

	// limiter 是按密钥的令牌桶 + 并发闸门（数据面限流）。
	limiter *guard.Limiter

	// adminGuard 是管理口令的失败退避（防爆破）；adminGuardBase 可被测试注入。
	adminGuard     *guard.AdminGuard
	adminGuardBase time.Duration

	// metrics 是 /metrics 导出的进程内计数（原子量，无需锁）。
	metrics *observe.Metrics

	// Version 是构建版本（main 用 -ldflags 注入）；空值对外表现为 dev。
	Version string

	// versionMu / versionCache / versionAt 缓存「最新 release」的查询结果，
	// 避免每次打开控制台都去打 GitHub（未认证请求有速率限制）。
	// versionInFlight 非 nil 表示已有一个 goroutine 在查（并发请求等它的结果）。
	versionMu       sync.Mutex
	versionCache    *versionInfo
	versionAt       time.Time
	versionInFlight chan struct{}

	// shapeNeed 记忆「哪些调用方需要桌面端形状」（见 shape.go）。
	shapeMu   sync.Mutex
	shapeNeed map[string]time.Time

	// history 是 /v1/responses 的伺服端對話歷史暫存（見 internal/respstate）。
	// 讓 previous_response_id 在無狀態合成路徑上也能續接，而非回 400 造成失憶。
	history *respstate.Store
}

// New 构造服务。
func New(st *store.Store) *Server {
	s := &Server{
		Store: st, Up: deepai.NewClient(), Login: newLoginSessions(),
		proxyClients:  map[string]*deepai.Client{},
		admissionWait: admissionWait,
		limiter:       guard.NewLimiter(),
		adminGuard:    guard.NewAdminGuard(),
		metrics:       observe.NewMetrics(),
		history:       respstate.NewFromEnv(),
	}
	// 运行期可调项（车道 / 签到预算）走**拉取式**：客户端每次请求现取，
	// 不再依赖 Server 主动推给「直连客户端 + 每个代理客户端」。
	// 上次探测到的车道由 CurrentLane 从设置里读（跨重启保留）。
	s.Up.Runtime = s.runtimeConfig
	// 上游车道漂移的观测钩子：所有 /theos 成功都会回传 lane_version。
	s.Up.LaneHook = s.noteUpstreamLane
	s.Pool = poolsvc.New(poolsvc.Config{
		Store:     st,
		ClientFor: s.clientFor,
		Save:      st.Save,
	})
	// 会话粘性绑定随重启保留（旁路文件，丢失不影响正确性）。
	s.Pool.LoadSticky()
	return s
}

// runtimeConfig 返回上游客户端的运行期配置（拉取式真源）。
//
// 车道与签到预算都能在控制台运行期调整。此前是「推」模式：改设置时要遍历
// 已缓存客户端逐个 Set，新建客户端时还要再补一遍（server.go / request.go
// 两处重复注入），漏一处的表现是「走代理的号与直连号行为不一致」。
// 现在客户端每次请求现取一次，只有这一份真源。
func (s *Server) runtimeConfig() deepai.RuntimeConfig {
	return deepai.RuntimeConfig{
		Lane:          s.CurrentLane(),
		CheckInBudget: time.Duration(s.Store.Settings().CheckinBlockMS) * time.Millisecond,
	}
}

// Handler 返回完整路由。
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	// 探活
	mux.HandleFunc("/ping", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("pong"))
	})
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/healthz", s.handleHealth)

	// 推理（OpenAI 兼容）
	mux.HandleFunc("/v1/models", s.withAuth(s.handleModels))
	mux.HandleFunc("/v1/chat/completions", s.withAuth(s.handleChatCompletions))
	mux.HandleFunc("/v1/responses", s.withAuth(s.handleResponses))

	// Anthropic 兼容
	mux.HandleFunc("/v1/messages", s.withAuth(s.handleAnthropicMessages))
	mux.HandleFunc("/v1/messages/count_tokens", s.withAuth(s.handleAnthropicCountTokens))

	// 音频
	mux.HandleFunc("/v1/audio/transcriptions", s.withAuth(s.handleTranscriptions))
	mux.HandleFunc("/v1/audio/translations", s.withAuth(s.handleTranscriptions))

	// DeepAI 原生能力扩展
	mux.HandleFunc("/v1/deepai/web_search", s.withAuth(s.handleWebSearch))
	mux.HandleFunc("/v1/deepai/find_service", s.withAuth(s.handleFindService))
	mux.HandleFunc("/v1/deepai/theos", s.withAuth(s.handleTheos))
	mux.HandleFunc("/v1/deepai/usage", s.withAuth(s.handleUsage))
	// 兼容简写
	mux.HandleFunc("/v1/web_search", s.withAuth(s.handleWebSearch))
	mux.HandleFunc("/v1/theos", s.withAuth(s.handleTheos))
	mux.HandleFunc("/v1/usage", s.withAuth(s.handleUsage))

	// 控制台 API（统一走管理鉴权 + 同源校验）
	mux.HandleFunc("/admin/api/overview", s.withAdminAuth(s.handleAdminOverview))
	mux.HandleFunc("/admin/api/accounts", s.withAdminAuth(s.handleAdminAccounts))
	mux.HandleFunc("/admin/api/keys", s.withAdminAuth(s.handleAdminKeys))
	mux.HandleFunc("/admin/api/logs", s.withAdminAuth(s.handleAdminLogs))
	mux.HandleFunc("/admin/api/stats", s.withAdminAuth(s.handleAdminStats))
	mux.HandleFunc("/admin/api/settings", s.withAdminAuth(s.handleAdminSettings))
	mux.HandleFunc("/admin/api/checkin", s.withAdminAuth(s.handleAdminCheckin))
	// 版本与更新检查（服务端代查 GitHub，前端 CSP 不允许直连）。
	mux.HandleFunc("/admin/api/version", s.withAdminAuth(s.handleAdminVersion))

	// 监控：Prometheus 文本格式（与 /admin/api/* 同级鉴权）。
	mux.HandleFunc("/metrics", s.withAdminAuth(s.handleMetrics))

	mux.HandleFunc("/admin/api/models", s.withAdminAuth(s.handleAdminModels))
	mux.HandleFunc("/admin/api/login/start", s.withAdminAuth(s.handleLoginStart))
	mux.HandleFunc("/admin/api/login/poll", s.withAdminAuth(s.handleLoginPoll))
	// 粘贴回调 URL：Docker / 远程部署时浏览器回调打不开，用这条入口完成兑换。
	mux.HandleFunc("/admin/api/login/callback", s.withAdminAuth(s.handleLoginCallback))
	// 浏览器登录跳转：session id 是一次性随机数，服务端只跳到固定的上游登录页。
	mux.HandleFunc("/login/open", s.handleLoginOpen)

	// 口令登录页：控制台顶栏不再放口令输入框，未登录时跳到这里。
	mux.HandleFunc("/login", serveLogin)
	// 口令校验入口：只回答“这个口令对不对”，不返回任何敏感信息。
	mux.HandleFunc("/admin/api/auth", s.withAdminAuth(s.handleAdminAuth))

	// 控制台静态资源：样式、标签页图标与 ES module 脚本（页面本身走 /）。
	mux.HandleFunc("/style.css", s.handleConsoleAsset)
	// 浏览器自动请求的固定路径：不给的话每次打开控制台都多一条 404。
	mux.HandleFunc("/favicon.ico", s.handleConsoleAsset)
	mux.HandleFunc("/apple-touch-icon.png", s.handleConsoleAsset)
	mux.HandleFunc("/js/", s.handleConsoleAsset)
	mux.HandleFunc("/", s.handleRoot)

	// 顺序（由外到内）：设置快照 → CORS → 观测 → 压缩 → 路由。
	// 设置快照必须最外层：CORS 来源校验、鉴权、限流、handler 与上游调用
	// 都必须看到同一份设置（同一请求里不能一半新一半旧）。
	// 压缩在最内层：外层（观测）看到的是压缩后的字节数。
	return s.withSettingsSnapshot(s.withCORS(observe.Middleware(s.metrics, httpx.WithGzip(mux))))
}

// RefreshModels 从上游拉取模型目录并落盘。
func (s *Server) RefreshModels(ctx context.Context) ([]deepai.Model, error) {
	s.refreshMu.Lock()
	defer s.refreshMu.Unlock()

	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	// 2026-11-14 起上游 /models 要求 Api-Key：没有账号时提前给出可行动的
	// 错误，而不是把上游的 401 原样抛给控制台（那看起来像「Key 失效」）。
	acct := s.probeAccount()
	if acct == nil {
		return nil, errors.New("上游 /models 需要 Api-Key：请先导入或添加一个账号再刷新目录")
	}
	models, def, err := s.Up.FetchModels(ctx, acct)
	if err != nil {
		return nil, err
	}
	s.Store.SetModels(models, def)
	if err := s.Store.Save(); err != nil {
		log.Printf("保存模型目录失败: %v", err)
	}
	return models, nil
}

// --- 基础端点 ---

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	settings := s.settings(r)
	// 口径见 deepai.AccountReady / AccountDegraded 与额度时效；
	// 与 /metrics 共用 poolSnapshot，避免两处给出互相矛盾的数。
	pool := s.poolSnapshot()
	models := s.catalog()
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"status":        "ok",
		"version":       s.currentVersion(),
		"accounts":      pool.Total,
		"ready":         pool.Ready,
		"degraded":      pool.Degraded,
		"blocked":       pool.Blocked,
		"quota_fresh":   pool.QuotaFresh,
		"quota_due":     pool.QuotaDue,
		"lane":          s.CurrentLane(),
		"lane_compiled": deepai.LaneVersion,
		"lane_upstream": s.UpstreamLane(),
		"lane_drift":    s.LaneDrift(),
		"models":        len(models),
		"default_model": s.Store.DefaultModel(),
		"strict_model":  settings.StrictModel,
		"machine_id":    shortMachine(s.Store.MachineID()),
		"machine_ready": s.Store.MachineID() != "",
		// 控制台据此决定是否把用户送到 /login：未设口令时本机直接放行。
		"admin_required": settings.AdminPassword != "",
	})
}

func (s *Server) handleModels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httpx.WriteErr(w, http.StatusMethodNotAllowed, "仅支持 GET", "invalid_request_error")
		return
	}
	catalog := s.catalog()
	settings := s.settings(r)
	models := registry.List(catalog, settings.ModelAliases)
	data := make([]map[string]any, 0, len(models))
	now := time.Now().Unix()
	for _, m := range models {
		data = append(data, map[string]any{
			"id":       m.ID,
			"object":   "model",
			"created":  now,
			"owned_by": "deepai",
			"deepai": map[string]any{
				"name":              m.Name,
				"reasoning":         m.Reasoning,
				"input":             m.Input,
				"context_window":    m.ContextWindow,
				"max_tokens":        m.MaxTokens,
				"premier":           m.Premier,
				"free":              m.Free(),
				"api":               m.API,
				"theos_per_request": m.TheosPerRequest,
				"theos_per_second":  m.TheosPerSecond,
			},
		})
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"object": "list", "data": data})
}

// handleRoot 返回控制台单页；其余未知路径返回 404。
func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		httpx.WriteErr(w, http.StatusNotFound, "未找到该路径", "invalid_request_error")
		return
	}
	serveConsole(w, r)
}
