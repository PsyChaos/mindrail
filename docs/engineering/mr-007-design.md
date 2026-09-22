# MR-007 — Design

- **Frozen at:** commit `8a75079`, before any implementation commit.
- **Refines and is refined by:** [mr-007-requirements.md](mr-007-requirements.md)
  (D-114…D-129 record every place this design left a gap the implementation
  would otherwise fill silently).
- **Specification anchors:** spec-1.0 §64 (protocol independence),
  §65 (reconcile workflow steps 1–5+7), §66 (ambiguity deferred);
  kernel-scope §3 (STRUCTURAL) and §5 (SLOs: after/reconcile budgets belong
  to the proof); tech-stack §87 (startup, untouched) and §74 (paths).

## 1. What the milestone is, in one paragraph

The first durable answer to "what actually changed". After MR-007, a task's
declared baseline and the repository's Git truth converge through one
discovery core into Change rows — files with kinds and hashes, symbols with
kinds, flags and uids — while the index updates underneath with identity
migration, divergence stays visible, and untasked work lands in a
retroactive Change instead of vanishing. Steps 6, 8 and 9 of spec §65
(ambiguity, impact, evidence plan) stay explicitly future work with shaped
outputs waiting for them.

## 2. What it is not

| Temptation | Ruling |
|---|---|
| Ownership ambiguity resolution | MR-008's; unattributed deltas are listed, never guessed |
| Impact analysis / traversal | MR-009's; deltas carry uids it will read |
| Validation profiles / evidence | MR-010's; hashes ride rows for later binding |
| Completion / verify enforcement | MR-013/MR-017's; findings are structs, not gates |
| Lease acquisition on before_change | out (D-126); leases stay MR-004's |
| CLI or MCP surface | none (D-127); service only, tests drive it |
| Full-repository cold drain | out; reconcile drives changed files (D-118) |

## 3. Package plan

| Package | Owns | Deliberately not there |
|---|---|---|
| `internal/changes` (new) | `Store` (5 tables + version gate), `Service` (baseline, open changes, operations, Git discovery, file delta, symbol delta, after_change, reconcile) | symbol matching, parsing, knowledge writes |
| `internal/git` (+1 function) | `StatusEntries` porcelain parse (best-effort like `DiffRenames`: failure → empty, never error) | any policy use of entries |
| `internal/app` | nothing new (D-124) | — |

`changes` imports `coordination` (task existence + operation grammar),
`git` (spelling), `index` (facts, indexer, store), `filesystem`/`storage`
(plumbing). It never imports `cli`, `bootstrap` or `knowledge` (records are
never written; invariant binding is read-only future work for MR-009+).

## 4. Entity model

**Change.** One open row per task (nullable task for retroactive): `CHG-`
ULID, `task_id`, `operation_id` (nullable, AC-15 replay), `created_at`,
`updated_at`. Updated in place by every discovery pass — rows accumulate by
natural key, never duplicate.

**File row.** `(change_id, path)` natural PK: `kind` (`added | modified |
deleted | renamed`), `old_path` (renames), `content_hash` (empty when
unhashable), `discovered_via` (`baseline | reconcile`).

**Symbol row.** `(change_id, logical_key)` natural PK: `symbol_uid`
(nullable — unidentified), `kind` (`added | removed | modified`),
`body_changed`/`signature_changed`/`structure_changed` flags,
`signature_hash`/`body_hash`/`structure_hash` (current values).

**Baseline.** `(task_id, path)` PK: `content_hash`, `captured_at`. Replaced
wholesale per capture (D-116).

**Operation log.** `operation_id` PK: `task_id`, `request_hash` (SHA-256 over
command + canonical params, the MR-004 shape), `result` (change id + counts,
JSON), `recorded_at`. Same id + same hash replays; same id + other hash is
`OPERATION_ID_CONFLICT`.

## 5. Runtime schema — `migrations/000006_changes.sql`

```sql
CREATE TABLE changes (
  change_id    TEXT PRIMARY KEY,           -- "CHG-<26>", identity.NewID
  task_id      TEXT REFERENCES tasks(task_id),
  operation_id TEXT,
  created_at   TEXT NOT NULL,              -- RFC 3339, UTC
  updated_at   TEXT NOT NULL
) STRICT;
CREATE UNIQUE INDEX idx_changes_task_unique ON changes(task_id) WHERE task_id IS NOT NULL;
CREATE INDEX idx_changes_operation ON changes(operation_id) WHERE operation_id IS NOT NULL;

CREATE TABLE change_files (
  change_id      TEXT NOT NULL REFERENCES changes(change_id),
  path           TEXT NOT NULL,              -- absolute
  kind           TEXT NOT NULL CHECK (kind IN ('added','modified','deleted','renamed')),
  old_path       TEXT NOT NULL DEFAULT '',
  content_hash   TEXT NOT NULL DEFAULT '',
  discovered_via TEXT NOT NULL CHECK (discovered_via IN ('baseline','reconcile')),
  PRIMARY KEY (change_id, path)
) STRICT;

CREATE TABLE change_symbols (
  change_id        TEXT NOT NULL REFERENCES changes(change_id),
  logical_key      TEXT NOT NULL,
  symbol_uid       TEXT REFERENCES symbol_identities(symbol_uid),
  kind             TEXT NOT NULL CHECK (kind IN ('added','removed','modified')),
  body_changed     INTEGER NOT NULL DEFAULT 0,
  signature_changed INTEGER NOT NULL DEFAULT 0,
  structure_changed INTEGER NOT NULL DEFAULT 0,
  signature_hash   TEXT NOT NULL DEFAULT '',
  body_hash        TEXT NOT NULL DEFAULT '',
  structure_hash   TEXT NOT NULL DEFAULT '',
  discovered_via   TEXT NOT NULL CHECK (discovered_via IN ('baseline','reconcile')),
  PRIMARY KEY (change_id, logical_key)
) STRICT;

CREATE TABLE change_baselines (
  task_id       TEXT NOT NULL,
  path          TEXT NOT NULL,              -- absolute, caller scope
  content_hash  TEXT NOT NULL DEFAULT '',
  captured_at   TEXT NOT NULL,
  PRIMARY KEY (task_id, path)
) STRICT;

CREATE TABLE change_operations (
  operation_id TEXT PRIMARY KEY,
  task_id      TEXT,
  request_hash TEXT NOT NULL,
  result       TEXT NOT NULL,              -- JSON: change id + counts
  recorded_at  TEXT NOT NULL
) STRICT;
```

`changes.TableSchemaVersion = 6`. Task FK is nullable-safe: NULL tasks skip
the constraint by SQL semantics, matching the retroactive shape. Partial
indexes keep the open-change and operation lookups indexed without taxing
NULL rows.

## 6. Discovery flows

**before_change(task, scope, operation?)**: resolve scope to clean absolute
paths under the worktree; hash each (empty for non-regular); replace
baseline rows; ensure open change (operation-scoped). No parsing, no Git —
it must stay under the §5 before_change budget by construction.

**after_change(task, operation?)**: load baseline; hash scope files; changed
= hash differs (or baseline entry missing for a scope file — declared but
never captured); for each changed file run the symbol delta (§7); upsert
rows with `discovered_via = baseline`; divergence = none on this path by
construction (scope-bounded), reported empty.

**reconcile(task?, operation?)**: parse porcelain (git failure → empty set,
proceed structurally); filter exclusions; for each entry run the symbol
delta (§7) with rename hints built from R entries; upsert rows with
`discovered_via = reconcile`; divergence = entries outside the task's
baseline scope (empty task scope = everything discovered, so a NULL-task
run never diverges); attribution = the named task's change, else a fresh
NULL-task change.

Convergence (AC-05.1) falls out of sharing §7: both paths funnel discovered
files through one symbol-delta core, so per-row `discovered_via` may differ
while file+symbol content cannot.

## 7. Symbol delta core

For one file with current bytes and a unit:

1. Extract current symbols + fingerprints (parse; on parse failure record
   the file delta and skip symbols — partial facts still index per D-81,
   but no symbol delta is claimed from a tree the parser disowned).
2. Load stored symbol rows for the path; match by `logical_key`.
3. Added: staged key with no stored row. Removed: stored key with no staged
   row. Both: compare the three hashes independently → flags.
4. Drive `Indexer.IndexFile` (project id threaded from the caller, rename
   hints where the Git layer supplied them): facts update, uids migrate,
   and the delta rows just computed name post-indexing uids (D-129) —
   looked up by key after the call, NULL only where identity stayed open.
5. Deleted paths skip 1–3 and mark every stored row `removed`.

The core never mints directly: all allocation flows through the indexing it
drives. Unsupported languages and unhashable files contribute file rows
only.

## 8. Readiness, CLI and performance integration

None for readiness/CLI (D-127). Performance: the §5 budgets
(`after_change` ≤10 files <1.5 s, `reconcile` ≤10 files <2 s) are met by
construction (bounded files, no repository walks — discovery reads Git
output and hashes only what it touches) and asserted in the TASK-05 proof
where a test can afford the wall clock: a ≤10-file fixture timed against
both budgets with generous margins, numbers recorded.

## 9. Outputs reserved for later milestones

- Unattributed deltas are listed per file/symbol with kinds and uids —
  MR-008's input, produced here, never guessed here.
- Change rows carry current hashes and uids — MR-009's traversal input
  and MR-010's evidence-binding input.
- Divergence entries carry path, kind and reason — MR-008's scope-drift
  input and MR-017's staged-verify input.

## 10. Test plan

Per tech-stack §90's categories, per task:

1. **Migration/store** (TASK-01): v5→v6 upgrade with MR-006 rows intact;
   shipped-bytes pin; ledger columns; natural-key upserts converge;
   goldens ride the version number.
2. **Baselines/operations** (TASK-02): capture/replace/no-stacking;
   non-regular empty hashes; open-change stability; replay same-hash,
   conflict on other-hash; baseline clearing discipline.
3. **Git/files** (TASK-03): porcelain fixtures (unstaged, staged,
   untracked, rename, delete); exclusions; non-regular rows; baseline and
   Git file deltas; file-level convergence.
4. **Symbols/after** (TASK-04): per-language fingerprint flags; index
   side effect (facts equal fresh parse, uids follow); after_change twice
   converges; staged-move hint proof (migrates) vs unstaged-move
   (mints).
5. **Proof** (TASK-05): convergence content-equality; divergence list;
   retroactive NULL-task change; separate-worktree fixture; SLO reading;
   non-goal greps; mutation ledger; Durum block.
6. **Mutation ledger**: every guard gets its mutation run before it is
   written down (unchanged rule).
