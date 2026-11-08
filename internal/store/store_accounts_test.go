package store

import (
	"strings"
	"testing"

	"deepaicode2api/internal/deepai"
)

// NoteAccountIssue 只写备注：不许改状态、不许累加失败次数。
//
// check_in_refused 这类错误下账号仍可推理（只是落在替身模型分支），
// 标成 error 会把好号踢出调度。
func TestNoteAccountIssueKeepsStatus(t *testing.T) {
	st := newStore(t)
	st.UpsertAccount(&deepai.Account{ID: "a1", APIKey: "k1", Enabled: true, Status: "ok"})
	st.SetAccountAllowance("a1", &deepai.Allowance{Enabled: true}, nil)
	if err := st.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	st.NoteAccountIssue("a1", "check_in_refused: too many restarts today")

	a := st.Accounts()[0]
	if a.Status != "ok" {
		t.Fatalf("状态不应变化，实际 %q", a.Status)
	}
	if a.Failures != 0 {
		t.Fatalf("失败计数不应增加，实际 %d", a.Failures)
	}
	if !strings.Contains(a.LastErr, "check_in_refused") {
		t.Fatalf("备注未写入: %q", a.LastErr)
	}
	if a.Theos == nil {
		t.Fatal("额度快照不应被清掉")
	}
}

// 未知账号不 panic、不产生副作用。
func TestNoteAccountIssueUnknown(t *testing.T) {
	st := newStore(t)
	st.NoteAccountIssue("nope", "x")
	if got := st.Accounts(); len(got) != 0 {
		t.Fatalf("不应新建账号: %+v", got)
	}
}
