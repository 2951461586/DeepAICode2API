package model

import (
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
	// 指纹行为与具体取值无关；这里刻意用「运行时拼出来的串」而不是字面量，
	// 免得被密钥扫描器（gitleaks generic-api-key）当成真实凭据。
	key := strings.Repeat("fa", 12)
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
