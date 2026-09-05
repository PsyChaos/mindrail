package git

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PsyChaos/mindrail/internal/app"
)

// The repositories in this file are the ones finding D6 is about: every one of
// them keeps its Git directory somewhere other than `<worktree>/.git`, and the
// classifier used to describe all of them as "inside the repository's .git
// directory" and send the reader to "the directory that contains this .git" —
// a directory that is either the one they are standing in or unrelated to the
// repository. The remedy could not succeed, so the reader looped.
//
// git itself is not confused by any of these, which is what makes them
// unambiguous rather than hard: a submodule's Git directory answers
// `git rev-parse --show-toplevel` with the real worktree.

// TestResolveInsideSubmoduleGitDirectoryNamesItsWorktree is the headline case.
// A submodule's Git directory lives at `<super>/.git/modules/<name>` and sets
// core.worktree; the worktree it serves is nowhere near the Git directory's
// parent.
func TestResolveInsideSubmoduleGitDirectoryNamesItsWorktree(t *testing.T) {
	super, submodule := newSubmoduleFixture(t)
	submoduleGitDir := filepath.Join(super, ".git", "modules", "sub")

	adapter := NewAdapter(NewExecRunner())

	for _, startDir := range []string{submoduleGitDir, filepath.Join(submoduleGitDir, "refs")} {
		t.Run(filepath.Base(startDir), func(t *testing.T) {
			_, err := adapter.Resolve(fixtureContext(t), startDir)
			if err == nil {
				t.Fatalf("Resolve accepted %q, which is a Git directory rather than a worktree", startDir)
			}
			if !errors.Is(err, ErrInsideGitDirectory) {
				t.Fatalf("Resolve error = %v, want ErrInsideGitDirectory", err)
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

			if !sameFile(t, payload.Metadata["worktree_root"], submodule) {
				t.Fatalf("worktree_root metadata = %q, want the submodule's worktree %q", payload.Metadata["worktree_root"], submodule)
			}
			// The sentence that used to be printed here was false: this Git
			// directory is not named .git and its parent holds no worktree.
			if strings.Contains(payload.Why, "the directory that contains this .git") {
				t.Errorf("why = %q, which describes a directory that is not the worktree", payload.Why)
			}
			assertRemedyDestinationsAreReal(t, startDir, payload)
			assertRemedyLeadsToAWorktree(t, adapter, payload)
		})
	}
}

// TestResolveInsideSeparateGitDirectoryDoesNotInventAWorktree covers the shape
// where nothing can name the worktree: measured against git 2.55, neither
// `git init --separate-git-dir` nor `git clone --separate-git-dir` records
// core.worktree, and the Git directory holds no back-pointer of any kind.
//
// The failure to guard against is the previous one — naming the Git
// directory's parent, which is not the worktree and in which `mindrail` would
// fail again.
func TestResolveInsideSeparateGitDirectoryDoesNotInventAWorktree(t *testing.T) {
	worktree, gitDir := newSeparateGitDirFixture(t)

	_, err := NewAdapter(NewExecRunner()).Resolve(fixtureContext(t), gitDir)
	if err == nil {
		t.Fatalf("Resolve accepted the separate Git directory %q", gitDir)
	}
	if !errors.Is(err, ErrInsideGitDirectory) {
		t.Fatalf("Resolve error = %v, want ErrInsideGitDirectory", err)
	}

	payload, ok := app.PayloadOf(err)
	if !ok {
		t.Fatalf("error carries no domain payload: %v", err)
	}
	assertPayloadIsActionable(t, payload)
	assertRemedyDestinationsAreReal(t, gitDir, payload)

	// The parent of a separate Git directory is not the worktree. Naming it is
	// exactly the loop this finding is about, and the reader standing in the Git
	// directory can reach the parent with `cd ..` and get nowhere.
	parent := filepath.Dir(gitDir)
	for _, dest := range changeIntoDestinations(payload) {
		if sameFile(t, dest, parent) || sameFile(t, dest, gitDir) {
			t.Errorf("next_action sends the reader to %q, which is not a worktree of this repository", dest)
		}
	}
	// What it can honestly say is where the worktree's .git file has to point,
	// which is enough to find it.
	if !strings.Contains(strings.Join(payload.NextAction, "\n"), gitDir) {
		t.Errorf("next_action %q never names the Git directory the worktree points at", payload.NextAction)
	}

	// And the fixture proves such a worktree exists and works, so the remedy is
	// reachable rather than theoretical.
	if _, err := NewAdapter(NewExecRunner()).Resolve(fixtureContext(t), worktree); err != nil {
		t.Fatalf("Resolve the separate-git-dir worktree %q: %v", worktree, err)
	}
}

// TestResolveInsideGitDirectoryWithHandSetCoreWorktree covers core.worktree set
// by hand. git answers --show-toplevel from inside the Git directory, so the
// remedy can name the worktree exactly — and the Git directory's own parent is
// not it.
func TestResolveInsideGitDirectoryWithHandSetCoreWorktree(t *testing.T) {
	worktree, gitDir := newSeparateGitDirFixture(t)
	runGit(t, gitDir, "config", "core.worktree", worktree)

	adapter := NewAdapter(NewExecRunner())

	_, err := adapter.Resolve(fixtureContext(t), gitDir)
	if err == nil {
		t.Fatalf("Resolve accepted the Git directory %q", gitDir)
	}
	if !errors.Is(err, ErrInsideGitDirectory) {
		t.Fatalf("Resolve error = %v, want ErrInsideGitDirectory", err)
	}

	payload, ok := app.PayloadOf(err)
	if !ok {
		t.Fatalf("error carries no domain payload: %v", err)
	}
	assertPayloadIsActionable(t, payload)
	if !sameFile(t, payload.Metadata["worktree_root"], worktree) {
		t.Fatalf("worktree_root metadata = %q, want %q", payload.Metadata["worktree_root"], worktree)
	}
	if sameFile(t, payload.Metadata["worktree_root"], filepath.Dir(gitDir)) {
		t.Fatalf("worktree_root metadata = %q, which is only the Git directory's parent", payload.Metadata["worktree_root"])
	}
	assertRemedyDestinationsAreReal(t, gitDir, payload)
	assertRemedyLeadsToAWorktree(t, adapter, payload)
}

// TestResolveRefusesToSendTheReaderToAMissingWorktree covers a core.worktree
// that names a directory nobody can enter. "Change into <path>" is not a remedy
// when <path> is gone — it is the same loop with a longer stride — so no such
// remedy may be printed.
func TestResolveRefusesToSendTheReaderToAMissingWorktree(t *testing.T) {
	repo := newRepoFixture(t)
	gitDir := filepath.Join(repo, ".git")
	missing := filepath.Join(t.TempDir(), "worktree-that-is-not-there")
	runGit(t, repo, "config", "core.worktree", missing)

	_, err := NewAdapter(NewExecRunner()).Resolve(fixtureContext(t), gitDir)
	if err == nil {
		t.Fatalf("Resolve accepted a Git directory whose worktree does not exist")
	}
	if !errors.Is(err, ErrInsideGitDirectory) {
		t.Fatalf("Resolve error = %v, want ErrInsideGitDirectory", err)
	}

	payload, ok := app.PayloadOf(err)
	if !ok {
		t.Fatalf("error carries no domain payload: %v", err)
	}
	assertPayloadIsActionable(t, payload)
	// assertRemedyDestinationsAreReal would fail on a "change into <missing>";
	// this is the case that must not produce one at all.
	assertRemedyDestinationsAreReal(t, gitDir, payload)
	if !strings.Contains(payload.Why, missing) {
		t.Errorf("why = %q, which never names the worktree that is missing", payload.Why)
	}
	if payload.Metadata["worktree_root"] != missing {
		t.Errorf("worktree_root metadata = %q, want %q", payload.Metadata["worktree_root"], missing)
	}
}

// TestResolveRepairsAWorktreeThatDoesNotPointBack is the remedy carried out.
//
// core.worktree is a one-way link: it lets the Git directory find the worktree,
// and only a `.git` entry in the worktree lets discovery go the other way. With
// that entry missing, "change into <worktree>" earns the reader
// NOT_A_GIT_REPOSITORY — a second wrong remedy rather than the first one fixed.
//
// The test runs the command the remedy prints, verbatim, and requires that the
// condition is then gone.
func TestResolveRepairsAWorktreeThatDoesNotPointBack(t *testing.T) {
	repo := newRepoFixture(t)
	gitDir := filepath.Join(filepath.Dir(repo), "detached-git-dir")
	if err := os.Rename(filepath.Join(repo, ".git"), gitDir); err != nil {
		t.Fatalf("detach the Git directory: %v", err)
	}
	runGit(t, gitDir, "config", "core.worktree", repo)

	adapter := NewAdapter(NewExecRunner())

	_, err := adapter.Resolve(fixtureContext(t), gitDir)
	if err == nil {
		t.Fatalf("Resolve accepted a Git directory whose worktree does not point back")
	}
	payload, ok := app.PayloadOf(err)
	if !ok {
		t.Fatalf("error carries no domain payload: %v", err)
	}
	assertPayloadIsActionable(t, payload)

	// The worktree is real and enterable, so the previous remedy would have sent
	// the reader there — where git finds nothing.
	if _, statErr := os.Stat(repo); statErr != nil {
		t.Fatalf("the fixture worktree is not there: %v", statErr)
	}
	for _, dest := range changeIntoDestinations(payload) {
		t.Errorf("next_action sends the reader to %q, where git finds no repository", dest)
	}

	wantCommand := "`git init --separate-git-dir=" + gitDir + " " + repo + "`"
	if !strings.Contains(payload.NextAction[0], wantCommand) {
		t.Fatalf("next_action[0] = %q, want it to contain %s", payload.NextAction[0], wantCommand)
	}
	if payload.Metadata["git_dir"] != gitDir {
		t.Errorf("git_dir metadata = %q, want %q", payload.Metadata["git_dir"], gitDir)
	}

	// Carry the printed remedy out.
	runGit(t, "", "init", "--separate-git-dir="+gitDir, repo)

	repaired, err := adapter.Resolve(fixtureContext(t), repo)
	if err != nil {
		t.Fatalf("the printed remedy did not clear the condition: Resolve %q: %v", repo, err)
	}
	if !sameFile(t, repaired.WorktreeRoot, repo) {
		t.Fatalf("after the remedy, WorktreeRoot = %q, want %q", repaired.WorktreeRoot, repo)
	}
	if !sameFile(t, repaired.CommonDir, gitDir) {
		t.Fatalf("after the remedy, CommonDir = %q, want the same Git directory %q", repaired.CommonDir, gitDir)
	}

	// The remedy names `git init`, so it has to be shown not to have created a
	// second repository inside the Git directory — the hazard the .git-directory
	// branch exists to avoid.
	if _, statErr := os.Stat(filepath.Join(gitDir, ".git")); statErr == nil {
		t.Fatalf("the remedy nested a repository inside %q", gitDir)
	}
	after, err := adapter.Resolve(fixtureContext(t), gitDir)
	if err == nil {
		t.Fatalf("after the remedy, the Git directory %q resolved as a worktree: %+v", gitDir, after)
	}
	if !errors.Is(err, ErrInsideGitDirectory) {
		t.Fatalf("after the remedy, Resolve %q = %v, want ErrInsideGitDirectory", gitDir, err)
	}
}

// TestGitDirectoryClassificationDoesNotOverFire is the guard the new detection
// needs in both directions.
//
// The first half proves a healthy repository is untouched by it, in every shape
// MR-001 supports. The second half proves the two adjacent conditions this
// detection is *not* about — a bare repository and a directory outside every
// repository — still get their own classifications rather than being swept into
// the Git-directory one.
func TestGitDirectoryClassificationDoesNotOverFire(t *testing.T) {
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
		if errors.Is(err, ErrInsideGitDirectory) {
			t.Fatalf("a bare repository was swept into the Git-directory classification: %v", err)
		}
		payload, _ := app.PayloadOf(err)
		if payload.Code != app.CodeBareRepository {
			t.Fatalf("code = %q, want %q", payload.Code, app.CodeBareRepository)
		}
	})

	t.Run("a directory outside every repository is still that", func(t *testing.T) {
		outside := t.TempDir()
		requireOutsideAnyRepository(t, outside)

		_, err := adapter.Resolve(fixtureContext(t), outside)
		if !errors.Is(err, ErrNotARepository) {
			t.Fatalf("Resolve error = %v, want ErrNotARepository", err)
		}
		if errors.Is(err, ErrInsideGitDirectory) {
			t.Fatalf("an empty directory was reported as being inside a Git directory: %v", err)
		}
		payload, _ := app.PayloadOf(err)
		// `git init` is right here and only here, so it must survive.
		if !strings.Contains(strings.Join(payload.NextAction, "\n"), "git init") {
			t.Fatalf("next_action %q lost the one remedy that fits an absent repository", payload.NextAction)
		}
	})

	t.Run("an ordinary .git directory keeps its own message", func(t *testing.T) {
		gitDir := filepath.Join(repo, ".git")

		_, err := adapter.Resolve(fixtureContext(t), gitDir)
		if !errors.Is(err, ErrInsideGitDirectory) {
			t.Fatalf("Resolve error = %v, want ErrInsideGitDirectory", err)
		}
		payload, _ := app.PayloadOf(err)
		if !strings.Contains(payload.Why, ".git directory") {
			t.Errorf("why = %q, want it to still say this is the repository's .git directory", payload.Why)
		}
		if !sameFile(t, payload.Metadata["worktree_root"], repo) {
			t.Errorf("worktree_root metadata = %q, want %q", payload.Metadata["worktree_root"], repo)
		}
		for _, action := range payload.NextAction {
			if strings.Contains(action, "git init") {
				t.Errorf("next_action %q would create a repository inside .git", action)
			}
		}
		assertRemedyDestinationsAreReal(t, gitDir, payload)
		assertRemedyLeadsToAWorktree(t, adapter, payload)
	})
}

// assertRemedyLeadsToAWorktree runs the remedy rather than reading it: every
// destination it names must be a directory the adapter resolves successfully.
// A remedy that does not clear the condition is not a remedy.
func assertRemedyLeadsToAWorktree(t *testing.T, adapter *Adapter, payload app.ErrorPayload) {
	t.Helper()

	destinations := changeIntoDestinations(payload)
	if len(destinations) == 0 {
		t.Fatalf("payload %q offers no directory to change into: %q", payload.Code, payload.NextAction)
	}
	for _, dest := range destinations {
		if _, err := adapter.Resolve(fixtureContext(t), dest); err != nil {
			t.Errorf("following the remedy into %q fails again: %v", dest, err)
		}
	}
}

// newSubmoduleFixture builds a superproject with one submodule and returns both
// worktree roots. `protocol.file.allow` is forced because git refuses local
// submodule transport by default since 2.38.
func newSubmoduleFixture(t *testing.T) (super, submodule string) {
	t.Helper()

	source := newRepoFixture(t)
	super = filepath.Join(t.TempDir(), "super")
	if err := os.MkdirAll(super, 0o700); err != nil {
		t.Fatalf("create superproject directory: %v", err)
	}
	runGit(t, "", "init", "--quiet", "--initial-branch=main", super)
	runGit(t, super, "config", "user.email", "fixture@example.invalid")
	runGit(t, super, "config", "user.name", "Mindrail Fixture")
	runGit(t, super, "config", "commit.gpgsign", "false")
	runGit(t, super, "-c", "protocol.file.allow=always", "submodule", "add", "--quiet", source, "sub")

	submodule = filepath.Join(super, "sub")
	if _, err := os.Stat(filepath.Join(super, ".git", "modules", "sub")); err != nil {
		t.Skipf("this git does not keep submodule Git directories under .git/modules: %v", err)
	}
	return super, submodule
}

// newSeparateGitDirFixture builds a repository whose Git directory is outside
// the worktree and whose worktree holds a `.git` file pointing at it.
func newSeparateGitDirFixture(t *testing.T) (worktree, gitDir string) {
	t.Helper()
	requireGit(t)
	isolateGitConfig(t)

	root := t.TempDir()
	worktree = filepath.Join(root, "checkout")
	gitDir = filepath.Join(root, "elsewhere", "repo.git")
	if err := os.MkdirAll(filepath.Dir(gitDir), 0o700); err != nil {
		t.Fatalf("create the Git directory's parent: %v", err)
	}

	runGit(t, "", "init", "--quiet", "--separate-git-dir="+gitDir, worktree)
	runGit(t, worktree, "config", "user.email", "fixture@example.invalid")
	runGit(t, worktree, "config", "user.name", "Mindrail Fixture")

	if out, err := exec.Command("git", "--git-dir="+gitDir, "config", "--get", "core.worktree").Output(); err == nil && strings.TrimSpace(string(out)) != "" {
		t.Skipf("this git records core.worktree for --separate-git-dir (%q), so the fixture cannot stand for the no-back-pointer case", strings.TrimSpace(string(out)))
	}
	return worktree, gitDir
}
