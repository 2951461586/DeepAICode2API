package server

import (
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"deepaicode2api/internal/deepai"
	"deepaicode2api/internal/gateway"
	"deepaicode2api/internal/guard"
	"deepaicode2api/internal/logx"
	"deepaicode2api/internal/respstate"
	"deepaicode2api/internal/store"
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
	// inMsgs / hasSys 是回喂历史的规模（消息数 / 是否带系统提示），不含正文。
	inMsgs int
	hasSys bool
	// outBytes 是实际发往上游的请求体字节数（0 = 未统计）。
	//
	// 与 inMsgs 配合回答 issue #5 / #7 的三个问题：客户端没给历史、网关弄丢了
	// 历史、请求体太长被上游静默砍头（实测 ~196 KB 起）。
	outBytes int

	// ua / ip 是调用方标识（见 withClient）。留着是为了能回答「谁在发」。
	ua string
	ip string

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

// withHistory 记录回喂历史的规模（消息数 / 是否带系统提示）。
//
// 不含任何正文：只用于在日志里区分「客户端没给历史」与「网关把历史弄丢了」。
func (l reqLog) withHistory(payload map[string]any) reqLog {
	l.inMsgs, l.hasSys = historyStats(payload)
	return l
}

// withOutboundBytes 记录实际发往上游的请求体字节数。
//
// 在“已经序列化好 body” 的路径上传 len(body)；不要在日志里落正文。
func (l reqLog) withOutboundBytes(n int) reqLog {
	l.outBytes = n
	return l
}

// withClient 记录调用方标识（UA 与真实来源 IP）。
//
// 线上链路是 Cloudflare → nginx → 网关，RemoteAddr 永远是回环或边缘地址，只有
// 转发头才认得出真正的调用方，所以优先取 CF-Connecting-IP / X-Forwarded-For 首段，
// 都没有才退回 RemoteAddr。UA 截前 120 字符：它有客户端自己拼的超长串，也有整段
// 被塞进 header 的异常情况，不能当作无限长的字段落盘。
func (l reqLog) withClient(r *http.Request) reqLog {
	if r == nil {
		return l
	}
	l.ua = truncateRunes(strings.TrimSpace(r.Header.Get("User-Agent")), 120)
	l.ip = clientAddr(r)
	return l
}

// clientAddr 取真实客户端地址（按可信度从高到低回退）。
func clientAddr(r *http.Request) string {
	if v := strings.TrimSpace(r.Header.Get("CF-Connecting-IP")); v != "" {
		return truncateRunes(v, 64)
	}
	if v := r.Header.Get("X-Forwarded-For"); v != "" {
		if first := strings.TrimSpace(strings.Split(v, ",")[0]); first != "" {
			return truncateRunes(first, 64)
		}
	}
	return truncateRunes(guard.ClientIP(r.RemoteAddr), 64)
}

// truncateRunes 按字符（而非字节）截断，避免把 UTF-8 串成乱码。
func truncateRunes(s string, n int) string {
	if n <= 0 || len(s) <= n {
		return s
	}
	rs := []rune(s)
	if len(rs) <= n {
		return s
	}
	return string(rs[:n])
}

// done 落一条请求日志；err 为 nil 视为成功（HTTP 200），否则按错误取状态码。
//
// 幂等：同一个请求只会被记一次。这样「某条提前返回的分支忘了记 / 加了一层 defer
// 兜底」都不会造成重复计数，调用方也不必先判断有没有记过。
func (l reqLog) done(outLen int, usage gateway.Usage, err error) {
	l.doneStatus(StatusOf(err), outLen, usage, err)
}

// doneStatus 与 done 相同，但状态码由调用方给出。
//
// 用于「响应已按某个码写给客户端，日志还需额外说明原因」的场景：典型是上游回的
// 不是 JSON、我们按原样透传成 200 —— 日志里要同时保留 200（客户端实际收到）
// 与失败原因（Record 会把 e.Err != "" 计入失败数，不会丢出统计）。
func (l reqLog) doneStatus(status int, outLen int, usage gateway.Usage, err error) {
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
		Messages:     l.inMsgs,
		Sys:          l.hasSys,
		TokensIn:     usage.PromptTokens,
		TokensOut:    usage.CompletionTokens,
		CachedTokens: usage.CachedTokens,
		Cost:         usage.Cost,
	}
	if l.acct != nil {
		e.Account = l.acct.ID
	}
	if l.ua != "" {
		e.UA = l.ua
	}
	if l.ip != "" {
		e.IP = l.ip
	}
	if l.outBytes > 0 {
		e.BodyBytes = l.outBytes
		// 90% 阈值：留一点余量给重试时补的 system 消息 / 报错前的重试。
		if limit := respstate.ContextBodyBytes(); limit > 0 && l.outBytes >= limit*9/10 {
			e.BodyNearCap = true
			logx.Warn("上游请求体接近静默截断阈值（issue #7）",
				"endpoint", l.endpoint, "model", l.model,
				"bytes", l.outBytes, "limit", limit)
		}
	}
	if err != nil {
		e.Err = err.Error()
	}
	l.s.Store.Record(e, outLen)
}

// apiKey 返回本次请求的调用密钥。
//
// 只供形状记忆 / 指标这类进程内逻辑使用；日志与统计落盘走 key 的脱敏视图，
// 不要把完整密钥写进日志。
func (l reqLog) apiKey() string { return l.key }
