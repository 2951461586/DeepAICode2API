# 逆向结论速查

内容来自对 DeepAI Code 桌面端（Electron + 本地 t3code server）的黑盒观察，
以及 `deepai.org.har` 抓包。**本文件是 HAR 的替代品**：HAR 体积大且含本人
凭据，不入库；结论以文本形式沉淀在这里，便于检索与 diff。

> 抓包时间：应用版本 **0.3.20**（车道 4，2026-11-07 发布）。0.3.19 与 0.3.20 的
> wire protocol 逐项比对后完全一致（车道 / 实例 / 机器指纹 / 签到形态），
> 只有 beacon 里的 `v=` 版本号变了。0.3.17（车道 2）已不再被上游接受：
> 所有 `/deepai_code` 端点返回 `426 lane_version_unsupported`。
> 上游随时可能变，字段以代码为准。

## 1. 进程与数据目录

| 项 | 值 |
| --- | --- |
| 应用形态 | Electron 壳（`app.asar`）+ 独立 server（`server.asar`，约 200 MB） |
| 数据目录 | `T3CODE_HOME` 或 `~/.deepai-code`；状态目录 `<base>/userdata` |
| 凭据文件 | `<stateDir>/secrets/deepai-account.bin` |
| 凭据内容 | `{"apiKey":"…","username":"…","isPro":false}`，**明文 JSON**，权限 0600 |
| API Key 形态 | UUID（`8-4-4-4-12`） |

自动探测顺序见 `internal/deepai/helpers.go` 的 `DesktopSecretDirs`。

## 2. 上游端点

基础地址 `https://api.deepai.org`（可用 `DEEPAI_API_ORIGIN` 覆盖）。

| 方法 | 路径 | 用途 |
| --- | --- | --- |
| POST | `/deepai_code/chat/completions` | 推理（OpenAI chat 兼容） |
| POST | `/deepai_code/responses` | 推理（仅 `api=openai-responses` 的模型） |
| GET | `/deepai_code/models` | 模型目录（无需 API Key） |
| GET | `/deepai_code/theos` | 配额 / 余额 + **签到挑战** |
| POST | `/deepai_code/web_search` | 联网搜索 |
| POST | `/deepai_code/find_service` | 服务发现 |
| GET | `/deepai_code/sessions/list` | 会话列表 |
| GET | `/deepai_code/sessions/get?uuid=…` | 会话详情 |
| POST | `/deepai_code/sessions/delete` | 删除会话 |
| POST | `/deepai_code/sessions/save` | 会话 / 偏好上传（0.3.20 新增，云端同步） |
| POST | `/deepai_code/session_turn` | 会话轮次上报（202/204 视为成功） |
| GET | `/deepai_code/desktop_auth/?state=…&port=…` | 浏览器登录起点 |
| POST | `/deepai_code/desktop_auth/exchange/` | 用 code 换 Key |
| GET | `/get_usage_summary` | 订阅与用量（根路径，不在 `/deepai_code` 下） |
| POST | `/speech_to_text` | 语音转写（表单字段 `voiceRecording`） |
| POST | `/chat_attachments/upload` | 附件上传（需车道头） |
| GET | `/chat_attachments/get?uuid=…` | 附件元信息 / 下载地址 |
| GET | `/gravity-display-proxy` | 赞助位渲染代理（根路径，`User-Agent: DeepAI-Code`） |
| GET | `/gravity-imp` | 赞助曝光上报（根路径） |

## 3. 请求头

| 头 | 值 | 作用 |
| --- | --- | --- |
| `Api-Key` | 账号 API Key | **鉴权走这个头，不是 `Authorization`** |
| `X-DeepAI-Code-Theos` | `4` | 计费车道版本（2/3/5 均 426） |
| `X-DeepAI-Code-Instance` | 16 字节随机数的 hex（每进程一次） | 进程实例标识，随所有 JSON 请求发出 |
| `X-DeepAI-Code-Machine` | `sha256(小写原始机器 UUID)` | 按天发放免费额度；计量模型缺失时 `machine_required` |
| `X-Session-Affinity` | 任意会话 id | 会话粘性（body 的 `prompt_cache_key` 同效） |
| `X-DeepAI-Code-Check-In` | 上一次 `/theos` 下发的 32 位 hex nonce | **仅 `/theos`**：签到回传 |
| `X-DeepAI-Code-Beacon` | `v=<版本>;up=<秒>;pi=<0|1>;focus=<0|1>` | **仅 `/theos`**：客户端心跳 |

普通 HTTP 头（很容易忽略，但它是“像不像桌面端”的一部分）：

| 头 | 桌面端（Node fetch）实测值 |
| --- | --- |
| `User-Agent` | `node`（**不是**浏览器 UA） |
| `Accept` / `Accept-Language` | `*/*` / `*` |
| `sec-fetch-mode` | `cors` |
| `sec-ch-ua*` | **不发** |
| `Content-Type` | `application/json`（上传/转写走 multipart） |

> 浏览器 UA + `sec-ch-ua` 只在**网页注册**链路需要（和网页端保持一致）；
> 带到推理请求上等于在 `Api-Key` 接口自报“我不是客户端”。
> 网关的 `deepai.Client.BrowserFingerprint` 就是这条开关：注册 true、推理 false。

要点：

- 车道 4 是硬门槛：对 `/models` `/theos` `/chat/completions` 一视同仁，
  不匹配直接 426「This version of DeepAI Code is no longer supported」。
- `X-DeepAI-Code-Instance` 只在进程启动时生成一次；网关按进程生成一份即可，
  不要每请求换值（桌面端也不是每请求换）。
- 机器指纹是**额度分配键**：多个账号共用一个指纹时，只有第一个能拿到当日额度。
  本项目因此给每个账号分配独立 UUID（见 README §6.9）。
- 原始 UUID 来源：Windows `Win32_ComputerSystemProduct.UUID`（失败回退 `MachineGuid`）、
  macOS `IOPlatformUUID`、Linux `/sys/class/dmi/id/product_uuid` 或 `/etc/machine-id`。
- 固件占位 UUID（全 0 / 全 f / `03000200-…`）必须过滤，否则指纹不稳定。

### 3.1 User-Agent 决定服务哪个模型（2026-11-08 实测）

这是本项目最重要的一条逆向结论：**上游按 `User-Agent` 分流**，同一个
`model` 会因为 UA 不同而落到不同的后端，且响应里的 `model` 字段照样写
`zai/glm-5.3-flash`，肉眼看不出来。

| 请求特征 | 响应 | FPD 指纹（同一批挑战） |
| --- | --- | --- |
| `user-agent: node`（桌面端 Node fetch） | 有 `system_fingerprint: fp_…`，**无** `provider` | `glm-5.3-flash` / `glm-5.3-flashx` |
| `user-agent: Go-http-client/1.1` | **无** `system_fingerprint`，有 `provider: DeepAI` | `muse-wonder` 89–100% |
| `user-agent: Mozilla/…Chrome/156…`（注册链路指纹） | **无** `system_fingerprint`，有 `provider: DeepAI` | `muse-wonder` 100% |

- 判定开关是 **UA**，与 `Accept` / `Accept-Language` / `sec-fetch-mode` 无关
  （实测 `node` + `application/json` 仍在 GLM 分支）；
- 便宜的旁路判据：**响应里出现 `provider` 且没有 `system_fingerprint`**
  = 被换到了另一条后端；这条在响应体里直接可读，不必跑指纹检测；
- 被替换的分支更慢（同请求 37–52s vs 8–22s），且 `max_tokens` 更容易被
  `finish_reason=length` 截断；
- 推论：**推理请求必须发 `user-agent: node`**（网关 `BrowserFingerprint=false`
  那条路径），带 Chrome UA 或 Go 默认 UA 都会被换模型。

## 4. 错误语义

| 状态 | body | 含义 | 网关处理 |
| --- | --- | --- | --- |
| 409 | `{"error":{"code":"session_busy"}}` + `Retry-After` | 容量拒绝 | 按 `Retry-After` 等待后**换号重试** |
| 401 | — | 鉴权失败 | 记失败、换号 |
| 403 + `email_unverified` | — | 邮箱未验证 | 视为鉴权失败 |
| 403 + `social_login_required` | — | 账号只能用 Google/Apple 登录 | **策略终态**：标记 blocked、换号不再打它 |
| 403 + `network_bound` | `Today's free Theos for your network have all been claimed. More become available at <RFC3339>` | **网络级**免费额度已领完 | 按 message 里的时间等 `refresh_at`；换号通常无效（同出口 IP） |
| 400 + `machine_required` | — | 计量模型缺机器指纹 | 不重试，提示配置 `DEEPAI_MACHINE_ID` |
| 426 + `lane_version_unsupported` | — | 车道版本不对 | 不重试，提示升级网关 |
| 400 + `invalid_request_error` | message = `This request is not one DeepAI Code sends.` | 请求体不是桌面端形状 | 同号按桌面端字段白名单 + 补 system 重试一次 |
| 400 / 404 / 422 | — | 请求本身的问题 | 不重试 |

`Retry-After` 会被夹到 30s 上限（`deepai.RetryAfter`）。

> `social_login_required` 是 0.3.19 新引入的账号策略：上游明确要求账号用
> Google/Apple 登录。实测邮箱密码注册的账号在 `/theos` 与推理端点都会命中，
> 且被拦截的账号连 `/theos` 都会 403（403 body 里仍带 `check_in`）。
>
> `network_bound` 是 0.3.19 实测到的**网络级额度限额**：每日免费 Theos 不是
> “一机一份、彼此独立”，而是同一出口 IP 共享一个当日池子。池子领完后，
> 未拿到额度的账号仍可通过 `/theos`（`balance=granted=0`），但调计量模型会
> 403 `network_bound`；免费模型（`theosPerRequest=0`）不受影响。
> message 里的时间就是下一次发放时间，可直接作为 `Retry-After` 语义使用。
> 实测同一批 14 个账号（同一直连 IP）只有 5 个拿到 200 Theos，其余 8 个等到
> `2026-11-07T17:14Z` 刷新。把号池按账号绑定到注册代理池（12 个出口）后，
> 拿到 200 Theos 的账号升到 10 个 —— 出口多样化能绕开单 IP 的网络级限额。

## 5. 已知坑

- **不要跟随上游 302**：无效 Key 会跳登录页，跟随会把鉴权失败伪装成 200。
  `deepai.Client` 的 `CheckRedirect` 返回 `ErrUseLastResponse`，是刻意的。
- 上游把思维链放在 `reasoning`；OpenAI 生态常用 `reasoning_content`，
  网关会补一份（已存在则不覆盖）。
- `models` 响应里的默认模型字段是 `default`（不是 `default_model`）。
- `/theos` 返回的 `balance` / `granted` 是**字符串**，不是数字。
- `/theos` 还会返回 `lane_version`（数字）与 `check_in:{nonce,seconds}`；
  客户端用「目录里是否存在 `theosPerRequest`」判断目录是否属于当前车道。
- `models` 目录现在每一项都带 `theosPerRequest`（字符串形式的 Theos 单价），
  `theosPerSecond` 档位限定为 `none/low/medium/high`；不存在 `gpt-oss-120b`
  （客户端会主动过滤它）。
- SSE 里同一事件可能有多行 `data:`，按规范用 `\n` 拼接后再解析。

## 5.1 签到（check-in）

- `/theos` 是签到通道：成功（200）与拒绝（403）都会带
  `check_in:{nonce,seconds=600}`。
- 桌面端缓存 nonce，下一次 `/theos` 带上 `X-DeepAI-Code-Check-In: <nonce>`
  与心跳 `X-DeepAI-Code-Beacon: v=<版本>;up=<秒>;pi=<0|1>;focus=<0|1>`。
- 重新签到的时机是 `seconds/2`（默认 300s）；推理前会先确保签到（失败不阻塞）。
- 推理请求本身**不带** check-in / beacon，只带车道、实例、机器指纹与 `Api-Key`。

## 5.2 请求体形状

上游会以 `400 invalid_request_error` 拒绝「不是 DeepAI Code 会发的」请求体：

- 桌面端每个 chat 请求都带 **system 消息**与 **tools**；
- 裸请求（无 system）只有在**本进程已签到**（`/theos` 回传过
  `X-DeepAI-Code-Check-In`）时才被接受；否则回
  `400 This request is not one DeepAI Code sends.`。补一条 system 也能过校验，
  但那是改采样条件——指纹类检测要的是“发得起 + 条件不变”，所以要先签到；
- 每个 reasoning 模型都会带 `reasoning_effort`：`deepAIPiModels` 给它设了
  `thinkingLevelMap:{off:"none"}`，所以「关思考」时发的是 `"none"`，选了档位就发档位值；
  翻译层（Anthropic 的 `thinking`/`output_config.effort`、Responses 的 `reasoning.effort`）
  必须把它映射到 `reasoning_effort`，丢掉就是静默降级成上游默认值；
- 白名单外字段（`user` / `n` / `presence_penalty` 等）在严格节点上可能被拒；
- 网关的兜底策略：仍先原样透传；只有命中该 400 时，才按
  `model/messages/tools/tool_choice/max_tokens/temperature/stream_options/seed/…` 白名单
  重排一次，并补齐 system 消息（`deepai.ChatBodyFallback`，已有 system 时原样保留）。

## 6. 复现方法

```bash
# 读取桌面端凭据并探测（不启动网关）
go run ./cmd/probe -usage

# 用目录里的默认模型跑一次对话
go run ./cmd/probe -prompt "reply OK" -stream

# 直接查配额
go run ./cmd/probe -usage
```
