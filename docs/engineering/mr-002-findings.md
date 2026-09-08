# MR-002 — findings

Three audit rounds over the knowledge validation pipeline, the two remediations
between them, and what each remediation cost. The session's audit budget was two
rounds; the user extended it by one after round 2 confirmed a regression the fix
pass had introduced.

- **Round 1** graded `8e5c2e5` (the implementation) against the requirements frozen
  at `c2e81d1`. 28 proposed, **22 confirmed**, 6 refuted, 0 unsettled.
- **Remediation 1** landed as `3bd453c`.
- **Round 2** graded it. 13 proposed, **11 confirmed**, 2 refuted, 0 unsettled.
  **All eleven were introduced by the remediation.**
- **Remediation 2** landed as `8832bfd`.
- **Round 3** graded it. 17 proposed, **12 confirmed**, 5 refuted, 0 unsettled.
  Nine were new; three pre-dated it. Appendix D.

**Status: `NOT_VERIFIED`.** One HIGH remains open — the same seam, flipped a third
time — and it is what decision **D-52** was written to end. The twelve round-3
findings are unfixed; the next session fixes them under D-52.

---

## 1. The number that matters

MR-001 closed 17 findings and produced 45 more, seven of which were defects the
fixes themselves introduced. MR-002 closed 22 and produced 11 — **and this time
every single one came from the fix pass.** Not one was a defect round 1 had missed.

That is the same lesson at a higher resolution. A remediation pass is not a
correction applied to a defect; it is new code written by someone who has just read
a finding and believed it, under exactly the conditions that produce the next
finding. The rule MR-001 wrote — *a remediation pass needs its own audit* — is not
prudence. It is the measured base rate of this repository.

---

## 2. Round 1: what was actually wrong

Two defects mattered; the other twenty were untested guards, unbounded output, and
documentation drift.

### The fail-open — one unknown JSON property disabled the milestone's only fatal check

`validate.go:216` built the cross-record graph from `judged`, the step-5 survivors,
instead of from the records the loader read. A schema-invalid member therefore
withdrew its edges, the cycle stopped closing, and step 9 fell silent.

| | Control | Scenario |
|---|---|---|
| Difference | — | `"note":"junk"` added to `DEC-0001.json` |
| `supersedes` arrays | untouched | **untouched** |
| `status` exit | 1 | **0** |
| Readiness | BLOCKED | **DEGRADED** |
| Code | `KNOWLEDGE_SUPERSEDE_CYCLE` | `KNOWLEDGE_INVALID` |
| Step-9 findings | 2, `Fatal: true` | **0, none fatal** |

This defeated the task list's fourth acceptance criterion outright. Two auditors
found it independently from opposite directions — the Reader from the requirement,
the Breaker from an upgrade-path corpus.

### The fabricated claim — step 8 denied a file the same report was correcting

`stepSupersedeTarget` resolved through the `judged` graph but suppressed on
`store.Problems`. The two sets disagree exactly on step-5 failures, so one
`next_action` array carried both of these at once:

> Correct `.mindrail/knowledge/decisions/DEC-0001.json` so it satisfies the knowledge
> schema document for its kind.
>
> Add the record `.mindrail/knowledge/decisions/DEC-0002.json` supersedes, or drop
> that id from its supersedes list.

The report told the reader to add a file the line above told them to correct.

### The measured costs

| | Before | After |
|---|---|---|
| Message text, n=2000 chain | 224,352,000 bytes | 1,518,000 bytes |
| `doctor --json`, n=1000 duplicate id | 45,386,339 bytes | ~10,177 bytes |
| 10,000-record cycle | 10.0 s, 16.1 GB peak RSS, 2.4 GB of JSON | 281 ms, 58.8 MB, 14,538 bytes |

Steps 7 and 10 built their message text inside the per-subject loop, so the text was
quadratic in the number of records. `describeFindings` was uncapped where
`findingRemedies` capped at ten.

### A containment gap MR-002 made load-bearing

A record file that is a **symlink** out of the repository was read and validated as
repository content. A symlinked *directory* was refused with `PATH_ESCAPES_ROOT`; a
symlinked *file* was not. The read predates MR-002; MR-002 is the first milestone to
derive verdicts from those bytes, which is what turned it into a finding.

---

## 3. Round 2: what closing them cost

Every headline fix is real and guarded. Fourteen surgical reversions of the fixes
were performed in a scratch tree and **all fourteen turned the suite red**, each
naming a test written for that finding. No false "closed" claim of the MR-001 kind
survives. `AC-12.4` holds mechanically: between `8e5c2e5` and `3bd453c` the set of
`app.Code` string values is byte-identical, every doctor check name literal is
identical, and the multiset of every `json:"..."` tag in every non-test file is
identical.

And the fix pass produced eleven new defects.

### 🔴 The one that matters — a stray draft file makes a healthy repository BLOCKED

Found independently by both Breakers (`BA-A1`, `B-A1`), confirmed by refutation.

The correct fix — build the graph over every record the loader read — also removed
an invariant nobody noticed was doing work. `graph.byID` is keyed by the `id` string
alone and unions the `supersedes` of every subject under that key. Before the change,
only step-5 survivors were in the graph, and a survivor's id is forced by its schema
`pattern` to be well-formed — so node identity was *implicitly verified*. The
remediation removed that guarantee without replacing it.

| | Control | Scenario |
|---|---|---|
| Store | `DEC-0001` (superseded, no `supersedes`), `DEC-0002` (active, supersedes `DEC-0001`) | the same two files **plus one draft** `DEC-0003.json` carrying `id: "DEC-0001"` and a typo'd property `"titel"` |
| `79716a9` | exit 0, knowledge OK | exit 0, knowledge OK |
| `8e5c2e5` | exit 0, READY | exit 0, DEGRADED, 1 finding — **naming `DEC-0003.json`** |
| `3bd453c` | exit 0, READY | **exit 1, ERROR, BLOCKED, `KNOWLEDGE_SUPERSEDE_CYCLE`** |

The two findings attach to `DEC-0001.json` and `DEC-0002.json` — both individually
valid, neither on a cycle. The message asserts *"Each of them is reachable from every
other by following supersedes"*, which is false: `DEC-0001.json` has no `supersedes`
property at all. The remedy reads "Break the supersede cycle by dropping the
superseded id from `.mindrail/knowledge/decisions/DEC-0001.json`" — **an instruction
that cannot be carried out on that file.** And `DEC-0003.json`, the file that created
the edge, is named nowhere in the output.

This breaks `AC-03.6` — "every finding costs the repository the records it names and
no more" — in the one place the milestone made fatal. `newGraph`'s own comment names
the hazard ("indexing every such record under the empty string would fuse them into
one node whose edges are the union of all of theirs, joining lineages that have
nothing to do with each other") and then guards only the empty-string case.

It is MR-001 rule 4 again, verbatim: **a widened classification over-fires before it
under-fires.** The proposed remedy: key `byID` by `(Kind, ID)` so the `DEC-`/`INV-`
namespaces cannot fuse, and when a subject with `schemaValid == false` carries an id
already held by a valid one, index it for resolution but do **not** union its
`supersedes` into that node's edges — an id the schema has just refused to vouch for
is precisely the unverified edge D-45 says closes nothing.

### 🟠 The containment fix cost a third of the status budget

Closing the symlink gap put every record file through `Root.Resolve`, which
canonicalizes every path component — ~3.9 µs per record, 45% of the loader's
per-record cost at scale.

| n (valid records) | `8e5c2e5` | `3bd453c` | ratio |
|---|---|---|---|
| 1,000 | 34 ms | 38 ms | 1.12 |
| 2,000 | 52.8 ms | 67.7 ms | 1.28 |
| 5,000 | 102.0 ms | 136.2 ms | 1.33 |
| 10,000 | 157.2 ms | 208.6 ms | 1.33 |

`loader.Load` alone went 48.3 → 87.1 ms. The 150 ms warm-path SLO is now crossed at
~5,700 records instead of ~8,700. Three independent lines of measurement agreed, on
interleaved runs. The fix is to `Lstat` the entry rather than canonicalize the whole
path, keeping the refusal at roughly the old cost.

### The remaining nine

Two `LOW`s about the new `created_at` pattern refusing offset spellings that both
readers load correctly; a machine-local absolute path reaching a printed
`Problem.Message` on the very path the containment fix added; a `PATH_ESCAPES_ROOT`
distinction the loader deliberately drew that no consumer can observe; a truncation
count that counts only the finding class the precedence ladder selected; a genuine
dangling-supersede diagnosis dropped by the widened step-8 suppression; and the new
failure condition owing a row in the agreement matrix that `AC-09.2` governs. Full
table in Appendix C.

Two round-2 proposals were **refuted** with produced evidence, both about the
`created_at` pattern — a reminder that the refutation round earns its keep in both
directions.

---

## 4. Status

**`NOT_VERIFIED`.** The consensus gate is not met: one HIGH finding, confirmed by two
independent auditors and surviving refutation, remains open, along with one MEDIUM
performance regression and nine LOW findings.

The audit budget of two rounds is spent. What MR-002 has:

- `make verify` green, `make tidy-check` green, 674 top-level test functions against
  a baseline of 507.
- Both round-1 headline defects closed and guarded, verified by reversion.
- Steps 5–11 implemented, wired into `status` and `doctor`, with the pipeline pure
  and the schema documents as the contract.

What it does not have: a clean audit. The remaining findings are documented above
with reproductions; none requires a further audit round to be believed, and all of
them require one to be *fixed safely* — which is the whole content of §1.

---

## 5. What generalises to MR-003

1. **The fix pass is where the defects come from now.** MR-001: 7 of 45. MR-002:
   11 of 11. Budget for it explicitly — a remediation is a change, and a change is
   audited.
2. **A fix that removes a constraint must ask what the constraint was doing.**
   Restricting the graph to step-5 survivors was wrong *as a filter on reporting* and
   right *as a guarantee about node identity*. Removing it fixed the first and lost
   the second, and no test noticed, because no test named the second.
3. **A remedy that cannot be carried out is a defect, not a wording problem.** Both
   milestones produced one. MR-001's matrix already asserts that carrying a remedy
   out clears the condition; it does not yet assert that the remedy is *applicable to
   the file it names*.
4. **Measure the healthy path after every fix.** The containment fix was correct,
   guarded, and cost a third of the status budget. Nothing in the suite could have
   said so.
## Appendix A — round 1 findings, as confirmed

Twenty-eight proposed, twenty-two confirmed, six refuted, none unsettled.

| ID | Severity | Where | What |
|---|---|---|---|
| F-R1 | HIGH | `internal/knowledge/validate/validate.go:216 (`lineages := newGraph(judged)`) fee` | A supersede cycle stops being reported — and stops being fatal — when any member of the cycle also fails step 5 |
| B-01 | HIGH | `internal/knowledge/validate/validate.go:188 (step-5 gate drops the record from `` | One unknown JSON property on any member of a supersede cycle turns the milestone's only fatal check off: exit 1/BLOCKED becomes exit 0/DEGRADED |
| B-02 | HIGH | `internal/knowledge/validate/steps.go:152-177 (`stepSupersedeTarget`); the suppre` | Step 8 reports "this store has no such record: <path> is not there" one line above a step-5 finding naming that same file |
| F-R2 | MEDIUM | `internal/knowledge/validate/steps.go:152-176 (`stepSupersedeTarget`), specifical` | Step 8 claims a supersede target "is not there" for a file that is in the store, contradicting a step-5 finding in the same report |
| BA-05 | MEDIUM | `internal/knowledge/validate/lineage.go:43 (slices.Sort(g.ids))` | The sort that makes the cycle walk deterministic has no test: deleting it makes the finding count vary run to run |
| BA-10 | MEDIUM | `internal/knowledge/loader/loader.go:237 (os.ReadFile(filepath.Join(absDir, name)` | A knowledge record file that is a symlink outside the repository root is read and validated as repository content |
| B-03 | MEDIUM | `internal/knowledge/validate/steps.go:135-137 (`strings.Join(others, ", ")` insid` | Steps 7 and 10 build O(n²) message text: `mindrail status` goes from 75 ms to 11.7 s and 17.3 GB peak RSS on repository content alone |
| F-R4 | LOW | `docs/engineering/mr-002-design.md:135 and docs/engineering/mr-002-requirements.m` | DOC_DRIFT: design §3 and AC-02.1 still publish a Validator.Validate signature the binary does not have, and no document was updated by the implementation commit |
| F-R5 | LOW | `internal/knowledge/loader/loader.go:6-11 versus internal/knowledge/validate/vali` | DOC_DRIFT: loader.go's package doc says steps 5-14 belong to MR-002, contradicting validate.go's package doc and AC-12.2 |
| F-R7 | LOW | `internal/knowledge/validate/validate.go:270-277 (`sorted`) and internal/knowledg` | AC-11.4 gap: two of D-47's four sort keys and step 8's file-naming clause can be deleted with the whole suite green |
| F-R9 | LOW | `internal/knowledge/record/new.go:118 and :131-133` | NewInvariant's frozen third parameter can never carry a value: the frozen contract contradicts itself (AC-01.1 versus AC-01.2) |
| F-R10 | LOW | `docs/engineering/mr-002-audit-package.md:107` | The audit package's own package count is wrong |
| BA-01 | LOW | `internal/knowledge/validate/lineage.go:114 (cycle := rotateToSmallest(slices.Clo` | Records genuinely inside a supersede cycle receive no finding — step 9 reports the DFS tree path, not the cycle |
| BA-03 | LOW | `internal/knowledge/loader/loader.go:80 (Body []byte `json:"-"`) and loader.go:28` | D-46's retained record bodies turn peak memory from O(largest record) into O(whole knowledge store) |
| BA-04 | LOW | `internal/doctor/checks.go:723 (if len(invalid) > 0 { ... return }) preceding che` | An unreadable record is named nowhere once any finding exists; corrupting one cycle member turns exit 1 into exit 0 and hides the file |
| BA-06 | LOW | `internal/knowledge/validate/steps.go:88 (step 6), :136 (step 7), :172 (step 8), ` | Four of the seven step messages can be stripped of the facts that make them actionable and the suite stays green |
| BA-07 | LOW | `internal/knowledge/validate/validate.go:275 (cmp.Compare(a.finding.Message, b.fi` | D-47's Message tie-break key in validate.sorted has no test, and a one-record store reaches it |
| BA-08 | LOW | `internal/knowledge/validate/validate.go:166-168 (if v == nil { panic(...) }) and` | Check's nil-validator guard has no test that can fail — the delivered test is satisfied by a nil-pointer dereference |
| BA-09 | LOW | `internal/knowledge/schema/validator.go:197 (this binary reads X but ships no Y),` | Ten guards added by MR-002 turn nothing red and change nothing observable — dead checks a future reader will trust |
| B-04 | LOW | `internal/doctor/checks.go:1016-1023 (`describeFindings` joins every finding with` | `describeFindings` is uncapped where `findingRemedies` caps at 10, so `doctor --json` emits 45 MB for a 1000-record store with one duplicated id |
| B-05 | LOW | `schemas/knowledge/decision.v1.schema.json:37-41 and invariant.v1.schema.json:38-` | Step 5 pronounces valid three `created_at` spellings that `record.Decision` cannot decode, and accepts non-UTC offsets that AC-01.3 and tech-stack §42 forbid |
| B-08 | LOW | `internal/knowledge/record/scope.go:112-118 — `if len(target) < 2 || target[1] !=` | `isWindowsDriveRooted` over-fires on a legal POSIX repository path whose first segment is a single letter followed by a colon |

### Refuted in round 1, with produced evidence

- **F-R3** — An unreadable knowledge record becomes unactionable — named in no next_action on any command — as soon as any unrelated finding exists
- **F-R6** — AC-05.2 is not met for doctor.kindForCode: neither new code has an entry, and the default it falls through to contradicts the exit class the CLI table declares
- **F-R8** — NewValidator refuses any registry document without an $id, contradicting the registry's documented promise that extra documents are kept — and in bootstrap that refusal is a panic
- **BA-02** — Carrying out a printed next_action verbatim leaves the repository BLOCKED with a new cycle
- **B-06** — Step 11's repo-relative check tests path structure, not content: one leading space makes an absolute path pass, and " ", tab, newline and NUL all pass as targets
- **B-07** — Upgrade alone moves a previously-READY repository to DEGRADED for ~30 record shapes and to BLOCKED for one, with no migration, grace window or opt-out

## Appendix B — what the remediation did


### FIX-A `internal/knowledge/validate` — reported `GREEN`

| Finding | Reproduced | Before | After | Over-fire guard |
|---|---|---|---|---|
| F-R1 / B-01 | YES | Byte-identical except `"note":"junk"` added to DEC-0001: Check -> 2 findings, step9=0, fatal=0 (step 5 on DEC-0001, step 8 on DEC-0002). `status` exit | Check -> 2 findings, step9=1, fatal=1: step 5 on DEC-0001, and a fatal step-9 KNOWLEDGE_SUPERSEDE_CYCLE on DEC-0002. End-to-end `status` exit 1, `doct | TestASchemaInvalidRecordDoesNotManufactureAFinding, 5 rows, each a healthy repository containing one schema-invalid record; measured result after the  |
| F-R2 / B-02 | YES | DEC-0001 present and read but carrying `"title": ""` (step-5 failure), DEC-0002 supersedes it. Check returned 2 findings: a step-5 finding naming DEC- | Check returns 1 finding: the step-5 finding on DEC-0001 only. Measured end-to-end on the same throwaway repository with the fixed binary, the doctor r | TestStepEightStillReportsATargetNothingInThisRunNamed: (a) a supersede target that no record, no Problem and no file in the run names still produces e |
| BA-01 | YES | 5 decisions all in one strongly connected component (DEC-0001 supersedes [DEC-0002, DEC-0004]; 0002->0003; 0003->0001; 0004->0005; 0005->0002) -> only | All 5 records get a fatal step-9 finding, and each message names all five members. | TestARecordOutsideAClosedGroupIsNotDraggedIntoIt: DEC-0001 points INTO the group and DEC-0009 is pointed at BY it; neither is on a closed walk. Measur |
| BA-05 | YES | TestCheckIsDeterministicAcrossRepeatedCalls compares runs only against each other, so it is satisfied by any order stable within one process and never | TestTheVerdictDoesNotDependOnTheOrderTheRecordsWereRead builds a 9-record store containing two closed groups, a fork, a dangling target and a straight | The test asserts equality against a fixed baseline rather than merely self-consistency, so an implementation that returned nothing at all would fail t |
| BA-06 | PARTIALLY | AC-06.4 requires remedies to name the offending path; status.componentFrom drops Diagnostic and Impact, so a fact absent from the message is a fact th | Step 6's message must name both the id it found and the id the file name claims; step 8's must name the unresolved id and the file it would occupy; st | The table asserts substring presence only, so it cannot force a particular wording and does not freeze prose. The step-11 row reuses the existing AC-0 |
| BA-07 / F-R7 | YES | The instance-location key is carried beside the finding and dropped before Check returns, so from outside the package no case could be constructed for | TestEveryOneOfTheFourSortKeysDecidesAnOrder builds four pairs, each differing in exactly one key and fed in the wrong order, with every lower-priority | TestSortedIsStableWhenEveryKeyAgrees: two findings agreeing on all four keys must keep production order. This is the guard against a fix that reached  |
| BA-08 | YES | With an EMPTY store the step-5 loop never runs, so a Check that had lost its guard would not panic at all -- it would return an empty finding list and | TestCheckRefusesANilValidatorEvenWithNothingToValidate calls Check(emptyStore, nil) and requires a panic. This is the one case where deleting the guar | The assertion is that the panic value is a string mentioning 'validator', not an exact string match, so rewording the message does not break the test  |
| B-03 | YES | Same 1000 records, each with supersedes:[DEC-<k-1>], all active. Message bytes -- n=100: 577,500; n=1000: 56,176,000; n=2000: 224,352,000 (exactly qua | Message bytes -- n=100: 75,600; n=1000: 758,000; n=2000: 1,518,000 (ratio 2.00 per doubling, linear). Allocated -- n=100: 0.7 MiB; n=1000: 6.8 MiB; n= | TestALargeGroupNamesTenRecordsAndCountsTheRest with a 40-record lineage: measured after the fix, the message names exactly 10 records, carries 'and th |

Not fixed, with the reason given:

- BA-06's step-7 claim (steps.go:136) is NOT real and I did not 'fix' it. Mutating step 7's message to drop the id and the sibling file names is KILLED by the pre-existing TestStepSevenNamesBothOffendingFiles. The other three claims in BA-06 (steps 6, 8 and 10) did reproduce and are fixed. I added a step-7 row to the new message-facts table anyway, for symmetry and so the coverage guard has no hole,
- BA-05's proposed remedy -- a test that goes red when `slices.Sort(g.ids)` is deleted -- is not achievable against the corrected code, and I did not fake one. The finding was real against the delivered depth-first walk (I reproduced the surviving mutant on HEAD). The BA-01 Tarjan rewrite removes the order-dependence structurally, so after the fix that line's deletion is genuinely unobservable in th

### FIX-B `internal/doctor` — reported `GREEN_WITH_DEVIATIONS`

| Finding | Reproduced | Before | After | Over-fire guard |
|---|---|---|---|---|
| B-04 | YES | every record carrying id "DEC-0001", `doctor --json`: 11,675 / 491,726 / 45,386,439 bytes at n = 10 / 100 / 1000. Decomposed at n=1000: 1,999 diagnost | duplicate-id arm: 7,827 / 10,160 / 10,177. Short-message arm: 6,009 / 6,163 / 6,175. describeProblems degraded: 4,755 / 4,926 / 4,937. describeProblem | Constructed the adjacent healthy and adjacent-small-but-broken cases and required them byte-identical, not merely "small". Healthy 1000-record store:  |
| BA-04 | YES | Repo with DEC-0002.json schema-invalid AND DEC-0009.json unparseable. `doctor --json`: state DEGRADED, code KNOWLEDGE_INVALID, diagnostic names only D | Invalid branch: state DEGRADED and code KNOWLEDGE_INVALID unchanged, diagnostic now ends "The loader could not read 1 further record:\n.mindrail/knowl | A remedy telling the reader to fix a perfectly readable file is worse than the silence it replaces, so the adjacent case was constructed for each of t |

Not fixed, with the reason given:

- doctor's knowledge `details` map still carries no `problems` key, so `doctor --json` publishes no COUNT of unreadable records (it now publishes their names in the diagnostic and next_action, and `status --json` publishes the count). Adding the key would change internal/doctor/testdata/doctor_report_{human,json}.golden AND internal/cli/testdata/doctor_human.golden, and internal/cli is outside the d
- The O(n^2) message CONSTRUCTION in internal/knowledge/validate/steps.go is not mine and was not touched; my byte cap bounds its effect on the report but not the cost of building it. At n=1000 the 45 KB duplicate-id message is still constructed and then cut.

### FIX-C `internal/knowledge/loader` — reported `GREEN_WITH_DEVIATIONS`

| Finding | Reproduced | Before | After | Over-fire guard |
|---|---|---|---|---|
| BA-10 | YES | SCENARIO (record file `.mindrail/knowledge/decisions/DEC-0002.json` -> /tmp/.../outside/evil.json, absolute target): exit=0, ok=true, knowledge DEGRAD | All three: exit=0, ok=true, knowledge DEGRADED, decisions=0, problems=1, findings=0, readiness=DEGRADED. doctor's diagnostic reads '.mindrail/knowledg | Full CLI matrix, 14 repositories, measured with the HEAD binary and the fixed binary. Every healthy arm is byte-identical before and after — exit=0, k |

Not fixed, with the reason given:

- Nothing in FIX-C was left unfixed. Two test failures exist in the tree but are outside my scope and provably not mine: internal/knowledge/validate/messages_test.go:185 (TestTwoFindingsOnOneRecordAndOneStepAreOrderedByTheirText) and internal/doctor/knowledge_bounds_test.go (a red assertion, and at one sampling a raw syntax error at line 257). Both files are untracked and being written right now by 

### FIX-D `internal/knowledge/record` + `schemas/` — reported `GREEN`

| Finding | Reproduced | Before | After | Over-fire guard |
|---|---|---|---|---|
| B-05 | YES | Four spellings, one character changed each, all measured against BOTH readers. schema=ACCEPT / record.Decision=REJECT: "2026-01-01T00:00:00z" (Go: can | All six now schema=REJECT, and end-to-end all six produce DEGRADED / KNOWLEDGE_INVALID / findings=1 with a next_action naming the file. The remaining  | This was the main risk and the auditor's literal remedy would have failed it. A pattern anchored on Z$ — the obvious reading of "require UTC" — would  |
| B-08 | YES | {"level":"FILE","target":"a:b/c.go"} — one character inserted into the control — REJECT: "...is an absolute machine path, not a repository-relative on | a:b/c.go -> accepted. End-to-end: READY, findings=0. Verified at all four scope levels that carry a target (PACKAGE, MODULE, FILE, SYMBOL), and throug | The failure mode to avoid was widening until C:/x slips through — the same defect written backwards, which this project has produced twice. Measured a |
| F-R9 | YES | Measured the parameter's domain directly: title in {" ", "A title", "\x00", "0"} all returned ErrInvariantHasNoTitle; only "" succeeded. The domain wa | The parameter does not exist, so the impossible state is unrepresentable. A caller who tries to pass a title now meets the fact at compile time instea | The adjacent risk of dropping a parameter is that the remaining arguments silently shift onto the wrong fields — the signature still compiles while wr |

### FIX-E sweep, docs, re-audit — reported `GREEN_WITH_DEVIATIONS`

| Finding | Reproduced | Before | After | Over-fire guard |
|---|---|---|---|---|
| BA-09 / internal/knowledge/schema/validator.go:197 — "this binary reads X but ships no Y" | YES | Neutralising the guard (`if !ok && false`) left `go test ./internal/knowledge/schema/` GREEN. The three rows only asked `err != nil`, and without the  | Same mutation now fails all three subtests of TestNewValidatorFailsWhenARequiredDocumentIsAbsent. | registryWith(t, DecisionSchemaName, InvariantSchemaName) — the adjacent registry that has everything — still constructs and still holds 2 compiled sch |
| BA-09 / internal/knowledge/schema/validator.go:404 — documentID's "is not a JSON object" | YES | Replacing `object, ok := doc.(map[string]any); if !ok {...}` with `object, _ := doc.(map[string]any)` left the suite GREEN: the nil map made the `obje | Same mutation fails the `document is not an object` and `document is a bare true` rows. | Both shipped documents, plus the four already-refused shapes, still get exactly the diagnosis that belongs to them — no two rows share a `want` fragme |
| BA-09 / internal/knowledge/schema/validator.go:188 — AddResource's error | YES | `_ = compiler.AddResource(id, doc)` left the suite GREEN. Nothing exercised a failing AddResource, so a v2 document that copied v1's $id would have be | Same mutation fails both new tests. | The adjacent healthy case is a v2 document with a correct, distinct $id beside v1 — it must not be refused, because the registry is additive by design |
| BA-09 / internal/knowledge/schema/validator.go:253 — the KeywordNoSchema finding | PARTIALLY | Rewriting the message to a constant while keeping Keyword == KeywordNoSchema left the suite GREEN. The keyword says only "no document governed this" a | The same message rewrite fails all three subtests. | The three refused pairs are an unknown kind and versions on either side of the reader window; the in-window pair (decision, 1) still returns an empty, |
| BA-09 / internal/knowledge/schema/validator.go:166 — the registry read-back | YES | Neutralising the guard left the suite GREEN. No input reached it, and the old message asserted a concurrency race that this immutable type cannot have | A Registry literal whose `names` carries one entry `docs` does not — the only producer of this state, and one this package's own helpers build — now f | The same helper without the extra name constructs cleanly (TestNewValidatorAcceptsARegistryWhoseTwoHalvesAgree), and every pre-existing registry test  |
| BA-09 / internal/knowledge/record/option.go:52 — the nil Option refusal | YES | Deleting the check left the suite GREEN — no test ever passed a nil Option, so a caller writing `NewDecision(id, at, t, d, optionalScope())` where the | Deleting it panics inside TestAConstructorNamesThePositionOfANilOption. The position is asserted, not just the failure: with three options, "one of th | TestOptionsThatAreNotNilAreStillApplied builds a Decision with WithScope + WithTags + WithSupersedes() (an option whose payload is an empty array — th |
| BA-09 / internal/knowledge/record/supersede.go:56 — the uniqueItems refusal in Supersede | YES | Deleting the loop body left the suite GREEN. The input needs a hand-assembled Decision — NewDecision's own checkSupersedes refuses a duplicate first — | All three rows fail, asserting *DuplicateItemError with its Property and Value, and that both returns are zero Decisions so an ignored error cannot wr | Three distinct entries must still be accepted — a long legitimate lineage is the normal shape — as must an array already containing the prior's id onc |
| RE-SWEEP FINDING (new, mine) — FIX-A's step-7 message silently drops the siblings it did not name | YES | steps.go:176 passes `nameList(others, len(paths)-1)`. Replacing the total with `len(others)` left the ENTIRE suite GREEN, while a store of 15 files un | 15 files under one id: every message names its own path plus exactly 10 siblings and says "and the remaining 4". The wrong-total mutation is now red. | The mirror failure is a tail printed when the list is complete — "and the remaining 0" on every ordinary two-file duplicate. TestStepSevenSaysNothingA |
| RE-SWEEP FINDING (new, mine) — FIX-A's sort-stability over-fire guard could not fail | YES | Replacing slices.SortStableFunc with slices.SortFunc left the suite GREEN. TestSortedIsStableWhenEveryKeyAgrees uses two elements with every key equal | 20 findings across two paths, ten per path, every finding within a path agreeing on all four D-47 keys and interleaved on the way in. The same mutatio | The new test is itself the over-fire arm — it asserts the full expected order, so a sort that stopped sorting (rather than stopping being stable) fail |
| F-R4 — the published Validate signature does not compile | YES | Both frozen documents published `Validate(kind loader.RecordKind, ...)`. internal/knowledge/loader imports internal/knowledge/schema (loader.go:26), s | Both documents state the signature the binary has, and D-50 records why it changed, so a later reader can tell a considered change from a drift. | The correction must not quietly loosen the contract: arity, parameter order, parameter meaning and return type are all unchanged, and the two RecordKi |
| F-R5 — the loader's package doc claims steps 5-14 belong to MR-002 | YES | A reader of the loader learned that MR-002 covered all ten remaining steps, which is exactly the "deferred vs forgotten" confusion AC-12.2 exists to p | Every deferred step names its milestone in both files. | The loader's actual scope claim — steps 1-4 only — is unchanged, and every loader test still passes (go test -count=1 ./internal/knowledge/loader/ ok) |
| F-R10 — mr-002-audit-package.md §7 states a wrong package count | YES | The row read "16 packages, up from 16" while its own parenthetical said two packages had been added — the sentence contradicted itself. | "18 packages, up from 16 at 79716a9". | The baseline was verified independently rather than assumed: the 16 comes from listing the Go package directories in commit 79716a9, and the two addit |
| RE-SWEEP OBSERVATION (mine) — five unfalsifiable ordering lines in internal/knowledge/validate/lineage.go | YES | The comments claimed these orderings mattered. Reversing the group order, reversing each lineage's members, dropping the union tie-break, and dropping | Same behaviour, honest comments. A future reader is told which line a test protects and which is a canonical form for debugging. | The one ordering that IS observable had to keep firing: `slices.Sort(component)` in closedGroups is rendered verbatim into every step-9 message. Mutat |

Not fixed, with the reason given:

- THE TENTH BA-09 SITE, AND THE SEVEN LABELLED SURVIVORS, COULD NOT BE LOCATED. The brief names nine file:line sites plus 'survivors the auditor labelled R05 R06 R07 R08 R14 R19 and Q03'. Those labels appear nowhere in the repository — I grepped docs/engineering/mr-002-audit-package.md and its appendix and found no occurrence of any of them. They index the auditor's own mutation catalogue, which is 
- internal/knowledge/validate/lineage.go:140 (rotateToSmallest's `if len(cycle) < 2`) NO LONGER EXISTS. FIX-A's Tarjan rewrite deleted rotateToSmallest entirely, so there was nothing to test or mark. Verified against `git show 8e5c2e5:internal/knowledge/validate/lineage.go`, where line 140 is that guard. Resolved by deletion, not by me.
- internal/knowledge/validate/validate.go:166 (the nil-validator panic) NEEDED NO WORK. FIX-A's TestCheckRefusesANilValidator (which asserts the panic VALUE, not merely that one happened) and TestCheckRefusesANilValidatorEvenWithNothingToValidate already kill it: deleting the guard produces a nil-pointer dereference that the first test explicitly rejects, and a silent clean verdict over an empty sto
- MUTATION A27 SURVIVES AND I DID NOT MANUFACTURE A TEST FOR IT. Raising step 7's sibling-walk cap from 10 to 1000 leaves every published message byte-identical, because nameList re-caps at 10 on the way out — so no output test can ever kill it. It is only visible to the allocator, and TestAllocationGrowsLinearlyWithTheStore measures at n=1000 and n=2000, sizes at which a cap of 1000 is still linear
- MUTATIONS W1, W1c, W2, W3 AND W4 SURVIVE BY DESIGN. Five ordering lines in lineage.go have no observable effect on any published finding: the group order from closedGroups, each lineage's member sort, `slices.Sort(g.ids)` (which FIX-A had already declared), and the edge sort/compact and union tie-break. Every finding they could influence passes through sorted(), which re-orders the whole report. I
- docs/engineering/mindrail-0.1-task-list.md IS UNTOUCHED, AS INSTRUCTED. Its MR-002 section still shows `### [ ] MR-002` with five unchecked acceptance criteria and no Durum block. I read it and changed nothing. Checking them is the orchestrator's call after the final audit.
- `make verify` EXITS 2 AND I DID NOT MAKE IT GREEN. The sole cause is two unformatted files inside .claude/worktrees/wf_af8fcaae-b1c-3/internal/auditb/ — a registered git worktree (visible in `git worktree list`) belonging to another session in this workflow, gitignored at .gitignore:172, outside this Go module. The Makefile's fmt-check runs `gofmt -l .`, which walks into it. I would not gofmt -w a
- F-R4 ASKED FOR D-49 AND GOT D-50, AND THAT IS A DEVIATION I CHOSE. The FIX-D wave landed `created_at`'s spelling rule as D-49 in this same remediation round, before I ran. Renumbering it would have dangled citations in internal/knowledge/schema/validator_test.go, both shipped schema documents' descriptions, and §1 of the requirements document. D-50 opens by saying exactly this, so a reader followi
- doctor's knowledge `details` map still publishes no `problems` count (FIX-B's own deferral). I confirmed it end to end: `doctor --json` over the escaping-record repo carries the file's name in the diagnostic and in next_action but no count, while `status --json` publishes the count. Adding the key touches internal/cli goldens, which is outside this wave's directory. Left as FIX-B left it.
- The O(n^2) message CONSTRUCTION in internal/knowledge/validate/steps.go is still there, as FIX-B noted. My step-7 tests bound and pin what is PUBLISHED; they do not change what is built before the cap applies.

## Appendix C — round 2 findings, as confirmed

Thirteen proposed, eleven confirmed, two refuted, none unsettled. **All eleven were introduced by the remediation.**

| ID | Severity | Introduced by the fix | Where | What |
|---|---|---|---|---|
| BA-A1 | HIGH | YES | `internal/knowledge/validate/lineage.go — newGraph (byID keyed by s.ref` | A schema-rejected record that duplicates a valid record's id injects its supersede edge into that record's graph node, manufacturing a fatal cycle against two correct files and never naming the file responsible |
| B-A1 | HIGH | YES | `internal/knowledge/validate/lineage.go — newGraph (the `if s.ref.ID ==` | A schema-rejected record's id and supersedes are unioned into a valid record's graph node, manufacturing a fatal supersede cycle against innocent files with an inapplicable remedy |
| R-03 | MEDIUM | YES | `internal/cli/agreement_test.go:489 (knowledgeConditions), against inte` | The failure condition BA-10's fix introduced has no row in the agreement matrix AC-09.2 governs |
| B-A3 | MEDIUM | YES | `internal/knowledge/loader/loader.go — readRecord now calls `l.root.Res` | The new per-record containment check makes loader.Load 80% slower and pushes `mindrail status` across the 150 ms SLO at 5,700 records instead of 8,700 |
| R-01 | LOW | YES | `internal/knowledge/loader/loader.go:269-274 (the new `abs, err := l.ro` | BA-10's fix leaks a machine-local absolute path into a printed knowledge Problem.Message |
| R-02 | LOW | YES | `schemas/knowledge/decision.v1.schema.json:41 and schemas/knowledge/inv` | The new created_at pattern refuses spellings that load to exactly the instant the document names, past B-05's split and past D-49's own stated harm |
| R-04 | LOW | YES | `schemas/knowledge/decision.v1.schema.json:41 and invariant.v1.schema.j` | The new pattern's failure message misquotes the rule it is enforcing |
| R-05 | LOW | YES | `internal/knowledge/loader/loader.go:391-408 (escapingRecord and its co` | escapingRecord's deliberately-distinct PATH_ESCAPES_ROOT is invisible to every consumer, and the reading it produces says the opposite |
| B-A2 | LOW | YES | `internal/doctor/checks.go — describeBounded, called from describeFindi` | doctor's new "... and N further findings under .mindrail/knowledge" counts only the one finding class the precedence ladder selected, so the number it publishes about the store is wrong |
| B-A5 | LOW | YES | `internal/knowledge/validate/validate.go — filesTheLoaderNamed now incl` | Widening step 8's suppression to every file the loader named drops a genuine dangling-supersede diagnosis when the expected file exists but carries a different id |
| B-A6 | LOW | YES | `internal/knowledge/loader/loader.go — escapingRecord sets app.CodePath` | The new PATH_ESCAPES_ROOT record problem is unobservable: every command renders it as KNOWLEDGE_UNREADABLE and "Knowledge store has unreadable records", which is the conflation the loader's own comment says it is avoidin |

### Refuted in round 2, with produced evidence

- **BA-A2** — The new created_at pattern refuses non-UTC RFC 3339 offsets that both readers load as exactly the correct instant, narrowing a shipped v1 schema document beyond D-49's own stated criterion
- **B-A4** — The new created_at pattern refuses two spellings that both readers load byte-identically while accepting one that does not, so it does not implement the criterion (AC-01.3) that was written to justify it

---

## Appendix D — round 3 findings, as confirmed

Round 3 graded the SECOND remediation (`8832bfd`). Seventeen proposed,
**twelve confirmed**, five refuted, none unsettled. Nine of the twelve were
introduced by that remediation; three pre-date it.

The base rate finally moved: this was the first fix pass in the repository whose
defects are almost all in published prose rather than in a verdict. It restored
the cost regression (5,000-record `status`: 152 ms at `3bd453c` -> 120 ms, faster
than the original implementation), weakened containment nowhere across 21 layouts
and four binaries, and differed from `3bd453c` on exactly four rows of an 88-shape
upgrade corpus — every one of them a round-2 finding being closed.

| ID | Severity | New in this pass | Where | What |
|---|---|---|---|---|
| BA-B1 | HIGH | YES | `internal/knowledge/validate/lineage.go:164-176 (newGraph: `vouched = v` | A misfiled accepted claimant vouches for a node and deletes the canonical record's supersede edge, turning a real fatal cycle into exit 0 |
| BA-B2 | MEDIUM | NO | `internal/knowledge/validate/lineage.go:160-186 (newGraph unions the su` | The over-fire fix was scoped to schema-invalid duplicates; two schema-VALID records claiming one id still fuse and land a fatal cycle finding with an impossible remedy on an innocent file |
| B-B1 | MEDIUM | YES | `internal/doctor/checks.go, escapingDiagnostic() and escapingImpact(); ` | D-51's new diagnostic asserts "The file is readable" about records that are not readable and hold nothing — the exact inversion of the defect D-51 exists to remove |
| RD-01 | LOW | YES | `internal/doctor/checks.go:1052 alsoHeading, the `default:` arm (lines ` | alsoHeading's mixed arm is a green mutation survivor: the sentence that names both degraded classes under a fatal reading has no guard |
| RD-02 | LOW | YES | `internal/doctor/checks.go:1302 remainingRecordRemedy and internal/doct` | Past the ten-record cap, escaping links are filed under "unreadable records" whenever the tail is mixed, one line below ten remedies in the other class's words |
| RD-04 | LOW | YES | `internal/knowledge/loader/containment_test.go:605-641` | TestLoadReadsTheEntryInsideTheDirectoryItListed builds no symbolic link at all and passes against a loader that has no fast path |
| RD-05 | LOW | YES | `internal/knowledge/loader/loader.go:375` | recordFile's `if filepath.Base(name) == name` guard is unfalsifiable: replacing it with `if true` leaves all 18 packages green |
| BA-B3 | LOW | YES | `internal/knowledge/validate/identity_test.go:418-445, cited by interna` | TestACycleMessageIsTrueOfEveryFileItIsAttachedTo cannot fail for the defect it is named after — it passes verbatim on the pre-fix code and is falsified by a live store on the post-fix binary |
| BA-B4 | LOW | NO | `internal/cli/agreement_test.go:559-660 and :714-747 (knowledgeConditio` | The seam's contract-layer coverage is one-sided: the over-fire arm gained two CLI rows, the fail-open arm gained none |
| B-B2 | LOW | NO | `internal/knowledge/loader/loader.go, recordFile() fast path (os.Lstat(` | The loader's containment check is not TOCTOU-safe, contrary to the explicit guarantee restated in recordFile's new comment — measured: 40 out-of-worktree records ingested by rem2 |
| B-B3 | LOW | YES | `internal/doctor/checks.go, escapingImpact(): the verbs "carries" and "` | escapingImpact publishes a subject-verb-disagreeing sentence for every degraded set of two or more declined records, and no test in the suite reads that field |
| B-B4 | LOW | YES | `internal/doctor/checks.go, recordNouns() and remainingRecordRemedy() —` | A mixed degraded set produces a reading that contradicts itself: its impact counts N escaping records and its omission tail and remedy tail then call those same records unreadable |

### Refuted in round 3, with produced evidence

- **RD-03** — D-51's stated ground for leaving the new remedies classUnknown is false: an exit-1 error object carries the sentence
- **RD-06** — TestACycleMessageIsTrueOfEveryFileItIsAttachedTo cannot exercise the fusion its doc-comment and a production comment both say it holds
- **RD-07** — Step 8's new "was read and carries id X instead" condition is a new published failure condition with no row in the agreement matrix or the broken-setup matrix
- **BA-B5** — Step 8 stays silent about a supersede pointing at a record D-51 says is not repository content, on a warrant D-51 itself denies
- **B-B5** — recordFile's `filepath.Base(name) == name` guard is a branch nothing can falsify — the exact shape the second remediation itself reported against its predecessor

### The HIGH, in full

**BA-B1 — A misfiled accepted claimant vouches for a node and deletes the canonical record's supersede edge, turning a real fatal cycle into exit 0**

- Violates: AC-03.6 / D-39 / D-40 — step 9 is the milestone's only fatal check; the round-1 defect this remediation's predecessor closed was 'one unknown JSON property on one member of a supersede cycle deleted the cycle'. mr-002-findings.md §'The fail-open'.
- Location: `internal/knowledge/validate/lineage.go:164-176 (newGraph: `vouched = vouched || s.schemaValid` at :166 and `if vouched && !s.schemaValid { continue }` at :171)`
- Expected: A store containing decisions/DEC-0001.json (schema-valid, id DEC-0001, supersedes DEC-0002) and decisions/DEC-0002.json (id DEC-0002, supersedes DEC-0001) holds a supersede cycle that a reader can trace file-to-file, each file named for the id it declares. Adding a third file that declares no `supersedes` at all must not change that verdict. Reporting must stay exit 1 / BLOCKED / KNOWLEDGE_SUPERSEDE_CYCLE.
- Actual: The third file — decisions/DEC-0009.json, schema-valid, carrying id "DEC-0002", no `supersedes`, and itself reported by step 6 as misfiled — makes `vouched[{decision,DEC-0002}]` true. DEC-0002.json's edge DEC-0002 -> DEC-0001 is dropped, the cycle disappears, and the run reports exit 0 / DEGRADED / KNOWLEDGE_INVALID with two findings (step 5 on DEC-0002.json, step 6 on DEC-0009.json). The vouching record is the one step 6 has just said does not belong at that path; the record it silences is the one that does.
- Control arm: Two files only — decisions/DEC-0001.json {id DEC-0001, supersedes ["DEC-0002"]}, decisions/DEC-0002.json {id DEC-0002, supersedes ["DEC-0001"], plus an unknown property "bogus"}. `mindrail status --json`: 3bd453c exit 1, readiness BLOCKED, findings 2; 8832bfd exit 1, readiness BLOCKED, findings 2. Both correct.
- Scenario arm: The same two files plus decisions/DEC-0009.json {schema_version 1, kind decision, id "DEC-0002", status "superseded", created_at 2026-01-01T00:00:00Z, title, decision — and NO supersedes property}. `mindrail status --json`: 3bd453c exit 1, readiness BLOCKED, findings 4; 8832bfd **exit 0, readiness DEGRADED, findings 2**, knowledge code KNOWLEDGE_INVALID. Measured twice, on separate freshly-`init`-ed repositories.
- Class sweep: Exhaustive over the 4,864-store space of 2- or 3-file decision stores (file names from {DEC-0001,DEC-0002,DEC-0003}.json, declared ids from {DEC-0001,DEC-0002}, supersedes any subset of {DEC-0001,DEC-0002}, each file schema-valid or carrying an unknown property). Oracle: a cycle traceable file-to-file among files whose name equals their declared id, with at least one schema-valid file on it — 1,169 such stores. 3bd453c misses **0**; 8832bfd misses **4**. All four were then re-run on the real binaries and every one reproduced exactly (impl exit 0, rem1 exit 1 BLOCKED KNOWLEDGE_SUPERSEDE_CYCLE, rem2 exit 0 DEGRADED KNOWLEDGE_INVALID). A random 400-store sample driven through both real binaries showed 38 stores where rem1 reports a cycle and rem2 does not, and 0 in the other direction. The same defect also fires with three claimants including a loader-unreadable one (case 10).


This finding is what **decision D-52** in
[mr-002-requirements.md](mr-002-requirements.md) §1 was written to end. D-52
replaces the case-by-case rules the first two remediations patched in, and it was
reviewed adversarially by a second model before implementation — that review
found the decision as first written governed only who *supplies* graph edges and
was silent about who *receives* a finding, which on this very store reproduces
round 2's defect through a different door. Rule 4 is that review's amendment.
