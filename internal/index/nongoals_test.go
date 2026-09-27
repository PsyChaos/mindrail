package index_test

import (
	goparser "go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/index"
	"github.com/PsyChaos/mindrail/internal/index/parser"
)

// moduleRoot finds the repository root from this file's position, so the walk
// below covers the whole tree however the test is invoked.
func moduleRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime caller unavailable")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("module root %s has no go.mod: %v", root, err)
	}
	return root
}

// TestNonGoalsHold scans the tree the way the task list asks: under grep.
// FULL semantic resolvers, language-server integrations, runtime coverage
// providers, vector/graph stores and languages beyond Python/TypeScript/JS
// must not exist as code — comments and documentation may name them, imports
// may not.
func TestNonGoalsHold(t *testing.T) {
	root := moduleRoot(t)
	for _, dir := range []string{"internal/semantic"} {
		if _, err := os.Stat(filepath.Join(root, dir)); !os.IsNotExist(err) {
			t.Errorf("%s exists; 0.1 builds no semantic tree", dir)
		}
	}
	banned := []string{
		"pyright", "tsserver", "typescript-language-server", "gopls",
		"tree-sitter-java/", "tree-sitter-go/", "tree-sitter-c-sharp/", "tree-sitter-php/",
		"pgvector", "neo4j", "onnx", "/embeddings",
	}
	var violations []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			switch entry.Name() {
			case ".git", "graphify-out", "testdata", ".wrongstack", ".zcode":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		fset := token.NewFileSet()
		f, err := goparser.ParseFile(fset, path, nil, goparser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, spec := range f.Imports {
			importPath := strings.Trim(spec.Path.Value, `"`)
			for _, ban := range banned {
				if strings.Contains(importPath, ban) {
					violations = append(violations, path+": "+importPath)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, violation := range violations {
		t.Errorf("non-goal import: %s", violation)
	}
}

// TestKnowledgeSchemaStaysV1 pins the frozen knowledge contract: no v2
// schema file may appear, and every shipped schema still writes version 1.
func TestKnowledgeSchemaStaysV1(t *testing.T) {
	schemas := filepath.Join(moduleRoot(t), "schemas", "knowledge")
	entries, err := os.ReadDir(schemas)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) == 0 {
		t.Fatal("no knowledge schemas")
	}
	versionPattern := regexp.MustCompile(`\.v([0-9]+)\.`)
	for _, entry := range entries {
		if match := versionPattern.FindStringSubmatch(entry.Name()); match != nil && match[1] != "1" {
			t.Errorf("schema %s: version %s is out of 0.1 scope", entry.Name(), match[1])
		}
		body, err := os.ReadFile(filepath.Join(schemas, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(body), `"const": 1`) {
			t.Errorf("schema %s no longer pins schema_version 1", entry.Name())
		}
	}
}

// TestRegistryShipsFourLanguagesOnly pins the 0.1 language surface: Python,
// JavaScript, TypeScript and TSX. A fourth grammar module is unsupported, not
// a roadmap item.
func TestRegistryShipsFourLanguagesOnly(t *testing.T) {
	registry, err := parser.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	defer registry.Close()
	want := map[string]bool{"python": true, "javascript": true, "typescript": true, "tsx": true}
	entries := registry.Entries()
	if len(entries) != len(want) {
		t.Fatalf("registry has %d languages, want 4", len(entries))
	}
	for _, entry := range entries {
		if !want[entry.Language] {
			t.Errorf("registry ships non-goal language %q", entry.Language)
		}
	}
}

// TestIndexCodesCarryDistinctRemedies pins TASK-01 AC-01.3: the three codes
// enter allCodes with a remedy each, and no two share one.
func TestIndexCodesCarryDistinctRemedies(t *testing.T) {
	errs := []error{
		index.UnsupportedLanguage("a.md"),
		index.ParseFailed("a.py", os.ErrInvalid),
		index.IndexStateCorrupt(os.ErrInvalid),
	}
	seenCodes := map[app.Code]bool{}
	seenRemedies := map[string]app.Code{}
	for _, err := range errs {
		payload, ok := app.PayloadOf(err)
		if !ok {
			t.Fatalf("error %v carries no payload", err)
		}
		if seenCodes[payload.Code] {
			t.Fatalf("duplicate code %q", payload.Code)
		}
		seenCodes[payload.Code] = true
		if len(payload.NextAction) == 0 {
			t.Fatalf("code %q carries no remedy", payload.Code)
		}
		for _, remedy := range payload.NextAction {
			if owner, ok := seenRemedies[remedy]; ok {
				t.Fatalf("remedy %q shared by %q and %q", remedy, owner, payload.Code)
			}
			seenRemedies[remedy] = payload.Code
		}
	}
}
