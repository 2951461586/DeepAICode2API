# 优化方案 R2：基于两批 50 号实测（前后端）

> 数据日期 2026-11-06 晚。所有数字都是本轮实测，不是估算；
> 标「估算」的条目已注明。上一轮方案见 `optimization-plan.md`（已实施完毕）。

---

## 0. 本轮实测快照

### 0.1 全链路（注册 → 登录 → 取 Key → 验活 → 入池）

| 批次 | 账号 | 并发 | 墙钟 | 注册 | 取 Key | 验活 | 入池 | 邮箱验证 |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `stress-50l` | 50 | 5 | **2m18s** | 50/50 | 50/50 | 50/50 | 50/50 | 50/50 |
| `stress-50m` | 50 | 5 | **3m17s** | 50/50 | 50/50 | 50/50 | 50/50 | 48/50 → `-reverify` 后 50/50 |

单号 E2E：`50l` 均值 **11.1s**（p50 11.1 / p95 19.3 / max 21.3）；
`50m` 均值 **12.9s**（p50 11.8 / p95 23.1 / max 29.9）。

阶段均值（秒）：

| 阶段 | 50l | 50m | 说明 |
| --- | ---: | ---: | --- |
| mailbox | 0.38 | 0.51 | 建临时邮箱 |
| register | 1.26 | 1.30 | 注册 POST |
| login | 1.10 | 1.21 | 注册后登录校验 |
| session | 0.68 | 0.88 | 取 API Key |
| validate | 3.60 | 4.31 | `/theos` 验活（最大单项） |
| verify | 2.51 | 7.08 | 收信+点链接（**后台，不占关键路径**） |
| enroll | 0.01 | 0.01 | 入池 |

### 0.2 数据面（同分组临时密钥，模型 `glm-5.3-flash`）

| 场景 | 额度覆盖 | 请求/并发 | 成功率 | rps | p50 | p90 | p95 | max |
| --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| 50l 冷（入池后立即） | 12–15/50 | 60/6 | 100% | 3.04 | 1279 | 4129 | 5276 | 9615 |
| 50l 热（3 分钟复探后） | 50/50 | 60/6 | 100% | 3.28 | 1111 | 2257 | 3432 | 6804 |
| 50l 热 + 高并发 | 50/50 | 120/12 | **100%** | **9.50** | 1083 | **1772** | 1967 | 2725 |
| 50m（部分额度） | 28/50 | 60/6 | 100% | 4.71 | 1141 | 1818 | 2046 | 2476 |

网关侧 500 条 chat 日志：全部 200、零错误、覆盖 236 个账号。

**结论：并发不是瓶颈。** 12 并发反而比 6 并发更快（9.50 vs 3.28 rps），
说明 60/6 的尾部（p90 4129ms）是**冷连接/预热**造成的，不是排队。
上游 p50 ≈ 1.1s 是硬地板。

### 0.3 额度复探（实测时间线）

入池时刻 `balance=0` → 3 分钟复探后 **47/50 拿到 200** → 9 分钟复探后 **50/50**。
网关日志 `复探拿到额度` 累计 88 条。

### 0.4 资源开销（本轮暴露的最大工程问题）

| 指标 | 实测值 |
| --- | --- |
| `/admin/api/state` 响应体 | **438,943 bytes**（603 账号，8ms） |
| 状态文件 | **1,135,122 bytes**（653 账号 + 500 日志） |
| 状态文件重写频率 | **≤30s 一次全量重写 + fsync**（日志每轮都变，`Save` 的“无变化跳过”永远不生效） |
| 日志保留 | `LogKeep=500`，压测时**瞬间顶满**（1500+ 请求只留得下 500 条） |

按 1.13MB / 30s 计：**约 130MB/小时**的磁盘写入，且每次重写都在持锁状态下
`json.MarshalIndent` 整个状态。

---

## 1. 后端方案

### P0-1 状态文件拆分：accounts 与 logs 分家

**根因**：`store.Save()` 序列化整个 `State`（含 500 条日志与 653 个账号），
日志每 30s 必变 → 每次都全量重写 1.13MB 并 fsync。
这才是「定期保存」的真实现金成本，而不是 CPU。

#### 方案

1. `deepaicode2api-data.json` 只保留 `accounts / keys / settings / models / stats`；
2. 日志改为独立 **append-only + ring 文件** `logs.jsonl`（或 `logs/YYYYMMDD.jsonl`），
   `Store.Record` 只追加一行，定期按 `LogKeep` 截断；
3. `Save()` 增加脏标记：`accounts/keys/settings` 未变则跳过。

**验收**：稳态下 `Save()` 单次写入 < 100KB；`logs.jsonl` 追加不触发全量序列化。

---

### P0-2 `/admin/api/state` 分页化

**根因**：一次返回全部账号（603 个 × 约 20 字段 = **429KB**），
而控制台一屏只渲染 20 行。前端分页是**客户端分页**，网络与解析成本全额支付，
且每次操作后 `refresh()` 都重下 429KB。

#### 方案

1. 新增 `GET /admin/api/accounts?offset=&limit=&group=&status=&q=`（默认 50，上限 200），
   返回 `{items, total}`；
2. `/admin/api/state` 拆为
   - `GET /admin/api/overview`：stats + keys + models + settings（< 20KB）；
   - 账号列表走上面的分页端点；
3. 两个端点都加 `ETag`（对 `stats.total_requests` + `models_updated` + 账号数做摘要）
   并支持 `If-None-Match` → 304。

**验收**：控制台首屏总传输 < 60KB；空闲刷新返回 304。

---

### P1-1 入池时的 0 额度不应被当作「刚探过的事实」

**根因**（本轮实测确认）：`enrollRegistration` 用
`CheckedAt: time.Now().Unix()` 写入上游返回的 `balance=0`。
`pool.HasQuota` 的判定是「余额 0 且**刚探过**」→ 返回 false
→ 该账号被排除在调度第一轮之外，直到 15 分钟陈旧阈值或复探成功。
实测后果：入池后立刻压测，**50 个号里只有 12–15 个可被选中**，
数据面覆盖仅 19/50。

**方案**（择一，推荐 A）

- **A. 区分「未知」与「确认 0」**：入池瞬间的 0 额度写 `CheckedAt=0`
  （= 未知），让调度把它当可用；真没额度时上游会拒，`MarkAccountResult`
  自动加失败计数并降权。代价是每个新号最多浪费一次上游请求。
- B. 复探提前：`3/6/12min` → `45s/2m/6min`，把「可用」时间从 9 分钟压到 2 分钟；
  但仍存在 2 分钟的空窗。
- 可两者叠加：A 解决空窗，B 加快真实额度的落库。

**验收**：新入池 50 号在 60s 内的「可调度数」≥ 45；数据面覆盖 ≥ 40/50。

---

### P1-2 每账号并发上限 + 整体 deadline

**根因**：`openModel` 只限制总重试次数（`MaxRetries=3`），没有
「单账号同时在途请求数」限制，也没有「整个请求的 deadline」。
`CapacityWaiting` 单次最长等 30s，3 次尝试理论上可挂 90s+。
120/12 压测时 12 个并发可能全落在同 1–2 个账号上。

#### 方案

1. 每账号 `semaphore(2)`，取不到就换号（复用现有换号逻辑）；
2. 非流式请求加整体 `context.WithTimeout(60s)`；流式改为 **idle timeout**
   （例如 90s 无新事件则收尾），而不是总时长限制。

**验收**：单请求 P99 < 10s（当前冷启动 max 9.6s 来自连接建立，不是重试）。

---

### P1-3 验证窗口 120s 会漏掉约 4%

**根因**：`VerifyWait=120s`，`verifyBackfill` 的 ctx 为 `wait+2min`。
本轮 `50m` 有 **2/50** 在窗口内没收到验证邮件，记录停在 `verify: pending`。

#### 方案

1. 批次结束后自动对 `verify:pending` 的账号跑一次 `Reverify`
   （可复用现成 CLI 逻辑，`server` 侧已有 `register.Reverify`）；
2. 或把 `VerifyWait` 提到 180s，并让后台回填的 ctx 与批次解耦（现已解耦，
   但 2 个样本仍超时）。

**验收**：批次内 `email_verified` 达成率 ≥ 99%（50/50）。

---

### P1-4 日志容量与可检索性

**根因**：`LogKeep=500` 在压测下瞬间顶满，事后无法按账号/状态/时间回溯
（本轮 1500+ 请求只留下最后 500 条）。

**方案**：配合 P0-1 的 append-only 日志文件，`GET /admin/api/logs`
支持 `since / endpoint / status / account / cursor` 过滤与游标分页；
`LogKeep` 改为「按文件滚动 + 保留 N 天」而非内存条数。

---

### P2-1 注册链路阶段并行

`session`（取 Key，0.68–0.88s）与 `SaveSignupData` 可以并发；`validate`
（3.6–4.3s）依赖 Key，无法提前。真正的大头是**并发度**：12 条出口下
`-concurrency 5` 偏保守，可试 8（上游风控风险需灰度验证）。

**验收**：`-concurrency 8` 时批次墙钟 < 2m，失败率不升。

### P2-2 连接预热

冷启动 p90 4129ms → 热 1772ms。可在进程启动时对每个代理建一次连接
（HEAD `/deepai_code/models`），把首包 RTT 从压测里摘出去。

---

## 2. 前端方案（`internal/server/web/index.html`，876 行单文件）

### P0-1 首屏与刷新改为「小端点 + 服务端分页」

**根因**：`refresh()` 拉 `/admin/api/state`（429KB），
`renderAccounts` 只渲染 20 行，其余全部浪费；每次增删改后都重拉一次。

#### 改法

- `refresh()` → `Promise.all([/admin/api/overview, /admin/api/accounts?offset=…])`；
- 分页控件由「本地 slice」改为「触发带 `offset` 的请求」；
- 表格行保留 `data-id`，后续操作只更新单行。

**验收**：首屏请求数 ≤ 3、总字节 < 60KB；翻页不再下载全量。

### P1-1 请求去重与竞态

**根因**：`refresh()` 没有 in-flight 标记，连点「刷新」或多个操作并发时，
后返回的旧响应会覆盖新状态（last-write-wins）。

**改法**：模块级 `var inflight = null;`，同 URL 复用 promise；
配合 `AbortController` 在切页/关页时取消。

### P1-2 管理密码不再落 localStorage

**根因**：`localStorage.setItem("deepaicode2api_pw", pw())` 永久驻留，
同源任意脚本可读。控制台虽无外链，但这是一条不必要的持久化面。

**改法**：默认只放内存；「记住本机」勾选后才写 `sessionStorage`（关页即失效）。

### P2-1 后台标签页暂停轮询

`setInterval(loadHealth, 20000)` 与注册轮询（2s）在后台标签页仍在跑。
用 `document.visibilityState` 暂停/恢复。

### P2-2 fetch 超时

当前 `fetch` 无超时，请求悬挂会让 UI 永久停在旧状态。加 15s `AbortSignal.timeout`。

### P2-3 渲染

`renderAccounts` 每次 `tb.textContent = ""` 后重建，100/页 × 10 列 = 1000 个节点。
当前规模可接受；若配合 P0-1 改成服务端分页后，单页 ≤ 50 行，可再加
「keyed diff 只更新变化行」。

### P3 文件组织

单文件 876 行、无构建步骤是**有意为之**（离线可用、`go:embed` 一行）。
建议保持单文件，但按 `style / api / render / events` 加显式分节注释；
若超过 1500 行再拆成多文件（`go:embed` 支持 `//go:embed web/*.css web/*.js`）。

---

## 3. 优先级与预期收益

| 优先级 | 事项 | 预期收益 | 风险 |
| --- | --- | --- | --- |
| **P0** | 状态文件拆分（accounts/logs） | 磁盘写放大约 130MB/h → < 5MB/h；`Save` 不再序列化 1.1MB | 中（涉及持久化格式，需迁移脚本） |
| **P0** | `/admin/api/state` → overview + 分页账号 | 控制台 429KB → < 60KB | 低（新增端点，旧端点保留兼容） |
| **P1** | 入池 0 额度按「未知」处理 | 新批可用时间 9min → < 1min；数据面覆盖 19/50 → ≥40/50 | 低-中（多一次上游请求） |
| **P1** | 每账号并发上限 + 整体 deadline | 消除单账号热点的长尾 | 低 |
| **P1** | 验证窗口 + 自动 reverify | 验证达成率 96% → ≥99% | 低 |
| **P1** | 日志 append-only + 过滤查询 | 压测后可回溯 | 低 |
| **P2** | 注册并发 5 → 8 | 批次墙钟约 -30%（估算） | 中（上游风控） |
| **P2** | 连接预热 | 冷启动 p90 4129 → ~2000ms | 低 |
| **P2** | 前端竞态 / 密码存储 / 轮询暂停 | 稳定性与安全 | 低 |

---

## 4. 复现命令

```bash
# 全链路
./bin/register.exe -count 50 -concurrency 5 -group stress-50n -result-dir result/stress50n

# 等 3 分钟复探后压数据面（同分组临时密钥）
KEY=$(curl -s -X POST localhost:10087/admin/api/keys -H 'Content-Type: application/json' \
      -d '{"name":"load","group":"stress-50n"}' | jq -r .key)
LT_TOTAL=120 LT_CONC=12 python bin/loadtest.py   # 见 bin/loadtest.py

# 补验证
./bin/register.exe -reverify -group stress-50n -concurrency 2
```
