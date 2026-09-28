-- Explicit host-runtime generations and the repository-local pending bridge.
--
-- Host ids are opaque bounded identifiers. Pending command-hook events retain
-- only SHA-256 identity digests until an MCP connection claims them; prompts,
-- transcript paths/content, source, logs, and credentials have no column.

CREATE TABLE host_runtime_bindings (
    runtime_id          TEXT PRIMARY KEY REFERENCES agent_runtimes(runtime_id) ON DELETE CASCADE,
    host                TEXT NOT NULL CHECK (host IN ('claude-code', 'codex')),
    host_session_hash   BLOB NOT NULL CHECK (length(host_session_hash) = 32),
    host_agent_hash     BLOB NOT NULL CHECK (length(host_agent_hash) = 32),
    is_main             INTEGER NOT NULL CHECK (is_main IN (0, 1)),
    parent_runtime_id   TEXT REFERENCES host_runtime_bindings(runtime_id),
    generation_hash     BLOB NOT NULL CHECK (length(generation_hash) = 32),
    agent_type          TEXT CHECK (agent_type IS NULL OR length(agent_type) BETWEEN 1 AND 64),
    lifecycle_state     TEXT NOT NULL CHECK (lifecycle_state IN ('ACTIVE', 'ENDED')),
    last_event          TEXT NOT NULL CHECK (last_event IN ('start', 'activity', 'telemetry', 'stop')),
    producer_sequence   INTEGER NOT NULL CHECK (producer_sequence >= 1),
    observed_at         TEXT NOT NULL,
    UNIQUE(host, host_session_hash, host_agent_hash, generation_hash)
) STRICT;

CREATE INDEX idx_host_runtime_session
    ON host_runtime_bindings(host, host_session_hash, lifecycle_state);
CREATE INDEX idx_host_runtime_parent
    ON host_runtime_bindings(parent_runtime_id);

CREATE TABLE pending_host_runtime_events (
    host                TEXT NOT NULL CHECK (host IN ('claude-code', 'codex')),
    host_session_hash   BLOB NOT NULL CHECK (length(host_session_hash) = 32),
    host_agent_hash     BLOB NOT NULL CHECK (length(host_agent_hash) = 32),
    generation_hash     BLOB NOT NULL CHECK (length(generation_hash) = 32),
    agent_type          TEXT CHECK (agent_type IS NULL OR length(agent_type) BETWEEN 1 AND 64),
    lifecycle_state     TEXT NOT NULL CHECK (lifecycle_state IN ('ACTIVE', 'ENDED')),
    last_event          TEXT NOT NULL CHECK (last_event IN ('start', 'activity', 'telemetry', 'stop')),
    producer_sequence   INTEGER NOT NULL CHECK (producer_sequence >= 1),
    model_key           TEXT,
    effort              TEXT,
    context_used        INTEGER,
    context_limit       INTEGER,
    started_at          TEXT NOT NULL,
    last_activity_at    TEXT NOT NULL,
    observed_at         TEXT NOT NULL,
    ended_at            TEXT,
    run_key_hash        BLOB CHECK (run_key_hash IS NULL OR length(run_key_hash) = 32),
    PRIMARY KEY(host, host_session_hash, host_agent_hash, generation_hash),
    CHECK ((context_used IS NULL) = (context_limit IS NULL)),
    CHECK (context_used IS NULL OR context_used BETWEEN 0 AND context_limit),
    CHECK (context_limit IS NULL OR context_limit BETWEEN 1 AND 100000000),
    CHECK ((lifecycle_state = 'ENDED') = (ended_at IS NOT NULL))
) STRICT, WITHOUT ROWID;

CREATE INDEX idx_pending_host_runtime_observed
    ON pending_host_runtime_events(observed_at);
CREATE INDEX idx_pending_host_runtime_run
    ON pending_host_runtime_events(run_key_hash) WHERE run_key_hash IS NOT NULL;
