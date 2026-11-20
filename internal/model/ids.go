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
func RandomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return strings.Repeat("0", n*2)
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
