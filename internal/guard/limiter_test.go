package guard

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestTokenBucketRefill 验证令牌按时间补充（而不是永久封死）。
func TestTokenBucketRefill(t *testing.T) {
	l := NewLimiter()
	if ok, _ := l.Allow("k", 20, 1); !ok {
		t.Fatal("桶初始应有一个令牌")
	}
	if ok, _ := l.Allow("k", 20, 1); ok {
		t.Fatal("令牌用尽后应立即拒绝")
	}
	// 20 rps 下约 50ms 补一个令牌；留足余量。
	time.Sleep(120 * time.Millisecond)
	if ok, _ := l.Allow("k", 20, 1); !ok {
		t.Fatal("等待后应按速率补充令牌")
	}
}

// TestGateConcurrency 验证单密钥并发闸门与名额归还。
func TestGateConcurrency(t *testing.T) {
	l := NewLimiter()
	cfg := RateConfig{Concurrency: 1}

	release, ok, _ := l.Gate("a", cfg)
	if !ok {
		t.Fatal("第一个请求应拿到并发名额")
	}
	if _, ok, _ := l.Gate("a", cfg); ok {
		t.Fatal("同一密钥的第二个并发请求应被拒绝")
	}
	// 不同密钥互不影响。
	releaseB, okB, _ := l.Gate("b", cfg)
	if !okB {
		t.Fatal("不同密钥不应共享并发名额")
	}
	release()
	releaseB()
	if _, ok, _ := l.Gate("a", cfg); !ok {
		t.Fatal("名额归还后应能再次获取")
	}
}

// TestGateRateLimited 验证 429 语义：Retry-After 至少 1 秒、响应体是
// OpenAI 兼容的 rate_limit_error。
func TestGateRateLimited(t *testing.T) {
	l := NewLimiter()
	cfg := RateConfig{RPS: 1, Burst: 1}
	if _, ok, _ := l.Gate("k", cfg); !ok {
		t.Fatal("第一个请求应放行")
	}
	if _, ok, retryAfter := l.Gate("k", cfg); ok {
		t.Fatal("突发放完应立即限流")
	} else if retryAfter < time.Second {
		t.Fatalf("Retry-After 应至少 1s，实际 %s", retryAfter)
	}

	w := httptest.NewRecorder()
	WriteRateLimited(w, time.Second)
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("状态码 = %d，期望 429", w.Code)
	}
	if ra := w.Header().Get("Retry-After"); ra != "1" {
		t.Fatalf("Retry-After = %q，期望 1", ra)
	}
	if body := w.Body.String(); !strings.Contains(body, "rate_limit_error") {
		t.Fatalf("响应体应为 rate_limit_error：%s", body)
	}
}

// TestTokenBucketSweepsIdleEntries 覆盖桶表回收：密钥轮换后旧桶不能永久驻留。
//
// 桶按密钥指纹索引，密钥删除 / 轮换不会主动通知限流器。没有回收时，一个长期
// 运行的网关会随着密钥历史线性涨内存（每条几百字节，规模不大但只增不减）。
func TestTokenBucketSweepsIdleEntries(t *testing.T) {
	l := NewLimiter()

	// 造满一桌闲置桶：直接把 last 拨到 TTL 之外，避免测试里等 30 分钟。
	for i := 0; i < limiterMaxBuckets; i++ {
		id := "old-" + string(rune('a'+i%26)) + string(rune('a'+i/26))
		l.Allow(id, 1, 1)
	}
	l.mu.Lock()
	for _, b := range l.buckets {
		b.last = time.Now().Add(-2 * limiterIdleTTL)
	}
	l.mu.Unlock()

	// 再建一个新桶会触发回收。
	if ok, _ := l.Allow("fresh", 1, 1); !ok {
		t.Fatal("新密钥应能正常限流")
	}
	l.mu.Lock()
	n := len(l.buckets)
	_, kept := l.buckets["fresh"]
	l.mu.Unlock()

	if n > limiterMaxBuckets {
		t.Fatalf("桶表未回收：%d 个（上限 %d）", n, limiterMaxBuckets)
	}
	if !kept {
		t.Fatal("新桶不应被回收")
	}

	// 有在途请求的桶不回收：它的 tokens/last 仍在参与并发判定。
	l.mu.Lock()
	l.buckets["busy"] = &tokenBucket{tokens: 1, last: time.Now().Add(-2 * limiterIdleTTL)}
	l.mu.Unlock()
	l.acquire("busy", 1)
	for i := 0; i < limiterMaxBuckets; i++ {
		l.Allow("fill-"+string(rune('a'+i%26))+string(rune('a'+i/26)), 1, 1)
	}
	l.mu.Lock()
	_, stillThere := l.buckets["busy"]
	l.mu.Unlock()
	l.release("busy", 1)
	if !stillThere {
		t.Fatal("有在途请求的桶不应被回收")
	}
}
