# Breaker report — dashboard observability and staged gate

Date: 2026-09-28  
Role: independent falsification (`BREAKER`)  
Changeset base: `31220dc4c89083ea0bc1b0ee41a48de3bdfe879e` plus the audit-start working tree  
Isolation: all mutations and destructive probes ran in `/tmp/mindrail-breaker-3Zpao8`; Vensift and the implementation working tree were not used as test subjects.

## Result

`BROKEN` — two behavioral failures were produced. One is a CI-range regression for already-completed task ownership; the other makes the dashboard state that task rows were omitted when none were omitted. A third finding is a mutation survivor demonstrating a missing regression test for the unsupported-file branch of completed-task exclusion.

| ID | Severity | Confidence | Contract | Produced failure |
| --- | --- | --- | --- | --- |
| BKR-001 | 🟠 HIGH | CONFIRMED | REQ-G07 | A committed range owned by a task verifies green while the task is active, then denies `UNREGISTERED_CHANGE` solely because the same owner row is marked `COMPLETED`. |
| BKR-002 | 🟡 MEDIUM | CONFIRMED | REQ-004, REQ-005, REQ-009 / AC-003, AC-004 | One displayed task with a long title sets `current_tasks_truncated=true`; the UI says “More claimed tasks omitted; exact total 1” although count and rendered rows are both 1. |
| BKR-003 | 🟡 MEDIUM | CONFIRMED | REQ-G03, AC-G03 | Removing the terminal-task predicate from `ReadActiveBaselines` leaves the repository suite green. A discriminating unsupported-file test kills the mutation, proving the guard matters and the checked-in test exercises only the symbol reader. |

## BKR-001 — terminal-task filtering breaks historical CI attribution

Files/symbols:

- `internal/changes/staged.go:124-150`, `Service.AttributeChanges`
- `internal/changes/automatic.go:142-171`, `Store.activeTaskChanges`

The staged fix changed the shared `AttributeChanges` path from all task changes to `activeTaskChanges`. That shared function is also used by `VerifyCI`. A CI range describes committed historical work; completing the task before CI runs must not erase its ownership.

### Attempt

Two fresh initialized repositories received identical Git history and runtime rows:

1. commit `pkg/pyproject.toml` and `pkg/a.py` (`return 1`), record that commit as `base`;
2. open a session and task;
3. capture a baseline for the absolute `pkg/a.py` path and ensure the task change row exists;
4. commit `return 2`;
5. run `verify --ci --base <base> --json`.

The sole difference was the task state immediately before the source commit: active control versus `COMPLETED` scenario.

Command:

```text
TMPDIR=<isolated-build-temp> GOCACHE=/tmp/mindrail-go-cache \
go test ./internal/cli -run TestBreakerCICompletedOwnerRemainsHistoricalOwner -count=1
```

Result: failed on the completed arm with `UNREGISTERED_CHANGE` for `a.py`.

| Measurement | Control: active owner | Scenario: completed owner |
| --- | ---: | ---: |
| Git files changed in range | 1 | 1 |
| Matching recorded baselines | 1 | 1 |
| Matching task change rows | 1 | 1 |
| Exit code | 0 | 1 |
| Denials | 0 | 1 |
| `UNREGISTERED_CHANGE` | 0 | 1 |

Expected: both arms attribute the already-committed range to the same durable owner and return the same verdict. Actual: task lifecycle state changes the meaning of an otherwise identical historical range.

Class sweep: the same filter is used by staged and CI composition through `AttributeChanges`. Staged commits plausibly require active ownership, but CI requires durable historical ownership. `AttributeTask` passes its own task ID to retain that task, while global `AttributeChanges` passes `""`; therefore the regression shape is concentrated in global verification callers, with CI confirmed affected.

Backward compatibility: any existing runtime DB with a completed/abandoned task, its baseline/change rows, and a CI run whose range contains that task's committed symbol now denies after this change. No schema version distinction is required to trigger it.

## BKR-002 — field truncation is reported as omitted task rows

Files/symbols:

- `internal/dashboard/collector.go:534-576`, `Collector.sessionActivity`
- `internal/dashboard/assets/app.js:134-137`, `renderAgents`

The collector uses one boolean for two different facts. Line 536 means “task rows exceed `maxSessionTasks`”; line 576 also sets it when a title field is truncated. The frontend gives that boolean only the first meaning and explicitly claims more tasks were omitted.

### Attempt

One session claimed exactly one non-terminal task. Control title length was within `maxTextRunes`; scenario title length was `maxTextRunes+1` (1,025 runes). The scratch assertion required `current_tasks_truncated=false` whenever `current_task_count == len(current_tasks) == 1`.

Command:

```text
TMPDIR=<isolated-build-temp> GOCACHE=/tmp/mindrail-go-cache \
go test ./internal/dashboard -run TestBreakerLongTitleDoesNotClaimTasksWereOmitted -count=1
```

Produced output:

```text
current_tasks_truncated=true with count=1 rows=1: UI will falsely say more tasks were omitted
```

| Measurement | Control: bounded title | Scenario: 1,025-rune title |
| --- | ---: | ---: |
| Exact current-task count | 1 | 1 |
| Task rows returned | 1 | 1 |
| Rows actually omitted | 0 | 0 |
| `current_tasks_truncated` | false | true |
| UI omission claim | no | yes |

Expected: disclose title-field truncation without claiming row omission. Actual: “More claimed tasks omitted; exact total 1.”

Class sweep: top-level task/session/lease truncation already uses a shared `Truncated` map that can describe collection/field truncation generically. The same misleading sentence occurs only once, in `renderAgents`; no sibling UI sentence was found. The underlying conflation is limited to `currentTasksTruncated`.

Backward compatibility: any existing task title longer than 1,024 runes triggers the false statement on the updated dashboard; no migration or new data shape is needed.

## BKR-003 — terminal-baseline guard survives the checked-in suite

File/symbol: `internal/changes/attribution.go:103-124`, `Store.ReadActiveBaselines`.

### Mutation and discriminating input

Mutation: replace

```sql
WHERE t.state NOT IN ('COMPLETED','ABANDONED')
```

with `WHERE 1=1`.

The checked-in targeted test `TestVerifyStagedCompletedHistoricalScopeDoesNotCompete` remained green because its Python file is represented by a symbol; the file-level reader skips represented paths, and symbol attribution independently filters terminal tasks. The mutation was therefore masked.

A discriminating input used unsupported `README.md`, declared by one active and one completed task. It reaches the file-level reader only.

| Measurement | Unmutated control | Mutated scenario |
| --- | ---: | ---: |
| Active owners | 1 | 1 |
| Completed historical scopes | 1 | 1 |
| Denials | 0 | 1 |
| Verdict | ALLOW | DENY (`RECONCILE_AMBIGUOUS`) |

The scratch discriminating test passed against the unmutated code and failed against the mutation. This is a test-coverage finding, not a claim that the current guard is functionally wrong.

Class sweep: two ownership readers exist: symbol attribution (`activeTaskChanges`) and unsupported/file-level attribution (`ReadActiveBaselines`). Existing completion coverage exercises the former only. Add the same terminal-owner matrix for unsupported files and keep CI's historical semantics separate.

Backward compatibility: current behavior remains correct while the guard exists; the concrete risk is a future refactor deleting the predicate with no checked-in red signal.

## Six-move ledger

### B1 — Mutation: delete each guard

Produced:

- Replacing staged symbol deletion with a no-op turned the A→B projection from `a=0, b>0` into `a=1, b=1`; `TestVerifyStagedReplacementContainsOnlyCurrentIndex` went red. The stale-symbol cleanup is protected.
- Removing the terminal filter from `ReadActiveBaselines` left the checked-in test green, then failed the discriminating unsupported-file test. Reported as BKR-003.

Not exhaustively mutated before finalization: package-local input validators in `replaceChangeProjection`, every SQL/error-propagation branch, `sameFileProjection` comparisons, JEV normalization branches, every session-activity timestamp/lease predicate, and frontend fallback booleans. Completing this move requires a table-driven package-local mutation harness (or equivalent hand mutations) that invokes each internal guard with a discriminating input. These are open experiments, not claimed passes.

### B2 — A/B of deleted behavior

The removed append-only staged behavior was reproduced by deleting the new `DELETE FROM change_symbols` step. On the same A→B sequence, current behavior stored only B (`a=0`, `b>0`); the old-shaped arm retained both (`a=1`, `b=1`). The current replacement behavior is the requested correction; no unrequested output difference was found in this A/B.

Exact base-commit execution was not run because `/tmp` was full and the parent requested immediate finalization. To complete that open experiment, create a second disposable base worktree, copy only the new black-box A→empty/A→B tests into it, and run them against `31220dc` with the same fixture.

### B3 — Exercise the written threat model

Attempted the credential boundary through the focused package suite. The tested arms cover non-blank environment precedence, whitespace-only environment fallback, keyring available/missing/unavailable, canceled context, metadata-only transport, redaction, loopback/token/Host/Origin/CSP and stream limits. All focused packages passed in the isolated copy:

```text
go test ./internal/changes ./internal/verify ./internal/dashboard ./internal/cli ./internal/mcp
```

Results: `changes`, `verify`, `dashboard`, `cli`, and `mcp` all `ok`. No credential value or unrelated environment value was produced in dashboard JSON/output by those tests.

Not attempted: a real hostile browser and OS keyring backend. Producing that requires an isolated Secret Service/keyring plus browser process, a sentinel credential, and network capture of all loopback responses/logs.

### B4 — Upgrade path with old runtime rows

Produced BKR-001 using durable task/baseline/change rows followed by a terminal lifecycle state, the relevant old-install/runtime shape. Also exercised the reused `verify-staged` operation with A→empty and A→B; checked-in tests proved stale file/symbol rows converge.

Not attempted: running an older released binary to create the DB and then opening it with this binary. That requires an archived old binary and schema fixture. No schema migration is introduced by this changeset.

### B5 — Weakest satisfying inputs

- `len(title) > maxTextRunes`: the minimum satisfying title, 1,025 runes, produced BKR-002.
- non-blank JEV environment predicate: whitespace-only is the minimum deceptive value; the focused dashboard CLI test rejects it and falls back to the store.
- active lease predicate: an expiry at/before `now` is excluded by focused collector tests; no failure produced.
- terminal filter: one active plus one completed owner on one unsupported file produced the mutation gap BKR-003.

Not attempted: every invalid row kind/provenance/key minimum for the unexported projection publisher; this needs package-local direct tests added to the scratch harness.

### B6 — Dual readers

Readers compared:

- parsable symbol ownership versus unsupported file ownership: both agree in unmutated behavior, but only the symbol branch is protected by the checked-in completed-task test (BKR-003);
- staged attribution versus CI attribution: the shared active-only reader gives CI the wrong historical answer (BKR-001);
- CLI JEV resolution versus dashboard JEV normalization: environment/keyring/none/unavailable focused tests agreed; no split produced;
- collector exact counts versus frontend truncation interpretation: disagreement produced BKR-002.

Unicode normalization/case variants were not relevant to exact canonical file paths in this increment and were not executed. A remaining dual-reader experiment would feed NFC/NFD and Turkish `İ/ı` labels through redaction plus rendering; it would test display consistency, not ownership.

## Mandatory off-spec checks

### Cost

One populated snapshot executes 16 SQL queries. The server permits 8 concurrent streams and ticks every 1.5 seconds:

```text
16 queries × 8 streams ÷ 1.5 seconds = 85.33 queries/second
```

At the 256 KiB payload cap:

```text
256 KiB × 8 ÷ 1.5 seconds = 1,365.33 KiB/second
```

The session projection is bounded to 120 sessions and 20 displayed tasks per session (maximum 2,400 task rows before serialization, with exact counts computed independently). This is a loopback-only operator surface and no unacceptable worst-case failure was produced. The architecture recomputes per stream rather than broadcasting one frame; record this as measured headroom, not a finding.

### Mechanical rule = automated test

- Exact staged replacement: automated and mutation-killed.
- Completed tasks must not compete with active staged ownership: automated only for parsable/symbol paths; unsupported-file branch is missing (BKR-003).
- CI-range parity: existing tests do not cover a terminal historical owner; BKR-001 demonstrates the rule regressed.
- Dashboard must not imply liveness: static/client tests cover wording and status labels, but did not cover the field/list truncation semantic seam (BKR-002).

### Class sweep

Recorded under every finding. No additional occurrences were promoted without a produced failure.

### Backward compatibility

- Old runtime rows for completed tasks break CI attribution (BKR-001).
- Existing long task titles produce false dashboard text without migration (BKR-002).
- Reused staged operation rows converge correctly in A→empty and A→B tests.
- Existing JEV env/keyring/none configurations passed focused compatibility tests.

## Controls and environment

Baseline command in the disposable copy passed:

```text
TMPDIR=<isolated-build-temp> GOCACHE=/tmp/mindrail-go-cache \
go test ./internal/changes ./internal/verify ./internal/dashboard ./internal/cli ./internal/mcp
```

Package results: all five `ok`. The external `TMPDIR` was required only because the shared `/tmp` tmpfs was full; all source mutations remained confined to `/tmp/mindrail-breaker-3Zpao8`.

## Open experiments

1. Complete B1 over every newly added error/input guard using package-local direct mutation tests; exact targets are listed in B1.
2. Run the exact base binary/worktree A/B described in B2.
3. Exercise a real isolated Secret Service/browser threat setup with sentinel secrets and response/log capture.
4. Create an old released-binary runtime DB, upgrade in place, and repeat staged/CI verification.

These were not executed and are not represented as passing.
