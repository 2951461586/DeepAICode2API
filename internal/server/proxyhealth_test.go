package server

import (
	"net/http"
	"testing"

	"deepaicode2api/internal/deepai"
)

// TestProxyHealthTracksTransportFailures 固化出口台账的接线：
// 传输失败进入冷却、拿到响应清零、直连（无代理）不记账。
func TestProxyHealthTracksTransportFailures(t *testing.T) {
	s, _ := newTestServer(t, nil)
	viaProxy := &deepai.Account{ID: "a1", Proxy: "http://peer:1"}
	direct := &deepai.Account{ID: "a2"}

	if s.proxyCooling(viaProxy) || s.ProxyCoolingCount() != 0 {
		t.Fatal("初始不应有冷却出口")
	}
	s.noteProxyResult(viaProxy, false)
	if !s.proxyCooling(viaProxy) {
		t.Fatal("传输失败后出口应进入冷却")
	}
	if s.ProxyCoolingCount() != 1 {
		t.Fatalf("冷却出口数 = %d，期望 1", s.ProxyCoolingCount())
	}
	if !s.avoidCoolingProxy(viaProxy) {
		t.Fatal("冷却中的出口应被调度谓词排除")
	}

	s.noteProxyResult(viaProxy, true)
	if s.proxyCooling(viaProxy) || s.ProxyCoolingCount() != 0 {
		t.Fatal("一次成功应立刻解除冷却")
	}

	// 直连不记账：空 proxy 不是「一个出口」。
	s.noteProxyResult(direct, false)
	if s.ProxyCoolingCount() != 0 {
		t.Fatal("直连不应进入出口台账")
	}
	// nil 安全：账号为空 / 服务未初始化时都不 panic。
	s.noteProxyResult(nil, false)
	var nilServer *Server
	nilServer.noteProxyResult(viaProxy, false)
	if nilServer.ProxyCoolingCount() != 0 {
		t.Fatal("nil 服务应返回 0")
	}
}

// TestLooksLikeHTML 固化「HTML 错误页 = 出口侧拦截」的判据：
// 只看首个非空白字节，避免把上游 JSON 错误误判成出口问题。
func TestLooksLikeHTML(t *testing.T) {
	cases := map[string]bool{
		"<html><head><title>403 Forbidden</title>": true,
		"\n  <!DOCTYPE html>":                      true,
		`{"error":{"code":"rate_limit_exceeded"}}`: false,
		"":        false,
		"   \n\t": false,
	}
	for body, want := range cases {
		if got := looksLikeHTML([]byte(body)); got != want {
			t.Fatalf("looksLikeHTML(%q) = %v，期望 %v", body, got, want)
		}
	}
}

// TestWAF403CoolsExitWithoutBlankingAccount 固化 2026-11-22 的取舍：
// 上游用 HTML 403（WAF / 代理网关）拒绝时，如果账号绑了专属出口，
// 那是**出口级**故障 —— 只冷却出口，不把账号标 error（否则池容量白掉）。
func TestWAF403CoolsExitWithoutBlankingAccount(t *testing.T) {
	u := newUpstream(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte("<html><head><title>403 Forbidden</title></head><body>nginx/1.24.0</body></html>"))
	})
	s, key := newTestServer(t, u)
	if !s.Store.UpdateAccount("acc-1", func(a *deepai.Account) { a.Proxy = "http://127.0.0.1:9" }) {
		t.Fatal("设置代理失败")
	}
	acct := s.Store.Account("acc-1")

	rec := do(t, s, http.MethodPost, "/v1/chat/completions", key,
		`{"model":"glm-5.3-flash","messages":[{"role":"user","content":"hi"}],"stream":false}`)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("状态码 = %d，期望 403（出口被拦，错误原样回传）", rec.Code)
	}
	if got := s.Store.Account("acc-1").Status; got != "ok" {
		t.Fatalf("账号状态 = %q，出口级故障不应把账号标 error", got)
	}
	if !s.proxyCooling(acct) {
		t.Fatal("被 WAF 拦的出口应进入冷却")
	}
	if s.ProxyCoolingCount() != 1 {
		t.Fatalf("冷却出口数 = %d，期望 1", s.ProxyCoolingCount())
	}
}
