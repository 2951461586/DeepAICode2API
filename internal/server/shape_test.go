package server

import (
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"deepaicode2api/internal/store"
)

// TestShapeCacheTTLAndLimit 覆盖形状记忆的过期与条目上限。
func TestShapeCacheTTLAndLimit(t *testing.T) {
	s := &Server{}
	if s.shapeNeeded("k1") {
		t.Fatal("未标记时不应命中")
	}
	s.markShapeNeeded("k1")
	if !s.shapeNeeded("k1") {
		t.Fatal("标记后应命中")
	}
	// 过期：把到期时间拨到过去。
	s.shapeMu.Lock()
	s.shapeNeed[shapeCacheKey("k1")] = pastTime()
	s.shapeMu.Unlock()
	if s.shapeNeeded("k1") {
		t.Fatal("过期条目不应命中")
	}

	// 超过上限后仍可用（内部会清理过期项 / 重建）。
	for i := 0; i < presetShapeMax+10; i++ {
		s.markShapeNeeded("key-" + strings.Repeat("x", i%7) + string(rune('a'+i%26)) + itoa64(int64(i)))
	}
	s.markShapeNeeded("last")
	if !s.shapeNeeded("last") {
		t.Fatal("超过上限后最新条目仍应命中")
	}
	if shapeCacheKey("") != "" {
		t.Fatal("空 key 不应产生哈希")
	}
}

func pastTime() (t time.Time) { return time.Now().Add(-time.Minute) }

// TestPresetDesktopShapeSkipsRejectedRoundTrip 固化 P1：
// 第一次请求走「400 → 补形状重试」，之后的请求在第一次发送前就补 system，
// 不再多打一次注定失败的上游请求。
func TestPresetDesktopShapeSkipsRejectedRoundTrip(t *testing.T) {
	var mu sync.Mutex
	var attemptsWithoutSystem []bool
	var calls atomic.Int64
	// handler 里要读「刚记录的请求体」：newUpstream 会先读 r.Body 再交给 handler，
	// 所以这里从 u.last 取，而不是再读一次 r.Body（那样拿到的是空 body）。
	var u *upstream
	u = newUpstream(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/theos") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"enabled":true,"lane_version":5}`)
			return
		}
		payload := u.last(t).Payload
		hasSystem := false
		if msgs, ok := payload["messages"].([]any); ok {
			for _, m := range msgs {
				if mm, ok := m.(map[string]any); ok && mm["role"] == "system" {
					hasSystem = true
				}
			}
		}
		calls.Add(1)
		mu.Lock()
		attemptsWithoutSystem = append(attemptsWithoutSystem, !hasSystem)
		mu.Unlock()
		if !hasSystem {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, `{"error":{"message":"This request cannot be accepted.",`+
				`"type":"invalid_request_error","code":"invalid_request_error"}}`)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"choices":[{"message":{"role":"assistant","content":"ok"}}]}`)
	})
	s, key := newTestServer(t, u)
	s.Store.UpdateSettings(func(st *store.Settings) { st.PresetDesktopShape = true })

	body := `{"model":"m","stream":false,"messages":[{"role":"user","content":"hi"}]}`

	// 第一次：学到「需要形状」，共 2 次上游调用（1 次被拒 + 1 次重试）。
	if rec := do(t, s, http.MethodPost, "/v1/chat/completions", key, body); rec.Code != http.StatusOK {
		t.Fatalf("第一次应 200，实际 %d: %s", rec.Code, rec.Body.String())
	}
	if calls.Load() != 2 {
		t.Fatalf("第一次应有 2 次上游调用，实际 %d", calls.Load())
	}

	// 第二次：预置 system，只有 1 次上游调用，且带 system。
	before := calls.Load()
	if rec := do(t, s, http.MethodPost, "/v1/chat/completions", key, body); rec.Code != http.StatusOK {
		t.Fatalf("第二次应 200，实际 %d: %s", rec.Code, rec.Body.String())
	}
	if calls.Load() != before+1 {
		t.Fatalf("第二次应只打 1 次上游，实际增加 %d", calls.Load()-before)
	}
	mu.Lock()
	last := attemptsWithoutSystem[len(attemptsWithoutSystem)-1]
	mu.Unlock()
	if last {
		t.Fatal("第二次请求应在首次发送就带 system")
	}
}

// TestPresetDesktopShapeDisabledByDefault：开关关闭时不预置，仍走学习路径。
func TestPresetDesktopShapeDisabledByDefault(t *testing.T) {
	var calls atomic.Int64
	u := newUpstream(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/theos") {
			_, _ = io.WriteString(w, `{"enabled":true,"lane_version":5}`)
			return
		}
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"choices":[{"message":{"role":"assistant","content":"ok"}}]}`)
	})
	s, key := newTestServer(t, u)

	body := `{"model":"m","stream":false,"messages":[{"role":"user","content":"hi"}]}`
	for i := 0; i < 2; i++ {
		if rec := do(t, s, http.MethodPost, "/v1/chat/completions", key, body); rec.Code != http.StatusOK {
			t.Fatalf("应 200，实际 %d", rec.Code)
		}
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("开关关闭时每个请求只打 1 次上游，实际 %d", got)
	}
}
