package server

import (
	"context"
	"log"
	"time"

	"deepaicode2api/internal/deepai"
)

// --- 进程生命周期 ---

// Close 停止后台任务并等待已排期的落盘工作收尾。
//
// 为什么需要：配额复探 / 入池探活 / 按需复探都是「先写内存、再 Save()」的
// goroutine。旧实现里 main 在关掉 HTTP 监听后直接 Save 并退出，后台任务可能
// 在最后那次 Save 之后才落盘（或被进程杀掉），表现为「刚探到的额度重启就没了」。
//
// 语义：
//   - 关闭 stopCh，打断后台任务里最长达 12 分钟的退避睡眠；
//   - 等待 quotaWG（以 ctx 为上限，超时只记日志，不阻塞退出）；
//   - 关闭各出口的空闲连接，避免进程退出时留下半开连接。
func (s *Server) Close(ctx context.Context) {
	s.stopOnce.Do(func() {
		if s.stopCh != nil {
			close(s.stopCh)
		}
	})

	done := make(chan struct{})
	go func() {
		s.quotaWG.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-ctx.Done():
		// 退出流程不能无限等：探测本身有 20s 超时，这里通常不会命中。
		log.Printf("[shutdown] 后台任务未在期限内收尾，继续退出（未落盘的探测结果会丢失）")
	}

	s.closeIdleConnections()
}

// stopped 返回停止信号通道（Close 时关闭）。
//
// 可能为 nil（零值 Server，主要出现在测试里），调用方必须容忍。
func (s *Server) stopped() <-chan struct{} { return s.stopCh }

// sleepOrStop 等待 d，收到停止信号时返回 false。
func (s *Server) sleepOrStop(d time.Duration) bool {
	if d <= 0 {
		return true
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-s.stopped():
		return false
	case <-timer.C:
		return true
	}
}

// closeIdleConnections 关闭所有上游客户端（含各出口代理客户端）的空闲连接。
func (s *Server) closeIdleConnections() {
	closeIdle := func(c *deepai.Client) {
		if c == nil || c.HTTP == nil {
			return
		}
		c.HTTP.CloseIdleConnections()
	}
	closeIdle(s.Up)

	s.proxyMu.Lock()
	clients := make([]*deepai.Client, 0, len(s.proxyClients))
	for _, c := range s.proxyClients {
		clients = append(clients, c)
	}
	s.proxyMu.Unlock()
	for _, c := range clients {
		closeIdle(c)
	}
}
