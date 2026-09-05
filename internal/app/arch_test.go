package app

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// TestAppPackageIsALeaf enforces the §3 dependency direction: app is the sink
// every other package may depend on, so it may depend on none of them. The
// check parses this package's own sources rather than shelling out to
// `go list`, so it stays honest while the rest of the tree is mid-flight.
func TestAppPackageIsALeaf(t *testing.T) {
	module := modulePath(t)
	files := packageFiles(t, true)

	for _, file := range files {
		for _, path := range importsOf(t, file) {
			if path == module || strings.HasPrefix(path, module+"/") {
				t.Errorf("%s imports %q; internal/app must import nothing from the module", filepath.Base(file), path)
			}
			if strings.Contains(path, "/internal/") {
				t.Errorf("%s imports %q; internal/app must import no internal package", filepath.Base(file), path)
			}
		}
	}
}

// TestAppUsesOnlyPermittedStdlib keeps the leaf small: a process-level package
// that reached for net/http or os/exec would drag the whole binary's blast
// radius down into the dependency sink.
func TestAppUsesOnlyPermittedStdlib(t *testing.T) {
	forbidden := map[string]struct{}{
		"net":          {},
		"net/http":     {},
		"os/exec":      {},
		"database/sql": {},
		"reflect":      {},
	}

	for _, file := range packageFiles(t, false) {
		for _, path := range importsOf(t, file) {
			if _, bad := forbidden[path]; bad {
				t.Errorf("%s imports %q, which does not belong in the app leaf", filepath.Base(file), path)
			}
		}
	}
}

// packageFiles lists this package's Go sources, optionally including tests.
func packageFiles(t *testing.T, includeTests bool) []string {
	t.Helper()

	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read package directory: %v", err)
	}

	var files []string
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") {
			continue
		}
		if !includeTests && strings.HasSuffix(name, "_test.go") {
			continue
		}
		files = append(files, name)
	}

	if len(files) == 0 {
		t.Fatalf("found no Go sources to inspect")
	}
	return files
}

func importsOf(t *testing.T, file string) []string {
	t.Helper()

	parsed, err := parser.ParseFile(token.NewFileSet(), file, nil, parser.ImportsOnly)
	if err != nil {
		t.Fatalf("parse %s: %v", file, err)
	}

	paths := make([]string, 0, len(parsed.Imports))
	for _, spec := range parsed.Imports {
		path, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			t.Fatalf("%s: unquote %s: %v", file, spec.Path.Value, err)
		}
		paths = append(paths, path)
	}
	return paths
}

// modulePath reads the module path from go.mod so the leaf assertion keeps
// working if the module is ever renamed.
func modulePath(t *testing.T) string {
	t.Helper()

	raw, err := os.ReadFile(filepath.Join("..", "..", "go.mod"))
	if err != nil {
		t.Fatalf("read go.mod: %v", err)
	}
	for _, line := range strings.Split(string(raw), "\n") {
		if rest, ok := strings.CutPrefix(strings.TrimSpace(line), "module "); ok {
			return strings.TrimSpace(rest)
		}
	}

	t.Fatalf("go.mod has no module directive")
	return ""
}
