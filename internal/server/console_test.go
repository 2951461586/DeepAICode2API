package server

import (
	"regexp"
	"strings"
	"testing"
)

// 控制台是内嵌单页，改动容易静默回归（例如前后端字段名不一致）。
// 这里守住几条关键约定：主题、分页、以及模型字段的 camelCase。

// consoleSource 返回「标记 + 全部 ES module 脚本 + 样式」的拼接。
//
// 前端已拆成多个 module：只扫 index.html 会让「某函数名 / 选择器存在」
// 这类断言静默失效（测试全绿但检查早已落空）。
func consoleSource() string {
	return consoleMarkup() + "\n" + consoleStyle() + "\n" + consoleJS()
}

// consoleMarkup 只返回页面标记（id 定义、标签页结构这类纯标记断言）。
func consoleMarkup() string { return string(consoleHTML) }

// consoleStyle 返回抽出的样式表。
func consoleStyle() string { return string(consoleCSS) }

func TestConsoleHTMLContracts(t *testing.T) {
	html := consoleSource()
	if html == "" {
		t.Fatal("控制台资源为空")
	}

	must := map[string]string{
		"主题按钮":    `id="theme"`,
		"浅色变量":    `:root[data-theme="light"]`,
		"分页容器":    `id="acctPager"`,
		"分页信息":    `id="acctInfo"`,
		"每页选择":    `id="acctSize"`,
		"上一页":     `id="acctPrev"`,
		"下一页":     `id="acctNext"`,
		"分页渲染":    `updateAcctPager`,
		"分页状态":    `acctPageSize`,
		"主题应用":    `applyTheme`,
		"主题持久化":   `deepaicode2api_theme`,
		"分页大小持久化": `deepaicode2api_acct_page_size`,
	}
	for name, needle := range must {
		if !strings.Contains(html, needle) {
			t.Errorf("控制台缺少%s：%s", name, needle)
		}
	}

	// 模型目录走 camelCase（deepai.Model 的 json tag），页面必须按 camelCase 取值。
	for _, field := range []string{"m.theosPerRequest", "m.contextWindow", "m.maxTokens", "m.input", "m.premier"} {
		if !strings.Contains(html, field) {
			t.Errorf("模型目录未按 json tag 取值：%s", field)
		}
	}
	for _, wrong := range []string{"m.TheosPerRequest", "m.ContextWindow", "m.MaxTokens", "m.Input"} {
		if strings.Contains(html, wrong) {
			t.Errorf("模型目录仍在使用 Go 字段名（会渲染成空）：%s", wrong)
		}
	}
}

// 分页默认 20，且只在 20/50/100 里取值。
func TestConsoleDefaultPageSize(t *testing.T) {
	html := consoleSource()
	// 分页状态已搬到 js/state.js：默认值在 ui 对象里。
	if !strings.Contains(html, "acctPageSize: 20") {
		t.Error("分页默认值应为 20")
	}
	// 只看 acctSize 这个 select 内部的选项（页面别处也有 value="200" 的控件）。
	start := strings.Index(html, `id="acctSize"`)
	if start < 0 {
		t.Fatal("找不到 acctSize 选择框")
	}
	end := strings.Index(html[start:], "</select>")
	if end < 0 {
		t.Fatal("acctSize 选择框未闭合")
	}
	block := html[start : start+end]
	for _, v := range []string{`value="20"`, `value="50"`, `value="100"`} {
		if !strings.Contains(block, v) {
			t.Errorf("分页选择框缺少选项 %s", v)
		}
	}
	if strings.Contains(block, `value="200"`) {
		t.Error("分页选择框不应有 200 选项")
	}
}

// 脚本引用的每个 DOM id 都必须在标记里存在。
//
// 这是最容易静默回归的一类改动：改标记时漏一个 id，控制台不报错，
// 只是某个功能永远不生效（按钮点了没反应、渲染函数写进 null）。
func TestConsoleScriptIDsExist(t *testing.T) {
	markup := consoleMarkup()
	defined := map[string]bool{}
	for _, m := range regexp.MustCompile(`id="([A-Za-z0-9_-]+)"`).FindAllStringSubmatch(markup, -1) {
		defined[m[1]] = true
	}

	script := consoleJS()
	if script == "" {
		t.Fatal("没有嵌入任何脚本（consoleJS 为空）")
	}

	refs := map[string]bool{}
	for _, re := range []*regexp.Regexp{
		regexp.MustCompile(`\$\("#([A-Za-z0-9_-]+)"\)`),
		regexp.MustCompile(`getElementById\("([A-Za-z0-9_-]+)"\)`),
		regexp.MustCompile(`querySelector\("#([A-Za-z0-9_-]+)"\)`),
	} {
		for _, m := range re.FindAllStringSubmatch(script, -1) {
			refs[m[1]] = true
		}
	}
	if len(refs) < 30 {
		t.Fatalf("只解析到 %d 个 id 引用，正则可能已失效", len(refs))
	}
	for id := range refs {
		if !defined[id] {
			t.Errorf("脚本引用了不存在的 id：%s", id)
		}
	}
}

// 每个 data-tab 必须有对应的 #tab-xxx 面板，且 <section> 数量与之相等。
//
// 脚本用 querySelectorAll("section") 切页：多出来的 <section>（比如把卡片
// 写成 section）会被当成标签页，切页时被摘掉 .on 而一起隐藏。
func TestConsoleTabsHavePanels(t *testing.T) {
	html := consoleSource()
	tabs := regexp.MustCompile(`data-tab="([A-Za-z0-9_-]+)"`).FindAllStringSubmatch(html, -1)
	if len(tabs) == 0 {
		t.Fatal("没有导航按钮")
	}
	for _, m := range tabs {
		if !strings.Contains(html, `id="tab-`+m[1]+`"`) {
			t.Errorf("导航按钮 %s 缺少对应面板", m[1])
		}
	}
	if got := strings.Count(html, "<section"); got != len(tabs) {
		t.Errorf("<section> 数量 %d 与导航按钮 %d 不一致", got, len(tabs))
	}
}

// 外壳布局与响应式收列的关键钩子。
func TestConsoleShellHooks(t *testing.T) {
	html := consoleSource()
	for name, needle := range map[string]string{
		"侧边栏":      `class="sidebar"`,
		"移动端开关":    `id="navToggle"`,
		"抽屉遮罩":     `id="scrim"`,
		"开关只在窄屏出现": `button.icon-btn.only-mobile{display:none}`,
		"账号表收列":    `#acctTable`,
		"日志表收列":    `#logTable`,
		"深链接":      `location.hash`,
	} {
		if !strings.Contains(html, needle) {
			t.Errorf("外壳缺少%s：%s", name, needle)
		}
	}
}

// 抽屉菜单 / 表头排序 / 中英双语的关键钩子。
//
// 这三块都是「静默失效」型功能：字典少一条不会报错，只是英文界面里
// 冒出一句中文；表头没有 wireSort 也不会报错，只是点了没反应。
func TestConsoleFeatures(t *testing.T) {
	html := consoleSource()

	for name, needle := range map[string]string{
		"语言按钮":      `id="lang"`,
		"英文字典":      `var I18N_EN = {`,
		"翻译入口":      `function translateDom(`,
		"语言应用":      `function applyLang(`,
		"语言深链接":     `[?&]lang=(en|zh)`,
		"语言持久化":     `deepaicode2api_lang`,
		"菜单构造":      `function openMenu(`,
		"账号行菜单":     `function rowMenu(`,
		"密钥行菜单":     `function keyMenu(`,
		"菜单关闭":      `function closeMenu(`,
		"账号表排序：服务端": `wireSort("#acctTable", "accounts"`,
		"模型表排序":     `wireSort("#modelTable", "models"`,
		"密钥表排序":     `wireSort("#keyTable", "keys"`,
		"日志表排序":     `wireSort("#logTable", "logs"`,
		"排序参数下推":    `"&sort=" + encodeURIComponent(st.key)`,
	} {
		if !strings.Contains(html, needle) {
			t.Errorf("缺少%s：%s", name, needle)
		}
	}

	// 每张声明了 data-sort 的表都必须有对应的 wireSort，否则点了没反应。
	for _, tbl := range []string{"acctTable", "modelTable", "keyTable", "logTable"} {
		if !strings.Contains(html, `id="`+tbl+`"`) {
			t.Errorf("表 %s 缺少 id（排序接线依赖它）", tbl)
		}
	}
	if n := strings.Count(html, "data-sort="); n < 20 {
		t.Errorf("data-sort 表头过少（%d），排序可能没接全", n)
	}
}

// 控制台要跟后端能力对齐：筛选、存量治理动作、K/M 单位都不能悄悄丢。
func TestConsoleBackendParity(t *testing.T) {
	html := consoleSource()

	// 筛选：后端 AccountPage / QueryLogs 支持的参数，前端必须真的发出去。
	for _, needle := range []string{
		`id="acctFilters"`, `id="acctGroup"`, `id="acctStatus"`, `id="acctQuery"`,
		`id="logFilters"`, `id="logEndpoint"`, `id="logAccount"`, `id="logMinStatus"`,
		`id="logKeyword"`, `id="logSince"`,
		`"&group="`, `"&status="`, `"&q="`, `"&endpoint="`, `"&min_status="`, `"&since="`,
	} {
		if !strings.Contains(html, needle) {
			t.Errorf("控制台缺少筛选接线：%s", needle)
		}
	}

	// 存量治理 / 密钥分组：后端 action 名必须与前端一致。
	for _, action := range []string{"assign-proxy", "assign-machine", "set-group", "mark-verified"} {
		if !strings.Contains(html, `"`+action+`"`) {
			t.Errorf("控制台缺少 action=%s 的入口", action)
		}
	}

	// 单位压缩：Token / 字符走 fmtCompact，且 KPI 与日志都在用。
	if !strings.Contains(html, "function fmtCompact(") {
		t.Fatal("缺少 fmtCompact")
	}
	if !strings.Contains(html, `fmtCompact(s.total_chars_out)`) ||
		!strings.Contains(html, `fmtCompact(s.total_tokens_in)`) {
		t.Error("概览的字符 / Token 应改用 fmtCompact")
	}
	for _, needle := range []string{`fmtCompact(l.prompt_len)`, `fmtCompact(l.output_len)`, `fmtCompact(l.tokens_in)`, `fmtCompact(l.tokens_out)`} {
		if !strings.Contains(html, needle) {
			t.Errorf("日志应改用 fmtCompact：%s", needle)
		}
	}

	// 严格模型模式开关要读到后端字段。
	if !strings.Contains(html, `strict_model`) {
		t.Error("设置页未接 strict_model")
	}
}

// 主题按钮与登录链接是两组容易回退的 UI 约定：
//   - 主题图标是内联 SVG（不再用 emoji），文案仍走 i18n 的「深色 / 浅色」；
//   - 浏览器登录面板要能把登录链接复制走（绝对地址），供另一个浏览器/机器打开。
func TestConsoleThemeAndLoginLink(t *testing.T) {
	html := consoleSource()

	for name, needle := range map[string]string{
		"太阳图标":  `id="themeIconSun"`,
		"月亮图标":  `id="themeIconMoon"`,
		"文案节点":  `id="themeLabel"`,
		"登录链接框": `id="oauthLink"`,
		"复制按钮":  `id="oauthCopy"`,
		"复制实现":  `function copyText(`,
	} {
		if !strings.Contains(html, needle) {
			t.Errorf("控制台缺少%s：%s", name, needle)
		}
	}

	// 图标改成 SVG 之后，applyTheme 不能再往按钮里塞 emoji 文本。
	if strings.Contains(html, "☀️ 浅色") || strings.Contains(html, "🌙 深色") {
		t.Error("主题按钮仍残留 emoji 文案")
	}
	if !strings.Contains(html, `label.textContent = mode === "light" ? "浅色" : "深色";`) {
		t.Error("applyTheme 应只切换「浅色 / 深色」文案，图标交给 SVG 控制显隐")
	}

	// 复制的是网关绝对地址，不能只写相对路径（换页面/换机器就失效）。
	if !strings.Contains(html, "location.origin") {
		t.Error("登录链接应拼成绝对地址（location.origin）")
	}
}
