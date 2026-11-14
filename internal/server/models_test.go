package server

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"deepaicode2api/internal/deepai"
)

// fixtureModels 是一份覆盖各种形态的目录：免费 / 付费 / 按秒计费 / responses 端点。
func fixtureModels() []deepai.Model {
	return []deepai.Model{
		{
			ID: "free-chat", Name: "Free Chat", Reasoning: true,
			Input: []string{"text", "image"}, ContextWindow: 1048576, MaxTokens: 131072,
			TheosPerRequest: "0",
		},
		{
			ID: "paid-chat", Name: "Paid Chat", Premier: true, Reasoning: true,
			Input: []string{"text"}, ContextWindow: 1000000, MaxTokens: 64000,
			TheosPerRequest: "15", TheosPerSecond: map[string]string{"standard": "0.5"},
		},
		{
			ID: "paid-responses", Name: "Paid Responses", Premier: true,
			Input: []string{"text", "image"}, API: "openai-responses",
			TheosPerRequest: "50",
		},
	}
}

// TestPublicModelsExposeDeepAIMeta 钉住 /v1/models 的 deepai 元信息字段。
//
// 客户端（含控制台之外的脚本）靠它判断上下文窗口与计费；少一个字段不会报错，
// 只会静默变成 undefined。
func TestPublicModelsExposeDeepAIMeta(t *testing.T) {
	s, key := newTestServer(t, nil)
	s.Store.SetModels(fixtureModels(), "free-chat")

	rec := do(t, s, http.MethodGet, "/v1/models", key, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /v1/models = %d：%s", rec.Code, rec.Body.String())
	}

	var payload struct {
		Object string `json:"object"`
		Data   []struct {
			ID     string         `json:"id"`
			Object string         `json:"object"`
			Owned  string         `json:"owned_by"`
			Deep   map[string]any `json:"deepai"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("响应不是合法 JSON: %v", err)
	}
	if payload.Object != "list" || len(payload.Data) != 3 {
		t.Fatalf("列表结构异常: object=%q count=%d", payload.Object, len(payload.Data))
	}

	for _, m := range payload.Data {
		if m.Object != "model" || m.Owned != "deepai" {
			t.Errorf("%s: object/owned_by 错误", m.ID)
		}
		for _, field := range []string{
			"name", "reasoning", "input", "context_window", "max_tokens",
			"premier", "free", "api", "theos_per_request", "theos_per_second",
		} {
			if _, ok := m.Deep[field]; !ok {
				t.Errorf("%s: deepai 元信息缺少 %q", m.ID, field)
			}
		}
	}

	// free 必须与 Model.Free() 同源：付费模型即使便宜也不能标免费。
	byID := map[string]map[string]any{}
	for _, m := range payload.Data {
		byID[m.ID] = m.Deep
	}
	if byID["free-chat"]["free"] != true {
		t.Error("free-chat 应标记 free=true")
	}
	if byID["paid-chat"]["free"] != false {
		t.Error("paid-chat 应标记 free=false")
	}
	sec, ok := byID["paid-chat"]["theos_per_second"].(map[string]any)
	if !ok || sec["standard"] != "0.5" {
		t.Fatalf("theos_per_second 未透出: %#v", byID["paid-chat"]["theos_per_second"])
	}
}

// TestAdminModelsExposeFreeAndPerSecond 钉住控制台模型列表的字段。
//
// 控制台的「计费」列直接读后端下发的 free（不再自己算口径），
// 「每秒计费」列读 theosPerSecond；两者缺一个，列就会静默空白。
func TestAdminModelsExposeFreeAndPerSecond(t *testing.T) {
	s, _ := newTestServer(t, nil)
	s.Store.SetModels(fixtureModels(), "free-chat")

	rec := do(t, s, http.MethodGet, "/admin/api/models", "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /admin/api/models = %d", rec.Code)
	}

	var got struct {
		Models []map[string]any `json:"models"`
		Def    string           `json:"default_model"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("响应不是合法 JSON: %v", err)
	}
	if got.Def != "free-chat" {
		t.Fatalf("default_model = %q", got.Def)
	}
	if len(got.Models) != 3 {
		t.Fatalf("模型数 = %d，期望 3", len(got.Models))
	}

	for _, m := range got.Models {
		if _, ok := m["free"]; !ok {
			t.Errorf("%v: 缺少 free（控制台计费列依赖它）", m["id"])
		}
		if _, ok := m["theosPerSecond"]; !ok {
			t.Errorf("%v: 缺少 theosPerSecond（控制台每秒计费列依赖它）", m["id"])
		}
		if _, ok := m["contextWindow"]; !ok {
			t.Errorf("%v: 缺少 contextWindow", m["id"])
		}
	}

	freeCount := 0
	for _, m := range got.Models {
		if m["free"] == true {
			freeCount++
		}
	}
	if freeCount != 1 {
		t.Fatalf("免费模型数 = %d，期望 1（只有 free-chat）", freeCount)
	}
}

// TestAdminModelsFallbackWhenCatalogEmpty 确认新装且上游不可达时，
// 控制台与 /v1/models 都回落到同一个兜底模型（曾出现一边空一边有）。
func TestAdminModelsFallbackWhenCatalogEmpty(t *testing.T) {
	s, key := newTestServer(t, nil)

	var admin struct {
		Models  []map[string]any `json:"models"`
		Default string           `json:"default_model"`
	}
	if err := json.Unmarshal(do(t, s, http.MethodGet, "/admin/api/models", "", "").Body.Bytes(), &admin); err != nil {
		t.Fatalf("管理端响应解析失败: %v", err)
	}
	var pub struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(do(t, s, http.MethodGet, "/v1/models", key, "").Body.Bytes(), &pub); err != nil {
		t.Fatalf("公开端响应解析失败: %v", err)
	}

	if len(admin.Models) != 1 || admin.Models[0]["id"] != deepai.DefaultModel {
		t.Fatalf("管理端兜底目录异常: %#v", admin.Models)
	}
	if admin.Default != deepai.DefaultModel {
		t.Fatalf("管理端 default_model = %q", admin.Default)
	}
	if len(pub.Data) != 1 || pub.Data[0].ID != deepai.DefaultModel {
		t.Fatalf("公开端兜底目录异常: %#v", pub.Data)
	}
}

// 2026-11-14 起上游 /models 要求 Api-Key：目录刷新必须借一个账号的凭据，
// 号池空时要给出可行动的错误，而不是把上游 401 原样抛给控制台。
func TestRefreshModelsNeedsAccount(t *testing.T) {
	u := newWarmupUpstream(t)
	s, _ := newTestServer(t, u)
	s.Store.UpdateAccounts(func(a *deepai.Account) bool {
		a.Enabled = false
		return true
	})

	_, err := s.RefreshModels(context.Background())
	if err == nil {
		t.Fatal("没有可用账号时应报错")
	}
	if !strings.Contains(err.Error(), "Api-Key") {
		t.Fatalf("错误应说明需要 Api-Key，实际 %v", err)
	}
	if got := countPath(u, deepai.ModelBasePath+deepai.PathModels); got != 0 {
		t.Fatalf("没有凭据时不应发请求，实际 %d 次", got)
	}
}

// 有账号时必须带上它的 Key（否则目录拉取必然 401）。
func TestRefreshModelsSendsAccountKey(t *testing.T) {
	u := newWarmupUpstream(t)
	s, _ := newTestServer(t, u)

	models, err := s.RefreshModels(context.Background())
	if err != nil {
		t.Fatalf("RefreshModels: %v", err)
	}
	if len(models) == 0 {
		t.Fatal("目录不应为空")
	}
	if key := u.last(t).Header.Get(deepai.HeaderKeyName); key != "upstream-key" {
		t.Fatalf("拉目录的 Api-Key = %q，期望 upstream-key", key)
	}
}

// probeAccount 优先「就绪 + 带机器指纹」的号；没有就退到任一有 Key 的启用号。
func TestProbeAccountPrefersReadyWithMachine(t *testing.T) {
	s, _ := newTestServer(t, nil) // acc-1：ok，但没机器指纹
	s.Store.UpsertAccount(&deepai.Account{
		ID: "with-machine", Name: "wm", APIKey: "k-wm", Enabled: true, Status: "ok",
		MachineID: "11111111-1111-1111-1111-111111111111",
	})
	s.Store.UpsertAccount(&deepai.Account{
		ID: "disabled", Name: "off", APIKey: "k-off", Enabled: false,
	})

	got := s.probeAccount()
	if got == nil || got.ID != "with-machine" {
		t.Fatalf("应优先选带机器指纹的就绪号，实际 %+v", got)
	}

	s.Store.UpdateAccounts(func(a *deepai.Account) bool {
		if a.ID == "with-machine" || a.ID == "disabled" {
			a.Enabled = false
		}
		return true
	})
	if got := s.probeAccount(); got == nil || got.ID != "acc-1" {
		t.Fatalf("应回落到仍启用的 acc-1，实际 %+v", got)
	}
}
