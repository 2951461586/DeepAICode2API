package server

import (
	"net/http"
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
	}{
		{"/", "text/html", "/js/app.js"},
		{"/style.css", "text/css", "--bg"},
		{"/js/app.js", "text/javascript", "from './i18n.js'"},
		{"/js/i18n.js", "text/javascript", "I18N_EN"},
		{"/js/format.js", "text/javascript", "fmtCompact"},
		{"/js/state.js", "text/javascript", "acctPageSize"},
		{"/js/render/accounts.js", "text/javascript", "renderAccounts"},
		{"/js/render/logs.js", "text/javascript", "renderLogs"},
		{"/js/render/keys.js", "text/javascript", "renderKeys"},
		{"/js/render/settings.js", "text/javascript", "renderSettings"},
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
		if cc := rec.Header().Get("Cache-Control"); cc != "no-store" {
			t.Errorf("GET %s 的 Cache-Control = %q，期望 no-store", c.path, cc)
		}
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
