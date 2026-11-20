package jsonx

import (
	"encoding/json"
	"testing"
)

// 这一组用例锁定的是「宽松取值的语义边界」。
//
// 它们原先散在 internal/gateway 的两个用例里（TestNumericCoercion /
// TestAnthropicTextLen），现在跟着代码搬到这里：契约由包自己守。

func TestStrOf(t *testing.T) {
	cases := []struct {
		in   any
		want string
	}{
		{"x", "x"},
		{"", ""},
		{nil, ""},
		{1, ""},
		{true, ""},
	}
	for _, c := range cases {
		if got := StrOf(c.in); got != c.want {
			t.Fatalf("StrOf(%#v) = %q，期望 %q", c.in, got, c.want)
		}
	}
}

func TestBoolOf(t *testing.T) {
	if !BoolOf(true) {
		t.Fatal("BoolOf(true) 应为 true")
	}
	for _, in := range []any{nil, false, 0, "true", 1} {
		if BoolOf(in) {
			t.Fatalf("BoolOf(%#v) 应为 false", in)
		}
	}
}

// TestIntOfRejectsString 锁定「数字字符串不解析」。
//
// 这是刻意的保守语义：把 "12" 当 12 会让「字段类型写错」从报错变成静默生效。
func TestIntOfRejectsString(t *testing.T) {
	if got := IntOf("12"); got != 0 {
		t.Fatalf(`IntOf("12") = %d，期望 0（字符串不解析）`, got)
	}
	if got := IntOf(float64(12)); got != 12 {
		t.Fatalf("IntOf(12.0) = %d，期望 12", got)
	}
	if got := IntOf(12); got != 12 {
		t.Fatalf("IntOf(12) = %d，期望 12", got)
	}
	if got := IntOf(json.Number("12")); got != 12 {
		t.Fatalf("IntOf(json.Number) = %d，期望 12", got)
	}
	for _, in := range []any{nil, true, []any{1}, map[string]any{}} {
		if got := IntOf(in); got != 0 {
			t.Fatalf("IntOf(%#v) = %d，期望 0", in, got)
		}
	}
	// float64 按截断取整（上游的 max_output_tokens 可能就是 12.0）。
	if got := IntOf(12.9); got != 12 {
		t.Fatalf("IntOf(12.9) = %d，期望截断为 12", got)
	}
}

func TestFloatOf(t *testing.T) {
	if got := FloatOf(float64(1.5)); got != 1.5 {
		t.Fatalf("FloatOf(1.5) = %v", got)
	}
	if got := FloatOf(2); got != 2 {
		t.Fatalf("FloatOf(2) = %v", got)
	}
	if got := FloatOf(json.Number("3.25")); got != 3.25 {
		t.Fatalf("FloatOf(json.Number) = %v", got)
	}
	if got := FloatOf("3.25"); got != 0 {
		t.Fatalf("FloatOf(string) = %v，期望 0（字符串不解析）", got)
	}
	if got := FloatOf(nil); got != 0 {
		t.Fatalf("FloatOf(nil) = %v", got)
	}
}

// TestLenOfStringCountsRunes 锁定「按 rune 计」。
//
// 按字节算会把中文成本放大三倍，输出长度统计与费用估算全部失真。
func TestLenOfStringCountsRunes(t *testing.T) {
	if got := LenOfString("你好abc"); got != 5 {
		t.Fatalf(`LenOfString("你好abc") = %d，期望 5（按 rune 计）`, got)
	}
	if got := LenOfString(""); got != 0 {
		t.Fatalf("LenOfString(\"\") = %d", got)
	}
	if got := LenOfString(nil); got != 0 {
		t.Fatalf("LenOfString(nil) = %d", got)
	}
}

func TestFirstString(t *testing.T) {
	if got := FirstString(nil, "", "b", "c"); got != "b" {
		t.Fatalf(`FirstString(nil,"","b","c") = %q，期望 b`, got)
	}
	if got := FirstString(nil, 1, ""); got != "" {
		t.Fatalf("全空/非字符串应返回空串，实际 %q", got)
	}
	if got := FirstString(); got != "" {
		t.Fatalf("无参应返回空串，实际 %q", got)
	}
}
