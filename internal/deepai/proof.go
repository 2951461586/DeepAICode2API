package deepai

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"deepaicode2api/internal/configenv"
	"deepaicode2api/internal/logx"
)

// X-DeepAI-Code-Proof 的生成（0.3.52 / 车道 8 新增的第 6 个 X-DeepAI-Code-* 头）。
//
// 桌面端的值来自随包原生 helper `resource-monitor --attest`：helper 用**本构建的
// 密钥**对一段输入做签名，输出 24–64 位 hex。两种输入形态：
//
//	签到证明（/theos）：{"n": 签到 nonce | "", "a": sha256(apiKey), "m": 机器指纹,
//	                     "i": 实例号, "v": 应用版本}
//	调用标签（模型调用）：{"n": 上次签到 nonce, "b": sha256(请求体)}
//
// 网关没有那个构建密钥，所以**能不能产出自证取决于运维有没有把 helper 放到
// 这台机器上**。因此这里做成三档显式模式（DEEPAI_PROOF），而不是偷偷伪造：
//
//	off    不发该头（历史行为，上游 2026-11-25 实测不校验，缺头照样 200）
//	random 发 32 位随机 hex：只对齐「协议面」，不声称任何自证
//	helper 调用 DEEPAI_PROOF_HELPER 指向的 resource-monitor，产出真实自证；
//	       任何失败（缺文件 / 超时 / 输出不合规）都**省略该头** —— 与桌面端
//	       「helper 不可用就省略」的行为一致
//
// 为什么要把 random 单独列出来：上游当前不校验，但「伪造一个看起来合法的自证」
// 与「明确声明我们在做协议面模拟」是两件事；把模式写进环境变量，日志与文档
// 才对得上。
const (
	ProofModeOff    = "off"
	ProofModeRandom = "random"
	ProofModeHelper = "helper"
)

// proofTimeout 是 helper 的单次调用预算。
//
// 桌面端同样是 5s（超时就省略该头）；这里与它对齐，且**必须**有预算：
// 这是推理热路径上的 exec。
const proofTimeout = 5 * time.Second

// proofValuePattern 是上游接受的 hex 长度区间（24–64）。
var proofValuePattern = regexp.MustCompile(`^[0-9a-f]{24,64}$`)

// ProofConfig 决定网关是否、以及如何生成 X-DeepAI-Code-Proof。
type ProofConfig struct {
	Mode   string // off | random | helper（空串等价 off）
	Helper string // Mode=helper 时的二进制路径
}

// ProofConfigFromEnv 读取 DEEPAI_PROOF / DEEPAI_PROOF_HELPER。
func ProofConfigFromEnv() ProofConfig {
	return ProofConfig{
		Mode:   strings.ToLower(strings.TrimSpace(configenv.Or("DEEPAI_PROOF", ProofModeOff))),
		Helper: strings.TrimSpace(configenv.Or("DEEPAI_PROOF_HELPER", "")),
	}
}

// normalize 把未知模式归一成 off（宁可少发一个头，也不要发出看不懂的东西）。
func (p ProofConfig) normalize() ProofConfig {
	switch p.Mode {
	case ProofModeRandom:
		return p
	case ProofModeHelper:
		if p.Helper != "" {
			return p
		}
		// 配了 helper 却没给路径：降级成 off，并把这件事说出来（配置错误不该静默）。
		logx.Warn("DEEPAI_PROOF=helper 但未设置 DEEPAI_PROOF_HELPER，已按 off 处理")
		return ProofConfig{Mode: ProofModeOff}
	default:
		if p.Mode != "" && p.Mode != ProofModeOff {
			logx.Warn("未知的 DEEPAI_PROOF 模式，已按 off 处理", "mode", p.Mode)
		}
		return ProofConfig{Mode: ProofModeOff}
	}
}

// enabled 报告是否需要生成该头。
func (p ProofConfig) enabled() bool {
	m := p.normalize().Mode
	return m == ProofModeRandom || m == ProofModeHelper
}

// sign 按模式产出一个自证值；空串表示「省略该头」。
func (c *Client) signProof(input map[string]any) string {
	cfg := c.Proof.normalize()
	switch cfg.Mode {
	case ProofModeRandom:
		// 16 字节 = 32 位 hex，落在 24–64 的合法区间里。
		var buf [16]byte
		if _, err := rand.Read(buf[:]); err != nil {
			return ""
		}
		return hex.EncodeToString(buf[:])
	case ProofModeHelper:
		return runProofHelper(cfg.Helper, input)
	default:
		return ""
	}
}

// runProofHelper 调用随包 helper 产出真实自证。
//
// 与桌面端一致的三条失败语义：helper 不存在 / 5s 内没输出 / 输出不是 24–64 位
// hex —— 一律返回空串（省略该头），绝不影响请求本身。
func runProofHelper(path string, input map[string]any) string {
	// 只接受绝对路径，并用 filepath.Clean 规范化：这个路径来自运维配置
	// （DEEPAI_PROOF_HELPER），不是请求数据，但也没有理由让相对路径 / `..`
	// 在这里被解析——越界执行是最不该出现在推理热路径上的事故。
	if !filepath.IsAbs(path) {
		logx.Warn("DEEPAI_PROOF_HELPER 必须是绝对路径，已省略 Proof 头", "path", path)
		return ""
	}
	target := filepath.Clean(path)
	raw, err := json.Marshal(input)
	if err != nil {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), proofTimeout)
	defer cancel()
	// 固定参数（只多一个 --attest），stdin 喂 JSON：不可注入。
	// 路径来自运维配置（绝对路径 + Clean），不来自请求数据，故对
	// "non-static command" 的静态告警按已审阅处置。
	// pi-lens-ignore: go.lang.security.audit.dangerous-exec-command
	cmd := exec.CommandContext(ctx, target, "--attest")
	cmd.Stdin = bytes.NewReader(raw)
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	value := strings.ToLower(strings.TrimSpace(string(out)))
	if !proofValuePattern.MatchString(value) {
		return ""
	}
	return value
}

// sha256Hex 是自证输入里的 a / b 字段。
func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// proofCheckInValue 生成 /theos 上的签到证明；空串表示省略。
func (c *Client) proofCheckInValue(acct *Account, nonce string) string {
	if c == nil || !c.Proof.enabled() {
		return ""
	}
	apiKey := ""
	if acct != nil {
		apiKey = acct.APIKey
	}
	return c.signProof(map[string]any{
		"n": nonce,
		"a": sha256Hex([]byte(apiKey)),
		"m": c.machineFingerprint(acct),
		"i": c.Instance,
		"v": AppVersion,
	})
}

// proofCallValue 生成模型调用上的请求标签；空串表示省略。
//
// n 用「上一次成功回传的签到 nonce」：与桌面端 laneProof.ts 的取法一致，
// 没有签到过就是空串。
func (c *Client) proofCallValue(acct *Account, body []byte) string {
	if c == nil || !c.Proof.enabled() {
		return ""
	}
	return c.signProof(map[string]any{
		"n": lastAckedNonce(c, acct),
		"b": sha256Hex(body),
	})
}

// lastAckedNonce 返回该账号最近一次**成功回传**的签到 nonce（没有就是空串）。
//
// 注意区分「拿到 nonce」与「回传完成」：调用标签要用后者（桌面端 laneProof.ts
// 也是这么取的），否则会把一个上游还没确认的挑战算进自证输入。
func lastAckedNonce(c *Client, acct *Account) string {
	if c == nil || acct == nil {
		return ""
	}
	st := c.stateFor(acct)
	c.checkIns.mu.Lock()
	defer c.checkIns.mu.Unlock()
	return st.ackedNonce
}
