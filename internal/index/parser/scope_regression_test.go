package parser

import (
	"strings"
	"testing"
)

func TestAnonymousShadowDoesNotHideSiblingCall(t *testing.T) {
	r := registryForTest(t)
	for _, a := range r.adapters {
		t.Run(a.info.Language, func(t *testing.T) {
			source := "function helper() {} function caller() { [1].map(helper => helper()); helper(); }"
			if a.info.Language == "python" {
				source = "def helper(): pass\ndef caller():\n    consume(lambda helper: helper())\n    helper()\n"
			}
			facts := extractForTest(t, a, source)
			var calls []Reference
			for _, ref := range facts.References {
				if ref.Name == "helper" {
					calls = append(calls, ref)
				}
			}
			if len(calls) != 2 || calls[0].TargetLocalKey != "" || calls[1].TargetLocalKey != LocalKey("", "function", "helper") {
				t.Fatalf("callback/sibling scopes: %+v", calls)
			}
		})
	}
}

func TestDestructuringBindingsShadowOnlyTheirLexicalScope(t *testing.T) {
	r := registryForTest(t)
	for _, language := range []string{"javascript", "typescript", "tsx"} {
		t.Run(language, func(t *testing.T) {
			a := r.adaptersByName(language)
			for _, source := range []string{
				"function helper() {} function caller({helper}) { helper(); }",
				"function helper() {} function caller() { const {helper} = value; helper(); }",
				"function helper() {} function caller() { { const {helper} = value; helper(); } helper(); }",
			} {
				facts := extractForTest(t, a, source)
				var calls []Reference
				for _, ref := range facts.References {
					if ref.Name == "helper" {
						calls = append(calls, ref)
					}
				}
				if len(calls) == 0 || calls[0].TargetLocalKey != "" {
					t.Fatalf("destructured helper resolved: %+v", calls)
				}
				if len(calls) == 2 && calls[1].TargetLocalKey != LocalKey("", "function", "helper") {
					t.Fatalf("block escaped into sibling: %+v", calls)
				}
			}
		})
	}
}

func TestPythonMatchCaptureShadowsFunction(t *testing.T) {
	a := registryForTest(t).adaptersByName("python")
	source := "def helper(): pass\ndef caller(value):\n    match value:\n        case helper:\n            helper()\n    helper()\n"
	facts := extractForTest(t, a, source)
	for _, ref := range facts.References {
		if ref.Name == "helper" && ref.TargetLocalKey != "" {
			t.Fatalf("match capture resolved as module function: %+v", ref)
		}
	}
}

func TestBindingPatternReadsDoNotShadowTargets(t *testing.T) {
	r := registryForTest(t)
	for _, a := range r.adapters {
		t.Run(a.info.Language, func(t *testing.T) {
			source := "function helper() {} function caller({value = helper}) { helper(); }"
			if a.info.Language == "python" {
				source = "def helper(): pass\ndef caller(value=helper):\n    helper()\n    match value:\n        case helper.VALUE:\n            helper()\n"
			}
			facts := extractForTest(t, a, source)
			for _, ref := range facts.References {
				if ref.Name == "helper" && ref.TargetLocalKey != LocalKey("", "function", "helper") {
					t.Fatalf("pattern read treated as binding: %+v", ref)
				}
			}
		})
	}
}

func TestBlockFunctionDoesNotResolveOutsideBlock(t *testing.T) {
	r := registryForTest(t)
	for _, language := range []string{"javascript", "typescript", "tsx"} {
		t.Run(language, func(t *testing.T) {
			facts := extractForTest(t, r.adaptersByName(language), "function caller() { { function helper() {} helper(); } helper(); }")
			if len(facts.References) != 2 || facts.References[0].TargetLocalKey == "" || facts.References[1].TargetLocalKey != "" {
				t.Fatalf("block function leaked: %+v", facts.References)
			}
		})
	}
}

func TestLocalKeysRemainBoundedAndUnambiguousAtDepth(t *testing.T) {
	container := ""
	for depth := range 100 {
		container = LocalKey(container, "function", "nested")
		if len(container) > 64 {
			t.Fatalf("depth %d key grew to %d bytes", depth+1, len(container))
		}
	}
	if LocalKey("", "function", "f") != LocalKey("", "function", "f") {
		t.Fatal("overload identity unstable")
	}
	if LocalKey("a", "function", "b") == LocalKey("", "function", "a/b") || LocalKey("", "class", "f") == LocalKey("", "function", "f") {
		t.Fatal("distinct identities collided")
	}
	var source strings.Builder
	for depth := range 20 {
		source.WriteString(strings.Repeat("    ", depth) + "def nested():\n")
	}
	source.WriteString(strings.Repeat("    ", 20) + "pass\n")
	facts := extractForTest(t, registryForTest(t).adaptersByName("python"), source.String())
	if len(facts.Symbols) != 20 {
		t.Fatalf("nested declarations: %d", len(facts.Symbols))
	}
	for _, symbol := range facts.Symbols {
		if len(symbol.LocalKey) > 64 || len(symbol.ContainerLocalKey) > 64 {
			t.Fatalf("unbounded extracted identity: %d", len(symbol.LocalKey))
		}
	}
}

func TestPythonDeclarationHeadersUseEnclosingScope(t *testing.T) {
	a := registryForTest(t).adaptersByName("python")
	for name, source := range map[string]string{
		"default":             "def helper(): pass\ndef run(x=helper()):\n    def helper(): pass\n    helper()\n",
		"annotation":          "def helper(): pass\ndef run(x: helper()):\n    def helper(): pass\n    helper()\n",
		"return_annotation":   "def helper(): pass\ndef run() -> helper():\n    def helper(): pass\n    helper()\n",
		"decorator":           "def helper(): pass\n@helper()\ndef run():\n    def helper(): pass\n    helper()\n",
		"parameter_same_name": "def helper(): pass\ndef run(helper=helper()):\n    helper()\n",
	} {
		t.Run(name, func(t *testing.T) {
			facts := extractForTest(t, a, source)
			if len(facts.References) != 2 || facts.References[0].TargetLocalKey != LocalKey("", "function", "helper") || facts.References[0].ScopeLocalKey != "" {
				t.Fatalf("header resolved inside declaration: %+v", facts.References)
			}
			want := LocalKey(LocalKey("", "function", "run"), "function", "helper")
			if name == "parameter_same_name" {
				want = ""
			}
			if facts.References[1].TargetLocalKey != want {
				t.Fatalf("body lost local binding: %+v", facts.References)
			}
		})
	}
}

func TestPythonNestedDefaultKeepsOuterFunctionScope(t *testing.T) {
	a := registryForTest(t).adaptersByName("python")
	facts := extractForTest(t, a, "def helper(): pass\ndef outer():\n    def helper(): pass\n    def run(x=helper()):\n        def helper(): pass\n        helper()\n")
	outer := LocalKey("", "function", "outer")
	run := LocalKey(outer, "function", "run")
	if len(facts.References) != 2 || facts.References[0].TargetLocalKey != LocalKey(outer, "function", "helper") || facts.References[0].ScopeLocalKey != outer || facts.References[0].ReferrerLocalKey != run {
		t.Fatalf("nested default lost enclosing evaluation scope: %+v", facts.References)
	}
	if facts.References[1].TargetLocalKey != LocalKey(run, "function", "helper") || facts.References[1].ScopeLocalKey != run {
		t.Fatalf("nested body lost its own scope: %+v", facts.References)
	}
}

func TestPythonLambdaDefaultUsesEnclosingScope(t *testing.T) {
	a := registryForTest(t).adaptersByName("python")
	facts := extractForTest(t, a, "def helper(): pass\nconsume(lambda helper=helper(): helper())\nhelper()\n")
	var calls []Reference
	for _, ref := range facts.References {
		if ref.Name == "helper" {
			calls = append(calls, ref)
		}
	}
	want := LocalKey("", "function", "helper")
	if len(calls) != 3 || calls[0].TargetLocalKey != want || calls[1].TargetLocalKey != "" || calls[2].TargetLocalKey != want {
		t.Fatalf("lambda default/body/sibling scopes: %+v", calls)
	}
}

func TestAnonymousGeneratorParameterDoesNotEscapeItsBody(t *testing.T) {
	r := registryForTest(t)
	for _, language := range []string{"javascript", "typescript", "tsx"} {
		t.Run(language, func(t *testing.T) {
			facts := extractForTest(t, r.adaptersByName(language), "function helper(){} consume(function* (helper){ helper(); }); helper();")
			var calls []Reference
			for _, ref := range facts.References {
				if ref.Name == "helper" {
					calls = append(calls, ref)
				}
			}
			if len(calls) != 2 || calls[0].TargetLocalKey != "" || calls[1].TargetLocalKey != LocalKey("", "function", "helper") {
				t.Fatalf("generator parameter scope: %+v", calls)
			}
		})
	}
}
