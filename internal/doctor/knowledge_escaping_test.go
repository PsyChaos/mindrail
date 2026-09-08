package doctor

import (
	"fmt"
	"strings"
	"testing"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/knowledge/loader"
	"github.com/PsyChaos/mindrail/internal/knowledge/validate"
)

// declinedRecord is loader.escapingRecord's Problem, carrying the same message
// unreadableRecord's fixture carries.
//
// The real message explains the condition in full — "resolves outside the
// repository root, so Mindrail will not read it as repository content" — and a
// reading assembled from it would satisfy every assertion below whether or not
// this package understood what it was rendering, the same reason findingAt
// writes a message that does not repeat the path.
//
// The message is not merely neutral but identical to the unreadable fixture's,
// so the two problems differ in exactly one field: the Code. Every difference
// these tests find between the two readings is then attributable to it, which is
// what findings R-05 and B-A6 are about — on 3bd453c a planted symlink and a
// truncated file produced the same code, the same summary and the same remedy
// shape, and nothing downstream could tell them apart.
func declinedRecord(path string) loader.Problem {
	return loader.Problem{
		Path:    path,
		Code:    app.CodePathEscapesRoot,
		Message: "record rejected",
		Fatal:   false,
	}
}

// readFailureWords are the claims a reading must not make about a record that
// reads perfectly. They are the words the KNOWLEDGE_UNREADABLE rendering used:
// before decision D-51 a planted symlink and a truncated file arrived under one
// code, one summary and one remedy shape, and a consumer branching on the code
// could not tell them apart.
var readFailureWords = []string{
	"unreadable",
	"could not be read",
	"could not read",
	"Fix or remove",
}

func assertSaysNothingAboutAReadFailure(t *testing.T, result Result) {
	t.Helper()

	whole := strings.Join(append([]string{result.Summary, result.Diagnostic, result.Impact}, result.NextAction...), "\n")
	for _, word := range readFailureWords {
		if strings.Contains(whole, word) {
			t.Errorf("the reading says %q about a record that reads perfectly:\n%s", word, whole)
		}
	}
}

// TestARecordMindrailDeclinedIsPublishedAsItsOwnCondition is decision D-51,
// findings R-05 and B-A6.
//
// loader.escapingRecord draws the distinction deliberately: PATH_ESCAPES_ROOT
// and not KNOWLEDGE_UNREADABLE, because the record reads and what happened is
// that Mindrail declined to treat it as repository content. Nothing could
// observe it. `knowledgeResult`'s degraded branch hard-coded
// KNOWLEDGE_UNREADABLE, the summary "Knowledge store has unreadable records" and
// the remedy "Fix or remove <path>", so a planted symlink and a truncated file
// were indistinguishable in the code, the summary and the remedy shape alike.
func TestARecordMindrailDeclinedIsPublishedAsItsOwnCondition(t *testing.T) {
	const path = ".mindrail/knowledge/decisions/DEC-0002.json"

	subject := healthySubject()
	subject.Knowledge.Problems = []loader.Problem{declinedRecord(path)}

	got := KnowledgeCheck(subject).Run(t.Context())

	assertDiagnosable(t, got)
	if got.Code != app.CodePathEscapesRoot {
		t.Errorf("code = %q, want %q", got.Code, app.CodePathEscapesRoot)
	}
	if got.State != StateDegraded {
		t.Errorf("state = %q, want %q; one declined record costs the repository that record", got.State, StateDegraded)
	}
	assertSaysNothingAboutAReadFailure(t, got)

	// What actually happened, in each of the three fields a non-OK reading owes
	// its reader. The diagnostic and the impact are asserted separately from the
	// remedy because status.componentFrom drops the first two and keeps the
	// third, so a condition explained only in the diagnostic is invisible
	// exactly where `status` prints this reading.
	if !strings.Contains(got.Diagnostic, "declined") || !strings.Contains(got.Diagnostic, "outside the repository root") {
		t.Errorf("the diagnostic does not say what happened:\n%s", got.Diagnostic)
	}
	if !strings.Contains(got.Diagnostic, path) {
		t.Errorf("the diagnostic does not name the record %q:\n%s", path, got.Diagnostic)
	}
	if !strings.Contains(got.Impact, "declined") {
		t.Errorf("the impact does not say what happened: %q", got.Impact)
	}
	remedies := strings.Join(got.NextAction, "\n")
	if !strings.Contains(remedies, path) {
		t.Errorf("no next_action names the record %q: %q", path, got.NextAction)
	}
	if !strings.Contains(remedies, "link") {
		t.Errorf("no next_action says what to do with the link: %q", got.NextAction)
	}
}

// TestTwoRecordsThatDifferOnlyInCodeDoNotReadTheSame is findings R-05 and B-A6
// stated as the measurement that produced them.
//
// On 3bd453c a planted symlink and a truncated file arrived under one code, one
// summary and one remedy shape. The two subjects here differ in exactly one
// field — loader.Problem.Code — so any field that still reads identically is a
// field through which the distinction the loader drew cannot be observed.
func TestTwoRecordsThatDifferOnlyInCodeDoNotReadTheSame(t *testing.T) {
	const path = ".mindrail/knowledge/decisions/DEC-0002.json"

	linked := healthySubject()
	linked.Knowledge.Problems = []loader.Problem{declinedRecord(path)}

	corrupt := healthySubject()
	corrupt.Knowledge.Problems = []loader.Problem{unreadableRecord(path, false)}

	if linked.Knowledge.Problems[0].Message != corrupt.Knowledge.Problems[0].Message {
		t.Fatalf("the two fixtures differ in more than the code, so a difference below proves nothing")
	}

	declined := KnowledgeCheck(linked).Run(t.Context())
	unread := KnowledgeCheck(corrupt).Run(t.Context())

	fields := []struct {
		name          string
		got, otherGot string
	}{
		{"code", string(declined.Code), string(unread.Code)},
		{"summary", declined.Summary, unread.Summary},
		{"diagnostic", declined.Diagnostic, unread.Diagnostic},
		{"impact", declined.Impact, unread.Impact},
		{"next_action", strings.Join(declined.NextAction, "\n"), strings.Join(unread.NextAction, "\n")},
	}
	for _, field := range fields {
		if field.got == field.otherGot {
			t.Errorf("a link out of the repository and a corrupt file publish the same %s: %q",
				field.name, field.got)
		}
	}
}

// TestAGenuinelyUnreadableRecordStillReportsUnreadable is the over-fire guard.
//
// The branch above fires on a Code, and a branch that fires on the wrong set
// costs the reader the reading that was right. A truncated file is the adjacent
// case — the loader could not read it, there is something in it to fix, and
// KNOWLEDGE_UNREADABLE with "Fix or remove <path>" is exactly what it should
// still say. The summary, the impact and the remedy are compared as whole
// strings rather than searched, because this reading is a golden and the fix
// must not have moved a byte of it.
func TestAGenuinelyUnreadableRecordStillReportsUnreadable(t *testing.T) {
	const path = ".mindrail/knowledge/decisions/DEC-0003.json"

	subject := healthySubject()
	subject.Knowledge.Problems = []loader.Problem{unreadableRecord(path, false)}

	got := KnowledgeCheck(subject).Run(t.Context())

	assertDiagnosable(t, got)
	if got.Code != app.CodeKnowledgeUnreadable {
		t.Errorf("code = %q, want %q", got.Code, app.CodeKnowledgeUnreadable)
	}
	if got.Summary != "Knowledge store has unreadable records" {
		t.Errorf("summary = %q; the unreadable reading moved", got.Summary)
	}
	if got.Impact != "The reported records are invisible to every command; the rest of the store is unaffected." {
		t.Errorf("impact = %q; the unreadable reading moved", got.Impact)
	}
	want := []string{"Fix or remove " + path + "."}
	if len(got.NextAction) != 1 || got.NextAction[0] != want[0] {
		t.Errorf("next_action = %q, want %q", got.NextAction, want)
	}
}

// TestAMixedDegradedSetKeepsTheUnreadableCodeAndNamesBothClasses is the
// under-fire guard.
//
// A reading publishes exactly one Code, so a store holding both classes has to
// choose, and decision D-51 chooses KNOWLEDGE_UNREADABLE: it is wrong about
// neither half, since a record the loader could not read is genuinely
// unreadable, while PATH_ESCAPES_ROOT would be a false claim about the
// truncated file. What the choice must not do is hide the half it did not name.
// The summary is where that matters most — `status` prints the summary and the
// code and drops the diagnostic and the impact entirely.
func TestAMixedDegradedSetKeepsTheUnreadableCodeAndNamesBothClasses(t *testing.T) {
	const (
		declined = ".mindrail/knowledge/decisions/DEC-0002.json"
		broken   = ".mindrail/knowledge/decisions/DEC-0003.json"
	)

	subject := healthySubject()
	subject.Knowledge.Problems = []loader.Problem{declinedRecord(declined), unreadableRecord(broken, false)}

	got := KnowledgeCheck(subject).Run(t.Context())

	assertDiagnosable(t, got)
	if got.Code != app.CodeKnowledgeUnreadable {
		t.Errorf("code = %q, want %q for a mixed degraded set", got.Code, app.CodeKnowledgeUnreadable)
	}

	// Both classes, in the summary and in the impact.
	if !strings.Contains(got.Summary, "unreadable") {
		t.Errorf("the summary does not name the unreadable record: %q", got.Summary)
	}
	if !strings.Contains(got.Summary, "outside the repository") {
		t.Errorf("the summary does not name the declined record: %q", got.Summary)
	}
	if !strings.Contains(got.Impact, "could not be read") {
		t.Errorf("the impact does not say one record could not be read: %q", got.Impact)
	}
	if !strings.Contains(got.Impact, "outside the repository root") {
		t.Errorf("the impact does not say one record was declined: %q", got.Impact)
	}

	// Both files, in the diagnostic and in the remedies, and the two remedies
	// have to differ: there is nothing in the declined file to fix.
	remedies := strings.Join(got.NextAction, "\n")
	for _, path := range []string{declined, broken} {
		if !strings.Contains(got.Diagnostic, path) {
			t.Errorf("the diagnostic does not name %q:\n%s", path, got.Diagnostic)
		}
		if !strings.Contains(remedies, path) {
			t.Errorf("no next_action names %q: %q", path, got.NextAction)
		}
	}
	if !strings.Contains(remedies, "Fix or remove "+broken+".") {
		t.Errorf("the unreadable record lost its own remedy: %q", got.NextAction)
	}
	if strings.Contains(remedies, "Fix or remove "+declined+".") {
		t.Errorf("the declined record is told to be fixed, and there is nothing in it to fix: %q", got.NextAction)
	}
}

// TestADeclinedRecordKeepsItsClassBelowEveryConditionThatOutranksIt walks the
// ladder decision D-51 places the new branch on.
//
// The branch sits with the other degraded-problem readings: a fatal loader
// Problem still outranks it (D-44) and so does a cycle (D-39/D-43), and an
// invalid record is reported ahead of it for the same reason an unreadable one
// is. The reading those conditions own must still name the declined file — the
// ladder decides which condition names the reading, not whether the others are
// mentioned — and must still not say the loader could not read it.
func TestADeclinedRecordKeepsItsClassBelowEveryConditionThatOutranksIt(t *testing.T) {
	const (
		declined  = ".mindrail/knowledge/decisions/DEC-0002.json"
		offending = ".mindrail/knowledge/decisions/DEC-0001.json"
	)

	tests := []struct {
		name     string
		problems []loader.Problem
		findings []validate.Finding
		wantCode app.Code
	}{
		{
			name:     "a fatal loader problem owns the reading",
			problems: []loader.Problem{unreadableRecord(".mindrail/knowledge/decisions/DEC-0009.json", true)},
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
			with.Knowledge.Problems = append(append([]loader.Problem{}, tc.problems...), declinedRecord(declined))
			with.KnowledgeFindings = tc.findings

			quiet := KnowledgeCheck(without).Run(t.Context())
			loud := KnowledgeCheck(with).Run(t.Context())

			assertDiagnosable(t, loud)

			if quiet.Code != tc.wantCode {
				t.Fatalf("the row does not exercise the branch it names: code = %q, want %q", quiet.Code, tc.wantCode)
			}
			if loud.Code != quiet.Code || loud.State != quiet.State {
				t.Errorf("the declined record took the reading from %q/%q to %q/%q",
					quiet.State, quiet.Code, loud.State, loud.Code)
			}

			if !strings.Contains(loud.Diagnostic, declined) {
				t.Errorf("the diagnostic does not name the declined record %q:\n%s", declined, loud.Diagnostic)
			}
			if !strings.Contains(strings.Join(loud.NextAction, "\n"), declined) {
				t.Errorf("no next_action names the declined record %q: %q", declined, loud.NextAction)
			}

			// The account of the declined record is still the account of a
			// declined record. The heading unreadableAlso writes above it said
			// "The loader could not read 1 further record", which is a false
			// sentence about a file that reads.
			if strings.Contains(loud.Diagnostic, "The loader could not read 1 further record") {
				t.Errorf("the reading says the loader could not read a record that reads:\n%s", loud.Diagnostic)
			}
			if strings.Contains(strings.Join(loud.NextAction, "\n"), "Fix or remove "+declined+".") {
				t.Errorf("the declined record is told to be fixed: %q", loud.NextAction)
			}
			if !strings.Contains(loud.Diagnostic, "declined") {
				t.Errorf("the reading does not say what happened to %q:\n%s", declined, loud.Diagnostic)
			}
		})
	}
}

// TestTheDeclinedBranchDoesNotFireOnAStoreWithNothingDeclined is the second
// over-fire guard, and it is the one the shape of the test above cannot make.
//
// `allEscapeRoot` answers a question about a set, and both degenerate answers
// are wrong in the same direction: an empty set is not homogeneously anything,
// so a store with no degraded records at all must not read as a store full of
// declined ones. Dropping the emptiness guard makes the branch fire on every
// healthy repository, and every assertion in this file about a declined record
// still passes while it does.
func TestTheDeclinedBranchDoesNotFireOnAStoreWithNothingDeclined(t *testing.T) {
	t.Run("a healthy store", func(t *testing.T) {
		got := KnowledgeCheck(healthySubject()).Run(t.Context())

		if got.State != StateOK {
			t.Fatalf("a healthy store read %q: %+v", got.State, got)
		}
		if got.Code != "" {
			t.Errorf("a healthy store published code %q", got.Code)
		}
	})

	t.Run("a store whose only defect is an invalid record", func(t *testing.T) {
		subject := healthySubject()
		subject.KnowledgeFindings = []validate.Finding{
			findingAt(".mindrail/knowledge/decisions/DEC-0001.json", validate.StepSchema),
		}

		got := KnowledgeCheck(subject).Run(t.Context())

		if got.Code != app.CodeKnowledgeInvalid {
			t.Errorf("code = %q, want %q", got.Code, app.CodeKnowledgeInvalid)
		}
		if strings.Contains(got.Diagnostic, "declined") {
			t.Errorf("the reading reports a declined record it was never handed:\n%s", got.Diagnostic)
		}
	})
}

// TestManyDeclinedRecordsAreBoundedAndCountedAsDeclinedRecords keeps the new
// branch inside the caps the rest of the knowledge reading lives under, and
// keeps the omission tail counting the class it is actually counting.
//
// "... and 5 further unreadable records" said about five files that all read
// perfectly is the same false statement the code was making, moved into the
// tail, and it is what the branch would print if it borrowed the noun the
// unreadable rendering uses.
func TestManyDeclinedRecordsAreBoundedAndCountedAsDeclinedRecords(t *testing.T) {
	problems := make([]loader.Problem, 0, 15)
	for i := 1; i <= 15; i++ {
		problems = append(problems, declinedRecord(fmt.Sprintf("%s/decisions/DEC-%05d.json", loader.StoreRoot, i)))
	}

	subject := healthySubject()
	subject.Knowledge.Problems = problems

	got := KnowledgeCheck(subject).Run(t.Context())

	assertDiagnosable(t, got)
	if got.Code != app.CodePathEscapesRoot {
		t.Fatalf("code = %q, want %q", got.Code, app.CodePathEscapesRoot)
	}
	assertSaysNothingAboutAReadFailure(t, got)

	omitted, printed := omissionIn(t, got)
	if !printed {
		t.Fatalf("15 declined records printed no omission tail:\n%s", got.Diagnostic)
	}
	if omitted.noun != "records resolving outside the repository" {
		t.Errorf("the tail counts %q, which is not what these records are", omitted.noun)
	}
	if shown := accountsIn(got); shown+omitted.count != len(problems) {
		t.Errorf("the reading accounts for %d + %d = %d records; the store holds %d",
			shown, omitted.count, shown+omitted.count, len(problems))
	}
	if len(got.NextAction) != maxNamedRecords+1 {
		t.Errorf("15 declined records produced %d remedies: %q", len(got.NextAction), got.NextAction)
	}
}
