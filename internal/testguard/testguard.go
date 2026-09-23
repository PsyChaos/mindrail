// Package testguard detects test weakening structurally: assertion loss,
// skip/xfail/disable markers and removed verification tests (MR-012). One
// pure service answers every trigger path (after_change, reconcile,
// staged, ci) with identical analysis — only the provenance string differs.
// Findings block exactly where the spec demands hardness; everything else
// warns for MR-013's gate.
package testguard

import (
	"context"
	"strconv"
	"strings"

	"github.com/PsyChaos/mindrail/internal/app"
)

// Trigger paths. The value rides findings as provenance; analysis never
// branches on it.
const (
	TriggerAfterChange = "after_change"
	TriggerReconcile   = "reconcile"
	TriggerStaged      = "staged"
	TriggerCI          = "ci"
)

// Languages. tsx shares the typescript analyzer.
const (
	LanguagePython     = "python"
	LanguageTypeScript = "typescript"
	LanguageJavaScript = "javascript"
	LanguageTSX        = "tsx"
)

// Weakening signals (spec minimum heuristics).
const (
	SignalAssertionRemoved        = "ASSERTION_REMOVED"
	SignalAssertionCountDecreased = "ASSERTION_COUNT_DECREASED"
	SignalTestSkipped             = "TEST_SKIPPED"
	SignalTestXFailed             = "TEST_XFAILED"
	SignalTestDisabled            = "TEST_DISABLED"
	SignalExpectationRemoved      = "EXPECTATION_REMOVED"
	SignalAssertToNoop            = "ASSERT_TO_NOOP"
	SignalCriticalTestRemoved     = "CRITICAL_TEST_REMOVED"
)

// Severities. Only CRITICAL hardens to blocking, and only on removal or
// disable (spec policy ladder).
const (
	SeverityCritical = "CRITICAL"
)

// Confidence is the structural constant: tree-sitter nodes, never text
// search — the MR-005/MR-009 vocabulary, never a heuristic bump.
const Confidence = 0.5

// AllowMarker suppresses a test function's findings while listing the
// suppression in the result: self-disclosing, never silent (decision
// D-175).
const AllowMarker = "mindrail: allow-test-weakening"

// FileDelta is one test file's before/after bytes with an explicit
// language and trigger provenance.
type FileDelta struct {
	Path     string
	Language string
	Before   []byte
	After    []byte
}

// TestMapping binds one test to the invariant it verifies. Mapping arrives
// as caller input (decision D-172); test keys the mapping names but neither
// version defines are refused before analysis, loudly.
type TestMapping struct {
	Path          string
	Test          string
	ProductionUID string
	InvariantID   string
	Severity      string
	Active        bool
}

// Finding is one detected weakening with its explanation. Related
// production symbol and invariant stay empty when unmapped; Blocking
// follows the policy table, never the signal alone.
type Finding struct {
	Code          string
	Signal        string
	TestKey       string
	ProductionUID string
	InvariantID   string
	BeforeSummary string
	AfterSummary  string
	Reason        string
	Confidence    float64
	Blocking      bool
	Trigger       string
}

// Suppression is one marked test listed, never silently dropped.
type Suppression struct {
	TestKey string
	Reason  string
}

// Result is one evaluation: findings beside self-disclosed suppressions.
type Result struct {
	Findings     []Finding
	Suppressions []Suppression
}

// Request is one evaluation over file deltas with invariant mapping.
type Request struct {
	Files    []FileDelta
	Mappings []TestMapping
	Trigger  string
}

// testFunc is one test function in one version: name, assertion count,
// weakening markers, trivial body, start row for hatch association, the
// hatch flag itself (kept apart from markers so summaries never print it),
// the count noun the summary speaks ("asserts" or "expects"), and the
// signals a loss reads as (assertions vs expectations).
type testFunc struct {
	name       string
	asserts    int
	markers    []string
	allowed    bool
	trivial    bool
	startRow   uint
	unit       string
	decSignal  string
	zeroSignal string
}

// analyzer parses one language version into test functions.
type analyzer interface {
	tests(source []byte) (map[string]testFunc, error)
	close()
}

// Service evaluates test weakening. It owns compiled queries; Close it
// when done.
type Service struct {
	python     *pythonAnalyzer
	typescript *ecmaAnalyzer
	javascript *ecmaAnalyzer
}

// New builds a Service with compiled queries.
func New() (*Service, error) {
	python, err := newPythonAnalyzer()
	if err != nil {
		return nil, err
	}
	typescript, err := newTypescriptAnalyzer()
	if err != nil {
		python.close()
		return nil, err
	}
	javascript, err := newJavascriptAnalyzer()
	if err != nil {
		python.close()
		typescript.close()
		return nil, err
	}
	return &Service{python: python, typescript: typescript, javascript: javascript}, nil
}

// Close releases compiled queries and parsers.
func (s *Service) Close() {
	if s.python != nil {
		s.python.close()
	}
	if s.typescript != nil {
		s.typescript.close()
	}
	if s.javascript != nil {
		s.javascript.close()
	}
}

// Evaluate runs one shared analysis over every delta. Unknown languages,
// unknown mapping keys and empty paths refuse before any finding; parse is
// structural throughout.
func (s *Service) Evaluate(ctx context.Context, request Request) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	switch request.Trigger {
	case TriggerAfterChange, TriggerReconcile, TriggerStaged, TriggerCI:
	default:
		return Result{}, invalidInput("evaluation needs a known trigger")
	}
	var result Result
	for _, file := range request.Files {
		if file.Path == "" {
			return Result{}, invalidInput("evaluation needs a path for every file")
		}
		partial, err := s.evaluateFile(ctx, file, request.Mappings, request.Trigger)
		if err != nil {
			return Result{}, err
		}
		result.Findings = append(result.Findings, partial.Findings...)
		result.Suppressions = append(result.Suppressions, partial.Suppressions...)
	}
	return result, nil
}

func (s *Service) evaluateFile(ctx context.Context, file FileDelta, mappings []TestMapping, trigger string) (Result, error) {
	var result Result
	analyzer, err := s.analyzerFor(file.Language)
	if err != nil {
		return Result{}, err
	}
	before, err := analyzer.tests(file.Before)
	if err != nil {
		return Result{}, err
	}
	after, err := analyzer.tests(file.After)
	if err != nil {
		return Result{}, err
	}
	for _, mapping := range mappings {
		if mapping.Path != file.Path {
			continue
		}
		_, inBefore := before[mapping.Test]
		_, inAfter := after[mapping.Test]
		if !inBefore && !inAfter {
			return Result{}, invalidInput("mapping names an unknown test: " + mapping.Test + " in " + mapping.Path)
		}
	}
	for name, beforeFunc := range before {
		afterFunc, survived := after[name]
		key := file.Path + "::" + name
		mapped := mappingsFor(mappings, file.Path, name)
		if !survived {
			for _, mapping := range mapped {
				result.Findings = append(result.Findings, Finding{
					Code:          code(),
					Signal:        SignalCriticalTestRemoved,
					TestKey:       key,
					ProductionUID: mapping.ProductionUID,
					InvariantID:   mapping.InvariantID,
					BeforeSummary: summarize(beforeFunc),
					AfterSummary:  "removed",
					Reason:        "verification test removed",
					Confidence:    Confidence,
					Blocking:      blocks(mapping, SignalCriticalTestRemoved),
					Trigger:       trigger,
				})
			}
			if len(mapped) == 0 {
				result.Findings = append(result.Findings, Finding{
					Code:          code(),
					Signal:        SignalTestDisabled,
					TestKey:       key,
					BeforeSummary: summarize(beforeFunc),
					AfterSummary:  "removed",
					Reason:        "test function removed",
					Confidence:    Confidence,
					Trigger:       trigger,
				})
			}
			continue
		}
		var weakened []Finding
		for _, marker := range addedMarkers(beforeFunc.markers, afterFunc.markers) {
			signal := signalForMarker(marker)
			for _, finding := range mappedFindings(mapped, key, signal, summarize(beforeFunc), summarize(afterFunc), "marker "+marker+" added", trigger) {
				weakened = append(weakened, finding)
			}
			if len(mapped) == 0 {
				weakened = append(weakened, Finding{
					Code: code(), Signal: signal, TestKey: key,
					BeforeSummary: summarize(beforeFunc), AfterSummary: summarize(afterFunc),
					Reason: "marker " + marker + " added", Confidence: Confidence, Trigger: trigger,
				})
			}
		}
		if afterFunc.asserts < beforeFunc.asserts {
			signal := afterFunc.decSignal
			if signal == "" {
				signal = SignalAssertionCountDecreased
			}
			afterSummary := summarize(afterFunc)
			if afterFunc.asserts == 0 {
				signal = afterFunc.zeroSignal
				if signal == "" {
					signal = SignalAssertionRemoved
				}
				if afterFunc.trivial {
					signal = SignalAssertToNoop
				}
			}
			for _, finding := range mappedFindings(mapped, key, signal, summarize(beforeFunc), afterSummary, "assertions decreased", trigger) {
				weakened = append(weakened, finding)
			}
			if len(mapped) == 0 {
				weakened = append(weakened, Finding{
					Code: code(), Signal: signal, TestKey: key,
					BeforeSummary: summarize(beforeFunc), AfterSummary: afterSummary,
					Reason: "assertions decreased", Confidence: Confidence, Trigger: trigger,
				})
			}
		}
		if suppressed(afterFunc) {
			if len(weakened) > 0 {
				result.Suppressions = append(result.Suppressions, Suppression{TestKey: key, Reason: "marked allow-test-weakening"})
			}
		} else {
			result.Findings = append(result.Findings, weakened...)
		}
		if err := ctx.Err(); err != nil {
			return Result{}, err
		}
	}
	return result, nil
}

func (s *Service) analyzerFor(language string) (analyzer, error) {
	switch language {
	case LanguagePython:
		return s.python, nil
	case LanguageTypeScript, LanguageTSX:
		return s.typescript, nil
	case LanguageJavaScript:
		return s.javascript, nil
	default:
		return nil, invalidInput("evaluation needs a supported language, not " + language)
	}
}

func mappingsFor(mappings []TestMapping, path, name string) []TestMapping {
	var out []TestMapping
	for _, mapping := range mappings {
		if mapping.Path == path && mapping.Test == name {
			out = append(out, mapping)
		}
	}
	return out
}

func mappedFindings(mapped []TestMapping, key, signal, before, after, reason, trigger string) []Finding {
	var out []Finding
	for _, mapping := range mapped {
		out = append(out, Finding{
			Code:          code(),
			Signal:        signal,
			TestKey:       key,
			ProductionUID: mapping.ProductionUID,
			InvariantID:   mapping.InvariantID,
			BeforeSummary: before,
			AfterSummary:  after,
			Reason:        reason,
			Confidence:    Confidence,
			Blocking:      blocks(mapping, signal),
			Trigger:       trigger,
		})
	}
	return out
}

// blocks is the policy table: active CRITICAL plus removal or disable.
// Everything else warns for MR-013's gate.
func blocks(mapping TestMapping, signal string) bool {
	if !mapping.Active || mapping.Severity != SeverityCritical {
		return false
	}
	return signal == SignalCriticalTestRemoved || signal == SignalTestDisabled
}

func summarize(function testFunc) string {
	unit := function.unit
	if unit == "" {
		unit = "asserts"
	}
	summary := unit + " " + strconv.Itoa(function.asserts)
	if len(function.markers) > 0 {
		summary += " markers [" + strings.Join(function.markers, ",") + "]"
	}
	return summary
}

func addedMarkers(before, after []string) []string {
	held := map[string]bool{}
	for _, marker := range before {
		held[marker] = true
	}
	var added []string
	for _, marker := range after {
		if !held[marker] {
			added = append(added, marker)
		}
	}
	return added
}

func signalForMarker(marker string) string {
	switch marker {
	case "skip":
		return SignalTestSkipped
	case "xfail":
		return SignalTestXFailed
	default:
		return SignalTestDisabled
	}
}

func suppressed(function testFunc) bool {
	return function.allowed
}

func code() string {
	return string(app.CodeTestGuardWeakened)
}

func invalidInput(why string) error {
	return app.NewError(app.CodeCommandLineInvalid, app.KindUsage, why,
		"No test facts were changed.", "Pass valid test files, mapping and trigger.")
}
