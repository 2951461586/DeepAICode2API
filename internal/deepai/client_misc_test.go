package deepai

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestDoTransportError 覆盖连接失败：错误必须原样上抛并被判定为瞬时故障。
func TestDoTransportError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := srv.URL
	srv.Close() // 端口已关闭，后续请求连接被拒

	c := NewClient()
	c.Origin = url

	_, _, err := c.FetchModels(context.Background(), nil)
	if err == nil {
		t.Fatal("连接失败应返回错误")
	}
	if !IsTransient(err) {
		t.Fatalf("连接被拒应判定为瞬时故障，得到 %v", err)
	}

	if _, err := c.OpenModel(context.Background(), &Account{APIKey: "k"}, PathChat, []byte(`{}`), "aff"); err == nil {
		t.Fatal("OpenModel 连接失败应返回错误")
	}
	if _, err := c.Theos(context.Background(), &Account{APIKey: "k"}); err == nil {
		t.Fatal("Theos 连接失败应返回错误")
	}
	if _, err := c.postJSON(context.Background(), nil, PathWebSearch, []byte(`{}`), "联网搜索"); err == nil {
		t.Fatal("postJSON 连接失败应返回错误")
	}
	if _, err := c.Transcribe(context.Background(), nil, "", "", []byte("x")); err == nil {
		t.Fatal("Transcribe 连接失败应返回错误")
	}
	if _, err := c.ExchangeCode(context.Background(), "code"); err == nil {
		t.Fatal("ExchangeCode 连接失败应返回错误")
	}
	if _, err := c.DetectLane(context.Background(), nil); err == nil {
		t.Fatal("DetectLane 连接失败应返回错误")
	}
	if c.Lane() != LaneVersion {
		t.Fatalf("探测失败不应改车道，实际 %q", c.Lane())
	}
}

// TestDetectLaneServerErrorFallback 覆盖「全是 5xx」：应给出兜底错误，
// 因为 5xx 说明上游抖动而不是车道不对（此时 lastErr 为空）。
func TestDetectLaneServerErrorFallback(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	})
	_, err := c.DetectLane(context.Background(), nil)
	if err == nil || !strings.Contains(err.Error(), "所有候选车道都被上游拒绝") {
		t.Fatalf("全 5xx 应给出兜底错误，得到 %v", err)
	}
}
