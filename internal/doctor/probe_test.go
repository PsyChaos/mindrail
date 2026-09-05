package doctor

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/filesystem"
	"github.com/PsyChaos/mindrail/internal/storage"
)

// These are the only tests in the package that touch a disk, and they have to:
// the conditions they cover — a runtime directory nothing can write, a database
// path occupied by a directory — are exactly the ones a Subject assembled by
// hand cannot express, which is why the branch that reports them was reachable
// only from an injected error and never from a real repository.

// TestUnwritableRuntimePathIsReportedNotRemediedWithInit is finding R2. A
// derivation that succeeded said nothing about whether the location is usable,
// so an unwritable repository was reported as a missing database and remedied
// with the `mindrail init` that cannot succeed there either.
func TestUnwritableRuntimePathIsReportedNotRemediedWithInit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("directory mode bits do not gate creation the same way on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("root ignores the mode bits this test depends on")
	}

	commonDir := filepath.Join(t.TempDir(), "repo", ".git")
	if err := os.MkdirAll(commonDir, 0o755); err != nil {
		t.Fatalf("create common dir: %v", err)
	}
	if err := os.Chmod(commonDir, 0o500); err != nil {
		t.Fatalf("chmod common dir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(commonDir, 0o700) })

	subject := Probe(subjectAt(t, commonDir))

	paths := RuntimePathCheck(subject).Run(t.Context())
	assertDiagnosable(t, paths)
	if paths.State != StateError {
		t.Fatalf("runtime_paths reported %q on an unwritable common dir, want %q (%+v)",
			paths.State, StateError, paths)
	}
	if paths.Code != app.CodeRuntimePathUnwritable {
		t.Errorf("runtime_paths code = %q, want %q", paths.Code, app.CodeRuntimePathUnwritable)
	}
	if !strings.Contains(strings.Join(paths.NextAction, "\n"), commonDir) {
		t.Errorf("next_action %v does not name the directory that carries the permission", paths.NextAction)
	}

	// The database is missing here too, but `mindrail init` is not the remedy:
	// running it fails on the same permission.
	db := SQLiteCheck(subject).Run(t.Context())
	assertDiagnosable(t, db)
	if db.Code == app.CodeWorkspaceNotInitialized {
		t.Errorf("sqlite reported %q, which sends the user to an init that cannot succeed", db.Code)
	}
	for _, action := range db.NextAction {
		if action == initCommand {
			t.Errorf("sqlite next_action %v recommends the command that fails on this repository", db.NextAction)
		}
	}
}

// TestDirectoryAtTheDatabasePathIsNotAMissingDatabase is finding R3. Both
// conditions stop the same commands, but only one of them is fixed by
// `mindrail init`; recommending it for the other is a loop that fails
// identically every time.
func TestDirectoryAtTheDatabasePathIsNotAMissingDatabase(t *testing.T) {
	commonDir := filepath.Join(t.TempDir(), "repo", ".git")
	subject := subjectAt(t, commonDir)
	if err := os.MkdirAll(subject.Paths.DBPath, 0o755); err != nil {
		t.Fatalf("create directory at the database path: %v", err)
	}

	subject = Probe(subject)
	if subject.Probes.DBPath != storage.PresenceUnusable {
		t.Fatalf("probe reported presence %q, want %q", subject.Probes.DBPath, storage.PresenceUnusable)
	}

	db := SQLiteCheck(subject).Run(t.Context())
	assertDiagnosable(t, db)
	if db.State != StateError {
		t.Fatalf("sqlite reported %q for an occupied database path, want %q (%+v)",
			db.State, StateError, db)
	}
	if db.Code != app.CodeRuntimeDBUnavailable {
		t.Errorf("sqlite code = %q, want %q", db.Code, app.CodeRuntimeDBUnavailable)
	}
	if !strings.Contains(db.Diagnostic, "directory") {
		t.Errorf("diagnostic does not name the obstruction: %q", db.Diagnostic)
	}

	// The two readings that hang off the runtime store name the same cause
	// rather than each inventing one.
	for _, result := range []Result{
		MigrationCheck(subject).Run(t.Context()),
		WorkspaceCheck(subject).Run(t.Context()),
	} {
		assertDiagnosable(t, result)
		if result.Code == app.CodeWorkspaceNotInitialized {
			t.Errorf("check %q blames an uninitialised repository for an occupied path", result.Name)
		}
		for _, fabricated := range fabricatedClaims {
			if strings.Contains(result.Diagnostic, fabricated) {
				t.Errorf("check %q states %q about a store it could not read", result.Name, fabricated)
			}
		}
	}
}

// TestProbeOnAHealthyRepositoryStaysOK guards the other direction: the probe
// must not turn an ordinary uninitialised repository into a failure, because
// "run mindrail init" is the right answer there and decision D-01 depends on it
// being reachable.
func TestProbeOnAHealthyRepositoryStaysOK(t *testing.T) {
	commonDir := filepath.Join(t.TempDir(), "repo", ".git")
	if err := os.MkdirAll(commonDir, 0o755); err != nil {
		t.Fatalf("create common dir: %v", err)
	}

	subject := Probe(subjectAt(t, commonDir))

	if paths := RuntimePathCheck(subject).Run(t.Context()); paths.State != StateOK {
		t.Errorf("runtime_paths reported %q on a writable repository, want %q (%+v)",
			paths.State, StateOK, paths)
	}
	db := SQLiteCheck(subject).Run(t.Context())
	if db.Code != app.CodeWorkspaceNotInitialized {
		t.Errorf("sqlite code = %q, want %q: an empty path is still fixed by init", db.Code, app.CodeWorkspaceNotInitialized)
	}
}

// TestProbeIsIdempotentAndSkipsUnreachedSteps pins the two properties that keep
// the probe honest: it answers once, and it answers nothing at all about
// locations the startup sequence never derived — a zero Writability must never
// read as "unwritable".
func TestProbeIsIdempotentAndSkipsUnreachedSteps(t *testing.T) {
	halted := Probe(haltedSubject(stepLoadConfig))
	if halted.Probes.RuntimeDirKnown {
		t.Error("probed a runtime directory that was never derived")
	}
	if halted.Probes.DBPath != "" {
		t.Errorf("probed a database path that was never derived: %q", halted.Probes.DBPath)
	}

	// A subject that already carries its answers is passed through untouched,
	// which is what lets every other test in this package stay diskless.
	fixture := healthySubject()
	fixture.Probes.RuntimeDir.Dir = "/sentinel"
	if got := Probe(fixture).Probes.RuntimeDir.Dir; got != "/sentinel" {
		t.Errorf("Probe overwrote an already-probed subject: %q", got)
	}
}

// subjectAt builds a Subject for a real repository layout under commonDir, with
// the probe answers deliberately absent so the caller can take them.
func subjectAt(t *testing.T, commonDir string) Subject {
	t.Helper()

	worktreeRoot := filepath.Dir(commonDir)
	if err := os.MkdirAll(worktreeRoot, 0o755); err != nil {
		t.Fatalf("create worktree root: %v", err)
	}

	paths, err := filesystem.ResolveRuntimePaths(filesystem.PathOptions{
		CommonDir:    commonDir,
		WorktreeRoot: worktreeRoot,
	})
	if err != nil {
		t.Fatalf("resolve runtime paths: %v", err)
	}

	subject := healthySubject()
	subject.Repo.CommonDir = commonDir
	subject.Repo.GitDir = commonDir
	subject.Repo.WorktreeRoot = worktreeRoot
	subject.Paths = paths
	subject.DBPresent = false
	subject.Pragmas = storage.Pragmas{}
	subject.Migrations = nil
	subject.Probes = Probes{}
	return subject
}
