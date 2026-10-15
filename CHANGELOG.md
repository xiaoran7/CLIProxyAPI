# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/).

## [Unreleased]

### Fixed
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
