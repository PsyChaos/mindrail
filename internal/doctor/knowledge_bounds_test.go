package doctor

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/knowledge/loader"
	"github.com/PsyChaos/mindrail/internal/knowledge/validate"
)

// The literals below are written out rather than derived from
// maxDiagnosticLines, maxDiagnosticLineBytes or maxNamedRecords.
//
// A bound expressed in terms of the constant it guards holds for every value of
// that constant, including the value that removes the guard, which is the class
// of assertion MR-001's second audit round was spent deleting. These numbers are
// what the report is allowed to print; changing a constant is supposed to break
// them.
const (
	// A cut account plus its "... (N further bytes not shown)" marker. The
	// marker is at most 40 bytes and 1000 is where the cut lands.
	wantLongestAccountAtMost = 1040
	// The whole knowledge reading, serialised the way `doctor --json` carries
	// it. Twenty-three accounts of at most 1040 bytes, plus the impact, the
	// details and twenty-two remedies, rounds to under this; the same reading
	// before the bound existed was 45,374,759 bytes of diagnostic alone.
	wantReadingBytesAtMost = 30_000
)

// findingsOver builds n findings, one per record, all from the same step.
func findingsOver(n int, step validate.Step, message string) []validate.Finding {
	findings := make([]validate.Finding, 0, n)
	for i := 1; i <= n; i++ {
		finding := findingAt(fmt.Sprintf(".mindrail/knowledge/decisions/DEC-%05d.json", i), step)
		if message != "" {
			finding.Message = message
		}
		findings = append(findings, finding)
	}
	return findings
}

// problemsOver builds n non-fatal loader problems, one per record.
func problemsOver(n int) []loader.Problem {
	problems := make([]loader.Problem, 0, n)
	for i := 1; i <= n; i++ {
		problems = append(problems, unreadableRecord(fmt.Sprintf(".mindrail/knowledge/problems/DEC-%05d.json", i), false))
	}
	return problems
}

// readingBytes is the size of one check as `doctor --json` publishes it. The
// report is measured through the encoder rather than by adding up field lengths,
// because the payload a caller is handed is the encoded one.
func readingBytes(t *testing.T, result Result) int {
	t.Helper()

	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal reading: %v", err)
	}
	return len(encoded)
}

func diagnosticLines(result Result) []string {
	if result.Diagnostic == "" {
		return nil
	}
	return strings.Split(result.Diagnostic, "\n")
}

func longestLine(lines []string) int {
	longest := 0
	for _, line := range lines {
		longest = max(longest, len(line))
	}
	return longest
}

// TestTheKnowledgeReadingIsBoundedByItsOwnCapsAndNotByTheStore is B-04.
//
// `doctor --json` over a healthy store is flat in the number of records —
// 2,164 / 2,175 / 2,186 bytes at 10 / 100 / 1000 on the tree this was measured
// on. Over the same counts of broken records it was 11,675 / 491,636 /
// 45,386,339: the diagnostic joined every account the store produced, and each
// account was itself free to name every record it collided with. A health report
// that costs 45 MB is not a health report.
//
// The three sizes are asserted against one literal ceiling rather than against
// each other, so a report that merely grows more slowly still fails. The rows
// are two shapes because the two directions are independent: many short accounts
// and few enormous ones each defeat a bound written for the other.
func TestTheKnowledgeReadingIsBoundedByItsOwnCapsAndNotByTheStore(t *testing.T) {
	huge := strings.Repeat("x", 50_000)

	// wantLines and wantRemedies are per shape and written out. Ten accounts
	// and a tail is eleven; a reading that carries both a finding and an
	// unreadable record prints two such blocks with a heading between them,
	// which is twenty-three, and offers both remedy lists, which is twenty-two.
	shapes := []struct {
		name         string
		findAt       func(n int) []validate.Finding
		problem      func(n int) []loader.Problem
		wantLines    int
		wantRemedies int
	}{
		{
			name:         "one short account per record",
			findAt:       func(n int) []validate.Finding { return findingsOver(n, validate.StepSchema, "") },
			wantLines:    11,
			wantRemedies: 11,
		},
		{
			name:         "one account that names the whole store",
			findAt:       func(n int) []validate.Finding { return findingsOver(n, validate.StepUniqueID, huge) },
			wantLines:    11,
			wantRemedies: 11,
		},
		{
			name:         "the loader could not read any of them",
			findAt:       func(int) []validate.Finding { return nil },
			problem:      problemsOver,
			wantLines:    11,
			wantRemedies: 11,
		},
		{
			name:         "a finding and an unreadable record for every file",
			findAt:       func(n int) []validate.Finding { return findingsOver(n, validate.StepSchema, "") },
			problem:      problemsOver,
			wantLines:    23,
			wantRemedies: 22,
		},
	}

	for _, shape := range shapes {
		t.Run(shape.name, func(t *testing.T) {
			for _, n := range []int{10, 100, 1000} {
				t.Run(fmt.Sprintf("n=%d", n), func(t *testing.T) {
					subject := healthySubject()
					subject.KnowledgeFindings = shape.findAt(n)
					if shape.problem != nil {
						subject.Knowledge.Problems = shape.problem(n)
					}

					got := KnowledgeCheck(subject).Run(t.Context())

					assertDiagnosable(t, got)

					lines := diagnosticLines(got)
					if len(lines) > shape.wantLines {
						t.Errorf("the diagnostic prints %d accounts for %d records, want at most %d",
							len(lines), n, shape.wantLines)
					}
					if longest := longestLine(lines); longest > wantLongestAccountAtMost {
						t.Errorf("the longest account is %d bytes, want at most %d",
							longest, wantLongestAccountAtMost)
					}
					if len(got.NextAction) > shape.wantRemedies {
						t.Errorf("the reading offers %d remedies for %d records, want at most %d",
							len(got.NextAction), n, shape.wantRemedies)
					}
					if size := readingBytes(t, got); size > wantReadingBytesAtMost {
						t.Errorf("the reading serialises to %d bytes for %d records, want at most %d",
							size, n, wantReadingBytesAtMost)
					}
				})
			}
		})
	}
}

// TestAnAbbreviatedReadingSaysWhatItLeftOut is the other half of the bound.
//
// A report that silently prints ten of a thousand accounts has traded an
// unreadable answer for a false one — the reader is left believing the store
// holds ten broken records. So the omission is stated: the accounts past the cap
// are counted and located, and an account cut short carries the number of bytes
// it lost.
//
// The cut keeps the head of the line, which is where both callers write the
// record path and the rule, so what a truncation can reach is the end of the
// prose and never the file name AC-06.2 requires.
func TestAnAbbreviatedReadingSaysWhatItLeftOut(t *testing.T) {
	t.Run("accounts past the cap are counted and located", func(t *testing.T) {
		subject := healthySubject()
		subject.KnowledgeFindings = findingsOver(1000, validate.StepSchema, "")

		got := KnowledgeCheck(subject).Run(t.Context())

		lines := diagnosticLines(got)
		tail := lines[len(lines)-1]
		// 1000 findings, ten of them printed.
		if !strings.Contains(tail, "990") {
			t.Errorf("the tail %q does not count the accounts it did not print", tail)
		}
		if !strings.Contains(tail, loader.StoreRoot) {
			t.Errorf("the tail %q does not say where the records it did not print are", tail)
		}
	})

	t.Run("an account cut short says how much it lost and keeps its head", func(t *testing.T) {
		const path = ".mindrail/knowledge/decisions/DEC-00001.json"

		subject := healthySubject()
		finding := findingAt(path, validate.StepUniqueID)
		finding.Message = strings.Repeat("y", 50_000)
		subject.KnowledgeFindings = []validate.Finding{finding}

		got := KnowledgeCheck(subject).Run(t.Context())

		lines := diagnosticLines(got)
		if len(lines) != 1 {
			t.Fatalf("one finding produced %d accounts", len(lines))
		}
		if !strings.HasPrefix(lines[0], path+": step 7 (unique ID): ") {
			t.Errorf("the cut reached the head of the account: %.120q", lines[0])
		}
		if !strings.Contains(lines[0], "further bytes not shown") {
			t.Errorf("the account was cut without saying so: %.120q", lines[0])
		}
		if len(lines[0]) > wantLongestAccountAtMost {
			t.Errorf("the cut account is %d bytes, want at most %d", len(lines[0]), wantLongestAccountAtMost)
		}
	})

	// The message is three bytes per rune and is offset by nought, one and two
	// ASCII bytes, so two of the three rows put the cut inside a rune whatever
	// the length of the head the caller wrote in front of it.
	//
	// The offsets are the whole point of the row. With one offset the assertion
	// could not fail: the first fixture it was written with put the head at 79
	// bytes and the run of runes at 921 = 3 x 307, so the cut landed on a
	// boundary by accident and a cut made in bytes passed it unnoticed.
	runeCut := func(pad int) func(*testing.T) {
		return func(t *testing.T) {
			subject := healthySubject()
			finding := findingAt(".mindrail/knowledge/decisions/DEC-00001.json", validate.StepSchema)
			finding.Message = strings.Repeat("-", pad) + strings.Repeat("★", 20_000)
			subject.KnowledgeFindings = []validate.Finding{finding}

			got := KnowledgeCheck(subject).Run(t.Context())

			if !utf8.ValidString(got.Diagnostic) {
				t.Errorf("the diagnostic is not valid UTF-8 after the cut")
			}
			encoded, err := json.Marshal(got)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			if strings.ContainsRune(string(encoded), utf8.RuneError) {
				t.Errorf("the encoder had to replace a byte the cut left behind")
			}
		}
	}
	for pad := range 3 {
		t.Run(fmt.Sprintf("a cut lands on a rune boundary, head offset by %d", pad), runeCut(pad))
	}
}

// TestAReportSmallEnoughToPrintIsPrintedInFull is B-04's over-fire guard.
//
// A bound that abbreviates a store anybody could read costs the reader the file
// names the report exists to give them, which is the same defect as the one
// above pointing the other way. So the adjacent case — a store broken but small,
// which is every store this is likely to run against — is asserted to render
// with nothing counted, nothing cut and every path present.
//
// Ten and nine are the two rows because ten is the boundary: an off-by-one at
// the cap abbreviates a report that fits.
func TestAReportSmallEnoughToPrintIsPrintedInFull(t *testing.T) {
	for _, n := range []int{1, 9, 10} {
		t.Run(fmt.Sprintf("n=%d", n), func(t *testing.T) {
			findings := findingsOver(n, validate.StepSchema, "")

			subject := healthySubject()
			subject.KnowledgeFindings = findings

			got := KnowledgeCheck(subject).Run(t.Context())

			if len(diagnosticLines(got)) != n {
				t.Errorf("%d findings rendered %d accounts; a report this size is printed whole",
					n, len(diagnosticLines(got)))
			}
			if strings.Contains(got.Diagnostic, "not listed here") {
				t.Errorf("%d findings produced an omission tail:\n%s", n, got.Diagnostic)
			}
			if strings.Contains(got.Diagnostic, "further bytes not shown") {
				t.Errorf("%d findings produced a cut account:\n%s", n, got.Diagnostic)
			}
			for _, finding := range findings {
				if !strings.Contains(got.Diagnostic, finding.Path) {
					t.Errorf("the diagnostic does not name %q:\n%s", finding.Path, got.Diagnostic)
				}
				if !strings.Contains(got.Diagnostic, finding.Message) {
					t.Errorf("the account for %q was shortened", finding.Path)
				}
			}
		})
	}

	t.Run("an account the report can carry is carried whole", func(t *testing.T) {
		// 900 bytes of message. Under the cut, and far longer than any account
		// this binary writes for a store that is merely broken: the longest one
		// measured is 240 bytes.
		message := strings.Repeat("z", 900)

		subject := healthySubject()
		finding := findingAt(".mindrail/knowledge/decisions/DEC-00001.json", validate.StepSchema)
		finding.Message = message
		subject.KnowledgeFindings = []validate.Finding{finding}

		got := KnowledgeCheck(subject).Run(t.Context())

		if !strings.Contains(got.Diagnostic, message) {
			t.Errorf("a 900-byte account was cut; the report can carry it:\n%.200q", got.Diagnostic)
		}
	})
}

// TestAnUnreadableRecordIsNamedEvenWhenAnotherConditionOwnsTheReading is BA-04.
//
// AC-06.1's ladder decides which condition names the reading, and it stays
// exactly as it is: the assertions below take the same subject with and without
// the unreadable record and require the State, the Code and the Summary to be
// identical. What the ladder does not license is silence about the condition it
// did not name, and silence is what was there — a store carrying both an
// unreadable record and a finding reported KNOWLEDGE_INVALID and named the
// unreadable file nowhere: not in doctor's diagnostic, not in any next_action,
// and `status` published the count without the name.
//
// next_action is asserted separately from the diagnostic and neither is allowed
// to stand in for the other. status.componentFrom keeps next_action and drops
// Diagnostic and Impact (MR-001 finding R6), so a file named only in the
// diagnostic is invisible at the one place status prints the reading.
func TestAnUnreadableRecordIsNamedEvenWhenAnotherConditionOwnsTheReading(t *testing.T) {
	const (
		unreadable = ".mindrail/knowledge/decisions/DEC-0009.json"
		offending  = ".mindrail/knowledge/decisions/DEC-0001.json"
	)

	tests := []struct {
		name     string
		problems []loader.Problem
		findings []validate.Finding
		wantCode app.Code
	}{
		{
			name:     "a fatal loader problem owns the reading",
			problems: []loader.Problem{unreadableRecord(".mindrail/knowledge/decisions/DEC-0002.json", true)},
			findings: nil,
			wantCode: app.CodeKnowledgeSchemaUnsupported,
		},
		{
			name:     "a supersede cycle owns the reading",
			findings: []validate.Finding{findingAt(offending, validate.StepSupersedeCycle)},
			wantCode: app.CodeKnowledgeSupersedeCycle,
		},
		{
			name:     "an invalid record owns the reading",
			findings: []validate.Finding{findingAt(offending, validate.StepSchema)},
			wantCode: app.CodeKnowledgeInvalid,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			without := healthySubject()
			without.Knowledge.Problems = tc.problems
			without.KnowledgeFindings = tc.findings

			with := healthySubject()
			with.Knowledge.Problems = append(append([]loader.Problem{}, tc.problems...),
				unreadableRecord(unreadable, false))
			with.KnowledgeFindings = tc.findings

			quiet := KnowledgeCheck(without).Run(t.Context())
			loud := KnowledgeCheck(with).Run(t.Context())

			assertDiagnosable(t, loud)

			// The ladder is AC-06.1's and is not being renegotiated here.
			if quiet.Code != tc.wantCode {
				t.Fatalf("the row does not exercise the branch it names: code = %q, want %q", quiet.Code, tc.wantCode)
			}
			if loud.State != quiet.State || loud.Code != quiet.Code || loud.Summary != quiet.Summary {
				t.Errorf("naming the unreadable record moved the reading: %q/%q/%q became %q/%q/%q",
					quiet.State, quiet.Code, quiet.Summary, loud.State, loud.Code, loud.Summary)
			}

			if !strings.Contains(loud.Diagnostic, unreadable) {
				t.Errorf("the diagnostic does not name the unreadable record %q:\n%s", unreadable, loud.Diagnostic)
			}
			if !strings.Contains(strings.Join(loud.NextAction, "\n"), unreadable) {
				t.Errorf("no next_action names the unreadable record %q, so it is invisible where status prints this reading: %q",
					unreadable, loud.NextAction)
			}
		})
	}
}

// TestAReadingWithNothingUnreadableSaysNothingAboutUnreadableRecords is BA-04's
// over-fire guard.
//
// The fix adds an account and a remedy to three branches, and a fix that adds
// them when there is nothing to add invents a condition — a reading that tells
// the reader to go and fix a file that is perfectly readable is worse than the
// silence it replaced. So the same three branches are driven with no unreadable
// record and required to say nothing about one, and the plain unreadable-records
// branch is required not to name its own records twice.
func TestAReadingWithNothingUnreadableSaysNothingAboutUnreadableRecords(t *testing.T) {
	const offending = ".mindrail/knowledge/decisions/DEC-0001.json"

	tests := []struct {
		name     string
		problems []loader.Problem
		findings []validate.Finding
		wantCode app.Code
	}{
		{
			name:     "a fatal loader problem and nothing else",
			problems: []loader.Problem{unreadableRecord(offending, true)},
			wantCode: app.CodeKnowledgeSchemaUnsupported,
		},
		{
			name:     "a supersede cycle and nothing else",
			findings: []validate.Finding{findingAt(offending, validate.StepSupersedeCycle)},
			wantCode: app.CodeKnowledgeSupersedeCycle,
		},
		{
			name:     "an invalid record and nothing else",
			findings: []validate.Finding{findingAt(offending, validate.StepSchema)},
			wantCode: app.CodeKnowledgeInvalid,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			subject := healthySubject()
			subject.Knowledge.Problems = tc.problems
			subject.KnowledgeFindings = tc.findings

			got := KnowledgeCheck(subject).Run(t.Context())

			assertDiagnosable(t, got)
			if got.Code != tc.wantCode {
				t.Fatalf("the row does not exercise the branch it names: code = %q, want %q", got.Code, tc.wantCode)
			}
			if strings.Contains(got.Diagnostic, "The loader could not read") {
				t.Errorf("the reading reports unreadable records it was never handed:\n%s", got.Diagnostic)
			}
			if len(got.NextAction) != 1 {
				t.Errorf("one condition produced %d remedies: %q", len(got.NextAction), got.NextAction)
			}
		})
	}

	t.Run("the unreadable-records branch names its records once", func(t *testing.T) {
		const path = ".mindrail/knowledge/decisions/DEC-0009.json"

		subject := healthySubject()
		subject.Knowledge.Problems = []loader.Problem{unreadableRecord(path, false)}

		got := KnowledgeCheck(subject).Run(t.Context())

		if got.Code != app.CodeKnowledgeUnreadable {
			t.Fatalf("code = %q, want %q", got.Code, app.CodeKnowledgeUnreadable)
		}
		if len(got.NextAction) != 1 {
			t.Errorf("one unreadable record produced %d remedies: %q", len(got.NextAction), got.NextAction)
		}
		if strings.Count(got.Diagnostic, path) != 1 {
			t.Errorf("the diagnostic names %q %d times:\n%s", path, strings.Count(got.Diagnostic, path), got.Diagnostic)
		}
	})

	t.Run("a store with an unreadable record is still said to have one", func(t *testing.T) {
		// The guard above is satisfied by a reading that never mentions an
		// unreadable record at all, so this row is what keeps it honest: the
		// invalid-record branch's account of the rest of the store has to
		// change when the rest of the store stops being intact.
		intact := healthySubject()
		intact.KnowledgeFindings = []validate.Finding{findingAt(offending, validate.StepSchema)}

		holed := healthySubject()
		holed.KnowledgeFindings = intact.KnowledgeFindings
		holed.Knowledge.Problems = []loader.Problem{
			unreadableRecord(".mindrail/knowledge/decisions/DEC-0009.json", false),
		}

		if KnowledgeCheck(intact).Run(t.Context()).Impact == KnowledgeCheck(holed).Run(t.Context()).Impact {
			t.Error("a store missing a record it cannot read is said to cost the reader exactly what an intact one costs")
		}
	})
}
