# MR-007 — Frozen requirements and acceptance criteria

- **Frozen at:** commit `8a75079`, branch `mr-002-knowledge-lifecycle`
- **Contract this refines:** [mr-007-design.md](mr-007-design.md). Where the two
  disagree, the design wins on intent and this document wins on the detail the
  design left open — every such gap is recorded below as a decision D-114…D-129.
- **Task:** [mindrail-0.1-task-list.md](mindrail-0.1-task-list.md) MR-007
- **Tier:** 3 (Program). It adds a runtime migration (five tables), a new
  domain package with a Git read path, the first production driver of the
  index/identity machinery (reconcile indexes), and the operation-log half of
  AC-15's Change promise. The escalation rule fires on "public API contract"
  and "database schema" regardless of line count.

This file is frozen **before** implementation so that an audit has something
to grade against that the implementation did not shape. The milestone runs
**one task at a time** as MR-004 through MR-006 did: each task in §4 is
implemented, gated by an independent Reader/Breaker pair and recorded before
the next is begun.

---

## 0. Baseline evidence

Captured on the tree this work starts from, so a red suite later is unambiguous.

| Measurement | Value | Command |
|---|---|---|
| Commit | `8a75079` | `git rev-parse --short HEAD` |
| Go toolchain | `go1.27.0 linux/amd64` | `go version` |
| Packages | 28 | `go list ./... \| wc -l` |
| Top-level test functions | 1074 | `go test -list '.*' ./... \| grep -c '^Test'` |
| `make verify` | green — exit 0 across check, race and smoke | `make verify` |
| `make tidy-check` | green, exit 0 | `make tidy-check` |
| Migrations | `000001_initial.sql` … `000005_symbol_identity.sql`; runtime `schema_version` reports 5 | `ls migrations/*.sql` |
| Registered `app.Code` values | 40 | `grep -c 'Code = "' internal/app/code.go` |
| `internal/changes` | does not exist | `ls internal/` |
| Change records | do not exist (no table, no type) | schema + coordination model |

---

## 1. The scenario, in one paragraph

An agent edits code without calling `before_change`. Later something asks
what actually changed. Mindrail does not shrug: reconcile reads the worktree
and staged Git diff, re-extracts the changed files, compares fingerprints
against stored facts, follows `symbol_uid`s, drives the index update with
identity migration, and links the resulting file/symbol set to the task's
Change — the same Change set an agent that *did* call `before_change` would
have produced through the baseline path. Files changed outside any baseline
are reported as divergence, not absorbed. Work with no task becomes a
retroactive Change instead of vanishing.

---

## 2. Decisions

MR-006 ended at D-113. These continue the sequence.

### D-114 — the domain lives in `internal/changes/`, schema version 6

`internal/changes` owns the Change Store (tables), the baseline store and
the discovery Service. `migrations/000006_changes.sql` creates `changes`,
`change_files`, `change_symbols`, `change_baselines` and `change_operations`;
`changes.TableSchemaVersion = 6` gates readers the MR-003 way. The service
composes `coordination` (task existence), `git` (diff spelling),
`index`/`scheduler`-style direct `Indexer` calls (symbol facts) and
`symbol` (bindings are read, never written here) — inward dependencies only.

### D-115 — one open change per task, provenance per row

A task holds at most one open Change: `before_change` ensures it,
`after_change` and `reconcile` upsert into it, creating it when absent (the
retroactive path included — with a NULL task when no task is named).
`discovered_via` (`baseline` | `reconcile`) rides each file and symbol row,
not the Change row: mixed flows are the norm (baseline for scope files,
reconcile for the rest), and the container must not pretend otherwise.

### D-116 — one baseline per task, replaced wholesale

`before_change(task, scopePaths)` replaces the task's baseline rows
`(task_id, path, content_hash, captured_at)` with the scope's current
hashes. No multi-baseline stacking in 0.1: the latest call is the truth
about what the agent declared, and anything outside it is divergence by
definition (D-122).

### D-117 — Git discovery is one porcelain call with explicit exclusions

Changed files come from `git status --porcelain=v1 -z
--untracked-files=normal -- <root>`: one call covers unstaged, staged and
untracked. Kind mapping: `M→modified`, `A→added`, `D→deleted`, `R→renamed`
(with old path), `C→added` (copied content is new content),
`T→modified`, `U→modified` (conflict is still a change),
`?→added` (untracked work is work). Excluded: `.git/` and `.mindrail/`
subtrees (runtime and knowledge machinery, not source) and non-regular
files, which are still recorded as file changes with an empty hash and no
symbols — discovery stays complete where parsing cannot follow. Rename
similarity from status applies to staged entries only (a Git limitation,
not a Mindrail choice); unstaged moves surface as delete+add pairs.

### D-118 — reconcile indexes; the 0.1 driver lands here

Discovering a changed file re-extracts it, diffs fingerprints against stored
facts, then drives `Indexer.IndexFile` (with the project id and
Git-corroborated rename hints where available) so facts, uids and migrations
move in the same flow (spec §65 step 3). This is the first production use of
the index/identity machinery — D-91's unwired scheduler stays future work:
reconcile drives changed files only, never the cold remainder.

### D-119 — symbol change vocabulary

Per symbol: presence `added | removed`, plus independent `body_changed`,
`signature_changed` and `structure_changed` flags (each hash compared on its
own; a signature edit flips signature and structure per the fingerprint
contract). Current hashes are stored; before-values are not (reconstructible
via Git, out of 0.1). Renames surface as removed-old plus added-new bound by
one uid through migration.

### D-120 — file change vocabulary

Per file: `added | modified | deleted | renamed` with `old_path` for
renames, and the current content SHA-256 (empty for deleted and non-regular
files). Untracked files are `added`.

### D-121 — Change creation honors operation_id (AC-15's Change half)

Creation accepts an operation id with coordination's grammar
(`coordination.ValidOperationID`) and replay discipline: same id plus same
request hash answers from the recorded result without new rows; same id plus
a different hash is `OPERATION_ID_CONFLICT` (existing code, reused). The log
lives in `change_operations`; the natural-key upserts underneath make
same-content redelivery converge even without an id.

### D-122 — divergence is reconcile-minus-baseline, reported not absorbed

Baseline divergence = Git-discovered changed files outside the task's
baseline scope, listed with paths and kinds. An empty list means none. The
list shape is MR-008's input; this milestone only produces it, visibly and
with the reason attached to each entry.

### D-123 — attribution is exact task match in 0.1

Deltas link to the named task's open Change, or to a NULL-task retroactive
Change when no task is named. No fuzzy ownership, no cross-task guessing —
MR-008 owns ambiguity.

### D-124 — no new app.Codes in MR-007

Divergence and unregistered entries ride result structs with string reasons.
Enforcement codes arrive with MR-008/MR-013/MR-017; inventing them here
would strand vocabulary no gate reads yet.

### D-125 — serial tasks, Reader/Breaker gates, findings rows

The MR-004…MR-006 process repeats: one task at a time, each gated before
the next, each recorded in `mr-007-findings.md` (new file — MR-006's record
stays closed).

### D-126 — lease acquisition is not in MR-007

The spec lists lease acquisition among `before_change` benefits, but no
MR-007 acceptance criterion requires it and leases stay MR-004's domain.
Noted for MR-015's tool wiring; not built here.

### D-127 — no CLI/MCP surface in MR-007

Service only, driven by tests now and by milestone drivers later (the D-91
pattern). Status, doctor and goldens are untouched.

### D-128 — deleted paths mark stored symbols removed

A deleted or emptied path contributes no fresh extraction, but its stored
symbols do not silently persist as current: they are recorded `removed`
against the Change. Stale-row pruning itself stays MR-007-reconcile's
documented future (D-113 carries forward).

### D-129 — symbol_uids are followed, never re-derived, in deltas

A symbol delta names the uid its rows carry (post-indexing), so renames
arrive already migrated. Deltas never mint: minting happens only inside the
indexing both paths drive.

---

## 3. Requirements and acceptance criteria

REQ-01…REQ-11. Each task's criteria are dotted under it, in the form the
gates grade. A criterion not met at the end of its task is recorded as not
met, with the reason, in `mr-007-findings.md`.

**REQ-01 — schema** (migration 000006, version gate, ledger pins).
**REQ-02 — baselines and open changes** (capture, replace, ensure).
**REQ-03 — operation idempotency** (log, replay, conflict).
**REQ-04 — Git discovery** (porcelain parse, kinds, exclusions).
**REQ-05 — file delta** (baseline-vs-actual per file).
**REQ-06 — symbol delta and index side effect** (extract-vs-stored, uid following).
**REQ-07 — after_change composition** (baseline path end to end).
**REQ-08 — reconcile composition** (Git path, convergence, divergence, retroactive).
**REQ-09 — non-goals, guarded**.
**REQ-10 — evidence discipline** (every new guard mutation-red).
**REQ-11 — surface discipline** (no new codes, commands, keys, timings).

### TASK-01 — migration 000006 and the changes store skeleton

Owns REQ-01.

- **AC-01.1** `migrations/000006_changes.sql` creates `changes` (uid PK,
  nullable task FK, operation id, discovered summary, timestamps),
  `change_files` (natural PK `(change_id, path)`, kind CHECK, old path,
  hash, per-row `discovered_via` CHECK), `change_symbols` (natural PK
  `(change_id, logical_key)`, uid nullable, kind CHECK, three change flags,
  current hashes), `change_baselines` (PK `(task_id, path)`, hash,
  captured_at) and `change_operations` (operation id PK, task, request hash,
  result JSON, recorded_at) — and nothing else.
- **AC-01.2** `changes.TableSchemaVersion = 6` gates the store; the
  migration applies cleanly to a schema-5 database carrying MR-006 rows,
  and `init` reports `schema_version` 6.
- **AC-01.3** `migrations/shipped_test.go` pins the new file's bytes; the
  ledger's expected-columns list covers the five tables (D-73 forward).
- **AC-01.4** no knowledge schema changes, no new codes, no CLI surface;
  only the `schema_version` 5→6 number rides the goldens.

### TASK-02 — baselines, open changes, operation replay

Owns REQ-02 and REQ-03.

- **AC-02.1** `CaptureBaseline(task, scopePaths)` hashes scope files and
  replaces the task's baseline rows; a second capture replaces again
  (no stacking, D-116); non-regular files record empty hashes without
  failing.
- **AC-02.2** `EnsureOpenChange(task?, operationID?)` returns the same open
  Change id for the same task across calls (no duplicates). A NULL task with
  an operation id replays its row; without an id each call creates a fresh
  retroactive row (callers that want convergence pass the id; AC-15's
  letter).
- **AC-02.3** same operation id plus same request content replays the
  recorded Change without new rows; same id plus different content is
  `OPERATION_ID_CONFLICT` (D-121).
- **AC-02.4** baseline rows for a task vanish exactly when the task's new
  baseline lands or when explicitly cleared — never as a side effect of
  discovery.

### TASK-03 — Git discovery and file delta

Owns REQ-04 and REQ-05.

- **AC-03.1** porcelain fixtures parse: unstaged `M`, staged `M`, untracked
  `??`, staged rename `R`, deleted `D`, each to the D-117 kind with the
  right paths (rename carries both).
- **AC-03.2** `.git/` and `.mindrail/` entries never become Change rows;
  non-regular paths become file rows with empty hashes and no symbols.
- **AC-03.3** file delta against a baseline: changed hashes (or missing
  baseline entries for scope files) become file rows; unchanged scope
  files do not.
- **AC-03.4** file delta against Git (no baseline): every discovered entry
  becomes a file row with its kind and current hash.
- **AC-03.5** the same on-disk edit yields the same file rows through both
  paths (shape convergence at file level, precursor to AC-05.1).

### TASK-04 — symbol delta, index side effect, after_change

Owns REQ-06 and REQ-07.

- **AC-04.1** per language, body-only edits flag `body_changed`,
  signature edits flag `signature_changed` (+`structure_changed`), added
  rows flag `added`, vanished rows flag `removed` — asserted on stored
  `change_symbols` rows with current hashes.
- **AC-04.2** discovery indexes what it reads: after the run, stored facts
  equal a fresh parse and uids follow renames (migration runs inside).
- **AC-04.3** `AfterChange(task)` composes baseline delta with symbol
  delta into the task's open Change end to end; calling it twice converges
  (no duplicate rows).
- **AC-04.4** rename hints from staged Git renames reach the migration
  (a staged move migrates where an identical unstaged move mints — pinned
  difference proving the hint path is live).

### TASK-05 — reconcile, convergence, divergence, proof

Owns REQ-08, REQ-09, REQ-10, REQ-11.

- **AC-05.1** the same on-disk edit yields the same file+symbol Change rows
  with and without `before_change` (baseline path vs reconcile-only path;
  per-row `discovered_via` may differ, content must not).
- **AC-05.2** baseline divergence is explicit: an edit outside the baseline
  scope appears in the divergence list with path, kind and reason — never
  silently absorbed, never blocking this milestone.
- **AC-05.3** retroactive: reconcile without a task creates a NULL-task
  Change carrying the discovered rows.
- **AC-05.4** the non-goals hold under grep: no semantic/impact/evidence
  machinery, no new codes/commands/keys, knowledge schema still v1.
- **AC-05.5** every new guard added by TASK-01…04 has its mutation run and
  recorded red (REQ-10), and the task list's MR-007 entry carries a
  `#### Durum` block written the way MR-006's is.

---

## 4. Work breakdown and dependency order

Serialised: each task is gated before the next is written.

| Task | Owns | REQ | Depends on |
|---|---|---|---|
| TASK-01 | migration 000006, store skeleton, ledger pins | REQ-01 | — |
| TASK-02 | baselines, open changes, operation replay | REQ-02, REQ-03 | TASK-01 |
| TASK-03 | porcelain parse, exclusions, file delta both paths | REQ-04, REQ-05 | TASK-02 |
| TASK-04 | symbol delta, index side effect, after_change, hint wiring | REQ-06, REQ-07 | TASK-02, TASK-03 |
| TASK-05 | reconcile, convergence, divergence, retroactive, proof/record | REQ-08, REQ-09, REQ-10, REQ-11 | TASK-04 |

---

## 5. Definition of done

1. Every acceptance criterion above is met, or is recorded as not met with
   the reason in `mr-007-findings.md`.
2. `make verify` green, `make tidy-check` green, and the test count reported
   with `go test -list '.*' ./... | grep -c '^Test'` — that command and no
   other.
3. Every new guard has a recorded mutation that turns it red, recorded after
   it was run.
4. Every task in §4 has passed its Reader/Breaker gate before the next was
   begun, and the gate's findings — confirmed, refuted, unconfirmed — are in
   `mr-007-findings.md` per task, with what was done about each.
5. The task list's MR-007 entry carries a `#### Durum` block written the way
   MR-006's is.
6. The knowledge graph is refreshed in its own commit at the end.

---

## 6. Traceability to the task list's four acceptance criteria

| Task list AC | Where it is owned |
|---|---|
| `before_change` called/un-called converge to the same real diff | AC-05.1 (REQ-08); file-level precursor AC-03.5 |
| Body/signature/structure changes linked to Change records | AC-04.1 (REQ-06) |
| Baseline divergence visible and explainable | AC-05.2 (REQ-08) |
| Unstaged, staged and separate-worktree Git fixtures | AC-03.1 + AC-05.1/05.3 fixtures (REQ-04, REQ-08) |
