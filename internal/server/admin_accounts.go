package server

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"deepaicode2api/internal/deepai"
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
				if deepai.IsSocialLoginRequired(err) {
					writeErr(w, http.StatusBadRequest, "上游已限制为 Google/Apple 登录（social_login_required），该 Key 无法推理", "social_login_required")
					return
				}
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

// handleAdminCheckin 对一批账号查询 Theos 配额。
//
// 可选参数：
//   - id：只探这个账号；
//   - group：只探该分组的账号；
//   - stale_only：只探额度已过时（refresh_at 已过 / 从未探 / 超 15 分钟）的账号；
//   - limit：本批最多探多少个（默认与上限都是 maxCheckinBatch）。
//
// 响应带 total / probed / remaining：653 号的池子全量探测要 11 分钟，
// 单次请求只做一批，控制台按 remaining 继续下一批（探过的 checked_at 会
// 推到现在，下一批自然取到没探过的，不会重复烧额度）。
func (s *Server) handleAdminCheckin(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID        string `json:"id"`
		Group     string `json:"group"`
		Limit     int    `json:"limit"`
		StaleOnly bool   `json:"stale_only"`
	}
	_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&body)

	now := time.Now()
	var targets []*deepai.Account
	switch {
	case strings.TrimSpace(body.ID) != "":
		if a := s.Store.Account(body.ID); a != nil {
			targets = append(targets, a)
		}
	default:
		group := strings.TrimSpace(body.Group)
		for _, a := range s.Store.Accounts() {
			if group != "" && !strings.EqualFold(a.GroupName(), group) {
				continue
			}
			if body.StaleOnly && !quotaNeedsRefresh(a, now, 0) {
				continue
			}
			targets = append(targets, a)
		}
	}
	total := len(targets)

	// 优先探「额度刚刷新 / 从未探过」的，分批才有意义。
	sortByQuotaDue(targets, now)
	limit := body.Limit
	if limit <= 0 || limit > maxCheckinBatch {
		limit = maxCheckinBatch
	}
	batch := targets
	if len(batch) > limit {
		batch = batch[:limit]
	}

	// 页面关掉也把这批探完：结果会落盘，重新打开就能看到。
	// 没有 WithoutCancel 时，浏览器一切换标签页（fetch 被 abort）
	// 就会让半批账号停在旧状态，控制台看起来像“刷新失败”。
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 10*time.Minute)
	defer cancel()
	out := s.probeBatch(ctx, batch, 30*time.Second, true)

	results := make([]map[string]any, 0, len(out))
	settled := make(map[string]bool, len(out))
	for _, o := range out {
		if o.Account == nil {
			continue
		}
		settled[o.Account.ID] = true
		item := map[string]any{"id": o.Account.ID, "name": o.Account.Name}
		switch {
		case o.Skipped:
			// 已有探测在跑：既不算失败，也不算这次探过。
			item["ok"] = true
			item["skipped"] = true
			item["note"] = "已有探测在进行"
		case o.Allowance == nil && o.Err == nil:
			item["ok"] = false
			item["err"] = "探测已取消"
		default:
			item["ok"] = o.Err == nil
			item["theos"] = o.Allowance
			if o.Err != nil {
				item["err"] = o.Err.Error()
			}
		}
		results = append(results, item)
	}

	// remaining 是「过滤集里仍然过期、且不在本批里」的账号数，而不是
	// total - probed：否则每次分批都剩同一批在途/失败的号，控制台会反复重试。
	remaining := 0
	for _, a := range targets {
		if settled[a.ID] {
			continue
		}
		if quotaNeedsRefresh(a, now, 0) {
			remaining++
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":        true,
		"total":     total,
		"probed":    len(batch),
		"remaining": remaining,
		"results":   results,
	})
}
