package git_test

import (
	"context"
	"errors"
	"testing"

	"github.com/PsyChaos/mindrail/internal/git"
)

func TestShowStagedTreatsDeletedIndexPathAsAbsent(t *testing.T) {
	runner := &git.FakeRunner{Responses: map[string]git.FakeResponse{
		"show :deleted.txt": {
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
		"show :tracked.txt": {
			Stderr: "fatal: cannot read object database\n",
			Err:    errors.New("exit status 128"),
		},
	}}

	if _, _, err := git.ShowStaged(context.Background(), runner, t.TempDir(), "tracked.txt"); err == nil {
		t.Fatal("unrelated git failure accepted as an absent path")
	}
}
