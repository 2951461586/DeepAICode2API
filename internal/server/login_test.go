package server

import (
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
