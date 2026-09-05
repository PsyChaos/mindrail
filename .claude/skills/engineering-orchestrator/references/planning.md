# Planning (Phases 0–7)

Contents:
- [Phase 0 — Task intake](#phase-0--task-intake)
- [Phase 1 — Project discovery](#phase-1--project-discovery)
- [Phase 2 — Clarification gate](#phase-2--clarification-gate)
- [Phase 3 — Requirement normalization](#phase-3--requirement-normalization)
- [Phase 4 — Acceptance criteria](#phase-4--acceptance-criteria)
- [Phase 5 — Solution design](#phase-5--solution-design)
- [Phase 6 — Work breakdown](#phase-6--work-breakdown)
- [Phase 7 — Dependency graph](#phase-7--dependency-graph)

---

## Phase 0 — Task intake

Determine what is explicitly requested, the expected outcome, what must remain
unchanged, and any technical, business, compatibility, performance, security, testing
or documentation constraints — plus explicitly excluded work.

Do not start coding here. The cost of a misunderstood task is paid at the end, in
rework, and it is always higher than the cost of ten minutes of reading.

---

## Phase 1 — Project discovery

Inspect the repository before designing anything: project type, languages,
frameworks, dependency versions, package managers, architecture, module boundaries,
coding conventions, test infrastructure, CI/CD, deployment model, relevant docs.

Then locate the parts of the system the task actually affects, tracing entrypoints,
callers, callees, APIs, models, services, repositories, state, configuration,
persistence, integrations and tests.

Never design from assumption where repository evidence exists. A solution that
ignores the codebase's established patterns will be rejected in review even when it
is technically correct.

---

## Phase 2 — Clarification gate

Ask the user only about decisions that cannot be safely inferred and that materially
affect architecture, business behavior, public API, data model, backwards
compatibility, security, destructive operations, irreversible decisions, major UX
behavior, or deployment.

Do **not** ask what the repository already answers — existing patterns, tests,
configuration, documentation and conventions. Asking a user to re-state what their
own code says is the fastest way to feel like a burden rather than help.

Questions must be specific, minimal, decision-oriented, and grouped into a single
message. For each, say briefly why the answer changes what gets built.

If nothing blocking remains, continue without asking.

---

## Phase 3 — Requirement normalization

Convert the task into explicit requirements with IDs (`REQ-001`, …).

Types: `FUNCTIONAL`, `TECHNICAL`, `BUSINESS`, `SECURITY`, `PERFORMANCE`,
`COMPATIBILITY`, `TESTING`, `DOCUMENTATION`, `NON_REGRESSION`.

| ID | Requirement | Type | Source | Acceptance Criteria |
| --- | --- | --- | --- | --- |

Every requirement must originate from the user's task, an unavoidable technical
consequence, or an established repository contract. Mark inferred ones explicitly as
`[INFERRED]` so the user can veto them — an invented requirement becomes invented
scope, and the user pays for work they never asked for.

Without subagents, write this table to a file now, before any code exists. It is the
frozen spec the audit will grade against.

---

## Phase 4 — Acceptance criteria

Every requirement needs objectively verifiable criteria. These are handed to the
audit agents later, so vagueness here becomes an unverifiable audit.

Weak: "Authentication should work correctly."

Strong: "Requests without a valid token to `POST /orders` return 401; requests with
valid credentials return the existing 201 response body unchanged, including field
`order_id`."

Prefer criteria a test can assert. Where something genuinely can't be automated, say
how it will be verified manually and by whom.

---

## Phase 5 — Solution design

Design the smallest correct solution. Identify affected modules, code changes,
schema/config changes, integrations, tests, documentation, migration requirements and
deployment implications.

**Minimal Change Principle.** No unrelated refactoring. Do not redesign working
architecture unless the task requires it. Follow existing project conventions even
where you'd personally choose differently — consistency is a property of the codebase,
and a locally better pattern that appears once is a net loss.

---

## Phase 6 — Work breakdown

(Tier 2 and 3.) Decompose into atomic tasks with IDs (`TASK-001`, …):

| Field | Description |
| --- | --- |
| Task ID | Unique identifier |
| Objective | Exact expected outcome |
| Requirements | Related `REQ-*` |
| Files/Modules | Expected scope |
| Dependencies | Other `TASK-*` |
| Inputs | Information required to start |
| Outputs | Expected artifacts |
| Acceptance | Completion criteria |
| Complexity | LOW / MEDIUM / HIGH |
| Parallelizable | YES / NO |

Tasks should be small enough for unambiguous ownership and large enough to be
meaningful engineering work. Splitting below that line multiplies coordination cost
without improving anything.

---

## Phase 7 — Dependency graph

(Tier 3, and Tier 2 where ordering isn't obvious.) Build a DAG and classify:
independent tasks (start immediately), dependent tasks (need another's output),
blocking tasks (gate several downstream tasks), integration tasks (combine outputs).

Never parallelize tasks with unsafe write dependencies to the same files or
contracts. Prefer parallelism when tasks modify independent modules, investigate
independent concerns, write independent test groups, update independent
documentation, or perform independent verification.

Correct ordering beats parallelism. Two agents editing the same interface
concurrently will produce work that has to be redone, which is slower than doing it
in sequence.
