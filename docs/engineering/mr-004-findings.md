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

Commit `abd1c3e`, and `a06cd42` after the gate. `make check` (19 `ok`, no
`FAIL`) and `make tidy-check` green after each; **815** top-level test
functions after the first, from 807, and **817** after the second. Owns
REQ-01 and the one code of REQ-07 it emits.

### What changed

| Where | What |
|---|---|
| `internal/storage/backoff.go` (new) | `Backoff` — the jittered ladder, 25 → 400 ms doubling, each step drawn from `[step/2, step]`; `DefaultBackoff`; `waitBusy`, the loop that retries an attempt on `SQLITE_BUSY` until a budget measured on an injectable clock is spent, with the last sleep cut to the deadline; `retrier`, the seam carrying ladder, sleep and clock |
| `internal/storage/db.go` | `openPollInterval` (20 ms, flat) is gone. `openWithinBusyBudget` delegates to `waitOpen`, which runs `openOnce` under `waitBusy` and names an exhaustion `busyOpenFailure`: `MINDRAIL_BUSY_RETRYABLE`, `KindUnavailable`, `metadata.path`, `condition = "locked"`, `waited_ms`, remedy "run the command again once the other Mindrail command has finished". Before, the exhaustion was `openFailure` — `RUNTIME_DB_UNAVAILABLE` with the remedy to check the Git common directory and run `mindrail init` |
| `internal/storage/tx.go` | `InTx` times `BeginTx`; a busy refusal is returned as `beginBusy{waited}` with the driver's error underneath, so `IsBusy` still answers; a committed transaction reports `Waited` and `Held`, a rolled-back one reports zero. No retry of `BEGIN` (D-74). *At `abd1c3e` the numbers went into a `*TxStats` planted in the context by `WithTxStats`; since `a06cd42` (the gate) `InTxMeasured` returns them by value and `InTx` discards them* |
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
returns an error after 64 sleeps, more than five times the twelve steps a
five-second budget holds at the cap, and M2 turns into the red line above in
0.01 s. Every mutation from M2 on was run as
`systemd-run --user --scope -p MemoryMax=1G timeout 300 go test -count=1 -timeout 240s -run <test> <pkg>`
with the file restored afterwards from a backup and compared with `cmp`; the
shell script that does this lives under `/tmp`, not in the tree, and the
rest of this milestone runs mutations the same way. *(The Reader of this task
refuted the first version of these two sentences: "five times twelve" is
sixty, not sixty-four, and "the runner script" named a file the repository
does not contain.)*

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
  names two sites, `BEGIN` and open, and both publish it. Others exist and
  publish no number, because none was measured
  (`TestABusyFromAStatementCarriesNoInventedWait`): a statement inside the
  transaction, the WAL checkpoint at shutdown — and, as the Breaker showed, a
  whole command: `session open` inserts with a bare `ExecContext` rather than
  through `InTx` (`internal/coordination/store.go`, `OpenSession`), so under
  the same held lock it exits 4 with the same code and no `waited_ms` where
  `task open` and `checkpoint write` print `5008`. That path predates MR-004
  and is closed by TASK-03, the first task that touches the store; a guessed
  budget in its place would be the kind of sentence this record exists to keep
  out. *(The first version of this bullet said "a third exists"; the gate
  counted a fourth.)*
- **`schema_ledger_test.go` was edited.** It is `internal/migration`'s
  contention test and asserted the old code; it now asserts the new one on
  the same exit class. Nothing else in that package changed.

### The gate

Two agents over `abd1c3e` and this record at `d7188ef`: a Reader (Sonnet)
checking the record sentence by sentence, a Breaker (Fable) attacking the
change from a built binary against scratch repositories under `/tmp`, with
throwaway tests in its own worktree. Both told to refute when uncertain and to
report only what they demonstrated. Fixes: `a06cd42`; `make check` (19 `ok`)
and `make tidy-check` green after it; **817** top-level test functions, from
815.

**The Reader** checked 30 claims: 19 confirmed, 2 false, 9 unconfirmed. The
two false were this record's own sentences — "five times the twelve steps"
for sixty-four, and "the runner script" for a file that lives under `/tmp`
and not in the tree — and are corrected in place above, marked. The nine
unconfirmed are the whole-suite and process claims it was not allowed to run
(`make check`, `make tidy-check`, the OOM kills, five mutations it did not
re-run); the three it re-ran (M1, M3, M4) matched the recorded red lines. Its
contract verdict: AC-01.1, AC-01.3, AC-01.4 met; AC-01.2 and AC-01.5 met with
the departures recorded above; D-74 and D-75 met for what TASK-01 owns.

**The Breaker** produced five findings and could not break the rest:

| Grade | What | Origin | Done |
|---|---|---|---|
| MEDIUM | Two goroutines running `InTx` under one `WithTxStats` context race on the shared `*TxStats` (`tx.go:101`, `-race`: `WARNING: DATA RACE … Write at … by goroutine 11 … Previous write … by goroutine 13`). No production caller shared one; the coordination store was about to | `abd1c3e` | The seam is reshaped rather than documented: `InTxMeasured` returns the stats **by value** beside the outcome and `InTx` discards them, so there is nothing to share (`a06cd42`; D-75 and AC-01.4 amended in place). `TestInTxMeasuredSharesNothingBetweenCallers` runs four goroutines × twenty transactions under `-race`; its falsifier is `abd1c3e`'s own seam, on which the Breaker's equivalent test printed the race above |
| MEDIUM | Same held lock, two answers: `session open` writes with a bare `ExecContext`, waits the whole budget, exits 4 with the code and **no** `waited_ms` (`elapsed_ms=5062`, `"metadata":{"condition":"locked","subject_id":"SES-…"}`), where `checkpoint write` prints `"waited_ms":"5007"` | predates (MR-003's `OpenSession`) | Recorded above and carried to **TASK-03**, the first task that edits the store: every writer goes through `InTx`, and TASK-05 needs that anyway for the operation record |
| LOW | The busy open failure's `cause` still named the retired code — `"cause":"runtime database is locked by another process: RUNTIME_DB_UNAVAILABLE: the runtime database could not be opened"` on `status --json` under a rollback-mode EXCLUSIVE lock — because `classifyOpenError`'s busy branch wrapped the driver's error in `openFailure` and `busyOpenFailure` then wrapped that | `abd1c3e` | The busy branch hands the driver's error through bare; `waitOpen` is its one caller and names the exhaustion once (`a06cd42`). `TestABusyOpenIsNamedOnce`; mutation M7 below |
| LOW | `init` under a foreign lock spends two open budgets — the read-only probe bootstrap, then the write-mode one — `init_elapsed_ms=10031`, two `startup stopped … MINDRAIL_BUSY_RETRYABLE` lines five seconds apart. Outcome correct, exit 4 | predates (MR-001's two-phase `init`) | Recorded, not fixed: it is `init`'s shape, not this task's; the backlog item is to let the write-mode start reuse the probe's verdict on a locked database |
| LOW | `TxStats.Waited` is measured from the call, so it includes database/sql handing out a pooled connection: pool of one, 50 ms busy budget, a holder sleeping 300 ms inside `InTx` → the contender's `stats={Waited:300.19ms Held:97µs}` with no busy refusal | `abd1c3e` | Documented on `TxStats` and in D-75 as amended: it is the wait an interactive writer experiences; AC-10.6's bound is on `Held` |

Attacked and not broken, in one line each. **D-74's falsifier failed**: with a
200 ms budget, a second handle holding `BEGIN IMMEDIATE` refused `InTx` after
`201.9 ms`, the same pool after `200.9 ms`, a `TRUNCATE` checkpoint holding the
WAL write lock after `202.2 ms` — the only immediate busy on this driver is a
deferred read transaction upgraded after a foreign commit (`11.3 µs`,
`database is locked (517)`), unreachable through `_txlock=immediate`. WAL
recovery after `kill -9` of a writer with an 80 MB log: two `task open` started
together both exited 0 in 147 and 122 ms. Eight concurrent `init` in a fresh
repository all exited 0. An 8 s foreign lock: `task open` exit 4,
`waited_ms 5008`, then exit 0 in 20 ms after release; `checkpoint write` the
same; a 2 s hold, exit 0 after 1551 ms; `status` and `doctor` exit 0 in 20 ms
with the write lock held; `init` on an initialised repository under the lock
exits 4 with the code and "could not register this worktree". A rollback-mode
EXCLUSIVE lock on a fresh file: `status` and `doctor` exit 4 with the code,
`condition = locked`, `waited_ms ≈ 5000` and the "run the command again" remedy;
`init` exits 0 after release. Every busy object on the wire exited 4. The
ladder's edge values — zero `Backoff`, `Base > Cap`, `Cap = 0`, negatives,
`Rand` of 1.0 and −1.0 — return a step at or below the cap or at or below zero,
so `waitBusy` stops after one attempt and never spins; `Step` is linear in the
attempt number, which a five-second budget cannot push past about thirty.
`Held` and `Waited` were never zero or negative on a committed transaction;
zero on a rolled-back one. `TestInTxTakesTheWriteLockAtBegin` ran in 0.06 s
and `TestInTxDoesNotRetryBegin` in 0.05 s.

Not demonstrated, worth a later look: `SQLITE_LOCKED` (6) is in `isBusyError`,
so a same-connection table lock would be reported as retryable contention; one
attempt to provoke it on this driver returned nil. Predates.

| # | Mutation (gate fixes) | Red |
|---|---|---|
| M7 | `classifyOpenError`'s busy branch wraps in `openFailure` again | `TestABusyOpenIsNamedOnce`: `classifyOpenError(busy) = RUNTIME_DB_UNAVAILABLE: …, want no diagnosis of its own`, and `payload.Cause = "… RUNTIME_DB_UNAVAILABLE: the runtime database could not be opened", names the code this condition no longer carries` |
| M8 | `InTxMeasured` returns the numbers on the rollback path | `TestInTxReportsItsWaitAndHoldWhenAsked`: `stats = {Waited:5.781µs Held:20.078126ms} for a rolled-back transaction, want zero` |

**Carried forward from this gate:** `session open` through `InTx` (TASK-03);
`init`'s second open budget under a lock (backlog); `SQLITE_LOCKED` in
`isBusyError` (backlog).
