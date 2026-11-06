package register

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func withFastVerifyRetry(t *testing.T, d ...time.Duration) {
	t.Helper()
	saved := verifyRetryDelays
	verifyRetryDelays = d
	t.Cleanup(func() { verifyRetryDelays = saved })
}

func newVerifyWeb(t *testing.T, h http.HandlerFunc) *WebClient {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return &WebClient{Origin: srv.URL, Site: srv.URL, HTTP: srv.Client()}
}

// 验证点击必须限并发：上游 /verify_email 单次要 13s 服务端时间，
// 并发打过去会把 Passenger worker 池打满（第八批 0/50 的根因）。
func TestVerifyEmailThrottledCapsConcurrency(t *testing.T) {
	var inFlight, peak int32
	var mu sync.Mutex
	web := newVerifyWeb(t, func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&inFlight, 1)
		mu.Lock()
		if n > peak {
			peak = n
		}
		mu.Unlock()
		time.Sleep(60 * time.Millisecond)
		atomic.AddInt32(&inFlight, -1)
		_, _ = io.WriteString(w, "ok")
	})

	const n = 6
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := verifyEmailThrottled(context.Background(), web, web.Origin+"/verify_email?key=x"); err != nil {
				t.Errorf("verify: %v", err)
			}
		}()
	}
	wg.Wait()

	if peak > 2 {
		t.Fatalf("并发峰值 = %d，超过闸门上限 2", peak)
	}
	if peak == 0 {
		t.Fatal("没有发出任何请求，测试无效")
	}
}

// 上游 502（worker 被打满时的典型响应）应当退避重试并最终成功。
func TestVerifyEmailRetriesOnGatewayError(t *testing.T) {
	withFastVerifyRetry(t, time.Millisecond, time.Millisecond)
	var calls int32
	web := newVerifyWeb(t, func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&calls, 1) <= 2 {
			w.WriteHeader(http.StatusBadGateway)
			_, _ = io.WriteString(w, `<h1>Incomplete response received from application</h1>`)
			return
		}
		_, _ = io.WriteString(w, "verified")
	})

	if err := verifyEmailThrottled(context.Background(), web, web.Origin+"/verify_email?key=x"); err != nil {
		t.Fatalf("应重试后成功: %v", err)
	}
	if got := atomic.LoadInt32(&calls); got != 3 {
		t.Fatalf("调用次数 = %d，期望 3", got)
	}
}

// 4xx 是链接本身的问题（过期 / 伪造），重试没有意义。
func TestVerifyEmailDoesNotRetry4xx(t *testing.T) {
	withFastVerifyRetry(t, time.Millisecond)
	var calls int32
	web := newVerifyWeb(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, "invalid key")
	})

	if err := verifyEmailThrottled(context.Background(), web, web.Origin+"/verify_email?key=x"); err == nil {
		t.Fatal("400 应返回错误")
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("4xx 不应重试，实际调用 %d 次", got)
	}
}

// 非 api.deepai.org 的链接一律拒绝，避免被拿去当开放代请求器。
func TestVerifyEmailRejectsForeignHost(t *testing.T) {
	web := newVerifyWeb(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("不应发出请求")
	})
	if err := verifyEmailThrottled(context.Background(), web, "https://evil.example/verify_email?key=x"); err == nil {
		t.Fatal("外域链接应被拒绝")
	}
}

// 退避期间不占用并发名额：一个慢请求不该把闸门攥死。
func TestVerifyEmailBackoffReleasesSlot(t *testing.T) {
	withFastVerifyRetry(t, 80*time.Millisecond, 80*time.Millisecond)
	var calls int32
	web := newVerifyWeb(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusBadGateway)
	})

	// 两个请求同时失败并进入退避；此时第三个应能立刻拿到名额。
	start := time.Now()
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = verifyEmailThrottled(context.Background(), web, web.Origin+"/verify_email?key=x")
		}()
	}
	wg.Wait()
	elapsed := time.Since(start)

	// 每个请求 3 次尝试 + 2 段退避 ≈ 160ms；若退避期间占着名额会接近翻倍。
	if elapsed > 600*time.Millisecond {
		t.Fatalf("退避期间疑似占用了并发名额：%v", elapsed)
	}
	if atomic.LoadInt32(&calls) < 4 {
		t.Fatalf("调用次数 = %d，期望至少 4（2 请求 × 2 次以上）", calls)
	}
}
