package store

import (
	"bufio"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// logMaxBytes 超过该大小就轮转：当前文件改名为 .1，旧的 .1 丢弃。
//
// 日志只用于事后排障，轮转保留一代足够；不设无上限增长。
const logMaxBytes = 8 << 20

// logStore 是 append-only 的请求日志文件（JSONL）+ 内存环形缓冲。
//
// 从主状态文件里拆出来的原因：日志每来一个请求就变一次，留在 State 里会让
// Save 的「内容未变则跳过」永远失效 —— 后台 30s 一次的保存退化成整份
// 1.1MB 状态文件的重写 + fsync（实测约 130MB/小时写放大）。
//
// 写入用「每次追加 open+write+close」而不是常驻句柄：
//   - 常驻句柄在 Windows 上会阻止临时目录/日志文件被删除或重命名；
//   - 网关的请求速率是个位数到几十 rps，一次 open/close 的开销可忽略；
//   - 不需要缓冲刷盘逻辑，也不会在崩溃时丢最近的日志。
//
// 配置主密钥时逐行加密：一行一个 seal 信封，与主状态文件同一密钥。
type logStore struct {
	path string
	key  []byte

	mu   sync.Mutex
	size int64 // 当前文件大小（轮转判定用，open 时初始化）
	ring []LogEntry
	keep int
}

// openLogStore 打开（或创建）日志文件，并把最近 keep 条读进内存环形缓冲。
func openLogStore(path string, key []byte, keep int) (*logStore, error) {
	l := &logStore{path: path, key: key, keep: keep}
	if fi, err := os.Stat(path); err == nil {
		l.size = fi.Size()
	}
	if err := l.loadTail(); err != nil {
		return nil, err
	}
	return l, nil
}

// loadTail 读取文件最近 keep 条日志填充环形缓冲。
func (l *logStore) loadTail() error {
	if l.keep <= 0 {
		return nil
	}
	entries, err := l.scan(QueryFilter{Limit: l.keep})
	if err != nil {
		return err
	}
	l.ring = entries
	return nil
}

// append 追加一条日志：写文件 + 更新环形缓冲。
//
// keep 由调用方按当前设置传入（LogKeep 可在控制台实时调整）。
func (l *logStore) append(e LogEntry, keep int) error {
	raw, err := json.Marshal(e)
	if err != nil {
		return err
	}
	if len(l.key) > 0 {
		sealed, err := seal(l.key, raw)
		if err != nil {
			return err
		}
		raw = sealed
	}
	raw = append(raw, '\n')

	l.mu.Lock()
	defer l.mu.Unlock()

	if err := l.writeLocked(raw); err != nil {
		return err
	}
	l.ring = append(l.ring, e)
	if keep > 0 && len(l.ring) > keep {
		l.ring = l.ring[len(l.ring)-keep:]
	}
	return nil
}

// writeLocked 追加一行到文件（调用方持锁）。
func (l *logStore) writeLocked(raw []byte) error {
	if err := os.MkdirAll(filepath.Dir(l.path), 0o700); err != nil {
		return err
	}
	if l.size >= logMaxBytes {
		_ = os.Remove(l.path + ".1")
		if err := os.Rename(l.path, l.path+".1"); err == nil {
			l.size = 0
			l.ring = nil
		}
	}
	f, err := os.OpenFile(l.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	n, werr := f.Write(raw)
	cerr := f.Close()
	l.size += int64(n)
	if werr != nil {
		return werr
	}
	return cerr
}

// Flush 是兼容旧调用点的空操作：写入不驻留缓冲。
func (l *logStore) Flush() error { return nil }

// Close 是兼容旧调用点的空操作：没有常驻句柄。
func (l *logStore) Close() error { return nil }

// tail 返回最近 limit 条日志（limit<=0 表示全部）。
func (l *logStore) tail(limit int) []LogEntry {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	n := len(l.ring)
	if limit <= 0 || limit > n {
		limit = n
	}
	out := make([]LogEntry, limit)
	copy(out, l.ring[n-limit:])
	return out
}

// query 在内存环形缓冲里过滤（快路径，覆盖最近 keep 条）。
func (l *logStore) query(f QueryFilter) []LogEntry {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]LogEntry, 0, 64)
	for i := len(l.ring) - 1; i >= 0; i-- {
		if !f.match(l.ring[i]) {
			continue
		}
		out = append(out, l.ring[i])
		if f.Limit > 0 && len(out) >= f.Limit {
			break
		}
	}
	reverseLogEntries(out)
	return out
}

// clear 清空内存缓冲并截断文件。
func (l *logStore) clear() error {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.ring = nil
	l.size = 0
	if err := os.Truncate(l.path, 0); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// scan 扫描日志文件并应用过滤（从新到旧，最多 limit 条）。
//
// 用于查询超出内存环形缓冲的历史：只读文件尾部（最多 4MB），
// 避免一次大文件扫描把管理接口拖慢。
func (l *logStore) scan(f QueryFilter) ([]LogEntry, error) {
	const maxScan = 4 << 20

	fh, err := os.Open(l.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	defer fh.Close()

	fi, err := fh.Stat()
	if err != nil {
		return nil, err
	}
	start := fi.Size() - maxScan
	if start < 0 {
		start = 0
	}
	if _, err := fh.Seek(start, 0); err != nil {
		return nil, err
	}

	sc := bufio.NewScanner(fh)
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	out := make([]LogEntry, 0, 64)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		// 起始位置可能切在半行上：不是合法 JSON 就跳过。
		e, ok := l.decodeLine([]byte(line))
		if !ok || !f.match(e) {
			continue
		}
		out = append(out, e)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	reverseLogEntries(out)
	if f.Limit > 0 && len(out) > f.Limit {
		out = out[:f.Limit]
	}
	return out, nil
}

// decodeLine 解析一行（配置主密钥时先解封）。
func (l *logStore) decodeLine(raw []byte) (LogEntry, bool) {
	var e LogEntry
	body := raw
	if len(l.key) > 0 && isEnvelope(raw) {
		plain, _, err := unseal(l.key, raw)
		if err != nil {
			return e, false
		}
		body = plain
	}
	if err := json.Unmarshal(body, &e); err != nil {
		return e, false
	}
	return e, true
}

func reverseLogEntries(xs []LogEntry) {
	for i, j := 0, len(xs)-1; i < j; i, j = i+1, j-1 {
		xs[i], xs[j] = xs[j], xs[i]
	}
}

// QueryFilter 描述一次日志查询。
//
// 只作用于内存环形缓冲（最近 LogKeep 条）或文件尾部，避免把管理接口
// 变成长事务。所有条件都是「与」关系。
type QueryFilter struct {
	// Limit 是最多返回条数（0 表示不限制，由调用方兜底）。
	Limit int
	// Since 只保留该 unix 秒之后的日志（0 表示不限）。
	Since int64
	// Endpoint / Account 按等值过滤（空串表示不限）。
	Endpoint string
	Account  string
	// MinStatus 只保留 status >= MinStatus 的日志（0 表示不限）。
	MinStatus int
	// Keyword 在 model 与 err 里做不区分大小写的子串匹配。
	Keyword string
}

func (f QueryFilter) match(e LogEntry) bool {
	if f.Since > 0 && e.Time < f.Since {
		return false
	}
	if f.Endpoint != "" && e.Endpoint != f.Endpoint {
		return false
	}
	if f.Account != "" && e.Account != f.Account {
		return false
	}
	if f.MinStatus > 0 && e.Status < f.MinStatus {
		return false
	}
	if f.Keyword != "" {
		kw := strings.ToLower(f.Keyword)
		if !strings.Contains(strings.ToLower(e.Model), kw) &&
			!strings.Contains(strings.ToLower(e.Err), kw) {
			return false
		}
	}
	return true
}
