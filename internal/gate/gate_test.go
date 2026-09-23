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
	decision := evaluate(t, service, gate.Input{
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
	clean := evaluate(t, service, gate.Input{})
	if !clean.Allow || len(clean.Denials) != 0 {
		t.Fatalf("empty = %+v", clean)
	}
	warnings := evaluate(t, service, gate.Input{
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
	decision := evaluate(t, service, gate.Input{
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
	first := evaluate(t, service, input)
	second := evaluate(t, service, input)
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
	third := evaluate(t, service, reordered)
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

func evaluate(t *testing.T, service *gate.Service, input gate.Input) gate.Decision {
	t.Helper()
	decision, err := service.Evaluate(input)
	if err != nil {
		t.Fatal(err)
	}
	return decision
}

// TestGateRefusesVacuousBlocking is the Breaker close-out: blocking
// findings without code/key and unknown binding statuses refuse loudly
// instead of denying vaguely or passing silently.
func TestGateRefusesVacuousBlocking(t *testing.T) {
	service := gate.New()
	if _, err := service.Evaluate(gate.Input{
		Attribution: []changes.Finding{{Blocking: true}},
	}); err == nil {
		t.Fatal("vacuous attribution accepted")
	}
	if _, err := service.Evaluate(gate.Input{
		Guard: []testguard.Finding{{Blocking: true}},
	}); err == nil {
		t.Fatal("vacuous guard accepted")
	}
	if _, err := service.Evaluate(gate.Input{
		Bindings: []gate.InvariantBlock{{
			InvariantID: "INV-1", UID: "SYM-1", Status: "haunted",
			Severity: "CRITICAL", Active: true,
		}},
	}); err == nil {
		t.Fatal("unknown status accepted")
	}
}

// TestGateDedupesIdenticalDenials pins one listing per fact: a driver
// reporting one binding twice still decides a single denial.
func TestGateDedupesIdenticalDenials(t *testing.T) {
	service := gate.New()
	block := gate.InvariantBlock{
		InvariantID: "INV-1", UID: "SYM-1", Status: "orphaned",
		Severity: "CRITICAL", Active: true,
	}
	decision := evaluate(t, service, gate.Input{Bindings: []gate.InvariantBlock{block, block}})
	if decision.Allow || len(decision.Denials) != 1 {
		t.Fatalf("decision = %+v", decision)
	}
}

// TestGateAmbiguousBindingDenies pins the Reader-noted path: an ambiguous
// binding under an active CRITICAL invariant denies with the identity
// code, not the orphan code.
func TestGateAmbiguousBindingDenies(t *testing.T) {
	service := gate.New()
	decision := evaluate(t, service, gate.Input{
		Bindings: []gate.InvariantBlock{{
			InvariantID: "INV-1", UID: "SYM-1", Status: "ambiguous",
			Severity: "CRITICAL", Active: true,
		}},
	})
	if decision.Allow || len(decision.Denials) != 1 {
		t.Fatalf("decision = %+v", decision)
	}
	if decision.Denials[0].Code != app.CodeSymbolIdentityAmbiguous {
		t.Fatalf("code = %q", decision.Denials[0].Code)
	}
}
