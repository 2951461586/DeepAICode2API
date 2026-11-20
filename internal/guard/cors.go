package guard

import (
	"net"
	"net/http"
	"net/url"
	"strings"
)

// --- CORS 与来源校验 ---
//
// 全是纯函数：设置由调用方传入，便于单测（见 cors_test.go）。

// Origin 返回应当回给浏览器的 Allow-Origin 值，空串表示不放行。
//
// configured 是控制台设置里的显式允许列表（逗号分隔）：非空时只认列表内的
// 精确匹配（不提供通配符，含 '*' 的条目一律视为无效）。列表为空时退回默认
// 策略：仅放行环回来源（localhost / 127.0.0.1 / [::1]，任意端口）。
func Origin(raw, configured string) string {
	if raw == "" || raw == "null" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return ""
	}

	// 1) 显式允许列表（逗号分隔，值来自控制台设置）
	if cfg := strings.TrimSpace(configured); cfg != "" {
		if InAllowList(raw, cfg) {
			return raw
		}
		return ""
	}

	// 2) 默认仅本机：主机名必须在环回集合内
	if !isLoopbackHost(u.Hostname()) {
		return ""
	}
	if port := u.Port(); port != "" {
		return u.Scheme + "://" + net.JoinHostPort(u.Hostname(), port)
	}
	return u.Scheme + "://" + u.Hostname()
}

// InAllowList 报告 origin 是否命中显式允许列表。
//
// 逐项精确匹配（大小写不敏感），含通配符的条目视为无效——宁可拒绝也不要
// 悄悄变成 `*`。管理接口的同源校验与控制台跨域共用这一个判定。
func InAllowList(origin, configured string) bool {
	for _, entry := range strings.Split(configured, ",") {
		entry = strings.TrimSpace(entry)
		if entry != "" && !hasWildcard(entry) && strings.EqualFold(entry, origin) {
			return true
		}
	}
	return false
}

// WriteHeaders 仅在来源通过校验时写入跨域响应头。
func WriteHeaders(w http.ResponseWriter, allowed string) {
	if allowed == "" || hasWildcard(allowed) {
		return
	}
	h := w.Header()
	h.Set("Access-Control-Allow-Origin", allowed)
	h.Set("Vary", "Origin")
	h.Set("Access-Control-Allow-Headers", "Authorization, Content-Type, x-api-key, anthropic-version, anthropic-beta, X-Admin-Password, X-Session-Affinity")
	h.Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
	h.Set("Access-Control-Expose-Headers", "X-Request-Id, Retry-After")
}

func hasWildcard(s string) bool { return strings.IndexByte(s, '*') >= 0 }

func isLoopbackHost(host string) bool {
	switch strings.ToLower(host) {
	case "localhost", "127.0.0.1", "::1":
		return true
	}
	return false
}

// IsLoopbackAddr 报告 remoteAddr 是否来自本机回环。
func IsLoopbackAddr(remoteAddr string) bool {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
