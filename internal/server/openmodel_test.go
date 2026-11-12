package server

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"sync/atomic"
	"testing"

	"deepaicode2api/internal/deepai"
)

// inflightCount 读取当前在途账号槽位数（仅测试用）。
func inflightCount(s *Server) int {
	s.inflightMu.Lock()
	defer s.inflightMu.Unlock()
	return len(s.inflight)
}

// TestShapeFallbackRetryKeepsIdleTimeoutGuard 覆盖「请求体形状回退」这条 2xx 返回路径。
//
// 两条返回 2xx 的路径（正常响应 / 回退重试）必须包同样的两层：空闲超时 + 归还
// 账号槽位。历史上回退路径只包了 releaseBody，上游发完首帧挂住时，槽位与连接会
// 一直挂到客户端断开为止——正是 idle.go 里描述的那个坑，只是漏了一条分支。
func TestShapeFallbackRetryKeepsIdleTimeoutGuard(t *testing.T) {
	var chatCalls atomic.Int32
	rejected := `{"error":{"message":"This request is not one DeepAI Code sends.","code":"invalid_request_error"}}`
	u := newUpstream(t, func(w http.ResponseWriter, r *http.Request) {
		// ModelURL 会拼上 /deepai_code 前缀，这里比较完整路径。
		if r.URL.Path != "/deepai_code"+deepai.PathChat {
			http.NotFound(w, r)
			return
		}
		if chatCalls.Add(1) == 1 {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, rejected)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"x","model":"m","choices":[{"index":0,"message":{"role":"assistant","content":"hi"}}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`)
	})
	s, _ := newTestServer(t, u)

	body, err := json.Marshal(map[string]any{
		"model":    "glm-5.3-flash",
		"stream":   false,
		"messages": []any{map[string]any{"role": "user", "content": "hi"}},
	})
	if err != nil {
		t.Fatalf("序列化请求: %v", err)
	}

	resp, acct, err := s.openModel(context.Background(), deepai.PathChat, body, "", "")
	if err != nil {
		t.Fatalf("openModel: %v", err)
	}
	if acct == nil {
		t.Fatal("成功路径应返回账号")
	}
	if got := chatCalls.Load(); got != 2 {
		t.Fatalf("形状回退应重试一次，实际 chat 调用 %d 次", got)
	}
	if _, ok := resp.Body.(*idleTimeoutBody); !ok {
		t.Fatalf("回退路径的响应体未包空闲超时（实际 %T）", resp.Body)
	}
	if n := inflightCount(s); n != 1 {
		t.Fatalf("请求进行中在途槽位 = %d，期望 1", n)
	}

	if _, err := io.ReadAll(resp.Body); err != nil {
		t.Fatalf("读响应体: %v", err)
	}
	if err := resp.Body.Close(); err != nil {
		t.Fatalf("关闭响应体: %v", err)
	}
	if n := inflightCount(s); n != 0 {
		t.Fatalf("关闭后在途槽位 = %d，期望 0（releaseBody 未生效）", n)
	}
}
