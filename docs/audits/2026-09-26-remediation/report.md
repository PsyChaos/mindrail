# Mindrail remediation — final verification report

## 1. Task

> o zaman bu sorunların hepsini düzeltmeni istiyorum

Tier 3 was used because the request changed validation proof semantics, database
schema, the public CLI/MCP contract and completion safety. Subagents were available:
three implementation owners worked on disjoint clusters, followed by two independent
Reader/Breaker auditors. The primary agent orchestrated, integrated and validated;
it did not write the production implementation.

Base ref: `2814acb22d8aa94d48ade1242b58e1fdb14239fc`. The frozen requirements,
acceptance criteria and decisions are in
[the remediation plan](../../engineering/remediation-2026-09-26.md). The historical
release findings remain intact in [the original report](../2026-09-26/report.md).

## 2. Requirements

Final result: **8 passed / 0 partial / 0 failed**.

| ID | Requirement | Final | Evidence |
|---|---|---|---|
| REQ-001 | Only a current, complete, wholly successful validation run satisfies proof; later full success recovers. | PASS | Run metadata/grouping, fail/error/mixed/partial replay and recovery tests; validation full/race suite. |
| REQ-002 | Directory/glob membership changes stale evidence without unrelated-tree false staleness. | PASS | Segment-aware glob matrix, zero-match/add/edit/remove controls, unreadable matched/unmatched boundaries and mutation proof. |
| REQ-003 | Broken/unsupported/unreadable knowledge fails completion closed. | PASS | Current and restarted-server loader/schema negative matrix and repair controls. |
| REQ-004 | Completion discovers the real Git diff through canonical reconcile before its final decision. | PASS | Skipped-protocol, declared-edit, broken-Git, repeated/restarted guard and HEAD-movement controls. |
| REQ-005 | The shipped binary exposes exactly 13 MCP tools over protocol-pure stdio. | PASS | Official SDK subprocess discovery/bootstrap test; `mcp_compatibility:"stdio"`; typed `mcp --json` refusal with zero stdout. |
| REQ-006 | Terminal task completion is gate-coupled, revision-safe and idempotent. | PASS | Raw COMPLETED refusal; ALLOW/DENY state+lease tests; completion-specific request hash; blank/Unicode required; final CAS race; seven-state executable recovery sweep. |
| REQ-007 | Usage/help/audit documentation agrees with executable behavior. | PASS | Updated Turkish guide, CLI help, historical snapshot notes and live binary comparisons. |
| REQ-008 | Scope is controlled and integrated verification passes. | PASS | Full normal/race/smoke, gate, benchmark, release, graph and independent audit evidence. |

## 3. Teams

| Team | Responsibility | Tasks | Status |
|---|---|---|---|
| Validation | Evidence run integrity and declarative scope freshness | TASK-001, FIX-001/002/006 | PASS |
| Completion | Knowledge/reconcile composition and durable guard baseline | TASK-002, FIX-005B | PASS |
| Public surface | MCP launcher, gated CLI completion, idempotency/CAS/help | TASK-003, FIX-003/004/005A/007 | PASS |
| Documentation | Final usage and historical/current separation | TASK-004 | PASS |
| Independent Reader | Requirement and contract conformance | TASK-005, rounds 1–3 | VERIFIED |
| Independent Breaker | Isolated falsification/mutation/upgrade/live binary probes | TASK-005, rounds 1–3 | VERIFIED |

## 4. Execution and implementation

The independent implementation streams were integrated through an SDK-neutral
`internal/completion` service. Material changes:

- Validation provenance now carries logical run identity, command completeness and
  declarative scope. Partial operation replay resumes the same run and validates
  persisted metadata. Segment-aware traversal prunes paths a glob cannot match while
  retaining fail-closed behavior for potentially matching unreadable paths.
- Completion reloads and validates current knowledge, captures guard mappings, runs
  canonical reconcile, verifies HEAD stability and evaluates one common gate for MCP
  and CLI.
- Migration 9 adds the workspace+HEAD guard-baseline store. Mapping identities are
  canonical and monotonic-union, so reconcile cannot erase protection across repeat,
  restart, concurrent writer or later-binding cases. Current knowledge remains the
  source for active/severity policy.
- `mindrail mcp` launches the 13-tool SDK server over stdio. Protocol stdout remains
  clean; `--json` is a typed usage refusal.
- Direct raw `COMPLETED` state mutation is refused. `task complete` evaluates the
  gate, then performs the terminal task/lease write with a final revision CAS.
  Completion idempotency includes the canonical required-profile set; changed
  arguments conflict rather than replaying another request.

No commit was created. Pre-existing user changes outside the product/remediation
scope were preserved.

## 5. Validation

Final round evidence is recorded in
[verification-round3.md](verification-round3.md). Executed results:

| Check | Result |
|---|---|
| Targeted package integration | PASS |
| `make tidy-check` | PASS |
| `make verify` | PASS: vet, full normal suite, full race suite and cmd smoke |
| `make gate` | PASS: unit 1345, domain 420, integration 10, race 9, knowledge 216, MCP 34, worktree 77+7, SQLite 17, E2E 10, smoke 5 |
| `make bench` | PASS: all declared thresholds; MCP reconcile p95 21.348 ms against 2 s target |
| `make release` | PASS: native linux/amd64, checksum and smoke |
| `graphify update .` | PASS: final graph refreshed |
| `git diff --check` | PASS |

Cross-compiled linux/arm64, Darwin and Windows targets remain best-effort
`not-built` because the existing Tree-sitter Go bindings exclude those builds. They
are not claimed as verified platforms.

## 6. Dual-agent audit

Mode: parallel independent subagents. Neither auditor implemented the delivery and
neither saw the other's conclusions before writing each round.

Audit depth was **A** for validation, completion, schema/baseline, public CLI/MCP and
integration seams; documentation was **C**. There were no depth downgrades. Breaker
round 1 ran B1–B6 and the mandatory cost, mechanical-rule, compatibility and class
sweep headings. It tested 15 semantic mutations: 13 were killed and two survivors
became regression-test findings.

Round 1 found five MEDIUM Breaker issues plus one overlapping Reader MEDIUM and two
Reader LOW documentation mismatches. Round 2 verified those fixes but found two
additional MEDIUM class-sweep/contract siblings. Round 3 was delta-only and closed
both:

- Breaker: **VERIFIED**. Six glob arms passed; neutralizing segment pruning killed
  the permanent test. A freshly built binary covered all seven task states: raw
  completion refused 7/7, state/lease remained unchanged 7/7 and the first recovery
  action executed successfully 7/7.
- Reader: **VERIFIED**. The glob matrix, state-derived refusal test, D-55 lifecycle
  ownership and final validation artifact all conformed; no new finding remained.

Detailed independent evidence:

- [Reader round 1](reader.md), [Breaker round 1](breaker.md)
- [Reader round 2](reader-round2.md), [Breaker round 2](breaker-round2.md)
- [Reader round 3](reader-round3.md), [Breaker round 3](breaker-round3.md)

Confirmed findings remaining: **none**. Refuted/closed findings are retained in the
round reports with the controls and mutations that settled them. Remediation rounds:
**3**, the configured cap; the third reached evidence-based consensus.

### Refuted findings

No produced final-round finding was discarded by unsupported disagreement. Earlier
candidate premises that were disproved, and the exact refuting experiments, remain in
the per-round reports. Every admitted round-1/2 finding was instead measured, fixed
and closed by a later control/scenario pair.

### Open items

None. There is no missing experiment that could change the task verdict to a
CRITICAL or HIGH failure.

### Off-spec headings

| Heading | Result | Evidence |
|---|---|---|
| Cost / worst case | PASS | Declared 100-sample benchmarks passed; guard-baseline capture is once-per-HEAD then monotonic reuse. Measured p95 values remain far below the 1.5–2 s operation budgets. |
| Mechanical rules | PASS | D-55 lifecycle ownership, authorized knowledge call sites, three task-mutation baseline callers, migration shape and protocol stdout purity all have source/integration assertions. |
| Class sweep | PASS | Glob traversal siblings, operation wrapper parameters, required-profile readers, baseline codecs and every task state were swept; round-3 found no sibling. |
| Backward compatibility | PASS within supported platform | Schema 8→9 with old rows, dirty-upgrade refusal, legacy validation provenance fail-closed behavior and operation replay were exercised. Cross platforms remain unverified as stated below. |

### Test and mutation assessment

Requirement coverage is explicit in section 2; it is not inferred from line
coverage. Breaker sampled 15 semantic guards in round 1 (13 killed, 2 survivors),
then permanent tests killed both survivors. Round-3 segment-pruning mutation was also
killed. Full normal/race/smoke and all gate categories passed after the last source
change.

## 7. Remaining issues and boundaries

No unresolved CRITICAL, HIGH, MEDIUM or LOW task finding remains.

The following are explicit product boundaries rather than incomplete remediation:

- An empty `required` list means no profile requirement (decision D-213); teams/CI
  must supply their required profiles.
- `verify --ci` does not execute validation commands (decision D-226); tests/lint/
  build remain separate CI steps.
- Canonical reconcile sees the whole dirty worktree. Separate worktrees or strict
  lease/scope coordination are required for concurrent agents.
- Only native linux/amd64 was built in this environment.
- The optional Graphify SQL parser and `govulncheck` executable were unavailable;
  neither is represented as a passed security/supply-chain scan.

## 8. Final consensus

### Reader — APPROVE

All frozen requirements and both round-3 deltas were produced as passing behavior;
no finding remains.

### Breaker — APPROVE

The final glob and state-guidance scenarios passed in fresh isolation; the relevant
mutation was killed and the real binary passed all seven lifecycle states.

## 9. Final status

# FINAL VERDICT: VERIFIED

Engineering-orchestrator status: **COMPLETED_AND_VERIFIED**.
