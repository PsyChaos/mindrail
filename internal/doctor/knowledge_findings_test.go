package doctor

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/knowledge/loader"
	"github.com/PsyChaos/mindrail/internal/knowledge/schema"
	"github.com/PsyChaos/mindrail/internal/knowledge/validate"
	"github.com/PsyChaos/mindrail/schemas"
)

// pipelineSteps is spec §95's steps 5-11 written out, so that a test iterating
// them iterates the whole pipeline rather than whatever a helper derived from
// the same table it is checking.
var pipelineSteps = []validate.Step{
	validate.StepSchema,
	validate.StepFilenameConsistency,
	validate.StepUniqueID,
	validate.StepSupersedeTarget,
	validate.StepSupersedeCycle,
	validate.StepDuplicateActiveLineage,
	validate.StepScopeSyntax,
}

// deferredStep is a step this milestone does not implement. Steps 12, 13 and 14
// are deferred rather than absent, so it is a real number the report may one day
// be handed and the honest input for the naming fallbacks.
const deferredStep = validate.Step(12)

// findingAt builds one finding whose message deliberately does not repeat the
// path.
//
// Every assertion below about a rendered file name is then an assertion about
// describeFindings and findingRemedies rather than about a sentence they only
// pass through. validate's own messages do open with the path today, which is
// exactly why a fixture that copied them would pass whether or not the doctor
// layer named the file at all.
func findingAt(path string, step validate.Step) validate.Finding {
	finding := validate.Finding{
		Path:    path,
		ID:      "DEC-0001",
		Step:    step,
		Code:    app.CodeKnowledgeInvalid,
		Message: "a rule this record breaks",
	}
	if step == validate.StepSupersedeCycle {
		finding.Code = app.CodeKnowledgeSupersedeCycle
		finding.Fatal = true
	}
	return finding
}

func unreadableRecord(path string, fatal bool) loader.Problem {
	code := app.CodeKnowledgeUnreadable
	if fatal {
		code = app.CodeKnowledgeSchemaUnsupported
	}
	return loader.Problem{Path: path, Code: code, Message: "record rejected", Fatal: fatal}
}

// TestKnowledgeFindingsFollowTheDeclaredPrecedence is AC-06.1 as a table, and
// the first row is decision D-44 itself.
//
// A verdict computed over a store the binary has just said it cannot fully read
// is a claim made from incomplete data, so a fatal loader problem outranks every
// finding — including the fatal one. The remaining rows walk down the ladder,
// and the last two are the over-fire guards: an unreadable record with no
// finding still reports what it always reported, and a store with neither stays
// OK.
func TestKnowledgeFindingsFollowTheDeclaredPrecedence(t *testing.T) {
	const decision = ".mindrail/knowledge/decisions/dec-1.json"

	cycle := findingAt(decision, validate.StepSupersedeCycle)
	invalid := findingAt(decision, validate.StepSchema)

	tests := []struct {
		name      string
		problems  []loader.Problem
		findings  []validate.Finding
		wantState State
		wantCode  app.Code
	}{
		{
			name:      "a fatal loader problem outranks a cycle",
			problems:  []loader.Problem{unreadableRecord(".mindrail/knowledge/decisions/dec-9.json", true)},
			findings:  []validate.Finding{cycle, invalid},
			wantState: StateError,
			wantCode:  app.CodeKnowledgeSchemaUnsupported,
		},
		{
			name:      "a cycle outranks every other finding",
			findings:  []validate.Finding{cycle, invalid},
			wantState: StateError,
			wantCode:  app.CodeKnowledgeSupersedeCycle,
		},
		{
			name:      "a finding on its own degrades",
			findings:  []validate.Finding{invalid},
			wantState: StateDegraded,
			wantCode:  app.CodeKnowledgeInvalid,
		},
		{
			name:      "a finding is reported ahead of a merely unreadable record",
			problems:  []loader.Problem{unreadableRecord(".mindrail/knowledge/decisions/dec-9.json", false)},
			findings:  []validate.Finding{invalid},
			wantState: StateDegraded,
			wantCode:  app.CodeKnowledgeInvalid,
		},
		{
			name:      "an unreadable record with no finding reports what it always did",
			problems:  []loader.Problem{unreadableRecord(".mindrail/knowledge/decisions/dec-9.json", false)},
			wantState: StateDegraded,
			wantCode:  app.CodeKnowledgeUnreadable,
		},
		{
			name:      "a store with neither is untouched",
			wantState: StateOK,
			wantCode:  "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			subject := healthySubject()
			subject.Knowledge.Problems = tc.problems
			subject.KnowledgeFindings = tc.findings

			got := KnowledgeCheck(subject).Run(t.Context())

			if got.State != tc.wantState {
				t.Errorf("state = %q, want %q (%+v)", got.State, tc.wantState, got)
			}
			if got.Code != tc.wantCode {
				t.Errorf("code = %q, want %q", got.Code, tc.wantCode)
			}
			assertDiagnosable(t, got)
		})
	}
}

// TestAFindingIsDistinguishableFromALoaderProblemByItsCode is AC-06.2's other
// half.
//
// The two conditions have opposite remedies — one is fixed by upgrading
// Mindrail, the other by editing a file this repository owns (decision D-38) —
// and a consumer branches on the code, not on the prose. So the same record path
// reported as unreadable and reported as invalid must not arrive under one code,
// and neither may borrow the other's rendering helper.
func TestAFindingIsDistinguishableFromALoaderProblemByItsCode(t *testing.T) {
	const path = ".mindrail/knowledge/decisions/dec-1.json"

	unreadable := healthySubject()
	unreadable.Knowledge.Problems = []loader.Problem{unreadableRecord(path, false)}

	wrong := healthySubject()
	wrong.KnowledgeFindings = []validate.Finding{findingAt(path, validate.StepSchema)}

	fromProblem := KnowledgeCheck(unreadable).Run(t.Context())
	fromFinding := KnowledgeCheck(wrong).Run(t.Context())

	if fromProblem.Code == fromFinding.Code {
		t.Fatalf("one record path reported as unreadable and as invalid carries one code %q", fromProblem.Code)
	}
	if fromProblem.State != fromFinding.State {
		t.Errorf("the two readings differ in state (%q vs %q); this test proves the codes differ for readings that do not",
			fromProblem.State, fromFinding.State)
	}
	if fromProblem.Summary == fromFinding.Summary {
		t.Errorf("both readings summarise as %q", fromProblem.Summary)
	}
	if reflect.DeepEqual(fromProblem.NextAction, fromFinding.NextAction) {
		t.Errorf("both readings offer the same remedy %v, so one of them is wrong about what to do", fromProblem.NextAction)
	}
}

// TestFindingDiagnosticsNameTheRuleAndTheFile is AC-06.2 and AC-06.4 per step.
//
// The label table is written out here rather than read from knowledgeStepNames:
// an assertion that compares a function to the constant it reads passes whatever
// either says, which is the entire second MR-001 audit round in one line.
func TestFindingDiagnosticsNameTheRuleAndTheFile(t *testing.T) {
	wantLabel := map[validate.Step]string{
		validate.StepSchema:                 "step 5 (JSON Schema validation)",
		validate.StepFilenameConsistency:    "step 6 (filename <-> kind/id consistency)",
		validate.StepUniqueID:               "step 7 (unique ID)",
		validate.StepSupersedeTarget:        "step 8 (supersede target)",
		validate.StepSupersedeCycle:         "step 9 (supersede DAG/cycle)",
		validate.StepDuplicateActiveLineage: "step 10 (duplicate active lineage)",
		validate.StepScopeSyntax:            "step 11 (scope syntax)",
	}
	if len(wantLabel) != len(pipelineSteps) {
		t.Fatalf("the label table holds %d steps and the pipeline has %d; one of them has been edited alone",
			len(wantLabel), len(pipelineSteps))
	}

	const path = ".mindrail/knowledge/decisions/dec-4.json"
	remedies := make(map[string]validate.Step, len(pipelineSteps))

	for _, step := range pipelineSteps {
		t.Run(fmt.Sprintf("step %d", int(step)), func(t *testing.T) {
			subject := healthySubject()
			subject.KnowledgeFindings = []validate.Finding{findingAt(path, step)}

			got := KnowledgeCheck(subject).Run(t.Context())

			assertDiagnosable(t, got)
			if !strings.Contains(got.Diagnostic, wantLabel[step]) {
				t.Errorf("diagnostic does not name the rule as %q:\n%s", wantLabel[step], got.Diagnostic)
			}
			if !strings.Contains(got.Diagnostic, path) {
				t.Errorf("diagnostic does not name the offending file %q:\n%s", path, got.Diagnostic)
			}

			if len(got.NextAction) != 1 {
				t.Fatalf("one finding produced %d remedies: %v", len(got.NextAction), got.NextAction)
			}
			if !strings.Contains(got.NextAction[0], path) {
				t.Errorf("remedy %q does not name the offending file %q; status prints this field with no diagnostic beside it",
					got.NextAction[0], path)
			}
			if owner, taken := remedies[got.NextAction[0]]; taken {
				t.Errorf("step %d and step %d ask the reader for the same thing: %q",
					int(step), int(owner), got.NextAction[0])
			}
			remedies[got.NextAction[0]] = step
		})
	}
}

// TestEveryPipelineStepIsNamedAndRemediedInItsOwnWords stops a step falling
// through to the deferred-step fallbacks unnoticed.
//
// Both fallbacks are real and reachable — steps 12, 13 and 14 exist and are
// deferred (AC-12.2), so a finding from one must still leave the reader a number
// and a file rather than nothing. What must not happen is a step this milestone
// does implement quietly arriving there because its row was deleted.
func TestEveryPipelineStepIsNamedAndRemediedInItsOwnWords(t *testing.T) {
	if got, want := knowledgeStep(deferredStep), "step 12"; got != want {
		t.Fatalf("knowledgeStep(%d) = %q, want %q: a step with no name is still reported by number",
			int(deferredStep), got, want)
	}
	deferredRemedy := remedyFormatFor(deferredStep)
	if !strings.Contains(deferredRemedy, "%s") {
		t.Fatalf("the fallback remedy %q has nowhere to put the offending path", deferredRemedy)
	}

	for _, step := range pipelineSteps {
		if got := knowledgeStep(step); got == fmt.Sprintf("step %d", int(step)) {
			t.Errorf("step %d renders as the bare number %q; this milestone implements it and owes it a name",
				int(step), got)
		}
		if got := remedyFormatFor(step); got == deferredRemedy {
			t.Errorf("step %d falls back to the deferred-step remedy %q; this milestone implements it and owes it an action",
				int(step), got)
		}
	}
}

// TestFindingRemediesNameOneRecordEachAndStopAtTheCap is AC-06.4.
//
// Twelve offending files and the expected counts are written as literals rather
// than derived from maxNamedRecords: a bound that scales with the constant it
// guards passes for every value of that constant, which is precisely the class
// of assertion MR-001's second audit round was spent deleting.
func TestFindingRemediesNameOneRecordEachAndStopAtTheCap(t *testing.T) {
	t.Run("one record with many findings is one action", func(t *testing.T) {
		const path = ".mindrail/knowledge/decisions/dec-1.json"

		// Every step but 9. A cycle finding would route this to the other
		// branch, where a single finding produces a single remedy whether or not
		// the deduplication works — the assertion below would then hold for the
		// wrong reason.
		subject := healthySubject()
		for _, step := range pipelineSteps {
			if step == validate.StepSupersedeCycle {
				continue
			}
			subject.KnowledgeFindings = append(subject.KnowledgeFindings, findingAt(path, step))
		}
		if len(subject.KnowledgeFindings) != 6 {
			t.Fatalf("the fixture holds %d findings, want 6", len(subject.KnowledgeFindings))
		}

		got := KnowledgeCheck(subject).Run(t.Context())

		if got.Code != app.CodeKnowledgeInvalid {
			t.Fatalf("code = %q, want %q: this row must exercise the many-findings branch", got.Code, app.CodeKnowledgeInvalid)
		}
		if len(got.NextAction) != 1 {
			t.Fatalf("six findings against one file produced %d remedies: %v", len(got.NextAction), got.NextAction)
		}
	})

	t.Run("twelve records are named ten times and counted once", func(t *testing.T) {
		subject := healthySubject()
		paths := make([]string, 0, 12)
		for i := 1; i <= 12; i++ {
			path := fmt.Sprintf(".mindrail/knowledge/decisions/dec-%02d.json", i)
			paths = append(paths, path)
			subject.KnowledgeFindings = append(subject.KnowledgeFindings, findingAt(path, validate.StepSchema))
		}

		got := KnowledgeCheck(subject).Run(t.Context())

		if len(got.NextAction) != 11 {
			t.Fatalf("twelve offending files produced %d remedies, want 11 (ten named, one tail): %v",
				len(got.NextAction), got.NextAction)
		}
		for _, path := range paths[:10] {
			if !strings.Contains(strings.Join(got.NextAction, "\n"), path) {
				t.Errorf("remedies never name %q: %v", path, got.NextAction)
			}
		}
		tail := got.NextAction[10]
		if !strings.Contains(tail, "remaining 2") {
			t.Errorf("tail remedy %q does not count the two files it did not name", tail)
		}
		if !strings.Contains(tail, loader.StoreRoot) {
			t.Errorf("tail remedy %q does not say where the remaining records are", tail)
		}
		for _, path := range paths[10:] {
			if strings.Contains(tail, path) {
				t.Errorf("tail remedy %q names %q, so the cap did not apply", tail, path)
			}
		}
		assertDiagnosable(t, got)
	})
}

// TestAReadingOfAStoreThatNeverLoadedIsBlindToFindings is AC-06.5.
//
// The reading produced when startup never reached the knowledge step must claim
// nothing about the knowledge store, and a findings count is a claim. The row
// beside it is the halt at the step itself: the store did not load, so any
// finding attached to it would be a verdict about records nobody read.
//
// Both assertions are an equality between two subjects differing only in the
// field, so no wording change can satisfy them and no new sentence can slip past
// them.
func TestAReadingOfAStoreThatNeverLoadedIsBlindToFindings(t *testing.T) {
	const path = ".mindrail/knowledge/decisions/dec-1.json"

	tests := []struct {
		name     string
		halt     step
		wantCode app.Code
	}{
		{
			name:     "startup stopped before the knowledge step",
			halt:     stepOpenSQLite,
			wantCode: app.CodeStartupIncomplete,
		},
		{
			name:     "the knowledge step ran and the store did not load",
			halt:     stepValidateKnowledge,
			wantCode: app.CodeKnowledgeUnreadable,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			carrying := haltedSubject(tc.halt)
			carrying.KnowledgeFindings = []validate.Finding{
				findingAt(path, validate.StepSupersedeCycle),
				findingAt(path, validate.StepSchema),
			}

			silent := KnowledgeCheck(haltedSubject(tc.halt)).Run(t.Context())
			loud := KnowledgeCheck(carrying).Run(t.Context())

			if !reflect.DeepEqual(silent, loud) {
				t.Fatalf("the reading changed when findings were attached:\n without: %+v\n with:    %+v", silent, loud)
			}
			if loud.Code != tc.wantCode {
				t.Errorf("code = %q, want %q", loud.Code, tc.wantCode)
			}
			if strings.Contains(loud.Diagnostic, path) || strings.Contains(strings.Join(loud.NextAction, "\n"), path) {
				t.Errorf("a reading of a store that never loaded names a record: %+v", loud)
			}
		})
	}
}

// TestTheKnowledgeReadingSurvivesAFindingFromEveryStep is the systemic guard the
// per-branch tables cannot make.
//
// Whatever step produced it, a finding drives the check off OK and therefore owes
// its reader a code, a diagnostic, an impact and an action that can be carried
// out. It runs the whole runner rather than the single check so the report-wide
// assertions apply too.
func TestTheKnowledgeReadingSurvivesAFindingFromEveryStep(t *testing.T) {
	for _, step := range append(append([]validate.Step{}, pipelineSteps...), deferredStep) {
		t.Run(fmt.Sprintf("step %d", int(step)), func(t *testing.T) {
			subject := healthySubject()
			subject.KnowledgeFindings = []validate.Finding{findingAt(".mindrail/knowledge/decisions/dec-1.json", step)}

			report := NewRunner(DefaultChecks(subject)...).Run(t.Context())

			knowledge := Result{}
			for _, result := range report.Checks {
				assertDiagnosable(t, result)
				if result.Name == checkKnowledge {
					knowledge = result
				}
			}
			if knowledge.State == StateOK {
				t.Fatalf("a finding left the knowledge reading OK: %+v", knowledge)
			}
		})
	}
}

// --- the pipeline's own output ----------------------------------------------

// shippedValidator compiles the documents this binary embeds. It needs no disk:
// the documents are an embed.FS and validate.Check is a pure function of a
// store, which is what lets the rows below be real pipeline output rather than
// findings a test invented.
func shippedValidator(t *testing.T) *schema.Validator {
	t.Helper()

	registry, err := schema.NewRegistry(schemas.KnowledgeFS)
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	validator, err := schema.NewValidator(registry)
	if err != nil {
		t.Fatalf("NewValidator: %v", err)
	}
	return validator
}

// decisionRef builds the record the loader would have produced for one decision
// document, including the raw bytes decision D-46 carries.
func decisionRef(t *testing.T, id string, body map[string]any) loader.RecordRef {
	t.Helper()

	encoded, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal %s: %v", id, err)
	}
	return loader.RecordRef{
		Kind:          loader.KindDecision,
		ID:            id,
		Path:          ".mindrail/knowledge/decisions/" + id + ".json",
		SchemaVersion: 1,
		Body:          encoded,
	}
}

func decisionBody(id, status string, supersedes ...string) map[string]any {
	body := map[string]any{
		"schema_version": 1,
		"kind":           "decision",
		"id":             id,
		"status":         status,
		"created_at":     "2026-03-14T09:26:53Z",
		"title":          "A decision",
		"decision":       "We chose this.",
	}
	if len(supersedes) > 0 {
		body["supersedes"] = supersedes
	}
	return body
}

// TestTheDoctorBranchesFireOnWhatThePipelineActuallyEmits is what makes every
// hand-built fixture above evidence.
//
// splitFindings keys on the Code, and nothing the doctor package can see
// guarantees that the code validate stamps on a step-9 finding is the code this
// branch looks for. So these three rows run the real pipeline over real record
// bytes and assert the reading it drives: a genuine cycle, a genuine schema
// violation, and — the over-fire guard — a legitimate supersede, which is the
// adjacent healthy shape a cycle detector gets wrong first.
func TestTheDoctorBranchesFireOnWhatThePipelineActuallyEmits(t *testing.T) {
	validator := shippedValidator(t)

	tests := []struct {
		name         string
		decisions    []loader.RecordRef
		wantState    State
		wantCode     app.Code
		wantFindings bool
	}{
		{
			name: "a lineage that closes on itself",
			decisions: []loader.RecordRef{
				decisionRef(t, "DEC-0001", decisionBody("DEC-0001", "active", "DEC-0002")),
				decisionRef(t, "DEC-0002", decisionBody("DEC-0002", "active", "DEC-0001")),
			},
			wantState:    StateError,
			wantCode:     app.CodeKnowledgeSupersedeCycle,
			wantFindings: true,
		},
		{
			name: "a record its own schema rejects",
			decisions: []loader.RecordRef{
				decisionRef(t, "DEC-0001", decisionBody("DEC-0001", "handwritten")),
			},
			wantState:    StateDegraded,
			wantCode:     app.CodeKnowledgeInvalid,
			wantFindings: true,
		},
		{
			name: "a legitimate supersede is not a cycle",
			decisions: []loader.RecordRef{
				decisionRef(t, "DEC-0001", decisionBody("DEC-0001", "superseded")),
				decisionRef(t, "DEC-0002", decisionBody("DEC-0002", "active", "DEC-0001")),
			},
			wantState: StateOK,
			wantCode:  "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			store := loader.Store{
				Present:                true,
				Root:                   loader.StoreRoot,
				Decisions:              tc.decisions,
				WriteSchemaVersion:     1,
				ReadableSchemaVersions: []int{1},
			}

			subject := healthySubject()
			subject.Knowledge = store
			subject.KnowledgeFindings = validate.Check(store, validator)

			if found := len(subject.KnowledgeFindings) > 0; found != tc.wantFindings {
				t.Fatalf("the pipeline reported %d findings, want any = %v: %+v",
					len(subject.KnowledgeFindings), tc.wantFindings, subject.KnowledgeFindings)
			}

			got := KnowledgeCheck(subject).Run(t.Context())

			if got.State != tc.wantState {
				t.Errorf("state = %q, want %q (%+v)", got.State, tc.wantState, got)
			}
			if got.Code != tc.wantCode {
				t.Errorf("code = %q, want %q", got.Code, tc.wantCode)
			}
			assertDiagnosable(t, got)
		})
	}
}
