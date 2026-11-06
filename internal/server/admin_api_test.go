package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"deepaicode2api/internal/deepai"
	"deepaicode2api/internal/store"
)

// overview 只应包含不随账号数增长的元信息，不含账号列表。
func TestOverviewEndpointShape(t *testing.T) {
	s, key := newTestServer(t, nil)

	rec := do(t, s, http.MethodGet, "/admin/api/overview", key, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("状态码 = %d：%s", rec.Code, rec.Body.String())
	}
	var d map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &d); err != nil {
		t.Fatalf("解析: %v", err)
	}
	for _, k := range []string{"accounts", "ready", "keys", "models", "settings", "stats", "default_model", "machine_id"} {
		if _, ok := d[k]; !ok {
			t.Fatalf("overview 缺少字段 %q", k)
		}
	}
	// 账号列表必须走分页端点，overview 不应下发。
	if _, ok := d["items"]; ok {
		t.Fatal("overview 不应包含 items")
	}
	if got, _ := d["accounts"].(float64); got < 1 {
		t.Fatalf("accounts 计数异常: %v", d["accounts"])
	}
}

// 分页端点应遵守 offset/limit 并返回过滤后的 total。
func TestAccountsPageEndpoint(t *testing.T) {
	s, key := newTestServer(t, nil)
	for _, id := range []string{"b1", "b2"} {
		s.Store.UpsertAccount(&deepai.Account{ID: id, APIKey: "sk-" + id, Group: "g2", Enabled: true})
	}

	rec := do(t, s, http.MethodGet, "/admin/api/accounts?offset=0&limit=2", key, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("状态码 = %d：%s", rec.Code, rec.Body.String())
	}
	var page struct {
		Items  []map[string]any `json:"items"`
		Total  int              `json:"total"`
		Offset int              `json:"offset"`
		Limit  int              `json:"limit"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil {
		t.Fatalf("解析: %v", err)
	}
	if page.Total != 3 {
		t.Fatalf("total = %d，期望 3", page.Total)
	}
	if len(page.Items) != 2 {
		t.Fatalf("items = %d，期望 2", len(page.Items))
	}
	// 下发的是投影：必须有掩码 key，且不含密码。
	if _, ok := page.Items[0]["key"]; !ok {
		t.Fatal("缺少 key 投影")
	}
	if _, ok := page.Items[0]["password"]; ok {
		t.Fatal("不应下发密码")
	}

	// 分组过滤
	rec = do(t, s, http.MethodGet, "/admin/api/accounts?group=g2", key, "")
	if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil {
		t.Fatalf("解析: %v", err)
	}
	if page.Total != 2 {
		t.Fatalf("分组过滤 total = %d，期望 2", page.Total)
	}
}

// 空闲刷新应命中 ETag 返回 304。
func TestOverviewETag(t *testing.T) {
	s, key := newTestServer(t, nil)

	rec := do(t, s, http.MethodGet, "/admin/api/overview", key, "")
	etag := rec.Header().Get("ETag")
	if etag == "" {
		t.Fatal("overview 缺少 ETag")
	}

	req := httptest.NewRequest(http.MethodGet, "/admin/api/overview", nil)
	req.RemoteAddr = "127.0.0.1:34567"
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("If-None-Match", etag)
	rec2 := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec2, req)

	if rec2.Code != http.StatusNotModified {
		t.Fatalf("命中 ETag 应返回 304，实际 %d", rec2.Code)
	}
}

// 兼容端点支持 ?accounts=0 只取元信息。
func TestStateEndpointCanOmitAccounts(t *testing.T) {
	s, key := newTestServer(t, nil)

	rec := do(t, s, http.MethodGet, "/admin/api/state?accounts=0", key, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("状态码 = %d", rec.Code)
	}
	var d map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &d); err != nil {
		t.Fatalf("解析: %v", err)
	}
	if v, ok := d["accounts"]; ok && v != nil {
		t.Fatalf("accounts 应为空，实际 %#v", v)
	}
	if _, ok := d["stats"]; !ok {
		t.Fatal("元信息仍应存在")
	}
}

// 日志端点应接受过滤参数。
func TestLogsEndpointFilters(t *testing.T) {
	s, key := newTestServer(t, nil)
	s.Store.Record(store.LogEntry{Time: 1, Endpoint: "chat", Status: 200}, 0)
	s.Store.Record(store.LogEntry{Time: 2, Endpoint: "audio", Status: 500}, 0)

	rec := do(t, s, http.MethodGet, "/admin/api/logs?endpoint=chat", key, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("状态码 = %d", rec.Code)
	}
	var d struct {
		Logs []map[string]any `json:"logs"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &d); err != nil {
		t.Fatalf("解析: %v", err)
	}
	if len(d.Logs) != 1 || d.Logs[0]["endpoint"] != "chat" {
		t.Fatalf("过滤未生效: %#v", d.Logs)
	}
}
