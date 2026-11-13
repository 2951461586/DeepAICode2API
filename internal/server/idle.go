package server

import (
	"errors"
	"io"
	"sync"
	"time"
)

// upstreamIdleTimeout 是上游响应「多久没有新数据就判定为死连接」。
//
// 为什么必须有：流式响应的账号槽位（inflight）随 Body 关闭才归还，而流式
// 可能持续数分钟。上游如果发完首帧后挂住，旧的实现只依赖 r.Context()
// ——客户端不断开就一直等，槽位与连接被无限占用，且没有任何日志能说明
// 是“卡住了”还是“还在生成”。
//
// 取 120s 的依据：推理 p50 约 1.1s、长尾 10s 级；深度思考模型首帧可能
// 十几秒。120s 内一个字节都没有，基本只能是连接已死或上游静默丢包。
const upstreamIdleTimeout = 120 * time.Second

// upstreamFirstByteTimeout 是**非流式**响应等待首字节的预算。
//
// 2026-11-13 线上两次 502（125.7s / 131.1s，都是 /v1/responses 非流式）
// 就是被上面的 120s 误杀的：非流式长思考在首字节前本来就可能超过两分钟，
// 而请求本身的预算是 request_timeout（默认 600s）。把「等首字节」与
// 「帧间空闲」分开：首字节最多等 300s，之后仍按 120s 判死。
//
// 流式保持 120s 不变：SSE 若连首帧都 120s 不给，槽位一直被占着更贵。
const upstreamFirstByteTimeout = 300 * time.Second

// errUpstreamIdle 是空闲超时后向上层报告的错误。
//
// 单独定义一个值：调用方据此把请求记成 upstream_error 而不是“读到 EOF”，
// 控制台的 last_err 才能一眼看出是网关主动收尾。
var errUpstreamIdle = errors.New("上游响应空闲超时（" + upstreamIdleTimeout.String() + " 无数据）")

// idleTimeoutBody 在 d 内没有任何成功读取时关闭底层 Body。
//
// 关闭底层 Body 是目前唯一可靠的打断手段：net/http 的响应体没有暴露
// SetReadDeadline，而 transport 内部正是用「取消 ctx → Close Body」来打断
// 在途读取的，所以并发 Close 与 Read 是它支持的模式。
type idleTimeoutBody struct {
	rc io.ReadCloser
	// idle 是拿到首字节之后的帧间空闲预算；first 是首字节预算。
	idle  time.Duration
	first time.Duration
	timer *time.Timer

	mu       sync.Mutex
	closed   bool
	timedOut bool
}

// newIdleTimeoutBody 包一层空闲超时；idle <= 0 时原样返回 rc。
//
// first 是首字节预算（<=0 时取 idle）：非流式请求用它避开「长思考被当成空闲」，
// 流式传 idle 即可。拿到首字节后统一切回 idle。
func newIdleTimeoutBody(rc io.ReadCloser, idle, first time.Duration) io.ReadCloser {
	if rc == nil || idle <= 0 {
		return rc
	}
	if first <= 0 {
		first = idle
	}
	b := &idleTimeoutBody{rc: rc, idle: idle, first: first}
	b.timer = time.AfterFunc(first, b.onIdle)
	return b
}

// onIdle 由计时器触发：标记超时并关闭底层连接，让阻塞中的 Read 立即返回。
func (b *idleTimeoutBody) onIdle() {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return
	}
	b.timedOut = true
	b.mu.Unlock()
	_ = b.rc.Close()
}

func (b *idleTimeoutBody) Read(p []byte) (int, error) {
	n, err := b.rc.Read(p)
	if n > 0 {
		// 首字节之后从 first 切回 idle；已触发超时时连接已被关闭，不复活它。
		b.mu.Lock()
		if !b.closed && !b.timedOut {
			b.timer.Reset(b.idle)
		}
		b.mu.Unlock()
	}
	if err != nil {
		b.mu.Lock()
		timedOut := b.timedOut
		b.mu.Unlock()
		if timedOut {
			return n, errUpstreamIdle
		}
	}
	return n, err
}

func (b *idleTimeoutBody) Close() error {
	b.mu.Lock()
	b.closed = true
	b.mu.Unlock()
	b.timer.Stop()
	return b.rc.Close()
}

// TimedOut 报告本次读取是否因空闲超时被收尾。
func (b *idleTimeoutBody) TimedOut() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.timedOut
}
