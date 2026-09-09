package coordination_test

import (
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PsyChaos/mindrail/internal/coordination"
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

	var offenders []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		switch {
		case err != nil:
			return err
		case entry.IsDir():
			// .git holds packed objects that would be read as text, and testdata
			// holds fixtures that are data rather than statements of the rule.
			if name := entry.Name(); name == ".git" || name == "testdata" {
				return fs.SkipDir
			}
			if path == thisPackage {
				return fs.SkipDir
			}
			return nil
		case !strings.HasSuffix(path, ".go"):
			return nil
		}

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
	return offenders
}

// moduleRoot finds the repository root from the test's working directory, which
// go test sets to the package directory.
func moduleRoot(t *testing.T) string {
	t.Helper()

	out, err := exec.Command("go", "env", "GOMOD").Output()
	if err != nil {
		t.Skipf("go env GOMOD: %v", err)
	}
	gomod := strings.TrimSpace(string(out))
	if gomod == "" || gomod == os.DevNull {
		t.Skip("not inside a module")
	}
	return filepath.Dir(gomod)
}
