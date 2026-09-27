# MR-016 — Frozen requirements and acceptance criteria

- **Frozen at:** commit `c60d0af`, branch `mr-002-knowledge-lifecycle`
- **Contract this refines:** [mr-016-design.md](mr-016-design.md). Where the two
  disagree, the design wins on intent and this document wins on the detail the
  design left open — every such gap is recorded below as a decision D-205…D-214.
- **Task:** [mindrail-0.1-task-list.md](mindrail-0.1-task-list.md) MR-016
- **Tier:** 3 (Program). It adds two tools (13 total), binds validation and
  completion services with a full five-family composition, and proves the
  stdio wire end to end. No database schema, no wire codes.

This file is frozen **before** implementation so that an audit has something
to grade against that the implementation did not shape. The milestone runs
**one task at a time** as MR-004 through MR-015 did: each task in §4 is
implemented, gated by an independent Reader/Breaker pair and recorded before
the next is begun.

---

## 0. Baseline evidence

Captured on the tree this work starts from, so a red suite later is unambiguous.

| Measurement | Value | Command |
|---|---|---|
| Commit | `c60d0af` | `git rev-parse --short HEAD` |
| Go toolchain | `go1.27.0 linux/amd64` | `go version` |
| Packages | 34 | `go list ./... \| wc -l` |
| Top-level test functions | 1229 | `go test -list '.*' ./... \| grep -c '^Test'` |
| `make verify` | green at MR-015 close | `make verify` |
| `make tidy-check` | green at MR-015 close | `make tidy-check` |
| Migrations | `000001_initial.sql` … `000008_evidence.sql`; runtime `schema_version` reports 8 | `ls migrations/*.sql` |
| Registered `app.Code` values | 46 | `grep -c 'Code = "' internal/app/code.go` |
| MCP tools | 11 | ListTools |

---

## 1. The scenario, in one paragraph

An agent runs `validate` naming the project's `test` profile and gets
evidence rows bound to the current snapshot; then it calls `complete` and
the server composes everything itself — attribution findings, evidence
freshness over stored rows, invariant bindings with severities from the
loaded knowledge, ambiguities, and guard findings over the changed test
files' git deltas — and answers ALLOW, or DENY with the same codes the
gate unit tests pin. A budget class or an approval request refuses as a
structured version error. Over stdio, a thirteenth tool appears beside
the twelve, and every one answers its smoke call.

---

## 2. Decisions

MR-015 ended at D-204. These continue the sequence.

### D-205 — validate runs named config profiles, nothing else

`validate{profile}` looks the profile up in the loaded project config;
unknown names refuse before anything spawns. Ad-hoc argv never exists —
the profile whitelist (spec §49) is the whole surface.

### D-206 — complete composes all five families server-side

Attribution via `EvaluateTask`, evidence via stored rows + `Check`,
bindings/ambiguities via index reads, guard via git deltas of changed
test files with reference-derived mapping. No family arrives as caller
input (except required profiles): the tool discovers, the gate judges.

### D-207 — test files and mapping are narrow and documented

Test file = under `test/` or `tests/`, or `test_*.py` / `*_test.py` /
`*.test.{ts,js}`. Mapping = referrers (by key→name resolution) of bound
production uids whose file is a test file. Before bytes = `git show
HEAD:path`, after = disk; untracked files read as added (quiet). Anything
else is not a verification test in 0.1, recorded not guessed.

### D-208 — budget/escalation/approval refuse as version errors

`budget`, `escalation`, `approval` optionals on validate/complete: any
present value returns `NOT_IMPLEMENTED_IN_THIS_VERSION` with next action.
Absence means default flow, never refusal.

### D-209 — stdio proof is in-process pipes, not a subprocess

The AC-4 test connects the SDK server and client over stdio transports in
one process: same wire bytes as production serving, none of the process
flakiness. A `mindrail mcp` command still does not exist (D-191 stands).

### D-210 — evidence reads gain one neutral method

`validation.Store.EvidenceForProfile` lists rows per profile (newest
first). Read-only shape like every other store read; MR-010's
append-only is untouched.

### D-211 — bindings read gains one neutral method

`index.Store` gains status-inclusive bindings per uid (bound rows already
readable; ambiguous/orphaned ride the same query). No traversal or
severity logic leaks into index — severity/active resolve from knowledge
bodies in the mcp layer.

### D-212 — serial tasks, Reader/Breaker gates, findings rows

The MR-004…MR-015 process repeats: one task at a time, each gated before
the next, each recorded in `mr-016-findings.md` (new file — MR-015's record
stays closed). Post-gate hygiene (`git status`/`git diff` on tracked
paths) applies to every subagent.

### D-213 — required profiles are caller input, possibly empty

`complete` takes `required: string[]`; empty means no evidence requirement
(MR-011 semantics preserved: coverage answers what it is asked).

### D-214 — contract is structs + behaviors, not prose

Typed In/Out plus the stdio discovery test ARE the contract. Prose
describes; structs decide.

---

## 3. Requirements and acceptance criteria

REQ-01…REQ-07. Each task's criteria are dotted under it, in the form the
gates grade. A criterion not met at the end of its task is recorded as not
met, with the reason, in `mr-016-findings.md`.

**REQ-01 — validate tool** (named profile → evidence).
**REQ-02 — complete tool** (five-family composition → ALLOW/DENY).
**REQ-03 — version-error params + stdio proof** (budget/escalation/approval, 13-tool wire).
**REQ-04 — non-goals, guarded**.
**REQ-05 — evidence discipline** (every new guard mutation-red).
**REQ-06 — surface discipline** (no codes/tables/CLI/timings; 13 tools).

### TASK-01 — validate tool + store/index reads

Owns REQ-01.

- **AC-01.1** named profile runs to evidence rows bound to the current
  snapshot (profile, statuses, snapshot hash in the answer).
- **AC-01.2** unknown profile names refuse before spawn; malformed inputs
  refuse value-free.
- **AC-01.3** `EvidenceForProfile` lists newest-first; bindings read is
  status-inclusive (both methods pinned, MR-010/MR-006 behavior
  unchanged).

### TASK-02 — complete tool + version errors + stdio

Owns REQ-02, REQ-03.

- **AC-02.1** complete composes attribution + evidence + bindings +
  ambiguities + guard and answers ALLOW on clean / DENY with gate codes
  on dirty (each family reachable in tests).
- **AC-02.2** stale evidence denies through the tool exactly as the gate
  unit pins (same code, same profile remedy) — the CLI-equivalence
  clause for a CLI path that does not exist yet.
- **AC-02.3** budget/escalation/approval present refuse as structured
  version errors with next_action; absent means default flow.
- **AC-02.4** stdio E2E discovers 13 tools and smoke-calls every one
  (readiness + shapes, not deep behavior — depth belongs to family
  tests).

### TASK-03 — proof, non-goals and the record

Owns REQ-04, REQ-05, REQ-06.

- **AC-03.1** full-suite green evidence: `make verify`, `make tidy-check`,
  test count by the single command.
- **AC-03.2** the non-goals hold under grep: no new codes (registry 46),
  thirteen tools exactly, no tables, no CLI command, no value echo.
- **AC-03.3** every new guard added by TASK-01…02 has its mutation run and
  recorded red (REQ-05), and the task list's MR-016 entry carries a
  `#### Durum` block written the way MR-015's is.

---

## 4. Work breakdown and dependency order

Serialised: each task is gated before the next is written.

| Task | Owns | REQ | Depends on |
|---|---|---|---|
| TASK-01 | validate tool, EvidenceForProfile, status bindings read | REQ-01 | — |
| TASK-02 | complete composition, version errors, stdio proof | REQ-02, REQ-03 | TASK-01 |
| TASK-03 | proof, non-goals, record, Durum block | REQ-04, REQ-05, REQ-06 | TASK-02 |

---

## 5. Definition of done

1. Every acceptance criterion above is met, or is recorded as not met with
   the reason in `mr-016-findings.md`.
2. `make verify` green, `make tidy-check` green, and the test count reported
   with `go test -list '.*' ./... | grep -c '^Test'` — that command and no
   other.
3. Every new guard has a recorded mutation that turns it red, recorded after
   it was run.
4. Every task in §4 has passed its Reader/Breaker gate before the next was
   begun, and the gate's findings — confirmed, refuted, unconfirmed — are in
   `mr-016-findings.md` per task, with what was done about each.
5. The task list's MR-016 entry carries a `#### Durum` block written the way
   MR-015's is.
6. The knowledge graph is refreshed in its own commit at the end.

---

## 6. Traceability to the task list's four acceptance criteria

| Task list AC | Where it is owned |
|---|---|
| Named profile sonucu mevcut snapshot'a bağlı Evidence üretir | AC-01.1 (REQ-01) |
| Missing/stale evidence MCP completion'da CLI ile aynı DENY | AC-02.2 (REQ-02; gate-unit parity) |
| Budget/escalation/approval structured version error | AC-02.3 (REQ-03) |
| Stdio E2E 13 tool discover + contract | AC-02.4 (REQ-03) |
