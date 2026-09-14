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

---

## 2. TASK-02 — migration 000003, and a ledger that can read `ADD COLUMN`

Commit `aa8d1a2`, and `fccc94f` after the gate. `make check` (19 `ok`, no
`FAIL`) and `make tidy-check` green after each; **822** top-level test
functions after the first, from 817, and **825** after the second. Owns
REQ-02.

### What changed

| Where | What |
|---|---|
| `migrations/000003_lease_idempotency.sql` (new) | Design §5 as written: `leases` and `operations`, both `STRICT`; `idx_leases_active` — unique on `(project_id, target_kind, target_key) WHERE released_at IS NULL`; `idx_leases_holder`; `ALTER TABLE tasks ADD COLUMN revision INTEGER NOT NULL DEFAULT 1`. Its comments carry the decisions the columns rest on (D-64, D-65, D-71, D-72, D-73). `000001` and `000002` are untouched |
| `migrations/shipped_test.go` | The third file's sha256 joins the pin, with the note that until MR-004 ships a change to that row is a change to a file no repository has applied, and is recorded here when it happens |
| `internal/migration/columns.go` | `alterTablePattern` reads the added column's name when the form is `ADD [COLUMN]`; `alterations` sorts every `ALTER TABLE` into `added` (table → columns, lower-cased like the `CREATE` names) and `forgotten` (every other form). `alteredTables` is gone |
| `internal/migration/load.go` | `Migration.Added`; `Altered` now means "altered in a form the checker cannot follow" |
| `internal/migration/migrator.go` | `declaredColumns` extends a tracked table by its added columns before it forgets the tables other forms touched; a table an earlier form made it forget, or one with no `CREATE` to extend, gets nothing invented |
| `internal/coordination/store.go` | `TableSchemaVersion` is 3, so `coordinationScope`'s gate refuses a ledger at 2 the way it refused a ledger at 1 (AC-02.5) |
| `internal/cli/testdata/*.golden` | Four lines, exactly the ones D-73 named: `Schema version: 2 → 3` in `status_human`, and `version 2 → 3`, `applied: 2 → 3`, `current_version: 2 → 3` in `doctor_human`. `write_schema_version` and `readable_schema_versions` stay at 1 on the lines beside them |
| `internal/cli/upgrade_test.go` | `TestCoordinationCommandsSendASchemaBehindDatabaseToInit` runs its two commands against two downgrades — to schema 1 (the MR-002 binary's database) and to schema 2 (the MR-003 binary's: every table present, no leases, no operations, no `revision`) — and asserts `MIGRATION_FAILED`, the `mindrail init` remedy, no raw `no such table` or `no such column` in `why`, and that `init` then clears it. `downgradeToSchemaOne` is now built on `downgradeToSchemaTwo` |
| `internal/cli/agreement_hostilefs_linux_test.go` | The ready-band sweep's wide end moves from 256 KiB to 512, with the reason measured (below); the sweep gains the points 224, 240, 272, 288 around the two edges |
| Tests | `internal/migration`: `TestStatusExpectsAColumnAMigrationAdded` and `TestStatusStillForgetsATableAnotherAlterTouched` (both arms of D-73, both spellings of `ADD COLUMN`), `TestATaskWrittenBeforeTheRevisionColumnStartsAtOne` (AC-02.3 on a row written at schema 2), `TestLoadReadsTheColumnsOfTheEmbeddedSchema` pins the two new tables' columns and that the embedded set's only `ALTER` is read as `{tasks: [revision]}` and forgets nothing, `tablesPerMilestone` gains version 3. `internal/coordination/lease_schema_test.go`: `TestTheDatabaseRefusesASecondActiveLeaseOnOneTarget` (AC-02.1 by direct `INSERT`, the released row not counting, the same path in a second project allowed) and `TestMigrationThreeLeavesTheObjectsTheDesignNames` (the four objects in `sqlite_master`, both tables `STRICT`, `revision` in `pragma_table_xinfo`, a task opened through the store at revision 1) |

The existing `TestADatabaseAtTheOlderSchemaTakesOnlyTheNewMigration` is written
over the embedded set's newest migration, so it now proves AC-02.3's first
sentence — a database at schema 2 applies exactly migration 3 and reports 3 —
without being edited.

### A number the freeze did not have

The hostile-filesystem test's wide end — "at a quarter-megabyte free everything
has to work", MR-001's over-fire guard — went red: with three migrations
`init` exited 4 (`RUNTIME_PATH_UNWRITABLE`) at 256 KiB free. Sweeping in
16 KiB steps on the test's own tmpfs, with the migration file temporarily
removed and then restored: with two migrations `init` fits at 240 KiB and not
at 224; with three it fits at 288 and not at 272. The third migration costs
about 48 KiB of headroom — two tables, two indexes and a schema rewrite, each a
page in the database file and a frame in the log that is checkpointed after
them. The wide end is now 512 KiB and the test's comment carries both edges,
so the next migration's author starts with the number rather than with a red
test. This is a consequence of D-73 the freeze did not name; it names no user
condition, since a repository with under 300 KiB of free space was already
inside the band where `init`'s answer and `status`'s answer are what the test
holds together.

### The mutations, and what each turned red

Run as in §1: `sed` on the committed code under a 1 GB memory scope and a
timeout, file restored and `cmp`'d. The migration-file mutations also turn the
shipped-file pin red, as they should; that line is given once.

| # | Mutation | Red |
|---|---|---|
| M1 | `alterations` never reads the column (`&& false` on the `ADD` branch) — every `ALTER` forgets, as before this task | `TestLoadReadsTheColumnsOfTheEmbeddedSchema`: `Added = map[], want exactly {tasks: [revision]}` and `migration 3 forgets [tasks]`; `TestStatusExpectsAColumnAMigrationAdded`: `Added[thing] = [], want [revision note]` |
| M2 | `declaredColumns` never applies `Added` | `TestStatusExpectsAColumnAMigrationAdded`: `Status = <nil>, want ErrSchemaShapeChanged: the added column is gone and the checker must say so` |
| M3 | `TableSchemaVersion` back to 2 | `TestCoordinationCommandsSendASchemaBehindDatabaseToInit`: `task list at schema 2 exited 0 on a schema-behind database, want 1` — the store does not read `revision` yet, so without the gate a schema-2 database answers as if it were current |
| M4 | the index loses its `WHERE released_at IS NULL` | `TestTheDatabaseRefusesASecondActiveLeaseOnOneTarget`: `active lease after the first was released = … UNIQUE constraint failed: leases.project_id, leases.target_kind, leases.target_key (2067), want no error` |
| M5 | `DEFAULT 1` → `DEFAULT 0` | `TestATaskWrittenBeforeTheRevisionColumnStartsAtOne`: `revision of a task written before the column = 0, want 1`; `TestMigrationThreeLeavesTheObjectsTheDesignNames`: `a newly opened task is at revision 0, want 1`; and the pin: `000003_lease_idempotency.sql has been edited after it was applied: sha256 9f2886…` |
| M6 | the pattern requires the `COLUMN` keyword | `TestStatusExpectsAColumnAMigrationAdded`: `Added[thing] = [revision], want [revision note]: both spellings of ADD COLUMN are read` |

### Where this task departed from the freeze, and why

- **The shipped-file pin covers 000003 from today, not from the release.** The
  pin's own comment said a row is gained "when it ships"; pinning now means an
  edit to the file during the remaining tasks turns a test red and has to be
  recorded here, which is the discipline the pin exists for. The comment says
  so.
- **AC-02.4's "dropping any of the three is reported as damaged" was first
  proved on a fixture table, not on `tasks`** — on the claim that dropping
  `revision` from the real `tasks` under `foreign_keys = 1` needs the rebuild
  the migration avoided. *That claim was false* (the Breaker ran
  `ALTER TABLE tasks DROP COLUMN revision` with foreign keys on and it
  succeeded): SQLite drops a column that no key, index or constraint names,
  whatever the foreign keys pointing at the table. The gate added
  `TestTheRealTasksTableIsCheckedForItsAddedColumn`, which drops the real
  column and gets `ErrSchemaShapeChanged`. The fixture test stays for the
  spellings, and `TestLoadReadsTheColumnsOfTheEmbeddedSchema` pins that the
  real set's `ADD COLUMN` is read as `revision` on `tasks`.
- **The RENAME arm uses `RENAME COLUMN`, not `RENAME TO`.** AC-02.4 said "an
  `ALTER TABLE … RENAME` in a fixture still forgets the table"; a `RENAME TO`
  is already a schema *effect* (the old name is removed, the new has no
  `CREATE`), so it never reached the column ledger. `RENAME COLUMN` is the
  form that does, and the fixture pairs it with an `ADD COLUMN` in the same
  file to show the forgetting wins.
- **One test outside REQ-02's files was edited:** the hostile-filesystem
  sweep, for the reason measured above.

### The gate

Two agents over `aa8d1a2` and this record at `85bb2f3`: a Reader (Sonnet)
over the record, a Breaker (Fable) with the new binary and the MR-003 binary
built from `6a78f8d` in a second worktree, over scratch repositories and a
throwaway parser test. Fixes: `fccc94f`; `make check` (19 `ok`) and
`make tidy-check` green after it; **825** top-level test functions, from 822.

**The Reader** checked 34 claims and D-73's four sentences: 27 confirmed,
**0 false**, 7 unconfirmed (the whole-suite counts it may not run, the
ready-band edges, and three mutations it did not re-run; the three it re-ran
— M2, M3, M6 — matched the recorded red lines, and it derived M5's pin hash
without touching the file). One MEDIUM: the ready-band edges (240/224,
288/272 KiB) rest on the sweep this record describes, and no test asserts the
boundary itself. That is deliberate and now said here: the sweep's own comment
records that the edge moves with the page size, the migration count and the
log SQLite keeps, so an assertion on it would be a row that stops reproducing;
the wide end at 512 KiB is the persisted assertion, and the edges are
measurements, dated by the commit that recorded them. One LOW — quoted
identifiers in `ADD COLUMN` unexercised — was overtaken by the Breaker's
first finding below.

**The Breaker** produced twelve findings, three of them HIGH and all three in
one place, and could not break the rest:

| Grade | What | Origin | Done |
|---|---|---|---|
| HIGH | `ALTER TABLE t ADD COLUMN [c] TEXT` (and the backtick form) was read as a column named **`column`**: the pattern's optional `COLUMN` group backtracked and the keyword matched as the name. `Added=map[t:[column]]` → `Status: … missing columns … t (column)` on the schema the migration itself built, with a rebuild remedy that replays the same file into the same error | `aa8d1a2` | The added column is read by `firstToken`, the tokenizer the column list uses, which knows SQLite's four quotings; a bare word `column` after `ADD` is the keyword, a quoted one is a name. `TestAddColumnIsReadTheWayTheColumnListIs`, fifteen shapes, each applied and then `Status`-checked on what it built; G2 below |
| HIGH | A bare name in another script, `sütun`, was read as `s`: the identifier class was ASCII | `aa8d1a2` | Same fix; the `sütun` row of the same test |
| HIGH | An `ADD COLUMN` at line start inside a `/* */` block, or inside a string literal spanning lines, was read as real — `Added=map[t:[ghost]]` on a healthy schema. Before this task the same text made the checker *forget* the table | `aa8d1a2` (the CREATE side's analogue predates) | Comments are stripped before the scan (`stripComments`, already in the file); G1. The string-literal case is the one shape neither this nor the CREATE side can tell from a real head — a statement head at line start inside a multi-line literal — and is recorded as the shared limit rather than fixed on one side |
| MEDIUM | `ALTER TABLE t` on one line and `ADD COLUMN c TEXT` on the next was sorted into `forgotten` — the silent removal from the F9 check D-73 says the task fixes, reachable by a line break | `aa8d1a2` | The words after the head are read across whitespace of any kind up to the semicolon; the "split over two lines" row; G3 |
| MEDIUM | The gate's sentence at ledger 2 was false: "the migration that creates the sessions, tasks and checkpoints tables has not been applied", on a database that had all three (`sqlite3` listed them). Code, exit and remedy were right | `aa8d1a2` | `schemaBehind` names the gap by number — "has applied migrations up to 2 and the coordination commands need 3" — with `applied_version` and `required_version` in the metadata; `TestCoordinationCommandsSendASchemaBehindDatabaseToInit` asserts both at both downgrades; G5 |
| MEDIUM | This record's sentence that dropping `revision` from the real `tasks` under `foreign_keys = 1` "needs the same rebuild the migration avoided" was false: `ALTER TABLE tasks DROP COLUMN revision` succeeded with foreign keys on, `foreign_key_check` clean, and the new binary then reported `MIGRATION_FAILED … tasks (revision)` | record | Corrected in place above, marked; `TestTheRealTasksTableIsCheckedForItsAddedColumn` proves AC-02.4 on the table that ships |
| MEDIUM | `CREATE TABLE cnt AS SELECT count(*) AS n FROM src` was read as a column list `[*]` — the parenthesis in `count(*)` — so every healthy schema was "missing" a column named `*` | predates (`tableColumns`) | The `AS` after the name is checked by word; `TestACreateTableAsSelectIsNotReadAsAColumnList`; G4 |
| LOW | A `revision` column added by hand at ledger 2 makes `init` fail on `duplicate column name` with the remedy "inspect the migration SQL … re-run `mindrail init`", which fails identically | `aa8d1a2` (first `ADD COLUMN`); needs a hand edit | Recorded, not fixed: it is the shape any hand-created object has against the migration that creates it, and the remedy's wording is MR-001's. Backlog |
| LOW | `Added` is applied after all of a file's effects, so `ADD COLUMN c; DROP TABLE t; CREATE TABLE t (a)` in one file demands `c` | `aa8d1a2`, contrived | Recorded as a limit: `Columns` is per table and the last `CREATE` wins there too; a file that adds a column and then recreates the table without it is a file that should not be written |
| LOW | Fail-open variants: `CREATE TABLE Tasks` against `ALTER TABLE tasks` (case), `ALTER TABLE main.t` (schema-qualified, forgotten as `main`), a `/* note */` between the table name and `ADD` (now read, since comments are stripped) | case and `main.` predate in class on the CREATE side | The comment case is closed by G1's fix; the other two are recorded as the CREATE side's existing limits |
| LOW | The missing-column remedy at ledger 3 is a data-losing rebuild ("move the database aside and run `mindrail init`") for a one-column repair | predates; `migrator.go` defers it to "a milestone that stores something irreplaceable" | Recorded. MR-004 is arguably that milestone; the decision is left to the record's reader rather than taken in a gate |
| LOW | `DROP INDEX idx_leases_active` by hand is invisible: `status` and `doctor` report the database healthy, and AC-02.1's guard is gone | predates (the object check covers tables; indexes were narrowed out by design) | Recorded. TASK-03's store refuses by name before the index would, so the guard's absence is not silent at the write; whether `doctor` should verify indexes is a question for its own work |

Attacked and not broken, in one line each. The real upgrade: a repository the
MR-003 binary initialised — one session, four tasks in four states, two
checkpoints — answered `status` 0 (DEGRADED), `doctor` 0, `task list` and
`task show` 1 before `init`, all 0 and READY at schema 3 after it; the dump
diff was the ledger row, `,1` on every task and `workspaces.updated_at`, and
the ledger's checksum for 3 equals the file's. The MR-003 binary over the
upgraded database: every command `RUNTIME_DB_SCHEMA_TOO_NEW` at exit 1 and
the data untouched. The gate at ledger 2 wrote nothing: `sessions=1 tasks=4
checkpoints=2` after every refusal. `leases` or `operations` dropped at ledger
3: `MIGRATION_FAILED … table leases` on `status`, `doctor`, `task list` and
`init`. At SQL level: a second unreleased row and an expired-but-unreleased
row plus a new one both `UNIQUE constraint failed`; release then re-lease
allowed; another project allowed; `src/A.go` and `src/a.go` are two targets
(by design); a holder that is no session and a `DELETE FROM sessions` under a
lease both `FOREIGN KEY constraint failed`; a hand-inserted task is at
`revision 1`; a text or real `revision` is refused by `STRICT`, `'2'` is
stored as 2 by STRICT's documented coercion, and `-7` and
`target_kind = 'symbol'` are accepted (no `CHECK`). The space edges
reproduced under `unshare -Urm`: with three migrations `init` exits 4 at 224,
240, 256 and 272 KiB and 0 from 288; the MR-003 binary exits 4 at 224 and 0
at 240 — both edges as recorded. Two `init` at once over a schema-2 database:
one applied `[3]`, the other `[]`, one ledger row, four objects, one
`revision` column. The parser on `"with space"`, lower-case keywords, `ADD`
without `COLUMN`, CRLF, `c$1`, a generated column, a `CHECK` whose literal
says `'ADD COLUMN d'`, `IF NOT EXISTS`, a `--` comment; add-then-drop, a
later `RENAME TO` and an `AS SELECT` forget by design; a duplicate `ADD` and
an `ADD` on a table that does not exist are `MIGRATION_FAILED` at `Up`.

Not demonstrated, for TASK-03: a hand-planted `released_at = ''` sits outside
the partial index while a reader that tests `IS NULL` would call it held.
TASK-03's reader parses the column; an empty string does not parse and the
row is refused as damaged (`COORDINATION_READ_FAILED`), which is the answer a
planted value gets everywhere in this package.

| # | Mutation (gate fixes) | Red |
|---|---|---|
| G1 | `alterations` scans the body with comments left in | `TestAddColumnIsReadTheWayTheColumnListIs/inside_a_block_comment`: `Added[t] = [ghost], want []` and `Status = MIGRATION_FAILED: … t (ghost) on the schema the migration built` |
| G2 | the quoted guard dropped: `"column"` in quotes is the keyword again | `…/a_column_named_column,_quoted`: `Added[t] = [text], want [column]` |
| G3 | the statement ends at the first newline instead of the semicolon | `…/split_over_two_lines`: `Added[t] = [], want [c]` and `Altered = [t], want forgotten = false` |
| G4 | `tableColumns` no longer checks for `AS` | `TestACreateTableAsSelectIsNotReadAsAColumnList`: `Columns[cnt] = [*], want the AS SELECT table skipped` and `Status = MIGRATION_FAILED: … cnt (*)` |
| G5 | the gate's threshold back to 2 | `TestCoordinationCommandsSendASchemaBehindDatabaseToInit`: `task list at schema 2 exited 0 on a schema-behind database, want 1` |

**Carried forward from this gate:** the multi-line-literal limit shared with
the CREATE side; the hand-added-column `init` loop and the data-losing
rebuild remedy (backlog, both predate in class); index verification in
`doctor` (unowned).

---

## 3. TASK-03 — the lease: one row per tenure, and the three verbs over a file

Commit `8c7107a`. `make check` (19 `ok`, no `FAIL`) and `make tidy-check`
green; **835** top-level test functions, from 825. Owns REQ-03, REQ-04 and
the three lease codes of REQ-07, plus the finding TASK-01's gate carried here.

### What changed

| Where | What |
|---|---|
| `internal/coordination/lease.go` (new) | `LeaseTTL` (20 min, D-69); `TargetKind` with `task` and `file` and `ParseTargetKind`; `Target`, `TaskTarget`, `FileTarget` — D-77's normalisation (backslashes read as separators, `path.Clean`, nothing else rewritten) and refusals (blank, invalid UTF-8, absolute, climbing above the root, the root itself), each `COMMAND_LINE_INVALID`; `LeaseStatus` (`active`, `expired`, `released`) judged in Go by `statusAt`; the four release reasons; `Lease` with design §5's column names plus `status`; `Acquisition` — the lease, whether the call renewed, the tenure it superseded |
| `internal/coordination/lease_store.go` (new) | `AcquireLease` (file targets only; a task target is refused with the remedy to move the task, D-66), `RenewLease`, `ReleaseLease`, `FindLease`, `ListLeases`. Design §6's table is `acquireIn` and `holderWrite`: the clock read inside the transaction under the write lock, the session resolved there, the target's unreleased row read and judged at that instant, and — active/mine renew, active/other `LEASE_CONFLICT`, expired close-with-`expired`-and-insert naming the superseded tenure, none/released insert. `acquireIn` takes the caller's `*sql.Tx` and `now`, so `Transition` can run the same rule for a task target in TASK-04 |
| `internal/coordination/errors.go` | `ErrLeaseConflict`, `ErrLeaseNotHeld`, `ErrLeaseNotFound`; `leaseConflict` (holder, expiry, lease id, target in metadata; remedy: wait until the expiry it names, or ask the holder to `lease release <id>`), `leaseNotHeld` (status and the remedy per kind — a move for a task, `lease acquire --file <key>` for a file), `leaseNotFound` (`mindrail lease list`), `noProject` beside `noWorkspace` |
| `internal/coordination/store.go` | `Write.Timing storage.TxStats` (D-75); `OpenSession` runs through `InTxMeasured` — the bare `ExecContext` TASK-01's Breaker found is gone, and a busy `BEGIN` there now carries `waited_ms` like every other writer; `OpenTask`, `Transition` and `WriteCheckpoint` use `InTxMeasured` and fill `Timing` |
| `internal/app/code.go`, `internal/cli/envelope_test.go` | `LEASE_CONFLICT`, `LEASE_NOT_HELD`, `LEASE_NOT_FOUND`, registered, all `ExitFailed` |
| Tests | `internal/coordination/lease_store_test.go` (ten, new): `TestFileTargetIsNormalisedAndRefusedByTheRule` (ten keys normalised, nine refused, `ParseTargetKind` both arms), `TestTheLeaseTTLIsTwentyMinutes` (on the row written), `TestEveryCellOfTheLeaseTable` (AC-04.2: fifteen cells, each built the way a repository reaches it, the successes asserted on the re-read row and the refusals on the code, the metadata and the row left as found), `TestAcquiringOverAnExpiredTenureClosesItAndSaysSo` (AC-04.3, one second before expiry refused, at expiry taken over, the old row closed at the takeover's instant, one unreleased row), `TestAnAcquisitionByTheHolderRenewsUnderTheSameID`, `TestListLeasesReportsActiveOnesOldestFirstAndMarksNothing` (AC-04.4, D-79), `TestLeaseRefusalsCarryWhatACallerActsOn` (AC-04.5), `TestATaskTargetIsNotAcquiredDirectly` (D-66), `TestLeaseWritersMintAndRefuseSessionsLikeTheOthers` (D-61 on the new writers, `Timing` filled), `TestSessionOpenIsOneTransactionWithItsWait` (the carried finding) |

### The mutations, and what each turned red

| # | Mutation | Red |
|---|---|---|
| L1 | `acquireIn` treats another session's active lease as the caller's own | `TestEveryCellOfTheLeaseTable/active,_another's_/_acquire`: `want LEASE_CONFLICT, got no error` |
| L2 | `statusAt` uses `now.After(expires)` instead of `!now.Before(expires)` — the boundary instant counts as held | `TestAcquiringOverAnExpiredTenureClosesItAndSaysSo`: `AcquireLease at expiry = LEASE_CONFLICT: file src/auth.go is held by session SES-… until 2026-09-09T08:50:00Z, want the takeover` |
| L3 | the takeover does not close the expired row before inserting | the same test: `AcquireLease at expiry = COORDINATION_WRITE_FAILED: the lease could not be written …` — the partial unique index refusing the second unreleased row, which is AC-02.1's guard holding behind a Go defect |
| L4 | `holderWrite` skips the holder check | `…/active,_another's_/_renew` and `…/_release`: `want LEASE_CONFLICT, got no error` |
| L5 | `ListLeases` lists every unreleased row | `TestListLeasesReportsActiveOnesOldestFirstAndMarksNothing`: `ListLeases = [{ID:LSE-… TargetKey:a.go …}], want only the second lease, active` |
| L6 | `FileTarget` stops refusing a key that climbs above the root | `TestFileTargetIsNormalisedAndRefusedByTheRule`: `want COMMAND_LINE_INVALID, got no error` |
| L7 | `renewIn` moves `renewed_at` and leaves `expires_at` | `…/active,_mine_/_acquire` and `…/_renew`: `expires_at = 2026-09-09 08:50:00 +0000 UTC, want now + TTL = … 08:51:00`; `TestAnAcquisitionByTheHolderRenewsUnderTheSameID`: `expires_at = …, want moved to now + TTL` |
| L8 | `OpenSession` back to a bare `ExecContext` | `TestSessionOpenIsOneTransactionWithItsWait`: `session open under a held lock publishes no waited_ms: map[condition:locked subject_id:SES-…]` |
| L9 | `LeaseTTL = 19 * time.Minute` | `TestTheLeaseTTLIsTwentyMinutes`: `LeaseTTL = 19m0s, want 20m (spec §59)` |

### Where this task departed from the freeze, and why

- **`AcquireLease` refuses a task target outright rather than not offering
  it.** AC-04.1 lists the method without saying what a task target does; D-66
  says a task lease is acquired only by moving the task. The refusal is a
  usage error naming `task state`, so a caller of the domain API — MR-015's
  tools — cannot grow a second entry point by accident.
- **`AcquireLease` takes a `Target`, not a kind and a key.** The value is
  built by `FileTarget` or `TaskTarget`, which is where D-77's rule lives; a
  method taking two strings would let a caller pass a key nothing normalised.
- **`Operation` is not yet a parameter.** The work breakdown gives it to
  TASK-05 on every writer at once; the three lease writers will take it then,
  as the four existing ones will.
- **Two things beyond REQ-03/04's letter, both from earlier gates:**
  `OpenSession` through `InTxMeasured` (TASK-01's gate, carried here), and
  `Write.Timing` on all seven writers (D-75 said the store carries it; this is
  the first task to touch the store).
- **`noProject` is a new constructor** for a writer that belongs to a project
  rather than a worktree; it is `noWorkspace`'s shape under the same code.

### The gate

*Filled after the Reader/Breaker pair has run.*
