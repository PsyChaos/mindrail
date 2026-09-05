# Mindrail 0.1 — Kernel Scope

> **Product release scope:** 0.1 Kernel
> **Architecture target:** Mindrail Technical Specification 1.0
> **Purpose:** Deliver the smallest usable Mindrail engineering gate before semantic/coverage breadth expands.

---

# 1. Goal

Mindrail 0.1 must prove one product hypothesis:

> **A coding agent can change code; Mindrail can independently discover the actual change, associate durable engineering constraints with it, require current evidence, and deny completion when the proof is insufficient.**

0.1 is deliberately not the full Technical Specification 1.0 implementation.

---

# 2. Primary End-to-End Scenario

```text
Agent A / user creates task
        ↓
Mindrail records task + invariant/decision
        ↓
Agent edits code
(before_change may or may not have been called)
        ↓
Mindrail reconcile discovers actual diff
        ↓
Tree-sitter identifies changed symbols
        ↓
Mindrail finds structural impact + related invariant
        ↓
Validation profile is selected
        ↓
Evidence is bound to source snapshot
        ↓
Source changes again? evidence becomes stale
        ↓
Completion Gate ALLOW / DENY
```

If this scenario is reliable and fast, Mindrail's core value is proven.

---

# 3. IN Scope

## Core executable

- Go executable
- CLI
- local-first runtime
- repository discovery
- Git common-dir/workspace discovery

## Runtime state

- SQLite
- migrations
- WAL
- short transactions
- bounded busy retry
- operation idempotency
- optimistic revision

## Engineering knowledge

- `.mindrail/`
- project facts
- Decision
- Invariant
- versioned JSON knowledge records
- knowledge schema validation

Schema policy for 0.1:

```text
write_schema_version     = 1
readable_schema_versions = [1]
```

Every record carries `schema_version` from day one, and an unknown newer version fails closed. The mixed-version reader window, lazy per-record upgrade and `mindrail knowledge migrate` are deferred — but the field and the fail-closed branch are not, because retrofitting them later would require rewriting existing repository knowledge.

## Coordination

- AgentSession
- Task
- Task lease
- Change
- symbol/file lease where available
- Checkpoint
- same-workspace sequential agent flow
- separate-worktree concurrent flow

## Change discovery

- `mindrail_before_change` as coordination optimization
- **reconcile as canonical correctness path**
- Git/worktree/staged diff discovery
- scope drift
- unregistered change attribution
- ambiguity instead of guessing

## Syntax intelligence

- Tree-sitter
- Python STRUCTURAL
- TypeScript STRUCTURAL
- JavaScript STRUCTURAL
- symbols
- imports
- obvious references/calls
- body/signature/structure fingerprints
- durable `symbol_uid`
- rename/move identity migration using structural + Git signals

## Basic impact

- direct structural references
- bounded reverse traversal
- file/module fallback
- invariant matching
- explainable provenance

### Structural precision limits

0.1 has no semantic resolver, so reverse references are resolved by name and scope heuristics. A lookup for `calculate_timeout` matches every same-named declaration in the repository. Left unbounded this produces over-broad impact sets, escalates validation breadth on every edit, and breaks the latency targets in section 5.

0.1 therefore constrains structural impact explicitly:

```text
structural edge confidence  ≤ 0.5, labelled STRUCTURAL_NAME_MATCH
default reverse depth       1
depth > 1                   only on explicit request
```

Escalation rule:

> **A structural-only edge may not raise validation breadth above MODULE by itself.**

Broader breadth in 0.1 requires a non-structural reason: an explicit invariant scope, a path policy, or a public API rule. This keeps the conservative-validation fallback from degenerating into "run the whole suite on every edit", which is the fastest way for users to disable the tool.

## Validation

- project-defined validation profiles
- argv-only process execution
- timeout
- bounded output
- secret redaction
- evidence snapshot hash
- stale evidence invalidation

## Test guard

Low-cost detection:

- assertion/expect removal
- assertion count decrease
- skip/xfail/disabled marker added
- critical verification test removal

## Completion gate

Completion is denied when:

- blocking invariant unresolved,
- required evidence missing,
- evidence stale,
- protected symbol identity ambiguous,
- unregistered change not reconciled.

## MCP

All 13 tools are implemented in 0.1. What 0.1 constrains is their **parameter surface**, not their count.

Technical Specification 1.0 treats MCP tool schemas as external contracts: adding a field or an enum value later is backward compatible, changing or removing a published one is not. 0.1 therefore publishes only the parameters it fully honours.

```text
mindrail_bootstrap
mindrail_status
mindrail_search
mindrail_context
mindrail_claim
mindrail_before_change
mindrail_after_change
mindrail_reconcile
mindrail_decide
mindrail_invariant
mindrail_checkpoint
mindrail_validate
mindrail_complete
```

0.1 parameter constraints:

| Tool | 0.1 surface | Deferred |
|---|---|---|
| `mindrail_context` | `detail_level = summary \| focused` | `full`, `target_type = impact_group`, `cursor`, `page_size` |
| `mindrail_invariant` | `mode = active` | `mode = candidate` |
| `mindrail_validate` | named profile from project config | budget classes, escalation alternatives |
| `mindrail_complete` | local evidence gate | managed/signed approval assurance levels |

A deferred value MUST be rejected with a structured `NOT_IMPLEMENTED_IN_THIS_VERSION` error carrying `next_action`. It is never silently ignored and never silently downgraded to a weaker behaviour.

## Git enforcement

- `mindrail verify --staged`
- `mindrail verify --ci`
- pre-commit integration without overwriting existing hooks

`mindrail verify --ci` is IN scope for 0.1 despite the extra surface, because a pre-commit hook alone is not a gate. `git commit --no-verify` bypasses it entirely, and agents genuinely reach for that flag when a hook fails. Without an independent re-check, the exit condition in section 8 would not hold.

The 0.1 CI path is deliberately minimal and reuses the same engine as the local path:

```text
fresh clone
    ↓
knowledge validation
    ↓
merge-base(base, head) diff
    ↓
reconcile → changed symbols
    ↓
applicable invariants + test-weakening guard
    ↓
required validation profiles
    ↓
structured ALLOW / DENY
```

No runtime task lease, no portable manifest and no cross-run impact cache in 0.1.

## Diagnostics

- `mindrail init`
- `mindrail status`
- `mindrail doctor`
- structured errors
- JSON output where appropriate

## Performance measurement

From 0.1, collect:

- status/context/before/after/reconcile duration,
- SQLite wait,
- parse time,
- impact traversal time,
- false-positive/missed-impact feedback storage hooks.

---

# 4. OUT of Scope for 0.1

The following remain part of the target Technical Specification 1.0 but are not required for 0.1:

- Pyright FULL semantic integration
- TypeScript Language Service FULL semantic integration
- Resolver Manager managed downloads
- resolver pool/LRU/resource budget beyond adapter stubs
- runtime coverage providers
- Python coverage mapping
- TS/JS coverage mapping
- C#/Java/Go/PHP analysis
- framework magic adapters
- managed identity approval
- signed human approval
- advanced approval providers
- candidate invariant generation
- calibration report UI
- advanced fan-out aggregation/drill-down
- portable CI change manifests
- remote MCP/HTTP
- remote/team Mindrail service
- vector search
- embeddings
- graph database

---

# 5. 0.1 Performance Targets

0.1 runs STRUCTURAL only, so these are the STRUCTURAL column of the Technical Specification 1.0 SLO table. The FULL semantic targets apply once resolvers land.

Warm-path goals:

| Operation | p95 target (STRUCTURAL) |
|---|---:|
| `mindrail_status` | <150 ms |
| `mindrail_context` | <250 ms |
| `mindrail_before_change` | <300 ms |
| `mindrail_after_change` ≤10 files | <1.5 s |
| `mindrail_reconcile` ≤10 files | <2 s |

These are product acceptance targets and must be measured from the first implementation.

An operation that cannot finish within budget returns a partial result with an explicit pending state and schedules the remainder. Blocking past the budget and silently skipping analysis are both non-compliant.

---

# 6. 0.1 Acceptance Criteria

1. **AC-01** Mindrail initializes in a Git repository without cloud services.
2. **AC-02** A Decision/Invariant survives process/session restart through `.mindrail/knowledge`.
3. **AC-03** Two sequential agents in the same workspace can continue the same task through checkpoint/context.
4. **AC-04** Two concurrent agents cannot silently obtain conflicting active leases for the same protected target.
5. **AC-05** An edit performed without `mindrail_before_change` is still discovered by reconcile.
6. **AC-06** Python/TS/JS changed function/method body is detected structurally.
7. **AC-07** A rename of a protected symbol either preserves `symbol_uid` or produces explicit ambiguity; the invariant cannot silently disappear.
8. **AC-08** A configured validation command produces evidence tied to the current source snapshot.
9. **AC-09** Editing source after validation makes that evidence stale.
10. **AC-10** Critical verification test removal/skip produces test-weakening finding.
11. **AC-11** Completion fails when required current evidence is missing.
12. **AC-12** `mindrail verify --staged` detects unreconciled changes.
13. **AC-13** Warm-path SLO benchmark results are emitted.
14. **AC-14** CLI and MCP use the same application/domain services.
15. **AC-15** A retried mutating operation with the same `operation_id` does not create a duplicate Task, Change or Evidence row.
16. **AC-16** A stale update against a mutable entity produces a revision conflict instead of silent last-write-wins.
17. **AC-17** A configured known secret does not reach evidence storage in plaintext.
18. **AC-18** Two processes observing the same new symbol concurrently agree on a single `symbol_uid`.
19. **AC-19** A commit made with `--no-verify` is still denied by `mindrail verify --ci`.
20. **AC-20** Removing or skipping the verification test of an active CRITICAL invariant produces a blocking finding even when the same change touches no production symbol.
21. **AC-21** A deferred MCP parameter value returns `NOT_IMPLEMENTED_IN_THIS_VERSION` rather than being ignored.
22. **AC-22** A structural-only edge does not by itself escalate validation breadth above MODULE.

---

# 7. Non-Goals

0.1 does not attempt to prove that static dependency analysis is complete.

It proves that Mindrail can provide value with:

```text
structural understanding
+
explicit invariants
+
conservative validation
+
current evidence
```

FULL semantic resolvers and coverage providers improve precision later; they are not required to validate the product's fundamental gate model.

---

# 8. Exit Condition

Mindrail 0.1 is complete when this statement is demonstrably true:

> **An agent cannot bypass the engineering gate merely by forgetting a protocol call, skipping the local hook, renaming a protected symbol, weakening its critical test, or claiming stale test results as proof.**
