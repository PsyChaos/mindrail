# MR-003 — design: sequential agent handover

Task-list milestone: *Session, Task ve Checkpoint ile sıralı ajan devri*
(`mindrail-0.1-task-list.md`). Scenario: **two different AgentSessions using the
same workspace one after the other continue the same Task through
checkpoint/context.**

This document fixes the package plan, the entity model, the runtime schema and
the command surface. The decisions it rests on (D-53…D-62) and the acceptance
criteria are in [mr-003-requirements.md](mr-003-requirements.md), which is frozen
before any code is written.

---

## 1. What the milestone is, in one paragraph

An agent opens a task, works, and leaves. A different agent — a new process, a
new session, possibly a different tool — arrives at the same repository and has
to answer three questions without asking a human: *what was being done, how far
did it get, and what did the last agent say about it.* MR-003 makes those three
answerable out of the runtime database: the Task carries the first two and the
Checkpoint carries the third. Nothing in the flow requires a Git worktree,
because spec §61 Mode A — same workspace, sequential agents — is the dominant
case today and worktrees are a recommendation for the concurrent modes, not a
requirement.

## 2. What it is not

| Not in MR-003 | Owner | Why not here |
|---|---|---|
| Task lease, expiry, renewal | MR-004 | A lease is a time-bounded claim with contention rules; MR-003 has no second concurrent agent to contend with. `CLAIMED` here means "a session claimed it", enforced by the state table, not by a clock |
| `operation_id` idempotency, optimistic revision | MR-004 | Same reason. Every MR-003 write is a single short transaction and the last writer wins by construction, which is what MR-004 replaces |
| `Change`, `Evidence`, reconcile | MR-006…MR-012 | A Task in MR-003 carries a title and a state, not a diff |
| MCP tools (`mindrail_claim`, `mindrail_checkpoint`) | MR-014…MR-016 | The domain package MR-003 builds is what those tools will call. The CLI is the surface that exists now |
| Cross-machine coordination | V2 | Spec §97: 1.0's scope is one local repository and its worktree set |
| Coordination affecting `readiness` | — | A blocked task is a fact about work, not about the installation. Decision D-62 |

## 3. Package plan

```text
internal/identity        NewID(prefix) — the ULID minter, extracted from
                         internal/workspace so five entity types can share it
internal/coordination    Session, Task, Checkpoint, the state table, and the
                         Store that persists them
internal/cli             task.go, checkpoint.go, session.go — three command
                         groups over the coordination service
internal/status          one additional observation block
migrations/              000002_coordination.sql
```

`internal/coordination` sits beside `internal/workspace` and `internal/knowledge`
in the domain layer: it imports `internal/app`, `internal/storage` and
`internal/identity`, and nothing above it. The existing `upwardRules` in
`internal/cli/arch_test.go` gain no new entry, because coordination is not an
adapter — but it is added to the layering test's expectations so that a later
import of `internal/status` from it is caught.

*Amended during the round-1 remediation (finding F16).* That last clause
described something the milestone did not do. `internal/cli/arch_test.go` was
not touched at all, so AC-03.5 was satisfied and unenforced: adding imports of
`internal/git` and `internal/workspace` to `internal/coordination` built,
vetted and tested clean. The one example this paragraph names was caught anyway,
by the compiler rather than by a test — `internal/status` already imports
`internal/coordination`, so the reverse edge is a cycle.

It is enforced now, and by the other shape of rule.
`TestDomainPackagesImportOnlyWhatTheirRequirementAllows` holds coordination to
an allow-list rather than to a list of forbidden edges: `upwardRules` says which
directions must not exist, and AC-03.5 fixes the whole set, so the allow-list is
what the criterion actually says.

### Why `internal/identity` is extracted rather than reused in place

`workspace.NewID(prefix)` already mints exactly the right value: a prefixed,
time-sortable, Crockford base32 ULID, monotonic within a millisecond. It is
generic in everything except which package it lives in. MR-003 needs three more
prefixes and MR-004, MR-006 and MR-011 need at least four more; every one of them
importing `internal/workspace` for an id would make a registration package into a
utility. The move is mechanical — the file and its tests move, `workspace`
imports the new package — and it is the smallest change that stops the drift.

### Why the terminology collision is handled by separation, not by renaming

`internal/storage/checkpoint.go` already exists and means the **SQLite WAL
checkpoint**: writing the write-ahead log back into the main database file. It is
MR-001 code, it is correct, and its name is SQLite's own. MR-003's Checkpoint is
an agent handover record. The two never appear in one package and neither is
renamed; decision D-54 records this so a later reader who greps for "checkpoint"
finds the answer rather than a puzzle.

## 4. Entity model

```text
Project ─┬─< Task ──< Checkpoint >── Session
         │                              │
Workspace ─────────────────────────────┘
```

- **Session** — one agent's run against one workspace. It is minted, never
  resumed: the second agent gets a *new* session and continues the *same* task.
  Continuity lives in the Task and the Checkpoint, which is what makes the
  handover work across a process boundary, a tool change and a machine restart.
- **Task** — the unit of work, owned by the **project** rather than by a
  workspace. Spec §61 shares task ownership at project level so that Mode B's two
  worktrees see one task; MR-003 has one workspace, and picking the project now
  means MR-004 does not have to migrate the column.
- **Checkpoint** — an append-only note a session leaves on a task. Never updated,
  never deleted. "The last checkpoint" is the newest by id, and the id is
  time-sortable, so ordering does not depend on a timestamp column whose
  resolution can tie.

  *Amended during the round-2 remediation (round 2, §4.4).* That argument is the
  one finding F37 refuted, and it survived here after D-59 was corrected: the
  id is time-sortable to the millisecond and, under the millisecond, ordered
  only within one process — between two processes writing in the same
  millisecond the two ids order by 80 random bits, which is the handover case
  this milestone exists for. "The last checkpoint" is the **last one
  inserted**, `ORDER BY rowid DESC`, safe because the table is append-only and
  a rowid is only reused after a delete. The timestamp column is still not the
  answer, for the reason D-59 gives: it is stamped before the transaction opens
  and its text form is not lexicographically ordered.

## 5. Runtime schema — `migrations/000002_coordination.sql`

```sql
CREATE TABLE sessions (
    session_id   TEXT PRIMARY KEY,
    workspace_id TEXT NOT NULL REFERENCES workspaces(workspace_id),
    label        TEXT,
    started_at   TEXT NOT NULL
) STRICT;

CREATE TABLE tasks (
    task_id        TEXT PRIMARY KEY,
    project_id     TEXT NOT NULL REFERENCES projects(project_id),
    title          TEXT NOT NULL,
    state          TEXT NOT NULL,
    blocked_reason TEXT,
    opened_by      TEXT NOT NULL REFERENCES sessions(session_id),
    claimed_by     TEXT REFERENCES sessions(session_id),
    created_at     TEXT NOT NULL,
    updated_at     TEXT NOT NULL
) STRICT;

CREATE TABLE checkpoints (
    checkpoint_id TEXT PRIMARY KEY,
    task_id       TEXT NOT NULL REFERENCES tasks(task_id),
    session_id    TEXT NOT NULL REFERENCES sessions(session_id),
    workspace_id  TEXT NOT NULL REFERENCES workspaces(workspace_id),
    note          TEXT NOT NULL,
    handoff       INTEGER NOT NULL,
    created_at    TEXT NOT NULL
) STRICT;

CREATE INDEX idx_tasks_project ON tasks(project_id, state);
CREATE INDEX idx_checkpoints_task ON checkpoints(task_id, checkpoint_id);
CREATE INDEX idx_sessions_workspace ON sessions(workspace_id);
```

`STRICT` and the `TEXT` timestamps follow `000001_initial.sql` exactly: every
column is written by one code path, and a type surprise in the runtime store is a
bug rather than a value to coerce.

There is deliberately **no task-event table**. Spec §59 keeps lease history as
events, and lease history is MR-004's; a table with one writer and no reader
would be a shape guess, and the existing migration's own comment is the
precedent for deferring a table rather than pre-building it.

## 6. The state table

Spec §58's seven states, and the only transitions that exist:

| From | To |
|---|---|
| `OPEN` | `CLAIMED`, `ABANDONED` |
| `CLAIMED` | `IN_PROGRESS`, `OPEN`, `ABANDONED` |
| `IN_PROGRESS` | `BLOCKED`, `READY_TO_COMPLETE`, `ABANDONED` |
| `BLOCKED` | `IN_PROGRESS`, `ABANDONED` |
| `READY_TO_COMPLETE` | `COMPLETED`, `IN_PROGRESS`, `ABANDONED` |
| `COMPLETED` | — terminal |
| `ABANDONED` | — terminal |

`CLAIMED → OPEN` is the release path: an agent that claimed a task and cannot do
it puts it back rather than abandoning work someone else could pick up.
`READY_TO_COMPLETE → IN_PROGRESS` is the reopen path, and it is what keeps the
completion gate MR-013 will add from being a one-way door.

Anything not in the table is `TASK_STATE_INVALID`, a named refusal carrying both
states and the transitions that *are* available from where the task is. It is
never a silent no-op: a command that reports success while changing nothing is
the failure mode MR-001's audit spent three findings on.

## 7. Command surface

Six subcommands in three groups. Every one honours `--json` and the standard
envelope, and every one that writes takes `--session`.

```bash
mindrail session open  [--label <text>]              # mint and print a session id
mindrail task open     --title <text> --session <id>
mindrail task state    <task-id> --to <STATE> [--reason <text>] --session <id>
mindrail task show     <task-id>                     # task + last checkpoint + who wrote it
mindrail task list     [--state <STATE>]
mindrail checkpoint write <task-id> --note <text> [--handoff] --session <id>
```

Two properties of this surface are deliberate.

**One transition command, not seven verbs.** `task state --to` is the only way a
task moves, so §6's table is consulted in exactly one place. A surface with
`claim`, `start`, `block`, `resume`, `ready` and `close` would let the CLI grow a
verb the table does not have, and would give MR-004's lease six places to hook
into instead of one.

**The session is supplied, not inferred.** Spec §98 makes `mindrail_session_id`
an explicit application handle carried by the caller rather than by the
connection — that is what lets an agent change tools mid-task. `--session` is
that handle at the CLI. A write with no `--session` mints one and says so in the
result, so a human at a terminal is not forced to run `session open` first, and
an agent that wants continuity passes the id it was given.

Reads (`task show`, `task list`) mint nothing. A command that created a row just
by asking a question would make the session table a log of curiosity.

## 8. Status integration

`status --json` gains one additive block beside `knowledge` and `workspace`:

```json
"coordination": {
  "observation": "observed",
  "tasks_open": 1,
  "tasks_in_progress": 2,
  "tasks_blocked": 0,
  "last_checkpoint": {"task_id": "TSK-…", "session_id": "SES-…", "at": "…"}
}
```

It is additive, so no published key changes meaning, and it does not touch the
six component names or `readiness` (D-62). `observation` follows the existing
`observed` / `not_observed` convention MR-001 finding F15 settled, so "there is
no coordination state" and "coordination could not be read" stay different
answers.

## 9. Test plan

The acceptance test is the milestone's own scenario, driven through the real
command tree in one repository:

1. Session A opens a task and moves it to `IN_PROGRESS`.
2. Session A writes a handoff checkpoint and stops.
3. **The database is closed and reopened** — a new process would.
4. Session B, a different id, runs `task show` and gets the task, its state, and
   A's checkpoint with A's session id on it.

Around it: the state table exercised in both directions (every legal transition
succeeds, every illegal one is refused by name), the CLI agreement matrix rows
for the new failure conditions, and the round trip through `status --json`.
