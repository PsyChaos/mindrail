package validate_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/PsyChaos/mindrail/internal/knowledge/validate"
)

// This file is about node identity in the supersede graph: which record gets to
// say what a given id supersedes.
//
// lineage_test.go answers the neighbouring question — which records are in the
// graph at all — and its answer is "every record the loader read". That answer
// is right and this file does not narrow it. What it narrows is whose "id" the
// graph believes when two files claim one, which is a question the first
// remediation never asked: before it, only step-5 survivors were nodes, so a
// node's id had been vouched for by the schema's `pattern` and, because D-40
// asks a survivor step 7, a survivor duplicating another's id was reported.
// Building the graph over every record read — the correct fix for the round-1
// fail-open — removed both guarantees in silence.
//
// Every test here therefore comes in a pair. The over-fire arm is a healthy
// store with one stray draft in it, which must stay quiet; the under-fire arm is
// a store where the rejected record's edges are the only account of an id there
// is, and they must still count. A rule that gets one of those right and the
// other wrong is the shape MR-002 has now produced twice.

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
// under-fire arm, and it is the one the fix above could have broken.
//
// The rule is not "a rejected record's supersedes never counts" — that is the
// round-1 fail-open, where one unknown property on one member deleted a fatal
// cycle. The rule is that a rejected record's claim about *which node it is*
// does not override an accepted record's. Where no accepted record carries the
// id, the rejected one is the only account there is and its edges stand.
//
// Both rows below are cycles whose closing edge comes out of a record step 5
// rejected, and both must still be fatal. The second row is the sharper one: the
// rejected record's id does not even agree with its file name, so it is not "the
// file its id names" — and it still owns the node, because nothing else claims
// it. The distinction that decides these cases is contest, not tidiness.
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
			// The same closed walk, with the rejected record filed under a name
			// that disagrees with its id. Nothing else claims DEC-0001, so its
			// "supersedes" is still the only statement about DEC-0001's lineage
			// this store contains, and the walk still has no end.
			name: "the rejected record is filed under another name",
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
			wantCycle: []string{".mindrail/knowledge/decisions/DEC-0002.json"},
		},
		{
			// Two rejected records under one id and no accepted one. Neither is
			// vouched for, so neither outranks the other and the pair is still
			// the store's only account of DEC-0001. Suppressing both would delete
			// the cycle on the strength of an unknown property, twice over.
			name: "two rejected records share the id and no accepted record has it",
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
			wantCycle: []string{".mindrail/knowledge/decisions/DEC-0002.json"},
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

// TestAnAcceptedRecordsIdIsNotOutrankedByARejectedOne states the boundary the
// two tests above sit on either side of, as one store with one difference.
//
// The two scenarios are the same three files. In the first, the file that would
// contest DEC-0001's id fails step 5 and is therefore not who DEC-0001 is; in
// the second it is corrected and becomes an equal claimant, at which point the
// contest is real, step 7 names both files, and the cycle its "supersedes"
// closes is reported against the records that actually declare it.
//
// This is what makes the suppression a deferral rather than a deletion: the
// condition surfaces the moment the record is readable, which is D-40's own
// shape, and the reader is never told to edit a file that cannot perform the
// remedy.
func TestAnAcceptedRecordsIdIsNotOutrankedByARejectedOne(t *testing.T) {
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

	rejected := validate.Check(storeOf(t, store(map[string]any{"titel": "oops"})), validator)
	if got := findingsAt(rejected, validate.StepSupersedeCycle); len(got) != 0 {
		t.Errorf("a rejected claimant closed a cycle over an accepted record's node:\n\t%s",
			describe(got))
	}

	accepted := validate.Check(storeOf(t, store(nil)), validator)
	wantCycle := []string{
		".mindrail/knowledge/decisions/DEC-0001.json",
		".mindrail/knowledge/decisions/DEC-0002.json",
		".mindrail/knowledge/decisions/DEC-0003.json",
	}
	if got := pathsAt(accepted, validate.StepSupersedeCycle); !slices.Equal(got, wantCycle) {
		t.Errorf("once the claimant satisfies its schema, step 9 reported %v, want %v\n\t%s",
			got, wantCycle, describe(accepted))
	}
	// And the reader is told why one id has two files, so the cycle's remedy is
	// a choice they can actually make.
	wantDuplicate := []string{
		".mindrail/knowledge/decisions/DEC-0001.json",
		".mindrail/knowledge/decisions/DEC-0003.json",
	}
	if got := pathsAt(accepted, validate.StepUniqueID); !slices.Equal(got, wantDuplicate) {
		t.Errorf("step 7 reported %v, want both files carrying DEC-0001\n\t%s",
			got, describe(accepted))
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

// TestACycleMessageIsTrueOfEveryFileItIsAttachedTo is the claim the HIGH finding
// caught being false, asserted as text rather than as a count.
//
// Step 9's message says every member "is reachable from every other by following
// supersedes". Over a fused node that was false of a file with no "supersedes"
// property at all, and the finding count alone would not have noticed: the right
// answer and the wrong one were both "some records are reported".
//
// The store below is a genuine three-record cycle. Every reported file declares
// a "supersedes" that is one of the group's own edges, so the sentence holds of
// each of them.
func TestACycleMessageIsTrueOfEveryFileItIsAttachedTo(t *testing.T) {
	store := storeOf(t, []testRecord{
		decisionAt("DEC-0001.json", decisionDoc("DEC-0001", map[string]any{
			"supersedes": []any{"DEC-0002"},
		})),
		decisionAt("DEC-0002.json", decisionDoc("DEC-0002", map[string]any{
			"supersedes": []any{"DEC-0003"},
		})),
		decisionAt("DEC-0003.json", decisionDoc("DEC-0003", map[string]any{
			"supersedes": []any{"DEC-0001"},
		})),
	})

	findings := findingsAt(validate.Check(store, shippedValidator(t)), validate.StepSupersedeCycle)
	if len(findings) != 3 {
		t.Fatalf("step 9 reported %d findings over a three-record cycle, want 3:\n\t%s",
			len(findings), describe(findings))
	}
	for _, finding := range findings {
		if !strings.Contains(finding.Message, "reachable from every other") {
			t.Fatalf("step 9's message no longer makes the claim this test checks: %q", finding.Message)
		}
		if !strings.Contains(finding.Message, "DEC-0001, DEC-0002, DEC-0003") {
			t.Errorf("step 9 named the group as something other than its three members: %q",
				finding.Message)
		}
	}
}
