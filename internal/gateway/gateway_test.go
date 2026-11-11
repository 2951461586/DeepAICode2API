package gateway

import (
	"encoding/json"
	"testing"
)

// 本文件补的是「转换层的边角分支」：平时都靠 internal/server 的端到端用例
// 间接覆盖，一旦某个 helper 只在一种形状下被测到，改坏另一条分支不会有任何
// 用例报警（`go tool cover` 显示这些函数 0%~30% 覆盖）。
//
// 约定：全部走表驱动 + 明确的「非法输入返回零值」断言，因为上游字段类型
// 不稳定，这一层的契约就是「拿不到就当零值，绝不 panic」。

func TestConvertResponsesToolChoice(t *testing.T) {
	cases := []struct {
		name string
		in   any
		want any
	}{
		{"字符串原样透传", "auto", "auto"},
		{"空字符串透传", "", ""},
		{"function + name 转成 chat 形状", map[string]any{"type": "function", "name": "get_weather"},
			map[string]any{"type": "function", "function": map[string]any{"name": "get_weather"}}},
		{"name 为空返回 nil", map[string]any{"type": "function", "name": ""}, nil},
		{"name 非字符串返回 nil", map[string]any{"type": "function", "name": 42}, nil},
		{"type 不是 function 返回 nil", map[string]any{"type": "tool", "name": "x"}, nil},
		{"空对象返回 nil", map[string]any{}, nil},
		{"nil 返回 nil", nil, nil},
		{"数字返回 nil", 3, nil},
		{"数组返回 nil", []any{"auto"}, nil},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ConvertResponsesToolChoice(c.in)
			if !equalJSON(got, c.want) {
				t.Fatalf("ConvertResponsesToolChoice(%#v) = %#v，期望 %#v", c.in, got, c.want)
			}
		})
	}
}

func TestResponsesToolOutput(t *testing.T) {
	cases := []struct {
		name string
		in   any
		want string
	}{
		{"字符串原样", "plain", "plain"},
		{"空字符串", "", ""},
		{"nil 返回空", nil, ""},
		{"对象序列化", map[string]any{"a": 1}, `{"a":1}`},
		{"数组序列化", []any{"x", 2}, `["x",2]`},
		{"数字序列化", 7, "7"},
		{"不可序列化返回空", make(chan int), ""},
		{"函数不可序列化返回空", func() {}, ""},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ResponsesToolOutput(c.in); got != c.want {
				t.Fatalf("ResponsesToolOutput(%#v) = %q，期望 %q", c.in, got, c.want)
			}
		})
	}
}

func TestAnthropicImageURL(t *testing.T) {
	cases := []struct {
		name  string
		block map[string]any
		want  string
	}{
		{"没有 source", map[string]any{"type": "image"}, ""},
		{"source 不是对象", map[string]any{"source": "x"}, ""},
		{"base64 显式 media", map[string]any{"source": map[string]any{
			"type": "base64", "media_type": "image/jpeg", "data": "AAA"}},
			"data:image/jpeg;base64,AAA"},
		{"base64 缺 media 默认 png", map[string]any{"source": map[string]any{
			"type": "base64", "data": "AAA"}},
			"data:image/png;base64,AAA"},
		{"base64 缺 data 返回空", map[string]any{"source": map[string]any{
			"type": "base64", "media_type": "image/png"}}, ""},
		{"url 原样", map[string]any{"source": map[string]any{
			"type": "url", "url": "https://x/y.png"}}, "https://x/y.png"},
		{"url 缺值返回空", map[string]any{"source": map[string]any{"type": "url"}}, ""},
		{"未知 type 返回空", map[string]any{"source": map[string]any{
			"type": "file", "data": "AAA"}}, ""},
		{"type 不是字符串", map[string]any{"source": map[string]any{"type": 1}}, ""},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := AnthropicImageURL(c.block); got != c.want {
				t.Fatalf("AnthropicImageURL = %q，期望 %q", got, c.want)
			}
		})
	}
}

func TestAnthropicDocumentDataURL(t *testing.T) {
	cases := []struct {
		name  string
		block map[string]any
		want  string
	}{
		{"没有 source", map[string]any{}, ""},
		{"base64 图片转 data URL", map[string]any{"source": map[string]any{
			"type": "base64", "media_type": "image/webp", "data": "AAA"}},
			"data:image/webp;base64,AAA"},
		{"非 image 的 media 返回空（PDF 不外传）", map[string]any{"source": map[string]any{
			"type": "base64", "media_type": "application/pdf", "data": "AAA"}}, ""},
		{"media 缺失返回空", map[string]any{"source": map[string]any{
			"type": "base64", "data": "AAA"}}, ""},
		{"url 类型返回空（文档只认 base64）", map[string]any{"source": map[string]any{
			"type": "url", "url": "https://x/y.png"}}, ""},
		{"data 为空返回空", map[string]any{"source": map[string]any{
			"type": "base64", "media_type": "image/png"}}, ""},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := AnthropicDocumentDataURL(c.block); got != c.want {
				t.Fatalf("AnthropicDocumentDataURL = %q，期望 %q", got, c.want)
			}
		})
	}
}

func TestNumericCoercion(t *testing.T) {
	if got := IntOf(float64(3.9)); got != 3 {
		t.Fatalf("IntOf(float64 3.9) = %d，期望 3（截断）", got)
	}
	if got := IntOf(7); got != 7 {
		t.Fatalf("IntOf(int 7) = %d", got)
	}
	if got := IntOf(json.Number("12")); got != 12 {
		t.Fatalf("IntOf(json.Number 12) = %d", got)
	}
	if got := IntOf("12"); got != 0 {
		t.Fatalf("IntOf(string) 应为 0，得到 %d", got)
	}
	if got := IntOf(nil); got != 0 {
		t.Fatalf("IntOf(nil) 应为 0，得到 %d", got)
	}

	if got := FloatOf(2); got != 2.0 {
		t.Fatalf("FloatOf(int 2) = %v", got)
	}
	if got := FloatOf(1.25); got != 1.25 {
		t.Fatalf("FloatOf(1.25) = %v", got)
	}
	if got := FloatOf(json.Number("0.5")); got != 0.5 {
		t.Fatalf("FloatOf(json.Number 0.5) = %v", got)
	}
	if got := FloatOf("0.5"); got != 0 {
		t.Fatalf("FloatOf(string) 应为 0，得到 %v", got)
	}

	// LenOfString 按 rune 计数：中文与 emoji 都不能按字节算，
	// 否则 usage 里的 input_tokens 会和上游对不上。
	if got := LenOfString("中文"); got != 2 {
		t.Fatalf("LenOfString(中文) = %d，期望 2", got)
	}
	if got := LenOfString("a😀b"); got != 3 {
		t.Fatalf("LenOfString(a😀b) = %d，期望 3", got)
	}
	if got := LenOfString(42); got != 0 {
		t.Fatalf("LenOfString(非字符串) 应为 0，得到 %d", got)
	}

	if !BoolOf(true) || BoolOf("true") {
		t.Fatal("BoolOf 只认 bool 类型")
	}
	if got := StrOf(1); got != "" {
		t.Fatalf("StrOf(非字符串) 应为空，得到 %q", got)
	}
}

// equalJSON 用 JSON 往返比较，避免 map 比较时 key 顺序带来的噪音。
func equalJSON(a, b any) bool {
	ja, errA := json.Marshal(a)
	jb, errB := json.Marshal(b)
	if errA != nil || errB != nil {
		return false
	}
	return string(ja) == string(jb)
}
