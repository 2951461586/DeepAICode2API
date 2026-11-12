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
	gate := make(chan struct{}) // 8 个调用真正重叠的闸门：都发起后再放行
	withUpdateFeed(t, func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		<-gate
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{"tag_name": "v0.2.0"}); err != nil {
			t.Errorf("写测试响应失败: %v", err)
		}
	})

	s, _ := newTestServer(t, nil)
	s.SetVersion("v0.1.0")

	const n = 8
	// 结果走 channel 而不是 results[i]：并发写同一个切片的不同下标虽然安全，
	// 但静态规则（go-shared-map-write-goroutine）看不出这是「各写各的」，
	// 每次扫描都要人工确认一遍。channel 让「谁产生、谁消费」写在类型上。
	results := make(chan versionInfo, n)
	var wg sync.WaitGroup
	wg.Add(n)
	for range n {
		go func() {
			defer wg.Done()
			results <- s.versionStatus(t.Context())
		}()
	}
	close(gate) // 放行：此时 8 个 goroutine 都已在单飞队列里
	wg.Wait()
	close(results)

	if got := hits.Load(); got != 1 {
		t.Fatalf("上游应只被查询一次，实际 %d 次", got)
	}
	seen := 0
	for got := range results {
		seen++
		if got.Current != "v0.1.0" || got.Latest != "v0.2.0" || !got.UpdateAvailable {
			t.Fatalf("第 %d 个结果不对：%+v", seen, got)
		}
	}
	if seen != n {
		t.Fatalf("结果条数 = %d，期望 %d", seen, n)
	}
}

// TestVersionStatusSurvivesCallerCancel 守住「第一个调用方断开」的缓存污染。
//
// 以前单飞的 leader 直接用调用方的 ctx：控制台刚打开就关掉标签页 → 在途的
// GitHub 查询被 abort → 失败结果写进缓存 6 小时 → 之后所有人打开控制台都是
// 「无法检查更新」，而 GitHub 其实完全可达。
func TestVersionStatusSurvivesCallerCancel(t *testing.T) {
	// arrived 在上游真的收到请求时关闭；release 由测试在 cancel 之后关闭。
	// 用 channel 而不是 sleep：没有随机等待，也不受机器快慢影响。
	arrived := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	hits := withUpdateFeed(t, func(w http.ResponseWriter, r *http.Request) {
		once.Do(func() { close(arrived) })
		<-release
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{"tag_name": "v9.9.9"}); err != nil {
			t.Errorf("写测试响应失败: %v", err)
		}
	})

	s, _ := newTestServer(t, nil)
	s.SetVersion("v0.1.0")

	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan versionInfo, 1)
	go func() { done <- s.versionStatus(ctx) }()

	select {
	case <-arrived: // leader 的查询已经在上游路上
	case <-time.After(5 * time.Second):
		t.Fatal("上游始终没收到请求")
	}
	cancel()       // 此刻调用方断开（浏览器关掉标签页）
	close(release) // 放上游返回
	leader := <-done

	// 关键断言：取消发生在查询途中，leader 仍应拿到结果，而不是把取消当失败缓存。
	if leader.Error != "" {
		t.Fatalf("调用方取消不应污染结果：%+v", leader)
	}
	got := s.versionStatus(t.Context())
	if got.Error != "" {
		t.Fatalf("缓存被污染：%+v", got)
	}
	if got.Latest != "v9.9.9" || !got.UpdateAvailable {
		t.Fatalf("缓存结果不对：%+v", got)
	}
	if n := hits.Load(); n != 1 {
		t.Fatalf("上游应只被查一次，实际 %d 次（取消可能触发了重查）", n)
	}
}
