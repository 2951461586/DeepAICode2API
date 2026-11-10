package server

import (
	"embed"
	"net/http"
	"path"
	"strings"
)

// 控制台是一个「标记 + 样式 + 一组 ES module」的静态包，全部嵌进二进制：
// 无外部依赖、无构建步骤、离线可用。
//
//go:embed web/index.html web/style.css web/js/*.js web/js/render/*.js
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
	// 顶层（app / i18n / dom …）与 render/ 子目录都要扫：漏一个目录，
	// 契约测试就会对那部分代码「看起来通过」。
	for _, dir := range []string{"web/js", "web/js/render"} {
		entries, err := consoleAssets.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".js") {
				continue
			}
			if raw, err := consoleAssets.ReadFile(dir + "/" + e.Name()); err == nil {
				b.Write(raw)
				b.WriteByte('\n')
			}
		}
	}
	return b.String()
}

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
	default:
		return "application/octet-stream"
	}
}

// serveConsole 输出控制台单页。
//
// 响应头刻意收紧：禁止缓存、禁止 MIME 嗅探、不发送 Referer。
// 资源与页面同源、无外部引用，因此不需要放宽 CSP。
func serveConsole(w http.ResponseWriter, _ *http.Request) {
	serveConsoleAsset(w, "web/index.html")
}

// serveConsoleAsset 输出一个内嵌控制台资源（页面 / 样式 / 脚本）。
func serveConsoleAsset(w http.ResponseWriter, name string) {
	raw, err := consoleAssets.ReadFile(name)
	if err != nil {
		http.NotFound(w, nil)
		return
	}
	h := w.Header()
	h.Set("Content-Type", consoleAssetType(name))
	// 资源随二进制一起更新：一律不缓存，避免升级后仍用旧 JS/CSS。
	h.Set("Cache-Control", "no-store")
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Referrer-Policy", "no-referrer")
	_, _ = w.Write(raw)
}

// handleConsoleAsset 响应 /style.css 与 /js/<name>.js。
//
// 用白名单而不是拼路径：控制台只有这两个入口，把「允许的名字」写成显式规则，
// 就不需要再讨论 embed FS 上的路径穿越。
func (s *Server) handleConsoleAsset(w http.ResponseWriter, r *http.Request) {
	p := strings.TrimPrefix(r.URL.Path, "/")
	var name string
	switch {
	case p == "style.css":
		name = "web/style.css"
	case strings.HasPrefix(p, "js/render/"):
		// render/ 下只允许一层扁平文件名
		rest := strings.TrimPrefix(p, "js/render/")
		if rest == "" || strings.ContainsAny(rest, "/\\") || strings.Contains(rest, "..") {
			http.NotFound(w, r)
			return
		}
		name = "web/js/render/" + rest
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
	serveConsoleAsset(w, name)
}
