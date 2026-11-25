<div align="center">

<img src="docs/assets/logo.png" alt="DeepAICode2API" width="128" height="128">

# DeepAICode2API

A reverse-engineered **Go gateway** for the upstream model service behind the local
**DeepAI Code desktop app**, speaking both the **OpenAI and Anthropic protocols**.

It reuses the API key the desktop app writes after sign-in, so no separate key is needed.
It is pure Go standard library, ships as a single ~10 MB binary, and has zero external
dependencies.

**[中文](README.md) / English**

[![ci](https://github.com/2951461586/DeepAICode2API/actions/workflows/ci.yml/badge.svg)](https://github.com/2951461586/DeepAICode2API/actions/workflows/ci.yml) [![Go](https://img.shields.io/badge/Go-1.26-00ADD8?logo=go&logoColor=white)](https://go.dev) [![deps](https://img.shields.io/badge/deps-0%20(stdlib)-00ADD8)](go.mod) [![tests](https://img.shields.io/badge/tests-652%20cases-brightgreen)](https://github.com/2951461586/DeepAICode2API/actions/workflows/ci.yml)
[![protocol](https://img.shields.io/badge/protocol-OpenAI%20%7C%20Anthropic-6b4fbb)](#3-endpoint-reference) [![docker](https://img.shields.io/badge/docker-ready-2496ED?logo=docker&logoColor=white)](docker-compose.yml) [![License](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)
[![Ask DeepWiki](https://img.shields.io/badge/Ask%20DeepWiki-blue.svg?labelColor=000000&logo=data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAACAAAAAgCAYAAABzenr0AAAACXBIWXMAAAPoAAAD6AG1e1JrAAACIUlEQVRYw+2XP2gUQRTGv7d3JhYWQawkIBaCVWzETos0gmAnVoKNTSoLK0E7EQQrC20sBVFBtLPQRgTBtBaKjYgIQUSxiJq7fT+LvCGPJZdNLt4dQj5Y3u7sznzfzLw/s9IO/icANiniCujEfWdsQgBLxAbsbraPZcmBfcAt4DNwKQsZNfEUsAB8YRV12LfA6ZHuedg7AO7ed/c+UBcbQhbiu+6wXIM6EnZWkszMJe2Ke0n6I2la0v51hFeSMLN6OwIK6jJwEBdUYb2MAxRSz6sY4geiahFgLe1Hwq6YWe3us8AN4FQQU4QM64SPY69Xyr43fADgNjAPXHb3peSsD4FDQ0VLEjAHvIhBPS6AHvA1bBO9JPAXcHFbIRtJ5yzwIQZ9CZwAusDJENIkLqsG8DpNyIZJwSUk9wLHgakcesC1JCCjiHm1kYDNOEjptCxpSVK/8f73OFLxPLAYM3oCzEX7MXf/lJbc/8kWpA4HgQdpYI9IWAbeh83whi84cHXL+58EPBoQhnmm94EzwE3gZ2p/Dhzdbhg+TaRr01x7ftb4/jBwFzjXrCvDpmLW9crVLNeR9CaapoGemb2TdCERW1tNaBNQ8jktwmozqwtpqRNtdWAjARYk39IS12ZWRX7vmJmA71nQZgi36gN7gCvAj5xc3P0jcH7kx7Ik5IC73wsh14GZsZySow5002l4Jr0b6+m4eSyvpAn8lEzsx2QHo8RfUrlN+uPq4ksAAAAASUVORK5CYII=)](https://deepwiki.com/2951461586/DeepAICode2API)

</div>

| Capability | Details |
| --- | --- |
| Dual protocol | OpenAI `/v1/chat/completions`, `/v1/responses`; Anthropic `/v1/messages` |
| Protocol alignment | Lane 8 + `X-DeepAI-Code-Instance` + `/theos` check-in; distinguishes `social_login_required` / `machine_required` / `lane_version_unsupported` |
| Account-pool governance | Group round-robin, failover, capacity backoff, per-account quota scheduling, automatic skip of policy-blocked accounts |
| Account onboarding | Browser sign-in (Google / Apple / email+password), desktop credential import, manual API key; pool entry assigns a machine fingerprint and a proxy automatically |
| Console | Single embedded page: accounts / keys / logs / stats / settings |
| Security | Key auth, admin password, same-origin write protection, optional AES-256-GCM state-file encryption |
| Regression suite | 616 cases (468 top-level + 148 subtests) / 14.7k lines of tests; `go test -race` + gofmt / vet / build / vulncheck all green; CI covers 10 packages with tests (5 more have none) plus front-end syntax and smoke checks (23) |

```text
┌──────────────┐  /v1/chat/completions   ┌────────────────┐   Api-Key + Theos headers   ┌─────────────────────────────┐
│ OpenAI SDK   │  /v1/responses          │ DeepAICode2API │  X-DeepAI-Code-Machine      │ https://api.deepai.org      │
│ Claude Code  │  /v1/messages           │   (gateway)    │ ──────────────────────────▶ │ /deepai_code/chat/completions│
│ pi / Cline   │  /v1/models             │                │ ◀────────────────────────── │ /deepai_code/responses      │
└──────────────┘  SSE streaming / JSON   └───────┬────────┘      OpenAI-compatible SSE  └─────────────────────────────┘
                                                 │ read automatically
                                                 ▼
                          ~/.deepai-code/userdata/secrets/deepai-account.bin
                          {"apiKey":"…","username":"…","isPro":false}
```

---

## 1. Reverse-engineering findings

DeepAI Code is an **Electron + local t3code server** architecture (app version **0.3.52**,
lane 8). Comparing the wire protocol item by item across
0.3.20 → 0.3.30 → 0.3.32 → 0.3.35 → 0.3.36 → 0.3.37 → 0.3.41 → 0.3.42 → 0.3.43 → 0.3.44 → 0.3.45 → 0.3.47 → 0.3.48 → 0.3.50 → 0.3.52:
**0.3.41 bumped the lane (6 → 7); 0.3.42 / 0.3.43 / 0.3.44 / 0.3.45 / 0.3.47 / 0.3.48 changed nothing on the
protocol surface (lane stays 7); the 0.3.50 agent-runtime migration (pi → dsh) touched none of the
request headers / fingerprints / check-in / inference endpoints / request-body fields; and 0.3.52
bumped the lane again (7 → 8) and added one self-identification header, `X-DeepAI-Code-Proof` —
beyond that the header set, fingerprints, check-in shape, inference endpoints and request body are
byte-identical**.
The DeepAI Connect features added in 0.3.45 (`/connect/*`, `/sponsored/authorize`, PKCE sign-in),
the 0.3.47 sign-in guidance / recovery hints / startup & storage error copy, the 0.3.48 startup
pairing-token fix plus directory-permission recovery copy, the 0.3.50 hosted DeepSeek / OpenCode
harness routing, and the 0.3.52 orchestration-v2 / ACP-provider / harness-runtime-probe work are all
account-, remote-host- and local-orchestration surfaces and do not enter the inference path;
the new `sessions/save_records` only serves history backup, also outside inference.
The component doing the real work is not the Electron shell but the t3code server inside
`resources/server.asar`; it wraps upstream capabilities as a local loopback HTTP service and
hands them to Pi (the coding agent).

### 1.1 Key facts

| Area | Finding |
| --- | --- |
| App shape | Electron shell (`app.asar`) + separate server (`server.asar`, ~200 MB) |
| Data directory | `T3CODE_HOME` or `~/.deepai-code`; state directory `<base>/userdata` |
| **Credentials on disk** | `<stateDir>/secrets/deepai-account.bin`, **plaintext JSON** (mode 0600) |
| Credential contents | `{apiKey, username, isPro}`; `apiKey` is UUID-formatted |
| **Inference entry** | `POST https://api.deepai.org/deepai_code/chat/completions` |
| Responses entry | `POST https://api.deepai.org/deepai_code/responses` (`api=openai-responses` models only) |
| Model catalog | `GET https://api.deepai.org/deepai_code/models` |
| Quota | `GET https://api.deepai.org/deepai_code/theos` |
| Usage | `GET https://api.deepai.org/get_usage_summary` |
| Speech-to-text | `POST https://api.deepai.org/speech_to_text` (field `voiceRecording`) |
| Web search | `POST https://api.deepai.org/deepai_code/web_search` |
| **Auth** | Header `Api-Key: <apiKey>` (not `Authorization`) |
| Protocol header | `X-DeepAI-Code-Theos: 8` (billing-lane version; 7 across 0.3.41–0.3.50, **8 as of 0.3.52**). **The lane gate is server-side behavior**: on 2026-11-18 (the day 0.3.41 shipped) only 7 passed; a 2026-11-19 recheck showed ≤7 all pass and 8+ returns 426; **the 2026-11-25 recheck found a window: 7 / 8 pass, 6 and 9 are refused** (`docs/recon.md` §3) |
| Self-identification header | `X-DeepAI-Code-Proof` (new in 0.3.52, lane 8): the value is produced by the bundled native helper `resource-monitor --attest` with this build's key — a check-in proof for `/theos` and a call tag for model calls. **The gateway does not implement it**: the 2026-11-25 recheck showed upstream does not validate it (missing or made-up hex both return 200) |
| Process instance | `X-DeepAI-Code-Instance: <32-hex>` (randomized once per egress proxy and sent with every JSON request through that proxy; the desktop app does it once per process) |
| Check-in | `/theos` responses carry `check_in:{nonce, seconds}`; the next `/theos` sends `X-DeepAI-Code-Check-In` plus `X-DeepAI-Code-Beacon: v=…;up=…;pi=0\|1` (`focus` is appended only when the focus state is known) |
| **Machine fingerprint** | `X-DeepAI-Code-Machine: sha256(raw machine UUID)`, used for daily free Theos; metered models return `machine_required` when absent |
| Session stickiness | `X-Session-Affinity` (the body's `prompt_cache_key` is also used upstream); the gateway additionally binds it to a specific account so one conversation's prefix cache stays on the same account (see §10) |
| Capacity rejection | `409 {"error":{"code":"session_busy"}}` + `Retry-After` |
| Account policy | `403 social_login_required`: a temporary 0.3.19 restriction, lifted as of the 2026-11-09 recheck; the gateway still treats it as a terminal `blocked` state (see `docs/recon.md` §4.1) |
| Browser sign-in | `GET /deepai_code/desktop_auth/?state=…&port=…&code_challenge=…&code_challenge_method=S256` → local `/auth-callback?code=…` → `POST /deepai_code/desktop_auth/exchange/` (body carries `code_verifier`). PKCE matches the 0.3.45 desktop app; `DEEPAI2API_LOGIN_PKCE=0` reverts to the old shape |

### 1.2 Upstream is already OpenAI-compatible

`chat/completions` requests and responses match OpenAI exactly. Responses additionally carry
OpenRouter-style `provider`, `reasoning`, `reasoning_details`, `cost`, and similar fields
(`id: gen-…`, `usage.cost_details.upstream_inference_*`, `native_finish_reason`).
Note that `provider` is the **real inference backend name** (`InferenceNet` / `Wafer` / `Reka` / …);
**the stand-in branch no longer includes a `provider` field** (the old `provider: "DeepAI"` is gone).
To tell real from stand-in you must look at whether `provider` is missing, whether `model`
echoes the requested `z-ai/<id>`, and whether the `id` prefix is `gen-` or `gen_`
(full criteria in `docs/recon.md` §3.1).

So the gateway does not need complex protocol reconstruction. The focus is:

1. **Credential import**: read the desktop app's plaintext credential file and de-duplicate;
2. **Account pool**: multi-key round-robin, failover, retry waits on `session_busy`;
3. **Model governance**: dynamic catalog cache, aliases, fallback model;
4. **Protocol adaptation**: translation for Anthropic clients and Responses clients;
5. **Observability**: request logs, token / cost stats, quota probing, console.

### 1.3 Where the machine fingerprint comes from

Desktop logic (`src/deepai/machineId.ts`):

- Windows: `(Get-CimInstance Win32_ComputerSystemProduct).UUID`, falling back to the registry
  `HKLM\SOFTWARE\Microsoft\Cryptography\MachineGuid` when the firmware UUID is a placeholder;
- macOS: `ioreg`'s `IOPlatformUUID`;
- Linux: `/sys/class/dmi/id/product_uuid`, then `/etc/machine-id`;
- Finally `sha256(lowercased raw value)`; the raw value is never uploaded.

This gateway mirrors the same logic. Containers / environments without SMBIOS can set
`DEEPAI_MACHINE_ID=<raw UUID>` explicitly.

---

## 2. Supported models

The catalog is **entirely upstream-driven**: the gateway only caches and aliases it, so upstream
additions and removals need no code change. The table below is the **live catalog** exported
from `/v1/models` on 2026-11-18 (28 models: 12 free / 16 paid).
**2026-11-24 recheck: vs. the 11-18 snapshot only one free model was added, `claude-haiku-5-5`**
(95,000 / 64,000 / 0); **the 2026-11-25 recheck (lane 8) found all 28 field-for-field identical, and
lanes 7 / 8 return the same catalog**; the other 27 are field-for-field identical (IDs / names / context /
max output / prices / `default: glm-5.3-flash`), with no removals or price changes.
`Theos/request` comes from upstream `theosPerRequest` (a string); `0` means free. The console's
"Model catalog" page reads the same data.
(The 2026-11-20 recheck was zero-diff vs. the 11-18 snapshot. Compared with the 2026-11-14 snapshot,
the **model set, names, and prices were unchanged**; only
`glyph-cluster`'s `contextWindow` moved from 320,000 to 256,000. The older `crouton-alpha` is
offline; `gpt-oss-120b` is filtered out by the client and is not shown by the gateway. The
2026-11-14 round added 6 vs. 11-13 with no removals: `muse-ember` (free), `kimi-k3.1-pro`,
`gpt-6.1-terra`, `gemini-4-hydrogen`, `gemini-4-carbon`, `claude-sonnet-5-5`.)

| ID | Name | Context | Max output | Theos/request | Notes |
| --- | --- | ---: | ---: | ---: | --- |
| `claude-haiku-5-5` | Claude Haiku 5.5 | 95,000 | 64,000 | **0** | Free (added 2026-11-24), the only non-320k entry with a premium-tier name |
| `deepai-cyber` | DeepAI Cyber | 320,000 | 131,072 | **0** | Free, text only |
| `deepseek-v4.1-flash` | DeepSeek V4.1 Flash 1017 | 320,000 | 131,072 | **0** | Free (upstream dropdown says `DeepSeek V4.1 Flash 1017`; the desktop app rewrites the old name to this) |
| `glm-5.3-flash` | GLM 5.3 Flash | 320,000 | 131,072 | **0** | Free, default model; measured 320k intact (see "Effective context ceiling" below) |
| `glyph-cluster` | Glyph Cluster | 256,000 | 131,072 | **0** | Free, text only; measured ceiling **256k** (260k intact, 320k hard-rejected) |
| `gpt-6-luna` | GPT-6 Luna | 320,000 | 128,000 | **0** | Free |
| `mimo-2.6-flash` | MiMo 2.6 Flash | 320,000 | 131,072 | **0** | Free; measured ceiling **140k** (upstream silently truncates from 170k; the gateway truncates earlier) |
| `mimo-2.6-pro` | MiMo 2.6 Pro | 320,000 | 131,072 | **0** | Free |
| `minimax-m3.1-flash` | MiniMax M3.1 Flash | 320,000 | 131,072 | **0** | Free |
| `muse-ember` | Muse Ember | 320,000 | 131,072 | **0** | Free (added 2026-11-14) |
| `qwen-3.8-flash` | Qwen 3.8 Flash | 320,000 | 128,000 | **0** | Free |
| `solar-mini-4` | Solar Mini 4 | 320,000 | 131,072 | **0** | Free, text only |
| `minimax-m3` | MiniMax M3 | 320,000 | 131,000 | 2 | Paid, text only |
| `glm-5.3` | GLM 5.3 | 320,000 | 131,072 | 3 | Paid, text only |
| `kimi-k3` | Kimi K3 | 320,000 | 131,072 | 3 | Paid |
| `kimi-k3.1-pro` | Kimi K3.1 Pro | 320,000 | 131,072 | 3 | Paid (added 2026-11-14) |
| `gpt-6.1-terra` | GPT-6.1 Terra | 320,000 | 128,000 | 5 | Paid (added 2026-11-14) |
| `muse-spark-1.3` | Muse Spark 1.3 | 320,000 | 131,072 | 6 | Paid |
| `gemini-4-hydrogen` | Gemini 4 Hydrogen | 320,000 | 131,072 | 8 | Paid (added 2026-11-14) |
| `claude-sonnet-5-5` | Claude Sonnet 5.5 | 320,000 | 64,000 | 10 | Paid (added 2026-11-14) |
| `gemini-4-argon` | Gemini 4 Argon | 320,000 | 131,072 | 10 | Paid |
| `gemini-4-carbon` | Gemini 4 Carbon | 320,000 | 131,072 | 10 | Paid (added 2026-11-14) |
| `gpt-6.1-sol` | GPT-6.1 Sol | 320,000 | 128,000 | 10 | Paid |
| `claude-opus-5-5` | Claude Opus 5.5 | 320,000 | 64,000 | 15 | Paid |
| `muse-wonder` | Muse Wonder | 320,000 | 131,072 | 15 | Paid |
| `qwen-3.8-max-prime` | Qwen 3.8 Max Prime | 320,000 | 131,072 | 15 | Paid |
| `claude-fable-5-2` | Claude Fable 5.2 | 320,000 | 64,000 | 20 | Paid |
| `gpt-6-astra` | GPT-6 Astra | 320,000 | 128,000 | 50 | Paid, `/v1/responses` only |

Notes:

- Only `deepai-cyber` / `glm-5.3` / `glyph-cluster` / `solar-mini-4` / `minimax-m3` are text-only
  (`input: [text]`); the rest accept image input (`input: [text, image]`). All support reasoning;
- **The lane-7 / lane-8 catalog converged `contextWindow` to 320,000 for all models except `glyph-cluster`
  at 256,000** (the live catalog since 0.3.41). **2026-11-19 recheck: lanes 5 / 6 / 7 return an
  identical catalog** (27 models at the time, zero diff); the per-lane differences of the 0.3.37 era are gone.
  The gateway passes upstream values through unchanged;
- **Since 2026-11-14 upstream `/deepai_code/models` requires `Api-Key`**: without one it always
  returns 401 `auth_required` (the earlier "same catalog with or without credentials" finding no
  longer holds). Catalog refresh and lane probing therefore borrow one pooled account's credential;
  see §10.1 for the operational pitfall;
- `gpt-6-astra` has `api: openai-responses` and must be called through `/v1/responses`;
  calling it on `/v1/chat/completions` is rejected upstream;
- Paid models are metered in `Theos/request`, deducted from the account's daily allowance /
  balance; when exhausted upstream returns 403 (see §6.9);
- If upstream also sends `theosPerSecond`, the console shows a "per-second billing" column and the
  `/v1/models` `deepai` metadata exposes it too;
- **The live `/v1/models` is authoritative**: the repo stores no state files
  (`deepaicode2api-data*.json` are runtime artifacts containing upstream keys, gitignored and
  cleaned). If this table and `/v1/models` disagree, click "Refresh model catalog" in the console
  and re-export in this table's format;
- **The built-in fallback matches the live catalog**: when the catalog is empty (first start before
  refresh / refresh failure), `/v1/models` returns a single `glm-5.3-flash` entry with live measured
  values (ctx **320,000**, maxTokens 131,072, free, text+image, reasoning). Older builds hardcoded
  `1,048,576`, which made clients build 1M-context requests that upstream rejected
  (fixed in v0.1.11, see `docs/changes.md` §70).

Built-in aliases (overridable in the console; 13 total, all pointing at the default free model
`glm-5.3-flash`):

- `deepai` / `deepai-code` / `default`
- `gpt-4` / `gpt-4o` / `gpt-4o-mini` / `gpt-4.1` / `gpt-3.5-turbo`
- `claude-3-5-sonnet` / `claude-sonnet-4` / `claude-3-haiku`
- `deepseek-chat` / `glm-4-flash`
- `deepai/<id>` and `openai/<id>` prefixes are stripped automatically (`deepai:` / `openai:` work too)

Unknown model names **by default** fall back to the "fallback model" → catalog default, so any
client works out of the box. With **strict model mode** enabled in the console, they instead return
`404 invalid_request_error` listing nearby catalog IDs, with no silent substitution
(turn it on for model-fingerprint / channel verification).

---

## 3. Endpoint reference

### OpenAI-compatible

| Method | Path | Description |
| --- | --- | --- |
| GET | `/v1/models` | Model catalog (with `deepai` metadata) |
| POST | `/v1/chat/completions` | Streaming / non-streaming; tools, images, `prompt_cache_key` |
| POST | `/v1/responses` | Passthrough for `openai-responses` models; translated for the rest (**stateless**; see continuation fields below) |
| POST | `/v1/audio/transcriptions` | Forward upstream speech-to-text (webm / mp4) |
| POST | `/v1/audio/translations` | Alias of the above |

### Anthropic-compatible

| Method | Path | Description |
| --- | --- | --- |
| POST | `/v1/messages` | Streaming / non-streaming; `tool_use`, `thinking`, images |
| POST | `/v1/messages/count_tokens` | Local estimate (upstream offers none) |

#### Context contract: `/v1/responses` server-side history replay (chained continuation)

Of the three protocols, `/v1/chat/completions` and `/v1/messages` still work as
"the caller sends full history each turn" (like the desktop app): the gateway stores no
conversation and does not fill in the previous turn. `/v1/responses`, however, has supported
**server-side conversation history storage + replay** since 2026-11-20, solving the "amnesia"
seen by reverse-proxy / magpie-plugin clients that carry `previous_response_id`:

- **Other models (chat/completions synthesis path, e.g. `glm-5.3-flash`)**: after each
  `/v1/responses` turn completes, the gateway stores that turn's input and output in an in-process
  cache (indexed by response id). On the next turn with `previous_response_id`, it walks the chain,
  reassembles the full input, and sends it upstream — clients need not maintain history themselves,
  continuation no longer returns 400, and context is not lost. The new response gets its own id and
  can be chained further.
  - **Cache limits**: default TTL 24 hours, at most 500 turns per process, adjustable via:
    - `DEEPAI2API_HISTORY_TTL_HOURS` (default `24`)
    - `DEEPAI2API_HISTORY_MAX_ENTRIES` (default `500`)
  - **Effective context ceiling (per-model measurement, to avoid upstream silent truncation)**:
    on 2026-11-20 all 11 free models were measured (pure passthrough + unique random ASCII ladder,
    with multi-turn cross-checks). The effective window is looked up per model:
    - **9 free models**: `glm-5.3-flash`, `deepseek-v4.1-flash`, `qwen-3.8-flash`,
      `minimax-m3.1-flash`, `muse-ember`, `solar-mini-4`, `gpt-6-luna`, `mimo-2.6-pro`,
      `deepai-cyber` → **320,000** (equal to the catalog value; no more artificial 120k cap).
    - **`glyph-cluster` → 256,000** (260k intact, 320k hard-rejected upstream with 400; catalog's
      256k matches).
    - **`mimo-2.6-flash` → 140,000** (140k intact; from 170k upstream silently truncates —
      `prompt_tokens` drops to 5,296 ≈ 3%; the ceiling is between 140k and 170k, so 140k is the
      conservative choice).
    - Unknown / paid models → `min(catalog value, 320,000)`; no more blanket 120k.
    - **Request-body byte gate (2026-11-22, issue #7)**: the real upstream cliff is a
      **request body of about 196 KB**, not a token count — a second gateway's cross-measurement:
      195,809 B read in full, from 196,811 B upstream silently drops the *head* (system prompt /
      earliest turns), pinning `usage.prompt_tokens` near 40k. In plain English (~4.2 B/token) a
      320k catalog value would need ~512 KB to trip this package's token estimate, so it cannot
      catch it. Replay therefore adds `DEEPAI2API_CONTEXT_BODY_BYTES` (default `196608`,
      `0`/negative disables): when exceeded it follows the same two paths as above
      (`truncation:auto` truncation, or an explicit 400).
    - `/v1/models` `deepai` metadata therefore adds **`effective_context_window`** (the measured
      per-model ceiling) and **`context_body_bytes`** (the byte gate's value); `context_window`
      remains the upstream catalog value, unmodified, so clients/scripts can pick the real ceiling.
    - The gateway truncates the oldest middle turns and leaves a "middle history omitted" marker
      in the input, rather than letting upstream silently cut history.
    - **Environment overrides** (for diagnosis / temporary tuning):
      - `DEEPAI2API_EFFECTIVE_CONTEXT_WINDOW`: global override (legacy behavior preserved),
        e.g. `64000`; `0`/negative disables the limit. Highest priority.
      - `DEEPAI2API_CONTEXT_WINDOWS`: per-model JSON override, e.g.
        `{"mimo-2.6-flash":140000,"glm-5.3-flash":320000}` (`0`/negative = no limit for that model;
        malformed JSON is ignored). Lower priority than the global override, higher than the
        built-in measured table.
    - One-shot probe (no chaining) measurement: unique random ASCII of 160,000 bytes →
      upstream `input_tokens=100,761` (~1.59 tokens/byte); repeated filler is prefix-cached, so
      `input_tokens` cannot be used to judge the window.
  - **Automatic context compression (`truncation`)**: after replaying the whole chain, if the
    effective window is exceeded (see above):
    - `"truncation":"auto"`: the gateway drops the oldest **middle** turns (keeping the opening
      system/first message and the most recent content that fits, inserting a "middle history
      omitted" note) and sends the request normally.
    - Unset or `"default"`: returns an explicit 400 `invalid_request_error` naming the token
      estimate and the effective/catalog window, and suggesting "compress history client-side, or
      use `truncation:auto`".
      **Never silently drop history** — that is the root of the "amnesia" symptom.
    - Token estimation: ASCII at ~1.6 bytes/token, non-ASCII at ~1 token/char (measured within
      ±30% of upstream `input_tokens`); computed only when checking limits, so normally zero
      overhead.
  - **Lifetime**: in-process storage, **cleared on restart**. If a client still carries an old
    `previous_response_id` after a restart, the gateway cannot find the record and returns 400
    `unsupported_parameter` — the client should resend the full `input` (or start a new
    conversation after restart).
  - Still **unsupported**: `conversation` (server-side sessions) and `input` `item_reference` —
    they rely on upstream-stored state the gateway cannot rebuild, so they return an explicit 400
    `unsupported_parameter`.
- **`openai-responses` models (e.g. `gpt-6-astra`)**: requests are **passed through untouched** to
  upstream `/responses`. Whether `previous_response_id` / `conversation` can continue depends on
  upstream (that model returned 429 continuously on 2026-11-19, so it is unverified; the desktop
  app itself never uses chained continuation).

**Which clients benefit**: any Responses client that runs multiple consecutive turns in one window
and relies on server-side state (typically Claude Code / Codex behind a reverse proxy, or the
magpie plugin's reverse proxy) — you can now use `previous_response_id` continuation confidently
instead of resending all history every turn. Non-chained usage (full history in `input` every turn)
is unchanged and does not regress.

Troubleshooting: request logs (the console log page's "msgs" column / the `messages` and `system`
fields in `<data file>-logs.jsonl`) record the number of messages fed back and whether a system
prompt was sent, never content — with chained continuation this count grows per turn, proving
history really is replayed.

Since 2026-11-22 the same log line also carries **`body_bytes`** (the actual request-body size sent
upstream) and **`body_near_cap`** (`true` when it reaches 90% of
`DEEPAI2API_CONTEXT_BODY_BYTES`; a log-only hint, never a block). If `messages` grows normally but
the model still forgets, check `body_bytes` first: above ~196 KB it is upstream head truncation
(issue #7), not the gateway losing history.

Claude Code only needs:

```bash
export ANTHROPIC_BASE_URL=http://127.0.0.1:10087
export ANTHROPIC_API_KEY=sk-...        # gateway key; may be left empty before any key is issued
export ANTHROPIC_MODEL=glm-5.3-flash
```

### DeepAI-native extensions

| Method | Path | Description |
| --- | --- | --- |
| GET | `/v1/deepai/theos` | Current account's free allowance / balance |
| GET | `/v1/deepai/usage` | Subscription and usage summary |
| POST | `/v1/deepai/web_search` | Upstream web search (`{query, num_results}`) |
| POST | `/v1/deepai/find_service` | Upstream service recommendation |

### Admin API (accounts / pool entry)

| Method | Path | Description |
| --- | --- | --- |
| POST | `/admin/api/accounts` | `action=enroll`: validate and write into the pool |
| POST | `/admin/api/checkin` | Quota probe. Optional `id` / `group` / `stale_only` / `limit`; the response carries `total` / `probed` / `remaining` and advances in batches |
| POST | `/admin/api/login/start` | Start browser sign-in; returns `session_id` / `port` / `loopback` / `expires_in` |
| POST | `/admin/api/login/poll` | Poll the sign-in result (loopback callbacks are captured automatically; `status=pending\|ok\|error\|expired`) |
| POST | `/admin/api/login/callback` | **Complete sign-in by pasting the callback URL** (`{session_id, callback}`). Required for Docker / remote deployments: when the browser lands on `127.0.0.1:<port>/auth-callback?code=…&state=…` and cannot open it, paste the full address-bar URL back |

#### Retired endpoint: `/admin/api/state`

`GET /admin/api/state` used to **flatten every account into one response** (603 accounts ≈ 430 KB).
It was retired on 2026-05-20 (RFC 8594 `Sunset`) and now returns `410 Gone`.
Two endpoints replace it:

| What you need | Use |
| --- | --- |
| Metadata (settings / stats / keys / models / lane / latency) | `GET /admin/api/overview` |
| Account list (paging + filtering + sorting) | `GET /admin/api/accounts?offset=&limit=&group=&status=&q=&sort=&order=` |

If a script still uses the old endpoint, migration is two requests:

```bash
curl -s -H "X-Admin-Password: $DEEPAI2API_ADMIN_PASSWORD"      http://127.0.0.1:10087/admin/api/overview
curl -s -H "X-Admin-Password: $DEEPAI2API_ADMIN_PASSWORD"      'http://127.0.0.1:10087/admin/api/accounts?limit=200&offset=0'
```

### Console

| Path | Description |
| --- | --- |
| `/` | Embedded single-page console (no external dependencies) |
| `/ping` | `pong` |
| `/health` | Account / model / machine-fingerprint summary; `ready` / `degraded` / `blocked` counts (definitions at the end of §6.4) |
| `/metrics` | Prometheus text metrics (same auth as `/admin/api/*`): request volume / status classes, **per-endpoint latency histograms**, **check-in duration/blocking/timeouts**, **upstream error-code distribution**, **shape-fallback count**, in-flight requests, pool distribution, goroutines/heap |
| `/admin/api/version` | Current version + latest-release comparison (server-side GitHub lookup, cached 6h; offline it returns only the current version without affecting the console) |
| `/admin/api/*` | Accounts, keys, logs, stats, settings, quota, sign-in |

> Every response carries `X-Request-Id` (a valid incoming value is echoed). Access logs are
> one JSON object per line (`event=access` with `request_id` / `status` / `ms` / `bytes`), directly
> filterable with `jq`.

---

## 4. Quick start

```bash
# 1) Build (outputs to bin/; or just: go build -o deepaicode2api.exe .)
make build

# 2) Start it (imports desktop credentials and pulls the model catalog automatically)
./bin/deepaicode2api

# 3) Open the console
#    http://127.0.0.1:10087/

# 4) Add an account (console)
#    "Browser sign-in" logs into upstream with Google / Apple / email+password;
#    you can also "Import local desktop" or "Manual API key"
```

> **`.env` is now loaded by the gateway at startup** (previously only lazily on some paths, so a
> `DEEPAI2API_MASTER_KEY` written there did not apply to the main process).
> The state file holds upstream API keys and account passwords; on Windows `0600` sets no ACL, so
> enable encryption on the first deployment. **Stop the running gateway first**, then:

```bash
printf 'DEEPAI2API_MASTER_KEY=%s\n' "$(openssl rand -hex 32)" >> .env
./bin/deepaicode2api -seal -yes   # seal existing plaintext (including append-only history logs)
./bin/deepaicode2api              # every later save is encrypted automatically
```

`-seal` prints a key fingerprint (first 8 chars of sha256) for verification and checks that no file
still contains plaintext. **Keep the master key together with your backups; losing it makes the
pool unrecoverable.** If you only set the env var without running `-seal`, the main file is rewritten
as ciphertext on the next save, but historical log lines stay plaintext (readers accept mixed
formats and will not error).

Sample startup log:

```text
state file: …/deepaicode2api-data.json
machine fingerprint: a1b2c3d4e5f6… (used by upstream for the daily free Theos)
imported account desktop-… (~/.deepai-code/userdata/secrets/deepai-account.bin)
model catalog updated: 28 models, default glm-5.3-flash
DeepAICode2API dev started: http://127.0.0.1:10087/
```

### Call examples

```bash
# OpenAI style
curl http://127.0.0.1:10087/v1/chat/completions \
  -H 'Content-Type: application/json' \
  -d '{"model":"glm-5.3-flash","messages":[{"role":"user","content":"hi"}]}'

# Streaming
curl -N http://127.0.0.1:10087/v1/chat/completions \
  -H 'Content-Type: application/json' \
  -d '{"model":"glm-5.3-flash","messages":[{"role":"user","content":"hi"}],"stream":true}'
```

Python OpenAI SDK:

```python
from openai import OpenAI
client = OpenAI(base_url="http://127.0.0.1:10087/v1", api_key="sk-...")
print(client.chat.completions.create(
    model="glm-5.3-flash",
    messages=[{"role": "user", "content": "hello"}],
).choices[0].message.content)
```

---

## 5. Command-line flags and environment variables

| Flag | Environment variable | Default | Description |
| --- | --- | --- | --- |
| `-host` | `DEEPAI2API_HOST` | `127.0.0.1` | Listen address |
| `-port` | `DEEPAI2API_PORT` | `10087` | Listen port |
| `-data` | `DEEPAI2API_DATA_PATH` | user config dir | State file |
| `-deepai-dir` | `DEEPAI2API_DATA_DIR` | auto-detected | Desktop app data directory |
| `-admin-password` | `DEEPAI2API_ADMIN_PASSWORD` | empty | Admin password |
| `-allow-insecure` | `DEEPAI2API_ALLOW_INSECURE` | `false` | Allow non-loopback listen without an admin password |
| `-import` | — | — | Import credentials and exit |
| `-no-import` | — | — | Do not auto-import at startup |
| `-seal` | — | — | Seal existing plaintext in state/stats/logs to ciphertext, then exit (needs `DEEPAI2API_MASTER_KEY`) |
| `-yes` | — | — | Confirm the irreversible operation with `-seal` |
| `-version` | — | — | Print the version |

Other environment variables:

| Variable | Description |
| --- | --- |
| `DEEPAI_API_KEY` | Add one upstream account directly (optional) |
| `DEEPAI_MACHINE_ID` | Override the raw machine-fingerprint UUID (required in containers) |
| `DEEPAI2API_MASTER_KEY` | State-file encryption master key (64-hex / base64, 32 bytes); empty means plaintext; putting it in `.env` is enough, it is loaded at startup |
| `DEEPAI_API_ORIGIN` | Override the upstream origin (self-hosted testing) |
| `DEEPAI2API_ADMIN_PASSWORD` | Admin password (console / admin API) |
| `PROXY_POOL` | Inference proxy pool (comma- or newline-separated full URLs; assigned round-robin per account at pool entry) |
| `PROXY_POOL_FILE` | Proxy-pool file (one per line) |
| `DEEPAI2API_ENV_FILE` | Path to `.env` |
| `DEEPAI2API_LOGIN_PKCE` | Whether browser sign-in uses PKCE (default `true`, matching desktop 0.3.52); `0` reverts to the old `state`+`port`-only shape |
| `DEEPAI2API_HISTORY_TTL_HOURS` | `/v1/responses` server-side history TTL in hours (default `24`) |
| `DEEPAI2API_HISTORY_MAX_ENTRIES` | Maximum turns in the `/v1/responses` server-side history cache (default `500`) |
| `DEEPAI2API_EFFECTIVE_CONTEXT_WINDOW` | Global override for the replayed effective context ceiling (highest priority; default is the per-model table above; `0`/negative disables) |
| `DEEPAI2API_CONTEXT_WINDOWS` | Per-model effective context ceiling, JSON object such as `{"mimo-2.6-flash":140000}` (`0`/negative = no limit for that model; malformed JSON is ignored) |
| `DEEPAI2API_CONTEXT_BODY_BYTES` | Request-body safety ceiling in bytes (default `196608` ≈ 192 KiB): upstream silently truncates the head from about 196 KB (issue #7); `0`/negative disables |
| `T3CODE_HOME` / `DEEPAI_CODE_HOME` | Desktop app data directory |

### Diagnostic tool

```bash
go run ./cmd/probe -usage                 # credentials + model catalog + quota
go run ./cmd/probe -prompt "hello"        # one real call
go run ./cmd/probe -prompt "hi" -stream   # streaming call
go run ./cmd/probe -key <api-key>         # validate a specific key directly
```

---

## 6. Account onboarding and pool governance

> **The email registration path has been removed** (2026-11-10): the console's "bulk register"
> page, `/admin/api/register`, `cmd/register`, and the `internal/register` package are all gone.
> Only browser sign-in, desktop import, and manual API key remain; pool governance (groups, proxy
> pool, machine fingerprints, quota re-probing) is unaffected. Historical design and measurements
> remain in `docs/changes.md` / `docs/lessons.md`; the code no longer provides registration.

### 6.1 Onboarding methods

| Method | Entry point | Description |
| --- | --- | --- |
| Browser sign-in | Console "Browser sign-in" / `POST /admin/api/login/start` | The DeepAI sign-in page supports Google / Apple / email+password; a local `127.0.0.1` callback exchanges the code for a key, and pool entry assigns a machine fingerprint and proxy automatically |
| Desktop import | Startup flag `-import` / console "Import local desktop" | Reads the desktop app's `deepai-account.bin` |
| Manual API key | `POST /admin/api/accounts {"action":"manual"}` | Key only; fill in the rest with `assign-machine` / `assign-proxy` |

The full browser sign-in flow is in `docs/recon.md` §4.2. Google sign-in must complete in the
browser: upstream's OAuth `redirect_uri` is pinned to deepai.org and the code-to-token exchange
needs its `client_secret`, so the server cannot perform it on your behalf.

### 6.2 Pool groups and round-robin

- Accounts have a `group` (default `default`);
- API keys also have a `group`; a request only round-robins within the matching group;
- Keys without a group use all groups;
- Admins can pick a group when creating a key in the console.

This lets one gateway serve multiple batches / tenants without cross-talk.

### 6.3 Proxy pool

- Inference and pool entry share one proxy pool: `PROXY_POOL` (comma / newline separated full URLs)
  or `PROXY_POOL_FILE` (one per line);
- Browser sign-in assigns an egress round-robin at pool entry; existing accounts can be bound later
  with `POST /admin/api/accounts {"action":"assign-proxy"}`
  (by default it only fills empty slots; `"reassign":true` overwrites, `"clear":true` rolls back;
  scope with `"id"` or `"group"`);
- The inference side calls upstream through the account-bound proxy (`Server.clientFor`); the admin
  API only shows `host:port` and never leaks proxy credentials;
- Diverse egress is valuable: the daily free Theos is granted **per network (egress IP)** (see
  `network_bound` in `docs/recon.md`). After spreading 14 accounts across 12 egresses, the number of
  accounts with quota rose from 5 to 10.

### 6.4 Machine fingerprints (why one per account)

Upstream grants free Theos **once per day per** `X-DeepAI-Code-Machine`, therefore:

- Each account gets an independent machine UUID at pool entry (`deepai.NewMachineUUID`), used for
  `/theos` and all inference thereafter (`Account.MachineID`);
- Only when there is no account-level fingerprint does it fall back to the process-level
  `MachineID()` (desktop imports / legacy accounts);
- With a shared fingerprint, multiple accounts on the same machine cancel each other's quota — the
  root cause of the old `granted=0` in the pool;
- Existing accounts can be backfilled in one call (only empty slots, never overwriting):

```bash
curl -s -X POST http://127.0.0.1:10087/admin/api/accounts   -H "X-Admin-Password: $DEEPAI2API_ADMIN_PASSWORD"   -d '{"action":"assign-machine"}'
# {"ok":true,"assigned":20}
```

Backfilling triggers an asynchronous quota re-probe. Scheduling also prefers accounts with quota
(`hasQuota`); accounts known to have `balance=0` are only used when the whole group is empty.

### 6.4.1 Automatic quota re-probing

Upstream grants quota once per day at `refresh_at`. Early on, the only option was a manual "refresh
all quotas", which took 11 minutes for 653 accounts and was interrupted halfway whenever the page
closed (`r.Context()` cancellation). Three automatic layers now cover it:

| Layer | Trigger | Scope | Failure handling |
| --- | --- | --- | --- |
| Pool-entry re-probe | 45s / 2min / 6min after entry | New accounts | Accept successful results |
| On-demand re-probe | Inference selects an account whose snapshot is stale | Only the accounts used, at most once per 15min | Does not touch `Status` / `Failures` (an upstream blip should not confuse scheduling) |
| Background guardian | One batch every 5 minutes (≤24) | `refresh_at` passed / never probed / not probed for over 6h | Same as above |

All three share one in-flight de-duplication table and **4 global probe slots**: when 653 accounts'
snapshots expire at once, a traffic burst does not fan out into hundreds of parallel `/theos`
requests (the shape most likely to trigger risk control). Those that miss a slot are picked up next
round.

`/admin/api/checkin` now has batch + progress semantics:

```bash
# Probe only stale-quota accounts in group g2, at most 50 this batch
curl -s -X POST localhost:10087/admin/api/checkin   -H "X-Admin-Password: $DEEPAI2API_ADMIN_PASSWORD"   -d '{"group":"g2","stale_only":true,"limit":50}'
# → {"ok":true,"total":120,"probed":50,"remaining":70,"results":[…]}
```

The console's "Refresh all quotas" pages through by `remaining` until the whole pool is probed.
`/health` additionally reports `quota_fresh` (probed within 15 minutes) and `quota_due` (the
guardian will de-duplicate and probe).

The three account counts in `/health` (`ready` / `degraded` / `blocked`) are defined as:

| Field | Meaning |
| --- | --- |
| `ready` | Disabled accounts excluded; `blocked` excluded (it will not self-heal); `error` excluded only while **the error is fresh** (within 10 minutes) |
| `degraded` | `error` with an error within 10 minutes — the console badge also shows "N recent failures" |
| `blocked` | Blocked by upstream policy (`social_login_required` etc.); needs manual action |

`status=error` only means "the last request failed", not that the account is broken: the scheduler
only skips `blocked`, so `error` accounts still participate in round-robin. After
`deepai.ErrorStaleAfter` (10 minutes) without another failure, the 30s background sweep sets it back
to `ok`.

Transient failures (EOF / reset / timeout / 429 rate limit / 5xx / capacity rejection / check-in
throttling) do not even mark `error`; they only bump `failures` and `last_err`. **Daily quota
exhaustion** (`Too many requests today`) is the exception — it will not self-heal that day, so it is
marked `error` and counted in `degraded` until the next day's `refresh_at`.

### 6.5 Measurement: quota before and after backfilling machine fingerprints

On the same pool (21 accounts: 1 desktop import + 20 legacy email accounts, 11 proxies), running
`/admin/api/checkin` before and after `assign-machine` backfill:

| Metric | Before | After |
| --- | --- | --- |
| Accounts with an independent machine fingerprint | 0 / 21 | **21 / 21** (all distinct) |
| Accounts with `balance > 0` | **0 / 21** | **20 / 21** |
| Typical quota | `balance=0.00000 granted=0.00000` | `balance=200.00000 granted=200.00000` |
| Quota refresh | — | `refresh_at=2026-11-07T04:03:03Z` (next day 04:03 UTC) |
| 8 consecutive real inferences | — | **8/8 HTTP 200**, spread across **8 different accounts** |

The only `balance=0` account is the desktop import (`source=desktop`, alone in the `default` group) —
its quota for that day had already been consumed under the original machine fingerprint.

> Note: `/theos` `balance` has 5 decimal places and a single request costs about `1.8e-6`, so small
> calls show no visible decrease; use `/v1/deepai/usage` to see consumption.

### 6.6 Acceptance criteria

| Dimension | How to check |
| --- | --- |
| Quota | Share of accounts with `balance > 0` after `assign-machine`; whether the three-layer re-probe fills in within 10 minutes |
| Inference | 200 rate on real models and the provider branch (criteria in `docs/recon.md` §3.1) |
| Health | `/health` `quota_fresh` / `quota_due`; `quota` / `pool` events in logs |
| Scheduling | Fair round-robin within a group; whether `blocked` is skipped |

Only two or more batches under the same configuration are comparable (`docs/lessons.md` §4.4).

### 6.7 Security and compliance

- The pool is only for managing **your own** DeepAI accounts; follow upstream's terms of service;
- State-file mode is `0600`; but **`0600` sets no ACL on Windows**, so other local users can still
  read it — strongly prefer setting `DEEPAI2API_MASTER_KEY` to enable whole-file encryption (see §7);
- Upstream grants daily free quota per machine; this project does not bypass that limit.

---

## 7. Console

- **Overview**: request volume, failures, total tokens in/out, **today's tokens in/out**, **cache hit
  rate**, upstream cost, ready pool accounts, model catalog (free / paid, Theos, context, endpoint).
  Tokens and characters are compacted to **K / M / B** (3 significant digits; hover for the exact
  value); the model catalog keeps exact integers. "Today" uses the **local date** and resets at
  midnight; cache hit rate = upstream-reported `cached_tokens / prompt_tokens` (all three upstream
  shapes are normalized, see `gateway.ParseUsage`), and shows `—` rather than 0.0% when there are no
  samples.
- **Accounts**: import desktop credentials, browser sign-in, manual API key, refresh quota; the list
  supports **group / status / keyword** filtering (server-side paging + sorting). Besides
  enable/disable, delete, and clear-error, the row menu covers pool hygiene: **bind / clear proxy,
  assign machine fingerprint (empty slots only), change group, mark verified**. A "masked display"
  toggle masks emails (`u***@domain`) and usernames; the preference lives in localStorage.
  Search/filtering still match full values — masking only affects display (handy for screenshots,
  screen recordings, and remote support).
- **Keys**: issue `sk-…` gateway keys; shown once at creation; **group can be rebound**.
  **With no key issued, only loopback callers may call keyless; non-loopback sources always get
  `503 setup_required`.** If listening on a non-loopback address with no keys, startup generates a
  `bootstrap` key and prints it once.
- **Logs**: endpoint, model, streaming, status, latency, character counts, tokens, cost, account;
  filter by **endpoint / account / minimum status code / keyword / start time** (characters and
  tokens likewise use K/M).
- **Settings**: timeouts, retries, log retention, CORS, fallback model, strict model mode, model
  aliases, admin password, and per-key rate limiting (RPS / burst / concurrency cap; 0 = unlimited).
- **Top-bar version**: shows the build version (`v0.1.2`); clicking opens the releases page. The
  console asks `/admin/api/version` once at startup (server caches 6 hours) and highlights newer
  releases.
- **Sign-in**: the admin password uses a separate `/login` page (validated by
  `POST /admin/api/auth`); only then does the console load. The password lives only in this tab's
  `sessionStorage`, and the top bar only offers "sign out". Any admin API returning 401 clears the
  password and returns to the login page. With no admin password set, nobody is blocked (loopback
  direct). Whether the gate is enabled comes from `/health`'s `admin_required`.

Secure defaults:

- Listening on a **non-loopback** address requires an admin password, otherwise startup is refused
  (use `-allow-insecure` to bypass);
- With no key issued, non-loopback data-plane requests get `503 setup_required`, not a free pass;
- With no admin password set, the admin API is **loopback-only**;
- Write operations are same-origin checked to block CSRF;
- CORS defaults to local origins only; no `*`;
- State-file mode `0600`; the console only shows masked keys.
- With `DEEPAI2API_MASTER_KEY` configured, the whole state file is AES-256-GCM encrypted, and no
  plaintext upstream key / account password appears on disk; an encrypted file **refuses to start**
  when the master key is missing or wrong instead of silently resetting to empty state.
- `-seal` also re-seals **historical plaintext lines** in append-only logs (per-line verified,
  idempotent); merely changing the env var does not backfill old logs.
- The state file defaults to `os.UserConfigDir()` (Windows `%AppData%`, Unix `~/.config`), avoiding
  Program Files or shared drives; if a file already exists in the old location it keeps being used,
  so the pool is not lost.

---

## 8. Architecture and layout

```text
main.go                     wiring: flags → storage → routes → signals (no business logic)
tasks.go                    background tasks: persistence / catalog refresh / quota guardian + import and seal commands
env.go                      flag and environment parsing (defaults centralized; see §5)
cmd/probe/                  diagnostic tool
internal/deepai/            upstream client (protocol, SSE, machine fingerprint, credential import)
internal/envfile/           .env loading (effective at startup)
internal/registry/          model catalog resolution and aliases
internal/store/             state persistence + account-pool scheduling (≤400 lines per file after the split)
  ├─ store_open.go         Open / Save / snapshots / atomic writes
  ├─ store_accounts.go     account CRUD / paging / scheduling entry points
  ├─ store_keys.go         gateway keys
  ├─ store_models.go       model catalog / machine fingerprints / settings
  ├─ store_telemetry.go    stats and log queries
  ├─ reseal.go             -seal: seal existing plaintext (including historical logs)
  └─ state_seal.go         AES-256-GCM envelope / key fingerprint
internal/proxy/             proxy pool (round-robin / masked labels / file loading)
internal/pool/              account-selection policy (group round-robin / quota decision / cooldown window); testable without Store
internal/parallel/          bounded concurrent fan-out (concurrency cap / ctx cancel / index-ordered write-back)
internal/retry/             backoff retry (thresholds / jitter / non-retryable classification)
internal/httpx/             pure HTTP helpers and middleware (JSON responses / error envelope / decode / gzip / ETag)
internal/gateway/           protocol translation: field mapping + usage parsing; no HTTP / store / pool deps
  ├─ anthropic_types.go    Anthropic request model (Messages / tools)
  ├─ anthropic_translate.go Anthropic ⇄ Chat bidirectional field mapping
  ├─ anthropic_stream.go   streaming content-block manager (thinking / text / tool_use)
  ├─ responses_translate.go Responses ⇄ Chat field mapping
  └─ usage.go              Usage + upstream usage parsing + lenient JSON accessors
internal/server/            HTTP routes, streaming I/O, console (assembly layer)
  ├─ chat.go                OpenAI Chat Completions (passthrough + normalization)
  ├─ anthropic.go           Anthropic Messages entry
  ├─ responses.go           Responses API entry (passthrough / Chat synthesis)
  ├─ responses_stream.go    Responses stream event generator (writes sseWriter, hence stays in this package)
  ├─ upstream.go            openModel: account selection / slot acquisition / failover / error classification
  ├─ accounts.go            account slots, concurrency caps, admission queueing
  ├─ models.go              model catalog resolution and alias fallback
  ├─ sse.go                SSE writing and [DONE]
  ├─ reqlog.go              per-request instrumentation and stats persistence
  ├─ probe.go               quota probes (on-demand / guardian / batched checkin)
  ├─ extensions.go          quota, usage, search, speech
  ├─ login.go               browser sign-in (RFC 8252 loopback callback)
  ├─ admin_accounts.go      account admin entry: GET paging / POST dispatch by action
  ├─ admin_actions.go       10 actions, each with a minimal request struct
  ├─ gateway_alias.go       short package-local names for gateway (type aliases, migration compatibility)
  ├─ web.go                 console asset route whitelist (assets live in the web/ package)
  └─ web/                   console static package (assets.go: go:embed + ETag + CSP)
       ├─ index.html        page markup (301 lines)
       ├─ style.css         styles (design tokens + layout)
       ├─ js/app.js         assembly: data fetch / URL state / startup order (231 lines)
       ├─ js/events.js      event wiring: nav / top bar / filters / sorting / language / masking
       ├─ js/theme.js       theme, sidebar collapse, mobile drawer
       ├─ js/pager.js       account pager (page number / page size)
       ├─ js/i18n.js        dictionary + text substitution (zero deps)
       ├─ js/format.js      pure formatting (zero deps)
       ├─ js/state.js       shared mutable UI state (paging cursor)
       ├─ js/dom.js         DOM helpers + row menu
       ├─ js/table.js       header sort state and pure functions
       ├─ js/api.js         admin API calls (auth / error normalization / de-duplication / timeout retry)
       ├─ js/ui.js          toast
       ├─ js/actions.js     account / key mutations + quota probes (refresh injected via setReload)
       └─ js/render/        four render modules: accounts / keys / logs / settings
```

Layering principles:

- **`deepai`** only cares about the upstream protocol (paths, headers, error semantics, SSE) and does
  not know the pool exists;
- **`store`** only handles state and scheduling (persistence, groups, failover, quota decisions) and
  sends no network requests;
- **`server`** only does protocol translation and auth; account selection always goes through
  `Store`;

So upstream protocol changes only touch `deepai`, scheduling-policy changes only touch `store`, and
tests can run against a fake upstream and a temporary state file independently.

---

## 9. Deployment (Docker)

```bash
docker build -t deepaicode2api .
docker run -d --name deepaicode2api \
  -p 10087:10087 \
  -e DEEPAI_API_KEY=<api-key> \
  -e DEEPAI_MACHINE_ID=<raw machine UUID> \
  -e DEEPAI2API_ADMIN_PASSWORD=<admin password> \
  -v $PWD/data:/data \
  -e DEEPAI2API_DATA_PATH=/data/deepaicode2api-data.json \
  deepaicode2api
```

**For long-lived deployments prefer `scripts/deploy.sh`** (build → immutable tag → up → wait for
health):

```bash
sudo install -m 600 /dev/null /etc/deepaicode2api.env   # secrets (accounts / passwords / proxies / machine fingerprint)
sh scripts/deploy.sh v0.1.13                            # deploy; on failure it prints logs and a rollback command
DEEPAI2API_IMAGE=<old tag> sh scripts/deploy.sh         # roll back to a specific image
```

- Keep secrets in `DEEPAI2API_ENV_FILE` (default `/etc/deepaicode2api.env`, 0600), **not** in the
  code directory; `.env` only holds deployment parameters such as `VERSION` /
  `DEEPAI2API_IMAGE` / `DEEPAI2API_ADDR`;
- The container has a built-in `healthcheck` (in-image `wget /healthz`, every 30s) and
  `mem_limit: 512m`; without one, `unless-stopped` never restarts a wedged process
  (2026-11-22 production retrospective).

> `X-DeepAI-Code-Theos: 8` is the current client's (0.3.52) lane (7 across 0.3.41–0.3.50). The lane gate is adjusted by
> upstream at any time: on 2026-11-18 it was "only 7"; by 2026-11-19 it had relaxed to "≤7 all pass,
> 8 and above return 426 `lane_version_unsupported`" (see `docs/recon.md` §3). Sending an old lane
> **still works**, so whether the protocol is caught up is judged by the `/theos` `lane_version`
> (the console overview's "upstream lane").
> `X-DeepAI-Code-Machine` is still required by metered models: without it they return
> `400 machine_required`. Containers usually cannot read the host's SMBIOS UUID, so
> `DEEPAI_MACHINE_ID` is **mandatory** in container deployments. Filling in the desktop machine's
> raw UUID reuses the same daily free quota.
>
> **Browser sign-in inside a container must use the paste callback**: upstream sends the browser to
> `http://127.0.0.1:<random session port>/auth-callback?code=…&state=…`, and that `127.0.0.1` is the
> **host's own loopback**; the gateway listens inside the container (with a per-login random port),
> and `-p` cannot map to the container's loopback. The browser therefore stops on a "cannot connect"
> page — that is expected, not a broken gateway. Paste the **full URL from the address bar** into the
> console's "Browser sign-in → Paste callback URL" (`POST /admin/api/login/callback`), and the
> gateway completes the code → key exchange and pool entry. The `code` is one-time; the same URL
> can only be exchanged once, and `state` must match this session (pasting only `code` is rejected).
>
> On Linux you can also use `--network host` so the container shares the host loopback and the local
> callback works directly; Docker Desktop (Windows/macOS) has no such semantics.

---

## 9.5 Versioning and releases

The version is injected at build time via `-ldflags -X main.version=...`, defaulting to `dev`:

```bash
make build                       # Makefile uses git describe as the version
go build -ldflags "-X main.version=v0.1.2" -o bin/deepaicode2api .
./bin/deepaicode2api -version    # print the version and exit
```

- The **console** shows this version at the top; click it to open the releases page.
- **Update check**: at startup the console requests `/admin/api/version`; the server looks up
  `api.github.com/repos/<owner>/<repo>/releases/latest` and caches it for 6 hours. When offline,
  blocked, or the repo is unreachable, it returns only the current version (with an `error` field
  explaining why), without affecting console use or popping an error in the UI.
- **Development builds** (`version=dev`) skip the comparison; otherwise they would always claim a
  newer version exists.
- **Docker builds** default to `VERSION=docker` (not semver, so no comparison and no upgrade prompt).
  To show a real version and highlight updates in the console:

  ```bash
  VERSION=$(git describe --tags) docker compose up -d --build
  # equivalent: make docker (automatically uses git describe)
  ```
- Release flow: `git tag v0.1.5 && git push origin v0.1.5`, then
  `sh scripts/release.sh v0.1.5 --publish` to cross-compile platform binaries and create a GitHub
  Release (the script clears the previous version's `deepaicode2api_*` artifacts from `dist/` before
  building: `gh release upload` uploads `dist/*`, so stale binaries would be uploaded too).
- **Naming rule (hard constraint)**: tags and Release titles are **always `vX.X.X` (three numeric
  segments)**, **without any descriptive suffix** (no "v0.1.23 — fix…", no dates, no Chinese).
  Descriptions go into the release notes (`dist/release-notes-<ver>.md`) and `docs/changes.md`.
  `release.sh` validates the format (exits on violation) and rewrites the title back to the version
  if the Release already exists; the console's update comparison only reads `tag_name`, so the title
  does not matter.

---

## 10. Limitations and notes

- `/v1/chat/completions` and `/v1/messages` store no conversation and do not fill in history.
  The `/v1/responses` synthesis path keeps completed responses in-process and supports
  `previous_response_id` replay; the cache has a TTL, an entry limit, and a 64 MiB total cap, and is
  cleared on restart. Missing history, `conversation`, and `item_reference` still return an explicit
  400; context overflow is handled per the "context contract";
- **A catalog value of 320k ≠ 320k actually usable**: upstream silently truncates the head (system
  prompt / earliest turns) at a request body of about 196 KB (issue #7 measurement; ~40k tokens in
  plain English, while ~69k tokens of Cyrillic still reads in — showing the criterion is bytes, not
  tokens). The gateway enforces `DEEPAI2API_CONTEXT_BODY_BYTES` (default 196,608 B) only on the
  `/v1/responses` replay path; `/v1/chat/completions` and `/v1/messages` stay transparent proxies and
  only log `body_bytes` / `body_near_cap` when over the line — clients should still compress history
  for long sessions;
- `X-DeepAI-Code-Theos` sends `8` (since 0.3.52; 7 across 0.3.41–0.3.50). The upstream gate may
  relax or narrow into a window: on 2026-11-19 ≤7 all passed and 8+ returned 426, on 2026-11-25
  7 / 8 passed while 6 and 9 were refused, so an old
  lane "working" does not mean it is the right lane — the difference from upstream's current lane is
  observed via `/theos`'s `lane_version` (`Server.UpstreamLane` / `LaneDrift`, visible in the console
  overview);
- The heartbeat `X-DeepAI-Code-Beacon`'s `v=` must be the current client version (now **0.3.52**,
  see `deepai.AppVersion`); `pi=` has been optional since 0.3.30, but sending it is harmless;
- Lane-probe results are trusted unconditionally when validated by **the current build**
  (`Settings.LaneBuild == deepai.LaneVersion`), even if older than the compiled value; otherwise
  `max(stored, compiled)` is used. The probe sequence starts from the compile-time lane and **must
  not try old lanes first**;
- Body-shape rejection matches both wording variants and also catches
  `400 + code=invalid_request_error`; if the shape-retry still fails, the second error is returned;
- Anthropic `budget_tokens` maps by magnitude to `low/medium/high/xhigh/max` (boundaries at
  4k/16k/32k/64k), and `output_config.effort` passes through unchanged; on 2026-11-15 upstream
  accepted all six levels;
- Inference requests send `user-agent: node` + `accept: */*` per desktop measurements, **without**
  Chrome UA / `sec-ch-ua` / `Accept-Language` (the inference side only sends desktop headers);
- Missing `X-DeepAI-Code-Machine` makes metered models return `machine_required`, so environments
  without SMBIOS must set `DEEPAI_MACHINE_ID`;
- 0.3.19 once restricted inference to **Google/Apple-signed-in** accounts (email accounts got
  `403 social_login_required`); **the 2026-11-09 recheck lifted this**, and email accounts work
  again. The gateway still treats that code as a terminal policy state (marks `blocked`, stops
  scheduling), so a future re-tightening needs no code change; when you see `blocked`, first check
  whether `last_err` is still that code before deciding to re-authorize;
- Upstream free quota is granted daily per "machine fingerprint + account" and returns 403 when
  exhausted; fingerprints are **one per account** (see §6.4), and when shared only the first account
  gets quota;
- `openai-responses` models such as `gpt-6-astra` must use `/v1/responses`; calling them via
  `/v1/chat/completions` is rejected upstream (the gateway returns the error unchanged);
- Speech-to-text upstream accepts only `audio/webm` / `audio/mp4`; transcode other formats first;
- **The managed-host control plane** `/deepai_code/connect/*` (device registration / remote hosts /
  TLS relay) **is not implemented**: it needs an Ed25519 + X25519 dual-key device identity and a
  managed-host lifecycle, and serves the desktop app's "turn this machine into a managed host"
  feature, unrelated to inference (reverse-engineering details in `docs/recon.md` §2.1);
- Catalog and lane probing **must carry one account's `Api-Key`** (since 2026-11-14; the earlier
  "no credentials needed" finding no longer holds): `GET /deepai_code/models` without a key always
  returns 401 `auth_required`. Catalog / lane probes consume no Theos, so the gateway borrows one
  pooled account's credential (`Server.probeAccount`: prefers ready accounts with a machine
  fingerprint); with no accounts, a catalog refresh reports "import an account first" instead of
  surfacing the 401 to the console; proxy warm-up counts 401 as success (the path is reachable), see
  `docs/recon.md` §2;
- `/v1/messages/count_tokens` is a character-count estimate, not upstream's real tokenizer;
- The state file stores upstream API keys in plaintext by default (like the desktop app); with
  `DEEPAI2API_MASTER_KEY`, the whole file is encrypted — always keep the master key out of backups
  that could leak;
- Browser sign-in exchanges for an upstream API key and writes it to the state file; whole-file
  encryption can be enabled (see above);
- Proxy URLs contain credentials and must live only in `.env` / local records (gitignored), never in
  commits;
- **Upstream reachability** (issue #1): the gateway has no built-in censorship circumvention;
  `api.deepai.org` must be directly reachable. Mainland-China users can route **upstream requests**
  through their own proxy via `PROXY_POOL` or a per-account egress (see §6.3); browser sign-in still
  needs the DeepAI sign-in page to be reachable locally, otherwise use the Docker paste-callback flow
  (see §9). The gateway only reuses networking you already have; it does not provide or bypass
  network controls;
- **Single-instance deployment**: the pool, rate-limit buckets, and in-flight counters live in
  process memory, and the state file has a single writer (`tmp + fsync + rename`). Do not mount the
  same `DEEPAI2API_DATA_PATH` into multiple replicas writing concurrently, and do not expect replicas
  to share scheduling state — horizontal scaling needs external storage and is out of scope;
- **Data-plane rate limiting is off by default**: the console's "runtime settings" can set
  `rate_limit_rps` (requests per second per key), `rate_limit_burst` (bucket size), and
  `key_concurrency` (per-key concurrency cap); 0 means unlimited. Limiting is per key and returns
  `429` + `Retry-After` + `rate_limit_error` when triggered;
- **Check-in budget** (`checkin_block_ms`, default 10000): how long a synchronous check-in may block
  before inference. On 2026-11-13 production measured 292 check-in blocks, 4.8s average, 53 hitting
  the 10s cap — the main source of first-byte latency. Lowering it to 2000 significantly improves
  latency (cost: this request may go out with an old nonce, and the next turn catches up; it will
  not fail);
- **Check-in warming** (since v0.1.13): a background sweep every 45s walks the pool and consumes
  "due" `/theos` outside the request path (at most 4 accounts and 2 concurrent per round, failures
  silent). The hot path still calls `EnsureCheckIn` (nonce semantics unchanged), but most calls
  return immediately because a check-in just happened; a 2026-11-22 production check found 940
  requests blocked by check-in over 37 hours (some consuming the full 10s budget);
- **Egress-proxy cooldown** (since v0.1.13): transport failures (proxy 504 / EOF / timeout) are
  recorded per proxy string, with consecutive failures cooling down 30s → 60s → 120s … up to 10
  minutes; scheduling prefers switching egress (`poolsvc.PickAvoiding`) and only uses a cooling
  egress when no other account is left, never returning 429 to clients for this reason. The console
  overview shows the "egress cooling" count;
- **Egress normalization and preflight** (since v0.1.15): proxies written as `host:port` (no scheme)
  in accounts now parse correctly (`http://` is added by default); but **an exclusive egress is
  probed before enabling** (HTTP CONNECT → upstream `:443`, 6s, result cached 10 minutes). If the
  probe fails it falls back to the global egress and logs a WARN — avoiding residential proxies that
  "look configured but cannot carry traffic" (2026-11-22 production incident: missing scheme →
  `Host=""` → all 16 accounts' exclusive egresses silently failed and traffic crowded onto one
  global egress);
- **Preset desktop shape** (`preset_desktop_shape`, default off): for callers already known to need
  the desktop shape (remembered by API key for 10 minutes), prepend a system message before the
  first send, saving a 400 that is certain to fail. When off, the learning path still runs
  "passthrough → 400 → shape retry";
- **Request body cap and what happens when it is exceeded** (since v0.1.28): `context_body_bytes`
  (default 196608) and `body_cap_mode` (default `warn`). Upstream silently truncates the *start* of
  request bodies above ~196 KB (issue #7: system prompt and the earliest turns are dropped while
  `usage.prompt_tokens` pins at ~40k). `warn` only records those requests in the log
  (`body_near_cap` + WARN at 90% of the cap); `reject` returns `400 invalid_request_error` with the
  byte count and the cap instead of forwarding a request that is known to be truncated. The
  `DEEPAI2API_CONTEXT_BODY_BYTES` env var still works as a fallback (console settings win);
- **Three timeout budgets** (console-tunable since v0.1.28): `nonstream_budget_sec` (overall budget
  for non-streaming requests, default 180s, range 30–600) replaces the hard-coded 90s — on
  2026-11-25 all 60 `504`s on `/v1/chat/completions` in 6 hours landed exactly on `ms=90000`;
  `stream_idle_sec` (per-frame idle while streaming, default 120s) and `first_byte_sec`
  (first byte for non-streaming, default 300s) cover the "stream stalled" and "long reasoning has
  not produced a first byte yet" tails separately;
- **Paid models only use accounts with a positive balance** (since v0.1.28): models with
  `theosPerRequest > 0` no longer accept accounts whose allowance is known to be non-positive, and
  session stickiness cannot pin a paid request to such an account. The old behaviour was "prefer
  accounts with quota, fall back to the whole pool", so negative-balance accounts were still picked
  and burned one upstream `403 theos_exhausted` per request (2026-11-25 health check: 3 accounts
  were negative);
- **Check-in cadence** matches the desktop app (anchored at the moment the reply completes,
  `seconds/2`, default 300s). The earlier "latest nonce ≠ replied nonce" check degenerated to one
  check-in every 30s (see `docs/changes.md` §60);
- **Admin passwords have brute-force backoff**: after 5 consecutive failures from the same source,
  exponential backoff starts (1s, capped at 30s), and even a correct password returns `429` during
  the backoff; one success clears it. If you lock yourself out, wait for the backoff to expire or
  restart the process (the table is in memory);
- **Logs are JSONL**: startup and request logs emit one JSON object per line (`slog`), with `msg`
  preserved verbatim; access logs carry `request_id` / `status` / `ms` / `bytes`; use `jq -r .msg` to
  restore a human-readable form.

### 10.1 Known pitfalls (read before changing things)

All of these have bitten us before; `docs/recon.md` has the fuller reverse-engineering record:

| Symptom | Cause | Do not "fix" it like this |
| --- | --- | --- |
| Invalid key returns 200 with HTML | Upstream 302-redirects to the sign-in page | Do not follow with `CheckRedirect`; `ErrUseLastResponse` is intentional |
| "Model changed" after switching UA | **The 2026-11-13 recheck found no UA routing** (`node` / `curl` / Chrome all return the same response shape); history in `docs/recon.md` §3.1 | Do not "fix" the pool's UA based on old conclusions; continuing to send `user-agent: node` only matches the desktop app |
| Response lacks `provider` | **It is no longer a criterion**: since 2026-11-13 real requests also stopped sending `provider` / `service_tier` / `cost_details`, `id` is uniformly `gen-<32 hex>`, and `model` echoes `zai/<slug>` | Stop judging stand-ins by `provider` / `gen_` / cost (see `docs/recon.md` §3.1; use model fingerprints to validate accounts) |
| Pool "fingerprints suddenly worsen" | Check-in incomplete: fetching `/theos` without replying with the nonce is not a check-in and falls into the stand-in branch | Wait one `EnsureCheckIn` renewal cycle; switching machine fingerprints turns it into a cold state again |
| `403 check_in_refused` | Today's "restart check-in" budget is used up (`restarted its check-in too many times today`) | Do not retry or mark the account `error`: inference still works and recovers tomorrow; stop running check-in health checks |
| `400 This request cannot be accepted.` | The body is not the desktop shape under the new wording (since 2026-11-13; `node` UA + real local machine fingerprint + missing system message. When the gateway and desktop app are co-located this is the **norm**) | Do not match only the old `not one DeepAI Code sends` wording: `BodyRejected` must match both, otherwise `ChatBodyFallback` will not retry (see `docs/recon.md` §5.2) |
| Upstream catalog default model is empty | The field is `default`, not `default_model` | Do not add `default_model` compatibility; it silently yields nothing |
| Catalog refresh / lane probe returns 401 `auth_required` | **Since 2026-11-14 `/models` requires `Api-Key`** (previously credential-free); an empty pool guarantees 401 | Do not treat it as "the key is dead" and clear accounts; the gateway now borrows an account's credential (see `docs/recon.md` §2.2) |
| Pooled account has `granted=0` | Shared machine fingerprint; daily quota is granted once | Do not remove the `Account.MachineID` fallback logic |
| Long responses cut off | Upstream heartbeat `: keep-alive` was dropped | `streamChat` must pass comment lines through verbatim |
| Tool-call arguments truncated | Multiple `data:` lines in one event were not joined with `\n` | Use `deepai.ReadSSE`; do not `json.Unmarshal` line by line |
| Passwords disappear from records | `Append` goes through `redactRecord` | That is by design; passwords are in `*-credentials.jsonl` |
| `go test -race` link failure (`cannot find 'ld'`) | The mingw runtime only ships `ld.bfd.exe` | Do not require PATH changes; run `make race-shim` to add a same-name copy |
| Non-streaming request returns 502 "not valid JSON" | Upstream **defaults to SSE when `stream` is absent** | Always send `stream:false` explicitly; do not treat `stream` as optional |
| New accounts stay at `balance=0` and are skipped | Upstream has not granted quota yet at pool entry | Do not cache 0 as fact; keep the 45s/2m/6min re-probes and the 15-minute staleness rule |
| Closing the console probes only half the quotas | Background tasks misused `r.Context()`, so closing the page cancels them | Background tasks must detach from the request ctx and use their own timeout |
| One dead egress makes a whole batch show 0 quota | `No exit node` is an egress failure, not a bad account | Do not delete and recreate accounts; rebind with `assign-proxy` |

Also: `/v1/chat/completions`'s `reasoning_content` is a compatibility field added by the gateway;
upstream natively has only `reasoning`, and an existing value is never overwritten.

## 11. Documentation index

**Check "status" before trusting content**: `maintained` = consistent with current code and usable as
a reference; `historical` = a process record kept for context whose conclusions may have been
overturned by later rechecks (trust the latest entries in `docs/changes.md` and `docs/recon.md`).

| File | Content | Status |
| --- | --- | --- |
| `README.md` | Overview, endpoints, parameters, acceptance baseline, and known pitfalls (this document's Chinese original) | Maintained |
| `README.en.md` | Complete English translation of the above (added 2026-11-22) | Maintained (kept in sync with the Chinese version) |
| `docs/recon.md` | Reverse-engineering quick reference: upstream endpoints, headers, error semantics, reproduction commands (replaces HAR) | Maintained (last rechecked 2026-11-22) |
| `docs/changes.md` | 2026-11 change log: root causes, decisions, acceptance (§71–§77 are the latest round) | Maintained |
| `docs/lessons.md` | **Engineering lessons**: ten stress rounds → reusable rules + pre-change checklist | Maintained |
| `docs/performance.md` | Stress data and optimization plan: single-account E2E, phase breakdown, P0/P1/P2 payoff estimates | Maintained (data snapshot 2026-11-10) |
| `docs/pi-lens.md` | This repo's pi-lens configuration: what is off, why, how to verify and restore | Maintained |
| `docs/model-fingerprint-2026-11-10.md` | Fingerprint measurements for pooled models (11-10 full sweep + 11-11 / 11-24 / 11-25 follow-ups, criteria and reproduction commands) | Historical snapshot: the catalog is now 28 models; the 11-25 rerun narrowed the rule to same-round 3/3 family comparison |
| `docs/optimization-plan.md` | The first scan's full optimization plan | Historical: fully implemented |
| `docs/optimization-plan-r2.md` | Second scan (write amplification / console / re-probe) plan and payoff estimates | Historical: implemented |
| `docs/assets/` | Project icons: `logo.png` (512, used by README) / `logo.ico` (16–256 multi-size, Windows); the console's `/favicon.ico` is the same file (see `docs/changes.md` §58) |
| `.env.example` | Every configurable option with notes |
| `Makefile` | `make build / test / test-race / lint / cover` |

## 12. License

MIT. This project only replicates the call path of an already-signed-in local client; please observe
DeepAI's terms of service.

> **The client's own declaration (added in 0.3.43, still present in 0.3.44 / 0.3.45 / 0.3.47 / 0.3.48 / 0.3.50 / 0.3.52, and extended in 0.3.52 to cover the `--attest` helper)**: in
> `server.asar`'s `src/deepai/api.ts`, next to the lane header and check-in, a comment cites DeepAI
> ToS §4 and states explicitly that these headers and the check-in exist for "DeepAI Code app
> self-identification" and **must not be copied into other clients**. This project only reuses the
> existing path of an already-signed-in local client, for its own accounts, and does not distribute
> or operate it on others' behalf; if you want to use these headers elsewhere, confirm permission
> yourself first.
