package validate_test

import (
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/PsyChaos/mindrail/internal/knowledge/loader"
	"github.com/PsyChaos/mindrail/internal/knowledge/validate"
)

// This file is about one question: which records the lineage graph is built
// over. Decision D-45 answers it for records the loader could NOT read, and
// suppression_test.go is where that is guarded. These tests answer it for
// records the loader read and step 5 then rejected — the case MR-002's audit
// found the pipeline getting backwards in both directions at once.

// TestAnUnknownPropertyOnOneMemberDoesNotDeleteTheSupersedeCycle is the
// regression for the root defect: the graph was built from step 5's survivors,
// so a record that failed step 5 took its supersede edges out of the store with
// it.
//
// The two stores below differ by one JSON property that no step reads. Neither
// "supersedes" array is touched and the cycle is physically identical. Before
// the fix the second store reported no step-9 finding at all — `status` exited
// 0, readiness was DEGRADED rather than BLOCKED, and the milestone's only fatal
// check was off. One unknown property cannot be allowed to do that: decision
// D-39 makes a cycle fatal because no lineage has an end and every answer about
// which record is current is wrong, and that is exactly as true when a member
// also breaks its schema.
//
// The assertion is on DEC-0002, which is untouched, fully valid, and was asked
// step 9. D-40's own words are that a rejected record is not asked steps 6-11
// but "every *other* record still is", and step 9's answer about DEC-0002 has to
// be the truth.
func TestAnUnknownPropertyOnOneMemberDoesNotDeleteTheSupersedeCycle(t *testing.T) {
	validator := shippedValidator(t)

	cycle := func(extra map[string]any) loader.Store {
		return storeOf(t, []testRecord{
			decisionAt("DEC-0001.json", decisionDoc("DEC-0001", withSupersedes(extra, "DEC-0002"))),
			decisionAt("DEC-0002.json", decisionDoc("DEC-0002", map[string]any{
				"supersedes": []any{"DEC-0001"},
			})),
		})
	}

	// Control: both members valid. Both are reported, both fatally.
	control := validate.Check(cycle(nil), validator)
	wantBoth := []string{
		".mindrail/knowledge/decisions/DEC-0001.json",
		".mindrail/knowledge/decisions/DEC-0002.json",
	}
	if got := pathsAt(control, validate.StepSupersedeCycle); !slices.Equal(got, wantBoth) {
		t.Fatalf("with both members valid, step 9 reported %v, want %v\n\t%s",
			got, wantBoth, describe(control))
	}

	// Scenario: byte-identical but for one unknown property on DEC-0001.
	scenario := validate.Check(cycle(map[string]any{"note": "junk"}), validator)

	wantSurvivor := []string{".mindrail/knowledge/decisions/DEC-0002.json"}
	if got := pathsAt(scenario, validate.StepSupersedeCycle); !slices.Equal(got, wantSurvivor) {
		t.Fatalf("with one member schema-invalid, step 9 reported %v, want %v\n\t%s",
			got, wantSurvivor, describe(scenario))
	}

	fatal := 0
	for _, finding := range scenario {
		if finding.Fatal {
			fatal++
		}
	}
	if fatal != 1 {
		t.Errorf("the store carries %d fatal findings, want the cycle's one:\n\t%s",
			fatal, describe(scenario))
	}

	// D-40's other half: the rejected record is reported by step 5 and by
	// nothing else. A fix that kept the cycle by asking a rejected record steps
	// 6-11 would add a step-9 finding here.
	got := stepsAt(scenario, ".mindrail/knowledge/decisions/DEC-0001.json")
	if !slices.Equal(got, []validate.Step{validate.StepSchema}) {
		t.Errorf("the schema-invalid member reported steps %v, want [5]\n\t%s", got, describe(scenario))
	}
}

// TestASchemaInvalidRecordDoesNotManufactureAFinding is the over-fire guard for
// the test above, and it is the half that matters most.
//
// Putting rejected records into the graph makes every cross-record step see more
// of the store, which is exactly the shape that starts reporting conditions that
// are not there. Each row below is a healthy repository that happens to contain
// one schema-invalid record, and the whole store must be quiet apart from that
// record's own step-5 finding. A guard that rejects a working configuration
// takes a repository down as thoroughly as one that lets a bad value through.
func TestASchemaInvalidRecordDoesNotManufactureAFinding(t *testing.T) {
	validator := shippedValidator(t)

	for _, tt := range []struct {
		name    string
		records []testRecord
	}{
		{
			// An invalid record in a straight, acyclic lineage. Nothing closes,
			// and the one active record at the head is the only one there is.
			name: "an invalid record in an acyclic lineage",
			records: []testRecord{
				decisionAt("DEC-0001.json", decisionDoc("DEC-0001", map[string]any{
					"status": "superseded",
					"note":   "junk",
				})),
				decisionAt("DEC-0002.json", decisionDoc("DEC-0002", map[string]any{
					"supersedes": []any{"DEC-0001"},
				})),
			},
		},
		{
			// The invalid record's own "status" says active. It must not be
			// counted as a second current record: "active" is read out of a field
			// step 5 may have just rejected, and D-40 did not ask it step 10.
			name: "an invalid record calling itself active beside a valid active one",
			records: []testRecord{
				decisionAt("DEC-0001.json", decisionDoc("DEC-0001", map[string]any{
					"note": "junk",
				})),
				decisionAt("DEC-0002.json", decisionDoc("DEC-0002", map[string]any{
					"supersedes": []any{"DEC-0001"},
				})),
			},
		},
		{
			// The rejected field IS "supersedes": it is a string where the schema
			// requires an array, so the body does not decode at all. No edge may
			// be drawn from a field this pipeline could not read, and inventing
			// one here would close a loop that nobody wrote.
			name: "a record whose supersedes is the thing step 5 rejected",
			records: []testRecord{
				decisionAt("DEC-0001.json", decisionDoc("DEC-0001", map[string]any{
					"supersedes": "DEC-0002",
				})),
				decisionAt("DEC-0002.json", decisionDoc("DEC-0002", map[string]any{
					"status":     "superseded",
					"supersedes": []any{"DEC-0001"},
				})),
			},
		},
		{
			// The same, one step subtler, and the row that decides how the body is
			// decoded. encoding/json fills a struct as it goes and reports the
			// first member it cannot convert, so ["DEC-0002", 5] both fails and
			// leaves "DEC-0002" behind in the slice. Unmarshalling straight into
			// the subject would therefore draw a real edge out of a rejected
			// array, closing this pair into a fatal supersede cycle on the
			// strength of half a field.
			name: "a record whose supersedes array is rejected after its first entry",
			records: []testRecord{
				decisionAt("DEC-0001.json", decisionDoc("DEC-0001", map[string]any{
					"supersedes": []any{"DEC-0002", 5},
				})),
				decisionAt("DEC-0002.json", decisionDoc("DEC-0002", map[string]any{
					"status":     "superseded",
					"supersedes": []any{"DEC-0001"},
				})),
			},
		},
		{
			// Two records with no id, each superseding a different lineage. Under
			// one empty-string node their edges would merge, and DEC-0003 and
			// DEC-0004 — which have nothing whatever to do with each other —
			// would be reported as two current records of one lineage. An id-less
			// record has no "from" node to hang a supersede on.
			name: "id-less records superseding two unrelated lineages",
			records: []testRecord{
				decisionAt("DEC-0001.json", decisionDoc("DEC-0001", map[string]any{
					"supersedes": []any{"DEC-0003"},
				}, "id")),
				decisionAt("DEC-0002.json", decisionDoc("DEC-0002", map[string]any{
					"supersedes": []any{"DEC-0004"},
				}, "id")),
				decisionAt("DEC-0003.json", decisionDoc("DEC-0003", nil)),
				decisionAt("DEC-0004.json", decisionDoc("DEC-0004", nil)),
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			findings := validate.Check(storeOf(t, tt.records), validator)

			for _, finding := range findings {
				if finding.Step != validate.StepSchema {
					t.Errorf("a healthy store with one invalid record produced a step-%d finding on %s: %q",
						finding.Step, finding.Path, finding.Message)
				}
			}
			// The row would prove nothing if the record it calls invalid were
			// valid: there would be no rejected record in the graph at all.
			if len(findingsAt(findings, validate.StepSchema)) == 0 {
				t.Errorf("no record in this row actually failed step 5, so it guards nothing:\n\t%s",
					describe(findings))
			}
		})
	}
}

// TestAClosedGroupOfOnlyInvalidRecordsIsLeftToStepFive records a boundary rather
// than hiding it.
//
// Every member of this cycle failed step 5, so decision D-40 asked none of them
// step 9 and no step-9 finding has a path to be attached to. The condition is
// not silently dropped — every record carries a step-5 finding naming it, the
// store reports KNOWLEDGE_INVALID, and the cycle surfaces as fatal the moment
// the records satisfy their schema.
//
// The alternative would be attaching a step 9 finding to a record D-40 says was
// not asked step 9, which is a frozen decision this fix does not get to reopen.
// It is written down here so that a later reader meets it as a decision rather
// than as a gap.
func TestAClosedGroupOfOnlyInvalidRecordsIsLeftToStepFive(t *testing.T) {
	validator := shippedValidator(t)
	broken := func(id, target string) string {
		return decisionDoc(id, map[string]any{
			"supersedes": []any{target},
			"note":       "junk",
		})
	}
	store := storeOf(t, []testRecord{
		decisionAt("DEC-0001.json", broken("DEC-0001", "DEC-0002")),
		decisionAt("DEC-0002.json", broken("DEC-0002", "DEC-0001")),
	})

	findings := validate.Check(store, validator)
	for _, finding := range findings {
		if finding.Step != validate.StepSchema {
			t.Errorf("step %d fired on a record D-40 did not ask it: %s", finding.Step, finding.Path)
		}
	}
	if len(findings) != 2 {
		t.Fatalf("want one step-5 finding per record, got:\n\t%s", describe(findings))
	}

	// Making the same two records schema-valid — one property, nothing else —
	// must produce the fatal cycle. Without this the test above would pass on a
	// pipeline that had simply stopped detecting cycles.
	repaired := storeOf(t, []testRecord{
		decisionAt("DEC-0001.json", decisionDoc("DEC-0001", map[string]any{"supersedes": []any{"DEC-0002"}})),
		decisionAt("DEC-0002.json", decisionDoc("DEC-0002", map[string]any{"supersedes": []any{"DEC-0001"}})),
	})
	if got := len(findingsAt(validate.Check(repaired, validator), validate.StepSupersedeCycle)); got != 2 {
		t.Errorf("with the same two records made valid, step 9 reported %d findings, want 2", got)
	}
}

// TestEveryRecordOnAClosedWalkIsReported is the regression for step 9 having
// reported a depth-first walk's back edge rather than the closed group.
//
// All five records below lie on one closed supersede walk: DEC-0001 supersedes
// DEC-0002 and DEC-0004; 0002 -> 0003 -> 0001 closes one loop and
// 0004 -> 0005 -> 0002 -> 0003 -> 0001 -> 0004 closes another through the same
// nodes, so every record is reachable from every other. The delivered walk
// reported three of them and left DEC-0004 and DEC-0005 out of every diagnostic
// and every next_action, while the comment above the step promised that "every
// member of the cycle is reported".
//
// Statuses are set so that step 10 stays quiet: this is an assertion about step
// 9's membership, and a store where every record is also a duplicate active
// would let a step-10 finding stand in for a missing step-9 one.
func TestEveryRecordOnAClosedWalkIsReported(t *testing.T) {
	superseded := func(targets ...any) map[string]any {
		return map[string]any{"status": "superseded", "supersedes": targets}
	}
	store := storeOf(t, []testRecord{
		decisionAt("DEC-0001.json", decisionDoc("DEC-0001", map[string]any{
			"supersedes": []any{"DEC-0002", "DEC-0004"},
		})),
		decisionAt("DEC-0002.json", decisionDoc("DEC-0002", superseded("DEC-0003"))),
		decisionAt("DEC-0003.json", decisionDoc("DEC-0003", superseded("DEC-0001"))),
		decisionAt("DEC-0004.json", decisionDoc("DEC-0004", superseded("DEC-0005"))),
		decisionAt("DEC-0005.json", decisionDoc("DEC-0005", superseded("DEC-0002"))),
	})

	findings := validate.Check(store, shippedValidator(t))

	want := make([]string, 0, 5)
	for i := 1; i <= 5; i++ {
		want = append(want, fmt.Sprintf(".mindrail/knowledge/decisions/DEC-000%d.json", i))
	}
	if got := pathsAt(findings, validate.StepSupersedeCycle); !slices.Equal(got, want) {
		t.Fatalf("step 9 reported %v, want every record on the closed walk %v\n\t%s",
			got, want, describe(findings))
	}

	// Each record must also be able to see the others it is closed with, or a
	// reader has no way to know which file to edit to break the group.
	for _, finding := range findingsAt(findings, validate.StepSupersedeCycle) {
		for i := 1; i <= 5; i++ {
			id := fmt.Sprintf("DEC-000%d", i)
			if !strings.Contains(finding.Message, id) {
				t.Errorf("the finding on %s does not name group member %s: %q",
					finding.Path, id, finding.Message)
			}
		}
	}
}

// TestARecordThatSupersedesItselfIsCountedAsOneRecord reads the text of the
// milestone's only fatal message, at the group size nothing else exercises.
//
// A record whose "supersedes" names its own id is a closed group of one — spec
// §95 step 9 counts it and closedGroups is written to find it — and the message
// hard-coded the plural noun, so that store was published as "a closed supersede
// group of 1 records" in `status --json`'s error.why, in status's stderr, and in
// both of doctor's renderings. It survived four audit rounds because no test in
// the repository read this string: the assertions elsewhere check which files are
// reported, and the two message-reading tests both use groups of three.
//
// Both counts are asserted here. A test that only checked the singular would
// pass on a message that had lost its plural instead.
func TestARecordThatSupersedesItselfIsCountedAsOneRecord(t *testing.T) {
	validator := shippedValidator(t)

	alone := validate.Check(storeOf(t, []testRecord{
		decisionAt("DEC-0001.json", decisionDoc("DEC-0001", map[string]any{
			"status":     "superseded",
			"supersedes": []any{"DEC-0001"},
		})),
	}), validator)

	got := findingsAt(alone, validate.StepSupersedeCycle)
	if len(got) != 1 {
		t.Fatalf("a self-superseding record produced %d step-9 findings, want 1:\n\t%s",
			len(got), describe(alone))
	}
	if want := "group of 1 record:"; !strings.Contains(got[0].Message, want) {
		t.Errorf("step 9 counts the group as %q; the message reads:\n\t%q",
			want, got[0].Message)
	}

	// The other arm, so a fix that simply dropped the "s" is red too.
	pair := validate.Check(storeOf(t, []testRecord{
		decisionAt("DEC-0001.json", decisionDoc("DEC-0001", map[string]any{
			"status":     "superseded",
			"supersedes": []any{"DEC-0002"},
		})),
		decisionAt("DEC-0002.json", decisionDoc("DEC-0002", map[string]any{
			"status":     "superseded",
			"supersedes": []any{"DEC-0001"},
		})),
	}), validator)

	for _, finding := range findingsAt(pair, validate.StepSupersedeCycle) {
		if want := "group of 2 records:"; !strings.Contains(finding.Message, want) {
			t.Errorf("step 9 counts a two-record group as something other than %q:\n\t%q",
				want, finding.Message)
		}
	}
}

// TestARecordOutsideAClosedGroupIsNotDraggedIntoIt is the over-fire guard for
// the test above.
//
// Widening step 9 from a walk to a component is exactly the direction that
// starts reporting records that are merely near a cycle. DEC-0001 points into
// the group, DEC-0009 is pointed at by it, and neither is on a closed walk: from
// DEC-0001 there is no way back to DEC-0001, and from DEC-0009 there is no way
// out at all.
func TestARecordOutsideAClosedGroupIsNotDraggedIntoIt(t *testing.T) {
	superseded := func(targets ...any) map[string]any {
		return map[string]any{"status": "superseded", "supersedes": targets}
	}
	store := storeOf(t, []testRecord{
		decisionAt("DEC-0001.json", decisionDoc("DEC-0001", map[string]any{
			"supersedes": []any{"DEC-0002"},
		})),
		decisionAt("DEC-0002.json", decisionDoc("DEC-0002", superseded("DEC-0003"))),
		decisionAt("DEC-0003.json", decisionDoc("DEC-0003", superseded("DEC-0002", "DEC-0009"))),
		decisionAt("DEC-0009.json", decisionDoc("DEC-0009", superseded())),
	})

	findings := validate.Check(store, shippedValidator(t))
	want := []string{
		".mindrail/knowledge/decisions/DEC-0002.json",
		".mindrail/knowledge/decisions/DEC-0003.json",
	}
	if got := pathsAt(findings, validate.StepSupersedeCycle); !slices.Equal(got, want) {
		t.Errorf("step 9 reported %v, want only the two records closed on each other\n\t%s",
			got, describe(findings))
	}
}

// TestTheVerdictDoesNotDependOnTheOrderTheRecordsWereRead is what BA-05 asked
// for, stated as the property rather than as one line of code.
//
// The graph traversals run over Go maps, and a traversal whose result depends on
// map iteration order produces a different report on every run of the same
// bytes — tech-stack §2.2 requires a deterministic core, and D-47 requires
// byte-identical output. TestCheckIsDeterministicAcrossRepeatedCalls compares
// runs only against each other, so a traversal that happened to be stable within
// one process would satisfy it.
//
// This permutes the input instead. The loader's buckets are sorted by file name,
// so the permutation is reconstructed into that order by storeOf and the only
// thing that actually varies is the order the records were appended and
// therefore the order the map was populated in. Deleting the sort of a
// component's members, or of the components themselves, turns this red.
func TestTheVerdictDoesNotDependOnTheOrderTheRecordsWereRead(t *testing.T) {
	validator := shippedValidator(t)

	// Two closed groups, one fork, one dangling target and one straight lineage,
	// so that every traversal in lineage.go has more than one entry point to be
	// unstable about.
	base := []testRecord{
		decisionAt("DEC-0001.json", decisionDoc("DEC-0001", map[string]any{"supersedes": []any{"DEC-0002", "DEC-0004"}})),
		decisionAt("DEC-0002.json", decisionDoc("DEC-0002", map[string]any{"supersedes": []any{"DEC-0003"}})),
		decisionAt("DEC-0003.json", decisionDoc("DEC-0003", map[string]any{"supersedes": []any{"DEC-0001"}})),
		decisionAt("DEC-0004.json", decisionDoc("DEC-0004", map[string]any{"supersedes": []any{"DEC-0005"}})),
		decisionAt("DEC-0005.json", decisionDoc("DEC-0005", map[string]any{"supersedes": []any{"DEC-0002"}})),
		decisionAt("DEC-0006.json", decisionDoc("DEC-0006", map[string]any{"supersedes": []any{"DEC-0007"}})),
		decisionAt("DEC-0007.json", decisionDoc("DEC-0007", map[string]any{"supersedes": []any{"DEC-0006"}})),
		decisionAt("DEC-0008.json", decisionDoc("DEC-0008", map[string]any{"supersedes": []any{"DEC-4242"}})),
		invariantAt("INV-0001.json", invariantDoc("INV-0001", nil)),
	}

	first, err := json.Marshal(validate.Check(storeOf(t, base), validator))
	if err != nil {
		t.Fatalf("json.Marshal(findings): %v", err)
	}
	if len(first) < 512 {
		t.Fatalf("the fixture produced %d bytes of findings; permuting one is not a test", len(first))
	}

	shuffler := rand.New(rand.NewPCG(1, 2))
	for run := range 24 {
		permuted := slices.Clone(base)
		shuffler.Shuffle(len(permuted), func(i, j int) {
			permuted[i], permuted[j] = permuted[j], permuted[i]
		})

		again, marshalErr := json.Marshal(validate.Check(storeOf(t, permuted), validator))
		if marshalErr != nil {
			t.Fatalf("json.Marshal(findings): %v", marshalErr)
		}
		if string(again) != string(first) {
			t.Fatalf("permutation %d produced different findings:\n  sorted: %s\npermuted: %s",
				run, first, again)
		}
	}
}

// TestALargeGroupNamesTenRecordsAndCountsTheRest pins the cap that keeps steps
// 7, 9 and 10 linear.
//
// The cap is not cosmetic. Naming every sibling in every member's message costs
// O(n) bytes per record and O(n^2) for the group: 1000 records in one lineage
// produced 56 MB of message text and 2000 produced 224 MB, against a 150 ms p95
// SLO for `mindrail status`. The count has to survive the cap, or a reader would
// be told about ten broken records when there are two hundred.
func TestALargeGroupNamesTenRecordsAndCountsTheRest(t *testing.T) {
	const total = 40

	records := make([]testRecord, 0, total)
	for k := 1; k <= total; k++ {
		id := fmt.Sprintf("DEC-%04d", k)
		extra := map[string]any{}
		if k > 1 {
			extra["supersedes"] = []any{fmt.Sprintf("DEC-%04d", k-1)}
		}
		records = append(records, decisionAt(id+".json", decisionDoc(id, extra)))
	}

	findings := findingsAt(validate.Check(storeOf(t, records), shippedValidator(t)),
		validate.StepDuplicateActiveLineage)
	if len(findings) != total {
		t.Fatalf("step 10 reported %d findings, want one per active record", len(findings))
	}

	message := findings[0].Message
	if named := strings.Count(message, ".json ("); named != 10 {
		t.Errorf("the message names %d records, want the cap of 10: %q", named, message)
	}
	if !strings.Contains(message, "and the remaining 30") {
		t.Errorf("the message does not account for the records it did not name: %q", message)
	}
	// The true size still has to be stated, or the cap would be hiding the scale
	// of the problem rather than the length of the list.
	if !strings.Contains(message, fmt.Sprintf("one of %d active records", total)) {
		t.Errorf("the message does not carry the real group size: %q", message)
	}
}

// TestStepSevenAlsoCountsTheSiblingsItDidNotName is the same property for step
// 7, which the test above does not reach.
//
// The cap is shared by steps 7, 9 and 10, but step 7 gets to its list a
// different way: it walks a sorted slice and stops early, so the slice it hands
// to nameList is *already* the ten it will show, and the true size has to be
// passed separately as `len(paths)-1`. That second argument is the whole
// difference between "here are ten of your fourteen colliding files" and "here
// are your files" — and it was unguarded. Replacing it with `len(others)` left
// every test in the suite green while a store with fourteen files under one id
// reported ten of them and said nothing about the other four.
//
// Fifteen files is the smallest size that separates the two readings: it has to
// exceed the cap of ten, and the count has to be one a wrong answer could not
// also produce.
func TestStepSevenAlsoCountsTheSiblingsItDidNotName(t *testing.T) {
	const files = 15

	records := make([]testRecord, 0, files)
	for k := 1; k <= files; k++ {
		// One id, fifteen files: step 7's condition, at a size past the cap.
		records = append(records, decisionAt(fmt.Sprintf("DEC-%04d.json", k),
			decisionDoc("DEC-0001", nil)))
	}

	findings := findingsAt(validate.Check(storeOf(t, records), shippedValidator(t)),
		validate.StepUniqueID)
	if len(findings) != files {
		t.Fatalf("step 7 reported %d findings, want one per colliding file", len(findings))
	}

	for _, finding := range findings {
		named := strings.Count(finding.Message, ".json")
		// One mention is the reporting record's own path at the head of the
		// message; the rest are the siblings it names, and the cap is ten.
		if named != 1+maxSiblingsAMessageNames {
			t.Errorf("%s names %d paths, want its own plus the cap of %d: %q",
				finding.Path, named, maxSiblingsAMessageNames, finding.Message)
		}
		// Fifteen files, one of them this record, ten of the other fourteen
		// named: four unaccounted for unless the message says so.
		if want := "and the remaining 4"; !strings.Contains(finding.Message, want) {
			t.Errorf("%s does not say how many colliding files it left unnamed (want %q): %q",
				finding.Path, want, finding.Message)
		}
	}
}

// TestStepSevenSaysNothingAboutRemainingFilesWhenThereAreNone is the over-fire
// guard for the test above.
//
// A tail computed from the wrong total is one failure; a tail printed when the
// list is complete is the other, and it is the one that reaches every ordinary
// report. Two files under one id is the common case, and a message ending "and
// the remaining 0" would be a defect in every duplicate this project will ever
// see.
func TestStepSevenSaysNothingAboutRemainingFilesWhenThereAreNone(t *testing.T) {
	for _, files := range []int{2, maxSiblingsAMessageNames, maxSiblingsAMessageNames + 1} {
		records := make([]testRecord, 0, files)
		for k := 1; k <= files; k++ {
			records = append(records, decisionAt(fmt.Sprintf("DEC-%04d.json", k),
				decisionDoc("DEC-0001", nil)))
		}

		findings := findingsAt(validate.Check(storeOf(t, records), shippedValidator(t)),
			validate.StepUniqueID)
		if len(findings) != files {
			t.Fatalf("%d files: step 7 reported %d findings, want one each", files, len(findings))
		}

		// With `files` files, each message names `files-1` siblings. The tail
		// belongs only when that exceeds the cap.
		wantTail := files-1 > maxSiblingsAMessageNames
		for _, finding := range findings {
			if got := strings.Contains(finding.Message, "and the remaining"); got != wantTail {
				t.Errorf("%d files: %s %s a remaining-count, want %v: %q",
					files, finding.Path,
					map[bool]string{true: "carries", false: "carries no"}[got], wantTail, finding.Message)
			}
			// Whatever the size, every message still names the file the reader
			// has to choose between and the id they collide on.
			if !strings.Contains(finding.Message, finding.Path) || !strings.Contains(finding.Message, "DEC-0001") {
				t.Errorf("%d files: message does not carry its own path and the colliding id: %q", files, finding.Message)
			}
		}
	}
}

// maxSiblingsAMessageNames is the cap as a literal.
//
// It is not read from validate.maxNamedRecords, and that is deliberate: a test
// that derives its expectation from the constant it is guarding passes whatever
// the constant becomes, which is the shape that let the cap go unpinned in the
// first place.
const maxSiblingsAMessageNames = 10

// TestMessageTextGrowsLinearlyWithTheStore is B-03's measured curve turned into
// an assertion.
//
// The defect was quadratic message text: every member of a lineage carried a
// list of every other member. Doubling the store quadrupled the bytes — measured
// at 56,176,000 bytes for 1000 records and 224,352,000 for 2000, with a worst
// case of 11.7 s and 17.3 GB of peak RSS. A ratio assertion is what distinguishes
// "smaller" from "no longer quadratic": halving the constant would leave the
// curve exactly as steep.
func TestMessageTextGrowsLinearlyWithTheStore(t *testing.T) {
	validator := shippedValidator(t)

	bytesFor := func(n int) int {
		records := make([]testRecord, 0, n)
		for k := 1; k <= n; k++ {
			id := fmt.Sprintf("DEC-%04d", k)
			extra := map[string]any{}
			if k > 1 {
				extra["supersedes"] = []any{fmt.Sprintf("DEC-%04d", k-1)}
			}
			records = append(records, decisionAt(id+".json", decisionDoc(id, extra)))
		}

		findings := validate.Check(storeOf(t, records), validator)
		if len(findings) != n {
			t.Fatalf("a chain of %d active records produced %d findings, want one each", n, len(findings))
		}
		total := 0
		for _, finding := range findings {
			total += len(finding.Message)
		}
		return total
	}

	small, large := bytesFor(500), bytesFor(1000)

	// Linear growth doubles; quadratic growth quadruples. Three is the midpoint
	// that neither can reach from the other's side, and it leaves room for the
	// per-record path getting one character longer as the ids run past 999.
	if ratio := float64(large) / float64(small); ratio > 3 {
		t.Errorf("doubling the store multiplied message text by %.2f (%d -> %d bytes), want about 2",
			ratio, small, large)
	}
}

// TestAllocationGrowsLinearlyWithTheStore is the half of B-03 that message bytes
// cannot see.
//
// Step 7 assembles, for each member of a duplicate-id group, the list of that
// member's siblings — so the text it publishes is capped but the slices it built
// to get there were not. The published bytes stay linear either way, and only
// the allocator can tell the two apart: with the group walk bounded, one
// thousand files sharing an id allocate 7.5 MB and two thousand allocate 15.5;
// unbounded, the same stores allocate 48.6 MB and 234.
//
// TotalAlloc is cumulative bytes and is exact rather than sampled, so the ratio
// is a measurement and not an estimate. The threshold is the same 3 the message
// test uses, for the same reason: doubling is linear, quadrupling is quadratic,
// and neither can reach 3 from the other side.
func TestAllocationGrowsLinearlyWithTheStore(t *testing.T) {
	validator := shippedValidator(t)

	allocatedFor := func(n int) uint64 {
		records := make([]testRecord, 0, n)
		for k := 1; k <= n; k++ {
			// One id, n files. Every record is in one step-7 group, which is the
			// shape whose per-member sibling list was quadratic.
			records = append(records, decisionAt(fmt.Sprintf("DEC-%04d.json", k),
				decisionDoc("DEC-0001", nil)))
		}
		store := storeOf(t, records)

		var before, after runtime.MemStats
		runtime.GC()
		runtime.ReadMemStats(&before)
		findings := validate.Check(store, validator)
		runtime.ReadMemStats(&after)

		if len(findings) < n {
			t.Fatalf("%d files sharing one id produced %d findings, want at least one each", n, len(findings))
		}
		return after.TotalAlloc - before.TotalAlloc
	}

	small, large := allocatedFor(1000), allocatedFor(2000)
	if ratio := float64(large) / float64(small); ratio > 3 {
		t.Errorf("doubling the store multiplied allocation by %.2f (%d -> %d bytes), want about 2",
			ratio, small, large)
	}
}

// withSupersedes merges a supersedes list into a fixture's extra properties
// without the caller having to restate the map when it has nothing else to add.
func withSupersedes(extra map[string]any, targets ...any) map[string]any {
	merged := make(map[string]any, len(extra)+1)
	for key, value := range extra {
		merged[key] = value
	}
	merged["supersedes"] = targets
	return merged
}
