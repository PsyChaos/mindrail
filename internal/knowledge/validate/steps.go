package validate

import (
	"fmt"
	"slices"
	"strings"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/knowledge/record"
	"github.com/PsyChaos/mindrail/internal/knowledge/schema"
)

// maxNamedRecords bounds how many sibling records one message lists by name.
//
// It is internal/doctor's recordRemedies cap, for the same reason and with the
// same "the remaining N" tail: a store with more broken records than this has a
// systemic problem, and a next_action longer than a screen is one nobody reads.
//
// It is also what keeps three steps linear. Steps 7, 9 and 10 each attach a
// message to every member of a group and name that member's siblings in it, so
// an uncapped list costs O(n) bytes per record and O(n^2) for the group. That is
// not a theoretical bound: 1000 records in one lineage produced 56 MB of message
// text and 2000 produced 224 MB, against a 150 ms p95 SLO for `mindrail status`.
const maxNamedRecords = 10

// nameList renders a group's members for a message: the first maxNamedRecords of
// them, and a count of the rest.
//
// total is passed rather than taken from len(shown) because a caller that has
// already bounded its slice — step 7 walks a sorted list and stops early — still
// has to say how many records it did not name. A message that silently listed
// ten of a hundred would be worse than one that listed all hundred.
func nameList(shown []string, total int) string {
	if len(shown) > maxNamedRecords {
		shown = shown[:maxNamedRecords]
	}
	if remaining := total - len(shown); remaining > 0 {
		return fmt.Sprintf("%s and the remaining %d", strings.Join(shown, ", "), remaining)
	}
	return strings.Join(shown, ", ")
}

// invalid builds the finding every step but 9 produces. Sharing the constructor
// is what keeps the code and the fatality of a finding a property of its step
// rather than of whoever wrote the step: decision D-39 says only a supersede
// cycle is fatal, and there is exactly one place in this package that can set
// Fatal to true.
func invalid(s *subject, step Step, message string) ordered {
	return ordered{finding: Finding{
		Path:    s.ref.Path,
		ID:      s.ref.ID,
		Step:    step,
		Code:    app.CodeKnowledgeInvalid,
		Message: message,
		Fatal:   false,
	}}
}

// stepSchema is spec §95 step 5: the record is evaluated against the shipped
// JSON Schema document for its kind and version.
//
// The rules are not restated in Go (decision D-36). The embedded documents are
// the contract, and a second implementation of pattern, const, uniqueItems and
// additionalProperties:false would drift from the JSON until two things claimed
// to be the rule. Everything this function does is turn one schema.Finding into
// one Finding by adding the three facts the schema package cannot know: which
// file the bytes came from, which id it carries, and which step asked.
//
// The record's kind comes from the directory it was found in (loader.go's
// kindDirs, and TestLoadKindComesFromTheDirectory), so validating against that
// kind's document is also what reconciles the document's own "kind" field with
// its location: both shipped documents pin "kind" with a const, so a record
// filed in decisions/ that calls itself an invariant fails here. That is step
// 6's kind half, discharged by the contract rather than by a second rule.
func stepSchema(s *subject, v *schema.Validator) []ordered {
	kind := schema.RecordKind(s.ref.Kind)
	document := schema.SchemaNameFor(kind, s.ref.SchemaVersion)

	violations := v.Validate(kind, s.ref.SchemaVersion, s.ref.Body)
	found := make([]ordered, 0, len(violations))
	for _, violation := range violations {
		found = append(found, ordered{
			// Carried, not published: decision D-47 sorts on the instance
			// location, and step 5 is the only step that has one.
			instance: violation.InstanceLocation,
			finding: Finding{
				Path: s.ref.Path,
				ID:   s.ref.ID,
				Step: StepSchema,
				Code: app.CodeKnowledgeInvalid,
				Message: fmt.Sprintf("%s breaks the %q rule of %s: %s",
					s.ref.Path, violation.Keyword, document, schemaClause(violation, document)),
				Fatal: false,
			},
		})
	}
	return found
}

// keywordPattern and timestampPointer name the one violation whose library
// wording states a rule the validator did not apply. They are spelled here
// rather than inline so the condition reads as the thing it is.
const (
	keywordPattern   = "pattern"
	timestampPointer = "/created_at"
)

// schemaClause is the violation's own wording, except where reproducing it would
// misquote the rule.
//
// Every other message in this package's step 5 is the library's, deliberately:
// decision D-36 makes the shipped document the contract and a second Go-language
// statement of any keyword would drift from it. That argument holds only while
// the library's sentence is true, and for one keyword it is not.
//
// The "pattern" message renders the regular expression through Go's %q, so a
// pattern containing a backslash is published with that backslash doubled. The
// created_at pattern contains "(\.[0-9]{1,9})?" and prints as
// "(\\.[0-9]{1,9})?", which is a different expression: it asks for a literal
// backslash. Measured — a record whose created_at is "…T00:00:00.1Z" is accepted
// by the validator and refused by the rule the message prints. A reader who
// pastes it into a tester is debugging a rule nothing enforces.
//
// The remedy is to stop reproducing the expression rather than to restate it.
// Restating it in prose would be the second implementation D-36 forbids, and
// re-deriving the true regex from the quoted one would be a second
// implementation of the library's quoting. The property's own "description" is
// part of the contract the reader can already open, so the message names it —
// TestTheCreatedAtRuleTheMessageSendsTheReaderToIsThere is what keeps that
// pointer from dangling.
//
// Narrow on purpose. The other two patterns both documents carry — the id and
// the supersedes items, "^DEC-[0-9]{4,}$" and its invariant twin — contain no
// character %q escapes, so the library prints them faithfully and they are the
// most useful sentence a reader could get. Suppressing every pattern message
// would trade one misquote for a whole class of lost facts.
func schemaClause(violation schema.Finding, document string) string {
	if violation.Keyword != keywordPattern || violation.InstanceLocation != timestampPointer {
		return violation.Message
	}
	return fmt.Sprintf(
		"at %q: the timestamp is not spelled the way this project writes one. "+
			"The rule is the %q property's own %q in %s, which is the contract; "+
			"the expression itself is not reproduced here, because a JSON Schema pattern "+
			"rendered into a Go string literal is not the expression the validator applied",
		timestampPointer, "created_at", "description", document)
}

// stepFilenameConsistency is spec §95 step 6: the file .../decisions/X.json has
// to carry id X.
//
// Only the id half is decided here. The kind half is decided by step 5, because
// the loader takes a record's kind from the directory it was walked in and step
// 5 validates it against that kind's document, whose "kind" property is a const
// — so a record whose own "kind" disagrees with its directory has already
// failed. Re-checking it here would be the second implementation decision D-36
// forbids, and it would be unreachable besides: D-40 does not ask step 6 about a
// record step 5 rejected.
func stepFilenameConsistency(subjects []*subject) []ordered {
	found := make([]ordered, 0, 1)
	for _, s := range subjects {
		named := idFromPath(s.ref.Path)
		if s.ref.ID == named {
			continue
		}
		found = append(found, invalid(s, StepFilenameConsistency, fmt.Sprintf(
			"%s carries id %q, but its file name says %q; rename the file or change the id so the two agree",
			s.ref.Path, s.ref.ID, named)))
	}
	return found
}

// stepUniqueID is spec §95 step 7: no two records share an id.
//
// This step is exempt from decision D-45's suppression, and the exemption is not
// an oversight. The other cross-record steps ask whether a record the loader
// could not read would change the verdict; here it cannot. Two files that were
// both read and both carry one id are a duplicate, and no unreadable third file
// can make that false.
//
// Every record in the group is reported, each naming the others, because the
// remedy is a choice between files and a reader who was shown only one of them
// cannot make it.
func stepUniqueID(subjects []*subject) []ordered {
	byID := make(map[string][]*subject, len(subjects))
	order := make([]string, 0, len(subjects))
	for _, s := range subjects {
		if _, seen := byID[s.ref.ID]; !seen {
			order = append(order, s.ref.ID)
		}
		byID[s.ref.ID] = append(byID[s.ref.ID], s)
	}

	found := make([]ordered, 0, 1)
	for _, id := range order {
		group := byID[id]
		if len(group) < 2 {
			continue
		}

		paths := make([]string, 0, len(group))
		for _, s := range group {
			paths = append(paths, s.ref.Path)
		}
		slices.Sort(paths)

		for _, s := range group {
			// Bounded rather than filtered: paths is sorted, so taking the first
			// maxNamedRecords entries that are not this record's own is the same
			// prefix the full list would have produced, at a cost that does not
			// grow with the group. A hundred files under one id would otherwise
			// cost a hundred messages of a hundred paths each.
			others := make([]string, 0, maxNamedRecords)
			for _, other := range paths {
				if other == s.ref.Path {
					continue
				}
				others = append(others, other)
				if len(others) == maxNamedRecords {
					break
				}
			}
			found = append(found, invalid(s, StepUniqueID, fmt.Sprintf(
				"%s carries id %q, which is also carried by %s; an id names one record",
				s.ref.Path, id, nameList(others, len(paths)-1))))
		}
	}
	return found
}

// stepSupersedeTarget is spec §95 step 8: every id in "supersedes" names a
// record that exists.
//
// Decision D-45 lives here in its plainest form, and this step publishes "there
// is no such record" only when nothing in this run knows of the file:
//
//   - The id resolves to a record the graph holds. That record was read. It may
//     also have failed step 5, and then the same report names it in a step-5
//     finding — which is the report telling the reader to correct a file it has
//     just called absent. This is the second half of MR-002's root defect, and
//     it is why the graph holds every record read rather than every record that
//     survived step 5.
//   - The file the id would occupy is one the loader recorded a Problem for. The
//     record exists and this binary could not read it; the loader has already
//     named the file in its own voice.
//   - The file the id would occupy was read, and the record in it carries no id
//     this run could read — its "id" property is missing or is not a string. The
//     file is there and the same report carries a step-5 finding against it.
//
// The three conditions are not redundant. A record carrying id DEC-0001 may live
// at decisions/DEC-0009.json — step 6's condition — so it resolves without its
// expected path having been read; and a record at decisions/DEC-0001.json whose
// own id property is missing carries no id at all, so its file was read without
// anything resolving. Dropping either check publishes "it is not there" about a
// file this run has in its hands.
//
// # The read file that carries someone else's id
//
// There is a fourth shape, and it is a finding rather than a suppression: the
// expected file was read, its id was readable, and it is a different id. Nothing
// in the store carries the referenced id, so the reference is dangling — and
// telling the reader only "rename DEC-0009.json" (step 6) leaves them to work
// out on their own why the supersede they wrote resolves to nothing.
//
// Collapsing this case into the suppression is how the diagnosis was lost once:
// widening the set to "every file this run named" made a supersede pointing at
// DEC-0009 silent merely because a file called DEC-0009.json had been read, even
// when that file declares itself DEC-0007. The suppression's whole warrant is
// that saying "it is not there" about a file this run holds is a fabricated
// claim — so the fix is not silence, it is a message that says what this run
// actually found: the file exists and carries another id.
//
// The expected file for id X of kind k is exactly what step 6 requires, so the
// mapping is not a guess.
func stepSupersedeTarget(subjects []*subject, lineages *graph, named map[string]account) []ordered {
	found := make([]ordered, 0, 1)
	for _, s := range subjects {
		for _, target := range s.doc.Supersedes {
			if lineages.resolves(node{kind: s.ref.Kind, id: target}) {
				continue
			}

			expected, placeable := expectedPath(s.ref.Kind, target)
			if !placeable {
				// A kind with no directory cannot be turned into a file name, so
				// there is no file to say anything about. Saying nothing is the
				// direction D-45 points.
				continue
			}

			accounted, known := named[expected]
			switch {
			case known && accounted.problem:
				// D-45 in its plainest form: the record exists and this binary
				// could not read it.
				continue
			case known && accounted.read && accounted.id == "":
				// The file was read and its own id is unreadable, so nothing could
				// have resolved. Step 5 names it one line above.
				continue
			case known && accounted.read:
				found = append(found, invalid(s, StepSupersedeTarget, fmt.Sprintf(
					"%s supersedes %q, but no record in this store carries that id: %s was read and carries id %q instead",
					s.ref.Path, target, expected, accounted.id)))
			default:
				found = append(found, invalid(s, StepSupersedeTarget, fmt.Sprintf(
					"%s supersedes %q, but this store has no such record: %s is not there",
					s.ref.Path, target, expected)))
			}
		}
	}
	return found
}

// stepSupersedeCycle is spec §95 step 9: the supersede relation is a DAG.
//
// This is the only fatal finding (decision D-39). A cycle means no lineage has
// an end, so "which Decision is current" has no answer at all — not a wrong one,
// none — and every consumer that walks the lineage either loops or picks
// arbitrarily. One malformed record costs the repository that record; a cycle
// costs it the question.
//
// Decision D-45 is satisfied structurally rather than by a condition: the graph
// only has edges between records that were read, so a chain through an id the
// loader could not read has no way back and closes nothing.
//
// The group is a strongly connected component, not a walk's back edge, and every
// member of it is named. A record that breaks its schema is a member like any
// other, because D-39's reason — no lineage has an end, so every answer about
// which record is current is wrong — does not become less true when a member
// also has an unknown property. What such a member does not get is a finding
// against its own path: decision D-40 did not ask it step 9. A group whose every
// member failed step 5 therefore reports nothing here, and the reader is told to
// correct those records first; the cycle surfaces the moment they are readable
// against the schema. That boundary is D-40's, not this step's, and
// TestAClosedGroupOfOnlyInvalidRecordsIsLeftToStepFive is where it is recorded.
//
// Every reported member carries the same member list, because breaking the group
// means editing one of them and the reader has to see which ones are on the
// table. The list is built once per group rather than once per member: it is the
// same string for all of them, and rebuilding it inside the loop is O(n^2) bytes
// for a lineage this project has already measured at 224 MB.
//
// The message says every member "is reachable from every other by following
// supersedes", and that sentence is only true while a node's edges come from the
// records the reader is being pointed at. It was false once: a graph that unioned
// a rejected draft's "supersedes" into a valid record's node reported a fatal
// cycle against a file with no "supersedes" property at all, with a remedy its
// bytes could not carry out. newGraph is where that is now prevented, and
// TestACycleMessageIsTrueOfEveryFileItIsAttachedTo is what holds the claim.
//
// One narrower case remains, deliberately and with its own row in
// TestARejectedRecordIsStillTheOnlyAccountOfAnIdNobodyElseCarries: an id whose
// only claimant is a record step 5 rejected and whose file name is something
// else. That id is named in the member list and no finding is attached to any
// file carrying it, so a reader looking for its file will not find one — they
// are pointed at the rejected record by step 5 instead. Naming the declaring
// file in this message would close that, at the cost of a clause on every step-9
// finding; it is left to a milestone that can measure the message-size tradeoff
// rather than folded into a fix pass.
func stepSupersedeCycle(lineages *graph) []ordered {
	found := make([]ordered, 0, 1)
	for _, group := range lineages.closedGroups() {
		members := nameList(ids(group), len(group))
		for _, key := range group {
			for _, s := range lineages.askedFor(key) {
				found = append(found, ordered{finding: Finding{
					Path: s.ref.Path,
					ID:   s.ref.ID,
					Step: StepSupersedeCycle,
					Code: app.CodeKnowledgeSupersedeCycle,
					Message: fmt.Sprintf(
						"%s is part of a closed supersede group of %d records: %s. Each of them is reachable from every other by following \"supersedes\", so no lineage among them has a newest record and no answer about which record is current is correct",
						s.ref.Path, len(group), members),
					Fatal: true,
				}})
			}
		}
	}
	return found
}

// stepDuplicateActiveLineage is spec §95 step 10: one supersede lineage has one
// current record.
//
// A lineage is a weakly connected component of the supersede graph: two records
// that both supersede a third are in one lineage even though neither reaches the
// other, and that fork is exactly what this step exists to find.
//
// Decision D-45 is satisfied by the graph's edges again. A lineage whose
// connecting record the loader could not read is two lineages here, so the
// members on either side of the unreadable record are not reported as duplicate
// actives — the record that would have joined them is precisely the one this
// binary cannot see.
//
// Activity is counted per id rather than per file. Two files carrying one id are
// step 7's condition and step 7 names both; counting them here as well would
// give one condition two diagnoses.
//
// A record step 5 rejected joins the lineage but is never counted active and
// never reported. Its "supersedes" is what puts two records in one lineage, and
// that relation holds whatever else is wrong with it — but "active" is read out
// of its "status", which is one of the fields step 5 may have just said is
// wrong, and decision D-40 did not ask it step 10 either way.
//
// The member list is built once per lineage rather than once per member, and
// capped. It is the same string for every finding in the group, and building it
// inside the loop is what made 1000 records in one lineage cost 56 MB of message
// text and 2000 cost 224 MB — quadratic, against a 150 ms p95 SLO.
func stepDuplicateActiveLineage(lineages *graph) []ordered {
	found := make([]ordered, 0, 1)
	for _, members := range lineages.lineages() {
		active := make([]*subject, 0, len(members))
		activeIDs := 0
		for _, key := range members {
			carrying := activeSubjects(lineages.askedFor(key))
			if len(carrying) == 0 {
				continue
			}
			activeIDs++
			active = append(active, carrying...)
		}
		if activeIDs < 2 {
			continue
		}

		named := make([]string, 0, len(active))
		for _, s := range active {
			named = append(named, fmt.Sprintf("%s (%s)", s.ref.Path, s.ref.ID))
		}
		slices.Sort(named)
		listed := nameList(named, len(named))

		for _, s := range active {
			found = append(found, invalid(s, StepDuplicateActiveLineage, fmt.Sprintf(
				"%s is one of %d active records in a single supersede lineage: %s. A lineage has one current record and the rest carry status %q",
				s.ref.Path, len(named), listed, record.StatusSuperseded)))
		}
	}
	return found
}

// activeSubjects returns the records under one id whose status is active.
func activeSubjects(carrying []*subject) []*subject {
	active := make([]*subject, 0, len(carrying))
	for _, s := range carrying {
		if s.doc.Status == string(record.StatusActive) {
			active = append(active, s)
		}
	}
	return active
}

// stepScopeSyntax is spec §95 step 11: a scope's level is one of the five, its
// target is present for every level but PROJECT, and the target is the
// repository-relative forward-slash spelling tech-stack §74 mandates.
//
// Syntax only, and the message says so. Whether the target names something that
// exists is step 12, which needs the structural index MR-005 builds; a
// syntactically valid target pointing at a file that is not there is not a
// finding here, and inventing one would reject records this milestone has no
// index to judge.
//
// The rule is asked of record.Scope rather than restated. Two implementations of
// one rule is the drift decision D-36 exists to prevent, and unlike every other
// rule in this package there is no schema document to fall back on: the
// documents constrain the level's enum and the target's length, but nothing in
// JSON Schema can say "repository-relative forward slashes". A writer and a
// validator disagreeing about what a scope is would be exactly the defect design
// §5's pairing looks for.
func stepScopeSyntax(subjects []*subject) []ordered {
	found := make([]ordered, 0, 1)
	for _, s := range subjects {
		if s.doc.Scope == nil {
			// A Decision's scope is optional. An Invariant's is required, and its
			// absence is its schema's business (step 5), not this step's.
			continue
		}

		scope := record.Scope{
			Level:  record.ScopeLevel(s.doc.Scope.Level),
			Target: s.doc.Scope.Target,
		}
		if err := scope.Validate(); err != nil {
			found = append(found, invalid(s, StepScopeSyntax, fmt.Sprintf(
				"%s has a scope this pipeline cannot accept: %v. Step 11 judges the scope's spelling only — whether the target names something that exists is step 12, which needs the structural index MR-005 builds",
				s.ref.Path, err)))
		}
	}
	return found
}
