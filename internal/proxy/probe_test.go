package proxy

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"
)

// TestParseNormalizesScheme 固化「缺 scheme 时补 http://」这条线上事故的修复：
// 账号里存的是 `host:port`，url.Parse 会把主机名当 scheme，导致代理被静默忽略。
func TestParseNormalizesScheme(t *testing.T) {
	cases := []struct {
		raw      string
		wantHost string
		wantErr  bool
	}{
		{"06gvja.example.com:7161", "06gvja.example.com:7161", false},
		{"http://u:p@10.0.0.1:8080", "10.0.0.1:8080", false},
		{"https://proxy.example:443", "proxy.example:443", false},
		{"socks5://127.0.0.1:1080", "127.0.0.1:1080", false},
		{"", "", true},
		{"   ", "", true},
		{"example.com", "", true},   // 没有端口
		{"ftp://host:21", "", true}, // scheme 不支持
	}
	for _, c := range cases {
		u, err := Parse(c.raw)
		if c.wantErr {
			if err == nil {
				t.Fatalf("%q 应当解析失败，实际 %+v", c.raw, u)
			}
			continue
		}
		if err != nil {
			t.Fatalf("%q 解析失败: %v", c.raw, err)
		}
		if u.Host != c.wantHost {
			t.Fatalf("%q → host=%q，期望 %q", c.raw, u.Host, c.wantHost)
		}
	}
}

// fakeConnectProxy 起一个只会应答 CONNECT 的假代理。
func fakeConnectProxy(t *testing.T, status, wantAuth string) (addr string, gotAuth chan string) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	gotAuth = make(chan string, 4)
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				br := bufio.NewReader(c)
				var auth string
				for {
					line, err := br.ReadString('\n')
					if err != nil {
						return
					}
					if strings.HasPrefix(strings.ToLower(line), "proxy-authorization:") {
						auth = strings.TrimSpace(strings.SplitN(line, ":", 2)[1])
					}
					if line == "\r\n" {
						break
					}
				}
				select {
				case gotAuth <- auth:
				default:
				}
				_, _ = fmt.Fprintf(c, "HTTP/1.1 %s\r\n\r\n", status)
			}(conn)
		}
	}()
	return ln.Addr().String(), gotAuth
}

// TestProbeAcceptsConnect200 固化「2xx 即认为出口可用」。
func TestProbeAcceptsConnect200(t *testing.T) {
	addr, _ := fakeConnectProxy(t, "200 Connection established", "")
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := Probe(ctx, addr, "api.deepai.org:443"); err != nil {
		t.Fatalf("探测应当成功: %v", err)
	}
}

// TestProbeSendsBasicAuth 固化「代理带凭据时发 Proxy-Authorization」：
// 住宅代理几乎都要鉴权，不发就只能看到 407。
func TestProbeSendsBasicAuth(t *testing.T) {
	addr, gotAuth := fakeConnectProxy(t, "200 OK", "")
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := Probe(ctx, "http://user:pass@"+addr, "api.deepai.org:443"); err != nil {
		t.Fatalf("探测应当成功: %v", err)
	}
	select {
	case auth := <-gotAuth:
		if auth != "Basic dXNlcjpwYXNz" { // base64("user:pass")
			t.Fatalf("Proxy-Authorization = %q", auth)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("假代理没有收到 Proxy-Authorization")
	}
}

// TestProbeRejectsNon2xx 固化「CONNECT 被拒即不可用」，并区分 socks5（未知）。
func TestProbeRejectsNon2xx(t *testing.T) {
	addr, _ := fakeConnectProxy(t, "407 Proxy Authentication Required", "")
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := Probe(ctx, addr, "api.deepai.org:443"); err == nil {
		t.Fatal("407 时探测应当失败")
	}

	if err := Probe(ctx, "socks5://127.0.0.1:1080", "api.deepai.org:443"); !errors.Is(err, ErrUnsupportedScheme) {
		t.Fatalf("socks5 应返回 ErrUnsupportedScheme，实际 %v", err)
	}
}
