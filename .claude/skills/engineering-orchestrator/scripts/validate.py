#!/usr/bin/env python3
"""Validation evidence helper for the engineering-orchestrator skill.

Detects the project's real validation commands, optionally runs them, and records
exact results to JSON so "tests pass" can be replaced with reproducible evidence.

Also compares two runs, which is how a pre-existing failure is told apart from one
the task introduced.

Usage:
    python validate.py <repo>                                   # detect only (dry run)
    python validate.py <repo> --run --label baseline -o baseline.json
    python validate.py <repo> --run --label post-change -o post.json
    python validate.py --compare baseline.json post.json
    python validate.py <repo> --run --only test,lint --timeout 900

Notes:
  * Dry run is the default on purpose: you should see what will execute before it does.
  * Nothing destructive is ever proposed - no installs, migrations, deploys or
    formatters that rewrite files. Detection is read-only.
"""

from __future__ import annotations

import argparse
import json
import os
import shutil
import subprocess
import sys
import time
from pathlib import Path

# kind -> (command, why it matters)
Command = dict


def _cmd(kind: str, argv: list[str], source: str) -> Command:
    return {"kind": kind, "argv": argv, "source": source}


def read_json(path: Path) -> dict | None:
    try:
        return json.loads(path.read_text(encoding="utf-8", errors="replace"))
    except (OSError, ValueError):
        return None


def read_text(path: Path) -> str:
    try:
        return path.read_text(encoding="utf-8", errors="replace")
    except OSError:
        return ""


def detect_node(repo: Path) -> list[Command]:
    pkg_path = repo / "package.json"
    pkg = read_json(pkg_path)
    if not pkg:
        return []
    scripts = pkg.get("scripts") or {}

    runner = "npm"
    if (repo / "pnpm-lock.yaml").exists():
        runner = "pnpm"
    elif (repo / "yarn.lock").exists():
        runner = "yarn"

    def invoke(script: str) -> list[str]:
        if runner == "npm":
            return ["npm", "run", script, "--if-present"]
        return [runner, "run", script]

    mapping = [
        ("build", ["build", "compile"]),
        ("typecheck", ["typecheck", "type-check", "tsc", "types"]),
        ("lint", ["lint", "eslint"]),
        ("test", ["test", "test:unit", "jest", "vitest"]),
        ("test_integration", ["test:integration", "test:int"]),
        ("test_e2e", ["test:e2e", "e2e"]),
    ]
    found: list[Command] = []
    for kind, candidates in mapping:
        for c in candidates:
            if c in scripts:
                found.append(_cmd(kind, invoke(c), f"package.json scripts.{c}"))
                break
    if not any(f["kind"] == "typecheck" for f in found) and (repo / "tsconfig.json").exists():
        found.append(_cmd("typecheck", ["npx", "tsc", "--noEmit"], "tsconfig.json"))
    return found


def detect_python(repo: Path) -> list[Command]:
    found: list[Command] = []
    py = "python3" if shutil.which("python3") else "python"
    pyproject = read_text(repo / "pyproject.toml")
    has_py_manifest = bool(pyproject) or (repo / "requirements.txt").exists() \
        or (repo / "setup.py").exists() or (repo / "setup.cfg").exists()
    has_tests = any((repo / d).is_dir() for d in ("tests", "test")) or \
        any(repo.glob("test_*.py")) or any(repo.glob("**/test_*.py"))

    if not (has_py_manifest or has_tests):
        return found

    if has_tests or "pytest" in pyproject:
        if _is_available([py, "-m", "pytest"]):
            found.append(_cmd("test", [py, "-m", "pytest", "-q"], "pytest layout"))
        else:
            found.append(_cmd("test", [py, "-m", "unittest", "discover", "-v"],
                              "tests present, pytest unavailable"))
    if "ruff" in pyproject or (repo / "ruff.toml").exists() or (repo / ".ruff.toml").exists():
        found.append(_cmd("lint", [py, "-m", "ruff", "check", "."], "ruff config"))
    elif (repo / ".flake8").exists() or "flake8" in read_text(repo / "setup.cfg"):
        found.append(_cmd("lint", [py, "-m", "flake8"], "flake8 config"))
    if "mypy" in pyproject or (repo / "mypy.ini").exists() or (repo / ".mypy.ini").exists():
        found.append(_cmd("typecheck", [py, "-m", "mypy", "."], "mypy config"))
    return found


def detect_go(repo: Path) -> list[Command]:
    if not (repo / "go.mod").exists():
        return []
    return [
        _cmd("build", ["go", "build", "./..."], "go.mod"),
        _cmd("vet", ["go", "vet", "./..."], "go.mod"),
        _cmd("test", ["go", "test", "./..."], "go.mod"),
    ]


def detect_rust(repo: Path) -> list[Command]:
    if not (repo / "Cargo.toml").exists():
        return []
    return [
        _cmd("build", ["cargo", "build"], "Cargo.toml"),
        _cmd("lint", ["cargo", "clippy", "--", "-D", "warnings"], "Cargo.toml"),
        _cmd("test", ["cargo", "test"], "Cargo.toml"),
    ]


def detect_jvm(repo: Path) -> list[Command]:
    if (repo / "pom.xml").exists():
        return [_cmd("build_test", ["mvn", "-B", "test"], "pom.xml")]
    if (repo / "gradlew").exists():
        return [_cmd("build_test", ["./gradlew", "test"], "gradlew")]
    if (repo / "build.gradle").exists() or (repo / "build.gradle.kts").exists():
        return [_cmd("build_test", ["gradle", "test"], "build.gradle")]
    return []


def detect_ruby(repo: Path) -> list[Command]:
    if not (repo / "Gemfile").exists():
        return []
    found = [_cmd("test", ["bundle", "exec", "rspec"], "Gemfile")] \
        if (repo / "spec").is_dir() else []
    if (repo / ".rubocop.yml").exists():
        found.append(_cmd("lint", ["bundle", "exec", "rubocop"], ".rubocop.yml"))
    return found


DETECTORS = (detect_node, detect_python, detect_go, detect_rust, detect_jvm, detect_ruby)


def detect(repo: Path) -> list[Command]:
    commands: list[Command] = []
    for d in DETECTORS:
        try:
            commands.extend(d(repo))
        except Exception as exc:  # a detector must never abort the whole run
            commands.append(_cmd("detector_error", [], f"{d.__name__}: {exc}"))
    for c in commands:
        c["available"] = _is_available(c["argv"])
    return commands


def _is_available(argv: list[str]) -> bool:
    """A `python -m pytest` command is only real if the module is importable."""
    if not argv:
        return False
    if shutil.which(argv[0]) is None:
        return False
    if len(argv) >= 3 and argv[1] == "-m":
        probe = subprocess.run(
            [argv[0], "-c", f"import importlib.util,sys;"
                            f"sys.exit(0 if importlib.util.find_spec('{argv[2]}') else 1)"],
            capture_output=True, text=True, timeout=30,
        )
        return probe.returncode == 0
    return True


def run_command(repo: Path, cmd: Command, timeout: int) -> Command:
    result = dict(cmd)
    if not cmd.get("available"):
        exe = cmd["argv"][0] if cmd["argv"] else "?"
        if len(cmd["argv"]) >= 3 and cmd["argv"][1] == "-m":
            reason = f"module '{cmd['argv'][2]}' not importable by {exe}"
        else:
            reason = f"{exe} not on PATH"
        result.update(status="skipped", reason=reason)
        return result

    start = time.time()
    try:
        p = subprocess.run(cmd["argv"], cwd=str(repo), capture_output=True,
                           text=True, timeout=timeout)
        out = (p.stdout or "") + (("\n" + p.stderr) if p.stderr else "")
        lines = out.splitlines()
        result.update(
            status="passed" if p.returncode == 0 else "failed",
            exit_code=p.returncode,
            duration_seconds=round(time.time() - start, 2),
            output_lines=len(lines),
            output_tail="\n".join(lines[-60:]),
        )
    except subprocess.TimeoutExpired:
        result.update(status="timeout", duration_seconds=timeout,
                      reason=f"exceeded {timeout}s")
    except Exception as exc:
        result.update(status="error", reason=str(exc))
    return result


def summarize(results: list[Command]) -> dict:
    counts: dict[str, int] = {}
    for r in results:
        counts[r.get("status", "detected")] = counts.get(r.get("status", "detected"), 0) + 1
    return counts


def print_run(report: dict) -> None:
    out = sys.stdout.write
    out(f"\n=== Validation [{report['label']}] {report['repo']} ===\n")
    if not report["results"]:
        out("No validation commands detected. Determine them from the repository's CI\n"
            "config or ask the user, and record whatever you run as evidence.\n\n")
        return
    for r in report["results"]:
        argv = " ".join(r["argv"]) if r["argv"] else "(none)"
        status = r.get("status", "detected")
        line = f"  [{status:<8}] {r['kind']:<16} {argv}"
        if "duration_seconds" in r:
            line += f"  ({r['duration_seconds']}s)"
        if r.get("reason"):
            line += f"  -- {r['reason']}"
        out(line + "\n")
        if status == "failed" and r.get("output_tail"):
            for tail_line in r["output_tail"].splitlines()[-12:]:
                out(f"        | {tail_line}\n")
    out(f"\n  summary: {report['summary']}\n")
    if report["mode"] == "detect":
        out("  (dry run - nothing was executed; re-run with --run)\n")
    out("\n")


def compare(before_path: Path, after_path: Path) -> int:
    before, after = read_json(before_path), read_json(after_path)
    if not before or not after:
        sys.stderr.write("error: could not read one of the reports\n")
        return 1

    idx = {(r["kind"], " ".join(r["argv"])): r for r in before.get("results", [])}
    out = sys.stdout.write
    out(f"\n=== Comparison: {before.get('label')} -> {after.get('label')} ===\n")

    regressions, fixes, unchanged, new = [], [], [], []
    for r in after.get("results", []):
        key = (r["kind"], " ".join(r["argv"]))
        prev = idx.get(key)
        now = r.get("status")
        if prev is None:
            new.append((key[0], now))
        elif prev.get("status") == now:
            unchanged.append((key[0], now))
        elif now == "failed":
            regressions.append((key[0], prev.get("status"), now))
        elif prev.get("status") == "failed":
            fixes.append((key[0], now))
        else:
            unchanged.append((key[0], f"{prev.get('status')}->{now}"))

    if regressions:
        out("\n-- INTRODUCED FAILURES (attributable to the change) --\n")
        for kind, was, now in regressions:
            out(f"  {kind:<18} {was} -> {now}\n")
    pre_existing = [k for k, s in unchanged if s == "failed"]
    if pre_existing:
        out("\n-- PRE-EXISTING FAILURES (already failing before the change) --\n")
        for kind in pre_existing:
            out(f"  {kind}\n")
    if fixes:
        out("\n-- NEWLY PASSING --\n")
        for kind, now in fixes:
            out(f"  {kind:<18} -> {now}\n")
    if new:
        out("\n-- NOT PRESENT IN BASELINE --\n")
        for kind, now in new:
            out(f"  {kind:<18} {now}\n")

    if not regressions:
        out("\nNo failures attributable to the change.\n")
    out("\nA clean comparison is evidence of no regression in what was measured - "
        "not proof of correctness.\n\n")
    return 3 if regressions else 0


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__,
                                 formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("repo", nargs="?", help="Path to the repository")
    ap.add_argument("--run", action="store_true", help="Actually execute the commands")
    ap.add_argument("--label", default="run", help="Label for this run (e.g. baseline)")
    ap.add_argument("--only", help="Comma-separated kinds to include (e.g. test,lint)")
    ap.add_argument("--timeout", type=int, default=600, help="Per-command timeout seconds")
    ap.add_argument("--output", "-o", help="Write the JSON report here")
    ap.add_argument("--compare", nargs=2, metavar=("BEFORE", "AFTER"),
                    help="Compare two JSON reports instead of running")
    ap.add_argument("--quiet", action="store_true")
    args = ap.parse_args()

    if args.compare:
        return compare(Path(args.compare[0]), Path(args.compare[1]))

    if not args.repo:
        ap.error("repo is required unless --compare is used")
    repo = Path(args.repo).resolve()
    if not repo.is_dir():
        sys.stderr.write(f"error: not a directory: {repo}\n")
        return 1

    commands = detect(repo)
    if args.only:
        wanted = {k.strip() for k in args.only.split(",")}
        commands = [c for c in commands if c["kind"] in wanted]

    results = [run_command(repo, c, args.timeout) for c in commands] if args.run \
        else commands

    report = {
        "repo": str(repo),
        "label": args.label,
        "mode": "run" if args.run else "detect",
        "timestamp": time.strftime("%Y-%m-%dT%H:%M:%S"),
        "results": results,
        "summary": summarize(results),
    }

    if not args.quiet:
        print_run(report)
    if args.output:
        Path(args.output).write_text(json.dumps(report, indent=2), encoding="utf-8")
        sys.stdout.write(f"Report written to {args.output}\n")

    if args.run and any(r.get("status") in {"failed", "error", "timeout"} for r in results):
        return 3
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
