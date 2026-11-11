#!/usr/bin/env python3
"""Self-test for tools/antigravity-words/apply.py.

Run it with no arguments; it builds fixtures in a temp dir and asserts the
tool's contract:

  * check reports drift and exits non-zero;
  * apply rewrites only the two managed keys and leaves every other byte of
    the file alone (secrets, comments, sibling keys such as connection-pool);
  * apply is idempotent -- the second run writes nothing;
  * apply on an already-synced config writes nothing at all;
  * a missing managed key is inserted rather than skipped;
  * the file stays valid YAML and the values land where CPA reads them.

    python3 tools/antigravity-words/selftest.py
"""

from __future__ import annotations

import os
import shutil
import subprocess
import sys
import tempfile

import yaml

HERE = os.path.dirname(os.path.abspath(__file__))
APPLY = os.path.join(HERE, "apply.py")
WORDS = os.path.join(os.path.dirname(HERE), "..", "examples", "antigravity-sensitive-words.yaml")
WORDS = os.path.abspath(WORDS)

CONFIG = """# CPA runtime config -- hand edited, keep my comments!
config-version: 8
management:
    secret-key: $2b$10$SUPERSECRETDONOTLEAK
    disable-control-panel: true
oauth:
    auth-dir: /root/.cli-proxy-api
    providers:
        aistudio:
            ws-auth: true
        antigravity:
            # stale list from before the fix
            sensitive-words:
                - "Claude Agent SDK"
            sensitive-words-replace: {}
            connection-pool:
                enabled: true
                idle-conn-timeout: "30s"
observability:
    logs:
        debug: true
        logging-to-file: true
api-keys:
    claude:
        - name: "claude-1"
          base-url: "https://api.minimax.cn/anthropic"
          api-key: "sk-REALKEY"
"""

EXPECTED_WORDS = ["Claude Agent SDK", "GPT-5", "Codex", "x-anthropic-billing-header", "Coding Assistant"]
EXPECTED_REPLACE = {"You are Codex, an agent based on GPT-5.": "You are a helpful coding assistant."}

failures = []


def check(condition, label):
    if condition:
        print("  ok   %s" % label)
    else:
        print("  FAIL %s" % label)
        failures.append(label)


def run(*args):
    return subprocess.run(
        [sys.executable, APPLY, *args], capture_output=True, text=True
    )


def main():
    workdir = tempfile.mkdtemp(prefix="agwords-selftest-")
    try:
        config = os.path.join(workdir, "config.yaml")
        with open(config, "w", encoding="utf-8", newline="") as handle:
            handle.write(CONFIG)
        before = open(config, "r", encoding="utf-8").read()

        print("1. check reports drift")
        result = run("check", "--config", config, "--words", WORDS)
        check(result.returncode != 0, "check exits non-zero on drift")
        check("DRIFT" in result.stderr, "check names the drift on stderr")

        print("2. apply rewrites the managed keys")
        result = run("apply", "--config", config, "--words", WORDS)
        check(result.returncode == 0, "apply exits 0")
        after = open(config, "r", encoding="utf-8").read()
        check(after != before, "the file changed")

        print("3. everything else is untouched")
        for needle, label in (
            ("# CPA runtime config -- hand edited, keep my comments!", "the header comment survives"),
            ("# stale list from before the fix", "a comment inside the block survives"),
            ("secret-key: $2b$10$SUPERSECRETDONOTLEAK", "the management secret survives"),
            ('api-key: "sk-REALKEY"', "the upstream api key survives"),
            ("enabled: true", "the sibling connection-pool.enabled survives"),
            ('idle-conn-timeout: "30s"', "the sibling connection-pool.timeout survives"),
            ("ws-auth: true", "the sibling aistudio block survives"),
            ("- name: \"claude-1\"", "the api-keys block survives"),
            ("logging-to-file: true", "the observability block survives"),
        ):
            check(needle in after, label)

        print("4. the values landed where CPA reads them")
        document = yaml.safe_load(after)
        node = document["oauth"]["providers"]["antigravity"]
        check(node["sensitive-words"] == EXPECTED_WORDS, "sensitive-words carries the tracked list")
        check(node["sensitive-words-replace"] == EXPECTED_REPLACE, "sensitive-words-replace carries the tracked map")
        check(node["connection-pool"]["enabled"] is True, "connection-pool is still there")
        check(document["management"]["secret-key"] == "$2b$10$SUPERSECRETDONOTLEAK", "the secret is unchanged")

        print("5. apply is idempotent")
        result = run("apply", "--config", config, "--words", WORDS)
        check(result.returncode == 0, "the second apply exits 0")
        check(open(config, "r", encoding="utf-8").read() == after, "the second apply writes nothing")

        print("6. check passes once synced")
        result = run("check", "--config", config, "--words", WORDS)
        check(result.returncode == 0, "check exits 0 when in sync")

        print("7. a missing managed key is inserted")
        partial = os.path.join(workdir, "partial.yaml")
        with open(partial, "w", encoding="utf-8", newline="") as handle:
            handle.write(
                "oauth:\n"
                "    providers:\n"
                "        antigravity:\n"
                "            connection-pool:\n"
                "                enabled: false\n"
            )
        result = run("apply", "--config", partial, "--words", WORDS)
        check(result.returncode == 0, "apply on a partial config exits 0")
        partial_doc = yaml.safe_load(open(partial, "r", encoding="utf-8"))
        partial_node = partial_doc["oauth"]["providers"]["antigravity"]
        check(partial_node["sensitive-words"] == EXPECTED_WORDS, "the list was inserted")
        check(partial_node["sensitive-words-replace"] == EXPECTED_REPLACE, "the map was inserted")
        check(partial_node["connection-pool"]["enabled"] is False, "the sibling key survived the insert")

        print("8. show prints the effective values")
        result = run("show", "--config", config, "--words", WORDS)
        check(result.returncode == 0, "show exits 0")
        check("GPT-5" in result.stdout, "show prints a tracked word")

        print("9. dry-run writes nothing")
        before_dry = open(partial, "r", encoding="utf-8").read()
        result = run("apply", "--config", partial, "--words", WORDS, "--dry-run")
        check(result.returncode == 0, "dry-run exits 0")
        check(open(partial, "r", encoding="utf-8").read() == before_dry, "dry-run leaves the file alone")

        print("10. a bad word list is refused")
        broken = os.path.join(workdir, "broken.yaml")
        with open(broken, "w", encoding="utf-8", newline="") as handle:
            handle.write("oauth:\n    providers:\n        antigravity:\n            unknown-key: 1\n")
        result = run("check", "--config", config, "--words", broken)
        check(result.returncode != 0, "an unmanaged key is rejected")
        check("does not manage" in result.stderr, "the rejection explains itself")
    finally:
        shutil.rmtree(workdir, ignore_errors=True)

    print("")
    if failures:
        print("%d check(s) FAILED:" % len(failures))
        for label in failures:
            print("  - %s" % label)
        return 1
    print("all checks passed")
    return 0


if __name__ == "__main__":
    sys.exit(main())
