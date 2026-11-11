package store

import (
	"time"
)

// --- 设置 / 统计 / 日志 ---

// Settings 返回设置快照。
func (s *Store) Settings() Settings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	cp := s.st.Settings
	cp.ModelAliases = map[string]string{}
	for k, v := range s.st.Settings.ModelAliases {
		cp.ModelAliases[k] = v
	}
	return cp
}

// UpdateSettings 局部更新设置。
func (s *Store) UpdateSettings(fn func(*Settings)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	fn(&s.st.Settings)
}

// statsDayLayout 是「当日用量」的日期键格式（本地时区）。
const statsDayLayout = "2006-01-02"

// rollStatsDay 在跨天时把当日计数清零并更新日期键。
//
// 放在 Record 的写锁内：它需要同时改 Day 与两个计数器，分开做会出现
// 「日期已翻页但计数还是昨天」的中间态。调用方必须持有 s.mu 写锁。
func rollStatsDay(st *Stats, now time.Time) {
	today := now.Format(statsDayLayout)
	if st.Day == today {
		return
	}
	st.Day = today
	st.DayTokensIn = 0
	st.DayTokensOut = 0
}

// Stats 返回统计快照。
//
// 读路径也做一次跨天判定（只改副本）：重启后当天还没发过请求时，
// 不能把昨天的当日用量当成今天的展示出去。
func (s *Store) Stats() Stats {
	s.mu.RLock()
	st := s.st.Stats
	s.mu.RUnlock()
	if st.Day != time.Now().Format(statsDayLayout) {
		st.Day = time.Now().Format(statsDayLayout)
		st.DayTokensIn = 0
		st.DayTokensOut = 0
	}
	return st
}

// Logs 返回最近的日志。
func (s *Store) Logs(limit int) []LogEntry {
	s.mu.RLock()
	logs := s.logs
	s.mu.RUnlock()
	if logs == nil {
		return nil
	}
	return logs.tail(limit)
}

// Record 记录一次请求结果并更新统计。
func (s *Store) Record(e LogEntry, outChars int) {
	s.mu.Lock()
	s.st.Stats.TotalRequests++
	if e.Status >= 400 || e.Err != "" {
		s.st.Stats.FailedRequests++
	}
	s.st.Stats.TotalCharsOut += int64(outChars)
	s.st.Stats.TotalTokensIn += int64(e.TokensIn)
	s.st.Stats.TotalTokensOut += int64(e.TokensOut)
	s.st.Stats.TotalCachedTokens += int64(e.CachedTokens)
	// 当日用量：跨天时先清零（不用定时任务，读路径上判定即可）。
	rollStatsDay(&s.st.Stats, time.Now())
	s.st.Stats.DayTokensIn += int64(e.TokensIn)
	s.st.Stats.DayTokensOut += int64(e.TokensOut)
	s.st.Stats.TotalCost += e.Cost
	s.statsDirty = true
	keep := s.st.Settings.LogKeep
	logs := s.logs
	s.mu.Unlock()

	// 日志写入（含文件 IO）放在锁外：它不应该阻塞号池调度。
	if logs != nil {
		_ = logs.append(e, keep)
	}
}

// QueryLogs 按条件查询日志。
//
// 走文件扫描（最多读尾部 4MB），所以能回溯超过内存环形缓冲的历史；
// 文件不可读时退回内存环形缓冲。limit<=0 时默认 200。
func (s *Store) QueryLogs(f QueryFilter) []LogEntry {
	if f.Limit <= 0 {
		f.Limit = 200
	}
	s.mu.RLock()
	logs := s.logs
	s.mu.RUnlock()
	if logs == nil {
		return nil
	}
	out, err := logs.scan(f)
	if err != nil {
		return logs.query(f)
	}
	return out
}

// ClearLogs 清空日志（内存环形缓冲与文件）。
func (s *Store) ClearLogs() {
	s.mu.RLock()
	logs := s.logs
	s.mu.RUnlock()
	if logs != nil {
		_ = logs.clear()
	}
}

// ClearStats 重置统计。
func (s *Store) ClearStats() {
	s.mu.Lock()
	started := s.st.Stats.StartedAt
	if started == 0 {
		started = time.Now().Unix()
	}
	// 保留 StartedAt（进程起点），单独记 ResetAt：日志不清空，
	// 控制台需要能解释「统计数小于日志条数」。
	day := time.Now().Format(statsDayLayout)
	s.st.Stats = Stats{StartedAt: started, ResetAt: time.Now().Unix(), Day: day}
	s.statsDirty = true
	s.mu.Unlock()
}
