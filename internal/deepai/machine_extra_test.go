package deepai

import (
	"context"
	"runtime"
	"strings"
	"testing"
)

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
	if got := MachineFingerprint(""); got != "" {
		t.Fatalf("空指纹应返回空串（调用方据此不发请求头），得到 %q", got)
	}
	if got := MachineFingerprint("   "); got != "" {
		t.Fatalf("纯空白应返回空串，得到 %q", got)
	}
	up := MachineFingerprint("3F2504E0-4F89-11D3-9A0C-0305E82C3301")
	low := MachineFingerprint("3f2504e0-4f89-11d3-9a0c-0305e82c3301")
	if up == "" || up != low {
		t.Fatalf("指纹应大小写不敏感: %q / %q", up, low)
	}
	if len(up) != 64 || strings.Trim(up, "0123456789abcdef") != "" {
		t.Fatalf("指纹应是 64 位小写 hex，得到 %q", up)
	}
}
