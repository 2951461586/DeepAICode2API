// Package registry 维护对外暴露的模型清单与别名解析。
//
// DeepAI Code 的模型目录由上游 /deepai_code/models 动态下发，本包只做
// 缓存快照上的解析与别名扩展，不承担网络请求。
package registry

import (
	"strings"

	"deepaicode2api/internal/deepai"
)

// Resolve 把用户传入的模型名解析为目录中的模型。
//
// 依次尝试：去掉 deepai/ 前缀 → 精确 ID → 自定义别名 → 内置别名 →
// 包含匹配 → 回落到 defaultID → 回落到第一个模型。
//
// 无法识别时不报错而是回落：这样任意客户端（Claude Code / Cline /
// OpenAI SDK）都能用自己习惯的模型名开箱即用。
func Resolve(catalog []deepai.Model, aliases map[string]string, name, defaultID string) deepai.Model {
	if m, ok := ResolveStrict(catalog, aliases, name, defaultID); ok {
		return m
	}
	n := normalize(name)
	if n != "" {
		// 宽松匹配：模型名包含用户输入，或用户输入包含模型 ID。
		for _, m := range catalog {
			if strings.Contains(strings.ToLower(m.ID), n) {
				return m
			}
		}
	}

	if m, ok := lookup(catalog, normalize(defaultID)); ok {
		return m
	}
	if len(catalog) > 0 {
		return catalog[0]
	}
	return deepai.DefaultModelEntry()
}

// ResolveStrict 与 Resolve 的“有依据”解析部分一致，但不做任何隐式回落。
//
// 依次尝试：去掉前缀 → 精确 ID → 自定义别名 → 内置别名 → 模型名称匹配。
// 全部不命中时返回 ok=false，**不**回落默认模型、**不**做子串包含匹配。
//
// 这是严格模式（Settings.StrictModel）的依据：子串匹配会把 "gpt-6" 解成
// 目录里第一个 “gpt-6-*“（可能是另一个模型），默认回落更会把任意拼写
// 错误变成默认模型——两者都会让调用方以为自己测的是 A，实际跑的是 B。
func ResolveStrict(catalog []deepai.Model, aliases map[string]string, name, defaultID string) (deepai.Model, bool) {
	n := normalize(name)
	if n == "" {
		return deepai.Model{}, false
	}
	if m, ok := lookup(catalog, n); ok {
		return m, true
	}
	if target, ok := lookupAlias(aliases, n); ok {
		if m, ok := lookup(catalog, target); ok {
			return m, true
		}
	}
	if target, ok := builtinAliases[n]; ok {
		if m, ok := lookup(catalog, target); ok {
			return m, true
		}
	}
	for _, m := range catalog {
		if n == strings.ToLower(strings.ReplaceAll(m.Name, " ", "-")) {
			return m, true
		}
	}
	return deepai.Model{}, false
}

// Suggest 返回与 name 相关的目录 ID，用于严格模式下的可执行错误提示。
//
// 只做提示，不参与解析。相似度只要求“够像”：子串命中，或忽略标点后
// 前 4 个字符相同（`gpt-6-sol` ↔ `gpt-6.1-sol` / `gpt-6-luna`，
// `mimo-v2.6-flash` ↔ `mimo-2.6-flash`）。
func Suggest(catalog []deepai.Model, name string, max int) []string {
	n := normalize(name)
	if n == "" || max <= 0 {
		return nil
	}
	key := similarityKey(n)
	out := make([]string, 0, max)
	for _, m := range catalog {
		id := strings.ToLower(m.ID)
		label := normalize(m.Name)
		near := strings.Contains(id, n) || strings.Contains(n, id) || strings.Contains(label, n)
		if !near {
			near = commonPrefix(key, similarityKey(m.ID)) >= 4
		}
		if !near {
			continue
		}
		out = append(out, m.ID)
		if len(out) >= max {
			break
		}
	}
	return out
}

// similarityKey 去掉分隔符，避免 `6.1` 与 `6-1`、“v2.6” 与 “2.6” 的差异。
func similarityKey(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func commonPrefix(a, b string) int {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	i := 0
	for i < n && a[i] == b[i] {
		i++
	}
	return i
}

// List 返回对外模型清单（含自定义别名）。
func List(catalog []deepai.Model, aliases map[string]string) []deepai.Model {
	out := make([]deepai.Model, len(catalog))
	copy(out, catalog)
	for alias, target := range aliases {
		alias = strings.TrimSpace(alias)
		if alias == "" {
			continue
		}
		if m, ok := lookup(catalog, normalize(target)); ok {
			m.ID = alias
			m.Name = alias + " → " + m.Name
			out = append(out, m)
		}
	}
	return out
}

// builtinAliases 是常见客户端模型名到 DeepAI 模型的映射。
//
// 只包含默认免费模型：用户可在控制台覆盖为任意目标。
var builtinAliases = map[string]string{
	"deepai":            deepai.DefaultModel,
	"deepai-code":       deepai.DefaultModel,
	"default":           deepai.DefaultModel,
	"gpt-3.5-turbo":     deepai.DefaultModel,
	"gpt-4":             deepai.DefaultModel,
	"gpt-4o":            deepai.DefaultModel,
	"gpt-4o-mini":       deepai.DefaultModel,
	"gpt-4.1":           deepai.DefaultModel,
	"claude-3-5-sonnet": deepai.DefaultModel,
	"claude-sonnet-4":   deepai.DefaultModel,
	"claude-3-haiku":    deepai.DefaultModel,
	"deepseek-chat":     deepai.DefaultModel,
	"glm-4-flash":       deepai.DefaultModel,
}

func normalize(name string) string {
	n := strings.TrimSpace(strings.ToLower(name))
	// 客户端常用 deepai/<id> 或 openai/<id> 前缀。
	for _, prefix := range []string{"deepai/", "deepai:", "openai/", "openai:"} {
		if strings.HasPrefix(n, prefix) {
			n = strings.TrimSpace(strings.TrimPrefix(n, prefix))
			break
		}
	}
	// 模型名里的空白当分隔符："Claude Opus 5.5" 与 "claude-opus-5.5" 等价。
	return strings.Join(strings.Fields(n), "-")
}

func lookup(catalog []deepai.Model, id string) (deepai.Model, bool) {
	if id == "" {
		return deepai.Model{}, false
	}
	for _, m := range catalog {
		if strings.EqualFold(m.ID, id) {
			return m, true
		}
	}
	return deepai.Model{}, false
}

func lookupAlias(aliases map[string]string, key string) (string, bool) {
	for k, v := range aliases {
		if strings.EqualFold(strings.TrimSpace(k), key) {
			return normalize(v), true
		}
	}
	return "", false
}

// Find 返回 ID 精确匹配的模型。
func Find(catalog []deepai.Model, id string) (deepai.Model, bool) {
	return lookup(catalog, normalize(id))
}
