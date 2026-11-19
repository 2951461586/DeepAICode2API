package store

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// --- 会话粘性绑定（旁路文件）---
//
// 绑定表（affinity → 账号）是缓存性质的数据：丢了只会让下一次请求重新学习，
// 不影响正确性。但它也不该随进程重启清零——重启后同一会话立刻换号，前缀缓存
// 白丢一次。所以放进独立的旁路文件，而不是主状态文件：
//
//   - 主状态文件（账号 / 密钥 / 设置）体积大且被 30s 周期重写，塞进绑定表会
//     把「内容未变则跳过」打穿（docs/optimization-plan-r2.md §0.4 的老问题）；
//   - 绑定表最多 2048 条、只记录 affinity 与账号 ID，单独写盘代价极小；
//   - 与 stats 一样按主密钥封存，affinity 可能是会话标识，不该明文躺着。

// StickyBinding 是一条会话粘性绑定。
type StickyBinding struct {
	// AccountID 是被绑定的账号。
	AccountID string `json:"account_id"`
	// At 是绑定时间（Unix 秒），用于 TTL 判定。
	At int64 `json:"at"`
}

// StickyPath 返回绑定表旁路文件路径。
func (s *Store) StickyPath() string { return s.stickyPath }

// deriveStickyPath 由主状态文件路径派生绑定表路径。
func deriveStickyPath(path string) string {
	base := path
	if trimmed, ok := strings.CutSuffix(path, ".json"); ok {
		base = trimmed
	}
	return base + "-sticky.json"
}

// LoadSticky 读取绑定表；文件不存在时返回空表（首次启动 / 老版本升级）。
//
// 解析失败或密钥不匹配时返回错误，调用方应当只记日志并继续启动：
// 丢掉绑定只是少一点缓存命中，绝不能因此拒绝服务。
func (s *Store) LoadSticky() (map[string]StickyBinding, error) {
	if s.stickyPath == "" {
		return map[string]StickyBinding{}, nil
	}
	raw, err := os.ReadFile(s.stickyPath)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]StickyBinding{}, nil
		}
		return nil, err
	}
	plain, _, uerr := unseal(s.masterKey, raw)
	if uerr != nil {
		return nil, uerr
	}
	out := map[string]StickyBinding{}
	if err := json.Unmarshal(plain, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// SaveSticky 原子写入绑定表；内容与上次写入一致时跳过。
//
// 与 Save 不同，这里不做时间戳量化：绑定表本身很小，且只在「有绑定变化」
// 时才会被调用（后台 30s 一次 + 退出前一次），写放大量级可忽略。
func (s *Store) SaveSticky(m map[string]StickyBinding) error {
	if s.stickyPath == "" {
		return nil
	}
	if m == nil {
		m = map[string]StickyBinding{}
	}
	plain, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	s.saveMu.Lock()
	defer s.saveMu.Unlock()
	if bytes.Equal(plain, s.stickyLast) {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(s.stickyPath), 0o700); err != nil {
		return err
	}
	if err := writeAtomicSealed(s.stickyPath, plain, s.masterKey); err != nil {
		return err
	}
	s.stickyLast = append(s.stickyLast[:0], plain...)
	return nil
}
