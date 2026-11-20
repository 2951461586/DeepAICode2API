package guard

import (
	"net/http/httptest"
	"testing"
)

// TestOriginLoopbackDefaults 验证默认策略：只回显本机来源。
func TestOriginLoopbackDefaults(t *testing.T) {
	cases := []struct{ raw, want string }{
		{"http://localhost:5173", "http://localhost:5173"},
		{"http://127.0.0.1:10087", "http://127.0.0.1:10087"},
		{"http://[::1]:8080", "http://[::1]:8080"},
		{"https://evil.example.com", ""},
		{"null", ""},
		{"", ""},
		{"file:///etc/passwd", ""},
		{"http://localhost.evil.com", ""},
	}
	for _, c := range cases {
		if got := Origin(c.raw, ""); got != c.want {
			t.Fatalf("Origin(%q) = %q，期望 %q", c.raw, got, c.want)
		}
	}
}

// TestOriginAllowList 验证显式列表：精确匹配才放行，通配符条目一律无效。
func TestOriginAllowList(t *testing.T) {
	const cfg = " https://app.example.com , https://*.evil.com "
	if got := Origin("https://app.example.com", cfg); got != "https://app.example.com" {
		t.Fatalf("列表内来源应放行，实际 %q", got)
	}
	if got := Origin("https://other.example.com", cfg); got != "" {
		t.Fatalf("列表外来源应拒绝，实际 %q", got)
	}
	// 即使来源是本机，配置了列表就以列表为准。
	if got := Origin("http://localhost:5173", cfg); got != "" {
		t.Fatalf("配置列表后本机来源也应走列表，实际 %q", got)
	}
	if got := Origin("https://x.evil.com", cfg); got != "" {
		t.Fatalf("含通配符的条目不应生效，实际 %q", got)
	}
}

// TestInAllowList 验证列表判定的边界（空条目、大小写、通配符）。
func TestInAllowList(t *testing.T) {
	if !InAllowList("https://A.example.com", " , https://a.example.com ") {
		t.Fatal("应大小写不敏感地命中")
	}
	if InAllowList("https://a.example.com", "") {
		t.Fatal("空列表不应命中")
	}
	if InAllowList("https://a.example.com", "*") {
		t.Fatal("通配符条目不应命中")
	}
}

// TestWriteHeaders 验证跨域头只在放行时写入，且带 Vary: Origin。
func TestWriteHeaders(t *testing.T) {
	w := httptest.NewRecorder()
	WriteHeaders(w, "https://app.example.com")
	h := w.Header()
	if got := h.Get("Access-Control-Allow-Origin"); got != "https://app.example.com" {
		t.Fatalf("Allow-Origin = %q", got)
	}
	if got := h.Get("Vary"); got != "Origin" {
		t.Fatalf("Vary = %q，期望 Origin", got)
	}
	if got := h.Get("Access-Control-Expose-Headers"); got == "" {
		t.Fatal("应声明 Expose-Headers（X-Request-Id / Retry-After）")
	}

	// 未放行时一个头都不写。
	w2 := httptest.NewRecorder()
	WriteHeaders(w2, "")
	if got := w2.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("未放行时不应写 Allow-Origin，实际 %q", got)
	}
}

// TestIsLoopbackAddr 验证来源判定。
func TestIsLoopbackAddr(t *testing.T) {
	cases := map[string]bool{
		"127.0.0.1:34567": true,
		"[::1]:8080":      true,
		"10.0.0.9:1234":   false,
		"192.168.1.5:80":  false,
		"garbage":         false,
	}
	for in, want := range cases {
		if got := IsLoopbackAddr(in); got != want {
			t.Fatalf("IsLoopbackAddr(%q) = %v，期望 %v", in, got, want)
		}
	}
}
