# MR-008 — Frozen requirements and acceptance criteria

- **Frozen at:** commit `fddeeec`, branch `mr-002-knowledge-lifecycle`
- **Contract this refines:** [mr-008-design.md](mr-008-design.md). Where the two
  disagree, the design wins on intent and this document wins on the detail the
  design left open — every such gap is recorded below as a decision D-132…D-145.
- **Task:** [mindrail-0.1-task-list.md](mindrail-0.1-task-list.md) MR-008
- **Tier:** 3 (Program). It adds a runtime migration (one table), three
  `app.Code` values that enter the wire vocabulary, the attribution core that
  decides which Change owns a symbol, and the blocking evaluation
  completion/verify will consume. The escalation rule fires on "public API
  contract" and "database schema" regardless of line count.

This file is frozen **before** implementation so that an audit has something
to grade against that the implementation did not shape. The milestone runs
**one task at a time** as MR-004 through MR-007 did: each task in §4 is
implemented, gated by an independent Reader/Breaker pair and recorded before
the next is begun.

---

## 0. Baseline evidence

Captured on the tree this work starts from, so a red suite later is unambiguous.

| Measurement | Value | Command |
|---|---|---|
| Commit | `fddeeec` | `git rev-parse --short HEAD` |
| Go toolchain | `go1.27.0 linux/amd64` | `go version` |
| Packages | 29 | `go list ./... \| wc -l` |
| Top-level test functions | 1122 | `go test -list '.*' ./... \| grep -c '^Test'` |
| `make verify` | green — exit 0 across check, race and smoke | `make verify` |
| `make tidy-check` | green, exit 0 | `make tidy-check` |
| Migrations | `000001_initial.sql` … `000006_changes.sql`; runtime `schema_version` reports 6 | `ls migrations/*.sql` |
| Registered `app.Code` values | 40 | `grep -c 'Code = "' internal/app/code.go` |
| Attribution records | do not exist (no table, no type) | schema |

---

## 1. The scenario, in one paragraph

Two tasks declare overlapping scopes and both touch `a.py`; a third edit
lands in a file no baseline covers. Mindrail attributes each changed symbol
to exactly one Change where it can, and refuses where it cannot: the shared
file's symbols become explicit ambiguity naming both candidate Changes, the
uncovered file becomes an unregistered-change finding, and both block
completion until a human assigns them or narrows the scopes. Nothing is
auto-assigned, nothing goes silent, and a lease holder is named in the
guidance — never used as a silent tiebreaker.

---

## 2. Decisions

MR-007 ended at D-131. These continue the sequence.

### D-132 — declared scope is the baseline, nothing else

A task's declared scope is exactly its `change_baselines` paths. Claims,
leases and task titles do not widen it: they are checked against it
(lease-conflict guidance) but never extend it. A task with no baseline
declares nothing — every one of its discovered files is unregistered scope.

### D-133 — candidates are open changes whose baseline covers the file

A symbol's candidate Changes are the open Changes of tasks whose baseline
scope contains the symbol's file. One candidate attributes; zero is
unregistered; two or more is ambiguous. File membership is the only
criterion in 0.1 — per-symbol scope narrowing does not exist yet.

### D-134 — leases advise, never attribute

A lease holder on the file or symbol is named in the finding's `next_action`
("confirm with session X, which holds the lease") when exactly one lease
covers it. Leases never resolve ambiguity and never create ownership: the
moment a lease silently assigned work, the gate would launder one agent's
edit as another's.

### D-135 — three codes with distinct remedies

`SCOPE_DRIFT`, `UNREGISTERED_CHANGE` and `RECONCILE_AMBIGUOUS` (the spec
§66 name, kept verbatim) enter `allCodes` with distinct remedies. No other
vocabulary is added.

### D-136 — findings are pure evaluation, stored nowhere

Attribution runs as a deterministic function of baselines, change rows,
tasks and leases. MR-017's CI verify re-runs the same evaluation on a fresh
clone instead of trusting stored verdicts — stored findings would be a
second truth to drift. What persists is inputs (baselines, rows, overrides),
never outputs.

### D-137 — overrides are explicit, validated, and honored first

`scope_attributions(logical_key → change_id, decided_by, reason)` records a
human/agent assignment. An override resolves its symbol to the recorded
Change before candidate counting — including away from ambiguity and away
from unregistered. `decided_by` is a non-empty free string (session or agent
id); empty deciders and unknown changes are refused. Overrides never expire
in 0.1; superseding one writes a new row over the same key.

### D-138 — everything ambiguous or unregistered blocks in 0.1

Ambiguous and unregistered findings carry `Blocking=true`, as do scope-drift
file findings. Resolution paths exist for every block (attribute, narrow the
scope, extend the baseline), so blocking is strict without being terminal.
MR-013 may refine per-severity behavior; it will not need new signals.

### D-139 — drift is per file, attribution per symbol

Scope drift fires per changed file outside baseline scope (reusing the
divergence shape plus the code). Symbol attribution runs per changed symbol
row. A drifted file's symbols additionally resolve through the candidate
rule — usually to unregistered, sometimes to ambiguous — and each layer
blocks on its own finding.

### D-140 — attribution reads open changes only

Candidates come from tasks with open Changes (a `changes` row exists for the
task). Closed/completed states do not exist in 0.1, so every `changes` row
is open by construction; the rule is written for the state machine MR-013
will add, not against today's rows.

### D-141 — serial tasks, Reader/Breaker gates, findings rows

The MR-004…MR-007 process repeats: one task at a time, each gated before
the next, each recorded in `mr-008-findings.md` (new file — MR-007's record
stays closed).

### D-142 — migration 000007 and version 7

`migrations/000007_scope_attribution.sql` creates `scope_attributions`;
`changes.TableSchemaVersion = 7`. The ledger's expected-columns list is
amended (D-73 forward, fourth time).

### D-143 — no CLI/MCP surface, no status/doctor keys, no timings

Service and store only, driven by tests now and by milestone drivers later
(the D-91 pattern). The findings travel as Go structs with reason codes;
rendering belongs to MR-013+.

### D-144 — knowledge schema frozen, leases read as input

Attribution never writes `.mindrail/`. Leases arrive as caller-provided
`LeaseView{Holder, Kind, Key} (Kind mirrors coordination `task`|`file`; file leases match by path, task leases by candidate task)` values (the TASK-03 invariants-input
pattern), never through a coordination store import — the changes package
stays free of coordination writes, and ownership stays with MR-004's domain.

### D-145 — untracked discovery gaps stay open questions, not verdicts

Evaluation reads stored change rows and baselines, never the live
filesystem: files changed but undiscovered (no reconcile run yet) are
outside every verdict by construction. Stale-row pruning (D-113) stays
future work and bounds this the same way.

---

## 3. Requirements and acceptance criteria

REQ-01…REQ-09. Each task's criteria are dotted under it, in the form the
gates grade. A criterion not met at the end of its task is recorded as not
met, with the reason, in `mr-008-findings.md`.

**REQ-01 — schema and codes** (migration 000007, version gate, three codes).
**REQ-02 — attribution core** (candidates, outcomes, drift findings).
**REQ-03 — unregistered track** (no-baseline tasks and files).
**REQ-04 — overrides** (record, validate, honor).
**REQ-05 — blocking evaluation** (completion/verify surface).
**REQ-06 — lease guidance** (name holders, never attribute).
**REQ-07 — non-goals, guarded**.
**REQ-08 — evidence discipline** (every new guard mutation-red).
**REQ-09 — surface discipline** (no commands/keys/timings/codes beyond REQ-01).

### TASK-01 — migration 000007, codes, finding vocabulary

Owns REQ-01.

- **AC-01.1** `migrations/000007_scope_attribution.sql` creates
  `scope_attributions` (`logical_key` PK, change FK, decider, reason,
  decided_at) — and nothing else.
- **AC-01.2** `changes.TableSchemaVersion = 7` gates the store; the
  migration applies cleanly to a schema-6 database carrying MR-007 rows,
  and `init` reports `schema_version` 7.
- **AC-01.3** the three codes of D-135 are registered with distinct
  remedies and enter `allCodes` (the registry test enforces it).
- **AC-01.4** `migrations/shipped_test.go` pins the new file's bytes; the
  ledger's expected-columns list covers the new table (D-73 forward).
- **AC-01.5** no knowledge schema changes, no new commands/keys/timings;
  only the `schema_version` 6→7 number rides the goldens.

### TASK-02 — attribution core and drift findings

Owns REQ-02.

- **AC-02.1** one candidate attributes: the outcome names the change, no
  finding, and repeated evaluation is stable (pure function, same rows).
- **AC-02.2** zero candidates is unregistered: finding carries
  `UNREGISTERED_CHANGE` with provenance (discovered_via, baseline
  absence) and `next_action`, and blocks.
- **AC-02.3** two or more candidates is ambiguous: finding carries
  `RECONCILE_AMBIGUOUS` naming every candidate change, and blocks; no
  candidate is chosen.
- **AC-02.4** scope drift fires per file outside baseline scope with
  `SCOPE_DRIFT`, path, kind and reason, and blocks; in-scope files never
  drift.
- **AC-02.5** every finding carries provenance (which baseline rows were
  read, which discovery rows were judged) and an actionable `next_action`;
  findings never invent paths.

### TASK-03 — overrides and blocking evaluation

Owns REQ-03, REQ-04, REQ-05, REQ-06.

- **AC-03.1** recording an override for an ambiguous symbol resolves it to
  the recorded change on the next evaluation (finding gone); recording for
  an unregistered symbol binds it the same way.
- **AC-03.2** empty deciders, unknown changes and malformed keys are
  refused before any write (fail-closed validation).
- **AC-03.3** `EvaluateTask` returns the blocking set for completion: every
  ambiguous/unregistered/drifted item with code, provenance and
  `next_action`; a clean task returns empty (ALLOW-shaped, no verdict
  invented here).
- **AC-03.4** lease guidance: a finding with exactly one covering lease
  names its holder in `next_action`; zero or several leases name none —
  and attribution never follows the lease either way (pinned both
  directions).
- **AC-03.5** unregistered work with no baseline task (NULL-task changes,
  never-captured tasks) evaluates through the unregistered track, never
  through an error path.

### TASK-04 — proof, non-goals and the record

Owns REQ-07, REQ-08, REQ-09.

- **AC-04.1** end-to-end: two tasks with overlapping baselines share a
  changed file — its symbols ambiguate naming both changes and block;
  human override clears one symbol while the other stays blocked; a third
  file outside both scopes drifts and blocks; resolving all three clears
  the evaluation.
- **AC-04.2** the non-goals hold under grep: no impact/evidence/coverage
  machinery, no new codes/commands/keys beyond REQ-01, knowledge schema
  still v1.
- **AC-04.3** every new guard added by TASK-01…03 has its mutation run and
  recorded red (REQ-08), and the task list's MR-008 entry carries a
  `#### Durum` block written the way MR-007's is.

---

## 4. Work breakdown and dependency order

Serialised: each task is gated before the next is written.

| Task | Owns | REQ | Depends on |
|---|---|---|---|
| TASK-01 | migration 000007, version gate, three codes, finding types | REQ-01 | — |
| TASK-02 | attribution outcomes, drift findings, unregistered track | REQ-02 | TASK-01 |
| TASK-03 | overrides, blocking evaluation, lease guidance | REQ-03, REQ-04, REQ-05, REQ-06 | TASK-02 |
| TASK-04 | E2E proof, non-goals, record, Durum block | REQ-07, REQ-08, REQ-09 | TASK-03 |

---

## 5. Definition of done

1. Every acceptance criterion above is met, or is recorded as not met with
   the reason in `mr-008-findings.md`.
2. `make verify` green, `make tidy-check` green, and the test count reported
   with `go test -list '.*' ./... | grep -c '^Test'` — that command and no
   other.
3. Every new guard has a recorded mutation that turns it red, recorded after
   it was run.
4. Every task in §4 has passed its Reader/Breaker gate before the next was
   begun, and the gate's findings — confirmed, refuted, unconfirmed — are in
   `mr-008-findings.md` per task, with what was done about each.
5. The task list's MR-008 entry carries a `#### Durum` block written the way
   MR-007's is.
6. The knowledge graph is refreshed in its own commit at the end.

---

## 6. Traceability to the task list's four acceptance criteria

| Task list AC | Where it is owned |
|---|---|
| Beyan edilen scope dışındaki değişiklik drift bulgusu üretir | AC-02.4 (REQ-02) |
| Birden fazla Change adayı bulunan sembol otomatik sahiplenilmez | AC-02.3 (REQ-02) |
| Unregistered/ambiguous completion/verify'de bloklanabilir | AC-02.2, AC-03.3, AC-03.5 (REQ-02, REQ-05) |
| Provenance + uygulanabilir `next_action` | AC-02.5 (REQ-02) |
