package poolsvc

import (
	"strconv"
	"testing"
	"time"

	"deepaicode2api/internal/store"
)

// --- 会话粘性 ---
//
// 从 internal/server/protocol_fixes_test.go 迁入：绑定表已随 PR-4 搬进
// poolsvc，按 docs/server-split-plan.md §4.1「单元测试跟着代码走」留在
// poolsvc；内核只保留一个「重启后仍生效」的端到端用例（那是 HTTP 契约）。

// TestStickyBindAndDrop 覆盖绑定的建立与定向解除。
func TestStickyBindAndDrop(t *testing.T) {
	p := New(Config{})

	p.BindSticky("sess", "acc-9")
	if got := p.StickyAccount("sess"); got != "acc-9" {
		t.Fatalf("绑定 = %q，期望 acc-9", got)
	}
	// 不匹配的账号不应误删：重试链上可能已经把会话改绑到别的号。
	p.DropSticky("sess", "acc-other")
	if got := p.StickyAccount("sess"); got != "acc-9" {
		t.Fatalf("DropSticky 误删了别的账号的绑定: %q", got)
	}
	p.DropSticky("sess", "acc-9")
	if got := p.StickyAccount("sess"); got != "" {
		t.Fatalf("DropSticky 后仍有绑定: %q", got)
	}
}

// TestStickyEmptyInputsIgnored 验证空 affinity / 空账号不产生垃圾绑定。
func TestStickyEmptyInputsIgnored(t *testing.T) {
	p := New(Config{})
	p.BindSticky("", "acc-0")
	p.BindSticky("sess", "")
	if n := p.Bindings(); n != 0 {
		t.Fatalf("空参数不应产生绑定，实际 %d 条", n)
	}
	if got := p.StickyAccount(""); got != "" {
		t.Fatalf("空 affinity 应返回空串，实际 %q", got)
	}
}

// TestStickyTableBounded 保证绑定表不会随 affinity 数量无限增长。
func TestStickyTableBounded(t *testing.T) {
	p := New(Config{})
	for i := 0; i < StickyMax*2; i++ {
		p.BindSticky("k-"+strconv.Itoa(i), "acc-0")
	}
	if n := p.Bindings(); n > StickyMax {
		t.Fatalf("绑定表 = %d，超过上限 %d", n, StickyMax)
	}
}

// TestStickyGCKeepsRecentWhenFull 验证超限时优先回收过期项，而不是整表清空。
func TestStickyGCKeepsRecentWhenFull(t *testing.T) {
	p := New(Config{})
	old := time.Now().Add(-2 * StickyTTL).Unix()

	p.stickyMu.Lock()
	for i := 0; i < StickyMax; i++ {
		p.sticky["old-"+strconv.Itoa(i)] = store.StickyBinding{AccountID: "acc-0", At: old}
	}
	p.stickyMu.Unlock()

	p.BindSticky("fresh", "acc-1")
	if got := p.StickyAccount("fresh"); got != "acc-1" {
		t.Fatalf("新绑定应保留，实际 %q", got)
	}
	if n := p.Bindings(); n != 1 {
		t.Fatalf("过期项应被回收，表内应只剩 1 条，实际 %d", n)
	}
}

// TestStickyDirtyFlushLifecycle 验证攒批写盘：无改动时 Flush 不写。
func TestStickyDirtyFlushLifecycle(t *testing.T) {
	st := newTestStore(t)
	p := newService(t, st, nil)

	// 无改动：FlushSticky 是空操作（dirty=false）。
	p.FlushSticky()
	if p.stickyDirty.Load() {
		t.Fatal("无改动时不应标脏")
	}

	p.BindSticky("sess", "acc-0")
	if !p.stickyDirty.Load() {
		t.Fatal("绑定后应标脏")
	}
	p.FlushSticky()
	if p.stickyDirty.Load() {
		t.Fatal("Flush 后应清脏")
	}

	// 落盘内容可被重新载入。
	got, err := st.LoadSticky()
	if err != nil {
		t.Fatalf("LoadSticky: %v", err)
	}
	if got["sess"].AccountID != "acc-0" {
		t.Fatalf("落盘内容 = %+v，期望 acc-0", got)
	}
}

// TestStickyLoadSkipsExpiredAndEmpty 验证启动时剔除过期 / 残缺绑定。
func TestStickyLoadSkipsExpiredAndEmpty(t *testing.T) {
	st := newTestStore(t)
	if err := st.SaveSticky(map[string]store.StickyBinding{
		"old":   {AccountID: "acc-0", At: time.Now().Add(-2 * StickyTTL).Unix()},
		"new":   {AccountID: "acc-1", At: time.Now().Unix()},
		"empty": {AccountID: "", At: time.Now().Unix()},
		"":      {AccountID: "acc-2", At: time.Now().Unix()},
	}); err != nil {
		t.Fatalf("SaveSticky: %v", err)
	}

	p := newService(t, st, nil)
	p.LoadSticky()

	if got := p.StickyAccount("old"); got != "" {
		t.Fatalf("过期绑定不应恢复，实际 %q", got)
	}
	if got := p.StickyAccount("new"); got != "acc-1" {
		t.Fatalf("有效绑定未恢复，实际 %q", got)
	}
	if n := p.Bindings(); n != 1 {
		t.Fatalf("应只恢复 1 条绑定，实际 %d", n)
	}
}

// TestStickyLoadMissingFileIsNotFatal 验证旁路文件不存在时按空表启动。
func TestStickyLoadMissingFileIsNotFatal(t *testing.T) {
	// 首次启动：旁路文件还不存在。
	// （读取失败只记日志，绑定表按空表启动。）
	st2 := newTestStore(t)
	p2 := newService(t, st2, nil)
	p2.LoadSticky()
	if n := p2.Bindings(); n != 0 {
		t.Fatalf("空文件应得到 0 条绑定，实际 %d", n)
	}

	// 绑定仍可在内存里工作（落盘失败不影响正确性）。
	p2.BindSticky("sess", "acc-0")
	if got := p2.StickyAccount("sess"); got != "acc-0" {
		t.Fatalf("内存绑定应生效，实际 %q", got)
	}
}
