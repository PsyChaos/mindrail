package testguard_test

import (
	"testing"

	"github.com/PsyChaos/mindrail/internal/testguard"
)

func ecmaDelta(path, before, after string) testguard.FileDelta {
	return testguard.FileDelta{Path: path, Language: testguard.LanguageTypeScript,
		Before: []byte(before), After: []byte(after)}
}

func TestTSXUsesTSXGrammarAndReportsFailingPath(t *testing.T) {
	service := newGuardService(t)
	path := "apps/web/app/(auth)/[org]/page.test.tsx"
	before := `test("renders", () => { render(<Panel title="ok" />); expect(screen.getByText("ok")).toBeVisible(); });`
	after := `test("renders", () => { render(<Panel title="ok" />); });`
	result, err := service.Evaluate(t.Context(), testguard.Request{Files: []testguard.FileDelta{{
		Path: path, Language: testguard.LanguageTSX, Before: []byte(before), After: []byte(after),
	}}, Trigger: testguard.TriggerStaged})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Findings) != 1 || result.Findings[0].Signal != testguard.SignalExpectationRemoved {
		t.Fatalf("findings = %+v", result.Findings)
	}
}

// TestEcmaExpectDecrease is TASK-02 AC-02.1: fewer expects after than
// before yields a count finding speaking expects on both sides.
func TestEcmaExpectDecrease(t *testing.T) {
	service := newGuardService(t)
	before := "it(\"auth\", () => { expect(login()).toBe(true); expect(session()).toBe(true); });\n"
	after := "it(\"auth\", () => { expect(login()).toBe(true); });\n"

	result, err := service.Evaluate(t.Context(), testguard.Request{
		Files:   []testguard.FileDelta{ecmaDelta("web/auth.test.ts", before, after)},
		Trigger: testguard.TriggerReconcile,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Findings) != 1 {
		t.Fatalf("findings = %+v", result.Findings)
	}
	finding := result.Findings[0]
	if finding.Signal != testguard.SignalAssertionCountDecreased {
		t.Fatalf("signal = %q", finding.Signal)
	}
	if finding.BeforeSummary != "expects 2" || finding.AfterSummary != "expects 1" {
		t.Fatalf("summaries = %q / %q", finding.BeforeSummary, finding.AfterSummary)
	}
	if finding.TestKey != "web/auth.test.ts::auth" {
		t.Fatalf("key = %q", finding.TestKey)
	}
	if finding.Trigger != testguard.TriggerReconcile {
		t.Fatalf("trigger = %q", finding.Trigger)
	}
}

// TestEcmaSkipVariants is TASK-02 AC-02.1: member skip forms detected,
// suite skip attributed to contained tests, near-cousins quiet.
func TestEcmaSkipVariants(t *testing.T) {
	service := newGuardService(t)
	body := "it(\"auth\", () => { expect(login()).toBe(true); });\n"
	for _, skipped := range []struct {
		body   string
		signal string
	}{
		{"it.skip(\"auth\", () => { expect(login()).toBe(true); });\n", testguard.SignalTestSkipped},
		{"test.skip(\"auth\", () => { expect(login()).toBe(true); });\n", testguard.SignalTestSkipped},
		{"describe.skip(\"auth suite\", () => { " + body + " });\n", testguard.SignalTestDisabled},
	} {
		result, err := service.Evaluate(t.Context(), testguard.Request{
			Files:   []testguard.FileDelta{ecmaDelta("w.test.ts", body, skipped.body)},
			Trigger: testguard.TriggerAfterChange,
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(result.Findings) != 1 || result.Findings[0].Signal != skipped.signal {
			t.Fatalf("skipped %q = %+v", skipped.body, result.Findings)
		}
	}
	for _, cousin := range []struct{ before, after string }{
		{
			"xit(\"auth\", () => { expect(login()).toBe(true); expect(x()).toBe(true); });\n",
			"xit(\"auth\", () => { expect(login()).toBe(true); });\n",
		},
		{
			"xdescribe(\"s\", () => { it(\"auth\", () => { expect(login()).toBe(true); }); });\n",
			"xdescribe(\"s\", () => { it(\"auth\", () => { }); });\n",
		},
		{
			"test.todo(\"auth\");\n",
			"test.todo(\"auth\");\n",
		},
		{
			"it.only(\"auth\", () => { expect(login()).toBe(true); expect(x()).toBe(true); });\n",
			"it.only(\"auth\", () => { expect(login()).toBe(true); });\n",
		},
	} {
		result, err := service.Evaluate(t.Context(), testguard.Request{
			Files:   []testguard.FileDelta{ecmaDelta("w.test.ts", cousin.before, cousin.after)},
			Trigger: testguard.TriggerAfterChange,
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(result.Findings) != 0 {
			t.Fatalf("cousin %q detected: %+v", cousin.before, result.Findings)
		}
	}
}

// TestEcmaExpectRemovedAndTestRemoved pins the zero remainder
// (EXPECTATION_REMOVED) and removed tests.
func TestEcmaExpectRemovedAndTestRemoved(t *testing.T) {
	service := newGuardService(t)
	gone, err := service.Evaluate(t.Context(), testguard.Request{
		Files: []testguard.FileDelta{ecmaDelta("w.test.ts",
			"it(\"auth\", () => { expect(login()).toBe(true); });\n",
			"it(\"auth\", () => { login(); });\n")},
		Trigger: testguard.TriggerStaged,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(gone.Findings) != 1 || gone.Findings[0].Signal != testguard.SignalExpectationRemoved {
		t.Fatalf("gone = %+v", gone.Findings)
	}
	removed, err := service.Evaluate(t.Context(), testguard.Request{
		Files: []testguard.FileDelta{ecmaDelta("w.test.ts",
			"it(\"auth\", () => { expect(login()).toBe(true); });\n",
			"it(\"other\", () => { expect(x()).toBe(true); });\n")},
		Trigger: testguard.TriggerCI,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(removed.Findings) != 1 || removed.Findings[0].AfterSummary != "removed" {
		t.Fatalf("removed = %+v", removed.Findings)
	}
}

// TestEcmaEscapedNamesMap is Breaker B-4's repro as a pin: escaped quotes
// unescape to the runtime name, so weakening is detected and the mapping
// with the true name is accepted.
func TestEcmaEscapedNamesMap(t *testing.T) {
	service := newGuardService(t)
	before := "it(\"a\\\"b\", () => { expect(x()).toBe(true); expect(y()).toBe(true); });\n"
	after := "it(\"a\\\"b\", () => { expect(x()).toBe(true); });\n"
	mapping := testguard.TestMapping{Path: "w.test.ts", Test: "a\"b",
		ProductionUID: "SYM-P", InvariantID: "INV-1", Severity: "CRITICAL", Active: true}

	result, err := service.Evaluate(t.Context(), testguard.Request{
		Files:    []testguard.FileDelta{ecmaDelta("w.test.ts", before, after)},
		Mappings: []testguard.TestMapping{mapping},
		Trigger:  testguard.TriggerCI,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Findings) != 1 || result.Findings[0].TestKey != "w.test.ts::a\"b" {
		t.Fatalf("findings = %+v", result.Findings)
	}
}

// TestEcmaDuplicateNamesAggregate is Breaker B-5's repro as a pin:
// same-name duplicates all execute, so asserts aggregate per name and no
// copy's weakening hides behind file ordering.
func TestEcmaDuplicateNamesAggregate(t *testing.T) {
	service := newGuardService(t)
	before := "it(\"a\", () => { expect(x()).toBe(true); expect(y()).toBe(true); });\n" +
		"it(\"a\", () => { expect(z()).toBe(true); });\n"
	weakened := "it(\"a\", () => { expect(x()).toBe(true); });\n" +
		"it(\"a\", () => { expect(z()).toBe(true); });\n"

	result, err := service.Evaluate(t.Context(), testguard.Request{
		Files:   []testguard.FileDelta{ecmaDelta("w.test.ts", before, weakened)},
		Trigger: testguard.TriggerAfterChange,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Findings) != 1 {
		t.Fatalf("masked duplicate = %+v", result.Findings)
	}
	if result.Findings[0].BeforeSummary != "expects 3" || result.Findings[0].AfterSummary != "expects 2" {
		t.Fatalf("finding = %+v", result.Findings[0])
	}
}

// TestEcmaAddedOnlyQuiet pins the added-quiet rule for ECMA: brand-new
// tests never report.
func TestEcmaAddedOnlyQuiet(t *testing.T) {
	service := newGuardService(t)
	result, err := service.Evaluate(t.Context(), testguard.Request{
		Files: []testguard.FileDelta{ecmaDelta("w.test.ts",
			"it(\"a\", () => { expect(x()).toBe(true); });\n",
			"it(\"a\", () => { expect(x()).toBe(true); });\nit(\"b\", () => { expect(y()).toBe(true); });\n")},
		Trigger: testguard.TriggerAfterChange,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Findings) != 0 || len(result.Suppressions) != 0 {
		t.Fatalf("added = %+v / %+v", result.Findings, result.Suppressions)
	}
}

// TestEcmaHatchSuppressesLoudly pins the shared hatch path for ECMA:
// marked weakening suppresses with listing, unmarked reports.
func TestEcmaHatchSuppressesLoudly(t *testing.T) {
	service := newGuardService(t)
	marked := "// mindrail: allow-test-weakening\nit(\"a\", () => { expect(x()).toBe(true); expect(y()).toBe(true); });\n"
	weakened := "// mindrail: allow-test-weakening\nit(\"a\", () => { expect(x()).toBe(true); });\n"
	result, err := service.Evaluate(t.Context(), testguard.Request{
		Files:   []testguard.FileDelta{ecmaDelta("w.test.ts", marked, weakened)},
		Trigger: testguard.TriggerAfterChange,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Findings) != 0 || len(result.Suppressions) != 1 {
		t.Fatalf("hatch = %+v / %+v", result.Findings, result.Suppressions)
	}
}

// TestEcmaUnmappedNeverBlocks pins the warn-by-default rule for ECMA:
// skip and decrease findings without mapping never block.
func TestEcmaUnmappedNeverBlocks(t *testing.T) {
	service := newGuardService(t)
	result, err := service.Evaluate(t.Context(), testguard.Request{
		Files: []testguard.FileDelta{ecmaDelta("w.test.ts",
			"it(\"a\", () => { expect(x()).toBe(true); expect(y()).toBe(true); });\n",
			"it.skip(\"a\", () => { expect(x()).toBe(true); });\n")},
		Trigger: testguard.TriggerAfterChange,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Findings) != 2 {
		t.Fatalf("findings = %+v", result.Findings)
	}
	for _, finding := range result.Findings {
		if finding.Blocking {
			t.Fatalf("unmapped blocks: %+v", finding)
		}
	}
}
