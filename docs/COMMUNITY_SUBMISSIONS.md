# Community Submissions & Awesome PR Templates (权威列表收录物料)

This document contains pre-formatted submission proposals and Pull Request drafts to submit CLI Proxy API to curated Awesome lists and AI developer directories.

---

## 1. Submission: `awesome-claude-code` (or `awesome-anthropic`)

### Target Repository
- **Repo**: `https://github.com/hesreallygood/awesome-claude-code` (or active community awesome-claude lists)
- **Target Category**: `Proxies & Infrastructure` or `Tools & Extensions`

### Pull Request Title
```text
Add CLI Proxy API: Resilient gateway with session affinity and SSE keepalive
```

### Pull Request Description
```markdown
### Summary
This PR adds [CLI Proxy API](https://github.com/router-for-me/CLIProxyAPI) under the Proxies & Infrastructure section.

CLI Proxy API is an enterprise-grade Go gateway specifically built for coding agents like Claude Code CLI:
- Injects SSE `: keep-alive` comment frames to prevent Cloudflare HTTP 524 timeouts during long model thinking phases.
- Maintains persistent session affinity across restarts (`session_affinity.json`) to maximize prompt-cache hit rates across multi-account pools.
- Bridges OpenAI, Gemini, and domestic models to standard Anthropic and OpenAI schemas.

### Proposed Diff (README.md)
```markdown
- [CLI Proxy API](https://github.com/router-for-me/CLIProxyAPI) - Enterprise-grade AI proxy gateway in Go providing unified Claude/OpenAI protocol translation, persistent session affinity for prompt caching, and SSE keepalives during extended reasoning.
```
```

---

## 2. Submission: `awesome-ai-devtools`

### Target Repository
- **Repo**: `https://github.com/reorx/awesome-ai-devtools`
- **Target Category**: `API Gateways & Proxies`

### Pull Request Title
```text
Add CLI Proxy API to API Gateways
```

### Pull Request Description
```markdown
### Summary
Add CLI Proxy API to the API Gateways and Proxy tools category.

[CLI Proxy API](https://github.com/router-for-me/CLIProxyAPI) is a high-concurrency Go proxy server supporting:
- Bidirectional protocol translation between OpenAI, Anthropic Claude, and Google Gemini.
- Multi-strategy quota scheduling (Round-Robin, Weighted, Fill-First, Smart Quota, Pace Quota).
- Automated 429 rate-limit cooldown sink queue and zero-downtime credential reload.

### Proposed Diff (README.md)
```markdown
- [CLI Proxy API](https://github.com/router-for-me/CLIProxyAPI) - High-concurrency Go proxy providing unified OpenAI/Claude/Gemini protocol translation and multi-account load balancing for coding agents.
```
```

---

## 3. Submission: `awesome-llm-infrastructure`

### Target Repository
- **Repo**: `https://github.com/Hannibal046/Awesome-LLM`
- **Target Category**: `Tools & Infrastructure / API Gateways`

### Pull Request Title
```text
Add CLI Proxy API: Unified LLM Gateway & Load Balancer
```

### Pull Request Description
```markdown
### Summary
Propose adding CLI Proxy API to the API Gateway section.

- **GitHub**: https://github.com/router-for-me/CLIProxyAPI
- **Language**: Go 1.26+
- **License**: MIT
- **Features**: Universal OpenAI / Anthropic / Codex translation, persistent prompt-cache affinity, and streaming keepalive resilience.
```
