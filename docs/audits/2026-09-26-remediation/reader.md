# Remediation audit — independent Reader report

Date: 2026-09-26
Base: `2814acb22d8aa94d48ade1242b58e1fdb14239fc`
Mode: parallel independent Reader/Breaker audit. This report was written without
reading the remediation Breaker's conclusions. The Reader did not participate in
the implementation and did not edit production source.

## 1. Task normalization

Original task, verbatim:

> o zaman bu sorunların hepsini düzeltmeni istiyorum

The referent and frozen acceptance contract are the eight requirements in
`docs/engineering/remediation-2026-09-26.md`. I normalized those requirements
before inspecting the implementation:

| ID | Requirement | Type | Verification method |
|---|---|---|---|
| REQ-001 | Failed, errored, incomplete, or mixed validation runs cannot satisfy completion; a later complete successful run can recover. | FUNCTIONAL / SECURITY | Validation service-to-freshness tests plus grouping/status/exit-code inspection. |
| REQ-002 | Directory/glob membership additions, edits, and removals stale evidence; out-of-scope changes do not. | FUNCTIONAL | Scope re-enumeration tests and `SnapshotScope`/provenance inspection. |
| REQ-003 | Current fatal/unreadable/invalid knowledge refuses completion deterministically. | FUNCTIONAL / COMPATIBILITY | Live-server and restarted-server negative tests plus loader/schema data flow. |
| REQ-004 | Completion performs canonical reconcile before the gate and fails closed on discovery errors. | FUNCTIONAL / SECURITY | Skipped-protocol, honest edit, repeated decision, and Git-failure tests. |
| REQ-005 | The production binary serves exactly 13 MCP tools over protocol-pure stdio and advertises compatibility. | FUNCTIONAL / PUBLIC CONTRACT | Built-binary SDK subprocess test and version contract. |
| REQ-006 | Raw `COMPLETED` mutation is refused; gated completion preserves state/lease on DENY and atomically completes/releases on ALLOW, with compatible JSON/revision/idempotency behavior. | FUNCTIONAL / PUBLIC CONTRACT / BACKWARD COMPATIBILITY | CLI integration tests plus black-box operation replay probe. |
| REQ-007 | Usage, audit addendum, CLI help, and compatibility text describe the executable behavior. | DOCUMENTATION | Documentation-to-binary comparison and live help/error probes. |
| REQ-008 | Scope remains limited and the targeted/full validation, gate, benchmark, tidy, and native release checks pass. | NON_REGRESSION / TESTING | Changeset review, targeted tests, vet, tidy, gate, and recorded pre-audit evidence. |

No task ambiguity changes these requirements: the remediation plan explicitly
freezes the four reproduced completion defects and the two public integration
gaps as the referent of the Turkish request.

## 2. Requirement verdicts

| Requirement | Verdict | Positive evidence | Notes |
|---|---|---|---|
| REQ-001 | PASS | `TestProfileRunMustBeWhollySuccessful` exercises pass, fail, spawn error, mixed pass/fail, missing row, and later recovery. `validation.Check` requires `PASS`, exit 0, complete command indices, consistent run metadata, and selects only the newest run (`internal/validation/freshness.go:77-154`). Focused package suite passed. | Legacy provenance fails closed in `TestCheckFailsClosedOnLegacyEvidence`. |
| REQ-002 | PASS | `TestDeclaredScopeMembershipChangesStaleEvidence` covers directory and recursive-glob add/edit/remove plus out-of-scope additions. `RunProfile` stores declarative `scope_paths`; `Check` re-runs `SnapshotScope` rather than trusting the historical file list. Focused package suite passed. | Literal files remain file-grained as documented; directory/glob membership is re-enumerated. |
| REQ-003 | PASS | `completion.Service.loadKnowledge` reloads current files, refuses every loader problem, and runs schema/lineage validation (`internal/completion/compose.go:97-134`). `TestCompleteRefusesBrokenKnowledge` covers unsupported schema, invalid JSON, missing version, invalid schema, unreadable record, current/restarted servers, repeatability, and repair recovery. | Completion is deliberately stricter than the reporting snapshot and does not trust long-lived startup state. |
| REQ-004 | PASS | `completion.Service.Evaluate` loads knowledge, captures the guard baseline, calls canonical `changes.Reconcile`, checks HEAD, composes, checks HEAD again, then evaluates (`internal/completion/service.go:43-74`). Skipped-protocol, declared edit, repeated decision, and broken-Git tests passed. | All three production task-discovery callers are inventoried by an architecture test and use ensure-before/check-after ordering. |
| REQ-005 | PASS | `TestMCPSubprocessServesThirteenTools` builds the executable, connects with the official SDK command transport, lists the exact 13 names, and calls `mindrail_bootstrap`. `version --json` asserts `mcp_compatibility:"stdio"`. The focused `cmd/mindrail`, CLI, and MCP suites passed. | The special `--json` refusal has a documentation mismatch recorded as RDR-003, but normal stdio launch and stdout purity satisfy this requirement's core acceptance criteria. |
| REQ-006 | PARTIAL | Raw completion refusal and unchanged task/lease are covered by `TestRawTaskCompletionIsRefusedWithoutMutatingTaskOrLease`; ALLOW/DENY state, lease, revision and identical replay are covered by `TestTaskCompleteCouplesTerminalStateToTheGate`. The final state/lease update is one `TransitionExpecting` transaction. | RDR-001 shows that changing `--required` while reusing the completion operation ID incorrectly replays success instead of raising `OPERATION_ID_CONFLICT`. The primary gate coupling works; the explicit replay-compatibility edge does not. |
| REQ-007 | PARTIAL | `docs/usage-tr.md` accurately documents schema 9, current evidence semantics, the 13-tool launcher, gated completion, historical audit status, and known policy limits. Historical audit/verification files have prominent snapshot addenda. | Live CLI help still says every transition goes through `task state` and that it is the only state-changing command (RDR-002). The troubleshooting table also promises a code the executable does not emit for `mcp --json` (RDR-003). |
| REQ-008 | PARTIAL | Independent targeted tests, `go vet ./...`, `make tidy-check`, formatting/diff whitespace checks, and `make gate` passed. Gate counts: unit 1332, domain 411, integration 10, race 9, knowledge-schema 216, MCP contract 34, worktree 77/7, SQLite concurrency 17, E2E 10, smoke 4. The audit plan records PASS for full verify, bench, release, and graph refresh. | Product scope is conformant and no Reader HIGH finding remains. Final dual-agent reconciliation is intentionally not available to this independent first pass, so the requirement's final Reader+Breaker clause cannot yet be closed here. |

## 3. Findings

### RDR-001 — MEDIUM — changed completion arguments bypass operation conflict

- Requirement: REQ-006.
- Files/symbols:
  - `internal/cli/task.go:69-73`, `newTaskCompleteCommand` reads both
    `operation` and `required`.
  - `internal/cli/task.go:88-101`, the terminal replay branch delegates directly
    to `TransitionExpecting` and never compares `required`.
  - `internal/cli/task.go:123-131`, the first allowed delivery records only the
    underlying transition.
  - `internal/coordination/store.go:408-424`, the operation request hash is for
    command `task state` and includes task, actor, destination, reason, and
    expected revision, but not completion's `required` profiles.
- Problem: `--required` changes the evaluated completion request. The public
  idempotency contract says a repeated operation ID answers the same request and
  conflicting content is rejected. Once the first completion succeeds, a retry
  with the same operation ID but a different `--required` list hashes identically
  at the coordination layer and is returned as a successful replay. This is a
  false statement that the changed request is the recorded request.
- Produced black-box evidence, built production binary and disposable Git repo:

| Arm | `required` | Exit | `ok` | `allow` | `replayed` | Error code |
|---|---|---:|---|---|---|---|
| Initial delivery | empty | 0 | true | true | false | none |
| Control: identical retry | empty | 0 | true | true | true | none |
| Scenario: changed retry | `never-proven` | 0 | true | true | true | none |

  Probe command sequence: initialize and commit a disposable repository; open a
  session/task; move it through `CLAIMED`, `IN_PROGRESS`,
  `READY_TO_COMPLETE`; run `task complete ... --operation-id reader2-complete`;
  repeat once unchanged; repeat once with `--required never-proven`.
- Expected: the identical retry replays; the changed request is refused with
  `OPERATION_ID_CONFLICT` (or is represented by a completion-specific request
  hash that includes canonical required profiles).
- Actual: both retries return exit 0 and the first ALLOW response.
- Impact: state safety is not bypassed after the task is already terminal, but
  the public response and replay/conflict contract are wrong for a gate input.
- Confidence: CONFIRMED.
- Class sweep: the mismatch is specific to wrapper parameters not included in
  the delegated store operation hash. Other coordination commands hash the
  parameters their store receives; `task complete` is the new wrapper with an
  extra decision parameter.

### RDR-002 — LOW — task help describes the removed raw-completion ownership model

- Requirement: REQ-007.
- File/symbol: `internal/cli/task.go:17-34` (`newTaskCommand`) and
  `internal/cli/task.go:225-232` (`newTaskStateCommand`).
- Problem: the group help says “Every move goes through `task state --to`” and
  the state help says it is “the only command that changes a task's state”. The
  same command tree now exposes `task complete`, whose purpose is the one
  terminal state transition. `task state --help` also lists `COMPLETED` in the
  `--to` vocabulary without saying that this value is always refused there.
- Produced evidence: the built binary's `task --help` printed the quoted
  statements while listing `complete`; `task complete --help` correctly stated
  that it marks the task COMPLETED after ALLOW.
- Expected: help divides ownership explicitly: `task state` owns non-terminal
  transitions and `task complete` owns the gated terminal edge.
- Actual: one help page describes the current split and two still describe the
  pre-remediation single-command model.
- Confidence: CONFIRMED.

### RDR-003 — LOW — `mcp --json` troubleshooting promises an error code not emitted

- Requirements: REQ-005, REQ-007.
- Files/symbols:
  - `internal/cli/root.go:434-460`, `newMCPCommand` returns a plain `fmt.Errorf`
    when `--json` is present.
  - `docs/usage-tr.md:307-312`, troubleshooting labels the case
    `COMMAND_LINE_INVALID`.
- Produced evidence from the built executable:

| Arm | Exit | stdout bytes | stderr | Structured code |
|---|---:|---:|---|---|
| `mindrail mcp` with SDK client | 0 after client close | protocol frames | none relevant | normal MCP lifecycle |
| `mindrail mcp --json` | 1 | 0 | `mindrail: mcp owns stdout for protocol frames; remove --json` | none |

- Expected: retain zero stdout bytes, but either emit the documented usage
  classification/code on stderr and exit 2, or document the actual plain exit-1
  refusal.
- Actual: stdout purity is correct; the documented code does not exist and the
  exit class is failed rather than usage.
- Confidence: CONFIRMED.

## 4. Scope compliance

The production changes are traceable to the frozen work clusters:

- validation run proof and declarative scope freshness;
- shared completion composition plus canonical reconcile;
- schema-9 durable guard baseline and its callers;
- production MCP stdio composition;
- gated CLI terminal transition and compatibility tests;
- usage and historical-audit addenda.

No unrelated production architecture change was identified. The changes under
`.claude/`, `.gitignore`, `.wrongstack/`, and `.zcode/` are explicitly recorded
as pre-existing user work and were excluded. `graphify-out/` is generated output
from the required refresh. This Reader changed only this report.

Classification:

- UNDER_IMPLEMENTATION: limited to REQ-006 replay conflict semantics and the
  small REQ-007 documentation/help mismatches above.
- OVER_IMPLEMENTATION: none found.
- SCOPE_CREEP: none found in the remediation-owned product surface.
- BEHAVIORAL_DRIFT: RDR-001 is a narrow idempotency drift; no unrelated runtime
  drift was produced.
- REQUIREMENT_MISINTERPRETATION: none found in the four original HIGH fixes or
  the two primary integration gaps.

## 5. Contract and documentation conformance

- Migration contract: schema 9 adds `guard_baselines`; migration load/shape and
  immutable shipped-checksum tests passed. The baseline is keyed by
  project/workspace/HEAD/version and unions canonical mappings under a serialized
  transaction.
- Validation contract: new provenance carries run identity, command index/count,
  declarative scope paths, and the concrete snapshot. Legacy rows cannot satisfy
  coverage.
- Completion contract: CLI and MCP share `completion.Service.Evaluate`; current
  knowledge and canonical reconcile therefore do not fork by transport.
- MCP contract: subprocess discovery and representative call prove the public
  launcher and exact 13-tool surface; version metadata says `stdio`.
- Coordination contract: raw CLI completion is refused and ALLOW commits task
  state plus lease release through the existing atomic transition. The outer
  completion request is not fully represented in the operation hash (RDR-001).
- Documentation: the main usage guide and historical snapshot notes mostly agree
  with code. RDR-002 and RDR-003 are the remaining concrete `DOC_DRIFT` items.
  Two internal comments are also stale but non-behavioral:
  `internal/mcp/server.go:36` says the server binds eleven tools, and
  `internal/mcp/server.go:141-142` still calls process serving future wiring.

No schema/API implementation conflict was found beyond the operation request
identity gap in RDR-001.

## 6. Executed verification

| Command/probe | Result |
|---|---|
| `go test -count=1 ./internal/validation ./internal/gate ./internal/completion ./internal/mcp ./internal/changes ./internal/migration ./migrations ./cmd/mindrail ./internal/cli` | PASS; package times 0.765 s to 43.171 s. |
| `go vet ./...` with writable isolated `GOCACHE` | PASS. |
| `staticcheck ./...` | NOT RUN; executable unavailable. |
| `gofmt -l` over changed Go files | PASS; no paths printed. |
| `git diff --check` over the remediation surface | PASS; no output. |
| `make tidy-check` with writable isolated `GOCACHE` | PASS. |
| `make gate` with writable isolated `GOCACHE` | PASS; all nine categories green with counts recorded under REQ-008. |
| Built-binary MCP subprocess contract (via package suite) | PASS; exact 13 tools and representative bootstrap call. |
| Built-binary changed-argument replay probe | FAIL as expected for audit; produced RDR-001. |
| Built-binary `task --help`, `task state --help`, `task complete --help` comparison | Produced RDR-002. |
| Built-binary `mcp --json` | Exit 1, stdout 0 bytes, no structured code; produced RDR-003. |

The first attempts to run Go commands without a custom cache failed because the
host's default Go build cache is read-only. Re-running with isolated writable
`GOCACHE` passed; this was an audit-environment constraint, not a product failure.

## 7. Open experiments

These are not unproduced bug claims:

1. **RDR-001 proposed-fix measurement.** In a scratch worktree, make the
   completion idempotency identity include canonical `required` profiles (or add
   a completion-specific operation record). Re-run the three-arm table above.
   Acceptance: unchanged retry remains exit 0/replayed true; changed-required
   retry becomes exit 1/`OPERATION_ID_CONFLICT`; fresh ALLOW and DENY behavior is
   unchanged.
2. **Final dual-agent reconciliation.** After the independent Breaker report is
   sealed, reproduce or refute every Breaker finding and then close REQ-008's
   final consensus clause. This Reader report intentionally cannot perform that
   step without destroying independence.
3. **Independent repetition of the longest recorded checks.** The audit plan
   records current-tree PASS for `make verify`, `make bench`, and `make release`;
   this Reader independently repeated targeted suites, vet, tidy and the full
   nine-category gate, but not those three longest commands. Exact repetition is
   `GOCACHE=<writable> make verify`, `make bench`, and native `make release` in a
   clean scratch copy.
4. **Static analysis availability.** Install a compatible `staticcheck` and run
   `staticcheck ./...`; it was not present on this host. `go vet ./...` passed.

## 8. Independent conclusion

**COMPLIANT_WITH_MINOR_ISSUES**

The Reader found positive executed evidence that all four original HIGH
completion defects are remediated, the production MCP launcher works, raw
terminal mutation is blocked, and the gated state/lease transition works. No
CRITICAL or HIGH Reader finding remains. Exact conformance is not perfect:
REQ-006 is partial because completion's `required` profiles are omitted from the
operation identity, and REQ-007 is partial because two live help/documentation
claims disagree with the executable. REQ-008's final team consensus remains for
the parent reconciliation after the independent Breaker report is sealed.
