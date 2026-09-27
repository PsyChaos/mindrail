# MR-008 — Design

- **Frozen at:** commit `fddeeec`, before any implementation commit.
- **Refines and is refined by:** [mr-008-requirements.md](mr-008-requirements.md)
  (D-132…D-145 record every place this design left a gap the implementation
  would otherwise fill silently).
- **Specification anchors:** spec-1.0 §62 (scope drift), §64
  (UNREGISTERED_CHANGE), §65 steps 4–6 (attribution, lease signals,
  ambiguity), §66 (RECONCILE_AMBIGUOUS + explicit assignment); kernel-scope
  §3 (STRUCTURAL) and §4 (deferred breadth); tech-stack §7 (layout).

## 1. What the milestone is, in one paragraph

The ownership answer on top of MR-007's discovery. After MR-008, every
changed symbol resolves to exactly one Change, or to an explicit finding
that blocks until a human assigns it or narrows the scopes: shared files
ambiguate across their candidate Changes, uncovered work is unregistered
rather than absorbed, drifted files are named per file, and lease holders
appear in guidance text — never in attribution logic. The evaluation is pure
and deterministic, so CI verify re-runs it instead of trusting it.

## 2. What it is not

| Temptation | Ruling |
|---|---|
| Fuzzy/nearest-task attribution | exact baseline-membership only (D-133) |
| Lease-based auto-assignment | guidance text only (D-134) |
| Completion gate enforcement | MR-013's; this milestone returns blocking sets |
| Findings persistence | none (D-136); inputs persist, verdicts recompute |
| Per-symbol scope narrowing | file membership only (D-133) |
| Impact/evidence/CLI/MCP | deferred per D-143; structs only |

## 3. Package plan

| Package | Owns | Deliberately not there |
|---|---|---|
| `internal/changes` (+ tables) | `scope_attributions` table + Store methods; `Scope` evaluation (candidates, outcomes, drift, overrides, blocking set, lease guidance) | matching beyond baseline membership |
| `internal/app` (+3 codes) | `SCOPE_DRIFT`, `UNREGISTERED_CHANGE`, `RECONCILE_AMBIGUOUS` with remedies | — |
| `internal/coordination` | untouched (leases read as caller input, never via store) | — |

No new packages: attribution is change-domain logic beside the rows it
judges. The `symbol` package is not involved — attribution keys off change
rows and baselines, not live index rows (D-145: undiscovered work is outside
every verdict).

## 4. Entity model

**Declared scope.** Per task: its `change_baselines` paths. No other table
widens it.

**Candidate set.** Per changed symbol row: the open Changes of tasks whose
baseline scope contains the row's file. Computed per evaluation from
`change_baselines` × `change_files`/`change_symbols` joins in Go (prefix
matching stays out of SQL per the D-84 lesson — file paths are absolute and
compared for equality, so no LIKE is needed at all: membership is exact
path equality).

**Outcome.** Per symbol: `attributed(change_id)` | `ambiguous(candidates)`
| `unregistered`. Per file: `drifted` when outside the evaluating task's
scope. Findings carry code, provenance (baseline paths read, discovery rows
judged, discovered_via of each) and `next_action`.

**Override.** `(logical_key → change_id, decided_by, reason)`. Honored
before counting; validated before writing; never expires in 0.1.

**Blocking set.** `EvaluateTask` returns every ambiguous/unregistered/drift
item with `Blocking=true`. A clean task returns empty — ALLOW-shaped, with
no verdict invented (MR-013 owns ALLOW/DENY).

## 5. Runtime schema — `migrations/000007_scope_attribution.sql`

```sql
CREATE TABLE scope_attributions (
  logical_key TEXT PRIMARY KEY,           -- what was assigned
  change_id   TEXT NOT NULL REFERENCES changes(change_id),
  decided_by  TEXT NOT NULL,              -- session/agent id, never empty
  reason      TEXT NOT NULL DEFAULT '',
  decided_at  TEXT NOT NULL               -- RFC 3339, UTC
) STRICT;
```

`changes.TableSchemaVersion = 7`. One table: findings persist nowhere by
design (D-136).

## 6. Evaluation flows

**Attribute(task):** for each of the task's change file/symbol rows —
resolve overrides first, then count candidates among all tasks' baselines.
Emit outcomes; upsert nothing (evaluation is read-only — the only writes in
this milestone are override records).

**Drift(task):** for each of the task's change file rows outside its
baseline scope — one `SCOPE_DRIFT` finding per file with kind and reason.
In-scope files never drift, even when changed.

**Unregistered(task?):** symbols with zero candidates (including every row
of a never-captured task) resolve unregistered with the absence that proves
it (which baselines were read, what they cover).

**Overrides:** `RecordAttribution(key, changeID, decidedBy, reason)` refuses
empty deciders, unknown changes and malformed keys before any write;
`EvaluateTask` consults them first, so one recorded row clears exactly its
symbol while siblings stay blocked.

**Blocking set:** `EvaluateTask(task)` unions ambiguous, unregistered and
drift findings with `Blocking=true`. Lease holders join `next_action`
when exactly one lease covers the item.

## 7. Lease guidance input

```text
LeaseView{Holder, Kind, Key} (Kind mirrors coordination `task`|`file`; file leases match by path, task leases by candidate task)
```

Kind mirrors lease target kinds (`file`, `symbol`); Key is the absolute
path or the logical key. Matching is exact equality. Zero or several
covering leases name none. The evaluation never branches on leases — only
the guidance sentence does.

## 8. Readiness, CLI and performance integration

None (D-143). Performance: evaluation touches only the task's rows plus one
baseline scan per candidate task — bounded by change size, never repository
size (§63: cost need not scale with the repository). No wall-clock
assertions in 0.1; the operation count is linear in rows read.

## 9. Outputs reserved for later milestones

- Blocking sets with codes, provenance and remedies — MR-013's gate input
  and MR-017's staged/CI input.
- Override rows — MR-015's tool input for explicit assignment.
- Attributed (change, symbol) pairs — MR-009's traversal input.

## 10. Test plan

Per tech-stack §90's categories, per task:

1. **Migration/codes** (TASK-01): v6→v7 upgrade with MR-007 rows intact;
   shipped-bytes pin; ledger columns; codes registry + distinct remedies;
   finding-type unit tests (codes, blocking flags, provenance shape);
   goldens ride the version number.
2. **Attribution** (TASK-02): one-candidate attribution + stability;
   zero-candidate unregistered + provenance; multi-candidate ambiguity
   naming all + no-choice; per-file drift + in-scope quiet; provenance and
   next_action shape on every finding.
3. **Overrides/blocking** (TASK-03): override clears one symbol while
   siblings block; empty/unknown/malformed refusals; blocking set
   composition; lease named iff exactly one covers (both directions);
   NULL-task and never-captured evaluation.
4. **Proof** (TASK-04): overlapping-baseline E2E (ambiguate → override one
   → drift third → clear all); non-goal greps; mutation ledger; Durum.
5. **Mutation ledger**: every guard gets its mutation run before it is
   written down (unchanged rule).
