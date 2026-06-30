#!/usr/bin/env python3
"""Sync missing local config entries from example files without overwriting values."""

from __future__ import annotations

import argparse
import re
import shutil
from dataclasses import dataclass
from pathlib import Path


TOP_LEVEL_KEY_RE = re.compile(r"^(?P<comment>#\s*)?(?P<key>[A-Za-z0-9_-]+):(?:\s|$)")
ENV_KEY_RE = re.compile(r"^(?:export\s+)?(?P<key>[A-Za-z_][A-Za-z0-9_]*)=")
COMMENTED_ENV_KEY_RE = re.compile(r"^#\s*(?:export\s+)?(?P<key>[A-Za-z_][A-Za-z0-9_]*)=")


@dataclass
class Block:
    key: str
    start: int
    end: int
    lines: list[str]


def read_lines(path: Path) -> list[str]:
    return path.read_text(encoding="utf-8").splitlines(keepends=True)


def write_lines(path: Path, lines: list[str]) -> None:
    path.write_text("".join(lines), encoding="utf-8")


def ensure_trailing_newline(lines: list[str]) -> list[str]:
    if lines and not lines[-1].endswith("\n"):
        return [*lines[:-1], f"{lines[-1]}\n"]
    return lines


def top_level_key(line: str) -> str | None:
    match = TOP_LEVEL_KEY_RE.match(line)
    if not match:
        return None
    return match.group("key")


def yaml_blocks(lines: list[str]) -> list[Block]:
    starts: list[tuple[int, str]] = []
    for index, line in enumerate(lines):
        key = top_level_key(line)
        if key:
            starts.append((index, key))
    blocks: list[Block] = []
    for pos, (start, key) in enumerate(starts):
        end = starts[pos + 1][0] if pos + 1 < len(starts) else len(lines)
        blocks.append(Block(key=key, start=start, end=end, lines=lines[start:end]))
    return blocks


def sync_yaml(example: Path, target: Path, dry_run: bool = False) -> bool:
    if not example.exists():
        return False
    if not target.exists():
        if not dry_run:
            shutil.copyfile(example, target)
        return True

    example_lines = read_lines(example)
    target_lines = ensure_trailing_newline(read_lines(target))
    example_blocks = yaml_blocks(example_lines)
    target_blocks = yaml_blocks(target_lines)
    target_keys = {block.key for block in target_blocks}
    changed = False

    for block in example_blocks:
        if block.key in target_keys:
            continue
        insert_at = len(target_lines)
        for previous in reversed(example_blocks[: example_blocks.index(block)]):
            for target_block in target_blocks:
                if target_block.key == previous.key:
                    insert_at = target_block.end
                    break
            if insert_at != len(target_lines):
                break

        addition = ["\n"] if insert_at > 0 and target_lines[insert_at - 1].strip() else []
        addition.extend(block.lines)
        if addition and not addition[-1].endswith("\n"):
            addition[-1] = f"{addition[-1]}\n"
        target_lines[insert_at:insert_at] = addition
        target_blocks = yaml_blocks(target_lines)
        target_keys.add(block.key)
        changed = True

    if changed and not dry_run:
        write_lines(target, target_lines)
    return changed


def env_key(line: str) -> str | None:
    match = ENV_KEY_RE.match(line.strip())
    if match:
        return match.group("key")
    commented = COMMENTED_ENV_KEY_RE.match(line.strip())
    if commented:
        return commented.group("key")
    return None


def collect_env_entries(lines: list[str]) -> list[Block]:
    entries: list[Block] = []
    pending_start = 0
    for index, line in enumerate(lines):
        key = env_key(line)
        if not key:
            if line.strip() == "":
                pending_start = index + 1
            continue
        entries.append(Block(key=key, start=pending_start, end=index + 1, lines=lines[pending_start : index + 1]))
        pending_start = index + 1
    return entries


def sync_env(example: Path, target: Path, dry_run: bool = False) -> bool:
    if not example.exists():
        return False
    if not target.exists():
        if not dry_run:
            shutil.copyfile(example, target)
        return True

    example_lines = read_lines(example)
    target_lines = ensure_trailing_newline(read_lines(target))
    target_keys = {key for line in target_lines if (key := env_key(line))}
    additions: list[str] = []

    for entry in collect_env_entries(example_lines):
        if entry.key in target_keys:
            continue
        if additions and additions[-1].strip():
            additions.append("\n")
        additions.extend(entry.lines)
        target_keys.add(entry.key)

    if not additions:
        return False
    if target_lines and target_lines[-1].strip():
        target_lines.append("\n")
    target_lines.append("\n# Added from example configuration. Existing values above were not changed.\n")
    target_lines.extend(additions)
    if not dry_run:
        write_lines(target, target_lines)
    return True


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--config", default="config.yaml", help="local YAML config path")
    parser.add_argument("--config-example", default="config.example.yaml", help="example YAML config path")
    parser.add_argument("--env", default=".env", help="local env path")
    parser.add_argument("--env-example", default=".env.example", help="example env path")
    parser.add_argument("--skip-yaml", action="store_true", help="skip YAML config sync")
    parser.add_argument("--skip-env", action="store_true", help="skip env sync")
    parser.add_argument("--dry-run", action="store_true", help="show whether files would change without writing")
    args = parser.parse_args()

    changed: list[str] = []
    if not args.skip_yaml and sync_yaml(Path(args.config_example), Path(args.config), args.dry_run):
        changed.append(args.config)
    if not args.skip_env and sync_env(Path(args.env_example), Path(args.env), args.dry_run):
        changed.append(args.env)

    if changed:
        prefix = "would update" if args.dry_run else "updated"
        print(f"{prefix}: " + ", ".join(changed))
    else:
        print("config files are already up to date")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
