-- MR-001 runtime schema: the two tables the bootstrap path needs to record
-- which repository it is looking at. Sessions, tasks, leases, symbols, changes
-- and evidence belong to later MRs and are deliberately absent.
--
-- STRICT is on because every column here is written by exactly one code path;
-- a type surprise in the runtime store is a bug, not a value to coerce.

CREATE TABLE projects (
    project_id    TEXT PRIMARY KEY,
    common_dir    TEXT NOT NULL UNIQUE,
    registered_at TEXT NOT NULL
) STRICT;

-- root_path and git_dir are machine-local location columns, not identity;
-- identity is the opaque workspace_id (decision D-26). A clone that moves
-- re-registers instead of silently orphaning its rows.
CREATE TABLE workspaces (
    workspace_id       TEXT PRIMARY KEY,
    project_id         TEXT NOT NULL REFERENCES projects(project_id),
    root_path          TEXT NOT NULL UNIQUE,
    git_dir            TEXT NOT NULL,
    is_linked_worktree INTEGER NOT NULL,
    registered_at      TEXT NOT NULL,
    last_seen_at       TEXT NOT NULL
) STRICT;

CREATE INDEX idx_workspaces_project ON workspaces(project_id);
