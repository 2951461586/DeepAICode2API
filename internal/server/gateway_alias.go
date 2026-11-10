package server

// 协议转换层（Anthropic / Responses 字段映射、流式块生成、用量解析）已抽到
// internal/gateway。那一层只依赖标准库与 deepai 协议常量，不碰 HTTP、store
// 或号池，因此可以脱离网关单独测、单独改。
//
// 这里保留包内短名（类型别名 + 函数变量），让既有 handler 与测试一行都不用改：
// 迁移是纯搬家，不是行为改动。
//
// 新增转换逻辑请直接写到 internal/gateway，不要在本文件里加实现。

import "deepaicode2api/internal/gateway"

type (
	// Usage 是上游返回的用量（token / 成本）。
	Usage = gateway.Usage

	// Anthropic 请求模型。
	anthropicRequest     = gateway.AnthropicRequest
	anthropicMessage     = gateway.AnthropicMessage
	anthropicTool        = gateway.AnthropicTool
	anthropicBlockWriter = gateway.AnthropicBlockWriter
)

var (
	// 用量解析。
	parseUsage  = gateway.ParseUsage
	intOf       = gateway.IntOf
	floatOf     = gateway.FloatOf
	lenOfString = gateway.LenOfString
	strOf       = gateway.StrOf
	boolOf      = gateway.BoolOf

	// errNoMessages 表示转换后没有可用消息（映射成 400）。
	errNoMessages = gateway.ErrNoMessages

	// Anthropic ⇄ Chat 映射。
	firstString              = gateway.FirstString
	anthropicToChatPayload   = gateway.AnthropicToChatPayload
	anthropicEffort          = gateway.AnthropicEffort
	appendAnthropicMessage   = gateway.AppendAnthropicMessage
	anthropicSystemText      = gateway.AnthropicSystemText
	anthropicImageURL        = gateway.AnthropicImageURL
	anthropicDocumentDataURL = gateway.AnthropicDocumentDataURL
	anthropicToolResultText  = gateway.AnthropicToolResultText
	anthropicToolChoice      = gateway.AnthropicToolChoice
	chatToAnthropicMessage   = gateway.ChatToAnthropicMessage
	anthropicStopReason      = gateway.AnthropicStopReason
	anthropicTextLen         = gateway.AnthropicTextLen
	newAnthropicBlockWriter  = gateway.NewAnthropicBlockWriter

	// Responses ⇄ Chat 映射。
	responsesReasoningEffort   = gateway.ResponsesReasoningEffort
	responsesToChatPayload     = gateway.ResponsesToChatPayload
	responsesInputContent      = gateway.ResponsesInputContent
	responsesToolOutput        = gateway.ResponsesToolOutput
	convertResponsesToolChoice = gateway.ConvertResponsesToolChoice
	chatToResponsesObject      = gateway.ChatToResponsesObject
)
