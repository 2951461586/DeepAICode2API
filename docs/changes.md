# 改造记录（2026-11）

一轮以「可交付、可回归」为目标的改造。在这里按时间顺序记下**根因 → 决策 → 验证**，
方便以后判断某项设计为什么长这样，以及哪些结论是实测出来的、哪些是推断。

完整的方案与验收标准见 `optimization-plan.md`（那是施工前写的计划，本文是施工后的事实）。

### 索引（倒序，最新在前）

**版本 → 条目**（发版时对着这里找本次改了什么）：

| 版本 | 条目 | 一句话 |
| --- | --- | --- |
| v0.1.2 | 53、54 | 控制台日志页漏 import（`renderLogs`）+ 二次全扫 7 处遗留 |
| v0.1.2 之后 | 55–58 | 密钥删除/启停的存在性语义与 404 自愈；缓存命中率改今日口径、499 客户端断开、docker 注入 VERSION；README 居中 + 项目 Logo + 控制台 favicon |
| v0.1.3 | 59 | 客户端 0.3.36 同步：车道 4 → 5（唯一协议面变更）+ 协议面全量复扫 |
| v0.1.4 | 60 | 线上数据驱动的 P0/P1：首字节预算、签到节奏、扩展指标、形状预置、错误策略表 + 前端 P1-P3 |
| v0.1.5 | 61 | 巡检四连：探测失败退避、版本号 `vv`、代理/指纹弹窗结构化、缓存命中率下钻脚本 |
| v0.1.5 | 62 | 上游服务端变更同步：`/models` 强制 Api-Key（目录/车道/预热三处改借账号凭据）+ 模型目录 21 → 27 |
| v0.1.5 | 63 | 文档整理与仓库清理（验收基线重算、索引状态列、dist 与陈旧状态文件）+ 发版 |
| v0.1.6 | 64 | 客户端 0.3.37 同步：车道 5 → 6（唯一协议面变更）+ 探测序列改为从编译期车道开始 |
| v0.1.6 | 65 | 结构治理：抽出 `internal/httpx` / `internal/server/web`、重构 `api.js`、0.3.37 复核修补（响应顺序 / 流式出错）+ race 与覆盖率基线 |
| v0.1.7 | 66 | 客户端 0.3.41 同步：车道 6 → 7（唯一协议面变更）+ `sessions/save_records` 登记 |
| v0.1.8 | 67 | 上游放宽车道门槛（≤7 全通）：车道漂移观测、Anthropic 出错即终止、缓存用量三处补齐、会话粘性落到账号 |
| v0.1.9 | 68 | issue #5 上下文/失忆：`/responses` 续接字段不再静默忽略（显式 400）+ 历史规模可观测 + 多轮/工具历史回归用例 |
| v0.1.10 | 69 | 客户端 0.3.42 同步复核：wire protocol **零变更**（车道仍 7），只跟心跳 `v=` + issue #5 白盒再确认 |
| v0.1.11 | 70 | 模型目录复核（27 个零差异）+ 修内置兜底条目的过期 `contextWindow`（1,048,576 → 320,000） |
| v0.1.12 | 71–76 | `/responses` 历史暂存（伺服端配对 / 预算 / 去重）、结构治理 PR-2~PR-4（`guard` / `observe` / `poolsvc`）、`/admin/api/state` 三层退役、客户端二次复核与 README 过期提示清理 |
| 未发版 | 77 | 客户端 0.3.43 同步复核：wire protocol **仍零变更**（车道仍 7），只跟心跳 `v=` + 上游新增 ToS 声明注释登记 |
| 未发版 | 78 | 客户端 0.3.44 同步复核：wire protocol **仍零变更**（车道仍 7），只跟心跳 `v=` + 本机升级与旧装包清理 |
| v0.1.13 | 78、79 | 客户端 0.3.44 同步 + 线上体检整改：签到预热、出口代理冷却、healthcheck / 机密出库 / deploy.sh |
| 未发版 | 81、82 | 客户端 0.3.45 同步复核：DeepAI Connect 功能轮，wire protocol **仍零变更**（车道仍 7）+ 本机升级清理；浏览器登录跟进 PKCE（`DEEPAI2API_LOGIN_PKCE`，默认开） |
| v0.1.23 | 84 | 客户端 0.3.47 同步复核：登录指引 / 恢复提示 / 启动与存储文案轮，wire protocol **仍零变更**（车道仍 7、请求体构造文件 sha256 一致）+ 本机升级与旧装包清理 |
| v0.1.24 | 85 | 客户端 0.3.48 同步复核：启动配对令牌修复 / 目录权限文案轮，wire protocol **仍零变更**（车道仍 7、`binCli` 只有 1 行语义改动）+ 本机升级与旧包清理 |
| v0.1.25 | 86 | 后台签到预热：补 `err` 日志、预算 10s → 20s、连续失败 2m → 20m 退避（治「每 30s 超时一次」的死循环） |
| v0.1.26 | 87 | 客户端 0.3.50 同步复核：agent 运行时 pi → dsh，wire protocol **仍零变更**（车道仍 7、请求头 / 指纹 / 请求体不变）+ 模型目录 27 → 28（新增免费 `claude-haiku-5-5`）+ 安装形态改 machine-wide + 指纹实测 |
| v0.1.27（未发新版） | 88、89 | 0.3.52 同步：车道 7 → 8 + 自证头 `X-DeepAI-Code-Proof`（只登记不发）；随后线上体检 + 指纹第二轮（4 模型）+ 「指纹不符 = 逆向不全」证伪 |
| v0.1.28 | 90 | 线上长尾整改：非流式预算 90 → 180 s（可调）、上游响应超时可调、issue #7 处置开关（warn/reject）、付费模型硬额度约束 |
| v0.1.29 | 90 | 硬额度约束的补丁：负余额按 `refresh_at` 判定（快照陈旧不再放行） |
| v0.1.29（未发新版） | 91 | 线上入口迁移：`deepai2api.liziai.cloud`（410）→ `deepai-gw.liziai.cloud`；文档可执行示例换新域名，无代码变更 |
| v0.1.29（未发新版） | 92 | 线上 400 归因（两个调用族均撞上游上下文窗口）+ 请求日志补 UA/IP 排障字段 + `liziwww235` 429 降权 |
| v0.1.29（未发新版） | 93 | 缓存命中率口径拆成「读 / 写 / 全新」三段（`cache_creation_input_tokens`）+ 5xx 改为同号重试保前缀缓存 + 客户端前缀契约进 README |
| v0.1.29（未发新版） | 94 | 请求日志补 `affinity` / `prefix` 指纹 + 控制台「会话」列 + `scripts/prefix-churn.mjs`：把「客户端改前缀」与「换号 / TTL 过期」分开量化 |
| v0.1.29（未发新版） | 95 | 上线 `v0.1.29-1693b20`（二进制 bind mount 覆盖镜像入口）；记录上线前后的 429 风暴与下游断流，时间线证明与本次部署无关 |
| v0.1.29（未发新版） | 96 | 全量 28 模型指纹一轮（fpd，`-e low` + 流式）：6 个 flash 模型同指 `gemini-4-carbon`；付费组 185 Theos |
| v0.1.29（未发新版） | 97 | 协议面复核：唯一缺头 `X-DeepAI-Code-Proof`（上游不校验）；固定挑战重测证明指纹漂移是采样噪声（同输入两次冠军不同，top1−top2 = 0.02） |
| v0.1.30 | 98 | 补齐第 6 个协议头 `X-DeepAI-Code-Proof`：`DEEPAI_PROOF=off\|random\|helper` 三档（helper 走随包 `resource-monitor --attest`），`/healthz` 回显 `proof` |
| v0.1.1 | 51（发版）、52 | 全扫落地（竞态 / 日志完整性 / 落盘可观测 / 8 项优化）+ 项目级 pi-lens 配置 |
| v0.1.0 | 49、50（发版）；45–48 是发版前的控制台修复 | P0/P1/P2 一轮落地 + 概览 KPI / 账号脱敏 / 版本检查；控制台 4 处裸 `refresh()`、行内菜单、登录页、侧边栏 |

本文按时间顺序追加，最新条目在最末。查最新结论从这里跳：

- 42 0.3.35 协议复查（针对号池）：车道门槛分端点、`/review_tool`、签到限流
- 43 体检三连：gateway 单测、前端拆分与死代码、工具链升级 + 连接预热
- 44 补测两个模型指纹：qwen-3.8-max-prime / gpt-6-astra（替身分支 + 请求体形状）
- 45 号池健康度显示修复：status=error 不再永久污染徽章
- 46 修「操作」菜单无反应 + 管理口令改成独立登录页
- 47 侧边栏状态文字溢出修复 + 侧边栏收起/展开
- 48 控制台 4 处裸 `refresh()`：注入式 reload + 静态契约 + 冒烟异步断言
- 49 后端 P0/P1/P2 + 前端 P0/P1/P2 一轮落地：选号热路径、流式 idle 超时、优雅关闭、按密钥限流、口令防爆破、request id/JSONL/metrics、gzip、CSP/ETag
- 50 概览 KPI 增「今日 token / 缓存命中率」、账号列表脱敏开关、版本号与更新检查、v0.1.0 发布
- 51 全量扫描落地：Keys() 竞态、请求日志完整性、回退路径空闲超时、落盘失败上报、8 项优化重构、遗留清理（v0.1.1）
- 52 项目的 pi-lens 配置：扫描范围、关闭未安装 runner/LSP、规则策略与两处真实修复
- 53 日志页报「日志加载失败：renderLogs is not defined」：补回漏掉的 import + 导入契约测试
- 54 二次全扫：英文空态漏翻、设置 0 被吞、概览连带失败、更新检查缓存污染、签到计数虚高
- 55 密钥删除/启停的存在性语义：404 不再伪装成功 + 控制台 404 自愈（v0.1.2 之后）
- 56 缓存命中率口径（今日 vs 累计）、客户端断开记 499 不计失败、docker 版本注入（v0.1.2 之后）
- 57 项目 Logo（复用 DeepAI Code 桌面端图标）+ README 顶部居中（v0.1.2 之后）
- 58 控制台用上同一图标：/favicon.ico + apple-touch-icon（v0.1.2 之后）
- 59 客户端 0.3.36 同步：车道 4 → 5、`/models` 带 Key、UA 分流复测失效（v0.1.3）
- 60 线上运行数据落地：非流式首字节预算、签到节奏修复、扩展指标、形状预置、错误策略表；前端对话框/实时日志/运行健康卡/i18n 拆分（v0.1.4）
- 61 巡检四连：探测失败退避（323 次/小时 → 2m/10m/1h）、版本号 `vv`、代理弹窗结构化、缓存命中率下钻脚本（v0.1.5）
- 62 上游服务端变更：`/models` 强制 `Api-Key`、模型目录 21 → 27（客户端仍 0.3.36，无需同步）（v0.1.5）
- 63 文档整理（验收基线重算、索引加状态列）+ 仓库清理（dist 旧产物、根目录陈旧状态文件）+ v0.1.5 发版
- 64 客户端 0.3.37 同步：车道 5 → 6（唯一协议面变更）+ 车道探测顺序修复（v0.1.6）
- 65 结构治理与 0.3.37 复核收尾：拆 `httpx` / `web` 包（14.6k → 7.0k 行）、`api.js` 重构、`response.output` 按 `output_index` 排序、流式出错不 flush 工具（v0.1.6）
- 66 客户端 0.3.41 同步：车道 6 → 7（唯一协议面变更）+ 桌面端升级（v0.1.7）
- 67 上游放宽车道门槛（≤7 全通）+ 车道漂移观测、Anthropic 出错即终止、缓存用量补齐、会话粘性（v0.1.8）
- 68 issue #5 上下文/失忆：`/responses` 续接字段显式 400、历史规模可观测、多轮与工具历史回归（v0.1.9）
- 69 客户端 0.3.42 同步复核：wire protocol 零变更（车道仍 7），只跟心跳 `v=`（v0.1.10）
- 70 模型目录复核（27 个零差异）+ 内置兜底条目 `contextWindow` 1,048,576 → 320,000（v0.1.11）
- 71 `/v1/responses` 历史暂存复核：伺服端配对、预算、去重与容量
- 72 结构治理 PR-2：抽出 `internal/guard`（限流 / 口令退避 / CORS）
- 73 结构治理 PR-3：抽出 `internal/observe`（指标 / 中间件）
- 74 结构治理 PR-4：抽出 `internal/poolsvc`（号池运行时，`Server` 字段 36 → 23）
- 75 `/admin/api/state` 三层退役：弃用指标 → `410 Gone` → 彻底删除（v0.1.12，**破坏性**）
- 76 客户端二次复核（`latest.yml` 仍 0.3.42）+ README 移除过期 ℹ️ 提示、更新文档索引（v0.1.12）
- 77 客户端 0.3.43 同步复核：协议面零变更、只跟心跳 `v=`、ToS 声明注释登记
- 78 客户端 0.3.44 同步复核：协议面仍零变更（9 个 region 全在协议面外）、本机升级到 0.3.44.0 并清理 2.35 GB 旧包
- 79 线上体检整改：签到预热（940 次阻塞 → 请求路径外消化）、出口代理冷却与换出口优先、healthcheck / mem_limit / 机密出库 / deploy.sh（v0.1.13）
- 41 客户端 0.3.35 复核：协议仍未变，只跟版本号 + 模型目录漂移
- 33 `internal/deepai` 覆盖率 56% → 92.8%（补测 + 一个 email 未 trim 的修复）
- 32 控制台：登录链接可复制 + 主题按钮换 SVG 图标
- 31 浏览器登录支持「粘贴回调 URL」，Docker 部署不再卡在回调页
- 30 客户端 0.3.30 / 0.3.32 复核：协议未变，真/替身判据重写
- 29 移除私域邮箱注册模块（前端 + 后端 + CLI）
- 28 实测复核：策略已放开 + Google 登录回调边界 + oauth 入池补全
- 27 渠道模型指纹实测：12/12 真分支，判据要改
- 26 0.3.20 客户端源码复核：请求头保真度五连修
- 25 控制台：K/M 单位 + 补齐后端已就绪的模块
- 24 对齐 0.3.20 + 让「模型指纹」测得准
- 23 文档收敛：批次日志 → 经验沉淀
- 22 号池代理池：存量账号补绑
- 21 HAR 批量导入与验活
- 20 密钥改绑分组 + SSE 收尾修正
- 19 0.3.19 协议对齐：车道 4 / 签到 / SSO 门槛
- 18 第十一批：stress-50n 全链路 + R3 上线前后的数据面对比
- 17 R3：模块拆分与启动摘要（P2）
- 16 R3：额度自动复探与 checkin 分批（P1-1）
- 15 R3：状态文件真正加密 + 写放大回归（依据扫描报告 §6）
- 14 启动不再被模型目录拉取阻塞
- 13 控制台：抽屉菜单 / 表头排序 / 中英双语
- 12 Save 并发竞态（随控制台改版一并入库）
- 11 控制台改版（视觉对齐 minimax2api）
- 10 R2：状态文件拆分与控制台分页（依据 `optimization-plan-r2.md`）
- 9 架构重构（P0/P1/P2）
- 8 50 账号全链路压测结论
- 7 计划之外：验证阶段发现并修复的三个问题
- 6 工程卫生
- 5 推理侧指纹与注册侧一致
- 4 账号级机器指纹（Theos 恒为 0 的根因）
- 3 状态文件可加密 + 台账脱敏
- 2 测试从 0 到 3800 行
- 1 未签发密钥时不再对非回环放行

---

## 1. 未签发密钥时不再对非回环放行

**根因**：`withAuth` 在 `HasKeys()` 为假时无条件放行。默认监听 `127.0.0.1` 时
只是自举便利，但 `Dockerfile` / `docker-compose.yml` 都是 `0.0.0.0` —— 从首次启动
到创建第一个密钥之间，网关是一个无需鉴权的开放中继。

决策：

- 本机回环继续放行（保留自举与本地调试）；
- 非回环来源返回 `503 setup_required`，带上可执行的提示；
- 非回环监听 + 无管理密码 → 拒绝启动（`-allow-insecure` 可显式绕过）；
- 非回环监听 + 尚无密钥 → 自动签发 `bootstrap` 密钥并只打印一次。

**验证**：`auth_test.go` 覆盖回环放行、非回环 503、错/停用密钥 401、管理口令、
同源写操作、CORS 默认收敛。

---

## 2. 测试从 0 到 3800 行

**根因**：`find . -name '*_test.go'` 为空。最容易静默回归的三块是 SSE 解析、
双协议转换、账号调度。

**决策**：纯函数优先，全部 `httptest` + `-race`，不引入第三方断言库。
覆盖 `deepai`（SSE / HTTPError 语义 / 请求头 / 不跟随 302）、`server`
（Anthropic 与 Responses 双向映射、流式事件序列、鉴权边界）、`store`
（轮询公平性、分组隔离、密钥生命周期、原子写盘、并发）、`register`
（台账合并）、`registry`（别名与兜底）。

**立即收益**：`Merge` 之前是浅拷贝入参，同级合并会就地写 `old.Stages`，
让台账历史记录被后续 `Mark` 静默污染 —— 测试第一天就抓到了，改用 `cloneRecord`。

**验证**：故意把流式 `index` 递增改错，`TestAnthropicStreamEventSequence` 变红；
覆盖率 registry 98% / store 89% / server 56%。

---

## 3. 状态文件可加密 + 台账脱敏

**根因**：明文凭据的风险不在 git（早已 ignore），而在磁盘与备份。
`store.go` 与 `deliver.go` 都写了 `0600`，但 **Windows 上 `os.WriteFile` /
`os.OpenFile` 的 mode 位不设 ACL** —— 实测落盘权限是 `666`，同机任意用户可读。

决策：

- 整文件 AES-256-GCM 封装：`{"v":2,"alg":"aes-256-gcm","nonce":"…","ct":"…"}`；
  主密钥来自 `DEEPAI2API_MASTER_KEY`（32 字节，hex 或 base64）；
- v1 裸 JSON 继续可读，配置主密钥后下一次 `Save` 自动迁移；
- 已加密的文件在缺密钥 / 错密钥时**拒绝启动**，不静默重置成空状态（那会丢光号池）；
- 默认路径从「可执行文件同目录」改为 `os.UserConfigDir()`；旧位置已有文件则继续沿用；
- 台账与 `success.jsonl` 落盘前过 `redactRecord`，密码 / Session / CSRF 单独写
  `<ledger>-credentials.jsonl`，`Load` 时按邮箱并回，`-resume` 能力不变；
- 新增 `Ledger.Compact()`：append-only 台账按邮箱压缩。

**验证**：`state_seal_test.go` 覆盖 hex/base64 解析、往返、错密钥、篡改密文、
明文迁移、端到端密文不含明文；实测启动日志会明确提示是否加密。

---

## 4. 账号级机器指纹（Theos 恒为 0 的根因）

**根因**：`NewClientWithFingerprint` 把 `Machine` 固定成 `sync.Once` 单例，
而上游按 `X-DeepAI-Code-Machine` **每天发一次**免费额度。21 个账号共用指纹，
只有第一个能领到，其余全是 `balance=0 granted=0`。

决策：

- `Account.MachineID`（原始 UUID）+ `MachineFingerprint()`（sha256），注册时一号一个；
- `commonHeaders` 优先取账号级指纹，空值才回退进程级；
- `NextAccountGroup` 两轮调度：先挑「额度未知或 > 0」，全组没额度才兜底；
- `action=assign-machine` 为存量账号补齐（只补空值，覆盖会让额度归属失效）；
- `checkin` 从串行改 4 并发（20 账号 × 30s 超时最坏 10 分钟）。

**实测**（详见 README §6.10）：补齐前 0/21 有余额；补齐后 20/21 为
`balance=200.00000 granted=200.00000`，连续 8 次推理 8/8 成功且落在 8 个不同账号。

---

## 5. 推理侧指纹与注册侧一致

**根因**：`clientFor` 走 `NewClientWithProxy`，直接把 User-Agent 丢了。
「用 Chrome 156 注册、用空 UA 推理」本身就是一个可被识别的异常信号。

**决策**：`Account` 增 `UserAgent` / `AcceptLanguage` / `SecCHUA` / `Platform`，
注册时从指纹库快照写入，`commonHeaders` 优先取账号级、缺省回退客户端默认值。

---

## 6. 工程卫生

- `Makefile` + `scripts/lint.sh`（与 CI 同款检查）；
- `LICENSE`（MIT）；
- `docs/recon.md`：把 HAR 的结论落成可检索文本，HAR 本身不入库；
- README 新增「已知坑（改动前先看）」；
- `Save()` 内容无变化时跳过写盘（后台每 30s 一次全量写太浪费），
  但配置主密钥后仍补写一次以完成明文→密文迁移。

---

## 7. 计划之外：验证阶段发现并修复的三个问题

这三条都不在最初的优化计划里，是在「补完机器指纹后跑真实流量」时暴露的。

### 7.1 非流式请求必须显式下发 `stream=false`

**现象**：补齐指纹后跑推理，非流式请求全部 `502 上游响应不是合法 JSON`。

**定位**：给解析失败加上诊断日志后拿到真相 —— 上游回了
`200 text/event-stream`，body 以 `data: {...chat.completion.chunk}` 开头。

**根因**：**上游在缺省 `stream` 字段时默认按 SSE 返回**，而 OpenAI 规范里缺省
等价于 `false`。网关原样转发客户端 payload，于是「客户端不写 `stream`」
（很常见）就必然失败。这不是新 bug，一直存在，只是之前的压测刚好都显式传了。

**决策**：`/v1/chat/completions` 与 `/v1/responses` 落 payload 前显式写
`payload["stream"] = stream`；解析失败时把 account / status / content-type /
长度 / 前 160 字节写进日志。

### 7.2 入池时的 0 额度要延迟复探

**现象**：50 账号压测后，47 个新账号入池后**全部显示 `balance=0`**。

**定位**：逐号复探发现 3~5 分钟后同一个 Key 就是 200 —— 额度不是没有，是**还没下发**。
原实现把这个 0 当事实缓存，调度侧 `hasQuota` 会长期跳过这些账号。

决策：

- `enrollRegistration` 发现 0 额度时安排延迟复探（3 / 6 / 12 分钟退避，拿到即停）；
- `store.hasQuota` 把**超过 15 分钟**的 0 额度视为「未知」，重新允许调度；
- 新增 `Store.AccountCopy`：后台任务读账号要用快照，`Account()` 返回活指针是裸竞争
  （`-race` 抓到了测试里的这个问题）。

### 7.3 Windows 上 `go test -race` 无法链接

**根因**：scoop 的 mingw **runtime** 包只提供 `ld.bfd.exe`，没有 `ld.exe`；
race detector 必须走 cgo 链接，`collect2` 按名字找 `ld`，报 `cannot find 'ld'`。

**决策**：`scripts/race-shim.sh` 在 gcc 同目录补一个同名副本（幂等、带 `--check`、
非 Windows 跳过），`make test-race` 自动依赖它。不要求使用者改 PATH。

---

## 8. 50 账号全链路压测结论

| 阶段 | 结果 |
| --- | --- |
| 注册 / 取 Key / 邮箱验证 | 50 / 50 / 50 |
| Key 验活 | 48 / 50 |
| 入池 | 47 / 50（`-resume` 后 49 / 50） |
| 耗时 | 4m40s（并发 5、10 代理） |
| 失败原因 | 全部是代理：`No exit node` ×2、`unexpected EOF` ×1 |

数据面 60 请求 / 并发 6：**100% 成功**，4.54 rps，p50 1114ms，覆盖 47/47 账号。

**结论**：链路本身稳定，瓶颈在代理池（10 条出口、且会偶发 `No exit node`），
不在注册或入池逻辑。下一批前应先 `-check-proxies` 掉失效出口。

---

## 9. 架构重构（P0/P1/P2）

第二轮扫描聚焦「架构 / 冗余 / 解耦」。基线本身健康（无循环依赖、无分层违规），
下列改动全部是等价的**搬家与收敛**，行为由既有测试 + 新增回归用例保护。

### 9.1 P0：状态不变量与死代码

| 改动 | 根因 | 验证 |
| --- | --- | --- |
| `Store.Account/Accounts` 改交付深拷贝，新增 `UpdateAccount/UpdateAccounts` | 旧实现返回号池活指针，6 处管理接口在锁外改 `Enabled/Status/MachineID`，与调度并发裸竞争；`AccountCopy` 这个「安全版」只有测试在用 | `-race`；`store_test.go` 改用 mutator API |
| `NextAccountGroup` 同样返回快照 | 调度结果被调用方跨 goroutine 读取 | `-race` |
| 删除 `anthropic.go` 的 `var _ = context/io` 死补丁 | 两个 import 实际都没用，靠哑变量骗过编译器 | `go vet` |
| 新增 `safeKeyList` / `s.catalog()` | 同一段脱敏 map 字面量与模型目录兜底各重复 2–4 份 | 现有测试 |
| `web_search/find_service/theos/usage` 补 `record()` | 这四个端点此前不进 `/admin/api/logs`，观测口径不一致 | 手工核对 |

### 9.2 P1：拆分与流式管道

- **`server.go` 958 → 176 行**，按职责拆为 `cors.go` / `auth.go` / `admin.go` /
  `admin_accounts.go` / `probe.go` / `httputil.go`。纯搬迁，零逻辑改动。
- **SSE 读帧四合一**（新 `stream.go`）：`chat`、`responses` 透传、`responses→chat`、
  `anthropic` 原本各写一遍 `data:`/`[DONE]`/`Unmarshal` 循环（~184 行）。
  现在统一为 `forEachSSEFrame`（保留 event:/注释帧，供透传）与
  `forEachChatChunk`（只交付 JSON）。
- **`chatDelta` 抽取**：`responses` 与 `anthropic` 的
  `choices[0]→delta→reasoning/content/tool_calls` 逐字相同（~70 行）→ `chatChunkDelta`。
- **顺手修掉一个真 bug**：`passthroughResponses` 把 `rewriteResponsesModel(obj)`
  作用在解析后的对象上，写回客户端时却用原始 `data` 字符串，**模型名改写从未生效**。
  现改为写出重编码后的对象；`TestResponsesPassthroughRewritesModel` 锁定。
- **`record()` 11 个位置参数 → `reqLog`**：25 处调用点原本全靠记忆参数顺序。
  现在构造时固化 `key/acct/model/endpoint/stream/start/inLen`，
  调用点只写 `rl.done(outLen, usage, err)`。

> `readSSELines` 刻意没有复用 `deepai.ReadSSE`：后者按规范合并多行 data，
> 会丢掉注释帧与 `event:` 行的原始形态，而透传路径需要它们。

### 9.3 P2：新包与策略外迁

| 新包 | 取代 | 理由 |
| --- | --- | --- |
| `internal/retry` | `theosWithRetry`、`probeAccountQuota` 两套退避循环 | 「重试几次/等多久/什么错不重试」单点定义 |
| `internal/parallel` | 6 处手写 jobs channel + WaitGroup（批量注册、台账补跑、配额探测、控制台巡检、代理检查、邮箱重验证） | 并发上限、ctx 取消、按下标回填统一 |
| `internal/pool` | `store` 里的 `pickLocked/hasQuota/QuotaStaleAfter` | 选号是业务策略，不该住在持久化层；抽出后无需构造 `Store` 即可测调度 |

顺带修掉两个缺陷：

1. **`parallel.For` 的 select 竞态**：`<-ctx.Done()` 与 `jobs <- i` 同时就绪时
   Go 等概率二选一，导致「已取消的 ctx」仍会跑掉部分任务。加前置短路，
   `TestForSkipsEverythingWhenCanceled` 锁定。
2. **`register.Merge` 漏合并指纹字段**：升级分支只回填 `Fingerprint`，
   丢掉 `MachineID/UserAgent/AcceptLanguage/SecCHUA/Platform`。
   `MachineID` 决定上游按机器发额度，丢失后额度归属会静默断开。
   补齐并加两条回归测试。

其它：

- `Account.MergeFrom` 收编 `UpsertAccount` 里 50 行逐字段拷贝（**A3 以此落地**）。
  没有按原计划把 `Account` 拆成 `Credential` + `AccountState`：
  该结构体直接序列化进状态文件与台账，结构性拆分要动数据兼容，
  而真正痛点是那 50 行拷贝 —— 用方法内聚即可拿到同样收益，风险低一个量级。
- `register.loadEnvFiles` 加 `sync.Once`：`os.Setenv` 是进程级副作用，
  重复加载会让直读 `os.Getenv` 的地方（`deepai.EnvOr`、`main`）看到
  取决于调用顺序的结果。

### 9.4 验收

```text
gofmt -l .        → 空
go build ./...    → ok
go vet ./...      → ok
go test ./... -race → 11 个包全绿（新增 retry 6 / parallel 4 / pool 7 条用例）
```

新增包与测试：`internal/retry`(6)、`internal/parallel`(4)、`internal/pool`(7)；
`server` 新增 `TestResponsesPassthroughRewritesModel`；
`register` 新增 `TestMergeUpgradeKeepsFingerprintFields`、
`TestMergeSameLevelTakesNewFingerprint`。

---

## 10. R2：状态文件拆分与控制台分页（依据 `optimization-plan-r2.md`）

两批 50 号实测暴露的不是「并发不够」，而是**写放大**与**全量下发**：

| 指标 | 改造前 | 改造后 |
| --- | --- | --- |
| 主状态文件 | 1.13MB，每 ≤30s 全量重写 | 958KB，账号/密钥/设置/模型未变即跳过 |
| 统计 | 混在主文件里，导致「跳过未变化写盘」永不生效 | 独立 `-stats.json`（约 140B） |
| 日志 | 内存 500 条 + 混在主文件 | append-only `-logs.jsonl`（8MB 轮转） |
| `/admin/api/state` | 429KB @603 账号 | `/admin/api/overview` 约 2.9KB + `/admin/api/accounts` 分页 |
| 空闲刷新 | 每次全量 | `ETag` / `304` |

同时落地的行为修正：

- 入池时上游返回的 `balance=0` 不再当作「刚探过的事实」（那会让新批最长 15 分钟
  不可调度，实测 50 个号只有 12~15 个能被选中），改为「未知」放行；
  复探序列 3/6/12 分钟 → 45s/2m/6min。
- 单账号并发上限 2；非流式请求 90s 整体预算。
- 批次结束自动补一轮邮箱验证（实测曾 2/50 停在 `verify:pending`）。
- `/admin/api/logs` 支持 `since / endpoint / account / min_status / q` 过滤。
- 控制台：服务端分页、GET 请求去重、管理密码改 `sessionStorage`。

**迁移**：`Open` 时识别旧格式（`logs`/`stats` 内嵌在主文件），自动拆出并重写主文件；
统计旁路文件缺失时标记脏位，保证首次/迁移后统计能落盘。

---

## 11. 控制台改版（视觉对齐 minimax2api）

参考 `zakolinxi/minimax2api` 管理台的设计语言重做了 `internal/server/web/index.html`
的外壳与样式。**业务逻辑零改动**：脚本块原样保留，只替换 `<head>`/样式与标记。

| 维度 | 改前 | 改后 |
| --- | --- | --- |
| 导航 | 内容区顶部横向按钮 | 左侧固定侧边栏（品牌 / 图标 + 标签 / 健康状态），窄屏收成抽屉 |
| 主色 | 蓝色 `--accent:#4f8cff` | 中性灰（主色=近黑/近白），仅在语义色上保留绿/黄/红 |
| 圆角 / 字号 | 7–12px，13px | 8px；按钮 12px、徽标 11px、表头 11px |
| 按钮 | 小方角 | 胶囊（999px） |
| 卡片 | 带边框 | 扁平无边框（靠底色区分层级） |
| KPI | `auto-fit` 换行，最后一张独占一行且折行 | 固定 4 列 × 2 行，时间戳单行 |
| 宽表 | 10 列溢出，右侧「状态」被截断 | 按断点收起次要列（`#acctTable` 来源/失败/最近使用；`#logTable` 字符/成本/流式） |

其它：

- 新增 `#tab-xxx` 深链接（`/#accounts` 直接打开账号页）。
- 修复 `.only-mobile` 被 `button.icon-btn` 的选择器优先级覆盖，导致移动端汉堡
  按钮在桌面端也显示。
- 新增 4 条结构性回归测试（`console_test.go`）：
  脚本引用的每个 DOM id 必须存在、每个 `data-tab` 必须有 `#tab-*` 面板、
  `<section>` 数量必须与导航数一致（脚本用 `querySelectorAll("section")` 切页，
  多出的 section 会被误当成标签页）、外壳关键钩子（侧边栏/抽屉/收列/深链接）。

---

## 12. Save 并发竞态（随控制台改版一并入库）

`Store.Save()` 此前没有任何串行化，高并发下有三个独立问题，已用 `-race` 复现并修复：

| 问题 | 表现 |
| --- | --- |
| `lastSaved` 底层数组读写竞争 | `bytes.Equal(plain, last)` 读 与 `append(s.lastSaved[:0], …)` 写并发（`store.go:317` ↔ `:322`） |
| 共享临时文件 | 固定 `<path>.tmp`：Windows 报「文件被另一个进程占用」并失败；Linux 上互相截断，`rename` 后可能留下坏 JSON |
| 顺序错乱 | 先到者的 `rename` 覆盖晚到者 → 磁盘比内存旧；若 `lastSaved` 抢先更新，后续 Save 判定「无变化」而跳过，增量永久丢失 |

触发路径不是理论：`scheduleQuotaReprobe` 每账号一个 goroutine 各自调 `Save`，
批量入池 50 个号后会在 45s / 2m / 6m 三个时间点同时醒来；叠加后台 30s ticker
与每个管理操作。

修复：

- 新增 `Store.saveMu`，`Save()` 全程持锁（同时消除竞态、tmp 冲突、乱序）；
- `writeAtomicSealed` 改用 `os.CreateTemp(dir, "."+base+".tmp-*")` 唯一临时名；
- 写入后 `f.Sync()` 再 `rename`（此前掉电可能留下「名字已换、内容为空」）。

回归：`save_race_test.go` 断言 8×15 轮并发写入后文件仍是合法 JSON 且能重新载入；
另有一条「同一路径两个 Store 实例并发写」的用例。

---

## 13. 控制台：抽屉菜单 / 表头排序 / 中英双语

三项交互能力，均在前端完成（排序中的账号列表例外：它必须服务端做）。

### 13.1 抽屉式操作菜单

账号行原本平铺 3~4 个按钮（配额 / 停用 / 清错 / 删除），行高被撑大且视觉噪音大。
改成行尾一个 `⋯` 触发**浮层菜单**：

- 菜单挂在 `document.body` 上而不是行内 —— `.table-wrap` 有 `overflow-x:auto`，
  行内绝对定位会被裁掉；
- 定位按触发按钮的 `getBoundingClientRect()` 计算，下方空间不足时向上翻；
- 点击空白处、Esc、滚动、缩放都会关闭；
- 危险项（删除）用红色并前置分隔线。

### 13.2 表头排序

| 表 | 排序位置 | 说明 |
| --- | --- | --- |
| 账号池 | **服务端** | 列表是服务端分页的，只排当前页会得到错误结果（第 2 页首条可能小于第 1 页末条） |
| 模型目录 / 密钥 / 日志 | 前端 | 数据量小，一次全量在手里 |

服务端接口扩展：`GET /admin/api/accounts?sort=theos&order=desc`。
`sort` 支持 `account / group / source / plan / theos / status / failures / last_used`；
未知字段忽略（不报错，保持原顺序）。相等值统一按 `ID` 兜底，
保证**翻页时条目不会在页之间跳动**。

`theos` 排序把无法解析的余额当 `-1`（未知排最前，升序），并按 3 位小数定标
避免浮点抖动。

### 13.3 中英双语

没有给每个节点加 `data-i18n`，而是用**一套字典 + 渲染后文本替换**：

- 字典以中文原文为键（`I18N_EN["概览"] = "Overview"`），静态标记与动态渲染共用；
- `translateDom(root)` 遍历文本节点做精确匹配，首次遇到时把中文原文记在
  `node.__zh` 上，切回中文时用它还原；
- 跳过 `SCRIPT / STYLE / TEXTAREA`，避免翻译脚本自身；
- 带插值的句子（分页信息、健康状态、E2E 统计）走 `t("共 {n} 个账号", {n})`；
- `placeholder` 与 `title` 由 `translateAttrs` 单独处理；
- 语言优先级：`?lang=en` > `localStorage` > 浏览器语言；切换按钮在顶栏右侧。

### 13.4 验证方式

新增 `scripts/console-e2e.mjs`：用 CDP 驱动无头 Chrome 做**真实交互**断言，
而不是只看截图 ——

```text
node scripts/console-e2e.mjs http://127.0.0.1:10087 bin/shots
```

它检查：账号表是否渲染出行、点 `⋯` 是否弹出 3 个菜单项、点表头是否发出带
`sort=theos&order=asc` 的请求、表头箭头是否更新、`?lang=en` 下导航与表头是否变英文，
并捕获所有 `Runtime.exceptionThrown` 与 `console.error`。

过程中这套检查抓到两个真实 bug：一次是把 `\n` 写成了真实换行导致整段脚本语法错误
（页面全空白），一次是 `\b` 在传递中变成退格符使 `?lang=` 正则永不匹配 ——
两者在静态检查里都看不出来。

另外 `console_test.go` 增加 `TestConsoleFeatures`，守住菜单/排序/双语的钩子存在性。

---

## 14. 启动不再被模型目录拉取阻塞

**根因**：`main()` 在起监听**之前**同步调用 `srv.RefreshModels()`。
`RefreshModels` 内部是 20s 超时，上游不通时这 20 秒整段压在启动路径上。

线上日志可以直接看到（2026-11-06 23:02 那次重启）：

```text
23:02:04 机器指纹: 8751f711d837…
23:02:24 拉取模型目录失败（沿用缓存）: context deadline exceeded
23:02:24 DeepAICode2API dev 已启动: http://127.0.0.1:10087/
```

**决策**：先监听，目录交给后台首个拉取。

- 目录为空时 `/v1/models` 与模型解析都会回落到内置兜底条目
  （`server.catalog()` / `registry.Resolve`），所以空目录期间服务是可用的；
- 首个拉取放在 `background()` 开头（该 goroutine 在起监听前启动），
  用独立 `context.WithTimeout(30s)`，失败只影响模型清单，不影响退出流程；
- 6 小时周期的刷新逻辑不变。

**实测**（`scripts/measure-startup.py`，上游指向不可达的 TEST-NET 地址）：

| | 启动 → `/health` 可服务 |
| --- | ---: |
| 改动前（同步拉取） | **6.20s** |
| 改动后（先监听） | **0.96s** |

> 本机 connect 约 6s 就失败；线上那次上游是「挂住」而非拒绝，所以吃满了
> 20s 的 ctx 超时——两种情况下改动收益方向一致。

正常网络下的日志顺序也验证了行为：

```text
23:49:49 机器指纹: 8751f711d837…
23:49:49 DeepAICode2API dev 已启动: http://127.0.0.1:10087/
23:49:52 模型目录已更新：14 个模型，默认 glm-5.3-flash
```

---

## 15. R3：状态文件真正加密 + 写放大回归（依据扫描报告 §6）

### 15.1 根因：`.env` 只有注册 CLI 加载

`main()` 只读 `os.Getenv`，而 `.env` 的加载逻辑埋在
`internal/register/config.go` 并由 `LoadConfig()` 惰性触发；首次调用发生在
`handleAdminRegister`，远晚于 `store.Open()`。于是写进 `.env` 的
`DEEPAI2API_MASTER_KEY` 对网关主进程**永远无效**，状态文件只能保持明文。

- 抽出 `internal/envfile`（`Load` / `LoadFiles`），网关与注册 CLI 共用；
- `main()` 第一行调用 `envfile.Load()`，早于 flag 默认值求值（`envOr`）与 `store.Open`；
- 启动日志打印主密钥指纹（sha256 前 8 位），磁盘仍是明文时明确提示下一步。

### 15.2 `-seal`：封存存量明文（含历史日志）

启用加密只影响"之后写入"的数据：主文件会在下一次 `Save` 自动重写为 v2，
但请求日志是 append-only 的，历史明文行不会消失（`decodeLine` 兼容混合格式，
读取时也不报错）。新增 `Store.Reseal()` 与 `-seal [-yes]`：重写主文件/统计、
逐行重新封存 `-logs.jsonl` 与 `.1`、每个文件校验"不再含明文"、失败即报错。

**顺带修掉一个真实缺陷**：`seal()` 用 `json.MarshalIndent` 产出多行信封，
而日志是逐行 JSONL —— 代码注释写"一行一个 seal 信封"，实际写入会把一条记录
拆成多行，`decodeLine` 按行切分永远解析不出来，表现为「开启加密后日志一重启
就消失」。现在日志走 `sealLine()`（紧凑单行信封），并新增
`TestEncryptedLogsSurviveReopen` 守住这个行为。

### 15.3 写放大：653 号把 R2 的问题带回来了

`MarkAccountResult` 每个请求都刷新 `LastUsed`（秒级），内容必变 →
`Save` 的「未变化跳过」失效 → 后台 30s 一次 `MarshalIndent` + fsync 约 950KB
（持续流量下约 115MB/h）。号池从 21 涨到 653 后，R2 拆日志省下的写放大被抵消。

- 落盘快照把 `LastUsed` 向下取整到 5 分钟（内存仍秒级）；
- `json.MarshalIndent` 移出读锁（此前 1MB 序列化期间顶住请求路径的写锁）；
- 新增 `SaveCounters()`（写盘次数 / 明文字节数），供压测直接观测，不靠估算。

### 15.4 统计语义与脏标志竞态

扫描时看到 `stats.total_requests=0` 而日志有 500 条：`stats.started_at`
（20:02:44）晚于最后一条日志（19:37:57），是 20:02 执行过"重置统计"，
日志按设计保留 —— 不是 bug，但界面没有可解释的字段。

- `Stats` 增 `reset_at`，`ClearStats` 保留 `started_at` 并记录重置时间；
  控制台 KPI 由"启动时间"改为"统计自"（优先 `reset_at`）。
- 修掉 `Save` 的脏标志窗口：写盘后改为「快照仍等于当前统计才清脏」，
  避免快照与落盘之间插入的 `Record` 只在内存里。

### 15.5 验证

```text
go build ./...                 OK
go vet ./...                   OK
gofmt -l .                     无输出
go test ./...                  9 个包全部通过
go test -race -count=1 ./...   9 个包全部通过
```

`-seal` 冒烟（临时目录，明文状态 + 明文日志，密钥写在 `.env`）：

```text
状态文件加密: 已启用（AES-256-GCM，密钥指纹 9a2db2e2，来自 DEEPAI2API_MASTER_KEY）
已封存 3 个文件（账号 1，历史明文日志行 1，原样保留 0）
  - state.json / state-stats.json / state-logs.jsonl
```

无密钥启动已加密文件时按预期拒绝：`状态文件已加密，但未设置 DEEPAI2API_MASTER_KEY`。

---

## 16. R3：额度自动复探与 checkin 分批（P1-1）

### 16.1 问题

扫描时全池 653 个账号的 `balance/granted` 都是 `0.00000`，`checked_at` 停在
04:27–11:55 UTC，而 `refresh_at` 统一是次日 09:58 UTC —— **到点没有任何东西
去复探**。自动复探只覆盖"新入池账号"（45s/2m/6m 三次），存量账号只能人工点
「刷新全部配额」：653 号 / 4 并发 × 上游 ~4s ≈ 11 分钟，而且 handler 用的是
`r.Context()`，浏览器一关页面就中断在半途。

### 16.2 三层复探

| 层 | 触发 | 范围 | 失败处理 |
| --- | --- | --- | --- |
| 入池复探 | 入池后 45s / 2m / 6m | 新账号 | 采信成功结果（既有逻辑） |
| 按需复探 | 推理选中"快照过期"的账号 | 仅用到的号，15min 最多一次 | **不改 Status/Failures** |
| 后台守护 | 每 5min ≤24 个 | `refresh_at` 已过 / 从未探 / 超 6h | 同上 |

- 按需复探只刷新**已有快照**的账号：从未探测的号不在热点路径上新增上游请求，
  交给入池复探与后台守护；
- 后台守护在独立 goroutine：上游挂住时一批最坏几分钟，不能拖住 30s 周期保存；
- 排序优先级：`refresh_at` 已过 → 从未探测 → 单纯过旧（同级按 checked_at 升序），
  分批时先探最值得探的。

### 16.3 两个必须的限流

1. **在途去重**（`Server.probing`）：手动 checkin、入池复探、按需复探三路会撞同一批号，
   没有去重时同一个号会被连打多次。
2. **全局 4 个探测槽位**（`Server.probeSem`）：653 个号的快照同时过期时，一波流量
   可能叠出上百个并行 `/theos` —— 那是最容易被上游风控的形态。按需复探拿不到
   槽位就放弃，留给下一轮；批次探测阻塞等待，总在途数恒 ≤4。

### 16.4 checkin 分批与进度语义

`/admin/api/checkin` 新增 `group` / `stale_only` / `limit`，响应带
`total` / `probed` / `remaining`：

- `remaining` 定义为「过滤集里仍然过期、且不在本批里」的账号数，而不是
  `total - probed`：否则每一批都剩同一批在途/失败的号，控制台会无限重试；
- 探测已改为 `context.WithoutCancel` + 10min 预算：页面关掉也会把这批探完并落盘；
- 控制台「刷新全部配额」按 `remaining` 自动翻页（≤30 轮），单批 100。

### 16.5 可观测性

- `/health` 增 `quota_fresh`（15min 内探过）与 `quota_due`（守护会去重探）；
- 账号分页项增 `quota_age`（秒）与 `quota_due`，控制台不必自己复制"多久算陈旧"。

### 16.6 随 PR3 一并处理的落盘写放大

按需复探每分钟可能改几十个账号的 `Theos.CheckedAt`，秒级精度会让「内容未变则
跳过」再次失效。落盘快照现在把 `Theos.CheckedAt` 与 `LastUsed` 一起量化到 5 分钟
（内存仍秒级），重启后"最近使用 / 额度探测时间"精度降为 5 分钟。

### 16.7 验证

```text
go build ./... / go vet ./... / gofmt -l .        OK
go test ./...                                    9 个包全部通过
go test -race -count=1 ./...                     9 个包全部通过
```

新增覆盖（`internal/server/quota_test.go`）：

- `TestQuotaNeedsRefresh`：refresh_at 已过 / 未来 / 从未探测 / 15min 陈旧；
- `TestCheckinBatchesAndReportsRemaining`：limit=3 探 5 个号，第二批补齐且 `remaining=0`；
- `TestCheckinFiltersByGroupAndStaleness`：`group` 与 `stale_only` 只打该打的号；
- `TestProbeBatchDedupesInFlight`：在途时标记 `Skipped`，上游 0 次请求；
- `TestReprobeStaleOnlyProbesDueAccounts`：后台守护不碰新鲜账号；
- `TestInferenceRefreshesStaleQuota`：推理命中过期快照会异步补探并写回；
- `TestProbeAsyncFailureDoesNotMarkAccount`：自动复探失败不改账号状态。

---

## 17. R3：模块拆分与启动摘要（P2）

三个文件超过 750 行后，review 与 AI 导航的成本明显上升（改一个协议分支要在
700 行里找上下文）。本轮只做**机械搬移**：不改任何函数体、不改调用方、测试文件
一行没动，用 `goimports` 统一导入。拆分后单文件全部 ≤400 行。

| 原文件 | 行数 | 拆分后 |
| --- | ---: | --- |
| `internal/store/store.go` | 1054 | `store.go`(163) + `store_open.go`(380) + `store_accounts.go`(390) + `store_models.go`(72) + `store_keys.go`(109) + `store_telemetry.go`(126) |
| `internal/server/anthropic.go` | 718 | `anthropic.go`(263) + `anthropic_translate.go`(397) + `anthropic_stream.go`(81) |
| `internal/server/responses.go` | 780 | `responses.go`(307) + `responses_translate.go`(252) + `responses_stream.go`(244) |
| `internal/register/pipeline.go` | 759 | `pipeline.go`(368) + `pipeline_verify.go`(181) + `pipeline_batch.go`(251) |

拆分边界按职责而不是按行数切：

- `store`：打开/落盘（含加密封装）与账号、密钥、模型、统计分开；
- `server`：协议入口（handler）、字段映射（translate）、流式状态机（stream）分开，
  以后改 Anthropic 字段映射只动 400 行以内的文件；
- `register`：单账号阶段、验证回填、批量调度分家。

### 启动摘要

启动日志从多行中文提示之外，增加一行可采集的 JSON：

```text
startup {"accounts":653,"data":"…/deepaicode2api-data.json","default_model":"glm-5.3-flash",
         "encrypted":true,"event":"startup","keys":1,"listen":"127.0.0.1:10087",
         "machine_ready":true,"models":14,"ready":649,"version":"dev"}
```

数据路径、加密状态、号池规模这些排障第一眼要看的东西，现在可以直接
`grep 'startup ' | jq`，不必再写正则解析中文日志。

### 验证

```text
gofmt -l . / go vet ./... / go build ./...      OK
go test -count=1 ./...                          9 个包全部通过
go test -race -count=1 ./...                    9 个包全部通过
node --check （控制台两段内联脚本）              语法通过
```

---

## 18. 第十一批：stress-50n 全链路 + R3 上线前后的数据面对比

### 18.1 环境与预检

- 机器指纹、CF 临时邮箱 Worker 健康检查通过；
- 代理池 `-check-proxies`：**12 / 12 可用**；
- 网关 `127.0.0.1:10087`，入池前号池 653。

### 18.2 全链路（50 号）

```bash
./bin/register.exe -count 50 -concurrency 5 -group stress-50n -result-dir result/stress50n
```

| 指标 | 结果 |
| --- | ---: |
| 墙钟 | **3m08s** |
| 注册 / 取 Key / 入池 | **50 / 50 / 50** |
| 邮箱验证 | 48/50 → `-reverify` 后 **50/50** |
| 失败 | **0** |

交付物写入 `result/stress50n/`（`keys.txt` / `apikeys.txt` / `success.jsonl`，gitignore 内）。

### 18.3 额度

| 时间点 | 余额 > 0 | 已探测 |
| --- | ---: | ---: |
| 入池 +1min | 36/50 | 49/50 |
| 入池 +4min | 39/50 | 50/50 |
| R3 切换后复探 | 46/50 | 50/50 |

R3 切换后手动 `checkin` 有 16 个账号失败，错误全部是 `proxyconnect tcp: EOF`：
是账号绑定的出口失效，不是账号不可用（数据面仍 100%）。出口恢复后
「按需复探」会在下一次命中时把状态改回 `ok`。

### 18.4 数据面（120 请求 / 12 并发，`glm-5.3-flash`）

| 轮次 | 网关 | 成功率 | 墙钟 | rps | p50 | p90 | p95 | max |
| --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| 1 | R3 前 | 100% | 19.2s | 6.24 | 1451 | 3713 | 5195 | 8193 |
| 2 | R3 前 | 100% | 42.1s | 2.85 | 1251 | 2769 | 4715 | 27307 |
| 1 | R3 后 | 100% | 20.3s | 5.93 | 1581 | 3313 | 3884 | 6046 |
| 2 | R3 后 | 100% | 17.5s | 6.85 | 1421 | 2324 | 2693 | 4323 |

（延迟单位 ms；同一分组、同一脚本、同一模型。）

- 成功率不变，全部 240 请求两轮合计 0 失败；
- **p95 由 5.2s / 4.7s 收敛到 3.9s / 2.7s**，27.3s 的单次长尾消失；
- 第二轮吞吐从 2.85 回到 **6.85 rps**；
- p50 略有上浮（+130 / +170ms）：本轮 16 个账号因出口失效需要换号重试，
  且额度覆盖 46/50，与历史 50/50 热态基线不可直接对齐。

> `sample reply=''` 不是故障：压测脚本用 `max_tokens=8`，推理 token 就吃满预算；
> 实测 `max_tokens=32/96` 时 reasoning 分别占 118 / 481 token。

### 18.5 加密切换与收尾

同批次期间把网关从明文迁到 v2 密文（先停进程，再迁移）：

```text
已封存 3 个文件（账号 703，历史明文日志行 741，原样保留 0）
startup {"accounts":703,"encrypted":true,"keys":1,"models":14,"ready":696,...}
```

- 主密钥写入 `.env`（指纹 `8bd06c78`，仅指纹入库，密钥不入库）；
- 切换后删除临时明文备份与两个历史 `*.bak-*` 明文备份；
- 临时压测密钥已删除，仅保留原有 `stress-group`。

---

## 19. 0.3.19 协议对齐：车道 4 / 签到 / SSO 门槛

**根因**：桌面端自动升级到 **0.3.19**，上游同时把 Theos 车道从 2 提到 **4**。
旧网关的每个 `/models`、`/theos`、`/chat/completions` 都开始返回
`426 lane_version_unsupported`（"This version of DeepAI Code is no longer supported"），
即整个网关不可用。对照解包后的 `server.asar`（`src/deepai/api.ts`）与线上实测，
协议还多了三处变化：

1. `X-DeepAI-Code-Instance`：每进程 16 字节随机 hex，随所有 JSON 请求发出；
2. `/theos` 变成签到通道：响应（含 403 拒绝）带 `check_in:{nonce,seconds}`，
   下次请求回传 `X-DeepAI-Code-Check-In` 与 `X-DeepAI-Code-Beacon`；
3. 新增账号策略 `403 social_login_required`：邮箱密码注册的账号被挡在推理之外。

**决策**：

- `deepai.LaneVersion = "4"`；`Client.Instance` 在构造时生成一次；
- 签到状态**按 API Key 隔离**（同一个 Client 会被多个账号复用），
  半衰期（`seconds/2`）后补签，推理前 `EnsureCheckIn` 且失败不阻塞；
  `captureCheckIn` 从成功/拒绝两种 body 里都取挑战；
- 错误分层：`social_login_required` → `StatusBlocked`（调度跳过、全池 blocked 时
  503 并说明原因）；`machine_required` / `lane_version_unsupported` → 直接上报不换号；
  其余 403 仍按账号失败换号；
- 针对 `400 This request is not one DeepAI Code sends.`：先原样透传，命中后
  同号用桌面端字段白名单 + 补 system 消息重试一次（`ChatBodyFallback`）；
- 注册链路：`/get_user_login_type` 预检社交-only 账号、`validate` 把
  `social_login_required` 落成 `blocked`（不再降级成 `failed`），
  对话探针保留 `*HTTPError` 类型以便上层识别；
- 新增测试：车道/实例头、签到握手（含从 403 取 nonce）、策略错误分类、
  形状兜底、blocked 调度跳过、网关两层（形状重试 / 策略拦截）。

**验证**（2026-11-07 线上）：

```text
lane 0/2/3/5/6 /models → 426 lane_version_unsupported
lane 4       /models → 200（17 个模型，default=glm-5.3-flash）
lane 4       /theos  → 200 {lane_version:4, check_in:{nonce, seconds:600}}
lane 4       /web_search → 200
邮箱注册号    /theos、/chat/completions → 403 social_login_required
```

结论：协议层已可用，但**池内邮箱密码账号已无法推理**；后续只能导入
Google/Apple 登录的账号，或等待上游放开该策略。`go test ./...` 全绿。

> 注：偶发 `400 invalid_request_error` 只在滚动升级窗口出现过一次，
> 触发条件与「缺 system / 缺 tools」相关但未定论；兜底重试已覆盖该路径。

---

## 20. 密钥改绑分组 + SSE 收尾修正

**根因**：

1. 清空 `stress-*` 号池后，唯一密钥仍绑定在 `stress-20261106` 分组，请求会
   命中「分组内无可用账号」；而密钥分组此前没有修改入口，只能删除重建
   （会改变密钥值，客户端全部要改）。
2. SSE 三个流式入口在 `newSSEWriter`（已 flush 隐式 200）之后又调了一次
   `WriteHeader`，net/http 每次流式请求都记一条 `superfluous` 日志；
   chat 流还会在上游已发 `[DONE]` 后再补一个，客户端收到重复终止帧。

**决策**：

- `store.SetKeyGroup` + `POST /admin/api/keys {"action":"set-group","key":…,"group":…}`，
  保持密钥值不变（支持掩码 ref）；空分组归一为 `default`；
- `newSSEWriter` 内显式 `WriteHeader(200)`，删除四处调用方重复写头；
- chat 流用 `doneSent` 标记，只发一个 `[DONE]`。

**验证**（2026-11-07）：

```text
密钥 stress-group: stress-20261106 → default（ok:true）
桌面端改用 Google 账号后导入：zhuhaoyi181（/theos 200，lane 4，带 check_in）
网关非流式 glm-5.3-flash → 200 content="42"
网关流式同一请求 → content delta "42"，[DONE] 恰好 1 个，无 superfluous 日志
```

> 备注：邮箱号与 Google 账号的差别在 `/theos` 就能看出来——前者 403
> `social_login_required`，后者 200。桌面端账号 `epsoft-…` 仍保留在池里
> （`blocked`，调度跳过），确认不再需要后可删除。

---

## 21. HAR 批量导入与验活

**背景**：`accounts/` 下积累了 13 个按邮箱命名的 HAR（浏览器抓包），
需要把其中的 API Key 批量导入号池并验活。

**做法**：

- 从每个 HAR 的响应体里提取 `"api_key"`（UUID 格式），文件名即邮箱，
  一号一键、零误配；
- 通过 `POST /admin/api/accounts {"action":"manual"}` 逐个导入（`default` 分组）；
- 因手动导入不带机器指纹，随即调用 `action=assign-machine` 给 13 个号各分一个
  独立 UUID（否则共用进程指纹，只有第一个能领到每日额度）；
- 验活分两层：`/theos` 批量探测（状态/lane/额度）+ 每个 Key 直连上游做一次
  免费模型对话（确认真正能推理，避免「/theos 200 但 chat 被拒」）。

**结果**（2026-11-07）：

```text
导入 13 个（new=true 13/13），号池 1 → 14
/theos：14/14 200，全部 lane_version=4，check_in 正常
chat：13/13 200（2 个首发瞬时失败——读超时 / TLS EOF——重试即通过）
额度：新机器指纹下 balance/granted 均为 0，等 refresh_at 后下发
```

> 注意：`zhuhaoyi181@gmail.com`（HAR 里的 Key）与 `zhuhaoyi181`（桌面端
> 当前 Key）是同一账号的两把不同 Key，都能用；如需去重可删其中一条。
> `accounts/` 已加入 `.gitignore`（HAR 含凭据，不入库）。

---

## 22. 号池代理池：存量账号补绑

**背景**：HAR 导入的 13 个账号 + 桌面端账号都走直连出口。实测 `network_bound`
（每日免费 Theos 按出口 IP 限额）后，同一直连 IP 下只有 5 个账号拿到额度，
其余 8 个要等刷新。

**做法**：

- 新增 `POST /admin/api/accounts {"action":"assign-proxy"}`：
  - 从 `PROXY_POOL` / `PROXY_POOL_FILE` 解析代理池，按账号轮询分配；
  - 默认只补空值，`reassign:true` 覆盖，`clear:true` 回滚；
  - 支持 `id` / `group` 限定范围；返回 `assigned` 与 `host:port` 分布（不含凭据）；
- 复用注册链路的 `register.NewProxyPool` / `ProxyLabel` / `CheckProxyPool`，
  不新增代理实现；
- 分配后跑一次 `/admin/api/checkin` 验证：代理 → `/theos` 全链路。

**验证**（2026-11-07）：

```text
代理池：12/12 可用（go run ./cmd/register -check-proxies）
分配：14 个账号 → 12 个出口（2 个出口各 2 号）
验活：14/14 最终 200（1 个 TLS 握手中超时，重试即通过）
额度：有 200 Theos 的账号 5 → 10（含 zhuhaoyi181 两把 Key 共用钱包）
计量 chat（glm-5.3）经代理：HTTP 200 content="42"，耗时 ~10.9s（直连 ~3s）
```

> 代价：代理链路有额外延迟，流式长连接对代理稳定性更敏感；
> 出问题时可 `"clear":true` 一键回到直连。

---

## 23. 文档收敛：批次日志 → 经验沉淀

**背景**：README §6.11~§6.21 累积了 10 批（第 1~10 批，50 号/批）的逐批执行日志，
每批新增一节，导致「总览」被流水账淹没：读者要翻 330 行才能看到安全与合规，
而真正可复用的规则（关键路径怎么切、重端点为什么要限流）散落在批次叙述里。

**做法**：

- README 删除逐批日志（§6.11~§6.21 共 335 行），只保留：
  - **§6.11 工程经验与验收基线**：6 条结论 + 当前基线（入池 / E2E 中位 / 吞吐 / 额度 / 验证）；
  - **§10.1 已知坑**新补 7 行，全部来自批次事故（验证点击 0/50、入池 0 额度、
    关页面中断探测、CLI 杀后台验证、注册 POST 无预算、出口故障误判）；
- 新增 `docs/lessons.md`：把日志转成 **现象 → 根因 → 规则 → 代码落点**，
  分 4 类 20 条（关键路径 / 上游调用纪律 / 额度与调度 / 观测口径），
  附 §5 改动前检查清单（10 项）与 §6 失效边界；
- `docs/performance.md` 保留测量数据，标注为回归基线并指向 `lessons.md`，
  修正错位的「不建议做的事」（原 §9 → §13）；
- README 文档索引补齐 `lessons.md` 与 `optimization-plan-r2.md`。

**原则**：**README 只放结论与口径，过程放 `changes.md`，数据放 `performance.md`，
规则沉淀到 `lessons.md`**。批次日志属于「一次性证据」，写完应当场转成规则，
否则每跑一批文档就胖一圈。

---

## 24. 对齐 0.3.20 + 让「模型指纹」测得准

**根因**：客户端 0.3.20（2026-11-07 发布）与网关对齐的 0.3.19 相差一个版本号；
同时用 `lm.ikale.io` 这类模型指纹检测对比「网关 vs 客户端」时，结果差异很大，
但差异**不是**上游换模型，而是三处可解释的原因：

1. **模型名被静默替换**：`registry.Resolve` 的顺序是
   「精确 → 别名 → 内置别名 → 子串 → 默认模型」。目录外的名字（`gpt-6-sol`、
   `mimo-v2.6-flash`）会回落到 `glm-5.3-flash`，`gpt-6` 会被子串匹配成
   `gpt-6-luna`——测的是 A，跑的是 B；
2. **采样条件不同**：客户端（Pi）每个请求都带 system + tools + `reasoning_effort`
   （`thinkingLevelMap:{off:"none"}` 让「关思考」也显式发 `none`），
   而检测器的参考库是在裸请求（无 system / tools / effort / max_tokens）下采的。
   网关的 Anthropic / Responses 翻译又把 `thinking` / `reasoning.effort` 丢了，
   等于把调用方要求的强度降级成上游默认；
3. **推理请求带着注册用的浏览器指纹**：桌面端走 Node fetch，实测只发
   `user-agent: node` / `accept: */*` / `accept-language: *` / `sec-fetch-mode: cors`，
   从没有 Chrome UA 与 `sec-ch-ua`。

决策：

- `AppVersion` → `0.3.20`（只用于 `/theos` 心跳的 `v=`）；车道仍是 4，
  wire protocol 与 0.3.19 完全一致，因此**不动**协议逻辑；
- 新增 `Settings.StrictModel`（控制台「严格模型模式」）：解析不到就返回
  `404 invalid_request_error` + 相近候选，不再回落。宽松路径保持原样；
- `registry` 拆出 `ResolveStrict`（不含子串/回落）与 `Suggest`（相似度提示）；
  `normalize` 把空白当分隔符，`Claude Opus 5.5` 与 `claude-opus-5-5` 等价；
- Anthropic 的 `output_config.effort` / `thinking` → `reasoning_effort`
  （预算 <4k→low、<16k→medium、其余 high；`disabled`→`none`）；
  Responses 的 `reasoning.effort` 同样映射；
- `ChatBodyFallback` 白名单补 `stream_options` / `seed` / `service_tier`，
  已有 system 原样保留（只在缺失时补默认提示词）；
- `deepai.Client.BrowserFingerprint`：注册链路 true（账号 Chrome 指纹），
  网关推理 false（Node fetch 默认头）；`NewClient` / `NewClientWithProxy`
  默认 false，`NewClientWithFingerprint` 保持 true。

**验证**：

```text
go test ./...                                          # 全绿
gpt-6-sol  → 严格模式 404（宽松模式仍回落 glm-5.3-flash）
gpt-6      → 严格模式 404（不再子串命中 gpt-6-luna）
Claude Opus 5.5 → claude-opus-5-5（名称匹配）
推理请求头 → user-agent: node / accept: */* / 无 sec-ch-ua
注册请求头 → Chrome UA + sec-ch-ua + Accept-Language（不变）
Anthropic thinking{disabled} → reasoning_effort=none
Responses reasoning.effort=medium → reasoning_effort=medium
```

> 口径提醒：模型指纹检测的置信度只是**参考库内**的相对概率，
> 同名不代表真实、异名也不代表被替换。要比较必须固定三件事：
> 精确目录 ID、与参考库一致的采样条件（裸请求、流式、不指定 effort）、
> 以及同一账号/出口。判别「网关改写」还是「渠道替换」的最快办法是
> 用同一份 body 直连 `api.deepai.org` 复测一次。

### 24.1 实测：根因是 User-Agent，不是网关改写

2026-11-08 用 lm-detector 的官方路径（`generateChallenges` + `complete` +
`analyzeSharedOutputs` + `data/shared_detector.bin`）做了一次 A/B：同一批
3 个挑战、同一账号、同一台机器，`glm-5.3-flash`：

| 链路 | 请求头 | FPD 排名 |
| --- | --- | --- |
| 网关（改造后） | `user-agent: node` | `glm-5.3-flash` 63% / `glm-5.3-flashx` 37% |
| 直连 | `user-agent: node` | `glm-5.3-flash` 50% / `glm-5.3-flashx` 50% |
| 直连 | `user-agent: Go-http-client/1.1` | **`muse-wonder` 89%** |
| 直连 | Chrome 156 UA | **`muse-wonder` 100%** |
| 网关（改造前的旧二进制，UA 为空 → Go 默认） | `Go-http-client/1.1` | **`muse-wonder` 95%** |

结论：

1. **网关本身是忠实的**：同一份 body 下，网关与直连的排名一致（GLM 家族、同一对候选）；
2. **真正的变量是 `User-Agent`**：非 `node` 的 UA 被上游换到另一条后端，
   指纹变成 `muse-wonder`（Meta 家族），但响应 `model` 仍写 `zai/glm-5.3-flash`；
3. 因此 `BrowserFingerprint=false`（发 `user-agent: node`）不只是“更像客户端”，
   而是**真的决定了你拿到哪个模型**；
4. 顺带发现：裸请求（无 system）只有在**已签到**的进程里才被接受，
   否则 `400 This request is not one DeepAI Code sends.` —— 网关推理前
   `EnsureCheckIn` 正好满足这个条件，直连复现时必须自己先 `GET /theos`。

复现步骤（实验脚本是临时的，不入库）：

1. `curl https://api.deepai.org/deepai_code/models -H 'X-DeepAI-Code-Theos: 4'` 取精确 ID；
2. 用 `lm-detector` 仓库的 `generateChallenges(3)` 生成 3 道挑战并**固定**；
3. 两条链路各发 3 次：`complete()`（chat 协议、`stream:true`、`stream_options`、
   无 system/tools/effort）+ 同一个 `analyzeSharedOutputs()` 打分；
4. 直连侧先 `GET /theos` 拿 `check_in.nonce` 并在 chat 上回传（否则裸请求 400）；
5. 只改 `User-Agent` 再跑一轮，对比排名。

### 24.2 号池体检：30/30 对齐（2026-11-08）

用「免费模型 + 最小请求」把 14 个账号逐个体检了一遍，判定依据是响应里的
`provider` / `system_fingerprint`（见 `docs/recon.md` §3.1 的验证）：

| 场景 | 对齐 | 替身 |
| --- | ---: | ---: |
| 逐号直连，**不签到** | 2/14 | 12/14 |
| 逐号直连，只拉 `/theos` 不回传 nonce | 0/14 | 14/14 |
| 逐号直连，完整签到（每号自己的机器指纹） | 14/14 | 0/14 |
| 逐号直连，完整签到但共用同一个新机器指纹 | 10–12/14 | 2–4/14 |
| **经网关**（每号自己的机器指纹 + 推理前签到） | **30/30** | 0/30 |

结论：

1. **产品路径（经网关）是对齐的**：30 次请求覆盖全部 14 个账号，全部落在真实分支；
2. 准入条件是 **`node` UA + 完成签到**，缺一个就落到替身分支（`provider: DeepAI`，
   指纹是 `muse-wonder`）；
3. 签到状态按 (账号, 机器指纹) 记忆；**共用机器指纹会分摊真实分支名额**，
   这也是「每个账号一份机器指纹」的又一条理由（原结论只提到每日额度）；
4. 唯一时间窗：网关冷启动后 30s 内若立刻推理，可能还没完成签到（`checkInMinInterval`
   兜底 30s）。实测启动探测→首次推理已经跨过该窗口，未观察到替身。

### 24.3 号池体检的两个修正（2026-11-08）

体检（14 个账号 / 免费模型 / 只读判定字段）暴露了两个实现问题：

1. **签到没有「回传」的记账**：`EnsureCheckIn` 只在 `seconds/2` 之后才补
   `/theos`，而「拿到 nonce」和「把 nonce 回传」是两件事 —— 前者不算签到。
   结果冷启动后前 300s 的推理都没完成签到，会落在替身模型分支。
   修法：`checkInState` 增加 `ackedNonce`，拿到挑战但未回传时立即补一次
   `/theos`（`internal/deepai/checkin.go`）；`Theos` 发出带 nonce 的请求后
   记为已回传（403 也算，与桌面端 `noteCheckIn` 一致）。
2. **`check_in_refused` 被当成账号故障**：`probeBatch` 用
   `SetAccountAllowance` 记录任何 `/theos` 错误，会把 `Status` 置成 `error`
   并累加 `Failures`，而这类账号其实还能正常推理（只是落在替身分支）。
   修法：新增 `deepai.IsCheckInRefused` 与 `Store.NoteAccountIssue`（只写备注，
   不动状态与失败计数）。

复测（冷启动、14 个账号、12 次请求）：第 1 次替身（该账号还没有挑战，
只来得及拉 nonce），第 2–12 次全部对齐。修复前是「前 300s 全替身」。

> ⚠️ 体检本身会消耗签到的每日重启配额：写脚本反复「拉 nonce + 回传」会撞上
> `403 check_in_refused`（当天不能再签到）。用网关正常跑不会，因为它每个
> `seconds/2` 才续签一次。

---

## 25. 控制台：K/M 单位 + 补齐后端已就绪的模块

**根因**：两类漂移。

1. **可读性**：概览与日志里的 Token / 字符是纯整数（`1,234,567`），
   一眼看不出量级；模型目录的 1,048,576 同样如此。
2. **前后端能力脱节**：后端早就有 `assign-proxy` / `assign-machine` /
   `set-group` / `mark-verified`、密钥 `set-group`、账号 `group/status/q` 过滤、
   日志 `endpoint/account/min_status/q/since` 过滤，但控制台一个都没接 ——
   这些操作只能靠 curl。

决策：

- **单位**：新增 `fmtCompact()` —— 3 位有效数字 + K/M/B，并修正进位边界
  （`999999 → 1M`，不是 `1000K`）。用于「概览」的输出字符、Token 入/出，
  以及「日志」的字符、Token 两列；**精确值放 `title`**，悬停可查。
  模型目录按用户选择保留精确整数（对照模型规格时需要逐位）。
- **账号**：列表加「分组 / 状态 / 关键字」筛选（直接对应服务端
  `AccountPage` 的 `group/status/q`，分页与排序保持不变）；行内菜单新增
  补绑代理（已有代理则 confirm 后 `reassign`）、清除代理、分配机器指纹
  （只在 `has_machine=false` 时出现）、改分组、标记已验证
  （只在 `email_verified=false` 时出现）。
- **密钥**：操作菜单新增「改绑分组」，走 `POST /admin/api/keys`
  的 `action=set-group`（列表是脱敏 Key，服务端 `resolveKeyRef` 认掩码）。
- **日志**：筛选行接 `endpoint / account / min_status / q / since`，
  `since` 用 `datetime-local` 转 Unix 秒。
- **i18n**：新增 20+ 条英文字典（含模板句与占位符），`?lang=en` 下
  筛选控件、行内动作、确认框都跟着切。

**验证**（2026-11-08）：

```text
node scripts/console-e2e.mjs http://127.0.0.1:10102 ...   ✅ 全部通过
  菜单项: 配额 停用 补绑代理 分配机器指纹 改分组 标记已验证 删除
  排序请求: offset=0&limit=20&sort=theos&order=asc
  EN: Overview / Account / Total requests
CDP 实测筛选:
  账号: offset=0&limit=20&group=default&status=ok&q=zhuhao
  日志: limit=50&endpoint=chat&account=abc&min_status=400&q=busy&since=1794067200
单位:
  3726 → 3.73K（日志「字符」列），title 仍是 154 / 116 这样的精确值
  边界: 999999 → 1M / 999999999 → 1B / 987 → 987
```

新增 `TestConsoleBackendParity` 守住这条契约：筛选参数名、四个 action 名、
`fmtCompact` 的使用点、`strict_model` 绑定，改 HTML 时漏接会直接变红。

## 26. 0.3.20 客户端源码复核：请求头保真度五连修

**根因**：此前的协议细节来自 HAR + 线上实测，属于黑盒推断。0.3.20 的
`server.asar` 里 `src/deepai/api.ts` / `machineId.ts` / `DeepAIAccount.ts`
是**未混淆**的（带注释），可以当白盒基准直接对着看。逐项比对后确认
车道 4、实例号、机器指纹算法、签到形态全部一致（`latest.yml` 仍是
0.3.20，`GET /deepai_code/models` 用车道 4 实测 200），但请求头有五处不保真：

| # | 偏差 | 真实客户端 | 修法 |
| --- | --- | --- | --- |
| 1 | `/get_usage_summary` 带 `User-Agent: DeepAI-Code` | 只有 `/gravity-display-proxy` 覆盖 UA，其余是 undici 的 `node` | 删掉覆盖（`client.go`） |
| 2 | GET 请求不带 `Content-Type` | `requestDeepAIJson` 的 header 字面量对所有方法都带 `application/json` | 挪进 `commonHeaders` |
| 3 | 心跳写死 `focus=1` | `deepAIBeacon` 对未知事实直接省略，只有窗口焦点已知才发 `focus=0/1` | 发 `v=…;up=…;pi=…` |
| 4 | 形状白名单缺 `store` | 桌面端每个 chat 请求都带 `store:false`（`supportsStore=true`） | 补进 `desktopChatKeys` |
| 5 | `Retry-After` 只夹上限 | `min(30, max(1, retry))` | 补 1s 下限 |

`seed` / `service_tier` / `top_p` / `stop` / `prompt_cache_key` 桌面端不发，
但它们是网关调用方显式给的采样/用量旋钮，**故意保留**在兜底白名单里：
形状回退是为了让请求被接受，不是为了把它改成另一个请求。

顺带记录（不改代码，属于有意偏离）：每账号机器指纹、首次无挑战也补签到、
错误码超集（`check_in_refused` / `network_bound` / `social_login_required` ——
**0.3.20 客户端源码里一个都没有**，它只特判 `session_busy`）。

**验证**（2026-11-09）：

```text
go test ./...                                    ✅ 全绿
TestGetRequestsMatchDesktopHeaders  GET /models /theos /get_usage_summary
  → Content-Type: application/json、User-Agent: node
TestRetryAfter                      0 → 1s、-5 → 1s、999 → 30s、abc → fallback
TestChatBodyFallbackKeepsSamplingFields   store:false 原值保留
```

客户端复核方法（当时用，不入库）：`app.asar`/`server.asar` 头部 16 字节
（4 + pickle 长度 + JSON 长度 + JSON），数据区从 `8+pickle` 开始；
`server.asar` 里的 `apps/server/dist/binCli-*.mjs` 就是整套协议实现。

## 27. 渠道模型指纹实测：12/12 真分支，判据要改

**背景**：§26 改完请求头后要验证「渠道是否还在真模型分支」，顺带发现
2026-11-09 起上游的响应形状变了。

**实测方法**（本地网关 + 直连对照）：

```text
网关（14 账号 / 每号独立机器指纹 / 已签到），glm-5.3-flash 非流式 ×12
  → 200 × 12，12/12 真分支
  provider 轮换: Wafer / InferenceNet / Reka / BaseTen / CoreWeave /
                  Makora / Modal / Morph
  单次 1.5–4.7s；日志里 14 个账号各命中一次（轮询公平）
直连对照（同一账号 / 同一机器指纹 / 同一请求体，只换 UA）
  user-agent: node          → provider=InferenceNet   真
  user-agent: Go-http-client→ provider=DeepAI         替身
  user-agent: Chrome/156    → provider=DeepAI         替身
替身响应解包: message.reasoning_details[0] = {type:"reasoning.encrypted",
  format:"openrouter-stealth-v1"}，data 的 JWE payload base64 解出
  {"endpoint_slug":"stealth/crouton-alpha|stealth"}
```

**结论**：

- UA 分流仍然生效（`node` 才进真分支），网关 `BrowserFingerprint=false` 是对的；
- 但**旧旁路判据失效**：「有 `provider` 就是替身」不再成立 —— 上游现在把请求
  转给 OpenRouter 一类路由，真分支的 `provider` 就是真实后端名；
- 新判据（按可靠度）：`provider == "DeepAI"` → 替身；`service_tier == "default"`
  → 替身；`usage.cost_details.upstream_inference_cost == 0` → 替身；
  `reasoning_details[*].format == "openrouter-stealth-v1"` → 替身；
  `system_fingerprint` **不再是判据**（真分支只有 vLLM 后端才给）；
- 两条分支的 `model` 都回 `z-ai/glm-5.3-flash`，不能用来判断；
- 替身今天实际是 `crouton-alpha` 的 stealth 通道（11-08 记录的是 `muse-wonder`），
  换后端了但机制没变。

**改动**：只改文档（`docs/recon.md` §3.1 / §5.1、README §1.2 与 §10.1），
代码不依赖这两个字段。

**未做**：用新判据重测「签到是不是准入条件」（§5.1 的表格是按旧判据测的，
需要冷启动一对新的 (账号, 机器指纹)）。签到是有限资源，没在这轮消耗。

---

## 28. 实测复核：策略已放开 + Google 登录回调边界 + oauth 入池补全

**背景**：`accounts/` 新增两个 11-09 的 HAR（`pwvjkhiq1hwz@lmoir.com`、
`zupxzenogljb@lmoir.com`），需要入池；同时确认能否用 Google OAuth 登录
取代私域邮箱注册。

**实测（2026-11-09，直连上游）**：

1. **`social_login_required` 已解除**。7 个账号（2 个 11-09 新号 + 5 个 11-07
   旧号，全部邮箱/密码注册）`/theos` 全部 200（lane 4、check_in 正常，
   余额 50/200）；其中 3 个做了真实推理，`glm-5.3-flash` 全部 200。
   结论：0.3.19 的收窄是阶段性策略，邮箱注册链路重新可用。
2. **Google 登录无法服务端代办**。allauth 端点
   `https://deepai.org/accounts/google/login/?process=login` 302 到 Google，
   `redirect_uri` 固定为 `https://deepai.org/accounts/google/login/callback/`，
   `state` 绑定 deepai.org 的 `sessionid`，code 换 token 需要 DeepAI 的
   `client_secret`；GIS 按钮的 `id_token` 又要求 `aud` 是 DeepAI 的 client_id。
   浏览器那一跳无法绕过。
3. **我们的回调本来就存在**：控制台「浏览器登录」→
   `GET /deepai_code/desktop_auth/?state&port` → 浏览器完成 Google 登录 →
   DeepAI 回跳本机 `http://127.0.0.1:<port>/auth-callback?code=…` →
   `POST /deepai_code/desktop_auth/exchange/` 换 Key（`internal/server/login.go`）。
   实测把链路走到登录页：页面 200、含 `Continue with Google` 与
   `google_client_id=465558899080-…`。

**HAR 入池结果**：

```text
POST /admin/api/accounts action=enroll（每号独立 machine_id）
  pwvjkhiq1hwz@lmoir.com → id 48a58e6465179d1a，ok，200 Theos
  zupxzenogljb@lmoir.com → id 517eea0453c5f38a，ok，200 Theos
action=assign-proxy（12 个出口轮询）→ :10001 / :10002
action=checkin → 2/2 ok；重启后回读仍为 2 个，状态 ok
```

**决策 / 改动**：

- `oauth` 入池（`handleLoginPoll`）此前不写机器指纹与代理：前者会让多个
  Google 号共用本机指纹、只有第一个能领每日额度；后者让所有号挤在同一个
  直连 IP（`network_bound` 是 IP 级共享）。现在两处都补齐：
  - 无 `machine_id` → `deepai.NewMachineUUID()`（与 `action=enroll` 一致）；
  - 有代理池 → 入池前 `pool.Next()` 写入 `acct.Proxy`，探活直接复用该出口；
- 代理池改为 **进程内单例**（`Server.enrollPool`）：浏览器登录一次只入一个号，
  每次新建池会让轮询游标从 0 开始，等于所有号都分到第一个出口；
- 配置解析抽成 `loadProxyPool()`（`PROXY_POOL` → `PROXY_POOL_FILE`），
  与 `action=assign-proxy` 共用，避免两处漂移；
- 文档口径修正：README 顶部与 §10.1、`docs/lessons.md` 不再把「邮箱号不可用」
  当现状；`docs/recon.md` 新增 §4.1（策略放开实测）与 §4.2（Google 回调边界）。

**验证**：

```text
go build ./...            OK
go vet ./internal/server  OK
go test ./internal/server OK（新增 2 个用例）
  TestLoginPollAssignsMachineID  → 来源 oauth、machine_id 非空
  TestLoginPollAssignsProxyFromPool → 连续两次入池轮询到不同出口
```

**未做**：真机上用 Google 账号走一遍控制台登录（需要人工完成 Google 侧
登录/2FA），以及把浏览器登录做成批量队列——Google 的风控决定了它只能
一号一次人工点击。

---

## 29. 移除私域邮箱注册模块（前端 + 后端 + CLI）

**背景**：账号接入已收敛到「浏览器登录 / 桌面端导入 / 手工 Key」三种方式。
邮箱注册链路（临时邮箱 → 注册 → 收信验证）与上游策略耦合深，且 0.3.19 的
`social_login_required` 随时可能再次收紧；产品决定整体移除，不再保留双轨。

**改动**：

- 前端 `internal/server/web/index.html`：删除「批量注册」导航、整页面板、
  轮询/渲染/启动 JS 与 8 条 i18n 词条；控制台只剩概览 / 账号池 / 密钥 /
  日志 / 设置。
- 后端：删除 `/admin/api/register`（GET / POST / DELETE）与任务调度
  （`registerJob` / `runRegisterJob` / `reverifyPending` / `localEnroller` /
  `enrollRecord` / `sanitizeRegisterRecord`）；保留号池写入口
  `enrollRegistration`、`markAccountVerified`、`allowanceFromEnroll`、
  `scheduleQuotaReprobe`，迁到 `internal/server/enroll.go`。
- 删除 `cmd/register` 与 `internal/register` 整个包（pipeline / mailbox /
  web / ledger / deliver / reverify / fingerprints / types 及其测试）。
- 号池仍在用的代理池拆到 `internal/proxy`（`ProxyPool` / `ParseProxyList` /
  `ProxyLabel` / `ReadProxyFile` / `LoadConfig`）；`DEEPAI_REGISTER_PROXY_POOL`
  作为 `PROXY_POOL` 的兼容别名保留。
- `Makefile` 去掉 `register` 目标；`.env.example` 删除 TEMPMAIL / 注册行为
  变量；README §六改写为「账号接入与号池治理」，管理接口、环境变量、目录树、
  已知坑同步收敛。

**验证**：

```text
go build ./...   OK
go vet ./...     OK
go test ./...    OK（internal/proxy 新增 5 个用例）
控制台 grep：register / 批量注册 / tab-register = 0
```

**影响**：不再具备自动批量产号能力；每个 Google/邮箱账号需要在浏览器完成
一次登录（边界见 `docs/recon.md` §4.2）。代理池、机器指纹、额度复探、
分组轮询均保留。

---

## 30. 客户端 0.3.30 / 0.3.32 复核：协议未变，真/替身判据重写

**根因**：仓库对齐的客户端版本停在 **0.3.20**（`AppVersion`、README、recon 都是
2026-11-07 的快照）。2026-11-10 复核发现上游已发到 **0.3.32**，且推理响应的
替身分支换了形状 —— 旧的「`provider == "DeepAI"` 就是替身」判据会把替身当
真模型。

**客户端更新核对**（白盒，解包 0.3.30 / 0.3.32 的 `app.asar`）：

- 最新版：官方更新源 `downloads.deepai.org/deepai-code/releases/latest.yml`
  = **0.3.32**（2026-11-10T04:28Z，releaseNotes 只有两条 bugfix）。
  本机装的是 0.3.30；`deepai.org/download/deepai-code/*` 的 302 仍指 0.3.30 ——
  下载页与更新源不同步，判断“最新”看 `latest.yml`。
- 0.3.30 与 0.3.32 的 41 个 `src/deepai/*` region 逐行 diff：差异**只有**
  压缩后的符号重编号（`make$102→make$100`、`locks make$157→make$155`），
  `api.ts` / `machineId.ts` / `apiOrigin.ts` / `localTransport.ts` /
  `DeepAIAccount.ts` / `modelTransportPolicy.ts` 逻辑零变化；pi 运行时同为 1.3.0。
- wire protocol 三方（0.3.20 / 0.3.30 / 0.3.32）一致：端点集合、5 个
  `X-DeepAI-Code-*` 头（车道仍 `4`）、实例号粒度、`sha256(小写原始 UUID)`
  机器指纹、签到 nonce 与 beacon 形态全部未变。只有同步的差异项是 beacon 的
  `v=` 版本号；另外 0.3.30 起 `pi=` 变成可选（未转发过推理时整段省略，
  网关恒发 `pi=<0|1>` 仍然合法）。

**线上实测（2026-11-10，真实账号，0.3.20 形状的请求）**：

```text
/models  lane=4 → 200（22 个模型）；lane=2/3/5/7/8 → 426 lane_version_unsupported
/theos   200 lane_version=4，check_in{nonce=32hex, seconds=600}，balance=50
回传 nonce 再 /theos → 200 并下发新 nonce
/chat/completions 200 provider=Wafer service_tier=null cost=3.29e-05
```

→ 网关的请求头 / 指纹 / 签到链路**不需要任何协议改动**，本次只跟版本号。

**判据变更（docs/recon.md §3.1 整体重写）**：

| 字段 | 真分支（`node`） | 替身分支（Chrome / Go UA） |
| --- | --- | --- |
| `id` | `gen-<epoch>-<rand>` | `gen_<ULID>` |
| `model` | 回显请求 slug：`z-ai/glm-5.3-flash` | **`reflection/beam`** |
| `provider` | `Wafer` / `InferenceNet` / … | **字段缺失**（不再有 `"DeepAI"`） |
| `service_tier` | `null`（字段在） | 字段缺失 |
| `system_fingerprint` | `null` | **`fp_i144zcu3ag`** |
| `reasoning_details[0].format` | `reasoning.text` | **`unknown`**（不再是 `openrouter-stealth-v1`） |
| `usage` | `upstream_inference_cost > 0` | `null`/`0`，且多一个 `market_cost: 0` |
| 顶层 `generationId` | 无 | 有 |

新判据（按可靠度）：`provider` 缺失 → 替身；`model` 不回显请求 slug → 替身；
`cost_details.upstream_inference_cost > 0` → 真；`id` 前缀 `gen-`/`gen_`、
`usage.market_cost` 出现可作旁证。`system_fingerprint` 与 `service_tier`
**彻底不能再用**。

**改动**：

- `internal/deepai/types.go`：`AppVersion` `0.3.20` → **`0.3.32`**（只影响心跳 `v=`）。
- `README.md`：§一版本号、§1.2 的替身判据、§二模型表（18 → **22**，11 免费 +
  11 付费；新增 `qwen-3.8-flash` / `minimax-m3.1-flash` / `minimax-m3` /
  `qwen-3.8-max-prime`；`claude-fable-5-1`(25) → `claude-fable-5-2`(20)）、
  §十 两条限制、§10.1 踩坑表那一行。
- `docs/recon.md`：头部 banner（0.3.30/0.3.32 复核结论 + 下载页不同步的坑）、
  §1/§2/§3/§5.1 标题版本、§3 补 0.3.32 头字面量复核、**§3.1 判据整节重写**、
  §5.1 补 `pi` 可选、§5.1 历史测量表加“判据已过期”的注。

**验证**：

```text
go build ./...   OK
go vet ./...     OK
go test ./...    OK
线上 4 条实测见上（/models 分车道、/theos + 签到回传、chat 真分支、UA 分流对比）
```

**影响**：号池、调度、翻译层零改动；过期的是“看到 `provider: DeepAI` 才叫替身”
这条口头判据 —— 还在用旧判据做渠道体检的脚本/文档需要同步，否则会把替身
记成真模型。

---

## 31. 浏览器登录支持「粘贴回调 URL」，Docker 部署不再卡在回调页

**根因**：浏览器登录走的是 RFC 8252 本机回调 —— 网关在**自己进程内**监听
`127.0.0.1:<随机端口>`，把这个端口写进上游的
`/deepai_code/desktop_auth/?state=…&port=…`，等浏览器回调
`http://127.0.0.1:<port>/auth-callback?code=…&state=…`。

Docker 部署下这条链路必然断：

1. 监听发生在**容器内**的 loopback，宿主机浏览器打不开；
2. 端口是**每次登录随机分配**的，`-p` 发布静态端口也映射不到容器的
   `127.0.0.1`（Docker 的 DNAT 目标是容器 eth0，不是它自己的回环）；
3. 于是浏览器停在“无法访问此页面”，而控制台那头只能一直 `pending`。

`code` 与 `state` 在地址栏里是完好的，兑换接口
（`POST /deepai_code/desktop_auth/exchange/`）也只认 `code`、不需要端口 ——
也就是说这条路本来就能走「授权 → 贴回调 URL」，只是之前没实现。

**改动**：

- `internal/server/login.go`
  - 新增 `POST /admin/api/login/callback`：接受完整回调 URL（或裸 query 串），
    校验 `state` 与本次会话一致后兑换 `code` 并入池；
  - 登录会话加 `exchanging` / `consumed` 两个状态：本机回调与粘贴是两条并行
    入口，**同一个 code 只能兑换一次**（并发时后者 409），入池也只做一次；
  - `poll` 里的入池逻辑抽成 `completeLogin`，两条入口共用（机器指纹、代理池
    分配、探活、关监听、删会话的语义完全一致）；
  - 本机监听绑不上回环时不再 500 直接失败，改为回退一个高位端口（回调必然
    打不开，正好走粘贴），响应新增 `loopback` 字段；
  - 兑换失败按上游状态码给可读提示（400/401/403 → “已用过或已过期”，
    429 → “登录尝试过多”）。
- `internal/server/server.go`：注册新路由。
- `internal/server/web/index.html`：登录面板加“粘贴回调 URL”输入框 + 按钮 +
  英文词条；发起新登录时清空输入框。
- `README.md`：管理接口表补三条登录接口；Docker 章节补“容器里必须走粘贴回调”
  的说明与 `--network host` 的例外。

**验证**：

```text
go build ./... / go vet ./...   OK
go test ./internal/server/ -run Login -v
  TestLoginPollAssignsMachineID            PASS
  TestLoginPollAssignsProxyFromPool        PASS
  TestLoginCallbackPasteCompletesLogin     PASS   # 贴 URL → 入池 → 二次粘贴被拒
  TestLoginCallbackRejectsForeignState     PASS   # state 不匹配 400
  TestLoginCallbackRejectsBareCode         PASS   # 只粘 code 400（拿不到 state）
  TestLoginCallbackExchangeFailure         PASS   # code 过期 → 400 + 可读提示
```

**影响**：非容器部署（本机直跑）体验不变，回调自动收；Docker / 远程部署从
“无解”变成“复制地址栏 → 粘贴一次”。注意 `code` 一次性：粘贴成功后同一段 URL
再粘会被拒（会话已结束，返回 404），需要重新发起登录。

---

## 32. 控制台：登录链接可复制 + 主题按钮换 SVG 图标

**动因**：Docker / 远程部署时，登录页往往要在**另一台机器**的浏览器里打开
（常驻服务器的网关没有桌面环境），而之前登录链接只存在于一个 `<form
action="/login/open">` 里 —— 想复制只能右键「复制链接地址」，且拿到的还是相对路径。

**改动**（`internal/server/web/index.html`）：

- 浏览器登录面板新增只读输入框 + 「复制登录链接」按钮：
  - 发起登录后写入 **绝对地址** `location.origin + /login/open?id=…`，
    换浏览器 / 换机器只要能访问网关就能完成授权；
  - 复制优先走 `navigator.clipboard`，非安全上下文（http + 非 localhost）
    退回「隐藏 textarea + execCommand」；两者都失败就选中文本提示 Ctrl+C；
  - 链接含一次性 `state`，会话 10 分钟过期，面板上写明。
- 主题按钮从 emoji 文案改成内联 SVG（太阳 / 月亮两个图标切显隐，`aria-hidden`），
  文案仍走 i18n 的「深色 / 浅色」；`applyTheme` 清掉 `__zh` 缓存后再翻译，
  否则切回英文会复现上一次的标签。
- 清掉一条重复的 i18n 键（「清空」出现两次）。

**验证**：

```text
go build / go vet / go test ./internal/server   OK
console_test.go: TestConsoleThemeAndLoginLink  PASS
  - 断言 themeIconSun / themeIconMoon / themeLabel / oauthLink / oauthCopy 存在
  - 断言 emoji 文案已被移除、applyTheme 只切「浅色 / 深色」
  - 断言登录链接按 location.origin 拼绝对地址
内联 <script> 过 node --check；i18n 对象无重复键；新增文案均有英文
```

---

## 33. `internal/deepai` 覆盖率 56% → 92.8%

**根因**：协议层（`deepai`）是上游一改就最先炸的模块，却是全仓库覆盖最低的一个。
`go tool cover -func` 显示 40+ 个函数 0 覆盖，集中在构造器（`NewClient*`）、
扩展端点（`WebSearch` / `Transcribe` / `ExchangeCode`）、凭据导入
（`ImportAllFromDesktop`）与机器指纹解析（`parseUUID` / `parseMachineGuid` / `run`）。
这些恰恰是「上游改协议 / 换机器 / 换部署方式」时最先出问题的地方。

**改动**：新增 7 个测试文件（约 1200 行），沿用既有 `httptest` 假上游，不引第三方断言库。

| 文件 | 覆盖对象 |
| --- | --- |
| `types_test.go` | `Model` 三个谓词、`Allowance`/`Account` 深拷贝、`MergeFrom` 的两条策略、`GroupName`、`KeyID`/`SessionID` |
| `helpers_test.go` | 随机串 / 指纹 / 脱敏、`MaskProxy` 的「绝不泄露凭据」断言、`EnvInt`/`EnvOr`、桌面端目录探测的四种形态、`dedupe` |
| `client_endpoints_test.go` | `NewClient*` 三种形态、`DEEPAI_API_ORIGIN` 校验、代理接线、`SignInURL`、`WebSearch`/`FindService` 逐字节透传、`Transcribe` 的 multipart 与 8 MB 上限、`ExchangeCode`、`EmailUnverified`/`IsSocialLoginRequired` |
| `machine_extra_test.go` | `parseUUID`（含三种固件占位）、`parseMachineGuid`、`formatUUID`、`run` 的三条路径、`MachineFingerprint` 归一化 |
| `import_test.go` | 目录 / 文件两种导入入口、重复 Key 去重、损坏凭据报错、`AccountFromAPIKey` 边界（512 字节恰好合法）、`AccountFromRegistration`、`EnvAccount` |
| `checkin_extra_test.go` | 签到状态按 Key 隔离、静默锁、`EnsureCheckIn` 的节流与解锁、心跳 `pi` 字段、`CheckIn` 的防御性拷贝 |
| `client_misc_test.go` | SSE 写入的三段错误、`BodyRejected` 必须同时满足 400 + 文案、连接失败路径、`hasSystemMessage` 脏数据、`detectMachineRaw`（仅非 Windows，见下） |

**顺带修掉的 bug**：`AccountFromRegistration` 对 `Email` 做了 `TrimSpace`，
却把**原始** `r.Email` 写进了 `Name` / `Username` —— 注册链路带空白时会污染
账号名与用户名。这是新测试第一次运行就抓到的（`Name: "  bob@example.com  "`）。
改为一处 `email := strings.TrimSpace(r.Email)` 统一使用。

**验证**：

```text
go test ./internal/deepai     coverage: 56.1% → 92.8%
go test ./...                 10 个包全绿（server 2.6s）
gofmt / go vet                PASS
用例数                        260 → 315
剩余未覆盖分支：rand.Read 失败、json.Marshal 失败、Windows 的
detectMachineRaw（会拉起 powershell，12s 超时）—— 均不可注入
```

`detectMachineRaw` 的 Linux/macOS 分支用 `runtime.GOOS` 门控，
在 CI（ubuntu）上会真实执行并覆盖，在 Windows 开发机上跳过。

---

## 34. 文档偏差修正（三处）

一次全仓扫描（`project_report` + `cover -func`）暴露了三处「文档与代码不一致」，
不影响运行，但会误导下一次改动：

| 位置 | 偏差 | 处理 |
| --- | --- | --- |
| `README.md` §八 架构树 | 漏了 `internal/pool` / `parallel` / `retry` 三个包（§9.3 拆分后就没补） | 补上三行，注明各自职责 |
| `docs/optimization-plan-r2.md` §2 与 §P3 | 前端写的是「876 行单文件」，实际已 1687 行，且越过了本文档自设的 1500 行拆分阈值 | §2 标注为「本轮改版前基线」；§P3 追加现状与拆分建议 |
| `docs/changes.md` | 1386 行按时间正序追加，最新结论在末尾，倒查成本高 | 顶部加「索引（倒序，最新在前）」，并补 §33/§34 |

README 的「可回归」一行同步更新为当前事实（315 用例 / 8.8k 行测试）。

---

## 35. HAR 抓包清理：凭据只保留在加密状态文件里

**动因**：仓库目录下积了 **99 MB 明文凭据** —— `accounts/` 15 个 HAR（84 MB）
加根目录 3 个逆向抓包（15 MB）。HAR 里既有上游 API Key，也有当时有效的
Cookie / 会话。`.gitignore` 挡住了入库，但**留在磁盘上就是明文**，
和「状态文件必须加密」的既定策略自相矛盾。

**不能直接删的原因**：先做了核对，不是所有 Key 都已入库。
用指纹（`sha256` 前 16 位）比对后：

| 来源 | Key 数 | 状态文件里是否已有 |
| --- | --- | --- |
| `accounts/*.har` 13 个 | 13 | 已入池（`status=ok`） |
| `accounts/deepai.org.pwvjkhiq1hwz.com.har` | 1 | **缺失**（`cb6ed3f5a44f1113`） |
| `accounts/deepai.org.zupxzenogljb.com.har` | 1 | **缺失**（`86afdbcf43310024`） |
| 根目录 `deepai.org-1.har` / `-2.har` | 1（两文件同 Key） | **缺失**（`5fdd083dfe798e06`） |
| 根目录 `deepai.org.har` | 1 | **缺失**（`f492f607eb3b45ec`） |

**决策**：先迁移，再删除。

1. 4 个孤儿 Key 以 `Source=har-import`、`Enabled=false`、`Status=unknown`
   写进状态文件，`Note` 记录来源 HAR 文件名；
   **停用**是关键：未验活的号不参与调度，不会污染号池、也不会在后台
   探测里打出无谓的上游请求；
2. 状态文件本身就是 AES-256-GCM 信封，删掉 HAR 后凭据只以密文存在；
3. 删除 18 个 HAR 文件，目录从 ~131 MB 降到 32 MB。

**验收**：

```text
状态文件账号 14 → 18（可调度仍为 14，新增 4 个 enabled=false）
清理前审计：15 个 HAR / 18 个 Key，4 个未入库      ← 直接删会丢 4 把 Key
清理后审计：0 个 HAR，未入库 Key 0
go test ./...  10 个包全绿（状态文件改动未影响 store）
console / 号池：14 个在线账号 status=ok 保持不变
```

**经验**：清理前的「看起来都导过了」必须用**指纹比对**证明，不能靠印象。
第一次审计脚本的匹配式漏掉了 HAR 里的 JSON 转义形态（`api_key\":\"…`），
对 15 个文件全部返回「0 个 Key、全部已入库」——一个会直接导致丢号的假绿灯。
如果审计结果的 Key 数为 0，先怀疑正则，再相信结论。

> 4 个停用账号可在控制台里验活后启用；确认无用则直接删除。
> `deepaicode2api-data.json.pre-harcleanup.bak` 是清理前的加密快照，确认无误后可删。

---

## 36. 解耦一轮：协议层独立成包 + god module 拆分（P0/P1/P3）

依据是一次全仓扫描（`project_report` + 逐函数覆盖 + 重复代码检测）。
结论：**依赖分层没问题**（0 环、0 越层），问题在文件粒度与包粒度。

### 36.1 P0：新增 `internal/gateway`（协议转换层）

`anthropic_translate.go` / `responses_translate.go` / `anthropic_stream.go` 有
**零个 `*Server` 依赖**——它们只是 map ⇄ struct 的映射。既然不依赖 HTTP、
store 或号池，就应该住在自己的包里：

- 迁入 `internal/gateway/`：字段映射、流式内容块管理器、`Usage` + 用量解析；
- `internal/server/gateway_alias.go` 用**类型别名 + 函数变量**保留包内短名，
  既有 handler 与 1,000+ 行测试**一行都没改**（迁移是纯搬家，不是行为改动）；
- `responses_stream.go` 留在 server：它直接写 `*sseWriter`，属于 HTTP 层。

效果：协议层可以脱离网关单独 evolve（上游一改协议，改动面收在一个包里），
`go test -coverpkg=./internal/gateway/ ./internal/server/` = **78.4%**。

### 36.2 P1：`common.go` 797 行 → 6 个职责文件

| 新文件 | 内容 |
| --- | --- |
| `models.go` | 目录解析、别名回落、未知模型提示 |
| `accounts.go` | 账号槽位、单号并发上限、准入排队、失败原因分类 |
| `upstream.go` | `openModel`（选号 / 占槽 / 失败换号 / 错误分类） |
| `reqlog.go` | 单请求埋点与统计 |
| `sse.go` | `sseWriter`、`jsonEvent`、`[DONE]` |
| `request.go` | body 上限、affinity、prompt 长度估算、分组 / 账号选择 |

`common.go` 只剩准入常量，改名 `admission.go`（14 行）。

### 36.3 P0/P1：三个 0% 覆盖的在用端点 + 两处显示不一致

`/admin/api/{keys,stats,models}` 是控制台主路径，此前**零测试**（`console_test`
只校验端点名存在）。新增 `admin_surface_test.go` 后顺带抓到两个真实问题：

1. `GET /admin/api/models` 走 `Store.Models()`，而 `/v1/models` 走带兜底的
   `s.catalog()` —— 新装且上游不可达时，**控制台模型页空白，API 却有兜底模型**；
2. `defaultModelID()` 在「目录有兜底项但 `DefaultModel` 为空」时返回空串，
   而推理实际已回落到目录首项（`registry.Resolve` 第 37 行）——**显示与行为不一致**。

两处都改为以 `s.catalog()` / `s.defaultModelID()` 为准。

### 36.4 P1：i18n 缺口从「静默」变「CI 报红」

控制台用**中文原文当 i18n 键**：漏加词条时翻成英文会静默保留中文。
实测 226 条词条 vs 232 处候选文案，**缺 25 条**（用户看到的是中英混排）。

- 补齐 17 条真漏翻（`严格模型模式` / `请求日志` / `检测中…` / `登录失败：` 等）；
- 新增 `console_i18n_test.go`：抽出 JS 字面量 + 标记文本 + placeholder/title 里的
  中文，与字典求差集，只允许 8 条**结构性片段**缺席（拼接片段 / 语言选择器
  自指的 `Language / 语言` / JS 求值后才带换行的那条），每条都在名单里写明理由；
  同时断言名单只变短、字典无重复键（重复键会静默覆盖）。

### 36.5 P3：删除不可达的 `Close` 链

`Store.Close()` 从未被调用，它唯一的动作是调 `logStore.Close()`（空操作）。
两者一并删除；`grep` 确认无调用者。

### 验证

```text
go build ./... / go vet ./...                        PASS
go test -count=1 ./...                               11 个包全绿
internal/server 覆盖率                                70.4% → 71.5%
internal/store  覆盖率                                78.7% → 79.1%
internal/gateway                                      新增包，经 server 测试 78.4%
gofmt                                                clean
```

### 未做（下一轮）

- **P1-2** `handleAdminAccounts`（255 行 / 10 action / 30 字段万能请求体）→ 表驱动
  + 每 action 独立结构体。要逐个分支搬运并保留行为，建议单独一次 review。
- **P2** 前端 1,706 行单文件 → 原生 ES module 拆分（`web/js/*.js`，`go:embed` 改一行，
  零构建工具链）；`main.go` 装配拆分。

---

## 37. P1-2 / P2：账号 action 表驱动 + 前端拆 ES module + main.go 拆分

### 37.1 P1-2：`handleAdminAccounts` 255 行 → 27 行

**根因**：一个 255 行的 switch 挂 10 个 action，共享一个 30 字段的「万能请求体」。
`action=remove` 只用 `id`，却和 `action=enroll`（20 个字段）共用同一个结构体，
必填项与可选性完全看不出来。

**改法**：两步解析。

1. 先只解出 `action`（`adminActionEnvelope`）；
2. 再由表 `adminActions` 分发给该 action 的处理函数，每个函数解**自己的**最小结构体
   （`adminImportReq` / `adminEnrollReq` / `adminIDReq` / …）。

| 文件 | 内容 | 行数 |
| --- | --- | --- |
| `admin_accounts.go` | `handleAdminAccounts`（27 行分发）+ `handleAdminCheckin` | 163 |
| `admin_actions.go` | 10 个 action + 各自的请求结构体 | 399 |

新增 `admin_actions_test.go` 补上改造前**零覆盖**的 5 个 action
（`import` / `manual` / `remove` / `toggle` / `clear-errors`），
外加「未知 action 文案带名字」「非法输入 400」「PUT 405」三条边界。

**唯一的行为差异**：某个 action 的请求体里带了**别的** action 才用的非法字段时，
以前整体解码失败，现在忽略——只会更宽容，不会更严格。

### 37.2 P2：控制台 1,706 行单文件 → 标记 + 样式 + 3 个 ES module

**根因**：CSS、标记、JS 全在一个 HTML 里；i18n 字典、格式化函数、渲染逻辑互相穿插，
改任何一处都要在 1,700 行里找位置。参考项目（minimax2api）用的是 React + Vite，
但本项目「单二进制 + 无构建步骤」是**有意决策**（`optimization-plan-r2.md` §P3），
所以走**原生 ES module**：零工具链、零构建，`go:embed` 仍然一行。

```text
web/index.html   288 行   标记 + <link> + <script type="module">
web/style.css    254 行   设计令牌与布局（原 <style> 原文搬出）
web/js/app.js    891 行   渲染 / 事件 / 轮询 / 主题
web/js/i18n.js   244 行   字典 + tr/t/translateDom/applyLang（零依赖）
web/js/format.js  45 行   时间 / 数字 / 成本 / 量级压缩（零依赖）
```

配套改动：

- `web.go`：`embed.FS` 嵌入整个 `web/`，`/style.css` 与 `/js/<name>` 走**白名单**分发
  （只允许两个入口，不再讨论路径穿越）；所有资源 `Cache-Control: no-store`，
  避免升级后仍用旧 JS。
- `console_test.go`：新增 `consoleSource()` / `consoleMarkup()` / `consoleStyle()` /
  `consoleJS()`。**这是本次最容易埋雷的地方**——契约测试原本靠 `strings.Contains`
  扫单文件，拆文件后如果不改成扫全部脚本，测试会全绿而检查早已落空。
- `console_i18n_test.go`：字典定位改为**字符串感知的花括号配对**
  （原先按「行首缩进 `};`」定位，字典一旦换文件缩进就变，会直接 panic）。

**拆分过程抓到的真问题**：`lang` 变成 i18n 模块私有后，`app.js` 里还有一处
`lang = …` 直接赋值 —— 点击语言按钮会抛 `ReferenceError`。用静态引用检查
（导出符号 vs 导入清单 + 私有符号残留扫描）发现并改为 `setLang(getLang() ? …)`。

### 37.3 P2：`main.go` 424 行 → 装配 + 任务 + 环境

| 文件 | 内容 | 行数 |
| --- | --- | --- |
| `main.go` | `version` + `main()`：参数 → 存储 → 路由 → 信号 | 220 |
| `tasks.go` | `background` / `quotaGuard` / `autoImport` / `runImport` / `runSeal` | 159 |
| `env.go` | `defaultDataPath` / `envOr` / `envInt` / `envBool` / `isLoopbackListen` | 71 |

### 验证

```text
go build ./... / go vet ./...             PASS
go test -count=1 ./...                    11 个包全绿
node --check web/js/*.js                  3 个模块全部通过
控制台契约测试（8 个） + i18n 棘轮测试      全绿
internal/server 覆盖率                     71.5%（新增 action 测试后再未下降）
gofmt                                      clean
```

**说明**：本轮只拆了零依赖模块（i18n / format）。`app.js` 里剩余的
`renderAccounts` / `renderLogs` / `renderKeys` / `renderSettings` / 事件接线
仍在一个文件里——它们是共享 `state` / `acctPage` 的紧耦合函数，需要先抽
`store.js` 再拆，属于下一轮；现在拆会在没有浏览器端运行时测试的情况下
引入静默回归风险。

---

## 38. P2 收尾：拆开 app.js 的四个渲染模块 + 控制台 DOM 冒烟

上一轮（§37）只拆了零依赖模块，理由是「渲染函数共享 state，且没有前端运行时
测试，拆了会静默断」。这一轮先补上那个前提，再拆。

### 38.1 先建安全网：`scripts/console-smoke.mjs`

用**极简 DOM 桩**在 Node 里真正执行 `web/js/*.js`：

- `import app.js` → 触发全部顶层接线（事件绑定、`applyLang`、`refresh` 调用链）；
- 再给四个渲染函数喂固定数据，确认「不抛异常 + 真的往表里写了行」；
- 配一个纯函数断言（`fmtCompact(1500) === "1.5K"`）防止顺手改坏格式化。

`internal/server/console_smoke_test.go` 在 `go test` 里调用它，**没装 node 就
`t.Skip`**：CI（ubuntu-latest 自带 Node）会真跑，本地按需装。

**为什么非它不可**：`go test` 只能断言字符串存在；`node --check` 按 CJS 解析
单个文件，会放过 ESM 的顶层 `return`。本轮它就抓到三个 Go 测试和 `node --check`
都放过的真问题：

| 问题 | 后果 | 谁能发现 |
| --- | --- | --- |
| 重复 import 同一个符号 | 模块直接 SyntaxError，整个控制台白屏 | 冒烟 |
| 抽屉段删掉 IIFE 外壳后留下顶层 `return` | 同上 | 冒烟（`node --check` 放过） |
| `state.js` 根本没被生成 | `ERR_MODULE_NOT_FOUND` | 冒烟 |

另外两处是静态引用检查（自写脚本：导出符号 vs 导入清单 + 私有符号残留）发现的：

- `app.js` 里残留 `lang = …`（`lang` 已是 i18n 模块私有）→ 点语言按钮 ReferenceError；
- `updateAcctPager` 里 `acctPage` 未改成 `ui.acctPage` → 翻页时 ReferenceError。

### 38.2 拆分结果

新增 6 个模块，`app.js` 从 891 → 509 行（只剩事件接线 / 轮询 / 主题 / 启动）：

```text
js/state.js          12 行   共享可变状态（acctPage / acctPageSize）
js/dom.js            66 行   $ / mk / cell / row / pill / emptyRow / 行内菜单
js/table.js          40 行   表头排序状态与纯函数
js/api.js            38 行   管理接口调用（鉴权 / 错误归一 / GET 去重）
js/ui.js             16 行   toast
js/actions.js        55 行   accountOp / keyOp / checkin（刷新通过 setReload 注入）
js/render/accounts.js 127 行 renderAccounts / updateAcctPager / rowMenu
js/render/keys.js     62 行  renderKeys / keyMenu
js/render/logs.js     49 行  renderLogs
js/render/settings.js 14 行  renderSettings
```

**依赖方向**：`i18n / format / state / dom / table`(叶子) ← `api / ui / actions` ←
`render/*` ← `app`。`actions` 需要在操作后刷新，而刷新属于 app 的职责，
用 `setReload(refresh)` 注入，避免 `actions ⇄ app` 循环导入。

配套修正：

- `go:embed` 加 `web/js/render/*.js`；`consoleJS()` 递归两个目录
  （只扫顶层会让契约测试对 render/* 部分「看起来通过」）；
- 资源路由白名单加 `js/render/<name>.js` 一档，仍然拒绝子目录与 `..`；
- `web_test.go` 补 render/* 的 200 断言与 `/js/render/../../app.js` 的拒绝断言。

### 验证

```text
node scripts/console-smoke.mjs        7 项全过
go test ./internal/server/            含 TestConsoleSmoke / 资源路由 / i18n 棘轮
go test -count=1 ./...                11 个包全绿
go build / go vet / gofmt             PASS
```

前端总行数 1,706（单文件）→ 288 标记 + 254 样式 + 1,277 脚本（10 个模块）。

### 遗留

`app.js` 仍有 509 行（事件接线 + 轮询 + 主题 + OAuth 流程）。再拆需要把
「事件绑定」独立成 `events.js`，但那里都是 DOM 副作用、几乎没有可断言的行为，
收益低于风险，暂不处理。

> **后续（§51，2026-11-12）**：这条已落地。「事件接线」拆到 `js/events.js`（207 行）、
> 主题/侧边栏/抽屉到 `js/theme.js`（84 行）、分页到 `js/pager.js`（39 行），
> `app.js` 降到 231 行（只剩取数 + URL 状态 + 装配）。风险靠既有的
> `node scripts/console-smoke.mjs`（import 即执行全部顶层接线）兜住。

---

## 39. 模型目录：用真实快照刷新，并把「计费口径」收成一处

### 39.1 数据来源：状态文件里的缓存目录

README §二 的表格此前是手写快照，写着 **22 个**模型（11 免费 / 11 付费）。
用临时工具从加密状态文件（`store.Open` → `Store.Models()`）导出真实目录后：

| 项 | README 旧值 | 实际缓存 |
| --- | --- | --- |
| 模型数 | 22 | **18** |
| 免费 / 付费 | 11 / 11 | **9 / 9** |
| 纯文本模型 | 含 `minimax-m3` | `deepai-cyber` / `glm-5.3` / `glyph-cluster` / `solar-mini-4` |
| 默认模型 | `glm-5.3-flash` | 一致 |

差异明细（已写进 README 的说明列表）：旧表里的 `qwen-3.8-flash`、
`minimax-m3.1-flash`、`minimax-m3`、`qwen-3.8-max-prime`、`claude-fable-5-2`
在缓存里都不存在，其中 `claude-fable-5-2` 现为 **`claude-fable-5-1`**（25 Theos）。

README 表格改为按「默认模型 → 免费 → 付费升序」排列，并注明数据是从网关缓存导出的，
不是手抄。别名段也补全为实际的 13 个内置别名（`registry.builtinAliases`）。

### 39.2 后端：`free` 与 `theos_per_second` 不再各算一份

| 端点 | 改动 |
| --- | --- |
| `GET /v1/models` | `deepai` 元信息补 `theos_per_second`（字段一直存在，但没透出） |
| `GET /admin/api/models` | 改为 `adminModelViews()`：补 `free`（与 `Model.Free()` **同源**），并显式带上 `theosPerSecond` |

`free` 由后端下发的意义：前端此前自己算 `!premier && (无价 || 价=0)`，
与 `Model.Free()`（`theosPerRequest` 为 `"0"/"0.0"/"0.00"` 才算免费）**不是同一个口径**。
两套算法在「premier=true 但 theos=0」这类模型上会分叉——控制台说免费、实际计费。

### 39.3 前端：新增「每秒计费」列

- `web/index.html`：`#modelTable` 加 `<th data-sort="theos2">每秒计费</th>`，空态 colspan 8 → 9；
- `web/js/format.js`：新增纯函数 `perSecondText(m)`——空返回 `—`，单档位只显示数值，
  多档位显示 `k: v · k: v`；
- `web/js/app.js`：`renderOverview` 加排序键 `theos2` 与新列；「计费」列改为
  优先读后端 `m.free`，旧响应（没有该字段）才回退本地判断；
- `web/js/i18n.js`：补 `每秒计费 → Per second` 词条（i18n 棘轮测试会检查差集）。

### 39.4 验证

```text
go test ./internal/server/            新增 3 个模型端点测试 + 既有模型刷新测试
  · TestPublicModelsExposeDeepAIMeta         deepai 元信息 10 个字段齐全、free 与 Model.Free() 同源
  · TestAdminModelsExposeFreeAndPerSecond    控制台字段 free / theosPerSecond 必须存在
  · TestAdminModelsFallbackWhenCatalogEmpty  空目录时两端都回落到同一个兜底模型
node scripts/console-smoke.mjs        新增 perSecondText 断言（空 / 单档 / 多档）
go test -count=1 ./...                11 个包全绿
```

> 数据是**网关缓存的快照**，不是实时上游：上游再改目录时，先 `POST /admin/api/models`
> 刷新，再按 §39.1 的方式重新导出即可，不需要改代码。

---

## 40. 号池模型指纹实测：22 个模型逐个过检测器

用 [LM Fingerpoint Detector](https://lm.ikale.io) 的 CLI（`lmfpd`）逐个测号池里
22 个模型实际被路由到哪个模型。完整结果与复现命令见
`docs/model-fingerprint-2026-11-10.md`，这里只记下结论与方法上的两个坑。

### 40.1 先纠正上一轮的目录结论（§39）

§39 的模型表用的是**仓库根目录那份 `deepaicode2api-data.json`**（18 个模型）。
但网关实际读的是 `%APPDATA%\deepaicode2api\deepaicode2api-data.json`，
启动时自动从上游刷新得到 **22 个（11 免费 / 11 付费）**——与更早的 README 一致。

结论：**仓库里那份状态文件是陈旧副本**，不能当目录依据。README §二 已按
线上 `/v1/models` 重新生成（22 行），并加了一句「以线上为准，别拿旧状态文件当目录」。

### 40.2 方法坑：`-p 1` 是硬要求

默认 `-p 3`（并发 3 采样）时，每条样本里的 3 次请求有 2 次返回**空文本**：
`used_outputs=1`、`probability_status=unavailable`，整份结果作废。
改成 `-p 1` 后同样 3 条样本全部有效。第一轮付费批量因此白花了约 100 Theos。

另：`gpt-6-astra` 只能 `-a responses`，其余 `-a chatcompletion`。

### 40.3 结果摘要（22 个）

| 分类 | 数量 | 模型 |
| --- | ---: | --- |
| ✅ 指纹对得上 | 8 | glm-5.3-flash（变体 x）、deepseek-v4.1-flash、qwen-3.8-flash、glm-5.3、kimi-k3、muse-spark-1.3、claude-opus-5-5、muse-wonder |
| ❌ 指纹不符 | 6 | minimax-m3.1-flash→grok-4.5、solar-mini-4→qwen3.8-omni-flash、gpt-6-luna→qwen3.8-omni-flash、minimax-m3→qwen3.8-max-0902、gpt-6.1-sol→mimo-v2.5-pro、gemini-4-argon→qwen3.8-omni-flash |
| ⚠️ 样本不足 | 2 | glyph-cluster（1/3）、claude-fable-5-2（2/3，但名字对得上） |
| 🚫 无有效输出 | 6 | mimo-2.6-flash / mimo-2.6-pro / qwen-3.8-max-prime（超时）；crouton-alpha / deepai-cyber / gpt-6-astra（3/3 空响应） |

> 检测器每次都返回 `evidence.insufficient=true`：结果是「参考库内封闭集合排序」，
> 是参考不是身份证明。别把 0.3 的候选当成定论。

### 40.4 旁证：空响应来自上游的请求体校验

实测期间网关日志出现 **75 次** `[upstream] 请求体被拒，按桌面端形状重试`
（`ChatBodyFallback` 路径）。空响应大概率是这条重试后上游仍回 200-无内容。
会话统计：`total_requests=122`、`failed_requests=50`、`total_cost=0.2764`。

### 40.5 未做

`qwen-3.8-max-prime`（15×3）与 `gpt-6-astra`（50×3）没能拿到有效输出；
两者要再测需额外约 195 Theos，且更可能重复同一失败模式（超时 / 空响应），
建议先查上游请求体校验再重测。

---

## 41. 客户端 0.3.35 复核：协议仍未变，只跟版本号 + 一处模型目录漂移

**触发**：2026-11-11 例行扫描发现官方更新源 `latest.yml` 已发 **0.3.35**
（2026-11-11T03:34Z，releaseNotes 只有一条：修 Linux 上免费每日 Theos 不发放），
而仓库/本机都还停在 0.3.32。本机 Electron 自动更新已在后台下 0.3.35，但停在
77 MB（`updater/pending/temp-*.exe` 不再增长），于是改用官方安装包静默安装。

**客户端更新**（0.3.32 → 0.3.35）：

- `latest.yml` 的 sha512 与下载包逐字节一致，`7z` 解 NSIS → `app-64.7z` →
  `resources/{app,server}.asar`；安装后 `app.asar` 的 `package.json` version = **0.3.35**。
- 安装前关掉桌面端（3 个进程），装完重新拉起；删掉 updater 里过期的半包。

**白盒比对 0.3.32 vs 0.3.35**（`server.asar:/apps/server/dist/binCli-*.mjs`，
14.3 MB 单行 bundle，按 `;` 切段做 `SequenceMatcher` → 26 个变更块，其中 18 个落在
deepai / 赞助位相关代码）：

| 面 | 结论 |
| --- | --- |
| `DEEPAI_LANE_HEADERS` | 一字未变：`{"X-DeepAI-Code-Theos":"4","X-DeepAI-Code-Instance":randomBytes(16).hex}` |
| `X-DeepAI-Code-Machine` | 头名与 `sha256(小写原始 UUID)` 未变；**新增 Linux 分支**（`/etc/machine-id`、`/var/lib/dbus/machine-id`，32 hex 且非全 0） |
| `-Check-In` / `-Beacon` | 字面量与 `v=…;up=…;pi=…` 拼装逻辑未变（`pi` 仍可选） |
| 端点集合 | `/models` `/theos` `/chat/completions` `/responses` `/web_search` `/find_service` `/get_usage_summary` `/gravity-display-proxy` 全部照旧 |
| 新增 `fetchDeepAI` | 仅 Linux：`NODE_USE_ENV_PROXY!==0` 且设了代理变量时，对 `api.deepai.org` / `deepai.org` 用 undici `EnvHttpProxyAgent`；Windows 走默认 `fetch` |
| pi 运行时 | 仍是 1.3.0；新增 `omitRejectedImages`（后端回 “received multimodal data but multimodal processing is not enabled” 后，后续轮把历史图片降级为文本）、纯文本端点把单段 text 数组压平成字符串；`fitCompactedReads` 与 thread identity（`DEEPAI_THREAD_ENV`）都在 |
| 其它 | `find_service` 工具描述改写；赞助卡片新增 `render.mode=iframe` + `sponsoredFrameUrl`（`server.trygravity.{com,ai}`）、`supportsFrames` |

→ **网关的请求头 / 指纹 / 签到 / 端点不需要任何协议改动**，本次只跟版本号。

**线上实测（2026-11-11，真实账号）**：

```text
/models        22 个，默认 glm-5.3-flash
/theos         200 lane_version=4，balance=50，refresh_at=2026-11-12T00:04Z
chat(system)   200 provider=Reka，id=gen-…，cost_details.upstream_inference_completions_cost>0   ← 真分支
chat(user-only) 400 "This request is not one DeepAI Code sends."                                  ← 已知行为，走 ChatBodyFallback
```

**模型目录漂移（唯一一处）**：`deepai-cyber` 上游已从 `65,536 / 8,192` 抬到
**`1,048,576 / 131,072`**（README 旧快照过期）；其余 21 个模型 ctx / maxTokens /
theosPerRequest 与 README 完全一致。

**改动**：

- `internal/deepai/types.go`：`AppVersion` `0.3.32` → **`0.3.35`**（只影响心跳 `v=`）+ 注释跟新。
- `internal/deepai/chatbody.go`：请求体形状的复核版本注释更新。
- `README.md`：§一 客户端版本、§十 两条（车道复核版本、`v=` 现值）、模型表 `deepai-cyber` 行。
- `docs/recon.md`：头部 banner（0.3.35 复核结论 + 0.3.35 的三处变更）、§1/§2/§3/§5.1 标题版本、
  §3 头字面量复核段、§5.1 `pi` 可选段落。
- `docs/changes.md`：本节 + 索引。

**验证**：

```text
go build ./...   OK
go vet ./...     OK
go test ./...    OK
```

**影响**：号池、调度、翻译层零改动。真/替身判据（§3.1）本次**未变**；仍需留意上游
新增的 `render.mode=iframe` 赞助卡片字段 —— 网关不消费赞助位，可忽略。

---

## 42. 0.3.35 协议复查（针对号池）：车道门槛分端点、/review_tool、签到限流

**触发**：客户端升到 0.3.35 后按「协议 / 请求头 / 模型列表」重扫一遍，重点看号池
（14 个启用号，各带专用机器指纹 + 出口代理）会不会被上游改动打坏。

**白盒（0.3.35 `server.asar` → `binCli-*.mjs`）**：请求头字面量、`sha256(小写 UUID)`
指纹、签到 nonce / beacon 形态、`X-Session-Affinity` 校验、409 `session_busy` +
`Retry-After`(1–30s) 重试、目录字段校验规则全部**与 0.3.32 一致**；
唯一新增端点是 `/review_tool`（auto 模式动作审查，15s 超时、失败降级为人工确认）。

**线上实测（2026-11-11，真实账号 + 号池 14 号）**：

- 目录：22 个、默认 `glm-5.3-flash`，字段集与 `deepai.Model` 完全一致（无 `theosPerSecond`），
  11 免费 + 11 付费，`gpt-6-astra` 是唯一 `openai-responses`。
- **车道门槛分端点**（新结论，旧文档「三个端点一视同仁」已过期）：

  | 端点 | 1 | 2 | 3 | 4 | 5 | 6+ | 0 |
  | --- | --- | --- | --- | --- | --- | --- | --- |
  | `/models` | 200 | 200 | 200 | 200 | 426 | 426 | 426 |
  | `/theos` | 200 | 200 | 200 | 200 | 未测 | 426 | 未测 |
  | `/chat/completions`（node UA 真分支） | 未测 | 426 | 426 | 200 | 未测 | 未测 | 未测 |

  → 元数据端点放宽到 1–4，推理仍只认 4。网关恒发 4 不变，号池无需按端点区分。
- 号池体检：13/14 号 `/theos` 200（`lane_version=4`、`balance≈50`、`granted=50`、
  `check_in` 正常下发）；1 号 `unexpected EOF`（代理/网络层，非协议）。
- **签到限流**：短时间连打约 6 次 `/theos` → `403 check_in_refused`
  「restarted its check-in too many times today」；随后 chat **仍走真分支**
  （`provider=InferenceNet`、`up_cost>0`）→ 拒绝签到 ≠ 号坏，恢复看 `refresh_at`。
- 状态文件里的模型缓存仍是 18 个（`deepai-cyber` 还是 `65,536/8,192`）；
  网关启动时 + 每 6h 刷新（`tasks.go`），下次启动即对齐 22 个。

**改动（纯文档，代码零改动）**：

- `docs/recon.md` §2 端点表补 `/review_tool`；§3 头表与「车道门槛」要点改为实测矩阵；
  §5.1 新增「签到被拒 ≠ 号坏了」小节（含探测限速建议）。
- `README.md` 三处 426 表述改为「推理才硬门槛，元数据 1–4」（§一 协议头表、§八 部署提示、§十 限制）。
- `docs/changes.md`：本节 + 索引。

**验证**：仅文档改动，`sh scripts/lint.sh` 仍全绿（gofmt / vet / test 未触碰代码）。

---

## 43. 体检三连：gateway 单测、前端拆分与死代码、工具链升级 + 连接预热

**触发**：2026-11-11 全仓扫描（deadcode / 覆盖率 / env 与文档漂移 / i18n 词条
使用率），按优先级 P1 → P2 → P3 落地。含一次实测修正：`govulncheck` 在 go1.25
上报了一条**可达**的 stdlib 漏洞，因此工具链直接落到 1.26.11。

### 43.1 P1：`internal/gateway` 补单测 + 文档/配置纠偏

- 新包 `gateway_test.go`：该包此前没有测试文件（靠 `internal/server` 间接覆盖）。
  补齐整型/浮点/长度的宽松转换、Responses 工具选择与输出、Anthropic 图片与文档
  data URL 的全部边角分支，并断言「非法输入返回零值、不 panic」。
  覆盖：`ConvertResponsesToolChoice` / `AnthropicDocumentDataURL` 0% → 100%，
  `ResponsesToolOutput` 28.6% → 100%，`IntOf`/`FloatOf`/`LenOfString` → 100%。
- README 数字与实测对齐：badge `260 → 333 用例`；正文「315 用例 / 8.8k 行 / 9 包」
  → 「333 用例（含 44 子用例）/ 9.8k 行 / 10 包」。
- `.env.example` 补齐 10 个真实可配项（HOST/PORT/DATA_PATH/DATA_DIR/ENV_FILE、
  API_ORIGIN/API_KEY/API_KEY_NAME、T3CODE_HOME/DEEPAI_CODE_HOME），并标注
  `DEEPAI_REGISTER_PROXY_POOL` 是兼容别名；本机 `.env` 清掉 7 个孤儿键
  （`TEMPMAIL_*` / `DEEPAI_REGISTER_*` / `DEEPAI2API_GATEWAY`，代码与文档零引用）。

### 43.2 P2：前端拆三块 + 死代码/死词条

- `app.js` 514 → 283 行，只留装配与取数；新增 `overview.js`（KPI/模型表/health）、
  `oauth.js`（登录面板 + 粘贴回调）、`settings-form.js`（设置保存）。
  两块子面板用 `init*({refresh})` 回调注入，避免与 `app.js` 循环导入。
- `console-smoke.mjs` 补 4 条断言（`renderOverview` 空/带数据、`loadHealth`、
  `init*` 可重复装配）——拆模块最怕「文件都在但没人调用」。
- i18n 字典 134 → **118 条**：删掉 16 条死词条（注册/邮箱链路、机器指纹库、
  代理池输入框、性能模板等下线功能残留）。新增**反向棘轮**
  `TestConsoleI18nNoDeadEntries`：词条必须在源码里真的被用到。判定时会先剥掉
  JS/HTML 注释 —— 第一版没剥，`阶段`（只出现在一句注释里）被误判成「还在用」。
- 死代码（`deadcode` 实测，全仓库零引用）：删 `Store.NextAccount`、`Usage.Add`、
  `ProxyPool.All`。`Store.Logs` / `SaveCounters` / `logStore.tail` 与
  `Client.CheckIn` / `Lane` / `EnvInt` / `waitQuotaReprobe` 属测试用访问器，保留
  （`deadcode` 默认忽略 `_test.go`，所以它们会一直出现在报告里）。

### 43.3 P3：工具链 1.24 → 1.26.11 + 连接预热

- `go.mod` `go 1.24` → **`go 1.26.11`**（patch 级：CI 的 setup-go 按 go-version-file
  装的就是这个补丁版）；`Dockerfile` builder 换 `golang:1.26-alpine`；README badge 同步。
- CI 的 `govulncheck` pin 从 v1.1.4（1.24 时代）→ **v1.9.0**，注释改成讲清「为什么
  是 1.26.11 而不是 1.26」。
- **为什么必须升到 1.26.11**：go1.25 实测命中 `GO-2026-6859`
  「HTTP/1 server crash due to MaxBytesReader header race in net/http」，
  且 trace 落到 `main.go`（`ListenAndServe`/`Shutdown`）与 `internal/server`，
  属**可达**漏洞；`Fixed in: net/http@go1.26.11`。升级后重跑输出
  `No vulnerabilities found.`。
- 连接预热 `Server.WarmUpstream`（`internal/server/warmup.go`）：按号池里实际用到的
  出口各打一次 `/models`（免鉴权、不烧 Theos），把 DNS/TCP/TLS（含代理 CONNECT）
  提前做完；4 并发、单出口 10s、总预算 60s，在 `tasks.go` 的 background goroutine
  里跑，失败静默。目标是把首个推理请求的整段握手从关键路径上拿掉
  （r2 压测冷启动 p90 4129ms → 热连接约 2000ms）。
  单测覆盖「同一出口只预热一次」「空号池不发请求」「上游 500 静默返回 0」。

**验证**：

```text
go build ./...                       OK（go1.26.11）
go vet ./...                         OK
go test ./...                        10 个包全绿
gofmt -l .                           无输出
node scripts/console-smoke.mjs       12 条断言全过
govulncheck@v1.9.0                   No vulnerabilities found.
sh scripts/lint.sh                   exit 0
```

---

## 44. 补测两个模型指纹：`qwen-3.8-max-prime` / `gpt-6-astra`

**触发**：§40.5 留的尾巴（两个模型拿不到有效输出，估需 195 Theos）。本次用号池账号
直接打上游、同账号跑免费模型做对照，不用外部检测器。

**结果**（详见 `docs/model-fingerprint-2026-11-10.md` 文末）：

| 模型 | 样本 | `id` | 回显 | `provider` | cost | 输出 |
| --- | ---: | --- | --- | --- | --- | --- |
| `qwen-3.8-max-prime` | 4 | `gen_<ULID>` | `alibaba/qwen3.8-max-prime` | 缺失 | 0 | ✅ |
| `gpt-6-astra` | 4 | `gen_<ULID>` | `openai/gpt-6-astra` | 缺失 | 0 | ✅ |
| `glm-5.3-flash`（同账号对照） | 2 | `gen-<epoch>-<rand>` | `z-ai/glm-5.3-flash` | `InferenceNet`/`Friendli` | 7e-6 / 3.2e-5 | ✅ |

1. 两个模型**都能正常出结果**，§40.5 记的"超时 / 3/3 空响应"是**检测器侧**问题
   （并发 3 采样 + 不支持 responses 端点），不是上游不可用。
2. 按 §3.1 判据两者都落在**替身分支**；同账号下 `glm-5.3-flash` 是干净的真分支，
   所以差异是模型级路由，不是账号/代理异常。
3. **替身现在会回显真实 vendor slug**（`alibaba/…` / `openai/…`），不再是
   `reflection/beam` —— §3.1 里"不回显 slug"这条**辅助**判据对这两个模型失效。
4. **目录价 ≠ 实扣**：`qwen` 价签 15、`astra` 价签 50，实测每个样本只扣
   **0.03–0.07 Theos**（对照组的 `upstream_inference_cost` 也只有 1e-5 美元级）。
   拿 `theosPerRequest` 估算号池成本会高估两个数量级。
5. 新发现 `/responses` 的**请求体形状**要求：`system` 放在 `input[0]`（不是顶层
   `instructions`）、user content 是 `[{type:"input_text",text}]` 数组、带
   `store:false` 与 `prompt_cache_key`；缺了直接 400
   `This request is not one DeepAI Code sends.`。

**改动**：只改文档（`docs/model-fingerprint-2026-11-10.md` 补测小节 + 原表两行标注、
本节与索引）。代码零改动 —— 网关的 responses 透传由调用方给 body，不构造它。

**成本**：本批共约 0.5 Theos（远低于按价签估的 195）。

---

## 45. 号池健康度显示修复：`status=error` 不再永久污染徽章

**现象**：控制台顶部胶囊显示 **1/16 就绪**，但离线直读状态文件、自己打上游的探测
结果是 **15/16 出过 200**；升级前后都是这个数，不是升级打坏的。

**根因**（三处叠加）：

1. `ready = enabled && status != "error"`（`store.AccountCounts`、`server.handleHealth`），
   而 `status` 是**持久化字段**，只有「网关自己跑通一次推理」或「一次成功 /theos」
   才会写回 `ok`；失败则写 `error` 且永不清除。
2. 失败来源是**网关自身流量**：外部订阅监控每 300s 打一次，上游概率性 EOF /
   `Too many requests today` 就把那个号标成 `error`。离线探测脚本直读状态文件、
   自打上游、**不回写 store**，所以实测正常而徽章纹丝不动。
3. 同一行判断还把 `blocked`（上游策略拦截，不会自愈）算成**就绪**，方向反了。

补充事实：`error` **不影响调度**（`pool.Scheduler.Pick` 只跳过 `blocked`），
只影响展示与排序 —— 号池一直在用。

**修复**（展示口径与记账口径一起改）：

| 改动 | 位置 |
| --- | --- |
| `Account.LastErrAt`（Unix 秒）：`error` 带时间戳，健康度按新鲜度判定 | `internal/deepai/types.go` |
| `Transient(err)`：EOF / reset / 超时 / 429 频控 / 5xx / 409 `session_busy` / `check_in_refused` 判为瞬时；401、`social_login_required`，以及**今日额度到顶**（`Too many requests today`，即使以 429 返回）不判瞬时 | `internal/deepai/health.go` |
| `AccountReady(a, now)` / `AccountDegraded(a, now)`：停用不算；`blocked` 不算；`error` 只在 `ErrorStaleAfter`（10 分钟）内不算就绪 | 同上 |
| `MarkAccountFailure(id, err)`：瞬时失败只累加 `failures` / `last_err`，**不动 status**；所有失败调用点改用它（7 处） | `internal/store/store_accounts.go`、`internal/server/{extensions,upstream}.go` |
| `SetAccountAllowance` 同样按瞬时判定决定是否标 `error`（探测失败常是成片出口抖动） | `internal/store/store_accounts.go` |
| `RecoverStaleErrors(now)`：后台 30s 巡检把「超过 10 分钟没再失败」的 `error` 改回 `ok`（保留 `last_err` 作历史），日志 `[decay] N 个账号…` | `internal/store`、`tasks.go` |
| `/health` 新增 `degraded`（错误仍新鲜）与 `blocked` 计数；胶囊在 `degraded>0` 时显示「账号 N/M 就绪 · K 个近期失败 · 模型 X」 | `internal/server/server.go`、`web/js/overview.js` + i18n |
| 顺手修正 `LockAccountProbe` 测试里「标 error 会把好号踢出调度」的错误注释（调度器只跳过 blocked） | `internal/store/store_accounts_test.go` |

老状态文件没有 `last_err_at`（= 0），按「历史噪声」处理 → 升级后第一次巡检就把
历史 `error` 号自愈成 `ok`，随后由真实流量按需重新标记。

**为什么是 10 分钟**：上游这类失败按次、成片出现，监控与真实流量通常 5 分钟内就会
再打一次；超过 10 分钟没再失败，说明是历史噪声。`degraded` 仍会把「刚刚 429 过」的号
如实标出来，不会把两种状态混成「1/16」。

**验证**：

```text
go test ./internal/deepai    TestTransient(16 例) / TestAccountReady(8 例) / TestAccountDegraded
go test ./internal/store     MarkAccountFailure 瞬时 vs 账号级 / RecoverStaleErrors / AccountCounts / SetAccountAllowance
go test ./internal/server    TestHealthReadySemantics（accounts=5 ready=2 degraded=1 blocked=1）
go test ./...                10 个包全绿
node scripts/console-smoke.mjs 12 条断言全过
sh scripts/lint.sh           exit 0
```

**影响**：调度、号池轮询、容量退避零改动；只改「失败怎么写进状态」与「健康度怎么算」。
现场那 2 个当天额度打满的号（`zhuhaoyi181` / `ming49798`）会显示为 `degraded`
且不计入 `ready`，明日额度重置后随一次成功 `/theos` 自动恢复。

---

## 46. 修「操作」菜单点了没反应 + 管理口令改成独立登录页

### 46.1 行内操作菜单点了没反应（`menuEl` 未声明）

**现象**：账号池每行的「⋯ / 操作」按钮点了毫无反应，账号治理（补绑代理、
分配机器指纹、改分组、删除…）全都没法用。

**根因**：`dom.js` 里 `closeMenu()` / `openMenu()` 都在读写 `menuEl`，但**没有声明**它。
拆模块前这个变量在单文件 `<script>` 的 IIFE 闭包里，搬到 `dom.js` 时漏了。
ES module 是严格模式，`openMenu()` 第一行 `closeMenu()` 就抛 `ReferenceError`，
于是菜单根本来不及挂到 `body` 上——表现就是「点了没反应」，控制台却看不到任何
可见错误（异常只在 console 里）。

**修复**：`dom.js` 补 `let menuEl = null;`（带注释说明为什么必须是模块级声明）。

**回归防线**：`scripts/console-smoke.mjs` 新增「行内菜单 open / close」断言
（挂载到 body、`closeMenu` 能清掉、重复关闭幂等）。这类「文件都在、语法都对、
点一下就炸」的断裂正是冒烟脚本存在的理由。

### 46.2 管理口令改成独立登录页

**动机**：顶栏常驻一个口令输入框 —— 口令在屏幕上一直可见，长驻的控制台页等于
把门虚掩着；截图/录屏/旁人一眼就能看到有没有设口令、输到第几位。

**改动**：

| 位置 | 内容 |
| --- | --- |
| `web/login.html`（新） | 独立登录页：品牌头 + 口令表单 + 错误行 + 说明；复用 `style.css`（新增 `.login-*` 段），文案放 `data-msg-*` 供脚本读取（登录页不进 i18n 字典棘轮） |
| `web/js/login.js`（新） | 提交 → `POST /admin/api/auth` 校验 → 写 `sessionStorage` → `location.replace("/")`；已登录过则先验一次，失败自动清掉；主题跟随控制台 |
| `web/index.html` | 顶栏移除口令输入框与「记住」按钮，换成「退出」（清口令 → `/login`） |
| `web/js/api.js` | 口令统一读写 `sessionStorage`（`pw` / `setPw` / `clearPw`）；**401 自动清口令并跳 `/login`** |
| `web/js/app.js` | 启动加登录门：`/health` 的 `admin_required` 为真且本页无口令 → 跳 `/login`；未设口令则本机直通 |
| `web/js/settings-form.js` | 改了管理口令后同步更新 sessionStorage 里那份（否则下一次请求立刻 401） |
| `server.go` | 新增 `GET /login` 路由与 `POST /admin/api/auth`；`/health` 增加 `admin_required` |
| `auth.go` | 新增 `handleAdminAuth`：走 `withAdminAuth`，口令错已在中间件拦成 401，成功只回 `{ok:true}`（不回显任何设置项） |
| `web.go` | embed 加 `web/login.html`；抽出 `consoleScriptNames()` 让契约测试能**按页面**校验 id 引用 |

**顺带修掉的**：`TestConsoleScriptIDsExist` 原本把全部脚本合起来对照 `index.html`
校验 id —— 加了登录页之后 `login.js` 的 `#loginPw` 必然"不存在"。改成按脚本所属页面
分组校验（`login.js` → `login.html`，其余 → `index.html`），拆页面时才不会误报。

**验证**：

```text
go test ./internal/server
  TestLoginPageServed / TestLoginScriptServed       登录页与脚本可访问、内容正确
  TestAdminAuthEndpoint                              未设口令 200 / 缺口令 401 / 错口令 401 / 对口令 200，回包只有 {ok:true}
  TestHealthExposesAdminRequired                     /health 的 admin_required 随设置变化
  TestConsoleScriptIDsExist（按页面分组）            11 个脚本、40+ 个 id 引用全部有定义
go test ./...                                       10 个包全绿
node scripts/console-smoke.mjs                      13 条断言（含新增的菜单开合）
sh scripts/lint.sh                                  exit 0
```

---

## 47. 侧边栏底部状态文字溢出修复 + 侧边栏支持收起/展开

### 47.1 溢出：「账号 N/M 就绪 · K 个近期失败 · 模型 X」顶出侧边栏

**现象**：号池出现近期失败（`degraded > 0`）时，侧边栏底部那颗状态胶囊的文案变长，
直接顶出 208px 的侧边栏，压在正文上。

**根因**：全局 `.badge` 是 `display:inline-flex` + `white-space:nowrap` + 固定 `height:20px`
（按钮/表格里的短标签需要这个形状），而底部那颗胶囊的文案是**动态**的：

```text
账号 14/16 就绪            38 字符 → 放得下
账号 14/16 就绪 · 2 个近期失败 · 模型 22    ⇒ 170px+，208px 栏内必然溢出
```

**修复**（只针对侧边栏内的胶囊，不动全局 `.badge`）：

- `.side-foot{min-width:0}`：flex 子项默认 `min-width:auto`，不置 0 时文本不会收缩换行；
- `.side-foot .badge{white-space:normal;height:auto;align-items:flex-start;border-radius:var(--r-sm);overflow-wrap:anywhere}`；
- `.side-foot .badge .dot{margin-top:5px}`：换行后圆点仍与首行对齐；
- `.side-meta{overflow-wrap:anywhere}`：64 位机器指纹是超长单 token，原来只 `word-break`。

### 47.2 侧边栏收起 / 展开

- 品牌行右侧新增 `#sideToggle`（箭头按钮，`aria-expanded` 同步）。
- 收起态只做一件事：`body.side-collapsed{--sidebar:var(--sidebar-min)}` —— 因为
  `.sidebar{width:var(--sidebar)}` 与 `.main{margin-left:var(--sidebar)}` 都读同一个变量，
  JS 不需要量尺寸、也不需要在两处同步宽度；两处各自 `transition` 220ms。
- 收起后：品牌文字、导航文字、`side-meta` 隐藏，导航只剩居中图标（按钮带 `title`，
  悬停仍能认路）；底部状态胶囊变成一颗 **26px 状态灯**，颜色仍是 ok/warn/err，
  完整文案进 `title`（`loadHealth` 里设置），信息不丢。
- 选择记在 `localStorage["deepaicode2api_sidebar"]`，刷新/重开保持。
- **移动端不参与**：`<=900px` 时侧边栏是抽屉，宽度固定 `--sidebar-full`、开关按钮隐藏，
  收起态的规则在媒体查询里被覆盖（否则手机上会缩成 64px 的窄条）。
- 导航文案从裸文本改成 `<span class="nav-text">`，收起时才能整体隐藏（`display:none`
  不能作用于匿名文本节点）。

**验证**：

```text
node scripts/console-smoke.mjs      14 条断言（新增「侧边栏收起 / 展开」：接线/切态/回切）
go test ./internal/server           契约测试全绿（按页面校验 id、i18n 正反向棘轮）
go test ./...                       10 个包全绿
sh scripts/lint.sh                  exit 0
真实实例抽查："/" 返回 5 个 .nav-text 与 #sideToggle；style.css 含 --sidebar-min 与
            body.side-collapsed 规则。
```

## 48. 控制台拆分遗留的裸 `refresh()`（4 处收尾全报「失败：refresh is not a function」）

**现象**：账号启停/删除、密钥启停/删除、「查询上游配额」、「改绑分组」成功后，
toast 显示 `失败：refresh is not a function`；数据其实已经改成，只是列表不刷新。

**根因**：`app.js` 拆分时把刷新函数留在装配层，下层模块靠 `setReload(fn)` 注入，
但 4 处收尾仍写着裸 `refresh()`：

| 位置 | 场景 |
| --- | --- |
| `js/actions.js` `accountOp` | 账号启停 / 删除 / 手工加号 |
| `js/actions.js` `keyOp` | 密钥启停 / 删除 |
| `js/actions.js` `finishCheckin` | 配额查询成功后的重拉 |
| `js/render/keys.js` keyMenu「改绑分组」 | 同款 |

模块作用域里没有这个绑定，等到「操作成功后的收尾」才抛 `TypeError`；更糟的是它被
同一个 `.catch` 接住，错误被包装成接口失败——看起来像后端挂了。

**修复**：

- 4 处一律改成注入的 `reload()`，默认值从空箭头函数改成**可调用的空实现**
  （`function(){ return Promise.resolve(); }`），注入前调用也不会炸；
- `setReload` 忽略非函数入参，避免装配顺序变化把 `reload` 设成 `undefined`；
- 「改绑分组」原本在 `render/keys.js` 里自己发请求，抽成 `actions.keySetGroup(key, group)`：
  render 层只绘制、变更类操作统一走 actions（才能共用同一条 reload 收尾）。

**回归防线**（这两条以前都不存在，所以拆分时漏改也没人发现）：

- `internal/server/console_reload_test.go`：静态契约——`actions.js` 与 `render/*.js`
  不允许出现裸 `refresh(`，并断言注入点仍在（`setReload` 导出 + 默认空实现）。
  扫描前先剥注释（复用 `console_i18n_test.go` 的 `stripJSComments`，并让它保留块注释
  里的换行，行号才报得准）；
- `scripts/console-smoke.mjs`：新增 `checkAsync` 与「变更操作收尾走注入的 reload」——
  真跑 4 条操作链路，断言注入的 reload 被调 4 次、toast 里没有 `失败：` 前缀。
  对拆分前的代码复跑会直接报 `refresh is not defined`（已实测）。

**验证**：

```text
node scripts/console-smoke.mjs      15 条断言全 PASS（新增的那条对旧代码 FAIL）
go test ./internal/server -run TestConsole      契约全绿
go test ./... -count=1                          10 个包全绿
gofmt -l . && go vet ./...                      无输出
```

## 49. 一轮按优先级落地的后端 / 前端加固（P0 → P1 → P2）

扫描后发现的问题按「正确性 → 治理 → 工程卫生」排序，逐项实现并补回归测试。
这里的每条都写了**根因**，因为其中三条是「看起来没问题、但复杂度随号池规模爆炸」
的类型——它们不会在 5 个号的测试里暴露。

### 49.1 后端 P0-1：选号热路径是 O(n²) 深拷贝 + 每轮写锁

**根因**：`pickFreeAccount` 写成

```go
for i, n := 0, len(s.Store.Accounts()); i < n; i++ {  // 每轮都全池 Clone
    a := s.Store.NextAccountGroup(group)              // 写锁 + Pick 全扫 + 再 Clone
```

`Accounts()` 对每个账号做深拷贝，`NextAccountGroup` 取的是写锁。653 个号、池子偏忙时
一次选号可能产生 **n×n ≈ 42 万次深拷贝**，并且所有并发请求在 store 写锁上串行化。
`openModel` 还为了 `len()` 多拷了一次全池。

**改法**：

- `Scheduler.rotate` 改成 `atomic.Int64`，选号可以用**读锁**并发进行；
- 新增 `Scheduler.PickFiltered` / `HasEligible`（纯谓词过滤，供「并发槽位已满」排除）；
- 新增 `Store.PickAccount(group, skip)`：一次读锁 + 一次扫描，**只克隆被选中的账号**，
  并同时返回「池里是否有可调度账号」，用于区分「全忙」与「没号」；
- `pickFreeAccount` 用纯谓词 `accountBusy` 排除忙号，`acquireAccount` 只兜住
  「检查与占用之间被抢走」的窄竞态；
- `hasFreeAccount` 改用 `Store.AnySchedulable`（零拷贝）。

**验证**：`internal/server/pick_test.go` 用 `testing.AllocsPerRun` 守住「500 个号时
单次选号分配数 ≤ 40」——旧实现至少 500 次分配，这条会直接失败。

```text
BenchmarkPick500-12          111.5 ns/op  0 B/op  0 allocs/op
BenchmarkPickFiltered500-12   84.5 ns/op  0 B/op  0 allocs/op
BenchmarkPickAccount600-12   282.5 ns/op  528 B/op  2 allocs/op
```

### 49.2 后端 P0-2：流式响应没有 idle 超时

**根因**：账号槽位随 `resp.Body.Close()` 归还，而流式可能持续数分钟；上游发完首帧
后挂住时，旧实现只依赖 `r.Context()`——客户端不断开就一直等，槽位与连接被无限占用。
`http.Server` 也只有 `ReadHeaderTimeout`。

**改法**：`idleTimeoutBody`（`internal/server/idle.go`）在 120s 无新数据时关闭底层
Body（这正是 transport 打断在途读取的方式：并发 Close + Read 是它支持的模式）。
包在 `releaseBody` 外面，**在 `openModel` 一处统一挂载**，所有流式/非流式调用点
自动受益、不会漏。另给 `http.Server` 补 `ReadTimeout` / `IdleTimeout`。

### 49.3 后端 P0-3：优雅关闭不等后台任务

**根因**：`main` 关掉监听后直接 `Save()`，而配额复探 / 入池探活 / 按需复探都是
「先写内存再 Save」的 goroutine（挂在 `quotaWG`），生产路径上从未 `Wait`。
退避睡眠最长 12 分钟，硬等会拖住退出。

**改法**：

- `Server.Close(ctx)`：关闭 `stopCh` → 等 `quotaWG`（以 ctx 为上限）→ 关闭各出口空闲连接；
- 后台退避改用 `sleepOrStop`，可被关停信号打断；
- `probeAsync` / 以及此前裸 `go s.probeAccounts(...)` 的管理动作统一登记到 `quotaWG`；
- `main` 的顺序改为 `Shutdown → srv.Close → Save`（反过来会让最后那次 Save 被后台
  的旧快照覆盖）。

### 49.4 后端 P1：按密钥限流、口令防爆破、可观测性

| 项 | 根因 | 落地 |
| --- | --- | --- |
| 数据面限流 | `withAuth` 只校验密钥有效性，一个失控脚本能打满号池 | 令牌桶（RPS + burst）+ 并发闸门，**按密钥指纹隔离**；`429 + Retry-After + rate_limit_error`；默认 0 = 不限，升级不改变既有行为 |
| 管理口令爆破 | 失败没有代价，`/login` 是公开页面 | 同源 IP 连续失败 5 次后指数退避（1s→30s），退避期内正确口令也回 429；成功即清零 |
| request id | CORS 早已 expose `X-Request-Id`，但从未生成 | 中间件生成 16 位 hex、回显合法传入值、写入上下文与响应头 |
| 访问日志 | 53 处中文自由文本，只有启动那一行是 JSON | `internal/logx` 把标准库 log 接管成 JSONL（既有调用一行不改），访问日志带 `request_id/status/ms/bytes/remote` |
| 指标 | 只有 `/health` 快照 | `/metrics`（Prometheus 文本）：状态码分类计数、耗时 sum+count、在途请求、号池分布、goroutine/堆；挂 `withAdminAuth` |

`poolSnapshot()` 把 `/health` 与 `/metrics` 的口径收成一处，避免两个端点给出互相矛盾的数。

### 49.5 后端 P2：gzip、基准测试、部署约束

- **gzip**：只压 JSON / text / JS / SVG，**跳过 SSE**（逐帧 flush 语义不能破坏），
  且 1xx/204/304 不套 `Content-Encoding`（304 撞 ETag 时最容易踩）；
- **bench**：`internal/pool` 与 `internal/store` 各加 3 个基准（`make bench`），
  把选号、分页、全忙判定的复杂度变成可观测的数字；
- **文档**：README §十 增补「单实例部署」「限流参数」「口令退避」「日志是 JSONL」四条约束。

### 49.6 前端 P0：CSP 与资源 ETag

**根因**：控制台承载管理口令，但此前没有任何 `Content-Security-Policy`；同时
`serveConsoleAsset` 一律 `Cache-Control: no-store`，首屏 17 个 ES module 每次全量重下。

**改法**：

- 严格 CSP（`default-src 'self'` … `frame-ancestors 'none'`），并**移除全部 30 处内联
  `style` 属性**（统一改用具名工具类）；主题启动脚本做成 `/js/theme-boot.js`
  （head 中的同步外部脚本，既赶在首帧前生效，又不违反 `script-src 'self'`）；
- 资源按内容哈希发 `ETag` + `no-cache`，命中 `If-None-Match` 回 304；HTML 仍 `no-store`
  （保证升级后立刻换壳）。

**验证**：`TestConsoleAssetsServed` 断言每个资源带 CSP/ETag/正确的 Cache-Control，
新增 `TestConsoleAssetConditionalGET` 覆盖 304 与「ETag 变了就重发」。

### 49.7 前端 P1 / P2

| 项 | 改法 |
| --- | --- |
| 英文界面冒中文错误 | 后端 message 只有中文，而把 80 多条塞进前端字典会触发「死词条」棘轮 → 改用稳定的 `error.type` 分类给英文文案 + HTTP 状态码，原文进 `err.detail`（控制台可见） |
| 筛选 / 页码不落地 | 写进 hash（`#accounts?group=…&page=2`），刷新与分享都能回到同一视图 |
| 输入不防抖 | 250ms debounce（Enter 立即生效），避免每敲一个字打一次分页接口 |
| 后台标签页仍在轮询 | `visibilityState` 感知：后台不轮询，切回来立刻补一次 |
| 请求无超时 | `AbortController`：GET 15s / 其它 90s；超时文案可翻译 |
| GET 去重键过粗 | 去重键改为 `method + path`（含查询串），不同筛选不再共用 promise |
| 主题闪烁（FOUC） | `theme-boot.js` 在首次绘制前写 `data-theme`；图标显隐改用 `.hidden` 类 |
| 可访问性 | 表头补 `scope="col"` / `aria-sort`，toast 改 `role="status" aria-live="polite"`，导航补 `aria-label` |
| JS 无质量门禁 | `scripts/lint.sh` 与 CI 增加 `node --input-type=module --check` 全量语法校验 + 控制台冒烟 |

### 49.8 验证

```text
go build ./... && go vet ./...                 通过
sh scripts/lint.sh                             gofmt / vet / 前端语法 / 冒烟 / 全测通过
go test ./... -count=1                         11 个包全绿（新增 4 个测试文件）
make bench                                     选号 111ns/0 alloc；选号+分页基线已固化
```

### 49.9 有意为之的边界

- **确认框仍用原生 `confirm`/`prompt`**：换成自绘弹窗属于交互重写，与本轮的
  「正确性/治理」目标不同批；原生对话框可键盘操作，不阻塞可用性；
- **不做状态文件再拆分**：`saveTimestampGranularity=300` 已把写放大从「每 30s」
  降到「每 5 分钟（且只有内容变化时）」，继续拆 accounts 会引入迁移风险而收益很小；
- **不支持多副本共享同一 data 路径**：调度/限流/在途计数都在进程内存里，已在
  README 写明；横向扩展需要外置存储，不在当前范围。

## 50. 概览 KPI 重排、账号脱敏、版本与发布（v0.1.0）

本轮是**产品面向**的改动，重点是「运营需要看的数」而非内部指标。三个 KPI 里有两个
（今日 token、缓存命中率）此前**根本没有数据源**，所以先在采集侧补字段，再改展示。

### 50.1 概览 KPI：今日 token 与缓存命中率

| 位置 | 原来 | 现在 |
| --- | --- | --- |
| 第 4 张卡 | `Token 入 / 出` | `总 Token 入 / 出` |
| 第 7 张卡 | `密钥`（数量） | `今日 Token 入 / 出` |
| 第 8 张卡 | `统计自`（时间戳） | `缓存命中率` |

**采集侧改动**（不然前端只能显示 0）：

- `gateway.Usage` 增加 `CachedTokens`，`cachedTokensOf` 兼容三种上游形态：
  `prompt_tokens_details.cached_tokens`（OpenAI）、
  `input_tokens_details.cached_tokens`（Responses）、
  `cache_read_input_tokens`（Anthropic）；
- `store.LogEntry` 增加 `cached_tokens`，`Record` 累加 `TotalCachedTokens`；
- `Stats` 增加 `Day / DayTokensIn / DayTokensOut`：**按本地日期**统计，
  跨天由 `rollStatsDay` 在写入路径清零（不需要定时任务）；读路径 `Stats()`
  也做一次判定，避免重启后把昨天的当日量当成今天。

**展示约定**：命中率 = `total_cached_tokens / total_tokens_in`；没有样本时显示
`—` 而不是 `0.0%`（后者会被误读成「缓存从不命中」）。

### 50.2 账号列表脱敏开关

后端本来就只下发脱敏后的 Key / 代理 / 机器指纹，但**邮箱与用户名**是明文——
远程协助、录屏、截图时仍会暴露。

- 账号列表工具条增加「脱敏显示」复选（偏好存 `localStorage`）；
- 脱敏在**渲染层**做：邮箱 `u***@domain`、其它标识 `j***n`；
- 搜索与筛选仍按**完整值**匹配，打码只影响显示——否则「脱敏后搜不到自己刚导入的号」
  会变成新的支持问题。

### 50.3 版本号与更新检查

- 版本经 `-ldflags -X main.version=` 注入；`Server.Version` + `/health.version`
  让控制台（含登录页可读的公开端点）随时拿到；
- 顶栏显示 `v0.1.0`，是一个静态 `<a href>` 指向 releases 页；
- `GET /admin/api/version`：服务端代查 GitHub 最新 release，**缓存 6 小时**，
  只比 `主.次.修订`；`dev` 构建不比对（否则永远提示有新版本）；
  查询失败返回 `200 + error`，前端只把标题改成「无法检查更新」，不弹错误。

为什么要服务端代查：前端 CSP 是 `connect-src 'self'`，本来就不允许直连
`api.github.com`；而且未认证的 GitHub API 有速率限制，不该让每台机器各自去打。

**安全细节**：跳转目标写死在标记里的静态 `<a href>`，不转发 GitHub 返回的
`html_url`——那属于外部输入，动态拼进导航就是一个 open redirect 面
（首版实现被静态规则拦下，顺势改成了更安全的形态）。

### 50.4 工程整理

- 新增 `scripts/release.sh`：交叉编译 linux/amd64、linux/arm64、windows/amd64、
  darwin/amd64、darwin/arm64 + `checksums.txt`，`--publish` 时调 `gh release create/upload`；
- 清理工作区临时产物：`.ruff_cache/`、`tmpfp.exe`、根目录旧 `deepaicode2api.exe`、
  `bin/` 下的旧二进制（含已移除的 `register`）；**保留**运行期状态
  （`deepaicode2api-data*.json` / logs / stats）、`exports/` 台账与 `.bak` 备份；
- README 增补：概览/账号/顶栏的新行为、`/admin/api/version`、§九·五「版本与发布」。

### 50.5 验证

```text
go build ./... && go vet ./... && gofmt -l .        无输出
go test -race -count=1 ./...                        11 个包全绿
node scripts/console-smoke.mjs                      全部通过
sh scripts/lint.sh                                  Go + 前端语法 + 冒烟全通过
```

新增测试：`TestParseSemver` / `TestVersionNewer` / `TestAdminVersionEndpoint`
（含「第二次命中缓存、不打上游」的断言）/ `TestAdminVersionDevBuild` /
`TestAdminVersionUpstreamFailure` / `TestHealthIncludesVersion`。

## 51. 全量扫描落地：竞态、日志完整性、落盘可观测、前端收口（2026-11-12）

本轮是**扫描驱动**的修正，不是新功能。触发点：对 24.7k 行 Go + 2.5k 行前端做一次
无假设复查（`go vet` / `gofmt` / `-race` / 覆盖率 / 前端冒烟 / 客户端版本），
把「测试全绿但行为仍有洞」的地方挑出来。每条都补了能独立复现的回归测试。

### 51.1 P0：`Store.Keys()` 返回活指针 → 真实数据竞争

`Keys()` 只复制了切片，元素仍是号池里的 `*APIKey`；而 `ValidKey` 在**每个**数据面
请求上都会持写锁累加 `LastUsed / Requests`，控制台的 `safeKeyList` 却在锁外读同一
结构体。用临时用例并发跑 `ValidKey` × `Keys()` 时 `-race` 稳定报
`WARNING: DATA RACE`（写 `store_keys.go:104`，读 `admin.go:230`）。

改法：`Keys()` 逐份拷贝（对齐 `Accounts()` 早已有的 Clone 语义），`AddKey` 的返回值
同样改成副本。回归测试 `TestKeysSnapshotIsRaceFree` 在旧实现下必失败（已实测），
`TestAddKeyReturnsCopy` 锁住「改返回值不得污染号池」。

### 51.2 P1：请求日志的完整性

| 问题 | 影响 | 改法 |
| --- | --- | --- |
| `passthroughResponses` 在「上游回的不是 JSON」分支直接 `return`，没调 `rl.done` | 上游偶发返回 HTML / SSE 时，请求从日志与统计里**彻底消失** | 补记一条 `status=200 + err=上游响应不是合法 JSON（已原样透传）`；`Record` 会把非空 `Err` 计入失败数 |
| `reqLog.done` 可被多条分支各调一次 | 重复计数（吞吐、错误率翻倍） | 增加 `reqLogState{recorded atomic.Bool}`，`done` 幂等；值类型 `reqLog` 靠指针共享标记 |
| `statusOf` 把所有错误记成 502 | 上游 400 / 426 / 429 全部记成 502，`min_status` 筛选与归因失真 | 优先 `errors.As(*deepai.HTTPError)` 取真实状态码 |
| `streamResponsesAsChat` / `streamAnthropic` 的序列化失败分支同样漏记 | 极小概率但同类型 | 一并补 `doneStatus`（状态码由调用方给出） |

回归：`TestReqLogDoneIsIdempotent` / `TestStatusOfUsesUpstreamStatusCode` /
`TestPassthroughNonJSONIsLogged`。

### 51.3 P1：形状回退路径漏了空闲超时

`openModel` 有两条返回 2xx 的路径。主路径包了
`newIdleTimeoutBody(releaseBody(...))`；「请求体被拒 → 按桌面端形状重试」那条只包了
`releaseBody`。上游发完首帧挂住时，账号槽位与连接会挂到客户端断开为止 —— 正是
`idle.go` 注释里描述的那个坑，只漏了一条分支。

改法：抽出 `holdResponseBody`，两条路径共用。回归：
`TestShapeFallbackRetryKeepsIdleTimeoutGuard`（断言回退路径的 Body 是
`*idleTimeoutBody`，且关闭后槽位归零）。

### 51.4 P1：落盘失败静默 + 畸形请求体被当成空请求

- 17 处 `_ = s.Store.Save()`：磁盘满 / Windows 文件被占用时接口仍回 `{"ok":true}`。
  改为两个入口：`persist()`（尽力而为，失败只记 warn：后台探测、登录入池、扩展用量）
  与 `persistOrFail(w)`（用户动作，失败回 500）。管理面 14 处走后者，`_ =` 归零。
- 3 处 `_ = json.NewDecoder(...).Decode(&body)`：畸形 JSON 会被当成空请求继续执行
  （`POST /admin/api/keys` 会建出一把空名字的密钥并回 200）。改为
  `decodeOptionalJSON`：**空体仍合法**（「刷新全部配额」不带参数就是空体），
  畸形 JSON 回 400。

回归：`TestAdminMutationReportsSaveFailure`（把状态文件换成同名目录，让 rename 必然
失败）/ `TestAdminKeysRejectsMalformedJSON`（400 + 不建密钥 + 空体仍 200）。

### 51.5 优化与重构

| 项 | 改法 |
| --- | --- |
| 限流桶表只增不减 | `keyRateLimiter` 增加 `sweepLocked`：超过 512 桶时回收闲置 30 分钟的桶（有在途请求的跳过）。`TestTokenBucketSweepsIdleEntries` |
| 版本查询无合并 | 缓存冷时并发请求合并：`versionInFlight` 通道，只让一个 goroutine 打 GitHub，其余等结果。`TestVersionStatusSingleFlight`（8 并发只命中 1 次） |
| 放弃的登录会话占着监听 | `loginSessions.get` 顺手回收过期兄弟会话（连同回环监听），被请求的那个仍原样返回以保留「expired vs 404」语义。`TestLoginSessionsPrunesExpiredOnGet` |
| 签到阻塞推理不可见 | 不改同步语义（nonce 回传是上游判「真实客户端」的依据，异步会让当次请求落到替身分支），只在阻塞 ≥2s 时记 warn。`checkInSlowWarn` |
| 转换层单包覆盖率 13.3% | 新增 `internal/gateway/convert_test.go`（含块写入器状态机）：**13.3% → 94.3%**。`make cover` 改用 `-coverpkg` 统计跨包调用 |
| 三个推理入口重复「读体 → 解析 → 报错」 | 抽出 `decodeJSONRequest[T]`，协议差异（OpenAI / Anthropic 错误信封）由 fail 回调注入。`TestDecodeJSONRequestEnvelopes` 覆盖 4 个端点 |
| `app.js` 509 行 | 拆 `events.js`（207）/ `theme.js`（84）/ `pager.js`（39），`app.js` 降到 231 行（只剩取数 + URL 状态 + 装配）；依赖回调注入，不 import app.js |
| 死 CSS | 删掉无引用的 `.flex-1` / `.mt-6` |

### 51.6 卫生与文档

- 删除 `bin/cdp_check.mjs`（`scripts/console-e2e.mjs` 的旧副本，缺 try/catch 与 KPI 断言）
  与 `bin/loadtest.py`（已被 `scripts/ab-load.mjs` 取代）；修 `console-e2e.mjs` 里过期的用法注释；
- **本机遗留产物清理（2026-11-12，经确认后执行）**：删除 `exports/`（明文密码台账
  + 脱敏台账）、`deepaicode2api-data.json.pre-harcleanup.bak`（加密旧备份）、
  `deepaicode2api-data-logs.jsonl`（407KB 历史请求日志）、`dist/`（39MB v0.1.0 旧构建）。
  §50.4 里「保留运行期状态 / `exports/` 台账 / `.bak` 备份」的表述作废；
  运行期状态主文件与 stats 保留，`make clean` 现在一并清理 `dist/` 与 `coverage.out`；
- 文档漂移对齐：README 用例数 383 → **542（412 顶层 + 130 子用例）**、测试行数
  11.6k → **12.8k**、「CI 覆盖 11 个包」→ **10 个含测试的包**、`index.html` 288 → 301 行、
  §8 模块清单补 `events.js` / `theme.js` / `pager.js`；§39 的「app.js 509 行」遗留标记为已落地。

### 51.7 验证

```text
go build ./... && go vet ./... && gofmt -l .        无输出
go test -count=1 ./...                              10 个包全绿（542 用例）
go test -race -count=1 ./...                        10 个包全绿
node scripts/console-smoke.mjs                      全部通过
make cover（-coverpkg=./internal/...）               83.2%（原 82.2%）
go test -cover ./internal/gateway/                  gateway 13.3% → 94.3%
```

新增 31 个测试函数（合计 542 个用例 / 130 个子用例）。其中「回归」性质的 4 组
在修复前必失败：`TestKeysSnapshotIsRaceFree` / `TestAddKeyReturnsCopy`（已用旧实现
实测确认 `-race` 报错）、`TestPassthroughNonJSONIsLogged`（旧实现不写日志）、
`TestAdminMutationReportsSaveFailure` 与 `TestAdminKeysRejectsMalformedJSON`
（旧实现回 200）。

## 52. 项目的 pi-lens 配置：从默认值到按本仓裁剪（2026-11-12）

此前本仓**没有**任何 pi-lens 配置（`effective_config` 显示 `documents: []`、
provenance 全 0），机器上也没有 `~/.pi-lens/config.json` —— 等于全程跑默认值。
默认值对「零工具链 + 中文长文 + 路由装配风格」的仓并不合适，表现为每会话几十条
`tool-not-found` 与几条稳定的误报。

### 52.1 证据先行

`~/.pi-lens/extension.log` 里与本仓相关的 spawn 失败统计（近两天）：

| 次数 | 工具 | 说明 |
| ---: | --- | --- |
| 16 | `typos` | 未安装；中文文档还会大量误报 |
| 10 + 6 + 4 | `vscode-html/css-language-server` | 未安装 |
| 6 + 5 | `bash-language-server` | 未安装 |
| 5 + 5 | `yamllint` / `yaml-language-server` | 未安装 |
| 5 + 5 | `pyright` / `basedpyright-langserver` | 未安装 |
| 2 + 1 | `docker-langserver` / `jscpd` | 未安装 |

同时把 `lens_diagnostics mode=full` 跑了一遍前端与 5 个热点 Go 文件，得到
4 条真实可改 + 5 条结构性误报（见下）。

### 52.2 改了什么

新增仓库根 `.pi-lens.json`（项目级，可提交、不参与编译）：

| 段 | 内容 | 理由 |
| --- | --- | --- |
| `ignore` | `dist/`、`bin/`、`exports/`、`coverage.out`、`*.out`、`.env*`、`deepaicode2api-data*.json*`、`*.jsonl`、`*.bak` | 运行期状态含上游 Key，本就不该进任何索引 |
| `lsp.disabledServers` | `bash` / `css` / `docker` / `html` / `python` / `typos` / `yaml` | 对应 server 均未安装；关掉后相关文件显示 *unconfirmed* 而不是每会话刷失败 |
| `dispatch.disabledRunners` | `vale` / `markdownlint` / `yamllint` | 中文长文与两份 YAML 不在本仓质量门禁内 |
| `rules.high-complexity` | 阈值 15 → 25；`internal/server/upstream.go` 单独豁免 | `openModel` 是已知调度状态机（cc 39），拆分需单独设计 |
| `rules.high-fan-out` | 20 → 30 | 默认值对路由/装配风格偏严 |
| `rules.nil-map-assignment` | disable | 规则不追踪 `&m` 写入：`var m map[…]` + `json.Unmarshal(raw, &m)` 合法 |
| `rules.waitgroup-done-scope` | disable | 规则把 `ctx.Done()` 也当成 `WaitGroup.Done()`（误报 3 处） |
| `rules.no-console-except-error-js` | `ignorePaths: js/api.js` | 该处 `console.warn` 是刻意保留的英文界面排障输出 |
| `rules.no-array-reverse-mutation-js` | `ignorePaths: render/logs.js` | 写法是 `list.slice().reverse()`，规则看不穿 `.slice()` |

顺带**改代码**而不是豁免的两处（真实问题）：

- `format.js`：`isFinite(v)` → `Number.isFinite(v)`（v 已是 Number，全局版会先做类型转换）；
- `overview.js`：`!!m.free` → `Boolean(m.free)`（等价且不触发 `no-extra-boolean-cast-js`）；
- `upstream.go`：上游错误体的三行手写响应头 → 复用 `writeRawJSON`，顺带消掉
  Semgrep 的 XSS 误报（网关写的是 JSON / SSE，不是 HTML，`html/template` 不适用）。

### 52.3 验证

```text
effective_config path=internal/server/upstream.go
  documents=[project .pi-lens.json] · provenanceCounts.project=29 · recordCounts={}
  7 个 server 显示 disabled-by-config（含 key 与 tier）

lens_diagnostics path=internal/server/web/js scope=paths mode=full source=lsp
  0 findings（2 条按规则策略隐藏，均已在 docs/pi-lens.md 记录原因）

lens_diagnostics path=internal/server/upstream.go … 5 个热点文件
  0 findings（4 条按规则策略隐藏）
```

`recordCounts` 第一版有 2 条 `PILENS_CFG_0005`：`typos` 与 `jscpd` 不是合法的
`dispatch.disabledRunners` id（`typos` 属于 LSP 辅助 server，已由
`lsp.disabledServers` 关闭）。修正后为空 —— 这类告警是配置是否真的被接受的
第一判据。

配置说明与「以后怎么打开」写在 `docs/pi-lens.md`。

## 53. 日志页报「日志加载失败：renderLogs is not defined」（v0.1.1 拆 app.js 漏了一条 import）

**现象**：打开控制台「请求日志」页（或点「刷新」/ 换筛选 / 点「重新加载」），
toast 显示 `日志加载失败：renderLogs is not defined`，日志表永远是空的。
而浏览器 devtools 里 `/admin/api/logs` 是 200，响应体里日志正常。

**根因**：`app.js` 拆分时 `refreshLogs()` 留在装配层（取数 + 渲染都在这一处），
但文件顶部的 import 块只有 `renderAccounts` / `renderKeys` / `renderSettings`，
漏了 `./render/logs.js`。ES module 的未绑定标识符**不在加载时**报错，只在执行到
`renderLogs(state.logs)` 这一行时抛 `ReferenceError`；它又被同一个 `.catch`
接住，于是被改写成了「日志加载失败」——提示指向接口，实际接口没问题。

所以「取数正常、渲染炸掉」被压成一句话，排查成本全落在读代码上。同类事故
在本仓已经发生过一次（提交 `ca0ed43`：4 处收尾还在调裸 `refresh()`）。

**修复**：

| 位置 | 改动 |
| --- | --- |
| `js/app.js` | 补 `import { renderLogs } from './render/logs.js'` |
| `js/app.js` `refreshLogs` | 取数与渲染**分开收尾**：接口失败才报「日志加载失败：」，渲染失败报「日志渲染失败：」并单独 try/catch；接口失败时不再拿空数组覆盖旧表格 |
| `js/i18n.js` | 补英文词条「日志渲染失败：」→ `Log render failed: `（否则英文界面漏翻） |

**回归防线**（这两条以前都没有，所以漏改没人发现）：

- `console_imports_test.go`（新增）：静态契约——某文件调用了别的模块导出的
  函数就**必须** import 它；import 的路径要能解析到内嵌资源、导出名要真的存在。
  对旧代码复跑直接 FAIL：`web/js/app.js:172 调用了 renderLogs()，但本文件没有 import 它`。
- `scripts/console-smoke.mjs`：加载 `app.js` 前先把 `#tab-logs` 标成当前页，
  让 `start()` 真的跑一遍 `refreshLogs()`，再断言 toast 不含「日志加载失败 /
  日志渲染失败」且日志表写进了行。旧代码复跑：`FAIL ... renderLogs is not defined`。
  （此前冒烟只 import `app.js` 不点任何页签，这条链路一次都没走过。）

**验证**：

```text
node scripts/console-smoke.mjs      16 条全放行（新增 1 条）
go test ./...                       13 个包全绿（新增 2 条契约）
sh scripts/lint.sh                  exit 0（gofmt / vet / JS 语法 / 冒烟 / 测试）
```

## 54. 二次全扫（v0.1.1 之后）：7 处遗留，5 类根因

**扫描方式**：`go test -race -cover`、`go vet`、`scripts/lint.sh`、逐文件通读前端（1920 行
ES module）、后端按「未覆盖函数 / 静默错误 / 未受约束的资源增长」定向抽查。以下是
**读代码能证明 + 复跑能证伪**的遗留项，按影响排序。

### 54.1 前端：英文界面空态整段是中文（真实用户可见）

| 位置 | 现象 |
| --- | --- |
| `dom.js` `emptyRow` | 「暂无日志 / 暂无密钥 / 暂无账号 / 无」在 EN 界面原样显示中文 |

根因：`emptyRow` 直接用 `mk(td, text)` 写文本，而空态分支渲染完就 `return`，
**不经过** `translateDom`（日志页连 `translateDom` 都没调过；账号/密钥/模型靠
`refresh()` 末尾那次全局 `translateDom(document.body)` 兜住，日志页没有这一步）。
而空态恰恰是新装用户的默认视图。

修复：`emptyRow` 内部过 `t()`——空态是唯一不经过 `translateDom` 的单元格，源头修一处
四个表全好。冒烟新增「英文界面空态不落中文」：切 `en` 后逐个空渲染四张表，断言无 CJK。

### 54.2 前端：设置里的显式 `0` 被吞成空框

`render/settings.js` 的注释写着「0 是合法值（关闭），不能用 `|| ""` 吞掉」，代码却
还在用 `s.rate_limit_rps || ""`：把限流关掉（存成 0）后重开控制台，输入框是空的，
看着像「没设置」。改为 `num()`（只把 `undefined/null` 显示成空串）并覆盖三个限流字段。

### 54.3 前端：账号接口失败会把整个概览拖成空白

`refresh()` 原来是 `Promise.all([overview, loadAccounts])`：账号页一失败，`renderOverview`
/ `renderKeys` / `renderSettings` 全都不执行，KPI 与模型表停在上一轮（首次就是全空），
提示只有一句「加载失败」。改为两个接口各自收尾，互不影响。

同时 `loadAccounts()` 现在**自己接住错误**并分离取数与渲染（同 `refreshLogs` 的处理）：
分页器 / 导航 / 筛选都是直接调它而不接 promise，以前任何失败都会变成浏览器控制台里的
unhandled rejection；渲染抛错也会被包成「接口失败」。新增 i18n 词条「账号渲染失败：」。

### 54.4 后端：更新检查的错误会被缓存 6 小时（关个标签页就能污染）

`versionStatus` 的单飞 leader 直接用调用方 `ctx` 去查 GitHub：控制台刚打开就关掉/刷新
页面 → 请求取消 → 查询 abort → **失败结果写进缓存 6 小时**，之后所有人打开控制台都显示
「无法检查更新」，而 GitHub 其实可达。改用 `context.WithoutCancel(ctx)`，预算仍由
`fetchLatestRelease` 的 8s 超时兜底。回归：`TestVersionStatusSurvivesCallerCancel`
（旧代码复跑：`Error:Get ...: context canceled` 且 `Cached:true`）。

### 54.5 后端：签到「已有探测在跑」既算探过、又算收敛

`handleAdminCheckin` 里 `probed: len(batch)` 把跳过的也算成本次探过，而跳过的条目又写进
`settled`，于是 `remaining` 认为它已完成。结果：控制台报「配额刷新完成：N 个账号」，
其中一部分根本没测；`remaining` 归零后也不再重试。现在 `probed` 只计真正发起探测的、
新增 `skipped` 字段，且跳过**不进** `settled`（与函数里原有的注释一致）。
回归：`TestCheckinSkipsInFlightProbes`。

### 54.6 顺手收紧的两处

- `handleAdminLogs`：以前只拦 `DELETE`，`POST/PUT/PATCH` 会落进查询分支把日志回包，
  调用方看到 200 容易误判「写成功」。现在只接受 `GET / DELETE`，其余 405
  （回归：`TestLogsEndpointRejectsUnsupportedMethod`）。
- 清理：`gzipWriter.req`（写了没人读）、`app.js` 的 `state.data`（早已不用的字段）、
  `render/logs.js` 的 `l.latency_ms + "ms"`（缺字段会显示 `undefinedms`）。

### 54.7 明确记下、本轮不改的三处

| 位置 | 为什么不改 |
| --- | --- |
| `store.Record` 丢掉 `logs.append` 的写盘错误 | 要做得对需要「限频告警 + 指标」，否则坏盘上每请求刷一行日志；等于给日志加一条观测通道，单独一轮 |
| `settingsPatch.apply` 忽略 `request_timeout_sec / max_retries / log_keep` 的 0 | 这三个字段后端把 0 当非法值，改成允许 0 得先定义「0 = 不超时 / 不保留」的语义，属于协议变更 |
| 日志轮转时清空内存环形缓冲 | `QueryLogs` 走文件扫描（最多回读 4MB），环形缓冲只是扫描失败时的兜底，实际不可见 |

### 54.8 验证

```text
sh scripts/lint.sh                 exit 0（gofmt / vet / JS 语法 / 冒烟 / 测试）
node scripts/console-smoke.mjs     18 条全放行（新增 2 条）
go test -race -cover ./...         13 包全绿；internal/server 77.6%
反向验证                           5 条新断言对旧代码复跑全部 FAIL：
                                  空态中文 / 设置 0 被吞 / 缓存污染 / 签到计数 / logs 405
```

冒烟桩同步修了一处**测试自身的失真**：`El.textContent = ""` 以前只改属性、不清子节点，
渲染函数清表格的写法在桩里等于没清，断言会看到已经不在页面上的旧行。现在赋值为
setter，语义与真实 DOM 一致。

## 55. 密钥删除 / 启停的「存在性」语义（v0.1.2 之后，未发版）

**现象**：控制台点「删除密钥」提示操作成功，列表却少了一行又好像没变；两个标签页
同时开着时更明显——其中一个已经删过，另一个点删除还是「成功」。

**根因**：`handleAdminKeys` 的 DELETE 分支把 Store 的返回值丢了：

```go
_ = s.Store.DeleteKey(s.resolveKeyRef(ref))   // 返回 bool，没人看
_ = s.Store.ToggleKey(s.resolveKeyRef(ref))
```

`DeleteKey` / `ToggleKey` 找不到目标时返回 `false`，但接口照样回 `200 {"ok":true}`
并**落了一次盘**。`resolveKeyRef` 也帮不上忙：它找不到时会把入参原样返回，所以
「解析结果非空」不代表目标存在。

**修复**（三处，都是「语义说清楚」而非重构）：

| 位置 | 改动 |
| --- | --- |
| `handleAdminKeys` DELETE | 两个参数都不带 → `400`；目标不存在 → `404 {"type":"not_found_error"}`；**先校验两个 ref 再变更**，避免「删成功、切失败」留下半途状态却回 404 |
| 成功响应 | 从 `{"ok":true}` 变为 `{"ok":true,"deleted":bool,"toggled":bool}`，调用方能看出这次到底做了什么 |
| `actions.js` `keyOp` | 收到 404 也触发一次 `reload()`：目标已经不在了，列表该与真实状态对齐，而不是留一行怎么点都 404 的幽灵行 |

失败路径不再落盘（测试用 `SaveCounters()` 断言），所以「狂点已删除的密钥」不会
变成写放大。

**回归防线**：

- `TestAdminKeysDeleteReportsMissing`：缺参数 400 / 不存在 404（且响应体里不含
  `"ok":true`）/ 号池与写盘计数都不动 / 真实目标的成功路径与 `deleted`·`toggled`
  回报。对旧代码复跑：`缺参数时 = 200，期望 400：{"ok":true}`。
- 冒烟新增「密钥操作遇到 404 仍然重拉」：把 fetch 桩临时换成 404，断言 `reload`
  被调用且仍报错。旧 `keyOp` 复跑：`404 后 reload 0 次（期望 1）`。

**验证**：`sh scripts/lint.sh` exit 0；冒烟 19 条全放行；`go test -race ./...` 全绿。

## 56. 线上部署暴露的三件事：缓存命中率口径、客户端断开被算成失败、docker 版本号

**触发**：v0.1.2 发布后对线上实例（`deepai2api.liziai.cloud`）做了一次体检，
三处「数字说不通 / 部署说不清」的地方都在本轮修掉。

### 56.1 缓存命中率的两个口径被混在一张卡片里

**现象**：控制台 KPI 显示缓存命中率 **15.1%**；而用当天的数字手算却是
`228.2M / 242.4M ≈ 94%`。

**根因**：卡片算的是 `total_cached_tokens / total_tokens_in`（**累计 / 累计**），
却和「今日 Token 入 / 出」并排展示，读起来像今日指标；而且它永远不随当天变化。
两个累计值本身没错（228,175,986 / 1,510,839,185 = 15.10%），错的是「拿累计分子
配当日分母」这种混用——那会得出一个既不是累计也不是当日的数。

**修复**：

| 位置 | 改动 |
| --- | --- |
| `store.Stats` | 新增 `DayCachedTokens`（`day_cached_tokens`），与 `DayTokensIn` 同生命周期：`Record` 累加、`rollStatsDay` 与 `Stats()` 的读路径跨天清零 |
| `overview.js` | KPI 改为「缓存命中率（今日）」，主值用当日；累计命中率与 `命中 / 入` 的绝对量放 tooltip |
| `overview.js` | 顺手把 `renderOverview` 拆成 `renderKpis` + `renderModels`（cc 32 → 两个都在门槛 25 之内），排序取值与行渲染各自成函数 |

### 56.2 客户端断开被记成 502 并计入失败率

**现象**：线上最近 400 条日志里有 4 条 502，其中 3 条是
`Post "…/chat/completions": context canceled`——那是调用方（浏览器 / SDK）
自己走了，上游没出错。

**根因**：`statusOf` 把除 `HTTPError` 之外的一律记成 502，而 `writeUpstreamErr`
对 `context.Canceled` 是**什么都不写**（连接已经没了）、对 `DeadlineExceeded` 写
**504**。日志状态码与实际响应因此不一致；`Store.Record` 又把「状态 ≥ 400」一律
计入 `failed_requests`，失败率被这类请求抬高。另外 `MarkAccountFailure` 还会给
账号 `failures++`（虽然被判 transient、状态不降级），让「这个号是不是坏了」的判断
掺进与上游无关的噪声。

**修复**：

| 位置 | 改动 |
| --- | --- |
| `store` | 新增 `StatusClientGone = 499`（nginx 约定），注释说明它只出现在我们自己的日志里 |
| `statusOf` | `context.Canceled` → 499；`context.DeadlineExceeded` → 504，与 `writeUpstreamErr` 的实际响应一致 |
| `Store.Record` | 499 既不计成功也不计失败（只保留日志行） |
| `Store.MarkAccountFailure` | `context.Canceled` 直接返回：我们主动放弃的请求不该算账号的错 |
| `render/logs.js` | 499 的徽章用 `warn` 色，与真正的 4xx/5xx 区分 |

### 56.3 Docker 版本号恒为 `docker`，控制台永远不提示升级

**现象**：线上 `/health` 与顶栏都显示 `docker`；`/admin/api/version` 返回
`latest=v0.1.2` 但 `update_available=false`。

**根因**：`Dockerfile` 里写死 `-X main.version=docker`，而 `parseSemver("docker")`
解析失败 → `versionNewer` 恒为 false。也就是说 docker 部署的用户永远收不到升级提示，
而发布页上明明有新版。

**修复**：`Dockerfile` 加 `ARG VERSION=docker`（默认值保持 `docker`，明确表示
「不是按 tag 构建的」）；`docker-compose.yml` 透传 `VERSION` 构建参数；
`make docker` 自动带上 `git describe` 的版本。部署方式：

```bash
VERSION=$(git describe --tags) docker compose up -d --build
```

### 56.4 回归防线

- `TestStatsDayCachedTokens`：当日 cached 累加、跨天清零、累计值不受跨天影响。
- `TestRecordClientGoneIsNotFailure`：499 不进 `FailedRequests`，但日志行保留。
- `TestStatusOfUsesUpstreamStatusCode` 补 3 个用例：`context.Canceled` → 499、
  包装后的取消仍是 499、`context.DeadlineExceeded` → 504（旧代码复跑：
  `statusOf = 502，期望 499`）。
- 冒烟新增「缓存命中率用今日口径，累计进 tooltip」（今日 75.0% + tooltip 含 10.0%）
  与「日志 499 用 warn 色」。

### 56.5 升级当天的过渡态（说清楚，免得当成新 bug）

老版本从来没按天累计 cached，所以升级那一刻 `day_cached_tokens = 0`，而
`day_tokens_in` 是从状态文件里带过来的（线上当天的 242.4M）。也就是说
**升级当天**那格可能显示 `0.0%`（或远低于真实值），第二天起才是完整的当日口径。
分母有值、分子没历史，这种「半个口径」没法补算——真正的修复是前面那句
「累计值退到 tooltip」：悬停就能看到累计命中率与绝对量。

### 56.6 验证

```text
sh scripts/lint.sh                  exit 0（gofmt / vet / JS 语法 / 冒烟 / 测试）
node scripts/console-smoke.mjs      21 条全放行（新增 2 条）
go test -race ./...                全绿；internal/server 覆盖率维持 77%+
```

线上实测对照（修复前）：`total_cached_tokens=228,175,986`、
`total_tokens_in=1,510,839,185` → 累计 15.10%（旧卡片显示的就是这个数）；
`day_tokens_in=242,357,695` → 当日命中率由新的 `day_cached_tokens` 给出。

## 57. 项目 Logo 与 README 顶部居中（v0.1.2 之后，未发版）

**需求**：用本机 DeepAI Code 桌面端的图标当项目 Logo，README 顶部（项目名 / 简介 /
徽章）居中展示。

### 57.1 图标从哪来

桌面端安装目录里**没有独立的 `.ico`/`.png` 文件**（只有 Electron 打包产物），
图标有两个来源：

| 来源 | 位置 | 可用尺寸 |
| --- | --- | --- |
| asar 内资源 | `resources/app.asar` → `apps/desktop/prod-resources/icon.png` | **1024×1024** PNG（93KB） |
| exe 内嵌图标组 | `DeepAI Code.exe` 的 `RT_GROUP_ICON`（16/24/32/48/64/128/256，PNG 压缩帧） | 最大 256 |

取 1024 那张（同一套图，分辨率更高），按下面的方式落到仓库：

- `docs/assets/logo.png`：缩到 **512×512**（LANCZOS + optimize），31KB —— README 里按
  128px 显示，2x 屏也清晰，不必下发 93KB 的原图；
- `docs/assets/logo.ico`：从同一张图生成 **16/24/32/48/64/128/256** 多尺寸 ICO（26KB），
  将来做 Windows 快捷方式或 favicon 直接用；
- `.gitattributes` 补 `*.ico binary`（原本只有 `*.png binary`，避免二进制被当文本 diff）。

### 57.2 README 布局

```html
<div align="center">

<img src="docs/assets/logo.png" alt="DeepAICode2API" width="128" height="128">

# DeepAICode2API
（简介两段）
（8 个徽章）

</div>
```

放在居中 `div` 里面而不是用 `<p align="center">`：这样标题 / 简介 / 徽章共享同一个
居中容器，后续增删徽章不用重复写对齐标记。徽章保持 markdown 写法（`[![alt](img)](link)`），
GFM 会把相邻行并成一段，按宽度自然换行。

### 57.3 版权说明（必须写清）

图标是 DeepAI Code 桌面端的**品牌资产**，不是本项目的原创物。因此：

- README 的 License 段落显式声明「Logo 例外：版权归其所有者，不在 MIT 授权范围内」；
- 文件清单里注明 `docs/assets/` 的用途与尺寸；
- 只把它当「这是哪个上游客户端的网关」的标识，不暗示官方背书。

### 57.4 验证

```text
python -c "from PIL import Image; print(Image.open('docs/assets/logo.png').size)"
  → (512, 512) RGBA；ICO 7 个尺寸帧

GitHub 渲染接口（POST /markdown, mode=gfm）：
  <div align="center"> 保留、<img> 在 div 内、8 个徽章仍在、License 段落为普通引用块
```

## 58. 控制台用上同一个 Logo：/favicon.ico + apple-touch-icon（v0.1.2 之后，未发版）

**需求**：§57 已经把 DeepAI Code 桌面端的图标放进 `docs/assets/` 当项目 Logo，
控制台也应该用它（标签页 / 书签 / iOS 主屏），而不是继续让浏览器去猜默认路径、
每开一次页面打一发 404。

### 58.1 做了什么

| 位置 | 改动 |
| --- | --- |
| `internal/server/web/favicon.ico` | `docs/assets/logo.ico` 的副本（26KB，16/24/32/48/64/128/256）——`go:embed` 只能读包内文件，所以必须有一份 |
| `internal/server/web/apple-touch-icon.png` | 同源图缩到 180×180（8KB）；iOS 主屏图标只认 PNG，ICO 会被忽略 |
| `web.go` | `go:embed` 加两条；`consoleAssetType` 补 `.ico → image/x-icon`、`.png → image/png` |
| `web.go` `handleConsoleAsset` | 白名单加 `favicon.ico` / `apple-touch-icon.png`（仍是「允许的名字」列表，不拼路径） |
| `server.go` | 注册 `/favicon.ico`、`/apple-touch-icon.png` 两条路由 |
| `index.html` / `login.html` | `<link rel="icon" href="/favicon.ico" sizes="any">` + PNG 版 + `apple-touch-icon` |
| `observability.go` | 两个图标加进 `isStaticAsset`：与 `/style.css`、`/js/*` 同等待遇，不写访问日志 |

复用现有 `serveConsoleAsset`，所以图标自动获得与其它静态资源一致的响应头：
`ETag` + `Cache-Control: no-cache`（可 304）、`X-Content-Type-Options: nosniff`、
`Referrer-Policy: no-referrer`、同一条 CSP（`img-src 'self' data:` 已覆盖）。

### 58.2 为什么 ICO 和 PNG 都要

- **favicon.ico**：浏览器直接请求 `/favicon.ico` 这个固定路径；多尺寸 ICO 让
  16px 的标签页与 256px 的书签/任务栏都清晰；
- **apple-touch-icon.png**：iOS Safari 不认 ICO 作主屏图标，必须 PNG（180×180 是
  iOS 推荐尺寸）。

### 58.3 回归防线（`console_favicon_test.go`）

| 断言 | 守住什么 |
| --- | --- |
| `TestConsoleFaviconServed` | 200 + 正确 Content-Type + **ISO 魔数**（`00 00 01 00` / PNG 签名）+ ETag + 304 + nosniff——防止「路径通了但内容是空文件」 |
| `TestConsolePagesReferenceFavicon` | 两个页面都真的写了 `<link rel="icon">`（只 embed 不引用 = 浏览器继续猜默认路径） |
| `TestConsoleFaviconMatchesDocsAsset` | `web/favicon.ico` 与 `docs/assets/logo.ico` **按字节比对**：换 Logo 时忘了同步，会出现「README 是新图、标签页还是旧图」 |
| `TestStaticAssetsSkipAccessLog` | 图标不进访问日志（否则每条控制台访问多两行噪声、`/metrics` 的 2xx 也会虚高） |

### 58.4 验证

本地起了一份临时实例（`-port 10099 -no-import`）实测：

```text
/favicon.ico        -> 200 image/x-icon 26735B   魔数 00 00 01 00
/apple-touch-icon.png -> 200 image/png 8204B
/                   -> 200 text/html，<head> 里三条 <link rel="…icon…">
带 If-None-Match     -> 304（ETag "ebe78f92c0121ac8…"）
响应头              -> nosniff + no-referrer + 同一条 CSP
```

测试：`go test ./...` 全绿（556 用例 = 424 顶层 + 132 子用例）、冒烟 21 条不变。

## 59. 客户端更新到 0.3.36：车道 4 → 5，其余协议面未变（v0.1.3）

**触发**：2026-11-13 例行扫描发现官方更新源 `latest.yml` 已发 **0.3.36**
（2026-11-13T05:42Z，releaseNotes 主要是「断线/中断回复后的聊天恢复、登录可靠性、
更新检查卡住、终端标签页与分屏浏览器修复」），本机还停在 0.3.35。

**客户端更新**（0.3.35 → 0.3.36）：

- `latest.yml` 的 sha512 与下载包逐字节一致（`cnqeCSWvK3…`，483 MB）；
  官方 CDN 单连接只有 ~40–60 KB/s，改用 16 路 Range 分段并发下载再按序拼接，
  校验通过后 `DeepAI-Code-0.3.36-x64.exe /S` 静默安装，注册表 `DisplayVersion=0.3.36`。
- 下载脚本与安装包放在临时目录（`/f/tmp/dl`），不入库。

**白盒比对 0.3.35 vs 0.3.36**（`server.asar:/apps/server/dist/binCli-*.mjs`，
14.3 MB → 14.8 MB；按 `//#region` 切段后逐段 `SequenceMatcher`）：

| 面 | 结论 |
| --- | --- |
| `DEEPAI_LANE_HEADERS` | **唯一变更**：`X-DeepAI-Code-Theos` `"4"` → **`"5"`**；`X-DeepAI-Code-Instance` 仍是 `randomBytes(16).hex`（每进程一次） |
| `-Machine` / `-Check-In` / `-Beacon` | 头名与拼装逻辑未变；`sha256(小写原始 UUID)` 指纹未变 |
| 端点集合 | 推理与元数据全部照旧；**新增托管主机控制面** `/deepai_code/connect/{enroll,poll,devices,rooms,grant}/`（`X-DeepAI-Protocol: 1` + `X-DeepAI-Device/-Time/-Nonce/-Signature` 设备签名头），与推理无关，网关不实现 |
| `/models` | 请求改为带 `account.apiKey`；`decodeDeepAIModels` 校验规则一字未变（仍过滤 `gpt-oss-120b`、仍把 `deepseek-v4.1-flash` 名字改写为 `… 1017`） |
| 聊天请求体 | 字段集合未变：`store` / `max_completion_tokens` / `tools` / `tool_choice` / `parallel_tool_calls` / `reasoning_effort` / `prompt_cache_key` 出现次数两版一致 |
| 签到 | nonce 回传与 `seconds/2` 续签逻辑未变；新增「聊天前 `ensureCheckedIn`」（`apiForOwner` 对 `/chat/completions` 加闸门）——网关的 `EnsureCheckIn` 早有同义逻辑 |
| pi 运行时 | 仍 **1.3.0**；MCP toolkit 列表仍 25 个；`deepai_code__*` 工具 id 未变 |
| 登录 | `desktop_auth/exchange` 加重试与证书错误文案（`UNABLE_TO_VERIFY_LEAF_SIGNATURE` 等），不在协议面 |

**线上实测（2026-11-13，真实账号，直连）**：

```text
/models lane 1..5  -> 200；lane 6/7 -> 426
/theos  lane 4/5   -> 200，lane_version=5，check_in:{nonce,seconds:600}
chat    lane 4/5   -> 200（同一请求体，UA=node/curl/Chrome 三种都一样）
```

响应形状已再次换形（同一账号、同一请求体，只换 UA，3 次）：

| 字段 | 2026-11-13 实测（三种 UA 相同） |
| --- | --- |
| `id` | `gen-<32 hex>`（如 `gen-639e7bf2dd35…`） |
| `model` | 回显请求 slug：`zai/glm-5.3-flash`（注意前缀从 `z-ai/` 变成 `zai/`） |
| `provider` / `service_tier` / `native_finish_reason` / `cost_details` / `market_cost` | **全部不出现** |
| `system_fingerprint` | 恒给 `fp_…` |
| `reasoning_details[0].format` | 文本回复 `stealth-v1`；强制工具调用 `unknown` |
| SSE | 形状未变：`delta.reasoning_details`（`gwst1.…` 加密串）+ 末帧 usage + `[DONE]` |
| 工具调用 | 仍标准 OpenAI：`tool_calls[].function.arguments` + `finish_reason=tool_calls` |

→ **「UA 分流」与整套「provider/gen_ 前缀判替身」判据作废**（`docs/recon.md` §3.1
已按当天实测重写，旧表保留为历史）。另新增一个错误码：余额为负时打付费模型回
`403 theos_exhausted`（body 带 `refresh_at`），已记入 §4 错误表。

**代码改动**：

| 文件 | 改动 |
| --- | --- |
| `internal/deepai/types.go` | `LaneVersion` `"4"` → `"5"`；`AppVersion` `0.3.35` → `0.3.36`；新增新 400 文案常量 `CodeNotAcceptedMsg`；注释按 0.3.36 复扫重写 |
| `internal/deepai/client.go` | `LaneCandidates` 改为 `5` 优先、`4` 兜底（上游抬车道时仍可自动跟随）；`BodyRejected` 同时匹配新旧两种文案 |
| `internal/server/lane.go` | 新增 `newerLane`：持久化的旧车道值不再压住编译期新车道；`CurrentLane` 走它 |
| `internal/server/server.go` | 启动时用 `newerLane(settings.Lane)` 初始化客户端车道 |
| `internal/deepai/client_test.go`、`internal/server/lane_test.go` | 车道断言 4 → 5；探测用「下一档车道 6」做夹具；新增 3 个测试（旧值升级、保留更新值、非数字兜底） |

**顺带发现的第二个兼容点：请求体形状的 400 文案变了**。同一账号、`lane=5`、
同一请求体（无 system）做 4×2 矩阵后确认：

| UA | 机器指纹 | 缺 system | 带 system |
| --- | --- | --- | --- |
| `node` | 本机真实指纹（桌面端用过的） | **400 `This request cannot be accepted.`** | 200 |
| `node` | 随机新指纹 | 200 | 200 |
| `curl/8.x` | 本机真实指纹 | 200 | 200 |

旧实现只认 `This request is not one DeepAI Code sends.`，遇到新文案不会触发
`ChatBodyFallback`（补 system + 收敛字段），400 会直接透传给调用方。
`BodyRejected` 已改成两种文案都匹配，并补了 `client_misc_test.go` 与
`server/chat_test.go` 两条回归（后者跑完整兜底重试链路）。

**这条不是边角问题**：网关默认部署在桌面端同一台机器上，`X-DeepAI-Code-Machine`
就是这台机器的真实指纹，正好落在新文案的强校验分支。本地起临时实例
（`-port 10097`）实测：不带 system 的普通客户端请求第一次就是
`400 This request cannot be accepted.`，日志打出
`[upstream] 请求体被拒，按桌面端形状重试`，补 system 后 200；流式 + 强制工具调用
同样 200（`tool_calls` 分片正常）。旧实现会把这个 400 原样回给调用方 ——
也就是说，若不改这一处，升级上游后大部分 OpenAI SDK 调用会直接失败。

**两处刻意的边界（已定稿并写进文档 / 测试）**：

| 面 | 决定 | 落点 |
| --- | --- | --- |
| `/deepai_code/connect/*`（0.3.36 托管主机控制面） | **不实现**：Ed25519 + X25519 设备身份 + 受管主机生命周期 + TLS 中继，服务的是桌面端「远程主机」功能，与推理无关 | `docs/recon.md` §2.1（完整协议：proof message 拼装、enroll 响应校验、ControlError 语义）；`internal/deepai/types.go` 包注释 |
| `GET /deepai_code/models` | **保持免凭据**：目录不随账号变化（带/不带 `Api-Key` 实测同一份 21 个模型），且它是代理预热与车道探测的探活路径 | `deepai.Client.FetchModels` 注释 + 新契约测试 `TestModelsAndLaneProbeCarryNoCredentials`（/models 与车道探测不得带 `Api-Key` / `-Check-In` / `-Beacon`） |

**验证**：`gofmt` / `go vet ./...` 无输出；`sh scripts/lint.sh` 全绿（含前端语法与 21 条冒烟）；
`go test ./...` 全绿（新增 3 个 lane 用例 + 2 个 400 文案回归 + 1 个免凭据契约用例，共 562 个用例 = 430 顶层 + 132 子用例）；
本地临时实例实测：启动即自动探测到 `车道版本: 5`、目录 `21 个模型`，
最小 body 与流式工具调用都经「补 system 重试」后 200。

**发版（v0.1.3）**：`git tag v0.1.3` + `sh scripts/release.sh v0.1.3 --publish`。
上传时发现 `release.sh` 传的是 `dist/*`，而 `dist/` 里还残留 v0.1.2 的五个二进制，
于是 v0.1.3 的 Release 一度带着上一版资产。已删掉旧资产、按 v0.1.3 重新生成
`checksums.txt`，并改脚本：构建前先清掉 `dist/deepaicode2api_*`（只清本项目产物，
不动 `dist/` 里其它文件），避免下次重演。

## 60. 线上运行数据落地：首字节预算、签到节奏、扩展指标、形状预置、错误策略表（v0.1.4）

**触发**：v0.1.3（0.3.36 车道 5）上线后跑了 82 分钟，容器日志给出四组可用数据：

```text
/health        16/16 账号就绪，0 blocked / 0 degraded，21 个模型
1500 请求      1492×200（99.5%），2×502（/v1/responses，125.7s / 131.1s）
签到           292 次「签到阻塞了推理请求」，均值 4.8s，53 次顶到 10s 超时
形状兜底       99 次「请求体被拒，按桌面端形状重试」
```

每一条都对应一个确定的设计问题；本轮把它们连同前端体验一起落地。

### 60.1 P0-1 非流式长响应被 120s 空闲超时误杀

**根因**：`holdResponseBody` 对所有响应体都套 `idleTimeoutBody`，计时器在 wrap 时
立即启动。流式请求用它防「发完首帧挂住」是对的；非流式请求在首字节到达前本来
就可能想很久（`request_timeout` 默认 600s），于是 120s 一到就被主动关闭，读成
`errUpstreamIdle` → 502。线上两次 502 的 125.7s / 131.1s 正好是 120s + 尾部开销。

**改法**：

| 文件 | 改动 |
| --- | --- |
| `internal/server/idle.go` | `newIdleTimeoutBody(rc, idle, first)`：首字节预算与帧间预算分开，拿到首字节后切回 idle；新增 `upstreamFirstByteTimeout = 300s` |
| `internal/server/upstream.go` | `openModel(..., stream bool)` + `holdResponseBody(body, acct, stream)`：流式 120s / 非流式首字节 300s |
| 各 handler | 按真实 `stream` 标志传参（chat / responses / anthropic / callJSON） |
| `idle_test.go` | 4 条：慢首字节放行、帧间静默判死、首字节超预算判死、流式/非流式预算不同 |

### 60.2 P0-2 签到节奏退化成 30s 一次

**根因**：`EnsureCheckIn` 用「最新 nonce != 已回传 nonce」判断是否要补签。上游每次
`/theos` 都下发**新** nonce，于是每次回传完成后条件又成立，桌面端的 `seconds/2`
（默认 300s）节奏被完全绕过，实际由 `checkInMinInterval`（30s）兜底 —— 线上
平均 17 秒一次「签到阻塞」正是这个节奏。

**改法**：

- `checkInState` 增加 `ackedAt`（回传完成时刻），`markAcked` 记录它；
  `EnsureCheckIn` 改为「`ackedAt` 距今 < seconds/2 就跳过」；
- `EnsureCheckIn` 返回 `CheckInOutcome{Attempted, Waited, TimedOut, Err}`，
  调用方据此记指标 / 告警（不再用 `time.Since` 裹一层）；
- 新增 `Client.CheckInBudget` + `SetCheckInBudget`，设置项 `checkin_block_ms`
  （默认 10000 = 行为不变；建议线上先观察，再按需下调到 2000）；
- `applyRuntimeSettings` 在启动与设置保存后同步 `s.Up` 与所有代理客户端；
- 测试：半衰期节奏（模拟时间）、预算超时、负数归零、既有签到测试全部保留。

### 60.3 P0-3 扩展指标：能回答「哪里慢 / 为什么慢」

`/metrics` 原来只有全局状态码计数与耗时 sum/count；上面两个 P0 都是靠翻 WARN
日志才发现的。新增（`internal/server/metrics.go`，零依赖定桶直方图）：

| 指标 | 类型 | 用途 |
| --- | --- | --- |
| `gateway_request_duration_seconds{endpoint}` | histogram | 按 chat / responses / messages / other 看延迟 |
| `gateway_checkin_seconds` + `gateway_checkin_total{result}` | histogram / counter | 签到耗时、慢阻塞、超预算次数 |
| `gateway_admission_wait_seconds` | histogram | 账号全忙排队等待 |
| `gateway_upstream_errors_total{path,code,status}` | counter | 上游错误码分布（Top 8 进概览） |
| `gateway_fallback_total{reason="body_shape"}` | counter | 形状兜底次数 |

`/admin/api/overview` 同时下发 `latency`（p50/p95/p99 + 按端点）、`checkin`、
`upstream_errors`、`fallback_total`，控制台概览页新增「运行健康」卡。

> 注意：`/admin/*` 只记状态码与总耗时，不进分端点直方图 —— 否则 overview 的响应体
> 会因「上一次 overview 的耗时」而每次变化，ETag 永远命中不了（`TestOverviewETag`
> 就是这么暴露的）。

### 60.4 P1-4 形状预置：省掉注定失败的 400

线上 99 次兜底重试 = 99 次多余上游调用。新增：

- `deepai.EnsureSystemMessage`：**只**在缺 system / developer 时补默认提示词，
  不裁剪其它字段（与 `ChatBodyFallback` 的全量收敛区分）；
- `internal/server/shape.go`：按 API Key 的 sha256 前缀记忆「需要桌面端形状」
  10 分钟（上限 1024 条，过期清理）；
- `openModel` 在首次发送前按需预置（仅 `preset_desktop_shape` 开启时），
  被 400 拒绝时写入记忆；命中后第二次请求只打一次上游；
- 设置项 `preset_desktop_shape`（默认关：这是采样语义变更，要显式选择）；
- 测试：TTL/上限、预置后少一次调用、开关默认关闭。

### 60.5 P1-5 错误策略表 + 当日额度用完

- 新增 `internal/server/upstream_policy.go`：`classifyUpstreamError` 把
  `(status, code)` 映射成 `actRetryOther / actReturn / actBlockAndRetry /
  actBlockAndReturn / actProbeLane / actWaitCapacity / actQuotaExhausted /
  actShapeFallback`，`openModel` 只负责执行动作；表驱动测试覆盖 11 种输入。
- `deepai` 新增 `theos_exhausted` 识别与 `QuotaResetAt`（解析 `refresh_at`，
  失败回落 6h）；
- `Account.QuotaUntil` + `AccountReady` + `pool` 调度过滤：额度用完的账号在恢复
  时间前不参与轮询（否则每个请求都白撞 403），到点自动恢复；推理成功或探测到
  余额恢复时清零。

### 60.6 前端 P1-P3

| 项 | 落点 |
| --- | --- |
| 原生对话框替代 `confirm/prompt` | 新增 `js/dialog.js`（`<dialog>` + 焦点管理 + ESC + 危险色），accounts / events 共 10 处调用改为 Promise 风格；新增样式与词条 |
| 日志「实时」开关 | 日志页新增按钮（`aria-pressed`），5s 轮询但只在**标签页可见且停在日志页**时取数 |
| GET 一次重试 | `api.js`：网络错误 / 5xx 退避 300ms 重试一次（POST 不重试，避免重复写） |
| 运行健康卡 | 概览页新增「运行健康」：延迟 P50/P95、签到 P95、形状兜底、上游错误 Top |
| 键盘与无障碍 | 行内菜单 ↑↓/Home/End/Esc、`role=menu/menuitem`、`aria-expanded`；toast 原本已带 `aria-live` |
| i18n 拆分 | 字典拆到 `js/i18n/{dict-shell,dict-pages}.js`，运行时在 `core.js`，`i18n.js` 保留薄转出；覆盖率/重复键/死词条三条棘轮改为扫描多字典块 |
| 设置页 | 新增「签到预算」「预置桌面端形状」两项及回填 |

### 60.7 验证

```text
gofmt / go vet           无输出
sh scripts/lint.sh       前端语法 + 21 条冒烟 + 全部 Go 测试通过
go test ./...            ok（594 用例 = 450 顶层 + 144 子用例）
```

新增测试：idle 4 条、签到节奏/预算 3 条、指标 4 条、形状预置 3 条、策略表 2 条、
`theos_exhausted` 2 条、pool 额度过滤 1 条、`EnsureSystemMessage` 1 条，另同步
修了 ETag 与 i18n 字典解析的契约测试。

### 60.8 上线建议

1. 先只升代码（`checkin_block_ms=0`、`preset_desktop_shape=false`）：应看到
   签到阻塞次数下降约 10×、非流式 502 归零；
2. 观察 24h 的 `gateway_checkin_*` 与 `gateway_request_duration_seconds`，
   再把签到预算调到 2000ms；
3. 打开「预置桌面端形状」前先看 `gateway_fallback_total` 的增长曲线，
   确认调用方确实缺 system（否则预置不会触发，只是多一层判断）。

---

## 61. 线上巡检四连：探测退避、版本号 `vv`、代理弹窗、缓存命中率下钻（v0.1.5）

**触发**：2026-11-14 例行巡检。客户端 `latest.yml` 仍是 **0.3.36**（与
`types.go` 的 `AppVersion` / `LaneVersion=5` 一致，无需同步）；线上容器
`local-20261113-v014-efa5d24` 跑 10h 零重启、16/16 号 ready、0 blocked。
但日志与前端暴露了四个问题。

### 61.1 探测失败没有退避：同一个号 1 小时被重探 323 次

**现象**：`docker logs --since 1h | grep -c 额度复探失败` = 323（≈每 11 秒一次），
全部指向同一个号，错误是 `DeepAI models are temporarily unavailable`。

**根因**：`probeAsync`（推理热路径的按需复探）失败时**只写日志**，没有调用
`SetAccountAllowance`，于是 `ProbeFailures` / `ProbeAfter` 永远是 0 与
「已过期」——`quotaNeedsRefresh` 一直返回 true，每个选中该号的请求都会再探一次。
`probeSem` 只限制并发（4），不限制频率；这既刷日志，也让探针本身变成上游风控特征。
对照：后台守护与手动 checkin 走的 `probeBatch` 有退避（`probeBackoff` 2m → 10m → 1h），
唯独热路径漏了。

**决策**：新增 `Store.RecordProbeFailure(id)` —— 只推进 `ProbeFailures` /
`ProbeAfter`，**不动** `Status` / `Failures` / `LastErr`。理由与既有约定一致：
一次出口抖动不该让调度器判号坏了，也不该盖掉控制台上更具体的原因
（`social_login_required` 之类）。`probeAsync` 与 `probeBatch(strict=false)` 的
失败分支都接上它，并落盘（退避窗口内最多 2 分钟一次写）。

**验证**：`internal/store` 新增 2 条（退避 2m → 10m → 1h、成功清零、不动调度字段、
未知账号无副作用）；`internal/server` 新增 1 条 —— 让上游回 400，走一次
`probeAsync`，断言 `ProbeAfter` 前进且 `quotaNeedsRefresh` 立即变 false。

### 61.2 控制台显示 `vv0.1.4+efa5d24`

**根因**：两处叠加。构建参数 `VERSION=$(git describe --tags)` 自带 `v` 前缀，
`/health` 与 `/admin/api/version` 照实返回 `v0.1.4+efa5d24`；而
`js/version.js` 的 `renderVersion` 无条件 `"v" + v`。

**决策**：前缀归展示层，服务端只回它真实知道的构建版本。`renderVersion`
改成「已经有 v/V 就不再补」，`dev` 兜底不变。

**验证**：冒烟新增 5 组用例（`v0.1.4+...` / `0.1.4` / `V1.0.0` / 空 / undefined）。

### 61.3 代理与机器指纹弹窗：从「一句话」改成「改前 → 改后」

**问题**：三个弹窗（补绑 / 重绑 / 清除代理、分配机器指纹）都是一句拼接文案，
长代理 host 会把句子扭成一团；「清除代理」用的是 primary 按钮，看起来跟安全操作一样；
英文界面下这三句还没有词条（静默显示中文）。

**决策**：`dialog.js` 支持 `rows`（label + 等宽 value），账号治理弹窗统一改成
「账号 / 当前出口 / 新出口」三行 + 一句后果说明；清除代理与删除用 `danger` 按钮
+ 语义化 `okText`（重新分配 / 分配 / 清除代理）。新增 12 条中英词条，并删掉 3 条
因改写而失效的旧词条（死词条棘轮会盯着）。

**验证**：冒烟新增 1 条结构化断言（字段数、label/value、mono 类、primary/danger 按钮与文案）。

### 61.4 缓存命中率：先分清「没报」与「没命中」

**口径**（`store.Record` + `overview.js`）：`今日命中率 = day_cached_tokens /
day_tokens_in`，分子分母都来自上游 `usage`（`prompt_tokens_details.cached_tokens` /
`input_tokens_details.cached_tokens` / `cache_read_input_tokens` 三形态）。

**观察**：控制台显示的 78.1% 无法自己解释「为什么走低」——同一个数背后可能是三件事：
上游没报缓存字段、前缀不再复用、或某条链路把 usage 丢了。另外「今日」按**网关
本地日期**滚动（容器未设 `TZ`，即 UTC），北京时间 08:00 才翻页：早上看到的
是一个只有几十分钟样本的数，天然会低且抖。

**决策**：新增 `scripts/cache-hit.mjs`，读 `/admin/api/logs` 把命中率按
模型 / 端点 / 密钥 / 小时拆开，并单独给出「有用量但 cached=0」的占比
（区分上面三种情况）；`--selftest` 带 9 条纯函数断言，不依赖网络。

```text
node scripts/cache-hit.mjs --selftest                  # 纯函数自检
DEEPAI2API_ADMIN_PASSWORD=… node scripts/cache-hit.mjs --hours 24
```

### 61.5 顺手修掉一条定时炸弹用例

`deepai.TestQuotaExhaustedDetectionAndReset` 把 `refresh_at` 写死成
`2026-11-14T00:05:52Z`，而 `QuotaResetAt` 对**已过期**的 refresh_at 会回落到
`now+6h`：日期一过，CI 必然变红（本次巡检就撞上了）。改成相对 `now+2h` 生成，
并补一条「refresh_at 已过期 → 回落到 6h」的断言，把踩过坑的语义固化成用例。

### 61.6 验证

```text
gofmt -l ./internal           无输出
go vet ./...                  无输出
go test ./... -count=1        11 个包全绿
node scripts/console-smoke.mjs 23 条全过
```

---

## 62. 2026-11-14 复核：客户端未变，但上游改了两处服务端行为（v0.1.5）

**触发**：例行扫描。官方更新源 `latest.yml` 仍是 **0.3.36**（`releaseDate`
2026-11-13T05:42Z，sha512 `cnqeCSWvK3…` 与 11-13 完全一致），本机注册表
`DisplayVersion=0.3.36` —— **客户端没有更新，无需拆包比对**。
但按「客户端没变也要复测协议面」的惯例打了一遍上游，发现两处变化都在**服务端**：

| 项 | 11-13 的结论 | 11-14 实测 |
| --- | --- | --- |
| `GET /deepai_code/models` | 带不带 `Api-Key` 返回同一份目录（21 个） | **不带 Key 一律 401 `auth_required`**；带任意账号 Key → 200，**27 个**模型 |
| 车道 | 5（4/6 回 426） | 仍是 5（`/theos` 200 且 `lane_version: 5`；4/6 仍 426） |
| 目录内容 | 21 个（10 免费 / 11 付费） | 27 个（11 免费 / 16 付费），新增 6 个、无下线 |

新增模型：`muse-ember`（免费）、`kimi-k3.1-pro`（3）、`gpt-6.1-terra`（5）、
`gemini-4-hydrogen`（8）、`gemini-4-carbon`（10）、`claude-sonnet-5-5`（10）；
`default` 仍是 `glm-5.3-flash`。

### 62.1 影响面：三条「无凭据探活」路径全部失效

| 路径 | 原来的依据 | 失败表现 |
| --- | --- | --- |
| `RefreshModels`（控制台「刷新模型目录」） | `/models` 免凭据 | 401，目录永远停在旧快照（线上 `/health` 的 `models=21` 就是这个快照） |
| `DetectLane` / `RefreshLane` | `/models` 免凭据，探活零成本 | 每个候选车道都 401，返回「所有候选车道都被上游拒绝」，车道无法自动跟进 |
| `WarmUpstream`（启动预热） | `/models` 免凭据、不烧 Theos | 全部失败，连接预热计数归零（首请求又要付整条握手成本） |

### 62.2 决策：借一个账号的凭据，而不是把 Key 撒进探活路径

- `deepai.Client.FetchModels(ctx, acct)` / `DetectLane(ctx, acct)` 新增账号参数：
  **acct 非 nil 时带 `Api-Key`，nil 时仍发匿名**（只用于区分「链路通」与「能拉目录」）；
- `Server.probeAccount()`：优先「就绪 + 带机器指纹」的号，其次任一启用且有 Key 的号。
  目录与车道探测都不消耗 Theos，借谁的 Key 都一样；带机器指纹是让请求形状更接近桌面端；
- `RefreshModels`：号池为空时提前返回「上游 /models 需要 Api-Key：请先导入或添加一个
  账号再刷新目录」，不把上游 401 包装成「Key 失效」；
- `RefreshLane`：没有账号时直接跳过探测（编译期 `LaneVersion=5` + 426 兜底仍在）；
- `WarmUpstream`：每个出口取该出口上的**真实账号**预热；**401 也算预热成功**
  （DNS/TCP/TLS/代理 CONNECT 都已打通），只有 5xx / 超时才不计。

### 62.3 验证

```text
curl -s https://api.deepai.org/deepai_code/models                     → 401 auth_required
curl -s https://api.deepai.org/deepai_code/models -H "Api-Key: …"      → 200，27 个模型
curl -s https://api.deepai.org/deepai_code/theos  -H "X-…-Theos: 5" -H "Api-Key: …"
                                                                       → 200，lane_version: 5
curl -s … -H "X-…-Theos: 4" / 6                                       → 426 lane_version_unsupported
```

新增测试 4 条：`TestModelsAndLaneProbeCarryAccountKeyOnly`（凭据契约反转：
带账号必须发 `Api-Key`、不带账号不发、签到/beacon 头始终不发）、
`TestWarmUpstreamCountsUnauthorizedAsReachable`、`TestWarmUpstreamSendsAccountKey`、
`TestRefreshModelsNeedsAccount`、`TestRefreshModelsSendsAccountKey`、
`TestProbeAccountPrefersReadyWithMachine`；既有调用点（`cmd/probe`、测试）同步改签名。

文档：`docs/recon.md` 头部 banner + §2 端点表 + 新增 §2.2（目录/车道探测的鉴权与
401 语义）；`README` 模型表按 11-14 快照重生成（27 行）+ 修正「免凭据」那条结论。

### 62.4 遗留

线上容器仍是 11-13 的镜像（目录快照 21 个）。重新部署后在控制台点一次
「刷新模型目录」，`/health` 的 `models` 才会变成 27；在此之前 `muse-ember` 等
6 个新模型在网关侧不可见（请求会被宽松解析回落，不会 404）。

---

## 63. 文档整理、仓库清理与 v0.1.5 发版

### 63.1 文档

| 动作 | 落点 |
| --- | --- |
| 验收基线按当前代码重算 | README 徽章与验收表：测试 **594 → 602**（458 顶层 + 144 子用例）、测试行数 14.4k、前端冒烟 **21 → 23** 条 |
| 修掉悬空引用 | README 模型表里「详见 §反向结论」并不存在该章节，改为指向 §10.1「已知坑」 |
| 版本示例跟上当前版本 | 发版流程示例 `v0.1.3` → `v0.1.5` |
| 已知坑补一条 | `/models` 401 `auth_required`（2026-11-14 起要求 `Api-Key`）：不要当成「Key 失效」去清号 |
| 文档索引加「状态」列 | 区分 `维护中` / `历史`：`optimization-plan*.md` 标为历史，`model-fingerprint-2026-11-10.md` 标为历史快照（目录已从 22 → 27，判据仍适用） |
| 快照文档加 banner | `docs/model-fingerprint-2026-11-10.md` 顶部注明目录已变，避免被当成当前目录引用 |

### 63.2 仓库清理

| 清理项 | 原因 |
| --- | --- |
| `dist/` 里 v0.1.4 的 5 个二进制 + `checksums.txt` | 构建产物（gitignore）；`gh release upload` 传的是 `dist/*`，残留会被一起上传（v0.1.3 踩过） |
| 仓库根目录 `deepaicode2api-data.json` / `-stats.json` | 运行期状态（含上游 Key，AES-GCM 加密）且是**陈旧快照**：README 与 §40.1 都说过「不能当模型目录依据」，直接删掉比留着提醒更可靠 |
| 巡检期临时文件（`.tmpk`、`.m2.json`、`.theos*.json`、`/tmp/m*.json`） | 含本机 API Key 或上游响应正文，用完即删并已复扫确认无残留 |

`.env` 仍保留在本地（gitignore，绝不入库）。

### 63.3 发版

```text
git tag v0.1.5 && git push origin v0.1.5
sh scripts/release.sh v0.1.5 --publish     # 5 个平台二进制 + checksums.txt → GitHub Release
```

版本内容：§61（探测退避 / 版本号 `vv` / 弹窗结构化 / 命中率下钻）+ §62
（`/models` 强制 `Api-Key`、模型目录 21 → 27）+ 本轮文档与清理。

发版后建议：控制台点一次「刷新模型目录」，`/health` 的 `models` 应从 21 变 27；
再跑一次 `node scripts/cache-hit.mjs --hours 24` 复核命中率口径。

---

## 64. 2026-11-15 复核：客户端 0.3.37（车道 5 → 6，v0.1.6）

**触发**：例行扫描。官方更新源 `latest.yml` 从 **0.3.36** 变为 **0.3.37**
（`releaseDate` 2026-11-14T08:08Z；本机装的还是 0.3.36）。这次客户端**真的更新了**，
所以按惯例拆包做 wire protocol 比对。

### 64.1 结论：协议面只有车道一处变更

下载 `DeepAI-Code-0.3.37-x64.exe` → 7z 解 NSIS 的 `$PLUGINSDIR/app-64.7z` →
`resources/server.asar` → `apps/server/dist/binCli-BIPvs5sY.mjs`（14.8 MB），与
0.3.36 的 `binCli-Dlwr1V0p.mjs` 做全量 diff：**27 个 hunk / 296 行**，其中

| 区域 | 0.3.37 的差异 | 是否协议面 |
| --- | --- | --- |
| `DEEPAI_LANE_HEADERS` | `X-DeepAI-Code-Theos: "5"` → **`"6"`** | **是（唯一一处）** |
| `deepAIBeacon` | 未改；`v=` 仍取打包的 `version$2`（0.3.37） | 否（只是版本号跟着走） |
| `/models` | 新增 `catalogAccountId`：切换账号时目录退回内置快照 | 否 |
| 模型条目 | `thinkingLevelMap` 给 `deepai-cyber` / `muse-ember` 加 `max` / `xhigh` | 否（桌面端下拉） |
| 其它 | 长会话压缩、工具失败去重、Codex 审批、SQLite 索引、trace 默认级别 | 否 |

端点集合、5 个 `X-DeepAI-Code-*` 头、实例号粒度（每进程 `randomBytes(16).hex`）、
机器指纹、签到 / beacon 形态、聊天请求体字段、`/models` 解码全部未变
（pi 运行时仍 1.3.0）。

**2026-11-15 实测**（单账号、单次/格）：

| 端点 | lane 4 | lane 5 | lane 6 | lane 7 |
| --- | --- | --- | --- | --- |
| `GET /deepai_code/models` | 未测 | 200 | **200** | 426 |
| `GET /deepai_code/theos` | 426 | 200 | **200**（`lane_version: 6`） | 未测 |
| `POST /deepai_code/chat/completions` | 未测 | 200 | **200** | 未测 |

**两份目录不一样**：车道 6 把 27 个模型的 `contextWindow` 全部收敛到 **320,000**
（车道 5 仍是各模型的真实上限 1M / 991k / 512k…），`maxTokens` 不变。
`/models` 对旧车道很宽容（5 也 200），所以它**不能**用来判断“当前车道”，
只能判断“某个车道能不能用”。

### 64.2 代码同步

- `deepai.LaneVersion` `5` → `6`，`deepai.AppVersion` `0.3.36` → `0.3.37`；
- **探测顺序重构**：原来的 `var LaneCandidates = {"5","6",…}` 是静态列表，第一项永远
  是旧车道。上游这次同时接受 5 / 6 之后，`DetectLane` 会第一次就命中 5 并
  `SetLane("5")`，而 `CurrentLane()` 又按 `newerLane` 报 6 —— 状态自相矛盾，
  实际请求仍带旧车道、目录也停在旧版本。改成 `laneCandidates()`：
  **编译期车道优先 → 向上猜 `laneGuessAhead` 个 → 再回落到 2**；
- 测试：`TestDetectLanePicksAcceptedLane` 改用 `LaneVersion`；新增
  `TestDetectLaneStartsAtCompiledLane`（所有车道都接受时，只能探测一次且必须是
  编译期车道）；`internal/server/lane_test.go` 的“新车道”场景从 6 改成 7，
  “旧状态文件”场景从 4 改成真实的 5 → 6 升级。

### 64.3 文档

`docs/recon.md` 头部加 0.3.37 banner + §3 头表 / 车道矩阵 / 白盒复核；
`README` 的客户端版本、车道、协议头、模型表上下文列（320k）与心跳版本同步。

### 64.4 客户端升级与清理

用官方安装包静默升级本机桌面端：`0.3.36 → 0.3.37`（sha512 与 `latest.yml` 一致），
安装后 `DeepAI Code.exe` 文件版本为 0.3.37、`resources/app.asar` 时间为
2026-11-14T08:07Z。清理了 0.3.36 / 0.3.37 的安装包与解包目录、`deepai-code-desktop-updater`
缓存里的 `installer.exe`、旧的 0.3.35 / 0.3.36 临时解包，以及含 Cookie 的旧
HAR 临时文件，共约 2.6 GB；`~/.deepai-code` 里的登录凭据和设置保留。

### 64.5 可选项落地

| 项 | 实现 |
| --- | --- |
| `BodyRejected` 文案兜底 | 400 + `code=invalid_request_error` 也按形状拒绝处理（`ChatBodyFallback` 同号重试一次）；其它 400（上下文超限 / 参数错误）不误判 |
| Anthropic `budget_tokens` 新档位 | 4k 以下 low、16k 以下 medium、32k 以下 high、64k 以下 xhigh、再高 max；`output_config.effort` 仍原样透传。2026-11-15 实测上游对 `deepai-cyber` / `muse-ember` / `glm-5.3-flash` 接受六档 |
| `newerLane` 反向场景 | 新增持久化 `Settings.LaneBuild`：写入 `Lane` 时记录当时的编译期车道。`LaneBuild == LaneVersion` 说明探测结果由本次构建验证过，即使比编译值旧也信任；否则仍按 `max(存储值, 编译值)`，避免升级后跑旧车道 |

### 64.6 深度扫描：指纹 / 请求接口 / 工具调用

| 模块 | 发现 | 处理 |
| --- | --- | --- |
| 机器指纹（Linux） | 网关先读 `/sys/class/dmi/id/product_uuid`，固件占位值会返回不稳定指纹；0.3.37 桌面端只读 `/etc/machine-id` → `/var/lib/dbus/machine-id`（32 位小写 hex、非全 0） | 删除 product_uuid 分支，新增 `parseLinuxMachineID` + 用例 |
| Anthropic 工具结果顺序 | `tool_result` 前的文本会先变成 user 消息，插在 assistant(tool_calls) 与 role=tool 之间，违反 OpenAI 顺序 | tool_result 全部先行，文本/图片随后 |
| Anthropic 流式并行工具 | OpenAI 的 `tool_calls[].index` 分片可能交错；原实现把后到分片写进当前打开块 | 按 index 缓冲参数，`Finish()` 时按序一次性发 `input_json_delta` |
| Responses 并行 function_call | 每个 item 单独成 assistant，`function_call_output` 不再紧跟对应 `tool_calls` | 连续 `function_call` 合并进同一条 assistant 的 `tool_calls` |
| Responses 流式输出顺序 | 工具项存 map，`closeTools` 直接遍历导致最终 `response.output` 顺序随机 | 按 `output_index` 排序后发出 |

请求接口与请求头逐项复扫：端点集合、`Api-Key` / 车道 / 实例号 / 机器指纹 /
签到 / beacon 头、普通 Node fetch 头、`/speech_to_text` 的 multipart 形态均与
0.3.37 一致；0.3.37 未新增推理侧请求头或端点。

### 64.7 验证

```text
gofmt -l ./internal ./cmd   无输出
go vet ./...                无输出
go test ./... -count=1      11 个含测试的包全绿
go test -race ./internal/deepai ./internal/server   全绿
测试总数 602 → 614（顶层 458 → 466，子用例 144 → 148）
```

新增用例：`TestBodyRejectedFallsBackToInvalidRequestCode`、
`TestChatBodyRejectedRetriesOnInvalidRequestCode`、`TestDetectLaneStartsAtCompiledLane`、
`TestNewTrustsLowerLaneVerifiedByThisBuild`、`TestNewPrefersCompiledOverProbeFromOlderBuild`、
`TestAnthropicBlockWriterInterleavedToolCalls`、`TestResponsesToChatParallelToolCalls`、
`TestParseLinuxMachineID`，以及 Anthropic effort 两档与 tool_result 顺序子用例。

### 64.8 遗留

- 车道 6 的 320k 是**客户端压缩预算**还是新的服务端上限，暂按上游原样透传；
  若上游恢复各模型真实上限，目录刷新会自动跟上，不需要改代码；
- `budget_tokens → xhigh/max` 的阈值是量级近似（Anthropic 没有标准换算）；
  若上游开始区分档位效果，再按实测调整分界；
- 本机已升级到 0.3.37，但暂无真实进程抓包；下次可直接对比运行中的 `/theos`
  beacon 与包内代码。

---

## 65. 结构治理与 0.3.37 复核收尾（2026-11-15）

**触发**：三项并行任务——拆 `internal/server`、重构 `api.js`、复核未提交的
0.3.37 改动，并补一次 `-race` + 覆盖率基线。

### 65.1 `internal/server` 拆分（第一阶段）

`internal/server` 原来 83 个文件 / 14.6k 行，把路由、协议、管理台、中间件、
静态资源混在同一层。第一阶段只搬**零耦合的叶子**（不碰 `*Server`、不反向依赖）：

| 新包 | 内容 | 行数 | 说明 |
| --- | --- | --- | --- |
| `internal/httpx` | `WriteJSON` / `WriteErr` / `WriteRawJSON` / `DecodeOptionalJSON` / `DecodeJSONRequest` / `ReadBodyWithLimit` / `ContextWithTimeout` / `StatusOf` / `ETagMatches` / `WithGzip`（含 `AcceptGzip`） | 310 | 160 处 `httpx.*` 调用点（编译期校验，纯机械替换）；`etagMatches` 原本在 `admin_api.go` 与资源服务里各一份，现在合并为一份 |
| `internal/server/web` | `embed.FS`（HTML/CSS/JS/图标）+ ETag 指纹 + Content-Type + CSP + 304 协商 | 170 | 资源路径白名单仍留在 `server`（属路由契约）；`web.Read` 兼容旧的 `web/` 前缀 |

结果：`internal/server` 非测试代码 **14.6k → 7.0k 行（83 → 38 个文件）**。
剩下的 38 个文件按域聚成四块，后续按此顺序搬（每步都保持 `*Server` 门面不动）：

1. `metrics`（`metrics.go` + `observability.go` 的计数/直方图核心）——渲染体
   仍读 `Store`，需要先抽出一个只读快照接口；
2. `relay`（`chat.go` / `responses.go` / `anthropic.go` / `stream.go` / `sse.go`）——
   `sseWriter` 与 `Usage` 要先下沉到 `gateway`；
3. `sched`（`probe.go` / `admission.go` / `upstream_policy.go` / `lane.go` /
   `warmup.go`）——依赖 `Server` 的探测信号量与 `quotaWG`；
4. `admin`（`admin*.go`）——依赖 `Store` 最多，留到最后。

### 65.2 `api.js` 重构

原实现把「发请求 → 解析 → 错误归一」写了两遍（首发 / 重试），复杂度 47 集中在
`api()` 一个函数里。拆成 `decodeResponse` / `fetchJSON` / `makeError` /
`isTransient` 后，`api()` 只剩去重、超时与重试编排。顺带修掉重试路径的两个
不一致：401 不再漏掉「清口令 + 回登录页」，英文错误分类在重试路径也生效。
`node --check` 与 `scripts/console-smoke.mjs`（Go 侧 `TestConsoleSmoke`）全过。

### 65.3 0.3.37 改动复核

逐文件看过未提交的 22 个文件（+777/−163），确认以下改动成立：车道 6 与
`laneCandidates` 的「先向上猜、再向下兜底」、`Settings.LaneBuild` 的
「本构建验证过的探测值优先」、Linux 机器指纹只认 systemd machine-id、
`budget_tokens → xhigh/max` 两档、Anthropic `tool_result` 前置、流式
`tool_calls[].index` 缓冲、Responses 并行 `function_call` 合并。

复核中修掉两处：

| 位置 | 问题 | 处理 |
| --- | --- | --- |
| `responses_stream.go` | 只按工具 index 排了 `toolItems`，但 `response.output` 的追加顺序是固定的「文本 → 推理 → 工具」。上游若先发工具、后发正文，最终 `output` 会把文本排在工具前面，与流里报的 `output_index` 矛盾 | `output` 改为 `{index, item}`，`response()` 用 `sortedOutput()` 按 index 排序；新增 `TestResponsesOutputSortedByOutputIndex` |
| `anthropic.go` | 流式出错时也调用 `Finish()`，把只到一半的 `input_json_delta` 当完整工具块发出，客户端解析必败 | 出错路径改为只 `Close()` 当前块；新增 `TestAnthropicBlockWriterCloseKeepsToolsBuffered` |

留作已知项（未改）：

- `HTTPError.BodyRejected()` 现在对任何 `400 + invalid_request_error` 都返回真，
  chat 路径会补形状重试一次。若上游把「上下文超限 / 参数错误」也用这个 code，
  每个真 400 会多打一次上游（`ChatBodyFallback` 只在确实改动了请求体时才重试）。
  目前与实测文案一致，暂按「宁可多一次、不吞新文案」保留；
- Linux 不再回退 `product_uuid`，Alpine / scratch 这类没有 `/etc/machine-id` 的
  容器会拿不到指纹（上游 `machine_required`）。这类部署必须配 `DEEPAI_MACHINE_ID`，
  `docker-compose.yml` 已有该变量。

### 65.4 验证基线

```text
gofmt -l ./internal ./cmd ./main.go ./tasks.go ./env.go   无输出
go vet ./...                                              无输出
go test ./... -count=1                                    10 个含测试的包全绿
go test -race ./... -count=1                              全绿（22.6s）
go test -coverpkg=./internal/... ./...                    84.1%（基线 84.0%）
测试总数 614 → 616（顶层 466 → 468，子用例 148 不变）
```

覆盖率热点（未被覆盖的主要是错误分支）：`deepai/client.go`、`store/store_accounts.go`、
`server/login.go`、`server/metrics.go`、`server/probe.go`。

---

## 66. 2026-11-18 复核：客户端 0.3.41（车道 6 → 7，v0.1.6+）

**触发**：例行扫描。官方更新源 `latest.yml` 从 **0.3.37** 变为 **0.3.41**
（`releaseDate` 2026-11-18T04:27Z；本机当时仍停在 0.3.37）。这次客户端真的更新了，
按惯例拆包做 wire protocol 比对。

### 66.1 结论：推理协议面只有车道一处变更

下载 `DeepAI-Code-0.3.41-x64.exe`（sha512 与 `latest.yml` 一致）→ 7z 解 NSIS 的
`$PLUGINSDIR/app-64.7z` → `resources/server.asar` → `apps/server/dist/binCli-DAkBc9bP.mjs`
（14.8 MB），与 0.3.37 的 `binCli-BIPvs5sY.mjs` 做全量 diff。原始 diff 有 709 个 hunk，
但绝大多数是 pnpm patch hash 与打包分块导致的标识符改号（`format$1 → format$2`）；
归一化后约 45 处真实差异，其中：

| 区域 | 0.3.41 的差异 | 是否协议面 |
| --- | --- | --- |
| `DEEPAI_LANE_HEADERS` | `X-DeepAI-Code-Theos: "6"` → **`"7"`** | **是（唯一一处）** |
| `deepAIBeacon` | 未改；`v=` 仍取打包的 `version$2`（0.3.41） | 否（只是版本号跟着走） |
| `src/deepai/signInDiagnostic.ts` | 新增登录失败分类（dns / tls / proxy / timeout …）与失败 HTML 页 | 否（登录链路） |
| `sessions/save_records` | 新增历史增量快照端点；备份工具输入/输出各截断到 20 KB | 否（云端历史，不进推理） |
| pi 运行时 | 仍 1.3.0；新增 `deepai-diagnostics.mjs`（stderr 崩溃上报）与 Git Bash 探测 | 否 |
| 其它 | Sentry 用户上下文、长会话压缩预算、provider 错误边界、HTTP 观测 | 否 |

端点集合（推理 / 目录 / 配额 / 签到 / 搜索）、5 个 `X-DeepAI-Code-*` 头、
实例号粒度（每进程 `randomBytes(16).hex`）、机器指纹、签到 / beacon 形态、
聊天请求体字段与 `/models` 解码全部未变。

### 66.2 实测：0.3.41 只接受车道 7

单账号、每格一次请求（2026-11-18）：

| 端点 | lane 5 | lane 6 | **lane 7** | lane 8 | lane 9 / 10 |
| --- | --- | --- | --- | --- | --- |
| `GET /deepai_code/models` | 426 | 426 | **200** | 426 | 426 |
| `GET /deepai_code/theos` | 426 | 426 | **200**（`lane_version: 7`） | 426 | 未测 |
| `POST /deepai_code/chat/completions` | 未测 | 426 | **200** | 未测 | 未测 |

与 0.3.37 时代不同，这次旧车道**不再被宽容**：车道 6 的目录 / 配额 / 推理全部 426
`lane_version_unsupported`。也就是说车道 7 是硬门槛，不是“建议值”。

模型目录仍是 **27 个（11 免费 / 16 付费）**，`default` 仍是 `glm-5.3-flash`；
ID、名称、`theosPerRequest`、`maxTokens` 与 11-14 快照完全一致，唯一变化是
`glyph-cluster` 的 `contextWindow` 从 320,000 变成 **256,000**（其余仍 320,000）。

### 66.3 代码同步

- `deepai.LaneVersion` `6` → `7`，`deepai.AppVersion` `0.3.37` → `0.3.41`；
- `types.go` 登记 0.3.41 新增的 `PathSessionsSaveRecords`（`/sessions/save_records`），
  仅用于端点清单，网关仍不实现云端会话同步；
- 测试里的车道锚点：`TestClientSendsRequiredHeaders` 断言 `7`；
  `internal/server/lane_test.go` 把“探测到更新车道”的场景改成 7 → 8，
  “旧状态文件”场景改成真实的 6 → 7 升级；
- 其余模块（请求头、机器指纹、签到、模型解析、协议翻译）无需改代码：
  车道探测从 6 改成 7 即可，候选序列仍是“编译期车道优先 → 向上猜 → 向下兜底”。

### 66.4 验证

```text
gofmt -l ./internal ./cmd ./main.go ./tasks.go ./env.go   无输出
go vet ./...                        无输出
go test ./... -count=1              10 个含测试的包全绿
go test -race ./internal/deepai ./internal/server  全绿
go test ./... -list '^Test' | grep -c  468 个顶层用例（总数仍 616）
```

### 66.5 客户端升级与清理

用官方安装包静默升级本机桌面端：`0.3.37 → 0.3.41`（sha512 与 `latest.yml` 一致）。
安装后 `DeepAI Code.exe` 文件版本 0.3.41、`resources/app.asar` 时间
2026-11-18T12:27Z、`t3codeCommitHash` `937fcf1858f1`；`~/.deepai-code` 里的登录凭据
与设置保留。

清理了旧安装包、解包目录、`deepai-code-desktop-updater` 缓存里的 `installer.exe`，
以及 `/f/tmp` 下残留的 0.3.41 下载/解包产物和旧网关二进制，共约 1.5 GB。

### 66.6 遗留

- 车道 7 的目录 `glyph-cluster` 为何单独回到 256k 未知，网关按上游原样透传，
  上游再变时目录刷新会自动跟上；
- 本机仍无真实进程抓包；下次可直接对比运行中的 `/theos` beacon 与包内代码。

---

## 67. 2026-11-19 巡检：上游放宽车道门槛 + 协议修补（v0.1.8）

**触发**：例行扫描。官方更新源 `latest.yml` 仍是 **0.3.41**（`releaseDate`
2026-11-18T04:27Z，sha512 与 §66 记录一致），本机桌面端也停在 0.3.41 ——
**客户端没有更新**。但用单账号复跑车道矩阵时发现**上游服务端行为变了**。

### 67.1 结论：车道门槛从「只接受 7」放宽为「≤7 全接受」

单账号、每格一次请求（2026-11-19）：

| 端点 | lane 2 | 4 | 5 | 6 | **7** | 8 |
| --- | --- | --- | --- | --- | --- | --- |
| `GET /deepai_code/models` | 200 | 200 | 200 | 200 | **200** | 426 |
| `GET /deepai_code/theos` | 200 | 200 | 200 | 200 | **200** | 426 |
| `POST .../chat/completions` | 未测 | 未测 | 200 | 200 | **200** | 未测 |

与 §66.2（0.3.41 发布当天只有 lane 7 通过）相比，硬门槛只维持了一天。其余观察：

- `/theos` **无论请求发几都回 `lane_version: 7`** —— 这是服务端当前车道，
  不是对请求车道的回显；
- `/models` 仍强制 `Api-Key`（不带 → 401 `auth_required`），未变；
- **车道 5 / 6 / 7 的目录逐字段一致**（27 个模型零差异，`glyph-cluster` 256k、
  其余 320k）：0.3.37 时代的分车道上下文差异已消失；
- 前缀缓存确实生效：同一段 2,687 token 前缀第二次调用 `cached_tokens=2560`；
  且 `prompt_cache_key` 会**分区**缓存（换 key 立即 miss）。

**对网关的影响**：编译期车道 7 仍被接受，不需要改协议面。但两件事必须记下来：

1. 「旧车道一律 426」的安全网没有了 —— 发旧车道也能跑通，所以**请求成功不再
   证明车道是对的**；
2. 车道探测（打 `/models`）从此只能回答「这一档能用」，再也分不出当前车道，
   也发现不了上游已经抬到 8。

### 67.2 缓存用量：三处丢弃点

上游的 `prompt_tokens_details.cached_tokens` 一直都有（**流式末帧也带**，不需要
`stream_options.include_usage`；2026-11-19 实测确认），`ParseUsage` 也解析了，
但三个出口把它丢了：

| 位置 | 问题 | 处理 |
| --- | --- | --- |
| `gateway/responses_translate.go` | `input_tokens_details.cached_tokens` 写死 0 | 改为 `usage.CachedTokens` |
| `server/responses_stream.go` | 同上（流式 `response.completed`） | 同上 |
| `gateway/anthropic_translate.go` | `usage` 只有 input/output，没有缓存字段 | 新增 `AnthropicUsage`：`input_tokens` 只算未命中部分，命中量进 `cache_read_input_tokens`，并补 `cache_creation_input_tokens: 0` |

Anthropic 的语义是 `input_tokens` 为「未命中缓存」的输入、命中量单列，所以这里
对上游的 `prompt_tokens` 做减法，保证 `input + cache_read` 仍等于上下文长度。

### 67.3 Anthropic 流式出错即终止

`streamAnthropic` 出错时先发 `error`，之后**仍然补发 `message_delta` +
`message_stop`**（`stop_reason` 被写成 `end_turn`）。Anthropic 的事件模型里
`error` 是终止事件：多发的 `message_delta` 会把一次中途失败的流派告成正常结束，
客户端可能把只到一半的工具调用当成功。现在出错分支发完 `error` 直接收尾。

### 67.4 车道漂移观测（不再依赖探测）

- `deepai.Client.LaneHook`：`/theos` 成功解析出 `lane_version` 时回调
  （直连与代理桶的 Client 都挂上）；
- `Server.noteUpstreamLane` 记下上游车道，只在数值变化时留痕；
  `UpstreamLane()` / `LaneDrift()` 对外暴露；
- `/health` 增 `lane` / `lane_compiled` / `lane_upstream` / `lane_drift`；
  `/admin/api/overview` 同名字段；`/metrics` 增 `gateway_lane{source=compiled}`
  与 `gateway_upstream_lane`；控制台「运行健康」多一张「上游车道」小卡，
  漂移 > 0 时标黄提示升级；
- **不自动换车道**：lane 8 的请求体形状没有白盒复核过，自动跳等于拿协议面赌。

### 67.5 会话粘性真正落到账号

`affinity`（`X-Session-Affinity` / `prompt_cache_key`）此前只作为请求头透传给上游，
选号仍然只看分组。几百个号的池子里，同一会话的连续请求会落到不同账号上，而前缀
缓存是按前缀分区的（见 §67.1 末条）—— 换号就等于每次都冷启动。

| 层 | 改动 |
| --- | --- |
| `internal/pool` | 新增导出的 `Scheduler.Eligible`（启用 / 未拦截 / 额度未用尽 / 分组匹配），`PickFiltered` 复用它 |
| `internal/store` | 新增 `PreferredAccount(id, group)`：一次读锁 + 单账号拷贝，不可调度时返回 nil |
| `internal/server/sticky.go` | affinity → 账号绑定表（TTL 10 分钟、上限 2048，超限先回收过期再整表清空） |
| `internal/server/accounts.go` | `pickFreeAccount(group, prefer)` 先试绑定号，忙/不可用静默回退 |
| `internal/server/upstream.go` | 只在**首次尝试**用绑定；2xx 后 `bindSticky`；拦截 / 额度用尽时 `dropSticky` |
| `internal/server/request.go` | 新增 `anthropicAffinity`：Claude Code 不发会话头，用「模型 + system + tools」的稳定哈希派生（不含 messages 正文，否则每轮都变） |

粘性永远不是硬约束：绑定号并发已满、被拦、额度用尽、或超过 TTL 都会回退常规轮询。

### 67.6 验证（首轮）

```text
gofmt -l ./internal ./cmd ./main.go ./tasks.go ./env.go   无输出
go vet ./...                                              无输出
go test ./... -count=1                                    11 个含测试的包全绿
go test -race ./internal/server ./internal/store ./internal/pool ./internal/gateway  全绿
node --input-type=module --check（全部 JS） + console-smoke.mjs  通过
顶层用例 468 → 479（+11）
```

新增用例：`TestAnthropicStreamErrorOmitsTerminalEvents`、
`TestResponsesStreamCachedTokens`、`TestAnthropicUsageCarriesCacheTokens`、
`TestChatToAnthropicMessageCachedUsage`、`TestChatToResponsesObjectCachedTokens`、
`TestStickyPickPrefersBoundAccount`、`TestStickyDropsOnBlockAndExpiry`、
`TestStickyTableBounded`、`TestUpstreamLaneDrift`、`TestAnthropicAffinityDerivation`、
`TestPreferredAccountHonorsEligibility`；`TestMetricsEndpointExposition` /
`TestOverviewIncludesRuntimeHealth` 补了车道字段断言。

### 67.7 遗留项收尾（同日）

| 遗留 | 处理 |
| --- | --- |
| lane 8 未白盒 | **已复核到现有证据的尽头**：本机 0.3.41 `server.asar` 里只有一个车道字面量 `"X-DeepAI-Code-Theos": "7"`（无 lane 8 代码路径）；实测 lane 8 在 `/models`、`/theos`、`/chat/completions` 全部 426，伪造 `X-DeepAI-Code-Beacon: v=0.3.42` 或换 `User-Agent` 都不改变。→ lane 8 目前既不存在于客户端也不被上游接受，「漂移只告警、不自动换道」是当前唯一正确策略 |
| affinity 只给 Anthropic 派生 | 推广成 `sessionAffinity`：OpenAI `messages` 与 Responses `instructions` / `input` 两种形状共用同一实现；chat / responses / messages 三条入口全部接上。调用方自己给的头或 `prompt_cache_key` 仍然优先 |
| 绑定表不持久化 | 落盘到旁路文件 `<base>-sticky.json`（与主状态文件同一把主密钥封存）：进程内改动只标脏，后台 30s 与退出前各 flush 一次（无改动跳过），启动时载入并剔除过期项；`-seal` 一并封存。不塞进主文件是为了不把「内容未变则跳过」打穿（`docs/optimization-plan-r2.md` §0.4） |
| `dist/` 残留 v0.1.7 产物 | 已清空（发版脚本打包前还会再清一次） |

### 67.8 验证（收尾）

```text
gofmt -l ./internal ./cmd ./main.go ./tasks.go ./env.go   无输出
go vet ./...                                              无输出
go test ./... -count=1                                    11 个含测试的包全绿
go test -race ./internal/server ./internal/store ./internal/pool ./internal/gateway  全绿
node --input-type=module --check（全部 JS） + console-smoke.mjs  通过
顶层用例 468 → 482（+14）
```

新增用例（共 14）：`TestAnthropicStreamErrorOmitsTerminalEvents`、
`TestResponsesStreamCachedTokens`、`TestAnthropicUsageCarriesCacheTokens`、
`TestChatToAnthropicMessageCachedUsage`、`TestChatToResponsesObjectCachedTokens`、
`TestStickyPickPrefersBoundAccount`、`TestStickyDropsOnBlockAndExpiry`、
`TestStickyTableBounded`、`TestStickyPersistsAcrossRestart`、
`TestStickySkipsExpiredOnLoad`、`TestStickySidecarRoundTrip`、
`TestPreferredAccountHonorsEligibility`、`TestUpstreamLaneDrift`、
`TestSessionAffinityDerivation`；`TestMetricsEndpointExposition` /
`TestOverviewIncludesRuntimeHealth` 补了车道字段断言。

---

## 68. 2026-11-19 issue #5：上下文 / 失忆（v0.1.9）

**触发**：线上 issue #5（`Context/amnesia: conversation history not preserved`）。
按“先证伪、再改代码”的顺序查了四层：客户端源码 → 网关翻译层 → 网关路由 → 上游行为。

### 68.1 结论：网关是（也应该继续是）无状态代理

| 层 | 证据 | 结论 |
| --- | --- | --- |
| 桌面端（0.3.41） | `binCli-DAkBc9bP.mjs` 全文 0 处 `previous_response_id`；4 处 `/chat/completions` 调用都是完整 `messages` 数组 | 官方客户端从不链式续接 |
| 网关 chat / messages | 多轮历史逐条透传，只有「补形状兜底」会裁剪白名单字段（`messages` 始终保留） | 不丢历史（新增 3 个回归用例锁定） |
| 网关 responses（合成） | `previous_response_id` / `conversation` / `item_reference` **被静默忽略** | **真 bug**：客户端以为续接成功，模型只看得到本轮 |
| 网关 responses（透传） | `gpt-6-astra` 原样转发 | 能否续接由上游决定，未验证（见下） |
| 上游 `/responses` | 2026-11-19 多次请求均 429 `rate_limit_exceeded`（单发 / 8s 重试） | 链式续接**无实测证据**，不能写进文档当能力 |

### 68.2 修复：不再静默丢上下文

- `gateway.StatefulParam(payload)` 识别三种真正携带状态的形态（空串 / null
  视为未设置，不误伤习惯性填空的客户端）：`previous_response_id`、`conversation`、
  `input[].type == "item_reference"`；
- `ResponsesToChatPayload` 遇到它们直接返回 `*StatefulParamError`；
- `/v1/responses` 合成路径映射成 **400 `unsupported_parameter`**，错误文案点名字段
  并给出两条出路（把完整历史放进 `input` 重发 / 改用透传模型）；
- 透传路径保持原样转发，但留一条 `WARN` 日志，便于事后判断“是不是上游没续接”；
- 指标：`gateway_stateful_requests_total{param=…}` +
  `gateway_stateful_rejected_total{reason="stateless_translation"}`。

顺带把 `responsesInputMessages` 从 `ResponsesToChatPayload` 里抽出来（复杂度
28 → 23），并补上默认分支的注释：`web_search_call` / `code_interpreter_call` 这类
**输出侧**条目本来就不是对话历史，跳过它们不影响用户可见上下文。

### 68.3 可观测性：区分「没发历史」与「丢了历史」

请求日志新增两个**不含正文**的字段：

| 字段 | 含义 | 怎么用 |
| --- | --- | --- |
| `messages` | 本次回喂给上游的消息条数 | 条数为 1 通常说明调用方只发了本轮（典型是想用 `previous_response_id` 续接） |
| `system` | 是否带 system / developer 提示 | 与「上游要求桌面端形状」的 400 对照看 |

控制台日志页新增「消息」列（`12 · sys` 形式，悬停显示完整说明）。JSONL /
`/admin/api/logs` 里同名字段，便于脚本统计。

### 68.4 验证

```text
gofmt / go vet / go test ./... -count=1 / -race（server,store,pool,gateway）
+ node --check（全部 JS）+ console-smoke   全绿
顶层用例 483 → 492（+9）
```

新增用例：`TestStatefulParam`（8 个子用例，覆盖空值变体）、
`TestResponsesToChatRejectsStateful`、`TestResponsesInputMessagesPreservesHistory`、
`TestResponsesStatefulRejectedOnTranslatedModel`、
`TestResponsesStatefulForwardedOnResponsesModel`、`TestChatMultiTurnHistoryPreserved`、
`TestAnthropicMultiTurnToolsPreserved`、`TestShapeFallbackKeepsHistory`、
`TestHistoryStats`；`TestConsoleBackendParity` 改为断言 `countCell` 契约。

### 68.5 对 issue 的回应要点

1. `/responses` 的 `previous_response_id` 确实没实现 —— 现在显式 400，不再假装续接；
2. 客户端需要发完整历史（这与官方客户端行为一致），`/v1/messages`、`/chat/completions`
   的多轮 + 工具历史逐条透传，已补回归用例；
3. `gpt-6-astra` 的透传路径是否支持链式续接，需要上游恢复可用后实测再定
   （当前 429）；文档已注明“未验证”，不把它当能力承诺。

### 68.6 遗留

- 上游 `/responses` 的 `previous_response_id` / `conversation` 行为待实测
  （需要 `gpt-6-astra` 不再 429）；若确认支持，再把「透传 + 会话粘性同号」写进文档；
- 若将来要支持「合成路径的链式续接」，只能由网关自己存响应 → 回放历史（需要
  持久化 + TTL + 同号路由），本次不做：无状态契约已经能覆盖官方客户端；
- 未知 `input` 条目类型仍按「输出侧条目」跳过（`web_search_call` 等），
  若上游将来用条目承载上下文，需要重新评估。

---

## 69. 2026-11-20 客户端 0.3.42 同步复核：协议面零变更（v0.1.10）

**触发**：例行扫描。官方更新源 `latest.yml` 从 **0.3.41** 跳到 **0.3.42**
（`releaseDate` 2026-11-19T07:44:40Z，比 v0.1.9 的发版时间晚 3 分钟，
也就是说 **v0.1.9 从未对 0.3.42 验证过**）。本机安装的仍是 0.3.41，按惯例下载解包做白盒比对。

### 69.1 结论：这一版只有版本号，协议面完全没动

下载 `DeepAI-Code-0.3.42-x64.exe`（489,954,768 B）→ sha512 与 `latest.yml` 逐字节一致
（`C9vGjoOQ…`）→ 7z 解 NSIS 的 `$PLUGINSDIR/app-64.7z` → `resources/server.asar`
→ `apps/server/dist/binCli-zM20e9Xy.mjs`（14.85 MB），与 0.3.41 的
`binCli-DAkBc9bP.mjs` 全量 diff：**312 个 hunk / 733 增 / 483 删**。

按 `//#region` 把 hunk 归到源模块后，**`src/deepai/*.ts` 协议模块 0 hunk**：

| 复核项 | 0.3.41 | 0.3.42 | 判据 |
| --- | --- | --- | --- |
| `DEEPAI_LANE_HEADERS` | `Theos: "7"` + `Instance: randomBytes(16).hex` | **逐字未动** | 字节级 diff |
| `/deepai_code/*` 端点集合 | 推理 / `/models` / `/theos` / 搜索 / `sessions/list`+`save_records` | **完全一致** | 端点字符串集合 diff |
| 聊天请求体 | `model/messages/stream/store/max_completion_tokens/temperature/tools/…` | **未变**（`src/deepai/api.ts` 0 hunk） | 源码级 |
| 5 个 `X-DeepAI-Code-*` 头 / 机器指纹 | — | **未变**（`machineId.ts` 0 hunk） | 源码级 |
| beacon | `v=${version$2};up=…;pi=…` | 同一函数，`version$2` 现在打包成 **0.3.42** | 源码级 |
| 内置 pi 运行时 | `@earendil-works/pi-ai 1.3.0` | **1.3.0**，`openai-completions.js` 字节级一致 | shasum |
| `previous_response_id` / `item_reference` | 0 / 0 | **0 / 0** | 全文 grep |

312 个 hunk 全在协议面之外，和 release notes 对得上：

| 区域 | hunk 数 | 内容 | 是否协议面 |
| --- | ---: | --- | --- |
| `orchestration-v2/Orchestrator.ts` | 260 | Full access 工具调用不中断、subagent 跨模型委派（「用户点名付费模型才允许」）、`editedByAgent` / `creationSource` 来源标记 | 否 |
| `provider-pi`（`rpc.ts` / `adapter.ts` / `mcpExtensionSource.ts`） | 13 | pi 进程退出码 143 / `0x40010004` 不再报 crash、审批摘要文案、MCP 扩展生成 | 否 |
| `shared/`、`config.ts`、`processRunner.ts`、`vcs/`、`sourceControl/` | ~10 | 日志 / 状态目录 `chmod 0700`、`PROJECT_FOLDER_COMMANDS`（git/gh/glab…）改按绝对路径 spawn | 否 |
| `cloud/*`、`productCapabilities.ts`、`binCli.ts` | 5 | `hostSelfUpdate` / `cliReleaseDownload` 能力开关、`boot-service` 判定、产物名 | 否 |
| `src/http.ts` | 2 | 本地 server 的 Origin 白名单、`wsTicket` / `token` 查询参数不追踪 | 否（本地回环，不是上游） |
| `deepai/DeepAIPiConnection.ts` | 1 | 写 pi 的 `models.json` 时 `apiKey` 改成占位符 `"$T3_DEEPAI_TOKEN"`（由 pi 进程从环境变量取） | 否（客户端内部） |
| `deepai/DeepAIApprovalReview.ts` | 1 | 审批上下文只收真人来源的用户消息 | 否 |
| `RunExecutionService.ts` | 1 | 新增 `EXPECTED_DEEPAI_REFUSAL` 正则，把 `theos_exhausted` / `rate_limit_exceeded` / `lane_version_unsupported` 等**预期上游拒绝**从 Sentry 上报里剔除 | 佐证（见下） |

最后一条值得记：0.3.42 把 `lane_version_unsupported` 明确列为「预期内的上游拒绝」，
说明**旧车道 426 仍是上游的标准拒绝方式**，与 §67 的车道漂移观测口径一致。

### 69.2 决策：只跟心跳版本号，协议逻辑不动

- `deepai.AppVersion` 0.3.41 → **0.3.42**（`/theos` 心跳 `v=` 自报；桌面端同源取自打包版本）；
- `deepai.LaneVersion` 保持 **7**（0.3.42 仍是 `Theos: "7"`，没有抬到 8；
  `check_in` 响应里 `lane_version < 7` 才走 legacy 分支，逻辑也未变）；
- 端点 / 请求头 / 请求体 / 指纹 / 签到形态一律不动 —— 没有证据支持改动，
  改了反而会与桌面端分叉。

### 69.3 issue #5 的连带结论

`docs/recon.md` §2.3 的判据在 0.3.42 上重跑了一遍：仍然是 **0 处 `previous_response_id`**，
`/chat/completions` 的 4 个调用点没动，pi 运行时的 `openai-completions.js` 字节级一致。
→ 「客户端每轮发完整历史、网关是无状态代理」这个契约在 0.3.42 上没有变化，
v0.1.9 的修法（合成路径 400 `unsupported_parameter` + `messages`/`system` 观测）继续有效。
上游 `/responses` 的链式续接**仍无实测**（`gpt-6-astra` 依旧 429），维持「未验证」措辞。

### 69.4 验证

```text
gofmt / go vet / go build ./... / go test ./... -count=1
+ node --check（全部 JS）+ console-smoke
```

改动面只有常量 + 注释 + 文档（无逻辑改动），用例数不变（492）。

### 69.5 遗留

- 0.3.42 起客户端进入「功能迭代」节奏（Full access / subagent），协议面可能继续不动，
  但**每次发版仍要复核 `DEEPAI_LANE_HEADERS` 与端点集合**这两处，其余可只看 hunk 归属；
- 车道 8 仍是黑盒：`/theos` 的 `lane_version` 是目前唯一能发现上游抬车道的信号；
- 上游 `/responses` 链式续接待 `gpt-6-astra` 恢复后实测（同 §68.6）。

---

## 70. 2026-11-20 模型目录复核 + 兜底条目修正（v0.1.11）

**触发**：客户端升到 0.3.42 后顺手核对「文档里的模型表还准不准」。用桌面端凭据直打
`GET https://api.deepai.org/deepai_code/models`（`Api-Key` + `Theos: 7` + `Instance`），
拿到 27 个条目，与 README 的模型表做**逐字段**比对。

### 70.1 结论：文档准确，只有一处代码里的过期值

| 检查项 | 结果 |
| --- | --- |
| 模型数量 / 免费·付费比 | 27 / 11·16 —— 与 README 一致 |
| ID / 名称 / `contextWindow` / `maxTokens` / `theosPerRequest` | 27 条**逐字段零差异**（`default` 仍 `glm-5.3-flash`） |
| 纯文本模型清单 | `deepai-cyber` / `glm-5.3` / `glyph-cluster` / `solar-mini-4` / `minimax-m3` —— 与 README 一致 |
| `api: openai-responses` | 仅 `gpt-6-astra` —— 一致 |
| `theosPerSecond`（按秒计费） | 仍未下发 —— 一致 |
| 字段并集 | 仍是 `id,name,reasoning,input,contextWindow,maxTokens,premier,theosPerRequest,api`，无新增字段 |
| 内置别名数量 | 13 个，全部指向默认免费模型 —— 与 README 一致 |
| **内置兜底条目 `deepai.DefaultModelEntry()`** | ❌ `contextWindow: 1,048,576`，线上是 **320,000** |

只有最后一行是错的。兜底条目用在「首次启动还没拉到目录」与「目录刷新失败」两种情形：
`/v1/models` 此时只回这一条，客户端会按它算上下文预算 —— 写 1M 就是让客户端组一个
远超上游上限（320k）的请求，最后被上游拒。§39 当时只修了 README 的表，漏了代码。

### 70.2 改动

- `internal/deepai/types.go`：`DefaultModelEntry()` 的 `ContextWindow` 1,048,576 → **320,000**，
  补 `TheosPerRequest: "0"`（线上免费），注释写清字段来源日期与「值必须与上游一致」的理由；
- `internal/deepai/types_test.go`：`TestDefaultModelEntry` 从「大于 0」改为**锁死线上值**
  （ctx 320,000 / maxTokens 131,072 / theos `"0"`），下次上游变动会在测试里炸出来；
- `README.md`：模型表补「2026-11-20 复核逐字段零差异」、启动日志示例的「18 个模型」改 27、
  新增一条「内置兜底条目与线上对齐」的说明；
- `docs/recon.md` §2：补 2026-11-20 目录复核段（27 条逐字段一致、字段并集未变）；
- `docs/model-fingerprint-2026-11-10.md`：注明目录自 11-14 起未变，遗漏的 6 个模型范围没变。

### 70.3 验证

```text
gofmt / go vet / go build ./... / go test ./... -count=1
+ node --check（全部 JS）
```

`TestDefaultModelEntry` 改为值断言后仍绿；顶层用例 492 → 492（改断言，不加用例）。

### 70.4 遗留

- `gpt-oss-120b` 仍由客户端主动过滤、网关不展示（上游目录里本来也没有）；
- `crouton-alpha` 已下线，只保留在 §40 / `docs/lessons.md` 的历史记录里；
- 6 个 11-14 新增的模型仍无指纹实测（要花 Theos，非必要不做）。

---

## 71. 2026-11-20 `/v1/responses` 歷史暫存复核：配對、預算、去重與容量

一輪針對「伺服端歷史暫存」（§68 引入、`response_history.go`）的复核。
七處問題全部有可重現的失敗形態，逐條記下根因 → 決策 → 驗證。

### 71.1 根因與決策

| # | 根因 | 失敗形態 | 決策 |
| --- | --- | --- | --- |
| 1 | 裁剪按 item 邊界切，`function_call` 與 `function_call_output` 被分到兩側 | gateway 把 output 翻成 `role:tool`，上游要求它緊跟帶 `tool_calls` 的 assistant → **歷史一被裁，之後每輪 400** | 新增 `dropOrphanToolOutputs`：位置掃描（訊息項即重置 active 集合），不是全局 id 集合——插入的說明訊息同樣打斷鏈路 |
| 2 | 說明訊息自身 token 未計入預算 | 裁剪結果剛好超限 → `replay` 誤判「壓不下去」，把本來能續接的請求回 400 | `avail = budget - itemApproxTokens(marker)`，先扣再選尾段 |
| 3 | `tools` 定義未計入上下文 | 幾十個 function schema 低估幾千 token，剛好超限的請求穿過檢查、由上游靜默截斷 | `fixedTokens = instructions + tools`，錯誤訊息改稱「請求（歷史 + 工具定義）」 |
| 4 | `max_output_tokens` 沒有預留輸出預算 | 歷史貼著上限時上游只能截斷歷史（失憶）或超額 | `total + reserve > limit` 即 400，訊息註明「已為 max_output_tokens 預留 N tokens」；`truncation:auto` 的預算同步扣除 |
| 5 | 客戶端同時送 `previous_response_id` 與完整 `input` 時，基底被拼兩次 | token 直接翻倍，更快撞上限 | `hasHistoryPrefix` 逐項深比較：`input` 以重組出的歷史為前綴就不再補基底（保守：只認「完整重送」，部分重送仍會重複） |
| 6 | `effectiveContextWindow` 每輪重放都 `json.Unmarshal` 一次環境變數 | 純 CPU 浪費在熱路徑上 | 以「原始字串」為 key 的單槽緩存：改環境變數（含 `t.Setenv`）自動失效，不需要重啟 |
| 7 | 每輪都存整條歷史（O(輪數²)），容量用 `json.Marshal` 量體積 | 長對話記憶體平方增長；每次寫入多一次全量序列化 | 只存增量 + `parent` 鏈；每 8 輪存一份檢查點。容量改為結構走訪估算（`jsonBytesOf`，不分配、不序列化） |

第 7 條的取捨寫清楚：

- **記憶體**：鏈長 n 時舊版約 n²/2 份 item，現在約 n 份增量 + n²/(2×8) 份檢查點 ≈ 16 倍改善；
- **韌性**：檢查點就是為了「父條目被淘汰」——`chain` 遇到帶快照的條目即停，不再因更早的祖先被淘汰而判定斷鏈；
- **明確失敗**：鏈上真的缺一環（TTL/淘汰且無檢查點）時回 400 `unsupported_parameter`，**不送半截歷史**——後者正是「靜默失憶」。

### 71.2 順帶修掉的熱路徑問題

- `get()` 的深拷貝移到鎖外：一條 300k token 的歷史 JSON 往返要幾十毫秒，放在全局鎖裡會讓所有 `/v1/responses` 排隊（條目發布後不再改寫，鎖外讀安全）；
- `replay` 只算一遍 `itemCosts` 傳給 `truncateHistoryWithCosts`，不再對同一批 item 重複序列化。

### 71.3 驗證

```text
gofmt -l . ; go vet ./... ; go build ./...
go test -count=1 ./...                       # 12 个包全绿
go test -race -count=1 ./internal/server ./internal/deepai ./internal/store
```

新增 9 个顶层用例，其中 6 个在旧实现上会红：

- `TestTruncateHistoryDropsOrphanToolOutputs`（孤兒 output 被丟 / 成對調用不被誤刪）
- `TestTruncateHistoryMarkerFitsBudget`
- `TestResponsesReplayCountsTools`（tools 挤破窗口 → 400）
- `TestResponsesFullInputWithPrevIDNotDuplicated`（暗號只出現一次）
- `TestResponsesReplayReservesMaxOutputTokens`（400 訊息含 `max_output_tokens`，對照組 200）
- `TestResponsesHistoryChainCompact`（5 輪 × 2k filler：佔用 < 16k，全量存會 ~30k）
- `TestResponsesHistoryCheckpointSurvivesEviction`（摘掉最舊三條仍可續接）
- `TestResponsesHistoryBrokenChainIsExplicit`（鏈斷 → 400 且不上游）
- `TestContextWindowOverridesCached`（同一份環境變數命中緩存，改值即失效）

### 71.4 遗留

- `max_output_tokens` 只在重放路径检查（合成路径目前没有统一的窗口检查点）。
- `remember` 仍对 output 做一次深拷贝（回應物件可能被后续改写），这一份保留。

### 71.5 结构搬迁（PR-1）：`internal/respstate`

按 `docs/server-split-plan.md` 的 PR-1 把整块歷史暫存搬出 `internal/server`：

| 项 | 内容 |
| --- | --- |
| 搬出 | `internal/server/response_history.go`（474 行）→ `internal/respstate/history.go`（774 行，含新逻辑） |
| 导出面 | `Store` / `Turn` / `Outcome`（`OK` `NotFound` `TooLong`）/ `New` / `NewFromEnv` / `Remember` / `Replay` / `Len` / `InputItems` / `EffectiveContextWindow` / `ItemApproxTokens` |
| 依赖方向 | `respstate → gateway`（仅取值工具），**不依赖 `internal/server`**，可脱离 HTTP 单测 |
| 测试拆分 | 单元用例（裁剪 / 估算 / 窗口 / 容量 / 链与检查点）搬进 `internal/respstate/history_test.go`（15 个）；HTTP 端到端留在 `internal/server`（11 个） |
| 内核变化 | `Server.history *respstate.Store`；`responses.go` 只改 6 处调用；`accounts.go` 无改动 |

搬迁时顺手删掉两个因重构而**再无生产调用**的符号：`Store.get`（`Replay` 改走 `chain` 后无人使用）
与 `truncateHistory` 薄包装（合并进 `truncateHistoryWithCosts`），避免留下“只有测试在用”的死代码。

数字：`internal/server` 非测试代码 8,008 → **7,557** 行（文件 40 → 39）；
`internal/respstate` 774 + 525 行测试。

### 71.6 部分重送的去重（后缀重叠）

上一轮只处理“完整重送”：客户只带**歷史尾段** + `previous_response_id` 时仍会重复。
现在改为按**最大后缀重叠**去重（`historyOverlap`）：

- 重疊必須貼住 base 末尾（`k = len(base)-j` 且 `k <= len(in)`），因此不会把「新回合里重复的同一句话」误判为重送：
  base 末项是上一轮助手输出、in 首项是 user 讯息，`k=1` 误配窗口极窄；
- 由前往后扫，第一个命中即最大重疊（完整重送 `k=len(base)` 与前缀判定等价）；
- 成本两道限：首项比对最多走一遍 base，整段验证最多 16 次；
- 合并式统一为 `base[:len(base)-k] + in`（`k=0` 时恰好是完整基底）。

用例：`TestHistoryOverlap`（完整/部分/新回合/重复文字不误判）、
`TestHistoryReplayDedupesResentHistory`（三种输入都得到 5 项，而不是 7 项）、
`TestResponsesPartialInputWithPrevIDNotDuplicated`（HTTP 级：暗号保留且 “turn 2” 只出现一次）。

---

## 72. 2026-11-20 结构治理 PR-2：抽出 `internal/guard`（限流 / 口令退避 / CORS）

按 `docs/server-split-plan.md` 的 PR-2 落地。与 §71 不同，这一条不是修 bug，
是**把准入层从 `internal/server` 里切出去**——它是包内少数「只依赖 Store 设置 +
标准库」的接缝，也是唯一能被单独压测/单测的安全层。

### 72.1 边界与导出面

| 新包文件 | 行 | 导出 | 来源 |
| --- | --- | --- | --- |
| `guard/limiter.go` | 193 | `Limiter` / `NewLimiter` / `Allow` / `Gate` / `RateConfig` / `KeyID` / `WriteRateLimited` | `server/ratelimit.go` |
| `guard/adminauth.go` | 142 | `AdminGuard` / `NewAdminGuard` / `Blocked` / `Failure` / `Success` / `BackoffPolicy` / `DefaultBackoffPolicy` / `ClientIP` / `FreeAttempts` | `server/authguard.go` |
| `guard/cors.go` | 91 | `Origin` / `InAllowList` / `WriteHeaders` / `IsLoopbackAddr` | `server/cors.go` |

判定逻辑全部变成**纯函数/纯内存态**：设置由调用方传入（`RateConfig`、`configured`
字符串、`BackoffPolicy`），包内不读 `store.Store`，也不碰路由表。

### 72.2 保留的胶水（server 侧 104 行）

| 文件 | 行 | 职责 |
| --- | --- | --- |
| `server/ratelimit.go` | 21 | `rateLimitGate(key)`：把设置翻译成 `guard.RateConfig` 后调 `Limiter.Gate` |
| `server/authguard.go` | 57 | `adminGuardPolicy`（注入 `adminGuardBase`）/ `checkAdminThrottle`（429 + `Retry-After`）/ 两个 `noteAdminAuth*`（失败日志） |
| `server/cors.go` | 26 | `withCORS` 中间件：取设置 → `guard.Origin` → `guard.WriteHeaders` |

顺带消掉一处重复实现：`adminSameOrigin` 里手写的「逗号分隔允许列表 + 丢通配符」循环
与 `corsOrigin` 里的完全同构，现在统一走 `guard.InAllowList`。
`observability.go` 的访问日志把 `clientIP` 换成 `guard.ClientIP`。

**行为零变更**：CORS 响应头顺序与取值、429 文案与 `Retry-After`（≥1s）、退避曲线
（5 次免费 → base×2ⁿ 封顶 30s）、`adminGuardBase` 测试注入点全部保持原样。

### 72.3 测试迁移

- 搬进 `internal/guard`：`TestTokenBucketRefill`、`TestTokenBucketSweepsIdleEntries`
  （改造为公开 API：`NewLimiter` / `Allow`），另新增 11 个单测 —— 令牌桶补充、
  `Gate` 并发闸门、429 头与响应体、退避曲线与封顶、按 IP 隔离、`BackoffPolicy`
  归一化、`ClientIP`、CORS 默认/列表/通配符/响应头、回环判定；
- **原地不动**：`TestRateLimitPerKey`、`TestKeyConcurrencyLimit`、`TestRateLimitSettingsRoundTrip`、
  `TestAdminPasswordThrottle*`、`TestCORSDefaultsToLoopback` —— 它们是 HTTP 级行为门槛，
  拆包不该顺带改断言。

### 72.4 验证与数字

```text
gofmt -l . ; go vet ./... ; go build ./...
go test -count=1 ./...                    # 12 个包全绿（新增 internal/guard）
go test -race -count=1 ./internal/guard ./internal/server
```

| 指标 | 变化 |
| --- | --- |
| `internal/server` 非测试 | 7,557 → **7,226** 行（-331） |
| `internal/server` 文件 | 39（删 3 个，新增 3 个胶水） |
| `internal/guard` | 426 行 + 311 行测试（13 个用例） |
| 反向依赖 | 无（`go list -deps ./internal/guard` 不含 `internal/server`） |

### 72.5 遗留

- `internal/httpx` 直接依赖 `internal/store`（为读设置？），所以 `guard` 的**传递**依赖里
  仍然会出现 `store`/`pool`；直接依赖只有 `deepai` + `httpx`。这层隐式依赖值得单独收一次。
- 下一步按计划是 **PR-3 `internal/observe`**（`metrics.go` + `observability.go`，688 行），
  前置动作是先把 `withObservability` / `handleMetrics` / `poolSnapshot` 改成窄接口。

---

## 73. 2026-11-20 结构治理 PR-3：抽出 `internal/observe`（指标 / 中间件）

按 `docs/server-split-plan.md` 的 PR-3 落地。原计划的前置动作是「把
`withObservability` / `handleMetrics` / `poolSnapshot` 改成窄接口」，实际落地时
发现**比接口更窄的做法**就够：

- `Middleware(m *Metrics, next http.Handler)` —— 中间件只依赖 `*Metrics`，连 Server 都不需要；
- `handleMetrics` 需要的号池/日志/车道数字用**值结构** `PromExtra` 传进去，
  比定义 `LaneSource` / `PoolSnapshotSource` 两个接口更窄（不用 mock、不暴露方法集）；
- `poolSnapshot()` 仍留在 server（要 `store` + 额度策略），返回 `observe.PoolSnapshot`。

### 73.1 边界与文件

| 新包文件 | 行 | 内容 |
| --- | --- | --- |
| `observe/metrics.go` | 379 | `Metrics`（计数器 + 定桶直方图）、`Snapshot` / `MetricsSnapshot`、`HistogramView`、`QuantileMs` / `LatencySummary`、`EndpointLabel` |
| `observe/exposition.go` | 155 | `WritePrometheus`（Prometheus 文本格式）、`PromExtra`、`PoolSnapshot`、直方图序列化 |
| `observe/middleware.go` | 146 | request id、访问日志、`Middleware`、`IsStaticAsset` |

server 只剩 **80 行**装配：`poolSnapshot()`（返回 `observe.PoolSnapshot`）与
`handleMetrics`（方法校验 + 组装 `PromExtra` + 写响应头）。

**直接依赖只有标准库 + `internal/guard`（取来源 IP）+ `internal/logx`**：
`observe` 不认识 store / 号池 / 协议层，`go list -deps` 确认无反向依赖。

### 73.2 顺带清掉的死代码

`metrics.upstreamLane` 是「只在 `noteUpstreamLane` 里写、从未被读」的镜像：
`/metrics` 的 `gateway_upstream_lane` 一直取的是 `Server.UpstreamLane()`（权威值）。
搬迁时删掉这个字段与 `SetUpstreamLane`，`/metrics` 改由 `PromExtra.UpstreamLane`
取值——口径不变，少一份会漂移的重复状态。

### 73.3 测试迁移

- 搬进 `internal/observe`：`TestHistQuantile`、`TestMetricsRecordsExtras`、
  `TestStaticAssetsSkipAccessLog`（改名为 `TestIsStaticAsset`）；
- 新增：`TestMetricsStatefulAndCheckIn`（状态字段与签到口径）、
  `TestWritePrometheusContract`（指标名 / 标签 / `+Inf` 桶 / 取值，逐条锁定）、
  `TestEndpointLabel`；
- server 侧：`s.metrics.statefulReject.Load()` 这类**内部字段访问**改成
  `s.metrics.Snapshot().Stateful["rejected"]`；签到计数改用 `ObserveCheckIn` 驱动；
  测试里用的 `itoa64` 换成 `strconv.Itoa`。

### 73.4 验证与数字

```text
gofmt -l . ; go vet ./... ; go build ./...
go test -count=1 ./...                       # 13 个包全绿（新增 internal/observe）
go test -race -count=1 ./internal/observe ./internal/server
```

| 指标 | 变化 |
| --- | --- |
| `internal/server` 非测试 | 7,226 → **6,607** 行（-619） |
| `internal/server` 文件 | 39 → **38**（`metrics.go` 整文件删除，`observability.go` 变装配层） |
| `internal/observe` | 680 行 + 178 行测试（6 个用例） |

### 73.5 遗留

- `handleMetrics` 仍直接读 `s.Store.Stats()` / `s.Store.Keys()`：这三个数字很小，
  留在装配层比再抽一层值类型更划算。
- 下一步是 **PR-4 `internal/poolsvc`**（`probe.go` / `enroll.go` / `sticky.go` /
  `proxypool.go` / `accounts.go` / `warmup.go`，约 1,012 行），它要搬并发原语，
  以 `-race` 为硬门槛。

---

## 74. 2026-11-20 结构治理 PR-4：抽出 `internal/poolsvc`（号池运行时）

按 `docs/server-split-plan.md` 的 PR-4 落地，这是四步里唯一搬运**并发原语**的一步。

### 74.1 边界：谁搬走、谁留下

| 搬进 `internal/poolsvc` | 行 | 留在 `internal/server` |
| --- | --- | --- |
| `slots.go` 在途槽位（in-flight / Pick / 响应体归还） | 133 | `pool_errors.go`：`errAllAccountsBusy` / `blockedReason` / `noAccountError` —— 这是**响应文案**，不是调度 |
| `probe.go` 配额探测（去重 / 批量 / 按需 / 后台守护） | 396 | `quotaStaleAfter` 常量（调度与探测共用的阈值口径） |
| `enroll.go` 入池 / 邮箱验证 / 0 额度复探 | 158 | `holdResponseBody`（HTTP 空闲超时） |
| `sticky.go` 会话粘性（affinity → 账号） | 154 | `clientFor`（按代理缓存的客户端工厂，**注入**给 poolsvc） |
| `proxypool.go` 入池代理池 | 38 | `lifecycle.Close`（编排：停后台 → flush 绑定 → 关空闲连接） |
| `warmup.go` 启动预热 | 76 | — |
| `poolsvc.go` Service / Config / Close / Wait | 168 | — |

`Service` 的注入面只有三项：

```go
type Config struct {
    Store     *store.Store
    ClientFor func(*deepai.Account) *deepai.Client // 含代理缓存
    Save      func() error                         // 落盘（server 决定日志策略）
}
```

原计划里 `Pick` 之类的「能力函数」最终没做成回调——它们本来就是 poolsvc 自己的
状态操作，做了反而是把内部逻辑倒着注入回去。

### 74.2 状态迁移

`Server` 从 36 个扁平字段降到 **23 个**：`inflightMu/inflight`、`probeMu/probing/probeSem`、
`quotaWG`、`stopCh/stopOnce`、`stickyMu/sticky/stickyDirty`、`quotaReprobeDelays`、
`enrollPoolOnce/enrollPool` 全部搬进 `Service`，`Server` 只保留 `Pool *poolsvc.Service`。

顺带把 `sleepOrStop` / `stopped()` 一起搬走（它们只服务入池复探的退避睡眠），
`Server.Close` 变成三步编排：`Pool.Close(ctx)` → `Pool.FlushSticky()` → `closeIdleConnections()`。

### 74.3 对外可见的注入点

| 符号 | 用途 |
| --- | --- |
| `QuotaGuardInterval` / `QuotaGuardBatch` | 后台守护节奏（`tasks.go` 直接用） |
| `GuardStaleAfter` / `MaxCheckinBatch` | 过旧阈值 / 单批探测上限 |
| `MaxInFlightPerAccount` / `StickyMax` / `StickyTTL` | 容量与 TTL（控制台与测试引用） |
| `NeedsRefresh` / `SortByDue` / `PositiveQuota` / `AllowanceFromEnroll` | 纯函数，`/health`、overview 与入池共用同一口径 |
| `Probing()` / `InFlight()` / `Bindings()` / `Wait()` | 观测与测试收尾（替代直接摸 `probeMu` / `quotaWG`） |
| `SetQuotaReprobeDelays` | 测试注入毫秒级退避；排查时可临时关掉入池后复探 |

### 74.4 测试迁移

- 机械替换：`s.pickFreeAccount/acquireAccount/releaseAccount/stickyAccount/bindSticky/
  dropSticky/FlushSticky/probeBatch/probeAsync/claimProbe/Wait…` → `s.Pool.*`；
- **内部字段访问**改成公开观测口：`len(s.probing)` → `s.Pool.Probing()`、
  `s.inflightMu/inflight` → `s.Pool.InFlight()`、`s.StickyBindings()` → `s.Pool.Bindings()`；
- 白盒用例搬进 `internal/poolsvc/poolsvc_test.go`：`TestCloseWaitsForProbes`（用
  `quotaWG` 登记一笔假工作）、`TestCloseInterruptsBackoff`（1 小时退避被打断）、
  `TestStickyExpired`（过期绑定顺手清除）；
- 「载入时剔除过期绑定」仍留在 server（它通过旁路文件构造数据），改用
  `poolsvc.StickyTTL` 生成过期时间戳。

### 74.5 验证与数字

```text
gofmt -l . ; go vet ./... ; go build ./...
go test -count=1 ./...                     # 14 个包全绿（新增 internal/poolsvc）
go test -race -count=1 ./internal/poolsvc ./internal/server
```

| 指标 | 变化 |
| --- | --- |
| `internal/server` 非测试 | 6,607 → **5,610** 行（-997） |
| `internal/server` 文件 | 38 → **33**（6 个整文件搬走，新增 1 个错误文案文件） |
| `Server` 字段 | 36 → **23** |
| `internal/poolsvc` | 1,123 行 + 68 行白盒测试 |
| 反向依赖 | 无（`go list -deps ./internal/poolsvc` 不含 `internal/server`） |

### 74.6 遗留

- `admissionWait`（全忙排队上限）仍在 server：它和 HTTP 超时预算绑在一起，属准入策略。
- `clientFor` 的代理缓存仍在 server：poolsvc 只需要一个「按出口取客户端」的函数值。
- PR-5（`relay` / `adminapi`）按计划**暂缓**：拆完的收益主要是包边界好看，
  代价是给 handler 造一个 30+ 字段的 Config。

## 75. 2026-11-20 `/admin/api/state` 三层退役收尾（v0.1.12）

### 75.1 事实基线

`GET /admin/api/state` 曾在一次响应里拍平下发全部账号（603 号约 430KB），
控制台早已改用 `/admin/api/overview`（元信息）+ `/admin/api/accounts`（分页），
前端对它 **0 引用**。响应头自 v0.1.4 起就带 `Deprecation` / `Sunset` /
`Link(successor-version)`，Sunset 日期定为 **2026-05-20**。

本次复核时点 2026-11-20：Sunset 已过去 6 个月，具备走完 RFC 8594 剩余两步的条件。

### 75.2 三个阶段

| 阶段 | 动作 | 目的 |
| --- | --- | --- |
| P1 | 10 处「借端点」的测试改走替代端点；新增 `gateway_deprecated_endpoint_total{path="/admin/api/state"}`；README 补「已弃用的端点」迁移小节 | 让 P3 变成纯删除，并给「还有没有人在用」一个可查询的信号 |
| P2 | 处理器改为 `410 Gone` + 保留 `Sunset`/`Link` 头 + `type=endpoint_removed` 错误信封 | 明确失败而不是静默继续；404 会被误认为拼错路径 |
| P3 | 删除路由 / 处理器 / `stateSunset` / 410 契约用例 / 弃用计数指标；README 改为「已下线」 | 彻底移除 |

### 75.3 为什么 P2 用 410 而不是继续返回数据

继续返回数据会让调用方一直不知道需要迁移，而该端点的成本随号池规模线性增长
（429KB / 603 号，且是同步深拷贝全池）。410 同时说明了「曾经存在」与
「迁移去哪」，比 404 更可行动。

### 75.4 遗留

- `docs/optimization-plan-r2.md` §P0-2 仍按「旧端点保留兼容」描述，属历史记录，
  保留原文（该文记录的是当时的决策）。
- `internal/server/admin_api.go` 的包注释仍提到拆分的动机是「state 一次下发
  全部账号」，属历史说明，保留。

## 76. 2026-11-20 客户端二次复核 + README 过期提示清理（v0.1.12）

### 76.1 复核结论：客户端没有更新

| 复核项 | 值 | 与同日上一轮（0.3.42 复核） |
| --- | --- | --- |
| `latest.yml` `version` | **0.3.42** | 一致 |
| `latest.yml` `releaseDate` | 2026-11-19T07:44:40.553Z | 一致 |
| x64 `sha512` / `size` | `C9vGjoOQ…` / 489,954,768 B | 一致 |
| arm64 `sha512` / `size` | `hry2CPlP…` / 469,900,760 B | 一致（本轮补记 arm64） |
| 本机 `ProductVersion` | 0.3.42.0 | 一致 |

更新源地址来自本机客户端 `resources/app-update.yml`（`provider: generic` →
`https://downloads.deepai.org/deepai-code/releases`）。判据只有 `latest.yml`：
下载页与更新源会不同步（2026-11-10 那次下载页还指着 0.3.30，更新源已是 0.3.32）。

→ `deepai.LaneVersion` 保持 `"7"`、`deepai.AppVersion` 保持 `"0.3.42"`，**代码零改动**。
需要留意的仍是车道：门槛是服务端行为，`/models` 探测已经分辨不出当前车道
（2026-11-19 起 ≤7 全通过），只能看 `/theos` 回的 `lane_version`。

### 76.2 为什么删掉 README 简介下方那段 ℹ️

原文提示 `0.3.19` 曾把推理收窄到 Google/Apple 登录的账号。它记的是**已经被推翻的
临时告警**（2026-11-09 复测解除），却摆在项目简介正下方 —— 读者第一眼看到的是
一个过期异常，而不是网关能做什么。删除不丢信息，事实与处置仍有两处落点：

- `docs/recon.md` §4.1 —— 原始观察、复现命令与解除复测；
- README §1.1「账号策略」行与 §七「已知限制」—— 保留 `403 social_login_required`
  作为运行时行为（网关仍把它当 `blocked` 终态，策略再收紧无需改代码）。

### 76.3 顺带修掉的陈旧索引

README §十一「文档索引」里 `docs/recon.md` 的维护状态写着「最近复核 2026-11-14」，
实际已复核到 2026-11-20（0.3.42 两轮），本轮同步更新；同日把索引表与实际条目
对齐（补 71–76 行与 bullet 列表里的 69–76）。

---

## 77. 2026-11-21 客户端 0.3.43 同步复核：协议面零变更（未发版）

**触发**：例行扫描。官方 `latest.yml` 从 **0.3.42** 跳到 **0.3.43**
（`releaseDate` 2026-11-20T18:52:38Z）—— 比 §76 那次「客户端没有更新」的复核
晚约 19 小时，所以 §76 的结论当时是对的，只是窗口没覆盖到这次发布。

### 77.1 版本核实

| 复核项 | 0.3.42 | 0.3.43 |
| --- | --- | --- |
| `latest.yml` `version` | 0.3.42 | **0.3.43** |
| `releaseDate` | 2026-11-19T07:44Z | **2026-11-20T18:52:38Z** |
| x64 `sha512` / `size` | `C9vGjoOQ…` / 489,954,768 B | `BTu5a5c3…` / **490,140,336 B** |
| arm64 `sha512` / `size` | `hry2CPlP…` / 469,900,760 B | `Ss/jMa5S…` / 470,082,840 B |
| `app.asar` `package.json` | 0.3.42 | **0.3.43**（`t3codeCommitHash: 1dfeffe45dee`） |
| 本机 `ProductVersion` | 0.3.42.0 | 复核时 0.3.42.0（作白盒基线）；复核后已升级到 **0.3.43.0** |

下载 `DeepAI-Code-0.3.43-x64.exe`（490,140,336 B）→ sha512 与 `latest.yml` 逐字节一致
→ 7z 解 NSIS 的 `$PLUGINSDIR/app-64.7z` → `resources/{app,server}.asar`。

### 77.2 结论：协议面仍然零变更，只跟心跳版本号

`server.asar` → `apps/server/dist/binCli-Cr9pebKe.mjs`（14,868,756 B，0.3.42 是
`binCli-zM20e9Xy.mjs` 14,847,501 B），全量 diff **410 hunk / +6430 / −5896**；
按 `//#region` 归属到 1752 个源模块 → **141 个变、1601 个字节级不变**。

| 复核项 | 0.3.42 | 0.3.43 | 判据 |
| --- | --- | --- | --- |
| `DEEPAI_LANE_HEADERS` | `Theos: "7"` + `Instance: randomBytes(16).hex` | **逐字未动** | 字节级 |
| `/deepai_code/*` 端点集合 | 推理 / `/models` / `/theos` / 搜索 / `sessions/*` | **完全一致** | 字面量集合 diff |
| `X-*` 请求头全集 | 47 个 | **集合零差异** | 字面量集合 diff |
| 聊天请求体 | `model/messages/stream/store/max_completion_tokens/…` | **未变** | `api.ts` 仅新增注释 + 错误码助手 |
| beacon `v=` | `v=${version$2};up=…;pi=…;focus=…` | **同一逻辑**，`version$2` 现在报 0.3.43 | 源码级 |
| `previous_response_id` / `item_reference` | 0 / 0 | **0 / 0** | 全库 grep |
| 机器指纹 `machineId.ts` | — | **字节级相同** | region hash |
| 内置 pi 运行时 | `pi-ai 1.3.0` | **1.3.0**，`dist/api/openai-completions.js` sha256 一致 | 解包 `node_modules` |
| `src/provider/model-manifest.json` | — | **字节级相同**（18,849 B） | region hash |

`src/deepai/*` 51 个模块里只有 8 个动：

- **纯压缩器重命名**（无语义）：`DeepAIApprovalReview.ts`（`layer$137→$136`）、
  `DeepAIOptionalProvenance.ts`、`DeepAIPiConnection.ts`、`DeepAIPeerCollaboration.ts`；
- `api.ts`：新增 **ToS 声明注释** + `deepAIFailureCode()`（错误码归因，只进 Sentry
  tag，不上行）；
- `DeepAIAccount.ts`：模型目录缓存进 secret store（`deepai-model-catalog`，按
  `accountId` 校验后再用）、`refreshModels` 前显式 `await deepAIMachineId()`、
  目录刷新与 `/theos` 失败首次上报 Sentry（各只报一次）；
- `localTransport.ts` / `modelTransportPolicy.ts`：本地回环错误体新增 `code`
  （`credentials_unreadable` / `request_unreadable` / `watchdog_idle` /
  `watchdog_total` / `check_in_unavailable`）。

其余 133 个变更模块在协议面之外：orchestration（`ProviderSessionManager`、
`ProviderEventIngestor`、`RunExecutionService`）、ACP / opencode / cursor / pi
provider、PR 与 source control、终端、`auth/http.ts` 的 500 上报，以及
`src/cloud/*` —— 对应 release notes「Fixed connecting to DeepAI cloud workspaces
and remote servers」。

→ 代码只需要把 `deepai.AppVersion` 跟到 **0.3.43**（心跳自报），`LaneVersion`
保持 `"7"`，协议逻辑、请求体、header 集合都不动。

### 77.3 顺带登记：上游开始把 ToS 写进代码

0.3.43 的 `src/deepai/api.ts` 在 `DEEPAI_LANE_HEADERS` 正上方加了注释，援引
DeepAI ToS 第 4 条，明写「这些头与签到是 DeepAI Code 应用自识别用，不允许复制进
其它客户端」。这是**文本声明、不改任何行为**，但要留档：

- 它解释的正是本项目的存在前提（复用本机已登录客户端的链路，面向自己的账号）；
- README §十二 License 已引用该声明，提醒再分发 / 代运营前先确认许可；
- 若后续版本把这类声明升级成行为（例如动态头、更强的指纹或服务端校验），
  应视为协议面变更重做复核，而不是当作文案变化略过。

### 77.4 本轮改动

- `internal/model/types.go`：`AppVersion` 0.3.42 → **0.3.43**，车道注释补 0.3.43 复核；
- 注释版本号同步：`internal/deepai/client_test.go`、`internal/machine/machine.go`、
  `internal/machine/machine_test.go`、`internal/gateway/chatbody.go`；
- `README.md`：应用版本 / 车道复核日期 / 心跳 `v=` / 文档索引；
- `docs/recon.md`：顶部新增 2026-11-21 复核条目，§1 / §2 / §3 / §5.1 / §5.3 的
  【源码级】标签升到 0.3.43，§3 登记 ToS 声明；

### 77.5 本机升级与清理

复核完成后用同一个官方安装包静默升级本机客户端（`DeepAI-Code-0.3.43-x64.exe /S`）：
退出码 0，`DeepAI Code.exe` 的 `ProductVersion` 变为 **0.3.43.0**、`FileVersion` 0.3.43，
`resources/{app,server}.asar` 与安装包内解包产物 sha256 逐字节一致（`server.asar`
`58c3e13a…` / `app.asar` `6271c2eb…`），`server.asar` 内仍是 `binCli-Cr9pebKe.mjs`，
卸载注册项从 `DeepAI Code 0.3.42` 变为 `0.3.43`（单条，无重复项）。

安装目录文件全部被替换（无 0.3.42 时间戳残留，总 1.4 GB），随后清理：
安装包与解包产物（2.3 GB）、空的 updater 缓存目录、上一轮留在 `/f/tmp/asar` 的旧版
`binCli.mjs`（15 MB），以及本轮测试产生的 `console-smoke-*` / `TestAssignMachine*` /
`go-build*` 临时目录。用户数据 `~/.deepai-code`（65 MB）与 Electron 缓存保持不动。

---

## 78. 2026-11-22 客户端 0.3.44 同步复核：协议面零变更（未发版）

**触发**：例行扫描。官方 `latest.yml` 从 **0.3.43** 跳到 **0.3.44**
（`releaseDate` 2026-11-21T22:31:45.367Z），比 §77 那次 0.3.43 复核晚约 28 小时。

### 78.1 版本核实

| 复核项 | 0.3.43 | 0.3.44 |
| --- | --- | --- |
| `latest.yml` `version` | 0.3.43 | **0.3.44** |
| `releaseDate` | 2026-11-20T18:52Z | **2026-11-21T22:31:45Z** |
| x64 `sha512` / `size` | `BTu5a5c3…` / 490,140,336 B | `haSxCvh0…` / **490,345,232 B** |
| arm64 `sha512` / `size` | `Ss/jMa5S…` / 470,082,840 B | `ZGe5HxI2…` / **470,274,560 B** |
| `app.asar` `package.json` | 0.3.43 | **0.3.44**（`t3codeCommitHash: e8800767e0c4`） |
| 本机 `ProductVersion` | 0.3.43.0 | 复核时 0.3.43.0；复核后已升级到 **0.3.44.0** |

下载 `DeepAI-Code-0.3.44-x64.exe`（490,345,232 B）→ sha512 与 `latest.yml` 逐字节一致
→ 7z 解 NSIS 的 `$PLUGINSDIR/app-64.7z` → `resources/{app,server}.asar`。

release notes 报的是「Windows / Linux 启动可靠性、慢网下的模型目录刷新、拖拽代码
选择外出崩溃、重复布局更新导致卡顿」四项，全部在客户端内部。

### 78.2 结论：协议面仍然零变更，只跟心跳版本号

`server.asar` → `apps/server/dist/binCli-B9W8M09_.mjs`（14,870,582 B，0.3.43 是
`binCli-Cr9pebKe.mjs` 14,868,756 B），全量 diff 只有 **13 hunk / +65 / −22**；
按 `//#region` 归属到 1751 个源模块 → **9 个变、1742 个字节级不变**。

| 复核项 | 0.3.43 | 0.3.44 | 判据 |
| --- | --- | --- | --- |
| `DEEPAI_LANE_HEADERS` | `Theos: "7"` + `Instance: randomBytes(16).hex` | **逐字未动** | 字节级 |
| `/deepai_code/*` 端点集合 | 推理 / `/models` / `/theos` / 搜索 / `sessions/*` | **完全一致** | 字面量集合 diff |
| `X-*` 头字面量 | 14 个（含 5 个 `X-DeepAI-Code-*`） | **集合零差异** | 字面量集合 diff |
| 聊天请求体 | `model/messages/stream/store/max_completion_tokens/…` | **未变** | `api.ts` 只动错误码归因 |
| beacon `v=` | `v=${version$2};up=…;pi=…;focus=…` | **同一逻辑**，`version$2` 现在报 0.3.44 | 源码级 |
| `previous_response_id` / `item_reference` | 0 / 0 | **0 / 0** | 全库 grep |
| 机器指纹 | — | **算法未变**（`machineId.ts` 只加了启动失败上报） | region hash |
| 内置 pi 运行时 | `pi-ai 1.3.0` | **1.3.0**，`dist/api/openai-completions.js` sha256 一致（`709e0a92…`） | 解包 `node_modules` |

### 78.3 9 个变更区域逐个过

`src/deepai/*` 里 3 个，全在协议面外：

- `machineId.ts`：`run()` 把 `execFile` 包进 `try/catch`，同步抛错时首次上报 Sentry
  （`operation: deepai.machine_id`，`source` / `code` 标签都按正则白名单截断）并
  `resolve("")`；`detect()` 的分支与 Linux `parseLinuxMachineId` 校验**一字未改**；
- `api.ts`：`reportError` 增加 `diagnosticErrorCodes(failure)` 归到 `error_codes` 标签
  （只进 Sentry tag，不上行）；`createRequire` 由具名导入改成 `NodeModule.createRequire`；
- `DeepAIAccount.ts`：`/models` 超时 **5 s → 30 s**（对应「slow connections」）；
  目录写 secret store 失败时上报 `deepai.models_save`（`code` / `syscall` 白名单标签）
  并把 `savedCatalogText` 回滚，避免下次误判「已存过」而跳过重写。

其余 6 个：environment id 落盘在 `EISDIR/ENOTSUP/EOPNOTSUPP` 时回落
`writeFileString(flag:"wx")`（上报 `environment.id_no_hard_link`）；`failureClasses()`
加环检测（`seen` 集合，上限 24 层）；pi `managed-refresh` 错误标签补 `code` / `syscall`；
`catchTag("PiRpcTimeoutError")` → `catchTags({PiRpcTimeoutError})`（修长任务下
`get_state` 超时未被捕获）；Sentry debugId 更新；依赖 chunk 重命名带来的导入顺序变化。

→ 代码只需要把 `deepai.AppVersion` 跟到 **0.3.44**，`LaneVersion` 保持 `"7"`，
协议逻辑、请求头、请求体与指纹都不动。

### 78.4 本轮改动

- `internal/model/types.go`：`AppVersion` 0.3.43 → **0.3.44**，车道注释补 0.3.44 复核；
- 注释版本号同步：`internal/deepai/client_test.go`、`internal/machine/machine.go`、
  `internal/machine/machine_test.go`、`internal/gateway/chatbody.go`；
- `README.md`：应用版本 / 版本链 / 车道复核日期 / 心跳 `v=`；
- `docs/recon.md`：顶部新增 2026-11-22 复核条目，§1 / §2 / §2.3 / §3 / §5.1 / §5.3 的
  【源码级】标签升到 0.3.44，车道路由表与 `v=0.3.44` 心跳示例同步。

### 78.5 本机升级与清理

用同一个官方安装包静默升级（`DeepAI-Code-0.3.44-x64.exe /S`）：退出码 0，
`DeepAI Code.exe` 的 `ProductVersion` 变为 **0.3.44.0**、`FileVersion` 0.3.44，
`resources/{app,server}.asar` 与安装包内解包产物 sha256 逐字节一致（`app.asar`
`596988b0…` / `server.asar` `b32eb1c2…`），`server.asar` 内为 `binCli-B9W8M09_.mjs`，
卸载注册项从 `DeepAI Code 0.3.43` 变为 `0.3.44`（单条，无重复项）。

安装目录总 1.29 GB，除 `resources/git/etc/{hosts,networks,protocols,services}`
四个 git 自带文件外没有早于本次构建的残留时间戳。随后清理：安装包（490 MB）、
NSIS 解包目录（468 MB）、asar 解包与两版 `binCli`（1.4 GB + 30 MB，含 diff 中间文件）、
`deepai-code-desktop-updater` 缓存里的 `installer.exe`（468 MB，是 0.3.43 的旧包），
合计约 **2.35 GB**；用户数据 `~/.deepai-code` 与 Electron 缓存保持不动。

---

## 79. 2026-11-22 线上体检整改：签到预热、出口冷却、部署形态（v0.1.13）

**触发**：SSH 上机只读体检（`instance-pu8iopa3`，容器 Up 37h）。三条证据都指向同一类问题——
**延迟与失败主要发生在「签到 + 出口代理」这条前置链路上，而不是协议或号池本身**：

| 观测项 | 实测 | 解读 |
| --- | --- | --- |
| `checkin` | attempts 2407 / **blocked 940（39%）** / timeouts 306 / p50 2s / p95 30s | 每个推理请求前同步打 `/theos`，慢时吃满 10s 预算 |
| `upstream_errors` | `transport` **1012**、429 ×127、403 ×92、ctx 超限 ×67 | 账号 last_err 全是 `Gateway Timeout` / `unexpected EOF`，属代理侧 |
| 代理连通性 | 16 个账号代理 TCP **16/16 全通** | 不是死代理，是吞吐/超时；需要「换出口」而不是「弃号」 |
| 累计 | 195 329 请求 / **12 586 失败（6.4%）**，`/v1/responses` p95 120s、p99 300s | — |

### 79.1 签到预热：把「到期」在请求路径外消化

- `internal/deepai/checkin.go`：抽出 `checkInDueAt()` 判据（refused 锁 / 半衰期
  `seconds/2` / 最小间隔 30s 三条规则不变），`EnsureCheckIn` 与新增的
  `Client.CheckInDue()` 共用同一判据 —— 预热**不改变签到节奏**，只是提前做；
- `internal/server/warmcheckin.go`：`Server.WarmCheckIns(ctx, limit, workers)`
  扫启用中的账号，对到期者补一次 `/theos`（默认每轮 4 个号 / 2 并发），失败静默、
  不写账号错误标记；
- `tasks.go`：`checkInWarm` 每 45s 触发一次（半衰期内通常只命中 1~2 个号）。
  热路径仍然调 `EnsureCheckIn`（nonce 回传是上游判定真实客户端的依据，不能挪到推理之后），
  但因为刚刚回传过，绝大多数会直接返回。

### 79.2 出口代理冷却：先换出口，再换号

- `internal/proxy/health.go`（新）：按**原始代理串**记账，连续失败 30s→60s→120s…
  最多 10 分钟冷却，一次成功立刻清零（`Health.Report/Cooling/Fails/CoolingCount`）；
- `internal/poolsvc/slots.go`：`Pick()` 重构出 `pickWith()`，新增
  `PickAvoiding(group, prefer, avoid)` —— 先在没有冷却出口的候选里选，**挖空整个池时
  退回常规调度**（宁可走一个已知慢的出口，也不要对外 429）；
- `internal/server/proxyhealth.go`（新）+ `upstream.go` 接线：传输失败 → 记失败；
  拿到任何上游响应（哪怕 4xx/5xx）→ 清零；`PickAvoiding` 用
  `avoidCoolingProxy` 谓词，粘性号在出口冷却时也会让位；
- `/admin/api/overview` 新增 `proxy_cooling`（当前冷却中的出口数）。

### 79.3 部署形态：机密出库、healthcheck、不可变 tag

- `docker-compose.yml`：
  - `env_file: ${DEEPAI2API_ENV_FILE:-/etc/deepaicode2api.env}`（`required: false`）——
    机密（账号 / 口令 / 代理 / 机器指纹）移出代码目录，权限 0600；
  - 新增 `healthcheck`（镜像内 `wget /healthz`，30s/5s/3 次，`start_period 20s`）：
    之前没有 healthcheck，进程假死时 `unless-stopped` 永远不会重启；
  - 新增 `mem_limit: 512m`（实测常驻 211MB，留两倍余量）与 `logging` 上限 10m×3；
  - `image: ${DEEPAI2API_IMAGE:-deepaicode2api:local}` + `ports: ${DEEPAI2API_ADDR:-10087}:10087`，
    部署脚本注入不可变 tag，回滚就是换回旧 tag；
- `scripts/deploy.sh`（新）：build → 打 `local-<日期>-<版本>-<sha>` tag → `up -d --no-build`
  → 轮询 healthcheck → 打印 `/healthz` 摘要；失败时打印最近 40 行日志并给出回滚命令；
- `.gitignore`：忽略本机 `docker-compose.override.yml`（线上用它挂 `sub2-shared`
  网络与 10088 端口，不入库）。

### 79.4 顺带：客户端 0.3.44 + 控制台 404 复查

- `AppVersion` 0.3.43 → **0.3.44**（§78），线上镜像此前仍是 0.3.42 的心跳；
- 控制台 404（`/admin/api/state|export|config|backup|import`）复查：仓库前端
  `internal/server/web/js/*.js` 里**已无**这些调用，线上那批是容器启动瞬间旧标签页
  打的（时间戳 2026-11-20 13:56–15:39），不需要改代码。

### 79.5 本轮改动清单

- 代码：`internal/deepai/checkin.go`、`internal/server/warmcheckin.go`(新)、
  `internal/server/proxyhealth.go`(新)、`internal/server/server.go`、
  `internal/server/upstream.go`、`internal/server/admin_api.go`、
  `internal/poolsvc/slots.go`、`internal/proxy/health.go`(新)、`tasks.go`、`main.go`；
- 测试：`internal/proxy/health_test.go`、`internal/poolsvc/slots_avoid_test.go`、
  `internal/server/warmcheckin_test.go`、`internal/server/proxyhealth_test.go`
  （覆盖：冷却退避与上限、成功清零、按代理串隔离、nil 安全、粘性让位、
  全 cool 时回退常规调度、预热幂等与停用账号跳过）；
- 部署：`docker-compose.yml`、`scripts/deploy.sh`、`.gitignore`、`README.md`

验证：`gofmt` 干净、`go vet ./...` 通过、`go test ./...` 全绿（含新增用例）。

### 79.6 上线与调优（v0.1.13 → v0.1.14）

**上线**（2026-11-22 11:34 CST，`sh scripts/deploy.sh v0.1.13`）：

| 项 | 结果 |
| --- | --- |
| 镜像 | `deepaicode2api:local-20261122-v0.1.13-3b1a5b4`（上一版 `local-20261120-v0112-d4c9b2f` 保留，可一键回滚） |
| 健康检查 | `Up (healthy)`，`mem=536870912`（512MB 上限生效），`restarts=0` |
| `/healthz` | `version=v0.1.13`、`accounts=16 ready=16`、`lane=7 lane_upstream=7`、`models=27` |
| 二进制 AppVersion | **0.3.44**（心跳不再是 0.3.42） |
| 新字段 | `/admin/api/overview` 出现 `proxy_cooling: 0` |
| 机密出库 | `.env` 371B → `/etc/deepaicode2api.env`（0600）；仓库目录只剩 compose 插值参数，`api-key.txt` / `.bootstrap-api-key` 移到 `/root/private/`（0700） |
| 本机覆盖 | `docker-compose.override.yml`（不入库）保留 `sub2-shared` 网络 |

**首次上线后的实测把预热预算调错了**：预热沿用了热路径的 2500ms，
而住宅代理 `/theos` 的 p95 约 5s —— 预热几乎全部 `timed_out`，
却已经占掉了 30s 的最小间隔，等于把「到期」白白消耗掉。于是 v0.1.14 把预算分层：

| 路径 | 预算 | 理由 |
| --- | --- | --- |
| 热路径（推理前 `EnsureCheckIn`） | 2500ms（设置项 `checkin_block_ms`） | 用户可感知；超时先带旧 nonce 放行 |
| 后台预热（`WarmCheckIns`） | **10s**（`warmCheckInBudget`） | 不在任何请求路径上，给足时间才真能把 nonce 回传成功 |

实现上是新增 `Client.EnsureCheckInWithin(ctx, acct, budget)`：`EnsureCheckIn` 变成
「用客户端预算调用它」，预热用显式 10s；预热批上限 4 → 8、周期 45s → 30s
（16 个号全部到期时最多 4 轮消化完）。

**顺带修好的系统问题**：

- **时钟未同步（差 1.8s）**：`systemd-timesyncd` 是 masked 状态，chrony 虽 running
  但源用的是开机时的旧配置（`Reach 0`、`LastRx 55d`，参考时间停在 09-27），
  且配置里的四台 `*.baidubce.com` **已无法解析**。改为可解析的国内源
  （`ntp.aliyun.com` / `ntp1.aliyun.com` / `time1.cloud.tencent.com`）后：
  `System clock synchronized: yes`，offset ~1.5ms，三源可达；
- **磁盘**：`docker builder prune -f` 回收 **1.226 GB**（构建缓存 2.25GB→1.02GB），
  根分区 27G/40G(70%) → **26G/40G(68%)**。没有执行 `docker image prune -a`：
  实测 `docker system df` 的「可回收 5.35GB」把运行中容器的共享层也算进去了，
  真正无主镜像只有 378MB（`golang:1.26-alpine` 构建基座 355MB + 上一版回滚镜像
  19MB + `alpine:3.20` 4MB），全删会让 CN 网络下的下次构建无法拉取基座；
  改为 `/etc/cron.d/docker-gc`（每周清 168h 以上的构建缓存 + 悬空镜像）；
- **补丁欠账**：353 个待升级包，82 天未重启 —— 已列为单独窗口执行（见 §79.7）。

### 79.7 发现并修复：按账号分出口静默失效（v0.1.15）

体检时顺手验证「16 个账号各绑一个住宅代理」是否真的生效，结果**没有**：

```
账号里存的出口： 06gvj…fireside.bringyour.com:7161   ← 没有 scheme
url.Parse 的结果：Scheme="06gvj…bringyour.com" Opaque="7161" Host=""
旧代码的判据：    if u.Host != "" { transport.Proxy = http.ProxyURL(u) }   ← 不成立
→ 专属出口从未被设置，全部静默退回环境变量里的全局代理（mihomo socks5）
```

- **后果**：16 个号共用同一个出口 IP，「按账号分出口 / 按出口共享每日额度」
  的设计完全没生效；`/theos` 走的是共享的美国出口（p50 2s、p95 5s+）；
- **根因**：`host:port` 里的主机名（含点号、数字）在 `url.Parse` 眼里是合法
  scheme，解析成功但 `Host` 为空，代码只判断了 `u.Host != ""`，没判断「有没有真的
  解析出 scheme」；
- **修复**：
  - `internal/proxy/probe.go`（新）：`Parse()` 缺 scheme 时补 `http://`，
    非 http/https/socks5 或缺少 host:port 一律显式报错；
  - `deepai.NewClientWithFingerprint` 改用 `proxy.Parse`，解析失败记到
    `Client.ProxyInvalid`，不再静默降级；
  - **启用前先验证**：修好解析后，一批从未真正用过的住宅代理会突然生效，线上
    不能直接赌。`Server.proxyUsable()` 先用 `proxy.Probe()`（HTTP CONNECT 到
    `api.deepai.org:443`，6s 预算，结论缓存 10 分钟）确认能穿透，**通过才启用
    专属出口**；未知 / 失败沿用全局出口（与修复前行为一致）。探测失败会留一条
    `出口代理探测失败（沿用全局出口）` 的 WARN，带脱敏后的出口标签。

**部署脚本加了一道版本一致性闸门**（2026-11-22 踩到）：服务器上
`git fetch --tags` 因 GitHub TLS 中断失败（服务器没有给 git 配代理），
而 `deploy.sh v0.1.15` 照常构建，于是打出 `local-20261122-v0.1.15-95125db`
这种「标签写着 v0.1.15、代码其实是 v0.1.14」的错标镜像。现在：

- `deploy.sh` 传入 tag 时必须 `refs/tags/<tag>` 存在且 `git rev-parse <tag>^{commit}`
  等于 `HEAD`，否则直接退出（错标镜像比部署失败更危险）；
- 服务器仓库写入 `http.proxy/https.proxy = http://127.0.0.1:17920`（mihomo mixed），
  之后 `git fetch --tags` 才能稳定拉取（直连 GitHub 会 TLS 中断）。

### 79.8 上线结果（v0.1.15，2026-11-22 11:52 CST）

| 项 | 结果 |
| --- | --- |
| 镜像 | `deepaicode2api:local-20261122-v0.1.15-170efcd`（v0.1.14 与错标镜像已清理） |
| 容器 | `Up (healthy)`，healthcheck 30s，`mem_limit 512MB`，`restarts=0` |
| `/healthz` | `version=v0.1.15`、`accounts=16`、`lane=7 / upstream 7`、`models=27` |
| 专属出口探测 | **9 个出口探测通过并启用**，2 个失败（`i/o timeout` → 沿用全局出口），
其余为缓存中/未到期；日志两类各带脱敏出口标签，可 grep `出口代理` |
| 流量 | 部署后 5 分钟：访问 19 条、状态全 200；累计 `failed_requests` 未上升 |
| 签到 | `checkin.attempts 2 / blocked 2`（流量低谷，样本不足；预热循环 30s 一轮、单次 10s 预算） |

**服务器侧另外修好的三件事**：

1. **git 无法直连 GitHub**：`git fetch` 报 `GnuTLS recv error (-110)`，
   现在仓库级配置 `http.proxy/https.proxy = http://127.0.0.1:17920`（mihomo mixed，
   socks 17912 对该路径是 403，见 `/root/scripts/backup-to-r2.sh` 的踩坑记录）；
2. **错标镜像**：一次 `git fetch` 失败 + 旧脚本没有一致性检查，打出
   `local-20261122-v0.1.15-95125db`（标签 v0.1.15 / 代码 v0.1.14）。已删除，
   并在 `deploy.sh` 加闸门（tag 必须存在且等于 HEAD）；
3. **时钟/磁盘**见 §79.6。

### 79.9 待排期：系统补丁与重启窗口

体检时 `apt` 待升级 **353 个包**，内核 `5.15.0-151`，主机已连续运行 **82 天**。
没有 `reboot-required` 标记，但重启会带走全部 11 个容器，因此单独安排窗口：

```bash
# 窗口前：确认数据与状态可回滚
docker compose -f /root/DeepAICode2API/docker-compose.yml ps
sudo /root/scripts/backup-to-r2.sh          # 已有每日 3:00 的 cron，可手动再跑一次

# 窗口内：只升安全补丁 + 全量升级二选一
sudo apt-get update
sudo apt-get -y --only-upgrade install $(apt-get -s -o Debug::NoLocking=1 upgrade | awk '/^Inst .*security/ {print $2}')   # 安全补丁
sudo apt-get -y dist-upgrade
sudo reboot      # 所有容器配置了 restart: unless-stopped，会随 docker 自动拉起

# 窗口后
docker ps --format '{{.Names}}\t{{.Status}}'
curl -s localhost:10088/healthz
```

不改系统默认策略（`unattended-upgrades` 未启用），避免自动重启打断请求。

### 79.10 出口被 WAF 拦时自动降级（v0.1.16）

v0.1.15 上线后立刻出现一个可解释的副作用：`laoda0226@gmail.com` 的专属出口
**探测通过、但推理被上游 WAF 拦**（`403 <html>…nginx/1.24.0 (Ubuntu)`），账号被标
`error`，池容量 16 → 15。根因是把「HTML 错误页」当成了出口可用（有响应 = 出口通），
而它恰恰是**出口被拦**的证据。

修复两处，形成闭环：

- `upstream.go`：响应体看起来是 HTML（首个非空白字节是 `<`）时记 **出口失败**，
  不再记成功 —— 让这条出口进入冷却（`proxy.Health`，30s 起指数退避）；
- `clientFor`：出口**冷却中**就退回全局出口（`!proxyUsable || proxyCooling`）。
  这是「自动降级」而不是「弃号」：账号继续服务，只是不再从这条出口出去；

- 新增 `looksLikeHTML()` 与用例：`<html>`/缩进的 `<!DOCTYPE>` 判真，
  上游 JSON 错误、空体、纯空白判假（避免把 `rate_limit_exceeded` 误判成出口问题）。

效果：被 WAF 拦的出口在冷却期内自动走全局出口，冷却到期后再试一次专属出口；
不再出现「账号永久 error、容量白掉」。

### 79.11 「分配代理」总落到同一个出口（v0.1.17）

**触发**：线上新增一个 OAuth 账号（`liziwww235`，email 为空）需要分配出口，
发现 17 个账号里 **3 个挤在同一条住宅代理上**，而池里另外 6 条出口各只有 1 个号。

根因是 `adminAssignProxy` 每次调用都 `poolsvc.LoadProxyPool()` **新建一个池**：
`ProxyPool.Next()` 的游标从 0 开始，于是每次「分配」都返回 `pool[0]` ——
新入池的账号（enroll 之外的路径）全部堆在第一条出口上，等于共享同一个出口 IP
的每日免费额度。老用例没抓到，是因为它一次调用里给多个账号分配（同一个池实例，
游标会前进）。

修法（`internal/server/admin_actions.go`）：

- 复用进程内缓存的池（`Service.EnrollProxyPool`），游标跨调用保留；
- 选出口改为「当前账号数最少」优先，同样多时按游标轮转
  （`proxyEgressKey` 把 `https://host:port` 与 `host:port` 归一成同一个出口再统计）；
- 响应新增 `in_use`（当前被占用的出口数），一眼看出池子够不够：
  17 个号 / 11 条出口必然有共享，要 1:1 就得补代理。

用例 `TestAssignProxySpreadsAcrossCalls` 复现并锁死这个 bug（两次独立调用必须
分到不同出口，`in_use` 随之为 2）。

### 79.12 追加代理后无需重启（v0.1.18）

v0.1.17 为了保住「分配游标」把代理池缓存在进程里，副作用是：运维往
`PROXY_POOL_FILE` 追加新出口后，控制台的 `assign-proxy` 仍只认启动时那份快照，
必须重启网关才看得到新代理。

v0.1.18 改成「环境变量值 + 文件 mtime」做指纹（`internal/server/assignpool.go`）：

- 指纹未变 → 复用缓存的池，游标继续前进（不会重现 v0.1.17 的「总分配同一条」）；
- 指纹变了（追加 / 删改代理文件，或改了 `PROXY_POOL`）→ 重建池，游标归零
  —— 「刚加完一批出口、准备重新铺开」正是这个语义。

顺带补上了「池子够不够」的可观测口径：`assign-proxy` 响应里的 `in_use`
（当前被占用的出口数）与 `pool_size` 一比就知道还差几条。

### 79.13 补 6 条出口做到 1:1，并修掉「注释带空格就解析失败」（v0.1.19）

**分配结果**（2026-11-22 12:20 CST，池 11 → 17 条）：

| 项 | 之前 | 之后 |
| --- | --- | --- |
| 出口数 | 11 | **17** |
| 账号数 | 17 | 17 |
| 出口占用 | 6 条 ×2 + 5 条 ×1 | **17 条 ×1（1:1）** |
| `assign-proxy` 响应 `in_use` | 11 | **17** |
| 账号状态 | 17 ok | 17 ok |

6 个新出口（`06h031…`）首次 `CONNECT` 冷启 3 条超时（10s）、回探 1.1–4.4s 全通；
网关探针（6s 预算）**6/6 全部启用**。搬迁的 6 个账号是原先共享出口里的后加入者，
每条共享出口保留最早的那个号。

**顺带修掉一个配置解析的坑**：`ParseProxyList` 先把空格 / 逗号 / 分号统统当分隔符、
再按 `#` 过滤，于是运维写的注释
`#added-20261122-1218 bringyour bundle 5（6 条，追加后无需重启）`
被切成残片，残片不以 `#` 开头就被当成代理串，`assign-proxy` 直接 400
`代理格式非法：invalid`。

v0.1.19 改为**先按行判注释**（行首 `#` 整行跳过），行内再用 `, ; 空白` 分隔多个代理：
注释里可以随便写空格、逗号、中文括号；同一行多代理的旧写法仍然有效。
用例 `TestParseProxyListCommentsWithSpaces` / `TestParseProxyListInlineSeparators` 锁死两种形态。

顺带修掉一个**测试自身的缺陷**：`TestHealthCooldownBacksOff` 用 `map` 遍历期望值，
Go 的 map 顺序随机导致偶发失败（`连续失败 = 1，期望 2`），改成有序切片。

### 79.14 出口被 WAF 拦不再惩罚账号（v0.1.20）

补的 6 条新出口里，**3 条 CONNECT 探测通过、但推理被上游 WAF 以 HTML 403 拒绝**
（`06h031b6` / `06h031cs` / `06h031e1`）：探测只能证明 TCP/TLS/CONNECT 通，
无法证明这个出口 IP 没有被上游拉黑。旧逻辑把这种 403 当账号错误 →
`MarkFailure` → 账号被标 `error`、不再调度，池容量白掉 2 个号。

v0.1.20 的判据：**HTML 错误页 + 账号绑了专属出口 = 出口级故障** ——
只冷却出口（`noteProxyResult(false)`）、丢掉粘性、换下一个候选，
**不 MarkFailure / 不 Block**。该账号下一轮因为出口冷却自动改走全局出口
（`clientFor` 检查 `proxyCooling`），成功后状态自然恢复。
用例 `TestWAF403CoolsExitWithoutBlankingAccount` 锁死这条：403 照原样回传给调用方，
但账号仍是 `ok`、出口进入冷却。

**线上处置**：把 3 条被拦的新出口在池文件里注释掉（`#blocked-20261122-…`），
把这 3 个账号重新分配回可用出口（14 条可用 / 17 个号，其中 3 个出口回到 2 个号）。

**最终状态（2026-11-22 12:55 CST，v0.1.20）**

| 项 | 值 |
| --- | --- |
| 号池 | 17 个账号**全部 `ok`**，`ready=17`、`degraded=0`、`blocked=0` |
| 出口 | 池 17 条 → **14 条可用**（3 条被 WAF 拦，已注释 `#blocked-20261122-1240`） |
| 占用 | 11 条 ×1 + 3 条 ×2（17 个号 / 14 条出口，必然有 3 条共享） |
| 冷却 | `proxy_cooling=1`（被 WAF 拦的出口自动冷却，账号不背锅） |
| 流量 | 近 5 分钟 60 条请求**全部 200**；`failed_requests` 停在 12595 未增长 |

处置记录：

- 新账号 `liziwww235`（OAuth，无 email）→ 分配到新出口 `06h031c1…`（探测 2.6s 通过）；
- 另 3 个号（`yvbeghihag6wvjd` / `pwvjkhiq1hwz` / `zupxzenogljb`）原本被搬到被 WAF 拦的
  新出口，已搬回可用出口并 `clear-errors`；`status=unknown` 按设计视为可调度，
  下一次 `/theos` 成功后自动回到 `ok`（实测 5 分钟内全部恢复）；
- 备份：`data/proxy_pool.txt.bak-20261122-2`（追加 6 条之前）。

要把这 3 条共享也变成 1:1，需要**再补 3 条能过推理的出口**（不只是 CONNECT 通，
要能真正打通 `/theos`，否则会被 WAF 403 打回）。

### 79.15 出口预检升级：CONNECT 之外再发一次真实请求（v0.1.21）

上一轮补的 6 条出口里有 3 条「CONNECT 通、推理 403 HTML」，说明**只看隧道是不够的**：
出口 IP 有没有被上游 WAF 拉黑，只有真发一个请求才知道。

v0.1.21 把 `probeExit()` 改成两步（都在后台、都在冷却之外，不影响请求路径）：

1. `proxy.Probe()`：CONNECT 到 `api.deepai.org:443` —— 隧道通不通；
2. 经该出口真发一次 `GET /deepai_code/theos`（**不带 `Api-Key`**，不消耗额度）——
   看上游是否接受这个出口。

判据（`classifyProbeResponse`）：**HTML 错误页 = 被拦**；JSON 的 4xx/5xx
（426 车道不对、401 缺 Key、429 限流）都算「出口是好的」。用例覆盖 6 种组合。

效果：被 WAF 拉黑的出口在预检阶段就被判死 → 该账号直接用全局出口，
不会再出现「先吃一次 403、再冷却、再回退」的浪费。

**第二轮补出口（bundle-6，2026-11-22 13:10 CST）**：3 条**全部不可用** —— 这正是
v0.1.21 出口预检要解决的问题，两种失败形态都被抓到：

| 出口 | 失败形态 | 判定 |
| --- | --- | --- |
| `06h037dbd6…:7143` | CONNECT 通，`GET /theos` 6s 超时 | `context deadline exceeded` |
| `06h037e6tb…:7155` | CONNECT 无响应 | `i/o timeout` |
| `06h037f3gm…:7143` | CONNECT 无响应 | `i/o timeout` |

用 `curl -x` 复测同样不通（`http://` 0.3–0.7s 被拒；`https://` 挂满 25s 无 CONNECT 应答），
与 bundle-5（6 条里 3 条可用）相比这一批更像是过期/欠费。

处置：3 条注释为 `#blocked-20261122-1310 preflight-failed-exitip …`，
这 3 个账号（`yvbeghihag6wvjd` / `pwvjkhiq1hwz` / `zupxzenogljb`）搬回可用出口。

**最终状态（13:15 CST，v0.1.21）**：

| 项 | 值 |
| --- | --- |
| 号池 | 17 个账号全部 `ok`，`ready=17`、`degraded=0` |
| 出口 | 池文件 32 行里 **14 条可用**（18 条注释：历史 blocked + 本轮 6 条不可用） |
| 占用 | 11 条 ×1 + 3 条 ×2（17 号 / 14 出口） |
| 流量 | 近 5 分钟 32 条请求全 200；`failed_requests` 停在 12595 |
| 备注 | 关键收益是「**不可用出口在预检阶段就被挡掉**」，没有再出现 403 打到账号 |

要做到 1:1 还差 **3 条能过预检的出口**（判定：CONNECT 通 **且** 经它 `GET /theos`
返回 JSON 而不是 HTML/超时）。

**第三轮补出口（bundle-7，2026-11-22 13:30 CST）**：预检预算 6s 太短，先修预算再看结果。

用与网关同形的独立校验器（`CONNECT → 隧道 TLS → GET /deepai_code/theos`，
在 `golang:1.26-alpine` 容器里跑，避免被测机自身的网络干扰）实测：

| 出口 | 结果 |
| --- | --- |
| `06h03a2l…:7125` | 第二次 6.1s → `426 Upgrade Required`（JSON，可用） |
| `06h03a8d…:7131` | 第二次 7.7s → `426 Upgrade Required`（JSON，可用） |
| `06h03a97…:7155` | 两次均 `CONNECT` 无应答（>12s） |

三条都能通过 TCP/TLS 到代理，但 **CONNECT 握手 6–8s**（住宅代理冷启动）。
原 `proxyProbeTimeout = 6s` 会把这两条可用出口误判成「不可用」——
对照组（已知可用的 bundle-5 出口）本次也出现过 13s 超时，说明这批出口的
CONNECT 抖动本身就大。

v0.1.22：预检预算 6s → **14s**（预检完全在后台，只多花一次等待，不影响请求）。

**结果：bundle-7 三条全部可用，首次做到 17 号 / 17 出口 1:1**

| 项 | 值 |
| --- | --- |
| 版本 | `v0.1.22`（`local-20261122-v0.1.22-e997e54`），`Up (healthy)` |
| 号池 | 17 个账号**全部 `ok`**，`ready=17`、`degraded=0`、`blocked=0` |
| 出口 | 池文件 17 条可用（14 + bundle-7 的 3 条），**占用 1:1（17 × 1）** |
| 预检 | bundle-7 三条全部「出口代理可用，已启用专属出口」（14s 预算下） |
| 流量 | 近 5 分钟 73 条请求**全部 200**，`WAF 拦截 0`，`failed_requests` 停在 12595 |
| 冷却 | `proxy_cooling=0` |

三条新出口的实测（预检器 + 网关预检一致）：`06h03a2l…` 6.1s、`06h03a8d…` 7.7s
均返回 `426 Upgrade Required`（JSON）；`06h03a97…` 在独立校验器上两次 CONNECT 超时，
但网关预检（14s 预算、CONNECT 偶发更快）判定可用 —— 说明这批出口的 CONNECT
抖动很大，**预算放宽到 14s 是必要的**，否则会把可用出口误杀。

至此「一个账号一个住宅出口」的目标达成；后续若再加号，直接：
1. 追加代理到 `data/proxy_pool.txt`（注释行可带空格，会自动重载）；
2. `assign-proxy`（不带 `reassign`，只补空值）或对新号单独 `reassign`；
3. 看日志里 `出口代理可用` / `探测失败` 两类行确认预检结果。

---

## 80. 发布命名规范：tag / Release 标题一律 `vX.X.X`（2026-11-22）

**问题**：v0.1.16–v0.1.22 这几轮是我手工 `gh release create --title "v0.1.17 — 修复……"`
发布的，列表里标题长短不一、带中文后缀，既不好扫、也没法机器解析（虽然有
`tag_name` 兜底，但标题与 tag 不一致本身就是噪音）。

**规则（已固化进脚本与文档）**：

- tag 与 Release **标题都必须是 `vX.X.X`**（三段数字，如 `v0.1.23`）；
- **不接任何描述后缀**：不写 `— 修复xxx`、不加日期、不加中文；
- 描述只写在两处：`dist/release-notes-<ver>.md`（Release body）与 `docs/changes.md`。

**实现**：

- `scripts/release.sh`：
  - 版本号格式校验（`^v[0-9]+\.[0-9]+\.[0-9]+$`），不合规直接退出，**不构建**；
  - 发布时 `--title "$VER"` 恒为版本号本身；
  - Release 已存在时（重新上传资产）顺手 `gh release edit --title "$VER"` 把标题纠回版本号；
  - 优先用 `dist/release-notes-<ver>.md` 作为 body，缺失才退回一句指向 `docs/changes.md`；
- 存量 7 个带后缀的 Release（v0.1.16–v0.1.22）已 `gh release edit` 改成纯版本号；
- 控制台的更新比对读的是 `tag_name`，与标题无关，改名不影响「有新版本」提示。

**现状核对**：23 个 Release 的 `tagName` 与 `name` 全部匹配 `^v[0-9]+\.[0-9]+\.[0-9]+$`。

---

## 81. 2026-11-22 客户端 0.3.45 同步复核：DeepAI Connect 功能轮，协议面仍零变更（未发版）

**触发**：例行扫描。官方 `latest.yml` 从 **0.3.44** 跳到 **0.3.45**
（`releaseDate` 2026-11-22T04:13:59.470Z，距 §78 那次 0.3.44 复核只隔约 5.7 小时，
是 11 月里节奏最快的一轮）。release notes 报的是 **DeepAI Connect**：一条命令把
Linux 服务器链到账号、断开后 agent 继续在服务器上跑，同时支持服务器上的
Claude Code / Codex 安装与 DeepAI 模型并用。

### 81.1 版本核实

| 复核项 | 0.3.44 | 0.3.45 |
| --- | --- | --- |
| `latest.yml` `version` | 0.3.44 | **0.3.45** |
| `releaseDate` | 2026-11-21T22:31Z | **2026-11-22T04:13:59Z** |
| x64 `sha512` / `size` | `haSxCvh0…` / 490,345,232 B | `ASQA2PXh…` / **490,374,704 B** |
| arm64 `sha512` / `size` | `ZGe5HxI2…` / 470,274,560 B | `f9ZAhJxu…` / **470,267,728 B** |
| `app.asar` `package.json` | 0.3.44 | **0.3.45**（`t3codeCommitHash: 30429cde7441`） |
| 本机 `ProductVersion` | 0.3.44.0 | 复核时 0.3.44.0；复核后已升级到 **0.3.45.0** |

下载 `DeepAI-Code-0.3.45-x64.exe`（490,374,704 B）→ sha512 与 `latest.yml` 逐字节一致
→ 7z 解 NSIS 的 `$PLUGINSDIR/app-64.7z` → `resources/{app,server}.asar`。

### 81.2 结论：推理协议面仍然零变更，只跟心跳版本号

`server.asar` → `apps/server/dist/binCli-C6U4jU1g.mjs`（14,896,813 B，0.3.44 是
`binCli-B9W8M09_.mjs` 14,870,582 B），全量 diff **107 hunk / +769 / −152**；
按 `//#region` 归属 **1756 个源模块（0.3.44 为 1751）→ 41 个变、4 个新增**，
其余 1711 个字节级不变。

| 复核项 | 0.3.44 | 0.3.45 | 判据 |
| --- | --- | --- | --- |
| `DEEPAI_LANE_HEADERS` | `Theos: "7"` + `Instance: randomBytes(16).hex` | **逐字未动** | 字节级 |
| `X-*` 头字面量 | 14 个（含 5 个 `X-DeepAI-Code-*`） | **集合零差异** | 字面量集合 diff |
| 推理路径 `/deepai_code/*` 端点 | 推理 / `/models` / `/theos` / 搜索 / `sessions/*` | **完全一致** | 字面量集合 diff |
| 聊天请求体 | `max_completion_tokens` / `"store"` / `reasoning_effort` | **字面量计数 2 / 1 / 9，`max_completion_tokens` 前后 1.5 KB 逐字一致** | 片段比对 |
| `previous_response_id` / `item_reference` | 0 / 0 | **0 / 0** | 全库 grep |
| beacon `v=` | `v=${version$2};up=…;pi=…;focus=…` | **同一逻辑**，`version$2`（`package-Cc0oX7hP.mjs` 的 `version`）报 0.3.45 | 源码级 |
| 机器指纹 | `machineId.ts` | **不在变更列表**（算法与 Linux `parseLinuxMachineId` 校验未动） | region hash |
| 内置 pi 运行时 | `pi-ai 1.3.0` | **1.3.0**，`dist/api/openai-completions.js` sha256 一致（`4b3875e5…`） | 解包 `node_modules` |
| 凭据文件 | `{apiKey,username,isPro}` | **只增可选字段** `connect{token,expiresAt}` / `connectUnavailable` | Schema diff |

### 81.3 这一轮真正改的：DeepAI Connect（协议面外）

4 个新增模块：`packages/contracts/src/managedConnection.ts`、
`packages/shared/src/managed/{setup,leasedSession}.ts`、`src/deepai/managedHostAccount.ts`。
41 个变更模块里 `src/deepai/*` 只占 5 个，全在推理链路之外：

- `api.ts`：登录加 **PKCE**（`deepAISignInUrl` 带 `code_challenge` +
  `code_challenge_method=S256`，`exchangeDeepAICode` 带 `code_verifier`）；
  `/desktop_auth/exchange/` 响应新增可选 `connect_token`（`^dc_[A-Za-z0-9_-]{43}$`）
  与 `connect_unavailable: "session_limit"`；拒答码白名单加 `email_unverified` /
  `machine_bound` / `network_bound` / `theos_exhausted`（`deepAIAccountRejectionCode()`，
  **只进 Sentry tag，不上行**）；`credentialAccountId()` 让 host 账号用
  `host-account:<id>` 而非 apiKey 哈希作账号标识。
- `DeepAIAccount.ts`：凭据 Schema 加可选 `connect` / `connectUnavailable`；`/connect/*`
  改走 `Authorization: Bearer dc_…`（不是 `Api-Key`），401 / 403 `device_revoked` 触发
  「重新登录」；host 账号时 `/get_usage_summary` 直接跳过；`/theos` 上报加
  `rejection` 标签。
- `localTransport.ts`：PKCE verifier/challenge 生成（`randomBytes(32).toString("base64url")`
  → sha256 → base64url）。
- `DeepAIOptionalServices.ts`：赞助卡片新增 `authorizeBrowserCard()` → 上游
  `/sponsored/authorize`，且所有入口加 `config.mode !== "desktop"` 门禁（离开桌面壳不跑）。
- `DeepAIPiConnection.ts`：`models.json` 写入前做内容幂等比对、`rename` 对
  `EPERM/EBUSY/EACCES` 重试 4 次并串行化；`settings.json` 解析失败从 `catchTag` 改
  `catchTags`（修 `PiRpcTimeoutError` 未捕获）。

`managedHostAccount.ts` 是新增的「托管主机账号」：向本机 `managed/state.sqlite` 的
control 通道要 `host-account`，拿到 `dch_` 前缀短期令牌（`expires_in ≤ 900`）后
**只在内存里**封装成 `{apiKey: dch_…, host:{accountId,expiresAt}}`，**不写凭据文件**。

→ 结论：`deepai.AppVersion` 跟到 **0.3.45**，`LaneVersion` 保持 `"7"`，协议逻辑、
请求头、请求体与指纹都不改。网关侧无需改动即可吃 0.3.45 桌面端写出的凭据：
`credimport` 用 `encoding/json` 解码，多出的可选字段自动忽略，`apiKey` 仍必填。

### 81.4 本轮改动

- `internal/model/types.go`：`AppVersion` 0.3.44 → **0.3.45**，车道注释补 0.3.45 复核；
- 注释版本号同步：`internal/deepai/client_test.go`、`internal/machine/machine.go`、
  `internal/machine/machine_test.go`、`internal/gateway/chatbody.go`；
- `README.md`：应用版本 / 版本链 / 车道复核日期 / 心跳 `v=` / ToS 注释标注；
- `docs/recon.md`：顶部新增 2026-11-22 二次复核条目（0.3.45），§1 / §2 / §2.3 / §3 /
  §5.1 / §5.3 的【源码级】标签升到 0.3.45，车道路由表与 `v=0.3.45` 心跳示例同步。

### 81.5 本机升级与清理

用同一个官方安装包静默升级（`DeepAI-Code-0.3.45-x64.exe /S`）：退出码 0，
`DeepAI Code.exe` 的 `ProductVersion` 变为 **0.3.45.0**、`FileVersion` 0.3.45，
`resources/{app,server}.asar` 与安装包内解包产物 sha256 逐字节一致（`app.asar`
`12bd0b26…` / `server.asar` `d1944e04…`），`server.asar` 内为 `binCli-C6U4jU1g.mjs`，
卸载注册项从 `DeepAI Code 0.3.44` 变为 `0.3.45`（单条，无重复项）。

安装目录 1.4 GB，除 `resources/git/etc/{hosts,networks,protocols,services}`
四个 git 自带文件外没有早于本次构建（2026-11-22 12:13）的残留时间戳。
随后清理：安装包（490 MB）、NSIS 解包目录 `$PLUGINSDIR/app-64.7z` 与解开的应用树
（0.46 + 1.29 GB）、0.3.44 的 `app.asar` / `server.asar` 副本与两版 `binCli`（0.24 + 0.03 GB）、
`deepai-code-desktop-updater` 缓存里的 `installer.exe`（490 MB，就是本轮 0.3.45 包），
合计约 **2.9 GB**；用户数据 `~/.deepai-code` 与 Electron 缓存保持不动。

---

## 82. 2026-11-22 浏览器登录跟进 0.3.45 的 PKCE（未发版）

**触发**：§81 复核时发现 0.3.45 的登录链路从「state + port」改成了 PKCE
（`deepAISignInUrl` 加 `code_challenge` + `code_challenge_method=S256`，
`exchangeDeepAICode` 带 `code_verifier`）。本项目浏览器登录（`internal/server/login.go`
+ `deepai.SignInURL` / `ExchangeCode`）此前只发 `state`+`port`，属于**未跟进**。

**先做的实测（2026-11-22，无凭据、可复现）**：

| 请求 | 结果 |
| --- | --- |
| `GET /deepai_code/desktop_auth/?state=<64hex>&port=…`（无 challenge） | 200，正文与带 challenge 的响应 **md5 完全一致**（`be530d22…`） |
| 同 URL + `code_challenge=<43 字符 base64url>&code_challenge_method=S256` | 200，逐字节相同 |
| 同 URL + `code_challenge=xx` | **400 `Invalid sign-in proof.`** → 上游按格式校验 challenge |
| 同 URL + `state=probe123`（过短） | 400 `Invalid desktop sign-in request.`（state 长度也在校验） |
| `POST exchange/`：`{code}` / `{code,code_verifier}` / 仅 `{code_verifier}` / 带 challenge | 四种都只回 `{"error":"invalid code"}`，**没有 verifier 缺失类错误** |

结论：非 PKCE 会话**当前仍可用**，但已是「能跑通≠姿势对」（与车道门槛同性质）。
本项目选择跟随桌面端（默认开），同时留一个环境变量退路。

**实现**：

- `internal/deepai/pkce.go`（新增）：`PKCEParamChallenge` / `PKCEParamVerifier` /
  `PKCEMethodParam` / `PKCEMethodS256` 常量、`PKCEChallenge()`（`base64url(sha256(verifier))`）、
  `NewPKCE()`（32 字节随机 → 43 字符 base64url 的 verifier + challenge）、
  `ValidPKCEVerifier()`（43 字符 base64url 格式校验）；
- `internal/deepai/client.go`：`SignInURL(state, port, challenge)` 在 challenge 非空时带
  S256 两参；`ExchangeCode(ctx, code, verifier)` 在 verifier 非空时把 `code_verifier`
  放进 body（同时把响应体变量改名 `respBody`，避免与请求体混淆）；
- `internal/server/login.go`：`loginSession` 增 `Verifier` / `Challenge`，`login/start`
  时生成（取不到随机数就都不发，绝不发配对不上的参数），`/login/open` 跳转带 challenge，
  本机回调与粘贴回调都回传 verifier；开关 `DEEPAI2API_LOGIN_PKCE`（`configenv.Bool`，默认 `true`）。

**回归**（新增 4 条用例，全绿）：

- `TestSignInURL`（扩展）：challenge 为空时不得带 PKCE 参数；
- `TestSignInURLWithPKCE` / `TestValidPKCEVerifier`：两参成对出现、长度 43、
  challenge = S256(verifier)、格式校验边界（过短 / 非 base64url / 43 字符合法）；
- `TestExchangeCodeWithPKCE`：兑换 body 带 `code_verifier` 且与先前 challenge 配对；
- `TestLoginPKCECarriedThrough`：会话生成 PKCE 对 → `/login/open` 的 Location 带
  `code_challenge`+`S256` → 兑换请求带同一个 verifier；
- `TestLoginPKCEDisabledByEnv`：`DEEPAI2API_LOGIN_PKCE=0` 时既不生成 verifier/challenge 也不带 challenge 参数。

**未做**：`machine_bound` / `network_bound` / `device_revoked` 三个新拒答码不动分类——
客户端自己也只把它们送进 Sentry tag（全库仅 1 处字面量），语义未验证；按普通 403
走换号即可，等上游真实出现时再定。

**上线**（2026-11-22 20:40 CST，`sh scripts/deploy.sh`，镜像
`deepaicode2api:local-20261122-v0.1.22-4-gdaa4e0b-daa4e0b`）：

- 服务器 `/root/DeepAICode2API` 从 `e997e54` fast-forward 到 `daa4e0b`（含上一轮 `7c06e1d`）；
- 健康检查通过，`/healthz` 200：`accounts=17 ready=17 lane=7 lane_drift=0 models=27
  version=v0.1.22-4-gdaa4e0b`；
- **线上实测 PKCE 生效**：`POST /admin/api/login/start` → `GET /login/open` 的 Location 为
  `…/desktop_auth/?code_challenge=<43 字符>&code_challenge_method=S256&port=…&state=<64 hex>`；
- 上一版镜像 `local-20261122-v0.1.22-e997e54` 仍在（回滚：
  `DEEPAI2API_IMAGE=deepaicode2api:local-20261122-v0.1.22-e997e54 docker compose up -d --no-build`）。

**镜像清理**（2026-11-22 20:43 CST，按「只留当前版」执行）：

- 删掉 11 个旧 `local-*` 镜像（`local-20261120-v0112-d4c9b2f` 起，到
  `local-20261122-v0.1.22-e997e54`），`docker builder prune -f` 回收 153.6 MB 构建缓存；
  镜像总占用 6.141 GB → 5.892 GB、构建缓存 2.457 GB → 2.303 GB；容器仍 `healthy`、`/healthz` 200；
- **代价**：旧镜像已不存在，回滚只能重新构建（`git checkout <旧 sha> && sh scripts/deploy.sh`）。
  下次发版若还想留回滚位，删镜像时至少留上一版标签；
- `data/` 按决定不动（`proxy_pool.txt.bak-20261122-{2,3,4}` 共 19 KB、`logs.jsonl.1` 8.1 MB）；
- 其它项目（workbuddy2api / magpie / sub2api / grok2api 等）的镜像与缓存未动
  （`docker system df` 剩下的 5.5 GB reclaimable 不是本项目的）。

---

## 83. 2026-11-22 远端 issue 核对：#7 请求体字节闸门 + #5 日志补充 + 英文 README（未发版）

**触发**：远端 3 个 open issue 逐条核对：

| Issue | 状态 | 本轮处理 |
| --- | --- | --- |
| #7 目录标称 320k、实测有效 ~40k（196 KB 请求体） | 复现路径成立，代码闸门缺位 | 新增字节闸门 + 有效值字段 + 日志告警 |
| #5 上下文/失忆 | 合成路径重放已修（v0.1.9 起）；补充建议「日志加出站字节数」未做 | 请求日志新增 `body_bytes` / `body_near_cap` |
| #1 中国大陆访问 | 纯网络可达性问题，非代码缺陷 | README/英文版新增「上游可达性」说明（可用 PROXY_POOL 承载上游请求），不新增翻墙能力 |

### 83.1 #7 的根因：token 闸门拦不住纯英文长请求

`internal/respstate/history.go` 原有一道按 token 的 `EffectiveContextWindow`
（9 个免费模型 320k）。这个估算按 1.6 B/token 计（随机 ASCII 实测值），而正常
英文约 4.2 B/token：196 KB 请求体只估到 ~122k tokens，远低于 320k，于是
**上游在 196 KB 处静默砍头时网关完全不知道**。issue #7 与
lavabyte/deepai-gateway 的交叉实测都把悬崖落在**请求体字节**上，不是 token 数。

**实现**：

- `respstate.ItemApproxBytes`：按序列化后的字节数计量（与 `ItemApproxTokens` 互补）；
- `respstate.ContextBodyBytes()`：默认 `196608`（≈192 KiB），
  `DEEPAI2API_CONTEXT_BODY_BYTES` 可覆写，`0`/负数关闭；
- `Replay` 的两道闸门共用一套裁剪逻辑（`truncateHistoryByCosts` 接收单位无关的
  `costs` + `markerCost`），token 裁剪后再按字节复量一次，避免二次裁剪过度；
- 超限错误信息同时给出 token 估算、请求体字节、`contextWindow` 标称值与
  `issue #7` 的实测依据；`truncation:"auto"` 仍先裁最旧中段并留省略标记。

### 83.2 `/v1/models` 不篡改标称值，另下发有效值

按「保持上游目录原样」的既有原则，`context_window` 仍是 320,000；新增：

- `deepai.effective_context_window`：按模型实测/覆写的 token 上限
  （与 `EffectiveContextWindow` 同源）；
- `deepai.context_body_bytes`：字节闸门的当前值。

控制台 `/admin/api/models` 同步下发 `effectiveContextWindow` / `contextBodyBytes`。

### 83.3 #5 补充：请求日志记录出站字节

`store.LogEntry` 新增 `body_bytes`（实际发往上游的请求体字节）与
`body_near_cap`（≥ 上限 90% 时为 true，仅提示不阻断），并打一条 WARN。
`/v1/chat/completions`、`/v1/messages`、`/v1/responses`（透传与合成）全部覆盖；
`callJSON` 因此多返回一个 body 长度（两个调用点同步更新）。

排障口径：`messages` 正常增长但模型仍失忆 → 先看 `body_bytes` 是否接近/超过
196 KB；是则属上游截断（#7），不是网关丢历史。

### 83.4 回归

- `TestReplayBodyBytesCeiling`（3 子用例）：默认拒绝并说明字节上限；
  `truncation:auto` 可裁掉超长中段且保留开头暗号；`DEEPAI2API_CONTEXT_BODY_BYTES=0`
  可退回旧行为；
- `TestPublicModelsExposeDeepAIMeta`（扩展）：标称值与有效值并存；
- `TestReqLogRecordsOutboundBytes`（2 子用例）：字节数落盘、90% 阈值标记。
- `go test ./...` 全绿；`go vet ./...` 干净。

### 83.5 英文 README

新增 `README.en.md`：与 `README.md` 同结构的完整翻译，顶部互链；
中文版文档索引同步登记，后续两版需一起改。

### 83.6 上线（2026-11-23 00:29 CST，`sh scripts/deploy.sh`）

- 服务器 `/root/DeepAICode2API` 原为 detached `c77fe8d`（`v0.1.22`），先切回 `main`
  再 ff 到 `bfe30d9`；工作区无本地改动；
- 构建并启动镜像
  `deepaicode2api:local-20261123-v0.1.22-7-gbfe30d9-bfe30d9`；健康检查通过；
- `/healthz` 200：`accounts=17 ready=17 lane=7 lane_drift=0 models=27
  version=v0.1.22-7-gbfe30d9`；
- **线上实测新字段已生效**：`GET /admin/api/models` 每条带
  `contextBodyBytes:196608` 与 `effectiveContextWindow`（`glm-5.3-flash` 为 320000）；
- 上一版镜像 `local-20261122-v0.1.22-4-gdaa4e0b-daa4e0b` 保留，回滚：
  `DEEPAI2API_IMAGE=deepaicode2api:local-20261122-v0.1.22-4-gdaa4e0b-daa4e0b docker compose up -d --no-build`；
- 宿主发布地址为 `127.0.0.1:10088`（`DEEPAI2API_ADDR`，nginx 终止 TLS），
  容器内健康检查用 `127.0.0.1:10087`，排障时别混。

---

## 84. 2026-11-23 客户端 0.3.47 同步复核：登录指引 / 恢复提示轮，协议面仍零变更（v0.1.23）

**触发**：例行扫描。官方 `latest.yml` 从 **0.3.45** 跳到 **0.3.47**
（`releaseDate` 2026-11-23T04:38:15.028Z）。`latest.yml` 只列 0.3.47 的位置，但它并非跳过号：
`DeepAI-Code-0.3.46-x64.exe`（490,365,616 B）在同一个 releases 目录下仍可下载。本轮
**没有单独下 0.3.46**，而是按 **0.3.45 → 0.3.47** 一次性比对——bundle 是累积的，
0.3.47 的产物包含了 0.3.46 的全部变更，所以结论对两个版本同时成立。release notes 报的是「更清楚的登录指引 + 临时账号中断的
自动恢复」「启动被另一实例或存储问题挡住时的可读提示」——两项都在协议面外。

### 84.1 版本核实

| 复核项 | 0.3.45 | 0.3.47 |
| --- | --- | --- |
| `latest.yml` `version` | 0.3.45 | **0.3.47** |
| `releaseDate` | 2026-11-22T04:13Z | **2026-11-23T04:38:15Z** |
| x64 `sha512` / `size` | `ASQA2PXh…` / 490,374,704 B | `P2KH/6vu…` / **490,430,232 B** |
| arm64 `sha512` / `size` | `f9ZAhJxu…` / 470,267,728 B | `i9KC2Zf4…` / **470,326,912 B** |
| `app.asar` `package.json` | 0.3.45（`t3codeCommitHash: 30429cde7441`） | **0.3.47**（`875c2277c21c`） |
| `app.asar` / `server.asar` sha256 | — | **`8156f7da…` / `01d06130…`**（安装后与本机文件逐字节一致） |
| 本机 `ProductVersion` | 0.3.45.0 | 复核时 0.3.45.0；复核后已升级到 **0.3.47.0** |

下载 `DeepAI-Code-0.3.47-x64.exe`（490,430,232 B）→ sha512 与 `latest.yml` 逐字节一致
→ 7z 解 NSIS 的 `$PLUGINSDIR/app-64.7z` → `resources/{app,server}.asar`。

### 84.2 结论：推理协议面仍然零变更，只跟心跳版本号

`server.asar` → `apps/server/dist/binCli-BfO7YMNb.mjs`（14,906,175 B，0.3.45 是
`binCli-C6U4jU1g.mjs` 14,896,813 B），全量 diff **42 hunk / +213 / −213**；
按 `//#region` 归属 **1755 → 1757 个源模块**，只有 **15 个变 + 2 个新增**，其余字节级不变。

| 复核项 | 0.3.45 | 0.3.47 | 判据 |
| --- | --- | --- | --- |
| `DEEPAI_LANE_HEADERS` 整个对象 | `Theos: "7"` + `Instance: randomBytes(16).hex` | **内容 sha256 相同** | 片段比对 |
| `X-*` 头字面量 | 14 个（含 5 个 `X-DeepAI-Code-*`） | **集合零差异** | 字面量集合 diff |
| `/deepai_code/*` 端点 | 推理 / `/models` / `/theos` / 搜索 / `sessions/*` / `connect/*` | **完全一致**（连 `/deepai_code_instructions` 等非推理串都在） | 字面量集合 md5 相同 |
| 聊天请求体 | 由 `pi-ai` 的 `dist/api/openai-completions.js` 构造 | **该文件 sha256 `4b3875e5…` 与 0.3.45 完全一致**；`pi-ai` / `pi-coding-agent` 仍 **1.3.0** | 解包 `node_modules` |
| `max_completion_tokens` / `reasoning_effort` | 2 / 9 处 | **2 / 9 处**，相关片段逐字一致 | 字面量计数 |
| `previous_response_id` / `item_reference` | 0 / 0 | **0 / 0** | 全库 grep |
| beacon `v=` | `v=${version$2};up=…;pi=…;focus=…` | **同一逻辑**，`version$2` 报 0.3.47 | 源码级 |
| 机器指纹 | `machineId.ts` | **不在 15 个变更模块里**（算法与 Linux `parseLinuxMachineId` 未动） | region hash |
| 凭据文件 schema | `{apiKey,username,isPro,connect?,connectUnavailable?}` | **未动**（`contracts/deepai.ts` 只改状态结构与 `retryAccount` 入参） | Schema diff |

→ 结论：`deepai.AppVersion` 跟到 **0.3.47**，`LaneVersion` 保持 `"7"`，协议逻辑、
请求头、请求体与指纹都不改。网关侧无需改动即可吃 0.3.47 桌面端写出的凭据。

### 84.3 这一轮真正改的：错误人话化 + 账号恢复提示（协议面外）

15 个变更模块 + 2 个新增（`packages/shared/src/startupFailure.ts`、`src/managed/userActivity.ts`）：

- `src/deepai/api.ts`：拒答码白名单加 `theos_insufficient` / `pro_required` /
  `rate_limit_exceeded`（**只进 Sentry 的 `error_codes` / `rejection` tag，不上行**）；
  新增 `deepAIAccountRecovery()`（拒答码 → `sign_in` / `social_sign_in` /
  `verify_email` / `unavailable` 四种面向用户的提示，注释明写「不暴露执法决定」）与
  `deepAIAccountDiagnosticTags()`（统一 tag 结构）。**车道头、`deepAIBeacon`、
  `checkInHeaders()`、`/theos` 分支 0 hunk。**
- `src/deepai/DeepAIAccount.ts`：`/theos` 被拒时置 `allowanceStale` + `accountRecovery`
  + `accountRefusal`，`ensureCheckedIn` 据此**在客户端侧直接失败**、不再打上游、
  也不再刷新额度；`/theos` 失败补一次性 `reportError`；新增 `credentialLock`
  把 `saveCredentials` / `logout` / `connect` 续期串行化；`connect` 会话按
  `session_expires_at` 续期（只在内存与凭据文件里改 `expiresAt`）。
- 启动 / 存储错误人话文案：`startupFailure.ts`（新增）、`persistence/Errors.ts`
  （SQLite 13/`ENOSPC`/`EDQUOT` → 磁盘满、8/`EROFS` → 只读，且**不回显 SQL / 路径**）、
  `orchestration-v2/UserFacingErrors.ts`、`observability/StartupErrorReporting.ts`。
- 托管云工作区契约：`contracts/managedConnection.ts` 增 `cloud-status` / `cloud-create` /
  `cloud-start` 三个 action、`workspace{id,desired,observed,revision,host_id,error}`、
  `session_expires_at`、`errorCode` / `statusCode` / `retryAfterMs`。
- 其余：`provider-pi` 的 `server_adapter.ts`（`now$62…$64` 变量重编号 + 传输错误文案）、
  `auth/RpcAuthorization.ts`、`binCli.ts`、`config.ts`（`ServerDirectoryError`）、
  `managed/hostAdapter.ts`、`shared/managed/{control,host}.ts`。

**对网关的可观测含义**：0.3.47 客户端在 `/theos` 被拒后会**跳过推理请求**，
所以同一账号在桌面端会「安静地不发请求」。网关不跟这条：本项目的策略仍是
「拒绝码是账号终态 → 标记后跳过」（`SocialLoginRequired` / `MachineRequired`），
语义等价，只是记录方式不同。

### 84.4 本轮改动

- `internal/model/types.go`：`AppVersion` 0.3.45 → **0.3.47**，车道注释补 0.3.47 复核，
  AppVersion 注释补 0.3.45 → 0.3.47 的差异清单；
- 注释版本号同步：`internal/deepai/client_test.go`、`internal/machine/machine.go`、
  `internal/machine/machine_test.go`、`internal/gateway/chatbody.go`；
- `README.md` / `README.en.md`：应用版本、版本链、车道复核日期、心跳 `v=`、PKCE 对齐版本、
  ToS 注释标注一并升到 0.3.47；
- `docs/recon.md`：顶部新增 2026-11-23 复核条目（0.3.47，含对照表），§1 / §2 /
  §2.3 / §3 / §5.1 / §5.3 的【源码级】标签与车道路由表、`v=0.3.47` 心跳示例同步。

### 84.5 本机升级与清理

用同一个官方安装包静默升级（`DeepAI-Code-0.3.47-x64.exe /S`）：退出码 0，
`DeepAI Code.exe` 的 `ProductVersion` 变为 **0.3.47.0**、`FileVersion` 0.3.47，
`resources/{app,server}.asar` 与安装包内解包产物 sha256 逐字节一致
（`app.asar` `8156f7da…` / `server.asar` `01d06130…`），`server.asar` 内为
`binCli-BfO7YMNb.mjs`，卸载注册项从 `DeepAI Code 0.3.45` 变为 **`DeepAI Code 0.3.47`（单条，
无重复项）**。安装目录 1.4 GB、9682 个文件，除 `resources/git/etc/{hosts,networks,protocols,services}`
四个 git 自带文件外没有早于本次构建（2026-11-23 12:37）的残留时间戳——即 0.3.45 已被
完全覆盖，没有旧版残留目录。

随后清理：安装包（468 MB）、NSIS 解包目录 `$PLUGINSDIR/app-64.7z` 与解开的应用树
（约 1.8 GB）、白盒比对用的两版 `binCli` / `pi` 运行时副本、
`deepai-code-desktop-updater` 缓存里的 `installer.exe`（468 MB，就是本轮 0.3.47 包），
合计约 **2.8 GB**；用户数据 `~/.deepai-code` 与 `AppData/Roaming/deepai-code-v2` 保持不动。

### 84.6 发版与上线（2026-11-23 16:05 CST）

**发版**：`gh release` 走 `sh scripts/release.sh v0.1.23 --publish`，交叉编译 5 个平台
（linux/amd64、linux/arm64、windows/amd64、darwin/amd64、darwin/arm64）+ `checksums.txt`，
`https://github.com/2951461586/DeepAICode2API/releases/tag/v0.1.23`，标题、tag、资产三处一致
（`name` / `tagName` 均为 `v0.1.23`）。

**上线**：服务器 `/root/DeepAICode2API` 从 `30f74ad` fast-forward 到 `e66c28c`
（`git describe` = `v0.1.23`），工作区无本地改动；`sh scripts/deploy.sh v0.1.23` 构建并启动
镜像 `deepaicode2api:local-20261123-v0.1.23-e66c28c`，健康检查一次通过。

- `/healthz`（宿主 `127.0.0.1:10088`，nginx 终止 TLS）：`accounts=17 ready=17 lane=7
  lane_compiled=7 lane_drift=0 models=27 version=v0.1.23`，`machine_ready=true`；
- **心跳版本已跟到**：容器内二进制含 `0.3.47` 字面量（`deepai.AppVersion` 编译进去），
  下次 `/theos` 签到上报的 `v=` 即为 0.3.47；
- 上一版镜像 `local-20261123-v0.1.22-7-gbfe30d9-bfe30d9` 保留（27.4 MB），回滚：
  `DEEPAI2API_IMAGE=deepaicode2api:local-20261123-v0.1.22-7-gbfe30d9-bfe30d9 docker compose up -d --no-build`；
- 容器内健康检查仍用 `127.0.0.1:10087`，宿主走 `10088`，排障别混；
- 服务器磁盘 `/` 27G/40G（72%），`local-*` 镜像共 3 个（各 27.4 MB），未做额外清理。

---

## 85. 2026-11-24 客户端 0.3.48 同步复核：启动配对令牌 + 目录权限文案轮，协议面仍零变更（v0.1.24）

**触发**：例行扫描。官方 `latest.yml` 从 **0.3.47** 跳到 **0.3.48**
（`releaseDate` 2026-11-23T07:36:17.026Z，距 0.3.47 的 04:38Z 不到 3 小时）。release notes 只有两条：
「修启动时被误要求 pairing token」「电脑权限阻止启动时的恢复指引更清楚」——都在协议面外，
但按惯例仍做全量白盒比对，而不是只看 release notes。

### 85.1 版本核实

| 复核项 | 0.3.47 | 0.3.48 |
| --- | --- | --- |
| `latest.yml` `version` / `releaseDate` | 0.3.47 / 2026-11-23T04:38:15.028Z | **0.3.48 / 2026-11-23T07:36:17.026Z** |
| x64 包 | 490,430,232 B | 490,364,120 B（sha512 `tjgx1vgu…`，与 `latest.yml` 逐字节一致） |
| arm64 包 | 470,326,912 B | 470,313,240 B（sha512 `WoR4oB1c…`） |
| `app.asar` | 51,026,363 B | 51,026,910 B（+547） |
| `server.asar` | 202,965,682 B | 202,966,040 B（+358） |
| `app.asar` `package.json` | 0.3.47（`t3codeCommitHash: 875c2277c21c`） | **0.3.48（`aad4d707c8c2`）** |
| `server.asar` `package.json` | 0.3.47 | 0.3.48（依赖清单逐项一致，只有 `version` 变） |
| 本机 `ProductVersion` | 0.3.47.0 | 复核后静默升级到 **0.3.48.0**（`FileVersion` 0.3.48） |

本轮机器上没有装 asar 工具链，用一个临时 Node 脚本直接按 asar 头结构读文件
（先读 8 字节头拿 pickle 长度，再解出里面的 JSON 头，按 `offset` 定位数据），
只抽需要的几个文件，不落盘 200 MB 全量解包。

### 85.2 协议面复核（逐项）

| 复核项 | 0.3.47 | 0.3.48 | 判据 |
| --- | --- | --- | --- |
| `DEEPAI_LANE_HEADERS` 整块 | `{Theos:"7", Instance:randomBytes(16).hex}` | 逐字节一致（块 sha256 相同） | 源码级 |
| `X-DeepAI-Code-*` 头字面量 | 5 个（Theos / Instance / Machine / Check-In / Beacon） | 集合零差异 | 源码级 |
| `/deepai_code/*`、`/get_usage_summary`、`/speech_to_text` 端点 | — | 字面量与出现次数全部一致 | 源码级 |
| 心跳 `deepAIBeacon()` | `v=${version$2};up=…;pi=…;focus=…` | **逐字节一致**，只有 `version$2` 报 0.3.48 | 源码级 |
| 机器指纹 | `machineId.ts` 逻辑 | 未进变更行（`binCli` 只有 1 行语义改动） | diff 级 |
| 聊天请求体 | 由 `pi-ai` 的 `openai-completions.js` 构造 | pi 仍 1.3.0，该文件 sha256 `4b3875e5…` **完全相同** | 字节级 |
| `binCli` 全量 diff | — | **4 hunk / +5 / −4** | 字节级 |
| `//#region` 源模块数 | 1757 | **1757**（零增删） | 源码级 |

`binCli` 的 4 个 hunk 里 3 个是构建噪声（Sentry debugId、`import "./package-*.mjs"` 的 chunk 名、
`sourceMappingURL`），唯一语义改动见 85.3。

### 85.3 唯一的语义变更：`directory_permission`

`server.asar` → `apps/server/dist/binCli-sW_tgWwL.mjs`（14,906,342 B，0.3.47 为
`binCli-BfO7YMNb.mjs` 14,906,175 B），`classifyStartupFailure()` 开头新增一行：

```js
if (classes.some((item) => item.startsWith("ServerDirectoryError:")) &&
    classes.some((item) => /:(?:EACCES|EPERM):mkdir$/.test(item))) return "directory_permission";
```

桌面端同步（`app.asar` 2320 个条目里只有 4 个变）：

| 文件 | 0.3.47 | 0.3.48 | 改动 |
| --- | --- | --- | --- |
| `apps/desktop/dist-electron/main.cjs` | 5,726,524 B | 5,727,043 B | 6 hunk / +16 / −9 |
| `apps/desktop/dist-electron/preload.cjs` | 32,111 B | 32,139 B | 1 行 |
| `apps/desktop/dist-electron/snapShot-Ds58oKsV.cjs` | 同尺寸 | 同尺寸 | 仅 Sentry debugId |
| `package.json` | 0.3.47 | 0.3.48 | 版本号 |

`main.cjs` 的两处实质改动：

1. `startupFailureMessages` 新增 `directory_permission` 的用户文案（对应 release notes 第 2 条）；
2. 本地环境 bearer token 支持「被拒即失效重取」——原来的单次 memo `getBearerToken` 改成
   `readBearerToken(rejectedToken)`：缓存命中且 `cached.value !== rejectedToken` 才复用，否则清缓存重取；
   新增 `refreshBearerToken`，IPC payload 从 `Void` 改成 `UndefinedOr(String)`，`preload.cjs` 透传
   `rejectedToken`（对应 release notes 第 1 条）。两处都在桌面端本地鉴权 / IPC 侧，不进推理链路。

### 85.4 其余差异全是构建噪声

- `server.asar` 15,514 个条目：同路径内容变化 8 个——`binCli`（换新文件名）、`bin.mjs` /
  `claude-history-worker.mjs`（只换 `import` 的 chunk 名与 debugId）、`client/index.html` 与
  `client/.vite/manifest.json`（只换 chunk 哈希）、`node_modules/.modules.yaml` 与
  `.pnpm-workspace-state-v1.json`（构建元数据）、`package.json`（版本号）。
- 另有 **1937 个 chunk 改名**：每个 chunk 都内嵌本次构建随机的 Sentry debugId，所以一次发版
  会让几乎所有 chunk 的内容哈希连锁变名。按「同名去哈希」一一配对（1937 对，无一落单）后，
  差异只剩 debugId / `sourceMappingURL` / 互相引用的哈希。
- 归一化后仍有数十个文件不同，逐个看下来都是压缩器变量名置换（同一 chunk 里 `st` ↔ `ot`、
  `D` ↔ `O` 之类），指令与字面量集合不变。
- 结论：`deepai.AppVersion` 跟到 **0.3.48**，`LaneVersion` 保持 `"7"`，协议逻辑 / 请求头 /
  请求体 / 指纹全不改，网关无需改动即可吃 0.3.48 桌面端写出的凭据。

### 85.5 代码与文档落地

- `internal/model/types.go`：`AppVersion` 0.3.47 → **0.3.48**；`LaneVersion` 注释补 2026-11-24 复核；
  AppVersion 注释补 0.3.47 → 0.3.48 的差异清单（4 hunk、1757 模块、唯一语义改动）；
- `internal/machine/machine.go` / `machine_test.go`、`internal/deepai/client_test.go`、
  `internal/gateway/chatbody.go`：复核版本列表补 0.3.48；
- `docs/recon.md`：顶部新增 2026-11-24 复核条目（0.3.48，含对照表），`【源码级 0.3.47】`
  标签、`v=0.3.47` 心跳示例、机器指纹与 PKCE 的复核日期同步到 0.3.48；
- `README.md` / `README.en.md`：应用版本、车道复核日期、`DEEPAI2API_LOGIN_PKCE` 对齐版本、
  心跳 `v=` 现值全部跟到 0.3.48（顺手清掉 §一 里重复的一行 `sessions/save_records`）。

### 85.6 本机升级与清理（2026-11-24 08:30 CST）

用同一个官方安装包静默升级（`DeepAI-Code-0.3.48-x64.exe /S`）：退出码 0，耗时 185 s；
`DeepAI Code.exe` 的 `ProductVersion` 变为 **0.3.48.0**（`FileVersion` 0.3.48），
卸载注册项单条 `DeepAI Code 0.3.48`（无重复项），安装目录
`%LOCALAPPDATA%\Programs\deepai-code-desktop` 9682 个文件 / 1,386,573,614 B；
`app.asar` / `server.asar` 尺寸与包内一致（51,026,910 / 202,966,040）；除 `resources/git`
自带的 hosts / networks / protocols / services 外，没有早于本次构建（2026-11-23 15:35:54）
的残留时间戳——即 0.3.47 已被完全覆盖。

随后清理：updater 缓存里的 `installer.exe`（490,364,120 B，就是本轮 0.3.48 包）、
白盒比对用的临时解包目录（安装包 + NSIS `app-64.7z` + 解开的应用树 + 抽取的 `binCli` /
`pi-ai` 副本，约 2.3 GB），合计约 **2.7 GB**；用户数据 `~/.deepai-code` 与
`AppData/Roaming/deepai-code-v2` 保持不动。

### 85.7 发版与上线（2026-11-24 08:48 CST）

**发版**：`sh scripts/release.sh v0.1.24 --publish`，交叉编译 5 个平台（linux/amd64、linux/arm64、
windows/amd64、darwin/amd64、darwin/arm64）+ `checksums.txt`，
`https://github.com/2951461586/DeepAICode2API/releases/tag/v0.1.24`，标题 / tag / 资产三处一致
（`name` / `tagName` 均为 `v0.1.24`）。**踩坑**：先打了本地 tag 没推就给 `--publish`，
`gh release create` 会直接报「tag exists locally but has not been pushed」；
本轮的顺序是 `git push origin main` → `git push origin v0.1.24` → 再跑发布脚本。

**上线**：服务器 `/root/DeepAICode2API` 从 `e66c28c` fast-forward 到 `589266d`
（`git describe` = `v0.1.24`），工作区无本地改动；`sh scripts/deploy.sh v0.1.24` 构建并启动
镜像 `deepaicode2api:local-20261124-v0.1.24-589266d`，健康检查一次通过。

- `/healthz`（宿主 `127.0.0.1:10088`，nginx 终止 TLS）：`accounts=17 ready=17 lane=7
  lane_compiled=7 lane_drift=0 models=27 version=v0.1.24`，`machine_ready=true`；
  刚重启时 `lane_upstream=0`（`/theos` 探测尚未完成），几分钟后回到 7；
- **心跳版本已跟到**：容器内 `/usr/local/bin/deepaicode2api` 的 `strings | grep -o '0\.3\.[0-9]*'`
  只输出 `0.3.48`（无 0.3.47），即下次 `/theos` 签到的 `v=` 就是 0.3.48；
- 上一版镜像 `local-20261123-v0.1.23-e66c28c` 保留（27.4 MB），回滚：
  `DEEPAI2API_IMAGE=deepaicode2api:local-20261123-v0.1.23-e66c28c docker compose up -d --no-build`；
- 容器内健康检查仍用 `127.0.0.1:10087`，宿主走 `10088`，排障别混；
- 服务器磁盘 `/` 25G/40G（65%），`local-*` 镜像 2 个（各 27.4 MB），未做额外清理。

---

## 86. 2026-11-24 后台签到预热：补错误日志、预算 10s → 20s、连续失败退避（v0.1.25）

**触发**：v0.1.24 上线后的运行巡检。`签到预热未完成（不影响推理）` 每 30s 一条，
18 分钟里 16 次，且**每次都恰好 `ms≈10002 / timed_out=true`**。

**定位过程（结论：既不是账号限流，也不是出口坏）**：

- 账号侧：无 403/429、没有 `Requests are temporarily limited`；`balance=178`、
  `lane_version=7`；`POST /admin/api/checkin` 手动探测 **1.06s 成功**，`failures` 归零，
  `quota_due` 变 false。进程内 `checkin.blocked=1` 那个限流命中是**另一个号**
  （`lizi45***`，`last_err` 是推理限流），与预热无关。
- 出口侧：`proxy_cooling=0`；同一条出口 + 同一个号手动 `/theos` 1.06s；
  启动期那 6 条 `出口代理探测失败` 全在别的出口（`fireside:71xx`）。
- 症状会**漂移**：18 分钟里 `ming49798 ×10`、`migueladorno236 ×5`、`zhuhaoyi181 ×1`，
  每轮换「下一个到期账号」继续超时 —— 与「某个号被封 / 被限流」不符。
- 时长恒等于预算 → 是 ctx deadline，不是上游拒绝（拒绝会秒回 403/429 并上
  `CheckInRefusedLock` 静默锁，之后就不再重试）。
- 同客户端对比：热路径指标 `checkin attempts=7 / p50=500ms / p95=1s / timeouts=1`，
  而预热 **16/16 全超时**。

**根因**：预热挑的都是「到期但闲置」的号，连接是**冷的**；住宅代理冷启动实测 6–8s
（`internal/server/proxyhealth.go` 里 `proxyProbeTimeout` 的注释本来就记着这个数），
加 TLS 与上游 RTT，10s 预算正好卡在临界值 → 每轮必超时；失败后只有
`attemptAt` 的 30s 最小间隔，于是变成「每 30s 白打一次 `/theos` + 刷一条日志」的
死循环。热路径与手动探测走的是热连接，所以从来没暴露。

### 86.1 改动（`internal/server/warmcheckin.go`、`tasks.go`）

1. **补错误日志**：`签到预热未完成` 原来只有 `ms` / `timed_out`，分不清「我方预算」
   还是「CONNECT 被重置 / 上游 5xx」。现在带 `err` 与 `fails`。
2. **预算 10s → 20s**：`warmCheckInBudget = 20s`；轮级 ctx 60s → 120s
   （8 个号 / 2 并发 × 20s = 80s 最坏情况装得下）。
3. **连续失败退避**：新增 `warmCooldown`（按账号 ID，零值可用，4096 条上限并顺带
   清理过期项），连续失败 1/2/3/4/5+ 次 → 2m / 4m / 8m / 16m / 20m 不再预热，
   成功一次立即清零。**只影响预热**：热路径 `EnsureCheckIn` 的语义与预算不变。

### 86.2 测试

- `TestWarmCooldownBackoffCurve`：退避曲线 2/4/8/16/20/20 分钟、窗口内外放行判据、
  成功清零；
- `TestWarmCooldownZeroValue`：零值可用；
- `TestWarmCheckInsHonoursCooldown`：退避中的账号这一轮上游请求数必须为 0
  （这条直接锁住「不再每 30s 白打一次」）；
- 既有 `TestWarmCheckInsConsumesDueOnce` / `TestWarmCheckInsSkipsDisabled` 保持通过；
- `gofmt` / `go build` / `go vet` / `go test ./...` 全绿。

### 86.3 巡检时顺带发现（未在本轮处理）

- **请求体闸门 WARN 34 次 / 18 分钟**（v0.1.24 新增的字节闸门）：
  `bytes=490,787` 与 `1,146,714`，而 `limit=196,608`（192 KiB）—— 这些请求**已经超过
  上游静默截断阈值**（issue #7），模型 `deepseek-v4.1-flash`，端点 `messages` / `responses`。
  这是「上下文已在被截断（失忆）」的真实信号，值得单独查是哪个 key / 会话在发超长上下文。
- 进程内 `upstream_errors`：`/chat/completions code=transport ×5`（重启后 18 分钟），
  与账号的 `last_err: Gateway Timeout` 同源，属出口/上游瞬断。

### 86.4 发版、上线与效果观察（2026-11-24 09:40 CST）

**发版**：`v0.1.25`（`71c325b`），5 平台交叉编译 + `checksums.txt`，
`https://github.com/2951461586/DeepAICode2API/releases/tag/v0.1.25`，标题 / tag / 资产一致。

**上线**：服务器 `/root/DeepAICode2API` 从 `589266d` fast-forward 到 `71c325b`，
`sh scripts/deploy.sh v0.1.25` 构建并启动镜像
`deepaicode2api:local-20261124-v0.1.25-71c325b`，健康检查一次通过：
`version=v0.1.25`、`accounts=17 ready=17 lane=7 lane_upstream=7`。

**效果（上线后 5 分钟）**：5 轮预热共消化 **8 + 8 + 7 + 7 + 1 = 31 个到期账号**，
`签到预热未完成` **0 次**（改前是每 30s 一条、16/16 全超时）；账号侧
`quota_due` 全为 false，`/healthz` 的 `quota_fresh` 4（半衰期内的号本来就只 1~4 个）。

→ 反证了 86 节的判断：冷连接确实卡在 10s 预算边缘，20s 之下全部一次成功；
退避这一轮没被触发（没有失败可退），属于兜底而不是主路径。

**回滚**：`DEEPAI2API_IMAGE=deepaicode2api:local-20261124-v0.1.24-589266d docker compose up -d --no-build`

---

## 87. 2026-11-25 客户端 0.3.50 同步复核：agent 运行时迁移（pi → dsh），协议面仍零变更；模型目录 27 → 28（v0.1.26）

**触发**：例行扫描。官方 `latest.yml` 从 **0.3.48** 跳到 **0.3.50**
（`releaseDate` 2026-11-24T05:37:49.435Z）。release notes 说明本轮「恢复 skills / 斜杠命令 /
goal mode / 压缩 / 用量统计 / 托管模型设置」，看起来是功能轮；白盒比对后发现底层是
**agent 运行时整体迁移**（`@earendil-works/pi-*` → `@deepseek-ai/dsh-*`），所以逐项复核了协议面。

### 87.1 版本核实

| 复核项 | 0.3.48 | 0.3.50 |
| --- | --- | --- |
| `latest.yml` `version` / `releaseDate` | 0.3.48 / 2026-11-23T07:36:17Z | **0.3.50 / 2026-11-24T05:37:49Z** |
| x64 包 | 490,364,120 B | 509,379,384 B（sha512 `gFLL4LJN…`，与 `latest.yml` 逐字节一致） |
| arm64 包 | 470,313,240 B | 488,534,408 B（sha512 `dUy0eEHY…`） |
| `app.asar` | 51,026,910 B / 2320 条目 | 46,667,328 B / **2485 条目**（新增 vendored undici 8.11.2 等） |
| `server.asar` | 202,966,040 B / 15,514 条目 | 279,410,481 B / **26,602 条目** |
| `app.asar` `package.json` | 0.3.48（`t3codeCommitHash: aad4d707c8c2`） | **0.3.50（`331d230b9e9e`）** |
| `binCli` | 14,906,342 B / 1757 个 `//#region` 模块 | 15,004,074 B / **1770 个** |
| 本机 `ProductVersion` | 0.3.48.0 | 复核后静默升级到 **0.3.50.0** |

### 87.2 协议面复核（逐项）

| 复核项 | 0.3.48 | 0.3.50 | 判据 |
| --- | --- | --- | --- |
| `DEEPAI_LANE_HEADERS` 整块 | `{Theos:"7", Instance:randomBytes(16).hex}` | **逐字节一致**（车道仍 7，未抬到 8） | 源码级 |
| `X-DeepAI-Code-*` 头字面量 | 5 个（Theos / Instance / Machine / Check-In / Beacon） | **集合零差异** | 源码级 |
| `/deepai_code/*`、`/get_usage_summary`、`/speech_to_text` 端点 | — | **字面量集合零差异**；只多一个本地回环归一化 `/v1/chat/completions` | 源码级 |
| 心跳 `deepAIBeacon()` | `v=${version$2};up=…;pi=…;focus=…` | **逐字节一致**，只有 `version$2` 报 0.3.50 | 源码级 |
| 机器指纹 | `machineId.ts` 逻辑 | **区域 sha256 完全相同**（`d8750df9…`） | 源码级 |
| 聊天请求体 | 由 `pi-ai` 的 `openai-completions.js` 构造 | pi 仍 **1.3.0**；该文件 `4b3875e5…` → `f0dd1d00…`，差异**只有 5 行 `extra_content` 回填**（仅当上游下发过工具调用 `extra_content` 时才出现） | 字节级 |
| `previous_response_id` / `item_reference` | 0 / 0 | **0 / 0** | 源码级 |
| `/theos` 请求形态 | `checkInHeaders()` + 3 个头 | **未变**（只把时间源从 wall clock 换成 monotonic + wall 双计） | 源码级 |

`binCli` 全量 diff **1012 hunk / +4335 / −2098**，151 个 `//#region` 有改动；但落在
`DEEPAI_LANE_HEADERS`、`machineId.ts`、`deepAIBeacon`、`mountDeepAIApi` 的普通头与
`/theos` 分支上的 hunk 是 **0**。改动集中在 `src/orchestration-v2/*`、`src/deepai/*`
（新增 harness 路由）、`packages/contracts/src/*` 与 `packages/provider-*`。

### 87.3 本轮真正的变化：运行时迁移 + 两个要登记的细节

1. **运行时迁移**：`server.asar` 新增 `@deepseek-ai/dsh-*`（0.2.3-alpha.2）整套运行时与
   `@deepseek-ai/dsh-llm-pi-ai`（内含 pin 住的 pi-ai 1.3.0），移除旧的顶层
   `@earendil-works/pi-ai` 依赖；新增 `src/deepai/DeepSeekHarness.ts` /
   `DeepSeekHarnessDriver.ts` / `HarnessRouter.ts` / `hostedHarness*.ts` /
   `HostedOpenCodeDiscovery.ts` / `DeepAIOpenCodeDriver.ts`。
   `HarnessRouter` 把 `minimax-m3` 固定到 OpenCode harness、把 `DEEPSEEK_HOSTED_MODELS`
   固定到 DeepSeek（dsh）harness；`localTransport` 用 scoped capability 限制每个 harness
   只能访问 `/chat/completions` 与其模型集合。全在本地编排面。
2. **默认上游换成 `https://api2.deepai.org`**（`T3CODE_DEEPAI_API2=false` 退回）。
   实测两个 origin 行为一致：车道 7 均 401 `auth_required`、8 均 426；
   `/theos` 对同一账号的「temporarily unavailable」在两边也一致。网关默认仍用
   `api.deepai.org`，需要时用 `DEEPAI_API_ORIGIN` 切换，本轮不改默认值。
3. **`REASONING_REPLAY_MODELS`（10 个）打开 pi-ai 的
   `requiresReasoningContentOnAssistantMessages`**：assistant 历史缺 `reasoning_content`
   时补空串。这是客户端构造历史的行为，网关只透传，不改请求语义。

### 87.4 模型目录 27 → 28

线上 `/v1/models`（重启后刷新）为 **28 条：12 免费 / 16 付费**。与 11-18 快照逐字段比对，
只多一个免费模型：

| ID | 名称 | 上下文 | 最大输出 | Theos/次 | input |
| --- | --- | ---: | ---: | ---: | --- |
| `claude-haiku-5-5` | Claude Haiku 5.5 | 95,000 | 64,000 | **0** | text+image |

其余 27 条 `id` / `name` / `contextWindow` / `maxTokens` / `theosPerRequest` / `input` / `api`
全部未变，`default` 仍 `glm-5.3-flash`，无下线。README 两张模型表已补该行并改成 28。

### 87.5 代码与文档落地

- `internal/model/types.go`：`AppVersion` 0.3.48 → **0.3.50**；`LaneVersion` 保持 `"7"`，
  注释补 2026-11-24 双版本复核；AppVersion 注释补 0.3.48 → 0.3.50 的差异清单；
- 版本链同步：`internal/machine/machine.go` / `machine_test.go`、`deepai/client_test.go`、
  `gateway/chatbody.go`（请求体构造者文件从「字节级一致」改为「只多 `extra_content` 回填」）；
- `README.md` / `README.en.md`：应用版本、车道复核日期、`DEEPAI2API_LOGIN_PKCE`、
  心跳 `v=`、模型表（28 个）与启动样例（`28 models`）；
- `docs/recon.md`：顶部新增 0.3.50 复核条目与对照表，`【源码级】` 标签同步；
- `scripts/release.sh`：原来 `gh release upload "$VER" dist/*` 会把 dist 里**历史版本的
  release notes**（已 gitignore、长期累积）一起挂到每个 Release 上。改成只上传
  「本版二进制 + checksums + 本版 notes」，并顺手删掉 v0.1.26 上 13 份旧 notes 资产。

### 87.6 本机升级与清理（2026-11-24 22:36 CST）

用官方 NSIS 安装包静默升级（`DeepAI-Code-0.3.50-x64.exe /S`）：退出码 0，
`DeepAI Code.exe` 的 `ProductVersion` 变为 **0.3.50.0**（`FileVersion` 0.3.50）。

**安装形态变了**：0.3.50 改成 **machine-wide** 安装 —— `C:\Program Files\DeepAI Code`
（9,696 个文件 / 1,494,555,341 B），卸载项 `"DeepAI Code 0.3.50" /allusers`，注册表只有一条；
旧的 per-user 安装 `%LOCALAPPDATA%\Programs\deepai-code-desktop` 已被安装器删除，
没有残留目录。

清理：updater 缓存里的 `installer.exe`（509,379,384 B，就是本轮安装包）+
白盒比对用的临时解包目录（两个安装包、两个 NSIS `app-64.7z`、两套应用树与抽取的
`binCli` / `openai-completions.js`，共 **2.5 GB**）。用户数据 `~/.deepai-code` 与
`AppData\Roaming\deepai-code-v2` 保持不动。

### 87.7 发版与上线（2026-11-24 22:45 CST）

**发版**：`v0.1.26`（`635af25`），5 平台交叉编译 + `checksums.txt`，
`https://github.com/2951461586/DeepAICode2API/releases/tag/v0.1.26`，标题 / tag / 资产一致
（资产只有本版 6 个文件 + 本版 notes，已无历史 notes 噪声）。

**上线**：服务器 `/root/DeepAICode2API` 从 `71c325b` fast-forward 到 `635af25`，
`sh scripts/deploy.sh v0.1.26` 构建并启动镜像
`deepaicode2api:local-20261124-v0.1.26-635af25`，健康检查一次通过：

- `/healthz`：`accounts=17 ready=17 lane=7 lane_compiled=7 lane_drift=0 models=28
  version=v0.1.26`、`machine_ready=true`；容器内 `strings` 只输出 `0.3.50`（无 0.3.48 残留）；
- **`lane_upstream` 仍为 0**：不是本次部署的问题 —— 手工触发两批 `/admin/api/checkin`
  （共 3 个号）全部返回 `查询 Theos: DeepAI models are temporarily unavailable. Try again later.`，
  同一账号在 `api.deepai.org` 与 `api2.deepai.org` 上结果一致；同时 `/v1/models` 刷新与
  `/v1/chat/completions` 推理都正常（启动后已拉到 28 个模型、指纹测试的聊天请求 200）。
  账号缓存的 `theos.lane_version` 仍是 7，`/theos` 恢复后 `lane_upstream` 会自动回到 7；
- 镜像只保留 2 个：当前 `local-20261124-v0.1.26-635af25` 与回滚用的
  `local-20261124-v0.1.25-71c325b`（各 27.4 MB），无更早的旧镜像。
- 回滚：`DEEPAI2API_IMAGE=deepaicode2api:local-20261124-v0.1.25-71c325b docker compose up -d --no-build`

### 87.8 模型指纹实测（0.3.50 上线后）

方法沿用 `docs/model-fingerprint-2026-11-10.md`：`npx lmfpd@latest`，`-a chatcompletion`、
`-p 1`（顺序采样）、`--count 3`；`gpt-6-astra` 需 `-a responses`（本轮未测，50 Theos/次）。
为不污染线上密钥表，临时建了一把 `fingerprint-check-20261124` 密钥，测完即删。

| 模型 | 有效样本 | 概率状态 | 判定为 | 家族 | 结论 |
| --- | ---: | --- | --- | --- | --- |
| `claude-haiku-5-5`（新） | **3/3** | reference_calibrated，p=0.296 | claude-fable-5.2 | **claude** | ✅ 家族对齐（参考库里没有 haiku，只能到家族级） |
| `qwen-3.8-flash`（对照） | **3/3** | reference_calibrated，p=0.598 | gpt-6-astra | gpt | ❌ 与目录名不符（与 11-10 的 qwen3.8-flash 0.912 也不同） |
| `glm-5.3-flash`（对照） | 2/3 | unavailable | grok-4.6 / openpangu-2.0-flash（两次跑） | grok / openpangu | ⚠️ 无法判定：一次样本首字节超 180s |
| `deepseek-v4.1-flash`（对照） | 2/3 | unavailable | gemini-4-carbon | gemini | ⚠️ 无法判定：一次样本空输出 |

判读注意：

- 检测器参考库本轮是 **76 个模型的 `shared-detector-v2`**（`reference_sha256 89f2a3ea…`），
  与 11-10 那次不是同一个库，**跨轮绝对值不可直接比较**；能比较的是「同一轮里 3/3 样本
  给出的家族是否与目录名一致」；
- 空样本不是网关 bug：日志里对应请求多为 `499 context canceled`（lmfpd 的首字节超时）或
  一次 `502 stream error: stream ID 31; INTERNAL_ERROR; received from peer`（上游 31 分钟后
  断流）；非流式（`-ns`）在 90s 预算下全部超时（线上非流式 p95 本身就是 300s），说明
  长提示下上游首字节/吞吐偶发不稳，属于上游侧抖动；
- 结论：**新模型 `claude-haiku-5-5` 的家族指纹对齐**；但**整个目录“指纹对齐”不能宣称成立** ——
  对照里 `qwen-3.8-flash` 3/3 被判成 GPT 家族，`glm-5.3-flash` / `deepseek-v4.1-flash`
  样本不足。这与 11-10 的历史结论一致：部分目录名并不对应真实路由，检测器只能作为参考，
  要固定参数、多轮重复再下结论。

---

## 88. 2026-11-25 客户端 0.3.52 同步复核：车道 7 → 8 + 新增自证头 `X-DeepAI-Code-Proof`（v0.1.27）

**触发**：例行扫描。官方 `latest.yml` 从 **0.3.50** 跳到 **0.3.52**
（`releaseDate` 2026-11-25T00:50:10.886Z）。release notes 是修 bug 轮（中文等非拉丁语长会话
压缩后不再失败、部分模型起不来、大图片、Linux 图片提示、输出过多的命令卡顿、切权限模式时
agent 重启、Windows 上模型列表 / secrets 被别的程序占用、安装没完成的 agent 起不来、本地库
损坏导致反复重启、Podman WSL 发行版、无关联程序的链接），但白盒比对发现**协议面本轮真的变了**：
车道抬到 8，并且第一次新增了一个请求头。

### 88.1 版本核实

| 复核项 | 0.3.50 | 0.3.52 |
| --- | --- | --- |
| `latest.yml` `version` / `releaseDate` | 0.3.50 / 2026-11-24T05:37:49Z | **0.3.52 / 2026-11-25T00:50:10Z** |
| x64 包 | 509,379,384 B | **586,881,344 B**（sha512 `j5LBDdKU…`，与 `latest.yml` 逐字节一致） |
| arm64 包 | 488,534,408 B | **565,693,024 B**（sha512 `cx/6sQF2A…`） |
| `app.asar` | 46,667,328 B | **46,674,489 B**（`package.json` 0.3.52，`t3codeCommitHash: 20b44c095c22`） |
| `server.asar` | 279,410,481 B / 26,602 条目 | **269,365,520 B / 26,699 条目** |
| `binCli` | `binCli-COh41gMK.mjs` 15,004,074 B | **`binCli-B89xRT2q.mjs` 15,039,943 B** |

包体变大 77 MB 主要是渲染层（下载页给的仍是 x64 NSIS 单包）：白盒只碰
`resources/{app,server}.asar` 与其中的 `binCli`。

### 88.2 协议面复核（逐项）

比对对象：本机已装的 0.3.50（`C:\Program Files\DeepAI Code
esources\server.asar`）与新包解出的 0.3.52。

| 复核项 | 0.3.50 | 0.3.52 | 判据 |
| --- | --- | --- | --- |
| `DEEPAI_LANE_HEADERS` 整块 | `{Theos:"7", Instance:randomBytes(16).hex}` | **`Theos:"8"`**（实例号粒度不变） | 源码级 |
| `X-DeepAI-Code-*` 头字面量 | 14 个 `X-*` 字面量、5 个 Code 头 | **+`X-DeepAI-Code-Proof`（第 6 个）** | 源码级 |
| `/deepai_code/*`、`/get_usage_summary`、`/speech_to_text`、`/sessions/*` 端点 | — | **字面量集合零差异** | 源码级 |
| 心跳 `deepAIBeacon()` | `v=${version$2};up=…;pi=…;focus=…` | **逐字节一致**，`version$2` 报 0.3.52 | 源码级 |
| 机器指纹 | `machineId.ts` 区域 3571 B | **区域 sha256 完全相同**（`d8750df9…`） | 源码级 |
| 聊天请求体 | 顶层 `pi-ai` `openai-completions.js` | **sha256 `f0dd1d00…` 与 0.3.50 完全相同**（字节级）；`dsh-llm-pi-ai` 内嵌副本变（新增 `openrouter-cost` / `streaming-json`），但 `max_tokens` / `max_completion_tokens` / `reasoning_effort` / `prompt_cache_key` / `reasoning_content` 字段集合不变 | 字节级 |
| `previous_response_id` / `item_reference` | 0 / 0 | **0 / 0** | 源码级 |
| 凭据文件 `deepai-account.bin` | `{apiKey,username,isPro,…}` | **Schema 未动** | 源码级 |

`binCli` 按 `;` 切段做 `SequenceMatcher`：**593 个变更块 / +1594 / −1262 段**。
落在 `src/deepai/*` 的只有 5 个模块：`api.ts`（车道 + 证明请求构造）、`DeepAIAccount.ts`
（`checkInHeaders(apiKey)` 异步化、`callProof`）、`localTransport.ts`（模型调用带头）、
新增 `laneProof.ts` 与 `resourceTelemetry/ResourceMonitorBinary.ts`；其余集中在
`src/orchestration-v2/Orchestrator.ts`（146 块）、`packages/provider-acp`（63 块）、
`provider-pi`（17 块）、katex（33 块）等编排/渲染面。

### 88.3 本轮唯一的新协议元素：`X-DeepAI-Code-Proof`

`src/deepai/laneProof.ts`：值来自随包原生 helper `resource-monitor --attest`
（Rust `native/resource-monitor/src/attest.rs`）用「本构建的密钥」算出的 24–64 位 hex，
一问一答（JSON 行，串行，5s 超时）。

- `/theos`：`{n:签到 nonce | "", a: sha256(apiKey), m: 机器指纹, i: 实例号, v: 版本}`；
- 模型调用（`localTransport` 与 `accountRequest` 的 `/chat/completions`）：
  `{n: 上次签到 nonce, b: sha256(请求体)}`，没有签到 nonce 时不发；
- helper 缺失 / 超时 / 输出不合规 → **省略该头**，请求照发（不影响可用性）；
- 源码注释明确写了 ToS §4：**不允许把 helper 复制进其它客户端**。

**网关的处理：只登记头名（`model.HeaderProof`），不实现、不发送。** 依据是 88.4 的实测 ——
上游当前不校验它；而实现它必须从客户端抠出密钥，正是注释明确禁止的行为。

### 88.4 线上实测（2026-11-25，真实账号，单请求）

| 端点 | lane 6 | lane 7 | lane 8 | lane 9 |
| --- | --- | --- | --- | --- |
| `GET /deepai_code/models` | 拒绝 | 200（28 个） | **200（28 个）** | 拒绝 |
| `GET /deepai_code/theos` | 拒绝 | 200（回 `lane_version: 8`） | **200（回 `lane_version: 8`）** | 拒绝 |
| `POST /deepai_code/chat/completions` | 未测 | 未测 | **200** | 未测 |

- 拒绝文案是 `This version of DeepAI Code is no longer supported. Please update the app.`
  （不是 426 错误码），即**门槛又换了形态：窗口 7 / 8**；
- **Proof 头不被校验**：`/theos` 与 `/chat/completions` 在「不发 / 64 位乱填 / 24 位乱填」
  三种情况下全部 200，`/theos` 照常下发 `check_in` nonce；
- 模型目录 **28 条（12 免费 / 16 付费）逐字段未变**，且**车道 7 与车道 8 拿到的目录完全相同**；
- 线上网关（v0.1.26，车道 7）的 `/healthz` 在复核期间已经报出 `lane_upstream: 8, lane_drift: 1`
  —— 与上面的矩阵一致：旧车道还能用，但已经不是当前车道。

### 88.5 代码与文档落地

- `internal/model/types.go`：`AppVersion` 0.3.50 → **0.3.52**、`LaneVersion` `"7"` → **`"8"`**；
  `LaneVersion` 注释补 2026-11-25 的窗口形态实测；`AppVersion` 注释补 0.3.50 → 0.3.52 的差异清单；
  新增 `HeaderProof`（只登记，附「不实现」的理由与实测结论）；
- 测试锚点：`deepai/client_test.go`（车道 8）、`observe/metrics_test.go`（`gateway_lane 8`）、
  `server/lane_test.go`（探测场景 7→9 上移一档，保留「旧状态文件 6 → 编译值 8」用例）；
- 版本链注释同步：`gateway/chatbody.go`、`machine/machine.go`、`machine/machine_test.go`；
- `README.md` / `README.en.md`：应用版本、车道（含 0.3.52 抬到 8）、车道门槛三次形态、
  `X-DeepAI-Code-Proof` 行、模型目录 11-25 复核结论、PKCE 默认值、心跳 `v=`、ToS 声明版本链；
- `docs/recon.md`：顶部新增 0.3.52 复核块（含 Proof 的请求构造与「上游不校验」实测）、
  §3 头表加 Proof、车道门槛矩阵新增 2026-11-25 行并改写「探测的局限」、`【源码级】` 标签跟进。

### 88.6 本机桌面端升级 0.3.50 → 0.3.52（2026-11-25 15:37 CST，提权安装）

0.3.50 起安装器是 machine-wide，需要提权；本轮由用户确认 UAC 后静默安装：

```powershell
Start-Process -FilePath 'DeepAI-Code-0.3.52-x64.exe' -ArgumentList '/S' -Verb RunAs -Wait
# ExitCode=0
```

**安装形态又变回 per-user**：0.3.52 装到 `%LOCALAPPDATA%\Programs\deepai-code-desktop`，
卸载项是 HKCU 的「DeepAI Code 0.3.52」，**0.3.50 的 `C:\Program Files\DeepAI Code`（machine-wide）
被安装器删掉，HKLM 无残留**。核对结果：

| 项 | 值 |
| --- | --- |
| `DeepAI Code.exe` | `ProductVersion 0.3.52.0` / `FileVersion 0.3.52` |
| `resources/app.asar` | 46,674,489 B（与本轮解包比对的 0.3.52 包一致） |
| `resources/server.asar` | 269,365,520 B（同上） |
| `wsl-runtime.tar.gz` | 195,479,586 B（0.3.50 为 115,961,053 B，安装包增大主要来自这里） |
| 凭据文件 `~/.deepai-code/userdata/secrets/deepai-account.bin` | **未改写**（仍是 2026-11-07 的 88 B，Schema 未变） |

装完用 `go run ./cmd/probe` 直接读桌面端凭据验证：`模型目录: 28 个，默认 glm-5.3-flash`
（首次调用遇到一次上游 `EOF`，重试即成功，属于上游抖动）。安装包与临时解包目录随即清理。

### 88.7 发版与上线（2026-11-25 15:10 CST）

**发版**：`v0.1.27`（`1b24cca`），5 平台交叉编译 + `checksums.txt`，
`https://github.com/2951461586/DeepAICode2API/releases/tag/v0.1.27`（只挂本版资产 + 本版 notes）。

**上线**：服务器 `/root/DeepAICode2API` 从 `635af25` fast-forward 到 `1b24cca`，
`sh scripts/deploy.sh v0.1.27` 构建并启动镜像
`deepaicode2api:local-20261125-v0.1.27-1b24cca`，健康检查一次通过：

- `/healthz`：`accounts=17 ready=17 lane=8 lane_compiled=8 lane_drift=0 lane_upstream=8 models=28
  version=v0.1.27`、`machine_ready=true`；容器内二进制里 `0.3.52` 1 处、`0.3.50` / `0.3.48` 0 处；
- 启动日志：`模型目录已更新：28 个模型，默认 glm-5.3-flash`、
  `[lane] 上游车道: 8（网关当前发 8，编译期 8）`（无 drift 告警）；
- **端到端实测**：临时建一把 `sync-check-20261125` 密钥 → `GET /v1/models` 28 条、
  `POST /v1/chat/completions`（`glm-5.3-flash`）**200** 且返回正文与 `reasoning_details`，
  `/admin/api/overview` 显示 `lane_upstream: 8`；测完立刻删除该密钥（列表回到只剩
  `liziai-deepai`）；
- 镜像：按用户要求**只保留当前版** `local-20261125-v0.1.27-1b24cca`（27.4 MB），
  v0.1.26 / v0.1.25 两个备份镜像都已删除（`docker images` 里只剩这一条）。
- 回滚方式相应变成「按 tag 重建」：`cd /root/DeepAICode2API && git checkout v0.1.26 &&   GIT_TAG=v0.1.26 sh scripts/deploy.sh v0.1.26`（脚本会校验 tag 与 HEAD 一致）。

> 注意：v0.1.26 那条「`lane_upstream` 为 0 / `/theos` temporarily unavailable」的遗留问题
> 本轮已自然恢复 —— 本次 `/theos` 正常回 `lane_version: 8`，签到与额度刷新都通。

---

## 89. 2026-11-25 线上体检 + 模型指纹第二轮（v0.1.27，无代码变更）

**触发**：例行「扫线上 + 测指纹」。线上就是 v0.1.27（`1b24cca`），本轮**不动代码、不发版**，
只做只读体检、一次聚焦 4 模型的指纹实测，并把「指纹对不上是不是因为客户端逆向不全」
这条反复出现的疑问用可观测证据证伪/证实。

本轮所有临时密钥用完即删：`fp-check-20261125b`、`proto-check-20261125` 已删；
另有一把**并行会话**留下的 `fp-probe-20261125`（85 次请求，测完无人清理）也已删除。
收尾时 `/admin/api/keys` 只剩 `liziai-deepai`。

### 89.1 线上运行情况（`deepai2api.liziai.cloud` → nginx → 127.0.0.1:10088）

> 注意端口：容器 10087 映射到**宿主 10088**，宿主上 `curl 127.0.0.1:10087` 是空响应，
> `/healthz`、`/admin/api/*`、`/metrics` 都要打 10088（宿主侧没有容器内的 wget 兜底）。

| 项 | 实测（容器启动于 07:11 UTC，约 3 h） |
| --- | --- |
| 版本 / 镜像 | `v0.1.27` / `deepaicode2api:local-20261125-v0.1.27-1b24cca`，`healthy`，`restarts=0` |
| 车道 | `lane=8 lane_compiled=8 lane_upstream=8 lane_drift=0` |
| 模型 | 28 个，默认 `glm-5.3-flash`，`/v1/models` 200 |
| 号池 | accounts=17、ready=17、degraded=0、blocked=0、`machine_ready=true`、`quota_fresh=4` |
| 请求量 | 2xx=3153、3xx=4、4xx=99、5xx=59（本次启动以来） |
| 上游拒绝 | `transport` 261、`session_busy`(409) 35+3、`context_length_exceeded`(400) 24、`theos_exhausted`(403) 1 |
| 延迟 | 整体 p50 30 s / p95 300 s；chat p50 30 s / p95 120 s；responses、messages p95 300 s |
| 签到 | 229 次尝试、**161 次慢阻塞**（≥`checkin_block_ms`=2.5 s）、**155 次超预算**（5 s），p50=p95=5 s |
| 请求体接近截断阈值 | **690 条**告警（issue #7，阈值 196,608 B，实测有 1 MB 级 body） |
| 资源 | 内存 2.1/3.9 GB、磁盘 68% |

三条待跟进（都不是本次回归）：

1. **3 个号 Theos 余额为负**：`yvbeghihag6wvjd` −13.89、`zupxzenogljb` −15.81、`pwvjkhiq1hwz` −0.05；
   付费模型对它们会 403 `theos_exhausted`。`quota_fresh` 只有 4/17 说明额度刷新偏慢。
2. **签到仍在热路径白等**：229 次里 155 次耗满 5 s 预算、161 次越过 2.5 s 告警线。
   功能上没坏（17 个号 `theos.lane_version` 全是 8、余额在刷新），但每次都推迟首字节。
3. **`body_near_cap` 690 条**：`deepseek-v4.1-flash` 有 1,027,760 B 的请求体，
   远超 196,608 B 的静默截断阈值，仍在 200 —— 属于「透传成功但可能已被上游截断」的灰区。

### 89.2 指纹实测（lmfpd，参考库 `shared-detector-v2` / 76 模型 / `89f2a3ea…`）

参数：`-a chatcompletion`、**默认 SSE（不用 `-ns`）**、`-p 1`、`--count 3`、`--timeout 180`
（`claude-haiku-5-5` 重试用 300）。选 4 个模型是为了与 §87.8（11-24）同口径可比：
1 个最新免费模型 + 3 个对照。

| 模型 | 有效样本 | 概率状态 | 判定为 | 家族 | 与目录名 |
| --- | ---: | --- | --- | --- | --- |
| `qwen-3.8-flash` | **3/3** | reference_calibrated，p=0.361 | qwen3.8-omni-flash | **qwen** | ✅ 家族对齐 |
| `glm-5.3-flash` | **3/3** | reference_calibrated，p=0.503 | grok-4.6 | grok | ❌ 与 §87.8 的 0.503 / grok-4.6 **逐位复现** |
| `deepseek-v4.1-flash` | **3/3** | reference_calibrated，p=0.205 | mimo-v2.5-pro | mimo | ❌ 真身 `deepseek-v4.1-flash-1017` 只排第 3（p=0.112） |
| `claude-haiku-5-5` | 1/3（重试 2/3） | unavailable | deepseek-v4.1-flash（1 样本）/ openpangu-2.0-flash（2 样本） | deepseek / openpangu | ⚠️ **待复测** |

网关侧对应观测（同一批请求，含复测）：**19 次上游请求：15×200、4×499**。4 次 499 里
3 次是 `claude-haiku-5-5`（两次运行，`latency_ms` 179,931 / 179,938，正好是 lmfpd 的 180 s
首字节超时），1 次是 `deepseek-v4.1-flash` 复测的第三条（流了 37 KB 后撞 168 s）；
当时并行还有另一个会话在用 `-ns` 非流式打同一条线上，把队列压满了。四个模型全是免费档，
**Theos 消耗为 0**。

复测 `deepseek-v4.1-flash`（同参数、19 分钟后）：**3/3 有效、openpangu-2.0-pro、p=0.920**。
同一模型、同一请求形状、同一参考库，20 分钟内给出两个完全不同且都「高置信」的答案 ——
这条数据是 §89.3 的关键证据。

### 89.3 「指纹不符 = 客户端逆向不全」不成立（协议面对齐的证据）

先把可测的协议面逐项对齐。同一模型（`glm-5.3-flash`）、同一提示、非流式，
「经网关」与「桌面端形状直连上游」（`go run ./cmd/probe`，用的是 `deepai.NewClient()`，
即 `BrowserFingerprint=false` 的推理形态）对比：

| 项 | 经网关 v0.1.27 | 直连上游（桌面端形状） |
| --- | --- | --- |
| HTTP | 200 | 200 |
| `id` | `gen-e59bff1e…`（32 hex） | `gen-8fcb98b8…`（32 hex） |
| `model` 回显 | `glm-5.3-flash`（网关按目录归一） | `zai/glm-5.3-flash` |
| `system_fingerprint` | `fp_wip25d53el` | `fp_65j0eshvcd` |
| `provider` / `service_tier` | 无 | 无 |
| `reasoning_details[0].format` | `unknown` | `unknown` |
| `usage` 字段集合 | 同上游（含 `*_tokens_details`） | 同上游 |

再叠加四类长期观测：

1. **请求体形状一次通过**：本次启动以来 `gateway_fallback_total{reason="body_shape"} = 0`，
   即上游从未以「请求体不是 DeepAI Code 发的」拒绝我们的首个 body（11-10 那次是 75 次）。
2. **车道 / 模型目录 / 签到逐项对齐**：`lane_upstream=8`、17/17 账号 `theos.lane_version=8`、
   目录 28 条与桌面端逐字段一致；`/theos` 正常下发 `check_in` nonce。
3. **`X-DeepAI-Code-Proof` 未发不影响对齐**：§88.4 已实测「不发 / 64 位乱填 / 24 位乱填」
   三种情况 `/theos` 与 `/chat/completions` 全部 200；桌面端 helper 缺失时自己也会省略该头。
4. **响应形状与 §3.1 的 2026-11-13 结论一致**：`gen-<32 hex>`、无 `provider` / `service_tier`、
   `system_fingerprint` 存在、`reasoning_details.format = unknown`。这是**当前真实分支**的形状，
   不是替身特征。

结论与判据：

- 如果问题出在「逆向不全」，症状会是**全体模型一起被判到同一家族**（同一个替身后端）。
  实测是**逐模型发散**：`qwen` 对得上、`glm` 落到 grok、`deepseek` 落到 mimo 又在 19 分钟后
  落到 openpangu。形状差异解释不了这种发散。
- `deepseek-v4.1-flash` 三次（11-10 deepseek / 11-24 gemini / 11-25 mimo → openpangu）
  **一次一个答案**，说明在当前参考库下该模型**没有可重复的指纹**，检测器给的排序是噪声；
  它自己每次也都带 `evidence.insufficient=true`。
- 因此本轮判读维持 §87.8 的口径：**只有「同一轮里 3/3 有效样本给出的家族是否与目录名一致」
  可以比较**；跨轮、跨参考库的绝对值得不出身份结论。**不能**用「指纹不符」反推客户端逆向不全。
- 反过来讲，逆向面确实还有两处已知留白（`X-DeepAI-Code-Proof` 不实现、交还 ToS §4 的
  `resource-monitor --attest` 不复制），二者都有实测证明「不参与模型路由」，不作为对齐证据。

### 89.4 顺带修掉的操作坑

| 坑 | 结论 |
| --- | --- |
| `DELETE /admin/api/keys?key=<名字>` | 回 404「密钥不存在」——删除只认**明文 key 或 key_id**，不认 name |
| 宿主 `curl 127.0.0.1:10087/healthz` 空响应 | 容器端口映射在 **10088**；10087 只在容器网络内 |
| 指纹实测用 `-ns`（非流式） | 线上非流式 p95=300 s，必然撞网关/客户端超时；**SSE + `-p 1`** 才稳 |

### 89.5 待办

- `claude-haiku-5-5` 需要一轮**低负载**下的 3/3 有效样本复测（本轮两次都因首字节超时丢样本，
  11-24 那次 3/3 判成 claude 家族，不能因此否定，也不能据此确认）。
- 3 个负余额账号要么补 Theos、要么移出付费档调度。
- 690 条 `body_near_cap` 告警值得单独立项：要么在网关侧提前拒绝（显式 400），
  要么把阈值/告警改成按模型区分。

---

## 90. 2026-11-25 线上体检第二轮 + 四项整改（v0.1.28 / v0.1.29）

**触发**：例行「扫项目 + 扫线上」。本轮**动了代码**：体检数据指向三条根因，逐条整改后
发了 v0.1.28，上线后对账又发现硬额度约束几乎不生效，补 v0.1.29 再上线。

本轮临时密钥用完即删：`scan-check-20261125`、`scan-check2-20261125`（v0.1.27 体检）、
`smoke-20261125-v0129`（v0.1.29 冒烟），收尾时 `/admin/api/keys` 只剩 `liziai-deepai`。

### 90.1 项目侧扫描（本地 HEAD `a1deba5`，未提交文档 4 个）

| 项 | 实测 |
| --- | --- |
| 代码规模 | 208 个 Go 文件 / 38,735 行；`internal/server` 84 文件 15.2k、`store` 4.2k、`poolsvc` 3.3k、`deepai` 3.3k；前端 ES module 2,436 行 / 253 KB |
| 依赖 | `go.mod` 零第三方依赖（纯标准库） |
| 构建与静态检查 | `go build ./...`、`go vet ./...`、`gofmt -l` 全干净 |
| 测试 | `go test ./...` 全绿；覆盖率 69–100%（`server` 78.7%、`gateway` 94.0%、`deepai` 92.4%、`machine` 69.0% 最低） |
| CI | 最近 5 次 `ci` 全 success（含 `-race`、每包覆盖率门禁、前端 `node --check` 冒烟） |
| 仓库卫生 | 无 TODO/FIXME；`.env` 未被跟踪；根目录 `deepaicode2api.exe` / `dist/` 已被 gitignore |
| 未关闭 issue | #7（请求体 ~40k 有效上限）、#5（`/responses` 上下文）、#1（大陆访问） |

### 90.2 线上运行（容器启动 6.6 h，`deepai2api.liziai.cloud` → nginx → 127.0.0.1:10088）

| 项 | 实测（2026-11-25 12:50 UTC） |
| --- | --- |
| 版本 / 镜像 | `v0.1.27` / `local-20261125-v0.1.27-1b24cca`，`healthy`、`restarts=0`、144 MB / 512 MB、CPU 0.00% |
| 车道 / 目录 | `lane=8 lane_compiled=8 lane_upstream=8 lane_drift=0`、28 个模型、默认 `glm-5.3-flash` |
| 请求（本次启动以来） | 2xx=5,274、3xx=63、4xx=243、**5xx=63** |
| 5xx 构成 | `/v1/chat/completions` 504 **59 次全部 `ms=90000`**；`/v1/responses` 504 3 次（`ms=600009`）；`/v1/responses` 408 74 次 |
| 生命周期累计 | 266,780 次请求、失败 14,609（5.5%）；入 7.32 B、出 197.4 M、缓存命中 4.42 B（≈60%）、成本 $358.44 |
| 延迟 | 整体 p50 30 s / p95 300 s；chat p50 30 s / p95 120 s；responses p95 300 s / p99 600 s |
| 上游拒绝 | `transport` 475、`call_timeout` 75、`session_busy` 38、`context_length_exceeded` 26、`upstream_error` 4、`theos_exhausted` 1 |
| 告警（6 h） | `body_near_cap` **1,073**、出口代理探测失败 **363**（≈1/分钟）、签到阻塞推理 202；ERROR 0 条 |
| 号池 | 17/17 `ok`、`lane_version` 全 8、`quota_fresh` **2/17**、`quota_due` 1、3 个号 Theos 为负 |
| 主机 | Ubuntu 22.04、11 个容器；load 8.33 但 CPU idle 92–95%；内存 2.7/3.9 GB + swap 1.06 GB；磁盘 26/40 G（68%）；无 OOM |
| TLS | `*.liziai.cloud` 有效期至 2027-01-25，acme.sh cron 0/6/12/18:49 在跑 |

端到端：并发高时一条探测 **150 s 零字节**（`admission_wait_seconds_count=0`，说明不是排队，
卡在上游/出口）；20 分钟后同参数复测 **TTFB 2.19 s / 总 2.52 s / 200**。

### 90.3 三条根因 → v0.1.28

| 现象 | 根因 | 改动 | 默认值 |
| --- | --- | --- | --- |
| chat 504 × 59 全部 `ms=90000` | 非流式预算硬编码 90 s，与实测非流式 p95（120 s 级）不匹配 | `nonstream_budget_sec` 进设置；`stream_idle_sec` / `first_byte_sec` 一并可调 | 180 / 120 / 300 |
| `body_near_cap` 1,073 条/6 h，1 MB 级请求体仍 200 | 196 KB 静默截断阈值只有告警、没有处置开关，也改不了（只有环境变量） | `context_body_bytes` 进设置 + `body_cap_mode`（`warn`/`reject`）；`reject` 在**选号前**回 400，`bodyCapError` 让三处错误映射统一记 400 | 196608 / `warn` |
| 3 个负余额号仍在被付费请求选中 | 选号是「先选有额度的、都没有就全集兜底」，且 `needQuota` 的硬过滤从没被打开 | `PickFilteredFor(requireQuota)` / `PickAccountFor` / `PickAvoidingFor`，粘性命中也要过 `HasQuota`；`requireQuotaFor(modelID)` 按 `theosPerRequest` 判付费 | 免费模型行为不变 |

顺带修掉一处真实回归：`server.TestHoldResponseBodyBudgets` 抓到「流式首字节误用 300 s」，
改成流式走 `stream_idle_sec`、非流式才走 `first_byte_sec`。

### 90.4 负余额三个号：Theos 充不了，只能等 grant（v0.1.29 的由来）

**Theos 无法在网关侧充值**——它只由上游每日 grant 下发（`refresh_at`）或账号侧购买。
本轮能做的两件事都做了：额度复探 + 让付费调度在 grant 到账前排除它们。

| 账号 | Theos | `refresh_at`（下次 grant） | 复探结果 |
| --- | --- | --- | --- |
| `yvbeghihag6wvjd@lmoir.com` | −13.89402 | 2026-11-26T09:56:50Z | `/theos` 正常，仍为负（grant 未到） |
| `zupxzenogljb` | −15.81422 | 2026-11-26T05:38:34Z | `DeepAI models are temporarily unavailable`（重试 2 次） |
| `pwvjkhiq1hwz` | −0.04773 | 2026-11-26T02:38:37Z | 同上 |

对账发现 v0.1.28 的硬约束**几乎不生效**：`pool.HasQuota` 原本只有「余额解析为 0 且快照
≤15 分钟」一条判定，负余额落进「其余一律放行」；而线上 `quota_fresh` 只有 2/17，
绝大多数快照都是陈旧的。v0.1.29 把两类已知没额度拆开判：

- `= 0`：仍按快照新鲜度（入池瞬间上游常回 `balance=0`，当日额度随后才下发）；
- `< 0`：按 `refresh_at` —— 未到就排除，已到 / 缺失 / 解析失败则放行
  （宁可白撞一次 403，也不要因为一条坏时间戳把号永久钉在付费调度之外）。

### 90.5 上线与验证

| 项 | 结果 |
| --- | --- |
| v0.1.28 | `local-20261125-v0.1.28-dd75d1f`，`/healthz` `version=v0.1.28`、healthy |
| v0.1.29 | `local-20261125-v0.1.29-2ca9d89`，`/healthz` `version=v0.1.29`、healthy、`quota_fresh=3` |
| 设置回显 | `/admin/api/settings`：`context_body_bytes=196608`、`body_cap_mode=warn`、`nonstream_budget_sec=180`、`stream_idle_sec=120`、`first_byte_sec=300`（旧状态文件无需迁移，`Normalize` 补默认） |
| 端到端 | 免费模型 SSE：**200 / TTFB 8.15 s / 2.4 KB**（当时池里还有并发流量）；密钥已删 |
| 回滚 | `DEEPAI2API_IMAGE=deepaicode2api:local-20261125-v0.1.27-1b24cca docker compose up -d --no-build` |

**`request_timeout_sec` 保持 600**：它是流式请求的外壳上限（responses p99 正好 600 s），
实测长尾来自「非流式预算」与「帧间空闲」两处，已由新设置覆盖；再往下调会把
300–600 s 的长流式回复从 200 变成 504，收益为负。

### 90.6 待办

- `zupxzenogljb` / `pwvjkhiq1hwz` 的 `/theos` 报 `temporarily unavailable`，等 `refresh_at`
  后复探；若仍失败再按账号级异常处置（`failures` 目前 0/1，未标记 blocked）；
- `body_cap_mode=reject` 目前只在单测里验证过，线上仍是 `warn`：要切需要先在低峰
  观察 sub2api 侧 1 MB 级请求的来源，避免一刀切；
- `claude-haiku-5-5` 仍需一轮**低负载**下的 3/3 有效样本指纹复测；
- 签到仍在推理前同步执行（设计取舍，`internal/server/upstream.go` 有注释说明不能改异步），
  6 h 内 195/296 次耗满 5 s 预算，每次都在推迟首字节；
- 出口代理探测失败 363/6 h（住宅出口可用性问题），继续按出口冷却自动换；
- 日志 `body_near_cap` 占比过高，下轮可考虑按模型分档而不是单一 196 KB。

---

## 91. 2026-11-27 线上入口迁移：`deepai2api.liziai.cloud` → `deepai-gw.liziai.cloud`（无代码变更）

**根因**：旧域名已停用（nginx 直接回 `410 Gone`），所有指向 `/v1` 的客户端配置、指纹脚本、
README 之外的文档示例都会 410 而不是 401/426 —— 这类失败不产生网关日志，排查时容易误判成「密钥错」。

**迁移后的入口**（2026-11-27 实测）：

| 路径 | 结果 |
| --- | --- |
| `/healthz` | 200，`version=v0.1.29`、`lane=8`、`lane_compiled=8`、`lane_drift=0`、`models=28`、`ready=17`、`status=ok` |
| `/` | 200（内嵌控制台；`/index.html`、`/console/`、`/admin/` 均 404，控制台只在根路径） |
| `/admin/api/version` | 需 `X-Admin-Password`；返回 `current=v0.1.29` / `latest=v0.1.29` / `update_available=false` |
| `/v1/models`、`/v1/chat/completions` | 无 key → 401 `缺少 API 密钥`（正常） |
| 旧域名 `/healthz` | **410 Gone**（nginx，非网关返回） |

**改动**：

1. `docs/model-fingerprint-2026-11-10.md` 的 `lmfpd` 复测命令换成新域名（可执行示例，必须能直接跑）；
2. 本文历史条目（§89 / §90 等）里的旧域名**保持原样**——那是当时的运行记录，改了就成了伪史；
3. 无代码变更：`DEEPAI_API_ORIGIN` / 客户端 base URL 都属于部署面，网关二进制与 `dist/` 产物不变。

**验证**：新域名 TLS 证书是 `*.liziai.cloud` 通配符（有效期至 2027-01-25），Cloudflare 边缘正常
返回 `X-Request-Id`；`/admin/api/keys` 只剩 `liziai-deepai`（303,212 次请求，最后使用 2026-11-27）。

---

## 92. 2026-11-27 线上 400 归因 + 请求日志补 UA/IP + `liziwww235` 降权

### 92.1 现象

迁移后第一轮体检（09:30–09:36 UTC+8）：`/admin/api/accounts` 全绿（17/17 ready，blocked 0，
quota_fresh 17，lane 8 = 编译 8 = 上游 8），但请求日志里 4xx 占 **10.8%**，且**全部是上游 400**。

| 调用族 | 窗口 | 400 / 总 | 特征 |
| --- | --- | --- | --- |
| `/v1/responses` | 46 min | **98 / 997（9.8%）** | `prompt_len` p50 **1.63M** 字符、`messages` 100+ 占 55/98、`body_bytes` 255 KB–2.14 MB |
| `/v1/messages` | 约 90 min 前 | 3 条 | `messages` **714–723**、`prompt_len` 2.83–2.86M、`body_bytes` 2.87 MB |
| `/v1/chat/completions` | 同窗口 | 0 / 2 | 正常（`messages` 1） |

上游回的文案统一是 `This conversation exceeds the context window. Compact it to continue.`，
进程内累计 `gateway_upstream_errors_total{code="context_length_exceeded",status="400"}` = **686**。

**判定：网关没丢历史、也没砍请求**（`messages` 与 `body_bytes` 都是客户端真实发出的规模），
是调用方把整段历史无压缩地重发，撞了模型窗口。两次是**同一段超长对话被反复重试**
（98 条 400 只有 54 个不同 `prompt_len`），代价是每次白等上游 2–44 s。

### 92.2 为什么当时说不清「是哪个客户端」

`store.LogEntry` 只有体积、消息数、账号、模型——能证明「发了多大」，证不了「谁发的」。
线上只有一把密钥（`liziai-deepai`），下游是一组共用它的客户端，网关侧无从区分。

**改动**：`LogEntry` 新增两个脱敏字段，9 处 `newReqLog` 调用统一挂 `.withClient(r)`：

| 字段 | 取值 | 说明 |
| --- | --- | --- |
| `ua` | `User-Agent` 前 120 字符 | SDK 常写死成一个值，是线索不是身份 |
| `ip` | `CF-Connecting-IP` → `X-Forwarded-For` 首段 → `RemoteAddr` | 线上是 CF → nginx → 网关，`RemoteAddr` 只会是回环/边缘地址 |

单测 `TestReqLogRecordsClientIdentity` 锁死三条回退路径、120 字符截断与 `nil` 请求。
**这份代码需要重新部署才生效**（`sh scripts/deploy.sh <tag>`），部署后同类问题可直接在
`/admin/api/logs` 里按 `ua` / `ip` 归因。

### 92.3 429：`liziwww235` 降权（可回滚）

429 在上游是**账号级**限流。46 分钟窗口内 5 条 429 的分布：`a0c967a1…`（= `liziwww235`）3 条、
`b1a1df81…` 1 条、`ccab41c2…` 1 条；`liziwww235` 的 `last_err` 也正是这条文案，且它是池里唯一
`last_verified=0` / `email` 为空 / `source=oauth` / `registered_at=0` 的账号（`failures=1`）。

住宅出口无法在网关侧新开（`PROXY_POOL` 是部署面输入），所以选**降权**而不是换出口：

```bash
# 已执行：迁到不透量给 default 分组密钥的独立分组（立即生效）
POST /admin/api/accounts {"action":"set-group","id":"a0c967a16714141b","group":"throttled"}
# 回滚（观察 30–60 min 不再出现 429 后建议执行）
POST /admin/api/accounts {"action":"set-group","id":"a0c967a16714141b","group":"default"}
```

结果：`{default: 16, throttled: 1}`；`/healthz` 仍是 `accounts=17, ready=17`（就绪数是分组无关的）。
**代价**：这把 207 Theos 暂时不参与调度——是一次可观测性实验，不是长期结论。

### 92.4 负余额三号：等 `refresh_at` 再复探

| 账号 | 余额 | `refresh_at`（UTC） | 处置 |
| --- | ---: | --- | --- |
| `zupxzenogljb` | −15.81422 | 2026-11-27T05:39:07Z | 到时复探，仍负则按账号级异常处置 |
| `pwvjkhiq1hwz` | −0.04773 | 2026-11-27T02:40:14Z | 同上 |
| `yvbeghihag6wvjd@lmoir.com` | −13.89402 | 2026-11-27T09:57:13Z | 同上 |

三个号的 `granted` 都是 `0.00000`（grant 未到）。`upstream.go` 在额度陈旧时会在请求路径上
自动复探（`quota_due=true` 时触发），所以 **不必手工干预**；到时直接看 `/admin/api/accounts`
的 `theos.balance` 即可。复探单号用 `POST /admin/api/checkin {"id":"<id>"}`（注意：该接口是
**真实签到**，会消耗 nonce，不是只读）。

### 92.5 未做的

- `body_cap_mode` 仍是 `warn`：`reject` 会在选号前直接回 400，省掉上游那 2–44 s，但同一窗口里
  也有 2.3 MB 的请求成功过（200），切之前需要先确认调用方来源（正好等 UA/IP 字段部署后的数据）。
- 签到预算（615 次里 63 次超时、p95 = 5 s）保持 §90.6 的待办口径，本轮未动。

---

## 93. 2026-11-27 缓存命中率口径拆账 + 5xx 同号重试

### 93.1 起因：60–70% 是不是统计错了

先做四项交叉校验，结论是**算术没错**：

| 校验 | 结果 |
| --- | --- |
| 分子分母同窗口 | 控制台主值 `day_cached_tokens / day_tokens_in`、tooltip `total_cached_tokens / total_tokens_in`，各自同窗口（`store.go` 里特意写了「累计值不能与当日值混算」的警告） |
| 用原始日志复算 | 最近 500 条中 456 条 200：`4,137,577 / 6,014,193` = **68.8%**，与接口今日 66.7%、累计 60.7% 同量级 |
| 数据完整性 | `cached > tokens_in` 的条目 0 条；200 但 `tokens_in=0` 的 0 条；`tokens_in/prompt_len` p50 = 0.30（≈3.3 字符/token） |
| 结构合理性 | 单请求命中率 p50 58% / p90 98%；按历史长度单调递增（msgs=1 → 20%，2–9 → 62%，10–49 → 70%，100+ → 80%）——统计错位不会长出这种结构 |

真实构成（同窗口）：

| 分组 | 请求 | prompt token | 命中率 |
| --- | ---: | ---: | ---: |
| `cached_tokens = 0` | 149（33%） | 1,244,732（20.7%） | 0% |
| `cached_tokens > 0` | 307 | 4,769,461 | **86.8%** |

即：**33% 的冷请求吃掉了 66% 的未命中 token**。冷请求按「距上一条同模型、同规模请求」
再分：< 60 s 占冷 token 57%（多为新会话首轮）、60–300 s 占 9.5%、**> 300 s 占 16.5%
（上游前缀缓存 TTL 过期）**、窗口内无前序占 16.9%。

同一段长会话内部是稳定的：`msgs` 130→143 那一串命中率 98.9 / 99.8 / 92.3 / 82.3 /
99.4 / 100%，只有两处断崖——`msgs=141, gap=376 s`（TTL 过期）与 `msgs=208, gap=3 s`
（3 秒就冷，只可能是换号或前缀被改）。**这正是本节两处改动的依据。**

### 93.2 改动 A：命中率拆成「读 / 写 / 全新」

| 层 | 改动 |
| --- | --- |
| `gateway.ParseUsage` | 新增 `CacheWriteTokens`，收 `cache_creation_input_tokens`（上游实发形态）与 `*_tokens_details.cache_creation_tokens` 两种写法；与 `CachedTokens` 严格分开 |
| `store.LogEntry` | 新增 `cache_write_tokens` |
| `store.Stats` | 新增 `total_cache_write_tokens` / `day_cache_write_tokens`，`Record` 累加、跨天清零 |
| `server.reqlog` | 落盘时带上该字段 |
| 控制台 KPI | 命中率 tooltip 改为「读 / 写 / 全新 / 入（累计命中 …）」 |

这样「新会话首轮把前缀写进缓存」与「本该命中却 miss」在界面上不再同形——前者是命中率的
结构性上限，后者才是要修的问题。

单测：`TestParseUsageCarriesCacheWriteTokens`（三种形态 + 读写不混）、
`TestRecordAccumulatesCacheWriteTokens`、`TestDayCacheWriteTokensResetOnNewDay`。

### 93.3 改动 B：5xx 改「同号重试一次」，保住前缀缓存

`openModel` 的重试循环此前「重试必须换号」。换成号等于整段上下文重新冷启动
（日志里换号那两次命中率直接 0%），而 502/503/504 多半只是边缘与上游之间抖一下。

- 新增 `sameAccountRetryable`：500/502/503/504 → 同号重试；429 / 403 / 400 / 426 不适用
  （账号级或请求级语义，同号重试只会白等）；
- 循环里加 `retrySame`：同号重试时 `avoid = nil`，允许该号在出口刚被冷却时改走全局出口
  （「换路不换号」）；每个号本轮最多一次；
- 传输层失败（连不上 / 断流）同样先同号重试一次。

单测：`TestSameAccountRetryable`（边界表）+ `TestOpenModelRetries5xxOnSameAccount`
（端到端断言两次上行请求的 `Api-Key` 相同——换号会直接让这条断言失败）。

### 93.4 改动 C：客户端前缀契约写进 README

前缀缓存按**字节前缀**匹配，网关无法替客户端稳定它。README §七 补了四条：
system/tools 字节级稳定、同一会话固定 `prompt_cache_key`/`X-Session-Affinity`、
空闲超过 TTL 后不要指望命中、重试留同号（对应 §93.3）。

### 93.5 未做

- `body_cap_mode` 仍是 `warn`（见 §92.5）；
- 日志里还没有 `affinity` / 前缀指纹：要量化「客户端是否真的固定了前缀」，需要
  再加两个短哈希字段（`affinity` + `prefix`），留到下一轮。

---

## 94. 2026-11-27 前缀抖动可量化：`affinity` / `prefix` 指纹进日志

### 94.1 要回答的问题

§93 把命中率拆成「读 / 写 / 全新」之后，还剩一个内部矛盾说不清：同样是 0 命中，

- **A. 客户端每轮重写 system / tools**（前缀变了，缓存必然 miss）——客户端侧要修；
- **B. 前缀稳定但换了账号 / 空闲超过上游 TTL**（缓存分区换了）——网关/网络侧。

日志里只有体积、消息数与（§92 加的）UA/IP，两种原因长得一模一样。

### 94.2 改动

| 层 | 内容 |
| --- | --- |
| `server/request.go` | 抽出 `prefixDigest(payload)`（模型 + system/developer 消息 + tools，**不含对话正文**）；`sessionAffinity` 与新增的 `prefixFingerprint` 共用这一份摘要，保证「派生的 `aa1-…`」与「日志里的 `pf-…`」指向同一段前缀 |
| `store.LogEntry` | 新增 `affinity`（会话粘性标识，落盘截 64 字符）与 `prefix`（`pf-` + 8 字节 hex） |
| `server/reqlog.go` | `withHistory(payload)` 顺手算前缀指纹；新增 `withAffinity(a)`；chat / responses / messages 三处落盘时挂上 |
| 控制台日志页 | 新增「会话」列：`aa1-3f2a… / pf-9c11…`，完整值与判读说明放 title |
| `scripts/prefix-churn.mjs` | 拉 `/admin/api/logs` 按 affinity 分组，输出 churn（1 − 最常见前缀占比）、每个会话的前缀数、稳定组 vs 抖动组的命中率 |

判读规则（脚本末尾也会打印）：

```
同一 affinity 下多个 prefix   → A：客户端在改 system / tools
同一 affinity 下只有一个 prefix → B：换号或上游 TTL 过期
```

### 94.3 单测

- `TestPrefixFingerprintTracksCacheablePrefix`：只增长用户轮次 → 指纹不变；tools / system 改一字节 → 指纹变；`affinity[4:] == prefix[3:]`（同一摘要）；裸请求两个都为空；
- `TestReqLogRecordsSessionFingerprints`：两条指纹进日志、affinity 落盘截断到 64 字符。

### 94.4 生效与用法

```bash
# 部署包含本改动的版本后：
ADMIN_PW=... node scripts/prefix-churn.mjs
GW=https://deepai-gw.liziai.cloud ADMIN_PW=... LIMIT=500 node scripts/prefix-churn.mjs
```

- 老日志没有这两个字段：脚本会直接提示「部署后再来」，不会给出误导性的 0%；
- 字段都是短哈希/截断串，不含正文；`affinity` 本身就是要发给上游的 `X-Session-Affinity`，不是新泄露面。

---

## 95. 2026-11-27 上线 `v0.1.29-1693b20`（二进制热替换）+ 上线前后的 429 风暴

### 95.1 部署方式

网关跑在 Altare workstation `dev`（不是 hermes 的 docker），镜像是私有 GHCR
`ghcr.io/2951461586/deepaicode2api:v0.1.29-altare`。本机没有 docker，也没有 GHCR 推送凭据，
所以这次不重建镜像，改为**只读 bind mount 覆盖镜像入口**：

| 步骤 | 内容 |
| --- | --- |
| 构建 | `CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "-s -w -X main.version=v0.1.29-1693b20"` |
| 传输 | `altr ws cp`（走 guest agent，base64，3.5 MB gzip 约 3 分钟；直接传 8.4 MB 会超时） |
| 校验 | 两侧 `sha256` 一致（`cb0ac5ee…9293fa`），`-version` 输出 `v0.1.29-1693b20` |
| 落地 | `/opt/deepaicode2api/bin/deepaicode2api-1693b20`（755），`start.sh` 备份为 `start.sh.bak-20261127` |
| 起容器 | `start.sh` 在 `-x "$BIN"` 时追加 `-v "$BIN":/usr/local/bin/deepaicode2api:ro`，镜像与数据卷都不动 |

回滚：把 `BIN` 指回上一版（或让文件不可执行），`sh /opt/deepaicode2api/start.sh` —— 立即回到镜像自带二进制。

### 95.2 上线校验

| 项 | 结果 |
| --- | --- |
| `/healthz` | `version=v0.1.29-1693b20`、`lane=8`/`lane_compiled=8`/`lane_drift=0`、`accounts=17`、`ready=17`、`models=28`、`machine_ready=true` |
| `lane_upstream` | 重启后先是 `0`（探针未跑），首个请求后回到 `8` |
| 冒烟 | 临时 key 打 `/v1/chat/completions`：200 / TTFB 2.63 s / `tokens_in=294`、`cached_tokens=132`；key 用后即删 |
| 新字段 | 同一条日志里 `ua=pi-deploy-smoke/1.0`、`ip=104.194.8.36`（CF-Connecting-IP）、`affinity=aa1-bff7f721f2c1d534`、`prefix=pf-bff7f721f2c1d534`（同源摘要，符合预期） |
| 进程指标 | 重启后 `2xx=22`、`4xx=0`、`5xx=0` |

### 95.3 上线前的一段 429 风暴与下游断流（与本次部署无关）

时间线（UTC）：

| 时刻 | 事件 |
| --- | --- |
| 02:08–02:19 | 零散 429（每分钟 1–3 条） |
| 02:20–02:32 | **429 风暴**：153 条，峰值 02:30 的 27 条/分钟；跨 6+ 个账号（`zhuhaoyi181` 31、`xiaoming` 26、`liziyy235` 16…），模型集中在 `deepseek-v4.1-flash` 86 / `glm-5.3-flash` 33 |
| 02:38 | 网关最后一条真实请求（499，`minimax-m3.1-flash`，128 s 客户端取消） |
| 02:44 | **本次部署**（此后只有一条冒烟请求） |

也就是说**下游在部署前 6 分钟就已经没有流量**，部署不是断流的原因。旁证：

- sub2api 侧 40 分钟状态分布：`200×1193`、`401×885`（扫描器噪声）、`429×392`（其中绝大多数是
  `ingress_reject_reason=invalid_auth_rate_limited`，**sub2api 自己的无效密钥限流**，不是上游 429）、
  `503×173`（池空）、`502×107`、`500×80`；
- sub2api 的两个账号（3696 / 3697）在 DB 里 `status=active`、`schedulable=t`、
  `temp_unschedulable_until` 为空、`rate_limit_reset_at` 已过期，`last_used_at` 分别停在
  02:38:14 / 02:36:27 —— 不是被冷却掐掉的，是**没有请求可发**；
- 两个账号的 `base_url` 仍指向 `https://deepai-gw.liziai.cloud(/v1)`，`header_override_enabled=true`。

**对 429 风暴的推断**：上游的 429 文案是账号级「Requests are temporarily limited」，但同时命中
6+ 个账号，更像是**出口 IP 集中**触发上游全局限流——重启前的日志里确有多条
`出口代理探测失败（沿用全局出口）… 出口被上游拦截（HTTP 403，HTML 错误页）`，那些账号会退回
VM 的全局出口，等于把十几个账号的流量压到同一个 IP 上。重启后 `proxy_cooling=0`（17 个出口全部探测通过），
这一条件暂时不存在。

### 95.4 清理（2026-11-27）

迁移后 hermes 上留下的旧部署痕迹已删：

| 对象 | 大小 | 判定依据 |
| --- | --- | --- |
| `/etc/deepaicode2api.env` | 371 B | 旧本机部署的机密文件；`cron` / `systemd` / 容器 / 进程四处都查过，无引用；容器早已迁到 workstation，留着只是把口令摊在第二台机器上 |
| `/root/DeepAICode2API` | 33 MB | 11-25 的旧代码副本（`.git` 停在 `2ca9d89`），工作区干净、仓库在 GitHub 与 workstation 都有，唯一独有内容是里面的 `.env`（同样 shred） |

**保留**：`/root/deepaicode2api-altare-runbook.md`、`/root/altare-migration-runbook.md`（迁移手册，
排查时要回头看）；workstation 上的 `start.sh.bak-20261127`（419 B，是目前唯一的秒级回滚路径，
等新版本观察过一段时间再删）。

workstation `dev` 侧的清理**已执行**（2026-11-27 03:5x UTC，先 `altr ws restart dev` 把挂掉的
guest agent 恢复：VM 重启 ~30 s，容器 `--restart unless-stopped` 自动拉起，bind mount 仍在，
`/healthz` 复验 `v0.1.29-1693b20`）：

| 对象 | 结果 |
| --- | --- |
| 构建缓存 | `docker builder prune -a -f` → **621.2 MB** |
| 构建基础镜像 | `golang:1.24-alpine`（395 MB）+ `alpine:3.20`（12.2 MB）→ 删；运行中的 `v0.1.29-altare`（27.4 MB）与它的层不受影响 |
| 旧网关镜像 | **本来就没有**（`docker images` 只有运行中的一个），无需处理 |
| 旧二进制 | 只有 `deepaicode2api-1693b20` 一个，无残留 |
| 陈旧数据 | `data-logs.jsonl.1`（8.4 MB 轮转日志）+ `.deepaicode2api-data-sticky.json.tmp-*`（重启时的半成品）→ 删；数据目录 10.5 MB → **2.3 MB** |

合计回收约 **1.03 GB**。保留：运行中的镜像、当前二进制、`start.sh.bak-20261127`（回滚）、
`proxy_pool.txt`、四个数据文件。

**未完成**：`altr ws set dev --auto-stop off` 一直返回 `The workstation host didn't answer`
（`ws exec` 间歇可用、`ws set` 稳定失败，`--json` 也只给出同一句），所以 `Auto-stop: 2h idle`
仍是平台默认值。影响：连续 2 小时无请求会被自动关机。当前流量足够不会触发，等 agent 稳定后
重试或从 console 的浏览器桌面改。

命令备查（恢复后可直接跑）：

```bash
# 只保留在用的镜像
KEEP=ghcr.io/2951461586/deepaicode2api:v0.1.29-altare
docker images --format '{{.Repository}}:{{.Tag}}' | grep '^ghcr.io/2951461586/deepaicode2api:'   | grep -v "^$KEEP$" | xargs -r docker rmi
docker image prune -f
# 只保留当前二进制
ls -1 /opt/deepaicode2api/bin/deepaicode2api-* | grep -v '1693b20$' | xargs -r rm -f
rm -f /opt/deepaicode2api/bin/*.gz
# 数据备份先看再删（不要动 data.json / -sticky.json / -stats.json / -logs.jsonl）
ls -la /opt/deepaicode2api/data
```

### 95.5 待办

- 观察 02:50 UTC 之后流量是否自然恢复（用户侧退避通常 10–30 分钟）；
- 若 429 风暴复现，先看 `proxy_cooling` 与出口探测失败数：全局出口集中是已知放大器；
- sub2api 侧对 5xx/429 的退避与重试策略值得单独调（本轮 502×107 + 500×80 放大了用户感知）。

---

## 96. 2026-11-27 全量 28 模型指纹一轮（fpd）

用 `npx lmfpd@latest` 对线上 28 个模型各做一轮数字指纹，明细见
**`docs/model-fingerprint-2026-11-27.md`**（含完整表格、方法、局限）。

三条技术结论值得留档：

1. **必须流式 + `-e low`**：这些模型都是推理模型，上游把 completion 硬顶在
   **8192**（`completion=8192 / reasoning=8192 / content=""`、`finish=length`）；
   不降推理强度时 323 个整数的挑战必然被推理吃光预算，样本直接失败。
   非流式还会撞 Cloudflare 的 100 s 上限（实测 `HTTP 524`）。
2. **`gpt-6-astra` 只支持 `/responses`**：走 chat 会回
   `400 Model 'gpt-6-astra' is not available on this endpoint`。
3. **6 个 flash 族模型指向同一个候选 `gemini-4-carbon`**（置信度 0.47–0.86），
   只有 3 个家族对齐（qwen / mimo / claude-opus）。但参考库版本已换
   （`reference_sha256 7d775ce2…`），与 11-10 / 11-25 两轮**不可直接对比**——
   那两轮 `glm-5.3-flash` 指向 grok 家族，本轮指向 `gemini-4-carbon`。

花费：付费 16 个模型用 `--count 1`（每模型 1 次请求），合计 **185 Theos**；
若用 `--count 3` 是 555，因此付费组只有排名、没有置信度。

采集用临时 key，结束后已删（`/admin/api/keys` 只剩 `liziai-deepai`）；采集期间
线上无 5xx，`fallback_total=0`。

---

## 97. 2026-11-27 协议面复核：指纹漂移不是缺头造成的

被质疑的因果链是「网关缺请求头 / 缺协议模块 → 上游换路由 → 模型指纹漂移」。逐项核对后
**不成立**，证据分两层。

### 97.1 协议面逐项对照（桌面端 0.3.52 vs 网关）

| 元素 | 结果 |
| --- | --- |
| `Api-Key` / `Content-Type` / `X-DeepAI-Code-Theos` / `-Machine` / `-Instance` / `X-Session-Affinity` | 全在（`commonHeaders` + `OpenModel`） |
| `user-agent: node` / `accept: */*` / `accept-language: *` / `sec-fetch-mode: cors` | 全在（`desktopInferenceHeaders`） |
| `/theos` 的 `X-DeepAI-Code-Check-In` + `X-DeepAI-Code-Beacon: v=0.3.52;up=…;pi=…` | 全在；`focus` 省略（桌面端也只在焦点已知时发） |
| **`X-DeepAI-Code-Proof`（0.3.52 车道 8 新增）** | **缺**：值是随包原生 helper 用构建密钥算的，网关无法实现；2026-11-25 实测上游不校验 |
| 请求体 | 默认透传调用方字段（`preset_desktop_shape=false`），仅在 400「不是桌面端形状」后收敛并记忆；`gateway_fallback_total=0` 说明从未触发 |
| 协议模块 | chat 合成 / responses 直通 / messages 合成 / `previous_response_id` 重放 / `prompt_cache_key` 透传均在位 |

即：**唯一的真实缺口是 Proof 头**，而它不是当前漂移的原因（缺它照样 200，且真模型响应
带 reasoning / 计费字段）。

### 97.2 漂移的主因是采样噪声（固定挑战实测）

同一模型 `glm-5.3-flash`、**同一组挑战**、同一协议、连续两次：

| 轮次 | top-1 | top1−top2 |
| --- | --- | ---: |
| run 1 | `gpt-6-astra`（1.62） | **0.02** |
| run 2 | `grok-4.6`（1.76） | 0.11 |

输入完全相同，冠军不同。再看全量 28 个模型的 top1−top2：**19 个 < 0.25，最大 0.88**。
换协议（cc ↔ responses）的差异也落在这个尺度内（`glm`/`qwen` 两路径同候选，
`deepseek-v4.1-flash` 从 `gemini-4-carbon` 变 `gemini-3.5-flash-lite`）。

→ §96 里「6 个 flash 模型同指 `gemini-4-carbon`」**降级为不可判定**：单轮 3 样本
撑不起「上游同源」的结论。`docs/model-fingerprint-2026-11-27.md` 已补复核章节与
下轮做法（固定挑战 + `-n 5`，`top1−top2 < 0.3` 视为无法判定）。

### 97.3 待办

- Proof 头保持不实现，等上游开始强制（届时会出现 400/403 而非静默换路由）；
- 若要正面验证「替身分支」，需在**真实桌面客户端**上抓同挑战请求做对照，或在固定挑战下
  对 `preset_desktop_shape` 做 A/B（它会补 system 消息，必须固定挑战才可比）；
- 签名/头清单已逐项落在本节，下次上游改协议只需 diff 这张表。

---

## 98. 2026-11-27 补齐 `X-DeepAI-Code-Proof`（v0.1.30）

§97 的结论是「协议面唯一缺口是 Proof 头」。这一版把它补上，但**做成显式三档**，
而不是偷偷伪造：网关没有客户端那种「构建密钥」，能不能产出自证取决于运维有没有把
helper 放到这台机器上。

### 98.1 三档模式（`DEEPAI_PROOF`）

| 模式 | 行为 | 用途 |
| --- | --- | --- |
| `off`（默认） | 不发该头 | 与 v0.1.29 及更早一致；上游当前不校验，缺头照样 200 |
| `random` | 发 32 位随机 hex（合法区间 24–64） | **只对齐协议面**，不声称任何自证；用于观察「带这个头」是否改变上游路由 |
| `helper` | 调 `DEEPAI_PROOF_HELPER` 指向的 `resource-monitor --attest`，产出**真实**自证 | 运维已把桌面端 helper 放到机器上时 |

helper 模式必须同时给 `DEEPAI_PROOF_HELPER`（绝对路径），否则归一成 `off` 并告警；
未知模式同样归一成 `off` —— 宁可少发一个头，也不要发出一个看不懂的值。

### 98.2 输入形态（与 `laneProof.ts` 对齐）

| 请求 | 输入 JSON | 说明 |
| --- | --- | --- |
| `/theos` | `{"n": 即将回传的 nonce \| "", "a": sha256(apiKey), "m": 机器指纹, "i": 实例号, "v": 0.3.52}` | 签到证明 |
| 模型调用 | `{"n": 上次**成功回传**的 nonce, "b": sha256(请求体)}` | 请求标签；用 ackedNonce，不是「刚拿到的 nonce」 |

helper 的调用方式实测确认：`resource-monitor --attest`，**JSON 走 stdin**，stdout 回
24–64 位 hex（本地 0.3.52 的 helper 实测输出 24 位）。

### 98.3 失败语义（与桌面端一致）

helper 缺失 / 5s 超时 / 输出不是 hex / 长度越界 → **省略该头**，推理请求照常发出。
路径只接受绝对路径并 `filepath.Clean`；exec 的参数固定（只多一个 `--attest`），
stdin 喂 JSON，不存在注入面。静态扫描对该 exec 的告警按「已审阅」处置（配置来源是运维，
不是请求数据）。

### 98.4 可观测

- `/healthz` 新增 `proof` 字段（`off|random|helper`），部署后一眼看出线上跑哪一档；
- 启动日志追加 `（proof=…）`。

### 98.5 本轮线上取值与回滚

线上先设 `DEEPAI_PROOF=random`（协议面对齐），因为：
1. 上游 2026-11-25 实测不校验，发随机 hex 与不发都 200；
2. 带这个头才有机会观察到「上游是否拿它做路由」，是 §97 里那个未决问题的下一步实验；
3. 真正的 `helper` 需要把客户端二进制搬到服务器上，属于另一件事（且要在确认上游开始校验后
   才有必要）。

回滚：把 `/opt/deepaicode2api/app.env` 里的 `DEEPAI_PROOF` 改回 `off` 并重启容器。

### 98.6 单测

`TestProofConfigFromEnvModes`（未知/缺路径归一）、`TestProofOffOmitsHeader`、
`TestProofRandomSendsHex`（合法 hex + 两次不同）、`TestProofHelperUsesOutput`
（采用 helper 输出、输入是 `{n,b}` 且 b=sha256(body)）、`TestProofHelperFailuresOmitHeader`
（缺文件 / 输出非 hex / 过短 / 相对路径 → 省略）、`TestProofOnCheckIn`（`/theos` 的
签到证明含 `n/a/m/i/v` 且 a=sha256(apiKey)）。helper 相关用例在 Windows 跳过
（脚本不可 exec），由 Linux CI 覆盖。
