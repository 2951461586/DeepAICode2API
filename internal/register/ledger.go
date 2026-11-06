package register

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// Ledger 是 append-only JSONL 台账（进程内带锁）。
//
// 台账保留全部尝试（含失败），交付物只写成功的；合并规则见 Merge。
type Ledger struct {
	Path string

	mu sync.Mutex
}

// NewLedger 构造台账（不立即创建文件）。
func NewLedger(path string) *Ledger { return &Ledger{Path: path} }

// Append 追加一条记录。
func (l *Ledger) Append(rec *Record) error {
	if l == nil || l.Path == "" || rec == nil {
		return nil
	}
	rec.Normalize()
	raw, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(l.Path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(l.Path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.Write(append(raw, '\n')); err != nil {
		return err
	}
	return f.Sync()
}

// Load 读取并合并台账（按邮箱并集合并，末次等级更高的记录胜出）。
func (l *Ledger) Load() ([]*Record, error) {
	if l == nil || l.Path == "" {
		return nil, nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	f, err := os.Open(l.Path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	byKey := map[string]*Record{}
	var order []string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 8<<20)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var rec Record
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			continue
		}
		rec.Normalize()
		if rec.Email == "" {
			continue
		}
		if old, ok := byKey[rec.Email]; ok {
			byKey[rec.Email] = Merge(old, &rec)
		} else {
			byKey[rec.Email] = &rec
			order = append(order, rec.Email)
		}
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("读取台账失败: %w", err)
	}
	out := make([]*Record, 0, len(order))
	for _, key := range order {
		out = append(out, byKey[key])
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].CreatedAt < out[j].CreatedAt })
	return out, nil
}
