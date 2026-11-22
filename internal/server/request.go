package server

import (
	"crypto/sha256"
	"deepaicode2api/internal/deepai"
	"deepaicode2api/internal/jsonx"
	"encoding/hex"
	"encoding/json"
	"hash"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// 入站请求体上限已移到 httpx.MaxRequestBody（由 httpx.ReadBodyWithLimit 使用）。

// affinityPattern 与桌面端一致：只透传安全字符集的会话标识。
var affinityPattern = regexp.MustCompile(`^[A-Za-z0-9._:-]{1,256}$`)

// affinityFor 解析会话粘性标识（响应头优先，其次 prompt_cache_key）。
func affinityFor(r *http.Request, payload map[string]any) string {
	if r != nil {
		if v := strings.TrimSpace(r.Header.Get("X-Session-Affinity")); affinityPattern.MatchString(v) {
			return v
		}
	}
	if payload != nil {
		if v, ok := payload["prompt_cache_key"].(string); ok && affinityPattern.MatchString(strings.TrimSpace(v)) {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// sessionAffinity 解析会话粘性标识：调用方显式给的最优先，否则从稳定前缀派生。
//
// 两类客户端都不带会话头：
//   - Claude Code（Anthropic `/v1/messages`）不发 `X-Session-Affinity`，也不送
//     `prompt_cache_key`，只在 system / tools 上打 `cache_control`；
//   - 多数 OpenAI SDK 调用方不送 `prompt_cache_key`。
//
// 派生键 = 模型 + 系统提示 + tools（**不含对话正文**：会话每轮都在增长，
// 含进去会让 id 每轮都变，粘性与缓存分区都会失效）。兼容两种请求形状：
// chat 的 `messages` 与 Responses 的 `instructions` / `input`。
//
// 同前缀 → 同一把 affinity → 上游前缀缓存落在同一个分区（实测换 key 即 miss），
// 选号层据此把同一会话钉在同一个账号上（见 sticky.go）。
// 只当 affinity 用（→ `X-Session-Affinity` 头），不改请求体。
func sessionAffinity(r *http.Request, payload map[string]any) string {
	if a := affinityFor(r, payload); a != "" {
		return a
	}
	h := sha256.New()
	h.Write([]byte(jsonx.StrOf(payload["model"])))
	// 只有「有可缓存前缀」的请求才派生：没有 system / tools 的裸请求
	// （例如一句 "hi"）本来就没有前缀可命中，派生只会把大量互不相关的
	// 短请求钉到同一个号上，白白把负载集中。
	hasPrefix := false
	// Responses 的系统提示通常在顶层 instructions；chat 在 messages 里。
	if ins := jsonx.StrOf(payload["instructions"]); ins != "" {
		h.Write([]byte(ins))
		hasPrefix = true
	}
	if hashSystemMessages(h, payload["messages"]) {
		hasPrefix = true
	}
	if hashSystemMessages(h, payload["input"]) {
		hasPrefix = true
	}
	if tools, ok := payload["tools"].([]any); ok && len(tools) > 0 {
		if raw, err := json.Marshal(tools); err == nil {
			h.Write(raw)
			hasPrefix = true
		}
	}
	if !hasPrefix {
		return ""
	}
	return "aa1-" + hex.EncodeToString(h.Sum(nil)[:8])
}

// hashSystemMessages 把 system / developer 角色的消息内容写进哈希，
// 返回是否写入了任何内容。
//
// 其余角色一律跳过：用户 / 助手正文每轮都变，参与哈希等于每轮换一个会话。
func hashSystemMessages(h hash.Hash, v any) bool {
	msgs, ok := v.([]any)
	if !ok {
		return false
	}
	wrote := false
	for _, mv := range msgs {
		m, ok := mv.(map[string]any)
		if !ok {
			continue
		}
		role := jsonx.StrOf(m["role"])
		if role != "system" && role != "developer" {
			continue
		}
		switch c := m["content"].(type) {
		case string:
			h.Write([]byte(c))
			wrote = true
		default:
			if raw, err := json.Marshal(c); err == nil {
				h.Write(raw)
				wrote = true
			}
		}
	}
	return wrote
}

// nonStreamBudget 是非流式请求的整体预算（选号 + 重试 + 读体）。
//
// 没有它时，「MaxRetries × Retry-After≤30s」理论上可挂 90s 以上。
const nonStreamBudget = 90 * time.Second

// requestModelName 提取请求里的 model 字段。
func requestModelName(payload map[string]any) string {
	v, _ := payload["model"].(string)
	return strings.TrimSpace(v)
}

// estimatePromptLen 粗略统计 messages / input 的字符数。
func estimatePromptLen(payload map[string]any) int {
	if v, ok := payload["messages"]; ok {
		if raw, err := json.Marshal(v); err == nil {
			return len(raw)
		}
	}
	if v, ok := payload["input"]; ok {
		if raw, err := json.Marshal(v); err == nil {
			return len(raw)
		}
	}
	return 0
}

// chatTextLen 统计 chat 响应中的正文 + 思维链字符数。
func chatTextLen(m map[string]any) int {
	choices, _ := m["choices"].([]any)
	total := 0
	for _, c := range choices {
		cm, ok := c.(map[string]any)
		if !ok {
			continue
		}
		if msg, ok := cm["message"].(map[string]any); ok {
			total += jsonx.LenOfString(msg["content"])
			total += jsonx.LenOfString(msg["reasoning"])
		}
		if d, ok := cm["delta"].(map[string]any); ok {
			total += jsonx.LenOfString(d["content"])
			total += jsonx.LenOfString(d["reasoning"])
		}
	}
	return total
}

// requestGroup 返回当前请求密钥绑定的号池分组（无密钥时为空=全部分组）。
func (s *Server) requestGroup(r *http.Request) string {
	if r == nil {
		return ""
	}
	key := extractKey(r)
	if key == "" {
		return ""
	}
	return s.Store.KeyGroup(key)
}

// nextAccount 按请求所属分组挑一个轮询账号。
func (s *Server) nextAccount(r *http.Request) *deepai.Account {
	return s.Store.NextAccountGroup(s.requestGroup(r))
}

// clientFor 返回账号绑定的上游客户端（按代理缓存）。
//
// 专属出口要先过一道可用性探测（proxyUsable）：代理串解析成功不等于能穿透，
// 2026-11-22 线上就存在 16 个「从未真正生效」的住宅代理（缺 scheme 导致
// 静默退回全局出口）。探测未过或未知时沿用全局客户端，行为与修复前一致。
func (s *Server) clientFor(acct *deepai.Account) *deepai.Client {
	if acct == nil || strings.TrimSpace(acct.Proxy) == "" {
		return s.Up
	}
	// 出口冷却中（连续传输失败 / 被 WAF 拦的 HTML 403）也退回全局出口：
	// 这是「自动降级」而不是「弃号」——账号仍可继续服务，只是不再从这条出口出去。
	if !s.proxyUsable(acct) || s.proxyCooling(acct) {
		return s.Up
	}
	s.proxyMu.Lock()
	defer s.proxyMu.Unlock()
	if s.proxyClients == nil {
		s.proxyClients = map[string]*deepai.Client{}
	}
	if c, ok := s.proxyClients[acct.Proxy]; ok {
		return c
	}
	c := deepai.NewClientWithProxy(acct.Proxy)
	// 代理出口的客户端也要上报上游车道，否则走代理的账号探到的 lane_version
	// 就到不了控制台（s.Up 只覆盖直连）。
	c.LaneHook = s.noteUpstreamLane
	// 运行期可调项（车道 / 签到预算）由客户端拉取，这里不再逐项注入。
	c.Runtime = s.runtimeConfig
	s.proxyClients[acct.Proxy] = c
	return c
}

// historyStats 统计本次请求回喂给上游的历史规模：消息条数与是否带系统提示。
//
// 只数条数与角色，不碰正文。用途是把 issue #5 里那三类原因分开：
//   - 客户端没给历史（messages 只有 1 条）→ 调用方问题；
//   - 网关把历史弄丢了（客户端发了 N 条、日志里却是 1 条）→ 网关 bug；
//   - 上游忽略历史（日志里条数正常、回答仍失忆）→ 上游行为。
//
// 兼容两种形状：chat 的 messages 与 Responses 的 instructions + input。
func historyStats(payload map[string]any) (msgs int, hasSystem bool) {
	if v, ok := payload["messages"].([]any); ok {
		for _, mv := range v {
			m, ok := mv.(map[string]any)
			if !ok {
				continue
			}
			role := jsonx.StrOf(m["role"])
			if role == "" {
				if _, isStr := m["content"].(string); isStr {
					msgs++ // 省略 role 的字面量消息（OpenAI 允许）
				}
				continue
			}
			msgs++
			if role == "system" || role == "developer" {
				hasSystem = true
			}
		}
		return msgs, hasSystem
	}
	if ins := jsonx.StrOf(payload["instructions"]); ins != "" {
		hasSystem = true
	}
	if v, ok := payload["input"].([]any); ok {
		for _, mv := range v {
			m, ok := mv.(map[string]any)
			if !ok {
				continue
			}
			if t := jsonx.StrOf(m["type"]); t != "" && t != "message" {
				continue
			}
			role := jsonx.StrOf(m["role"])
			if role == "" {
				role = "user"
			}
			msgs++
			if role == "system" || role == "developer" {
				hasSystem = true
			}
		}
	}
	if msgs == 0 && jsonx.StrOf(payload["input"]) != "" {
		msgs = 1 // Responses 的字符串简写等价于一条 user 消息
	}
	return msgs, hasSystem
}
