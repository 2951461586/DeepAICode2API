package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"deepaicode2api/internal/model"
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
	st.UpsertAccount(&model.Account{ID: "a1", APIKey: "sk-1", Enabled: true})
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
		Accounts: []*model.Account{{ID: "old", APIKey: "sk-old", Enabled: true}},
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

// 排序必须由服务端做：分页切片前后都要有序，且相等值按 ID 兜底，
// 否则同一批数据在翻页时会重复或漏条。

// TestRecordAccumulatesCacheWriteTokens 锁住「读命中 / 写缓存」两段的分账。
//
// 两者都随 prompt token 计费，但语义相反：读是收益，写是投资。混算会让
// 「命中率」这种比率失去意义（把首轮写缓存也算成命中即虚高）。
func TestRecordAccumulatesCacheWriteTokens(t *testing.T) {
	st := newStore(t)
	st.Record(LogEntry{Status: 200, TokensIn: 1000, CachedTokens: 600, CacheWriteTokens: 300}, 0)
	st.Record(LogEntry{Status: 200, TokensIn: 500, CacheWriteTokens: 100}, 0)

	got := st.Stats()
	if got.TotalCachedTokens != 600 || got.TotalCacheWriteTokens != 400 {
		t.Fatalf("累计 读/写 = %d/%d，期望 600/400", got.TotalCachedTokens, got.TotalCacheWriteTokens)
	}
	if got.DayCachedTokens != 600 || got.DayCacheWriteTokens != 400 {
		t.Fatalf("当日 读/写 = %d/%d，期望 600/400", got.DayCachedTokens, got.DayCacheWriteTokens)
	}
}

// TestDayCacheWriteTokensResetOnNewDay 保证跨天后当日「写缓存」也归零。
//
// 漏掉这个字段不会报错，只会让第二天的界面沿用一个不存在的分子/分母组合
// （当日写缓存越积越大、命中率越看越低），属于典型的静默统计漂移。
func TestDayCacheWriteTokensResetOnNewDay(t *testing.T) {
	st := newStore(t)
	st.Record(LogEntry{Status: 200, TokensIn: 1000, CachedTokens: 600, CacheWriteTokens: 300}, 0)

	st.mu.Lock()
	st.st.Stats.Day = "2000-01-01"
	st.mu.Unlock()

	got := st.Stats() // 读路径跨天判定：只改返回值
	if got.DayCacheWriteTokens != 0 || got.DayCachedTokens != 0 || got.DayTokensIn != 0 {
		t.Fatalf("跨天未清零: %+v", got)
	}
	if got.TotalCacheWriteTokens != 300 {
		t.Fatalf("累计不该被清零: %d", got.TotalCacheWriteTokens)
	}
}
