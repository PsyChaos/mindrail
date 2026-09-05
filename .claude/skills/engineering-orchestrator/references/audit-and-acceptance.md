# Audit & Acceptance (Phases 18–23)

These phases apply at every tier. They are the point of the skill.

---

## Phase 18 — Pre-audit completeness check

Before invoking the audit, confirm: all `TASK-*` complete, all `REQ-*` have
implementation evidence, relevant tests pass, integration finished, documentation
updated where required, no known blocker outstanding.

Then assemble the audit package:

- **Original task** — the user's request, verbatim.
- **Requirements** — all `REQ-*` with types and sources.
- **Acceptance criteria** — all expected behaviors.
- **Implementation summary** — what changed.
- **Change references** — files, commits, diffs.
- **Test evidence** — exact commands and their output.
- **Decision log** — relevant `DEC-*`.

Do not tell the auditors what conclusion to reach, and do not include your own
assessment of quality. An audit package that argues for its own correctness produces
an auditor that grades the argument instead of the code.

---

## Phase 19 — Mandatory dual-agent audit

Invoke the `dual-agent-task-audit` skill with the package above. This step is not
optional and is not skipped because the change is small or the fix looked obvious.

If that skill isn't available, run its protocol inline at minimum: two independent
passes over the implementation that do not see each other's conclusions, each
producing per-requirement verdicts with evidence, followed by cross-review and
evidence-based reconciliation. Findings that survive only because one pass asserted
them do not enter the report.

The auditors verify exact task compliance, every `REQ-*`, implementation correctness,
integration correctness, edge cases, regression safety, scope compliance, test
quality and documentation where relevant.

**Anti-bias rule.** An agent that implemented a task must not audit that task. The
auditors receive the implementation as something to verify, not something to defend.
Where subagents don't exist and the same model does both, say so explicitly in the
final report rather than presenting simulated separation as real.

---

## Phase 20 — Evidence requirement

Every finding carries: requirement ID, file path, line/symbol where available,
expected behavior, actual behavior, deterministic proof or reproduction, severity
(🔴 CRITICAL / 🟠 HIGH / 🟡 MEDIUM / 🟢 LOW) and confidence.

Unsupported speculation does not survive the audit.

---

## Phase 21 — Consensus gate

The audit returns exactly one verdict:

- **`VERIFIED`** — proceed to completion.
- **`VERIFIED_WITH_MINOR_ISSUES`** — review the remaining findings and proceed only
  if they genuinely do not violate any acceptance criterion. Write down which
  criterion each remaining finding was checked against; "minor" is a judgment that
  should be visible, not asserted.
- **`NOT_VERIFIED`** — do not finish. Create remediation tasks.
- **`INCONCLUSIVE`** — do not assume success. Collect the missing evidence or ask the
  user for what's needed.

---

## Phase 22 — Remediation loop

For each confirmed finding create `FIX-001`, … containing: the audit finding, the
affected `REQ-*`, the evidence, the required correction, the regression risk, and the
validation criteria.

Assign each to the appropriate team or agent; parallelize independent fixes. Then:
targeted tests → regression tests → integrate → full validation → re-invoke the
audit.

**Cap the loop at 3 rounds.** If the audit still hasn't reached `VERIFIED` or
`VERIFIED_WITH_MINOR_ISSUES`, stop and report to the user with what remains and what
each round changed. Repeated failure to converge usually means the requirements were
wrong, not the code — and continuing to grind produces churn instead of progress.

Never bypass the audit because a fix looks obvious. Obvious fixes are exactly the
ones applied without checking their blast radius.

---

## Phase 23 — Final acceptance

Complete only when implementation is done, requirements are satisfied, integrations
work, tests pass, no blocking validation failures remain, the dual-agent audit
finished, both auditors reached evidence-based agreement, no unresolved
CRITICAL/HIGH issue remains, and the scope matches the original request.

---

# Final report

## 1. Task
What was requested. State the tier chosen and why, and whether subagents were
available.

## 2. Requirements
Counts: passed / partial / failed, with the table of `REQ-*` and final status.

## 3. Teams
| Team | Responsibility | Tasks | Status |
| --- | --- | --- | --- |

(Omit at Tier 1.)

## 4. Execution
Parallel workstreams, important dependencies, notable coordination between agents.

## 5. Implementation
What changed, with file references.

## 6. Validation
Build, lint, static analysis, unit / integration / regression tests, requirement
coverage, code coverage where applicable — with the exact commands run.

Report only checks that were actually executed. A claimed check that didn't run is a
false statement about the system's state, and it's the kind users discover in
production.

## 7. Dual-agent audit
Agent A verdict, Agent B verdict, consensus verdict, confirmed findings, rejected
false positives, number of remediation rounds. Note if the separation was simulated.

## 8. Remaining issues
Only genuine unresolved issues. If none: `None.`

## 9. Final status

Exactly one of:

- `COMPLETED_AND_VERIFIED`
- `COMPLETED_WITH_NON_BLOCKING_ISSUES`
- `NOT_COMPLETED`
- `BLOCKED`
