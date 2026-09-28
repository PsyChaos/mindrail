# Continuity policy requirements

Status: frozen for implementation on `feature/continuity-policy`.

## Outcome

Mindrail must preserve engineering continuity when an agent conversation approaches
its context limit. A capable host may create and activate a successor conversation
without routine user approval. An incapable host must expose a truthful, resumable
manual handoff instead of pretending that automation occurred.

## Requirements

### REQ-001 — Truthful runtime telemetry

Mindrail accepts monotonic host observations containing model, reasoning effort,
context used, context limit, observation time, source, and confidence. Unknown values
remain unknown. Raw prompts, reasoning, source code, logs, credentials, and raw run
keys are never persisted.

Acceptance criteria:

- context pressure is calculated only when both positive token counts are present;
- stale or duplicate observation sequence numbers cannot move policy state;
- model and effort are displayed with their source and freshness;
- generic MCP client metadata is never presented as measured model/context telemetry.

### REQ-002 — Configurable pressure policy

The default policy warns at 55% used, prepares automatic handoff at 60% used, and
enters the hard protection state at 75% used. Thresholds are repository-configurable,
strictly ordered, and evaluated using integer basis points.

Acceptance criteria:

- boundary values fire exactly once;
- decreasing or out-of-order observations cannot reverse a transition;
- a configurable number of consecutive observations is required before handoff;
- missing telemetry never triggers a context-based handoff.

### REQ-003 — Durable, exactly-once continuity intent

Each handoff is represented by a durable intent with a monotonic state machine and
compare-and-swap revision. At most one active intent exists for a predecessor task.
Host operations are idempotent on the intent ID.

States:

`OBSERVING -> WARNED -> CHECKPOINTED -> SPAWN_REQUESTED -> SPAWN_READY -> HANDED_OFF -> CLAIMED -> RESUMED -> COMPLETED`

Side exits are `MANUAL_REQUIRED`, `CANCELLED`, and `EXPIRED`.

Acceptance criteria:

- retries after every state transition are safe;
- a takeover token is random, one-time, and stored only as a hash;
- only one successor can consume the token;
- after handoff, the predecessor can no longer renew or finalize the task;
- a process crash cannot silently release a reserved task to an unrelated agent.

### REQ-004 — Same-task automatic resume

The normal transition creates a fresh conversation, session, and run key while
explicitly resuming the same Mindrail task. The task scope, baseline, durable
decisions, evidence, and latest checkpoint remain authoritative.

Acceptance criteria:

- no new Mindrail task is created for context rollover;
- the successor uses `resume_task_id` and a fresh run key;
- the predecessor checkpoint is written before ownership is released;
- delayed predecessor replies cannot erase the successor binding.

### REQ-005 — Optional next-task continuation

After successful completion, a capable host may continue to a separately registered
next task. Mindrail must never infer the next task from recency or list order.

Acceptance criteria:

- the next task is explicitly registered and belongs to the same project;
- terminal, reserved, or incompatible targets fail closed to `MANUAL_REQUIRED`;
- failure to start the next task does not undo a valid completion.

### REQ-006 — Host capability and authorization boundary

Conversation creation is performed only by an optional host adapter implementing
idempotent prepare and activate operations. Auto-spawn authorization is host-local;
repository configuration cannot grant it.

Acceptance criteria:

- no-capability and non-idempotent hosts stop at a resumable manual handoff;
- one host-local grant removes routine handoff approval prompts;
- revocation is checked before each host side effect;
- security-sensitive or external side effects retain their existing approval rules.

### REQ-007 — Operational visibility

The dashboard shows agent/client family, model, effort, used/limit tokens, pressure,
freshness, current continuity phase, threshold, elapsed time, successor identity when
known, and actionable failure/fallback reason. Only dashboard panels scroll; the page
shell remains fixed.

Acceptance criteria:

- unknown and stale data are visually distinct;
- connection heartbeat is not labelled model or process liveness;
- stalled phases and `MANUAL_REQUIRED` are visible;
- SSE updates render new observations and state transitions without page refresh.

### REQ-008 — Compatibility and verification

Existing bootstrap, checkpoint, completion, validation, JEV routing, 14-tool MCP
surface, and legacy clients remain compatible.

Acceptance criteria:

- existing MCP contract tests remain green;
- migration upgrades existing databases and preserves rollback-safe startup failure;
- policy, store, crash/retry, token replay, race, dashboard, and host fallback tests
  cover the new behavior;
- `make check`, `make gate`, independent review, and Mindrail completion pass.

## Work breakdown

1. Build and test the pure pressure policy and continuity domain model.
2. Add migration and compare-and-swap store with replay protection.
3. Integrate reservation, checkpoint, takeover, and resume into workflow.
4. Add the host bridge and truthful fallback protocol without expanding the 14 MCP
   tools unless a later contract review proves expansion necessary.
5. Add configuration and host-local authorization boundaries.
6. Add dashboard collection, SSE projection, and fixed-shell panel layout.
7. Implement documented Codex and Claude adapters only for capabilities their public
   interfaces actually expose; otherwise ship the manual bridge for that host.
8. Run targeted tests, full gates, graph update, and independent adversarial audits.

## Non-goals

- Capturing prompts, hidden reasoning, or conversation transcripts.
- Treating MCP connection heartbeat as proof that a model is actively working.
- Automatically selecting the newest open task.
- Allowing repository files to authorize host conversation creation.
- Claiming seamless automatic continuation on an unsupported client.
