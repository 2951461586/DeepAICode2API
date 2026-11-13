package server

import (
	"time"

	"deepaicode2api/internal/deepai"
)

// upstreamAction 是一次上游拒绝之后网关应当采取的动作。
//
// 把判定从 openModel 的 if 链里抽出来，是为了：
//   - 让「哪个状态码 / 错误码该换号、该拉黑、该等、该回传」变成可表驱动测试的
//     纯函数（历史上换号与终态语义改过多次）；
//   - 新增错误码（例如 2026-11-13 的 theos_exhausted）时只改一处；
//   - openModel 只剩下「执行动作」，分支不再随错误码增长。
type upstreamAction int

const (
	// actRetryOther 换一个账号重试（401/403 普通失败、429、5xx 等）。
	actRetryOther upstreamAction = iota
	// actReturn 直接把上游错误回传给调用方（请求本身的问题 / 缺机器指纹）。
	actReturn
	// actBlockAndRetry 标记 blocked 后换号（邮箱未验证：号本身登录态无效）。
	actBlockAndRetry
	// actBlockAndReturn 标记 blocked 后直接回传（账号级策略拦截）。
	actBlockAndReturn
	// actProbeLane 先探测车道，命中新车道就换道重试。
	actProbeLane
	// actWaitCapacity 容量拒绝：池里有空号就换号，整池忙才等 Retry-After。
	actWaitCapacity
	// actQuotaExhausted 账号当日额度用完：标记到 refresh_at 并换号。
	actQuotaExhausted
	// actShapeFallback 请求体形状被拒：补桌面端形状后同号重试一次。
	actShapeFallback
)

// classifyUpstreamError 是纯函数：错误 → 动作。
//
// 判定顺序有意义（先账号终态，再车道/额度/容量，最后才看通用可重试性）。
func classifyUpstreamError(e *deepai.HTTPError) upstreamAction {
	switch {
	case e == nil:
		return actReturn
	case e.EmailUnverified():
		return actBlockAndRetry
	case e.SocialLoginRequired():
		return actBlockAndReturn
	case e.MachineRequired():
		return actReturn
	case e.LaneUnsupported():
		return actProbeLane
	case e.QuotaExhausted():
		return actQuotaExhausted
	case e.CapacityWaiting():
		return actWaitCapacity
	case e.BodyRejected():
		return actShapeFallback
	case e.Retryable():
		return actRetryOther
	}
	return actReturn
}

// quotaExhaustedFallback 是 theos_exhausted 没给 refresh_at 时的恢复等待。
//
// 上游文案是 “They refresh at <RFC3339>”，一般都能解析；解析不到时取 6h，
// 与 check_in_refused 的「Try again tomorrow」同量级。
const quotaExhaustedFallback = 6 * time.Hour
