package model

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// 上游错误词汇。
//
// 放在 model 而不是传输层：store / pool / poolsvc / server 都要用
// 「这是不是 social_login_required」「额度是不是今天用完」这类**领域判断**
// 来改账号状态，而它们不该为此依赖 HTTP 客户端。

// HTTPError 是上游返回的非 2xx 响应。
type HTTPError struct {
	Op     string
	Status int
	Body   []byte
}

// Error 返回可读错误信息（优先使用上游 error.message）。
func (e *HTTPError) Error() string {
	var payload struct {
		Error struct {
			Message string `json:"message"`
			Type    string `json:"type"`
			Code    string `json:"code"`
		} `json:"error"`
	}
	if json.Unmarshal(e.Body, &payload) == nil && payload.Error.Message != "" {
		return fmt.Sprintf("%s: %s", e.Op, payload.Error.Message)
	}
	body := strings.TrimSpace(string(e.Body))
	if len(body) > 200 {
		body = body[:200] + "…"
	}
	if body == "" {
		return fmt.Sprintf("%s: HTTP %d", e.Op, e.Status)
	}
	return fmt.Sprintf("%s: HTTP %d: %s", e.Op, e.Status, body)
}

// Code 返回上游 error.code。
func (e *HTTPError) Code() string {
	var payload struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if json.Unmarshal(e.Body, &payload) == nil {
		return payload.Error.Code
	}
	return ""
}

// Retryable 报告该错误是否值得换账号 / 重试。
//
// 400 / 404 / 422 是请求本身的问题，重试无意义；
// 401 / 403 / 409 / 429 / 5xx 与账号或容量相关，可以重试。
// 例外：策略类错误码（social_login_required / machine_required /
// lane_version_unsupported）是账号或客户端终态，换号 / 重试都没用。
func (e *HTTPError) Retryable() bool {
	switch e.Code() {
	case CodeSocialLogin, CodeMachineRequired, CodeLaneUnsupported:
		return false
	}
	switch e.Status {
	case http.StatusUnauthorized, http.StatusForbidden, http.StatusConflict,
		http.StatusTooManyRequests, http.StatusInternalServerError,
		http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return true
	}
	return e.Status >= 500
}

// AuthFailed 报告是否为账号鉴权失败。
func (e *HTTPError) AuthFailed() bool {
	if e.Status == http.StatusUnauthorized {
		return true
	}
	code := e.Code()
	return code == CodeAuthRequired || code == CodeInvalidAPIKey || code == CodeEmailUnverified
}

// SocialLoginRequired 报告账号是否被上游要求改用 Google/Apple 登录。
//
// 0.3.19 起上游对“邮箱 + 密码”注册的账号在 /theos 与推理端点返回 403；
// 这是账号级策略拦截，不是鉴权失败，也不是可重试错误。
func (e *HTTPError) SocialLoginRequired() bool { return e.Code() == CodeSocialLogin }

// MachineRequired 报告是否因缺少机器指纹被拒（计量模型专属）。
func (e *HTTPError) MachineRequired() bool { return e.Code() == CodeMachineRequired }

// EmailUnverified 报告上游是否因邮箱未验证拒绝该账号。
//
// 与普通 403 不同：账号的登录态无效，换不换号都一样，
// 应标记后跳过（而不是当成可重试失败反复换号）。
func (e *HTTPError) EmailUnverified() bool { return e.Code() == CodeEmailUnverified }

// LaneUnsupported 报告车道版本不再被上游接受（426）。
func (e *HTTPError) LaneUnsupported() bool {
	return e.Status == http.StatusUpgradeRequired || e.Code() == CodeLaneUnsupported
}

// BodyRejected 报告上游是否因“请求体不是 DeepAI Code 的形状”而拒绝。
//
// 上游对新车道加了请求体校验，典型响应是 400 + code=invalid_request_error，
// message 有两种实测文案：
//   - `This request is not one DeepAI Code sends.`（旧，2026-11-07 起）
//   - `This request cannot be accepted.`（2026-11-13 起，0.3.36 时代）
//
// 两版都要能识别：漏了新文案，ChatBodyFallback（补 system + 收敛字段）就不会重试，
// 调用方直接吃 400。
//
// 2026-11-15 起再加一层兜底：文案可能再次更换，而 code 一直是
// invalid_request_error。只要 400 + 该 code 就按形状拒绝处理（仅 chat 路径会
// 真正重试）；补形状后仍失败时，第二个错误会原样回传，不会把原因吞掉。
func (e *HTTPError) BodyRejected() bool {
	if e.Status != http.StatusBadRequest {
		return false
	}
	body := strings.ToLower(string(e.Body))
	if strings.Contains(body, CodeNotDeepAICodeMsg) || strings.Contains(body, CodeNotAcceptedMsg) {
		return true
	}
	return e.Code() == CodeInvalidReq
}

// QuotaExhausted 报告账号当日额度是否用完（付费模型 / 余额为负）。
func (e *HTTPError) QuotaExhausted() bool { return e.Code() == CodeTheosExhausted }

// QuotaResetAt 解析额度类错误里的 refresh_at（RFC3339）。
//
// body 形如 {"error":{"code":"theos_exhausted","refresh_at":"2026-11-14T00:05:52+00:00"}}；
// 解析不到时用 fallback（调用方通常传 6h，与「Try again tomorrow」的节奏一致）。
func (e *HTTPError) QuotaResetAt(fallback time.Duration, now time.Time) time.Time {
	var payload struct {
		Error struct {
			RefreshAt string `json:"refresh_at"`
		} `json:"error"`
	}
	if len(e.Body) > 0 && json.Unmarshal(e.Body, &payload) == nil && payload.Error.RefreshAt != "" {
		if ts, err := time.Parse(time.RFC3339, payload.Error.RefreshAt); err == nil && ts.After(now) {
			return ts
		}
	}
	return now.Add(fallback)
}

// CheckInRefused 报告签到是否被上游按「当天重启次数」限流。
//
// 它与账号不可用不同：推理照常 200，只是会落在替身模型分支
// （见 docs/recon.md §5.1），因此不能把账号标成 error。
func (e *HTTPError) CheckInRefused() bool { return e.Code() == CodeCheckInRefused }

// IsSocialLoginRequired 报告 err 是否为 social_login_required。
func IsSocialLoginRequired(err error) bool {
	var he *HTTPError
	return errors.As(err, &he) && he.SocialLoginRequired()
}

// IsCheckInRefused 报告 err 是否为签到被限流（check_in_refused）。
func IsCheckInRefused(err error) bool {
	var he *HTTPError
	return errors.As(err, &he) && he.CheckInRefused()
}

// CapacityWaiting 报告是否为“会话忙”的容量拒绝。
func (e *HTTPError) CapacityWaiting() bool {
	return e.Status == http.StatusConflict && e.Code() == "session_busy"
}
