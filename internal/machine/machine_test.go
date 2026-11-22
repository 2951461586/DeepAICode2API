package machine

import (
	"context"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

func TestFingerprint(t *testing.T) {
	// 必须是 sha256 十六进制（64 位小写）。
	got := Fingerprint("11111111-2222-3333-4444-555555555555")
	if !regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(got) {
		t.Fatalf("指纹格式错误: %q", got)
	}
	// 大小写与首尾空白应归一。
	if again := Fingerprint("  11111111-2222-3333-4444-555555555555  "); again != got {
		t.Fatalf("同一 UUID 的不同写法应得到同一指纹: %q vs %q", again, got)
	}
	if upper := Fingerprint("11111111-2222-3333-4444-555555555555"); upper != got {
		t.Fatalf("大小写应归一: %q vs %q", upper, got)
	}
	// 空串返回空串（调用方据此不发请求头）。
	if Fingerprint("") != "" || Fingerprint("   ") != "" {
		t.Fatal("空输入应返回空指纹")
	}
	// 不同 UUID 必须不同指纹。
	if Fingerprint("a") == Fingerprint("b") {
		t.Fatal("不同输入得到了相同指纹")
	}
}

func TestNewMachineUUID(t *testing.T) {
	pattern := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	seen := map[string]bool{}
	for i := 0; i < 200; i++ {
		id := NewUUID()
		if !pattern.MatchString(id) {
			t.Fatalf("不是合法的 v4 UUID: %q", id)
		}
		if seen[id] {
			t.Fatalf("生成了重复 UUID: %q", id)
		}
		seen[id] = true
	}
}

func TestMachineUUIDIsStableThroughFingerprint(t *testing.T) {
	// 同一账号多次请求必须发同一个指纹，否则上游会认为机器在漂移。
	id := NewUUID()
	if id == "" {
		t.Fatal("生成 UUID 失败")
	}
	first := Fingerprint(id)
	for i := 0; i < 5; i++ {
		if got := Fingerprint(id); got != first {
			t.Fatalf("同一 UUID 的指纹不稳定: %q vs %q", got, first)
		}
	}
	if strings.EqualFold(id, first) {
		t.Fatal("指纹不应该是 UUID 本身")
	}
}

// 与 0.3.41 / 0.3.42 / 0.3.43 / 0.3.44 桌面端 parseLinuxMachineId 对齐（0.3.37 起未变）：32 位小写 hex、非全 0。
func TestParseLinuxMachineID(t *testing.T) {
	cases := []struct {
		raw, want string
	}{
		{"0123456789abcdef0123456789abcdef", "0123456789abcdef0123456789abcdef"},
		{"  0123456789ABCDEF0123456789ABCDEF\n", "0123456789abcdef0123456789abcdef"},
		{"00000000000000000000000000000000", ""},
		{"", ""},
		{"not-a-machine-id", ""},
		{"0123456789abcdef0123456789abcde", ""},   // 31 位
		{"0123456789abcdef0123456789abcdef0", ""}, // 33 位
		{"0123456789abcdef0123456789abcdeg", ""},  // 非 hex
	}
	for _, c := range cases {
		if got := parseLinuxMachineID(c.raw); got != c.want {
			t.Fatalf("parseLinuxMachineID(%q) = %q，期望 %q", c.raw, got, c.want)
		}
	}
}

func TestParseUUID(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"标准 UUID", "3F2504E0-4F89-11D3-9A0C-0305E82C3301", "3f2504e0-4f89-11d3-9a0c-0305e82c3301"},
		{"前后有噪声", "UUID: 3f2504e0-4f89-11d3-9a0c-0305e82c3301 (SMBIOS)", "3f2504e0-4f89-11d3-9a0c-0305e82c3301"},
		{"全零占位", "00000000-0000-0000-0000-000000000000", ""},
		{"全 F 占位", "FFFFFFFF-FFFF-FFFF-FFFF-FFFFFFFFFFFF", ""},
		{"厂商占位", "03000200-0400-0500-0006-000700080009", ""},
		{"无 UUID", "MachineGuid REG_SZ not-a-uuid", ""},
		{"空串", "", ""},
		{"截断的 UUID", "3f2504e0-4f89-11d3-9a0c", ""},
	}
	for _, c := range cases {
		if got := parseUUID(c.in); got != c.want {
			t.Errorf("%s: parseUUID(%q)=%q，期望 %q", c.name, c.in, got, c.want)
		}
	}
}

// TestParseUUIDTakesFirstMatch 记录当前语义：只校验第一个匹配，
// 占位值在前时直接判空（不继续往后找）。
func TestParseUUIDTakesFirstMatch(t *testing.T) {
	in := "00000000-0000-0000-0000-000000000000 then 3f2504e0-4f89-11d3-9a0c-0305e82c3301"
	if got := parseUUID(in); got != "" {
		t.Fatalf("首个匹配是占位时应返回空串（现状），得到 %q", got)
	}
}

func TestParseMachineGuid(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "标准 reg query 输出",
			in: "HKEY_LOCAL_MACHINE\\SOFTWARE\\Microsoft\\Cryptography\n" +
				"    MachineGuid    REG_SZ    3F2504E0-4F89-11D3-9A0C-0305E82C3301\n\n",
			want: "3f2504e0-4f89-11d3-9a0c-0305e82c3301",
		},
		{
			name: "非 UUID 的 Guid 原样返回",
			in:   "    MachineGuid    REG_SZ    abc-123-xyz\n",
			want: "abc-123-xyz",
		},
		{
			name: "空输出",
			in:   "",
			want: "",
		},
		{
			name: "缺少该键",
			in:   "ERROR: The system was unable to find the specified registry key or value.",
			want: "",
		},
	}
	for _, c := range cases {
		if got := parseMachineGuid(c.in); got != c.want {
			t.Errorf("%s: parseMachineGuid()=%q，期望 %q", c.name, got, c.want)
		}
	}
}

func TestFormatUUIDLayout(t *testing.T) {
	var b [16]byte
	for i := range b {
		b[i] = byte(i)
	}
	got := formatUUID(b)
	if len(got) != 36 {
		t.Fatalf("长度 = %d，期望 36", len(got))
	}
	if want := "00010203-0405-0607-0809-0a0b0c0d0e0f"; got != want {
		t.Fatalf("formatUUID()=%q，期望 %q", got, want)
	}
	if strings.Count(got, "-") != 4 {
		t.Fatalf("连字符数量错误: %q", got)
	}
}

// TestRunReturnsEmptyOnFailure 覆盖 run 的两条失败路径：
// 可执行文件不存在、非零退出码。
func TestRunReturnsEmptyOnFailure(t *testing.T) {
	ctx := context.Background()

	if got := run(ctx, "definitely-not-a-real-binary-deepai-test", "arg"); got != "" {
		t.Fatalf("不存在的命令应返回空串，得到 %q", got)
	}

	// 非零退出码：cmd.Output() 返回 error → 空串。
	if runtime.GOOS == "windows" {
		if got := run(ctx, "cmd", "/c", "exit 1"); got != "" {
			t.Fatalf("非零退出码应返回空串，得到 %q", got)
		}
		return
	}
	if got := run(ctx, "sh", "-c", "exit 1"); got != "" {
		t.Fatalf("非零退出码应返回空串，得到 %q", got)
	}
}

// TestRunReturnsStdout 确认成功时原样返回 stdout（不裁剪）。
func TestRunReturnsStdout(t *testing.T) {
	var got string
	if runtime.GOOS == "windows" {
		got = run(context.Background(), "cmd", "/c", "echo hello")
	} else {
		got = run(context.Background(), "sh", "-c", "printf 'hello'")
	}
	if !strings.Contains(got, "hello") {
		t.Fatalf("stdout = %q，期望包含 hello", got)
	}
}

// TestMachineFingerprintNormalizes 补充空值 / 大小写 / 空白三条分支。
func TestMachineFingerprintNormalizes(t *testing.T) {
	if got := Fingerprint(""); got != "" {
		t.Fatalf("空指纹应返回空串（调用方据此不发请求头），得到 %q", got)
	}
	if got := Fingerprint("   "); got != "" {
		t.Fatalf("纯空白应返回空串，得到 %q", got)
	}
	up := Fingerprint("3F2504E0-4F89-11D3-9A0C-0305E82C3301")
	low := Fingerprint("3f2504e0-4f89-11d3-9a0c-0305e82c3301")
	if up == "" || up != low {
		t.Fatalf("指纹应大小写不敏感: %q / %q", up, low)
	}
	if len(up) != 64 || strings.Trim(up, "0123456789abcdef") != "" {
		t.Fatalf("指纹应是 64 位小写 hex，得到 %q", up)
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
