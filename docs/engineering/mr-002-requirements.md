# MR-002 — Frozen requirements and acceptance criteria

- **Frozen at:** commit `79716a9`, branch `mr-002-knowledge-lifecycle`
- **Contract this refines:** [mr-002-design.md](mr-002-design.md). Where the two
  disagree, the design wins on intent and this document wins on the detail the
  design left open — every such gap is recorded below as a decision D-42…D-48.
- **Task:** [mindrail-0.1-task-list.md](mindrail-0.1-task-list.md) MR-002
- **Tier:** 3 (Program). Not for its size — for its surface. It adds a
  compile-time dependency, a JSON Schema contract evaluated against
  repository-owned content, two new `app.Code` values that enter the wire
  vocabulary, and a reader-compatibility window. The orchestrator's escalation
  rule fires on "public API contract" and "backwards compatibility" regardless
  of line count.

This file is frozen **before** implementation so the audit has something to grade
against that the implementation did not shape.

---

## 0. Baseline evidence

Captured on the tree this work starts from, so a red suite later is unambiguous.

| Measurement | Value | Command |
|---|---|---|
| Commit | `79716a9` | `git rev-parse HEAD` |
| Go toolchain | `go1.27.0 linux/amd64` | `go version` |
| Packages | 16 | `go list ./...` |
| Top-level test functions | 507 | `go test ./... -list '.*'` |
| `make verify` | green — gofmt, `go vet`, all packages `ok`, race detector all `ok`, smoke `ok` in 1.48s | `make verify` |
| `internal/knowledge/` contents | `loader/`, `schema/` only | — |
| `go.mod` direct requires | `go-toml/v2 v2.4.3`, `cobra v1.10.2`, `modernc.org/sqlite v1.58.0` | — |

`make tidy-check` is **not** part of `check` or `verify`. MR-002 adds a
dependency, so it must be run explicitly and reported.

---

## 1. Decisions this document freezes

MR-001 ended at D-35; the design added D-36…D-41. These continue the sequence and
close gaps the design left open. Each one existed as an ambiguity that two
implementers would have resolved differently.

### D-42 — findings reach the report layers through `doctor.Subject`, never by recomputation

`validate.Check` is called exactly once, in `internal/bootstrap`, immediately
after `a.subject.Knowledge = store`, and its result is filed on a new
`Subject.KnowledgeFindings []validate.Finding`.

Not inside the check body: `doctor` checks are pure functions of an
already-resolved `Subject` that "open nothing, create nothing and stat nothing"
(`internal/doctor/subject.go:23-28`), and `schema.NewValidator` returns an error
that a `doctor.Result` has no honest way to express. Not in `status.Build`
either, for the same reason plus D-41.

Findings are **data, not an error**. A store full of invalid records must not set
`KnowledgeErr` — that halts the startup sequence and makes every later block
report `observation: not_observed`, which would be a fabricated absence.

### D-43 — a Finding gets its own codes, because D-38 says it is not a Problem

Two new `app.Code` constants, both registered in the `allCodes` slice and both
given an exit class:

| Code | Meaning | Fatal | doctor state | Exit |
|---|---|---|---|---|
| `KNOWLEDGE_INVALID` | this binary read the record and the record is wrong | no | `StateDegraded` | 0 |
| `KNOWLEDGE_SUPERSEDE_CYCLE` | supersede lineage has no end (D-39) | yes | `StateError` | 1 |

Two codes rather than one code plus a `Fatal` flag: `status` publishes exactly
one `Code` per component, and the two conditions differ in state, exit class and
remedy. Collapsing them would make the component code ambiguous at the one place
a consumer reads it. Reusing `KNOWLEDGE_UNREADABLE` is refused outright — it
already means "this binary could not read it", and D-38 exists to keep the two
apart.

### D-44 — a fatal loader Problem outranks every Finding

When `store.HasFatalProblem()` is true, the knowledge component reports
`KNOWLEDGE_SCHEMA_UNSUPPORTED` and says nothing about findings. A verdict
computed over a store the binary admits it cannot fully read is a claim made from
incomplete data — the exact defect class MR-001's audits kept finding.

### D-45 — cross-record steps reason only over records the loader actually read

Steps 7, 8, 9 and 10 are cross-record: they compare one record against the set.
The set is incomplete whenever the loader recorded a `Problem`, fatal or not.

Rule: **where a `Problem` names the file a referenced id would occupy, the
reference is _unverifiable_, and no finding is emitted for it.** The expected
file for id `X` of kind `k` is `.mindrail/knowledge/<k-dir>/<X>.json` — which is
exactly what step 6 requires, so the mapping is not a guess.

Consequences, each of which owes an over-fire guard:

- **Step 8** must not report "supersede target `DEC-0001` does not exist" when
  `decisions/DEC-0001.json` is in `Problems`. It exists; this binary could not
  read it. One condition, one diagnosis — the loader already named that file.
- **Step 9** must not claim a cycle whose chain passes through an unverifiable
  id. A cycle is a closed walk; an unverified edge does not close anything.
- **Step 10** must not claim a duplicate active lineage when the record that
  would disambiguate it is unreadable.

Step 7 (unique id) is exempt from the suppression: a duplicate is proven by the
two records that _were_ read, and no unread file can make it false.

### D-46 — `RecordRef` carries the bytes step 1 read, and the wire shape does not change

`loader.RecordRef` gains exactly one field:

```go
// Body is the exact bytes step 1 read. It is what steps 5-11 evaluate, and it
// is excluded from JSON so the store's published shape is unchanged.
Body []byte `json:"-"`
```

`validate` decodes `Body` itself. The loader continues to perform steps 1–4 only
and continues to record no `Problem` for anything steps 5–14 own.

### D-47 — findings are sorted before they leave `Check`

`jsonschema/v6` returns `ValidationError.Causes` in Go map-iteration order, which
was empirically observed to differ between five consecutive `Validate` calls on
the same schema and the same instance in one process. tech-stack §2.2 requires a
deterministic core. `Check` therefore sorts its result by
`(Path, Step, InstanceLocation, Message)` before returning, and a test asserts
byte-identical output across repeated runs.

### D-48 — format assertion is switched on explicitly

Under Draft 2020-12 `jsonschema/v6` treats `format` as an annotation unless the
metaschema declares the `format-assertion` vocabulary. The shipped documents do
not. Without an explicit `compiler.AssertFormat()` the `"format": "date-time"`
constraint on `created_at` **enforces nothing** — verified empirically:
`created_at: "yesterday"` produced no error at all until the option was set.

`schema.NewValidator` calls `AssertFormat()`, and a test feeds a record with a
non-timestamp `created_at` and requires a finding. Without that test the schema
would claim a rule it does not have.

---

## 2. Requirements

Every `REQ-*` carries acceptance criteria stated so that a test can fail them.
"Works correctly" is not an acceptance criterion anywhere in this document.

### REQ-01 — `internal/knowledge/record`: the typed records and their lifecycle

New package. Depends on nothing else in `internal/knowledge/`.

- **AC-01.1** `Decision` and `Invariant` structs exist with exactly the fields
  and JSON tags their schema documents define. `Invariant` mirrors
  `invariant.v1.schema.json`, which means it carries `Statement`, `Rationale`,
  `Severity` and a **required** `Scope` — the design's one-line placeholder
  (`/* mirrors the invariant.v1 schema */`) is resolved here, not by the
  implementer's memory.
- **AC-01.2** `NewDecision(id string, at time.Time, title, decision string, opts ...Option) (Decision, error)`
  and `NewInvariant(id string, at time.Time, title, statement string, opts ...Option) (Invariant, error)`
  exist with those exact signatures, stamp `SchemaVersion: 1`, stamp `Kind`, and
  default `Status` to `StatusActive`.
- **AC-01.3** `created_at` is emitted as UTC RFC 3339 (tech-stack §42). A
  constructor given a non-UTC `time.Time` produces a UTC timestamp; a test
  asserts the serialized string, not the `time.Time`.
- **AC-01.4** `Supersede(prior, replacement Decision) (Decision, Decision, error)`
  returns the replacement carrying `prior.ID` in `Supersedes` and the prior with
  `Status` moved to `StatusSuperseded`. It writes nothing and touches no
  filesystem. It rejects `prior.ID == replacement.ID` and rejects a `prior`
  already superseded, with distinct errors.
- **AC-01.5** Every record produced by these constructors passes the **full**
  `validate.Check` pipeline. A writer whose own validator rejects its output is
  the defect this pairing exists to catch (design §5).
- **AC-01.6** Round-trip: a golden document per kind, decoded into the struct,
  re-encoded, and compared byte-for-byte against the golden. A field silently
  dropped by a wrong struct tag fails here.

### REQ-02 — `internal/knowledge/schema.Validator`: step 5's mechanism

- **AC-02.1** `NewValidator(reg *Registry) (*Validator, error)` and
  `(*Validator) Validate(kind loader.RecordKind, version int, document []byte) []Finding`
  exist with those exact signatures (design §3).
- **AC-02.2** Every document in the registry is registered with
  `AddResource($id, doc)` where `doc` came from `jsonschema.UnmarshalJSON`, and
  compiled with `Compile($id)` using the **same** `$id` string. Passing raw
  `[]byte` to `AddResource` returns `nil` and fails only later at `Compile` with
  `invalid jsonType []uint8` — a test asserts the validator rejects a document
  whose `$id` was not registered, so this trap cannot be reintroduced silently.
- **AC-02.3** `AssertFormat()` is enabled (D-48). A record with
  `created_at: "yesterday"` produces a finding naming `created_at`. **Deleting
  the `AssertFormat()` call must turn this test red.**
- **AC-02.4** No network access on any path. The test that proves it does not
  rely on the machine being offline: removing a required document from the
  registry must make `NewValidator` **fail**, not silently skip the check (D-37).
- **AC-02.5** Compilation happens once, at construction. A malformed shipped
  schema is a defect in the binary and is reported at `NewValidator`, never
  per-record.
- **AC-02.6** Every violation in one document is reported, not the first: the
  `Causes` tree is walked to its leaves. A record breaking `pattern`,
  `minLength`, `uniqueItems`, `additionalProperties`, `format` and `$ref`/`type`
  simultaneously yields six findings. A `$ref` produces a `kind.Reference`
  wrapper whose single child carries the real keyword — the wrapper must not be
  reported as the finding.
- **AC-02.7** `kind.AdditionalProperties` reports the containing object as its
  instance location and names the offending properties only in
  `.Properties`; the finding message must name the offending property, not just
  the parent object.

### REQ-03 — `internal/knowledge/validate.Check`: steps 5–11

- **AC-03.1** `Check(store loader.Store, v *schema.Validator) []Finding` exists
  with that exact signature, opens nothing, and is a pure function of its
  arguments (D-41).
- **AC-03.2** `Step` constants start at 5 and run to 11 with the spec's numbers
  (design §3). `Finding` carries `Path`, `ID`, `Step`, `Code`, `Message`,
  `Fatal`.
- **AC-03.3** Steps run in order and the pipeline does not stop (D-40). A record
  that fails step 5 is **not** asked steps 6–11; every *other* record still is.
  A test with one schema-invalid record and one cycle asserts both are reported
  in one run.
- **AC-03.4** Each of steps 6, 7, 8, 9, 10, 11 detects its condition:
  filename↔kind/id disagreement; a duplicate id; a supersede target that does not
  exist; a supersede cycle; two active records in one lineage; a scope whose
  `level` is not one of the five, or whose `target` is absent for a non-`PROJECT`
  level, or is not a repo-relative forward-slash spelling.
- **AC-03.5** Step 11 validates scope **syntax only**, and its message says so —
  resolution needs MR-005's index (design §6). A syntactically valid target
  naming a file that does not exist is **not** a finding.
- **AC-03.6** Only step 9 sets `Fatal: true` (D-39). Every other finding costs
  the repository the records it names and no more.
- **AC-03.7** D-45's suppression holds, with one over-fire guard per affected
  step: a supersede target whose file the loader reported as a `Problem` yields
  **no** step-8 finding; a chain through such an id yields **no** step-9 finding;
  a lineage missing its unreadable member yields **no** step-10 finding.
- **AC-03.8** Determinism (D-47): `Check` over the same store returns
  byte-identical findings across at least 20 consecutive calls in one process.
- **AC-03.9** `Check` over a valid store returns an empty, non-nil slice.
- **AC-03.10** `Check` over an absent store (`Present: false`) returns no
  findings — a clean clone with no records is healthy, not invalid (D-06).

### REQ-04 — `internal/knowledge/loader` carries the body

- **AC-04.1** `RecordRef.Body []byte \`json:"-"\`` holds the exact bytes read
  (D-46).
- **AC-04.2** `loader.Store`'s marshalled JSON is **unchanged**. A test marshals
  a populated store and compares against the pre-MR-002 shape.
- **AC-04.3** `TestLoadDoesNotPerformMR002Validation` and
  `TestLoadKindComesFromTheDirectory` still pass, unedited. The loader's scope
  line is still steps 1–4.
- **AC-04.4** A companion test in `validate` feeds the **same three records** that
  `TestLoadDoesNotPerformMR002Validation` proves the loader ignores, and asserts
  the pipeline finds each of them, named by step. The pair is what stops either
  side drifting.

### REQ-05 — the code vocabulary

- **AC-05.1** `CodeKnowledgeInvalid` and `CodeKnowledgeSupersedeCycle` are added
  to the const block **and** to `allCodes`.
  `TestCodeRegistryIsUniqueAndExhaustive` AST-parses `code.go` and fails if
  either list is missed.
- **AC-05.2** Both codes have an exit class in `doctor.kindForCode` and in
  `cli_test.exitForCode`; `TestExitClassTableCoversEveryRegisteredCode` passes.
- **AC-05.3** No existing code changes meaning. `KNOWLEDGE_UNREADABLE` continues
  to mean only "this binary could not read the record".

### REQ-06 — `internal/doctor` reports findings distinctly from problems

- **AC-06.1** The knowledge check's state follows D-39/D-43/D-44:
  fatal loader problem → `StateError` / `KNOWLEDGE_SCHEMA_UNSUPPORTED`;
  else cycle finding → `StateError` / `KNOWLEDGE_SUPERSEDE_CYCLE`;
  else any finding → `StateDegraded` / `KNOWLEDGE_INVALID`;
  else existing behaviour, unchanged.
- **AC-06.2** A finding's rendered text names the **rule that was broken and the
  file**, and is distinguishable from a loader problem's text by more than
  wording — by its `Code`. `describeFindings` / `findingRemedies` are new
  helpers; `describeProblems` / `recordRemedies` are not reused, because they say
  "could not be read".
- **AC-06.3** Every new non-OK result carries a registered `Code`, a
  `Diagnostic`, an `Impact` and at least one `NextAction` (`assertDiagnosable`),
  and every `NextAction` is an action, not a statement (`assertActionable`: it
  must not begin with `This `, `There `, `It `, `Mindrail cannot `, `No `, and
  must not mention `mindrail doctor`).
- **AC-06.4** Remedies **name the offending record path**. `status.componentFrom`
  drops `Diagnostic` and `Impact`, so a remedy that says "fix the reported
  records" without naming one is unactionable where `status` prints it (MR-001
  finding R6). Follow `recordRemedies`' `maxNamedRecords = 10` cap and its
  "the remaining N" tail.
- **AC-06.5** The `unreached` branch still claims nothing: no findings count is
  added to the `STARTUP_INCOMPLETE` result.
  `TestNoCheckAssertsAFactAboutAStepThatNeverRan` must stay green.
- **AC-06.6** Still seven checks, in the same order, with the same names
  (decision D-10; `internal/cli/contract_test.go:181`).

### REQ-07 — `internal/status` publishes the count

- **AC-07.1** `KnowledgeInfo` gains `Findings int \`json:"findings"\``, qualified
  by `Observation` exactly as `Problems` is, with no `omitempty`.
- **AC-07.2** `status.Build` reads it from the Subject and constructs no
  validator (D-41).
- **AC-07.3** The human report gains a `Findings` row, and
  `TestStatusJSONParityWithHuman`'s hand-written table gains
  `{label: "Findings:", path: "knowledge.findings"}` — the table is fixed, not
  derived, so a row without an entry passes silently and must not be left out.
- **AC-07.4** `ReadableSchemaVersions` still never marshals as `null`;
  `TestSchemaWindowSurvivesEverySubject` and `TestBuildNeverReportsPartialReady`
  stay green.
- **AC-07.5** A cycle finding drives `Readiness: BLOCKED` with
  `blocking_component: knowledge`; a non-fatal finding does **not** block
  (DEGRADED never blocks).

### REQ-08 — `internal/bootstrap` wires it once

- **AC-08.1** `validate.Check` is called exactly once per process, in bootstrap,
  after the store loads and using the registry already constructed there.
- **AC-08.2** A `schema.NewValidator` failure is a defect in the binary and is
  reported as such — not as a repository condition, and not silently swallowed.
- **AC-08.3** Findings never set `KnowledgeErr` and never halt the startup
  sequence (D-42).

### REQ-09 — the CLI agreement matrix and the one classifier table

- **AC-09.1** `remedyPhrases` in `internal/cli/coherence_test.go` gains the new
  class(es) needed by MR-002's remedies. **Added to, never forked** — MR-001's
  path-contradiction check went blind to five real remedies because the table had
  been split in two.
- **AC-09.2** `knowledgeConditions()` in `internal/cli/agreement_test.go` gains
  **one row per new failure condition**: a supersede cycle (`broken: true`, with
  a declared `remedy` class and a `clear` that clears it), and over-fire guard
  rows (`broken: false`) for a schema-invalid record and for a legitimate
  supersede that must not read as a cycle.
- **AC-09.3** `TestEveryRemedyThatNamesAPathIsClassified` still passes and still
  harvests a non-zero number of remedies.
- **AC-09.4** `TestOneConditionIsNamedTheSameWayByEveryCommand` passes for the
  new rows: `status`, `doctor` and `init` produce one `Code`, one exit int and
  one `next_action` slice for the same bytes; the declared remedy class is
  asserted **before** `clear` runs; after `clear`, all three exit 0.
- **AC-09.5** `TestHealthyRepositoryIsUntouchedByRemedyCoherence` stays green: a
  healthy initialised repository still offers zero remedies.
- **AC-09.6** Every golden regenerated by `-update` is inspected, not just
  accepted. Regenerated files: `internal/cli/testdata/{status_json,init_json,status_human,doctor_human}.golden`,
  `internal/doctor/testdata/doctor_report_{human,json}.golden`,
  `internal/status/testdata/{status_ready_human,status_blocked_human,init_ready_human,init_blocked_human,init_ready_json}.golden`.

### REQ-10 — the dependency

- **AC-10.1** `github.com/santhosh-tekuri/jsonschema/v6 v6.0.3` appears in the
  **direct** require block of `go.mod` (`go get` alone leaves it marked
  `// indirect`; only `go mod tidy` after real code imports it promotes it).
- **AC-10.2** `make tidy-check` exits 0.
- **AC-10.3** `go.sum` gains exactly the six expected lines, including
  `github.com/dlclark/regexp2 v1.11.0`, which is in `go.sum` but not `go.mod`
  because the library marks it "used for testing". This is tidy-stable.
- **AC-10.4** tech-stack §99 requires the version be pinned in `go.mod` and the
  choice recorded; §39's requirement of Draft 2020-12 and no network fetch is
  satisfied by REQ-02.

### REQ-11 — the test discipline MR-001 paid for

Applied from the start rather than after the first audit.

- **AC-11.1 (mutation)** Deleting **any one** of steps 5–11 turns the suite red,
  and the failing test names which step. Demonstrated by actually performing each
  deletion and recording the failing test, not by assertion.
- **AC-11.2 (over-fire)** Every detection has a guard proving the adjacent
  healthy condition does not trip it: a legitimate supersede is not a cycle; a
  second record with a *different* id is not a duplicate; a `PROJECT`-scoped
  record with no target is not a scope violation; a valid store yields nothing.
- **AC-11.3 (no phrase assertions)** No test classifies on a substring that both
  the right and the wrong answer contain (MR-001 rule 2). Assert the `Step`, the
  `Code`, the record path, the extracted field name — never a sentence fragment.
- **AC-11.4 (guards that can fail)** No assertion compares a function to the
  constant it reads, and no bound scales with the constant it guards (the entire
  second MR-001 audit round). Every new assertion is checked by mutating the code
  it guards and confirming it fails.
- **AC-11.5 (writer validates)** REQ-01's AC-01.5.
- **AC-11.6 (CI parity)** Acceptance criterion 4 says rejection holds "on all
  validation paths including CI". No CI entry point exists until MR-018, so the
  test asserts parity across `status`, `doctor` and `init`, and the CI row is
  recorded as **owed to MR-018** — not claimed.

### REQ-12 — the non-goals hold

- **AC-12.1** No CLI write surface. No `mindrail decision add` or sibling. MR-014
  owns the agent-facing write path (design §1).
- **AC-12.2** Steps 12, 13 and 14 are not implemented, and the code names each as
  deferred with the milestone that unblocks it, so a later reader can tell
  "deferred" from "forgotten".
- **AC-12.3** No `mindrail knowledge migrate`, no lazy per-record upgrade. 0.1
  writes 1 and reads `[1]`.
- **AC-12.4** No change to the six status component names, the seven doctor check
  names, or any existing `app.Code` value.

---

## 3. Traceability to the task list's five acceptance criteria

| Task-list criterion | Held by |
|---|---|
| Decision and Invariant records survive a restart | REQ-01 (AC-01.5, AC-01.6) + REQ-04 (AC-04.2) — records are files, and the round-trip proves nothing is dropped |
| A clean clone validates from repository content alone | REQ-03 (AC-03.1, AC-03.10) — `Check` is pure and reads only the store |
| Malformed JSON errors before source validation | REQ-04 (AC-04.3) + REQ-03 (AC-03.3) — step 2 reports, steps 5–11 are not reached for that record |
| Unsupported forward schema and supersede cycles rejected on every path | REQ-05, REQ-06 (AC-06.1), REQ-07 (AC-07.5), REQ-09 (AC-09.4); CI row owed to MR-018 per AC-11.6 |
| Knowledge schema fixture/contract tests | REQ-01 (AC-01.6) + REQ-11 (AC-11.1) |

---

## 4. Work breakdown and dependency order

Waves are serialized where a shared build artefact would otherwise be mutated
under a concurrent reader. `go.mod` is shared by every package, so TASK-01 runs
alone.

| Task | Owns | REQ | Depends on |
|---|---|---|---|
| TASK-01 | dependency + `schema.Validator` | REQ-02, REQ-10 | — |
| TASK-02 | `internal/knowledge/record` | REQ-01 | TASK-01 |
| TASK-03 | `RecordRef.Body` | REQ-04 | TASK-01 |
| TASK-04 | `app.Code` additions + exit classes | REQ-05 | TASK-01 |
| TASK-05 | `validate.Check`, steps 5–11 | REQ-03 | TASK-02, TASK-03, TASK-04 |
| TASK-06 | bootstrap + doctor + status wiring, goldens | REQ-06, REQ-07, REQ-08 | TASK-05 |
| TASK-07 | agreement matrix + classifier + CLI goldens | REQ-09 | TASK-06 |
| TASK-08 | mutation and over-fire sweep | REQ-11 | TASK-07 |

Wave 1: TASK-01 alone. Wave 2: TASK-02, TASK-03, TASK-04 in parallel (disjoint
files). Wave 3: TASK-05. Wave 4: TASK-06. Wave 5: TASK-07. Wave 6: TASK-08.

---

## 5. Definition of done

All of: every `REQ-*` has implementation evidence; `make verify` green;
`make tidy-check` green; the mutation sweep performed and its results recorded;
an independent dual-agent audit reaching `VERIFIED` or
`VERIFIED_WITH_MINOR_ISSUES`; and the audit's own remediation audited in turn
(MR-001 rule 1). Audit budget: **2 rounds, then stop regardless** — the standing
decision from the MR-001 session.
