# MR-005 — Design

- **Frozen at:** commit `309073f`, before any implementation commit.
- **Refines and is refined by:** [mr-005-requirements.md](mr-005-requirements.md)
  (D-80…D-88 record every place this design left a gap the implementation
  would otherwise fill silently).
- **Specification anchors:** kernel-scope §3 (STRUCTURAL, precision limits),
  §5 (SLOs); tech-stack §25–31 (Tree-sitter, adapters, query files,
  content-addressed cache), §47–49 (scheduler, backpressure), §7 (layout);
  spec-1.0 §12 (cache), §14 (tiers), §25 (fingerprints), §107 (readiness),
  §108 (parse error), §129 (init steps); D-62 (no seventh readiness
  component).

## 1. What the milestone is, in one paragraph

The first rung of the code-intelligence ladder. After MR-005, a repository
containing Python, TypeScript or JavaScript has its ProjectUnits discovered,
its files parsed by Tree-sitter through per-language adapters, and the
structural facts — symbols, imports, explicit references, the three
fingerprints — stored in the runtime database and reflected in `status`.
Nothing in this milestone resolves a reference semantically, downloads a
language server, or blocks a command on the cold index.

## 2. What it is not

| Temptation | Ruling |
|---|---|
| FULL resolvers (Pyright, tsserver) | out of 0.1 by kernel-scope §4; not stubbed, not imported |
| Durable `symbol_uid` allocation | MR-006's; D-83 keeps the table ready, nothing more |
| Change records / evidence association | MR-007's; this milestone only makes the fingerprints exist |
| Impact graph traversal | MR-009's; D-87 stores the edge facts it will read |
| A seventh readiness component | refused again, per D-62; `inventory` and `syntax` are two of the six §107 names, dormant until now |
| Metrics on the wire | MR-019's shape; D-86 |
| Languages beyond Python/TS/JS | out of 0.1 (kernel-scope §4); a fourth extension is `unsupported`, not a roadmap item |

## 3. Package plan

Per D-80, conforming to §7's names for what 0.1 uses:

| Package | Owns | Deliberately not there |
|---|---|---|
| `internal/index` | the store over migration 000004's tables; index state read at §87 step 8; `TableSchemaVersion`; `Timing` (D-86) | — |
| `internal/index/inventory` | ProjectUnit discovery (§4 below); the facts the `inventory` readiness component reports | anything semantic |
| `internal/index/parser` | the registry (D-88's extension map), the `SyntaxAdapter` interface, four adapters, embedded query files, `ParseSchemaVersion` (D-85), parse/dispose lifecycle (§28) | any grammar-specific name outside the package |
| `internal/index/snapshot` | the content-addressed snapshot cache (D-82, §31) | persistence of facts |
| `internal/index/scheduler` | the bounded cold queue, coalescing, P0/P4 (D-84), resume | its own persistence table |
| `internal/index/symbol`, `internal/index/graph`, `internal/semantic/**` | **not created** (D-80) | — |

Dependencies point inward: `scheduler → index → parser/snapshot/inventory →
storage/app`; nothing imports upward; `internal/cli` reaches the index only
through `bootstrap`, as every other subsystem does.

## 4. Entity model

**ProjectUnit.** One analysis root: a directory containing a unit marker
(`pyproject.toml`, `package.json`, with `tsconfig.json` refining a JS unit
into TS scope). Keyed by absolute path under the repository; a file admissible
into two units belongs to the unit whose root is the longest matching prefix
(deterministic; the tie-break is a rule, not an accident). A unit row
records: id (minted, `UNT-` prefix via `identity.NewID`), path, kind
(python/typescript/javascript), discovered_at. Re-running discovery over an
unchanged tree re-reads rows; it never re-mints.

**FileIndexState.** One row per file under a discovered unit: unit_id, path,
language, content_hash (SHA-256), state (`pending | indexed | failed |
unsupported`, D-81), attempt count, last error text, indexed_at. This table
is the queue's durable state (D-84) and the resume set (D-84, AC-05.6).

**Symbol.** unit_id, path, `logical_key` (indexed, not unique — D-83), kind
(function/method/class/variable), name, container (the enclosing symbol's
key or empty), span (line/col start–end), signature_hash, body_hash,
structure_hash (§25). No uid column (D-83).

**Import.** unit_id, path, importing symbol's key (empty for module-level),
module text, imported names (JSON array), alias, is_relative.

**Reference.** unit_id, path, referencing symbol's key, target text (the
identifier as written), scope text, label (`STRUCTURAL_NAME_MATCH` when
unresolved), confidence (≤ 0.5 by rule, D-87), `resolved_symbol_id` (nullable
FK to symbols; filled only under the single-same-key rule, D-87).

## 5. Runtime schema — `migrations/000004_index.sql`

Five tables, per D-80/D-83/D-87; every column the store scans is in the
migration (the D-73 lesson applied forward — the ledger's expected-columns
list is regenerated for these tables by TASK-01):

```sql
CREATE TABLE project_units (
  id            TEXT PRIMARY KEY,           -- "UNT-<26>"
  path          TEXT NOT NULL UNIQUE,       -- absolute, below the repo root
  kind          TEXT NOT NULL,              -- python | typescript | javascript
  discovered_at TEXT NOT NULL               -- RFC 3339, UTC
) STRICT;

CREATE TABLE file_index_state (
  path         TEXT PRIMARY KEY,            -- absolute
  unit_id      TEXT NOT NULL REFERENCES project_units(id),
  language     TEXT NOT NULL,               -- registry entry name
  content_hash TEXT,                        -- NULL until first hashed
  state        TEXT NOT NULL CHECK (state IN ('pending','indexed','failed','unsupported')),
  attempts     INTEGER NOT NULL DEFAULT 0,
  last_error   TEXT,
  indexed_at   TEXT
) STRICT;
CREATE INDEX idx_file_state_unit ON file_index_state(unit_id, state);

CREATE TABLE symbols (
  id              INTEGER PRIMARY KEY,
  unit_id         TEXT NOT NULL REFERENCES project_units(id),
  path            TEXT NOT NULL,
  logical_key     TEXT NOT NULL,            -- indexed, NOT unique (D-83)
  kind            TEXT NOT NULL,
  name            TEXT NOT NULL,
  container       TEXT NOT NULL DEFAULT '',
  start_line      INTEGER NOT NULL,
  start_col       INTEGER NOT NULL,
  end_line        INTEGER NOT NULL,
  end_col         INTEGER NOT NULL,
  signature_hash  TEXT NOT NULL,
  body_hash       TEXT NOT NULL,
  structure_hash  TEXT NOT NULL
) STRICT;
CREATE INDEX idx_symbols_key ON symbols(unit_id, logical_key);
CREATE INDEX idx_symbols_path ON symbols(unit_id, path);

CREATE TABLE symbol_imports (
  id           INTEGER PRIMARY KEY,
  unit_id      TEXT NOT NULL REFERENCES project_units(id),
  path         TEXT NOT NULL,
  importer_key TEXT NOT NULL DEFAULT '',
  module       TEXT NOT NULL,
  names        TEXT NOT NULL,               -- JSON array of strings
  alias        TEXT NOT NULL DEFAULT '',
  is_relative  INTEGER NOT NULL
) STRICT;
CREATE INDEX idx_imports_path ON symbol_imports(unit_id, path);

CREATE TABLE symbol_references (
  id                 INTEGER PRIMARY KEY,
  unit_id            TEXT NOT NULL REFERENCES project_units(id),
  path               TEXT NOT NULL,
  referrer_key       TEXT NOT NULL DEFAULT '',
  target_text        TEXT NOT NULL,
  scope_text         TEXT NOT NULL DEFAULT '',
  label              TEXT NOT NULL DEFAULT 'STRUCTURAL_NAME_MATCH',
  confidence         REAL NOT NULL CHECK (confidence <= 0.5),   -- D-87, kernel-scope §3
  resolved_symbol_id INTEGER REFERENCES symbols(id)             -- nullable (D-87)
) STRICT;
CREATE INDEX idx_refs_path ON symbol_references(unit_id, path);
CREATE INDEX idx_refs_resolved ON symbol_references(resolved_symbol_id)
  WHERE resolved_symbol_id IS NOT NULL;
```

`index.TableSchemaVersion = 4`. Extraction runs inside one short transaction
per file (`InTxMeasured`): the file's rows are deleted and re-inserted, its
state row flips to `indexed`, and the write carries the timing (D-86) — the
MR-004 shape, reused because it is the shape the busy ladder already prices.

## 6. Parsing rules

- **Registry.** A fixed, test-asserted table: `.py → python`; `.js/.mjs/.cjs
  → javascript`; `.ts/.mts/.cts → typescript`; `.tsx → tsx` (D-88). Anything
  else is `unsupported` before a parser is ever built.
- **Adapters.** `SyntaxAdapter` per §29: `Parse(source) → tree`,
  `Symbols(tree) → []Symbol`, `References(tree) → []Reference`. Each adapter
  owns its query files (`queries/<lang>/{symbols,references}.scm` compiled
  once at registry construction; a query that fails to compile fails
  TASK-03's test, never a user's command — AC-03.3). Fingerprinting
  (`signature_hash` over the signature subtree's bytes, `body_hash` over the
  body's, `structure_hash` over the shape — child kinds in order) is shared
  adapter machinery, not per-language code.
- **Resources.** Parsers and query cursors are closed in the adapter's own
  scope; the 1,000-cycle leak test (AC-03.4) runs in the default suite (§28).
- **Partial trees.** A parse that returns a tree and an error is both used:
  what was extracted is stored, the file's state is `failed`, the error text
  is its `why`, and `syntax` reports DEGRADED while any such row exists
  (§108, D-81).

## 7. Incremental reindex and resume

The unit of work is one file. Its hash decides everything: hash equal to the
recorded one → no parse, no write (AC-05.4); different or unrecorded → parse
through a snapshot, extract, replace this file's rows in one transaction. The
resume set is a query: every `file_index_state` row whose state is not
`indexed` and not `unsupported` (D-84). There is no index-run journal to
corrupt and no queue table to drain — interrupting a run loses only the
in-flight file, which AC-05.6 and AC-07.2 assert at unit scale and process
scale respectively.

## 8. The scheduler

One bounded queue of unit-file work items, one pending item per path
(coalescing, §49), drained by parse workers. Priorities: P0 — a request
targeting a file reorders its unit's pending files to the front (queue order
only; no in-flight parse is preempted, §48); P4 — the cold remainder in
deterministic path order. P1–P3 are named constants with §47's meanings in
their comments and nothing else (D-84). The scheduler owns no state of its
own beyond the in-memory queue; its durable state is `file_index_state`.

## 9. Readiness integration

Two of §107's six components wake up:

- **`inventory`** — from discovery: units found, files under them; the
  observation vocabulary MR-001 established (`observed` / `not_observed` /
  `indeterminate`), unchanged.
- **`syntax`** — from index state: `UNINITIALIZED` (no units), `INVENTORY`
  (units, nothing indexed yet), `INDEXING` (pending work remains),
  `READY` (none), each with a pending count where honest; `DEGRADED` over
  any `failed` row (§108). No filesystem walk on the read path — the counts
  come from the persisted state, and re-derivation happens in the
  scheduler's normal work (AC-06.4).

`status`'s JSON gains keys under these two components; the goldens and the
schema version ride them (AC-06.5). `doctor` is not extended in this
milestone — the damaged-row check the MR-004 gates carried forward is
backlog, and adding a seventh check here would outrun the record.

## 10. Test plan

Per tech-stack §90's categories, per task:

1. **Adapter fixtures** (TASK-03/05): one small package per language with a
   module function, a class method, the import forms of AC-05.1, and an
   explicit call — asserted through the store, not through parser internals.
2. **Cache/identity** (TASK-04): sharing, worktree sharing, deletion
   invisibility, version-bump invalidation — four tests, each with its
   mutation.
3. **Interruption** (TASK-05/07): a cancelled context mid-file at unit scale;
   a killed process mid-run at process scale.
4. **Queue** (TASK-06): bound, coalescing, preemption order, no preemption
   in flight.
5. **Readiness** (TASK-06): the lifecycle states through `status --json`,
   goldens updated; DEGRADED over a seeded `failed` row.
6. **Proof** (TASK-07): the synthetic large inventory of AC-07.1, the SLO
   reading, the resume-at-scale, and the non-goal greps.
7. **Mutation ledger**: every guard gets its mutation run before it is
   written down (the rule MR-003's round 2 added, §7 rule 8).
