package store

import (
	"time"

	"deepaicode2api/internal/deepai"
)

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
