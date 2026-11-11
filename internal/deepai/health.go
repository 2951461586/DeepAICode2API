package deepai

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

// 账号健康度的判定就在这里。
//
// 背景（docs/changes.md §45）：`status` 是**上一次请求的结果**，不是账号的
// 永久属性。上游的 EOF / 429「Too many requests today」是按次出现的，只要
// 把每次失败都写成 status=error 且永不清除，徽章就会长期停在 1/16，
// 而号池其实一直在正常出活。所以：
//
//   - Transient 先区分「这次没成」与「这个号坏了」；
//   - AccountReady 再把「错误是否够新」折算进健康度（ErrorStaleAfter）。

// Transient 报告一次失败是否属于「瞬时」类别：只说明这次请求没成，
// 不代表账号坏了（不该被打成 error，也不该被当成健康度依据）。
//
// 判为瞬时的：网络层错误（EOF / reset / 超时）、429 频控、5xx、409 session_busy
// （容量拒绝）、check_in_refused（能推理、只是当天签到到顶）。
// 不瞬时的：401 / 无效 Key、social_login_required、email_unverified，
// 以及**今日额度到顶**（`Too many requests today`）—— 后者是“今天真的不能
// 用”，要如实显示成 degraded，而不是当成噪声抹掉。
func Transient(err error) bool {
	if err == nil {
		return false
	}
	var he *HTTPError
	if errors.As(err, &he) {
		// 额度到顶优先判定：它即使以 429 返回，也不是“歇一下就好”。
		if isDailyQuota(string(he.Body)) {
			return false
		}
		switch {
		case he.Status == http.StatusTooManyRequests:
			return true
		case he.Status >= 500:
			return true
		case he.Status == http.StatusConflict && he.Code() == CodeSessionBusy:
			// 容量拒绝：整池都满时才看得到，和账号本身无关。
			return true
		case he.Code() == CodeCheckInRefused:
			// 签到限流：推理照常，只是当天签到额度到顶。
			return true
		}
		return false
	}
	if errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, io.EOF) ||
		errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return true
	}
	var ne net.Error
	if errors.As(err, &ne) {
		return true // 超时 / 临时网络错误
	}
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		return true
	}
	// 上游偶尔把“正在重启 / 网关挂了”塞进 message 而不是状态码，
	// 这类只靠类型判断不出来，文本兜底。注意：额度到顶不算。
	text := strings.ToLower(err.Error())
	if isDailyQuota(text) {
		return false
	}
	for _, needle := range []string{
		"unexpected eof",
		"connection reset",
		"broken pipe",
		"temporarily unavailable",
		"bad gateway",
		"service unavailable",
		"gateway timeout",
		"i/o timeout",
	} {
		if strings.Contains(text, needle) {
			return true
		}
	}
	return false
}

// isDailyQuota 识别“今日额度到顶”类文案。
//
// 这类失败当天不会自愈（要到 refresh_at 才重置），所以不能算瞬时噪声：
// 号池要能看出“这个号今天用不了了”。
func isDailyQuota(text string) bool {
	return strings.Contains(strings.ToLower(text), "too many requests today")
}

// AccountReady 报告账号在 now 时刻是否算「就绪」。
//
// 规则：
//   - 停用的不算；
//   - blocked（上游策略拦截）不算：它不会自愈，必须人工处理；
//   - status=error 但错误已经超过 ErrorStaleAfter 没再出现 → 算就绪
//     （那是历史噪声，调度器也从来没跳过 error 号）；
//   - 其余（ok / unknown / 新错）都算就绪，unknown 是「还没探过」，
//     刚导入时不该被算成坏号。
func AccountReady(a *Account, now time.Time) bool {
	if a == nil || !a.Enabled || a.Status == StatusBlocked {
		return false
	}
	if a.Status != StatusError {
		return true
	}
	return now.Sub(time.Unix(a.LastErrAt, 0)) >= ErrorStaleAfter
}

// AccountDegraded 报告账号是否「最近失败过」（error 且错误仍然新鲜）。
//
// health 用它给出第二个数字：就绪之外还有多少号正在出错。只显示 ready/total
// 会把「刚被 429 打了一下」和「真的坏了」混为一谈。
func AccountDegraded(a *Account, now time.Time) bool {
	if a == nil || !a.Enabled || a.Status != StatusError {
		return false
	}
	return now.Sub(time.Unix(a.LastErrAt, 0)) < ErrorStaleAfter
}
