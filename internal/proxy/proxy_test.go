package proxy

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseProxyList(t *testing.T) {
	raw := "http://a:1\n#c\nhttp://b:2, http://a:1; http://c:3\n"
	got := ParseProxyList(raw)
	want := []string{"http://a:1", "http://b:2", "http://c:3"}
	if len(got) != len(want) {
		t.Fatalf("ParseProxyList 长度 = %d, want %d (%v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestNewProxyPoolRejectsBadScheme(t *testing.T) {
	if _, err := NewProxyPool("ftp://a:1"); err == nil {
		t.Fatal("不支持的协议应报错")
	}
	if _, err := NewProxyPool("http://a:1"); err != nil {
		t.Fatalf("合法代理被拒: %v", err)
	}
	if p, err := NewProxyPool(""); err != nil || p != nil {
		t.Fatalf("空配置应返回 (nil, nil)，实际 (%v, %v)", p, err)
	}
}

func TestProxyPoolNextRoundRobin(t *testing.T) {
	p, err := NewProxyPool("http://a:1,http://b:2")
	if err != nil {
		t.Fatal(err)
	}
	first := p.Next()
	second := p.Next()
	third := p.Next()
	if first == second {
		t.Fatalf("轮询应换出口，实际两次都是 %q", first)
	}
	if third != first {
		t.Fatalf("两出口应回到第一个，实际 %q vs %q", third, first)
	}
	var nilPool *ProxyPool
	if got := nilPool.Next(); got != "" {
		t.Fatalf("nil 池应返回空串，实际 %q", got)
	}
	if nilPool.Len() != 0 {
		t.Fatal("nil 池长度应为 0")
	}
}

func TestProxyLabelHidesCredentials(t *testing.T) {
	if got := ProxyLabel("http://user:pass@eu.example:10001"); got != "eu.example:10001" {
		t.Fatalf("ProxyLabel = %q", got)
	}
	if got := ProxyLabel(""); got != "-" {
		t.Fatalf("空代理标签 = %q", got)
	}
	if got := ProxyLabel("not a url"); got != "invalid" {
		t.Fatalf("非法代理标签 = %q", got)
	}
}

func TestReadProxyFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "proxies.txt")
	if err := os.WriteFile(path, []byte("http://a:1\nhttp://b:2\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	raw, err := ReadProxyFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := len(ParseProxyList(raw)); got != 2 {
		t.Fatalf("文件代理数 = %d, want 2", got)
	}
}
