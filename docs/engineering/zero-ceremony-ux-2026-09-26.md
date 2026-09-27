# Zero-ceremony UX program — 2026-09-26

Status: FROZEN FOR IMPLEMENTATION
Tier: 3 — public CLI/MCP contracts, coordination, configuration and documentation change together.
Starting point: existing dirty remediation worktree at HEAD `2814acb`; unrelated/user-owned changes must be preserved.

## User outcome

Normal human use is limited to:

```text
mindrail init       # once per repository
mindrail status
mindrail doctor
mindrail verify     # local verification; CI keeps `mindrail verify --ci`
```

The coding agent uses MCP. Session IDs, task IDs, lease holders, revisions,
operation IDs and terminal state transitions are implementation details and are
not copied between shell commands by a person. The existing 13-tool wire surface
is retained; optional automatic-mode fields evolve it compatibly.

## Requirements and acceptance criteria

| ID | Requirement | Type | Source | Acceptance criteria |
| --- | --- | --- | --- | --- |
| REQ-001 | The default human workflow exposes only setup and health/verification commands. | FUNCTIONAL / UX | User complaint | Root help presents `init`, `status`, `doctor`, `verify`, and `version` as the normal surface. Existing expert commands remain directly callable for compatibility but are absent from the default command list and are documented as advanced. |
| REQ-002 | `mindrail init` performs all safe repository-local setup idempotently. | FUNCTIONAL | User complaint; managed-section contract | One command scaffolds Mindrail, installs/chains the managed pre-commit hook, and creates or updates only Mindrail's managed AGENTS section. Repeated init is a no-op outside managed content. Existing user text, hooks and unrelated client configuration are preserved; malformed/unsafe targets fail visibly. |
| REQ-003 | Agents have a high-level automatic start operation without a new tool name. | FUNCTIONAL / PUBLIC CONTRACT | Inferred from “user should not deal with this”; 13-tool spec | `mindrail_bootstrap {goal, run_key, paths?, resume_task_id?}` activates automatic mode. It mints a per-run session, creates or explicitly resumes one task, claims it, advances it to working state, acquires applicable leases and returns useful context. It never steals an arbitrary active task. Legacy `mindrail_bootstrap {}` remains read-only and creates no rows. |
| REQ-004 | Agents have a high-level automatic finish operation without changing legacy completion semantics. | FUNCTIONAL / PUBLIC CONTRACT | Inferred; compatibility audit | `mindrail_complete {finalize:true, run_key?}` reconciles the actual diff, runs configured validation profiles in deterministic order, evaluates the shared completion gate, performs guarded terminal transitions on ALLOW, releases leases and returns an actionable denial otherwise. No caller supplies session/task/revision/operation IDs or required-profile overrides on this path. A legacy explicit payload remains evaluation-only. |
| REQ-005 | Automatic workflows are isolated per MCP connection and safe under concurrency. | SECURITY / CORRECTNESS | Existing lease/session invariants | Two connections on disjoint files can proceed; overlapping scope conflicts. Each connection has a fresh session. Long work renews leases without revision churn. Disconnect/shutdown stops renewal. A renewal failure is surfaced and prevents completion. |
| REQ-006 | Automatic retries and partial failures are idempotent and recoverable across process restart. | CORRECTNESS | Existing operation/revision contracts and crash-recovery spec | The agent supplies a stable opaque `run_key` that no person handles. Duplicate start/finalize delivery and MCP restart recover the same logical work without duplicate sessions/tasks or double completion. A failed start reports any durable state it created and can be retried. Gate denial or stale CAS never reports completion and retains recoverable task state. |
| REQ-007 | Scope attribution remains correct across old dirty work and sequential tasks. | DATA INTEGRITY / NON_REGRESSION | Architecture discovery | Pre-existing unrelated dirty files are not silently attributed to the new task. Two successive completed tasks may touch the same file without ambiguous attribution. Historical change rows remain queryable while terminal tasks are excluded from active ownership candidates. |
| REQ-008 | Existing low-level clients and scripts continue to work. | COMPATIBILITY | Existing 0.1 contract | The MCP list remains exactly the original 13 tool names. Their legacy payloads and explicit-ID semantics remain valid and side-effect compatible. Existing CLI paths remain callable. Protocol stdout remains frame-only. |
| REQ-009 | Local verification needs no mode flag. | FUNCTIONAL / UX | User complaint | `mindrail verify` behaves as the local staged verification path; explicit `--staged` and `--ci` continue to work, and conflicting modes remain rejected. Human output makes the selected mode clear. |
| REQ-010 | Documentation leads with the simple path and moves mechanics out of the way. | DOCUMENTATION | User request | README and Turkish usage docs begin with the four human commands and the automatic agent lifecycle. No happy-path section asks a person to use `jq`, copy IDs/revisions, acquire leases, or manually complete a task. A clearly labelled advanced reference retains those details. |
| REQ-011 | The delivery is behaviorally tested and independently audited. | TESTING | Repository policy | Unit/contract tests cover lifecycle, retries, dirty tree, same-file sequential tasks, simultaneous clients, lease renewal/failure, gate denial, hidden help, init idempotency and default verify. A real stdio E2E completes a task without coordination CLI calls. Full project gates and dual-agent audit pass. |

## Design decisions

| ID | Decision | Evidence and impact |
| --- | --- | --- |
| DEC-001 | Keep exactly 13 tools. Automatic mode is explicit optional input on `mindrail_bootstrap` and `mindrail_complete`; legacy payloads retain their original side-effect profile. | The 0.1 spec fixes the tool names/count and permits additive schema fields. This preserves old clients while removing human choreography. |
| DEC-002 | Put lifecycle composition in an SDK-neutral `internal/workflow` service; MCP handlers remain adapters. | CLI completion already showed why duplicated gate logic drifts. This also makes failure and concurrency tests deterministic. |
| DEC-003 | Automatic context is keyed by the MCP `*ServerSession`, not its textual ID. | Stdio transports may expose an empty session ID; the connection object is the isolation boundary. |
| DEC-004 | Sessions are always fresh; only an explicitly named resumable task/handoff may continue. | Existing coordination contract D-56 says sessions are per run and are never resumed. |
| DEC-005 | Finish runs every configured validation profile in sorted order. Zero profiles are reported as “none configured,” never “tests passed.” | Avoids adding a new config migration and prevents callers from weakening required evidence. |
| DEC-006 | Expert CLI commands are hidden from default help, not renamed or moved. | Keeps scripts and muscle memory compatible while removing visual noise. |
| DEC-007 | `mindrail verify` defaults to the existing staged path. | Removes a needless flag without inventing a third verification scope; CI stays explicit. |
| DEC-008 | Init owns the generated agent instructions and managed hook setup. | Matches the existing managed-marker promise and makes setup truly one command. It must merge/preserve, never overwrite user-owned text. |
| DEC-009 | `run_key` is an opaque stable retry key generated/carried by the agent; phase operation IDs are derived from it and existing durable replay records. | Connection memory alone cannot recover after an MCP process crash. People never copy or inspect the key. |

## Work breakdown and dependency graph

| Task | Objective | Requirements | Ownership | Dependencies | Complexity |
| --- | --- | --- | --- | --- | --- |
| TASK-001 | Build SDK-neutral automatic workflow service, including attribution, idempotency, leases/heartbeat and completion composition. | REQ-003–007 | `internal/workflow/**`; only necessary `internal/changes/**` and `internal/coordination/**` changes | none | HIGH |
| TASK-002 | Expose connection-scoped automatic bootstrap/finalize modes and optional-context lifecycle adapters without changing explicit legacy semantics or the 13-tool list. | REQ-003–008 | `internal/mcp/**`, MCP subprocess tests | TASK-001 contracts | HIGH |
| TASK-003 | Simplify human CLI/help/init/default verify and generate managed instructions/hook setup safely. | REQ-001, REQ-002, REQ-008, REQ-009 | `internal/cli/**`, setup renderer/helper, related bootstrap/config tests | none | HIGH |
| TASK-004 | Rewrite quickstart/reference documentation around the implemented workflow. | REQ-010 | `README.md`, `docs/usage-tr.md`, focused specs | TASK-001–003 interfaces | MEDIUM |
| TASK-005 | Integrate and add real stdio/concurrency/recovery E2E coverage. | REQ-005–011 | cross-module tests only | TASK-001–003 | HIGH |

```text
TASK-001 ──► TASK-002 ──┐
                        ├──► TASK-005 ──► full validation ──► dual-agent audit
TASK-003 ────────────────┘
TASK-001/2/3 ──► TASK-004
```

## Validation map

| Requirement group | Targeted evidence |
| --- | --- |
| REQ-003–007 | `go test ./internal/workflow ./internal/changes ./internal/coordination` plus race coverage for the new workflow tests |
| REQ-003–008 | `go test ./internal/mcp ./cmd/mindrail` including persistent stdio client |
| REQ-001/002/009 | `go test ./internal/cli ./internal/config ./internal/bootstrap` |
| REQ-010 | Reader doc↔code comparison and runnable quickstart snippets |
| REQ-011 | `make tidy-check`, `make verify`, `make gate`, `make bench`, `make release`, then independent Reader/Breaker audit |

Baseline before implementation:

```text
GOCACHE=/tmp/mindrail-gocache go test ./internal/cli ./internal/mcp \
  ./internal/coordination ./internal/completion ./internal/config \
  ./internal/bootstrap ./cmd/mindrail
PASS
```

## Explicitly out of scope

- Adding/removing MCP tool names, removing expert commands, or breaking their existing explicit-ID inputs.
- A daemon, HTTP service or remote coordinator.
- Automatic selection/stealing of another agent's active task.
- Editing arbitrary global MCP client configuration formats.
- Hiding the fact that a repository has no configured validation profiles.
