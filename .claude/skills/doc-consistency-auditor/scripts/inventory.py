#!/usr/bin/env python3
"""Repository inventory helper for the doc-consistency-auditor skill.

Builds a fast, mechanical map of a repository so the audit can start from facts
instead of guesses:

  * every Markdown/docs file with headings, links, code fences and version-like strings
  * source files grouped by language
  * manifests, lockfiles, CI configs, container/IaC files, .env files
  * environment variables referenced in code vs. mentioned in docs vs. declared in .env*

It deliberately does NOT judge anything. It tells you where to look.

Usage:
    python inventory.py <repo-path> [--output inventory.json] [--max-files N]
                        [--exclude DIR ...] [--quiet]
"""

from __future__ import annotations

import argparse
import json
import os
import re
import sys
from collections import Counter, defaultdict
from pathlib import Path

DEFAULT_EXCLUDES = {
    ".git", ".hg", ".svn", "node_modules", "vendor", "dist", "build", "out",
    "target", "__pycache__", ".venv", "venv", "env", ".env.d", ".tox", ".mypy_cache",
    ".pytest_cache", ".next", ".nuxt", ".cache", "coverage", ".idea", ".vscode",
    "site-packages", ".gradle", "Pods", ".terraform", "bin", "obj",
}

DOC_EXTS = {".md", ".markdown", ".mdx", ".rst", ".adoc", ".txt"}

# License and font-license text files are not documentation and drown the doc list.
LICENSE_NOISE_RE = re.compile(
    r"^(license|licence|copying|notice|authors|patents)\b|-(ofl|ofl)\.txt$|ofl\.txt$",
    re.IGNORECASE,
)

CODE_EXTS = {
    ".py": "python", ".js": "javascript", ".jsx": "javascript", ".mjs": "javascript",
    ".cjs": "javascript", ".ts": "typescript", ".tsx": "typescript", ".go": "go",
    ".rs": "rust", ".java": "java", ".kt": "kotlin", ".rb": "ruby", ".php": "php",
    ".cs": "csharp", ".cpp": "cpp", ".cc": "cpp", ".c": "c", ".h": "c-header",
    ".hpp": "cpp-header", ".swift": "swift", ".scala": "scala", ".ex": "elixir",
    ".exs": "elixir", ".sh": "shell", ".bash": "shell", ".sql": "sql",
    ".vue": "vue", ".svelte": "svelte", ".dart": "dart", ".m": "objc",
}

MANIFEST_NAMES = {
    "package.json", "package-lock.json", "yarn.lock", "pnpm-lock.yaml",
    "requirements.txt", "requirements-dev.txt", "pyproject.toml", "poetry.lock",
    "Pipfile", "Pipfile.lock", "setup.py", "setup.cfg", "go.mod", "go.sum",
    "Cargo.toml", "Cargo.lock", "pom.xml", "build.gradle", "build.gradle.kts",
    "Gemfile", "Gemfile.lock", "composer.json", "composer.lock", "mix.exs",
    "pubspec.yaml", "*.csproj",
}

CONTAINER_IAC_HINTS = (
    "dockerfile", "docker-compose", "compose.yml", "compose.yaml", "helm",
    "kustomization", "terraform", ".tf", "serverless.yml", "k8s", "kubernetes",
    "procfile", "vercel.json", "netlify.toml", "fly.toml",
)

API_SPEC_HINTS = (
    "openapi", "swagger", "asyncapi", "schema.graphql", ".graphql", ".gql",
    "api-spec", "apispec",
)

CI_DIR_HINTS = (".github/workflows", ".gitlab-ci", ".circleci", "azure-pipelines",
                "jenkinsfile", ".travis.yml", "bitbucket-pipelines")

TEST_HINTS = ("test", "spec", "__tests__", "e2e")

MIGRATION_HINTS = ("migration", "migrate", "alembic", "flyway", "liquibase", "schema")

# --- regexes -----------------------------------------------------------------

ENV_PATTERNS = [
    re.compile(r"process\.env\.([A-Z_][A-Z0-9_]*)"),
    re.compile(r"process\.env\[['\"]([A-Z_][A-Z0-9_]*)['\"]\]"),
    re.compile(r"os\.environ\.get\(\s*['\"]([A-Z_][A-Z0-9_]*)['\"]"),
    re.compile(r"os\.environ\[\s*['\"]([A-Z_][A-Z0-9_]*)['\"]\s*\]"),
    re.compile(r"os\.getenv\(\s*['\"]([A-Z_][A-Z0-9_]*)['\"]"),
    re.compile(r"getenv\(\s*['\"]([A-Z_][A-Z0-9_]*)['\"]"),
    re.compile(r"ENV\[['\"]([A-Z_][A-Z0-9_]*)['\"]\]"),
    re.compile(r"System\.getenv\(\s*\"([A-Z_][A-Z0-9_]*)\""),
    re.compile(r"\$\{([A-Z_][A-Z0-9_]{2,})\}"),
]

HEADING_RE = re.compile(r"^(#{1,6})\s+(.*?)\s*#*\s*$")
FENCE_RE = re.compile(r"^\s*```+\s*([A-Za-z0-9_+-]*)")
LINK_RE = re.compile(r"\[[^\]]*\]\(([^)\s]+)(?:\s+\"[^\"]*\")?\)")
VERSION_RE = re.compile(
    r"\b(?:v|version\s*|>=|<=|==|~|\^)?\s?(\d+\.\d+(?:\.\d+)?(?:-[0-9A-Za-z.]+)?)\b"
)
ENV_DECL_RE = re.compile(r"^\s*(?:export\s+)?([A-Z_][A-Z0-9_]*)\s*=")
URL_RE = re.compile(r"https?://[^\s)\"'<>\]]+")
DOC_ENV_MENTION_RE = re.compile(r"\b([A-Z][A-Z0-9]*(?:_[A-Z0-9]+){1,})\b")

# "Node 18", "Python 3.11", "Postgres 15" — bare major versions are the most common
# source of setup drift and the plain semver regex misses them.
RUNTIME_NAMES = (
    "node|nodejs|npm|yarn|pnpm|python|pip|java|jdk|go|golang|ruby|rails|php|dotnet|"
    "postgres|postgresql|mysql|mariadb|mongodb|redis|elasticsearch|kafka|rabbitmq|"
    "docker|kubernetes|k8s|django|flask|fastapi|react|next\\.?js|vue|angular|spring|"
    "laravel|rust|cargo|terraform|nginx|ubuntu|debian|alpine"
)
RUNTIME_VERSION_RE = re.compile(
    rf"\b({RUNTIME_NAMES})\b[\s:>=^~v]*?(\d+(?:\.\d+)*)", re.IGNORECASE
)


def is_excluded(path: Path, root: Path, excludes: set[str]) -> bool:
    try:
        rel_parts = path.relative_to(root).parts
    except ValueError:
        return True
    return any(part in excludes for part in rel_parts)


def read_text(path: Path, limit: int = 2_000_000) -> str | None:
    try:
        if path.stat().st_size > limit:
            return None
        return path.read_text(encoding="utf-8", errors="replace")
    except (OSError, ValueError):
        return None


def analyze_doc(path: Path, root: Path) -> dict:
    text = read_text(path)
    rel = str(path.relative_to(root))
    info = {
        "path": rel,
        "size_bytes": path.stat().st_size,
        "lines": 0,
        "headings": [],
        "code_fence_languages": [],
        "internal_links": [],
        "external_links": [],
        "version_strings": [],
        "runtime_versions": [],
        "env_var_mentions": [],
        "unreadable": text is None,
    }
    if text is None:
        return info

    lines = text.splitlines()
    info["lines"] = len(lines)
    in_fence = False
    fence_langs: list[str] = []

    for i, line in enumerate(lines, 1):
        fence = FENCE_RE.match(line)
        if fence:
            if not in_fence:
                in_fence = True
                lang = fence.group(1).lower() or "unspecified"
                fence_langs.append(lang)
            else:
                in_fence = False
            continue
        if in_fence:
            continue
        h = HEADING_RE.match(line)
        if h:
            info["headings"].append(
                {"level": len(h.group(1)), "text": h.group(2).strip(), "line": i}
            )

    info["code_fence_languages"] = [
        {"language": lang, "count": n} for lang, n in Counter(fence_langs).most_common()
    ]

    for target in LINK_RE.findall(text):
        if target.startswith(("http://", "https://")):
            info["external_links"].append(target)
        elif not target.startswith(("#", "mailto:")):
            info["internal_links"].append(target)
    info["external_links"].extend(URL_RE.findall(text))
    info["external_links"] = sorted(set(info["external_links"]))[:50]
    info["internal_links"] = sorted(set(info["internal_links"]))[:100]

    info["version_strings"] = sorted(set(VERSION_RE.findall(text)))[:40]
    info["runtime_versions"] = sorted(
        {f"{name.lower()} {ver}" for name, ver in RUNTIME_VERSION_RE.findall(text)}
    )[:40]

    mentions = {
        m for m in DOC_ENV_MENTION_RE.findall(text)
        if 4 <= len(m) <= 60 and not m.startswith("HTTP_STATUS")
    }
    info["env_var_mentions"] = sorted(mentions)[:120]
    return info


def extract_env_from_code(text: str) -> set[str]:
    found: set[str] = set()
    for pattern in ENV_PATTERNS:
        found.update(pattern.findall(text))
    return found


def classify_special(rel: str) -> list[str]:
    low = rel.lower()
    tags = []
    name = os.path.basename(low)
    if name in {n.lower() for n in MANIFEST_NAMES} or low.endswith(".csproj"):
        tags.append("manifest")
    if any(h in low for h in CONTAINER_IAC_HINTS):
        tags.append("container_iac")
    if any(h in low for h in API_SPEC_HINTS):
        tags.append("api_spec")
    if any(h in low for h in CI_DIR_HINTS):
        tags.append("ci")
    if name.startswith(".env") or name.endswith(".env"):
        tags.append("env_file")
    if any(h in low for h in MIGRATION_HINTS):
        tags.append("migration")
    if any(f"/{h}" in f"/{low}" for h in TEST_HINTS) or name.startswith("test_") \
            or ".test." in name or ".spec." in name:
        tags.append("test")
    return tags


def build_inventory(root: Path, excludes: set[str], max_files: int) -> dict:
    docs: list[dict] = []
    code_by_lang: dict[str, list[str]] = defaultdict(list)
    special: dict[str, list[str]] = defaultdict(list)
    env_from_code: dict[str, list[str]] = defaultdict(list)
    env_declared: dict[str, list[str]] = defaultdict(list)
    scanned = 0
    skipped_for_limit = 0

    for dirpath, dirnames, filenames in os.walk(root):
        dirnames[:] = [d for d in dirnames if d not in excludes]
        for filename in sorted(filenames):
            path = Path(dirpath) / filename
            if is_excluded(path, root, excludes):
                continue
            if path.is_symlink() or not path.is_file():
                continue
            if scanned >= max_files:
                skipped_for_limit += 1
                continue
            scanned += 1
            rel = str(path.relative_to(root))
            ext = path.suffix.lower()

            for tag in classify_special(rel):
                special[tag].append(rel)

            if ext in DOC_EXTS:
                if not LICENSE_NOISE_RE.search(path.name):
                    docs.append(analyze_doc(path, root))
                continue

            if ext in CODE_EXTS:
                code_by_lang[CODE_EXTS[ext]].append(rel)
                text = read_text(path)
                if text:
                    for var in extract_env_from_code(text):
                        env_from_code[var].append(rel)
                continue

            name = path.name.lower()
            if name.startswith(".env") or name.endswith(".env"):
                text = read_text(path)
                if text:
                    for line in text.splitlines():
                        m = ENV_DECL_RE.match(line)
                        if m:
                            env_declared[m.group(1)].append(rel)
            elif ext in {".yml", ".yaml", ".toml", ".json", ".ini", ".cfg", ".tf"}:
                text = read_text(path)
                if text:
                    for var in extract_env_from_code(text):
                        env_from_code[var].append(rel)

    doc_env_mentions: dict[str, list[str]] = defaultdict(list)
    for d in docs:
        for var in d["env_var_mentions"]:
            doc_env_mentions[var].append(d["path"])

    runtime_claims: dict[str, dict[str, list[str]]] = defaultdict(lambda: defaultdict(list))
    for d in docs:
        for claim in d["runtime_versions"]:
            name, _, ver = claim.rpartition(" ")
            runtime_claims[name][ver].append(d["path"])
    conflicting_runtimes = {
        name: {ver: sorted(set(paths)) for ver, paths in sorted(versions.items())}
        for name, versions in sorted(runtime_claims.items())
        if len(versions) > 1
    }

    code_vars = set(env_from_code)
    declared_vars = set(env_declared)
    doc_vars = set(doc_env_mentions)

    env_report = {
        "referenced_in_code": {k: sorted(set(v))[:10] for k, v in sorted(env_from_code.items())},
        "declared_in_env_files": {k: sorted(set(v)) for k, v in sorted(env_declared.items())},
        "in_code_but_not_in_env_files": sorted(code_vars - declared_vars),
        "in_env_files_but_not_in_code": sorted(declared_vars - code_vars),
        "in_code_but_never_mentioned_in_docs": sorted(code_vars - doc_vars),
        "mentioned_in_docs_but_not_in_code": sorted(
            v for v in (doc_vars & declared_vars) - code_vars
        ),
    }

    return {
        "root": str(root),
        "files_scanned": scanned,
        "files_skipped_due_to_limit": skipped_for_limit,
        "documentation": {
            "count": len(docs),
            "files": sorted(docs, key=lambda d: d["path"]),
        },
        "code": {
            "total_files": sum(len(v) for v in code_by_lang.values()),
            "by_language": {
                lang: {"count": len(files), "files": sorted(files)[:200]}
                for lang, files in sorted(
                    code_by_lang.items(), key=lambda kv: -len(kv[1])
                )
            },
        },
        "special_files": {k: sorted(v) for k, v in sorted(special.items())},
        "conflicting_runtime_versions_across_docs": conflicting_runtimes,
        "environment_variables": env_report,
    }


def print_summary(inv: dict) -> None:
    out = sys.stdout.write
    out(f"\n=== Repository inventory: {inv['root']} ===\n")
    out(f"Files scanned: {inv['files_scanned']}\n")
    if inv["files_skipped_due_to_limit"]:
        out(f"!! {inv['files_skipped_due_to_limit']} files skipped (--max-files limit)\n")

    docs = inv["documentation"]["files"]
    out(f"\n-- Documentation ({len(docs)} files) --\n")
    for d in docs[:60]:
        first = d["headings"][0]["text"] if d["headings"] else "(no heading)"
        out(f"  {d['path']:<50} {d['lines']:>5} lines  | {first[:45]}\n")
    if len(docs) > 60:
        out(f"  ... and {len(docs) - 60} more (see JSON)\n")

    out("\n-- Code by language --\n")
    for lang, data in inv["code"]["by_language"].items():
        out(f"  {lang:<14} {data['count']:>5} files\n")

    if inv["special_files"]:
        out("\n-- Special files --\n")
        for tag, files in inv["special_files"].items():
            out(f"  {tag:<14} {len(files):>4}: {', '.join(files[:6])}")
            out(" ...\n" if len(files) > 6 else "\n")

    conflicts = inv["conflicting_runtime_versions_across_docs"]
    if conflicts:
        out("\n-- Docs disagree on runtime/tool versions --\n")
        for name, versions in conflicts.items():
            out(f"  {name}:\n")
            for ver, paths in versions.items():
                out(f"      {ver:<12} <- {', '.join(paths[:4])}\n")

    env = inv["environment_variables"]
    out("\n-- Environment variables (candidate drift signals) --\n")
    out(f"  referenced in code:        {len(env['referenced_in_code'])}\n")
    out(f"  declared in .env files:    {len(env['declared_in_env_files'])}\n")
    for label, key in [
        ("in code, missing from .env files", "in_code_but_not_in_env_files"),
        ("in .env files, unused in code", "in_env_files_but_not_in_code"),
        ("in code, never in docs", "in_code_but_never_mentioned_in_docs"),
        ("documented+declared, unused in code", "mentioned_in_docs_but_not_in_code"),
    ]:
        vals = env[key]
        if vals:
            out(f"  {label} ({len(vals)}): {', '.join(vals[:12])}")
            out(" ...\n" if len(vals) > 12 else "\n")

    out("\nThese are leads, not findings. Verify each one in the source before reporting.\n\n")


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__,
                                 formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("repo", help="Path to the repository root")
    ap.add_argument("--output", "-o", help="Write full JSON inventory here")
    ap.add_argument("--max-files", type=int, default=20000,
                    help="Safety cap on files scanned (default: 20000)")
    ap.add_argument("--exclude", action="append", default=[],
                    help="Extra directory name to exclude (repeatable)")
    ap.add_argument("--quiet", action="store_true", help="Suppress the text summary")
    args = ap.parse_args()

    root = Path(args.repo).resolve()
    if not root.is_dir():
        sys.stderr.write(f"error: not a directory: {root}\n")
        return 1

    excludes = DEFAULT_EXCLUDES | set(args.exclude)
    inv = build_inventory(root, excludes, args.max_files)

    if not args.quiet:
        print_summary(inv)

    if args.output:
        Path(args.output).write_text(json.dumps(inv, indent=2), encoding="utf-8")
        sys.stdout.write(f"Full inventory written to {args.output}\n")

    return 0


if __name__ == "__main__":
    raise SystemExit(main())
