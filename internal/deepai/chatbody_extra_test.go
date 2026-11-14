package deepai

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"
	"time"
)

// TestEnsureSystemMessageAddsOnlyWhenMissing 固化「预置形状只补 system」。
func TestEnsureSystemMessageAddsOnlyWhenMissing(t *testing.T) {
	body := []byte(`{"model":"m","temperature":0.3,"messages":[{"role":"user","content":"hi"}]}`)
	out, inserted := EnsureSystemMessage(body, "")
	if !inserted {
		t.Fatal("缺少 system 应插入")
	}
	var payload map[string]any
	if err := json.Unmarshal(out, &payload); err != nil {
		t.Fatalf("输出不是合法 JSON: %v", err)
	}
	msgs := payload["messages"].([]any)
	if len(msgs) != 2 {
		t.Fatalf("messages 长度 = %d，期望 2", len(msgs))
	}
	if msgs[0].(map[string]any)["role"] != "system" {
		t.Fatalf("第一条应为 system: %#v", msgs[0])
	}
	// 其它字段必须原样保留（不能像 ChatBodyFallback 那样裁剪）。
	if payload["temperature"].(float64) != 0.3 {
		t.Fatalf("temperature 被改写: %#v", payload["temperature"])
	}
	if !bytes.Contains(out, []byte(`"model":"m"`)) {
		t.Fatalf("model 丢失: %s", out)
	}

	// 已有 system（或 developer）时不动。
	for _, role := range []string{"system", "developer"} {
		with := []byte(`{"model":"m","messages":[{"role":"` + role + `","content":"s"},{"role":"user","content":"hi"}]}`)
		out2, inserted := EnsureSystemMessage(with, "")
		if inserted {
			t.Fatalf("已有 %s 时不应插入", role)
		}
		if !bytes.Equal(out2, with) {
			t.Fatalf("已有 %s 时不应改写请求体", role)
		}
	}

	// 非法 / 空 messages：不插入、返回 false。
	for _, bad := range []string{`not json`, `{}`, `{"messages":[]}`} {
		if _, ins := EnsureSystemMessage([]byte(bad), ""); ins {
			t.Fatalf("body=%q 不应插入", bad)
		}
	}
}

// TestQuotaExhaustedDetectionAndReset 覆盖 403 theos_exhausted 的识别与恢复时间。
func TestQuotaExhaustedDetectionAndReset(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	// refresh_at 必须是**相对** now 的时间：写死日期会让这条用例变成定时炸弹
	// （2026-11-14T00:05:52 一过，QuotaResetAt 按“已过期”回退到 now+6h，CI 就红）。
	resetAt := now.Add(2 * time.Hour)
	stamp := resetAt.Format(time.RFC3339)
	body := `{"error":{"message":"You've used today's Theos. They refresh at ` + stamp + `.",` +
		`"type":"theos_exhausted","code":"theos_exhausted","refresh_at":"` + stamp + `"}}`
	e := mustErr(t, http.StatusForbidden, body)
	if !e.QuotaExhausted() {
		t.Fatal("应识别为 theos_exhausted")
	}
	if e.SocialLoginRequired() || e.EmailUnverified() || e.MachineRequired() {
		t.Fatal("不应误判为其它账号终态")
	}
	if got := e.QuotaResetAt(6*time.Hour, now); !got.Equal(resetAt) {
		t.Fatalf("恢复时间 = %v，期望 %v", got, resetAt)
	}
	// refresh_at 已经过期（上游给的是过去时间）时用 fallback，而不是立刻重试。
	past := now.Add(-time.Hour).UTC().Format(time.RFC3339)
	stale := mustErr(t, http.StatusForbidden,
		`{"error":{"code":"theos_exhausted","refresh_at":"`+past+`"}}`)
	if got, want := stale.QuotaResetAt(6*time.Hour, now), now.Add(6*time.Hour); !got.Equal(want) {
		t.Fatalf("过期 refresh_at 应回退到 now+6h，实际 %v", got)
	}
	// 其它错误码不应命中。
	if mustErr(t, http.StatusForbidden, `{"error":{"code":"check_in_refused"}}`).QuotaExhausted() {
		t.Fatal("check_in_refused 不应识别为额度用完")
	}
}
