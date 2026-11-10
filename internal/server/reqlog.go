package server

import (
	"deepaicode2api/internal/deepai"
	"deepaicode2api/internal/store"
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
}

// newReqLog 固化一次请求的公共字段。acct 可以为 nil（尚未选到账号）。
func (s *Server) newReqLog(key string, acct *deepai.Account, model, endpoint string, stream bool, start time.Time, inLen int) reqLog {
	return reqLog{
		s: s, key: key, acct: acct, model: model,
		endpoint: endpoint, stream: stream, start: start, inLen: inLen,
	}
}

// done 落一条请求日志；err 为 nil 视为成功（HTTP 200），否则记为 502。
func (l reqLog) done(outLen int, usage Usage, err error) {
	e := store.LogEntry{
		Time:      time.Now().Unix(),
		Key:       mask(l.key),
		Model:     l.model,
		Endpoint:  l.endpoint,
		Stream:    l.stream,
		Status:    statusOf(err),
		LatencyMS: time.Since(l.start).Milliseconds(),
		PromptLen: l.inLen,
		OutputLen: outLen,
		TokensIn:  usage.PromptTokens,
		TokensOut: usage.CompletionTokens,
		Cost:      usage.Cost,
	}
	if l.acct != nil {
		e.Account = l.acct.ID
	}
	if err != nil {
		e.Err = err.Error()
	}
	l.s.Store.Record(e, outLen)
}
