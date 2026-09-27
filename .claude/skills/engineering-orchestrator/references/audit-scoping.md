# Audit scoping — making audit cost follow risk, not task count

Contents:
- [The problem this solves](#the-problem-this-solves)
- [Rule 1 — One audit per delivery, never one per task](#rule-1--one-audit-per-delivery-never-one-per-task)
- [Rule 2 — Depth is set per cluster by risk](#rule-2--depth-is-set-per-cluster-by-risk)
- [Rule 3 — Mutation runs targeted tests, the full suite runs once](#rule-3--mutation-runs-targeted-tests-the-full-suite-runs-once)
- [Rule 4 — Checkpoint audits hide audit time behind implementation time](#rule-4--checkpoint-audits-hide-audit-time-behind-implementation-time)
- [Rule 5 — Re-audits are delta-only](#rule-5--re-audits-are-delta-only)
- [The audit plan artifact](#the-audit-plan-artifact)
- [Worked example](#worked-example)

---

## The problem this solves

The dual-agent audit is the most expensive stage of the lifecycle, and the Breaker's
first move — neutralize each new guard, run the tests, watch them go red — is the
most expensive part of it: its cost is *guards × suite runtime*. On a Tier 3 delivery
with eight tasks and forty new conditionals, an unscoped audit over the whole
changeset with the full suite is hours of wall-clock, and the remediation loop can
run it three times.

Left unscoped, the audit grows with the *number of sub-tasks*. The rules below make
it grow with the *amount of risk* instead, which is the thing the audit is actually
there to reduce. None of them weaken the gate: every delivery still ends behind one
independent audit with produced evidence. They decide how much of the Breaker's
arsenal each part of the change earns, and they stop paying twice for the same
verification.

---

## Rule 1 — One audit per delivery, never one per task

`dual-agent-task-audit` is invoked **once**, at Phase 19, over the integrated result.
It is not invoked when a `TASK-*` finishes. Phase 14 (continuous task validation)
is the orchestrator checking an agent's self-report — expected files changed, tests
named in the brief pass, no scope creep — and it costs one diff read and one
targeted test run. It is deliberately not the audit, because auditing eight tasks
separately means eight Reader passes over shared context, eight mutation harness
setups, and then a ninth audit at integration anyway.

The audit skill's own description triggers on phrases like "verify this properly",
which is exactly what a conscientious orchestrator says to itself at task boundaries.
Resist it. Write "verified per Phase 14 (not audited)" next to each accepted task so
the distinction is on the record.

The two exceptions are Rule 4 (checkpoint audits, which are *parts* of the one
audit run early) and a task that on its own would have been Tier-escalated — an auth
change in the middle of a larger feature — where a checkpoint audit of that task is
worth starting the moment it lands rather than waiting for the rest.

---

## Rule 2 — Depth is set per cluster by risk

Group the changeset into clusters (by `TASK-*` at Tier 2/3, by module at Tier 1)
and assign each a depth. Depth is a **floor**: the auditors may raise it on evidence
they produce; the orchestrator may not lower it below the classification without
writing the reason into the audit plan.

| Depth | Name | Reader | Breaker | When |
| --- | --- | --- | --- | --- |
| **A** | Full | yes | all six moves + off-spec headings (cost, mechanical-rule test, class sweep, backward compatibility) | Any tier escalator: schema/migrations, authn/authz, public API contract, backward compatibility, destructive ops, secrets, CI/deploy, config defaults, money, guards/validators/budgets. Also any cluster the Reader flags as contract-bearing. |
| **B** | Targeted | yes | move 1 (mutation, targeted tests) + move 5 (weakest satisfying input) + class sweep; moves 2/3/4 only where their trigger is present (behavior removed / threat model described / storage or upgrade path touched) | Ordinary business logic, new branches, changed tests, dependency manifest changes. The default. |
| **C** | Conformance | yes | none | Docs, comments, renames with no semantic change, assets. The failure mode is "wrong words", and the Reader's doc↔code contradiction check already covers it. |

`scripts/audit_scope.py` does a first pass of this classification from path and diff
content heuristics and prints the plan table. Treat its output as a draft to
correct, not a verdict: it upgrades on keywords it recognizes and cannot see that a
`utils.py` change silently redefines a contract. Reading the diff is still your job;
the script exists so the risky files are never *missed*, not so they are never
*thought about*.

Why per-cluster and not per-delivery: the alternative is that one auth line drags
forty documentation and test files into six-move treatment, or — worse, and more
common under time pressure — that forty innocuous files argue the whole delivery
down to a light pass and the auth line goes with it. Scoping per cluster lets the
risky part get everything and the rest get what it needs.

---

## Rule 3 — Mutation runs targeted tests, the full suite runs once

The Phase 15 test map already says which tests verify which `REQ-*`, and
`scripts/impacted_tests.py` derives the narrowest command for each cluster from the
diff. Carry that into the audit package as a per-cluster `test_cmd`. The Breaker neutralizes a guard, runs
*that cluster's* tests, and expects red; it does not run the whole suite per guard.
The full suite runs exactly once, at the end, via `validate.py --label post-audit`,
which catches the cross-cluster breakage the targeted runs can't.

This is where most of the time goes, so it's worth being explicit: forty guards ×
a two-minute suite is eighty minutes; forty guards × a ten-second targeted run is
seven. The full suite once at the end costs two more minutes. The evidence is the
same — a guard that survives its own cluster's tests is still a guard with no test.

If the repo has no usable test map (no per-module test layout, one monolithic
suite, selector confidence low across the board), say so in the plan, give `audit_scope.py` the suite runtime with
`--test-seconds`, and if the projected mutation time exceeds ~30 minutes, mutate
depth-A guards exhaustively and sample depth-B guards by risk — and write the
sampling into the report so the unmutated guards are visible as unverified.

---

## Rule 4 — Checkpoint audits hide audit time behind implementation time

(Tier 3 with subagents.) When an independent workstream is accepted at Phase 14
while other workstreams are still implementing, launch its audit **then**, as a
checkpoint, in parallel with the remaining implementation. The checkpoint audit
covers that cluster at its assigned depth against the ref it was accepted at.

The final Phase 19 audit then covers only:

- clusters that landed after the last checkpoint,
- files touched *again* since their checkpoint (the checkpoint's verdict on them is
  void — say so),
- the integration seams from Phase 16, which no checkpoint could see.

This doesn't reduce total audit work; it reduces wall-clock, which is what the user
waits for. The cost is that a checkpointed cluster can be invalidated by later
changes, so the orchestrator must track the audited ref per cluster (the audit plan
holds it) and re-scope honestly rather than assuming an early verdict still holds.

Without subagents there is no parallelism to gain, so don't checkpoint for time.
Do split a very large Tier 3 changeset (roughly: more files than fit in one careful
read) into sequential per-workstream audits followed by a short integration audit —
not for speed, but because one pass over a huge diff degrades into skimming, and
skimming is the failure mode the audit exists to prevent.

---

## Rule 5 — Re-audits are delta-only

After a remediation round, the re-audit is scoped with
`audit_scope.py --base <last-audited-ref> --delta`. It covers:

- the `FIX-*` changes and their callers (a fix in a guard reaches every call site),
- the `REQ-*` those fixes map to, re-verified end to end,
- the class-sweep siblings named in the original finding ("is there another place
  with the same shape?" — the fix has to have covered them),
- anything the previous round left as "not produced + what would produce it", now
  produced.

It does **not** re-run mutation over guards the fix didn't touch, re-read clusters
whose files and callers are unchanged, or reopen findings the previous round refuted
with evidence. A requirement verified in round 1 stays verified unless something in
its verification path changed — and "something changed" is decided from the diff,
not from a feeling that it's safer to redo everything.

The full suite still runs once per round *if the round changed code* — that is the
global check the impacted-test selector cannot replace, and it runs at the end of
the round, not per fix. The mutation harness is the expensive, local one, and only
the local one is delta-scoped.

---

## The audit plan artifact

Write the plan to a file before Phase 19 and include it in the audit package. It is
also the record the final report cites when it says what depth each part received.

```markdown
# Audit plan — <task>
Base ref: <sha>   Suite runtime: <s>   Mode: subagents | sequential

| Cluster | Files | Depth | Breaker moves | test_cmd | Audited at (ref) | Status |
| TASK-001 auth guard | src/auth/... | A | 1-6 + off-spec | pytest tests/auth -q | - | pending |
| TASK-002 exporter   | src/export/... | B | 1, 5, sweep; 4 (writes rows) | pytest tests/export -q | a1b2c3d | checkpoint VERIFIED |
| TASK-003 docs       | docs/... | C | Reader only | - | - | pending |
| integration seams   | src/api/routes.py ↔ src/export/ | A | 1, 5, 6 (dual readers) | full suite | - | pending |

Downgrades from classification (with reason): none
Sampling (if any): none
Projected mutation runs: 23 (~4 min targeted; ~46 min full-suite)
```

The "Downgrades" and "Sampling" lines exist so that scoping decisions are things the
user can see and veto, not things that happened.

---

## Worked example

Tier 3 feature: add CSV export to a reporting service. Eight tasks: a new export
module, a new API route, an auth check on that route, a background job, a migration
adding an `exports` table, tests for each, docs, and a CLI flag.

Unscoped: one audit over ~30 files, ~45 new guards, 3-minute suite → mutation alone
≈ 2¼ hours, then the same again for each remediation round.

Scoped:

- Migration, route, and auth check → **A**. Migration also triggers move 4 (upgrade
  path: old rows, old config). Auth check gets move 3 if the task text describes
  what it protects against.
- Export module, background job, CLI flag → **B**, each with its own `test_cmd`.
  Export module triggers move 4 (it writes rows) — that's one extra experiment, not
  a depth change.
- Docs → **C**.
- Integration seam (route → export module → job) → **A**, mostly move 6 (dual
  readers: the route's row count vs the job's row count vs the CLI's on the same
  data).

The migration and auth check landed first and were checkpoint-audited while the
export module was still being written. Final audit: export/job/CLI clusters, the
seam, and the auth file again because the route task modified it after its
checkpoint. Mutation: 45 guards × ~8-second targeted runs ≈ 6 minutes, full suite
once. Round-2 re-audit after two FIX-* items: 2 clusters, 5 guards, callers of both,
full suite once. Same evidence standard, same gate, roughly a tenth of the
wall-clock.
