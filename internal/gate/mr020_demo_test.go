package gate_test

// TEMPORARY MR-020 demonstration driver — deleted before milestone close
// (D-241). Verdicts come from production service calls
// (validation.Service.RunProfile, validation.Check, gate.Service.Evaluate)
// over a scratch repository; t.Log carries the verbatim outputs.

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/PsyChaos/mindrail/internal/config"
	"github.com/PsyChaos/mindrail/internal/gate"
	"github.com/PsyChaos/mindrail/internal/validation"
)

// TestMR020DemoStaleEvidence is MR-020 AC-03.2 across two separate checkouts:
// agent A validates in copy A; agent B works in copy B (identical tree).
// Agent A's old result claimed over copy B's edited tree is refused as
// stale with the profile to re-run named; a fresh run restores ALLOW
// (AC-01.1's evidence half).
func TestMR020DemoStaleEvidence(t *testing.T) {
	fx := newKernelFixture(t)
	const testFile = "def test_x():\n    assert True\n"
	mkCopy := func() string {
		dir := t.TempDir()
		tests := filepath.Join(dir, "tests")
		if err := os.MkdirAll(tests, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(tests, "t.py"), []byte(testFile), 0o644); err != nil {
			t.Fatal(err)
		}
		return dir
	}
	copyA, copyB := mkCopy(), mkCopy()
	profile := config.ValidationProfile{
		Type:     "AUTOMATED_TEST",
		Paths:    []string{"tests"},
		Commands: [][]string{{"echo", "hi"}},
	}

	evidences, err := fx.valid.RunProfile(t.Context(), "test", profile, copyA, nil, "OP-AGENT-A-1")
	if err != nil {
		t.Fatalf("agent-A RunProfile: %v", err)
	}
	if len(evidences) == 0 {
		t.Fatal("no evidence recorded")
	}
	t.Logf("agent-A evidence (copy A): profile=%s snapshot=%s", evidences[0].Profile, evidences[0].SnapshotHash)

	// Honest completion in copy A: current evidence, no edits.
	_, coverageA, err := validation.Check(copyA, evidences, []string{"test"})
	if err != nil {
		t.Fatalf("copy-A Check: %v", err)
	}
	honest, err := fx.gate.Evaluate(gate.Input{Coverage: coverageA})
	if err != nil {
		t.Fatalf("honest Evaluate: %v", err)
	}
	t.Logf("honest completion (copy A, unedited): allow=%v denials=%+v", honest.Allow, honest.Denials)
	if !honest.Allow {
		t.Fatalf("honest run denied: %+v", honest.Denials)
	}

	// Cross-copy attempt (logged, not required): agent A's rows claimed over
	// copy B's tree fail safe — provenance scope is absolute, so the check
	// refuses instead of calling foreign bytes current.
	xv, _, err := validation.Check(copyB, evidences, []string{"test"})
	if err != nil {
		t.Fatalf("cross-copy Check: %v", err)
	}
	for _, v := range xv {
		t.Logf("cross-copy verdict: profile=%s status=%s reason=%q", v.Profile, v.Status, v.Reason)
	}

	// Agent B validates in copy B, then edits the validated scope there
	// and claims the old result as proof.
	evB, err := fx.valid.RunProfile(t.Context(), "test", profile, copyB, nil, "OP-AGENT-B-1")
	if err != nil {
		t.Fatalf("agent-B RunProfile: %v", err)
	}
	_, coverageB, err := validation.Check(copyB, evB, []string{"test"})
	if err != nil {
		t.Fatalf("copy-B fresh Check: %v", err)
	}
	if !coverageB.Satisfied["test"] {
		t.Fatalf("copy-B fresh coverage unsatisfied: %+v", coverageB)
	}
	if err := os.WriteFile(filepath.Join(copyB, "tests", "t.py"),
		[]byte("def test_x():\n    assert True\n\ndef test_y():\n    assert True\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	verdicts, staleCoverage, err := validation.Check(copyB, evB, []string{"test"})
	if err != nil {
		t.Fatalf("stale Check: %v", err)
	}
	for _, v := range verdicts {
		t.Logf("freshness verdict: profile=%s status=%s reason=%q", v.Profile, v.Status, v.Reason)
		if v.Status == validation.FreshCurrent {
			t.Fatalf("edited scope still current: %+v", v)
		}
	}
	decision, err := fx.gate.Evaluate(gate.Input{Coverage: staleCoverage})
	if err != nil {
		t.Fatalf("stale Evaluate: %v", err)
	}
	t.Logf("stale-proof completion: allow=%v denials=%+v", decision.Allow, decision.Denials)
	if decision.Allow {
		t.Fatal("stale evidence accepted as proof")
	}
	if len(decision.Denials) != 1 || string(decision.Denials[0].Code) != "REQUIRED_EVIDENCE_NOT_CURRENT" {
		t.Fatalf("denials = %+v, want single REQUIRED_EVIDENCE_NOT_CURRENT", decision.Denials)
	}
	if len(decision.Denials[0].NextAction) == 0 {
		t.Fatal("stale denial names no remedy")
	}

	// Agent A re-runs the named profile over copy B; the proof is current again.
	fresh, err := fx.valid.RunProfile(t.Context(), "test", profile, copyB, nil, "OP-AGENT-A-2")
	if err != nil {
		t.Fatalf("re-run: %v", err)
	}
	_, recovered, err := validation.Check(copyB, fresh, []string{"test"})
	if err != nil {
		t.Fatalf("recovered Check: %v", err)
	}
	again, err := fx.gate.Evaluate(gate.Input{Coverage: recovered})
	if err != nil {
		t.Fatalf("re-run Evaluate: %v", err)
	}
	t.Logf("re-run completion: allow=%v denials=%+v", again.Allow, again.Denials)
	if !again.Allow {
		t.Fatalf("re-run still denied: %+v", again.Denials)
	}
}
