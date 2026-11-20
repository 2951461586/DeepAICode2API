package store

import (
	"os"
	"testing"
	"time"

	"deepaicode2api/internal/model"
)

// PreferredAccount 是会话粘性的选号入口：只返回仍然可调度的那个号。
//
// 判定必须与调度器一致（启用 / 未拦截 / 额度未用尽 / 分组匹配），否则会出现
// 「绑定到一个已经被上游拦死的号」这种情况，让会话粘性变成持续失败。
func TestPreferredAccountHonorsEligibility(t *testing.T) {
	st := newStore(t)
	st.UpsertAccount(&model.Account{ID: "a1", Name: "a1", APIKey: "k1", Enabled: true, Status: "ok"})
	st.UpsertAccount(&model.Account{ID: "a2", Name: "a2", APIKey: "k2", Enabled: true, Status: "ok", Group: "vip"})

	if got := st.PreferredAccount("a1", ""); got == nil || got.ID != "a1" {
		t.Fatalf("应返回 a1，实际 %+v", got)
	}
	// 分组不匹配：a1 属于 default，请求 vip 分组时不能选它。
	if got := st.PreferredAccount("a1", "vip"); got != nil {
		t.Fatalf("分组不匹配仍返回了 %+v", got)
	}
	if got := st.PreferredAccount("a2", "vip"); got == nil || got.ID != "a2" {
		t.Fatalf("应返回 vip 组的 a2，实际 %+v", got)
	}
	// 不存在的 ID 与空 ID 都返回 nil。
	if got := st.PreferredAccount("missing", ""); got != nil {
		t.Fatalf("不存在的 ID 返回了 %+v", got)
	}
	if got := st.PreferredAccount("", ""); got != nil {
		t.Fatalf("空 ID 返回了 %+v", got)
	}
	// 被拦 / 停用的号不能作为粘性目标。
	// 状态转移已移到 internal/poolsvc；这里是 store 查询用例的前置数据。
	st.UpdateAccount("a1", func(a *model.Account) { a.Status = model.StatusBlocked; a.LastErr = "social_login_required" })
	if got := st.PreferredAccount("a1", ""); got != nil {
		t.Fatalf("blocked 账号仍被返回: %+v", got)
	}
	st.UpdateAccount("a2", func(a *model.Account) { a.Enabled = false })
	if got := st.PreferredAccount("a2", ""); got != nil {
		t.Fatalf("停用账号仍被返回: %+v", got)
	}
}

// SaveSticky / LoadSticky 是会话粘性绑定表的落盘入口：独立旁路文件、
// 与主状态文件同一把主密钥封存，内容未变时不重写。
func TestStickySidecarRoundTrip(t *testing.T) {
	st := newStore(t)
	want := map[string]StickyBinding{
		"aa1-deadbeef": {AccountID: "a1", At: time.Now().Unix()},
	}
	if err := st.SaveSticky(want); err != nil {
		t.Fatalf("SaveSticky: %v", err)
	}
	if _, err := os.Stat(st.StickyPath()); err != nil {
		t.Fatalf("绑定表文件不存在: %v", err)
	}
	got, err := st.LoadSticky()
	if err != nil {
		t.Fatalf("LoadSticky: %v", err)
	}
	if len(got) != 1 || got["aa1-deadbeef"].AccountID != "a1" {
		t.Fatalf("绑定表往返不一致: %#v", got)
	}
	// 再把主状态文件重开一次：绑定表必须还在（不依赖主文件）。
	reopened, err := Open(st.Path())
	if err != nil {
		t.Fatalf("重开状态文件: %v", err)
	}
	got2, err := reopened.LoadSticky()
	if err != nil {
		t.Fatalf("重开后 LoadSticky: %v", err)
	}
	if got2["aa1-deadbeef"].AccountID != "a1" {
		t.Fatalf("重开后绑定丢失: %#v", got2)
	}
}
