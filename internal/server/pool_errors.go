package server

import (
	"errors"
	"net/http"
	"strings"

	"deepaicode2api/internal/deepai"
	"deepaicode2api/internal/pool"
)

// quotaStaleAfter 是「按需复探」的触发阈值：使用中的账号额度信息
// 超过这个时间就异步刷新一次。与调度侧的 pool.QuotaStaleAfter 对齐 ——
// 调度在同一阈值之后不再信任 balance=0，两边判断必须一致。
const quotaStaleAfter = pool.QuotaStaleAfter

// --- 号池相关的对外错误 ---
//
// 这一层留在 server 而不是 poolsvc：它是**响应文案**（HTTP 状态码 + 可行动的
// 错误体），与「号池怎么调度」无关。poolsvc 只回事实（忙 / 没有可调度账号）。

// errAllAccountsBusy 表示所有候选账号都达到了并发上限。
//
// 用 429 而不是 502：这是可重试的背压，客户端应稍后重试。
func errAllAccountsBusy() error {
	return &deepai.HTTPError{
		Op:     "账号调度",
		Status: http.StatusTooManyRequests,
		Body:   []byte(`{"error":{"message":"所有账号并发已满，请稍后重试","type":"rate_limit_error"}}`),
	}
}

// blockedReason 把上游错误码拼进账号的 last_err。
//
// 光存 message 会丢掉 code：全池被拦时要靠它区分 social_login_required /
// email_unverified，才能给调用方一个能行动的错误。
func blockedReason(he *deepai.HTTPError) string {
	if code := he.Code(); code != "" {
		return code + ": " + he.Error()
	}
	return he.Error()
}

// noAccountError 在没有可选账号时给出可执行的错误。
//
// 池里全部账号都被上游要求改用 Google/Apple 登录时，不能只回一句
// 「全部账号均不可用」——那是运营层面的策略变更，必须让调用方看见原因。
func (s *Server) noAccountError() error {
	accts := s.Store.Accounts()
	if len(accts) == 0 {
		return errors.New("没有可用账号，请先在控制台导入桌面端凭据或添加 API Key")
	}
	blocked := 0
	for _, a := range accts {
		if a.Status == deepai.StatusBlocked {
			blocked++
		}
	}
	if blocked > 0 && blocked == len(accts) {
		// 全部被拦时要把原因说清楚：只报“全部不可用”会让人以为网关坏了。
		social, unverified := 0, 0
		for _, a := range accts {
			switch {
			case strings.Contains(a.LastErr, deepai.CodeSocialLogin):
				social++
			case strings.Contains(a.LastErr, deepai.CodeEmailUnverified):
				unverified++
			}
		}
		if unverified > 0 && social == 0 {
			return &deepai.HTTPError{
				Op:     "账号调度",
				Status: http.StatusServiceUnavailable,
				Body:   []byte(`{"error":{"message":"全部账号都被上游判定为邮箱未验证（email_unverified），请先验证邮箱后重试","type":"email_unverified","code":"email_unverified"}}`),
			}
		}
		if social > 0 {
			return &deepai.HTTPError{
				Op:     "账号调度",
				Status: http.StatusServiceUnavailable,
				Body:   []byte(`{"error":{"message":"全部账号已被上游要求改用 Google/Apple 登录（social_login_required）；邮箱密码注册的账号无法再推理","type":"social_login_required","code":"social_login_required"}}`),
			}
		}
		return &deepai.HTTPError{
			Op:     "账号调度",
			Status: http.StatusServiceUnavailable,
			Body:   []byte(`{"error":{"message":"全部账号都被上游拦截（策略 / 登录态），请到控制台查看各账号的 last_err","type":"account_blocked","code":"account_blocked"}}`),
		}
	}
	return errors.New("全部账号均不可用")
}
