package store

import (
	"strings"
	"time"

	"deepaicode2api/internal/deepai"
)

// --- 密钥 ---

// Keys 返回密钥快照。
//
// 元素必须逐份拷贝：调用方拿到切片后会在锁外读字段（控制台的 safeKeyList），
// 而 ValidKey 在每个数据面请求上都会就地累加 LastUsed / Requests。返回号池里的
// 活指针会让「控制台开着 + 有流量」变成实打实的数据竞争（-race 可复现）。
func (s *Store) Keys() []*APIKey {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*APIKey, len(s.st.Keys))
	for i, k := range s.st.Keys {
		c := *k
		out[i] = &c
	}
	return out
}

// AddKey 新增密钥（group 为空时归入 default）。
//
// 返回值是副本：调用方通常紧接着读 Key 字段返回给用户，不应让它持有号池里的
// 活指针（同 Keys 的理由）。
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
	c := *k
	return &c
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

// SetKeyGroup 修改密钥绑定的号池分组（空串等价于 default）。
//
// 用于运营调整：例如清空压测分组后，把密钥改绑回 default，
// 避免「分组内无可用账号」。返回是否找到该密钥。
func (s *Store) SetKeyGroup(key, group string) bool {
	group = strings.TrimSpace(group)
	if group == "" {
		group = "default"
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, k := range s.st.Keys {
		if k.Key == key {
			k.Group = group
			return true
		}
	}
	return false
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
