// Package machine 负责「本机机器指纹」：读固件/系统标识 → sha256。
//
// 为什么单独成包：它要 exec 平台命令（Windows 的 powershell/reg、macOS 的
// ioreg、Linux 的 /etc/machine-id）、做 sync.Once 缓存，而 internal/deepai
// 的其余部分是「一个 HTTP 请求怎么发」。原先混在一起时，任何只想要
// Account/Model 类型的包都会被拖进 OS 探测与 exec 的依赖里。
//
// 上游按机器每天发一次免费 Theos，所以指纹必须稳定；读不到时返回空串
// （调用方据此不发该请求头，而不是发一个空值）。
package machine

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"
)

// uuidPattern 匹配标准 UUID 字符串。
var uuidPattern = regexp.MustCompile(`[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}`)

// smbiosPlaceholders 是固件常见的占位 UUID，不能作为稳定机器标识。
var smbiosPlaceholders = map[string]bool{
	"00000000-0000-0000-0000-000000000000": true,
	"ffffffff-ffff-ffff-ffff-ffffffffffff": true,
	"03000200-0400-0500-0006-000700080009": true,
}

var (
	idOnce  sync.Once
	idValue string
)

// ID 返回本机稳定的机器指纹（原始标识的 sha256）。
//
// 上游用它按天发放免费 Theos：同一台机器每天只发一次。
// 结果在进程内只计算一次；读不到时返回空串（上游会拒绝免费额度，
// 但付费请求仍可工作）。
//
// 可用 DEEPAI_MACHINE_ID 显式覆盖（写入原始 UUID，内部仍做 sha256），
// 便于容器 / 无 SMBIOS 环境部署。
//
// 注意：这是**本机**指纹，只适合不需要区分账号的请求（模型目录等）。
// 推理 / 配额请求应优先使用账号自带的 MachineID（见 Fingerprint），
// 否则多个账号共用一个指纹，上游只会给第一个账号发放每日额度。
func ID() string {
	idOnce.Do(func() {
		raw := strings.TrimSpace(os.Getenv("DEEPAI_MACHINE_ID"))
		if raw == "" {
			raw = detectMachineRaw()
		}
		idValue = Fingerprint(raw)
	})
	return idValue
}

// Fingerprint 把原始机器标识（UUID / machine-id）转成上游要求的 sha256 十六进制。
//
// 空串返回空串：调用方据此“不发该请求头”，而不是发一个空值。
func Fingerprint(raw string) string {
	raw = strings.ToLower(strings.TrimSpace(raw))
	if raw == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

// NewUUID 生成一个新的随机 UUID（v4）（v4）。
//
// 每个注册账号分配一个独立的机器指纹，才能各自拿到上游按机器发放的每日额度；
// 全局共用一个指纹时，只有第一个账号能拿到额度（其余 balance=0）。
func NewUUID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return ""
	}
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant RFC 4122
	return formatUUID(b)
}

// formatUUID 把 16 字节格式化为 8-4-4-4-12。
func formatUUID(b [16]byte) string {
	const hexDigits = "0123456789abcdef"
	buf := make([]byte, 36)
	j := 0
	for i, v := range b {
		if i == 4 || i == 6 || i == 8 || i == 10 {
			buf[j] = '-'
			j++
		}
		buf[j] = hexDigits[v>>4]
		buf[j+1] = hexDigits[v&0x0f]
		j += 2
	}
	return string(buf)
}

func detectMachineRaw() string {
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()

	switch runtime.GOOS {
	case "windows":
		if id := parseUUID(run(ctx, "powershell", "-NoProfile", "-NonInteractive", "-Command",
			"(Get-CimInstance Win32_ComputerSystemProduct).UUID")); id != "" {
			return id
		}
		return parseMachineGuid(run(ctx, "reg", "query", `HKLM\SOFTWARE\Microsoft\Cryptography`, "/v", "MachineGuid"))
	case "darwin":
		out := run(ctx, "ioreg", "-rd1", "-c", "IOPlatformExpertDevice")
		if m := regexp.MustCompile(`"IOPlatformUUID"\s*=\s*"([^"]+)"`).FindStringSubmatch(out); len(m) == 2 {
			if uuidPattern.MatchString(m[1]) {
				return strings.ToLower(m[1])
			}
		}
	case "linux":
		// 0.3.41 / 0.3.42 / 0.3.43 / 0.3.44 桌面端只用 systemd machine-id（0.3.37 起未变；无 root 读不到固件 UUID）：
		// /etc/machine-id → /var/lib/dbus/machine-id，要求 32 位小写 hex 且非全 0。
		// 早前版本先读 /sys/class/dmi/id/product_uuid，但固件占位值会返回一个
		// 不稳定的指纹（桌面端早已不用），这里与 0.3.44 对齐。
		for _, path := range []string{"/etc/machine-id", "/var/lib/dbus/machine-id"} {
			if b, err := os.ReadFile(path); err == nil {
				if id := parseLinuxMachineID(string(b)); id != "" {
					return id
				}
			}
		}
	}
	return ""
}

// parseUUID 提取并校验第一个非占位 UUID。
func parseUUID(s string) string {
	m := uuidPattern.FindString(s)
	if m == "" {
		return ""
	}
	low := strings.ToLower(m)
	if smbiosPlaceholders[low] {
		return ""
	}
	return low
}

// parseLinuxMachineID 校验 systemd machine-id：32 位小写 hex，且不能是全 0。
//
// 与桌面端 parseLinuxMachineId 同义（0.3.41 / 0.3.42 / 0.3.43 / 0.3.44 未变）：全 0 是未初始化标志，
// 不能当作稳定指纹。
func parseLinuxMachineID(raw string) string {
	id := strings.ToLower(strings.TrimSpace(raw))
	if len(id) != 32 {
		return ""
	}
	for _, r := range id {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return ""
		}
	}
	if strings.Trim(id, "0") == "" {
		return ""
	}
	return id
}

// parseMachineGuid 从 reg query 输出中提取 MachineGuid。
func parseMachineGuid(out string) string {
	m := regexp.MustCompile(`MachineGuid\s+REG_SZ\s+(\S+)`).FindStringSubmatch(out)
	if len(m) != 2 {
		return ""
	}
	if uuidPattern.MatchString(m[1]) {
		return strings.ToLower(m[1])
	}
	return strings.TrimSpace(m[1])
}

// run 执行外部命令并在超时后放弃，输出为空时返回空串。
func run(ctx context.Context, name string, args ...string) string {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stderr = nil
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return string(out)
}
