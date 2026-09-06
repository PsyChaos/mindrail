package bootstrap_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/bootstrap"
	"github.com/PsyChaos/mindrail/internal/doctor"
	"github.com/PsyChaos/mindrail/internal/filesystem"
	"github.com/PsyChaos/mindrail/internal/git"
	"github.com/PsyChaos/mindrail/internal/storage"
)

// TestInitFinishesWhenOnlyTheCacheDirectoryIsUnusable is finding F13 at the
// layer that produced it.
//
// EnsureDirs creates the runtime root and the cache directory and returns one
// error for whichever stopped it. Treating both as fatal aborted the §87
// sequence at step 3, so a repository that could have been fully initialised —
// database created, migrations applied, worktree registered — was reported as
// BLOCKED at exit 4 over a directory MR-001 never writes to.
//
// filesystem.RootKindOf is what makes the two separable, and this test is the
// evidence that bootstrap uses it rather than assuming.
func TestInitFinishesWhenOnlyTheCacheDirectoryIsUnusable(t *testing.T) {
	repo := newGitRepo(t)
	cacheHolder := t.TempDir()
	blockCacheCreation(t, cacheHolder)

	application := bootstrap.New(options(t, repo, bootstrap.ModeInit, func(o *bootstrap.Options) {
		o.CacheDirOverride = filepath.Join(cacheHolder, "cache")
	}))
	t.Cleanup(func() { _ = application.Shutdown(context.Background()) })

	if err := application.Start(t.Context()); err != nil {
		t.Fatalf("Start: %v\nan unusable cache directory must not stop the startup sequence", err)
	}

	subject := application.Subject()
	if subject.PathsErr != nil {
		t.Errorf("PathsErr = %v; a cache failure is not a runtime-path failure", subject.PathsErr)
	}
	if subject.CacheErr == nil {
		t.Fatal("CacheErr = nil; the condition was swallowed instead of graded")
	}
	if kind, known := filesystem.RootKindOf(subject.CacheErr); !known || kind != filesystem.RootCache {
		t.Errorf("CacheErr is about the %q root, want the cache directory", kind)
	}

	// The rest of the sequence really ran: this is what the regression cost.
	if !subject.DBPresent {
		t.Error("no runtime database was created")
	}
	if len(subject.Migrations) == 0 {
		t.Error("no migration was applied")
	}
	if subject.Workspace.ID == "" {
		t.Error("the worktree was not registered")
	}

	// And the verdict every output surface derives from is nil.
	report, verdict := application.Diagnosis(t.Context())
	if verdict != nil {
		t.Errorf("Diagnosis verdict = %v, want nil", verdict)
	}
	if report.WorstState != doctor.StateDegraded {
		t.Errorf("worst_state = %q, want %q: the condition is reported, just not fatal",
			report.WorstState, doctor.StateDegraded)
	}
}

// TestInitStopsWhenTheRuntimeRootIsUnusable is the condition the grading above
// must not relax. The runtime root holds the database every command reads, and
// a failure to create it has to stop the sequence exactly as it always did.
func TestInitStopsWhenTheRuntimeRootIsUnusable(t *testing.T) {
	repo := newGitRepo(t)
	runtimeHolder := t.TempDir()
	blockCacheCreation(t, runtimeHolder)

	application := bootstrap.New(options(t, repo, bootstrap.ModeInit, func(o *bootstrap.Options) {
		o.RuntimeDirOverride = filepath.Join(runtimeHolder, "runtime")
	}))
	t.Cleanup(func() { _ = application.Shutdown(context.Background()) })

	err := application.Start(t.Context())
	if err == nil {
		t.Fatal("Start succeeded although the runtime root could not be created")
	}

	payload, ok := app.PayloadOf(err)
	if !ok {
		t.Fatalf("Start error %v carries no domain payload", err)
	}
	if payload.Code != app.CodeRuntimePathUnwritable {
		t.Errorf("code = %q, want %q", payload.Code, app.CodeRuntimePathUnwritable)
	}
	if app.ExitCode(err) != app.ExitUnavailable {
		t.Errorf("exit code = %d, want %d", app.ExitCode(err), app.ExitUnavailable)
	}
	if application.Subject().CacheErr != nil {
		t.Error("a runtime-root failure was filed as a cache failure")
	}

	// And it is fatal all the way out, which is what "still caught" means.
	if _, verdict := application.Diagnosis(t.Context()); verdict == nil {
		t.Error("Diagnosis verdict = nil for a runtime root nothing can create")
	}
}

// TestDiagnosisIsTheOnlyVerdict is finding F11's structural claim, asserted
// where the split used to be.
//
// Run returned the start error whenever the command body reported none, and the
// command body wrote the envelope from a value that did not include it. That is
// two inputs for three answers. Run now returns the body's verdict and nothing
// else, and Diagnosis is where that verdict comes from — so a start error the
// checks cannot see would surface as a silent exit 0 here, which is a failure
// this test can catch and the previous arrangement could not.
func TestDiagnosisIsTheOnlyVerdict(t *testing.T) {
	repo := newGitRepo(t)
	initialize(t, repo)
	corruptDatabase(t, repo)

	application := bootstrap.New(options(t, repo, bootstrap.ModeReadOnly, nil))
	t.Cleanup(func() { _ = application.Shutdown(context.Background()) })

	var verdict error
	returned := application.Run(t.Context(), func(ctx context.Context, a *bootstrap.App) error {
		_, verdict = a.Diagnosis(ctx)
		return verdict
	})

	if application.StartError() == nil {
		t.Fatal("startup succeeded against a damaged runtime database; this test proves nothing")
	}
	if verdict == nil {
		t.Fatal("Diagnosis returned no verdict for a startup that failed")
	}
	if returned != verdict {
		t.Errorf("Run returned %v, want the body's verdict %v", returned, verdict)
	}

	wantCode, _ := app.PayloadOf(application.StartError())
	gotCode, _ := app.PayloadOf(verdict)
	if gotCode.Code != wantCode.Code {
		t.Errorf("verdict code = %q, start error code = %q", gotCode.Code, wantCode.Code)
	}

	// A body that reports nothing means the command found nothing to report,
	// and Run must not invent a second answer behind its back.
	quiet := bootstrap.New(options(t, repo, bootstrap.ModeReadOnly, nil))
	t.Cleanup(func() { _ = quiet.Shutdown(context.Background()) })
	if err := quiet.Run(t.Context(), func(context.Context, *bootstrap.App) error { return nil }); err != nil {
		t.Errorf("Run returned %v although the command body reported nothing", err)
	}
}

// TestEveryStartFailureReachesTheVerdict closes the one hole the envelope
// invariant cannot see.
//
// That invariant compares `ok`, the error object and the exit code with each
// other, so a start error nobody notices is self-consistent: ok:true, no error,
// exit 0, all three agreeing about a repository that is broken. Run no longer
// carries the start error separately, so this is the property that has to hold
// instead — every failure that stops the §87 sequence has to be visible to the
// checks the verdict is derived from.
//
// It is a property over failures rather than a golden, because the value of it
// is that a future step whose error the Subject does not record fails here.
func TestEveryStartFailureReachesTheVerdict(t *testing.T) {
	tests := []struct {
		name   string
		mode   bootstrap.Mode
		setup  func(t *testing.T) string
		runner git.CommandRunner

		// wantCode is the code the verdict has to carry when it is not the one
		// the failing step raised. The empty string means "the start error's own
		// code", which is every row but one.
		//
		// It exists because a check is allowed to name a failure better than the
		// layer that raised it, and exactly one condition in MR-001 does: an
		// unusable `.mindrail/knowledge` reaches the loader as a directory it
		// could not read, and reaches doctor's read-only probe as a path
		// condition with a remedy — the chmod, or the move — that clears it. The
		// probe's answer is the one every command publishes (finding E10), so the
		// verdict deliberately refines the code rather than repeating it. The row
		// names the refinement instead of the comparison being loosened for all
		// of them: the property this test exists for is that no start failure
		// reaches the process as a *different* failure, and an unstated code
		// change is exactly that.
		wantCode app.Code
	}{
		{
			name: "not a git repository",
			mode: bootstrap.ModeReadOnly,
			setup: func(t *testing.T) string {
				requireGit(t)
				return t.TempDir()
			},
		},
		{
			// The row the property exists for. Decision D-30 makes a missing git
			// UNAVAILABLE rather than ERROR, so this is the one failure no check
			// reports as an error and the only thing that makes it fatal is the
			// halt itself. Without it the table would pass against a Verdict
			// that had never learned to look (finding F11).
			name:  "git is not installed",
			mode:  bootstrap.ModeReadOnly,
			setup: func(t *testing.T) string { return t.TempDir() },
			runner: &git.FakeRunner{Default: git.FakeResponse{
				Err: app.NewError(
					app.CodeGitUnavailable,
					app.KindUnavailable,
					`the git executable "git" could not be run`,
					"Mindrail drives the system git binary and cannot operate without it",
					"install git and make sure it is on PATH",
				).WithCause(git.ErrGitUnavailable),
			}},
		},
		{
			name: "unwritable runtime root",
			mode: bootstrap.ModeInit,
			setup: func(t *testing.T) string {
				repo := newGitRepo(t)
				blockCacheCreation(t, filepath.Join(repo, ".git"))
				return repo
			},
		},
		{
			name: "corrupt runtime database",
			mode: bootstrap.ModeReadOnly,
			setup: func(t *testing.T) string {
				repo := newGitRepo(t)
				initialize(t, repo)
				corruptDatabase(t, repo)
				return repo
			},
		},
		{
			name: "malformed configuration",
			mode: bootstrap.ModeReadOnly,
			setup: func(t *testing.T) string {
				repo := newGitRepo(t)
				initialize(t, repo)
				writeConfig(t, repo, "[[[\n")
				return repo
			},
		},
		{
			name: "unreadable knowledge store",
			mode: bootstrap.ModeReadOnly,
			setup: func(t *testing.T) string {
				repo := newGitRepo(t)
				initialize(t, repo)
				blockAccess(t, filepath.Join(repo, ".mindrail", "knowledge"))
				return repo
			},
			wantCode: app.CodeRuntimePathUnwritable,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := tc.setup(t)

			application := bootstrap.New(options(t, repo, tc.mode, func(o *bootstrap.Options) {
				o.Runner = tc.runner
			}))
			t.Cleanup(func() { _ = application.Shutdown(context.Background()) })

			startErr := application.Start(t.Context())
			if startErr == nil {
				t.Fatalf("startup succeeded on %q; this row proves nothing", tc.name)
			}

			_, verdict := application.Diagnosis(t.Context())
			if verdict == nil {
				t.Fatalf("Start failed with %v but Diagnosis reported nothing: this failure would exit 0 with ok:true", startErr)
			}

			startPayload, _ := app.PayloadOf(startErr)
			verdictPayload, _ := app.PayloadOf(verdict)
			wantCode := tc.wantCode
			if wantCode == "" {
				wantCode = startPayload.Code
			}
			if verdictPayload.Code != wantCode {
				t.Errorf("verdict code = %q, want %q (start error code %q): the verdict describes a different failure",
					verdictPayload.Code, wantCode, startPayload.Code)
			}
			if app.ExitCode(verdict) == app.ExitSuccess {
				t.Errorf("verdict %q exits 0 although startup failed", verdictPayload.Code)
			}

			// The exit *class* is deliberately not compared with the start
			// error's. The two are not always the same today —
			// KNOWLEDGE_UNREADABLE is KindUnavailable where the loader raises it
			// and KindFailed where doctor.kindForCode grades it — and the
			// verdict is the one that reaches the process, so comparing them
			// would pin a value nothing reads. The code is what has to agree,
			// because that is what says which failure is being described.
		})
	}
}

// TestConcurrentFirstRunIsNotAWorkspaceFailure is finding F16 at the layer that
// made the query.
//
// A read-only start against a database another process created and has not
// finished migrating used to run the workspaces lookup anyway, and SQLite's
// `no such table: workspaces (1)` became the user-facing why line of a
// WORKSPACE_REGISTRATION_FAILED error at exit 1.
func TestConcurrentFirstRunIsNotAWorkspaceFailure(t *testing.T) {
	repo := newGitRepo(t)
	createUnmigratedDatabase(t, repo)

	application := bootstrap.New(options(t, repo, bootstrap.ModeReadOnly, nil))
	t.Cleanup(func() { _ = application.Shutdown(context.Background()) })

	if err := application.Start(t.Context()); err != nil {
		t.Fatalf("Start: %v", err)
	}

	subject := application.Subject()
	if subject.WorkspaceErr != nil {
		t.Errorf("WorkspaceErr = %v, want nil: no lookup was possible, so none should have been made", subject.WorkspaceErr)
	}
	if subject.PendingCount == 0 {
		t.Fatal("PendingCount = 0; the fixture is already migrated and proves nothing")
	}

	_, verdict := application.Diagnosis(t.Context())
	if verdict != nil {
		t.Errorf("Diagnosis verdict = %v, want nil: a first run in flight is a state", verdict)
	}
}

// TestFullyMigratedReadOnlyStartStillLooksTheWorkspaceUp is the over-fire guard
// for the skip above: the guard must only suppress a lookup that could not have
// worked, never one that can.
func TestFullyMigratedReadOnlyStartStillLooksTheWorkspaceUp(t *testing.T) {
	repo := newGitRepo(t)
	initialize(t, repo)

	application := bootstrap.New(options(t, repo, bootstrap.ModeReadOnly, nil))
	t.Cleanup(func() { _ = application.Shutdown(context.Background()) })

	if err := application.Start(t.Context()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if got := application.Subject().Workspace.ID; got == "" {
		t.Error("the workspace lookup was skipped on a fully migrated database")
	}
}

// blockCacheCreation makes dir readable but not writable, so a directory
// underneath it cannot be created. Root ignores the mode bits, so the scenario
// is unreachable there.
func blockCacheCreation(t *testing.T, dir string) {
	t.Helper()

	if os.Geteuid() == 0 {
		t.Skip("permission-denied scenarios are unreachable as root")
	}

	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat %s: %v", dir, err)
	}
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatalf("chmod %s: %v", dir, err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, info.Mode().Perm()) })
}

// blockAccess makes a path unreachable altogether.
func blockAccess(t *testing.T, path string) {
	t.Helper()

	if os.Geteuid() == 0 {
		t.Skip("permission-denied scenarios are unreachable as root")
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	if err := os.Chmod(path, 0o000); err != nil {
		t.Fatalf("chmod %s: %v", path, err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, info.Mode().Perm()) })
}

// writeConfig replaces the repository configuration file.
func writeConfig(t *testing.T, repo, content string) {
	t.Helper()

	path := filepath.Join(repo, ".mindrail", "config.toml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// createUnmigratedDatabase reproduces the window a concurrent first run opens:
// the winning process has created the database file and has not committed the
// migrations that create its tables.
func createUnmigratedDatabase(t *testing.T, repo string) {
	t.Helper()

	runtimeRoot := filepath.Join(repo, ".git", "mindrail")
	if err := os.MkdirAll(runtimeRoot, 0o700); err != nil {
		t.Fatalf("create %s: %v", runtimeRoot, err)
	}

	// storage.Open with no migrator run is exactly what the winning process has
	// done at that instant: the file exists, the pragmas are set, and no table
	// has been created. Reaching it this way makes the scenario deterministic
	// rather than a race the suite would have to hope for.
	db, err := storage.Open(t.Context(), storage.Options{Path: filepath.Join(runtimeRoot, "mindrail.db")})
	if err != nil {
		t.Fatalf("create the unmigrated database: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close the unmigrated database: %v", err)
	}
}
