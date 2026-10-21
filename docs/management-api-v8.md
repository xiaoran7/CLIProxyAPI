# v8 Management API

Base path: `/v8/management` (follows [config.example.yaml](../config.example.yaml)). Authenticate via `Authorization: Bearer <management-key>` or `X-Management-Key: <management-key>` (OAuth callbacks validate `state` without a management key; Home mode disables local management endpoints). Legacy `/v0/management` remains for backward compatibility only.

## Configuration Endpoints

All paths are relative to `/v8/management`.

| Path | Methods | Description |
| :--- | :--- | :--- |
| `/config` | GET, PUT, PATCH | Read, replace, or merge the complete configuration as JSON (supports SHA-256 ETag / `If-Match`). |
| `/config.yaml` | GET, PUT | Read or replace the complete configuration as YAML. |
| `/config/<section>/<field>` | GET, PUT, PATCH, DELETE | Read or modify a specific section or field in the v8 configuration tree. |

- **Semantics**: `GET` returns the v8 layout without mutating the file. JSON writes take raw values directly (no `{"value": ...}` envelope). `PUT` replaces the target; `PATCH` merges objects and replaces lists/scalars while preserving YAML comments on scalar updates; `DELETE` removes a field.
- **Redaction & Locks**: JSON reads omit TURN credentials under `oauth.providers.codex.live-media-relay.ice-servers` (included in YAML reads). Home-owned revision fields (`credentials.concurrency.*-revision`, `plugins.auth-revision`) are read-only.
- **Examples**: `/config/access/api-keys`, `/config/api-keys/codex`, `/config/client/codex/optimize-multi-agent-v2` (canonical setting migrating legacy `codex.*` aliases), `/config/observability/logs/debug`, `/config/routing/retry/request-retry`, `/config/plugins/configs/<id>`.

## Operational Endpoints

| Path | Methods | Description |
| :--- | :--- | :--- |
| `/config/upstream/<provider>` | GET, PUT, PATCH, DELETE | Manage shared provider settings. |
| `/server/latest-version` | GET | Get latest release information. |
| `/requests/api-call` | POST | Make an authenticated upstream call. |
| `/routing/cooldown/reset` | POST | Clear credential cooldown and rate-limit sinks. |
| `/routing/model-definitions/<channel>` | GET | Get model definitions for a channel. |
| `/models/global-excluded-models` | GET, PUT, PATCH, DELETE | Manage global exit model exclusions (also `/routing/global-excluded-models`). |
| `/models/global-excluded-models/toggle` | POST | Toggle exclusion state for a specific model. |
| `/models/exit-catalog` | GET | List all active upstream models with exclusion status. |
| `/observability/logs` | GET, DELETE | Read or clear application logs. |
| `/observability/logs/errors` | GET | List error-log files (`/observability/logs/errors/<name>` to download). |
| `/observability/logs/requests/<id>` | GET | Get a specific request log by ID. |
| `/observability/usage/api-keys` | GET | Get API-key usage (`/observability/usage/queue` for queued events). |
| `/credentials` | GET, POST, DELETE | List, upload, or delete credential files (excludes internal state files). |
| `/credentials/models` | GET | Get models exposed by credentials (`/credentials/download` to download file). |
| `/credentials/status` | PATCH | Change credential status (`/credentials/fields` to patch attributes). |
| `/credentials/refresh` | POST | Trigger credential token refresh. |
| `/oauth/import?provider=vertex` | POST | Import a Vertex service account via multipart `file` upload. |
| `/oauth/auth-url?provider=<provider>` | GET | Start OAuth (`claude`, `codex`, `antigravity`, `workbuddy`, `workbuddy-cn`, `kimi`, `kimi-ai`, `xai`, `devin`, `meta`, or plugin ID). |
| `/oauth/siwc/bind` | POST | Start SIWC binding to an existing file-backed Codex OAuth credential. |
| `/oauth/siwc/status` | GET | Read local SIWC credential status (`off`, `valid`, or `invalid`; also `/credentials/siwc/status`). |
| `/oauth/status?state=<state>` | GET | Poll OAuth status (`wait` -> `ok` or `error`; `DELETE /oauth/session?state=<state>` to cancel). |
| `/oauth/callback` | GET, POST | Submit OAuth callback (`provider`, `state`, `code`, `error`, `redirect_url`). |
| `/plugins` | GET | List installed plugins (`DELETE /plugins/<id>` to remove). |
| `/plugins/store` | GET | List plugin store (`POST /plugins/store/<id>/install` to install/update). |
| `/plugins/<id>/quota` | GET, POST, DELETE | Read, fetch, or reset plugin quota (including `workbuddy` batch quotas). |
| `/workbuddy/checkin` | GET, PUT, POST | Query check-in records, toggle `CheckinEngine` (`{"enabled": bool}`), or trigger check-in. |
| `/workbuddy/compensation` | GET, POST | Query or claim WorkBuddy outage/activity credit compensations. |

### Sign in with ChatGPT (SIWC) binding

SIWC is a second credential partition of an existing Codex credential, not a new provider. Available only in standalone CPA with local file-backed Codex OAuth credentials (`Home` mode and non-file storage backends unsupported).

Start binding with `POST /v8/management/oauth/siwc/bind` and `{"name":"codex-example.json"}` (`name` matches an existing managed credential ID or file name). Returns authorization URL, random `state`, and `expires_in: 300`. Redirect URI is fixed to `http://127.0.0.1:1455/auth/callback`. Submit the callback to `POST /v8/management/oauth/callback` with `redirect_url` or direct `state`, `code`, and issued `client_id`. Poll `GET /oauth/status?state=<state>` until `ok` or `error`. Cancel with `DELETE /oauth/session?state=<state>`.

Query local SIWC status with `GET /v8/management/oauth/siwc/status?name=<name>` (or `?auth_index=<auth_index>`), which returns `{"status": "off" | "valid" | "invalid"}`.

For Codex entries, `GET /credentials` exposes `siwc_bind_supported` (bool), `use_siwc` (bool), `siwc_status` (string: `off` | `valid` | `invalid`), `siwc_bound` (bool), and `siwc_expires_at` (RFC3339 string).

