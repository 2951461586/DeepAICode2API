package server

import (
	"context"
	"errors"
	"log"
	"strconv"
	"strings"
	"time"

	"deepaicode2api/internal/deepai"
	"deepaicode2api/internal/store"
)

// laneProbeCooldown 是两次车道探测之间的最小间隔。
//
// 车道不对时 426 会成批出现（所有请求同时撞上）；没有冷却，一次车道升级
// 就会把每个失败请求都变成一次 /models 探测。
const laneProbeCooldown = 60 * time.Second

// CurrentLane 返回当前生效的车道。
//
// 探测结果由**当前构建**验证过（LaneBuild == 编译期车道）时无条件信任它，
// 即使它比编译值旧 —— 上游可能只剩旧车道可用，此时继续发编译值会一直 426。
// 否则退回 newerLane：状态文件可能来自旧版本，升级后要直接用较新的编译值。
func (s *Server) CurrentLane() string {
	st := s.Store.Settings()
	lane := strings.TrimSpace(st.Lane)
	if lane != "" && st.LaneBuild == deepai.LaneVersion {
		return lane
	}
	return newerLane(lane)
}

// newerLane 返回持久化车道值与编译期默认值中较新的一个。
//
// 两边都按十进制整数比较；任一方不是数字时退回编译期默认值，
// 避免一个手填的怪值把车道卡死（车道不对时上游会回 426，探测会重写它）。
func newerLane(stored string) string {
	stored = strings.TrimSpace(stored)
	if stored == "" {
		return deepai.LaneVersion
	}
	s, errStored := strconv.Atoi(stored)
	c, errCompiled := strconv.Atoi(deepai.LaneVersion)
	if errStored != nil || errCompiled != nil {
		return deepai.LaneVersion
	}
	if s > c {
		return stored
	}
	return deepai.LaneVersion
}

// RefreshLane 探测一次可用车道并落盘，返回最终使用的车道。
//
// 探测打 /models（不消耗 Theos、不触发签到），按 deepai.Client 的候选序列
// （编译期车道优先）试到 2xx 为止 —— 上游抬车道后网关自动跟上，不用改代码。**2026-11-14 起
// /models 要求 Api-Key**，所以探测要借一个账号的凭据；没有账号时探测器
// 只会拿到 401，直接跳过探测（编译期 LaneVersion + 426 兜底仍然有效）。
// 冷却期内直接返回当前值；探测失败时保留旧值并把错误交给调用方。
func (s *Server) RefreshLane(ctx context.Context) (string, error) {
	if strings.TrimSpace(s.Up.Machine) == "" {
		return s.CurrentLane(), errors.New("缺少机器指纹，无法探测车道")
	}
	acct := s.probeAccount()
	if acct == nil {
		return s.CurrentLane(), errors.New("没有可用账号：上游 /models 需要 Api-Key，无法探测车道")
	}

	s.laneMu.Lock()
	if time.Since(s.laneCheckedAt) < laneProbeCooldown {
		lane := s.CurrentLane()
		s.laneMu.Unlock()
		return lane, nil
	}
	s.laneCheckedAt = time.Now()
	s.laneMu.Unlock()

	cctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	lane, err := s.Up.DetectLane(cctx, acct)
	if err != nil {
		return s.CurrentLane(), err
	}
	s.applyLane(lane)
	return lane, nil
}

// applyLane 把车道写回设置与所有已缓存的客户端。
//
// Client.SetLane 走 atomic，所以这里可以在请求进行中安全地换道；
// 新建的代理客户端在 clientFor 里会带上当前车道。
func (s *Server) applyLane(lane string) {
	lane = strings.TrimSpace(lane)
	if lane == "" {
		return
	}
	prev := s.CurrentLane()

	s.Store.UpdateSettings(func(st *store.Settings) {
		st.Lane = lane
		st.LaneBuild = deepai.LaneVersion
	})
	if err := s.Store.Save(); err != nil {
		log.Printf("保存车道版本失败: %v", err)
	}

	s.Up.SetLane(lane)
	s.proxyMu.Lock()
	for _, c := range s.proxyClients {
		c.SetLane(lane)
	}
	s.proxyMu.Unlock()

	if prev != lane {
		log.Printf("[lane] 车道切换: %s → %s", prev, lane)
	}
}
