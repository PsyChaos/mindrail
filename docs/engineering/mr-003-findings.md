# MR-003 — findings

- **Requirements frozen at** `cd74767`, before any code: [mr-003-requirements.md](mr-003-requirements.md)
- **Design**: [mr-003-design.md](mr-003-design.md)
- **Implementation**: `f7ac67f..ce0c708`
- **Audit round 1**: graded `cd74767..f51478e`. 7 auditors, 107 agents, 50
  proposed, 46 confirmed, **27 distinct defects — 5 HIGH, 15 MEDIUM, 7 LOW**.
  The evidence is [mr-003-audit-round-1.md](mr-003-audit-round-1.md); §5 below is
  the summary.
- **Status: not done.** No remediation has been written.
- `make verify` green, `make tidy-check` green
- **768** top-level test functions, from a baseline of 732
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
| `LastCheckpoint` orders by `created_at` instead of `checkpoint_id` | `TestTheLastCheckpointIsTheNewestIdNotTheNewestTimestamp` — returns the *first* note |
| `WriteCheckpoint` stops verifying the session | `TestAnUnknownSessionIsRefusedByEveryWriter` — the foreign key answers instead, with `COORDINATION_WRITE_FAILED` |
| Leaving `BLOCKED` no longer clears the reason | `TestABlockCarriesItsReasonAndLeavingClearsIt` |
| The commands run in `ModeReadOnly` again | the handover acceptance test, with `RUNTIME_PATH_UNWRITABLE` |
| `identity.nextEntropy`'s backwards-clock clamp is removed | `TestABackwardsClockStillMintsAscendingIds` |
| The entropy counter's byte carry is removed | `TestTheEntropyCounterCarriesAcrossAByte` |

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
