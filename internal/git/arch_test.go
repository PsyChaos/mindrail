package git

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// forbiddenImportFragments name the Git implementations tech-stack §34 rules
// out. Mindrail shells out to the system git on purpose: a reimplementation
// would have to be kept bug-compatible with the binary users actually run.
var forbiddenImportFragments = []string{
	"go-git",
	"libgit2",
	"git2go",
}

// shellLiterals are the exact strings that turn an argv invocation into a shell
// invocation. tech-stack §35 forbids the shell, and spec §113 names "no
// arbitrary shell interpolation" as a security requirement.
var shellLiterals = []string{
	"sh",
	"-c",
	"/bin/sh",
	"/bin/bash",
	"bash",
	"zsh",
	"cmd.exe",
	"powershell",
	"powershell.exe",
}

func TestNoGoGitDependency(t *testing.T) {
	t.Run("go.mod requires no git library", func(t *testing.T) {
		raw, err := os.ReadFile(filepath.Join("..", "..", "go.mod"))
		if err != nil {
			t.Fatalf("read go.mod: %v", err)
		}
		for _, fragment := range forbiddenImportFragments {
			if strings.Contains(string(raw), fragment) {
				t.Errorf("go.mod mentions %q; Mindrail drives the system git binary (tech-stack §34)", fragment)
			}
		}
	})

	t.Run("package sources import no git library", func(t *testing.T) {
		for _, file := range packageSources(t, true) {
			for _, path := range importsOf(t, file) {
				for _, fragment := range forbiddenImportFragments {
					if strings.Contains(path, fragment) {
						t.Errorf("%s imports %q", filepath.Base(file), path)
					}
				}
			}
		}
	})

	t.Run("package sources contain no shell literal", func(t *testing.T) {
		// Tests are excluded on purpose: this very table is a list of the
		// forbidden strings, and the rule is about what ships in the binary.
		for _, file := range packageSources(t, false) {
			for _, literal := range stringLiteralsOf(t, file) {
				for _, shell := range shellLiterals {
					if literal == shell {
						t.Errorf("%s contains the literal %q; every git call is argv-only", filepath.Base(file), literal)
					}
				}
			}
		}
	})

	t.Run("package sources never reach for a shell wrapper", func(t *testing.T) {
		for _, file := range packageSources(t, false) {
			for _, path := range importsOf(t, file) {
				if path == "os/exec" {
					continue
				}
				if strings.HasPrefix(path, "github.com/") && strings.Contains(path, "shell") {
					t.Errorf("%s imports the shell helper %q", filepath.Base(file), path)
				}
			}
		}
	})
}

// packageSources lists this package's Go files. Parsing the package itself
// rather than shelling out to `go list` keeps the assertion honest while the
// rest of the module is still being written.
func packageSources(t *testing.T, includeTests bool) []string {
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

func stringLiteralsOf(t *testing.T, file string) []string {
	t.Helper()

	parsed, err := parser.ParseFile(token.NewFileSet(), file, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", file, err)
	}

	var literals []string
	ast.Inspect(parsed, func(node ast.Node) bool {
		lit, ok := node.(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}
		value, unquoteErr := strconv.Unquote(lit.Value)
		if unquoteErr != nil {
			return true
		}
		literals = append(literals, value)
		return true
	})

	if len(literals) == 0 {
		t.Fatalf("%s yielded no string literals; the scan is not looking at anything", file)
	}
	return literals
}
