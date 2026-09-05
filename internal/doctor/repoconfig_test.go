package doctor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/config"
	"github.com/PsyChaos/mindrail/internal/filesystem"
	"github.com/PsyChaos/mindrail/internal/knowledge/loader"
)

// refuseRepoConfig puts a refused repository config directory into s.
//
// The directory is a real one under t.TempDir, and the error comes from the same
// filesystem constructor `mindrail init` fails through, because both of those
// decide the sentence the reader gets: the remedy has to name the directory that
// actually carries the mode, and that path is chosen by walking the disk. A
// fixture of invented paths produced "check the permissions on /", which is the
// class of remedy this whole audit is about.
//
// exists chooses which of the two shapes is set up: the directory is there and
// refuses writes, or it is absent and the worktree above it refuses to hold it.
func refuseRepoConfig(t *testing.T, s Subject, exists bool) Subject {
	t.Helper()

	worktree := t.TempDir()
	dir := filepath.Join(worktree, config.RepoDir)
	probed := worktree
	if exists {
		if err := os.Mkdir(dir, 0o700); err != nil {
			t.Fatalf("create %s: %v", dir, err)
		}
		probed = dir
	}

	s.Paths.WorktreeRoot = worktree
	s.Paths.RepoConfigDir = dir
	s.Probes.RepoConfigDir = filesystem.Writability{
		Dir:     dir,
		Kind:    filesystem.RootRepository,
		Purpose: filesystem.RootRepository.Purpose(),
		Probed:  probed,
		Exists:  exists,
		Usable:  false,
		Err: filesystem.UnwritablePath(filesystem.RootRepository, "", dir,
			&refusalCause{"access " + probed + ": permission denied"}),
	}
	return s
}

// refusalCause is a cause with no classification, so the remedy under test is
// the one the filesystem layer picks from the path rather than from the errno.
type refusalCause struct{ message string }

func (c *refusalCause) Error() string { return c.message }

// unscaffolded is a subject whose repository config directory holds nothing
// `mindrail init` has already put there: no config.toml, no knowledge tree.
func unscaffolded(s Subject) Subject {
	s.Config.RepoFile = ""
	s.Knowledge.Present = false
	return s
}

// TestAnUnwritableRepositoryConfigDirectoryBlocksTheReport is finding D4.
//
// The condition: a checkout whose worktree refuses new entries, so
// <worktree>/.mindrail cannot be created. Measured against the compiled binary
// before the fix, `mindrail doctor` reported all seven checks OK, printed
// `repo_config_dir: <repo>/.mindrail` as a detail line, exited 0, and offered
// `mindrail init` as the remedy — while that init exited 4 with
// RUNTIME_PATH_UNWRITABLE every time it was run. The directory was named in the
// report and never asked a question, which is why the report's own remedy could
// not succeed.
func TestAnUnwritableRepositoryConfigDirectoryBlocksTheReport(t *testing.T) {
	s := refuseRepoConfig(t, unscaffolded(healthySubject()), false)

	result := RuntimePathCheck(s).Run(t.Context())

	if result.State != StateError {
		t.Fatalf("runtime_paths = %s %q, want %s: `mindrail init` cannot create this directory",
			result.State, result.Summary, StateError)
	}
	if result.Code != app.CodeRuntimePathUnwritable {
		t.Errorf("code = %q, want %q", result.Code, app.CodeRuntimePathUnwritable)
	}

	// The reading has to name the directory that actually carries the mode, not
	// the one that is missing: a chmod on a path that does not exist is not a
	// remedy, and it is the sentence `init` prints for the same disk.
	remedy := strings.Join(result.NextAction, " ")
	if !strings.Contains(remedy, s.Paths.WorktreeRoot) {
		t.Errorf("next_action = %v, which does not name %s, the directory that carries the mode",
			result.NextAction, s.Paths.WorktreeRoot)
	}
	if strings.Contains(remedy, initCommand) {
		t.Errorf("next_action = %v, which sends the reader to the command that fails on this condition",
			result.NextAction)
	}

	// Both readings are published whatever the verdict, because the detail line
	// that named the directory for five audits is what made the missing question
	// invisible.
	if got := result.Details["repo_config_dir_usable"]; got != "false" {
		t.Errorf("repo_config_dir_usable = %q, want %q", got, "false")
	}
	if got := result.Details["repo_config_dir_exists"]; got != "false" {
		t.Errorf("repo_config_dir_exists = %q, want %q", got, "false")
	}
}

// TestAnUnwritableRepositoryConfigDirectorySupersedesTheInitRemedy is the other
// half of the same finding: the runtime store's three checks each recommend
// `mindrail init` on a repository nobody has initialised, and none of them can
// see that the worktree will refuse it.
func TestAnUnwritableRepositoryConfigDirectorySupersedesTheInitRemedy(t *testing.T) {
	s := refuseRepoConfig(t, unscaffolded(healthySubject()), false)
	s.DBPresent = false
	s.Migrations = nil
	s.Workspace.ID = ""

	for _, check := range DefaultChecks(s) {
		result := check.Run(t.Context())
		if offersInit(result.NextAction) {
			t.Errorf("check %q recommends `%s` in a worktree that will refuse it: %v",
				result.Name, initCommand, result.NextAction)
		}
	}
}

// TestARepositoryConfigDirectoryWithNothingLeftToReceiveIsNotFatal is the
// over-fire guard, and it is the adjacent condition the detection must not
// swallow.
//
// It was measured, not reasoned about. Against a fully scaffolded `.mindrail` at
// mode 0500, `mindrail init` exits 0 — WriteIfAbsent finds config.toml and
// writes nothing, EnsureKnowledgeDirs finds both knowledge directories and
// creates nothing — and `mindrail status` exits 0. Grading that as fatal would
// be finding F13 again one directory over: a fully working repository driven to
// ERROR and exit 4 over a directory this run never needs to write to.
func TestARepositoryConfigDirectoryWithNothingLeftToReceiveIsNotFatal(t *testing.T) {
	s := refuseRepoConfig(t, healthySubject(), true)
	s.Config.RepoFile = filepath.Join(s.Paths.RepoConfigDir, config.ConfigFileName)
	s.Knowledge.Present = true

	result := RuntimePathCheck(s).Run(t.Context())

	if result.State != StateDegraded {
		t.Fatalf("runtime_paths = %s %q, want %s: `mindrail init` exits 0 against this repository",
			result.State, result.Summary, StateDegraded)
	}
	if result.Code != app.CodeRuntimePathUnwritable {
		t.Errorf("code = %q, want %q", result.Code, app.CodeRuntimePathUnwritable)
	}
	if len(result.NextAction) == 0 {
		t.Error("a DEGRADED reading with no next action is one nobody can act on")
	}

	// And it must not reach into the rest of the document either: a repository
	// that has nothing left to write here is a repository `mindrail init` still
	// works in.
	s.DBPresent = false
	s.Migrations = nil
	s.Workspace.ID = ""

	sqlite := SQLiteCheck(s).Run(t.Context())
	if !offersInit(sqlite.NextAction) {
		t.Errorf("sqlite check no longer offers `%s` in a repository init would fix: %v",
			initCommand, sqlite.NextAction)
	}
}

// TestAnUnreadRepositoryConfigDirectoryIsNotAFinding is the second over-fire
// guard, and it is the rule the rest of this package is built on: a zero value
// is an unasked question, never an answer.
func TestAnUnreadRepositoryConfigDirectoryIsNotAFinding(t *testing.T) {
	s := unscaffolded(healthySubject())
	s.Probes.RepoConfigDirKnown = false
	s.Probes.RepoConfigDir = filesystem.Writability{}

	result := RuntimePathCheck(s).Run(t.Context())

	if result.State != StateOK {
		t.Fatalf("runtime_paths = %s %q from a probe that was never taken", result.State, result.Summary)
	}
	if _, published := result.Details["repo_config_dir_usable"]; published {
		t.Errorf("details publish repo_config_dir_usable for a probe nobody took: %v", result.Details)
	}
}

// TestAScaffoldNobodyReadIsNotReportedAsMissing is the third over-fire guard.
//
// The severity rule reads Knowledge.Present, and a startup that stopped at the
// runtime store leaves that false because the knowledge step never ran — not
// because the directory is absent. Reading the second from the first would have
// the report invent a finding out of an unasked question, which is the defect
// this whole package is organised against.
func TestAScaffoldNobodyReadIsNotReportedAsMissing(t *testing.T) {
	s := refuseRepoConfig(t, healthySubject(), true)

	// Startup aborted opening the database, so nothing after it ran.
	s.DBErr = app.NewError(app.CodeRuntimeDBUnavailable, app.KindUnavailable,
		"the runtime database could not be opened", "nothing can run", "retry")
	s.DBPresent = false
	s.Migrations = nil
	s.Knowledge = loader.Store{}
	s.Workspace.ID = ""

	result := RuntimePathCheck(s).Run(t.Context())

	if result.State == StateError {
		t.Fatalf("runtime_paths = %s %q; the knowledge step never ran, so nothing here establishes "+
			"that the scaffold is missing", result.State, result.Summary)
	}
}

// TestAnUnusableCacheDirectoryStillOutranksNothing keeps the two DEGRADED
// readings from being reordered by accident. The cache reading came first and
// stays first; adding a second one below it must not make a repository with a
// broken cache report the repository config directory instead.
func TestAnUnusableCacheDirectoryStillOutranksNothing(t *testing.T) {
	s := healthySubject()
	s.Probes.CacheDir.Usable = false
	s.Probes.CacheDir.Err = filesystem.UnwritablePath(filesystem.RootCache, "",
		s.Paths.CacheDir, &refusalCause{"access " + s.Paths.CacheDir + ": permission denied"})

	result := RuntimePathCheck(s).Run(t.Context())

	if result.State != StateDegraded {
		t.Fatalf("runtime_paths = %s %q, want %s", result.State, result.Summary, StateDegraded)
	}
	if !strings.Contains(result.Summary, "Cache") {
		t.Errorf("summary = %q, want the cache directory named; the repository config directory is usable here",
			result.Summary)
	}
}

// TestBothRootsUnusableNamesTheOneInitMeetsFirst pins the order the two fatal
// branches are asked in, which is the whole of its value.
//
// A filesystem mounted read-only makes both directories unusable at once, and
// whichever branch fires first becomes the document's error object. `mindrail
// init` writes the repository config directory at §87 step 2 and creates the
// runtime root at step 3, so naming the runtime root first published a `why`,
// a path and a remedy that the `init` about to fail on the same disk did not
// print. Measured on a read-only tmpfs before the reorder: `doctor` said "the
// runtime root <repo>/.git/mindrail could not be created … check the permissions
// on <repo>/.git" and `init` said "the repository config directory
// <repo>/.mindrail could not be created … check the permissions on <repo>".
func TestBothRootsUnusableNamesTheOneInitMeetsFirst(t *testing.T) {
	s := refuseRepoConfig(t, unscaffolded(healthySubject()), false)
	s.Probes.RuntimeDir.Usable = false
	s.Probes.RuntimeDir.Err = filesystem.UnwritablePath(filesystem.RootRuntime, "",
		s.Paths.RuntimeRoot, &refusalCause{"access " + s.Paths.RuntimeRoot + ": read-only file system"})

	result := RuntimePathCheck(s).Run(t.Context())

	if result.State != StateError {
		t.Fatalf("runtime_paths = %s %q, want %s", result.State, result.Summary, StateError)
	}
	if got := result.Metadata["root_kind"]; got != string(filesystem.RootRepository) {
		t.Errorf("root_kind = %q, want %q: `mindrail init` meets that directory first, and the two "+
			"documents have to name one condition the same way", got, filesystem.RootRepository)
	}
}

// TestOnlyTheRuntimeRootUnusableStillNamesTheRuntimeRoot is the over-fire guard
// for the reorder. Only one branch can fire when only one root is broken, and
// the far commoner condition — an unwritable .git with a perfectly good worktree
// — must keep the remedy that names it and the MINDRAIL_RUNTIME_DIR escape hatch
// that goes with it.
func TestOnlyTheRuntimeRootUnusableStillNamesTheRuntimeRoot(t *testing.T) {
	s := healthySubject()
	s.Probes.RuntimeDir.Usable = false
	s.Probes.RuntimeDir.Err = filesystem.UnwritablePath(filesystem.RootRuntime, "",
		s.Paths.RuntimeRoot, &refusalCause{"access " + s.Paths.RuntimeRoot + ": permission denied"})

	result := RuntimePathCheck(s).Run(t.Context())

	if result.State != StateError {
		t.Fatalf("runtime_paths = %s %q, want %s", result.State, result.Summary, StateError)
	}
	if got := result.Metadata["root_kind"]; got != string(filesystem.RootRuntime) {
		t.Errorf("root_kind = %q, want %q", got, filesystem.RootRuntime)
	}
	if !strings.Contains(strings.Join(result.NextAction, " "), "MINDRAIL_RUNTIME_DIR") {
		t.Errorf("next_action = %v drops the escape hatch that relocates this root", result.NextAction)
	}
}
