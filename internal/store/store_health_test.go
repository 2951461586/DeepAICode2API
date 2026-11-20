package store

import (
	"testing"
	"time"

	"deepaicode2api/internal/model"
)

// 这一组锁的是「失败怎么记账」与「错误标记怎么自愈」（docs/changes.md §45）。
//
// 旧实现：任何失败都写 status=error，且只有成功请求 / 成功 /theos 才会清；
// 于是上游概率性 EOF / 429 会让账号在控制台上长期显示不健康，而调度器其实
// 只跳过 blocked —— 展示与事实脱节。

func TestAccountCountsUsesStalenessAndBlocked(t *testing.T) {
	st := newStore(t)
	stale := time.Now().Add(-model.ErrorStaleAfter - time.Minute).Unix()
	fresh := time.Now().Unix()
	st.UpsertAccount(&model.Account{ID: "ok", APIKey: "k-ok", Enabled: true, Status: "ok"})
	st.UpsertAccount(&model.Account{ID: "unknown", APIKey: "k-unknown", Enabled: true, Status: "unknown"})
	st.UpsertAccount(&model.Account{ID: "staleErr", APIKey: "k-staleerr", Enabled: true, Status: model.StatusError, LastErrAt: stale})
	st.UpsertAccount(&model.Account{ID: "freshErr", APIKey: "k-fresherr", Enabled: true, Status: model.StatusError, LastErrAt: fresh})
	st.UpsertAccount(&model.Account{ID: "blocked", APIKey: "k-blocked", Enabled: true, Status: model.StatusBlocked})
	st.UpsertAccount(&model.Account{ID: "off", APIKey: "k-off", Enabled: false, Status: "ok"})

	total, ready := st.AccountCounts()
	if total != 6 {
		t.Fatalf("总数 = %d，期望 6", total)
	}
	// ok + unknown + staleErr = 3；freshErr / blocked / off 不算。
	if ready != 3 {
		t.Fatalf("就绪数 = %d，期望 3（ok/unknown/陈旧 error）", ready)
	}
}
