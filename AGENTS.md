# AGENTS.md

Go 1.26+ proxy server providing OpenAI/Gemini/Claude/Codex compatible APIs with OAuth and round-robin load balancing.

## Repository
- GitHub: https://github.com/router-for-me/CLIProxyAPI

## Commands
```bash
gofmt -w . # Format (required after Go changes)
go build -o cli-proxy-api ./cmd/server # Build
go run ./cmd/server # Run dev server
go test ./... # Run all tests
go test -v -run TestName ./path/to/pkg # Run single test
go build -o test-output ./cmd/server && rm test-output # Verify compile (REQUIRED after changes)
```
- Common flags: `--config <path>`, `--tui`, `--standalone`, `--local-model`, `--no-browser`, `--oauth-callback-port <port>`

## Config
- Default config: `config.yaml` (template: `config.example.yaml`)
- `.env` is auto-loaded from the working directory
- Auth material defaults under `auths/`
- Storage backends: file-based default; optional Postgres/git/object store (`PGSTORE_*`, `GITSTORE_*`, `OBJECTSTORE_*`)

## Architecture
- `cmd/server/` — Server entrypoint
- `internal/api/` — Gin HTTP API (routes, middleware, modules)
- `internal/api/modules/amp/` — Amp integration (Amp-style routes + reverse proxy)
- `internal/thinking/` — Main thinking/reasoning pipeline. `ApplyThinking()` (apply.go) parses suffixes (`suffix.go`, suffix overrides body), normalizes config to canonical `ThinkingConfig` (`types.go`), normalizes and validates centrally (`validate.go`/`convert.go`), then applies provider-specific output via `ProviderApplier`. Do not break this "canonical representation → per-provider translation" architecture.
- `internal/runtime/executor/` — Per-provider runtime executors (incl. Codex WebSocket)
- `internal/translator/` — Provider protocol translators (and shared `common`)
- `internal/registry/` — Model registry + remote updater (`StartModelsUpdater`); `--local-model` disables remote updates
- `internal/store/` — Storage implementations and secret resolution
- `internal/managementasset/` — Config snapshots and management assets
- `internal/cache/` — Request signature caching
- `internal/watcher/` — Config hot-reload and watchers
- `internal/wsrelay/` — WebSocket relay sessions
- `internal/usage/` — Usage and token accounting
- `internal/home/` — CLIProxyAPIHome control plane integration (bootstrap, RESP communication, dispatch coordination)
- `internal/tui/` — Bubbletea terminal UI (`--tui`, `--standalone`)
- `sdk/cliproxy/` — Embeddable SDK entry (service/builder/watchers/pipeline)
- `test/` — Cross-module integration tests

## Code Conventions
- Keep changes small and simple (KISS)
- Comments in English only
- If editing code that already contains non-English comments, translate them to English (don’t add new non-English comments)
- For user-visible strings, keep the existing language used in that file/area
- New Markdown docs should be in English unless the file is explicitly language-specific (e.g. `README_CN.md`)
- As a rule, do not make standalone changes to `internal/translator/`. You may modify it only as part of broader changes elsewhere.
- If a task requires changing only `internal/translator/`, run `gh repo view --json viewerPermission -q .viewerPermission` to confirm you have `WRITE`, `MAINTAIN`, or `ADMIN`. If you do, you may proceed; otherwise, file a GitHub issue including the goal, rationale, and the intended implementation code, then stop further work.
- `internal/runtime/executor/` should contain executors and their unit tests only. Place any helper/supporting files under `internal/runtime/executor/helps/`.
- Payload configuration MUST be the final semantic barrier before sending requests in every executor, including streaming, WebSocket, continuation, retry/fallback, image, and token-count paths where applicable. Complete all built-in payload translation, normalization, injection, and cleanup first, then evaluate and apply user payload rules exactly once to the final business payload. No subsequent logic may overwrite configured values or restore filtered fields. Only necessary transport framing, serialization, signature calculation, and read-only validation may follow without changing business payload semantics. Add regression tests when introducing or changing request-building paths to enforce this ordering.
- Follow `gofmt`; keep imports goimports-style; wrap errors with context where helpful
- Do not use `log.Fatal`/`log.Fatalf` (terminates the process); prefer returning errors and logging via logrus
- Shadowed variables: use method suffix (`errStart := server.Start()`)
- Wrap defer errors: `defer func() { if err := f.Close(); err != nil { log.Errorf(...) } }()`
- Use logrus structured logging; avoid leaking secrets/tokens in logs
- Avoid panics in HTTP handlers; prefer logged errors and meaningful HTTP status codes
- Timeouts are allowed only during credential acquisition; after an upstream connection is established, do not set timeouts for any subsequent network behavior. Intentional exceptions that must remain allowed are the Codex websocket liveness deadlines in `internal/runtime/executor/codex_websockets_executor.go`, the wsrelay session deadlines in `internal/wsrelay/session.go`, the management APICall timeout in `internal/api/handlers/management/api_tools.go`, and the `cmd/fetch_antigravity_models` utility timeouts
- Avoid wall-clock `time.Sleep` in TTL, expiration, ordering, or cache-eviction unit tests due to platform timer granularity (e.g. Windows default timer resolution of ~15.6ms) and CI jitter under load; prefer controllable clocks (`nowFunc` / mock clock), explicit timestamp manipulation, or deterministic synchronization primitives.
- Note: if modifying features that involve CLIProxyAPIHome, check if corresponding updates are needed in the CLIProxyAPIHome repository.
- Endpoints under the `/v0/management` base URL are deprecated and no longer maintained. For any feature changes, do not modify endpoints under `/v0/management` unless necessary to fix compilation errors.
- WorkBuddy reverse-engineering, anti-detection cloaking, conversation header families, and WAF rules: Refer to https://github.com/ardeyouxipianyi/workbuddy2api-hub as the authoritative source of truth for upstream Tencent/WorkBuddy dual-realm protocol changes, fingerprinting, and account maintenance. For any WorkBuddy upstream issues, check this project for updates first.

<!-- NEXUS-COORDINATION-START -->
## Agent-Nexus 多 Agent 并发协同与工程记忆纪律

本工程已接入 **Agent-Nexus** 协同中枢与常驻 AI 架构师（Tech Lead）。所有在本工作区并发作业的 AI Agent 必须遵守以下协同三原则：

1. **进场声明意图（Onboarding）**：
   - 开工前调用 MCP 工具 `nexus_onboard` 申报任务意图与预期修改文件列表；
   - 若遇到资源占用，系统将自动划分出“安全施工走廊（safeToStart）”与“避障等待队列（mustWait）”，请优先开发空闲模块，避免盲目重试。

2. **排他资源锁与跨工作树防撞（Resource Locks & Worktree）**：
   - 运行独占性命令前申报排他资源锁（例如 `tests`、`dev-server`、`build`）；
   - 在 Git Worktree 中修改文件时，Nexus 会自动基于主仓库共同根做跨工作树相对化对齐，若出现 `CROSS-WORKTREE CONFLICT` 警示，必须立即规避以杜绝 Git 合并冲突。

3. **完工释放与经验沉淀（Release & Long-Term Memory）**：
   - 阶段任务完工后立即调用 `nexus_release_lease` 释放锁定，唤醒后排等待会话；
   - 重大架构决策、避坑经验与代码规约，调用 `nexus_save_memory` 沉淀到项目的长期记忆库中，供后续会话持续复用；
   - 达成里程碑时调用 `nexus_report_milestone` 广播战果。
<!-- NEXUS-COORDINATION-END -->
