package server

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"deepaicode2api/internal/store"
)

// 管理口令登录页（/login）与它的校验入口（/admin/api/auth）。
//
// 回归背景：控制台顶栏原本直接摆一个口令输入框，口令在屏幕上、在长驻页面上
// 一直可见。改成独立登录页后，这条链路不能断：页面能打开、校验能生效、
// 未设口令时本机不挡人。

func TestLoginPageServed(t *testing.T) {
	u := newWarmupUpstream(t)
	s, _ := newTestServer(t, u)

	rec := do(t, s, http.MethodGet, "/login", "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("/login HTTP %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "text/html") {
		t.Fatalf("/login Content-Type = %q", ct)
	}
	body := rec.Body.String()
	for _, want := range []string{`id="loginForm"`, `id="loginPw"`, `id="loginErr"`, `/js/login.js`} {
		if !strings.Contains(body, want) {
			t.Errorf("/login 缺少 %s", want)
		}
	}
	// 登录页不该把控制台的表单带出来（两页职责不同，混在一起会让人以为
	// 没登录也能直接改数据）。
	if strings.Contains(body, `id="acctTable"`) {
		t.Error("/login 不应包含控制台账号表")
	}
}

func TestLoginScriptServed(t *testing.T) {
	u := newWarmupUpstream(t)
	s, _ := newTestServer(t, u)

	rec := do(t, s, http.MethodGet, "/js/login.js", "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("/js/login.js HTTP %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "/admin/api/auth") {
		t.Error("login.js 应调用 /admin/api/auth 校验口令")
	}
}

func TestAdminAuthEndpoint(t *testing.T) {
	u := newWarmupUpstream(t)
	s, _ := newTestServer(t, u)

	// 未设管理口令：本机请求直接放行（否则控制台永远进不去）。
	if rec := do(t, s, http.MethodPost, "/admin/api/auth", "", "{}"); rec.Code != http.StatusOK {
		t.Fatalf("未设口令时应 200，实际 %d", rec.Code)
	}

	s.Store.UpdateSettings(func(st *store.Settings) { st.AdminPassword = "s3cret" })
	if rec := do(t, s, http.MethodPost, "/admin/api/auth", "", "{}"); rec.Code != http.StatusUnauthorized {
		t.Fatalf("缺口令应 401，实际 %d", rec.Code)
	}
	// do() 会把口令放进 Authorization: Bearer。
	if rec := do(t, s, http.MethodPost, "/admin/api/auth", "wrong", "{}"); rec.Code != http.StatusUnauthorized {
		t.Fatalf("错口令应 401，实际 %d", rec.Code)
	}
	if rec := do(t, s, http.MethodPost, "/admin/api/auth", "s3cret", "{}"); rec.Code != http.StatusOK {
		t.Fatalf("对口令应 200，实际 %d", rec.Code)
	}
	// 返回体里不能带口令本身。
	var body map[string]any
	rec := do(t, s, http.MethodPost, "/admin/api/auth", "s3cret", "{}")
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("解析: %v", err)
	}
	if len(body) != 1 || body["ok"] != true {
		t.Fatalf("返回体只应有 {ok:true}，实际 %#v", body)
	}
}

func TestHealthExposesAdminRequired(t *testing.T) {
	u := newWarmupUpstream(t)
	s, _ := newTestServer(t, u)

	read := func() bool {
		rec := do(t, s, http.MethodGet, "/health", "", "")
		var body map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("解析 /health: %v", err)
		}
		v, ok := body["admin_required"].(bool)
		if !ok {
			t.Fatalf("/health 缺少 admin_required 字段：%#v", body)
		}
		return v
	}

	if read() {
		t.Fatal("未设口令时 admin_required 应为 false")
	}
	s.Store.UpdateSettings(func(st *store.Settings) { st.AdminPassword = "s3cret" })
	if !read() {
		t.Fatal("设了口令后 admin_required 应为 true")
	}
}
