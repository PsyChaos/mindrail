# Staged gate reconciliation contract

Date: 2026-09-28
Status: frozen for implementation
Risk tier: 2 (local commit-gate correctness)

## Original report

In an initialized repository, `mindrail verify --staged` continued to report old P0-01/P0-02 scope drift and appeared to compare the commit against the wrong task after the staged set changed.

## Produced diagnosis

- The Vensift Git index contained zero paths while `verify --staged` still denied 89 findings.
- The fixed `verify-staged` change row retained 91 prior `change_files` rows because staged reconciliation upserts current rows but never removes rows absent from the new index snapshot.
- The staged drift projection compares every staged file against every task baseline, including completed tasks. A file correctly owned by one task is therefore reported outside unrelated task scopes.
- Knowledge records are not Git-filtered: the same repository's `mindrail status --json` reads all seven uncommitted decision files directly from `.mindrail/knowledge`.

## Requirements

- REQ-G01: Every `VerifyStaged` run MUST judge exactly the current Git index. Reusing the fixed operation identity MUST replace the prior staged file/symbol projection, including convergence to an empty set.
- REQ-G02: Replacement MUST be atomic. A failed discovery/index/symbol-sync run MUST NOT publish a partial staged snapshot or erase the last internally consistent change projection.
- REQ-G03: Staged scope drift MUST be evaluated against the task that owns/claims a staged file or symbol, not against every unrelated baseline. Completed historical task baselines MUST NOT cause drift for a file attributed to another task.
- REQ-G04: A staged file that belongs to no declared task scope MUST still fail closed as unregistered. A file matching multiple task scopes MUST still fail as ambiguous.
- REQ-G05: A commit containing files from multiple unambiguously owning task scopes MAY pass the scope-attribution layer; it MUST NOT require every file to appear in every task baseline.
- REQ-G06: Directory paths are not introduced as implicit recursive scope in this change. Scope remains canonical concrete file paths; diagnostics MUST not instruct callers to pass unsupported directory-only scope.
- REQ-G07: Existing CI-range parity and staged-vs-worktree isolation MUST remain intact.

## Acceptance criteria

- AC-G01: Stage A, verify, unstage A, verify: the second result contains zero A-derived file/symbol/scope findings.
- AC-G02: Stage A, verify, then replace the index with B: the second stored projection and verdict contain B only.
- AC-G03: With disjoint task baselines A and B, a staged A+B commit produces no cross-task scope drift; staged C produces one unregistered finding; a path declared by both remains ambiguous.
- AC-G04: Empty-index verification is ALLOW when no independent gate condition blocks.
- AC-G05: Focused changes/verify/CLI tests pass, then the repository full gate passes after independent review and audit.

## Decision log

- DEC-G01: The `verify-staged` row is a reusable materialized snapshot, not an append-only history. Exact replacement is required by the command's promise to judge the current index.
- DEC-G02: Ownership/attribution is the selector for the relevant scope. Universal comparison across all task baselines makes disjoint valid tasks mutually exclusive and is rejected.
- DEC-G03: Do not use `--no-verify` as remediation; it bypasses the faulty policy instead of repairing it.
