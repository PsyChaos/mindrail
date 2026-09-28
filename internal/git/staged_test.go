package git_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/PsyChaos/mindrail/internal/git"
)

func TestShowStagedTreatsDeletedIndexPathAsAbsent(t *testing.T) {
	runner := &git.FakeRunner{Responses: map[string]git.FakeResponse{
		"cat-file blob :./deleted.txt": {
			Stderr: "fatal: ambiguous argument ':deleted.txt': unknown revision or path not in the working tree.\n",
			Err:    errors.New("exit status 128"),
		},
	}}

	content, exists, err := git.ShowStaged(context.Background(), runner, t.TempDir(), "deleted.txt")
	if err != nil {
		t.Fatal(err)
	}
	if exists || content != nil {
		t.Fatalf("deleted staged path = (%q, %v), want absent", content, exists)
	}
}

func TestShowStagedStillRejectsUnrelatedGitFailure(t *testing.T) {
	runner := &git.FakeRunner{Responses: map[string]git.FakeResponse{
		"cat-file blob :./tracked.txt": {
			Stderr: "fatal: cannot read object database\n",
			Err:    errors.New("exit status 128"),
		},
	}}

	if _, _, err := git.ShowStaged(context.Background(), runner, t.TempDir(), "tracked.txt"); err == nil {
		t.Fatal("unrelated git failure accepted as an absent path")
	}
}

func TestShowHEADTreatsMissingBracketPathAsAbsentInsteadOfCommitText(t *testing.T) {
	root := t.TempDir()
	runner := git.NewExecRunner()
	for _, args := range [][]string{{"init", "-q"}, {"config", "user.email", "test@example.com"}, {"config", "user.name", "Test"}} {
		if _, stderr, err := runner.Run(t.Context(), root, args...); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, stderr)
		}
	}
	readme := filepath.Join(root, "README.md")
	if err := os.WriteFile(readme, []byte("seed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"add", "README.md"}, {"commit", "-qm", "seed"}} {
		if _, stderr, err := runner.Run(t.Context(), root, args...); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, stderr)
		}
	}
	content, exists, err := git.ShowHEAD(t.Context(), runner, root, "app/(console)/[org]/error.tsx")
	if err != nil {
		t.Fatal(err)
	}
	if exists || content != nil {
		t.Fatalf("missing bracket path returned commit bytes: exists=%v content=%q", exists, content)
	}
}
