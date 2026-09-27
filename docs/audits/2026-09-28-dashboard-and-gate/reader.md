# Reader audit — dashboard observability and staged gate

Date: 2026-09-28  
Role: independent Reader / conformance audit  
Implementation base: `31220dc4c89083ea0bc1b0ee41a48de3bdfe879e` plus the audit-plan working-tree changes

## R1 — Task normalization

This table was derived from the verbatim user text in `audit-plan.md` before reading the implementation.

| ID | Requirement | Source from task | Type | Verification method |
| --- | --- | --- | --- | --- |
| USR-001 | Fix the reported dashboard defects as a coherent whole. | “her şeyin düzeltilmesi gerekiyor” | FUNCTIONAL | Compare the dashboard behavior and frozen observability contract with implementation and focused tests. |
| USR-002 | Make it possible to see whether the dashboard is updating, how long it has run, and which durable agent/session is associated with current work. | “gerçekten çalışıp çalışmadığını anlayamıyorum. ne kadar süredir çalışıyor, hangi agent çalışıyor vs görmek istiyorum” | BEHAVIORAL | Inspect snapshot/UI contracts and execute fixed-clock collector/client tests. |
| USR-003 | Identify the real causes of the reported zero-decision display and stale/wrong-task commit-gate findings. | “şöyle bir durum geldi … sebepleri ne olabilir?” plus the quoted report | ACCEPTANCE | Trace knowledge loading and staged projection/attribution data flow. |
| USR-004 | Restore the normal Mindrail commit workflow so Vensift need not bypass the gate or remain paused. | “Vensift'i o zaman mindrail düzeltilene kadar bekletiyorum.” | ACCEPTANCE | Exercise the post-completion staged-commit workflow and inspect CI parity. |
| USR-005 | Preserve existing read-only/security/compatibility boundaries while fixing the above. | Implied by fixing Mindrail rather than bypassing it | NON_REGRESSION | Review changed interfaces, security seams, boundedness, and focused regression tests. |

`TASK_AMBIGUITY`: “hangi agent” can mean exact client/model identity or the durable Mindrail session responsible for work. The current schema does not record authoritative client/model identity or process liveness. The frozen contract resolves this safely: expose session label/ID and durable task/lease/activity signals, and explicitly state that exact identity/liveness is unavailable. The implementation is evaluated against that truthful interpretation; fabricating exact identity would not satisfy the task.

## R2 — Requirement verdicts

### Live dashboard contract

| Requirement | Verdict | Evidence | Notes |
| --- | --- | --- | --- |
| REQ-001 | PASS | `internal/cli/dashboard.go:101-116` applies environment → keyring → none precedence; `DashboardStart` carries metadata only. `TestResolveDashboardJEVUsesEnvironmentWithoutReadingKeyring` and the focused CLI tests pass. | No credential value or full environment crosses the long-running dashboard seam. |
| REQ-002 | PASS | `internal/cli/agent.go:104-132` bounds the keyring read to one second and maps failures to unavailable; `app.js` renders unavailable separately and labels restart behavior. | Keyring lookup is a startup snapshot and fails open. |
| REQ-003 | PASS | `collector.go:123-124` emits start/uptime; `index.html` distinguishes dashboard stream health from agent liveness; `view.js` renders elapsed values. Fixed-clock Node assertions passed. | The connection label is explicitly dashboard/SSE health. |
| REQ-004 | PASS | `collector.go:504-691` derives current claimed tasks, active leases, latest activity, renewal and expiry from the read transaction; `app.js:120-164` renders them with session label/ID and age. | Positive collector and client behavior tests pass. |
| REQ-005 | PASS | `applySessionActivity` uses `active_signal` only when both a non-terminal claim and valid lease exist; HTML/docs explicitly disclaim exact client/model identity and process liveness. | Claim-only and lease-only are not labelled live/working. |
| REQ-006 | PASS | Activity counts are queried independently from bounded top-level arrays. `TestCollectorSessionActivityIsIndependentOfTaskDetailTruncation`, current-task bound, and large invisible-history tests pass. | Displayed task names have their own explicit bound. |
| REQ-007 | PASS | Readiness remains `startup_snapshot` in `collector.go`; HTML, telemetry, README and Turkish usage docs say restart to refresh. | No SSE freshness claim is made. |
| REQ-008 | NOT_VERIFIABLE | Extension warnings are correctly documented as external, static sink checks pass, Node client behavior passes, and `go test ./internal/dashboard` passes. | No browser run captured the supported path's console. Exact missing evidence: launch the tokenized dashboard in a clean browser profile, load all three views through at least two SSE frames, and assert zero console warning/error entries whose source is a Mindrail asset. |
| REQ-009 | PASS | The transport/security server code is unchanged in its loopback/token/origin/CSP/read-only boundaries; snapshot additions remain bounded and redacted. Focused dashboard/server tests pass. | Snapshot additions are additive. |

### Staged gate reconciliation contract

| Requirement | Verdict | Evidence | Notes |
| --- | --- | --- | --- |
| REQ-G01 | PARTIAL | Replacement tests prove A→empty and A→B file/symbol clearing. However `ReconcileStaged` returns early on file equality at `internal/changes/staged.go:46-52` without proving/rebuilding exact symbol rows. | RDR-003. |
| REQ-G02 | PARTIAL | `replaceChangeProjection` replaces files/symbols in one transaction and the failed-replacement test preserves a previous projection. But `EnsureOpenChange` publishes the target row before symbol construction (`staged.go:42-64`), and file indexing occurs before the replacement transaction. | The established-projection case passes; the first-run failure case is not atomic as written. See RDR-004. |
| REQ-G03 | FAIL | Both baseline and symbol candidates are restricted to non-terminal tasks (`attribution.go:103-113`, `automatic.go:142-170`). A produced post-completion test denies the sole correct owner as unregistered. | RDR-001. |
| REQ-G04 | PARTIAL | No-owner and overlapping-active-owner tests pass. A uniquely declared completed owner is nevertheless treated as no owner, so the no-declared-scope rule is applied to a file that does have declared scope. | RDR-001. |
| REQ-G05 | PARTIAL | `TestVerifyStagedDisjointTaskScopesDoNotCrossDrift` passes for active tasks. The same commit after normal task completion is denied. | The user-facing workflow is not restored. |
| REQ-G06 | PASS | New remedies name a concrete file path and explicitly avoid directory-only scope semantics (`internal/changes/scope.go`). | No implicit recursive directory scope was added. |
| REQ-G07 | FAIL | Staged/worktree isolation tests pass, but shared `AttributeChanges` now filters terminal task changes (`staged.go:124-131`) and is called by `VerifyCI` (`ci.go:95-108`). A committed range owned by a completed task therefore loses its attribution candidates. | RDR-002; this is behavioral drift outside the staged-only fix. |

### Acceptance criteria

| Criterion | Verdict | Evidence / missing evidence |
| --- | --- | --- |
| AC-001–AC-004 | PASS | Keyring/environment/missing/unavailable tests, collector fixed-clock tests, independent truncation tests, and Node client behavior assertions pass. |
| AC-005 | PARTIAL | Focused command `go test ./internal/dashboard ./internal/cli ./internal/verify ./internal/changes` passed. The required post-audit full gate has not yet run. |
| AC-006 | FAIL | This audit has unresolved HIGH findings. |
| AC-G01 | PASS | `TestVerifyStagedUnstageReplacesProjectionWithEmpty` passes. |
| AC-G02 | PASS | `TestVerifyStagedReplacementContainsOnlyCurrentIndex` passes. |
| AC-G03 | PARTIAL | Active disjoint A+B, unregistered C, and active overlap are covered; completed sole-owner behavior fails. |
| AC-G04 | PASS | Existing clean/empty-index verification tests pass when no independent condition blocks. |
| AC-G05 | PARTIAL | Focused changes/verify/CLI tests pass; full gate is pending and correctness findings remain. |

## Findings

| ID | Severity | Requirement | File / symbol | Problem | Evidence | Confidence |
| --- | --- | --- | --- | --- | --- | --- |
| RDR-001 | 🟠 HIGH | USR-004, REQ-G03, REQ-G04, REQ-G05 | `internal/changes/attribution.go:103-113` `ReadActiveBaselines`; `internal/changes/automatic.go:142-170` `activeTaskChanges`; `internal/verify/verify.go:193-250` `stagedDrift` | Completed tasks are removed from both file- and symbol-ownership candidates. Normal work is completed before its commits are created, so the only correct declared owner disappears and the gate still blocks the workflow the user paused Vensift for. | Produced in an external overlay without editing the working tree: create `TSK-A`, capture `a.py`, create its change, mark it `COMPLETED`, stage an edit, call `VerifyStaged`. `TestReaderCompletedSoleOwnerMayCommit` failed with `Allow:false` and `UNREGISTERED_CHANGE` for `a.py`. Control `TestVerifyStagedDisjointTaskScopesDoNotCrossDrift` passed while owners remained active. The implementation's own completed-history test only covers completed B overlapping still-active A (`verify_test.go:587-617`); it omits the sole completed owner. | CONFIRMED |
| RDR-002 | 🟠 HIGH | USR-005, REQ-G07 | `internal/changes/staged.go:124-131` `AttributeChanges`; `internal/verify/ci.go:95-108` `VerifyCI`/`compose` | The terminal-task filter was inserted into the shared attribution path, so it changes committed-range CI behavior, not only staged drift. CI normally evaluates commits after their tasks are complete; their symbol candidates are now removed and become unregistered. | Deterministic data flow: `VerifyCI` calls shared `compose`, `compose` calls `AttributeChanges`, and `AttributeChanges` now calls `activeTaskChanges(ctx, "")`; lines 149-170 of `automatic.go` exclude every completed task. There is no CI regression test in `internal/verify`. A direct produced CI experiment could not reach attribution because the existing test fixture's runtime DB/cache files make `VerifyCI` correctly reject a dirty worktree; the exact required experiment is listed below. | HIGH |
| RDR-003 | 🟡 MEDIUM | REQ-G01 | `internal/changes/staged.go:46-52` `ReconcileStaged` / `sameFileProjection` | Exactness is inferred solely from file rows. If an upgraded runtime DB has the same current file rows but stale accumulated `change_symbols` from the old upsert implementation, the fast path returns without replacing them. That contradicts “every run” replacing the file/symbol projection exactly and can preserve the class of stale gate evidence this change is meant to repair. | `sameFileProjection` receives and compares only `[]FileChange`; no symbol rows are read or validated before the return. The replacement tests force different file sets/hashes and therefore do not exercise this compatibility case. A deterministic regression is: seed a `verify-staged` change with exact current file row plus one extra symbol row, rerun unchanged index, then assert the extra symbol is gone. Current control flow leaves it intact. | HIGH |
| RDR-004 | 🟡 MEDIUM | REQ-G02 | `internal/changes/staged.go:42-66` `ReconcileStaged`; `internal/changes/service.go:85-130` `fileSymbolRows` | Atomic replacement begins too late for a first failed run: `EnsureOpenChange` durably creates the fixed operation row before staged content/symbol construction, and each successful `IndexFile` mutates the shared semantic index before all files succeed. The existing projection-preservation test covers a previous successful projection, not first-run publication or partial index mutation. | Direct execution order in the cited code. Exact required test: on a fresh DB with no `verify-staged` row, inject failure on the second staged file, then assert no fixed-operation row/projection and no first-file staged index mutation were published. | HIGH |

## R3 — Scope compliance

- `UNDER_IMPLEMENTATION`: the post-completion commit workflow remains blocked (RDR-001), so the primary gate outcome is not delivered.
- `BEHAVIORAL_DRIFT`: terminal-task filtering was applied to shared `AttributeChanges`, altering CI committed-range attribution (RDR-002) although the requested defect was staged verification.
- `REQUIREMENT_MISINTERPRETATION`: “completed historical baselines must not compete with another owner” was implemented as “completed tasks can never own a commit.” Those are not equivalent; the latter removes the normal just-completed owner.
- No unrelated UI or architecture scope creep was found. Dashboard, credential metadata, operator docs, and focused tests remain within the frozen plan.

## R4 — Contract and documentation conformance

- `SPEC_IMPLEMENTATION_CONFLICT`: the staged contract says ownership selects the relevant task and permits disjoint task scopes; the implementation excludes a sole completed owner and therefore conflicts with REQ-G03/G05.
- `SPEC_IMPLEMENTATION_CONFLICT`: REQ-G07 requires CI-range parity, but shared attribution now excludes terminal owners for CI.
- `DOC_DRIFT`: the frozen diagnosis/work plan reads as a remediation of Vensift's commit gate, while the implementation still denies the common complete-then-commit sequence. Operator documentation does not warn that commits must be made before completion, and imposing such an ordering would itself conflict with the intended workflow.
- Dashboard implementation, README and `docs/usage-tr.md` agree on SSE-vs-agent semantics, startup freshness, keyring/unavailable behavior, and browser-extension diagnostics.
- The decision-count diagnosis is consistent with repository behavior/documentation: knowledge files are repository-owned and loaded from `.mindrail/knowledge`; they are not filtered by whether Git has committed them. No implementation change was required for that part, but the knowledge files should still be committed for clone portability as existing docs state.

## Evidence executed

- `TMPDIR=<workspace-on-disk> GOCACHE=<workspace-on-disk> go test ./internal/dashboard ./internal/cli ./internal/verify ./internal/changes` — PASS (`dashboard`, `cli`, `verify`, `changes`).
- `node internal/dashboard/testdata/client_behavior_test.cjs` — PASS, `client behavior assertions passed`.
- External-overlay produced test `TestReaderCompletedSoleOwnerMayCommit` — FAIL as expected for RDR-001, with `UNREGISTERED_CHANGE` and `Allow:false`.
- Control `TestVerifyStagedDisjointTaskScopesDoNotCrossDrift` — PASS.

The first focused-test attempt using `/tmp` failed only because `/tmp` was full during concurrent audit work; rerunning with `TMPDIR` and `GOCACHE` on the normal filesystem passed all four focused packages.

## Open items / exact experiments

1. Add the RDR-001 produced test permanently: same sole-owner fixture, one active control and one identical `COMPLETED` case; both must attribute to that task and allow at the scope layer.
2. Produce RDR-002 in a clean repository fixture whose runtime DB is outside the Git worktree: commit a file uniquely scoped to a task, mark the task completed, run `VerifyCI(base, head)`, and require no unregistered attribution denial.
3. Produce RDR-003 by seeding exact current file rows plus an extra legacy symbol row under the fixed `verify-staged` change; rerun unchanged staged verification and require exact symbol replacement.
4. Produce RDR-004 with no prior fixed-operation row, two staged files, and injected second-file symbol/index failure; require no fixed operation/projection and no partial first-file semantic-index publication.
5. After remediation, run the repository's required full validation gate once.
6. For REQ-008, run the tokenized dashboard in a clean browser profile through all views and at least two SSE frames while capturing the console; require zero warning/error entries sourced from Mindrail assets.

## Independent conclusion

`PARTIALLY_COMPLIANT`

The dashboard observability/JEV work is substantially conformant and its focused tests pass. The staged-gate change, however, does not restore the user's real workflow: completing the owning task makes its staged files unregistered, and the same filter regresses CI attribution. Those HIGH findings must be remediated before Vensift should resume or the change can be called final.
