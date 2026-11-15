package server

import (
	"net/http"
	"strings"

	"deepaicode2api/internal/server/web"
)

// 本文件只剩「路由白名单」：把 URL 映射成内嵌资源名。
//
// 资源本体、ETag、Content-Type、CSP 全部在 internal/server/web 包里；
// 这里保留在 server 包是因为白名单属于路由契约（哪些 URL 合法），
// 与资源内容无关。

// serveConsole 输出控制台单页。
func serveConsole(w http.ResponseWriter, r *http.Request) {
	web.ServeAsset(w, r, "index.html")
}

// serveLogin 输出管理口令登录页（/login）。
//
// 与控制台同一套静态资源与响应头；页面只做一件事：验一次口令后跳到 /。
func serveLogin(w http.ResponseWriter, r *http.Request) {
	web.ServeAsset(w, r, "login.html")
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
		name = "style.css"
	case p == "favicon.ico":
		// 浏览器自动请求的固定路径；标签页/书签图标（多尺寸 ICO，见 docs/assets）
		name = "favicon.ico"
	case p == "apple-touch-icon.png":
		// iOS 主屏图标只认 PNG（ICO 会被忽略），所以单独放一张 180×180
		name = "apple-touch-icon.png"
	case strings.HasPrefix(p, "js/render/"):
		// render/ 下只允许一层扁平文件名
		rest := strings.TrimPrefix(p, "js/render/")
		if rest == "" || strings.ContainsAny(rest, "/\\") || strings.Contains(rest, "..") {
			http.NotFound(w, r)
			return
		}
		name = "js/render/" + rest
	case strings.HasPrefix(p, "js/i18n/"):
		// i18n/ 与 render/ 同规则：只允许一层扁平文件名
		rest := strings.TrimPrefix(p, "js/i18n/")
		if rest == "" || strings.ContainsAny(rest, "/\\") || strings.Contains(rest, "..") {
			http.NotFound(w, r)
			return
		}
		name = "js/i18n/" + rest
	case strings.HasPrefix(p, "js/"):
		rest := strings.TrimPrefix(p, "js/")
		// 只允许扁平文件名：不能有子目录、反斜杠或 ..
		if rest == "" || strings.ContainsAny(rest, "/\\") || strings.Contains(rest, "..") {
			http.NotFound(w, r)
			return
		}
		name = "js/" + rest
	default:
		http.NotFound(w, r)
		return
	}
	web.ServeAsset(w, r, name)
}
