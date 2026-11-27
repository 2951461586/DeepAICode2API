package deepai

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestProofConfigFromEnvModes 锁住三档模式的解析与降级。
//
// 上游当前不校验该头，所以「发错」比「不发」更危险：未知模式必须归一到 off，
// 配了 helper 却没给路径也必须归一到 off，而不是发出一个看起来合法的垃圾值。
func TestProofConfigFromEnvModes(t *testing.T) {
	cases := []struct {
		mode, helper string
		wantMode     string
	}{
		{"", "", ProofModeOff},
		{"off", "/x", ProofModeOff},
		{"random", "", ProofModeRandom},
		{"helper", "/usr/local/bin/resource-monitor", ProofModeHelper},
		{"helper", "", ProofModeOff},     // 缺路径 → off
		{"HELPER", "", ProofModeOff},     // 大小写归一
		{"random ", "", ProofModeRandom}, // 去空白
		{"bogus", "", ProofModeOff},      // 未知 → off
	}
	for _, c := range cases {
		t.Setenv("DEEPAI_PROOF", c.mode)
		t.Setenv("DEEPAI_PROOF_HELPER", c.helper)
		got := ProofConfigFromEnv().normalize()
		if got.Mode != c.wantMode {
			t.Fatalf("mode=%q helper=%q → %q，期望 %q", c.mode, c.helper, got.Mode, c.wantMode)
		}
	}
}

// TestProofOffOmitsHeader 是默认行为：off 时不发头（与 v0.1.29 之前一致）。
func TestProofOffOmitsHeader(t *testing.T) {
	var got http.Header
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Clone()
		_, _ = io.WriteString(w, `{"ok":true}`)
	})
	c.Proof = ProofConfig{Mode: ProofModeOff}

	if _, err := c.OpenModel(context.Background(), &Account{APIKey: "k"}, PathChat, []byte(`{"model":"x"}`), ""); err != nil {
		t.Fatalf("OpenModel: %v", err)
	}
	if v, ok := got[HeaderProof]; ok {
		t.Fatalf("off 模式不应发 %s，实际 %v", HeaderProof, v)
	}
}

// TestProofRandomSendsHex 是协议面对齐：random 模式发 32 位 hex（合法区间 24–64）。
func TestProofRandomSendsHex(t *testing.T) {
	var got http.Header
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Clone()
		_, _ = io.WriteString(w, `{"ok":true}`)
	})
	c.Proof = ProofConfig{Mode: ProofModeRandom}

	if _, err := c.OpenModel(context.Background(), &Account{APIKey: "k"}, PathChat, []byte(`{"model":"x"}`), ""); err != nil {
		t.Fatalf("OpenModel: %v", err)
	}
	v := got.Get(HeaderProof)
	if !proofValuePattern.MatchString(v) {
		t.Fatalf("%s = %q，期望 24–64 位小写 hex", HeaderProof, v)
	}

	// 两次调用不能是同一个值（否则就不是随机而是常量）。
	_, _ = c.OpenModel(context.Background(), &Account{APIKey: "k"}, PathChat, []byte(`{"model":"x"}`), "")
	if again := got.Get(HeaderProof); again == v {
		t.Fatalf("两次随机值相同: %q", v)
	}
}

// writeHelper 写一个假的 resource-monitor：从 stdin 读 JSON，往 stdout 写 $out。
//
// 是 `/bin/sh` 脚本，只在 Unix 上可执行；helper 模式的端到端断言因此在 Windows
// 跳过（CI 与线上都是 Linux，真正的覆盖在那里）。
func writeHelper(t *testing.T, out string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("helper 是 Unix 脚本，Windows 上无法 exec；由 Linux CI 覆盖")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "input.json"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(dir, "fake-helper")
	body := "#!/bin/sh\ncat > " + filepath.Join(dir, "input.json") + "\nprintf '%s' '" + out + "'\n"
	if err := os.WriteFile(script, []byte(body), 0o700); err != nil {
		t.Fatal(err)
	}
	return script
}

// TestProofHelperUsesOutput 验证 helper 模式：把输入喂给 helper、采用它的输出。
func TestProofHelperUsesOutput(t *testing.T) {
	helper := writeHelper(t, "1383b80e3ea1b91f0dd76d74")
	var got http.Header
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Clone()
		_, _ = io.WriteString(w, `{"ok":true}`)
	})
	c.Proof = ProofConfig{Mode: ProofModeHelper, Helper: helper}

	if _, err := c.OpenModel(context.Background(), &Account{APIKey: "k"}, PathChat, []byte(`{"model":"x"}`), ""); err != nil {
		t.Fatalf("OpenModel: %v", err)
	}
	if v := got.Get(HeaderProof); v != "1383b80e3ea1b91f0dd76d74" {
		t.Fatalf("%s = %q，期望 helper 的输出", HeaderProof, v)
	}

	// 输入必须是调用标签形态：{n, b=sha256(body)}。
	raw, err := os.ReadFile(filepath.Join(filepath.Dir(helper), "input.json"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	if !strings.Contains(text, `"b":"`) || !strings.Contains(text, `"n":`) {
		t.Fatalf("helper 收到的输入不对: %s", text)
	}
	if want := sha256Hex([]byte(`{"model":"x"}`)); !strings.Contains(text, want) {
		t.Fatalf("helper 输入里缺少 sha256(body) %s: %s", want, text)
	}
}

// TestProofHelperFailuresOmitHeader 锁住「helper 不可用就省略该头」。
//
// 桌面端的三条失败语义（缺文件 / 超时 / 输出不合规）都必须退化成「不发头」，
// 绝不能因为自证失败把推理请求本身带崩。
func TestProofHelperFailuresOmitHeader(t *testing.T) {
	cases := []struct {
		name   string
		helper string
	}{
		{"helper 不存在", filepath.Join(t.TempDir(), "missing")},
		{"输出不是 hex", writeHelper(t, "not-a-hex-value")},
		{"输出过短", writeHelper(t, "abc")},
		{"相对路径", "fake-helper"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var got http.Header
			cli, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				got = r.Header.Clone()
				_, _ = io.WriteString(w, `{"ok":true}`)
			})
			cli.Proof = ProofConfig{Mode: ProofModeHelper, Helper: c.helper}
			if _, err := cli.OpenModel(context.Background(), &Account{APIKey: "k"}, PathChat, []byte(`{"model":"x"}`), ""); err != nil {
				t.Fatalf("OpenModel 不该因自证失败而报错: %v", err)
			}
			if v, ok := got[HeaderProof]; ok {
				t.Fatalf("应省略 %s，实际 %v", HeaderProof, v)
			}
		})
	}
}

// TestProofOnCheckIn 验证 /theos 也带自证，且输入是签到证明形态。
func TestProofOnCheckIn(t *testing.T) {
	helper := writeHelper(t, "abcdefabcdefabcdefabcdef")
	var got http.Header
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Clone()
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"enabled":true,"balance":"1.00000","lane_version":8}`)
	})
	c.Proof = ProofConfig{Mode: ProofModeHelper, Helper: helper}
	if _, err := c.Theos(context.Background(), &Account{APIKey: "sk-x"}); err != nil {
		t.Fatalf("Theos: %v", err)
	}
	if v := got.Get(HeaderProof); v != "abcdefabcdefabcdefabcdef" {
		t.Fatalf("%s = %q", HeaderProof, v)
	}
	raw, err := os.ReadFile(filepath.Join(filepath.Dir(helper), "input.json"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	// 签到证明的字段：n / a=sha256(apiKey) / m / i / v
	for _, key := range []string{`"n":`, `"a":"`, `"m":`, `"i":`, `"v":"`} {
		if !strings.Contains(text, key) {
			t.Fatalf("签到证明缺少字段 %s: %s", key, text)
		}
	}
	if !strings.Contains(text, sha256Hex([]byte("sk-x"))) {
		t.Fatalf("签到证明的 a 应是 sha256(apiKey): %s", text)
	}
}
