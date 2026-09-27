# Remediation round 2 — independent Reader delta audit

Date: 2026-09-26
Base: `2814acb22d8aa94d48ade1242b58e1fdb14239fc`
Scope: only the round-2 remediation for Reader RDR-001..003 and Breaker
BR-01..05, their callers/class siblings, permanent regression tests, and the
round-2 validation artifact. Requirements already verified and unchanged in the
first Reader pass were not reopened. The Reader did not inspect any round-2
Breaker conclusion before writing this report.

## Delta verdict

**NOT_VERIFIED**

The replay, glob scope, zero-match, partial-run provenance, explicit blank
required profile, Unicode identity, MCP refusal, help text, final CAS, and
persisted guard-baseline fixes are present and pass their focused controls. The
two former mutation survivors are now killed by permanent tests. One public
contract defect remains: the raw-completion refusal's first remedy is invalid in
six of the seven task states, including both terminal states. That leaves the
requested per-state remedy check unverified.

No CRITICAL or HIGH finding was produced.

## Finding

### R2-RDR-001 — MEDIUM — raw completion advertises an invalid primary remedy in six states

- Contract: REQ-006 requires refusal of raw `COMPLETED` with actionable
  guidance. Round 2 additionally asked for valid per-state remedies/help.
- Location: `internal/cli/task.go`, `rawCompletionRefusal`, especially the
  unconditional first `next_action` at lines 302–305 and the state-specific
  branch at lines 306–324.
- Problem: every `task state --to COMPLETED` refusal begins with “Run `mindrail
  task complete ...`”. `task complete` is valid only from `READY_TO_COMPLETE`.
  It deterministically refuses from `OPEN`, `CLAIMED`, `IN_PROGRESS`, `BLOCKED`,
  `ABANDONED`, and `COMPLETED`. For `ABANDONED` and `COMPLETED`, the next entry
  immediately says the task is terminal, contradicting the first remedy. For
  `IN_PROGRESS`, the first state-specific alternative is `BLOCKED` but omits
  that this destination requires `--reason`.
- Produced evidence: a built-binary sweep created a task in each state and
  invoked raw completion. OPEN/CLAIMED/IN_PROGRESS/BLOCKED/READY_TO_COMPLETE/
  ABANDONED all returned the unconditional completion command first; a separate
  clean-gate control created a real COMPLETED task and returned the same command
  followed by “This task is COMPLETED and terminal”.
- Impact: the safety boundary holds and the task/lease are unchanged, but a user
  following the primary recovery instruction receives another predictable
  refusal. The existing permanent test exercises only READY_TO_COMPLETE and
  therefore does not detect the six invalid cases.
- Required correction: emit `task complete` only for READY_TO_COMPLETE. For
  non-ready states, lead with complete, executable lifecycle guidance derived
  from the current state (including `--reason` for BLOCKED), and for terminal
  states offer only inspection/new-task guidance. Add a table-driven test over
  all seven states.

## Previous-finding and Breaker-delta results

| Delta | Verdict | Independent evidence |
|---|---|---|
| RDR-001 / BR-03 — completion operation identity omitted required profiles | VERIFIED | `coordination.Store.CompleteExpecting` canonicalizes required profiles and passes them to `transitionRequestHash`; the JSON request hash includes `required`. Both the normal completion and completed-task replay paths call it. Same request replays; changed required profiles return `OPERATION_ID_CONFLICT`. |
| RDR-002 — stale task help | VERIFIED | Built release binary help now states that `task state` owns non-terminal moves and `task complete` owns the evaluated COMPLETED edge. `task state --help` explicitly says raw COMPLETED is always refused. |
| RDR-003 — `mcp --json` contract mismatch | VERIFIED | `newMCPCommand` returns typed `COMMAND_LINE_INVALID`/usage before starting stdio. Built-binary probe: exit 2, stdout 0 bytes, stderr contains `COMMAND_LINE_INVALID`. `cmd/mindrail/mcp_subprocess_test.go:69` permanently covers it, and `docs/usage-tr.md:311` matches. |
| BR-01 — glob walked unrelated unreadable trees; zero-match scope was nil/stale | VERIFIED | `validation.enumerateScope` starts with a non-nil empty slice; `globWalkRoot` prunes to the deepest safe literal prefix. Focused tests passed for unrelated unreadable trees, unreadable in-scope trees, root-walk patterns, symlink boundaries, escapes, and zero-match literal/glob freshness. |
| BR-02 — partial operation replay split one logical validation run | VERIFIED | `Service.RunProfile` adopts command zero's durable `RunID`, checks profile/count/index/declarative scope/concrete scope, and requires later commands to carry the same run ID. Partial-row replay and tampered provenance tests passed; complete-run coverage remained satisfied. |
| BR-04 — explicit blank CLI required value disappeared; Unicode exactness | VERIFIED | CLI checks `Flag.Changed` plus the empty-slice parse case, then `CanonicalRequiredProfiles` rejects Unicode whitespace via `TrimSpace` without trimming or normalizing nonblank names. It preserves exact spelling, case, and normalization form while sorting/deduplicating exact strings. Focused CLI controls gave exit 2 / `COMMAND_LINE_INVALID` for `--required ""` and a normal evidence denial for the non-ASCII profile `İ`, with no task mutation. |
| BR-05A — final completion CAS mutation survived | VERIFIED | Permanent `TestTaskCompleteFinalCASRejectsRevisionChangedDuringGate` passed in the current tree. In an isolated `/tmp` copy, replacing the final `expect` argument with `0` made that test fail: completion incorrectly exited 0 and persisted COMPLETED revision 6. |
| BR-05B — incomplete persisted guard mapping mutation survived | VERIFIED | Permanent `TestGuardBaselineRejectsIncompletePersistedMappings` passed in the current tree for every field, malformed JSON, and valid controls. In an isolated `/tmp` copy, removing the completeness loop made all four empty-field cases fail because corrupt mappings were accepted. |

## Integration and compatibility assessment

- Completion replay semantics are exact at the operation boundary: task, actor,
  destination, reason, expected revision, command identity, and canonical
  required-profile set participate in the hash. Order-only and exact-duplicate
  changes replay; spelling/case/normalization changes conflict rather than alias.
- Explicit empty and whitespace-only required names are rejected before gate
  evaluation. Nonblank Unicode names are not case-folded, normalized, or
  trimmed. This preserves the existing exact profile-name contract.
- Validation zero-match declarations now serialize as `scope: []`, so membership
  additions change the snapshot rather than making the evidence intrinsically
  stale. Safe literal-prefix pruning does not weaken failure on an unreadable
  subtree that could match.
- Partial validation replay preserves one logical run and rejects incompatible
  persisted provenance before writing the missing command row.
- MCP stdout remains protocol-only. The typed refusal and the user guide now
  agree on code, exit status, and stream ownership.
- The final coordination write still uses the observed revision in its SQL CAS;
  CLI completion supplies the caller's expected revision, and the permanent
  trigger regression proves an intervening revision cannot complete the task.
- Persisted guard mappings fail closed when any path, test, production UID, or
  invariant ID is empty; an empty mapping set remains a valid control.
- No new schema or compatibility regression was found in this delta. Existing
  non-terminal transitions and canonical required-set replay remain compatible.

## Documentation and scope

The round-2 production changes are confined to the named seams. Generated
Graphify changes and pre-existing unrelated `.claude`, `.wrongstack`, `.zcode`,
and `.gitignore` dirt were excluded. `docs/usage-tr.md` now correctly describes
canonical required-set replay, explicit blank rejection, split task-command
ownership, typed MCP refusal, exit 2, and zero-byte stdout. The remaining issue
is in runtime refusal guidance, not the guide or command help.

## Verification evidence

Independent focused runs:

- validation replay/provenance/glob/zero-match selections: PASS;
- guard-baseline persisted-corruption selection: PASS;
- CLI completion/replay/blank-Unicode/final-CAS/raw-refusal selections: PASS;
- MCP built-subprocess launcher and `mcp --json` selections: PASS;
- coordination transition/idempotency selections: PASS;
- `go vet ./...`: PASS;
- `make tidy-check`: PASS;
- `git diff --check`: PASS;
- `staticcheck`: unavailable on this host, so no PASS is claimed.

`verification-round2.md` is internally consistent with inspectable artifacts:
the release binary reports version 0.1.0, base `2814acb`, `dirty`, linux/amd64,
and `mcp_compatibility:"stdio"`; `dist/SHA256SUMS` verifies; `MATRIX.txt` records
linux/amd64 built and the four cross-target Tree-sitter constraint reasons; all
audited source/test mtimes precede the release binary. Its stated `make verify`,
full race, `make gate`, benchmark, and release results were not re-run wholesale
in this bounded delta pass, but the artifact claims are coherent with the
Makefile targets, release outputs, and the independent focused controls above.

The first checksum attempt was run from the repository root and correctly
failed because `SHA256SUMS` contains paths relative to `dist`; rerunning from
`dist` passed. This is a probe-invocation error, not a product failure.

## Open experiment

Only R2-RDR-001 remains open: correct and permanently test the per-state raw
completion remedies. All requested technical experiments and both former
mutation survivors are otherwise closed by produced evidence.
