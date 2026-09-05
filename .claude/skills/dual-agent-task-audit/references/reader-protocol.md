# Reader Protocol — Conformance

The Reader answers: *was the requested thing done, was more than requested done, and
do the code, contracts and documentation agree?*

It works outward from the task text. It does not try to break anything — that is the
Breaker's job, and duplicating it wastes the asymmetry.

---

## R1 — Task normalization

Convert the original task into explicit requirements **before** reading the
implementation. Once you've seen what was built, your sense of what was asked for is
contaminated by it.

| ID | Requirement | Source (quote from task) | Type | Verification Method |
| --- | --- | --- | --- | --- |

Types: `FUNCTIONAL`, `BEHAVIORAL`, `TECHNICAL`, `CONSTRAINT`, `ACCEPTANCE`,
`NON_REGRESSION`.

Every requirement traces to specific words in the task. Do not invent requirements —
grading an implementation against standards the task never set produces findings the
user is right to ignore.

Where the task genuinely admits more than one reading, mark `TASK_AMBIGUITY`, record
both readings, and check the implementation against both. Silently picking one is how
an audit ends up confidently wrong.

The orchestrator checks this table's traceability before the audit proceeds. A
misread task poisons everything downstream, and the Breaker won't catch it because
the Breaker doesn't work from requirements.

---

## R2 — Requirement verdicts

Every `REQ-*` gets `PASS`, `PARTIAL`, `FAIL` or `NOT_VERIFIABLE`.

| Requirement | Verdict | Evidence | Notes |
| --- | --- | --- | --- |

`PASS` requires positive evidence of compliance. The absence of an obvious bug is the
absence of evidence, not evidence — treating the two as equivalent is the most common
way an audit becomes decorative.

- `PASS`: what demonstrates the requirement is met? Prefer an executed test or an
  observed behavior over a reading of the code.
- `PARTIAL` / `FAIL`: exactly what differs from required behavior.
- `NOT_VERIFIABLE`: what evidence is missing, and what would supply it.

---

## R3 — Scope compliance

Scope is a first-class dimension. An implementation that does more than asked is not
bonus value; it is unrequested, unreviewed behavior in production.

- `UNDER_IMPLEMENTATION` — required work missing.
- `OVER_IMPLEMENTATION` — behavior introduced that the task didn't request.
- `SCOPE_CREEP` — unrelated code or architecture changed unnecessarily.
- `BEHAVIORAL_DRIFT` — task technically addressed, but existing behavior changed
  beyond what was required.
- `REQUIREMENT_MISINTERPRETATION` — a different interpretation was solved than the one
  stated.

---

## R4 — Contract and documentation conformance

Where the change touches an interface, compare the implementation against whatever
binds it: schema files, OpenAPI/GraphQL definitions, type definitions, validation
layers, and the documentation that describes the behavior to its consumers.

Report disagreement in both directions:

- Implementation violates a current, explicit specification → `SPEC_IMPLEMENTATION_CONFLICT`.
- Documentation describes behavior the implementation no longer has → `DOC_DRIFT`.

Do not resolve these by rewriting the spec to match the code. Which side is
authoritative is a judgment that belongs in the report, with its reasoning visible.

---

## R5 — Evidence standard

Every Reader finding carries: exact file path, symbol where applicable, line or range,
the relevant code, an execution/data-flow explanation, expected behavior, actual
behavior, deterministic reasoning or a reproduction, and the affected `REQ-*`.

Nothing is reported because code "looks suspicious", something "might" fail, or
another architecture would be cleaner.

Where a Reader finding can be turned into a produced failure, hand it to the Breaker
rather than reporting it as a reading. A finding backed by an executed failure
survives refutation; a finding backed by an interpretation usually doesn't — and
shouldn't.

---

## R6 — Severity and confidence

**🔴 CRITICAL** — security exposure, unrecoverable data loss, total failure,
authn/authz bypass, corruption of critical state, or fundamental violation of the
task's primary objective.

**🟠 HIGH** — a major requirement incorrect or missing, an important workflow broken, a
significant regression, a failed integration, or an implementation that cannot
reliably do what was asked.

**🟡 MEDIUM** — secondary behavior incorrect, an important edge case broken, limited
incompleteness, or a maintainability problem with a concrete correctness risk.

**🟢 LOW** — minor mismatch, low-impact edge behavior, small verifiable inconsistency.

Confidence: `CONFIRMED` (direct evidence proves it), `HIGH`, `MEDIUM`, `LOW`.

Do not inflate severity. Style preferences are not bugs; architectural preferences are
not findings unless they cause a concrete, task-related problem. Inflated severity is
the fastest way to get a real CRITICAL skimmed past.

---

## R7 — Reader report

Written before seeing the Breaker's conclusions.

- Requirement table with verdicts and evidence
- Findings table: ID, severity, requirement, file, symbol, problem, evidence, confidence
- Scope compliance
- Contract/documentation conformance
- Open items: anything not produced this round, each with **exactly what is required
  to produce it**. No lead lists.
- Independent conclusion: one of `FULLY_COMPLIANT`, `COMPLIANT_WITH_MINOR_ISSUES`,
  `PARTIALLY_COMPLIANT`, `NON_COMPLIANT`, `NOT_VERIFIABLE`.
