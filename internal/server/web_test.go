package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestConsoleAssetsServed 守住控制台的资源接线。
//
// 控制台已拆成「标记 + 样式 + ES module」：`go:embed` 少列一条、路由漏注册，
// 都会变成运行时 404（页面白屏但 go test 全绿）。这里逐个访问一遍。
func TestConsoleAssetsServed(t *testing.T) {
	s, key := newTestServer(t, nil)

	cases := []struct {
		path     string
		ctype    string
		contains string
		html     bool
	}{
		{"/", "text/html", "/js/app.js", true},
		{"/style.css", "text/css", "--bg", false},
		{"/js/theme-boot.js", "text/javascript", "data-theme", false},
		{"/js/app.js", "text/javascript", "from './i18n.js'", false},
		{"/js/i18n.js", "text/javascript", "I18N_EN", false},
		{"/js/format.js", "text/javascript", "fmtCompact", false},
		{"/js/state.js", "text/javascript", "acctPageSize", false},
		{"/js/render/accounts.js", "text/javascript", "renderAccounts", false},
		{"/js/render/logs.js", "text/javascript", "renderLogs", false},
		{"/js/render/keys.js", "text/javascript", "renderKeys", false},
		{"/js/render/settings.js", "text/javascript", "renderSettings", false},
	}
	for _, c := range cases {
		rec := do(t, s, http.MethodGet, c.path, key, "")
		if rec.Code != http.StatusOK {
			t.Errorf("GET %s = %d，期望 200", c.path, rec.Code)
			continue
		}
		if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, c.ctype) {
			t.Errorf("GET %s 的 Content-Type = %q，期望包含 %q", c.path, ct, c.ctype)
		}
		if !strings.Contains(rec.Body.String(), c.contains) {
			t.Errorf("GET %s 的内容里找不到 %q", c.path, c.contains)
		}
		// 页面不缓存（随二进制更新），静态资源用 ETag 重校验：
		// no-store 会让首屏 17 个模块每次都全量重下。
		cc := rec.Header().Get("Cache-Control")
		if c.html {
			if cc != "no-store" {
				t.Errorf("GET %s 的 Cache-Control = %q，期望 no-store", c.path, cc)
			}
		} else {
			if cc != "no-cache" {
				t.Errorf("GET %s 的 Cache-Control = %q，期望 no-cache", c.path, cc)
			}
			if rec.Header().Get("ETag") == "" {
				t.Errorf("GET %s 缺少 ETag（无法 304 重校验）", c.path)
			}
		}
		// 页面与脚本都承载管理会话，CSP 不能漏。
		if csp := rec.Header().Get("Content-Security-Policy"); !strings.Contains(csp, "default-src 'self'") {
			t.Errorf("GET %s 缺少严格 CSP，实际 %q", c.path, csp)
		}
	}
}

// TestConsoleAssetConditionalGET 验证 ETag 能命中 304（升级后自然失效）。
func TestConsoleAssetConditionalGET(t *testing.T) {
	s, key := newTestServer(t, nil)
	first := do(t, s, http.MethodGet, "/js/app.js", key, "")
	etag := first.Header().Get("ETag")
	if etag == "" {
		t.Fatal("首次响应应有 ETag")
	}

	req := httptest.NewRequest(http.MethodGet, "/js/app.js", nil)
	req.RemoteAddr = "127.0.0.1:34567"
	req.Header.Set("If-None-Match", etag)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotModified {
		t.Fatalf("带 If-None-Match 应回 304，实际 %d", rec.Code)
	}
	if rec.Body.Len() != 0 {
		t.Fatalf("304 不应带响应体，实际 %d 字节", rec.Body.Len())
	}

	// 换了内容（升级二进制）就必须重新下发：这里用错误的 ETag 模拟。
	req2 := httptest.NewRequest(http.MethodGet, "/js/app.js", nil)
	req2.RemoteAddr = "127.0.0.1:34567"
	req2.Header.Set("If-None-Match", `"stale"`)
	rec2 := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("ETag 不匹配应回 200，实际 %d", rec2.Code)
	}
}

// TestConsolePageHasNoInlineAssets 确认标记里不再内联样式与脚本。
//
// 内联回去会让 style.css / app.js 成为死文件，同时绕过资源路由的测试。
func TestConsolePageHasNoInlineAssets(t *testing.T) {
	page := consoleMarkup()
	if strings.Contains(page, "<style>") {
		t.Error("index.html 里又出现内联 <style>")
	}
	if strings.Contains(page, "<script>") {
		t.Error("index.html 里又出现内联 <script>")
	}
	if !strings.Contains(page, `href="/style.css"`) {
		t.Error("index.html 没引用 /style.css")
	}
	if !strings.Contains(page, `type="module" src="/js/app.js"`) {
		t.Error("index.html 没以 ES module 方式引用 /js/app.js")
	}
}

// TestConsoleAssetPathIsWhitelisted 确认资源路由不接受任意路径。
//
// 注意 `/js/../../etc/passwd` 会被 net/http 的 ServeMux 先规整成 301 跳转，
// 这同样是「不提供该文件」——断言的重点是「不含文件内容且不是 200」。
func TestConsoleAssetPathIsWhitelisted(t *testing.T) {
	s, key := newTestServer(t, nil)
	for _, p := range []string{
		"/js/../../etc/passwd",
		"/js/sub/app.js",
		"/js/render/../../app.js",
		"/web/index.html",
		"/neat.css",
	} {
		rec := do(t, s, http.MethodGet, p, key, "")
		if rec.Code == http.StatusOK {
			t.Errorf("GET %s 不应 200（应 404 或被 mux 规整成 301）", p)
		}
		if strings.Contains(rec.Body.String(), "root:x:") {
			t.Errorf("GET %s 泄露了文件内容", p)
		}
	}
}
