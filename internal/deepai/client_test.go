package deepai

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestMain 固定机器指纹，避免每个测试都去读 SMBIOS（Windows 下要起 powershell）。
// MachineID 带 sync.Once，必须在任何 Client 构造前设置。
func TestMain(m *testing.M) {
	os.Setenv("DEEPAI_MACHINE_ID", "11111111-2222-3333-4444-555555555555")
	os.Exit(m.Run())
}

func mustErr(t *testing.T, status int, body string) *HTTPError {
	t.Helper()
	return &HTTPError{Op: "上游推理", Status: status, Body: []byte(body)}
}

func TestHTTPErrorRetryable(t *testing.T) {
	cases := []struct {
		status int
		want   bool
		why    string
	}{
		{400, false, "请求本身的问题"},
		{404, false, "模型不存在，重试无意义"},
		{422, false, "参数非法"},
		{401, true, "鉴权失败可换号"},
		{403, true, ""},
		{409, true, "容量拒绝"},
		{429, true, "限流"},
		{500, true, ""},
		{502, true, ""},
		{503, true, ""},
		{504, true, ""},
		{418, false, "未知 4xx 不重试"},
	}
	for _, c := range cases {
		if got := mustErr(t, c.status, "{}").Retryable(); got != c.want {
			t.Errorf("status=%d Retryable()=%v，期望 %v（%s）", c.status, got, c.want, c.why)
		}
	}
}

func TestHTTPErrorAuthFailed(t *testing.T) {
	cases := []struct {
		status int
		body   string
		want   bool
	}{
		{401, `{}`, true},
		{403, `{"error":{"code":"auth_required"}}`, true},
		{403, `{"error":{"code":"invalid_api_key"}}`, true},
		{403, `{"error":{"code":"email_unverified"}}`, true},
		{403, `{"error":{"code":"session_busy"}}`, false},
		{500, `{}`, false},
	}
	for _, c := range cases {
		if got := mustErr(t, c.status, c.body).AuthFailed(); got != c.want {
			t.Errorf("status=%d body=%s AuthFailed()=%v，期望 %v", c.status, c.body, got, c.want)
		}
	}
}

func TestHTTPErrorCapacityWaiting(t *testing.T) {
	busy := mustErr(t, http.StatusConflict, `{"error":{"code":"session_busy"}}`)
	if !busy.CapacityWaiting() {
		t.Fatal("409 + session_busy 应判定为容量等待")
	}
	if !busy.Retryable() {
		t.Fatal("容量等待必须可重试")
	}

	conflict := mustErr(t, http.StatusConflict, `{"error":{"code":"duplicate"}}`)
	if conflict.CapacityWaiting() {
		t.Fatal("非 session_busy 的 409 不应判定为容量等待")
	}
	if got := mustErr(t, 429, `{"error":{"code":"session_busy"}}`).CapacityWaiting(); got {
		t.Fatal("非 409 即使 code=session_busy 也不算容量等待")
	}
}

func TestHTTPErrorCodeAndMessage(t *testing.T) {
	e := mustErr(t, 409, `{"error":{"message":"模型正忙","code":"session_busy"}}`)
	if e.Code() != "session_busy" {
		t.Fatalf("Code() = %q", e.Code())
	}
	if got := e.Error(); !strings.Contains(got, "模型正忙") {
		t.Fatalf("Error() 应优先使用上游 message，得到 %q", got)
	}

	if got := mustErr(t, 500, "not json").Code(); got != "" {
		t.Fatalf("非 JSON body 的 Code() 应为空，得到 %q", got)
	}
}

func TestHTTPErrorTruncatesLongBody(t *testing.T) {
	// 上游异常时会回吐 HTML / 堆栈，不能整段塞进错误串再写进日志。
	long := strings.Repeat("A", 1000)
	got := mustErr(t, 502, long).Error()
	if !strings.HasSuffix(got, "…") {
		t.Fatalf("长 body 应被截断并以省略号结尾: %q", got)
	}
	if strings.Count(got, "A") > 301 {
		t.Fatalf("截断长度不合理：%d 个 A", strings.Count(got, "A"))
	}

	if got := mustErr(t, 500, "   ").Error(); !strings.Contains(got, "HTTP 500") {
		t.Fatalf("空 body 应回退到状态码: %q", got)
	}
}

func TestRetryAfter(t *testing.T) {
	resp := func(v string) *http.Response {
		h := http.Header{}
		if v != "" {
			h.Set("Retry-After", v)
		}
		return &http.Response{Header: h}
	}
	fallback := 7 * time.Second

	cases := []struct {
		header string
		want   time.Duration
	}{
		{"", fallback},
		{"3", 3 * time.Second},
		{"1", time.Second},
		{"999", 30 * time.Second}, // 上限 30s，避免被上游拖死
		{"0", time.Second},        // 与桌面端一致：夹到下限 1s
		{"-5", time.Second},       // 负数同样夹到 1s
		{"abc", fallback},         // 非数字回退
		{"Wed, 21 Oct", fallback}, // HTTP-date 不支持，回退
	}
	for _, c := range cases {
		if got := RetryAfter(resp(c.header), fallback); got != c.want {
			t.Errorf("Retry-After=%q → %v，期望 %v", c.header, got, c.want)
		}
	}
}

// newTestClient 构造指向本地假上游的客户端。
func newTestClient(t *testing.T, h http.HandlerFunc) (*Client, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	c := NewClientWithFingerprint("", "test-ua/1.0")
	c.Origin = srv.URL
	c.Machine = "machine-fingerprint"
	return c, srv
}

func TestClientSendsRequiredHeaders(t *testing.T) {
	var got http.Header
	var gotPath string
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Clone()
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"ok":true}`)
	})

	acct := &Account{APIKey: "sk-upstream-key"}
	_, err := c.OpenModel(context.Background(), acct, PathChat, []byte(`{"model":"x"}`), "affinity-1")
	if err != nil {
		t.Fatalf("OpenModel: %v", err)
	}

	if gotPath != ModelBasePath+PathChat {
		t.Fatalf("请求路径 = %q，期望 %q", gotPath, ModelBasePath+PathChat)
	}
	if v := got.Get(HeaderKeyName); v != "sk-upstream-key" {
		t.Fatalf("%s = %q", HeaderKeyName, v)
	}
	if v := got.Get(HeaderLane); v != LaneVersion {
		t.Fatalf("%s = %q，期望 %q", HeaderLane, v, LaneVersion)
	}
	if LaneVersion != "4" {
		t.Fatalf("车道版本 = %q，0.3.19 客户端使用 4", LaneVersion)
	}
	if inst := got.Get(HeaderInstance); len(inst) != 32 || strings.Trim(inst, "0123456789abcdef") != "" {
		t.Fatalf("%s = %q，期望 32 位小写 hex", HeaderInstance, inst)
	}
	if v := got.Get(HeaderMachine); v != "machine-fingerprint" {
		t.Fatalf("%s = %q", HeaderMachine, v)
	}
	if v := got.Get(HeaderAffinity); v != "affinity-1" {
		t.Fatalf("%s = %q", HeaderAffinity, v)
	}
	if v := got.Get("User-Agent"); v != "test-ua/1.0" {
		t.Fatalf("User-Agent = %q", v)
	}
	if v := got.Get("Content-Type"); v != "application/json" {
		t.Fatalf("Content-Type = %q", v)
	}
}

func TestClientEmptyAffinityOmitsHeader(t *testing.T) {
	var got http.Header
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Clone()
		_, _ = io.WriteString(w, `{}`)
	})
	if _, err := c.OpenModel(context.Background(), &Account{APIKey: "k"}, PathChat, []byte(`{}`), ""); err != nil {
		t.Fatalf("OpenModel: %v", err)
	}
	if v, ok := got[HeaderAffinity]; ok {
		t.Fatalf("空 affinity 不应发送 %s，实际 %v", HeaderAffinity, v)
	}
}

func TestClientDoesNotFollowRedirects(t *testing.T) {
	// 上游对无效 Key 会 302 到登录页；跟随只会把鉴权失败伪装成 200。
	var finalHits int
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/login" {
			finalHits++
			_, _ = io.WriteString(w, "login page")
			return
		}
		http.Redirect(w, r, "/login", http.StatusFound)
	})

	resp, err := c.OpenModel(context.Background(), &Account{APIKey: "bad"}, PathChat, []byte(`{}`), "")
	if err != nil {
		t.Fatalf("OpenModel: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("状态码 = %d，期望 302（不跟随）", resp.StatusCode)
	}
	if finalHits != 0 {
		t.Fatal("不应跟随重定向到登录页")
	}
}

func TestFetchModels(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != ModelBasePath+PathModels {
			t.Errorf("路径 = %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"models":[{"id":"m1","name":"M1"},{"id":"m2","name":"M2"}],"default":"m2"}`)
	})

	models, def, err := c.FetchModels(context.Background())
	if err != nil {
		t.Fatalf("FetchModels: %v", err)
	}
	if len(models) != 2 || models[0].ID != "m1" {
		t.Fatalf("模型目录解析错误: %#v", models)
	}
	if def != "m2" {
		t.Fatalf("default = %q，期望 m2", def)
	}
	if len(models[0].Input) != 1 || models[0].Input[0] != "text" {
		t.Fatalf("缺失 input 应补为 [text]，实际 %#v", models[0].Input)
	}
}

func TestFetchModelsNormalizes(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		// 无 default、有重复 ID、有空 ID。
		_, _ = io.WriteString(w, `{"models":[{"id":"a"},{"id":"a"},{"id":""},{"id":"b","input":["text","image"]}]}`)
	})
	models, def, err := c.FetchModels(context.Background())
	if err != nil {
		t.Fatalf("FetchModels: %v", err)
	}
	if len(models) != 2 {
		t.Fatalf("去重后应为 2 个模型，实际 %d: %#v", len(models), models)
	}
	if def != "a" {
		t.Fatalf("无 default 时应回退到首个模型，实际 %q", def)
	}
	if len(models[1].Input) != 2 {
		t.Fatalf("已声明的 input 不应被覆盖: %#v", models[1].Input)
	}
}

func TestFetchModelsEmptyIsError(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"models":[]}`)
	})
	if _, _, err := c.FetchModels(context.Background()); err == nil {
		t.Fatal("空模型目录应返回错误")
	}
}

func TestTheosParsesAllowance(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"enabled":true,"balance":"1.50000","granted":"0.00000","lane_version":2}`)
	})
	al, err := c.Theos(context.Background(), &Account{APIKey: "k"})
	if err != nil {
		t.Fatalf("Theos: %v", err)
	}
	if !al.Enabled || al.Balance != "1.50000" || al.Granted != "0.00000" {
		t.Fatalf("配额解析错误: %#v", al)
	}
	if al.CheckedAt == 0 {
		t.Fatal("CheckedAt 应被填充")
	}
}

func TestTheosReturnsHTTPError(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"error":{"code":"invalid_api_key"}}`)
	})
	_, err := c.Theos(context.Background(), &Account{APIKey: "bad"})
	if err == nil {
		t.Fatal("401 应返回错误")
	}
	var httpErr *HTTPError
	if !asHTTPError(err, &httpErr) {
		t.Fatalf("错误类型 = %T，期望 *HTTPError", err)
	}
	if !httpErr.AuthFailed() {
		t.Fatal("应判定为鉴权失败")
	}
}

// asHTTPError 是 errors.As 的薄包装，避免测试文件重复 import errors 语义混淆。
func asHTTPError(err error, target **HTTPError) bool {
	e, ok := err.(*HTTPError)
	if ok {
		*target = e
	}
	return ok
}

func TestJSONBodyIsVerbatim(t *testing.T) {
	var got []byte
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		got, _ = io.ReadAll(r.Body)
		_, _ = io.WriteString(w, `{}`)
	})
	body := []byte(`{"model":"x","messages":[{"role":"user","content":"你好"}]}`)
	if _, err := c.OpenModel(context.Background(), &Account{APIKey: "k"}, PathChat, body, ""); err != nil {
		t.Fatalf("OpenModel: %v", err)
	}
	if string(got) != string(body) {
		t.Fatalf("请求体被改写:\n got=%s\nwant=%s", got, body)
	}
	var probe map[string]any
	if err := json.Unmarshal(got, &probe); err != nil {
		t.Fatalf("请求体不是合法 JSON: %v", err)
	}
}

// IsTransient 决定关键路径要不要退避重试，误判的代价是浪费一次重试
// 或把一个本该救回的账号判死。
func TestIsTransient(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"400 参数错误", mustErr(t, 400, `{"error":{"code":"invalid"}}`), false},
		{"401 鉴权失败", mustErr(t, 401, `{}`), false},
		{"403", mustErr(t, 403, `{}`), false},
		{"404", mustErr(t, 404, `{}`), false},
		{"429 限流", mustErr(t, 429, `{}`), true},
		{"408", mustErr(t, 408, `{}`), true},
		{"500", mustErr(t, 500, `{}`), true},
		{"503", mustErr(t, 503, `{}`), true},
		{"unexpected eof", errors.New(`Get "https://x": unexpected EOF`), true},
		{"no exit node", errors.New(`Get "https://x": No exit node`), true},
		{"tls handshake", errors.New("net/http: TLS handshake timeout"), true},
		{"connection reset", errors.New("read tcp: connection reset by peer"), true},
		{"调用方取消", context.Canceled, false},
		{"超时", context.DeadlineExceeded, true},
		{"未知错误", errors.New("something else"), false},
	}
	for _, c := range cases {
		if got := IsTransient(c.err); got != c.want {
			t.Errorf("%s: IsTransient=%v，期望 %v", c.name, got, c.want)
		}
	}
}

// 车道 4 的签到握手：/theos 下发 check_in，下一次请求回传 nonce 与心跳。
func TestTheosCheckInHandshake(t *testing.T) {
	var (
		mu     sync.Mutex
		header []http.Header
	)
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		header = append(header, r.Header.Clone())
		n := len(header)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if n == 1 {
			_, _ = io.WriteString(w, `{"enabled":true,"balance":"1","granted":"0","lane_version":4,`+
				`"check_in":{"nonce":"0123456789abcdef0123456789abcdef","seconds":600}}`)
			return
		}
		_, _ = io.WriteString(w, `{"enabled":true,"lane_version":4}`)
	})

	acct := &Account{APIKey: "k-handshake"}
	if _, err := c.Theos(context.Background(), acct); err != nil {
		t.Fatalf("Theos: %v", err)
	}
	ci := c.CheckIn(acct)
	if ci == nil || ci.Nonce != "0123456789abcdef0123456789abcdef" || ci.Seconds != 600 {
		t.Fatalf("签到挑战解析错误: %#v", ci)
	}
	if got := header[0].Get(HeaderCheckIn); got != "" {
		t.Fatalf("首次请求不应带签到 nonce，实际 %q", got)
	}
	if b := header[0].Get(HeaderBeacon); !strings.HasPrefix(b, "v="+AppVersion+";up=") || !strings.Contains(b, "pi=") {
		t.Fatalf("心跳格式不对: %q", b)
	}

	if _, err := c.Theos(context.Background(), acct); err != nil {
		t.Fatalf("第二次 Theos: %v", err)
	}
	if got := header[1].Get(HeaderCheckIn); got != ci.Nonce {
		t.Fatalf("第二次请求应回传 nonce，实际 %q", got)
	}
}

// 403 拒绝里也会带 check_in，必须照样吃掉，否则永远签不上到。
func TestTheosCapturesCheckInFromRefusal(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = io.WriteString(w, `{"error":{"message":"social","code":"social_login_required"},`+
			`"check_in":{"nonce":"ffffffffffffffffffffffffffffffff","seconds":600}}`)
	})
	acct := &Account{APIKey: "k-blocked"}
	_, err := c.Theos(context.Background(), acct)
	var he *HTTPError
	if !asHTTPError(err, &he) || !he.SocialLoginRequired() {
		t.Fatalf("应返回 social_login_required，实际 %v", err)
	}
	if got := c.CheckIn(acct); got == nil || got.Seconds != 600 {
		t.Fatalf("拒绝响应里的 check_in 未被保存: %#v", got)
	}
}

// 策略类错误是终态：不能换号重试，也不能被当成可重试 403。
func TestHTTPErrorPolicyClassification(t *testing.T) {
	social := mustErr(t, http.StatusForbidden, `{"error":{"code":"social_login_required"}}`)
	if !social.SocialLoginRequired() || social.Retryable() {
		t.Fatalf("social_login_required 应识别为策略拦截且不可重试: %+v", social)
	}
	machine := mustErr(t, http.StatusBadRequest, `{"error":{"code":"machine_required"}}`)
	if !machine.MachineRequired() || machine.Retryable() {
		t.Fatalf("machine_required 应不可重试: %+v", machine)
	}
	lane := mustErr(t, http.StatusUpgradeRequired, `{"error":{"code":"lane_version_unsupported"}}`)
	if !lane.LaneUnsupported() || lane.Retryable() {
		t.Fatalf("lane_version_unsupported 应识别为车道错误且不可重试: %+v", lane)
	}
	// 普通 403 仍然可换号。
	if !mustErr(t, http.StatusForbidden, `{}`).Retryable() {
		t.Fatal("普通 403 应保持可重试")
	}
}

// 400 形状拒绝 -> 桌面端字段白名单 + 补 system 消息。
func TestChatBodyFallback(t *testing.T) {
	body := []byte(`{"model":"m","stream":true,"user":"u1","messages":[{"role":"user","content":"hi"}],"tools":[]}`)
	rejected := mustErr(t, http.StatusBadRequest,
		`{"error":{"message":"This request is not one DeepAI Code sends.","code":"invalid_request_error"}}`)
	if !rejected.BodyRejected() {
		t.Fatal("应识别为请求体形状拒绝")
	}

	out, ok := ChatBodyFallback(body, "")
	if !ok {
		t.Fatal("可解析的 messages 应能兜底")
	}
	var got map[string]any
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("兜底结果不是合法 JSON: %v", err)
	}
	if _, exists := got["user"]; exists {
		t.Fatal("白名单外的字段应被剔除")
	}
	if got["stream"] != true || got["model"] != "m" {
		t.Fatalf("白名单字段应保留: %#v", got)
	}
	msgs, _ := got["messages"].([]any)
	if len(msgs) != 2 {
		t.Fatalf("应插入一条 system 消息，实际 %d 条", len(msgs))
	}
	if role, _ := msgs[0].(map[string]any)["role"].(string); role != "system" {
		t.Fatalf("第一条应为 system，实际 %q", role)
	}

	if _, ok := ChatBodyFallback([]byte(`{"model":"m"}`), ""); ok {
		t.Fatal("没有 messages 时不应兜底")
	}
}

// 已带 system 的请求不应被重复插入。
func TestChatBodyFallbackKeepsExistingSystem(t *testing.T) {
	body := []byte(`{"model":"m","messages":[{"role":"system","content":"s"},{"role":"user","content":"hi"}]}`)
	out, ok := ChatBodyFallback(body, "")
	if !ok {
		t.Fatal("应能兜底")
	}
	var got map[string]any
	_ = json.Unmarshal(out, &got)
	if msgs, _ := got["messages"].([]any); len(msgs) != 2 {
		t.Fatalf("不应插入额外 system，实际 %d 条", len(msgs))
	}
}

// 推理客户端必须与桌面端一致：Node fetch 不发 User-Agent / sec-ch-ua /
// Accept-Language，网关带上去等于在推理接口自报“我不是客户端”。
func TestInferenceClientOmitsBrowserFingerprint(t *testing.T) {
	var got http.Header
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Clone()
		_, _ = io.WriteString(w, `{"ok":true}`)
	})
	c.BrowserFingerprint = false

	acct := &Account{
		APIKey:         "k1",
		UserAgent:      "Mozilla/5.0 (Windows NT 10.0; Win64; x64) Chrome/156.0.0.0",
		AcceptLanguage: "en-US,en;q=0.9",
		SecCHUA:        `"Chromium";v="156"`,
		Platform:       `"Windows"`,
	}
	if _, err := c.OpenModel(context.Background(), acct, PathChat, []byte(`{"model":"x"}`), ""); err != nil {
		t.Fatalf("OpenModel: %v", err)
	}
	// 桌面端走 Node fetch：UA 是 node，没有 sec-ch-ua / Accept-Language 指纹。
	if v := got.Get("User-Agent"); v != "node" {
		t.Errorf("推理请求的 UA 应为 node，实际 %q", v)
	}
	if v := got.Get("Accept-Language"); v != "*" {
		t.Errorf("Accept-Language 应为 *，实际 %q", v)
	}
	for _, h := range []string{"sec-ch-ua", "sec-ch-ua-platform", "sec-ch-ua-mobile"} {
		if v := got.Get(h); v != "" {
			t.Errorf("推理请求不应携带 %s: %q", h, v)
		}
	}
}

// 注册链路继续带浏览器指纹（与网页注册时的特征保持一致）。
func TestRegisterClientKeepsBrowserFingerprint(t *testing.T) {
	var got http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Clone()
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))
	t.Cleanup(srv.Close)

	c := NewClientWithFingerprint("", "agent-ua/1.0")
	c.Origin = srv.URL
	c.Machine = "machine-fingerprint"

	acct := &Account{APIKey: "k2", SecCHUA: `"Chromium";v="156"`, Platform: `"Windows"`, AcceptLanguage: "en-US"}
	if _, err := c.OpenModel(context.Background(), acct, PathChat, []byte(`{"model":"x"}`), ""); err != nil {
		t.Fatalf("OpenModel: %v", err)
	}
	if v := got.Get("User-Agent"); v != "agent-ua/1.0" {
		t.Fatalf("注册客户端应带 UA，实际 %q", v)
	}
	if got.Get("sec-ch-ua") == "" || got.Get("sec-ch-ua-platform") == "" || got.Get("Accept-Language") == "" {
		t.Fatalf("注册客户端应带齐指纹头: %#v", got)
	}
}

// 形状回退不能顺手改掉采样条件：seed / service_tier / stream_options
// 会改变采样与用量上报，指纹类检测甚至会因此换一个候选模型。
// store 则是桌面端每次请求都带的字段，必须同样保留。
func TestChatBodyFallbackKeepsSamplingFields(t *testing.T) {
	body := []byte(`{"model":"m","messages":[{"role":"user","content":"hi"}],` +
		`"seed":7,"service_tier":"flex","stream":true,"store":false,` +
		`"stream_options":{"include_usage":true},"presence_penalty":0.5,"user":"u1"}`)
	out, ok := ChatBodyFallback(body, "")
	if !ok {
		t.Fatal("应能兜底")
	}
	var got map[string]any
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("兜底结果不是 JSON: %v", err)
	}
	for _, k := range []string{"seed", "service_tier", "stream_options", "store"} {
		if _, exists := got[k]; !exists {
			t.Errorf("白名单丢了 %s，会改变采样条件或用量上报", k)
		}
	}
	if v, exists := got["store"]; exists && v != false {
		t.Errorf("store 应原值保留，实际 %#v", v)
	}
	for _, k := range []string{"presence_penalty", "user"} {
		if _, exists := got[k]; exists {
			t.Errorf("非桌面字段 %s 仍应被剥离", k)
		}
	}
}

// GET 请求也要与桌面端同形：requestDeepAIJson 的 header 字面量对无 body 的
// /models、/theos、/get_usage_summary 一样带 Content-Type: application/json，
// 且 /get_usage_summary 不覆盖 UA（只有 /gravity-display-proxy 用 DeepAI-Code）。
func TestGetRequestsMatchDesktopHeaders(t *testing.T) {
	type seen struct {
		path        string
		contentType string
		userAgent   string
	}
	var got []seen
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		got = append(got, seen{r.URL.Path, r.Header.Get("Content-Type"), r.Header.Get("User-Agent")})
		if r.URL.Path == ModelBasePath+PathModels {
			_, _ = io.WriteString(w, `{"models":[{"id":"glm-5.3-flash","name":"GLM","reasoning":true,"input":["text"],"contextWindow":1000,"maxTokens":100}],"default":"glm-5.3-flash"}`)
			return
		}
		_, _ = io.WriteString(w, `{"enabled":true}`)
	})
	c.BrowserFingerprint = false // 推理形态：UA 应为 node
	acct := &Account{APIKey: "k"}

	if _, _, err := c.FetchModels(context.Background()); err != nil {
		t.Fatalf("FetchModels: %v", err)
	}
	if _, err := c.Theos(context.Background(), acct); err != nil {
		t.Fatalf("Theos: %v", err)
	}
	if _, err := c.UsageSummary(context.Background(), acct); err != nil {
		t.Fatalf("UsageSummary: %v", err)
	}

	for _, s := range got {
		if s.contentType != "application/json" {
			t.Errorf("GET %s 的 Content-Type = %q，期望 application/json", s.path, s.contentType)
		}
		if s.userAgent != "node" {
			t.Errorf("GET %s 的 User-Agent = %q，期望 node", s.path, s.userAgent)
		}
	}
}

// 拿到 nonce 后必须尽快回传：上游把「完成签到」当作真实模型分支的准入条件，
// 只拉不回传等于没签到（docs/recon.md §5.1）。
//
// 没有 ackedNonce 时 EnsureCheckIn 会在 seconds/2（300s）内直接返回，
// 冷启动后的头 5 分钟推理都还没签到。
func TestEnsureCheckInAcksPromptly(t *testing.T) {
	var (
		mu     sync.Mutex
		header []http.Header
	)
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		header = append(header, r.Header.Clone())
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"enabled":true,"balance":"1","granted":"0","lane_version":4,`+
			`"check_in":{"nonce":"0123456789abcdef0123456789abcdef","seconds":600}}`)
	})
	acct := &Account{APIKey: "k-ack"}
	ctx := context.Background()

	// 模拟启动探测：只拿到挑战，还没回传。
	if _, err := c.Theos(ctx, acct); err != nil {
		t.Fatalf("Theos: %v", err)
	}
	if got := header[0].Get(HeaderCheckIn); got != "" {
		t.Fatalf("首次请求不应带 nonce，实际 %q", got)
	}

	// 推理前补签到：必须立刻回传，而不是等半衰期。
	c.EnsureCheckIn(ctx, acct)
	if len(header) != 2 {
		t.Fatalf("应立刻补一次 /theos，实际请求数 %d", len(header))
	}
	if got := header[1].Get(HeaderCheckIn); got != "0123456789abcdef0123456789abcdef" {
		t.Fatalf("补签到必须回传 nonce，实际 %q", got)
	}
	if b := header[1].Get(HeaderBeacon); !strings.Contains(b, "pi=") {
		t.Fatalf("补签到应带心跳，实际 %q", b)
	}

	// 已回传且未到半衰期 → 不再打扰上游。
	c.EnsureCheckIn(ctx, acct)
	if len(header) != 2 {
		t.Fatalf("已签到后不应重复请求，实际 %d", len(header))
	}
}

// 签到被限流（check_in_refused）不是账号坏：推理仍 200，只是落在替身分支。
// 必须能和 social_login_required 区分开，否则好号会被标成 error。
func TestCheckInRefusedDetection(t *testing.T) {
	e := mustErr(t, http.StatusForbidden, `{"error":{"type":"check_in_refused","code":"check_in_refused"}}`)
	if !IsCheckInRefused(e) {
		t.Fatal("应识别为 check_in_refused")
	}
	if e.SocialLoginRequired() {
		t.Fatal("不应被当成 social_login_required")
	}
	if e.AuthFailed() {
		t.Fatal("不应被当成鉴权失败")
	}
	// 其它 403 不应误判。
	if IsCheckInRefused(mustErr(t, http.StatusForbidden, `{"error":{"code":"social_login_required"}}`)) {
		t.Fatal("social_login_required 不应识别为 check_in_refused")
	}
}

// check_in_refused 之后必须静默锁住签到：403 里仍会下发新的 check_in，
// 不锁的话 needAck 分支会让 EnsureCheckIn 每 30s（checkInMinInterval）
// 再撞一次 /theos —— 自己制造一场签到风暴（docs/recon.md §5.1）。
func TestEnsureCheckInHonorsRefusalLock(t *testing.T) {
	var mu sync.Mutex
	hits := 0
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		hits++
		mu.Unlock()
		w.WriteHeader(http.StatusForbidden)
		_, _ = io.WriteString(w, `{"error":{"type":"check_in_refused","code":"check_in_refused"},`+
			`"check_in":{"nonce":"0123456789abcdef0123456789abcdef","seconds":600}}`)
	})
	acct := &Account{APIKey: "k-refused"}
	ctx := context.Background()
	count := func() int {
		mu.Lock()
		defer mu.Unlock()
		return hits
	}
	// 绕过 checkInMinInterval（30s）的节流：本用例只验证「锁」本身。
	resetAttempt := func() {
		st := c.stateFor(acct)
		c.checkIns.mu.Lock()
		st.attemptAt = time.Time{}
		c.checkIns.mu.Unlock()
	}

	c.EnsureCheckIn(ctx, acct)
	if got := count(); got != 1 {
		t.Fatalf("首次应触发一次 /theos，实际 %d", got)
	}
	until := c.CheckInRefusedUntil(acct)
	if until.IsZero() || time.Until(until) < 5*time.Hour {
		t.Fatalf("应锁约 6h，实际剩余 %v", time.Until(until))
	}

	resetAttempt()
	c.EnsureCheckIn(ctx, acct)
	if got := count(); got != 1 {
		t.Fatalf("锁期内不应再打 /theos，实际 %d 次", got)
	}

	// 锁到期后允许再试一次（仍被拒就重新续锁）。
	st := c.stateFor(acct)
	c.checkIns.mu.Lock()
	st.refusedUntil = time.Now().Add(-time.Second)
	c.checkIns.mu.Unlock()
	resetAttempt()
	c.EnsureCheckIn(ctx, acct)
	if got := count(); got != 2 {
		t.Fatalf("锁到期后应允许重试，实际 %d 次", got)
	}
}
