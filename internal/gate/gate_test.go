package gate_test

import (
	"reflect"
	"testing"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/changes"
	"github.com/PsyChaos/mindrail/internal/gate"
	"github.com/PsyChaos/mindrail/internal/testguard"
	"github.com/PsyChaos/mindrail/internal/validation"
)

func denyCodes(decision gate.Decision) []app.Code {
	var codes []app.Code
	for _, denial := range decision.Denials {
		codes = append(codes, denial.Code)
	}
	return codes
}

// TestGateDeniesPerFamily is TASK-01 AC-01.1: one input per denial family,
// each denial carrying its deterministic code, provenance and next_action.
func TestGateDeniesPerFamily(t *testing.T) {
	service := gate.New()
	decision := service.Evaluate(gate.Input{
		Bindings: []gate.InvariantBlock{{
			InvariantID: "INV-1", UID: "SYM-1", Status: "orphaned",
			Severity: "CRITICAL", Active: true,
		}},
		Ambiguities: []gate.Ambiguity{{
			Key: "k", Candidates: []string{"a", "b"},
			InvariantID: "INV-2", Severity: "HIGH", Active: true,
		}},
		Attribution: []changes.Finding{
			changes.UnregisteredChange("TSK-1", "CHG-1", "k", "reconcile", nil),
		},
		Coverage: validation.Coverage{
			Required:  []string{"test"},
			Satisfied: map[string]bool{"test": false},
			ReRun:     []validation.ReRunItem{{Profile: "test", Reason: "no current evidence"}},
		},
		Guard: []testguard.Finding{{
			Code: testguard.SignalCriticalTestRemoved, Signal: testguard.SignalCriticalTestRemoved,
			TestKey: "t.py::test_a", Reason: "verification test removed",
			Confidence: 0.5, Blocking: true, Trigger: testguard.TriggerCI,
		}},
	})
	if decision.Allow {
		t.Fatal("mixed dirt allows")
	}
	codes := denyCodes(decision)
	want := []app.Code{
		app.CodeOrphanedProtectedSymbol,
		app.CodeRequiredEvidenceNotCurrent,
		app.CodeSymbolIdentityAmbiguous,
		app.CodeTestGuardWeakened,
		app.CodeUnregisteredChange,
	}
	if !reflect.DeepEqual(codes, want) {
		t.Fatalf("codes = %q, want %q (sorted)", codes, want)
	}
	for _, denial := range decision.Denials {
		if denial.Reason == "" || denial.Key == "" || denial.Provenance == "" ||
			len(denial.NextAction) == 0 {
			t.Fatalf("denial unexplained: %+v", denial)
		}
		if !app.IsRegistered(denial.Code) {
			t.Fatalf("code %q not registered", denial.Code)
		}
	}
}

// TestGateAllowsClean is TASK-01 AC-01.2: empty inputs decide ALLOW with
// zero denials — and warnings alone (non-blocking findings, satisfied
// coverage, bound bindings) never deny.
func TestGateAllowsClean(t *testing.T) {
	service := gate.New()
	clean := service.Evaluate(gate.Input{})
	if !clean.Allow || len(clean.Denials) != 0 {
		t.Fatalf("empty = %+v", clean)
	}
	warnings := service.Evaluate(gate.Input{
		Bindings: []gate.InvariantBlock{{
			InvariantID: "INV-1", UID: "SYM-1", Status: "bound",
			Severity: "CRITICAL", Active: true,
		}},
		Attribution: []changes.Finding{{
			Code: app.CodeScopeDrift, Blocking: false,
			Provenance: changes.Provenance{ChangeKey: "/r/a.py"},
			NextAction: []string{"remedy"},
		}},
		Coverage: validation.Coverage{
			Required:  []string{"test"},
			Satisfied: map[string]bool{"test": true},
		},
		Guard: []testguard.Finding{{
			Code: testguard.SignalTestSkipped, Signal: testguard.SignalTestSkipped,
			TestKey: "t.py::test_a", Blocking: false,
		}},
	})
	if !warnings.Allow || len(warnings.Denials) != 0 {
		t.Fatalf("warnings-only denies: %+v", warnings)
	}
}

// TestGateSeverityGatePinsWarnScope is TASK-01 AC-01.5: bindings and
// ambiguities without active HIGH/CRITICAL scope stay outside the gate.
func TestGateSeverityGatePinsWarnScope(t *testing.T) {
	service := gate.New()
	decision := service.Evaluate(gate.Input{
		Bindings: []gate.InvariantBlock{
			{InvariantID: "INV-1", UID: "SYM-1", Status: "orphaned", Severity: "MEDIUM", Active: true},
			{InvariantID: "INV-2", UID: "SYM-2", Status: "orphaned", Severity: "CRITICAL", Active: false},
			{InvariantID: "INV-3", UID: "SYM-3", Status: "ambiguous", Severity: "LOW", Active: true},
		},
		Ambiguities: []gate.Ambiguity{
			{Key: "k", Candidates: []string{"a", "b"}, Severity: "MEDIUM", Active: true},
			{Key: "j", Candidates: []string{"a"}, Severity: "CRITICAL", Active: true},
		},
	})
	if !decision.Allow || len(decision.Denials) != 0 {
		t.Fatalf("warn-scope denies: %+v", decision)
	}
}

// TestGateDecisionIdempotent is TASK-01 AC-01.3: the same inputs decide
// byte-identical outputs — structural, no timestamp anywhere.
func TestGateDecisionIdempotent(t *testing.T) {
	service := gate.New()
	input := gate.Input{
		Ambiguities: []gate.Ambiguity{{
			Key: "k", Candidates: []string{"b", "a"},
			InvariantID: "INV-2", Severity: "CRITICAL", Active: true,
		}},
		Coverage: validation.Coverage{
			Required:  []string{"lint", "test"},
			Satisfied: map[string]bool{"lint": true, "test": false},
			ReRun:     []validation.ReRunItem{{Profile: "test", Reason: "scope changed"}},
		},
	}
	first := service.Evaluate(input)
	second := service.Evaluate(input)
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("decisions differ:\n%+v\n%+v", first, second)
	}
	if first.Allow {
		t.Fatalf("dirty allows: %+v", first)
	}
	// Order independence: the same denials in different input orders
	// decide identically — the sort is load-bearing, not cosmetic.
	reordered := gate.Input{
		Ambiguities: []gate.Ambiguity{{
			Key: "k", Candidates: []string{"a", "b"},
			InvariantID: "INV-2", Severity: "CRITICAL", Active: true,
		}},
		Coverage: validation.Coverage{
			Required:  []string{"test", "lint"},
			Satisfied: map[string]bool{"lint": true, "test": false},
			ReRun:     []validation.ReRunItem{{Profile: "test", Reason: "scope changed"}},
		},
	}
	third := service.Evaluate(reordered)
	if !reflect.DeepEqual(first, third) {
		t.Fatalf("order leaks:\n%+v\n%+v", first, third)
	}
}

// TestEvidenceCodeRegistered is TASK-01 AC-01.4 at the registry.
func TestEvidenceCodeRegistered(t *testing.T) {
	if !app.IsRegistered(app.CodeRequiredEvidenceNotCurrent) {
		t.Fatal("REQUIRED_EVIDENCE_NOT_CURRENT is not registered")
	}
}
