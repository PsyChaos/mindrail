#!/usr/bin/env python3
"""Changeset scoping helper for the dual-agent-task-audit skill.

Summarizes what a git changeset actually touched, so the audit starts from facts
about the diff instead of an impression of it:

  * added / modified / deleted / renamed files with churn counts
  * classification per file (source, test, config, dependency manifest, migration,
    CI, container/IaC, documentation)
  * high-risk touches (dependencies, migrations, CI, auth-ish paths) called out
  * source files changed with no corresponding test change
  * commit list in the range

This tool scopes the audit. It is NOT evidence. A changed file is not a completed
requirement, and an unchanged file is not proof that behavior is unaffected.

Usage:
    python changeset.py <repo-path> [--base HEAD~1] [--head HEAD]
                        [--output changeset.json] [--quiet]
"""

from __future__ import annotations

import argparse
import json
import os
import re
import subprocess
import sys
from collections import defaultdict
from pathlib import Path

TEST_HINTS = ("test", "spec", "__tests__", "e2e", "fixtures")
DEP_FILES = {
    "package.json", "package-lock.json", "yarn.lock", "pnpm-lock.yaml",
    "requirements.txt", "pyproject.toml", "poetry.lock", "Pipfile", "Pipfile.lock",
    "go.mod", "go.sum", "Cargo.toml", "Cargo.lock", "pom.xml", "build.gradle",
    "build.gradle.kts", "Gemfile", "Gemfile.lock", "composer.json", "composer.lock",
    "mix.exs", "pubspec.yaml",
}
CONFIG_EXTS = {".yml", ".yaml", ".toml", ".ini", ".cfg", ".conf", ".properties", ".json"}
DOC_EXTS = {".md", ".markdown", ".mdx", ".rst", ".adoc", ".txt"}
CI_HINTS = (".github/workflows", ".gitlab-ci", ".circleci", "azure-pipelines",
            "jenkinsfile", ".travis.yml", "bitbucket-pipelines")
IAC_HINTS = ("dockerfile", "docker-compose", "compose.yml", "compose.yaml", ".tf",
             "helm", "kustomization", "k8s", "kubernetes", "serverless.yml")
MIGRATION_HINTS = ("migration", "migrate", "alembic", "flyway", "liquibase")
SENSITIVE_HINTS = ("auth", "login", "session", "token", "password", "secret",
                   "crypto", "permission", "role", "acl", "payment", "billing")


def run_git(repo: Path, args: list[str]) -> tuple[int, str, str]:
    try:
        p = subprocess.run(
            ["git", "-C", str(repo), *args],
            capture_output=True, text=True, timeout=120,
        )
        return p.returncode, p.stdout, p.stderr
    except FileNotFoundError:
        return 127, "", "git executable not found"
    except subprocess.TimeoutExpired:
        return 124, "", "git command timed out"


def classify(path: str) -> list[str]:
    low = path.lower()
    name = os.path.basename(low)
    ext = os.path.splitext(low)[1]
    tags: list[str] = []

    if name in {d.lower() for d in DEP_FILES} or low.endswith(".csproj"):
        tags.append("dependency")
    if any(h in low for h in CI_HINTS):
        tags.append("ci")
    if any(h in low for h in IAC_HINTS) or name.startswith("dockerfile"):
        tags.append("container_iac")
    if any(h in low for h in MIGRATION_HINTS):
        tags.append("migration")
    if ext in DOC_EXTS:
        tags.append("documentation")
    if (any(f"/{h}" in f"/{low}" for h in TEST_HINTS)
            or name.startswith("test_") or ".test." in name or ".spec." in name):
        tags.append("test")
    if ext in CONFIG_EXTS and "dependency" not in tags:
        tags.append("config")
    if name.startswith(".env"):
        tags.append("env_file")
    if any(h in low for h in SENSITIVE_HINTS):
        tags.append("sensitive")
    if not tags or tags == ["sensitive"]:
        tags.append("source")
    return tags


def parse_name_status(out: str) -> list[dict]:
    files = []
    for line in out.splitlines():
        if not line.strip():
            continue
        parts = line.split("\t")
        status = parts[0]
        if status.startswith("R") and len(parts) >= 3:
            files.append({"status": "renamed", "path": parts[2], "old_path": parts[1]})
        elif status.startswith("C") and len(parts) >= 3:
            files.append({"status": "copied", "path": parts[2], "old_path": parts[1]})
        elif len(parts) >= 2:
            mapping = {"A": "added", "M": "modified", "D": "deleted", "T": "typechange"}
            files.append({"status": mapping.get(status[0], status), "path": parts[1]})
    return files


def parse_numstat(out: str) -> dict[str, dict]:
    churn: dict[str, dict] = {}
    for line in out.splitlines():
        if not line.strip():
            continue
        parts = line.split("\t")
        if len(parts) < 3:
            continue
        added, deleted, path = parts[0], parts[1], parts[-1]
        churn[path] = {
            "added_lines": None if added == "-" else int(added),
            "deleted_lines": None if deleted == "-" else int(deleted),
            "binary": added == "-",
        }
    return churn


def test_dirs_for(path: str) -> set[str]:
    """Rough module key so a source file and its test can be matched."""
    stem = os.path.splitext(os.path.basename(path))[0]
    stem = re.sub(r"^test_|_test$|\.test$|\.spec$", "", stem)
    return {stem.lower()}


def build(repo: Path, base: str, head: str) -> dict:
    rc, _, err = run_git(repo, ["rev-parse", "--is-inside-work-tree"])
    if rc != 0:
        return {"error": f"not a git repository ({err.strip() or 'rev-parse failed'})",
                "repo": str(repo)}

    rc, _, err = run_git(repo, ["rev-parse", "--verify", base])
    if rc != 0:
        return {"error": f"base ref '{base}' not found: {err.strip()}", "repo": str(repo)}

    rng = f"{base}..{head}"
    rc, ns_out, err = run_git(repo, ["diff", "--name-status", "-M", rng])
    if rc != 0:
        return {"error": f"git diff failed: {err.strip()}", "repo": str(repo)}
    rc, num_out, _ = run_git(repo, ["diff", "--numstat", "-M", rng])
    _, log_out, _ = run_git(
        repo, ["log", "--pretty=format:%h|%an|%ad|%s", "--date=short", rng]
    )

    files = parse_name_status(ns_out)
    churn = parse_numstat(num_out)

    by_status: dict[str, list[str]] = defaultdict(list)
    by_tag: dict[str, list[str]] = defaultdict(list)
    for f in files:
        f["tags"] = classify(f["path"])
        f.update(churn.get(f["path"], {}))
        by_status[f["status"]].append(f["path"])
        for tag in f["tags"]:
            by_tag[tag].append(f["path"])

    changed_test_keys: set[str] = set()
    for p in by_tag.get("test", []):
        changed_test_keys |= test_dirs_for(p)

    source_without_test = [
        f["path"] for f in files
        if "source" in f["tags"] and f["status"] != "deleted"
        and not (test_dirs_for(f["path"]) & changed_test_keys)
    ]

    commits = []
    for line in log_out.splitlines():
        parts = line.split("|", 3)
        if len(parts) == 4:
            commits.append({"sha": parts[0], "author": parts[1],
                            "date": parts[2], "subject": parts[3]})

    high_risk = sorted({
        p for tag in ("dependency", "migration", "ci", "container_iac", "env_file",
                      "sensitive")
        for p in by_tag.get(tag, [])
        if p not in set(by_tag.get("test", []))
    })

    total_added = sum(f.get("added_lines") or 0 for f in files)
    total_deleted = sum(f.get("deleted_lines") or 0 for f in files)

    return {
        "repo": str(repo),
        "range": rng,
        "commit_count": len(commits),
        "commits": commits,
        "file_count": len(files),
        "lines_added": total_added,
        "lines_deleted": total_deleted,
        "files": sorted(files, key=lambda f: f["path"]),
        "by_status": {k: sorted(v) for k, v in sorted(by_status.items())},
        "by_category": {k: sorted(v) for k, v in sorted(by_tag.items())},
        "high_risk_touches": high_risk,
        "source_changed_without_test_change": sorted(source_without_test),
    }


def print_summary(cs: dict) -> None:
    out = sys.stdout.write
    if "error" in cs:
        out(f"\n!! {cs['error']}\n"
            "   Proceed without changeset scoping: identify the implementation from "
            "the task description and inspect it directly.\n\n")
        return

    out(f"\n=== Changeset: {cs['repo']}  [{cs['range']}] ===\n")
    out(f"{cs['commit_count']} commit(s), {cs['file_count']} file(s), "
        f"+{cs['lines_added']} / -{cs['lines_deleted']} lines\n")

    if cs["commits"]:
        out("\n-- Commits --\n")
        for c in cs["commits"][:20]:
            out(f"  {c['sha']}  {c['date']}  {c['subject'][:70]}\n")
        if len(cs["commits"]) > 20:
            out(f"  ... and {len(cs['commits']) - 20} more\n")

    out("\n-- By status --\n")
    for status, paths in cs["by_status"].items():
        out(f"  {status:<12} {len(paths):>4}: {', '.join(paths[:5])}")
        out(" ...\n" if len(paths) > 5 else "\n")

    out("\n-- By category --\n")
    for tag, paths in cs["by_category"].items():
        out(f"  {tag:<15} {len(paths):>4}\n")

    if cs["high_risk_touches"]:
        out("\n-- High-risk touches (verify these first) --\n")
        for p in cs["high_risk_touches"]:
            out(f"  {p}\n")

    missing = cs["source_changed_without_test_change"]
    if missing:
        out(f"\n-- Source changed with no matching test change ({len(missing)}) --\n")
        for p in missing[:25]:
            out(f"  {p}\n")
        if len(missing) > 25:
            out(f"  ... and {len(missing) - 25} more\n")

    out("\nScoping only. A changed file is not a satisfied requirement, and an "
        "unchanged file is not proof of unchanged behavior.\n\n")


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__,
                                 formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("repo", help="Path to the git repository")
    ap.add_argument("--base", default="HEAD~1", help="Base ref (default: HEAD~1)")
    ap.add_argument("--head", default="HEAD", help="Head ref (default: HEAD)")
    ap.add_argument("--output", "-o", help="Write full JSON here")
    ap.add_argument("--quiet", action="store_true", help="Suppress the text summary")
    args = ap.parse_args()

    repo = Path(args.repo).resolve()
    if not repo.is_dir():
        sys.stderr.write(f"error: not a directory: {repo}\n")
        return 1

    cs = build(repo, args.base, args.head)

    if not args.quiet:
        print_summary(cs)
    if args.output:
        Path(args.output).write_text(json.dumps(cs, indent=2), encoding="utf-8")
        sys.stdout.write(f"Full changeset written to {args.output}\n")

    return 2 if "error" in cs else 0


if __name__ == "__main__":
    raise SystemExit(main())
