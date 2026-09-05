package app

import (
	"go/ast"
	"go/parser"
	"go/token"
	"regexp"
	"sort"
	"strconv"
	"testing"
)

// screamingSnake is decision D-18's wire shape: unprefixed SCREAMING_SNAKE.
var screamingSnake = regexp.MustCompile(`^[A-Z][A-Z0-9]*(_[A-Z0-9]+)*$`)

// TestCodeRegistryIsUniqueAndExhaustive reads code.go itself, so a constant
// that is declared but never registered fails here rather than surfacing as an
// empty code in someone else's JSON.
func TestCodeRegistryIsUniqueAndExhaustive(t *testing.T) {
	declared := parseDeclaredCodes(t)
	if len(declared) == 0 {
		t.Fatalf("parsed no Code constants from code.go")
	}

	registered := RegisteredCodes()

	seen := make(map[Code]int, len(registered))
	for _, code := range registered {
		seen[code]++
	}
	for code, count := range seen {
		if count != 1 {
			t.Errorf("code %q is registered %d times, want exactly 1", code, count)
		}
	}

	for name, value := range declared {
		if seen[value] == 0 {
			t.Errorf("constant %s (%q) is declared but not returned by RegisteredCodes", name, value)
		}
		if !screamingSnake.MatchString(string(value)) {
			t.Errorf("constant %s has value %q, want unprefixed SCREAMING_SNAKE", name, value)
		}
		if !IsRegistered(value) {
			t.Errorf("IsRegistered(%q) = false, want true", value)
		}
	}

	if len(registered) != len(declared) {
		t.Errorf("RegisteredCodes has %d entries, code.go declares %d constants", len(registered), len(declared))
	}

	if !sort.SliceIsSorted(registered, func(i, j int) bool { return registered[i] < registered[j] }) {
		t.Errorf("RegisteredCodes is not sorted: %v", registered)
	}

	if IsRegistered(Code("DEFINITELY_NOT_A_CODE")) {
		t.Errorf("IsRegistered(unknown) = true, want false")
	}
	if IsRegistered(Code("")) {
		t.Errorf("IsRegistered(\"\") = true, want false")
	}
}

// TestRegisteredCodesIsNotAliased guards the registry against a caller that
// sorts or truncates the slice it was handed.
func TestRegisteredCodesIsNotAliased(t *testing.T) {
	first := RegisteredCodes()
	if len(first) == 0 {
		t.Fatalf("RegisteredCodes is empty")
	}
	original := first[0]
	first[0] = Code("MUTATED")

	if second := RegisteredCodes(); second[0] != original {
		t.Errorf("RegisteredCodes()[0] = %q after caller mutation, want %q", second[0], original)
	}
}

// parseDeclaredCodes returns every `X Code = "..."` constant in code.go.
func parseDeclaredCodes(t *testing.T) map[string]Code {
	t.Helper()

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "code.go", nil, 0)
	if err != nil {
		t.Fatalf("parse code.go: %v", err)
	}

	declared := make(map[string]Code)
	for _, decl := range file.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.CONST {
			continue
		}
		for _, spec := range genDecl.Specs {
			valueSpec, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			ident, ok := valueSpec.Type.(*ast.Ident)
			if !ok || ident.Name != "Code" {
				continue
			}
			for i, name := range valueSpec.Names {
				if i >= len(valueSpec.Values) {
					continue
				}
				lit, ok := valueSpec.Values[i].(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					t.Fatalf("constant %s is not a string literal", name.Name)
				}
				unquoted, err := strconv.Unquote(lit.Value)
				if err != nil {
					t.Fatalf("unquote %s: %v", lit.Value, err)
				}
				declared[name.Name] = Code(unquoted)
			}
		}
	}
	return declared
}
