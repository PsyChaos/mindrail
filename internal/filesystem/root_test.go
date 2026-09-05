package filesystem

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/PsyChaos/mindrail/internal/app"
)

func TestNormalizeSlashPaths(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "already slash form", in: "a/b/c", want: "a/b/c"},
		{name: "leading dot segment", in: "./a/b", want: "a/b"},
		{name: "duplicate separators", in: "a//b", want: "a/b"},
		{name: "interior dot segment", in: "a/./b", want: "a/b"},
		{name: "interior parent segment", in: "a/b/../c", want: "a/c"},
		{name: "trailing separator", in: "a/b/", want: "a/b"},
		{name: "empty stays empty", in: "", want: ""},
		{name: "os native join", in: filepath.Join("a", "b", "c"), want: "a/b/c"},
		{name: "escaping prefix is preserved for the caller to reject", in: "../a", want: "../a"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Normalize(tt.in)
			if got != tt.want {
				t.Fatalf("Normalize(%q) = %q, want %q", tt.in, got, tt.want)
			}
			if os.PathSeparator != '/' && strings.ContainsRune(got, os.PathSeparator) {
				t.Fatalf("Normalize(%q) = %q, which still contains the OS separator", tt.in, got)
			}
		})
	}
}

func TestRootResolveContainment(t *testing.T) {
	root := newTestRoot(t, t.TempDir())

	tests := []struct {
		name    string
		rel     string
		wantRel string // relative to the canonical root, slash form; "" means expect an escape
	}{
		{name: "parent traversal", rel: "../../etc/passwd"},
		{name: "traversal after a descent", rel: "a/../../outside"},
		{name: "absolute outside", rel: string(os.PathSeparator) + "etc"},
		{name: "slash form absolute", rel: "/etc/passwd"},
		{name: "bare parent", rel: ".."},
		{name: "valid nested", rel: "a/b/c", wantRel: "a/b/c"},
		{name: "root itself", rel: ".", wantRel: "."},
		{name: "empty is the root", rel: "", wantRel: "."},
		{name: "normalized interior traversal stays inside", rel: "a/../b", wantRel: "b"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := root.Resolve(tt.rel)

			if tt.wantRel == "" {
				if !errors.Is(err, ErrEscapesRoot) {
					t.Fatalf("Resolve(%q) error = %v, want ErrEscapesRoot", tt.rel, err)
				}
				payload, ok := app.PayloadOf(err)
				if !ok {
					t.Fatalf("Resolve(%q) error carries no domain payload", tt.rel)
				}
				if payload.Code != app.CodePathEscapesRoot {
					t.Fatalf("Resolve(%q) payload code = %q, want %q", tt.rel, payload.Code, app.CodePathEscapesRoot)
				}
				return
			}

			if err != nil {
				t.Fatalf("Resolve(%q) unexpected error: %v", tt.rel, err)
			}
			want := root.Path()
			if tt.wantRel != "." {
				want = filepath.Join(root.Path(), filepath.FromSlash(tt.wantRel))
			}
			if got != want {
				t.Fatalf("Resolve(%q) = %q, want %q", tt.rel, got, want)
			}
			if !filepath.IsAbs(got) {
				t.Fatalf("Resolve(%q) = %q, which is not absolute", tt.rel, got)
			}

			contained, err := root.Contains(got)
			if err != nil {
				t.Fatalf("Contains(%q) unexpected error: %v", got, err)
			}
			if !contained {
				t.Fatalf("Contains(%q) = false for a path Resolve accepted", got)
			}
		})
	}
}

func TestRootHandlesSymlinkedRoot(t *testing.T) {
	requireSymlinks(t)

	base := t.TempDir()
	real := filepath.Join(base, "real")
	if err := os.MkdirAll(real, DirMode); err != nil {
		t.Fatalf("create real root: %v", err)
	}
	link := filepath.Join(base, "link")
	if err := os.Symlink(real, link); err != nil {
		t.Fatalf("symlink root: %v", err)
	}

	canonicalReal, err := filepath.EvalSymlinks(real)
	if err != nil {
		t.Fatalf("canonicalize real root: %v", err)
	}

	root, err := NewRoot(link)
	if err != nil {
		t.Fatalf("NewRoot on a symlinked directory: %v", err)
	}
	if root.Path() != canonicalReal {
		t.Fatalf("Path() = %q, want the canonical target %q", root.Path(), canonicalReal)
	}

	// A path reached through the symlinked root is still inside the root.
	resolved, err := root.Resolve("a/b")
	if err != nil {
		t.Fatalf("Resolve through symlinked root: %v", err)
	}
	if want := filepath.Join(canonicalReal, "a", "b"); resolved != want {
		t.Fatalf("Resolve = %q, want %q", resolved, want)
	}

	dir, err := root.EnsureDir("a/b")
	if err != nil {
		t.Fatalf("EnsureDir through symlinked root: %v", err)
	}
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		t.Fatalf("EnsureDir did not create a directory at %q (err=%v)", dir, err)
	}

	// The pre-canonicalization spelling of an inside path must be accepted too.
	contained, err := root.Contains(filepath.Join(link, "a", "b"))
	if err != nil {
		t.Fatalf("Contains through the symlinked spelling: %v", err)
	}
	if !contained {
		t.Fatalf("Contains(%q) = false; the symlinked spelling names the same directory", filepath.Join(link, "a", "b"))
	}
}

func TestRootRejectsSymlinkEscape(t *testing.T) {
	requireSymlinks(t)

	base := t.TempDir()
	rootDir := filepath.Join(base, "root")
	outside := filepath.Join(base, "outside")
	for _, dir := range []string{rootDir, outside} {
		if err := os.MkdirAll(dir, DirMode); err != nil {
			t.Fatalf("create %s: %v", dir, err)
		}
	}

	const original = "original contents"
	target := filepath.Join(outside, "target.txt")
	if err := os.WriteFile(target, []byte(original), FileMode); err != nil {
		t.Fatalf("seed target file: %v", err)
	}

	if err := os.Symlink(outside, filepath.Join(rootDir, "escape-dir")); err != nil {
		t.Fatalf("symlink escaping directory: %v", err)
	}
	if err := os.Symlink(target, filepath.Join(rootDir, "escape-file")); err != nil {
		t.Fatalf("symlink escaping file: %v", err)
	}
	if err := os.Symlink(filepath.Join(outside, "absent.txt"), filepath.Join(rootDir, "dangling")); err != nil {
		t.Fatalf("symlink dangling escape: %v", err)
	}

	root := newTestRoot(t, rootDir)

	escapes := []string{"escape-dir/target.txt", "escape-file", "escape-dir", "dangling"}
	for _, rel := range escapes {
		t.Run(rel, func(t *testing.T) {
			if _, err := root.Resolve(rel); !errors.Is(err, ErrEscapesRoot) {
				t.Fatalf("Resolve(%q) error = %v, want ErrEscapesRoot", rel, err)
			}
			written, _, err := root.WriteFileIfAbsent(rel, []byte("pwned"))
			if !errors.Is(err, ErrEscapesRoot) {
				t.Fatalf("WriteFileIfAbsent(%q) error = %v, want ErrEscapesRoot", rel, err)
			}
			if written {
				t.Fatalf("WriteFileIfAbsent(%q) reported a write through an escaping symlink", rel)
			}
			if _, err := root.EnsureDir(rel); !errors.Is(err, ErrEscapesRoot) {
				t.Fatalf("EnsureDir(%q) error = %v, want ErrEscapesRoot", rel, err)
			}
		})
	}

	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("re-read target file: %v", err)
	}
	if string(got) != original {
		t.Fatalf("target file content = %q, want %q; the escape was not contained", got, original)
	}
	if _, err := os.Lstat(filepath.Join(outside, "absent.txt")); !os.IsNotExist(err) {
		t.Fatalf("a dangling escaping symlink was followed and created outside the root")
	}
}

func TestRelSlashRejectsEscape(t *testing.T) {
	base := t.TempDir()
	rootDir := filepath.Join(base, "root")
	if err := os.MkdirAll(rootDir, DirMode); err != nil {
		t.Fatalf("create root: %v", err)
	}
	root := newTestRoot(t, rootDir)

	t.Run("outside absolute path", func(t *testing.T) {
		outside := filepath.Join(base, "outside", "file.txt")
		if _, err := root.RelSlash(outside); !errors.Is(err, ErrEscapesRoot) {
			t.Fatalf("RelSlash(%q) error = %v, want ErrEscapesRoot", outside, err)
		}
	})

	t.Run("sibling with a shared name prefix", func(t *testing.T) {
		sibling := rootDir + "-sibling"
		if err := os.MkdirAll(sibling, DirMode); err != nil {
			t.Fatalf("create sibling: %v", err)
		}
		if _, err := root.RelSlash(filepath.Join(sibling, "file.txt")); !errors.Is(err, ErrEscapesRoot) {
			t.Fatalf("RelSlash on a name-prefix sibling was accepted")
		}
	})

	t.Run("relative input is rejected", func(t *testing.T) {
		if _, err := root.RelSlash("a/b"); err == nil {
			t.Fatalf("RelSlash accepted a relative path")
		}
	})

	t.Run("nested path becomes slash form", func(t *testing.T) {
		got, err := root.RelSlash(filepath.Join(root.Path(), "a", "b", "c.txt"))
		if err != nil {
			t.Fatalf("RelSlash on a nested path: %v", err)
		}
		if got != "a/b/c.txt" {
			t.Fatalf("RelSlash = %q, want %q", got, "a/b/c.txt")
		}
	})

	t.Run("root itself is dot", func(t *testing.T) {
		got, err := root.RelSlash(root.Path())
		if err != nil {
			t.Fatalf("RelSlash on the root: %v", err)
		}
		if got != "." {
			t.Fatalf("RelSlash(root) = %q, want %q", got, ".")
		}
	})
}

func TestRootRejectsUnusableTargets(t *testing.T) {
	base := t.TempDir()
	root := newTestRoot(t, base)

	t.Run("the zero root contains nothing", func(t *testing.T) {
		var zero Root

		if zero.Path() != "" {
			t.Errorf("zero Root Path() = %q, want empty", zero.Path())
		}
		if _, err := zero.Resolve("a"); !errors.Is(err, errUninitializedRoot) {
			t.Errorf("zero Root Resolve error = %v, want errUninitializedRoot", err)
		}
		if _, err := zero.Contains(base); !errors.Is(err, errUninitializedRoot) {
			t.Errorf("zero Root Contains error = %v, want errUninitializedRoot", err)
		}
		if _, err := zero.RelSlash(base); !errors.Is(err, errUninitializedRoot) {
			t.Errorf("zero Root RelSlash error = %v, want errUninitializedRoot", err)
		}
	})

	t.Run("a file is not a root", func(t *testing.T) {
		file := filepath.Join(base, "plain.txt")
		if err := os.WriteFile(file, []byte("x"), FileMode); err != nil {
			t.Fatalf("seed file: %v", err)
		}
		if _, err := NewRoot(file); !errors.Is(err, ErrNotDirectory) {
			t.Fatalf("NewRoot on a file error = %v, want ErrNotDirectory", err)
		}
	})

	t.Run("a missing directory is not a root", func(t *testing.T) {
		if _, err := NewRoot(filepath.Join(base, "absent")); err == nil {
			t.Fatalf("NewRoot accepted a directory that does not exist")
		}
		if _, err := NewRoot("   "); err == nil {
			t.Fatalf("NewRoot accepted a blank path")
		}
	})

	t.Run("EnsureDir refuses to replace a file", func(t *testing.T) {
		if err := os.WriteFile(filepath.Join(base, "occupied"), []byte("x"), FileMode); err != nil {
			t.Fatalf("seed file: %v", err)
		}
		if _, err := root.EnsureDir("occupied"); !errors.Is(err, ErrNotDirectory) {
			t.Fatalf("EnsureDir over a file error = %v, want ErrNotDirectory", err)
		}
		content, err := os.ReadFile(filepath.Join(base, "occupied"))
		if err != nil || string(content) != "x" {
			t.Fatalf("the occupying file was disturbed: content=%q err=%v", content, err)
		}
	})

	t.Run("WriteFileIfAbsent refuses a file as a parent directory", func(t *testing.T) {
		if _, _, err := root.WriteFileIfAbsent("occupied/child.txt", []byte("x")); err == nil {
			t.Fatalf("WriteFileIfAbsent accepted a file as a parent directory")
		}
	})

	t.Run("Contains requires an absolute path", func(t *testing.T) {
		if _, err := root.Contains("relative/path"); err == nil {
			t.Fatalf("Contains accepted a relative path")
		}
	})

	t.Run("EnsureDir is idempotent", func(t *testing.T) {
		first, err := root.EnsureDir("nested/dir")
		if err != nil {
			t.Fatalf("EnsureDir: %v", err)
		}
		second, err := root.EnsureDir("nested/dir")
		if err != nil {
			t.Fatalf("second EnsureDir: %v", err)
		}
		if first != second {
			t.Fatalf("EnsureDir returned %q then %q", first, second)
		}
	})
}

// newTestRoot builds a Root or fails the test; every table below needs one.
func newTestRoot(t *testing.T, dir string) Root {
	t.Helper()

	root, err := NewRoot(dir)
	if err != nil {
		t.Fatalf("NewRoot(%q): %v", dir, err)
	}
	return root
}

// requireSymlinks skips on platforms where creating a symlink needs a
// privilege the test runner may not hold.
func requireSymlinks(t *testing.T) {
	t.Helper()

	if runtime.GOOS == "windows" {
		t.Skip("symbolic links require elevated privileges on Windows")
	}
}
