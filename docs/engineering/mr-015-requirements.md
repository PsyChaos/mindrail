# MR-015 — Frozen requirements and acceptance criteria

- **Frozen at:** commit `5b42d40`, branch `mr-002-knowledge-lifecycle`
- **Contract this refines:** [mr-015-design.md](mr-015-design.md). Where the two
  disagree, the design wins on intent and this document wins on the detail the
  design left open — every such gap is recorded below as a decision D-195…D-204.
- **Task:** [mindrail-0.1-task-list.md](mindrail-0.1-task-list.md) MR-015
- **Tier:** 3 (Program). It adds five tools to the wire vocabulary on the
  existing server, binds coordination/change services, and exercises
  idempotency, revision conflict and structured errors end to end. No
  database schema, no wire codes.

This file is frozen **before** implementation so that an audit has something
to grade against that the implementation did not shape. The milestone runs
**one task at a time** as MR-004 through MR-014 did: each task in §4 is
implemented, gated by an independent Reader/Breaker pair and recorded before
the next is begun.

---

## 0. Baseline evidence

Captured on the tree this work starts from, so a red suite later is unambiguous.

| Measurement | Value | Command |
|---|---|---|
| Commit | `5b42d40` | `git rev-parse --short HEAD` |
| Go toolchain | `go1.27.0 linux/amd64` | `go version` |
| Packages | 34 | `go list ./... \| wc -l` |
| Top-level test functions | 1223 | `go test -list '.*' ./... \| grep -c '^Test'` |
| `make verify` | green at MR-014 close | `make verify` |
| `make tidy-check` | green at MR-014 close | `make tidy-check` |
| Migrations | `000001_initial.sql` … `000008_evidence.sql`; runtime `schema_version` reports 8 | `ls migrations/*.sql` |
| Registered `app.Code` values | 46 | `grep -c 'Code = "' internal/app/code.go` |
| MCP tools | 6 (bootstrap/status/search/context/decide/invariant) | ListTools |

---

## 1. The scenario, in one paragraph

An agent claims a task, declares scope, edits, runs after_change and reads
back the change plus its attribution findings; a second agent reconciles
the same tree without ever declaring and gets the same discovered rows;
both retries with the same operation id replay instead of duplicating; a
stale revision conflicts loudly with the remedy attached; and a checkpoint
written under one session reads back under another. Every answer carries
pending/partial state or next actions where the work is not done.

---

## 2. Decisions

MR-014 ended at D-194. These continue the sequence.

### D-195 — one server, ModeWrite, eleven tools

The server starts `ModeWrite` (existing DB, no creation/migration) and
serves all eleven tools. Read tools behave identically under either
handle; the mode change is recorded, not re-tested per tool.

### D-196 — tools bind services, claim maps to Transition

`claim` = `Transition` to CLAIMED under a named session (revision-guarded
variant via `TransitionExpecting`); `before_change` = `CaptureBaseline`;
`after_change` = `AfterChange` + `EvaluateTask`; `reconcile` =
`Reconcile` (exec git runner over the root) + `EvaluateTask`;
`checkpoint` = `WriteCheckpoint`, or `Handover`/`LastCheckpoint` when no
note is given (present-vs-absent, the D-188 pattern).

### D-197 — before_change stays optional, reconcile stays canonical

Declaring scope is an optimization agents may skip: every proof path runs
a never-declared flow and shows reconcile discovering the same rows.
No test may depend on a baseline existing unless it created it.

### D-198 — idempotency and conflict ride existing machinery

Same operation id replays (coordination `Write` replay, change
`withOperation`, evidence op scope — all pre-existing); stale revisions
conflict with the coordination error payload (code + remedy) surfacing as
the tool error. No new codes: conflicts and ambiguities already have
vocabulary (registry stays 46, pinned).

### D-199 — results carry state, not just data

Change answers include pending/partial posture (open change, blocking
findings with next actions); checkpoint answers include handover
readability; conflicts include expected/actual revision. A bare row
without its state is an incomplete answer, pinned per tool.

### D-200 — checkpoint read is the same tool, note-absent

`checkpoint{note}` writes; `checkpoint{}` (no note) reads the last
checkpoint/handover. One tool, two directions, distinguished by presence
— the D-188 pattern a third time.

### D-201 — attribution of writes names sessions, never invents them

Claim/checkpoint writes attribute to caller-named sessions
(`NamedSession`); unknown sessions refuse before any write (the stores'
own rule, preserved through the tools).

### D-202 — secret values never cross tool boundaries differently

Tool inputs/outputs carry no secrets by construction (ids, paths, notes);
the redaction boundary stays MR-010's. Diagnostics echo names and keys,
never values (spec §19, MR-014 precedent).

### D-203 — serial tasks, Reader/Breaker gates, findings rows

The MR-004…MR-014 process repeats: one task at a time, each gated before
the next, each recorded in `mr-015-findings.md` (new file — MR-014's record
stays closed). Post-gate hygiene (`git status`/`git diff` on tracked
paths) applies to every subagent.

### D-204 — contract is structs + behaviors, not prose

Tool In/Out structs plus idempotency/conflict/ambiguity behaviors ARE the
contract, pinned by tests driving two clients where identity could
matter. Prose describes; structs decide.

---

## 3. Requirements and acceptance criteria

REQ-01…REQ-07. Each task's criteria are dotted under it, in the form the
gates grade. A criterion not met at the end of its task is recorded as not
met, with the reason, in `mr-015-findings.md`.

**REQ-01 — five tools** (claim/before/after/reconcile/checkpoint wiring).
**REQ-02 — idempotency + conflict** (replay, revision conflict, errors).
**REQ-03 — lifecycle proof** (claim→scope→edit→after→reconcile→checkpoint across sessions).
**REQ-04 — state discipline** (pending/partial/next_action in results).
**REQ-05 — non-goals, guarded**.
**REQ-06 — evidence discipline** (every new guard mutation-red).
**REQ-07 — surface discipline** (no codes/tables/CLI/timings; 11 tools).

### TASK-01 — claim, before_change, after_change

Owns REQ-01 (claim/before/after), REQ-02 (op replay on these), REQ-04 (their state).

- **AC-01.1** claim transitions an OPEN task to CLAIMED under a named
  session, returning task + revision; unknown sessions/tasks refuse
  before any write; expected_revision mismatch conflicts with code +
  remedy (no write).
- **AC-01.2** before_change captures the baseline scope and returns its
  summary; works with or without a prior claim (claim never required).
- **AC-01.3** after_change returns the open change plus its attribution
  findings (blocking items with next_action); same operation id replays
  the change without duplicating.
- **AC-01.4** malformed inputs (empty ids, unknown profiles of input)
  refuse before side effects; diagnostics name parameters, never values.

### TASK-02 — reconcile, checkpoint, lifecycle proof

Owns REQ-01 (reconcile/checkpoint), REQ-02 (their replay/conflict), REQ-03, REQ-04.

- **AC-02.1** reconcile discovers undeclared edits into the task's change
  (same rows a declared flow would hold) plus attribution findings;
  same operation id replays; reconcile-first needs no baseline.
- **AC-02.2** checkpoint writes under one session and reads back under
  another (handover readability); note-absent reads, note-present
  writes; unknown sessions refuse.
- **AC-02.3** full lifecycle across two client identities: claim →
  declare → edit → after → reconcile (second identity, undeclared) →
  checkpoint → read-back, with pending/partial state and next actions
  visible at every step that is not done.
- **AC-02.4** registry holds eleven tools exactly, no new codes (46),
  no tables, no CLI command.

### TASK-03 — proof, non-goals and the record

Owns REQ-05, REQ-06, REQ-07.

- **AC-03.1** full-suite green evidence: `make verify`, `make tidy-check`,
  test count by the single command.
- **AC-03.2** the non-goals hold under grep: no new codes (registry 46),
  eleven tools exactly, no tables, no value echo, before_change optional
  in every proof path (no test depends on undeclared baselines existing).
- **AC-03.3** every new guard added by TASK-01…02 has its mutation run and
  recorded red (REQ-06), and the task list's MR-015 entry carries a
  `#### Durum` block written the way MR-014's is.

---

## 4. Work breakdown and dependency order

Serialised: each task is gated before the next is written.

| Task | Owns | REQ | Depends on |
|---|---|---|---|
| TASK-01 | claim, before_change, after_change + their replay/state | REQ-01, REQ-02, REQ-04 | — |
| TASK-02 | reconcile, checkpoint + lifecycle proof + registry | REQ-01, REQ-02, REQ-03, REQ-04 | TASK-01 |
| TASK-03 | proof, non-goals, record, Durum block | REQ-05, REQ-06, REQ-07 | TASK-02 |

---

## 5. Definition of done

1. Every acceptance criterion above is met, or is recorded as not met with
   the reason in `mr-015-findings.md`.
2. `make verify` green, `make tidy-check` green, and the test count reported
   with `go test -list '.*' ./... | grep -c '^Test'` — that command and no
   other.
3. Every new guard has a recorded mutation that turns it red, recorded after
   it was run.
4. Every task in §4 has passed its Reader/Breaker gate before the next was
   begun, and the gate's findings — confirmed, refuted, unconfirmed — are in
   `mr-015-findings.md` per task, with what was done about each.
5. The task list's MR-015 entry carries a `#### Durum` block written the way
   MR-014's is.
6. The knowledge graph is refreshed in its own commit at the end.

---

## 6. Traceability to the task list's four acceptance criteria

| Task list AC | Where it is owned |
|---|---|
| Beş aracın başarı, idempotent retry, conflict ve ambiguity contract testleri | AC-01.1, AC-01.3, AC-02.1, AC-02.2 (REQ-01, REQ-02) |
| `before_change` opsiyonel, `reconcile` kanonik | AC-01.2, AC-02.1, AC-03.2 (REQ-01, REQ-03, REQ-05; D-197) |
| Checkpoint farklı session tarafından okunabilir | AC-02.2 (REQ-01) |
| Pending/partial + `next_action` | AC-01.3, AC-02.3, REQ-04 |
