package coordination_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PsyChaos/mindrail/internal/coordination"
	"github.com/PsyChaos/mindrail/internal/moduletree"
)

// lifecycleLiteralsOutsideThisPackage returns every Go file outside
// internal/coordination that spells all seven task states as literals.
//
// The threshold is *all seven* rather than any of them, and that is what keeps
// this from being noise. A command test that arranges a blocked task will
// naturally write "BLOCKED"; a file that writes out the whole vocabulary is
// stating the lifecycle, and the statement is the thing decision D-55 keeps in
// one place. Help text, flag validation and rendering all have
// coordination.States() to call instead.
func lifecycleLiteralsOutsideThisPackage(t *testing.T) []string {
	t.Helper()

	root := moduleRoot(t)
	thisPackage := filepath.Join(root, "internal", "coordination")

	needles := make([]string, 0, 7)
	for _, state := range coordination.States() {
		needles = append(needles, `"`+string(state)+`"`)
	}

	inspected := 0
	var offenders []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		switch {
		case err != nil:
			return err
		case entry.IsDir():
			// moduletree.SkipDir keeps the walk inside this module's own
			// sources: out of tooling state, out of fixtures, and out of any
			// nested checkout, whose copy of state.go is the same statement
			// rather than a second one.
			if moduletree.SkipDir(root, path) {
				return fs.SkipDir
			}
			if path == thisPackage {
				return fs.SkipDir
			}
			return nil
		case !strings.HasSuffix(path, ".go"):
			return nil
		}
		inspected++

		body, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		for _, needle := range needles {
			if !strings.Contains(string(body), needle) {
				return nil
			}
		}

		relative, relErr := filepath.Rel(root, path)
		if relErr != nil {
			relative = path
		}
		offenders = append(offenders, relative)
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", root, err)
	}
	// An empty offender list is supposed to mean nothing outside this package
	// spells the lifecycle out. It would mean exactly the same thing if the walk
	// never left the root, so absence of offenders alone cannot be the pass
	// condition.
	if inspected == 0 {
		t.Fatal("no files outside internal/coordination were inspected; the lifecycle check would be vacuous")
	}
	return offenders
}

// moduleRoot finds the repository root from the test's working directory, which
// go test sets to the package directory.
func moduleRoot(t *testing.T) string {
	t.Helper()

	working, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	root, err := moduletree.Root(working)
	if err != nil {
		t.Fatalf("module root above %s: %v", working, err)
	}
	return root
}
