# Breaker remediation delta — dashboard observability and staged gate

Date: 2026-09-28  
Role: independent falsification (`BREAKER`)  
Subject: remediation applied after `breaker.md`  
Isolation: source probes and temporary tests ran only in `/tmp/mindrail-breaker-delta`; Vensift and the implementation working tree were not modified.

## Result

`REMEDIATION_INCOMPLETE`.

All three original Breaker findings are closed by direct rerun, and the new staged retry/legacy projection behavior passed the exercised cases. One new HIGH gate defect was produced: an `ABANDONED` task is treated as a valid historical owner and can authorize a staged commit without an active or completed owner.

| Item | Delta verdict | Evidence |
| --- | --- | --- |
| BKR-001 — completed owner lost in CI | CLOSED | Fresh active and completed-owner repositories both returned exit `0`, with `0` denials. |
| BKR-002 — long title claims omitted tasks | CLOSED | One 1,025-rune title produced count `1`, rows `1`, `current_tasks_truncated=false`, and title `[TRUNCATED]`; real 23→20 list truncation still sets the flag. |
| BKR-003 — unsupported completed-owner test gap | CLOSED | Checked-in source and unsupported-file subtests pass for completed sole ownership; active-over-completed precedence also passes. |
| Staged retry after partial indexing | PASS | Same-set retry published two modified body deltas; narrower A+B→B retry published one B symbol and zero stale A symbols. |
| Legacy `verify-staged` projection | PASS | Legacy row remained isolated (`1` file); the `verify-staged-v2` row independently published `1` file and `1` symbol. |
| BKR-D01 — abandoned task authorizes commit | 🟠 HIGH / CONFIRMED | Sole `ABANDONED` owner + staged source edit returned `Allow:true`, `0` denials. |

## BKR-D01 — abandoned ownership bypasses the staged gate

Files/symbols:

- `internal/changes/attribution.go:129-153`, `PreferredTaskOwners`
- `internal/changes/staged.go:181-245`, `AttributeChanges`
- `internal/verify/verify.go:198-252`, `stagedDrift`

`PreferredTaskOwners` puts both `COMPLETED` and `ABANDONED` tasks into the same `historical` bucket. If no current owner exists, it returns that bucket unchanged. The completed exception is needed because normal completion precedes the commit hook. The abandoned exception is not equivalent: abandoned work is discarded/final, and the technical specification reserves “recover abandoned change ownership” for an administrative override rather than ordinary agent flow.

### Control/scenario

Both arms used the same repository and data shape:

1. commit baseline `pkg/a.py` with `return 1`;
2. capture `pkg/a.py` for the sole task and create its change row;
3. make the task terminal;
4. stage `return 2`;
5. run `VerifyStaged`.

Only the terminal state differs.

| Measurement | Control: `COMPLETED` | Scenario: `ABANDONED` |
| --- | ---: | ---: |
| Declared matching scopes | 1 | 1 |
| Non-terminal owners | 0 | 0 |
| Verdict | ALLOW | ALLOW |
| Denials | 0 | 0 |
| Expected | ALLOW | DENY / unregistered unless administratively recovered |

Produced command:

```text
TMPDIR=<isolated-build-temp> GOCACHE=/tmp/mindrail-go-cache \
go test -v ./internal/verify \
  -run TestBreakerDeltaAbandonedTaskCannotAuthorizeCommit -count=1
```

Produced output:

```text
abandoned sole owner authorized commit: {Allow:true Denials:[] Knowledge:[] FatalKnown:false}
```

Severity: 🟠 HIGH. This is a fail-open commit-gate authorization path: declaring a scope and then abandoning the task still authorizes later staged source under that scope.  
Confidence: CONFIRMED by execution.

Expected remediation: distinguish completed historical ownership from abandoned ownership. `COMPLETED` may be returned when no current owner exists; `ABANDONED` should not become an ordinary candidate. If abandoned recovery is required, route it through the explicit administrative recovery/override contract.

Class sweep:

- Parsed source is confirmed affected through symbol attribution.
- Unsupported files use the same `PreferredTaskOwners` selector in `stagedDrift` and therefore have the same shape.
- CI also uses `AttributeChanges`; whether CI should recognize abandoned history needs an explicit contract decision, but staged verification must not silently authorize it.
- Multiple abandoned scopes would deny as ambiguous, but one abandoned scope fails open; adding another abandoned scope must not be the security mechanism.

Backward compatibility: every existing DB that retains an abandoned task's baseline/change row can trigger this without migration. The task only needs to be the sole matching scope.

## Original finding reruns

### BKR-001 — completed CI ownership

The original control/scenario harness was recreated in the disposable copy. Each arm used a fresh initialized repository and identical Git/runtime data; only the owner state differed.

```text
TMPDIR=<isolated-build-temp> GOCACHE=/tmp/mindrail-go-cache \
go test -v ./internal/cli \
  -run TestBreakerDeltaCICompletedOwnerRemainsHistoricalOwner -count=1
```

Output:

```text
CI owner matrix: active=0/0 completed=0/0
PASS
```

Numbers are `exit-code/denial-count`. The prior `0/0` versus `1/1 UNREGISTERED_CHANGE` split is gone. `ListTaskChanges` plus `PreferredTaskOwners` retains a sole completed owner while allowing current owners to take precedence.

### BKR-002 — long task title versus list truncation

```text
TMPDIR=<isolated-build-temp> GOCACHE=/tmp/mindrail-go-cache \
go test -v ./internal/dashboard \
  -run 'TestCollector(LongTaskTitleDoesNotClaimAnotherTaskWasOmitted|SessionCurrentTaskProjectionDisclosesItsOwnBound)' \
  -count=1
```

Both tests passed.

| Input | Exact count | Returned rows | List-truncated flag | Title projection |
| --- | ---: | ---: | --- | --- |
| One 1,025-rune title | 1 | 1 | false | `[TRUNCATED]` |
| 23 ordinary claimed tasks | 23 | 20 | true | ordinary |

The collection boolean now represents only omitted rows; title truncation remains visibly bounded without triggering the false UI sentence.

### BKR-003 — completed historical overlap and unsupported reader

```text
TMPDIR=<isolated-build-temp> GOCACHE=/tmp/mindrail-go-cache \
go test -v ./internal/verify \
  -run 'TestVerifyStaged(CompletedHistoricalScopeDoesNotCompete|CompletedSoleOwnerMayCommit)' \
  -count=1
```

Passed cases:

- active owner plus completed overlap: ALLOW;
- sole completed owner, parsed source: ALLOW;
- sole completed owner, unsupported file: ALLOW.

The missing unsupported-reader regression coverage is now checked in.

## Staged retry experiments

### Same index after partial semantic indexing

Checked-in test sequence:

1. stage valid A and invalid B;
2. first verify indexes A, then fails parsing B;
3. repair B and retry the same staged set.

`TestVerifyStagedRetryAfterPartialIndexingKeepsEverySymbolDelta` passed and stored exactly two `modified` + `body_changed=1` symbol deltas.

### Narrower index after failure

Breaker-only sequence:

1. stage valid A and invalid B;
2. first verify fails after indexing A;
3. unstage A;
4. repair/stage B;
5. retry with B as the entire current index.

```text
narrow retry projection: symbols=1, stale_a=0
PASS
```

The retry published exactly the current B projection; the failed attempt's A symbol did not leak into the materialized staged view.

## Legacy projection experiment

A pre-remediation ownerless change row with operation ID `verify-staged` and one stale file was inserted before staging a valid owned source change. The new run used the versioned `verify-staged-v2` identity.

```text
legacy isolation: legacy_files=1 v2_files=1 v2_symbols=1
PASS
```

The legacy row neither short-circuited nor contaminated the v2 result. Keeping the old row is acceptable durable history; the verdict was derived from the v2 projection.

## Focused regression control

Before adversarial overlay tests, the unmodified remediation copy passed:

```text
TMPDIR=<isolated-build-temp> GOCACHE=/tmp/mindrail-go-cache \
go test ./internal/changes ./internal/verify ./internal/dashboard ./internal/cli ./internal/mcp
```

Results:

```text
ok  internal/changes
ok  internal/verify
ok  internal/dashboard
ok  internal/cli
ok  internal/mcp
```

This green control is why BKR-D01 matters: the abandoned branch is not covered by the checked-in remediation suite.

## Residual risks and unexecuted experiments

1. BKR-D01 remains open and blocks a clean Breaker verdict.
2. Crash injection between every SQLite statement was not performed. Atomic replacement and the two partial-index retry sequences passed, but process-kill durability still needs a kill/reopen harness to prove it empirically.
3. A real previous released binary was not used to manufacture the legacy DB. The exact legacy operation identity/data shape was inserted through current storage APIs; a binary-to-binary upgrade test would add packaging confidence.
4. The real OS keyring/browser threat setup from the original report was not repeated; no credential code changed in this remediation delta.
5. Full repository gate/race/E2E validation belongs to final orchestration and was not duplicated here; this report records the focused five-package control and adversarial cases only.

No other produced retry, legacy projection, dashboard truthfulness, or completed-owner failure remains from the exercised matrix.

## Final closure after BKR-D01 remediation

Final verdict: `CLOSED — NO OPEN PRODUCED BREAKER FINDINGS`.

The final remediation excludes `ABANDONED` tasks from ordinary preferred ownership while retaining `COMPLETED` as the complete-then-commit historical fallback (`PreferredTaskOwners`, `internal/changes/attribution.go:133-153`). The two required control arms were rerun from a fresh disposable scratch copy:

```text
TMPDIR=<isolated-build-temp> GOCACHE=/tmp/mindrail-go-cache \
go test -v ./internal/verify \
  -run 'TestVerifyStaged(AbandonedSoleOwnerDenies|CompletedSoleOwnerMayCommit)' \
  -count=1
```

Results:

| Arm | Source result | Unsupported-file sibling |
| --- | --- | --- |
| Sole `COMPLETED` owner | PASS / ALLOW | PASS / ALLOW |
| Sole `ABANDONED` owner | PASS / DENY | PASS / DENY |

Package result: `ok internal/verify` (`0.316s`). BKR-D01 is closed. The numbered residual experiments above remain unexecuted confidence work, but none is an open produced defect and none changes this focused Breaker verdict.
