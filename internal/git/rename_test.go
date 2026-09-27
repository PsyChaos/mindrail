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

func renameRunner(t *testing.T, stdout string, err error) *git.FakeRunner {
	t.Helper()
	return &git.FakeRunner{
		Responses: map[string]git.FakeResponse{
			"diff --no-color --name-status -z -M HEAD -- .": {Stdout: stdout, Err: err},
		},
	}
}

// TestDiffRenamesParsesREntries pins the corroboration spelling: R rows become
// entries, everything else is skipped, and a truncated tail is dropped.
func TestDiffRenamesParsesREntries(t *testing.T) {
	out := "R100\x00old/a.py\x00new/a.py\x00M\x00touched.py\x00A\x00added.py\x00R09\x00"
	entries, err := git.DiffRenames(context.Background(), renameRunner(t, out, nil), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].OldPath != "old/a.py" || entries[0].NewPath != "new/a.py" {
		t.Fatalf("entries = %+v", entries)
	}
}

// TestDiffRenamesFailureIsEmptySignal is decision D-101: a git that cannot
// answer is a missing signal, never an error, and matching proceeds.
func TestDiffRenamesFailureIsEmptySignal(t *testing.T) {
	for _, tc := range []struct {
		name   string
		runner *git.FakeRunner
	}{
		{"command error", renameRunner(t, "", errors.New("exit status 128"))},
		{"empty output", renameRunner(t, "", nil)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			entries, err := git.DiffRenames(context.Background(), tc.runner, t.TempDir())
			if err != nil || len(entries) != 0 {
				t.Fatalf("entries = %+v, err = %v", entries, err)
			}
		})
	}
	if _, err := git.DiffRenames(context.Background(), nil, t.TempDir()); err == nil {
		t.Fatal("nil runner accepted")
	}
	if _, err := git.DiffRenames(context.Background(), renameRunner(t, "", nil), ""); err == nil {
		t.Fatal("empty directory accepted")
	}
}

// TestDiffRenamesReadsARealStagedRename proves the spelling against real git:
// a staged move surfaces as one R entry with both paths.
func TestDiffRenamesReadsARealStagedRename(t *testing.T) {
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
	run("config", "user.email", "t@t")
	run("config", "user.name", "t")
	if err := os.WriteFile(filepath.Join(repo, "a.py"), []byte("x = 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// A non-ASCII name proves -z disables pathname munging without quotepath.
	unicode := "\u00fcn\u00efcode.py"
	if err := os.WriteFile(filepath.Join(repo, unicode), []byte("y = 2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "-A")
	run("commit", "-qm", "base")
	if err := os.Rename(filepath.Join(repo, "a.py"), filepath.Join(repo, "b.py")); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(repo, unicode), filepath.Join(repo, "renamed.py")); err != nil {
		t.Fatal(err)
	}
	run("add", "-A")
	entries, err := git.DiffRenames(context.Background(), &git.ExecRunner{}, repo)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("entries = %+v, want both moves", entries)
	}
	byNew := map[string]string{}
	for _, entry := range entries {
		byNew[entry.NewPath] = entry.OldPath
	}
	if byNew["b.py"] != "a.py" || byNew["renamed.py"] != unicode {
		t.Fatalf("entries = %+v, want raw unquoted paths", entries)
	}
}
