# Audit package — zero-ceremony UX

## Original user request

> “ben hayatimda bu kadar karisik kullanim gormedim, surekli cli da birseyler
> yapmak lazim. ne alaka yani. kullanacak kisi neden bui kadar seyle ugrassin”
>
> “yapin o zaman”

Standing execution constraint from the same conversation: the primary agent must
not write the production implementation; it must form and supervise a model-tiered
team, parallelize safe work, use applicable skills and independently verify the result.

## Frozen requirements

The authoritative normalized requirements and decisions are in
`docs/engineering/zero-ceremony-ux-2026-09-26.md` (`REQ-001` through `REQ-011`).
Audit against that file without reshaping it around the implementation.

## Implementation summary

- Human default CLI: `init`, `status`, `doctor`, `verify`, `version`; expert paths
  remain callable but hidden from root help. Bare `verify` selects staged/local mode.
- `init` idempotently scaffolds config/knowledge, owns only the marked AGENTS section,
  safely chains a foreign pre-commit hook, and preserves user content.
- The existing 13 MCP names remain exact. Optional automatic bootstrap/finalize mode
  hides session/task/lease/revision/operation IDs from people while legacy payloads
  keep their original side effects.
- `internal/workflow` owns durable run-key start/replay, scope, heartbeat, reconcile,
  configured-profile validation, guarded completion and handoff.
- Automatic reconciliation isolates inherited dirty files, re-applies A→B→A states,
  handles exact-baseline restoration, and retires only the current task’s obsolete
  attribution.
- Migration 010 adds durable file-index generations so stale remove/completion tokens
  cannot match a recreated file after the live row was removed.
- README and Turkish usage guide lead with the simple path; mechanics live under an
  advanced compatibility section.

## Review and remediation history

Independent code and Go reviews produced and then re-verified FIX-001 through FIX-012,
including ID-free finalize replay, historical reconcile, context races, guard baseline
ordering, stale reply monotonicity, subdirectory launch, protocol stdout isolation,
historical profile replay, heartbeat/finalize race, handoff cleanup, exact-baseline
restoration and file-index CAS ABA. Current reviewer verdicts are APPROVE with no open
confirmed findings; the final Reader/Breaker must verify rather than trust those claims.

## Test evidence on the final schema-10 tree

All commands used `GOCACHE=/tmp/mindrail-gocache` where applicable.

- `make tidy-check` — PASS.
- `make verify` — PASS: vet, full normal suite, full `-race ./...`, compiled-binary smoke.
- `make gate` — PASS: unit 1417, domain 421, integration 12, race 9,
  knowledge-schema 216, MCP contract 57, git-worktree 77,
  named-worktree 8, sqlite-concurrency 17, end-to-end 12, smoke 7.
- `make bench` — PASS. MCP p95: status 0.920 ms, context 0.729 ms,
  before-change 1.282 ms, after-change 11.233 ms, reconcile 11.923 ms.
- `make release` — PASS for native linux/amd64 plus checksums and smoke.
  linux/arm64, Darwin and Windows remain not-built because Tree-sitter Go bindings
  exclude those cross-build targets; this is an existing release constraint.
- Built-binary live workflow on this repository: `init` upgraded through schema 10,
  installed generated AGENTS/hook, then `status`, `doctor` and flagless `verify`
  all exited 0. Status is `PARTIAL_READY` only because no project units are indexed;
  doctor is `Overall: OK`; verify is `ALLOW` in staged/local mode.
- `graphify update .` — PASS after final code; no topology change remained on last run.

## Required audit outputs

Reader and Breaker must write separate reports in this directory before seeing the
other conclusion. Breaker uses an isolated scratch copy and must confirm the original
worktree was not mutated. Reconciliation writes `report.md` using the skill template
and emits exactly one final verdict.

The audit plan is `audit-plan.md`. Depth is a floor, especially for migration 010,
hook/setup behavior, public MCP schemas and concurrency/lease state.
