package server

import (
	"errors"
	"io"
	"sync"
	"testing"
	"time"
)

// slowBody 是一个可以按测试节奏喂字节的 ReadCloser。
//
// 用 channel 而不是 time.Sleep：测试要证明“在 X 毫秒内没数据就超时”，
// 让 Read 自己阻塞在 channel 上，计时由 idleTimeoutBody 负责。
type slowBody struct {
	ch     chan []byte
	once   sync.Once
	closed chan struct{}
}

func newSlowBody() *slowBody {
	return &slowBody{ch: make(chan []byte, 4), closed: make(chan struct{})}
}

func (b *slowBody) Read(p []byte) (int, error) {
	select {
	case data, ok := <-b.ch:
		if !ok {
			return 0, io.EOF
		}
		return copy(p, data), nil
	case <-b.closed:
		return 0, errors.New("closed")
	}
}

func (b *slowBody) Close() error {
	b.once.Do(func() { close(b.closed) })
	return nil
}

// readAsync 在后台读一次，用 channel 把结果送回来（Read 会阻塞到超时）。
func readAsync(body io.Reader) <-chan error {
	done := make(chan error, 1)
	go func() {
		buf := make([]byte, 8)
		_, err := body.Read(buf)
		done <- err
	}()
	return done
}

// TestIdleTimeoutAllowsSlowFirstByte 固化「非流式长思考不被当成空闲」：
// 首字节超过帧间空闲预算、但仍在首字节预算内时必须放行。
func TestIdleTimeoutAllowsSlowFirstByte(t *testing.T) {
	src := newSlowBody()
	body := newIdleTimeoutBody(src, 20*time.Millisecond, 300*time.Millisecond)
	defer body.Close()

	// 80ms > idle(20ms)，但 < first(300ms)：以前这里会被判死。
	time.Sleep(80 * time.Millisecond)
	src.ch <- []byte("a")

	select {
	case err := <-readAsync(body):
		if err != nil {
			t.Fatalf("首字节在首字节预算内，不应超时: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("首字节读取卡住")
	}
}

// TestIdleTimeoutAfterFirstByte 固化「拿到首字节后切回帧间空闲预算」：
// 首字节之后静默超过 idle 必须被收尾。
func TestIdleTimeoutAfterFirstByte(t *testing.T) {
	src := newSlowBody()
	body := newIdleTimeoutBody(src, 20*time.Millisecond, 500*time.Millisecond)
	defer body.Close()

	src.ch <- []byte("a")
	if err := <-readAsync(body); err != nil {
		t.Fatalf("首字节不应超时: %v", err)
	}

	select {
	case err := <-readAsync(body):
		if !errors.Is(err, errUpstreamIdle) {
			t.Fatalf("帧间静默应报告 errUpstreamIdle，实际 %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("帧间空闲没有触发超时")
	}
}

// TestIdleTimeoutFirstByteBudget 固化「首字节预算耗尽」：上游连首字节都不给。
func TestIdleTimeoutFirstByteBudget(t *testing.T) {
	src := newSlowBody()
	body := newIdleTimeoutBody(src, 20*time.Millisecond, 30*time.Millisecond)
	defer body.Close()

	select {
	case err := <-readAsync(body):
		if !errors.Is(err, errUpstreamIdle) {
			t.Fatalf("首字节超预算应报告 errUpstreamIdle，实际 %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("首字节预算没有触发超时")
	}
}

// TestHoldResponseBodyBudgets 固化「流式与非流式用不同首字节预算」。
func TestHoldResponseBodyBudgets(t *testing.T) {
	s := &Server{inflight: map[string]int{}}

	streamBody := s.holdResponseBody(newSlowBody(), "acct", true)
	defer streamBody.Close()
	if b, ok := streamBody.(*idleTimeoutBody); !ok {
		t.Fatalf("应返回 idleTimeoutBody，实际 %T", streamBody)
	} else if b.first != upstreamIdleTimeout {
		t.Fatalf("流式首字节预算 = %v，期望 %v", b.first, upstreamIdleTimeout)
	}

	blockBody := s.holdResponseBody(newSlowBody(), "acct", false)
	defer blockBody.Close()
	if b, ok := blockBody.(*idleTimeoutBody); !ok {
		t.Fatalf("应返回 idleTimeoutBody，实际 %T", blockBody)
	} else if b.first != upstreamFirstByteTimeout {
		t.Fatalf("非流式首字节预算 = %v，期望 %v", b.first, upstreamFirstByteTimeout)
	} else if b.idle != upstreamIdleTimeout {
		t.Fatalf("非流式帧间预算 = %v，期望 %v", b.idle, upstreamIdleTimeout)
	}
}
