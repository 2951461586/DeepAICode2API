package deepai

import (
	"os"
	"strings"
	"testing"
)

func TestRandomTokenIsHexAndUnique(t *testing.T) {
	a, b := RandomToken(16), RandomToken(16)
	if len(a) != 32 {
		t.Fatalf("RandomToken(16) 长度 = %d，期望 32", len(a))
	}
	if strings.Trim(a, "0123456789abcdef") != "" {
		t.Fatalf("RandomToken 应是小写 hex，得到 %q", a)
	}
	if a == b {
		t.Fatal("两次 RandomToken 不应相同")
	}

	if got := RandomToken(0); got != "" {
		t.Fatalf("RandomToken(0) = %q，期望空串", got)
	}
	if got := RandomHex(1); len(got) != 2 {
		t.Fatalf("RandomHex(1) 长度 = %d，期望 2", len(got))
	}
}

func TestKeyFingerprintAndShortHash(t *testing.T) {
	key := "sk-live-abcdef123456"
	fp := KeyFingerprint(key)
	if len(fp) != 16 {
		t.Fatalf("KeyFingerprint 长度 = %d，期望 16", len(fp))
	}
	if strings.Trim(fp, "0123456789abcdef") != "" {
		t.Fatalf("KeyFingerprint 应是小写 hex，得到 %q", fp)
	}
	if fp != KeyFingerprint("  "+key+"\n") {
		t.Fatal("KeyFingerprint 应先 TrimSpace")
	}
	if fp == KeyFingerprint(key+"x") {
		t.Fatal("不同 Key 不应撞指纹")
	}

	short := ShortHash(key)
	if len(short) != 8 || !strings.HasPrefix(fp, short) {
		t.Fatalf("ShortHash=%q 应是 KeyFingerprint=%q 的前 8 位", short, fp)
	}
}

func TestMaskSecret(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"", ""},
		{"a", "a***"},
		{"abcdefghij", "a***"},           // len == 10 走短分支
		{"abcdefghijk", "abcdef...hijk"}, // len 11 才保留首 6 尾 4
		{"sk-1234567890abcdef", "sk-123...cdef"},
	}
	for _, c := range cases {
		if got := MaskSecret(c.in); got != c.want {
			t.Errorf("MaskSecret(%q)=%q，期望 %q", c.in, got, c.want)
		}
	}
}

// TestMaskProxyNeverLeaksCredentials 是安全断言：代理里的用户名/密码
// 绝不能出现在日志或控制台标签里。
func TestMaskProxyNeverLeaksCredentials(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"", ""},
		{"   ", ""},
		{"http://user:secret@10.0.0.1:8080", "10.0.0.1:8080"},
		{"  socks5://u:p@proxy.local:1080  ", "proxy.local:1080"},
		{"http://10.0.0.1:8080", "10.0.0.1:8080"},
		{"not-a-url", ""},
		{"http://", ""},
	}
	for _, c := range cases {
		got := MaskProxy(c.in)
		if got != c.want {
			t.Errorf("MaskProxy(%q)=%q，期望 %q", c.in, got, c.want)
		}
		if strings.Contains(got, "secret") || strings.Contains(got, "u:p") {
			t.Errorf("MaskProxy(%q) 泄露了凭据: %q", c.in, got)
		}
	}
}

func TestEnvIntAndEnvOr(t *testing.T) {
	t.Setenv("DEEPAI_TEST_INT", " 42 ")
	if got := EnvInt("DEEPAI_TEST_INT", 7); got != 42 {
		t.Fatalf("EnvInt 合法值 = %d，期望 42", got)
	}

	t.Setenv("DEEPAI_TEST_INT", "abc")
	if got := EnvInt("DEEPAI_TEST_INT", 7); got != 7 {
		t.Fatalf("EnvInt 非法值应回退默认，得到 %d", got)
	}

	t.Setenv("DEEPAI_TEST_INT", "   ")
	if got := EnvInt("DEEPAI_TEST_INT", 7); got != 7 {
		t.Fatalf("EnvInt 纯空白应回退默认，得到 %d", got)
	}

	os.Unsetenv("DEEPAI_TEST_INT")
	if got := EnvInt("DEEPAI_TEST_INT", 7); got != 7 {
		t.Fatalf("EnvInt 未设置应回退默认，得到 %d", got)
	}

	t.Setenv("DEEPAI_TEST_STR", "  value  ")
	if got := EnvOr("DEEPAI_TEST_STR", "def"); got != "value" {
		t.Fatalf("EnvOr 应去空白，得到 %q", got)
	}
	t.Setenv("DEEPAI_TEST_STR", "   ")
	if got := EnvOr("DEEPAI_TEST_STR", "def"); got != "def" {
		t.Fatalf("EnvOr 纯空白应回退默认，得到 %q", got)
	}
}
