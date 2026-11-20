# `internal/server` 拆分方案

> 对象：`internal/server` —— 39 个非测试文件 / 8,008 行；39 个测试文件 / 8,474 行。
> 结论：**不要一次性拆包**。按「叶子模块 → 中间层 → 内核门面」五个 PR 分批推进，
> 前四个 PR 收益明确、行为零变更；最后一个（relay/admin 拆包）按需再做。

---

## 一、现状度量（决定方案的事实）

| 指标 | 数值 |
| --- | --- |
| 非测试代码 | 8,008 行 / 39 文件 |
| 测试代码 | 8,474 行 / 39 文件，**全部 `package server`（同包白盒）** |
| `Server` 结构体字段 | 36 个（无子结构分组） |
| `Store` 被引用 | **23/39 个文件** |
| 最大单文件 | `login.go` 479 / `response_history.go` 455 / `admin_actions.go` 424 |
| 包内环 | 0（无循环导入风险） |
| 分层违规 | 0（`internal/server` 从不被下层反向导入） |

包内耦合最密的几条边（按被引用符号数）：

```
upstream.go      -> accounts.go       8  Close/blockedReason/errAllAccountsBusy/
                                          pickFreeAccount/releaseAccount/...
observability.go -> metrics.go        7  直方图/快照/标签
responses.go     -> responses_stream.go 7 流式生成器
server.go        -> admin.go          7  路由表 → admin handler
anthropic.go     -> reqlog.go         6  请求日志
responses.go     -> reqlog.go         6
```

测试侧耦合（这是拆分成本的主要来源）：

```
s.Store            138 次      s.Handler           17 次
s.metrics           11 次      s.pickFreeAccount    7 次
s.stickyAccount      6 次      s.CurrentLane        6 次
s.acquireAccount     5 次      s.WaitQuotaReprobe   5 次
```

**推论**：`*Server` 是共享可变状态的宿主，任何「把 handler 挪走」的方案都必然
先回答「状态怎么给」；而测试全部同包白盒，所以包边界一动，测试就跟着动。
这决定了拆分必须从**没有状态依赖的叶子**开始，而不是从最大的文件开始。

---

## 二、目标形态

```
                       main.go / tasks.go
                              │
                    internal/server  （内核）
                    Server 状态 · 路由表 · 生命周期 · 优雅关闭
                    /    |      |      |      |      \
        poolsvc   relay  adminapi  observe  guard  respstate  (web 已完成)
         号池     数据面   控制面    可观测   准入     Responses
        治理     协议面   页面/API   指标     限流     历史重放
                              │
                  internal/{deepai,store,gateway,httpx,pool}
```

依赖方向**只允许向下**：`relay → {poolsvc, respstate, observe, guard}`，
`adminapi → {poolsvc, observe, guard}`，内核 → 全部；**任何子包不得 import
`internal/server`**（用 CI 断言，见 §五）。

关键设计约束：

1. **子包不接收 `*server.Server`**。每个子包自己定义配置结构体（`relay.Config{...}`），
   内核在 `New()` 里填充。这样没有循环导入，也不会退化成「把 Server 结构体
   复制一遍」。
2. **内核只传函数值和数据**，不传访问器集合。例如选号给的是
   `Pick func(ctx, group string) (*deepai.Account, error)`，不是
   `func (s *Server) Pick(...)`。
3. **状态跟着领域走**：`responses`/`probe`/`sticky` 这些字段随宿主包一起搬，
   而不是留在 36 个扁平字段里让所有包共享。

---

## 三、分阶段方案（每个 PR 都要求行为零变更）

### PR-1 `internal/respstate` — Responses 历史重放（455 行）✅ 已完成（2026-11-20）

**实际结果**：`response_history.go` → `internal/respstate/history.go`（774 行，含本轮新增逻辑），
`respstate` 只依赖 `internal/gateway`，不依赖 `internal/server`；单元测试 15 个进新包，
HTTP 端到端 11 个留在 `internal/server`。`internal/server` 非测试代码 8,008 → 7,557 行。
详见 `docs/changes.md` §71.5。

**为什么先做**：`response_history.go` 不碰 `Server` 任何字段（0 处 `s.`），
只依赖 `gateway.StrOf` 与自身的工具函数。它是唯一可以「整文件平移」的模块，
也是最近三个提交（issue #5）的焦点，值得独立可测。

**搬迁**：`response_history.go` → `internal/respstate/history.go`。

**导出面（草图）**：

```go
package respstate

type Turn struct { Input []any; Instructions string; Parent string }
type Outcome int
const ( OK Outcome = iota; NotFound; TooLong )

type Store struct{ /* ttl, max, maxBytes, entries, checkpointEvery, ... */ }
func New(ttl time.Duration, max int) *Store
func NewFromEnv() *Store
func (s *Store) Remember(turn *Turn, respID string, output []any)
func (s *Store) Replay(payload map[string]any, prevID, modelID string,
                       catalogWindow int) (Outcome, string)
func (s *Store) Len() int

// 请求体 → item 切片（含 string input 的正规化，前缀/后缀比对依赖它）。
func InputItems(payload map[string]any) []any

// 上下文窗口换算与 token 估算一并搬走（它们只服务这条链路）。
func EffectiveContextWindow(modelID string, catalog int) int
func ItemApproxTokens(v any) int
```

（落地时没有导出 `Get`：`Replay` 走 `chain` 后它只剩测试调用，已删除。）

**内核侧改动**：`history *respstate.Store`；`responses.go` 只改 4 处调用
（`Replay` / `Turn{}` / 两个 `Outcome` 常量）；`accounts.go` 的 `s.history.len()`
→ `s.history.Len()`。

**测试迁移**：`response_history_test.go` 一分为二：
- 单元（`truncateHistory*`、`itemApproxTokens`、`effectiveContextWindow`、
  `historyEntry` 容量/TTL）→ `internal/respstate/history_test.go`；
- 端到端（3 轮链式记忆、`truncation:auto`、400 语义、tools 计入）→ 留在
  `internal/server`，改用 `respstate` 的导出 API。

**风险**：低。收益：内核少 455 行；token 估算与裁剪逻辑可以单独 bench。

---

### PR-2 `internal/guard` — 准入与来源校验（430 行）✅ 已完成（2026-11-20）

**实际结果**：`internal/guard` 共 426 行（`limiter.go` 193 / `adminauth.go` 142 / `cors.go` 91）
+ 311 行测试（13 个用例）；`internal/server` 只剩 104 行胶水（`rateLimitGate` 21 /
口令退避的 HTTP 语义 57 / `withCORS` 26）。`internal/server` 非测试代码 7,557 → 7,226 行。
详见 `docs/changes.md` §72。

**搬迁**：`ratelimit.go`(181) + `authguard.go`(160) + `cors.go`(89)。

**为什么能搬**：三者都只依赖 `Store`（读设置）与请求本身，互不引用；
`Server` 只持有它们的实例（`limiter` / `adminGuard` / `adminGuardBase`）。
这是「安全层」的天然接缝，拆出去之后可以独立压测与单测。

**导出面（草图）**：

```go
package guard

type Limiter struct{ /* buckets, inflight */ }
func NewLimiter() *Limiter
// Gate 返回放行函数；超限时返回 retryAfter。设置从 Store 快照传入，
// 避免 guard 反向依赖 store 的具体结构。
func (l *Limiter) Gate(keyID string, cfg RateConfig) (release func(), retryAfter time.Duration, err error)

type AdminGuard struct{}
func NewAdminGuard() *AdminGuard
func (g *AdminGuard) Allow(ip string) (retryAfter time.Duration, ok bool)
func (g *AdminGuard) NoteFailure(ip string)
func (g *AdminGuard) NoteSuccess(ip string)

func CORSHeaders(w http.ResponseWriter, allowed string)
func AllowedOrigin(raw string, cfg Origins) string   // 原 s.corsOrigin 的纯函数化
```

**测试迁移**：`ratelimit_test.go`(177) + `authguard_test.go`(100) 整体搬走；
`auth_test.go` 里 CORS 相关子用例跟着走，鉴权（`withAuth`）留在内核。

**风险**：低-中。CORS 响应头属于对外契约，搬迁时保持
`writeCORSHeaders` 的 `Set` 顺序与取值不变；`console_*` 契约测试会兜住。

---

### PR-3 `internal/observe` — 指标与访问日志（688 行）✅ 已完成（2026-11-20）

**实际结果**：`observe` 680 行（`metrics.go` 379 / `exposition.go` 155 / `middleware.go` 146）
+ 178 行测试；server 只剩 80 行装配（`poolSnapshot` 返回 `observe.PoolSnapshot`、
`handleMetrics` 组装 `PromExtra`）。`internal/server` 非测试 7,226 → **6,607** 行（文件 39 → 38）。
详见 `docs/changes.md` §73。

**与计划的差异**：没有做 `LaneSource` / `PoolSnapshotSource` 两个接口——
`Middleware(m, next)` 只依赖 `*Metrics`，`handleMetrics` 用值结构 `PromExtra` 收数字，
比接口更窄、更好测。顺带删掉了只写不读的 `metrics.upstreamLane` 镜像。

**搬迁**：`metrics.go`(332) + `observability.go`(356) 中的指标与中间件部分。

**落地形态**（比原计划的「窄接口」更窄：中间件不依赖任何 Server 状态，业务数字
用值结构传入）：

```go
package observe

type Metrics struct{ /* 原子量 + 直方图 */ }
func NewMetrics() *Metrics
func (m *Metrics) Observe(endpoint string, status int, d time.Duration)
func (m *Metrics) ObserveBasic(status int, d time.Duration)
func (m *Metrics) ObserveDuration(endpoint string, sec float64)
func (m *Metrics) ObserveUpstreamError(path, code string, status int)
func (m *Metrics) ObserveCheckIn(attempted bool, waited, slowAfter time.Duration, timedOut bool)
func (m *Metrics) ObserveAdmissionWait(d time.Duration)
func (m *Metrics) AddFallback() / AddStateful(param string, rejected bool)
func (m *Metrics) Snapshot() MetricsSnapshot
func (m *Metrics) WritePrometheus(w io.Writer, extra PromExtra)

func Middleware(m *Metrics, next http.Handler) http.Handler // request-id + 访问日志 + 指标
func EndpointLabel(path string) string
func IsStaticAsset(path string) bool

type PromExtra struct { Pool PoolSnapshot; LogRequests, LogFailed int64; Keys, Lane, CompiledLane int; UpstreamLane int64 }
```

`poolSnapshot()` 留在 server（要 store + 额度策略），返回 `observe.PoolSnapshot`；
`/health` 与 `/metrics` 共用它，避免两处口径打架。

**测试迁移**：纯单测（直方图 / 扩展指标 / 静态资源）搬进 `internal/observe`；
HTTP 级用例（request id 回显、/metrics 文本、overview 字段）留在 server，
并新增 `TestWritePrometheusContract` 逐条锁定指标名与标签。

**风险**：中。`/metrics` 的指标名、标签、分位算法是外部契约（看板/告警）。
搬迁时**只改包路径，不改名字**，并用黄金用例做逐字比对。

---

### PR-4 `internal/poolsvc` — 号池治理（约 1,012 行）✅ 已完成（2026-11-20）

**实际结果**：`poolsvc` 1,123 行（slots 133 / probe 396 / enroll 158 / sticky 154 /
proxypool 38 / warmup 76 / Service 168）+ 68 行白盒测试。`Server` 字段 36 → 23，
`internal/server` 非测试 6,607 → **5,610** 行（文件 38 → 33）。注入面只有
`Config{Store, ClientFor, Save}`；原计划的「能力回调」最终没做——它们本来就是
poolsvc 自己的状态操作。详见 `docs/changes.md` §74。

**搬迁**：`probe.go`(394) + `enroll.go`(157) + `sticky.go`(156) +
`proxypool.go`(38) + `accounts.go`(191) + `warmup.go`(76)。

**为什么值得做**：它一次性解掉两条最密的耦合边
（`upstream.go → accounts.go` 8 个符号；`enroll/admin_actions → probe`）。
`internal/pool` 已经是**纯选号算法**（`Scheduler.Pick`，无状态、可 bench），
`poolsvc` 承接它的**有状态另一半**：在途计数、探测去重、粘性绑定、入池分配。
两块拼起来才是 README 里说的「号池治理」。

**导出面（草图）**：

```go
package poolsvc

type Service struct{ /* inflight, probing, sticky, enrollPool, wg */ }
func New(cfg Config) *Service
func (p *Service) Acquire(ctx context.Context, acct *deepai.Account) (release func(), err error)
func (p *Service) Pick(ctx context.Context, group string) (*deepai.Account, error)
func (p *Service) Probe(acct *deepai.Account, opts ProbeOpts) (*deepai.Quota, error)
func (p *Service) BindSticky(affinity, accountID string)
func (p *Service) StickyAccount(affinity string) (string, bool)
func (p *Service) Close()                     // 等 quotaWG 收尾
```

`Config` 只收 `Store`、`*deepai.Client` 工厂、`Save` 回调、`Metrics` 钩子。

**风险**：中-高。这里全是并发原语（`inflightMu`、`probeMu`、`stopCh`、
`quotaWG`）。搬迁时**保持锁的粒度与顺序不变**，用 `go test -race` 与现有的
竞态回归用例（`*_race_test.go`、`save_race_test.go`）作为门槛。

---

### PR-5（可选）`internal/relay` + `internal/adminapi` — 数据面/控制面（约 4,000 行）

**只有在确实需要「多个二进制」或「独立演进的协议层」时才做。** 成本主要不在
代码，而在**内核门面**：handler 们今天直接摸 `s.nextAccount` / `s.openModel` /
`s.metrics` / `s.shapeNeeded` / `s.rateLimitGate` / `s.history`，拆包后这些都要
变成 `relay.Config` 里的函数值。

建议的边界（如果做）：

```go
package relay
type Config struct {
    Up      *deepai.Client
    History *respstate.Store
    Metrics *observe.Metrics
    Pick    func(ctx context.Context, r *http.Request) (*deepai.Account, func(), error)
    Record  func(Entry)                 // reqlog 落盘
    Shape   func(key string) bool       // shape.go
    Timeout func() time.Duration        // idle.go / 设置
}
func (h *Handlers) Chat(w, r) / Responses(w, r) / Anthropic(w, r) / Models(w, r)
```

`adminapi` 同理，额外需要 `Post` 动作（`admin_actions.go` 的 enroll/import/
assign-proxy）与 `version` 查询。

**不推荐现在做的理由**：handler 与内核状态的耦合是「每个 handler 都要 5～7 个
能力」，拆完会得到一个 30+ 字段的 `Config`；除了包边界好看，没有可测性收益
（端到端测试本来就是 HTTP 级的）。**先做 PR-1～PR-4，再重新评估。**

---

## 四、测试迁移规则

1. **单元测试跟着代码走**：被测符号搬进哪个包，测试就搬进哪个包（同包白盒）。
2. **HTTP 端到端测试留在内核**：只通过 `New(st)` + `Handler()` + `httptest` 的用例
   （`console_*`、`responses_test`、`protocol_fixes_test` 等）不动。
3. **跨域用例拆开**：像 `quota_test.go`(328) 这种同时断言 HTTP 与号池内部的，
   按断言拆成两份，不要为了搬测试去导出内部符号。
4. **禁止**：为了让测试留在 `internal/server` 而给子包加「测试专用导出」。

---

## 五、验收门槛（每个 PR 都跑）

```bash
gofmt -l . && go vet ./... && go build ./...
go test -race -count=1 ./...
node --input-type=module --check < internal/server/web/js/events.js   # 动到前端时
node scripts/console-smoke.mjs                                        # 动到前端时
```

额外的结构断言（建议加进 `.github/workflows/ci.yml` 的 build job）：

```bash
# 子包不得反向依赖内核（拆包后才有意义）
! go list -deps ./internal/respstate ./internal/guard ./internal/observe \
           ./internal/poolsvc 2>/dev/null | grep -q 'deepaicode2api/internal/server$'
```

---

## 六、投入与顺序建议

| PR | 范围 | 移出 LOC | 风险 | 建议 |
| --- | --- | --- | --- | --- |
| 1 | `respstate` | 455 | 低 | ✅ 已完成（-451 行，见 §3 PR-1） |
| 2 | `guard` | 430 | 低-中 | ✅ 已完成（-331 行，见 §3 PR-2） |
| 3 | `observe` | 688 | 中 | ✅ 已完成（-619 行，见 §3 PR-3） |
| 4 | `poolsvc` | ~1,012 | 中-高 | ✅ 已完成（-997 行，`-race` 全绿） |
| 5 | `relay` / `adminapi` | ~4,000 | 高 | **暂缓**，PR-1～4 完成后再评估 |

做完 PR-1～PR-4：`internal/server` **5,610 行**（由 8,008 起步，-30%），
`Server` 的 36 个字段里约 20 个迁到子包的宿主结构体，包内最大文件降到
`login.go`(479)。做完 PR-5 才会降到 1,500 行级，但边际收益明显递减。

---

## 七、明确不做的事

- **不把 `Store` 复制到各子包**：它是有锁的单一状态源，只传递 `*store.Store`。
- **不为拆分导出内部符号**（`pickFreeAccount` / `probeMu` / `stickyDirty` 之类）。
  需要共享的是**能力**，不是字段。
- **不拆 `upstream.go` 单独成包**：它是推理热路径（选号 → 调用 → 错误策略），
  与 `accounts.go` 有 8 个符号的强耦合；要拆就和 PR-4 一起拆。
- **不动 `gateway` / `deepai` / `store`**：它们已经是干净的独立包，
  且 `server → gateway → deepai` 的方向正确。
