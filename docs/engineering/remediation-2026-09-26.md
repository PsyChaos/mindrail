# Release remediation plan — 2026-09-26

Base ref: `2814acb22d8aa94d48ade1242b58e1fdb14239fc`

User request: fix every problem reported by the 2026-09-26 release audit. This
plan treats that request as the four reproduced HIGH completion defects plus the
two public integration gaps named in the user-facing result: no production MCP
launcher and an ungated raw `COMPLETED` task transition. Audit-only open experiments
(`govulncheck` availability, optional wide enumeration, future runtime budgets) are
not product defects in this remediation scope.

## Frozen requirements

| ID | Requirement | Type | Source | Acceptance criteria |
|---|---|---|---|---|
| REQ-001 | Failed, errored, missing-command, or mixed-command validation runs must never satisfy completion evidence. A later wholly successful rerun may recover. | FUNCTIONAL / SECURITY | AUD-01 | Regression tests cover pass, fail, spawn error, mixed pass/fail, and successful rerun; only a current wholly successful run satisfies its required profile. |
| REQ-002 | Evidence freshness must detect membership changes under configured directory/glob scopes, including newly added files. | FUNCTIONAL | AUD-02 | Untouched scope stays current; edit/remove/add under declared scope becomes stale; files outside the declared scope do not stale it. |
| REQ-003 | Fatal knowledge loading/schema problems must fail completion closed exactly as status/CLI verification do. | FUNCTIONAL / COMPATIBILITY | AUD-03 | Unsupported, unreadable, or invalid relevant knowledge produces a deterministic completion denial/refusal; valid knowledge behavior stays unchanged. |
| REQ-004 | Completion must discover the actual Git diff through the canonical reconcile engine before evaluating the gate. | FUNCTIONAL / SECURITY | AUD-04 and kernel contract | A skipped before/after call cannot yield ALLOW for an unregistered edit; honest unchanged and properly reconciled flows retain their expected result; reconcile errors fail closed. |
| REQ-005 | The shipped binary must expose the 13-tool MCP server over stdio. | FUNCTIONAL / PUBLIC CONTRACT | Release audit integration gap | `mindrail mcp` starts stdio without contaminating stdout; a real subprocess SDK client lists 13 tools and completes a representative call; version metadata advertises the supported compatibility. |
| REQ-006 | Persisting terminal task state `COMPLETED` must be coupled to the evaluated completion gate. | FUNCTIONAL / PUBLIC CONTRACT / BACKWARD COMPATIBILITY | Public CLI probe | Direct raw state transition to `COMPLETED` is refused with actionable guidance; a public gated completion command leaves the task unchanged on DENY and atomically marks it completed/releases its lease only on ALLOW. JSON envelopes, operation replay, revision conflicts, and existing non-terminal transitions remain compatible. |
| REQ-007 | Documentation and usage examples must describe the final executable behavior and no longer carry the pre-fix warnings as current limitations. | DOCUMENTATION | User request | Usage, release audit addendum, CLI help, and compatibility text agree with tested behavior. |
| REQ-008 | No unrelated user changes may be modified, and the integrated repository must pass targeted tests, full verification, gate, benchmark, tidy, and native release checks. | NON_REGRESSION / TESTING | Repository policy | Product diff is scoped; exact validation commands are recorded; independent Reader and Breaker reach an evidence-based verified verdict with no unresolved HIGH finding. |

## Work breakdown and ownership

| Task | Objective | Requirements | Owner scope | Dependency | Complexity |
|---|---|---|---|---|---|
| TASK-001 | Correct validation result grouping and scope-membership freshness, tests first. | REQ-001, REQ-002 | `internal/validation/**` and directly necessary config-facing tests only | none | HIGH |
| TASK-002 | Make completion fail closed on knowledge problems and run canonical reconcile before gate evaluation, tests first. | REQ-003, REQ-004 | `internal/mcp/complete*`, completion composition tests, narrowly required reconcile adapters | none | HIGH |
| TASK-003 | Ship the MCP stdio launcher and gated CLI completion lifecycle, including subprocess and compatibility tests. | REQ-005, REQ-006 | `cmd/mindrail`, `internal/cli/**`, MCP public launcher wiring, coordination command tests | TASK-002 contract for gate evaluation | HIGH |
| TASK-004 | Reconcile documentation with the integrated behavior. | REQ-007 | usage and relevant release/spec documentation | TASK-001…003 | MEDIUM |
| TASK-005 | Integration, full validation, and independent dual-agent audit. | REQ-008 | whole changed surface; no implementation ownership | TASK-001…004 | HIGH |

Dependency graph: `TASK-001 || TASK-002 || TASK-003(launcher-first)` →
`TASK-003(gated-completion integration)` → `TASK-004` → `TASK-005`.

## Decisions

| ID | Decision | Evidence and impact |
|---|---|---|
| DEC-001 | A validation profile is proof only as a complete successful run, not as arbitrary individual rows. | Closes mixed-command false ALLOW while permitting a later clean rerun. Legacy rows without sufficient grouping metadata must fail closed where completeness cannot be proven. |
| DEC-002 | Scope provenance must preserve enough declarative membership information to re-enumerate at check time. | A concrete file list alone cannot detect additions. Existing evidence that cannot prove membership remains conservative. |
| DEC-003 | Public task completion becomes an evaluated operation; raw state mutation cannot write `COMPLETED`. | Removes the split-brain between coordination state and the release gate. Non-terminal state transitions retain the existing command. |
| DEC-004 | MCP runs over stdio with protocol output isolated on stdout and diagnostics elsewhere. | Required for real client compatibility and subprocess testing. |
| DEC-005 | Guard mappings that canonical reconcile would overwrite are captured in an explicit migration-backed, append-only workspace+HEAD baseline before every task-scoped index mutation. | Reusing the operation replay journal would couple unrelated contracts and break idempotency retention. Each ensure transaction monotonically unions canonical sorted mapping identities, so bindings introduced later under the same HEAD remain protected; current knowledge supplies severity/activation. Repeated calls, restarts and concurrent writers must converge, and a moving or untrustworthy HEAD fails closed. |

## Validation map

| Requirement | Targeted verification |
|---|---|
| REQ-001/002 | `go test ./internal/validation` plus completion evidence regressions |
| REQ-003/004 | `go test ./internal/mcp` plus the four audit reproductions |
| REQ-005 | CLI/MCP subprocess contract tests using the built executable |
| REQ-006 | CLI task completion agreement, denial persistence, replay, and revision tests |
| REQ-007 | doc link/command consistency checks |
| REQ-008 | `make verify`, `make tidy-check`, `make gate`, `make bench`, `make release`, followed by one dual-agent audit |
