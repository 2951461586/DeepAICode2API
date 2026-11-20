package deepai

import (
	"fmt"
	"net/url"
	"os"
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

// EnvOr 返回环境变量值或默认值。
func EnvOr(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

// EnvInt 返回整型环境变量或默认值。
func EnvInt(key string, def int) int {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		var n int
		if _, err := fmt.Sscanf(v, "%d", &n); err == nil {
			return n
		}
	}
	return def
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
