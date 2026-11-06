package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"deepaicode2api/internal/deepai"
)

// 日志应写入旁路文件、并可跨重启恢复。
func TestLogsPersistAcrossReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	st, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	for i := 0; i < 3; i++ {
		st.Record(LogEntry{Time: int64(100 + i), Endpoint: "chat", Status: 200, Model: "m"}, 1)
	}
	if err := st.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	back, err := Open(path)
	if err != nil {
		t.Fatalf("重新打开: %v", err)
	}
	got := back.Logs(0)
	if len(got) != 3 {
		t.Fatalf("日志未持久化：got %d", len(got))
	}
	if got[2].Endpoint != "chat" {
		t.Fatalf("日志内容错误: %#v", got[2])
	}
}

// 主状态文件不应再包含 logs / stats；统计落到自己的小文件。
func TestStateFileExcludesLogsAndStats(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	st, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	st.UpsertAccount(&deepai.Account{ID: "a1", APIKey: "sk-1", Enabled: true})
	st.Record(LogEntry{Endpoint: "chat", Status: 200}, 2)
	if err := st.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读主文件: %v", err)
	}
	if strings.Contains(string(raw), `"logs"`) {
		t.Fatal("主状态文件仍包含 logs")
	}
	if strings.Contains(string(raw), `"stats"`) {
		t.Fatal("主状态文件仍包含 stats")
	}

	statsPath, logsPath := deriveSidePaths(path)
	if _, err := os.Stat(statsPath); err != nil {
		t.Fatalf("统计旁路文件缺失: %v", err)
	}
	if _, err := os.Stat(logsPath); err != nil {
		t.Fatalf("日志旁路文件缺失: %v", err)
	}
}

// 旧格式（logs/stats 内嵌在主文件）应被自动迁移。
func TestMigrateLegacyState(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	legacy := legacyState{
		Accounts: []*deepai.Account{{ID: "old", APIKey: "sk-old", Enabled: true}},
		Keys:     []*APIKey{{Key: "sk-gw", Enabled: true}},
		Settings: DefaultSettings(),
		Stats:    Stats{TotalRequests: 7, StartedAt: 1000},
		Logs:     []LogEntry{{Time: 1000, Endpoint: "chat", Status: 200}, {Time: 1001, Endpoint: "audio", Status: 500}},
	}
	raw, err := json.Marshal(legacy)
	if err != nil {
		t.Fatalf("序列化旧格式: %v", err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatalf("写旧文件: %v", err)
	}

	st, err := Open(path)
	if err != nil {
		t.Fatalf("Open(legacy): %v", err)
	}
	if got := st.Stats().TotalRequests; got != 7 {
		t.Fatalf("统计未迁移: %d", got)
	}
	if got := len(st.Logs(0)); got != 2 {
		t.Fatalf("日志未迁移: %d", got)
	}

	// 主文件应已被重写为不含 logs/stats 的新格式。
	rewritten, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("重读主文件: %v", err)
	}
	if strings.Contains(string(rewritten), `"logs"`) || strings.Contains(string(rewritten), `"stats"`) {
		t.Fatal("迁移后主文件仍含 logs/stats")
	}

	// 统计必须真的落到旁路文件：否则只活在内存里，下次重启就丢了。
	statsPath, _ := deriveSidePaths(path)
	if _, err := os.Stat(statsPath); err != nil {
		t.Fatalf("迁移后缺少统计旁路文件: %v", err)
	}
	again, err := Open(path)
	if err != nil {
		t.Fatalf("重新打开: %v", err)
	}
	if got := again.Stats().TotalRequests; got != 7 {
		t.Fatalf("统计未跨重启保留: %d", got)
	}
	if got := len(again.Logs(0)); got != 2 {
		t.Fatalf("日志未跨重启保留: %d", got)
	}
}

// QueryLogs 支持 since / endpoint / status / 关键词过滤。
func TestQueryLogsFilters(t *testing.T) {
	st := newStore(t)
	st.Record(LogEntry{Time: 100, Endpoint: "chat", Status: 200, Model: "glm"}, 0)
	st.Record(LogEntry{Time: 200, Endpoint: "chat", Status: 502, Err: "upstream boom"}, 0)
	st.Record(LogEntry{Time: 300, Endpoint: "audio", Status: 200, Model: "whisper"}, 0)

	if got := st.QueryLogs(QueryFilter{Endpoint: "chat"}); len(got) != 2 {
		t.Fatalf("endpoint 过滤: got %d", len(got))
	}
	if got := st.QueryLogs(QueryFilter{MinStatus: 400}); len(got) != 1 || got[0].Status != 502 {
		t.Fatalf("状态码过滤: %#v", got)
	}
	if got := st.QueryLogs(QueryFilter{Keyword: "boom"}); len(got) != 1 {
		t.Fatalf("关键词过滤: %#v", got)
	}
	if got := st.QueryLogs(QueryFilter{Since: 250}); len(got) != 1 || got[0].Time != 300 {
		t.Fatalf("since 过滤: %#v", got)
	}
	if got := st.QueryLogs(QueryFilter{Limit: 1}); len(got) != 1 {
		t.Fatalf("limit 未生效: %d", len(got))
	}
}

// AccountPage 分页与过滤。
func TestAccountPage(t *testing.T) {
	st := newStore(t)
	for i := 0; i < 25; i++ {
		st.UpsertAccount(&deepai.Account{
			ID:      "id" + strconv.Itoa(i),
			APIKey:  "sk-" + strconv.Itoa(i),
			Email:   "u" + strconv.Itoa(i) + "@x.com",
			Group:   "g1",
			Enabled: true,
		})
	}
	st.UpsertAccount(&deepai.Account{ID: "other", APIKey: "sk-other", Group: "g2", Enabled: true})

	page := st.AccountPage(0, 10, "", "", "")
	if page.Total != 26 || len(page.Items) != 10 {
		t.Fatalf("首页: total=%d items=%d", page.Total, len(page.Items))
	}
	page = st.AccountPage(20, 10, "", "", "")
	if len(page.Items) != 6 {
		t.Fatalf("末页应剩 6，实际 %d", len(page.Items))
	}
	page = st.AccountPage(0, 10, "g2", "", "")
	if page.Total != 1 {
		t.Fatalf("分组过滤: %d", page.Total)
	}
	page = st.AccountPage(0, 10, "", "", "u3@")
	if page.Total != 1 {
		t.Fatalf("关键词过滤: %d", page.Total)
	}
	// 越界 offset 不应 panic，返回空页。
	page = st.AccountPage(999, 10, "", "", "")
	if len(page.Items) != 0 || page.Total != 26 {
		t.Fatalf("越界: items=%d total=%d", len(page.Items), page.Total)
	}
}

// 日志环形缓冲按 LogKeep 裁剪（内存语义保持）。
func TestLogsTrimmedToKeep(t *testing.T) {
	st := newStore(t)
	st.UpdateSettings(func(s *Settings) { s.LogKeep = 3 })
	for i := 0; i < 10; i++ {
		st.Record(LogEntry{Time: int64(i), Status: 200}, 0)
	}
	if got := len(st.Logs(0)); got != 3 {
		t.Fatalf("应裁剪到 3 条，实际 %d", got)
	}
	if got := st.Stats().TotalRequests; got != 10 {
		t.Fatalf("统计不应被裁剪，实际 %d", got)
	}
}
