# Launch Copywriting Package (全平台宣发物料包)

This document contains publication-ready copywriting tailored for specific developer communities. Copy and paste into the respective platforms upon release.

---

## 1. V2EX (程序员 / 节点：/go/share 或 /go/create)

**标题**：做了一个解决 AI 编程智能体断流与多账号缓存失效的 Go 代理网关：CLI Proxy API

**正文**：
各位 V 友大家好，

在日常重度使用 Claude Code CLI、Codex、Cursor 或 Aider 进行工程开发时，经常遇到两个难以忍受的痛点：
1. **长思考断流（HTTP 524 / 504）**：当模型开启深度思考（如 Claude Sonnet 3.7 / 5.5 或 o3）时，往往需要沉思 30-60 秒才输出首个 token。中间由于 TCP 链路静默，经过 Cloudflare 或边缘网关时极容易被直接判超时切断；
2. **多账号负载均衡导致 Prompt Cache 全废**：为了分摊配额，很多朋友挂了多个 API Key 或账号轮询。但传统的无状态 Round-Robin 使得同一个对话的每一轮提问都被打散到不同账号上，Anthropic 的 Prompt Cache 命中率直接跌到 0，不仅响应极慢，Token 费用更是暴增 10 倍。

为了彻底解决这些工程痛点，我们基于 Go 1.26 开发并重构了这个面向 Coding Agents 的生产级代理网关 —— **CLI Proxy API**：

### 核心亮点
- **SSE Keepalive 心跳泵**：在模型思考静默期，每 15 秒主动向客户端灌入合规的 `: keep-alive` 注释帧，既保活了边缘代理连接，又不影响前端客户端的 JSON 规范解析；
- **跨重启会话亲和（Session Affinity）**：通过会话标识将同一上下文持久化绑定到同一账号，并记录在 `session_affinity.json` 中，重启不丢状态，最大化维持 90%+ 的 Prompt Cache 命中率；
- **五种企业级调度策略**：支持 Round-Robin、权重轮询、顺序打满（Fill-First）、配额到期优先（Smart Quota）与速率平滑（Pace Quota），遇到 429 自动沉入动态冷却队列无缝重试；
- **国产代码智能体深度纳管**：
  - 腾讯 WorkBuddy / CodeBuddy 国内版与国际版原生双区自动识别；
  - 内建确定性 IDE 设备指纹（WAF 穿透防护）；
  - 内建北京时间自动签到与领额度后台引擎（CheckinEngine）；
  - 零配置直通 OpenCode 1M 上下文模型；
- **全内置管理控制台（CPAMC）**：单二进制内嵌可视化 Web 运维面板，凭据、配额、打卡状态一目了然。

### 快速上手
```bash
git clone https://github.com/router-for-me/CLIProxyAPI.git
cd CLIProxyAPI
go build -o cli-proxy-api ./cmd/server
cp config.example.yaml config.yaml
./cli-proxy-api
```

把 Claude Code 的 Base URL 指向 `http://127.0.0.1:8317` 即可无感体验：
```bash
export ANTHROPIC_BASE_URL="http://127.0.0.1:8317"
claude
```

开源地址：https://github.com/router-for-me/CLIProxyAPI
欢迎大家 Star 与提 PR 交流！

---

## 2. Reddit (r/ClaudeAI / r/LocalLLaMA / r/Cursor)

**Post Title**: CLI Proxy API: Open-source Go gateway that fixes Claude Code stream timeouts and preserves Prompt Cache across multi-account pools

**Post Body**:
Hey everyone,

If you are using Claude Code CLI, Cursor, or Aider heavily, you've likely hit two major friction points:
1. **Edge Gateway Timeouts (HTTP 524 / 504)**: During extended thinking pauses (e.g. Claude Sonnet 3.7/5.5 thinking mode or o3 reasoning), models can sit silent for 30–60+ seconds before emitting tokens. Edge proxies like Cloudflare kill the idle socket.
2. **Cache Thrashing on Multi-Account Pools**: When distributing load across multiple Anthropic/OpenAI accounts, naive round-robin shuffles consecutive turns across different credentials, destroying Anthropic prompt cache prefixes and spiking token bills by up to 10x.

We built and open-sourced **CLI Proxy API** (written in Go 1.26+) to address these exact problems.

### What it does:
- **SSE Keepalive Pumper**: Injects compliant `: keep-alive` SSE comment frames every 15 seconds during model thinking pauses, keeping edge proxies alive without corrupting client JSON parsing.
- **Persistent Session Affinity**: Tracks conversation keys and binds ongoing turns to the same upstream credential across restarts (`session_affinity.json`), maximizing prompt cache reuse.
- **Smart Quota & 429 Sink Pools**: Supports 5 routing strategies (Round-Robin, Weighted, Fill-First, Smart Quota by expiry, Pace Quota). If an account hits HTTP 429, it sinks into a backoff queue while the turn seamlessly retries on a healthy account.
- **Universal Protocol Translation**: Translates between OpenAI (`/v1/chat/completions`, `/v1/responses`), Anthropic (`/v1/messages`), Google Gemini, and Codex WebSocket streaming.
- **Embedded Web Dashboard**: Fully bundled CPAMC console for inspecting quotas, latency, and credentials.

### Quick Start:
```bash
git clone https://github.com/router-for-me/CLIProxyAPI.git
cd CLIProxyAPI
go build -o cli-proxy-api ./cmd/server
./cli-proxy-api --claude-login
```

Point Claude Code:
```bash
export ANTHROPIC_BASE_URL="http://127.0.0.1:8317"
claude
```

GitHub: https://github.com/router-for-me/CLIProxyAPI

Feedback and PRs welcome!

---

## 3. Hacker News (Show HN)

**Title**: Show HN: CLI Proxy API – Resilient LLM proxy gateway for coding agents in Go

**Body**:
CLI Proxy API is an open-source, high-concurrency API gateway and protocol translator written in Go 1.26+. It sits between coding agents (Claude Code CLI, OpenAI Codex CLI, Cursor, Aider) and upstream model providers (Anthropic, OpenAI, Google, Grok, and domestic providers).

### Architectural Highlights:
- **Protocol Normalization**: Bidirectional mapping between OpenAI Chat/Responses, Anthropic Messages, and Codex WebSocket schemas.
- **Stream Resilience**: SSE keepalive heartbeats (`: keep-alive`) mitigate Cloudflare 524/504 timeouts during long model reasoning phases. A reasoning loop circuit breaker halts runaway token loops.
- **Session Affinity**: Preserves Anthropic prompt-cache prefixes via persistent conversation binding across restarts.
- **Scheduling Strategies**: Round-Robin, Weighted Round-Robin, Fill-First, Smart Quota (expires-first), and Pace Quota, combined with an automatic 429 rate-limit sink pool.
- **Zero-Knowledge Sanitization**: Sensitive authorization tokens, cookies, and prompt bodies are strictly redacted before logging.

Repository: https://github.com/router-for-me/CLIProxyAPI
Technical Specification: https://github.com/router-for-me/CLIProxyAPI/blob/main/llms-full.txt

---

## 4. X (Twitter / 英文推文)

**Tweet**:
🚀 Tired of Claude Code dropping connections during long thinking phases? Or losing prompt cache hits when balancing across multiple accounts?

Meet **CLI Proxy API** — an enterprise-grade Go gateway for coding agents:
⚡ SSE Keepalive Pumper against HTTP 524 timeouts
🎯 Persistent Session Affinity for 90%+ prompt cache reuse
⚖️ Smart Quota & 429 Cooldown Sink Pools
🔄 Universal OpenAI ⟷ Claude ⟷ Codex protocol translation

100% open source & production ready:
👉 https://github.com/router-for-me/CLIProxyAPI

#Golang #ClaudeCode #OpenAI #Anthropic #DevOps #CodingAgents
