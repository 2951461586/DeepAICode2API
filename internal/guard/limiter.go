// Package guard 是网关的「准入与来源校验」层：数据面的按密钥限流、管理口令的
// 失败退避、以及 CORS 来源判定。
//
// 这一层只有内存态（令牌桶 / 在途计数 / 失败表），不持久化、不碰路由表与状态
// 文件；唯一的外部依赖是 internal/deepai 的密钥指纹与 internal/httpx 的错误体。
// 因此可以脱离网关单测与压测（见 limiter_test.go / adminauth_test.go）。
package guard

import (
	"net/http"
	"strconv"
	"sync"
	"time"

	"deepaicode2api/internal/httpx"
	"deepaicode2api/internal/model"
)

// KeyID 把密钥映射成限流桶的键。
//
// 用指纹而不是明文：限流器是长期驻留内存的 map，没必要把完整密钥留在里面。
func KeyID(key string) string { return model.KeyFingerprint(key) }

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

// RateConfig 是一把密钥当前生效的限流参数（各项 0 = 不限）。
type RateConfig struct {
	RPS         float64 // 令牌桶速率（每秒请求数）
	Burst       int     // 突发容量
	Concurrency int     // 单密钥在途请求上限
}

// Limiter 是进程内的按密钥限流器。
//
// 不持久化：限流窗口是秒级的，重启清零没有意义，也避免把运行态写进状态文件。
type Limiter struct {
	mu       sync.Mutex
	buckets  map[string]*tokenBucket
	inflight map[string]int
}

type tokenBucket struct {
	tokens float64
	last   time.Time
}

const (
	// limiterMaxBuckets 是桶表规模上限：超过后先回收闲置桶再新建。
	//
	// 桶按密钥指纹索引，密钥轮换 / 删除后旧桶不会被自动引用；不限规模时
	// 一个长期运行的网关会把这些「历史密钥」的桶一直留在内存里。
	limiterMaxBuckets = 512
	// limiterIdleTTL 是桶的闲置回收时长。
	//
	// 比令牌桶填满所需时间长得多：闲置 30 分钟的桶早已回满，直接丢掉等价于
	// 「重新建一个满桶」，不会给调用方多放请求。
	limiterIdleTTL = 30 * time.Minute
)

// NewLimiter 返回一个不限流的限流器（桶与在途表按需增长）。
func NewLimiter() *Limiter {
	return &Limiter{
		buckets:  map[string]*tokenBucket{},
		inflight: map[string]int{},
	}
}

// Allow 按 rps/burst 做令牌桶判定；rps <= 0 表示不限速。
//
// 返回 retryAfter 供 429 的 Retry-After 头使用（向上取整到秒，至少 1s）。
func (l *Limiter) Allow(id string, rps float64, burst int) (bool, time.Duration) {
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
		// 表满时先回收闲置桶（持锁，见 sweepLocked），避免密钥轮换把内存顶大。
		if len(l.buckets) >= limiterMaxBuckets {
			l.sweepLocked(now)
		}
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

// sweepLocked 回收长期未用的桶（调用方持锁）。
//
// 有在途请求的桶不回收：它的 tokens/last 还参与着并发限流的判定。
func (l *Limiter) sweepLocked(now time.Time) {
	for id, b := range l.buckets {
		if now.Sub(b.last) <= limiterIdleTTL {
			continue
		}
		if _, busy := l.inflight[id]; busy {
			continue
		}
		delete(l.buckets, id)
	}
}

// acquire 尝试占用一个并发名额；limit <= 0 表示不限。
func (l *Limiter) acquire(id string, limit int) bool {
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
func (l *Limiter) release(id string, limit int) {
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

// Gate 对一次新请求做限流判定：先过令牌桶，再占并发名额。
//
// 通过时返回 release（调用方 defer 归还名额）；被拒时 ok=false，
// retryAfter 供 429 的 Retry-After 使用。
func (l *Limiter) Gate(id string, cfg RateConfig) (release func(), ok bool, retryAfter time.Duration) {
	if allowed, wait := l.Allow(id, cfg.RPS, cfg.Burst); !allowed {
		return nil, false, wait
	}
	if !l.acquire(id, cfg.Concurrency) {
		return nil, false, time.Second
	}
	limit := cfg.Concurrency
	return func() { l.release(id, limit) }, true, 0
}

// WriteRateLimited 写 429（OpenAI 兼容的 rate_limit_error）。
func WriteRateLimited(w http.ResponseWriter, retryAfter time.Duration) {
	secs := int(retryAfter.Round(time.Second) / time.Second)
	if secs < 1 {
		secs = 1
	}
	w.Header().Set("Retry-After", strconv.Itoa(secs))
	httpx.WriteErr(w, http.StatusTooManyRequests,
		"请求过于频繁（该 API 密钥已触发限流），请在 "+strconv.Itoa(secs)+" 秒后重试", "rate_limit_error")
}
