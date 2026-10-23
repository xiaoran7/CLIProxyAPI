# Gateway Console Subsystem (Native Integration)

[English](./console.md) | [中文说明](#中文介绍)

The Gateway Console (`internal/console/`) is a native, high-performance API gateway and multi-tenant management subsystem embedded directly into CLIProxyAPI (CPA). It supersedes the legacy external Node.js gateway (`Project/gate`), eliminating PM2, systemd daemon dependencies, and 150MB+ of background memory overhead while preserving 100% backward compatibility.

---

## Key Capabilities

1. **Zero-Latency In-Memory Authentication (`ConsoleAccessProvider`)**:
   Multi-tenant API keys (`sk-cpa-...`) authenticate in-memory with zero network hops, registered globally into CPA's core `sdk/access` manager. Supports protocol penetration across OpenAI (`Authorization: Bearer`), AI (`x-api-key`), Azure OpenAI (`api-key`), and Google Gemini (`x-goog-api-key` and `?key=` query parameters).

2. **RPM & Token Quota Enforcement**:
   - **Sliding-window RPM limits**: Enforced instantly before quota checks to prevent request floods.
   - **Daily Token Quota**: Enforces daily consumption caps with configurable burst ratio (`hard_ratio`, default `1.2`).
   - **Smart Model Whitelisting**: Restricts key access by model patterns (exact, wildcards, provider prefixes).
   - **Instant Revocation**: Deleting or deactivating a key in Console immediately returns `401 Unauthorized` on proxy endpoints.

3. **Zero-Migration Database Parity**:
   Retains bit-for-bit schema parity across all 14 SQLite tables (`keys`, `deleted_keys`, `quota_rules`, `usage_daily`, `request_events`, `usage_outbox`, `meta`, `error_events`, `model_prices`, `quota_snapshots`, `upstream_auth_baselines`, `upstream_daily_stats`, `upstream_validations`, `provider_registry`), allowing in-place reuse of existing `gate.db` data files.

4. **Transactional Outbox & Aggregation**:
   Synchronously records request events into `usage_outbox` (WAL mode) and periodically flushes batches into `usage_daily` and `request_events` via atomic transactions, surviving process restarts with automatic crash replay.

5. **Embedded Vanilla JS SPA (`//go:embed`)**:
   Single-page console UI embedded into the Go binary at compile-time with 8-byte hex MD5 ETag caching and conditional 304 responses, accessible at `/console` with automatic 301 redirection from legacy `/admin`.

---

## Route Overview

Routes are registered twice: under `/console/api` (canonical) and `/admin/api` (legacy alias). Every admin route requires the admin secret; member routes require a valid tenant API key.

### Pages

| Route Pattern | Method | Description |
| :--- | :--- | :--- |
| `/console` | `GET`, `HEAD` | Native Web Console Dashboard (Aozora UI SPA) |
| `/admin` | `GET`, `HEAD` | 301 Permanent Redirect to `/console` (legacy compatibility) |
| `/member` | `GET`, `HEAD` | 302 Redirect to `/console` (legacy compatibility) |
| `/console/healthz`, `/console/readyz`, `/console/metrics` | `GET` | Liveness, readiness, and Prometheus metrics |

### Keys

| Route Pattern | Method | Description |
| :--- | :--- | :--- |
| `/console/api/keys` | `GET`, `POST` | List and create multi-tenant API keys |
| `/console/api/keys/:id/reveal` | `GET` | Reveal the full plaintext key |
| `/console/api/keys/:id/rotate` | `POST` | Rotate the key secret |
| `/console/api/keys/:id/alias` | `PUT` | Update the human-readable alias |
| `/console/api/keys/:id` | `DELETE` | Delete key and revoke access immediately |
| `/console/api/keys/:id/quota` | `PUT`, `DELETE` | Set or clear the quota rule (daily limit, RPM, model whitelist) |

### Usage, Logs and Status

| Route Pattern | Method | Description |
| :--- | :--- | :--- |
| `/console/api/stats` | `GET` | Aggregated usage telemetry and cost estimates |
| `/console/api/requests` | `GET` | Request audit trail with token and cache breakdowns |
| `/console/api/errors` | `GET` | Upstream 429/5xx error and circuit-breaker events |
| `/console/api/status-summary` | `GET` | Per-provider health, latency, and availability |

### Pricing and Provider Registry

| Route Pattern | Method | Description |
| :--- | :--- | :--- |
| `/console/api/model-prices` | `GET`, `POST` | Dynamic model token pricing configuration |
| `/console/api/model-prices/:id` | `DELETE` | Remove a pricing row |
| `/console/api/provider-registry` | `GET`, `POST` | Provider labels and model/auth match patterns |
| `/console/api/provider-registry/:key` | `DELETE` | Remove a custom provider entry |

### Upstream Quotas and Credentials

| Route Pattern | Method | Description |
| :--- | :--- | :--- |
| `/console/api/upstream-quotas` | `GET`, `POST` | Upstream credential quota, health, and bucket snapshot |
| `/console/api/upstream-routing` | `GET`, `PUT` | Routing strategy and session affinity |
| `/console/api/upstream-auth/priority` | `PUT` | Set scheduling priority and weight for a credential |
| `/console/api/upstream-auth/status` | `PUT` | Enable or disable a credential |
| `/console/api/upstream-auth/batch` | `PUT` | Batch enable/disable by credential name |
| `/console/api/upstream-auth/activate-window` | `POST` | Force an upstream quota re-query |
| `/console/api/upstream-auth-stats` | `GET` | Per-credential request counts and success rates (`range=5h\|24h\|7d\|all`) |
| `/console/api/upstream-validations` | `GET` | Pending Google verification records |
| `/console/api/upstream-validations/:id/resolve` | `POST` | Mark a verification as resolved |
| `/console/api/upstream-validations/:id/dismiss` | `POST` | Dismiss a verification record |
| `/console/api/upstream-validations/:id` | `DELETE` | Delete a verification record |
| `/console/api/upstream-validations/manual` | `POST` | Register a verification manually (requires `accounts.google.com` URL) |
| `/console/api/quota-snapshots` | `GET` | Historical quota snapshots |
| `/console/api/openrouter/accounts` | `GET`, `POST` | OpenRouter credential accounts |
| `/console/api/openrouter/accounts/:id` | `PUT`, `PATCH`, `DELETE` | Update or delete an OpenRouter account |
| `/console/api/runtime-facts` | `GET` | Runtime counters, windows, and provider facts |
| `/console/api/runtime-config` | `GET` | Effective runtime configuration snapshot |

### CPA Loopback Proxy (`/console/api/cpa/*`)

Read-mostly pass-through to CPA's own management surface: `config-yaml`, `config`, `auth-files` (+ `models`, `download`, `refresh`, `fields`, `status`), `oauth/:provider/start`, `oauth/status`, `oauth/callback`, `oauth/cancel`, `models`, `models/catalog`, `logs`, `request-error-logs` (+ `preview`, `download`), `reset-quota`, `cooldown/reset`, `cooldown/reset-all`, `providers/:provider`, `settings/:setting`, `runtime-config`.

### Member Portal (`/console/api/member/*`, legacy `/api/member/*`)

| Route Pattern | Method | Description |
| :--- | :--- | :--- |
| `.../member/info` | `GET` | Tenant key metadata and quota rules |
| `.../member/status-summary` | `GET` | Provider health scoped to the key |
| `.../member/requests` | `GET` | The key's own request history |

### Gateway

| Route Pattern | Method | Description |
| :--- | :--- | :--- |
| `/v1/*`, `/anthropic/v1/*` | All | Core model gateway routes protected by `ConsoleAccessProvider` |
| `/v1/models` | `GET` | Model listing for a valid tenant key |

---

## Front-End Layout

The SPA is a set of classic `<script>` and `<link>` tags sharing one global namespace — no bundler, no IIFE, no ES modules. This is deliberate: cross-file helpers such as `escapeHtml`, `getAdminSecret`, `showToast`, `switchNavTab`, `openModal`, and `formatResetAbs` are called by filename order.

| Group | Files |
| :--- | :--- |
| State and shell | `console-state.js`, `console-nav.js`, `console-auth.js`, `console-format.js`, `console-polling.js`, `console-member.js`, `console-refresh.js`, `console-init.js`, `console-i18n.js` |
| Views | `console-overview.js`, `console-stats.js`, `console-logs.js`, `console-keys.js`, `console-cpa.js` |
| Upstream quotas | `console-quotas-priority.js`, `console-quotas-validation.js`, `console-quotas-fetch.js`, `console-quotas-render.js`, `console-quotas-workbuddy.js`, `console-quotas-stats.js` |
| Styles (load in order) | `console-base.css`, `console-panels.css`, `console-members.css`, `console-tables.css`, `console-modals.css`, `console-feedback.css`, `console-logs.css`, `console-cpa.css`, `console-overview.css`, `console-theme-beszel.css`, `console-theme-v239.css`, `console-theme-v2310.css`, `console-validations.css`, `console-upstream-stats.css` |

Stylesheet order matters: six cross-section overrides (`:root`, `body`, `header`, `.brand-icon`, `.modal-backdrop`, `.modal`) rely on the concatenated cascade, so new theme files must be appended, not reordered.

Legacy asset aliases still resolve: `/console/style.css`, `/admin/style.css`, and `/admin/admin-style.css` all map to `console-base.css`; `/admin/quotas.js` and `/admin/admin-quotas.js` map to the quota module set.

---

## Configuration & Environment Variables

| Variable | Default | Description |
| :--- | :--- | :--- |
| `CONSOLE_ENABLED` | `true` | Enable or disable the native console subsystem (`DISABLE_CONSOLE=true` also disables it) |
| `CONSOLE_DB_PATH` | Resolved default (falls back to `GATE_DB_PATH`, `GATE_DB`, `CPA_GUARD_DB`, `$GUARD_DIR`) | Path to the SQLite database file |
| `CONSOLE_ADMIN_SECRET` | Inherits `ADMIN_SECRET`, then `GATE_ADMIN_SECRET` | Admin password for `/console` operations |
| `CONSOLE_FLUSH_INTERVAL` | `100ms` | Background outbox transactional flush interval |
| `CPA_MGMT_HOST` | `127.0.0.1` | CPA loopback host used by `/console/api/cpa/*` |
| `CPA_MGMT_PORT` | `8000` | CPA loopback port |
| `CPA_MGMT_SECRET` | Inherits `MANAGEMENT_PASSWORD` | Secret used for CPA loopback calls |

---

<a id="中文介绍"></a>

# 网关控制台子系统（原生内嵌版）

网关控制台（`internal/console/`）是原生集成于 CLIProxyAPI 内部的高性能多租户网关与管理子系统。它彻底取代了历史上的独立 Node.js Gate 网关（`Project/gate`），消除了 PM2、systemd 等外部守护依赖及 150MB+ 的内存开销，同时保持 100% 数据库与路由兼容。

## 核心特性

1. **零延迟内存鉴权（`ConsoleAccessProvider`）**：
   多租户密钥（`sk-cpa-...`）通过 CPA 原生 `sdk/access` 提供者机制在内存秒级完成解析验证，完全杜绝回环 HTTP 调用与 401 延迟。支持 OpenAI、AI、Azure、Gemini 全协议头与 URL 参数穿透。

2. **RPM 与每日 Token 配额熔断**：
   - **滑动窗口 RPM 限制**：在消耗配额前即时限流，抵御高并发突发。
   - **每日配额流控**：支持设定每日 Token 上限及硬限制突发系数（`hard_ratio`，默认 1.2）。
   - **模型白名单拦截**：支持精确匹配、通配符（如 `gpt-4o*`）与提供商前缀白名单限制。
   - **即时吊销**：控制台停用或删除 Key 立即在推理网关生效，返回 401 拦截。

3. **零迁移数据库复用**：
   完整对齐 SQLite 14 张数据表结构（含 `deleted_keys`），无感挂载旧版生产 `gate.db`，保留全部历史账单流水与规则配置。

4. **事务性 Outbox 与状态持久化**：
   基于 SQLite WAL 模式，请求流水毫秒级同步写入 `usage_outbox`，随后按 `CONSOLE_FLUSH_INTERVAL`（默认 100ms）通过单一数据库事务批量落入 `usage_daily` 与 `request_events`，启动时自动重放未落盘流水。

5. **编译期全内嵌 SPA 页面（`//go:embed`）**：
   前端资源打入单一二进制文件，配备 8 字节 Hex MD5 ETag 缓存校验，直达 `/console`，原 `/admin` 自动 301 重定向。

## 路由约定

所有管理路由同时挂在 `/console/api`（正式）与 `/admin/api`（旧版别名）下。管理路由需管理员密钥；`/console/api/member/*`（及别名 `/api/member/*`）需租户 Key。CPA 回环代理统一位于 `/console/api/cpa/*`，透传 CPA 自身的管理面。完整路由表见上方英文小节。

## 前端文件布局

SPA 由一组经典 `<script>` 与 `<link>` 标签组成，共享同一全局命名空间——无打包器、无 IIFE、无 ES Module。这是刻意设计：`escapeHtml`、`getAdminSecret`、`showToast`、`switchNavTab`、`openModal`、`formatResetAbs` 等跨文件辅助函数依赖加载顺序。

样式表的加载顺序不可随意调整：`:root`、`body`、`header`、`.brand-icon`、`.modal-backdrop`、`.modal` 六处跨章节覆盖依赖拼接后的层叠顺序，新主题文件只能追加到末尾。

## 配置项

环境变量表见上方英文小节。
