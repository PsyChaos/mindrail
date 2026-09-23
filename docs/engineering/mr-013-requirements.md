# MR-013 — Frozen requirements and acceptance criteria

- **Frozen at:** commit `eb55e21`, branch `mr-002-knowledge-lifecycle`
- **Contract this refines:** [mr-013-design.md](mr-013-design.md). Where the two
  disagree, the design wins on intent and this document wins on the detail the
  design left open — every such gap is recorded below as a decision D-179…D-184.
- **Task:** [mindrail-0.1-task-list.md](mindrail-0.1-task-list.md) MR-013
- **Tier:** 3 (Program). It adds the ALLOW/DENY model every milestone fed,
  one `app.Code`, and the gate MR-014's tools and MR-017's CI will call. No
  database schema.

This file is frozen **before** implementation so that an audit has something
to grade against that the implementation did not shape. The milestone runs
**one task at a time** as MR-004 through MR-012 did: each task in §4 is
implemented, gated by an independent Reader/Breaker pair and recorded before
the next is begun.

---

## 0. Baseline evidence

Captured on the tree this work starts from, so a red suite later is unambiguous.

| Measurement | Value | Command |
|---|---|---|
| Commit | `eb55e21` | `git rev-parse --short HEAD` |
| Go toolchain | `go1.27.0 linux/amd64` | `go version` |
| Packages | 32 | `go list ./... \| wc -l` |
| Top-level test functions | 1205 | `go test -list '.*' ./... \| grep -c '^Test'` |
| `make verify` | green at MR-012 close | `make verify` |
| `make tidy-check` | green at MR-012 close | `make tidy-check` |
| Migrations | `000001_initial.sql` … `000008_evidence.sql`; runtime `schema_version` reports 8 | `ls migrations/*.sql` |
| Registered `app.Code` values | 44 | `grep -c 'Code = "' internal/app/code.go` |
| Completion verdicts | do not exist (no ALLOW/DENY model) | code |

---

## 1. The scenario, in one paragraph

A task changed code without ever calling `before_change`. Completion does
not take the agent's word: it reconciles first, then judges — an orphaned
CRITICAL invariant blocks with its remedy, a stale `test` profile blocks
with its re-run name, an ambiguous shared symbol blocks with both
claimants, and a removed CRITICAL verification test blocks with its own
code. Each denial carries a deterministic reason code, provenance and a
`next_action`. When every family clears, the same inputs decide ALLOW
twice in a row, byte-identical.

---

## 2. Decisions

MR-012 ended at D-178. These continue the sequence.

### D-179 — the gate composes values; reconcile-first is composition

`gate.Evaluate` calls no store: it judges the rows, verdicts and findings
other milestones produced. "Completion reconciles even without
before_change" holds because the completion flow runs reconcile before the
gate — proven by an E2E that never declares, only discovers (MR-007's
guarantee, not a second implementation).

### D-180 — four families reuse codes; evidence gets one new code

Invariant, ambiguity, attribution and guard denials reuse
`ORPHANED_PROTECTED_SYMBOL`, `SYMBOL_IDENTITY_AMBIGUOUS`,
`SCOPE_DRIFT`/`UNREGISTERED_CHANGE`/`RECONCILE_AMBIGUOUS` and
`TEST_GUARD_WEAKENED`. Missing and stale evidence share one new code,
`REQUIRED_EVIDENCE_NOT_CURRENT` — the remedy (run the profile) is the
same; the reason tells which. Registry goes 44→45, pinned.

### D-181 — blocking guard findings deny too

`TEST_GUARD_WEAKENED` with `Blocking=true` denies completion. The task
list names four families; the guard is the fifth because MR-012 handed it
over as gate input and ignoring a hard block at completion would be
absurd. No new code needed.

### D-182 — decisions carry no timestamp

`Decision` has no clock field: idempotency is structural, not asserted —
the same inputs decide byte-identical outputs, pinned by double
evaluation.

### D-183 — serial tasks, Reader/Breaker gates, findings rows

The MR-004…MR-012 process repeats: one task at a time, each gated before
the next, each recorded in `mr-013-findings.md` (new file — MR-012's record
stays closed). Post-gate hygiene (`git status`/`git diff` on tracked
paths) applies to every subagent.

### D-184 — no CLI/MCP surface, no migration, no timings

Service and structs only, driven by tests now and by milestone drivers
later (the D-91 pattern). No wall-clock assertions.

---

## 3. Requirements and acceptance criteria

REQ-01…REQ-06. Each task's criteria are dotted under it, in the form the
gates grade. A criterion not met at the end of its task is recorded as not
met, with the reason, in `mr-013-findings.md`.

**REQ-01 — gate core** (ALLOW/DENY, four families + guard, reason codes).
**REQ-02 — reconcile-first + idempotency** (composition, determinism).
**REQ-03 — non-goals, guarded**.
**REQ-04 — evidence discipline** (every new guard mutation-red).
**REQ-05 — surface discipline** (one code; no commands/keys/tables/timings).

### TASK-01 — gate core, evidence code, idempotency

Owns REQ-01, REQ-02.

- **AC-01.1** every denial carries deterministic reason code, provenance
  and `next_action`; families: unresolved blocking invariant, ambiguity
  with active HIGH/CRITICAL invariant, unreconciled-change blocking set,
  missing/stale required evidence, blocking guard findings.
- **AC-01.2** clean inputs decide ALLOW with zero denials; every denial
  names its remedy (override/scope/assignment/run-restore).
- **AC-01.3** same inputs evaluated twice decide byte-identical
  (idempotency structural, no timestamp in the decision).
- **AC-01.4** `REQUIRED_EVIDENCE_NOT_CURRENT` registered with remedy +
  `ExitFailed`; registry 44→45 pinned.
- **AC-01.5** severity gate: orphaned/ambiguous bindings and ambiguities
  without active HIGH/CRITICAL invariants do not deny (warn-scope stays
  outside the gate).

### TASK-02 — kernel E2E, non-goals, record

Owns REQ-03, REQ-04, REQ-05.

- **AC-02.1** kernel E2E: real git-less discovery composition —
  no-baseline change → reconcile discovers → attribution ambiguates →
  DENY naming both claimants; override + fresh evidence + resolved
  invariant → ALLOW; every DENY family fires at least once across the
  suite (kernel scenario + family beats).
- **AC-02.2** the non-goals hold under grep: one new code (registry 45),
  no store calls in the gate (pure composer), no commands/keys/tables.
- **AC-02.3** every new guard added by TASK-01 has its mutation run and
  recorded red (REQ-04), and the task list's MR-013 entry carries a
  `#### Durum` block written the way MR-012's is.

---

## 4. Work breakdown and dependency order

Serialised: each task is gated before the next is written.

| Task | Owns | REQ | Depends on |
|---|---|---|---|
| TASK-01 | gate core, 5 families, evidence code, idempotency | REQ-01, REQ-02 | — |
| TASK-02 | kernel E2E + family beats, non-goals, record, Durum | REQ-03, REQ-04, REQ-05 | TASK-01 |

---

## 5. Definition of done

1. Every acceptance criterion above is met, or is recorded as not met with
   the reason in `mr-013-findings.md`.
2. `make verify` green, `make tidy-check` green, and the test count reported
   with `go test -list '.*' ./... | grep -c '^Test'` — that command and no
   other.
3. Every new guard has a recorded mutation that turns it red, recorded after
   it was run.
4. Every task in §4 has passed its Reader/Breaker gate before the next was
   begun, and the gate's findings — confirmed, refuted, unconfirmed — are in
   `mr-013-findings.md` per task, with what was done about each.
5. The task list's MR-013 entry carries a `#### Durum` block written the way
   MR-012's is.
6. The knowledge graph is refreshed in its own commit at the end.

---

## 6. Traceability to the task list's five acceptance criteria

| Task list AC | Where it is owned |
|---|---|
| Tüm denial nedenleri deterministic reason code, provenance ve çözüm önerisi taşır | AC-01.1 (REQ-01) |
| `before_change` çağrılmamış olsa da completion önce reconcile eder | AC-02.1 via D-179 (REQ-03; composition proof) |
| Gerekli current evidence ve çözümlenmiş invariant olduğunda ALLOW üretir | AC-01.2 (REQ-01) |
| Aynı snapshot ve state için karar tekrarlanabilir/idempotent olur | AC-01.3 (REQ-02) |
| Birincil uçtan uca kernel senaryosu ALLOW ve her DENY dalı için test edilir | AC-02.1 (REQ-03) |
