package git

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/PsyChaos/mindrail/internal/app"
)

// The repositories in this file are the ones finding E6 is about: a
// `core.worktree` that names a directory belonging to a *different* repository.
//
// That shape reached unlinkedWorktreeError, whose two remedies both dead-end on
// it — one is a command git refuses, the other names a working tree that does
// not exist — and whose reason ("git finds no repository there") is false about
// a directory that is a repository. Finding E8 is that no test ever built the
// shape, which is why the branch could stay wrong through six audits.

// TestResolveForeignWorktreeAtAnotherRepositoryRoot is the headline case: the
// configured working tree is another repository's own root.
//
// It reproduces the dead end first — the command the old remedy printed is run
// here and required to fail — and then carries out the printed replacement and
// requires the condition to be gone.
func TestResolveForeignWorktreeAtAnotherRepositoryRoot(t *testing.T) {
	repo := newRepoFixture(t)
	other := newRepoFixture(t)
	// A tracked file, so the remedy's last step has something to restore and the
	// claim that it leaves a usable checkout is testable rather than vacuous.
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("content\n"), 0o600); err != nil {
		t.Fatalf("write the tracked file: %v", err)
	}
	runGit(t, repo, "add", "tracked.txt")
	runGit(t, repo, "commit", "--quiet", "-m", "add tracked file")
	gitDir := detachGitDir(t, repo)
	runGit(t, gitDir, "config", "core.worktree", other)

	adapter := NewAdapter(NewExecRunner())

	_, err := adapter.Resolve(fixtureContext(t), gitDir)
	if err == nil {
		t.Fatalf("Resolve accepted a Git directory whose working tree belongs to another repository")
	}
	if !errors.Is(err, ErrForeignWorktree) {
		t.Fatalf("Resolve error = %v, want ErrForeignWorktree", err)
	}
	if !errors.Is(err, ErrNotARepository) {
		t.Fatalf("Resolve error = %v, want it to still unwrap to ErrNotARepository", err)
	}

	payload, ok := app.PayloadOf(err)
	if !ok {
		t.Fatalf("error carries no domain payload: %v", err)
	}
	if payload.Code != app.CodeNotAGitRepository {
		t.Fatalf("code = %q, want %q (decision D-18 keeps the code stable)", payload.Code, app.CodeNotAGitRepository)
	}
	if got := app.ExitCode(err); got != app.ExitUsage {
		t.Fatalf("ExitCode = %d, want %d (usage, decision D-03)", got, app.ExitUsage)
	}
	assertPayloadIsActionable(t, payload)

	// The false reason. `other` is a repository, so "git finds no repository
	// there" was wrong about it, and the sentence has to name what is true
	// instead: which repository the configured directory belongs to.
	if strings.Contains(payload.Why, "git finds no repository there") {
		t.Errorf("why = %q, which is false: %q is a repository", payload.Why, other)
	}
	otherCommonDir := filepath.Join(other, ".git")
	if !strings.Contains(payload.Why, otherCommonDir) {
		t.Errorf("why = %q, which never names the repository %q belongs to", payload.Why, other)
	}
	if !sameFile(t, payload.Metadata["other_common_dir"], otherCommonDir) {
		t.Errorf("other_common_dir metadata = %q, want %q", payload.Metadata["other_common_dir"], otherCommonDir)
	}
	if payload.Metadata["git_dir"] != gitDir {
		t.Errorf("git_dir metadata = %q, want %q", payload.Metadata["git_dir"], gitDir)
	}
	assertRemedyDestinationsAreReal(t, gitDir, payload)

	// The dead end, reproduced. The old first remedy was this exact command.
	out, relinkErr := runGitAllowingFailure(t, "", "init", "--separate-git-dir="+gitDir, other)
	if relinkErr == nil {
		t.Fatalf("`git init --separate-git-dir=%s %s` succeeded; the premise of this test no longer holds:\n%s", gitDir, other, out)
	}
	t.Logf("the remedy this replaces still fails: %v\n%s", relinkErr, out)
	if !strings.Contains(strings.Join(payload.NextAction, "\n"), "cannot re-link them") {
		t.Errorf("next_action %q never says why the obvious command does not apply", payload.NextAction)
	}

	// The old second remedy named a working tree whose .git reads
	// `gitdir: <gitDir>`. No such directory exists, or discovery would have found
	// it, so no remedy may send the reader looking for one.
	if strings.Contains(strings.Join(payload.NextAction, "\n"), "already reads `gitdir:") {
		t.Errorf("next_action %q sends the reader to a working tree that does not exist", payload.NextAction)
	}

	// The printed remedy, carried out verbatim.
	fresh := filepath.Join(t.TempDir(), "own-worktree")
	assertRemedyMentionsAll(t, payload, "`git init --separate-git-dir="+gitDir+" <path>`",
		"`git --git-dir="+gitDir+" config core.worktree <path>`",
		"`git -C <path> checkout --force HEAD`")
	runGit(t, "", "init", "--separate-git-dir="+gitDir, fresh)
	runGit(t, "", "--git-dir="+gitDir, "config", "core.worktree", fresh)
	runGit(t, fresh, "checkout", "--force", "HEAD")

	repaired, err := adapter.Resolve(fixtureContext(t), fresh)
	if err != nil {
		t.Fatalf("the printed remedy did not clear the condition: Resolve %q: %v", fresh, err)
	}
	if !sameFile(t, repaired.CommonDir, gitDir) {
		t.Fatalf("after the remedy, CommonDir = %q, want this repository's Git directory %q", repaired.CommonDir, gitDir)
	}
	if !sameFile(t, repaired.WorktreeRoot, fresh) {
		t.Fatalf("after the remedy, WorktreeRoot = %q, want %q", repaired.WorktreeRoot, fresh)
	}
	// A working tree with none of the repository's files in it would resolve and
	// still be useless, so the remedy has to leave the checkout populated.
	restored, err := os.ReadFile(filepath.Join(fresh, "tracked.txt"))
	if err != nil {
		t.Fatalf("the remedy left the working tree without the repository's files: %v", err)
	}
	if string(restored) != "content\n" {
		t.Fatalf("restored file = %q, want %q", restored, "content\n")
	}

	// And the other repository is untouched by any of it.
	untouched, err := adapter.Resolve(fixtureContext(t), other)
	if err != nil {
		t.Fatalf("the other repository stopped resolving: %v", err)
	}
	if !sameFile(t, untouched.CommonDir, otherCommonDir) {
		t.Fatalf("the other repository's CommonDir = %q, want %q", untouched.CommonDir, otherCommonDir)
	}
}

// TestResolveForeignWorktreeInsideAnotherRepository is the second shape, and
// the more dangerous of the two: `git init --separate-git-dir` *succeeds* on a
// directory that merely sits inside another repository's working tree, by
// carving a nested repository out of that checkout. A remedy that exits 0 while
// doing that is worse than one that refuses, so the message has to say what it
// would do.
func TestResolveForeignWorktreeInsideAnotherRepository(t *testing.T) {
	repo := newRepoFixture(t)
	other := newRepoFixture(t)
	inner := filepath.Join(other, "nested", "dir")
	if err := os.MkdirAll(inner, 0o700); err != nil {
		t.Fatalf("create the nested directory: %v", err)
	}
	gitDir := detachGitDir(t, repo)
	runGit(t, gitDir, "config", "core.worktree", inner)

	adapter := NewAdapter(NewExecRunner())

	_, err := adapter.Resolve(fixtureContext(t), gitDir)
	if !errors.Is(err, ErrForeignWorktree) {
		t.Fatalf("Resolve error = %v, want ErrForeignWorktree", err)
	}
	payload, ok := app.PayloadOf(err)
	if !ok {
		t.Fatalf("error carries no domain payload: %v", err)
	}
	assertPayloadIsActionable(t, payload)
	assertRemedyDestinationsAreReal(t, gitDir, payload)

	if !sameFile(t, payload.Metadata["other_worktree_root"], other) {
		t.Errorf("other_worktree_root metadata = %q, want the enclosing repository %q", payload.Metadata["other_worktree_root"], other)
	}
	actions := strings.Join(payload.NextAction, "\n")
	if !strings.Contains(actions, "carves a nested repository") {
		t.Errorf("next_action %q never says what the obvious command would do here", payload.NextAction)
	}
	if strings.Contains(actions, "cannot re-link them") {
		t.Errorf("next_action %q claims git would refuse, which it does not for this shape", payload.NextAction)
	}

	// The destination the remedy offers as an alternative is the enclosing
	// repository, and it has to be one the adapter resolves.
	destinations := changeIntoDestinations(payload)
	if len(destinations) == 0 {
		t.Fatalf("next_action %q offers nowhere to go", payload.NextAction)
	}
	for _, dest := range destinations {
		resolved, err := adapter.Resolve(fixtureContext(t), dest)
		if err != nil {
			t.Errorf("following the remedy into %q fails again: %v", dest, err)
			continue
		}
		if !sameFile(t, resolved.WorktreeRoot, other) {
			t.Errorf("the remedy's destination %q resolves to %q, not the repository the message named", dest, resolved.WorktreeRoot)
		}
	}
}

// TestForeignWorktreeDetectionDoesNotOverFire is the guard the new detection
// needs in both directions.
//
// The first half proves every healthy shape MR-001 supports is untouched. The
// second half proves the adjacent condition — a `core.worktree` naming a real
// directory that holds no repository at all — still gets the linking remedy,
// which is correct for it and only for it.
func TestForeignWorktreeDetectionDoesNotOverFire(t *testing.T) {
	repo := newRepoFixture(t)
	nested := filepath.Join(repo, "a", "b")
	if err := os.MkdirAll(nested, 0o700); err != nil {
		t.Fatalf("create nested directory: %v", err)
	}
	linked := filepath.Join(filepath.Dir(repo), "linked")
	runGit(t, repo, "worktree", "add", "--detach", linked)
	symlinked := filepath.Join(filepath.Dir(repo), "via-symlink")
	if err := os.Symlink(repo, symlinked); err != nil {
		t.Skipf("this platform cannot create a directory symlink: %v", err)
	}

	adapter := NewAdapter(NewExecRunner())

	t.Run("healthy shapes still resolve", func(t *testing.T) {
		for name, dir := range map[string]string{
			"repository root":     repo,
			"nested subdirectory": nested,
			"linked worktree":     linked,
			"symlinked root":      symlinked,
		} {
			got, err := adapter.Resolve(fixtureContext(t), dir)
			if err != nil {
				t.Errorf("Resolve the %s (%q): %v", name, dir, err)
				continue
			}
			if got.WorktreeRoot == "" {
				t.Errorf("Resolve the %s: WorktreeRoot is empty", name)
			}
		}
	})

	t.Run("a bare repository is still a bare repository", func(t *testing.T) {
		bare := filepath.Join(t.TempDir(), "bare.git")
		runGit(t, "", "init", "--quiet", "--bare", bare)

		_, err := adapter.Resolve(fixtureContext(t), bare)
		if !errors.Is(err, ErrBareRepository) {
			t.Fatalf("Resolve error = %v, want ErrBareRepository", err)
		}
		if errors.Is(err, ErrForeignWorktree) {
			t.Fatalf("a bare repository was swept into the foreign-worktree classification: %v", err)
		}
	})

	t.Run("a directory outside every repository is still that", func(t *testing.T) {
		outside := t.TempDir()
		requireOutsideAnyRepository(t, outside)

		_, err := adapter.Resolve(fixtureContext(t), outside)
		if !errors.Is(err, ErrNotARepository) {
			t.Fatalf("Resolve error = %v, want ErrNotARepository", err)
		}
		if errors.Is(err, ErrForeignWorktree) || errors.Is(err, ErrInsideGitDirectory) {
			t.Fatalf("an empty directory was misclassified: %v", err)
		}
	})

	t.Run("a real .git directory keeps its own message", func(t *testing.T) {
		gitDir := filepath.Join(repo, ".git")

		_, err := adapter.Resolve(fixtureContext(t), gitDir)
		if !errors.Is(err, ErrInsideGitDirectory) {
			t.Fatalf("Resolve error = %v, want ErrInsideGitDirectory", err)
		}
		if errors.Is(err, ErrForeignWorktree) {
			t.Fatalf("an ordinary .git directory was reported as a foreign worktree: %v", err)
		}
		payload, _ := app.PayloadOf(err)
		if !strings.Contains(payload.Why, ".git directory") {
			t.Errorf("why = %q, want it to still say this is the repository's .git directory", payload.Why)
		}
	})

	t.Run("a separate Git directory still names no worktree", func(t *testing.T) {
		_, gitDir := newSeparateGitDirFixture(t)

		_, err := adapter.Resolve(fixtureContext(t), gitDir)
		if !errors.Is(err, ErrInsideGitDirectory) {
			t.Fatalf("Resolve error = %v, want ErrInsideGitDirectory", err)
		}
		if errors.Is(err, ErrForeignWorktree) {
			t.Fatalf("a --separate-git-dir repository was reported as a foreign worktree: %v", err)
		}
	})

	t.Run("a submodule Git directory still names its own worktree", func(t *testing.T) {
		super, submodule := newSubmoduleFixture(t)

		_, err := adapter.Resolve(fixtureContext(t), filepath.Join(super, ".git", "modules", "sub"))
		if !errors.Is(err, ErrInsideGitDirectory) {
			t.Fatalf("Resolve error = %v, want ErrInsideGitDirectory", err)
		}
		if errors.Is(err, ErrForeignWorktree) {
			t.Fatalf("a submodule Git directory was reported as a foreign worktree: %v", err)
		}
		payload, _ := app.PayloadOf(err)
		if !sameFile(t, payload.Metadata["worktree_root"], submodule) {
			t.Fatalf("worktree_root metadata = %q, want the submodule's worktree %q", payload.Metadata["worktree_root"], submodule)
		}
	})

	// The adjacent condition. A configured working tree that is a real directory
	// with no repository in it is *not* foreign, and the linking remedy is right
	// for it — the one this whole finding is about not printing anywhere else.
	t.Run("a working tree with no repository in it still gets the linking remedy", func(t *testing.T) {
		unlinked := newRepoFixture(t)
		gitDir := detachGitDir(t, unlinked)
		runGit(t, gitDir, "config", "core.worktree", unlinked)

		_, err := adapter.Resolve(fixtureContext(t), gitDir)
		if errors.Is(err, ErrForeignWorktree) {
			t.Fatalf("a working tree holding no repository was called foreign: %v", err)
		}
		payload, ok := app.PayloadOf(err)
		if !ok {
			t.Fatalf("error carries no domain payload: %v", err)
		}
		want := "`git init --separate-git-dir=" + gitDir + " " + unlinked + "`"
		if !strings.Contains(payload.NextAction[0], want) {
			t.Fatalf("next_action[0] = %q, want it to contain %s", payload.NextAction[0], want)
		}

		// Carried out, because that is the only proof the split kept the correct
		// branch correct.
		runGit(t, "", "init", "--separate-git-dir="+gitDir, unlinked)
		repaired, err := adapter.Resolve(fixtureContext(t), unlinked)
		if err != nil {
			t.Fatalf("the printed remedy did not clear the condition: %v", err)
		}
		if !sameFile(t, repaired.CommonDir, gitDir) {
			t.Fatalf("after the remedy, CommonDir = %q, want %q", repaired.CommonDir, gitDir)
		}
	})
}

// TestResolveOutsideAConfiguredWorkingTreeDescribesWhereTheReaderIs is finding
// E7. A repository whose config redirects `core.worktree` elsewhere reports
// "not inside a work tree" from its own checkout root — an ordinary directory
// holding an ordinary `.git`, where the reader is not inside a Git directory at
// all and was told they were.
func TestResolveOutsideAConfiguredWorkingTreeDescribesWhereTheReaderIs(t *testing.T) {
	repo := newRepoFixture(t)
	elsewhere := filepath.Join(t.TempDir(), "elsewhere")
	if err := os.MkdirAll(elsewhere, 0o700); err != nil {
		t.Fatalf("create the configured working tree: %v", err)
	}
	subdir := filepath.Join(repo, "src")
	if err := os.MkdirAll(subdir, 0o700); err != nil {
		t.Fatalf("create the subdirectory: %v", err)
	}
	runGit(t, repo, "config", "core.worktree", elsewhere)

	adapter := NewAdapter(NewExecRunner())

	for _, startDir := range []string{repo, subdir} {
		t.Run(filepath.Base(startDir), func(t *testing.T) {
			_, err := adapter.Resolve(fixtureContext(t), startDir)
			if err == nil {
				t.Fatalf("Resolve accepted %q, whose repository's working tree is elsewhere", startDir)
			}
			if !errors.Is(err, ErrNotARepository) {
				t.Fatalf("Resolve error = %v, want ErrNotARepository", err)
			}
			// The reader is standing in their checkout, not in a Git directory.
			// The sentinel says "inside a git directory rather than a worktree",
			// so claiming it here would make it true of everything.
			if errors.Is(err, ErrInsideGitDirectory) {
				t.Fatalf("Resolve %q reports ErrInsideGitDirectory, but %q is not inside a Git directory", startDir, startDir)
			}

			payload, ok := app.PayloadOf(err)
			if !ok {
				t.Fatalf("error carries no domain payload: %v", err)
			}
			assertPayloadIsActionable(t, payload)
			if strings.Contains(payload.Why, "inside a Git directory") {
				t.Errorf("why = %q, which describes a place the reader is not", payload.Why)
			}
			if !strings.Contains(payload.Why, elsewhere) {
				t.Errorf("why = %q, which never names the working tree this repository is configured to use", payload.Why)
			}
			if payload.Impact != outsideWorktreeImpact {
				t.Errorf("impact = %q, want the outside-the-working-tree impact", payload.Impact)
			}
			if strings.Contains(payload.Impact, "administrative directory") {
				t.Errorf("impact = %q, which warns about nesting a repository inside a directory the reader is not in", payload.Impact)
			}
		})
	}

	// The remedy, carried out.
	gitDir := filepath.Join(repo, ".git")
	runGit(t, "", "init", "--separate-git-dir="+gitDir, elsewhere)
	repaired, err := adapter.Resolve(fixtureContext(t), elsewhere)
	if err != nil {
		t.Fatalf("the printed remedy did not clear the condition: Resolve %q: %v", elsewhere, err)
	}
	if !sameFile(t, repaired.WorktreeRoot, elsewhere) {
		t.Fatalf("after the remedy, WorktreeRoot = %q, want %q", repaired.WorktreeRoot, elsewhere)
	}
}

// TestStandpointStillReportsAGitDirectoryAsOne is the over-fire guard for the
// wording change above: the shapes where the reader really is inside a Git
// directory must keep saying so, or E7's fix has simply moved the wrong
// sentence to a different set of readers.
func TestStandpointStillReportsAGitDirectoryAsOne(t *testing.T) {
	repo := newRepoFixture(t)
	elsewhere := filepath.Join(t.TempDir(), "elsewhere")
	if err := os.MkdirAll(elsewhere, 0o700); err != nil {
		t.Fatalf("create the configured working tree: %v", err)
	}
	runGit(t, repo, "config", "core.worktree", elsewhere)

	gitDir := filepath.Join(repo, ".git")
	for _, startDir := range []string{gitDir, filepath.Join(gitDir, "refs")} {
		t.Run(filepath.Base(startDir), func(t *testing.T) {
			_, err := NewAdapter(NewExecRunner()).Resolve(fixtureContext(t), startDir)
			if !errors.Is(err, ErrInsideGitDirectory) {
				t.Fatalf("Resolve %q = %v, want ErrInsideGitDirectory", startDir, err)
			}
			payload, _ := app.PayloadOf(err)
			if !strings.Contains(payload.Why, "inside a Git directory") {
				t.Errorf("why = %q, want it to still say the reader is inside a Git directory", payload.Why)
			}
			if payload.Impact != gitDirectoryImpact {
				t.Errorf("impact = %q, want the Git-directory impact", payload.Impact)
			}
		})
	}
}

// TestResolveInsideALinkedWorktreeAdminDirectoryNamesThatWorktree is finding
// E9. `<common-dir>/worktrees/<name>` answers --show-toplevel with a refusal and
// --git-common-dir with the *main* repository's `.git`, so the generic
// classification described the main worktree — true of the repository, and not
// where the reader is or what they were working on.
func TestResolveInsideALinkedWorktreeAdminDirectoryNamesThatWorktree(t *testing.T) {
	repo := newRepoFixture(t)
	linked := filepath.Join(filepath.Dir(repo), "linked")
	runGit(t, repo, "worktree", "add", "--detach", "--quiet", linked)
	adminDir := filepath.Join(repo, ".git", "worktrees", "linked")
	if _, err := os.Stat(adminDir); err != nil {
		t.Skipf("this git does not keep linked worktree records at %q: %v", adminDir, err)
	}

	adapter := NewAdapter(NewExecRunner())

	for _, startDir := range []string{adminDir, filepath.Join(adminDir, "refs")} {
		t.Run(filepath.Base(startDir), func(t *testing.T) {
			if err := os.MkdirAll(startDir, 0o700); err != nil {
				t.Fatalf("create %q: %v", startDir, err)
			}

			_, err := adapter.Resolve(fixtureContext(t), startDir)
			if err == nil {
				t.Fatalf("Resolve accepted %q, which is an administrative directory", startDir)
			}
			if !errors.Is(err, ErrLinkedWorktreeAdminDir) {
				t.Fatalf("Resolve error = %v, want ErrLinkedWorktreeAdminDir", err)
			}
			if !errors.Is(err, ErrInsideGitDirectory) || !errors.Is(err, ErrNotARepository) {
				t.Fatalf("Resolve error = %v, want it to still unwrap to the broader sentinels", err)
			}

			payload, ok := app.PayloadOf(err)
			if !ok {
				t.Fatalf("error carries no domain payload: %v", err)
			}
			if payload.Code != app.CodeNotAGitRepository {
				t.Fatalf("code = %q, want %q", payload.Code, app.CodeNotAGitRepository)
			}
			assertPayloadIsActionable(t, payload)
			assertRemedyDestinationsAreReal(t, startDir, payload)

			// The whole finding: the paths reported have to be this worktree's.
			if !sameFile(t, payload.Metadata["worktree_root"], linked) {
				t.Fatalf("worktree_root metadata = %q, want the linked worktree %q", payload.Metadata["worktree_root"], linked)
			}
			if sameFile(t, payload.Metadata["worktree_root"], repo) {
				t.Fatalf("worktree_root metadata = %q, which is the main worktree, not the one this directory administers", payload.Metadata["worktree_root"])
			}
			if !strings.Contains(payload.Why, linked) {
				t.Errorf("why = %q, which never names the linked working tree", payload.Why)
			}

			// And the remedy, carried out: the destination has to be the linked
			// worktree, whose Git directory is the one the reader was standing in.
			destinations := changeIntoDestinations(payload)
			if len(destinations) == 0 {
				t.Fatalf("next_action %q offers nowhere to go", payload.NextAction)
			}
			for _, dest := range destinations {
				resolved, err := adapter.Resolve(fixtureContext(t), dest)
				if err != nil {
					t.Errorf("following the remedy into %q fails again: %v", dest, err)
					continue
				}
				if !sameFile(t, resolved.GitDir, adminDir) {
					t.Errorf("the remedy's destination %q has GitDir %q, want the administrative directory %q", dest, resolved.GitDir, adminDir)
				}
				if !resolved.IsLinkedWorktree {
					t.Errorf("the remedy's destination %q did not resolve as a linked worktree", dest)
				}
			}
		})
	}
}

// TestLinkedWorktreeAdminDetectionDoesNotOverFire keeps the new detection off
// every directory that is not one of these, including the adjacent shape it
// most resembles: a record whose worktree has been moved away, where naming the
// recorded path would send the reader somewhere that is not there.
func TestLinkedWorktreeAdminDetectionDoesNotOverFire(t *testing.T) {
	repo := newRepoFixture(t)
	linked := filepath.Join(filepath.Dir(repo), "linked")
	runGit(t, repo, "worktree", "add", "--detach", "--quiet", linked)
	gitDir := filepath.Join(repo, ".git")

	adapter := NewAdapter(NewExecRunner())

	t.Run("healthy shapes are unaffected", func(t *testing.T) {
		for name, dir := range map[string]string{
			"main worktree":   repo,
			"linked worktree": linked,
		} {
			if _, err := adapter.Resolve(fixtureContext(t), dir); err != nil {
				t.Errorf("Resolve the %s (%q): %v", name, dir, err)
			}
		}
	})

	t.Run("the main .git directory still names the main worktree", func(t *testing.T) {
		for _, startDir := range []string{gitDir, filepath.Join(gitDir, "refs"), filepath.Join(gitDir, "worktrees")} {
			_, err := adapter.Resolve(fixtureContext(t), startDir)
			if !errors.Is(err, ErrInsideGitDirectory) {
				t.Errorf("Resolve %q = %v, want ErrInsideGitDirectory", startDir, err)
				continue
			}
			if errors.Is(err, ErrLinkedWorktreeAdminDir) {
				t.Errorf("Resolve %q was classified as a linked worktree's administrative directory", startDir)
			}
			payload, _ := app.PayloadOf(err)
			if !sameFile(t, payload.Metadata["worktree_root"], repo) {
				t.Errorf("Resolve %q: worktree_root metadata = %q, want the main worktree %q", startDir, payload.Metadata["worktree_root"], repo)
			}
		}
	})

	// The adjacent condition: the record is still there and the worktree it
	// names is not. Naming a directory that has gone is the loop every one of
	// these findings is about, so the detection has to stand down.
	t.Run("a record whose worktree is gone falls back", func(t *testing.T) {
		gone := filepath.Join(filepath.Dir(repo), "gone")
		runGit(t, repo, "worktree", "add", "--detach", "--quiet", gone)
		adminDir := filepath.Join(gitDir, "worktrees", "gone")
		if err := os.RemoveAll(gone); err != nil {
			t.Fatalf("remove the worktree: %v", err)
		}

		_, err := adapter.Resolve(fixtureContext(t), adminDir)
		if !errors.Is(err, ErrInsideGitDirectory) {
			t.Fatalf("Resolve %q = %v, want ErrInsideGitDirectory", adminDir, err)
		}
		if errors.Is(err, ErrLinkedWorktreeAdminDir) {
			t.Fatalf("Resolve %q named a working tree that is not on disk: %v", adminDir, err)
		}
		payload, _ := app.PayloadOf(err)
		assertPayloadIsActionable(t, payload)
		assertRemedyDestinationsAreReal(t, adminDir, payload)
		for _, dest := range changeIntoDestinations(payload) {
			if sameFile(t, dest, gone) || dest == gone {
				t.Errorf("next_action sends the reader to %q, which has been removed", dest)
			}
		}
	})
}

// detachGitDir moves a repository's `.git` directory out of the worktree and
// returns its new path, leaving the worktree with no `.git` entry at all. It is
// how every fixture in this file produces a Git directory that has to be told
// where its working tree is.
func detachGitDir(t *testing.T, repo string) string {
	t.Helper()

	gitDir := filepath.Join(filepath.Dir(repo), filepath.Base(repo)+"-gitdir")
	if err := os.Rename(filepath.Join(repo, ".git"), gitDir); err != nil {
		t.Fatalf("detach the Git directory of %q: %v", repo, err)
	}
	return gitDir
}

// runGitAllowingFailure is runGit for the invocations a test needs to *fail*.
// Reproducing a dead end means running the command that dead-ends, and runGit
// would end the test on it.
func runGitAllowingFailure(t *testing.T, dir string, args ...string) (string, error) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "LC_ALL=C")

	out, err := cmd.CombinedOutput()
	return string(out), err
}

// assertRemedyMentionsAll checks that every command a multi-step remedy claims
// to print is actually in it, so the sequence a test then carries out is the
// sequence a reader would.
func assertRemedyMentionsAll(t *testing.T, payload app.ErrorPayload, fragments ...string) {
	t.Helper()

	actions := strings.Join(payload.NextAction, "\n")
	for _, fragment := range fragments {
		if !strings.Contains(actions, fragment) {
			t.Errorf("next_action %q does not contain %s", payload.NextAction, fragment)
		}
	}
}
