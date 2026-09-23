package testguard_test

import (
	"testing"

	"github.com/PsyChaos/mindrail/internal/testguard"
)

func mappedRequest(delta testguard.FileDelta, mappings []testguard.TestMapping, trigger string) testguard.Request {
	return testguard.Request{Files: []testguard.FileDelta{delta}, Mappings: mappings, Trigger: trigger}
}

func criticalMapping(path, test string) testguard.TestMapping {
	return testguard.TestMapping{Path: path, Test: test,
		ProductionUID: "SYM-PROD", InvariantID: "INV-1", Severity: "CRITICAL", Active: true}
}

// TestMappedCriticalRemovalBlocks is TASK-02 AC-02.2: removing the test
// that verifies an active CRITICAL invariant blocks with production and
// invariant named — even with no production edit in sight.
func TestMappedCriticalRemovalBlocks(t *testing.T) {
	service := newGuardService(t)
	result, err := service.Evaluate(t.Context(), mappedRequest(
		guardDelta("tests/t.py",
			"def test_token():\n    assert issue()\n",
			"def test_other():\n    assert x\n"),
		[]testguard.TestMapping{criticalMapping("tests/t.py", "test_token")},
		testguard.TriggerAfterChange))
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Findings) != 1 {
		t.Fatalf("findings = %+v", result.Findings)
	}
	finding := result.Findings[0]
	if finding.Signal != testguard.SignalCriticalTestRemoved {
		t.Fatalf("signal = %q", finding.Signal)
	}
	if !finding.Blocking {
		t.Fatal("CRITICAL removal does not block")
	}
	if finding.ProductionUID != "SYM-PROD" || finding.InvariantID != "INV-1" {
		t.Fatalf("finding = %+v", finding)
	}
}

// TestPolicyMatrixBlocking is TASK-02 AC-02.2 + AC-02.5: the documented
// block/warn table — only active-CRITICAL removal/disable blocks.
func TestPolicyMatrixBlocking(t *testing.T) {
	service := newGuardService(t)
	weaken := guardDelta("t.py",
		"def test_a():\n    assert x\n    assert y\n",
		"def test_a():\n    assert x\n")
	skipped := guardDelta("t.py",
		"def test_a():\n    assert x\n",
		"@pytest.mark.skip\ndef test_a():\n    assert x\n")
	xfailed := guardDelta("t.py",
		"def test_a():\n    assert x\n",
		"@pytest.mark.xfail\ndef test_a():\n    assert x\n")
	removed := guardDelta("t.py",
		"def test_a():\n    assert x\n",
		"def test_b():\n    assert x\n")
	critical := criticalMapping("t.py", "test_a")
	high := critical
	high.Severity = "HIGH"
	inactive := critical
	inactive.Active = false
	suiteDisabled := testguard.FileDelta{Path: "w.test.ts", Language: testguard.LanguageTypeScript,
		Before: []byte("it(\"auth\", () => { expect(login()).toBe(true); });\n"),
		After:  []byte("describe.skip(\"s\", () => { it(\"auth\", () => { expect(login()).toBe(true); }); });\n")}
	suiteCritical := testguard.TestMapping{Path: "w.test.ts", Test: "auth",
		ProductionUID: "SYM-PROD", InvariantID: "INV-1", Severity: "CRITICAL", Active: true}

	for _, tc := range []struct {
		name    string
		delta   testguard.FileDelta
		mapping testguard.TestMapping
		block   bool
	}{
		{"critical removal blocks", removed, critical, true},
		{"high removal warns", removed, high, false},
		{"inactive removal warns", removed, inactive, false},
		{"critical skip warns", skipped, critical, false},
		{"critical xfail warns", xfailed, critical, false},
		{"critical decrease warns", weaken, critical, false},
		{"critical suite disable blocks", suiteDisabled, suiteCritical, true},
	} {
		result, err := service.Evaluate(t.Context(), mappedRequest(tc.delta,
			[]testguard.TestMapping{tc.mapping}, testguard.TriggerReconcile))
		if err != nil {
			t.Fatal(err)
		}
		if len(result.Findings) != 1 {
			t.Fatalf("%s: findings = %+v", tc.name, result.Findings)
		}
		if result.Findings[0].Blocking != tc.block {
			t.Fatalf("%s: blocking = %v", tc.name, result.Findings[0].Blocking)
		}
	}
}

// TestUnknownMappingKeysRefused pins D-172's loud garbage: a mapping that
// names no test in either version refuses before analysis.
func TestUnknownMappingKeysRefused(t *testing.T) {
	service := newGuardService(t)
	_, err := service.Evaluate(t.Context(), mappedRequest(
		guardDelta("t.py", "def test_a():\n    assert x\n", "def test_a():\n    assert x\n"),
		[]testguard.TestMapping{criticalMapping("t.py", "test_ghost")},
		testguard.TriggerStaged))
	if err == nil {
		t.Fatal("unknown mapping key accepted")
	}
}

// TestSharedServiceAcrossTriggers is TASK-02 AC-02.3: identical findings
// across all four provenances, only the trigger string differing.
func TestSharedServiceAcrossTriggers(t *testing.T) {
	service := newGuardService(t)
	delta := guardDelta("t.py",
		"def test_a():\n    assert x\n    assert y\n",
		"def test_a():\n    assert x\n")
	var first []testguard.Finding
	for _, trigger := range []string{
		testguard.TriggerAfterChange, testguard.TriggerReconcile,
		testguard.TriggerStaged, testguard.TriggerCI,
	} {
		result, err := service.Evaluate(t.Context(), mappedRequest(delta,
			[]testguard.TestMapping{criticalMapping("t.py", "test_a")}, trigger))
		if err != nil {
			t.Fatal(err)
		}
		if len(result.Findings) != 1 {
			t.Fatalf("trigger %s: %+v", trigger, result.Findings)
		}
		if result.Findings[0].Trigger != trigger {
			t.Fatalf("trigger = %q", result.Findings[0].Trigger)
		}
		stripped := result.Findings[0]
		stripped.Trigger = ""
		if first == nil {
			first = []testguard.Finding{stripped}
			continue
		}
		if stripped.Signal != first[0].Signal || stripped.BeforeSummary != first[0].BeforeSummary ||
			stripped.AfterSummary != first[0].AfterSummary || stripped.Blocking != first[0].Blocking {
			t.Fatalf("trigger %s differs: %+v vs %+v", trigger, stripped, first[0])
		}
	}
}

// TestEscapeHatchSuppressesLoudly is TASK-02 AC-02.4: marked tests suppress
// with self-disclosing listing; unmarked behavior unchanged;
// marker-without-weakening quiet.
func TestEscapeHatchSuppressesLoudly(t *testing.T) {
	service := newGuardService(t)
	marked := "# mindrail: allow-test-weakening\ndef test_a():\n    assert x\n"
	weakened := "# mindrail: allow-test-weakening\ndef test_a():\n    pass\n"
	result, err := service.Evaluate(t.Context(), mappedRequest(
		guardDelta("t.py", marked, weakened),
		[]testguard.TestMapping{criticalMapping("t.py", "test_a")},
		testguard.TriggerCI))
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Findings) != 0 {
		t.Fatalf("marked findings = %+v", result.Findings)
	}
	if len(result.Suppressions) != 1 || result.Suppressions[0].TestKey != "t.py::test_a" {
		t.Fatalf("suppressions = %+v", result.Suppressions)
	}
	quiet, err := service.Evaluate(t.Context(), mappedRequest(
		guardDelta("t.py", marked, marked),
		[]testguard.TestMapping{criticalMapping("t.py", "test_a")},
		testguard.TriggerCI))
	if err != nil {
		t.Fatal(err)
	}
	if len(quiet.Findings) != 0 || len(quiet.Suppressions) != 0 {
		t.Fatalf("marker without weakening = %+v / %+v", quiet.Findings, quiet.Suppressions)
	}
}
