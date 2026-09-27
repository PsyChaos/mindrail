package record_test

import (
	"go/parser"
	"go/token"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// allowedImports is every package the non-test sources of internal/knowledge/record
// may import.
//
// It is an exact set rather than a deny-list of dangerous packages, because a
// deny-list only refuses what somebody thought of: "os/exec", "net/http" and
// "internal/filesystem" are obvious, while "io/fs", "embed" and any future
// helper that wraps one of them are not. Widening this set is a deliberate
// three-character edit that shows up in review; reaching a filesystem without
// touching it is impossible.
var allowedImports = []string{
	"errors",
	"fmt",
	"regexp",
	"slices",
	"strings",
	"time",
}

// TestRecordPackageImportsNothingThatCanReachTheFilesystem is what makes
// AC-01.4's "writes nothing and touches no filesystem" checkable.
//
// Asserting it about Supersede alone would be an assertion about one function
// body, and the next edit could add an os.WriteFile two lines below without any
// test noticing. Asserting the whole import set makes the claim structural: no
// function in this package can open, read or write anything, because nothing it
// imports can. It is also what keeps design §2's "record depends on nothing else
// in internal/knowledge" true, since an internal import would fail here too.
func TestRecordPackageImportsNothingThatCanReachTheFilesystem(t *testing.T) {
	sources, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("listing package sources: %v", err)
	}

	// A glob that matched nothing, or only tests, would make every assertion
	// below vacuous — the loop would simply not run.
	var production []string
	for _, source := range sources {
		if !strings.HasSuffix(source, "_test.go") {
			production = append(production, source)
		}
	}
	if len(production) == 0 {
		t.Fatal("found no non-test sources in the package directory, so this test asserted nothing")
	}

	fileSet := token.NewFileSet()
	seen := make(map[string][]string)
	for _, source := range production {
		parsed, parseErr := parser.ParseFile(fileSet, source, nil, parser.ImportsOnly)
		if parseErr != nil {
			t.Fatalf("parsing %s: %v", source, parseErr)
		}
		for _, spec := range parsed.Imports {
			path, unquoteErr := strconv.Unquote(spec.Path.Value)
			if unquoteErr != nil {
				t.Fatalf("%s has an unparsable import path %s: %v", source, spec.Path.Value, unquoteErr)
			}
			seen[path] = append(seen[path], source)
		}
	}

	for path, files := range seen {
		if !slices.Contains(allowedImports, path) {
			t.Errorf("%s imports %q, which is not in the allowed set %v; a record package that can reach a filesystem cannot promise it does not",
				strings.Join(files, ", "), path, allowedImports)
		}
	}
}
