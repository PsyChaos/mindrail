#!/usr/bin/env python3
"""Impacted-test selector for the engineering-orchestrator skill.

Given a changeset, finds the tests that could be affected by it and prints a
command that runs only those. Meant for the inner loop - Phase 14 task acceptance,
the Breaker's mutation move, remediation rounds - where the question is "does this
change keep its own promise?" and the full suite answers it no better, only later.

It is NOT a substitute for the single full-suite run at the completion gate. Every
selection strategy below has a blind spot (dynamic imports, string registries,
config-driven behavior, shared fixtures), and the full suite exists to catch exactly
what the selector cannot see. The script therefore always reports *which* strategy
it used and how much to trust it, and refuses to narrow e2e tests on anything but an
explicit map.

Strategies, tried in order of reliability, per changed file:
    1. explicit map   - the repo's own file: .impact-map.json / tests/impact-map.json
                        (or .yaml if PyYAML is installed). Only source that may
                        select e2e tests.
    2. coverage       - coverage.py database with per-test contexts
                        (run once with: pytest --cov --cov-context=test). Measured
                        data, not inference.
    3. import graph   - reverse dependency graph (Python via ast, JS/TS via regex),
                        transitive to --depth (default 2).
    4. naming         - foo.py <-> test_foo.py, foo.ts <-> foo.spec.ts, etc.
    5. none           - nothing selectable: run the full suite. Never narrow silently.

Usage:
    python impacted_tests.py <repo> --base <ref>
    python impacted_tests.py <repo> --changed src/a.py src/b.py
    python impacted_tests.py <repo> --base <ref> --depth 3 --coverage .coverage -o impact.json

Explicit map format (globs -> test paths or test commands):
    {"src/auth/**": ["tests/auth", "e2e/login.spec.ts"],
     "src/export/*.py": ["tests/test_export.py"]}

Read-only.
"""

from __future__ import annotations

import argparse
import ast
import fnmatch
import json
import os
import re
import sqlite3
import subprocess
import sys
from collections import defaultdict, deque
from pathlib import Path, PurePosixPath

TEST_RE = re.compile(r"(^|/)(tests?|__tests__|spec|specs)/|(^|/)test_[^/]+\.py$|_test\.py$|[._-](test|spec)\.[jt]sx?$|_test\.go$")
E2E_RE = re.compile(r"(^|/)(e2e|end2end|end-to-end|integration|acceptance|cypress|playwright|smoke)(/|[._-])", re.I)
SKIP_DIRS = {".git", "node_modules", "__pycache__", ".venv", "venv", "dist", "build", ".tox", ".mypy_cache", "coverage", ".pytest_cache"}
PY = (".py",)
JS = (".js", ".jsx", ".ts", ".tsx", ".mjs", ".cjs")
JS_IMPORT_RE = re.compile(r"""(?:import|export)\s+(?:[^'"]*?\s+from\s+)?['"]([^'"]+)['"]|require\(\s*['"]([^'"]+)['"]\s*\)""")


def git(repo: Path, *args: str) -> str:
    try:
        return subprocess.run(["git", *args], cwd=repo, check=True, capture_output=True, text=True).stdout
    except (subprocess.CalledProcessError, FileNotFoundError) as exc:
        sys.exit(f"git {' '.join(args)} failed: {getattr(exc, 'stderr', exc)}")


def changed_files(repo: Path, base: str) -> list[str]:
    files = set()
    for line in git(repo, "diff", "--name-only", "-M", base).splitlines():
        if line.strip():
            files.add(line.strip())
    for line in git(repo, "status", "--porcelain", "--untracked-files=all").splitlines():
        if line.startswith("??"):
            files.add(line[3:].strip())
    return sorted(files)


def walk_sources(repo: Path) -> list[str]:
    out = []
    for root, dirs, files in os.walk(repo):
        dirs[:] = [d for d in dirs if d not in SKIP_DIRS and not d.startswith(".")]
        for f in files:
            if f.endswith(PY + JS):
                out.append(str(Path(root, f).relative_to(repo).as_posix()))
    return out


def is_test(path: str) -> bool:
    return bool(TEST_RE.search(path))


def is_e2e(path: str) -> bool:
    return bool(E2E_RE.search(path))


# ----------------------------------------------------------------------------- #
# 1. explicit map
# ----------------------------------------------------------------------------- #

def load_explicit_map(repo: Path) -> dict[str, list[str]] | None:
    candidates = [".impact-map.json", "tests/impact-map.json", ".impact-map.yaml", ".impact-map.yml",
                  "tests/impact-map.yaml", "tests/impact-map.yml"]
    for name in candidates:
        p = repo / name
        if not p.exists():
            continue
        text = p.read_text(encoding="utf-8", errors="replace")
        if name.endswith(".json"):
            try:
                return json.loads(text)
            except ValueError as exc:
                sys.exit(f"{name}: invalid JSON: {exc}")
        try:
            import yaml  # type: ignore
        except ImportError:
            print(f"warning: {name} found but PyYAML is not installed; ignoring it", file=sys.stderr)
            continue
        return yaml.safe_load(text) or {}
    return None


def select_explicit(changed: str, impact_map: dict[str, list[str]]) -> list[str]:
    hits = []
    for pattern, targets in impact_map.items():
        if fnmatch.fnmatch(changed, pattern) or PurePosixPath(changed).match(pattern):
            hits.extend(targets if isinstance(targets, list) else [targets])
    return hits


# ----------------------------------------------------------------------------- #
# 2. coverage.py contexts
# ----------------------------------------------------------------------------- #

def load_coverage_contexts(repo: Path, db: str | None) -> dict[str, set[str]] | None:
    """file (repo-relative) -> set of test node ids that executed lines in it."""
    path = repo / (db or ".coverage")
    if not path.exists():
        return None
    try:
        con = sqlite3.connect(f"file:{path}?mode=ro", uri=True)
        cur = con.cursor()
        cur.execute("SELECT id, context FROM context")
        contexts = {cid: ctx for cid, ctx in cur.fetchall()}
        cur.execute("SELECT id, path FROM file")
        files = {fid: p for fid, p in cur.fetchall()}
        cur.execute("SELECT file_id, context_id FROM line_bits")
        rows = cur.fetchall()
    except sqlite3.Error:
        return None
    if not any(ctx for ctx in contexts.values() if ctx):
        return {}  # database exists but was recorded without --cov-context
    result: dict[str, set[str]] = defaultdict(set)
    for fid, cid in rows:
        ctx = contexts.get(cid) or ""
        if not ctx:
            continue
        node = ctx.split("|")[0]  # "tests/test_x.py::test_y|run" -> node id
        try:
            rel = Path(files[fid]).resolve().relative_to(repo.resolve()).as_posix()
        except ValueError:
            rel = files[fid]
        result[rel].add(node)
    return result


# ----------------------------------------------------------------------------- #
# 3. import graph
# ----------------------------------------------------------------------------- #

def python_module_index(sources: list[str]) -> dict[str, str]:
    """dotted module name -> path, for every plausible package root."""
    index: dict[str, str] = {}
    for path in sources:
        if not path.endswith(".py"):
            continue
        parts = list(PurePosixPath(path).with_suffix("").parts)
        if parts[-1] == "__init__":
            parts = parts[:-1]
        # register every suffix so "src/pkg/mod.py" answers to pkg.mod and src.pkg.mod
        for i in range(len(parts)):
            index.setdefault(".".join(parts[i:]), path)
    return index


def python_imports(repo: Path, path: str, index: dict[str, str]) -> set[str]:
    try:
        tree = ast.parse((repo / path).read_text(encoding="utf-8", errors="replace"))
    except (SyntaxError, OSError):
        return set()
    deps = set()
    pkg_parts = list(PurePosixPath(path).parent.parts)
    for node in ast.walk(tree):
        names: list[str] = []
        if isinstance(node, ast.Import):
            names = [a.name for a in node.names]
        elif isinstance(node, ast.ImportFrom):
            base = node.module or ""
            if node.level:
                anchor = pkg_parts[: len(pkg_parts) - (node.level - 1)] if node.level - 1 <= len(pkg_parts) else []
                base = ".".join([*anchor, base] if base else anchor)
            names = [base] + [f"{base}.{a.name}" for a in node.names]
        for name in names:
            while name:
                if name in index:
                    deps.add(index[name])
                    break
                name = name.rpartition(".")[0]
    return deps


def js_imports(repo: Path, path: str, sources: set[str]) -> set[str]:
    try:
        text = (repo / path).read_text(encoding="utf-8", errors="replace")
    except OSError:
        return set()
    deps = set()
    here = PurePosixPath(path).parent
    for m in JS_IMPORT_RE.finditer(text):
        spec = m.group(1) or m.group(2)
        if not spec or not spec.startswith("."):
            continue
        target = PurePosixPath(os.path.normpath(str(here / spec))).as_posix()
        for cand in [target, *[f"{target}{ext}" for ext in JS], *[f"{target}/index{ext}" for ext in JS]]:
            if cand in sources:
                deps.add(cand)
                break
    return deps


def reverse_graph(repo: Path, sources: list[str]) -> dict[str, set[str]]:
    index = python_module_index(sources)
    src_set = set(sources)
    rev: dict[str, set[str]] = defaultdict(set)
    for path in sources:
        deps = python_imports(repo, path, index) if path.endswith(".py") else js_imports(repo, path, src_set)
        for dep in deps:
            rev[dep].add(path)
    return rev


def select_by_graph(changed: str, rev: dict[str, set[str]], depth: int) -> tuple[list[str], int]:
    """BFS over importers; returns (test files reached, max depth actually used)."""
    seen = {changed}
    frontier = deque([(changed, 0)])
    tests, used = set(), 0
    while frontier:
        node, d = frontier.popleft()
        if d >= depth:
            continue
        for importer in rev.get(node, ()):
            if importer in seen:
                continue
            seen.add(importer)
            if is_test(importer):
                tests.add(importer)
                used = max(used, d + 1)
            else:
                frontier.append((importer, d + 1))
    return sorted(tests), used


# ----------------------------------------------------------------------------- #
# 4. naming convention
# ----------------------------------------------------------------------------- #

def select_by_name(changed: str, sources: list[str]) -> list[str]:
    stem = PurePosixPath(changed).stem
    if stem == "__init__":
        stem = PurePosixPath(changed).parent.name
    wanted = {f"test_{stem}.py", f"{stem}_test.py", f"tests_{stem}.py",
              *[f"{stem}{mid}{ext}" for mid in (".test", ".spec", "_test", "-test") for ext in JS],
              f"{stem}_test.go"}
    return sorted(p for p in sources if PurePosixPath(p).name in wanted and is_test(p))


# ----------------------------------------------------------------------------- #
# main
# ----------------------------------------------------------------------------- #

def suggest_command(repo: Path, tests: list[str]) -> str | None:
    if not tests:
        return None
    # a whole file already selected makes its individual node ids redundant
    whole = {t for t in tests if "::" not in t}
    tests = [t for t in tests if "::" not in t or t.split("::")[0] not in whole]
    dirs = {t for t in whole if (repo / t).is_dir()}
    tests = [t for t in tests if not any(t != d and t.startswith(d.rstrip("/") + "/") for d in dirs)]
    def looks_python(t: str) -> bool:
        if t.endswith(".py") or "::" in t:
            return True
        if t.endswith(JS):
            return False
        d = repo / t
        if d.is_dir():
            has_py = any(d.rglob("*.py"))
            has_js = any(True for e in JS for _ in d.rglob(f"*{e}"))
            return has_py and not has_js
        return True  # bare path with no extension: assume pytest node
    py = [t for t in tests if looks_python(t)]
    js = [t for t in tests if not looks_python(t)]
    parts = []
    if py:
        parts.append("pytest -q " + " ".join(sorted(py)))
    if js:
        runner = "npx jest"
        pkg = repo / "package.json"
        if pkg.exists():
            try:
                text = pkg.read_text(encoding="utf-8")
                if "vitest" in text:
                    runner = "npx vitest run"
            except OSError:
                pass
        parts.append(f"{runner} " + " ".join(sorted(js)))
    return " && ".join(parts)


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("repo")
    ap.add_argument("--base", help="Ref to diff against")
    ap.add_argument("--changed", nargs="*", help="Explicit list of changed files (instead of --base)")
    ap.add_argument("--depth", type=int, default=2, help="Transitive importer depth for the graph strategy (default 2)")
    ap.add_argument("--coverage", help="Path to a coverage.py database with per-test contexts (default .coverage)")
    ap.add_argument("--output", "-o")
    ap.add_argument("--quiet", action="store_true")
    args = ap.parse_args()

    repo = Path(args.repo).resolve()
    if args.changed:
        changed = sorted(args.changed)
    elif args.base:
        changed = changed_files(repo, args.base)
    else:
        sys.exit("give --base <ref> or --changed <files...>")

    sources = walk_sources(repo)
    impact_map = load_explicit_map(repo)
    cov = load_coverage_contexts(repo, args.coverage)
    rev = reverse_graph(repo, sources)

    per_file = []
    selected: set[str] = set()
    e2e_selected: set[str] = set()
    unscoped: list[str] = []
    strategies_used: set[str] = set()
    changed_tests = [c for c in changed if is_test(c)]

    for c in changed:
        if is_test(c):
            selected.add(c)
            per_file.append({"file": c, "strategy": "self (changed test)", "confidence": "high", "tests": [c]})
            continue
        if not c.endswith(PY + JS):
            per_file.append({"file": c, "strategy": "none (not source)", "confidence": "n/a", "tests": []})
            continue
        entry = None
        if impact_map:
            hits = select_explicit(c, impact_map)
            if hits:
                entry = {"strategy": "explicit map", "confidence": "high", "tests": hits}
                for h in hits:
                    (e2e_selected if is_e2e(h) else selected).add(h)
        if entry is None and cov:
            nodes = sorted(n for n in cov.get(c, set()) if not is_e2e(n))
            if nodes:
                entry = {"strategy": "coverage contexts", "confidence": "high", "tests": nodes}
                selected.update(nodes)
        if entry is None:
            tests, used = select_by_graph(c, rev, args.depth)
            tests = [t for t in tests if not is_e2e(t)]
            if tests:
                entry = {"strategy": f"import graph (depth {used})", "confidence": "medium" if used <= 1 else "low",
                         "tests": tests}
                selected.update(tests)
        if entry is None:
            tests = [t for t in select_by_name(c, sources) if not is_e2e(t)]
            if tests:
                entry = {"strategy": "naming convention", "confidence": "low", "tests": tests}
                selected.update(tests)
        else:
            # naming hits are cheap and direct, so they always ride along with a
            # stronger strategy - a coverage db or graph can be stale for a new test
            extra = [t for t in select_by_name(c, sources) if not is_e2e(t) and t not in entry["tests"]]
            if extra:
                entry["tests"] = entry["tests"] + extra
                selected.update(extra)
        if entry is None:
            entry = {"strategy": "none", "confidence": "n/a", "tests": []}
            unscoped.append(c)
        strategies_used.add(entry["strategy"].split(" (")[0])
        per_file.append({"file": c, **entry})

    e2e_exists = any(is_e2e(p) for p in sources)
    narrowed_ok = not unscoped
    report = {
        "changed": changed,
        "per_file": per_file,
        "selected_tests": sorted(selected),
        "selected_e2e": sorted(e2e_selected),
        "unscoped_files": unscoped,
        "e2e_status": ("selected via explicit map" if e2e_selected
                       else ("present but not selectable - run full e2e or list as unverified" if e2e_exists
                             else "no e2e tests detected")),
        "strategies_used": sorted(strategies_used),
        "overall_confidence": ("high" if strategies_used <= {"explicit map", "coverage contexts", "self"}
                               else "medium" if "import graph" in strategies_used and narrowed_ok
                               else "low"),
        "narrowing_allowed": narrowed_ok,
        "command": suggest_command(repo, sorted(selected)) if narrowed_ok else None,
        "e2e_command": suggest_command(repo, sorted(e2e_selected)) if e2e_selected else None,
        "note": (None if narrowed_ok else
                 "At least one changed source file maps to no test by any strategy. Run the full "
                 "unit suite for this round; do not narrow. Each unscoped file is also a Reader "
                 "finding: changed code with no test reachable from it."),
    }
    if args.output:
        Path(args.output).write_text(json.dumps(report, indent=2), encoding="utf-8")
    if not args.quiet:
        print(f"# Impacted tests (confidence: {report['overall_confidence']})\n")
        print("| Changed file | Strategy | Confidence | Tests |")
        print("| --- | --- | --- | --- |")
        for e in per_file:
            print(f"| {e['file']} | {e['strategy']} | {e['confidence']} | {', '.join(e['tests']) or '-'} |")
        print()
        print(f"Unit/integration: {report['command'] or '(run full suite - see note)'}")
        print(f"E2E: {report['e2e_status']}" + (f" -> {report['e2e_command']}" if report['e2e_command'] else ""))
        if report["note"]:
            print(f"\nNote: {report['note']}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
