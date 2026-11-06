package server

import (
	"regexp"
	"strings"
	"testing"
)

// 控制台是内嵌单页，改动容易静默回归（例如前后端字段名不一致）。
// 这里守住几条关键约定：主题、分页、以及模型字段的 camelCase。
func TestConsoleHTMLContracts(t *testing.T) {
	html := string(consoleHTML)
	if html == "" {
		t.Fatal("consoleHTML 为空")
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
	html := string(consoleHTML)
	if !strings.Contains(html, "var acctPageSize = 20;") {
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
	html := string(consoleHTML)
	defined := map[string]bool{}
	for _, m := range regexp.MustCompile(`id="([A-Za-z0-9_-]+)"`).FindAllStringSubmatch(html, -1) {
		defined[m[1]] = true
	}

	idx := strings.Index(html, "<script>")
	if idx < 0 {
		t.Fatal("找不到 <script>")
	}
	script := html[idx:]

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
	html := string(consoleHTML)
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
	html := string(consoleHTML)
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
