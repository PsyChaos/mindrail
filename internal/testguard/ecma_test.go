package testguard_test

import (
	"testing"

	"github.com/PsyChaos/mindrail/internal/testguard"
)

func ecmaDelta(path, before, after string) testguard.FileDelta {
	return testguard.FileDelta{Path: path, Language: testguard.LanguageTypeScript,
		Before: []byte(before), After: []byte(after)}
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
