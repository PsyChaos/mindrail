package status

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/PsyChaos/mindrail/internal/doctor"
)

// TestReadinessEnumIsExactlyFourValues pins the spec §107 overall vocabulary.
// It is a separate enum from doctor.State and status.TerminalState on purpose
// (decision D-16): the three describe a component, the whole installation and
// the terminal verdict of init, and collapsing them would lose a distinction
// the spec makes deliberately.
func TestReadinessEnumIsExactlyFourValues(t *testing.T) {
	want := []Readiness{ReadinessPartialReady, ReadinessReady, ReadinessDegraded, ReadinessBlocked}

	got := Readinesses()
	if len(got) != len(want) {
		t.Fatalf("Readinesses() = %v, want exactly %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("Readinesses()[%d] = %q, want %q", i, got[i], want[i])
		}
		if !want[i].Valid() {
			t.Errorf("%q is in Readinesses() but Valid() is false", want[i])
		}
		if string(want[i]) != strings.ToUpper(string(want[i])) {
			t.Errorf("readiness %q is not uppercase on the wire", want[i])
		}
	}

	got[0] = Readiness("MUTATED")
	if Readinesses()[0] != ReadinessPartialReady {
		t.Error("Readinesses() returned its backing array; the enum is mutable")
	}
}

// TestReadinessUnmarshalRejectsUnknown proves the decoder fails closed, so a
// readiness value from a persisted report or another process cannot enter the
// process unexamined.
func TestReadinessUnmarshalRejectsUnknown(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    Readiness
		wantErr bool
	}{
		{name: "ready", input: `"READY"`, want: ReadinessReady},
		{name: "partial", input: `"PARTIAL_READY"`, want: ReadinessPartialReady},
		{name: "degraded", input: `"DEGRADED"`, want: ReadinessDegraded},
		{name: "blocked", input: `"BLOCKED"`, want: ReadinessBlocked},
		{name: "lowercase", input: `"ready"`, wantErr: true},
		{name: "doctor state", input: `"OK"`, wantErr: true},
		{name: "not applicable", input: `"NOT_APPLICABLE"`, wantErr: true},
		{name: "terminal state", input: `"READY FOR TARGETED WORK"`, wantErr: true},
		{name: "empty", input: `""`, wantErr: true},
		{name: "number", input: `4`, wantErr: true},
		{name: "null", input: `null`, wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var got Readiness
			err := json.Unmarshal([]byte(tc.input), &got)

			if tc.wantErr {
				if err == nil {
					t.Fatalf("unmarshal %s succeeded with %q, want an error", tc.input, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unmarshal %s: %v", tc.input, err)
			}
			if got != tc.want {
				t.Errorf("unmarshal %s = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

// TestComponentsMapHasExactlySixKeys pins spec §107's component list, in order.
// The names are part of the wire contract: `mindrail_status` promises the exact
// blocking component, and a renamed key silently breaks every consumer of it.
func TestComponentsMapHasExactlySixKeys(t *testing.T) {
	want := []ComponentName{
		ComponentKnowledge,
		ComponentInventory,
		ComponentSyntax,
		ComponentSemantic,
		ComponentCoverageMap,
		ComponentRuntimeDB,
	}
	wantNames := []string{"knowledge", "inventory", "syntax", "semantic", "coverage_map", "runtime_db"}

	got := Components()
	if len(got) != len(want) {
		t.Fatalf("Components() = %v, want exactly %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("Components()[%d] = %q, want %q", i, got[i], want[i])
		}
		if string(got[i]) != wantNames[i] {
			t.Errorf("Components()[%d] is spelled %q, want %q", i, got[i], wantNames[i])
		}
	}

	report := Build(healthySubject(), time.Millisecond)
	if len(report.Components) != len(want) {
		t.Fatalf("report has %d components, want exactly %d: %v", len(report.Components), len(want), report.Components)
	}
	for _, name := range want {
		component, ok := report.Components[name]
		if !ok {
			t.Errorf("report has no %q component", name)
			continue
		}
		if !component.State.Valid() {
			t.Errorf("component %q reported invalid state %q", name, component.State)
		}
		if strings.TrimSpace(component.Summary) == "" {
			t.Errorf("component %q reported no summary", name)
		}
	}

	got[0] = ComponentName("mutated")
	if Components()[0] != ComponentKnowledge {
		t.Error("Components() returned its backing array; the list is mutable")
	}
}

// TestTerminalStateSpellingIsTheSpecLiteral pins spec §82. The two strings are
// what a human and a CI grep look for at the end of init, so they are frozen
// text, not a description.
func TestTerminalStateSpellingIsTheSpecLiteral(t *testing.T) {
	if TerminalReady != "READY FOR TARGETED WORK" {
		t.Errorf("TerminalReady = %q, want %q", TerminalReady, "READY FOR TARGETED WORK")
	}
	if TerminalBlocked != "BLOCKED" {
		t.Errorf("TerminalBlocked = %q, want %q", TerminalBlocked, "BLOCKED")
	}
	if doctor.State(TerminalBlocked).Valid() {
		t.Error("TerminalBlocked collides with the doctor state vocabulary")
	}
}
