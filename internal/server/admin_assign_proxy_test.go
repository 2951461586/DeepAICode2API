package server

import (
	"net/http"
	"testing"

	"deepaicode2api/internal/deepai"
)

// TestAssignProxySpreadsAcrossCalls 固化 2026-11-22 线上的 bug：
// 每次调用都新建代理池 → Next() 永远从第 0 条开始 → **每次分配都落到同一个出口**。
// 游标必须跨调用保留，并且优先选「当前账号数最少」的条目。
func TestAssignProxySpreadsAcrossCalls(t *testing.T) {
	t.Setenv("PROXY_POOL", "http://127.0.0.1:9001,http://127.0.0.1:9002")
	s := newEmptyServer(t)
	for _, id := range []string{"a", "b"} {
		if !s.Store.UpsertAccount(&deepai.Account{ID: id, Name: id, APIKey: "k-" + id, Enabled: true, Status: "ok"}) {
			t.Fatalf("写入账号 %s 失败", id)
		}
	}

	for _, id := range []string{"a", "b"} {
		rec := postAdmin(t, s, "/admin/api/accounts", `{"action":"assign-proxy","id":"`+id+`"}`)
		if rec.Code != http.StatusOK {
			t.Fatalf("分配 %s 失败: %d %s", id, rec.Code, rec.Body.String())
		}
	}

	a, b := s.Store.Account("a"), s.Store.Account("b")
	if a.Proxy == "" || b.Proxy == "" {
		t.Fatalf("代理未写入: a=%q b=%q", a.Proxy, b.Proxy)
	}
	if a.Proxy == b.Proxy {
		t.Fatalf("两次分配到同一个出口（回归）：%q", a.Proxy)
	}

	// 再分配一次（reassign a）：池里只有两个出口，应换到 b 之外……实际是
	// 「账号数最少」——两个出口各 1 个号，轮转后结果仍是二者之一，
	// 这里只要求落在池内且不等于它自己的旧值之外仍然合法。
	rec := postAdmin(t, s, "/admin/api/accounts", `{"action":"assign-proxy","id":"a","reassign":true}`)
	out := decodeJSON[proxyAssignResult](t, rec.Body.Bytes())
	if out.Assigned != 1 || out.InUse != 2 {
		t.Fatalf("reassign 结果 assigned=%d in_use=%d，期望 1 / 2", out.Assigned, out.InUse)
	}
}
