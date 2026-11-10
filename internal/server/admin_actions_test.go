package server

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"deepaicode2api/internal/deepai"
)

// 这组测试覆盖改造前**零覆盖**的 5 个 action：import / manual / remove / toggle /
// clear-errors。表驱动重构（admin_actions.go）之后，每个 action 的入参结构体
// 都变了，这里把它们的行为钉住。

// theosUpstream 返回一个 /theos 有额度的假上游。
func theosUpstream(t *testing.T) *upstream {
	t.Helper()
	return newUpstream(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, deepai.PathTheos) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"enabled":true,"balance":"200","granted":"200"}`))
			return
		}
		http.NotFound(w, r)
	})
}

func TestAdminActionManualThenToggleClearRemove(t *testing.T) {
	s, _ := newTestServer(t, theosUpstream(t))

	// manual：新建账号。
	rec := do(t, s, http.MethodPost, "/admin/api/accounts", "",
		`{"action":"manual","api_key":"sk-manual-1","name":"手工号","group":"g9"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("manual = %d：%s", rec.Code, rec.Body.String())
	}
	type manualResp struct {
		OK  bool   `json:"ok"`
		New bool   `json:"new"`
		ID  string `json:"id"`
	}
	created := decodeJSON[manualResp](t, rec.Body.Bytes())
	if !created.OK || !created.New || created.ID == "" {
		t.Fatalf("manual 响应异常: %+v", created)
	}
	acct := s.Store.Account(created.ID)
	if acct == nil {
		t.Fatal("manual 之后账号不存在")
	}
	if acct.Name != "手工号" || acct.Group != "g9" || acct.Source != "manual" {
		t.Fatalf("账号字段未按请求写入: %+v", acct)
	}
	if !acct.Enabled {
		t.Fatal("新账号默认应启用")
	}
	s.waitQuotaReprobe() // 等异步探活收尾，避免和 TempDir 清理竞争

	// toggle：启用状态翻转。
	if rec := do(t, s, http.MethodPost, "/admin/api/accounts", "",
		`{"action":"toggle","id":"`+created.ID+`"}`); rec.Code != http.StatusOK {
		t.Fatalf("toggle = %d", rec.Code)
	}
	if s.Store.Account(created.ID).Enabled {
		t.Fatal("toggle 未停用账号")
	}
	do(t, s, http.MethodPost, "/admin/api/accounts", "", `{"action":"toggle","id":"`+created.ID+`"}`)
	if !s.Store.Account(created.ID).Enabled {
		t.Fatal("toggle 第二次未恢复启用")
	}

	// clear-errors：清失败计数与错误状态。
	s.Store.UpdateAccount(created.ID, func(a *deepai.Account) {
		a.Failures = 7
		a.Status = "error"
		a.LastErr = "boom"
	})
	if rec := do(t, s, http.MethodPost, "/admin/api/accounts", "",
		`{"action":"clear-errors","id":"`+created.ID+`"}`); rec.Code != http.StatusOK {
		t.Fatalf("clear-errors = %d", rec.Code)
	}
	cleared := s.Store.Account(created.ID)
	if cleared.Failures != 0 || cleared.Status != "unknown" || cleared.LastErr != "" {
		t.Fatalf("clear-errors 未清干净: %+v", cleared)
	}

	// remove：删除存在的账号返回 ok=true，不存在的返回 ok=false。
	type okResp struct {
		OK bool `json:"ok"`
	}
	rec = do(t, s, http.MethodPost, "/admin/api/accounts", "", `{"action":"remove","id":"`+created.ID+`"}`)
	removed := decodeJSON[okResp](t, rec.Body.Bytes())
	if !removed.OK {
		t.Fatalf("remove 已存在的账号应 ok=true: %s", rec.Body.String())
	}
	if s.Store.Account(created.ID) != nil {
		t.Fatal("remove 之后账号仍然存在")
	}

	rec = do(t, s, http.MethodPost, "/admin/api/accounts", "", `{"action":"remove","id":"no-such-id"}`)
	removed = decodeJSON[okResp](t, rec.Body.Bytes())
	if removed.OK {
		t.Fatalf("remove 不存在的账号应 ok=false: %s", rec.Body.String())
	}
}

func TestAdminActionImportFromDir(t *testing.T) {
	s, _ := newTestServer(t, nil)

	base := t.TempDir()
	dir := filepath.Join(base, "userdata", "secrets")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	cred := `{"apiKey":"sk-from-desktop","username":"desk@example.com"}`
	if err := os.WriteFile(filepath.Join(dir, deepai.CredentialFile), []byte(cred), 0o600); err != nil {
		t.Fatal(err)
	}

	type importResp struct {
		OK       bool `json:"ok"`
		Count    int  `json:"count"`
		Imported []struct {
			ID   string `json:"id"`
			Path string `json:"path"`
			New  bool   `json:"new"`
		} `json:"imported"`
	}

	body := mustMarshal(t, map[string]string{"action": "import", "dir": base}) // Windows 路径含反斜杠，必须走 JSON 编码
	rec := do(t, s, http.MethodPost, "/admin/api/accounts", "", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("import = %d：%s", rec.Code, rec.Body.String())
	}
	got := decodeJSON[importResp](t, rec.Body.Bytes())
	if !got.OK || got.Count != 1 || len(got.Imported) != 1 {
		t.Fatalf("import 结果异常: %+v", got)
	}
	if !got.Imported[0].New {
		t.Fatal("首次导入应为新建")
	}
	if !strings.HasSuffix(got.Imported[0].Path, deepai.CredentialFile) {
		t.Fatalf("导入路径 = %q", got.Imported[0].Path)
	}
	if acct := s.Store.Account(got.Imported[0].ID); acct == nil || acct.Source != "desktop" {
		t.Fatalf("导入账号未落库或来源错误: %+v", acct)
	}

	// 再导一次：同一个 Key 走 Upsert，不是新建。
	rec = do(t, s, http.MethodPost, "/admin/api/accounts", "", body)
	again := decodeJSON[importResp](t, rec.Body.Bytes())
	if len(again.Imported) != 1 || again.Imported[0].New {
		t.Fatalf("重复导入应复用已有账号: %+v", again)
	}
}

func TestAdminActionRejectsBadInput(t *testing.T) {
	s, _ := newTestServer(t, nil)

	cases := []struct {
		name string
		body string
		want int
	}{
		{"未知 action", `{"action":"nope"}`, http.StatusBadRequest},
		{"缺 action", `{}`, http.StatusBadRequest},
		{"JSON 非法", `{`, http.StatusBadRequest},
		{"manual 缺 key", `{"action":"manual"}`, http.StatusBadRequest},
		{"import 目录不存在", `{"action":"import","dir":"/no/such/dir"}`, http.StatusBadRequest},
		{"set-group 缺 group", `{"action":"set-group","id":"x"}`, http.StatusBadRequest},
	}
	for _, c := range cases {
		if rec := do(t, s, http.MethodPost, "/admin/api/accounts", "", c.body); rec.Code != c.want {
			t.Errorf("%s: 状态码 = %d，期望 %d：%s", c.name, rec.Code, c.want, rec.Body.String())
		}
	}

	// 未知 action 的错误文案要带上 action 名，便于控制台排错。
	rec := do(t, s, http.MethodPost, "/admin/api/accounts", "", `{"action":"nope"}`)
	if !strings.Contains(rec.Body.String(), "未知操作: nope") {
		t.Fatalf("错误文案未带 action: %s", rec.Body.String())
	}

	// 只读方法之外的动词应 405。
	if rec := do(t, s, http.MethodPut, "/admin/api/accounts", "", ""); rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("PUT = %d，期望 405", rec.Code)
	}
}
