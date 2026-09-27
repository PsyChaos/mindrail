# Live dashboard observability contract

Date: 2026-09-27
Status: frozen for implementation
Risk tier: 3 (credential boundary and operational-status semantics)

## Problem

The dashboard currently reports JEV as unconfigured when the credential is stored in the OS keyring, labels a startup-only readiness projection without enough context, and does not let an operator distinguish dashboard connectivity from durable agent activity. Browser-extension warnings attributed to `contentscript.js` also need to be separated clearly from Mindrail-owned client errors.

## Requirements

- REQ-001: JEV status shown by the dashboard MUST use the same precedence as the CLI: non-blank environment credential, then OS keyring, then none. Only metadata may cross into the dashboard server; credential values and the full process environment MUST NOT be retained, serialized, logged, or served.
- REQ-002: Credential lookup MUST be bounded and fail open. An unavailable keyring MUST be displayed as `unavailable`, distinct from `not configured`, while the dashboard remains usable. JEV metadata is a startup snapshot and the UI MUST say that a restart is required after connect/disconnect.
- REQ-003: The UI MUST show dashboard-server start time, elapsed uptime, SSE connection state, last snapshot age, and snapshot sequence. These signals MUST NOT be presented as proof that an agent process is alive.
- REQ-004: Each agent/session card MUST show the durable identity available today (session label and ID), session age, non-terminal tasks currently claimed by that session, active lease count, latest recorded coordination activity, and lease renewal/expiry information.
- REQ-005: Agent/session status MUST be derived conservatively. A current task plus a valid lease may be called `active signal`; a task claim alone MUST NOT be called live, connected, or working. The UI MUST explain that exact client/model identity and process liveness are not recorded.
- REQ-006: Snapshot aggregates used for session activity MUST remain correct even when bounded task/lease/event detail arrays are truncated. No schema migration is required for this increment.
- REQ-007: Readiness freshness MUST remain truthful. Startup-only readiness is displayed explicitly as a startup snapshot with restart guidance; it MUST NOT be implied to refresh over SSE.
- REQ-008: Mindrail-owned browser assets MUST produce no console warnings/errors in the supported path. Warnings whose source URL is `chrome-extension://.../contentscript.js` are documented as injected extension code, not suppressed or misattributed to Mindrail.
- REQ-009: The dashboard remains loopback-only, token-protected, read-only, bounded, redacted, and compatible with existing snapshot consumers.

## Acceptance criteria

- AC-001: A key present only in the OS keyring renders JEV as configured with source `keyring`; a non-blank environment key renders source `environment` without reading keyring; missing and unavailable credentials render distinct states.
- AC-002: No sentinel credential or unrelated environment value appears in snapshot JSON, logs, errors, or dashboard launch contracts.
- AC-003: With a fixed clock, collector/server tests prove dashboard uptime, session age inputs, latest activity, current claimed tasks, valid/expired lease semantics, and correctness under detail truncation.
- AC-004: Client contract tests prove the UI distinguishes dashboard SSE health from agent activity, renders elapsed durations and current tasks, and labels JEV/readiness freshness.
- AC-005: Focused Go tests pass during implementation; after audit remediation, the repository's full validation gate passes once.
- AC-006: Independent Reader and Breaker audits find no unresolved correctness, security, or misleading-status issue.

## Decision log

- DEC-001: Do not claim exact agent product/model or process liveness. The current schema records session ID, label, tasks, leases, checkpoints, and timestamps, but not client/model identity or an authoritative connection heartbeat.
- DEC-002: Resolve JEV once at dashboard startup and pass metadata only. Re-reading a native keyring every 1.5 seconds can block, prompt, or repeatedly touch the secret boundary.
- DEC-003: Treat browser-extension `contentscript.js` warnings as an external diagnostic. Mindrail serves only its embedded `assets/app.js`; the reported warning source is not part of the repository.
- DEC-004: Avoid a schema migration in this increment. Durable activity and conservative status can be derived exactly from existing coordination records.

## Work plan

- TASK-001: Replace the dashboard environment seam with sanitized JEV startup metadata and add bounded credential-status resolution tests.
- TASK-002: Extend the snapshot with dashboard runtime metadata and exact per-session activity projections.
- TASK-003: Update the frontend to render connection freshness, uptime, conservative agent activity, tasks, leases, and freshness explanations.
- TASK-004: Update operator documentation, including extension-warning diagnosis and restart requirements.
- TASK-005: Run focused tests, graph refresh, independent audits, remediation, full validation, and logical commits.
