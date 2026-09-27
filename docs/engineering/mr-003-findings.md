# MR-003 — findings

- **Requirements frozen at** `cd74767`, before any code: [mr-003-requirements.md](mr-003-requirements.md)
- **Design**: [mr-003-design.md](mr-003-design.md)
- **Implementation**: `f7ac67f..ce0c708`
- **Audit round 1**: graded `cd74767..f51478e`. 7 auditors, 107 agents, 50
  proposed, 46 confirmed, **27 distinct defects — 5 HIGH, 15 MEDIUM, 7 LOW**.
  The evidence is [mr-003-audit-round-1.md](mr-003-audit-round-1.md); §5 below is
  the summary.
- **Round-1 remediation**: all fifteen items of the audit's brief, `c29efa3`
  through `e7e946c` — thirteen commits, items 8–10 sharing one. §6 below is the
  record.
- **Audit round 2**: graded the round-1 remediation. 9 auditors; 35 proposed,
  30 distinct, **17 confirmed — 0 HIGH, 4 MEDIUM, 13 LOW**; all 27 of round 1's
  defects closed in the code; twelve of the seventeen are the pass
  over-describing its own fixing, six of them a false sentence in the record;
  one user-reachable regression. The evidence is
  [mr-003-audit-round-2.md](mr-003-audit-round-2.md).
- **Round-2 remediation**: all nine items of round 2's brief, `0f94e23` through
  `0ea96d5`, then a two-agent verification pass and the two fixes it produced,
  `dd93a1d` and `db7647d`. §7 below is the record.
- **Status: remediated twice; no round 3 proposed.** Round 2 found zero HIGH,
  and its remediation is small enough that a third 85-agent round would not pay
  for itself. The next gate is per task in MR-004, not per milestone.
- `make check` green, `make verify` green, `make tidy-check` green
- **807** top-level test functions, from 799 after the round-1 remediation, 768
  at the audit and 732 at the freeze (`go test -list '.*' ./...`, the method
  R4-M11 settled)

Sections 1–4 are the **implementer's own account**, written before the audit so
that the audit had something to grade against. They are left exactly as they were
written, including the places round 1 proved them wrong — §5 says which, and a
record that quietly corrected itself would destroy the only evidence of what an
unaudited pass believes about itself.

---

## 1. What the milestone does now

`mindrail session open` mints an agent session and prints its id.
`mindrail task open --title … --session …` opens a task. `mindrail task state
<id> --to <STATE>` moves it through spec §58's lifecycle, and it is the only
command that does. `mindrail checkpoint write <id> --note … --handoff` leaves the
handover note. `mindrail task show <id>` returns the task, its state and the
newest note with the session that wrote it; `mindrail task list` returns the
project's tasks, newest first.

The scenario the task list names is a test:
`TestTwoSequentialAgentsContinueOneTaskAcrossAProcessBoundary` in
`internal/cli`. Nothing is carried between its two halves in Go — no shared
store, no cached row, no open handle. Every step is a command invocation that
starts the application, does one thing and shuts the database down again, which
is what a second agent in a second process actually gets.

`status --json` gained one additive block, `coordination`. It cannot move
readiness (decision D-62) and both arms of that are asserted.

---

## 2. Two defects the implementation found in itself

Recorded because both were found by running the thing rather than by reading it,
and because the second is the more useful of the two.

### The commands could not write, against a database whose permissions were fine

The first draft ran the coordination commands in `ModeReadOnly`, on the argument
that decision D-01's authority is about *creating* the database and that
appending a row to a table `init` already made exercises none of it. The argument
is right and the mode was wrong: `ModeReadOnly` also opens the SQLite handle
read-only, so every coordination command failed with
`RUNTIME_PATH_UNWRITABLE` — a remedy telling the user to restore write
permission on a file that already had it.

It was found by building the binary and running the handover by hand, and by
nothing else: the package tests at that point exercised the store directly.
`ModeWrite` is the fix — open for writing, create nothing, migrate nothing,
register nothing — and reverting it turns
`TestTwoSequentialAgentsContinueOneTaskAcrossAProcessBoundary` red with exactly
the message above.

**Generalises:** *a mode that bundles two authorities will be wrong for the first
caller that needs one of them.* `ModeReadOnly` meant "does not create" and "does
not write" at once, and MR-003 is the first milestone that needs the second
without the first.

### `WORKSPACE_NOT_INITIALIZED` could not be reused, and finding out why added a code

AC-06.7 asked for the existing code, and the existing code is right about the
condition — the repository has not been initialised. It is decision D-03's single
zero-exit row, and the exit-class table's own comment is explicit that such a code
"must never appear in an error object at all". A command that was asked to open a
task and did not open one has not reported a state; it has failed, and exiting 0
there would make `mindrail task open && …` run its second half against a task
that does not exist.

So `COORDINATION_UNAVAILABLE` was added, with the same remedy sentence. The
requirements' AC-05.1 said five new codes and there are six.

*Corrected during the round-1 remediation (finding F15).* This paragraph used to
end "the amendment is recorded in mr-003-requirements.md", and it was not: that
file's diff over the whole milestone was three hunks, all in §1, and
`grep -c COORDINATION_UNAVAILABLE` over it returned 0. The criterion genuinely
deviated from is AC-06.7, which names the code the commands return; AC-05.1
names five values that must exist and all five do. The amendments are in the
requirements now, in place under AC-05.1 and AC-06.7, in the form the other
amendments use. Five auditors met the false clause fresh and graded it from LOW
to HIGH, which is what one wrong cross-reference costs under a parallel audit.

---

## 3. Mutations run, and what each turned red

Requirement AC-10.3 makes every new guard's falsifiability a condition of the
work rather than a claim about it.

| Mutation | Turned red |
|---|---|
| `newGraph`-equivalent for this milestone: the state table's `BLOCKED` row gains a jump to `COMPLETED` | `TestEveryOrderedPairOfStatesGetsTheAnswerTheSpecificationGives` — `CanTransition(BLOCKED, COMPLETED) = true, want false` |
| The release path `CLAIMED → OPEN` is deleted from the table | the same test, from the other side |
| `Transition`'s `CanTransition` guard is removed | `TestEveryTransitionTheTableRefusesLeavesTheRowUntouched`, on the first illegal pair |
| `LastCheckpoint` orders by `created_at` instead of `checkpoint_id` | `TestTheLastCheckpointIsTheNewestIdNotTheNewestTimestamp` — returns the *first* note. *(Renamed `TestATimestampTieDoesNotDecideTheLastCheckpoint` during the round-2 remediation, §4.4: it tells `created_at` from an id, and cannot tell an id from `rowid`.)* |
| `WriteCheckpoint` stops verifying the session | `TestAnUnknownSessionIsRefusedByEveryWriter` — the foreign key answers instead, with `COORDINATION_WRITE_FAILED` |
| Leaving `BLOCKED` no longer clears the reason | `TestABlockCarriesItsReasonAndLeavingClearsIt` |
| The commands run in `ModeReadOnly` again | the handover acceptance test, with `RUNTIME_PATH_UNWRITABLE` |
| `identity.nextEntropy`'s backwards-clock clamp is removed | `TestABackwardsClockStillMintsAscendingIds` |
| The entropy counter's byte carry is removed | `TestTheEntropyCounterCarriesAcrossAByte` |
| `RenderError` drops its "Next" section — `return nil` in place of `renderSection(w, "Next", …)` in `internal/app/errors.go` | `TestTheFourCoordinationRefusalsAgreeAcrossBothRenderings` — all four rows, on the human rendering. `TestACoordinationCommandNamesAHaltedStartupTheWayStatusDoes` stays green under it, and goes red alone under the reverse (deleting the `Diagnosis` call), so the two falsifiers are disjoint |

*Extended during the round-2 remediation (round 2, §4.6).* The last row was added
then. The round-1 remediation's item 12 added the AC-09.1 matrix as a test-only
change and recorded "reverting item 2" as its falsifier — which falsifies item 2's
own test and none of the four rows, so the guard had no recorded falsifier at all.
The mutation above was run in both directions before it was written down.

---

## 4. What an audit should look at first

Stated plainly, because a document that lists only what was done reads as one
that looked everywhere.

- **Concurrency between two coordination writers is untested.** MR-002's coverage
  pass established that two *readers* keep a coherent envelope; this milestone
  adds writers and nothing runs two of them against one task at the same time.
  That is MR-004's subject — leases, `operation_id`, optimistic revision — but the
  gap exists now, and the current answer is "last writer wins", which no test
  states.
- **The state machine is enumerated; the store's use of it is not.** All 49
  ordered pairs are graded at both layers, but always from a task reached by the
  one route `pathTo` writes down. A task that arrived at `IN_PROGRESS` through
  `BLOCKED` is never tested against the same pairs.
- **`Summarize` is graded on one project.** Nothing asserts that a second project
  in the same database does not contribute to the counts, and the fixture cannot
  produce one — `openFixture` registers a single worktree.
- **~~The human renderings are goldens, not assertions.~~ Four of the human
  renderings are never executed.** *(Corrected during the round-1 remediation,
  finding F17.)* There was no golden for any coordination command: the five
  files in `internal/cli/testdata/` were all MR-001's, and `TestHumanOutputGolden`
  looped over `{"status", "doctor"}`. The gap was worse than "printed and
  unread": replacing the entire body of `sessionResult`, `handoverResult`,
  `taskListResult` and `checkpointResult` with `panic()` left every package
  green and `make smoke` green, because the code path was never entered at all.
  A panic, a nil dereference or an error-return regression in the default output
  of `session open`, `checkpoint write`, `task show` and `task list` would have
  shipped. The four commands are in `TestCoordinationHumanOutputGolden` now, a
  test of their own that executes them to produce the bytes it pins;
  `TestHumanOutputGolden` still loops over `status` and `doctor` only. *(This
  sentence credited the wrong test until the round-2 remediation, §4.10.)*
  `taskResult`, shared by `task open` and `task state`, was executed already.
- **`internal/identity` moved code that MR-001 shipped.** The tests moved with
  it and the format is asserted, but nobody has graded the move itself against
  the original.
- Everything MR-002's Appendix F left open is still open: the unaudited
  `record`, `schema`, migration, storage and workspace packages, and doctor's six
  non-knowledge checks.
- **Four mutations of the store still survive** *(added by the round-1
  remediation, finding F21)*. §3's table is not "every new guard", and the
  Definition of Done's item 3 is therefore not met in full. What is left, after
  the two the remediation closed:

  | Surviving mutation | Site | Why it is still open |
  |---|---|---|
  | `OpenSession`'s empty-workspace guard deleted | `store.go` | The CLI never calls it with an empty workspace: `coordinationScope` refuses first. Reachable only from a future caller |
  | The four `.UTC()` normalisations dropped | `store.go` | An inherited pattern — the same mutation survives on `internal/workspace/store.go` — and the fixtures are already UTC, so no test can see it |
  | `Summarize`'s count query loses its project scope | `store.go` | The gap named two bullets above: `openFixture` registers one worktree, so a second project cannot be arranged |
  | The newest-checkpoint join loses its project scope | `store.go` | The same fixture limitation |

  `requireSession`'s empty-id guard is struck from the list rather than left
  open: with the guard deleted the SELECT matches no row and produces the
  byte-identical `SESSION_NOT_FOUND`, so it is an equivalent mutant that no test
  could kill.

---

## 5. Audit round 1

Seven auditors graded `cd74767..f51478e` in parallel — three Readers walking the
frozen contract forward, four Breakers attacking the binary from isolated git
worktrees — with two adversarial verifiers per finding and an arbiter on every
split. 107 agents. They proposed 50 findings; 46 survived verification; because
the seven worked without seeing each other, those 46 are **27 distinct defects**.
Three were refuted, all three for the same reason: a sentence graded outside the
scope it states for itself.

The full round, with every finding's evidence, the refutations and a fifteen-item
remediation brief, is [mr-003-audit-round-1.md](mr-003-audit-round-1.md).

### The verdict, in one paragraph

**The handover works; the explanations do not.** Two agents in two processes
continuing one task runs end to end, and the store underneath it held: a Breaker
attacked the state machine five ways and could not make it write on a refusal,
could not make two claimants both win, and could not make one project's tasks
leak into another's counts. Two of the gaps §4 above admitted were tested and the
code held — twenty in-process races, a cross-process race and eight concurrent
`checkpoint write` processes all serialise correctly, exactly one claimant wins,
and the loser gets a clean `TASK_STATE_INVALID`. What is wrong is **what the new
commands say when anything else about the repository is wrong**, and a set of
guards no test can turn red.

### The five HIGH findings

| What is wrong | Where |
|---|---|
| Every coordination command discards the startup verdict and collapses six repository conditions — corrupt database, missing git, unparseable `config.toml`, bare repository, not a repository, unwritable runtime path — into one of two sentences, with `mindrail init` as a remedy that cannot clear any of them | `internal/cli/coordination.go:92` |
| A `--json` write whose title or note is not valid UTF-8 **commits the row and then reports failure**: the refusal happens in `emit`, after the command body ran. The caller's correct response — retry — opens a second task | `internal/cli/coordination.go:57-81` |
| One such task then permanently disables `task list --json` and `task show --json` for that project, and no verb can edit or delete a title | `internal/cli/representable.go:63` |
| The D-55 single-statement guard walks the whole filesystem, so any nested checkout or `git worktree add` inside the tree turns `make check` red and accuses the file it exists to protect | `internal/coordination/lifecycle_scan_test.go:41` |
| "The newest checkpoint" is a lexical comparison of minted ids, and that monotonicity is **process-local**: two agents writing in the same millisecond hand the arriving agent a coin flip. Concurrency is safe for the writes and unsafe for the read that decides which write was last | `internal/coordination/store.go:305-321` |

### What the round says about this document

Round 1 graded §§1–4 above as claims, and three of them did not survive.

- §2 says the sixth error code's amendment "is recorded in
  `mr-003-requirements.md`". It is not:
  `grep -c COORDINATION_UNAVAILABLE docs/engineering/mr-003-requirements.md`
  returns 0. AC-05.1 still says five codes and AC-06.7 still names the superseded
  one. Found by five auditors independently, graded HIGH by one and argued down
  to LOW by every verifier who examined it.
- §3's mutation table is honest — nine of nine re-confirmed — but it is nine of
  nineteen guards. Ten further mutations of the store leave the whole suite
  green, including one that stops `updated_at` advancing.
- §4 says "the human renderings are goldens". There are no goldens for any of the
  six new commands, and the smoke suite executes four of them not at all.

### The three rules this round adds

1. **A command that runs after a failed startup must say what the startup said.**
   `bootstrap.App.Run` deliberately calls the body when `Start` failed and names
   the obligation in its own doc comment. Three commands ask `Diagnosis`; the
   fourth family does not. The rule is not "consult the verdict" — it is that a
   code path which can be entered after a failure has to be written as if it will
   be, because the caller cannot see the WARN line on stderr.
2. **A write that has already committed cannot be reported as a failure.** Two
   independent defects, one fix: judge everything that can be judged before
   opening anything, and let what cannot be judged early live inside the
   transaction it belongs to. `init` paid for this rule once already and wrote it
   down in the source; the new commands did not inherit it.
3. **Asserting the code is not asserting the cause.** Four mutations survived the
   whole suite for one reason: the test reaches the guard from a state where
   several different guards would produce the same code. A guard's test has to be
   built from the state that reaches that guard and no other.

And one about auditing rather than about code, recorded because this was the
first round here run with seven parallel auditors: **documentation drift inflates
under parallel audit.** One wrong cross-reference was found five times and graded
HIGH, MEDIUM, MEDIUM, MEDIUM and LOW. A synthesis step that merges before grading
is not a nicety at that width; without it the round's headline severity would have
been set by the fifth auditor to meet a `grep`.

---

## 6. The round-1 remediation

All fifteen items of the audit's remediation brief, in the order it gave. Every
item's stated mutation was applied and confirmed **red** before the suite was run
and confirmed green, because a green suite over a fix is this repository's most
frequently produced false claim.

`make check`, `make verify` and `make tidy-check` are green. 799 top-level test
functions, from 768 at `3f5ab3e`, counted with `go test -list '.*' ./...`.

### What changed, and what proves it

| # | Findings | What the fix does | The mutation that turns the new test red |
|---|---|---|---|
| 1 | F20, F12 | The D-55 lifecycle guard skips dot-prefixed directories, `vendor`, `testdata`, `graphify-out` and any nested checkout. One `internal/moduletree` for the three module-wide walkers under `internal/` | a file in `internal/cli` spelling all seven states — named, red. From a clone with `git worktree add ./mode-b` — green |
| 2 | F09, F29, F36, F43 | `runCoordination` takes the startup verdict from `Diagnosis` and returns it before `coordinationScope` is consulted | reverting the `Diagnosis` call — six conditions × six commands red on the code, the exit class and the remedy |
| 3 | F28, F42, F30 | `--title`, `--note`, `--reason` and `--label` are refused for UTF-8 at the flag boundary under `--json`, before anything opens. `unrepresentableError` gained a stored-field form that names the member | removing the boundary refusal — `wrote 1 row(s) into tasks, want 0`. Collapsing the two error forms — the remedy tells the reader to rename a path for a column |
| 4 | F37, F24 | "The newest checkpoint" is `ORDER BY rowid`, the order the database assigned | `ORDER BY checkpoint_id DESC` — `LastCheckpoint`, `Handover` and `Summarize` all red |
| 5 | F02, F11, F31, F38 | The session is resolved inside the transaction that attributes the write (`Attribution` / `Write`); empty title and note are judged at the CLI boundary | restoring the eager mint — `a refusal that reports "COMMAND_LINE_INVALID" minted 1 session(s)`. Minting outside the transaction — deadlock |
| 6 | F10, F03, F39, F45 | `init` reads the coordination summary too | removing the one line — `init` and `status` publish different blocks, `observation: not_observed` |
| 7 | F01, F41 | The workspace lookup is skipped only when the migration that creates the table is pending; a reading nobody made is marked as no reading | the blanket `PendingCount > 0` skip — an upgraded repository is told it is not registered. Dropping the marker — `observed` over a query that could not run |
| 8 | F32 | (test only) all six commands from an unregistered linked worktree, on the code *and* the branch | deleting the `space.ID == ""` branch — `task show` returns another worktree's task at `ok:true` |
| 9 | F40 | (test only) `ModeWrite` creates, migrates and registers nothing | `creates()` returning true for `ModeWrite`, and the same for the migrator and the registration — three separate reds |
| 10 | F04 | (test only) a write failure is named by the storage layer first | removing the `storage.WriteFailure` call — all four writers red on `COORDINATION_WRITE_FAILED` |
| 11 | F21 | An accepted move advances `updated_at`; abandoning keeps the claimant. Not test-only: the same commit gave `Attribution` a `mint` flag, made `attribute` dispatch on it instead of on an empty handle, and moved what `MintFor("")` answers from `SESSION_NOT_FOUND` to the no-workspace error — `COORDINATION_UNAVAILABLE` once item 13 coded it. *(Corrected during the round-2 remediation, §4.14: this row said "(test only)".)* | `updated_at = updated_at`, and adding `ABANDONED` to the clearing branch |
| 12 | F13, F35 | A coordination agreement matrix: AC-09.1's four conditions asserted across the envelope, the human rendering and the exit code, with the printed remedy carried out | `RenderError` dropping its "Next" section — all four rows red, item 2's halted-startup test green. *Corrected during the round-2 remediation (§4.6): this cell said "reverting item 2 — the startup rows red", a mutation that fails item 2's own test and none of these four* |
| 13 | F44, F46, F26 | `COORDINATION_READ_FAILED` wraps the read paths; `CoordinationErr` gives the block `indeterminate`; the empty-workspace guards are coded | Unwrapping `FindTask`'s scan — `task show` publishes `code: ""`; unwrapping `ListTasks`'s scan — `task list` does. Each red on its own, since the round-2 remediation drove a damaged `tasks` row through both. *(Corrected during the round-2 remediation, §4.2: this cell said "reverting one read wrap — `code: """ for ten wraps, and round 2 showed no test reached any of them — the sweep's rows were all answered by item 2's startup verdict before a store read ran.)* Removing the indeterminate route — `not_observed` over a read that ran |
| 14 | F33, F34, F22, F48, F49 | The usage envelope publishes the full command path; a group named without a subcommand is an envelope under `--json`; the self-transition refusal names the claimant; the project-scoped checkpoint query stops sorting the project; the "Last checkpoint" line has a subject | `cmd.Name()`, help under `--json`, the dropped `claimed_by`, the join instead of `EXISTS` — each red |
| 15 | F15, F05, F14, F23, F47, F06, F25, F18, F16, F17, F07 | The ledger, plus two things it turned out to be cheaper to fix than to record: AC-03.5 is now enforced by an allow-list, and the four unexecuted renderings have goldens | `internal/coordination` importing `internal/git` — red. `sessionResult.RenderHuman` panicking — red, where before it was green in 18 packages and in `make smoke` |

### Where this pass departed from the brief, and why

Three items were carried out differently from the letter of the brief. Each is a
judgment a second audit should grade rather than take on trust.

- **Item 10's test lives in `internal/coordination`, not in the command layer.**
  The brief says "make the database unwritable and assert all four writers
  report `CodeRuntimePathUnwritable`". Driven through the CLI that assertion now
  passes for the wrong reason: item 2 made the startup verdict come first, so an
  unwritable runtime path is caught before any store method runs, and the
  mutation the brief names — removing `storage.WriteFailure` from
  `(*Store).writeFailure` — leaves the CLI test green. The store-level test
  reaches the code under test and dies on that mutation.

- **Item 13's "make `emit` refuse to publish an envelope whose payload carries
  no registered code" is enforced as a test, not as a runtime branch.**
  `app.WriteJSON`'s own comment argues that dropping an uncoded error is worse
  than reporting it uncoded, and the alternative — inventing a general
  "internal defect" code — is a wire-vocabulary addition this brief did not ask
  for. `TestNoCoordinationFailureReachesTheWireUncoded` drives `task list --json`
  over all of the MR-001 agreement matrix instead, which is the sweep that found
  F09 in the first place.

- **Item 14's migration comment is not corrected.** `migrations/000002_coordination.sql`
  mis-attributes which command runs which query, and the migrator checksums the
  file it applied: editing an applied migration reports
  `MIGRATION_CHECKSUM_MISMATCH` on every existing database, which is the loudest
  possible failure for a corrected comment. The correction is in
  `internal/coordination/store.go`, on the constant that holds the query, with
  the reason.

  F48's index is also not added, and does not need to be: the query was rewritten
  from a join to an `EXISTS`, which changes the plan from "walk every task of the
  project, then sort" to "walk the checkpoints backwards and stop at the first
  one" — from 7–29 ms to 10 µs at 20,000 tasks, with no schema change.
  `TestTheNewestCheckpointQueryDoesNotSortTheProject` asserts the plan rather
  than the timing.

  *Corrected during the round-2 remediation (§4.11, §4.13).* This paragraph said
  "1.2 ms to 10 µs at 20,000 tasks"; 1.2 ms is round 1's figure at **1,000**
  tasks, copied from the wrong row of its table, which puts 28.98 ms at 20,000
  for the whole of `Summarize`; round 2's isolated re-measurement of the join
  put it at 7.3 ms (min) to 19.4 ms (mean). The same wrong figure sat in
  `store.go`'s comment and in the regression test's rationale, and all three are
  corrected. The paragraph also recorded only the winning half of the trade:
  when the queried project has **no** checkpoints and the database carries
  another project's history — reachable by moving a repository directory and
  re-running `init`, which mints a second project row over the same
  `.git/mindrail` — the reverse scan probes every checkpoint and matches none,
  about 2.3 µs per checkpoint (45 ms at 20,000) on every read-only startup until
  that project's first checkpoint is written, where the join cost nearly
  nothing. The window closes itself and the cost at plausible sizes is under
  5 ms, so it is recorded rather than fixed.

### What this pass did not close

- **Four store mutations still survive**, listed in §4 with the reason each is
  still open. Two need a fixture that can register a second project.
- **The two remaining `commandsUnderTest` invariants** —
  `TestNoDocumentContradictsItsOwnErrorObject` and
  `TestEveryRemedyAboutAFileNamesThatFileAbsolutely` — still do not cover the
  coordination commands. Recorded under AC-09.1 with the reason each is vacuous
  there.
- **Everything MR-002's Appendix F left open**, plus `internal/workspace` and
  `internal/migration`, which round 1 added.
- **Linux only.** No macOS or Windows path behaviour has been exercised by
  anyone, in any round.

---

## 7. The round-2 remediation

All nine items of round 2's brief (`mr-003-audit-round-2.md` §8), plus one
carried-forward test repair the brief named but did not number. The commits run
from `0f94e23` to `0ea96d5` inclusive: twelve, of which eleven are items — one
each, item 9 being two — and one (`043457e`) is round 2's own record entering
the tree. After them come this record (`280db02`), the two fixes the
verification pass below produced (`dd93a1d`, `db7647d`), this section's update
and the graph refresh. Items 2, 3, 5 and 7
were written by one subagent each in its own worktree and item 9 was decided and
written by a second model; every mutation below was re-run by the orchestrator
in the main tree before the commit was taken, whoever wrote the fix. Sections
1–4 and 6 are corrected in place with a marker where round 2 proved a sentence
wrong; nothing in them was rewritten silently.

`make check`, `make tidy-check` and `make verify` are green. **807** top-level
test functions, from 799 at `24069aa` (`go test -list '.*' ./...`): six from
the nine items, two from the verification pass.

### What changed, and what proves it

| # | Round 2 | What the fix does | The mutation that turns the new test red |
|---|---|---|---|
| 1 | §4.1 | `coordinationScope` refuses a database whose ledger is short of the coordination migration with `MIGRATION_FAILED` / "Schema is behind this binary" and `mindrail init` as the remedy, after the workspace lookup that F01 requires to succeed. `coordination.TableSchemaVersion` is the second constant of `workspace.TableSchemaVersion`'s kind | removing the check — `task list` answers `COORDINATION_READ_FAILED` and `session open` `COORDINATION_WRITE_FAILED`, both sending the reader to `mindrail doctor`; `TestAWorktreeRegisteredByAnOlderBinaryIsStillRegistered` stays green either way |
| 2 | §4.2 | `TestNoCoordinationFailureReachesTheWireUncoded` gains a second sweep: a `tasks` row with an unparseable `created_at`, driven through `task show` and `task list`. Test-only; no `emit` backstop, because the sweep reaches both sites | unwrapping `FindTask`'s scan — `task show` publishes `code: ""`; unwrapping `ListTasks`'s scan — `task list` does; each on its own |
| 3 | §4.3 | `TestTheUnrepresentablePathRemediesBothWork` runs the refusal first and asserts the location form's code, its message ("in this report") and that the second `next_action` names a rename, before renaming | both `locationTypes` entries `false` — red on the message, `TestAStoredValueIsNotReportedAsAPath` green |
| 4 | §4.4 | AC-04.4 and design §4 amended in place to `ORDER BY rowid DESC`; the same-timestamp test renamed `TestATimestampTieDoesNotDecideTheLastCheckpoint` and its comment says what it cannot tell apart; the F37 test's comment binds it to AC-04.4 | `ORDER BY checkpoint_id DESC` at both sites — the F37 test red on all three readers, the renamed test green; `ORDER BY created_at DESC` — both red |
| 5 | §4.5 | `moduletree.SkipDir` escapes the root before the name check; `lifecycle_scan_test.go` and `storage/arch_test.go` fail when the walk inspected nothing; the moduletree fixture's root is now `.mindrail` | the one-line move reverted — `SkipDir(…/.mindrail) = true, want false`. In a copy of the tree under a dot-named directory with a seven-state file planted in `internal/cli` and a driver import in `internal/doctor`, both guards fire; before the fix both packages were `ok` |
| 6 | §4.6 | §3 and §6 record the falsifier of the AC-09.1 matrix; nothing executable | `RenderError` dropping its "Next" section — all four rows red, the halted-startup test green; the reverse mutation flips exactly which one fails |
| 7 | §4.8 | `TestANeverGivenWorkspaceIsRefusedByNameRatherThanByAConstraint` pins both `noWorkspace` sites; `TestARefusedWriteMintsNoSession` asserts `metadata.flag`, the impact sentence and the `--help` remedy for the two `requireFlagText` boundaries | deleting `attribute`'s guard — `COORDINATION_WRITE_FAILED` for `COORDINATION_UNAVAILABLE`; deleting `requireFlagText` from `task open` — `metadata.flag = ""`; the same from `checkpoint write` — the same, each alone |
| 8 | §4.9–§4.11, §4.13–§4.17 | The ledger: the golden test's name, the 1.2 ms figure in three places, row 11's "(test only)", AC-04.6's "all six", the task list's "on beş commit", the F37 fixture's ids (26 characters, one millisecond), the `EXISTS` rewrite's empty-project cost, `migrations/README.md` | nothing executable in the ledger itself. Added beside it, because §4.17's weight is in its last clause: `TestAnAppliedMigrationFileIsNeverEdited` pins both shipped files' sha256 — editing one word of the `.sql` comment turns it red, where before `go test ./...` was fully green |
| 9a | §4.7 | A precondition read that fails inside a write transaction is `COORDINATION_READ_FAILED` naming the row that could not be read — `requireSession`'s scan, and the task reads in `Transition` and `WriteCheckpoint` — so `task show` and `task state` give one damaged row one code. AC-05.1 amended in place | unwrapping `requireSession`'s scan — `task open --session` publishes `COORDINATION_WRITE_FAILED` with a `subject_id` naming a task no statement attempted; `TestAPreconditionReadThatFailsIsReportedAsAReadOfThatRow` red on the same arm at store level |
| 9b | §4.12 | `attribute` takes the write's own `now`; `Transition` reads the clock once, before the transaction, like the other two writers | reading the clock again inside `attribute` — `TestAMintedSessionStartsNoLaterThanTheRowItAttributes` red on all three writers, and the SQL count of rows attributed to a later session is 1 |
| + | §8, carried forward | `TestConcurrentInitAcrossProcessesWaits` sums the migrations each racer applied and asserts the set once per fresh database, instead of one applier per round — an invariant that stopped holding at `000002` because `Up` takes each migration in its own transaction. Three of five subagents met the old assertion red once today, under load, with the ledger correct each time | none claimed: this is a repair of a test's arithmetic, not a new guard. Ten runs green in isolation, and green inside the three `make` targets above |

### Where this pass departed from the brief, and why

- **Item 3 asserts the remedy's text and still performs the rename itself.**
  The brief said to read the second `next_action` and "carry it out". Parsing a
  path out of prose to execute it would pin the sentence's grammar rather than
  its meaning; the test now asserts the code, the location form's message and
  that the remedy names a rename, then renames. The mutation the brief named
  goes red on the message.
- **Item 8's two literal checks do not return what the brief predicted, on
  purpose.** `grep -rn "1.2 ms" internal/ docs/` still matches: the figure is
  kept where it is true — 1,000 tasks — and quoted in each correction note.
  `grep -c 'TestHumanOutputGolden' mr-003-findings.md` returns 2, because the
  corrected sentence now names both tests and says which one still loops over
  `status` and `doctor` only; a reader grepping for the old name finds the
  correction rather than nothing.
- **Item 9a was applied to three reads, not one.** §4.7 named `requireSession`.
  `Transition`'s and `WriteCheckpoint`'s task reads returned a bare error from
  inside the same kind of transaction, and `task state` on a damaged task row
  already disagreed with `task show` — the rule "the code follows what happened
  to the row" is only a rule if it holds at all three.
- **Item 9b touched `Transition` too.** The brief counted two call sites;
  `Transition` is the third, and it had no captured `now` to pass — it read the
  clock inside the transaction for `updated_at` as well. It now reads once.
- **Item 5's vacuity guards say "no files … were inspected", not the brief's
  quoted "no first-party packages were parsed".** The two existing guards do
  not share wording either, and neither of the two new walkers builds a
  package map; the message names what each actually counts.
- **A tenth item was done that the brief listed as backlog.** The concurrency
  test's false red is the last row above. It was moved forward because it
  fires under exactly the load a per-task gate produces, and because the brief
  had already written the fix.

### The verification pass, and what it changed

Not a third audit round — two agents, not eighty-five, over the twelve commits:
a Reader checking this record against the code and git sentence by sentence,
and a Breaker (a second model) attacking the four production changes. Both were
told to refute when uncertain.

**The Reader** checked 34 claims: 29 confirmed, 0 false, 4 unconfirmed. The
four were this section's own meta-statements — an agent count for round 2 that
the audit document never states, a partition of round 2's seventeen that was
this record's rather than the document's, and two process claims (who wrote
which item; that every mutation was re-run before commit) that git cannot show.
The first two are corrected above to the document's own words; the process
claims stay, marked here as unverifiable from the tree. It also found a second
copy of the superseded checkpoint-ordering rule in
`migrations/000002_coordination.sql`, above the table, which no round had
named; `migrations/README.md` now covers both. Four mutations re-run by the
Reader matched their recorded red lines.

**The Breaker** produced four findings and could not break the rest:

| Grade | What | Origin | Done |
|---|---|---|---|
| MEDIUM | Item 9b moved `Transition`'s clock reading before its transaction, so a move that waited on the write lock behind another writer committed second carrying the earlier stamp — `updated_at` went backwards. Reproduced 3 of 3 against the new binary with a 2 s lock held from `sqlite3`, 0 of 3 against `0f94e23~1`. Bounded: no shipped reader orders by `updated_at` | **introduced by this pass** | `dd93a1d`: the reading is taken inside the transaction, under the lock, and still passed to `attribute`, so 9b's invariant holds. `TestAMoveIsStampedUnderTheWriteLock` asserts it from inside the clock reading — a second handle with a one-millisecond busy budget must be refused when the store reads the clock — and goes red with the reading moved back before `BEGIN` |
| MEDIUM | A checkout entered through a symlink walks nothing: `filepath.WalkDir` does not descend a symlink handed to it as the root. Before item 5 two of the four walkers passed vacuously there; after it all four fail loudly on a healthy tree | predates; item 5 made it visible | `db7647d`: `moduletree.Root` resolves the real directory, and `internal/cli`'s layering test takes its root from `moduletree.Root` instead of a walk of its own. `TestRootResolvesACheckoutReachedThroughASymlink`; all four walkers pass from `/tmp/mindrail-link` |
| LOW | A refused *mint* insert is still published as the caller's row — `task open` with the session insert refused says "the task could not be written", `subject_id` a task id. Reachable only through a planted trigger or a foreign key lost mid-transaction | predates (round-1 item 5's mint move) | recorded, not fixed: no shipped command reaches it |
| LOW | The three `readFailed` arms send a damaged row to `mindrail doctor`, which exits 0 and `ok:true` on that database; and `checkpoint write` still succeeds on a task row that `show`, `list` and `state` refuse, because its existence probe selects only `task_id` and never decodes the row | predates (`doctor` has no damaged-row check; the probe has read one column since the milestone shipped) | recorded for MR-004's `doctor` work; the remedy is the same one item 13 chose in round 1 |

Attacked and not broken, in one line each: the schema gate under a ledger at 2
with the tables dropped, a ledger at 1 with the tables present, a genuine
downgrade, a never-initialised repository, `init` twice, a ledger ahead of the
binary, a bad checksum, a dropped column, a linked worktree and a shared
downgraded database — every one answered with one code and a remedy that,
carried out, cleared it. A read-only file and a held lock still answer
`RUNTIME_PATH_UNWRITABLE` and `RUNTIME_DB_UNAVAILABLE` as before. Nine shipped
commands over eight sessions left the session-clock inversion count at 0.
Roots named `.dotroot`, `vendor`, `testdata`, `graphify-out` and a nested module
copy all walk. The migration pin catches a trailing newline, a rename and a
narrowed glob; a duplicate `000002_*.sql` passes the pin and fails the loader.
Four subagent-written mutations re-run by the Breaker went red as recorded.

`make check`, `make tidy-check` and `make verify` are green after the two
fixes. **807** top-level test functions.

### What this pass did not close

Unchanged from §6, and round 2 §6 has the reasons: the four surviving store
mutations (two need a fixture registering a second project), the two
`commandsUnderTest` invariants that do not reach the coordination commands,
MR-002's Appendix F leftovers, `internal/workspace` and `internal/migration`
beyond the one test above, and every platform that is not Linux.
`docs/adr/0002-sqlite-driver.md` is still Proposed.
