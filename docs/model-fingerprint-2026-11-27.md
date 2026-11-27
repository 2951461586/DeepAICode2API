# 全量模型指纹（2026-11-27，网关 v0.1.29-1693b20）

用 [LM Fingerpoint Detector](https://lm.ikale.io)（`npx lmfpd@latest`）对线上
`https://deepai-gw.liziai.cloud/v1` 的 **28 个模型全部**做了一轮检测。

> 工具原话：**结果是参考库内的封闭集合排序，排名与置信度都不是身份证明。**
> 判断渠道是否可信要固定参数、重复多轮、结合其他证据。本文只是把一次采样记下来。

## 方法

| 项 | 取值 |
| --- | --- |
| CLI | `npx lmfpd@latest`（参考库 77 个模型，`reference_sha256 7d775ce2…66db6`） |
| 端点 | `https://deepai-gw.liziai.cloud/v1`，协议 `-a cc`（Chat Completions）；`gpt-6-astra` 例外：只支持 `-a responses`，打 chat 会回 `400 Model 'gpt-6-astra' is not available on this endpoint` |
| 参数 | 免费 12 个：`-e low --count 3 -p 3`（3 样本，可校准，出 prob）；付费 16 个：`--count 1`（1 样本，只有排名）——控制花费，见下 |
| 流式 | 必须流式。`-ns` 非流式会撞 Cloudflare 100 s 上限（实测 `HTTP 524`） |
| 采样 | 每轮 300+ 个 1–355 的整数，提示词语言随机 |

### 为什么一定要 `-e low`

不降推理强度时，上游把 completion 硬顶在 **8192**：`completion_tokens=8192`、
`reasoning_tokens=8192`、`content=""`、`finish_reason=length` —— 推理把预算吃光，
正文一个字都没吐，样本直接失败。加上 `-e low` 之后 reasoning 显著缩短，样本
变成 `truncated`（数量够、可评分）。

## 结果

| 请求模型 | 计费 | 有效样本 | decision | 最像（参考库内） | score | prob |
| --- | --- | --- | --- | --- | ---: | ---: |
| `glm-5.3-flash` | 免费 | 3/3 | — | `gemini-4-carbon` | 2.34 | 0.86 |
| `deepseek-v4.1-flash` | 免费 | 3/3 | — | `gemini-4-carbon` | 1.84 | 0.66 |
| `mimo-2.6-flash` | 免费 | 3/3 | — | `gemini-4-carbon` | 2.09 | 0.82 |
| `qwen-3.8-flash` | 免费 | 1/1 | partial | `qwen3.8-omni-flash` | 1.68 | — |
| `minimax-m3.1-flash` | 免费 | 2/3 | partial | `qwen3.8-flash` | 2.15 | — |
| `muse-ember` | 免费 | 3/3 | — | `gemini-4-carbon` | 1.83 | 0.47 |
| `solar-mini-4` | 免费 | 3/3 | — | `gemini-4-carbon` | 1.92 | 0.50 |
| `gpt-6-luna` | 免费 | 3/3 | — | `gemini-4-carbon` | 1.89 | 0.63 |
| `claude-haiku-5-5` | 免费 | 3/3 | — | `gpt-6-astra` | 1.60 | 0.34 |
| `mimo-2.6-pro` | 免费 | 2/3 | partial | `mimo-v2.5` | 2.56 | — |
| `glyph-cluster` | 免费 | 1/3 | partial | `grok-4.7` | 2.16 | — |
| `deepai-cyber` | 免费 | 3/3 | — | `muse-spark-1.2` | 1.62 | 0.24 |
| `minimax-m3` | 付费 2 | 1/1 | partial | `grok-4.7` | 1.91 | — |
| `gpt-6.1-sol` | 付费 10 | 1/1 | partial | `swe-2.1` | 2.18 | — |
| `gemini-4-argon` | 付费 10 | 1/1 | partial | `claude-fable-5.1` | 2.06 | — |
| `glm-5.3` | 付费 3 | 1/1 | partial | `step-5-preview` | 2.00 | — |
| `kimi-k3` | 付费 3 | 1/1 | partial | `claude-opus-5.5` | 2.41 | — |
| `kimi-k3.1-pro` | 付费 3 | 1/1 | partial | `qwen3.8-max-0902` | 1.84 | — |
| `gpt-6.1-terra` | 付费 5 | 1/1 | partial | `gemini-4-carbon` | 1.93 | — |
| `muse-spark-1.3` | 付费 6 | 1/1 | partial | `grok-4.5` | 2.12 | — |
| `gemini-4-hydrogen` | 付费 8 | 1/1 | partial | `claude-opus-4.8` | 1.75 | — |
| `gemini-4-carbon` | 付费 10 | 1/1 | partial | `mimo-v2.5-pro` | 1.78 | — |
| `claude-sonnet-5-5` | 付费 10 | 1/1 | partial | `qwen3.8-max-0902` | 2.09 | — |
| `claude-opus-5-5` | 付费 15 | 1/1 | partial | `claude-opus-4.8` | 2.22 | — |
| `muse-wonder` | 付费 15 | 1/1 | partial | `qwen3.8-omni-flash` | 2.11 | — |
| `qwen-3.8-max-prime` | 付费 15 | 1/1 | partial | `gpt-6.1-terra` | 1.87 | — |
| `claude-fable-5-2` | 付费 20 | 1/1 | partial | `qwen3.8-max-0902` | 1.85 | — |
| `gpt-6-astra` | 付费 50 | 1/1 | partial | `gemini-4-carbon` | 1.89 | — |

付费 16 个模型按 `theosPerRequest` 计：**共花 185 Theos**（`--count 3` 会是 555，所以付费组用了 1 样本；
代价是这些行没有置信度，只有排名）。

## 看到的模式

1. **6 个「flash」族模型指向同一个候选 `gemini-4-carbon`**，且置信度并不低：
   `glm-5.3-flash 0.86`、`mimo-2.6-flash 0.82`、`deepseek-v4.1-flash 0.66`、
   `gpt-6-luna 0.63`、`solar-mini-4 0.50`、`muse-ember 0.47`。
   这有两种解释，本轮数据分不开：
   - 上游确实把这些名字挂在同一个后端上（换名不换模型）；
   - 参考库里 `gemini-4-carbon` 是这类「通用强模型」输出的吸引子。
2. **家族对齐的只有 3 个**：`qwen-3.8-flash → qwen3.8-omni-flash`、
   `mimo-2.6-pro → mimo-v2.5`、`claude-opus-5-5 → claude-opus-4.8`。
3. **大量不符**：`glm-5.3 → step-5-preview`、`kimi-k3 → claude-opus-5.5`、
   `gemini-4-argon → claude-fable-5.1`、`gpt-6.1-sol → swe-2.1`、
   `muse-wonder → qwen3.8-omni-flash`…
4. **直接请求 `gemini-4-carbon` 反而判成 `mimo-v2.5-pro`**（1 样本），
   说明单样本噪声不小——这也是 `--count 1` 行不能当结论的原因。

## 局限（必须一起看）

- 付费 16 个只有 **1 个样本**：工具会标 `decision=partial`，`probability` 为空，
  仅表示「在参考库里的相对排序」，不能与免费组的 `prob` 直接比较；
- 与 `docs/model-fingerprint-2026-11-10.md` / 11-25 两轮**不可直接对比**：
  参考库版本变了（那两轮的结论里 `glm-5.3-flash` 指向 grok 家族，本轮指向
  `gemini-4-carbon`），工具自己在文档里也强调「跨参考库不可比」；
- `qwen-3.8-flash`、`glyph-cluster`、`minimax-m3.1-flash` 有样本失败/截断，
  `claude-sonnet-5-5`、`qwen-3.8-max-prime` 首次流中断（重试后成功），
  这些行的分数只代表剩下的有效样本；
- 结果不与「渠道是否逆向完整」挂钩：本轮请求形状、车道、签到都与桌面端一致
  （`gateway_fallback_total=0`），指纹不符不等于网关有问题。

## 复现

```bash
KEY=<临时 key>            # 用完即删
BASE=https://deepai-gw.liziai.cloud/v1
npx -y lmfpd@latest -b "$BASE" -k "$KEY" -m glm-5.3-flash -a cc -e low --count 3 -p 3   --timeout 180 --json --output glm.json
npx -y lmfpd@latest -b "$BASE" -k "$KEY" -m gpt-6-astra -a responses -e low --count 1   --timeout 180 --json --output astra.json
```

采集期间的临时 key 已在结束后删除（`/admin/api/keys` 只剩 `liziai-deepai`）。

---

## 复核（同日）：先看协议面，再看噪声

### 1. 协议面逐项对照（桌面端 0.3.52 vs 网关）

**推理请求（`/deepai_code/chat/completions` 与 `/responses`）**

| 元素 | 桌面端 | 网关 | 说明 |
| --- | --- | --- | --- |
| `Api-Key` | ✅ | ✅ | `commonHeaders` |
| `X-DeepAI-Code-Theos`（车道 8） | ✅ | ✅ | `laneVersion()` |
| `X-DeepAI-Code-Machine` | ✅ | ✅ | 账号级机器指纹 |
| `X-DeepAI-Code-Instance` | ✅（进程级随机） | ✅（进程级随机） | 语义一致 |
| `X-Session-Affinity` | ✅（会话内固定） | ✅（有 affinity 时） | 客户端给或网关派生 |
| `user-agent: node` / `accept: */*` / `accept-language: *` / `sec-fetch-mode: cors` | ✅ | ✅ | `desktopInferenceHeaders` |
| `Content-Type: application/json` | ✅ | ✅ | |
| **`X-DeepAI-Code-Proof`（0.3.52 新增，车道 8）** | ✅ | **❌ 缺** | 值由随包原生 helper + **构建密钥**算出，网关无法实现；2026-11-25 实测上游不校验（缺失/伪造均 200） |

**签到（`/deepai_code/theos`）**：网关额外带 `X-DeepAI-Code-Check-In`（nonce 回传）与
`X-DeepAI-Code-Beacon: v=0.3.52;up=<秒>;pi=<0|1>`，只有 `focus` 省略（桌面端也只在焦点已知时发）。

**请求体**：桌面端固定发 `model/messages/stream/stream_options/store=false/max_completion_tokens/
temperature/tools/tool_choice/reasoning_effort`；网关**默认透传调用方字段**（`preset_desktop_shape=false`），
仅在收到 `400 ... not one DeepAI Code sends / cannot be accepted` 后才用 `ChatBodyFallback`
收敛成桌面形状并记忆。本次指纹的 97 条请求里有 **13 条没有 system 消息**（fpd 的 `-a cc` 单条 user），
桌面端不会这样发。

**协议模块**：chat 合成 / responses 直通 / messages 合成 / `previous_response_id` 重放 /
`prompt_cache_key` 透传 / 推理内容透传 —— 均在位；`gateway_fallback_total=0`
（从未触发形状回退）。

**唯一实打实的缺口就是 `X-DeepAI-Code-Proof`**，其余是「非桌面客户端请求体形态不同」。

### 2. 但漂移的主要来源不是协议面，是采样噪声

同一模型（`glm-5.3-flash`）、**同一组挑战**（`--challenges` 固定）、同一协议、同一天、连续两次：

| 轮次 | top-1 | top-1 分数 | top1−top2 |
| --- | --- | ---: | ---: |
| run 1 | `gpt-6-astra` | 1.62 | **0.02** |
| run 2 | `grok-4.6` | 1.76 | 0.11 |

输入完全相同，冠军不同，且分差只有 0.02。再看本轮 28 个模型的 top1−top2 间隔：
**19 个 < 0.25、最大也只有 0.88**（`kimi-k3`）——也就是说「第一名」和「第二名」
普遍在噪声尺度内。

换协议（`/chat/completions` vs `/responses`）也一样：`glm-5.3-flash` 两路径都指向
`gemini-4-carbon`（0.86 / 0.44），`qwen-3.8-flash` 都是 `qwen3.8-omni-flash`，
而 `deepseek-v4.1-flash` 从 `gemini-4-carbon` 变 `gemini-3.5-flash-lite`（同家族、不同型号）。
有影响，但幅度没有超出上面的噪声。

### 3. 结论修正

- **「6 个 flash 模型都指向 `gemini-4-carbon`」这一条要降级**：它们的分差在
  0.14–0.63，而同一模型同挑战重复实验的漂移已经能跨候选、跨家族。单轮 3 样本
  不足以支持「上游同源」的结论。
- **没有找到「缺请求头 / 缺协议模块 → 指纹漂移」的证据**：唯一的缺头（Proof）
  上游当前不校验，其余头与模块逐项对齐，形状回退从未触发。
- 真要查「替身分支」，需要**固定挑战 + 多轮（`-n 5` 以上）+ 固定协议**，
  并把 `preset_desktop_shape` 打开做 A/B（注意它会补 system 消息，会改提示词，
  必须配合固定挑战才可比）。

### 4. 下一轮的建议做法

```bash
# 一次采样，固定挑战，5 轮，每轮 3 样本；只认「最常见候选」与稳定的家族
npx -y lmfpd@latest -b "$BASE" -k "$KEY" -m glm-5.3-flash -a cc -e low   --challenges ch.json --count 3 -p 3 -n 5 --timeout 180 --json --output out.json
```

判读门槛（按本轮实测的噪声水平）：**top1−top2 < 0.3 视为无法判定**；只有连续多轮
冠军一致、且分差 > 0.5 时才谈「疑似同源」。
