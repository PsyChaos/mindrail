package moduletree_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/PsyChaos/mindrail/internal/moduletree"
)

// TestRootFindsTheModuleFromBelow states what the four walkers rely on: the
// answer does not depend on how deep in the tree the caller stands.
func TestRootFindsTheModuleFromBelow(t *testing.T) {
	tree := t.TempDir()
	writeFile(t, filepath.Join(tree, "go.mod"), "module example.test\n")
	deep := filepath.Join(tree, "internal", "one", "two")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", deep, err)
	}

	for _, from := range []string{tree, filepath.Join(tree, "internal"), deep} {
		root, err := moduletree.Root(from)
		if err != nil {
			t.Fatalf("Root(%s): %v", from, err)
		}
		if root != tree {
			t.Errorf("Root(%s) = %s, want %s", from, root, tree)
		}
	}
}

// TestRootReportsThatThereIsNoModule keeps the failure a value rather than a
// silent empty string, so a caller can decide between skipping and failing.
func TestRootReportsThatThereIsNoModule(t *testing.T) {
	root, err := moduletree.Root(t.TempDir())
	if !errors.Is(err, moduletree.ErrNoModule) {
		t.Fatalf("Root of a directory with no go.mod above it = %q, %v; want ErrNoModule", root, err)
	}
}

// TestSkipDirKeepsTheWalkInsideThisModule is the rule F20 was reported against:
// a nested checkout carries a copy of every file in this module, and a walk
// that reads those copies attributes a second module's contents to this one.
func TestSkipDirKeepsTheWalkInsideThisModule(t *testing.T) {
	// The root's own basename starts with a dot on purpose: this project's own
	// tooling creates linked worktrees under .claude/worktrees/, and a checkout
	// living under a dot-prefixed directory of its own must not make the root
	// escape below unreachable.
	root := filepath.Join(t.TempDir(), ".mindrail")
	writeFile(t, filepath.Join(root, "go.mod"), "module example.test\n")

	nested := filepath.Join(root, "mode-b")
	writeFile(t, filepath.Join(nested, "go.mod"), "module example.test\n")

	cases := []struct {
		name string
		path string
		want bool
	}{
		{"the root itself is never skipped", root, false},
		{"ordinary source directory", filepath.Join(root, "internal"), false},
		{"dot-prefixed tooling state", filepath.Join(root, ".git"), true},
		{"the worktree directory this project's tooling creates", filepath.Join(root, ".claude"), true},
		{"vendored third-party source", filepath.Join(root, "vendor"), true},
		{"fixtures", filepath.Join(root, "testdata"), true},
		{"generated graph output", filepath.Join(root, "graphify-out"), true},
		{"a linked worktree, which carries its own go.mod", nested, true},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := moduletree.SkipDir(root, testCase.path); got != testCase.want {
				t.Errorf("SkipDir(%s) = %v, want %v", testCase.path, got, testCase.want)
			}
		})
	}
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
