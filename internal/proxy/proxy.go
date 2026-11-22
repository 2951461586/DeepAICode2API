// Package proxy 提供号池共用的代理池：轮询分配、脱敏标签与代理文件读取。
//
// 历史背景：代理池原本长在注册链路内部（internal/register）。邮箱注册模块
// 移除后，号池仍需要"按账号分出口"（上游每日免费额度按出口 IP 共享），
// 所以把它拆成独立包，不再依赖任何注册/收信逻辑。
package proxy

import (
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync"
)

// ProxyPool 是线程安全的轮询代理池。
type ProxyPool struct {
	entries []string

	mu   sync.Mutex
	next int
}

// NewProxyPool 解析代理列表；raw 为空时返回 (nil, nil)。
//
// 支持换行 / 逗号 / 分号分隔，元素必须是完整 URL：
// http://user:pass@host:port、https://... 或 socks5://...
func NewProxyPool(raw string) (*ProxyPool, error) {
	entries := ParseProxyList(raw)
	if len(entries) == 0 {
		return nil, nil
	}
	for _, entry := range entries {
		u, err := url.Parse(entry)
		if err != nil || u.Host == "" {
			return nil, fmt.Errorf("代理格式非法：%s", ProxyLabel(entry))
		}
		switch strings.ToLower(u.Scheme) {
		case "http", "https", "socks5", "socks5h":
		default:
			return nil, fmt.Errorf("不支持的代理协议 %q：%s", u.Scheme, ProxyLabel(entry))
		}
	}
	return &ProxyPool{entries: entries}, nil
}

// ParseProxyList 把多行 / 逗号分隔的代理串解析为列表（去空、去重、去注释）。
//
// 注释按**行**判定（行首 `#`），而不是先拆分隔符再过滤：早先把空格 / 逗号 /
// 分号全当分隔符，于是 `#added-2026…（6 条，追加后无需重启）` 这类带空格或
// 逗号的注释会被切成残片，残片不以 `#` 开头就被当成代理串 →
// `代理格式非法：invalid`（2026-11-22 线上踩到）。注释就该是整行。
func ParseProxyList(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	replacer := strings.NewReplacer(";", "\n", ",", "\n", " ", "\n", "\t", "\n")
	var out []string
	seen := map[string]bool{}
	for _, line := range strings.Split(strings.ReplaceAll(raw, "\r\n", "\n"), "\n") {
		line = strings.TrimSpace(line)
		// 整行注释：跳过（允许注释里有空格 / 逗号 / 中文括号）。
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		for _, entry := range strings.Split(replacer.Replace(line), "\n") {
			entry = strings.TrimSpace(entry)
			if entry == "" || strings.HasPrefix(entry, "#") || seen[entry] {
				continue
			}
			seen[entry] = true
			out = append(out, entry)
		}
	}
	return out
}

// Len 返回代理数量。
func (p *ProxyPool) Len() int {
	if p == nil {
		return 0
	}
	return len(p.entries)
}

// Next 轮询返回一个代理；池为空时返回空串。
func (p *ProxyPool) Next() string {
	if p == nil || len(p.entries) == 0 {
		return ""
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	raw := p.entries[p.next%len(p.entries)]
	p.next++
	return raw
}

// ProxyLabel 返回不含凭据的 host:port（用于日志 / 控制台）。
func ProxyLabel(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "-"
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return "invalid"
	}
	return u.Host
}

// ReadProxyFile 读取代理文件（每行一个，支持 # 注释）。
func ReadProxyFile(path string) (string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}
