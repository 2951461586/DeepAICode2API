package server

import (
	"strings"

	"deepaicode2api/internal/deepai"
)

// 出口代理的健康观测。
//
// 线上形态（2026-11-22，16 个账号各绑一个住宅代理）：失败几乎全部发生在
// 代理侧——`Gateway Timeout`(504)、`unexpected EOF`、`context deadline
// exceeded`——而账号本身是好的（TCP 探测 16/16 全通）。这类故障的正确处置
// 是「换出口」，不是「弃号」，所以这里只记代理的连续失败，不做账号级惩罚。

// noteProxyResult 记录一次出口结果：ok=true 表示这次确实拿到了上游响应
// （哪怕是 4xx/5xx —— 那已经证明 DNS/TCP/TLS/CONNECT 全通）。
func (s *Server) noteProxyResult(acct *deepai.Account, ok bool) {
	if s == nil || s.proxyHealth == nil || acct == nil {
		return
	}
	key := proxyKey(acct)
	if key == "" {
		return // 直连没有可观测的出口
	}
	s.proxyHealth.Report(key, ok)
}

// proxyCooling 报告账号绑定的出口是否正处于冷却窗口。
func (s *Server) proxyCooling(acct *deepai.Account) bool {
	if s == nil || s.proxyHealth == nil || acct == nil {
		return false
	}
	return s.proxyHealth.Cooling(proxyKey(acct))
}

// avoidCoolingProxy 是传给 PickAvoiding 的谓词：出口冷却中的账号排到候选末尾。
func (s *Server) avoidCoolingProxy(a *deepai.Account) bool { return s.proxyCooling(a) }

// ProxyCoolingCount 返回当前处于冷却的出口数（概览 / 诊断用）。
func (s *Server) ProxyCoolingCount() int {
	if s == nil || s.proxyHealth == nil {
		return 0
	}
	return s.proxyHealth.CoolingCount()
}

// proxyKey 是出口的台账键：原始代理串（不同凭据不能混合统计）。
func proxyKey(acct *deepai.Account) string {
	if acct == nil {
		return ""
	}
	return strings.TrimSpace(acct.Proxy)
}
