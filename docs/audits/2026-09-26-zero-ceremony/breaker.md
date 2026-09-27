# Breaker report — zero-ceremony UX

**Verdict: FAIL**
**Confidence: high**
**Audit depth: A**

The untouched implementation behaved correctly in every runtime scenario exercised.
Acceptance still fails because three protective conditions added by this change
survived the existing test suite. In each case a scratch-only discriminating test
failed when the guard was removed and passed after it was restored. These are test
adequacy findings, not claims that the shipping guard currently misbehaves.

I did not read the Reader result or `report.md` before completing this report.

## B0 — isolation and safety

- Full dirty-worktree copy: `/tmp/mindrail-zero-breaker-g1vJvW`
- Historical A/B archive: `/tmp/mindrail-breaker-base-hwFEXy` at `2814acb`
- All source mutations, scratch tests, databases, binaries and Go caches lived under
  `/tmp`.
- No production source file was edited in the original worktree. At the end,
  `cmp -s` matched the original and restored scratch copies of all four mutated
  product files:
  `internal/workflow/service.go`, `internal/setup/setup.go`,
  `internal/index/store_cas.go`, and `internal/mcp/complete.go`.
- Scratch-only breaker tests were never created in the original tree. The only
  original-tree write made by this audit is this report.

## Summary of produced findings

| ID | Severity | Confidence | Produced failure | Acceptance impact |
| --- | --- | --- | --- | --- |
| BRK-001 | Medium | High | A different `run_key` can resume a completed task after the terminal-resume guard is removed, while the full existing workflow + MCP suite stays green. | REQ-003/006 lifecycle recovery rule is not regression-pinned. |
| BRK-002 | Medium | High | An in-repository symlinked hooks parent is accepted after the explicit symlink guard is removed, while the full existing setup + CLI suite stays green. | REQ-002 symlink-safe one-command setup is not fully regression-pinned. |
| BRK-003 | Medium | High | Automatic finalize accepts manual `task_id` and `required` fields after the mixed-mode guard is removed, while the existing MCP suite stays green. | REQ-004/008 public wire-mode separation is not regression-pinned. |

No untouched-product functional failure, race, migration loss, ABA acceptance,
connection-context leak, stdout protocol contamination or backward-compatibility
break was produced.

## BRK-001 — terminal resume guard survives the existing suite

**Guard:** `internal/workflow/service.go:175`,
`t.State.Terminal() && !replay` in `Service.Start`.

**Attempt.** In the scratch copy I changed only the terminal arm to an always-false
condition. I then ran:

```text
GOCACHE=/tmp/mindrail-breaker-cache go test -count=1 \
  ./internal/workflow ./internal/mcp
```

The existing suite remained green. I then added a scratch-only test that:

1. starts `completed-owner` over `a.txt`;
2. changes the file and successfully finalizes that run;
3. calls `Start` with a new key `new-run` and
   `ResumeTaskID=<the completed task>`;
4. requires rejection.

With the guard removed, the new run resumed the completed task and returned it in
`COMPLETED` state. With the original guard restored, the same test passed by receiving
the expected error.

| Arm | Existing workflow + MCP packages | Discriminating scenario | Result |
| --- | ---: | ---: | --- |
| Control, guard present | 2/2 pass | 1/1 pass | Correct refusal |
| Scenario, guard deleted | 2/2 pass | 0/1 pass | Completed task accepted by a new run |

The existing `TestHandoffResumeAndCompletedBootstrapRetry` covers the intentionally
allowed opposite arm: the **same** durable run key replays its own completed bootstrap.
It does not cover a different run key attempting to adopt that terminal task.

**Class sweep.** `rg 'Terminal\(\).*replay|ResumeTaskID' internal/workflow internal/mcp`
found one terminal-resume admission guard. Completed and abandoned tasks share this
`Terminal()` predicate; there is no second admission implementation to patch. The
same-run terminal replay is covered, but the distinct-run terminal rejection is not.

**Required remediation.** Add the discriminating different-run completed-task test
(and preferably the abandoned-task sibling) to `internal/workflow/service_test.go`.
The product guard itself should remain unchanged.

## BRK-002 — in-root hook-parent symlink guard survives the existing suite

**Guard:** `internal/setup/setup.go:172`,
`info.Mode()&os.ModeSymlink != 0` in `readTarget`.

**Attempt.** In the scratch copy I disabled only that explicit symlink check. With no
scratch test present, the complete existing packages stayed green:

```text
GOCACHE=/tmp/mindrail-breaker-cache go test -count=1 \
  ./internal/setup ./internal/cli
```

Observed output was `ok` for setup in `0.003s` and CLI in `46.964s`.

I then created a repository-local alias `custom-hooks -> .git/hooks` and asked setup
to install to `custom-hooks/pre-commit`. This path remains inside the repository, so
`os.Root` containment does not mask the deleted guard. A scratch-only test required
the alias to be refused.

| Arm | Existing setup + CLI packages | Discriminating scenario | Result |
| --- | ---: | ---: | --- |
| Control, guard present | 2/2 pass | 1/1 pass | In-root symlink refused |
| Scenario, guard deleted | 2/2 pass | 0/1 pass | In-root symlink accepted |

The existing `TestHookParentSymlinkRefused` points the parent symlink outside the
repository. That case is also rejected by the containment layer, so it cannot prove
that the explicit symlink guard works. `TestInitRefusesUnsafeOrMalformedManagedTargets`
covers direct AGENTS/hook symlink targets, not an in-root parent alias.

**Class sweep.** `readTarget` is the shared reader for managed AGENTS and hook paths.
AGENTS is always a root-level target and its direct-symlink arm is covered. The
uncovered sibling class is a multi-segment repository-local `core.hooksPath` whose
parent is a symlink but whose resolved destination is still inside the root.

**Required remediation.** Add the in-root parent-alias test to
`internal/setup/setup_test.go` or the CLI integration suite. The product guard itself
should remain unchanged.

## BRK-003 — automatic/manual complete separation survives the existing suite

**Guard:** `internal/mcp/complete.go:64`,
`in.TaskID != "" || len(in.Required) > 0` in automatic finalize mode.

**Attempt.** I disabled only this rejection and ran the existing MCP package suite;
it stayed green. I then added a scratch-only two-arm wire-contract test. Each arm
bootstrapped a fresh automatic run, disconnected, resolved it by `run_key` on a new
connection, and sent one forbidden legacy/manual field with `finalize:true`:

- `task_id=<the durable automatic task>`
- `required=["unit"]`

| Arm | Existing MCP package | task_id case | required override case |
| --- | ---: | ---: | ---: |
| Control, guard present | pass | refusal/pass | refusal/pass |
| Scenario, guard deleted | pass | accepted/fail | accepted/fail |

The mutated output showed both requests completing without an MCP error. Restoring
the guard made both subtests pass. This is the exact compatibility boundary in
REQ-004: automatic callers cannot supply task identity or weaken/replace the configured
profile set, while legacy explicit completion remains evaluation-only.

**Class sweep.** Both forbidden fields behind the same guard were exercised. Search
found no second automatic finalize implementation. Legacy `task_id` behavior is
covered by `TestLegacyCompleteAllowsButDoesNotTransitionTask`; schema exposure of
`finalize` is covered by `TestCompleteSchemaAddsFinalizeMode`. The missing class is
mixed automatic + manual payload rejection.

**Required remediation.** Add both subtests to
`internal/mcp/automatic_contract_test.go`. The product guard itself should remain
unchanged.

## B1 — mutation results

Four representative Depth-A semantic classes were mutated by hand.

| Semantic class | Mutated protection | Existing-suite outcome | Discriminator outcome | Finding |
| --- | --- | --- | --- | --- |
| Workflow lifecycle | non-replay terminal resume rejection | survived | failed mutated, passed restored | BRK-001 |
| Managed setup filesystem safety | explicit symlink-component rejection | survived | failed mutated, passed restored | BRK-002 |
| Public MCP mode separation | automatic finalize rejects `task_id`/`required` | survived | both failed mutated, both passed restored | BRK-003 |
| Migration-010 CAS/ABA | `current != completed.token` stale-removal check | killed | four existing ABA/race tests failed | none |

Deleting the CAS generation/token check was strongly detected by the existing suite:

- `TestIndexFilePostCommitDeletionCannotRemoveNewerRegistration`
- `TestRemoveFileCASRetiresOnlyExactObservedState`
- `TestRemoveFileCASCompetesWithRegistration`
- `TestRemoveFileCASRejectsObserverFromBeforeRecreation`, all four
  `legacy={false,true} × restart={false,true}` arms

The mutated failures included a lost H2 registration, both concurrent contenders
winning, and a pre-removal observer deleting a recreated file. After restoration all
focused schema/CAS tests passed.

## B2 — A/B of deleted behavior

I built the base commit `2814acb` and the dirty feature tree as separate binaries and
ran them on equivalent disposable repositories.

| Same input | Base behavior | New behavior | Requested? |
| --- | --- | --- | --- |
| `mindrail --help` | 12 product commands visible (13 including Cobra `help`) | 5 product commands visible (6 including `help`) | Yes, REQ-001 |
| hidden expert help (`task`, `hook`, `session`) | callable | callable; 3/3 exit 0 | Yes, REQ-008 |
| bare `mindrail verify --json` | exit 2, mode required | exit 0, staged decision | Yes, REQ-009 |
| explicit `verify --staged --json` | exit 0, ALLOW | exit 0, ALLOW | Preserved |
| first `init --json` | schema 8, no managed AGENTS/hook setup | schema 10, managed AGENTS + hook | Yes, REQ-002 and migrations 9/10 |

No unrequested deleted behavior was produced. Expert paths are hidden, not removed;
the explicit staged mode and legacy CLI commands remain callable.

## B3 — written threat model exercised

| Threat / misuse | Concrete exercise | Outcome |
| --- | --- | --- |
| Cross-connection context confusion | Two persistent MCP connections with different keys and overlapping misuse attempt | Refused; identities remained distinct |
| Duplicate/concurrent lifecycle delivery | concurrent bootstrap/scope/finalize plus stale replies under `-race` | Converged; no race report |
| Crash/restart | close first MCP client/server, recreate server, bootstrap same key and finalize | Same durable session/task; one discovered change |
| Dirty-worktree attribution | pre-existing dirt, restoration A→B→A, later drift and out-of-scope changes | Existing workflow/changes tests denied or attributed as specified |
| Hook path escape / overwrite | external hooks path, occupied backup, foreign hook exit, direct and parent symlinks | Untouched product refused/preserved; BRK-002 exposes missing in-root-alias regression test |
| CAS ABA | remove, recreate identical bytes, restart/no-restart and legacy/new writers | All stale observers refused |
| Protocol stdout contamination | 11 malformed MCP/JSON CLI arrangements in subprocess tests | All typed refusals; stdout stayed protocol-clean |

The exact full race commands from the audit plan passed in the scratch copy:

```text
go test -race ./internal/workflow ./internal/changes ./internal/coordination \
  ./internal/completion ./internal/mcp ./cmd/mindrail
PASS (workflow 34.669s, changes 16.321s, coordination 40.546s,
      completion 5.201s, MCP 57.618s, cmd 7.312s)

go test -race ./internal/setup ./internal/cli
PASS (setup 1.010s, CLI 285.696s)

go test -race ./internal/index/... ./internal/migration ./migrations \
  ./internal/workflow
PASS (index 28.925s, migration 10.909s, migrations 3.885s,
      workflow 30.136s; all index subpackages passed)
```

Real stdio coverage also passed:
`TestMCPSubprocessServesThirteenTools`, both
`TestZeroCeremonyEndToEndOverStdio*` arms, and all 11
`TestMCPJSONIsTypedUsageRefusalWithProtocolCleanStdout` arms.

## B4 — upgrade path

The schema-9 path was exercised, not inferred from a fresh database:

1. apply embedded migrations only through version 9;
2. preserve a live legacy `file_index_state` row at attempt 7 and its symbol;
3. confirm generation-aware mutation refuses with `required_version=10`;
4. apply migration 010;
5. confirm the legacy row/symbol remains unchanged;
6. register and remove with generation CAS;
7. confirm a negative generation violates the database constraint.

`TestFileGenerationMutationRequiresUpgradeFromVersionNine` passed. The generic
one-step old-schema upgrade, concurrent process initialization and shipped-migration
immutability tests also passed. ABA across legacy/new writers and store restart passed
all four combinations. No old-data loss or first-run ordering failure was produced.

## B5 — weakest satisfying input

I sent the literal minimum values through the public MCP server in the scratch copy:

| Predicate / discriminator | Weak input | Observed response |
| --- | --- | --- |
| any automatic bootstrap field is present | `goal=""`, `run_key=""` | refused; zero new sessions/tasks |
| content, not mere presence, is required | `goal=" "`, `run_key="x"`, `paths=[]` | refused after trim; zero new sessions/tasks |
| smallest useful automatic start | `goal="g"`, `run_key="k"`, `paths=[]` | accepted; automatic run started |
| finalize must be true | `finalize:false` | refused |
| automatic/manual modes must not mix | `finalize:true` plus one forbidden field | product refused both fields; BRK-003 shows the missing regression test |

No weakest-input bypass was produced in the untouched product.

## B6 — dual readers

The same questions were sent through independent readers:

- **Verify scope:** bare `verify` and explicit `verify --staged` returned the same
  JSON ALLOW decision on the same staged repository.
- **Automatic bootstrap validity:** the MCP presence discriminator activated
  automatic mode for whitespace/empty inputs, and the workflow content validator
  refused them without durable rows. The readers agreed on the final decision.
- **Hook safety:** lexical repository-boundary validation and filesystem-component
  validation both refused outside-root symlinks; the in-root alias specifically
  reached the second reader, producing BRK-002's missing-test result rather than a
  product disagreement.
- **Schema/CAS:** migration ledger version 9 and the index store's required version 10
  agreed before and after migration; generation tokens survived removal/recreation.
- **Completion semantics:** CLI/manual completion and automatic finalize both use the
  shared gate; legacy completion remained evaluation-only and automatic completion
  performed the guarded terminal transition.

No dual-reader disagreement was produced.

## Cost

The automatic finalize path runs validation profiles serially. Let `C` be the total
number of configured commands across all profiles:

- timeout ceiling: `C × 10 minutes`;
- captured process output: at most `C × (64 KiB stdout + 64 KiB stderr)` persisted
  through one evidence result per command, while execution is sequential;
- profile and command counts have no product maximum, so the configuration-derived
  total has no finite global ceiling;
- run keys are capped at 1024 bytes;
- each live automatic MCP connection owns at most one current automatic run and one
  heartbeat watcher; lease TTL is 20 minutes and the renewal interval is
  `20m / 3 = 6m40s`;
- MCP search is separately capped at 50 results.

No cost failure was produced. The unbounded `C` is repository-controlled local
configuration, every command has a hard timeout and process-group kill, execution is
cancelable, and no service-level latency target is stated. It should nevertheless be
documented operationally because, for example, 100 configured commands permit
`100 × 10m = 1000m` (16h40m) before caller cancellation.

## Mechanical rule = automated test

The major mechanical rules are machine checked: five-command root surface, expert
callability, exactly 13 MCP tools, legacy empty bootstrap side effects, legacy
completion semantics, managed-marker preservation, foreign-hook chaining, protocol
stdout, schema checksums, schema-9 upgrade, generation ABA, connection isolation,
restart recovery, dirty attribution and race freedom.

Three mechanical safety/contract rules are not machine-pinned strongly enough; those
are exactly BRK-001 through BRK-003. No additional prose-only rule was found in the
audited task boundary.

## Class sweep

Each finding includes its local class sweep above. Cross-finding sweep result:

- lifecycle admission has one terminal-resume guard; same-key replay is covered,
  different-key terminal adoption is not;
- managed target reads share one symlink-component guard; direct targets and
  outside-root parents are covered, in-root hook-parent aliases are not;
- automatic finalize has one mixed-mode guard containing both manual-field siblings;
  neither sibling was covered before the scratch discriminator;
- file-index registration, completion and removal generation checks have sibling ABA
  tests and were killed by representative mutation.

No second production implementation with the same untested shapes was found.

## Backward compatibility

Concrete compatibility combinations exercised:

- schema-9 database + legacy live file row + symbols → migration 010 → CAS mutation;
- empty legacy `mindrail_bootstrap {}` → read-only, zero coordination rows;
- legacy `mindrail_complete {task_id}` → evaluation-only, task remains OPEN;
- original 13 MCP names → still exactly 13 after optional automatic fields;
- old expert CLI paths (`task`, `hook`, `session` sampled) → still callable;
- explicit `verify --staged` → unchanged while bare verify gains the new default;
- foreign pre-commit hook and existing user AGENTS bytes → preserved across repeated
  init;
- MCP process restart + same opaque run key → same durable identities.

No backward-compatibility break was produced.

## Open items

None. Every reported item was produced in this audit and has a concrete control,
scenario, numeric outcome and class sweep. The acceptance blocker is the three missing
regression tests above; no product-code change is indicated by this Breaker run.

## Post-remediation Breaker recheck — 2026-09-26

This is a narrow, independent recheck of the remediation delta for BRK-001 through
BRK-003. It supersedes the original `FAIL` only for those three test-adequacy
findings; no production behavior was changed for this recheck.

The current dirty tree was copied to the disposable directory
`/tmp/mindrail-remediation-recheck-aybfjz/repo`. Each production guard/operand was
mutated there one at a time, its dedicated regression test was executed, the source
was restored, and the control was rerun. The original working tree was not mutated by
the Breaker.

| Finding | Control, guard present | Scenario mutation | Mutant result | Recheck |
| --- | ---: | --- | ---: | --- |
| BRK-001 | 4/4 terminal × restart arms pass | disable `t.State.Terminal() && !replay` | 0/4 pass | KILLED |
| BRK-002 | 1/1 repository-local parent-alias arm passes | disable `ModeSymlink` rejection | 0/1 pass | KILLED |
| BRK-003 `task_id` | 2/2 connection/reconnect arms pass | neutralize only the `task_id` operand | 0/2 targeted arms pass; both `required` controls still pass | KILLED |
| BRK-003 `required` | 2/2 connection/reconnect arms pass | neutralize only the `required` operand | 0/2 targeted arms pass; both `task_id` controls still pass | KILLED |

Produced failure details:

- `TestNewRunCannotResumeTerminalTask` covers both `COMPLETED` and `ABANDONED`, with
  and without workflow-service restart. The mutant accepted both completed-task
  adoptions and created partial identities for both abandoned-task attempts.
- `TestRepositoryLocalHookParentSymlinkRefusedWithoutWrites` uses
  `custom-hooks -> .git/hooks`, so containment cannot mask the component-symlink
  guard. The mutant accepted the alias immediately.
- `TestAutomaticFinalizeRejectsManualFields` exercises `task_id` and `required`
  independently on both the existing connection and a reconnected client. Each
  single-operand mutant completed only its two forbidden-field arms, while the two
  sibling controls remained green. This proves neither operand is accidentally
  covered by the other.

After restoring every mutation, the focused controls all passed again:

```text
go test -count=1 ./internal/workflow -run '^TestNewRunCannotResumeTerminalTask$'
PASS (0.388s)

go test -count=1 ./internal/setup -run '^TestRepositoryLocalHookParentSymlinkRefusedWithoutWrites$'
PASS (0.002s)

go test -count=1 ./internal/mcp -run '^TestAutomaticFinalizeRejectsManualFields$'
PASS (0.964s)
```

Byte comparisons confirmed that the restored scratch copies of
`internal/workflow/service.go`, `internal/setup/setup.go`, and
`internal/mcp/complete.go` matched the original working tree. No surviving mutant,
over-refusal, new compatibility break, or open remediation item was produced.

### Final post-remediation verdict: PASS

BRK-001, BRK-002 and BRK-003 are closed. Their new regression tests independently
kill the exact guards/operands that previously survived, and the untouched controls
remain green.
