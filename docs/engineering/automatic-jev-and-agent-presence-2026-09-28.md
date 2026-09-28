# Automatic JEV routing and agent presence — 2026-09-28

## Intake

The user wants optional JEV advice to participate in real tool, agent, model and
reasoning-effort decisions without mentioning Mindrail or JEV in each task prompt.
They also want the dashboard to show, in real time, which agent is associated with
work, how long its connection has existed and whether it is still producing an
activity signal. Vensift P0-03 is already running; this release will be installed
there before P0-04 begins.

This is Tier 3 because it adds an MCP contract, touches credentials and an external
provider, adds durable schema, changes the live dashboard contract and must preserve
older CLI and repository workflows.

## Requirements

| ID | Requirement | Type | Source | Acceptance criteria |
| --- | --- | --- | --- | --- |
| REQ-001 | Add a visible, client-neutral `mindrail_route` MCP tool for closed tool/agent/model/effort candidate sets. | FUNCTIONAL | User request | Tool discovery exposes it; an accepted provider answer can only select caller-supplied candidates. |
| REQ-002 | Managed instructions invoke routing once before an ambiguous supported choice, without requiring prompt text from the user. | FUNCTIONAL | User request | Fresh init/update text names the MCP tool as primary and the hidden CLI bridge only as fallback. |
| REQ-003 | JEV remains optional and advisory. | COMPATIBILITY | Prior user decision | Missing key, credential failure, provider failure, timeout, malformed/low-confidence answer, throttling or telemetry failure returns disabled/fallback data and normal reasoning continues. No Mindrail gate or permission is weakened. |
| REQ-004 | Automatic routing minimizes disclosure and never persists sensitive request/provider material. | SECURITY | Inferred consequence of automatic external calls | Raw goal/context/descriptions, prompts, source, diffs, paths, logs, environment, credentials and provider bodies are absent from the runtime DB, dashboard, logs and tool output beyond the validated advisory contract. |
| REQ-005 | Persist bounded proof that JEV was actually invoked, distinct from credential configuration. | FUNCTIONAL / SECURITY | User request | Route events contain attribution, enum status/reason, dimensions, candidate counts, selected ordinal, quantized confidence, credential source and duration; no candidate descriptions or raw payloads. Dashboard distinguishes configured from used. |
| REQ-006 | Capture a short-lived MCP runtime presence signal independent of 20-minute work leases. | FUNCTIONAL | User request | Bootstrap binds a unique runtime to the MCP connection; heartbeat is written every 5 seconds; disconnect/completion ends it; a crash becomes stale within 15 seconds. |
| REQ-007 | Dashboard reports truthful real-time duration and activity semantics. | FUNCTIONAL / NON_REGRESSION | User request | SSE remains 1.5 seconds and browser counters update each second; cards show self-reported client identity, CONNECTED FOR, last heartbeat, last MCP activity and CONNECTED/IDLE/STALE/ENDED. It never claims model/process/reasoning liveness that is not observed. |
| REQ-008 | Existing hidden CLI routing and no-key workflow remain compatible. | COMPATIBILITY | Existing repository contract | `mindrail agent route` keeps version-1 JSON behavior and projects without JEV continue normally. |
| REQ-009 | Schema upgrade is additive, deterministic and safe for existing initialized repositories. | COMPATIBILITY | Repository migration contract | New migration applies after 000010, old rows remain readable, re-run is idempotent and shipped migrations remain unchanged. |
| REQ-010 | Inputs, provider work and telemetry are bounded. | SECURITY / PERFORMANCE | Inferred from external-call boundary | Existing request/output/deadline bounds remain; route calls have per-session serialization, a minimum 10-second interval and duplicate requests within 30 seconds fall back without provider work. |
| REQ-011 | Documentation states the hard client limits. | DOCUMENTATION | Inferred truthfulness requirement | Docs say MCP cannot observe internal ambiguity, change the already-running main model/effort or prove thought/token activity; restart is required to discover the new tool. |
| REQ-012 | Full validation and independent dual-agent audit gate release. | TESTING | Engineering protocol | Targeted tests, `make check`, `make gate`, security/Reader audit and Breaker audit pass with no unresolved critical/high finding. |

## Public contracts

### `mindrail_route` request

The request keeps the existing version-1 route vocabulary: bounded `goal`, optional
bounded structured `context`, and one or more of `tools`, `agents`, `models`, or
`efforts`. Every candidate contains exactly `id` and `description`. Candidate sets
are supplied only after the host has applied permissions, safety and availability
filters. The managed instruction requires minimal, non-sensitive summaries; raw
prompts, code, diffs, paths, logs and secrets are forbidden.

### `mindrail_route` response

The validated adapter result remains additive version 1 (`enabled`, `advisory`,
`mode`, `status`, `reason`, `selections`). MCP adds `route_id` when a telemetry row
was recorded and always adds `telemetry_recorded`. Only `status=ok` with every
selection accepted may influence the subsequent choice. JEV never grants permission
or overrides Mindrail validation/completion.

### Durable route event

Migration 000011 creates `jev_route_events` with IDs and foreign keys for project,
workspace, task and session; provider/model/version enums; status/reason enums;
credential-source enum; start/end/duration; per-dimension candidate count, selected
ordinal and quantized confidence. The store never accepts raw goal, context,
description, candidate ID, provider response or credential material.

### Durable runtime presence

Migration 000011 also creates `agent_runtimes`:

- runtime/project/workspace/task/session identity;
- bounded MCP `ClientInfo` name/title/version marked as self-reported;
- `started_at`, `last_heartbeat_at`, `last_activity_at`, monotonic sequence;
- `ended_at` and bounded enum end reason.

The MCP connection owns its runtime identity; callers cannot submit another runtime
or session ID. Heartbeat proves only that the Mindrail MCP connection recently
updated the row. Tool activity proves only that a Mindrail tool was called.

Dashboard status is derived from the server clock:

- `CONNECTED`: heartbeat age <= 15 seconds and activity age <= 30 seconds;
- `IDLE`: heartbeat age <= 15 seconds and activity age > 30 seconds;
- `STALE`: no end marker and heartbeat age > 15 seconds;
- `ENDED`: an end marker exists; duration freezes at `ended_at - started_at`.

## Material decisions

| Decision | Context | Evidence | Impact |
| --- | --- | --- | --- |
| DEC-001 | MCP cannot see an agent's private ambiguity. | Current server only observes tool calls. | “Automatic” means a visible tool plus managed MUST-call policy for ambiguous closed choices, not interception of private reasoning. |
| DEC-002 | Current lease heartbeat is too coarse for real-time presence. | Lease TTL is 20m and renewal is TTL/3. | Presence gets a separate 5s heartbeat and 15s freshness window; work leases remain unchanged. |
| DEC-003 | Model/effort are not in MCP ClientInfo. | SDK exposes client name/title/version only. | Dashboard shows model/effort only when a future explicit trusted source exists; otherwise unknown. |
| DEC-004 | Persisting candidate IDs can persist secrets. | Candidate IDs are caller-controlled. | Persist selected ordinal/count, not IDs, descriptions or raw payload hashes. |
| DEC-005 | Auditable advice must not disappear silently. | Dashboard promises actual-use evidence. | Telemetry failure clears selections and returns fallback; engineering continues through normal reasoning. |
| DEC-006 | Presence is connection telemetry, not thought telemetry. | MCP cannot observe tokens, CPU or internal reasoning. | UI uses CONNECTED/IDLE/STALE/ENDED and explicitly labels client identity self-reported. |

## Post-audit security amendment

The historical requirements and decisions above record the original design and
are not rewritten. Adversarial review established that arbitrary `ClientInfo`
cannot be both preserved verbatim and guaranteed credential-free: name, title,
or version are caller-controlled and may themselves contain credentials.

The accepted persistence and dashboard contract therefore stores and displays a
server-owned canonical client family derived only from the self-reported
`ClientInfo.name`. Known aliases map to fixed families; every unknown or custom
name maps to `unknown-client`, and the runtime ID distinguishes individual
connections. Raw title/version never persist or display. The UI labels the value
as a canonical client family whose source is self-reported, rather than promising
to preserve the caller's raw identity. Presence duration, heartbeat, MCP activity,
and `CONNECTED`/`IDLE`/`STALE`/`ENDED` semantics are unchanged.

## Work breakdown

| Task | Objective | Requirements | Ownership | Dependencies | Acceptance |
| --- | --- | --- | --- | --- | --- |
| TASK-001 | Extract a shared bounded JEV router and preserve CLI behavior. | REQ-001, 003, 004, 008, 010 | `internal/agent/**`, `internal/cli/agent*` | none | Focused agent/CLI tests pass. |
| TASK-002 | Add migration 000011 and stores for route events and runtime presence. | REQ-004–006, 009–010 | `migrations/**`, new store files/tests | none | Migration/upgrade/store tests pass and prohibited fields cannot be written. |
| TASK-003 | Add visible MCP route and bind presence lifecycle/activity to connections. | REQ-001–003, 005–006, 008, 010 | `internal/mcp/**` | TASK-001, TASK-002 | MCP contract, attribution, throttle, disconnect and restart tests pass. |
| TASK-004 | Project route use and runtime presence into the live dashboard. | REQ-005–007 | `internal/dashboard/**` | TASK-002 schema contract | Collector/client/security contract tests pass. |
| TASK-005 | Upgrade managed instructions and user documentation. | REQ-002, 003, 007, 011 | `internal/setup/**`, `README.md`, `docs/usage-tr.md` | TASK-003, TASK-004 | Fresh init and update text agree with executable behavior. |
| TASK-006 | Integrate, validate, graph and independently audit. | REQ-012 | graph/audit artifacts | TASK-001–005 | Full gates and dual-agent consensus pass. |

Dependency graph:

```text
TASK-001 ─┐
          ├─ TASK-003 ─┐
TASK-002 ─┤            ├─ TASK-005 ─ TASK-006
          └─ TASK-004 ─┘
```

## Test map

| Cluster | Targeted command |
| --- | --- |
| Router/CLI | `go test ./internal/agent ./internal/cli -run 'JEV|AgentRoute' -count=1` plus `python -B -m unittest discover -s internal/agent -p 'test_jev_route.py'` |
| Migration/stores | `go test ./internal/migration ./internal/agent ./migrations -count=1` |
| MCP route/presence | `go test ./internal/mcp -run 'Route|Presence|Tool|Automatic|Disconnect' -count=1` |
| Dashboard | `go test ./internal/dashboard -count=1` |
| Setup/docs | `go test ./internal/setup ./internal/cli -run 'Init|Setup|Usage|JEV|Tool' -count=1` |
| Full gate | `make check && make gate` |

Baseline note: `go build ./...`, `go vet ./...` and `go test ./...` passed before
implementation. The generic validation helper also ran `python3 -m unittest
discover -v` at repository root, which discovered zero tests and returned failure;
the repository's actual Python JEV suite uses the explicit discovery command above.
