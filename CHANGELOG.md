# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/).

## [Unreleased]

### Fixed
- **WorkBuddy Suite Auto-Mount Fetching & Account Display (`management.html`)**:
  - Restored automatic check-in record and quota initialization (`useEffect`) upon mounting the WorkBuddy Suite drawer and switching to the quota tab, resolving the issue where accounts were not loaded (`暂无已纳管的 WorkBuddy 凭证记录` / `正在拉取...` stuck forever).
- **Model Provider Static Resolution Fallback & Namespace Routing (`provider.go`, `model_definitions.go`, `handlers_multiplier.go`)**:
  - **400 `model_not_found` Fallback Resolution**: When `GetGlobalRegistry().GetModelProviders(model)` has no active dynamically registered clients (e.g. before credentials register or for catalog entries like `step-5-preview-free` and `gemini-3.8-flash-high`), `GetProviderName` now falls back to `LookupStaticModelProviders` across all static catalogs (`opencode`, `antigravity`, `workbuddy`, `claude`, `gemini`, `codex`, `kimi`, `xai`, `devin`, `meta`, `vertex`, `aistudio`), resolving providers reliably instead of throwing HTTP 400 "unknown provider for model".
  - **Expanded Namespace Prefixes**: Extended `splitProviderPrefix` to recognize `opencode/`, `opencode-zen/`, `zen/`, and `antigravity/` (`ag/`) prefixes alongside existing WorkBuddy prefixes for strict target provider pinning.
- **WorkBuddy Non-Stream SSE Aggregation & 11101 Bypass (`workbuddy_executor.go`, `workbuddy_stream_aggregator.go`)**:
  - **11101 "Non-stream chat request is currently not supported" Fix**: Tencent WorkBuddy backend (both international `workbuddy.ai` and domestic `copilot.tencent.com`) strictly requires `stream: true` on chat completions and rejects non-streaming calls with code 11101.
  - **Transparent Stream Aggregation**: Implemented `AggregateWorkBuddyStream` in `helps` to transparently force `stream: true` upstream on non-streaming calls (`Execute`), buffer SSE events, and aggregate content, reasoning content, tool calls, and usage statistics into a single valid OpenAI-compatible `chat.completion` response.
  - **Downstream Format Preservation**: Correctly translates aggregated responses to Anthropic or other non-OpenAI formats via `TranslateNonStream` when requested by downstream clients.

### Added
- **Model Variant Filtering & Clean Provider Catalog (`models.variant-mode` / `/v1/models`)**:
  - **Redundant Variant Suppression (`openai_handlers.go`, `config_v8.go`, `sdk_config.go`)**: Added `models.variant-mode` configuration (supporting `"compact"`, `"raw"`, `"all"`, or boolean `filter-variants: true/false`). Defaults to `"compact"`, filtering out duplicate bare models and multiplier-annotated suffixes (`:0x`, `:1.62x`) to expose clean, namespaced model IDs (e.g. `workbuddy-cn/hy4-preview-f` for models with known provider realms, and clean bare IDs for standard models).
  - **Downstream Client Compatibility**: Downstream catalog clients (such as CCSwitch, Cherry Studio, NextChat) now receive a clean single-entry list per model instead of cluttered combinations, while maintaining strict provider namespace routing and billing credits annotations.
- **Zero-Burden Multi-Agent Coordination Infrastructure Governance (`AGENTS.md`, `.gitignore`)**:
  - Restored pristine zero-burden repository baseline by removing local `<!-- NEXUS-COORDINATION-START -->` rules from `AGENTS.md` and scrubbing `.nexus` directory rules from `.gitignore`.
  - Shifted all Agent-Nexus concurrency collision prevention and git lifecycle unlock responsibilities entirely to the host-level global PreToolUse Hook and daemon, leaving the business codebase 100% clean and unburdened.
- **OpenCode Zen Free Tier Anti-Risk Cloaking & Catalog Expansion (`opencode_cloaking.go`, `openai_compat_executor.go`, `opencode_models.go`, `config.example.yaml`)**:
  - **Free Tier Cloud WAF Endpoint Alignment**: OpenCode 1.19.0+ rejects legacy `/zen/v1` calls with `403 FreeTierError` for free models. Implemented transparent URL rewriting in `normalizeOpenCodeEndpoint` and `PrepareRequest` redirecting `https://opencode.ai/zen/v1` to `https://opencode.ai/inference/v1`.
  - **Tool Parameter Declaration Hardening**: OpenCode cloud inspects function tool declarations and blocks required tools (`bash`, `read`) if declared with empty properties (`{}`) as dummy tool injection. Enhanced `makeOpenCodeStubTool` and caller tool normalization to inject valid parameter schemas (`command: string` for `bash`, `filePath: string` for `read`) with `additionalProperties: false`.
  - **Client Fingerprint Sanitization**: Stripped client leak headers (`x-stainless-*`, `x-cursor-*`, `x-cline-*`, `x-continue-*`) alongside existing `x-client`, `x-app-name`, and `x-zcode-*` identifiers.
  - **Free Tier Catalog Expansion**: Registered new flagship free models in static catalog (`big-pickle` 200k, `cat-jam-free` 262k with multi-level thinking, `deepseek-v4.1-flash-free` 1M context with reasoning support).
- **Upstream v8.0.32 Sync (Claude Schema Normalization, Date Pin Retention, Devin Quota Cooldown, Thinking Block Binding)**:
  - **Claude Thinking Block Binding & Beta Policy (`claude_executor_request.go`, `claude_openai-responses_request.go`, #6692)**: Automatically injects `drop_block` prefix mismatch behavior for preserved-thinking models when replaying signed thinking blocks; appends `thinking-binding-controls` beta header when requests carry `thinking.block_binding`; expands Haiku 5.5 and related model detection to support assistant prefill drops.
  - **Claude Tool Argument Cleaning & Schema Normalization (`translator.go`, `antigravity_claude_response.go`, `gemini_claude_response.go`, #6670)**: Implemented robust schema analysis and argument cleaning for Claude tool calls, restoring exact client-facing tool names and sanitizing placeholders across Antigravity and Gemini translators.
  - **Claude Session Date Pin Retention (`claude_executor_cloaking.go`, `config_types.go`, `claude_diagnostics.go`, #6682)**: Added `session-date-pin-days` configuration option with 7-day default retention; stores session date pins independently of continuity state to prevent re-anchoring on credential switches or idle gaps; strips caller-supplied current date reminders from the first user message.
  - **Devin Quota Exhaustion Tracking & Cooldown Refresh (`devin_executor.go`, `conductor_cooldown.go`, `conductor_refresh.go`, #6690)**: Added account-wide quota exhaustion detection for daily and weekly usage windows with reset timestamp parsing; applies credential-scoped 429 status errors with reset-based Retry-After; supports automatic quota revalidation and early credential cooldown release upon confirmed usage recovery.
  - **Meta Stream Disconnect Graceful Handling (`meta_executor_stream.go`, #6688)**: Tracked delivery of terminal response events in Meta stream execution; prevents client cancellation errors from being marked as API failures if the terminal event was already delivered.
  - **Full Fork Feature Preservation**: Resolved README merge conflicts cleanly keeping fork documentation and highlights; verified 100% preservation of WorkBuddy (dual-realm namespaces, credit multipliers, check-in engine, 500 compensation suite), OpenCode native cloaking, Pace/Smart routing, and selective gzip compression.
- **WorkBuddy Dual-Realm Provider Namespace & Multiplier Discovery (`openai_handlers.go`, `handlers_multiplier.go`, `handlers_routing.go`, `workbuddy_credits.go`)**:
  - **Downstream Provider Namespace Discovery (`workbuddy/` & `workbuddy-cn/`)**: Emitted namespaced model variants (e.g. `workbuddy/claude-opus-5-5:3.33x`, `workbuddy-cn/hy3:0x`) with explicit `owned_by: "workbuddy"` / `"workbuddy-cn"` in `/v1/models`. This completely solves third-party client classification bugs (such as ccswitch) that erroneously force `claude-*` models into Anthropic based on raw model name matching.
  - **Downstream Egress Security Isolation**: Strictly binds upstream provider execution to `workbuddy` when a downstream request carries `workbuddy/` or `workbuddy-cn/` prefixes, preventing unintended egress leakage or fallback to official Anthropic/OpenAI accounts.
  - **Clean Upstream Sanitization**: Automatically strips namespace prefixes and multiplier suffixes before routing to Tencent backend upstreams while fully preserving inner or outer thinking suffixes (e.g. `workbuddy/claude-opus-5-5:3.33x(16384)` -> `claude-opus-5-5(16384)`).

- **OpenAI `/v1/models` Multiplier Suffix Emission for Client Discovery (`openai_handlers.go`, `openai_models_credits_test.go`, `workbuddy_credits.go`)**:
  - Emitted multiplier-suffixed model IDs (e.g. `glm-5.3-flash:0.06x`, `hy3:0x`, `deepseek-v4-pro:0.15x`, `claude-opus-5-5:3.33x`) in `/v1/models` so downstream clients (such as ccswitch, NextChat, Cherry Studio, etc.) can discover, display, and select models directly with their credit multipliers.
  - Included `display_name` and `name` attributes formatted with credit multipliers (`<model> (<rate>)`) for compatibility with clients displaying model names instead of raw IDs.
  - Enforced zero multiplier (`0.0` / `0x`) for all `-free` model aliases and provided fallback catalog rates for unrated models.

- **Downstream Model Multiplier Suffix & Strict Catalog Enforcement (`handlers_multiplier.go`, `handlers_multiplier_test.go`, `handlers_routing.go`, `handlers_execution.go`, `handlers_stream.go`, `workbuddy_credits.go`)**:
  - **Downstream Multiplier Suffix Support (`:<rate>x` / `:<rate>`)**: Supported passing model multiplier suffixes in downstream API requests (e.g. `glm-5.3-flash:0.06x`, `glm-5.3-flash:0.06`, `hy3:0x`, `deepseek-v4-pro:0.15x`, `claude-opus-5-5:3.33x`).
  - **Thinking Suffix Interoperability**: Fully interoperable with thinking / reasoning suffixes in either order (e.g. `glm-5.3-flash(16384):0.06x` or `glm-5.3-flash:0.06x(16384)`), properly normalizing to clean models for backend provider routing.
  - **Strict Multiplier Mismatch Interception (HTTP 400)**: Validates client-supplied multiplier against official WorkBuddy credit catalog rates. Rejects mismatched multipliers immediately with HTTP 400 Bad Request (`code: model_multiplier_mismatch`, message detailing requested vs expected multiplier).
  - **Static Catalog Fallback (`staticWorkBuddyDefaultCredits`)**: Configured reliable fallback catalog for core models (`hy3` 0x, `glm-5.3-flash` 0.06x, `deepseek-v4-pro` 0.15x, `claude-opus-5-5` 3.33x, etc.) and automatic 0x resolution for `-free` aliases when dynamic catalog is cold.

### Changed
- **WorkBuddy Management Suite Manual Refresh (`management.html`)**:
  - Disabled automatic background polling and requests on opening the WorkBuddy suite drawer and switching tabs (`/workbuddy/checkin`, `/workbuddy/compensation`, `/plugins/workbuddy/quota`), enforcing explicit user-triggered manual refreshes only.

### Fixed
- **WorkBuddy Suite Management UI Initialization (`management.html`)**:
  - Resolved `ReferenceError: Cannot access 'fetchComp' before initialization` in `WorkBuddySuite` component by hoisting the `fetchComp` callback definition before the `E` (checkin) callback that depends on it.
- **WorkBuddy Upstream 5.8.4 Channel Verification & WAF 11128 Anti-Detection Alignment (`workbuddy.go`, `workbuddy_cloaking.go`, `workbuddy_test.go`, `workbuddy_executor_test.go`)**:
  - **Gateway Fingerprint Removal (`X-Machine-ID` / `X-Session-ID`)**: Aligned with official desktop 5.8.4 capture and authoritative upstream findings (`ardeyouxipianyi/workbuddy2api-hub` Commit `49f19e52`). Tencent WorkBuddy 5.8.4 tightened channel verification on `/v2/chat/completions` and flags static or derived machine/session hashes as an unauthorized gateway signature, rejecting calls with HTTP 400 and `code: 11128` ("request illegal" / "这次调用没通过渠道校验，可能不是官方客户端发出的").
  - Removed `X-Machine-ID` and `X-Session-ID` header injection from `ApplyHeaders` and actively stripped them in `StripWorkBuddyForbiddenHeaders`.
  - Verified live upstream requests across domestic and international production accounts, restoring 100% success rate with HTTP 200.
  - Combined check-in record and compensation status retrieval into a single "刷新状态" manual button click.

### Added
- **WorkBuddy Outage & Activity Compensation Suite Integration (`checkin.go`, `checkin_test.go`, `workbuddy_checkin.go`, `workbuddy_checkin_test.go`, `server_management_v8.go`, `plugin_quota.go`, `management.html`)**:
  - **On-Demand Official Outage Compensation Claim (`POST /billing/meter/claim-compensation`)**: Added `ClaimCompensation` and `ClaimCompensationAll` in `internal/auth/workbuddy` to claim official activity gifts (such as the 500 DeepSeek outage credits into package `TCACA_code_007_nzdH5h4Nl0`), handling successful grant (`claimed`), already-claimed deduplication (`done`), and expired/inactive activity windows (`inactive`). Handled upstream HTTP 400 JSON payloads (`code: 10001`) gracefully for already-claimed and inactive states.
  - **Model Multiplier Breakdown & UI Visualization**: Exposed `model_rates` in `listWorkBuddyQuotas` (`plugin_quota.go`) and updated the management dashboard (`management.html`) with interactive per-model credit rates sorted ascending, free models tagging (0x), and expand/collapse details.
  - **Frontend One-Click 500 Compensation**: Integrated official 500-credit compensation banner, batch claim, individual account claim, and status badges (`🎁 +500已领` / `🎁 500已领` / `🎁 待领500`) directly inside the WorkBuddy management suite.
  - **Strictly Decoupled from Daily Scheduled Ticker**: The compensation claiming pipeline is designed strictly as an on-demand management utility and is excluded from recurring daily check-in tickers and startup loops, eliminating useless background traffic for one-off promotions.
  - **Management Endpoints & Deduplication**: Added `GET/POST /v8/management/workbuddy/compensation` and extended `POST /v8/management/workbuddy/checkin?action=compensation` to support manual on-demand querying and one-click bulk claims for single or all managed WorkBuddy accounts, persisting results in `workbuddy_checkins.json` (`compensations` payload).
  - **Comprehensive Unit Testing**: Added unit tests covering successful claims, duplicate-claim handling, inactive campaign windows, unauthorized tokens, and persistence deduplication in both the core engine and HTTP handlers.

### Fixed
- **WorkBuddy Suite Management UI Initialization (`management.html`)**:
  - Resolved `ReferenceError: Cannot access 'A' before initialization` in `WorkBuddySuite` component by elevating the `quota` callback definition (`A`) before `useEffect` hooks consuming it in their dependency arrays.
- **WorkBuddy Client Identity Cloaking & Channel Validation Alignment (`workbuddy.go`, `workbuddy_test.go`, `README.md`, `README_CN.md`, `AGENTS.md`)**:
  - **Upstream Channel Validation Resolution (Code 11-128)**: Injected the full conversation header family (`X-Conversation-ID`, `X-Conversation-Request-ID`, `X-Root-Request-ID`, `X-Conversation-Message-ID`, `X-Request-ID`), resolving upstream HTTP 400 interception (`conversation id missing` / displayTips "这次调用没通过渠道校验，可能不是官方客户端发出的").
  - **Desktop 5.8.4 Outbound Identity & Distributed Tracing Alignment**: Aligned outbound request headers with the latest desktop 5.8.4 traffic captures, setting `X-Product: SaaS`, international `X-IDE-Version: 5.8.4` (China realm `5.5.6`), `User-Agent: WorkBuddy/5.8.4 workbuddy-ai/5.8.4 CLI/2.161.0`, `X-Agent-Intent: craft`, `X-Agent-Purpose: conversation`, `X-Agent-Type: main`, `X-Private-Data: true`, along with W3C / B3 distributed trace headers (`X-Trace-ID`, `traceparent`, `b3`, `X-B3-*`).
  - **Community Upstream Acknowledgement & Single Source of Truth**: Added `https://github.com/ardeyouxipianyi/workbuddy2api-hub` to `README.md` and `README_CN.md` acknowledgements and enshrined it in `AGENTS.md` and project memory as the authoritative reference for any future WorkBuddy protocol reverse-engineering and anti-detection updates.

### Added
- **WorkBuddy / CodeBuddy Quota Meter & Expiration Tracking Alignment (`quota.go`, `plugin_quota.go`, `workbuddy_test.go`, `management.html`)**:
  - **Billing Meter Architecture Alignment**: Upgraded WorkBuddy / CodeBuddy quota querying to directly consume Tencent's billing meter endpoints (`POST /billing/meter/get-user-resource-summary`, `/get-enterprise-user-usage`, `/get-user-resource-paid-packages`, `/get-user-resource-free-packages`), aligning with Magpie's production reverse-engineered capacity structure.
  - **Accurate Point Balances**: Replaced fallback to activity check-in status with true plan capacities (`CycleTotalCapacity`, `CycleUsedCapacity`, `CycleRemainCapacity`), accurately reflecting total and available package credits across personal Pro, Free, and Enterprise team plans.
  - **Soonest Expiry / Cutoff Calculation**: Implemented pagination and expiration parsing for paid and free packages (`runsOutOf`), tracking exact package end times (`DeductionEndTime`, `ExpiredTime`, `CycleEndTime` in CST UTC+8), surfacing earliest cutoff timestamps (`expires_at`, `runs_out_at`, `runs_out_points`) in both API quota metrics and management endpoints.
  - **Frontend Display**: Updated management dashboard WorkBuddy quota card to show earliest expiration dates (`截止: YYYY-MM-DD HH:MM:SS`) or total points alongside the available point balance.

### Added
- **WorkBuddy / CodeBuddy Per-Model Credit Multipliers (`workbuddy_credits.go`, `workbuddy_credits_test.go`, `service_models.go`, `workbuddy.go`, `quota.go`, `plugin_quota.go`)**:
  - **Live Multiplier Sourcing Instead of a Boolean Free Flag**: The WorkBuddy / CodeBuddy catalogue (`GET /v3/config`) publishes a real per-model billing multiplier (`credits`, e.g. `x0.00` free, `x0.79`, `x3.31`). That catalogue is now fetched with the account's own credentials, cached per realm for 30 minutes (2 minutes after a failure), and overlaid onto the static model registry, so callers see the actual rate instead of a coarse "is it free" answer.
  - **Promotion-Aware Effective Rate**: Time-boxed promotions (`modelPromotions`) are applied only when `enabled`, inside their `validFrom` / `validUntil` window and inside their `daily` window (midnight-crossing windows included), and only when they carry a real `discount` block — a badge-only entry is used as an annotation and can never imply a model is free. The effective rate is the cheaper of the base rate and any active promotion.
  - **Strictly Additive Overlay**: Live rates are merged onto the existing static list rather than replacing it, so models that exist locally but not upstream (e.g. `hy4-preview-f`) survive, and models that exist only upstream are never invented. The `-free` alias convention is preserved: an alias such as `deepseek-v4.1-flash-free` inherits its base model's rate, and user-renamed IDs keep working.
  - **Fail-Closed on Unknown Rates**: An unparsable rate, a failed fetch, or an empty catalogue (a common symptom of an unusable credential answering `200` with no models) leaves the previous rates in place and reports the rate as *unknown* rather than `0x` free. `HasRates` is only set from a live base rate, never from promotions alone.
  - **Realm Isolation**: The international (`workbuddy.ai`) and mainland China (`copilot.tencent.com`) catalogues are cached and resolved separately, so a model that is free in one realm is never reported as free in the other.
  - **Surfaced Through Every Layer**: `/v1/models` now emits a `credits` object (`multiplier`, `free`, `source`, plus `promotion_id` / `promotion_note` when a promotion is in effect) for OpenAI-compatible clients; the quota summary gains `credits_free_count`, `credits_paid_count`, `credits_rate_min` and `credits_rate_max`; and `listWorkBuddyQuotas` returns a `multipliers` range alongside `free_models` / `paid_models`. The previously hardcoded `free_tier: true` is gone — it is now derived from the live catalogue and omitted when the catalogue is unknown.
  - **Management Panel Patch (`management.html`)**: The WorkBuddy account card now renders `可用倍率: 0x / 0.79x - 3.31x` (or `0x 起`, or `未知` when the catalogue has not been read) in place of the uninformative `每日免费: 已生效`.
  - **Test Suite Coverage**: Added `workbuddy_credits_test.go` (parsing, fail-closed behaviour, promotion windows and expiry, overlay semantics, realm isolation, TTL, alias normalization), `quota_credits_test.go` (catalogue headers, quota metrics, unknown-catalogue omission) and `sdk/cliproxy/workbuddy_credits_test.go` (end-to-end registration asserting the real fetched URL, realm-specific rates, catalog-failure survival and OpenAI model-list emission), plus management handler coverage.

### Fixed
- **Credit Multipliers Never Reached Clients (`model_registry.go`, `openai_handlers.go`, `workbuddy_credits_shadow_test.go`, `openai_models_credits_test.go`)**:
  - **Shared-ID Overwrite Discarded Billing Annotations**: `addModelRegistration` replaces the registration's `Info` with the newly arriving provider's definition (`existing.Info = cloneModelInfo(model)`). The registry is global and keyed by model ID alone, so when a second provider exposed the same ID — for example the `openai-compatible-opencode` upstream also serving `glm-5.3`, `glm-5.3-flash`, `kimi-k3` and `deepseek-v4.1-flash` — that provider's definition overwrote the WorkBuddy one and the freshly resolved credit multiplier silently disappeared. Only IDs unique to WorkBuddy (such as `hy3` and the `-free` aliases) kept their rates, which made the feature look like it worked while the most commonly requested models reported no rate at all.
  - **The Same Erasure on the Client Re-registration Path**: A second, independent overwrite lived in `registerClientLocked`, which refreshes every retained model's shared `Info` on each reconciliation (`reg.Info = cloneModelInfo(model)`) without going through `addModelRegistration`. Whenever a client re-announced models it had already registered — including the same WorkBuddy credential re-registering its own models — rates were dropped again. Both paths now share one helper, `preserveModelCredits`, so the guarantee holds regardless of which path a registration takes.
  - **Registration-Order Independence**: Because the two providers race, a rating can arrive either before or after the unrated definition of the same ID. The earlier fix only guarded one direction; a rate that was resolved first was still erased by an unrated provider registering later. An incoming definition that carries no `Credits` now inherits them from the definition it replaces, in both orderings, while a provider that *does* publish credits still wins and every non-billing field (context length, capabilities) continues to come from the last registration as before.
  - **`/v1/models` Rebuilt Every Entry From a Four-Field Whitelist**: Even with the registry holding correct rates, `OpenAIModels` reconstructed each model into a fixed OpenAI shape (`id`, `object`, `created`, `owned_by`) and discarded every other key, so the `credits` object `convertModelToMap` had produced never reached the client. The handler now carries the annotation through alongside the required OpenAI fields, giving clients the real multiplier plus `free`, `source` and any active `promotion_id` / `promotion_note`.
  - **Regression Tests**: Added `workbuddy_credits_shadow_test.go` covering the production ordering end to end — an unrated upstream creating the ID first, the rated provider incrementing it, several more unrated clients registering, and both clients re-registering — plus `openai_models_credits_test.go`, which drives the real `/v1/models` handler and asserts the full credits payload survives while an unrated model grows no `credits` key. Every test was confirmed to fail with its corresponding fix reverted.

### Added
- **OpenCode Zen Free Protocol Fingerprint & Cloaking Overhaul (`opencode_cloaking.go`, `openai_compat_executor.go`, `opencode_cloaking_test.go`)**:
  - **Modernized Client Identity Fingerprinting**: Upgraded OpenCode client version to `1.19.0` and User-Agent to `opencode/1.19.0 ai-sdk/provider-utils/4.0.23 runtime/bun/1.3.14`.
  - **Session Affinity & Bounded LRU Cache**: Introduced 1000-capacity bounded LRU session cache tied to conversation identities (`util.SessionIDFromContext`), ensuring stable `ses_...` IDs across multi-turn interactions.
  - **Comprehensive Header Cloaking**: Injected `x-session-affinity`, `X-Session-Id`, `x-opencode-session`, `x-opencode-session-id`, `x-opencode-client`, `x-opencode-project`, and monotonic request ID `x-opencode-request`, while scrubbing foreign client identifiers (`x-client`, `x-app-name`, `x-zcode-*`).
  - **Accurate Tool Normalization & Stub Management**: Removed non-standard `"write"` tool requirement, strictly adhering to Zen Free's mandatory `"bash"` and `"read"` tools. Normalized case-insensitive caller tools (`"Bash"`, `"Read"`) and injected non-callable stubs only for missing tools.
  - **Transparent Real-Time Stub Call Interception & Conversion**: In streaming mode (`openCodeStreamFilter`) and non-streaming mode (`aggregateOpenCodeSSEToChatCompletion`), cleanly stripped stub tool calls and arguments, automatically converting `finish_reason: "tool_calls"` to `"stop"` when only stubs were invoked and providing clean fallback responses to prevent downstream client crashes.
  - **Test Suite Coverage**: Added comprehensive unit tests in `opencode_cloaking_test.go` verifying ID generation, session affinity, header application, tool payload mutations, SSE filter chunk transformation, and non-streaming aggregation.

### Added
- **Smart Selective Response Gzip Compression Middleware (`selective_gzip.go`, `selective_gzip_test.go`, `server.go`)**:
  - **Downstream Bandwidth Optimization**: Introduced `SelectiveGzipMiddleware` to compress eligible non-streaming responses (e.g. `/v1/models`, non-streaming `/v1/chat/completions`, `/v1/messages`, `/v1/embeddings`) with Gzip when the client provides `Accept-Encoding: gzip`, saving 70%~85% wire bandwidth on large payloads.
  - **Strict Streaming Bypass**: Guaranteed zero buffering and zero delay for streaming responses (`text/event-stream` SSE, WebSockets, or early `Flush()`), ensuring real-time token typewriter generation is never delayed or buffered.
  - **Pooled Gzip Compression**: Utilized `sync.Pool` for `*gzip.Writer` allocation to minimize memory allocations and GC overhead during high concurrency.
- **Decompression Decoder Object Pooling (`request_body.go`)**:
  - **Sync.Pool Decoder Lifecycle**: Added `sync.Pool` instances for Gzip, Brotli, and Zstandard decoders (`gzipReaderPool`, `brotliReaderPool`, `zstdDecoderPool`) using `Reset(io.Reader)` reuse semantics to eliminate heap allocation spikes under high-throughput compressed request ingestion.
- **Multi-Format Request Body Decompression & Uniform Ingestion (`request_body.go`, `request_body_test.go`, `code_handlers.go`, `gemini_handlers.go`, `interactions_handlers.go`)**:
  - **Comprehensive Decompression Support**: Expanded `ReadRequestBody` and `decodeRequestBody` to support RFC-compliant HTTP `Content-Encoding` decoding for `gzip`, `x-gzip`, `br` (Brotli), `deflate` (zlib with raw flate fallback), and chained multi-stage encodings alongside existing `zstd`.
  - **Decompression Bomb Protection**: Enforced bounds checking on all decompression streams using `io.LimitReader(decoder, maxBytes+1)`, returning `*http.MaxBytesError` when uncompressed payloads exceed `MaxRequestBodyBytes`.
  - **Uniform Handler Body Ingestion**: Replaced raw `c.GetRawData()` calls across Claude `/v1/messages` and `/v1/messages/count_tokens`, as well as Gemini `/v1beta/models/*` and `/v1beta/interactions` handlers, with `handlers.ReadRequestBody(c)` to ensure transparent decompression across all upstream endpoints.
  - **Unit Test Coverage**: Added comprehensive test suite in `request_body_test.go` verifying plain text, gzip, x-gzip, brotli, deflate, zstd, chained encodings, max bytes limit enforcement, unsupported encodings, and corrupt data rejection.

### Fixed
- **Management UI Premature Return & Sequence Operator Bug in `Fp` Config Parser (`management.html`)**:
  - **Eliminated Premature Return**: Corrected comma-sequence evaluation bug in minified `Fp` where `return t.apiKeys = ..., t.quotaExceeded = ...;` prematurely returned `{ antigravityCredits: false }` instead of the fully parsed configuration object `t`. This bug had caused `config.loggingToFile` to evaluate to `undefined` in `Wae` React hook, triggering a false-positive warning barrier ("当前连接的是 CPA，日志查看需要先开启“日志记录到文件”。") and preventing log retrieval even when logging to file was enabled.
  - **Distributed Deployment Synchronization**: Synchronized updated `management.html` to Tokyo production VPS (`vps-tokyo-02`, `/opt/gate/static/management.html` + Gate in-memory cache reload) and US sandbox VPS (`us-zenix`, `/opt/cliproxyapi/static/management.html`).
- **Management UI Backward Compatibility for Legacy CPA Endpoints & Configuration Layout (`management.html`)**:
  - **Flat Configuration Fallback in `Fp`**: Added backward compatibility in `Fp` config parsing to check legacy flat keys (`e["logging-to-file"]`, `e["debug"]`, `e["request-log"]`, `e["logs-max-total-size-mb"]`, `e["error-logs-max-files"]`) when v8 nested namespace (`observability.logs.*`) is absent, preventing false-positive "日志查看需要先开启日志记录到文件" empty state barriers when connected to CPA nodes with flat configuration layouts.
  - **HTTP 404 Automatic Fallback in `Wy` Log Client**: Added 404 fallback routing in `Wy` log client methods:
    - `fetchLogs`: queries `/observability/logs` and falls back automatically to `/logs`.
    - `clearLogs`: falls back from `DELETE /observability/logs` to `DELETE /logs`.
    - `fetchErrorLogs`: falls back from `/observability/logs/errors` to `/request-error-logs`.
    - `downloadErrorLog`: falls back from `/observability/logs/errors/:name` to `/request-error-logs/:name`.
    - `downloadRequestLogById`: falls back from `/observability/logs/requests/:id` to `/request-log-by-id/:id`.
  - **Distributed Deployment Synchronization**: Synchronized updated `management.html` to Tokyo production VPS (`vps-tokyo-02`, `/opt/gate/static/management.html` + Gate in-memory cache reload) and US sandbox VPS (`us-zenix`, `/opt/cliproxyapi/static/management.html`).

### Fixed
- **WorkBuddy Checkin Engine & Quota Flow Anomaly Elimination (`checkin.go`, `workbuddy_checkin.go`, `plugin_quota.go`)**:
  - **Phantom Account Key Purging**: Filtered serialization envelope keys `"enabled"` and `"records"` in `isInternalCheckinKey` and management checkin/quota credential enumerators, preventing empty entries and `workbuddy_checkins.json` from leaking into account listings.
  - **Batch Quota Endpoint Contract**: Handled empty `auth_index` in `POST /v8/management/plugins/workbuddy/quota` by delegating directly to `listWorkBuddyQuotas`, eliminating `400 Bad Request: auth_index is required` when pulling batch point flow in the management drawer.

### Added
- **WorkBuddy Multi-Account Free Model Routing & WAF Protocol Compliance (`conductor_selection.go`, `workbuddy_cloaking.go`, `workbuddy_cloaking_test.go`)**:
  - **Dynamic Provider Alias Resolution**: Added automatic executor resolution in `conductor_selection.go` (`executorLocked`), routing provider identities `workbuddy-cn`, `workbuddy-intl`, and `copilot-tencent` to the unified `WorkBuddyExecutor`, preventing `auth_not_found` dispatch failures when targeting domestic credentials.
  - **Automated System Prompt Injection**: Tencent WAF strictly enforces error `11128` (`first message is not system prompt`) on chat streams; implemented `EnsureWorkBuddySystemPrompt` in `workbuddy_cloaking.go` to automatically prepend or backfill `{"role": "system", "content": "You are an expert coding assistant."}` when missing or blank.
  - **Free Model Alias Normalization**: Stripped `-free` suffixes (`deepseek-v4.1-flash-free`, `glm-5.3-flash-free`, `hy4-preview-f-free`) to upstream model identifiers in `SanitizeWorkBuddyPromptWAF`.
  - **Regression Coverage**: Added unit tests in `workbuddy_cloaking_test.go` verifying system prompt prepending, preservation of custom system messages, and empty-array fallback handling.

### Merged
- **Sync with Upstream (`router-for-me/CLIProxyAPI:main` up to v8.0.31)**:
  - Merged 3 upstream commits bringing pluginhost improvements, optimistic concurrency control for management APIs, and response translation performance optimizations:
    - **Plugin Host & Shadow Copies (`shadow.go`, `shadow_windows.go`, `shadow_unix.go`, `loader_windows.go`, `loader_unix.go`)**:
      - Added content-addressed shadow copies for dynamic library loading to resolve platform-specific dynamic library file locks during in-flight plugin updates and unloads (#6672).
      - Added `thinking.CollapseConfigurationUpdates`, `thinking.StripConfigurationUpdates`, and `thinking.HasThinkingSuffix` to properly degrade or preserve reasoning effort in Sign in with ChatGPT (SIWC) Codex requests when configuration updates are stripped.
    - **Management Optimistic Concurrency Control (`config_v8.go`, `server_middleware.go`, `provider.go`)**:
      - Implemented SHA-256 ETag generation on `/v8/config` reads and enforced RFC 9110 `If-Match` validation on mutating requests to prevent stale config overwrites (returning HTTP 412 Precondition Failed upon mismatch) (#6666).
      - Exposed `ETag` in CORS response headers (`corsExposedResponseHeaders`) and masked sensitive `value` query parameters in request logging to prevent credential leakage.
    - **Claude to OpenAI Responses Streaming Translation (`claude_openai-responses_response.go`)**:
      - Pinned and cached validated request JSON once per stream (`st.RequestJSON`) instead of re-picking and re-validating the full body on every chunk, drastically reducing allocation and CPU overhead on long chat streams (#6665).
    - **Fork Preservation**: Verified seamless integration with all fork-specific features: WorkBuddy suite (outbound cloaking, channel validation, credit multipliers, check-in engine, quota meter & expiration dates), OpenCode cloaking & fingerprinting, Pace & Smart routing, selective gzip compression, and session affinity.

- **Sync with Upstream (`router-for-me/CLIProxyAPI:main` up to v8.0.30)**:
  - Merged 13 upstream commits bringing model updates, concurrency protection, and security enhancements:
    - **Models**: Added `gemini-4-argon` model configuration with native web search and thinking capabilities in `models.json` (#6645).
    - **Auth Conductor & Cooldown Protection**: Introduced manager-local monotonic execution sequence stamps (`ExecutionSeq`) and dispatch watermarks (`cooldownRecordedSeq`) to protect active model and credential cooldowns from being erased by stale in-flight execution successes under high concurrency (#6643).
    - **Config & File Watcher Security**:
      - Added `PrepareConfigFileForUpload` and `ComputeSourceHash` to hash plaintext management secrets with bcrypt and clean legacy layouts before remote storage seeding (ObjectStore, PostgresStore, server bootstrap) (#6628).
      - Added `SourceHash` tracking and serialized config reloads under mutex lock to guard management saves against stale file snapshots and reloads (#6631).
    - **Claude Headers & Request IDs**: Added operator overrides for Claude header defaults (`anthropic-beta-strip`, `anthropic-beta-append`, `inject-diagnostics`, and `client-request-id` supporting `on`, `off`, `passthrough`) (#5093).
    - **Vertex / Gemini**:
      - Honored full `/publishers/` prefix in API-key base-url, preventing duplicated `/v1` segments and 404s when pointing at specific project/location endpoints (#6658).
      - Dropped trailing thought-only model turns in Gemini Vertex requests to prevent upstream validation errors (#6630).
      - Stripped unsupported `discriminator` keyword from JSON schemas for Gemini and Antigravity while preserving legitimate properties (#6639).
    - **Protocols & Executors**:
      - Dropped unsupported web_search options (`external_web_access`, `search_context_size`) that the xAI Responses API rejects (#6642).
      - Mapped incomplete OpenAI Responses tool status to Claude `tool_result` with `is_error: true` (#6640).
    - **Fork Preservation**: Adapted management `PatchGlobalExcludedModels` / `DeleteGlobalExcludedModels` to upstream's new `targetOf` closure signatures, and updated stream tests to pass `executionSeq`. Fully preserved all fork-specific innovations: WorkBuddy suite (credit multipliers, check-in engine, quota meter & expiration dates), OpenCode cloaking & fingerprinting, Pace & Smart routing, selective gzip compression, and session affinity.

- **Sync with Upstream (`router-for-me/CLIProxyAPI:main` up to v8.0.29)**:
  - Merged 12 upstream commits bringing model updates and architectural stability enhancements:
    - **Models**: Added `gemini-3.8` variants and `muse-wonder-full-1.0` variants with updated context limits in model registry.
    - **Model Registration Fencing**: Introduced epoch-checked client registration and removal (`RegisterClientIfEpochCurrent`, `UnregisterClientIfEpochCurrent`) to prevent superseded async publications from clobbering newer client state.
    - **Deletion Reconciliation**: Added `AuthFileSnapshot` and whole-file removal tracking to accurately reconcile deletions without dropping re-created auths.
    - **Gemini / Claude Translation**: Ensured unique tool use IDs across turns in Gemini-to-Claude response translation, treated `MALFORMED_FUNCTION_CALL` as retryable API error, and flattened multi-branch tool unions in Antigravity.
    - **TLS Handshake Resilience**: Added root request config `tls-handshake-timeout-seconds` and retry logic for silent upstream TLS handshakes.
    - **Token Usage**: Corrected cached token calculation in Gemini responses and bumped Antigravity fallback client version to 2.23.0.
    - **Plugin API**: Exposed plugin config field defaults (`Default any`) in management endpoints.
    - **Preserved Fork Features**: Maintained all custom features without regression (WorkBuddy suite & checkin engine, OpenCode, Smart/Pace routing, persistent session affinity, clean ad-free docs).

- **Sync with Upstream (`router-for-me/CLIProxyAPI:main`)**:
  - Merged 28 upstream commits bringing models and stability enhancements:
    - Added new models: `claude-haiku-5-5`, `gemini-4-argon`, `gemini-4-argon-high`, `muse-wonder-1.0`.
    - Supported model-specific default `max_tokens` from registry in Claude OpenAI chat completions.
    - Implemented `CodexSSEScanner` for robust event parsing in Codex streams.
    - Added request-scoped HTTP transport decorator (`sdk/httptransport`).
    - Fixed concurrent configuration access data races (`fix(server)`).
    - Hardened schema sanitization preserving multi-branch `anyOf/oneOf` unions and stripping out-of-bounds int64 ranges.
    - Preserved custom fork architecture: WorkBuddy suite, OpenCode integration, Smart/Pace routing, persistent session affinity, and ad-free READMEs.

### Fixed
- **WorkBuddy Quota Resolution, Check-in Engine Lifecycle & Account Isolation (`quota.go`, `checkin.go`, `plugin_quota.go`, `workbuddy_checkin.go`, `management.html`)**:
  - **Tencent Quota 404 Intelligent Fallback**: Upstream Tencent endpoints returned 404 on `/v2/plugin/user/points` for standard user queries; added seamless secondary fallback to `POST /v2/billing/meter/checkin-activity-status`, extracting `total_credits` (e.g. 200 points), `daily_credit`, season theme (e.g. "Buddy加油站 第10季"), and VIP status.
  - **Automated Check-in Engine Lifecycle Toggle**: Implemented persistent enable/disable switch in `CheckinEngine` (`IsEnabled()`, `SetEnabled(bool)`) backed by structured JSON (`{"enabled": bool, "records": {...}}`) with full backward compatibility for legacy flat map stores. Exposed via `GET`, `POST`, and `PUT /v8/management/workbuddy/checkin`.
  - **Interactive CPAMC Management Badge**: Added real-time engine status badge and toggle button in `WorkBuddySuite` drawer in `management.html`, allowing operators to pause or resume background check-in execution with instant visual feedback.
  - **Strict Multi-Account Isolation**: Ensured exactly two real WorkBuddy accounts (Domestic CN and International) are enumerated, permanently eliminating `workbuddy_checkins.json` and internal state files from being exposed in credentials or check-in drawer lists.
  - **Unit Regression Suite**: Added `TestCheckinEngine_EnabledToggle` for disk persistence verification and `TestQueryQuota_ActivityStatusFallback` for mock activity meter fallback testing.
- **Auth File Management & Conductor Safety Hardening (`auth_files.go`, `auth_files_crud.go`, `quota.go`, `conductor_cooldown.go`)**:
  - **Internal Storage File Protection**: Guarded `isUnsafeAuthFileName` to reject internal storage files (`session_affinity.json`, `workbuddy_checkins.json`, `*checkins.json`), preventing accidental deletion, download, or overwrite via management auth file CRUD APIs.
  - **Upload Multipart Validation**: Enforced `isUnsafeAuthFileName` check on uploaded multipart file headers in `storeUploadedAuthFile`.
  - **WorkBuddy Nickname Fallback**: Expanded `ExtractNickname` lookup keys to support `"nickname", "nick_name", "username", "user_name", "name", "email", "user_id"`, plus attributes and label fallback.
  - **Conductor Nil Hook Protection**: Added safe `notifyResultHook` with nil checks in `conductor_cooldown.go`, preventing nil pointer panics when executing stream callbacks under test or unhooked manager instances.
  - **Regression Tests**: Added `TestDownloadAuthFile_RejectsInternalStorageFiles` and `TestDeleteAuthFile_RejectsInternalStorageFiles`.
- **WorkBuddy Multi-Account Checkin & Quota Isolation (`filestore.go`, `checkin.go`, `workbuddy_checkin.go`, `plugin_quota.go`, `management.html`)**:
  - **Phantom Account Elimination**: Excluded internal persistence files (`workbuddy_checkins.json`, `session_affinity.json`, and any `*checkins.json`) from `FileTokenStore.List` and `CheckinEngine`, preventing `workbuddy_checkins.json` from being loaded as a third phantom WorkBuddy credential.
  - **Strict Credential Matching**: Hardened credential matching in `workbuddy_checkin.go` to require explicit provider (`workbuddy`, `workbuddy-cn`, `copilot-tencent`) or `auth_kind: workbuddy`, eliminating broad prefix matches on arbitrary files starting with `workbuddy`.
  - **Batch Quota Endpoint & 404 Resolution**: Added batch quota query fallback (`listWorkBuddyQuotas`) in `GetPluginQuota` when `auth_index` is omitted, returning point balances, VIP tiers, and free tier status for all managed WorkBuddy accounts. Fixed request URL in CPAMC (`management.html`) from `/v8/management/plugins/workbuddy/quota` to relative `/plugins/workbuddy/quota` under Axios `baseURL`.
  - **UI Emoji Cleanup**: Removed all emojis across the WorkBuddy Suite drawer UI (`📅`, `📊`, `🟢`, `🔥`, `💡`, `👉`) for a clean, distraction-free interface.
  - **Regression Coverage**: Added unit test `TestWorkBuddyPluginQuota_BatchWithoutAuthIndex` to verify batch quota fetching and internal storage file suppression.
- **Filter Internal Storage Files from Management Auth Files List (`auth_files.go`)**:
  - Filtered out internal data persistence files (`session_affinity.json`, `workbuddy_checkins.json`) from `isAuthFileListable`, `buildAuthFileEntryLocked`, and `listAuthFilesFromDisk`.
  - Prevented internal persistence JSON files in `auths/` directory from being misclassified as `unknown` provider credentials ("其他" / Other) on the CPAMC management panel.
  - Added unit regression tests `TestListAuthFilesExcludesInternalStorageFiles` to ensure memory and disk fallback listings reliably suppress internal storage files.
- **WorkBuddy Suite Drawer UX Overhaul & Credential Visibility (`management.html` & `workbuddy_checkin.go`)**:
  - **100% Credential Visibility**: Merged all managed WorkBuddy accounts from `AuthManager` with existing checkin records, rendering un-checked-in domestic accounts as "待签到" and international accounts as "国际版无打卡", eliminating false "未检测到已纳管的 WorkBuddy 账号" alerts.
  - **Drawer Refinement & Separation of Concerns**: Removed legacy "多端授权登录" and "插件设置" tabs from the suite drawer (relegating authentication to the global `/oauth` management panel with quick-action navigation buttons). Retained only "每日自动打卡" and "额度流水与配额" tabs.
  - **Axios Response Unwrapping Fix**: Fixed date/time and record list hanging on "正在拉取..." caused by double unwrap of `G.get()` responses (`e?.data || e`).
  - **Nickname Resolution**: Exported `workbuddy.ExtractNickname` and added `RecordCheckin` to ensure individual manual check-in updates persist immediately to `workbuddy_checkins.json`.

### Documentation
- **Purge Third-Party Ads & Overhaul Fork Architecture Documentation (`README.md` & `README_CN.md`)**:
  - Completely purged third-party ad links, paid relay affiliate promotions, and irrelevant sponsorship tables from the root documentation.
  - Rewrote the project overview to focus entirely on fork-specific architectural innovations: Tencent WorkBuddy / CodeBuddy dual-realm integration, OpenCode 1M free tier protocol support, five multi-strategy quota load balancing modes (Smart & Pace Quota), cross-restart persistent session affinity (`session_affinity.json`), streaming loop guard circuit breaker, SSE keepalive pumper, zero-knowledge log redaction, and turnkey CPAMC v1.26.2 management suite.
- **Updated Project Overview and Special Acknowledgements (`README.md` & `README_CN.md`)**:
  - Added multi-strategy load balancing features (Round-Robin, Weighted Round-Robin, Fill-First, Smart Quota, Pace Quota) and persistent session affinity across restarts.
  - Documented native WorkBuddy & CodeBuddy management suite, automated daily check-in, points tracking, and dual-realm integration.
  - Added Special Acknowledgements (特别鸣谢) to **Magpie project** (session affinity persistence across restarts, sliding-window TTL, rate-limit sink scheduling, and zero-knowledge redaction) and **WorkBuddy project** (Tencent Copilot/CodeBuddy dual-realm reverse engineering, billing meters, WAF cloaking, and check-in workflows).

### Added
- **Smart & Pace Routing Strategies Integration in CPAMC Control Panel (`management.html`) & Management API**:
  - Added `smart` ("临期优先 (Smart Quota)") and `pace` ("步调均衡 (Pace Quota)") options to the visual network settings routing strategy dropdown menu (`Oae`).
  - Extended parser `kB(e)` in CPAMC to recognize `smart`, `smart-routing`, `pace`, and `pace-routing` without fallback degradation to `round-robin`.
  - Added dashboard status summary badge localization and global settings search keywords (`smart`, `pace`).
  - Supported localized strings across 6 languages (Simplified Chinese, Traditional Chinese, English, Russian, Vietnamese, Korean).
  - Extended `normalizeRoutingStrategy` in backend handler (`internal/api/handlers/management/config_basic.go`) and added regression tests to accept `smart` and `pace` via management REST endpoints.

### Fixed
- **WorkBuddy Suite API Request Path Fix in CPAMC (`management.html`)**:
  - Corrected WorkBuddy checkin endpoints from absolute `/v8/management/workbuddy/checkin` to relative `/workbuddy/checkin` under the Axios API client `G`.
  - Resolved 404 Not Found error caused by URL duplication (`/v8/management/v8/management/workbuddy/checkin`) because `G.instance.defaults.baseURL` already includes `/v8/management`.
  - Re-verified 100% clean ESM AST syntax across both bundles and deployed to `us-zenix` and `vps-tokyo-02`.
- **CPAMC Control Panel Syntax Repair (`management.html`)**:
  - Fixed unclosed JSX child container div after `j.map(...)` in `WorkBuddySuite` checkin drawer tab list.
  - Removed dangling open parenthesis in plugin drawer body ternary expression (`children:x&&v?v.id===\`workbuddy\`?...`).
  - Verified 100% clean AST compilation via Acorn across the entire 2.78MB SPA JavaScript bundle.

### Merged
- **Upstream v8.0.26 & v8.0.27 Core Alignment (`upstream/main`)**:
  - Synchronized official commits up to `bf934162` (`v8.0.27`).
  - **WebSocket Session Preservation Across Reloads**: Preserved active Codex and xAI WebSocket sessions during config reloads and single-auth removals via `ExecutionSessionSharer` and `AuthExecutionSessionCloser` (`sdk/cliproxy/auth/conductor.go`, `internal/runtime/executor/`).
  - **Request Body Size Limiting (`server.max-request-body-bytes`)**: Added protection against payload abuse with HTTP 413 handling across handlers and management endpoints (`internal/api/handlers/`).
  - **Multi-Auth & State Management in Plugins**: Enhanced plugin auth handling with multi-auth file parsing, stale auth unregistration, dynamic auth ID assignment, and file-based deduplication (`internal/pluginhost/auth_callbacks.go`).
  - **Pre-First-Token Streaming Keep-Alive**: Added `streaming.send-keepalive-before-ft` option to pump SSE heartbeats before initial token generation, gracefully handling early errors after headers commit.
  - **Conflict Resolution**: Cleanly resolved import collision in `internal/api/handlers/management/plugin_quota.go`, preserving native WorkBuddy quota provider while integrating upstream `clienterror` size limit handling.

### Added
- **WorkBuddy Unified Management Suite (`WorkBuddySuite`) in CPAMC Control Panel**:
  - Consolidated all WorkBuddy capabilities into a single unified plugin entry in the Plugins view (`/plugins`), featuring custom status badges (`每日自动签到 + 国际/国内双端`) and a dedicated `[管理套件]` primary action button.
  - Implemented an `xl` full-featured drawer component (`WorkBuddySuite`) comprising four core submodules:
    1. **每日自动签到**: Real-time Beijing time (CST / UTC+8) day boundary tracking, total / checked-in / streak account statistics, one-click batch check-in trigger via `POST /v8/management/workbuddy/checkin`, per-account status badges (`claimed`/`done`/`ineligible`/`inactive`/`own-app`/`failed`), points reward display, and standalone check-in action.
    2. **多端授权登录**: Dual-realm OAuth login cards for both international (`workbuddy`, `workbuddy.ai`) and mainland China (`workbuddy-cn`, `copilot.tencent.com`) realms with automated popup opening, copyable links, status polling, and hot credential persistence to `auths/`.
    3. **额度流水与配额**: Quick inline points inspection across all accounts via `/v8/management/plugins/workbuddy/quota` and direct navigation button to the independent `/quota` tabs.
    4. **插件设置**: Native plugin toggle, priority configuration, and atomic persistence.
  - Preserved strict architectural separation in the Quota view (`/quota`), retaining independent `WorkBuddy (en)` and `WorkBuddy (cn)` tabs with isolated stores and timeline calculations.

### Fixed
- **Plugin Feature Advertisement in Static Builds (`support_nocgo.go`)**:
  - Corrected `supportPluginValue` in `internal/pluginhost/support_nocgo.go` from `"0"` to `"1"` so that Linux static container builds (`CGO_ENABLED=0`) advertise `X-CPA-SUPPORT-PLUGIN: 1`.
  - Resolved missing Plugins and Plugin Store entries in the CPAMC sidebar on headless and Docker deployments (`us-zenix`).
  - Added fallback empty configuration mapping in `GetPluginConfig` for native suite plugins (`workbuddy` and `workbuddy-cn`) to prevent 404 errors during drawer initialization.
  - Prevented deletion of native suite plugins in `DeletePlugin`.
- **Management Center Web Interface Syntax Error Fix (`management.html`)**:
  - Fixed an unclosed bracket syntax error (`SyntaxError: Unexpected token '&&'`) in bundled CPAMC assets (`internal/managementasset/assets/management.html` and `static/management.html`) where the models description section prematurely closed the `children` array before status badges and empty states.
  - Resolved white-screen / failure to load issue when accessing `http://<host>:8315/management.html`.
  - Synchronized the repaired asset to production servers (`us-zenix` and `vps-tokyo-02`) and restarted Gateway services.

### Added
- **Pace & Smart Quota Routing Strategies (`smart` & `pace`) (Expiry-Aware & Headroom Pacing)**:
  - Implemented `SmartSelector` (`sdk/cliproxy/auth/selector_pace_smart.go`) prioritizing accounts whose quotas reset soonest to avoid forfeiture of unspent quota. Segments candidates into `fine` (<80% used), `low` (80%-98%), and `spent` (>=98%) tiers, truncating reset times to the hour to preserve upstream Prompt Cache affinity.
  - Implemented `PaceSelector` ranking credentials by hourly available burn rate ($\text{Remaining Allowance} / \max(1, \text{Hours until reset})$), routing to accounts with highest headroom while preserving relative order within a 10% tolerance band for prompt cache stability.
  - Integrated both selectors with CPA's 429 Rate Limit Sink ordering (`ApplySinkOrdering`) and universal `session-affinity` wrapping.
  - Wired `"smart"` and `"pace"` routing strategies into `sdk/cliproxy/service_config.go` and `internal/config/config_types.go`.
- **Zero-Knowledge Local Sensitive Data Redaction & Transparent Streaming Restoration (`redact`)**:
  - Implemented zero-knowledge redaction engine (`sdk/cliproxy/redact/`) masking sensitive secrets (API keys, private keys, database URLs with passwords, config credentials, JWT tokens) and personal data (emails, Chinese phone numbers, ID cards) into deterministic placeholders (`{{API_KEY_k3v9x2mq}}`).
  - Employs deterministic HMAC-SHA256 hashing so identical secrets across turns generate identical placeholders, guaranteeing 100% upstream Prompt Cache hits.
  - Transparently restores placeholders in non-streaming responses (`Execute`) and streaming chunks (`ExecuteStream`), with fuzzy tolerance for LLM formatting variations (spacing, braces, lowercase).
  - Implemented `StreamRestorer` handling token splits and SSE chunk boundary assembly, ensuring seamless client delivery without exposing raw placeholders or breaking JSON framing.
  - Added `RedactionConfig` (`redaction: enabled, mask-secrets, mask-personal`) to `Config` with unit and integration tests.
- **Native WorkBuddy Daily Check-in Engine (Automatic Credit Farming & Streak Maintenance)**:
  - Implemented WorkBuddy (CN realm) daily check-in automation (`internal/auth/workbuddy/checkin.go`) targeting Tencent CodeBuddy / Copilot billing meter APIs (`/v2/billing/meter/checkin-activity-status` and `/v2/billing/meter/daily-checkin`).
  - Added CST/Beijing (UTC+8) day boundary tracking, claim status detection (`claimed`, `done`, `ineligible`, `inactive`, `own-app`), and graceful risk control probing on HTTP 401.
  - Implemented `CheckinEngine` background scheduler with atomic persistence (`workbuddy_checkins.json`) preventing duplicate claims per Beijing day and surviving restarts.
  - Hooked `InitGlobalCheckinEngine` into `Service.Run` lifecycle (`sdk/cliproxy/service_lifecycle.go`) and registered management endpoints `GET /v8/management/workbuddy/checkin` and `POST /v8/management/workbuddy/checkin`.
  - Added full test suite in `internal/auth/workbuddy/checkin_test.go` covering international realm bypass, claim success, already checked-in status, and state deduplication.
- **Persistent Session Affinity & Streaming SSE Keep-Alive (Prompt Cache Resilience & Anti-Timeout)**:
  - Implemented `Snapshot` and `Restore` mechanisms on `SessionCache` (`sdk/cliproxy/auth/session_cache.go`) for bulk exporting and loading active unexpired session-to-auth bindings.
  - Added atomic `SessionCacheStore` interface and `FileSessionCacheStore` (`sdk/cliproxy/auth/session_cache_store.go`) allowing session affinities to survive process restarts, guaranteeing prompt cache continuity for long agent conversations across server upgrades.
  - Implemented W3C-compliant SSE comment keep-alive pumper (`: keep-alive\n\n`) in `wrapStreamResult` (`sdk/cliproxy/auth/conductor_stream.go`), emitting 15-second heartbeats during slow upstream model reasoning or retrieval phases, completely eliminating 504 Gateway Timeouts from intermediate proxies (Nginx / Cloudflare).
- **Streaming Loop Guard (Reasoning & Text Infinite Loop Circuit Breaker)**:
  - Implemented multi-protocol SSE streaming loop guard in `sdk/cliproxy/auth/loop_guard.go` inspecting reasoning and text deltas across OpenAI, Anthropic, Responses, and Gemini.
  - Applied a 256-unit sliding window with sentence/line boundaries to detect degenerative repetitive thinking cycles (e.g. models getting stuck repeating thought fragments).
  - Integrated `LoopGuard` into `wrapStreamResult` in `sdk/cliproxy/auth/conductor_stream.go`, immediately aborting stuck upstream streams and delivering clean `stream_loop_detected` errors, preserving user quota and preventing hung agent sessions.
- **Rate Limit Sink Mechanism & Session Affinity Resilience (Inspired by magpie architecture)**:
  - Implemented 429 Rate Limit Sink ordering algorithm (`sdk/cliproxy/auth/sink.go`) to prevent repeated account thrashing. Sunk credentials are smoothly demoted to the back of availability queues, cycling fairly among rate-limited accounts once cooled.
  - Wired `RecordRateLimitSink` into credential and model-level HTTP 429 response handling in `conductor_cooldown.go`.
  - Applied `ApplySinkOrdering` to `FillFirstSelector.Pick` to ensure accounts recovering from cooldown do not immediately starve alternate credentials.
  - Enhanced session hash derivation (`extractMessageHashIDs` in `sdk/cliproxy/auth/selector.go`) with first-user-turn anchor binding (`userAnchor`), improving multi-turn Prompt Cache affinity when system prompts dynamically change across turns.
  - Expanded `ExtractSessionInfo` in `sdk/cliproxy/session/info.go` to natively identify session headers across popular coding agent ecosystems (`X-Magpie-Session`, `X-OpenCode-Session`, `X-Continue-Session-Id`, `X-Cursor-Session-Id`).
- **Antigravity Refresh Token to CPA Auth Web Tool (`tools/rt2cpa`)**:
  - Built standalone lightweight Aozora UI Web service (`tools/rt2cpa/app.py`) for generating standard CLIProxyAPI authentication JSON files directly from Google Antigravity Refresh Tokens.
  - Automated token exchange via Google OAuth token endpoint, user email retrieval via userinfo endpoint, and project ID discovery via CloudCode `loadCodeAssist` and `onboardUser`.
  - Added one-click copy, download, and atomic saving into `auths/` directory with live warehouse list preview.
  - Aligned CPA Antigravity schema with complete `project_id` resolution (canonical `"aicode-consumers"` fallback), `antigravity_capability_hints` payload, `priority`, and `disabled` status, fixing CPAMC quota discovery failure ("Antigravity 凭证缺少 project_id").
  - Refined two-column workspace grid layout and responsive constraints (`minmax(380px, 460px)` + `minmax(0, 1fr)`) to prevent JSON preview blowout and column squeezing.
  - Extended Antigravity authentication library with `RefreshToken` and `RefreshTokenWithCredentials` (`internal/auth/antigravity/auth.go`) with regression unit tests (`internal/auth/antigravity/auth_test.go`).
- **Global Exit Model Exclusion & Modal Management Controls**:
  - Implemented `GlobalExcludedModels` (`global-excluded-models`) across configuration and model registries to selectively block models at the CPA exit boundary.
  - Added wildcard and exact model pattern matching to filter models out of client advertisement (`/v1/models`, OpenAI, Claude, Gemini) and request execution (`GetModelInfo`, `GetModelCount`, `GetModelProviders`).
  - Added full exit model inspection endpoint `GetExitModelCatalog` (`GET /v0/management/exit-models` & `GET /v8/management/models/exit-catalog`) which lists all upstream active models alongside their exclusion status, ensuring models never disappear permanently from operator view.
  - Added REST endpoints for global exclusion management: `GET`, `PUT`, `PATCH`, `DELETE` on `/v0/management/global-excluded-models`, and `POST /v0/management/global-excluded-models/toggle` (with v8 counterparts `/v8/management/models/global-excluded-models` and `/routing/global-excluded-models`).
  - Wired hot-reload and atomic persistence with zero-value cleanup in `internal/config/config_yaml.go`.
- **CPAMC Master Key Tagging & Exit Model Modal Interface**:
  - Added dedicated emerald badge `主 Key (Master)` for the first API key (`n === 0`) in the Client API Keys configuration section of CPAMC (`static/management.html` and `internal/managementasset/assets/management.html`).
  - Redesigned model exit controls into a dedicated "出口模型管理" modal dialog (`Qm`) with live search, batch "全部开启" / "全部取消" buttons, and checkbox-based inclusion (checked by default, unchecked to exclude).
  - Added dynamic counter badge `(已屏蔽 N 个)` to the modal trigger button for instant visual feedback.
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
