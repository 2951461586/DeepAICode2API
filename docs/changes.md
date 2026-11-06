# 改造记录（2026-11）

一轮以「可交付、可回归」为目标的改造。在这里按时间顺序记下**根因 → 决策 → 验证**，
方便以后判断某项设计为什么长这样，以及哪些结论是实测出来的、哪些是推断。

完整的方案与验收标准见 `optimization-plan.md`（那是施工前写的计划，本文是施工后的事实）。

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
