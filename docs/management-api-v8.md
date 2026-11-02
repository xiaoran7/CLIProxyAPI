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
| `/credentials/verify` | POST | Run an operator-triggered, account-pinned inference verification using an isolated session and minimal 1-token prompt. |
| `/oauth/import?provider=vertex` | POST | Import a Vertex service account via multipart `file` upload. |
| `/oauth/auth-url?provider=<provider>` | GET, POST | Start built-in or plugin OAuth (`claude`, `codex`, `antigravity`, `workbuddy`, `workbuddy-cn`, `kimi`, `kimi-ai`, `xai`, `devin`, `meta`, or plugin ID; supports `name`/`auth_index` for targeted credential recovery and optional `verify` opt-in). |
| `/oauth/siwc/bind` | POST | Start SIWC binding to an existing file-backed Codex OAuth credential. |
| `/oauth/siwc/status` | GET | Read local SIWC credential status (`off`, `valid`, or `invalid`; also `/credentials/siwc/status`). |
| `/oauth/status?state=<state>` | GET | Poll OAuth status (`wait` -> `ok` or `error`; `DELETE /oauth/session?state=<state>` to cancel). |
| `/oauth/callback` | GET, POST | Submit OAuth callback (`provider`, `state`, `code`, `error`, `redirect_url`). |
| `/plugins` | GET | List installed plugins. |
| `/plugins/<id>` | DELETE | Delete a plugin. |
| `/plugins/store` | GET | List the plugin store (`POST /plugins/store/<id>/install` to install/update). |
| `/plugins/<id>/quota` | GET, POST, DELETE | Read, fetch, or reset plugin quota (including `workbuddy` batch quotas). |
| `/workbuddy/checkin` | GET, PUT, POST | Query check-in records, toggle `CheckinEngine` (`{"enabled": bool}`), or trigger check-in. |
| `/workbuddy/compensation` | GET, POST | Query or claim WorkBuddy outage/activity credit compensations. |

### Sign in with ChatGPT (SIWC) binding

SIWC is a second credential partition of an existing Codex credential, not a new provider. Available only in standalone CPA with local file-backed Codex OAuth credentials (`Home` mode and non-file storage backends unsupported).

Start binding with `POST /v8/management/oauth/siwc/bind` and `{"name":"codex-example.json"}` (`name` matches an existing managed credential ID or file name). Returns authorization URL, random `state`, and `expires_in: 300`. Redirect URI is fixed to `http://127.0.0.1:1455/auth/callback`. Submit the callback to `POST /v8/management/oauth/callback` with `redirect_url` or direct `state`, `code`, and issued `client_id`. Poll `GET /oauth/status?state=<state>` until `ok` or `error`. Cancel with `DELETE /oauth/session?state=<state>`.

Query local SIWC status with `GET /v8/management/oauth/siwc/status?name=<name>` (or `?auth_index=<auth_index>`), which returns `{"status": "off" | "valid" | "invalid"}`.

For Codex entries, `GET /credentials` exposes `siwc_bind_supported` (bool), `use_siwc` (bool), `siwc_status` (string: `off` | `valid` | `invalid`), `siwc_bound` (bool), and `siwc_expires_at` (RFC3339 string).

`name` identifies an existing managed credential by its ID or file name; it is not
an arbitrary filesystem path. The credential must contain nonempty outer Codex
access and refresh tokens. A successful start returns:

```json
{
  "status": "ok",
  "url": "https://auth.openai.com/api/accounts/authorize?...",
  "state": "<random-state>",
  "flow": "siwc_bind",
  "expires_in": 300
}
```

Opening the returned URL starts authorization. Its redirect URI is fixed to
`http://127.0.0.1:1455/auth/callback`. This endpoint does not start a listener on
port 1455: for remote deployments, paste the complete URL from the browser address
bar, even if the loopback page cannot be reached, and submit it to the server:

```http
POST /v8/management/oauth/callback
Content-Type: application/json

{"provider":"codex","redirect_url":"http://127.0.0.1:1455/auth/callback?state=<state>&code=<code>&client_id=<issued-client-id>"}
```

The v8 callback also accepts `state`, `code`, and `client_id` directly as POST JSON
fields or GET query parameters. SIWC requires the **issued** `client_id` from the
callback; the authorization URL's `dynamic_agent_client` is not a substitute.
Error callbacks can supply `error` instead of `code` and `client_id`. Conflicting
parameters are rejected. The target, PKCE verifier, device ID, and redirect URI
are retained on the server and cannot be changed by a callback. Callback values
are handed directly to the SIWC session rather than written to legacy callback
files. Neither callbacks nor status responses return tokens.

Poll `GET /oauth/status?state=<state>` until `ok` or `error`. `wait` means binding
has not completed, including while tokens are being exchanged. Success means the
new `siwc` partition and `use_siwc=true` have been saved and published to the auth
manager. Other credential fields are retained. Rebinding explicitly replaces the
SIWC partition and reuses a valid existing device ID.

`DELETE /oauth/session?state=<state>` cancels a pending binding. Cancellation that
wins the commit gate prevents saving a late token response; cancellation after
commit returns `cancelled:false`. Duplicate callbacks and a second pending binding
for the same credential return `409`. Missing targets return `404`; ineligible
targets and incomplete callbacks return `400`. Deleting or replacing the target
through the v8 credential API invalidates its pending binding. An old server
returns `404` for the start endpoint; clients must not fall back to ordinary Codex
login. Sessions are process-local, expire after five minutes, and do not survive
restart; deployments with multiple independent instances require session affinity.

Query local SIWC status for a Codex credential with `GET /v8/management/oauth/siwc/status?name=<name>` (or `?auth_index=<auth_index>`). The endpoint reads local credentials and runtime rejection state without triggering refresh or upstream verification requests, and returns `{"status": "off" | "valid" | "invalid"}`.

For Codex entries, `GET /credentials` includes these non-secret summary fields:

| Field | Type | Meaning |
| --- | --- | --- |
| `siwc_bind_supported` | boolean | This credential is eligible for the binding flow. Missing means unsupported. |
| `use_siwc` | boolean | Requests are configured to use the nested SIWC credential. |
| `siwc_status` | string | Local SIWC status: `off`, `valid`, or `invalid`. |
| `siwc_bound` | boolean | SIWC access token, refresh token, and issued client ID are present; not a live validity check. |
| `siwc_expires_at` | optional RFC3339 string | Stored SIWC expiration, when available. |
| `access_token_expires_at` | optional RFC3339 string | Active access token expiration deadline, when available. |
| `next_refresh_after` | optional RFC3339 string | Next scheduled automatic refresh check, when in the future. |
| `requires_sign_in` | boolean | Automatic renewal failed with a terminal grant rejection and independent sign-in is required. |
| `renewal_state` | string | Renewal lifecycle state: `sign_in_required`, `expired_sign_in_required`, `retry_scheduled`, `awaiting_first_renewal`, or `renewal_verified`. |
| `recovery_stage` | string | Post-login runtime adoption and verification stage: `sign_in_required`, `quota_blocked`, `loaded`, `inference_verified`, or `renewal_verified`. |
| `adopted` | boolean | Whether the current credential version is loaded in the running auth manager. |
| `inference_verified` | boolean | Whether the current credential version has completed an inference response. |
| `renewal_verified` | boolean | Whether the current credential version has completed an automatic refresh cycle. |
| `refresh_error` | optional object | Sanitized last refresh failure (`code`, `message`, `http_status`) with no tokens, codes, or URLs. |
| `next_action` | optional string | Required operator recovery action (`sign_in` when `requires_sign_in` is true). |
| `recovery_hint` | optional string | Operator guidance explaining instance-scoped OAuth recovery when `requires_sign_in` is true. |
| `renewal_event` | optional object | Deduplicated structured event (`renewal_sign_in_required`) for terminal renewal failures. |

The v0 callback and credential-list contracts are unchanged. SIWC activation,
unbinding, and standalone SIWC credential creation are not separate management
operations in this release.

## v0 compatibility

The `/v0/management` endpoints keep their original paths and payloads, including
provider-specific OAuth login URLs, callbacks, status queries, and cancellation.
For example, `GET /v0/management/codex-auth-url` and
`GET /v8/management/oauth/auth-url?provider=codex` start the same login flow.

Legacy flat configuration endpoints such as `/debug`, `/request-retry`, and
`/codex-api-key` exist under `/v0/management` only.
`/v0/management/api-keys` continues to manage client authentication keys; the v8
equivalent is `/v8/management/config/access/api-keys`.

Legacy-only configuration files keep their layout until a successful v8
configuration write. When both layouts specify a field, the new field takes
precedence and its legacy equivalent is removed. V0 setters keep legacy-only
files in their original layout; existing v8 files are saved in the latest v8
layout. `PUT /v0/management/config.yaml` still replaces the complete file and
accepts legacy, new, or mixed layouts, normalizing v8 documents on save.

Plugin OAuth uses the shared v8 login endpoint. Other plugin-defined HTTP
extensions retain their declared `/v0/management` routes.
