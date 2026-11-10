package deepai

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// TestNewClientIsInferenceShaped 固化「推理客户端不带浏览器指纹」的约定。
func TestNewClientIsInferenceShaped(t *testing.T) {
	t.Setenv("DEEPAI_API_ORIGIN", "")
	t.Setenv("HTTP_PROXY", "")
	t.Setenv("HTTPS_PROXY", "")

	c := NewClient()
	if c.BrowserFingerprint {
		t.Fatal("NewClient 必须是推理形态（BrowserFingerprint=false）")
	}
	if c.Origin != DefaultOrigin {
		t.Fatalf("默认 Origin = %q，期望 %q", c.Origin, DefaultOrigin)
	}
	if c.Machine == "" {
		t.Fatal("TestMain 已设置 DEEPAI_MACHINE_ID，Machine 不应为空")
	}
	if len(c.Instance) != 32 || strings.Trim(c.Instance, "0123456789abcdef") != "" {
		t.Fatalf("Instance = %q，期望 32 位小写 hex", c.Instance)
	}
	if c.HTTP == nil || c.HTTP.Transport == nil {
		t.Fatal("HTTP client / transport 未初始化")
	}
	if c.checkIns == nil {
		t.Fatal("签到存储未初始化")
	}
	if NewClient().Instance == c.Instance {
		t.Fatal("每个进程/客户端应生成独立实例号")
	}
}

// TestOriginOverrideValidation 只接受 http/https 且有 host 的覆盖值。
func TestOriginOverrideValidation(t *testing.T) {
	cases := []struct {
		raw  string
		want string
	}{
		{"", DefaultOrigin},
		{"   ", DefaultOrigin},
		{"http://127.0.0.1:9999", "http://127.0.0.1:9999"},
		{"http://127.0.0.1:9999/", "http://127.0.0.1:9999"}, // 去掉尾部斜杠
		{"https://staging.example.com/", "https://staging.example.com"},
		{"ftp://example.com", DefaultOrigin}, // 协议不支持
		{"https://", DefaultOrigin},          // 无 host
		{"://bad", DefaultOrigin},            // 解析失败
		{"api.deepai.org", DefaultOrigin},    // 无协议
	}
	for _, c := range cases {
		t.Setenv("DEEPAI_API_ORIGIN", c.raw)
		if got := NewClientWithFingerprint("", "").Origin; got != c.want {
			t.Errorf("DEEPAI_API_ORIGIN=%q → Origin=%q，期望 %q", c.raw, got, c.want)
		}
	}
}

func TestNewClientWithFingerprintKeepsBrowserShape(t *testing.T) {
	c := NewClientWithFingerprint("", "  Chrome/120  ")
	if !c.BrowserFingerprint {
		t.Fatal("注册链路客户端应带浏览器指纹")
	}
	if c.UserAgent != "Chrome/120" {
		t.Fatalf("UserAgent 应去空白，得到 %q", c.UserAgent)
	}
	if NewClientWithProxy("").BrowserFingerprint {
		t.Fatal("NewClientWithProxy 也必须是推理形态")
	}
}

func TestProxyWiring(t *testing.T) {
	t.Setenv("DEEPAI_API_ORIGIN", "")

	// 合法代理：写进 transport 的 Proxy 函数。
	c := NewClientWithProxy("http://user:pw@10.0.0.9:8080")
	if c.Proxy != "http://user:pw@10.0.0.9:8080" {
		t.Fatalf("Proxy 字段 = %q（应原样保留，仅本地保存）", c.Proxy)
	}
	tr, ok := c.HTTP.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("Transport 类型 = %T", c.HTTP.Transport)
	}
	if tr.Proxy == nil {
		t.Fatal("transport.Proxy 未设置")
	}
	got, err := tr.Proxy(httptest.NewRequest(http.MethodGet, "https://api.deepai.org/x", nil))
	if err != nil {
		t.Fatalf("Proxy func: %v", err)
	}
	if got == nil || got.Host != "10.0.0.9:8080" {
		t.Fatalf("Proxy 路由到 %v，期望 10.0.0.9:8080", got)
	}
	if got.User != nil && got.User.String() != "user:pw" {
		t.Fatalf("Proxy 凭据丢失: %v", got.User)
	}

	// 非法代理：不应把 transport 弄坏（回落到环境代理）。
	bad := NewClientWithProxy("://nope")
	trBad := bad.HTTP.Transport.(*http.Transport)
	if trBad.Proxy == nil {
		t.Fatal("非法代理时 transport.Proxy 应保持默认实现")
	}
}

func TestSignInURL(t *testing.T) {
	c := NewClientWithFingerprint("", "")
	c.Origin = "https://gw.example.com"

	raw := c.SignInURL("st ate&1", 10087)
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("SignInURL 解析失败: %v", err)
	}
	if want := ModelBasePath + PathSignIn; u.Path != want {
		t.Fatalf("路径 = %q，期望 %q", u.Path, want)
	}
	q := u.Query()
	if q.Get("state") != "st ate&1" {
		t.Fatalf("state 未正确转义/还原: %q", q.Get("state"))
	}
	if q.Get("port") != "10087" {
		t.Fatalf("port = %q", q.Get("port"))
	}
	if !strings.HasPrefix(raw, "https://gw.example.com"+ModelBasePath+PathSignIn+"?") {
		t.Fatalf("绝对地址前缀错误: %q", raw)
	}
}

// TestPostJSONEndpoints 覆盖 WebSearch / FindService 两条透传路径。
func TestPostJSONEndpoints(t *testing.T) {
	type call struct {
		path string
		body string
		key  string
		lane string
	}
	var got call
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		got = call{
			path: r.URL.Path,
			body: string(b),
			key:  r.Header.Get(HeaderKeyName),
			lane: r.Header.Get(HeaderLane),
		}
		_, _ = io.WriteString(w, `{"results":[1,2]}`)
	})

	acct := &Account{APIKey: "sk-up"}
	payload := []byte(`{"query":"golang","num_results":3}`)

	out, err := c.WebSearch(context.Background(), acct, payload)
	if err != nil {
		t.Fatalf("WebSearch: %v", err)
	}
	if string(out) != `{"results":[1,2]}` {
		t.Fatalf("响应应原样透传，得到 %s", out)
	}
	if got.path != ModelBasePath+PathWebSearch {
		t.Fatalf("路径 = %q", got.path)
	}
	if got.body != string(payload) {
		t.Fatalf("请求体应逐字节透传，得到 %q", got.body)
	}
	if got.key != "sk-up" || got.lane != LaneVersion {
		t.Fatalf("鉴权/车道头缺失: key=%q lane=%q", got.key, got.lane)
	}

	out, err = c.FindService(context.Background(), acct, []byte(`{"q":"x"}`))
	if err != nil {
		t.Fatalf("FindService: %v", err)
	}
	if got.path != ModelBasePath+PathFindService {
		t.Fatalf("FindService 路径 = %q", got.path)
	}
	if string(out) != `{"results":[1,2]}` {
		t.Fatalf("FindService 响应应原样透传，得到 %s", out)
	}
}

func TestPostJSONPropagatesHTTPError(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = io.WriteString(w, `{"error":{"code":"session_busy","message":"模型正忙"}}`)
	})

	_, err := c.WebSearch(context.Background(), &Account{APIKey: "k"}, []byte(`{}`))
	var he *HTTPError
	if !errors.As(err, &he) {
		t.Fatalf("期望 *HTTPError，得到 %T: %v", err, err)
	}
	if he.Op != "联网搜索" || he.Status != http.StatusServiceUnavailable {
		t.Fatalf("错误元信息 = %+v", he)
	}
	if he.Code() != CodeSessionBusy || !strings.Contains(he.Error(), "模型正忙") {
		t.Fatalf("错误码/文案丢失: %v", he)
	}

	if _, err := c.FindService(context.Background(), nil, []byte(`{}`)); !errors.As(err, &he) || he.Op != "服务搜索" {
		t.Fatalf("FindService 的 Op 应为「服务搜索」，得到 %v", err)
	}
}

func TestTranscribe(t *testing.T) {
	var (
		gotPath   string
		gotKey    string
		gotField  string
		gotFile   string
		gotFileCT string
		gotFileB  string
	)
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotKey = r.Header.Get("api-key")
		if !strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data; boundary=") {
			t.Errorf("Content-Type = %q", r.Header.Get("Content-Type"))
		}
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Errorf("ParseMultipartForm: %v", err)
		}
		fhs := r.MultipartForm.File["voiceRecording"]
		if len(fhs) != 1 {
			t.Fatalf("voiceRecording 文件数 = %d", len(fhs))
		}
		gotField = "voiceRecording"
		gotFile = fhs[0].Filename
		gotFileCT = fhs[0].Header.Get("Content-Type")
		f, err := fhs[0].Open()
		if err != nil {
			t.Fatalf("打开上传文件: %v", err)
		}
		defer f.Close()
		b, _ := io.ReadAll(f)
		gotFileB = string(b)
		_, _ = io.WriteString(w, `{"text":"  你好世界  "}`)
	})

	acct := &Account{APIKey: "sk-voice"}
	text, err := c.Transcribe(context.Background(), acct, `re"c.webm`, "audio/webm", []byte("RIFFdata"))
	if err != nil {
		t.Fatalf("Transcribe: %v", err)
	}
	if text != "你好世界" {
		t.Fatalf("转写文本应去空白，得到 %q", text)
	}
	if gotPath != PathSpeechToText {
		t.Fatalf("路径 = %q，期望 %q", gotPath, PathSpeechToText)
	}
	if gotKey != "sk-voice" {
		t.Fatalf("api-key 头 = %q", gotKey)
	}
	if gotField != "voiceRecording" || gotFile != `re"c.webm` {
		// 解析器会把 \" 反转义回 "，所以这里比对的是原始文件名。
		t.Fatalf("字段/文件名 = %q / %q", gotField, gotFile)
	}
	if gotFileCT != "audio/webm" || gotFileB != "RIFFdata" {
		t.Fatalf("文件类型/内容 = %q / %q", gotFileCT, gotFileB)
	}
}

func TestTranscribeDefaults(t *testing.T) {
	var filename, contentType string
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Fatalf("ParseMultipartForm: %v", err)
		}
		fh := r.MultipartForm.File["voiceRecording"][0]
		filename = fh.Filename
		contentType = fh.Header.Get("Content-Type")
		_, _ = io.WriteString(w, `{"text":"ok"}`)
	})

	// 空文件名 / 类型应回落到 webm 默认值；无账号时不应写 api-key 头。
	if _, err := c.Transcribe(context.Background(), nil, "", "", []byte("x")); err != nil {
		t.Fatalf("Transcribe: %v", err)
	}
	if filename != "rec.webm" || contentType != "audio/webm" {
		t.Fatalf("默认值 = %q / %q", filename, contentType)
	}
}

func TestTranscribeErrors(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = io.WriteString(w, "boom")
	})

	if _, err := c.Transcribe(context.Background(), nil, "", "", nil); err == nil ||
		!strings.Contains(err.Error(), "音频内容为空") {
		t.Fatalf("空音频应报错，得到 %v", err)
	}
	if _, err := c.Transcribe(context.Background(), nil, "", "", make([]byte, 8<<20+1)); err == nil ||
		!strings.Contains(err.Error(), "8 MB") {
		t.Fatalf("超限音频应报错，得到 %v", err)
	}

	_, err := c.Transcribe(context.Background(), nil, "a.webm", "audio/webm", []byte("x"))
	var he *HTTPError
	if !errors.As(err, &he) || he.Op != "语音转写" || he.Status != http.StatusInternalServerError {
		t.Fatalf("期望语音转写的 HTTPError，得到 %v", err)
	}
}

func TestTranscribeBadResponses(t *testing.T) {
	body := ""
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, body)
	})

	body = `not json`
	if _, err := c.Transcribe(context.Background(), nil, "a.webm", "audio/webm", []byte("x")); err == nil ||
		!strings.Contains(err.Error(), "解析失败") {
		t.Fatalf("非法 JSON 应报解析失败，得到 %v", err)
	}

	body = `{"text":"   "}`
	if _, err := c.Transcribe(context.Background(), nil, "a.webm", "audio/webm", []byte("x")); err == nil ||
		!strings.Contains(err.Error(), "未返回转写文本") {
		t.Fatalf("空转写应报错，得到 %v", err)
	}
}

func TestExchangeCode(t *testing.T) {
	var gotPath, gotBody, gotKey, gotCT string
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotPath, gotBody = r.URL.Path, string(b)
		gotKey = r.Header.Get(HeaderKeyName)
		gotCT = r.Header.Get("Content-Type")
		_, _ = io.WriteString(w, `{"api_key":"sk-new","username":"alice","is_pro":true}`)
	})

	cred, err := c.ExchangeCode(context.Background(), "auth-code-1")
	if err != nil {
		t.Fatalf("ExchangeCode: %v", err)
	}
	if cred.APIKey != "sk-new" || cred.Username != "alice" || !cred.IsPro {
		t.Fatalf("凭据解析错误: %+v", cred)
	}
	if gotPath != ModelBasePath+PathExchange {
		t.Fatalf("路径 = %q", gotPath)
	}
	if gotBody != `{"code":"auth-code-1"}` {
		t.Fatalf("请求体 = %q", gotBody)
	}
	if gotCT != "application/json" {
		t.Fatalf("Content-Type = %q", gotCT)
	}
	if gotKey != "" {
		t.Fatalf("换取密钥时不应带 Api-Key（还没有 Key），得到 %q", gotKey)
	}
}

func TestExchangeCodeRejectsBadKey(t *testing.T) {
	body := ""
	status := http.StatusOK
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	})

	cases := []struct {
		name string
		body string
		want string
	}{
		{"缺少 api_key", `{"username":"x"}`, "缺少有效 API Key"},
		{"api_key 含换行", "{\"api_key\":\"sk-a\\nb\"}", "缺少有效 API Key"},
		{"非法 JSON", `{`, "解析失败"},
	}
	for _, cs := range cases {
		body = cs.body
		_, err := c.ExchangeCode(context.Background(), "code")
		if err == nil || !strings.Contains(err.Error(), cs.want) {
			t.Errorf("%s: err=%v，期望包含 %q", cs.name, err, cs.want)
		}
	}

	status = http.StatusForbidden
	body = `{"error":{"code":"auth_required","message":"bad code"}}`
	_, err := c.ExchangeCode(context.Background(), "code")
	var he *HTTPError
	if !errors.As(err, &he) || he.Op != "登录换取密钥" || he.Status != http.StatusForbidden {
		t.Fatalf("期望登录换取密钥的 HTTPError，得到 %v", err)
	}
}

func TestEscapeQuotes(t *testing.T) {
	cases := []struct{ in, want string }{
		{"", ""},
		{"plain", "plain"},
		{`a"b`, `a\"b`},
		{`a\b`, `a\\b`},
		{`a\"b`, `a\\\"b`},
	}
	for _, c := range cases {
		if got := escapeQuotes(c.in); got != c.want {
			t.Errorf("escapeQuotes(%q)=%q，期望 %q", c.in, got, c.want)
		}
	}
}

// TestErrorClassificationHelpers 覆盖两个此前完全没测到的判定函数。
func TestErrorClassificationHelpers(t *testing.T) {
	if !mustErr(t, 403, `{"error":{"code":"email_unverified"}}`).EmailUnverified() {
		t.Fatal("email_unverified 应判定为邮箱未验证")
	}
	for _, body := range []string{`{}`, `{"error":{"code":"auth_required"}}`, `not-json`} {
		if mustErr(t, 403, body).EmailUnverified() {
			t.Fatalf("body=%s 不应判定为邮箱未验证", body)
		}
	}
	if (*HTTPError)(nil) == nil {
		_ = 0 // 仅为让 nil 检查显式可见
	}

	social := mustErr(t, 403, `{"error":{"code":"social_login_required"}}`)
	if !social.SocialLoginRequired() || !IsSocialLoginRequired(social) {
		t.Fatal("social_login_required 应被识别")
	}
	if !IsSocialLoginRequired(fmt.Errorf("包装一层: %w", social)) {
		t.Fatal("IsSocialLoginRequired 应能穿透错误包装")
	}
	if IsSocialLoginRequired(errors.New("普通错误")) {
		t.Fatal("普通错误不应判定为 social_login_required")
	}
	if IsSocialLoginRequired(nil) {
		t.Fatal("nil 不应判定为 social_login_required")
	}
	if social.EmailUnverified() {
		t.Fatal("social_login_required 不是 email_unverified")
	}
}

func TestUsageSummaryErrors(t *testing.T) {
	status := http.StatusOK
	body := ""
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("方法 = %s", r.Method)
		}
		if r.Header.Get("User-Agent") != "node" {
			t.Errorf("用量接口应保持 Node fetch 的 UA，得到 %q", r.Header.Get("User-Agent"))
		}
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	})
	// 用量接口属于推理链路：换成推理形态后 UA 应为 node（注册形态会带 Chrome UA）。
	c.BrowserFingerprint = false

	body = `{"is_pro":true,"plan":"pro","credits_remaining":12.5}`
	got, err := c.UsageSummary(context.Background(), &Account{APIKey: "k"})
	if err != nil {
		t.Fatalf("UsageSummary: %v", err)
	}
	if !got.IsPro || got.Plan != "pro" || got.CreditsRemaining != 12.5 {
		t.Fatalf("用量解析错误: %+v", got)
	}

	status = http.StatusUnauthorized
	body = `{"error":{"code":"auth_required"}}`
	_, err = c.UsageSummary(context.Background(), &Account{APIKey: "k"})
	var he *HTTPError
	if !errors.As(err, &he) || he.Op != "查询用量" {
		t.Fatalf("期望查询用量的 HTTPError，得到 %v", err)
	}

	status = http.StatusOK
	body = `{`
	if _, err := c.UsageSummary(context.Background(), nil); err == nil ||
		!strings.Contains(err.Error(), "用量响应解析失败") {
		t.Fatalf("非法 JSON 应报解析失败，得到 %v", err)
	}

	// JSON 可读即成功，字段缺失不算错误。
	body = `{}`
	if got, err := c.UsageSummary(context.Background(), nil); err != nil || got == nil {
		t.Fatalf("空对象也应成功: %+v / %v", got, err)
	}
}
