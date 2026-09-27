package validation_test

import (
	"os"
	"path/filepath"
	"reflect"
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

func TestSnapshotScopeExpandsRecursiveGlobs(t *testing.T) {
	root := t.TempDir()
	writeScopeFile(t, root, "src/top.py", "print(1)\n")
	writeScopeFile(t, root, "src/nested/deep.py", "print(2)\n")
	writeScopeFile(t, root, "src/nested/not-python.txt", "ignored\n")

	snapshot, err := validation.SnapshotScope(root, []string{"src/**/*.py"})
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Scope) != 2 {
		t.Fatalf("glob scope = %+v, want the two Python files", snapshot.Scope)
	}
}

func TestBreakerGlobOutsideUnreadable(t *testing.T) {
	root := t.TempDir()
	writeScopeFile(t, root, "tests/unit/a.py", "print(1)\n")
	writeScopeFile(t, root, "outside/private/secret.txt", "secret\n")
	private := filepath.Join(root, "outside", "private")
	if err := os.Chmod(private, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(private, 0o755) })

	snapshot, err := validation.SnapshotScope(root, []string{"tests/**/*.py"})
	if err != nil {
		t.Fatalf("out-of-scope unreadable directory affected glob snapshot: %v", err)
	}
	want := []string{filepath.Join(root, "tests", "unit", "a.py")}
	if !reflect.DeepEqual(snapshot.Scope, want) {
		t.Fatalf("scope = %#v, want %#v", snapshot.Scope, want)
	}
}

func TestSnapshotScopeSelectionBoundaries(t *testing.T) {
	root := t.TempDir()
	writeScopeFile(t, root, "tests/a.py", "print(1)\n")
	writeScopeFile(t, root, "tests/nested/b.py", "print(2)\n")
	writeScopeFile(t, root, "other/c.py", "print(3)\n")
	if err := os.Symlink(filepath.Join(root, "other", "c.py"), filepath.Join(root, "tests", "linked.py")); err != nil {
		t.Fatal(err)
	}

	assertCount := func(t *testing.T, paths []string, want int) {
		t.Helper()
		snapshot, err := validation.SnapshotScope(root, paths)
		if err != nil {
			t.Fatal(err)
		}
		if len(snapshot.Scope) != want {
			t.Fatalf("scope for %q = %#v, want %d files", paths, snapshot.Scope, want)
		}
	}

	t.Run("literal file", func(t *testing.T) { assertCount(t, []string{"tests/a.py"}, 1) })
	t.Run("literal directory", func(t *testing.T) { assertCount(t, []string{"tests"}, 2) })
	t.Run("recursive double star", func(t *testing.T) { assertCount(t, []string{"tests/**/*.py"}, 2) })
	t.Run("wildcard before literal prefix", func(t *testing.T) { assertCount(t, []string{"*/**/*.py"}, 3) })
	t.Run("literal symlink is not followed", func(t *testing.T) { assertCount(t, []string{"tests/linked.py"}, 0) })
	t.Run("glob prefix symlink is not followed", func(t *testing.T) {
		outside := t.TempDir()
		writeScopeFile(t, outside, "nested/leaked.py", "leaked\n")
		if err := os.Symlink(outside, filepath.Join(root, "linked-dir")); err != nil {
			t.Fatal(err)
		}
		assertCount(t, []string{"linked-dir/nested/**/*.py"}, 0)
	})
	t.Run("escape is refused", func(t *testing.T) {
		if _, err := validation.SnapshotScope(root, []string{"../outside/*.py"}); err == nil {
			t.Fatal("escaping glob accepted")
		}
	})

	t.Run("unreadable in scope is refused", func(t *testing.T) {
		writeScopeFile(t, root, "tests/private/hidden.py", "hidden\n")
		private := filepath.Join(root, "tests", "private")
		if err := os.Chmod(private, 0); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(private, 0o755) })
		if _, err := validation.SnapshotScope(root, []string{"tests/**/*.py"}); err == nil {
			t.Fatal("unreadable in-scope directory accepted")
		}
	})

	t.Run("no safe prefix retains root walk", func(t *testing.T) {
		private := filepath.Join(root, "other")
		if err := os.Chmod(private, 0); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(private, 0o755) })
		if _, err := validation.SnapshotScope(root, []string{"*/**/*.py"}); err == nil {
			t.Fatal("root-walk glob ignored an unreadable potentially matching subtree")
		}
	})
}

func TestSnapshotScopeWildcardSegmentsStayAtTheirLevel(t *testing.T) {
	root := t.TempDir()
	writeScopeFile(t, root, "tests/a.py", "top\n")
	writeScopeFile(t, root, "tests/unit/b.py", "one level\n")
	writeScopeFile(t, root, "tests/unit/deeper/c.py", "two levels\n")
	writeScopeFile(t, root, "tests/private/deep.txt", "not python\n")

	for _, tc := range []struct {
		name    string
		pattern string
		want    []string
	}{
		{name: "one segment", pattern: "tests/*.py", want: []string{"tests/a.py"}},
		{name: "two segments", pattern: "tests/*/*.py", want: []string{"tests/unit/b.py"}},
		{name: "recursive", pattern: "tests/**/*.py", want: []string{"tests/a.py", "tests/unit/b.py", "tests/unit/deeper/c.py"}},
		{name: "wildcard first", pattern: "*/**/*.py", want: []string{"tests/a.py", "tests/unit/b.py", "tests/unit/deeper/c.py"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			snapshot, err := validation.SnapshotScope(root, []string{tc.pattern})
			if err != nil {
				t.Fatal(err)
			}
			want := make([]string, len(tc.want))
			for i, rel := range tc.want {
				want[i] = filepath.Join(root, filepath.FromSlash(rel))
			}
			if !reflect.DeepEqual(snapshot.Scope, want) {
				t.Fatalf("scope = %#v, want %#v", snapshot.Scope, want)
			}
		})
	}
}

func TestSnapshotScopeUnreadableGlobBoundaries(t *testing.T) {
	t.Run("unmatched directory is pruned", func(t *testing.T) {
		root := t.TempDir()
		writeScopeFile(t, root, "tests/unit/a.py", "selected\n")
		writeScopeFile(t, root, "tests/private/deep.py", "not selectable\n")
		private := filepath.Join(root, "tests", "private")
		if err := os.Chmod(private, 0); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(private, 0o755) })
		snapshot, err := validation.SnapshotScope(root, []string{"tests/unit*/*.py"})
		if err != nil {
			t.Fatal(err)
		}
		if len(snapshot.Scope) != 1 || snapshot.Scope[0] != filepath.Join(root, "tests", "unit", "a.py") {
			t.Fatalf("scope = %#v", snapshot.Scope)
		}
	})

	t.Run("matched directory fails closed", func(t *testing.T) {
		root := t.TempDir()
		writeScopeFile(t, root, "tests/unit/a.py", "selected\n")
		writeScopeFile(t, root, "tests/private/deep.txt", "unknown contents\n")
		private := filepath.Join(root, "tests", "private")
		if err := os.Chmod(private, 0); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(private, 0o755) })
		if _, err := validation.SnapshotScope(root, []string{"tests/*/*.py"}); err == nil {
			t.Fatal("potentially matching unreadable directory accepted")
		}
	})

	t.Run("matched file fails closed", func(t *testing.T) {
		root := t.TempDir()
		writeScopeFile(t, root, "tests/a.py", "selected\n")
		selected := filepath.Join(root, "tests", "a.py")
		if err := os.Chmod(selected, 0); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(selected, 0o644) })
		if _, err := validation.SnapshotScope(root, []string{"tests/*.py"}); err == nil {
			t.Fatal("matching unreadable file accepted")
		}
	})

	t.Run("recursive glob fails closed", func(t *testing.T) {
		root := t.TempDir()
		writeScopeFile(t, root, "tests/a.py", "selected\n")
		writeScopeFile(t, root, "tests/private/deep.txt", "unknown contents\n")
		private := filepath.Join(root, "tests", "private")
		if err := os.Chmod(private, 0); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(private, 0o755) })
		if _, err := validation.SnapshotScope(root, []string{"tests/**/*.py"}); err == nil {
			t.Fatal("recursive glob ignored a potentially matching unreadable directory")
		}
	})
}
