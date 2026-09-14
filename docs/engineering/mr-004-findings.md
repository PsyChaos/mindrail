# MR-004 — Findings and per-task record

- **Contract:** [mr-004-requirements.md](mr-004-requirements.md) (frozen at
  `e4bad83`) and [mr-004-design.md](mr-004-design.md).
- **Baseline:** `6a78f8d`, 807 top-level test functions, `make verify` and
  `make tidy-check` green (requirements §0).

This milestone is run one task at a time. Each task below ends with the
mutations that turn its new tests red — run before they were recorded — a
commit, and a gate: a Reader that checks this record against the code and
git sentence by sentence, refuting when uncertain, and a Breaker that attacks
the production change and reports only what it demonstrated. The gate's
findings are recorded under the task, with what was done about each, before
the next task is begun. That is the process change MR-003's second round was
run to justify (requirements, preamble).

Rules the record keeps: a proposed finding is not a confirmed one and a
confirmed one is not a distinct one; every sentence here is something that was
run, not something believed; a number is given where a claim needs one and
nowhere else.

---

## 1. TASK-01 — the bounded wait, named and measured

Commit `abd1c3e`. `make check` (19 `ok`, no `FAIL`) and `make tidy-check`
green; **815** top-level test functions, from 807. Owns REQ-01 and the one
code of REQ-07 it emits.

### What changed

| Where | What |
|---|---|
| `internal/storage/backoff.go` (new) | `Backoff` — the jittered ladder, 25 → 400 ms doubling, each step drawn from `[step/2, step]`; `DefaultBackoff`; `waitBusy`, the loop that retries an attempt on `SQLITE_BUSY` until a budget measured on an injectable clock is spent, with the last sleep cut to the deadline; `retrier`, the seam carrying ladder, sleep and clock |
| `internal/storage/db.go` | `openPollInterval` (20 ms, flat) is gone. `openWithinBusyBudget` delegates to `waitOpen`, which runs `openOnce` under `waitBusy` and names an exhaustion `busyOpenFailure`: `MINDRAIL_BUSY_RETRYABLE`, `KindUnavailable`, `metadata.path`, `condition = "locked"`, `waited_ms`, remedy "run the command again once the other Mindrail command has finished". Before, the exhaustion was `openFailure` — `RUNTIME_DB_UNAVAILABLE` with the remedy to check the Git common directory and run `mindrail init` |
| `internal/storage/tx.go` | `InTx` times `BeginTx`; a busy refusal is returned as `beginBusy{waited}` with the driver's error underneath, so `IsBusy` still answers; a committed transaction writes `Waited` and `Held` into the `*TxStats` a caller planted with `WithTxStats`; a rolled-back one writes nothing. No retry of `BEGIN` (D-74) |
| `internal/storage/classify.go` | `busyFailure` publishes `MINDRAIL_BUSY_RETRYABLE` instead of `RUNTIME_DB_UNAVAILABLE`, keeps `condition = "locked"` and its sentences, and adds `waited_ms` when the cause is a `beginBusy` |
| `internal/app/code.go` | `CodeBusyRetryable = "MINDRAIL_BUSY_RETRYABLE"`, registered |
| `internal/doctor/check.go` | `kindForCode` places the new code in the unavailable class beside `RUNTIME_DB_UNAVAILABLE`, so a `status` halted by a locked database still exits 4 |
| Tests | `backoff_internal_test.go` (five, new); `tx_test.go` +2 (`TestInTxReportsItsWaitAndHoldWhenAsked`, `TestInTxDoesNotRetryBegin`); `classify_test.go` +1 (`TestABusyFromAStatementCarriesNoInventedWait`) and `TestWriteFailureRatesContentionUnavailable` now asserts the new code, `condition` and a `waited_ms` no smaller than the contender's 50 ms budget; `migration/schema_ledger_test.go`'s contention test asserts the new code on the same exit class; `cli/envelope_test.go`'s exit table gains the row (`TestExitClassTableCoversEveryRegisteredCode` fails without it) |

Every existing `RUNTIME_DB_UNAVAILABLE` row of the CLI contract matrix is
unchanged: the code moved for the locked condition only (AC-01.3, D-74).

### The mutations, and what each turned red

Each was applied to the committed code with `sed` or `perl`, the named tests
run under `systemd-run --user --scope -p MemoryMax=1G` and `timeout`, and the
file restored from a byte-compared backup. The red line is quoted as the test
printed it.

| # | Mutation | Red |
|---|---|---|
| M1 | `Backoff.Step` never doubles (`step = step`) | `TestTheLadderStepsByTheSpecsExample`: `Step(1) with Rand=0 = 12.5ms, want the lower bound 25ms`, and every later step the same |
| M2 | `waitBusy` takes the full step instead of `min(step, time left)` | `TestWaitBusySleepsByTheLadderAndStopsAtTheBudget`: `sleeps = [12.5ms 25ms 50ms 100ms 200ms …×60], want [12.5ms 25ms 50ms 100ms 12.5ms]`. See the fixture note below: the first run of this mutation did not print a red line |
| M3 | `busyFailure` back to `CodeRuntimeDBUnavailable` | `TestWriteFailureRatesContentionUnavailable` and `TestABusyFromAStatementCarriesNoInventedWait`: `payload.Code = "RUNTIME_DB_UNAVAILABLE", want "MINDRAIL_BUSY_RETRYABLE"` |
| M3b | `waitOpen` returns `openFailure` instead of `busyOpenFailure` | `TestAnOpenRefusedForTheWholeBudgetIsRetryable`, four lines: `want errors.Is(err, ErrBusy)`, the code, `metadata.condition = "", want "locked"`, `metadata.waited_ms = "", want "200"` |
| M4 | `InTx` returns a plain `fmt.Errorf` for a busy `BEGIN`, dropping the wait | `TestWriteFailureRatesContentionUnavailable`: `metadata.waited_ms = "", want the milliseconds BEGIN waited` |
| M5 | `stats.Held +=` instead of `=` | `TestInTxReportsItsWaitAndHoldWhenAsked`: `stats.Held = 20.07ms after an empty transaction, want it replaced rather than accumulated` |
| M5b | stats written on the rollback path too | the same test: `stats = {Waited:6µs Held:20.1ms} after a rolled-back transaction, want {…} untouched` |
| M6 | `BeginTx` wrapped in `waitBusy` with the 5 s default budget — the second ladder D-74 refuses | `TestInTxDoesNotRetryBegin`: `InTx = <nil> after 312.9ms, want SQLITE_BUSY: BEGIN was tried again until the lock was released`. `TestInTxTakesTheWriteLockAtBegin` stayed green and took 5.3 s instead of 0.05 s, which is the budget multiplication the decision describes |

**A fixture rule this task learned.** M2's first run had no cap on the virtual
clock: the mutated loop had no exit left — the attempt always refused, the
context was never cancelled, and the fake sleep passed time instantly while
recording each sleep — so it spun until the kernel's out-of-memory killer ended
the test binary and the desktop session running it (exit 137, twice). A fake
that passes time in zero time must refuse eventually: `virtualClock.sleep` now
returns an error after 64 sleeps, five times the twelve steps a five-second
budget holds at the cap, and M2 turns into the red line above in 0.01 s. Every
mutation from M2 on was run under a 1 GB memory scope and a timeout, and the
runner script is the way mutations are run for the rest of this milestone.

### Where this task departed from the freeze, and why

- **AC-01.2 is proved on a virtual clock, not a wall clock.** The criterion
  asks for at least three attempts with rising gaps and none after the budget
  plus one jittered step. The test observes six attempts under a 200 ms budget
  with gaps 12.5, 25, 50, 100 and 12.5 ms — the last one cut to the deadline —
  and the final attempt exactly at the budget, not after it. The stronger
  statement is what the code does, and a wall clock could not have asserted
  the gaps.
- **AC-01.5 is decided by outcome rather than by the stopwatch it names.** The
  criterion says a 50 ms contender "still fails within 250 ms". A stopwatch
  bound is either loose enough to miss a two-attempt loop (100 ms plus jitter)
  or tight enough to flake under `-race` and a parallel `make check`.
  `TestInTxDoesNotRetryBegin` holds the lock for six budgets and releases it: a
  single attempt is refused at one budget; any loop that tried again meets the
  released lock and succeeds, which is what M6 shows. `TestInTxTakesTheWriteLockAtBegin`
  keeps its 50 ms contender and still runs in 0.05 s.
- **`waited_ms` is absent on a busy that `BEGIN` did not measure.** AC-01.3
  names two sites, `BEGIN` and open, and both publish it. A third exists — a
  statement inside the transaction, or the WAL checkpoint at shutdown, refused
  as busy — and publishes no number, because none was measured
  (`TestABusyFromAStatementCarriesNoInventedWait`). A guessed budget in its
  place would be the kind of sentence this record exists to keep out.
- **`schema_ledger_test.go` was edited.** It is `internal/migration`'s
  contention test and asserted the old code; it now asserts the new one on
  the same exit class. Nothing else in that package changed.

### The gate

*Filled after the Reader/Breaker pair has run.*
