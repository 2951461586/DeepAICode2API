package server

import (
	"context"
	"errors"
	"log"
	"strconv"
	"strings"
	"time"

	"deepaicode2api/internal/deepai"
	"deepaicode2api/internal/logx"
	"deepaicode2api/internal/store"
)

// laneProbeCooldown 是两次车道探测之间的最小间隔。
//
// 车道不对时 426 会成批出现（所有请求同时撞上）；没有冷却，一次车道升级
// 就会把每个失败请求都变成一次 /models 探测。
const laneProbeCooldown = 60 * time.Second

// atoiOr 把十进制字符串转成 int，失败时返回 def。
func atoiOr(v string, def int) int {
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil {
		return def
	}
	return n
}

// UpstreamLane 返回 /theos 回报的上游车道（0 = 尚未观测到）。
func (s *Server) UpstreamLane() int { return int(s.upstreamLane.Load()) }

// LaneDrift 返回「上游车道 − 网关当前车道」。
//
// 正数表示客户端已经抬了车道而网关还在发旧值：2026-11-19 起上游对 ≤7 全放行，
// 请求仍然能成功，所以这条路径只在观测层面报警（日志 / 指标 / 控制台），
// 不自动换车道 —— 新车道可能伴随请求体形状变化，必须白盒复核后再升。
// 0 或负数表示没有漂移（或尚未观测）。
func (s *Server) LaneDrift() int {
	up := s.UpstreamLane()
	if up <= 0 {
		return 0
	}
	return up - atoiOr(s.CurrentLane(), 0)
}

// noteUpstreamLane 记录一次 /theos 回报的上游车道，并在变化时留痕。
//
// 由 deepai.Client.LaneHook 在每次 /theos 成功解析后调用，调用频率与额度
// 探测一致（低频）；只在**数值变化**时打日志，所以稳态下没有噪声。
func (s *Server) noteUpstreamLane(v int) {
	if v <= 0 {
		return
	}
	prev := s.upstreamLane.Swap(int64(v))
	if prev == int64(v) {
		return
	}
	compiled := atoiOr(deepai.LaneVersion, 0)
	switch {
	case prev == 0:
		log.Printf("[lane] 上游车道: %d（网关当前发 %s，编译期 %s）", v, s.CurrentLane(), deepai.LaneVersion)
	case v > compiled:
		logx.Warn("上游车道领先网关：客户端已抬车道，建议升级网关",
			"upstream", v, "gateway", deepai.LaneVersion, "lane", s.CurrentLane())
	default:
		log.Printf("[lane] 上游车道变化: %d → %d（网关当前发 %s）", prev, v, s.CurrentLane())
	}
}

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
