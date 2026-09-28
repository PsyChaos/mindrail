-- Privacy-safe runtime observations and crash-safe continuity intents.
--
-- These tables deliberately store operational metadata only. They cannot hold
-- prompts, reasoning, conversation text, source code, logs, paths, credentials,
-- provider payloads, or raw run keys.

CREATE TABLE agent_runtime_observations (
    runtime_id     TEXT PRIMARY KEY REFERENCES agent_runtimes(runtime_id) ON DELETE CASCADE,
    model_key      TEXT CHECK (model_key IS NULL OR model_key IN (
        'claude-opus', 'claude-sonnet', 'claude-haiku',
        'gpt-5', 'gpt-5-codex', 'gpt-5.6-sol', 'gpt-5.6-terra', 'gpt-5.6-luna', 'gpt-5.5', 'gpt-6-astra', 'unknown'
    )),
    effort         TEXT CHECK (effort IS NULL OR effort IN (
        'off', 'minimal', 'low', 'medium', 'high', 'xhigh', 'max', 'ultra', 'not_applicable'
    )),
    context_used   INTEGER,
    context_limit  INTEGER,
    source         TEXT NOT NULL CHECK (source IN ('mcp_meta', 'host_adapter')),
    confidence     TEXT NOT NULL CHECK (confidence IN ('self_reported', 'structured_host', 'derived')),
    observed_at    TEXT NOT NULL,
    revision       INTEGER NOT NULL CHECK (revision >= 1),
    CHECK ((context_used IS NULL) = (context_limit IS NULL)),
    CHECK (context_used IS NULL OR context_used BETWEEN 0 AND context_limit),
    CHECK (context_limit IS NULL OR context_limit BETWEEN 1 AND 100000000)
) STRICT;

CREATE INDEX idx_agent_runtime_observations_time
    ON agent_runtime_observations(observed_at, runtime_id);

CREATE TABLE continuity_intents (
    intent_id                         TEXT PRIMARY KEY,
    project_id                        TEXT NOT NULL REFERENCES projects(project_id),
    workspace_id                      TEXT NOT NULL REFERENCES workspaces(workspace_id),
    task_id                           TEXT NOT NULL REFERENCES tasks(task_id),
    predecessor_session_id            TEXT NOT NULL REFERENCES sessions(session_id),
    predecessor_run_hash              BLOB NOT NULL CHECK (length(predecessor_run_hash) = 32),
    kind                              TEXT NOT NULL CHECK (kind IN ('SAME_TASK', 'NEXT_TASK')),
    target_task_id                    TEXT REFERENCES tasks(task_id),
    state                             TEXT NOT NULL CHECK (state IN (
        'OBSERVING', 'WARNED', 'CHECKPOINTED', 'SPAWN_REQUESTED', 'SPAWN_READY',
        'HANDED_OFF', 'CLAIMED', 'RESUMED', 'COMPLETED',
        'MANUAL_REQUIRED', 'CANCELLED', 'EXPIRED'
    )),
    revision                          INTEGER NOT NULL CHECK (revision >= 1),
    last_observation_sequence         INTEGER NOT NULL CHECK (last_observation_sequence >= 0),
    last_used_basis_points            INTEGER NOT NULL CHECK (last_used_basis_points BETWEEN -1 AND 10000),
    consecutive_handoff_observations  INTEGER NOT NULL CHECK (consecutive_handoff_observations BETWEEN 0 AND 20),
    checkpoint_id                     TEXT REFERENCES checkpoints(checkpoint_id),
    host_operation_id                 TEXT CHECK (host_operation_id IS NULL OR length(host_operation_id) BETWEEN 1 AND 256),
    takeover_token_hash               BLOB CHECK (takeover_token_hash IS NULL OR length(takeover_token_hash) = 32),
    successor_run_hash                BLOB CHECK (successor_run_hash IS NULL OR length(successor_run_hash) = 32),
    successor_session_id              TEXT REFERENCES sessions(session_id),
    failure_code                      TEXT CHECK (failure_code IS NULL OR length(failure_code) BETWEEN 1 AND 64),
    created_at                        TEXT NOT NULL,
    updated_at                        TEXT NOT NULL,
    expires_at                        TEXT NOT NULL,
    CHECK ((kind = 'SAME_TASK' AND (target_task_id IS NULL OR target_task_id = task_id)) OR
           (kind = 'NEXT_TASK' AND target_task_id IS NOT NULL)),
    CHECK (successor_session_id IS NULL OR successor_run_hash IS NOT NULL)
) STRICT;

CREATE UNIQUE INDEX idx_continuity_active_predecessor
    ON continuity_intents(task_id)
    WHERE state NOT IN ('COMPLETED', 'MANUAL_REQUIRED', 'CANCELLED', 'EXPIRED');
CREATE UNIQUE INDEX idx_continuity_takeover_token
    ON continuity_intents(takeover_token_hash)
    WHERE takeover_token_hash IS NOT NULL;
CREATE INDEX idx_continuity_project_state
    ON continuity_intents(project_id, state, updated_at);
CREATE INDEX idx_continuity_successor
    ON continuity_intents(successor_session_id)
    WHERE successor_session_id IS NOT NULL;
