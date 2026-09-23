# MR-011 — Frozen requirements and acceptance criteria

- **Frozen at:** commit `5aaaae3`, branch `mr-002-knowledge-lifecycle`
- **Contract this refines:** [mr-011-design.md](mr-011-design.md). Where the two
  disagree, the design wins on intent and this document wins on the detail the
  design left open — every such gap is recorded below as a decision D-165…D-169.
- **Task:** [mindrail-0.1-task-list.md](mindrail-0.1-task-list.md) MR-011
- **Tier:** 2 (Normal). It adds no tables, no codes, no config: one pure
  evaluation over MR-010's rows plus the tests that pin it. The escalation
  rule does not fire.

This file is frozen **before** implementation so that an audit has something
to grade against that the implementation did not shape. The milestone runs
**one task at a time** as MR-004 through MR-010 did: each task in §4 is
implemented, gated by an independent Reader/Breaker pair and recorded before
the next is begun.

---

## 0. Baseline evidence

Captured on the tree this work starts from, so a red suite later is unambiguous.

| Measurement | Value | Command |
|---|---|---|
| Commit | `5aaaae3` | `git rev-parse --short HEAD` |
| Go toolchain | `go1.27.0 linux/amd64` | `go version` |
| Packages | 31 | `go list ./... \| wc -l` |
| Top-level test functions | 1174 | `go test -list '.*' ./... \| grep -c '^Test'` |
| `make verify` | green at MR-010 close | `make verify` |
| `make tidy-check` | green at MR-010 close | `make tidy-check` |
| Migrations | `000001_initial.sql` … `000008_evidence.sql`; runtime `schema_version` reports 8 | `ls migrations/*.sql` |
| Registered `app.Code` values | 43 | `grep -c 'Code = "' internal/app/code.go` |
| Freshness verdicts | do not exist (no type, no stored flag) | schema + code |

---

## 1. The scenario, in one paragraph

A `test` profile passes over `tests/` and its evidence binds hash H1.
Nobody touches `tests/` — the next check still calls it current. Then an
edit lands in `tests/`: the same check now calls it stale, names the
profile for re-run, and a required-profile list containing `test` reports
unsatisfied until a fresh run lands. An edit elsewhere never disturbs it.
Nothing is rewritten: the old row stays, the verdict recomputes.

---

## 2. Decisions

MR-010 ended at D-164. These continue the sequence.

### D-165 — staleness is computed, never stored

MR-010's append-only rule holds absolutely: no UPDATE, no stale flag, no
new table. Freshness is a pure function of the stored row (scope +
snapshot hash) and the live tree, recomputed on every check — the D-136
pattern a third time.

### D-166 — relevance is scope-hash mismatch, absence is stale

An evidence row goes stale exactly when its own scope re-hashes
differently. Files that vanished or unreadable scope fail the re-hash:
fail-safe to stale with the reason naming the gap, never to current on a
partial read.

### D-167 — required profiles are caller input, re-run is a union

The check takes required profile names from the caller (MR-013 will own
the policy). Coverage holds for a required profile with ≥1 current row.
The re-run list is the union of stale rows' profiles and required
profiles without current coverage — every name on it has its reason
beside it.

### D-168 — no codes, no migration, no config, no CLI, no timings

Freshness verdicts are values for MR-013's gate, never blockers
themselves: the registry stays 43, pinned. Service method only, driven by
tests now and by milestone drivers later (the D-91 pattern). No
wall-clock assertions.

### D-169 — serial tasks, Reader/Breaker gates, findings rows

The MR-004…MR-010 process repeats: one task at a time, each gated before
the next, each recorded in `mr-011-findings.md` (new file — MR-010's record
stays closed). Post-gate hygiene (`git status`/`git diff` on tracked
paths) applies to every subagent, per the MR-010 process note.

---

## 3. Requirements and acceptance criteria

REQ-01…REQ-05. Each task's criteria are dotted under it, in the form the
gates grade. A criterion not met at the end of its task is recorded as not
met, with the reason, in `mr-011-findings.md`.

**REQ-01 — freshness evaluation** (re-hash, current/stale, reasons).
**REQ-02 — required coverage** (caller input, unsatisfied, re-run union).
**REQ-03 — non-goals, guarded**.
**REQ-04 — evidence discipline** (every new guard mutation-red).
**REQ-05 — surface discipline** (no codes/migration/config/commands/keys/timings).

### TASK-01 — freshness verdicts and required coverage

Owns REQ-01, REQ-02.

- **AC-01.1** unchanged scope re-hashes equal: the row evaluates current
  with its profile, hash and scope named.
- **AC-01.2** a relevant post-validation edit (scope content change, added
  or removed file in scope, unreadable scope) evaluates stale with the
  reason naming the gap and the profile for re-run.
- **AC-01.3** an edit outside the row's scope leaves it current.
- **AC-01.4** required coverage: every required profile with ≥1 current
  row is satisfied; stale-only or missing profiles are unsatisfied with
  reason; the re-run list unions stale rows' profiles and uncovered
  required profiles, each with its reason.
- **AC-01.5** evaluation is read-only: no UPDATE/DELETE/INSERT touches
  evidence (pinned by row-count + content comparison, not by trust).

### TASK-02 — proof, non-goals and the record

Owns REQ-03, REQ-04, REQ-05.

- **AC-02.1** end-to-end on a real tree: run → current; relevant edit →
  stale + named re-run + required unsatisfied; unrelated edit → current;
  second run → current again with two rows coexisting (append-only
  visible).
- **AC-02.2** the non-goals hold under grep: no UPDATE/DELETE on
  evidence, no new codes (registry still 43), no migration, no config
  surface.
- **AC-02.3** every new guard added by TASK-01 has its mutation run and
  recorded red (REQ-04), and the task list's MR-011 entry carries a
  `#### Durum` block written the way MR-010's is.

---

## 4. Work breakdown and dependency order

Serialised: each task is gated before the next is written.

| Task | Owns | REQ | Depends on |
|---|---|---|---|
| TASK-01 | freshness verdicts, required coverage, re-run union | REQ-01, REQ-02 | — |
| TASK-02 | E2E proof, non-goals, record, Durum block | REQ-03, REQ-04, REQ-05 | TASK-01 |

---

## 5. Definition of done

1. Every acceptance criterion above is met, or is recorded as not met with
   the reason in `mr-011-findings.md`.
2. `make verify` green, `make tidy-check` green, and the test count reported
   with `go test -list '.*' ./... | grep -c '^Test'` — that command and no
   other.
3. Every new guard has a recorded mutation that turns it red, recorded after
   it was run.
4. Every task in §4 has passed its Reader/Breaker gate before the next was
   begun, and the gate's findings — confirmed, refuted, unconfirmed — are in
   `mr-011-findings.md` per task, with what was done about each.
5. The task list's MR-011 entry carries a `#### Durum` block written the way
   MR-010's is.
6. The knowledge graph is refreshed in its own commit at the end.

---

## 6. Traceability to the task list's four acceptance criteria

| Task list AC | Where it is owned |
|---|---|
| Değişmemiş snapshot için kanıt current kalır | AC-01.1 (REQ-01) |
| Validation sonrasındaki ilgili source edit kanıtı stale yapar | AC-01.2 (REQ-01) |
| Stale kanıt completion için required evidence şartını karşılamaz | AC-01.4 (REQ-02) |
| Stale nedeni ve yeniden çalıştırılması gereken profil kullanıcıya açıklanır | AC-01.2, AC-01.4 (REQ-01, REQ-02) |
