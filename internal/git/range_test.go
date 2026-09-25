package git

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestResolveRevKnownSHA pins the resolution contract: a known rev yields
// 40 hex, and every later range command consumes the SHA, not the rev.
func TestResolveRevKnownSHA(t *testing.T) {
	repo := newRepoFixture(t)
	ctx := fixtureContext(t)
	runner := NewExecRunner()

	sha, err := ResolveRev(ctx, runner, repo, "HEAD")
	if err != nil {
		t.Fatalf("ResolveRev HEAD: %v", err)
	}
	if len(sha) != 40 {
		t.Fatalf("SHA = %q, want 40 hex", sha)
	}
	if _, err := ResolveRev(ctx, runner, repo, sha); err != nil {
		t.Fatalf("ResolveRev SHA: %v", err)
	}
}

// TestResolveRevUnknownRefuses pins the never-empty-green rule: an unknown
// rev is an error naming the rev, not an empty answer.
func TestResolveRevUnknownRefuses(t *testing.T) {
	repo := newRepoFixture(t)
	ctx := fixtureContext(t)

	if _, err := ResolveRev(ctx, NewExecRunner(), repo, "no-such-rev"); err == nil {
		t.Fatal("unknown rev resolved")
	} else if !strings.Contains(err.Error(), "no-such-rev") {
		t.Fatalf("error names no rev: %v", err)
	}
}

// TestDefaultBaseChain pins decision D-223: the fixture's main branch
// resolves through the chain without flags.
func TestDefaultBaseChain(t *testing.T) {
	repo := newRepoFixture(t)
	ctx := fixtureContext(t)

	sha, err := DefaultBase(ctx, NewExecRunner(), repo)
	if err != nil {
		t.Fatalf("DefaultBase: %v", err)
	}
	if len(sha) != 40 {
		t.Fatalf("SHA = %q, want 40 hex", sha)
	}
}

// TestDefaultBaseAbsentRefuses pins the structured refusal: a repo with no
// candidate branch names the tried list instead of judging nothing.
func TestDefaultBaseAbsentRefuses(t *testing.T) {
	repo := newRepoFixture(t)
	ctx := fixtureContext(t)
	runGit(t, repo, "branch", "-m", "lonely")

	if _, err := DefaultBase(ctx, NewExecRunner(), repo); err == nil {
		t.Fatal("default base resolved with no candidate branch")
	} else if !strings.Contains(err.Error(), "origin/main") {
		t.Fatalf("error names no candidates: %v", err)
	}
}

// TestMergeBaseForkPoint pins the fork-point contract over a real branch.
func TestMergeBaseForkPoint(t *testing.T) {
	repo := newRepoFixture(t)
	ctx := fixtureContext(t)
	runner := NewExecRunner()

	writeFile := func(rel, content string) {
		t.Helper()
		abs := filepath.Join(repo, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	writeFile("a.txt", "base\n")
	runGit(t, repo, "add", "a.txt")
	runGit(t, repo, "commit", "--quiet", "-m", "base file")
	baseSHA, err := ResolveRev(ctx, runner, repo, "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "checkout", "--quiet", "-b", "feature")
	writeFile("b.txt", "head\n")
	runGit(t, repo, "add", "b.txt")
	runGit(t, repo, "commit", "--quiet", "-m", "head file")
	headSHA, err := ResolveRev(ctx, runner, repo, "HEAD")
	if err != nil {
		t.Fatal(err)
	}

	mergeBase, err := MergeBase(ctx, runner, repo, baseSHA, headSHA)
	if err != nil {
		t.Fatalf("MergeBase: %v", err)
	}
	if mergeBase != baseSHA {
		t.Fatalf("merge-base = %q, want base %q", mergeBase, baseSHA)
	}

	entries, err := RangeEntries(ctx, runner, repo, mergeBase, headSHA)
	if err != nil {
		t.Fatalf("RangeEntries: %v", err)
	}
	if len(entries) != 1 || entries[0].Path != "b.txt" {
		t.Fatalf("entries = %+v, want exactly b.txt", entries)
	}

	// ShowRev reads committed bytes on both sides: head has b.txt, the
	// merge-base does not.
	if _, found, err := ShowRev(ctx, runner, repo, headSHA, "b.txt"); err != nil || !found {
		t.Fatalf("head bytes missing: found=%v err=%v", found, err)
	}
	if _, found, err := ShowRev(ctx, runner, repo, mergeBase, "b.txt"); err != nil || found {
		t.Fatalf("base unexpectedly has b.txt: found=%v err=%v", found, err)
	}
}

// TestMergeBaseUnrelatedRefuses pins the no-common-ancestor refusal: two
// roots have no range, and that is an error, never an empty diff.
func TestMergeBaseUnrelatedRefuses(t *testing.T) {
	repo := newRepoFixture(t)
	ctx := fixtureContext(t)
	runner := NewExecRunner()

	rootSHA, err := ResolveRev(ctx, runner, repo, "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "checkout", "--quiet", "--orphan", "other")
	runGit(t, repo, "commit", "--quiet", "--allow-empty", "-m", "other root")
	otherSHA, err := ResolveRev(ctx, runner, repo, "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := MergeBase(ctx, runner, repo, rootSHA, otherSHA); err == nil {
		t.Fatal("unrelated histories merged")
	}
}
