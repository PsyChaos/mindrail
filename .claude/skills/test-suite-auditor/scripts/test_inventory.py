#!/usr/bin/env python3
"""Test-suite inventory helper for the test-suite-auditor skill.

Discovers test files and individual tests across common frameworks, then surfaces
mechanical leads:

  * counts per directory/framework
  * assertion-free tests (no assert/expect/should in the body)
  * verbatim copy-paste duplicates via normalized body hashing
  * skipped/disabled tests
  * very large snapshot files

Leads, not findings. Name similarity is deliberately NOT reported - two tests with
similar names routinely cover different branches, and verbatim body equality is the
only textual signal reliable enough to surface.

Usage:
    python test_inventory.py <repo> [--output inventory.json] [--min-dup-lines 3]
"""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import re
import sys
from collections import defaultdict
from pathlib import Path

EXCLUDES = {
    ".git", "node_modules", "vendor", "dist", "build", "target", "__pycache__",
    ".venv", "venv", ".tox", ".mypy_cache", ".pytest_cache", ".next", "coverage",
}

TEST_FILE_PATTERNS = [
    ("pytest/unittest", re.compile(r"(^test_.*\.py$|_test\.py$)")),
    ("jest/vitest", re.compile(r"\.(test|spec)\.(js|jsx|ts|tsx|mjs)$")),
    ("go", re.compile(r"_test\.go$")),
    ("junit", re.compile(r"(Test\w*|\w*Test|\w*Tests|\w*IT)\.(java|kt)$")),
    ("rspec", re.compile(r"_spec\.rb$")),
    ("phpunit", re.compile(r"\w+Test\.php$")),
]

TEST_DIR_HINTS = ("test", "tests", "spec", "specs", "__tests__", "e2e")

# framework -> (test definition regex with <name>, block style)
TEST_DEF = {
    "pytest/unittest": re.compile(r"^\s*(?:async\s+)?def\s+(?P<name>test_\w+)\s*\("),
    "jest/vitest": re.compile(
        r"^\s*(?:it|test)(?:\.(?:skip|only|todo|failing|concurrent))?"
        r"(?:\.each\([^)]*\))?\s*\(\s*[`'\"](?P<name>[^`'\"]+)[`'\"]"),
    "go": re.compile(r"^func\s+(?P<name>(?:Test|Benchmark|Fuzz)\w+)\s*\("),
    "junit": re.compile(r"^\s*@(?:Test|ParameterizedTest)\b"),
    "rspec": re.compile(
        r"^\s*(?:x?it|specify|pending)\s+[`'\"](?P<name>[^`'\"]+)[`'\"]"),
    "phpunit": re.compile(r"^\s*(?:public\s+)?function\s+(?P<name>test\w+)\s*\("),
}

ASSERTION_HINTS = re.compile(
    r"\b(assert\w*|expect|should|verify|require\.|toBe|toEqual|toHave|toThrow|"
    r"assertEquals|assertTrue|assertFalse|assertRaises|assertThat|"
    r"t\.(Error|Fatal|Fail)|Expect\()", )

SKIP_HINTS = re.compile(
    r"(@pytest\.mark\.skip|@unittest\.skip|\.skip\(|xit\(|xdescribe\(|"
    r"it\.skip|test\.skip|t\.Skip\(|@Disabled|@Ignore|pending\b)")

COMMENT_PREFIXES = ("#", "//", "*", "/*", "--")

SNAPSHOT_HINTS = ("__snapshots__", ".snap")
SNAPSHOT_SIZE_WARN = 50_000  # bytes


def detect_framework(name: str) -> str | None:
    for fw, rx in TEST_FILE_PATTERNS:
        if rx.search(name):
            return fw
    return None


def looks_like_test_path(rel: str) -> bool:
    parts = rel.lower().split(os.sep)
    return any(p in TEST_DIR_HINTS for p in parts)


def normalize_body(lines: list[str]) -> str:
    """Whitespace- and comment-insensitive body for duplicate hashing."""
    out = []
    for line in lines:
        s = line.strip()
        if not s or any(s.startswith(p) for p in COMMENT_PREFIXES):
            continue
        out.append(re.sub(r"\s+", " ", s))
    return "\n".join(out)


def extract_tests(path: Path, fw: str, text: str) -> list[dict]:
    """Split a test file into individual tests with rough body boundaries."""
    lines = text.splitlines()
    rx = TEST_DEF.get(fw)
    if rx is None:
        return []

    starts: list[tuple[int, str]] = []
    if fw == "junit":
        # @Test annotation: the test is the following method definition.
        method_rx = re.compile(r"^\s*(?:public\s+|void\s+|\w+\s+)*(\w+)\s*\(")
        for i, line in enumerate(lines):
            if rx.match(line):
                for j in range(i + 1, min(i + 4, len(lines))):
                    m = method_rx.match(lines[j])
                    if m:
                        starts.append((j, m.group(1)))
                        break
    else:
        for i, line in enumerate(lines):
            m = rx.match(line)
            if m:
                starts.append((i, m.group("name")))

    tests = []
    for idx, (start, name) in enumerate(starts):
        end = starts[idx + 1][0] if idx + 1 < len(starts) else len(lines)
        # Trim trailing decorator/annotation lines that belong to the NEXT test.
        while end - 1 > start and lines[end - 1].strip().startswith("@"):
            end -= 1
        body = lines[start:end]
        # Hash the body WITHOUT the definition line, so two tests that differ
        # only in name still hash equal - that is exactly the duplicate we want.
        norm = normalize_body(body[1:])
        # Skip markers live on the definition line itself or on decorator lines
        # directly above it - never in the body, which may belong to this test
        # but reference other names.
        decoration = "\n".join(lines[max(0, start - 3):start + 1])
        tests.append({
            "name": name,
            "line": start + 1,
            "body_lines": len([l for l in body[1:] if l.strip()]),
            "normalized": norm,
            "has_assertion": bool(ASSERTION_HINTS.search("\n".join(body))),
            "skipped": bool(SKIP_HINTS.search(decoration)),
        })
    return tests


def build(repo: Path, min_dup_lines: int) -> dict:
    files: list[dict] = []
    by_fw: dict[str, int] = defaultdict(int)
    by_dir: dict[str, int] = defaultdict(int)
    all_tests = 0
    no_assertion: list[dict] = []
    skipped: list[dict] = []
    snapshots: list[dict] = []
    body_index: dict[str, list[dict]] = defaultdict(list)

    for dirpath, dirnames, filenames in os.walk(repo):
        dirnames[:] = [d for d in dirnames if d not in EXCLUDES]
        for fn in sorted(filenames):
            path = Path(dirpath) / fn
            rel = str(path.relative_to(repo))

            if any(h in rel for h in SNAPSHOT_HINTS):
                try:
                    size = path.stat().st_size
                except OSError:
                    continue
                if size > SNAPSHOT_SIZE_WARN:
                    snapshots.append({"path": rel, "size_bytes": size})
                continue

            fw = detect_framework(fn)
            if fw is None:
                continue
            # Require either a matching filename in a test-ish dir, or a strongly
            # test-shaped filename anywhere.
            if not looks_like_test_path(rel) and fw in {"junit"}:
                continue
            try:
                text = path.read_text(encoding="utf-8", errors="replace")
            except OSError:
                continue

            tests = extract_tests(path, fw, text)
            files.append({"path": rel, "framework": fw, "tests": len(tests)})
            by_fw[fw] += len(tests)
            by_dir[str(Path(rel).parent)] += len(tests)
            all_tests += len(tests)

            for t in tests:
                ref = {"file": rel, "name": t["name"], "line": t["line"]}
                if not t["has_assertion"] and t["body_lines"] >= 2:
                    no_assertion.append(ref)
                if t["skipped"]:
                    skipped.append(ref)
                if t["body_lines"] >= min_dup_lines and t["normalized"]:
                    h = hashlib.sha256(t["normalized"].encode()).hexdigest()[:16]
                    body_index[h].append(ref)

    duplicates = [
        {"hash": h, "count": len(refs), "tests": refs}
        for h, refs in sorted(body_index.items(), key=lambda kv: -len(kv[1]))
        if len(refs) > 1
    ]

    return {
        "repo": str(repo),
        "test_files": len(files),
        "total_tests": all_tests,
        "by_framework": dict(sorted(by_fw.items(), key=lambda kv: -kv[1])),
        "by_directory": dict(sorted(by_dir.items(), key=lambda kv: -kv[1])[:40]),
        "files": sorted(files, key=lambda f: -f["tests"])[:200],
        "verbatim_duplicates": duplicates,
        "assertion_free_tests": no_assertion,
        "skipped_tests": skipped,
        "large_snapshots": sorted(snapshots, key=lambda s: -s["size_bytes"])[:40],
    }


def print_summary(inv: dict) -> None:
    out = sys.stdout.write
    out(f"\n=== Test inventory: {inv['repo']} ===\n")
    out(f"{inv['total_tests']} tests in {inv['test_files']} files\n")

    out("\n-- By framework --\n")
    for fw, n in inv["by_framework"].items():
        out(f"  {fw:<16} {n:>5}\n")

    out("\n-- Largest test directories --\n")
    for d, n in list(inv["by_directory"].items())[:10]:
        out(f"  {d:<50} {n:>5}\n")

    dups = inv["verbatim_duplicates"]
    if dups:
        out(f"\n-- Verbatim duplicate bodies ({len(dups)} group(s)) --\n")
        for g in dups[:12]:
            out(f"  x{g['count']}:\n")
            for t in g["tests"][:5]:
                out(f"      {t['file']}:{t['line']}  {t['name'][:55]}\n")
            if g["count"] > 5:
                out(f"      ... and {g['count'] - 5} more\n")
        if len(dups) > 12:
            out(f"  ... and {len(dups) - 12} more groups (see JSON)\n")

    if inv["assertion_free_tests"]:
        out(f"\n-- Tests with no detectable assertion "
            f"({len(inv['assertion_free_tests'])}) --\n")
        for t in inv["assertion_free_tests"][:15]:
            out(f"  {t['file']}:{t['line']}  {t['name'][:55]}\n")
        if len(inv["assertion_free_tests"]) > 15:
            out(f"  ... and {len(inv['assertion_free_tests']) - 15} more\n")

    if inv["skipped_tests"]:
        out(f"\n-- Skipped/disabled tests ({len(inv['skipped_tests'])}) --\n")
        for t in inv["skipped_tests"][:10]:
            out(f"  {t['file']}:{t['line']}  {t['name'][:55]}\n")

    if inv["large_snapshots"]:
        out(f"\n-- Large snapshot files ({len(inv['large_snapshots'])}) --\n")
        for s in inv["large_snapshots"][:10]:
            out(f"  {s['path']}  ({s['size_bytes']:,} bytes)\n")

    out("\nLeads, not findings. Assertion detection is heuristic (custom helper\n"
        "assertions won't match); verify before classifying anything INVALID.\n\n")


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__,
                                 formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("repo")
    ap.add_argument("--output", "-o")
    ap.add_argument("--min-dup-lines", type=int, default=2,
                    help="Minimum non-empty body lines for duplicate hashing (default 2)")
    ap.add_argument("--quiet", action="store_true")
    args = ap.parse_args()

    repo = Path(args.repo).resolve()
    if not repo.is_dir():
        sys.stderr.write(f"error: not a directory: {repo}\n")
        return 1

    inv = build(repo, args.min_dup_lines)
    if not args.quiet:
        print_summary(inv)
    if args.output:
        Path(args.output).write_text(json.dumps(inv, indent=2), encoding="utf-8")
        sys.stdout.write(f"Full inventory written to {args.output}\n")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
