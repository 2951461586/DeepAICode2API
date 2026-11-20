package model

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// 标识与指纹：账号 / 密钥 / 会话的稳定短标识。
//
// 放在 model 而不是 deepai：store（账号去重）、credimport（新建账号）、
// guard（按密钥限流）都要用，而它们不该为此依赖传输层。

// RandomHex 生成 n 字节的随机十六进制串。
//
// 熵源失败时**panic**，不返回可预测的占位串：
// 这个函数的产物是账号 ID 与 OAuth state。静默返回全 0 会让两个账号拿到
// 同一 ID（UpsertAccount 按 ID 去重 → 数据被覆盖），并让 state 变得可猜。
// 拿不到熵的运行环境已经不可能安全工作，早失败比晚损坏好。
func RandomHex(n int) string {
	if n <= 0 {
		return ""
	}
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand 不可用，拒绝生成可预测的标识: " + err.Error())
	}
	return hex.EncodeToString(b)
}

// RandomToken 生成 n 字节随机数的十六进制串（用于 OAuth state）。
func RandomToken(n int) string { return RandomHex(n) }

// KeyFingerprint 返回 API Key 的 sha256 前 16 位十六进制。
func KeyFingerprint(key string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(key)))
	return hex.EncodeToString(sum[:8])
}

// ShortHash 返回 API Key 的短哈希，用于展示。
func ShortHash(key string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(key)))
	return hex.EncodeToString(sum[:4])
}
