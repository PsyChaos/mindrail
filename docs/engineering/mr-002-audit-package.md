# MR-002 — audit package

Assembled for an independent audit. It contains what was asked for, what was
built, and what was measured. It deliberately contains no assessment of quality:
the auditors are to reach their own.

---

## 1. Original request, verbatim

> `/tmp/handoff-Yvyr9V.md dosyasini oku ve MR-002 task'ine gececegiz. fakat
> gecmeden once su anda yapilanlari test edebilir miyim? Test edebilirsem nasil
> test edebilirim anlatmani istiyorum.`
> `MR-002 icin "engineering-orchestrator" skilini kullaniriz.`

and, after the test walkthrough was given, `evet basla` — begin.

The handoff being referenced states the task: implement MR-002, whose design is
`docs/engineering/mr-002-design.md`, under the standing session rules that the
audit budget is at most 2 rounds and that a remediation pass needs its own audit.

## 2. The task

MR-002 of `docs/engineering/mindrail-0.1-task-list.md`: the versioned
Decision/Invariant knowledge lifecycle. It implements steps 5–11 of the fourteen
step validation pipeline in `docs/specification/mindrail-technical-specification-1.0.md`
§95, over the repository-owned knowledge store at `.mindrail/knowledge`.

The task list's five acceptance criteria:

1. Decision and Invariant records survive a restart.
2. A clean clone validates knowledge records from repository content alone.
3. Malformed JSON errors before source validation.
4. Unsupported forward schema and supersede cycles are rejected on all validation
   paths including CI.
5. Knowledge schema fixture/contract tests exist.

## 3. Requirements and acceptance criteria

`docs/engineering/mr-002-requirements.md` — **12 requirements, REQ-01…REQ-12,
with roughly sixty numbered acceptance criteria.** Read it in full; it is the
contract this work is graded against.

It was committed at `c2e81d1` **before any implementation commit**, so it did not
take its shape from what was built. Verify that ordering yourself with
`git log --oneline` — the claim is checkable and should be checked.

That document also freezes decisions **D-42 … D-48**, which close gaps the design
document left open. Two of them are load-bearing for correctness rather than
style:

- **D-44** — a fatal loader `Problem` outranks every `Finding`.
- **D-45** — cross-record steps (7–10) reason only over records the loader
  actually read; where a `Problem` names the file a referenced id would occupy,
  the reference is *unverifiable* and no finding is emitted.

## 4. Design contract

`docs/engineering/mr-002-design.md` — package plan, frozen signatures, decisions
D-36…D-41, test plan, non-goals, AC traceability.

**Note for the audit:** two frozen signatures in design §3 do not compile as
written, because `internal/knowledge/loader` already imports
`internal/knowledge/schema`, so `schema.Validate(kind loader.RecordKind, ...)`
closes an import cycle, and `schema.Validate` returning `[]validate.Finding`
closes another. The implementers changed both and reported the changes. Whether
their resolutions are correct is for the audit to decide.

## 5. Implementation summary

| Package | Change |
|---|---|
| `internal/knowledge/record` | new — typed `Decision`/`Invariant`, `NewDecision`/`NewInvariant`, `Supersede`, `Scope`, options, sentinel errors |
| `internal/knowledge/schema` | extended — `Validator`, `NewValidator`, `Validate`, `Finding`, `RecordKind`, compiled-at-construction embedded documents |
| `internal/knowledge/validate` | new — `Check`, `Finding`, `Step` 5…11, the seven steps, lineage analysis, D-45 suppression |
| `internal/knowledge/loader` | extended — `RecordRef.Body []byte \`json:"-"\`` |
| `internal/app` | extended — `CodeKnowledgeInvalid`, `CodeKnowledgeSupersedeCycle` |
| `internal/bootstrap` | extended — builds the validator, calls `Check` once, files findings on the Subject |
| `internal/doctor` | extended — `Subject.KnowledgeFindings`, new branches in `knowledgeResult`, `describeFindings`/`findingRemedies` |
| `internal/status` | extended — `KnowledgeInfo.Findings`, human row, readiness |
| `internal/cli` | extended (tests) — one new remedy class in the single classifier table, new agreement-matrix rows |

## 6. Change references

| Commit | Contents |
|---|---|
| `79716a9` | starting point — MR-001 closed, graph refreshed |
| `c2e81d1` | **the frozen requirements, before any code** |
| `8e5c2e5` | the implementation |
| `4b89e0c` | graph refresh |

`git diff c2e81d1..8e5c2e5` is the whole change. It touches 34 tracked files and
adds 30 new files: 9 production `.go` files, 19 test files, and 2 golden JSON
fixtures.

## 7. Test evidence

Measured by the orchestrator on the committed tree, not reported by an
implementer.

| Command | Result |
|---|---|
| `make verify` | **exit 0.** gofmt, `go vet`, all 16 packages `ok`, race detector all `ok` (`internal/cli` 44.3s), smoke `ok` 1.53s |
| `make tidy-check` | **exit 0** |
| `gofmt -l .` (excluding `testdata/`) | no output |
| `go test ./... -list '.*'` top-level test functions | **619**, against a baseline of **507** at `79716a9` — 112 added |
| `go list ./...` | 16 packages, up from 16 (two new `internal/knowledge/*` packages, `migrations`/`schemas` unchanged) |

Baseline for comparison is recorded in §0 of the requirements document, captured
before implementation began.

## 8. Decision log

D-42…D-48 in `docs/engineering/mr-002-requirements.md` §1, in full, with
reasoning. D-36…D-41 in `docs/engineering/mr-002-design.md` §4.

## 9. Reported deviations from the frozen requirements

Implementers were required to report every deviation. They reported **34** across
seven tasks. The full text of each is in
`docs/engineering/mr-002-audit-package-appendix.md`, alongside every file
changed, every test added with the AC it claims to hold, and every mutation
claimed.

The deviations that change a frozen signature, add unfrozen exported API, or
introduce a runtime failure mode:

- **W1/1** — `schema.Validate` takes `schema.RecordKind`, not `loader.RecordKind`
  (import cycle).
- **W1/2** — `Finding` lives in `schema` as a narrower type
  (`{InstanceLocation, Keyword, Message}`) than the design's `validate.Finding`.
- **W1/3, W2/4** — new exported API not named in the frozen design: in `schema`,
  `RecordKind`/`Finding`/`ErrSchemaNotShipped`/`SchemaNameFor` and two keyword
  constants; in `record`, `Severity`, `ScopeLevel`, `Option` and seven `With*`
  constructors, sixteen `Err*` sentinels, three typed errors, `Scope.Validate`.
- **W1/5** — `NewValidator` now requires every registry document to declare an
  `$id`, which is stricter than the registry's documented "extra documents are
  kept" promise.
- **W2/1** — `NewInvariant`'s third parameter (`title`) is **refused when
  non-empty** rather than stored; `invariant.v1` has no `title` property and sets
  `additionalProperties: false`.
- **W2/2** — `Supersede` returns `(replacement, prior)`, the reverse of its
  parameter order.
- **W2/3** — `Invariant.Scope` is a value, `Decision.Scope` a pointer.
- **W2/5, W2/6** — the constructors are stricter than the pipeline in three
  places (a `PROJECT` scope carrying a target, a zero `created_at`, a record
  superseding its own id).
- **W3/4** — step 6 implements the id half only; the kind half is argued to be
  structurally discharged by step 5.
- **W3/5** — AC-03.4's "level is not one of the five" clause is argued to be
  unreachable through `Check`.
- **W3/6** — **`validate.Check` panics on a nil validator.**
- **W3/7, W3/8** — step 10 counts active ids per lineage rather than active
  files, and takes the conservative reading of D-45.
- **W4/1** — **a `schema.NewValidator` failure in bootstrap is raised as a
  panic.**
- **W4/2** — AC-06.1's ordering places findings ahead of non-fatal loader
  problems, implemented verbatim; the implementer notes a store carrying both
  reports `KNOWLEDGE_INVALID` and says nothing about the unreadable records.
- **W4/4** — no `findings` key was added to the doctor check's `Details` map, so
  three goldens on the affected list regenerated to identical bytes.
- **W5/1** — **one** new remedy class was added, not one per new remedy sentence;
  six step remedies, two tails and a deferred-step fallback are left
  unclassified.

## 10. The independent mutation sweep, and what it found

TASK-08 was not an implementer. It re-performed the mutation sweep independently
rather than trusting the six earlier waves' claims, and reported **41 mutations**
and **seven findings**. Three it fixed, four it reported only.

Fixed:

1. Both exit-class table rows MR-002 added in `internal/cli/envelope_test.go`
   were assertions that could not fail — flipping either value left
   `go test ./...` at exit 0.
2. The wire spellings `"KNOWLEDGE_INVALID"` and `"KNOWLEDGE_SUPERSEDE_CYCLE"`
   were asserted by nothing; renaming either constant's value left the suite
   green, because every test names them through the Go identifier.
3. `validate.Finding.ID` — a published member of the frozen wire shape — was
   asserted nowhere; blanking all four assignments left the suite green.

Reported, not fixed:

4. Three of D-47's four sort keys are unguarded; dropping any one leaves the
   suite green.
5. Step 8's message can stop naming the file the referenced id would occupy and
   the suite stays green; step 7 has an analogous guard, step 8 does not.
6. `internal/doctor/knowledge_findings_test.go:196` compares against
   `pipelineSteps`, which may be the constant it reads.
7. `internal/status/knowledge_findings_test.go:97` asserts one space after
   `"Findings:"` under a message claiming to check alignment against its
   neighbours.

## 11. What the audit is asked to establish

Whether the implementation satisfies `docs/engineering/mr-002-requirements.md`
and the task list's five acceptance criteria, and whether anything in §9 or §10
is a defect rather than a justified deviation.

No conclusion is offered here, and none should be inferred from the ordering or
the emphasis of this document.
