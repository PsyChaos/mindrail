# MR-003 — findings

- **Requirements frozen at** `cd74767`, before any code: [mr-003-requirements.md](mr-003-requirements.md)
- **Design**: [mr-003-design.md](mr-003-design.md)
- **Implementation**: `f7ac67f..ce0c708`
- **Audit round 1**: graded `cd74767..f51478e`. 7 auditors, 107 agents, 50
  proposed, 46 confirmed, **27 distinct defects — 5 HIGH, 15 MEDIUM, 7 LOW**.
  The evidence is [mr-003-audit-round-1.md](mr-003-audit-round-1.md); §5 below is
  the summary.
- **Round-1 remediation**: all fifteen items of the audit's brief, `c29efa3..e7e946c`.
  §6 below is the record.
- **Status: remediated, not re-audited.** A remediation pass in this repository
  is audited: MR-002's first remediation closed 22 findings and introduced 11,
  and all eleven came from the fix pass. Round 2 grades this one.
- `make check` green, `make verify` green, `make tidy-check` green
- **799** top-level test functions, from 768 at the audit and 732 at the freeze
  (`go test -list '.*' ./...`, the method R4-M11 settled)

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
  shipped. The four commands are in `TestHumanOutputGolden` now, which executes
  them to produce the bytes it pins. `taskResult`, shared by `task open` and
  `task state`, was executed already.
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
| 11 | F21 | (test only) an accepted move advances `updated_at`; abandoning keeps the claimant | `updated_at = updated_at`, and adding `ABANDONED` to the clearing branch |
| 12 | F13, F35 | A coordination agreement matrix: AC-09.1's four conditions asserted across the envelope, the human rendering and the exit code, with the printed remedy carried out | `RenderError` dropping its "Next" section — all four rows red, item 2's halted-startup test green. *Corrected during the round-2 remediation (§4.6): this cell said "reverting item 2 — the startup rows red", a mutation that fails item 2's own test and none of these four* |
| 13 | F44, F46, F26 | `COORDINATION_READ_FAILED` wraps the read paths; `CoordinationErr` gives the block `indeterminate`; the empty-workspace guards are coded | reverting one read wrap — `code: ""`. Removing the indeterminate route — `not_observed` over a read that ran |
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
  one" — 1.2 ms to 10 µs at 20,000 tasks, with no schema change.
  `TestTheNewestCheckpointQueryDoesNotSortTheProject` asserts the plan rather
  than the timing.

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
