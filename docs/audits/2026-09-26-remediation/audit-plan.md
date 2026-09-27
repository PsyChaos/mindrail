# Remediation audit plan

Base ref: `2814acb22d8aa94d48ade1242b58e1fdb14239fc`

Audit mode: parallel independent subagents. Neither auditor participated in the
implementation. Reader and Breaker must write their reports before seeing the
other's conclusions. Mutation and destructive experiments run only in scratch
copies, never in the shared working tree.

## Original task, verbatim

> o zaman bu sorunların hepsini düzeltmeni istiyorum

The referent is the immediately preceding release result: four reproduced HIGH
completion defects, the missing production MCP launcher, and the fact that raw
`COMPLETED` state was not coupled to the evaluated gate. The frozen normalization
and acceptance criteria are in
[`docs/engineering/remediation-2026-09-26.md`](../../engineering/remediation-2026-09-26.md).

## Implementation clusters

| Cluster | Requirements | Main surface | Depth | Breaker moves | Target command |
|---|---|---|---|---|---|
| C1 validation proof/freshness | REQ-001/002 | `internal/validation/**` | A | 1–6; fail/error/mixed/replay/legacy/scope membership | `go test -count=1 ./internal/validation ./internal/gate` |
| C2 completion/knowledge/reconcile | REQ-003/004 | `internal/completion/**`, MCP adapters/tests | A | 1–6; repeated/restart/skipped protocol/current knowledge | `go test -count=1 ./internal/completion ./internal/mcp` |
| C3 durable guard baseline/schema | REQ-004/008 | migration 9, `internal/changes/guard_baseline.go`, migration contracts | A | 1–6; upgrade, empty/late/disjoint concurrency, HEAD movement | `go test -count=1 ./internal/changes ./internal/migration ./migrations` |
| C4 public MCP/task completion | REQ-005/006 | `cmd/mindrail`, `internal/cli/**`, MCP launcher | A | 1–6; real subprocess, stdout purity, denial state/lease, CAS/replay | `go test -count=1 ./cmd/mindrail ./internal/cli` |
| C5 docs | REQ-007 | usage guide and audit addendum | C | Reader doc↔code check | link and whitespace checks |
| Integration seams | REQ-008 | validation→completion→gate; baseline→reconcile; CLI→completion→coordination; binary→stdio | A | dual-reader parity plus full suite | `make verify && make gate` |

Downgrades from automatic risk classification: none. Sampling: mutation may use
representative guards per distinct semantic class rather than every generated error
branch, but every new public/safety boundary must have at least one neutralization
experiment and all survivors require a discriminating input. Migration, public
contract and fail-closed paths receive full depth A.

## Decisions to audit

- A required validation profile is satisfied only by the latest complete run whose
  commands are all current PASS/exit-0 rows; a later full success recovers.
- Declarative scope provenance is re-enumerated so membership additions stale proof.
- Completion rejects loader and schema-validation knowledge problems and performs
  canonical reconcile before the final gate decision.
- Guard mappings survive reconcile in a schema-9, workspace+HEAD, monotonic-union
  baseline; severity/activation remain live knowledge.
- `mindrail mcp` is a protocol-pure stdio launcher exposing exactly 13 tools.
- Direct raw `task state --to COMPLETED` is refused. `task complete` evaluates the
  gate and only then uses revision-guarded atomic state/lease transition.

## Pre-audit validation evidence

| Command | Result |
|---|---|
| targeted six-package integration test | PASS; validation 0.675 s, changes 2.554 s, completion 1.122 s, MCP 10.568 s, CLI 37.786 s, migrations 0.111 s |
| `make tidy-check` | PASS |
| `make verify` | PASS after two implementation remediation rounds; normal, vet, full race, smoke all green |
| `make gate` | PASS; unit 1332, domain 411, integration 10, race 9, knowledge-schema 216, MCP contract 34, worktree 77/7, SQLite concurrency 17, E2E 10, smoke 4 |
| `make bench` | PASS; MCP reconcile p95 10.796 ms / 2 s target; after-change p95 10.236 ms / 1.5 s target; changes reconcile p95 11.284 ms / 2 s target |
| `make release` | PASS for native linux/amd64 and smoke/checksums; other best-effort cross targets remain not-built due existing Tree-sitter Go binding constraints |
| `graphify update .` | PASS; graph refreshed, SQL parser dependency remains unavailable and reported by tool |

## Scope boundary

Audit only the product/remediation files listed by the clusters and their callers.
Pre-existing user changes under `.claude/`, `.gitignore`, `.wrongstack/`, `.zcode/`
are outside this delivery and must not be mutated. Graphify artifacts are expected
generated output from the required final update. No commit was created.

## Required auditor outputs

- Reader: `reader.md`
- Breaker: `breaker.md`
- Parent reconciliation/final: `report.md`
