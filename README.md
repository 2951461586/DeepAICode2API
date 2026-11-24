<div align="center">

<img src="docs/assets/logo.png" alt="DeepAICode2API" width="128" height="128">

# DeepAICode2API

把本机 **DeepAI Code 桌面端**的上游模型服务，逆向复刻成一个
**OpenAI / Anthropic 双协议兼容的 Go 网关**。

复用桌面端登录后写入的 API Key，无需另申请密钥；纯 Go 标准库实现，
单文件约 10 MB，零外部依赖。

**中文 / [English](README.en.md)**

[![ci](https://github.com/2951461586/DeepAICode2API/actions/workflows/ci.yml/badge.svg)](https://github.com/2951461586/DeepAICode2API/actions/workflows/ci.yml) [![Go](https://img.shields.io/badge/Go-1.26-00ADD8?logo=go&logoColor=white)](https://go.dev) [![deps](https://img.shields.io/badge/deps-0%20(stdlib)-00ADD8)](go.mod) [![tests](https://img.shields.io/badge/tests-616%20cases-brightgreen)](https://github.com/2951461586/DeepAICode2API/actions/workflows/ci.yml)
[![protocol](https://img.shields.io/badge/protocol-OpenAI%20%7C%20Anthropic-6b4fbb)](#三接口清单) [![docker](https://img.shields.io/badge/docker-ready-2496ED?logo=docker&logoColor=white)](docker-compose.yml) [![License](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)
[![Ask DeepWiki](https://img.shields.io/badge/Ask%20DeepWiki-blue.svg?labelColor=000000&logo=data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAACAAAAAgCAYAAABzenr0AAAACXBIWXMAAAPoAAAD6AG1e1JrAAACIUlEQVRYw+2XP2gUQRTGv7d3JhYWQawkIBaCVWzETos0gmAnVoKNTSoLK0E7EQQrC20sBVFBtLPQRgTBtBaKjYgIQUSxiJq7fT+LvCGPJZdNLt4dQj5Y3u7sznzfzLw/s9IO/icANiniCujEfWdsQgBLxAbsbraPZcmBfcAt4DNwKQsZNfEUsAB8YRV12LfA6ZHuedg7AO7ed/c+UBcbQhbiu+6wXIM6EnZWkszMJe2Ke0n6I2la0v51hFeSMLN6OwIK6jJwEBdUYb2MAxRSz6sY4geiahFgLe1Hwq6YWe3us8AN4FQQU4QM64SPY69Xyr43fADgNjAPXHb3peSsD4FDQ0VLEjAHvIhBPS6AHvA1bBO9JPAXcHFbIRtJ5yzwIQZ9CZwAusDJENIkLqsG8DpNyIZJwSUk9wLHgakcesC1JCCjiHm1kYDNOEjptCxpSVK/8f73OFLxPLAYM3oCzEX7MXf/lJbc/8kWpA4HgQdpYI9IWAbeh83whi84cHXL+58EPBoQhnmm94EzwE3gZ2p/Dhzdbhg+TaRr01x7ftb4/jBwFzjXrCvDpmLW9crVLNeR9CaapoGemb2TdCERW1tNaBNQ8jktwmozqwtpqRNtdWAjARYk39IS12ZWRX7vmJmA71nQZgi36gN7gCvAj5xc3P0jcH7kx7Ik5IC73wsh14GZsZySow5002l4Jr0b6+m4eSyvpAn8lEzsx2QHo8RfUrlN+uPq4ksAAAAASUVORK5CYII=)](https://deepwiki.com/2951461586/DeepAICode2API)

</div>

| 能力 | 说明 |
| --- | --- |
| 双协议 | OpenAI `/v1/chat/completions`、`/v1/responses`；Anthropic `/v1/messages` |
| 协议对齐 | 车道 7 + `X-DeepAI-Code-Instance` + `/theos` 签到；区分 `social_login_required` / `machine_required` / `lane_version_unsupported` |
| 号池治理 | 分组轮询、失败换号、容量退避、按账号配额调度、策略拦截自动跳过 |
| 账号接入 | 浏览器登录（Google / Apple / 邮箱密码）、桌面端凭据导入、手工 API Key；入池自动分配机器指纹与代理 |
| 控制台 | 单页内嵌 UI：账号 / 密钥 / 日志 / 统计 / 设置 |
| 安全 | 密钥鉴权、管理口令、同源写保护、状态文件 AES-256-GCM 可选加密 |
| 可回归 | 616 个用例（468 顶层 + 148 子用例）/ 14.7k 行测试，`go test -race` + gofmt / vet / build / vulncheck 全绿，CI 覆盖 10 个含测试的包（另 5 个无测试）+ 前端语法与冒烟（23 条） |

```text
┌──────────────┐  /v1/chat/completions   ┌────────────────┐   Api-Key + Theos 头   ┌─────────────────────────────┐
│ OpenAI SDK   │  /v1/responses          │ DeepAICode2API │  X-DeepAI-Code-Machine │ https://api.deepai.org      │
│ Claude Code  │  /v1/messages           │   （本网关）    │ ─────────────────────▶ │ /deepai_code/chat/completions│
│ pi / Cline   │  /v1/models             │                │  ◀───────────────────── │ /deepai_code/responses      │
└──────────────┘  SSE 流式 / 非流式       └───────┬────────┘      OpenAI 兼容 SSE    └─────────────────────────────┘
                                                  │ 自动读取
                                                  ▼
                          ~/.deepai-code/userdata/secrets/deepai-account.bin
                          {"apiKey":"…","username":"…","isPro":false}
```

---

## 一、逆向结论

DeepAI Code 是 **Electron + 本地 t3code server** 架构（应用版本 **0.3.50**，车道 7；
0.3.20 → 0.3.30 → 0.3.32 → 0.3.35 → 0.3.36 → 0.3.37 → 0.3.41 → 0.3.42 → 0.3.43 → 0.3.44 → 0.3.45 → 0.3.47 → 0.3.48 → 0.3.50 的 wire protocol 逐项比对：
**0.3.41 只抬了车道（6 → 7），0.3.42 / 0.3.43 / 0.3.44 / 0.3.45 / 0.3.47 / 0.3.48 在协议面上零变更（车道仍 7），0.3.50 的 agent 运行时迁移（pi → dsh）同样不碰请求头 / 指纹 / 签到 / 推理端点 / 请求体字段**；
0.3.45 新增的 DeepAI Connect（`/connect/*`、`/sponsored/authorize`、PKCE 登录）与
0.3.47 的登录指引 / 恢复提示 / 启动与存储错误文案、0.3.48 的启动配对令牌修复与目录权限恢复指引、
0.3.50 的 DeepSeek / OpenCode 托管 harness 路由，
都是账号、远程主机与本地编排面，不进推理链路；`sessions/save_records` 只服务历史备份）。
真正干活的不是桌面壳，而是 `resources/server.asar` 里的 t3code server；
它把上游能力封装成本地回环 HTTP 服务，再交给 Pi（编码 agent）调用。

### 1.1 关键事实

| 环节 | 逆向结果 |
| --- | --- |
| 应用形态 | Electron 壳（`app.asar`）+ 独立 server（`server.asar`，约 200 MB） |
| 数据目录 | `T3CODE_HOME` 或 `~/.deepai-code`，状态目录为 `<base>/userdata` |
| **凭据落盘** | `<stateDir>/secrets/deepai-account.bin`，**明文 JSON**（文件权限 0600） |
| 凭据内容 | `{apiKey, username, isPro}`，`apiKey` 为 UUID 格式 |
| **推理入口** | `POST https://api.deepai.org/deepai_code/chat/completions` |
| Responses 入口 | `POST https://api.deepai.org/deepai_code/responses`（仅 `api=openai-responses` 模型） |
| 模型目录 | `GET https://api.deepai.org/deepai_code/models` |
| 配额 | `GET https://api.deepai.org/deepai_code/theos` |
| 用量 | `GET https://api.deepai.org/get_usage_summary` |
| 语音转写 | `POST https://api.deepai.org/speech_to_text`（字段 `voiceRecording`） |
| 联网搜索 | `POST https://api.deepai.org/deepai_code/web_search` |
| **鉴权** | 请求头 `Api-Key: <apiKey>`（不是 `Authorization`） |
| 协议头 | `X-DeepAI-Code-Theos: 7`（计费车道版本；0.3.41 起为 7，2026-11-24 复核 0.3.48 / 0.3.50 仍为 7）。**车道门槛是服务端行为**：2026-11-18（0.3.41 发布当天）只有 7 通过，2026-11-19 复测已放宽为 ≤7 全通过、8 及以上 426（`docs/recon.md` §3） |
| 进程实例 | `X-DeepAI-Code-Instance: <32 位 hex>`（每个出口代理随机一次，随该代理的所有 JSON 请求发出；桌面端是每进程一次） |
| 签到 | `/theos` 响应带 `check_in:{nonce, seconds}`；下次 `/theos` 回传 `X-DeepAI-Code-Check-In` 与 `X-DeepAI-Code-Beacon: v=…;up=…;pi=0\|1`（`focus` 只在焦点状态已知时才追加） |
| **机器指纹** | `X-DeepAI-Code-Machine: sha256(原始机器 UUID)`，用于按天发放免费 Theos；计量模型缺失时回 `machine_required` |
| 会话粘性 | `X-Session-Affinity`（body 里的 `prompt_cache_key` 也会被上游使用）；网关另把它绑定到具体账号，让同一会话的前缀缓存留在同一个号上（见 §十） |
| 容量拒绝 | `409 {"error":{"code":"session_busy"}}` + `Retry-After` |
| 账号策略 | `403 social_login_required`：0.3.19 的阶段性收窄，2026-11-09 复测已解除；网关仍按 `blocked` 终态处理（见 `docs/recon.md` §4.1） |
| 浏览器登录 | `GET /deepai_code/desktop_auth/?state=…&port=…&code_challenge=…&code_challenge_method=S256` → 本机 `/auth-callback?code=…` → `POST /deepai_code/desktop_auth/exchange/`（body 带 `code_verifier`）。PKCE 与 0.3.45 桌面端一致，可用 `DEEPAI2API_LOGIN_PKCE=0` 退回旧形态 |

### 1.2 上游本身就是 OpenAI 兼容的

`chat/completions` 的请求 / 响应与 OpenAI 完全一致，响应还额外带了
OpenRouter 风格的 `provider`、`reasoning`、`reasoning_details`、`cost` 等字段
（`id: gen-…`、`usage.cost_details.upstream_inference_*`、`native_finish_reason`）。
注意 `provider` 是**真实推理后端名**（`InferenceNet` / `Wafer` / `Reka` / …），
**替身分支现在不带 `provider` 字段**（旧版的 `provider: "DeepAI"` 已消失），
判断真/替身要同时看 `provider` 是否缺失、`model` 是否回显请求的 `z-ai/<id>`、
`id` 前缀是 `gen-` 还是 `gen_`（完整判据表见 `docs/recon.md` §3.1）。
因此本网关不需要做复杂的协议还原，重点在：

1. **凭据导入**：从桌面端明文凭据文件读取并去重；
2. **账号池**：多 Key 轮询、失败换号、`session_busy` 等待重试；
3. **模型治理**：动态目录缓存、别名、兜底模型；
4. **协议适配**：为 Anthropic 客户端和 Responses 客户端做翻译；
5. **可观测性**：请求日志、token / 成本统计、配额探测、控制台。

### 1.3 机器指纹怎么来的

桌面端逻辑（`src/deepai/machineId.ts`）：

- Windows：`(Get-CimInstance Win32_ComputerSystemProduct).UUID`，固件占位 UUID 时回落
  到注册表 `HKLM\SOFTWARE\Microsoft\Cryptography\MachineGuid`；
- macOS：`ioreg` 的 `IOPlatformUUID`；
- Linux：`/sys/class/dmi/id/product_uuid`，其次 `/etc/machine-id`；
- 最终取 `sha256(小写原始值)`，原始值绝不上传。

本网关复刻了同一套逻辑。容器 / 无 SMBIOS 环境可用
`DEEPAI_MACHINE_ID=<原始UUID>` 显式覆盖。

---

## 二、支持的模型

模型目录**完全由上游下发**：网关只做缓存与别名，上游新增或下线模型都无需改代码。
下表是 2026-11-18 从线上 `/v1/models` 导出的**实时目录**（28 个：12 免费 / 16 付费），
**2026-11-24 复核：与 11-18 快照相比只多 1 个免费模型 `claude-haiku-5-5`**（95,000 / 64,000 / 0），
其余 27 个逐字段相同（ID / 名称 / 上下文 / 最大输出 / 价格 / `default: glm-5.3-flash` 全部相同，无下线改价），
`Theos/次` 取自上游 `theosPerRequest`（字符串），`0` 即免费。控制台「模型目录」页读的是同一份数据。
（11-20 复核与 11-18 快照逐字段零差异；与 2026-11-14 的快照相比：**模型集合、名称、价格均未变**，只有 `glyph-cluster`
 的 `contextWindow` 从 320,000 变为 256,000。更早的 `crouton-alpha` 已下线；
 `gpt-oss-120b` 客户端会主动过滤，网关也不展示。
 2026-11-14 那轮相对 11-13 新增 6 个、无下线：`muse-ember`（免费）、`kimi-k3.1-pro`、
 `gpt-6.1-terra`、`gemini-4-hydrogen`、`gemini-4-carbon`、`claude-sonnet-5-5`。）

| ID | 名称 | 上下文 | 最大输出 | Theos/次 | 说明 |
| --- | --- | ---: | ---: | ---: | --- |
| `claude-haiku-5-5` | Claude Haiku 5.5 | 95,000 | 64,000 | **0** | 免费（2026-11-24 新增），唯一非 320k 的付费级命名模型 |
| `deepai-cyber` | DeepAI Cyber | 320,000 | 131,072 | **0** | 免费，纯文本 |
| `deepseek-v4.1-flash` | DeepSeek V4.1 Flash 1017 | 320,000 | 131,072 | **0** | 免费（上游下拉是 `DeepSeek V4.1 Flash 1017`，桌面端会把旧名改写成它） |
| `glm-5.3-flash` | GLM 5.3 Flash | 320,000 | 131,072 | **0** | 免费，默认模型；實測 320k 完整（見下方「有效上下文上限」） |
| `glyph-cluster` | Glyph Cluster | 256,000 | 131,072 | **0** | 免费，纯文本；實測上限 **256k**（260k 完整、320k 硬拒絕） |
| `gpt-6-luna` | GPT-6 Luna | 320,000 | 128,000 | **0** | 免费 |
| `mimo-2.6-flash` | MiMo 2.6 Flash | 320,000 | 131,072 | **0** | 免费；實測上限 **140k**（170k 起上游靜默截斷，網關提前裁剪） |
| `mimo-2.6-pro` | MiMo 2.6 Pro | 320,000 | 131,072 | **0** | 免费 |
| `minimax-m3.1-flash` | MiniMax M3.1 Flash | 320,000 | 131,072 | **0** | 免费 |
| `muse-ember` | Muse Ember | 320,000 | 131,072 | **0** | 免费（2026-11-14 新增） |
| `qwen-3.8-flash` | Qwen 3.8 Flash | 320,000 | 128,000 | **0** | 免费 |
| `solar-mini-4` | Solar Mini 4 | 320,000 | 131,072 | **0** | 免费，纯文本 |
| `minimax-m3` | MiniMax M3 | 320,000 | 131,000 | 2 | 付费，纯文本 |
| `glm-5.3` | GLM 5.3 | 320,000 | 131,072 | 3 | 付费，纯文本 |
| `kimi-k3` | Kimi K3 | 320,000 | 131,072 | 3 | 付费 |
| `kimi-k3.1-pro` | Kimi K3.1 Pro | 320,000 | 131,072 | 3 | 付费（2026-11-14 新增） |
| `gpt-6.1-terra` | GPT-6.1 Terra | 320,000 | 128,000 | 5 | 付费（2026-11-14 新增） |
| `muse-spark-1.3` | Muse Spark 1.3 | 320,000 | 131,072 | 6 | 付费 |
| `gemini-4-hydrogen` | Gemini 4 Hydrogen | 320,000 | 131,072 | 8 | 付费（2026-11-14 新增） |
| `claude-sonnet-5-5` | Claude Sonnet 5.5 | 320,000 | 64,000 | 10 | 付费（2026-11-14 新增） |
| `gemini-4-argon` | Gemini 4 Argon | 320,000 | 131,072 | 10 | 付费 |
| `gemini-4-carbon` | Gemini 4 Carbon | 320,000 | 131,072 | 10 | 付费（2026-11-14 新增） |
| `gpt-6.1-sol` | GPT-6.1 Sol | 320,000 | 128,000 | 10 | 付费 |
| `claude-opus-5-5` | Claude Opus 5.5 | 320,000 | 64,000 | 15 | 付费 |
| `muse-wonder` | Muse Wonder | 320,000 | 131,072 | 15 | 付费 |
| `qwen-3.8-max-prime` | Qwen 3.8 Max Prime | 320,000 | 131,072 | 15 | 付费 |
| `claude-fable-5-2` | Claude Fable 5.2 | 320,000 | 64,000 | 20 | 付费 |
| `gpt-6-astra` | GPT-6 Astra | 320,000 | 128,000 | 50 | 付费，仅 `/v1/responses` |

说明：

- 只有 `deepai-cyber` / `glm-5.3` / `glyph-cluster` / `solar-mini-4` / `minimax-m3` 是纯文本
  （`input: [text]`），其余都接受图片输入（`input: [text, image]`）；全部支持推理（`reasoning`）；
- **车道 7 的目录把 `contextWindow` 统一收敛到 320,000，只有 `glyph-cluster`
  是 256,000**（0.3.41 起的线上目录）。**2026-11-19 复测：车道 5 / 6 / 7 拿到的目录
  完全一致**（当时 27 个模型零差异），0.3.37 时代的分车道差异已消失；网关按上游原样透传；
- **2026-11-14 起上游 `/deepai_code/models` 要求 `Api-Key`**：不带 Key 一律 401
  `auth_required`（此前「带不带都返回同一份目录」的结论已失效）。网关的目录刷新与
  车道探测因此都借用号池里一个账号的凭据；运维上要知道的坑见 §10.1；
- `gpt-6-astra` 的 `api` 是 `openai-responses`，必须走 `/v1/responses`，
  用 `/v1/chat/completions` 调用会被上游拒绝；
- 付费模型按 `Theos/次` 计量，从账号的每日额度 / 余额里扣；额度耗尽后上游返回 403（见 §6.9）；
- 上游若还下发 `theosPerSecond`（按秒计费），控制台会在「每秒计费」列显示，
  `/v1/models` 的 `deepai` 元信息里也能读到；
- **以线上 `/v1/models` 为准**：仓库里不放状态文件（`deepaicode2api-data*.json`
  是运行期产物，含上游 Key，已 gitignore 且已清理）；本表与 `/v1/models` 不一致时，
  先在控制台点「刷新模型目录」再按本表格式重新导出。
- **内置兜底条目与线上对齐**：目录为空（首启未拉到 / 刷新失败）时 `/v1/models`
  只回一条 `glm-5.3-flash`，字段取线上实测值（ctx **320,000**、maxTokens 131,072、
  免费、text+image、reasoning）。旧构建写的是 `1,048,576`，会让客户端按 1M
  上下文组请求后被上游拒（v0.1.11 已修，见 `docs/changes.md` §70）。

内置别名（可在控制台覆盖，共 13 个，全部指向默认免费模型 `glm-5.3-flash`）：

- `deepai` / `deepai-code` / `default`
- `gpt-4` / `gpt-4o` / `gpt-4o-mini` / `gpt-4.1` / `gpt-3.5-turbo`
- `claude-3-5-sonnet` / `claude-sonnet-4` / `claude-3-haiku`
- `deepseek-chat` / `glm-4-flash`
- `deepai/<id>`、`openai/<id>` 前缀会被自动剥离（`deepai:` / `openai:` 同效）

无法识别的模型名**默认**回落到「兜底模型」→ 目录默认模型，保证任意客户端开箱即用；
控制台打开「**严格模型模式**」后改为返回 `404 invalid_request_error`，并列出相近的
目录 ID，不再静默替换（模型指纹 / 渠道核对场景一定要开）。

---

## 三、接口清单

### OpenAI 兼容

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/v1/models` | 模型目录（含 `deepai` 元信息） |
| POST | `/v1/chat/completions` | 流式 / 非流式；支持 tools、图片、`prompt_cache_key` |
| POST | `/v1/responses` | `openai-responses` 模型透传；其余模型自动翻译（**无状态**：续接字段见下） |
| POST | `/v1/audio/transcriptions` | 转发上游语音转写（webm / mp4） |
| POST | `/v1/audio/translations` | 同上（别名） |

### Anthropic 兼容

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| POST | `/v1/messages` | 流式 / 非流式，支持 `tool_use`、`thinking`、图片 |
| POST | `/v1/messages/count_tokens` | 本地估算（上游不提供） |

#### 上下文契约：`/v1/responses` 伺服端歷史暫存（链式续接）

三套协议中，`/v1/chat/completions` 与 `/v1/messages` 仍按「调用方每轮发完整历史」工作
（和桌面端一致）：网关不存对话，也不会替你补上一轮。但 `/v1/responses` 自 2026-11-20
起支持**伺服端對話歷史暫存 + 重放**，解决反代 / magpie 插件客户端攜帶
`previous_response_id` 續接时的「失忆」问题：

- **其余模型（chat/completions 合成路径，如 `glm-5.3-flash`）**：每轮 `/v1/responses`
  回應完成後，网关把该轮的 input 与 output 存进进程内暫存（按 response id 索引）。
  下一轮帶 `previous_response_id` 时，网关沿鏈重組完整 input 再送上游——客户端无需
  自己维护历史，链式续接不再回 400，也不会失忆。新回應有自己的 id，可继续链接。
  - **暫存上限**：默认 TTL 24 小时、每个进程最多 500 轮，可用环境变量调整：
    - `DEEPAI2API_HISTORY_TTL_HOURS`（默认 `24`）
    - `DEEPAI2API_HISTORY_MAX_ENTRIES`（默认 `500`）
  - **有效上下文上限（按模型實測值，避免上游靜默丟歷史）**：2026-11-20 把 11 個
    免費模型全部實測（純透傳 + 唯一隨機 ASCII 階梯，附多輪對照），有效窗口按模型查表：
    - **9 個免費模型**：`glm-5.3-flash`、`deepseek-v4.1-flash`、`qwen-3.8-flash`、
      `minimax-m3.1-flash`、`muse-ember`、`solar-mini-4`、`gpt-6-luna`、
      `mimo-2.6-pro`、`deepai-cyber` → **320,000**（等於標示值，不再人為砍到 120k）。
    - **`glyph-cluster` → 256,000**（260k 完整，320k 被上游硬拒絕 400，目錄標 256k 一致）。
    - **`mimo-2.6-flash` → 140,000**（140k 完整，170k 起上游「靜默截斷」：
      `prompt_tokens` 掉到 5,296 ≈ 3%；上限落在 140k–170k，保守取 140k）。
    - 未知／付費模型 → `min(標示值, 320,000)`，不再一律硬壓 120k。
    - **請求體字節閘門（2026-11-22，issue #7）**：上游真正的懸崖是**請求體約 196 KB**，
      不是 token 數 —— 第三方網關交叉實測：195,809 B 完整讀入，196,811 B 起上游靜默丟掉開頭
      （system / 最初幾輪），`usage.prompt_tokens` 釘在 ~40k。目錄標示 320k 在純英文下
      （~4.2 B/token）要 ~512 KB 才會被本包的 token 估算觸發，攔不住，所以重放另加一道
      `DEEPAI2API_CONTEXT_BODY_BYTES`（默認 `196608`，`0`／負數關閉）：超限時同樣走上面兩條
      路徑（`truncation:auto` 裁剪或 400 明確報錯）。
    - `/v1/models` 的 `deepai` 元信息因此新增 **`effective_context_window`**（按模型實測上限）與
      **`context_body_bytes`**（字節閘門的值）；`context_window` 仍是上游標稱值，不篡改，
      方便客戶端／腳本自行取用真實上限。
    - 網關重放時若超過各模型的有效窗口，先由網關裁剪並在 input 留下
      「中間歷史已省略」標記，也不讓上游偷偷砍歷史。
    - **環境變數覆寫**（排查／臨時調整用）：
      - `DEEPAI2API_EFFECTIVE_CONTEXT_WINDOW`：全域覆寫（舊行為保留）。如 `64000`；
        設 `0`／負數關閉上限。優先權最高。
      - `DEEPAI2API_CONTEXT_WINDOWS`：按模型 JSON 覆寫，例如
        `{"mimo-2.6-flash":140000,"glm-5.3-flash":320000}`（值 `0`／負數 = 該模型不設上限，
        JSON 格式錯誤則忽略）。優先權低於全域覆寫、高於內建實測表。
    - 單發探測（不鏈式）實測：唯一隨機 ASCII 160,000 bytes → 上游
      `input_tokens=100,761`（比值約 1.59 tokens/byte）；重複填充會被前綴快取，
      `input_tokens` 不可作為窗口判斷依據。
  - **自動上下文壓縮（`truncation`）**：重放整條鏈後若超過有效窗口（見上）：
    - `"truncation":"auto"`：網關自動裁掉最舊的**中段**輪次（保留開頭系統/首則訊息
      與最近的可容納範圍，並插入一則「中間歷史已省略」說明），照常送上游。
    - 未設或 `"default"`：回明確 400 `invalid_request_error`，訊息寫明 token 估算與
      有效窗口/標示值，並提示「請由客戶端壓縮歷史，或改用 `truncation:auto`」。
      **绝不靜默砍歷史**——那正是「失憶」的根源。
    - token 估算：ASCII 按 ~1.6 bytes/token、非 ASCII 按 ~1 token/字粗估
      （實測與上游 `input_tokens` 偏差在 ±30% 內）；只在上限判斷時計算，
      正常情況零額外開銷。
  - **生命周期**：进程内存储，**重启后历史清空**。重启后客户端若仍帶旧的
    `previous_response_id`，网关找不到记录会回 400 `unsupported_parameter`——
    此时客户端应重新发送完整 `input`（或在重启后先开新对话）。
  - 仍**不支持**：`conversation`（服务端会话）与 `input` 里的 `item_reference`
    ——它们依赖上游保存的状态，网关无从重建，会回明确的 400 `unsupported_parameter`。
- **`openai-responses` 模型（如 `gpt-6-astra`）**：请求**原样透传**到上游 `/responses`，
  `previous_response_id` / `conversation` 能否续接取决于上游
  （2026-11-19 该模型持续 429，尚未验证；桌面端自身从不使用链式续接）。

**受益的客户端用法**：任何在「同一視窗」连续多轮、且依赖服务端状态的 Responses 客户端
（典型是 Claude Code / Codex 经反代、magpie 插件反代）——现在可以放心用
`previous_response_id` 续接，不必每轮重发整段历史。非链式用法（每轮 `input` 直接带
完整历史）行为不变，不会回歸。

排障：請求日志（控制台日志页「消息」列 / `<数据文件>-logs.jsonl` 的 `messages`、
`system` 字段）记录每次请求回喂的消息条数与是否带系统提示，不含正文——
链式续接时该条数会随轮次增长，说明历史确实被重放。

2026-11-22 起同一行日志还带 **`body_bytes`**（实际发往上游的请求体字节数）与
**`body_near_cap`**（达到 `DEEPAI2API_CONTEXT_BODY_BYTES` 的 90% 时置 `true`，
只在日志里提示、不阻断）。`messages` 正常增长但模型仍失忆时，先看 `body_bytes`：
超过约 196 KB 就是上游静默砍头（issue #7），而不是网关丢历史。

Claude Code 只需：

```bash
export ANTHROPIC_BASE_URL=http://127.0.0.1:10087
export ANTHROPIC_API_KEY=sk-...        # 网关密钥；未签发时可留空
export ANTHROPIC_MODEL=glm-5.3-flash
```

### DeepAI 原生能力扩展

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/v1/deepai/theos` | 当前账号免费额度 / 余额 |
| GET | `/v1/deepai/usage` | 订阅与用量摘要 |
| POST | `/v1/deepai/web_search` | 上游联网搜索（`{query, num_results}`） |
| POST | `/v1/deepai/find_service` | 上游服务推荐 |

### 管理接口（账号 / 入池）

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| POST | `/admin/api/accounts` | `action=enroll`：验活并写入号池 |
| POST | `/admin/api/checkin` | 配额探测。可选 `id` / `group` / `stale_only` / `limit`；响应带 `total` / `probed` / `remaining`，按批推进 |
| POST | `/admin/api/login/start` | 发起浏览器登录，返回 `session_id` / `port` / `loopback` / `expires_in` |
| POST | `/admin/api/login/poll` | 轮询登录结果（本机回调自动收，`status=pending\|ok\|error\|expired`） |
| POST | `/admin/api/login/callback` | **粘贴回调 URL 完成登录**（`{session_id, callback}`）。Docker / 远程部署必用：浏览器落在 `127.0.0.1:<port>/auth-callback?code=…&state=…` 打不开时，把地址栏完整 URL 粘回来即可 |

#### 已下线的端点：`/admin/api/state`

`GET /admin/api/state` 曾在一次响应里**拍平下发全部账号**（603 个号约 430KB），
已于 2026-05-20（RFC 8594 的 `Sunset`）下线，现在返回 `410 Gone`。
它由两个端点取代：

| 需要什么 | 用哪个 |
| --- | --- |
| 元信息（设置 / 统计 / 密钥 / 模型 / 车道 / 延迟） | `GET /admin/api/overview` |
| 账号列表（分页 + 过滤 + 排序） | `GET /admin/api/accounts?offset=&limit=&group=&status=&q=&sort=&order=` |

如果你还有脚本在用旧端点，迁移就是两次请求：

```bash
curl -s -H "X-Admin-Password: $DEEPAI2API_ADMIN_PASSWORD"      http://127.0.0.1:10087/admin/api/overview
curl -s -H "X-Admin-Password: $DEEPAI2API_ADMIN_PASSWORD"      'http://127.0.0.1:10087/admin/api/accounts?limit=200&offset=0'
```

### 控制台

| 路径 | 说明 |
| --- | --- |
| `/` | 内嵌单页控制台（无外部依赖） |
| `/ping` | `pong` |
| `/health` | 账号 / 模型 / 机器指纹摘要；`ready` / `degraded` / `blocked` 三个计数（口径见 §6.4 末段） |
| `/metrics` | Prometheus 文本格式指标（与 `/admin/api/*` 同级鉴权）：请求量/状态码分类、**分端点延迟直方图**、**签到耗时/阻塞/超时**、**上游错误码分布**、**形状兜底次数**、在途请求、号池分布、goroutine/堆 |
| `/admin/api/version` | 当前版本 + 最新 release 比对（服务端代查 GitHub，缓存 6h；离线时只回当前版本，不影响控制台） |
| `/admin/api/*` | 账号、密钥、日志、统计、设置、配额、登录 |

> 每个响应都会回写 `X-Request-Id`（合法的传入值原样回显），访问日志是一行一条
> JSON（`event=access` 带 `request_id` / `status` / `ms` / `bytes`），可直接 `jq` 过滤。

---

## 四、快速开始

```bash
# 1) 编译（产物在 bin/；也可直接 go build -o deepaicode2api.exe .）
make build

# 2) 直接启动（会自动导入桌面端凭据 + 拉取模型目录）
./bin/deepaicode2api

# 3) 打开控制台
#    http://127.0.0.1:10087/

# 4) 添加账号（控制台）
#    「浏览器登录」用 Google / Apple / 邮箱密码登录上游；
#    也可以「导入本机桌面端」或「手工 API Key」
```

> **`.env` 现在由网关在启动时加载**（此前只在部分路径惰性加载，
> 写进 `.env` 的 `DEEPAI2API_MASTER_KEY` 对主进程不生效）。
> 状态文件里是上游 API Key 与账号密码，Windows 上 `0600` 不设 ACL，
> 因此首次部署就应开启加密。**先停掉正在运行的网关**，再执行：

```bash
printf 'DEEPAI2API_MASTER_KEY=%s\n' "$(openssl rand -hex 32)" >> .env
./bin/deepaicode2api -seal -yes   # 把存量明文（含 append-only 历史日志）封存
./bin/deepaicode2api              # 之后每次保存都自动加密
```

`-seal` 会打印密钥指纹（sha256 前 8 位）便于核对，并校验每个文件不再含明文；
**主密钥必须与备份一起保管，丢失后号池不可恢复**。
只改环境变量而不跑 `-seal` 时，主文件会在下一次保存时自动重写为密文，
但历史日志行会保持明文（读取端兼容混合格式，不会报错）。

启动日志示例：

```text
状态文件: …/deepaicode2api-data.json
机器指纹: a1b2c3d4e5f6…（用于上游按天发放免费 Theos）
已导入账号 desktop-…（~/.deepai-code/userdata/secrets/deepai-account.bin）
模型目录已更新：28 个模型，默认 glm-5.3-flash
DeepAICode2API dev 已启动: http://127.0.0.1:10087/
```

### 调用示例

```bash
# OpenAI 风格
curl http://127.0.0.1:10087/v1/chat/completions \
  -H 'Content-Type: application/json' \
  -d '{"model":"glm-5.3-flash","messages":[{"role":"user","content":"hi"}]}'

# 流式
curl -N http://127.0.0.1:10087/v1/chat/completions \
  -H 'Content-Type: application/json' \
  -d '{"model":"glm-5.3-flash","messages":[{"role":"user","content":"hi"}],"stream":true}'
```

Python OpenAI SDK：

```python
from openai import OpenAI
client = OpenAI(base_url="http://127.0.0.1:10087/v1", api_key="sk-...")
print(client.chat.completions.create(
    model="glm-5.3-flash",
    messages=[{"role": "user", "content": "hello"}],
).choices[0].message.content)
```

---

## 五、命令行参数与环境变量

| 参数 | 环境变量 | 默认值 | 说明 |
| --- | --- | --- | --- |
| `-host` | `DEEPAI2API_HOST` | `127.0.0.1` | 监听地址 |
| `-port` | `DEEPAI2API_PORT` | `10087` | 监听端口 |
| `-data` | `DEEPAI2API_DATA_PATH` | 用户配置目录 | 状态文件 |
| `-deepai-dir` | `DEEPAI2API_DATA_DIR` | 自动探测 | 桌面端数据目录 |
| `-admin-password` | `DEEPAI2API_ADMIN_PASSWORD` | 空 | 管理密码 |
| `-allow-insecure` | `DEEPAI2API_ALLOW_INSECURE` | `false` | 允许非回环监听但不设管理密码 |
| `-import` | — | — | 仅导入凭据后退出 |
| `-no-import` | — | — | 启动时不自动导入 |
| `-seal` | — | — | 把状态/统计/日志中的存量明文封存为密文后退出（需 `DEEPAI2API_MASTER_KEY`） |
| `-yes` | — | — | 配合 `-seal` 确认不可逆操作 |
| `-version` | — | — | 打印版本 |

其他环境变量：

| 变量 | 说明 |
| --- | --- |
| `DEEPAI_API_KEY` | 直接添加一个上游账号（可选） |
| `DEEPAI_MACHINE_ID` | 覆盖机器指纹原始 UUID（容器部署必填） |
| `DEEPAI2API_MASTER_KEY` | 状态文件加密主密钥（64 位 hex / base64，32 字节），留空则明文；写在 `.env` 即可，启动时自动加载 |
| `DEEPAI_API_ORIGIN` | 覆盖上游地址（自建测试） |
| `DEEPAI2API_ADMIN_PASSWORD` | 管理密码（控制台 / 管理接口） |
| `PROXY_POOL` | 推理代理池（逗号或换行分隔完整 URL；入池时按账号轮询分配） |
| `PROXY_POOL_FILE` | 代理池文件（每行一个） |
| `DEEPAI2API_ENV_FILE` | 指定 `.env` 路径 |
| `DEEPAI2API_LOGIN_PKCE` | 浏览器登录是否带 PKCE（默认 `true`，与桌面端 0.3.50 一致）；`0` 退回只发 `state`+`port` 的旧形态 |
| `DEEPAI2API_HISTORY_TTL_HOURS` | `/v1/responses` 伺服端歷史暫存存活小時數（默认 `24`） |
| `DEEPAI2API_HISTORY_MAX_ENTRIES` | `/v1/responses` 伺服端歷史暫存最多輪次（默认 `500`） |
| `DEEPAI2API_EFFECTIVE_CONTEXT_WINDOW` | 重放有效上下文上限的全域覆寫（优先級最高；默认按模型查表，見上方上下文契約；`0`/負數關閉） |
| `DEEPAI2API_CONTEXT_WINDOWS` | 按模型覆寫有效上下文上限，JSON 物件如 `{"mimo-2.6-flash":140000}`（值 `0`/負數 = 該模型不設上限；格式錯誤忽略） |
| `DEEPAI2API_CONTEXT_BODY_BYTES` | 單次請求體安全上限（字節，默認 `196608` ≈ 192 KiB）：上游實測約 196 KB 起靜默截斷開頭（issue #7）；`0`/負數關閉 |
| `T3CODE_HOME` / `DEEPAI_CODE_HOME` | 桌面端数据目录 |

### 诊断工具

```bash
go run ./cmd/probe -usage                 # 凭据 + 模型目录 + 配额
go run ./cmd/probe -prompt "hello"        # 真实调用一次
go run ./cmd/probe -prompt "hi" -stream   # 流式调用
go run ./cmd/probe -key <api-key>         # 直接验证某个 Key
```

---

## 六、账号接入与号池治理

> **邮箱注册链路已移除**（2026-11-10）：控制台「批量注册」页、
> `/admin/api/register`、`cmd/register` 与 `internal/register` 包均已删除。
> 账号接入只保留浏览器登录、桌面端导入与手工 API Key 三种方式；号池治理
> （分组、代理池、机器指纹、额度复探）不受影响。历史设计与实测留在
> `docs/changes.md` / `docs/lessons.md`，代码不再提供该能力。

### 6.1 账号接入方式

| 方式 | 入口 | 说明 |
| --- | --- | --- |
| 浏览器登录 | 控制台「浏览器登录」/ `POST /admin/api/login/start` | DeepAI 登录页支持 Google / Apple / 邮箱密码；本机 `127.0.0.1` 回调换 Key，入池自动分配机器指纹与代理 |
| 桌面端导入 | 启动参数 `-import` / 控制台「导入本机桌面端」 | 读取桌面端 `deepai-account.bin` |
| 手工 API Key | `POST /admin/api/accounts {"action":"manual"}` | 只填 Key，随后用 `assign-machine` / `assign-proxy` 补全 |

浏览器登录的完整链路见 `docs/recon.md` §4.2。Google 侧登录必须在浏览器里完成：
上游 OAuth 的 `redirect_uri` 固定在 deepai.org，code 换 token 需要它的
`client_secret`，服务端无法代办。

### 6.2 号池分组与轮询

- 账号有 `group`（默认 `default`）；
- API Key 也有 `group`，请求进来后只会在对应分组内轮询；
- 未绑定分组的 Key 使用全部分组；
- 管理员在控制台创建密钥时可指定分组。

这样可以用一个网关同时服务多个批次 / 租户，而不会串号。

### 6.3 代理池

- 推理与入池共用同一代理池：`PROXY_POOL`（逗号 / 换行分隔的完整 URL）或
  `PROXY_POOL_FILE`（每行一个）；
- 浏览器登录入池时按轮询自动分配一个出口；存量账号可随时补绑：
  `POST /admin/api/accounts {"action":"assign-proxy"}`
  （默认只补空值；`"reassign":true` 覆盖、`"clear":true` 回滚；
  可用 `"id"` 或 `"group"` 限定范围）；
- 推理侧按账号绑定的代理调用上游（`Server.clientFor`），管理接口只展示
  `host:port`，不泄露代理凭据；
- 多样化出口有价值：每日免费 Theos 是**按网络（出口 IP）限额**发放的
  （见 `docs/recon.md` 的 `network_bound`），把 14 个号分散到 12 个出口后，
  有额度的账号从 5 个提升到 10 个。

### 6.4 机器指纹（为什么每号一个）

上游按 `X-DeepAI-Code-Machine` **每天发放一次**免费 Theos，因此：

- 每个账号在入池时生成一个独立的机器 UUID（`deepai.NewMachineUUID`），
  之后 `/theos` 与推理全程使用它（`Account.MachineID`）；
- 没有账号级指纹时才回退到进程级 `MachineID()`（桌面端导入 / 历史账号）；
- 共用指纹时，同一台机器上的多个账号会互相抵消额度 —— 这是旧版本
  池内 `granted=0` 的根因；
- 存量账号可一键补齐（只补空值，不覆盖已有指纹）：

```bash
curl -s -X POST http://127.0.0.1:10087/admin/api/accounts   -H "X-Admin-Password: $DEEPAI2API_ADMIN_PASSWORD"   -d '{"action":"assign-machine"}'
# {"ok":true,"assigned":20}
```

补齐后会自动异步重探配额。调度侧也会优先挑有额度的账号（`hasQuota`），
已知 `balance=0` 的账号只在全组都没额度时才兜底。

### 6.4.1 额度自动复探

上游按 `refresh_at` 每天发放一次额度。早期只能人工点「刷新全部配额」，
653 个号一次要 11 分钟，还会因为关掉页面（`r.Context()` 取消）中断一半。
现在三层自动机制覆盖：

| 层 | 触发 | 范围 | 失败处理 |
| --- | --- | --- | --- |
| 入池复探 | 入池后 45s / 2min / 6min | 新账号 | 采信成功结果 |
| 按需复探 | 推理选中「快照已过期」的账号 | 仅用到的号，每 15min 最多一次 | 不改 `Status` / `Failures`（上游抖一下不该被调度误判） |
| 后台守护 | 每 5 分钟一批（≤24 个） | `refresh_at` 已过 / 从未探测 / 超 6h 未探 | 同上 |

三条路径共享一张在途去重表与**全局 4 个探测槽位**：653 个号的快照同时过期时，
不会因为一波流量就叠出上百个并行 `/theos`（那是最容易被风控的形态）。
拿不到槽位的会在下一轮补上。

`/admin/api/checkin` 改为分批 + 进度语义：

```bash
# 只探 g2 分组里额度已过时的账号，本批最多 50 个
curl -s -X POST localhost:10087/admin/api/checkin   -H "X-Admin-Password: $DEEPAI2API_ADMIN_PASSWORD"   -d '{"group":"g2","stale_only":true,"limit":50}'
# → {"ok":true,"total":120,"probed":50,"remaining":70,"results":[…]}
```

控制台「刷新全部配额」会按 `remaining` 自动翻页，直到整池都探过；
`/health` 额外给出 `quota_fresh`（15 分钟内探过）与 `quota_due`（守护会去重探）。

`/health` 的三个账号计数（`ready` / `degraded` / `blocked`）口径：

| 字段 | 含义 |
| --- | --- |
| `ready` | 停用的不算；`blocked` 不算（不会自愈）；`error` 只有**错误还新鲜**（10 分钟内）时不算 |
| `degraded` | `error` 且错误在 10 分钟内 —— 控制台胶囊会额外显示「N 个近期失败」 |
| `blocked` | 被上游策略拦截（`social_login_required` 等），需要人工处理 |

`status=error` 只是「上一次请求失败」，不代表账号坏了：调度器只跳过 `blocked`，
所以 `error` 号照常参与轮询；超过 `deepai.ErrorStaleAfter`（10 分钟）没有再失败，
后台 30s 的巡检会把它改回 `ok`。

瞬时失败（EOF / reset / 超时 / 429 频控 / 5xx / 容量拒绝 / 签到限流）连 `error`
都不会标，只累加 `failures` 与 `last_err`；**今日额度到顶**（`Too many requests today`）
例外 —— 它当天不会自愈，所以会标成 `error` 并计入 `degraded`，直到次日 `refresh_at`。

### 6.5 实测：补齐机器指纹前后的额度对比

同一份号池（21 个账号：1 个桌面端导入 + 20 个历史邮箱号，11 个代理），
用 `assign-machine` 补齐前后各跑一次 `/admin/api/checkin`：

| 指标 | 补齐前 | 补齐后 |
| --- | --- | --- |
| 有独立机器指纹的账号 | 0 / 21 | **21 / 21**（21 个互不相同） |
| `balance > 0` 的账号 | **0 / 21** | **20 / 21** |
| 典型配额 | `balance=0.00000 granted=0.00000` | `balance=200.00000 granted=200.00000` |
| 配额刷新 | — | `refresh_at=2026-11-07T04:03:03Z`（次日 UTC 04:03） |
| 连续 8 次真实推理 | — | **8/8 HTTP 200**，落在 **8 个不同账号** |

唯一 `balance=0` 的是桌面端导入的账号（`source=desktop`，单独在
`default` 分组）——它当天在原机器指纹下的额度已被先前使用消耗。

> 注意：`/theos` 的 `balance` 精度是 5 位小数，单次请求成本约 `1.8e-6`，
> 所以小额调用看不出余额下降；要看消耗请查 `/v1/deepai/usage`。

### 6.6 验收口径

| 维度 | 怎么看 |
| --- | --- |
| 额度 | `assign-machine` 后 `balance > 0` 的比例；三层复探是否在 10 分钟内补齐 |
| 推理 | 真实模型 200 率与 provider 分支（判据见 `docs/recon.md` §3.1） |
| 健康 | `/health` 的 `quota_fresh` / `quota_due`，日志里的 `quota` / `pool` 事件 |
| 调度 | 同一分组内轮询是否公平；`blocked` 是否被跳过 |

同配置复跑两批以上才有对照意义（`docs/lessons.md` §4.4）。

### 6.7 安全与合规

- 号池只用于管理**自己的** DeepAI 账号；请遵守上游服务条款；
- 状态文件权限 `0600`；但 **Windows 上 `0600` 不设 ACL**，同机其他用户仍可读，
  因此强烈建议配置 `DEEPAI2API_MASTER_KEY` 开启整文件加密（见 §七）；
- 上游按机器发放每日免费额度，本项目不绕过该限制。

---

## 七、控制台

- **概览**：请求量、失败数、总 token 入/出、**今日 token 入/出**、**缓存命中率**、
  上游成本、号池就绪数、模型目录（免费 / 付费、Theos、上下文、端点）。
  Token 与字符按 **K / M / B** 压缩显示（3 位有效数字，悬停看精确值）；模型目录保留精确整数。
  「今日」按**本地日期**统计，跨天自动清零；缓存命中率 = 上游报告的
  `cached_tokens / prompt_tokens`（三种上游形态都已归一，见 `gateway.ParseUsage`），
  没有样本时显示 `—` 而不是 0.0%。
- **账号**：导入桌面端凭据、浏览器登录、手工添加 API Key、刷新配额；
  列表支持**分组 / 状态 / 关键字**筛选（服务端分页 + 排序）；
  行内菜单除启停 / 删除 / 清错外，还接了存量治理：**补绑代理 / 清除代理、
  分配机器指纹（只补空值）、改分组、标记已验证**。
  「脱敏显示」开关会把邮箱（`u***@domain`）与用户名打码，偏好记在 localStorage；
  搜索/筛选仍按完整值匹配，打码只影响显示（方便截图、录屏、远程协助）。
- **密钥**：签发 `sk-…` 网关密钥；只在创建时显示一次；可**改绑分组**。
  **未签发任何密钥时，仅本机回环允许无密钥调用；非回环来源一律 `503 setup_required`。**
  若监听非回环地址且尚无密钥，启动时会自动生成一个 `bootstrap` 密钥并打印一次。
- **日志**：端点、模型、流式、状态、耗时、字符数、token、成本、账号；
  支持按**端点 / 账号 / 最小状态码 / 关键字 / 起始时间**过滤（字符与 token 同样是 K/M）。
- **设置**：超时、重试、日志保留、CORS、兜底模型、严格模型模式、模型别名、管理密码，
  以及按密钥限流（RPS / burst / 并发上限，0 = 不限）。
- **顶栏版本号**：显示构建版本（`v0.1.2`），点击跳转 releases 页；控制台启动时会
  向 `/admin/api/version` 查一次最新 release（服务端缓存 6 小时），有新版则高亮提示。
- **登录**：管理口令走独立的 `/login` 页（入口 `POST /admin/api/auth` 校验），
  成功后才进控制台；口令只保存在**本标签页**的 `sessionStorage`，顶栏只有「退出」。
  任一管理接口返回 401 时自动清口令并回登录页。未设管理口令时不挡人（本机直通）。
  是否启用登录门由 `/health` 的 `admin_required` 决定。

安全默认值：

- 监听**非回环**地址时必须设置管理密码，否则拒绝启动（如需绕过用 `-allow-insecure`）；
- 未签发密钥时，非回环来源的数据面请求返回 `503 setup_required`，不是放行；
- 未设置管理密码时，管理接口**只允许本机环回**；
- 写操作做同源校验，阻断 CSRF；
- CORS 默认仅允许本机来源，不提供 `*`；
- 状态文件权限 `0600`，控制台只展示脱敏后的 Key。
- 配置 `DEEPAI2API_MASTER_KEY` 后状态文件整体 AES-256-GCM 加密，
  磁盘上不再出现明文上游 Key / 账号密码；已加密的文件在缺少或错配主密钥时
  **拒绝启动**，不会静默重置成空状态。
- `-seal` 会把 append-only 日志里的**历史明文行**一并重新封存（逐行校验、
  幂等）；只改环境变量不会回填旧日志。
- 状态文件默认落在 `os.UserConfigDir()`（Windows `%AppData%`、Unix `~/.config`），
  避免落在 Program Files 或共享盘；旧位置已有文件时会继续沿用，不会丢号池。

---

## 八、架构与目录

```text
main.go                     装配：参数 → 存储 → 路由 → 信号（不再放业务）
tasks.go                    后台任务：落盘 / 目录刷新 / 额度守护 + 导入与封存命令
env.go                      启动参数与环境变量解析（默认值集中处，对照 §五）
cmd/probe/                  诊断工具
internal/deepai/            上游客户端（协议、SSE、机器指纹、凭据导入）
internal/envfile/           .env 加载（启动时生效）
internal/registry/          模型目录解析与别名
internal/store/             状态持久化 + 账号池调度（拆分后单文件 ≤400 行）
  ├─ store_open.go         Open / Save / 落盘快照 / 原子写
  ├─ store_accounts.go     账号 CRUD / 分页 / 调度入口
  ├─ store_keys.go         网关密钥
  ├─ store_models.go       模型目录 / 机器指纹 / 设置
  ├─ store_telemetry.go    统计与日志查询
  ├─ reseal.go             -seal：存量明文（含历史日志）封存
  └─ state_seal.go         AES-256-GCM 信封 / 密钥指纹
internal/proxy/             代理池（轮询 / 脱敏标签 / 文件读取）
internal/pool/              选号策略（分组轮询 / 配额判定 / 冷却窗口），不依赖 Store 即可测
internal/parallel/          有界并发扇出（并发上限 / ctx 取消 / 按下标回填）
internal/retry/             退避重试（阈值 / 抖动 / 不可重试错误判定）
internal/httpx/             纯 HTTP 工具与中间件（JSON 响应 / 错误信封 / 解码 / gzip / ETag）
internal/gateway/           协议转换层：字段映射 + 用量解析，不依赖 HTTP / store / 号池
  ├─ anthropic_types.go    Anthropic 请求模型（Messages / tools）
  ├─ anthropic_translate.go Anthropic ⇄ Chat 双向字段映射
  ├─ anthropic_stream.go   流式内容块管理器（thinking / text / tool_use）
  ├─ responses_translate.go Responses ⇄ Chat 字段映射
  └─ usage.go              Usage + 上游用量解析 + JSON 宽松取值
internal/server/            HTTP 路由、流式收发、控制台（装配层）
  ├─ chat.go                OpenAI Chat Completions（透传 + 归一化）
  ├─ anthropic.go           Anthropic Messages 入口
  ├─ responses.go           Responses API 入口（透传 / Chat 合成）
  ├─ responses_stream.go    Responses 流事件生成器（写 sseWriter，故留在本包）
  ├─ upstream.go            openModel：选号 / 占槽 / 失败换号 / 错误分类
  ├─ accounts.go            账号槽位、并发上限、准入排队
  ├─ models.go              模型目录解析与别名回落
  ├─ sse.go                SSE 写出与 [DONE]
  ├─ reqlog.go             单请求埋点与统计落盘
  ├─ probe.go               额度探测（按需 / 守护 / checkin 分批）
  ├─ extensions.go          配额、用量、搜索、语音
  ├─ login.go               浏览器登录（RFC 8252 本机回调）
  ├─ admin_accounts.go      账号管理入口：GET 分页 / POST 按 action 分发
  ├─ admin_actions.go       10 个 action，各自一个最小请求结构体
  ├─ gateway_alias.go       给 gateway 的包内短名（类型别名，迁移期兼容）
  ├─ web.go                 控制台资源路由白名单（资源本体在 web/ 包）
  └─ web/                   控制台静态包（assets.go：go:embed + ETag + CSP）
       ├─ index.html        页面标记（301 行）
       ├─ style.css         样式（设计令牌 + 布局）
       ├─ js/app.js         装配层：取数 / URL 状态 / 启动顺序（231 行）
       ├─ js/events.js      事件接线：导航 / 顶栏 / 筛选 / 排序 / 语言 / 脱敏
       ├─ js/theme.js       主题、侧边栏收起、移动端抽屉
       ├─ js/pager.js       账号分页控件（页码 / 每页条数）
       ├─ js/i18n.js        字典 + 文本替换（零依赖）
       ├─ js/format.js      纯格式化（零依赖）
       ├─ js/state.js       共享可变 UI 状态（分页游标）
       ├─ js/dom.js         DOM 助手 + 行内菜单
       ├─ js/table.js       表头排序状态与纯函数
       ├─ js/api.js         管理接口调用（鉴权 / 错误归一 / 去重 / 超时重试）
       ├─ js/ui.js          toast
       ├─ js/actions.js     账号 / 密钥变更 + 配额探测（刷新通过 setReload 注入）
       └─ js/render/        四个渲染模块：accounts / keys / logs / settings
```

分层原则：

- **`deepai`** 只关心上游协议（路径、请求头、错误语义、SSE），不知道号池存在；
- **`store`** 只负责状态与调度（持久化、分组、失败换号、配额判定），不发网络请求；
- **`server`** 只做协议转换与鉴权，账号选择一律通过 `Store`；

这样上游协议变动只影响 `deepai`，调度策略变动只影响 `store`，
测试可以分别对着假上游和临时状态文件跑。

---

## 九、部署（Docker）

```bash
docker build -t deepaicode2api .
docker run -d --name deepaicode2api \
  -p 10087:10087 \
  -e DEEPAI_API_KEY=<api-key> \
  -e DEEPAI_MACHINE_ID=<原始机器UUID> \
  -e DEEPAI2API_ADMIN_PASSWORD=<管理密码> \
  -v $PWD/data:/data \
  -e DEEPAI2API_DATA_PATH=/data/deepaicode2api-data.json \
  deepaicode2api
```

**长期部署建议用 `scripts/deploy.sh`**（build → 不可变 tag → up → 等健康检查）：

```bash
sudo install -m 600 /dev/null /etc/deepaicode2api.env   # 机密（账号 / 口令 / 代理 / 机器指纹）
sh scripts/deploy.sh v0.1.13                            # 部署；失败会打印日志并给出回滚命令
DEEPAI2API_IMAGE=<旧 tag> sh scripts/deploy.sh           # 回滚到指定镜像
```

- 机密统一放 `DEEPAI2API_ENV_FILE`（默认 `/etc/deepaicode2api.env`，0600），
  **不要**留在代码目录；`.env` 只放 `VERSION` / `DEEPAI2API_IMAGE` / `DEEPAI2API_ADDR` 这类部署参数；
- 容器自带 `healthcheck`（镜像内 `wget /healthz`，30s 一次）与 `mem_limit: 512m`；
  之前没有 healthcheck，进程假死时 `unless-stopped` 永远不会重启（2026-11-22 线上复盘）；

> `X-DeepAI-Code-Theos: 7` 是当前客户端（0.3.50）的车道。车道门槛由上游随时调整：
> 2026-11-18 是「只有 7」，2026-11-19 已放宽成「≤7 全通过、8 及以上 426
> `lane_version_unsupported`」（见 `docs/recon.md` §3）。发旧车道**仍能跑通**，
> 所以判断协议是否跟进要看 `/theos` 回的 `lane_version`（控制台概览的「上游车道」）。
> `X-DeepAI-Code-Machine` 仍是计量模型的必需头：缺失时返回 `400 machine_required`。
> 容器里通常读不到宿主机的 SMBIOS UUID，因此 `DEEPAI_MACHINE_ID` 在容器部署时**必填**。
> 填桌面端所在机器的原始 UUID 即可复用同一份每日免费额度。
>
> **容器里的「浏览器登录」要走粘贴回调**：上游把浏览器送到
> `http://127.0.0.1:<随机会话端口>/auth-callback?code=…&state=…`，而这个
> `127.0.0.1` 是**宿主机自己的回环**；网关监听在容器内（而且端口是每次登录
> 随机分配的），`-p` 也映射不到容器的 loopback。所以浏览器会停在“无法访问”
> 的页面上 —— 这是预期行为，不是网关坏了。把**地址栏里的完整 URL**粘到控制台
> 「浏览器登录 → 粘贴回调 URL」里（`POST /admin/api/login/callback`），由网关
> 完成 code → Key 兑换并入池。`code` 是一次性的，同一段 URL 只能兑换一次；
> `state` 必须与本次会话匹配（只粘 `code` 会被拒）。
>
> Linux 上也可以用 `--network host` 让容器共享宿主回环，本机回调就能直接工作；
> Docker Desktop（Windows/macOS）不具备这个语义。

---

## 九·五、版本与发布

版本号在构建时通过 `-ldflags -X main.version=...` 注入，缺省为 `dev`：

```bash
make build                       # Makefile 自动取 git describe 作为版本
go build -ldflags "-X main.version=v0.1.2" -o bin/deepaicode2api .
./bin/deepaicode2api -version    # 打印版本后退出
```

- **控制台**顶部显示该版本号；点它打开 releases 页。
- **更新检查**：控制台启动时请求 `/admin/api/version`，服务端代查
  `api.github.com/repos/<owner>/<repo>/releases/latest` 并缓存 6 小时。
  离线、被墙或仓库不可达时只返回当前版本（`error` 字段说明原因），
  不会影响控制台使用，也不会在界面上弹错误。
- **开发构建**（`version=dev`）不做更新比对：否则永远提示「有新版本」。
- **Docker 构建**默认 `VERSION=docker`（解析不出 semver，因此不比对、也不提示升级）。
  想要控制台显示真实版本并在有新版时高亮：

  ```bash
  VERSION=$(git describe --tags) docker compose up -d --build
  # 等价：make docker（自动带上 git describe 的版本）
  ```
- 发版流程：`git tag v0.1.5 && git push origin v0.1.5`，再用
  `sh scripts/release.sh v0.1.5 --publish` 交叉编译各平台二进制并创建 GitHub Release
  （脚本构建前会清掉 `dist/` 里上一版的 `deepaicode2api_*` 产物：`gh release upload`
  传的是 `dist/*`，残留的旧二进制会被一起传上去）。
- **命名规则（硬约束）**：tag 与 Release 标题**一律 `vX.X.X`（三段数字）**，
  **不接任何描述后缀**（不要「v0.1.23 — 修复……」，不要日期、不要中文）。
  描述写进 release notes（`dist/release-notes-<ver>.md`）与 `docs/changes.md`。
  `release.sh` 会校验格式（不合规直接退出），并在 Release 已存在时把标题改回版本号
  本身；控制台的更新比对只看 `tag_name`，与标题无关。

## 十、限制与说明

- `/v1/chat/completions` 与 `/v1/messages` 不存对话，也不补历史。`/v1/responses`
  合成路径会在进程内暂存已完成响应，支持 `previous_response_id` 重放；缓存有 TTL、条目数
  与 64 MiB 总量上限，进程重启后清空。找不到历史、`conversation` / `item_reference`
  仍回明确 400；上下文超限时按「上下文契约」处理；
- **目录标称 320k ≠ 实际可用 320k**：上游在请求体约 196 KB 处静默截断开头的 system /
  最初几轮（issue #7 实测；纯英文约 40k tokens，西里尔文约 69k tokens 仍可读入，
  说明判据是字节而非 token）。网关只对 `/v1/responses` 重放路径强制
  `DEEPAI2API_CONTEXT_BODY_BYTES`（默认 196,608 B）；`/v1/chat/completions` 与
  `/v1/messages` 保持透明代理，超限只在日志里打 `body_bytes` / `body_near_cap`——
  客户端仍应对长会话自行压缩历史；
- `X-DeepAI-Code-Theos` 发 `7`（0.3.41 起，2026-11-24 复核 0.3.48 / 0.3.50 未变）。上游门槛可能放宽：2026-11-19 实测 ≤7 全通过、
  8 及以上 426，此时旧车道「能用」并不代表车道是对的 —— 与上游当前车道的差异由
  `/theos` 的 `lane_version` 观测（`Server.UpstreamLane` / `LaneDrift`，控制台概览可见）；
- 心跳 `X-DeepAI-Code-Beacon` 的 `v=` 必须是当前客户端版本（现为 **0.3.50**，见 `deepai.AppVersion`）；`pi=` 在 0.3.30 起是可选的，但发了不会错；
- 车道探测结果由**当前构建**验证过（`Settings.LaneBuild == deepai.LaneVersion`）时无条件信任，即使它比编译值旧；否则按 `max(存储值, 编译值)` 取较新值。探测序列从编译期车道开始，**不能先试旧车道**；
- 请求体形状拒绝除匹配两版文案外，还兜底 `400 + code=invalid_request_error`；补形状重试仍失败时回传第二个错误；
- Anthropic `budget_tokens` 按量级映射到 `low/medium/high/xhigh/max`（4k/16k/32k/64k 分界），`output_config.effort` 原样透传；2026-11-15 实测上游接受六档；
- 推理请求按桌面端实测值发 `user-agent: node` + `accept: */*`，**不带** Chrome UA / `sec-ch-ua` / `Accept-Language`（推理侧只发桌面端请求头）；
- `X-DeepAI-Code-Machine` 缺失时计量模型返回 `machine_required`，因此无 SMBIOS 的环境必须设置 `DEEPAI_MACHINE_ID`；
- 0.3.19 曾把推理收窄到 **Google/Apple 登录**的账号（邮箱密码号 `403
  social_login_required`）；**2026-11-09 复测已解除**，邮箱号恢复可用。
  网关仍把该错误码当策略终态（标记 `blocked`、停止调度），策略再次收紧时
  无需改代码；看到 `blocked` 时先看 `last_err` 是否仍是该码，再决定是否重新授权；
- 上游免费额度按「机器指纹 + 账号」每天发放，额度耗尽后返回 403；
  机器指纹**每个账号一份**（见 §6.4），共用时只有第一个账号能领到；
- `gpt-6-astra` 等 `openai-responses` 模型必须走 `/v1/responses`；用
  `/v1/chat/completions` 调用会被上游拒绝（网关会原样回传错误）；
- 语音转写上游只接受 `audio/webm` / `audio/mp4`，其他格式请先转码；
- **不实现托管主机控制面** `/deepai_code/connect/*`（设备注册 / 远程主机 / TLS 中继）：
  它要 Ed25519 + X25519 双密钥设备身份与受管主机生命周期，服务的是桌面端
  「把机器变成受管主机」功能，与推理无关（逆向细节见 `docs/recon.md` §2.1）；
- 模型目录与车道探测**必须带一个账号的 `Api-Key`**（2026-11-14 起，此前「免凭据」
  的结论已失效）：`GET /deepai_code/models` 不带 Key 一律 401 `auth_required`。
  目录 / 车道探测都不消耗 Theos，所以网关借号池里一个账号的凭据即可
  （`Server.probeAccount`：优先就绪且带机器指纹的号），没有账号时刷新目录会直接
  报「请先导入账号」而不是把 401 抛给控制台；代理预热把 401 也算成功
  （链路已通），见 `docs/recon.md` §2；
- `/v1/messages/count_tokens` 是字符数估算，不是上游真实 tokenizer；
- 状态文件默认明文保存上游 API Key（与桌面端一致）；配置
  `DEEPAI2API_MASTER_KEY` 后整文件加密，请务必保证主密钥不随备份泄露；
- 浏览器登录会换取上游 API Key 并写入状态文件；可开启整文件加密（见上）；
- 代理 URL 含账号密码，必须只放在 `.env` / 本地台账（已 gitignore），不要提交；
- **上游可达性**（issue #1）：网关不内置翻墙能力，`api.deepai.org` 必须能直连。
  中国大陆用户可以把**上游请求**放进自己的代理：`PROXY_POOL` 或账号专属出口
  （见 §6.3）；浏览器登录仍需本地能打开 DeepAI 登录页，否则用 Docker 的粘贴回调
  完成（见 §九）。网关只复用你已有的网络，不提供也不绕过网络管制；
- **单实例部署**：号池、限流桶、在途计数都在进程内存里，状态文件是单写者
  （`tmp + fsync + rename`）。不要把同一个 `DEEPAI2API_DATA_PATH` 挂给多个副本
  同时写，也不要期待多副本之间共享调度状态——横向扩展需要外置存储，当前不在范围内；
- **数据面限流默认关闭**：控制台「运行参数」可设 `rate_limit_rps`（每密钥每秒请求数）、
  `rate_limit_burst`（桶容量）、`key_concurrency`（每密钥并发上限），填 0 即不限。
  限流按密钥隔离，触发时回 `429` + `Retry-After` + `rate_limit_error`；
- **签到预算**（`checkin_block_ms`，默认 10000）：推理前同步签到最多等多久。
  2026-11-13 线上实测签到阻塞 292 次、均值 4.8s、53 次顶到 10s，是首包延迟的
  主要来源；下调到 2000 可显著改善延迟（代价：这次请求可能带着旧 nonce 发出，
  下一轮继续补，不会失败）；
- **签到预热**（v0.1.13 起）：后台每 45s 扫一遍号池，把「到期」的 `/theos`
  在请求路径之外消化掉（每轮最多 4 个号、2 并发，失败静默）。热路径仍然调
  `EnsureCheckIn`（nonce 回传语义不变），但因为刚回传过，绝大多数会直接返回；
  2026-11-22 线上体检发现 37 小时里 940 次请求被签到阻塞（最长吃满 10s 预算）；
- **出口代理冷却**（v0.1.13 起）：传输失败（代理 504 / EOF / 超时）按代理串记账，
  连续失败 30s→60s→120s…最多 10 分钟冷却；调度时优先换出口
  （`poolsvc.PickAvoiding`），只有池里再没有别的号时才用冷却中的出口，
  不会因此对外 429。冷却数见控制台概览的「出口冷却」；
- **出口串规范化与预检**（v0.1.15 起）：账号里的代理写 `host:port`（无 scheme）
  也能正确解析（缺省补 `http://`）；但**启用专属出口前会先探一次**
  （HTTP CONNECT → 上游 `:443`，6s，结论缓存 10 分钟），探测不过就沿用全局出口
  并留 WARN —— 避免把请求压到「看起来配了、实际穿不出去」的住宅代理上
  （2026-11-22 线上事故：缺 scheme → 解析出 `Host=""` → 16 个号的专属出口
  全部静默失效，流量挤在同一个全局出口上）；
- **预置桌面端形状**（`preset_desktop_shape`，默认关）：对已确认需要桌面端形状的
  调用方（按 API Key 记忆 10 分钟），在首次发送前补 system 消息，省掉一次注定
  失败的 400。关闭时仍走「透传 → 400 → 补形状重试」的学习路径；
- **签到节奏**与桌面端一致（以回传完成时刻为基准，`seconds/2` 默认 300s）；
  之前用「最新 nonce ≠ 已回传 nonce」判断，会退化成 30s 一次（见 `docs/changes.md` §60）；
- **管理口令有防爆破退避**：同一来源连续失败 5 次后进入指数退避（1s 起，上限 30s），
  退避期内即使口令正确也回 `429`；成功一次即清零。若把自己锁在外面，等退避到期
  或重启进程（退避表在内存里）即可；
- **日志是 JSONL**：启动与请求日志都输出一行一条 JSON（`slog`），`msg` 保留原文，
  访问日志带 `request_id` / `status` / `ms` / `bytes`；用 `jq -r .msg` 可还原成人读形式。

### 10.1 已知坑（改动前先看）

以下都是踩过的坑，`docs/recon.md` 有更完整的逆向记录：

| 现象 | 原因 | 不要“修”成什么样 |
| --- | --- | --- |
| 无效 Key 返回 200 且是 HTML | 上游 302 到登录页 | 不要 `CheckRedirect` 跟随；`ErrUseLastResponse` 是刻意的 |
| 换用别的 UA 后「模型变了」 | **2026-11-13 复测已无 UA 分流**（`node` / `curl` / Chrome 响应形状一致）；历史原因见 `docs/recon.md` §3.1 | 不要按旧结论给号池“修”UA；网关继续发 `user-agent: node` 只是与桌面端一致 |
| 响应里缺 `provider` | **已经不能当判据**：2026-11-13 起真实请求也不再发 `provider` / `service_tier` / `cost_details`，`id` 统一 `gen-<32 hex>`，`model` 回 `zai/<slug>` | 别再按 `provider` / `gen_` / cost 判替身（见 `docs/recon.md` §3.1；要验号用模型指纹） |
| 号池「指纹突然变差」 | 没完成签到：只拉 `/theos` 不回传 nonce 等于没签到，会落到替身分支 | 等一轮 `EnsureCheckIn` 续签即可；换机器指纹会重新变成冷状态 |
| `403 check_in_refused` | 当天「重启签到」次数用尽（`restarted its check-in too many times today`） | 不要重试、不要把账号标 error：推理仍可用，明天自动恢复；也别再跑签到体检 |
| `400 This request cannot be accepted.` | 请求体不是桌面端形状的新文案（2026-11-13 起；`node` UA + 本机真实机器指纹 + 缺 system 消息。网关与桌面端同机部署时这就是**常态**） | 别只认旧文案 `not one DeepAI Code sends`：`BodyRejected` 要两种都匹配，否则 `ChatBodyFallback` 不重试（见 `docs/recon.md` §5.2） |
| 上游模型目录默认模型为空 | 字段叫 `default` 不是 `default_model` | 不要加 `default_model` 兼容，会静默拿不到值 |
| 刷新模型目录 / 车道探测返回 401 `auth_required` | **2026-11-14 起 `/models` 要求 `Api-Key`**（此前免凭据）；号池为空时必然 401 | 不要当成「Key 失效」去清号；网关已改为借一个账号的凭据（见 `docs/recon.md` §2.2） |
| 池内账号 `granted=0` | 共用机器指纹，每日额度只发一次 | 不要去掉 `Account.MachineID` 回退逻辑 |
| 长响应被掐断 | 上游心跳 `: keep-alive` 被丢弃 | `streamChat` 要原样透传注释行 |
| 工具调用参数被截断 | 同一事件多行 `data:` 未按 `\n` 拼接 | 用 `deepai.ReadSSE`，不要逐行 `json.Unmarshal` |
| 台账里密码消失 | `Append` 走了 `redactRecord` | 这是设计；密码在 `*-credentials.jsonl` |
| `go test -race` 链接失败（`cannot find 'ld'`） | mingw runtime 只带 `ld.bfd.exe` | 不要求改 PATH；跑 `make race-shim` 补一个同名副本 |
| 非流式请求返回 502「不是合法 JSON」 | 上游**缺省 `stream` 时默认按 SSE 返回** | 必须显式下发 `stream:false`；别把 `stream` 当可选字段略过 |
| 新号入池后 `balance=0` 被长期跳过 | 入池瞬间上游尚未下发额度 | 不要把 0 当事实缓存；保留 45s/2m/6min 复探与 15 分钟陈旧判定 |
| 关掉控制台后配额只探了一半 | 后台任务误用 `r.Context()`，页面一关就取消 | 后台任务一律脱离请求 ctx，用独立超时 |
| 某条出口挂了导致整批 0 额度 | `No exit node` 是出口故障，不是账号坏 | 不要删号重建；用 `assign-proxy` 改绑 |

另：`/v1/chat/completions` 的 `reasoning_content` 是网关补的兼容字段，
上游原生只有 `reasoning`；已存在时不会被覆盖。

## 十一、文档索引

**先看「状态」再看内容**：`维护中` = 与当前代码一致、可当依据；`历史` = 保留的
过程记录，结论可能已被后来的复核推翻（以 `docs/changes.md` 与 `docs/recon.md` 的
最新条为准）。

| 文件 | 内容 | 状态 |
| --- | --- | --- |
| `README.md` | 总览、接口、参数、验收基线与已知坑（本文） | 维护中 |
| `README.en.md` | 上面这份的完整英文翻译（新增 2026-11-22） | 维护中（与中文版同步） |
| `docs/recon.md` | 逆向速查：上游端点、请求头、错误语义、复现命令（替代 HAR） | 维护中（最近复核 2026-11-22） |
| `docs/changes.md` | 2026-11 的改造记录：根因、决策与验收（§71–§77 是最新一轮） | 维护中 |
| `docs/lessons.md` | **工程经验沉淀**：十批压测 → 可复用规则 + 改动前检查清单 | 维护中 |
| `docs/performance.md` | 压测数据与优化方案：单号 E2E、阶段分解、P0/P1/P2 收益测算 | 维护中（数据快照 2026-11-10） |
| `docs/pi-lens.md` | 本仓的 pi-lens 配置说明：关了什么、为什么、怎么验证与恢复 | 维护中 |
| `docs/model-fingerprint-2026-11-10.md` | 号池 22 个模型的指纹实测（判据与复现命令） | 历史快照：目录已变 28 个，判据仍适用 |
| `docs/optimization-plan.md` | 首轮扫描的完整优化方案 | 历史：已全部实施 |
| `docs/optimization-plan-r2.md` | 第二轮扫描（写放大 / 控制台 / 复探）的方案与收益测算 | 历史：已实施 |
| `docs/assets/` | 项目图标：`logo.png`（512，README 用）/ `logo.ico`（16–256 多尺寸，Windows 用）；控制台的 `/favicon.ico` 是同一份（见 `docs/changes.md` §58） |
| `.env.example` | 全部可配置项与说明 |
| `Makefile` | `make build / test / test-race / lint / cover` |

## 十二、License

MIT。本项目仅复刻本机已登录客户端的调用链路，请遵守 DeepAI 的服务条款。

> **客户端自身的声明（0.3.43 新增，0.3.44 / 0.3.45 / 0.3.47 / 0.3.48 / 0.3.50 仍在）**：`server.asar` 的 `src/deepai/api.ts` 在
> 车道头与签到旁边加了一段注释，援引 DeepAI ToS 第 4 条，明写这些请求头与签到是
> 「DeepAI Code 应用自识别用」，**不允许复制进其它客户端**。本项目只是复用本机
> 已登录客户端的既有链路、面向自己的账号，不做分发或代运营；如果你要把这套头
> 用到别处，先自行确认许可。

> **Logo 例外**：`docs/assets/logo.png` / `logo.ico` 取自 DeepAI Code 桌面端自身的应用图标
> （`resources/app.asar` 内 `apps/desktop/prod-resources/icon.png`），版权归其所有者，
> **不在 MIT 授权范围内**；这里只作为「这是哪个上游客户端的网关」的标识使用。
> 如需自行分发，请先确认图标的使用许可，或换成自己的图。
