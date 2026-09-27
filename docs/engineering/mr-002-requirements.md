# MR-002 — Frozen requirements and acceptance criteria

- **Frozen at:** commit `79716a9`, branch `mr-002-knowledge-lifecycle`
- **Contract this refines:** [mr-002-design.md](mr-002-design.md). Where the two
  disagree, the design wins on intent and this document wins on the detail the
  design left open — every such gap is recorded below as a decision D-42…D-51.
- **Amended after freezing.** Four remediation waves edited this document, each
  edit marked in the text where it stands.
  - **FIX-D**: **AC-01.2** (finding F-R9, a frozen signature that contradicted
    the frozen schema beside it) and the new **D-49** (finding B-05,
    `created_at`'s spelling). AC-01.3 gained a read-side clause under D-49.
  - **FIX-E**: **AC-02.1** (finding F-R4, a frozen signature that cannot compile
    because it closes an import cycle) and the new **D-50** carrying its
    argument. F-R4's text asks for this to be recorded as "D-49"; that number
    was already taken by FIX-D in the same round, so it is D-50 and D-50 says so.
  - **FIX-I**: **D-49**'s argument only (round-2 finding R-02). The rule D-49
    states is unchanged and no acceptance criterion moved; three of its six
    refusals were recorded as unloadable spellings and are in fact loadable ones
    refused on tech-stack §42's spelling ground. AC-01.3's read-side clause is
    untouched and still one-directional — it bounds what step 5 may *accept*, and
    it never was the authority for refusing a correctly-loading offset.
  - **FIX-J**: the new **D-51** (round-2 findings R-05 and B-A6), recording how a
    record file that resolves outside the repository is published. No acceptance
    criterion moved and no `app.Code` was added; the decision names which of two
    already-registered codes a reading carries, and on what condition.
  - Nothing in D-36…D-48 was reinterpreted by any wave.
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

### D-49 — `created_at`'s spelling is stated in the schema documents, not in the Go decoder

**Added after MR-002 shipped, by the FIX-D remediation wave, in response to
finding B-05. It adds a rule; it does not reinterpret D-36…D-48.**

**Amended by the FIX-I wave, in response to round-2 finding R-02. The rule is
unchanged: the `pattern` in both documents is byte-for-byte the one FIX-D
shipped, and no spelling changed side — verified against `3bd453c` and `45ab5c9`.
What changed is the reason recorded for three of the six refusals, which was
false. The paragraphs below are the amended text; the measurements they now cite
are reproduced in the finding record.**

`format: "date-time"` is RFC 3339, and RFC 3339 is wider than the timestamp this
project writes. It is wider on two grounds, and the amendment exists because the
original text ran them together.

The first ground is that the reader cannot load the spelling at all. Measured
against `record.Decision`, `record.Invariant` and `app.ParseTime`, three
spellings were accepted by step 5 and refused by every one of them: a lowercase
`z` and a lowercase `t` (RFC 3339 §5.6 makes the literals case-insensitive; Go's
`time.RFC3339` layout does not), and a leap second `23:59:60` (§5.7 permits it;
Go's `time.Time` has no representation for it). A record spelled any of these
was called clean and could then not be opened by anything.

The second ground is that the spelling is not the one tech-stack §42 persists,
which is a rule about how a timestamp is written rather than about which instant
it names. Three more spellings are refused on that ground alone, and **each of
them loads, through all three readers, as exactly the instant it spells**:
`2026-01-01T00:00:00-05:00` loads as `2026-01-01T05:00:00Z`, `+03:00` and
`+05:30` likewise, and twelve fractional digits whose tail is zero loads the same
instant `Z` would. §42 says "All persisted timestamps: UTC", and both v1
documents have said "UTC RFC 3339 timestamp (tech-stack §42)" in this property's
own `description` since they shipped at `8e5c2e5` — before there was a `pattern`
to enforce it. The `pattern` makes the document's own published sentence true; it
does not introduce a rule the document did not already state.

One row belongs to both grounds and was described correctly by accident: more
than nine fractional digits **whose tail is not zero** — `.123456789012Z` loads
as `.123456789Z`, a different instant. The original text generalised that to the
whole class, and the zero-tailed member of the class refutes it. Both are
refused, and the honest statement is that the class is refused on §42's spelling
ground, with the non-zero tail additionally lossy.

Both schema documents therefore gain a `pattern` beside the existing `format`,
and the Go decoder is left alone.

Why the rule was not widened instead. R-02 offered that as its first remedy:
narrow the change to what AC-01.3's clause covers and hand the offset question
back to `format`. Measured at `8e5c2e5`, where the `pattern` did not exist,
`created_at: "2026-01-01T00:00:00-05:00"` produced `READY` and zero findings —
`format: "date-time"` accepts every offset RFC 3339 allows, so handing it the
question is not delegating the rule, it is dropping it, and §42's read-side
enforcement would go with it. Nothing this repository can write is affected by
keeping the refusal: the constructors force `.UTC()`, measured — `NewDecision`
given `2026-01-02T03:04:05+05:30` writes `2026-01-01T21:34:05Z`. A stated reason
that is false is a defect even when the behaviour it justifies is right, and the
repair for a false reason is a true reason, not a changed behaviour.

The two keywords divide one rule rather than restating it. `format` owns
calendar validity — no regular expression knows how long February is, and
`2026-02-30T00:00:00Z` is refused by `format` alone. `pattern` owns the
spelling and the offset, which `format` is deliberately permissive about. Two
keywords, two questions, one authority each.

Why the document and not the decoder. D-36 makes the document the contract, and
a `pattern` in the document is the document stating its own rule — not the
second Go implementation D-36 forbids. Widening the decoder instead would put a
hand-rolled RFC 3339 parser in `record`, and it **cannot** close the finding:
`time.Time` has no representation for a leap second, so the decoder would have
to either keep rejecting `23:59:60` (the split survives) or smear it to the next
day (the record loads as an instant it does not name, and AC-01.6's byte
round-trip breaks). The document has to narrow for that row whatever else is
done, and narrowing it once and coherently is simpler than narrowing it partly
and widening Go partly.

Why this is not a silent break of a shipped v1 document. The measured
enumeration of spellings that were valid before and are not now is: lowercase
`z`, lowercase `t`, a leap second, any offset other than `+00:00`/`-00:00`/`Z`,
and more than nine fractional digits. **No record any Mindrail binary has ever
written is in that set** — the constructors force `.UTC()` and `time.Time`
marshals `Z` with at most nine digits — and neither golden fixture nor any
fixture in the suite is either. ~~Every one of the newly-refused spellings was
already a record this binary could not load or could not load faithfully~~
(**struck by the FIX-I wave: false for the three offsets and for a zero-tailed
long fraction, all of which load faithfully — this is finding R-02**). Three of
the six were unloadable, and the change converts a silent failure downstream into
a step-5 finding that names the file. The other three are refused because §42
fixes the spelling of a persisted timestamp, and because the property's own
`description` has said so since `8e5c2e5`; for those the change converts a record
that contradicted the document's published sentence into a finding that names the
file. What is deliberately still accepted, because it is currently valid, parses
identically and denotes the same instant, is `+00:00` and `-00:00`: a pattern
anchored on `Z$` would have refused what most ISO-8601 libraries emit for UTC,
and that would have been a fresh over-fire wearing a fix's clothes. Those two are
also the reason the spelling ground is stated as *zero offset* rather than
*`Z`* — §42 asks for UTC, and `+00:00` is a UTC spelling.

The `description` is not commentary any more, and the FIX-I amendment is partly
about that. Finding R-04 changed step 5's message for a `created_at` `pattern`
violation: it no longer reproduces the expression, because the library renders it
through Go's `%q` and publishes a doubled backslash, so it names the property's
own `description` as the rule instead. A reader holding a record spelled
`-05:00` is now sent to that sentence by the CLI, and the sentence told them
their timestamp could not be read back as the same instant — which is false, and
sends them to look for a decoding bug that is not there. Both documents now state
the two grounds apart, and state the accepted spelling positively so the sentence
answers the question the reader arrived with.

No `schema_version` bump: the window is `[1]`, bumping would mean shipping a v2
document and widening the reader, which orphans every v1 record to fix a rule
the v1 document's own `description` already stated.

`TestNoTimestampIsAcceptedByTheSchemaAndRejectedByTheTypesThatLoadIt` asserts
the property one-directionally — the schema being stricter than Go is harmless,
the reverse is the defect — and
`TestTheTimestampSpellingsTheContractAcceptsAreExactlyTheOnesItShould` is its
over-fire guard. Both documents are held together by
`TestBothDocumentsStateTheSameCreatedAtRule`, which exists because a mutation
proved that deleting the pattern from `invariant.v1` alone was otherwise silent.

The FIX-I amendment added the tests that make *this section's reasons*
falsifiable rather than only its rule.
`TestEveryTimestampTheContractRefusesIsRefusedOnTheGroundTheDocumentsRecord`
carries the six refused spellings with the ground each is refused on, and derives
the ground by asking the three readers rather than by reading anything written
down, so a row whose reason stops being true goes red.
`TestTheCreatedAtDescriptionSaysOnlyThingsThatAreTrueOfThisRepository` is the
pair: each claim is a sentence the shipped `description` must carry and a
measurement that sentence must survive, so reverting the prose is as red as
breaking the rule. `TestTheAmendmentAcceptsEveryTimestampTheContractAcceptedBeforeIt`
is the over-fire arm that holds the accepted set to what `3bd453c` shipped, since
the amendment was to the reason and not to the rule.
`TestTheCreatedAtDescriptionIsTheSameSentenceInBothDocuments` extends the older
drift guard to the `description`, which R-04 made load-bearing — the older test
compares only the two `pattern` values, and would sleep through a description
that drifted between the two kinds.

One consequence for D-48: `created_at: "yesterday"` now breaks both keywords,
and this library reports one finding per instance location. AC-02.3's guard
still fails when `AssertFormat()` is deleted — it asserts the keyword, and the
finding becomes `/created_at pattern` — but that made the guard depend on a
tie-break. `TestValidatorAssertsTheDateTimeFormat` gained a second row,
`2026-02-30T00:00:00Z`, which only `format` can refuse, so D-48's guard no
longer rests on which keyword wins.

### D-50 — `Validator.Validate` takes `schema.RecordKind`, because `loader.RecordKind` cannot compile

**Added after MR-002 shipped, by the FIX-E remediation wave, in response to
finding F-R4. It records a change the implementation had already made and this
document had not; it does not reinterpret D-36…D-49.**

**This is D-50 and not D-49 because D-49 was taken first.** The FIX-D wave
landed `created_at`'s spelling rule as D-49 in the same remediation round, and
renumbering a decision that is already cited from
`internal/knowledge/schema/validator_test.go`,
`schemas/knowledge/*.schema.json` and §1 of this document would be worse than
the gap in the finding's own numbering. A reader following F-R4 to "D-49" lands
here.

Design §3 and AC-02.1 both froze:

```go
func (v *Validator) Validate(kind loader.RecordKind, version int, document []byte) []Finding
```

That signature **cannot exist in this binary**, and the reason is structural
rather than a matter of taste. `internal/knowledge/loader` imports
`internal/knowledge/schema` — it holds a `*schema.Validator` and calls
`schema.NewRegistry` — so `schema` naming `loader.RecordKind` closes an import
cycle, and Go refuses to build it. No ordering of the two packages fixes this
while step 5's mechanism lives in `schema` and the loader is the thing that
calls it.

The resolution, taken during implementation and recorded only in
`mr-002-audit-package-appendix.md` until now:

```go
type RecordKind string
const (
	KindDecision  RecordKind = "decision"
	KindInvariant RecordKind = "invariant"
)

func (v *Validator) Validate(recordKind RecordKind, version int, document []byte) []Finding
```

`schema` declares its own `RecordKind` with the two values spelled identically,
and a caller converts at the call site with `schema.RecordKind(ref.Kind)`.
Arity, parameter order, parameter meaning and return type are all unchanged;
only the named type of the first parameter differs, and it differs by a
conversion that is free at run time.

The alternative considered and rejected was a plain `string` first parameter.
It compiles too, and it throws away the one place type safety is worth having:
the boundary where two same-shaped string types meet.

What the change costs, and what pays for it. A conversion between two named
string types is silent when the strings themselves drift, so the compiler stops
being the thing that holds them together.
`TestSchemaKindsAreSpeltTheSameWayAsTheLoaders` asserts
`schema.RecordKind(loader.KindDecision) == schema.KindDecision` for both kinds
and drives a `Validate` call through the converted value — an *external* test
package may import `loader` without closing the cycle, which is why the
assertion is possible at all.
`TestKindConstantsAgreeWithTheDocumentsTheLoaderAndTheSchemaPackage` extends the
same equality to `record`'s constants and to the `const` in each shipped schema
document, so all four spellings fail together or not at all.

**AC-02.1 is corrected below to the signature the binary has.** The frozen form
was not a choice that was later revisited; it was a form that never compiled,
and leaving it in the contract would make a reader grade a working binary
against an impossible one.

### D-51 — a declined record is published as `PATH_ESCAPES_ROOT` when the degraded set is homogeneous, and stays `KNOWLEDGE_UNREADABLE` when it is mixed

**Added after MR-002 shipped, by the FIX-J wave, in response to round-2 findings
R-05 and B-A6. It resolves a condition the first remediation introduced and left
unnamed. No acceptance criterion moves and no new `app.Code` is registered; both
codes named here were already in the wire vocabulary.**

**The condition.** The first remediation closed finding BA-10 by putting the
containment boundary in front of every record file, not only in front of the
bucket directory: a record that is a symbolic link resolving outside the
repository is refused rather than read and counted as repository content. The
loader files that refusal as a non-fatal `loader.Problem` carrying
`app.CodePathEscapesRoot`. The report layers did not read that code. Every
non-fatal loader problem produced one reading — `KNOWLEDGE_UNREADABLE`, summary
"Knowledge store has unreadable records", remedy "Fix or remove `<path>`" — so a
file this binary never opened was published as one it could not read.

**Why that is a defect and not a wording preference.** The code is the value a
consumer branches on, and `KNOWLEDGE_UNREADABLE` is a claim about a read attempt
that never happened: `loader.recordFile` decides `PATH_ESCAPES_ROOT` by resolving
the path, before anything is opened. The remedy inherits the falsehood and becomes
unfollowable — the file's contents are not under discussion, and "remove it"
removes a link whose target the reader may still want. The thing to act on is the
link, and only a reading that knows the record was *declined* rather than *unread*
can say so. This is the same defect shape as finding E10, one level further down:
one condition wearing another condition's name, with that name's remedy attached.

> **Amended after audit round 4 (finding R4-M22).** As first written, the two
> paragraphs above justified the new reading by saying the declined file "reads
> perfectly" and that "nothing is wrong with its bytes, its permissions or its
> JSON". That is the same shape of fabricated claim D-51 exists to remove, pointed
> the other way: a link whose target has been deleted, or cannot be opened, is
> declined identically, and no layer in this pipeline has ever held its bytes. The
> warrant is not that the file is fine — it is that **this run knows nothing about
> the file's contents and must not speak about them.** Nothing about the decision's
> content changes: the code, the summary, the remedy split and the homogeneity test
> are all as ruled. Only the ground under them is corrected, and it is a stronger
> one.

**The rule.**

- The loader's account of the condition does not move. `escapingRecord` keeps
  `app.CodePathEscapesRoot`, keeps `Fatal: false`, and keeps its comment and its
  message — the function's body is byte-identical across this decision, and only
  its call site moved when `recordFile` was extracted for a separate finding. The
  classification belongs at the layer that detected it, and `doctor` reads
  `loader.Problem.Code` rather than the prose in `Message` — for the reason D-43
  gives about findings: a classification taken from a sentence disagrees with the
  published code the first time the sentence is reworded.
- `doctor.knowledgeResult` gains one branch. When the **degraded** loader-problem
  set is non-empty and **every** member of it carries `app.CodePathEscapesRoot`,
  the reading is published under that code, with its own summary, diagnostic,
  impact and remedy.
- When the degraded set is **mixed**, the reading stays `KNOWLEDGE_UNREADABLE`.

**Why homogeneity is the condition, rather than "any escaping record".** A
component publishes exactly one code (AC-06.4). Over a mixed set,
`KNOWLEDGE_UNREADABLE` is wrong about none of it — a declined record is, from a
consumer's side, a record that is not in the store — while `PATH_ESCAPES_ROOT`
would be a false statement about the genuinely unreadable half, which is the
defect this decision exists to remove, reintroduced from the other direction. The
tie is broken towards the code that overclaims about nothing. The summary is the
one prose field `status` prints beside the code, so a mixed set says
"Knowledge store has unreadable records and records that resolve outside the
repository" and names one remedy per record in that record's own class. Nothing
is filed under the other class's sentence, and nothing is silent.

**Exit class, measured rather than derived.** The reading is `DEGRADED` at
`app.ExitSuccess` from `doctor --json`, `status --json` and `init --json`, with
no error object on any of them; `status` readiness is `DEGRADED` and no
`blocking_component` is named. The contrast that makes the grade legible is the
`.mindrail/knowledge` *directory* resolving outside the worktree, which is the
same code on the same policy and is `BLOCKED` at exit 2 with
`blocking_component` `knowledge` — measured on the same binary. One condition
costs the repository a record; the other costs it the store. `app.CodePathEscapesRoot` maps to `KindUsage` and
therefore to exit 2 *where it is a fatal error object* — the `.mindrail/knowledge`
directory escaping is such a case — and that mapping is not on this path, because
`doctor.errorFrom` is reached only for an ERROR reading or the halting one.
Declining one record costs the repository that record and no more, which is
decision D-39's grade for a record this binary cannot use, and the same grade the
unparseable record beside it already had. Promoting one bad record to a usage
error would be a new policy, not a fix.

**Remedy wording**, in full, because it is the field a consumer acts on:
`Replace the link at <repo-relative slash path> with the record itself, or
remove it.` Past the `maxNamedRecords` cap of 10: `Replace or remove the
remaining N links under .mindrail/knowledge that resolve outside the repository
root.` Paths are repo-relative (tech-stack §74), like every other knowledge
remedy. Neither sentence is classified by `internal/cli`'s shared remedy table,
and that is deliberate and stated as a fact rather than left to inference: a
DEGRADED reading produces no error object, so no agreement-matrix row can hold
the binary to a class for it and a class added here would be a claim nothing
could falsify. `TestTheRemedyClassifierRefusesASentenceThatMerelyContainsAVerb`
carries both sentences with `classUnknown`, which is also what keeps a future
entry matching a bare "the link" from collapsing this condition into
`classRelink` — a dangling link to be pointed somewhere real and a link that
resolves perfectly to something this repository does not own are two conditions
with two remedies.

**What holds it.** `internal/doctor/knowledge_escaping_test.go` carries the
regression, the over-fire guard (a genuinely unreadable record still reports
`KNOWLEDGE_UNREADABLE`; a store with nothing declined does not enter the branch)
and the under-fire guard (the declined record keeps its class below every
condition that outranks it on AC-06.1's ladder). At the layer a user meets it,
`TestBrokenSetupMatrix` carries "knowledge record that resolves outside the
repository" and its mixed-set twin, and `TestOneConditionIsNamedTheSameWayByEveryCommand`
carries the exit-0 half — that `status`, `doctor` and `init` all agree this disk
costs the repository one record and stops nothing.

### D-52 — a supersede node is owned by the record filed at the path its id names

**Added by the third remediation, after three audit rounds each flipped this seam
the other way.** It replaces the case-by-case rules the first two remediations
patched in, and it is written as one sentence deliberately: the previous three
attempts were each correct about the case in front of them and silent about the
case behind them.

#### The history this decision exists to end

| Round | What was wrong | Direction |
|---|---|---|
| 1 | An unknown property on a cycle member deleted its edges, so the cycle stopped being reported | too loose |
| 2 | A schema-rejected draft duplicating a valid id injected its edges, manufacturing a fatal cycle against innocent files | too tight |
| 3 | A schema-valid but **misfiled** record vouched for a node and deleted the canonical record's edge, so the cycle stopped being reported | too loose |

Each fix asked "is this record credible?" and answered with a different property —
`schemaValid`, then `schemaValid` plus whether anyone else was accepted. Neither
property is what the question is about. **Credibility of an id claim is not a
property of the record's contents; it is a property of where the record sits.**

#### The rule

Spec §95 step 6 already fixes the file an id belongs in: the record for id `X` of
kind `k` lives at `.mindrail/knowledge/<k-dir>/X.json`. That mapping is a
requirement, not a guess, and file names are unique within a directory — so **at
most one record can be canonical for any id.**

1. **A node's edges come from its canonical record** — the one filed at the path its
   id names — **whether or not step 5 accepted it.** A record's claim about *what it
   supersedes* is read from its own bytes, and a schema violation elsewhere in the
   document does not make that field a lie. This is round 1's case.
   A record is canonical for the id it carries when **its own file name spells that
   id** — step 6's id half, `ref.ID == idFromPath(ref.Path)`; a record merely sitting
   at `X.json` while carrying another id is canonical for nothing.
2. **A non-canonical claimant never supplies edges to a node it does not own, and
   never removes them.** It is still indexed so `resolves` sees it and step 8 cannot
   call its file absent, and — when step 5 accepted it (D-40) — it still receives its
   own step-6 finding. This is rounds 2 and 3, which are the same case seen from two
   sides.
3. **An id with no canonical record supplies no edges at all.** Nothing in the store
   establishes which record that id is, so no cycle verdict may be built on it.
   Step 6 already tells the reader, on every claimant, that the file name and the id
   disagree; once they fix that, a canonical record exists and rule 1 applies.
4. **Ownership governs reporting as it governs edges.** A step-9 or step-10 finding
   for node `X` attaches only to `X`'s canonical record, and only when step 5 accepted
   it. **A non-canonical claimant is never named in a verdict about a node it does not
   own.**

Rule 3 is deliberately the conservative arm. A genuine cycle among wholly misfiled
records is reported in two steps rather than one — first "these files are misfiled",
then, after the reader acts, the cycle. That costs a round trip. The alternative
costs a **fatal** claim assembled from records whose identity the store cannot
establish, carrying a remedy the named files may be unable to perform — which is
precisely the defect rounds 2 and 3 produced, and D-39 makes this the one fatal
finding in the milestone. A false fatal is worse than a true fatal discovered one
step later.

#### Why this is not a fourth patch

The previous three rules were each derived from the failing input. This one is
derived from step 6, which the pipeline already enforces and which the requirements
already freeze. It introduces no new concept: "canonical" is just the name for the
relationship step 6 tests.

#### What holds it

The implementation must be checked against **every store the three audit rounds
constructed**, not against a new fixture set — specifically round 1's fail-open,
round 2's over-fire, round 3's misfiled voucher, and the two-schema-valid-duplicates
case that has been present since `8e5c2e5`. Round 3's Breaker enumerated 4,864
stores over this state space; the rule must be evaluated against that enumeration.

Two stores must be in the test set by name, because they are where the rule was
nearly wrong:

- **Round 3's own store** — a step-5-rejected canonical `DEC-0001.json` superseding
  `DEC-0002`, a valid `DEC-0002.json` superseding `DEC-0001`, and a fully valid
  `DEC-0009.json` carrying id `DEC-0001`. Rules 1 and 2 restore the cycle; **rule 4
  is what stops the fatal finding landing on `DEC-0009.json`**, whose bytes assert no
  supersede at all and which cannot perform the remedy. Assert both halves: the cycle
  is reported, and `DEC-0009.json` is named in no step-9 finding.
- **Two misfiled records forming a true cycle** — rule 3's cost, stated end to end.
  The store reports two step-6 findings and no cycle at exit 0; after the reader
  renames both files, the same store reports the cycle as fatal. Assert the two-step
  path, not just the first step, so nobody later reads the exit 0 as a clean bill.

**If any case in that enumeration comes out wrong under this rule, the rule is
wrong.** Report it and stop — do not add a fifth exception. A rule that needs an
exception to survive its own state space is the thing this decision was written to
replace.

#### Review

This decision was reviewed adversarially by a second model before implementation
began, against the four known cases and constructed attacks. It returned
`SOUND_WITH_AMENDMENTS`: the ownership criterion survived every attack on the **edge**
side, but the decision as first written governed only who *supplies* edges and was
silent about who *receives* a finding — and on round 3's own store that silence
reproduced round 2's defect through a different door. Rule 4 and the canonical
definition in rule 1 are that review's amendments; the `(D-40)` qualification in
rule 2 closes a contradiction it found between rule 2's unqualified step-6 clause and
D-40's letter. The review also endorsed rule 3's trade after arguing both sides, and
asked for the second store above.

#### Outcome

*Added after implementation.* The rule was implemented in `newGraph` (edges) and
`askedFor` (reporting) and graded against the enumeration this decision demands.
All 4,864 stores agree with an oracle written from the four rules above and sharing
no line with the implementation, on the exact set of files step 9 reports — not
merely on whether it reported anything. The oracle finds 1,169 stores holding a
reportable cycle, the same population round 3's Breaker reported for the same space.
**No case came out wrong, so no fifth exception was added.** Both named stores are
in the test set. `docs/engineering/mr-002-findings.md` Appendix E records the whole
of it.

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
  and `NewInvariant(id string, at time.Time, statement string, opts ...Option) (Invariant, error)`
  exist with those exact signatures, stamp `SchemaVersion: 1`, stamp `Kind`, and
  default `Status` to `StatusActive`.

  > **Amended after MR-002 shipped, by the FIX-D remediation wave, in response
  > to finding F-R9.** As originally frozen, this criterion gave `NewInvariant`
  > a fourth string parameter, `title`, placed before `statement`. That
  > contradicted AC-01.1 immediately above it: AC-01.1 freezes the `Invariant`
  > as a mirror of `invariant.v1.schema.json`, and that document declares no
  > `title` property and sets `additionalProperties: false`, so a title had
  > nowhere to go. The implementation honoured both halves by accepting the
  > parameter and then refusing every value it could hold except `""` —
  > measured, the parameter's domain was the single point `{""}` — which left a
  > published constructor carrying an argument that could never be anything and
  > a run-time error standing in for a compile-time fact.
  >
  > The two halves could not both stand. The half backed by the schema document
  > wins, because D-36 makes the document the contract, and a signature is not
  > entitled to promise a field the contract does not define. So the parameter
  > is removed rather than the document changed, and `ErrInvariantHasNoTitle`
  > goes with it: a sentinel no input can produce is a guard no mutation can
  > falsify. `TestNewInvariantHasNoParameterForAPropertyItsSchemaDoesNotDeclare`
  > reads the property set back out of the shipped document, so if a future
  > `invariant.v2` does declare a `title`, that test says the signature owes a
  > parameter again.
  >
  > This is a source-breaking change to an exported constructor. It had no
  > non-test caller in the repository at the time of the amendment.

- **AC-01.3** `created_at` is emitted as UTC RFC 3339 (tech-stack §42). A
  constructor given a non-UTC `time.Time` produces a UTC timestamp; a test
  asserts the serialized string, not the `time.Time`.

  Read side (**added by the FIX-D wave for finding B-05; see D-49**): no
  `created_at` accepted by step 5 may be one `record.Decision`, `record.Invariant`
  or `app.ParseTime` cannot load, or loads as a different instant. This is the
  dual of AC-01.5 and is asserted in that one direction only — a contract
  stricter than the decoder is harmless, because nothing reaches a decoder
  without passing step 5 first.
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
  `(*Validator) Validate(recordKind RecordKind, version int, document []byte) []Finding`
  exist with those exact signatures (design §3), where `RecordKind` is
  `schema.RecordKind` and `Finding` is `schema.Finding`.

  > **Amended after MR-002 shipped, by the FIX-E remediation wave, in response
  > to finding F-R4; see D-50.** As frozen, this criterion and design §3 both
  > spelled the first parameter `loader.RecordKind`. That form does not compile:
  > `internal/knowledge/loader` imports `internal/knowledge/schema`, so naming
  > `loader.RecordKind` here closes an import cycle. The implementation shipped
  > `schema.RecordKind` from the start and this document had not caught up, so
  > the criterion was grading the binary against a signature no binary can have.
  > Arity, order, parameter meaning and return type are unchanged; the two
  > `RecordKind` types are held to the same strings by
  > `TestSchemaKindsAreSpeltTheSameWayAsTheLoaders`. D-50 carries the full
  > argument, including why a plain `string` was rejected.
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
