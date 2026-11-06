package store

import (
	"strings"
	"time"

	"deepaicode2api/internal/deepai"
)

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
