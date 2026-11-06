// Package server 暴露 OpenAI 兼容的 HTTP 接口。
package server

import (
	"context"
	"log"
	"net/http"
	"sync"
	"time"

	"deepaicode2api/internal/deepai"
	"deepaicode2api/internal/registry"
	"deepaicode2api/internal/store"
)

// Server 是网关 HTTP 服务。
type Server struct {
	Store *store.Store
	Up    *deepai.Client
	// Login 保存进行中的浏览器登录会话（内存态）。
	Login *loginSessions
	// Registers 保存批量注册任务（内存态）。
	Registers *registerJobs

	refreshMu    sync.Mutex
	proxyMu      sync.Mutex
	proxyClients map[string]*deepai.Client

	// inflight 限制单账号同时进行的上游请求数。
	// 调度只保证「这一轮选到谁」，不保证不会把并发全压在少数账号上：
	// 压测时 12 并发可能全落在同一两个号上，放大上游限流后的长尾。
	inflightMu sync.Mutex
	inflight   map[string]int

	// quotaReprobeDelays 是入池后 0 额度的复探退避序列（测试可注入短值）。
	quotaReprobeDelays []time.Duration
}

// New 构造服务。
func New(st *store.Store) *Server {
	return &Server{
		Store: st, Up: deepai.NewClient(), Login: newLoginSessions(),
		Registers: newRegisterJobs(), proxyClients: map[string]*deepai.Client{},
		inflight:           map[string]int{},
		quotaReprobeDelays: quotaReprobeDelays,
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
	mux.HandleFunc("/admin/api/state", s.withAdminAuth(s.handleAdminState))
	mux.HandleFunc("/admin/api/overview", s.withAdminAuth(s.handleAdminOverview))
	mux.HandleFunc("/admin/api/accounts", s.withAdminAuth(s.handleAdminAccounts))
	mux.HandleFunc("/admin/api/keys", s.withAdminAuth(s.handleAdminKeys))
	mux.HandleFunc("/admin/api/logs", s.withAdminAuth(s.handleAdminLogs))
	mux.HandleFunc("/admin/api/stats", s.withAdminAuth(s.handleAdminStats))
	mux.HandleFunc("/admin/api/settings", s.withAdminAuth(s.handleAdminSettings))
	mux.HandleFunc("/admin/api/checkin", s.withAdminAuth(s.handleAdminCheckin))
	mux.HandleFunc("/admin/api/models", s.withAdminAuth(s.handleAdminModels))
	mux.HandleFunc("/admin/api/login/start", s.withAdminAuth(s.handleLoginStart))
	mux.HandleFunc("/admin/api/login/poll", s.withAdminAuth(s.handleLoginPoll))
	mux.HandleFunc("/admin/api/register", s.withAdminAuth(s.handleAdminRegister))
	// 浏览器登录跳转：session id 是一次性随机数，服务端只跳到固定的上游登录页。
	mux.HandleFunc("/login/open", s.handleLoginOpen)

	mux.HandleFunc("/", s.handleRoot)

	return s.withCORS(mux)
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
	accts := s.Store.Accounts()
	ok := 0
	for _, a := range accts {
		// 尚未探测过的账号按“可用”计，避免刚导入时健康度显示为 0。
		if a.Enabled && a.Status != "error" {
			ok++
		}
	}
	models := s.catalog()
	writeJSON(w, http.StatusOK, map[string]any{
		"status":        "ok",
		"accounts":      len(accts),
		"ready":         ok,
		"models":        len(models),
		"default_model": s.Store.DefaultModel(),
		"machine_id":    shortMachine(s.Store.MachineID()),
		"machine_ready": s.Store.MachineID() != "",
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
