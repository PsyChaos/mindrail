package validate_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/PsyChaos/mindrail/internal/knowledge/validate"
)

// These are the adjacent healthy cases for the FIX-A wave, written against the
// pipeline from the outside rather than derived from the fix.
//
// MR-001's most repeated failure was over-fire: a widened classification that
// refuses a working configuration takes a repository down as thoroughly as one
// that lets a bad value through, and F03's fix and its own correction were the
// same mistake pointing in opposite directions. FIX-A widened two things at
// once — the graph now holds every record the loader read, and closedGroups now
// reports whole strongly connected components — so the question these tests ask
// is not "does the detection still fire" but "does it now fire on a store that
// is fine".

// TestALegitimateThreeRecordSupersedeChainIsSilent is the healthy case the whole
// of steps 8, 9 and 10 has to leave alone.
//
// Three records, two supersede edges, one active record at the head and two
// carrying status superseded behind it. This is the shape spec §93 describes as
// the normal outcome of two Supersede calls, and it exercises every widened path
// at once: the graph holds all three, the lineage is one weakly connected
// component of three (step 10's input), the walk from the head reaches both
// others without closing (step 9's input), and both targets resolve (step 8's).
//
// A single finding here would mean the fixes had made an ordinary lineage
// unreportable, which is the failure that costs a repository more than the
// defect being fixed.
func TestALegitimateThreeRecordSupersedeChainIsSilent(t *testing.T) {
	store := storeOf(t, []testRecord{
		decisionAt("DEC-0001.json", decisionDoc("DEC-0001", map[string]any{
			"status": "superseded",
		})),
		decisionAt("DEC-0002.json", decisionDoc("DEC-0002", map[string]any{
			"status":     "superseded",
			"supersedes": []any{"DEC-0001"},
		})),
		decisionAt("DEC-0003.json", decisionDoc("DEC-0003", map[string]any{
			"status":     "active",
			"supersedes": []any{"DEC-0002"},
		})),
	})

	if findings := validate.Check(store, shippedValidator(t)); len(findings) != 0 {
		for _, finding := range findings {
			t.Errorf("step %d reported %s: %s", finding.Step, finding.Path, finding.Message)
		}
		t.Fatalf("a legitimate three-record chain produced %d findings, want none", len(findings))
	}
}

// TestALongerLegitimateChainIsAlsoSilent is the same case past the point where
// the widened code paths change shape.
//
// Three records is short enough that a component of the graph and a lineage of
// the graph are hard to tell apart, and short enough to stay under the
// ten-record cap the message builders apply. Twelve is past both. A chain this
// long is also what an actively maintained decision looks like after a year.
func TestALongerLegitimateChainIsAlsoSilent(t *testing.T) {
	const links = 12

	records := make([]testRecord, 0, links)
	for k := 1; k <= links; k++ {
		id := fmt.Sprintf("DEC-%04d", k)
		extra := map[string]any{"status": "superseded"}
		if k == links {
			extra["status"] = "active"
		}
		if k > 1 {
			extra["supersedes"] = []any{fmt.Sprintf("DEC-%04d", k-1)}
		}
		records = append(records, decisionAt(id+".json", decisionDoc(id, extra)))
	}

	if findings := validate.Check(storeOf(t, records), shippedValidator(t)); len(findings) != 0 {
		for _, finding := range findings {
			t.Errorf("step %d reported %s: %s", finding.Step, finding.Path, finding.Message)
		}
		t.Fatalf("a legitimate %d-record chain produced %d findings, want none", links, len(findings))
	}
}

// TestARecordWhoseSupersedesIsWhatStepFiveRejectedGetsOnlyAStepFiveFinding is
// the second half of FIX-A's over-fire surface, and the harder half.
//
// The fix put schema-invalid records back into the lineage graph so their edges
// survive. The risk that creates is the mirror image of the defect: a record
// whose "supersedes" is *itself* the thing step 5 refused now contributes edges
// drawn from a field the report has just called wrong, and those edges could
// produce a step 8, 9 or 10 finding built on a value nobody should trust.
//
// D-40 answers it, and each row here is one way the field can be wrong:
//
//   - a duplicate entry, which is valid JSON and decodes, so the edge is real
//     and points at a record that exists;
//   - a badly spelled id, which decodes into a target that resolves to nothing —
//     the shape that would otherwise become a step-8 "there is no such record";
//   - the wrong JSON type, which does not decode at all, so no edge exists and
//     the code has to reach the same answer down a different path.
//
// In every row the record must receive its step-5 finding and nothing else, and
// the healthy record beside it must receive nothing at all.
func TestARecordWhoseSupersedesIsWhatStepFiveRejectedGetsOnlyAStepFiveFinding(t *testing.T) {
	const offender = ".mindrail/knowledge/decisions/DEC-0002.json"

	tests := []struct {
		name string
		// extra is DEC-0002's document beyond id, kind and created_at. Every row
		// must be refused by step 5 and must also carry a "supersedes" the later
		// steps would have had an opinion about.
		extra map[string]any
	}{
		{
			name:  "a repeated entry, which its schema marks uniqueItems",
			extra: map[string]any{"status": "active", "supersedes": []any{"DEC-0001", "DEC-0001"}},
		},
		{
			name:  "an id that does not match the pattern",
			extra: map[string]any{"status": "active", "supersedes": []any{"dec-1"}},
		},
		{
			// The "supersedes" here is perfectly well formed and names a record
			// nothing in this store holds, which is step 8's exact condition.
			// The record fails step 5 on an unrelated property, so step 8 must
			// not be asked — this is the row where D-40 and D-45 could most
			// easily be got backwards.
			name:  "a well formed target nothing holds, on a record refused for another reason",
			extra: map[string]any{"status": "active", "supersedes": []any{"DEC-0009"}, "surprise": true},
		},
		{
			name:  "a string where the schema declares an array",
			extra: map[string]any{"status": "active", "supersedes": "DEC-0001"},
		},
		{
			name:  "an array of the wrong element type",
			extra: map[string]any{"status": "active", "supersedes": []any{1, 2}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := storeOf(t, []testRecord{
				decisionAt("DEC-0001.json", decisionDoc("DEC-0001", map[string]any{"status": "superseded"})),
				decisionAt("DEC-0002.json", decisionDoc("DEC-0002", tt.extra)),
			})

			findings := validate.Check(store, shippedValidator(t))
			if len(findings) == 0 {
				t.Fatal("no finding at all: step 5 has to refuse this record")
			}

			for _, finding := range findings {
				if finding.Step != validate.StepSchema {
					t.Errorf("step %d reported %s: %s\n\tD-40 did not ask this record steps 6-11, "+
						"and the answer would have been computed over the field step 5 has just refused",
						finding.Step, finding.Path, finding.Message)
				}
				if finding.Path != offender {
					t.Errorf("%s was reported, but only DEC-0002 is wrong: %s", finding.Path, finding.Message)
				}
				if finding.Fatal {
					t.Errorf("%s was reported as fatal: %s; D-39 makes only a supersede cycle fatal",
						finding.Path, finding.Message)
				}
			}
		})
	}
}

// TestTheHealthyRecordBesideARejectedOneIsStillFullyJudged is the guard against
// answering the over-fire question the other way.
//
// "No finding against the rejected record" must not become "no finding computed
// over its edges", because D-40's own words are that a rejected record is not
// asked steps 6-11 while every other record still is. The store below is the
// regression case F-R1 was about, reduced to two records: DEC-0002 breaks its
// schema on a property no step reads, and the cycle it is part of has to remain
// fatal for DEC-0001.
func TestTheHealthyRecordBesideARejectedOneIsStillFullyJudged(t *testing.T) {
	store := storeOf(t, []testRecord{
		decisionAt("DEC-0001.json", decisionDoc("DEC-0001", map[string]any{
			"supersedes": []any{"DEC-0002"},
		})),
		decisionAt("DEC-0002.json", decisionDoc("DEC-0002", map[string]any{
			"supersedes": []any{"DEC-0001"},
			// Not a property any cross-record step reads, and not one the
			// document allows either.
			"surprise": true,
		})),
	})

	findings := validate.Check(store, shippedValidator(t))

	cycles := findingsAt(findings, validate.StepSupersedeCycle)
	if len(cycles) != 1 {
		t.Fatalf("step 9 reported %d findings, want exactly one — against DEC-0001, which was asked", len(cycles))
	}
	if want := ".mindrail/knowledge/decisions/DEC-0001.json"; cycles[0].Path != want {
		t.Errorf("step 9 reported %s, want %s", cycles[0].Path, want)
	}
	if !cycles[0].Fatal {
		t.Error("the cycle was not reported as fatal; D-39 is the milestone's only fatal check")
	}
	// The member list has to name both records even though only one of them was
	// asked: the reader breaking the cycle needs to know which files are on the
	// table, and the other one is where they will probably do it.
	if !strings.Contains(cycles[0].Message, "DEC-0002") {
		t.Errorf("the cycle finding does not name the other member: %q", cycles[0].Message)
	}
}
