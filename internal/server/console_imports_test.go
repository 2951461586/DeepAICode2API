package server

import (
	"path"
	"regexp"
	"strings"
	"testing"
)

// 控制台拆成 ES module 之后，「忘了 import」是掉一行代码、零语法错误、
// 零加载报错的故障：未绑定的标识符只在真正执行那一刻抛 ReferenceError，
// 而且通常被同一个 .catch 接住，伪装成「接口失败」。
//
// 2026-11-11 的真实回归：app.js 的 import 块只有 renderAccounts /
// renderKeys / renderSettings，漏了 ./render/logs.js；refreshLogs 里那句
// renderLogs(state.logs) 于是抛 `renderLogs is not defined`，toast 显示
// 「日志加载失败：renderLogs is not defined」——而 /admin/api/logs 是 200。
// 这类断裂 go test 原本完全看不见（文件在、路由在、字符串都在），所以这里
// 用静态契约把「跨模块标识符必须 import」钉住。

// consoleImportRe 匹配 `import { a, b as c } from './x.js'`（含多行写法）。
var consoleImportRe = regexp.MustCompile(`(?s)import\s*\{([^}]*)\}\s*from\s*['"]([^'"]+)['"]`)

// consoleExportRe 匹配 `export function f` / `export const f` / `export var f` / `export let f`。
var consoleExportRe = regexp.MustCompile(`(?m)^\s*export\s+(?:async\s+)?(?:function|const|let|var|class)\s+([A-Za-z_$][\w$]*)`)

// consoleLocalDeclRe 匹配本文件自己的顶层声明与函数内声明（含 `function f`, `var f =`）。
var consoleLocalDeclRe = regexp.MustCompile(`(?m)(?:^|[^\w$.])(?:function|const|let|var|class)\s+([A-Za-z_$][\w$]*)`)

// consoleNamedImport 是一条具名导入（orig 是目标模块的导出名，local 是本文件的名字）。
type consoleNamedImport struct {
	orig, local, from string
}

// consoleModule 是一个脚本的静态视图：去掉注释后的源码 + 导入表。
type consoleModule struct {
	name    string
	src     string
	imports map[string]consoleNamedImport // local -> import
	locals  map[string]bool               // 本文件声明过的名字
	exports map[string]bool               // 本文件导出的名字
}

// loadConsoleModules 读出全部内嵌脚本（注释已剥离，行号与原文对齐）。
func loadConsoleModules(t *testing.T) map[string]*consoleModule {
	t.Helper()
	mods := make(map[string]*consoleModule, 16)
	for _, name := range consoleScriptNames() {
		raw, err := consoleAssets.ReadFile(name)
		if err != nil {
			t.Fatalf("读取脚本 %s: %v", name, err)
		}
		src := stripJSComments(string(raw))
		m := &consoleModule{
			name:    name,
			src:     src,
			imports: map[string]consoleNamedImport{},
			locals:  map[string]bool{},
			exports: map[string]bool{},
		}
		for _, im := range consoleImportRe.FindAllStringSubmatch(src, -1) {
			from := im[2]
			for _, spec := range strings.Split(im[1], ",") {
				spec = strings.TrimSpace(spec)
				if spec == "" {
					continue
				}
				orig, local := spec, spec
				if parts := strings.SplitN(spec, " as ", 2); len(parts) == 2 {
					orig, local = strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
				}
				m.imports[local] = consoleNamedImport{orig: orig, local: local, from: from}
			}
		}
		for _, ex := range consoleExportRe.FindAllStringSubmatch(src, -1) {
			m.exports[ex[1]] = true
		}
		for _, d := range consoleLocalDeclRe.FindAllStringSubmatch(src, -1) {
			m.locals[d[1]] = true
		}
		// 形参与 catch 绑定也算本文件可用（渲染函数会收到注入的回调）。
		for _, fn := range regexp.MustCompile(`(?s)function\s*[A-Za-z_$\w]*\s*\(([^)]*)\)`).FindAllStringSubmatch(src, -1) {
			for _, p := range strings.Split(fn[1], ",") {
				p = strings.TrimSpace(strings.SplitN(p, "=", 2)[0])
				p = strings.TrimSpace(strings.SplitN(p, ":", 2)[0])
				if p != "" && !strings.ContainsAny(p, "{}[]()") {
					m.locals[p] = true
				}
			}
		}
		for _, cb := range regexp.MustCompile(`(?m)catch\s*\(\s*([A-Za-z_$][\w$]*)`).FindAllStringSubmatch(src, -1) {
			m.locals[cb[1]] = true
		}
		mods[name] = m
	}
	if len(mods) < 10 {
		t.Fatalf("只读到 %d 个脚本（期望 10+），consoleScriptNames 可能已失效", len(mods))
	}
	return mods
}

// TestConsoleModulesImportWhatTheyCall 是「拆模块漏 import」的静态防线：
//
//	只要某文件调用了别的模块导出的函数（`renderLogs(...)`），就必须在本文件
//	import 到它；否则只在运行到那一行时抛 ReferenceError，而错误提示还常被
//	同一个 .catch 改写成「接口失败」。
func TestConsoleModulesImportWhatTheyCall(t *testing.T) {
	mods := loadConsoleModules(t)

	// 导出名 -> 提供它的模块（重名说明拆分本身有问题，单独报出来）。
	owner := map[string]string{}
	for name, m := range mods {
		for ex := range m.exports {
			if prev, dup := owner[ex]; dup && prev != name {
				t.Errorf("%s 与 %s 都导出了 %s：重名会让 import 的指向变得不可预测", prev, name, ex)
			}
			owner[ex] = name
		}
	}
	if len(owner) < 20 {
		t.Fatalf("只解析到 %d 个导出名，export 正则可能已失效", len(owner))
	}

	calls := 0
	for _, name := range consoleScriptNames() {
		m := mods[name]
		for symbol := range owner {
			if owner[symbol] == name {
				continue // 自己导出的，未绑定调用也不会 ReferenceError
			}
			// 裸调用 / new：`renderLogs(`、`new Foo(`；点号后的同名方法不算。
			call := regexp.MustCompile(`(?:^|[^\w$.])` + regexp.QuoteMeta(symbol) + `\s*\(`)
			idx := call.FindStringIndex(m.src)
			if idx == nil {
				continue
			}
			calls++
			_, imported := m.imports[symbol]
			if imported || m.locals[symbol] {
				continue
			}
			line := 1 + strings.Count(m.src[:idx[1]], "\n")
			t.Errorf("%s:%d 调用了 %s()，但本文件没有 import 它（导出在 %s）。"+
				"ESM 的未绑定标识符不会在加载时报错，只会在执行到这一行时抛 ReferenceError。",
				name, line, symbol, owner[symbol])
		}
	}
	if calls < 20 {
		t.Fatalf("只匹配到 %d 处跨模块调用，扫描可能已失效", calls)
	}
}

// TestConsoleImportsResolve 校验 import 的落点：路径必须存在，导出名必须真的
// 在目标模块里。拆分时最常见的两种手误——路径写成 ./render/log.js（少个 s）、
// 名字拼错——在这里直接失败，而不是浏览器里 404 / undefined。
func TestConsoleImportsResolve(t *testing.T) {
	mods := loadConsoleModules(t)

	checked := 0
	for _, name := range consoleScriptNames() {
		m := mods[name]
		for _, im := range m.imports {
			checked++
			target := name
			if strings.HasPrefix(im.from, "./") || strings.HasPrefix(im.from, "../") {
				target = path.Join(path.Dir(name), im.from)
			} else {
				t.Errorf("%s 里 import 了非相对路径 %q：控制台只从内嵌资源加载", name, im.from)
				continue
			}
			tm, ok := mods[target]
			if !ok {
				t.Errorf("%s 里 import %q 解析到 %s，内嵌资源里没有这个文件", name, im.from, target)
				continue
			}
			if !tm.exports[im.orig] {
				t.Errorf("%s 里 import 的 %s 在 %s 里没有 export", name, im.orig, target)
			}
		}
	}
	if checked < 30 {
		t.Fatalf("只解析到 %d 条 import，正则可能已失效", checked)
	}
}
