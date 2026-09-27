# MR-012 — Frozen requirements and acceptance criteria

- **Frozen at:** commit `36c2af5`, branch `mr-002-knowledge-lifecycle`
- **Contract this refines:** [mr-012-design.md](mr-012-design.md). Where the two
  disagree, the design wins on intent and this document wins on the detail the
  design left open — every such gap is recorded below as a decision D-170…D-178.
- **Task:** [mindrail-0.1-task-list.md](mindrail-0.1-task-list.md) MR-012
- **Tier:** 3 (Program). It adds a new package (`internal/testguard`) with
  tree-sitter queries, one `app.Code` (`TEST_GUARD_WEAKENED`) entering the
  wire vocabulary, and the weakening detector MR-013's gate will consume.

This file is frozen **before** implementation so that an audit has something
to grade against that the implementation did not shape. The milestone runs
**one task at a time** as MR-004 through MR-011 did: each task in §4 is
implemented, gated by an independent Reader/Breaker pair and recorded before
the next is begun.

---

## 0. Baseline evidence

Captured on the tree this work starts from, so a red suite later is unambiguous.

| Measurement | Value | Command |
|---|---|---|
| Commit | `36c2af5` | `git rev-parse --short HEAD` |
| Go toolchain | `go1.27.0 linux/amd64` | `go version` |
| Packages | 31 | `go list ./... \| wc -l` |
| Top-level test functions | 1185 | `go test -list '.*' ./... \| grep -c '^Test'` |
| `make verify` | green at MR-011 close | `make verify` |
| `make tidy-check` | green at MR-011 close | `make tidy-check` |
| Migrations | `000001_initial.sql` … `000008_evidence.sql`; runtime `schema_version` reports 8 | `ls migrations/*.sql` |
| Registered `app.Code` values | 43 | `grep -c 'Code = "' internal/app/code.go` |
| Guard findings | do not exist (no code, no type) | code + schema |

---

## 1. The scenario, in one paragraph

A change deletes three asserts from `test_auth`, marks `test_session`
`pytest.mark.skip`, and removes `test_token` — the test that calls the
production symbol a CRITICAL invariant watches. The guard reports three
findings, each with test symbol, related production symbol, before/after
summaries, reason and confidence: the skip and the assertion loss warn,
while the removed CRITICAL verification test blocks — even though no
production file changed. A sibling test carrying a `mindrail:
allow-test-weakening` note is listed as suppressed, never silently
dropped. The same evaluation serves after_change, reconcile, staged and
CI callers with only the provenance string differing.

---

## 2. Decisions

MR-011 ended at D-169. These continue the sequence.

### D-170 — pure service over before/after bytes, trigger as provenance

The guard reads no store: one `Evaluate` over test-file deltas
(path, language, before/after bytes) plus caller-supplied invariant
mapping. The trigger path (after_change, reconcile, staged, ci) arrives
as a provenance string only — shared service by construction, identical
analysis on every path.

### D-171 — test identity is narrow and documented

Python: `def test_*`. TypeScript/JavaScript: `it(` / `test(` with a
string-literal name. Markers are the spec list only
(pytest.mark.skip/xfail, unittest.skip, it/test/describe.skip); near
cousins (`xit`, `test.todo`, `pytestmark`, conditional skipif) do not
exist in 0.1 and are recorded, not guessed.

### D-172 — invariant mapping arrives as caller input

Which test verifies which invariant (ids + severity + active flag) is
input, not computed: reference-based mapping is the intended later rule,
but computing it needs drivers that do not exist yet. The guard judges
the mapping it is given — garbage mapping judges garbage, loudly (unknown
test keys are refused, not skipped).

### D-173 — one code, narrow blocking

`TEST_GUARD_WEAKENED` enters `allCodes` with `ExitFailed`. Blocking iff
the mapped invariant is active CRITICAL **and** the signal is removal or
disable. Skip/xfail/assertion-loss on CRITICAL tests warn (spec policy:
HIGH wants stronger evidence, not a hard block) — MR-013 owns what
follows a warning.

### D-174 — confidence is the structural constant

Every finding carries 0.5: the analysis is structural by construction
(tree-sitter nodes, never text search), and the number says exactly that —
the same vocabulary as MR-005/MR-009, never a heuristic bump.

### D-175 — escape hatch is a self-disclosing marker

A test function annotated `mindrail: allow-test-weakening` suppresses its
findings, and every suppression is listed in the result with the test
named. Nothing suppresses silently; the marker without weakening reports
nothing (no reward for decoration).

### D-176 — every detected signal is reported

Mapped or not, every assertion-loss/skip/disable/removal finding is
emitted; mapping + severity decide blocking, never existence. Test-only
changes evaluate identically — production edits are not required.

### D-177 — serial tasks, Reader/Breaker gates, findings rows

The MR-004…MR-011 process repeats: one task at a time, each gated before
the next, each recorded in `mr-012-findings.md` (new file — MR-011's record
stays closed). Post-gate hygiene (`git status`/`git diff` on tracked
paths) applies to every subagent.

### D-178 — no CLI/MCP surface, no migration, no timings

Service and structs only, driven by tests now and by milestone drivers
later (the D-91 pattern). No wall-clock assertions.

---

## 3. Requirements and acceptance criteria

REQ-01…REQ-07. Each task's criteria are dotted under it, in the form the
gates grade. A criterion not met at the end of its task is recorded as not
met, with the reason, in `mr-012-findings.md`.

**REQ-01 — Python guard** (assert/remove/skip/xfail + finding shape).
**REQ-02 — TS/JS guard** (expect/remove/skip variants).
**REQ-03 — invariant mapping + blocking** (CRITICAL remove/disable blocks).
**REQ-04 — shared service + escape hatch** (4 provenances, marker, policy).
**REQ-05 — non-goals, guarded**.
**REQ-06 — evidence discipline** (every new guard mutation-red).
**REQ-07 — surface discipline** (one code; no commands/keys/timings/tables).

### TASK-01 — Python guard, finding vocabulary, code

Owns REQ-01.

- **AC-01.1** assertion removal and count decrease detected per
  `def test_*` (before/after summaries carry the counts); zero-assert
  remainder keeps the function named (no silent pass-through).
- **AC-01.2** skip/xfail/disabled markers detected (spec list);
  near-cousins explicitly not detected (pinned negative cases).
- **AC-01.3** removed test functions detected with before-summary;
  added tests are quiet (no finding for new strength).
- **AC-01.4** `TEST_GUARD_WEAKENED` registered with remedy + `ExitFailed`;
  every finding carries test symbol, related production symbol (empty
  when unmapped), before/after summaries, reason, confidence 0.5.
- **AC-01.5** tree-sitter nodes, never text search (pinned: assert-like
  text inside strings/comments does not count).

### TASK-02 — TS/JS guard, invariant mapping, blocking, hatch

Owns REQ-02, REQ-03, REQ-04.

- **AC-02.1** `expect(` removal/count-decrease and skip variants
  (`it/test/describe.skip`) detected per named test; removed tests
  detected; added tests quiet.
- **AC-02.2** mapped CRITICAL removal/disable blocks; mapped CRITICAL
  skip/xfail/assertion-loss warns; unmapped findings never block;
  unknown test keys in mapping refused before analysis.
- **AC-02.3** shared entry: identical findings across after_change,
  reconcile, staged and ci provenances (only the provenance string
  differs).
- **AC-02.4** escape hatch: marked tests suppress with self-disclosing
  listing; unmarked behavior unchanged; marker-without-weakening quiet.
- **AC-02.5** policy matrix (signal × severity → block/warn) documented
  and tested; near-cousin non-detections documented.

### TASK-03 — proof, non-goals and the record

Owns REQ-05, REQ-06, REQ-07.

- **AC-03.1** end-to-end: the §1 scenario in one evaluation (3 findings,
  1 blocking, 1 suppression listed) across all four provenances.
- **AC-03.2** the non-goals hold under grep: one new code (registry 44),
  no commands/keys/tables, tree-sitter only (no text-search fallback).
- **AC-03.3** every new guard added by TASK-01…02 has its mutation run and
  recorded red (REQ-06), and the task list's MR-012 entry carries a
  `#### Durum` block written the way MR-011's is.

---

## 4. Work breakdown and dependency order

Serialised: each task is gated before the next is written.

| Task | Owns | REQ | Depends on |
|---|---|---|---|
| TASK-01 | Python guard, finding type, TEST_GUARD_WEAKENED | REQ-01 | — |
| TASK-02 | TS/JS guard, mapping + blocking, hatch + policy | REQ-02, REQ-03, REQ-04 | TASK-01 |
| TASK-03 | E2E proof, non-goals, record, Durum block | REQ-05, REQ-06, REQ-07 | TASK-02 |

---

## 5. Definition of done

1. Every acceptance criterion above is met, or is recorded as not met with
   the reason in `mr-012-findings.md`.
2. `make verify` green, `make tidy-check` green, and the test count reported
   with `go test -list '.*' ./... | grep -c '^Test'` — that command and no
   other.
3. Every new guard has a recorded mutation that turns it red, recorded after
   it was run.
4. Every task in §4 has passed its Reader/Breaker gate before the next was
   begun, and the gate's findings — confirmed, refuted, unconfirmed — are in
   `mr-012-findings.md` per task, with what was done about each.
5. The task list's MR-012 entry carries a `#### Durum` block written the way
   MR-011's is.
6. The knowledge graph is refreshed in its own commit at the end.

---

## 6. Traceability to the task list's five acceptance criteria

| Task list AC | Where it is owned |
|---|---|
| Assertion kaldırma/azaltma ile skip/xfail/disabled ekleme fixture'ları yakalanır | AC-01.1, AC-01.2, AC-02.1 (REQ-01, REQ-02) |
| Aktif CRITICAL invariant verification testinin kaldırılması blocking bulgu üretir | AC-02.2 (REQ-03) |
| Yalnız test dosyasına dokunan değişiklik de guard tarafından değerlendirilir | AC-02.2 via D-176 (REQ-03; production edits never required) |
| Aynı guard reconcile, staged verify ve CI verify yollarında ortak servis olarak kullanılır | AC-02.3 (REQ-04) |
| Açıklanabilir false-positive escape hatch/policy davranışı belgelenir ve test edilir | AC-02.4, AC-02.5 (REQ-04) |
