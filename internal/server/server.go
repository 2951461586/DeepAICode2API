// Package server 暴露 OpenAI 兼容的 HTTP 接口。
package server

import (
	"context"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"deepaicode2api/internal/deepai"
	"deepaicode2api/internal/proxy"
	"deepaicode2api/internal/registry"
	"deepaicode2api/internal/store"
)

// Server 是网关 HTTP 服务。
type Server struct {
	Store *store.Store
	Up    *deepai.Client
	// Login 保存进行中的浏览器登录会话（内存态）。
	Login *loginSessions

	refreshMu    sync.Mutex
	proxyMu      sync.Mutex
	proxyClients map[string]*deepai.Client

	// enrollPool 是入池自动分配代理用的进程内单例。
	//
	// 浏览器登录一次只入一个号；若每次新建池，轮询游标都会从 0 开始，
	// 所有号就都落在第一个出口上（等于没分配）。单例让连续入池能真正轮转。
	enrollPoolOnce sync.Once
	enrollPool     *proxy.ProxyPool

	// inflight 限制单账号同时进行的上游请求数。
	// 调度只保证「这一轮选到谁」，不保证不会把并发全压在少数账号上：
	// 压测时 12 并发可能全落在同一两个号上，放大上游限流后的长尾。
	inflightMu sync.Mutex
	inflight   map[string]int

	// quotaReprobeDelays 是入池后 0 额度的复探退避序列（测试可注入短值）。
	quotaReprobeDelays []time.Duration

	// laneMu / laneCheckedAt 是车道探测的节流：426 会成批出现，
	// 没有冷却就会把一次车道升级变成一轮探测风暴。
	laneMu        sync.Mutex
	laneCheckedAt time.Time

	// admissionWait 是「槽位全满」时的排队上限（测试可注入 0 立即 429）。
	admissionWait time.Duration

	// quotaWG 跟踪已排期的后台落盘工作（配额复探 / 入池探活 / 按需复探）。
	//
	// 这些 goroutine 在写完内存后还要 Save() 落盘；测试必须等它完全收尾
	// 再结束，否则 t.TempDir() 的清理会和落盘撞上（CI 上表现为
	// "TempDir RemoveAll cleanup: directory not empty"）。
	// 进程退出时由 Close 统一等待，避免「最后一次 Save 之后又有后台写」。
	quotaWG sync.WaitGroup

	// stopCh 在 Close 时关闭，用于打断后台任务里的长睡眠（配额复探退避最长 12 分钟）。
	stopCh   chan struct{}
	stopOnce sync.Once

	// probing 是配额探测的在途去重表（按账号 ID）。
	//
	// 三条路径会同时探同一批账号：手动 checkin、入池后延迟复探、
	// 推理时的按需复探。没有它时同一个号会被连打多次，既浪费额度
	// 也容易触发上游风控。
	probeMu sync.Mutex
	probing map[string]struct{}

	// probeSem 是全局在途探测上限（手动批次 / 后台守护 / 按需复探共用）。
	//
	// 653 个号的额度快照会同时变旧；若按需复探不加限制，一波流量就能
	// 叠出几十上百个并行的 /theos 请求——那是最容易被上游风控的形态。
	probeSem chan struct{}

	// limiter 是按密钥的令牌桶 + 并发闸门（数据面限流）。
	limiter *keyRateLimiter

	// adminGuard 是管理口令的失败退避（防爆破）；adminGuardBase 可被测试注入。
	adminGuard     *adminGuard
	adminGuardBase time.Duration

	// metrics 是 /metrics 导出的进程内计数（原子量，无需锁）。
	metrics *metrics

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
}

// New 构造服务。
func New(st *store.Store) *Server {
	s := &Server{
		Store: st, Up: deepai.NewClient(), Login: newLoginSessions(),
		proxyClients:       map[string]*deepai.Client{},
		inflight:           map[string]int{},
		probing:            map[string]struct{}{},
		probeSem:           make(chan struct{}, probeWorkers),
		quotaReprobeDelays: quotaReprobeDelays,
		admissionWait:      admissionWait,
		stopCh:             make(chan struct{}),
		limiter:            newKeyRateLimiter(),
		adminGuard:         newAdminGuard(),
		metrics:            newMetrics(),
	}
	// 上次探测到的车道优先（跨重启保留），但较新的编译期默认值优先于更旧的探测值：
	// 否则升级到对齐新客户端的版本后，还会用旧车道跑到下一轮探测。
	if lane := strings.TrimSpace(st.Settings().Lane); lane != "" {
		s.Up.SetLane(newerLane(lane))
	}
	s.applyRuntimeSettings()
	return s
}

// applyRuntimeSettings 把「可运行期调整」的设置同步到已缓存的客户端。
//
// 车道走 SetLane；签到预算走 SetCheckInBudget。两者都必须同时覆盖
// s.Up 和按代理缓存的客户端，否则直连号与代理号表现不一致。
func (s *Server) applyRuntimeSettings() {
	budget := time.Duration(s.Store.Settings().CheckinBlockMS) * time.Millisecond
	s.Up.SetCheckInBudget(budget)
	s.proxyMu.Lock()
	for _, c := range s.proxyClients {
		c.SetCheckInBudget(budget)
	}
	s.proxyMu.Unlock()
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
	mux.HandleFunc("/admin/api/state", s.withAdminAuth(s.handleAdminState))
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

	// 压缩在最内层：外层（观测）看到的是压缩后的字节数。
	return s.withCORS(s.withObservability(withGzip(mux)))
}

// RefreshModels 从上游拉取模型目录并落盘。
func (s *Server) RefreshModels(ctx context.Context) ([]deepai.Model, error) {
	s.refreshMu.Lock()
	defer s.refreshMu.Unlock()

	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	models, def, err := s.Up.FetchModels(ctx)
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
	// 口径见 deepai.AccountReady / AccountDegraded 与额度时效；
	// 与 /metrics 共用 poolSnapshot，避免两处给出互相矛盾的数。
	pool := s.poolSnapshot()
	models := s.catalog()
	writeJSON(w, http.StatusOK, map[string]any{
		"status":        "ok",
		"version":       s.currentVersion(),
		"accounts":      pool.Total,
		"ready":         pool.Ready,
		"degraded":      pool.Degraded,
		"blocked":       pool.Blocked,
		"quota_fresh":   pool.QuotaFresh,
		"quota_due":     pool.QuotaDue,
		"models":        len(models),
		"default_model": s.Store.DefaultModel(),
		"strict_model":  s.Store.Settings().StrictModel,
		"machine_id":    shortMachine(s.Store.MachineID()),
		"machine_ready": s.Store.MachineID() != "",
		// 控制台据此决定是否把用户送到 /login：未设口令时本机直接放行。
		"admin_required": s.Store.Settings().AdminPassword != "",
	})
}

func (s *Server) handleModels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErr(w, http.StatusMethodNotAllowed, "仅支持 GET", "invalid_request_error")
		return
	}
	catalog := s.catalog()
	models := registry.List(catalog, s.Store.Settings().ModelAliases)
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
	writeJSON(w, http.StatusOK, map[string]any{"object": "list", "data": data})
}

// handleRoot 返回控制台单页；其余未知路径返回 404。
func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		writeErr(w, http.StatusNotFound, "未找到该路径", "invalid_request_error")
		return
	}
	serveConsole(w, r)
}
