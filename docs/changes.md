# 改造记录（2026-11）

一轮以「可交付、可回归」为目标的改造。在这里按时间顺序记下**根因 → 决策 → 验证**，
方便以后判断某项设计为什么长这样，以及哪些结论是实测出来的、哪些是推断。

完整的方案与验收标准见 `optimization-plan.md`（那是施工前写的计划，本文是施工后的事实）。

### 索引（倒序，最新在前）

**版本 → 条目**（发版时对着这里找本次改了什么）：

| 版本 | 条目 | 一句话 |
| --- | --- | --- |
| v0.1.2 | 53、54 | 控制台日志页漏 import（`renderLogs`）+ 二次全扫 7 处遗留 |
| v0.1.2 之后 | 55 | 密钥删除/启停的存在性语义 + 控制台 404 自愈（未发版） |
| v0.1.2 之后 | 56 | 缓存命中率改今日口径、499 客户端断开、docker 构建注入 VERSION（未发版） |
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

### 56.5 验证

```text
sh scripts/lint.sh                  exit 0（gofmt / vet / JS 语法 / 冒烟 / 测试）
node scripts/console-smoke.mjs      21 条全放行（新增 2 条）
go test -race ./...                全绿；internal/server 覆盖率维持 77%+
```

线上实测对照（修复前）：`total_cached_tokens=228,175,986`、
`total_tokens_in=1,510,839,185` → 累计 15.10%（旧卡片显示的就是这个数）；
`day_tokens_in=242,357,695` → 当日命中率由新的 `day_cached_tokens` 给出。
