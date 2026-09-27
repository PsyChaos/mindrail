#!/usr/bin/env python3
"""Audit scoping helper for the engineering-orchestrator skill.

Turns a changeset into an *audit plan*: which files (or TASK-* clusters) need the
full dual-agent treatment, which need only targeted breaking, and which need only a
conformance read. The point is to make audit cost scale with risk rather than with
the number of sub-tasks - and to make that scoping decision visible instead of
silent.

It also estimates the cost of the Breaker's mutation move (guards x suite runtime),
which is the single largest time sink of a big audit, so a targeted test command can
be supplied where the full suite would be too slow.

Usage:
    python audit_scope.py <repo> --base <ref>                        # plan for everything since <ref>
    python audit_scope.py <repo> --base <ref> --tasks tasks.json      # cluster by TASK-* instead of directory
    python audit_scope.py <repo> --base <last-audited-ref> --delta    # remediation re-audit: delta only
    python audit_scope.py <repo> --base <ref> --test-seconds 90 -o plan.json

tasks.json (optional) - maps tasks to the files they own, with optional overrides:
    [
      {"id": "TASK-001", "files": ["src/auth/*.py"], "test_cmd": "pytest tests/auth -q"},
      {"id": "TASK-002", "files": ["docs/**"], "depth": "C"}
    ]

Depth levels (see references/audit-scoping.md):
    A  full        Reader + Breaker, all six moves + off-spec headings
    B  targeted    Reader + Breaker moves 1 (mutation) and 5 (weakest input) + class sweep
    C  conformance Reader only

Depth is a floor. Auditors may raise it on evidence; the orchestrator never lowers it
below what this classification says without writing the reason into the audit plan.

Read-only: it only runs `git diff` / `git status` and never modifies the tree.
"""

from __future__ import annotations

import argparse
import fnmatch
import json
import re
import subprocess
import sys
from collections import defaultdict
from pathlib import Path, PurePosixPath

# --------------------------------------------------------------------------- #
# Classification rules
# --------------------------------------------------------------------------- #

# Path fragments that put a file at depth A regardless of what the diff contains.
# These are the same escalators SKILL.md uses to raise the tier: size is a weak proxy
# for risk, and these surfaces are where a three-line diff can take a site down.
PATH_A = [
    (r"(^|/)(migrations?|alembic|schema|schemas|models?)(/|\.|$)", "schema / persistence"),
    (r"(^|/)(auth|authn|authz|login|session|token|jwt|oauth|permission|perm|acl|rbac|policy)", "authentication / authorization"),
    (r"(^|/)(api|routes?|routers?|endpoints?|controllers?|handlers?|views?|graphql|grpc|proto)(/|\.|$)", "public API surface"),
    (r"(^|/)(openapi|swagger|contract|interface|dto)s?(/|\.|$)", "contract / DTO"),
    (r"(^|/)(config|settings|defaults?|env)(/|\.|$)|\.env(\.|$)", "configuration / defaults"),
    (r"(^|/)(secret|credential|vault|keys?)(/|\.|$)", "secrets / credentials"),
    (r"(^|/)(\.github/workflows|\.gitlab-ci|ci|cd|deploy|helm|k8s|kubernetes|terraform|ansible)(/|\.|$)|Dockerfile|docker-compose", "CI / deployment"),
    (r"(^|/)(billing|payment|checkout|invoice|ledger)(/|\.|$)", "money-moving code"),
    (r"(^|/)(validator|validation|guard|budget|quota|limit|rate)s?(/|\.|$)", "guard / validator / budget"),
]

# Added-line content that escalates to depth A even in an innocuous-looking file.
CONTENT_A = [
    (r"\b(DROP\s+(TABLE|COLUMN|INDEX)|DELETE\s+FROM|TRUNCATE)\b", "destructive SQL"),
    (r"\b(os\.remove|os\.unlink|shutil\.rmtree|rm\s+-rf|fs\.rmSync|unlinkSync)\b", "destructive filesystem operation"),
    (r"\b(is_admin|is_superuser|has_perm|authorize|@login_required|@permission|check_permission|verify_token)\b", "authorization check"),
    (r"\b(password|passwd|api[_-]?key|secret|private[_-]?key|client[_-]?secret)\b\s*[:=]", "credential handling"),
    (r"\b(ALTER\s+TABLE|CREATE\s+TABLE|AddField|RemoveField|op\.(add|drop|alter)_column)\b", "schema change"),
    (r"\b(deprecated|backward|backwards|compat|legacy|breaking)\b", "compatibility-sensitive wording"),
]

# Depth C candidates: files whose failure mode is "wrong words", not "wrong behavior".
PATH_C = [
    (r"\.(md|rst|txt|adoc)$|(^|/)docs?/|(^|/)CHANGELOG|(^|/)LICENSE", "documentation"),
    (r"(^|/)(assets|static|public)/.*\.(png|jpg|jpeg|gif|svg|ico|woff2?)$", "binary asset"),
]

# Test files are depth B by default: no mutation runs *on* them, but the Reader must
# confirm they assert something and the Breaker's move 1 uses them as the oracle.
PATH_TEST = r"(^|/)(tests?|__tests__|spec|specs)/|(^|/)test_[^/]+\.py$|[._-](test|spec)\.[jt]sx?$|_test\.go$|Test[^/]*\.(java|kt|cs)$"

# Lock / dependency manifests: an incompatible package is an escalation reason in
# Phase 13, so these are B with an explicit note rather than C.
PATH_DEPS = r"(^|/)(package(-lock)?\.json|yarn\.lock|pnpm-lock\.yaml|requirements[^/]*\.txt|pyproject\.toml|poetry\.lock|Pipfile(\.lock)?|go\.(mod|sum)|Cargo\.(toml|lock)|Gemfile(\.lock)?|pom\.xml|build\.gradle(\.kts)?)$"

# Added lines that look like a guard/branch. Each of these is one mutation run for
# the Breaker, so counting them is how the cost estimate is built.
GUARD_RE = re.compile(
    r"^\s*(if|elif|else if|unless|guard|match|case|when|switch|catch|except|rescue)\b"
    r"|^\s*(raise|throw|assert|panic|abort|return\s+(None|False|null|nil|false|err|0|-1)\b)"
    r"|\?\s*[^:]+\s*:|\|\||&&|\band\b|\bor\b|\bnot\b|!=|==|<=|>=|<|>"
)

# Signals for the Breaker's conditional moves.
THREAT_RE = re.compile(r"\b(attacker|malicious|adversar|threat|abuse|tamper|forg|spoof|inject|bypass|untrusted)", re.I)
STORAGE_RE = re.compile(r"\b(migration|migrate|schema|table|column|serializ|deserializ|pickle|json\.load|from_dict|load_state|persist|upgrade)\b", re.I)


# --------------------------------------------------------------------------- #
# git helpers (read-only)
# --------------------------------------------------------------------------- #

def git(repo: Path, *args: str) -> str:
    try:
        return subprocess.run(["git", *args], cwd=repo, check=True, capture_output=True,
                              text=True).stdout
    except (subprocess.CalledProcessError, FileNotFoundError) as exc:
        sys.exit(f"git {' '.join(args)} failed: {getattr(exc, 'stderr', exc)}")


def changed_files(repo: Path, base: str) -> dict[str, str]:
    """path -> status letter (A/M/D/R). Includes untracked files as 'A'."""
    files: dict[str, str] = {}
    for line in git(repo, "diff", "--name-status", "-M", base).splitlines():
        parts = line.split("\t")
        status = parts[0][0]
        path = parts[-1]
        files[path] = status
    for line in git(repo, "status", "--porcelain", "--untracked-files=all").splitlines():
        if line.startswith("??"):
            files[line[3:].strip()] = "A"
    return files


def diff_lines(repo: Path, base: str, path: str) -> tuple[list[str], list[str]]:
    """(added, removed) lines for one file. Untracked files read whole as added."""
    out = subprocess.run(["git", "diff", "-U0", "-M", base, "--", path], cwd=repo,
                         capture_output=True, text=True).stdout
    added, removed = [], []
    if out:
        for line in out.splitlines():
            if line.startswith("+++") or line.startswith("---"):
                continue
            if line.startswith("+"):
                added.append(line[1:])
            elif line.startswith("-"):
                removed.append(line[1:])
    else:
        try:
            added = (repo / path).read_text(encoding="utf-8", errors="replace").splitlines()
        except OSError:
            pass
    return added, removed


# --------------------------------------------------------------------------- #
# Classification
# --------------------------------------------------------------------------- #

def classify(path: str, status: str, added: list[str], removed: list[str]) -> dict:
    reasons: list[str] = []
    depth = "B"
    is_test = bool(re.search(PATH_TEST, path))
    is_deps = bool(re.search(PATH_DEPS, path))

    for pat, why in PATH_A:
        if re.search(pat, path, re.I):
            depth = "A"
            reasons.append(f"path: {why}")
            break

    if not is_test:
        joined = "\n".join(added)
        for pat, why in CONTENT_A:
            if re.search(pat, joined, re.I):
                depth = "A"
                reasons.append(f"content: {why}")

    if depth != "A":
        for pat, why in PATH_C:
            if re.search(pat, path, re.I):
                depth = "C"
                reasons.append(f"path: {why}")
                break

    if is_test:
        reasons.append("test file: Reader checks the assertions are real; no mutation on the test itself")
    if is_deps:
        depth = "A" if depth == "A" else "B"
        reasons.append("dependency manifest: check for incompatible / changed versions")

    code_added = [l for l in added if l.strip() and not l.strip().startswith(("#", "//", "/*", "*", "--"))]
    guards = 0 if (is_test or depth == "C") else sum(1 for l in code_added if GUARD_RE.search(l))

    signals = {
        "deleted_behavior": bool(removed) and status != "A" and depth != "C",
        "threat_model_described": bool(THREAT_RE.search("\n".join(added))),
        "storage_or_upgrade": bool(STORAGE_RE.search("\n".join(added))) and not is_test,
    }
    if not reasons:
        reasons.append("ordinary code change")
    return {
        "path": path,
        "status": status,
        "depth": depth,
        "reasons": reasons,
        "added": len(added),
        "removed": len(removed),
        "guards": guards,
        "signals": signals,
    }


def load_tasks(path: Path | None) -> list[dict]:
    if not path:
        return []
    try:
        data = json.loads(path.read_text(encoding="utf-8"))
    except (OSError, ValueError) as exc:
        sys.exit(f"could not read tasks file: {exc}")
    return data if isinstance(data, list) else data.get("tasks", [])


def assign_cluster(path: str, tasks: list[dict]) -> str:
    for task in tasks:
        for pattern in task.get("files", []):
            if fnmatch.fnmatch(path, pattern) or PurePosixPath(path).match(pattern):
                return task["id"]
    if tasks:
        return "UNASSIGNED"
    parts = PurePosixPath(path).parts
    return parts[0] if len(parts) > 1 else "(root)"


DEPTH_ORDER = {"C": 0, "B": 1, "A": 2}


def build_plan(files: list[dict], tasks: list[dict], test_seconds: float | None) -> dict:
    by_task = {t["id"]: t for t in tasks}
    clusters: dict[str, dict] = defaultdict(lambda: {"files": [], "depth": "C", "guards": 0,
                                                     "reasons": set(), "signals": defaultdict(bool)})
    for f in files:
        cid = assign_cluster(f["path"], tasks)
        c = clusters[cid]
        c["files"].append(f["path"])
        c["guards"] += f["guards"]
        if DEPTH_ORDER[f["depth"]] > DEPTH_ORDER[c["depth"]]:
            c["depth"] = f["depth"]
        if f["depth"] != "C":
            c["reasons"].update(f["reasons"])
        for k, v in f["signals"].items():
            c["signals"][k] = c["signals"][k] or v

    plan = []
    for cid, c in sorted(clusters.items(), key=lambda kv: (-DEPTH_ORDER[kv[1]["depth"]], kv[0])):
        override = by_task.get(cid, {})
        depth = c["depth"]
        note = None
        if override.get("depth"):
            requested = override["depth"].upper()
            if DEPTH_ORDER[requested] < DEPTH_ORDER[depth]:
                note = (f"tasks.json asked for depth {requested} but classification says {depth}; "
                        f"kept {depth} - lowering must be justified in the audit plan by hand")
            else:
                depth = requested
        moves = {"A": "1-6 + off-spec headings", "B": "1, 5 + class sweep", "C": "Reader only"}[depth]
        conditional = []
        if depth != "C":
            if c["signals"]["deleted_behavior"]:
                conditional.append("2 (behavior was removed)")
            if c["signals"]["threat_model_described"]:
                conditional.append("3 (a threat model is described)")
            if c["signals"]["storage_or_upgrade"]:
                conditional.append("4 (storage / upgrade path touched)")
        mutation_runs = c["guards"] if depth != "C" else 0
        est = round(mutation_runs * test_seconds / 60, 1) if test_seconds else None
        plan.append({
            "cluster": cid,
            "depth": depth,
            "breaker_moves": moves,
            "conditional_moves_triggered": conditional,
            "files": sorted(c["files"]),
            "guards": c["guards"],
            "mutation_runs": mutation_runs,
            "mutation_minutes_est": est,
            "test_cmd": override.get("test_cmd"),
            "reasons": sorted(c["reasons"]),
            "note": note,
        })

    total_runs = sum(p["mutation_runs"] for p in plan)
    total_est = round(total_runs * test_seconds / 60, 1) if test_seconds else None
    warnings = []
    if test_seconds and total_est and total_est > 30:
        warnings.append(
            f"Mutation alone is projected at ~{total_est} min with the full suite. Give every "
            f"A/B cluster a targeted test_cmd (from the Phase 15 test map) and run the full "
            f"suite once at the end instead of once per guard.")
    unassigned = [p for p in plan if p["cluster"] == "UNASSIGNED"]
    if unassigned:
        warnings.append("Some changed files belong to no TASK-*: either scope creep or a missing "
                        "task entry. Either way it is a Reader finding before the audit starts.")
    return {
        "clusters": plan,
        "totals": {
            "files": len(files),
            "clusters": len(plan),
            "depth_A": sum(1 for p in plan if p["depth"] == "A"),
            "depth_B": sum(1 for p in plan if p["depth"] == "B"),
            "depth_C": sum(1 for p in plan if p["depth"] == "C"),
            "mutation_runs": total_runs,
            "mutation_minutes_est": total_est,
        },
        "warnings": warnings,
    }


def print_markdown(report: dict) -> None:
    t = report["totals"]
    print(f"# Audit plan  (base: `{report['base']}`{', delta re-audit' if report['delta'] else ''})\n")
    print(f"{t['files']} changed files in {t['clusters']} clusters - "
          f"A: {t['depth_A']}, B: {t['depth_B']}, C: {t['depth_C']}. "
          f"Mutation runs: {t['mutation_runs']}"
          + (f" (~{t['mutation_minutes_est']} min)" if t['mutation_minutes_est'] is not None else "")
          + "\n")
    print("| Cluster | Depth | Breaker moves | Conditional | Guards | Test cmd | Why |")
    print("| --- | --- | --- | --- | --- | --- | --- |")
    for p in report["clusters"]:
        why = "; ".join(p["reasons"][:3]) or "-"
        cond = ", ".join(p["conditional_moves_triggered"]) or "-"
        print(f"| {p['cluster']} | **{p['depth']}** | {p['breaker_moves']} | {cond} | "
              f"{p['guards']} | {'-' if p['depth'] == 'C' else (p['test_cmd'] or '(full suite)')} | {why} |")
    print()
    for p in report["clusters"]:
        print(f"- **{p['cluster']}** ({p['depth']}): " + ", ".join(p["files"]))
        if p["note"]:
            print(f"  - note: {p['note']}")
    if report["warnings"]:
        print("\n## Warnings")
        for w in report["warnings"]:
            print(f"- {w}")


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__,
                                 formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("repo", help="Path to the repository")
    ap.add_argument("--base", required=True,
                    help="Ref the changeset is measured from (task start, or the last audited ref for --delta)")
    ap.add_argument("--tasks", help="tasks.json mapping TASK-* ids to file globs")
    ap.add_argument("--test-seconds", type=float,
                    help="Full suite runtime in seconds; enables the mutation cost estimate")
    ap.add_argument("--delta", action="store_true",
                    help="Mark this as a remediation re-audit (base = last audited ref)")
    ap.add_argument("--output", "-o", help="Write the JSON plan here")
    ap.add_argument("--quiet", action="store_true")
    args = ap.parse_args()

    repo = Path(args.repo).resolve()
    if not (repo / ".git").exists() and not git(repo, "rev-parse", "--git-dir"):
        sys.exit(f"{repo} is not a git repository")

    tasks = load_tasks(Path(args.tasks) if args.tasks else None)
    files = []
    for path, status in sorted(changed_files(repo, args.base).items()):
        if status == "D":
            files.append({"path": path, "status": "D", "depth": "B",
                          "reasons": ["file deleted: Breaker move 2 (A/B of deleted behavior) applies"],
                          "added": 0, "removed": 0, "guards": 0,
                          "signals": {"deleted_behavior": True, "threat_model_described": False,
                                      "storage_or_upgrade": False}})
            continue
        added, removed = diff_lines(repo, args.base, path)
        files.append(classify(path, status, added, removed))

    report = build_plan(files, tasks, args.test_seconds)
    report["base"] = args.base
    report["delta"] = args.delta
    report["files"] = files

    if args.output:
        Path(args.output).write_text(json.dumps(report, indent=2), encoding="utf-8")
    if not args.quiet:
        print_markdown(report)
    return 0


if __name__ == "__main__":
    sys.exit(main())
