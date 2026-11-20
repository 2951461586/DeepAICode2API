package httpx

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// --- ETag 协商 ---

// TestETagMatches 覆盖强/弱校验、列表与通配符。
func TestETagMatches(t *testing.T) {
	cases := []struct {
		header string
		etag   string
		want   bool
	}{
		{"", `"abc"`, false},
		{`"abc"`, `"abc"`, true},
		{`"xyz", "abc"`, `"abc"`, true},
		{`"xyz"`, `"abc"`, false},
		{"*", `"abc"`, true},
		// 弱校验前缀不影响匹配（弱比较）。
		{`W/"abc"`, `"abc"`, true},
		{`"abc"`, `W/"abc"`, true},
		{`  "abc"  `, `"abc"`, true},
	}
	for _, c := range cases {
		if got := ETagMatches(c.header, c.etag); got != c.want {
			t.Fatalf("ETagMatches(%q,%q) = %v，期望 %v", c.header, c.etag, got, c.want)
		}
	}
}

// --- 入站体读取 ---

// TestReadBodyWithLimitAllowsNormalBody 验证正常体积的请求体被读全。
func TestReadBodyWithLimitAllowsNormalBody(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"a":1}`))
	w := httptest.NewRecorder()
	raw, err := ReadBodyWithLimit(w, r)
	if err != nil {
		t.Fatalf("读取: %v", err)
	}
	if string(raw) != `{"a":1}` {
		t.Fatalf("请求体 = %q", raw)
	}
}

// TestReadBodyWithLimitRejectsOversize 验证超过上限时报错而不是无限读。
func TestReadBodyWithLimitRejectsOversize(t *testing.T) {
	big := strings.Repeat("x", MaxRequestBody+1)
	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(big))
	w := httptest.NewRecorder()
	if _, err := ReadBodyWithLimit(w, r); err == nil {
		t.Fatal("超过上限应报错")
	}
}

// TestDecodeOptionalJSON 验证「空体合法、畸形体报错」。
//
// 静默把畸形 JSON 当空请求会让调用方以为参数已生效（历史 bug：POST
// /admin/api/keys 发错字段时建出一把空名字的密钥仍回 200）。
func TestDecodeOptionalJSON(t *testing.T) {
	t.Run("空体合法", func(t *testing.T) {
		var dst struct {
			Name string `json:"name"`
		}
		r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(""))
		w := httptest.NewRecorder()
		if !DecodeOptionalJSON(w, r, &dst) {
			t.Fatalf("空体应被接受，实际 %d %q", w.Code, w.Body.String())
		}
	})
	t.Run("畸形体 400", func(t *testing.T) {
		var dst struct {
			Name string `json:"name"`
		}
		r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("{"))
		w := httptest.NewRecorder()
		if DecodeOptionalJSON(w, r, &dst) {
			t.Fatal("畸形 JSON 应被拒绝")
		}
		if w.Code != http.StatusBadRequest {
			t.Fatalf("状态码 = %d，期望 400", w.Code)
		}
	})
	t.Run("正常体", func(t *testing.T) {
		var dst struct {
			Name string `json:"name"`
		}
		r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"name":"k"}`))
		w := httptest.NewRecorder()
		if !DecodeOptionalJSON(w, r, &dst) {
			t.Fatalf("正常体应被接受: %q", w.Body.String())
		}
		if dst.Name != "k" {
			t.Fatalf("name = %q", dst.Name)
		}
	})
}

// TestDecodeJSONRequestUsesInjectedErrorEnvelope 验证协议差异由回调注入。
//
// 三个推理入口的错误信封不同（OpenAI / Anthropic），所以失败分支必须可注入。
func TestDecodeJSONRequestUsesInjectedErrorEnvelope(t *testing.T) {
	t.Run("成功", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"model":"m"}`))
		w := httptest.NewRecorder()
		got, ok := DecodeJSONRequest[map[string]any](w, r, func(int, string) {})
		if !ok {
			t.Fatalf("应解析成功: %q", w.Body.String())
		}
		if got["model"] != "m" {
			t.Fatalf("model = %v", got["model"])
		}
	})
	t.Run("失败走注入的回调", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("{"))
		w := httptest.NewRecorder()
		var (
			calledCode int
			calledMsg  string
		)
		if _, ok := DecodeJSONRequest[map[string]any](w, r, func(code int, msg string) {
			calledCode, calledMsg = code, msg
			w.WriteHeader(code)
		}); ok {
			t.Fatal("畸形 JSON 应解析失败")
		}
		if calledCode != http.StatusBadRequest || calledMsg == "" {
			t.Fatalf("回调参数 = (%d,%q)，期望 (400,非空)", calledCode, calledMsg)
		}
	})
}
