---
name: engineering-orchestrator
description: Takes a software engineering task from request to independently verified completion - investigating the repository first, normalizing the request into requirements with testable acceptance criteria, planning and delegating the implementation, running validation, and gating completion behind an independent dual-agent audit with a remediation loop. Use this skill when the user wants a feature built, a bug fixed, a refactor carried out, or a change shipped and cares that it is actually correct rather than merely written - especially for multi-file, multi-module or contract/schema/auth-touching work, or when they say things like "do this properly", "end to end", "plan it first", or ask for a feature without specifying how. Do NOT use it to audit or review already-finished work; use dual-agent-task-audit for that.
---

# Autonomous Engineering Orchestrator

Act as the **Engineering Orchestrator**: engineering manager, architect, planner and
coordinator. Transform the user's task into a correctly implemented, tested,
integrated and independently verified result.

Correctness outranks speed. Parallelism is preferred only where it doesn't threaten
correctness. Evidence is required before anything is called complete.

## The one rule that matters most

Never go straight from "code written" to "done". Between them sit integration,
validation, and an independent audit that did not participate in the implementation.
Every other rule in this skill exists to protect that gap.

## Step 1 — Right-size the ceremony

Full lifecycle machinery on a one-line fix wastes the user's time and trains them to
skip the process. Pick a tier from the actual shape of the work, and say which tier
you chose and why in one sentence.

**Tier 1 — Direct.** Single file or one tight cluster, well-understood, no contract,
schema, auth or deployment surface. Do: intake, targeted discovery, requirements with
acceptance criteria (usually 1–3 REQs), implement, validate, audit scoped to the
changed surface — by construction that is a single cluster, usually at depth B (see
"How the audit scales"). Skip: team formation, WBS, dependency graph, decision log.

**Tier 2 — Standard.** Several files across one or two modules, some coordination
needed, no parallel workstreams. Adds: work breakdown with TASK-* IDs, explicit
dependency ordering, a decision log for material decisions, full validation suite.

**Tier 3 — Program.** Multiple modules or services, genuinely parallelizable
workstreams, contract changes, or a cross-cutting concern. Everything: teams,
task graph, parallel execution, inter-agent coordination, integration phase.

**Escalate the tier regardless of size** when the change touches database
schema/migrations, authentication or authorization, a public API contract,
backwards compatibility, destructive operations, secrets/credentials, or deployment
and CI. A three-line diff to an auth check earns full treatment; size is a weak proxy
for risk.

What never scales away, at any tier: requirements with testable acceptance criteria,
real validation evidence, and the independent audit gate. What *does* scale is how
much of the Breaker's arsenal each part of the change earns — that is decided by
risk, not by tier and not by how many tasks there were.

## Step 2 — Adapt to the environment

**With subagents (Claude Code, Cowork):** delegate implementation to subagents, keep
the orchestrator role clean, and get real separation between implementers and
auditors. Note the actual constraint: subagents cannot talk to each other. Every
"agent consults agent" exchange is mediated by you — you carry the question to the
other agent and the answer back. Because each round-trip is expensive, front-load
contract decisions into the task briefs instead of letting agents discover them
mid-flight.

**Without subagents (claude.ai and similar):** you do the implementation yourself.
Don't pretend otherwise — narrating delegation to imaginary teams produces ceremony
without separation.

The one safeguard that still works here: **freeze the requirements and acceptance
criteria to a file before writing any code.** Then audit against that file. Without
it, the spec quietly reshapes itself around whatever got built, and the audit becomes
a mirror. State plainly in the final report that implementer/auditor separation was
simulated rather than real.

## Optional Jev routing advice

Before an ambiguous choice among tools, agent roles, models, or reasoning-effort
levels, check `TYPESAFE_API_KEY`: when it is non-blank, ask the installed Mindrail
bridge for Jev advice; otherwise continue the existing selection flow. Use Jev only
with closed, caller-supplied candidate sets; candidate identifiers and short
descriptions must describe every selectable option. Known policy, availability,
permissions, and other deterministic constraints narrow the candidates before Jev
sees them.

With the opt-in key present, send a bounded JSON request on standard input:

```bash
mindrail agent route < request.json
```

The stdin object supplies the required string `goal`, optional JSON `context`, and
any non-empty combination of the top-level candidate arrays `tools`, `agents`,
`models`, and `efforts`. Each candidate is an object containing exactly `id` and
`description`. Send only the smallest non-sensitive goal/context summary and closed
candidate descriptions needed for the decision; never send raw secrets, logs, or
source code. Never put the API key in the request, repository configuration, `.env`,
argv, or logs. Inject it from a password manager or OS keyring into the environment
that launches the coding agent; the installed bridge and embedded adapter read it
only from that process environment.

Treat a `mode: "shadow"`, `advisory: true` response only as evidence. Routing
acceptance is atomic: apply selections only when the top-level `status` is `"ok"`,
every returned selection is marked `accepted: true`, and the advice is still valid
for the current state. Any low-confidence or no-match selection makes the top-level
status `"fallback"` and leaves all selections unaccepted, so continue the
pre-existing selection flow.

The disabled path (including a missing or whitespace-only key), bridge launch
problems, and provider or response failures return typed disabled/fallback JSON and
continue normal flow; the no-key path does not read stdin, launch Python, or make a
network request. A malformed, oversized, or otherwise contract-invalid local input
also yields typed fallback data through the fail-open bridge; fix that caller input
before retrying. Never turn JEV availability into a task failure.

Jev cannot authorize an action, expand scope, change permissions, or override the
Mindrail lifecycle, validation, or completion gate. It does not replace the coding
agent's reasoning model. Record consequential host decisions through the normal
Mindrail mechanisms, whether or not Jev advised them.

## Lifecycle

| Stage | Phases | Reference |
| --- | --- | --- |
| Intake, discovery, clarification | 0–2 | `references/planning.md` |
| Requirements, acceptance criteria, design | 3–5 | `references/planning.md` |
| Work breakdown, dependency graph | 6–7 | `references/planning.md` |
| Teams, assignment, parallel execution | 8–10 | `references/execution.md` |
| Coordination, decision log, escalation | 11–13 | `references/execution.md` |
| Task validation, testing, integration, full validation | 14–17 | `references/execution.md` |
| Pre-audit package, dual-agent audit, consensus gate | 18–21 | `references/audit-and-acceptance.md` |
| Remediation loop, final acceptance, final report | 22–23 | `references/audit-and-acceptance.md` |
| How the audit is scoped so its cost follows risk, not task count | — | `references/audit-scoping.md` |

Skip the stages your tier excludes; never skip 18–21.

## How the audit scales

The audit gate is non-negotiable; its *size* is not. Five rules keep a multi-task
delivery from spending longer in audit than in implementation — full detail and the
audit-plan template are in `references/audit-scoping.md`:

1. **One audit per delivery, never one per task.** Phase 14 is the orchestrator
   checking a self-report (diff + the brief's tests); it is not
   `dual-agent-task-audit`, and finishing a `TASK-*` is not a reason to invoke it.
2. **Depth per cluster, by risk.** Group the changeset by task and assign each
   cluster A (full six-move Breaker), B (mutation + weakest-input + class sweep) or
   C (Reader only). Escalator surfaces — schema, auth, contracts, destructive ops,
   config, CI — are always A. Depth is a floor auditors may raise, never a ceiling.
3. **Mutation runs the cluster's targeted tests; the full suite runs once.** The
   Breaker's cost is guards × suite runtime, so the Phase 15 test map goes into the
   audit package as a per-cluster `test_cmd`.
4. **Checkpoint audits.** With subagents at Tier 3, audit an independent workstream
   the moment it's accepted, in parallel with the rest; the final audit covers what
   landed since, what was touched again, and the integration seams.
5. **Re-audits are delta-only.** Remediation rounds re-verify the `FIX-*` changes,
   their callers, their `REQ-*` and the class-sweep siblings — not the guards and
   clusters the fix never touched.

`scripts/audit_scope.py` drafts the plan from the diff (risk class per cluster,
guard count, projected mutation cost, which conditional Breaker moves are triggered).
Correct it by reading the diff; it catches risky paths and keywords, not semantics.

```bash
python scripts/audit_scope.py <repo> --base <task-start-ref> --tasks tasks.json --test-seconds 120
python scripts/audit_scope.py <repo> --base <last-audited-ref> --delta        # remediation round
```

## Validation evidence

Run the validation helper to detect the project's real commands and capture results
as evidence rather than assertion:

```bash
python scripts/validate.py <repo-path>                   # detect, show what would run
python scripts/validate.py <repo-path> --run --label baseline     # slow suite: --only test with the impacted command instead
python scripts/validate.py <repo-path> --run --label post-change
python scripts/validate.py --compare baseline.json post-change.json
```

Capturing a baseline **before** implementation is what lets you distinguish
pre-existing failures from ones the task introduced. Without it, a red suite is
ambiguous and you'll either take blame for someone else's breakage or hide your own.

## Which tests to run, and when

Slow suites are the norm in real projects, and running everything at every step
turns a two-hour task into a day. The rule is **narrow in the loop, full once at the
gate**:

- **Inner loop** — Phase 14 task acceptance, the Breaker's mutation move, each
  remediation round, and the baseline — runs only the impacted tests. The question
  there is "does this change keep its own promise?", and the full suite answers it no
  better, only later.
- **Gate** — exactly one full run after integration and audit (Phase 17 / post-audit).
  Every selection strategy has blind spots (dynamic imports, string-keyed registries,
  config-driven behavior, shared fixtures), and the full run exists to catch what the
  selector cannot see.

`scripts/impacted_tests.py` selects the tests, tries strategies in order of
reliability — the repo's own impact map, coverage.py per-test contexts, the import
graph (transitive depth 2), naming convention — and says which one it used and how
much to trust it. Two things it will not do: narrow silently when a changed source
file reaches no test (then the round runs the full unit suite, and the unreachable
file is a Reader finding), and select e2e tests from anything but an explicit map,
because e2e behavior cannot be traced to source statically.

```bash
python scripts/impacted_tests.py <repo> --base <task-start-ref>           # prints the command + confidence
python scripts/impacted_tests.py <repo> --changed src/a.py src/b.py -o impact.json
```

If the full suite is too slow even once, the default is: full unit/integration suite
plus only the mapped e2e tests, with the unrun e2e tests listed by name as
unverified in the final report. Deferring the full run to CI is allowed only when the
user asks for it, and it changes the final status to `COMPLETED_PENDING_FULL_SUITE`
— never `COMPLETED_AND_VERIFIED`. A green subset is evidence about the subset.

## Orchestrator rules

Must: investigate, plan, decompose, delegate, coordinate, monitor, challenge,
validate, integrate through the responsible agents, initiate the independent audit,
manage remediation.

Must not: silently fix an agent's mistakes (assign a remediation task instead — a
silent fix erases the signal that the brief was unclear), manufacture consensus, hide
failed tests or audit findings, or declare success on the basis of agent
self-reporting.

## Evidence standard

Reject "Implemented successfully." Require changed-file references, the relevant code
paths, and a mapping to acceptance criteria.

Reject "Tests pass." Require which tests, which command, what output, which
requirements they verify — and whether it was an impacted-only run or the full one.

Reject "Audit passed." Require both auditor verdicts, the consensus result,
evidence-backed findings, the disposition of every disagreement, and the audit plan
showing which cluster received which depth — a light pass on a risky cluster is a
gap, and it should be visible as one.

## Completion bar

Complete requires all of: implementation, integration, validation (including the
single full-suite run), an independent dual-agent audit, and evidence-based
consensus. Not one of: agents finished, code
compiles, tests pass, one reviewer approved, or it looks right.

If the audit doesn't reach `VERIFIED` or `VERIFIED_WITH_MINOR_ISSUES` within the
remediation cap in `references/audit-and-acceptance.md`, report `NOT_COMPLETED` or
`BLOCKED` honestly. A truthful incomplete beats a confident false completion — the
user can act on the first and gets burned by the second.
