# MR-004 — design: leases, idempotent mutations and optimistic revision

Task-list milestone: *Güvenli lease, idempotency ve optimistic revision*
(`mindrail-0.1-task-list.md`). Scenario: **two agents running at the same time
cannot silently obtain conflicting active leases on one protected target, and
cannot produce a duplicate record by retrying.**

This document fixes the package plan, the entity model, the runtime schema, the
lease rules and the command surface. The decisions it rests on (D-64…D-79) and
the acceptance criteria are in [mr-004-requirements.md](mr-004-requirements.md),
which is frozen before any code is written — the same order MR-002 and MR-003
took, for the reason MR-003's findings record: an audit needs something to grade
against that the implementation did not shape.

---

## 1. What the milestone is, in one paragraph

MR-003 made one agent's work legible to the next agent. MR-004 makes two agents
working *at once* unable to damage each other's work. Three guards, one per
acceptance criterion in the task list: a task or a file has at most one
**active lease**, so the second agent to reach a target is told who holds it and
until when, instead of both proceeding; a mutation retried with the same
**`operation_id`** — because a tool call was delivered twice, a client retried
on a lost response, or a process died after committing — happens exactly once
and the retry is answered with the recorded result; and a task move made on a
stale reading of the task is refused with the current **revision**, never
applied over what somebody else wrote in between. Underneath all three, a
writer that meets the SQLite write lock waits a bounded time and then gets a
code that says *retry*, not *broken*.

## 2. What it is not

| Not in MR-004 | Owner | Why not here |
|---|---|---|
| Symbol leases (`target_kind = symbol`) | MR-006 | There is no `symbol_uid` to lease until MR-006 allocates one. The lease table takes the kind as a value, so adding it is a constant, not a migration |
| `before_change` acquiring a lease; `Change` rows | MR-007 | `before_change` is the caller of the file/symbol lease. MR-004 builds what it calls and gives it one caller of its own, the `lease` command group, so the code is exercised before MR-007 depends on it |
| Operator lease recovery (`mindrail override … --reason`, spec §119) | out of 0.1 | No milestone in the task list owns `override`, and spec §120's approver model (allowlist, roles, signed identity) is out of 0.1 by kernel-scope §4. Recovery is expiry: a lease outlives its holder by at most 20 minutes (D-70, user decision) |
| Configurable lease TTL | deferred | Spec §59 gives one default and no knob. A constant that nothing reads from config is the smallest thing that satisfies it (D-69) |
| Deterministic operation ids derived from request content (tech-stack §23's `after_change`, validation runs, imports) | MR-007, MR-010, MR-015 | Those operations do not exist yet. MR-004 takes the id from the caller and defines what the store does with it |
| `revision` on entities other than `tasks` | MR-007 (`Change`) | A lease's mutable columns are functions of its holder and the clock, and every lease write is refused by identity before it could be stale (D-72) |
| A second busy-retry ladder around `BEGIN IMMEDIATE` | — | SQLite's own busy handler runs for the write lock and for a stale snapshot alike from a fresh `BEGIN IMMEDIATE`; a BUSY that leaves `BEGIN` has already consumed the budget (D-74) |
| A cross-process write broker | — | Spec §11 says no, and says why |
| MCP tools | MR-015 | The store's writers are what `mindrail_claim` and `mindrail_before_change` will call |
| Wait/hold telemetry on the wire | MR-019 | MR-004 measures every write transaction's wait and hold and logs them; the wire shape is MR-019's |
| A `doctor` check that decodes coordination rows | unowned | Recorded by MR-003 §7 as belonging to "MR-004's doctor work" if there were any; there is none. It stays recorded |

## 3. Package plan

```text
internal/storage         backoff.go — the jittered ladder; tx.go names the
                         budget-exhausted BEGIN and measures every transaction
internal/migration       columns.go learns `ALTER TABLE … ADD COLUMN`
internal/coordination    lease.go (model, target kinds, statuses), the lease
                         methods on Store, operation.go (idempotency),
                         revision on Task; Transition and WriteCheckpoint gain
                         lease and revision semantics
internal/cli             lease.go — four commands; --operation-id on every
                         writer, --expect-revision on `task state`
internal/status          one additional count in the coordination block
migrations/              000003_lease_idempotency.sql
```

The tech-stack §7 layout lists `lease/` as its own package beside `session/`,
`task/` and `checkpoint/`. MR-003 put those three in `internal/coordination`,
and the lease joins them there for the reason that decided it then: **the task
lease is written in the transaction that moves the task.** A package boundary
through the middle of one `BEGIN IMMEDIATE` is a boundary through nothing — the
two halves would have to share a `*sql.Tx`, and the lifecycle rules of §7 would
be stated across two packages that cannot be tested apart.

`internal/coordination`'s import allow-list (`internal/app`, `internal/storage`,
`internal/identity`, standard library) does not change. The idempotency hash
uses `crypto/sha256` and `encoding/json`, both standard library.

## 4. Entity model

```text
Project ─┬─< Task ──< Checkpoint >── Session
         │    ╎                        │
         │    ╎ (kind = task)          │
         └─< Lease ──── holder ────────┘
              (kind = file)

Operation: operation_id ──> recorded result
```

- **Lease** — one *tenure*: one session holding one target from an acquisition
  to a release. The target is a pair, `(kind, key)`: `task` with the task id, or
  `file` with a repository-relative path. A lease is **active** while it has
  not been released and has not expired. It is never updated into a different
  tenure: a takeover after expiry closes the old row and opens a new one, so the
  table is the lease history spec §59 asks for, one row per tenure (D-64,
  D-65).
- **Task** gains `revision`, an integer that starts at 1 and rises by one on
  every update of the row. A caller that read the task at revision *n* and asks
  for a move "expecting *n*" is refused if the row has moved on (D-72).
  `claimed_by` keeps its MR-003 meaning — which session claimed — and the lease
  is what makes the claim enforceable; §7 says exactly when the two agree.
- **Operation** — a caller-supplied id, the command it named, a hash of the
  request, and the result the command produced. It exists so that the second
  delivery of one request is answered from the first (D-71).
- **Session** and **Checkpoint** are unchanged. A session holds leases; a
  checkpoint written by the holder renews the task lease, and a handoff
  checkpoint releases it (D-78).

## 5. Runtime schema — `migrations/000003_lease_idempotency.sql`

```sql
CREATE TABLE leases (
    lease_id       TEXT PRIMARY KEY,
    project_id     TEXT NOT NULL REFERENCES projects(project_id),
    target_kind    TEXT NOT NULL,
    target_key     TEXT NOT NULL,
    holder         TEXT NOT NULL REFERENCES sessions(session_id),
    acquired_at    TEXT NOT NULL,
    renewed_at     TEXT NOT NULL,
    expires_at     TEXT NOT NULL,
    released_at    TEXT,
    release_reason TEXT
) STRICT;

CREATE UNIQUE INDEX idx_leases_active
    ON leases(project_id, target_kind, target_key)
    WHERE released_at IS NULL;
CREATE INDEX idx_leases_holder ON leases(holder);

CREATE TABLE operations (
    operation_id TEXT PRIMARY KEY,
    command      TEXT NOT NULL,
    request_hash TEXT NOT NULL,
    result       TEXT NOT NULL,
    recorded_at  TEXT NOT NULL
) STRICT;

ALTER TABLE tasks ADD COLUMN revision INTEGER NOT NULL DEFAULT 1;
```

Four things about this file are decisions rather than habits.

**The partial unique index is the guard, and the Go code is the explanation.**
`idx_leases_active` makes it impossible for two unreleased rows to name one
target in one project, whatever the code does. The store reads the target's
row inside its transaction and refuses by name before it inserts, so a user
meets `LEASE_CONFLICT` with the holder and the expiry, never a constraint
message — but the constraint is what the two-process test relies on, because it
holds even for a writer that never ran the read.

**`expires_at` is `TEXT` like every timestamp in this schema, and is never
compared in SQL.** Decision D-59 already recorded why: `app.FormatTime` writes
RFC 3339 with trailing zeros trimmed, so the column is not lexicographically
ordered — `…00Z` sorts after `…00.1Z` and is earlier. Every expiry judgment
parses the value and compares in Go, against the store's clock (D-65). A
`WHERE expires_at > ?` anywhere in this package is a defect.

**`revision` is added with `ALTER TABLE`, and the ledger learns to read it.**
Recreating `tasks` would mean dropping a table `checkpoints.task_id` points a
foreign key at, under `foreign_keys = 1`, inside a transaction where the
pragma cannot be turned off.
`ADD COLUMN` with a non-null default is the one form of `ALTER` that adds
without moving anything, and STRICT tables accept it. The cost is in
`internal/migration`: its column ledger forgets any table an `ALTER` touched,
because modelling `ALTER` in general is the schema diff the package refuses to
be — so today this line would silently remove `tasks` from the shape check
finding F9 introduced. MR-004 teaches the ledger the one form it can read
exactly, `ADD COLUMN <name>`, and keeps forgetting for every other form
(D-73).

**`000001_initial.sql` and `000002_coordination.sql` are not edited**, for the
reason D-53 gave and `migrations/shipped_test.go` now enforces: the migrator
checksums what it applied. The comment in `000002` that attributes a query to
`task show` stays wrong, and `migrations/README.md` stays the place that says
so.

There is no `lease_events` table. A row of `leases` *is* the event: acquired
at, renewed at, and how the tenure ended. Renewals collapse into `renewed_at`
rather than accumulating a row per write, because every holder write would
otherwise leave one, and a table that grows with every command is a log rather
than a history.

## 6. Lease rules

### Targets

| Kind | Key | Acquired by |
|---|---|---|
| `task` | the task id | `task state` — and only that (D-66) |
| `file` | repository-relative path, forward slashes, no leading `./`, no `..` segment, not absolute, not empty (D-77) | `lease acquire --file` |

The file need not exist. An agent about to create a file leases the path; a
lease on a path is a statement about who is writing there, not about what is
there. A key that fails the rule is a usage error at the command line, and the
store refuses it the same way — the store is the surface MR-015's tools call
without a command line in front of it.

### One session, one target, one active lease

| Situation on the target | `acquire` (file) | `renew <id>` | `release <id>` |
|---|---|---|---|
| no lease | insert | `LEASE_NOT_FOUND` | `LEASE_NOT_FOUND` |
| active, held by **this** session | renew it, same id | renew: `expires_at = now + TTL` | release: `released_at = now`, reason `released` |
| active, held by **another** session | `LEASE_CONFLICT` | `LEASE_CONFLICT` | `LEASE_CONFLICT` |
| expired, not released | close it (reason `expired`) and insert; the result names the tenure it superseded | `LEASE_NOT_HELD` (expired) | `LEASE_NOT_HELD` (expired) |
| released | insert | `LEASE_NOT_HELD` (released) | `LEASE_NOT_HELD` (released) |

"Active" is decided against the store's clock at the moment of the write,
inside the transaction. The TTL is `LeaseTTL = 20 * time.Minute` (spec §59,
D-69).

Two rows of that table carry the milestone's scenario. The **conflict** row is
AC-04 of the kernel scope: the second agent is told the holder, the lease id and
the expiry, and the remedy is to wait until then or to ask the holder to release
— it is never told to force. The **expired** row is the recovery path: the
holder crashed twenty minutes ago and its lease is taken over by whoever moves
next, and the result says so out loud, naming the session and the time it
expired (D-67, D-70). Neither row is a silent outcome.

Reads mark nothing. `lease list`, `task show` and `status` judge expiry against
the clock and report it; an expired row stays unreleased until an acquirer
closes it (D-79). A read that wrote would be the log of curiosity D-56 kept out
of the sessions table.

## 7. The task lifecycle with a lease

`task state --to` remains the only mover, and the state table of decision D-55
is unchanged: same seven states, same transitions, same `TASK_STATE_INVALID`.
What changes is that the move now runs three judgments before the table, in
this order, all inside the one `BEGIN IMMEDIATE` transaction:

1. **Revision** (D-72). If the caller passed `--expect-revision n` and the row is
   not at *n*, the move is `STATE_REVISION_CONFLICT`, naming the revision and
   state the task actually has. Nothing else is looked at: a caller whose
   reading is stale should not be told about the lease as if its reading were
   current.
2. **Lease** (D-67). The task's unreleased lease row is read and classified.
   Active and held by another session: `LEASE_CONFLICT`, before the state table
   — the reason the move cannot happen is ownership, and a state refusal would
   read the same whether the lease had expired or not.
3. **State table** (D-55) and the blocked-reason rule (D-63), exactly as today.

Then the effect, by destination:

| Destination | Lease effect | `claimed_by` |
|---|---|---|
| `CLAIMED` (from `OPEN`) | acquire for the mover | the mover |
| `IN_PROGRESS`, `BLOCKED`, `READY_TO_COMPLETE` | held by the mover: renew. Expired or absent: acquire for the mover (an expired tenure is closed with reason `expired`, and the result names it) | the mover |
| `OPEN` (from `CLAIMED`) | release, reason `released` — or `expired`, when the tenure had already run out | cleared |
| `COMPLETED`, `ABANDONED` | release the mover's own tenure with reason `finished`, or an expired one with reason `expired`; nothing to release is not an error | unchanged |

The invariant that falls out, and that a test walks every transition to check
(D-68): **an `OPEN` task has no active lease, and a task with an active lease
has `claimed_by` equal to its holder.** `claimed_by` on a task whose lease has
expired or been released is attribution of the last claim — `task show` prints
it that way, with the tenure's end and why — and the next move rewrites it.
This amends D-58, which said only a claim or a release touches `claimed_by`: a
takeover is a claim.

The row is written as tech-stack §22 writes it —
`UPDATE tasks SET …, revision = revision + 1 WHERE task_id = ? AND revision = ?`
with the revision read at step 1 — and an update that affects no row is
`COORDINATION_WRITE_FAILED`, not a second conflict: the transaction holds the
write lock, so the row cannot have moved, and zero rows here is a defect.

**Checkpoints** (D-78). A checkpoint written by the session that holds the
task's active lease renews it; with `--handoff` it releases it, reason
`handoff` — the first behaviour `--handoff` has had, and the one spec §99's
`mindrail_checkpoint(handoff=true)` describes: *I am leaving; the next agent
may take this*. A checkpoint by any other session is written and touches no
lease. A note is not a claim, and MR-003's scenario — the arriving agent's
first act is often to write "picking this up" — must not be refused for the
lease it is about to take over.

The handover of MR-003, replayed under MR-004: agent A claims and works, holding
the lease; A writes `checkpoint write --handoff` and exits — the lease is
released; agent B, a new session, moves the task to `IN_PROGRESS` and acquires
it. If A crashed instead of handing off, B's first move within twenty minutes is
`LEASE_CONFLICT` naming A and the expiry; after it, the same move takes over and
says so.

## 8. Idempotent mutations

Every writer in the store — `OpenSession`, `OpenTask`, `Transition`,
`WriteCheckpoint`, `AcquireLease`, `RenewLease`, `ReleaseLease` — takes an
`Operation`. The zero value means the write is not idempotent and runs as it
does today. A non-zero id makes the transaction do this first:

```text
BEGIN IMMEDIATE
  SELECT command, request_hash, result FROM operations WHERE operation_id = ?
  found, hash equal      → decode result, return it marked replayed; no other
                            statement runs
  found, hash different  → OPERATION_ID_CONFLICT; nothing is written
  not found              → perform the write
                           INSERT INTO operations (…, result, …)
COMMIT
```

Three properties follow from putting the lookup and the record in the *same*
transaction as the write, and they are the whole of AC-15:

- **Two concurrent deliveries serialise.** The second waits at `BEGIN
  IMMEDIATE` and then finds the first's record. There is no window in which
  both find nothing.
- **A failed operation records nothing.** The record is inserted after the
  write, in its transaction; a refusal rolls both back, and a later retry of a
  refused operation runs fresh. Recording failures would freeze a transient
  refusal — a lease held for another minute — into a permanent one.
- **A process killed after `COMMIT` and before it printed** is the case
  tech-stack §23 names as "process interruption": the retry finds the record
  and prints what the dead process would have.

The result stored is the store's own result type, marshalled as JSON, and the
command rebuilds its output from it. A replay therefore renders exactly as the
original did — the same task, the same session, the same `session_minted` —
plus `"replayed": true`, which is the one thing the caller must be able to see:
it tells an agent that the state it is looking at is as of the first delivery,
not as of now.

`request_hash` is SHA-256 over a canonical JSON encoding of the command name and
the store parameters that reach it: for `task open`, the project, the session
handle or the workspace a session is minted for, and the title; for `task
state`, the task id, the handle, the destination, the reason and the expected
revision; and so on. It is computed by the store, not by the command, so
MR-015's tools get the same rule. `--session` is part of it: a retry of one
request carries the same handle, and a "retry" with a different one is a
different request wearing the same id, which is what `OPERATION_ID_CONFLICT`
exists to say.

An `operation_id` matches `^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`. It is an
identifier the caller minted — a ULID, a UUID, a counter — and anything else is
a usage error before the application starts. There is no retention in 0.1:
the table holds one row per idempotent mutation, as `checkpoints` holds one per
note (D-71).

## 9. The bounded wait for the write lock

Today a writer that meets the lock waits inside SQLite's busy handler for up to
`busy_timeout` (5 s, decisions D-08 and D-24) and is then refused with
`RUNTIME_DB_UNAVAILABLE` and `metadata.condition = "locked"`; a process that
meets the lock while opening a database another process is still converting to
WAL polls every 20 ms for the same budget, and is then refused with the same
code under the generic open remedy — "check that the Git common directory
exists and is writable", which names neither the lock nor the wait. Both are
bounded; neither is named as what it is, and the second is the flat poll
`storage.openPollInterval`'s comment leaves for this milestone.

MR-004 changes three things and deliberately not a fourth (D-74).

**The code.** Both refusals become `MINDRAIL_BUSY_RETRYABLE`, spec §11's name,
which decision D-18 reserved under the `MINDRAIL_` prefix as the one
infrastructure condition a domain command reports. It keeps exit 4
(`KindUnavailable`) and `condition = "locked"`, and gains `waited_ms`. The
remedy is unchanged: run the command again. `RUNTIME_DB_UNAVAILABLE` keeps
every other condition it names today; what it loses is the one whose remedy is
"nothing is wrong, wait".

**The ladder.** `storage.Backoff` steps 25, 50, 100, 200, 400 ms and then holds
at 400, each step drawn from `[step/2, step]` so that *n* processes refused
together do not retry together. It replaces the flat poll at the open path,
inside the same 5 s budget. A test drives it with a fixed source and reads the
steps back.

**The measurement.** `InTx` records how long `BEGIN` waited and how long the
transaction held the lock, and hands both to a caller that asked for them
(`storage.WithTxStats`). The coordination store asks on every write, carries
them on `Write`, and the command logs them at DEBUG — spec §11's "p95 write
wait" and "write transaction duration", collected from the first
implementation as kernel-scope §3 requires, without a wire shape MR-019 has
not designed yet (D-75).

**Not a second ladder at `BEGIN`.** The spec's example ladder sums to 775 ms
and reads as an application retry around a short per-attempt wait. That is
not the shape this codebase has: `_txlock=immediate` makes `BEGIN` take the
write lock at once, and SQLite invokes the busy handler for the write lock and
for a stale snapshot alike when the transaction starts from nothing — so a
`SQLITE_BUSY` that reaches Go from `BEGIN` has already waited the whole budget,
and a ladder on top of it would wait five seconds five more times, past the
shutdown budget D-08 fixed at the same number. The wait stays SQLite's, which
is itself a stepped backoff (1 → 100 ms); what MR-004 adds is the name, the
jitter where jitter can matter, and the number.

## 10. Command surface

Four new commands in one group, and two flags on the existing writers.

```bash
mindrail lease acquire --file <path> | --task <task-id>  [--session <id>] [--operation-id <id>]
mindrail lease renew   <lease-id>    [--session <id>] [--operation-id <id>]
mindrail lease release <lease-id>    [--session <id>] [--operation-id <id>]
mindrail lease list                                   # active leases, any kind

mindrail session open     …  [--operation-id <id>]
mindrail task open        …  [--operation-id <id>]
mindrail task state       …  [--operation-id <id>] [--expect-revision <n>]
mindrail checkpoint write …  [--operation-id <id>]
```

`lease acquire --task <id>` is the claim without a move (D-66 as amended
during TASK-04): a task already in a working state is taken where it stands,
the holder becomes the claimant, and the state does not change. It exists
because the handover in §7 releases the lease of a task that is almost always
`IN_PROGRESS`, and D-55 has no move that keeps a task `IN_PROGRESS`. An
`OPEN` task is claimed by moving it to `CLAIMED`, and `--task` refuses it with
that remedy. `lease renew` and `lease release` take any lease id, task or
file, and are refused by identity for anyone but the holder — they are how an
agent running a thirty-minute test suite keeps its task, and how one steps
away without a note.

Every writer honours `--json` and the standard envelope, mints a session when
`--session` is absent and says so, and refuses an unrepresentable text flag
before starting, exactly as MR-003's do. `--operation-id` and
`--expect-revision` are judged before the application starts: a malformed id
or a non-positive revision is a mistake in the command line.

What the writers publish gains three things: the task's `revision`; the
`lease` the write left active on the target, or `null`; and `replayed`, true
only when the operations table answered. `task state` also publishes the
tenure it took over, when it did. `task show` prints the lease beside the
claimant: *claimed by SES-…, lease active until …* or *… lease expired at …*
or *… released at …*.

## 11. Status integration

`status --json`'s coordination block gains one additive key:

```json
"coordination": {
  "observation": "observed",
  "tasks_open": 1,
  "tasks_in_progress": 2,
  "tasks_blocked": 0,
  "leases_active": 2,
  "last_checkpoint": {"task_id": "TSK-…", "session_id": "SES-…", "at": "…"}
}
```

Spec §68 lists leases among what `mindrail_status` returns; a count is the
fixed-size form of it. `readiness` and the six components do not move (D-62
holds). The human rendering gains one line, `Leases active`, and the goldens are
regenerated and read key by key; the only other change in them is
`runtime.schema_version` 2 → 3 (D-73).

## 12. Test plan

The acceptance tests are the two-process cases tech-stack §93 and §134 name,
driven through the real command tree and, for the lease race, through two
operating-system processes over one database — because SQLite's locks are per
process and two goroutines in one process share connection state
(`open_contention_test.go` already re-executes the test binary for this reason,
and the same dispatch is reused):

1. **Single lease winner.** Two processes race `task state --to CLAIMED` on one
   `OPEN` task, and two race `lease acquire --file` on one path. Exactly one of
   each pair succeeds; the other publishes `LEASE_CONFLICT` naming the winner's
   session and expiry, or `TASK_STATE_INVALID` naming the claimant when the
   winner's commit landed first — and the leases table holds exactly one
   unreleased row for the target afterwards, asserted by SQL.
2. **Stale lease recovery.** The clock is advanced past the TTL; a third
   session's move succeeds, the result names the superseded tenure, and the old
   row is closed with reason `expired`.
3. **Idempotent retry.** The same `task open --operation-id X` runs twice
   concurrently and twice sequentially; one task exists, the second answer is
   the first answer plus `replayed: true`. The same id with a different title is
   `OPERATION_ID_CONFLICT`. A `task state` whose process is killed between
   `COMMIT` and output is simulated by running the store directly and then the
   command: one move, one record.
4. **Revision conflict.** Two sessions read a task at revision *n*; the first
   moves it; the second's move with `--expect-revision n` is
   `STATE_REVISION_CONFLICT` naming *n+1* and the row is unchanged, asserted by
   re-reading it.
5. **Bounded busy wait.** A process holds `BEGIN IMMEDIATE` for longer than a
   contender's budget; the contender's `task open` publishes
   `MINDRAIL_BUSY_RETRYABLE` at exit 4 within budget plus one ladder step, and
   the same command succeeds once the holder commits. The open path is asserted
   the same way through the existing WAL-conversion race, now on the ladder.
6. **Short transactions.** Eight sessions write 25 moves each against one
   database under `-race`; every write completes, none is refused as busy, and
   the longest hold recorded on any `Write` is under 250 ms. The bound is
   chosen to be about starvation rather than about the machine; the cost of
   one write it is compared against is measured and recorded in TASK-08.

Around them: every row of §6's table in both directions, every destination of
§7's table with the D-68 invariant checked after each, the D-55 table
re-walked (49 pairs) to show the lease changed no transition, the agreement
matrix rows for the six new codes, and the round trip through `status --json`.

Each task of the work breakdown is gated before the next begins — a Reader over
its record and a Breaker over its change — and records the mutation that turns
its new test red, in the findings document, after running it.
