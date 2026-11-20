package deepai

import (
	"net/url"
	"strings"
)

// MaskSecret 对敏感串脱敏，仅保留首尾片段。
func MaskSecret(s string) string {
	if s == "" {
		return ""
	}
	if len(s) <= 10 {
		return s[:1] + "***"
	}
	return s[:6] + "..." + s[len(s)-4:]
}

// MaskProxy 只返回代理的 host:port（绝不泄露用户名/密码）。
func MaskProxy(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return ""
	}
	return u.Host
}
