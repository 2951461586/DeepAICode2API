package deepai

import (
	"strings"
	"testing"
)

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
