package cli_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/PsyChaos/mindrail/internal/moduletree"
)

// The dependency rules of design §3, spelled as data.
var (
	// ormModules and diModules are what tech-stack §9 and §20 rule out. Both are
	// matched by import-path prefix so a versioned suffix still trips.
	ormModules = []string{
		"gorm.io/",
		"github.com/jinzhu/gorm",
		"entgo.io/ent",
		"github.com/volatiletech/sqlboiler",
		"github.com/uptrace/bun",
		"xorm.io/",
		"github.com/jmoiron/sqlx",
		"github.com/go-gorp/gorp",
	}
	diModules = []string{
		"github.com/google/wire",
		"go.uber.org/fx",
		"go.uber.org/dig",
		"github.com/samber/do",
	}
	gitLibraries = []string{
		"go-git",
		"libgit2",
		"git2go",
	}
)

// upwardRules are the §3 edges that must not exist. The reporting layers know
// about the adapters; an adapter that learned about doctor or status would
// close the loop and make the graph cyclic in meaning even where the compiler
// still accepts it.
var upwardRules = map[string][]string{
	"internal/git":        {"internal/doctor", "internal/status", "internal/bootstrap", "internal/cli"},
	"internal/storage":    {"internal/doctor", "internal/status", "internal/bootstrap", "internal/cli"},
	"internal/filesystem": {"internal/doctor", "internal/status", "internal/bootstrap", "internal/cli"},
}

// TestMainImportsAreMinimal keeps package main a launcher.
//
// tech-stack §85 puts the root context, bootstrap, CLI execution and the exit
// code in main and nothing else. The import set is the enforceable half of that:
// a main that reached for internal/storage would be doing domain work, whatever
// its function bodies looked like.
func TestMainImportsAreMinimal(t *testing.T) {
	module, root := moduleInfo(t)

	allowed := map[string]struct{}{
		module + "/internal/app": {},
		module + "/internal/cli": {},
	}

	mainDir := filepath.Join(root, "cmd", "mindrail")
	files := goSources(t, mainDir, false)
	if len(files) == 0 {
		t.Fatal("cmd/mindrail has no non-test Go sources")
	}

	for _, file := range files {
		for _, imported := range fileImports(t, file) {
			if isStandardLibrary(imported) {
				continue
			}
			if _, ok := allowed[imported]; !ok {
				t.Errorf("%s imports %q; package main may import only the standard library, internal/app and internal/cli",
					filepath.Base(file), imported)
			}
		}
	}

	assertMainDeclaresOnlyMain(t, files)
}

// assertMainDeclaresOnlyMain fails if package main grew anything other than the
// entry point. A helper here is a helper nothing can test in isolation.
func assertMainDeclaresOnlyMain(t *testing.T, files []string) {
	t.Helper()

	for _, file := range files {
		parsed, err := parser.ParseFile(token.NewFileSet(), file, nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatalf("parse %s: %v", file, err)
		}

		for _, decl := range parsed.Decls {
			switch typed := decl.(type) {
			case *ast.GenDecl:
				if typed.Tok == token.IMPORT {
					continue
				}
				t.Errorf("%s declares a package-level %s; package main declares only main()",
					filepath.Base(file), typed.Tok)
			case *ast.FuncDecl:
				if typed.Recv != nil || typed.Name.Name != "main" {
					t.Errorf("%s declares %s; package main declares only main()",
						filepath.Base(file), typed.Name.Name)
				}
			}
		}
	}
}

// TestForbiddenDependencies enforces decision D-34 and tech-stack §20 at the
// module boundary, which is where the decision is actually made: a dependency
// that is required but not yet imported is the state just before someone starts
// using it.
func TestForbiddenDependencies(t *testing.T) {
	_, root := moduleInfo(t)

	forbidden := slices.Concat(ormModules, diModules, gitLibraries)

	t.Run("go.mod", func(t *testing.T) {
		data, err := os.ReadFile(filepath.Join(root, "go.mod"))
		if err != nil {
			t.Fatalf("read go.mod: %v", err)
		}
		for _, module := range forbidden {
			if strings.Contains(string(data), module) {
				t.Errorf("go.mod requires %q; MR-001 uses database/sql, the system git and explicit constructor wiring", module)
			}
		}
	})

	t.Run("sources", func(t *testing.T) {
		for importPath, pkg := range firstPartyPackages(t, root) {
			for _, imported := range pkg.imports {
				if match := matchForbidden(imported, forbidden); match != "" {
					t.Errorf("%s imports %q, which matches the forbidden module %q", importPath, imported, match)
				}
			}
		}
	})

	// Positive control. Every forbidden module is absent from this repository,
	// so without this the two subtests above would pass identically if the
	// matcher were broken.
	t.Run("matcher", func(t *testing.T) {
		matches := map[string]bool{
			"gorm.io/gorm":                    true,
			"gorm.io/gorm/v2":                 true,
			"github.com/go-git/go-git/v5":     true,
			"go.uber.org/fx":                  true,
			"github.com/google/wire":          true,
			"database/sql":                    false,
			"github.com/spf13/cobra":          false,
			"modernc.org/sqlite":              false,
			"github.com/pelletier/go-toml/v2": false,
		}
		for importPath, want := range matches {
			if got := matchForbidden(importPath, forbidden) != ""; got != want {
				t.Errorf("matchForbidden(%q) = %v, want %v", importPath, got, want)
			}
		}
	})
}

// matchForbidden returns the forbidden module importPath belongs to, or "".
// The comparison is a prefix on the module path and a substring for the bare
// library names, because "go-git" arrives as github.com/go-git/go-git/v5 and as
// gopkg.in/src-d/go-git.v4.
func matchForbidden(importPath string, forbidden []string) string {
	for _, module := range forbidden {
		if strings.HasPrefix(importPath, module) || strings.Contains(importPath, module) {
			return module
		}
	}
	return ""
}

// TestInitPathDoesNotImportNetHTTP is acceptance criterion 1 turned into a
// compile-time fact: `mindrail init` cannot depend on the network because the
// code that runs it cannot reach a network package.
//
// The first-party check parses production sources only. Test files are excluded
// on purpose — internal/bootstrap's own test installs a refusing HTTP transport
// to prove that nothing dials, and that proof needs net/http.
func TestInitPathDoesNotImportNetHTTP(t *testing.T) {
	module, root := moduleInfo(t)
	packages := firstPartyPackages(t, root)

	networkPackages := []string{"net/http", "net/rpc", "net/smtp"}

	for _, entry := range []string{"internal/bootstrap", "internal/cli"} {
		reached := reachableFrom(t, packages, module+"/"+entry)

		for _, forbidden := range networkPackages {
			if via, ok := reached[forbidden]; ok {
				t.Errorf("%s reaches %q via %s", entry, forbidden, via)
			}
		}
	}

	assertNoNetworkInLinkedPackages(t, root, networkPackages)
}

// assertNoNetworkInLinkedPackages widens the claim past first-party code.
// Parsing the repository proves nothing about what cobra or the SQLite driver
// drag in, and those are linked into the same binary.
func assertNoNetworkInLinkedPackages(t *testing.T, root string, networkPackages []string) {
	t.Helper()

	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("the go tool is not on PATH, so the linked dependency set cannot be listed")
	}

	cmd := exec.Command("go", "list", "-deps", "./internal/bootstrap")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		t.Skipf("go list -deps failed (%v); the first-party check above still applies", err)
	}

	deps := make(map[string]struct{})
	for line := range strings.SplitSeq(string(out), "\n") {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			deps[trimmed] = struct{}{}
		}
	}
	if len(deps) == 0 {
		t.Fatal("go list -deps returned nothing; the assertion would be vacuous")
	}

	for _, forbidden := range networkPackages {
		if _, ok := deps[forbidden]; ok {
			t.Errorf("the linked dependency set of internal/bootstrap contains %q", forbidden)
		}
	}
}

// TestNoUpwardImports keeps the §3 graph pointing one way. The adapters are the
// sinks; a sink that imported a reporting layer would make the two impossible
// to reason about separately and impossible to test separately.
func TestNoUpwardImports(t *testing.T) {
	module, root := moduleInfo(t)
	packages := firstPartyPackages(t, root)

	for from, forbidden := range upwardRules {
		fromPath := module + "/" + from
		if _, ok := packages[fromPath]; !ok {
			t.Fatalf("%s is not a package in this module; the rule table is stale", from)
		}

		reached := reachableFrom(t, packages, fromPath)
		for _, to := range forbidden {
			toPath := module + "/" + to
			if via, ok := reached[toPath]; ok {
				t.Errorf("%s reaches %s via %s; §3 points that edge the other way", from, to, via)
			}
		}
	}

	// Positive control: the edges §3 does require have to be visible to the same
	// traversal, or the absence of the forbidden ones proves nothing.
	required := map[string][]string{
		"internal/cli":        {"internal/bootstrap", "internal/status", "internal/doctor", "internal/app"},
		"internal/bootstrap":  {"internal/git", "internal/storage", "internal/filesystem", "internal/migration"},
		"internal/doctor":     {"internal/git", "internal/filesystem", "internal/app"},
		"internal/git":        {"internal/app"},
		"internal/filesystem": {"internal/app"},
	}
	for from, targets := range required {
		reached := reachableFrom(t, packages, module+"/"+from)
		for _, to := range targets {
			if _, ok := reached[module+"/"+to]; !ok {
				t.Errorf("%s does not reach %s; the traversal is not following the graph", from, to)
			}
		}
	}
}

// --- import graph -----------------------------------------------------------

// firstPartyPackage is one directory's production import set.
type firstPartyPackage struct {
	imports []string
}

// firstPartyPackages parses every non-test source in the module into an import
// graph keyed by import path.
//
// Parsing is preferred to `go list` for the first-party graph because it works
// while the module is mid-flight and because build tags cannot hide an edge from
// it: a file excluded on this platform still declares its imports.
func firstPartyPackages(t *testing.T, root string) map[string]firstPartyPackage {
	t.Helper()

	module, _ := moduleInfo(t)
	packages := make(map[string]firstPartyPackage)

	err := filepath.WalkDir(root, func(p string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if moduletree.SkipDir(root, p) {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			return nil
		}

		rel, relErr := filepath.Rel(root, filepath.Dir(p))
		if relErr != nil {
			return relErr
		}
		importPath := module
		if rel != "." {
			importPath = module + "/" + filepath.ToSlash(rel)
		}

		pkg := packages[importPath]
		pkg.imports = append(pkg.imports, fileImports(t, p)...)
		packages[importPath] = pkg
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}

	if len(packages) == 0 {
		t.Fatal("no first-party packages were parsed; the graph would be vacuous")
	}
	return packages
}

// reachableFrom returns every import path reachable from start, mapped to the
// first-party package that pulled it in. External packages are recorded but not
// descended into, which is exactly the boundary this graph can see.
func reachableFrom(t *testing.T, packages map[string]firstPartyPackage, start string) map[string]string {
	t.Helper()

	if _, ok := packages[start]; !ok {
		t.Fatalf("%s is not a package in this module", start)
	}

	reached := make(map[string]string)
	queue := []string{start}
	visited := map[string]struct{}{start: {}}

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		for _, imported := range slices.Sorted(slices.Values(packages[current].imports)) {
			if _, seen := reached[imported]; !seen {
				reached[imported] = current
			}
			if _, isFirstParty := packages[imported]; !isFirstParty {
				continue
			}
			if _, done := visited[imported]; done {
				continue
			}
			visited[imported] = struct{}{}
			queue = append(queue, imported)
		}
	}

	return reached
}

// --- helpers ----------------------------------------------------------------

// moduleInfo returns the module path and the directory its go.mod lives in.
func moduleInfo(t *testing.T) (module, root string) {
	t.Helper()

	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		candidate := filepath.Join(dir, "go.mod")
		if data, readErr := os.ReadFile(candidate); readErr == nil {
			for line := range strings.SplitSeq(string(data), "\n") {
				if rest, ok := strings.CutPrefix(strings.TrimSpace(line), "module "); ok {
					return strings.TrimSpace(rest), dir
				}
			}
			t.Fatalf("%s has no module directive", candidate)
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found above the test working directory")
		}
		dir = parent
	}
}

func goSources(t *testing.T, dir string, includeTests bool) []string {
	t.Helper()

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read %s: %v", dir, err)
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
		files = append(files, filepath.Join(dir, name))
	}
	slices.Sort(files)
	return files
}

func fileImports(t *testing.T, file string) []string {
	t.Helper()

	parsed, err := parser.ParseFile(token.NewFileSet(), file, nil, parser.ImportsOnly)
	if err != nil {
		t.Fatalf("parse %s: %v", file, err)
	}

	paths := make([]string, 0, len(parsed.Imports))
	for _, spec := range parsed.Imports {
		unquoted, unquoteErr := strconv.Unquote(spec.Path.Value)
		if unquoteErr != nil {
			t.Fatalf("%s: unquote %s: %v", file, spec.Path.Value, unquoteErr)
		}
		paths = append(paths, unquoted)
	}
	return paths
}

// isStandardLibrary uses the rule the go tool itself uses: a standard library
// path has no dot in its first segment, because a domain name must.
func isStandardLibrary(importPath string) bool {
	first, _, _ := strings.Cut(importPath, "/")
	return !strings.Contains(first, ".")
}
