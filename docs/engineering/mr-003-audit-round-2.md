<!--
MR-003 audit round 2, in full.

Nine auditors graded c29efa3..24069aa — the round-1 remediation pass, not the
implementation it repaired — in parallel: two Readers on the amended contract,
four Breakers attacking the binary from isolated worktrees, and three judges
assigned one each to the three places the pass departed from the brief. Two
adversarial verifiers per finding on a refute-when-uncertain threshold, and an
arbiter on every split.

Round 1's evidence is in mr-003-audit-round-1.md. The remediation record it
produced is mr-003-findings.md §6. This document grades that record.
-->

# Audit round 2

Nine auditors graded `c29efa3..24069aa` — the fifteen-item remediation pass
against the brief that commissioned it. Two read the amended contract forward;
four attacked the binary from isolated worktrees; three were given one deviation
each, with the instruction that "the deviation is sound" is a verdict and not an
absence of work.

They proposed 35 findings; merging collapsed them to **30 distinct claims**;
**17 survived adversarial verification**. Thirteen died, and §5 writes out why,
because round 1's record of its own three refutations turned out to be the most
re-read part of that document.

Of the 17: **no HIGH, 4 MEDIUM, 13 LOW.** Sixteen of the seventeen were
introduced by the fix pass itself.

---

## 1. The verdict

**The repairs hold; the account of them does not.** Every one of round 1's 27
defects is closed, and closed in the code rather than in prose — a Breaker
re-ran all fifteen of the record's stated mutations in the priority order given
and fourteen turned their named test red, which is the strongest single result
this round produced. The five HIGHs are gone and stayed gone under attack: a
refused write mints no session across 20 in-process racers and 20 concurrent
processes, and the losers that used to mint 19 orphan rows now mint none; the
newest checkpoint is chosen by the key the database assigned; the unrepresentable
title is refused before anything opens; the lifecycle guard no longer walks out
of the module. Nothing found in this round loses or corrupts data, and nothing
round 1 found came back.

What is wrong divides in three. One user-reachable regression: item 7 widened the
workspace-lookup gate correctly for the upgrade case it was written for, and in
doing so un-gated the six coordination commands on a schema that has no `tasks`
table, so the one path every existing repository takes on its first run after
upgrading now answers with a remedy that cannot clear the condition, where
before the pass it answered with one that could. Second, several of the new
guards assert less than the record says they do: item 13's substitute sweep
cannot reach a single one of the ten read paths it was written to protect, nine
of which are killed by nothing; the path form of item 3's two-form split is
pinned in one direction only; and the mutation §6 records for item 12 turns a
different item's test red, leaving the AC-09.1 matrix — the entire answer to F13
— with no recorded falsifier. Third, the pass amended the contract in seven
places and left the three other places that state the same rules untouched, so
`AC-04.4` and design §4 still specify the checkpoint ordering that finding F37
refuted and item 4 removed from the code.

The pattern behind twelve of the seventeen is one thing: **this pass fixed the
code and then over-described the fixing.** Six of the LOWs are a false sentence
in the record about work that was genuinely done — a cross-reference to a test
that does not contain the fix, a benchmark figure copied from the wrong row of
the table it cites, a commit count of fifteen over thirteen commits, a row marked
"(test only)" whose commit changed production behaviour. None of them is
reachable by a user. All of them are what round 3 would cite.

| | Count |
|---|---|
| Proposed / distinct / confirmed | 35 / **30** / **17** |
| HIGH | 0 |
| MEDIUM | 4 |
| LOW | 13 |
| Introduced by the fix pass | 16 of 17 |
| Round-1 defects reopened | 0 |

---

## 2. The base rate, which is why this round exists

MR-002's first remediation pass closed 22 findings and introduced 11 new
defects, and all eleven came from the fix pass itself. That is the number this
round was commissioned to measure against.

| | MR-002 remediation 1 | MR-003 remediation 1 |
|---|---|---|
| Round-1 defects closed | 22 | **27** |
| New defects introduced | 11 | **16** |
| New per closed | 0.50 | **0.59** |
| Of them, HIGH | — | **0** |
| Of them, user-reachable behaviour | — | **1** |

**The rate did not improve; the severity collapsed.** Measured by count alone
this pass is marginally worse than the one before it — 0.59 new defects per
defect closed against 0.50. Measured by what a person can reach, it is a
different class of result. MR-002's eleven included live behaviour regressions;
this pass's sixteen include exactly one, a remedy that sends the reader to
`mindrail doctor` where `mindrail init` is what clears it. Twelve of the sixteen
are a wrong sentence in a document or a guard that cannot fail, and the four
MEDIUMs are one behaviour regression plus three cases of a test or a criterion
claiming more coverage than it has.

The honest reading is that **the discipline the brief imposed worked on the code
and not on the record.** The pass ran every stated mutation before running the
suite, which is exactly what it was told to do and what caught the fourteen; what
it did not do was apply the same standard to the sentences it wrote about those
mutations. A recorded mutation that turns the wrong test red (§4.4), a measured
figure taken from the wrong row (§4.11), and a cross-reference to a test that
does not contain the fix (§4.10) are all the same failure as a green suite over
a broken fix, one level up.

---

## 3. The three deviations, graded

§6 records three places where the pass carried out an item differently from the
letter of the brief, and says each "is a judgment a second audit should grade
rather than take on trust". One judge was assigned to each.

**Item 10 — the `storage.WriteFailure` test moved to `internal/coordination`.
SOUND. Examined end to end and held; zero findings.** The judge tested the
deviation's own premise rather than accepting it. At HEAD, with the database and
its WAL and shm files `chmod 444`, all four writer commands report
`RUNTIME_PATH_UNWRITABLE` at exit 4 from the startup verdict — and under the
mutation the brief named, `(*Store).writeFailure` reduced to a plain
`writeFailed`, they report the *identical* envelope and the identical exit. The
CLI-level test the brief asked for would therefore have stayed green under the
mutation that was supposed to kill it. The premise is true. The store-level test
that replaced it dies correctly: the same mutation turns all four subtests red,
flipping to `COORDINATION_WRITE_FAILED`, which proves through `adopt`'s
domain-error passthrough that the code each subtest observes is minted inside
`(*Store).writeFailure` and that no other guard answers in that state. Coverage
at the CLI boundary is separately held by `TestBrokenSetupMatrix`'s two
unwritable rows. This is round 1's own rule 7 applied correctly, by a pass that
noticed its own fix had invalidated the brief's test placement.

**Item 13 — the uncoded-envelope refusal enforced as a test rather than a runtime
branch. NOT SOUND, and one MEDIUM is owed.** The argument for the deviation is
defensible: `app.WriteJSON`'s comment argues that dropping an uncoded error is
worse than reporting it uncoded, and inventing a general "internal defect" code
is a wire-vocabulary addition the brief did not ask for. The substitute is not.
`TestNoCoordinationFailureReachesTheWireUncoded` drives one command over the
MR-001 agreement matrix, and because item 2 put the startup verdict first, every
broken matrix row is answered before any store method runs. The sweep therefore
cannot reach *any* of the ten `readFailed` sites the same item created — which
are the paths F44 existed for. Stripping all ten leaves the sweep passing.
Written up in §4.2.

**Item 14 — the migration comment left wrong, and F48's index not added. SOUND in
substance; two corrections owed.** Both halves were tested rather than read. The
checksum justification is true: `internal/migration/load.go` takes sha256 over
the whole file body including comments, both checksum gates compare it, and a
comment-only edit to `migrations/000002_coordination.sql` empirically produces
`MIGRATION_CHECKSUM_MISMATCH` on `status` and `task list` against an existing
database while the clean binary stays READY. Editing the applied file is
correctly ruled out. The `EXISTS` rewrite is real and better than advertised: on
a 20,000-task database the old plan is exactly F48's (`SEARCH t
idx_tasks_project`, a per-task probe, and a `TEMP B-TREE`), the new plan is
`SCAN c` with an `EXISTS` probe and no sort, and a differential run over a
two-project database with interleaved checkpoints, an empty prefix-named project
and a nonexistent project produced zero mismatches. Asserting the plan rather
than the timing is the right call — the driver is pinned at `modernc.org/sqlite
v1.58.0`, and one judge's own timing means swung threefold under load. What is
owed: the recorded speedup attaches the 1,000-task measurement to 20,000 tasks
in three places (§4.11), and the rewrite trades an unconditional cost for a
conditional one that is not recorded anywhere (§4.13).

---

## 4. The confirmed defects

Ordered by severity, then by how easily a person reaches them.

| # | Sev | What is wrong | Where | New in the pass |
|---|---|---|---|---|
| 4.1 | MEDIUM | The upgrade gate un-gates the six coordination commands on a schema with no `tasks` table; the remedy they print cannot clear it | `internal/bootstrap/app.go:921`, `internal/cli/coordination.go:120` | yes |
| 4.2 | MEDIUM | Item 13's substitute sweep reaches no read path; 9 of 10 `readFailed` wraps are killed by nothing | `internal/cli/coordination_agreement_test.go:193` | yes |
| 4.3 | MEDIUM | The path form of item 3's two-form split is asserted by no test | `internal/cli/representable.go:74` | yes |
| 4.4 | MEDIUM | D-59 was amended to `rowid`; AC-04.4 and design §4 still state the rule F37 refuted | `mr-003-requirements.md:340`, `mr-003-design.md:108` | yes |
| 4.5 | LOW | `moduletree.SkipDir` skips the module root when the root's own name starts with a dot, silently disabling two module-wide guards | `internal/moduletree/moduletree.go:66-70` | yes |
| 4.6 | LOW | The mutation §6 records for item 12 turns a different item's test red; the AC-09.1 matrix has no recorded falsifier | `mr-003-findings.md:280` | yes |
| 4.7 | LOW | A read failure inside a write transaction is published as a failure to write, with a subject id nothing tried to insert | `internal/coordination/store.go:161, 609` | yes |
| 4.8 | LOW | The empty-workspace guard item 5 added inside `(*Store).attribute` is killed by no test and never executes | `internal/coordination/store.go:149` | yes |
| 4.9 | LOW | The F37 regression fixture uses checkpoint ids this binary cannot mint, and its comment claims otherwise | `internal/coordination/checkpoint_order_test.go:46` | yes |
| 4.10 | LOW | §4's corrected F17 bullet credits the fix to a test that does not contain it | `mr-003-findings.md:148` | yes |
| 4.11 | LOW | The F48 figure attaches the 1,000-task measurement to 20,000 tasks, in three places | `mr-003-findings.md:319`, `internal/coordination/store.go:30` | yes |
| 4.12 | LOW | A minted session's `started_at` is now stamped after the row it attributes | `internal/coordination/store.go:155` vs `:192, :371` | yes |
| 4.13 | LOW | The `EXISTS` rewrite degrades to a full checkpoint scan for a project with no checkpoints in a shared database | `internal/coordination/store.go:44-48` | yes |
| 4.14 | LOW | Item 11 is recorded as "(test only)"; its commit changes production behaviour | `mr-003-findings.md:279` | yes |
| 4.15 | LOW | AC-04.6's amendment claims all six constructors carry the id in `why`; one does not, and two more were added after the sentence | `mr-003-requirements.md:357` | yes |
| 4.16 | LOW | The task list records fifteen commits for the remediation; there are thirteen | `mindrail-0.1-task-list.md:180` | yes |
| 4.17 | LOW | The wrong `.sql` comment stays wrong with no marker reachable from the file, and a zero-risk third option was not weighed | `migrations/000002_coordination.sql:75-78` | no |

---

### 4.1 — MEDIUM. The upgrade gate un-gates the coordination commands on a schema that has no tables

**What it is.** Item 7 replaced the blanket workspace-lookup skip `if
a.subject.PendingCount > 0` with `if !a.schemaHasWorkspaceTable()`. That is right
for the workspace reading F01 was about, and the item's own test proves it. It
also un-gates everything downstream. On a database that has migration 1 and not
migration 2, the workspace row is now found, `readCoordination` runs, and
`coordinationScope` hands the six coordination commands a store over a database
with no `sessions`, `tasks` or `checkpoints` table. `coordinationScope` has no
schema test of its own — it checks for a nil store and an empty workspace or
project id, and both now pass — and item 2's startup verdict does not fire,
because schema-behind is DEGRADED rather than a halt.

**What a person sees.** A user upgrades to the MR-003 binary. Their repository is
at schema 1 with migration 2 pending, which is the state every existing
repository is in on its first run after the upgrade. Before running `mindrail
init` they run `mindrail task list --json` and get exit 1:

```json
{"code":"COORDINATION_READ_FAILED",
 "next_action":["Run `mindrail doctor` to check the runtime database, then re-run the command."],
 "cause":"SQL logic error: no such table: tasks (1)"}
```

They run `mindrail doctor`, which exits 0 and clears nothing. They re-run, and
get a byte-identical failure — verified with `diff`. Meanwhile `mindrail status`
on the same bytes at the same moment is `ok:true` at exit 0, calls the condition
`MIGRATION_FAILED` / "Schema is behind this binary", and prints `next_action:
["mindrail init"]`, which does clear it: `task list` returns `ok:true`
afterwards. One condition, two codes, two exit classes, two remedies. All six
commands reproduce it — `task list`, `task show`, `task open`, `task state`,
`session open`, `checkpoint write` — two on the read side and four on the write.

**Evidence.**

```
go build -o /tmp/mindrail-b ./cmd/mindrail
# in a fresh git repo:
/tmp/mindrail-b init --json
sqlite3 .git/mindrail/mindrail.db "DROP INDEX IF EXISTS idx_checkpoints_task;
  DROP INDEX IF EXISTS idx_tasks_project; DROP TABLE IF EXISTS checkpoints;
  DROP TABLE IF EXISTS tasks; DROP TABLE IF EXISTS sessions;
  DELETE FROM schema_migrations WHERE version=2;"   # = downgradeToSchemaOne
/tmp/mindrail-b task list --json ; /tmp/mindrail-b doctor ; echo $?
/tmp/mindrail-b status --json
```

The counterfactual is what pins it on the pass. Revert `internal/bootstrap/app.go:921`
to `if a.subject.PendingCount > 0 {`, rebuild, run on the same repository **at the
same path** — the answer is `COORDINATION_UNAVAILABLE` with `next_action: ["Run
`mindrail init` in this worktree, then re-run the command."]`. A binary built at
`2ac0324` gives the same. So the pass converted a misleading-but-working remedy
into a true-but-unworkable one. (One verifier's first A/B was a false negative
because copying the repository to a new path broke the workspace row's
`worktree_root` match and both arms answered `COORDINATION_UNAVAILABLE` for a
path reason; the divergence appears only when both arms run at one fixed path.)

**Why it survived.** `internal/cli/upgrade_test.go:110` builds exactly this
fixture — `downgradeToSchemaOne` — and `grep -rn downgradeToSchemaOne internal/`
returns one caller, which drives `status`, `doctor` and `init` and no coordination
command.

**Two qualifications, recorded because they set the severity.** `doctor` is not
entirely a dead end: its human output prints "Next: mindrail init" under the
schema block, and `doctor --json` carries `next_action: ["mindrail init"]` inside
the nested `runtime_db` component. So a reader who runs doctor and reads its body
is one hop from the fix; what fails is the literal "then re-run the command", and
a caller that branches on doctor's exit code (0) loops forever. And AC-06.7's
round-1 amendment deliberately sanctions exit 1 for the coordination commands
against exit 0 for the reporting commands, so the exit-class split alone is not
the defect. The defect is the remedy. That is why this is MEDIUM and not HIGH.

The rule the pass wrote for itself in the same commit is the one it missed: `0d5209a`
added to the requirements the sentence that "a decision that adds a migration has
to name every place that reads 'pending' as 'absent'". It named two places. The
third is the one that change created.

---

### 4.2 — MEDIUM. Item 13's substitute sweep cannot reach any read path

**What it is.** The brief asked for `emit` to refuse an uncoded envelope. The
pass substituted `TestNoCoordinationFailureReachesTheWireUncoded`, which drives
`task list --json` over the MR-001 agreement matrix. Because item 2 put the
startup `Diagnosis` verdict first, every broken matrix row is answered before any
store read runs, so the sweep cannot reach a single one of the ten `readFailed`
sites in `store.go` — the exact paths item 13 and F44 existed for. Only one read
wrap in the binary is guarded by any test at all: `LastCheckpoint`, via
`TestADamagedRowIsReportedWithACode`, which damages the `checkpoints` table.
`FindTask` and the three `ListTasks` wraps — the remaining wire-facing read paths
— are guarded by nothing.

The test's own comment at lines 190-192 claims the sweep is how F44's empty code
would have been found. In the code it ships in, that is false.

**Evidence, and it is worse than the finding as filed.**

```
# strip two wraps: store.go:309 readFailed(...) -> err ; store.go:338 -> scanErr
go test ./... -count=1        # exit 0, 18 packages ok, 0 FAIL
```

Then build and run: `init`, `--json task open --title a`, `sqlite3 ... "UPDATE
tasks SET created_at='yesterday'"`, `--json task list` →
`{"code":"","impact":"","next_action":[]}` at exit 1, with a Go parser format
string as the `why`. Same for `task show`. Pristine HEAD answers
`COORDINATION_READ_FAILED` on both. One verifier went further: strip **nine of
ten** (keeping only `LastCheckpoint`) and the whole suite is still green; strip
all ten and the sweep *still passes* — only `TestADamagedRowIsReportedWithACode`
dies. That is the direct proof that the sweep cannot make any read path fire.

**Why it matters.** HEAD's behaviour is correct today, so this is guard coverage
and not a live defect. But nine new guards on wire-facing read paths are held by
nothing, and AC-10.1 ("every guard is checked in both directions") and AC-10.3
("every new assertion is checked by mutating the code it guards") are frozen,
unamended criteria that this item does not meet. §6 records item 13's mutation as
the singular "reverting one read wrap — `code: ''`", which reads as validating the
item when it validates one wrap of ten.

**Two concrete completions were available**, and are the brief in §8: an `emit`
backstop beside `refuseUnrepresentableJSON` in `internal/cli/output.go`, or the
cheaper test-side fix — add semantic-damage rows to the matrix and drive the sweep
through `task show` as well as `task list`.

---

### 4.3 — MEDIUM. The path form of `unrepresentableError` is asserted by nothing

**What it is.** Item 3 split `unrepresentableError` into a location (path) form
and a stored-field form, keyed off `locationTypes` at
`internal/cli/representable.go:74`. The record names one mutation — collapsing
everything into the location form — and that one is caught, by
`TestAStoredValueIsNotReportedAsAPath`. The opposite collapse is caught by
nothing.

**What a person sees.** Set both entries of `locationTypes` to `false`. `go test
./...` is green in every package; so is `make verify`, smoke tests included. Then
run `mindrail init --json` in a repository whose directory name contains a `0xFF`
byte, and the binary prints:

> Or correct the stored value at `Repo.common_dir`. It is not a path, so
> renaming anything on disk does not reach it; a `--json` command cannot write
> such a value any more.

about `/tmp/badpath/repo\xff/.git`, which *is* a path, and which renaming does
fix — as the sibling test `TestTheUnrepresentablePathRemediesBothWork/rename_the_offending_path`
proves by performing the rename and asserting the command then succeeds. The
suite contains a test that carries out the remedy the shipped message would deny,
and nothing connects the two.

**Why it survived.** `TestTheUnrepresentablePathRemediesBothWork` constructs the
rename itself rather than reading it out of `next_action`, and asserts only the
exit code. `TestInitRefusesAnUnrepresentableRepositoryBeforeWriting` asserts only
`payload.Code`, which is `CodePathNotRepresentable` in both forms. The unit tests
in `representable_test.go` use anonymous local structs and so never exercise
`locationTypes` at `true`. Repo-wide, "rename the offending path" appears in test
files only as a subtest label and in comments, never in an assertion.

**Evidence.** Edit `internal/cli/representable.go:75-76` to set both map values
`false`; `go test ./...` → all packages ok. `go build -o /tmp/mrD ./cmd/mindrail`;
`git init` a directory named `$(printf 'repo\xff')`; `/tmp/mrD init --json` →
the stored-value message above. Restore; suite green again. AC-10.1 asks for
both directions of every guard; this one has one.

---

### 4.4 — MEDIUM. D-59 was amended to `rowid`; two other frozen statements of the same rule were not

**What it is.** Item 4 changed "the newest checkpoint" to `ORDER BY rowid DESC`
at `internal/coordination/store.go:432` and `:48`, and amended decision D-59 in
place with F37's argument that a minted id is monotonic only within one process.
The same rule is stated in two other frozen places and neither was touched:

- `AC-04.4` (`mr-003-requirements.md:340`) still reads "`LastCheckpoint` returns
  the newest by `checkpoint_id`", with no `*Amended during …*` note, unlike
  AC-04.1 and AC-04.6 two lines above and below it which this same pass amended.
- design §4 (`mr-003-design.md:108-110`) still reads "'The last checkpoint' is
  the newest by id, and the id is time-sortable, so ordering does not depend on a
  timestamp column whose resolution can tie" — verbatim the argument F37 refuted.
  The pass had this file open; it added fourteen lines to §3 for F16.

The requirements' own header says the design wins on intent, so the contract now
states the superseded rule from its authoritative side.

**What makes this more than stale prose.** Implement AC-04.4 exactly as written —
`ORDER BY checkpoint_id DESC` at both call sites — and the test whose comment
says "is AC-04.4 and decision D-59" (`store_test.go:429-431`) stays **green**. It
cannot tell the two rules apart, because within one process the fixture's ids and
rowids agree, which is precisely F37's blind spot. Only
`TestTheNewestCheckpointIsTheOneWrittenLast`, labelled "is finding F37" and bound
to no criterion, goes red — on all three readers. So a passing test in this
repository now demonstrates that AC-04.4's literal claim is false of the shipped
code, and no test bound to AC-04.4 can object.

**The failure a person reaches.** MR-004 adds a second "newest checkpoint" reader
— the newest checkpoint written by a given session, for lease attribution. Its
implementer reads AC-04.4 and design §4, the two places the frozen contract
states the ordering rule, and writes `ORDER BY checkpoint_id DESC`. Two agents in
two processes write in the same millisecond, their ids differ only in 80 random
bits (`internal/identity/id.go`: 48 bits of millisecond, 80 of randomness, with a
process-local entropy counter), and the new reader returns the superseded note
half the time. F37 reproduced with the contract as its authority.

Definition-of-done item 1 requires every criterion to be met or recorded as not
met with the reason. `grep -rn "AC-04.4" docs/` finds it nowhere in
`mr-003-findings.md`, so AC-04.4 is now neither.

**Evidence.** `grep -n "checkpoint_id\|rowid" docs/engineering/mr-003-requirements.md
docs/engineering/mr-003-design.md internal/coordination/store.go` →
requirements:187 `rowid DESC`, requirements:340 `checkpoint_id`, design:108
"newest by id", store.go:432 `ORDER BY rowid DESC`. `git diff c29efa3~1..HEAD --
docs/engineering/mr-003-requirements.md | grep -c 'AC-04.4'` → 0.

*Out of scope, and noted so round 3 does not re-file it:* design §5's
`idx_checkpoints_task(task_id, checkpoint_id)` is accurate to the real migration,
which cannot be edited (§4.17). It is a now-unused-for-ordering index, not doc
drift.

---

### 4.5 — LOW. The shared tree-walk helper skips the module root when the root's own name starts with a dot

**What it is.** `moduletree.SkipDir` applies its name rules (dot-prefix,
`vendor`, `testdata`, `graphify-out`) *before* the `if path == root { return
false }` escape, so that escape is unreachable whenever the module root's own
basename matches a rule. `filepath.WalkDir` calls the callback for the root
first, so `SkipDir(root, root) == true` terminates the whole walk. Two of the
four walkers the pass consolidated onto this helper have no vacuity guard and
therefore pass while asserting nothing: `internal/coordination`'s D-55
single-statement guard, where an empty offender list is a pass, and
`internal/storage`'s driver confinement, where no files visited means no
`t.Errorf`.

The helper's own unit test asserts a claim the implementation does not hold:
`moduletree_test.go:57` is named "the root itself is never skipped" and only
exercises a `t.TempDir()` root, whose basename is numeric.

**What a person sees.** Check the module out into any directory whose name begins
with a dot — `git worktree add ../.review`, a clone into `~/src/.mindrail`, a CI
workspace named `.build` — and run the suite from inside it. A second full
statement of the seven task states added to `internal/cli`, and an `import _
"modernc.org/sqlite"` added to `internal/doctor`, both go undetected while `go
test ./internal/coordination/ ./internal/storage/` is green. `make check` still
goes red overall, but on four `internal/cli/arch_test.go` lines and one in
`internal/bootstrap`, all saying "no first-party packages were parsed; the graph
would be vacuous" — messages that point at nothing related to the two injected
defects.

**Evidence.** rsync the repo (minus `.git`, `graphify-out`, `.claude`) to
`/tmp/x/.mode-b` **and** `/tmp/x/mode-b`. In both, add `internal/cli/lifecycle_copy.go`
declaring all seven state strings and `internal/doctor/driver_leak.go` with the
driver import. `go test ./internal/coordination/ ./internal/storage/` fails in
`mode-b` on both guards and is `ok` in `.mode-b`. Isolating the mechanism: a walk
rooted at a directory named `.review` visits 0 files; after `os.Rename` to
`review` it visits 2.

**Attribution, precisely.** `internal/storage` was already vacuous in a dot-named
root before the pass — its old `shouldSkipDir(entry.Name())` had no root escape at
all. `internal/coordination`'s old walk skipped only `.git` and `testdata` by
exact name, so a dot-named root **was** walked correctly at `c29efa3^` and is not
walked at all now. The D-55 vacuity is newly created by item 1 — the item whose
stated purpose was to keep the D-55 guard working — as is the false unit-test
assertion. The fix is one line: move the `path == root` escape above the name
check.

---

### 4.6 — LOW. The mutation recorded for item 12 falsifies a different item's test

**What it is.** §6 opens with "Every item's stated mutation was applied and
confirmed **red**", and its column is headed "The mutation that turns the new
test red". Item 12's cell reads "reverting item 2 — the startup rows red". Item
12's new test is `TestTheFourCoordinationRefusalsAgreeAcrossBothRenderings`,
whose four rows are AC-09.1's four conditions — unknown task, unknown session,
illegal transition, never-initialised repository. None is a halted startup, so
reverting item 2's `a.Diagnosis` call does not touch any of them.

What goes red is `TestACoordinationCommandNamesAHaltedStartupTheWayStatusDoes`,
which item 2 added in `7ea8d83` and which row 2 of the same table already claims
as its own proof. The two rows appear to name one mutation because both items
wrote to `coordination_agreement_test.go`.

**Evidence, both directions.** Delete the three-line block at
`internal/cli/coordination.go:99-101` and run the four named tests: exactly one
fails — `TestACoordinationCommandNamesAHaltedStartupTheWayStatusDoes`, all six
subtests; the item-12 matrix passes all four. Then the mirror image: replace
`return renderSection(w, "Next", payload.NextAction...)` at
`internal/app/errors.go:199` with `return nil` — all four matrix rows go red and
the halted-startup test passes all six. The two mutations are disjoint. So a real
falsifier exists and is unrecorded, while the recorded one provably cannot
falsify the guard.

**Why it matters more than a stray table cell.** Item 12's commit `b4629cd` is
test-only — `coordination_agreement_test.go` plus the requirements doc, zero
production code — so no mutation inside its own change could falsify it; a
falsifier has to reach production code elsewhere, and the record never names one.
AC-10.3 and definition-of-done item 3 both require a recorded mutation for every
new guard, §3's mutation ledger was never extended (`grep -rn
"AgreeAcrossBothRenderings" docs/` returns nothing), and item 12 is the answer to
F13 — the criterion round 1 found written and never implemented.

*Correction of detail:* the test is at `coordination_agreement_test.go:249`; line
237 falls in its doc comment.

---

### 4.7 — LOW. A read failure inside a write transaction is published as a failure to write

**What it is.** `(*Store).attribute` returns its errors bare, and `(*Store).adopt`
turns anything that is not already a domain payload into
`COORDINATION_WRITE_FAILED` carrying the **outer** subject id. Item 5 widened
`requireSession` from `SELECT session_id` to a full `scanSession`, which parses
`started_at`; its scan-error arm returns a bare error. So a *read* failure
surfaces as "the task could not be written to the runtime database", with a
`subject_id` naming a task no statement ever attempted, when nothing was
attempted and a row could not be read.

**What a person sees.**

```
mindrail session open --json
sqlite3 .git/mindrail/mindrail.db "UPDATE sessions SET started_at='yesterday'"
mindrail task open --title t --session SES-… --json
```

→ exit 1, `COORDINATION_WRITE_FAILED`, why "the task could not be written to the
runtime database", `metadata.subject_id: TSK-01M23FTGYQ…` — a task id that exists
nowhere. The session that could not be read appears only in free-text `cause`.
The same shape reaches a direct store caller: `OpenTask(ctx, projectID,
coordination.MintFor("WS-NOTHING"), "a task")` returns the same code for a
foreign-key failure entirely about the workspace, since `noWorkspace` only catches
the empty string. AC-11.4 names this package as the surface MR-014's MCP tools
call with no `coordinationScope` in front of it.

**Why this is the pass's.** At `2ac0324` the timestamp parse lived in the
since-deleted `FindSession` and this condition surfaced *uncoded* — F44's shape.
Item 5's commit `6000d63` created this path; item 13's commit `a20c781` created
`COORDINATION_READ_FAILED` for exactly "a row the runtime database holds and
could not answer for" and did not wire it here. This is the one read path item
13's sweep does not cover, because item 5 created it two commits earlier. The
pass moved the path from uncoded to miscoded rather than to the code it created
for it.

Three of the pass's own artifacts classify this condition as `READ_FAILED`:
`internal/app/code.go:128-130` ("a timestamp column that does not parse"), the
AC-05.1 amendment, and §6 row 13. The shipped behaviour contradicts all three.
It stays LOW because the code is registered, the exit class is right, the
transaction rolls back and the cause is preserved — materially better than the
uncoded envelope it replaced — and it is reachable only through corrupted data,
an out-of-range host clock, or a future direct caller.

---

### 4.8 — LOW. The empty-workspace guard item 5 added is killed by no test and never executes

**What it is.** Item 5 moved minting into the write's transaction and added a
second `noWorkspace` guard at the new call site, justified by AC-11.4. Deleting
it leaves `go test ./...` green in every package, and `noWorkspace`'s whole body
has execution count 0 under `-coverprofile` — so neither of its two call sites is
ever reached by any test. §4 records "`OpenSession`'s empty-workspace guard" as an
open survivor; this is a second, distinct guard at a different call site with a
different message ("a session minted for a write"), created by this pass, and
listed nowhere.

**Evidence, with both arms measured.** Delete
`internal/coordination/store.go:148-150`; `go test ./...` → every package ok.
Then a two-arm probe on `OpenTask(ctx, projectID, coordination.MintFor(""),
"a title")`: guard present → `COORDINATION_UNAVAILABLE`, "a session minted for a
write needs a worktree to belong to, and none was given"; guard deleted →
`COORDINATION_WRITE_FAILED` wrapping "FOREIGN KEY constraint failed (787)". Both
write zero rows. No test in the repository distinguishes them.

Round 1's F26 fix said "code it **and a two-arm test** — or delete it". The pass
took the code-it branch, for two guards, and shipped neither test.

---

### 4.9 — LOW. The F37 regression fixture uses ids this binary cannot mint

**What it is.** `internal/coordination/checkpoint_order_test.go:45-46` says "Both
are the shape `identity.NewID` mints; only their order is arranged". AC-01.1
fixes the id at prefix + `-` + 26 Crockford characters; `firstID` carries 28 and
`secondID` carries 29, and they are not the same length as each other, which no
fixed-width format admits. They also diverge at character 8 of the body, inside
the 10-character ULID millisecond field, so they encode two *different*
milliseconds — whereas the condition F37 describes is two ids sharing the
millisecond prefix and differing only in the 16 characters of per-process
entropy.

**Evidence.** `python3 -c "print(len('CKP-01M2337ZZZZZZZZZZZZZZZZZZZZZ')-4,
len('CKP-01M23370000000000000000000000')-4)"` prints `28 29`; AC-01.1 requires 26.
A real minted id printed by the same test run's failure output —
`SES-01M23FWDX20QV4D7HZDAGK1BKT` — has a 26-character body.

**What is not wrong.** The assertion is correct and falsifiable: replacing both
`ORDER BY rowid DESC` sites with `checkpoint_id DESC` turns
`TestTheNewestCheckpointIsTheOneWrittenLast` red on `LastCheckpoint`, `Handover`
and `Summarize` together. The guard is sound. What is wrong is the fixture's
stated fidelity to the production condition, in the document a later reader
consults to decide whether the F37 case is still covered. The file's header
sentence ("the shape two processes in one millisecond produce … two checkpoints
inserted in a known order, whose ids sort the other way") is defensible as an
apposition; the line-45 comment is not.

---

### 4.10 — LOW. §4's corrected F17 bullet credits the fix to a test that does not contain it

`mr-003-findings.md:148` ends "The four commands are in `TestHumanOutputGolden`
now, which executes them to produce the bytes it pins." They are not.
`TestHumanOutputGolden` (`internal/cli/contract_test.go:2459`) still loops over
`[]string{"status", "doctor"}` — the same two commands the sentence three lines
earlier correctly reports it looped over. The four goldens the pass added are
produced by a different, newly written test, `TestCoordinationHumanOutputGolden`
at `contract_test.go:2425`, which is where all four `*_human.golden` assertions
live (line 2448).

The fix itself is real: the four goldens exist and both tests pass. The
cross-reference is not, and the document never names the correct test anywhere,
so the error is not self-correcting from context. Searching for
`TestHumanOutputGolden` does not match `TestCoordinationHumanOutputGolden`, so a
round-3 auditor running `grep -n -A6 'func TestHumanOutputGolden'` sees only
status and doctor and would file "F17 not fixed".

Graded LOW on this repository's own precedent: round 1 met this exact defect
class five times in this file and consolidated it at LOW with the note that
"nothing reachable is wrong; the cost is one `grep`", warning that parallel
auditors inflate documentation drift. It is worth recording only because the pass
wrote a false cross-reference into the paragraph it wrote to close F15's sibling.
§6's ledger row for item 15 names no test and is clean.

---

### 4.11 — LOW. The F48 figure attaches the 1,000-task measurement to 20,000 tasks

§6 ("1.2 ms to 10 µs at 20,000 tasks") and `internal/coordination/store.go:30`
("1.2 ms at 20,000 tasks with 200 checkpoints") both attach 1.2 ms to 20,000
tasks. Round 1's own F48 table (`mr-003-audit-round-1.md:963-968`) puts 1.20 ms
at **1,000** tasks and 28.98 ms at 20,000. The number was copied from the wrong
row of the table it cites.

Two independent re-measurements, on databases built through the project's own
`storage.Open` and migrations and under the plan round 1 recorded, put the
isolated pre-fix JOIN at 7.27 ms (min) / 19.4 ms (mean) and 23.64 ms (min of 200
warm iterations under load) at 20,000 tasks, against 1.23 ms and 3.25 ms at
1,000. No measurement basis puts the pre-fix query at 20,000 tasks near 1.2 ms.

**There is a third copy the finding did not name:**
`internal/coordination/query_plan_internal_test.go:21` carries the identical
sentence in the doc comment of the test that closes F48. So the wrong number sits
in the remediation record, in a production comment, and in the regression test's
own rationale.

The direction is conservative — the real improvement is roughly 240x, not 120x,
so nobody over-credits the fix — but the figure understates the problem F48
measured by about 20x, and it is the number round 3 will cite.

---

### 4.12 — LOW. A minted session now starts after the row it attributes

`OpenTask` takes `now := s.clock.Now().UTC()` before opening the transaction
(`store.go:192`) and `WriteCheckpoint` does the same (`store.go:371`), while
`(*Store).attribute` reads the clock a second time *inside* the transaction for
the minted session's `StartedAt` (`store.go:155`). The mint used to happen first,
in the CLI, so the session always started before the row it opened; now it always
starts after it. Both fields are published by `task open --json`, so the
inversion is on the wire.

```
mindrail --json task open --title x
# data.task.created_at    …04.553693112Z
# data.session.started_at …04.553898714Z   (205 µs later)
sqlite3 … "select count(*) from tasks t join sessions s on s.session_id=t.opened_by
           where s.started_at > t.created_at"   -> 1 of 1
```

Under the store suite's injected `stepClock` the same inversion is a full minute
wide. A/B against a binary built at `6000d63~1` gives `0 of 1`, so the pass
introduced it.

**Why it is LOW and not refuted.** No requirement, test, golden or consumer
anywhere orders `sessions.started_at` against `tasks.created_at` —
`grep` over `docs/` finds `started_at` once, in design §5's DDL. But the
inversion was not required by the fix: brief item 5 asked for the session to be
*resolved inside the transaction that attributes it so a refusal rolls it back*,
which is a placement requirement on the INSERT, not a second clock read. Both
call sites already hold the pre-captured `now`; passing it into `attribute` would
satisfy the brief verbatim and preserve ordering at zero cost. It is an avoidable
side effect of the fix, deterministic and wire-visible, and unrecorded in §6.

---

### 4.13 — LOW. The `EXISTS` rewrite degrades to a full checkpoint scan for a project with no checkpoints

The rewritten query's cost is O(checkpoints scanned until one matches the
project). For the normal case — one project per database — the newest checkpoint
is the first row met and the comment's "stop at the first one" holds exactly. When
the registered project has **no** checkpoints and the database carries another
project's history, the reverse scan probes every historical checkpoint and finds
nothing, on every read-only startup. The old JOIN in the same state cost
O(tasks of the queried project), which is nearly nothing.

The state is reachable through shipped commands. Project identity is keyed on the
absolute git common dir (`internal/workspace/store.go:194-218`) and the database
travels inside `.git/mindrail`, so moving a repository directory and re-running
`init` mints a second project row over the old history. Two auditors reproduced
this end to end: after `mv repoA repoB` and `init`, the `projects` table holds two
rows over one database and `status` runs this query for the checkpoint-less one
until its first checkpoint is written.

Measured, on the real migrated schema for an empty second project: 682 µs at 200
checkpoints, 5.02 ms at 2,000, 44.8 ms at 20,000, 113 ms at 50,000 — flat ~2.3 µs
per checkpoint — against 115-216 µs for the old JOIN at every size. The plan is
`SCAN c` + an `EXISTS` probe, so the `LIMIT 1` never fires.

LOW because the window is bounded (until the first new checkpoint is written),
the absolute cost at plausible checkpoint counts is under 5 ms, and a user in
that state has a louder problem anyway — their task list is empty, because the
tasks stayed with the old project row. What the finding correctly identifies is
that the pass traded an unconditional cost for a conditional one and recorded only
the winning half, in both `store.go`'s comment and §6.

---

### 4.14 — LOW. Item 11 is recorded as "(test only)" but its commit changes production behaviour

§6 row 11 opens with "(test only)", the same marker rows 8, 9 and 10 use and which
is accurate for their shared commit `79533ac` (three test files, nothing else).
Commit `2d8ee73` is not test-only: alongside the two new assertions it adds a
`mint bool` field to `Attribution`, changes `attribute`'s branch condition from
`by.handle != ""` to `!by.mint`, and replaces the error returned for a mint with
no workspace id — `internal/coordination/store.go` +9/-3. The error
`MintFor("")` produces moves from `SESSION_NOT_FOUND` to (after item 13 coded it)
`COORDINATION_UNAVAILABLE`. Measured across three builds: `SESSION_NOT_FOUND` at
`6000d63`, `COORDINATION_WRITE_FAILED` at `2d8ee73` itself, `COORDINATION_UNAVAILABLE`
at HEAD.

The behaviour is unreachable from today's CLI, because `MintFor`'s only
non-definition call site (`internal/cli/coordination.go:214`) passes `s.space.ID`
behind `coordinationScope` — which is why nothing caught it. The production change
also belongs to item 11 rather than to some unrelated hunk: the §4
surviving-mutation analysis that `2d8ee73` added is tagged "(added by the round-1
remediation, finding F21)", and its claim that `requireSession`'s empty-id guard is
an equivalent mutant is only true *because* of the dispatch change made in that
same commit. So the row is false at the granularity §6 itself speaks at.

Row 13's plural "the empty-workspace guards are coded" covers the final coded
state, and the `noWorkspace` doc comment explains it — but no record row
attributes the creation of the second guard site, the mint-flag semantics, or the
error-class transition. The only item-level statement about that surface's
provenance is row 11's "(test only)".

---

### 4.15 — LOW. AC-04.6's amendment claims all six constructors carry the id in `why`

The amendment closes with "All six carry the id in `why` and in `metadata`, which
is where a caller reads it." The sixth of F07's six is
`COORDINATION_WRITE_FAILED`, and `writeFailed`
(`internal/coordination/errors.go:184-191`) builds its `why` as
`fmt.Sprintf("%s could not be written to the runtime database", what)` — `what`
is a description such as "the agent session", never the id; the id reaches the
payload only through `WithMetadata("subject_id", id)`. Round 1's own F07 table
(`mr-003-audit-round-1.md:1076`) already recorded that row as carrying no id.

A probe over all eight current constructors, run in-package: five carry the id in
`why`; `writeFailed` and `readFailed` do not (metadata only); `noWorkspace`
carries it in `why`, `metadata` and `next_action` alike — nowhere. `readFailed`
and `noWorkspace` were added by item 13 in `a20c781`, and the sentence was
written two commits later in `e7e946c`, so "six" was already stale when it was
typed.

The operative clause immediately above — "the id must appear in the payload —
`why`, `metadata` or a `next_action`" — is satisfied by everything except
`noWorkspace`, which has no offending id to name. So no runtime behaviour is
wrong; what is wrong is the summary sentence a future reader takes as the
specification. §6 files F07 under item 15, "the ledger", meaning this amendment
*is* the entire remediation for F07 and it misdescribes the code it freezes.

---

### 4.16 — LOW. The task list records fifteen commits for the remediation; there are thirteen

`docs/engineering/mindrail-0.1-task-list.md:180`, written by the pass in
`6680e32`, says "**Remediasyon** (`c29efa3..e7e946c`, on beş kalem, on beş
commit)". `git rev-list --count c29efa3..e7e946c` returns 12; `c29efa3~1..e7e946c`
returns 13. All fifteen items are present in §6; brief items 8, 9 and 10 were
delivered together in `79533ac`. Only the commit arithmetic is wrong.

The count fifteen is true only of `c29efa3~1..24069aa`, which reaches HEAD and
gains its extra two from the record commit and a graphify refresh — neither a
brief item.

It matters because the record presents the log as one commit per item in the
brief's order, which invites reconstruction by `git log --oneline`. A round-3
auditor doing that counts thirteen subjects against fifteen items and must decide
whether two items were skipped or two commits squashed — and the bundled commit is
the one carrying item 10, the F04 test the pass deviated from the brief on.

---

### 4.17 — LOW. The wrong `.sql` comment stays wrong, with no marker reachable from the file

*The only confirmed finding not introduced by the pass.*

The deviation's justification is true — `internal/migration/load.go:141`
checksums the entire file body including comments, both gates compare it, and a
comment-only edit empirically produces `MIGRATION_CHECKSUM_MISMATCH` on `status`
and `task list` against an existing database while the clean binary stays READY.
Editing the applied file is correctly ruled out. But the pass framed the choice as
binary — edit the file, or annotate the constant in `store.go` — and the `.sql`
itself carries no pointer to either correction (`grep` for `store.go`,
`coordination/store` or `internal/` in the file returns nothing).

A third, checksum-free option existed and is not shown to have been weighed:
`migrations/README.md`. `load.go:113` skips non-`.sql` entries and `embed.go`'s
glob is `*.sql`, so a README is neither checksummed nor embedded. Verified: adding
one leaves `go build`, the full suite and a rebuilt binary against a
previously-initialised database all clean.

The clause that carries the weight is the second one. **With the comment mutated,
`go test ./...` is fully green.** No test pins the shipped file's sha256 —
`load_test.go` re-derives the hash from the same bytes it loaded, and the two
checksum-mismatch tests use synthetic fixtures. So an engineer who spots the wrong
comment and helpfully corrects it ships `MIGRATION_CHECKSUM_MISMATCH` to every
existing user's database with a green gate.

---

## 5. What was refuted, and why

Thirteen claims died. They split cleanly, and the split is the useful part:
**nine were real and out of the graded scope; four were not real.** Round 1's
three refutations all died the same way — a sentence graded outside the scope it
states for itself — and that remains the dominant cause here, but a second cause
appears in this round which did not exist in the last: **grading the pass for
doing exactly what the brief told it to do.**

**Real, reproducible, and outside the scope this round grades (9).**

- *"The coordination gate takes the whole doctor verdict, not the startup
  verdict."* Filed HIGH; reproduced exactly on three A/B pairs. Refuted because
  the brief said to take it "the way `status.go:48`, `init.go:72` and
  `doctor.go:51` take it" — and all three take the whole `a.Diagnosis(ctx)`
  verdict, unfiltered. ERROR-is-fatal-everywhere is D-14, frozen in MR-001 and
  already governing the other three commands. What survives is a doc-comment
  mislabel at `coordination.go:82` and a genuine open design question — should a
  pure read survive an ERROR-but-completed startup — that deserves a decision
  record in MR-004, not a defect here.
- *"Item 5's CLI-boundary half ships unguarded."* True: deleting both
  `requireFlagText` blocks leaves all 19 packages green and reverts the envelope
  to its pre-remediation shape. Refuted on two grounds. The store still makes the
  identical blank-title judgment *before* `storage.InTx`, so under the mutant
  nothing is written and the reverted sentence "Nothing was written." is now
  **true** — F02's defect does not return. And brief item 5 names exactly one
  mutation, which the delivered test implements verbatim, while explicitly ruling
  out payload assertions for it ("it must count rows"). A LOW coverage note on an
  above-contract envelope improvement; carried into §8 item 8.
- *"The agreement matrix structurally cannot express a condition where `status`
  exits 0."* The `t.Fatalf` at line 114 does abort such a row. But
  `startupHaltedConditions` states its own membership rule ("states that stop the
  startup sequence"), a schema-behind repository does not stop it, and the
  disagreement claimed cannot be expressed by the matrix's own `answer` type
  regardless — status's "mindrail init" on that state lives in the success `data`
  block, which `answerFrom` never reads. The underlying condition is §4.1.
- *"`COORDINATION_READ_FAILED` carries a remedy that cannot be carried out."*
  Fully reproduced: doctor exits 0, sees nothing, and the re-run fails
  identically. Refuted because F44's own Fix paragraph prescribes that exact
  remedy — "a `COORDINATION_READ_FAILED` carrying the doctor remedy" — in the same
  finding whose reachability paragraph already recorded that the trigger is
  semantic damage `PRAGMA integrity_check` cannot see. The pass executed the
  brief. AC-09.2 scopes remedy-clears-condition to matrix rows. Worth a wording
  change in MR-004, not a defect of this pass.
- *"The `MetadataNothingRead` convention was applied to one of three no-lookup
  branches."* True at the word level. But `observationOf`'s own doc defines an
  inspected-and-empty database path as a real observation, the sibling `runtime`
  block grades the identical absent file identically, and both untouched branches
  are MR-001 code (`git blame`: `a399c6c`). The distinction the pass drew is
  semantic: where the row *can* exist, "not registered" is an answer nobody has;
  with no database it is entailed and true.
- *"The payload half of the representability walk is hard-coded to the path
  form."* Reproduced: a mistyped task id with an invalid byte is reported as a
  path to rename. But `git show 2ac0324:internal/cli/representable.go` proves the
  pre-pass single form was byte-for-byte the location form — the behaviour is
  unchanged — and deleting the line produces a *different* category error
  ("correct the stored value at `metadata.task_id`" for a value stored nowhere).
  The line is load-bearing for the case the brief's must-not-break clause
  protects. Residue: an over-broad comment on an unpinned line.
- *"`LastCheckpoint` now forces a temp b-tree where its index used to answer
  directly."* The plan claim is literally true and the cost is linear in the
  task's checkpoint history (57 µs → 55.8 ms from 10 to 20,000 checkpoints).
  Refuted because brief item 4 names `rowid` by name as the ordering key, D-59 was
  amended in place to say so, no criterion constrains a query plan, and SQLite
  refuses to index `rowid` — closing it needs a new migration adding a per-task
  sequence, the heavier option the brief offered and did not require. Carried
  forward as MR-019 material.
- *"A migration concurrency test can go red with no code change."* Real, and
  worse than filed: `TestConcurrentInitAcrossProcessesWaits` fails about 1 run in
  8-14 (I reproduced 4 failures in 20 runs), on the gate the pass certifies green.
  The cause is now known and is not scheduling noise: `Migrator.Up` applies each
  migration in its own `BEGIN IMMEDIATE`, so process A can commit v1 and lose v2
  to process B; both then report `len(Applied) > 0` and the ledger stays correct.
  Refuted for this round because the pass touched nothing under
  `internal/migration` or `migrations/`, the test was written in MR-001 when the
  set had one migration (where the invariant was deterministic), and the second
  migration arrived in `f7ac67f`, the first MR-003 implementation commit. §6
  explicitly withdraws `internal/migration` from its own warranty. Carried
  forward in §7 with the fix.
- *"A third `observation` value was published without amending AC-07.1."* True
  that `indeterminate` appears nowhere in the requirements or design. Refuted
  because round 1 already ruled on this sentence under F46 — "AC-07.1 asks only
  for `observation` 'following the existing convention', so this is not a breach
  of the graded ledger" — item 15's ledger does not list AC-07.1, and the shared
  `Observation` type was already three-valued at the freeze commit.

**Not real (4).**

- *"The group-envelope test misses `completion`."* The enumeration really does
  list four names. But `completion` is covered where the deliberate division of
  labour puts it: `contract_test.go:1487` asserts `completion bogus --json` and
  `completion bash extra --json` at exit 2 with `COMMAND_LINE_INVALID`. All three
  named mutations turn the suite red, one of them printing the exact F34 symptom
  the finding predicted would slip through. Cobra returns `ErrHelp` before
  argument validation, so the `RunE` assignment is a single hinge for both
  behaviours; nothing can break one without the other.
- *"AC-09.1's amendment asserts the logical opposite of what §6 says."* The two
  sentences do disagree at a word-level read. But the amendment closes "Recorded
  here so the next milestone decides rather than inherits", disclaiming binding
  force; the reading "neither omission is empty — each has a reason of its own"
  makes it self-consistent; and empirically only *one* of the two invariants is
  vacuous — extending `TestEveryRemedyAboutAFileNamesThatFileAbsolutely` to the
  six coordination commands produces genuine, mutation-killable assertions, with
  every command naming the absolute path.
- *"Item 5's row quotes a failure message the code it describes cannot produce."*
  It can. Restoring the pre-fix arrangement — the eager mint back at the top of
  the command body, with the empty-title judgment back in the store where it still
  lives — printed the record's quote character for character, trailing clause
  included. The disagreement was about what "restoring the eager mint" denotes,
  and the verbatim match settles it: an author who had run only the narrow
  mutation would have had `TASK_NOT_FOUND` on screen and quoted that.
- *"The corrective comment on the query constant misdescribes the migration
  comment."* The migration's error is a conflation of two queries, and a
  conflation is symmetric; "attributes this query to `task show`" and "credits
  `task show`'s query to `status`" describe one error from opposite ends. The
  second clause — "and names the task-scoped one instead" — resolves the pointer
  uniquely against the migration's actual text.

---

## 6. What this round did not look at

Assembled from the nine auditors' own coverage notes, and from what the
remediation record itself declares open. Stated plainly, because this round
graded a fix pass and the surface it did not touch is larger than the surface it
did.

**Graded thoroughly, so not on this list:** all fifteen of §6's stated mutations,
re-run in the priority order given (fourteen red, one not — §4.6); the full diff
of all thirteen code commits mapped item by item; every one of the seven in-place
requirements amendments and the design §3 amendment, traced to the item that
claims it; round 1's F15 greps re-run (`COORDINATION_UNAVAILABLE` now returns 4
hits, `Counts` only an amendment note); the seven-code arithmetic against
`internal/app/code.go` and `envelope_test.go`; item 5's concurrency shape
re-tested at round 1's own width (20 in-process racers, 20 concurrent processes, 8
cross-process checkpoint writes, one claimant, no orphan session); all five
command groups enumerated bare with and without `--json`, plus `completion
zsh|bash` and the runtime `__complete` protocol; the four upgrade states item 7
names, driven through real repositories; `internal/moduletree`'s shipping status
(`go list -deps ./cmd/mindrail` does not contain it; it imports only the standard
library); and D-62's readiness arm, which is sound and produced no finding.

**Not looked at, by anyone in this round:**

- **`-race` and the smoke suite.** `make check` and `make tidy-check` were run
  green; `make verify`'s race gate and smoke targets were run by nobody. Round 1
  ran both.
- **macOS and Windows.** Linux only, in every round so far. §4.5 is a
  path-name defect that a differently-named checkout directory triggers, which
  makes this gap slightly less theoretical than it was.
- **`internal/cli/root.go`'s usage-envelope changes and `internal/status/render.go`
  beyond reading them.** Item 14's F33 and F49 halves were read, not attacked.
- **Item 11's `updated_at` / `claimed_by` column tests beyond reading**, and the
  item-3/6/9/10/11/13/14 mutations as traced by the Readers (the Breaker executed
  all fifteen, so they are covered — but only once each).
- **`Summarize`'s two `readFailed` wraps end to end.** Their failures fold into
  `status`'s `indeterminate` and never reach the wire as error payloads, so §4.2's
  experiments could not exercise them.
- **The CLI-level item-10 test inside the Go harness.** The judge's binary
  experiment is behaviourally equivalent but is not a test in the repository.
- **A second project registered by the fixture.** Still twelve lines away, still
  not written — and §4.13 is a defect that only exists in that state.
- **MR-001's and MR-002's test bodies**, except where a mutation happened to touch
  them.
- **Timing under contention.** Two auditors worked with load averages of 46-83
  from unrelated processes; every performance claim in §4.11 and §4.13 rests on
  min-of-runs and within-run ratios for that reason, and absolute figures should
  not be quoted.

**Still open from the remediation's own record (§6, "What this pass did not
close"), unchanged by this round:**

- The four surviving store mutations listed in §4 — `OpenSession`'s
  empty-workspace guard, the four `.UTC()` normalisations, `Summarize`'s count
  project scope, and the newest-checkpoint scope. Two need a fixture that can
  register a second project. §4.8 adds a fifth that the list does not mention.
- The two `commandsUnderTest` invariants,
  `TestNoDocumentContradictsItsOwnErrorObject` and
  `TestEveryRemedyAboutAFileNamesThatFileAbsolutely`, still not covering the
  coordination commands. This round established that the second of the two would
  **not** be vacuous if extended, contrary to the recorded reason.
- Everything MR-002's Appendix F left open, plus `internal/workspace` and
  `internal/migration` — where this round did find the concurrency-test defect in
  §5, by accident, and stopped there.

**One methodology note, recorded because it cost real time.** Four of the nine
auditors mutated the shared working tree concurrently. Backups collided, one
auditor's `/tmp` copy was contaminated by another's reverted mutation, one
verifier read a file mid-mutation and nearly refuted a true finding as fabricated,
and at least three full-suite runs had to be discarded and redone. Everything
salvageable was salvaged by re-running in `git archive HEAD` copies under `/tmp`,
and the tree is clean at `24069aa`. **The next round should give every Breaker its
own worktree or its own `git archive` copy as a precondition, not as a recovery
step.** A verifier that reads the shared tree while another agent is mutating it
is not running the experiment it thinks it is running.

---

## 7. What generalises

Round 1 left three rules on top of MR-002's four. This round adds two, and both
come from the same place: the pass applied its own discipline to the code and not
to the sentences about the code.

8. **A recorded mutation is a claim, and it is checkable.** §6's table is the
   pass's own evidence, and one cell of fifteen names a mutation that turns a
   different item's test red (§4.6), while another summarises "collapsing the two
   error forms" when only one of the two collapses is caught (§4.3). Both were
   found in minutes by running the cell. The rule: **after writing the mutation
   into the record, run it from the record**, not from memory of having run it.
   Item 12 shipped zero production code, which makes the failure structural — an
   item with no production change of its own cannot be falsified by a mutation
   inside its own diff, and needs one named elsewhere.

9. **A change that widens a gate has to name what the gate was holding back.**
   Item 7's own commit added the rule "a decision that adds a migration has to
   name every place that reads 'pending' as 'absent'". It named two and missed the
   third, which was the one that change created (§4.1). Generalised: **when a
   predicate is relaxed, the question is not "is the new predicate right for the
   case I am fixing" but "what now runs that did not run before".** Six commands
   now run against a schema they cannot read, and no test drives a coordination
   command against a schema-behind database even though the fixture that builds
   one lives two files away.

And one that is a re-statement rather than a new rule, because the round produced
it again from a different direction: **documentation drift inflates under parallel
audit, and the correction is precedent, not judgment.** Six of this round's
thirteen LOWs are one wrong sentence each. Two verifiers independently argued
their finding down to LOW by citing round 1's own consolidated ruling on the same
defect class rather than by re-reasoning it. That is the mechanism working; it
only works because round 1 wrote the ruling down.

---

## 8. Remediation brief for round 2

MR-004 is **not blocked**, but four of these should land before anything builds on
the contract — item 1 because it is the only defect a user reaches, and item 4
because MR-004 inherits the sentence.

Nine work items. Items 1-4 are the MEDIUMs; 5-7 are guards that cannot fail; 8-9
are the ledger. As in round 1, each names what the fix must not break and the
mutation that proves a test holds it.

---

**1. Give the coordination commands a schema test of their own — §4.1.**
`internal/cli/coordination.go:120`. Before `coordinationScope` hands the store to
the six commands, establish that the coordination tables exist — or route a
schema-behind database to the same answer `status` gives it,
`MIGRATION_FAILED` / "Schema is behind this binary" with `next_action:
["mindrail init"]`. Do not narrow item 7's gate back: the workspace lookup must
still succeed on a schema-1 database, which is the whole of F01.
*Must not break:* `TestAWorktreeRegisteredByAnOlderBinaryIsStillRegistered`; the
never-initialised repository must still answer `COORDINATION_UNAVAILABLE` at exit
1 (AC-06.7 as amended); a healthy repository must still exit 0 on all six.
*Mutation that proves it:* build `downgradeToSchemaOne` and drive `task list
--json` and `session open --json` through it — the printed `next_action` must be
one that clears the condition, and the raw `no such table: tasks` must not be the
only cause the caller gets. Removing the new check must turn that row red on the
doctor remedy.

**2. Make item 13's guard reach a read path — §4.2.** Either add the `emit`
backstop the brief originally asked for, beside `refuseUnrepresentableJSON` in
`internal/cli/output.go`, or extend the sweep: add semantic-damage rows (a
`tasks` row with an unparseable `created_at`, the shape
`internal/cli/coordination_read_test.go` already builds for `checkpoints`) to the
matrix, and drive it through `task show` as well as `task list`.
*Must not break:* `TestNoCoordinationFailureReachesTheWireUncoded`'s existing
rows, and `TestADamagedRowIsReportedWithACode`.
*Mutation that proves it:* replace `readFailed("the task", id, err)` at
`store.go:309` with `err`, and the `ListTasks` wrap at `:338` with `scanErr` —
the suite must go red. Today it is green in all 18 packages while the binary
publishes `{"code":"","impact":"","next_action":[]}`.
*Then correct §6 row 13's mutation cell*, which currently records the singular
"reverting one read wrap" for ten wraps.

**3. Pin both directions of item 3's two-form split — §4.3.**
`internal/cli/representable.go:74`. Assert the location form's message, not only
its code: read the second `next_action` out of the payload in
`TestTheUnrepresentablePathRemediesBothWork` and carry it out, rather than
constructing the rename in the test body.
*Must not break:* `TestAStoredValueIsNotReportedAsAPath`, which pins the other
direction and is correct.
*Mutation that proves it:* set both entries of `locationTypes` to `false` — the
suite must go red. Today `go test ./...` and `make verify` are both green.

**4. Amend AC-04.4 and design §4 to the ordering item 4 shipped — §4.4.**
`mr-003-requirements.md:340` and `mr-003-design.md:108-110`, in the same
`*Amended during …*` form the pass used seven times elsewhere, citing F37 and
D-59's existing amendment. Rename
`TestTheLastCheckpointIsTheNewestIdNotTheNewestTimestamp` and correct its comment,
which currently says "is AC-04.4 and decision D-59" for a test that passes under
either rule.
*Must not break:* nothing executable.
*The check that proves it:* `grep -n "checkpoint_id" mr-003-requirements.md
mr-003-design.md` returns only amendment notes and the schema DDL, and
implementing AC-04.4 literally (`ORDER BY checkpoint_id DESC`) turns a test whose
name cites AC-04.4 red. Today it stays green.

**5. Move the root escape above the name check — §4.5.**
`internal/moduletree/moduletree.go:66-70`. One line. Then give
`internal/coordination/lifecycle_scan_test.go` and `internal/storage/arch_test.go`
the vacuity guard the other two walkers already have ("no first-party packages
were parsed").
*Must not break:* the four walkers' existing behaviour in an ordinarily-named
checkout.
*Mutation that proves it:* copy the tree to a directory whose name starts with a
dot, plant a second statement of the seven states in `internal/cli` and a
`modernc.org/sqlite` import in `internal/doctor` — both guards must fire. Today
both are `ok`. Also extend `moduletree_test.go:57`, whose case is named "the root
itself is never skipped" and only ever runs against a numeric `t.TempDir()`.

**6. Record a mutation that actually falsifies the AC-09.1 matrix — §4.6.**
`mr-003-findings.md:280` and §3's mutation ledger, which was never extended for
item 12.
*The mutation to record:* replace `return renderSection(w, "Next",
payload.NextAction...)` at `internal/app/errors.go:199` with `return nil` — all
four rows of `TestTheFourCoordinationRefusalsAgreeAcrossBothRenderings` go red,
and the halted-startup test stays green. Verify the pairing in both directions
before writing it down.

**7. Cover the guards item 5 left unpinned — §4.8, and §5's item-5 refutation.**
Two arms in `internal/coordination`: `MintFor("")` must produce
`COORDINATION_UNAVAILABLE` and not a foreign-key error wrapped as
`COORDINATION_WRITE_FAILED` (deleting `store.go:148-150` must go red — today the
whole suite is green and `noWorkspace`'s body has coverage count 0). And one in
`internal/cli`: assert the boundary refusal's envelope, not only its code —
`metadata.flag`, the `… --help` remedy, and the impact sentence — so that deleting
`requireFlagText` from `newTaskOpenCommand` and `newCheckpointWriteCommand` cannot
pass. That deletion is currently invisible to all 19 packages.

**8. The ledger — §4.10, §4.11, §4.14, §4.15, §4.16, §4.9, §4.13.** One pass over
three documents and three comments:

| Edit | File |
|---|---|
| `TestHumanOutputGolden` → `TestCoordinationHumanOutputGolden` | `mr-003-findings.md:148` |
| "1.2 ms at 20,000 tasks" → the measured figure at that size (round 1's table says 28.98 ms; re-measured 7-24 ms for the isolated query) | `mr-003-findings.md:319`, `internal/coordination/store.go:30`, `internal/coordination/query_plan_internal_test.go:21` |
| Drop "(test only)" from row 11 and attribute the `mint` flag and the `MintFor("")` error class | `mr-003-findings.md:279` |
| Correct "All six carry the id in `why`"; there are eight constructors and three carry it only in `metadata` | `mr-003-requirements.md:357` |
| "on beş commit" → thirteen, and note that items 8-10 share one | `mindrail-0.1-task-list.md:180` |
| Make the F37 fixture ids 26 Crockford characters sharing a millisecond prefix, or correct the comment that says they are what `identity.NewID` mints | `internal/coordination/checkpoint_order_test.go:45` |
| Record the `EXISTS` rewrite's empty-project case beside the speedup | `internal/coordination/store.go:24-48`, `mr-003-findings.md:319` |
| Add `migrations/README.md` naming the corrections that cannot live in the applied `.sql` | `migrations/` |

*Must not break:* nothing executable. *The check that proves it:* `grep -rn "1.2
ms" internal/ docs/` returns nothing; `grep -c 'TestHumanOutputGolden'
mr-003-findings.md` returns 1, on the sentence about the pre-fix state.

**9. Two decisions to take rather than fixes to apply.**

- **The attribution error class (§4.7).** A read failure inside a write
  transaction currently publishes `COORDINATION_WRITE_FAILED` with a `subject_id`
  naming a row nothing tried to insert. Either wrap `requireSession`'s scan arm in
  `readFailed`, or accept the write code and give it the id of the thing that
  actually failed. *Mutation:* damage `sessions.started_at` and run `task open
  --title t --session SES-… --json` — whichever code is chosen, `metadata` must
  name the session, not a phantom task.
- **The session clock (§4.12).** Pass the already-captured `now` into
  `(*Store).attribute` rather than reading the clock a second time inside the
  transaction. It satisfies brief item 5 verbatim and removes the inversion at
  zero cost. *Check:* `select count(*) from tasks t join sessions s on
  s.session_id = t.opened_by where s.started_at > t.created_at` returns 0.

---

**Carried forward, not a round-2 defect.**
`internal/migration/concurrent_test.go:155` asserts `appliers == racerRounds`,
which stopped being a valid invariant when `migrations/000002_coordination.sql`
was added in `f7ac67f` — `Migrator.Up` applies each migration in its own
`BEGIN IMMEDIATE`, so two processes can each apply a disjoint subset and both
report `len(Applied) > 0`. It fails roughly one run in ten on `make check`, with
the ledger correct every time. The fix is one line of test logic: count distinct
applied versions per round, or assert `sum(len(Applied)) == racerRounds *
len(set)`, rather than counting processes that applied anything. It belongs on the
MR-003 implementation backlog beside `internal/migration`'s other deferrals, and
it should carry the corrected reproduction rate, because the first report of it
said "reproduced once in two full-suite runs" and that understates it enough to
get it dismissed.
