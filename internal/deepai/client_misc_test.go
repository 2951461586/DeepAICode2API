package deepai

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"strings"
	"testing"
)

// errWriter 累计写入预算耗尽后返回错误，用于覆盖 SSE 写出错误的分支。
type errWriter struct{ budget int }

func (w *errWriter) Write(p []byte) (int, error) {
	if w.budget <= 0 {
		return 0, errors.New("写入失败")
	}
	if len(p) > w.budget {
		n := w.budget
		w.budget = 0
		return n, errors.New("写入失败")
	}
	w.budget -= len(p)
	return len(p), nil
}

func TestWriteSSEFormatAndErrors(t *testing.T) {
	var buf strings.Builder
	if err := WriteSSE(&buf, []byte(`{"a":1}`)); err != nil {
		t.Fatalf("WriteSSE: %v", err)
	}
	if got := buf.String(); got != "data: {\"a\":1}\n\n" {
		t.Fatalf("WriteSSE 输出 = %q", got)
	}

	// 三段写入（"data: " 6 字节 / payload 7 字节 / "\n\n" 2 字节）
	// 任意一段失败都要把错误抛给调用方（否则流会静默截断）。
	for _, budget := range []int{0, 6, 13} {
		if err := WriteSSE(&errWriter{budget: budget}, []byte(`{"a":1}`)); err == nil {
			t.Fatalf("写入预算 %d 字节用尽时应报错", budget)
		}
	}
}

func TestWriteSSEEventFormatAndErrors(t *testing.T) {
	var buf strings.Builder
	if err := WriteSSEEvent(&buf, "content_block_delta", []byte(`{"x":1}`)); err != nil {
		t.Fatalf("WriteSSEEvent: %v", err)
	}
	want := "event: content_block_delta\ndata: {\"x\":1}\n\n"
	if got := buf.String(); got != want {
		t.Fatalf("WriteSSEEvent 输出 = %q，期望 %q", got, want)
	}

	// 空 event 名不写 event 行（OpenAI 兼容端点的形态）。
	buf.Reset()
	if err := WriteSSEEvent(&buf, "", []byte(`[DONE]`)); err != nil {
		t.Fatalf("WriteSSEEvent: %v", err)
	}
	if got := buf.String(); got != "data: [DONE]\n\n" {
		t.Fatalf("空 event 输出 = %q", got)
	}

	// "event: e\n" 9 + "data: " 6 + payload 7 + "\n\n" 2 = 24 字节。
	for _, budget := range []int{0, 5, 9, 15, 22} {
		if err := WriteSSEEvent(&errWriter{budget: budget}, "e", []byte(`{"x":1}`)); err == nil {
			t.Fatalf("写入预算 %d 字节用尽时应报错", budget)
		}
	}
}

// TestBodyRejectedRequires400 固化「只有 400 且文案匹配才算请求体形状拒绝」。
func TestBodyRejectedRequires400(t *testing.T) {
	msg := `{"error":{"message":"This request is not one DeepAI Code sends.","code":"invalid_request_error"}}`
	if !mustErr(t, http.StatusBadRequest, msg).BodyRejected() {
		t.Fatal("400 + 固定文案应判定为请求体拒绝")
	}
	if mustErr(t, http.StatusInternalServerError, msg).BodyRejected() {
		t.Fatal("非 400 不应判定为请求体拒绝")
	}
	if mustErr(t, http.StatusBadRequest, `{"error":{"message":"bad param"}}`).BodyRejected() {
		t.Fatal("文案不匹配不应判定为请求体拒绝")
	}
	if mustErr(t, http.StatusBadRequest, ``).BodyRejected() {
		t.Fatal("空 body 不应判定为请求体拒绝")
	}
}

// TestBodyRejectedAcceptsNewMessage 覆盖 2026-11-13 实测到的新文案：
// 上游把同一个门槛的 message 换成了 “This request cannot be accepted.”，
// 漏掉它 ChatBodyFallback 就不会重试，调用方直接吃 400。
func TestBodyRejectedAcceptsNewMessage(t *testing.T) {
	msg := `{"error":{"message":"This request cannot be accepted.","type":"invalid_request_error","code":"invalid_request_error"}}`
	if !mustErr(t, http.StatusBadRequest, msg).BodyRejected() {
		t.Fatal("400 + 新文案（cannot be accepted）应判定为请求体拒绝")
	}
	if mustErr(t, http.StatusForbidden, msg).BodyRejected() {
		t.Fatal("非 400 不应判定为请求体拒绝")
	}
}

// TestHomeDirFallback 覆盖主目录不可用时的回落顺序。
func TestHomeDirFallback(t *testing.T) {
	if runtime.GOOS == "windows" {
		// Windows 上 os.UserHomeDir 读 USERPROFILE，两者都清空才会失败。
		t.Setenv("USERPROFILE", "")
		t.Setenv("HOMEDRIVE", "")
		t.Setenv("HOMEPATH", "")
	} else {
		t.Setenv("HOME", "")
	}
	// 无论是否拿到真实主目录，都不允许返回空串。
	if got := HomeDir(); got == "" {
		t.Fatal("HomeDir 不应为空串")
	}
}

// TestDoTransportError 覆盖连接失败：错误必须原样上抛并被判定为瞬时故障。
func TestDoTransportError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := srv.URL
	srv.Close() // 端口已关闭，后续请求连接被拒

	c := NewClient()
	c.Origin = url

	_, _, err := c.FetchModels(context.Background())
	if err == nil {
		t.Fatal("连接失败应返回错误")
	}
	if !IsTransient(err) {
		t.Fatalf("连接被拒应判定为瞬时故障，得到 %v", err)
	}

	if _, err := c.OpenModel(context.Background(), &Account{APIKey: "k"}, PathChat, []byte(`{}`), "aff"); err == nil {
		t.Fatal("OpenModel 连接失败应返回错误")
	}
	if _, err := c.Theos(context.Background(), &Account{APIKey: "k"}); err == nil {
		t.Fatal("Theos 连接失败应返回错误")
	}
	if _, err := c.postJSON(context.Background(), nil, PathWebSearch, []byte(`{}`), "联网搜索"); err == nil {
		t.Fatal("postJSON 连接失败应返回错误")
	}
	if _, err := c.Transcribe(context.Background(), nil, "", "", []byte("x")); err == nil {
		t.Fatal("Transcribe 连接失败应返回错误")
	}
	if _, err := c.ExchangeCode(context.Background(), "code"); err == nil {
		t.Fatal("ExchangeCode 连接失败应返回错误")
	}
	if _, err := c.DetectLane(context.Background()); err == nil {
		t.Fatal("DetectLane 连接失败应返回错误")
	}
	if c.Lane() != LaneVersion {
		t.Fatalf("探测失败不应改车道，实际 %q", c.Lane())
	}
}

// TestDetectLaneServerErrorFallback 覆盖「全是 5xx」：应给出兜底错误，
// 因为 5xx 说明上游抖动而不是车道不对（此时 lastErr 为空）。
func TestDetectLaneServerErrorFallback(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	})
	_, err := c.DetectLane(context.Background())
	if err == nil || !strings.Contains(err.Error(), "所有候选车道都被上游拒绝") {
		t.Fatalf("全 5xx 应给出兜底错误，得到 %v", err)
	}
}

// TestImportCredentialFileReadError 覆盖「路径存在但读不了」。
func TestImportCredentialFileReadError(t *testing.T) {
	dir := t.TempDir()
	if _, err := importCredentialFile(dir); err == nil ||
		!strings.Contains(err.Error(), "读取凭据失败") {
		t.Fatalf("目录当作凭据文件应报读取失败，得到 %v", err)
	}
	if _, err := importCredentialFile(dir + string(os.PathSeparator) + "missing.bin"); err == nil {
		t.Fatal("不存在的文件应报错")
	}
}

func TestAccountFromCredentialsPropagatesInvalidKey(t *testing.T) {
	_, err := AccountFromCredentials(&Credentials{APIKey: "bad key"}, "desktop")
	if err == nil || !strings.Contains(err.Error(), "格式非法") {
		t.Fatalf("非法 Key 应透传校验错误，得到 %v", err)
	}

	acct, err := AccountFromCredentials(&Credentials{APIKey: "sk-x", Username: "u", IsPro: true}, "")
	if err != nil {
		t.Fatalf("AccountFromCredentials: %v", err)
	}
	if acct.Source != "manual" {
		t.Fatalf("空 source 应回落 manual，得到 %q", acct.Source)
	}
	if acct.Name != "u" || !acct.IsPro {
		t.Fatalf("字段丢失: %+v", acct)
	}
}

// TestHasSystemMessageIgnoresMalformed 覆盖 messages 里的脏数据。
func TestHasSystemMessageIgnoresMalformed(t *testing.T) {
	cases := []struct {
		name string
		msgs []any
		want bool
	}{
		{"空", nil, false},
		{"只有 user", []any{map[string]any{"role": "user"}}, false},
		{"system", []any{map[string]any{"role": "system"}}, true},
		{"developer 也算", []any{map[string]any{"role": "developer"}}, true},
		{"非对象条目被跳过", []any{"oops", 42, nil, map[string]any{"role": "system"}}, true},
		{"role 非字符串", []any{map[string]any{"role": 7}}, false},
		{"大小写敏感", []any{map[string]any{"role": "System"}}, false},
	}
	for _, c := range cases {
		if got := hasSystemMessage(c.msgs); got != c.want {
			t.Errorf("%s: hasSystemMessage()=%v，期望 %v", c.name, got, c.want)
		}
	}
}

// TestChatBodyFallbackDirtyMessages 覆盖 messages 里混入非对象条目时仍能兜底。
func TestChatBodyFallbackDirtyMessages(t *testing.T) {
	body := []byte(`{"model":"m","messages":["oops",{"role":"user","content":"hi"}]}`)
	out, ok := ChatBodyFallback(body, "自定义提示词")
	if !ok {
		t.Fatal("messages 非空即应兜底")
	}
	var got map[string]any
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("兜底结果不是合法 JSON: %v", err)
	}
	msgs, _ := got["messages"].([]any)
	if len(msgs) != 3 {
		t.Fatalf("应插入一条 system，实际 %d 条", len(msgs))
	}
	first, _ := msgs[0].(map[string]any)
	if first["role"] != "system" || first["content"] != "自定义提示词" {
		t.Fatalf("自定义提示词未生效: %#v", first)
	}

	if _, ok := ChatBodyFallback([]byte(`not json`), ""); ok {
		t.Fatal("非法 JSON 不应兜底")
	}
	if _, ok := ChatBodyFallback([]byte(`{"messages":{}}`), ""); ok {
		t.Fatal("messages 不是数组时不应兜底")
	}
	if _, ok := ChatBodyFallback([]byte(`{"messages":[]}`), ""); ok {
		t.Fatal("messages 为空时不应兜底")
	}
}

// TestDetectMachineRawNonWindows 只在非 Windows 上跑：Windows 分支会拉起
// powershell（12s 超时），不适合放进单元测试。
func TestDetectMachineRawNonWindows(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows 分支依赖 powershell / reg，跳过")
	}
	got := detectMachineRaw()
	if got == "" {
		t.Log("未探测到机器标识（容器无 SMBIOS / 无 machine-id），回落空串符合预期")
		return
	}
	if got != strings.ToLower(got) {
		t.Fatalf("机器标识应统一小写，得到 %q", got)
	}
	if strings.ContainsAny(got, " \t\r\n") {
		t.Fatalf("机器标识不应含空白，得到 %q", got)
	}
}
