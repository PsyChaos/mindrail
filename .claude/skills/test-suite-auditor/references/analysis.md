# Analysis (Phases 0–7)

Contents:
- [Phase 0 — Suite inventory](#phase-0--suite-inventory)
- [Phase 1 — Behavior map](#phase-1--behavior-map)
- [Phase 2 — Test classification](#phase-2--test-classification)
- [Phase 3 — Duplication clustering](#phase-3--duplication-clustering)
- [Phase 4 — Implementation-detail tests](#phase-4--implementation-detail-tests)
- [Phase 5 — Framework/library tests](#phase-5--frameworklibrary-tests)
- [Phase 6 — Parametric explosion](#phase-6--parametric-explosion)
- [Phase 7 — Layer overlap](#phase-7--layer-overlap)

---

## Phase 0 — Suite inventory

Scan the entire repository: unit, integration, E2E, contract, smoke, regression,
performance, security, property-based and snapshot tests, plus fixtures, factories,
utilities and test configuration. Also record the frameworks, coverage tooling,
mutation tooling, CI test commands, parallelization strategy and total runtime.

| Test Area | Type | Count | Main Purpose | Approx Runtime |
| --- | --- | --- | --- | --- |

`scripts/test_inventory.py` produces the mechanical part. Snapshot tests deserve a
line of their own in the inventory: large auto-generated snapshots that get
re-recorded on every failure are coverage theater — they execute much and assert
nothing a human ever reads.

---

## Phase 1 — Behavior map

Before judging any individual test, map what the system must do: functional
requirements, business rules, public APIs, state transitions, critical workflows,
failure scenarios, edge cases, security-sensitive paths, integration boundaries.

| Behavior ID | Behavior | Risk | Relevant Tests |
| --- | --- | --- | --- |

IDs: `BEH-001`, … The map is what makes "unique protection" a checkable claim
instead of a feeling. A test's value is its coverage of this map, not its resemblance
to other tests — and a behavior with **zero** tests found during mapping is a finding
in the other direction, worth reporting even though nobody asked.

---

## Phase 2 — Test classification

Classify every meaningful test:

- **`ESSENTIAL`** — protects unique, important behavior; removal reduces confidence.
  The only regression test for a past bug, the only test of an authorization
  boundary, the only cover for a dangerous error path.
- **`VALUABLE`** — meaningful protection with partial overlap. Keep unless
  consolidation is clearly better.
- **`REDUNDANT`** — another test or smaller group provides equivalent protection.
  Deletable only if proven through the evidence phases.
- **`LOW_VALUE`** — very little unique value: framework-obvious behavior, trivial
  getters, near-identical variants. Not automatically deleted.
- **`BRITTLE`** — coupled to implementation details: exact internal call sequences,
  over-mocking, DOM-structure assertions with no user-visible behavior. Prefer
  rewrite over deletion.
- **`FLAKY`** — nondeterministic pass/fail. Handled per the flaky-test policy in
  SKILL.md; never counted as protection while flaky, never silently deleted while
  it's the only cover.
- **`INVALID`** — does not verify the intended behavior: no useful assertion, asserts
  only mocked output, impossible scenario, or passes even when the feature is
  broken. These are worse than dead weight — they are false confidence.
- **`MANUAL_REVIEW`** — evidence insufficient.

---

## Phase 3 — Duplication clustering

Group tests that appear to protect the same behavior into `CLUSTER-001`, … For each
cluster compare the tested requirement, inputs, execution path, assertions, branches,
side effects, error conditions, mocks and integration level.

Answer per cluster: do they actually test the same behavior? different branches?
materially different inputs? does one dominate the others? can several become one
parameterized test? does higher-level coverage already make lower-level duplication
unnecessary?

Similar names or similar structure prove nothing — two tests named
`test_login_success` may cover different branches, and two differently named tests
may be verbatim copies. The inventory script's body-hash matches are the reliable
starting point; everything else needs reading.

---

## Phase 4 — Implementation-detail tests

Find tests verifying internals rather than observable behavior: private-method
invocation, internal call order with no contractual weight, exact helper selection,
over-mocked collaborators, internal state no user observes.

For each: is the internal behavior actually part of a contract? does the test block
legitimate refactoring? does a behavioral replacement already exist? Recommend
`REWRITE`, `MERGE`, `KEEP` or `DELETE`.

The trap to avoid: a brittle test that is the **only** protection for important
behavior gets rewritten, not deleted. Deleting it fixes the annoyance and uncovers
the behavior in the same move.

---

## Phase 5 — Framework/library tests

Find tests that primarily verify behavior owned by a framework or third-party
library: the framework's generic 404, the ORM saving an ordinary field, the router
dispatching with no custom logic, standard serialization with no project-specific
transformation.

Classify as low-value **only** when the project adds no behavior on top. If project
configuration changes the framework's behavior, the test is testing that
configuration and stays.

---

## Phase 6 — Parametric explosion

Detect brute-force variation: dozens of tests exercising the same rule with slightly
different values. For each parameter family, ask whether the values represent
distinct **equivalence classes**.

Instead of 1, 2, 3, 4, 5, 6… prefer: below minimum, exactly minimum, normal, exactly
maximum, above maximum.

Keep specific values that encode known regressions or domain rules even when they
look arbitrary — that 37 in the test data may be the exact value that took
production down, and the git history phase is where you find out.

---

## Phase 7 — Layer overlap

Analyze the same behavior tested at unit, integration and E2E levels. E2E coverage
does not make unit tests unnecessary, and vice versa — the question per layer is
what signal it uniquely provides:

- Which layer gives the cheapest reliable signal for this behavior?
- Does the unit test isolate an edge case the E2E can't reach deterministically?
- Does the integration test verify wiring the unit test mocks away?
- Does the E2E protect a real user workflow end to end?

Flag only cases where a layer adds **no meaningful additional assurance** — same
inputs, same assertions, same failure modes, just slower.
