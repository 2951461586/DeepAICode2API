package store

import (
	"time"

	"deepaicode2api/internal/model"
)

// 测试夹具：造「已经记过账」的账号数据。
//
// 状态转移（一次失败该不该标 error、探测失败退避多久）已经搬到
// internal/poolsvc/accountstate.go，并由那边的用例覆盖。这里只提供最小的
// 造数能力，让 store 自己的查询 / 选号 / 落盘用例能摆出前置状态 ——
// 不复制任何业务判定。

func markResult(st *Store, id string, ok bool, note string) {
	st.UpdateAccount(id, func(a *model.Account) {
		now := time.Now().Unix()
		a.LastUsed = now
		if ok {
			a.Failures = 0
			a.Status = "ok"
			a.LastErr = ""
			a.LastErrAt = 0
			a.QuotaUntil = 0
			return
		}
		a.Failures++
		a.Status = model.StatusError
		a.LastErr = note
		a.LastErrAt = now
	})
}

func markFailure(st *Store, id string, err error) {
	if err == nil {
		return
	}
	markResult(st, id, false, err.Error())
}

// setAllowance 缓存一次配额结果（只用于成功路径的造数）。
func setAllowance(st *Store, id string, al *model.Allowance, err error) {
	if err != nil {
		return // 失败路径的记账规则在 poolsvc 侧，这里只负责成功造数
	}
	st.UpdateAccount(id, func(a *model.Account) {
		a.LastUsed = time.Now().Unix()
		a.Theos = al
		a.Status = "ok"
		a.LastErr = ""
		a.LastErrAt = 0
		a.Failures = 0
		a.ProbeFailures = 0
		a.ProbeAfter = 0
	})
}
