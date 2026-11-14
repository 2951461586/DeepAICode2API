# 号池模型指纹实测（2026-11-10）

用 [LM Fingerpoint Detector](https://lm.ikale.io)（`lmfpd` CLI）逐个测号池里 22 个模型
**实际被路由到哪个模型**。检测器让模型凭第一反应写 ~300 个 1–355 的整数，
再用这些数字的分布与参考库比对。

> **目录已变（2026-11-14）**：上游模型目录从 22 涨到 27（新增 `muse-ember` /
> `kimi-k3.1-pro` / `gpt-6.1-terra` / `gemini-4-hydrogen` / `gemini-4-carbon` /
> `claude-sonnet-5-5`）。本文是**历史快照**：那 6 个新模型没有测过，
> 已有结论的**判据仍适用**（挑模型看真实路由，不看 `provider` 字段）。
>
> **结论的性质**：检测器给出的是「参考库内的封闭集合排序」，
> 每次结果都带 `evidence.insufficient=true`。**它是参考，不是身份证明**。
> 判断渠道可信度要固定参数、多轮重复、结合其它证据。

## 方法

```bash
# 网关本地回环免密钥（keys=0 时）
npx lmfpd@latest -b http://127.0.0.1:10087/v1 -k sk-local \
  -m <model> -a chatcompletion --count 3 -p 1 --timeout 180 --json
```

两条必须记住的经验：

1. **`-p 1`（顺序采样）是硬要求**。默认 `-p 3` 并发时，3 条样本里有 2 条返回空文本
   （`used_outputs=1`、`probability_status=unavailable`），结果全部作废。
   改成 `-p 1` 后同样 3 条样本全部有效（同成本、只多花时间）。
2. **`gpt-6-astra` 要 `-a responses`**，其余用 `-a chatcompletion`。

## 结果

| 模型 | 计费 | 判定为 | 置信度 | 结论 |
| --- | --- | --- | ---: | --- |
| `glm-5.3-flash` | 免费 | glm-5.3-flashx | 0.747 | ✅ 同族变体 |
| `deepseek-v4.1-flash` | 免费 | deepseek-v4.1-flash-1017 | 0.980 | ✅ |
| `qwen-3.8-flash` | 免费 | qwen3.8-flash | 0.912 | ✅ |
| `minimax-m3.1-flash` | 免费 | grok-4.5 | 0.305 | ❌ 不符 |
| `solar-mini-4` | 免费 | qwen3.8-omni-flash | 0.177 | ❌ 不符 |
| `gpt-6-luna` | 免费 | qwen3.8-omni-flash | 0.323 | ❌ 不符 |
| `glyph-cluster` | 免费 | kimi-k3.1-pro | — | ⚠️ 仅 1/3 样本有效 |
| `mimo-2.6-flash` | 免费 | — | — | 🚫 超时（3 次均 300s） |
| `mimo-2.6-pro` | 免费 | — | — | 🚫 超时 |
| `crouton-alpha` | 免费 | — | — | 🚫 3/3 空响应 |
| `deepai-cyber` | 免费 | — | — | 🚫 3/3 空响应 |
| `glm-5.3` | 3 | glm-5.3 | 0.999 | ✅ |
| `kimi-k3` | 3 | kimi-k3 | 0.550 | ✅ |
| `muse-spark-1.3` | 6 | muse-spark-1.3 | 0.984 | ✅ |
| `claude-opus-5-5` | 15 | claude-opus-5.5 | 0.999 | ✅ |
| `muse-wonder` | 15 | muse-wonder | 1.000 | ✅ |
| `claude-fable-5-2` | 20 | claude-fable-5.2 | — | ⚠️ 仅 2/3 样本有效 |
| `minimax-m3` | 2 | qwen3.8-max-0902 | 0.651 | ❌ 不符 |
| `gpt-6.1-sol` | 10 | mimo-v2.5-pro | 0.523 | ❌ 不符 |
| `gemini-4-argon` | 10 | qwen3.8-omni-flash | 0.562 | ❌ 不符 |
| `qwen-3.8-max-prime` | 15 | — | — | 🚫 超时 → **2026-11-11 补测见文末** |
| `gpt-6-astra` | 50 | — | — | 🚫 3/3 空响应（responses 端点）→ **2026-11-11 补测见文末** |

统计：**8 个对得上**、**6 个指纹不符**、**2 个样本不足**、**6 个拿不到有效输出**。

> 2026-11-11 补测：`qwen-3.8-max-prime` 与 `gpt-6-astra` 都能正常出结果，但按 §3.1 判据落在**替身分支**（`gen_` 前缀 + 无 `provider` + cost=0），见文末。

## 号池侧的旁证

- 网关日志里 **75 次** `[upstream] 请求体被拒，按桌面端形状重试`
  —— 上游对请求体形状仍在挑剔，网关走 `ChatBodyFallback` 重试路径；
  空响应大概率来自这条重试后上游仍返回 200-无内容。
- 单次检测会话统计：`total_requests=122`、`failed_requests=50`、
  `total_cost=0.2764`（上游计费口径）；一个账号额度从 200 降到 119.35。
- 只有 **2 个账号就绪**，付费档的并发/余量都紧张；这也是 `-p 3` 更容易翻车的原因之一。

## 复现

```bash
# 1) 起网关（本机）
go build -o bin/deepaicode2api.exe .
./bin/deepaicode2api.exe -no-import

# 2) 取实时目录
curl -s http://127.0.0.1:10087/v1/models | python -c "import json,sys;print('\n'.join(m['id'] for m in json.load(sys.stdin)['data']))"

# 3) 逐个测（务必 -p 1）
for m in $(cat models.txt); do
  npx -y lmfpd@latest -b http://127.0.0.1:10087/v1 -k sk-local \
    -m "$m" -a chatcompletion --count 3 -p 1 --json > "out/$m.json"
done
```

## 注意

- 目录以**线上 `/v1/models`** 为准（22 个）。仓库里若残留旧的
  `deepaicode2api-data.json`，其 `models` 字段可能落后于上游（曾见 18 个），
  不要拿它当目录依据。
- 本文件是 2026-11-10 的一次快照。上游换后端、改路由后结论会变，届时重新跑一遍。

---

## 补测：`qwen-3.8-max-prime` 与 `gpt-6-astra`（2026-11-11）

上面两个"拿不到有效输出"的模型重测了一遍。这次不用外部检测器，直接看上游原始
响应（判据见 `recon.md` §3.1），并**同账号同代理跑免费模型做对照**，排除账号/代理
因素。每个样本用不同号池账号，避免单号配额或缓存干扰。

| 模型 | 端点 | 样本 | `id` | `model` 回显 | `provider` | `system_fingerprint` | `cost_details` | 输出 |
| --- | --- | ---: | --- | --- | --- | --- | --- | --- |
| `qwen-3.8-max-prime` | chat | 4 | `gen_<ULID>` | `alibaba/qwen3.8-max-prime` | **缺失** | `fp_…`（逐次不同） | 全 0 | ✅ 正确 |
| `gpt-6-astra` | responses | 4 | `gen_<ULID>` | `openai/gpt-6-astra` | **缺失** | **缺失** | 无该字段 | ✅ 正确 |
| `glm-5.3-flash`（对照） | chat | 2 | `gen-<epoch>-<rand>` | `z-ai/glm-5.3-flash` | `InferenceNet` / `Friendli` | 缺失 | 7e-6 / 3.2e-5 美元 | ✅ 正确 |

### 结论

1. **两个模型都能正常出结果**：`qwen-3.8-max-prime` 4/4、`gpt-6-astra` 4/4 返回 200，
   内容符合指令（"fp-sample-N"）。此前记录的"超时 / 3/3 空响应"是**检测器侧**的问题
   （并发 3 采样、responses 端点未支持），不是上游不可用。
2. **按 §3.1 判据，两者落在替身分支**：`id` 是 ULID 前缀 `gen_`、`provider` 缺失、
   `cost_details.upstream_inference_*` 全 0。同账号下 `glm-5.3-flash` 是干净的
   `gen-<epoch>-<rand>` + `provider=…` + cost>0 —— 差异是**模型级路由**，不是账号或代理异常。
3. **替身现在会回显真实 vendor slug**（`alibaba/…`、`openai/…`），不再是
   `reflection/beam`。所以 §3.1 里「`model` 不回显请求 slug → 替身」这条**辅助**判据
   对这两个模型失效；可靠判据只剩「`provider` 缺失 + `id` 用 `gen_` 前缀 + cost 为 0」。
4. **计费与目录价不是一回事**：目录价 15 / 50 Theos，实测同一号连续样本后 Theos 余额
   只降了 **0.03–0.07/次**（`qwen` 与 `astra` 同量级）；对照组的
   `upstream_inference_cost` 也只有 1e-5 美元级。也就是说 `theosPerRequest` 更像
   **价签/上限**，不是逐次硬扣；拿它做号池成本估算会高估两个数量级。

### `/responses` 的请求体形状（新发现）

`gpt-6-astra` 走 `/deepai_code/responses` 时，上游同样有"必须是 DeepAI Code 发的形状"
校验，不合规直接 400 `This request is not one DeepAI Code sends.`。可用的最小形状
（与 pi 的 `openai-responses.js` 一致）：

```json
{
  "model": "gpt-6-astra",
  "stream": false,
  "store": false,
  "prompt_cache_key": "<32 hex>",
  "input": [
    {"role": "system", "content": "You are a helpful AI assistant. …"},
    {"role": "user", "content": [{"type": "input_text", "text": "…"}]}
  ]
}
```

要点：**system 讲稿要放在 `input[0]`**（不是顶层 `instructions`），user 的 content 必须
是 `[{type:"input_text",text}]` 数组，并且带 `store:false` 与 `prompt_cache_key`。
只发 `{"model":…,"input":[{"role":"user",…}]}` 会被上面那条 400 拒掉。
