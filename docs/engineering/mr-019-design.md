# MR-019 — Design

- **Frozen at:** commit `18fcf9d`, before any implementation commit.
- **Refines and is refined by:** [mr-019-requirements.md](mr-019-requirements.md)
  (D-229…D-236 record every place this design left a gap the implementation
  would otherwise fill silently).
- **Specification anchors:** kernel-scope §5 (STRUCTURAL warm targets);
  spec-1.0 §28 (performance contract, deferral rule, telemetry list),
  §29 (PARTIAL_READY lifecycle); tech-stack §43 (latency engineering),
  §96 (race), §98 (bench tooling), §102–§109 (supply chain, build,
  version, CGO, matrix, GoReleaser, CI provider, Makefile).

## 1. What the milestone is, in one paragraph

The measurable half of "done": after MR-019, every warm-path operation
carries its own stopwatch with named breakdowns, a benchmark report
grades p95 against the STRUCTURAL targets, work that outruns its budget
returns partial-plus-pending instead of blocking or lying, and one
`make release` produces stamped, checksummed, smoke-tested binaries
behind a nine-category gate matrix — all with provider-neutral commands.

## 2. What it is not

| Temptation | Ruling |
|---|---|
| Optimizing from the numbers | measure only; no perf-driven refactors (D-230) |
| FULL-semantic targets | STRUCTURAL only in 0.1 (no resolvers exist) |
| Wall-clock unit tests | budgets injected, never slept on (AC-01.4) |
| New error codes for pending | result shape, existing vocabulary (D-235) |
| New queues, pools, goroutines | existing `Scheduler.Prioritize` only (D-232) |
| GitHub Actions coupling | Makefile targets CI invokes (D-233) |
| Advertising unproven platforms | §106 rule: build + smoke proven first (D-233) |
| Silent scanner skip | absence reported, never passed (D-234) |

## 3. Package plan

| Package | Owns | Deliberately not there |
|---|---|---|
| `internal/perf` (new) | operation timer, breakdown recorder (total/sqlite-wait/parse/traversal), p50/p95 report, Budget with cooperative checkpoints | policy decisions |
| `internal/impact` (+budget) | budget-aware traversal: partial entries + pending frontier on exhaustion | scheduling (delegates to scheduler) |
| `internal/cli` (+version) | `buildDate`/`dirty` stamp fields beside version/commit | new commands |
| `Makefile` (+targets) | `bench`, `release`, `gate`, `vuln` (present-or-reported) | CI provider files |
| everything else | untouched (measured or composed as services) | — |

No migration, no codes, no MCP surface change. Telemetry observes;
deferral cooperates; release stamps.

## 4. Entity model

**Telemetry.** `perf.Timer` starts per operation; checkpoints record
`sqlite-wait` (folding existing `TxStats`), `parse`, `traversal`
durations; `Report` renders per-op p50/p95 plus breakdowns. Zero
behavioral coupling: the timer cannot fail the operation.

**Benchmarks.** `go test -bench` per op over fixture repos (≤10 files
for after_change/reconcile). Targets table (STRUCTURAL):

| Operation | p95 target |
|---|---|
| status | <150 ms |
| context | <250 ms |
| before_change | <300 ms |
| after_change ≤10 files | <1.5 s |
| reconcile ≤10 files | <2 s |

Over-target fails the bench target. Recalibration (if ever) is a
documented decision with numbers, never a quiet edit (spec-1.0 §28).

**Deferral.** `perf.Budget` carries a deadline; traversal loops call
`budget.Yield()` at work-item boundaries. On exhaustion the traversal
stops and returns `{entries-so-far, pending-frontier, complete:false}`;
the caller re-queues the frontier's units via `Scheduler.Prioritize`
(high priority, existing path). The Budget has no wait and no drop —
the two refused behaviors are unrepresentable. Zero-budget forces
partial deterministically in tests.

**Release.** `make release` versions via ldflags
(`version`, `commit`, `buildDate`, `dirty`), builds the §106 matrix
(linux/amd64, linux/arm64, darwin/amd64, darwin/arm64, windows/amd64),
writes SHA256 checksums, and runs the smoke suite against the native
binary. Each target ends `built` or `not-built: <toolchain reason>`,
recorded in the milestone findings. `make gate` runs the nine
categories by explicit package/test selections; `make vuln` runs
govulncheck when present, reports absence otherwise.

## 5. Runtime schema

None.

## 6. Readiness, CLI and performance integration

This milestone *is* the performance integration: benchmarks run the
real services over fixture repos (never mocks for timing), telemetry
reads observed durations only. No wall-clock assertions in unit tests.

## 7. Secrets

None new. Reports carry durations, counts and paths — never contents,
never environment.

## 8. Outputs reserved for later milestones

- FULL-semantic targets — need resolvers that do not exist in 0.1.
- Recalibrated targets — only with benchmark evidence + decision.
- Profile execution under budget — needs the profile→scope mapping.
- Provenance/signing, GoReleaser orchestration — post-0.1 hardening.

## 9. Test plan

Per tech-stack §90's categories, per task:

1. **Measure** (TASK-01): timer/breakdown unit tests (injected clock —
   deterministic), five bench functions green with headroom, report
   shape goldens, over-target-fails pin (mutant: inflated target? no —
   mutant: dropped checkpoint → breakdown test red).
2. **Defer** (TASK-02): zero-budget partial + pending frontier (exact
   frontier asserted), generous-budget complete, Prioritize observed
   (queue position, not speed), no-new-code/no-new-table greps.
3. **Release** (TASK-03): version-stamp test (ldflags + fallback),
   matrix build statuses recorded, checksums present, native smoke
   green, gate matrix each category runnable, vuln absence reported.
4. **Proof** (TASK-04): verify + tidy, non-goal greps, mutation ledger,
   Durum.
5. **Mutation ledger**: every guard gets its mutation run before it is
   written down (unchanged rule).
