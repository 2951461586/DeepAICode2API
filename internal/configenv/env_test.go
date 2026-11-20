package configenv

import (
	"os"
	"testing"
)

// 从 internal/deepai 迁入（原 TestEnvIntAndEnvOr），并补齐 Bool / PositiveInt。
func TestIntAndOr(t *testing.T) {
	t.Setenv("DEEPAI_TEST_INT", " 42 ")
	if got := Int("DEEPAI_TEST_INT", 7); got != 42 {
		t.Fatalf("EnvInt 合法值 = %d，期望 42", got)
	}

	t.Setenv("DEEPAI_TEST_INT", "abc")
	if got := Int("DEEPAI_TEST_INT", 7); got != 7 {
		t.Fatalf("EnvInt 非法值应回退默认，得到 %d", got)
	}

	t.Setenv("DEEPAI_TEST_INT", "   ")
	if got := Int("DEEPAI_TEST_INT", 7); got != 7 {
		t.Fatalf("EnvInt 纯空白应回退默认，得到 %d", got)
	}

	os.Unsetenv("DEEPAI_TEST_INT")
	if got := Int("DEEPAI_TEST_INT", 7); got != 7 {
		t.Fatalf("EnvInt 未设置应回退默认，得到 %d", got)
	}

	t.Setenv("DEEPAI_TEST_STR", "  value  ")
	if got := Or("DEEPAI_TEST_STR", "def"); got != "value" {
		t.Fatalf("EnvOr 应去空白，得到 %q", got)
	}
	t.Setenv("DEEPAI_TEST_STR", "   ")
	if got := Or("DEEPAI_TEST_STR", "def"); got != "def" {
		t.Fatalf("EnvOr 纯空白应回退默认，得到 %q", got)
	}
}

func TestBoolAndPositiveInt(t *testing.T) {
	t.Setenv("CFG_BOOL", "YES")
	if !Bool("CFG_BOOL", false) {
		t.Fatal("YES 应解析为 true")
	}
	t.Setenv("CFG_BOOL", "off")
	if Bool("CFG_BOOL", true) {
		t.Fatal("off 应解析为 false")
	}
	t.Setenv("CFG_BOOL", "maybe")
	if !Bool("CFG_BOOL", true) {
		t.Fatal("无法识别时应返回默认值")
	}

	t.Setenv("CFG_N", "0")
	if got := PositiveInt("CFG_N", 7); got != 7 {
		t.Fatalf("0 应回落默认值，得到 %d", got)
	}
	t.Setenv("CFG_N", "-3")
	if got := PositiveInt("CFG_N", 7); got != 7 {
		t.Fatalf("负数应回落默认值，得到 %d", got)
	}
	t.Setenv("CFG_N", "12")
	if got := PositiveInt("CFG_N", 7); got != 12 {
		t.Fatalf("正整数原样返回，得到 %d", got)
	}
}
