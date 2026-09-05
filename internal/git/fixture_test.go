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

func TestResolveRealRepository(t *testing.T) {
	repo := newRepoFixture(t)
	adapter := NewAdapter(NewExecRunner())

	got, err := adapter.Resolve(fixtureContext(t), repo)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	for label, value := range map[string]string{
		"CommonDir":    got.CommonDir,
		"GitDir":       got.GitDir,
		"WorktreeRoot": got.WorktreeRoot,
	} {
		if value == "" {
			t.Fatalf("%s is empty", label)
		}
		if !filepath.IsAbs(value) {
			t.Fatalf("%s = %q, which is not absolute", label, value)
		}
		if value != filepath.Clean(value) {
			t.Fatalf("%s = %q, which is not cleaned", label, value)
		}
	}

	if want := filepath.Join(repo, ".git"); !sameFile(t, got.CommonDir, want) {
		t.Fatalf("CommonDir = %q, want the repository's .git directory %q", got.CommonDir, want)
	}
	if !sameFile(t, got.GitDir, got.CommonDir) {
		t.Fatalf("GitDir = %q, want it equal to CommonDir %q in a single-worktree repository", got.GitDir, got.CommonDir)
	}
	if !sameFile(t, got.WorktreeRoot, repo) {
		t.Fatalf("WorktreeRoot = %q, want %q", got.WorktreeRoot, repo)
	}
	if got.IsLinkedWorktree {
		t.Errorf("IsLinkedWorktree = true for the main worktree")
	}
	if got.IsBare {
		t.Errorf("IsBare = true for a repository with a worktree")
	}

	version, err := adapter.Version(fixtureContext(t))
	if err != nil {
		t.Fatalf("Version: %v", err)
	}
	if !strings.HasPrefix(version, "git version ") {
		t.Fatalf("Version = %q, want the trimmed output of `git --version`", version)
	}
}

func TestResolveFromNestedSubdirectoryIsStable(t *testing.T) {
	repo := newRepoFixture(t)
	nested := filepath.Join(repo, "a", "b", "c")
	if err := os.MkdirAll(nested, 0o700); err != nil {
		t.Fatalf("create nested directory: %v", err)
	}

	adapter := NewAdapter(NewExecRunner())

	fromRoot, err := adapter.Resolve(fixtureContext(t), repo)
	if err != nil {
		t.Fatalf("Resolve from the repository root: %v", err)
	}
	fromNested, err := adapter.Resolve(fixtureContext(t), nested)
	if err != nil {
		t.Fatalf("Resolve from %q: %v", nested, err)
	}

	if fromNested != fromRoot {
		t.Fatalf("Resolve from a subdirectory returned %+v, want the byte-identical %+v", fromNested, fromRoot)
	}
}

func TestResolveLinkedWorktreeSharesCommonDir(t *testing.T) {
	repo := newRepoFixture(t)
	linked := filepath.Join(filepath.Dir(repo), "linked")
	runGit(t, repo, "worktree", "add", "--detach", linked)

	adapter := NewAdapter(NewExecRunner())

	main, err := adapter.Resolve(fixtureContext(t), repo)
	if err != nil {
		t.Fatalf("Resolve the main worktree: %v", err)
	}
	worktree, err := adapter.Resolve(fixtureContext(t), linked)
	if err != nil {
		t.Fatalf("Resolve the linked worktree: %v", err)
	}

	// The point of deriving runtime state from the common dir: every worktree
	// of one repository must land on one database (tech-stack §112).
	if worktree.CommonDir != main.CommonDir {
		t.Fatalf("CommonDir differs between worktrees: %q vs %q", worktree.CommonDir, main.CommonDir)
	}
	if strings.Contains(filepath.ToSlash(worktree.CommonDir), ".git/worktrees/") {
		t.Fatalf("CommonDir = %q, which is the per-worktree directory, not the shared one", worktree.CommonDir)
	}

	if worktree.WorktreeRoot == main.WorktreeRoot {
		t.Fatalf("WorktreeRoot is identical for two distinct worktrees: %q", worktree.WorktreeRoot)
	}
	if !sameFile(t, worktree.WorktreeRoot, linked) {
		t.Fatalf("WorktreeRoot = %q, want %q", worktree.WorktreeRoot, linked)
	}

	if worktree.GitDir == main.GitDir {
		t.Fatalf("GitDir is identical for two distinct worktrees: %q", worktree.GitDir)
	}
	if !strings.Contains(filepath.ToSlash(worktree.GitDir), "/worktrees/") {
		t.Fatalf("GitDir = %q, want the per-worktree directory under the common dir", worktree.GitDir)
	}

	if !worktree.IsLinkedWorktree {
		t.Errorf("IsLinkedWorktree = false for a linked worktree")
	}
	if main.IsLinkedWorktree {
		t.Errorf("IsLinkedWorktree = true for the main worktree")
	}
}

func TestResolveRejectsRealBareRepository(t *testing.T) {
	requireGit(t)

	bare := filepath.Join(t.TempDir(), "bare.git")
	runGit(t, "", "init", "--quiet", "--bare", bare)

	_, err := NewAdapter(NewExecRunner()).Resolve(fixtureContext(t), bare)
	if err == nil {
		t.Fatalf("Resolve accepted a bare repository")
	}
	if !errors.Is(err, ErrBareRepository) {
		t.Fatalf("Resolve error = %v, want ErrBareRepository", err)
	}
}

func TestResolveRejectsDirectoryOutsideAnyRepository(t *testing.T) {
	requireGit(t)

	// The directory has to be outside every repository in fact, not by an
	// environment variable: this package's whole job is to ignore the variables
	// that steer discovery, so a fixture that relies on one would be asserting
	// the leak it is supposed to prove closed.
	outside := t.TempDir()
	requireOutsideAnyRepository(t, outside)

	_, err := NewAdapter(NewExecRunner()).Resolve(fixtureContext(t), outside)
	if err == nil {
		t.Fatalf("Resolve accepted a directory outside any repository")
	}
	if !errors.Is(err, ErrNotARepository) {
		t.Fatalf("Resolve error = %v, want ErrNotARepository", err)
	}
	if errors.Is(err, ErrInsideGitDirectory) {
		t.Fatalf("a directory outside every repository was reported as being inside a .git directory: %v", err)
	}
}

// TestResolveIgnoresAmbientDiscoveryVariables is the end-to-end form of
// decision D-29: whatever the caller's environment says about which repository
// git should answer for, the answer is decided by the start directory alone.
// A pre-commit hook (MR-017) is exactly this situation.
func TestResolveIgnoresAmbientDiscoveryVariables(t *testing.T) {
	repo := newRepoFixture(t)
	nested := filepath.Join(repo, "a", "b")
	if err := os.MkdirAll(nested, 0o700); err != nil {
		t.Fatalf("create nested directory: %v", err)
	}
	// Ceiling entries are compared against the resolved path, so a symlinked
	// temp root (macOS /tmp) has to be resolved or git ignores the entry and
	// the test proves nothing.
	resolvedRepo, err := filepath.EvalSymlinks(repo)
	if err != nil {
		t.Fatalf("resolve the fixture path: %v", err)
	}
	elsewhere := t.TempDir()

	adapter := NewAdapter(NewExecRunner())
	want, err := adapter.Resolve(fixtureContext(t), nested)
	if err != nil {
		t.Fatalf("Resolve with a clean environment: %v", err)
	}

	tests := []struct {
		name string
		env  map[string]string
	}{
		{
			// The reproduction: a ceiling above the repository truncates the
			// upward walk and makes a valid subdirectory look like nowhere.
			name: "GIT_CEILING_DIRECTORIES",
			env:  map[string]string{"GIT_CEILING_DIRECTORIES": resolvedRepo},
		},
		{
			name: "GIT_DIR and GIT_WORK_TREE",
			env: map[string]string{
				"GIT_DIR":       filepath.Join(elsewhere, ".git"),
				"GIT_WORK_TREE": elsewhere,
			},
		},
		{
			// Finding F2: a malformed count makes git refuse to parse its
			// command line at all, so every invocation exits 128 and a valid
			// repository reports as no repository. Nothing about this value
			// mentions a repository; it still decides the answer.
			name: "GIT_CONFIG_COUNT is malformed",
			env:  map[string]string{"GIT_CONFIG_COUNT": "notanumber"},
		},
		{
			// The stale-hook shape: a count left behind without its pairs.
			name: "GIT_CONFIG_COUNT is stale",
			env:  map[string]string{"GIT_CONFIG_COUNT": "1"},
		},
		{
			name: "GIT_CONFIG pairs inject repository configuration",
			env: map[string]string{
				"GIT_CONFIG_COUNT":   "2",
				"GIT_CONFIG_KEY_0":   "core.worktree",
				"GIT_CONFIG_VALUE_0": elsewhere,
				"GIT_CONFIG_KEY_1":   "core.bare",
				"GIT_CONFIG_VALUE_1": "true",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for name, value := range tt.env {
				t.Setenv(name, value)
			}

			got, err := adapter.Resolve(fixtureContext(t), nested)
			if err != nil {
				t.Fatalf("Resolve with %s in the environment: %v", tt.name, err)
			}
			if got != want {
				t.Fatalf("Resolve with %s = %+v, want the ambient-free answer %+v", tt.name, got, want)
			}
		})
	}
}

func TestResolveInsideTheGitDirectoryDoesNotAdviseGitInit(t *testing.T) {
	repo := newRepoFixture(t)
	gitDir := filepath.Join(repo, ".git")

	_, err := NewAdapter(NewExecRunner()).Resolve(fixtureContext(t), gitDir)
	if err == nil {
		t.Fatalf("Resolve accepted %q, which is not a worktree", gitDir)
	}
	if !errors.Is(err, ErrInsideGitDirectory) {
		t.Fatalf("Resolve error = %v, want it to unwrap to ErrInsideGitDirectory", err)
	}

	payload, ok := app.PayloadOf(err)
	if !ok {
		t.Fatalf("error carries no domain payload: %v", err)
	}
	// Followed literally inside .git, `git init` creates a nested repository, so
	// no remedy offered here may name it.
	for _, action := range payload.NextAction {
		if strings.Contains(action, "git init") {
			t.Fatalf("next_action %q tells the user to create a repository inside .git", action)
		}
	}
	if len(payload.NextAction) == 0 {
		t.Fatalf("payload %q offers no remedy at all", payload.Code)
	}
	if payload.Metadata["start_dir"] == "" {
		t.Errorf("payload carries no start_dir; the reader cannot tell where the question was asked")
	}
}

// TestResolveOverFireGuard is the guard demanded after findings F1 and F2: the
// new classification and the wider environment sanitation must not turn any
// healthy repository shape into a failure.
//
// It runs the four shapes MR-001 has to support twice — once with a clean
// environment and once with the poison from F2 present — because a fix that
// works only when nothing is set is not a fix.
func TestResolveOverFireGuard(t *testing.T) {
	poison := map[string]string{
		"GIT_CONFIG_COUNT":   "notanumber",
		"GIT_CONFIG_KEY_0":   "core.bare",
		"GIT_CONFIG_VALUE_0": "true",
	}

	environments := []struct {
		name string
		env  map[string]string
	}{
		{name: "clean environment"},
		{name: "poisoned environment", env: poison},
	}

	for _, environment := range environments {
		t.Run(environment.name, func(t *testing.T) {
			repo := newRepoFixture(t)

			nested := filepath.Join(repo, "a", "b", "c")
			if err := os.MkdirAll(nested, 0o700); err != nil {
				t.Fatalf("create nested directory: %v", err)
			}

			linked := filepath.Join(filepath.Dir(repo), "linked")
			runGit(t, repo, "worktree", "add", "--detach", linked)

			// Decision D-32 keeps inside-root symlinks legal, and the macOS
			// /tmp fixture path is itself one, so a repository reached through a
			// symlink is an ordinary case rather than an exotic one.
			symlinked := filepath.Join(filepath.Dir(repo), "via-symlink")
			if err := os.Symlink(repo, symlinked); err != nil {
				t.Skipf("this platform cannot create a directory symlink: %v", err)
			}

			for name, value := range environment.env {
				t.Setenv(name, value)
			}

			adapter := NewAdapter(NewExecRunner())

			main, err := adapter.Resolve(fixtureContext(t), repo)
			if err != nil {
				t.Fatalf("Resolve the repository root: %v", err)
			}

			resolves := []struct {
				name string
				dir  string
				// sameRepository is false for the linked worktree, which is a
				// different worktree of the same repository.
				sameRepository bool
			}{
				{name: "nested subdirectory", dir: nested, sameRepository: true},
				{name: "through a symlink", dir: symlinked, sameRepository: true},
				{name: "linked worktree", dir: linked},
			}

			for _, tc := range resolves {
				got, err := adapter.Resolve(fixtureContext(t), tc.dir)
				if err != nil {
					t.Errorf("Resolve %s (%q): %v", tc.name, tc.dir, err)
					continue
				}
				if !sameFile(t, got.CommonDir, main.CommonDir) {
					t.Errorf("Resolve %s: CommonDir = %q, want the repository's %q", tc.name, got.CommonDir, main.CommonDir)
				}
				if tc.sameRepository && !sameFile(t, got.WorktreeRoot, main.WorktreeRoot) {
					t.Errorf("Resolve %s: WorktreeRoot = %q, want %q", tc.name, got.WorktreeRoot, main.WorktreeRoot)
				}
				if got.IsBare {
					t.Errorf("Resolve %s: IsBare = true for a repository with a worktree", tc.name)
				}
			}

			t.Run("linked worktree is still flagged as one", func(t *testing.T) {
				got, err := adapter.Resolve(fixtureContext(t), linked)
				if err != nil {
					t.Fatalf("Resolve the linked worktree: %v", err)
				}
				if !got.IsLinkedWorktree {
					t.Fatalf("IsLinkedWorktree = false for %q", linked)
				}
			})

			t.Run("a genuinely absent repository is still NOT_A_GIT_REPOSITORY", func(t *testing.T) {
				outside := t.TempDir()
				requireOutsideAnyRepository(t, outside)

				_, err := adapter.Resolve(fixtureContext(t), outside)
				if err == nil {
					t.Fatalf("Resolve accepted a directory outside every repository")
				}
				if !errors.Is(err, ErrNotARepository) {
					t.Fatalf("Resolve error = %v, want ErrNotARepository", err)
				}
				if errors.Is(err, ErrRepositoryUnreadable) {
					t.Fatalf("an absent repository was reported as an unreadable one: %v", err)
				}
				payload, ok := app.PayloadOf(err)
				if !ok {
					t.Fatalf("error carries no domain payload: %v", err)
				}
				if payload.Code != app.CodeNotAGitRepository {
					t.Fatalf("code = %q, want %q", payload.Code, app.CodeNotAGitRepository)
				}
				if got := app.ExitCode(err); got != app.ExitUsage {
					t.Fatalf("ExitCode = %d, want %d (usage)", got, app.ExitUsage)
				}
			})
		})
	}
}

// TestResolveReportsADamagedRepositoryAsDamaged is the end-to-end form of
// finding F1, against the real git binary. A repository whose config git cannot
// parse exits 128 exactly like a directory outside every repository; only git's
// stderr tells them apart.
func TestResolveReportsADamagedRepositoryAsDamaged(t *testing.T) {
	repo := newRepoFixture(t)
	config := filepath.Join(repo, ".git", "config")
	if err := os.WriteFile(config, []byte("[core\n"), 0o600); err != nil {
		t.Fatalf("damage the repository config: %v", err)
	}

	_, err := NewAdapter(NewExecRunner()).Resolve(fixtureContext(t), repo)
	if err == nil {
		t.Fatalf("Resolve accepted a repository whose config git cannot parse")
	}

	if !errors.Is(err, ErrRepositoryUnreadable) {
		t.Fatalf("Resolve error = %v, want ErrRepositoryUnreadable", err)
	}
	if errors.Is(err, ErrNotARepository) {
		t.Fatalf("a real repository with a damaged config was reported as no repository: %v", err)
	}

	payload, ok := app.PayloadOf(err)
	if !ok {
		t.Fatalf("error carries no domain payload: %v", err)
	}
	if payload.Code != app.CodeGitUnavailable {
		t.Fatalf("code = %q, want %q", payload.Code, app.CodeGitUnavailable)
	}
	// `git init` here would create nothing and fix nothing: the repository is
	// already there.
	for _, action := range payload.NextAction {
		if strings.Contains(action, "git init") {
			t.Errorf("next_action %q advises creating a repository that already exists", action)
		}
	}
	if !strings.Contains(payload.Metadata["git_stderr"], "bad config") {
		t.Errorf("git_stderr = %q, want git's own explanation", payload.Metadata["git_stderr"])
	}
}

// requireOutsideAnyRepository establishes the precondition by observation
// rather than by asserting it: if the machine's temp root happens to sit under
// a checkout, the fixture cannot stand for "outside every repository" and the
// test says so instead of passing for the wrong reason.
func requireOutsideAnyRepository(t *testing.T, dir string) {
	t.Helper()

	for current := filepath.Clean(dir); ; {
		if _, err := os.Stat(filepath.Join(current, ".git")); err == nil {
			t.Skipf("%q lies under the repository at %q, so it cannot stand for a directory outside every repository", dir, current)
		}
		parent := filepath.Dir(current)
		if parent == current {
			return
		}
		current = parent
	}
}

// newRepoFixture builds a real single-commit repository in a temp directory.
// `git worktree add` needs a commit to point at, so the empty commit is not
// decoration.
func newRepoFixture(t *testing.T) string {
	t.Helper()
	requireGit(t)
	isolateGitConfig(t)

	repo := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(repo, 0o700); err != nil {
		t.Fatalf("create repository directory: %v", err)
	}

	runGit(t, "", "init", "--quiet", "--initial-branch=main", repo)
	runGit(t, repo, "config", "user.email", "fixture@example.invalid")
	runGit(t, repo, "config", "user.name", "Mindrail Fixture")
	runGit(t, repo, "config", "commit.gpgsign", "false")
	runGit(t, repo, "commit", "--quiet", "--allow-empty", "-m", "root commit")

	return repo
}

// isolateGitConfig detaches the fixtures from whoever runs them: a developer's
// global includeIf, hooksPath or template directory must not decide whether
// this package's tests pass.
func isolateGitConfig(t *testing.T) {
	t.Helper()

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_SYSTEM", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
}

func requireGit(t *testing.T) {
	t.Helper()

	if _, err := exec.LookPath("git"); err != nil {
		t.Skipf("git is not installed: %v", err)
	}
}

// runGit drives the fixture setup. It is deliberately not the package's own
// runner: a fixture built with the code under test could hide a defect in it.
func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "LC_ALL=C")

	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}

func fixtureContext(t *testing.T) context.Context {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	return ctx
}

// sameFile compares two paths by identity rather than spelling, so a fixture
// under a symlinked temp root (the macOS /tmp case) still matches.
func sameFile(t *testing.T, a, b string) bool {
	t.Helper()

	if a == b {
		return true
	}
	infoA, err := os.Stat(a)
	if err != nil {
		return false
	}
	infoB, err := os.Stat(b)
	if err != nil {
		return false
	}
	return os.SameFile(infoA, infoB)
}
