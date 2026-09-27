-- Preserve exact proof edges before task-scoped discovery replaces index facts.
-- One monotonic canonical JSON union per worktree and committed baseline.
-- Newly discovered exact edges are added; reindexing never removes old proof.
-- An empty array is an observed empty mapping set, not a missing capture.
-- Current knowledge supplies severity and active status at evaluation time.
CREATE TABLE guard_baselines (
    project_id     TEXT NOT NULL REFERENCES projects(project_id),
    workspace_id   TEXT NOT NULL REFERENCES workspaces(workspace_id),
    head_oid       TEXT NOT NULL,
    format_version INTEGER NOT NULL CHECK (format_version > 0),
    mappings_json  TEXT NOT NULL,
    captured_at    TEXT NOT NULL,
    PRIMARY KEY (project_id, workspace_id, head_oid, format_version)
) STRICT;
