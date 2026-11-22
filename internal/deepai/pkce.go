package deepai

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
)

// PKCE（RFC 7636）参数名与算法，取值与桌面端 0.3.45 的
// `deepAISignInUrl` / `exchangeDeepAICode` 逐字一致。
//
// 桌面端形态（0.3.45 起的 `src/deepai/api.ts`）：
//
//	GET  /deepai_code/desktop_auth/?state=…&port=…&code_challenge=…&code_challenge_method=S256
//	POST /deepai_code/desktop_auth/exchange/  {"code": …, "code_verifier": …}
//
// verifier = base64url(32 字节随机数)（43 字符，无填充），
// challenge = base64url(sha256(verifier))（43 字符，无填充）。
const (
	// PKCEParamChallenge / PKCEParamVerifier 是两端的参数名。
	PKCEParamChallenge = "code_challenge"
	PKCEParamVerifier  = "code_verifier"
	// PKCEMethodParam 声明 challenge 的摘要算法。
	PKCEMethodParam = "code_challenge_method"
	// PKCEMethodS256 是本项目唯一支持（也是桌面端唯一使用）的算法。
	PKCEMethodS256 = "S256"

	// pkceVerifierBytes 是 verifier 的熵（32 字节 → 43 字符 base64url）。
	pkceVerifierBytes = 32
	// pkceChallengeLen 是 S256 challenge/verifier 的固定长度，上游按此校验格式：
	// 2026-11-22 实测 `code_challenge=xx` 被回 400 `Invalid sign-in proof.`。
	pkceChallengeLen = 43
)

// ErrPKCERandom 表示取不到随机数（正常环境不会发生）。
var ErrPKCERandom = errors.New("PKCE 随机数生成失败")

// PKCEChallenge 计算 S256 challenge：base64url(sha256(verifier))，无填充。
func PKCEChallenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// NewPKCE 生成一对 (verifier, challenge)。
//
// 返回的 verifier 只存在内存里（loginSession），兑换完即随会话销毁。
func NewPKCE() (string, string, error) {
	buf := make([]byte, pkceVerifierBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", "", fmt.Errorf("%w: %v", ErrPKCERandom, err)
	}
	verifier := base64.RawURLEncoding.EncodeToString(buf)
	return verifier, PKCEChallenge(verifier), nil
}

// ValidPKCEVerifier 报告 verifier 是否符合 S256 的格式要求（43 字符 base64url）。
//
// 用途：`DEEPAI2API_LOGIN_PKCE=0` 关闭时不做任何 PKCE 处理，但若调用方
// 从别处拿到一个 verifier，至少要拒绝明显非法的值再发给上游。
func ValidPKCEVerifier(verifier string) bool {
	if len(verifier) != pkceChallengeLen {
		return false
	}
	if _, err := base64.RawURLEncoding.DecodeString(verifier); err != nil {
		return false
	}
	return true
}

// Package-level note: 常量与工具集中在此文件，client.go / login.go 只引用。
