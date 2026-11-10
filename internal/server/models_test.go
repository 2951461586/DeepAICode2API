package server

import (
	"encoding/json"
	"net/http"
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
