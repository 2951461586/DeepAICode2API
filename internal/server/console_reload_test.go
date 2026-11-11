package server

import (
	"regexp"
	"strings"
	"testing"
)

// bareRefreshRe 匹配「模块顶层不存在的 refresh( 调用」。
//
// 只认标识符本身：`refreshLogs(` 匹配不到（前面是字母），`obj.refresh(`
// 也匹配不到（前面是点），这两种在 app.js 里都是合法的。
var bareRefreshRe = regexp.MustCompile(`(^|[^\w.$])refresh\s*\(`)

// TestConsoleModulesUseInjectedReload 守住「刷新只能经 setReload 注入」这条约定。
//
// 背景：app.js 拆分后，刷新函数留在 app.js，下层模块靠 `setReload(fn)` 注入
// `reload`。拆分时 actions.js 与 render/keys.js 还留着 4 处裸 `refresh()`：
// accountOp / keyOp / finishCheckin / 「改绑分组」。它们不会在加载时报错，
// 只在「操作成功后收尾」这一刻抛 `refresh is not a function`——而且被同一个
// .catch 接住，提示成「失败：refresh is not a function」，看起来像接口失败。
//
// 浏览器里跑不起来才不会挡住 go test，所以这里做成静态契约：
// actions.js 与 render/*.js 里不允许出现裸 refresh(。
func TestConsoleModulesUseInjectedReload(t *testing.T) {
	checked := 0
	for _, name := range consoleScriptNames() {
		if name != "web/js/actions.js" && !strings.HasPrefix(name, "web/js/render/") {
			continue
		}
		checked++
		raw, err := consoleAssets.ReadFile(name)
		if err != nil {
			t.Fatalf("读取 %s 失败: %v", name, err)
		}
		src := stripJSComments(string(raw))
		for _, m := range bareRefreshRe.FindAllStringIndex(src, -1) {
			line := 1 + strings.Count(src[:m[0]], "\n")
			t.Errorf("%s:%d 调用了裸 refresh()；下层模块必须用注入的 reload()（见 actions.js setReload）", name, line)
		}
	}
	// 目录结构变了（比如 render/ 被合并）会让这条契约静默失效。
	if checked < 4 {
		t.Fatalf("只检查到 %d 个模块（actions.js + render/*），断言可能已失效", checked)
	}
}

// TestConsoleActionsInjectsReload 确认注入点在 actions.js 里仍然成立：
// 有 setReload 导出、有默认空实现，且收尾用的是 reload()。
func TestConsoleActionsInjectsReload(t *testing.T) {
	raw, err := consoleAssets.ReadFile("web/js/actions.js")
	if err != nil {
		t.Fatalf("读取 web/js/actions.js 失败: %v", err)
	}
	src := string(raw)
	for _, want := range []string{"export function setReload", "return reload();"} {
		if !strings.Contains(src, want) {
			t.Errorf("actions.js 里找不到 %q：刷新收尾必须继续走注入的 reload", want)
		}
	}
	// 默认值必须是函数：否则 setReload 之前的调用会抛 TypeError。
	if !strings.Contains(src, "let reload = function") {
		t.Error("actions.js 的 reload 默认值必须是可以直接调用的空实现")
	}
}

// TestStripJSComments 保证剥离器本身不吃掉真代码，也不把字符串当注释。
func TestStripJSComments(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		{"行注释", "a(); // refresh()\nb();", "a(); \nb();"},
		{"块注释", "a(); /* refresh()\nstill */ b();", "a(); \n b();"},
		{"字符串里的 //", `var u = "http://x"; refresh();`, `var u = "http://x"; refresh();`},
		{"字符串里的转义", `var s = "a\"b"; refresh();`, `var s = "a\"b"; refresh();`},
	}
	for _, c := range cases {
		if got := stripJSComments(c.in); got != c.want {
			t.Errorf("%s: stripJSComments(%q) = %q，期望 %q", c.name, c.in, got, c.want)
		}
	}
}
