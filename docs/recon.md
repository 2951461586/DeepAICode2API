# 逆向结论速查

内容来自对 DeepAI Code 桌面端（Electron + 本地 t3code server）的黑盒观察，
以及 `deepai.org.har` 抓包。**本文件是 HAR 的替代品**：HAR 体积大且含本人
凭据，不入库；结论以文本形式沉淀在这里，便于检索与 diff。

> 抓包时间：应用版本 **0.3.20**（车道 4，2026-11-07 发布）。0.3.19 与 0.3.20 的
> wire protocol 逐项比对后完全一致（车道 / 实例 / 机器指纹 / 签到形态），
> 只有 beacon 里的 `v=` 版本号变了。0.3.17（车道 2）已不再被上游接受：
> 所有 `/deepai_code` 端点返回 `426 lane_version_unsupported`。
> 上游随时可能变，字段以代码为准。
>
> 2026-11-09 用 0.3.20 的 `server.asar` 做了白盒复核（请求头 / 机器指纹 /
> 签到 / 响应判据），并重测了 UA 分流：**推理响应已改成 OpenRouter 形状**，
> 「`provider` 出现 = 替身」的旧判据失效 —— 见 §3.1。


## 0. 证据分级

本文每条结论都带标记，**先看标记再看内容**：

| 标记 | 含义 | 怎么复核 |
| --- | --- | --- |
| **【源码级】** | 白盒确认：读的是 0.3.20 客户端 `server.asar`（`src/deepai/*.ts`） | 解包 asar 后对着代码看，可 100% 复现 |
| **【实测】** | 线上真打过，附日期与样本量 | 按文中命令复跑；样本量小的结论会写明 |
| **【推断】** | 由前两类推广而来，**尚未验证** | 照着改代码前先补一次实测 |

没有标记的段落是背景说明（端点清单、字段名），以代码为准。
判据类结论（“看到 X 就说明 Y”）尤其要注意标记 —— 上游改响应形状时，
旧的【实测】会过期（§3.1 就是例子）。

---

## 1. 进程与数据目录【源码级 0.3.20】

| 项 | 值 |
| --- | --- |
| 应用形态 | Electron 壳（`app.asar`）+ 独立 server（`server.asar`，约 200 MB） |
| 数据目录 | `T3CODE_HOME` 或 `~/.deepai-code`；状态目录 `<base>/userdata` |
| 凭据文件 | `<stateDir>/secrets/deepai-account.bin` |
| 凭据内容 | `{"apiKey":"…","username":"…","isPro":false}`，**明文 JSON**，权限 0600 |
| API Key 形态 | UUID（`8-4-4-4-12`） |

自动探测顺序见 `internal/deepai/helpers.go` 的 `DesktopSecretDirs`。

## 2. 上游端点【源码级 0.3.20】

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

## 3. 请求头【源码级 0.3.20】

| 头 | 值 | 作用 |
| --- | --- | --- |
| `Api-Key` | 账号 API Key | **鉴权走这个头，不是 `Authorization`** |
| `X-DeepAI-Code-Theos` | `4` | 计费车道版本（2/3/5 均 426） |
| `X-DeepAI-Code-Instance` | 16 字节随机数的 hex（每进程一次） | 进程实例标识，随所有 JSON 请求发出 |
| `X-DeepAI-Code-Machine` | `sha256(小写原始机器 UUID)` | 按天发放免费额度；计量模型缺失时 `machine_required` |
| `X-Session-Affinity` | 任意会话 id | 会话粘性（body 的 `prompt_cache_key` 同效） |
| `X-DeepAI-Code-Check-In` | 上一次 `/theos` 下发的 32 位 hex nonce | **仅 `/theos`**：签到回传 |
| `X-DeepAI-Code-Beacon` | `v=<版本>;up=<秒>;pi=<0\|1>`（焦点已知时才有 `;focus=<0\|1>`） | **仅 `/theos`**：客户端心跳 |

普通 HTTP 头（很容易忽略，但它是“像不像桌面端”的一部分）：

| 头 | 桌面端（Node fetch）实测值 |
| --- | --- |
| `User-Agent` | `node`（**不是**浏览器 UA） |
| `Accept` / `Accept-Language` | `*/*` / `*` |
| `sec-fetch-mode` | `cors` |
| `sec-ch-ua*` | **不发** |
| `Content-Type` | `application/json`（**无 body 的 GET 也带**；上传/转写走 multipart） |

> 2026-11-09 用 0.3.20 的 `server.asar`（`src/deepai/api.ts`）逐项复核：
> `requestDeepAIJson` 的 header 字面量无条件带 `Content-Type: application/json`，
> 即使 `/models`、`/theos`、`/get_usage_summary` 是 GET；`User-Agent: DeepAI-Code`
> **只**出现在 `/gravity-display-proxy` 一个路径上，`/get_usage_summary` 仍是
> undici 默认的 `node`。心跳 `focus` 由窗口焦点事件决定，未知时整段省略
> （`deepAIBeacon`），所以网关发 `v=…;up=…;pi=…` 即可。
>
> 浏览器 UA + `sec-ch-ua` 只在**网页注册**链路需要（和网页端保持一致）；
> 带到推理请求上等于在 `Api-Key` 接口自报“我不是客户端”。
> 网关的 `deepai.Client.BrowserFingerprint` 就是这条开关：注册 true、推理 false。

要点：

- 车道 4 是硬门槛：对 `/models` `/theos` `/chat/completions` 一视同仁，
  不匹配直接 426「This version of DeepAI Code is no longer supported」。
- `X-DeepAI-Code-Instance` 只在进程启动时生成一次；网关按「进程 / 出口代理」
  各生成一份即可（`clientFor` 按代理缓存 Client），**不要每请求换值**
  （桌面端也不是每请求换）。
- 机器指纹是**额度分配键**：多个账号共用一个指纹时，只有第一个能拿到当日额度。
  本项目因此给每个账号分配独立 UUID（见 README §6.9）。
- 原始 UUID 来源：Windows `Win32_ComputerSystemProduct.UUID`（失败回退 `MachineGuid`）、
  macOS `IOPlatformUUID`、Linux `/sys/class/dmi/id/product_uuid` 或 `/etc/machine-id`。
- 固件占位 UUID（全 0 / 全 f / `03000200-…`）必须过滤，否则指纹不稳定。

### 3.1 User-Agent 决定服务哪个模型【实测 2026-11-09，判据已修正】

这是本项目最重要的一条逆向结论：**上游按 `User-Agent` 分流**，同一个
`model` 会因为 UA 不同而落到不同的后端，且响应里的 `model` 字段照样写
`z-ai/glm-5.3-flash`，肉眼看不出来。

**2026-11-09 重测：分流依然生效，但响应形状变了** —— 上游现在把请求转给
OpenRouter 一类路由（响应带 `id: gen-…`、`usage.cost_details.upstream_inference_*`、
`is_byok`、`native_finish_reason`、`service_tier`），**`provider` 字段变成了
真实后端名**，于是「有 `provider` 就是替身」这条旧判据**已经失效**。

同一账号、同一机器指纹、同一请求体（`glm-5.3-flash`，已签到），只换 UA：

| `user-agent` | 状态 | `provider` | `system_fingerprint` | 分支 |
| --- | --- | --- | --- | --- |
| `node`（桌面端 Node fetch） | 200 | `InferenceNet` / `Inceptron` / `Wafer` … | 仅 vLLM 后端有（`vllm-0.33.0rc1…`），其余 `null` | **真模型** |
| `Go-http-client/1.1` | 200 | **`DeepAI`** | `null` | 替身 |
| `Mozilla/…Chrome/156…` | 200 | **`DeepAI`** | `null` | 替身 |

真 / 替身现在的判据（按可靠度排序）：

1. **`provider == "DeepAI"` → 替身**；其它名字（`InferenceNet` / `Wafer` /
   `Reka` / `BaseTen` / `CoreWeave` / `Makora` / `Modal` / `Morph` /
   `Inceptron` / `OpenInference` …）→ 真模型；
2. `service_tier`：`null` = 真，`"default"` = 替身；
3. `usage.cost_details.upstream_inference_cost`：>0 = 真（实测 0.000019575），
   `0` = 替身（stealth 路由不花钱）；
4. `message.reasoning_details[0].format == "openrouter-stealth-v1"` = 替身，
   它的 `data` 是 JWE，payload 里 base64 解出 `{"endpoint_slug":
   "stealth/crouton-alpha|stealth"}` —— **今天的替身实际是 `crouton-alpha`
   在 stealth 通道上冒充 GLM**（2026-11-08 记录的 FPD 指纹是 `muse-wonder`，
   换了后端）；
5. `system_fingerprint` **不再是判据**：真分支只有 vLLM 系后端才给，替身恒为 `null`。

其它要点：

- 判定开关仍然是 **UA**，与 `Accept` / `Accept-Language` / `sec-fetch-mode`
  无关（实测 `node` + `application/json` 仍在真分支）；
- 两条分支的 `model` 字段都回 `z-ai/glm-5.3-flash`，**不能**拿它判断；
- 替身分支仍更容易被 `finish_reason=length` / `native_finish_reason=max_output_tokens`
  截断（reasoning 吃 token 却不计 content）；
- 推论不变：**推理请求必须发 `user-agent: node`**（网关 `BrowserFingerprint=false`
  那条路径），带 Chrome UA 或 Go 默认 UA 都会被换模型。

**网关链路实测（2026-11-09）**：本地起网关（14 个账号、每号独立机器指纹），
用 `glm-5.3-flash` 非流式打 12 次：`200 × 12`，**12/12 真分支**，
`provider` 在 `Wafer` / `InferenceNet` / `Reka` / `BaseTen` / `CoreWeave` /
`Makora` / `Modal` / `Morph` 之间轮换，单次 1.5–4.7s；日志里 14 个账号
各命中一次（轮询公平）。

## 4. 错误语义【实测 2026-11-07 ~ 11-08】

| 状态 | body | 含义 | 网关处理 |
| --- | --- | --- | --- |
| 409 | `{"error":{"code":"session_busy"}}` + `Retry-After` | 容量拒绝 | 按 `Retry-After` 等待后**换号重试** |
| 401 | — | 鉴权失败 | 记失败、换号 |
| 403 + `email_unverified` | — | 邮箱未验证 | 视为鉴权失败 |
| 403 + `social_login_required` | — | 账号只能用 Google/Apple 登录 | **策略终态**：标记 blocked、换号不再打它 |
| 403 + `check_in_refused` | `DeepAI Code has restarted its check-in too many times today. Try again tomorrow.` | **签到当日次数用尽** | 不改账号状态（推理仍可用，但落在替身分支）；今天不要再重启签到 |
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

## 5. 已知坑【实测 2026-11-07 ~ 11-08】

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

## 5.1 签到（check-in）【源码级 0.3.20 + 实测】

- `/theos` 是签到通道：成功（200）与拒绝（403）都会带
  `check_in:{nonce,seconds=600}`。
- 桌面端缓存 nonce，下一次 `/theos` 带上 `X-DeepAI-Code-Check-In: <nonce>`
  与心跳 `X-DeepAI-Code-Beacon: v=<版本>;up=<秒>;pi=<0\|1>`（`focus` 可选，
  只在窗口焦点状态已知时才出现）。
- 重新签到的时机是 `seconds/2`（默认 300s）；推理前会先确保签到（失败不阻塞）。
- 推理请求本身**不带** check-in / beacon，只带车道、实例、机器指纹与 `Api-Key`。

### 签到是「真实模型分支」的准入条件【实测 2026-11-08，判据待重测】

> ⚠ 2026-11-09 起响应形状变了（见 §3.1），下表当时的「对齐」是按
> 「有 `system_fingerprint`、无 `provider`」判的，今天要改成
> **`provider != "DeepAI"`**。结论（签到是准入条件）尚未用新判据重测。

配合 §3.1（UA 分流）一起看：**`node` UA + 完成签到**，两条都满足才拿到真模型。

| 观测 | 结果（14 个号池账号，免费模型 `glm-5.3-flash`） |
| --- | --- |
| 只拉 `/theos`（不回传 nonce） | **0/14 对齐**，14 个全是 `provider: DeepAI` 替身分支 |
| 拉 nonce + 回传（完整签到） | **14/14 对齐**（`system_fingerprint` 有、`provider` 无） |
| 完整签到但 14 个号**共用同一个新机器指纹** | 10–12/14 对齐（其余落替身分支） |
| 经网关（每号自己的机器指纹 + 推理前签到） | **30 次请求 / 14 个账号，30/30 对齐** |

- 判定按 **(账号, 机器指纹)** 记忆：换机器指纹等于冷启动，必须重新签到；
- 替身分支明显更慢（3–9s vs 对齐后 1–2s），响应里 `model` 仍写 `z-ai/glm-5.3-flash`；
- 网关的 `EnsureCheckIn`：拿到 nonce 但**还没回传**时立刻补一次 `/theos`
  （`ackedNonce` 记住已回传的那个）；之后才退回 `seconds/2` 的续签节奏。
- 冷启动仍有**一次性**窗口：某账号还没有任何挑战时，第一次推理只来得及拉 nonce，
  这一次会落在替身分支，从第二次起对齐（实测 12 次请求里只有第 1 次是替身）。
- **签到是有限资源**：`403 check_in_refused` 表示当天「重启签到」次数用尽，
  要等到明天。所以不要用脚本反复跑「拉 nonce + 回传」；网关的节奏
  （启动探测一次 + 每 `seconds/2` 续签）是安全的。被限流的账号仍能推理，
  只是继续落在替身分支（`provider: DeepAI`）。

## 5.2 请求体形状【实测 2026-11-07】

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
