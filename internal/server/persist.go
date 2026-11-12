package server

import (
	"net/http"

	"deepaicode2api/internal/logx"
)

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
		writeErr(w, http.StatusInternalServerError,
			"状态保存失败（改动未落盘）: "+err.Error(), "server_error")
		return false
	}
	return true
}
