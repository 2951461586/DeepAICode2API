# DeepAICode2API

把本机 **DeepAI Code 桌面端**的上游模型服务，逆向复刻成一个
**OpenAI / Anthropic 双协议兼容的 Go 网关**。

复用桌面端登录后写入的 API Key，无需另申请密钥；纯 Go 标准库实现，
单文件约 10 MB，零外部依赖。

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

DeepAI Code 是 **Electron + 本地 t3code server** 架构（应用版本 0.3.17）。
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
| 协议头 | `X-DeepAI-Code-Theos: 2`（计费车道版本） |
| **机器指纹** | `X-DeepAI-Code-Machine: sha256(原始机器 UUID)`，用于按天发放免费 Theos |
| 会话粘性 | `X-Session-Affinity`（body 里的 `prompt_cache_key` 也会被上游使用） |
| 容量拒绝 | `409 {"error":{"code":"session_busy"}}` + `Retry-After` |
| 浏览器登录 | `GET /deepai_code/desktop_auth/?state=…&port=…` → 本机 `/auth-callback?code=…` → `POST /deepai_code/desktop_auth/exchange/` |

### 1.2 上游本身就是 OpenAI 兼容的

`chat/completions` 的请求 / 响应与 OpenAI 完全一致，响应还额外带了
OpenRouter 风格的 `provider`、`reasoning`、`reasoning_details`、`cost` 等字段。
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

模型目录**完全由上游下发**（14 个），网关只做缓存与别名。默认免费模型：

| ID | 名称 | 上下文 | 最大输出 | 说明 |
| --- | --- | ---: | ---: | --- |
| `glm-5.3-flash` | GLM 5.3 Flash | 1,048,576 | 131,072 | 默认模型，免费 |
| `deepseek-v4.1-flash` | DeepSeek V4.1 Flash | 1,048,576 | 131,072 | 免费 |
| `mimo-2.6-flash` | MiMo 2.6 Flash | 1,048,576 | 131,072 | 免费 |
| `solar-mini-4` | Solar Mini 4 | 524,288 | 131,072 | 免费，纯文本 |
| `gpt-6-luna` | GPT-6 Luna | 1,048,576 | 128,000 | 免费 |
| `mimo-2.6-pro` | MiMo 2.6 Pro | 1,048,576 | 131,072 | 免费 |
| `crouton-alpha` | Crouton Alpha | 1,048,576 | 131,072 | 免费 |
| `deepai-cyber` | DeepAI Cyber | 65,536 | 8,192 | 免费，纯文本 |

付费（premier）模型包括 `gpt-6.1-sol`、`glm-5.3`、`kimi-k3`、
`claude-opus-5-5`、`claude-fable-5-1`、`gpt-6-astra`（Responses 端点）。

内置别名（可在控制台覆盖）：

- `gpt-4o` / `gpt-4` / `gpt-3.5-turbo` / `claude-3-5-sonnet` / `deepseek-chat` … → `glm-5.3-flash`
- `deepai/<id>`、`openai/<id>` 前缀会被自动剥离

无法识别的模型名**不会报错**，而是回落到「兜底模型」→ 目录默认模型，
保证任意客户端开箱即用。

---

## 三、接口清单

### OpenAI 兼容

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/v1/models` | 模型目录（含 `deepai` 元信息） |
| POST | `/v1/chat/completions` | 流式 / 非流式；支持 tools、图片、`prompt_cache_key` |
| POST | `/v1/responses` | `openai-responses` 模型透传；其余模型自动翻译 |
| POST | `/v1/audio/transcriptions` | 转发上游语音转写（webm / mp4） |
| POST | `/v1/audio/translations` | 同上（别名） |

### Anthropic 兼容

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| POST | `/v1/messages` | 流式 / 非流式，支持 `tool_use`、`thinking`、图片 |
| POST | `/v1/messages/count_tokens` | 本地估算（上游不提供） |

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

### 管理接口（注册 / 入池）

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| POST | `/admin/api/register` | 启动批量注册任务（`count` / `concurrency` / `group` / `mail_backend` / `email` / `password` / `verify`） |
| GET | `/admin/api/register?id=…` | 查询任务进度与逐账号结果 |
| POST | `/admin/api/accounts` | `action=enroll`：验活并写入号池 |

### 控制台

| 路径 | 说明 |
| --- | --- |
| `/` | 内嵌单页控制台（无外部依赖） |
| `/ping` | `pong` |
| `/health` | 账号 / 模型 / 机器指纹摘要 |
| `/admin/api/*` | 账号、密钥、日志、统计、设置、配额、登录 |

---

## 四、快速开始

```bash
# 1) 编译
go build -o deepaicode2api.exe .

# 2) 直接启动（会自动导入桌面端凭据 + 拉取模型目录）
./deepaicode2api.exe

# 3) 打开控制台
#    http://127.0.0.1:10087/

# 4) 注册链路（可选）
cp .env.example .env             # 填 TEMPMAIL_* 或 DEEPAI_REGISTER_DOMAIN
go build -o register.exe ./cmd/register
./register.exe -doctor          # 检查邮箱后端 / 网关 / 机器指纹
./register.exe -count 1         # 注册 1 个账号 → 验证 → 验活 → 入池
./register.exe -count 3 -concurrency 2 -group batch-a
./register.exe -check-proxies   # 检查代理池 → 上游连通性
./register.exe -resume          # 补跑台账里未入池的账号
```

启动日志示例：

```text
状态文件: …/deepaicode2api-data.json
机器指纹: 8751f711d837…（用于上游按天发放免费 Theos）
已导入账号 epsoft-…（~/.deepai-code/userdata/secrets/deepai-account.bin）
模型目录已更新：14 个模型，默认 glm-5.3-flash
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
| `-version` | — | — | 打印版本 |

其他环境变量：

| 变量 | 说明 |
| --- | --- |
| `DEEPAI_API_KEY` | 直接添加一个上游账号（可选） |
| `DEEPAI_MACHINE_ID` | 覆盖机器指纹原始 UUID（容器部署必填） |
| `DEEPAI2API_MASTER_KEY` | 状态文件加密主密钥（64 位 hex / base64，32 字节），留空则明文 |
| `DEEPAI_API_ORIGIN` | 覆盖上游地址（自建测试） |
| `TEMPMAIL_BASE` / `TEMPMAIL_ADMIN_KEY` / `TEMPMAIL_DOMAIN` | CF Temp Email Worker（注册链路收信） |
| `DEEPAI_REGISTER_DOMAIN` | 静态邮箱域名（无收件箱） |
| `DEEPAI_REGISTER_MAIL_BACKEND` | 强制邮箱后端：`cf` / `static` / `none` |
| `DEEPAI2API_GATEWAY` | `register.exe` 默认入池地址 |
| `DEEPAI2API_ADMIN_PASSWORD` | 管理密码（CLI 入池 / 控制台） |
| `DEEPAI_REGISTER_GROUP` | 默认号池分组 |
| `PROXY_POOL` | 注册 / 推理代理池（逗号或换行分隔完整 URL） |
| `PROXY_POOL_FILE` | 代理池文件（每行一个） |
| `DEEPAI_REGISTER_PROXY` | 单代理（覆盖代理池） |
| `DEEPAI2API_ENV_FILE` | 指定 `.env` 路径 |
| `T3CODE_HOME` / `DEEPAI_CODE_HOME` | 桌面端数据目录 |

### 诊断工具

```bash
go run ./cmd/probe -usage                 # 凭据 + 模型目录 + 配额
go run ./cmd/probe -prompt "hello"        # 真实调用一次
go run ./cmd/probe -prompt "hi" -stream   # 流式调用
go run ./cmd/probe -key <api-key>         # 直接验证某个 Key
```

---

## 六、注册 → 登录 → 取 Session → 验活 → 入池

本项目把 `deepai.org.har` 里的网页注册链路复刻成了纯 HTTP 的自动化流水线，
产物直接写入 DeepAICode2API 的轮询号池。

### 6.1 HAR 逆向结论

| 阶段 | 请求 | 说明 |
| --- | --- | --- |
| 预检 | `POST /get_user_login_type`（multipart） | 字段 `email`；返回 `user_exists` / `login_with_password` |
| 注册来源 | `POST /save_custom_signup_data`（JSON） | 对齐网页端，失败可忽略 |
| 注册 | `POST /daily-time-sync/registration/`（multipart） | `email` / `username` / `password1` / `password2`；201 后下发 `sessionid` |
| 登录 | `POST /daily-time-sync/login/`（multipart） | `username`（邮箱）/ `password`；400 = 凭据错误 |
| 取 Session | `GET /daily-time-sync/user/` | 返回 `pk`、`username`、`userprofile.api_key`、`plan` |
| 邮箱验证 | `GET /verify_email?key=…` | 链接来自验证邮件；验证后每日 Theos 才会发放 |
| 验活 | `GET /deepai_code/theos` | 带 `Api-Key` + `X-DeepAI-Code-Machine`，200 即可用 |

关键点：

- `userprofile.api_key` **就是** DeepAICode2API 使用的上游 Key；
- 注册 / 登录都是普通 Cookie 会话，不需要浏览器、不需要 CSRF 头；
- 邮箱验证后 `/theos` 才会返回 `granted > 0`；
- 每日 Theos 按“机器指纹 + 账号”发放：同一台机器当天已领过额度时，
  新账号的 `granted` 可能为 `0`，但免费模型仍然可用。

### 6.2 链路与状态

```text
建临时邮箱 → 预检邮箱 → 注册 → 登录 → 取 Session/API Key
           → 收信验证（可选）→ /theos 验活 → 写入 2API 号池 → 台账/交付物
```

| 状态 | 含义 |
| --- | --- |
| `registered` | 会话已建立，已取到 API Key |
| `verified` | 邮箱验证链接已访问 |
| `keyed` | `/theos` 验活通过（可入池但尚未入池） |
| `enrolled` | 已写入号池 |
| `failed` | 关键阶段失败 |

台账按邮箱并集合并：高等级记录胜出，已赚到的 `api_key` / `account_id` 不会被后续失败覆盖。

### 6.3 CLI

```bash
./register.exe -doctor                     # 环境体检（不出网注册）
./register.exe -count 1                    # 注册 → 验证 → 验活 → 入池
./register.exe -count 5 -concurrency 2 -group batch-a
./register.exe -email a@b.com -password '...'   # 复用已有邮箱（count=1）
./register.exe -resume                     # 补跑台账里未入池的账号
./register.exe -no-enroll                  # 只注册/验活，不写号池
./register.exe -mail-backend static -domain liziai.cloud  # 无收件箱，跳过验证
```

常用参数：`-gateway`、`-admin-password`、`-ledger`、`-result-dir`、
`-verify=false`、`-probe-chat`、`-timeout`、`-delay`。

### 6.4 控制台

控制台新增「注册」页：

- 数量 / 并发 / 分组 / 邮箱域名 / 邮箱后端 / 自动验证；
- 启动后台任务，实时轮询每个账号的阶段、Key、Theos、错误；
- 完成后自动刷新账号池。

### 6.5 邮箱后端

| 后端 | 能力 | 配置 |
| --- | --- | --- |
| `cf` | 建邮箱 + 收信 + 自动点验证链接 | `TEMPMAIL_BASE` / `TEMPMAIL_ADMIN_KEY` / `TEMPMAIL_DOMAIN` |
| `static` | 随机前缀 `@域名`，无收件箱（跳过验证） | `DEEPAI_REGISTER_DOMAIN` |
| `none` | 使用 `-email` 指定的固定邮箱 | 显式 `-email` |

留空自动选择：配置了 CF 就用 `cf`，否则有域名就用 `static`，否则 `none`。

### 6.6 台账与交付物

- `exports/register-ledger.jsonl`：全部尝试（含失败），按邮箱合并；
- `exports/register-ledger-credentials.jsonl`：**仅**密码 / Session / CSRF，供 `-resume` 使用；
- `result/keys.txt`：`email----api_key----account_id`；
- `result/apikeys.txt`：纯 API Key，一行一个；
- `result/success.jsonl`：机器可读成功记录（已脱敏，不含密码）。

台账与交付物写入前会过一遍 `redactRecord`：`password` / `session_key` /
`csrf_token` 不会出现在主台账与 `success.jsonl` 里，因此台账可以直接贴进
issue 或发给别人排查。凭据单独落在 `*-credentials.jsonl`（`0600`，读取时按
邮箱并回），不需要补跑时可直接删掉这一个文件。

这些都已在 `.gitignore` / `.dockerignore` 中排除。

### 6.7 号池分组与轮询

- 账号有 `group`（默认 `default`）；
- API Key 也有 `group`，请求进来后只会在对应分组内轮询；
- 未绑定分组的 Key 使用全部分组；
- 管理员在控制台创建密钥时可指定分组。

这样可以用一个网关同时服务多个批次 / 租户，而不会串号。

### 6.8 代理池与指纹库

- 注册与推理共用同一代理池：`PROXY_POOL`（逗号 / 换行分隔的完整 URL）或
  `PROXY_POOL_FILE`（每行一个）；`DEEPAI_REGISTER_PROXY` 可指定单代理；
- 每个账号轮询分配一个代理并写入台账，补跑时复用历史代理；
- 推理侧按账号绑定的代理调用上游（`Server.clientFor`），管理接口只展示
  `host:port`，不泄露代理凭据；
- 指纹库内置 6 套自洽的 `User-Agent` / `Sec-CH-UA` / 平台 / `Accept-Language`，
  按邮箱稳定分配（同一账号补跑得到同一套请求头）；
- CLI：`-check-proxies`、`-proxy-pool`、`-proxy-file`、`-proxy`、`-no-fingerprint`；
- 控制台「注册」页可直接粘贴代理池并关闭指纹库。

### 6.9 机器指纹（为什么每号一个）

上游按 `X-DeepAI-Code-Machine` **每天发放一次**免费 Theos，因此：

- 每个账号在注册时生成一个独立的机器 UUID（`deepai.NewMachineUUID`），
  存入台账与号池，之后 `/theos` 与推理全程使用它（`Account.MachineID`）；
- 没有账号级指纹时才回退到进程级 `MachineID()`（桌面端导入 / 历史账号）；
- 共用指纹时，同一台机器上的多个账号会互相抵消额度 —— 这是旧版本
  池内 `granted=0` 的根因；
- 存量账号可一键补齐（只补空值，不覆盖已有指纹）：

```bash
curl -s -X POST http://127.0.0.1:10087/admin/api/accounts \
  -H "X-Admin-Password: $DEEPAI2API_ADMIN_PASSWORD" \
  -d '{"action":"assign-machine"}'
# {"ok":true,"assigned":20}
```

补齐后会自动异步重探配额。调度侧也会优先挑有额度的账号（`hasQuota`），
已知 `balance=0` 的账号只在全组都没额度时才兜底。

本机 20 账号压测实测：10 个代理各承载 2 个账号，**20/20 入池**，
19/20 邮箱验证成功（1 个验证链接超时但 Key 验活通过），总耗时 **4m33s**。
该批次在共用机器指纹下 `granted=0`（额度已被先前账号领取）；
分配独立指纹后可各自获得每日额度。

### 6.9 安全与合规

- 注册能力用于管理**自己的** DeepAI 账号；请遵守上游服务条款，控制批次大小与频率；
- 状态文件与台账权限 `0600`；但 **Windows 上 `0600` 不设 ACL**，同机其他用户仍可读，
  因此强烈建议配置 `DEEPAI2API_MASTER_KEY` 开启整文件加密（见 §七）；
- 台账与 `success.jsonl` 不含密码；密码只在 `*-credentials.jsonl` 里，可单独删除；
- 不需要补跑时可删除台账或使用 `-no-enroll` / 不保存密码；
- 上游按机器发放每日免费额度，本项目不绕过该限制。

---

## 七、控制台

- **概览**：请求量、失败数、token 入/出、上游成本、模型目录（免费 / 付费、Theos、上下文、端点）。
- **账号**：导入桌面端凭据、浏览器登录、手工添加 API Key、刷新配额、启停 / 删除 / 清错。
- **密钥**：签发 `sk-…` 网关密钥；只在创建时显示一次。
  **未签发任何密钥时，仅本机回环允许无密钥调用；非回环来源一律 `503 setup_required`。**
  若监听非回环地址且尚无密钥，启动时会自动生成一个 `bootstrap` 密钥并打印一次。
- **日志**：端点、模型、流式、状态、耗时、字符数、token、成本、账号。
- **设置**：超时、重试、日志保留、CORS、兜底模型、模型别名、管理密码。

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
- 状态文件默认落在 `os.UserConfigDir()`（Windows `%AppData%`、Unix `~/.config`），
  避免落在 Program Files 或共享盘；旧位置已有文件时会继续沿用，不会丢号池。

---

## 八、架构与目录

```text
main.go                     启动、导入、后台落盘 / 目录刷新
cmd/probe/                  诊断工具
internal/deepai/            上游客户端（协议、SSE、机器指纹、凭据导入）
internal/registry/          模型目录解析与别名
internal/store/             状态持久化 + 账号池调度
internal/register/          注册链路（stages / runner / ledger / mail backend）
  ├─ web.go                DeepAI 网页注册 / 登录 / Session / 验证
  ├─ mailbox.go            CF Temp Email / static / none
  ├─ pipeline.go           单账号阶段 + 批量并发 + 重试
  ├─ ledger.go             JSONL 台账（并集合并 / 升级保护）
  ├─ gateway.go            入池 HTTP 客户端
  └─ deliver.go            keys.txt / apikeys.txt / success.jsonl
cmd/register/               注册链路 CLI（doctor / count / resume）
internal/server/            HTTP 路由、协议转换、控制台
  ├─ chat.go                OpenAI Chat Completions（透传 + 归一化）
  ├─ anthropic.go           Anthropic Messages（含 tool_use / thinking）
  ├─ responses.go           Responses API（透传 / Chat 合成）
  ├─ extensions.go          配额、用量、搜索、语音
  ├─ login.go               浏览器登录（RFC 8252 本机回调）
  └─ web/index.html         单页控制台
```

设计上刻意对齐参考项目 **Doubao2API**：同样的 store / registry / server 分层，
同样的账号池与失败换号语义，同样的控制台交互与安全策略。

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

> `X-DeepAI-Code-Machine` 是**所有模型请求的必需头**：缺失时上游直接返回
> `426 lane_version_unsupported`。容器里通常读不到宿主机的 SMBIOS UUID，
> 因此 `DEEPAI_MACHINE_ID` 在容器部署时**必填**。填桌面端所在机器的原始 UUID
> 即可复用同一份每日免费额度。

---

## 十、限制与说明

- `X-DeepAI-Code-Machine` 缺失时上游对所有模型请求返回 426，因此无 SMBIOS 的环境必须设置 `DEEPAI_MACHINE_ID`；
- 上游免费额度按「机器指纹 + 账号」每天发放，额度耗尽后返回 403；
- `gpt-6-astra` 等 `openai-responses` 模型必须走 `/v1/responses`；用
  `/v1/chat/completions` 调用会被上游拒绝（网关会原样回传错误）；
- 语音转写上游只接受 `audio/webm` / `audio/mp4`，其他格式请先转码；
- `/v1/messages/count_tokens` 是字符数估算，不是上游真实 tokenizer；
- 状态文件保存上游 API Key 明文（与桌面端一致），请确保本机文件权限与磁盘安全；
- 注册链路会把上游 API Key、网页密码、Session 与完整代理 URL 写入本机状态文件与台账（0600）；
- 代理 URL 含账号密码，必须只放在 `.env` / 本地台账（已 gitignore），不要提交；
- 每日免费 Theos 按机器发放，同一机器上批量注册的后续账号 `granted` 可能为 0，
  但仍可使用免费模型。

## 十一、License

MIT。本项目仅复刻本机已登录客户端的调用链路，请遵守 DeepAI 的服务条款。
