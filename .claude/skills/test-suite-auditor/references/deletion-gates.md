# Deletion Gates (Phases 14–17)

Contents:
- [Phase 14 — Deletion confidence](#phase-14--deletion-confidence)
- [Phase 15 — Safety gate](#phase-15--safety-gate)
- [Phase 16 — Controlled removal](#phase-16--controlled-removal)
- [Phase 17 — Consolidation](#phase-17--consolidation)

---

## Phase 14 — Deletion confidence

Every candidate gets a rating:

- **`VERY_HIGH`** — exact behavior duplication, no unique branch/path, no unique
  requirement, zero meaningful coverage delta, zero meaningful mutation delta, no
  regression history, remaining tests fully protect the behavior. All seven.
- **`HIGH`** — strong evidence with exactly one secondary signal unavailable (e.g.
  no mutation tooling, but everything else holds and the manual break test passed).
- **`MEDIUM`** — likely redundant, evidence incomplete.
- **`LOW`** — insufficient proof.

Only `VERY_HIGH` and `HIGH` may enter a deletion batch without prior human review.
`MEDIUM` and `LOW` are reported for the user to decide. Entering a batch is not the
same as being deleted — the batch validation still applies to everything.

---

## Phase 15 — Safety gate

Before any test is deleted, demonstrate every applicable condition:

1. No unique requirement is lost.
2. No unique behavior is lost.
3. No meaningful edge case is lost.
4. No important branch/path protection is lost.
5. No important regression protection is lost (Phase 12 checked).
6. Coverage does not materially decrease.
7. Mutation effectiveness does not materially decrease, where measurable.
8. Critical-path confidence does not decrease.
9. **The break test:** where practical, intentionally break the protected behavior
   in a scratch copy and confirm the *remaining* suite goes red without the
   candidate.

Condition 9 is the strongest gate and the substitute for missing tooling. Concrete
procedure: copy the repo to a temp directory, apply a realistic breaking change to
the behavior the candidate protects (invert the condition, remove the guard, corrupt
the transformation), run the suite minus the candidate, require red. Green means the
candidate was the only protection — it stays, and the audit just proved it
`ESSENTIAL`.

Anything not demonstrable → the candidate does not delete automatically, whatever
its confidence rating says.

---

## Phase 16 — Controlled removal

Never delete in one large unverified batch. Batch by independent cluster:

```
Batch A: duplicate unit tests in module X   → validate
Batch B: parameter explosion in module Y    → validate
Batch C: …
```

After each batch run: the tests of the affected modules, the full suite where
practical, coverage, mutation where practical, static checks.

If any signal of regression confidence decreases — new surviving mutants, a coverage
drop in a risk area, a behavior in the map losing its last test — **restore the
batch** and record why. A restore is the process working, not the process failing;
the report gains a proven-`ESSENTIAL` classification either way.

Keep batches reversible: one commit per batch, message listing the deleted tests and
the evidence IDs, so a restore is a revert rather than an archaeology project.

---

## Phase 17 — Consolidation

Where a cluster contains useful variations, prefer consolidation over deletion:
parameterized tests, table-driven tests, shared fixtures, reusable assertions,
helper functions.

Constraints:

- One test should still have one clear reason to fail. A 40-case table whose failure
  message is `case[17] failed` trades maintenance cost for diagnosis cost, which is
  usually a bad trade at 3 a.m.
- Keep boundary values and regression values as named, visible cases — not folded
  into a range that hides why they matter.
- A consolidation is a refactor of the suite and gets the same validation as a
  deletion batch: the consolidated test must still fail for each behavior its
  ancestors failed for. Where practical, prove it with the break test per merged
  behavior.

Consolidation done well is the main way the suite gets *smaller and stronger* at the
same time — deletions alone only make it smaller.
