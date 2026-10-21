# Gateway Console Subsystem (Native Integration)

[English](./console.md) | [中文说明](#中文介绍)

The Gateway Console (`internal/console/`) is a native, high-performance API gateway and multi-tenant management subsystem embedded directly into CLIProxyAPI (CPA). It supersedes the legacy external Node.js gateway (`Project/gate`), eliminating PM2, systemd daemon dependencies, and 150MB+ of background memory overhead while preserving 100% backward compatibility.

---

## Key Capabilities

1. **Zero-Latency In-Memory Authentication (`ConsoleAccessProvider`)**:
   Multi-tenant API keys (`sk-cpa-...`) authenticate in-memory with zero network hops, registered globally into CPA's core `sdk/access` manager. Supports protocol penetration across OpenAI (`Authorization: Bearer`), Anthropic (`x-api-key`), Azure OpenAI (`api-key`), and Google Gemini (`x-goog-api-key` and `?key=` query parameters).

2. **RPM & Token Quota Enforcement**:
   - **Sliding-window RPM limits**: Enforced instantly before quota checks to prevent request floods.
   - **Daily Token Quota**: Enforces daily consumption caps with configurable burst ratio (`hard_ratio`, default `1.2`).
   - **Smart Model Whitelisting**: Restricts key access by model patterns (exact, wildcards, provider prefixes).
   - **Instant Revocation**: Deleting or deactivating a key in Console immediately returns `401 Unauthorized` on proxy endpoints.

3. **Zero-Migration Database Parity**:
   Retains bit-for-bit schema parity across all 13 SQLite tables (`keys`, `quota_rules`, `usage_daily`, `request_events`, `usage_outbox`, `meta`, `error_events`, `model_prices`, `quota_snapshots`, `upstream_auth_baselines`, `upstream_daily_stats`, `upstream_validations`, `provider_registry`), allowing in-place reuse of existing `gate.db` data files.

4. **Transactional Outbox & Aggregation**:
   Synchronously records request events into `usage_outbox` (WAL mode) and periodically flushes batches into `usage_daily` and `request_events` via atomic transactions, surviving process restarts with automatic crash replay.

5. **Embedded Vanilla JS SPA (`//go:embed`)**:
   Single-page console UI embedded into the Go binary at compile-time with 8-byte hex MD5 ETag caching and conditional 304 responses, accessible at `/console` with automatic 301 redirection from legacy `/admin`.

---

## Route Overview

| Route Pattern | Method | Description |
| :--- | :--- | :--- |
| `/console` | `GET`, `HEAD` | Native Web Console Dashboard (Aozora UI SPA) |
| `/admin` | `GET`, `HEAD` | 301 Permanent Redirect to `/console` (Legacy compatibility) |
| `/member` | `GET`, `HEAD` | Member quota and usage inspection portal |
| `/console/api/keys` | `GET`, `POST` | List and create multi-tenant API keys |
| `/console/api/keys/:id` | `PATCH`, `DELETE` | Update alias/active state or delete key |
| `/console/api/keys/:id/quota` | `POST` | Update quota rule (daily limit, RPM, model whitelist) |
| `/console/api/stats/overview` | `GET` | Dashboard real-time telemetry and 24h usage breakdown |
| `/console/api/stats/timeseries`| `GET` | Minute-slot traffic and latency timeseries |
| `/console/api/events/recent` | `GET` | Recent request audit events and token breakdowns |
| `/console/api/prices` | `GET`, `POST` | Dynamic model token pricing configuration |
| `/v1/*`, `/anthropic/v1/*` | All | Core model gateway routes protected by `ConsoleAccessProvider` |

---

## Configuration & Environment Variables

| Variable | Default | Description |
| :--- | :--- | :--- |
| `CONSOLE_ENABLED` | `true` | Enable or disable the native console subsystem |
| `CONSOLE_DB_PATH` | `/CLIProxyAPI/data/gate.db` | Path to the SQLite database file |
| `CONSOLE_ADMIN_SECRET` | Auto (inherits CPA secret) | Admin password for `/console` operations |
| `CONSOLE_FLUSH_INTERVAL`| `5s` | Background outbox transactional flush interval |

---

<a id="中文介绍"></a>

# 网关控制台子系统（原生内嵌版）

网关控制台（`internal/console/`）是原生集成于 CLIProxyAPI 内部的高性能多租户网关与管理子系统。它彻底取代了历史上的独立 Node.js Gate 网关（`Project/gate`），消除了 PM2、systemd 等外部守护依赖及 150MB+ 的内存开销，同时保持 100% 数据库与路由兼容。

## 核心特性

1. **零延迟内存鉴权（`ConsoleAccessProvider`）**：
   多租户密钥（`sk-cpa-...`）通过 CPA 原生 `sdk/access` 提供者机制在内存秒级完成解析验证，完全杜绝回环 HTTP 调用与 401 延迟。支持 OpenAI、Anthropic、Azure、Gemini 全协议头与 URL 参数穿透。

2. **RPM 与每日 Token 配额熔断**：
   - **滑动窗口 RPM 限制**：在消耗配额前即时限流，抵御高并发突发。
   - **每日配额流控**：支持设定每日 Token 上限及硬限制突发系数（`hard_ratio`，默认 1.2）。
   - **模型白名单拦截**：支持精确匹配、通配符（如 `gpt-4o*`）与提供商前缀白名单限制。
   - **即时吊销**：控制台停用或删除 Key 立即在推理网关生效，返回 401 拦截。

3. **零迁移数据库复用**：
   完整对齐 SQLite 13 张数据表结构，无感挂载旧版生产 `gate.db`，保留全部历史账单流水与规则配置。

4. **事务性 Outbox 与状态持久化**：
   基于 SQLite WAL 模式，请求流水毫秒级同步写入 `usage_outbox`，每 5 秒通过单一数据库事务批量落入 `usage_daily` 与 `request_events`，启动时自动重放未落盘流水。

5. **编译期全内嵌 SPA 页面（`//go:embed`）**：
   前端资源打入单一二进制文件，配备 8 字节 Hex MD5 ETag 缓存校验，直达 `/console`，原 `/admin` 自动 301 重定向。
