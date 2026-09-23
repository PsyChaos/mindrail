# MR-013 — Design

- **Frozen at:** commit `eb55e21`, before any implementation commit.
- **Refines and is refined by:** [mr-013-requirements.md](mr-013-requirements.md)
  (D-179…D-184 record every place this design left a gap the implementation
  would otherwise fill silently).
- **Specification anchors:** spec-1.0 §89 (minimum checklist), §90
  (uncertainty ladder — deferred to config, not this milestone), §31
  (scores never block); AC-20 area (agent independence: the gate judges
  rows, never who called); kernel-scope §3 (STRUCTURAL) and §4 (deferred
  breadth); tech-stack §7 (layout).

## 1. What the milestone is, in one paragraph

The decision point every earlier milestone fed: after MR-013, completion
is a pure function from discovered change, invariant bindings,
ambiguities, attribution findings, evidence coverage and guard findings to
ALLOW or a list of DENY items — each with a stable reason code,
provenance and a next action, reproducible to the byte.

## 2. What it is not

| Temptation | Ruling |
|---|---|
| Calling reconcile/index/verify inside | pure composer (D-179); drivers sequence |
| Required-policy ownership (which profiles) | caller input via coverage; policy is configuration, later |
| Warning enforcement | warnings are listed, never denying |
| Score/risk aggregation | no scores enter (§31) |
| Overriding a denial | no override path here; resolution paths named per denial |

## 3. Package plan

| Package | Owns | Deliberately not there |
|---|---|---|
| `internal/gate` (new) | `Decision`, `Denial`, `Input`, `Service.Evaluate` (5 families, idempotent) | stores, drivers |
| `internal/app` (+1 code) | `REQUIRED_EVIDENCE_NOT_CURRENT` with remedy | — |
| everything else | untouched (consumed as value types) | — |

No migration. The gate imports `changes`, `validation` and `testguard`
as value types only — no store handles cross the boundary, which is what
makes read-only structural.

## 4. Entity model

**Input.** `{Bindings []InvariantBlock, Ambiguities []Ambiguity,
Attribution []changes.Finding, Coverage validation.Coverage,
Guard []testguard.Finding}`. All plain values; empty means clean.

**InvariantBlock.** `{InvariantID, UID, Status bound|ambiguous|orphaned,
Severity, Active bool}`. Denies iff status ≠ bound AND active AND
severity HIGH/CRITICAL — with the MR-006 code matching the status
(orphaned → ORPHANED_PROTECTED_SYMBOL, ambiguous → ... hmm, MR-006's
binding-ambiguous code is SYMBOL_IDENTITY_AMBIGUOUS? That code marks
removed-meets-candidates at migration. For binding ambiguity the finding
code... — decision: binding-ambiguous denies with SYMBOL_IDENTITY_AMBIGUOUS
(the identity cannot be settled), binding-orphaned with
ORPHANED_PROTECTED_SYMBOL. Both exist; mapping documented + tested.

**Ambiguity.** `{Key string, Candidates []string, InvariantID string,
Severity string, Active bool}`. Denies iff candidates ≥2 AND active AND
severity HIGH/CRITICAL, with SYMBOL_IDENTITY_AMBIGUOUS. Without an active
HIGH/CRITICAL invariant: listed? No — silent (AC-01.5: warn-scope stays
outside). Hmm — "listed, never denying"? Warnings aren't in Decision...
Decision has only denials. Unqualifying inputs contribute nothing. OK:
silent.

**Attribution.** Each `changes.Finding` (already Blocking=true by
construction) denies with its own code + provenance + next_action,
verbatim.

**Coverage.** Every unsatisfied required profile denies with
REQUIRED_EVIDENCE_NOT_CURRENT, reason naming missing-vs-stale (from the
re-run reason: "no current evidence" vs scope-change text), next_action
naming the profile run. Stale rows of NON-required profiles: silent
(coverage only answers required).

**Guard.** Each `testguard.Finding` with Blocking=true denies with
TEST_GUARD_WEAKENED verbatim; warnings silent.

**Decision.** `{Allow bool, Denials []Denial}` sorted by (code, key) for
byte-stability. Allow iff zero denials.

**Denial.** `{Code app.Code, Reason, Key string, Provenance string,
NextAction []string}`. Key = the human handle (invariant id, test key,
profile, path...). Provenance = compact source pointer (which input family
+ identifiers), never full rows.

## 5. Runtime schema

None (D-184). No migration.

## 6. Evaluation flows

**Evaluate(input):** collect denials per family in fixed family order,
sort, Allow = empty. Pure: no I/O, no clock, no randomness —
double-evaluation DeepEqual pins it.

Family order for stability (then sort anyway): invariant, ambiguity,
attribution, evidence, guard.

## 7. Secret inputs

None. Findings carry names and counts, never contents.

## 8. Readiness, CLI and performance integration

None (D-184). Performance: linear in input rows. No wall-clock assertions.

## 9. Outputs reserved for later milestones

- Decisions — MR-014's tool output (completion answers agents read).
- Denial codes — MR-017's CI output (machine-readable verdicts).
- Coverage gaps — MR-015's run lists (what to execute).

## 10. Test plan

Per tech-stack §90's categories, per task:

1. **Core** (TASK-01): per-family denial with code/provenance/remedy;
   ALLOW on clean; idempotency double-run; severity gate (non-CRITICAL
   silent); registry 44→45 + exit class.
2. **Kernel E2E + beats** (TASK-02): no-baseline discover→ambiguate→DENY;
   resolve→ALLOW; every family fires ≥1 (kernel + unit beats); non-goal
   greps (45 codes, pure composer); mutation ledger; Durum.
3. **Mutation ledger**: every guard gets its mutation run before it is
   written down (unchanged rule).
