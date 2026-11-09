package server

import (
	"io"
	"net/http"
	"testing"
	"time"
)

// 槽位全满时先短暂排队，而不是立刻 429（docs/performance.md §14：
// 14 号 / 28 并发时只有 48~70% 成功，根因之一就是秒拒）。
func TestAllBusyQueuesInsteadOfRejecting(t *testing.T) {
	u := newUpstream(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, okChatResponse)
	})
	s, key := newTestServer(t, u)
	s.admissionWait = 2 * time.Second

	// 占满唯一账号的全部在途槽位。
	if !s.acquireAccount("acc-1") || !s.acquireAccount("acc-1") {
		t.Fatal("预占槽位失败")
	}
	go func() {
		time.Sleep(150 * time.Millisecond)
		s.releaseAccount("acc-1")
	}()

	begin := time.Now()
	rec := do(t, s, http.MethodPost, "/v1/chat/completions", key, chatProbeBody)
	elapsed := time.Since(begin)
	if rec.Code != http.StatusOK {
		t.Fatalf("排到空槽后应成功，实际 %d: %s", rec.Code, rec.Body.String())
	}
	if elapsed < 100*time.Millisecond {
		t.Fatalf("应在排队，不能“恰好”立刻成功（耗时 %v）", elapsed)
	}
}

// 排队有上限：一直没空槽就回 429（可重试背压），不能把调用方挂死。
func TestAllBusyRejectsAfterAdmissionWait(t *testing.T) {
	u := newUpstream(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, okChatResponse)
	})
	s, key := newTestServer(t, u)
	s.admissionWait = 120 * time.Millisecond
	if !s.acquireAccount("acc-1") || !s.acquireAccount("acc-1") {
		t.Fatal("预占槽位失败")
	}

	begin := time.Now()
	rec := do(t, s, http.MethodPost, "/v1/chat/completions", key, chatProbeBody)
	elapsed := time.Since(begin)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("应回 429，实际 %d: %s", rec.Code, rec.Body.String())
	}
	if elapsed > 2*time.Second {
		t.Fatalf("排队必须有上限，实际 %v", elapsed)
	}
}
