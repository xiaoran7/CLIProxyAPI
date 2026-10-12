# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/).

## [Unreleased]

### Added
- **Native WorkBuddy / CodeBuddy Provider & Dual Realm Support**:
  - Implemented dedicated `WorkBuddyExecutor` (`internal/runtime/executor/workbuddy_executor.go`) with OpenAI-compatible chat completions routing to `/v2/chat/completions`.
  - Added dual-realm resolution (`internal/auth/workbuddy/workbuddy.go`) supporting both international (`intl`, `www.workbuddy.ai`) and China mainland (`cn`, `copilot.tencent.com`) backends with distinct domains, headers, and endpoints.
  - Implemented deterministic device fingerprinting engine (`DeriveID = MD5(salt:uid)`) generating consistent `X-Machine-ID`, `X-Session-ID`, and `X-Request-ID` headers to prevent multi-account risk correlation.
  - Implemented WAF prompt cloaking and header sanitization (`internal/runtime/executor/helps/workbuddy_cloaking.go`):
    - Sanitizes sensitive Claude Code system instructions to prevent Tencent WAF error `11128` ("Illegal API invocation").
    - Strips foreign billing and client headers (`x-anthropic-billing-header`, `anthropic-version`, etc.).
    - Backfills `reasoning_content` in multi-turn assistant messages for DeepSeek thinking models to avoid 400 validation failures.
  - Added singleflight-protected token refresh (`/v2/plugin/auth/token/refresh`) with envelope unwrapping and disk persistence (`internal/auth/workbuddy/token.go`).
  - Added native static model catalogs for both realms (`internal/registry/workbuddy_models.go`) exposing `deepseek-v4.1-flash`, `gpt-6-astra`, `claude-fable-5-1`, `hy4-preview-f`, `glm-5.3`, and `kimi-k3`.
  - Registered `workbuddy`, `workbuddy-intl`, and `workbuddy-cn` in executor registry, model registry, and file auth synthesizer.
- **AOCI Code Cognition & Baseline Assets**:
  - Initialized AOCI v0.1.0-rc18 code cognition layer (`aoci.txt`, `aoci.meta.txt`, `aoci.code.txt`, `.gitattributes`).
  - Established project baseline across 1,882 managed files with Claude Code MCP hooks and settings integration.
- **Native OpenCode Provider & Free Tier Support**:
  - Implemented OpenCode Zen reverse-engineered fingerprint engine (`internal/runtime/executor/opencode_cloaking.go`):
    - Injects dynamic 48-bit bitwise-inverted Base62 session and request IDs (`ses_` and `msg_`).
    - Supplies dummy agent tool schemas (`read`, `bash`, `write`) to completely eliminate the `FreeTierError` cloud gate restriction.
    - Implemented transparent server-side streaming aggregation to support both SSE streaming and non-streaming HTTP client calls.
  - Added native OpenCode model catalog (`internal/registry/opencode_models.go`) exposing 1M context free models (`step-5-preview-free`, `longcat-2.5-preview-free`, `mimo-v2.6-flash-free`, `ling-3.1-flash-free`, etc.) and Zen flagship models without requiring manual model entries in `config.yaml`.
  - Added native OpenRouter baseline model catalog (`internal/registry/openrouter_models.go`) enabling zero-configuration model discovery for OpenRouter endpoints.
  - Registered `opencode` as an auto-bound executor and model catalog provider in `sdk/cliproxy/service_executors.go` and `sdk/cliproxy/service_models.go`.
  - Hardened OpenCode fingerprinting against external IDE/agent tool contamination by merging mandatory `read`/`bash`/`write` schemas into caller tools and enforcing fingerprint header application order.
