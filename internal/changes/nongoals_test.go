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
// resolvers, MCP transport and HTTP remoting must not exist as code —
// comments may name them, imports may not. Test files are excluded: fixtures
// may reference anything, production may not.
func TestNoLaterMilestoneMachinery(t *testing.T) {
	root := moduleRoot(t)
	// knowledge/cli deliberately absent: bootstrap, doctor and the root
	// command legitimately import loader/schema/validate, so a tree-wide ban
	// would forbid the existing architecture. The discovery path's own
	// restraint is structural — internal/changes imports neither — and the
	// command surface is pinned separately.
	banned := []string{
		"impact", "evidence", "coverage", "semantic",
		"/mcp", "mcp/", "net/http",
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

// TestNoNewCodesIn01 pins the 0.1 code vocabulary: MR-008 added the three
// attribution codes, MR-012 the guard code, MR-013 the evidence code;
// further enforcement codes arrive later, never here.
func TestNoNewCodesIn01(t *testing.T) {
	if got := len(app.RegisteredCodes()); got != 45 {
		t.Fatalf("registered codes = %d, want 45 (three attribution codes in MR-008, one guard code in MR-012, one evidence code in MR-013)", got)
	}
}
