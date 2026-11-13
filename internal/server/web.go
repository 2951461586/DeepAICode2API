package server

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"net/http"
	"path"
	"sort"
	"strings"
	"sync"
)

// 控制台是一个「标记 + 样式 + 一组 ES module」的静态包，全部嵌进二进制：
// 无外部依赖、无构建步骤、离线可用。
//
//go:embed web/index.html web/login.html web/style.css web/favicon.ico web/apple-touch-icon.png web/js/*.js web/js/render/*.js web/js/i18n/*.js
var consoleAssets embed.FS

// consoleHTML 是控制台的页面标记（不含内联脚本与样式）。
//
// 保留成变量是为了让契约测试能直接断言标记内容（见 console_test.go）。
var consoleHTML = mustConsoleAsset("web/index.html")

// consoleCSS 是控制台样式表（已从 index.html 抽出为独立资源）。
var consoleCSS = mustConsoleAsset("web/style.css")

// consoleJS 返回全部模块化脚本的拼接结果，供契约测试做「字符串存在性」断言。
//
// 前端已拆成多个 ES module：断言某个函数名/选择器存在时，必须扫全部脚本，
// 否则测试会在拆分后静默失效（这比测错更危险）。
func consoleJS() string {
	var b strings.Builder
	for _, name := range consoleScriptNames() {
		if raw, err := consoleAssets.ReadFile(name); err == nil {
			b.Write(raw)
			b.WriteByte('\n')
		}
	}
	return b.String()
}

// consoleScriptNames 返回全部内嵌脚本的资源名（已排序）。
//
// 单独暴露的原因：契约测试要按「这个脚本属于哪个页面」分组校验 id 引用
// （login.js 的 #loginPw 在 login.html 里，不在 index.html 里）。
func consoleScriptNames() []string {
	var names []string
	for _, dir := range []string{"web/js", "web/js/render", "web/js/i18n"} {
		entries, err := consoleAssets.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".js") {
				continue
			}
			names = append(names, dir+"/"+e.Name())
		}
	}
	sort.Strings(names)
	return names
}

// consoleLoginHTML 是 /login 页面标记（登录页有自己的 DOM，与 index.html 不同）。
var consoleLoginHTML = mustConsoleAsset("web/login.html")

func mustConsoleAsset(name string) []byte {
	b, err := consoleAssets.ReadFile(name)
	if err != nil {
		panic("控制台资源缺失: " + name + ": " + err.Error())
	}
	return b
}

// consoleAssetType 给出内嵌资源的 Content-Type。
func consoleAssetType(name string) string {
	switch path.Ext(name) {
	case ".html":
		return "text/html; charset=utf-8"
	case ".css":
		return "text/css; charset=utf-8"
	case ".js":
		return "text/javascript; charset=utf-8"
	case ".ico":
		// 官方类型是 image/vnd.microsoft.icon（RFC 7903），但 image/x-icon 是
		// 实际世界里最常见的写法、所有浏览器都认，这里跟 nginx 的默认保持一致。
		return "image/x-icon"
	case ".png":
		return "image/png"
	default:
		return "application/octet-stream"
	}
}

// serveConsole 输出控制台单页。
//
// 响应头刻意收紧：禁止缓存、禁止 MIME 嗅探、不发送 Referer。
// 资源与页面同源、无外部引用，因此不需要放宽 CSP。
func serveConsole(w http.ResponseWriter, r *http.Request) {
	serveConsoleAsset(w, r, "web/index.html")
}

// serveLogin 输出管理口令登录页（/login）。
//
// 与控制台同一套静态资源与响应头；页面只做一件事：验一次口令后跳到 /。
func serveLogin(w http.ResponseWriter, r *http.Request) {
	serveConsoleAsset(w, r, "web/login.html")
}

// consoleCSP 是控制台页面的内容安全策略。
//
// 控制台承载管理口令，是整套系统里最值得加 CSP 的页面。资源全部同源、
// 无外部引用，因此不需要放宽任何一条：
//   - script-src 'self'：内联脚本一律禁止（主题启动脚本也做成了外部文件）；
//   - style-src 'self'：inline style 属性已从标记里移除（统一改用具名工具类）；
//   - connect-src 'self'：控制台只调自己的 /admin/api/*；
//   - frame-ancestors 'none' + form-action 'self'：防点击劫持与外发。
const consoleCSP = "default-src 'self'; script-src 'self'; style-src 'self'; " +
	"img-src 'self' data:; connect-src 'self'; font-src 'self'; " +
	"base-uri 'none'; form-action 'self'; frame-ancestors 'none'; object-src 'none'"

// assetETags 缓存内嵌资源的内容指纹（资源在编译期固定，算一次即可）。
var assetETags sync.Map // name -> string

// assetETag 返回资源的内容 ETag（形如 "abc…"）。
func assetETag(name string, raw []byte) string {
	if v, ok := assetETags.Load(name); ok {
		return v.(string)
	}
	sum := sha256.Sum256(raw)
	etag := `"` + hex.EncodeToString(sum[:16]) + `"`
	assetETags.Store(name, etag)
	return etag
}

// serveConsoleAsset 输出一个内嵌控制台资源（页面 / 样式 / 脚本）。
func serveConsoleAsset(w http.ResponseWriter, r *http.Request, name string) {
	raw, err := consoleAssets.ReadFile(name)
	if err != nil {
		http.NotFound(w, nil)
		return
	}
	h := w.Header()
	h.Set("Content-Type", consoleAssetType(name))
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Referrer-Policy", "no-referrer")
	// 页面与脚本同源，CSP 对两者都生效（页面里没有内联脚本 / 样式）。
	h.Set("Content-Security-Policy", consoleCSP)

	// 资源随二进制一起更新：HTML 每次都重新校验，JS/CSS 用 ETag 命中 304。
	// 旧实现一律 no-store，导致首屏 17 个模块每次全量重下。
	etag := assetETag(name, raw)
	h.Set("ETag", etag)
	if strings.HasSuffix(name, ".html") {
		h.Set("Cache-Control", "no-store")
	} else {
		h.Set("Cache-Control", "no-cache")
		if etagMatches(r.Header.Get("If-None-Match"), etag) {
			w.WriteHeader(http.StatusNotModified)
			return
		}
	}
	_, _ = w.Write(raw)
}

// handleConsoleAsset 响应 /style.css、/favicon.ico、/apple-touch-icon.png 与 /js/<name>.js。
//
// 用白名单而不是拼路径：控制台只有这两个入口，把「允许的名字」写成显式规则，
// 就不需要再讨论 embed FS 上的路径穿越。
func (s *Server) handleConsoleAsset(w http.ResponseWriter, r *http.Request) {
	p := strings.TrimPrefix(r.URL.Path, "/")
	var name string
	switch {
	case p == "style.css":
		name = "web/style.css"
	case p == "favicon.ico":
		// 浏览器自动请求的固定路径；标签页/书签图标（多尺寸 ICO，见 docs/assets）
		name = "web/favicon.ico"
	case p == "apple-touch-icon.png":
		// iOS 主屏图标只认 PNG（ICO 会被忽略），所以单独放一张 180×180
		name = "web/apple-touch-icon.png"
	case strings.HasPrefix(p, "js/render/"):
		// render/ 下只允许一层扁平文件名
		rest := strings.TrimPrefix(p, "js/render/")
		if rest == "" || strings.ContainsAny(rest, "/\\") || strings.Contains(rest, "..") {
			http.NotFound(w, r)
			return
		}
		name = "web/js/render/" + rest
	case strings.HasPrefix(p, "js/i18n/"):
		// i18n/ 与 render/ 同规则：只允许一层扁平文件名
		rest := strings.TrimPrefix(p, "js/i18n/")
		if rest == "" || strings.ContainsAny(rest, "/\\") || strings.Contains(rest, "..") {
			http.NotFound(w, r)
			return
		}
		name = "web/js/i18n/" + rest
	case strings.HasPrefix(p, "js/"):
		rest := strings.TrimPrefix(p, "js/")
		// 只允许扁平文件名：不能有子目录、反斜杠或 ..
		if rest == "" || strings.ContainsAny(rest, "/\\") || strings.Contains(rest, "..") {
			http.NotFound(w, r)
			return
		}
		name = "web/js/" + rest
	default:
		http.NotFound(w, r)
		return
	}
	serveConsoleAsset(w, r, name)
}
