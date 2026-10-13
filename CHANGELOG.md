# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/).

## [Unreleased]

### Added
- **WorkBuddy Dedicated Quota Tabs & Provider Split**:
  - Split WorkBuddy international (`workbuddy`) and China mainland (`workbuddy-cn`) realms out of generic `Plugin` quota grouping in CPAMC (`static/management.html` and `internal/managementasset/assets/management.html`).
  - Added dedicated provider definitions to `SN`, `lne`, `dne`, and `DA` so `WorkBuddy (en)` and `WorkBuddy (cn)` render as standalone tabs with official green robot avatars, localized titles, and independent credential counts.
  - Mapped store selectors (`pluginQuota`) and grouped cache cleanup by store setter (`setPluginQuota`) to prevent shared quota state eviction between WorkBuddy realms.
  - Dynamically hidden the generic `Plugin` tab when no non-WorkBuddy plugin credentials are registered (`R.plugin === 0`).
  - Extended Quota Timeline window calculation (`MO` and `uP`) to recognize `workbuddy` and `workbuddy-cn` quota groups and buckets.
- **WorkBuddy Quota Provider & Free Tier Labeling**:
  - Implemented native `workbuddy.QueryQuota()` (`internal/auth/workbuddy/quota.go`) querying Tencent `/v2/plugin/user/points` for real-time account points balance, VIP subscription tier, and daily free tier status.
  - Wired native WorkBuddy quota provider into management handlers (`/v0/management/quota/fetch` and `/v0/management/plugins/:id/quota`) to enable one-click quota inspection in CPAMC.
  - Bound `supports_quota` and `quota_provider` for `workbuddy` and `workbuddy-cn` auth files in management response (`internal/api/handlers/management/auth_files.go` and `plugins.go`).
  - Differentiated base models from `-free` model aliases in `DisplayName`: base models (`deepseek-v4.1-flash`, `hy4-preview-f`, `glm-5.3-flash`) labeled with `(WorkBuddy)` / `(CodeBuddy)`, and dedicated `-free` aliases labeled with `(Free / 1M)` (`internal/registry/workbuddy_models.go`).
  - Added native `-free` model alias resolution (`deepseek-v4.1-flash-free`, `hy4-preview-f-free`, `glm-5.3-flash-free`) with transparent upstream model name unwrapping in executor prompt cloaking (`internal/runtime/executor/helps/workbuddy_cloaking.go`).
  - Injected WorkBuddy official green cat-eared robot avatar into CPAMC `RD` data URI map, `Zv` alias map, and `$x` provider logo dictionary across `static/management.html` and embedded binary assets (`internal/managementasset/assets/management.html`).
  - Added localized filter labels (`filter_workbuddy` and `filter_workbuddy-cn`) across all CPAMC locale bundles (Chinese, English, etc.) to eliminate fallback unlocalized "Workbuddy" text.
  - Applied polished border radius styling to tab icons for consistent avatar rendering.
- **Upstream Sync (v8.0.25 & Latest Mainline)**:
  - Merged upstream `v8.0.25` commits through `696bbaee`.
  - Added native Codex "Sign in with ChatGPT" (SIWC) OAuth authentication binding (`internal/auth/codex/siwc_auth.go` and `internal/cmd/siwc_bind.go`).
  - Added session affinity rebind control (`session-affinity-rebind-on`) across upstream executors.
  - Added plugin host quota provider resolution and v8 auth file quota provider bindings.
  - Fixed Gemini schema unsupported dependent keywords sanitization and Codex fallback default model template when `gpt-5.5` is omitted.
  - Added native Claude and Codex Responses API `tool_search` support (`upstream.claude.tool-search` and `native-tool-search` configs).
  - Synchronized Antigravity streaming fixes, Gemini media function response trailing turn adjustments, and Codex over-length tool call ID handling.
  - Refactored Devin and Meta model getters in registry while maintaining compatibility with native OpenCode and WorkBuddy static catalogs.
- **Native WorkBuddy / CodeBuddy Provider & Dual Realm Support**:
  - Implemented dedicated `WorkBuddyExecutor` (`internal/runtime/executor/workbuddy_executor.go`) with OpenAI-compatible chat completions routing to `/v2/chat/completions`.
  - Added dual-realm resolution (`internal/auth/workbuddy/workbuddy.go`) supporting both international (`intl`, `www.workbuddy.ai`) and China mainland (`cn`, `copilot.tencent.com`) backends with distinct domains, headers, and endpoints.
  - Implemented deterministic device fingerprinting engine (`DeriveID = MD5(salt:uid)`) generating consistent `X-Machine-ID`, `X-Session-ID`, and `X-Request-ID` headers to prevent multi-account risk correlation.
  - Implemented WAF prompt cloaking and header sanitization (`internal/runtime/executor/helps/workbuddy_cloaking.go`):
    - Sanitizes sensitive Claude Code system instructions to prevent Tencent WAF error `11128` ("Illegal API invocation").
    - Strips foreign billing and client headers (`x-anthropic-billing-header`, `anthropic-version`, etc.).
    - Backfills `reasoning_content` in multi-turn assistant messages for DeepSeek thinking models to avoid 400 validation failures.
  - Added singleflight-protected token refresh (`/v2/plugin/auth/token/refresh`) with envelope unwrapping and disk persistence (`internal/auth/workbuddy/token.go`).
  - Added native interactive OAuth login authorization flow (`internal/auth/workbuddy/login.go`):
    - Implemented session initiation (`StartLogin`), single/polling authorization check (`PollToken`, `WaitForAuthorization`), and JWT claim parsing (`ParseJWTClaims`).
    - Added management endpoints (`/v0/management/workbuddy-auth-url`, `/v0/management/workbuddy-cn-auth-url`, `/v8/management/oauth/auth-url?provider=workbuddy`) with automatic session tracking and token persistence to `auths/workbuddy-*.json`.
    - Integrated native OAuth discovery in management plugins list so CPAMC (`management.html`) displays WorkBuddy and CodeBuddy cards in the OAuth tab with one-click login.
    - Added WorkBuddy and CodeBuddy options to TUI OAuth tab and Gate admin control panel.
  - Added native static model catalogs for both realms (`internal/registry/workbuddy_models.go`) exposing `deepseek-v4.1-flash`, `gpt-6-astra`, `claude-fable-5-1`, `hy4-preview-f`, `glm-5.3`, and `kimi-k3`.
  - Registered `workbuddy`, `workbuddy-intl`, and `workbuddy-cn` in executor registry, model registry, and file auth synthesizer.
  - Standardized WorkBuddy edition names to `WorkBuddy (en)` and `WorkBuddy (cn)` in management plugin metadata, OAuth controllers, and TUI menus.
  - Unified WorkBuddy avatar icon to official green cat-eared robot logo across all editions and OAuth login cards.
  - Elevated OpenCode as a top-level provider in CPAMC control panel frontend assets (`management.html`) with dedicated brand card, filtering, and independent brand configuration.
  - Upgraded embedded CPAMC management control panel to latest upstream v1.26.2, seamlessly merging native OpenCode brand cards (geometric square icon, standalone workbench configuration) and WorkBuddy native support (both `WorkBuddy (en)` and `WorkBuddy (cn)` OAuth login cards, robot avatar icons, auth-file filter tabs, and preset mappings) without discarding any previous customizations, while bringing in upstream Claude extra usage/dollar balance trackers, plugin quota routing fixes, and Korean/Vietnamese locales.
  - Embedded customized `management.html` directly into the binary bundle (`internal/managementasset/embedded.go`) with zero network dependency, ensuring OpenCode brand cards and WorkBuddy assets work out-of-the-box on clean installations.
  - Added HTTP cache busting headers (`no-cache, no-store, must-revalidate`) for `/management.html` to prevent stale browser caching.
  - Protected customized CPAMC assets from upstream auto-update overwrite when using default repository.
- **AOCI Zero-Footprint Local Cognition Governance**:
  - Enforced strict zero-footprint repository boundary for AOCI cognition layer per `aoci-setup` policy.
  - Standardized `.gitignore` with full AI tooling and local cognition ignore rules (`aoci.*`, `.aoci/`, `.mcp.json`, `AGENT.md`, etc.).
  - Untracked all AOCI local state and baseline files from Git index without data loss while preserving offline local cognition readiness.
  - Sanitized `AGENTS.md` by stripping private AOCI operational blocks for clean open-source compliance.
- **Native OpenCode Provider & Free Tier Support**:
  - Implemented OpenCode Zen reverse-engineered fingerprint engine (`internal/runtime/executor/opencode_cloaking.go`):
    - Injects dynamic 48-bit bitwise-inverted Base62 session and request IDs (`ses_` and `msg_`).
    - Supplies dummy agent tool schemas (`read`, `bash`, `write`) to completely eliminate the `FreeTierError` cloud gate restriction.
    - Implemented transparent server-side streaming aggregation to support both SSE streaming and non-streaming HTTP client calls.
  - Added native OpenCode model catalog (`internal/registry/opencode_models.go`) exposing 1M context free models (`step-5-preview-free`, `longcat-2.5-preview-free`, `mimo-v2.6-flash-free`, `ling-3.1-flash-free`, etc.) and Zen flagship models without requiring manual model entries in `config.yaml`.
  - Added native OpenRouter baseline model catalog (`internal/registry/openrouter_models.go`) enabling zero-configuration model discovery for OpenRouter endpoints.
  - Registered `opencode` as an auto-bound executor and model catalog provider in `sdk/cliproxy/service_executors.go` and `sdk/cliproxy/service_models.go`.
  - Hardened OpenCode fingerprinting against external IDE/agent tool contamination by merging mandatory `read`/`bash`/`write` schemas into caller tools and enforcing fingerprint header application order.
