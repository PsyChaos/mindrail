-- Optional JEV routing telemetry and short-lived MCP connection presence.
--
-- Both tables deliberately store metadata only. In particular there is no
-- column capable of holding a route goal, context, candidate identifier or
-- description, provider body, prompt, source, path, log or credential.

CREATE TABLE jev_route_events (
    route_id          TEXT PRIMARY KEY,
    project_id        TEXT NOT NULL REFERENCES projects(project_id),
    workspace_id      TEXT NOT NULL REFERENCES workspaces(workspace_id),
    task_id           TEXT NOT NULL REFERENCES tasks(task_id),
    session_id        TEXT NOT NULL REFERENCES sessions(session_id),
    provider          TEXT NOT NULL CHECK (provider IN ('typesafe')),
    model             TEXT NOT NULL CHECK (model IN ('jev-latest')),
    version           INTEGER NOT NULL CHECK (version = 1),
    status            TEXT NOT NULL CHECK (status IN ('ok', 'fallback', 'disabled')),
    reason            TEXT NOT NULL CHECK (reason IN (
        'adapter_failure', 'advice_available', 'api_key_missing', 'atomic_fallback',
        'credential_unavailable', 'deadline_unavailable', 'duplicate', 'input_not_json',
        'input_too_large', 'invalid_api_key', 'invalid_candidates',
        'invalid_goal', 'invalid_input', 'invalid_response', 'low_confidence',
        'no_candidates', 'no_match', 'provider_http_error', 'provider_timeout',
        'provider_unavailable', 'python_unavailable', 'rate_limited', 'response_too_large',
        'secret_in_input', 'telemetry_failed', 'unknown_input_field'
    )),
    credential_source TEXT NOT NULL CHECK (credential_source IN ('keyring', 'environment', 'none')),
    started_at        TEXT NOT NULL,
    ended_at          TEXT NOT NULL,
    duration_ms       INTEGER NOT NULL CHECK (duration_ms >= 0 AND duration_ms <= 60000),

    tool_candidate_count INTEGER NOT NULL CHECK (tool_candidate_count BETWEEN 0 AND 32),
    tool_selected_ordinal INTEGER,
    tool_confidence_milli INTEGER,
    agent_candidate_count INTEGER NOT NULL CHECK (agent_candidate_count BETWEEN 0 AND 32),
    agent_selected_ordinal INTEGER,
    agent_confidence_milli INTEGER,
    model_candidate_count INTEGER NOT NULL CHECK (model_candidate_count BETWEEN 0 AND 32),
    model_selected_ordinal INTEGER,
    model_confidence_milli INTEGER,
    effort_candidate_count INTEGER NOT NULL CHECK (effort_candidate_count BETWEEN 0 AND 32),
    effort_selected_ordinal INTEGER,
    effort_confidence_milli INTEGER,

    CHECK (tool_selected_ordinal IS NULL OR
           (tool_selected_ordinal BETWEEN 0 AND tool_candidate_count - 1 AND tool_confidence_milli IS NOT NULL)),
    CHECK (tool_confidence_milli IS NULL OR tool_confidence_milli BETWEEN 0 AND 1000),
    CHECK (agent_selected_ordinal IS NULL OR
           (agent_selected_ordinal BETWEEN 0 AND agent_candidate_count - 1 AND agent_confidence_milli IS NOT NULL)),
    CHECK (agent_confidence_milli IS NULL OR agent_confidence_milli BETWEEN 0 AND 1000),
    CHECK (model_selected_ordinal IS NULL OR
           (model_selected_ordinal BETWEEN 0 AND model_candidate_count - 1 AND model_confidence_milli IS NOT NULL)),
    CHECK (model_confidence_milli IS NULL OR model_confidence_milli BETWEEN 0 AND 1000),
    CHECK (effort_selected_ordinal IS NULL OR
           (effort_selected_ordinal BETWEEN 0 AND effort_candidate_count - 1 AND effort_confidence_milli IS NOT NULL)),
    CHECK (effort_confidence_milli IS NULL OR effort_confidence_milli BETWEEN 0 AND 1000)
) STRICT;

CREATE INDEX idx_jev_route_events_project ON jev_route_events(project_id, route_id);
CREATE INDEX idx_jev_route_events_task ON jev_route_events(task_id, route_id);
CREATE INDEX idx_jev_route_events_session ON jev_route_events(session_id, route_id);

CREATE TABLE agent_runtimes (
    runtime_id         TEXT PRIMARY KEY,
    project_id         TEXT NOT NULL REFERENCES projects(project_id),
    workspace_id       TEXT NOT NULL REFERENCES workspaces(workspace_id),
    task_id            TEXT NOT NULL REFERENCES tasks(task_id),
    session_id         TEXT NOT NULL REFERENCES sessions(session_id),
    client_name        TEXT NOT NULL CHECK (length(client_name) BETWEEN 1 AND 128),
    client_title       TEXT CHECK (client_title IS NULL OR length(client_title) BETWEEN 1 AND 256),
    client_version     TEXT CHECK (client_version IS NULL OR length(client_version) BETWEEN 1 AND 64),
    started_at         TEXT NOT NULL,
    last_heartbeat_at  TEXT NOT NULL,
    last_activity_at   TEXT NOT NULL,
    sequence           INTEGER NOT NULL CHECK (sequence >= 1),
    ended_at           TEXT,
    end_reason         TEXT CHECK (end_reason IS NULL OR end_reason IN ('completed', 'disconnected', 'error', 'replaced', 'shutdown')),
    CHECK ((ended_at IS NULL) = (end_reason IS NULL))
) STRICT;

CREATE INDEX idx_agent_runtimes_project ON agent_runtimes(project_id, runtime_id);
CREATE INDEX idx_agent_runtimes_task ON agent_runtimes(task_id, runtime_id);
CREATE INDEX idx_agent_runtimes_session ON agent_runtimes(session_id, runtime_id);
