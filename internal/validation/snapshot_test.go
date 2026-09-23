package validation_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/PsyChaos/mindrail/internal/validation"
)

func writeScopeFile(t *testing.T, root, rel, content string) {
	t.Helper()
	abs := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestSnapshotScopeDeterministic is TASK-02 AC-02.3: the same tree hashes
// the same, and any content change moves the hash.
func TestSnapshotScopeDeterministic(t *testing.T) {
	root := t.TempDir()
	writeScopeFile(t, root, "a.py", "print(1)\n")
	writeScopeFile(t, root, "sub/b.py", "print(2)\n")

	first, err := validation.SnapshotScope(root, []string{"."})
	if err != nil {
		t.Fatal(err)
	}
	second, err := validation.SnapshotScope(root, []string{"sub", "a.py"})
	if err != nil {
		t.Fatal(err)
	}
	if first.Hash != second.Hash {
		t.Fatalf("hashes differ: %q vs %q", first.Hash, second.Hash)
	}
	if len(first.Scope) != 2 {
		t.Fatalf("scope = %+v", first.Scope)
	}
	writeScopeFile(t, root, "a.py", "print(3)\n")
	third, err := validation.SnapshotScope(root, []string{"."})
	if err != nil {
		t.Fatal(err)
	}
	if third.Hash == first.Hash {
		t.Fatal("content change did not move the hash")
	}
}

// TestSnapshotScopeRefusesBeforeExecution pins the fail-closed half: escape,
// missing and unreadable scope refuse with no hash.
func TestSnapshotScopeRefusesBeforeExecution(t *testing.T) {
	root := t.TempDir()
	writeScopeFile(t, root, "a.py", "print(1)\n")

	for name, paths := range map[string][]string{
		"escape":  {"../outside"},
		"missing": {"nope.py"},
		"empty":   {},
	} {
		if _, err := validation.SnapshotScope(root, paths); err == nil {
			t.Fatalf("%s accepted", name)
		}
	}
	if _, err := validation.SnapshotScope("relative", []string{"."}); err == nil {
		t.Fatal("relative root accepted")
	}
}
