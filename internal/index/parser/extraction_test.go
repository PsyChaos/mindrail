package parser

import (
	"context"
	"errors"
	"strings"
	"testing"
)

var packageFixtures = map[string]string{
	"python":     "import os\nimport sys as system\nfrom .lib import tool as renamed\ndef helper(value):\n    return value\nclass Box:\n    def run(self):\n        return helper(1)\n",
	"javascript": "import main, {tool as renamed} from './lib';\nimport * as ns from 'pkg';\nfunction helper(value) { return value; }\nclass Box { run() { return helper(1); } }\nconst arrow = (value) => helper(value);\nfunction* gen() { yield helper(1); }",
	"typescript": "import main, {tool as renamed} from './lib';\nimport * as ns from 'pkg';\nfunction helper(value: number): number { return value; }\nclass Box { run(): number { return helper(1); } }\nfunction overloaded(x: string): string;\nfunction overloaded(x: number): number;\nfunction overloaded(x: any): any { return x; }\n",
	"tsx":        "import main, {tool as renamed} from './lib';\nimport * as ns from 'pkg';\nfunction helper(value: number): number { return value; }\nclass Box { run() { return <div>{helper(1)}</div>; } }\n",
}

func extractForTest(t *testing.T, a SyntaxAdapter, source string) Facts {
	t.Helper()
	facts, err := Extract(t.Context(), a, SourceFile{Content: []byte(source)})
	if err != nil {
		t.Fatal(err)
	}
	return facts
}

func TestCompletePackageExtraction(t *testing.T) {
	r := registryForTest(t)
	for language, source := range packageFixtures {
		t.Run(language, func(t *testing.T) {
			facts := extractForTest(t, r.adaptersByName(language), source)
			byName := map[string][]Symbol{}
			for _, s := range facts.Symbols {
				byName[s.Name] = append(byName[s.Name], s)
				if s.LocalKey != LocalKey(s.ContainerLocalKey, s.Kind, s.Name) || len(s.SignatureHash) != 64 || len(s.BodyHash) != 64 || len(s.StructureHash) != 64 {
					t.Fatalf("incomplete symbol: %+v", s)
				}
			}
			if len(byName["helper"]) != 1 || len(byName["Box"]) != 1 || len(byName["run"]) != 1 {
				t.Fatalf("declarations: %+v", byName)
			}
			method := byName["run"][0]
			if method.Kind != "method" || method.ContainerLocalKey != byName["Box"][0].LocalKey {
				t.Fatalf("method scope: %+v", method)
			}
			if len(facts.Imports) != 3 {
				t.Fatalf("imports: %+v", facts.Imports)
			}
			foundAlias := false
			for _, imp := range facts.Imports {
				if imp.Alias == "renamed" && len(imp.Names) == 1 && imp.Names[0] == "tool" && imp.IsRelative {
					foundAlias = true
				}
			}
			if !foundAlias {
				t.Fatalf("aliased relative import: %+v", facts.Imports)
			}
			foundCall := false
			for _, ref := range facts.References {
				if ref.ReferrerLocalKey == method.LocalKey && ref.TargetLocalKey == byName["helper"][0].LocalKey {
					foundCall = true
				}
			}
			if !foundCall {
				t.Fatalf("same-file call missing: %+v", facts.References)
			}
			if language == "javascript" && (len(byName["arrow"]) != 1 || len(byName["gen"]) != 1) {
				t.Fatalf("arrow/generator omitted: %+v", byName)
			}
			if language == "typescript" {
				if len(byName["overloaded"]) != 3 {
					t.Fatalf("overloads collapsed: %+v", byName)
				}
				for _, s := range byName["overloaded"] {
					if s.LocalKey != byName["overloaded"][0].LocalKey {
						t.Fatal("overloads have different keys")
					}
				}
			}
		})
	}
}

func TestFingerprintSeparatesBodyAndSignature(t *testing.T) {
	r := registryForTest(t)
	for _, a := range r.adapters {
		t.Run(a.info.Language, func(t *testing.T) {
			source := "function f(x) { return x + 1; }"
			bodyChange := "function f(x) { if (x) { return x * 2; } return 0; }"
			sigChange := "function f(x, y) { return x + 1; }"
			if a.info.Language == "python" {
				source = "def f(x):\n    return x + 1\n"
				bodyChange = "def f(x):\n    if x:\n        return x * 2\n    return 0\n"
				sigChange = "def f(x, y):\n    return x + 1\n"
			}
			before := extractForTest(t, a, source).Symbols[0]
			body := extractForTest(t, a, bodyChange).Symbols[0]
			sig := extractForTest(t, a, sigChange).Symbols[0]
			renamed := extractForTest(t, a, strings.Replace(source, "(x)", "(z)", 1)).Symbols[0]
			if before.BodyHash == body.BodyHash || before.SignatureHash != body.SignatureHash || before.StructureHash != body.StructureHash {
				t.Fatalf("body contaminated signature/structure: %+v %+v", before, body)
			}
			if before.BodyHash != sig.BodyHash || before.SignatureHash == sig.SignatureHash || before.StructureHash == sig.StructureHash {
				t.Fatalf("signature hash isolation: %+v %+v", before, sig)
			}
			if before.StructureHash == renamed.StructureHash {
				t.Fatal("same-shaped signature rename did not change structure hash")
			}
		})
	}
}

func TestMethodBodyDoesNotChangeEnclosingClassStructure(t *testing.T) {
	r := registryForTest(t)
	for _, a := range r.adapters {
		source := "class Box { run() { return 1; } }"
		if a.info.Language == "python" {
			source = "class Box:\n    def run(self):\n        return 1\n"
		}
		before := extractForTest(t, a, source).Symbols
		after := extractForTest(t, a, strings.Replace(source, "return 1", "return 200", 1)).Symbols
		for i := range before {
			if before[i].SignatureHash != after[i].SignatureHash || before[i].StructureHash != after[i].StructureHash || before[i].BodyHash == after[i].BodyHash {
				t.Fatalf("%s nested body fingerprint: %+v %+v", a.info.Language, before, after)
			}
		}
	}
}

func TestPythonBodyLeadingCommentsStayOutOfSignature(t *testing.T) {
	a := registryForTest(t).adaptersByName("python")
	before := extractForTest(t, a, "def f():\n    # before\n    return 1\n").Symbols[0]
	after := extractForTest(t, a, "def f():\n    # after\n    # extra comment\n    return 1\n").Symbols[0]
	if before.SignatureHash != after.SignatureHash || before.StructureHash != after.StructureHash || before.BodyHash == after.BodyHash {
		t.Fatalf("body-leading comment affected wrong fingerprint: %+v %+v", before, after)
	}
}

func TestReferencesRespectShadowingAndUnknownReceivers(t *testing.T) {
	r := registryForTest(t)
	for _, a := range r.adapters {
		source := "function helper() {} function caller(helper) { helper(); obj.helper(); }"
		if a.info.Language == "python" {
			source = "def helper(): pass\ndef caller(helper):\n    helper()\n    obj.helper()\n"
		}
		facts := extractForTest(t, a, source)
		if len(facts.References) != 2 {
			t.Fatalf("calls: %+v", facts.References)
		}
		for _, ref := range facts.References {
			if ref.TargetLocalKey != "" {
				t.Fatalf("invented resolution: %+v", ref)
			}
		}
	}
}

func TestAnonymousAndLocalBindingsDoNotInventTargets(t *testing.T) {
	r := registryForTest(t)
	for _, a := range r.adapters {
		sources := []string{
			"function helper() {} function caller() { let helper = other; helper(); }",
			"function helper() {} function caller() { list.map(helper => helper()); }",
			"import helper from 'external'; function helper() {} helper();",
		}
		if a.info.Language == "python" {
			sources = []string{
				"def helper(): pass\ndef caller():\n    helper = other\n    helper()\n",
				"def helper(): pass\ndef caller():\n    consume(lambda helper: helper())\n",
				"from external import helper\ndef helper(): pass\nhelper()\n",
			}
		}
		for _, source := range sources {
			facts := extractForTest(t, a, source)
			for _, ref := range facts.References {
				if ref.Name == "helper" && ref.TargetLocalKey != "" {
					t.Fatalf("%s shadowed helper resolved: %+v", a.info.Language, ref)
				}
			}
		}
	}
}

func TestUnknownReceiverDoesNotResolveSameNamedFunction(t *testing.T) {
	r := registryForTest(t)
	for _, a := range r.adapters {
		source := "function helper() {} obj.helper();"
		if a.info.Language == "python" {
			source = "def helper(): pass\nobj.helper()\n"
		}
		facts := extractForTest(t, a, source)
		if len(facts.References) != 1 || facts.References[0].TargetLocalKey != "" {
			t.Fatalf("%s invented receiver target: %+v", a.info.Language, facts.References)
		}
	}
}

func TestBareMethodCallUsesLexicalScopeAndAmbiguityStaysUnresolved(t *testing.T) {
	r := registryForTest(t)
	for _, a := range r.adapters {
		source := "function helper() {} class Box { helper() {} run() { helper(); } }"
		ambiguous := "function helper() {} class helper {} helper();"
		if a.info.Language == "python" {
			source = "def helper(): pass\nclass Box:\n    def helper(self): pass\n    def run(self): helper()\n"
			ambiguous = "def helper(): pass\nclass helper: pass\nhelper()\n"
		}
		facts := extractForTest(t, a, source)
		if len(facts.References) != 1 || facts.References[0].TargetLocalKey != LocalKey("", "function", "helper") {
			t.Fatalf("%s class member treated as lexical: %+v", a.info.Language, facts.References)
		}
		facts = extractForTest(t, a, ambiguous)
		if len(facts.References) != 1 || facts.References[0].TargetLocalKey != "" {
			t.Fatalf("%s ambiguous keys resolved: %+v", a.info.Language, facts.References)
		}
	}
}

type extractionFailureAdapter struct {
	SyntaxAdapter
	snapshot *SyntaxSnapshot
	cancel   func()
	failure  error
}

func (a *extractionFailureAdapter) Parse(ctx context.Context, src SourceFile) (*SyntaxSnapshot, error) {
	s, err := a.SyntaxAdapter.Parse(ctx, src)
	a.snapshot = s
	return s, err
}
func (a *extractionFailureAdapter) Symbols(s *SyntaxSnapshot) ([]Symbol, error) {
	if a.cancel != nil {
		a.cancel()
	}
	symbols, _ := a.SyntaxAdapter.Symbols(s)
	return symbols, a.failure
}

func TestExtractionFailureAndCancellationCloseSnapshots(t *testing.T) {
	a := registryForTest(t).adapters[0]
	wantErr := errors.New("extraction interrupted")
	wrapper := &extractionFailureAdapter{SyntaxAdapter: a, failure: wantErr}
	facts, err := Extract(t.Context(), wrapper, SourceFile{Content: []byte(fixtures["python"])})
	if !errors.Is(err, wantErr) || len(facts.Symbols) != 1 || wrapper.snapshot.tree != nil {
		t.Fatalf("error disposal: %+v %v", facts, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	wrapper = &extractionFailureAdapter{SyntaxAdapter: a, cancel: cancel}
	facts, err = Extract(ctx, wrapper, SourceFile{Content: []byte(fixtures["python"])})
	if !errors.Is(err, context.Canceled) || len(facts.Symbols) != 0 || len(facts.References) != 0 || wrapper.snapshot.tree != nil {
		t.Fatalf("canceled extraction completion: %+v %v", facts, err)
	}
}

func TestExtractPreservesPartialFactsAndCancellation(t *testing.T) {
	a := registryForTest(t).adapters[0]
	facts, err := Extract(t.Context(), a, SourceFile{Content: []byte("def intact(): pass\ndef broken(:\n")})
	if !errors.Is(err, ErrSyntax) || !facts.HasSyntaxErrors || len(facts.Symbols) == 0 || facts.Symbols[0].Name != "intact" {
		t.Fatalf("partial facts: %+v %v", facts, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	facts, err = Extract(ctx, a, SourceFile{Content: []byte(fixtures["python"])})
	if !errors.Is(err, context.Canceled) || len(facts.Symbols) != 0 {
		t.Fatalf("canceled completion: %+v %v", facts, err)
	}
}
