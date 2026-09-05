package storage_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// driverModule is the one SQLite implementation this binary links. Everything
// else in the repository talks database/sql (tech-stack §20), so swapping the
// driver stays a one-file change and no query can grow a driver-specific
// dependency by accident.
const driverModule = "modernc.org/sqlite"

// driverFile is the single file allowed to import it.
const driverFile = "driver.go"

// ormModules are the packages tech-stack §20 rules out. They are listed by
// import path prefix so a versioned suffix (gorm.io/gorm/v2) still matches.
var ormModules = []string{
	"gorm.io/",
	"github.com/jinzhu/gorm",
	"entgo.io/ent",
	"github.com/volatiletech/sqlboiler",
	"github.com/uptrace/bun",
	"xorm.io/",
	"github.com/jmoiron/sqlx",
	"github.com/go-gorp/gorp",
}

// TestDriverImportConfinedToStorage parses this package rather than trusting a
// convention: an import added to pragma.go or tx.go compiles fine and would go
// unnoticed until someone tried to change drivers.
func TestDriverImportConfinedToStorage(t *testing.T) {
	imports := packageImports(t, ".")

	for _, file := range slices.Sorted(maps.Keys(imports)) {
		uses := false
		for _, path := range imports[file] {
			if path == driverModule || strings.HasPrefix(path, driverModule+"/") {
				uses = true
			}
		}

		switch {
		case file == driverFile && !uses:
			t.Errorf("%s does not import %s; the driver seam moved without updating this test", driverFile, driverModule)
		case file != driverFile && uses:
			t.Errorf("%s imports %s; only %s may (tech-stack §20)", file, driverModule, driverFile)
		}
	}
}

// TestNoORMInStorage is the second half of the §20 rule: the storage package
// must remain a database/sql adapter, not a mapping layer.
func TestNoORMInStorage(t *testing.T) {
	for file, paths := range packageImports(t, ".") {
		for _, path := range paths {
			for _, orm := range ormModules {
				if strings.HasPrefix(path, orm) {
					t.Errorf("%s imports %q; MR-001 uses database/sql only", file, path)
				}
			}
		}
	}
}

// TestDriverImportUnreachableFromOtherPackages widens the confinement to the
// whole module. Import confinement inside one package is worth little if
// internal/doctor can reach the driver directly.
func TestDriverImportUnreachableFromOtherPackages(t *testing.T) {
	root := moduleRoot(t)
	storageDir := filepath.Join(root, "internal", "storage")

	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if shouldSkipDir(entry.Name()) {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		if filepath.Dir(path) == storageDir {
			return nil // covered, file by file, by TestDriverImportConfinedToStorage
		}

		for _, imported := range fileImports(t, path) {
			if imported == driverModule || strings.HasPrefix(imported, driverModule+"/") {
				rel, relErr := filepath.Rel(root, path)
				if relErr != nil {
					rel = path
				}
				t.Errorf("%s imports %s; only internal/storage/%s may", rel, driverModule, driverFile)
			}
			for _, orm := range ormModules {
				if strings.HasPrefix(imported, orm) {
					rel, relErr := filepath.Rel(root, path)
					if relErr != nil {
						rel = path
					}
					t.Errorf("%s imports %q; MR-001 uses database/sql only", rel, imported)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
}

// TestGoModDeclaresNoORM catches a dependency that is vendored in but not yet
// imported, which is the state just before someone starts using it.
func TestGoModDeclaresNoORM(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(moduleRoot(t), "go.mod"))
	if err != nil {
		t.Fatalf("read go.mod: %v", err)
	}

	for _, orm := range ormModules {
		if strings.Contains(string(data), orm) {
			t.Errorf("go.mod requires %q; MR-001 uses database/sql only", orm)
		}
	}
}

// packageImports maps each .go file in dir to its import paths.
func packageImports(t *testing.T, dir string) map[string][]string {
	t.Helper()

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read %s: %v", dir, err)
	}

	imports := make(map[string][]string, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") {
			continue
		}
		imports[entry.Name()] = fileImports(t, filepath.Join(dir, entry.Name()))
	}
	if len(imports) == 0 {
		t.Fatalf("no Go files found in %s", dir)
	}
	return imports
}

func fileImports(t *testing.T, path string) []string {
	t.Helper()

	parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}

	paths := make([]string, 0, len(parsed.Imports))
	for _, spec := range parsed.Imports {
		paths = append(paths, importPath(t, spec))
	}
	return paths
}

func importPath(t *testing.T, spec *ast.ImportSpec) string {
	t.Helper()

	unquoted, err := strconv.Unquote(spec.Path.Value)
	if err != nil {
		t.Fatalf("unquote import %s: %v", spec.Path.Value, err)
	}
	return unquoted
}

// shouldSkipDir keeps the walk inside first-party source.
func shouldSkipDir(name string) bool {
	return strings.HasPrefix(name, ".") || name == "vendor" || name == "testdata" || name == "graphify-out"
}

// moduleRoot walks up from the test's working directory to the go.mod, so the
// test does not care how deep in the tree the package sits.
func moduleRoot(t *testing.T) string {
	t.Helper()

	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found above the test working directory")
		}
		dir = parent
	}
}
