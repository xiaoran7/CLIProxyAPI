# CLI Proxy API (Enhanced Fork)

English | [中文](README_CN.md) | [日本語](README_JA.md)

[![Go Version](https://img.shields.io/badge/go-1.26+-00ADD8.svg?style=flat-square&logo=go)](https://golang.org)
[![Tests](https://img.shields.io/badge/tests-8444%20passed-10b981.svg?style=flat-square)](https://github.com/router-for-me/CLIProxyAPI)
[![Protocol](https://img.shields.io/badge/protocol-OpenAI%20%7C%20Claude%20%7C%20Codex-emerald.svg?style=flat-square)](llms-full.txt)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg?style=flat-square)](LICENSE)
[![LLMs Index](https://img.shields.io/badge/LLMs-llms.txt-orange.svg?style=flat-square)](llms.txt)

> Enterprise-hardened fork of [router-for-me/CLIProxyAPI](https://github.com/router-for-me/CLIProxyAPI) providing unified API routing, protocol translation, multi-account quota scheduling, and resilient stream orchestration across OpenAI, Claude, Gemini, Codex, Grok, Tencent WorkBuddy, and OpenCode.

```text
Coding Agents (Claude Code / Codex CLI / Cursor / OpenAI & Anthropic SDKs)
  -> Gateway Layer (Auth, Body Decompression, SelectiveGzip, 15s SSE Keepalive, LoopGuard)
  -> Routing & Affinity (SessionCache [session_affinity.json], 429 Sink Pool, 5 Selectors)
  -> Canonical Pipeline (ApplyThinking -> ThinkingConfig -> Payload Semantic Barrier)
  -> Provider Executors (Claude, Codex, Gemini/Antigravity, WorkBuddy, OpenCode, Kimi, Grok)
```

## Core Highlights

- **Tencent WorkBuddy / CodeBuddy Dual-Realm Suite**: Native CN (`workbuddy-cn/`, `copilot.tencent.com`, `WB_CN_CHAT_UPSTREAM`) and Intl (`workbuddy/`) routing; Desktop 5.8.4 trace cloaking (`11128` guard); upstream `stream: true` folding (`AggregateWorkBuddyStream`, `11101` fix); live credit multipliers (`:3.33x`); billing meters; and CST `CheckinEngine` (`workbuddy_checkins.json`) with compensation claiming.
- **OpenCode Zen 1M Free-Tier Protocol**: Rewrites `/zen/v1` to `/inference/v1`, sanitizes `oc_sk_...` keys, injects `1.19.0` headers and valid `bash`/`read` tool schemas with stub filtering, and provides zero-config 1M free models (`glm-5.3-flash-free`, `deepseek-v4.1-flash-free`).
- **Multi-Strategy Load Balancing & Affinity**: `round-robin`, `weighted-round-robin`, `fill-first`, `smart` (earliest reset window), and `pace` (hourly headroom); persistent session bindings (`session_affinity.json`) and HTTP 429 cooldown sink queues.
- **Stream Resilience, Redaction & Console**: 15s SSE `: keep-alive` heartbeats, `LoopGuard` breaker, `SelectiveGzipMiddleware`, HMAC-SHA256 zero-knowledge redaction (`{{API_KEY_...}}`), and embedded `/management.html` CPAMC console.

## Supported Providers

| Provider | Representative Models | Supported Channels & Protocols |
| :--- | :--- | :--- |
| **Anthropic Claude** | Claude Fable 5.1, Opus 5.5, Sonnet 5.5, Haiku 5.5 | OAuth, Setup-Token, API Key, Interactions, Thinking binding |
| **OpenAI & Codex** | GPT-6 Astra, GPT-6.1 Sol, GPT-6 Luna, o3 | Codex OAuth (SIWC), API Key, Responses HTTP/WS Relay |
| **Google Gemini** | Gemini 4 Argon, Gemini 3.8 Flash, 3.1 Pro | AI Studio, Vertex AI, Gemini CLI, Antigravity Hub 2.23.0 |
| **Tencent WorkBuddy** | DeepSeek V4.1 Flash, GLM 5.3, Kimi K3, Hy4 Preview | Dual-realm (`workbuddy/` & `workbuddy-cn/`), OAuth, CheckinEngine |
| **OpenCode** | `glm-5.3-flash-free`, `deepseek-v4.1-flash-free` (1M) | `/inference/v1` cloaking, tool stub filtering, zero-config free tier |
| **Kimi / Grok / Muse / Devin** | Kimi K3, Grok 4.7, Muse Wonder/Spark 1.3, SWE-2 | OAuth, CLI login, and OpenAI-compatible endpoints |

## Quick Start

```bash
gofmt -w . && go build -o cli-proxy-api ./cmd/server
./cli-proxy-api --config ./config.yaml --no-browser
```

| Key / Variable | Default | Description |
| :--- | :--- | :--- |
| `routing.strategy` | `round-robin` | Strategy: `round-robin`, `weighted-round-robin`, `fill-first`, `smart`, `pace` |
| `session-affinity` | `true` | Persist session bindings (`session_affinity.json`) with `session-affinity-ttl` (`1h`) |
| `models.variant-mode` | `compact` | `/v1/models` mode: `compact` (rate-suffixed IDs only), `raw`, or `all` |
| `global-excluded-models` | `[]` | Wildcard patterns stripped from outbound `/v1/models` and `/v1beta/models` |
| `WB_CN_CHAT_UPSTREAM` | `copilot.tencent.com` | CN WorkBuddy chat upstream override (e.g. `https://www.workbuddy.cn`) for overseas hosts |

## Documentation

- **Management V8 API Reference**: [docs/management-api-v8.md](docs/management-api-v8.md) (`/v8/management/config`, `/credentials`, `/workbuddy/checkin`, `/workbuddy/compensation`, `/plugins/workbuddy/quota`, `/models/global-excluded-models`, `/routing/cooldown/reset`)
- **Go SDK Guides**: [SDK Usage](docs/sdk-usage.md) | [Advanced Executors & Translators](docs/sdk-advanced.md) | [Access & Auth](docs/sdk-access.md)
- **LLM Context Specs**: [llms.txt](llms.txt) (Compact Index) | [llms-full.txt](llms-full.txt) (Full Architectural Reference)

## Acknowledgements & License

Built on [router-for-me/CLIProxyAPI](https://github.com/router-for-me/CLIProxyAPI) with inspiration from [Magpie](https://github.com/magpie-project) and [WorkBuddy2API-Hub](https://github.com/ardeyouxipianyi/workbuddy2api-hub). Released under the [MIT License](LICENSE).
