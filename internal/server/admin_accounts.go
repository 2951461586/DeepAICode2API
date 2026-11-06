package server

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"deepaicode2api/internal/deepai"
	"deepaicode2api/internal/parallel"
)

// --- 账号管理接口与配额巡检 ---

// 账号管理接口的 action 取值。
const (
	actionImport        = "import"
	actionManual        = "manual"
	actionEnroll        = "enroll"
	actionRemove        = "remove"
	actionToggle        = "toggle"
	actionClearErrors   = "clear-errors"
	actionAssignMachine = "assign-machine"
	actionSetGroup      = "set-group"
	actionMarkVerified  = "mark-verified"
)

func (s *Server) handleAdminAccounts(w http.ResponseWriter, r *http.Request) {
	// 同一路径按方法分发：GET 走分页查询，POST 走变更操作。
	if r.Method == http.MethodGet {
		s.handleAdminAccountsPage(w, r)
		return
	}
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "仅支持 GET / POST", "invalid_request_error")
		return
	}
	var body struct {
		Action        string `json:"action"`
		ID            string `json:"id"`
		Dir           string `json:"dir"`
		APIKey        string `json:"api_key"`
		Name          string `json:"name"`
		Username      string `json:"username"`
		Password      string `json:"password"`
		Email         string `json:"email"`
		SessionID     string `json:"session_id"`
		CSRFToken     string `json:"csrftoken"`
		Plan          string `json:"plan"`
		Group         string `json:"group"`
		UserPK        int64  `json:"user_pk"`
		EmailVerified bool   `json:"email_verified"`
		TheosBalance  string `json:"theos_balance"`
		TheosGranted  string `json:"theos_granted"`
		// SkipProbe 让调用方跳过入池时的 /theos 探测。
		// 注册链路刚刚验活过并随请求带上了 theos_balance/granted，
		// 再探一次既慢（~2s）又会引入全新的失败点。
		SkipProbe      bool   `json:"skip_probe"`
		Proxy          string `json:"proxy"`
		Fingerprint    string `json:"fingerprint"`
		MachineID      string `json:"machine_id"`
		FromGroup      string `json:"from_group"`
		UserAgent      string `json:"user_agent"`
		AcceptLanguage string `json:"accept_language"`
		SecCHUA        string `json:"sec_ch_ua"`
		Platform       string `json:"platform"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "请求体解析失败", "invalid_request_error")
		return
	}

	switch body.Action {
	case actionImport:
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
	case actionManual:
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
		go s.probeAndSave(acct)
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "new": isNew, "id": acct.ID})
	case actionEnroll:
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
		switch {
		case body.SkipProbe && strings.TrimSpace(body.TheosBalance) != "":
			al = allowanceFromEnroll(
				strings.TrimSpace(body.TheosBalance),
				strings.TrimSpace(body.TheosGranted), "")
		default:
			probed, err := s.probeAccountQuota(acct)
			if err != nil {
				writeErr(w, http.StatusBadRequest, "Key 验活失败: "+err.Error(), "invalid_request_error")
				return
			}
			al = probed
			if strings.TrimSpace(body.TheosBalance) != "" {
				al.Balance = strings.TrimSpace(body.TheosBalance)
			}
			if strings.TrimSpace(body.TheosGranted) != "" {
				al.Granted = strings.TrimSpace(body.TheosGranted)
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
	case actionRemove:
		ok := s.Store.RemoveAccount(body.ID)
		_ = s.Store.Save()
		writeJSON(w, http.StatusOK, map[string]any{"ok": ok})
	case actionToggle:
		s.Store.UpdateAccount(body.ID, func(a *deepai.Account) { a.Enabled = !a.Enabled })
		_ = s.Store.Save()
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	case actionClearErrors:
		s.Store.UpdateAccount(body.ID, func(a *deepai.Account) {
			a.Failures = 0
			a.Status = "unknown"
			a.LastErr = ""
		})
		_ = s.Store.Save()
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	case actionAssignMachine:
		// 为存量账号补一个独立机器指纹。只补空值，不覆盖已有指纹
		// （覆盖会让该账号已被上游记住的额度归属失效）。
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
		go s.probeAccounts(targets)
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "assigned": assigned})
	case actionMarkVerified:
		// 后台验证完成后回填：入池时 email_verified 可能还是 false。
		if err := s.markAccountVerified(body.ID, body.Email); err != nil {
			writeErr(w, http.StatusNotFound, err.Error(), "invalid_request_error")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	case actionSetGroup:
		// 把账号迁到另一个分组（例如把压测批次的账号并入日常号池）。
		// id 为空时按 from_group 批量迁移，便于整批归位。
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
	default:
		writeErr(w, http.StatusBadRequest, "未知操作: "+body.Action, "invalid_request_error")
	}
}

// handleAdminCheckin 对全部（或指定）账号查询一次 Theos 配额。
//
// 并发度限制为 4：串行探测 20 个账号最坏要 10 分钟，全并发又容易
// 把探针本身变成上游的风控特征。
func (s *Server) handleAdminCheckin(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID string `json:"id"`
	}
	_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&body)

	targets := s.Store.Accounts()
	if body.ID != "" {
		targets = nil
		if a := s.Store.Account(body.ID); a != nil {
			targets = append(targets, a)
		}
	}

	const workers = 4
	// 按下标回填，结果顺序与号池顺序一致（旧实现按完成顺序 append，
	// 控制台列表会随机跳动）。
	results := make([]map[string]any, len(targets))
	parallel.For(r.Context(), workers, len(targets), func(ctx context.Context, i int) {
		a := targets[i]
		ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		al, err := s.clientFor(a).Theos(ctx, a)
		s.Store.SetAccountAllowance(a.ID, al, err)
		note := ""
		if err != nil {
			note = err.Error()
		}
		results[i] = map[string]any{
			"id": a.ID, "name": a.Name,
			"ok": err == nil, "err": note, "theos": al,
		}
	})
	_ = s.Store.Save()

	// 请求取消时未跑完的槽位为 nil，从响应里去掉，避免下发 null 条目。
	done := make([]map[string]any, 0, len(results))
	for _, res := range results {
		if res != nil {
			done = append(done, res)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "results": done})
}
