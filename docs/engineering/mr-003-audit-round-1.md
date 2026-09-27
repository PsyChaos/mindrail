<!--
MR-003 audit round 1, in full.

Seven auditors graded cd74767..f51478e in parallel — three Readers on the frozen
contract, four Breakers attacking the binary from isolated git worktrees — with
two adversarial verifiers per finding and an arbiter on every split. 107 agents.
It is a separate document rather than an appendix because at 1,500 lines it is
twice the length of mr-002-findings.md's whole four-round record, and the value
of a findings document is that a person reads it.

mr-003-findings.md carries the summary and the verdict. This is the evidence.
-->

# Audit round 1

Seven auditors graded `cd74767..f51478e` — the frozen contract against the whole
implementation — in parallel, four reading the requirements forward and three
attacking the binary. They proposed 50 findings; 49 survived merging; **46
survived adversarial verification**, each one re-run by a second agent whose job
was to kill it. Three died and are written out in §4.

Because the seven worked without seeing each other, the same defect was often
found two, four or five times. The 46 confirmed findings are **27 distinct
defects**. Each one below carries every ledger id that reported it.

---

## 1. The verdict

**The handover works; the explanations do not.** Two agents in two processes
continuing one task runs end to end, and the store underneath it is sound — a
Breaker attacked the state machine five ways and could not make it write on a
refusal, could not make two claimants both win, and could not make one project's
tasks leak into another's counts. What is wrong is what the new commands say when
anything *else* about the repository is wrong. They throw away the diagnosis the
startup sequence already made and print one of two sentences instead — *this
repository has no runtime database*, or *this worktree is not registered* — over
a database that exists and holds the tasks, and they attach `mindrail init` as
the remedy, which for a corrupt database, a bare repository, a directory that is
not a repository at all, or a broken `config.toml` fails on the same condition
and leaves the answer unchanged. Underneath that sit three narrower but sharper
defects: a `task open` whose title carries a byte JSON cannot represent **commits
the row and then reports failure**, so an agent that retries on failure opens the
task twice and can never remove either through `task list --json` again; the
newest checkpoint is chosen by comparing minted ids, and that comparison is only
monotonic *within one process*, so two agents writing in the same millisecond
hand the arriving agent a coin flip; and every `mindrail init` publishes
`coordination.observation: not_observed` with the sentence *"startup stopped
before this subsystem was read"* on a run that finished and printed READY. The
milestone's own record says the one contract deviation it made "is recorded in
`mr-003-requirements.md`" — `grep -c COORDINATION_UNAVAILABLE
docs/engineering/mr-003-requirements.md` returns 0. Nothing found in this round
loses or corrupts data on the healthy path; what it produces is wrong sentences,
duplicate rows and a set of guards that no test can turn red.

| | Count |
|---|---|
| Proposed / merged / confirmed | 50 / 49 / **46** |
| Distinct defects behind them | **27** |
| HIGH | 5 |
| MEDIUM | 15 |
| LOW | 7 |
| Introduced by MR-003 | 25 of 27 |
| Pre-existing, made reachable by MR-003 | 2 |

---

## 2. The confirmed findings

Ordered by severity, then by how easily a reader reaches them. The first id in
each row is the entry I write up below; the rest are the same defect found again.

| id | Sev | What is wrong | Where | New in MR-003 |
|---|---|---|---|---|
| F09 +F29 F36 F43 | HIGH | Every coordination command discards the startup verdict and misnames six repository conditions, with a remedy that cannot clear them | `internal/cli/coordination.go:92` | yes |
| F28 +F42 | HIGH | A `--json` write commits the row and then reports failure when a title or note is not valid UTF-8; a retry duplicates it | `internal/cli/coordination.go:57-81` | yes |
| F30 | HIGH | One such task permanently disables `task list --json`, and no verb can edit or delete a title | `internal/cli/representable.go:63` | yes |
| F20 +F12 | HIGH | The D-55 single-statement guard walks the whole filesystem, so any nested checkout turns `make check` red and accuses the file it protects | `internal/coordination/lifecycle_scan_test.go:41` | yes |
| F37 +F24 | HIGH | "The newest checkpoint" is a lexical id comparison whose monotonicity is process-local; two writers in one millisecond get a stale note | `internal/coordination/store.go:305-321` | yes |
| F10 +F03 F39 F45 | MEDIUM | `mindrail init` never reads coordination, so it publishes `not_observed` and a false sentence about a startup that completed | `internal/bootstrap/app.go:897` | yes |
| F02 +F11 F31 F38 | MEDIUM | A refused write mints and commits a session row while its envelope says "Nothing was written." | `internal/cli/coordination.go:125` | yes |
| F01 +F41 | MEDIUM | On the first run after upgrading, a registered worktree is reported as unregistered, over a `workspaces` table the sentence denies exists | `internal/bootstrap/app.go:907`, `internal/doctor/checks.go:858` | no — MR-003 made it the steady state |
| F13 +F35 | MEDIUM | AC-09.1 is unmet and unrecorded: no coordination command or condition ever joined the agreement matrix | `internal/cli/agreement_test.go:23` | yes |
| F21 | MEDIUM | Ten mutations of the store leave the whole suite green, including one that stops `updated_at` advancing | `internal/coordination/store.go:166,186,354,389` | yes |
| F04 | MEDIUM | AC-04.5's `storage.WriteFailure` ordering has no test; deleting it silently degrades exit 4 to exit 1 | `internal/coordination/store.go:527` | yes |
| F32 | MEDIUM | The unregistered-worktree guard has no test; deleting it makes `task list` answer `ok:true` with an empty list for a project holding three tasks | `internal/cli/coordination.go:99` | yes |
| F40 | MEDIUM | `ModeWrite`'s "creates nothing" boundary has no test; the mutant creates a database from a read command | `internal/bootstrap/app.go:100` | yes |
| F16 | MEDIUM | The design says `internal/coordination` was added to the layering test. It was not, and AC-03.5 is enforced by nothing | `docs/engineering/mr-003-design.md:55` | yes |
| F44 | MEDIUM | A read failure inside the store reaches the user with an empty `code`, empty `impact` and empty `next_action` | `internal/coordination/store.go:321` | yes |
| F46 | MEDIUM | A coordination read that ran and failed is published as "nobody looked"; the block has no path to `indeterminate` | `internal/bootstrap/app.go:941` | yes |
| F22 | MEDIUM | D-58 says a second claim's message names the holding session. No field anywhere does | `internal/coordination/errors.go:77` | yes |
| F33 | MEDIUM | On a rejected command line the envelope's `command` is the leaf word, so `task open` and `session open` both publish `"open"` | `internal/cli/root.go:328` | yes |
| F34 | MEDIUM | `mindrail task --json` prints help on stdout and exits 0, with no envelope at all | `internal/cli/root.go:244` | no — three new names reach it |
| F48 | MEDIUM | `Summarize` adds an unindexed sort to every startup, and the index comment says the queries are covered | `migrations/000002_coordination.sql:75` | yes |
| F15 +F05 F14 F23 F47 | LOW | The implementation record points at an amendment that was never written; AC-05.1 and AC-06.7 still state the superseded contract | `docs/engineering/mr-003-findings.md:78` | yes |
| F06 +F25 | LOW | AC-04.1 names a `Counts` method; the store ships `Summarize`, unrecorded | `docs/engineering/mr-003-requirements.md:279` | yes |
| F07 | LOW | AC-04.6 requires a `next_action` naming the offending id; two of six constructors do it, and nothing asserts it | `internal/coordination/errors.go:50` | yes |
| F17 | LOW | §4 calls the new commands' human renderings "goldens". There are none, and four of them are never executed by any test | `docs/engineering/mr-003-findings.md:120` | yes |
| F18 | LOW | The exit-class table's MR-003 section is introduced as "MR-003's five" above six rows | `internal/cli/envelope_test.go:63` | yes |
| F26 | LOW | `OpenSession`'s empty-workspace refusal is a bare error with no code, impact or next action | `internal/coordination/store.go:45` | yes |
| F49 | LOW | The "Last checkpoint" line renders as a stray space when the block was not observed | `internal/status/render.go:396` | yes |

### Where the round disagreed with itself about severity

Three defects were filed at more than one grade, because the auditors who found
them could not see each other. The grade in the table above is the one to act on.

| Defect | Filed as | Acted on as | Why |
|---|---|---|---|
| Write-then-refuse (F28, F42) | CRITICAL once, HIGH three times | HIGH | Nothing pre-existing is lost; the damage is additive duplicate rows and a loud failure. Three of four verifiers rejected CRITICAL on that ground. |
| Lifecycle scan (F12, F20) | MEDIUM once, HIGH once | HIGH | Both of F20's verifiers considered MEDIUM and rejected it: the printed message accuses the file it is protecting, so the reader cannot infer the workaround. |
| The unwritten amendment (F05, F14, F15, F23, F47) | HIGH once, MEDIUM three times, LOW once | LOW | Every verifier who examined the HIGH argued it down. Nothing reachable is wrong; the cost is one `grep`. Five independent findings on one wrong cross-reference is the clearest evidence in this round that parallel auditors inflate documentation drift. |

---

## 3. The findings in full

### F09 (also F29, F36, F43) — HIGH. A coordination command reports a halted startup as a missing database

**What it is.** `bootstrap.App.Run` calls the command body even when `Start`
failed, and its own doc comment says so: *"A command that wants the start error
in its verdict asks Diagnosis."* `status`, `doctor` and `init` ask.
`runCoordination` is the fourth caller and the only one that never does. It goes
straight to `coordinationScope`, which reads two facts — is the store `nil`, is
the workspace row empty — and has exactly two answers for every possible cause.

**Evidence.** Six repository states, one binary, side by side with `status` on
the identical bytes:

| Repository state | `status` says | `task list` says | Does `mindrail init` clear it? |
|---|---|---|---|
| not a Git repository | `NOT_A_GIT_REPOSITORY`, exit 2 | `COORDINATION_UNAVAILABLE`, exit 1 | no — exit 2, same code |
| bare repository | `BARE_REPOSITORY_UNSUPPORTED`, exit 2 | same, exit 1 | no |
| unparseable `config.toml` | `CONFIG_INVALID`, exit 2 | same, exit 1 | no |
| database corrupt | `RUNTIME_DB_CORRUPT`, exit 4 | same, exit 1 | no |
| coordination tables dropped | `MIGRATION_FAILED`, exit 4 | same, exit 1 | no |
| schema too new | `RUNTIME_DB_SCHEMA_TOO_NEW`, exit 4 | same, exit 1 | no |

In the `config.toml` case the repository holds a live task and the false sentence
is published on stdout as the answer:

```
$ sqlite3 .git/mindrail/mindrail.db "select task_id,title,state from tasks;"
TSK-01M22WXCD83KZ5R0AYDHCB1KJY|real work in flight|OPEN
$ mindrail task list --json
{"command":"task list","ok":false,"error":{"code":"COORDINATION_UNAVAILABLE",
 "why":"this repository has no runtime database, so it holds no sessions, tasks or checkpoints",
 "next_action":["Run `mindrail init` in this worktree, then re-run the command."]}}
exit 1
```

The true code is on stderr in the same run (`level=WARN msg="startup stopped"
error="CONFIG_INVALID: ..."`) and is dropped before the envelope is built. The
JSON envelope has no `warnings` key, so for the agent audience this milestone
exists to serve the mitigation is worth nothing.

**Both arms.** On a repository that genuinely has never been initialised the same
sentence is correct, `mindrail init` clears it, and the refused command creates
no database. The guard is right for the input it exists for and wrong for every
other startup that stopped.

**Why it matters.** An agent told *"this repository has no runtime database, so
it holds no sessions, tasks or checkpoints"* has been told its handover state
does not exist — the exact answer MR-003 was built to prevent — while the task
sits in the file. It then runs the one command it was told to run, that command
fails with a different code, and the coordination command repeats itself verbatim.

**Fix.** In `runCoordination`, take the start verdict the way `status.go:48`,
`init.go:72` and `doctor.go:51` take it, and surface it unchanged when it is
non-nil. `COORDINATION_UNAVAILABLE` should be reachable only when the startup
sequence completed.

**One citation to drop.** Three of the four reports cite AC-09.2. Read in place,
AC-09.2's "every row" refers to AC-09.1's four rows — unknown task, unknown
session, illegal transition, uninitialised repository — and none of the six
states above is one of them. The defect stands on AC-08.2 ("a failure to
construct it is reported as a defect in the binary … never as a repository
condition") and on the plain fact that the sentence is false.

---

### F28 (also F42) — HIGH. A write commits, then reports that it failed

**What it is.** `runCoordination` runs the body, which commits, and only then
calls `emit`, where `refuseUnrepresentableJSON` swaps a successful payload for a
refusal. Any string in the result that is not valid UTF-8 therefore produces
`ok:false` and exit 2 **after** the row is in SQLite.

**Evidence.** Counting rows around each call, in a scratch repository:

```
after init:                       tasks=0 sessions=0
$ mindrail --json task open --title "fix parser \xff bug"
{"command":"task open","ok":false,"error":{"code":"PATH_NOT_REPRESENTABLE", ...}}   exit 2
                                  tasks=1 sessions=1
$ (an agent that retries on failure runs it again)          exit 2
                                  tasks=2 sessions=2
$ (twice more)                    tasks=4 sessions=4
$ mindrail status --json | jq .data.coordination
{"observation":"observed","tasks_open":4, ...}
```

The same split holds for `task state` (the move lands) and `checkpoint write`
(the checkpoint is appended). The repository's own source calls this shape a
defect: `internal/cli/init.go:87-101` explains why MR-002 gave `init` a read-only
pre-flight — *"The command did every part of its job and reported that it had
failed, which is the worst of the two answers available: a caller retrying on
failure re-runs a completed initialisation."* `init.go:44` calls that pre-flight
before anything is written. `runCoordination` has no equivalent.

**Reachability is not exotic.** A repository with `i18n.commitEncoding =
ISO-8859-1` — the ordinary setting for a legacy Western-European or Turkish repo
— makes `git log -1 --format=%s` emit raw latin-1 bytes. `mindrail task open
--title "$(git log -1 --format=%s)"` reaches it on the first try.

**Both arms.** A valid-UTF-8 title in `--json` returns `ok:true` at exit 0, and
human mode writes the row and reports success honestly. The guard is not
over-firing; only its position is wrong.

**Why it matters.** Checkpoints are append-only by D-59 and there is no delete
verb, so every retry leaves a permanent duplicate handover note that nothing can
remove. On `task state` the retry gets a third, unrelated-looking answer:
`TASK_STATE_INVALID: task ... is CLAIMED and cannot move to CLAIMED`.

**Fix.** Validate `--title`, `--note`, `--reason` and `--label` for UTF-8 at the
command boundary, before `runCoordination` opens anything, and refuse there with
a usage error naming the flag. A flag value is user input at a system boundary
and can be refused; a filesystem path — the case the emit-time refusal was
written for — cannot.

---

### F30 — HIGH. One bad title kills the machine-readable task list, permanently

**What it is.** Once such a row exists, the refusal fires on every read that
serialises it.

**Evidence.**

```
$ mindrail task list --json ; echo exit=$?
{"command":"task list","ok":false,"error":{"code":"PATH_NOT_REPRESENTABLE",
 "next_action":["Run this command without --json; the human report carries these bytes through unchanged.",
                "Or rename the offending path so that every byte of it is valid UTF-8."]}}
exit=2
$ mindrail task --help | sed -n '/Available Commands/,/^$/p'
Available Commands:
  list   open   show   state
```

The second remedy cannot be carried out: the offending value is `tasks.title`,
not a path, and design §7 and AC-06.1 fix the surface at six subcommands ("No
seventh verb"), so there is no `task edit` and no `task delete`. Abandoning the
task does not remove it from an unfiltered list — and `task state --to ABANDONED
--json` has F28's defect, so it commits the move and reports failure, and the
retry is refused as a self-transition.

**What survives.** `task list --state <other-state> --json` still works, and
`task show` on sibling tasks still works, so the loss is the default listing plus
that one task's `show`. The first remedy ("run without `--json`") works and is
not a remedy for a machine consumer.

**Why it matters.** The only surface an agent has for seeing and cleaning up the
duplicates F28 created is the surface F28's own input disabled.

**Fix.** F28's boundary validation prevents new rows. For rows that already
exist, `unrepresentableError` needs a second form whose `why` and `next_action`
describe a stored field rather than a path — and something must be able to reach
that field.

---

### F20 (also F12) — HIGH. The single-statement guard goes red on a legitimate nested checkout

**What it is.** `TestNoOtherPackageStatesTheLifecycle` walks the module root and
skips only `.git`, `testdata`, and the one absolute path
`<root>/internal/coordination`. A nested checkout carries its own
`internal/coordination/state.go`, which is not that absolute path, so the guard
fires on a copy of the file it exists to protect.

**Evidence.** From a pristine clone, one ordinary Git command:

```
$ go test ./internal/coordination/ -run TestNoOtherPackageStatesTheLifecycle
ok      github.com/PsyChaos/mindrail/internal/coordination
$ git worktree add -q ./mode-b -b mode-b
$ go build ./... && echo build ok
build ok
$ go test ./internal/coordination/ -run TestNoOtherPackageStatesTheLifecycle
    state_test.go:202: mode-b/internal/coordination/state.go spells the task
    lifecycle out; decision D-55 keeps it in internal/coordination
FAIL
```

This is not hypothetical for this repository: `.gitignore:172` is
`.claude/worktrees/`, added in the first commit, and the shared checkout carried
sixteen worktrees there during this round, each with its own copy. Two auditors
found the finding by having `make check` fail underneath them. The directory is
empty at the time of writing, which is the only reason the gate is green now.

**The repository already had the rule.** Three older module-root walkers —
`internal/storage/arch_test.go:188`, `internal/cli/arch_test.go:457` and
`internal/bootstrap/knowledge_findings_test.go:274` — all skip dot-prefixed
directories, and all three are green under the same conditions. MR-003's new
walker is the only one that dropped the convention.

**Both arms.** A genuine second statement of the vocabulary — a file in
`internal/cli` listing all seven states — still turns it red and names that file.

**Why it matters.** `make check` is the gate this project measures itself with,
and it turns red for a reason unrelated to the code, on the layout the project's
own tooling creates. The message accuses `internal/coordination/state.go` of
violating the rule that keeps the lifecycle in `internal/coordination`, so a
maintainer's first move is to look for a copy that does not exist. A red gate
whose message is nonsense is one people learn to ignore.

**Fix.** Give the walker the skip predicate the repository already has: skip any
directory whose name starts with `.`, plus `vendor`, `testdata` and
`graphify-out`. Better, share one `shouldSkipDir` rather than keeping a fourth
copy.

---

### F37 (also F24) — HIGH. The newest checkpoint is decided by 80 random bits when two processes collide

**What it is.** `LastCheckpoint`, `Handover` and `Summarize` all take the row with
the largest `checkpoint_id`, and the code comment justifies it: *"the id carries
a 48-bit millisecond prefix and is monotonic within a millisecond, so it orders
two checkpoints written in the same millisecond."* That monotonicity lives in a
package-level `var entropy` in `internal/identity`. It is per process, nothing
seeds it from the database, and `nextEntropy` draws fresh `crypto/rand` bytes
whenever the millisecond advances — which it always has, for a freshly started
process.

**Evidence.** Ten concurrent `mindrail checkpoint write` processes on one task,
25 rounds, comparing `order by checkpoint_id desc` against `order by created_at
desc`:

```
ROUND 3 MISMATCH: task=TSK-01M22WZN102AKRGJ37PKB2197M  by_id=n6  by_created_at=n10
CKP-01M22WZN3P4KMHNRFT4G66F4K0|n10|2026-09-09T10:56:25.206400502Z
CKP-01M22WZN3P7M782PHH0C4DC4RR|n6 |2026-09-09T10:56:25.206087460Z
```

Both ids share the millisecond prefix `01M22WZN3P`; the random tails decide, and
`7M… > 4K…`, so the note written 313 microseconds earlier sorts newest. SQLite's
own `rowid` — assigned under the write lock — agrees with `created_at`, so this
is not an artefact of the yardstick. What the arriving agent sees:

```
$ mindrail task show TSK-01M22WZN102AKRGJ37PKB2197M
Last checkpoint, written by session SES-…:
  n6
```

The newest note on that task is `n10`, and there is no `checkpoint list`, so it
is unreachable.

**Bounds, honestly.** All ids are the same length over an ascending alphabet, and
exactly the first ten body characters carry the millisecond, so ids from
different milliseconds always sort correctly. The wrong answer is confined to a
single millisecond bucket — a coin flip between two contemporaneous notes, not a
stale note preferred over a fresh one. That is why this is HIGH and not CRITICAL.

**Why it matters.** D-59's justification is false as written, and the property it
claims is the one property this milestone's central read depends on. The
suggested `created_at` fallback is *not* a straight swap: `RFC3339Nano` trims
trailing zeros, so the TEXT column is not lexicographically monotone
(`'…51.26Z' > '…51.2645Z'` is true in SQLite), and the fixed clocks the tests
inject do not move at all.

**Fix.** Order by a key the database assigns — `rowid` is already correct, since
`checkpoints` is a plain `STRICT` table and D-59 forbids deletion — or add a
per-task sequence inside the insert transaction. Then restate D-59 to say what is
actually guaranteed: the id is monotonic within one process.

---

### F10 (also F03, F39, F45) — MEDIUM. `mindrail init` says startup stopped, three lines under READY

**What it is.** `registerWorkspace` returns on the `ModeInit` branch at
`app.go:897`, before `a.readCoordination(ctx)` at `app.go:924`. Init's mode can
therefore never set `CoordinationObserved`.

**Evidence.** One repository holding one OPEN task, two commands seconds apart:

```
$ mindrail init --json | jq .data.status.coordination
{"observation":"not_observed","tasks_open":0,"tasks_in_progress":0,"tasks_blocked":0,"last_checkpoint":null}
   readiness: READY,  stopped_at_step: absent
$ mindrail status --json | jq .data.coordination
{"observation":"observed","tasks_open":1, ...}
```

and the human rendering of that same init run:

```
Readiness: READY
Coordination
  Observation:       not observed — startup stopped before this subsystem was read
  Tasks open:        0 (not_observed)
...
READY FOR TARGETED WORK
```

`report.go:51-53` defines `NotObserved` as *"the sequence stopped before the
check's inputs were populated. Nobody looked."* The sequence did not stop.

**The fixture asserts the opposite of what ships.**
`internal/status/testdata/init_ready_json.golden` carries `"observation":
"observed"` for init, because it is built from `healthySubject()`, whose
`CoordinationObserved: true` was set by hand at `report_test.go:473`. The
comment beside it records that leaving the flag false *"published `not_observed`
on a report the observation tests require to carry no caveat at all — which is
exactly what those tests are for, and they caught it."* The guard was satisfied
by editing the fixture rather than by making the command observe.

**Mutation.** Adding the one line `a.readCoordination(ctx)` to the `ModeInit`
branch leaves the entire suite green — 2342 tests, 20 packages — and makes init
publish `observed` with the real count, agreeing with `status`. The correct and
incorrect behaviours are indistinguishable to the whole suite, and the goldens
already describe the fixed world.

**Why it matters.** `init` is idempotent and is the command a user runs on an
existing repository. On a re-init of a repository with work in flight it
publishes `tasks_open: 0` beside `readiness: READY` while `status` publishes
`tasks_open: 1`. The `not_observed` flag stops a careful consumer acting on the
zeros, which is why this is MEDIUM — but the sentence attached to it is false
about the run that just succeeded.

**Fix.** Call `readCoordination` on the init branch too, immediately after
`a.subject.Workspace = ws`, then regenerate the init goldens from the subject the
command actually builds and assert init's and status's coordination blocks match.

---

### F02 (also F11, F31, F38) — MEDIUM. A refused write leaves a session row and says nothing was written

**What it is.** `resolveSession` mints and commits a session before the operation
is judged. `OpenSession` is a bare `ExecContext` with no enclosing transaction,
so nothing can roll it back, and `task.go:47`, `task.go:95` and
`checkpoint.go:40` all call it before the store operation that can refuse.

**Evidence.** Counting `sessions` around each refusal:

```
before: 0
$ mindrail task open --json                      # no --title
  {"code":"COMMAND_LINE_INVALID","why":"a task needs a title","impact":"Nothing was written."}   exit 2
after:  1
$ mindrail checkpoint write TSK-NOPE --note hi --json
  {"code":"TASK_NOT_FOUND","impact":"There is no task to read or move, so nothing was written."}
after:  2
$ mindrail task state <id> --to CLAIMED --json   # already CLAIMED
  {"code":"TASK_STATE_INVALID","impact":"The task was left exactly as it was; nothing was written."}
after:  3
```

Thirty refusals in a loop produced exactly thirty rows. There is no `DELETE FROM
sessions` anywhere outside tests, no prune, and no command that lists sessions —
`coordination_test.go:397` says so: *"There is no command that lists sessions —
nothing needs one."*

**The repository already draws this distinction correctly.** `task state <id>
--to BOGUS` is caught before the application starts, mints nothing, and says
*"Mindrail did not run: no task was read and nothing was written."* The adjacent
`task open` with no title mints a row and says *"Nothing was written."* Same
command group, same error code, opposite truth value — because one check lives in
the CLI ahead of `resolveSession` and the other lives inside `OpenTask` behind it.

**Both arms.** With an explicit `--session`, or with a `--session` that does not
exist, a refused write mints nothing. The defect is confined to the session-less
path and `--session` is a real workaround.

**Mutation.** Moving the blank-title judgment ahead of `resolveSession`, emitting
a byte-identical error, leaves the suite green — 2342 passed — while changing the
row count. `TestTheReadCommandsMintNoSession` counts sessions around the two
reads and around one *successful* session-less write; nothing counts them around
a failing one.

**Why it matters.** `impact` is the structured field a caller reads to decide
whether to retry, and it is false on four paths, one of which is an exit-2 usage
error. The table grows one unreferenceable agent identity per attempt, and no
shipped command can see or remove them.

**Fix.** Move the empty-title and empty-note checks to the CLI boundary beside the
`--to` parse, and resolve the session inside the transaction that attributes it —
or defer the mint until the operation's own preconditions pass. Then add the
missing arm to `TestTheReadCommandsMintNoSession`.

---

### F01 (also F41) — MEDIUM. After upgrading, a registered worktree is told it is not registered

**What it is.** MR-003 adds migration 2, so every database written by an earlier
binary is exactly one migration behind. `registerWorkspace` skips the workspace
lookup whenever anything is pending, and the resulting reading borrows
`WORKSPACE_NOT_INITIALIZED` and a diagnostic written for the case where migration
1 itself has not run.

**Evidence.** A database created by the `cd74767` binary, read by HEAD:

```
$ sqlite3 .git/mindrail/mindrail.db ".tables"
projects     schema_migrations     workspaces
$ sqlite3 .git/mindrail/mindrail.db "select workspace_id, root_path from workspaces;"
WS-01M22WKY0T2C9CPC1KFJ8Z8KK9|/tmp/br3/repoD

$ mindrail status --json | jq .data.workspace
{"observation":"observed","registered":false}

$ mindrail doctor
Workspace
– UNAVAILABLE Workspace not registered
  The runtime schema is not established — 1 migration is still pending — so there
  is no workspace table to query for /tmp/br3/repoD.
```

The table is there and holds the row. `observation: "observed"` is the report's
own marker for *"the check ran and the values below it are findings"* — published
about a lookup that provably did not run, which is the shape of defect the
observation vocabulary was added to prevent.

**Origin.** Both the skip and the sentence pre-date the freeze (`a399c6c`). With a
single migration, "pending" meant migration 1, which really does mean no
`workspaces` table, so the sentence was true in every reachable case. D-53 is
what turned a true sentence false and moved its reachability from a startup race
to every existing database on first run. D-53 names only `runtime.schema_version`
1 → 2 as its visible consequence.

**Both arms.** `mindrail init` applies exactly migration 2, preserves the same
`WS-` and `PRJ-` ids and the original `registered_at`, and everything recovers —
which is what keeps this MEDIUM. On a genuinely unregistered linked worktree at
schema 2, the same sentence is correct.

**`status` is the surface that hides the cause.** `worst()` ranks the workspace
UNAVAILABLE above the migration DEGRADED, so `status` never mentions the pending
migration at all; only `doctor` prints *"Schema is behind this binary."*

**Fix.** Gate the skip on whether migration 1 in particular is pending, or attempt
the lookup and let a `no such table` error select the sentence. Give the block a
reading that means "the runtime schema is behind this binary", and mark
`WorkspaceInfo.Registered` `NotObserved` when no lookup was made — the convention
`RuntimeInfo` already uses.

---

### F13 (also F35) — MEDIUM. The one criterion written to catch this class was never implemented

**What it is.** AC-09.1 reads: *"The new failure conditions join the existing
agreement matrix, so the human rendering, the JSON envelope and the exit code are
asserted together for each: unknown task, unknown session, illegal transition,
uninitialised repository."* `internal/cli/agreement_test.go` was not touched by
MR-003, and `commandsUnderTest` is still `{"status", "doctor", "init"}`.

**Evidence.** `git diff --stat cd74767..HEAD -- internal/cli/agreement_test.go` is
empty. That one slice also drives `TestNoDocumentContradictsItsOwnErrorObject`
(`coherence_test.go:68`) and `TestEveryRemedyAboutAFileNamesThatFileAbsolutely`
(`contract_test.go:1707`), so the coordination commands are outside all three
invariants. Every coordination failure assertion in `coordination_test.go` runs
with `--json`; the only two non-JSON invocations are on success paths.

**The consequence is not coverage, it is F09.** An auditor's probe ran `task list
--json` beside `status --json` across all 45 existing matrix rows and found the
substitution on at least thirteen of them, including three where the two commands
disagree in the *other* direction. The matrix rows that were never added are
hiding live wrong answers, not merely untested ones.

**Why it matters.** MR-002's §5 rule 3 says a remedy that cannot be carried out is
a defect, and the matrix is the machinery this repository built to catch it.
Leaving the new commands out of it while the milestone reports itself implemented
means the guard was published and not delivered. The Definition of Done requires
an unmet criterion to be recorded with its reason in `mr-003-findings.md`; AC-09
appears nowhere in that document.

**One real obstacle the remediation must know about.** The matrix's
"never-initialised" row expects exit 0 with no code, and the coordination
commands deliberately exit 1 there. `assertExemptionsAreStillEarned` refuses to
let an exemption cover a different exit class, so that row cannot be joined
without a decision about the matrix's exemption semantics. That is a legitimate
reason for the criterion to be partly unmet — and it had to be written down.

**Fix.** Add the coordination commands to `commandsUnderTest` with a small
argument mapping (`task list` is two words, and bare `mindrail task` prints
help), or add a coordination-specific matrix with the same three-way assertion,
reusing `assertRemedyIsWhatTheRowExpects` so the printed remedy is carried out
rather than substring-matched. F09 must be fixed first; the rows go green only
once the startup error is surfaced.

---

### F21 — MEDIUM. Ten mutations of the store leave the suite green

**What it is.** Definition of done item 3 says *"Every new guard has a recorded
mutation that turns it red (AC-10.3)"*, and §3 of the implementation record
presents its nine-row table as discharging it. Ten further mutations survive.

**Evidence.** Each applied, `go test ./internal/... -count=1` run, then reverted:

| Mutation | Site | Suite |
|---|---|---|
| `Transition` stops advancing `updated_at` (`updated_at = updated_at`) | `store.go:186` | green |
| `CLAIMED → ABANDONED` also clears `claimed_by` | `store.go:181` | green |
| `OpenSession`'s empty-workspace guard deleted | `store.go:45-47` | green |
| the four `.UTC()` normalisations dropped | `store.go:53,99,166,269` | green |
| `Summarize`'s count query loses its project scope | `store.go:354` | green |
| the newest-checkpoint join loses its project scope | `store.go:389` | green |
| `requireSession`'s empty-id guard deleted | `store.go:485-487` | green |

The harness kills what it should: the six coordination-side mutations §3 records
all turn red as stated, so §3's log is honest — it is simply not "every new
guard".

**The one that matters.** `updated_at` is a published field on every `Task` the
CLI marshals, and with the mutation applied the real binary reports the same
`updated_at` after `task open`, after `--to CLAIMED` and after `--to
IN_PROGRESS`, with the suite green. The only assertion on the column
(`store_test.go:313`) is the *refusal* arm — it proves a refusal does not rewrite
the column, and a store that never wrote the column at all satisfies it
identically.

**Two rows to strike.** The `requireSession` mutation is an equivalent mutant (the
SELECT returns no rows and produces the byte-identical error, so no test could
ever kill it), and the two `Summarize` scoping mutations are the gap §4 already
records. The `.UTC()` mutations are an inherited pattern — the same mutation on
`internal/workspace/store.go:81` also survives.

**Fix.** Assert that an accepted transition advances `updated_at` past the value
the row held before — the `stepClock` fixture exists precisely to make that
observable without sleeping — and assert the abandon path's claim column. Record
the remaining survivors in §4 so the Definition of Done is either met or
explicitly not met.

---

### F04 — MEDIUM. The write-failure ordering AC-04.5 asks for is correct and unguarded

**What it is.** `(*Store).writeFailure` routes a failed write through
`storage.WriteFailure` first, so a permission error is named as one. Nothing
asserts it. Replacing the body with a plain `writeFailed(...)` leaves `make
verify` — fmt-check, vet, test, `-race` and smoke — completely green.

**Evidence.** Both binaries against the same repository with `chmod 444` on the
database file:

```
HEAD:    {"code":"RUNTIME_PATH_UNWRITABLE","why":"the runtime database is not writable, so
          Mindrail could not write the agent session",
          "next_action":["restore write permission on …mindrail.db, …-wal, …-shm"]}   exit 4
mutated: {"code":"COORDINATION_WRITE_FAILED","why":"the agent session could not be written…",
          "next_action":["Run `mindrail doctor` to check the runtime database, then re-run."]} exit 1
```

A coverage run over the whole module reports execution count **0** for every
statement of `writeFailure`. Neither arm of the classifier is exercised.

**Why it matters.** `internal/cli/envelope_test.go:74` justifies putting
`COORDINATION_WRITE_FAILED` in the `ExitFailed` class *because* "the storage
conditions that really are 'come back later' are named by `storage.WriteFailure`
before this code is reached". A hand-written constant table's stated rationale
rests on an ordering nothing checks. The regression it would permit is a silent
exit-class change from 4 to 1 — which an orchestrator branching on "runtime
unavailable" would misread. (The mutant's "run doctor" remedy does still lead to
the truth in two hops; `doctor` correctly reports `RUNTIME_PATH_UNWRITABLE` at
exit 4 on the same file, so the closed loop the finding's first draft described
was fixed before MR-003.)

**Fix.** A coordination test that makes the database unwritable and asserts
`OpenSession`, `OpenTask`, `Transition` and `WriteCheckpoint` report
`CodeRuntimePathUnwritable` rather than `CodeCoordinationWriteFailed` — with the
other arm, that a writable database produces neither.

---

### F32 — MEDIUM. The unregistered-worktree guard is undefended by the whole gate

**What it is.** `coordinationScope`'s second branch — the refusal for a worktree
that is not in the `workspaces` table — has no test in either direction. Deleting
it leaves `make verify` green in every leg.

**Evidence.** Driven from an unregistered linked worktree of a repository whose
project holds three tasks and one checkpoint:

```
HEAD:    task list --json -> exit 1, {"code":"COORDINATION_UNAVAILABLE", …}
MUTANT:  task list --json -> exit 0, {"command":"task list","ok":true,"data":{"tasks":[]}}
         task open --json -> exit 1, {"code":"","why":"opening a session: no workspace id",
                                      "impact":"","next_action":[]}
```

`grep -rn "not registered in the runtime database" internal/` returns exactly one
line — the source. `TestEveryCoordinationCommandRefusesAnUninitialisedRepository`
uses `newRepo(t)`, a repository with no `.git/mindrail` at all, which reaches the
*first* branch and produces the same code, so the test cannot tell the two guards
apart.

**Why it matters.** The behaviour the guard prevents is `ok:true` with an empty
list for a project that has work in it — the one answer an arriving agent cannot
distinguish from a genuinely empty project. `git worktree add` followed by any
coordination command before `mindrail init` reaches the state.

**Fix.** A row that uses the existing `addWorktree` helper, runs all six commands
from the unregistered linked worktree, and asserts `COORDINATION_UNAVAILABLE`
plus the branch-specific `why`, against a project that demonstrably has tasks.

---

### F40 — MEDIUM. `ModeWrite`'s central promise — "creates nothing" — is asserted by nothing

**What it is.** `Mode.creates()` is the predicate that stops a coordination
command bringing a runtime database into existence. Changing it to `m == ModeInit
|| m == ModeWrite` leaves all 768 tests and the smoke suite green.

**Evidence.** In a repository where `init` ran and the database file was then
deleted, leaving `.git/mindrail/` behind:

```
HEAD:    task list -> COORDINATION_UNAVAILABLE ; files unchanged
MUTANT:  task list -> COORDINATION_UNAVAILABLE ; .git/mindrail/mindrail.db now exists
```

A read command created a runtime database and reported failure while doing it —
which the `ModeWrite` doc comment says the mode exists to prevent.

**Scope, corrected.** In a repository that was *never* initialised the mutant can
create nothing, because `ensurePaths` gates `EnsureDirs()` on `ModeInit` and
`storage.Open` does not create parent directories. So `creates()` is load-bearing
only in the residual state above. The criterion the finding actually meets is
AC-10.1 ("every guard is checked in both directions"), not AC-10.3.

**Fix.** A bootstrap test that starts an App in `ModeWrite` against a repository
with the runtime directory present and no database file, and asserts
`os.Stat(paths.DBPath)` is still `ErrNotExist` afterwards. Mirror it for
"migrates nothing" and "registers nothing".

---

### F16 — MEDIUM. The design says the layering test was extended; it was not

**What it is.** `mr-003-design.md` §3 states: *"The existing `upwardRules` in
`internal/cli/arch_test.go` gain no new entry, because coordination is not an
adapter — but it is added to the layering test's expectations so that a later
import of `internal/status` from it is caught."* `grep -rn "coordination"
internal/cli/arch_test.go` returns nothing, and `git diff --stat cd74767..HEAD --
internal/cli/arch_test.go` is empty.

**Evidence.** Adding imports of `internal/git` and `internal/workspace` to
`internal/coordination/model.go` gives `go build ./...` OK, `go vet ./...` OK, and
every package `ok`. The one example the design names — a later import of
`internal/status` — happens to be caught anyway, because
`internal/status/report.go:8` already imports `internal/coordination`, so the
compiler refuses the cycle. Every other direction AC-03.5 forbids is open.

**Why it matters.** AC-03.5 is currently *satisfied* — coordination's production
imports are exactly `app`, `identity`, `storage` and the standard library — so
nothing is wrong today. What is wrong is that a frozen design document tells the
next reader a guard exists, and it does not. The design doc has no amendment note
and no commits after the freeze.

**Fix.** Either add `"internal/coordination"` to `upwardRules` plus an allow-list
assertion for AC-03.5's four permitted imports, or amend §3 to say the layering
test was deliberately left alone and record the gap in §4.

---

### F44 — MEDIUM. A read failure reaches the user with no code, no impact and no remedy

**What it is.** Every write path in `internal/coordination` is wrapped in a domain
error; every read path returns a bare `fmt.Errorf`. `emit` publishes it verbatim,
and `internal/app/output.go:44` says so in its own comment: *"An error that
reached the boundary without a code is a defect, but dropping it would be worse
than reporting it uncoded."*

**Evidence.** One damaged timestamp (`UPDATE checkpoints SET created_at =
'yesterday'`):

```
$ mindrail --json task show TSK-…
{"command":"task show","ok":false,"error":{"code":"","why":"look up the last checkpoint of
 \"TSK-…\": checkpoint CKP-… created_at: parse timestamp \"yesterday\": parsing time
 \"yesterday\" as \"2006-01-02T15:04:05.999999999Z07:00\": cannot parse \"yesterday\" as \"2006\"",
 "impact":"","next_action":[]}}
exit 1
```

The `why` is a Go parser message with a format string in it. AC-04.6 says *"Every
error carries a registered `app.Code`, a diagnostic, an impact and at least one
`next_action`"*; fifteen bare `fmt.Errorf` sites in `store.go` do not.

**Reachability, honestly.** Structural corruption is intercepted by the startup
integrity check before any store read runs, so the demonstrated trigger is
semantic damage `PRAGMA integrity_check` cannot see. One route needs no external
SQL at all: `FormatTime` writes `RFC3339Nano` and `ParseTime` is strict, so a
host clock outside years 1–9999 writes checkpoints that `task show` can never
read back.

**Contrast that decides it.** The pre-MR-003 analogue in `internal/workspace` has
the identical bare read errors, and damaging it the same way still reaches the
user fully coded — because the *caller* wraps it. MR-003's commands have no such
wrapper.

**Fix.** Add the read counterpart to `writeFailed` — a `COORDINATION_READ_FAILED`
carrying the doctor remedy, or `storage` classification — and wrap the read paths
in it. Separately, `emit` should refuse to publish an envelope whose payload has
no registered code.

---

### F46 — MEDIUM. A read that ran and failed is published as "nobody looked"

**What it is.** `readCoordination` swallows every error into the single
`CoordinationObserved = false` flag, and `coordinationInfo` maps that flag to
`NotObserved`. The block has no path to `Indeterminate`, the value the report
introduced for *"the check ran, failed, and cannot vouch for the values"*.

**Evidence.** The same damaged row as F44:

```
readiness READY, stopped_at_step absent
coordination: {"observation":"not_observed","tasks_open":0, …,"last_checkpoint":null}
Observation: not observed — startup stopped before this subsystem was read
```

The store was constructed, both queries ran, the second failed. The adjacent
Runtime block does produce `indeterminate` correctly — `chmod 000` on the
database gives *"indeterminate — this subsystem was read and could not answer"* —
so the vocabulary works; coordination simply has no route to it.
`coordinationInfo`'s own doc comment states the dichotomy as *"either the summary
was read or the sequence never got that far"*, which is not exhaustive.

**Scope, corrected.** AC-07.1 asks only for `observation` "following the existing
`observed` / `not_observed` convention", so this is not a breach of the graded
ledger. It is a breach of the type's own documented meaning.

**Fix.** Give `doctor.Subject` a `CoordinationErr` beside the flag — the shape
`WorkspaceErr` already has — and return `Indeterminate` when the read was
attempted and failed, `NotObserved` only when step 7 never reached it.

---

### F22 — MEDIUM. D-58 promises the refusal names the holder; nothing does

**What it is.** D-58, frozen and unamended on this clause: *"A second claim on a
claimed task is refused by D-55's table … and the message says the task is
already claimed and by which session."*

**Evidence.** Session A claims, session B tries to claim:

```
code           = TASK_STATE_INVALID
why            = task TSK-… is CLAIMED and cannot move to CLAIMED
impact         = The task was left exactly as it was; nothing was written.
next_actions   = [From CLAIMED this task can move to: IN_PROGRESS, OPEN, ABANDONED.]
metadata       = map[from_state:CLAIMED task_id:TSK-… to_state:CLAIMED]
```

Session A's id appears in none of the four fields. The information is in hand:
`store.go:149` reads the full row into `current` inside the transaction and
`store.go:157` refuses eight lines later with `current.ClaimedBy` populated.

**Both sides of the ledger.** D-58's TASK-05 amendment is entirely about identity
not blocking a transition and leaves this clause standing. D-55, frozen in the
same commit and restated in the design, specifies the message for the whole
illegal-transition class as exactly three items — the state it is in, the state
asked for, and the moves available — and the code implements exactly those. So
the frozen contract is self-inconsistent: two documents say three items, one
sentence asks for a fourth on one row.

**The workaround exists and is one command.** `mindrail task show <id>` prints
`Claimed by: SES-…` and the handoff note with its author, which is the graded
one-call handover read AC-06.3 defines. That is why this is MEDIUM.

**Fix.** Pass `current.ClaimedBy` into `transitionNotAvailable` and, when `from ==
StateClaimed && to == StateClaimed` and the claimant is non-empty, say so in
`why` and add a `claimed_by` metadata key — asserted on the metadata key, not on
a sentence fragment (AC-10.2). Or amend D-58 in place, the way three other
decisions were amended.

---

### F33 — MEDIUM. A rejected command line publishes the leaf word as its identity

**What it is.** `Root.emitUsageEnvelope` builds the envelope with `cmd.Name()`.

**Evidence.** Same binary, same repository:

```
success path:            task list --json      -> "command":"task list"
                         session open --json   -> "command":"session open"
usage-rejection path:    task open extra --json    -> "command":"open"
                         session open extra --json -> "command":"open"
                         task open --titel x --json -> "command":"open"
```

Worse, one command contradicts itself under one error class: `task state <id>
--to BOGUS --json` publishes `"task state"` (raised inside the body, which uses
`cmd.CommandPath()`), while `task state --to CLAIMED --json` with the positional
missing publishes `"state"` (raised by cobra through `usageError`). Both are exit
2, both `COMMAND_LINE_INVALID`.

**Base rate.** At `cd74767` every Mindrail command was top level, so `cmd.Name()`
happened to equal the published name. MR-003 added the first nested commands,
which is what makes the value both wrong and ambiguous. Replacing `cmd.Name()`
with an obviously wrong value leaves the suite green — nothing asserts this field
on the usage path for any command.

**Fix.** Use the same string the command body passes to `runCoordination` (or
`cmd.CommandPath()` with the binary name stripped) in `emitUsageEnvelope`, and
assert the `command` member for both a well-formed and a rejected line.

---

### F34 — MEDIUM. `mindrail task --json` prints help on stdout and exits 0

**What it is.** `root.go:244` makes a command group with subcommands runnable by
printing help. Under `--json`, stdout is then a page of text and there is no
envelope at all.

**Evidence.**

```
$ mindrail task --json ; echo exit=$?
Manage tasks.
...
exit=0
```

968 bytes on stdout, 0 on stderr, no JSON. The adjacent healthy input is correct:
`mindrail task bogus --json` emits a proper `COMMAND_LINE_INVALID` envelope at
exit 2. `mindrail checkpoint --json && echo "handoff recorded"` prints "handoff
recorded" with nothing written.

**Origin.** Pre-existing: `completion --json` and bare `mindrail --json` do the
same at `cd74767`. MR-003 added three more names that reach it — names an agent
is far likelier to type than `completion`. `root.go:96` registers the flag as
*"emit the result as a single JSON object on stdout"*, and that sentence is
printed inside the output that is not one.

**Fix.** Route the group's no-argument case through `usageError` so it emits a
real envelope at exit 2, or state in the requirements that a group command with
no subcommand is a help request outside the `--json` contract. Either way assert
it for all four groups including the root.

---

### F48 — MEDIUM. Every startup pays an unindexed sort, and the index comment says otherwise

**What it is.** `readCoordination` runs two queries on every read-only startup.
The second — the newest checkpoint of a *project* — is not served by
`idx_checkpoints_task`.

**Evidence.**

```
-- SELECT c.task_id, c.session_id, c.created_at FROM checkpoints c
--   JOIN tasks t ON t.task_id = c.task_id WHERE t.project_id = ?
--   ORDER BY c.checkpoint_id DESC LIMIT 1
|--SEARCH t USING INDEX idx_tasks_project (project_id=?)
|--SEARCH c USING INDEX idx_checkpoints_task (task_id=?)
`--USE TEMP B-TREE FOR ORDER BY
```

| tasks | `Summarize` |
|---|---|
| 0 | 35 µs |
| 1,000 | 1.20 ms |
| 5,000 | 6.41 ms |
| 20,000 | 28.98 ms |
| 50,000 | 75.83 ms |

The driver is task count, not checkpoint count: the plan walks every task of the
project and probes the checkpoint index per task, so the cost is ~38 ms at 50,000
tasks even with zero checkpoints. The comment at
`migrations/000002_coordination.sql:75` reads *"The two queries status and `task
list` actually run: tasks of a project by state, and the newest checkpoint of a
task"* — the task-scoped query is `task show`'s, and status runs a project-wide
variant the comment never names.

**Scale, honestly.** The `<150 ms` p95 `mindrail_status` budget is already blown
at 50,000 tasks *without* MR-003: a variant binary with `readCoordination` gutted
takes ~280 ms on the same database, because `PRAGMA integrity_check` alone costs
130 ms there. At every plausible size the surcharge is small (+4 ms at 8,000
tasks, +21 ms at 16,000). This is a correct answer delivered later, plus a
comment that mis-attributes which command runs which query.

**Fix.** Either add `idx_checkpoints_newest ON checkpoints(checkpoint_id DESC)` so
the join can walk backwards and stop at the first row belonging to the project, or
carry `project_id` on `checkpoints` (derivable at write time) and index
`(project_id, checkpoint_id)`. Correct the comment to name the query status
actually runs — and `task list`'s own `ORDER BY task_id DESC`, which is likewise
a temp b-tree.

---

### F15 (also F05, F14, F23, F47) — LOW. The amendment that was never written

**What it is.** The implementation record says, of the sixth error code: *"The
requirements' AC-05.1 said five new codes and there are six; the amendment is
recorded in `mr-003-requirements.md`."*

**Evidence.**

```
$ grep -c COORDINATION_UNAVAILABLE docs/engineering/mr-003-requirements.md
0
$ sed -n '296,298p' docs/engineering/mr-003-requirements.md
- **AC-05.1** Five values are added to `app.Code` and to the registry:
  `SESSION_NOT_FOUND`, `TASK_NOT_FOUND`, `TASK_STATE_INVALID`,
  `CHECKPOINT_NOT_FOUND`, `COORDINATION_WRITE_FAILED`.
$ sed -n '319,321p' docs/engineering/mr-003-requirements.md
- **AC-06.7** Every new command works in an uninitialised repository the way the
  existing ones do: `WORKSPACE_NOT_INITIALIZED` with the `mindrail init` remedy,
  never a panic and never a half-written row.
```

`git diff cd74767..HEAD` on that file has exactly three hunks, all in §1 —
amendments to D-53 and D-58 and a new D-63. §2 is byte-identical to the freeze.
The binary emits `COORDINATION_UNAVAILABLE` where AC-06.7 names
`WORKSPACE_NOT_INITIALIZED`.

**What is right about it.** The substitution is correct and well argued —
`WORKSPACE_NOT_INITIALIZED` is D-03's single zero-exit row, and reusing it would
make `mindrail task open && …` run its second half after a failed open. The
reason is written out in three places a reader reaches: §2 of the implementation
record, a fifteen-line doc comment at `internal/app/code.go:104-118`, and the
task list's Turkish status block. Definition-of-done item 1 names
`mr-003-findings.md` as the place to record an unmet criterion, and §2 does
exactly that.

**What is wrong.** One clause of one sentence points at a file that contains no
such record, and only AC-06.7 is genuinely deviated from — AC-05.1 names five
values that must exist and all five do, with no exclusivity clause (compare
AC-06.1's explicit "No seventh verb"). Five auditors met this fresh and graded it
from LOW to HIGH.

**Fix.** Correct the clause at `mr-003-findings.md:78-79`, and add the in-place
amendment to AC-06.7 naming the code the commands return and why, in the
`*Amended during TASK-0n*` form the three existing amendments use. Fix the stale
count in the Tier paragraph at `mr-003-requirements.md:10`.

---

### F06 (also F25) — LOW. AC-04.1 names a method the store does not have

AC-04.1 lists eight `Store` methods. Seven exist verbatim; the eighth, `Counts`,
exists nowhere in the module — `grep -rn 'func (s \*Store) Counts'
internal/coordination/` returns nothing, and `go doc ./internal/coordination
Store` lists `Summarize` instead. The word `Counts` appears in exactly one place
in the whole repository: the requirements line itself.

`Summarize` is the better name and the substance is delivered — the method takes
a `context.Context` and returns the three counts plus the newest checkpoint
reference, which design §8's `last_checkpoint` key requires and which
`store_test.go:596` asserts in both arms. What is missing is the record.
Definition-of-done item 1 makes it "met, or recorded as not met with the reason",
and it is neither.

**Fix.** A one-line amendment under AC-04.1: `Counts` ships as `Summarize`,
because design §8 requires the newest-checkpoint join alongside the counts and a
name saying only "counts" would understate what `status` reads from it.

---

### F07 — LOW. Two of six error constructors put the offending id in a remedy

AC-04.6 requires *"at least one `next_action` that names the offending id"*. From
the real binary:

| Code | `next_action` | Names the id? |
|---|---|---|
| `TASK_NOT_FOUND` | Run `mindrail task list` to see the tasks this repository has. | no |
| `SESSION_NOT_FOUND` | Run `mindrail session open` and pass the id it prints as `--session`. | no |
| `TASK_STATE_INVALID` | From IN_PROGRESS this task can move to: BLOCKED, READY_TO_COMPLETE, ABANDONED. | no |
| `TASK_STATE_INVALID` (terminal) | Task TSK-… is ABANDONED, which is final; open a new task instead. | yes |
| `CHECKPOINT_NOT_FOUND` | Run `mindrail checkpoint write TSK-… --note …` | yes |
| `COORDINATION_WRITE_FAILED` | Run `mindrail doctor` … | no id exists |

Two of the four "misses" are specified verbatim elsewhere in the frozen contract —
D-55 enumerates exactly the three items the transition message carries, and D-63
(an amendment recorded in place) requires *"a remedy naming `--reason`"*. So the
honest residual is two unexplained constructors, not four.

The part worth acting on is the guard, and it is broader than the AC clause.
Removing the id from `why`, from `metadata` **and** from the remedy in
`sessionNotFound`, `checkpointNotFound`, `transitionNotAvailable` and
`blockedReasonMissing` leaves `go test ./...` fully green. Only
`taskNotFound`'s `why` is protected, by
`TestAnUnknownTaskIsRefusedByNameRatherThanByAConstraint`.

**Fix.** Sweep every constructor and assert the id is where the criterion says it
is — or amend AC-04.6 in place to say the id must appear in the payload rather
than specifically in a `next_action`.

---

### F17 — LOW. "The human renderings are goldens" — there are none, and four are never executed

§4 says: *"**The human renderings are goldens, not assertions.** The four new
commands print text no test reads for meaning; a rendering that named the wrong
session would pass."* There is no golden for any coordination command:
`internal/cli/testdata/` holds five files, all added by MR-001, and
`TestHumanOutputGolden` loops over `{"status", "doctor"}`.

The gap is materially worse than the wording admits, and the proof is a mutation
neither the finding nor its verifiers ran first. Replacing the **entire body** of
`sessionResult`, `handoverResult`, `taskListResult` and `checkpointResult` with
`panic()` leaves `go test ./...` green in all 18 packages **and** `make smoke`
green. §4 says the text is *printed and unread*; in fact the code path is never
entered at all, so a panic, a nil dereference or an error-return regression in the
default output of `session open`, `checkpoint write`, `task show` and `task list`
ships green. A golden would necessarily execute the command to produce the bytes
it pins, which is exactly what the word conceals.

The count "four" is correct as written — four renderings are unexecuted;
`taskResult` (shared by `task open` and `task state`) is executed and its
minted-session line is asserted.

**Fix.** Correct the sentence to say the four renderings are unexecuted, and add
the four commands to `TestHumanOutputGolden`.

---

### F18 — LOW. "MR-003's five" introduces six rows

`internal/cli/envelope_test.go:63` opens the milestone's section of the
exit-class table with *"MR-003's five, all ExitFailed (requirement AC-05.3)"*,
and six map entries follow it, ending in `CodeCoordinationUnavailable` and
`CodeCoordinationWriteFailed`. Git history shows a genuine staleness rather than
a typo: the comment was written when the block had five rows (`f7ac67f`) and the
sixth arrived later in the same range (`39875e3`).

The table is right and the comment is wrong — deleting the sixth row turns
`TestExitClassTableCoversEveryRegisteredCode` red with *"code
`COORDINATION_UNAVAILABLE` has no exit class in `exitForCode`"*. The comment's
description of the class is stale too: it says these codes "found the repository
healthy", which is not true of a code defined as *"a coordination command run
against a repository that has no runtime store yet"*.

**Fix.** "MR-003's six", and correct the class description.

---

### F26 — LOW. `OpenSession`'s refusal carries none of the four fields

`Store.OpenSession(ctx, "", "")` returns `fmt.Errorf("opening a session: %w",
errors.New("no workspace id"))`. `app.PayloadOf` reports no domain payload:
`has domain payload = false, payload = {Code: Why: Impact: NextAction:[]}`.

Not reachable from today's CLI — `coordinationScope` refuses first, so both call
sites always pass a non-empty id — which is why it is LOW. Deleting the whole
guard leaves the suite green in both directions, and a coverage run shows the
adjacent unknown-workspace path produces a *better-structured* error, because the
foreign key answers with a code.

AC-11.4 makes this package the surface MR-014's MCP tools will call directly,
without `coordinationScope` in front of it.

**Fix.** Either give the guard an `app.NewError` with a code, a diagnostic naming
the empty handle, an impact and a next action — and a two-arm test — or delete it
and let the foreign key answer.

---

### F49 — LOW. The stray space

`lastCheckpointNote` returns `observed(o, "")` for a non-observed block, and
`observed` concatenates `value + " (" + string(o) + ")"`, so the empty value
leaves a leading space that `writeSection` then pads around:

```
Coordination
  Observation:       not observed — startup stopped before this subsystem was read
  Tasks open:        0 (not_observed)
  Last checkpoint:    (not_observed)
```

Four spaces after the colon against the section's own three. Replacing the empty
string with `"none"` renders `Last checkpoint:   none (not_observed)` — correctly
aligned — which is how the misalignment was proved rather than asserted. On an
observed block the line reads `none` or `TSK-… by SES-…` correctly, and the JSON
member is `null` and correct, so nothing consumes it. It is printed on every
`mindrail init`, which is the only reason it is worth a line.

**Fix.** Give the line a subject: `"none (" + string(o) + ")"`, or `"unknown"`
once F46's `indeterminate` arm exists.

---

## 4. What was refuted, and why

Three findings died, and all three died the same way: **a sentence was graded
outside the scope it states for itself.** That is the same death that killed two
thirds of MR-002's fourth round, and it is worth writing out.

**F08 — "D-53's amendment says nothing else in either golden moved; one of them
gained a six-line block."** The arithmetic is right: `git diff cd74767..HEAD` on
the two CLI goldens is four changed lines plus a six-line `Coordination` block.
But the paragraph opens `*Amended during TASK-02, before the implementation it
describes was finished.*` and reports an action taken in that commit. At
`f7ac67f` the golden diff is exactly four lines, and nothing else moved. The
finding's cumulative reading also falsifies a clause three sentences earlier that
it does not challenge — "it named `internal/status/report_test.go` and
`internal/status/testdata/`, and neither moved" — which at HEAD is equally false
under the same reading. Only one reading makes the paragraph self-consistent, and
it is not the finding's. The cumulative fact is carried correctly by AC-07.4:
*"the only changes are the additive block and `runtime.schema_version` 1 → 2"*,
which is exactly the HEAD diff.

**F19 — "'the same remedy sentence' — the two strings differ."** They do: `status`
prints `mindrail init`, the coordination refusal prints "Run `mindrail init` in
this worktree, then re-run the command." But this repository defines remedy
identity as *the command named*, and says so in code written before MR-003
existed: `contract_test.go:950` — *"Matching is on the whole action, not a
substring: 'Move the file aside and run `mindrail init`' is a good remedy that
happens to name the same command"* — and `coherence_test.go:41` — *"neither is a
spelling of one hard-coded string"*. The repository already ships four spellings
of this one remedy. And the harm claimed cannot occur: `internal/cli/coordination.go`
does not exist at `cd74767`, so there is no "caller who scripts against the old
remedy". Worth acting on anyway: the word "sentence" is loose applied to a
two-word command, and the sentence one line below it is F15.

**F27 — "§4 says the fixture cannot produce a second project; it can, in twelve
lines."** The twelve lines work, and the production code is correctly scoped —
that part is true and useful. But the finding's own demonstration begins "I added
this helper to the existing fixture", and for a fixture with a fixed set of
methods, "cannot" and "does not" collapse. The clause the finding quotes as the
excuse — "— `openFixture` registers a single worktree" — is the clause that names
the one-line remedy, which is the opposite of framing the gap as blocked. §4 makes
no cost claim anywhere, and where it does defer a gap it says so explicitly ("That
is MR-004's subject"). Under the brief's own rule, a §4 gap is reportable only if
shown *worse* than admitted, and this showed it exactly as admitted. The two
`Summarize` scoping mutations in F21 are the same gap and carry the same caveat.

---

## 5. What this round did not look at

Assembled from the seven auditors' own coverage notes. Stated plainly, because
this round found 46 findings and the surface it did not touch is larger than the
surface it did.

**Graded thoroughly, so not on this list:** the identity extraction against its
MR-001 original (byte-identical apart from the package clause and two comments,
with three tests *added*); the migration and its checksum (`000001_initial.sql`
sha256 unchanged at `fd8d958`, `cd74767` and HEAD); the real upgrade path from a
database created by the frozen binary; all 49 ordered state pairs at both the pure
and the store layer; all nine mutations §3 records, re-run and re-confirmed; the
JSON key surface of all four pre-existing commands, diffed old binary against new
and found purely additive; `make verify` and `make tidy-check`, both green; and
the test count, 768 from a baseline of 732 by `go test -list '.*' ./...`.

**Two of §4's admitted gaps were tested and the code held.** Two Breakers ran 20
in-process races and one cross-process race on a single task, and eight concurrent
`checkpoint write` processes: `_txlock=immediate` serialises, exactly one claimant
wins, the loser gets a clean `TASK_STATE_INVALID`, and everything is green under
`-race`. §4's statement that "the current answer is 'last writer wins'" is
pessimistic — for the claim path the answer is a clean refusal. What the same
work *did* find is F37: concurrency is safe for writes and unsafe for the read
that decides which write was last.

**Not looked at at all:**

- **MR-002's Appendix F leftovers are all still open**: `internal/knowledge/record`,
  `internal/knowledge/schema`, the schema documents, and doctor's six
  non-knowledge checks have never been audited by anyone, in any milestone.
- **`internal/workspace` and `internal/migration`** beyond the specific behaviours
  MR-003 touches. The bare-`fmt.Errorf` read pattern F44 reports exists there too,
  and was not graded.
- **The disk-full, read-only and busy branches of `storage.WriteFailure`.** F04
  established that coordination's routing through it is untested; nobody tested
  the classifier's own arms from a coordination call.
- **Linux only.** No Windows or macOS path behaviour was exercised by anyone.
- **The smoke suite's content.** Two auditors ran it; nobody read it. F17 shows it
  does not execute four of the six new commands' default output.
- **Performance of anything but `Summarize`.** F48 incidentally established that
  `status` is ~280 ms at 50,000 tasks *without* MR-003's contribution, and nobody
  looked at why.
- **A second project in one database, end to end.** F27 shows the fixture is twelve
  lines away; no test exists and none was written.
- **The state machine reached by a route other than `pathTo`** — §4's second
  admitted gap. A task that arrived at `IN_PROGRESS` through `BLOCKED` is still
  never tested against the 49 pairs.
- **REQ-11's non-goals** were not checked against the implementation by anyone.
- **`internal/cli/coordination_test.go` as a body of work.** Individual assertions
  were mutated; nobody graded the file against AC-10.1 and AC-10.2 criterion by
  criterion.

---

## 6. What generalises

MR-002's §5 left four rules. This round would add three, and each is drawn from a
defect that was found by more than one auditor working independently.

5. **A command that runs after a failed startup must say what the startup said.**
   `bootstrap.App.Run` deliberately calls the body even when `Start` failed, and
   its doc comment names the obligation that creates: *"A command that wants the
   start error in its verdict asks Diagnosis."* Three commands ask; the fourth
   family does not, and the result is six repository conditions collapsed into two
   false sentences with a remedy that cannot clear them (F09). The rule is not
   "consult the verdict" — it is that **a code path which can be entered after a
   failure has to be written as if it will be**, because the caller cannot see the
   `WARN` line on stderr.

6. **A write that has already committed cannot be reported as a failure.** This is
   MR-002's rule 3 sharpened by two independent defects. F28: the row is in
   SQLite, the envelope says `ok:false`, and the caller's correct response —
   retry — duplicates the work. F02: the session is in SQLite and the envelope
   says "Nothing was written." In both cases the fix is positional, not textual:
   **judge everything that can be judged before opening anything, and let
   everything that cannot be judged early live inside the transaction it belongs
   to.** MR-002 already paid for this once, in `init`'s pre-flight probe, and
   wrote the reason down in the source; the new commands did not inherit it.

7. **Asserting the code is not asserting the cause.** Four separate mutations
   survived the whole suite for the same reason: the test drives the guard from a
   state where *several different guards would produce the same code*.
   `TestEveryCoordinationCommandRefusesAnUninitialisedRepository` reaches
   `COORDINATION_UNAVAILABLE` through a repository with no database, so deleting
   the *registration* guard is invisible (F32); `ModeWrite`'s "creates nothing"
   is invisible because the mutant still refuses with the same code (F40);
   `storage.WriteFailure`'s ordering is invisible because nothing ever makes a
   write fail (F04). The rule: **a guard's test must be built from the state that
   reaches that guard and no other**, and the way to know it is is to delete the
   guard and watch the test go red — which is what AC-10.1 asks for and what nine
   recorded mutations out of nineteen actually did.

One observation that is about auditing rather than about code, recorded because
this was the first round in this repository run with seven parallel auditors:
**documentation drift inflates under parallel audit.** One wrong cross-reference
in `mr-003-findings.md` was found five times and graded HIGH, MEDIUM, MEDIUM,
MEDIUM and LOW. Every verifier who examined the HIGH argued it down. A synthesis
step that merges before grading is not a nicety at this width; without it, the
round's headline severity would have been set by the fifth auditor to meet a
`grep`.

---
---

# Remediation brief

Fifteen work items, ordered. The first is one line and unblocks the measurement
everything else depends on. Items 2–5 are the reachable defects; 6–10 are the
guards that cannot fail; 11–15 are the ledger.

Every entry names **what the fix must not break** and **the mutation that proves a
test now holds it**. A remediation pass in this repository is audited (MR-002 §5
rule 1: 11 of 11 of the last round's defects came from the fix pass), so each
mutation below is the evidence that pass will be graded on.

---

**1. Stop the lifecycle scan walking the filesystem — F20, F12.**
`internal/coordination/lifecycle_scan_test.go:41`. Replace the two-name skip with
the predicate `internal/storage/arch_test.go:188` already has: skip any directory
whose name starts with `.`, plus `vendor`, `testdata`, `graphify-out`. Share one
helper rather than keeping a fourth copy of `moduleRoot`.
*Must not break:* the guard must still fire on a genuine second statement of the
vocabulary anywhere in the module, including under `internal/`.
*Mutation that proves it:* drop a file into `internal/cli` listing all seven state
strings — the test must name that file and go red. And, from a clone with `git
worktree add ./mode-b`, the test must stay green.
*Why first:* `make check` cannot be trusted as the gate for items 2–15 while a
nested checkout turns it red for an unrelated reason.

**2. Surface the startup verdict in the coordination commands — F09, F29, F36,
F43.** In `runCoordination`, take the verdict the way `status.go:48`,
`init.go:72` and `doctor.go:51` take it, and return it unchanged when non-nil,
before `coordinationScope` is consulted. Then split `coordinationScope`'s second
branch so "the schema is behind this binary" is not spelled "this worktree is not
registered".
*Must not break:* on a repository that has genuinely never been initialised, the
answer must stay `COORDINATION_UNAVAILABLE` at exit 1 with the `mindrail init`
remedy (AC-06.7 as amended), and `mindrail init` must still clear it. On a healthy
repository the six commands must still exit 0.
*Mutation that proves it:* revert the `Diagnosis` call and run the new agreement
rows — the "not a Git repository", "bare repository" and "config.toml cannot be
parsed" rows must go red naming both the code and the exit class. Second arm:
delete the `space.ID == ""` branch and confirm the F32 test (item 8) goes red, so
the two guards are separately pinned.

**3. Refuse unrepresentable input at the boundary — F28, F42, F30.** Validate
`--title`, `--note`, `--reason` and `--label` for UTF-8 in the command's own flag
handling, before `runCoordination` opens anything, and refuse with
`COMMAND_LINE_INVALID` naming the flag. Separately, give `unrepresentableError` a
second form for a stored field, so the rows that already exist produce a remedy
that describes a column rather than a path.
*Must not break:* `init`'s existing pre-flight
(`TestInitRefusesAnUnrepresentableRepositoryBeforeWriting`) and the emit-time
refusal for genuine filesystem paths, which is correct and must keep firing. Human
mode must keep carrying the bytes through unchanged at exit 0.
*Mutation that proves it:* move the validation back behind `resolveSession` — a
new test that counts `tasks` around a refused `--json task open` with a `\xff`
title must go red on `tasks: 1, want 0`. Second arm: a valid-UTF-8 title must
still return `ok:true`.

**4. Make "the newest checkpoint" mean the newest — F37, F24.** Order by a key the
database assigns. `rowid` is already monotonic and correct here, since
`checkpoints` is a plain `STRICT` table and D-59 forbids deletion; a per-task
sequence assigned inside the insert transaction is the more explicit option. Do
**not** substitute `created_at`: it is stamped before the transaction, and
`RFC3339Nano` trims trailing zeros so the TEXT column is not lexicographically
monotone. Restate D-59 to say the id is monotonic within one process.
*Must not break:* `TestTheLastCheckpointIsTheNewestIdNotTheNewestTimestamp`, whose
fixture shares one `created_at` across checkpoints and whose injected clock does
not move — the new key must still order that fixture correctly.
*Mutation that proves it:* a test that inserts two checkpoints whose ids are in
the opposite order to their insertion (which is what two processes in one
millisecond produce) and asserts `LastCheckpoint`, `Handover` and `Summarize` all
return the later insert. Reverting to `ORDER BY checkpoint_id DESC` must turn it
red on all three.

**5. Stop refused writes minting sessions — F02, F11, F31, F38.** Move the
empty-title and empty-note judgments to the CLI boundary beside the existing
`--to` parse, and resolve the session inside the transaction that attributes it
so a refusal rolls it back.
*Must not break:* `TestAWriteWithNoSessionMintsOneAndSaysSo` (AC-06.5) — a
successful session-less write must still mint exactly one and report the id — and
`TestTheReadCommandsMintNoSession` (AC-06.6).
*Mutation that proves it:* restore `resolveSession` to the top of
`newTaskOpenCommand` — a new arm of `TestTheReadCommandsMintNoSession` counting
sessions around each of the four refusal paths must go red with `sessions: 1, want
0`. Note that this mutation emits a byte-identical error, so a test that asserts
only the payload cannot catch it; it must count rows.

**6. Read coordination on the init path — F10, F03, F39, F45.** Call
`a.readCoordination(ctx)` on the `ModeInit` branch of `registerWorkspace`,
immediately after `a.subject.Workspace = ws`. Then regenerate the init goldens
from the subject the command actually builds, and build one status fixture with
`CoordinationObserved` left false so the not-observed rendering is still
exercised for the case it genuinely describes.
*Must not break:* `TestRepositoryBlockIsObservedOnAHealthyRepository`, the guard
whose earlier failure is why `healthySubject()` carries
`CoordinationObserved: true` by hand. Readiness must not move (D-62).
*Mutation that proves it:* revert the one line — a new end-to-end assertion that
`init --json` and `status --json` publish the same coordination block on a
repository with one task must go red on `observation: not_observed`. The existing
goldens will not catch it; they already assert the fixed value.

**7. Distinguish "the schema is behind" from "not registered" — F01, F41.** Gate
the workspace-lookup skip on whether migration 1 in particular is pending, or
attempt the lookup and let a `no such table` error select the sentence. Mark
`WorkspaceInfo.Registered` `NotObserved` when no lookup ran. Record the upgrade
consequence in D-53, which currently names only the schema version.
*Must not break:*
`TestFullyMigratedDatabaseStillReportsAnUnregisteredWorktree` — the deliberate
over-fire guard for a genuinely unregistered worktree at schema 2.
*Mutation that proves it:* a new test that builds a schema-1 database with a
`workspaces` row and asserts the reading is a schema-behind code, not
`WORKSPACE_NOT_INITIALIZED`, and that `workspace.observation` is `not_observed`
rather than `observed`. Restoring the blanket `PendingCount > 0` skip must turn it
red.

**8–10. Close the guards that cannot fail — F32, F40, F04.** Three tests, no
production change.
- **F32:** drive all six commands from an unregistered linked worktree of a
  repository whose project holds tasks, asserting the code *and* the
  branch-specific `why`. *Mutation:* delete the `space.ID == ""` branch; the test
  must go red on `ok:true, tasks:[]`, which the mutant currently produces silently.
- **F40:** start an App in `ModeWrite` against a repository whose runtime
  directory exists and whose database file does not, and assert
  `os.Stat(paths.DBPath)` is `ErrNotExist` after `Start`. *Mutation:* `creates()`
  returns `m == ModeInit || m == ModeWrite`; the test must go red. Mirror for
  "migrates nothing" and "registers nothing".
- **F04:** make the database unwritable and assert all four writers report
  `CodeRuntimePathUnwritable`. *Mutation:* remove the `storage.WriteFailure` call
  from `(*Store).writeFailure`; the test must go red on
  `COORDINATION_WRITE_FAILED`. Both arms — a writable database must produce
  neither code.

**11. Assert `updated_at` advances, and the abandon path's claim column — F21.**
*Mutation:* `updated_at = updated_at` in `Transition`'s UPDATE must turn the new
assertion red. Use the existing `stepClock` so no sleep is needed. Record the
remaining survivors in §4 rather than closing them, and strike the equivalent
mutant (`requireSession`) from the list.

**12. Bring the coordination commands into the agreement matrix — F13, F35.**
Requires item 2. Add the four AC-09.1 conditions as rows and extend
`commandsUnderTest` (or add a coordination matrix) so the human block, the
envelope and the exit code come from one run, with a `clear` closure that carries
out the printed `next_action`.
*Must not break:* `assertExemptionsAreStillEarned`. The never-initialised row
expects exit 0 with no code and the coordination commands correctly exit 1 there —
that conflict needs a decision recorded in the requirements, not an exemption
quietly widened.
*Mutation that proves it:* revert item 2's `Diagnosis` call; the new rows must go
red. Then record the mutation in §3.

**13. Give the reads a code, and the block an `indeterminate` — F44, F46, F26.**
Add `COORDINATION_READ_FAILED` (or route through `storage` classification) and
wrap the fifteen bare read errors; add `CoordinationErr` beside
`CoordinationObserved` and return `Indeterminate` when the read was attempted and
failed. Make `emit` refuse to publish an envelope whose payload carries no
registered code. Code or delete `OpenSession`'s empty-workspace guard.
*Must not break:* `not_observed` must still be produced when the sequence
genuinely never reached step 7 — `chmod 000` on the database must still give
`not_observed` on Knowledge, Workspace and Coordination together.
*Mutation that proves it:* damage a `created_at` and assert `task show --json`
carries a registered code; reverting the wrap must go red on `code: ""`.

**14. The two remaining behaviour items — F33, F34, F22, F48, F49.** Use the
command's full path in `emitUsageEnvelope` and assert the `command` member on both
a well-formed and a rejected line (F33). Route a group's no-argument case through
`usageError` (F34) — or record the carve-out. Add `claimed_by` to the
self-transition refusal and assert on the metadata key (F22) — or amend D-58.
Index the project-scoped checkpoint query and correct the migration comment (F48).
Give the "Last checkpoint" line a subject (F49).

**15. The ledger — F15, F05, F14, F23, F47, F06, F25, F18, F16, F17, F07.** One
pass over four documents and two comments:

| Edit | File |
|---|---|
| Correct the clause claiming the amendment is recorded | `mr-003-findings.md:78-79` |
| Amend AC-06.7 to name `COORDINATION_UNAVAILABLE`, with the D-03 zero-exit argument | `mr-003-requirements.md:319` |
| Fix "five `app.Code` values" in the Tier paragraph | `mr-003-requirements.md:10` |
| Amend AC-04.1: `Counts` ships as `Summarize`, and why | `mr-003-requirements.md:279` |
| "MR-003's five" → six, and correct the class description | `internal/cli/envelope_test.go:63` |
| Say the layering test was left alone and AC-03.5 is unenforced — or enforce it | `mr-003-design.md:55` |
| Say the four renderings are unexecuted, not "goldens" | `mr-003-findings.md:120` |
| Record AC-04.6's two unexplained constructors, or put the ids in the remedies | `mr-003-requirements.md:291` |

*Must not break:* nothing executable. *The check that proves it:* `grep -c
COORDINATION_UNAVAILABLE docs/engineering/mr-003-requirements.md` returns a
non-zero number, and `grep -n Counts` returns only an amendment note.

---

**One instruction for the pass itself.** Item 6's mutation is the shape to
generalise from: the fix is one line, the whole suite is green both before and
after, and the goldens already assert the fixed value. Nine of this round's
findings are defects that no existing test can see in either direction. After each
item above, run the stated mutation and confirm red **before** running the suite
and confirming green. A green suite over a fix is this repository's most
frequently produced false claim.