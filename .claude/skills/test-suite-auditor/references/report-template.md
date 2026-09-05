# Report Template

Use this exact structure. Metrics that were not measured say `NOT_MEASURED` — never a
fabricated number.

---

# Test Suite Health & Redundancy Audit

## 1. Executive Summary

Current test count, estimated meaningful tests, redundancy level, main causes of
inflation, major quality risks, overall health. Lead with the risks — an INVALID
test in an auth module matters more than five hundred redundant helpers.

## 2. Test Inventory

| Area | Unit | Integration | E2E | Other | Total |
| --- | --- | --- | --- | --- | --- |

Include total runtime and the slowest offenders.

## 3. Behavior Coverage Map

| Behavior ID | Description | Risk | Tests | Coverage Status |
| --- | --- | --- | --- | --- |

Behaviors with zero tests are findings; list them even though nobody asked.

## 4. Redundancy Clusters

| Cluster | Behavior | Tests | Recommended Representatives | Candidates |
| --- | --- | --- | --- | --- |

Per cluster, one paragraph on why the tests overlap — inputs, branches, assertions —
not just that they do.

## 5. Deletion Candidates

| Test | Classification | Risk Area | Confidence | Evidence | Action |
| --- | --- | --- | --- | --- | --- |

Actions: `KEEP`, `DELETE`, `MERGE`, `PARAMETERIZE`, `REWRITE`, `MANUAL_REVIEW`.

## 6. Evidence Report

Mandatory for every `DELETE`. A DELETE without this section is an opinion, not a
recommendation.

### TEST-[ID]

**File:** · **Test:** · **Classification:** · **Confidence:** · **Protected
Behavior:** · **Risk Level:**

**Why it appears redundant** — precisely, with the overlapping tests named.

**Remaining protection** — the tests that continue covering the behavior.

**Requirement impact** — which BEH-* entries are affected; show none is orphaned.

**Branch/path impact** — whether unique execution paths are lost.

**Coverage evidence** — before / after / delta, or `NOT_MEASURED`.

**Mutation evidence** — before / after / delta, or `NOT_MEASURED` plus what the
manual break test showed.

**Regression history** — the git evidence: origin commit, linked fix or incident, or
"no regression association found".

**Final decision** — `DELETE` / `KEEP` / `MANUAL_REVIEW`.

## 7. Brittle / Flaky / Low-Value Tests

Separate from proven-redundant candidates — mixing them lets weak classifications
borrow the strong ones' evidence. For each: rewrite, merge, simplify, quarantine
(flaky), retain, or manual review, with the reason.

## 8. Invalid Tests

Tests providing false confidence. For each: why it is ineffective, how it passes
while production behavior is broken, and the recommended replacement. These are
fix-first items regardless of any deletion work.

## 9. Test Pyramid Analysis

Overlap among unit / integration / E2E; what should remain at each layer and what
signal that layer uniquely provides.

## 10. Cleanup Plan

- **Batch 1 — Proven redundancy** (`VERY_HIGH`)
- **Batch 2 — Strong candidates** (`HIGH`)
- **Batch 3 — Refactoring** (brittle/duplicated tests to consolidate)
- **Batch 4 — Manual review** (`MEDIUM`/`LOW`, flaky quarantine decisions)

Per batch: tests affected, estimated runtime reduction, expected confidence impact,
validation required.

## 11. Before / After Comparison

| Metric | Before | After | Δ |
| --- | --- | --- | --- |
| Tests | | | |
| Runtime | | | |
| Line Coverage | | | |
| Branch Coverage | | | |
| Mutation Score | | | |
| Requirement Coverage | | | |

Fill only what was measured. `After` columns stay empty for batches not yet applied.

## 12. Final Confidence Assessment

Answer explicitly: Did requirement coverage decrease? Did any critical behavior lose
its only test? Did branch coverage materially decrease? Did mutation score
materially decrease? Did regression protection materially decrease? Did runtime
improve? Is the suite easier to maintain?

Verdict: `HEALTHIER` / `UNCHANGED` / `DEGRADED` / `INCONCLUSIVE`.

`DEGRADED` is not a shippable end state — the restores from Phase 16 happen first,
and the report documents them.
