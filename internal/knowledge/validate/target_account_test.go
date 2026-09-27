package validate_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/PsyChaos/mindrail/internal/knowledge/loader"
	"github.com/PsyChaos/mindrail/internal/knowledge/validate"
)

// This file is about the three different things step 8 can know about the file a
// referenced id would occupy, and the three different sentences they warrant.
//
// The set used to be two-valued — "this run named the file" or not — and both
// values were wrong once. Keyed only on the loader's Problems it published "it
// is not there" about files the same report told the reader to correct (round
// 1's F-R2). Widened to every file the loader named at all, it went silent about
// a reference nothing in the store carries merely because a file of that name
// had been read (round 2's B-A5). What a suppression set cannot carry, a message
// can: the file exists, and it holds a different record.

// TestStepEightSaysWhichIdTheExpectedFileActuallyCarries is the regression for
// B-A5.
//
// DEC-0002 supersedes DEC-0009. A file called DEC-0009.json was read — and it
// declares itself DEC-0007. Nothing in the store carries DEC-0009, so the
// supersede resolves to nothing and the reader's lineage has a hole in it. Step
// 6 tells them the file's name and id disagree, which is a true and different
// fact; it does not tell them their supersede points at nobody.
//
// Measured before the fix: this store produced one finding (step 6). The step-8
// diagnosis was suppressed because a file at the expected path had been read.
func TestStepEightSaysWhichIdTheExpectedFileActuallyCarries(t *testing.T) {
	store := storeOf(t, []testRecord{
		decisionAt("DEC-0002.json", decisionDoc("DEC-0002", map[string]any{
			"supersedes": []any{"DEC-0009"},
		})),
		decisionAt("DEC-0009.json", decisionDoc("DEC-0007", map[string]any{
			"status": "superseded",
		})),
	})

	findings := validate.Check(store, shippedValidator(t))

	want := []string{".mindrail/knowledge/decisions/DEC-0002.json"}
	if got := pathsAt(findings, validate.StepSupersedeTarget); !slices.Equal(got, want) {
		t.Fatalf("step 8 reported %v, want the record whose supersede resolves to nothing\n\t%s",
			got, describe(findings))
	}

	message := findingsAt(findings, validate.StepSupersedeTarget)[0].Message
	// The three facts the remedy needs: the id that resolved to nothing, the
	// file the reader will look in, and what that file actually says. Without
	// the third the message is the old "is not there" with extra words, and the
	// reader goes looking for a file that is in front of them.
	for _, fact := range []string{
		`"DEC-0009"`,
		".mindrail/knowledge/decisions/DEC-0009.json",
		`"DEC-0007"`,
	} {
		if !strings.Contains(message, fact) {
			t.Errorf("step 8's message does not carry %s: %q", fact, message)
		}
	}
	// And it must not be the contradiction round 1 removed. The same report
	// carries a step-6 finding against DEC-0009.json; calling it absent one line
	// away would tell the reader to do two opposite things about one file.
	if strings.Contains(message, "is not there") {
		t.Errorf("step 8 called a file this run read absent: %q", message)
	}
	// Step 6 still says its own half, because they are two conditions.
	wantNamed := []string{".mindrail/knowledge/decisions/DEC-0009.json"}
	if got := pathsAt(findings, validate.StepFilenameConsistency); !slices.Equal(got, wantNamed) {
		t.Errorf("step 6 reported %v, want the misfiled record\n\t%s", got, describe(findings))
	}
}

// TestStepEightStaysSilentWhenTheExpectedFileCannotAnswer is the over-fire arm,
// and it is round 1's F-R2 restated so that the fix above cannot undo it.
//
// Each row is a store where the file the id would occupy is one this run has in
// its hands and cannot use to contradict the reference. Nothing at all may be
// published for the reference, and in particular no message anywhere in the
// report may say a file this run read or named is not there.
func TestStepEightStaysSilentWhenTheExpectedFileCannotAnswer(t *testing.T) {
	validator := shippedValidator(t)

	referrer := decisionAt("DEC-0002.json", decisionDoc("DEC-0002", map[string]any{
		"supersedes": []any{"DEC-0001"},
	}))

	for _, tt := range []struct {
		name     string
		records  []testRecord
		problems []loader.Problem
	}{
		{
			// D-45 in its plainest form. The record exists; this binary could not
			// read it, and the loader has already said so in its own voice.
			name:     "the loader recorded a problem for the file",
			records:  []testRecord{referrer},
			problems: []loader.Problem{problemAt(loader.KindDecision, "DEC-0001.json")},
		},
		{
			// The file was read and its own "id" property is missing, so nothing
			// could have resolved and the file cannot say whose record it holds.
			// This is the row the fix above must not turn into "it carries id """.
			name: "the file was read and carries no id at all",
			records: []testRecord{
				referrer,
				decisionAt("DEC-0001.json", decisionDoc("DEC-0001", nil, "id")),
			},
		},
		{
			// The id property is present and is not a string, which the loader
			// leaves empty for step 5 to judge. Same conclusion, different cause,
			// and an implementation that keyed on "the property is absent" rather
			// than on "no id could be read" would fail here.
			name: "the file was read and its id is not a string",
			records: []testRecord{
				referrer,
				decisionAt("DEC-0001.json", decisionDoc("DEC-0001", map[string]any{"id": 1})),
			},
		},
		{
			// The file was read, rejected by step 5, and does carry the id. It
			// resolves, so step 8 never asks — and the report that says "correct
			// this file" must not also say it is absent.
			name: "the file was read and rejected but carries the id",
			records: []testRecord{
				referrer,
				decisionAt("DEC-0001.json", decisionDoc("DEC-0001", map[string]any{
					"status": "superseded",
					"titel":  "oops",
				})),
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			findings := validate.Check(storeOf(t, tt.records, tt.problems...), validator)

			if got := findingsAt(findings, validate.StepSupersedeTarget); len(got) != 0 {
				t.Errorf("step 8 spoke about a file it cannot contradict:\n\t%s", describe(got))
			}
			for _, finding := range findings {
				if strings.Contains(finding.Message, "is not there") {
					t.Errorf("a finding says a file this run named is not there: %q", finding.Message)
				}
			}
		})
	}
}

// TestStepEightStillSaysItIsNotThereWhenNothingNamedTheFile keeps the widened
// message from swallowing the plain case.
//
// Nothing in this run — no record, no Problem, no read file — mentions
// DEC-0001. "It is not there" is a claim this binary can make, and it is the one
// sentence that tells the reader to create the file rather than to look inside
// one.
func TestStepEightStillSaysItIsNotThereWhenNothingNamedTheFile(t *testing.T) {
	store := storeOf(t, []testRecord{
		decisionAt("DEC-0002.json", decisionDoc("DEC-0002", map[string]any{
			"supersedes": []any{"DEC-0001"},
		})),
	})

	findings := validate.Check(store, shippedValidator(t))
	got := findingsAt(findings, validate.StepSupersedeTarget)
	if len(got) != 1 {
		t.Fatalf("step 8 reported %d findings for a genuinely absent target, want 1:\n\t%s",
			len(got), describe(findings))
	}
	if !strings.Contains(got[0].Message, "is not there") {
		t.Errorf("step 8 no longer says an absent file is absent: %q", got[0].Message)
	}
	if !strings.Contains(got[0].Message, ".mindrail/knowledge/decisions/DEC-0001.json") {
		t.Errorf("step 8 does not name the file the reader has to create: %q", got[0].Message)
	}
}
