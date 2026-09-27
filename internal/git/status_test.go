package git_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/PsyChaos/mindrail/internal/git"
)

func statusRunner(t *testing.T, stdout string, err error) *git.FakeRunner {
	t.Helper()
	return &git.FakeRunner{
		Responses: map[string]git.FakeResponse{
			"status --porcelain=v1 -z --untracked-files=normal -- .": {Stdout: stdout, Err: err},
		},
	}
}

// TestStatusEntriesParsesKinds pins the porcelain grammar the discovery
// layer stands on: unstaged and staged modifies, untracked adds, staged
// renames (new path first, source second), and deletes.
func TestStatusEntriesParsesKinds(t *testing.T) {
	out := " M b.py\x00M  staged.py\x00MM both.py\x00R  c.py\x00a.py\x00?? d.py\x00 D gone.py\x00" + "C  copy.py\x00src.py\x00T  mode.py\x00UU conflict.py\x00"
	entries, err := git.StatusEntries(context.Background(), statusRunner(t, out, nil), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	want := []git.StatusEntry{
		{X: ' ', Y: 'M', Path: "b.py"},
		{X: 'M', Y: ' ', Path: "staged.py"},
		{X: 'M', Y: 'M', Path: "both.py"},
		{X: 'R', Y: ' ', Path: "c.py", OrigPath: "a.py"},
		{X: '?', Y: '?', Path: "d.py"},
		{X: ' ', Y: 'D', Path: "gone.py"},
		{X: 'C', Y: ' ', Path: "copy.py", OrigPath: "src.py"},
		{X: 'T', Y: ' ', Path: "mode.py"},
		{X: 'U', Y: 'U', Path: "conflict.py"},
	}
	if len(entries) != len(want) {
		t.Fatalf("entries = %+v, want %+v", entries, want)
	}
	for i := range want {
		if entries[i] != want[i] {
			t.Fatalf("entry %d = %+v, want %+v", i, entries[i], want[i])
		}
	}
}

// TestStatusEntriesRefusesMalformedRows pins fail-closed parsing: a row that
// does not fit the grammar fails the discovery instead of joining it
// half-read, and a git that cannot answer is an error, never an empty diff.
func TestStatusEntriesRefusesMalformedRows(t *testing.T) {
	for _, out := range []string{"M", "M b.py", "R  c.py\x00", "R  \x00a.py\x00"} {
		if _, err := git.StatusEntries(context.Background(), statusRunner(t, out, nil), t.TempDir()); err == nil {
			t.Fatalf("malformed %q accepted", out)
		}
	}
	if _, err := git.StatusEntries(context.Background(), statusRunner(t, "err", errors.New("exit status 128")), t.TempDir()); err == nil {
		t.Fatal("failing git accepted as empty diff")
	}
}

// TestStatusEntriesReadsARealWorktree proves the spelling against real git:
// unstaged edits, staged renames and untracked files surface together.
func TestStatusEntriesReadsARealWorktree(t *testing.T) {
	repo := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null",
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-q")
	if err := os.WriteFile(filepath.Join(repo, "a.py"), []byte("x = 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "-A")
	run("commit", "-qm", "base")
	if err := os.WriteFile(filepath.Join(repo, "a.py"), []byte("x = 2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "new.py"), []byte("y = 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	entries, err := git.StatusEntries(context.Background(), &git.ExecRunner{}, repo)
	if err != nil {
		t.Fatal(err)
	}
	byPath := map[string]git.StatusEntry{}
	for _, entry := range entries {
		byPath[entry.Path] = entry
	}
	if byPath["a.py"].Y != 'M' {
		t.Fatalf("a.py = %+v, want unstaged modify", byPath["a.py"])
	}
	if got := byPath["new.py"]; got.X != '?' || got.Y != '?' {
		t.Fatalf("new.py = %+v, want untracked", got)
	}
}
