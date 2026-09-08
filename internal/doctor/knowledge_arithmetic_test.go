package doctor

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/PsyChaos/mindrail/internal/knowledge/loader"
	"github.com/PsyChaos/mindrail/internal/knowledge/validate"
)

// omissionTail is the sentence describeBounded prints when it could not print
// everything. It is matched as a whole sentence, with the count, the noun and
// the location captured separately, because the defect this file is about was a
// true-looking sentence carrying a false number: the words said "findings under
// .mindrail/knowledge" and the number counted one class of them.
var omissionTail = regexp.MustCompile(`^\.\.\. and (\d+) further (.+) under (.+), not listed here\.$`)

type tail struct {
	count int
	noun  string
	root  string
}

// omissionIn finds the tail in a rendered reading, if there is one.
func omissionIn(t *testing.T, result Result) (tail, bool) {
	t.Helper()

	var found tail
	seen := 0
	for _, line := range diagnosticLines(result) {
		match := omissionTail.FindStringSubmatch(line)
		if match == nil {
			continue
		}
		seen++
		count, err := strconv.Atoi(match[1])
		if err != nil {
			t.Fatalf("the tail %q does not carry a number: %v", line, err)
		}
		found = tail{count: count, noun: match[2], root: match[3]}
	}
	if seen > 1 {
		t.Fatalf("the reading carries %d omission tails, and this helper reads one:\n%s", seen, result.Diagnostic)
	}
	return found, seen == 1
}

// accountsIn counts the per-record accounts a reading printed. Every one of them
// opens with the record path, which is what makes the head of the line safe to
// keep when the byte cap cuts it.
func accountsIn(result Result) int {
	printed := 0
	for _, line := range diagnosticLines(result) {
		if strings.HasPrefix(line, loader.StoreRoot+"/") {
			printed++
		}
	}
	return printed
}

// cycleAndInvalid builds a store whose findings span both classes: a cycle over
// the first group and an unrelated schema failure on the second.
func cycleAndInvalid(cycles, invalid int) []validate.Finding {
	findings := make([]validate.Finding, 0, cycles+invalid)
	for i := 1; i <= cycles; i++ {
		findings = append(findings, findingAt(
			fmt.Sprintf("%s/decisions/CYC-%05d.json", loader.StoreRoot, i), validate.StepSupersedeCycle))
	}
	for i := 1; i <= invalid; i++ {
		findings = append(findings, findingAt(
			fmt.Sprintf("%s/decisions/BAD-%05d.json", loader.StoreRoot, i), validate.StepSchema))
	}
	return findings
}

// TestTheOmissionTailCountsTheSetItsSentenceNames is B-A2.
//
// `describeBounded` computed its omission from the slice it was handed, and the
// slice is one finding class: the precedence ladder gives the diagnostic to the
// cycles and never describes the invalid records at all. A store with a
// 25-record cycle and five unrelated schema failures printed ten accounts and
// "... and 15 further findings under .mindrail/knowledge, not listed here" while
// `status` published 30 for the same store. 10 + 15 = 25, and the sentence is
// about the store.
//
// The assertion is the arithmetic and not the prose. shown + omitted has to
// equal the population the sentence names, which the sentence itself is read for
// — "findings under <root>" is every finding steps 5-11 produced — so a fix that
// corrects the number and a fix that narrows the words are both green only if
// they agree with each other, and a drift in either is red.
func TestTheOmissionTailCountsTheSetItsSentenceNames(t *testing.T) {
	tests := []struct {
		name     string
		cycles   int
		invalid  int
		wantTail bool
	}{
		// The measured reproduction.
		{name: "a cycle beside unrelated invalid records", cycles: 25, invalid: 5, wantTail: true},
		// Neither class alone can catch the defect: the omitted set and the
		// class are the same slice, which is why a green suite carried it.
		{name: "a cycle and nothing else", cycles: 25, wantTail: true},
		{name: "invalid records and nothing else", invalid: 25, wantTail: true},
		// Under the cap in the rendered class, so the block is complete and the
		// tail exists only because the other class does. A tail computed from
		// the block prints nothing at all here, and the reader is left believing
		// the store holds three findings.
		{name: "a small cycle beside invalid records", cycles: 3, invalid: 5, wantTail: true},
		{name: "one cycle beside one invalid record", cycles: 1, invalid: 1, wantTail: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			findings := cycleAndInvalid(tc.cycles, tc.invalid)

			subject := healthySubject()
			subject.KnowledgeFindings = findings

			got := KnowledgeCheck(subject).Run(t.Context())
			assertDiagnosable(t, got)

			shown := accountsIn(got)
			omitted, printed := omissionIn(t, got)
			if printed != tc.wantTail {
				t.Fatalf("a store of %d findings printed %d accounts and a tail=%v, want tail=%v:\n%s",
					len(findings), shown, printed, tc.wantTail, got.Diagnostic)
			}

			// The sentence says "findings under <root>". That population is
			// every finding the pipeline produced for this store, and the
			// number beside those words has to be over the same population.
			if omitted.noun != plural(omitted.count, "finding", "findings") {
				t.Errorf("the tail counts %q, which is not the population its own sentence names", omitted.noun)
			}
			if omitted.root != loader.StoreRoot {
				t.Errorf("the tail locates the omission at %q, want %q", omitted.root, loader.StoreRoot)
			}
			if shown+omitted.count != len(findings) {
				t.Errorf("the reading accounts for %d + %d = %d findings; the store holds %d",
					shown, omitted.count, shown+omitted.count, len(findings))
			}
		})
	}
}

// TestAStoreWhoseFindingsAllFitIsNotGivenAnOmissionTail is the over-fire guard
// for the test above.
//
// A tail counted over the store rather than over the block will announce an
// omission on a report that omitted nothing if the total it is handed is not the
// store's, and "and 0 further findings" or a tail on a complete list is the same
// class of false statement pointing the other way. Ten is the row that matters:
// an off-by-one at the cap invents an omission on a report that fits exactly.
func TestAStoreWhoseFindingsAllFitIsNotGivenAnOmissionTail(t *testing.T) {
	for _, n := range []int{1, 9, 10} {
		t.Run(fmt.Sprintf("%d cycle findings and nothing else", n), func(t *testing.T) {
			subject := healthySubject()
			subject.KnowledgeFindings = cycleAndInvalid(n, 0)

			got := KnowledgeCheck(subject).Run(t.Context())

			if _, printed := omissionIn(t, got); printed {
				t.Errorf("%d findings, all of them printed, produced an omission tail:\n%s", n, got.Diagnostic)
			}
			if shown := accountsIn(got); shown != n {
				t.Errorf("%d findings rendered %d accounts", n, shown)
			}
		})
	}

	t.Run("a store with no findings at all says nothing about findings", func(t *testing.T) {
		got := KnowledgeCheck(healthySubject()).Run(t.Context())

		if got.State != StateOK {
			t.Fatalf("a healthy store read %q: %+v", got.State, got)
		}
		if got.Diagnostic != "" {
			t.Errorf("a healthy store carries a diagnostic:\n%s", got.Diagnostic)
		}
	})
}

// TestTheLoaderProblemTailCountsTheBlockItCloses is the other caller, and the
// answer to "the same argument applies to recordRemedies' tail".
//
// It does not, and the difference is structural rather than a matter of taste.
// The loader's problems are rendered in blocks that partition them — the fatal
// branch prints the fatal records, then unreadableAlso prints the degraded ones
// under a heading carrying their own count — so a tail counted over the store
// would say "not listed here" about records listed three lines below. Each block
// closes over itself, and the heading between them is what lets a reader add
// them up. That is asserted here rather than left to be inferred, because it is
// the reason the two callers are scoped differently.
func TestTheLoaderProblemTailCountsTheBlockItCloses(t *testing.T) {
	fatal := make([]loader.Problem, 0, 15)
	for i := 1; i <= 15; i++ {
		fatal = append(fatal, unreadableRecord(fmt.Sprintf("%s/decisions/FAT-%05d.json", loader.StoreRoot, i), true))
	}
	degraded := make([]loader.Problem, 0, 3)
	for i := 1; i <= 3; i++ {
		degraded = append(degraded, unreadableRecord(fmt.Sprintf("%s/decisions/DEG-%05d.json", loader.StoreRoot, i), false))
	}

	subject := healthySubject()
	subject.Knowledge.Problems = append(append([]loader.Problem{}, fatal...), degraded...)

	got := KnowledgeCheck(subject).Run(t.Context())
	assertDiagnosable(t, got)

	omitted, printed := omissionIn(t, got)
	if !printed {
		t.Fatalf("15 fatal records printed no omission tail:\n%s", got.Diagnostic)
	}
	// The first block shows ten of the fifteen fatal records; the tail closes
	// that block and no other.
	if omitted.count+maxDiagnosticLines != len(fatal) {
		t.Errorf("the tail accounts for %d + %d = %d records; the block it closes holds %d",
			maxDiagnosticLines, omitted.count, maxDiagnosticLines+omitted.count, len(fatal))
	}

	// The heading is what carries the second block's count. Without it the
	// reader has no way to add the two blocks up, and a store-wide tail on the
	// first block would double-count the three records printed under it.
	heading := fmt.Sprintf("The loader could not read %d further records:", len(degraded))
	if !strings.Contains(got.Diagnostic, heading) {
		t.Errorf("the reading does not introduce the degraded block with %q:\n%s", heading, got.Diagnostic)
	}
	for _, problem := range degraded {
		if !strings.Contains(got.Diagnostic, problem.Path) {
			t.Errorf("the degraded record %q is counted and not named:\n%s", problem.Path, got.Diagnostic)
		}
	}
}

// TestTheRemedyTailCountsTheRecordsItsInstructionApplied is recordRemedies'
// half of the same question.
//
// "Fix or remove the remaining N unreadable records" is an instruction, and the
// set it counts has to be the set the instruction applies to. On the fatal
// branch that is the degraded records and not the store: the fatal ones are
// repaired by upgrading the binary, so counting them here would ask the reader
// to delete records whose only defect is being too new to read.
func TestTheRemedyTailCountsTheRecordsItsInstructionApplied(t *testing.T) {
	degraded := make([]loader.Problem, 0, 15)
	for i := 1; i <= 15; i++ {
		degraded = append(degraded, unreadableRecord(fmt.Sprintf("%s/decisions/DEG-%05d.json", loader.StoreRoot, i), false))
	}

	tests := []struct {
		name     string
		problems []loader.Problem
		// The remedies that are not about the degraded records: the fatal
		// branch offers one upgrade instruction before them.
		other int
	}{
		{name: "degraded records alone", problems: degraded},
		{
			name: "degraded records below a fatal one",
			problems: append([]loader.Problem{
				unreadableRecord(loader.StoreRoot+"/decisions/FAT-00001.json", true),
			}, degraded...),
			other: 1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			subject := healthySubject()
			subject.Knowledge.Problems = tc.problems

			got := KnowledgeCheck(subject).Run(t.Context())
			assertDiagnosable(t, got)

			named := 0
			remaining := 0
			for _, action := range got.NextAction {
				if strings.HasPrefix(action, "Fix or remove "+loader.StoreRoot+"/") {
					named++
					continue
				}
				var count int
				if _, err := fmt.Sscanf(action, "Fix or remove the remaining %d unreadable", &count); err == nil {
					remaining = count
				}
			}
			if named+remaining != len(degraded) {
				t.Errorf("the remedies account for %d named + %d remaining = %d records; the instruction applies to %d",
					named, remaining, named+remaining, len(degraded))
			}
			if len(got.NextAction) != named+1+tc.other {
				t.Errorf("the reading offers %d remedies: %q", len(got.NextAction), got.NextAction)
			}
		})
	}
}

// TestABoundedAccountNeverPromisesFewerRecordsThanItPrinted is the guard on
// describeBounded's own arithmetic.
//
// The function is handed a total by its caller, and a caller that understates it
// must not be able to make the report subtract: "... and -3 further findings" is
// what an omission computed without a sign check renders as. It is called
// directly rather than through a check because no caller can pass a total below
// its own line count today — an assertion driven through knowledgeResult would
// be green whatever this function did with one.
func TestABoundedAccountNeverPromisesFewerRecordsThanItPrinted(t *testing.T) {
	lines := []string{
		loader.StoreRoot + "/decisions/DEC-0001.json: one",
		loader.StoreRoot + "/decisions/DEC-0002.json: two",
		loader.StoreRoot + "/decisions/DEC-0003.json: three",
	}

	for _, total := range []int{-1, 0, 2, 3} {
		t.Run(fmt.Sprintf("total=%d", total), func(t *testing.T) {
			got := describeBounded(lines, total, "finding", "findings")

			if strings.Contains(got, "not listed here") {
				t.Errorf("a total of %d for %d printed accounts produced an omission tail:\n%s",
					total, len(lines), got)
			}
			if strings.Contains(got, "-") && strings.Contains(got, "further") {
				t.Errorf("the tail carries a negative count:\n%s", got)
			}
			for _, line := range lines {
				if !strings.Contains(got, line) {
					t.Errorf("the account %q was dropped:\n%s", line, got)
				}
			}
		})
	}

	t.Run("total=4", func(t *testing.T) {
		got := describeBounded(lines, 4, "finding", "findings")

		if !strings.Contains(got, "... and 1 further finding under "+loader.StoreRoot+", not listed here.") {
			t.Errorf("a total one above the accounts on hand did not produce a tail of one:\n%s", got)
		}
	})
}
