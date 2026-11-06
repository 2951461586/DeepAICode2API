package deepai

import (
	"context"
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

// UUIDPattern 匹配标准 UUID 字符串。
var UUIDPattern = regexp.MustCompile(`[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}`)

// SMBIOSPlaceholders 是固件常见的占位 UUID，不能作为稳定机器标识。
var SMBIOSPlaceholders = map[string]bool{
	"00000000-0000-0000-0000-000000000000": true,
	"ffffffff-ffff-ffff-ffff-ffffffffffff": true,
	"03000200-0400-0500-0006-000700080009": true,
}

var (
	machineOnce sync.Once
	machineID   string
)

// MachineID 返回本机稳定的机器指纹（原始标识的 sha256）。
//
// 上游用它按天发放免费 Theos：同一台机器每天只发一次。
// 结果在进程内只计算一次；读不到时返回空串（上游会拒绝免费额度，
// 但付费请求仍可工作）。
//
// 可用 DEEPAI_MACHINE_ID 显式覆盖（写入原始 UUID，内部仍做 sha256），
// 便于容器 / 无 SMBIOS 环境部署。
func MachineID() string {
	machineOnce.Do(func() {
		raw := strings.TrimSpace(os.Getenv("DEEPAI_MACHINE_ID"))
		if raw == "" {
			raw = detectMachineRaw()
		}
		if raw == "" {
			return
		}
		sum := sha256.Sum256([]byte(strings.ToLower(raw)))
		machineID = hex.EncodeToString(sum[:])
	})
	return machineID
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
			if UUIDPattern.MatchString(m[1]) {
				return strings.ToLower(m[1])
			}
		}
	case "linux":
		for _, path := range []string{"/sys/class/dmi/id/product_uuid", "/etc/machine-id", "/var/lib/dbus/machine-id"} {
			if b, err := os.ReadFile(path); err == nil {
				s := strings.TrimSpace(string(b))
				if s == "" {
					continue
				}
				if id := parseUUID(s); id != "" {
					return id
				}
				return s
			}
		}
	}
	return ""
}

// parseUUID 提取并校验第一个非占位 UUID。
func parseUUID(s string) string {
	m := UUIDPattern.FindString(s)
	if m == "" {
		return ""
	}
	low := strings.ToLower(m)
	if SMBIOSPlaceholders[low] {
		return ""
	}
	return low
}

// parseMachineGuid 从 reg query 输出中提取 MachineGuid。
func parseMachineGuid(out string) string {
	m := regexp.MustCompile(`MachineGuid\s+REG_SZ\s+(\S+)`).FindStringSubmatch(out)
	if len(m) != 2 {
		return ""
	}
	if UUIDPattern.MatchString(m[1]) {
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
