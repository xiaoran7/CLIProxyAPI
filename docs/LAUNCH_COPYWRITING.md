# Launch Copywriting Package

## 1. V2EX (`/go/share` | `/go/create`)
**Title**: Open-sourcing a Go proxy gateway that fixes long-thinking stream drops and multi-account cache invalidation for AI coding agents: CLI Proxy API

**Body**:
Two pain points show up constantly when you lean on Claude Code, Codex, and Cursor:
1. **Long-thinking stream drops (HTTP 524/504)**: a silent 30–60 second deep-thinking pause is easily cut off by Cloudflare/Nginx;
2. **Multi-account round-robin invalidates the Prompt Cache**: stateless rotation scatters a multi-turn conversation across credentials, so the cache hit rate collapses to zero and the bill balloons.

We built **CLI Proxy API** on Go 1.26:
- **SSE keepalive pump + loop breaker**: sends `: keep-alive` every 15 seconds during thinking pauses to hold the edge connection open, and a 256-sample sliding window automatically breaks runaway reasoning loops;
- **Session affinity across restarts (`session_affinity.json`)**: a sliding-window TTL pins a long session to a fixed credential, and a 429 sinks it into a cold pool with a smooth switchover;
- **Five quota scheduling strategies**: `round-robin`, `weighted-round-robin`, `fill-first`, `smart` (expiring-first), and `pace` (even pacing);
- **Native WorkBuddy & OpenCode suites**: supports Tencent WorkBuddy's CN/INTL dual realms (5.8.4 session-header spoofing, `WB_CN_CHAT_UPSTREAM` overseas egress, automatic check-in on Beijing time, live credit multipliers) plus OpenCode's 1M free models with no key configuration;
- **Management console embedded in a single binary (CPAMC)**: manage credentials, quota, and model blocking out of the box.

Repository: https://github.com/router-for-me/CLIProxyAPI

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
