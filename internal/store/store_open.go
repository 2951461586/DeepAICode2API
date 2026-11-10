package store

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"time"

	"deepaicode2api/internal/deepai"
	"deepaicode2api/internal/pool"
)

// Open 载入（或初始化）状态文件。
//
// 主密钥来自 DEEPAI2API_MASTER_KEY：配置后写出的状态文件会被整体加密，
// 未配置时保持明文（并在启动日志中警告）。
func Open(path string) (*Store, error) {
	key, err := MasterKeyFromEnv()
	if err != nil {
		return nil, err
	}
	return OpenWithKey(path, key)
}

// deriveSidePaths 由主状态文件路径派生旁路文件路径。
func deriveSidePaths(path string) (statsPath, logsPath string) {
	base := path
	if trimmed, ok := strings.CutSuffix(path, ".json"); ok {
		base = trimmed
	}
	return base + "-stats.json", base + "-logs.jsonl"
}

// OpenWithKey 用指定主密钥载入状态文件（key 为 nil 表示不加密）。
//
// 主文件之外还有两个旁路文件（由主路径派生）：
//   - <base>-stats.json   累计统计（几百字节，每 30s 写一次）
//   - <base>-logs.jsonl   请求日志（append-only，按大小轮转）
//
// 旧版本把 logs / stats 内嵌在主文件里，载入时自动迁出并重写主文件。
func OpenWithKey(path string, key []byte) (*Store, error) {
	statsPath, logsPath := deriveSidePaths(path)
	s := &Store{
		path: path, statsPath: statsPath, logsPath: logsPath,
		st: &State{Settings: DefaultSettings()}, masterKey: key, sched: pool.New(),
	}

	var (
		seedLogs    []LogEntry
		needMigrate bool
	)
	raw, err := os.ReadFile(path)
	switch {
	case err == nil:
		plain, encrypted, uerr := unseal(key, raw)
		if uerr != nil {
			return nil, uerr
		}
		s.encrypted = encrypted
		var legacy legacyState
		if err := json.Unmarshal(plain, &legacy); err != nil {
			return nil, fmt.Errorf("状态文件解析失败: %w", err)
		}
		s.st.Accounts = legacy.Accounts
		s.st.Keys = legacy.Keys
		s.st.Settings = legacy.Settings
		s.st.Models = legacy.Models
		s.st.DefaultModel = legacy.DefaultModel
		s.st.ModelsUpdated = legacy.ModelsUpdated
		s.st.MachineID = legacy.MachineID
		// 记住原始内容，首次 Save 可直接跳过。
		s.lastSaved = append([]byte(nil), plain...)
		// 旧格式：logs / stats 还在主文件里 → 迁出，稍后重写主文件。
		if len(legacy.Logs) > 0 || legacy.Stats != (Stats{}) {
			s.st.Stats = legacy.Stats
			seedLogs = legacy.Logs
			needMigrate = true
		}
	case errors.Is(err, os.ErrNotExist):
		s.st.Stats.StartedAt = time.Now().Unix()
	default:
		return nil, err
	}

	s.applySettingsDefaults()

	// 旁路文件比旧内嵌字段更新，优先采用。
	_, statErr := os.Stat(s.statsPath)
	hadStatsFile := statErr == nil
	if err := s.loadStats(); err != nil {
		return nil, err
	}
	if s.st.Stats.StartedAt == 0 {
		s.st.Stats.StartedAt = time.Now().Unix()
	}
	if !hadStatsFile {
		// 首次运行或从旧格式迁移：必须把统计写到旁路文件，
		// 否则它只活在内存里，下次重启就丢了。
		s.statsDirty = true
	}

	logs, err := openLogStore(logsPath, key, s.st.Settings.LogKeep)
	if err != nil {
		return nil, err
	}
	s.logs = logs
	if len(seedLogs) > 0 && len(logs.ring) == 0 {
		for _, e := range seedLogs {
			if err := logs.append(e, s.st.Settings.LogKeep); err != nil {
				return nil, err
			}
		}
	}

	if needMigrate {
		// 强制重写主文件：把 logs / stats 从里面捕掉。
		s.lastSaved = nil
		if err := s.Save(); err != nil {
			return nil, fmt.Errorf("迁移状态文件失败: %w", err)
		}
	}
	return s, nil
}

// applySettingsDefaults 补齐旧状态文件里缺失或已废弃的设置项。
func (s *Store) applySettingsDefaults() {
	def := DefaultSettings()
	if s.st.Settings.ModelAliases == nil {
		s.st.Settings.ModelAliases = def.ModelAliases
	}
	if s.st.Settings.RequestTimeout == 0 {
		s.st.Settings.RequestTimeout = def.RequestTimeout
	}
	if s.st.Settings.MaxRetries == 0 {
		s.st.Settings.MaxRetries = def.MaxRetries
	}
	if s.st.Settings.LogKeep == 0 {
		s.st.Settings.LogKeep = def.LogKeep
	}
	if s.st.Settings.CORSOrigin == "*" {
		// 历史状态文件里的通配符不再生效，收紧为本机来源。
		s.st.Settings.CORSOrigin = ""
	}
}

// loadStats 读取统计旁路文件；不存在时保留旧内嵌值。
func (s *Store) loadStats() error {
	raw, err := os.ReadFile(s.statsPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	plain, _, uerr := unseal(s.masterKey, raw)
	if uerr != nil {
		return fmt.Errorf("统计文件解析失败: %w", uerr)
	}
	var st Stats
	if err := json.Unmarshal(plain, &st); err != nil {
		return fmt.Errorf("统计文件解析失败: %w", err)
	}
	s.st.Stats = st
	return nil
}

// Path 返回状态文件路径。
func (s *Store) Path() string { return s.path }

// Save 原子写回状态（配置主密钥时整文件加密）。
//
// 分三部分，各自有独立的“未变化”判定：
//  1. 主文件（账号/密钥/设置/模型）：内容未变则跳过；
//  2. 统计旁路文件：仅在有新请求时写，只有几百字节；
//  3. 日志：交给 logStore 的缓冲落盘。
//
// 序列化在锁外进行：号池有 653 个账号时一次 marshal 约 1MB，
// 持读锁做这件事会顶住请求路径上的写锁（MarkAccountResult）。
func (s *Store) Save() error {
	s.saveMu.Lock()
	defer s.saveMu.Unlock()

	s.mu.RLock()
	snap := s.snapshotLocked()
	key := s.masterKey
	last := s.lastSaved
	enc := s.encrypted
	stats := s.st.Stats
	statsDirty := s.statsDirty
	logs := s.logs
	s.mu.RUnlock()

	plain, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return err
	}

	// 1) 主文件。刚配置主密钥且磁盘上还是明文时必须重写一次。
	if !bytes.Equal(plain, last) || (len(key) > 0 && !enc) {
		if err := writeAtomicSealed(s.path, plain, key); err != nil {
			return err
		}
		s.savesTotal.Add(1)
		s.savedBytes.Add(int64(len(plain)))
		s.mu.Lock()
		s.lastSaved = append(s.lastSaved[:0], plain...)
		if len(key) > 0 {
			s.encrypted = true
		}
		s.mu.Unlock()
	}

	// 2) 统计。
	if statsDirty {
		rawStats, err := json.Marshal(stats)
		if err != nil {
			return err
		}
		if err := writeAtomicSealed(s.statsPath, rawStats, key); err != nil {
			return err
		}
		s.clearStatsDirtyIfUnchanged(stats)
	}

	// 3) 日志缓冲。
	return logs.Flush()
}

// saveTimestampGranularity 是运行态时间戳（LastUsed / Theos.CheckedAt）落盘时的精度（秒）。
//
// 两者都会随每个请求或每次探测变化，秒级精度意味着「内容未变则跳过」在持续
// 流量下永远失效：653 个账号的状态文件约 950KB，30s 一次重写就是
// 约 115MB/h 的写放大。落盘时向下取整到 5 分钟，内存仍是秒级；
// 代价只是重启后「最近使用 / 额度探测时间」精度降到 5 分钟。
const saveTimestampGranularity = 300

// snapshotLocked 构造落盘快照。调用方必须持有读锁。
//
// 快照必须深拷贝所有会被就地修改的字段：Save 的序列化在锁外进行，
// 复用活指针会让 ToggleKey / DeleteKey 与 marshal 撞车（-race 可见）。
func (s *Store) snapshotLocked() State {
	snap := *s.st
	snap.Accounts = make([]*deepai.Account, len(s.st.Accounts))
	for i, a := range s.st.Accounts {
		c := a.Clone()
		c.LastUsed = quantizeTimestamp(c.LastUsed)
		if c.Theos != nil {
			// Clone 已复制 Allowance 结构体，这里改的是快照自己的副本。
			c.Theos.CheckedAt = quantizeTimestamp(c.Theos.CheckedAt)
		}
		snap.Accounts[i] = c
	}
	snap.Keys = make([]*APIKey, len(s.st.Keys))
	for i, k := range s.st.Keys {
		c := *k
		snap.Keys[i] = &c
	}
	snap.Models = cloneModels(s.st.Models)
	// ModelAliases 是 map：深拷贝一份，保证锁外序列化不会被并发写撞上。
	if s.st.Settings.ModelAliases != nil {
		snap.Settings.ModelAliases = maps.Clone(s.st.Settings.ModelAliases)
	}
	return snap
}

// cloneModels 复制模型目录（含 Input 切片与 TheosPerSecond map）。
func cloneModels(models []deepai.Model) []deepai.Model {
	if models == nil {
		return nil
	}
	out := make([]deepai.Model, len(models))
	for i, m := range models {
		if m.Input != nil {
			m.Input = append([]string(nil), m.Input...)
		}
		if m.TheosPerSecond != nil {
			m.TheosPerSecond = maps.Clone(m.TheosPerSecond)
		}
		out[i] = m
	}
	return out
}

// quantizeTimestamp 把运行态时间戳向下取整到落盘精度；0 / 负值原样返回。
func quantizeTimestamp(t int64) int64 {
	if t <= 0 {
		return t
	}
	return t - t%saveTimestampGranularity
}

// clearStatsDirtyIfUnchanged 只在「写出的快照仍是当前统计」时清脏。
//
// Save 取快照与落盘之间可能插入 Record：直接置 false 会吞掉那次自增，
// 让最后一条统计只在内存里（进程随后退出就丢）。Stats 全字段可比较，
// 用它精确定界即可。
func (s *Store) clearStatsDirtyIfUnchanged(written Stats) {
	s.mu.Lock()
	if s.st.Stats == written {
		s.statsDirty = false
	}
	s.mu.Unlock()
}

// SaveCounters 返回主文件实际写盘次数与明文字节数（用于观测写放大）。
func (s *Store) SaveCounters() (saves, bytes int64) {
	return s.savesTotal.Load(), s.savedBytes.Load()
}

// writeAtomicSealed 以「唯一临时文件 + fsync + rename」原子写入，必要时整体加密。
func writeAtomicSealed(path string, plain, key []byte) error {
	raw := plain
	if len(key) > 0 {
		sealed, err := seal(key, plain)
		if err != nil {
			return err
		}
		raw = sealed
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	// 临时文件名必须唯一：早期实现固定用 <path>.tmp，两个写入方
	// （并发 Save，或误起的第二个网关实例）会互相截断。
	f, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer func() { _ = os.Remove(tmp) }() // rename 成功后是 no-op

	if _, err := f.Write(raw); err != nil {
		_ = f.Close()
		return err
	}
	// 先 fsync 再 rename：否则掉电可能留下「名字已换、内容为空」的文件。
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Encrypted 报告磁盘上的状态文件是否为加密格式。
//
// 注意：配置了主密钥但还没保存过时，磁盘上依然是明文，
// 此时本方法返回 false（用 MasterKeySet 判断是否已配置加密）。
func (s *Store) Encrypted() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.encrypted
}

// MasterKeySet 报告是否已配置主密钥（配置后每次 Save 都会加密）。
func (s *Store) MasterKeySet() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.masterKey) > 0
}
