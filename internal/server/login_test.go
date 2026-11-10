package server

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"deepaicode2api/internal/deepai"
)

// 浏览器登录（含 Google 登录）回调只换来 API Key，不带机器指纹。
// 入池时必须补一个独立 UUID：上游按机器发每日免费额度，沿用本机指纹会让
// 多个号共用同一份额度（只有第一个能领到）。
func TestLoginPollAssignsMachineID(t *testing.T) {
	u := newUpstream(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/theos") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"enabled":true,"balance":"200","granted":"200","lane_version":4}`)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})
	s, _ := newTestServer(t, u)

	sess := &loginSession{ID: "sid", State: "st", Created: time.Now()}
	sess.setResult(&loginResult{Creds: &deepai.Credentials{APIKey: "oauth-key", Username: "google-user"}})
	s.Login.put(sess)

	rec := do(t, s, http.MethodPost, "/admin/api/login/poll", "", `{"session_id":"sid"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("login/poll 应 200，实际 %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"status":"ok"`) {
		t.Fatalf("login/poll 应返回 status=ok: %s", rec.Body.String())
	}

	var acct *deepai.Account
	for _, a := range s.Store.Accounts() {
		if a.APIKey == "oauth-key" {
			acct = a
			break
		}
	}
	if acct == nil {
		t.Fatal("浏览器登录的账号没有入池")
	}
	if acct.Source != "oauth" {
		t.Errorf("来源应为 oauth，实际 %q", acct.Source)
	}
	if strings.TrimSpace(acct.MachineID) == "" {
		t.Fatal("浏览器登录入池未分配独立机器指纹")
	}
}

// 浏览器登录入池时若配置了代理池（PROXY_POOL / PROXY_POOL_FILE），要按轮询
// 分配出口：同一直连 IP 的每日免费额度是共享的。这里在同一 Server 上连续两次
// 入池，验证游标在进程内持续前进（而不是每次都从第一个代理重新开始）。
func TestLoginPollAssignsProxyFromPool(t *testing.T) {
	t.Setenv("PROXY_POOL", "http://127.0.0.1:1,http://127.0.0.1:2")

	u := newUpstream(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/theos") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"enabled":true,"balance":"200","granted":"200","lane_version":4}`)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})
	s, _ := newTestServer(t, u)

	poll := func(id, key string) *deepai.Account {
		t.Helper()
		sess := &loginSession{ID: id, State: "st-" + id, Created: time.Now()}
		sess.setResult(&loginResult{Creds: &deepai.Credentials{APIKey: key, Username: "user-" + id}})
		s.Login.put(sess)
		rec := do(t, s, http.MethodPost, "/admin/api/login/poll", "", `{"session_id":"`+id+`"}`)
		if rec.Code != http.StatusOK {
			t.Fatalf("login/poll(%s) 应 200，实际 %d: %s", id, rec.Code, rec.Body.String())
		}
		for _, a := range s.Store.Accounts() {
			if a.APIKey == key {
				return a
			}
		}
		t.Fatalf("账号 %s 未入池", key)
		return nil
	}

	a1 := poll("sid-1", "oauth-key-1")
	a2 := poll("sid-2", "oauth-key-2")
	if a1.Proxy == "" || a2.Proxy == "" {
		t.Fatalf("oauth 入池应自动分配代理，实际 %q / %q", a1.Proxy, a2.Proxy)
	}
	if a1.Proxy == a2.Proxy {
		t.Fatalf("代理池应轮询到不同出口，实际都是 %q", a1.Proxy)
	}
}

// 假上游：兑换 code，其余路径按需响应。
func loginUpstream(t *testing.T, apiKey, username string) *upstream {
	t.Helper()
	return newUpstream(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/desktop_auth/exchange/"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"api_key":"`+apiKey+`","username":"`+username+`","is_pro":false}`)
		case strings.HasSuffix(r.URL.Path, "/theos"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"enabled":true,"balance":"200","granted":"200","lane_version":4}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
}

// 启动一次登录会话并返回 (session_id, state)。
func startLogin(t *testing.T, s *Server) (string, string) {
	t.Helper()
	rec := do(t, s, http.MethodPost, "/admin/api/login/start", "", `{}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("login/start 应 200，实际 %d: %s", rec.Code, rec.Body.String())
	}
	var out struct {
		SessionID string `json:"session_id"`
		Port      int    `json:"port"`
		Loopback  bool   `json:"loopback"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("解析 login/start 响应: %v", err)
	}
	if out.SessionID == "" || out.Port <= 0 {
		t.Fatalf("login/start 响应缺少 session_id/port: %s", rec.Body.String())
	}
	if !out.Loopback {
		t.Fatalf("测试环境应能绑定本机回环，实际 loopback=false: %s", rec.Body.String())
	}
	sess := s.Login.get(out.SessionID)
	if sess == nil {
		t.Fatal("login/start 没有登记会话")
	}
	return out.SessionID, sess.State
}

// Docker / 远程部署时浏览器回调打不开，用户把地址栏 URL 粘回控制台，
// 由网关用 code 兑换并完成入池 —— 等价于本机回调那条路。
func TestLoginCallbackPasteCompletesLogin(t *testing.T) {
	u := loginUpstream(t, "pasted-key", "pasted-user")
	s, _ := newTestServer(t, u)

	sid, state := startLogin(t, s)
	cb := "http://127.0.0.1:34105/auth-callback?code=bmQ6lztT61GCm9ED_bcN&state=" + state
	rec := do(t, s, http.MethodPost, "/admin/api/login/callback", "",
		`{"session_id":"`+sid+`","callback":"`+cb+`"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("login/callback 应 200，实际 %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"status":"ok"`) {
		t.Fatalf("应返回 status=ok: %s", rec.Body.String())
	}

	var acct *deepai.Account
	for _, a := range s.Store.Accounts() {
		if a.APIKey == "pasted-key" {
			acct = a
			break
		}
	}
	if acct == nil {
		t.Fatal("粘贴回调的账号没有入池")
	}
	if acct.Source != "oauth" {
		t.Errorf("来源应为 oauth，实际 %q", acct.Source)
	}
	if strings.TrimSpace(acct.MachineID) == "" {
		t.Error("粘贴回调入池未分配独立机器指纹")
	}
	if s.Login.get(sid) != nil {
		t.Error("登录完成后会话应被删除")
	}

	// code 是一次性的：同一个 code 再粘一次不能重复入池。
	again := do(t, s, http.MethodPost, "/admin/api/login/callback", "",
		`{"session_id":"`+sid+`","callback":"`+cb+`"}`)
	if again.Code == http.StatusOK {
		t.Fatalf("同一会话二次粘贴不应成功: %s", again.Body.String())
	}
}

// 只接受「本次登录」的回调：state 不匹配必须拒绝（防止贴错标签页）。
func TestLoginCallbackRejectsForeignState(t *testing.T) {
	u := loginUpstream(t, "k", "u")
	s, _ := newTestServer(t, u)

	sid, _ := startLogin(t, s)
	cb := "http://127.0.0.1:34105/auth-callback?code=abc&state=deadbeef"
	rec := do(t, s, http.MethodPost, "/admin/api/login/callback", "",
		`{"session_id":"`+sid+`","callback":"`+cb+`"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("state 不匹配应 400，实际 %d: %s", rec.Code, rec.Body.String())
	}
	if len(s.Store.Accounts()) != 1 {
		t.Errorf("被拒的回调不应入池，账号数 = %d", len(s.Store.Accounts()))
	}
}

// 只粘 code 拿不到 state，无法确认它属于本次登录：直接拒绝并给出提示。
func TestLoginCallbackRejectsBareCode(t *testing.T) {
	u := loginUpstream(t, "k", "u")
	s, _ := newTestServer(t, u)

	sid, _ := startLogin(t, s)
	rec := do(t, s, http.MethodPost, "/admin/api/login/callback", "",
		`{"session_id":"`+sid+`","callback":"bmQ6lztT61GCm9ED_bcN-AhOdT2GzgAdHC2iJvT-Bxc"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("只粘 code 应 400，实际 %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "state") {
		t.Errorf("错误信息应提示缺少 state: %s", rec.Body.String())
	}
}

// 兑换失败（code 过期 / 已用过）要给可读提示，并结束会话。
func TestLoginCallbackExchangeFailure(t *testing.T) {
	u := newUpstream(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/desktop_auth/exchange/") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, `{"error":{"code":"invalid_code","message":"expired"}}`)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})
	s, _ := newTestServer(t, u)

	sid, state := startLogin(t, s)
	cb := "http://127.0.0.1:34105/auth-callback?code=abc&state=" + state
	rec := do(t, s, http.MethodPost, "/admin/api/login/callback", "",
		`{"session_id":"`+sid+`","callback":"`+cb+`"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("兑换失败应 400，实际 %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "只能兑换一次") {
		t.Errorf("应提示 code 只能用一次: %s", rec.Body.String())
	}
	if s.Login.get(sid) != nil {
		t.Error("兑换失败后会话应被清理")
	}
}
