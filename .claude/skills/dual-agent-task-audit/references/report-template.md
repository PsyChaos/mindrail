# Consolidated Report Template

One report. Findings are measurements, not assertions.

---

# Task Verification Report

## 1. Original Task

The task verbatim. State the audit mode (parallel subagents / sequential passes) and,
for sequential, that independence was partial.

State which of the six Breaker moves ran, and for any that didn't, why. A report that
omits this reads as if the audit were complete when it wasn't.

## 2. Requirement Matrix

| ID | Requirement | Reader Verdict | Evidence | Final |
| --- | --- | --- | --- | --- |

Include `TASK_AMBIGUITY` entries and how each reading was handled.

## 3. Confirmed Findings

| ID | Severity | Requirement | File/Line | Finding | Origin | Confidence |
| --- | --- | --- | --- | --- | --- | --- |

Origin is `Reader` or `Breaker`. Single-agent origin is not a weakness and is not
flagged as one.

Then per finding:

### FINDING-[ID]

**Severity:** · **Confidence:** · **Requirement:** · **File:** · **Line(s):** · **Symbol:**

**What was attempted** — the exact commands, inputs and sequence, reproducible by the
reader.

**Control arm** — the untouched case and its result.

**Scenario arm** — the same data with one difference, and its result.

**Numbers**

| Measure | Control | Scenario | Delta |
| --- | --- | --- | --- |

**Expected behavior** — what the task or the contract requires.

**Actual behavior** — what was produced.

**Class sweep** — where else this shape occurs, and the search used. Record "no other
occurrences" explicitly when that's the answer.

**Refutation attempt** — what the other agent tried in order to disprove this, and why
it failed.

**Proposed fix** — the remedy.

**Fix measurement** — applied in the scratch environment; both arms re-run; the
resulting numbers and any regression. If the fix was not measured, say so and label it
a suggestion.

## 4. Refuted Findings

| Finding | Proposed By | Refuting Evidence |
| --- | --- | --- |

Mandatory. Refuting evidence is a produced result, never "could not reproduce". If
nothing was refuted, say so explicitly.

## 5. Open Items

| Item | Why not produced | Exact experiment that would settle it |
| --- | --- | --- |

The third column is an instruction someone can execute: environment, data, command,
and the output that would discriminate between the two outcomes. No lead lists, no
"investigate further".

## 6. Off-Spec Headings

| Heading | Result | Evidence |
| --- | --- | --- |
| Cost / worst case | | |
| Mechanical rules without automated checks | | |
| Backward compatibility | | |

Every row filled, including "checked, nothing found". A blank row reads as a skipped
check.

For cost, show the multiplication, not the conclusion.

## 7. Scope Compliance

Missing requirements, unnecessary implementation, scope creep, behavioral drift,
unrelated modifications. State explicitly whether the implementation matches the
original task exactly (*birebir*).

## 8. Test & Verification Assessment

Tests executed with exact commands, passed, failed. Mutation results: guards killed vs
**survived** — survivors are findings, listed as such.

Requirement coverage, distinguished from code coverage. Full code coverage does not
imply full requirement coverage; conflating them reports untested requirements as
verified.

## 9. Final Consensus

### Reader — `APPROVE` / `REJECT`
Reason:

### Breaker — `APPROVE` / `REJECT`
Reason:

Note the audit mode again here if separation was simulated, so the agreement is not
read as stronger than it is.

---

# FINAL VERDICT

Exactly one of `VERIFIED`, `VERIFIED_WITH_MINOR_ISSUES`, `NOT_VERIFIED`,
`INCONCLUSIVE`, justified against the criteria in `reconciliation.md`.
