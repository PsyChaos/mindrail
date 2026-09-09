package loader

import (
	"os"
	"path"
	"path/filepath"
	"testing"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/filesystem"
)

// This is the package's only in-package test, and the reason it exists is the
// reason the finding that asked for it was filed.
//
// recordFile's fast path is sound only because the name it is handed is a single
// path component: filepath.Join(absDir, name) is the entry's canonical path when
// name is a leaf, and is an arbitrary path anywhere on the disk when it is not.
// The guard that states that precondition could not be falsified from outside
// the package — every caller reaches it through Load, and os.ReadDir yields
// nothing but leaf names — so replacing the guard with `if true` left all
// eighteen packages green (finding RD-05). A branch no test can turn red is a
// branch nobody is holding.
//
// Everything else about this package is tested through Load, from
// package loader_test, and should stay there.

// TestTheFastPathIsNotTakenForANameThatIsNotOneComponent turns that guard red.
//
// The name below is what a caller assembling paths itself could hand this
// function. Joined onto the bucket it walks out of the worktree entirely, so a
// fast path taken for it would Lstat a file the repository does not contain,
// find an ordinary file, and hand its path back as a record to read — the
// containment boundary skipped, not consulted.
//
// The assertion is on both returns. A test that only checked for a non-nil
// Problem would pass on a function that refused everything, and refusing the
// ordinary record is the other half of what this guard must not do; the second
// half of the test is the row that holds it.
func TestTheFastPathIsNotTakenForANameThatIsNotOneComponent(t *testing.T) {
	parent := t.TempDir()
	worktree := filepath.Join(parent, "worktree")
	relDir := path.Join(StoreRoot, "decisions")
	if err := os.MkdirAll(filepath.Join(worktree, filepath.FromSlash(relDir)), 0o755); err != nil {
		t.Fatalf("creating the decisions bucket: %v", err)
	}

	// Canonicalise the worktree before anything is built from it. On macOS
	// TMPDIR is reached through a symbolic link by default, so filesystem.Root
	// would resolve to /private/var/... while a path assembled here with
	// filepath.Join would keep /var/... — and the final comparison below would
	// fail on the platform rather than on the code. Audit round 4 raised this
	// (R4-M23); the finding was refuted, because a broken guard still aborts at
	// the assertion above that comparison, but a test that goes red on a
	// supported platform is worth not shipping either way.
	canonical, err := filepath.EvalSymlinks(worktree)
	if err != nil {
		t.Fatalf("canonicalising the worktree %q: %v", worktree, err)
	}
	worktree = canonical

	// An ordinary file outside the worktree, in the directory the escaping name
	// below lands in. Without it the fast path would fail at the Lstat and the
	// test would pass for the wrong reason.
	outside := filepath.Join(parent, "outside.json")
	if err := os.WriteFile(outside, []byte(`{"id":"DEC-0001"}`), 0o644); err != nil {
		t.Fatalf("writing the file outside the worktree: %v", err)
	}

	root, err := filesystem.NewRoot(worktree)
	if err != nil {
		t.Fatalf("NewRoot(%q): %v", worktree, err)
	}
	l := New(root, nil)

	absDir, err := root.Resolve(relDir)
	if err != nil {
		t.Fatalf("resolving the bucket %q: %v", relDir, err)
	}

	// rel is built exactly as readRecord builds it, so the second branch is
	// handed the path it would really see.
	const escaping = "../../../../outside.json"
	abs, problem := l.recordFile(absDir, path.Join(relDir, escaping), escaping)

	if problem == nil {
		t.Fatalf("recordFile accepted %q and returned %q; the name leaves the worktree, "+
			"so the fast path skipped the boundary rather than consulting it", escaping, abs)
	}
	if problem.Code != app.CodePathEscapesRoot {
		t.Errorf("recordFile reported %q for a name that leaves the worktree, want %q",
			problem.Code, app.CodePathEscapesRoot)
	}
	if abs != "" {
		t.Errorf("recordFile refused %q and still returned a path to read: %q", escaping, abs)
	}

	// The over-fire arm, in the same function and on the same disk: an ordinary
	// leaf name is still answered by the fast path, with the entry's own
	// canonical path and no problem at all.
	const ordinary = "DEC-0001.json"
	entry := filepath.Join(worktree, filepath.FromSlash(relDir), ordinary)
	if err := os.WriteFile(entry, []byte(`{"id":"DEC-0001"}`), 0o644); err != nil {
		t.Fatalf("writing the record: %v", err)
	}

	abs, problem = l.recordFile(absDir, path.Join(relDir, ordinary), ordinary)
	if problem != nil {
		t.Fatalf("recordFile refused an ordinary record: %+v", problem)
	}
	if abs != entry {
		t.Errorf("recordFile returned %q for an ordinary record, want the entry it walked %q", abs, entry)
	}
}
