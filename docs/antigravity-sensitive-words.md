# Antigravity Sensitive-Word List

The Antigravity obfuscation word list is **tracked in git**. The runtime
`config.yaml` is not, because it holds the management secret and the upstream
API keys. This document says where the list lives, how it reaches the runtime
config, and how to prove the two have not drifted apart.

## Where the list lives

| Copy | Path | Tracked | Holds |
| :--- | :--- | :--- | :--- |
| Source of truth | [`examples/antigravity-sensitive-words.yaml`](../examples/antigravity-sensitive-words.yaml) | yes | the word list only -- no secrets |
| Runtime | `config.yaml` -> `oauth.providers.antigravity` | no (`.gitignore`) | the same list, next to every secret |

The tracked file is the one to edit. The runtime copy is derived from it.

## Applying it

```bash
python3 tools/antigravity-words/apply.py check  --config /path/to/config.yaml
python3 tools/antigravity-words/apply.py apply  --config /path/to/config.yaml [--dry-run]
python3 tools/antigravity-words/apply.py show   --config /path/to/config.yaml
```

`check` exits non-zero and prints a diff when the runtime config has drifted
from the tracked list; it is meant to be usable as a CI or cron gate.

`apply` rewrites **only** `sensitive-words` and `sensitive-words-replace`
inside `oauth.providers.antigravity`. Every other byte of the file survives:
the management secret, the upstream API keys, the operator's comments, and
sibling keys such as `connection-pool`. A config that is already in sync is
not written at all, so the tool never churns a healthy production file.

### Where it runs

- **Tokyo production (`vps-tokyo-02`)**: the runtime file is
  `/opt/1panel/docker/compose/cliproxyapi/config.yaml`, bind-mounted into the
  container as `/CLIProxyAPI/config.yaml`. CPA watches it and hot-reloads the
  word list; no restart is needed after `apply`.

### Self-test

```bash
python3 tools/antigravity-words/selftest.py
```

The self-test builds fixtures in a temp directory and asserts the tool's
contract: drift is reported, only the managed keys are rewritten, the file
stays valid YAML, a missing key is inserted, `apply` is idempotent, and a
word list carrying keys the tool does not manage is refused.

## How CPA reads the list

1. `internal/config/config_v8.go` -- the v8 path table entry
   `{"antigravity", "oauth.providers.antigravity"}` moves the v8 layout back
   to the flat legacy key `antigravity` during parsing.
2. `internal/config/config.go` -- `Config.Antigravity` (`AntigravityConfig`)
   receives `sensitive-words` and `sensitive-words-replace`.
3. `internal/watcher/diff/config_diff.go` -- the field is diffed, so editing
   the runtime config hot-reloads the word list.

## Why the list exists

Since 2026-11-10 19:00 UTC the upstream Antigravity endpoint strips U+200B
before matching, which turned the old zero-width-character obfuscation into a
no-op. A request whose system instruction carried an AI-tool identity phrase
came back as `429 RESOURCE_EXHAUSTED`. CPA read that synthetic 429 as quota
exhaustion, retried across healthy credentials and cooled them down, which
surfaced to clients as `503 "auth_not_found: no auth available"`.

The fix is upstream commit
[`9a7e1e6b`](https://github.com/router-for-me/CLIProxyAPI/commit/9a7e1e6b412747729dedecefd7e23bd2e3ee1d3d)
("add sensitive-words-replace and harden Antigravity obfuscation", 2026-11-11),
adopted by this fork in `f633c2216`: Antigravity system-instruction
obfuscation switched from U+200B to per-subword U+034F (CGJ), and the literal
`sensitive-words-replace` map was added. The algorithm is upstream's, unchanged.

Keeping the word list itself in git is what makes the deployment
reproducible: without it, rebuilding or replacing the host would silently
drop the list and bring the synthetic-429 failure mode back. Per the
project's upstream-first rule, report identity-quota regressions upstream
before patching locally.
