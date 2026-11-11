package server

import (
	"net/http"
	"strconv"
	"sync"
	"time"

	"deepaicode2api/internal/deepai"
)

// keyIDOf 把密钥映射成限流桶的键。
//
// 用指纹而不是明文：限流器是长期驻留内存的 map，没必要把完整密钥留在里面。
func keyIDOf(key string) string { return deepai.KeyFingerprint(key) }

// --- 数据面：按密钥限流 ---
//
// 背景：网关对外签发的密钥此前只有「有效 / 无效」两种状态。任何一把密钥都能
// 把整个号池打满——池里的账号是有限的、上游额度按天发放，一个失控脚本就足以
// 让所有调用方一起 429。这里补上两道闸门，默认都是关闭的（0 = 不限），
// 升级不会改变既有行为：
//
//   - 令牌桶限速（每秒请求数 + 突发容量）；
//   - 单密钥并发上限（在途请求数）。
//
// 两者都按「密钥」维度隔离：一个分组 / 一把密钥被限流，不会影响其它密钥。

// keyRateLimiter 是进程内的按密钥限流器。
//
// 不持久化：限流窗口是秒级的，重启清零没有意义，也避免把运行态写进状态文件。
type keyRateLimiter struct {
	mu       sync.Mutex
	buckets  map[string]*tokenBucket
	inflight map[string]int
}

type tokenBucket struct {
	tokens float64
	last   time.Time
}

func newKeyRateLimiter() *keyRateLimiter {
	return &keyRateLimiter{
		buckets:  map[string]*tokenBucket{},
		inflight: map[string]int{},
	}
}

// allow 按 rps/burst 做令牌桶判定；rps <= 0 表示不限速。
//
// 返回 retryAfter 供 429 的 Retry-After 头使用（向上取整到秒，至少 1s）。
func (l *keyRateLimiter) allow(id string, rps float64, burst int) (bool, time.Duration) {
	if rps <= 0 {
		return true, 0
	}
	if burst <= 0 {
		burst = int(rps)
		if burst < 2 {
			burst = 2
		}
	}

	now := time.Now()
	l.mu.Lock()
	defer l.mu.Unlock()

	b := l.buckets[id]
	if b == nil {
		b = &tokenBucket{tokens: float64(burst), last: now}
		l.buckets[id] = b
	}
	// 按经过时间补充令牌，上限为桶容量。
	elapsed := now.Sub(b.last).Seconds()
	if elapsed > 0 {
		b.tokens += elapsed * rps
		if b.tokens > float64(burst) {
			b.tokens = float64(burst)
		}
		b.last = now
	}
	if b.tokens >= 1 {
		b.tokens--
		return true, 0
	}
	// 还差多少秒才能攒够一个令牌。
	wait := time.Duration((1 - b.tokens) / rps * float64(time.Second))
	if wait < time.Second {
		wait = time.Second
	}
	return false, wait
}

// acquire 尝试占用一个并发名额；limit <= 0 表示不限。
func (l *keyRateLimiter) acquire(id string, limit int) bool {
	if limit <= 0 {
		return true
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.inflight[id] >= limit {
		return false
	}
	l.inflight[id]++
	return true
}

// release 归还并发名额。
func (l *keyRateLimiter) release(id string, limit int) {
	if limit <= 0 {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.inflight[id] <= 1 {
		delete(l.inflight, id)
		return
	}
	l.inflight[id]--
}

// rateLimitGate 对新请求做限流判定，通过后把并发名额的归还在返回时完成。
//
// 放在 withAuth 里（校验密钥之后）而不是全局中间件：限流是按密钥隔离的，
// 没有密钥的请求（未签发密钥的本机自举）不该计入任何人的配额。
func (s *Server) rateLimitGate(keyID string) (release func(), ok bool, retryAfter time.Duration) {
	settings := s.Store.Settings()

	if allowed, wait := s.limiter.allow(keyID, settings.RateLimitRPS, settings.RateLimitBurst); !allowed {
		return nil, false, wait
	}
	if !s.limiter.acquire(keyID, settings.KeyConcurrency) {
		return nil, false, time.Second
	}
	limit := settings.KeyConcurrency
	return func() { s.limiter.release(keyID, limit) }, true, 0
}

// writeRateLimited 写 429（OpenAI 兼容的 rate_limit_error）。
func writeRateLimited(w http.ResponseWriter, retryAfter time.Duration) {
	secs := int(retryAfter.Round(time.Second) / time.Second)
	if secs < 1 {
		secs = 1
	}
	w.Header().Set("Retry-After", strconv.Itoa(secs))
	writeErr(w, http.StatusTooManyRequests,
		"请求过于频繁（该 API 密钥已触发限流），请在 "+strconv.Itoa(secs)+" 秒后重试", "rate_limit_error")
}
