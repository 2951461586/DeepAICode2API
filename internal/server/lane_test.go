package server

import (
	"io"
	"net/http"
	"path/filepath"
	"sync"
	"testing"

	"deepaicode2api/internal/deepai"
	"deepaicode2api/internal/store"
)

// 426 → 借账号凭据打 /models 探到新车道 → 换道重试成功，并把结果落盘。
func TestLaneUnsupportedRefreshesAndRetries(t *testing.T) {
	var mu sync.Mutex
	var chatLanes []string
	// 上游只接受 8（编译期车道 7 的下一档），模拟“先抬车道、后发客户端”。
	u := newUpstream(t, func(w http.ResponseWriter, r *http.Request) {
		lane := r.Header.Get("X-DeepAI-Code-Theos")
		mu.Lock()
		chatLanes = append(chatLanes, lane)
		mu.Unlock()
		if lane != "8" {
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

	if got := s.CurrentLane(); got == "8" {
		t.Fatalf("初始车道不应是 8: %q", got)
	}
	rec := do(t, s, http.MethodPost, "/v1/chat/completions", key, chatProbeBody)
	if rec.Code != http.StatusOK {
		t.Fatalf("换道后应成功，实际 %d: %s", rec.Code, rec.Body.String())
	}
	if s.CurrentLane() != "8" {
		t.Fatalf("探测到的车道应生效，实际 %q", s.CurrentLane())
	}
	if got := s.Store.Settings().Lane; got != "8" {
		t.Fatalf("车道应落盘，实际 %q", got)
	}
	if got := s.Store.Settings().LaneBuild; got != deepai.LaneVersion {
		t.Fatalf("LaneBuild 应记录本次构建，实际 %q", got)
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
	if len(added) != 1 || added[0] != "8" {
		t.Fatalf("第二次应只用新车道打一次，实际 %v", added)
	}
}

// 状态文件里还留着上一版探测到的旧车道时，升级后应直接用编译期的新车道，
// 而不是等一次 426 才换道。
func TestNewPrefersNewerCompiledLane(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "state.json"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	st.UpdateSettings(func(s *store.Settings) { s.Lane = "6" })
	if err := st.Save(); err != nil {
		t.Fatalf("保存状态: %v", err)
	}

	s := New(st)
	if got := s.CurrentLane(); got != deepai.LaneVersion {
		t.Fatalf("车道 = %q，期望编译期默认值 %q", got, deepai.LaneVersion)
	}
	if got := s.Up.Lane(); got != deepai.LaneVersion {
		t.Fatalf("客户端车道 = %q，期望 %q", got, deepai.LaneVersion)
	}
}

// 探测值由当前构建验证过时，即使比编译期默认值旧也要信任：
// 上游可能只剩旧车道可用，继续发编译值只会一直 426。
func TestNewTrustsLowerLaneVerifiedByThisBuild(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "state.json"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	st.UpdateSettings(func(s *store.Settings) {
		s.Lane = "6"
		s.LaneBuild = deepai.LaneVersion
	})
	if err := st.Save(); err != nil {
		t.Fatalf("保存状态: %v", err)
	}

	s := New(st)
	if got := s.CurrentLane(); got != "6" {
		t.Fatalf("车道 = %q，期望本次构建验证过的 6", got)
	}
	if got := s.Up.Lane(); got != "6" {
		t.Fatalf("客户端车道 = %q，期望 6", got)
	}
}

// 探测值来自旧构建（LaneBuild 不匹配）时不能压过升级后的编译值。
func TestNewPrefersCompiledOverProbeFromOlderBuild(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "state.json"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	st.UpdateSettings(func(s *store.Settings) {
		s.Lane = "6"
		s.LaneBuild = "6" // 写这条状态时的编译期车道
	})
	if err := st.Save(); err != nil {
		t.Fatalf("保存状态: %v", err)
	}

	if got := New(st).CurrentLane(); got != deepai.LaneVersion {
		t.Fatalf("车道 = %q，期望升级后的编译值 %q", got, deepai.LaneVersion)
	}
}

// 探测到的车道比编译期默认值新时，仍以探测结果为准（上游先抬车道的情形）。
func TestNewKeepsNewerProbedLane(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "state.json"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	st.UpdateSettings(func(s *store.Settings) { s.Lane = "8" })
	if err := st.Save(); err != nil {
		t.Fatalf("保存状态: %v", err)
	}

	if got := New(st).CurrentLane(); got != "8" {
		t.Fatalf("车道 = %q，期望保留探测到的 8", got)
	}
}

// 手填的非数字车道不能把网关卡死：退回编译期默认值。
func TestNewerLaneFallsBackOnGarbage(t *testing.T) {
	if got := newerLane("  "); got != deepai.LaneVersion {
		t.Fatalf("空值 = %q，期望 %q", got, deepai.LaneVersion)
	}
	if got := newerLane("latest"); got != deepai.LaneVersion {
		t.Fatalf("非数字 = %q，期望 %q", got, deepai.LaneVersion)
	}
}
