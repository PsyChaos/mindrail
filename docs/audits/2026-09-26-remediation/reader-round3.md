# Remediation round 3 — independent Reader final delta audit

Date: 2026-09-26
Base: `2814acb22d8aa94d48ade1242b58e1fdb14239fc`
Mode: independent Reader conformance pass. No round-3 Breaker conclusion was
read before this report was written.

## Final delta verdict

**VERIFIED**

Both prior MEDIUM findings are corrected in the current tree. The segment-aware
glob implementation distinguishes path depth and match reachability while
remaining fail-closed for potentially selected unreadable content. Raw
COMPLETED refusals now give a first action that is valid for every state derived
from the domain lifecycle, and the refusal changes neither task nor lease state.
No remaining or new finding was produced in this two-item delta.

## Delta requirements

| ID | Requirement | Verdict | Produced evidence |
|---|---|---|---|
| R3-REQ-01 | Glob traversal must be segment-aware: `tests/*.py` must not descend into an unmatched unreadable nested directory; `tests/*/*.py`, `tests/**/*.py`, matching unreadable content, wildcard-first patterns, and zero-match declarations must retain their intended scope/fail-closed behavior. | PASS | Focused permanent tests passed for wildcard depth, recursive matching, wildcard-first matching, matched directory/file refusal, recursive unreadable refusal, symlink/escape boundaries, and zero-match freshness. An independent scratch control using exactly `tests/*.py` selected only `tests/a.py` while `tests/private/` was unreadable. |
| R3-REQ-02 | Raw `COMPLETED` refusal must derive valid first actions from every domain state: completion only from READY, executable lifecycle transition for other nonterminal states with required flags, inspection/new-task guidance for terminal states, and no task/lease mutation. The D-55 test must derive the lifecycle rather than restate it. | PASS | Permanent all-state test passed for all seven values returned by `coordination.States()`. An independent scratch control substituted real IDs/revisions and executed every advertised first action successfully. Before/after task and lease JSON remained byte-identical across the refusal. Test path construction uses BFS over `coordination.TransitionsFrom`, not a copied lifecycle table. |

## R3-REQ-01 — segment-aware glob assessment

`internal/validation/snapshot.go:125-238` now traverses the pattern one segment
at a time:

- ordinary wildcard segments consume exactly one path component;
- `**` consumes zero or more directory components;
- unmatched entries are discarded before `Info`/descent;
- a matching directory is entered, so unreadability fails closed when it could
  contain selected files;
- a matching unreadable file reaches the normal content hash and is refused;
- symlinks remain untraversed;
- missing literal segments and zero-match globs return a valid non-nil empty
  scope.

Produced controls:

| Pattern/scenario | Expected | Observed |
|---|---|---|
| `tests/*.py`, unreadable `tests/private/`, top-level `tests/a.py` | Ignore nested directory; select one file | PASS; one selected file |
| `tests/*/*.py` | Select exactly one nested level | PASS |
| `tests/*/*.py`, matched unreadable child directory | Refuse | PASS; refused |
| `tests/**/*.py` | Select top-level and arbitrary depth | PASS |
| `tests/**/*.py`, unreadable reachable directory | Refuse | PASS; refused |
| matching unreadable `tests/a.py` | Refuse during snapshot/hash | PASS; refused |
| `*/**/*.py` | Preserve wildcard-first breadth | PASS for selection; unreadable potentially matching subtree refused |
| zero-match literal directory and recursive glob | Valid current empty scope until membership changes | PASS |
| unmatched sibling outside a safe/matching path | Do not affect snapshot | PASS |

The new permanent tests are `TestSnapshotScopeWildcardSegmentsStayAtTheirLevel`
and `TestSnapshotScopeUnreadableGlobBoundaries` in
`internal/validation/snapshot_test.go`, supplemented by the existing zero-match
freshness and selection-boundary tests.

## R3-REQ-02 — state-derived remedy assessment

`internal/cli/task.go:302-340` now divides the remedies correctly:

- `READY_TO_COMPLETE`: first action is `task complete` with session and expected
  revision placeholders;
- terminal states: first action is `task show`, followed by opening a new task;
- other nonterminal states: the first domain transition that is neither raw
  COMPLETED nor BLOCKED is offered with its required session flag. For the
  current D-55 graph this yields CLAIMED, IN_PROGRESS, READY_TO_COMPLETE, and
  IN_PROGRESS respectively for OPEN, CLAIMED, IN_PROGRESS, and BLOCKED.

The permanent `TestRawCompletionRefusalIsStateSpecific` does not restate D-55.
It enumerates `coordination.States()`, constructs each reachable state using a
BFS over `coordination.TransitionsFrom`, and compares the refusal against an
action derived from the same public domain APIs. Its fallback includes
`--reason` if a future state's only executable nonterminal edge is BLOCKED, so a
lifecycle change that makes production guidance incomplete will turn the test
red.

Independent execution in a scratch copy went beyond string comparison: the
first action was run with real task/session/revision values in OPEN, CLAIMED,
IN_PROGRESS, BLOCKED, READY_TO_COMPLETE, COMPLETED, and ABANDONED. Results were
7/7 successful. The raw refusal's pre/post task and lease JSON was identical in
all seven cases.

## Scope, contracts, and documentation

The production delta is limited to the two named seams. The new tests cover the
same behavior without duplicating the lifecycle table. No schema, wire-shape,
state-machine, or documentation change was needed. Pre-existing unrelated dirty
files and generated Graphify output were excluded from the conformance judgment.

No backward-compatibility regression was found: existing literal scopes,
recursive globs, wildcard-first declarations, symlink exclusions, zero-match
evidence, lifecycle states, refusal code/exit class, and non-mutating task/lease
semantics remain intact.

## Final verification evidence

Independent executions in this pass:

- focused validation edge matrix: PASS;
- focused raw-completion all-state tests: PASS;
- exact `tests/*.py` unreadable-nested scratch control: PASS;
- executable first-action scratch control over all seven domain states: PASS;
- `make gate`: PASS with exactly unit 1345, domain 420, integration 10, race 9,
  knowledge-schema 216, MCP-contract 34, Git worktree 77 plus named 7, SQLite
  concurrency 17, E2E 10, and smoke 5;
- focused `go vet` for the two changed packages: PASS;
- `make tidy-check`: PASS;
- `git diff --check`: PASS.

`verification-round3.md` is consistent with the current artifacts:

- it records `make verify` PASS, including the normal full suite, full
  `go test -race ./...`, vet, and command smoke;
- all audited production/test files predate the release binary, so that result
  covers the inspected source;
- the native binary checksum verifies and reports version 0.1.0, base
  `2814acb`, `dirty`, linux/amd64, and MCP stdio compatibility;
- `MATRIX.txt` records linux/amd64 built and gives the expected Tree-sitter
  build-constraint reason for every best-effort cross target;
- the independently repeated category gate reproduced every published count;
- the prior benchmark record remains applicable because neither round-3 path is
  part of those benchmark targets.

The full multi-minute `make verify`/full-race run was not redundantly repeated by
this Reader; its final execution record, post-source release artifacts, exact
gate reproduction, and focused positive/negative controls were cross-checked
instead. `staticcheck` is not installed on this host, so no staticcheck PASS is
claimed.

## Open items

None for this final two-finding delta.
