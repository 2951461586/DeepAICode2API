package server

import (
	"encoding/json"
	"net/http"
	"strings"

	"deepaicode2api/internal/httpx"
	"deepaicode2api/internal/logx"
	"deepaicode2api/internal/registry"
	"deepaicode2api/internal/store"
)

// --- 控制台管理接口 ---

// sanitizedSettings 返回可下发给前端的设置（管理密码不外泄）。
func sanitizedSettings(st store.Settings) map[string]any {
	return map[string]any{
		"model_aliases":        st.ModelAliases,
		"request_timeout_sec":  st.RequestTimeout,
		"max_retries":          st.MaxRetries,
		"log_keep":             st.LogKeep,
		"cors_origin":          st.CORSOrigin,
		"fallback_model":       st.FallbackModel,
		"strict_model":         st.StrictModel,
		"lane":                 st.Lane,
		"rate_limit_rps":       st.RateLimitRPS,
		"rate_limit_burst":     st.RateLimitBurst,
		"key_concurrency":      st.KeyConcurrency,
		"checkin_block_ms":     st.CheckinBlockMS,
		"preset_desktop_shape": st.PresetDesktopShape,
		"admin_password_set":   st.AdminPassword != "",
	}
}

// --- 控制台 API ---

// stateSunset 是旧全量端点的下线日期（RFC 8594 的 Sunset 头）。
//
// 控制台早已改用 /admin/api/overview + /admin/api/accounts（分页），
// 保留这个端点只是为了给外部脚本留迁移窗口：响应里带 Deprecation /
// Sunset / Link(successor-version) 三个头，并在日志里留一行痕迹，
// 这样「谁还在用」可以从日志直接查，而不是靠猜。
const stateSunset = "Wed, 20 May 2026 00:00:00 GMT"

func (s *Server) handleAdminState(w http.ResponseWriter, r *http.Request) {
	// Deprecated: 见 stateSunset。新代码请用分页端点。
	h := w.Header()
	h.Set("Deprecation", "true")
	h.Set("Sunset", stateSunset)
	h.Add("Link", `</admin/api/overview>; rel="successor-version"`)
	logx.Warn("调用了已弃用的 /admin/api/state（请改用 overview + accounts 分页）",
		"remote", r.RemoteAddr, "ua", r.UserAgent())

	// 兼容端点：默认仍下发全量账号（外部脚本可能在用）。
	// 需要小响应时加 ?accounts=0。
	var safe []map[string]any
	if r.URL.Query().Get("accounts") != "0" {
		accts := s.Store.Accounts()
		safe = make([]map[string]any, 0, len(accts))
		for _, a := range accts {
			safe = append(safe, accountView(a))
		}
	}
	safeKeys := safeKeyList(s.Store.Keys())
	catalog := s.catalog()
	settings := s.settings(r)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"accounts":       safe,
		"keys":           safeKeys,
		"models":         registry.List(catalog, settings.ModelAliases),
		"default_model":  s.Store.DefaultModel(),
		"models_updated": s.Store.ModelsUpdated(),
		"machine_id":     shortMachine(s.Store.MachineID()),
		"settings":       sanitizedSettings(settings),
		"stats":          s.Store.Stats(),
		"upstream":       s.Up.Origin,
	})
}

func (s *Server) handleAdminKeys(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"keys": safeKeyList(s.Store.Keys())})
	case http.MethodPost:
		var body struct {
			Action string `json:"action"`
			Name   string `json:"name"`
			Group  string `json:"group"`
			Key    string `json:"key"`
		}
		// 畸形 JSON 直接报错；空体仍然合法（等价于「新建一把默认密钥」）。
		if !httpx.DecodeOptionalJSON(w, r, &body) {
			return
		}
		// action=set-group：只改绑定分组（保持密钥值不变）；其余 / 空值仍为新建。
		if body.Action == "set-group" {
			key := s.resolveKeyRef(strings.TrimSpace(body.Key))
			if key == "" {
				httpx.WriteErr(w, http.StatusBadRequest, "缺少 key", "invalid_request_error")
				return
			}
			ok := s.Store.SetKeyGroup(key, body.Group)
			if !s.persistOrFail(w) {
				return
			}
			httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": ok, "group": s.Store.KeyGroup(key)})
			return
		}
		k := s.Store.AddKey(body.Name, body.Group)
		if !s.persistOrFail(w) {
			return
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true, "key": k.Key})
	case http.MethodDelete:
		q := r.URL.Query()
		delRef := strings.TrimSpace(q.Get("key"))
		togRef := strings.TrimSpace(q.Get("toggle"))
		if delRef == "" && togRef == "" {
			httpx.WriteErr(w, http.StatusBadRequest, "缺少 key 或 toggle 参数", "invalid_request_error")
			return
		}
		// 先校验再变更：resolveKeyRef 找不到时会原样返回，所以存在性得自己查。
		// 否则「删不存在的 Key」会回 200 {"ok":true} 还落一次盘，控制台看到成功、
		// 列表却没变（典型场景：另一个标签页已经删过这把 Key）。
		// 两个 ref 一起校验也是为此：不能「删成功了、切失败了」半途而废后回 404。
		delKey, togKey := s.resolveKeyRef(delRef), s.resolveKeyRef(togRef)
		if delRef != "" && !s.keyExists(delKey) {
			httpx.WriteErr(w, http.StatusNotFound, "密钥不存在（可能已被删除或吊销）", "not_found_error")
			return
		}
		if togRef != "" && !s.keyExists(togKey) {
			httpx.WriteErr(w, http.StatusNotFound, "密钥不存在（可能已被删除或吊销）", "not_found_error")
			return
		}
		deleted, toggled := false, false
		if delRef != "" {
			deleted = s.Store.DeleteKey(delKey)
		}
		if togRef != "" {
			toggled = s.Store.ToggleKey(togKey)
		}
		if !s.persistOrFail(w) {
			return
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true, "deleted": deleted, "toggled": toggled})
	default:
		httpx.WriteErr(w, http.StatusMethodNotAllowed, "不支持的方法", "invalid_request_error")
	}
}

func (s *Server) handleAdminLogs(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodDelete:
		s.Store.ClearLogs()
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
		return
	case http.MethodGet:
	default:
		// 以前只拦 DELETE，POST / PUT 会把日志当查询结果回包，容易被当成“写成功”。
		httpx.WriteErr(w, http.StatusMethodNotAllowed, "仅支持 GET / DELETE", "invalid_request_error")
		return
	}
	// 支持 limit / since / endpoint / account / min_status / q。
	// 无参数时行为与旧版一致（最近 limit 条，默认 200）。
	f := logFilterFromQuery(r)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"logs":  s.Store.QueryLogs(f),
		"limit": f.Limit,
		"filter": map[string]any{
			"since": f.Since, "endpoint": f.Endpoint, "account": f.Account,
			"min_status": f.MinStatus, "q": f.Keyword,
		},
	})
}

func (s *Server) handleAdminStats(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodDelete {
		s.Store.ClearStats()
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
		return
	}
	httpx.WriteJSON(w, http.StatusOK, s.Store.Stats())
}

// settingsPatch 是控制台提交的设置增量。
//
// 全部字段用指针：nil 表示“本次不改”，与“显式设成 0 / 空串”区分开。
// 这一点对限流参数尤其重要：0 是“关闭限流”的合法值。
type settingsPatch struct {
	ModelAliases       map[string]string `json:"model_aliases"`
	RequestTimeout     *int              `json:"request_timeout_sec"`
	MaxRetries         *int              `json:"max_retries"`
	LogKeep            *int              `json:"log_keep"`
	CORSOrigin         *string           `json:"cors_origin"`
	AdminPassword      *string           `json:"admin_password"`
	FallbackModel      *string           `json:"fallback_model"`
	StrictModel        *bool             `json:"strict_model"`
	RateLimitRPS       *float64          `json:"rate_limit_rps"`
	RateLimitBurst     *int              `json:"rate_limit_burst"`
	KeyConcurrency     *int              `json:"key_concurrency"`
	CheckinBlockMS     *int              `json:"checkin_block_ms"`
	PresetDesktopShape *bool             `json:"preset_desktop_shape"`
}

// apply 把增量合入设置（调用方持写锁）。
func (p settingsPatch) apply(st *store.Settings) {
	if p.ModelAliases != nil {
		st.ModelAliases = p.ModelAliases
	}
	if p.RequestTimeout != nil && *p.RequestTimeout > 0 {
		st.RequestTimeout = *p.RequestTimeout
	}
	if p.MaxRetries != nil && *p.MaxRetries > 0 {
		st.MaxRetries = *p.MaxRetries
	}
	if p.LogKeep != nil && *p.LogKeep > 0 {
		st.LogKeep = *p.LogKeep
	}
	if p.CORSOrigin != nil {
		st.CORSOrigin = *p.CORSOrigin
	}
	if p.AdminPassword != nil {
		st.AdminPassword = strings.TrimSpace(*p.AdminPassword)
	}
	if p.FallbackModel != nil {
		st.FallbackModel = strings.TrimSpace(*p.FallbackModel)
	}
	if p.StrictModel != nil {
		st.StrictModel = *p.StrictModel
	}
	// 限流参数允许显式设成 0（关闭限流），所以用 nil 判断而不是 > 0。
	if p.RateLimitRPS != nil && *p.RateLimitRPS >= 0 {
		st.RateLimitRPS = *p.RateLimitRPS
	}
	if p.RateLimitBurst != nil && *p.RateLimitBurst >= 0 {
		st.RateLimitBurst = *p.RateLimitBurst
	}
	if p.KeyConcurrency != nil && *p.KeyConcurrency >= 0 {
		st.KeyConcurrency = *p.KeyConcurrency
	}
	// 签到预算允许显式设成 0（= 默认值），所以用 nil 判断而不是 > 0。
	if p.CheckinBlockMS != nil && *p.CheckinBlockMS >= 0 {
		st.CheckinBlockMS = *p.CheckinBlockMS
	}
	if p.PresetDesktopShape != nil {
		st.PresetDesktopShape = *p.PresetDesktopShape
	}
}

func (s *Server) handleAdminSettings(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpx.WriteJSON(w, http.StatusOK, sanitizedSettings(s.Store.Settings()))
		return
	}
	var body settingsPatch
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&body); err != nil {
		httpx.WriteErr(w, http.StatusBadRequest, "请求体解析失败", "invalid_request_error")
		return
	}
	// 先合进副本再校验，通不过就不落地：只改一半的状态比不改更难排障。
	// 校验规则与默认值都在 store.Settings 上（Normalize / Validate），
	// 与运行期读取共用同一套口径。
	next := s.Store.Settings()
	body.apply(&next)
	next = next.Normalize()
	if err := next.Validate(); err != nil {
		httpx.WriteErr(w, http.StatusBadRequest, err.Error(), "invalid_request_error")
		return
	}
	s.Store.UpdateSettings(func(st *store.Settings) { *st = next })
	if !s.persistOrFail(w) {
		return
	}
	// 预算类设置在客户端侧是「拉取式」的，下一次请求即生效，无需推送。
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true, "settings": sanitizedSettings(s.Store.Settings())})
}

// handleAdminModels 刷新模型目录；GET 返回缓存。
func (s *Server) handleAdminModels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		// 用 s.catalog() / s.defaultModelID() 而不是 Store 的裸值：
		// 目录为空时（新装且上游不可达）它们会给出兜底模型，与 /v1/models
		// 保持一致。否则控制台模型页空白，而 API 还能用默认模型。
		httpx.WriteJSON(w, http.StatusOK, map[string]any{
			"models":         adminModelViews(registry.List(s.catalog(), s.Store.Settings().ModelAliases)),
			"default_model":  s.defaultModelID(),
			"models_updated": s.Store.ModelsUpdated(),
		})
		return
	}
	models, err := s.RefreshModels(r.Context())
	if err != nil {
		httpx.WriteErr(w, http.StatusBadGateway, "刷新模型目录失败: "+err.Error(), "upstream_error")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"ok": true, "count": len(models), "default_model": s.Store.DefaultModel(),
	})
}

// safeKeyList 把密钥列表转成控制台下发的视图（不含完整密钥）。
func safeKeyList(keys []*store.APIKey) []map[string]any {
	out := make([]map[string]any, 0, len(keys))
	for _, k := range keys {
		out = append(out, map[string]any{
			"key": mask(k.Key), "name": k.Name, "group": k.Group, "enabled": k.Enabled,
			"created_at": k.CreatedAt, "last_used": k.LastUsed, "requests": k.Requests,
		})
	}
	return out
}

// mask 对敏感串做脱敏，仅保留首尾片段。
func mask(s string) string {
	if len(s) <= 10 {
		if s == "" {
			return ""
		}
		return s[:1] + "***"
	}
	return s[:6] + "..." + s[len(s)-4:]
}

// shortMachine 只展示机器指纹前 12 位。
func shortMachine(id string) string {
	if len(id) <= 12 {
		return id
	}
	return id[:12]
}

// maskMachine 返回机器指纹的可读摘要。
//
// 账号级 machine 是原始 UUID，完整值不上控制台（它决定上游怎么发额度）。
func maskMachine(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if len(raw) <= 8 {
		return raw
	}
	return raw[:8] + "…"
}
