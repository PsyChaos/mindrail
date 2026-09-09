# MR-003 — implementation record

This is not an audit document. MR-001's and MR-002's findings files record what
independent auditors found; **MR-003 has not been audited by anyone**, and §1 of
[mr-002-findings.md](mr-002-findings.md) is the measurement that says what fresh,
ungraded code is worth in this repository. What follows is the implementer's own
account, written so that an audit has something to grade against.

- **Requirements frozen at** `cd74767`, before any code: [mr-003-requirements.md](mr-003-requirements.md)
- **Design**: [mr-003-design.md](mr-003-design.md)
- **Implementation**: `f7ac67f..ce0c708`
- `make verify` green, `make tidy-check` green
- **768** top-level test functions, from a baseline of 732
  (`go test -list '.*' ./...`, the method R4-M11 settled)

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
requirements' AC-05.1 said five new codes and there are six; the amendment is
recorded in [mr-003-requirements.md](mr-003-requirements.md).

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
- **The human renderings are goldens, not assertions.** The four new commands
  print text no test reads for meaning; a rendering that named the wrong session
  would pass.
- **`internal/identity` moved code that MR-001 shipped.** The tests moved with
  it and the format is asserted, but nobody has graded the move itself against
  the original.
- Everything MR-002's Appendix F left open is still open: the unaudited
  `record`, `schema`, migration, storage and workspace packages, and doctor's six
  non-knowledge checks.
