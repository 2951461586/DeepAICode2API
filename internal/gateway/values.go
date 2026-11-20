package gateway

import (
	"crypto/rand"
	"encoding/hex"
	"strconv"
	"time"

	"deepaicode2api/internal/jsonx"
)

// newID 生成 <prefix>_<24 hex> 形式的协议级 ID。
//
// 只用于「客户端与上游都能接受的占位 id」（msg_ / resp_ / rs_ / fc_）：
// 它们不参与去重、也不要求全球唯一，12 字节随机足够。
//
// 特意不调 deepai.RandomHex：协议转换层不该为了一个随机串依赖「上游客户端
// 包」（那会把 deepai 的传输/账号语义带进纯函数层）。随机源失败时退化成
// 时间戳 —— id 只用于对账展示，不能因此中断响应。
func newID(prefix string) string {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		return prefix + "_" + strconv.FormatInt(time.Now().UnixNano(), 16)
	}
	return prefix + "_" + hex.EncodeToString(b[:])
}

// 这一层只做「JSON 里取值」的宽松转换：上游字段类型不稳定，
// 拿不到就当零值，绝不因为一个字段形状不对让整轮请求失败。

// ClientError 是「请求本身不合法」的错误（映射成 4xx）。
//
// 与上游错误区分：它不由 deepai.HTTPError 表达，调用方据此决定是
// 回 400 还是回 502。
type ClientError struct{ Message string }

func (e *ClientError) Error() string { return e.Message }

// ErrNoMessages 表示转换后没有任何可用消息。
var ErrNoMessages = &ClientError{Message: "input 不能为空"}

// StatefulParamError 表示请求依赖**上游保存的响应状态**。
//
// 典型字段：`previous_response_id`（续接上一轮）、`conversation`（服务端会话）、
// 以及 `input` 里的 `item_reference`（引用一个已存条目）。
// 这类请求只能由上游 /responses 完成；网关把 Responses 翻译成 Chat Completions
// 时无法重建那份状态，**静默忽略字段就等于把对话截成一问一答**（用户感知为
// “模型失忆”），所以必须显式报错。
type StatefulParamError struct {
	// Param 是触发拒绝的字段名。
	Param string
}

func (e *StatefulParamError) Error() string {
	return "`" + e.Param + "` 依赖上游保存的响应状态，但该模型走的是 chat/completions " +
		"合成路径（无状态）：请把完整历史放进 `input` 重发，或改用 api=openai-responses 的模型"
}

// StatefulParam 返回请求里第一个「需要上游状态」的字段名（没有则返回空串）。
//
// 只认真正携带状态的形态：空字符串的 `previous_response_id` / 空 `conversation`
// 与不传等价，不能因为客户端习惯性填了 null 就拒绝。
func StatefulParam(payload map[string]any) string {
	if jsonx.StrOf(payload["previous_response_id"]) != "" {
		return "previous_response_id"
	}
	switch c := payload["conversation"].(type) {
	case string:
		if c != "" {
			return "conversation"
		}
	case map[string]any:
		if len(c) > 0 {
			return "conversation"
		}
	}
	if items, ok := payload["input"].([]any); ok {
		for _, it := range items {
			if m, ok := it.(map[string]any); ok && jsonx.StrOf(m["type"]) == "item_reference" {
				return "item_reference"
			}
		}
	}
	return ""
}
