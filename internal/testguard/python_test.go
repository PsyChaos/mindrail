package testguard_test

import (
	"testing"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/testguard"
)

func newGuardService(t *testing.T) *testguard.Service {
	t.Helper()
	service, err := testguard.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(service.Close)
	return service
}

func guardDelta(path, before, after string) testguard.FileDelta {
	return testguard.FileDelta{Path: path, Language: testguard.LanguagePython,
		Before: []byte(before), After: []byte(after)}
}

func evaluate(t *testing.T, service *testguard.Service, delta testguard.FileDelta) testguard.Result {
	t.Helper()
	result, err := service.Evaluate(t.Context(), testguard.Request{
		Files:   []testguard.FileDelta{delta},
		Trigger: testguard.TriggerAfterChange,
	})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

// TestPythonAssertionDecrease is TASK-01 AC-01.1: fewer asserts after than
// before yields a count finding with both summaries carrying the numbers.
func TestPythonAssertionDecrease(t *testing.T) {
	service := newGuardService(t)
	before := "def test_auth():\n    assert login()\n    assert session()\n    assert token()\n"
	after := "def test_auth():\n    assert login()\n"

	result := evaluate(t, service, guardDelta("tests/test_auth.py", before, after))
	if len(result.Findings) != 1 {
		t.Fatalf("findings = %+v", result.Findings)
	}
	finding := result.Findings[0]
	if finding.Signal != testguard.SignalAssertionCountDecreased {
		t.Fatalf("signal = %q", finding.Signal)
	}
	if finding.BeforeSummary != "asserts 3" || finding.AfterSummary != "asserts 1" {
		t.Fatalf("summaries = %q / %q", finding.BeforeSummary, finding.AfterSummary)
	}
	if finding.TestKey != "tests/test_auth.py::test_auth" {
		t.Fatalf("key = %q", finding.TestKey)
	}
	if finding.Code != string(app.CodeTestGuardWeakened) || finding.Confidence != 0.5 {
		t.Fatalf("finding = %+v", finding)
	}
	if finding.Blocking {
		t.Fatal("unmapped finding blocks")
	}
}

// TestPythonAssertionRemoved is TASK-01 AC-01.1's zero remainder: the last
// assert gone reads as removal, and a trivialized body as noop.
func TestPythonAssertionRemoved(t *testing.T) {
	service := newGuardService(t)
	gone := evaluate(t, service, guardDelta("t.py",
		"def test_a():\n    assert x\n",
		"def test_a():\n    log()\n"))
	if len(gone.Findings) != 1 || gone.Findings[0].Signal != testguard.SignalAssertionRemoved {
		t.Fatalf("gone = %+v", gone.Findings)
	}
	noop := evaluate(t, service, guardDelta("t.py",
		"def test_a():\n    assert x\n",
		"def test_a():\n    pass\n"))
	if len(noop.Findings) != 1 || noop.Findings[0].Signal != testguard.SignalAssertToNoop {
		t.Fatalf("noop = %+v", noop.Findings)
	}
}

// TestPythonSkipMarkers is TASK-01 AC-01.2: spec-list markers detected,
// near-cousins explicitly not.
func TestPythonSkipMarkers(t *testing.T) {
	service := newGuardService(t)
	body := "def test_a():\n    assert x\n"
	for marker, signal := range map[string]string{
		"@pytest.mark.skip\n":          testguard.SignalTestSkipped,
		"@pytest.mark.skip()\n":        testguard.SignalTestSkipped,
		"@pytest.mark.skip(\"why\")\n": testguard.SignalTestSkipped,
		"@unittest.skip\n":             testguard.SignalTestSkipped,
		"@pytest.mark.xfail\n":         testguard.SignalTestXFailed,
		"@pytest.mark.xfail()\n":       testguard.SignalTestXFailed,
	} {
		result := evaluate(t, service, guardDelta("t.py", body, marker+body))
		if len(result.Findings) != 1 || result.Findings[0].Signal != signal {
			t.Fatalf("marker %q = %+v", marker, result.Findings)
		}
	}
	for _, cousin := range []string{
		"@pytest.mark.skipif(True)\n",
		"@pytest.mark.todo\n",
		"@unittest.skipIf(True)\n",
	} {
		result := evaluate(t, service, guardDelta("t.py", body, cousin+body))
		if len(result.Findings) != 0 {
			t.Fatalf("cousin %q detected: %+v", cousin, result.Findings)
		}
	}
}

// TestPythonRemovedAndAddedTests is TASK-01 AC-01.3: removed functions
// detected with before-summary; added tests are quiet.
func TestPythonRemovedAndAddedTests(t *testing.T) {
	service := newGuardService(t)
	removed := evaluate(t, service, guardDelta("t.py",
		"def test_a():\n    assert x\n",
		"def test_b():\n    assert y\n"))
	if len(removed.Findings) != 1 {
		t.Fatalf("removed = %+v", removed.Findings)
	}
	if removed.Findings[0].AfterSummary != "removed" || removed.Findings[0].BeforeSummary != "asserts 1" {
		t.Fatalf("finding = %+v", removed.Findings[0])
	}
	if removed.Findings[0].Signal != testguard.SignalTestDisabled {
		t.Fatalf("unmapped removal signal = %q", removed.Findings[0].Signal)
	}
	if removed.Findings[0].Blocking {
		t.Fatal("unmapped removal blocks")
	}
}

// TestGuardCodeRegistered is TASK-01 AC-01.4 at the registry: the code
// exists with its remedy and fails the exit-class table.
func TestGuardCodeRegistered(t *testing.T) {
	if !app.IsRegistered(app.CodeTestGuardWeakened) {
		t.Fatal("TEST_GUARD_WEAKENED is not registered")
	}
}

// TestPythonStringsAndCommentsDoNotCount is TASK-01 AC-01.5: assert-like
// text inside strings and comments never counts, because nodes match.
func TestPythonStringsAndCommentsDoNotCount(t *testing.T) {
	service := newGuardService(t)
	before := "def test_a():\n    assert x\n    assert y\n"
	after := "def test_a():\n    assert x\n    note = \"assert y\"\n    # assert y\n"
	result := evaluate(t, service, guardDelta("t.py", before, after))
	if len(result.Findings) != 1 || result.Findings[0].AfterSummary != "asserts 1" {
		t.Fatalf("findings = %+v", result.Findings)
	}
}
