# GitHub Discussions Seed Q&A Library

## 1. Preventing Cloudflare HTTP 524 / Gateway 504 Timeouts During Extended Thinking
**Q**: How to prevent Cloudflare HTTP 524 / gateway 504 timeouts when using Claude Code or o3 with long reasoning blocks?

**A**: CLI Proxy API includes a built-in **SSE Keepalive Pumper** and **Reasoning Loop Guard**:
- **SSE Keepalive Pumper**: Emits compliant `: keep-alive\n\n` comment frames every 15s while upstream reasoning is silent, resetting edge proxy (Cloudflare/Nginx) idle timers without affecting SSE JSON parsing.
- **Reasoning Loop Guard**: Detects repetitive 256-window token cycles and aborts stuck streams (`stream_loop_detected`) to protect quota.

---

## 2. Multi-Account Load Balancing with Prompt-Cache Session Affinity
**Q**: How to balance load across multiple accounts while preserving Claude Code and Cursor prompt-cache hit rates?

**A**: Configure **Persistent Session Affinity** (`session_affinity.json`) with **Smart/Pace Quota Routing** and the **429 Sink Pool**:
```yaml
routing:
  strategy: "smart" # round-robin | weighted-round-robin | fill-first | smart | pace
  session-affinity: true
  session-affinity-ttl: "1h"
  session-affinity-subagents: true
  session-affinity-rebind-on: "any"
```
- Binds conversation session hashes (`session_id`, `X-Magpie-Session`, `X-OpenCode-Session`, `X-Cursor-Session-Id`, or first-user-turn anchor) to the same credential across restarts.
- On HTTP 429, demotes the throttled credential to a cooldown sink queue and transparently migrates the turn to a healthy account.

---

## 3. Tencent WorkBuddy / CodeBuddy Dual-Realm Routing & Daily Check-in
**Q**: How to configure Tencent WorkBuddy / CodeBuddy dual-realm accounts (`copilot.tencent.com` & `workbuddy.ai`), bypass WAF `11128`/`11101` errors, and automate daily check-in?

**A**:
1. **CLI QR Login**: Run `./cli-proxy-api --workbuddy-login` to authenticate via WeChat/WeCom and persist credentials under `auths/` (`workbuddy-cn` or `workbuddy`).
2. **Overseas Egress & WAF Cloaking**: Set `WB_CN_CHAT_UPSTREAM=https://www.workbuddy.cn` if running on an overseas VPS to avoid TLS RST drops. Outbound calls automatically inject 5.8.4 conversation headers (`X-Conversation-ID`, W3C/B3 traces), prepend required system prompts (`11128` guard), and enforce `"stream": true` with transparent SSE aggregation (`11101` bypass).
3. **Automated Check-in & Multipliers**: Background CST `CheckinEngine` claims daily credits (`workbuddy_checkins.json`), while `/v1/models` exposes live credit multipliers (`workbuddy/claude-opus-5-5:3.33x`).
