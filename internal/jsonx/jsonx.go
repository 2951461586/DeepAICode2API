// Package jsonx 提供「从解析后的 JSON 里宽松取值」的小工具。
//
// 为什么单独成包：这些函数原先住在 internal/gateway（协议转换），于是
// internal/respstate（历史暂存）为了取一个字段就得依赖整个协议翻译层 ——
// 一个状态仓库反向依赖协议层。抽成叶子包之后依赖方向变成：
//
//	deepai / gateway / respstate / server → jsonx（只依赖标准库）
//
// 取值策略一律「拿不到就给零值」：上游字段类型不稳定（有时 number 有时
// string、有时 null），一个字段形状不对不该让整轮请求失败。
//
// 注意语义是**刻意保守**的：数字字符串不解析（见 IntOf），字符串长度按
// rune 计（见 LenOfString）。这两条都是行为契约，有测试锁定（
// internal/gateway/gateway_test.go 的 TestNumericCoercion / TestAnthropicTextLen）。
package jsonx

import "encoding/json"

// StrOf 取字符串，非字符串返回空串。
func StrOf(v any) string {
	s, _ := v.(string)
	return s
}

// BoolOf 取布尔，非布尔返回 false。
func BoolOf(v any) bool {
	b, _ := v.(bool)
	return b
}

// IntOf 取整数，只认 JSON 解出来的三种数值形态。
//
// 字符串形态（"12"）**不解析**：上游的 `"max_tokens": "12"` 这样的字段
// 至今没出现过，但把字符串当数字会让「字段类型写错」从报错变成静默生效。
func IntOf(v any) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case json.Number:
		i, _ := n.Int64()
		return int(i)
	}
	return 0
}

// FloatOf 取浮点数，只认 JSON 解出来的三种数值形态。
func FloatOf(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case int:
		return float64(n)
	case json.Number:
		f, _ := n.Float64()
		return f
	}
	return 0
}

// LenOfString 取字符串长度（按 rune 计），非字符串返回 0。
//
// 必须按 rune：这些长度用于「输出 token 估算」与日志，中文按字节算会
// 放大三倍，让成本与长度统计全部失真。
func LenOfString(v any) int {
	s, _ := v.(string)
	return len([]rune(s))
}

// FirstString 返回第一个非空字符串。
//
// 用于「同一语义可能有多个字段名」的上游形态（如工具调用 id 可能叫
// call_id，也可能叫 id）。
func FirstString(vals ...any) string {
	for _, v := range vals {
		if s, ok := v.(string); ok && s != "" {
			return s
		}
	}
	return ""
}
