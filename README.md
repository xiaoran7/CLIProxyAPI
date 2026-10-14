# CLI Proxy API (Enhanced Fork)

English | [中文](README_CN.md) | [日本語](README_JA.md)

> This repository is a heavily customized, enterprise-hardened, and production-ready fork of the upstream [router-for-me/CLIProxyAPI](https://github.com/router-for-me/CLIProxyAPI).
> It provides unified API routing, protocol translation, multi-account high-concurrency scheduling, and resilient orchestration for OpenAI, Anthropic Claude, Google Gemini, OpenAI Codex, Grok, Tencent WorkBuddy, and OpenCode across modern coding agents and CLI workflows.

---

## Core Highlights & Fork Enhancements

Building upon upstream's high-performance proxying and protocol normalization, this fork introduces substantial architectural advancements focusing on **agent connection resilience**, **multi-strategy quota load balancing**, **deeply reverse-engineered providers (WorkBuddy & OpenCode)**, and **out-of-the-box management dashboard integration (CPAMC)**:

### 1. Deep Reverse-Engineering & Native Domestic Agent Suites

- **Tencent WorkBuddy / CodeBuddy Dual-Realm Native Management**:
  - **Dual-Realm Automatic Resolution**: Native support for Mainland China (`copilot.tencent.com`) and International (`workbuddy.ai`) domains, dedicated endpoint routing, and credential translation.
  - **Deterministic WAF Fingerprinting**: Built-in deterministic device fingerprint synthesis (`X-Machine-ID`, `X-Session-ID`, `X-IDE-Name`, `X-Domain`) to cloak requests as genuine IDE clients against Tencent WAF blocks.
  - **Automated Daily Check-in Engine (`CheckinEngine`)**: Background daemon running on Beijing time (CST) to automatically claim daily quota, track consecutive check-in streaks, deduplicate requests, and persist state across restarts (`workbuddy_checkins.json`).
  - **Dynamic Lifecycle Control**: Toggle background check-in on or off on the fly via the management UI or REST API (`PUT /v8/management/workbuddy/checkin`).
  - **Intelligent Quota 404 Fallback**: Seamlessly falls back to Tencent activity meter billing endpoints when standard points paths return 404, extracting actual points balance (e.g. 200 points), daily allowances, and active season themes.
  - **Singleflight Token Refresh**: De-duplicates concurrent refresh token requests to prevent token invalidation under high concurrency.
  - **Interactive CLI OAuth Login**: One-command interactive QR-code scanning via WeChat or WeCom to effortlessly generate structured credentials.

- **OpenCode Native Protocol & 1M Free-Tier Models Zero-Config Integration**:
  - **Anti-Detection Schema Cloaking**: Reverse-engineered protocol headers, cloaking caller tool signatures, and injecting virtual system prompts to bypass vendor scrutiny.
  - **Zero-Config 1M Context Models**: Native out-of-the-box routing for OpenCode 1M-context models (`step-5-preview-free`, `longcat-2.5-preview-free`, `mimo-v2.6-flash-free`, `ling-3.1-flash-free`).
  - **First-Class Provider Presentation**: Elevated as a top-level provider inside the CPAMC control panel.

### 2. Enterprise-Grade Multi-Strategy Load Balancing & Affinity

- **Five Advanced Scheduling Algorithms**:
  - **Round-Robin**: Standard stateless uniform distribution.
  - **Weighted Round-Robin**: Capacity and weight-aware proportional dispatch.
  - **Fill-First**: Saturates accounts sequentially to preserve standby credentials.
  - **Smart Quota Routing**: Dispatches requests preferentially to accounts whose quota refresh window expires soonest, maximizing token utilization before resets.
  - **Pace Quota Routing**: Dynamically paces request rates against remaining window time and consumption speed, eliminating quota starvation early in the cycle.
- **Cross-Restart Persistent Session Affinity**:
  - Persists long-running session affinities in `session_affinity.json`, surviving restarts and crashes without cache loss.
  - Features sliding-window TTL to extend active session bindings, maximizing prompt-cache hit rates for Claude Code and Codex.
  - Integrated with a **429 Rate-Limit Sink Pool**: throttled credentials automatically sink into a dynamic cooldown queue while requests seamlessly retry on healthy credentials.

### 3. Agent Stream Resilience & High Availability

- **Streaming Loop Guard Circuit Breaker**:
  - Detects runaway reasoning or repetitive token loops during extended thinking phases, severing stalled connections before they exhaust upstream quotas and client bandwidth.
- **SSE Keepalive Pumper**:
  - Pumps compliant SSE comment heartbeats (`: keep-alive`) during prolonged model thinking pauses, preventing edge CDN / WAF / Cloudflare timeouts (HTTP 524 / 504).
- **Zero-Knowledge Redaction & Privacy**:
  - Comprehensive log and audit sanitization, ensuring API tokens, authorization headers, cookies, and sensitive prompt bodies are never leaked in application logs.

### 4. Turnkey Management Console (Deeply Customized CPAMC v1.26.2)

- **Fully Embedded Static Serving**: Bundles the complete Web UI directly within the binary and static distribution directory.
- **Dedicated WorkBuddy Suite Drawer**:
  - Clean separation between Mainland CN and International accounts.
  - One-click batch quota refresh showing balance and VIP tier.
  - Interactive engine status badge with instant Pause / Resume controls.
  - 100% account visibility without ghost credentials.
- **Storage File Safety Hardening**:
  - Enforces strict isolation of internal state files (`workbuddy_checkins.json`, `session_affinity.json`), preventing accidental exposure, download, or deletion via management endpoints.
  - Full catalog inspection with global exit model exclusions.

---

## Supported Providers

| Provider | Description | Supported Channels & Protocols |
| :--- | :--- | :--- |
| **Anthropic Claude** | Claude Fable 5.1, Claude Opus 5.5, Claude Sonnet 5.5, Claude Haiku 5.5 | OAuth, Setup-Token, API Key, Interactions, Codex protocol mapping |
| **OpenAI & Codex** | GPT-6 Astra, GPT-6.1 Sol, GPT-6 Luna, o3, o1-preview | Codex OAuth, API Key, Responses, Stream/WS Relay |
| **Google Gemini** | Gemini 3.8 Flash, Gemini 3.1 Pro Preview, 2.5 Flash / Pro | AI Studio, Vertex AI, Gemini CLI, Antigravity accounts |
| **Tencent WorkBuddy** | DeepSeek V4.1 Flash (1M), GLM 5.3, Kimi K3, Hy4 Preview | Dual-realm reverse engineering (CN/Intl), OAuth, automated check-in |
| **OpenCode** | `step-5-preview-free`, `longcat-2.5-preview-free` (1M context) | Native OpenCode reverse protocol, WAF cloaking, zero-config free tier |
| **Kimi (Moonshot)** | Kimi K3 (2.8T 3T-class open flagship), K2.8 Preview | OAuth, Kimi Code subscription, Open Platform API Key |
| **xAI Grok** | Grok 4.7, Grok 4.7 Build Fast | Grok Build OAuth, xAI API Key |
| **Meta Muse** | Muse Spark 1.3, Muse Spark 1.2 | Meta login authentication, Meta Model API Key |
| **Devin** | SWE-2, GPT-6 Astra, Claude Fable 5.1 | `--devin-login` account reverse routing |
| **OpenAI Compatible** | Any standard OpenAI / Claude upstream (DeepSeek, OpenRouter, SiliconFlow) | Declarative configuration mapping |

---

## Getting Started

### 1. Build from Source

This project requires Go 1.26+ and supports static cross-compilation:

```bash
# Format code
gofmt -w .

# Build local binary
go build -o cli-proxy-api ./cmd/server

# Build static Linux amd64 binary without external dependencies
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o cli-proxy-api-linux ./cmd/server
```

### 2. Running

CLIProxyAPI auto-loads `config.yaml` (see `config.example.yaml`) and `.env` from the current working directory:

```bash
# Run server directly
./cli-proxy-api

# Run with interactive terminal UI
./cli-proxy-api --tui

# Specify custom config and disable browser auto-opening
./cli-proxy-api --config ./config.yaml --no-browser
```

### 3. Docker Deployment

```yaml
services:
  cliproxyapi:
    image: cliproxyapi:v8.0.28-unified
    container_name: cliproxyapi
    restart: unless-stopped
    ports:
      - "8315:8317"
    volumes:
      - ./config.yaml:/CLIProxyAPI/config.yaml
      - ./auths:/root/.cli-proxy-api
      - ./logs:/CLIProxyAPI/logs
      - ./plugins:/CLIProxyAPI/plugins
      - ./data:/CLIProxyAPI/data
      - ./static:/CLIProxyAPI/static
```

---

## Management V8 API Endpoints

| Method | Path | Description |
| :--- | :--- | :--- |
| `GET` | `/v8/management/credentials` | List managed credentials (filters out internal state files) |
| `GET` | `/v8/management/workbuddy/checkin` | Query check-in dashboard, account records, and engine status |
| `PUT` | `/v8/management/workbuddy/checkin` | Toggle automated check-in engine state (`{"enabled": bool}`) |
| `POST` | `/v8/management/workbuddy/checkin` | Trigger immediate check-in for accounts or toggle engine |
| `GET` | `/v8/management/plugins/workbuddy/quota` | Batch query WorkBuddy points balances and season VIP tiers |
| `GET` | `/v8/management/config` | View or hot-reload core proxy configuration |
| `POST` | `/v8/management/routing/cooldown/reset` | Emergency reset for cooldown sinks and quota rate-limits |

---

## Acknowledgements

Special thanks to the following open-source projects for their architectural inspiration, insights, and contributions:

- **[router-for-me/CLIProxyAPI](https://github.com/router-for-me/CLIProxyAPI)**: Foundational proxy architecture and cross-protocol normalization engine.
- **[Magpie](https://github.com/magpie-project)**: Architectural inspiration for persistent session affinity across restarts, sliding-window TTL, 429 sink scheduling, and zero-knowledge log redaction.
- **[WorkBuddy](https://github.com/workbuddy)**: Reverse-engineering insights into Tencent CodeBuddy / WorkBuddy dual-realm APIs, metering protocols, WAF cloaking, and check-in workflows.

---

## License

This project is open-source under the [MIT License](LICENSE).
