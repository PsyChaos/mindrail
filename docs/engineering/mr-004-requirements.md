# MR-004 — Frozen requirements and acceptance criteria

- **Frozen at:** commit `6a78f8d`, branch `mr-002-knowledge-lifecycle` (MR-004
  work continues on this branch)
- **Contract this refines:** [mr-004-design.md](mr-004-design.md). Where the two
  disagree, the design wins on intent and this document wins on the detail the
  design left open — every such gap is recorded below as a decision D-64…D-79.
- **Task:** [mindrail-0.1-task-list.md](mindrail-0.1-task-list.md) MR-004
- **Tier:** 3 (Program). It adds a runtime migration, two new persisted
  entities and a column on a third, six `app.Code` values that enter the wire
  vocabulary, one command group, one flag on every existing writer and a
  second on `task state`, and an additive key in `status --json`. One existing condition changes code
  (D-74). The escalation rule fires on "public API contract" regardless of line
  count.

This file is frozen **before** implementation so that an audit has something to
grade against that the implementation did not shape. MR-004 is also the first
milestone run **one task at a time**: each task in §4 is implemented, gated by
an independent Reader/Breaker pair and recorded before the next is begun. That
is the process change round 2 of MR-003 was run to measure the need for —
twelve of its seventeen defects were the remediation over-describing its own
work, and a pile of nine tasks audited at the end is where such sentences
accumulate.

---

## 0. Baseline evidence

Captured on the tree this work starts from, so a red suite later is unambiguous.

| Measurement | Value | Command |
|---|---|---|
| Commit | `6a78f8d` | `git rev-parse --short HEAD` |
| Go toolchain | `go1.27.0 linux/amd64` | `go version` |
| Packages | 21 | `go list ./... \| wc -l` |
| Top-level test functions | 807 | `go test -list '.*' ./... \| grep -c '^Test'` |
| `make verify` | green — 39 `ok` lines across check, race and smoke, no `FAIL`, exit 0 | `make verify` |
| `make tidy-check` | green, exit 0 | `make tidy-check` |
| Migrations | `000001_initial.sql`, `000002_coordination.sql`; runtime `schema_version` reports 2 | `ls migrations/*.sql`; `internal/cli/testdata/status_human.golden` line 15 |
| `go.mod` direct requires | `go-toml/v2 v2.4.3`, `jsonschema/v6 v6.0.3`, `cobra v1.10.2`, `modernc.org/sqlite v1.58.0` | `go.mod` |

MR-004 adds **no** module dependency. `make tidy-check` must still be run
explicitly and reported, because it is in neither `check` nor `verify`.

---

## 1. Decisions this document freezes

MR-003 ended at D-63. These continue the sequence. Each one existed as an
ambiguity two implementers would have resolved differently, or as a place where
the specification leaves the milestone to decide.

### D-64 — one `leases` table for every target kind, one row per tenure, inside `internal/coordination`

A lease is a session holding a target from an acquisition to a release. The
target is `(kind, key)`; MR-004 accepts `task` and `file`, and `symbol` is a
constant MR-006 adds when there is a `symbol_uid` to name. All kinds share one
table, one unique index and one set of rules, so that the file lease
`before_change` will acquire in MR-007 is the code the task lease already runs
through in MR-004.

A row is never turned into a different tenure. Renewal moves `renewed_at` and
`expires_at` on the row; a release or an expiry closes it; a takeover after
expiry closes the old row and inserts a new one. The table is therefore the
lease history spec §59 keeps as events, and there is no second table for
them. Renewals do not append: every holder write would leave a row, and a
table that grows with every command is a log, not a history.

The tech-stack §7 layout has a `lease/` package. It is filled inside
`internal/coordination`, where `session/`, `task/` and `checkpoint/` already
are, because the task lease is written in the transaction that moves the task
and a package boundary through one `BEGIN IMMEDIATE` separates nothing.
`internal/coordination`'s import allow-list does not change.

### D-65 — active means unreleased and unexpired by the store's clock, judged in Go, and expiry is lazy

A lease is active when `released_at IS NULL` and `expires_at` is after the
store's clock reading, parsed and compared in Go. No SQL compares `expires_at`:
the column is `TEXT` in the format decision D-59 recorded as not
lexicographically ordered, and the unique index does not need the comparison —
it is on `released_at IS NULL` alone.

An expired lease stays unreleased until the next writer that needs the target
closes it with reason `expired` and takes over, in its own transaction. Reads
do not close it: `lease list`, `task show` and `status` report the row as
expired and write nothing (D-79). The alternative — a background sweep or a
close-on-read — would be a write with no session to attribute it to.

### D-66 — a task lease is acquired only by moving the task; renew and release by id work for any kind

Decision D-58 made `CLAIMED` a state and left the lease to this milestone "on
top of this column". The claim is the acquisition: `task state --to CLAIMED`
acquires, the working moves renew, `--to OPEN` releases, the terminal moves
release. There is no `lease acquire --task`, for the reason design §7 of
MR-003 gave for one transition command rather than seven verbs: the lifecycle
is consulted in one place, and the lease has one hook into it.

`lease renew <id>` and `lease release <id>` take any lease id, task or file.
They are the explicit forms of what a holder's own writes do implicitly, for
the holder that has nothing to write — an agent inside a long test run keeps
its task with a renew; one stepping away without a note releases. Both are
refused by identity for anyone but the holder.

`lease acquire` takes `--file` and nothing else in MR-004.

*Amended during TASK-04.* `lease acquire` also takes `--task <id>`: **the
claim without a move.** The handover design §7 describes releases the lease
of a task that is almost always `IN_PROGRESS`, and D-55 has no
`IN_PROGRESS → IN_PROGRESS`, so the next agent had no move that kept the
task where it was and could not take it. On a task in a working state —
`CLAIMED`, `IN_PROGRESS`, `BLOCKED`, `READY_TO_COMPLETE` — `--task` takes the
lease where the task stands, makes the holder the claimant and raises the
revision, and does not touch the state; an `OPEN` task is refused with the
remedy `--to CLAIMED`, and a finished one as final. Both entry points run the
same acquisition and write `claimed_by` in one transaction (D-68); the
lifecycle table is still consulted in one place, because this path does not
consult it.

### D-67 — the lease is judged before the state table, and an expired lease is taken over out loud

`Transition` judges, in order and inside one transaction: the caller's
expected revision (D-72), then the task's lease, then D-55's table and D-63's
reason rule. A task whose active lease another session holds is
`LEASE_CONFLICT` whatever the destination: the reason the move cannot happen
is ownership, and a state refusal would read the same whether the lease had
expired or not, which would tell the reader nothing about what to do.

A move on a task whose lease has expired, or that has none, acquires the lease
for the mover. When a tenure was expired, it is closed with reason `expired`
and the result names it — the session it belonged to and when it ran out.
"Silently" in the task list's criterion is the word this decision answers: a
takeover is reported, a conflict is named, and neither is a no-op.

### D-68 — `claimed_by` is the holder while a lease is active, and attribution afterwards (amends D-58)

The invariant, checked after every legal transition by a test that walks them
all: **an `OPEN` task has no active lease, and a task with an active lease has
`claimed_by` equal to its holder.** Every code path that writes a task lease
writes `claimed_by` in the same statement, and the only such paths are inside
`Transition`.

Once the lease expires or is released, `claimed_by` is what D-58 said it was —
the attribution of the last claim. `task show` prints it with the tenure's end
and its reason, so a reader is never shown a claimant as if the claim still
bound anyone. The next move rewrites it, which is the amendment to D-58's "only
a claim or a release touches it": a takeover is a claim.

### D-69 — the TTL is twenty minutes, a constant

`coordination.LeaseTTL = 20 * time.Minute`, spec §59's default. Nothing reads
it from configuration and no flag overrides it. A repository that needs more
has `lease renew`; a test that needs less has the injected clock. The knob is
deferred until a repository asks for it, and when it does, it is a project
setting under `.mindrail/config.toml`, not a user one.

### D-70 — stale-lease recovery is expiry; operator override is outside 0.1

Spec §119 names "releasing a wrong or stale lease" as an override use-case and
§120 binds override to an approver model — allowlist, roles, mandatory reason,
optionally signed identity. Kernel-scope §4 puts the approval providers out of
0.1, and no milestone in the task list owns `override`. Building the command
without the model would make "who may break a lease" a question the tool
cannot answer.

So in 0.1 a lease outlives an absent holder by at most `LeaseTTL`, and the
next mover takes it over (D-67). The user chose this over a minimal
`lease break --reason` at the freeze; it is recorded so that MR-020's
acceptance review sees the gap as a decision.

### D-71 — every mutation takes an optional caller-supplied `operation_id`, and a replay is answered inside the write transaction

Seven writers — `OpenSession`, `OpenTask`, `Transition`, `WriteCheckpoint`,
`AcquireLease`, `RenewLease`, `ReleaseLease` — take an `Operation`. The zero
value means the write is not idempotent. A non-zero id is looked up **inside
the `BEGIN IMMEDIATE` transaction that would perform the write**, before any
other statement:

- found, with the same `request_hash` — the recorded result is decoded and
  returned marked `Replayed`; no other statement runs;
- found, with a different hash — `OPERATION_ID_CONFLICT`, nothing written;
- not found — the write runs, and its result is inserted into `operations` in
  the same transaction, after it.

Because lookup and record share the transaction with the write, two deliveries
serialise at `BEGIN` and the second finds the first's record. Because the
record follows the write, a refused operation records nothing and a later retry
runs fresh — recording a refusal would freeze a lease held for one more minute
into a permanent no. Because the record precedes `COMMIT`, a process killed
after the commit and before printing is answered on retry with what it would
have printed.

The hash is SHA-256 over a canonical JSON encoding of the command name and the
store parameters, computed by the store so that MR-015's tools inherit the
rule. The session handle is one of the parameters: a retry carries the same
handle, and a different handle under the same id is a different request.

The id matches `^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$` and is refused as a usage
error otherwise, before the application starts. There is no retention: one row
per idempotent mutation, as `checkpoints` keeps one per note. `session open` is
a mutation and takes the id like the others.

### D-72 — `revision` is on `tasks`, judged inside the transaction against the caller's expectation, and the update is guarded by the revision that was read

`tasks.revision` starts at 1 and every update of the row adds one. A caller may
pass `--expect-revision n` to `task state`; inside the transaction, after the
row is read and before anything else is judged, a row not at *n* is
`STATE_REVISION_CONFLICT` naming the revision and the state it actually has.
Nothing is written. Without the flag, no expectation is judged — a human at a
terminal has not read a revision and should not be made to.

The update itself is written the way tech-stack §22 writes it:
`SET …, revision = revision + 1 WHERE task_id = ? AND revision = ?` with the
value read in the same transaction. Under `BEGIN IMMEDIATE` that guard cannot
fail, so an update affecting no row is reported as
`COORDINATION_WRITE_FAILED` — a defect — and not as a second kind of conflict.

Leases carry no revision in MR-004. Their mutable columns are functions of the
holder and the clock, and every lease write is refused by identity before a
stale reading could matter. `Change` gets its own in MR-007.

### D-73 — migration 3 adds the column with `ALTER TABLE`, the ledger learns `ADD COLUMN`, and every reader of "pending" is named

`migrations/000003_lease_idempotency.sql` creates `leases`, `operations`, two
indexes, and `ALTER TABLE tasks ADD COLUMN revision INTEGER NOT NULL DEFAULT 1`.
`000001` and `000002` are not edited (D-53, `migrations/shipped_test.go`), and
the new file's sha256 joins the pin.

`ALTER` rather than recreate, because `checkpoints.task_id` points a foreign
key at `tasks`, `foreign_keys = 1` is on, and the pragma cannot be turned off
inside the migration's transaction. (`leases.target_key` is plain text: the
target is polymorphic, so no foreign key can name it.) `ADD COLUMN` with a non-null
default adds without moving anything and STRICT tables accept it.

The ledger `internal/migration` builds forgets any table an `ALTER TABLE`
touched, so this line would otherwise remove `tasks` from the column check
finding F9 introduced. The ledger learns the one form it can read exactly —
`ALTER TABLE <t> ADD COLUMN <c>` extends `<t>`'s expected columns — and keeps
forgetting for every other form.

`runtime.schema_version` changes from 2 to 3 in `status --json`, `init --json`
and doctor's rendering; the two goldens that carry it are
`internal/cli/testdata/status_human.golden` and `doctor_human.golden`, and the
diff is reviewed key by key. D-53's obligation — name every place that reads
"pending" as "absent" — has one member here: `coordination.TableSchemaVersion`
becomes 3, because the store now needs the leases and operations tables, and
`coordinationScope`'s gate reads that constant. `workspace.TableSchemaVersion`
stays 1.

### D-74 — the write-lock wait stays SQLite's, bounded by `busy_timeout`; exhaustion is `MINDRAIL_BUSY_RETRYABLE` at both sites; the ladder replaces the open path's flat poll

Spec §11 asks for bounded exponential backoff with jitter on `SQLITE_BUSY`,
and `MINDRAIL_BUSY_RETRYABLE` after the budget. MR-001 deferred both here.

What is bounded already: a writer meeting the lock at `BEGIN IMMEDIATE` waits
inside SQLite's busy handler for `busy_timeout` (5 s, D-08/D-24), which is
itself a stepped backoff from 1 ms to 100 ms. SQLite invokes that handler for
the write lock and for a stale snapshot alike when the transaction starts from
nothing, so a `SQLITE_BUSY` that reaches Go from `BEGIN` has already waited the
whole budget. A second ladder on top would wait the budget again per step,
past the shutdown budget D-08 fixed at the same five seconds. **There is no
application retry of `BEGIN`.** The Breaker of TASK-01 is asked to produce an
immediate `SQLITE_BUSY` at `BEGIN` on this driver and this DSN; if it can, this
decision is amended.

What is not bounded by a handler: the open path, where a database another
process is still converting to WAL refuses at once and `openWithinBusyBudget`
polls every 20 ms. That poll becomes the ladder: 25, 50, 100, 200, 400 ms and
then 400, each step drawn from `[step/2, step]`, under the same budget.

Both exhaustions publish `MINDRAIL_BUSY_RETRYABLE`, `KindUnavailable` (exit 4
unchanged), `metadata.condition = "locked"`, `metadata.waited_ms`, and the
remedy to run the command again. `storage.busyFailure`'s
`RUNTIME_DB_UNAVAILABLE` for the locked condition is retired, and so is the
generic open remedy on that path. `RUNTIME_DB_UNAVAILABLE` keeps every other
condition it names. The `MINDRAIL_` prefix is decision D-18's reservation for
exactly this code.

### D-75 — every write transaction is measured, the numbers ride on `Write`, and the wire waits for MR-019

`storage.InTx` records the wait for `BEGIN` and the time from `BEGIN` to
`COMMIT`, and hands them to a caller that asked (`storage.WithTxStats(ctx)`).
The coordination store asks on every write and carries `Timing{Waited, Held}`
on `Write`; the command logs both at DEBUG with the operation's name. They do
not appear in the JSON envelope: kernel-scope §3 requires collecting SQLite
wait from the first implementation, and MR-019 owns the shape it is published
in. AC-10.6 reads them from the store.

*Amended during TASK-01's gate.* The seam is `storage.InTxMeasured`, which
returns the `TxStats` **by value** beside the transaction's outcome; `InTx` is
the same transaction without the numbers. The shape above planted one
`*TxStats` in a context, and two goroutines running `InTx` under one context
raced on it under `-race` (the Breaker's finding). A value per call cannot be
shared by accident, so the store needs no rule about where it plants what.
Two facts about the numbers, recorded because the Breaker measured them:
`Waited` runs from the call to the lock and so includes database/sql's pool
queueing — nothing for the one-goroutine command line, and the wait an
interactive writer actually experiences otherwise; AC-10.6's eight goroutines
over a pool of four therefore read pool queueing as well as SQLite's, and
its bound is on `Held`, which is unaffected. A rolled-back transaction reports
zero.

### D-76 — six codes, each with a remedy the others do not have

| Code | Condition | Remedy |
|---|---|---|
| `LEASE_CONFLICT` | the target's active lease is held by another session | wait for the expiry it names, or ask the holder to release |
| `LEASE_NOT_HELD` | this session's lease on this target has expired or been released | acquire again — `lease acquire` for a file, a move for a task |
| `LEASE_NOT_FOUND` | no lease carries this id | `mindrail lease list` |
| `STATE_REVISION_CONFLICT` | the task is not at the revision the caller expected | `task show`, then retry with the revision it prints |
| `OPERATION_ID_CONFLICT` | the id was used before for a different request | mint a new id for a new request |
| `MINDRAIL_BUSY_RETRYABLE` | the write lock was held past the budget | run the command again |

All six are `KindFailed` (exit 1) except `MINDRAIL_BUSY_RETRYABLE`, which is
`KindUnavailable` (exit 4) because nothing is wrong. `LEASE_CONFLICT` and
`LEASE_NOT_HELD` are two codes rather than one with a flag because a caller's
next action differs: wait, or re-acquire.

### D-77 — a file lease key is a normalised repository-relative path, and the file need not exist

Forward slashes, no leading `./`, no `.` or `..` segment, not absolute, not
empty, and not a path that after cleaning is the repository root itself. The
key is stored cleaned so that `a/./b` and `a/b` are one target. A key that
fails is `COMMAND_LINE_INVALID` (exit 2) at the command line, and the store
refuses it with the same code — it is the surface MR-015 calls without a
command line in front of it, and `OpenTask` already refuses an empty title
there the same way.

The file need not exist. A lease on a path is a statement about who is writing
there.

### D-78 — a holder's checkpoint renews, a holder's handoff releases, and anyone else's checkpoint touches no lease

`checkpoint write` by the session holding the task's active lease renews it;
with `--handoff` it releases it with reason `handoff`. This is the first
behaviour `--handoff` has had; MR-003 persisted the column for exactly this.
A checkpoint by any other session is written and changes no lease: a note is
not a claim, and the arriving agent's first act in MR-003's scenario — writing
"picking this up" — must not be refused for the lease it is about to take.

### D-79 — reads write nothing, and still tell the truth about time

`lease list`, `task show` and `status` evaluate expiry against the store's
clock at read time and report `active`, `expired` or `released`. They mark no
row (D-65), mint no session (D-56) and hold no write transaction. The counts
`status` publishes exclude expired rows because they are evaluated, not because
they were closed.

---

## 2. Requirements

### REQ-01 — `internal/storage`: the bounded wait, named and measured

- **AC-01.1** `storage.Backoff` yields 25, 50, 100, 200, 400 ms and then 400 for
  every further step before jitter; with jitter each step lies in
  `[step/2, step]`. Asserted with a fixed random source over ten steps.
- **AC-01.2** `openWithinBusyBudget` sleeps by the ladder and gives up at the
  same budget it gives up at today; `TestOpenWaitsOutAConcurrentWALConversion`
  still passes, and a new test that answers every open attempt with busy under
  a 200 ms budget observes at least three attempts, the gaps between them
  rising by the ladder, and no attempt after the budget plus one jittered step.
- **AC-01.3** A `BEGIN` refused as busy after the budget, and an open refused as
  busy after the budget, both publish `MINDRAIL_BUSY_RETRYABLE` with
  `KindUnavailable`, `condition = "locked"` and `waited_ms`. No other condition
  changes code: the `RUNTIME_DB_UNAVAILABLE` rows of the contract matrix are
  unchanged.
- **AC-01.4** `InTx` records wait and hold into `*TxStats` when the context
  carries one, and touches nothing when it does not. Asserted both ways.

  *Amended during TASK-01's gate (D-75 as amended).* `InTxMeasured` returns
  the two numbers by value; a committed transaction fills them, a rolled-back
  one reports zero, and `InTx` is the same transaction without them. The
  concurrent-callers arm runs under `-race`.
- **AC-01.5** There is no retry loop around `BeginTx` in `InTx`; a test with a
  50 ms contender still fails within 250 ms
  (`TestInTxTakesTheWriteLockAtBegin` stays fast).

### REQ-02 — `migrations/000003_lease_idempotency.sql`

- **AC-02.1** The two tables, two indexes and one added column of design §5
  exist, `STRICT`, with the partial unique index on `released_at IS NULL`. A
  second unreleased row for one target is refused by the database, asserted
  with a direct `INSERT` and no Go in between.
- **AC-02.2** `000001_initial.sql` and `000002_coordination.sql` are
  byte-identical to their content at `6a78f8d`; the shipped-file pin covers
  the third file.
- **AC-02.3** A database created at `6a78f8d` and opened by this binary applies
  exactly migration 3, keeps every task row with `revision = 1`, and reports
  `schema_version` 3. A fresh database applies all three and reports 3.
- **AC-02.4** The migration ledger names `leases` and `operations`, and expects
  `revision` among `tasks`' columns: dropping any of the three is reported as
  damaged, not healthy. The `ADD COLUMN` grammar is exercised in both
  directions — an `ALTER TABLE … RENAME` in a fixture still forgets the table.
- **AC-02.5** `coordination.TableSchemaVersion` is 3, and a database at ledger
  version 2 is refused by every coordination command with `MIGRATION_FAILED` /
  "Schema is behind this binary" and the `mindrail init` remedy — the round-2
  §4.1 gate, one version later.

### REQ-03 — `internal/coordination`: the lease domain

- **AC-03.1** `Lease` is an exported struct with JSON tags matching design §5's
  columns plus `status` (`active`, `expired`, `released`), computed at read
  time by the store's clock.
- **AC-03.2** `TargetKind` has the values `task` and `file`; `ParseTargetKind`
  refuses anything else. `FileTarget(path)` normalises by D-77 and refuses by
  D-77, and the refusal is `COMMAND_LINE_INVALID`.
- **AC-03.3** `LeaseTTL` is 20 minutes and is the only place the number appears
  in non-test code.
- **AC-03.4** The package's import allow-list is unchanged
  (`TestDomainPackagesImportOnlyWhatTheirRequirementAllows` still holds).

### REQ-04 — `internal/coordination.Store`: leases

- **AC-04.1** `AcquireLease`, `RenewLease`, `ReleaseLease`, `ListLeases` and
  `FindLease`, each one short transaction, each taking `context.Context`, the
  writers taking `Attribution` and `Operation` and returning a `Write`.
- **AC-04.2** Every cell of design §6's table is exercised: five situations ×
  three verbs, each asserted on the `Code` and, for the writes, on the row
  afterwards by re-reading it.
- **AC-04.3** An acquisition over an expired tenure closes it with reason
  `expired` and inserts a new row; the result names the superseded tenure's
  holder and expiry; the table holds exactly one unreleased row for the target.
- **AC-04.4** `ListLeases` returns active leases only, oldest acquisition first;
  an expired row is neither listed nor modified — asserted by reading
  `released_at` afterwards.
- **AC-04.5** Every error carries a registered code, a diagnostic, an impact and
  a remedy, and names the lease id or the target in `metadata` (AC-04.6 of
  MR-003, as amended, applies unchanged).

### REQ-05 — the task lifecycle with a lease and a revision

- **AC-05.1** Each row of design §7's destination table is exercised, and after
  every legal transition of D-55's 49 pairs the D-68 invariant holds — asserted
  by a test that walks all of them, in both the "held by the mover" and the
  "expired" starting states.
- **AC-05.2** A move on a task whose active lease another session holds is
  `LEASE_CONFLICT` naming the holder, the lease id and the expiry, for every
  destination including the terminal ones; the row is unchanged, asserted by
  re-reading it.
- **AC-05.3** A move on a task whose lease expired succeeds, closes the old
  tenure with reason `expired`, and the result names it.
- **AC-05.4** `--expect-revision n` on a row not at *n* is
  `STATE_REVISION_CONFLICT` naming the current revision and state, before any
  lease or state judgment; the row is unchanged. On a row at *n* the move
  proceeds and the result carries *n+1*.
- **AC-05.5** `revision` rises by exactly one on every successful `Transition`
  and on nothing else; `OpenTask` writes 1.
- **AC-05.6** A holder's `WriteCheckpoint` renews (`expires_at` moves forward);
  with `handoff` it releases with reason `handoff`; a non-holder's writes the
  checkpoint and leaves the lease row byte-identical.
- **AC-05.7** The D-55 table is re-walked with the lease in place: every legal
  pair still succeeds when the mover holds the lease or none exists, every
  illegal pair is still `TASK_STATE_INVALID`. The lease changed no transition.

### REQ-06 — idempotent mutations

- **AC-06.1** All seven writers take `Operation`; with the zero value their
  behaviour and their SQL are unchanged (no `operations` statement runs,
  asserted by counting rows).
- **AC-06.2** The same id and the same request, twice: one row of the entity,
  one row of `operations`, and the second `Write` is `Replayed` with a result
  equal to the first's. Asserted for every writer.
- **AC-06.3** The same id with a different request is `OPERATION_ID_CONFLICT`
  and writes nothing. A different `--session` counts as a different request.
- **AC-06.4** A refused operation records nothing: a `Transition` refused as
  `LEASE_CONFLICT` under an id, then retried under the same id after the lease
  is released, succeeds and records then.
- **AC-06.5** Two concurrent deliveries of one operation produce one entity row
  and one operation row, under `-race`.
- **AC-06.6** The hash is stable across processes: computed twice from the same
  parameters in two `Store`s it is equal; changing any one parameter changes it.

### REQ-07 — the code vocabulary

- **AC-07.1** Six values are added to `app.Code` and to the registry:
  `LEASE_CONFLICT`, `LEASE_NOT_HELD`, `LEASE_NOT_FOUND`,
  `STATE_REVISION_CONFLICT`, `OPERATION_ID_CONFLICT`,
  `MINDRAIL_BUSY_RETRYABLE`.
- **AC-07.2** No existing `app.Code` value changes spelling or meaning. The one
  condition that changes code — the locked database — is recorded in D-74, and
  `RUNTIME_DB_UNAVAILABLE` keeps every other condition.
- **AC-07.3** Exit classes: `MINDRAIL_BUSY_RETRYABLE` is 4; the other five are 1;
  a wrong command line stays 2. No new exit code.

### REQ-08 — the command surface

- **AC-08.1** The four `lease` subcommands of design §10 exist with those names
  and flags; `lease acquire` has `--file` and no `--task`. No fifth verb.

  *Amended during TASK-04 (D-66 as amended).* `lease acquire` has `--file` and
  `--task`, exactly one of which is given.
- **AC-08.2** Every writer — the four existing and the three new — takes
  `--operation-id`; `task state` takes `--expect-revision`. A malformed id or a
  revision below 1 is refused before the application starts, exit 2, with
  "Mindrail did not run" in the impact.
- **AC-08.3** Each new command honours `--json` and emits exactly one JSON object
  on stdout, including on failure; `lease list` says `[]`, not `null`, for a
  project with no leases.
- **AC-08.4** A replayed write renders as the original did plus
  `"replayed": true` in JSON and one sentence in human output saying nothing was
  written again.
- **AC-08.5** `task show` prints the lease beside the claimant in each of the
  three states — active with the expiry, expired with the time, released with
  the time — and prints no lease line for a task that never had one.
- **AC-08.6** `lease list` and `task show` write no row (sessions, leases and
  operations counted before and after).
- **AC-08.7** The six new codes join the coordination agreement matrix, so the
  human rendering, the JSON envelope and the exit code are asserted together
  for each; for every row, carrying out the printed remedy clears the
  condition (AC-09.2 of MR-003).
- **AC-08.8** Every new command answers an uninitialised repository and a
  schema-behind repository exactly as MR-003's do (AC-06.7 of MR-003, as
  amended).

### REQ-09 — `internal/status` publishes the lease count

- **AC-09.1** The additive `leases_active` key of design §11, counting active
  leases of the project by the clock; expired rows are not counted.
- **AC-09.2** `readiness` and the six component names are unchanged for a store
  with active leases, with expired leases and with none.
- **AC-09.3** Goldens regenerated; the diff is the new key, its human line, and
  `runtime.schema_version` 2 → 3.

### REQ-10 — concurrency, proved

- **AC-10.1** Two operating-system processes race `task state --to CLAIMED` on
  one `OPEN` task; one succeeds, the other publishes `LEASE_CONFLICT` or
  `TASK_STATE_INVALID` naming the winner's session; exactly one unreleased
  `leases` row for the task afterwards, by SQL.
- **AC-10.2** The same over `lease acquire --file` on one path, and over two
  different paths — where both succeed (tech-stack §134's "different-symbol
  concurrency", at file grain).
- **AC-10.3** After the clock passes the TTL, a third session's move takes over
  and the result names the superseded tenure.
- **AC-10.4** `task open --operation-id X` twice concurrently and twice
  sequentially: one task, one operation row, the second answer `replayed`.
- **AC-10.5** A process holding `BEGIN IMMEDIATE` past a contender's budget: the
  contender's `task open` publishes `MINDRAIL_BUSY_RETRYABLE` at exit 4 within
  the budget plus one ladder step; the same command succeeds after the holder
  commits.
- **AC-10.6** Eight sessions, 25 moves each, one database, under `-race`: every
  write completes, none is busy-refused, and the longest `Held` on any `Write`
  is under 250 ms.
- **AC-10.7** Every test above is run five times in parallel with `make check`
  before it is believed, and a red under load is rerun in isolation before it
  is called a failure (MR-003's trap 4).

### REQ-11 — the test discipline both earlier milestones paid for

- **AC-11.1** Every guard is checked in both directions.
- **AC-11.2** No test classifies on a substring both the right and the wrong
  answer contain: assert the `Code`, the status, the id.
- **AC-11.3** Every new assertion is checked by mutating the code it guards and
  confirming it goes red, and the mutation is recorded in the findings document
  **after** being run — per task, before the next task begins.
- **AC-11.4** The two-process tests carry a comment saying why goroutines would
  not do, in the words `open_contention_test.go` already uses.

### REQ-12 — the non-goals hold

- **AC-12.1** No `symbol` target kind, no `Change`, no `before_change`.
- **AC-12.2** No `override`, no `lease break`, no force flag anywhere.
- **AC-12.3** No configuration key and no flag for the TTL.
- **AC-12.4** No `revision` column outside `tasks`.
- **AC-12.5** No retry loop around `BEGIN` in `InTx` (AC-01.5).
- **AC-12.6** No timing on the JSON wire.
- **AC-12.7** No change to the knowledge pipeline, its schema documents or its
  reader window.

---

## 3. Traceability to the task list's five acceptance criteria

| Task-list criterion | Held by |
|---|---|
| Two processes cannot obtain conflicting active leases on one logical target at once | REQ-02 (AC-02.1, the index) + REQ-04 (AC-04.2) + REQ-05 (AC-05.2) + REQ-10 (AC-10.1, AC-10.2) — kernel-scope AC-04 |
| A mutation repeated with the same `operation_id` produces no duplicate Task, Change or Evidence | REQ-06 + REQ-10 (AC-10.4) — kernel-scope AC-15; Change and Evidence inherit the mechanism when MR-007 and MR-010 add them |
| A stale-revision update yields an explicit conflict, not last-write-wins | REQ-05 (AC-05.4, AC-05.5) — kernel-scope AC-16 |
| Concurrency/race tests prove bounded retry and conflict behaviour | REQ-01 (AC-01.2, AC-01.3) + REQ-10 |
| Transactions stay short enough not to starve interactive writes | REQ-10 (AC-10.6) + REQ-01 (AC-01.4, the measurement) + the standing `TxActive` guard |

---

## 4. Work breakdown and dependency order

Serialised: each task is gated before the next is written. A task ends with its
stated mutation red, the suite green, a commit, a Reader over its record and a
Breaker over its change, and a row in the findings document.

| Task | Owns | REQ | Depends on |
|---|---|---|---|
| TASK-01 | `internal/storage` — `Backoff`, `MINDRAIL_BUSY_RETRYABLE` at both sites, `TxStats`; the code in the registry | REQ-01, REQ-07 (one code) | — |
| TASK-02 | `migrations/000003_lease_idempotency.sql`, the ledger's `ADD COLUMN`, `TableSchemaVersion` 3, the goldens' schema version, the shipped-file pin | REQ-02 | — |
| TASK-03 | `internal/coordination` — lease model, targets, TTL, the five lease methods, three lease codes | REQ-03, REQ-04, REQ-07 (three codes) | TASK-02 |
| TASK-04 | `Transition` and `WriteCheckpoint` with lease and revision; `STATE_REVISION_CONFLICT` | REQ-05, REQ-07 (one code) | TASK-03 |
| TASK-05 | `Operation` on every writer; `OPERATION_ID_CONFLICT` | REQ-06, REQ-07 (one code) | TASK-04 |
| TASK-06 | `internal/cli` — the `lease` group, the two flags, results, `task show`, the matrix rows | REQ-08 | TASK-05 |
| TASK-07 | `internal/status` — `leases_active`, goldens | REQ-09 | TASK-06 |
| TASK-08 | the two-process and load suites, the mutation log, the findings record, the task list's `Durum` | REQ-10, REQ-11 | TASK-07 |

TASK-01 and TASK-02 touch disjoint files; they are still run one after the
other, because the gate is the point.

---

## 5. Definition of done

1. Every acceptance criterion above is met, or is recorded as not met with the
   reason in `mr-004-findings.md`.
2. `make verify` green, `make tidy-check` green, and the test count reported
   with `go test -list '.*' ./... | grep -c '^Test'` — that command and no
   other.
3. Every new guard has a recorded mutation that turns it red (AC-11.3), recorded
   after it was run.
4. Every task in §4 has passed its Reader/Breaker gate before the next was
   begun, and the gate's findings — confirmed, refuted, unconfirmed — are in
   `mr-004-findings.md` per task, with what was done about each.
5. The task list's MR-004 entry carries a `#### Durum` block written the way
   MR-003's is: what is green, which criterion was gated on what, where the
   findings live, and the process change this milestone was run to test.
6. The knowledge graph is refreshed in its own commit at the end.
