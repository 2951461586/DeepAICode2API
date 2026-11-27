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

// sameAccountRetryable 报告这个上游拒绝是否值得「在同一个号上再试一次」。
//
// 判据是「账号本身没问题、链路抖了一下」：
//   - 5xx（502 unexpected EOF 之类）：上游或边缘的一次失败，号还能用；
//   - 429：账号级限流，同号重试只会立刻再被拒，必须换号；
//   - 403 / 400 / 426：策略、语义、车道问题，重试同一个号没有意义。
//
// 为什么这件事值得单独成函数：换号的隐性代价是上游前缀缓存整段冷启动。
// 线上日志里同一段会话命中率 98~100%，只有换号那几次是 0%——而缓存命中
// 折价的是首字节延迟与上游成本，不会以错误的形式暴露出来。
func sameAccountRetryable(e *deepai.HTTPError) bool {
	if e == nil {
		return false
	}
	switch e.Status {
	case 500, 502, 503, 504:
		return true
	}
	return false
}

// quotaExhaustedFallback 是 theos_exhausted 没给 refresh_at 时的恢复等待。
//
// 上游文案是 “They refresh at <RFC3339>”，一般都能解析；解析不到时取 6h，
// 与 check_in_refused 的「Try again tomorrow」同量级。
const quotaExhaustedFallback = 6 * time.Hour
