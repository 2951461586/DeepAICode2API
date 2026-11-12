package server

import (
	"context"
	"encoding/json"
	"io"
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
	actionAssignProxy   = "assign-proxy"
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
	// 两步解析：先取 action，再让具体 action 解自己的字段（见 admin_actions.go）。
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "请求体解析失败", "invalid_request_error")
		return
	}
	var env adminActionEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		writeErr(w, http.StatusBadRequest, "请求体解析失败", "invalid_request_error")
		return
	}
	if fn, ok := adminActions[env.Action]; ok {
		fn(s, w, raw)
		return
	}
	writeErr(w, http.StatusBadRequest, "未知操作: "+env.Action, "invalid_request_error")
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
	// 可以不传参数（等价于「探一批全部账号」），但畸形 JSON 必须报错。
	if !decodeOptionalJSON(w, r, &body) {
		return
	}

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
	probed := 0
	for _, o := range out {
		if o.Account == nil {
			continue
		}
		item := map[string]any{"id": o.Account.ID, "name": o.Account.Name}
		switch {
		case o.Skipped:
			// 已有探测在跑：既不算失败，也不算这次探过，所以**不进 settled**。
			// 进 settled 会让 remaining 把它当成「已探过」而收敛，而它其实还
			// 处于未知状态；不进则下一批会把它重新算进 remaining，等另一路
			// 探完自然就散了。
			item["ok"] = true
			item["skipped"] = true
			item["note"] = "已有探测在进行"
		default:
			settled[o.Account.ID] = true
			probed++
			if o.Allowance == nil && o.Err == nil {
				item["ok"] = false
				item["err"] = "探测已取消"
			} else {
				item["ok"] = o.Err == nil
				item["theos"] = o.Allowance
				if o.Err != nil {
					item["err"] = o.Err.Error()
				}
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
		"probed":    probed,              // 真正发起探测的账号数（skipped 不算）
		"skipped":   len(batch) - probed, // 已有探测在跑、本轮跳过的
		"remaining": remaining,
		"results":   results,
	})
}
