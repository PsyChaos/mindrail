# Reconciliation — Refutation, Not Agreement

Begin when both reports exist as written artifacts.

The operator is inverted from the usual one. Agreement is not what admits a finding;
failure to refute is.

---

## The admission rule

> A finding produced live by one agent, which the other could not refute, **is a
> finding**.

Originating from a single agent does not weaken it. The Breaker is expected to produce
things the Reader never would — that asymmetry is the design, not a defect in it.

The Reader's job in this round is not to confirm the Breaker's findings. It is to
attempt refutation and report honestly whether it succeeded.

---

## The refutation standard

To remove a proposed finding from the report, produce its absence:

> I ran *this exact thing*, I got *this output*, therefore the claim is false.

These do **not** refute a finding:

- "I couldn't reproduce it." — absence of your reproduction, not absence of the defect.
- "That path probably isn't reachable." — then produce the unreachability.
- "The framework likely handles that." — then show the framework handling it.
- "It's an unlikely scenario." — likelihood is a severity input, not a refutation.

A finding refuted this way moves to Refuted Findings **with the refuting evidence
recorded**. The record is what makes the surviving findings credible; a deleted
finding teaches the reader nothing.

Refutation applies to reasons, not only conclusions. A finding can be right for the
wrong reason — the verdict holds, but its stated premise is false ("this sha doesn't
resolve" when it does, while the document fails for a different cause). Verify the
**reason** independently of the conclusion, and correct it in the report even when
the verdict survives: a correct verdict on a false premise moves next time, in a
case where the premise is all that differs.

---

## No unresolved bucket

There is no "unresolved disagreement" category. It is where real findings go to die —
filed as a difference of opinion, never acted on, and rediscovered later by someone
who wasn't in the room.

Every proposed finding ends in exactly one of three states:

1. **Confirmed** — produced, and not refuted.
2. **Refuted** — its absence was produced.
3. **Open** — neither could be produced, written with **the exact experiment that
   would settle it**: the environment, the data, the command, the expected
   discriminating output.

State 3 is an instruction to whoever picks it up, not a shrug. If it can't be written
as a concrete experiment, it wasn't a finding to begin with.

---

## Severity reconciliation

Where the two agents assign different severities, resolve on consequence, not on
seniority of the agent. Ask what happens to someone who trusts the system: the size of
the textual difference is irrelevant.

If a Breaker finding is real but its severity depends on reachability, reachability is
an experiment — run it rather than debating it.

---

## Class sweep closure

Before finalizing, confirm every confirmed finding has its class sweep recorded. An
audit that fixes one instance and leaves the siblings alive has moved the defect, not
removed it.

Where a sweep found siblings, they are part of the same finding, not separate ones —
one root cause, one entry, all locations listed.

---

## The fix carries its own proof

A finding being real does not make its proposed remedy correct.

For every recommended fix:

1. Apply it in the scratch environment.
2. Re-run the control arm and the scenario arm.
3. Write the regression: what changed, what didn't, what the numbers are now.

A fix that was never measured is a suggestion. Label it as one, or measure it.

---

## Final compliance decision

Evaluate on the consolidated evidence: completeness (all requirements implemented),
correctness (they behave correctly), scope (only appropriate work performed),
integration (correct interaction with the existing system), regression safety
(unrelated behavior preserved), verification (sufficient produced evidence).

| Dimension | Result |
| --- | --- |
| Functional Requirements | PASS / PARTIAL / FAIL |
| Technical Requirements | PASS / PARTIAL / FAIL |
| Constraints | PASS / PARTIAL / FAIL |
| Edge Cases | PASS / PARTIAL / FAIL |
| Tests | PASS / PARTIAL / FAIL |
| Scope Compliance | PASS / PARTIAL / FAIL |
| Regression Safety | PASS / PARTIAL / FAIL |
| Backward Compatibility | PASS / PARTIAL / FAIL |
| Cost / Worst Case | PASS / PARTIAL / FAIL |

Do not compute a percentage unless the task defines weighted acceptance criteria. A
number invented for the report implies precision the evidence doesn't have.

---

## Final verdict

**`VERIFIED`** — all requirements pass, no confirmed CRITICAL/HIGH findings, no open
items that could be CRITICAL/HIGH if produced, scope matches, all six Breaker moves
executed.

**`VERIFIED_WITH_MINOR_ISSUES`** — core requirements satisfied, only LOW/MEDIUM
confirmed findings remain, and each was checked against a named acceptance criterion.
"Minor" is a judgment that must be visible, not asserted.

**`NOT_VERIFIED`** — a requirement fails, a confirmed CRITICAL/HIGH finding exists, the
implementation materially differs from the task, or a regression was produced.

**`INCONCLUSIVE`** — the environment prevented the Breaker moves from running, or open
items exist that could be CRITICAL/HIGH once produced.

`INCONCLUSIVE` is a real result. It says the audit could not be performed, which is
different from the audit passing — and reporting it honestly is more useful than a
`VERIFIED` the user later discovers was unearned.
