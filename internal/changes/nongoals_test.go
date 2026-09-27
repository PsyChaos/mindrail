package changes_test

import (
	goparser "go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/PsyChaos/mindrail/internal/app"
)

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

// TestNoLaterMilestoneMachinery scans import paths the way the task list
// asks: impact traversal, evidence binding, coverage mapping, semantic
// resolvers must not exist as code — comments may name them, imports may not.
// Test files are excluded: fixtures may reference anything, production may
// not. MCP transport was on this list until MR-014 built it deliberately.
// The loopback-only read dashboard and client-neutral JEV browser connector
// are the two deliberate net/http islands. Their dependencies stay isolated
// from bootstrap/internal/cli and are covered by dedicated security,
// architecture and contract tests.
func TestNoLaterMilestoneMachinery(t *testing.T) {
	root := moduleRoot(t)
	// knowledge/cli deliberately absent: bootstrap, doctor and the root
	// command legitimately import loader/schema/validate, so a tree-wide ban
	// would forbid the existing architecture. The discovery path's own
	// restraint is structural — internal/changes imports neither — and the
	// command surface is pinned separately.
	banned := []string{
		"impact", "evidence", "coverage", "semantic",
	}
	var violations []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			switch entry.Name() {
			case ".git", "graphify-out", "testdata", ".wrongstack", ".zcode", "bin":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
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
				if ban == "net/http" && strings.Contains(filepath.ToSlash(path), "/internal/jevconnect/") {
					continue
				}
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
		t.Errorf("later-milestone import: %s", violation)
	}
}

// TestNoUnplannedCodesIn02 pins the 0.2 code vocabulary. In addition to the
// 0.1 vocabulary, the client-neutral JEV workflow owns seven explicit
// unavailable/error identities so automation never has to branch on prose.
func TestNoUnplannedCodesIn02(t *testing.T) {
	if got := len(app.RegisteredCodes()); got != 53 {
		t.Fatalf("registered codes = %d, want 53 (the 0.1 vocabulary plus seven JEV connection codes)", got)
	}
}
