#!/usr/bin/env python3
"""Mutation harness for the dual-agent-task-audit skill (Breaker move 1).

Neutralizes one guard at a time in the changed files, runs the test command, and
reports which guards SURVIVED - a guard whose removal turns nothing red is an
untested guard, and that is a finding.

Safety: the original repository is never modified. Everything happens in a scratch
copy under a temp directory, which is removed afterwards unless --keep is passed.

Usage:
    python mutate.py <repo> --base HEAD~1 --test-cmd "python -m pytest -q"
    python mutate.py <repo> --files src/auth.py,src/api.py --test-cmd "npm test"
    python mutate.py <repo> --base main --test-cmd "go test ./..." --dry-run
    python mutate.py <repo> --base HEAD~1 --test-cmd "pytest -q" -o /tmp/mutations.json

Exit codes: 0 = every guard killed, 4 = survivors found, 1 = usage/setup error.
"""

from __future__ import annotations

import argparse
import json
import os
import re
import shutil
import subprocess
import sys
import tempfile
import time
from pathlib import Path

# --- guard patterns ----------------------------------------------------------
# Each entry: (name, regex, mutation kind)
#   force_false  -> make the condition never fire
#   neutralize   -> replace the statement with a language-appropriate no-op

PY_GUARDS = [
    ("if-condition", re.compile(r"^(?P<indent>\s*)(?:el)?if\s+(?P<cond>.+?):\s*$"), "force_false"),
    ("raise", re.compile(r"^(?P<indent>\s*)raise\s+\S.*$"), "neutralize"),
    ("assert", re.compile(r"^(?P<indent>\s*)assert\s+\S.*$"), "neutralize"),
]

C_LIKE_GUARDS = [
    ("if-condition", None, "force_false"),  # handled by balanced-paren scan
    ("throw", re.compile(r"^(?P<indent>\s*)throw\s+\S.*;\s*$"), "neutralize"),
    ("panic", re.compile(r"^(?P<indent>\s*)panic\(.*\)\s*$"), "neutralize"),
]

C_IF_START = re.compile(r"^(?P<indent>\s*)(?:\}\s*else\s+)?if\s*\(")


def c_if_condition(line: str) -> tuple[str, str] | None:
    """Extract the full condition of a C-style `if (...)`, respecting nesting.

    A non-greedy regex mis-parses `if (foo(x) > 1)`, so scan for the matching
    parenthesis instead. Returns (indent, condition) or None.
    """
    m = C_IF_START.match(line)
    if not m:
        return None
    start = line.index("(", m.end() - 1) if line[m.end() - 1] != "(" else m.end() - 1
    depth = 0
    in_str: str | None = None
    for i in range(start, len(line)):
        ch = line[i]
        if in_str:
            if ch == in_str and line[i - 1] != "\\":
                in_str = None
            continue
        if ch in "\"'`":
            in_str = ch
        elif ch == "(":
            depth += 1
        elif ch == ")":
            depth -= 1
            if depth == 0:
                return m.group("indent"), line[start + 1:i]
    return None

LANG_BY_EXT = {
    ".py": ("python", PY_GUARDS),
    ".js": ("javascript", C_LIKE_GUARDS),
    ".jsx": ("javascript", C_LIKE_GUARDS),
    ".mjs": ("javascript", C_LIKE_GUARDS),
    ".ts": ("typescript", C_LIKE_GUARDS),
    ".tsx": ("typescript", C_LIKE_GUARDS),
    ".go": ("go", C_LIKE_GUARDS),
    ".java": ("java", C_LIKE_GUARDS),
    ".cs": ("csharp", C_LIKE_GUARDS),
    ".rs": ("rust", C_LIKE_GUARDS),
    ".php": ("php", C_LIKE_GUARDS),
}

NOOP = {
    "python": "pass",
    "javascript": ";",
    "typescript": ";",
    "go": "_ = 0",
    "java": ";",
    "csharp": ";",
    "rust": "()",
    "php": ";",
}

FALSE_LITERAL = {
    "python": "False",
    "javascript": "false",
    "typescript": "false",
    "go": "false",
    "java": "false",
    "csharp": "false",
    "rust": "false",
    "php": "false",
}

TEST_PATH_HINTS = ("test", "spec", "__tests__", "e2e")


def run(argv: list[str], cwd: Path | None = None, timeout: int = 120) -> tuple[int, str]:
    try:
        p = subprocess.run(argv, cwd=str(cwd) if cwd else None, capture_output=True,
                           text=True, timeout=timeout)
        return p.returncode, (p.stdout or "") + (("\n" + p.stderr) if p.stderr else "")
    except FileNotFoundError:
        return 127, f"command not found: {argv[0]}"
    except subprocess.TimeoutExpired:
        return 124, f"timed out after {timeout}s"


def is_test_path(path: str) -> bool:
    low = path.lower()
    base = os.path.basename(low)
    return (any(f"/{h}" in f"/{low}" for h in TEST_PATH_HINTS)
            or base.startswith("test_") or ".test." in base or ".spec." in base)


def changed_files(repo: Path, base: str, head: str) -> tuple[list[str], str | None]:
    rc, out = run(["git", "-C", str(repo), "rev-parse", "--verify", base])
    if rc != 0:
        return [], f"base ref '{base}' not usable: {out.strip()}"
    rc, out = run(["git", "-C", str(repo), "diff", "--name-only", "--diff-filter=d",
                   "-M", f"{base}..{head}"])
    if rc != 0:
        return [], f"git diff failed: {out.strip()}"
    return [p for p in out.splitlines() if p.strip()], None


def added_lines(repo: Path, base: str, head: str, path: str) -> set[int]:
    """Line numbers (in the head version) that the diff added."""
    rc, out = run(["git", "-C", str(repo), "diff", "-U0", f"{base}..{head}", "--", path])
    if rc != 0:
        return set()
    lines: set[int] = set()
    cur = 0
    for line in out.splitlines():
        m = re.match(r"^@@ -\d+(?:,\d+)? \+(\d+)(?:,(\d+))? @@", line)
        if m:
            cur = int(m.group(1))
            continue
        if line.startswith("+") and not line.startswith("+++"):
            lines.add(cur)
            cur += 1
        elif line.startswith("-") or line.startswith("\\"):
            continue
        elif not line.startswith("@@"):
            cur += 1
    return lines


def find_guards(text: str, lang: str, patterns: list, only_lines: set[int] | None) -> list[dict]:
    guards = []
    for i, line in enumerate(text.splitlines(), 1):
        if only_lines is not None and i not in only_lines:
            continue
        stripped = line.strip()
        if not stripped or stripped.startswith(("#", "//", "*", "/*")):
            continue
        for name, rx, kind in patterns:
            if rx is None:  # C-style if, needs balanced-paren scanning
                parsed = c_if_condition(line)
                if not parsed:
                    continue
                indent, cond = parsed
                if cond.strip().lower() in {"true", "false", "1", "0", ""}:
                    break
                guards.append({
                    "line": i, "kind": kind, "pattern": name,
                    "original": line.rstrip("\n"), "indent": indent,
                    "condition": cond.strip(),
                })
                break
            m = rx.match(line)
            if not m:
                continue
            # Skip conditions that are already constant - mutating them proves nothing.
            cond = (m.groupdict().get("cond") or "").strip()
            if cond.lower() in {"true", "false", "1", "0", ""} and kind == "force_false":
                break
            guards.append({
                "line": i, "kind": kind, "pattern": name,
                "original": line.rstrip("\n"),
                "indent": m.groupdict().get("indent", ""),
                "condition": cond or None,
            })
            break
    return guards


def mutate_line(line: str, guard: dict, lang: str) -> str | None:
    if guard["kind"] == "force_false":
        cond = guard["condition"]
        if not cond:
            return None
        # Replace only the first occurrence of the exact condition text.
        return line.replace(cond, FALSE_LITERAL[lang], 1)
    if guard["kind"] == "neutralize":
        return f"{guard['indent']}{NOOP[lang]}"
    return None


def setup_scratch(repo: Path, keep: bool) -> tuple[Path, Path]:
    tmp = Path(tempfile.mkdtemp(prefix="audit-mutate-"))
    dest = tmp / "work"
    ignore = shutil.ignore_patterns(
        ".git", "node_modules", "__pycache__", ".venv", "venv", ".tox",
        ".mypy_cache", ".pytest_cache", "dist", "build", "target", ".next",
    )
    shutil.copytree(repo, dest, ignore=ignore, symlinks=True)
    return tmp, dest


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__,
                                 formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("repo")
    ap.add_argument("--test-cmd", required=True,
                    help="Command that must go RED when a guard is removed")
    ap.add_argument("--base", help="Base ref; restricts mutation to lines the diff added")
    ap.add_argument("--head", default="HEAD")
    ap.add_argument("--files", help="Comma-separated files to mutate (instead of --base)")
    ap.add_argument("--max-mutations", type=int, default=40)
    ap.add_argument("--timeout", type=int, default=300, help="Per-test-run timeout")
    ap.add_argument("--dry-run", action="store_true", help="List guards, run nothing")
    ap.add_argument("--keep", action="store_true", help="Keep the scratch directory")
    ap.add_argument("--output", "-o")
    args = ap.parse_args()

    repo = Path(args.repo).resolve()
    if not repo.is_dir():
        sys.stderr.write(f"error: not a directory: {repo}\n")
        return 1

    # --- select files -------------------------------------------------------
    line_filter: dict[str, set[int]] = {}
    if args.files:
        targets = [f.strip() for f in args.files.split(",") if f.strip()]
    elif args.base:
        targets, err = changed_files(repo, args.base, args.head)
        if err:
            sys.stderr.write(f"error: {err}\n")
            return 1
        for t in targets:
            line_filter[t] = added_lines(repo, args.base, args.head, t)
    else:
        sys.stderr.write("error: pass --base or --files\n")
        return 1

    targets = [t for t in targets
               if Path(t).suffix.lower() in LANG_BY_EXT and not is_test_path(t)]
    if not targets:
        sys.stdout.write("\nNo mutable source files in scope "
                         "(test files are excluded on purpose).\n\n")
        return 0

    # --- collect guards -----------------------------------------------------
    plan: list[dict] = []
    for rel in targets:
        src = repo / rel
        if not src.is_file():
            continue
        lang, patterns = LANG_BY_EXT[Path(rel).suffix.lower()]
        try:
            text = src.read_text(encoding="utf-8", errors="replace")
        except OSError:
            continue
        only = line_filter.get(rel) if args.base else None
        for g in find_guards(text, lang, patterns, only):
            g.update(file=rel, lang=lang)
            plan.append(g)

    if not plan:
        sys.stdout.write("\nNo guard-shaped lines found in scope. Either the change "
                         "added no protective conditions, or they take a form this "
                         "harness doesn't match - inspect by hand before concluding "
                         "anything.\n\n")
        return 0

    truncated = len(plan) > args.max_mutations
    plan = plan[:args.max_mutations]

    sys.stdout.write(f"\n=== Mutation plan: {len(plan)} guard(s) ===\n")
    for g in plan:
        sys.stdout.write(f"  {g['file']}:{g['line']:<5} {g['pattern']:<13} "
                         f"{g['original'].strip()[:70]}\n")
    if truncated:
        sys.stdout.write(f"  (capped at --max-mutations={args.max_mutations})\n")

    if args.dry_run:
        sys.stdout.write("\nDry run - nothing executed.\n\n")
        return 0

    # --- baseline -----------------------------------------------------------
    tmp, work = setup_scratch(repo, args.keep)
    test_argv = args.test_cmd.split()
    try:
        sys.stdout.write(f"\nBaseline: {args.test_cmd}\n")
        rc, out = run(test_argv, cwd=work, timeout=args.timeout)
        if rc != 0:
            sys.stdout.write(
                f"  baseline is RED (exit {rc}). Mutation results would be meaningless: "
                "a suite that is already failing cannot demonstrate that removing a "
                "guard broke something.\n"
                f"  --- tail ---\n{chr(10).join(out.splitlines()[-15:])}\n\n")
            return 1
        sys.stdout.write("  baseline GREEN\n\n")

        results = []
        for idx, g in enumerate(plan, 1):
            target = work / g["file"]
            original_text = target.read_text(encoding="utf-8", errors="replace")
            lines = original_text.splitlines(keepends=True)
            raw = lines[g["line"] - 1]
            mutated = mutate_line(raw.rstrip("\n"), g, g["lang"])
            if mutated is None or mutated.strip() == raw.strip():
                results.append({**g, "status": "not_mutable",
                                "note": "could not construct a distinct mutation"})
                continue

            lines[g["line"] - 1] = mutated + ("\n" if raw.endswith("\n") else "")
            target.write_text("".join(lines), encoding="utf-8")
            start = time.time()
            rc, out = run(test_argv, cwd=work, timeout=args.timeout)
            target.write_text(original_text, encoding="utf-8")

            status = "killed" if rc != 0 else "survived"
            results.append({
                **g, "status": status, "exit_code": rc,
                "mutation": mutated.strip(),
                "duration_seconds": round(time.time() - start, 2),
                "output_tail": "\n".join(out.splitlines()[-10:]) if status == "survived" else "",
            })
            marker = "SURVIVED <-- finding" if status == "survived" else "killed"
            sys.stdout.write(f"  [{idx}/{len(plan)}] {g['file']}:{g['line']} "
                             f"{g['pattern']:<13} {marker}\n")

        survivors = [r for r in results if r["status"] == "survived"]
        report = {
            "repo": str(repo), "test_cmd": args.test_cmd,
            "base": args.base, "files": targets,
            "total": len(results),
            "killed": sum(1 for r in results if r["status"] == "killed"),
            "survived": len(survivors),
            "not_mutable": sum(1 for r in results if r["status"] == "not_mutable"),
            "truncated": truncated,
            "results": results,
        }

        sys.stdout.write(f"\n--- {report['killed']} killed / {report['survived']} survived"
                         f" / {report['not_mutable']} not mutable ---\n")
        if survivors:
            sys.stdout.write("\nSURVIVING GUARDS (untested conditions - each is a finding):\n")
            for r in survivors:
                sys.stdout.write(f"  {r['file']}:{r['line']}  {r['original'].strip()[:70]}\n")
                sys.stdout.write(f"      neutralized to: {r['mutation'][:70]}\n")
        sys.stdout.write(
            "\nVerify a sample by hand. A mutation that didn't actually disable the "
            "guard reports a false 'killed' and hides the gap.\n\n")

        if args.output:
            Path(args.output).write_text(json.dumps(report, indent=2), encoding="utf-8")
            sys.stdout.write(f"Report written to {args.output}\n")
        return 4 if survivors else 0
    finally:
        if args.keep:
            sys.stdout.write(f"Scratch kept at {tmp}\n")
        else:
            shutil.rmtree(tmp, ignore_errors=True)


if __name__ == "__main__":
    raise SystemExit(main())
