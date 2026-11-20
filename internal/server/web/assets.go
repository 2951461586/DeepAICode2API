// Package web 是控制台静态资源包：把 HTML / CSS / ES module / 图标嵌进二进制，
// 并负责内容指纹（ETag）、Content-Type 与 CSP 等响应头。
//
// 这些资源原先是 internal/server 里的一个 embed.FS + 200 行服务代码，与路由和
// 协议实现混在一起。资源在编译期固定、无运行时依赖，因此独立成包后：
//   - internal/server 只需要 `web.ServeAsset(w, r, name)`；
//   - 资源路径的合法性（白名单）仍留在路由层，embed 不打开展示；
//   - 契约测试（console_*.go）可以直接对 web 包断言。
package web

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"net/http"
	"path"
	"sort"
	"strings"
	"sync"

	"deepaicode2api/internal/httpx"
)

// 控制台是「标记 + 样式 + 一组 ES module」的静态包，全部嵌进二进制：
// 无外部依赖、无构建步骤、离线可用。
//
//go:embed index.html login.html style.css favicon.ico apple-touch-icon.png js/*.js js/render/*.js js/i18n/*.js
var FS embed.FS

// HTML 是控制台页面标记（不含内联脚本与样式）。
//
// 保留成变量是为了让契约测试能直接断言标记内容（见 internal/server/console_test.go）。
var HTML = MustRead("index.html")

// LoginHTML 是 /login 页面标记（登录页有自己的 DOM，与 index.html 不同）。
var LoginHTML = MustRead("login.html")

// CSS 是控制台样式表（已从 index.html 抽出为独立资源）。
var CSS = MustRead("style.css")

// Read 读取一个内嵌资源。
//
// 资源名相对本包目录（"index.html" / "js/app.js"），与 //go:embed 的写法一致。
// 历史上还兼容过 "web/" 前缀（那时资源挂在 internal/server/web/ 下），
// 两套写法并存只会让调用点无从判断该写哪个，已清掉。
func Read(name string) ([]byte, error) {
	return FS.ReadFile(name)
}

// MustRead 读取一个内嵌资源，缺失即 panic（编译期资源，缺了就是构建错误）。
func MustRead(name string) []byte {
	b, err := Read(name)
	if err != nil {
		panic("控制台资源缺失: " + name + ": " + err.Error())
	}
	return b
}

// ScriptNames 返回全部内嵌脚本的资源名（已排序，可直接交给 Read）。
//
// 单独暴露的原因：契约测试要按「这个脚本属于哪个页面」分组校验 id 引用
// （login.js 的 #loginPw 在 login.html 里，不在 index.html 里）。
func ScriptNames() []string {
	var names []string
	for _, dir := range []string{"js", "js/render", "js/i18n"} {
		entries, err := FS.ReadDir(dir)
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

// Scripts 返回全部模块化脚本的拼接结果，供契约测试做「字符串存在性」断言。
//
// 前端已拆成多个 ES module：断言某个函数名/选择器存在时，必须扫全部脚本，
// 否则测试会在拆分后静默失效（这比测错更危险）。
func Scripts() string {
	var b strings.Builder
	for _, name := range ScriptNames() {
		if raw, err := Read(name); err == nil {
			b.Write(raw)
			b.WriteByte('\n')
		}
	}
	return b.String()
}

// assetType 给出内嵌资源的 Content-Type。
func assetType(name string) string {
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

// CSP 是控制台页面的内容安全策略。
//
// 控制台承载管理口令，是整套系统里最值得加 CSP 的页面。资源全部同源、
// 无外部引用，因此不需要放宽任何一条：
//   - script-src 'self'：内联脚本一律禁止（主题启动脚本也做成了外部文件）；
//   - style-src 'self'：inline style 属性已从标记里移除（统一改用具名工具类）；
//   - connect-src 'self'：控制台只调自己的 /admin/api/*；
//   - frame-ancestors 'none' + form-action 'self'：防点击劫持与外发。
const CSP = "default-src 'self'; script-src 'self'; style-src 'self'; " +
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

// ServeAsset 输出一个内嵌资源（页面 / 样式 / 脚本 / 图标）。
func ServeAsset(w http.ResponseWriter, r *http.Request, name string) {
	raw, err := Read(name)
	if err != nil {
		http.NotFound(w, nil)
		return
	}
	h := w.Header()
	h.Set("Content-Type", assetType(name))
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Referrer-Policy", "no-referrer")
	// 页面与脚本同源，CSP 对两者都生效（页面里没有内联脚本 / 样式）。
	h.Set("Content-Security-Policy", CSP)

	// 资源随二进制一起更新：HTML 每次都重新校验，JS/CSS 用 ETag 命中 304。
	// 旧实现一律 no-store，导致首屏 17 个模块每次全量重下。
	etag := assetETag(name, raw)
	h.Set("ETag", etag)
	if strings.HasSuffix(name, ".html") {
		h.Set("Cache-Control", "no-store")
	} else {
		h.Set("Cache-Control", "no-cache")
		if httpx.ETagMatches(r.Header.Get("If-None-Match"), etag) {
			w.WriteHeader(http.StatusNotModified)
			return
		}
	}
	_, _ = w.Write(raw)
}
