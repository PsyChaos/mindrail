# Evidence (Phases 8–13)

Contents:
- [Phase 8 — Coverage contribution](#phase-8--coverage-contribution)
- [Phase 9 — Mutation impact](#phase-9--mutation-impact)
- [Phase 10 — Assertion strength](#phase-10--assertion-strength)
- [Phase 11 — Mock quality](#phase-11--mock-quality)
- [Phase 12 — Git history and regression value](#phase-12--git-history-and-regression-value)
- [Phase 13 — Risk weighting](#phase-13--risk-weighting)

---

## Phase 8 — Coverage contribution

Where coverage tooling exists, measure line/branch/function coverage with and
without each deletion candidate — **per cluster**, per the scoping rule in SKILL.md,
not per test.

| Cluster / Test | Line Δ | Branch Δ | Function Δ |
| --- | --- | --- | --- |

A zero delta is evidence of redundancy, never sufficient proof by itself: coverage
shows execution, not assertion quality. A test can uniquely execute a branch while
asserting nothing about it — deleting it loses nothing real; keeping a duplicate
that actually asserts the branch loses nothing either. The delta narrows the
question; it doesn't answer it.

---

## Phase 9 — Mutation impact

The strongest available evidence, mandatory for CRITICAL/HIGH risk areas.

**With mutation tooling** (mutmut, Stryker, PIT, cargo-mutants…): record the
baseline score; disable the candidate cluster; re-run on the relevant modules;
compare killed/surviving mutants.

| Cluster / Test | Baseline Score | After Removal | Δ | New Survivors |
| --- | --- | --- | --- | --- |

Removal that produces no new surviving relevant mutants, no coverage loss and no
requirement loss is strong redundancy evidence.

**Without tooling**, run the manual equivalent on a scratch copy: introduce a small
set of hand-picked mutations into the protected behavior (invert the condition,
off-by-one the boundary, drop the error branch) and record which tests go red. If
the candidate is the *only* red, it is not redundant. If the remaining suite goes
red without it, that is the produced evidence. Never report a mutation "score" that
no tool computed — state `NOT_MEASURED` and describe what the manual runs showed.

Scope either form to the modules the candidate tests touch. Whole-repo mutation runs
on every candidate is how an audit takes a week and gets abandoned.

---

## Phase 10 — Assertion strength

Inspect what tests actually verify. Detect: no assertions, always-true assertions,
assertions against mocked return values, assertions unrelated to the test's name,
tests that only verify "no exception was thrown".

The governing question:

> Would this test fail if the feature were meaningfully broken?

If no, the test is `INVALID` or `LOW_VALUE` regardless of what it covers. Where
practical, answer empirically rather than by reading: break the feature in a scratch
copy and watch whether the test notices.

---

## Phase 11 — Mock quality

Find mocking that manufactures confidence: mocking the unit under test itself,
mocking every collaborator, re-implementing production logic inside mock setup,
asserting only that mocks were called, fakes whose behavior has drifted from the
real dependency.

A test that proves its own mocks behave as configured protects nothing. Distinguish
that from legitimate isolation: mocking a payment gateway at a boundary is design;
mocking the function the test claims to verify is theater.

---

## Phase 12 — Git history and regression value

Before any deletion, check the candidate's origin:

```bash
git log --follow --oneline -- path/to/test_file.py
git log -S "test_function_name" --oneline
```

A test added in the same commit as a bug fix, after a production incident, or
referencing an issue number is a **regression test**, and its apparent redundancy is
usually the point — it pins the exact input that once broke. These require strictly
stronger evidence to delete, and the odd-looking magic values inside them are load-
bearing.

The commit message and linked issue often state the protected behavior more
precisely than the test name does. Read them before classifying.

---

## Phase 13 — Risk weighting

Weight deletion evidence by the risk of the protected behavior:

- **CRITICAL** — authentication, authorization, money/billing, destructive
  operations, data integrity, security, migrations, concurrent state, critical
  external integrations. Redundancy here is a feature: keep deliberate overlap, and
  demand mutation-grade evidence for any deletion.
- **HIGH** — core business logic.
- **MEDIUM** — important supporting behavior.
- **LOW** — presentation and simple helpers.

The same coverage-zero, duplicate-looking test is a reasonable deletion in a LOW
area and a `MANUAL_REVIEW` in a CRITICAL one. Asymmetry is intentional: the cost of
a lost critical protection is unbounded; the cost of one redundant test is a few
milliseconds of CI.
