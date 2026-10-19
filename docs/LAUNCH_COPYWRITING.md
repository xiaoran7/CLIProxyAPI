# Launch Copywriting Package

## 1. V2EX (`/go/share` | `/go/create`)
**标题**：开源一个解决 AI 编程智能体长思考断流与多账号缓存失效的 Go 代理网关：CLI Proxy API

**正文**：
重度使用 Claude Code、Codex、Cursor 时常见两个痛点：
1. **长思考断流（HTTP 524/504）**：深度思考静默 30–60 秒易被 Cloudflare/Nginx 切断；
2. **多账号轮询导致 Prompt Cache 失效**：无状态轮询打散多轮对话，缓存命中率归零、账单暴增。

我们基于 Go 1.26 构建了 **CLI Proxy API**：
- **SSE Keepalive 心跳泵 + 循环熔断**：思考期每 15 秒发送 `: keep-alive` 保活边缘连接，256 滑窗自动熔断推理死循环；
- **跨重启会话亲和（`session_affinity.json`）**：滑动窗口 TTL 绑定长会话至固定凭据，遇 429 自动沉入冷沉淀池并平滑切换；
- **五种配额调度**：支持 `round-robin`、`weighted-round-robin`、`fill-first`、`smart`（临期优先）、`pace`（步调均衡）；
- **WorkBuddy & OpenCode 原生套件**：支持腾讯 WorkBuddy 国内/国际双域（5.8.4 会话头伪装、`WB_CN_CHAT_UPSTREAM` 海外出口、北京时间自动打卡、实时倍率显示）及 OpenCode 1M 免费模型免配直通；
- **单二进制内嵌管理台（CPAMC）**：开箱即用管理凭据、配额与模型屏蔽。

仓库：https://github.com/router-for-me/CLIProxyAPI

---

## 2. Reddit (`r/ClaudeAI` | `r/LocalLLaMA`)
**Title**: CLI Proxy API: Open-source Go gateway fixing Claude Code stream timeouts and preserving Prompt Cache across multi-account pools

**Body**:
CLI Proxy API (Go 1.26+) solves two major friction points for Claude Code CLI, Codex, and Cursor:
- **SSE Keepalive Pumper & LoopGuard**: Injects `: keep-alive` frames every 15s during extended thinking pauses to prevent Cloudflare HTTP 524/504 drops, and aborts repetitive reasoning loops.
- **Persistent Session Affinity**: Pins multi-turn conversations to the same credential across restarts (`session_affinity.json`) with a 429 cooldown sink pool and 5 routing strategies (`smart`, `pace`, `fill-first`, `weighted`, `round-robin`).
- **Universal Translation & Web Console**: Bridges OpenAI, Anthropic, Gemini, Codex WS, WorkBuddy, and OpenCode with an embedded management UI.

GitHub: https://github.com/router-for-me/CLIProxyAPI

---

## 3. Hacker News (`Show HN`) & X (Twitter)
- **Show HN: CLI Proxy API – Resilient Go LLM gateway for coding agents**: Bidirectional OpenAI/Claude/Codex WS translation, 15s SSE keepalives against 524 timeouts, persistent prompt-cache session affinity, Smart/Pace quota routing, and HMAC-SHA256 zero-knowledge redaction. (`https://github.com/router-for-me/CLIProxyAPI`)
