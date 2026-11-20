package server

import (
	"context"

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
	// 停止后台任务并等待已排期的落盘收尾（打断复探退避 + 等 quotaWG）。
	s.Pool.Close(ctx)

	// 粘性绑定攒批写盘，退出前必须补一次（否则重启后所有会话重新学一遍）。
	s.Pool.FlushSticky()

	s.closeIdleConnections()
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
