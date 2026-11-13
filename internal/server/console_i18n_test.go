package server

import (
	"regexp"
	"strings"
	"testing"
)

// i18nMissingAllowlist 是「已知不做翻译」的中文片段。
//
// 它们分三类，都不是漏翻：
//   - 拼接片段：`t("共 ") + n + t(" 个模型")`。正确做法是改成
//     `t("共 {n} 个模型", {n})`，属于前端重写，不在 i18n 字典的职责内；
//   - 标识符而非文案：`Language / 语言`（语言选择器自指）、`中`（切换按钮）；
//   - 转义后查不到的：`已创建密钥（仅显示一次）：\n` 在 JS 里求值带换行，
//     按原文做键永远命中不了，需要改成不带换行的句子。
//
// 新增条目必须在这里写明理由——这个名单只应该变短。
var i18nMissingAllowlist = map[string]string{
	" 个模型":                    "字符串拼接片段",
	" 个账号，失败 ":                "字符串拼接片段",
	"，超过 8MB 自动轮转；支持":         "字符串拼接片段",
	"模型目录已更新：共 ":              "字符串拼接片段",
	"&lt;数据文件&gt;-logs.jsonl": "HTML 实体占位符，不是文案",
	"Language / 语言":           "语言选择器自指，故意双语",
	"DeepAICode2API 控制台":      "浏览器标签页标题：品牌名 + 固定后缀",
	"中":                       "EN/中 切换按钮的标签",
	`已创建密钥（仅显示一次）：\n`:         "JS 求值后含换行，键对不上",
}

func hasCJK(s string) bool {
	for _, r := range s {
		if r >= 0x4e00 && r <= 0x9fff {
			return true
		}
	}
	return false
}

// i18nDictBlocks 截出全部 `export const DICT = { ... }` 的完整文本。
//
// 字典已按用途拆到 js/i18n/dict-*.js（shell / pages），所以会有多个块；
// 仍然用**字符串感知的花括号配对**定位结尾（字面量里的 `{n}` 与 JSON 示例
// 不能干扰配对）。
func i18nDictBlocks(t *testing.T, src string) []string {
	t.Helper()
	const marker = "_DICT = {"
	var out []string
	offset := 0
	for {
		rel := strings.Index(src[offset:], marker)
		if rel < 0 {
			break
		}
		start := offset + rel
		depth := 0
		inStr := false
		closed := false
		for i := start; i < len(src); i++ {
			c := src[i]
			if inStr {
				if c == '\\' {
					i++
					continue
				}
				if c == '"' {
					inStr = false
				}
				continue
			}
			switch c {
			case '"':
				inStr = true
			case '{':
				depth++
			case '}':
				depth--
				if depth == 0 {
					out = append(out, src[start:i+1])
					offset = i + 1
					closed = true
				}
			}
			if closed {
				break
			}
		}
		if !closed {
			t.Fatal("DICT 字典括号未闭合")
		}
	}
	if len(out) == 0 {
		t.Fatal("找不到 DICT 字典块")
	}
	return out
}

// i18nDictBlock 返回全部字典块的拼接（供词条解析 / 重复键检查使用）。
func i18nDictBlock(t *testing.T, src string) string {
	t.Helper()
	return strings.Join(i18nDictBlocks(t, src), "\n")
}

// i18nDictionary 解析脚本里的 I18N_EN，返回「中文原文 → 英文」。
func i18nDictionary(t *testing.T, src string) map[string]string {
	t.Helper()
	body := i18nDictBlock(t, src)

	entry := regexp.MustCompile(`"((?:[^"\\]|\\.)*)"\s*:`)
	dict := map[string]string{}
	for _, m := range entry.FindAllStringSubmatch(body, -1) {
		dict[m[1]] = ""
	}
	if len(dict) < 100 {
		t.Fatalf("只解析到 %d 条词条，正则可能已失效", len(dict))
	}
	return dict
}

// TestConsoleI18nCoverage 是「翻译缺口」的棘轮测试。
//
// 控制台用「中文原文」当 i18n 键：漏加词条时翻成英文会**静默**保留中文，
// 用户看到中英混排而 CI 全绿。这里把源码里所有含中文的文案抽出来与字典求差集，
// 只允许 i18nMissingAllowlist 里列出的结构性片段缺席。
func TestConsoleI18nCoverage(t *testing.T) {
	markup := consoleMarkup()
	script := consoleJS()
	dict := i18nDictionary(t, script)

	candidates := map[string]string{} // 文案 → 出处

	// 1) JS 字符串字面量（动态渲染的文案）。
	for _, m := range regexp.MustCompile(`"((?:[^"\\]|\\.)*)"`).FindAllStringSubmatch(script, -1) {
		if v := m[1]; hasCJK(v) && len(v) < 120 {
			candidates[v] = "JS 字面量"
		}
	}
	// 2) 标记里的文本节点（静态文案）。
	for _, m := range regexp.MustCompile(`>([^<>]+)<`).FindAllStringSubmatch(markup, -1) {
		if v := strings.TrimSpace(m[1]); hasCJK(v) && len(v) < 80 {
			candidates[v] = "标记文本"
		}
	}
	// 3) 占位符与 title（translateAttrs 会翻这两类）。
	for _, m := range regexp.MustCompile(`(?:placeholder|title)="([^"]+)"`).FindAllStringSubmatch(markup, -1) {
		if hasCJK(m[1]) {
			candidates[m[1]] = "属性"
		}
	}

	if len(candidates) < 100 {
		t.Fatalf("只抽到 %d 条候选文案，提取逻辑可能已失效", len(candidates))
	}

	var missing []string
	for text, from := range candidates {
		if _, ok := dict[text]; ok {
			continue
		}
		if _, ok := i18nMissingAllowlist[text]; ok {
			continue
		}
		missing = append(missing, from+"："+text)
	}
	if len(missing) > 0 {
		t.Errorf("以下文案缺少英文词条（翻成英文会静默显示中文）：\n  %s\n"+
			"补进 index.html 的 I18N_EN，或（确实不该翻时）加进 i18nMissingAllowlist 并写明理由",
			strings.Join(missing, "\n  "))
	}

	// 允许名单只应该变短：已经补上的词条要从名单里删掉。
	for text := range i18nMissingAllowlist {
		if _, ok := dict[text]; ok {
			t.Errorf("i18nMissingAllowlist 里的 %q 已经有词条了，请从名单中删除", text)
		}
		if _, ok := candidates[text]; !ok {
			t.Errorf("i18nMissingAllowlist 里的 %q 在源码中已不存在，请从名单中删除", text)
		}
	}
}

// TestConsoleI18nNoDuplicateKeys 防止手改字典时出现重复键。
//
// JS 对象里重复键不报错，后者静默覆盖前者——表现是「改了词条但不生效」。
func TestConsoleI18nNoDuplicateKeys(t *testing.T) {
	html := consoleJS()
	body := i18nDictBlock(t, html)

	seen := map[string]int{}
	for _, m := range regexp.MustCompile(`"((?:[^"\\]|\\.)*)"\s*:`).FindAllStringSubmatch(body, -1) {
		seen[m[1]]++
	}
	for k, n := range seen {
		if n > 1 {
			t.Errorf("I18N_EN 里 %q 出现 %d 次（后者会静默覆盖前者）", k, n)
		}
	}
}

// TestConsoleI18nNoDeadEntries 是覆盖率棘轮的**反向**方向：词条必须在源码里
// 真的被用到，否则就是死词条。
//
// 为什么需要这条：功能下线（注册链路 / 指纹库 / 代理池输入框）时，源码删了
// 但字典没人清理，词条会一直烂在里面。正向棘轮（覆盖率）只看「缺不缺」，
// 对「多不多」完全无感 —— 2026-11-11 首次引入本测试时清掉了 9 条死词条。
//
// 判定方式：把字典块本身从源码里挖掉，剩下的 JS + 标记里出现该原文即算「在用」。
// 字典里形如 {"a\":\"b\"} 的转义键先反转义再匹配，避免因为 JSON 示例里的
// 反斜杠误判成死词条。
func TestConsoleI18nNoDeadEntries(t *testing.T) {
	script := consoleJS()
	dict := i18nDictionary(t, script)
	// 挖掉字典：否则每个键都能在字典声明处匹配到自己。
	// 同时去掉注释：注释里顺带提到某个词条（例如“阶段 A 的搬家”）不算文案在用 ——
	// 第一版没去注释，把一个真死词条当成“还在用”放过了。
	rest := script
	for _, block := range i18nDictBlocks(t, script) {
		rest = strings.Replace(rest, block, "", 1)
	}
	rest = stripJSComments(rest) + "\n" + stripHTMLComments(consoleMarkup())

	var dead []string
	for key := range dict {
		decoded := strings.NewReplacer(`\"`, `"`, `\\`, `\`).Replace(key)
		if strings.Contains(rest, decoded) || strings.Contains(rest, key) {
			continue
		}
		dead = append(dead, key)
	}
	if len(dead) > 0 {
		t.Errorf("以下词条在源码里已经没人用了（删掉它们，功能下线时别只删代码）：\n  %s",
			strings.Join(dead, "\n  "))
	}
}

// stripJSComments 去掉 // 与 /* */ 注释，字符串字面量与模板串原样保留。
//
// 块注释里的换行会保留：控制台的契约测试要靠它从 `src` 直接数出行号。
func stripJSComments(src string) string {
	var b strings.Builder
	b.Grow(len(src))
	inStr, inTpl, inLine, inBlock := false, false, false, false
	for i := 0; i < len(src); i++ {
		c := src[i]
		switch {
		case inLine:
			if c == '\n' {
				inLine = false
				b.WriteByte(c)
			}
		case inBlock:
			if c == '*' && i+1 < len(src) && src[i+1] == '/' {
				inBlock = false
				i++
				continue
			}
			if c == '\n' {
				b.WriteByte(c)
			}
		case inStr, inTpl:
			b.WriteByte(c)
			if c == '\\' && i+1 < len(src) {
				i++
				b.WriteByte(src[i])
				continue
			}
			if inStr && c == '"' {
				inStr = false
			}
			if inTpl && c == '`' {
				inTpl = false
			}
		default:
			if c == '/' && i+1 < len(src) && src[i+1] == '/' {
				inLine = true
				i++
				continue
			}
			if c == '/' && i+1 < len(src) && src[i+1] == '*' {
				inBlock = true
				i++
				continue
			}
			if c == '"' {
				inStr = true
			}
			if c == '`' {
				inTpl = true
			}
			b.WriteByte(c)
		}
	}
	return b.String()
}

// stripHTMLComments 去掉 <!-- ... -->，理由同上。
func stripHTMLComments(src string) string {
	for {
		start := strings.Index(src, "<!--")
		if start < 0 {
			return src
		}
		end := strings.Index(src[start:], "-->")
		if end < 0 {
			return src
		}
		src = src[:start] + src[start+end+3:]
	}
}
