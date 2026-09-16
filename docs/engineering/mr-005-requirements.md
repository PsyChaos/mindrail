# MR-005 — Frozen requirements and acceptance criteria

- **Frozen at:** commit `309073f`, branch `mr-002-knowledge-lifecycle`
- **Contract this refines:** [mr-005-design.md](mr-005-design.md). Where the two
  disagree, the design wins on intent and this document wins on the detail the
  design left open — every such gap is recorded below as a decision D-80…D-88.
- **Task:** [mindrail-0.1-task-list.md](mindrail-0.1-task-list.md) MR-005
- **Tier:** 3 (Program). It adds a runtime migration, five new persisted
  entities, three `app.Code` values that enter the wire vocabulary, four
  grammar modules behind CGO, the first two populated readiness components
  beyond `runtime_db`/`knowledge`, and the cold index the startup sequence
  has been deferring since MR-001. The escalation rule fires on "public API
  contract" and "database schema" regardless of line count.

This file is frozen **before** implementation so that an audit has something
to grade against that the implementation did not shape. The milestone runs
**one task at a time** as MR-004 did: each task in §4 is implemented, gated by
an independent Reader/Breaker pair and recorded before the next is begun.

---

## 0. Baseline evidence

Captured on the tree this work starts from, so a red suite later is unambiguous.

| Measurement | Value | Command |
|---|---|---|
| Commit | `309073f` | `git rev-parse --short HEAD` |
| Go toolchain | `go1.27.0 linux/amd64` | `go version` |
| Packages | 21 | `go list ./... \| wc -l` |
| Top-level test functions | 872 | `go test -list '.*' ./... \| grep -c '^Test'` |
| `make verify` | green — exit 0 across check, race and smoke | `make verify` |
| `make tidy-check` | green, exit 0 | `make tidy-check` |
| Migrations | `000001_initial.sql`, `000002_coordination.sql`, `000003_lease_idempotency.sql`; runtime `schema_version` reports 3 | `ls migrations/*.sql`; `internal/cli/testdata/status_human.golden` |
| Registered `app.Code` values | 35 | `grep -c 'Code = "' internal/app/code.go` |
| `go.mod` direct requires | `go-toml/v2 v2.4.3`, `jsonschema/v6 v6.0.3`, `cobra v1.10.2`, `modernc.org/sqlite v1.58.0` | `go.mod` |
| `internal/index` | does not exist | `ls internal/` |

---

## 1. The scenario, in one paragraph

Mindrail changes a Python, TypeScript or JavaScript function body and
recognises that change in STRUCTURAL mode — no FULL resolver, no language
server, no semantic analysis. Discovery finds the ProjectUnits of the
repository; a Tree-sitter registry parses files through per-language adapters;
extracted symbols, imports, explicit references and the three fingerprints are
stored; an unchanged file is never parsed twice; an interrupted cold index
resumes from persisted state without re-processing completed hashes; a request
aimed at an unindexed unit moves it ahead of the cold remainder; and until the
remainder drains the readiness answer says so explicitly instead of pretending
otherwise.

---

## 2. Decisions

MR-004 ended at D-79. These continue the sequence. Each one existed as an
ambiguity two implementers would have resolved differently, or as a place
where the specification leaves the milestone to decide.

### D-80 — the index lands in `internal/index/`, and the deviation from §7's tree is named once

Technical Specification §7 prescribes `internal/index/{inventory, scheduler,
snapshot, parser, symbol, graph}` and `internal/semantic/{…, projectunit, …}`.
0.1 creates only the packages it uses — `internal/index` (the store, the index
state, the per-package `TableSchemaVersion`), `internal/index/inventory`
(ProjectUnit discovery), `internal/index/parser` (the Tree-sitter registry,
adapters and query files), `internal/index/snapshot` (the content-addressed
parse snapshot cache) and `internal/index/scheduler` (the cold queue). It does
**not** create `internal/semantic/` at all, and does not create
`internal/index/symbol` or `internal/index/graph`: durable symbol identity is
MR-006's, the impact graph is MR-009's, and empty packages are not layout.
ProjectUnit discovery lives in `internal/index/inventory`, not
`internal/semantic/projectunit` — nothing semantic exists in 0.1 to sit beside
it. This is one decision covering both deviations from §7's tree, recorded
here so MR-006/MR-009 do not inherit a rename they cannot see the reason for.
The store and its tables stay in the `internal/index` root package; §7's
`symbol/` and `graph/` are extraction consumers, not owners of the schema.

### D-81 — unsupported is a terminal state, not pending; a partial parse is DEGRADED

The lifecycle of spec-1.0 §107 (`UNINITIALIZED → INVENTORY → PARTIAL_READY →
INDEXING → READY`) must be able to reach READY. A repository that contains
anything other than Python, TypeScript or JavaScript always will contain such
files — `Makefile`, `.md`, images — so "every file is indexed" is not a
condition 0.1 can meet. Files whose language has no adapter are recorded
**out of scope** (`unsupported`) and are not pending work: they neither block
READY nor appear in the pending count. A file whose grammar exists but whose
parse produced a partial tree, or whose extraction failed, is `failed`: it
stays pending, it is named by its `why`, and per spec-1.0 §108 it reports
`syntax_health = DEGRADED` while any failed row exists. READY is reached when
no `pending` or `failed` row remains for a discovered unit.

### D-82 — snapshots on disk beside the database; facts in SQLite

The content-addressed parse snapshot (tech-stack §31) is a disk artifact under
the runtime directory — the same `.git/mindrail` scope the database travels
in, which is the repository's **common dir**, so two linked worktrees sharing
one project share one snapshot store, as §31 requires. Its key is
`language / grammar version / parser schema version / content hash`, hashed
with `crypto/sha256`. SQLite stores only the durable facts: index state,
symbols, imports, references. A snapshot is a cache in the strict sense — any
snapshot can be re-derived by parsing, so losing one is never an error and
never reported as one.

### D-83 — `logical_key` is an index, not an identity

A symbol's `logical_key` (path + kind + name + container) identifies a symbol
only until a rename or an overload makes two meanings share one key.
Spec-1.0 §33 is explicit that the mapping "bir indextir, kimliğin kendisi
değil", so the `symbols` table carries a plain index on `logical_key`, **not**
a UNIQUE constraint, and no `symbol_uid` column: allocation is MR-006's
insert-then-read-back atomic step (AC-07/AC-18), and it will arrive as an
`ALTER TABLE … ADD COLUMN` that the ledger's expected-columns list is amended
for (the D-73 lesson, applied forward).

### D-84 — the queue is P0 and P4; P1–P3 are constants, not scaffolding

Tech-stack §47 names five priority classes. 0.1 implements two: **P0**, a
request aimed at an unindexed unit reorders it ahead of the cold remainder
(the task list's AC), and **P4**, the cold remainder itself, in deterministic
path order. P1–P3 exist as named constants with their §47 meanings in their
doc comments and no machinery behind them — an empty scheduling ladder is
honest documentation; a fake scheduler is a defect. Work is never preempted
mid-parse (§48); priority applies to queue order only. The queue is bounded,
one pending job per file path (duplicate jobs coalesce), and its durable work
state is the `file_index_state` table itself: a process that dies mid-index
loses only the in-flight file, and resume is "every row that is not `indexed`
and not `unsupported`", which is why no separate queue-persistence table
exists.

### D-85 — the parser schema version is a constant in `internal/index/parser`, not the migration ledger

Tech-stack §31 puts a "parser schema version" in the snapshot key — the thing
that changes when the extraction rules change shape (a new fingerprint field,
a corrected query file) and every stored snapshot must invalidate. It is
`parser.ParseSchemaVersion = 1`, bumped by hand when extraction semantics
change. It is deliberately not the migration ledger's version and not any
package's `TableSchemaVersion`: those describe the database, this describes
the extractor. Bumping it must not require a migration, and migrating must not
invalidate snapshots.

### D-86 — parse and extract timings are collected and never reach the wire

Collected in a `Timing` struct on the same pattern as MR-004's `TxStats`
(held/waited), logged at DEBUG. The JSON envelope carries none of it — the
D-75 ruling stands, and MR-019 owns the published shape. The STRUCTURAL SLOs
of kernel-scope §5 are met by construction and asserted only where a test can
afford the wall clock (TASK-07's proof, not the default suite).

### D-87 — reference edges are stored as facts about one file, with a nullable pointer to a symbol row

An explicit call or reference extracted from file F names a target by
identifier text and the scope it appears in (§3's "obvious references/calls");
that is all a Tree-sitter pass without a resolver knows. The `references`
table additionally carries `resolved_symbol_id`, nullable, filled only when
extraction finds exactly one same-`logical_key` symbol row in the same unit —
a deterministic, local resolution that costs nothing and gives MR-009's
impact graph something to traverse without re-resolution. When zero or
several same-named declarations exist, the pointer stays NULL and the edge is
labelled `STRUCTURAL_NAME_MATCH` with confidence ≤ 0.5 (kernel-scope §3); the
reverse-depth walk is not built in 0.1 at all. A structural-only edge may not
raise validation breadth above MODULE (AC-22) — that rule binds MR-009's
consumer and is recorded here only so the field it reads exists.

### D-88 — TSX is its own grammar, not a TypeScript flag

Tech-stack §25 lists "TypeScript / TSX" — the official tree-sitter-typescript
module ships two grammars, and parsing `.tsx` with the typescript grammar
fails on JSX, which would make every React file a D-81 `failed` row. Four
language entries ship: `python`, `javascript`, `typescript` (`.ts`, `.mts`,
`.cts`), `tsx` (`.tsx`), each with its own query directory under
`queries/`. Extension mapping lives in the registry; ambiguity resolves
deterministically (first entry wins, order fixed in the registry's own test).

---

## 3. Requirements and acceptance criteria

REQ-01…REQ-12. Each task's criteria are dotted under it, in the form the
gates grade. A criterion not met at the end of its task is recorded as not
met, with the reason, in `mr-005-findings.md`.

**REQ-01 — ProjectUnit discovery** (`internal/index/inventory`).
**REQ-02 — language registry and adapters** (`internal/index/parser`).
**REQ-03 — content-addressed snapshot cache** (`internal/index/snapshot`).
**REQ-04 — structural extraction** (parser → store).
**REQ-05 — incremental reindex and resume** (store + scheduler).
**REQ-06 — cold queue and preemption** (`internal/index/scheduler`).
**REQ-07 — readiness** (`internal/status` wiring of `inventory` and `syntax`).
**REQ-08 — error codes** (`internal/app`).
**REQ-09 — CLI surface** (`internal/cli`: status/doctor; no new commands).
**REQ-10 — non-goals, guarded**.
**REQ-11 — evidence discipline** (every new guard mutation-red).
**REQ-12 — timing off the wire**.

### TASK-01 — migration 000004 and the index store

Owns REQ-08 and the schema half of REQ-01.

- **AC-01.1** `migrations/000004_index.sql` creates `project_units`,
  `file_index_state` (with the `state` column of D-81: `pending`, `indexed`,
  `failed`, `unsupported`), `symbols` (plain `logical_key` index per D-83, no
  uid column), `symbol_imports` and `symbol_references` (with the nullable
  `resolved_symbol_id` of D-87), and nothing else.
- **AC-01.2** `index.TableSchemaVersion = 4` gates the store the way
  `coordination.TableSchemaVersion` does since MR-003.
- **AC-01.3** the three codes of REQ-08 are registered with distinct remedies
  and enter `allCodes` (the registry test enforces it).
- **AC-01.4** `migrations/shipped_test.go` pins the new file's bytes; the
  ledger's expected-columns list is extended for every table the migration
  creates (the D-73 lesson).
- **AC-01.5** the migration applies cleanly to a database at version 3
  carrying MR-004's rows, and `init` reports `schema_version` 4.

### TASK-02 — ProjectUnit discovery

Owns REQ-01.

- **AC-02.1** a repository with a Python package, a TS package and a JS
  package yields three units, keyed deterministically (decision recorded in
  the design §4; a file admissible to two units resolves by the rule
  spec-1.0 §(inventory) states: deterministic, and the tie is broken the
  same way on every run).
- **AC-02.2** a repository with none of the three yields zero units and the
  syntax component reports `INVENTORY`-state honestly (no unit, no index,
  no READY).
- **AC-02.3** discovery is read-only over the filesystem and idempotent:
  running it twice records the same units and changes nothing else.
- **AC-02.4** discovery runs inside the startup sequence at §87 step 8's
  neighbourhood without a blocking step (the sequence's shape is unchanged
  apart from the new step's work), and its findings are what `status`'s
  `inventory` component reports.

### TASK-03 — the language registry and adapters

Owns REQ-02.

- **AC-03.1** the registry maps the extensions of D-88 to four language
  entries backed by the official grammar modules; the mapping table and its
  tie-break are asserted by test.
- **AC-03.2** adapters implement the `SyntaxAdapter` interface (§29:
  Parse/Symbols/References) and no code outside `internal/index/parser`
  names a grammar-specific node type.
- **AC-03.3** query files ship embedded per language under `queries/`, and a
  query file that fails to compile fails a test, not a user's command.
- **AC-03.4** repeated parse/dispose cycles (a loop of at least 1,000 parses
  per language) show no resource growth (§28: no finalizer reliance), in a
  test that runs in the default suite.
- **AC-03.5** a file that is valid UTF-8 but syntactically broken produces a
  partial tree and a parse error; the adapter reports both rather than
  panicking or silently truncating (D-81's DEGRADED input).

### TASK-04 — the snapshot cache

Owns REQ-03.

- **AC-04.1** the cache key is `language / grammar version / parser schema
  version / content hash`, SHA-256 (§31); two files with identical content
  in one worktree share one snapshot entry.
- **AC-04.2** a snapshot written through one worktree is found through
  another (the common-dir scope of D-82), asserted by a test that opens the
  same repository from two worktree paths.
- **AC-04.3** deleting the snapshot store entirely is invisible to
  correctness: the next index run repopulates it and the index facts agree
  with a run that had it warm (D-82: a snapshot is re-derivable).
- **AC-04.4** bumping `parser.ParseSchemaVersion` invalidates every snapshot
  without touching the database (D-85), asserted by test.

### TASK-05 — structural extraction and the incremental path

Owns REQ-04 and REQ-05. This is the milestone's centre of mass.

- **AC-05.1** per language, a fixture package exercises: a module-level
  function, a method in a class, an import (plain, aliased, from-import for
  Python; default/named for JS/TS), and an explicit call of a same-file
  symbol — and the stored rows say so (the task list's first AC).
- **AC-05.2** every symbol row carries `signature_hash`, `body_hash` and
  `structure_hash` (spec-1.0 §25); changing a body changes `body_hash` and
  nothing else, changing a signature changes `signature_hash` and
  `structure_hash`, asserted by fixture.
- **AC-05.3** an explicit call whose target has exactly one same-`logical_key`
  symbol row in the unit stores `resolved_symbol_id`; a call to an ambiguous
  name stores NULL and the `STRUCTURAL_NAME_MATCH` label with confidence
  ≤ 0.5 (D-87, kernel-scope §3).
- **AC-05.4** re-indexing a file whose content hash is unchanged performs no
  parse and writes nothing (the task list's second AC) — asserted by a
  counter or an observable, not by timing.
- **AC-05.5** a changed file is re-extracted in place: its old symbol rows
  are replaced, not appended to, and no row of another file's symbols moves.
- **AC-05.6** interrupting an index run (a cancelled context mid-file) leaves
  completed hashes recorded and the in-flight file un-recorded; the resumed
  run processes exactly the un-recorded remainder and nothing else (the task
  list's third AC).

### TASK-06 — the scheduler, preemption and readiness

Owns REQ-06, REQ-07 and REQ-09.

- **AC-06.1** the queue is bounded; a repository of N pending files never
  holds N queued jobs for one path (coalescing, D-84).
- **AC-06.2** a request aimed at an unindexed unit's file reorders that
  unit's files ahead of the cold remainder; no in-flight parse is
  interrupted (the task list's fourth AC, D-84).
- **AC-06.3** `status` reports the `inventory` component from discovery and
  the `syntax` component from index state: PARTIAL_READY/pending is explicit
  with a pending count while the cold index runs, READY when none remains,
  DEGRADED while any `failed` row exists (the task list's fifth AC, D-81,
  §108).
- **AC-06.4** a `status` run on a large-index repository does no hashing
  beyond the files it must judge and answers within the §5 SLO budget in the
  TASK-07 proof; in the default suite it is asserted structurally (no
  filesystem walk on the read path), not by timing.
- **AC-06.5** the status goldens gain the new keys; the JSON schema version
  rides them (the MR-002/004 precedent).

### TASK-07 — the proof, the non-goals and the record

Owns REQ-10, REQ-11, REQ-12.

- **AC-07.1** a large synthetic inventory (thousands of files across the
  three languages) shows: PARTIAL_READY reported while the cold index runs,
  init returning without waiting for it (spec-1.0 §129), the pending count
  draining to READY, and a preemption moving a targeted unit ahead of the
  remainder — all demonstrated, with the numbers recorded.
- **AC-07.2** the interrupted-run resume of AC-05.6 is demonstrated at
  process scale (kill a cold-index process mid-run, restart, observe the
  remainder only).
- **AC-07.3** the non-goals hold under grep: no FULL-resolver code, no
  `internal/semantic/`, no fourth language, no coverage provider, and the
  three codes' remedies are each distinct.
- **AC-07.4** every new guard added by TASK-01…06 has its mutation run and
  recorded red (REQ-11), and no timing number reaches the JSON envelope
  (REQ-12, D-86).

---

## 4. Work breakdown and dependency order

Serialised: each task is gated before the next is written. A task ends with
its stated mutation red, the suite green, a commit, a Reader over its record
and a Breaker over its change, and a row in the findings document — the
process MR-004 ran and round 2 of MR-003's audit validated.

| Task | Owns | REQ | Depends on |
|---|---|---|---|
| TASK-01 | migration 000004, the index store, `TableSchemaVersion`, the three codes | REQ-01 (schema), REQ-08 | — |
| TASK-02 | ProjectUnit discovery, the `inventory` component | REQ-01 | TASK-01 |
| TASK-03 | registry, four adapters, query files, leak tests | REQ-02 | — |
| TASK-04 | the snapshot cache | REQ-03 | TASK-03 |
| TASK-05 | extraction, fingerprints, the incremental path | REQ-04, REQ-05 | TASK-01, TASK-03, TASK-04 |
| TASK-06 | scheduler, preemption, readiness, status wiring | REQ-06, REQ-07, REQ-09 | TASK-02, TASK-05 |
| TASK-07 | the proof, the non-goals, the record, the Durum block | REQ-10, REQ-11, REQ-12 | TASK-06 |

TASK-02 and TASK-03 touch disjoint files; they are still run one after the
other, because the gate is the point (the MR-004 §4 sentence, repeated
because it held).

---

## 5. Definition of done

1. Every acceptance criterion above is met, or is recorded as not met with
   the reason in `mr-005-findings.md`.
2. `make verify` green, `make tidy-check` green, and the test count reported
   with `go test -list '.*' ./... | grep -c '^Test'` — that command and no
   other.
3. Every new guard has a recorded mutation that turns it red (AC-11.3 of
   MR-004's discipline, AC-07.4 here), recorded after it was run.
4. Every task in §4 has passed its Reader/Breaker gate before the next was
   begun, and the gate's findings — confirmed, refuted, unconfirmed — are in
   `mr-005-findings.md` per task, with what was done about each.
5. The task list's MR-005 entry carries a `#### Durum` block written the way
   MR-003's and MR-004's are.
6. The knowledge graph is refreshed in its own commit at the end.

---

## 6. Traceability to the task list's five acceptance criteria

| Task list AC | Where it is owned |
|---|---|
| Fixture'lar declaration, body, import, explicit call/reference çıkarımını doğrular | AC-05.1, AC-05.3 (REQ-04) |
| Değişmemiş content hash yeniden parse edilmez | AC-05.4 (REQ-05) |
| Kesilen indeksleme tamamlanmış hash'leri tekrar işlemeden devam eder | AC-05.6, AC-07.2 (REQ-05) |
| Unindexed aktif ProjectUnit cold queue önüne alınır | AC-06.2 (REQ-06) |
| Büyük inventory sonrası PARTIAL_READY/pending açıkça raporlanır | AC-06.3, AC-07.1 (REQ-07) |
