# MR-002 — Versioned Decision/Invariant knowledge lifecycle

- **Task:** [mindrail-0.1-task-list.md](mindrail-0.1-task-list.md) MR-002
- **Blocked by:** MR-001 (closed)
- **Status:** contract for implementation

---

## 0. What MR-001 left here, exactly

MR-001 implemented spec §95's validation pipeline down to step 4 and stopped on
a marked line. `internal/knowledge/loader.Load`'s doc comment says so, and
`TestLoadDoesNotPerformMR002Validation` asserts it: a record whose file name
disagrees with its id, a duplicate id, and a record missing every required field
all load today without a single problem being recorded.

The pipeline, from the specification:

```text
 1. File read                              MR-001
 2. JSON syntax                            MR-001
 3. schema_version parse                   MR-001
 4. reader compatibility                   MR-001
 5. JSON Schema validation                 MR-002
 6. filename ↔ kind/id consistency         MR-002
 7. unique ID                              MR-002
 8. supersede target                       MR-002
 9. supersede DAG/cycle                    MR-002
10. duplicate active lineage               MR-002
11. scope syntax                           MR-002
12. scope resolution when index available   deferred — needs MR-005
13. referenced validation profile           deferred — needs MR-010
14. referenced evidence/test mapping syntax deferred — needs MR-011/MR-012
```

**MR-002 owns steps 5 through 11.** Twelve, thirteen and fourteen name things
that do not exist yet: there is no structural index to resolve a scope target
against, no validation profile to reference, and no evidence mapping. Each is
recorded as a non-goal in §6 with the milestone that unblocks it, so a later
reader can tell "deferred" from "forgotten" — which is the distinction this
project's findings keep turning on.

## 1. Scope decision: no CLI write surface

MR-002 completes creation, reading, validation and supersede as a **package
API**. It adds no `mindrail decision add`-style command.

Three reasons, in order of weight:

1. **None of the five acceptance criteria needs one.** They are about records
   surviving a restart, a clean clone validating from repository content alone,
   malformed JSON failing before source validation, unsupported schemas and
   supersede cycles being rejected everywhere including CI, and fixture tests.
   A write command is orthogonal to all five.
2. **MR-014 owns the agent-facing surface.** The user ruled during MR-001 that
   writing a protocol block advertising thirteen non-existent MCP tools was a
   correctness hazard; inventing a CLI write path ahead of the milestone that
   owns record creation is the same mistake with a different noun. MR-001 made
   `init` the only command that writes and recorded that as a decision.
3. **`.mindrail/knowledge` is repository content.** It is committed, reviewed and
   diffed (spec §9). A Decision is a reviewed artefact, and authoring it as a
   file that a human or an agent writes — then validated by this pipeline — is
   the model the storage layout already implies.

The creation lifecycle is still built, as `record.NewDecision` / `NewInvariant` /
`Supersede`, because MR-014 will need exactly that API and because it lets the
validation path be tested against a real writer rather than only against
hand-made fixtures. A writer and a validator that disagree is a defect neither
can see alone.

## 2. Package plan

| Package | Owns | New? |
|---|---|---|
| `internal/knowledge/record` | The typed Decision and Invariant, their construction, and the supersede transition | new |
| `internal/knowledge/validate` | Steps 5–11 as one deterministic pipeline over a loaded store | new |
| `internal/knowledge/schema` | Compiling the embedded documents and validating one decoded record against them (step 5's mechanism) | extended |
| `internal/knowledge/loader` | Unchanged contract; gains the decoded body the validator needs | extended |
| `internal/doctor`, `internal/status` | Report validation findings through the existing knowledge component | extended |

Dependency direction is unchanged and one-way: `record` depends on nothing in
this tree, `schema` on the embedded documents, `validate` on `record`, `schema`
and `loader`, and the report layers on `validate`.

## 3. Frozen signatures

```go
// internal/knowledge/record

type Decision struct {
    SchemaVersion int       `json:"schema_version"`
    Kind          string    `json:"kind"`
    ID            string    `json:"id"`
    Status        Status    `json:"status"`
    CreatedAt     time.Time `json:"created_at"`
    Title         string    `json:"title"`
    Context       string    `json:"context,omitempty"`
    Decision      string    `json:"decision"`
    Consequences  string    `json:"consequences,omitempty"`
    Supersedes    []string  `json:"supersedes,omitempty"`
    Scope         *Scope    `json:"scope,omitempty"`
    Tags          []string  `json:"tags,omitempty"`
}

type Invariant struct { /* mirrors the invariant.v1 schema */ }

type Status string
const (
    StatusActive     Status = "active"
    StatusSuperseded Status = "superseded"
)

type Scope struct {
    Level  ScopeLevel `json:"level"`
    Target string     `json:"target,omitempty"`
}

func NewDecision(id string, at time.Time, title, decision string, opts ...Option) (Decision, error)

// Amended after MR-002 shipped (finding F-R9). This sketch originally gave
// NewInvariant a `title` parameter before `statement`, which contradicted the
// same section's own statement that Invariant mirrors invariant.v1 — a document
// that declares no "title" and sets additionalProperties:false. The parameter
// could therefore never carry a value, and the implementation refused every one
// except "". D-36 makes the document the contract, so the parameter went. See
// AC-01.2 in mr-002-requirements.md for the full argument.
func NewInvariant(id string, at time.Time, statement string, opts ...Option) (Invariant, error)

// Supersede returns the pair the caller must write: the replacement carrying
// `supersedes`, and the prior record with its status moved to superseded. It
// returns them rather than writing, because what a caller does with two files
// that must land together is a transaction question and this package has no
// filesystem.
func Supersede(prior Decision, replacement Decision) (Decision, Decision, error)

// internal/knowledge/schema

// Validator compiles the embedded documents once. Compilation is where a
// malformed shipped schema is found, and finding it per record instead would
// report a defect in the binary as a defect in the repository.
type Validator struct{ /* ... */ }
func NewValidator(reg *Registry) (*Validator, error)

// Amended after MR-002 shipped (finding F-R4). This sketch originally spelled
// the first parameter `loader.RecordKind`. That signature does not compile:
// internal/knowledge/loader imports internal/knowledge/schema, so naming a
// loader type here closes an import cycle. schema declares its own RecordKind
// with the two values spelled identically, callers convert with
// schema.RecordKind(ref.Kind), and TestSchemaKindsAreSpeltTheSameWayAsTheLoaders
// holds the two spellings equal. Arity, order and return type are unchanged.
// See D-50 in mr-002-requirements.md for the full argument, including why a
// plain `string` was rejected.
type RecordKind string
const (
    KindDecision  RecordKind = "decision"
    KindInvariant RecordKind = "invariant"
)
func (v *Validator) Validate(recordKind RecordKind, version int, document []byte) []Finding

// internal/knowledge/validate

// Finding is one thing wrong with one record, named by the pipeline step that
// found it so a report can say which rule was broken rather than only that one was.
// Note that schema.Finding above and validate.Finding here are two distinct
// types with the same short name: the one returned by Validate carries a keyword
// and an instance location, and this one adds the path and the step that the
// schema package deliberately does not know.
type Finding struct {
    Path    string   `json:"path"`    // repo-relative, slash
    ID      string   `json:"id,omitempty"`
    Step    Step     `json:"step"`
    Code    app.Code `json:"code"`
    Message string   `json:"message"`
    Fatal   bool     `json:"fatal"`
}

type Step int
const (
    StepSchema Step = iota + 5 // the numbers are spec §95's, deliberately
    StepFilenameConsistency
    StepUniqueID
    StepSupersedeTarget
    StepSupersedeCycle
    StepDuplicateActiveLineage
    StepScopeSyntax
)

// Check runs steps 5-11 over one loaded store, in order, and returns every
// finding. It is a pure function of the store: it opens nothing.
func Check(store loader.Store, v *schema.Validator) []Finding
```

## 4. Decisions

Numbering continues MR-001's, which ended at D-35.

- **D-36 — the embedded schema documents are the contract, not the Go structs.**
  Step 5 validates through `santhosh-tekuri/jsonschema/v6` against the documents
  in `schemas/knowledge/`, as tech-stack §39 prescribes, rather than by
  hand-rolling the same rules in Go. A second implementation of `pattern`,
  `const`, `uniqueItems` and `additionalProperties: false` would drift from the
  JSON, and then two things would claim to be the rule.
- **D-37 — schemas are registered by `$id`, never fetched.** The compiler gets
  every document through `AddResource` before `Compile`, so validation performs
  no network access on any path (tech-stack §39). A test asserts that a compiler
  with no network still validates, and that removing a document fails
  compilation rather than silently skipping the check.
- **D-38 — a validation finding is not the same as a load problem.** The loader's
  `Problem` means "this binary could not read the record". A `Finding` means
  "this binary read it and it is wrong". They travel separately because their
  remedies differ: one is fixed by upgrading Mindrail, the other by editing a
  file the repository owns.
- **D-39 — only steps 4 and 9 are fatal.** An unreadable newer schema stays fatal
  (MR-001's rule). A supersede cycle joins it, because a cycle means no lineage
  has an end and every answer about "which Decision is current" is wrong. Every
  other finding costs the repository the records it names and no more, so one
  malformed record cannot make a working repository unusable — the over-fire this
  project has now produced twice.
- **D-40 — the pipeline runs in order and does not stop.** A record that fails
  step 5 is not asked steps 6–11, because the later steps read fields step 5 has
  just said are wrong; but *other* records still are. A reader fixing their
  knowledge store should see every problem in one run rather than one per run.
- **D-41 — `Check` is a pure function of a loaded store.** It performs no I/O, so
  `status`, `doctor` and CI reach identical verdicts from identical bytes, and a
  test needs no filesystem to exercise a rule.

## 5. Test plan

The discipline MR-001 arrived at applies from the start rather than after the
first audit:

- **Every rule gets a mutation test.** Deleting any of steps 5–11 must turn the
  suite red. The step numbering exists partly so the test table can name which.
- **Every detection gets an over-fire guard.** A valid store must stay valid: one
  row per rule proving a healthy repository is unaffected, and one proving the
  adjacent condition does not trip it — a record that legitimately supersedes
  another must not read as a cycle, a second record with a *different* id must
  not read as a duplicate.
- **The writer validates.** Every record `record.NewDecision`/`NewInvariant`
  produces is fed through the full pipeline and must pass. A writer that emits
  something its own validator rejects is the defect this pairing exists to catch.
- **Fixtures are round-tripped, not asserted by eye.** A golden document per kind,
  decoded, re-encoded and compared, so a field silently dropped by a struct tag
  fails.
- **The agreement matrix gains a row per new failure condition**, per the rule
  MR-001 established: `status`, `doctor` and `init` must name each new condition
  with one code, one exit class and one remedy, and the remedy must be carried
  out and clear it.
- **CI path parity.** Acceptance criterion 4 says unsupported schemas and cycles
  are rejected "on all validation paths including CI". Until MR-018 exists there
  is no CI entry point, so the test asserts parity between the paths that do
  exist and records the CI row as owed to MR-018 rather than claiming it.

## 6. Explicit non-goals

- **Steps 12–14.** Scope resolution needs MR-005's index, a referenced validation
  profile needs MR-010, evidence/test mapping needs MR-011/MR-012. Step 11
  validates scope *syntax* only, which is all that can be checked without an
  index, and says so in its finding message.
- **A CLI write surface** — see §1.
- **`mindrail knowledge migrate` and lazy per-record upgrade.** Kernel scope §
  defers both explicitly; 0.1 writes 1 and reads [1].
- **Mixed readable versions.** The window is `[1]`, so the reader-compatibility
  branch is exercised but no second generation exists to read.

## 7. Acceptance criteria traceability

| Criterion | Held by |
|---|---|
| Records survive restart | round-trip through `.mindrail/knowledge` + the existing loader tests |
| A clean clone validates from repository content alone | `Check` is pure and reads only the store; a test runs it against a fresh clone fixture with no runtime database |
| Malformed JSON errors before source validation | ordering test: step 2 reports and steps 5–11 are not reached for that record |
| Unsupported forward schema and supersede cycles rejected on every path | D-39's two fatal rules, asserted through `status`, `doctor` and the agreement matrix; the CI row is owed to MR-018 |
| Knowledge schema fixture/contract tests | §5's fixture round-trip and the per-step mutation table |
