# 性能测量与优化方案（2026-11）

第三批 50 账号压测的实测数据，以及据此推导的优化方案。
结论先说：**瓶颈不在代码，在两个「等待」**——验证邮件等待（55%）和串行化的阶段顺序。
按 P0 三项改造，单号端到端可从 **27.0s 降到约 15.5s**，吞吐翻倍。

> 本文保留**测量数据**（阶段分解、收益测算、批次基线）；逐批执行日志已不再
> 增补。可复用规则、检查清单与失效边界见 `docs/lessons.md`，
> 当前验收基线见 README §6.11。

---

## 1. 测量方法

`register.Record` 新增 `FinishedAt`（秒，含小数）：

```go
E2E = FinishedAt - CreatedAt     // 该账号从建邮箱到入池的真实耗时
```

- `CreatedAt` 在 `runOne` 开始时写入（**不含**排队等待 `waitTurn` 的时间）；
- `FinishedAt` 在 `finish()` / `resumeWithKey()` 结束时写入；
- 每次尝试都写台账，因此可区分「首次尝试」与「补跑」；
- 阶段耗时来自 `Record.Timings`（每个 stage 的秒数，`Mark()` 写入）。

复现：

```bash
./register.exe -count 50 -concurrency 5 -group stress-50c-$(date +%Y%m%d) \
  -result-dir result/stress50c
# 然后按 group 过滤 exports/register-ledger.jsonl
```

## 2. 三批实测对比

| 批次 | 账号 | 并发 | 出口 | 墙钟 | 入池 | 单号平均 E2E |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| 第一批 | 50 | 5 | 10 | 4m40s | 47（-resume 49） | — |
| 第二批 | 50 | 5 | 12 | 8m23s | 49（-resume 51） | — |
| **第三批** | **50** | **5** | **12** | **4m39s** | **46（-resume 49）** | **27.0s** |

> 前两批未埋点，只有墙钟；第三批起有逐号 `FinishedAt`。

## 3. 第三批：单号端到端速率

| 指标 | 值 |
| --- | --- |
| 样本 | 50 个账号（全部有 E2E） |
| 平均 | **27.0s** |
| 中位数 | 25.3s |
| p90 / p95 | 33.3s / 35.0s |
| 最小 / 最大 | 21.0s / 37.7s |
| 墙钟 | 279s（4m39s） |
| **平均速率** | **5.58s/号 = 10.75 号/分钟 = 645 号/小时** |
| 并发利用率 | **4.83 / 5 = 96.6%** |

并发利用率 = `sum(E2E) / 墙钟`。96.6% 说明 5 个 worker **几乎没有空转**，
吞吐完全由「单号耗时 ÷ 并发数」决定 —— 想提速只有两条路：**降单号耗时**或**加并发**。

## 4. 阶段分解（均值，占 27.0s 的比例）

| 阶段 | 均值 | 占比 | 性质 |
| --- | ---: | ---: | --- |
| `verify`（等验证邮件） | **14.8s** | **54.8%** | 纯等待 |
| `exists`（邮箱是否已注册） | 3.3s | 12.1% | 网络往返（**可省**） |
| `validate`（`/theos` 验活） | 3.1s | 11.6% | 网络往返（必需） |
| `enroll`（入池） | 2.0s | 7.3% | 网络往返（**含重复探测**） |
| `register`（建号） | 1.2s | 4.4% | 网络往返 |
| `login` | 1.1s | 4.1% | 网络往返 |
| `session` | 0.5s | 2.0% | 网络往返 |
| `mailbox`（建收件箱） | 0.4s | 1.4% | 网络往返 |

**单号真实时间线**（第三批样本）：

```text
13:48:52 邮箱就绪
13:48:57 注册成功              (+5s)
13:49:02 取到 API Key          (+5s)   ← 不依赖邮箱验证
13:49:16 邮箱验证完成          (+14s)  ← 纯等待，占满整个中段
13:49:20 Key 验活通过          (+4s)
13:49:25 已入池                (+5s)
```

关键观察：**取 Key 早于验证完成**，所以验证等待完全可以与其他阶段重叠。

## 5. 失败分析（第三批）

50 个账号里 4 个未入池，**全部卡在 `/theos`**：

| 账号数 | 阶段 | 错误 |
| ---: | --- | --- |
| 3 | `enroll` | `HTTP 400 Key 验活失败: .../theos`（网关在入池时**又探了一次**） |
| 1 | `validate` | `unexpected EOF` |

两个特征：

1. **失败集中在同一类瞬时网络错误**，重试即可救回 —— `-resume` 两轮后 49/50；
2. **入池路径重复探测**：pipeline 已经验活过（`validate`），网关的 `enroll`
   又自己探一次 `/theos`。这一次多花 2.0s，还是 3 个失败的直接来源。

---

## 6. 优化方案

### P0-1 入池不再二次探测 `/theos`（预计 −2.0s/号，失败率 8% → ~2%）

**位置**：`internal/server/server.go` `handleAdminAccounts` 的 `actionEnroll` 分支

```go
al, err := s.clientFor(acct).Theos(ctx, acct)   // ← 重复探测
if err != nil { /* 直接 400 拒绝 */ }
```

**方案**：调用方（`register` 链路）已经在 payload 里带了 `theos_balance` /
`theos_granted`，并且刚刚验活成功。改成：

- 请求带 `"skip_probe": true` 且带 `theos_balance` → **信任调用方结果，不再探测**；
- 控制台手工入池仍走探测（人可能填错 Key，值得验）；
- 探测失败时 **重试一次**（1s 退避），只有重试仍失败才拒绝。

**验收**：`enroll` 阶段均值 ≤ 0.6s；`stress-*` 批次里 `keyed` 状态归零。

### P0-2 全新邮箱跳过存在性预检（预计 −3.3s/号）

**位置**：`internal/register/pipeline.go` 第 2 步「预检：邮箱是否已存在」

**方案**：新建的 CF 临时邮箱**必然**未注册，预检只对「补跑」和「用户指定邮箱」
有意义。给 mailbox 返回一个 `Fresh bool`（`cfProvider.Create` 返回 true，
`Inbox(email)` / `static` / `none` 返回 false），`Fresh` 时直接跳过 `exists`。

**验收**：`exists` 阶段从新批次消失；补跑路径仍保留。

### P0-3 验证等待与后续阶段并行（预计 −6.2s/号，−23%）

**位置**：`pipeline.go` 的 `runOne` 阶段顺序

**方案**：注册成功后立刻触发验证邮件，然后**不要阻塞**，继续 login → key；
把 `WaitForVerification` 放到独立 goroutine，在 `validate` 之前 `join`：

```text
现在:  mailbox → register → login → key → verify(14.8s) → validate → enroll
改后:  mailbox → register → login → key ┐
                              verify ────┴→ validate → enroll      (并行)
```

线程模型不变（一个账号仍占一个 worker），只是把等待藏在其他网络往返后面。

**验收**：E2E 均值 ≤ 21s；`verify` 与 `key` 的时间区间在日志里重叠。

### P1-1 并发 5 → 10（预计吞吐 +80%）

利用率 96.6% 说明 5 并发已打满；出口有 12 条，`-concurrency 10` 仍低于出口数。
前提是上游不触发风控 —— 建议先跑 20 个账号观察 `register` 阶段的错误率。

**验收**：50 账号墙钟 ≤ 2m30s；失败率不高于 5%。

### P1-2 `/theos` 失败退避重试（失败率 8% → <2%）

**位置**：`pipeline.validate` 与 `client.Theos` 调用点

现有 `Settings.MaxRetries` 只管推理请求，探测没有重试。加：网络错误 /
5xx / 429 重试 2 次（1s、3s 退避）；401/403 不重试（那是 Key 真有问题）。

**验收**：`unexpected EOF` / `No exit node` / `TLS handshake timeout` 类失败
在 50 账号批次中出现次数 ≤ 1。

### P2-1 收信轮询 1s → 300ms

`verify` 14.8s 里有一部分是轮询粒度损失（平均 0.5s/号）。改成 300ms 可省
约 0.3~0.5s/号，代价是对 CF Worker 的请求量 ×3。默认保持 1s，做成参数。

### P2-2 台账 / 交付物写入合并

每个账号当前写 3 次台账（`Append` 各带 `fsync`）+ 1 次交付物。
高并发下是明显的 IO 放大。改成「批量追加 + 每批 fsync 一次」。

**验收**：50 账号批次的 `Sync` 调用次数从 ~150 降到 ~5。

### P2-3 把速率做进控制台

`Timings` 已经在台账里，只差聚合展示。在控制台「注册」页加：本批单号平均
E2E、阶段耗时条形图、失败阶段分布。这样每次改动都能看到回退。

---

## 7. 预期收益汇总

| 改动 | 单号 E2E | 50 号墙钟（并发 5） | 吞吐 |
| --- | ---: | ---: | ---: |
| 现状 | 27.0s | 279s | 10.75 号/分 |
| + P0-2 | 23.7s | 245s | 12.2 号/分 |
| + P0-1 | 21.7s | 225s | 13.3 号/分 |
| + P0-3 | **15.5s** | **160s** | **18.8 号/分** |
| + P1-1（并发 10） | 15.5s | **80s** | **37.5 号/分** |

即：不动并发，仅改代码就能把吞吐提高 **75%**；再把并发提到 10，可达 **3.5×**。

## 8. 实施结果（P0 / P1 / P2 全部落地）

| 项 | 状态 | 验证方式 |
| --- | --- | --- |
| P0-1 入池免二次探测 | ✅ | `enroll` 阶段 2.0s → **0.00s**；本批 10/10 入池，0 失败 |
| P0-2 新邮箱跳过预检 | ✅ | `exists` 标记为 `skipped (fresh)`，10/10 |
| P0-3 验证等待并行 | ✅ | `validate` 移入并行窗口；同批 `verify` 与 `validate` 时间区间重叠 |
| P1-1 并发不超过出口 | ✅ | `CapConcurrency` 单测 4 例；CLI / 服务端都生效 |
| P1-2 验活退避重试 | ✅ | `IsTransient` 16 例、`theosWithRetry` 3 例 |
| P2-1 自适应轮询 | ✅ | 起始 300ms、×1.5 退避到配置间隔；单测覆盖 |
| P2-2 台账批量 fsync | ✅ | 20 行 / 1s 批量；`Ledger.Flush/Close` 由 CLI 与服务端任务调用 |
| P2-3 控制台速率 | ✅ | 注册页顶部显示单号均值 / p50 / p95 / 阶段耗时 |

### 实测对比（优化前 50 号 @并发 5 vs 优化后 50 号 @并发 10）

| 指标 | 优化前（50 号 / 并发 5） | 优化后（50 号 / 并发 10） |
| --- | ---: | ---: |
| 入池 | 46/50（补跑后 49） | **50/50** |
| 失败账号 | 4 | **0** |
| 墙钟 | 279s | **233s** |
| 吞吐 | 10.75 号/分 | **12.88 号/分（+20%）** |
| 单号 E2E 均值 | 27.0s | 31.4s（去离群 28.0s） |
| `exists` | 3.3s | **0（`skipped (fresh)`）** |
| `enroll` | 2.0s | **0.00s** |
| `verify` | 14.8s | 20.3s |
| `validate` | 3.1s | 4.3s |
| 并发利用率 | 4.83 / 5 = 97% | 6.73 / 10 = 67% |

**怎么读这组数**：

- **确定的收益**：5.3s 的可去除开销清零（`exists` 3.3 + `enroll` 2.0），
  入池失败率从 8% 降到 **0**，吞吐 +20%（并发 5 → 10）。
- **单号 E2E 没降**（31.4s vs 27.0s）：因为 `verify`（外部邮件投递）本轮
  慢了 5.5s，把省下的 5.3s 抵消了；去掉 1 个 198s 的投递离群后是 28.0s。
  并发从 5 提到 10 也会抬高单号耗时（出口/上游争用，`validate` 3.1s → 4.3s）。
- **利用率 67%** 说明 10 并发还没打满：理论上限 `10 × 60 / 31.4 ≈ 19 号/分`，
  实际 12.88，差距来自「5 波 × 部分尾波」和 `MinInterval` 拉开的启动间隔。
  想再提速应该加出口（现在 12 条）而不是继续加并发。

结论：**可去除开销已清零、失败率已归零、单号耗时已完全由邮件投递决定**。
下一步只能从邮件获取方式上要收益（例如把轮询换成推送），而不是继续在网关侧优化。

## 10. P0-4：把 verify 移出关键路径（已实施）

第五批数据把问题指得很清楚：

| 批次 | `verify` | 其余阶段合计 | E2E | 吞吐 |
| --- | ---: | ---: | ---: | ---: |
| 第三批 | 14.8s | 12.2s | 27.0s | 10.75 |
| 第四批 | 20.3s | 11.1s | 31.4s | 12.88 |
| 第五批 | **56.1s** | **11.1s** | 67.2s | 7.73 |

- 网关侧（`register` / `login` / `session` / `validate` / `enroll`）三批几乎不变，
  合计 **11~12s**；
- `verify` 在 14.8s → 56.1s 之间剧烈波动，是唯一的不确定性来源；
- 而且它**不影响额度**：第五批 14 个验证超时账号最终全部拿到 200（延迟复探补齐）。

### 方案

入池不等待验证，验证在后台继续：

```text
现在:  mailbox → register → login → session → validate ─┐
                              verify (阻塞 join) ────────┴→ enroll
改后:  mailbox → register → login → session → validate → enroll
                              verify (后台) ────────────────→ 成功后回填
```

- `enroll` 携带 `email_verified=false`（当前上游不禁用）；
- 后台验证成功后，通过管理接口回填该账号的 `email_verified=true`；
- 需要一个新的后台任务池 ＋ 一条 `action=mark-verified`；
- 延迟等待窗口可以缩短到 45s（晚到的邮件不再需要等，因为额度不依赖它）。

### 实测结果（已实施）

| 指标 | 第五批（阻塞验证） | 第六批（P0-4） | 预测 |
| --- | ---: | ---: | ---: |
| 单号 E2E 中位 | 52.5s | **12.3s** | ~11s |
| 单号 E2E 均值（成功账号） | 67.2s | **12.6s** | — |
| p90 / 最小 | 107.6s / 36.4s | 15.6s / **9.1s** | — |
| 后台验证回填 | — | **33/34** | — |
| 推算吞吐（并发 10） | 7.73 号/分 | **~49 号/分** | ~45 |

20 号确认批次（并发 10）复核：19/20 成功，中位 E2E **11.9s**，
后台验证 18/19 回填成功 —— 与第一批一致。

实现要点：

- `Enroller` 新增可选接口 `VerifiedMarker`（`MarkVerified(ctx, accountID, email)`）；
- 入池后 `verifyBackfill` 在**脱离调用方 ctx** 的后台 goroutine 里继续等邮件，
  成功后点链接、写一条新台账（Merge 把 `verify` 从 `pending` 更新为 `ok`）、
  再调 `MarkVerified` 回填号池；
- 所有后台写入都作用在**记录副本**上，避免与 `finish()`/`notify()` 竞争；
- 唯一仍同步等待的场景：`/theos` 返回 `email_unverified`。
- 管理接口新增 `action=mark-verified`（按 id，缺失时按邮箱回退，幂等）。

### 已修复：注册 POST 超时不重试（第六批暴露）

```text
POST https://api.deepai.org/daily-time-sync/registration/ → 504 Gateway Timeout
```

三处改动：

1. **45s 总预算**：`registerPostBudget` 包住注册 POST，失败快速返回
   （原来 3 次内部重试 × 60s 客户端超时 ≈ 190s 才判死）；
2. **退避可被 ctx 打断**：`do()` 里两处 `time.Sleep` 换成 `sleepCtx`，
   否则 ctx 已经超时还要把 700ms/1400ms 睡完，预算形同虚设；
3. **登录兜底**：注册 POST 失败（非「已注册」）时用同一邮箱密码登录一次 ——
   504 往往意味着账号**其实已建成**，登录成功即继续，天然幂等。

第七批实测（并发 10）：**50/50 入池、0 失败、墙钟 82s**，
单号 E2E 中位 **11.7s**，吞吐 **36.6 号/分**（预测 ~45，差在 76% 利用率）。

### 已修复：CLI 退出会杀掉后台验证

后台验证跑在 CLI 进程里，批次结束即被带走，导致号池里的 `email_verified`
不准（第六批只有 11/50 回填）。新增：

- `-verify-grace`（默认 60s）：批次结束后最多等这么久；
- `Pipeline.PendingVerify()` / `WaitVerify(d)`：`WaitGroup.Add` 放在起
  goroutine **之前**，避免与 `Wait` 竞争；
- 等待结束后从台账重新读一遍记录，汇总与交付物才反映真实验证状态。

验证：5 号小批 `等待 5 个后台邮箱验证（最多 1m0s）…` → `邮箱已验证 5`。

### 历史遗留：注册 POST 没有重试

第六批 16/50、确认批次 1/20 卡在同一处：

```text
POST https://api.deepai.org/daily-time-sync/registration/ → 504 Gateway Timeout
```

- 单个账号要白等 188~219s（客户端超时 + 内部重试），把墙钟从 ~60s 拖到 430s；
- 该调用目前**没有任何重试**，且失败即判死（`register` 阶段 failed）；
- 修法：对瞬时错误退避重试一次（1~3s）；若上游返回「已注册」，
  走已有的 `isAlreadyRegistered` 分支转登录，天然幂等。

预计收益：失败率 32% → <5%，且批次墙钟不再被单个超时账号拖长。

风险与缓解：

- 若上游将来把「邮箱未验证」作为额度或推理前置条件，这条改动会失效
  —— 用`email_verified` 的比例做告警即可提前发现；
- 后台验证需要幂等：同号重复点链接无副作用，回填按邮箱去重。

## 11. 验证点击的上游成本（HAR 实测）

`deepai.org-2.har`（17:07 本地，浏览器手动验证一次）给出端点画像：

| 指标 | 值 |
| --- | --- |
| 请求 | `GET https://api.deepai.org/verify_email?key=<uuid>` |
| 响应 | `200 text/html`，server `nginx + Phusion Passenger` |
| `wait`（服务端处理） | **13.2 s** |
| ssl/connect / receive | 0.37 s / 0.33 s |
| 合计 | **13.9 s** |

同文件里 `/daily-time-sync/user/` 的字段只有
`pk / username / email / userprofile.api_key / userprofile2.plan / …`，
**没有 email-verified 字段** —— 上游不提供可轮询的验证状态。

结论：这个端点「能用但很贵」，并发点必然触发 Passenger worker 超时（502
`Incomplete response received from application`）。因此必须限流（当前：全局 2 并发，
退避 2s/10s/60s），并且**不要把它放进关键路径**（P0-4 已经移出）。

补齐能力：`register -reverify [-group <分组>]`，从收件箱重取链接 → 限流点击 →
回填 `email_verified`。`stress-50i` / `stress-50j` / `stress-50k` 均实测补齐到 100%。

**边界**：临时邮箱里的验证邮件只保留很短时间。批次结束几分钟内补还行，
几小时后收件箱已空 —— 第七批 39 个未验证账号事后补验证全部超时（25s 内无邮件）。
所以正确做法是**批次内完成验证**（限流就是为此），而不是事后补救。

## 12. 批次稳定性观察（第 6–10 批）

> 本节只作为**回归基线**：口径与读数方法见 `docs/lessons.md` §4。

| 批次 | 并发 | 墙钟 | 入池 | E2E 中位 | p90 | 备注 |
| --- | ---: | ---: | ---: | ---: | ---: | --- |
| 6 | 10 | 430s | 34/50 | 12.3s | 15.6s | 注册 POST 504，无重试 |
| 7 | 10 | 82s | 50/50 | 11.7s | 19.1s | 修复后基线 |
| 8 | 10 | 228s | 50/50 | **11.9s** | 89.2s | 上游整体变慢（verify 端点故障） |
| 9 | 10 | 126s | 50/50 | **8.9s** | 13.4s | 验证限流后，验证/额度均 50/50 |
| 10 | 10 | 124s | 50/50 | **10.3s** | 13.8s | 配置未变，稳定复现 |

结论：

- **单号中位 E2E 稳定在 8.9~12.3s** —— 关键路径已经与上游波动解耦；
  第九批（验证限流 + 上游正常）做到 8.9s / 45.5 号/分，是当前的口径上限；
- 上游变慢时，`register` / `login` / `session` / `validate` 会同步变慢
  （第八批分别 6.6 / 4.2 / 3.0 / 6.7s，是第七批的 2~5 倍），表现为 p90 拉长；
- 验证是唯一会被上游端点故障整体打断的环节（第八批 0/50），但它不在关键路径、
  也不影响额度，属于可接受的降级。

## 13. 不建议做的事

- **不要为提速去掉 `validate`**：它决定了账号能不能用，省 3.1s 换来一堆
  0 额度账号，等于把成本推到运行时。
- **不要把 `enroll` 改成批量接口**：入池是状态文件的唯一写入口，
  批量会放大回滚难度；`enroll` 真实成本只有 2.0s，且 P0-1 后更低。
- **不要靠提高并发掩盖单号耗时**：出口只有 12 条，并发 20 以上会开始排队，
  表现是 p95 暴涨而不是吞吐提升。

## 14. 单账号在途上限对照压测（2026-11-09）

**问题**：号池并发上限 = `账号数 × maxInFlightPerAccount`，后者是编译期常量
（`internal/server/common.go:132`，默认 **2**）。14 个号 = 28 个槽位，但**客户端
并发 28 时成功率只有 48~70%**：槽位不够时网关不会排队，而是换号重试，重试预算
（`max_retries`，默认 3）用尽就秒回 `429 rate_limit_error`（`所有账号并发已满，请稍后重试`）。

这轮只做测量，用来决定「常量 2 → 3」值不值，以及「加号 / 调设置 / 改代码」哪条路更划算。

### 方法与口径

- 池子 **14 个号**（`/health`：`accounts=14 ready=14`），模型 `glm-5.3-flash`
  （`theosPerRequest=0`，不消耗 Theos），`stream=false`、`max_tokens=16`、带一条 system；
- 客户端固定并发 `C`、总请求 `N`（约 2 个请求/并发位，保证能跑满两波）；
- 统计口径：`ok` = 200；`429` 单独计，且只统计 `error.type=rate_limit_error`
  —— 它**是网关自己的背压**，不是上游 `409 session_busy`（那条会内部等待重试，
  只表现为延迟变长）；
- `cap=3` 变体：临时把常量改成 3、编成独立二进制，**立刻 `git checkout` 还原**，
  仓库不留改动；两轮**交替**跑（cap2→cap3，再 cap3→cap2），抵消上游时段漂移；
- 每轮之间重启网关（换二进制 + 复位轮询游标），状态文件用副本，真实号池不动。

### 14.1 cap = 2（现状：28 槽位）

| 客户端并发 | 请求 | 成功 | 429(容量) | 墙钟 | 成功率 | P50 | P95 |
| ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| 14 | 42 | 42 | 0 | 7.0s | **100%** | 1.18s | 3.97s |
| 20 | 40 | 40 | 0 | 5.1s | **100%** | 2.34s | 4.29s |
| 24 | 48 | 42 | 6 | 5.5s | 87.5% | 2.15s | 4.49s |
| 28 | 56 | 39 | 17 | 6.5s | 70% | 2.65s | 5.50s |
| 28 | 56 | 27 | 29 | 19.5s | 48% | 0.36s | 5.37s |

> 最后两行是同配置的两轮，差别来自上游时段（第二轮墙钟 19.5s，明显更慢）；
> 两轮都出现了成规模拒绝，说明饱和不是偶发。

### 14.2 cap = 3（42 槽位）

| 客户端并发 | 请求 | 成功 | 429 | 墙钟 | 成功率 | P50 | P95 |
| ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| 28 | 56 | **56** | 0 | 7.7s | **100%** | 2.09s | 5.65s |
| 28 | 56 | **56** | 0 | 11.7s | **100%** | 2.70s | 9.42s |

### 14.3 只改设置：`max_retries 3 → 16`（不动代码）

| 客户端并发 | 请求 | 成功 | 429 | 墙钟 | 成功率 | P50 |
| ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| 28 | 56 | 26 | 30 | 6.0s | 46% | **30ms** |
| 28 | 56 | 26 | 30 | 4.7s | 46% | 28ms |

> ⚠ 上表是**改造前**的基线。2026-11-09 之后调度侧已改为「扫一圈号池 + 有界排队
> 2s」而不是「贪心 3 次就 429」，409 也会立即换号 —— 用本节脚本复测时 429 应基本
> 消失；成功率的提升主要来自这两项改动，而不是把常量从 2 调到 3。

### 14.4 结论

1. **cap 2 的有效并发不是 28，而是 ~20**（≈1.4 条/号）：24 开始漏，28 只剩 48~70%；
2. **cap 3 在 28 并发下零拒绝**（两轮 56/56），P95 与 cap 2 同量级——涨的是容量，不是延迟；
3. **调 `max_retries` 没用**（46%，且 429 的 P50 只有 30ms）：429 是槽位不够，
   被拒的请求在几十毫秒内就把重试预算烧完。这是纯容量问题，不是重试策略问题；
4. 28 并发下 cap2 打不满 28 个理论槽位，代码路径上有两个放大器：
   - `acquireAccount` 失败也算一次重试预算（`common.go:227` 的 `continue`），
     饱和时贪心选号只试 `max_retries` 个号就放弃；
   - 槽位在 handler 返回时才归还（`releaseBody`），客户端拿到响应立刻发下一个请求时，
     会撞上「响应已收到、槽位未归位」的窗口。
   所以对客户端的要求是：**遇到 `429 rate_limit_error` 立即退避**，重试风暴只会放大拒绝。

按目标并发选杠杆：

| 目标并发 | 做法 |
| --- | --- |
| ≤ 20 | 不用改（cap 2 已足够） |
| 24 ~ 28 | 常量 `maxInFlightPerAccount 2 → 3`（需重编），实测零拒绝 |
| > 28 | **加号**（每号 +2 槽位）比继续抬常量稳：上游单号容量（`409 session_busy`）才是硬约束 |

### 14.5 复现

两个脚本，`scripts/` 下可直接跑（需先按 §1 的方式把网关跑起来，真实号池建议用副本）。

```js
// scripts/ab-load.mjs — 固定并发压测，输出一行 JSON
// 用法: BASE=http://127.0.0.1:10087 KEY=sk-... CONC=28 TOTAL=56 node scripts/ab-load.mjs
const BASE = process.env.BASE || 'http://127.0.0.1:10087';
const KEY = process.env.KEY;
const CONC = Number(process.env.CONC || 28);
const TOTAL = Number(process.env.TOTAL || 56);
const MODEL = process.env.MODEL || 'glm-5.3-flash';

const body = JSON.stringify({
  model: MODEL, stream: false, max_tokens: 16, temperature: 0,
  messages: [
    { role: 'system', content: 'You are a helpful AI assistant.' },
    { role: 'user', content: 'Reply with exactly: OK' },
  ],
});

async function one() {
  const t0 = performance.now();
  try {
    const r = await fetch(BASE + '/v1/chat/completions', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', Authorization: 'Bearer ' + KEY },
      body, signal: AbortSignal.timeout(180000),
    });
    const text = await r.text();
    let j = null; try { j = JSON.parse(text); } catch {}
    return { status: r.status, ms: performance.now() - t0, type: j?.error?.type || '' };
  } catch {
    return { status: 0, ms: performance.now() - t0, type: 'client_error' };
  }
}

const t0 = performance.now();
let next = 0;
const out = [];
const workers = [];
for (let w = 0; w < Math.min(CONC, TOTAL); w++) {
  workers.push((async () => {
    for (;;) {
      const i = next++;
      if (i >= TOTAL) return;
      out.push(await one());
    }
  })());
}
await Promise.all(workers);
const wall = performance.now() - t0;

const ms = out.map(r => r.ms).sort((a, b) => a - b);
const q = p => ms.length ? Math.round(ms[Math.min(ms.length - 1, Math.floor(ms.length * p))]) : 0;
const byStatus = {}, byType = {};
for (const r of out) {
  byStatus[r.status] = (byStatus[r.status] || 0) + 1;
  if (r.status !== 200) byType[`${r.status} ${r.type || '-'}`] = (byType[`${r.status} ${r.type || '-'}`] || 0) + 1;
}
console.log(JSON.stringify({
  conc: CONC, total: TOTAL, wall_s: +(wall / 1000).toFixed(1),
  rps: +(out.length / (wall / 1000)).toFixed(2), ok: byStatus[200] || 0,
  byStatus, p50: q(0.5), p95: q(0.95), p99: q(0.99), errors: byType,
}));
```

```bash
#!/usr/bin/env bash
# scripts/ab-arm.sh — 起网关 → 建临时 Key → 压测 → 停网关（一轮）
# 用法: CONC=28 TOTAL=56 ./scripts/ab-arm.sh ./bin/deepaicode2api
set -euo pipefail
BIN=${1:?用法: ab-arm.sh <binary>}
WORK=$(mktemp -d)
cp deepaicode2api-data.json "$WORK/state.json"    # 副本：不动真实号池
"$BIN" -data "$WORK/state.json" -port 10087 >"$WORK/gw.log" 2>&1 &
GW=$!
trap 'kill $GW 2>/dev/null || true; rm -rf "$WORK"' EXIT
until curl -sf -m1 -o /dev/null http://127.0.0.1:10087/healthz; do sleep 0.5; done
KEY=$(curl -sS -X POST -H 'Content-Type: application/json' -d '{"name":"ab","group":""}' \
  http://127.0.0.1:10087/admin/api/keys | sed -n 's/.*"key":"\([^"]*\)".*/\1/p')
# 只测 max_retries 时，在这里插一句（不需要重编）：
# curl -sS -X POST -H 'Content-Type: application/json' -d '{"max_retries":16}' \
#   http://127.0.0.1:10087/admin/api/settings >/dev/null
BASE=http://127.0.0.1:10087 KEY="$KEY" CONC=${CONC:-28} TOTAL=${TOTAL:-56} node scripts/ab-load.mjs
```

```bash
# cap=3 变体：临时改常量 → 编独立二进制 → 立刻还原（仓库不留改动）
python - <<'PY'
p='internal/server/common.go'
s=open(p, encoding='utf-8').read()
open(p,'w',encoding='utf-8',newline='').write(s.replace(
    'const maxInFlightPerAccount = 2', 'const maxInFlightPerAccount = 3'))
PY
go build -o /tmp/gw-cap3 . && git checkout -- internal/server/common.go
go build -o /tmp/gw-cap2 .
git status --short        # 应为空
```

复跑时按「cap2 → cap3 → cap3 → cap2」交替，每轮 `CONC=28 TOTAL=56`，取成功率与 P95 对照。
