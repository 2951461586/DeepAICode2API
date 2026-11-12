package server

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"deepaicode2api/internal/deepai"
	"deepaicode2api/internal/store"
)

// TestReqLogDoneIsIdempotent 锁死「同一个请求只记一条日志」。
//
// 各 handler 用值类型 reqLog 传递，done 可能在多个分支被各调一次（提前返回 +
// defer 兜底）。没有幂等标记时，一个请求会被记成两条，吞吐与错误率全部翻倍。
func TestReqLogDoneIsIdempotent(t *testing.T) {
	s, _ := newTestServer(t, nil)
	rl := s.newReqLog("sk-test", nil, "glm-5.3-flash", "chat", false, time.Now(), 12)

	rl.done(3, Usage{PromptTokens: 5, CompletionTokens: 2}, nil)
	rl.done(3, Usage{PromptTokens: 5, CompletionTokens: 2}, nil) // 第二次应被忽略
	copied := rl
	copied.done(3, Usage{}, nil) // 副本共享同一标记，同样被忽略

	logs := s.Store.Logs(10)
	if len(logs) != 1 {
		t.Fatalf("日志条数 = %d，期望 1（done 必须幂等）", len(logs))
	}
	if logs[0].Status != http.StatusOK || logs[0].PromptLen != 12 {
		t.Fatalf("日志字段不对: %+v", logs[0])
	}
}

// TestStatusOfUsesUpstreamStatusCode 保证日志里的状态码与客户端收到的一致。
//
// 旧实现把所有错误一律记成 502，而上游的 400 / 426 / 429 是会被 writeUpstreamErr
// 原样回给客户端的：控制台的「状态 ≥」筛选与错误归因因此长期失真。
func TestStatusOfUsesUpstreamStatusCode(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want int
	}{
		{"无错误", nil, http.StatusOK},
		{"上游 400", &deepai.HTTPError{Op: "聊天", Status: 400, Body: []byte(`{"error":"bad"}`)}, 400},
		{"上游 426", &deepai.HTTPError{Op: "聊天", Status: 426}, 426},
		{"上游 429", &deepai.HTTPError{Op: "聊天", Status: 429}, 429},
		{"包装后的上游 403", wrap(&deepai.HTTPError{Op: "聊天", Status: 403}), 403},
		{"普通错误回 502", errors.New("boom"), http.StatusBadGateway},
		// 客户端断开 / 我们自己的超时都要跟 writeUpstreamErr 的实际响应对齐：
		// 前者什么都不写（连接已经没了），后者写 504。以前一律记 502，会让
		// 「上游 5xx」与「客户端走了」「我们超时了」在日志里混成一堆。
		{"上下文取消记 499", context.Canceled, store.StatusClientGone},
		{"包装后的上下文取消", wrap(context.Canceled), store.StatusClientGone},
		{"上下文超时记 504", context.DeadlineExceeded, http.StatusGatewayTimeout},
		{"异常状态码回 502", &deepai.HTTPError{Op: "x", Status: 0}, http.StatusBadGateway},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := statusOf(c.err); got != c.want {
				t.Fatalf("statusOf = %d，期望 %d", got, c.want)
			}
		})
	}
}

// wrap 给错误套一层 %w，验证 statusOf 用 errors.As 而不是类型断言。
func wrap(err error) error { return errors.Join(errors.New("外层"), err) }

// TestPassthroughNonJSONIsLogged 覆盖「上游回的既不是 JSON 也不是流」的分支。
//
// 这个分支曾经直接 return：客户端拿到原文 200，但日志里**一条记录都没有**，
// 于是「上游偶发返回 HTML 登录页」在控制台完全看不见。
func TestPassthroughNonJSONIsLogged(t *testing.T) {
	u := newUpstream(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, "<html><body>login required</body></html>")
	})
	s, key := newTestServer(t, u)
	s.Store.SetModels([]deepai.Model{{ID: "resp-model", API: "openai-responses"}}, "resp-model")

	rec := do(t, s, http.MethodPost, "/v1/responses", key,
		`{"model":"resp-model","input":"hi"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("状态码 = %d（应保持原样透传 200）：%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "login required") {
		t.Fatalf("原文未透传: %s", rec.Body.String())
	}

	logs := s.Store.Logs(10)
	if len(logs) != 1 {
		t.Fatalf("日志条数 = %d，期望 1（该分支不能漏记）", len(logs))
	}
	got := logs[0]
	if got.Status != http.StatusOK {
		t.Fatalf("日志状态 = %d，期望 200（客户端实际收到 200）", got.Status)
	}
	if !strings.Contains(got.Err, "不是合法 JSON") {
		t.Fatalf("日志未记录失败原因: %+v", got)
	}
	if stats := s.Store.Stats(); stats.FailedRequests != 1 {
		t.Fatalf("FailedRequests = %d，期望 1（Err 非空即计入失败）", stats.FailedRequests)
	}
}
