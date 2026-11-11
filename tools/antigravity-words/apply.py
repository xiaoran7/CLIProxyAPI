#!/usr/bin/env python3
"""Apply / verify the tracked Antigravity obfuscation word list.

The word list is tracked in ``examples/antigravity-sensitive-words.yaml``
(secret-free, so it can live in git).  The runtime ``config.yaml`` is
gitignored because it holds the management secret and the upstream API keys.
This tool is the bridge between the two.

It merges **only** the two managed keys under the v8 path
``oauth.providers.antigravity``:

    sensitive-words          (list of strings)
    sensitive-words-replace  (map of string -> string)

Everything else in the target file -- every secret, every operator comment,
every other provider block -- is left byte-for-byte alone.  A key that is
already in sync is not rewritten at all, so running the tool twice leaves the
file unchanged, and running it against an up-to-date file writes nothing.

Usage:
    apply.py show   --config PATH
    apply.py check  --config PATH            # exit 1 when the config drifts
    apply.py apply  --config PATH [--dry-run]

Exit codes:
    0  success / in sync
    1  drift detected (check) or a usage/IO error
    2  apply failed
"""

from __future__ import annotations

import argparse
import os
import re
import sys

import yaml

MANAGED_KEYS = ("sensitive-words", "sensitive-words-replace")
TARGET_PATH = ("oauth", "providers", "antigravity")

REPO_ROOT = os.path.abspath(os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", ".."))
DEFAULT_WORDS = os.path.join(REPO_ROOT, "examples", "antigravity-sensitive-words.yaml")

_KEY_RE = re.compile(r"^([^:#]+):(\s|$)")


class ToolError(Exception):
    """A problem the operator has to fix (bad path, unparsable YAML, ...)."""


# --------------------------------------------------------------------------
# YAML helpers
# --------------------------------------------------------------------------


def load_yaml(path):
    with open(path, "r", encoding="utf-8") as handle:
        return yaml.safe_load(handle)


def get_at(doc, path):
    node = doc
    for key in path:
        if not isinstance(node, dict) or key not in node:
            return None
        node = node[key]
    return node


def scalar(value):
    """Render a scalar the way the tracked fragment writes it: double quoted."""
    if isinstance(value, bool):
        return "true" if value else "false"
    if value is None:
        return "null"
    if isinstance(value, (int, float)):
        return str(value)
    text = str(value)
    text = text.replace("\\", "\\\\").replace('"', '\\"')
    return '"%s"' % text


# --------------------------------------------------------------------------
# Raw-text block location
# --------------------------------------------------------------------------


def _indent_of(line):
    return len(line) - len(line.lstrip(" "))


def _is_skippable(line):
    stripped = line.strip()
    return not stripped or stripped.startswith("#")


def find_key(lines, start, end, indent, key):
    """Locate ``key`` at exactly ``indent`` between lines[start:end].

    Returns (line_index, found) or (None, False).
    """
    index = start
    while index < end:
        line = lines[index]
        if _is_skippable(line):
            index += 1
            continue
        current = _indent_of(line)
        if current < indent:
            return None, False
        if current > indent:
            index += 1
            continue
        match = _KEY_RE.match(line.strip())
        if match and match.group(1).strip().strip("\"'") == key:
            return index, True
        index += 1
    return None, False


def block_end(lines, index, indent):
    """First line after the block opened at ``index`` whose indent drops back."""
    cursor = index + 1
    while cursor < len(lines):
        line = lines[cursor]
        if not _is_skippable(line):
            if _indent_of(line) <= indent:
                break
        cursor += 1
    return cursor


def detect_unit(lines):
    """The indentation step used by the file, e.g. 4 for four-space YAML."""
    for line in lines:
        if _is_skippable(line):
            continue
        indent = _indent_of(line)
        if indent > 0:
            return indent
    return 4


def locate_antigravity(lines):
    """Find the ``oauth.providers.antigravity`` mapping.

    Returns (key_line, child_indent, unit) or raises ToolError.
    """
    unit = detect_unit(lines)
    oauth_index, found = find_key(lines, 0, len(lines), 0, "oauth")
    if not found:
        raise ToolError("the config has no top-level 'oauth:' block")
    oauth_end = block_end(lines, oauth_index, 0)

    providers_index, found = find_key(lines, oauth_index + 1, oauth_end, unit, "providers")
    if not found:
        raise ToolError("the config has no 'oauth.providers:' block")
    providers_end = block_end(lines, providers_index, unit)

    antigravity_index, found = find_key(lines, providers_index + 1, providers_end, unit * 2, "antigravity")
    if not found:
        raise ToolError("the config has no 'oauth.providers.antigravity:' block")
    return antigravity_index, unit * 3, unit


# --------------------------------------------------------------------------
# Rendering
# --------------------------------------------------------------------------


def render_value(key, value, indent, unit):
    """Render one managed key as a list of YAML lines at ``indent``."""
    pad = " " * indent
    inner = " " * (indent + unit)
    lines = ["%s%s:" % (pad, key)]
    if isinstance(value, list):
        for item in value:
            lines.append("%s- %s" % (inner, scalar(item)))
    elif isinstance(value, dict):
        for map_key, map_value in value.items():
            lines.append("%s%s: %s" % (inner, scalar(map_key), scalar(map_value)))
    else:
        lines.append("%s%s" % (lines.pop(), scalar(value)))
    return lines


def render_block(data, indent, unit):
    lines = []
    for key in MANAGED_KEYS:
        lines.extend(render_value(key, data[key], indent, unit))
    return lines


# --------------------------------------------------------------------------
# Sync
# --------------------------------------------------------------------------


def collect_words(words_path):
    """Read the tracked fragment and return the managed mapping."""
    document = load_yaml(words_path)
    node = get_at(document, TARGET_PATH)
    if not isinstance(node, dict):
        raise ToolError(
            "the tracked word list must define %s" % ".".join(TARGET_PATH)
        )
    unknown = sorted(set(node) - set(MANAGED_KEYS))
    if unknown:
        raise ToolError(
            "the tracked word list carries keys this tool does not manage: %s"
            % ", ".join(unknown)
        )
    missing = [key for key in MANAGED_KEYS if key not in node]
    if missing:
        raise ToolError("the tracked word list is missing: %s" % ", ".join(missing))
    return {key: node[key] for key in MANAGED_KEYS}


def current_values(config_path):
    """The managed values as they currently sit in the runtime config."""
    document = load_yaml(config_path)
    node = get_at(document, TARGET_PATH)
    if not isinstance(node, dict):
        return {key: None for key in MANAGED_KEYS}
    return {key: node.get(key) for key in MANAGED_KEYS}


def in_sync(config_path, words):
    current = current_values(config_path)
    return all(current[key] == words[key] for key in MANAGED_KEYS)


def apply_words(config_path, words, dry_run=False):
    """Merge the tracked word list into the runtime config.

    Returns (changed, message).
    """
    with open(config_path, "r", encoding="utf-8") as handle:
        original = handle.read()

    if in_sync(config_path, words):
        return False, "already in sync; the config was not touched"

    lines = original.splitlines()
    antigravity_index, child_indent, unit = locate_antigravity(lines)
    antigravity_end = block_end(lines, antigravity_index, unit * 2)

    # Rewrite each managed key in place; everything else in the block stays.
    for key in reversed(MANAGED_KEYS):
        key_index, found = find_key(lines, antigravity_index + 1, antigravity_end, child_indent, key)
        rendered = render_value(key, words[key], child_indent, unit)
        if found:
            end = block_end(lines, key_index, child_indent)
            lines[key_index:end] = rendered
        else:
            insert_at = antigravity_end
            lines[insert_at:insert_at] = rendered
            antigravity_end += len(rendered)
            # Inserted lines shift every later index; re-locate the block.
            antigravity_index, child_indent, unit = locate_antigravity(lines)
            antigravity_end = block_end(lines, antigravity_index, unit * 2)

    updated = "\n".join(lines) + ("\n" if original.endswith("\n") else "")
    if updated == original:
        return False, "already in sync; the config was not touched"
    if dry_run:
        return True, "would update %s" % config_path
    with open(config_path, "w", encoding="utf-8", newline="") as handle:
        handle.write(updated)
    return True, "updated %s" % config_path


# --------------------------------------------------------------------------
# CLI
# --------------------------------------------------------------------------


def format_value(value):
    if isinstance(value, dict):
        return "\n".join("      %s: %s" % (scalar(k), scalar(v)) for k, v in value.items())
    if isinstance(value, list):
        return "\n".join("      - %s" % scalar(item) for item in value)
    return "      %s" % scalar(value)


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    parser.add_argument("command", choices=("show", "check", "apply"))
    parser.add_argument("--config", required=True, help="path to the runtime config.yaml")
    parser.add_argument("--words", default=DEFAULT_WORDS, help="path to the tracked word list")
    parser.add_argument("--dry-run", action="store_true", help="apply without writing")
    args = parser.parse_args(argv)

    try:
        if not os.path.isfile(args.config):
            raise ToolError("no such config file: %s" % args.config)
        words = collect_words(args.words)
    except ToolError as exc:
        print("error: %s" % exc, file=sys.stderr)
        return 1
    except yaml.YAMLError as exc:
        print("error: cannot parse %s: %s" % (args.words, exc), file=sys.stderr)
        return 1

    if args.command == "show":
        current = current_values(args.config)
        print("%s in %s:" % (".".join(TARGET_PATH), args.config))
        for key in MANAGED_KEYS:
            print("  %s:" % key)
            print(format_value(current[key]))
        return 0

    if args.command == "check":
        current = current_values(args.config)
        drifted = [key for key in MANAGED_KEYS if current[key] != words[key]]
        if not drifted:
            print("in sync: %s matches %s" % (args.config, args.words))
            return 0
        print("DRIFT in %s:" % args.config, file=sys.stderr)
        for key in drifted:
            print("  %s" % key, file=sys.stderr)
            print("    tracked:", file=sys.stderr)
            print(format_value(words[key]), file=sys.stderr)
            print("    runtime:", file=sys.stderr)
            print(format_value(current[key]), file=sys.stderr)
        return 1

    try:
        changed, message = apply_words(args.config, words, dry_run=args.dry_run)
    except ToolError as exc:
        print("error: %s" % exc, file=sys.stderr)
        return 2
    except yaml.YAMLError as exc:
        print("error: cannot parse %s: %s" % (args.config, exc), file=sys.stderr)
        return 2
    print(message)
    return 0


if __name__ == "__main__":
    sys.exit(main())
