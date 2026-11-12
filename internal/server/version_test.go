package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestParseSemver 覆盖解析边界：v 前缀、预发布后缀、非规范值。
func TestParseSemver(t *testing.T) {
	cases := []struct {
		in   string
		want [3]int
		ok   bool
	}{
		{"v0.1.0", [3]int{0, 1, 0}, true},
		{"1.2.3", [3]int{1, 2, 3}, true},
		{"v1.2", [3]int{1, 2, 0}, true},
		{"v1.2.3-rc1", [3]int{1, 2, 3}, true},
		{"v1.2.3+build.7", [3]int{1, 2, 3}, true},
		{"dev", [3]int{}, false},
		{"docker", [3]int{}, false},
		// git describe 形态（v0.1.0-3-gabc123）按基础 tag 解析：
		// 3 个提交之后的构建仍然属于 v0.1.0，拿它比对 release 是合理的。
		{"v0.1.0-3-gabc123", [3]int{0, 1, 0}, true},
		{"v1.2.3.4", [3]int{}, false},
		{"", [3]int{}, false},
	}
	for _, c := range cases {
		got, ok := parseSemver(c.in)
		if ok != c.ok || (ok && got != c.want) {
			t.Errorf("parseSemver(%q) = %v,%v，期望 %v,%v", c.in, got, ok, c.want, c.ok)
		}
	}
}

// TestVersionNewer 守住「不误报升级」：解析不出数字时一律不回 true。
func TestVersionNewer(t *testing.T) {
	cases := []struct {
		latest, current string
		want            bool
	}{
		{"v0.2.0", "v0.1.0", true},
		{"v0.1.1", "v0.1.0", true},
		{"v0.1.0", "v0.1.0", false},
		{"v0.1.0", "v0.2.0", false},
		{"v1.0.0", "dev", false},
		{"dev", "v0.1.0", false},
		// 同日构建的 rc 后缀不影响判定。
		{"v0.2.0-rc1", "v0.1.0", true},
	}
	for _, c := range cases {
		if got := versionNewer(c.latest, c.current); got != c.want {
			t.Errorf("versionNewer(%q,%q) = %v，期望 %v", c.latest, c.current, got, c.want)
		}
	}
}

// withUpdateFeed 把更新源指向一个测试服务器，返回请求计数器。
func withUpdateFeed(t *testing.T, handler http.HandlerFunc) *atomic.Int64 {
	t.Helper()
	var hits atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		handler(w, r)
	}))
	t.Cleanup(srv.Close)

	old := updateFeedURL
	updateFeedURL = srv.URL
	t.Cleanup(func() { updateFeedURL = old })
	return &hits
}

// TestAdminVersionEndpoint 验证「有新版本」的完整链路与缓存。
func TestAdminVersionEndpoint(t *testing.T) {
	hits := withUpdateFeed(t, func(w http.ResponseWriter, r *http.Request) {
		if ua := r.Header.Get("User-Agent"); ua == "" {
			t.Error("请求应带 User-Agent（GitHub API 强制要求）")
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"tag_name":     "v0.2.0",
			"html_url":     "https://github.com/2951461586/DeepAICode2API/releases/tag/v0.2.0",
			"published_at": "2026-11-12T00:00:00Z",
		})
	})

	s, _ := newTestServer(t, nil)
	s.SetVersion("v0.1.0")

	rec := do(t, s, http.MethodGet, "/admin/api/version", "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("状态码 = %d：%s", rec.Code, rec.Body.String())
	}
	out := decodeJSON[map[string]any](t, rec.Body.Bytes())
	if out["current"] != "v0.1.0" {
		t.Fatalf("current = %v", out["current"])
	}
	if out["update_available"] != true {
		t.Fatalf("应判定有新版本：%v", out)
	}
	if out["latest"] != "v0.2.0" {
		t.Fatalf("latest = %v", out["latest"])
	}
	if out["release_url"] == "" || out["release_url"] == nil {
		t.Fatalf("应带 release_url：%v", out)
	}

	// 第二次必须命中缓存：GitHub 未认证请求有速率限制。
	_ = do(t, s, http.MethodGet, "/admin/api/version", "", "")
	if n := hits.Load(); n != 1 {
		t.Fatalf("应只查一次上游（缓存 6h），实际 %d 次", n)
	}
}

// TestAdminVersionDevBuild 验证开发构建不做比对（否则永远提示「有新版本」）。
func TestAdminVersionDevBuild(t *testing.T) {
	hits := withUpdateFeed(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("dev 构建不应请求更新源")
	})
	s, _ := newTestServer(t, nil) // 未 SetVersion → dev
	rec := do(t, s, http.MethodGet, "/admin/api/version", "", "")
	out := decodeJSON[map[string]any](t, rec.Body.Bytes())
	if out["update_available"] != false {
		t.Fatalf("dev 构建不应提示更新：%v", out)
	}
	if out["error"] == nil || out["error"] == "" {
		t.Fatalf("dev 构建应说明原因：%v", out)
	}
	if hits.Load() != 0 {
		t.Fatalf("dev 构建不应打上游，实际 %d 次", hits.Load())
	}
}

// TestAdminVersionUpstreamFailure 验证上游失败时仍返回 200 + 当前版本。
func TestAdminVersionUpstreamFailure(t *testing.T) {
	withUpdateFeed(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})
	s, _ := newTestServer(t, nil)
	s.SetVersion("v0.1.0")

	rec := do(t, s, http.MethodGet, "/admin/api/version", "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("上游失败不应让接口失败，实际 %d：%s", rec.Code, rec.Body.String())
	}
	out := decodeJSON[map[string]any](t, rec.Body.Bytes())
	if out["current"] != "v0.1.0" || out["update_available"] != false {
		t.Fatalf("应返回当前版本且不提示更新：%v", out)
	}
	if out["error"] == nil {
		t.Fatalf("应带 error 字段说明查询失败：%v", out)
	}

	// 方法不对时 405。
	if rec := do(t, s, http.MethodPost, "/admin/api/version", "", ""); rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST 应回 405，实际 %d", rec.Code)
	}
}

// TestHealthIncludesVersion 验证控制台能从公开端点拿到版本号。
func TestHealthIncludesVersion(t *testing.T) {
	s, _ := newTestServer(t, nil)
	s.SetVersion("v0.1.0")
	rec := do(t, s, http.MethodGet, "/health", "", "")
	out := decodeJSON[map[string]any](t, rec.Body.Bytes())
	if out["version"] != "v0.1.0" {
		t.Fatalf("health.version = %v", out["version"])
	}
}

// TestVersionStatusSingleFlight 覆盖「缓存冷时并发查询合并」。
//
// 控制台可能在多个标签页同时刷新；没有合并时每个请求都会打一次 GitHub，
// 而未认证的 GitHub API 有速率限制。这里只允许上游被命中一次。
func TestVersionStatusSingleFlight(t *testing.T) {
	var hits atomic.Int64
	withUpdateFeed(t, func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		time.Sleep(40 * time.Millisecond) // 拉长窗口，让并发请求真正重叠
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"tag_name": "v0.2.0"})
	})

	s, _ := newTestServer(t, nil)
	s.SetVersion("v0.1.0")

	const n = 8
	var wg sync.WaitGroup
	wg.Add(n)
	results := make([]versionInfo, n)
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			results[i] = s.versionStatus(context.Background())
		}(i)
	}
	wg.Wait()

	if got := hits.Load(); got != 1 {
		t.Fatalf("上游应只被查询一次，实际 %d 次", got)
	}
	for i, got := range results {
		if got.Current != "v0.1.0" || got.Latest != "v0.2.0" || !got.UpdateAvailable {
			t.Fatalf("第 %d 个结果不对：%+v", i, got)
		}
	}
}
