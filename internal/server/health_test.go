package server

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"deepaicode2api/internal/deepai"
)

// /health 的 ready 口径直接决定控制台顶部那颗胶囊（“账号 N/M 就绪”）。
//
// 回归背景：旧实现用 `enabled && status != "error"`，于是
//   - 一次概率性 EOF / 429 就把号永久算成不健康（徽章长期 1/16）；
//   - 反而把 blocked（不会自愈的账号级拦截）算成健康。
//
// 这组测试把三条口径都钉住：陈旧错误算就绪、blocked 不算、新鲜错误进 degraded。
func TestHealthReadySemantics(t *testing.T) {
	u := newWarmupUpstream(t)
	s, _ := newTestServer(t, u)

	stale := time.Now().Add(-deepai.ErrorStaleAfter - time.Minute).Unix()
	fresh := time.Now().Unix()

	// 已有 acc-1（ok）。再补：陈旧 error、新鲜 error、blocked、停用。
	s.Store.UpsertAccount(&deepai.Account{ID: "stale-err", Name: "stale", APIKey: "k-stale",
		Enabled: true, Status: deepai.StatusError, LastErrAt: stale})
	s.Store.UpsertAccount(&deepai.Account{ID: "fresh-err", Name: "fresh", APIKey: "k-fresh",
		Enabled: true, Status: deepai.StatusError, LastErrAt: fresh})
	s.Store.UpsertAccount(&deepai.Account{ID: "blocked", Name: "blocked", APIKey: "k-blocked",
		Enabled: true, Status: deepai.StatusBlocked})
	s.Store.UpsertAccount(&deepai.Account{ID: "off", Name: "off", APIKey: "k-off",
		Enabled: false, Status: "ok"})

	rec := do(t, s, http.MethodGet, "/health", "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("HTTP %d", rec.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("解析 /health: %v", err)
	}

	got := map[string]int{
		"accounts": int(body["accounts"].(float64)),
		"ready":    int(body["ready"].(float64)),
		"degraded": int(body["degraded"].(float64)),
		"blocked":  int(body["blocked"].(float64)),
	}
	// acc-1(ok) + stale-err(陈旧=就绪) = 2；fresh-err/blocked/off 不算就绪。
	want := map[string]int{"accounts": 5, "ready": 2, "degraded": 1, "blocked": 1}
	for k, w := range want {
		if got[k] != w {
			t.Fatalf("%s = %d，期望 %d（全量 %#v）", k, got[k], w, got)
		}
	}
}
