package server

import (
	"net/http"
	"os"
	"testing"
)

// breakStatePath 让下一次 Store.Save 必然失败：把状态文件换成同名目录。
//
// rename(临时文件 → 目标) 在 Linux 与 Windows 上都会失败，比改权限可靠
// （Windows 上目录只读位基本拦不住写入）。
func breakStatePath(t *testing.T, s *Server) {
	t.Helper()
	path := s.Store.Path()
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		t.Fatalf("移除状态文件: %v", err)
	}
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatalf("创建同名目录: %v", err)
	}
}

// TestAdminMutationReportsSaveFailure 锁死「用户主动改动必须报告落盘失败」。
//
// 历史行为：`_ = s.Store.Save()` 吞掉错误，磁盘满 / 文件被占用时接口回 200，
// 改动只活在内存里，重启即丢且无任何日志线索。
func TestAdminMutationReportsSaveFailure(t *testing.T) {
	s, _ := newTestServer(t, nil)
	before := len(s.Store.Keys())
	breakStatePath(t, s)

	rec := do(t, s, http.MethodPost, "/admin/api/keys", "", `{"name":"落盘失败"}`)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("状态码 = %d，期望 500（保存失败必须上报）：%s", rec.Code, rec.Body.String())
	}
	if got := len(s.Store.Keys()); got != before+1 {
		t.Fatalf("密钥数 = %d，期望 %d（内存改动已发生，只是没落盘）", got, before+1)
	}
}

// TestAdminKeysRejectsMalformedJSON 覆盖「畸形请求体被静默当成空请求」。
//
// 历史行为：decode 错误被忽略 → POST /admin/api/keys 发错字段时会建出一把
// 空名字的密钥并回 200。
func TestAdminKeysRejectsMalformedJSON(t *testing.T) {
	s, _ := newTestServer(t, nil)
	before := len(s.Store.Keys())

	rec := do(t, s, http.MethodPost, "/admin/api/keys", "", `{"name":`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("状态码 = %d，期望 400：%s", rec.Code, rec.Body.String())
	}
	if got := len(s.Store.Keys()); got != before {
		t.Fatalf("畸形请求不应创建密钥：%d → %d", before, got)
	}

	// 空体仍然合法：等价于「用默认参数新建一把密钥」。
	rec = do(t, s, http.MethodPost, "/admin/api/keys", "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("空体状态码 = %d，期望 200：%s", rec.Code, rec.Body.String())
	}
	if got := len(s.Store.Keys()); got != before+1 {
		t.Fatalf("空体应创建一把密钥：%d → %d", before, got)
	}
}
