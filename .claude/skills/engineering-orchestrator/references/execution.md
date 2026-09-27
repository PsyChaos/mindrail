# Execution (Phases 8–17)

Contents:
- [Phase 8 — Team formation](#phase-8--team-formation)
- [Phase 9 — Agent assignment](#phase-9--agent-assignment)
- [Phase 10 — Parallel execution](#phase-10--parallel-execution)
- [Phase 11 — Coordination between agents](#phase-11--coordination-between-agents)
- [Phase 12 — Decision log](#phase-12--decision-log)
- [Phase 13 — Escalation](#phase-13--escalation)
- [Phase 14 — Continuous task validation](#phase-14--continuous-task-validation)
- [Phase 15 — Testing strategy](#phase-15--testing-strategy)
- [Phase 16 — Integration](#phase-16--integration)
- [Phase 17 — Full validation](#phase-17--full-validation)

Phases 8–13 apply at Tier 3, partially at Tier 2, and are skipped at Tier 1.
Phases 14–17 apply at every tier.

---

## Phase 8 — Team formation

Create teams only where the task has a genuinely distinct concern. Candidates:
Architecture (contracts, interfaces, boundaries), Backend (services, APIs,
persistence, business logic), Frontend (UI, state, client integration), Database
(schema, migrations, queries), Integration (external APIs, queues, events), Security
(authn/authz, sensitive operations), Testing, Documentation, DevOps (CI/CD,
containers, deployment).

A "team" here is a scope boundary and a brief, not headcount. One agent may hold one
team. Creating a Frontend Team for a change with no UI produces an agent that reports
"nothing to do" and consumes a round-trip for it.

---

## Phase 9 — Agent assignment

Each agent's brief contains: the original user task, its relevant `REQ-*`, its
assigned `TASK-*`, the acceptance criteria, the relevant repository context,
constraints, dependencies and expected outputs.

Give the agent enough context to understand how its work fits the whole, and a scope
narrow enough that it doesn't wander. Unrestricted scope produces unrequested changes
that surface later as audit findings.

---

## Phase 10 — Parallel execution

Run genuinely independent workstreams concurrently, then synchronize before
integration.

When an independent workstream is accepted (Phase 14) while others are still
running, start its checkpoint audit now rather than holding it for the end — see
Rule 4 in `references/audit-scoping.md`. The audit then runs in the shadow of the
remaining implementation instead of after it. Record the ref it was audited at; if
a later task touches those files again, the checkpoint verdict is void for them.

Before launching, check that parallelism cannot create conflicting edits to the same
files, incompatible contracts, duplicated implementations, or divergent architectural
assumptions. Detecting these afterwards means throwing work away — that is the
orchestrator's responsibility, not the agents'.

---

## Phase 11 — Coordination between agents

Agents' tasks intersect, and those intersections need resolving. The runtime
constraint: **subagents cannot message each other.** Every exchange goes through the
orchestrator, which means each one costs a round-trip.

Two consequences:

1. Front-load the contracts. Decide shared types, response shapes and interface
   signatures in Phase 5 and put them in the briefs. Most inter-agent questions exist
   because a decision was deferred that could have been made up front.
2. When a question does arise, carry it explicitly: take it to the other agent with
   enough context to answer, and return the answer to the asker and to any other
   agent the answer affects.

Legitimate subjects: contracts, assumptions, interfaces, shared types, data
structures, API behavior, architectural constraints, test expectations, discovered
edge cases, implementation risks.

Agents may challenge each other and request evidence. They must not make cross-team
architectural decisions silently — those come to the orchestrator and enter the
decision log.

Without subagents, this phase becomes explicit self-checking: before implementing a
piece that another piece consumes, write down the contract and verify the consumer
against it, rather than assuming both sides of an interface you're writing agree.

---

## Phase 12 — Decision log

(Tier 2 and 3.) Record every material decision as `DEC-001`, …:

| Decision | Context | Agents Involved | Evidence | Impact |
| --- | --- | --- | --- | --- |

Its purpose is to stop two workstreams from proceeding on incompatible assumptions,
and to let the audit check that decisions were grounded in requirements, repository
evidence, existing architecture or user clarification — not preference.

---

## Phase 13 — Escalation

An agent escalates on: ambiguous requirement, architecture conflict, unexpected
dependency, potential breaking change, security concern, data-loss risk, incompatible
package, scope expansion, or conflicting implementation by another team.

The orchestrator then resolves from repository evidence, coordinates agents, adjusts
dependencies, creates a new task, or asks the user.

Blockers must never be hidden. An agent that works around a blocker silently
converts a five-minute decision into a defect discovered at audit.

---

## Phase 14 — Continuous task validation

When an agent reports `TASK-*` complete, verify before accepting: the expected files
actually changed, acceptance criteria are met, relevant tests pass, no scope creep,
dependencies still compatible, and downstream tasks have the outputs they need.

Code being written is not task completion. Accepting self-reports unverified is how
a chain of "done" tasks produces a broken system.

This check is deliberately cheap — one diff read, one impacted-test run
(`scripts/impacted_tests.py --changed <task's files>`, not the suite), one look
for files the brief didn't name. If the selector reports a changed file that reaches
no test, that is the first thing to raise with the agent: the task shipped code
nothing exercises. It is **not** the dual-agent audit, and a task
finishing is not a trigger to invoke `dual-agent-task-audit`; that skill's own
description ("verify this properly", "is this really done") will tempt you at every
task boundary, and giving in multiplies the audit by the number of tasks. Write
"verified per Phase 14, not audited" against each accepted task. The one audit
happens at Phase 19 over the integrated result, sized per cluster by risk
(`references/audit-scoping.md`).

---

## Phase 15 — Testing strategy

Testing is mandatory. Choose forms that fit the change: unit, integration,
regression, contract, edge-case, failure-path, end-to-end.

| Requirement | Test / Verification | Status |
| --- | --- | --- |

Every `REQ-*` maps to at least one verification mechanism.

Record, per `TASK-*`, the narrowest command that runs that task's tests — take it
from `scripts/impacted_tests.py` and correct it by hand where you know better. This
becomes the cluster's `test_cmd` in the audit plan: the Breaker's mutation move runs
it once per neutralized guard instead of the whole suite, which is the difference
between minutes and hours on a multi-task delivery. If the repo has no impact map
and no coverage contexts and the selector's confidence comes back low, the cheapest
fix is usually to add a `.impact-map.json` for the touched modules as part of the
task — it costs ten minutes and pays back on every round, and every future task.

E2e tests are never narrowed by inference; they run in full at the gate or via the
explicit map only. Say here which it will be.

Distinguish **code coverage** from **requirement coverage**. High coverage means
lines executed; it says nothing about whether the required behavior is asserted.
Enforce a 100% coverage rule only if the project already has one — otherwise prefer
meaningful behavioral assertions over coverage inflation.

---

## Phase 16 — Integration

After team tasks complete, verify that contracts align, modules compile and import
correctly, migrations match models, API consumers match providers, shared types
agree, configuration is complete, tests agree with the implementation, and docs
reflect final behavior.

Resolve conflicts through the responsible agents. The orchestrator assigns
remediation rather than patching integration directly — patching it yourself hides
which brief was wrong and guarantees the same conflict next time.

---

## Phase 17 — Full validation

Run everything applicable: build, compilation, type checking, linting, static
analysis, unit tests, integration tests, regression tests, end-to-end tests,
migration validation, dependency validation.

Capture exact commands and exact results. `scripts/validate.py` detects the project's
real command set and records output to JSON.

This is the **one** full-suite run of the delivery (a second happens after
remediation only if a round changed code). Everything before it ran impacted tests;
this is where the selector's blind spots get covered, so do not narrow it. If it is
too slow to run even once, run the full unit/integration suite and only the mapped
e2e tests, and list the e2e tests that did not run — by name — as unverified.
Handing the full run to CI is a user decision, not the orchestrator's, and it lowers
the final status to `COMPLETED_PENDING_FULL_SUITE`.

Distinguish pre-existing baseline failures from failures the task introduced. If a
baseline wasn't captured before implementation, say so and treat ambiguous failures
as unresolved rather than assuming they were already broken.
