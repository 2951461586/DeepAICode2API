package proxy

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"
)

// ErrUnsupportedScheme 表示代理串能解析，但不是我们实现的三种之外的类型。
var ErrUnsupportedScheme = errors.New("proxy: unsupported scheme")

// Parse 规范化并校验代理串。
//
// 线上踩过的坑（2026-11-22）：账号里存的出口是 `host:port`（**没有 scheme**），
// 而 `url.Parse` 会把 `host:port` 里的 `host` 当成 scheme（点号在 scheme 字符集里合法），
// 于是 `u.Host` 为空、`transport.Proxy` 没被设置，**按账号分出口静默失效** ——
// 所有账号都退回到环境变量里的全局代理。这里统一补 `http://` 并在解析失败时
// 明确报错，调用方据此决定「用全局出口」而不是假装用了专属出口。
func Parse(raw string) (*url.URL, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return nil, errors.New("proxy: empty")
	}
	if !strings.Contains(s, "://") {
		// 绝大多数代理网关（含 bringyour / proxylane）走 HTTP CONNECT。
		s = "http://" + s
	}
	u, err := url.Parse(s)
	if err != nil {
		return nil, fmt.Errorf("proxy: %w", err)
	}
	switch u.Scheme {
	case "http", "https", "socks5", "socks5h":
	default:
		return nil, fmt.Errorf("%w: %q", ErrUnsupportedScheme, u.Scheme)
	}
	if u.Host == "" || u.Port() == "" {
		return nil, fmt.Errorf("proxy: 缺少 host:port（%q）", raw)
	}
	return u, nil
}

// Probe 验证代理能否把 target（host:port）穿透出去：发 HTTP CONNECT，看是否 2xx。
//
// 只实现 http / https（CONNECT）。socks5 与不支持的 scheme 返回 ErrUnsupportedScheme，
// 调用方应当把它当成「未知」而不是「可用」——线上宁可先走全局出口，也不要凭猜测换道。
func Probe(ctx context.Context, raw, target string) error {
	u, err := Parse(raw)
	if err != nil {
		return err
	}
	switch u.Scheme {
	case "http", "https":
	case "socks5", "socks5h":
		return ErrUnsupportedScheme
	}

	d := net.Dialer{Timeout: 5 * time.Second}
	conn, err := d.DialContext(ctx, "tcp", u.Host)
	if err != nil {
		return fmt.Errorf("proxy: 连不上出口: %w", err)
	}
	defer conn.Close()
	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(5 * time.Second)
	}
	_ = conn.SetDeadline(deadline)

	if u.Scheme == "https" {
		host := u.Hostname()
		tc := tls.Client(conn, &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12})
		if err := tc.HandshakeContext(ctx); err != nil {
			return fmt.Errorf("proxy: TLS 握手失败: %w", err)
		}
		// 后续读写都走 TLS 连接；Close 由外层的 conn 负责（tc 只是包装）。
		conn = tc
	}

	var req strings.Builder
	req.WriteString("CONNECT " + target + " HTTP/1.1\r\nHost: " + target + "\r\n")
	if u.User != nil {
		pw, _ := u.User.Password()
		token := base64.StdEncoding.EncodeToString([]byte(u.User.Username() + ":" + pw))
		req.WriteString("Proxy-Authorization: Basic " + token + "\r\n")
	}
	req.WriteString("\r\n")
	if _, err := conn.Write([]byte(req.String())); err != nil {
		return fmt.Errorf("proxy: 写 CONNECT 失败: %w", err)
	}

	br := bufio.NewReader(conn)
	line, err := br.ReadString('\n')
	if err != nil {
		return fmt.Errorf("proxy: 读 CONNECT 响应失败: %w", err)
	}
	fields := strings.Fields(strings.TrimSpace(line))
	if len(fields) < 2 || !strings.HasPrefix(fields[1], "2") {
		return fmt.Errorf("proxy: CONNECT 被拒（%s）", strings.TrimSpace(line))
	}
	return nil
}
