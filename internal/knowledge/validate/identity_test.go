package validate_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/PsyChaos/mindrail/internal/knowledge/validate"
)

// This file is about node identity in the supersede graph: which record gets to
// say what a given id supersedes, and which record a verdict about that id may
// name.
//
// lineage_test.go answers the neighbouring question — which records are in the
// graph at all — and its answer is "every record the loader read". That answer
// is right and this file does not narrow it. What it narrows is whose "id" the
// graph believes when two files claim one.
//
// Three audit rounds each flipped that seam and each was answered with a
// property of the record's *contents*: "step 5 accepted it", then "step 5
// accepted it and nobody else was accepted". Round 1 lost a fatal cycle to an
// unknown JSON property, round 2 manufactured one against innocent files, round 3
// lost one again to a schema-valid record filed under the wrong name. Decision
// D-52 replaces all three with a property of where the record sits: a node is
// owned by the record filed at the path its id names, and ownership governs both
// who supplies edges and who may be named in a verdict.
//
// Every test here therefore comes in a pair. The over-fire arm is a healthy store
// with one stray file in it, which must stay quiet; the under-fire arm is a store
// whose cycle is real and must still be fatal. A rule that gets one of those
// right and the other wrong is the shape MR-002 produced three times, and
// checking a fix in one direction only is how round 2 shipped an over-fire while
// closing a fail-open.

// TestADraftCarryingAValidRecordsIdDoesNotManufactureAFatalCycle is the
// regression for the audit's one HIGH finding.
//
// The store is two correct decisions in a straight lineage. Adding one draft
// that fails step 5 and happens to carry DEC-0001's id turned the repository
// BLOCKED with a KNOWLEDGE_SUPERSEDE_CYCLE naming both correct files — neither
// of which is on any cycle. The message asserted that each member "is reachable
// from every other by following supersedes", which was false of DEC-0001.json:
// it has no "supersedes" property at all. The remedy told the reader to drop the
// superseded id from that file, an instruction its bytes cannot carry out. The
// draft that created the edge was named nowhere.
//
// AC-03.6 — "every finding costs the repository the records it names and no
// more" — is the clause this broke, in the one place decision D-39 made fatal.
func TestADraftCarryingAValidRecordsIdDoesNotManufactureAFatalCycle(t *testing.T) {
	validator := shippedValidator(t)

	healthy := []testRecord{
		decisionAt("DEC-0001.json", decisionDoc("DEC-0001", map[string]any{
			"status": "superseded",
		})),
		decisionAt("DEC-0002.json", decisionDoc("DEC-0002", map[string]any{
			"supersedes": []any{"DEC-0001"},
		})),
	}

	// Control: the store without the draft is silent. Without this the scenario
	// below would pass over a store that was never healthy to begin with.
	if control := validate.Check(storeOf(t, healthy), validator); len(control) != 0 {
		t.Fatalf("the store without the draft is not healthy:\n\t%s", describe(control))
	}

	// Scenario: byte-identical, plus one draft. It fails step 5 on an unknown
	// property, claims DEC-0001's id, and its "supersedes" points back at
	// DEC-0002 — which is what closed the walk over the fused node.
	scenario := validate.Check(storeOf(t, append(slices.Clone(healthy),
		decisionAt("DEC-0003.json", decisionDoc("DEC-0001", map[string]any{
			"supersedes": []any{"DEC-0002"},
			"titel":      "oops",
		})),
	)), validator)

	if got := findingsAt(scenario, validate.StepSupersedeCycle); len(got) != 0 {
		t.Errorf("a draft manufactured a supersede cycle against records that are not on one:\n\t%s",
			describe(got))
	}
	for _, finding := range scenario {
		if finding.Fatal {
			t.Errorf("a draft made the repository BLOCKED: %s: %q", finding.Path, finding.Message)
		}
		if finding.Path != ".mindrail/knowledge/decisions/DEC-0003.json" {
			t.Errorf("a finding names a file the draft did not break: %s: %q", finding.Path, finding.Message)
		}
	}
	// And the draft is still reported, or the store would be silently wrong
	// rather than loudly right.
	if got := stepsAt(scenario, ".mindrail/knowledge/decisions/DEC-0003.json"); !slices.Equal(got, []validate.Step{validate.StepSchema}) {
		t.Errorf("the draft reported steps %v, want [5]\n\t%s", got, describe(scenario))
	}
}

// TestADraftCarryingAValidRecordsIdDoesNotMergeTwoLineages is the same defect
// reached through step 10 rather than step 9, and it is here because a fix
// aimed only at the fatal finding would leave this one standing.
//
// DEC-0001 and DEC-0002 are two unrelated lineages of one record each, each with
// its own current record — which is exactly what step 10 exists to permit. The
// draft claims DEC-0001's id and supersedes DEC-0002, and a graph that unions
// its edges into DEC-0001's node joins the two into one lineage with two active
// records, reported against both correct files.
func TestADraftCarryingAValidRecordsIdDoesNotMergeTwoLineages(t *testing.T) {
	validator := shippedValidator(t)

	healthy := []testRecord{
		decisionAt("DEC-0001.json", decisionDoc("DEC-0001", nil)),
		decisionAt("DEC-0002.json", decisionDoc("DEC-0002", nil)),
	}
	if control := validate.Check(storeOf(t, healthy), validator); len(control) != 0 {
		t.Fatalf("two unrelated one-record lineages are not healthy:\n\t%s", describe(control))
	}

	scenario := validate.Check(storeOf(t, append(slices.Clone(healthy),
		decisionAt("DEC-0009.json", decisionDoc("DEC-0001", map[string]any{
			"supersedes": []any{"DEC-0002"},
			"titel":      "oops",
		})),
	)), validator)

	if got := findingsAt(scenario, validate.StepDuplicateActiveLineage); len(got) != 0 {
		t.Errorf("a draft joined two unrelated lineages:\n\t%s", describe(got))
	}
	if got := stepsAt(scenario, ".mindrail/knowledge/decisions/DEC-0009.json"); !slices.Equal(got, []validate.Step{validate.StepSchema}) {
		t.Errorf("the draft reported steps %v, want [5]\n\t%s", got, describe(scenario))
	}
}

// TestARejectedRecordIsStillTheOnlyAccountOfAnIdNobodyElseCarries is the
// under-fire arm, and it is the one every fix in this seam could have broken.
//
// The rule is not "a rejected record's supersedes never counts" — that is the
// round-1 fail-open, where one unknown property on one member deleted a fatal
// cycle. Decision D-52 rule 1 is that a node's edges come from the record filed
// at the path its id names, *whether or not step 5 accepted it*: a schema
// violation elsewhere in the document does not make that record's "supersedes"
// a lie.
//
// Both rows below close a walk with an edge declared by a record step 5
// rejected, and both must stay fatal. The second is the sharper one — the
// rejected record is the middle of a three-record chain, so no other file in the
// store could supply the edge that closes it.
func TestARejectedRecordIsStillTheOnlyAccountOfAnIdNobodyElseCarries(t *testing.T) {
	validator := shippedValidator(t)

	for _, tt := range []struct {
		name      string
		records   []testRecord
		wantCycle []string
	}{
		{
			// The round-1 case, restated as a rule about identity: DEC-0001 is
			// carried by exactly one file, which is the file its id names, and
			// which fails step 5 on a property no step reads.
			name: "the rejected record is the file its id names",
			records: []testRecord{
				decisionAt("DEC-0001.json", decisionDoc("DEC-0001", map[string]any{
					"supersedes": []any{"DEC-0002"},
					"note":       "junk",
				})),
				decisionAt("DEC-0002.json", decisionDoc("DEC-0002", map[string]any{
					"status":     "superseded",
					"supersedes": []any{"DEC-0001"},
				})),
			},
			// DEC-0001.json failed step 5, so D-40 did not ask it step 9.
			wantCycle: []string{".mindrail/knowledge/decisions/DEC-0002.json"},
		},
		{
			// The rejected record is a middle member rather than an endpoint.
			// DEC-0002's edge to DEC-0003 is declared nowhere else in the store,
			// so dropping it on the strength of an unknown property leaves the
			// remaining two records in an open chain and the cycle unreported.
			name: "the rejected record is the middle of the chain",
			records: []testRecord{
				decisionAt("DEC-0001.json", decisionDoc("DEC-0001", map[string]any{
					"status":     "superseded",
					"supersedes": []any{"DEC-0002"},
				})),
				decisionAt("DEC-0002.json", decisionDoc("DEC-0002", map[string]any{
					"status":     "superseded",
					"supersedes": []any{"DEC-0003"},
					"note":       "junk",
				})),
				decisionAt("DEC-0003.json", decisionDoc("DEC-0003", map[string]any{
					"status":     "superseded",
					"supersedes": []any{"DEC-0001"},
				})),
			},
			wantCycle: []string{
				".mindrail/knowledge/decisions/DEC-0001.json",
				".mindrail/knowledge/decisions/DEC-0003.json",
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			findings := validate.Check(storeOf(t, tt.records), validator)

			if got := pathsAt(findings, validate.StepSupersedeCycle); !slices.Equal(got, tt.wantCycle) {
				t.Errorf("step 9 reported %v, want %v\n\t%s", got, tt.wantCycle, describe(findings))
			}
			// The row proves nothing unless the closing edge really did come out
			// of a record step 5 rejected.
			if len(findingsAt(findings, validate.StepSchema)) == 0 {
				t.Errorf("no record in this row failed step 5, so it guards nothing:\n\t%s",
					describe(findings))
			}
		})
	}
}

// TestAnIdNoFileIsNamedAfterSuppliesNoEdges is decision D-52 rule 3.
//
// Where no record sits at the path an id names, nothing in the store establishes
// which record that id is — so no cycle verdict may be built on it, not from the
// union of its claimants' edges and not from whichever claimant looks best. Both
// earlier rules said the opposite in one direction or the other, and both were
// wrong: the union manufactured a fatal cycle against innocent files (round 2),
// and picking a claimant deleted a real one (round 3).
//
// The final check in each row is the one that keeps this honest. Rule 3 buys its
// safety with a round trip, not with silence: the reader is always told
// something about the misfiled files, and the cycle follows once they act. A row
// that reported nothing at all would be a fail-open wearing this rule's name.
func TestAnIdNoFileIsNamedAfterSuppliesNoEdges(t *testing.T) {
	validator := shippedValidator(t)

	for _, tt := range []struct {
		name    string
		records []testRecord
	}{
		{
			// Round 3's shape without its voucher: the only claimant of DEC-0001
			// is a rejected record filed as DEC-0009.json. Step 5 names that file.
			name: "the only claimant is a rejected record under another name",
			records: []testRecord{
				decisionAt("DEC-0002.json", decisionDoc("DEC-0002", map[string]any{
					"status":     "superseded",
					"supersedes": []any{"DEC-0001"},
				})),
				decisionAt("DEC-0009.json", decisionDoc("DEC-0001", map[string]any{
					"supersedes": []any{"DEC-0002"},
					"note":       "junk",
				})),
			},
		},
		{
			// Two claimants and still no file named DEC-0001.json. Under the
			// previous rule neither outranked the other, so both supplied edges
			// and the union closed a walk over a node no file owns.
			name: "two rejected claimants, neither filed under the id",
			records: []testRecord{
				decisionAt("DEC-0002.json", decisionDoc("DEC-0002", map[string]any{
					"status":     "superseded",
					"supersedes": []any{"DEC-0001"},
				})),
				decisionAt("DEC-0008.json", decisionDoc("DEC-0001", map[string]any{
					"supersedes": []any{"DEC-0002"},
					"note":       "junk",
				})),
				decisionAt("DEC-0009.json", decisionDoc("DEC-0001", map[string]any{
					"supersedes": []any{"DEC-0002"},
					"note":       "junk",
				})),
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			findings := validate.Check(storeOf(t, tt.records), validator)

			if got := findingsAt(findings, validate.StepSupersedeCycle); len(got) != 0 {
				t.Errorf("a cycle was built over an id no file is named after:\n\t%s", describe(got))
			}
			for _, finding := range findings {
				if finding.Fatal {
					t.Errorf("an id no file is named after made the repository BLOCKED: %s: %q",
						finding.Path, finding.Message)
				}
			}
			if len(findings) == 0 {
				t.Errorf("this store is reported as clean, so rule 3 bought silence rather than a round trip:\n\t%s",
					describe(findings))
			}
		})
	}
}

// TestTwoMisfiledRecordsReportTheirCycleOnceTheyAreRenamed is the second of the
// two stores decision D-52 names, and it states rule 3's cost end to end.
//
// A genuine cycle between two records that are both filed under the wrong name
// is reported in two steps rather than one. Asserting only the first step would
// let a later reader take the exit-0 report as a clean bill of health, which is
// the fail-open this whole seam keeps producing — so the second half of this
// test renames the files and requires the cycle to be fatal.
func TestTwoMisfiledRecordsReportTheirCycleOnceTheyAreRenamed(t *testing.T) {
	validator := shippedValidator(t)

	misfiled := validate.Check(storeOf(t, []testRecord{
		decisionAt("DEC-0008.json", decisionDoc("DEC-0001", map[string]any{
			"status":     "superseded",
			"supersedes": []any{"DEC-0002"},
		})),
		decisionAt("DEC-0009.json", decisionDoc("DEC-0002", map[string]any{
			"status":     "superseded",
			"supersedes": []any{"DEC-0001"},
		})),
	}), validator)

	wantMisfiled := []string{
		".mindrail/knowledge/decisions/DEC-0008.json",
		".mindrail/knowledge/decisions/DEC-0009.json",
	}
	if got := pathsAt(misfiled, validate.StepFilenameConsistency); !slices.Equal(got, wantMisfiled) {
		t.Errorf("step 6 reported %v, want both misfiled files\n\t%s", got, describe(misfiled))
	}
	if got := pathsAt(misfiled, validate.StepFilenameConsistency); len(got) != len(misfiled) {
		t.Errorf("the misfiled store reported something other than its two step-6 findings:\n\t%s",
			describe(misfiled))
	}
	for _, finding := range misfiled {
		if finding.Fatal {
			t.Errorf("two misfiled records made the repository BLOCKED before anything named them: %s: %q",
				finding.Path, finding.Message)
		}
	}

	// The reader performs the remedy step 6 printed — rename each file to the id
	// it carries — and the same two records now say what they always said.
	renamed := validate.Check(storeOf(t, []testRecord{
		decisionAt("DEC-0001.json", decisionDoc("DEC-0001", map[string]any{
			"status":     "superseded",
			"supersedes": []any{"DEC-0002"},
		})),
		decisionAt("DEC-0002.json", decisionDoc("DEC-0002", map[string]any{
			"status":     "superseded",
			"supersedes": []any{"DEC-0001"},
		})),
	}), validator)

	wantRenamed := []string{
		".mindrail/knowledge/decisions/DEC-0001.json",
		".mindrail/knowledge/decisions/DEC-0002.json",
	}
	if got := pathsAt(renamed, validate.StepSupersedeCycle); !slices.Equal(got, wantRenamed) {
		t.Errorf("after the rename step 9 reported %v, want both records\n\t%s",
			got, describe(renamed))
	}
	for _, finding := range findingsAt(renamed, validate.StepSupersedeCycle) {
		if !finding.Fatal {
			t.Errorf("the cycle surfaced without being fatal: %s: %q", finding.Path, finding.Message)
		}
	}
}

// TestRoundThreesMisfiledVoucherDoesNotDeleteTheCycle is the first of the two
// stores decision D-52 names, and it is the HIGH finding of the third audit
// round stated as a test.
//
// The store is a real two-record cycle whose DEC-0001.json fails step 5, plus
// DEC-0009.json — schema-valid, carrying id DEC-0001, declaring no "supersedes"
// at all. Under the previous rule that third file "vouched" for DEC-0001, which
// deleted the rejected record's edge and turned a fatal cycle into exit 0.
//
// Both halves are asserted, because the two are what the rule was nearly wrong
// about in opposite directions. Rules 1 and 2 restore the cycle; rule 4 is what
// keeps the fatal finding off DEC-0009.json, whose bytes declare no supersede
// and which therefore cannot perform the remedy the message prints.
func TestRoundThreesMisfiledVoucherDoesNotDeleteTheCycle(t *testing.T) {
	findings := validate.Check(storeOf(t, []testRecord{
		decisionAt("DEC-0001.json", decisionDoc("DEC-0001", map[string]any{
			"status":     "superseded",
			"supersedes": []any{"DEC-0002"},
			"note":       "junk",
		})),
		decisionAt("DEC-0002.json", decisionDoc("DEC-0002", map[string]any{
			"status":     "superseded",
			"supersedes": []any{"DEC-0001"},
		})),
		decisionAt("DEC-0009.json", decisionDoc("DEC-0001", nil)),
	}), shippedValidator(t))

	// Half one: the cycle is still there and still fatal.
	wantCycle := []string{".mindrail/knowledge/decisions/DEC-0002.json"}
	if got := pathsAt(findings, validate.StepSupersedeCycle); !slices.Equal(got, wantCycle) {
		t.Errorf("step 9 reported %v, want the one cycle member step 5 accepted\n\t%s",
			got, describe(findings))
	}
	for _, finding := range findingsAt(findings, validate.StepSupersedeCycle) {
		if !finding.Fatal {
			t.Errorf("the cycle surfaced without being fatal: %s: %q", finding.Path, finding.Message)
		}
	}

	// Half two: the file that owns nothing is named in no verdict about the node
	// it does not own — and is still told, by step 6, what is wrong with it.
	stray := ".mindrail/knowledge/decisions/DEC-0009.json"
	if got := stepsAt(findings, stray); !slices.Equal(got, []validate.Step{validate.StepFilenameConsistency}) {
		t.Errorf("the misfiled namesake reported steps %v, want [6]\n\t%s", got, describe(findings))
	}
}

// TestTwoValidRecordsClaimingOneIdDoNotFuseTheirLineages closes the one finding
// in this seam that pre-dates every remediation: the round-2 fix was scoped to
// schema-*invalid* duplicates, so two records that both satisfied their schema
// still had their edges unioned into one node.
//
// DEC-0001.json declares no "supersedes" and is on no cycle. Fusing DEC-0003's
// edge into its node produced a fatal finding naming it, with a remedy — drop
// the superseded id — that its bytes cannot carry out. Under D-52 the answer no
// longer depends on whether the second claimant satisfies its schema at all,
// which is the point of the decision: both runs below must agree.
func TestTwoValidRecordsClaimingOneIdDoNotFuseTheirLineages(t *testing.T) {
	validator := shippedValidator(t)

	store := func(extra map[string]any) []testRecord {
		return []testRecord{
			decisionAt("DEC-0001.json", decisionDoc("DEC-0001", map[string]any{
				"status": "superseded",
			})),
			decisionAt("DEC-0002.json", decisionDoc("DEC-0002", map[string]any{
				"supersedes": []any{"DEC-0001"},
			})),
			decisionAt("DEC-0003.json", decisionDoc("DEC-0001", withSupersedes(extra, "DEC-0002"))),
		}
	}

	accepted := validate.Check(storeOf(t, store(nil)), validator)
	if got := findingsAt(accepted, validate.StepSupersedeCycle); len(got) != 0 {
		t.Errorf("a second claimant's edge closed a cycle over a node it does not own:\n\t%s",
			describe(got))
	}
	for _, finding := range accepted {
		if finding.Fatal {
			t.Errorf("a misfiled duplicate made the repository BLOCKED: %s: %q",
				finding.Path, finding.Message)
		}
	}
	// The reader is still told both things that are true of this store: one id is
	// carried by two files, and one of those files is not where its id says.
	wantDuplicate := []string{
		".mindrail/knowledge/decisions/DEC-0001.json",
		".mindrail/knowledge/decisions/DEC-0003.json",
	}
	if got := pathsAt(accepted, validate.StepUniqueID); !slices.Equal(got, wantDuplicate) {
		t.Errorf("step 7 reported %v, want both files carrying DEC-0001\n\t%s",
			got, describe(accepted))
	}
	wantMisfiled := []string{".mindrail/knowledge/decisions/DEC-0003.json"}
	if got := pathsAt(accepted, validate.StepFilenameConsistency); !slices.Equal(got, wantMisfiled) {
		t.Errorf("step 6 reported %v, want the file whose name and id disagree\n\t%s",
			got, describe(accepted))
	}
	// And the record that is on nothing is named in nothing.
	if got := stepsAt(accepted, ".mindrail/knowledge/decisions/DEC-0002.json"); len(got) != 0 {
		t.Errorf("a record on no cycle and under no contested id reported steps %v\n\t%s",
			got, describe(accepted))
	}

	// The same store with the second claimant rejected. Under every earlier rule
	// this row and the one above disagreed about the cycle; under D-52 they
	// cannot, because neither answer was ever about the document's contents.
	rejected := validate.Check(storeOf(t, store(map[string]any{"titel": "oops"})), validator)
	if got := findingsAt(rejected, validate.StepSupersedeCycle); len(got) != 0 {
		t.Errorf("a rejected claimant closed a cycle over an accepted record's node:\n\t%s",
			describe(got))
	}
}

// TestADecisionAndAnInvariantNeverShareAGraphNode is the second half of the
// identity fix: the graph is keyed by (kind, id), not by the id alone.
//
// Today both shipped documents pin the id's spelling with a `pattern`, so no
// record step 5 accepted can carry the other kind's id and a bare-string key
// would behave identically. That is precisely why this is worth a test: the
// separation would otherwise be an accident of two regular expressions in a
// document decision D-36 lets the repository's contract change — and a record
// step 5 rejected is under no such constraint at all.
//
// The store below is an invariant whose id property says "DEC-0001". Under a
// bare-string key it becomes the decision lineage's missing node, its
// "supersedes" closes the walk, and a fatal supersede cycle is reported against
// a decision on the strength of a typo in an invariant. Under a (kind, id) key
// the two namespaces cannot touch: the decision's reference to DEC-0001 is
// dangling, which is what step 8 says.
func TestADecisionAndAnInvariantNeverShareAGraphNode(t *testing.T) {
	store := storeOf(t, []testRecord{
		decisionAt("DEC-0002.json", decisionDoc("DEC-0002", map[string]any{
			"supersedes": []any{"DEC-0001"},
		})),
		invariantAt("INV-0001.json", invariantDoc("DEC-0001", map[string]any{
			"supersedes": []any{"DEC-0002"},
		})),
	})

	findings := validate.Check(store, shippedValidator(t))

	if got := findingsAt(findings, validate.StepSupersedeCycle); len(got) != 0 {
		t.Errorf("an invariant's id closed a cycle in the decision namespace:\n\t%s", describe(got))
	}
	for _, finding := range findings {
		if finding.Fatal {
			t.Errorf("a cross-namespace fusion made the repository BLOCKED: %s: %q",
				finding.Path, finding.Message)
		}
	}
	// The decision's target genuinely is not there, and saying so is what proves
	// the invariant did not resolve it.
	wantTarget := []string{".mindrail/knowledge/decisions/DEC-0002.json"}
	if got := pathsAt(findings, validate.StepSupersedeTarget); !slices.Equal(got, wantTarget) {
		t.Errorf("step 8 reported %v, want the decision whose target nothing carries\n\t%s",
			got, describe(findings))
	}
	// And the invariant is reported where it belongs: its id breaks its own
	// document's pattern, which is step 5's business.
	if len(findingsAt(findings, validate.StepSchema)) == 0 {
		t.Errorf("the invariant carrying a decision's id was not reported at all:\n\t%s",
			describe(findings))
	}
}

// TestAValidStoreIsStillSilentWithAStrayDraftBesideIt is the broad over-fire
// guard for everything in this file.
//
// Each row is a repository that is working, plus one file a contributor left
// behind. The whole store must be quiet apart from that file's own step-5
// finding — a guard that takes a healthy repository down costs its owner exactly
// as much as one that lets a broken store through, and this fix pass exists
// because the last one did the first.
func TestAValidStoreIsStillSilentWithAStrayDraftBesideIt(t *testing.T) {
	validator := shippedValidator(t)

	for _, tt := range []struct {
		name    string
		records []testRecord
	}{
		{
			name: "a draft duplicating the id of the head of a lineage",
			records: []testRecord{
				decisionAt("DEC-0001.json", decisionDoc("DEC-0001", map[string]any{
					"status": "superseded",
				})),
				decisionAt("DEC-0002.json", decisionDoc("DEC-0002", map[string]any{
					"supersedes": []any{"DEC-0001"},
				})),
				decisionAt("DEC-0003.json", decisionDoc("DEC-0002", map[string]any{
					"supersedes": []any{"DEC-0001"},
					"titel":      "oops",
				})),
			},
		},
		{
			name: "a draft superseding itself under someone else's id",
			records: []testRecord{
				decisionAt("DEC-0001.json", decisionDoc("DEC-0001", nil)),
				decisionAt("DEC-0003.json", decisionDoc("DEC-0001", map[string]any{
					"supersedes": []any{"DEC-0001"},
					"titel":      "oops",
				})),
			},
		},
		{
			name: "an invariant draft carrying a decision's id",
			records: []testRecord{
				decisionAt("DEC-0001.json", decisionDoc("DEC-0001", map[string]any{
					"status": "superseded",
				})),
				decisionAt("DEC-0002.json", decisionDoc("DEC-0002", map[string]any{
					"supersedes": []any{"DEC-0001"},
				})),
				invariantAt("INV-0001.json", invariantDoc("DEC-0001", map[string]any{
					"supersedes": []any{"DEC-0002"},
				})),
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			findings := validate.Check(storeOf(t, tt.records), validator)

			for _, finding := range findings {
				if finding.Step != validate.StepSchema {
					t.Errorf("a healthy store with one stray draft produced a step-%d finding on %s: %q",
						finding.Step, finding.Path, finding.Message)
				}
				if finding.Fatal {
					t.Errorf("a stray draft made the repository BLOCKED: %s: %q",
						finding.Path, finding.Message)
				}
			}
			if len(findingsAt(findings, validate.StepSchema)) == 0 {
				t.Errorf("no record in this row actually failed step 5, so it guards nothing:\n\t%s",
					describe(findings))
			}
		})
	}
}

// TestACycleMessageIsTrueOfEveryFileItIsAttachedTo is the claim the round-2 HIGH
// finding caught being false, asserted against each reported file's own bytes.
//
// Step 9's message says every member "is reachable from every other by following
// supersedes". Over a fused node that was false of a file with no "supersedes"
// property at all, and neither a finding count nor a clean three-record cycle
// would have noticed: on a store where every file really is on the cycle, the
// sentence is true whatever rule produced it. That is what made the first
// version of this test unable to fail for the defect it is named after.
//
// So the check is derived rather than written down. Each row declares what each
// file supersedes, and every step-9 finding must name a file that declares at
// least one target the same message lists as a group member. A finding attached
// to a file whose bytes say nothing about the group fails this, whatever the
// count is — which is exactly the fusion, and exactly what row 2 produced before
// decision D-52.
func TestACycleMessageIsTrueOfEveryFileItIsAttachedTo(t *testing.T) {
	validator := shippedValidator(t)

	// declaring is one fixture file, kept in a form the assertion can read back:
	// which id it carries, what it says it supersedes, and whether step 5 takes
	// it.
	type declaring struct {
		file       string
		id         string
		supersedes []string
		rejected   bool
	}

	for _, tt := range []struct {
		name    string
		records []declaring
		want    []string
	}{
		{
			// The baseline: every file is on the cycle and every rule agrees.
			name: "a three-record cycle",
			records: []declaring{
				{file: "DEC-0001.json", id: "DEC-0001", supersedes: []string{"DEC-0002"}},
				{file: "DEC-0002.json", id: "DEC-0002", supersedes: []string{"DEC-0003"}},
				{file: "DEC-0003.json", id: "DEC-0003", supersedes: []string{"DEC-0001"}},
			},
			want: []string{
				".mindrail/knowledge/decisions/DEC-0001.json",
				".mindrail/knowledge/decisions/DEC-0002.json",
				".mindrail/knowledge/decisions/DEC-0003.json",
			},
		},
		{
			// The fusion. DEC-0003.json carries DEC-0001's id and supersedes
			// DEC-0002; unioning its edge into DEC-0001's node closes a walk and
			// files the fatal finding on DEC-0001.json, which declares nothing.
			name: "a misfiled namesake supplying the closing edge",
			records: []declaring{
				{file: "DEC-0001.json", id: "DEC-0001"},
				{file: "DEC-0002.json", id: "DEC-0002", supersedes: []string{"DEC-0001"}},
				{file: "DEC-0003.json", id: "DEC-0001", supersedes: []string{"DEC-0002"}},
			},
			want: nil,
		},
		{
			// Round 3's store: the cycle is real, one member is rejected, and the
			// namesake that declares nothing must be named in no verdict.
			name: "a rejected member and a namesake that declares nothing",
			records: []declaring{
				{file: "DEC-0001.json", id: "DEC-0001", supersedes: []string{"DEC-0002"}, rejected: true},
				{file: "DEC-0002.json", id: "DEC-0002", supersedes: []string{"DEC-0001"}},
				{file: "DEC-0009.json", id: "DEC-0001"},
			},
			want: []string{".mindrail/knowledge/decisions/DEC-0002.json"},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			records := make([]testRecord, 0, len(tt.records))
			declared := make(map[string][]string, len(tt.records))
			for _, rec := range tt.records {
				extra := map[string]any{"status": "superseded"}
				if len(rec.supersedes) > 0 {
					targets := make([]any, 0, len(rec.supersedes))
					for _, target := range rec.supersedes {
						targets = append(targets, target)
					}
					extra["supersedes"] = targets
				}
				if rec.rejected {
					extra["note"] = "junk"
				}
				records = append(records, decisionAt(rec.file, decisionDoc(rec.id, extra)))
				declared[".mindrail/knowledge/decisions/"+rec.file] = rec.supersedes
			}

			all := validate.Check(storeOf(t, records), validator)
			findings := findingsAt(all, validate.StepSupersedeCycle)
			if got := pathsAt(all, validate.StepSupersedeCycle); !slices.Equal(got, tt.want) {
				t.Errorf("step 9 reported %v, want %v\n\t%s", got, tt.want, describe(all))
			}

			for _, finding := range findings {
				if !strings.Contains(finding.Message, "reachable from every other") {
					t.Fatalf("step 9's message no longer makes the claim this test checks: %q",
						finding.Message)
				}
				// The message lists the group's ids. The file it is attached to
				// has to declare a supersede into that list, or the sentence is
				// not true of it.
				onTheGroup := false
				for _, target := range declared[finding.Path] {
					if strings.Contains(finding.Message, target) {
						onTheGroup = true
						break
					}
				}
				if !onTheGroup {
					t.Errorf("step 9 told %s it is reachable from every other member, but its own %q names none of them: %q",
						finding.Path, "supersedes", finding.Message)
				}
			}
		})
	}
}
