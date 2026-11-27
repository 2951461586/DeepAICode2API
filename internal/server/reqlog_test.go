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
	"deepaicode2api/internal/gateway"
	"deepaicode2api/internal/respstate"
	"deepaicode2api/internal/store"
)

// TestReqLogDoneIsIdempotent 锁死「同一个请求只记一条日志」。
//
// 各 handler 用值类型 reqLog 传递，done 可能在多个分支被各调一次（提前返回 +
// defer 兜底）。没有幂等标记时，一个请求会被记成两条，吞吐与错误率全部翻倍。
func TestReqLogDoneIsIdempotent(t *testing.T) {
	s, _ := newTestServer(t, nil)
	rl := s.newReqLog("sk-test", nil, "glm-5.3-flash", "chat", false, time.Now(), 12)

	rl.done(3, gateway.Usage{PromptTokens: 5, CompletionTokens: 2}, nil)
	rl.done(3, gateway.Usage{PromptTokens: 5, CompletionTokens: 2}, nil) // 第二次应被忽略
	copied := rl
	copied.done(3, gateway.Usage{}, nil) // 副本共享同一标记，同样被忽略

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
			if got := StatusOf(c.err); got != c.want {
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

// TestReqLogRecordsOutboundBytes 锁定 issue #5 / #7 的排障字段：
// 出站请求体字节数必须落进日志，接近上游静默截断阈值（~196 KB）时要打标记。
func TestReqLogRecordsOutboundBytes(t *testing.T) {
	limit := respstate.ContextBodyBytes()
	if limit <= 0 {
		t.Fatalf("默认字節閘門不應關閉（actual %d）", limit)
	}

	t.Run("接近上限", func(t *testing.T) {
		s, _ := newTestServer(t, nil)
		rl := s.newReqLog("sk-test", nil, "glm-5.3-flash", "chat", false, time.Now(), 12).
			withOutboundBytes(limit)
		rl.done(0, gateway.Usage{}, nil)

		logs := s.Store.Logs(1)
		if len(logs) != 1 {
			t.Fatalf("日志条数 = %d，期望 1", len(logs))
		}
		if logs[0].BodyBytes != limit {
			t.Fatalf("BodyBytes = %d，期望 %d", logs[0].BodyBytes, limit)
		}
		if !logs[0].BodyNearCap {
			t.Fatal("达到上限时应标记 BodyNearCap")
		}
	})

	t.Run("普通大小", func(t *testing.T) {
		s, _ := newTestServer(t, nil)
		rl := s.newReqLog("sk-test", nil, "glm-5.3-flash", "chat", false, time.Now(), 12).
			withOutboundBytes(4096)
		rl.done(0, gateway.Usage{}, nil)

		logs := s.Store.Logs(1)
		if len(logs) != 1 {
			t.Fatalf("日志条数 = %d，期望 1", len(logs))
		}
		if logs[0].BodyBytes != 4096 {
			t.Fatalf("BodyBytes = %d，期望 4096", logs[0].BodyBytes)
		}
		if logs[0].BodyNearCap {
			t.Fatal("4 KB 不應標記 BodyNearCap")
		}
	})
}

// TestReqLogRecordsClientIdentity 锁定「谁在发」的排障字段。
//
// 2026-11-27 线上 /responses 约 10% 的 400 全是上游「conversation exceeds the
// context window」，但日志只有体积与消息数，网关侧无法归因到具体客户端；只能挨个
// 让下游自查。UA / IP 落盘之后，同类问题下一次可以在日志里直接定位来源。
func TestReqLogRecordsClientIdentity(t *testing.T) {
	cases := []struct {
		name   string
		remote string
		header map[string]string
		wantIP string
	}{
		{"Cloudflare 优先", "10.0.0.5:443", map[string]string{
			"CF-Connecting-IP": "203.0.113.7",
			"X-Forwarded-For":  "198.51.100.9, 10.0.0.5",
		}, "203.0.113.7"},
		{"XFF 取首段", "10.0.0.5:443", map[string]string{
			"X-Forwarded-For": "198.51.100.9, 10.0.0.5",
		}, "198.51.100.9"},
		{"直连回退 RemoteAddr", "127.0.0.1:51234", nil, "127.0.0.1"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s, _ := newTestServer(t, nil)
			req, err := http.NewRequest(http.MethodPost, "/v1/responses", nil)
			if err != nil {
				t.Fatal(err)
			}
			req.RemoteAddr = c.remote
			for k, v := range c.header {
				req.Header.Set(k, v)
			}
			req.Header.Set("User-Agent", "codex_cli_rs/0.61.0")

			s.newReqLog("sk-test", nil, "glm-5.3-flash", "responses", true, time.Now(), 12).
				withClient(req).done(0, gateway.Usage{}, nil)

			logs := s.Store.Logs(1)
			if len(logs) != 1 {
				t.Fatalf("日志条数 = %d，期望 1", len(logs))
			}
			if logs[0].IP != c.wantIP {
				t.Fatalf("IP = %q，期望 %q", logs[0].IP, c.wantIP)
			}
			if logs[0].UA != "codex_cli_rs/0.61.0" {
				t.Fatalf("UA = %q", logs[0].UA)
			}
		})
	}

	t.Run("UA 截断到 120 字符", func(t *testing.T) {
		s, _ := newTestServer(t, nil)
		req, err := http.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
		if err != nil {
			t.Fatal(err)
		}
		req.RemoteAddr = "127.0.0.1:1"
		req.Header.Set("User-Agent", strings.Repeat("长", 400))

		s.newReqLog("sk-test", nil, "glm-5.3-flash", "chat", false, time.Now(), 1).
			withClient(req).done(0, gateway.Usage{}, nil)

		got := []rune(s.Store.Logs(1)[0].UA)
		if len(got) != 120 {
			t.Fatalf("UA 字符数 = %d，期望 120", len(got))
		}
	})

	t.Run("nil 请求不 panic", func(t *testing.T) {
		s, _ := newTestServer(t, nil)
		s.newReqLog("sk-test", nil, "glm-5.3-flash", "chat", false, time.Now(), 1).
			withClient(nil).done(0, gateway.Usage{}, nil)
		if got := s.Store.Logs(1)[0].UA; got != "" {
			t.Fatalf("UA 应为空，得到 %q", got)
		}
	})
}

// TestReqLogRecordsSessionFingerprints 锁住「会话 / 前缀指纹」落盘。
//
// 两条指纹配对照料的是同一类问题：命中率 0 到底是客户端在改前缀，还是网关
// 换了账号 / 上游 TTL 过期。任一字段丢失，这两种原因就重新混在一起。
func TestReqLogRecordsSessionFingerprints(t *testing.T) {
	s, _ := newTestServer(t, nil)
	payload := map[string]any{
		"model":    "glm-5.3-flash",
		"system":   "你是代码助手",
		"messages": []any{map[string]any{"role": "system", "content": "你是代码助手"}},
		"tools":    []any{map[string]any{"name": "read_file"}},
	}
	rl := s.newReqLog("sk-test", nil, "glm-5.3-flash", "chat", false, time.Now(), 3).
		withHistory(payload).
		withAffinity("client-sess-1")
	rl.done(0, gateway.Usage{}, nil)

	logs := s.Store.Logs(1)
	if len(logs) != 1 {
		t.Fatalf("日志条数 = %d，期望 1", len(logs))
	}
	got := logs[0]
	if got.Affinity != "client-sess-1" {
		t.Fatalf("Affinity = %q", got.Affinity)
	}
	if want := prefixFingerprint(payload); got.Prefix != want {
		t.Fatalf("Prefix = %q，期望 %q", got.Prefix, want)
	}
	if got.Prefix == "" || got.Prefix[:3] != "pf-" {
		t.Fatalf("Prefix 形态不对: %q", got.Prefix)
	}

	// affinity 来自请求头/体，长度不可信：落盘前必须截断。
	long := strings.Repeat("a", 300)
	rl2 := s.newReqLog("sk-test", nil, "glm-5.3-flash", "chat", false, time.Now(), 1).withAffinity(long)
	rl2.done(0, gateway.Usage{}, nil)
	if n := len([]rune(s.Store.Logs(1)[0].Affinity)); n != 64 {
		t.Fatalf("Affinity 字符数 = %d，期望 64", n)
	}
}
