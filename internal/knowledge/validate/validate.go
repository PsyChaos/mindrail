// Package validate performs spec §95's validation pipeline over one loaded
// knowledge store: steps 5 through 11.
//
// Steps 1-4 — file read, JSON syntax, schema_version parse, reader
// compatibility — belong to internal/knowledge/loader and have already run by
// the time a Store reaches this package. What arrives here is the set of
// records this binary could read, plus the loader's account of the ones it
// could not.
//
//	step 5   JSON Schema validation             StepSchema
//	step 6   filename <-> kind/id consistency   StepFilenameConsistency
//	step 7   unique ID                          StepUniqueID
//	step 8   supersede target                   StepSupersedeTarget
//	step 9   supersede DAG/cycle                StepSupersedeCycle
//	step 10  duplicate active lineage           StepDuplicateActiveLineage
//	step 11  scope syntax                       StepScopeSyntax
//
// Steps 12, 13 and 14 are deferred rather than forgotten, and each names the
// milestone that unblocks it. Step 12 resolves a scope target against the
// repository's structure and needs MR-005's index; step 13 follows a referenced
// validation profile and needs MR-010; step 14 checks evidence and test mapping
// syntax and needs MR-011/MR-012. Step 11 therefore judges a scope's spelling
// and says so in its message, because a target naming a file that is not there
// is a question this milestone has no index to answer.
//
// Three properties of this package are load-bearing rather than incidental.
//
// Check is a pure function of its arguments (decision D-41). It opens nothing,
// stats nothing and creates nothing, so `status`, `doctor` and a future CI entry
// point reach identical verdicts from identical bytes, and every test in this
// package builds its store in memory.
//
// The pipeline runs in order and does not stop (decision D-40). A record that
// fails step 5 is not asked steps 6-11, because those steps read fields step 5
// has just said are wrong; every *other* record still is. A reader fixing their
// knowledge store sees every problem in one run rather than one per run.
//
// The cross-record steps reason only over records the loader actually read
// (decision D-45). Where a loader Problem names the file a referenced id would
// occupy, the reference is unverifiable and no finding is emitted for it: the
// record exists, this binary could not read it, and publishing "it does not
// exist" would be a fabricated claim about a file the loader has already named.
package validate

import (
	"cmp"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/knowledge/loader"
	"github.com/PsyChaos/mindrail/internal/knowledge/schema"
)

// Step names the spec §95 pipeline step that produced a Finding.
//
// The numbers are the specification's, deliberately: a report that says "step 9"
// says the same thing the specification, the design and this package say, and a
// renumbering would silently move every finding onto a different rule.
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

// Finding is one thing wrong with one record, named by the pipeline step that
// found it so a report can say which rule was broken rather than only that one
// was.
//
// It is data, not an error (decision D-38, D-42). A store full of invalid
// records must not halt the startup sequence: the records the repository owns
// are wrong, the binary is fine, and turning that into an error would make every
// later reading report itself as never taken.
type Finding struct {
	// Path is the record's repository-relative, forward-slash location
	// (tech-stack §74). Every finding names one, because a remedy that does not
	// name the offending file is unactionable where status prints it — status
	// drops Diagnostic and Impact and keeps only the next action.
	Path string `json:"path"`
	// ID is the id the record carries, omitted when the record has none. It is
	// the document's own spelling, which for step 6 is precisely the value under
	// dispute.
	ID   string `json:"id,omitempty"`
	Step Step   `json:"step"`
	// Code is KNOWLEDGE_INVALID for every step but 9, which is
	// KNOWLEDGE_SUPERSEDE_CYCLE (decision D-43). Neither is
	// KNOWLEDGE_UNREADABLE: that code means "this binary could not read the
	// record", and these all mean "this binary read it and it is wrong".
	Code    app.Code `json:"code"`
	Message string   `json:"message"`
	// Fatal is true only for step 9 (decision D-39). A cycle means no lineage
	// has an end, so every answer about which Decision is current is wrong;
	// every other finding costs the repository the records it names and no more.
	Fatal bool `json:"fatal"`
}

// ordered is a Finding plus the sort key decision D-47 names but the frozen
// Finding does not carry.
//
// D-47 sorts by (Path, Step, InstanceLocation, Message). InstanceLocation is a
// fact only step 5 has — it is the RFC 6901 pointer of the offending value
// inside the record — and adding it to Finding would widen a frozen wire shape
// to carry a sort key. Carrying it beside the finding until the sort has run
// costs one unexported type and keeps the published shape exactly as design §3
// froze it.
type ordered struct {
	finding  Finding
	instance string
}

// subject is one record the loader read, together with the fields the steps
// after 5 need out of its body.
//
// The body is decoded once, after step 5 has passed, rather than per step: six
// steps read the same handful of properties, and decoding per step would let
// two of them disagree about what the record says.
type subject struct {
	ref loader.RecordRef
	doc document
}

// document is the part of a record this pipeline reads.
//
// It is deliberately not record.Decision or record.Invariant. Those two are the
// writer's types and carry every property; the steps here need five fields that
// both kinds share, and decoding into a kind-specific struct would mean two
// decode paths and a switch on kind at every step. It is also not a second
// statement of the schema: nothing here is validated, because step 5 has already
// validated it against the shipped document, which is the contract (D-36).
type document struct {
	Status     string    `json:"status"`
	Supersedes []string  `json:"supersedes"`
	Scope      *scopeDoc `json:"scope"`
}

// scopeDoc mirrors the "scope" object both schema documents define in $defs.
// It is a pointer on document because a Decision's scope is optional, and
// "absent" and "present but empty" are different records.
type scopeDoc struct {
	Level  string `json:"level"`
	Target string `json:"target"`
}

// Check runs spec §95 steps 5-11 over one loaded store, in order, and returns
// every finding.
//
// It is a pure function of the store (decision D-41): it opens nothing. The
// result is sorted (decision D-47) and is empty but non-nil for a healthy store,
// so a caller can append to it and a marshalled report never says null.
//
// v must not be nil, and Check panics rather than accepting one. There is no
// honest alternative: returning no findings would publish "nobody looked" as "we
// looked and it is clean", and no registered code says "this binary could not
// build its own validator" — AC-08.2 makes that a defect in the binary that
// bootstrap reports as such, never a repository condition dressed up as a
// finding. A panic is the binary failing in its own voice instead of blaming the
// repository.
func Check(store loader.Store, v *schema.Validator) []Finding {
	if v == nil {
		panic("knowledge validate: Check needs a compiled schema validator; a nil one would report unvalidated records as clean")
	}

	// An absent store is healthy (decision D-06): a clean clone of a repository
	// that carries no knowledge records is not an invalid one. Nothing below
	// needs a special case for it, because a store with no records has no record
	// to judge — but a later step that wanted to say "the store is missing" would
	// have to add one, and TestCheckOverAnAbsentStoreFindsNothing is what refuses
	// it.
	records := readRecords(store)
	problems := problemPaths(store)

	found := make([]ordered, 0, 8)

	// Step 5, and the gate that decides who is asked the rest. A record whose
	// schema the document rejects is not asked steps 6-11, because those steps
	// read fields step 5 has just said are wrong; the records beside it are.
	judged := make([]*subject, 0, len(records))
	for i := range records {
		s := &records[i]

		if schemaFindings := stepSchema(s, v); len(schemaFindings) > 0 {
			found = append(found, schemaFindings...)
			continue
		}

		// Unreachable in practice: a document that satisfied its schema is an
		// object whose every property has the type this struct declares. It is
		// handled rather than ignored because the alternative is dropping a
		// record from steps 6-11 in silence, which is the same fabricated pass
		// the nil-validator panic above refuses.
		if err := json.Unmarshal(s.ref.Body, &s.doc); err != nil {
			found = append(found, ordered{finding: Finding{
				Path:    s.ref.Path,
				ID:      s.ref.ID,
				Step:    StepSchema,
				Code:    app.CodeKnowledgeInvalid,
				Message: fmt.Sprintf("%s satisfies its schema but could not be decoded: %v", s.ref.Path, err),
			}})
			continue
		}

		judged = append(judged, s)
	}

	// The lineage graph every cross-record step reads. It is built once, from
	// the records that were read and passed step 5, and its edges point only at
	// ids that resolve to such a record — which is decision D-45 expressed as a
	// data structure rather than as a condition each step has to remember.
	lineages := newGraph(judged)

	found = append(found, stepFilenameConsistency(judged)...)
	found = append(found, stepUniqueID(judged)...)
	found = append(found, stepSupersedeTarget(judged, lineages, problems)...)
	found = append(found, stepSupersedeCycle(lineages)...)
	found = append(found, stepDuplicateActiveLineage(lineages)...)
	found = append(found, stepScopeSyntax(judged)...)

	return sorted(found)
}

// readRecords flattens the store's two buckets into the order the loader walked
// them: decisions then invariants, each already sorted by file name because
// os.ReadDir sorts. Ordering here is not the output contract — sorted() is — but
// a deterministic input is what keeps the stable sort's tie-breaking stable.
func readRecords(store loader.Store) []subject {
	records := make([]subject, 0, store.Count())
	for _, ref := range store.Decisions {
		records = append(records, subject{ref: ref})
	}
	for _, ref := range store.Invariants {
		records = append(records, subject{ref: ref})
	}
	return records
}

// problemPaths indexes the files the loader could not read.
//
// It is the input to decision D-45's suppression: a referenced id whose expected
// file is in this set is unverifiable, because the record exists and this binary
// could not read it. The loader spells Problem.Path repo-relative with forward
// slashes, which is the same spelling expectedPath builds, so the two are
// directly comparable.
func problemPaths(store loader.Store) map[string]struct{} {
	paths := make(map[string]struct{}, len(store.Problems))
	for _, problem := range store.Problems {
		paths[problem.Path] = struct{}{}
	}
	return paths
}

// sorted applies decision D-47 and drops the sort key.
//
// jsonschema/v6 returns a ValidationError's Causes in Go map-iteration order —
// five consecutive Validate calls on one schema and one instance in one process
// were observed to produce five different leaf orders — so an unsorted result
// would differ between two runs over the same bytes. tech-stack §2.2 requires a
// deterministic core, and a report that reorders itself is one no consumer can
// diff.
//
// The sort is stable so that two findings agreeing on all four keys keep the
// order they were produced in, which is itself deterministic.
func sorted(found []ordered) []Finding {
	slices.SortStableFunc(found, func(a, b ordered) int {
		return cmp.Or(
			cmp.Compare(a.finding.Path, b.finding.Path),
			cmp.Compare(a.finding.Step, b.finding.Step),
			cmp.Compare(a.instance, b.instance),
			cmp.Compare(a.finding.Message, b.finding.Message),
		)
	})

	findings := make([]Finding, 0, len(found))
	for _, item := range found {
		findings = append(findings, item.finding)
	}
	return findings
}
