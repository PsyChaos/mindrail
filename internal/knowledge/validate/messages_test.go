package validate_test

import (
	"strings"
	"testing"

	"github.com/PsyChaos/mindrail/internal/knowledge/validate"
)

// fact is one thing a step's message has to say beyond naming the record it is
// attached to.
//
// AC-06.4 is the reason this table exists. status.componentFrom drops Diagnostic
// and Impact and keeps only the next action, and doctor builds that action out
// of the finding's message — so a fact that is not in the message is a fact the
// reader never sees. Every existing assertion about messages checked only that
// the finding's own path appears in its own message, which four of the seven
// steps satisfy while saying nothing a reader could act on: each of the messages
// below could be reduced to "<path> is wrong" and the whole suite stayed green.
type fact struct {
	step validate.Step
	name string
	// records is the store, and want is every substring the finding on subject
	// must contain.
	records []testRecord
	subject string
	want    []string
}

func facts() []fact {
	return []fact{
		{
			step: validate.StepFilenameConsistency,
			name: "step 6 names both the id it found and the one the file name claims",
			records: []testRecord{
				decisionAt("DEC-0001.json", decisionDoc("DEC-9999", nil)),
			},
			subject: ".mindrail/knowledge/decisions/DEC-0001.json",
			// Without both, the reader is told two things disagree and not what
			// either of them says — and the remedy is a choice between renaming
			// the file and changing the id.
			want: []string{`"DEC-9999"`, `"DEC-0001"`},
		},
		{
			step: validate.StepUniqueID,
			name: "step 7 names the id and the other file carrying it",
			records: []testRecord{
				decisionAt("DEC-0001.json", decisionDoc("DEC-0001", nil)),
				decisionAt("DEC-0002.json", decisionDoc("DEC-0001", nil)),
			},
			subject: ".mindrail/knowledge/decisions/DEC-0001.json",
			want:    []string{`"DEC-0001"`, ".mindrail/knowledge/decisions/DEC-0002.json"},
		},
		{
			step: validate.StepSupersedeTarget,
			name: "step 8 names the unresolved id and the file it would occupy",
			records: []testRecord{
				decisionAt("DEC-0002.json", decisionDoc("DEC-0002", map[string]any{
					"supersedes": []any{"DEC-0001"},
				})),
			},
			subject: ".mindrail/knowledge/decisions/DEC-0002.json",
			// The id is what the reader deletes from the supersedes array; the
			// path is what they create instead. A message with neither leaves both
			// halves of the remedy unstated.
			want: []string{`"DEC-0001"`, ".mindrail/knowledge/decisions/DEC-0001.json"},
		},
		{
			step: validate.StepSupersedeCycle,
			name: "step 9 names every other record in the closed group",
			records: []testRecord{
				decisionAt("DEC-0001.json", decisionDoc("DEC-0001", map[string]any{
					"supersedes": []any{"DEC-0002"},
				})),
				decisionAt("DEC-0002.json", decisionDoc("DEC-0002", map[string]any{
					"supersedes": []any{"DEC-0003"},
				})),
				decisionAt("DEC-0003.json", decisionDoc("DEC-0003", map[string]any{
					"supersedes": []any{"DEC-0001"},
				})),
			},
			subject: ".mindrail/knowledge/decisions/DEC-0001.json",
			// Asserted as one ordered string, not as three names. Tarjan pops a
			// component off its stack in reverse discovery order, so this group
			// leaves the traversal as "DEC-0003, DEC-0002, DEC-0001" — a correct
			// set, spelled backwards, and spelled differently again for a group
			// the walk entered elsewhere. Sorting the members is what makes the
			// list a property of the group, and an unordered assertion would not
			// notice its removal.
			want: []string{"DEC-0001, DEC-0002, DEC-0003"},
		},
		{
			step: validate.StepDuplicateActiveLineage,
			name: "step 10 names the other active records in the lineage",
			records: []testRecord{
				decisionAt("DEC-0001.json", decisionDoc("DEC-0001", nil)),
				decisionAt("DEC-0002.json", decisionDoc("DEC-0002", map[string]any{
					"supersedes": []any{"DEC-0001"},
				})),
			},
			subject: ".mindrail/knowledge/decisions/DEC-0001.json",
			// The remedy is "all but one of these carry status superseded", which
			// is a choice the reader cannot make without seeing the candidates or
			// knowing what to set.
			want: []string{".mindrail/knowledge/decisions/DEC-0002.json", "superseded"},
		},
		{
			step: validate.StepScopeSyntax,
			name: "step 11 says what is wrong with the scope and which step resolves targets",
			records: []testRecord{
				decisionAt("DEC-0001.json", decisionDoc("DEC-0001", map[string]any{
					"scope": map[string]any{"level": "FILE", "target": "../outside/thing.go"},
				})),
			},
			subject: ".mindrail/knowledge/decisions/DEC-0001.json",
			want:    []string{"step 12"},
		},
	}
}

// TestAStepsMessageCarriesTheFactsItsRemedyNeeds is AC-06.4 pushed back to where
// the text is produced.
func TestAStepsMessageCarriesTheFactsItsRemedyNeeds(t *testing.T) {
	validator := shippedValidator(t)

	for _, tt := range facts() {
		t.Run(tt.name, func(t *testing.T) {
			findings := findingsAt(validate.Check(storeOf(t, tt.records), validator), tt.step)

			var message string
			for _, finding := range findings {
				if finding.Path == tt.subject {
					message = finding.Message
				}
			}
			if message == "" {
				t.Fatalf("step %d reported nothing on %s:\n\t%s", tt.step, tt.subject, describe(findings))
			}

			for _, want := range tt.want {
				if !strings.Contains(message, want) {
					t.Errorf("step %d's message does not carry %q: %q", tt.step, want, message)
				}
			}
		})
	}
}

// TestEveryStepsMessageIsCovered keeps the table above from silently losing a
// row, the same way TestEveryStepIsRepresentedInTheDetectionTable does for the
// detections. A step whose message has no row is one whose facts can be deleted
// green.
func TestEveryStepsMessageIsCovered(t *testing.T) {
	covered := make(map[validate.Step]int, 7)
	for _, tt := range facts() {
		covered[tt.step]++
	}
	for step := validate.StepFilenameConsistency; step <= validate.StepScopeSyntax; step++ {
		if covered[step] == 0 {
			t.Errorf("step %d has no message-facts row, so its remedy can be emptied green", step)
		}
	}
	// Step 5 is deliberately absent: its message is the schema document's, and
	// TestStepFiveNamesTheOffendingFieldRatherThanTheRecord already pins the one
	// fact this package adds to it.
	if covered[validate.StepSchema] != 0 {
		t.Error("step 5's message belongs to the schema document (D-36), not to this table")
	}
}

// TestTwoFindingsOnOneRecordAreOrderedByTheirInstanceLocation is the
// reachability half of decision D-47's tie-break keys: a one-record store
// produces two findings that agree on both published sort keys, so the third key
// is what decides the report's order over real bytes.
//
// InstanceLocation is the key that does it, and it is the one a consumer cannot
// see: it is carried beside the finding and dropped before Check returns, so
// without this the whole third key was reachable only from inside the package.
// The pair below is ordered "/aa" before "/zz" while their messages happen to
// agree with that, which is why this test does not claim to pin the fourth key
// — sorted()'s own unit test does that, and it is the only thing that can.
//
// The fourth key, Message, decides only when two findings share a pointer as
// well. No shipped schema document produces two leaves at one instance location
// — every keyword that can fail at the same place was checked, and each yields
// one finding — so the Message key is a last resort against a future document
// rather than something a store can be written to reach today. It is kept
// because D-47 froze it and because a sort with a genuinely undecided pair would
// be publishing an order it did not choose.
func TestTwoFindingsOnOneRecordAreOrderedByTheirInstanceLocation(t *testing.T) {
	store := storeOf(t, []testRecord{
		decisionAt("DEC-0001.json", decisionDoc("DEC-0001", map[string]any{"aa": 1, "zz": 2})),
	})

	findings := validate.Check(store, shippedValidator(t))
	if len(findings) != 2 {
		t.Fatalf("want two step-5 findings on one record, got:\n\t%s", describe(findings))
	}
	if findings[0].Path != findings[1].Path || findings[0].Step != findings[1].Step {
		t.Fatalf("the two findings differ before the tie-break keys, so they do not reach them:\n\t%s",
			describe(findings))
	}
	if !strings.Contains(findings[0].Message, "'aa'") || !strings.Contains(findings[1].Message, "'zz'") {
		t.Errorf("two findings agreeing on path and step came out unordered:\n\t%q\n\t%q",
			findings[0].Message, findings[1].Message)
	}
}
