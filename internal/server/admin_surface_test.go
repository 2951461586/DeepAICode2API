package server

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"deepaicode2api/internal/deepai"
	"deepaicode2api/internal/store"
)

// keyView 对应 safeKeyList 下发的单条密钥视图。
type keyView struct {
	Key      string `json:"key"`
	Name     string `json:"name"`
	Group    string `json:"group"`
	Enabled  bool   `json:"enabled"`
	Created  int64  `json:"created_at"`
	LastUsed int64  `json:"last_used"`
	Requests int64  `json:"requests"`
}

func keysOf(t *testing.T, rec interface{ Body() []byte }) []keyView {
	t.Helper()
	var out struct {
		Keys []keyView `json:"keys"`
	}
	if err := json.Unmarshal(rec.Body(), &out); err != nil {
		t.Fatalf("密钥列表不是合法 JSON: %v\n%s", err, rec.Body())
	}
	return out.Keys
}

// body 让 httptest.ResponseRecorder / 内存缓冲统一取字节。
type bodyRecorder struct{ b []byte }

func (r bodyRecorder) Body() []byte { return r.b }

// TestAdminKeysNeverLeakPlaintext 是安全断言：列表接口只能下发脱敏值。
func TestAdminKeysNeverLeakPlaintext(t *testing.T) {
	s, key := newTestServer(t, nil)

	rec := do(t, s, http.MethodGet, "/admin/api/keys", "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /admin/api/keys = %d，期望 200", rec.Code)
	}
	keys := keysOf(t, bodyRecorder{rec.Body.Bytes()})
	if len(keys) != 1 {
		t.Fatalf("密钥数 = %d，期望 1", len(keys))
	}
	if keys[0].Key == key {
		t.Fatal("列表接口下发了完整密钥")
	}
	if keys[0].Key != mask(key) {
		t.Fatalf("脱敏值 = %q，期望 %q", keys[0].Key, mask(key))
	}
	if keys[0].Group != "default" || !keys[0].Enabled {
		t.Fatalf("密钥视图字段错误: %+v", keys[0])
	}
	if !strings.Contains(rec.Body.String(), `"keys"`) {
		t.Fatal("响应缺少 keys 字段")
	}
	// 完整密钥一次都不该出现在响应体里。
	if strings.Contains(rec.Body.String(), key) {
		t.Fatal("响应体包含完整密钥")
	}
}

func TestAdminKeysCreateAndGroupAndDelete(t *testing.T) {
	s, first := newTestServer(t, nil)

	// 新建：name/group 落库，密钥值形如 sk-xxx 且与既有密钥不同。
	rec := do(t, s, http.MethodPost, "/admin/api/keys", "", `{"name":"第二把","group":"g2"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST = %d，期望 200：%s", rec.Code, rec.Body.String())
	}
	var created struct {
		OK  bool   `json:"ok"`
		Key string `json:"key"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("解析: %v", err)
	}
	if !created.OK || created.Key == "" || created.Key == first {
		t.Fatalf("新建结果异常: %+v", created)
	}
	if !strings.HasPrefix(created.Key, "sk-") {
		t.Fatalf("密钥格式 = %q", created.Key)
	}

	keys := keysOf(t, bodyRecorder{do(t, s, http.MethodGet, "/admin/api/keys", "", "").Body.Bytes()})
	if len(keys) != 2 {
		t.Fatalf("密钥数 = %d，期望 2", len(keys))
	}
	var found bool
	for _, k := range keys {
		if k.Key == mask(created.Key) {
			found = true
			if k.Name != "第二把" || k.Group != "g2" {
				t.Fatalf("新建密钥字段错误: %+v", k)
			}
		}
	}
	if !found {
		t.Fatal("新建的密钥没出现在列表里")
	}

	// set-group：用脱敏值当引用（控制台实际传的就是这个）。
	rec = do(t, s, http.MethodPost, "/admin/api/keys", "",
		`{"action":"set-group","key":"`+mask(created.Key)+`","group":"g3"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("set-group = %d：%s", rec.Code, rec.Body.String())
	}
	var grouped struct {
		OK    bool   `json:"ok"`
		Group string `json:"group"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &grouped)
	if !grouped.OK || grouped.Group != "g3" {
		t.Fatalf("分组未生效: %+v", grouped)
	}
	if got := s.Store.KeyGroup(created.Key); got != "g3" {
		t.Fatalf("存储里的分组 = %q，期望 g3", got)
	}

	// set-group 缺少 key → 400。
	if rec := do(t, s, http.MethodPost, "/admin/api/keys", "", `{"action":"set-group","group":"g4"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("缺 key 时应 400，得到 %d", rec.Code)
	}

	// toggle：启用状态翻转。
	rec = do(t, s, http.MethodDelete, "/admin/api/keys?toggle="+mask(created.Key), "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("toggle = %d", rec.Code)
	}
	for _, k := range keysOf(t, bodyRecorder{do(t, s, http.MethodGet, "/admin/api/keys", "", "").Body.Bytes()}) {
		if k.Key == mask(created.Key) && k.Enabled {
			t.Fatal("toggle 未把密钥停用")
		}
	}

	// 删除。
	if rec := do(t, s, http.MethodDelete, "/admin/api/keys?key="+mask(created.Key), "", ""); rec.Code != http.StatusOK {
		t.Fatalf("delete = %d", rec.Code)
	}
	if got := len(keysOf(t, bodyRecorder{do(t, s, http.MethodGet, "/admin/api/keys", "", "").Body.Bytes()})); got != 1 {
		t.Fatalf("删除后密钥数 = %d，期望 1", got)
	}

	// 不支持的方法。
	if rec := do(t, s, http.MethodPut, "/admin/api/keys", "", ""); rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("PUT = %d，期望 405", rec.Code)
	}
}

func TestAdminStatsReadAndReset(t *testing.T) {
	s, _ := newTestServer(t, nil)

	rec := do(t, s, http.MethodGet, "/admin/api/stats", "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /admin/api/stats = %d", rec.Code)
	}
	var st struct {
		TotalRequests int64   `json:"total_requests"`
		Failed        int64   `json:"failed_requests"`
		Chars         int64   `json:"total_chars_out"`
		TokensIn      int64   `json:"total_tokens_in"`
		TokensOut     int64   `json:"total_tokens_out"`
		Cost          float64 `json:"total_cost"`
		StartedAt     int64   `json:"started_at"`
		ResetAt       int64   `json:"reset_at"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &st); err != nil {
		t.Fatalf("统计不是合法 JSON: %v", err)
	}
	if st.ResetAt != 0 {
		t.Fatalf("未重置过时 reset_at 应为 0，得到 %d", st.ResetAt)
	}
	if st.StartedAt == 0 {
		t.Fatal("started_at 不应为 0")
	}

	// 制造一次真实记录，确认计数被写进统计。
	s.Store.Record(store.LogEntry{Status: http.StatusOK, TokensIn: 10, TokensOut: 5, Cost: 0.5}, 100)
	if got := s.Store.Stats(); got.TotalRequests != 1 || got.TotalTokensIn != 10 || got.TotalCost != 0.5 {
		t.Fatalf("统计未计入：%+v", got)
	}

	if rec := do(t, s, http.MethodDelete, "/admin/api/stats", "", ""); rec.Code != http.StatusOK {
		t.Fatalf("DELETE = %d", rec.Code)
	}
	if err := json.Unmarshal(do(t, s, http.MethodGet, "/admin/api/stats", "", "").Body.Bytes(), &st); err != nil {
		t.Fatalf("重置后统计解析失败: %v", err)
	}
	if st.TotalRequests != 0 || st.TokensIn != 0 || st.Cost != 0 {
		t.Fatalf("重置后计数未清零: %+v", st)
	}
	if st.ResetAt == 0 {
		t.Fatal("重置后应写入 reset_at")
	}
	if st.StartedAt == 0 {
		t.Fatal("重置不应清掉 started_at")
	}
}

func TestAdminModelsGetAndRefresh(t *testing.T) {
	u := newUpstream(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != deepai.ModelBasePath+deepai.PathModels {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"models":[
			{"id":"glm-5.3-flash","name":"GLM","input":["text"],"contextWindow":1000,"maxTokens":100},
			{"id":"other-model","name":"Other","input":["text"]}
		],"default":"glm-5.3-flash"}`))
	})
	s, _ := newTestServer(t, u)

	rec := do(t, s, http.MethodGet, "/admin/api/models", "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /admin/api/models = %d", rec.Code)
	}
	var got struct {
		Models    []map[string]any `json:"models"`
		Default   string           `json:"default_model"`
		UpdatedAt int64            `json:"models_updated"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("模型列表不是合法 JSON: %v", err)
	}
	// 尚未刷新过：必须回落到内置兜底模型，与 /v1/models 保持一致。
	if len(got.Models) != 1 || got.Models[0]["id"] != deepai.DefaultModel {
		t.Fatalf("未刷新时应给出兜底模型，得到: %+v", got.Models)
	}
	if got.Default != deepai.DefaultModel {
		t.Fatalf("default_model = %q，期望 %q", got.Default, deepai.DefaultModel)
	}

	// POST 触发拉取：必须以假上游的目录为准。
	rec = do(t, s, http.MethodPost, "/admin/api/models", "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("POST = %d：%s", rec.Code, rec.Body.String())
	}
	var refreshed struct {
		OK    bool             `json:"ok"`
		Count int              `json:"count"`
		List  []map[string]any `json:"models"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &refreshed); err != nil {
		t.Fatalf("刷新响应不是 JSON: %v", err)
	}
	if !refreshed.OK {
		t.Fatalf("刷新未返回 ok: %s", rec.Body.String())
	}
	if refreshed.Count != 2 {
		t.Fatalf("刷新返回 count=%d，期望 2：%s", refreshed.Count, rec.Body.String())
	}
	if len(s.Store.Models()) != 2 {
		t.Fatalf("刷新后目录 = %d 条，期望 2", len(s.Store.Models()))
	}
	if s.Store.DefaultModel() != "glm-5.3-flash" {
		t.Fatalf("默认模型 = %q", s.Store.DefaultModel())
	}

	// 上游失败 → 502 upstream_error。
	failing := newUpstream(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	})
	s2, _ := newTestServer(t, failing)
	rec = do(t, s2, http.MethodPost, "/admin/api/models", "", "")
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("上游失败时 = %d，期望 502", rec.Code)
	}
	var e struct {
		Error struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &e); err != nil {
		t.Fatalf("错误响应不是 JSON: %v", err)
	}
	if e.Error.Type != "upstream_error" || !strings.Contains(e.Error.Message, "刷新模型目录失败") {
		t.Fatalf("错误语义不对: %+v", e.Error)
	}
}
