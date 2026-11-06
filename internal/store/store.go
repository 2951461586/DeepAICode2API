// Package store 负责网关状态的持久化（账号 / 密钥 / 设置 / 日志）。
//
// 选号策略在 internal/pool；本包只在持锁状态下调用它，并把结果作为快照交付。
package store

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"deepaicode2api/internal/deepai"
	"deepaicode2api/internal/pool"
)

// APIKey 是网关对外签发的调用密钥。
type APIKey struct {
	Key       string `json:"key"`
	Name      string `json:"name"`
	Group     string `json:"group,omitempty"`
	Enabled   bool   `json:"enabled"`
	CreatedAt int64  `json:"created_at"`
	LastUsed  int64  `json:"last_used"`
	Requests  int64  `json:"requests"`
}

// Settings 是运行期可调参数。
type Settings struct {
	ModelAliases   map[string]string `json:"model_aliases"`
	RequestTimeout int               `json:"request_timeout_sec"`
	MaxRetries     int               `json:"max_retries"`
	LogKeep        int               `json:"log_keep"`
	CORSOrigin     string            `json:"cors_origin"`
	AdminPassword  string            `json:"admin_password"`
	FallbackModel  string            `json:"fallback_model"`
}

// LogEntry 是一条请求日志（只记元信息，不落对话内容）。
type LogEntry struct {
	Time      int64   `json:"time"`
	Key       string  `json:"key"`
	Account   string  `json:"account"`
	Model     string  `json:"model"`
	Endpoint  string  `json:"endpoint"`
	Stream    bool    `json:"stream"`
	Status    int     `json:"status"`
	LatencyMS int64   `json:"latency_ms"`
	PromptLen int     `json:"prompt_len"`
	OutputLen int     `json:"output_len"`
	TokensIn  int     `json:"tokens_in"`
	TokensOut int     `json:"tokens_out"`
	Cost      float64 `json:"cost"`
	Err       string  `json:"err,omitempty"`
}

// Stats 是累计统计。
type Stats struct {
	TotalRequests  int64   `json:"total_requests"`
	FailedRequests int64   `json:"failed_requests"`
	TotalCharsOut  int64   `json:"total_chars_out"`
	TotalTokensIn  int64   `json:"total_tokens_in"`
	TotalTokensOut int64   `json:"total_tokens_out"`
	TotalCost      float64 `json:"total_cost"`
	StartedAt      int64   `json:"started_at"`
}

// State 是全部持久化状态。
//
// Logs 与 Stats 刻意不写进主文件：它们随每个请求变动，留在里面会让 Save 的
// “内容未变则跳过”永远失效。日志走 append-only 的 <base>-logs.jsonl，
// 统计走 <base>-stats.json。
type State struct {
	Accounts      []*deepai.Account `json:"accounts"`
	Keys          []*APIKey         `json:"keys"`
	Settings      Settings          `json:"settings"`
	Stats         Stats             `json:"-"`
	Logs          []LogEntry        `json:"-"`
	Models        []deepai.Model    `json:"models"`
	DefaultModel  string            `json:"default_model"`
	ModelsUpdated int64             `json:"models_updated"`
	MachineID     string            `json:"machine_id"`
}

// legacyState 是拆分前的状态文件格式（含 logs / stats），仅用于读取迁移。
type legacyState struct {
	Accounts      []*deepai.Account `json:"accounts"`
	Keys          []*APIKey         `json:"keys"`
	Settings      Settings          `json:"settings"`
	Stats         Stats             `json:"stats"`
	Logs          []LogEntry        `json:"logs"`
	Models        []deepai.Model    `json:"models"`
	DefaultModel  string            `json:"default_model"`
	ModelsUpdated int64             `json:"models_updated"`
	MachineID     string            `json:"machine_id"`
}

// Store 是带锁的状态容器。
type Store struct {
	mu    sync.RWMutex
	path  string
	st    *State
	sched *pool.Scheduler

	// statsPath / logsPath 是由主路径派生的旁路文件。
	statsPath string
	logsPath  string
	logs      *logStore

	// masterKey 非空时状态文件整体加密（AES-256-GCM）。
	masterKey []byte
	// encrypted 记录上次读写时文件是否已加密（用于启动日志 / 控制台提示）。
	encrypted bool
	// lastSaved 是上次落盘的明文快照，用于跳过无变化的重复写盘。
	lastSaved []byte
	// statsDirty 标记统计数据有变化（单独小文件，每次请求都会变）。
	statsDirty bool
}

// DefaultSettings 返回默认设置。
func DefaultSettings() Settings {
	return Settings{
		ModelAliases:   map[string]string{},
		RequestTimeout: 600,
		MaxRetries:     3,
		LogKeep:        500,
		// 默认仅放行本机来源；如需局域网访问，在控制台显式配置。
		CORSOrigin:    "",
		FallbackModel: "",
	}
}

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
// 日志与统计从主文件里拆出去后，这里才真正会「内容未变」。
func (s *Store) Save() error {
	s.mu.RLock()
	plain, err := json.MarshalIndent(s.st, "", "  ")
	key := s.masterKey
	last := s.lastSaved
	enc := s.encrypted
	stats := s.st.Stats
	statsDirty := s.statsDirty
	logs := s.logs
	s.mu.RUnlock()
	if err != nil {
		return err
	}

	// 1) 主文件。刚配置主密钥且磁盘上还是明文时必须重写一次。
	if !bytes.Equal(plain, last) || (len(key) > 0 && !enc) {
		if err := writeAtomicSealed(s.path, plain, key); err != nil {
			return err
		}
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
		s.mu.Lock()
		s.statsDirty = false
		s.mu.Unlock()
	}

	// 3) 日志缓冲。
	return logs.Flush()
}

// writeAtomicSealed 以 tmp + rename 原子写入，必要时整体加密。
func writeAtomicSealed(path string, plain, key []byte) error {
	raw := plain
	if len(key) > 0 {
		sealed, err := seal(key, plain)
		if err != nil {
			return err
		}
		raw = sealed
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Close 落盘并关闭日志句柄（进程退出前调用）。
func (s *Store) Close() error {
	if s.logs == nil {
		return nil
	}
	return s.logs.Close()
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

// --- 账号 ---

// Accounts 返回账号快照列表（每项都是深拷贝）。
func (s *Store) Accounts() []*deepai.Account {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*deepai.Account, len(s.st.Accounts))
	for i, a := range s.st.Accounts {
		out[i] = a.Clone()
	}
	return out
}

// Account 按 ID 查找账号，返回深拷贝快照。
//
// 不返回号池里的活指针：账号状态会被调度、后台探测、管理接口并发改写，
// 把活指针交出去等于把加锁责任推给每个调用方。需要就地改字段时用
// UpdateAccount / UpdateAccounts（在写锁内完成）。
func (s *Store) Account(id string) *deepai.Account {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, a := range s.st.Accounts {
		if a.ID == id {
			return a.Clone()
		}
	}
	return nil
}

// AccountPage 是一次账号分页查询的结果。
type AccountPage struct {
	Items []*deepai.Account
	Total int // 过滤后的总数（未分页）
}

// AccountCounts 返回账号总数与就绪数，不解包快照。
//
// 控制台首屏需要在不同步整个账号列表的情况下展示“就绪/总”，
// 而 Accounts() 会给每个账号做深拷贝。
func (s *Store) AccountCounts() (total, ready int) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, a := range s.st.Accounts {
		if a.Enabled && a.Status != "error" {
			ready++
		}
	}
	return len(s.st.Accounts), ready
}

// AccountPage 按分组/状态/关键词过滤后分页返回账号快照。
//
// 控制台一屏只渲染几十行，而全量账号快照动辄几百 KB；
// 分页把传输量从 O(全部账号) 降到 O(单页)。
//
// offset<0 视为 0；limit<=0 用默认 50；limit>200 封顶 200。
func (s *Store) AccountPage(offset, limit int, group, status, query string) AccountPage {
	if offset < 0 {
		offset = 0
	}
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	group = strings.TrimSpace(group)
	status = strings.ToLower(strings.TrimSpace(status))
	query = strings.ToLower(strings.TrimSpace(query))

	s.mu.RLock()
	defer s.mu.RUnlock()

	matched := make([]*deepai.Account, 0, len(s.st.Accounts))
	for _, a := range s.st.Accounts {
		if group != "" && !strings.EqualFold(a.GroupName(), group) {
			continue
		}
		if status != "" && !strings.EqualFold(a.Status, status) {
			continue
		}
		if query != "" && !accountMatches(a, query) {
			continue
		}
		matched = append(matched, a)
	}

	total := len(matched)
	if offset >= total {
		return AccountPage{Items: nil, Total: total}
	}
	end := offset + limit
	if end > total {
		end = total
	}
	items := make([]*deepai.Account, 0, end-offset)
	for _, a := range matched[offset:end] {
		items = append(items, a.Clone())
	}
	return AccountPage{Items: items, Total: total}
}

// accountMatches 在 ID / 名称 / 邮箱 / 用户名 / 分组 / 代理上做子串匹配。
func accountMatches(a *deepai.Account, kw string) bool {
	for _, field := range []string{a.ID, a.Name, a.Email, a.Username, a.GroupName(), a.Proxy, a.Fingerprint} {
		if strings.Contains(strings.ToLower(field), kw) {
			return true
		}
	}
	return false
}

// UpdateAccount 在写锁内就地修改一个账号；返回账号是否存在。
func (s *Store) UpdateAccount(id string, fn func(*deepai.Account)) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, a := range s.st.Accounts {
		if a.ID == id {
			fn(a)
			return true
		}
	}
	return false
}

// UpdateAccounts 在写锁内批量遍历全部账号。
//
// fn 返回 true 表示本次改动了该账号；返回值为改动个数。
func (s *Store) UpdateAccounts(fn func(*deepai.Account) bool) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, a := range s.st.Accounts {
		if fn(a) {
			n++
		}
	}
	return n
}

// UpsertAccount 按 API Key 指纹或邮箱去重后写入账号，返回是否为新增。
//
// 同一账号重新登录时 API Key 可能不变，但重新注册/补跑时邮箱相同、
// Key 可能已轮换；按邮箱二次匹配可以避免号池里出现同一账号两份。
func (s *Store) UpsertAccount(a *deepai.Account) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, ex := range s.st.Accounts {
		if ex.KeyID() != a.KeyID() && !sameEmail(ex.Email, a.Email) {
			continue
		}
		a.ID = ex.ID
		// 运行态以旧值优先，身份字段以新值优先（空值回落）——见 MergeFrom。
		a.MergeFrom(ex)
		s.st.Accounts[i] = a.Clone()
		return false
	}
	s.st.Accounts = append(s.st.Accounts, a.Clone())
	return true
}

func sameEmail(a, b string) bool {
	a, b = strings.TrimSpace(a), strings.TrimSpace(b)
	return a != "" && b != "" && strings.EqualFold(a, b)
}

// RemoveAccount 删除账号。
func (s *Store) RemoveAccount(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, a := range s.st.Accounts {
		if a.ID == id {
			s.st.Accounts = append(s.st.Accounts[:i], s.st.Accounts[i+1:]...)
			return true
		}
	}
	return false
}

// NextAccount 以轮询方式挑一个可用账号（全部分组）。
func (s *Store) NextAccount() *deepai.Account { return s.NextAccountGroup("") }

// NextAccountGroup 在指定分组内以轮询方式挑一个可用账号。
//
// group 为空表示全部分组。连续失败过多的账号会被跳过；全组不可用时
// 回退到失败最少的那个，保证不会硬阻塞请求。分组不存在时返回 nil，
// 由调用方给出“该分组没有账号”的明确错误。
//
// 调度分两轮：第一轮只考虑「额度未知或额度 > 0」的账号。上游按机器发放
// 每日免费额度，池里常有 balance=0 的账号（免费额度已耗尽），先打过去只会
// 被上游拒一次再换号；第二轮才在去重后的全集里选，保证不会硬阻塞。
func (s *Store) NextAccountGroup(group string) *deepai.Account {
	s.mu.Lock()
	defer s.mu.Unlock()
	acct := s.sched.Pick(s.st.Accounts, group)
	if acct == nil {
		return nil
	}
	return acct.Clone()
}

// MarkAccountResult 记录一次账号调用结果。
func (s *Store) MarkAccountResult(id string, ok bool, note string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, a := range s.st.Accounts {
		if a.ID != id {
			continue
		}
		a.LastUsed = time.Now().Unix()
		if ok {
			a.Failures = 0
			a.Status = "ok"
			a.LastErr = ""
			if note != "" {
				a.Note = note
			}
		} else {
			a.Failures++
			a.Status = "error"
			a.LastErr = note
		}
		return
	}
}

// SetAccountAllowance 缓存一次配额探测结果。
func (s *Store) SetAccountAllowance(id string, al *deepai.Allowance, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, a := range s.st.Accounts {
		if a.ID != id {
			continue
		}
		a.LastUsed = time.Now().Unix()
		if err != nil {
			a.Status = "error"
			a.LastErr = err.Error()
			a.Failures++
			return
		}
		a.Theos = al
		a.Status = "ok"
		a.LastErr = ""
		a.Failures = 0
		return
	}
}

// --- 模型目录 ---

// Models 返回模型目录快照。
func (s *Store) Models() []deepai.Model {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]deepai.Model, len(s.st.Models))
	copy(out, s.st.Models)
	return out
}

// DefaultModel 返回当前默认模型。
func (s *Store) DefaultModel() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.st.DefaultModel
}

// ModelsUpdated 返回目录更新时间。
func (s *Store) ModelsUpdated() int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.st.ModelsUpdated
}

// SetModels 更新模型目录缓存。
func (s *Store) SetModels(models []deepai.Model, def string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.st.Models = models
	if def != "" {
		s.st.DefaultModel = def
	} else if len(models) > 0 && s.st.DefaultModel == "" {
		s.st.DefaultModel = models[0].ID
	}
	s.st.ModelsUpdated = time.Now().Unix()
}

// SetMachineID 记录本机机器指纹。
func (s *Store) SetMachineID(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.st.MachineID = id
}

// MachineID 返回已记录的机器指纹。
func (s *Store) MachineID() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.st.MachineID
}

// --- 密钥 ---

// Keys 返回密钥快照。
func (s *Store) Keys() []*APIKey {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*APIKey, len(s.st.Keys))
	copy(out, s.st.Keys)
	return out
}

// AddKey 新增密钥（group 为空时归入 default）。
func (s *Store) AddKey(name, group string) *APIKey {
	s.mu.Lock()
	defer s.mu.Unlock()
	group = strings.TrimSpace(group)
	if group == "" {
		group = "default"
	}
	k := &APIKey{
		Key:       "sk-" + deepai.RandomHex(24),
		Name:      name,
		Group:     group,
		Enabled:   true,
		CreatedAt: time.Now().Unix(),
	}
	s.st.Keys = append(s.st.Keys, k)
	return k
}

// KeyGroup 返回某个网关密钥绑定的号池分组（未知密钥返回空串）。
func (s *Store) KeyGroup(key string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, k := range s.st.Keys {
		if k.Key == key {
			return k.Group
		}
	}
	return ""
}

// DeleteKey 删除密钥。
func (s *Store) DeleteKey(key string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, k := range s.st.Keys {
		if k.Key == key {
			s.st.Keys = append(s.st.Keys[:i], s.st.Keys[i+1:]...)
			return true
		}
	}
	return false
}

// ToggleKey 启用/停用密钥。
func (s *Store) ToggleKey(key string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, k := range s.st.Keys {
		if k.Key == key {
			k.Enabled = !k.Enabled
			return true
		}
	}
	return false
}

// ValidKey 校验密钥并计数。
func (s *Store) ValidKey(key string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, k := range s.st.Keys {
		if k.Key == key && k.Enabled {
			k.LastUsed = time.Now().Unix()
			k.Requests++
			return true
		}
	}
	return false
}

// HasKeys 报告是否已签发任何密钥。
func (s *Store) HasKeys() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.st.Keys) > 0
}

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

// Stats 返回统计快照。
func (s *Store) Stats() Stats {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.st.Stats
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
	s.st.Stats = Stats{StartedAt: time.Now().Unix()}
	s.statsDirty = true
	s.mu.Unlock()
}
