# AGENTS.md

## Working Agreements

- **Git and collaboration**: run `git status --short` before editing so changes from other sessions are preserved; every change must update `CHANGELOG.md` in the same pass; commits, pushes, and deployments are executed step by step with the user's authorization.
- **Code and documentation language**: follow KISS; Go comments must be in English only (translate any non-English comment to English while editing), user-facing copy keeps its original language; documents use UTF-8 without BOM and pure LF line endings.
- **Final payload semantics barrier**: every executor (streaming / WebSocket / continuation / retry / image / billing) must first complete the built-in transformation, normalization, injection, and cleanup, and only then apply the user's payload rules exactly once; afterwards only transport frame wrapping (e.g. WorkBuddy `stream: true`), serialization, and signing are allowed. Any change to a build path must come with unit tests.
- **Error handling and logging**: follow `gofmt`/goimports; HTTP handlers must never `panic` or call `log.Fatal`; use `logrus` to log in redacted form, add a method suffix to masked variables (`errStart`), and wrap `defer Close()` errors.
- **Network timeouts and deterministic tests**: timeouts may only be set while obtaining credentials, never after the upstream connection is established (exceptions: `codex_websockets_executor.go`, `wsrelay/session.go`, `api_tools.go`, `cmd/fetch_antigravity_models`); TTL/cache unit tests must not use `time.Sleep` — use a controllable clock (`nowFunc`) instead.

## Project Boundaries

- **Authoritative upstream and reverse-engineering sources**: a Go 1.26+ multi-protocol AI proxy gateway (upstream `router-for-me/CLIProxyAPI`); for the WorkBuddy dual-realm protocol, session header families, WAF rules, and fingerprinting, `ardeyouxipianyi/workbuddy2api-hub` is the first reference source.
- **Build and test gates**: after a change, always run `gofmt -w .` and `go build -o test-output ./cmd/server && rm test-output`; run tests with `go test ./...` or `go test -v -run TestName ./path/to/pkg`.
- **Core architecture constraints**: `internal/thinking/` follows the standard pipeline (`suffix.go` → `ThinkingConfig` → `validate/convert` → `ProviderApplier`); `internal/runtime/executor/` holds only executors and their unit tests, with helpers placed in `helps/`; `internal/translator/` must not be modified on its own; `/v0/management` is deprecated; changes to `internal/home/` require a matching update to `CLIProxyAPIHome`.
- **Cross-repository static sync red line**: after modifying `static/management.html` (and `internal/managementasset/`), immediately sync it to `Project/gate/static/management.html` and verify the MD5. The native gateway console is hosted in `internal/console/` (serving `/console`, `/admin`, and `/member`), superseding the standalone Node.js implementation in `Project/gate`.
- **Configuration and detailed documentation**: `config.yaml` and `.env` are loaded by default, credentials are stored in `auths/`. For the detailed contracts see [docs/management-api-v8.md](docs/management-api-v8.md), [docs/console.md](docs/console.md), [docs/sdk-usage.md](docs/sdk-usage.md), and [llms-full.txt](llms-full.txt).
