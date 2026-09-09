# MR-003 — Frozen requirements and acceptance criteria

- **Frozen at:** commit `fd8d958`, branch `mr-002-knowledge-lifecycle` (MR-003
  work branches from here)
- **Contract this refines:** [mr-003-design.md](mr-003-design.md). Where the two
  disagree, the design wins on intent and this document wins on the detail the
  design left open — every such gap is recorded below as a decision D-53…D-62.
- **Task:** [mindrail-0.1-task-list.md](mindrail-0.1-task-list.md) MR-003
- **Tier:** 3 (Program). It adds a runtime migration, three new persisted
  entities, five `app.Code` values that enter the wire vocabulary, three new
  command groups and an additive key in `status --json`. The escalation rule
  fires on "public API contract" regardless of line count.

This file is frozen **before** implementation so that an audit has something to
grade against that the implementation did not shape. That sequence is not
ceremony here: it is the one process change that made MR-002's fourth round come
back with no defect in behaviour.

---

## 0. Baseline evidence

Captured on the tree this work starts from, so a red suite later is unambiguous.

| Measurement | Value | Command |
|---|---|---|
| Commit | `fd8d958` | `git rev-parse HEAD` |
| Go toolchain | `go1.27.0 linux/amd64` | `go version` |
| Packages | 18 | `go list ./...` |
| Top-level test functions | 732 | `go test -list '.*' ./...` |
| `make verify` | green — gofmt, `go vet`, all packages `ok`, race detector all `ok`, smoke `ok` | `make verify` |
| `make tidy-check` | green | `make tidy-check` |
| Migrations | `000001_initial.sql` only; runtime `schema_version` reports 1 | — |
| `go.mod` direct requires | `go-toml/v2 v2.4.3`, `jsonschema/v6 v6.0.3`, `cobra v1.10.2`, `modernc.org/sqlite v1.58.0` | — |

MR-003 adds **no** module dependency. `make tidy-check` must still be run
explicitly and reported, because it is in neither `check` nor `verify`.

---

## 1. Decisions this document freezes

MR-002 ended at D-52. These continue the sequence. Each one existed as an
ambiguity two implementers would have resolved differently.

### D-53 — the coordination tables are a second migration, and the reported runtime schema version becomes 2

`migrations/000002_coordination.sql` is added; `000001_initial.sql` is not
edited. The migrator is forward-only and checksums what it applied, so editing
the first file would make every existing database report a checksum mismatch —
which is the loudest possible failure for a change that was meant to be additive.

The visible consequence is named here so it is a decision rather than a surprise:
**`status --json`'s `runtime.schema_version` changes from 1 to 2**, and so does
the same value in `init --json` and in doctor's rendering. Exactly two places
record the old value today — the assertion at `internal/status/report_test.go:69`
and the golden `internal/status/testdata/init_ready_json.golden` — because the
two CLI goldens carry key paths rather than values and `internal/migration`'s
tests build their own synthetic migration sets. Both are updated as part of the
change, not worked around.

`readable_schema_versions` and `write_schema_version` are the **knowledge**
record window and are untouched. The two numbers are unrelated and the report
prints both; a change to one that silently moved the other would be the defect
this paragraph exists to prevent.

### D-54 — "Checkpoint" means two things in this repository, and neither is renamed

`internal/storage/checkpoint.go` is the SQLite **WAL checkpoint**: writing the
write-ahead log back into the main database file. It is MR-001 code, it is
correct, and the name is SQLite's own.

MR-003's `coordination.Checkpoint` is an agent handover record.

Neither is renamed and the two never appear in one package. A reader who greps
for "checkpoint" and finds both is meant to find this decision with them.

### D-55 — the task state machine is a total function, stated once, and an illegal transition is a named refusal

Spec §58's seven states, and the transitions that exist:

| From | To |
|---|---|
| `OPEN` | `CLAIMED`, `ABANDONED` |
| `CLAIMED` | `IN_PROGRESS`, `OPEN`, `ABANDONED` |
| `IN_PROGRESS` | `BLOCKED`, `READY_TO_COMPLETE`, `ABANDONED` |
| `BLOCKED` | `IN_PROGRESS`, `ABANDONED` |
| `READY_TO_COMPLETE` | `COMPLETED`, `IN_PROGRESS`, `ABANDONED` |
| `COMPLETED` | — terminal |
| `ABANDONED` | — terminal |

Everything not in that table is refused with `TASK_STATE_INVALID`, and the
message names the state the task is in, the state that was asked for, and the
transitions that *are* available from where it is.

It is **never** a silent no-op, and a transition to the state a task is already
in is refused rather than treated as idempotent. MR-004 owns idempotency and
will do it with an `operation_id`; a self-transition quietly reporting success
now would be a second, weaker idempotency rule that MR-004 would have to
contradict.

The table lives in one place and every writer goes through it. A second copy —
in the CLI's flag validation, say — is how the two would come to disagree about
what `BLOCKED` may become.

### D-56 — a session is minted, never resumed; continuity lives in the Task and the Checkpoint

The milestone's scenario is *two different* AgentSessions. The second agent gets
a **new** session id and continues the **same** task. Nothing looks a session up
to reattach to it, and there is no "current session" stored anywhere.

That is what makes the handover survive the things it has to survive: a process
exit, a tool change, a machine restart. A design that resumed sessions would put
the continuity in the least durable of the three entities.

Sessions are minted by commands that **write** and by `session open`. A read
mints nothing: a command that created a row just by asking a question would make
the session table a log of curiosity rather than of work.

### D-57 — the id minter moves to `internal/identity`

`workspace.NewID(prefix)` is generic in everything but its package. MR-003 needs
three more prefixes (`SES`, `TSK`, `CKP`) and MR-004, MR-006 and MR-011 need at
least four more. The file and its tests move to `internal/identity`;
`internal/workspace` imports it and keeps `projectIDPrefix` and
`workspaceIDPrefix` as its own constants.

It is a move, not a rewrite: the ULID layout, the Crockford alphabet, the
monotonic-within-a-millisecond entropy and the clamp on a backwards clock are
unchanged, and the tests that hold them move with the code.

### D-58 — `CLAIMED` is a state, not a lease

MR-003 records **which** session claimed a task. It records no expiry, renews
nothing, and refuses nothing on the grounds of time. A second claim on a claimed
task is refused by D-55's table — `CLAIMED` has no transition to `CLAIMED` — and
the message says the task is already claimed and by which session.

MR-004 adds the lease. It will add expiry, renewal and contention on top of this
column; it will not have to unpick a half-built lease first. A 20-minute default
appears nowhere in MR-003.

### D-59 — checkpoints are append-only, and "the last checkpoint" is the newest id

A checkpoint is never updated and never deleted. The newest is selected by
`checkpoint_id DESC`, not by `created_at DESC`: the id carries a 48-bit
millisecond prefix and is monotonic within a millisecond, so it orders two
checkpoints written in the same millisecond and a timestamp column does not.

`--handoff` marks a checkpoint as the one the writer left on the way out (spec
§99's `mindrail_checkpoint(handoff=true)`). It is recorded and reported; in
MR-003 it changes no behaviour, and no code branches on it. It is persisted now
because retrofitting a column onto rows already written would mean either a
migration with a default that lies or a nullable column nobody can interpret.

### D-60 — a task belongs to a project, not to a workspace

Spec §61 shares task ownership at project level so that Mode B's two worktrees
see one task. MR-003 exercises one workspace, but the column is chosen for the
final shape: moving `tasks.workspace_id` to `tasks.project_id` in MR-004 would be
a migration over rows a user already has.

A **checkpoint** records the workspace as well as the session, because "which
worktree was this written from" is a real question in Mode B and the answer is
only knowable at write time.

### D-61 — the session handle is supplied by the caller, and a missing one is minted and reported

Spec §98 makes `mindrail_session_id` an explicit application handle carried by
the caller rather than by the connection; that is what lets an agent change tools
mid-task. `--session <id>` is that handle at the CLI.

A write with no `--session` mints a session and **says so in the result**, so a
human at a terminal is not forced to run `session open` first and an agent that
wants continuity has the id it must pass back. A `--session` naming a session
that does not exist is refused with `SESSION_NOT_FOUND`; it is not silently
minted, because that would turn a typo into a second agent identity.

### D-62 — coordination never changes `readiness`, and never changes a component

`status` publishes coordination as an additive top-level block beside
`knowledge`, `runtime` and `workspace`. It does not add a seventh component, does
not rename any of the six, and cannot move `readiness`.

A blocked task is a fact about work, not about the installation. A tool that
reported `BLOCKED` readiness — the value reserved for "this repository cannot be
verified" — because an agent parked a task would be unusable in exactly the
situation the task was parked for.

---

## 2. Requirements

### REQ-01 — `internal/identity`: the shared id minter

- **AC-01.1** `identity.NewID(prefix string) string` returns `prefix + "-" + 26`
  Crockford base32 characters, the ULID layout, monotonic within a millisecond.
- **AC-01.2** Every test that held `workspace.NewID`'s properties moves with the
  code and still passes: the alphabet excludes `I`, `L`, `O`, `U`; two ids minted
  in one millisecond sort in mint order; a clock that steps backwards does not
  produce a descending id.
- **AC-01.3** `internal/workspace` produces byte-identical id shapes to those it
  produced at `fd8d958`, asserted by a test that mints one of each prefix and
  matches it against the pattern.
- **AC-01.4** `internal/identity` imports nothing from this module.

### REQ-02 — `migrations/000002_coordination.sql`

- **AC-02.1** The three tables and three indexes of design §5 exist, `STRICT`,
  with the foreign keys as written.
- **AC-02.2** `000001_initial.sql` is byte-identical to its content at `fd8d958`.
- **AC-02.3** A database created at `fd8d958` and opened by this binary applies
  exactly migration 2 and reports `schema_version` 2. A fresh database applies
  both and reports 2.
- **AC-02.4** The migration's own verification path — the object and column
  ledger `internal/migration` builds — names the three new tables, so a database
  missing one of them is reported as damaged rather than as healthy.

### REQ-03 — `internal/coordination`: the domain

- **AC-03.1** `Session`, `Task` and `Checkpoint` are exported structs with JSON
  tags matching design §5's column names.
- **AC-03.2** `State` is a string type with the seven spec §58 values as
  constants, and a `ParseState` that refuses anything else.
- **AC-03.3** The transition table of D-55 is one exported function,
  `CanTransition(from, to State) bool`, and one table literal. There is no second
  copy anywhere in the module — asserted by a test that walks all 49 ordered
  pairs and compares against an independently written expectation.
- **AC-03.4** Every legal transition is exercised and succeeds; every illegal one
  is exercised and is refused with `TASK_STATE_INVALID`. Both arms, all 49 pairs.
- **AC-03.5** The package imports `internal/app`, `internal/storage`,
  `internal/identity` and the standard library, and nothing else from this module.

### REQ-04 — `internal/coordination.Store`: persistence

- **AC-04.1** `OpenSession`, `OpenTask`, `Transition`, `WriteCheckpoint`,
  `FindTask`, `ListTasks`, `LastCheckpoint` and `Counts`, each taking a
  `context.Context` and each a single short transaction (tech-stack §11).
- **AC-04.2** A task written by one `Store` is read back with every field intact
  by a second `Store` over a **reopened** database. This is the restart criterion
  and it is asserted on the reopened handle, not on a cached value.
- **AC-04.3** `Transition` refuses an illegal move without writing, and the row
  is unchanged afterwards — asserted by re-reading it, not by trusting the error.
- **AC-04.4** `LastCheckpoint` returns the newest by `checkpoint_id`, proved by a
  fixture whose checkpoints share a `created_at` value.
- **AC-04.5** A write failure is reported through `storage.WriteFailure` first,
  exactly as `workspace.Store.registrationFailure` does, so an unwritable runtime
  database gets the remedy that can succeed rather than a generic one.
- **AC-04.6** Every error carries a registered `app.Code`, a diagnostic, an
  impact and at least one `next_action` that names the offending id.

### REQ-05 — the code vocabulary

- **AC-05.1** Five values are added to `app.Code` and to the registry:
  `SESSION_NOT_FOUND`, `TASK_NOT_FOUND`, `TASK_STATE_INVALID`,
  `CHECKPOINT_NOT_FOUND`, `COORDINATION_WRITE_FAILED`.
- **AC-05.2** No existing `app.Code` value changes spelling or meaning.
- **AC-05.3** Each new code maps to exit 1 (`ExitFailed`) except where the
  command line itself is wrong, which stays exit 2 through the existing usage
  path. No new exit code is introduced.

### REQ-06 — the command surface

- **AC-06.1** The six subcommands of design §7 exist, with those exact names and
  flags. No seventh verb.
- **AC-06.2** Each honours `--json` and emits exactly one JSON object on stdout,
  the standard envelope, including on failure.
- **AC-06.3** `task show` on a task with checkpoints returns the task, its state,
  the newest checkpoint and the session id that wrote it, in one call.
- **AC-06.4** `task state --to` with a value outside the seven states is a usage
  error (exit 2) naming the accepted values; with a legal value that is an
  illegal transition it is `TASK_STATE_INVALID` (exit 1).
- **AC-06.5** A write with no `--session` succeeds, mints a session and reports
  the minted id in the result — human and JSON both.
- **AC-06.6** `task show` and `task list` write no row. Asserted by counting the
  sessions table before and after.
- **AC-06.7** Every new command works in an uninitialised repository the way the
  existing ones do: `WORKSPACE_NOT_INITIALIZED` with the `mindrail init` remedy,
  never a panic and never a half-written row.

### REQ-07 — `internal/status` publishes the coordination state

- **AC-07.1** The additive `coordination` block of design §8, with `observation`
  following the existing `observed` / `not_observed` convention.
- **AC-07.2** `readiness` is unchanged for every store of tasks, including one
  with a blocked task and one with none — both arms (D-62).
- **AC-07.3** The six component names and their meanings are unchanged.
- **AC-07.4** Goldens are regenerated and the diff is reviewed key by key; the
  only changes are the additive block and `runtime.schema_version` 1 → 2.

### REQ-08 — `internal/bootstrap` wires it once

- **AC-08.1** The coordination store is constructed in the startup sequence
  beside the workspace store, once, and handed to the commands. No command
  constructs its own.
- **AC-08.2** A failure to construct it is reported as a defect in the binary —
  the existing `STARTUP_INCOMPLETE` path — never as a repository condition.

### REQ-09 — the CLI agreement matrix

- **AC-09.1** The new failure conditions join the existing agreement matrix, so
  the human rendering, the JSON envelope and the exit code are asserted together
  for each: unknown task, unknown session, illegal transition, uninitialised
  repository.
- **AC-09.2** For every row, carrying out the printed `next_action` clears the
  condition. A remedy that cannot be performed by the reader of the file it names
  is a defect (both milestones have produced one).

### REQ-10 — the test discipline both earlier milestones paid for

- **AC-10.1** Every guard is checked in **both** directions: it does not fire on
  the adjacent healthy condition, and it still fires on the condition it exists
  for.
- **AC-10.2** No test classifies on a substring both the right and the wrong
  answer contain. Assert the `Code`, the state, the id — never a sentence
  fragment.
- **AC-10.3** No assertion compares a function to the constant it reads. Every
  new assertion is checked by mutating the code it guards and confirming it goes
  red, and the mutation is recorded.
- **AC-10.4** The two-agent handover is asserted over a **reopened** database, in
  one test, driven through the real command tree.
- **AC-10.5** Both record kinds of the state table — the legal and the illegal
  pairs — are enumerated rather than sampled. 49 ordered pairs is small enough
  that sampling would be a choice to look at less than everything.
- **AC-10.6** Two commands running against one store concurrently keep the
  property MR-002's coverage pass established: no data race, and a coherent
  envelope. The new writers make this a stronger claim than it was, and it is
  asserted under `-race`.

### REQ-11 — the non-goals hold

- **AC-11.1** No lease: no expiry column, no renewal path, no time-based refusal.
- **AC-11.2** No `operation_id`, no revision column, no optimistic concurrency.
- **AC-11.3** No `Change`, no `Evidence`, no reconcile.
- **AC-11.4** No MCP surface. The domain package is what MR-014 will call.
- **AC-11.5** No change to the knowledge pipeline, its schema documents or its
  reader window.

---

## 3. Traceability to the task list's four acceptance criteria

| Task-list criterion | Held by |
|---|---|
| Task creation, continuation and closing states are explicitly modelled | REQ-03 (AC-03.2, AC-03.3, AC-03.4) — the seven states and the total transition table |
| A checkpoint is readable after a restart and from a different AgentSession | REQ-04 (AC-04.2, AC-04.4) + REQ-06 (AC-06.3) + REQ-10 (AC-10.4) |
| Sequential agent handover in one workspace loses no data and produces no needless conflict | REQ-10 (AC-10.4) — the reopened-database handover; D-56 is why it works, D-62 is why it does not report a conflict |
| Domain, persistence and CLI flow are verified together by an integration test | REQ-10 (AC-10.4) drives the real command tree; REQ-09 asserts the failure rows the same way |

---

## 4. Work breakdown and dependency order

Waves are serialized where a shared artefact would otherwise be edited under a
concurrent reader.

| Task | Owns | REQ | Depends on |
|---|---|---|---|
| TASK-01 | `internal/identity` extraction, `internal/workspace` call sites | REQ-01 | — |
| TASK-02 | `migrations/000002_coordination.sql`, migration ledger | REQ-02 | — |
| TASK-03 | `app.Code` additions | REQ-05 | — |
| TASK-04 | `internal/coordination` domain + state table | REQ-03 | TASK-01, TASK-03 |
| TASK-05 | `internal/coordination.Store` | REQ-04 | TASK-02, TASK-04 |
| TASK-06 | `internal/bootstrap` wiring | REQ-08 | TASK-05 |
| TASK-07 | `internal/cli` three command groups | REQ-06, REQ-09 | TASK-06 |
| TASK-08 | `internal/status` block and goldens | REQ-07 | TASK-06 |
| TASK-09 | the handover integration test, mutation log | REQ-10 | TASK-07, TASK-08 |

TASK-01, TASK-02 and TASK-03 touch disjoint files and may run together.
TASK-07 and TASK-08 both regenerate goldens and must not.

---

## 5. Definition of done

1. Every acceptance criterion above is met, or is recorded as not met with the
   reason in `mr-003-findings.md`.
2. `make verify` green, `make tidy-check` green, and the test count reported with
   `go test -list '.*' ./...` — that command and no other.
3. Every new guard has a recorded mutation that turns it red (AC-10.3).
4. The task list's MR-003 entry carries a `#### Durum` block written the way
   MR-001's and MR-002's are: what is green, which criterion was gated on what,
   and where the findings live.
5. An independent audit has graded the implementation. Not the implementer's own
   re-reading — §1 of `mr-002-findings.md` is the measurement that says what that
   is worth in this repository.
