package status

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/doctor"
	"github.com/PsyChaos/mindrail/internal/knowledge/validate"
)

// findingOn builds one finding of the kind the pipeline files against a record
// it read. The cycle is the only fatal one (decision D-39), and it is the only
// one that carries KNOWLEDGE_SUPERSEDE_CYCLE.
func findingOn(path string, cycle bool) validate.Finding {
	if cycle {
		return validate.Finding{
			Path:    path,
			ID:      "DEC-1",
			Step:    validate.StepSupersedeCycle,
			Code:    app.CodeKnowledgeSupersedeCycle,
			Message: "a lineage that closes on itself",
			Fatal:   true,
		}
	}
	return validate.Finding{
		Path:    path,
		ID:      "DEC-1",
		Step:    validate.StepSchema,
		Code:    app.CodeKnowledgeInvalid,
		Message: "a rule this record breaks",
	}
}

// TestKnowledgeFindingsAreCountedFromTheSubject is AC-07.1 and AC-07.2.
//
// The count is read off doctor.Subject, where bootstrap filed it once (decision
// D-42). Two rows with different counts are what stop the field being a constant
// that happens to be right for the fixture.
func TestKnowledgeFindingsAreCountedFromTheSubject(t *testing.T) {
	tests := []struct {
		name     string
		findings []validate.Finding
		want     int
	}{
		{name: "a clean store", want: 0},
		{
			name:     "one invalid record",
			findings: []validate.Finding{findingOn(".mindrail/knowledge/decisions/dec-1.json", false)},
			want:     1,
		},
		{
			name: "three findings across two records",
			findings: []validate.Finding{
				findingOn(".mindrail/knowledge/decisions/dec-1.json", false),
				findingOn(".mindrail/knowledge/decisions/dec-1.json", false),
				findingOn(".mindrail/knowledge/decisions/dec-2.json", false),
			},
			want: 3,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			subject := healthySubject()
			subject.KnowledgeFindings = tc.findings

			report := Build(subject, time.Millisecond)

			if report.Knowledge.Findings != tc.want {
				t.Errorf("knowledge.findings = %d, want %d", report.Knowledge.Findings, tc.want)
			}

			decoded := decodeJSON(t, report)
			value, present := lookup(decoded, "knowledge.findings")
			if !present {
				t.Fatalf("the report's JSON has no knowledge.findings key: %v", decoded)
			}
			// A count a consumer has to infer from an absent key is a count it
			// cannot tell from a report that never took it, which is why the
			// field carries no omitempty. json.Unmarshal into any gives every
			// number as a float64.
			if got, ok := value.(float64); !ok || int(got) != tc.want {
				t.Errorf("knowledge.findings decoded as %#v, want the number %d", value, tc.want)
			}

			rendered := renderHuman(t, report)
			if !strings.Contains(rendered, "Findings:") {
				t.Errorf("the human report has no Findings row:\n%s", rendered)
			}
			if !strings.Contains(rendered, "Findings:"+strings.Repeat(" ", 1)) {
				t.Errorf("the Findings row is not padded like its neighbours:\n%s", rendered)
			}
			if !strings.Contains(rendered, strconv.Itoa(tc.want)) {
				t.Errorf("the human report never shows the count %d:\n%s", tc.want, rendered)
			}
		})
	}
}

// TestTheFindingsCountIsQualifiedByObservation is the other half of AC-07.1.
//
// A field the report did not observe is still printed and still qualified, never
// omitted and never rendered as a bare zero that reads as a fact. Problems has
// carried that treatment since MR-001; Findings has to arrive with it rather
// than be given it after somebody notices.
func TestTheFindingsCountIsQualifiedByObservation(t *testing.T) {
	report := Build(haltedAtSQLiteSubject(), time.Millisecond)

	if report.Knowledge.Observation.Known() {
		t.Fatalf("observation = %q on a run that stopped before the knowledge step", report.Knowledge.Observation)
	}

	rendered := renderHuman(t, report)
	problems := rowValue(t, rendered, "Problems:")
	findings := rowValue(t, rendered, "Findings:")

	if findings != problems {
		t.Errorf("Findings renders as %q while Problems renders as %q; the two are qualified by one observation",
			findings, problems)
	}
	if !strings.Contains(findings, string(report.Knowledge.Observation)) {
		t.Errorf("Findings renders as %q and never says the reading was %q", findings, report.Knowledge.Observation)
	}
}

// rowValue pulls one labelled row out of the human report.
func rowValue(t *testing.T, rendered, label string) string {
	t.Helper()

	for _, line := range strings.Split(rendered, "\n") {
		trimmed := strings.TrimSpace(line)
		if rest, found := strings.CutPrefix(trimmed, label); found {
			return strings.TrimSpace(rest)
		}
	}
	t.Fatalf("the human report has no %q row:\n%s", label, rendered)
	return ""
}

// TestACycleBlocksAndAnInvalidRecordDoesNot is AC-07.5.
//
// The two conditions differ in exactly the way §84 says a state should: a cycle
// makes every answer about which decision is current wrong, so work cannot
// start; a malformed record costs the repository that record, and DEGRADED never
// blocks. Both directions are asserted, because a readiness model that blocked on
// both would look correct on the row that matters and quietly stop a working
// repository on the row that does not.
func TestACycleBlocksAndAnInvalidRecordDoesNot(t *testing.T) {
	tests := []struct {
		name          string
		cycle         bool
		wantReadiness Readiness
		wantBlocking  ComponentName
		wantState     doctor.State
		wantCode      app.Code
	}{
		{
			name:          "a supersede cycle blocks and names knowledge",
			cycle:         true,
			wantReadiness: ReadinessBlocked,
			wantBlocking:  ComponentKnowledge,
			wantState:     doctor.StateError,
			wantCode:      app.CodeKnowledgeSupersedeCycle,
		},
		{
			name:          "an invalid record degrades and blocks nothing",
			cycle:         false,
			wantReadiness: ReadinessDegraded,
			wantBlocking:  "",
			wantState:     doctor.StateDegraded,
			wantCode:      app.CodeKnowledgeInvalid,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			const path = ".mindrail/knowledge/decisions/dec-1.json"
			subject := healthySubject()
			subject.KnowledgeFindings = []validate.Finding{findingOn(path, tc.cycle)}

			report := Build(subject, time.Millisecond)

			if report.Readiness != tc.wantReadiness {
				t.Errorf("readiness = %q, want %q", report.Readiness, tc.wantReadiness)
			}
			if report.BlockingComponent != tc.wantBlocking {
				t.Errorf("blocking_component = %q, want %q", report.BlockingComponent, tc.wantBlocking)
			}

			component := report.Components[ComponentKnowledge]
			if component.State != tc.wantState {
				t.Errorf("knowledge component state = %q, want %q", component.State, tc.wantState)
			}
			if component.Code != tc.wantCode {
				t.Errorf("knowledge component code = %q, want %q", component.Code, tc.wantCode)
			}
			if len(component.NextAction) == 0 {
				t.Fatalf("the knowledge component reports %q with no remedy", component.State)
			}
			for i, action := range component.NextAction {
				assertActionable(t, string(ComponentKnowledge), i, action)
				if !strings.Contains(action, path) {
					t.Errorf("remedy %q does not name %q; status prints this field with no diagnostic beside it",
						action, path)
				}
			}
			if len(report.NextAction) == 0 {
				t.Errorf("readiness %q carries no next_action", report.Readiness)
			}
		})
	}
}

// TestStatusBuildsNoValidatorOfItsOwn is AC-07.2 as a property of the package
// rather than a claim about one function body.
//
// Decision D-41 puts the verdict in one place: bootstrap runs validate.Check
// once and files the result on the Subject. A second caller here would be a
// second answer about one store, and the day the two disagreed the report would
// carry both. The import set is the enforceable half of that — this package
// cannot construct a validator it cannot name.
func TestStatusBuildsNoValidatorOfItsOwn(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read the package directory: %v", err)
	}

	forbidden := []string{
		"github.com/PsyChaos/mindrail/internal/knowledge/schema",
		"github.com/PsyChaos/mindrail/internal/knowledge/validate",
		"github.com/PsyChaos/mindrail/internal/knowledge/loader",
	}

	inspected := 0
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		inspected++

		parsed, parseErr := parser.ParseFile(token.NewFileSet(), filepath.Join(".", name), nil, parser.ImportsOnly)
		if parseErr != nil {
			t.Fatalf("parse %s: %v", name, parseErr)
		}
		for _, spec := range parsed.Imports {
			imported, unquoteErr := strconv.Unquote(spec.Path.Value)
			if unquoteErr != nil {
				t.Fatalf("%s: unquote %s: %v", name, spec.Path.Value, unquoteErr)
			}
			for _, banned := range forbidden {
				if imported == banned {
					t.Errorf("%s imports %q; the knowledge verdict is bootstrap's to compute and this package's to report",
						name, imported)
				}
			}
		}
	}

	if inspected == 0 {
		t.Fatal("no production source was inspected, so this proves nothing")
	}
}
