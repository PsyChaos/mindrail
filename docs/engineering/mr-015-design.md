# MR-015 — Design

- **Frozen at:** commit `5b42d40`, before any implementation commit.
- **Refines and is refined by:** [mr-015-requirements.md](mr-015-requirements.md)
  (D-195…D-204 record every place this design left a gap the implementation
  would otherwise fill silently).
- **Specification anchors:** spec-1.0 §64 (agent-independent discovery),
  §89 area (completion sequencing), coordination lifecycle (claim → work →
  handover); AC-20 (agent independence); kernel-scope §3 (STRUCTURAL) and
  §4 (deferred breadth); tech-stack §7 (layout).

## 1. What the milestone is, in one paragraph

The doing surface beside MR-014's knowing surface: after MR-015, agents
claim tasks, declare scope, record changes, reconcile cold trees and hand
checkpoints across sessions — eleven tools on one server, every write
idempotent or conflicting loudly, every result carrying the state of the
work beside the data.

## 2. What it is not

| Temptation | Ruling |
|---|---|
| Reimplemented coordination/change logic | bind stores/services (D-196) |
| Mandatory before_change | optional optimization (D-197) |
| Second reconcile implementation | exec git runner + existing service |
| New conflict/ambiguity codes | existing vocabulary (D-198) |
| Auto-handover policies | explicit checkpoint reads only |
| CLI commands for the tools | deferred wiring (D-91 pattern) |

## 3. Package plan

| Package | Owns | Deliberately not there |
|---|---|---|
| `internal/mcp` (+file) | 5 tool handlers (typed In/Out), changes.Service construction, git runner, session attribution | transport serving, CLI |
| everything else | untouched (consumed as services/stores) | — |

No migration. The server gains `ModeWrite`, a changes service (index
store + registry + indexer + snapshot over the app DB), and five
registrations (6→11).

## 4. Entity model

**Tool I/O.** Typed structs; deferred-free in 0.1 (every field supported —
no NOT_IMPLEMENTED in these five tools' happy paths; refusals are usage
and row errors, not version gaps).

- `claim{task_id, session, expected_revision?}` →
  `{task_id, state, revision, replayed?}`. Revision absent = unguarded
  transition; present = `TransitionExpecting` (conflict on mismatch).
- `before_change{task_id, paths[], operation_id?}` → `{task_id,
  scope_files, baseline}` (summary counts + scope list).
- `after_change{task_id, operation_id?}` → `{change_id, files, symbols?,
  findings[] (code+key+next_action), pending}`. Symbols included as
  counts + keys (not full rows — bounded answers).
- `reconcile{task_id, operation_id?}` → `{change_id, discovered[],
  findings[], pending}` — same shape as after_change (one discovery
  answer, two provenances).
- `checkpoint{task_id, session, note?, handoff?}` → write returns
  `{checkpoint_id?, task_id, note, handoff}`; read returns
  `{task_id, note, handoff, readable_by}` — handover readability named.

**Replayed flags.** Every idempotent write echoes `replayed: true` when
the operations table answered (coordination `Write.Replayed`, change
op-replay). Tests pin true on retry, false/absent on first delivery...
— `replayed` always present (false on first write): uniform shape, no
absent-vs-false ambiguity.

**Revision.** Task revision surfaces where the store tracks it
(`Task.Revision`? — verify at implementation; if untracked, the conflict
test drives `TransitionExpecting` directly and the tool echoes the
post-write revision from a re-read).

## 5. Runtime schema

None. Coordination/change tables already exist.

## 6. Tool flows

**claim:** `FindTask` (unknown → refuse) → `Transition` or
`TransitionExpecting` to CLAIMED under `NamedSession` (unknown session →
store refuses) → return task + revision + replayed.

**before_change:** `CaptureBaseline(task, paths, op)` → summary. No claim
required (store requires the task to exist, not to be claimed).

**after_change:** `AfterChange(task, op)` → `EvaluateTask(task, nil)` →
change + findings (code, key, next_action each) + pending flag
(pending = any blocking finding).

**reconcile:** `Reconcile(project, root, task, op, execRunner)` →
`EvaluateTask` → same shape as after_change with discovered file list.
Project id: server derives from coordination (workspace project? — the
changes service needs projectID for indexing scope; derive from the
task's project via FindTask).

**checkpoint:** note present → `WriteCheckpoint(task, session,
workspace, note, handoff)`; absent → `Handover(task)` (cross-session
readable struct) with `LastCheckpoint` fallback... — Handover vs
LastCheckpoint: use Handover (the cross-session shape AC-3 names);
empty (no checkpoint yet) → usage refusal naming the absence, never an
empty success.

## 7. Session attribution

Every write names its session handle; `NamedSession` refuses unknown
handles before any write (stores' rule, preserved). Tests open two
sessions and cross-read.

## 8. Readiness, CLI and performance integration

None beyond D-195. No wall-clock assertions. Reconcile/indexing cost is
the services' own; tools add no traversal.

## 9. Outputs reserved for later milestones

- Eleven-tool server — MR-016's validation/completion tools extend it.
- Checkpoint handoffs — MR-017's CI reads them across worktrees.
- Operation ids — MR-017's retry protocol reuses them.

## 10. Test plan

Per tech-stack §90's categories, per task:

1. **Claim/before/after** (TASK-01): claim + revision conflict +
   unknown refusals; baseline summary + claimless declare; after_change
   change + findings + replay-true; malformed refusals value-free.
2. **Reconcile/checkpoint/lifecycle** (TASK-02): undeclared reconcile
   discovery + replay; cross-session checkpoint read; full two-identity
   lifecycle; eleven-tool registry; no-new-codes.
3. **Proof** (TASK-03): verify + tidy, non-goal greps, mutation ledger,
   Durum.
4. **Mutation ledger**: every guard gets its mutation run before it is
   written down (unchanged rule).
