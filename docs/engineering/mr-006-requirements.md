# MR-006 — Frozen requirements and acceptance criteria

- **Frozen at:** commit `b32399b`, branch `mr-002-knowledge-lifecycle`
- **Contract this refines:** [mr-006-design.md](mr-006-design.md). Where the two
  disagree, the design wins on intent and this document wins on the detail the
  design left open — every such gap is recorded below as a decision D-93…D-109.
- **Task:** [mindrail-0.1-task-list.md](mindrail-0.1-task-list.md) MR-006
- **Tier:** 3 (Program). It adds a runtime migration (one `ALTER TABLE`, three
  new tables), a `symbols` column that enters every extraction write path, two
  `app.Code` values that enter the wire vocabulary, a new service package, a
  Git read path, and the first runtime join between repository-owned knowledge
  and structural facts. The escalation rule fires on "public API contract" and
  "database schema" regardless of line count.

This file is frozen **before** implementation so that an audit has something
to grade against that the implementation did not shape. The milestone runs
**one task at a time** as MR-004 and MR-005 did: each task in §4 is
implemented, gated by an independent Reader/Breaker pair and recorded before
the next is begun.

---

## 0. Baseline evidence

Captured on the tree this work starts from, so a red suite later is unambiguous.

| Measurement | Value | Command |
|---|---|---|
| Commit | `b32399b` | `git rev-parse --short HEAD` |
| Go toolchain | `go1.27.0 linux/amd64` | `go version` |
| Packages | 27 | `go list ./... \| wc -l` |
| Top-level test functions | 1028 | `go test -list '.*' ./... \| grep -c '^Test'` |
| `make verify` | green — exit 0 across check, race and smoke | `make verify` |
| `make tidy-check` | green, exit 0 | `make tidy-check` |
| Migrations | `000001_initial.sql` … `000004_index.sql`; runtime `schema_version` reports 4 | `ls migrations/*.sql`; `internal/cli/testdata/status_human.golden` |
| Registered `app.Code` values | 38 | `grep -c 'Code = "' internal/app/code.go` |
| `index.TableSchemaVersion` | 4 | `internal/index/index.go` |
| `parser.ParseSchemaVersion` | 2 | `internal/index/parser/types.go` |
| `internal/index/symbol` | does not exist | `ls internal/index` |
| `symbols.symbol_uid` | does not exist | migration 000004 columns |

---

## 1. The scenario, in one paragraph

An invariant protects `SessionService.calculate_timeout`. An agent renames it
to `compute_timeout`. Mindrail does not create a new symbol and orphan the
invariant: the durable `symbol_uid` migrates to the new `logical_key`, the
`INV-xxx → SYM-yyy` binding never moves, and completion sees the same
protected identity under its new name. When two agents race to index the same
new symbol, they agree on one uid. When a rename could mean two different
targets, Mindrail guesses nothing: it records `SYMBOL_IDENTITY_AMBIGUOUS` and
blocks completion while a HIGH/CRITICAL invariant points at it. When the
protected symbol is gone with no confident heir, the binding goes orphaned
and blocks rather than going silent.

---

## 2. Decisions

MR-005 ended at D-92. These continue the sequence. Each one existed as an
ambiguity two implementers would have resolved differently, or as a place
where the specification leaves the milestone to decide.

### D-93 — the service lives in `internal/index/symbol/`, the SQL in `internal/index`

D-80 deferred `internal/index/symbol` as "durable symbol identity is MR-006's":
this milestone creates it. It owns allocation, migration matching, ambiguity
and binding refresh as a service over `index.Store` methods; the tables stay
in the `internal/index` root package beside the migration that creates them,
as D-80 prescribed for `symbol/` and `graph/` consumers. No
`internal/semantic/` is created: matching is STRUCTURAL (body, kind,
container, path, Git), never semantic.

### D-94 — allocation key and lineage shape

Spec-1.0 §24's first-observation key, in 0.1 terms, is
`(project_id, unit_id, language, logical_key)` with a UNIQUE constraint. The
project id is the workspace project the store already scopes coordination by;
the unit id is 0.1 reality beside the spec (two units share no relpaths by
construction, but the key does not rely on it). Overloads share one
`logical_key` by extractor design, so they share one lineage with no
discriminator column in 0.1. Uids are opaque `SYM-` ids from `identity.NewID`
(spec §24's example format).

### D-95 — facts and identity commit atomically; Git arrives as hints

`EnsureIdentity` runs **inside** `replaceFileFactsTx`, in the same short
transaction that replaces the file's facts: facts and uids can never diverge,
and there is no crash window that leaves uid-less rows behind. Process
execution is forbidden under the write lock (MR-004 discipline), so the Git
rename set is fetched once per `IndexFile` call, outside any transaction, and
travels as `FileFacts.RenameHints` (empty by default — structural-only
matching). No `ReplaceFileFactsCAS` signature changes: the hints ride the
facts struct TASK-05 already passes.

### D-96 — the migration bar, the cascade, and the count rule

A disappeared key (an identity whose `logical_key` has no live symbol rows in
the unit) migrates to an added key (rows without an identity) iff **exactly
one** added key meets the bar; **two or more** meeting it record ambiguity;
**zero** mints a new uid. The 0.1 STRUCTURAL bar:

```text
body_hash equal AND kind equal AND container-local-key equal
AND (same file path OR git-corroborated old-path → new-path)
```

Rename changes the signature bytes, so `structure_hash` is deliberately not
in the bar (it moves on rename by extractor design); body, kind and container
do not. Parents migrate before children within one completion, remapping
`oldContainerKey → newContainerKey` for the children's comparison — otherwise
a renamed class orphans every method inside it. Every migration appends the
abandoned key to the identity's `previous_keys` lineage memory, so a later
refresh can resolve a stale target text to the migrated uid instead of
orphaning it. A migration whose added key is already owned by another uid is
ambiguous, never a steal. The bar, the threshold (one confident candidate)
and the remap are asserted per case by fixture, not by prose.

### D-97 — target resolution walks live rows, stores no names

No human-readable name column is added: a SYMBOL target
`path:Container.name` resolves by walking the file's current symbol rows
segment by segment (each segment must match exactly one row name under the
current container key). Overloads sharing one uid bind; zero or several
distinct uids resolve to orphan-candidate and ambiguous respectively. FILE
targets match by path; MODULE/PACKAGE by prefix/dir; PROJECT needs no target.
Unresolvable forms are never guessed.

### D-98 — bindings are runtime-only; blocking follows severity

The knowledge schema stays v1 (frozen): `invariant_symbol_bindings`
maps `INV-xxx → SYM-yyy` with status `bound | ambiguous | orphaned`. Only
`ambiguous`/`orphaned` bindings over an **active HIGH or CRITICAL**
invariant block (spec §24); anything else is reported, not blocking. MR-013
owns the gate; MR-006 produces the findings with reason codes and remedies.

### D-110 — sticky bound persistence and no-owner orphans

A bound binding persists while its uid has live rows, whatever the target
text now says: the binding names a lineage, not a spelling, so a rename
carried by migration needs no update and a stale target cannot silently
unprotect. Divergence that names several live lineages still blocks —
ambiguity wins over stickiness, because the text may genuinely mean another
lineage now. A path owned by no discovered unit can never produce rows, so
absence is proven by construction and the track is orphan, not deferred.

### D-111 — orphan rows need a dead lineage; findings do not

Bindings join lineages, so a stored `orphaned` row attaches to the dropped
uid of a lineage that died. A target that never bound has no lineage to
name: its orphanhood is finding-only (code + remedy + blocking), with zero
stored rows. Both shapes block iff active HIGH/CRITICAL.

(Added at the TASK-03 gate: D-110/D-111 refined AC-03.2/AC-03.4's mechanism
without changing their verdicts.)

### D-99 — orphan requires evidence of absence, not absence of evidence

A cold file (pending, never indexed) defers its bindings — it is not orphaned.
Orphan needs: file state `indexed`/`failed` with no match, or no state row
and no symbol rows for the path while the unit is READY (cold drained).
Anything less is "not yet known", never a block.

### D-100 — backfill mints, never migrates

All pre-MR-006 rows are uid-less and all their keys are present, so nothing
disappeared: `BackfillUnit` looks up or mints per distinct key and stamps the
rows. No reparse (fingerprints and keys are stored), no `ParseSchemaVersion`
bump. Migration matching runs only on the live completion path.

### D-101 — Git signal is best-effort corroboration

The rename set comes from
`git diff --no-color --name-status -z -M HEAD -- <paths>`, parsed for `R`
entries. No `-c` config flag rides along: `-z` already disables pathname
munging, and the literal would trip the no-shell tripwire (tech-stack §35).
(Amended at TASK-04 implementation: the frozen text carried `-c
core.quotepath=off`.) Any failure (no HEAD, not a repo, no git) yields an
empty set, never an error: matching proceeds structurally. Tests inject via
the `git.CommandRunner` interface (fake) plus one real-repository test.

### D-102 — two codes with distinct remedies

`SYMBOL_IDENTITY_AMBIGUOUS` and `ORPHANED_PROTECTED_SYMBOL` enter `allCodes`
with distinct remedies (the TASK-01 AC-01.3 pattern). No other vocabulary is
added.

### D-103 — schema version 5 and the nullable uid column

`index.TableSchemaVersion = 5`. `migrations/000005_symbol_identity.sql`
creates `symbol_identities`, `invariant_symbol_bindings`,
`symbol_identity_ambiguities`, and `ALTER TABLE symbols ADD COLUMN
symbol_uid` (nullable: pre-existing rows predate it). The ledger's
expected-columns list is amended for every touched table (the D-73 lesson,
applied forward again).

### D-104 — serial tasks,Reader/Breaker gates, findings rows

The MR-004/MR-005 process repeats: one task at a time, each gated before the
next, each recorded in `mr-006-findings.md` (new file — MR-005's record stays
closed).

### D-105 — unit id beside project id in the key

Spec §24 names `project_id`; 0.1 adds `unit_id` to the UNIQUE key. Units own
disjoint relpaths by inventory construction, so the unit half never fires in
practice — it is a bulkhead, not a behavior.

### D-106 — semantic signals deferred

`semantic_signature_hash`, resolver declaration/reference identity and
decorator-aware re-evaluation (§24 signals, §25 extra) need FULL resolvers:
deferred with the resolvers (kernel-scope §4). The 0.1 bar is body, kind,
container, path and Git — and says so on every finding it produces
(`STRUCTURAL_*` provenance, confidence ≤ 0.5 discipline carried forward).

### D-107 — failed parses participate

Partial facts from failed parses carry uids through the same path: a broken
file's intact declarations keep their identity instead of losing it until
someone fixes the syntax.

### D-108 — references keep row-ID pointers

`resolved_symbol_id` stays a row-ID pointer; the uid is joined, never
duplicated (normalization — MR-009 traverses rows and reads uids off them).

### D-109 — agreement is proven across processes, not just threads

AC-18 says processes. Threads racing over two `*sql.DB` handles prove the
SQL mechanism; one re-exec subprocess test (worker env flag) proves it across
an OS boundary, where no Go memory is shared and only SQLite serializes.
Both are required; the UNIQUE constraint plus insert-then-read-back is what
makes both pass.

---

## 3. Requirements and acceptance criteria

REQ-01…REQ-10. Each task's criteria are dotted under it, in the form the
gates grade. A criterion not met at the end of its task is recorded as not
met, with the reason, in `mr-006-findings.md`.

**REQ-01 — schema and codes** (migration 000005, `TableSchemaVersion`, two codes).
**REQ-02 — allocation and agreement** (`symbol.Service.EnsureIdentity`, backfill).
**REQ-03 — target resolution and bindings** (resolve, refresh, orphan/deferred rules).
**REQ-04 — rename/move migration** (bar, cascade, Git hints, ambiguity records).
**REQ-05 — no new CLI surface** (status/doctor/goldens untouched).
**REQ-06 — non-goals, guarded**.
**REQ-07 — evidence discipline** (every new guard mutation-red).
**REQ-08 — concurrency proof** (threads + subprocess).
**REQ-09 — knowledge schema frozen** (no v1 change; bindings runtime-only).
**REQ-10 — timing off the wire** (no new timings; none reach JSON).

### TASK-01 — migration 000005, store skeleton, codes

Owns REQ-01 and the schema half of REQ-09.

- **AC-01.1** `migrations/000005_symbol_identity.sql` creates
  `symbol_identities` (uid PK, project/unit/language/key columns, UNIQUE over
  the D-94 key, `previous_keys` lineage memory), `invariant_symbol_bindings`
  (pair-grain UNIQUE index, status CHECK), `symbol_identity_ambiguities`, and
  nothing else; plus `ALTER TABLE symbols ADD COLUMN symbol_uid` (nullable).
  (Amended at the TASK-01 gate: "composite PK" said more than the schema
  does — the design §5 UNIQUE index is authoritative and functionally
  equivalent for the pair grain.)
- **AC-01.2** `index.TableSchemaVersion = 5` gates the store; the migration
  applies cleanly to a schema-4 database carrying MR-005 rows, and `init`
  reports `schema_version` 5.
- **AC-01.3** the two codes of D-102 are registered with distinct remedies
  and enter `allCodes` (the registry test enforces it).
- **AC-01.4** `migrations/shipped_test.go` pins the new file's bytes; the
  ledger's expected-columns list covers the new tables and the new column
  (D-73 forward).
- **AC-01.5** no knowledge schema file changes (v1 frozen); no new commands
  and no new JSON keys — the `schema_version` 4→5 number rides the existing
  goldens (established precedent).

### TASK-02 — allocation, agreement, backfill

Owns REQ-02 and REQ-08 (threads half).

- **AC-02.1** `EnsureIdentity(projectID, unit, key, fingerprints)` returns the
  same uid for the same key across calls (lookup hit, no duplicate rows).
- **AC-02.2** two threads racing first-sight allocation over separate DB
  handles agree on one uid; the identities table holds one row for the key.
- **AC-02.3** `BackfillUnit` stamps every uid-less row of a unit from
  lookup-or-mint without reparse; a second run is a no-op.
- **AC-02.4** completion-path stamping: `ReplaceFileFacts(CAS)` assigns uids
  to the file's new rows in the same transaction (D-95); re-completing
  identical facts keeps the uids (idempotent).
- **AC-02.5** overloads sharing one `logical_key` share one uid (no
  discriminator in 0.1).

### TASK-03 — target resolution and binding refresh

Owns REQ-03 and REQ-09.

- **AC-03.1** per scope form, a fixture resolves: FILE path, SYMBOL
  `path:name`, SYMBOL `path:Container.name`, MODULE prefix, PROJECT
  targetless — each to the expected uid or to the expected non-match.
- **AC-03.2** `RefreshBindings` writes `bound` rows for resolvable active
  invariants and re-resolves after a rename (binding follows the uid, target
  text untouched).
- **AC-03.3** cold (pending) files defer: no binding row, no finding, no
  block.
- **AC-03.4** genuinely unresolvable targets over indexed/failed files, or
  vanished paths in READY units, record `orphaned` bindings with the
  D-102 orphan code, remedy and blocking behavior per D-98 (blocking iff
  active HIGH/CRITICAL).
- **AC-03.5** knowledge records are never written by refresh (v1 frozen —
  assert no `.mindrail/knowledge` mtime/content change in the test).

### TASK-04 — rename/move migration and ambiguity

Owns REQ-04 and REQ-08 (remainder).

- **AC-04.1** same-file function rename migrates the uid (body/kind/
  container/path bar); the binding still points at the uid; exactly one
  `SYMBOL_IDENTITY_MIGRATED` provenance row.
- **AC-04.2** cross-file move with a Git R-entry migrates (structure + Git
  corroboration); without Git signal and with a changed path it mints anew
  (documented limitation, test-pinned).
- **AC-04.3** class rename cascades: parent migrates first, children follow
  through the container remap with no orphaned method bindings.
- **AC-04.4** twin candidates (one removed, two added meeting the bar)
  record ambiguity, mint nothing for the removed key, and block iff the
  binding is active HIGH/CRITICAL (D-98); the ambiguity names all
  candidates. Migrating onto a key already owned by another uid is likewise
  ambiguous — identities are never stolen.
- **AC-04.5** minting happens only after migration was attempted and failed
  (spec §24 rule): a rename with zero bar-meeting candidates mints exactly
  one new uid and records no migration.
- **AC-04.6** Git command failure yields an empty hint set and structural
  matching proceeds (best-effort, D-101); one real-git test proves an R
  entry parses.
- **AC-04.7** two OS processes racing first-sight allocation agree on one
  uid (D-109 subprocess test).

### TASK-05 — the proof, the non-goals and the record

Owns REQ-05, REQ-06, REQ-07, REQ-10.

- **AC-05.1** end-to-end: protect `Box.run` (active CRITICAL) → rename to
  `Box.drive` → binding unchanged (same uid), completion-relevant state
  shows migrated-not-orphaned; then ambiguous twin rename →
  `SYMBOL_IDENTITY_AMBIGUOUS` blocking; then delete the file → orphaned
  blocking. One test, three acts, all assertions on stored rows.
- **AC-05.2** the non-goals hold under grep: no `internal/semantic/`, no new
  language, no resolver/coverage/vector imports, knowledge schema still v1,
  no new CLI commands.
- **AC-05.3** every new guard added by TASK-01…04 has its mutation run and
  recorded red (REQ-07), and no timing number reaches the JSON envelope
  (REQ-10).
- **AC-05.4** the task list's MR-006 entry carries a `#### Durum` block
  written the way MR-005's is.

---

## 4. Work breakdown and dependency order

Serialised: each task is gated before the next is written.

| Task | Owns | REQ | Depends on |
|---|---|---|---|
| TASK-01 | migration 000005, `TableSchemaVersion`, two codes, ledger pins | REQ-01 | — |
| TASK-02 | allocation, thread agreement, backfill, completion stamping | REQ-02 | TASK-01 |
| TASK-03 | target resolution, binding refresh, orphan/deferred | REQ-03, REQ-09 | TASK-02 |
| TASK-04 | migration bar/cascade/Git, ambiguity, mint-last, subprocess agreement | REQ-04, REQ-08 | TASK-02, TASK-03 |
| TASK-05 | E2E proof, non-goals, record, Durum block | REQ-05, REQ-06, REQ-07, REQ-10 | TASK-04 |

---

## 5. Definition of done

1. Every acceptance criterion above is met, or is recorded as not met with
   the reason in `mr-006-findings.md`.
2. `make verify` green, `make tidy-check` green, and the test count reported
   with `go test -list '.*' ./... | grep -c '^Test'` — that command and no
   other.
3. Every new guard has a recorded mutation that turns it red, recorded after
   it was run.
4. Every task in §4 has passed its Reader/Breaker gate before the next was
   begun, and the gate's findings — confirmed, refuted, unconfirmed — are in
   `mr-006-findings.md` per task, with what was done about each.
5. The task list's MR-006 entry carries a `#### Durum` block written the way
   MR-005's is.
6. The knowledge graph is refreshed in its own commit at the end.

---

## 6. Traceability to the task list's four acceptance criteria

| Task list AC | Where it is owned |
|---|---|
| İki process aynı yeni sembol için tek bir `symbol_uid` üzerinde uzlaşır | AC-02.2 (threads), AC-04.7 (subprocess) (REQ-08) |
| Güvenilir rename/move, invariant ilişkisini aynı kimliğe taşır | AC-04.1, AC-04.2, AC-04.3, AC-03.2 (REQ-03, REQ-04) |
| Belirsiz rename korunan invariant'ı düşürmez; blocking ambiguity üretir | AC-04.4, AC-03.4 (REQ-03, REQ-04) |
| Concurrent allocation, rename, move ve ambiguity fixture testleri bulunur | AC-02.2, AC-04.1…04.4 (REQ-02, REQ-04) |
