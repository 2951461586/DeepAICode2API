# 号池模型指纹实测（2026-11-10）

用 [LM Fingerpoint Detector](https://lm.ikale.io)（`lmfpd` CLI）逐个测号池里 22 个模型
**实际被路由到哪个模型**。检测器让模型凭第一反应写 ~300 个 1–355 的整数，
再用这些数字的分布与参考库比对。

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
| `qwen-3.8-max-prime` | 15 | — | — | 🚫 超时 |
| `gpt-6-astra` | 50 | — | — | 🚫 3/3 空响应（responses 端点） |

统计：**8 个对得上**、**6 个指纹不符**、**2 个样本不足**、**6 个拿不到有效输出**。

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
