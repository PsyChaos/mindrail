# Host runtime binding — frozen requirements

Status: frozen for implementation on `dev`.

## Goal

Make Claude Code and Codex runtime identity, activity, model, effort, context
pressure, subagent lifecycle, and continuity visible without guessing which host
session produced an observation. Installation is managed by `mindrail init` and
subsequent Mindrail updates; unavailable host facts remain explicitly unknown.

## Requirements

| ID | Requirement | Acceptance evidence |
| --- | --- | --- |
| REQ-001 | Runtime observations are bound to an explicit host session and optional subagent identity. Process-global environment variables, first-arrival ordering, and parent-to-child copying are forbidden attribution mechanisms. | Unit tests cover subagent-first and parallel parent/child events. |
| REQ-002 | Claude integration preserves existing project hooks and status line configuration. The main session may report model, effort, context and activity from the documented status-line payload. Subagents report identity, type and lifecycle only; unsupported child model/effort/context stay unknown. | Installer merge tests and payload tests. |
| REQ-003 | Codex integration preserves existing project hooks. Lifecycle events use documented hook fields and an already-connected Mindrail MCP tool hook. Unsupported fields stay unknown; transcript formats are not treated as stable telemetry APIs. | Installer merge tests and MCP contract tests. |
| REQ-004 | Startup, resume, clear/compact, stop/end, reconnect, stale timestamps, future timestamps and replayed sequence values cannot silently reuse or overwrite another runtime generation. | Lifecycle, freshness and replay tests. |
| REQ-005 | Dashboard agent cards show host, parent/child relationship, agent type, model, effort, context pressure, connected duration, last activity and freshness. Subagents are visible even when they were not spawned by Mindrail. | Collector and rendered-view tests. |
| REQ-006 | Automatic continuity can be driven only by fresh, verified main-runtime context telemetry. Child-only lifecycle events never trigger a parent handoff. | Continuity integration tests. |
| REQ-007 | No prompt, response, transcript contents, API keys or other secrets are persisted. Identifiers and bounded operational metadata only are accepted. | Validation and redaction tests. |
| REQ-008 | Installation/update is idempotent and repository-local. Claude/Codex configuration owned by the user is preserved. If Codex requires a one-time trust confirmation for project hooks, Mindrail reports it truthfully rather than bypassing it. | Repeated-init fixtures and documentation. |
| REQ-009 | A multi-dimension JEV `atomic_fallback` remains non-binding. A promising independent dimension may be retried once as a single-dimension request and is used only if that response is accepted. | Routing policy documentation and tests. |
| REQ-010 | Existing databases migrate forward, existing MCP clients keep working, and unavailable host adapters do not block normal Mindrail operation. | Migration, subprocess discovery and fallback tests. |

## Official capability boundary

- Claude status line exposes the main session's model, effort and context-window
  values. Claude subagent hooks expose `agent_id` and `agent_type`, but not a
  child context window. Parent context is therefore never presented as child
  context.
- Codex hooks expose the current session id and active model, plus subagent ids
  and types. Codex App Server exposes thread token-usage notifications, but a
  repository hook is not an App Server client; Mindrail accepts context data only
  when a supported host adapter supplies it with the matching bound identity.
- Codex project hooks are subject to Codex trust review. Mindrail installs the
  declarative integration, but does not weaken or bypass that review.

## Security invariants

1. Host-provided ids are opaque, bounded strings; they are not filesystem paths.
2. A producer sequence is monotonic within one `(host, session, agent, generation)`.
3. Ended generations cannot be revived by delayed observations.
4. Only main runtimes may own continuity pressure.
5. Missing or unverifiable data is `UNKNOWN`, never inferred.
