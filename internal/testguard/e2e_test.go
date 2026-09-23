package testguard_test

import (
	"testing"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/testguard"
)

// TestWeakeningScenarioEndToEnd is TASK-03 AC-03.1: the frozen §1 scenario
// in one evaluation — an assertion loss that warns, a skip that warns, a
// removed CRITICAL verification test that blocks without any production
// edit, and a marked sibling listed as suppressed. Every trigger path
// answers identically.
func TestWeakeningScenarioEndToEnd(t *testing.T) {
	service := newGuardService(t)
	before := "def test_auth():\n" +
		"    assert login()\n" +
		"    assert session()\n" +
		"    assert token()\n" +
		"def test_session():\n" +
		"    assert start()\n" +
		"def test_token():\n" +
		"    assert issue()\n" +
		"# mindrail: allow-test-weakening\n" +
		"def test_legacy():\n" +
		"    assert old()\n" +
		"    assert older()\n"
	after := "def test_auth():\n" +
		"    assert login()\n" +
		"@pytest.mark.skip\n" +
		"def test_session():\n" +
		"    assert start()\n" +
		"# mindrail: allow-test-weakening\n" +
		"def test_legacy():\n" +
		"    pass\n"
	mappings := []testguard.TestMapping{{
		Path: "tests/test_auth.py", Test: "test_token",
		ProductionUID: "SYM-TOKEN", InvariantID: "INV-CRIT",
		Severity: "CRITICAL", Active: true,
	}}

	for _, trigger := range []string{
		testguard.TriggerAfterChange, testguard.TriggerReconcile,
		testguard.TriggerStaged, testguard.TriggerCI,
	} {
		result, err := service.Evaluate(t.Context(), testguard.Request{
			Files: []testguard.FileDelta{{
				Path: "tests/test_auth.py", Language: testguard.LanguagePython,
				Before: []byte(before), After: []byte(after),
			}},
			Mappings: mappings,
			Trigger:  trigger,
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(result.Findings) != 3 {
			t.Fatalf("trigger %s: findings = %+v", trigger, result.Findings)
		}
		bySignal := map[string]testguard.Finding{}
		for _, finding := range result.Findings {
			bySignal[finding.Signal] = finding
			if finding.Code != string(app.CodeTestGuardWeakened) {
				t.Fatalf("code = %q", finding.Code)
			}
			if finding.Confidence != 0.5 || finding.Reason == "" {
				t.Fatalf("unexplained: %+v", finding)
			}
			if finding.Trigger != trigger {
				t.Fatalf("trigger = %q", finding.Trigger)
			}
		}
		decrease, ok := bySignal[testguard.SignalAssertionCountDecreased]
		if !ok || decrease.Blocking {
			t.Fatalf("decrease = %+v, want warning", bySignal)
		}
		skipped, ok := bySignal[testguard.SignalTestSkipped]
		if !ok || skipped.Blocking {
			t.Fatalf("skipped = %+v, want warning", bySignal)
		}
		removed, ok := bySignal[testguard.SignalCriticalTestRemoved]
		if !ok || !removed.Blocking {
			t.Fatalf("removed = %+v, want blocking", bySignal)
		}
		if removed.ProductionUID != "SYM-TOKEN" || removed.InvariantID != "INV-CRIT" {
			t.Fatalf("removed = %+v", removed)
		}
		if len(result.Suppressions) != 1 ||
			result.Suppressions[0].TestKey != "tests/test_auth.py::test_legacy" {
			t.Fatalf("suppressions = %+v", result.Suppressions)
		}
	}
}
