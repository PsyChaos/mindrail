// Package moduletree answers the two questions every walk over this module's
// own sources has to answer: where the module root is, and which directories
// under it are not this module's sources.
//
// Four tests walk the whole tree to state a rule about it — the driver
// confinement in internal/storage, the layering graph in internal/cli, the
// single knowledge call site in internal/bootstrap and the single statement of
// the task lifecycle in internal/coordination. Each carried its own copy of the
// walk, and the copies drifted: the newest one skipped two directory names
// where the older three skipped every dot-prefixed directory, so a linked
// worktree — the layout this project's own tooling creates under
// .claude/worktrees/ — turned it red for a file that was a copy of the file the
// rule protects.
//
// The package deliberately does not import testing. It is an ordinary library
// so that the callers, which are tests, stay free to decide whether a failure
// here is a skip or a fatal.
package moduletree

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// ErrNoModule reports that no go.mod was found above the starting directory.
var ErrNoModule = errors.New("no go.mod above the starting directory")

// Root walks up from dir to the directory holding go.mod.
//
// Walking up is preferred to asking the go tool, because it works without the
// go tool on PATH and because it answers for the tree the caller is standing
// in rather than for the tree the environment points at.
func Root(dir string) (string, error) {
	absolute, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	for {
		if _, statErr := os.Stat(filepath.Join(absolute, "go.mod")); statErr == nil {
			return absolute, nil
		}
		parent := filepath.Dir(absolute)
		if parent == absolute {
			return "", ErrNoModule
		}
		absolute = parent
	}
}

// SkipDir reports whether a walk rooted at root must not descend into path.
//
// Two rules. The first is by name: a dot-prefixed directory holds tooling
// state rather than source, vendor holds other people's source, and testdata
// and graphify-out hold data that reads as source without being a statement of
// anything.
//
// The second is by boundary. A nested checkout — a linked worktree, a second
// clone, a vendored sibling module — carries its own go.mod and its own copy of
// every file in this module. Those copies are not this module's statements
// about itself, and a walk that reads them attributes a second module's
// contents to this one.
func SkipDir(root, path string) bool {
	name := filepath.Base(path)
	if strings.HasPrefix(name, ".") || name == "vendor" || name == "testdata" || name == "graphify-out" {
		return true
	}
	if path == root {
		return false
	}
	_, err := os.Stat(filepath.Join(path, "go.mod"))
	return err == nil
}
