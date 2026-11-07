package server

import (
	"net/http"
	"strings"
	"testing"

	"deepaicode2api/internal/deepai"
)

type proxyAssignResult struct {
	OK       bool           `json:"ok"`
	Assigned int            `json:"assigned"`
	Cleared  int            `json:"cleared"`
	PoolSize int            `json:"pool_size"`
	Proxies  map[string]int `json:"proxies"`
}

// 注册代理池轮询分配给号池：默认只补空值，reassign 覆盖，clear 可回滚。
func TestAssignProxyFromPool(t *testing.T) {
	t.Setenv("PROXY_POOL", "http://127.0.0.1:9001,http://127.0.0.1:9002")
	s := newEmptyServer(t)

	for _, a := range []*deepai.Account{
		{ID: "a", Name: "a", APIKey: "k-a", Enabled: true, Status: "ok"},
		{ID: "b", Name: "b", APIKey: "k-b", Enabled: true, Status: "ok"},
		{ID: "c", Name: "c", APIKey: "k-c", Enabled: true, Status: "ok", Proxy: "http://127.0.0.1:8000"},
	} {
		s.Store.UpsertAccount(a)
	}

	rec := postAdmin(t, s, "/admin/api/accounts", `{"action":"assign-proxy"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("状态码 = %d：%s", rec.Code, rec.Body.String())
	}
	out := decodeJSON[proxyAssignResult](t, rec.Body.Bytes())
	if out.Assigned != 2 || out.PoolSize != 2 {
		t.Fatalf("应只补 2 个空代理，实际 assigned=%d pool=%d", out.Assigned, out.PoolSize)
	}
	a, b, c := s.Store.Account("a"), s.Store.Account("b"), s.Store.Account("c")
	if a.Proxy == "" || b.Proxy == "" {
		t.Fatalf("空代理未被补齐: a=%q b=%q", a.Proxy, b.Proxy)
	}
	if a.Proxy == b.Proxy {
		t.Fatalf("轮询应分配不同代理，实际都是 %q", a.Proxy)
	}
	if c.Proxy != "http://127.0.0.1:8000" {
		t.Fatalf("已有代理不应被覆盖，实际 %q", c.Proxy)
	}

	// 默认不重复分配。
	rec = postAdmin(t, s, "/admin/api/accounts", `{"action":"assign-proxy"}`)
	out = decodeJSON[proxyAssignResult](t, rec.Body.Bytes())
	if out.Assigned != 0 {
		t.Fatalf("重复调用不应再分配，实际 %d", out.Assigned)
	}

	// reassign 覆盖全部。
	rec = postAdmin(t, s, "/admin/api/accounts", `{"action":"assign-proxy","reassign":true}`)
	out = decodeJSON[proxyAssignResult](t, rec.Body.Bytes())
	if out.Assigned != 3 {
		t.Fatalf("reassign 应覆盖 3 个，实际 %d", out.Assigned)
	}

	// clear 回滚。
	rec = postAdmin(t, s, "/admin/api/accounts", `{"action":"assign-proxy","clear":true}`)
	out = decodeJSON[proxyAssignResult](t, rec.Body.Bytes())
	if out.Cleared != 3 {
		t.Fatalf("clear 应清空 3 个，实际 %d", out.Cleared)
	}
	for _, id := range []string{"a", "b", "c"} {
		if got := s.Store.Account(id).Proxy; got != "" {
			t.Fatalf("%s 的代理未清空: %q", id, got)
		}
	}
}

// 未配置代理池时应给出可执行的提示，而不是静默成功。
func TestAssignProxyWithoutPool(t *testing.T) {
	t.Setenv("PROXY_POOL", "")
	t.Setenv("DEEPAI_REGISTER_PROXY_POOL", "")
	t.Setenv("PROXY_POOL_FILE", "")
	s := newEmptyServer(t)
	s.Store.UpsertAccount(&deepai.Account{ID: "a", Name: "a", APIKey: "k-a", Enabled: true})

	rec := postAdmin(t, s, "/admin/api/accounts", `{"action":"assign-proxy"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("状态码 = %d：%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "PROXY_POOL") {
		t.Fatalf("错误信息应提示 PROXY_POOL：%s", rec.Body.String())
	}
}
