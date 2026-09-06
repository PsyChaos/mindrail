package git

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PsyChaos/mindrail/internal/app"
)

// TestStandpointIsDecidedByIdentityRatherThanSpelling is finding F10.
//
// sameDirectory compares two paths with os.Stat and os.SameFile after the fast
// string-equality path, and its comment says that is what keeps "a repository
// reached through a symlink — or a /tmp that is really /private/tmp" from
// reading as a different place. Nothing exercised it: reducing the function to
// `left == right` passed the entire suite and the smoke tests, while changing
// which of two sentences a reader gets and dropping the sentinel a caller
// branches on.
//
// The shape below is the smallest one where the two answers differ. git reports
// its own paths resolved, and the reader arrives through a link, so the strings
// do not match and the inodes do — which is the whole question the function
// exists to answer.
func TestStandpointIsDecidedByIdentityRatherThanSpelling(t *testing.T) {
	repo := newRepoFixture(t)
	elsewhere := filepath.Join(t.TempDir(), "elsewhere")
	if err := os.MkdirAll(elsewhere, 0o700); err != nil {
		t.Fatalf("create the configured working tree: %v", err)
	}
	runGit(t, repo, "config", "core.worktree", elsewhere)

	link := filepath.Join(filepath.Dir(repo), "repolink")
	if err := os.Symlink(repo, link); err != nil {
		t.Skipf("this filesystem does not support symlinks: %v", err)
	}

	// The reader is inside the Git directory, reached through the link.
	startDir := filepath.Join(link, ".git")
	_, err := NewAdapter(NewExecRunner()).Resolve(fixtureContext(t), startDir)
	if err == nil {
		t.Fatalf("Resolve accepted %q, whose repository's working tree is elsewhere", startDir)
	}
	if !errors.Is(err, ErrNotARepository) {
		t.Fatalf("Resolve error = %v, want ErrNotARepository", err)
	}

	// The sentinel is the machine-readable half, and it is the one a caller
	// branching on "am I standing in a Git directory?" stops matching when the
	// identity comparison is reduced to string equality.
	if !errors.Is(err, ErrInsideGitDirectory) {
		t.Fatalf("Resolve %q = %v, want ErrInsideGitDirectory; the reader is inside %q reached through a symlink",
			startDir, err, filepath.Join(repo, ".git"))
	}

	payload, ok := app.PayloadOf(err)
	if !ok {
		t.Fatalf("error carries no domain payload: %v", err)
	}
	assertPayloadIsActionable(t, payload)
	if !strings.Contains(payload.Why, "inside a Git directory") {
		t.Errorf("why = %q, which tells a reader standing in a Git directory that they are not", payload.Why)
	}
	if payload.Impact != gitDirectoryImpact {
		t.Errorf("impact = %q, want the Git-directory impact", payload.Impact)
	}
}

// TestStandpointIdentityDoesNotOverFire is the other direction: two directories
// that really are different must keep reading as different, or the identity
// comparison has been widened into one that answers "yes" everywhere.
func TestStandpointIdentityDoesNotOverFire(t *testing.T) {
	repo := newRepoFixture(t)
	elsewhere := filepath.Join(t.TempDir(), "elsewhere")
	if err := os.MkdirAll(elsewhere, 0o700); err != nil {
		t.Fatalf("create the configured working tree: %v", err)
	}
	runGit(t, repo, "config", "core.worktree", elsewhere)

	// Standing in the checkout root rather than in the Git directory: the same
	// repository, a genuinely different place, and the sentence has to say so.
	_, err := NewAdapter(NewExecRunner()).Resolve(fixtureContext(t), repo)
	if !errors.Is(err, ErrNotARepository) {
		t.Fatalf("Resolve %q = %v, want ErrNotARepository", repo, err)
	}
	if errors.Is(err, ErrInsideGitDirectory) {
		t.Fatalf("Resolve %q reports ErrInsideGitDirectory, but %q is an ordinary checkout root", repo, repo)
	}
}

// TestLinkedWorktreeRecordIsOnlyBelievedWhenItPointsBack is finding F11.
//
// linkedWorktreeOfAdminDir requires that the worktree root recorded in
// `<common>/worktrees/<name>/gitdir` has a `.git` pointing back at that same
// administrative directory before the E9 message names it. Deleting that check
// passed the whole gate, and made `mindrail` inside the administrative directory
// direct the reader into a directory belonging to a different repository.
//
// The two rows are the two ways the back-pointer can fail to agree: it names
// somewhere else, and it is not there at all. The row where the recorded root
// has been removed entirely is in TestLinkedWorktreeAdminDetectionDoesNotOverFire;
// it stops one check earlier and so cannot cover these.
func TestLinkedWorktreeRecordIsOnlyBelievedWhenItPointsBack(t *testing.T) {
	tests := []struct {
		name string
		// breakBackPointer leaves the recorded worktree root on disk and makes
		// its `.git` disagree with the record that names it.
		breakBackPointer func(t *testing.T, linked, otherGitDir string)
	}{
		{
			name: "the recorded worktree belongs to a different repository",
			breakBackPointer: func(t *testing.T, linked, otherGitDir string) {
				t.Helper()
				if err := os.RemoveAll(filepath.Join(linked, ".git")); err != nil {
					t.Fatalf("clear the back-pointer: %v", err)
				}
				writeGitFile(t, linked, otherGitDir)
			},
		},
		{
			name: "the recorded worktree has no .git at all",
			breakBackPointer: func(t *testing.T, linked, _ string) {
				t.Helper()
				if err := os.RemoveAll(filepath.Join(linked, ".git")); err != nil {
					t.Fatalf("remove the back-pointer: %v", err)
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			main := newRepoFixture(t)
			linked := filepath.Join(filepath.Dir(main), "lw")
			runGit(t, main, "worktree", "add", "--detach", "--quiet", linked)

			adminDir := filepath.Join(main, ".git", "worktrees", "lw")
			if _, err := os.Stat(adminDir); err != nil {
				t.Skipf("this git does not keep linked worktree records at %q: %v", adminDir, err)
			}

			other := newRepoFixture(t)
			tc.breakBackPointer(t, linked, filepath.Join(other, ".git"))

			_, err := NewAdapter(NewExecRunner()).Resolve(fixtureContext(t), adminDir)
			if !errors.Is(err, ErrInsideGitDirectory) {
				t.Fatalf("Resolve %q = %v, want ErrInsideGitDirectory", adminDir, err)
			}
			if errors.Is(err, ErrLinkedWorktreeAdminDir) {
				t.Fatalf("Resolve %q described %q as this repository's linked working tree, "+
					"but its .git no longer points back at %q: %v", adminDir, linked, adminDir, err)
			}

			payload, ok := app.PayloadOf(err)
			if !ok {
				t.Fatalf("error carries no domain payload: %v", err)
			}
			assertPayloadIsActionable(t, payload)
			assertRemedyDestinationsAreReal(t, adminDir, payload)

			// The generic classification names the main worktree, which is the
			// one directory here that this Git directory really does administer.
			if !sameFile(t, payload.Metadata["worktree_root"], main) {
				t.Fatalf("worktree_root metadata = %q, want the main worktree %q",
					payload.Metadata["worktree_root"], main)
			}
			for _, dest := range changeIntoDestinations(payload) {
				if sameFile(t, dest, linked) {
					t.Errorf("next_action sends the reader into %q, which is no longer this repository's worktree", dest)
				}
			}
		})
	}
}

// TestWorktreeHoldingGitDirAcceptsASeparateGitDirectoryNamedGit pins the claim
// the guard removed by finding F16 used to make and never checked.
//
// `git init --separate-git-dir=<parent>/.git <worktree>` produces a Git
// directory named `.git` whose parent is not the working tree it was created
// for — the shape the old os.SameFile guard existed to reject. It does not
// reject it, and it cannot: when the base name is `.git`, the path the guard
// stats and the path it compares against are the same string. git itself
// resolves that parent as a worktree of that Git directory, so agreeing with it
// is correct, and the test is here so the agreement is verified rather than
// assumed.
func TestWorktreeHoldingGitDirAcceptsASeparateGitDirectoryNamedGit(t *testing.T) {
	root := t.TempDir()
	parent := filepath.Join(root, "srv", "x")
	worktree := filepath.Join(root, "realwt")
	gitDir := filepath.Join(parent, ".git")

	if err := os.MkdirAll(parent, 0o700); err != nil {
		t.Fatalf("create %q: %v", parent, err)
	}
	runGit(t, "", "init", "--quiet", "--separate-git-dir="+gitDir, worktree)

	if got := worktreeHoldingGitDir(gitDir); !sameFile(t, got, parent) {
		t.Errorf("worktreeHoldingGitDir(%q) = %q, want %q", gitDir, got, parent)
	}

	// The two shapes it must still refuse: a Git directory not named `.git`, and
	// a parent that cannot be entered.
	notDotGit := filepath.Join(root, "elsewhere.git")
	if err := os.MkdirAll(notDotGit, 0o700); err != nil {
		t.Fatalf("create %q: %v", notDotGit, err)
	}
	if got := worktreeHoldingGitDir(notDotGit); got != "" {
		t.Errorf("worktreeHoldingGitDir(%q) = %q, want no worktree; it is not named .git", notDotGit, got)
	}
	if got := worktreeHoldingGitDir(""); got != "" {
		t.Errorf("worktreeHoldingGitDir(\"\") = %q, want no worktree", got)
	}
}
