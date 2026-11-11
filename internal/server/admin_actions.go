package server

// 账号管理的 10 个 action 各自一个处理函数。
//
// 改造前是一个 255 行的 switch，共享一个 30 字段的「万能请求体」：
// `action=remove` 只需要 id，却和 `action=enroll` 共用同一个结构体，
// 必填项与可选性完全看不出来。
//
// 现在分两步解析：
//  1. 先只解出 `action`（adminActionEnvelope）；
//  2. 再按 action 解它自己的最小结构体。
//
// 好处是每个 action 的入参在签名上就写清楚了，新增 action 只加一个函数
// 和一行表项，不会让任何一个函数变长。
//
// 行为差异（唯一的）：某个 action 的请求体里带了**别的** action 才用的
// 非法字段时，以前会整体解码失败，现在忽略。这只会让接口更宽容。

import (
	"encoding/json"
	"net/http"
	"strings"

	"deepaicode2api/internal/deepai"
	"deepaicode2api/internal/proxy"
)

// adminActionEnvelope 只承载 action 本身，用于第一次解析。
type adminActionEnvelope struct {
	Action string `json:"action"`
}

// adminActionFunc 处理一个 action：自己写完整响应（含错误）。
type adminActionFunc func(s *Server, w http.ResponseWriter, raw []byte)

// adminActions 是 action → 处理函数的表。
//
// 新增 action 时：加一个常量（admin_accounts.go）、加一个函数、加一行表项；
// 别忘了给它自己的最小请求结构体。
var adminActions = map[string]adminActionFunc{
	actionImport:        (*Server).adminImport,
	actionManual:        (*Server).adminManual,
	actionEnroll:        (*Server).adminEnroll,
	actionRemove:        (*Server).adminRemove,
	actionToggle:        (*Server).adminToggle,
	actionClearErrors:   (*Server).adminClearErrors,
	actionAssignMachine: (*Server).adminAssignMachine,
	actionAssignProxy:   (*Server).adminAssignProxy,
	actionMarkVerified:  (*Server).adminMarkVerified,
	actionSetGroup:      (*Server).adminSetGroup,
}

// decodeAction 把原始请求体解成某个 action 自己的结构体。
func decodeAction[T any](w http.ResponseWriter, raw []byte) (T, bool) {
	var v T
	if err := json.Unmarshal(raw, &v); err != nil {
		var zero T
		writeErr(w, http.StatusBadRequest, "请求体解析失败", "invalid_request_error")
		return zero, false
	}
	return v, true
}

// --- 各 action 的请求结构体（只含该 action 真正用到的字段）---

type adminImportReq struct {
	Dir string `json:"dir"`
}

type adminManualReq struct {
	APIKey string `json:"api_key"`
	Name   string `json:"name"`
	Group  string `json:"group"`
}

// adminEnrollReq 是入池请求：注册链路会带齐身份 + 指纹 + 预探测结果。
type adminEnrollReq struct {
	Email          string `json:"email"`
	Password       string `json:"password"`
	Username       string `json:"username"`
	APIKey         string `json:"api_key"`
	SessionID      string `json:"session_id"`
	CSRFToken      string `json:"csrftoken"`
	UserPK         int64  `json:"user_pk"`
	Plan           string `json:"plan"`
	EmailVerified  bool   `json:"email_verified"`
	Group          string `json:"group"`
	Proxy          string `json:"proxy"`
	Fingerprint    string `json:"fingerprint"`
	MachineID      string `json:"machine_id"`
	UserAgent      string `json:"user_agent"`
	AcceptLanguage string `json:"accept_language"`
	SecCHUA        string `json:"sec_chua"`
	Platform       string `json:"platform"`

	// SkipProbe 让调用方跳过入池时的 /theos 探测：注册链路刚验活过并随请求
	// 带上了 theos_balance / theos_granted，再探一次既慢（~2s）又会引入新失败点。
	SkipProbe    bool   `json:"skip_probe"`
	TheosBalance string `json:"theos_balance"`
	TheosGranted string `json:"theos_granted"`
}

// adminIDReq 是「按 id 操作单个账号」的公共形状。
type adminIDReq struct {
	ID string `json:"id"`
}

type adminAssignProxyReq struct {
	ID    string `json:"id"`
	Group string `json:"group"`
	// Reassign 为 true 时覆盖已有代理；Clear 为 true 时清空（回滚用）。
	Reassign bool `json:"reassign"`
	Clear    bool `json:"clear"`
}

type adminMarkVerifiedReq struct {
	ID    string `json:"id"`
	Email string `json:"email"`
}

type adminSetGroupReq struct {
	ID        string `json:"id"`
	Group     string `json:"group"`
	FromGroup string `json:"from_group"`
}

// --- action 实现 ---

// adminImport 导入本机桌面端凭据。
func (s *Server) adminImport(w http.ResponseWriter, raw []byte) {
	body, ok := decodeAction[adminImportReq](w, raw)
	if !ok {
		return
	}
	results, err := deepai.ImportAllFromDesktop(body.Dir)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "导入失败: "+err.Error(), "invalid_request_error")
		return
	}
	imported := make([]map[string]any, 0, len(results))
	for _, res := range results {
		isNew := s.Store.UpsertAccount(res.Account)
		imported = append(imported, map[string]any{
			"id": res.Account.ID, "name": res.Account.Name,
			"username": res.Account.Username, "path": res.Path, "new": isNew,
		})
	}
	_ = s.Store.Save()
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "count": len(imported), "imported": imported})
}

// adminManual 用一把手工粘贴的 API Key 建账号。
func (s *Server) adminManual(w http.ResponseWriter, raw []byte) {
	body, ok := decodeAction[adminManualReq](w, raw)
	if !ok {
		return
	}
	acct, err := deepai.AccountFromAPIKey(body.APIKey, body.Name, "manual", false)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error(), "invalid_request_error")
		return
	}
	if strings.TrimSpace(body.Group) != "" {
		acct.Group = strings.TrimSpace(body.Group)
	}
	isNew := s.Store.UpsertAccount(acct)
	_ = s.Store.Save()
	s.probeAndSaveAsync(acct)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "new": isNew, "id": acct.ID})
}

// adminEnroll 把已验活的账号写入号池（唯一写入口）。
func (s *Server) adminEnroll(w http.ResponseWriter, raw []byte) {
	body, ok := decodeAction[adminEnrollReq](w, raw)
	if !ok {
		return
	}
	reg := deepai.Registration{
		Email: body.Email, Password: body.Password, Username: body.Username,
		APIKey: body.APIKey, SessionID: body.SessionID, CSRFToken: body.CSRFToken,
		UserPK: body.UserPK, Plan: body.Plan, EmailVerified: body.EmailVerified,
		Group: body.Group, Proxy: body.Proxy, Fingerprint: body.Fingerprint,
		MachineID: strings.TrimSpace(body.MachineID),
		UserAgent: strings.TrimSpace(body.UserAgent), AcceptLanguage: strings.TrimSpace(body.AcceptLanguage),
		SecCHUA: strings.TrimSpace(body.SecCHUA), Platform: strings.TrimSpace(body.Platform),
		Source: "register",
	}
	// 每个账号必须有独立机器指纹；否则上游的每日免费额度会与其他账号互斥。
	if reg.MachineID == "" {
		reg.MachineID = deepai.NewMachineUUID()
	}
	acct, err := deepai.AccountFromRegistration(reg)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error(), "invalid_request_error")
		return
	}

	// 调用方已验活并带上结果时直接采信；否则自己探（控制台手工入池走这条）。
	var al *deepai.Allowance
	balance, granted := strings.TrimSpace(body.TheosBalance), strings.TrimSpace(body.TheosGranted)
	if body.SkipProbe && balance != "" {
		al = allowanceFromEnroll(balance, granted, "")
	} else {
		probed, err := s.probeAccountQuota(acct)
		if err != nil {
			if deepai.IsSocialLoginRequired(err) {
				writeErr(w, http.StatusBadRequest, "上游已限制为 Google/Apple 登录（social_login_required），该 Key 无法推理", "social_login_required")
				return
			}
			writeErr(w, http.StatusBadRequest, "Key 验活失败: "+err.Error(), "invalid_request_error")
			return
		}
		al = probed
		if balance != "" {
			al.Balance = balance
		}
		if granted != "" {
			al.Granted = granted
		}
	}

	id, isNew, err := s.enrollRegistration(reg, al)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "入池失败: "+err.Error(), "server_error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "id": id, "new": isNew,
		"email_verified": body.EmailVerified, "theos": al,
	})
}

func (s *Server) adminRemove(w http.ResponseWriter, raw []byte) {
	body, ok := decodeAction[adminIDReq](w, raw)
	if !ok {
		return
	}
	removed := s.Store.RemoveAccount(body.ID)
	_ = s.Store.Save()
	writeJSON(w, http.StatusOK, map[string]any{"ok": removed})
}

func (s *Server) adminToggle(w http.ResponseWriter, raw []byte) {
	body, ok := decodeAction[adminIDReq](w, raw)
	if !ok {
		return
	}
	s.Store.UpdateAccount(body.ID, func(a *deepai.Account) { a.Enabled = !a.Enabled })
	_ = s.Store.Save()
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) adminClearErrors(w http.ResponseWriter, raw []byte) {
	body, ok := decodeAction[adminIDReq](w, raw)
	if !ok {
		return
	}
	s.Store.UpdateAccount(body.ID, func(a *deepai.Account) {
		a.Failures = 0
		a.Status = "unknown"
		a.LastErr = ""
	})
	_ = s.Store.Save()
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// adminAssignMachine 为存量账号补一个独立机器指纹。
//
// 只补空值，不覆盖已有指纹（覆盖会让该账号已被上游记住的额度归属失效）。
func (s *Server) adminAssignMachine(w http.ResponseWriter, raw []byte) {
	body, ok := decodeAction[adminIDReq](w, raw)
	if !ok {
		return
	}
	var targets []*deepai.Account
	assigned := s.Store.UpdateAccounts(func(a *deepai.Account) bool {
		if body.ID != "" && a.ID != body.ID {
			return false
		}
		if strings.TrimSpace(a.MachineID) != "" {
			return false
		}
		a.MachineID = deepai.NewMachineUUID()
		targets = append(targets, a.Clone())
		return true
	})
	_ = s.Store.Save()
	// 重新探测配额，让调度能立即看到新指纹下的额度。
	// 走登记过的批量入口：进程关停时要等这次落盘跑完。
	s.probeBatchAsync(targets)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "assigned": assigned})
}

// adminAssignProxy 把注册代理池（PROXY_POOL / PROXY_POOL_FILE）轮询分配给号池账号。
//
// 默认只补空值；reassign=true 覆盖，clear=true 清空（回滚）。
func (s *Server) adminAssignProxy(w http.ResponseWriter, raw []byte) {
	body, ok := decodeAction[adminAssignProxyReq](w, raw)
	if !ok {
		return
	}
	id := strings.TrimSpace(body.ID)
	group := strings.TrimSpace(body.Group)
	match := func(a *deepai.Account) bool {
		if id != "" {
			return a.ID == id
		}
		if group != "" {
			return strings.EqualFold(a.GroupName(), group)
		}
		return true
	}

	if body.Clear {
		cleared := s.Store.UpdateAccounts(func(a *deepai.Account) bool {
			if !match(a) || strings.TrimSpace(a.Proxy) == "" {
				return false
			}
			a.Proxy = ""
			return true
		})
		_ = s.Store.Save()
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "cleared": cleared})
		return
	}

	pool, err := loadProxyPool()
	if err != nil {
		writeErr(w, http.StatusBadRequest, "代理池配置错误: "+err.Error(), "invalid_request_error")
		return
	}
	if pool == nil || pool.Len() == 0 {
		writeErr(w, http.StatusBadRequest, "未配置代理池（PROXY_POOL / DEEPAI_REGISTER_PROXY_POOL / PROXY_POOL_FILE）", "invalid_request_error")
		return
	}

	dist := map[string]int{}
	assigned := s.Store.UpdateAccounts(func(a *deepai.Account) bool {
		if !match(a) {
			return false
		}
		if !body.Reassign && strings.TrimSpace(a.Proxy) != "" {
			return false
		}
		p := pool.Next()
		if p == "" {
			return false
		}
		a.Proxy = p
		dist[proxy.ProxyLabel(p)]++
		return true
	})
	_ = s.Store.Save()
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "assigned": assigned, "pool_size": pool.Len(), "proxies": dist,
	})
}

// adminMarkVerified 由后台验证链路回填 email_verified（入池时可能还是 false）。
func (s *Server) adminMarkVerified(w http.ResponseWriter, raw []byte) {
	body, ok := decodeAction[adminMarkVerifiedReq](w, raw)
	if !ok {
		return
	}
	if err := s.markAccountVerified(body.ID, body.Email); err != nil {
		writeErr(w, http.StatusNotFound, err.Error(), "invalid_request_error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// adminSetGroup 把账号迁到另一个分组（例如把压测批次并入日常号池）。
//
// id 为空时按 from_group 批量迁移，便于整批归位。
func (s *Server) adminSetGroup(w http.ResponseWriter, raw []byte) {
	body, ok := decodeAction[adminSetGroupReq](w, raw)
	if !ok {
		return
	}
	target := strings.TrimSpace(body.Group)
	if target == "" {
		writeErr(w, http.StatusBadRequest, "group 不能为空", "invalid_request_error")
		return
	}
	moved := s.Store.UpdateAccounts(func(a *deepai.Account) bool {
		if body.ID != "" && a.ID != body.ID {
			return false
		}
		if body.FromGroup != "" && !strings.EqualFold(a.GroupName(), body.FromGroup) {
			return false
		}
		if a.GroupName() == target {
			return false
		}
		a.Group = target
		return true
	})
	_ = s.Store.Save()
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "moved": moved, "group": target})
}
