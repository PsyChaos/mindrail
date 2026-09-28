package validation_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/PsyChaos/mindrail/internal/validation"
)

func initSnapshotGitRepository(t *testing.T, root string) {
	t.Helper()
	command := exec.Command("git", "init", "--quiet")
	command.Dir = root
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, output)
	}
}

func addSnapshotGitPaths(t *testing.T, root string, paths ...string) {
	t.Helper()
	args := append([]string{"add", "--"}, paths...)
	command := exec.Command("git", args...)
	command.Dir = root
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git add: %v: %s", err, output)
	}
}

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

func TestSnapshotScopeGitWorktreeIgnoresRuntimeOutputsAndGitMetadata(t *testing.T) {
	root := t.TempDir()
	initSnapshotGitRepository(t, root)
	writeScopeFile(t, root, ".gitignore", ".next/\n.remember/\n*.tsbuildinfo\n")
	writeScopeFile(t, root, "src/tracked.go", "package source\n")
	writeScopeFile(t, root, ".mindrail/config.toml", "[validation]\n")
	writeScopeFile(t, root, "src/untracked.go", "package source\n")
	writeScopeFile(t, root, ".next/build.json", "first\n")
	writeScopeFile(t, root, ".remember/tmp/hook.json", "first\n")
	writeScopeFile(t, root, "app/tsconfig.tsbuildinfo", "first\n")
	addSnapshotGitPaths(t, root, ".gitignore", "src/tracked.go", ".mindrail/config.toml")

	first, err := validation.SnapshotScope(root, []string{"."})
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range first.Scope {
		rel, err := filepath.Rel(root, file)
		if err != nil {
			t.Fatal(err)
		}
		if rel == ".git" || strings.HasPrefix(rel, ".git"+string(filepath.Separator)) ||
			strings.HasPrefix(rel, ".next"+string(filepath.Separator)) ||
			strings.HasPrefix(rel, ".remember"+string(filepath.Separator)) ||
			strings.HasSuffix(rel, ".tsbuildinfo") {
			t.Fatalf("runtime or Git metadata entered snapshot scope: %q", rel)
		}
	}

	writeScopeFile(t, root, ".git/mindrail/runtime.db", "changed\n")
	writeScopeFile(t, root, ".next/build.json", "changed\n")
	writeScopeFile(t, root, ".remember/tmp/hook.json", "changed\n")
	writeScopeFile(t, root, "app/tsconfig.tsbuildinfo", "changed\n")
	second, err := validation.SnapshotScope(root, []string{"."})
	if err != nil {
		t.Fatal(err)
	}
	if second.Hash != first.Hash {
		t.Fatalf("ignored runtime output or Git metadata moved snapshot hash: %q != %q", second.Hash, first.Hash)
	}

	writeScopeFile(t, root, "src/untracked.go", "package changed\n")
	untracked, err := validation.SnapshotScope(root, []string{"."})
	if err != nil {
		t.Fatal(err)
	}
	if untracked.Hash == first.Hash {
		t.Fatal("non-ignored untracked source change did not move snapshot hash")
	}

	writeScopeFile(t, root, "src/untracked.go", "package source\n")
	writeScopeFile(t, root, "src/tracked.go", "package changed\n")
	tracked, err := validation.SnapshotScope(root, []string{"."})
	if err != nil {
		t.Fatal(err)
	}
	if tracked.Hash == first.Hash {
		t.Fatal("tracked source change did not move snapshot hash")
	}
}

func TestSnapshotScopeGitWorktreeKeepsTrackedAndExplicitIgnoredFiles(t *testing.T) {
	root := t.TempDir()
	initSnapshotGitRepository(t, root)
	writeScopeFile(t, root, ".gitignore", "*.lock\n*.log\n")
	writeScopeFile(t, root, "generated.lock", "first\n")
	writeScopeFile(t, root, "explicit.log", "first\n")
	addSnapshotGitPaths(t, root, ".gitignore")
	command := exec.Command("git", "add", "--force", "--", "generated.lock")
	command.Dir = root
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git add --force: %v: %s", err, output)
	}

	trackedFirst, err := validation.SnapshotScope(root, []string{"."})
	if err != nil {
		t.Fatal(err)
	}
	writeScopeFile(t, root, "generated.lock", "changed\n")
	trackedSecond, err := validation.SnapshotScope(root, []string{"."})
	if err != nil {
		t.Fatal(err)
	}
	if trackedSecond.Hash == trackedFirst.Hash {
		t.Fatal("tracked ignored file change did not move snapshot hash")
	}

	explicitFirst, err := validation.SnapshotScope(root, []string{"explicit.log"})
	if err != nil {
		t.Fatal(err)
	}
	writeScopeFile(t, root, "explicit.log", "changed\n")
	explicitSecond, err := validation.SnapshotScope(root, []string{"explicit.log"})
	if err != nil {
		t.Fatal(err)
	}
	if explicitSecond.Hash == explicitFirst.Hash {
		t.Fatal("explicit ignored file change did not move snapshot hash")
	}
}

func TestSnapshotScopeGitWorktreeTracksDeletion(t *testing.T) {
	root := t.TempDir()
	initSnapshotGitRepository(t, root)
	writeScopeFile(t, root, "src/a.go", "package source\n")
	writeScopeFile(t, root, "src/b.go", "package source\n")
	addSnapshotGitPaths(t, root, "src/a.go", "src/b.go")

	first, err := validation.SnapshotScope(root, []string{"src"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(root, "src", "b.go")); err != nil {
		t.Fatal(err)
	}
	second, err := validation.SnapshotScope(root, []string{"src"})
	if err != nil {
		t.Fatal(err)
	}
	if second.Hash == first.Hash {
		t.Fatal("tracked file deletion did not move snapshot hash")
	}
	if len(second.Scope) != 1 {
		t.Fatalf("scope after deletion = %#v, want one existing file", second.Scope)
	}
}

func TestSnapshotScopeGitWorktreeIgnoresAmbientRepositoryOverrides(t *testing.T) {
	root := t.TempDir()
	initSnapshotGitRepository(t, root)
	writeScopeFile(t, root, "root.txt", "root\n")
	addSnapshotGitPaths(t, root, "root.txt")

	foreign := t.TempDir()
	initSnapshotGitRepository(t, foreign)
	writeScopeFile(t, foreign, "foreign.txt", "foreign\n")
	addSnapshotGitPaths(t, foreign, "foreign.txt")
	t.Setenv("GIT_DIR", filepath.Join(foreign, ".git"))
	t.Setenv("GIT_WORK_TREE", foreign)
	t.Setenv("GIT_INDEX_FILE", filepath.Join(foreign, ".git", "index"))

	snapshot, err := validation.SnapshotScope(root, []string{"."})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{filepath.Join(root, "root.txt")}
	if !reflect.DeepEqual(snapshot.Scope, want) {
		t.Fatalf("scope = %#v, want repository-local inventory %#v", snapshot.Scope, want)
	}
}

func TestSnapshotScopeGitWorktreeTreatsLiteralPathspecAsLiteral(t *testing.T) {
	root := t.TempDir()
	initSnapshotGitRepository(t, root)
	writeScopeFile(t, root, ":(glob)literal.txt", "source\n")

	snapshot, err := validation.SnapshotScope(root, []string{":(glob)literal.txt"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{filepath.Join(root, ":(glob)literal.txt")}
	if !reflect.DeepEqual(snapshot.Scope, want) {
		t.Fatalf("scope = %#v, want %#v", snapshot.Scope, want)
	}
}

func TestSnapshotScopeGitWorktreeDoesNotFollowParentSymlinks(t *testing.T) {
	root := t.TempDir()
	initSnapshotGitRepository(t, root)
	writeScopeFile(t, root, "src/a.go", "package source\n")
	addSnapshotGitPaths(t, root, "src/a.go")

	outside := t.TempDir()
	writeScopeFile(t, outside, "a.go", "outside secret\n")
	if err := os.Remove(filepath.Join(root, "src", "a.go")); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(root, "src")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "src")); err != nil {
		t.Fatal(err)
	}

	for name, paths := range map[string][]string{
		"root directory": {"."},
		"recursive glob": {"src/**/*.go"},
		"literal file":   {"src/a.go"},
	} {
		t.Run(name, func(t *testing.T) {
			snapshot, err := validation.SnapshotScope(root, paths)
			if err != nil {
				t.Fatal(err)
			}
			if len(snapshot.Scope) != 0 {
				t.Fatalf("scope followed a parent symlink outside the worktree: %#v", snapshot.Scope)
			}
		})
	}
}

func TestSnapshotScopeGitWorktreeRefusesSelectedSubmodule(t *testing.T) {
	root := t.TempDir()
	initSnapshotGitRepository(t, root)
	writeScopeFile(t, root, "tracked.txt", "source\n")
	addSnapshotGitPaths(t, root, "tracked.txt")
	commit := exec.Command("git", "-c", "user.name=Mindrail Tests", "-c", "user.email=mindrail@example.invalid", "commit", "--quiet", "-m", "fixture")
	commit.Dir = root
	if output, err := commit.CombinedOutput(); err != nil {
		t.Fatalf("git commit: %v: %s", err, output)
	}
	head := exec.Command("git", "rev-parse", "HEAD")
	head.Dir = root
	output, err := head.Output()
	if err != nil {
		t.Fatal(err)
	}
	writeScopeFile(t, root, "vendor/src/module.go", "package module\n")
	cacheInfo := "160000," + strings.TrimSpace(string(output)) + ",vendor"
	update := exec.Command("git", "update-index", "--add", "--cacheinfo", cacheInfo)
	update.Dir = root
	if output, err := update.CombinedOutput(); err != nil {
		t.Fatalf("git update-index: %v: %s", err, output)
	}

	for name, paths := range map[string][]string{
		"root":                 {"."},
		"recursive glob":       {"vendor/**/*.go"},
		"descendant directory": {"vendor/src"},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := validation.SnapshotScope(root, paths); err == nil {
				t.Fatal("selected Git submodule was silently omitted")
			}
		})
	}

	for name, paths := range map[string][]string{
		"root-only glob":          {"*.go"},
		"unmatched nested prefix": {"tests/unit*/*.go"},
	} {
		t.Run("unrelated "+name, func(t *testing.T) {
			if _, err := validation.SnapshotScope(root, paths); err != nil {
				t.Fatalf("unrelated submodule affected narrow scope: %v", err)
			}
		})
	}
}

func TestSnapshotScopeGitWorktreeRefusesSelectedEmbeddedRepository(t *testing.T) {
	root := t.TempDir()
	initSnapshotGitRepository(t, root)
	writeScopeFile(t, root, "root.go", "package root\n")
	addSnapshotGitPaths(t, root, "root.go")
	nested := filepath.Join(root, "nested")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	initSnapshotGitRepository(t, nested)
	writeScopeFile(t, nested, "source.go", "package nested\n")

	for name, paths := range map[string][]string{
		"root":                 {"."},
		"recursive glob":       {"nested/**/*.go"},
		"embedded directory":   {"nested"},
		"descendant directory": {"nested/subdir"},
	} {
		t.Run(name, func(t *testing.T) {
			if name == "descendant directory" {
				if err := os.MkdirAll(filepath.Join(nested, "subdir"), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := validation.SnapshotScope(root, paths); err == nil {
				t.Fatal("selected embedded repository was silently omitted")
			}
		})
	}

	snapshot, err := validation.SnapshotScope(root, []string{"*.go"})
	if err != nil {
		t.Fatalf("unrelated embedded repository affected root-only glob: %v", err)
	}
	want := []string{filepath.Join(root, "root.go")}
	if !reflect.DeepEqual(snapshot.Scope, want) {
		t.Fatalf("scope = %#v, want %#v", snapshot.Scope, want)
	}
}

func TestSnapshotScopeGitWorktreeFindsEmbeddedRepositoryBelowUntrackedParent(t *testing.T) {
	root := t.TempDir()
	initSnapshotGitRepository(t, root)
	nested := filepath.Join(root, "outer", "nested")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	initSnapshotGitRepository(t, nested)
	writeScopeFile(t, nested, "source.go", "package nested\n")

	if _, err := validation.SnapshotScope(root, []string{"outer/nested/**/*.go"}); err == nil {
		t.Fatal("embedded repository below an untracked parent was silently omitted")
	}
	if _, err := validation.SnapshotScope(root, []string{"unrelated/**/*.go"}); err != nil {
		t.Fatalf("unrelated embedded repository affected narrow scope: %v", err)
	}
}

func TestSnapshotScopeGitWorktreeRefusesUnreadableUntrackedDirectory(t *testing.T) {
	root := t.TempDir()
	initSnapshotGitRepository(t, root)
	writeScopeFile(t, root, "tracked.txt", "source\n")
	writeScopeFile(t, root, "private/unknown.txt", "unknown\n")
	addSnapshotGitPaths(t, root, "tracked.txt")
	private := filepath.Join(root, "private")
	if err := os.Chmod(private, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(private, 0o755) })

	if _, err := validation.SnapshotScope(root, []string{"."}); err == nil {
		t.Fatal("unreadable untracked directory was silently omitted")
	}
}

func TestSnapshotScopeGitWorktreeIgnoresUnreadableDirectoryOutsideNarrowScope(t *testing.T) {
	root := t.TempDir()
	initSnapshotGitRepository(t, root)
	writeScopeFile(t, root, "tests/unit/a.go", "package unit\n")
	writeScopeFile(t, root, "private/unknown.txt", "unknown\n")
	addSnapshotGitPaths(t, root, "tests/unit/a.go")
	private := filepath.Join(root, "private")
	if err := os.Chmod(private, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(private, 0o755) })

	snapshot, err := validation.SnapshotScope(root, []string{"tests/**/*.go"})
	if err != nil {
		t.Fatalf("out-of-scope unreadable directory affected Git snapshot: %v", err)
	}
	want := []string{filepath.Join(root, "tests", "unit", "a.go")}
	if !reflect.DeepEqual(snapshot.Scope, want) {
		t.Fatalf("scope = %#v, want %#v", snapshot.Scope, want)
	}
}

func TestSnapshotScopeGitWorktreeIgnoresUnreadableTrackedSiblingOutsideScope(t *testing.T) {
	root := t.TempDir()
	initSnapshotGitRepository(t, root)
	writeScopeFile(t, root, "tests/unit/a.go", "package unit\n")
	writeScopeFile(t, root, "private/tracked.go", "package private\n")
	addSnapshotGitPaths(t, root, "tests/unit/a.go", "private/tracked.go")
	private := filepath.Join(root, "private")
	if err := os.Chmod(private, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(private, 0o755) })

	snapshot, err := validation.SnapshotScope(root, []string{"tests/**/*.go"})
	if err != nil {
		t.Fatalf("out-of-scope unreadable tracked directory affected Git snapshot: %v", err)
	}
	want := []string{filepath.Join(root, "tests", "unit", "a.go")}
	if !reflect.DeepEqual(snapshot.Scope, want) {
		t.Fatalf("scope = %#v, want %#v", snapshot.Scope, want)
	}
}

func TestSnapshotScopeGitWorktreePrunesUnreadableUnmatchedGlobSibling(t *testing.T) {
	root := t.TempDir()
	initSnapshotGitRepository(t, root)
	writeScopeFile(t, root, "tests/unit/a.go", "package unit\n")
	writeScopeFile(t, root, "tests/private/unknown.go", "package private\n")
	addSnapshotGitPaths(t, root, "tests/unit/a.go")
	private := filepath.Join(root, "tests", "private")
	if err := os.Chmod(private, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(private, 0o755) })

	snapshot, err := validation.SnapshotScope(root, []string{"tests/unit*/*.go"})
	if err != nil {
		t.Fatalf("unmatched unreadable glob sibling affected Git snapshot: %v", err)
	}
	want := []string{filepath.Join(root, "tests", "unit", "a.go")}
	if !reflect.DeepEqual(snapshot.Scope, want) {
		t.Fatalf("scope = %#v, want %#v", snapshot.Scope, want)
	}
}

func TestSnapshotScopeGitWorktreePrunesUnreadableTrackedUnmatchedGlobSibling(t *testing.T) {
	root := t.TempDir()
	initSnapshotGitRepository(t, root)
	writeScopeFile(t, root, "tests/unit/a.go", "package unit\n")
	writeScopeFile(t, root, "tests/private/tracked.go", "package private\n")
	addSnapshotGitPaths(t, root, "tests/unit/a.go", "tests/private/tracked.go")
	private := filepath.Join(root, "tests", "private")
	if err := os.Chmod(private, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(private, 0o755) })

	snapshot, err := validation.SnapshotScope(root, []string{"tests/unit*/*.go"})
	if err != nil {
		t.Fatalf("unmatched unreadable tracked glob sibling affected Git snapshot: %v", err)
	}
	want := []string{filepath.Join(root, "tests", "unit", "a.go")}
	if !reflect.DeepEqual(snapshot.Scope, want) {
		t.Fatalf("scope = %#v, want %#v", snapshot.Scope, want)
	}
}

func TestSnapshotScopeSupportsLinkedGitWorktree(t *testing.T) {
	primary := t.TempDir()
	initSnapshotGitRepository(t, primary)
	writeScopeFile(t, primary, ".gitignore", ".remember/\n")
	writeScopeFile(t, primary, "tracked.txt", "first\n")
	addSnapshotGitPaths(t, primary, ".gitignore", "tracked.txt")
	for _, args := range [][]string{
		{"-c", "user.name=Mindrail Tests", "-c", "user.email=mindrail@example.invalid", "commit", "--quiet", "-m", "fixture"},
		{"branch", "linked-fixture"},
	} {
		command := exec.Command("git", args...)
		command.Dir = primary
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, output)
		}
	}

	linked := filepath.Join(t.TempDir(), "linked")
	command := exec.Command("git", "worktree", "add", "--quiet", linked, "linked-fixture")
	command.Dir = primary
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git worktree add: %v: %s", err, output)
	}
	t.Cleanup(func() {
		cleanup := exec.Command("git", "worktree", "remove", "--force", linked)
		cleanup.Dir = primary
		_ = cleanup.Run()
	})
	writeScopeFile(t, linked, ".remember/tmp/hook", "first\n")

	first, err := validation.SnapshotScope(linked, []string{"."})
	if err != nil {
		t.Fatal(err)
	}
	writeScopeFile(t, linked, ".remember/tmp/hook", "changed\n")
	second, err := validation.SnapshotScope(linked, []string{"."})
	if err != nil {
		t.Fatal(err)
	}
	if second.Hash != first.Hash {
		t.Fatal("ignored output moved linked-worktree snapshot hash")
	}
	writeScopeFile(t, linked, "tracked.txt", "changed\n")
	third, err := validation.SnapshotScope(linked, []string{"."})
	if err != nil {
		t.Fatal(err)
	}
	if third.Hash == second.Hash {
		t.Fatal("tracked change did not move linked-worktree snapshot hash")
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
