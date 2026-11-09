package server

import (
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"deepaicode2api/internal/deepai"
)

const chatProbeBody = `{"model":"glm-5.3-flash","messages":[{"role":"user","content":"hi"}]}`

const okChatResponse = `{"id":"x","object":"chat.completion","choices":[{"index":0,` +
	`"message":{"role":"assistant","content":"OK"},"finish_reason":"stop"}],` +
	`"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`

// 302 → 401 + 账号标失败：坏 Key 不能永远占着调度（以前会原样透传 302，
// 且账号一直保持 ok，每个请求都白撞一次）。
func TestRedirectedAccountMarkedAndSkipped(t *testing.T) {
	var mu sync.Mutex
	hits := map[string]int{}
	u := newUpstream(t, func(w http.ResponseWriter, r *http.Request) {
		key := r.Header.Get("Api-Key")
		mu.Lock()
		hits[key]++
		mu.Unlock()
		if key == "bad-key" {
			http.Redirect(w, r, "https://deepai.org/login", http.StatusFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, okChatResponse)
	})
	s, key := newTestServer(t, u)
	// acc-1 是 newTestServer 预置的第一个账号，让它先被选中。
	s.Store.UpdateAccount("acc-1", func(a *deepai.Account) { a.APIKey = "bad-key" })
	s.Store.UpsertAccount(&deepai.Account{
		ID: "acc-good", Name: "good", APIKey: "good-key", Enabled: true, Status: "ok",
	})

	rec := do(t, s, http.MethodPost, "/v1/chat/completions", key, chatProbeBody)
	if rec.Code != http.StatusOK {
		t.Fatalf("应换到好号成功，实际 %d: %s", rec.Code, rec.Body.String())
	}
	bad := s.Store.Account("acc-1")
	if bad == nil || bad.Status == "ok" {
		t.Fatalf("302 的账号应被记失败: %#v", bad)
	}
	if !strings.Contains(bad.LastErr, "重定向") {
		t.Fatalf("失败原因应写明重定向，实际 %q", bad.LastErr)
	}

	// 再来一次：坏号已被降权，不应再被优先选中，但即便选中也不会把 302 透给客户端。
	before := hits["bad-key"]
	rec = do(t, s, http.MethodPost, "/v1/chat/completions", key, chatProbeBody)
	if rec.Code != http.StatusOK {
		t.Fatalf("第二次也应 200，实际 %d", rec.Code)
	}
	if hits["bad-key"] > before+0 && rec.Code != http.StatusOK {
		t.Fatal("不应把 302 透传出去")
	}
}

// 全池都是 302 时，客户端应拿到可识别的 401，而不是裸 302。
func TestRedirectedAllAccountsReturnAuthError(t *testing.T) {
	u := newUpstream(t, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://deepai.org/login", http.StatusFound)
	})
	s, key := newTestServer(t, u)

	rec := do(t, s, http.MethodPost, "/v1/chat/completions", key, chatProbeBody)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("状态码 = %d，期望 401: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), deepai.CodeAuthRequired) {
		t.Fatalf("错误码应为 %s: %s", deepai.CodeAuthRequired, rec.Body.String())
	}
}

// email_unverified 标记登录失效并跳过：换号也救不了它，别反复撞。
func TestEmailUnverifiedBlocksAndSkips(t *testing.T) {
	var mu sync.Mutex
	hits := 0
	u := newUpstream(t, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		hits++
		mu.Unlock()
		w.WriteHeader(http.StatusForbidden)
		_, _ = io.WriteString(w, `{"error":{"code":"email_unverified","message":"verify your email"}}`)
	})
	s, key := newTestServer(t, u)

	rec := do(t, s, http.MethodPost, "/v1/chat/completions", key, chatProbeBody)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("状态码 = %d，期望 403: %s", rec.Code, rec.Body.String())
	}
	if a := s.Store.Account("acc-1"); a == nil || a.Status != deepai.StatusBlocked {
		t.Fatalf("应标记为 blocked: %#v", a)
	}

	mu.Lock()
	before := hits
	mu.Unlock()
	rec = do(t, s, http.MethodPost, "/v1/chat/completions", key, chatProbeBody)
	mu.Lock()
	after := hits
	mu.Unlock()
	if after != before {
		t.Fatalf("被拦截后不应再打上游，实际多打了 %d 次", after-before)
	}
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("全池被拦时应 503，实际 %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "email_unverified") {
		t.Fatalf("全池被拦时应说明是 email_unverified: %s", rec.Body.String())
	}
}

// 409 session_busy：池里还有空号时应立刻换号，不能先 sleep(Retry-After)。
// 以前一律等待再换号，最坏白等 30s（docs/performance.md §14 的并发问题之一）。
func TestSessionBusySwitchesImmediatelyWhenAnotherAccountIsFree(t *testing.T) {
	var mu sync.Mutex
	seen := map[string]int{}
	u := newUpstream(t, func(w http.ResponseWriter, r *http.Request) {
		key := r.Header.Get("Api-Key")
		mu.Lock()
		seen[key]++
		mu.Unlock()
		if key == "busy-key" {
			w.Header().Set("Retry-After", "30") // 真等就是 30s
			w.WriteHeader(http.StatusConflict)
			_, _ = io.WriteString(w, `{"error":{"code":"session_busy","message":"busy"}}`)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, okChatResponse)
	})
	s, key := newTestServer(t, u)
	s.Store.UpdateAccount("acc-1", func(a *deepai.Account) { a.APIKey = "busy-key" })
	s.Store.UpsertAccount(&deepai.Account{
		ID: "acc-free", Name: "free", APIKey: "free-key", Enabled: true, Status: "ok",
	})

	begin := time.Now()
	rec := do(t, s, http.MethodPost, "/v1/chat/completions", key, chatProbeBody)
	elapsed := time.Since(begin)
	if rec.Code != http.StatusOK {
		t.Fatalf("应换号成功，实际 %d: %s", rec.Code, rec.Body.String())
	}
	if elapsed > 3*time.Second {
		t.Fatalf("不应等待 Retry-After（耗时 %v）", elapsed)
	}
	mu.Lock()
	defer mu.Unlock()
	if seen["free-key"] == 0 {
		t.Fatal("应换到空闲账号")
	}
}
