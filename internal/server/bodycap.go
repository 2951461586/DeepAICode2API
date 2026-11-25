package server

import (
	"fmt"

	"deepaicode2api/internal/respstate"
	"deepaicode2api/internal/store"
)

// bodyCapError 表示请求体撞上网关侧安全上限，且控制台把处置设成了 reject。
//
// 单独成一个错误类型（而不是普通 error）的原因：它必须在三处被识别成 400，
// 而不是被当成上游故障：
//   - writeUpstreamErr / writeAnthropicUpstreamErr（回给客户端的文案）
//   - StatusOf（请求日志里的状态码）
//
// 否则控制台会把这些请求记成 502，与真正的代理故障混在一起。
type bodyCapError struct {
	Bytes int
	Limit int
}

func (e *bodyCapError) Error() string {
	return fmt.Sprintf("请求体 %d 字节超过网关安全上限 %d 字节：上游在约 196 KB 起会静默丢掉开头的历史"+
		"（issue #7，usage.prompt_tokens 钉在约 40k）。请压缩对话历史 / 减少 tools，"+
		"或由管理员把设置里的「请求体超限处置」改回 warn 再透传", e.Bytes, e.Limit)
}

// bodyCapReached 判断本次出站请求体是否超限且需要拒绝。
//
// 模式语义（settings.BodyCapMode）：
//   - warn（默认）：不拦，只在达到 90% 时由 reqlog 打 body_near_cap 标记 + 告警；
//   - reject：超过上限直接 400，不把一个已知会被上游截断的请求送上去。
//
// limit <= 0 表示关闭这道闸门（ContextBodyBytes 支持显式关闭）。
func bodyCapReached(settings store.Settings, body []byte) error {
	if settings.BodyCapMode != store.BodyCapReject {
		return nil
	}
	limit := respstate.ContextBodyBytes()
	if limit <= 0 || len(body) <= limit {
		return nil
	}
	return &bodyCapError{Bytes: len(body), Limit: limit}
}
