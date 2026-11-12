package server

import (
	"deepaicode2api/internal/deepai"
	"deepaicode2api/internal/store"
	"sync/atomic"
	"time"
)

// record 记录一次请求元信息。
// reqLog 是一次请求的日志上下文。
//
// 旧版 record() 收 11 个位置参数（key/acct/model/endpoint/stream/status/
// start/inLen/outLen/usage/err），二十余处调用完全靠记忆参数顺序，
// 写错编译器也不报错。现在把不变量在构造时固化，调用点只补变化的部分。
type reqLog struct {
	s        *Server
	key      string
	acct     *deepai.Account
	model    string
	endpoint string
	stream   bool
	start    time.Time
	inLen    int

	// state 是「是否已记录」的共享标记。reqLog 是值类型（各 handler 按值传递），
	// 所以标记必须放在指针后面：done 可能被不同分支各调一次，只有第一次生效
	// 才不会把同一个请求记成两条日志。
	state *reqLogState
}

// reqLogState 是 reqLog 的跨副本共享状态。
type reqLogState struct {
	recorded atomic.Bool
}

// newReqLog 固化一次请求的公共字段。acct 可以为 nil（尚未选到账号）。
func (s *Server) newReqLog(key string, acct *deepai.Account, model, endpoint string, stream bool, start time.Time, inLen int) reqLog {
	return reqLog{
		s: s, key: key, acct: acct, model: model,
		endpoint: endpoint, stream: stream, start: start, inLen: inLen,
		state: &reqLogState{},
	}
}

// done 落一条请求日志；err 为 nil 视为成功（HTTP 200），否则按错误取状态码。
//
// 幂等：同一个请求只会被记一次。这样「某条提前返回的分支忘了记 / 加了一层 defer
// 兜底」都不会造成重复计数，调用方也不必先判断有没有记过。
func (l reqLog) done(outLen int, usage Usage, err error) {
	l.doneStatus(statusOf(err), outLen, usage, err)
}

// doneStatus 与 done 相同，但状态码由调用方给出。
//
// 用于「响应已按某个码写给客户端，日志还需额外说明原因」的场景：典型是上游回的
// 不是 JSON、我们按原样透传成 200 —— 日志里要同时保留 200（客户端实际收到）
// 与失败原因（Record 会把 e.Err != "" 计入失败数，不会丢出统计）。
func (l reqLog) doneStatus(status int, outLen int, usage Usage, err error) {
	if l.s == nil || l.state == nil {
		return
	}
	if !l.state.recorded.CompareAndSwap(false, true) {
		return
	}
	e := store.LogEntry{
		Time:         time.Now().Unix(),
		Key:          mask(l.key),
		Model:        l.model,
		Endpoint:     l.endpoint,
		Stream:       l.stream,
		Status:       status,
		LatencyMS:    time.Since(l.start).Milliseconds(),
		PromptLen:    l.inLen,
		OutputLen:    outLen,
		TokensIn:     usage.PromptTokens,
		TokensOut:    usage.CompletionTokens,
		CachedTokens: usage.CachedTokens,
		Cost:         usage.Cost,
	}
	if l.acct != nil {
		e.Account = l.acct.ID
	}
	if err != nil {
		e.Err = err.Error()
	}
	l.s.Store.Record(e, outLen)
}
