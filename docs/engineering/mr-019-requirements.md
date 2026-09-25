# MR-019 — Frozen requirements and acceptance criteria

- **Frozen at:** commit `18fcf9d`, branch `mr-002-knowledge-lifecycle`
- **Contract this refines:** [mr-019-design.md](mr-019-design.md). Where the two
  disagree, the design wins on intent and this document wins on the detail the
  design left open — every such gap is recorded below as a decision D-229…D-236.
- **Task:** [mindrail-0.1-task-list.md](mindrail-0.1-task-list.md) MR-019
- **Tier:** 3 (Program). It adds the warm-path benchmark suite with
  telemetry, cooperative budget deferral (partial/pending), and the
  automated release quality gate. No database schema, no wire codes, no
  MCP surface change.
- **Blocked by:** MR-015, MR-016, MR-018 (measures the MCP/verify paths
  those milestones built).

This file is frozen **before** implementation so that an audit has something
to grade against that the implementation did not shape. The milestone runs
**one task at a time** as MR-004 through MR-018 did: each task in §4 is
implemented, gated by an independent Reader/Breaker pair and recorded before
the next is begun.

---

## 0. Baseline evidence

Captured on the tree this work starts from, so a red suite later is unambiguous.

| Measurement | Value | Command |
|---|---|---|
| Commit | `18fcf9d` | `git rev-parse --short HEAD` |
| Go toolchain | `go1.27.0 linux/amd64` | `go version` |
| Packages | 35 | `go list ./... \| wc -l` |
| Top-level test functions | 1293 | `go test -list '.*' ./... \| grep -c '^Test'` |
| Benchmarks | none (`grep -r "func Benchmark" internal/` empty) | repo surface |
| Telemetry | fragments only (TxStats, startup slog, report DurationMS) | code reading |
| `make verify` | green at MR-018 close | `make verify` |
| `make tidy-check` | green at MR-018 close | `make tidy-check` |
| Migrations | `000001_initial.sql` … `000008_evidence.sql`; runtime `schema_version` reports 8 | `ls migrations/*.sql` |
| Registered `app.Code` values | 46 | `grep -c 'Code = "' internal/app/code.go` |
| Release automation | none (no matrix, no ldflags stamping, no gate matrix) | repo surface |
| Prior warm-path evidence | status ~25–200µs, after_change 10 files ~9ms, reconcile ~3ms (MR-005/007 findings) | findings |

---

## 1. The scenario, in one paragraph

A contributor runs `make bench` and gets p50/p95 per warm-path operation
against the STRUCTURAL targets in one report — all green with headroom.
A traversal that cannot finish inside its budget returns partial entries
with an explicit pending frontier and re-queues the remainder at high
priority instead of blocking or silently skipping. `make release` stamps
version metadata, builds the platform matrix, publishes checksums, and
smoke-tests the native binary; `make gate` runs the nine test categories
by name. A platform that cannot build is recorded with its toolchain
reason, never silently dropped from the matrix.

---

## 2. Decisions

MR-018 ended at D-228. These continue the sequence.

### D-229 — benchmarks are `go test -bench` on fixture repos, and p95 over target fails

Standard tooling only (tech-stack §98). Each of the five warm-path ops
runs against a representative fixture repo; the suite publishes
p50/p95 per op and fails when any STRUCTURAL p95 exceeds its target.
Targets are calibrated by evidence and never deleted (spec-1.0 §28);
a recalibration lands as a documented decision with numbers, not a
quiet constant change. Current headroom is ~10–100x, so the pins are
stable, not flaky.

### D-230 — telemetry is additive and changes no behavior

A small recorder captures per-operation duration with breakdowns
(total, SQLite wait via existing TxStats, parse, impact traversal).
It rides the measured paths as an observer: same inputs, same outputs,
same codes. No new failure modes may originate in telemetry.

### D-231 — deferral is cooperative with deterministic proof

Budget checkpoints sit in traversal/drain loops; a zero (or nanosecond)
budget forces the partial path deterministically — no wall-clock
flakiness in tests. The partial result carries entries-so-far plus an
explicit pending frontier; blocking past budget and silent skipping are
both refused by construction (the Budget has no "wait" and no "drop").

### D-232 — remainder scheduling reuses the cold scheduler's Prioritize

No new queue, no new goroutine ownership: the pending frontier's units
are re-queued through the existing `Scheduler.Prioritize` (MR-005),
which already moves units ahead of the cold remainder. Deferral
schedules; it does not execute inline.

### D-233 — release is Makefile-first and provider-neutral

Important logic stays ordinary commands (tech-stack §109): `make
release` stamps (`version`, `commit`, `buildDate`, `dirty` ldflags per
tech-stack §103), builds the §106 matrix, publishes checksums, and
smoke-tests the native binary. Per-target build status is recorded;
only targets with CI build + smoke are advertised (§106 rule) — 0.1
advertises linux/amd64, the rest are best-effort with toolchain reasons
on record. No `.github/` coupling: CI invokes the same targets.

### D-234 — the gate matrix maps nine categories to commands

Unit, domain, integration, race, knowledge-schema, MCP-contract,
Git-worktree, SQLite-concurrency, end-to-end — each maps to explicit
package/test-name selections run by one target. `govulncheck` is CI-side
(the binary is absent from this tree); the target runs it when present
and says so when absent — never a silent skip presented as a pass.

### D-235 — no new `app.Code` values, no migration

Registry stays 46; deferral/pending states reuse existing vocabulary
(`PARTIAL_READY`, pending counts — MR-005's, not new codes). No
migration (schema stays v8). Pending is a result shape, not an error.

### D-236 — serial tasks, Reader/Breaker gates, findings rows

The MR-004…MR-018 process repeats: one task at a time, each gated before
the next, each recorded in `mr-019-findings.md` (new file — MR-018's record
stays closed). Post-gate hygiene (`git status`/`git diff` on tracked
paths, unique mutant-backup names, md5-verified restore + full-suite
re-run) applies to every subagent.

---

## 3. Requirements and acceptance criteria

REQ-01…REQ-07. Each task's criteria are dotted under it, in the form the
gates grade. A criterion not met at the end of its task is recorded as not
met, with the reason, in `mr-019-findings.md`.

**REQ-01 — warm-path benchmarks** (five ops, p50/p95, targets).
**REQ-02 — latency telemetry** (total + SQLite-wait/parse/traversal breakdowns).
**REQ-03 — budget deferral** (partial + pending + prioritized reschedule).
**REQ-04 — release automation** (stamp, matrix, checksums, native smoke).
**REQ-05 — test-gate matrix** (nine categories, explicit commands).
**REQ-06 — non-goals, guarded**.
**REQ-07 — evidence discipline** (every new guard mutation-red).

### TASK-01 — telemetry + benchmark suite

Owns REQ-01, REQ-02.

- **AC-01.1** `make bench` (or the documented bench target) reports
  p50/p95 for `status`, `context`, `before_change`, `after_change`
  (≤10 files) and `reconcile` (≤10 files) against fixture repos.
- **AC-01.2** every STRUCTURAL p95 in the report sits under its target
  (`150ms`, `250ms`, `300ms`, `1.5s`, `2s`); over-target fails the
  target (D-229).
- **AC-01.3** telemetry breakdowns (operation total, SQLite wait, parse,
  impact traversal) are captured per op and published in the report;
  identical inputs still produce identical outputs and codes (D-230 —
  observer only, proven by the untouched existing suite staying green).
- **AC-01.4** benchmarkStencil: no wall-clock assertions in unit tests;
  budgets appear only as injected values in deferral tests (TASK-02),
  never as `time.Sleep`-shaped timing tests.

### TASK-02 — budget deferral with partial/pending

Owns REQ-03.

- **AC-02.1** a traversal that exceeds its budget returns partial
  entries plus an explicit pending frontier — never a block past budget,
  never a silent skip (D-231).
- **AC-02.2** the pending remainder is re-queued at high priority
  through the existing scheduler (D-232); the test observes the queue
  position, not wall-clock speed.
- **AC-02.3** a zero/nanosecond budget forces the partial path
  deterministically; a generous budget returns the complete result —
  both without timing flakiness.
- **AC-02.4** pending is a result shape with existing vocabulary, not a
  new error code and not a new table (D-235).

### TASK-03 — release automation + test-gate matrix

Owns REQ-04, REQ-05.

- **AC-03.1** `make release` (or documented equivalent) stamps
  version/commit/buildDate/dirty, builds the §106 matrix, writes
  checksums, and smoke-tests the native binary; `mindrail version`
  reports the stamped fields.
- **AC-03.2** every matrix target ends recorded: built, or not-built
  with its toolchain reason. Advertised set follows the §106 rule
  (build + smoke proven).
- **AC-03.3** one gate target runs the nine categories by explicit
  selection (unit, domain, integration, race, knowledge-schema,
  MCP-contract, Git-worktree, SQLite-concurrency, end-to-end); each
  category maps to commands a reader can re-run by hand.
- **AC-03.4** `govulncheck` runs when present and reports absence when
  not — a missing scanner is never presented as a passed scan (D-234).

### TASK-04 — proof, non-goals and the record

Owns REQ-06, REQ-07.

- **AC-04.1** full-suite green evidence: `make verify`, `make tidy-check`,
  test count by the single command.
- **AC-04.2** the non-goals hold under grep: registry still 46, no
  migration beyond `000008`, no MCP surface change, no new queues or
  goroutines outside the existing scheduler, no `.github/` coupling.
- **AC-04.3** every new guard added by TASK-01…03 has its mutation run and
  recorded red (REQ-07), and the task list's MR-019 entry carries a
  `#### Durum` block written the way MR-018's is.

---

## 4. Work breakdown and dependency order

Serialised: each task is gated before the next is written.

| Task | Owns | REQ | Depends on |
|---|---|---|---|
| TASK-01 | telemetry, benchmark suite, report | REQ-01, REQ-02 | — |
| TASK-02 | budget deferral, partial/pending, reschedule | REQ-03 | TASK-01 |
| TASK-03 | release stamping/matrix/checksums/smoke, gate matrix | REQ-04, REQ-05 | TASK-02 |
| TASK-04 | proof, non-goals, record, Durum block | REQ-06, REQ-07 | TASK-03 |

---

## 5. Definition of done

1. Every acceptance criterion above is met, or is recorded as not met with
   the reason in `mr-019-findings.md`.
2. `make verify` green, `make tidy-check` green, and the test count reported
   with `go test -list '.*' ./... | grep -c '^Test'` — that command and no
   other.
3. Every new guard has a recorded mutation that turns it red, recorded after
   it was run.
4. Every task in §4 has passed its Reader/Breaker gate before the next was
   begun, and the gate's findings — confirmed, refuted, unconfirmed — are in
   `mr-019-findings.md` per task, with what was done about each.
5. The task list's MR-019 entry carries a `#### Durum` block written the way
   MR-018's is.
6. The knowledge graph is refreshed in its own commit at the end.

---

## 6. Traceability to the task list's five acceptance criteria

| Task list AC | Where it is owned |
|---|---|
| STRUCTURAL p95 hedefleri raporlanır | AC-01.1, AC-01.2 (REQ-01) |
| SQLite wait, parse, traversal, operasyon süreleri ölçülür | AC-01.3 (REQ-02) |
| Bütçe aşımı partial/pending üretir | AC-02.1…02.4 (REQ-03) |
| Dokuz test kapısı çalışır | AC-03.3 (REQ-05) |
| Platform binary'leri build + smoke | AC-03.1, AC-03.2 (REQ-04) |
