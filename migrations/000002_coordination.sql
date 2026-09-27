-- MR-003 coordination schema: the three tables sequential agent handover needs.
--
-- An agent opens a task, works, and leaves; a different agent arrives at the
-- same repository and has to answer what was being done, how far it got, and
-- what the last agent said about it. The Task carries the first two and the
-- Checkpoint carries the third. The Session says who.
--
-- Leases, operation_id idempotency and optimistic revision belong to MR-004 and
-- are deliberately absent, as sessions and tasks were deliberately absent from
-- 000001. Change, Evidence and the reconcile tables belong to MR-006 onwards.
--
-- STRICT is on for the same reason it is on in 000001: every column here is
-- written by exactly one code path, and a type surprise in the runtime store is
-- a bug, not a value to coerce.

-- A session is one agent's run against one workspace. It is minted, never
-- resumed (decision D-56): the second agent of a handover gets a new row and
-- continues the same task. There is no "current session" column anywhere,
-- because continuity that lived in a session would not survive the process exit
-- the handover exists for.
CREATE TABLE sessions (
    session_id   TEXT PRIMARY KEY,
    workspace_id TEXT NOT NULL REFERENCES workspaces(workspace_id),
    label        TEXT,
    started_at   TEXT NOT NULL
) STRICT;

-- A task belongs to a project, not to a workspace (decision D-60). Spec §61
-- shares task ownership at project level so that two worktrees of one repository
-- see one task; MR-003 exercises one workspace, and choosing the column now is
-- what keeps MR-004 from migrating rows a user already has.
--
-- claimed_by is nullable because an OPEN task has no claimant and a released one
-- has none again. It records which session claimed the task and nothing more:
-- there is no expiry and no renewal, and CLAIMED is a state rather than a lease
-- (decision D-58).
--
-- blocked_reason is set when a task enters BLOCKED and cleared when it leaves,
-- so a reader never meets a reason belonging to a block that was lifted.
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

-- Checkpoints are append-only (decision D-59): never updated, never deleted.
-- "The last checkpoint" is the newest checkpoint_id rather than the newest
-- created_at, because the id carries a 48-bit millisecond prefix and is
-- monotonic within a millisecond, and a timestamp column is not.
--
-- workspace_id is recorded as well as session_id because "which worktree was
-- this written from" is a real question in the separate-worktree mode and the
-- answer is only knowable at write time.
--
-- handoff marks the checkpoint a writer left on the way out (spec §99's
-- mindrail_checkpoint(handoff=true)). Nothing in MR-003 branches on it; the
-- column exists now because adding it later would mean a default that lies about
-- every row already written.
CREATE TABLE checkpoints (
    checkpoint_id TEXT PRIMARY KEY,
    task_id       TEXT NOT NULL REFERENCES tasks(task_id),
    session_id    TEXT NOT NULL REFERENCES sessions(session_id),
    workspace_id  TEXT NOT NULL REFERENCES workspaces(workspace_id),
    note          TEXT NOT NULL,
    handoff       INTEGER NOT NULL,
    created_at    TEXT NOT NULL
) STRICT;

-- The two queries status and `task list` actually run: tasks of a project by
-- state, and the newest checkpoint of a task. The second index is on
-- (task_id, checkpoint_id) so the newest is the last entry of a contiguous range
-- rather than a sort over the task's whole history.
CREATE INDEX idx_tasks_project ON tasks(project_id, state);
CREATE INDEX idx_checkpoints_task ON checkpoints(task_id, checkpoint_id);
CREATE INDEX idx_sessions_workspace ON sessions(workspace_id);
