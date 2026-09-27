# MR-006 — Design

- **Frozen at:** commit `b32399b`, before any implementation commit.
- **Refines and is refined by:** [mr-006-requirements.md](mr-006-requirements.md)
  (D-93…D-109 record every place this design left a gap the implementation
  would otherwise fill silently).
- **Specification anchors:** spec-1.0 §24 (identity, allocation, migration,
  ambiguity, orphan protection), §25 (fingerprints), §33 (logical_key as
  index); kernel-scope §3 (STRUCTURAL precision limits) and §4 (FULL
  resolvers out); tech-stack §7 (layout), §29 (adapters, untouched).

## 1. What the milestone is, in one paragraph

The second rung of the code-intelligence ladder. After MR-006, every symbol
row can carry a durable `symbol_uid` that survives renames and moves; two
processes racing first sight agree on one uid through the database; an
invariant's runtime binding follows the uid across a confident rename; an
unconfident one becomes explicit, blocking ambiguity or orphanhood instead of
a silent drop. Nothing in this milestone resolves a reference semantically,
touches the knowledge schema, or adds a CLI command.

## 2. What it is not

| Temptation | Ruling |
|---|---|
| Semantic matching (types, resolver identity) | FULL work; deferred with the resolvers (D-106) |
| Knowledge schema v1 change | frozen; bindings are runtime rows (D-98) |
| CLI surface (`mindrail` commands, status keys) | none (REQ-05); findings are stored + returned, not rendered |
| Completion gate enforcement | MR-013's; this milestone produces findings with codes |
| Cross-file reference resolution | D-90 stands; matching is same-unit, file-scoped keys |
| Evidence/lease relation migration | relations stay row-anchored (D-108); only invariant bindings follow uids in 0.1 |

## 3. Package plan

| Package | Owns | Deliberately not there |
|---|---|---|
| `internal/index` (root) | migration 000005 tables; `Store` identity/binding/ambiguity methods; `FileFacts.RenameHints`; completion-path stamping inside `replaceFileFactsTx` | matching policy, target grammar |
| `internal/index/symbol` (new) | `Service`: allocation orchestration, migration matching, target resolution, binding refresh, cascade order, Git hint fetching | SQL text (calls `Store`), grammar specifics |
| `internal/git` (+1 method) | `RenameEntries(ctx, dir, paths...)`: best-effort R-entry set | any policy use of the set |
| `internal/app` (+2 codes) | `SYMBOL_IDENTITY_AMBIGUOUS`, `ORPHANED_PROTECTED_SYMBOL` with remedies | — |

Dependencies point inward: `symbol → index → parser/snapshot/inventory →
storage/app`; `symbol` reads knowledge loader types but never writes
`.mindrail/`; `internal/cli` and `bootstrap` do not import `symbol` in 0.1
(no driver wiring — the MR-005/D-91 pattern: services compose explicitly in
tests, milestone drivers wire them later).

## 4. Entity model

**Identity.** One lineage: `symbol_uid` (`SYM-` ULID), `project_id`,
`unit_id`, `language`, `logical_key` (mutable current key), `container_uid`
(nullable parent uid, resolved when the parent is known), `previous_keys`
(JSON array of abandoned keys — the lineage memory a refresh consults when a
target text predates a migration), `created_at`. UNIQUE on
`(project_id, unit_id, language, logical_key)` — the D-94 allocation key.
Migration rewrites `logical_key` (and `container_uid`) and appends the old
key; it never mints, and never steals a key owned by another uid.

**Symbol row.** Gains nullable `symbol_uid`. Rows are stamped in the same
transaction that writes their facts (D-95). Uniqueness is not enforced on the
column (overloads share a uid by lineage).

**Binding.** `(invariant_id, symbol_uid)` with status
`bound | ambiguous | orphaned`, a reason code, and `updated_at`. The pair is
the grain: one invariant may bind several uids (multi-target scopes bind
each); one uid may serve several invariants.

**Ambiguity.** One row per blocked decision: removed identity uid, the
candidate new keys (JSON array), the bar scores that met, `created_at`.
Candidates that later resolve do not delete the row (audit trail); refresh
supersedes its effect by rebinding.

## 5. Runtime schema — `migrations/000005_symbol_identity.sql`

```sql
CREATE TABLE symbol_identities (
  symbol_uid    TEXT PRIMARY KEY,           -- "SYM-<26>"
  project_id    TEXT NOT NULL,
  unit_id       TEXT NOT NULL REFERENCES project_units(id),
  language      TEXT NOT NULL,              -- registry entry name
  logical_key   TEXT NOT NULL,              -- mutable current key (D-94)
  container_uid TEXT REFERENCES symbol_identities(symbol_uid),
  previous_keys TEXT NOT NULL DEFAULT '[]', -- abandoned keys, JSON array (D-96)
  created_at    TEXT NOT NULL               -- RFC 3339, UTC
) STRICT;
CREATE UNIQUE INDEX idx_identity_alloc
  ON symbol_identities(project_id, unit_id, language, logical_key);
CREATE INDEX idx_identity_unit_key
  ON symbol_identities(unit_id, logical_key);

CREATE TABLE invariant_symbol_bindings (
  invariant_id TEXT NOT NULL,               -- INV-xxxx, repo-owned record id
  symbol_uid   TEXT NOT NULL REFERENCES symbol_identities(symbol_uid),
  status       TEXT NOT NULL CHECK (status IN ('bound','ambiguous','orphaned')),
  reason       TEXT NOT NULL DEFAULT '',
  updated_at   TEXT NOT NULL
) STRICT;
CREATE UNIQUE INDEX idx_binding_pair
  ON invariant_symbol_bindings(invariant_id, symbol_uid);

CREATE TABLE symbol_identity_ambiguities (
  id                INTEGER PRIMARY KEY,
  unit_id           TEXT NOT NULL REFERENCES project_units(id),
  removed_uid       TEXT NOT NULL REFERENCES symbol_identities(symbol_uid),
  removed_key       TEXT NOT NULL,
  candidate_keys    TEXT NOT NULL,          -- JSON array of logical_key
  created_at        TEXT NOT NULL
) STRICT;

ALTER TABLE symbols ADD COLUMN symbol_uid TEXT
  REFERENCES symbol_identities(symbol_uid);
```

`ON DELETE` behavior follows D-89's precedent discussion in TASK-01's record:
references to identities use the default (restrictive) posture inside the
index's own prune paths — unit/file pruning deletes facts, and identities
without rows are historic lineage, not garbage: they are what a later
migration matches against. Nothing in 0.1 deletes identities.

`index.TableSchemaVersion = 5`.

## 6. Allocation

`EnsureIdentity(projectID, unit, key, fingerprints, hints)`:

```text
lookup identity by (project, unit, language, key)
    ↓ hit → return uid
    ↓ miss → migration attempt (§7) over disappeared keys
        ↓ exactly one confident → rewrite its logical_key, return uid
        ↓ several confident → record ambiguity, return Ambiguous (no uid)
        ↓ none → insert-if-absent (mint SYM-uid)
                 ↓ unique conflict → read back winner, return it
                 ↓ inserted → return minted
```

Insert-then-read-back only: `read → absent → insert` never appears, per
spec §24. The UNIQUE index is the arbiter across processes (D-109).

Backfill (`BackfillUnit`): for each distinct key with uid-less rows,
lookup-or-mint and stamp — never migration (D-100), never reparse.

Completion stamping: `replaceFileFactsTx` calls Ensure per distinct key of
the incoming facts (parents before children for the cascade, §7), then
inserts rows with the returned uid. Ambiguous keys insert rows with NULL uid
and their ambiguity row in the same transaction: the facts land, the identity
question stays open, and nothing is guessed.

## 7. Rename/move matching

Disappeared set: identities of the unit whose key has zero live symbol rows
*after* the incoming facts are staged (matching sees the post-write world;
it runs inside the same transaction). Added set: staged keys with no
identity. For each disappeared key, score every added key:

```text
must:      body_hash equal AND kind equal AND container-local equal
path gate: same file path OR git-corroborated old-path → new-path
```

Meeting both is "meeting the bar". Exactly one meeting candidate migrates
(rewrite identity key, adopt container_uid remap); several record one
ambiguity row and mint nothing; zero proceeds to mint (AC-04.5).

Cascade: stage order is parents-first (container depth ascending); each
migration registers `oldContainerKey → newContainerKey`, and children's
container comparison applies the remap before scoring. Git hints
(`FileFacts.RenameHints`, `old → new` path pairs from `git diff
--name-status -z -M`) satisfy the path gate for moves; without them a move
mints anew (AC-04.2, documented).

Scores and the bar live in `symbol` as pure functions over stored
fingerprints, so gates can unit-test the policy without a database.

## 8. Target resolution and binding refresh

`ResolveTarget(projectID, unit, path, dotted)` walks the file's live rows:
the first segment must match exactly one row name at the root container;
each further segment exactly one row name under the previous row's local
key; the final row's `symbol_uid` ( NULL → unresolvable) is the answer.
Overloads converging on one uid bind; divergence is ambiguous; an empty walk
is unresolvable. Container kinds are never inferred from text — a segment
naming two distinct declarations is ambiguous, not guessed (D-97).

`RefreshBindings(projectID, invariants)` resolves every active invariant
target with a SYMBOL or FILE level (MODULE/PACKAGE prefix-match the path;
PROJECT binds nothing) and upserts binding rows: `bound` on a uid,
`ambiguous` with the code, `orphaned` with the code under D-99's evidence
rules, and *deletion* of stale bound rows the refresh no longer supports —
a binding the world stopped supporting must not linger as `bound`. Resolution
tries the live-row walk first, then the `previous_keys` lineage memory (a
stale target text resolving to a migrated uid with live rows binds it as
migrated, not orphaned). Pending files defer silently. Blocking is reported
per binding (severity + activeness) for MR-013; nothing here blocks anything.

## 9. Readiness integration

None. No status keys, no doctor checks, no CLI commands in 0.1 (REQ-05).
`init` reports `schema_version` 5 through the existing shape (AC-01.2 rides
the established golden).

## 10. Test plan

Per tech-stack §90's categories, per task:

1. **Migration/store** (TASK-01): v4→v5 upgrade with MR-005 rows intact;
   shipped-bytes pin; ledger columns; codes registry + distinct remedies;
   knowledge-tree untouched assertion.
2. **Allocation** (TASK-02): lookup-hit idempotence; thread race over two
   handles → one row; backfill stamps + rerun no-op; completion stamping +
   identical-facts recompletion keeps uids; overloads share.
3. **Resolution/bindings** (TASK-03): scope-form fixtures; rename-following
   refresh; cold deferral (no rows, no findings); orphan over indexed/failed
   and READY-vanished paths; knowledge-tree mtime/content invariance.
4. **Migration** (TASK-04): same-file rename; Git-corroborated move;
   move-without-Git mints; class cascade; twins ambiguity + blocking gates
   by severity; mint-last ordering; Git-failure empty hints; subprocess
   agreement.
5. **Proof** (TASK-05): three-act E2E on stored rows; non-goal greps
   (extended: no `internal/semantic/`, still 4 languages, schema still v1,
   no new commands); mutation ledger; Durum block.
6. **Mutation ledger**: every guard gets its mutation run before it is
   written down (the MR-003 round-2 rule, unchanged).
