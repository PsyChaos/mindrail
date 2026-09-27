# Optional Jev routing — 2026-09-27

Status: FROZEN FOR IMPLEMENTATION
Tier: 3 — the change introduces an optional external API, secret handling, agent routing policy, and backward-compatibility requirements.
Base: `ba35fbe`

## User outcome

Agents may use TypeSafe Jev to advise bounded tool, agent, model, and reasoning-effort
choices. The integration is opt-in: `TYPESAFE_API_KEY` enables it. When the key is
absent or blank, the existing agent workflow continues without a network request or
behavioral dependency on TypeSafe.

Mindrail's deterministic core, thirteen-tool MCP contract, permissions, lifecycle,
validation, and completion gate remain authoritative and unchanged.

## Requirements and acceptance criteria

| ID | Requirement | Type | Source | Acceptance criteria |
| --- | --- | --- | --- | --- |
| REQ-001 | Jev routing is opt-in through `TYPESAFE_API_KEY`. | FUNCTIONAL / COMPATIBILITY | User request | Missing or whitespace-only key returns a typed disabled result, performs zero HTTP calls, and leaves the caller on its normal routing path. |
| REQ-002 | Jev judges only caller-supplied, closed candidate sets. | FUNCTIONAL | User request; TypeSafe Choice contract | One request may independently choose among supplied tools, agents, models, and effort levels. Unknown or omitted candidates can never be selected. |
| REQ-003 | Routing remains advisory and host-controlled. | TECHNICAL / NON_REGRESSION | Repository architecture | Results are explicitly shadow/advisory. Mindrail lifecycle, permission, validation, scope, and completion decisions cannot be altered by Jev output. |
| REQ-004 | Provider failures preserve the normal flow. | RELIABILITY / COMPATIBILITY | Inferred technical consequence | Timeout, DNS, HTTP error, malformed/oversized response, invalid choice, or low confidence returns a sanitized fallback result without raising a secret-bearing error or blocking normal routing. |
| REQ-005 | The API key and unrelated repository data remain private. | SECURITY | User opt-in; repository secret policy | The key appears only in the Bearer header, never in request JSON, stdout, errors, or logs. The adapter sends only the goal/context and candidate descriptions explicitly supplied by its caller and enforces input/output bounds. |
| REQ-006 | The integration follows the current TypeSafe HTTP contract without a runtime dependency. | TECHNICAL | TypeSafe live documentation | It posts `state`, `model: jev-latest`, and typed Choice questions to `/v1/systemone`, parses choice/confidence answers, uses a bounded timeout, rejects redirects, and adds no third-party package. |
| REQ-007 | Repository agents can discover and invoke the optional router. | DOCUMENTATION | User request | The engineering-orchestrator skill and repository agent instructions explain activation, candidate input, shadow semantics, fallback behavior, and the no-key path. |
| REQ-008 | Both branches are behaviorally tested. | TESTING | Repository completion contract | Unit tests cover disabled/no-network, successful multi-dimension routing, key non-disclosure, low confidence, provider failures, malformed/oversized responses, unknown choices, and bounded inputs. Existing Go gates remain green. |

## Design decisions

| ID | Decision | Evidence | Impact |
| --- | --- | --- | --- |
| DEC-001 | Implement the adapter in the host-side engineering-orchestrator skill, not the Go core. | Mindrail does not own host tool/model execution; production Go imports of `net/http` are intentionally prohibited. | The thirteen MCP tools and deterministic core remain unchanged. |
| DEC-002 | Use TypeSafe Choice for every non-empty candidate dimension and send independent questions together. | TypeSafe Choice and speculative fan-out documentation. | A single API call can advise tool, agent, model, and effort selections. |
| DEC-003 | Treat confidence below `0.60` as unaccepted advice and continue normally. | TypeSafe confidence guidance; this is a low-stakes advisory threshold to validate, not a universal model property. | Raw confidence remains visible, but the host applies only accepted advice. |
| DEC-004 | Provider errors are typed, sanitized data with process exit success. | The feature is optional and must not become a new availability dependency. | Agents can inspect status while continuing the pre-existing flow. |

## Work breakdown

| Task | Objective | Requirements | Files | Dependencies | Acceptance |
| --- | --- | --- | --- | --- | --- |
| TASK-001 | Implement the bounded TypeSafe HTTP adapter and CLI. | REQ-001–006 | `.claude/skills/engineering-orchestrator/scripts/jev_route.py` | none | Focused adapter tests pass. |
| TASK-002 | Add exhaustive offline tests for activation, response validation, and secret safety. | REQ-001, REQ-002, REQ-004–006, REQ-008 | `.claude/skills/engineering-orchestrator/scripts/test_jev_route.py` | TASK-001 | Tests make no real network calls and cover every listed branch. |
| TASK-003 | Integrate the optional decision step into agent guidance. | REQ-003, REQ-007 | `.claude/skills/engineering-orchestrator/SKILL.md`, `AGENTS.md` | TASK-001 | Key/no-key and advisory semantics are unambiguous. |
| TASK-004 | Validate, audit, document evidence, and refresh Graphify. | REQ-001–008 | audit artifacts and `graphify-out/**` | TASK-001–003 | Targeted tests, full gates, and independent audit reach VERIFIED. |

## Validation map

| Requirements | Command |
| --- | --- |
| REQ-001–006, REQ-008 | `python -m unittest discover -s .claude/skills/engineering-orchestrator/scripts -p 'test_jev_route.py' -v` |
| REQ-003, REQ-007 | Reader comparison of frozen requirements, skill instructions, and `AGENTS.md` |
| REQ-008 regression | `make tidy-check && make verify && make gate` |

## Explicitly out of scope

- Replacing the coding agent's reasoning model with Jev.
- Letting Jev authorize destructive actions or weaken Mindrail gates.
- Adding a fourteenth MCP tool or network access to Mindrail's Go core.
- Hard-coding one host's model names or installed agent inventory.
- Persisting `TYPESAFE_API_KEY` in repository configuration.
