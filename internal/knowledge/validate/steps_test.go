package validate_test

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/knowledge/loader"
	"github.com/PsyChaos/mindrail/internal/knowledge/validate"
)

// detection is one repository condition that exactly one step is meant to find.
//
// The table is keyed by Step so that AC-11.1 holds by construction: deleting a
// step's call from Check turns exactly the row named after that step red, and
// the subtest name is the step number. A table keyed by prose would fail
// somewhere the reader has to go looking for.
type detection struct {
	step validate.Step
	name string
	// records is the fixture store. Every row builds a whole store rather than
	// one record, because four of the seven steps are cross-record and a rule
	// about a set cannot be exercised by a singleton.
	records []testRecord
	// wantPaths is every record the step must report, sorted. Asserting the
	// exact set rather than "at least one" is what makes an over-firing step
	// fail here too: a step 7 that reported a record with a unique id would add
	// a path this table does not list.
	wantPaths []string
}

func detections() []detection {
	return []detection{
		{
			step: validate.StepSchema,
			name: "a required property is missing",
			records: []testRecord{
				decisionAt("DEC-0001.json", decisionDoc("DEC-0001", nil, "title")),
			},
			wantPaths: []string{".mindrail/knowledge/decisions/DEC-0001.json"},
		},
		{
			step: validate.StepFilenameConsistency,
			name: "the file name and the id disagree",
			records: []testRecord{
				decisionAt("DEC-0001.json", decisionDoc("DEC-9999", nil)),
			},
			wantPaths: []string{".mindrail/knowledge/decisions/DEC-0001.json"},
		},
		{
			step: validate.StepUniqueID,
			name: "two files carry one id",
			records: []testRecord{
				decisionAt("DEC-0001.json", decisionDoc("DEC-0001", nil)),
				decisionAt("DEC-0002.json", decisionDoc("DEC-0001", nil)),
			},
			wantPaths: []string{
				".mindrail/knowledge/decisions/DEC-0001.json",
				".mindrail/knowledge/decisions/DEC-0002.json",
			},
		},
		{
			step: validate.StepSupersedeTarget,
			name: "the superseded record is not in the store",
			records: []testRecord{
				decisionAt("DEC-0002.json", decisionDoc("DEC-0002", map[string]any{
					"supersedes": []any{"DEC-0001"},
				})),
			},
			wantPaths: []string{".mindrail/knowledge/decisions/DEC-0002.json"},
		},
		{
			step: validate.StepSupersedeCycle,
			name: "two records supersede each other",
			records: []testRecord{
				decisionAt("DEC-0001.json", decisionDoc("DEC-0001", map[string]any{
					"supersedes": []any{"DEC-0002"},
				})),
				decisionAt("DEC-0002.json", decisionDoc("DEC-0002", map[string]any{
					"supersedes": []any{"DEC-0001"},
				})),
			},
			wantPaths: []string{
				".mindrail/knowledge/decisions/DEC-0001.json",
				".mindrail/knowledge/decisions/DEC-0002.json",
			},
		},
		{
			step: validate.StepDuplicateActiveLineage,
			name: "both members of one lineage are active",
			records: []testRecord{
				decisionAt("DEC-0001.json", decisionDoc("DEC-0001", nil)),
				decisionAt("DEC-0002.json", decisionDoc("DEC-0002", map[string]any{
					"supersedes": []any{"DEC-0001"},
				})),
			},
			wantPaths: []string{
				".mindrail/knowledge/decisions/DEC-0001.json",
				".mindrail/knowledge/decisions/DEC-0002.json",
			},
		},
		{
			step: validate.StepScopeSyntax,
			name: "the scope target leaves the repository",
			records: []testRecord{
				invariantAt("INV-0001.json", invariantDoc("INV-0001", map[string]any{
					"scope": map[string]any{"level": "FILE", "target": "../outside/thing.go"},
				})),
			},
			wantPaths: []string{".mindrail/knowledge/invariants/INV-0001.json"},
		},
	}
}

// TestEachStepDetectsItsCondition is AC-03.4 and AC-11.1.
//
// Deleting any one of steps 5-11 from Check turns exactly one subtest red, and
// the subtest is named after the step number, so the failure says which rule
// stopped being enforced rather than only that something did.
func TestEachStepDetectsItsCondition(t *testing.T) {
	validator := shippedValidator(t)

	for _, tt := range detections() {
		t.Run(fmt.Sprintf("step %d %s", tt.step, tt.name), func(t *testing.T) {
			findings := validate.Check(storeOf(t, tt.records), validator)

			got := pathsAt(findings, tt.step)
			if !slices.Equal(got, tt.wantPaths) {
				t.Fatalf("step %d reported %v, want %v\nall findings:\n\t%s",
					tt.step, got, tt.wantPaths, describe(findings))
			}

			wantCode := app.CodeKnowledgeInvalid
			if tt.step == validate.StepSupersedeCycle {
				wantCode = app.CodeKnowledgeSupersedeCycle
			}
			for _, finding := range findingsAt(findings, tt.step) {
				if finding.Code != wantCode {
					t.Errorf("step %d finding on %s carries code %q, want %q",
						tt.step, finding.Path, finding.Code, wantCode)
				}
				if finding.Fatal != (tt.step == validate.StepSupersedeCycle) {
					t.Errorf("step %d finding on %s has Fatal = %v", tt.step, finding.Path, finding.Fatal)
				}
			}
		})
	}
}

// TestEveryStepIsRepresentedInTheDetectionTable stops the table above from
// silently losing a row.
//
// Without it, deleting a step and its row together would leave the suite green,
// which is the one way AC-11.1's mutation can be defeated. The bound is the two
// declared constants rather than a literal count, so a step added to the
// pipeline has to arrive with a row.
func TestEveryStepIsRepresentedInTheDetectionTable(t *testing.T) {
	covered := make(map[validate.Step]int, 7)
	for _, tt := range detections() {
		covered[tt.step]++
	}

	for step := validate.StepSchema; step <= validate.StepScopeSyntax; step++ {
		if covered[step] == 0 {
			t.Errorf("step %d has no row in detections(), so deleting it would not fail anything", step)
		}
	}
	for step := range covered {
		if step < validate.StepSchema || step > validate.StepScopeSyntax {
			t.Errorf("detections() names step %d, which the pipeline does not have", step)
		}
	}
}

// overFire is one healthy repository condition adjacent to a detection, with the
// step that must not read it as a failure (AC-11.2).
type overFire struct {
	step    validate.Step
	name    string
	records []testRecord
}

func overFires() []overFire {
	return []overFire{
		{
			step: validate.StepSchema,
			name: "a record carrying every optional property its schema allows",
			records: []testRecord{
				decisionAt("DEC-0001.json", decisionDoc("DEC-0001", map[string]any{
					"context":      "The situation that forced the choice.",
					"consequences": "What it costs.",
					"tags":         []any{"alpha", "beta"},
					"scope":        map[string]any{"level": "PACKAGE", "target": "internal/app"},
				})),
			},
		},
		{
			step: validate.StepFilenameConsistency,
			name: "the file name and the id agree",
			records: []testRecord{
				decisionAt("DEC-0001.json", decisionDoc("DEC-0001", nil)),
				invariantAt("INV-0001.json", invariantDoc("INV-0001", nil)),
			},
		},
		{
			step: validate.StepUniqueID,
			name: "a second record with a different id",
			records: []testRecord{
				decisionAt("DEC-0001.json", decisionDoc("DEC-0001", nil)),
				decisionAt("DEC-0002.json", decisionDoc("DEC-0002", nil)),
			},
		},
		{
			step: validate.StepSupersedeTarget,
			name: "the superseded record is in the store",
			records: []testRecord{
				decisionAt("DEC-0001.json", decisionDoc("DEC-0001", map[string]any{
					"status": "superseded",
				})),
				decisionAt("DEC-0002.json", decisionDoc("DEC-0002", map[string]any{
					"supersedes": []any{"DEC-0001"},
				})),
			},
		},
		{
			step: validate.StepSupersedeCycle,
			name: "a legitimate three-record lineage",
			records: []testRecord{
				decisionAt("DEC-0001.json", decisionDoc("DEC-0001", map[string]any{
					"status": "superseded",
				})),
				decisionAt("DEC-0002.json", decisionDoc("DEC-0002", map[string]any{
					"status":     "superseded",
					"supersedes": []any{"DEC-0001"},
				})),
				decisionAt("DEC-0003.json", decisionDoc("DEC-0003", map[string]any{
					"supersedes": []any{"DEC-0002"},
				})),
			},
		},
		{
			step: validate.StepDuplicateActiveLineage,
			name: "one lineage with one current record",
			records: []testRecord{
				decisionAt("DEC-0001.json", decisionDoc("DEC-0001", map[string]any{
					"status": "superseded",
				})),
				decisionAt("DEC-0002.json", decisionDoc("DEC-0002", map[string]any{
					"status":     "superseded",
					"supersedes": []any{"DEC-0001"},
				})),
				decisionAt("DEC-0003.json", decisionDoc("DEC-0003", map[string]any{
					"supersedes": []any{"DEC-0002"},
				})),
			},
		},
		{
			step: validate.StepDuplicateActiveLineage,
			name: "two separate lineages each with one current record",
			records: []testRecord{
				decisionAt("DEC-0001.json", decisionDoc("DEC-0001", nil)),
				decisionAt("DEC-0002.json", decisionDoc("DEC-0002", nil)),
			},
		},
		{
			step: validate.StepScopeSyntax,
			name: "a PROJECT scope with no target",
			records: []testRecord{
				decisionAt("DEC-0001.json", decisionDoc("DEC-0001", map[string]any{
					"scope": map[string]any{"level": "PROJECT"},
				})),
			},
		},
		{
			step: validate.StepScopeSyntax,
			name: "a repository-relative target naming a file that does not exist",
			records: []testRecord{
				// AC-03.5: resolution is step 12 and needs MR-005's index. A
				// syntactically valid target pointing at nothing is not a finding,
				// and this row is what stops step 11 from growing into step 12.
				decisionAt("DEC-0001.json", decisionDoc("DEC-0001", map[string]any{
					"scope": map[string]any{"level": "FILE", "target": "internal/nothing/here.go"},
				})),
			},
		},
		{
			step: validate.StepScopeSyntax,
			name: "a SYMBOL scope naming a symbol reference",
			records: []testRecord{
				invariantAt("INV-0001.json", invariantDoc("INV-0001", map[string]any{
					"scope": map[string]any{"level": "SYMBOL", "target": "internal/app.Code"},
				})),
			},
		},
	}
}

// TestNoStepFiresOnTheAdjacentHealthyCondition is AC-11.2.
//
// Each row asserts the whole store is clean, not merely that the named step is
// quiet: a guard that let another step fire would be describing a repository
// this pipeline still rejects, and "healthy" would be doing no work.
func TestNoStepFiresOnTheAdjacentHealthyCondition(t *testing.T) {
	validator := shippedValidator(t)

	for _, tt := range overFires() {
		t.Run(fmt.Sprintf("step %d %s", tt.step, tt.name), func(t *testing.T) {
			findings := validate.Check(storeOf(t, tt.records), validator)

			if got := findingsAt(findings, tt.step); len(got) != 0 {
				t.Errorf("step %d fired on a healthy store:\n\t%s", tt.step, describe(got))
			}
			if len(findings) != 0 {
				t.Errorf("the store is meant to be healthy but produced:\n\t%s", describe(findings))
			}
		})
	}
}

// TestEveryStepHasAnOverFireGuard keeps the guards and the detections in step
// with each other. A detection added without one is a rule nobody has proved
// stays quiet on a working repository.
func TestEveryStepHasAnOverFireGuard(t *testing.T) {
	guarded := make(map[validate.Step]int, 7)
	for _, tt := range overFires() {
		guarded[tt.step]++
	}
	for step := validate.StepSchema; step <= validate.StepScopeSyntax; step++ {
		if guarded[step] == 0 {
			t.Errorf("step %d has no over-fire guard", step)
		}
	}
}

// TestARecordThatFailsStepFiveIsNotAskedTheLaterSteps is decision D-40's first
// half.
//
// The record below would trip steps 6, 8 and 11 if the later steps were asked:
// its file name disagrees with its id, it supersedes a record that is not there,
// and its scope target is spelled with backslashes. It must produce step 5 and
// nothing else, because steps 6-11 read fields step 5 has just said are wrong.
func TestARecordThatFailsStepFiveIsNotAskedTheLaterSteps(t *testing.T) {
	store := storeOf(t, []testRecord{
		decisionAt("DEC-0001.json", decisionDoc("DEC-9999", map[string]any{
			"supersedes": []any{"DEC-4242"},
			"scope":      map[string]any{"level": "FILE", "target": `internal\app\code.go`},
		}, "decision")),
	})

	findings := validate.Check(store, shippedValidator(t))
	got := stepsAt(findings, ".mindrail/knowledge/decisions/DEC-0001.json")
	want := []validate.Step{validate.StepSchema}

	if !slices.Equal(got, want) {
		t.Errorf("steps reported = %v, want %v\nall findings:\n\t%s", got, want, describe(findings))
	}
}

// TestOtherRecordsAreStillAskedEveryStep is decision D-40's second half and
// AC-03.3.
//
// One schema-invalid record and one cycle, in one store, reported in one run. A
// pipeline that stopped at the first bad record would make a reader fix their
// store one problem at a time and re-run between each.
func TestOtherRecordsAreStillAskedEveryStep(t *testing.T) {
	store := storeOf(t, []testRecord{
		decisionAt("DEC-0001.json", decisionDoc("DEC-0001", map[string]any{
			"supersedes": []any{"DEC-0002"},
		})),
		decisionAt("DEC-0002.json", decisionDoc("DEC-0002", map[string]any{
			"supersedes": []any{"DEC-0001"},
		})),
		invariantAt("INV-0001.json", invariantDoc("INV-0001", nil, "statement")),
	})

	findings := validate.Check(store, shippedValidator(t))

	if got := stepsAt(findings, ".mindrail/knowledge/invariants/INV-0001.json"); !slices.Equal(got, []validate.Step{validate.StepSchema}) {
		t.Errorf("the schema-invalid record reported steps %v, want [5]\n\t%s", got, describe(findings))
	}

	cycled := pathsAt(findings, validate.StepSupersedeCycle)
	want := []string{
		".mindrail/knowledge/decisions/DEC-0001.json",
		".mindrail/knowledge/decisions/DEC-0002.json",
	}
	if !slices.Equal(cycled, want) {
		t.Errorf("step 9 reported %v, want %v\n\t%s", cycled, want, describe(findings))
	}
}

// TestStepFiveNamesTheOffendingFieldRatherThanTheRecord is AC-11.3 applied to
// the one step whose finding is about a value inside a record.
//
// The pointer is what a reader edits. A message that named only the file would
// be true of every schema violation in it, which is exactly the substring both
// the right and the wrong answer contain.
func TestStepFiveNamesTheOffendingFieldRatherThanTheRecord(t *testing.T) {
	store := storeOf(t, []testRecord{
		decisionAt("DEC-0001.json", decisionDoc("DEC-0001", map[string]any{
			"created_at": "yesterday",
		})),
	})

	findings := findingsAt(validate.Check(store, shippedValidator(t)), validate.StepSchema)
	if len(findings) != 1 {
		t.Fatalf("step 5 reported %d findings, want 1:\n\t%s", len(findings), describe(findings))
	}
	if !strings.Contains(findings[0].Message, "/created_at") {
		t.Errorf("step 5 message does not name the offending pointer: %q", findings[0].Message)
	}
}

// TestStepFiveReportsEveryViolationInOneRecord keeps this package from
// narrowing what the schema package found.
//
// AC-02.6 makes schema.Validate walk the Causes tree to its leaves so that a
// reader fixing a record sees every rule it breaks in one run. That work is
// undone by a step 5 that reports the first violation and moves on, and the
// count alone would not notice: the assertion is that both offending pointers
// are named, so dropping either one fails.
func TestStepFiveReportsEveryViolationInOneRecord(t *testing.T) {
	store := storeOf(t, []testRecord{
		decisionAt("DEC-0001.json", decisionDoc("DEC-0001", map[string]any{
			"created_at": "yesterday",
			"tags":       []any{"repeated", "repeated"},
		})),
	})

	findings := findingsAt(validate.Check(store, shippedValidator(t)), validate.StepSchema)

	for _, pointer := range []string{"/created_at", "/tags"} {
		named := false
		for _, finding := range findings {
			if strings.Contains(finding.Message, pointer) {
				named = true
			}
		}
		if !named {
			t.Errorf("no step 5 finding names %s:\n\t%s", pointer, describe(findings))
		}
	}
	if len(findings) != 2 {
		t.Errorf("step 5 reported %d findings, want one per broken rule:\n\t%s", len(findings), describe(findings))
	}
}

// TestAKindThatDisagreesWithItsDirectoryIsReportedByStepFive records where step
// 6's kind half actually lives.
//
// The loader takes a record's kind from the directory it walked
// (TestLoadKindComesFromTheDirectory), and step 5 validates it against that
// kind's document, whose "kind" property is a const. So a record filed among the
// invariants that calls itself a decision fails step 5 — reported against the
// document, which decision D-36 makes the contract — rather than being caught a
// second time in Go. The assertion is the Step, so an implementation that moved
// the rule into step 6 fails here rather than passing on a shared phrase.
func TestAKindThatDisagreesWithItsDirectoryIsReportedByStepFive(t *testing.T) {
	store := storeOf(t, []testRecord{
		invariantAt("INV-0001.json", invariantDoc("INV-0001", map[string]any{
			"kind": "decision",
		})),
	})

	findings := validate.Check(store, shippedValidator(t))
	got := stepsAt(findings, ".mindrail/knowledge/invariants/INV-0001.json")

	if !slices.Contains(got, validate.StepSchema) {
		t.Errorf("steps reported = %v, want step 5 among them\n\t%s", got, describe(findings))
	}
	if slices.Contains(got, validate.StepFilenameConsistency) {
		t.Errorf("step 6 also fired; the kind rule belongs to the schema document (D-36)\n\t%s", describe(findings))
	}
}

// TestAScopeLevelOutsideTheEnumIsReportedByStepFive is the other half of the
// same boundary, and is recorded rather than hidden.
//
// AC-03.4 lists "a scope whose level is not one of the five" among step 11's
// conditions, and record.Scope.Validate does refuse it — but both schema
// documents pin the level with an enum, so decision D-40 means step 11 is never
// asked about a record whose level is wrong. Step 5 gets there first. The
// condition is detected; the step that reports it is 5.
func TestAScopeLevelOutsideTheEnumIsReportedByStepFive(t *testing.T) {
	store := storeOf(t, []testRecord{
		invariantAt("INV-0001.json", invariantDoc("INV-0001", map[string]any{
			"scope": map[string]any{"level": "GALAXY", "target": "internal/app"},
		})),
	})

	findings := validate.Check(store, shippedValidator(t))
	got := stepsAt(findings, ".mindrail/knowledge/invariants/INV-0001.json")

	if !slices.Equal(got, []validate.Step{validate.StepSchema}) {
		t.Errorf("steps reported = %v, want [5]\n\t%s", got, describe(findings))
	}
}

// TestStepElevenDetectsAMissingTargetAsWellAsABadlySpelledOne keeps the second
// of step 11's three conditions from being carried only by the detection table's
// single row.
//
// The schema requires nothing but "level" inside a scope, so a MODULE scope with
// no target satisfies step 5 and reaches step 11 — which is what makes this
// condition reachable at all, unlike the level enum above.
func TestStepElevenDetectsAMissingTargetAsWellAsABadlySpelledOne(t *testing.T) {
	store := storeOf(t, []testRecord{
		decisionAt("DEC-0001.json", decisionDoc("DEC-0001", map[string]any{
			"scope": map[string]any{"level": "MODULE"},
		})),
	})

	findings := validate.Check(store, shippedValidator(t))
	if got := pathsAt(findings, validate.StepScopeSyntax); !slices.Equal(got, []string{".mindrail/knowledge/decisions/DEC-0001.json"}) {
		t.Errorf("step 11 reported %v, want the record with the targetless MODULE scope\n\t%s", got, describe(findings))
	}
}

// TestStepElevenSaysItJudgesSyntaxOnly is AC-03.5.
//
// The message has to carry the boundary, because a reader who reads "invalid
// scope" and knows the file is not there will go looking for the wrong problem.
// Step 12 is the one that resolves a target, and it needs MR-005's index.
func TestStepElevenSaysItJudgesSyntaxOnly(t *testing.T) {
	store := storeOf(t, []testRecord{
		decisionAt("DEC-0001.json", decisionDoc("DEC-0001", map[string]any{
			"scope": map[string]any{"level": "FILE", "target": "../outside/thing.go"},
		})),
	})

	findings := findingsAt(validate.Check(store, shippedValidator(t)), validate.StepScopeSyntax)
	if len(findings) != 1 {
		t.Fatalf("step 11 reported %d findings, want 1:\n\t%s", len(findings), describe(findings))
	}
	if !strings.Contains(findings[0].Message, "step 12") {
		t.Errorf("step 11's message does not name the step that resolves a target: %q", findings[0].Message)
	}
}

// TestStepSevenNamesBothOffendingFiles is the half of AC-03.4's duplicate-id
// rule a path-set assertion cannot reach: which file the reader has to look at
// besides this one. A remedy that named only the record it was attached to would
// leave the choice between two files undecidable.
func TestStepSevenNamesBothOffendingFiles(t *testing.T) {
	store := storeOf(t, []testRecord{
		decisionAt("DEC-0001.json", decisionDoc("DEC-0001", nil)),
		decisionAt("DEC-0002.json", decisionDoc("DEC-0001", nil)),
	})

	findings := findingsAt(validate.Check(store, shippedValidator(t)), validate.StepUniqueID)
	if len(findings) != 2 {
		t.Fatalf("step 7 reported %d findings, want one per record:\n\t%s", len(findings), describe(findings))
	}

	for _, finding := range findings {
		other := ".mindrail/knowledge/decisions/DEC-0002.json"
		if finding.Path == other {
			other = ".mindrail/knowledge/decisions/DEC-0001.json"
		}
		if !strings.Contains(finding.Message, other) {
			t.Errorf("the finding on %s does not name %s: %q", finding.Path, other, finding.Message)
		}
	}
}

// TestTwoFilesCarryingOneIdAreNotAlsoADuplicateActiveLineage is step 10's
// over-fire guard against step 7's condition.
//
// Two files under one id are one node in the lineage graph, not two records in
// one lineage. Counting them as two would give one condition two diagnoses and
// would fire on the very fixture MR-001 left behind — the duplicate DEC-9999 in
// TestLoadDoesNotPerformMR002Validation — which is a repository with a naming
// mistake, not a forked lineage.
func TestTwoFilesCarryingOneIdAreNotAlsoADuplicateActiveLineage(t *testing.T) {
	store := storeOf(t, []testRecord{
		decisionAt("DEC-0001.json", decisionDoc("DEC-0002", nil)),
		decisionAt("DEC-0003.json", decisionDoc("DEC-0002", nil)),
	})

	findings := validate.Check(store, shippedValidator(t))
	want := []validate.Step{validate.StepFilenameConsistency, validate.StepUniqueID}

	for _, recordPath := range []string{
		".mindrail/knowledge/decisions/DEC-0001.json",
		".mindrail/knowledge/decisions/DEC-0003.json",
	} {
		if got := stepsAt(findings, recordPath); !slices.Equal(got, want) {
			t.Errorf("%s reported steps %v, want %v\n\t%s", recordPath, got, want, describe(findings))
		}
	}
}

// TestACycleIsReportedFromItsSmallestMemberNotFromWhereTheWalkEnteredIt pins the
// rotation decision D-47's determinism rests on for step 9.
//
// DEC-0001 is not in the cycle; it points into it. The traversal therefore
// enters the cycle at DEC-0003 and closes it at DEC-0002, so an unrotated chain
// would read "DEC-0003 -> DEC-0002 -> DEC-0003" — a correct description of the
// same cycle, spelled differently depending on which record happened to be
// walked first. Rotation makes the message a property of the cycle.
//
// It is also the over-fire guard for the cycle detection itself: a record that
// merely references a cycle is not part of one, and DEC-0001 must not be
// reported.
func TestACycleIsReportedFromItsSmallestMemberNotFromWhereTheWalkEnteredIt(t *testing.T) {
	store := storeOf(t, []testRecord{
		decisionAt("DEC-0001.json", decisionDoc("DEC-0001", map[string]any{
			"supersedes": []any{"DEC-0003"},
		})),
		decisionAt("DEC-0002.json", decisionDoc("DEC-0002", map[string]any{
			"supersedes": []any{"DEC-0003"},
		})),
		decisionAt("DEC-0003.json", decisionDoc("DEC-0003", map[string]any{
			"supersedes": []any{"DEC-0002"},
		})),
	})

	findings := findingsAt(validate.Check(store, shippedValidator(t)), validate.StepSupersedeCycle)

	want := []string{
		".mindrail/knowledge/decisions/DEC-0002.json",
		".mindrail/knowledge/decisions/DEC-0003.json",
	}
	if got := pathsAt(findings, validate.StepSupersedeCycle); !slices.Equal(got, want) {
		t.Fatalf("step 9 reported %v, want only the two records in the cycle\n\t%s", got, describe(findings))
	}

	// Every member carries the same chain, so a reader comparing two findings
	// sees one cycle rather than two rotations of it.
	wantChain := "DEC-0002 -> DEC-0003 -> DEC-0002"
	for _, finding := range findings {
		if !strings.Contains(finding.Message, wantChain) {
			t.Errorf("the finding on %s does not carry the rotated chain %q: %q",
				finding.Path, wantChain, finding.Message)
		}
	}
}

// TestARecordThatSupersedesItselfIsACycleOfOne keeps step 9's closed walk of
// length one reachable. record.NewDecision refuses to author one, so the only
// way it reaches a repository is by hand — which is exactly the case a validator
// exists for.
func TestARecordThatSupersedesItselfIsACycleOfOne(t *testing.T) {
	store := storeOf(t, []testRecord{
		decisionAt("DEC-0001.json", decisionDoc("DEC-0001", map[string]any{
			"supersedes": []any{"DEC-0001"},
		})),
	})

	findings := findingsAt(validate.Check(store, shippedValidator(t)), validate.StepSupersedeCycle)
	if len(findings) != 1 {
		t.Fatalf("step 9 reported %d findings, want 1:\n\t%s", len(findings), describe(findings))
	}
	if !findings[0].Fatal {
		t.Error("a self-supersede is a lineage with no end, so its finding is fatal (D-39)")
	}
	if findings[0].Code != app.CodeKnowledgeSupersedeCycle {
		t.Errorf("code = %q, want %q", findings[0].Code, app.CodeKnowledgeSupersedeCycle)
	}
}

// TestFindingsAreSortedByPathThenStep is decision D-47's ordering, asserted on
// the two keys a consumer can observe.
func TestFindingsAreSortedByPathThenStep(t *testing.T) {
	findings := validate.Check(everyConditionStore(t), shippedValidator(t))
	if len(findings) < 2 {
		t.Fatalf("the fixture produced %d findings; an ordering over one is not a test", len(findings))
	}

	for i := 1; i < len(findings); i++ {
		previous, current := findings[i-1], findings[i]
		switch {
		case previous.Path > current.Path:
			t.Errorf("finding %d (%s) sorts before %d (%s)", i-1, previous.Path, i, current.Path)
		case previous.Path == current.Path && previous.Step > current.Step:
			t.Errorf("on %s, step %d sorts before step %d", current.Path, previous.Step, current.Step)
		}
	}
}

// TestCheckIgnoresRecordsTheLoaderRefused is decision D-38's boundary.
//
// A Problem means "this binary could not read the record", and there are no
// bytes for a finding to judge. Reporting one anyway would give one condition
// two diagnoses, and the loader has already named the file.
func TestCheckIgnoresRecordsTheLoaderRefused(t *testing.T) {
	store := storeOf(t,
		[]testRecord{decisionAt("DEC-0001.json", decisionDoc("DEC-0001", nil))},
		problemAt(loader.KindDecision, "DEC-0002.json"),
	)

	if findings := validate.Check(store, shippedValidator(t)); len(findings) != 0 {
		t.Errorf("Check reported findings for a store whose only fault is a loader Problem:\n\t%s",
			describe(findings))
	}
}
