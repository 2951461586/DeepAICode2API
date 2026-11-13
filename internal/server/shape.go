package server

import (
	"crypto/sha256"
	"encoding/hex"
	"time"
)

// 请求体形状的「学习记忆」。
//
// 2026-11-13 线上：同一台机器 + node UA + 缺 system 的请求会被上游 400
// （This request cannot be accepted.）。网关目前的策略是「先透传 → 400 →
// 补形状重试」，每个这类请求都要多打一次上游（线上 82 分钟 99 次）。
//
// 记忆按 API Key 的哈希（不是明文）保存 10 分钟：一旦某个调用方被确认需要
// 桌面端形状，后续缺 system 的请求在**第一次发送前**就补上 system，省掉
// 那次注定失败的往返；调用方自己带 system 时不做任何改写。
//
// 只在 Settings.PresetDesktopShape 打开时生效（默认关闭）：
// 这是采样语义变更（多一条 system），要显式选择。
const (
	presetShapeTTL = 10 * time.Minute
	// presetShapeMax 限制记忆条目数，避免被随机 key 打爆内存。
	presetShapeMax = 1024
)

func shapeCacheKey(apiKey string) string {
	if apiKey == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(apiKey))
	return hex.EncodeToString(sum[:6])
}

// shapeNeeded 报告该调用方最近是否被上游按形状拒绝过。
func (s *Server) shapeNeeded(apiKey string) bool {
	k := shapeCacheKey(apiKey)
	if k == "" {
		return false
	}
	s.shapeMu.Lock()
	defer s.shapeMu.Unlock()
	until, ok := s.shapeNeed[k]
	if !ok {
		return false
	}
	if time.Now().After(until) {
		delete(s.shapeNeed, k)
		return false
	}
	return true
}

// markShapeNeeded 记住「该调用方需要桌面端形状」。
func (s *Server) markShapeNeeded(apiKey string) {
	k := shapeCacheKey(apiKey)
	if k == "" {
		return
	}
	s.shapeMu.Lock()
	defer s.shapeMu.Unlock()
	if s.shapeNeed == nil {
		s.shapeNeed = map[string]time.Time{}
	}
	if len(s.shapeNeed) >= presetShapeMax {
		// 简单清理：先删过期项；仍然超限就整体重建（宁可忘掉旧记忆）。
		now := time.Now()
		for key, until := range s.shapeNeed {
			if now.After(until) {
				delete(s.shapeNeed, key)
			}
		}
		if len(s.shapeNeed) >= presetShapeMax {
			s.shapeNeed = map[string]time.Time{}
		}
	}
	s.shapeNeed[k] = time.Now().Add(presetShapeTTL)
}
