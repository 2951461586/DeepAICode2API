package deepai

import "testing"

func TestModelPredicates(t *testing.T) {
	cases := []struct {
		name          string
		m             Model
		wantResponses bool
		wantFree      bool
		wantImage     bool
	}{
		{
			name:      "无 TheosPerRequest 且非 premier → 免费",
			m:         Model{ID: "a", Input: []string{"text"}},
			wantFree:  true,
			wantImage: false,
		},
		{
			name:      "premier 但每次 0 Theos 仍算免费",
			m:         Model{ID: "b", Premier: true, TheosPerRequest: "0"},
			wantFree:  true,
			wantImage: false,
		},
		{
			name:      "TheosPerRequest=0.00 也是免费",
			m:         Model{ID: "c", TheosPerRequest: "0.00"},
			wantFree:  true,
			wantImage: false,
		},
		{
			name:          "responses 模型 + 图片输入",
			m:             Model{ID: "d", API: "openai-responses", Input: []string{"text", "image"}},
			wantResponses: true,
			wantFree:      true, // Premier 为 false
			wantImage:     true,
		},
		{
			name:      "收费模型带图片",
			m:         Model{ID: "e", Premier: true, TheosPerRequest: "0.5", Input: []string{"image"}},
			wantFree:  false,
			wantImage: true,
		},
	}
	for _, c := range cases {
		if got := c.m.IsResponses(); got != c.wantResponses {
			t.Errorf("%s: IsResponses()=%v，期望 %v", c.name, got, c.wantResponses)
		}
		if got := c.m.Free(); got != c.wantFree {
			t.Errorf("%s: Free()=%v，期望 %v", c.name, got, c.wantFree)
		}
		if got := c.m.SupportsImage(); got != c.wantImage {
			t.Errorf("%s: SupportsImage()=%v，期望 %v", c.name, got, c.wantImage)
		}
	}
}

func TestAllowanceCloneIsDeepCopy(t *testing.T) {
	if got := (*Allowance)(nil).Clone(); got != nil {
		t.Fatalf("nil Allowance Clone() 应为 nil，得到 %+v", got)
	}

	src := &Allowance{Enabled: true, Balance: "12", CheckIn: &CheckIn{Nonce: "n1", Seconds: 600}}
	cp := src.Clone()
	if cp == src {
		t.Fatal("Clone 不应返回同一指针")
	}
	cp.Balance = "0"
	cp.CheckIn.Nonce = "mutated"
	if src.Balance != "12" || src.CheckIn.Nonce != "n1" {
		t.Fatalf("修改副本污染了源对象: %+v / %+v", src, src.CheckIn)
	}
}

func TestAccountCloneIsDeepCopy(t *testing.T) {
	if got := (*Account)(nil).Clone(); got != nil {
		t.Fatalf("nil Account Clone() 应为 nil，得到 %+v", got)
	}

	src := &Account{ID: "id1", APIKey: "sk-1", Theos: &Allowance{Balance: "9"}}
	cp := src.Clone()
	if cp == src {
		t.Fatal("Clone 不应返回同一指针")
	}
	cp.APIKey = "sk-2"
	cp.Theos.Balance = "0"
	if src.APIKey != "sk-1" || src.Theos.Balance != "9" {
		t.Fatalf("修改副本污染了源账号: %+v / %+v", src, src.Theos)
	}
}

// TestAccountMergeFrom 固化 MergeFrom 的两条策略：
// 运行态以旧账号为准，身份字段只在新账号为空时回落。
func TestAccountMergeFrom(t *testing.T) {
	var nilAcct *Account
	nilAcct.MergeFrom(&Account{}) // 不应 panic
	(&Account{}).MergeFrom(nil)   // 不应 panic

	existing := &Account{
		ID:            "keep-id",
		Name:          "old-name",
		Failures:      3,
		Status:        "error",
		LastErr:       "boom",
		LastUsed:      42,
		Note:          "old-note",
		ProbeAfter:    1000,
		ProbeFailures: 2,
		Theos:         &Allowance{Balance: "5"},
		Group:         "g2",
		Proxy:         "http://proxy:8080",
		MachineID:     "machine-old",
		UserAgent:     "UA-old",
		Platform:      "win",
		Email:         "old@example.com",
		Password:      "old-pw",
		Fingerprint:   "fp-old",
		RegisteredAt:  777,
	}
	// 新注册结果：只有部分字段非空。
	acct := &Account{
		Name:           "new-name",
		Username:       "new-user",
		APIKey:         "sk-new",
		Email:          "new@example.com",
		Group:          "", // 空 → 回落到旧分组
		MachineID:      "",
		Proxy:          "",
		UserAgent:      "",
		SecCHUA:        "chromium",
		AcceptLanguage: "zh-CN",
	}

	acct.MergeFrom(existing)

	if acct.ID != "keep-id" || acct.Failures != 3 || acct.Status != "error" ||
		acct.LastErr != "boom" || acct.LastUsed != 42 ||
		acct.ProbeAfter != 1000 || acct.ProbeFailures != 2 || acct.Theos != existing.Theos {
		t.Fatalf("运行态未被旧账号覆盖: %+v", acct)
	}
	if acct.Group != "g2" || acct.MachineID != "machine-old" || acct.Proxy != "http://proxy:8080" ||
		acct.UserAgent != "UA-old" || acct.Fingerprint != "fp-old" ||
		acct.Password != "old-pw" || acct.RegisteredAt != 777 {
		t.Fatalf("空身份字段未回落到旧账号: %+v", acct)
	}
	if acct.Name != "new-name" || acct.Email != "new@example.com" {
		t.Fatalf("非空身份字段被旧值覆盖: name=%q email=%q", acct.Name, acct.Email)
	}
	if acct.SecCHUA != "chromium" || acct.AcceptLanguage != "zh-CN" {
		t.Fatalf("新账号自带字段丢失: %+v", acct)
	}
	if acct.APIKey != "sk-new" || acct.Username != "new-user" {
		t.Fatalf("MergeFrom 不应改写密钥与用户名: %+v", acct)
	}
}

func TestAccountGroupName(t *testing.T) {
	cases := []struct {
		name string
		acct *Account
		want string
	}{
		{"nil 账号", nil, "default"},
		{"空分组", &Account{}, "default"},
		{"纯空白", &Account{Group: "  \t "}, "default"},
		{"已命名", &Account{Group: "g2"}, "g2"},
		{"去空白", &Account{Group: "  g3  "}, "g3"},
	}
	for _, c := range cases {
		if got := c.acct.GroupName(); got != c.want {
			t.Errorf("%s: GroupName()=%q，期望 %q", c.name, got, c.want)
		}
	}
}

func TestAccountKeyAndSessionID(t *testing.T) {
	a := &Account{APIKey: "sk-abcdef"}
	if got, want := a.KeyID(), KeyFingerprint("sk-abcdef"); got != want {
		t.Fatalf("KeyID()=%q，期望 %q", got, want)
	}
	if got, want := a.SessionID(), ShortHash("sk-abcdef"); got != want {
		t.Fatalf("SessionID()=%q，期望 %q", got, want)
	}
	if len(a.KeyID()) != 16 || len(a.SessionID()) != 8 {
		t.Fatalf("标识长度异常: key=%q session=%q", a.KeyID(), a.SessionID())
	}

	// 两侧空白不应影响指纹（Key 从文件/剪贴板复制时常见）。
	if (&Account{APIKey: "  sk-abcdef  "}).KeyID() != a.KeyID() {
		t.Fatal("KeyID 应先 TrimSpace")
	}
	if (&Account{APIKey: "sk-other"}).KeyID() == a.KeyID() {
		t.Fatal("不同 Key 不应产生相同指纹")
	}
}

func TestDefaultModelEntry(t *testing.T) {
	m := DefaultModelEntry()
	if m.ID != DefaultModel {
		t.Fatalf("ID=%q，期望 %q", m.ID, DefaultModel)
	}
	if m.Name == "" || m.ContextWindow <= 0 || m.MaxTokens <= 0 {
		t.Fatalf("兜底条目缺少展示字段: %+v", m)
	}
	if !m.Reasoning {
		t.Fatal("兜底模型应标记 reasoning")
	}
	if !m.SupportsImage() {
		t.Fatal("兜底模型应支持图片输入")
	}
	if m.IsResponses() {
		t.Fatal("兜底模型走 chat/completions，不应是 responses")
	}
}
