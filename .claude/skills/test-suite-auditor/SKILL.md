---
name: test-suite-auditor
description: Performs an evidence-driven audit of an entire test suite to find and safely eliminate redundant, duplicated, brittle, flaky, invalid or low-value tests - building a behavior map first, clustering overlapping tests, measuring coverage and mutation impact where tooling allows, gating every deletion behind safety checks and incremental batch validation, and consolidating survivors into parameterized forms. Use this skill whenever the user wants the test suite itself examined or slimmed down - complaints like "too many tests", "tests are slow", "CI takes forever", "half these tests are copy-paste", "are these tests actually useful", "clean up / deduplicate / optimize the tests", or questions about test quality, test debt, flaky tests, or whether coverage numbers are real. Do NOT use it to verify whether an implementation matches its task (use dual-agent-task-audit) or to write new tests for a feature.
---

# Test Suite Audit & Redundancy Elimination

Act as a senior test architect and regression-risk analyst. The objective is not
fewer tests. It is:

> The smallest suite that still provides strong, evidence-backed protection for
> every meaningful behavior and risk — with similar or better defect detection,
> less maintenance, less runtime, less noise.

Deleting tests destroys information that is expensive to recreate. Every deletion
therefore carries a burden of proof; keeping a test never does.

## Core stance

A test earns its place by protecting at least one **unique, meaningful** aspect of
the system: a requirement, a contract, a regression, an edge case, an error path, a
security boundary, a state transition, an integration.

A test is a deletion candidate only when removing it demonstrably does not reduce
requirement coverage, behavior coverage, branch/path coverage, mutation detection,
regression protection, or integration confidence.

Never delete because: another test looks similar, the test is short, it passes,
coverage stays high, or "there are too many tests". Code coverage measures
execution, not protection — 100% lines with weak assertions is a suite that runs
everything and verifies nothing.

## Workflow

| Step | What happens | Reference |
| --- | --- | --- |
| 0–1 | Inventory the suite; build the behavior map | `references/analysis.md` |
| 2–3 | Classify every test; cluster overlapping ones | `references/analysis.md` |
| 4–7 | Detect implementation-detail, framework, parametric-explosion and layer-overlap patterns | `references/analysis.md` |
| 8–13 | Evidence: coverage delta, mutation impact, assertion strength, mock quality, git history, risk weighting | `references/evidence.md` |
| 14–17 | Deletion confidence, safety gate, batched removal, consolidation | `references/deletion-gates.md` |
| — | Metrics and report | `references/report-template.md` |

Read `references/analysis.md` before starting; `references/evidence.md` when
evaluating candidates; `references/deletion-gates.md` before deleting anything.

## Getting started

1. Run the inventory helper:

   ```bash
   python scripts/test_inventory.py <repo> --output /tmp/test-inventory.json
   ```

   It discovers test files and individual tests across common frameworks (pytest,
   unittest, Jest/Vitest, Go, JUnit, RSpec), counts them per area, flags
   assertion-free tests, and — most usefully — hashes normalized test bodies to find
   **verbatim copy-paste duplicates**. A body-hash match is a strong lead; a name
   similarity is not a lead at all and the script deliberately doesn't report it.

   Leads, not findings: every candidate the script surfaces still goes through the
   evidence phases.

2. Establish what measurement tooling exists — coverage, mutation testing, test
   timing — before promising evidence that depends on it. Where a signal is
   unavailable, say `NOT_MEASURED`; never invent a number.

3. Confirm scope in one short message if genuinely unclear: whole suite or a module,
   and whether the user wants deletions applied or a report with candidates. If
   they've said, don't re-ask.

## Scoping the expensive measurements

Per-candidate coverage and mutation reruns are O(candidates × suite runtime) and can
take hours on a mid-size repository. Scale them deliberately:

- Measure **per cluster**, not per test: remove the whole candidate group, measure
  once, restore. If the cluster's removal has zero delta, individual members don't
  need separate runs.
- Mutation evidence is **mandatory only for CRITICAL/HIGH risk areas** (auth, money,
  data integrity, destructive operations, migrations, concurrency). Elsewhere,
  coverage delta + behavior-map reasoning + regression history may suffice at HIGH
  confidence, never VERY_HIGH.
- If neither coverage nor mutation tooling exists, the manual break test
  (`references/deletion-gates.md`) is the substitute: intentionally break the
  protected behavior in a scratch copy and confirm the *remaining* suite goes red.

## Deletion is never automatic

Even VERY_HIGH confidence candidates are removed only through the batch process:
independent clusters, one batch at a time, full validation after each, restore on any
confidence decrease. "Automatic deletion permitted" means *permitted to enter a
batch without human review* — it never means skipping the batch validation itself.

## Flaky tests

A test that fails nondeterministically is its own category (`FLAKY`), distinct from
brittle. A flaky test is worse than no test: it trains people to re-run CI until
green, which masks real failures across the whole suite. Detection: rerun suspicious
tests N times in isolation, then in randomized order. Remedy order: fix the
nondeterminism (time, ordering, shared state, network) → quarantine with a tracking
note → delete only if it protects nothing unique. Never silently delete a flaky test
that is the only cover for its behavior — that converts "unreliable signal" into "no
signal" and calls it cleanup.

## Output

Use `references/report-template.md`. Write the report to a Markdown file, in the
user's language, code and identifiers left as-is. The Evidence Report section is
mandatory for every proposed deletion — a DELETE row without its evidence section is
not a recommendation, it's an opinion.

## Completion bar

Do not declare the audit complete while any applied batch lacks its post-validation,
or any DELETE recommendation lacks its evidence section. The final verdict must
answer: did requirement coverage decrease, did any critical behavior lose its only
test, did mutation/branch protection materially drop, did runtime improve, is the
suite easier to maintain — and conclude `HEALTHIER`, `UNCHANGED`, `DEGRADED` or
`INCONCLUSIVE`. If it's `DEGRADED`, the restores happen before the report ships.
