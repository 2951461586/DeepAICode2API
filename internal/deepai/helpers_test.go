package deepai

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRandomTokenIsHexAndUnique(t *testing.T) {
	a, b := RandomToken(16), RandomToken(16)
	if len(a) != 32 {
		t.Fatalf("RandomToken(16) 长度 = %d，期望 32", len(a))
	}
	if strings.Trim(a, "0123456789abcdef") != "" {
		t.Fatalf("RandomToken 应是小写 hex，得到 %q", a)
	}
	if a == b {
		t.Fatal("两次 RandomToken 不应相同")
	}

	if got := RandomToken(0); got != "" {
		t.Fatalf("RandomToken(0) = %q，期望空串", got)
	}
	if got := RandomHex(1); len(got) != 2 {
		t.Fatalf("RandomHex(1) 长度 = %d，期望 2", len(got))
	}
}

func TestKeyFingerprintAndShortHash(t *testing.T) {
	key := "sk-live-abcdef123456"
	fp := KeyFingerprint(key)
	if len(fp) != 16 {
		t.Fatalf("KeyFingerprint 长度 = %d，期望 16", len(fp))
	}
	if strings.Trim(fp, "0123456789abcdef") != "" {
		t.Fatalf("KeyFingerprint 应是小写 hex，得到 %q", fp)
	}
	if fp != KeyFingerprint("  "+key+"\n") {
		t.Fatal("KeyFingerprint 应先 TrimSpace")
	}
	if fp == KeyFingerprint(key+"x") {
		t.Fatal("不同 Key 不应撞指纹")
	}

	short := ShortHash(key)
	if len(short) != 8 || !strings.HasPrefix(fp, short) {
		t.Fatalf("ShortHash=%q 应是 KeyFingerprint=%q 的前 8 位", short, fp)
	}
}

func TestMaskSecret(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"", ""},
		{"a", "a***"},
		{"abcdefghij", "a***"},           // len == 10 走短分支
		{"abcdefghijk", "abcdef...hijk"}, // len 11 才保留首 6 尾 4
		{"sk-1234567890abcdef", "sk-123...cdef"},
	}
	for _, c := range cases {
		if got := MaskSecret(c.in); got != c.want {
			t.Errorf("MaskSecret(%q)=%q，期望 %q", c.in, got, c.want)
		}
	}
}

// TestMaskProxyNeverLeaksCredentials 是安全断言：代理里的用户名/密码
// 绝不能出现在日志或控制台标签里。
func TestMaskProxyNeverLeaksCredentials(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"", ""},
		{"   ", ""},
		{"http://user:secret@10.0.0.1:8080", "10.0.0.1:8080"},
		{"  socks5://u:p@proxy.local:1080  ", "proxy.local:1080"},
		{"http://10.0.0.1:8080", "10.0.0.1:8080"},
		{"not-a-url", ""},
		{"http://", ""},
	}
	for _, c := range cases {
		got := MaskProxy(c.in)
		if got != c.want {
			t.Errorf("MaskProxy(%q)=%q，期望 %q", c.in, got, c.want)
		}
		if strings.Contains(got, "secret") || strings.Contains(got, "u:p") {
			t.Errorf("MaskProxy(%q) 泄露了凭据: %q", c.in, got)
		}
	}
}

func TestEnvIntAndEnvOr(t *testing.T) {
	t.Setenv("DEEPAI_TEST_INT", " 42 ")
	if got := EnvInt("DEEPAI_TEST_INT", 7); got != 42 {
		t.Fatalf("EnvInt 合法值 = %d，期望 42", got)
	}

	t.Setenv("DEEPAI_TEST_INT", "abc")
	if got := EnvInt("DEEPAI_TEST_INT", 7); got != 7 {
		t.Fatalf("EnvInt 非法值应回退默认，得到 %d", got)
	}

	t.Setenv("DEEPAI_TEST_INT", "   ")
	if got := EnvInt("DEEPAI_TEST_INT", 7); got != 7 {
		t.Fatalf("EnvInt 纯空白应回退默认，得到 %d", got)
	}

	os.Unsetenv("DEEPAI_TEST_INT")
	if got := EnvInt("DEEPAI_TEST_INT", 7); got != 7 {
		t.Fatalf("EnvInt 未设置应回退默认，得到 %d", got)
	}

	t.Setenv("DEEPAI_TEST_STR", "  value  ")
	if got := EnvOr("DEEPAI_TEST_STR", "def"); got != "value" {
		t.Fatalf("EnvOr 应去空白，得到 %q", got)
	}
	t.Setenv("DEEPAI_TEST_STR", "   ")
	if got := EnvOr("DEEPAI_TEST_STR", "def"); got != "def" {
		t.Fatalf("EnvOr 纯空白应回退默认，得到 %q", got)
	}
}

func TestHomeDirIsNonEmpty(t *testing.T) {
	if got := HomeDir(); got == "" {
		t.Fatal("HomeDir 任何时候都不应返回空串（需保证有回落）")
	}
}

func TestDesktopBaseDirsExplicit(t *testing.T) {
	got := DesktopBaseDirs("/tmp/explicit-home")
	if len(got) != 1 || got[0] != "/tmp/explicit-home" {
		t.Fatalf("显式路径应原样返回单元素，得到 %v", got)
	}
}

// TestDesktopBaseDirsFromEnv 覆盖两个候选环境变量并验证去重。
func TestDesktopBaseDirsFromEnv(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("T3CODE_HOME", dir)
	t.Setenv("DEEPAI_CODE_HOME", "  "+dir+"  ") // 同值（带空白）应被去重

	got := DesktopBaseDirs("")
	if len(got) != 2 {
		t.Fatalf("期望 [envDir, ~/.deepai-code]，得到 %v", got)
	}
	if got[0] != dir {
		t.Fatalf("首个候选应来自 T3CODE_HOME 且已去空白，得到 %q", got[0])
	}
	if !strings.HasSuffix(got[1], filepath.Join(".deepai-code")) {
		t.Fatalf("末位候选应为 ~/.deepai-code，得到 %q", got[1])
	}

	// 环境变量清空后只剩主目录候选。
	t.Setenv("T3CODE_HOME", "")
	t.Setenv("DEEPAI_CODE_HOME", "")
	if got := DesktopBaseDirs(""); len(got) != 1 {
		t.Fatalf("无环境变量时应只有一个候选，得到 %v", got)
	}
}

// TestDesktopSecretDirsLayouts 覆盖“用户可能指到任意一层”的四种形态。
func TestDesktopSecretDirsLayouts(t *testing.T) {
	base := t.TempDir()

	// 1) 指向 baseDir：展开成三个候选（userdata/secrets、secrets、dev/secrets）。
	got := DesktopSecretDirs(base)
	want := []string{
		filepath.Join(base, "userdata", "secrets"),
		filepath.Join(base, "secrets"),
		filepath.Join(base, "dev", "secrets"),
	}
	if len(got) != len(want) {
		t.Fatalf("baseDir 展开 = %v，期望 %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("baseDir 展开[%d] = %q，期望 %q", i, got[i], want[i])
		}
	}

	// 2) 直接指向凭据文件：回落到它所在目录。
	file := filepath.Join(base, CredentialFile)
	if err := os.WriteFile(file, []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := DesktopSecretDirs(file); len(got) != 1 || got[0] != base {
		t.Fatalf("凭据文件应回落到所在目录，得到 %v", got)
	}

	// 3) 指向不存在的路径：仍展开三个候选（调用方自己 Stat）。
	missing := filepath.Join(base, "nope")
	if got := DesktopSecretDirs(missing); len(got) != 3 {
		t.Fatalf("不存在的路径也应展开候选，得到 %v", got)
	}

	// 4) 空串：沿用 DesktopBaseDirs 的探测结果。
	t.Setenv("T3CODE_HOME", base)
	t.Setenv("DEEPAI_CODE_HOME", "")
	got = DesktopSecretDirs("")
	if len(got) != 6 { // base 的 3 个 + ~/.deepai-code 的 3 个
		t.Fatalf("空串探测应有 6 个候选，得到 %v", got)
	}
}

func TestDedupeNormalizesCaseAndClean(t *testing.T) {
	// Windows 下大小写不敏感，Linux 下 cleaner 只做 path.Clean；
	// dedupe 统一小写比较，两种平台都应只保留一份。
	got := dedupe([]string{"/a/b", "/a/b/", "/A/B", "/a//b", "/a/c"})
	if len(got) != 2 {
		t.Fatalf("dedupe 结果 = %v，期望 2 项", got)
	}
	if got[0] != "/a/b" {
		t.Fatalf("dedupe 应保留首次出现的原值，得到 %q", got[0])
	}
	if got := dedupe(nil); len(got) != 0 {
		t.Fatalf("nil 输入应返回空切片，得到 %v", got)
	}
}
