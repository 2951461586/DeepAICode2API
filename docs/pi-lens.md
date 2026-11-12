# 本项目的 pi-lens 配置

仓库根目录的 `.pi-lens.json` 是本项目的 **pi-lens**（编码期诊断工具）配置。
它不参与编译、也不进二进制，只影响人与 AI 在本仓里写代码时看到的诊断噪音。

pi-lens 的配置分两层：**项目级** `.pi-lens.json`（本文件所在仓库，可提交、可 review）
与**机器级** `~/.pi-lens/config.json`（每个人自己的）。本仓只提供项目级，
不替使用者决定机器上装什么工具。

## 为什么需要它

此前本仓没有任何 pi-lens 配置，于是它按默认值跑：

| 现象 | 根因 | 处理 |
| --- | --- | --- |
| 每个会话几十条 `spawn failure classified: tool-not-found` | `typos` / `vale` / `markdownlint` / `yamllint` / `bash-language-server` / `vscode-css|html-language-server` / `docker-langserver` / `yaml-language-server` 都没装（本仓刻意零工具链） | 关掉对应 runner / server（见下） |
| 中文长文被拼写/散文检查反复扫 | `README.md`（47KB）与 `docs/*.md`（300KB+）是中文技术长文 | 关掉 `typos` / `vale` / `markdownlint` |
| 合法写法被报 | `waitgroup-done-scope` 把 `ctx.Done()` 当成 `WaitGroup.Done()`；`nil-map-assignment` 看不到 `json.Unmarshal(&m)` 会分配；Semgrep XSS 规则针对 HTML，而网关写的是 JSON / SSE | 关掉前两条规则；第三条改成走统一的 `writeRawJSON`（顺带消掉重复的响应头拼装） |
| 复杂度告警长期挂着 | `openModel` 是已知的调度状态机（cc 39），拆分需要单独设计 | 阈值 15 → 25，并对该文件单独豁免（其余新代码仍受 25 约束） |

## 配置在做什么

### 扫描范围（`ignore`）

运行期状态与构建产物不进索引：`dist/`、`bin/`、`exports/`、`coverage.out`、
`*.out`、`.env*`、`deepaicode2api-data*.json*`、`*.jsonl`、`*.bak`。
其中状态文件与台账**含上游 Key / 密码**，本就不该被任何工具读进索引。

### 关掉的 LSP / runner

- `lsp.disabledServers`：`bash`、`css`、`docker`、`html`、`python`、`typos`、`yaml`
  —— 这些语言的 server 在本机/本仓均未安装，启用只会每会话刷 `tool-not-found`。
  **代价**：这些文件会显示为 *unconfirmed*（无法确认干净），而不是 *clean*。
- `dispatch.disabledRunners`：`vale`、`markdownlint`、`yamllint`
  —— 中文文档与 YAML 不在本仓质量门禁里（YAML 只有 CI 与 compose 各一份）。

### 保留的检查

Go：`go-vet`、go LSP、tree-sitter、ast-grep、opengrep（Semgrep）、`fact-rules`。
前端（零构建 ES module）：TypeScript/Deno LSP、`oxlint`、`biome-check-json`、ast-grep。

### 规则策略（`rules`）

| 规则 | 策略 | 原因 |
| --- | --- | --- |
| `high-complexity` | 阈值 25；`internal/server/upstream.go` 豁免 | 见上表；新代码仍受 25 约束 |
| `high-fan-out` | 阈值 30 | 默认 20 对「路由 + 装配」风格的 handler 偏严 |
| `nil-map-assignment` | disable | 规则不追踪 `&m` 的写入，`var m map[…]` + `json.Unmarshal` 是合法写法 |
| `waitgroup-done-scope` | disable | 规则把 `ctx.Done()` 也当成 `WaitGroup.Done()` |
| `no-console-except-error-js` | `ignorePaths: js/api.js` | 该处 `console.warn` 是刻意保留的英文界面排障输出 |
| `no-array-reverse-mutation-js` | `ignorePaths: render/logs.js` | 实际写法是 `list.slice().reverse()`，规则看不穿 `.slice()` |

真实的写法问题**不靠豁免**，而是直接改代码：本轮据此把 `isFinite` → `Number.isFinite`、
`!!m.free` → `Boolean(m.free)`，并把上游错误体统一交给 `writeRawJSON`。

## 怎么验证配置生效

```bash
# 在 pi 里：
#   effective_config path=internal/server/upstream.go
# 期望：documents 含项目 .pi-lens.json、provenanceCounts.project > 0、
#       recordCounts 为空（有值说明某条目 id 写错了，日志会给出 PILENS_CFG_0005）、
#       被关掉的 server 显示 disabled-by-config 并指明 key 与 tier。

#   lens_diagnostics path=<目录> scope=paths mode=full source=lsp
# 期望：被豁免的条目不再出现；输出末尾会写明
#       "suppressed by disposition: N finding(s) …"（不是"干净"，是"已按策略隐藏"）。
```

改配置后不需要重启：`rules` 与 denial 列表按文件解析，下一次派发即生效。

### 状态栏的 `●1E !20W` 是什么

那个徽章读的是**会话缓存**（本次会话里各文件的累计发现），不是「当前代码有 1 个
错误」。编辑过的文件里，旧行号上的条目会一直挂在缓存里，直到重新扫一次。真实判定
必须主动探一次：

```text
lens_diagnostics mode=full source=lsp scope=paths paths=[<改过的文件…>]
```

判据是 `Outcomes` 里的 `clean=N` 与 `findings=N`，而不是徽章数字。2026-11-12 实测：
徽章 `●1E !20W`，对该次发布的 14 个文件做 `mode=full` → `1 warning`（
`console-smoke.mjs` 里一处 `!!on`，已改 `Boolean(on)`），其余 clean；再探一轮 → `0`。
剩下的是测试代码的**质量提示**（`time.Sleep` / `context.Background()` /
`_ = Encode(...)` / `s, _ := newTestServer(...)`），工具把测试当 handler 误判，
不值得为它们引入封装。

## 什么时候该把这些打开

- 装上 `typos` / `vale` / `markdownlint` / `yamllint` / `shfmt` 之后：删掉
  `dispatch.disabledRunners` 里对应条目；
- 想给控制台的 CSS / HTML 做 LSP 诊断：`npm i -g vscode-langservers-extracted`
  （提供 css/html/json/eslint server），再从 `lsp.disabledServers` 移除 `css` / `html`；
- 装了 `golangci-lint` 之后：本仓还需一份 `.golangci.yml` 才会被 pi-lens 选中
  （它要求「有项目配置」才启用，避免用默认规则集扫出与项目约定不符的问题）。
