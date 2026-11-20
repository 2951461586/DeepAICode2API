package server

import (
	"context"
	"net/http"

	"deepaicode2api/internal/deepai"
	"deepaicode2api/internal/httpx"
	"deepaicode2api/internal/logx"
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

// --- 状态落盘 ---
//
// 管理动作改完内存就直接回 {"ok":true} 是不够的：磁盘满、文件被占用（Windows
// 上常见）、目录只读时，改动会在重启后消失，而调用方拿到的仍是成功。
// 这里统一两个入口，把「保存失败」变成可观测、且（对用户动作）可回传的失败。

// persist 落盘并记录失败，不改变接口返回。
//
// 用于「结果已经产生、保存只是尽力而为」的场景：后台探测结果、登录入池收尾、
// 扩展端点的实时用量。失败会留下一行 warn 日志，但不必让本次请求失败。
func (s *Server) persist() {
	if err := s.Store.Save(); err != nil {
		logx.Warn("状态文件保存失败（改动仅存在于内存）", "err", err, "path", s.Store.Path())
	}
}

// persistOrFail 落盘；失败时回 500 并返回 false，调用方应立即 return。
//
// 用于用户主动触发的改动（增删密钥、改设置、账号治理）：这类操作的语义就是
// 「已持久化」，静默吞掉失败等于骗调用方。
func (s *Server) persistOrFail(w http.ResponseWriter) bool {
	if err := s.Store.Save(); err != nil {
		logx.Warn("状态文件保存失败", "err", err, "path", s.Store.Path())
		httpx.WriteErr(w, http.StatusInternalServerError,
			"状态保存失败（改动未落盘）: "+err.Error(), "server_error")
		return false
	}
	return true
}
