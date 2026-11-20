package deepai

// 兼容别名：协议契约与领域类型已搬到 internal/model。
//
// 为什么保留这层：deepai 内部（client / checkin / sse）用裸名引用这些符号，
// 改名会把 800 行 diff 混进一次「纯粹搬迁」里。这里的别名是**零成本**的
// （类型别名不产生新类型，常量别名在编译期折叠，函数用 var 绑定），
// 且新代码应当直接依赖 internal/model —— 见 docs/server-split-plan.md。
//
// 判断标准：deepai 只保留「怎么发请求」；Account / Model / 错误词汇 / 常量
// 的**定义**在 model，deepai 只是它的第一个使用者。

import "deepaicode2api/internal/model"

type (
	HTTPError    = model.HTTPError
	Model        = model.Model
	Credentials  = model.Credentials
	CheckIn      = model.CheckIn
	Allowance    = model.Allowance
	UsageSummary = model.UsageSummary
	Account      = model.Account
	Registration = model.Registration
)

const (
	LaneVersion             = model.LaneVersion
	AppVersion              = model.AppVersion
	DesktopUserAgent        = model.DesktopUserAgent
	HeaderKeyName           = model.HeaderKeyName
	HeaderLane              = model.HeaderLane
	HeaderMachine           = model.HeaderMachine
	HeaderAffinity          = model.HeaderAffinity
	HeaderInstance          = model.HeaderInstance
	HeaderCheckIn           = model.HeaderCheckIn
	HeaderBeacon            = model.HeaderBeacon
	CodeSessionBusy         = model.CodeSessionBusy
	CodeLaneUnsupported     = model.CodeLaneUnsupported
	CodeSocialLogin         = model.CodeSocialLogin
	CodeMachineRequired     = model.CodeMachineRequired
	CodeEmailUnverified     = model.CodeEmailUnverified
	CodeAuthRequired        = model.CodeAuthRequired
	CodeInvalidAPIKey       = model.CodeInvalidAPIKey
	CodeInvalidReq          = model.CodeInvalidReq
	CodeCheckInRefused      = model.CodeCheckInRefused
	CodeTheosExhausted      = model.CodeTheosExhausted
	CodeNotDeepAICodeMsg    = model.CodeNotDeepAICodeMsg
	CodeNotAcceptedMsg      = model.CodeNotAcceptedMsg
	StatusBlocked           = model.StatusBlocked
	StatusError             = model.StatusError
	ErrorStaleAfter         = model.ErrorStaleAfter
	ModelBasePath           = model.ModelBasePath
	PathModels              = model.PathModels
	PathChat                = model.PathChat
	PathResponses           = model.PathResponses
	PathTheos               = model.PathTheos
	PathWebSearch           = model.PathWebSearch
	PathFindService         = model.PathFindService
	PathSessionsList        = model.PathSessionsList
	PathSessionsGet         = model.PathSessionsGet
	PathSessionsDel         = model.PathSessionsDel
	PathSessionsSave        = model.PathSessionsSave
	PathSessionsSaveRecords = model.PathSessionsSaveRecords
	PathSessionTurn         = model.PathSessionTurn
	PathSignIn              = model.PathSignIn
	PathExchange            = model.PathExchange
	PathUsageSummary        = model.PathUsageSummary
	PathSpeechToText        = model.PathSpeechToText
	PathAttachments         = model.PathAttachments
	PathAttachmentsGet      = model.PathAttachmentsGet
	PathGravityDisplay      = model.PathGravityDisplay
	PathGravityImpression   = model.PathGravityImpression
	DefaultOrigin           = model.DefaultOrigin
	DefaultModel            = model.DefaultModel
)

var (
	IsSocialLoginRequired = model.IsSocialLoginRequired
	IsCheckInRefused      = model.IsCheckInRefused
	Transient             = model.Transient
	AccountReady          = model.AccountReady
	AccountDegraded       = model.AccountDegraded
	RandomHex             = model.RandomHex
	RandomToken           = model.RandomToken
	KeyFingerprint        = model.KeyFingerprint
	ShortHash             = model.ShortHash
	DefaultModelEntry     = model.DefaultModelEntry
)
