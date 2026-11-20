package server

import (
	"testing"
	"time"

	"deepaicode2api/internal/deepai"
	"deepaicode2api/internal/store"
)

// TestRuntimeConfigIsPulled 锁定「改设置后已缓存的客户端立刻看到新值」。
//
// 这是把推送式改成拉取式要保住的行为：以前靠 applyRuntimeSettings 遍历
// 所有客户端 Set，新建客户端时还要在 clientFor 里再补一次；现在客户端
// 每次请求现取，只要 Server.runtimeConfig 读的是设置即可。用例直接拿
// clientFor 缓存出来的代理客户端验证，避免只测到 s.Up。
func TestRuntimeConfigIsPulled(t *testing.T) {
	s, _ := newTestServer(t, nil)

	// 造一个有代理的账号：clientFor 会为它缓存一个独立客户端。
	acct := &deepai.Account{ID: "proxy-acc", APIKey: "sk-p", Proxy: "http://127.0.0.1:9"}
	c := s.clientFor(acct)
	if c == nil {
		t.Fatal("clientFor 返回 nil")
	}
	if c.Runtime == nil {
		t.Fatal("代理客户端应挂上 Runtime 源（否则仍是推送式，漏改就会不一致）")
	}

	s.Store.UpdateSettings(func(st *store.Settings) { st.CheckinBlockMS = 1234 })
	if got := c.Runtime().CheckInBudget; got != 1234*time.Millisecond {
		t.Fatalf("签到预算 = %v，期望 1.234s（拉取式应立即生效）", got)
	}

	// 车道：CurrentLane 只在 LaneBuild 与编译期一致时才无条件信任探测值。
	s.Store.UpdateSettings(func(st *store.Settings) {
		st.Lane = "6"
		st.LaneBuild = deepai.LaneVersion
	})
	if got := c.Runtime().Lane; got != "6" {
		t.Fatalf("车道 = %q，期望 6（拉取式应立即生效）", got)
	}

	// 直连客户端与代理客户端必须看到同一份值。
	if got := s.Up.Runtime().Lane; got != "6" {
		t.Fatalf("直连客户端车道 = %q，期望与代理客户端一致", got)
	}
}
