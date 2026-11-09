package server

import (
	"io"
	"net/http"
	"sync"
	"testing"
)

// 426 → 用免鉴权 /models 探到新车道 → 换道重试成功，并把结果落盘。
func TestLaneUnsupportedRefreshesAndRetries(t *testing.T) {
	var mu sync.Mutex
	var chatLanes []string
	u := newUpstream(t, func(w http.ResponseWriter, r *http.Request) {
		lane := r.Header.Get("X-DeepAI-Code-Theos")
		mu.Lock()
		chatLanes = append(chatLanes, lane)
		mu.Unlock()
		if lane != "5" {
			w.WriteHeader(http.StatusUpgradeRequired)
			_, _ = io.WriteString(w, `{"error":{"code":"lane_version_unsupported","message":"old build"}}`)
			return
		}
		if r.URL.Path == "/deepai_code/models" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"models":[{"id":"glm-5.3-flash","name":"GLM","reasoning":true,`+
				`"input":["text"],"contextWindow":1000,"maxTokens":100}],"default":"glm-5.3-flash"}`)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, okChatResponse)
	})
	s, key := newTestServer(t, u)

	if got := s.CurrentLane(); got == "5" {
		t.Fatalf("初始车道不应是 5: %q", got)
	}
	rec := do(t, s, http.MethodPost, "/v1/chat/completions", key, chatProbeBody)
	if rec.Code != http.StatusOK {
		t.Fatalf("换道后应成功，实际 %d: %s", rec.Code, rec.Body.String())
	}
	if s.CurrentLane() != "5" {
		t.Fatalf("探测到的车道应生效，实际 %q", s.CurrentLane())
	}
	if got := s.Store.Settings().Lane; got != "5" {
		t.Fatalf("车道应落盘，实际 %q", got)
	}

	// 第二次请求直接用新车道，不再触发探测。
	mu.Lock()
	before := len(chatLanes)
	mu.Unlock()
	rec = do(t, s, http.MethodPost, "/v1/chat/completions", key, chatProbeBody)
	if rec.Code != http.StatusOK {
		t.Fatalf("第二次应 200，实际 %d", rec.Code)
	}
	mu.Lock()
	added := chatLanes[before:]
	mu.Unlock()
	if len(added) != 1 || added[0] != "5" {
		t.Fatalf("第二次应只用新车道打一次，实际 %v", added)
	}
}
