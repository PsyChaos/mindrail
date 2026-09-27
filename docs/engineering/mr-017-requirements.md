# MR-017 — Frozen requirements and acceptance criteria

- **Frozen at:** commit `2339a1b`, branch `mr-002-knowledge-lifecycle`
- **Contract this refines:** [mr-017-design.md](mr-017-design.md). Where the two
  disagree, the design wins on intent and this document wins on the detail the
  design left open — every such gap is recorded below as a decision D-215…D-220.
- **Task:** [mindrail-0.1-task-list.md](mindrail-0.1-task-list.md) MR-017
- **Tier:** 3 (Program). It adds the first new CLI commands (`verify`,
  `knowledge`, `hook`), staged-diff evaluation through shared services, and
  the pre-commit installer later milestones and CI rely on. No database
  schema, no wire codes.

This file is frozen **before** implementation so that an audit has something
to grade against that the implementation did not shape. The milestone runs
**one task at a time** as MR-004 through MR-016 did: each task in §4 is
implemented, gated by an independent Reader/Breaker pair and recorded before
the next is begun.

---

## 0. Baseline evidence

Captured on the tree this work starts from, so a red suite later is unambiguous.

| Measurement | Value | Command |
|---|---|---|
| Commit | `2339a1b` | `git rev-parse --short HEAD` |
| Go toolchain | `go1.27.0 linux/amd64` | `go version` |
| Packages | 34 | `go list ./... \| wc -l` |
| Top-level test functions | 1241 | `go test -list '.*' ./... \| grep -c '^Test'` |
| `make verify` | green at MR-016 close | `make verify` |
| `make tidy-check` | green at MR-016 close | `make tidy-check` |
| Migrations | `000001_initial.sql` … `000008_evidence.sql`; runtime `schema_version` reports 8 | `ls migrations/*.sql` |
| Registered `app.Code` values | 46 | `grep -c 'Code = "' internal/app/code.go` |
| Verify/knowledge/hook commands | do not exist | CLI surface |

---

## 1. The scenario, in one paragraph

A developer stages a commit that deletes asserts from a guarded test and
ships a malformed knowledge record. `verify --staged` reconciles the index
into change rows, the guard fires blocking on the staged test file, the
knowledge check fails closed on the corrupt record, and the command exits
non-zero with every reason named. The pre-commit installer appends its
guarded block to an existing hook without touching a line of it — running
it twice changes nothing. A clean tree verifies green.

---

## 2. Decisions

MR-016 ended at D-214. These continue the sequence.

### D-215 — staged evaluation writes change rows like reconcile

Evaluation needs stored rows; `verify --staged` reconciles the index
into them exactly as worktree reconcile does (same service family, same
semantics). A check that cannot see is worse than a check that records.

### D-216 — staged means index, never worktree

Staged bytes come from `git show :path`, before bytes from HEAD. Unstaged
worktree edits are invisible to this gate by construction — the commit is
judged, not the desk.

### D-217 — knowledge validate fails closed before source checks

Corrupt records and newer-than-binary schemas refuse the whole validation
with the loader's codes before any source evaluation runs. A gate that
judges code against unreadable knowledge lies about both.

### D-218 — hook install appends a guarded block, idempotent

`.git/hooks/pre-commit` gains a marker-delimited block calling
`mindrail verify --staged`; existing content is never rewritten, a second
install changes nothing, and a missing hook file is created with a
shebang. Markers are exact-match; foreign edits between them are left
alone (owned lines only).

### D-219 — serial tasks, Reader/Breaker gates, findings rows

The MR-004…MR-016 process repeats: one task at a time, each gated before
the next, each recorded in `mr-017-findings.md` (new file — MR-016's record
stays closed). Post-gate hygiene (`git status`/`git diff` on tracked
paths) applies to every subagent.

### D-220 — CLI commands are in scope here, and only these

`verify --staged`, `knowledge validate`, `hook install`. No other
commands, flags or surfaces; `--ci` belongs to MR-018 and is not stubbed.

---

## 3. Requirements and acceptance criteria

REQ-01…REQ-07. Each task's criteria are dotted under it, in the form the
gates grade. A criterion not met at the end of its task is recorded as not
met, with the reason, in `mr-017-findings.md`.

**REQ-01 — verify --staged** (staged eval, unreconciled/weakening denials).
**REQ-02 — knowledge validate** (fail-closed corruption/newer-schema).
**REQ-03 — hook install** (append-only, idempotent).
**REQ-04 — git fixtures** (clean/allow/denial scenarios).
**REQ-05 — non-goals, guarded**.
**REQ-06 — evidence discipline** (every new guard mutation-red).
**REQ-07 — surface discipline** (3 commands; no codes/tables/timings beyond).

### TASK-01 — verify --staged + knowledge validate

Owns REQ-01, REQ-02.

- **AC-01.1** staged unreconciled changes (ambiguous/unregistered/drift)
  deny with the attribution codes through the shared evaluation.
- **AC-01.2** staged test weakening denies blocking through the shared
  guard (before = HEAD bytes, after = index bytes).
- **AC-01.3** corrupt knowledge and newer-than-binary schema fail closed
  with loader codes before any source check runs.
- **AC-01.4** clean staged tree verifies green (exit 0, empty denials);
  every denial carries code + remedy; exit codes follow the envelope
  table (new codes: none).

### TASK-02 — hook install + git fixtures

Owns REQ-03, REQ-04.

- **AC-02.1** install creates a missing hook with shebang + block;
  appends the block to foreign content without rewriting a line of it.
- **AC-02.2** second install is a no-op (byte-identical file); foreign
  edits between markers are preserved; markers are exact-match.
- **AC-02.3** git fixtures cover clean (green), allow (warnings only)
  and denial (each family at least once) through the real commands.
- **AC-02.4** hook content executes `verify --staged` (block calls the
  command by name; execution pinned by running the installed block).

### TASK-03 — proof, non-goals and the record

Owns REQ-05, REQ-06, REQ-07.

- **AC-03.1** full-suite green evidence: `make verify`, `make tidy-check`,
  test count by the single command.
- **AC-03.2** the non-goals hold under grep: no new codes (registry 46),
  three commands exactly, no tables, no value echo.
- **AC-03.3** every new guard added by TASK-01…02 has its mutation run and
  recorded red (REQ-06), and the task list's MR-017 entry carries a
  `#### Durum` block written the way MR-016's is.

---

## 4. Work breakdown and dependency order

Serialised: each task is gated before the next is written.

| Task | Owns | REQ | Depends on |
|---|---|---|---|
| TASK-01 | verify --staged, knowledge validate | REQ-01, REQ-02 | — |
| TASK-02 | hook install, git fixtures (clean/allow/denial) | REQ-03, REQ-04 | TASK-01 |
| TASK-03 | proof, non-goals, record, Durum block | REQ-05, REQ-06, REQ-07 | TASK-02 |

---

## 5. Definition of done

1. Every acceptance criterion above is met, or is recorded as not met with
   the reason in `mr-017-findings.md`.
2. `make verify` green, `make tidy-check` green, and the test count reported
   with `go test -list '.*' ./... | grep -c '^Test'` — that command and no
   other.
3. Every new guard has a recorded mutation that turns it red, recorded after
   it was run.
4. Every task in §4 has passed its Reader/Breaker gate before the next was
   begun, and the gate's findings — confirmed, refuted, unconfirmed — are in
   `mr-017-findings.md` per task, with what was done about each.
5. The task list's MR-017 entry carries a `#### Durum` block written the way
   MR-016's is.
6. The knowledge graph is refreshed in its own commit at the end.

---

## 6. Traceability to the task list's five acceptance criteria

| Task list AC | Where it is owned |
|---|---|
| `verify --staged` unreconciled staged değişikliği yakalar | AC-01.1 (REQ-01) |
| Knowledge corruption/ileri şema fail-closed | AC-01.3 (REQ-02) |
| Test weakening staged yolda blocking bulgu üretir | AC-01.2 (REQ-01) |
| Kurulum hook içeriğini overwrite etmez | AC-02.1, AC-02.2 (REQ-03) |
| Git fixture clean/allow/denial | AC-02.3, AC-02.4 (REQ-04) |
