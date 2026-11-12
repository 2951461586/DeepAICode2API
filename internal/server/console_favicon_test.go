package server

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestConsoleFaviconServed 守住「标签页图标」这条浏览器自动请求的固定路径。
//
// 以前没有 favicon：每次打开控制台 / 登录页，浏览器都会朝 /favicon.ico 打一发
// 404（还会进访问日志）。图标复用 DeepAI Code 桌面端的应用图标（见 docs/assets），
// 所以这里不只验 200，还验内容确实是 ICO / PNG——免得某次改动把空文件塞进去。
func TestConsoleFaviconServed(t *testing.T) {
	s, key := newTestServer(t, nil)

	cases := []struct {
		path  string
		ctype string
		magic []byte
	}{
		// ICO 头：reserved=0, type=1, count>0
		{"/favicon.ico", "image/x-icon", []byte{0x00, 0x00, 0x01, 0x00}},
		{"/apple-touch-icon.png", "image/png", []byte("\x89PNG\r\n\x1a\n")},
	}
	for _, c := range cases {
		rec := do(t, s, http.MethodGet, c.path, key, "")
		if rec.Code != http.StatusOK {
			t.Errorf("GET %s = %d，期望 200", c.path, rec.Code)
			continue
		}
		if ct := rec.Header().Get("Content-Type"); ct != c.ctype {
			t.Errorf("GET %s 的 Content-Type = %q，期望 %q", c.path, ct, c.ctype)
		}
		if body := rec.Body.Bytes(); !bytes.HasPrefix(body, c.magic) {
			t.Errorf("GET %s 不是合法的 %s（前 8 字节 %x）", c.path, c.ctype, body[:min(8, len(body))])
		}
		// 静态资源走 ETag 重校验：跟随二进制更新，但不能让浏览器每次全量重下。
		if rec.Header().Get("ETag") == "" {
			t.Errorf("GET %s 缺少 ETag", c.path)
		}
		if cc := rec.Header().Get("Cache-Control"); cc != "no-cache" {
			t.Errorf("GET %s 的 Cache-Control = %q，期望 no-cache", c.path, cc)
		}
		if xcto := rec.Header().Get("X-Content-Type-Options"); xcto != "nosniff" {
			t.Errorf("GET %s 缺少 nosniff", c.path)
		}

		// 条件请求要能命中 304（图标随版本更新，但不必每次重下）。
		req := httptest.NewRequest(http.MethodGet, c.path, nil)
		req.RemoteAddr = "127.0.0.1:34567"
		req.Header.Set("If-None-Match", rec.Header().Get("ETag"))
		rec2 := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec2, req)
		if rec2.Code != http.StatusNotModified {
			t.Errorf("GET %s 带 If-None-Match 应回 304，实际 %d", c.path, rec2.Code)
		}
	}
}

// TestConsolePagesReferenceFavicon 确认两个页面都声明了图标。
//
// 只把文件嵌进二进制不够：页面不写 <link rel="icon">，浏览器仍会去猜默认路径，
// 移动端「添加到主屏」也拿不到图标。
func TestConsolePagesReferenceFavicon(t *testing.T) {
	for _, name := range []string{"web/index.html", "web/login.html"} {
		page := string(mustConsoleAsset(name))
		for _, want := range []string{
			`<link rel="icon" href="/favicon.ico" sizes="any">`,
			`<link rel="apple-touch-icon" href="/apple-touch-icon.png">`,
		} {
			if !strings.Contains(page, want) {
				t.Errorf("%s 缺少图标声明：%s", name, want)
			}
		}
	}
}

// TestConsoleFaviconMatchesDocsAsset 守住「图标只有一份真源」。
//
// 控制台里的 favicon.ico 是 docs/assets/logo.ico 的副本（embed 只能读包内文件）。
// 换 Logo 时忘了同步，就会出现「README 是新图、标签页还是旧图」——肉眼几乎
// 发现不了，但确实是两套品牌资产。这里直接按字节比对。
func TestConsoleFaviconMatchesDocsAsset(t *testing.T) {
	want, err := os.ReadFile(filepath.Join("..", "..", "docs", "assets", "logo.ico"))
	if err != nil {
		t.Fatalf("读取 docs/assets/logo.ico 失败: %v", err)
	}
	got := mustConsoleAsset("web/favicon.ico")
	if !bytes.Equal(got, want) {
		t.Fatalf("web/favicon.ico 与 docs/assets/logo.ico 不一致（%d vs %d 字节）："+
			"换图标后要同时更新两份，见 docs/changes.md §58", len(got), len(want))
	}
}

// TestStaticAssetsSkipAccessLog 确认图标不会被写进访问日志。
//
// 浏览器每次打开页面都会自动请求 /favicon.ico（以及 Apple 的 touch icon），
// 它们和 /style.css、/js/* 一样是纯静态资源：进日志只会把 access 行刷成
// 「一半是图标」的噪声，还会让 /metrics 的 2xx 计数虚高。
func TestStaticAssetsSkipAccessLog(t *testing.T) {
	for _, p := range []string{"/favicon.ico", "/apple-touch-icon.png", "/style.css", "/js/app.js", "/ping"} {
		if !isStaticAsset(p) {
			t.Errorf("%s 应视为静态资源（不写访问日志）", p)
		}
	}
	for _, p := range []string{"/", "/login", "/admin/api/logs", "/v1/models"} {
		if isStaticAsset(p) {
			t.Errorf("%s 不是静态资源，不应被跳过", p)
		}
	}
}
