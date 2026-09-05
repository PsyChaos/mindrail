package git

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PsyChaos/mindrail/internal/app"
)

// exit128 is what every git fatal looks like to os/exec: one status, no clue.
// The whole point of the tests below is that the status is not the signal.
func exit128() error { return errors.New("exit status 128") }

// TestExit128IsClassifiedByWhatGitSaid is the regression test for finding F1.
//
// Before the fix every non-zero exit from `rev-parse --is-inside-work-tree`
// became NOT_A_GIT_REPOSITORY with the `git init` remedy, including inside a
// real repository whose config git could not parse. The stderr — the only text
// that says what actually happened — was discarded on the way out of the
// runner.
//
// Every stderr below is copied from git 2.55.0 with LC_ALL=C, which is the
// locale the runner forces, so the strings are the ones this code will really
// see rather than the ones it would like to see.
func TestExit128IsClassifiedByWhatGitSaid(t *testing.T) {
	tests := []struct {
		name string
		// stderr is git's own output for the condition.
		stderr string
		// wantAbsent is true when git itself said there is no repository here,
		// which is the only case that may keep the `git init` remedy.
		wantAbsent bool
	}{
		{
			name:       "outside every repository",
			stderr:     "fatal: not a git repository (or any parent up to mount point /)\nStopping at filesystem boundary (GIT_DISCOVERY_ACROSS_FILESYSTEM not set).\n",
			wantAbsent: true,
		},
		{
			name:       "outside every repository, parent-directory phrasing",
			stderr:     "fatal: not a git repository (or any of the parent directories): .git\n",
			wantAbsent: true,
		},
		{
			name:       "named repository is absent",
			stderr:     "fatal: not a git repository: '/srv/gone/.git'\n",
			wantAbsent: true,
		},
		{
			name:   "unparseable repository config",
			stderr: "fatal: bad config line 1 in file .git/config\n",
		},
		{
			name:   "repository format this git cannot read",
			stderr: "fatal: Expected git repo version <= 1, found 99\n",
		},
		{
			name:   "ownership check refused",
			stderr: "fatal: detected dubious ownership in repository at '/srv/repo'\nTo add an exception for this directory, call:\n\n\tgit config --global --add safe.directory /srv/repo\n",
		},
		{
			name:   "ambient command-line config is malformed",
			stderr: "error: bogus count in GIT_CONFIG_COUNT\nfatal: unable to parse command-line config\n",
		},
		{
			name:   "corrupt object store",
			stderr: "fatal: loose object 0f1e2d3 is corrupt\n",
		},
		{
			// git always says something on a fatal. If it somehow said nothing,
			// claiming it reported an absent repository would be inventing a
			// statement, so the conservative branch is the right one.
			name:   "git said nothing at all",
			stderr: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := &FakeRunner{Default: FakeResponse{Stderr: tt.stderr, Err: exit128()}}

			_, err := NewAdapter(fake).Resolve(context.Background(), "/srv/somewhere")

			payload, ok := app.PayloadOf(err)
			if !ok {
				t.Fatalf("error carries no domain payload: %v", err)
			}

			if tt.wantAbsent {
				if !errors.Is(err, ErrNotARepository) {
					t.Fatalf("error = %v, want ErrNotARepository", err)
				}
				if errors.Is(err, ErrRepositoryUnreadable) {
					t.Fatalf("git said the repository is absent but it was reported as unreadable: %v", err)
				}
				if payload.Code != app.CodeNotAGitRepository {
					t.Fatalf("code = %q, want %q", payload.Code, app.CodeNotAGitRepository)
				}
				if got := app.ExitCode(err); got != app.ExitUsage {
					t.Fatalf("ExitCode = %d, want %d (usage)", got, app.ExitUsage)
				}
				if !mentionsGitInit(payload.NextAction) {
					t.Errorf("next_action = %q, want the `git init` remedy on the one case that genuinely has no repository", payload.NextAction)
				}
				return
			}

			// Everything else: git ran inside something it could not read. The
			// repository may well exist, so `git init` is not a remedy.
			if !errors.Is(err, ErrRepositoryUnreadable) {
				t.Fatalf("error = %v, want ErrRepositoryUnreadable", err)
			}
			if errors.Is(err, ErrNotARepository) {
				t.Fatalf("a git failure that never mentioned a missing repository was reported as one: %v", err)
			}
			if payload.Code != app.CodeGitUnavailable {
				t.Fatalf("code = %q, want %q", payload.Code, app.CodeGitUnavailable)
			}
			if got := app.ExitCode(err); got != app.ExitUnavailable {
				t.Fatalf("ExitCode = %d, want %d (unavailable)", got, app.ExitUnavailable)
			}
			if mentionsGitInit(payload.NextAction) {
				t.Errorf("next_action = %q advises `git init` for a repository that may already exist", payload.NextAction)
			}
			assertPayloadIsActionable(t, payload)
			if payload.Metadata["start_dir"] != "/srv/somewhere" {
				t.Errorf("start_dir metadata = %q, want the directory the question was asked in", payload.Metadata["start_dir"])
			}
		})
	}
}

// TestGitStderrReachesTheReader pins the second half of F1: classifying
// correctly is not enough if git's own sentence is still thrown away, because
// it is the only text that names the actual fault.
func TestGitStderrReachesTheReader(t *testing.T) {
	const stderr = "fatal: bad config line 1 in file .git/config\n"
	fake := &FakeRunner{Default: FakeResponse{Stderr: stderr, Err: exit128()}}

	_, err := NewAdapter(fake).Resolve(context.Background(), "/srv/repo")

	const want = "fatal: bad config line 1 in file .git/config"

	payload, ok := app.PayloadOf(err)
	if !ok {
		t.Fatalf("error carries no domain payload: %v", err)
	}
	if got := payload.Metadata["git_stderr"]; !strings.Contains(got, want) {
		t.Errorf("git_stderr metadata = %q, want it to contain %q", got, want)
	}
	if !strings.Contains(payload.Why, want) {
		t.Errorf("why = %q, want it to contain git's own message %q", payload.Why, want)
	}
	if !strings.Contains(err.Error(), want) {
		t.Errorf("error text = %q, want git's message to survive into the cause chain", err.Error())
	}
}

// TestGitStderrAdviceDoesNotSpillIntoWhy keeps the multi-line advice git
// appends to some fatals in the metadata rather than in a field a renderer
// prints as one sentence.
func TestGitStderrAdviceDoesNotSpillIntoWhy(t *testing.T) {
	const stderr = "fatal: detected dubious ownership in repository at '/srv/repo'\nTo add an exception for this directory, call:\n\n\tgit config --global --add safe.directory /srv/repo\n"
	fake := &FakeRunner{Default: FakeResponse{Stderr: stderr, Err: exit128()}}

	_, err := NewAdapter(fake).Resolve(context.Background(), "/srv/repo")

	payload, _ := app.PayloadOf(err)
	if strings.Contains(payload.Why, "\n") {
		t.Errorf("why spans several lines:\n%s", payload.Why)
	}
	if !strings.Contains(payload.Why, "dubious ownership") {
		t.Errorf("why = %q, dropped git's first line", payload.Why)
	}
	if !strings.Contains(payload.Metadata["git_stderr"], "safe.directory") {
		t.Errorf("git_stderr = %q, dropped git's follow-up advice", payload.Metadata["git_stderr"])
	}
}

// TestSecondProbeFailureIsClassifiedToo covers the other place a fatal can
// arrive: git answered "not in a worktree", and the follow-up probe that
// separates a bare repository from a .git subdirectory then failed. Before the
// fix that path also collapsed into "not a Git repository".
func TestSecondProbeFailureIsClassifiedToo(t *testing.T) {
	fake := &FakeRunner{Responses: map[string]FakeResponse{
		"rev-parse --is-inside-work-tree": {Stdout: "false\n"},
		"rev-parse --is-bare-repository": {
			Stderr: "fatal: bad config line 1 in file .git/config\n",
			Err:    exit128(),
		},
	}}

	_, err := NewAdapter(fake).Resolve(context.Background(), "/srv/repo/.git")

	if !errors.Is(err, ErrRepositoryUnreadable) {
		t.Fatalf("error = %v, want ErrRepositoryUnreadable", err)
	}
	payload, ok := app.PayloadOf(err)
	if !ok {
		t.Fatalf("error carries no domain payload: %v", err)
	}
	if !strings.Contains(payload.Metadata["git_stderr"], "bad config line 1") {
		t.Errorf("git_stderr = %q, dropped git's message from the second probe", payload.Metadata["git_stderr"])
	}
}

// TestLayoutProbeFailureKeepsStderr covers the probes that run after discovery
// has already succeeded. They were classified correctly before the fix; what
// they lost was git's message.
func TestLayoutProbeFailureKeepsStderr(t *testing.T) {
	fake := &FakeRunner{
		Responses: map[string]FakeResponse{
			"rev-parse --is-inside-work-tree": {Stdout: "true\n"},
		},
		Default: FakeResponse{Stderr: "fatal: unsafe repository\n", Err: exit128()},
	}

	_, err := NewAdapter(fake).Resolve(context.Background(), "/srv/repo")

	payload, ok := app.PayloadOf(err)
	if !ok {
		t.Fatalf("error carries no domain payload: %v", err)
	}
	if payload.Code != app.CodeGitUnavailable {
		t.Fatalf("code = %q, want %q", payload.Code, app.CodeGitUnavailable)
	}
	if !strings.Contains(payload.Metadata["git_stderr"], "unsafe repository") {
		t.Errorf("git_stderr = %q, dropped git's message", payload.Metadata["git_stderr"])
	}
}

// TestVersionKeepsGitStderr closes the same hole on the one call that does not
// go through rev-parse.
func TestVersionKeepsGitStderr(t *testing.T) {
	fake := &FakeRunner{Default: FakeResponse{
		Stderr: "fatal: this build of git is broken\n",
		Err:    errors.New("exit status 1"),
	}}

	_, err := NewAdapter(fake).Version(context.Background())

	const want = "fatal: this build of git is broken"

	if !errors.Is(err, ErrGitUnavailable) {
		t.Fatalf("error = %v, want ErrGitUnavailable", err)
	}
	payload, ok := app.PayloadOf(err)
	if !ok {
		t.Fatalf("error carries no domain payload: %v", err)
	}
	if got := payload.Metadata["git_stderr"]; !strings.Contains(got, want) {
		t.Errorf("git_stderr metadata = %q, want it to contain %q", got, want)
	}
	if !strings.Contains(payload.Cause, want) {
		t.Errorf("cause = %q, want git's own message to survive into the cause chain", payload.Cause)
	}
}

// TestUnusableWorkingDirectoryIsNotBlamedOnGit is the regression test for
// finding F3. Cmd.Dir pointing at a directory that cannot be entered fails
// before git is executed, and the resulting error unwraps to os.ErrNotExist —
// indistinguishable, to the old classifier, from a missing binary. The user was
// told to install git, and the directory was never named.
func TestUnusableWorkingDirectoryIsNotBlamedOnGit(t *testing.T) {
	tmp := t.TempDir()
	file := filepath.Join(tmp, "a-file")
	if err := os.WriteFile(file, []byte("not a directory"), 0o600); err != nil {
		t.Fatalf("write fixture file: %v", err)
	}

	tests := []struct {
		name string
		dir  string
	}{
		{name: "directory does not exist", dir: filepath.Join(tmp, "no-such-directory")},
		{name: "path is a file", dir: file},
		// Finding H6. This is the shape the F3 fix missed: os/exec's pre-flight
		// check is an os.Stat, which a directory at mode 0000 passes, so the
		// refusal happens after the fork and arrives as an *fs.PathError whose
		// Op is "fork/exec" and whose Path is the git binary. Read literally it
		// says git could not be executed — GIT_UNAVAILABLE, exit 4, "install
		// git", and the directory never named at all.
		{name: "directory exists but cannot be entered", dir: unenterableDir(t, 0o000)},
		// The same refusal from the other direction: read permission without
		// the search bit. chdir needs the search bit, so git cannot enter this
		// one either.
		{name: "directory is readable but not searchable", dir: unenterableDir(t, 0o444)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			requireGit(t)

			_, _, err := NewExecRunner().Run(context.Background(), tt.dir, "rev-parse", "--is-inside-work-tree")

			if err == nil {
				t.Fatalf("Run succeeded with Dir = %q", tt.dir)
			}
			if !errors.Is(err, ErrWorkingDirectory) {
				t.Fatalf("error = %v, want ErrWorkingDirectory", err)
			}
			// The binary is fine. Saying otherwise sends the reader to
			// reinstall something that is not broken.
			if errors.Is(err, ErrGitUnavailable) {
				t.Fatalf("a bad working directory was reported as an unavailable git: %v", err)
			}

			payload, ok := app.PayloadOf(err)
			if !ok {
				t.Fatalf("error carries no domain payload: %v", err)
			}
			if payload.Code != app.CodeNotAGitRepository {
				t.Fatalf("code = %q, want %q", payload.Code, app.CodeNotAGitRepository)
			}
			if got := app.ExitCode(err); got != app.ExitUsage {
				t.Fatalf("ExitCode = %d, want %d (usage: a mistyped path is user-correctable)", got, app.ExitUsage)
			}
			assertPayloadIsActionable(t, payload)

			// The entire content of this failure is *which* directory, so the
			// directory has to be in the sentence, not only in the metadata.
			if !strings.Contains(payload.Why, tt.dir) {
				t.Errorf("why = %q, does not name the directory %q", payload.Why, tt.dir)
			}
			if payload.Metadata["start_dir"] != tt.dir {
				t.Errorf("start_dir metadata = %q, want %q", payload.Metadata["start_dir"], tt.dir)
			}
			for _, action := range payload.NextAction {
				if strings.Contains(action, "install git") {
					t.Errorf("next_action %q tells the reader to fix git, which is not what broke", action)
				}
			}
		})
	}
}

// TestUnusableWorkingDirectorySurvivesTheAdapter proves the classification is
// not undone one layer up: Resolve used to re-wrap anything it could not
// recognise as "not a Git repository", which would throw the directory's name
// away again.
func TestUnusableWorkingDirectorySurvivesTheAdapter(t *testing.T) {
	requireGit(t)

	missing := filepath.Join(t.TempDir(), "no-such-directory")

	_, err := NewAdapter(NewExecRunner()).Resolve(fixtureContext(t), missing)

	if !errors.Is(err, ErrWorkingDirectory) {
		t.Fatalf("Resolve error = %v, want ErrWorkingDirectory", err)
	}
	payload, ok := app.PayloadOf(err)
	if !ok {
		t.Fatalf("error carries no domain payload: %v", err)
	}
	if !strings.Contains(payload.Why, missing) {
		t.Fatalf("why = %q, does not name the directory %q", payload.Why, missing)
	}
}

// TestWorkingDirectoryDetectionDoesNotOverFire is the guard the last pass was
// missing: the condition being detected must not fire on the adjacent
// conditions it is supposed to be distinguished *from*.
func TestWorkingDirectoryDetectionDoesNotOverFire(t *testing.T) {
	t.Run("a genuinely missing binary is still reported as unavailable", func(t *testing.T) {
		// The directory is real; only the binary is not. This is the case the
		// old code was right about, and the new check must not steal it.
		runner := &ExecRunner{Bin: filepath.Join(t.TempDir(), "no-such-git")}

		_, _, err := runner.Run(context.Background(), t.TempDir(), "rev-parse")

		if !errors.Is(err, ErrGitUnavailable) {
			t.Fatalf("error = %v, want ErrGitUnavailable", err)
		}
		if errors.Is(err, ErrWorkingDirectory) {
			t.Fatalf("a missing binary was reported as a bad working directory: %v", err)
		}
		payload, _ := app.PayloadOf(err)
		if payload.Code != app.CodeGitUnavailable {
			t.Fatalf("code = %q, want %q", payload.Code, app.CodeGitUnavailable)
		}
	})

	t.Run("an ordinary git fatal in a real directory is untouched", func(t *testing.T) {
		requireGit(t)
		outside := t.TempDir()
		requireOutsideAnyRepository(t, outside)

		_, stderr, err := NewExecRunner().Run(context.Background(), outside, "rev-parse", "--is-inside-work-tree")

		if err == nil {
			t.Fatalf("rev-parse succeeded outside every repository; stderr = %q", stderr)
		}
		if errors.Is(err, ErrWorkingDirectory) {
			t.Fatalf("a usable directory was reported as unusable: %v", err)
		}
		if !strings.Contains(strings.ToLower(string(stderr)), "not a git repository") {
			t.Fatalf("stderr = %q, want git's own not-a-repository message; the classifier reads this text", stderr)
		}
	})

	t.Run("a successful call in a real directory is untouched", func(t *testing.T) {
		repo := newRepoFixture(t)

		stdout, _, err := NewExecRunner().Run(context.Background(), repo, "rev-parse", "--is-inside-work-tree")
		if err != nil {
			t.Fatalf("Run in a healthy repository: %v", err)
		}
		if strings.TrimSpace(string(stdout)) != "true" {
			t.Fatalf("stdout = %q, want \"true\"", stdout)
		}
	})

	// The adjacent condition the H6 fix must not steal. A git binary that
	// exists but is not executable fails with exactly the error a directory at
	// mode 0000 produces — *fs.PathError, Op "fork/exec", permission denied —
	// and here the directory really is fine and git really is the problem.
	t.Run("a git binary that cannot be executed is still reported as unavailable", func(t *testing.T) {
		requirePermissionsAreEnforced(t)

		bin := filepath.Join(t.TempDir(), "git")
		if err := os.WriteFile(bin, []byte("#!/bin/sh\nexit 0\n"), 0o644); err != nil {
			t.Fatalf("write fixture binary: %v", err)
		}

		runner := &ExecRunner{Bin: bin}
		_, _, err := runner.Run(context.Background(), t.TempDir(), "rev-parse")

		if !errors.Is(err, ErrGitUnavailable) {
			t.Fatalf("error = %v, want ErrGitUnavailable", err)
		}
		if errors.Is(err, ErrWorkingDirectory) {
			t.Fatalf("a non-executable git binary was blamed on the working directory: %v", err)
		}
		payload, _ := app.PayloadOf(err)
		if payload.Code != app.CodeGitUnavailable {
			t.Fatalf("code = %q, want %q", payload.Code, app.CodeGitUnavailable)
		}
	})

	// The over-fire the obvious implementation makes. Opening a directory needs
	// read permission; entering it needs the search bit. An execute-only
	// directory is entirely usable to git, and an os.Open-based probe would
	// condemn it.
	t.Run("an execute-only directory is usable and is left alone", func(t *testing.T) {
		requireGit(t)
		requirePermissionsAreEnforced(t)

		dir := filepath.Join(t.TempDir(), "execute-only")
		if err := os.Mkdir(dir, 0o111); err != nil {
			t.Fatalf("create fixture directory: %v", err)
		}
		t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
		if fault := directoryEntryFault(dir); fault != nil {
			t.Fatalf("directoryEntryFault(%q) = %v, want nil: git enters this directory happily", dir, fault)
		}

		_, stderr, err := NewExecRunner().Run(context.Background(), dir, "rev-parse", "--is-inside-work-tree")

		// git runs, and then fails for its own reason: there is no repository
		// here. That is a completely different verdict and must survive.
		if errors.Is(err, ErrWorkingDirectory) {
			t.Fatalf("an execute-only directory git can enter was reported as unusable: %v", err)
		}
		if !strings.Contains(strings.ToLower(string(stderr)), "not a git repository") {
			t.Fatalf("stderr = %q, want git's own not-a-repository message", stderr)
		}
	})

	t.Run("Version runs with no directory at all", func(t *testing.T) {
		requireGit(t)

		// Cmd.Dir is empty for --version. An eager directory check would fire
		// on every invocation of it.
		version, err := NewAdapter(NewExecRunner()).Version(fixtureContext(t))
		if err != nil {
			t.Fatalf("Version: %v", err)
		}
		if !strings.HasPrefix(version, "git version ") {
			t.Fatalf("Version = %q", version)
		}
	})
}

// unenterableDir builds a directory at a mode that denies the search
// permission chdir needs. It restores the mode afterwards so the test framework
// can remove the temp tree.
func unenterableDir(t *testing.T, mode os.FileMode) string {
	t.Helper()
	requirePermissionsAreEnforced(t)

	dir := filepath.Join(t.TempDir(), "unenterable")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatalf("create fixture directory: %v", err)
	}
	if err := os.Chmod(dir, mode); err != nil {
		t.Fatalf("chmod fixture directory: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	// The precondition is established by observation rather than asserted: on a
	// filesystem or platform that does not enforce the mode, the fixture cannot
	// stand for "cannot be entered" and the test says so instead of passing for
	// the wrong reason.
	if _, err := os.Stat(dir + string(os.PathSeparator) + "."); err == nil {
		t.Skipf("this environment does not enforce mode %o on %q", mode, dir)
	}
	return dir
}

// requirePermissionsAreEnforced skips tests that depend on a permission bit
// actually denying something. Running as root defeats every one of them, and a
// test that silently passes because it proved nothing is worse than a skip.
func requirePermissionsAreEnforced(t *testing.T) {
	t.Helper()

	if os.Geteuid() == 0 {
		t.Skip("running as root: permission bits do not deny anything")
	}
}

func mentionsGitInit(actions []string) bool {
	for _, action := range actions {
		if strings.Contains(action, "git init") {
			return true
		}
	}
	return false
}
