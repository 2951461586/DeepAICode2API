# 免费模型指纹重复性测试（2026-11-27，线上 v0.1.31）

针对 12 个免费模型做了一次**控制变量**的重复性实验，想回答一个问题：

> 同一个模型、同一段提示词、同一协议，重复采样得到的指纹判定是否稳定？

结论：**完全不稳定**。12 个模型里**没有一个**在 3 次重复中给出同一个 top-1。

## 方法

| 项 | 取值 |
| --- | --- |
| 工具 | [LM Fingerpoint Detector](https://lm.ikale.io)，`npx lmfpd@latest` |
| 端点 | `https://deepai-gw.liziai.cloud/v1`，`-a cc`（Chat Completions） |
| 关键控制 | **固定挑战**（`--challenges ch.json`，1 道题、要求 319 个整数）+ `--count 1 -n 3`：三次重复用的是**同一段提示词** |
| 其他参数 | `-e low`（不降推理强度会被 8192 completion 上限吃掉正文）、流式、`--timeout 240` |
| 线上版本 | `v0.1.31`，`proof=random`，车道 8，17 号就绪 |

固定挑战是从一次 `glm-5.3-flash` 的采样里取出来的，其余 11 个模型复用这同一道题。

## 结果（每模型 3 轮，同一道题）

| 模型 | 有效轮次 | 各轮 top-1 | 平均 top1−top2 | 3 轮一致？ |
| --- | ---: | --- | ---: | --- |
| `glm-5.3-flash` | 3/3 | `glm-5.2` / `openpangu-2.0-pro` / `grok-4.6` | 0.21 | 否（3 种） |
| `deepseek-v4.1-flash` | 3/3 | `grok-4.5` / `grok-4.6` / `deepseek-v3.2` | 0.30 | 否（3 种） |
| `mimo-2.6-flash` | 2/3 | `kimi-k3` / `deepseek-v4.1-flash-1017` | 0.30 | 否（2 种） |
| `qwen-3.8-flash` | 3/3 | `swe-2.1` / `gemini-4-carbon` / `mimo-v2.5-pro` | 0.30 | 否（3 种） |
| `minimax-m3.1-flash` | 3/3 | `openpangu-2.0-pro` / `gemini-4-hydrogen` / `gpt-6-sol` | 0.23 | 否（3 种） |
| `muse-ember` | 3/3 | `qwen3.8-max-0902` / `kimi-k3` / `grok-4.5` | 0.07 | 否（3 种） |
| `solar-mini-4` | 3/3 | `qwen3.8-max-0902` / `gpt-6.1-terra` / `gpt-6-astra` | 0.04 | 否（3 种） |
| `gpt-6-luna` | 3/3 | `glm-5.4` / `deepseek-v4-flash-0731` / `claude-fable-5.2` | 0.27 | 否（3 种） |
| `claude-haiku-5-5` | 3/3 | `gpt-6-astra` / `kimi-k3.1-pro` / `gpt-6-sol` | 0.32 | 否（3 种） |
| `mimo-2.6-pro` | 3/3 | `deepseek-v4.1-flash-1017` / `deepseek-v4-flash` / `mimo-v2.5-pro` | 0.13 | 否（3 种） |
| `glyph-cluster` | 2/3 | `deepseek-v4-pro` / `deepseek-v4.1-flash-1017` | 0.32 | 否（2 种） |
| `deepai-cyber` | 3/3 | `seed-2.0-mini` / `gemini-3.7-flash` / `qwen3.8-27b` | 0.16 | 否（3 种） |

汇总：**34 个可评分轮次**，`top1−top2` 均值 **0.22**、中位 **0.18**、最大 0.66；
只有 **10/34** 的轮次分差 ≥ 0.3。两个模型各有一轮样本失败（`mimo-2.6-flash`、
`glyph-cluster`），不影响上面的结论。

## 判读

1. **指纹不能用于识别这个渠道**：同一模型、同一提示词、同一协议、三分钟内重复三次，
   top-1 每次都换，且候选之间分差普遍 < 0.3。任何「这个档位其实是 X 模型」的说法，
   在没有固定挑战 + 多轮一致 + 大分差的情况下都不成立。
2. **上一轮「6 个 flash 模型同指 `gemini-4-carbon`」确认为噪声**：那轮用的是随机挑战、
   每模型只跑 1 轮，本轮的重复实验直接否证了它的可重复性。
3. **不要把这轮结果读成「渠道在乱换模型」**：单次采样不可重复，既可能来自上游温度
   采样，也可能来自多账号路由到不同后端；但**用这个工具分不出来**，所以不做结论。
4. 上游当前很慢（这次采样窗口内 p50 ≈ 9 s、p95 ≈ 105 s），长尾会让样本更容易被截断 ——
   `--timeout 240` 是必要的，`120` 会大量丢样本。

## 复现

```bash
KEY=<临时 key，用完即删>; BASE=https://deepai-gw.liziai.cloud/v1
# 1) 先取一道固定挑战
npx -y lmfpd@latest -b "$BASE" -k "$KEY" -m glm-5.3-flash -a cc -e low \
  --count 1 -p 1 --timeout 240 --json --output gen.json
python -c "import json;d=json.load(open('gen.json'));json.dump(d['challenges'],open('ch.json','w'),ensure_ascii=False)"
# 2) 每个模型用同一道题重复 3 次
npx -y lmfpd@latest -b "$BASE" -k "$KEY" -m <model> -a cc -e low \
  --challenges ch.json --count 1 -p 1 -n 3 --timeout 240 --json --output m.json
```

采集用的临时 key 已在结束后删除（`/admin/api/keys` 只剩 `liziai-deepai`，旧 key 复验 401）。
