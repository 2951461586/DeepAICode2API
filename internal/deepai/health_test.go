package deepai

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"testing"
	"time"
)

// 这一组测试锁的是「一次失败算不算账号坏了」的判据。
//
// 回归背景：旧实现把每次失败都写成 status=error 且永不清除，上游概率性的
// EOF / 429 会让徽章长期停在 1/16（docs/changes.md §45）。

func httpErr(status int, body string) error {
	return &HTTPError{Op: "上游", Status: status, Body: []byte(body)}
}

func TestTransient(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"意外 EOF", io.ErrUnexpectedEOF, true},
		{"EOF", io.EOF, true},
		{"超时", context.DeadlineExceeded, true},
		{"429 限流", httpErr(http.StatusTooManyRequests, `{}`), true},
		{"500", httpErr(http.StatusInternalServerError, `{}`), true},
		{"502", httpErr(http.StatusBadGateway, `{}`), true},
		{"容量拒绝 409", httpErr(http.StatusConflict, `{"error":{"code":"session_busy"}}`), true},
		{"签到限流", httpErr(http.StatusForbidden, `{"error":{"code":"check_in_refused"}}`), true},
		{"401 无效 Key", httpErr(http.StatusUnauthorized, `{"error":{"code":"invalid_api_key"}}`), false},
		{"social_login_required", httpErr(http.StatusForbidden, `{"error":{"code":"social_login_required"}}`), false},
		{"纯文本 unexpected EOF", errors.New("Get \"https://api.deepai.org\": unexpected EOF"), true},
		{"今日额度用尽", errors.New(`上游: DeepAI Code has restarted its check-in too many times today`), false},
		// 额度到顶当天不会自愈：要如实显示成 degraded，不能当噪声抹掉。
		{"今日额度到顶（文本）", errors.New("查询 Theos: Too many requests today"), false},
		{"今日额度到顶（429 体）", httpErr(http.StatusTooManyRequests, `{"error":{"message":"Too many requests today"}}`), false},
		{"网络错误", &net.OpError{Op: "dial", Err: errors.New("connection refused")}, true},
		{"普通业务错误", errors.New("boom"), false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Transient(c.err); got != c.want {
				t.Fatalf("Transient(%v) = %v，期望 %v", c.err, got, c.want)
			}
		})
	}
}

func TestAccountReady(t *testing.T) {
	now := time.Now()
	stale := now.Add(-defaultStale - time.Minute).Unix()
	fresh := now.Add(-time.Minute).Unix()

	cases := []struct {
		name string
		a    *Account
		want bool
	}{
		{"nil", nil, false},
		{"停用", &Account{Enabled: false, Status: "ok"}, false},
		{"ok", &Account{Enabled: true, Status: "ok"}, true},
		{"unknown 视为可用", &Account{Enabled: true, Status: "unknown"}, true},
		{"blocked 永不就绪", &Account{Enabled: true, Status: StatusBlocked}, false},
		{"新鲜的 error", &Account{Enabled: true, Status: StatusError, LastErrAt: fresh}, false},
		{"陈旧的 error 算自愈", &Account{Enabled: true, Status: StatusError, LastErrAt: stale}, true},
		// 老状态文件没有 last_err_at（=0）：按“历史噪声”处理，否则升级后
		// 所有历史 error 号会一直显示不健康。
		{"缺 last_err_at 的 error", &Account{Enabled: true, Status: StatusError}, true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := AccountReady(c.a, now); got != c.want {
				t.Fatalf("AccountReady = %v，期望 %v", got, c.want)
			}
		})
	}
}

func TestAccountDegraded(t *testing.T) {
	now := time.Now()
	fresh := now.Add(-time.Minute).Unix()
	stale := now.Add(-defaultStale - time.Minute).Unix()

	if !AccountDegraded(&Account{Enabled: true, Status: StatusError, LastErrAt: fresh}, now) {
		t.Fatal("新鲜的 error 应算 degraded")
	}
	if AccountDegraded(&Account{Enabled: true, Status: StatusError, LastErrAt: stale}, now) {
		t.Fatal("陈旧的 error 不该算 degraded")
	}
	if AccountDegraded(&Account{Enabled: true, Status: "ok"}, now) {
		t.Fatal("ok 不该算 degraded")
	}
	if AccountDegraded(&Account{Enabled: false, Status: StatusError, LastErrAt: fresh}, now) {
		t.Fatal("停用账号不该算 degraded")
	}
}

// defaultStale 是 ErrorStaleAfter 的别名，单独取出来避免表里直接依赖常量名。
const defaultStale = ErrorStaleAfter
