package validate_test

import (
	"fmt"
	"slices"
	"testing"

	"github.com/PsyChaos/mindrail/internal/knowledge/loader"
	"github.com/PsyChaos/mindrail/internal/knowledge/validate"
)

// suppression is decision D-45 stated as a pair of stores.
//
// The two halves are what make either half mean anything. The suppressed store
// on its own would pass under an implementation that had simply forgotten the
// step, so every row carries a proven store with the same shape in which the
// record the loader could not read *is* readable — and the condition fires. The
// only difference between the halves is whether one file could be read, and the
// verdict flips on exactly that.
type suppression struct {
	step validate.Step
	name string

	// proven is the store in which every record was read and the condition is
	// real, together with the record paths the step must report.
	proven    []testRecord
	wantPaths []string

	// suppressed is the same condition with one record moved out of the store
	// and into Problems. Nothing at all may be reported for it: the record
	// exists, this binary could not read it, and the loader has already named
	// the file.
	suppressed []testRecord
	problems   []loader.Problem
}

func suppressions() []suppression {
	return []suppression{
		{
			step: validate.StepSupersedeTarget,
			name: "a supersede target whose file the loader reported as a problem",
			proven: []testRecord{
				decisionAt("DEC-0002.json", decisionDoc("DEC-0002", map[string]any{
					"supersedes": []any{"DEC-0001"},
				})),
			},
			wantPaths: []string{".mindrail/knowledge/decisions/DEC-0002.json"},
			// Identical records. The only change is that the loader now says why
			// DEC-0001 is not in the store, which turns "it does not exist" into
			// "this binary could not read it".
			suppressed: []testRecord{
				decisionAt("DEC-0002.json", decisionDoc("DEC-0002", map[string]any{
					"supersedes": []any{"DEC-0001"},
				})),
			},
			problems: []loader.Problem{problemAt(loader.KindDecision, "DEC-0001.json")},
		},
		{
			step: validate.StepSupersedeCycle,
			name: "a chain that would close through an unreadable record",
			proven: []testRecord{
				decisionAt("DEC-0001.json", decisionDoc("DEC-0001", map[string]any{
					"supersedes": []any{"DEC-0002"},
				})),
				decisionAt("DEC-0002.json", decisionDoc("DEC-0002", map[string]any{
					"status":     "superseded",
					"supersedes": []any{"DEC-0003"},
				})),
				decisionAt("DEC-0003.json", decisionDoc("DEC-0003", map[string]any{
					"status":     "superseded",
					"supersedes": []any{"DEC-0001"},
				})),
			},
			wantPaths: []string{
				".mindrail/knowledge/decisions/DEC-0001.json",
				".mindrail/knowledge/decisions/DEC-0002.json",
				".mindrail/knowledge/decisions/DEC-0003.json",
			},
			// DEC-0003 is the record whose "supersedes" closes the walk. Without
			// it the chain has an end, and a cycle is a closed walk: an unverified
			// edge closes nothing.
			suppressed: []testRecord{
				decisionAt("DEC-0001.json", decisionDoc("DEC-0001", map[string]any{
					"supersedes": []any{"DEC-0002"},
				})),
				decisionAt("DEC-0002.json", decisionDoc("DEC-0002", map[string]any{
					"status":     "superseded",
					"supersedes": []any{"DEC-0003"},
				})),
			},
			problems: []loader.Problem{problemAt(loader.KindDecision, "DEC-0003.json")},
		},
		{
			step: validate.StepDuplicateActiveLineage,
			name: "a lineage whose connecting record is unreadable",
			proven: []testRecord{
				decisionAt("DEC-0001.json", decisionDoc("DEC-0001", nil)),
				decisionAt("DEC-0002.json", decisionDoc("DEC-0002", map[string]any{
					"status":     "superseded",
					"supersedes": []any{"DEC-0001"},
				})),
				decisionAt("DEC-0003.json", decisionDoc("DEC-0003", map[string]any{
					"supersedes": []any{"DEC-0002"},
				})),
			},
			wantPaths: []string{
				".mindrail/knowledge/decisions/DEC-0001.json",
				".mindrail/knowledge/decisions/DEC-0003.json",
			},
			// DEC-0002 is the record that joins DEC-0001 to DEC-0003. Without it
			// they are two lineages with one current record each, and the record
			// that would disambiguate them is the one this binary cannot read.
			suppressed: []testRecord{
				decisionAt("DEC-0001.json", decisionDoc("DEC-0001", nil)),
				decisionAt("DEC-0003.json", decisionDoc("DEC-0003", map[string]any{
					"supersedes": []any{"DEC-0002"},
				})),
			},
			problems: []loader.Problem{problemAt(loader.KindDecision, "DEC-0002.json")},
		},
		{
			step: validate.StepDuplicateActiveLineage,
			name: "two records superseding one unreadable record",
			proven: []testRecord{
				decisionAt("DEC-0001.json", decisionDoc("DEC-0001", nil)),
				decisionAt("DEC-0002.json", decisionDoc("DEC-0002", map[string]any{
					"supersedes": []any{"DEC-0001"},
				})),
				decisionAt("DEC-0003.json", decisionDoc("DEC-0003", map[string]any{
					"supersedes": []any{"DEC-0001"},
				})),
			},
			wantPaths: []string{
				".mindrail/knowledge/decisions/DEC-0001.json",
				".mindrail/knowledge/decisions/DEC-0002.json",
				".mindrail/knowledge/decisions/DEC-0003.json",
			},
			// The lineage the two survivors share exists only through DEC-0001,
			// and DEC-0001 is the record this binary could not read. A graph that
			// let an edge point at an id nothing resolved would join them anyway
			// and publish a fork nobody verified — which is the shape D-45's step
			// 10 clause forbids.
			suppressed: []testRecord{
				decisionAt("DEC-0002.json", decisionDoc("DEC-0002", map[string]any{
					"supersedes": []any{"DEC-0001"},
				})),
				decisionAt("DEC-0003.json", decisionDoc("DEC-0003", map[string]any{
					"supersedes": []any{"DEC-0001"},
				})),
			},
			problems: []loader.Problem{problemAt(loader.KindDecision, "DEC-0001.json")},
		},
	}
}

// TestAnUnreadableRecordSuppressesTheClaimItWouldHaveDecided is AC-03.7.
func TestAnUnreadableRecordSuppressesTheClaimItWouldHaveDecided(t *testing.T) {
	validator := shippedValidator(t)

	for _, tt := range suppressions() {
		t.Run(fmt.Sprintf("step %d %s", tt.step, tt.name), func(t *testing.T) {
			proven := validate.Check(storeOf(t, tt.proven), validator)
			if got := pathsAt(proven, tt.step); !slices.Equal(got, tt.wantPaths) {
				t.Fatalf("with every record readable, step %d reported %v, want %v\n\t%s",
					tt.step, got, tt.wantPaths, describe(proven))
			}

			suppressed := validate.Check(storeOf(t, tt.suppressed, tt.problems...), validator)
			if len(suppressed) != 0 {
				t.Errorf("with one record unreadable, Check still claimed:\n\t%s", describe(suppressed))
			}
		})
	}
}

// TestEveryCrossRecordStepThatCanBeSuppressedIsGuarded keeps the table above
// honest about its coverage. Decision D-45 names steps 8, 9 and 10; step 7 is
// exempt and has its own test below.
func TestEveryCrossRecordStepThatCanBeSuppressedIsGuarded(t *testing.T) {
	guarded := make(map[validate.Step]int, 3)
	for _, tt := range suppressions() {
		guarded[tt.step]++
	}
	for _, step := range []validate.Step{
		validate.StepSupersedeTarget,
		validate.StepSupersedeCycle,
		validate.StepDuplicateActiveLineage,
	} {
		if guarded[step] == 0 {
			t.Errorf("step %d is cross-record and has no D-45 suppression guard", step)
		}
	}
}

// TestStepSevenIsExemptFromTheSuppression is decision D-45's one exception,
// stated as the reason rather than as an exception.
//
// The other cross-record steps ask whether a record the loader could not read
// would change the verdict. Here it cannot: two files were read and both carry
// one id, and no unread third file can make that false. The plausible wrong
// implementation is a blanket "the set is incomplete, so skip the cross-record
// steps", and this store is what it would fail on.
func TestStepSevenIsExemptFromTheSuppression(t *testing.T) {
	store := storeOf(t,
		[]testRecord{
			decisionAt("DEC-0001.json", decisionDoc("DEC-0001", nil)),
			decisionAt("DEC-0002.json", decisionDoc("DEC-0001", nil)),
		},
		problemAt(loader.KindDecision, "DEC-0003.json"),
		problemAt(loader.KindInvariant, "INV-0001.json"),
	)

	findings := validate.Check(store, shippedValidator(t))
	want := []string{
		".mindrail/knowledge/decisions/DEC-0001.json",
		".mindrail/knowledge/decisions/DEC-0002.json",
	}
	if got := pathsAt(findings, validate.StepUniqueID); !slices.Equal(got, want) {
		t.Errorf("step 7 reported %v, want both records\n\t%s", got, describe(findings))
	}
}

// TestSuppressionKeysOnTheFileTheReferencedIdWouldOccupy is what stops the
// suppression from being a blanket amnesty.
//
// Decision D-45 suppresses a reference when a Problem names *that* record's
// file. A Problem naming some other file says nothing about this reference, and
// treating it as though it did would let one unreadable record hide every
// dangling supersede target in the store.
func TestSuppressionKeysOnTheFileTheReferencedIdWouldOccupy(t *testing.T) {
	store := storeOf(t,
		[]testRecord{
			decisionAt("DEC-0002.json", decisionDoc("DEC-0002", map[string]any{
				"supersedes": []any{"DEC-0001"},
			})),
		},
		// A different file, and an invariant with the numerically matching id, so
		// a suppression keyed on "some problem exists" or on the bare number
		// rather than on the expected path fails here.
		problemAt(loader.KindDecision, "DEC-0007.json"),
		problemAt(loader.KindInvariant, "INV-0001.json"),
	)

	findings := validate.Check(store, shippedValidator(t))
	want := []string{".mindrail/knowledge/decisions/DEC-0002.json"}
	if got := pathsAt(findings, validate.StepSupersedeTarget); !slices.Equal(got, want) {
		t.Errorf("step 8 reported %v, want the record with the dangling target\n\t%s",
			got, describe(findings))
	}
}
