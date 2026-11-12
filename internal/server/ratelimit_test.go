package server

import (
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"deepaicode2api/internal/store"
)

// chatUpstreamOK 返回一个总是成功的最小 chat 响应。
func chatUpstreamOK(t *testing.T) *upstream {
	t.Helper()
	return newUpstream(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"gen-1","choices":[{"index":0,"finish_reason":"stop",
			"message":{"role":"assistant","content":"ok"}}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`)
	})
}

const chatBody = `{"model":"gpt-4o-mini","messages":[{"role":"user","content":"hi"}]}`

// TestRateLimitDisabledByDefault 守住「升级不改变既有行为」：
// 未配置限流时，连续请求必须全部成功。
func TestRateLimitDisabledByDefault(t *testing.T) {
	s, key := newTestServer(t, chatUpstreamOK(t))
	for i := 0; i < 10; i++ {
		rec := do(t, s, http.MethodPost, "/v1/chat/completions", key, chatBody)
		if rec.Code != http.StatusOK {
			t.Fatalf("第 %d 次请求 = %d：%s", i, rec.Code, rec.Body.String())
		}
	}
}

// TestRateLimitPerKey 验证令牌桶按密钥生效，且 429 带 Retry-After。
func TestRateLimitPerKey(t *testing.T) {
	s, key := newTestServer(t, chatUpstreamOK(t))
	s.Store.UpdateSettings(func(st *store.Settings) {
		st.RateLimitRPS = 1
		st.RateLimitBurst = 1
	})

	first := do(t, s, http.MethodPost, "/v1/chat/completions", key, chatBody)
	if first.Code != http.StatusOK {
		t.Fatalf("第一次请求应放行，实际 %d：%s", first.Code, first.Body.String())
	}
	second := do(t, s, http.MethodPost, "/v1/chat/completions", key, chatBody)
	if second.Code != http.StatusTooManyRequests {
		t.Fatalf("第二次请求应被限流，实际 %d：%s", second.Code, second.Body.String())
	}
	if ra := second.Header().Get("Retry-After"); ra == "" || ra == "0" {
		t.Fatalf("429 必须带正的 Retry-After，实际 %q", ra)
	}
	if body := second.Body.String(); !strings.Contains(body, "rate_limit_error") {
		t.Fatalf("429 响应体应为 rate_limit_error：%s", body)
	}

	// 另一把密钥不受影响：限流是 per-key 而不是全局。
	other := s.Store.AddKey("other", "").Key
	if rec := do(t, s, http.MethodPost, "/v1/chat/completions", other, chatBody); rec.Code != http.StatusOK {
		t.Fatalf("另一把密钥应不受影响，实际 %d：%s", rec.Code, rec.Body.String())
	}
}

// TestKeyConcurrencyLimit 验证单密钥并发闸门与名额归还。
func TestKeyConcurrencyLimit(t *testing.T) {
	s, _ := newTestServer(t, nil)
	s.Store.UpdateSettings(func(st *store.Settings) { st.KeyConcurrency = 1 })

	release, ok, _ := s.rateLimitGate("fingerprint-a")
	if !ok {
		t.Fatal("第一个请求应拿到并发名额")
	}
	if _, ok, _ := s.rateLimitGate("fingerprint-a"); ok {
		t.Fatal("同一密钥的第二个并发请求应被拒绝")
	}
	// 不同密钥互不影响。
	releaseB, okB, _ := s.rateLimitGate("fingerprint-b")
	if !okB {
		t.Fatal("不同密钥不应共享并发名额")
	}
	release()
	releaseB()
	if _, ok, _ := s.rateLimitGate("fingerprint-a"); !ok {
		t.Fatal("名额归还后应能再次获取")
	}
}

// TestTokenBucketRefill 验证令牌按时间补充（而不是永久封死）。
func TestTokenBucketRefill(t *testing.T) {
	l := newKeyRateLimiter()
	if ok, _ := l.allow("k", 20, 1); !ok {
		t.Fatal("桶初始应有一个令牌")
	}
	if ok, _ := l.allow("k", 20, 1); ok {
		t.Fatal("令牌用尽后应立即拒绝")
	}
	// 20 rps 下约 50ms 补一个令牌；留足余量。
	time.Sleep(120 * time.Millisecond)
	if ok, _ := l.allow("k", 20, 1); !ok {
		t.Fatal("等待后应按速率补充令牌")
	}
}

// TestRateLimitSettingsRoundTrip 验证控制台能保存并读回限流参数（含显式 0）。
func TestRateLimitSettingsRoundTrip(t *testing.T) {
	s, _ := newTestServer(t, nil)

	rec := do(t, s, http.MethodPost, "/admin/api/settings", "", `{"rate_limit_rps":2.5,"rate_limit_burst":5,"key_concurrency":3}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("保存设置失败：%d %s", rec.Code, rec.Body.String())
	}
	st := s.Store.Settings()
	if st.RateLimitRPS != 2.5 || st.RateLimitBurst != 5 || st.KeyConcurrency != 3 {
		t.Fatalf("设置未生效：%+v", st)
	}

	// 显式 0 表示关闭限流，不能被 “<= 0 忽略” 的旧逻辑吃掉。
	do(t, s, http.MethodPost, "/admin/api/settings", "", `{"rate_limit_rps":0,"key_concurrency":0}`)
	st = s.Store.Settings()
	if st.RateLimitRPS != 0 || st.KeyConcurrency != 0 {
		t.Fatalf("显式 0 应关闭限流：%+v", st)
	}
}

// TestTokenBucketSweepsIdleEntries 覆盖桶表回收：密钥轮换后旧桶不能永久驻留。
//
// 桶按密钥指纹索引，密钥删除 / 轮换不会主动通知限流器。没有回收时，一个长期
// 运行的网关会随着密钥历史线性涨内存（每条几百字节，规模不大但只增不减）。
func TestTokenBucketSweepsIdleEntries(t *testing.T) {
	l := newKeyRateLimiter()

	// 造满一桌闲置桶：直接把 last 拨到 TTL 之外，避免测试里等 30 分钟。
	for i := 0; i < keyRateLimiterMaxBuckets; i++ {
		id := "old-" + string(rune('a'+i%26)) + string(rune('a'+i/26))
		l.allow(id, 1, 1)
	}
	l.mu.Lock()
	for _, b := range l.buckets {
		b.last = time.Now().Add(-2 * keyRateLimiterIdleTTL)
	}
	l.mu.Unlock()

	// 再建一个新桶会触发回收。
	if ok, _ := l.allow("fresh", 1, 1); !ok {
		t.Fatal("新密钥应能正常限流")
	}
	l.mu.Lock()
	n := len(l.buckets)
	_, kept := l.buckets["fresh"]
	l.mu.Unlock()

	if n > keyRateLimiterMaxBuckets {
		t.Fatalf("桶表未回收：%d 个（上限 %d）", n, keyRateLimiterMaxBuckets)
	}
	if !kept {
		t.Fatal("新桶不应被回收")
	}

	// 有在途请求的桶不回收：它的 tokens/last 仍在参与并发判定。
	l.mu.Lock()
	l.buckets["busy"] = &tokenBucket{tokens: 1, last: time.Now().Add(-2 * keyRateLimiterIdleTTL)}
	l.mu.Unlock()
	l.acquire("busy", 1)
	for i := 0; i < keyRateLimiterMaxBuckets; i++ {
		l.allow("fill-"+string(rune('a'+i%26))+string(rune('a'+i/26)), 1, 1)
	}
	l.mu.Lock()
	_, stillThere := l.buckets["busy"]
	l.mu.Unlock()
	l.release("busy", 1)
	if !stillThere {
		t.Fatal("有在途请求的桶不应被回收")
	}
}
