package server

import (
	"context"
	"io"
	"net/http"
	"testing"

	"deepaicode2api/internal/deepai"
)

// modelsForWarmup 是 /models 的最小合法响应：FetchModels 要求非空目录。
const modelsForWarmup = `{"models":[{"id":"m1","name":"M1","reasoning":false,` +
	`"input":["text"],"contextWindow":1000,"maxTokens":100,"theosPerRequest":"0"}],"default":"m1"}`

// newWarmupUpstream 起一个只认 /models 的假上游。
func newWarmupUpstream(t *testing.T) *upstream {
	t.Helper()
	return newUpstream(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != deepai.ModelBasePath+deepai.PathModels {
			w.WriteHeader(http.StatusTeapot)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, modelsForWarmup)
	})
}

func countPath(u *upstream, path string) int {
	u.mu.Lock()
	defer u.mu.Unlock()
	n := 0
	for _, r := range u.requests {
		if r.Path == path {
			n++
		}
	}
	return n
}

// TestWarmUpstreamPrimesDistinctEgresses 锁住「同一出口只预热一次」：
// 号池里 14 个账号往往只有几个出口，按账号预热会把代理打爆并拖慢启动。
func TestWarmUpstreamPrimesDistinctEgresses(t *testing.T) {
	u := newWarmupUpstream(t)
	s, _ := newTestServer(t, u)

	// 再补两个「直连」账号 + 一个停用账号：出口集合仍然是 {""}。
	for _, id := range []string{"acc-2", "acc-3"} {
		s.Store.UpsertAccount(&deepai.Account{ID: id, Name: id, APIKey: "k-" + id, Enabled: true, Status: "ok"})
	}
	s.Store.UpsertAccount(&deepai.Account{ID: "acc-off", Name: "off", APIKey: "k-off", Enabled: false})

	if got := s.WarmUpstream(context.Background()); got != 1 {
		t.Fatalf("预热出口数 = %d，期望 1（直连只有一个）", got)
	}
	if got := countPath(u, deepai.ModelBasePath+deepai.PathModels); got != 1 {
		t.Fatalf("/models 请求数 = %d，期望 1", got)
	}
}

// TestWarmUpstreamEmptyPool 空号池不应发任何请求，也不该报错。
func TestWarmUpstreamEmptyPool(t *testing.T) {
	u := newWarmupUpstream(t)
	s, _ := newTestServer(t, u)
	// Accounts() 返回副本，必须走 store 的变更入口才能真的停用。
	s.Store.UpdateAccounts(func(a *deepai.Account) bool {
		a.Enabled = false
		return true
	})

	if got := s.WarmUpstream(context.Background()); got != 0 {
		t.Fatalf("空号池预热数 = %d，期望 0", got)
	}
	if got := countPath(u, deepai.ModelBasePath+deepai.PathModels); got != 0 {
		t.Fatalf("空号池不应发请求，实际 %d 次", got)
	}
}

// TestWarmUpstreamFailureIsSilent 上游挂掉时预热只返回 0，不 panic、不阻塞。
func TestWarmUpstreamFailureIsSilent(t *testing.T) {
	u := newUpstream(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	s, _ := newTestServer(t, u)

	if got := s.WarmUpstream(context.Background()); got != 0 {
		t.Fatalf("上游 500 时预热数 = %d，期望 0", got)
	}
}

// 401 也算预热成功：上游 2026-11-14 起对 /models 要求 Api-Key，而预热只关心
// DNS/TCP/TLS 与代理 CONNECT 是否打通 —— 401 已经证明链路是通的。
func TestWarmUpstreamCountsUnauthorizedAsReachable(t *testing.T) {
	u := newUpstream(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"error":{"code":"auth_required","type":"auth_required"}}`)
	})
	s, _ := newTestServer(t, u)

	if got := s.WarmUpstream(context.Background()); got != 1 {
		t.Fatalf("401 应算预热成功（链路已通），实际 %d", got)
	}
}

// 预热必须带账号凭据：/models 没有 Api-Key 只会 401，带上才真的把目录请求
// 走完整链路（也顺带验证了「按出口挑代表账号」确实拿到了真实账号）。
func TestWarmUpstreamSendsAccountKey(t *testing.T) {
	u := newWarmupUpstream(t)
	s, _ := newTestServer(t, u)

	if got := s.WarmUpstream(context.Background()); got != 1 {
		t.Fatalf("预热数 = %d，期望 1", got)
	}
	if key := u.last(t).Header.Get(deepai.HeaderKeyName); key != "upstream-key" {
		t.Fatalf("预热请求的 Api-Key = %q，期望 upstream-key", key)
	}
}
