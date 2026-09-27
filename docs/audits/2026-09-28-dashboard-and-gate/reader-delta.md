# Reader delta — post-remediation conformance

Date: 2026-09-28  
Scope: re-check of the four findings in `reader.md` and their affected frozen requirements only

## Finding closure

| Finding | Status | Closure evidence |
| --- | --- | --- |
| RDR-001 — completed sole owner denied | CLOSED | `internal/changes/attribution.go:103-159` now reads durable task states and `PreferredTaskOwners` prefers non-terminal owners only when one exists; otherwise completed ownership remains valid. `internal/verify/verify.go:198-253` applies the same selector to file-level staged ownership. `TestVerifyStagedCompletedSoleOwnerMayCommit` covers both parsed source and unsupported files after the sole owner is `COMPLETED`; both pass. This is the exact complete-then-commit workflow that previously produced `UNREGISTERED_CHANGE`. |
| RDR-002 — CI terminal-owner regression | CLOSED | `internal/changes/staged.go:181-251` now uses `ListTaskChanges` plus `PreferredTaskOwners`; it no longer removes completed changes from shared attribution. `internal/verify/verify.go:202-205` disables staged-only file-owner enforcement for CI while symbol attribution remains shared. An independent external-overlay control created a clean committed range uniquely owned by a completed task and executed `VerifyCI(base, HEAD)`; `TestReaderDeltaCIAttributesCompletedOwner` passed with `Allow:true` and zero denials. |
| RDR-003 — legacy stale symbols accepted by file-only fast path | CLOSED | `internal/verify/verify.go:52-55,80-83` versions the reusable materialized view as `verify-staged-v2`, so an installation with legacy accumulated rows under `verify-staged` cannot enter the equality fast path. The first v2 run builds a complete file/symbol projection; subsequent unchanged file bytes imply the same HEAD-to-index delta. A→empty and A→B exact replacement tests query the v2 identity and pass. |
| RDR-004 — failure/retry atomicity gap | CLOSED | `internal/changes/staged_projection.go` publishes `change_files` and `change_symbols` together in one transaction, after construction succeeds. The durable operation identity created before construction contains no staged projection and is not consumed after an error. `stagedHEADDelta` (`internal/changes/staged.go:101-149`) makes the desired symbol delta authoritative from HEAD→index, so a failed attempt's earlier semantic-index mutation cannot erase symbols on retry. `TestVerifyStagedFailedReplacementPreservesPreviousProjection` and `TestVerifyStagedRetryAfterPartialIndexingKeepsEverySymbolDelta` both pass. |

No prior finding remains open.

## Requirement re-check

### Dashboard observability

| Requirement | Delta verdict | Evidence |
| --- | --- | --- |
| REQ-001–REQ-007 | PASS | Remediation did not regress the previously conformant credential, uptime/SSE, conservative session activity, truncation, or startup-freshness paths. Focused dashboard and CLI packages pass. |
| REQ-008 | NOT_VERIFIABLE | Static client checks, Node behavior assertions, and dashboard tests pass, and extension warnings remain correctly documented. This Reader still has no real-browser console capture for all views/two SSE frames in a clean profile. |
| REQ-009 | PASS | Loopback/token/read-only/bounded/redacted compatibility remains intact; focused dashboard tests pass. |

### Staged gate reconciliation

| Requirement | Delta verdict | Evidence |
| --- | --- | --- |
| REQ-G01 | PASS | The v2 materialized-view identity prevents legacy projection reuse; A→empty and A→B tests prove current-index replacement of files and symbols. |
| REQ-G02 | PASS | File/symbol publication is transactional; failed replacement preserves the prior projection; retry after partial index mutation reconstructs all HEAD→index symbol deltas. |
| REQ-G03 | PASS | Current owners take precedence over historical overlap, while a sole completed owner remains valid. Active-over-historical and completed-sole-owner tests pass. |
| REQ-G04 | PASS | No owner remains unregistered; multiple preferred owners remain ambiguous; unsupported-file ambiguity and completed sole ownership are both covered. |
| REQ-G05 | PASS | Disjoint active task scopes pass, and complete-then-commit no longer loses their durable ownership. |
| REQ-G06 | PASS | Diagnostics continue to request concrete file paths; no recursive directory scope was introduced. |
| REQ-G07 | PASS | Staged/worktree isolation remains covered by the existing suite. Independent committed-range execution confirms a completed owner is attributed in CI. |

### Acceptance criteria

| Criterion | Delta verdict | Evidence / remaining evidence |
| --- | --- | --- |
| AC-001–AC-004 | PASS | Unchanged focused credential, collector, truncation, and fixed-clock client tests pass. |
| AC-005 | PARTIAL | Focused packages pass. The frozen contract's one post-audit full validation run belongs to final reconciliation and was not run by this Reader. |
| AC-006 | PARTIAL | Reader has no unresolved correctness/security finding; final status also depends on the independent Breaker and reconciliation. |
| AC-G01–AC-G04 | PASS | Exact replacement, empty index, disjoint/ambiguous/unregistered ownership, completed owner, and retry tests pass. |
| AC-G05 | PARTIAL | Focused changes/verify/CLI tests pass; repository full gate is pending final reconciliation. |

## Evidence executed

- `go test ./internal/verify ./internal/changes ./internal/cli ./internal/dashboard` with isolated on-disk `TMPDIR` and `GOCACHE` — PASS.
- Independent external-overlay `TestReaderDeltaCIAttributesCompletedOwner` — PASS (`VerifyCI` allowed a clean committed range owned only by a completed task).
- The permanent remediation tests included in the focused run passed, including completed sole ownership and retry after partial indexing.

## Scope and contract delta

- The prior `REQUIREMENT_MISINTERPRETATION` is resolved: “completed historical scope must not compete with current work” is now implemented as precedence, not blanket removal of completed ownership.
- The prior CI `BEHAVIORAL_DRIFT` is resolved by restoring completed changes to shared attribution while keeping staged-only file enforcement out of CI.
- The versioned staged operation is a bounded compatibility migration at the materialized-view identity level; it does not change schema or external snapshot contracts.
- No new scope creep or documentation contradiction was found in the remediation.

## Remaining exact evidence

1. Run the repository full validation gate once after both audits/remediation are final, as required by AC-005/AC-G05.
2. For REQ-008, capture a clean-profile browser console while loading all dashboard views across at least two SSE frames and assert zero warnings/errors sourced from Mindrail assets.

## Final Reader verdict

`COMPLIANT_WITH_MINOR_ISSUES`

All four prior implementation findings are closed with code-path and executed-test evidence. The only remaining items are final validation/audit orchestration and the clean-browser console observation; neither is a presently identified implementation defect.
