# 工程经验沉淀（2026-11 十批压测）

本文把 2026-11 的十批 50 号压测（注册 → 登录 → 取 Key → 验活 → 入池 → 推理）
**从执行日志转成可复用规则**。README 不再保留逐批流水账，只留结论与
验收基线；原始数字见 `docs/performance.md`，改动过程见 `docs/changes.md`。

> **2026-11-10 注**：邮箱注册链路（`internal/register`、`cmd/register`、
> `/admin/api/register` 与控制台「批量注册」页）已整体移除。本文涉及
> `internal/register/*` 的「代码落点」属于历史记录，只用于解释当时的决策；
> 现役代理池在 `internal/proxy`，号池写入口在 `internal/server/enroll.go`。
>
> 同日清理：所有 HAR 抓包（`accounts/*.har` 与仓库根目录的 `deepai.org*.har`）
> 已删除，其中未入库的凭据已折进加密状态文件（停用状态，见 `changes.md` §35）。
> 本文提到的 `deepai.org-2.har` 等文件名仅作证据索引，文件本身已不在仓库内。

每条经验的格式是 **现象 → 根因 → 规则 → 代码落点**，末尾给出失效边界。
改动注册 / 调度 / 探测相关代码前，请先过一遍 §5 的检查清单。

---

## 1. 关键路径怎么切

### 1.1 只有「决定账号能不能用」的阶段才配留在关键路径

- **现象**：第三~五批单号端到端 27.0s → 31.4s → 67.2s，波动全部来自
  `verify`（等验证邮件 / 点验证链接），网关侧 `register` / `login` /
  `session` / `validate` 合计稳定在 11~12s。
- **根因**：`verify` 是**外部服务的不确定等待**，且不在可用性判定上：
  `/theos` 只看 `Api-Key`，邮箱没验证的号照样能推理、照样有 200 额度。
- **规则**：关键路径 = 「拿不到就白干」的阶段（`register` / `login` /
  `session` / `validate` / `enroll`）。其余一切等待（邮件、验证点击、配额复探）
  都改成后台 + 回填。
- **代码落点**：`internal/register/pipeline.go`（`validate` 在 join 之前，
  `enroll` 之后 `go p.verifyBackfill(...)`）、`internal/register/pipeline_verify.go`。
- **收益**：单号 E2E 中位 52.5s → **12.3s**（第六批），之后长期稳定在 8.9~12.3s。

### 1.2 后台任务不能挂在调用方 ctx 或进程生命周期上

- **现象**：三个独立事故同一根因 ——
  1. 控制台「刷新全部配额」关掉页面就走 `r.Context()` 取消，653 个号有一半没探完；
  2. CLI 批次一结束进程退出，后台验证 goroutine 被一起带走，交付物里
     `email_verified` 全是假值；
  3. 后台验证在批次 ctx 取消后才跑完，写回时与 `finish()` 竞争。
- **根因**：后台任务借用了「请求作用域」的 ctx。
- **规则**：后台任务用 `context.Background()` + 自己的超时；写回只操作记录副本；
  进程退出前必须显式等待（有上限），或把任务交给常驻进程（服务端）。
- **代码落点**：`verifyBackfill` 的独立 `WithTimeout`；`cloneRecord`；
  CLI `-verify-grace`（默认 60s）→ `Pipeline.WaitVerify`；服务端配额任务
  不绑 `r.Context()`。

### 1.3 关键路径上不允许「二次探测」

- **现象**：入池阶段还要再打一次 `/theos`（2.0s），失败率 8%，
  而且入池瞬间上游常返回 `balance=0`，探了也白探。
- **根因**：把「验活」和「写号池」耦合在一个阶段里。
- **规则**：同一个事实只在一个阶段确认一次。入池只写状态，额度交给
  后台复探（见 §3.3）；重复探测只会放大上游抖动和失败面。
- **代码落点**：`internal/server/register.go` `enrollRegistration`
  （唯一写入口，不探活）、`allowanceFromEnroll`（入池携带快照即可）。

---

## 2. 上游调用纪律

### 2.1 每个上游 POST 都要有总预算，退避必须可被 ctx 打断

- **现象**：第六批 16/50 卡在 `POST /daily-time-sync/registration/` 的
  **504 Gateway Timeout**，单号白等 188~219s，整批墙钟 430s。
- **根因**：默认 120s 超时 + 退避用 `time.Sleep`（超时后仍要睡完 700ms+1400ms）。
- **规则**：给上游 POST 定**比全局超时更小的总预算**（注册 45s），
  退避一律走可取消的 `sleepCtx`，让批次能快速失败、不拖长尾。
- **代码落点**：`internal/register/web.go` `registerPostBudget`、`sleepCtx`。

### 2.2 504 不等于没成功 —— 用登录兜底

- **现象**：504 的账号里相当一部分其实**已经建成**，只是响应没回来。
- **规则**：注册失败后不要直接判死，先试登录：登录成功就继续走取 Key，
  等于拿到幂等语义；上游返回「已注册」同样走登录分支。
- **代码落点**：`internal/register/` 的 `isAlreadyRegistered`（`pipeline_batch.go`）
  + `pipeline.go` 里转登录的分支。

### 2.3 重端点必须限流：先测端点成本，再定并发

- **现象**：第八批邮箱验证 **0/50**：17 次超时、16 次 `unexpected EOF`、
  2 次 `502 Incomplete response`。同一时段浏览器手动验证 `201 / 4.8s` 正常。
- **根因**：`GET /verify_email?key=…` 的 HAR 画像（`deepai.org-2.har`）是
  `nginx + Phusion Passenger`，**wait 13.2s**、合计 13.9s。10 个后台
  goroutine 同时点，直接把上游 worker 池打满 —— **不是端点坏了，是自己把上游打限流了**。
- **规则**：动手并发之前先量单次服务端耗时；凡是「秒级 CPU/worker 占用」的
  端点，全局并发闸门 + 长退避，绝不放关键路径，绝不 per-account 并发。
- **代码落点**：`internal/register/reverify.go` `verifyClickSlots`（全局 2）、
  `verifyRetryDelays`（2s / 10s / 60s，4xx 不重试）、`verifyEmailThrottled`。
- **收益**：验证从 0/50 → 批次内 49/50，`-reverify` 补 1，合计 50/50
  （第一轮 8m04s，重试轮 <1m）。

### 2.4 重试只能有一层

- **现象**：重端点失败时，内外两层各重试 3 次 → 一次业务动作打 9 次上游。
- **规则**：底层函数只做**单次尝试**，重试策略集中在唯一的外层；
  重试序列按端点成本分级（快端点 700ms 级，13s 级端点 2s/10s/60s）。
- **代码落点**：`VerifyEmail` 单次 + 外层 `verifyEmailThrottled` 统一重试。

### 2.5 按错误类型分流，而不是无脑重试

- **规则**：
  - 可重试：`5xx`、`timeout`、`unexpected EOF`、`TLS handshake`、出口错误；
  - 不重试：`4xx`、`social_login_required`、`machine_required`、
    `lane_version_unsupported` —— 这些是策略/协议判定，重试只是浪费时间；
  - 需要等待：`409 session_busy` + `Retry-After`，换号或按提示等待。
- **代码落点**：`internal/deepai` 的错误语义判定 + `internal/retry`、
  `IsTransient` / `IsSocialLoginRequired`。

### 2.6 出口故障 ≠ 账号坏

- **现象**：`No exit node` 挂在单条出口上，17 个账号全部 0 额度；
  换出口后全部恢复 `balance=200`。
- **规则**：先查出口，再删号。代理绑定是可改的元数据
  （`assign-proxy`，默认只补空值），不需要重新注册。
- **代码落点**：`internal/register/proxy.go`、`/admin/api/accounts`
  的 `assign-proxy`。

### 2.7 请求头也是协议的一部分：UA 决定上游给你哪个模型

- **现象**：同一把 Key、同一台机器、同一份 body，经网关测出的模型指纹是
  `muse-wonder`（Meta 家族），客户端直测是 `glm-5.3-flash`。`Accept` /
  `Accept-Language` / `sec-fetch-mode` 全试过，只换 `User-Agent` 就能复现。
- **根因**：上游按 `User-Agent` 分流到不同后端 —— `node`（桌面端 Node fetch）
  走真实模型；`Go-http-client/1.1` 或 Chrome UA 被换到另一条后端，而响应里的
  `model` 仍写 `z-ai/glm-5.3-flash`，所以只校验 `model` 永远发现不了。
- **规则**：**推理请求的每个头都要能对上桌面端**。注册链路（网页 UA +
  `sec-ch-ua`）与推理链路（Node fetch 默认头）必须分成两套。
- **判据（2026-11-09 更新）**：上游现在把请求转给 OpenRouter 一类路由，
  真分支的 `provider` 是**真实后端名**（`InferenceNet` / `Wafer` / `Reka` /
  `BaseTen` / `CoreWeave` / `Makora` / `Modal` / `Morph` / `Inceptron` …），
  所以旧判据「出现 `provider` 且没有 `system_fingerprint`」会把真分支全部误判
  成替身。现在只看 **`provider` 是否等于 `"DeepAI"`**：等于 = 替身，不等于 = 真。
  - 交叉验证：`service_tier`（真 `null` / 替身 `"default"`）、
    `usage.cost_details.upstream_inference_cost`（真 >0 / 替身 `0`）、
    `reasoning_details[*].format == "openrouter-stealth-v1"`（替身）；
  - `system_fingerprint` **不再是判据**：真分支里只有 vLLM 系后端才给；
  - 两条分支的 `model` 都回 `z-ai/glm-5.3-flash`，**不能**用它判断；
  - 2026-11-09 的替身是 `crouton-alpha` 的 stealth 通道（11-08 是 `muse-wonder`）：
    换后端但机制未变。详见 `docs/recon.md` §3.1。
- **代码落点**：`deepai.Client.BrowserFingerprint`（`internal/deepai/client.go`，
  `NewClient` / `NewClientWithProxy` = 推理形态，`NewClientWithFingerprint`
  = 注册形态）、`desktopInferenceHeaders`。
- **另一个准入条件**：光有 `node` UA 还不够，还得**完成签到**（`/theos` 拉
  nonce 再回传）。实测只拉不回传 = 0/14 对齐，完整签到 = 14/14 对齐；
  判定按 (账号, 机器指纹) 记忆，换指纹等于冷启动。
  > 这两个数字是 2026-11-08 按「有 `system_fingerprint`、无 `provider`」判的；
  > 11-09 响应形状变了，用新判据（`provider != "DeepAI"`）需重测。
- **签到是有限资源**：`403 check_in_refused`（"restarted its check-in too many
  times today"）说明当天重启次数用尽，要等明天。别写脚本反复「拉 nonce + 回传」
  做体检；被限流的号仍能推理，只是一直落在替身分支。
- **代价**：`user-agent: node` 是 Node fetch 的默认值，桌面端换 HTTP 栈后
  这条要跟着改（见 `docs/recon.md` §3.1）。

---

## 3. 额度与调度

### 3.1 机器指纹必须每号一份

- **现象**：21 个号补齐指纹前 `balance>0` 的是 **0/21**，补齐后 **20/21**
  （唯一 0 是桌面端导入号，当日额度已被消耗）。
- **根因**：上游按 `X-DeepAI-Code-Machine` **每天只发一次**免费 Theos，
  共用指纹时池内账号互相抵消。
- **规则**：注册时生成账号级 UUID 并随记录/号池持久化（`Account.MachineID`），
  推理与 `/theos` 全程复用；只有历史/桌面端导入号才回退到进程级指纹。
- **代码落点**：`deepai.NewMachineUUID`、`Account.MachineID`（推理与
  `/theos` 共用）、`/admin/api/accounts {"action":"assign-machine"}`
  （只补空值）。**不要**为省事去掉这个回退逻辑。

### 3.2 `balance=0` 是「未知」，不是「没额度」

- **现象**：新号入池那一刻 `/theos` 常返回 `balance=0`，几分钟后复探是 200；
  旧实现把这个 0 当事实缓存，账号被调度侧长期跳过。
- **规则**：零额度 + 快照超过 `QuotaStaleAfter`（15 分钟）→ 视为未知，
  允许重新调度；入池/后台复探拿到即停。
- **代码落点**：`internal/pool/pool.go` `QuotaStaleAfter` / `HasQuota`；
  `internal/server/register.go` `quotaReprobeDelays`（45s / 2m / 6min）。

### 3.3 复探要三层 + 全局槽位 + 在途去重

- **现象**：早期只能人工点「刷新全部配额」，653 个号要 11 分钟，
  且一次 HTTP 请求跑不完；反过来全量并发扫描又是明显的风控特征。
- **规则**：三条路径共享机制 ——
  1. **入池复探**：45s / 2m / 6min，拿到即停；
  2. **按需复探**：推理选中「快照过期」的号，每 15min 最多一次；
  3. **后台守护**：每 5 分钟一批（≤24 个），只挑 `refresh_at` 已过 /
     从未探测 / 超 6h 未探；
  4. 共享在途去重表 + **全局 4 个探测槽位**。
- **代码落点**：`internal/server/probe.go`（`probeWorkers`、`QuotaGuardInterval`、
  `QuotaGuardBatch`、`quotaNeedsRefresh`、`ReprobeStale`）。

### 3.4 探测失败不能改调度状态

- **现象**：上游抖一下就把账号标成 `error` / 累加 `Failures`，号池被误判。
- **规则**：复探失败只记日志、**不改** `Status` / `Failures`（不调
  `SetAccountAllowance`）；只有真实推理失败才影响调度计数。
  例外是策略终态（`social_login_required`）必须落盘为 `blocked`，
  否则调度会一直重试一个永远不会成功的号。
- **代码落点**：`internal/server/probe.go` `probeAsync`（错误分支直接
  `return`，仅策略错误走 `BlockAccount`）vs `MarkAccountResult`。

### 3.5 长任务要分批 + 进度语义

- **规则**：管理接口不要设计成「一次跑完 N 百个号」。用
  `stale_only` / `limit` / `remaining` 让调用方翻页；每批探过的 `checked_at`
  推到现在，下一批自然挑到没探过的。
- **代码落点**：`POST /admin/api/checkin`（`maxCheckinBatch`），
  控制台按 `remaining` 自动翻页。

### 3.6 出口既是产能，也是额度边界

- **现象**：把 14 个号分散到 12 个出口后，有额度账号从 5 个 → 10 个。
- **根因**：每日免费 Theos 是**按网络（出口 IP）限额**发放的
  （`network_bound`，见 `docs/recon.md`）。
- **规则**：加号之前先加出口；并发上限对齐出口数
  （`CapConcurrency`），否则只会把 p95 抬高而不是提吞吐。

---

## 4. 观测与验收口径

### 4.1 没有逐号埋点就没有优化

- **规则**：每个账号都要有 `CreatedAt` / `FinishedAt` / 分阶段 `Timings`，
  `E2E = FinishedAt - CreatedAt`（**不含**排队时间）。
  没有逐号数据的批次只能看到墙钟，无法归因。
- **代码落点**：`internal/register/types.go` `Record.Mark` / `E2ESeconds`,
  `pipeline_batch.go` 写 `FinishedAt`；控制台注册页显示单号均值 / p50 / p95 / 阶段耗时。

### 4.2 看中位和长尾，不要只看均值

- **现象**：第八批上游整体变慢，**中位 11.9s 几乎没动**，但 p90 从 19.1s
  涨到 89.2s、均值涨到 25.4s（被几个 120s 超时拖的）。
- **规则**：
  - **中位 = 架构稳定性**（是否与上游波动解耦）；
  - **p90 / max = 依赖健康度**（哪个上游端点出问题了）；
  - 均值只用于总额/吞吐核算，不要拿来比较批次好坏。

### 4.3 利用率决定下一步动作

- **规则**：`利用率 = sum(E2E) / 墙钟`。
  - 利用率 > 90%：瓶颈在单号耗时 → 优化阶段；
  - 利用率 < 75%：瓶颈在排队/尾波/出口 → 加出口或调并发；
  - 单号耗时已由外部服务决定时，继续在网关侧优化是负收益。

### 4.4 复跑同配置才叫对照

- **现象**：第四批 233s / 第五批 388s，配置完全相同，差异 100% 来自邮件投递。
- **规则**：任何「提速 X%」的结论必须基于**同配置 ≥2 批**，并区分
  「代码收益」与「上游波动」；上游故障窗口内的批次只能用来验证**降级行为**，
  不能用来评价性能。

### 4.5 验收口径要拆开写

- **规则**：入池数 / 额度数 / 验证数分开计数，补跑单独标注
  （写成 `49/50 → 补 1 → 50/50`），不要合成一个「成功率」。
  三个数的物理含义完全不同：入池是可调度，额度是能推理，验证只是长期稳定性。

### 4.6 注册面和数据面分开压

- **规则**：注册 100% 只说明账号建得出来；必须另打一轮数据面
  （如 60~120 请求 / 并发 6~12，统计 rps / p50 / p90 / 账号覆盖），
  否则「号池满格但推理全 429/403」要等到线上才发现。

---

## 5. 改动前检查清单

| # | 检查项 | 不满足的后果 |
| --- | --- | --- |
| 1 | 新增的上游调用有总预算且可被 ctx 打断 | 单号白等 2 分钟，墙钟被一个号拖长 |
| 2 | 新增的等待不在关键路径上（或已证明必要） | 单号 E2E 翻倍，且随外部服务波动 |
| 3 | 新增的后台任务有独立 ctx + 写回用副本 | 竞态 / 数据丢失 / 进程退出即丢 |
| 4 | 重端点有全局并发闸门 | 自己把上游打成 502/EOF（0/50） |
| 5 | 重试只在一层，且按错误类型分流 | 请求放大 3~9 倍，触发风控 |
| 6 | 零额度/失败判定不会误伤调度 | 号池「有号不可用」，需人工清错 |
| 7 | 有逐号计时与阶段埋点 | 无法归因，只能靠猜 |
| 8 | 验收数字按入池/额度/验证拆开 | 虚报成功率，掩盖真实退化 |
| 9 | 状态文件/台账改动仍保持脱敏与 `0600` | 密码与 Key 泄露（见 README §6.12） |
| 10 | 批次并行时出口数 ≥ 并发数 | p95 暴涨，吞吐不升 |

---

## 6. 边界：这些经验什么时候会失效

- **上游把邮箱验证变成额度/推理前置条件**：`verify` 必须回到关键路径，
  但要保留限流（§2.3），并用 `email_verified` 比例做告警提前发现；
- **临时邮箱邮件保留时间很短**（实测几小时后收件箱已空）：
  所以「批次内完成验证」是硬约束，`-reverify` 只是补齐手段，不是兜底方案；
- **`/theos` 语义变化**（例如零额度不再自动刷新）：§3.2 的「0 = 未知」
  会变成误判，需要在复探里加入 `refresh_at` 校验兜底；
- **上游对社交登录的收窄**（0.3.19 起邮箱密码号 `403 social_login_required`）：
  这是**策略性**失败，任何重试/换出口都无效，必须识别并标记 `blocked`。
  注意它是**阶段性**的：2026-11-09 复测该限制已解除（见 `recon.md` §4.1），
  但错误码处理必须保留——策略随时可能再次收紧，别把「邮箱号可用」写死成判据。
