package bootstrap_test

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/bootstrap"
	"github.com/PsyChaos/mindrail/internal/git"
	"github.com/PsyChaos/mindrail/internal/storage"
)

// TestStartupStepOrderMatchesSpec pins the tech-stack §87 sequence.
//
// The order is asserted, not merely the outcome: a pipeline that opened SQLite
// before it knew which repository it was in would still produce a working
// database in the common case and would still be wrong, because every later
// milestone hangs its work off a stage of this sequence.
func TestStartupStepOrderMatchesSpec(t *testing.T) {
	tests := []struct {
		name string
		mode bootstrap.Mode
	}{
		{name: "init", mode: bootstrap.ModeInit},
		{name: "status", mode: bootstrap.ModeReadOnly},
		{name: "doctor", mode: bootstrap.ModeReadOnly},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := newGitRepo(t)
			recorder := &stepRecorder{}

			application := bootstrap.New(options(t, repo, tc.mode, func(o *bootstrap.Options) {
				o.Recorder = recorder
			}))
			t.Cleanup(func() { _ = application.Shutdown(context.Background()) })

			ran := false
			if err := application.Run(t.Context(), func(context.Context, *bootstrap.App) error {
				ran = true
				return nil
			}); err != nil {
				t.Fatalf("Run: %v", err)
			}
			if !ran {
				t.Fatal("Run did not call the command function")
			}

			want := bootstrap.Steps()
			got := recorder.steps()
			if len(got) != len(want) {
				t.Fatalf("recorded %v, want %v", got, want)
			}
			for i := range want {
				if got[i] != want[i] {
					t.Errorf("step %d = %q, want %q (full order %v)", i, got[i], want[i], got)
				}
			}
		})
	}
}

// TestNonRepoFailsAtStepOneAndWritesNothing proves fail-fast is real. Every
// later step needs a common-dir, so continuing past a failed discovery could
// only produce state in a location nobody chose.
func TestNonRepoFailsAtStepOneAndWritesNothing(t *testing.T) {
	requireGit(t)

	dir := t.TempDir()
	recorder := &stepRecorder{}

	application := bootstrap.New(options(t, dir, bootstrap.ModeInit, func(o *bootstrap.Options) {
		o.Recorder = recorder
	}))
	t.Cleanup(func() { _ = application.Shutdown(context.Background()) })

	err := application.Start(t.Context())
	if err == nil {
		t.Fatal("Start succeeded outside a repository")
	}
	if !errors.Is(err, git.ErrNotARepository) {
		t.Errorf("Start error = %v, want ErrNotARepository", err)
	}

	if got := recorder.steps(); len(got) != 1 || got[0] != bootstrap.StepResolveRepository {
		t.Errorf("recorded %v, want only %v", got, []bootstrap.Step{bootstrap.StepResolveRepository})
	}

	assertNoRuntimeState(t, dir)
}

// TestReadOnlyModeDoesNotCreateDatabase is decision D-01: status and doctor
// report a missing setup, and a read path that created the database would make
// the condition they exist to describe unobservable.
func TestReadOnlyModeDoesNotCreateDatabase(t *testing.T) {
	repo := newGitRepo(t)

	application := bootstrap.New(options(t, repo, bootstrap.ModeReadOnly, nil))
	t.Cleanup(func() { _ = application.Shutdown(context.Background()) })

	if err := application.Start(t.Context()); err != nil {
		t.Fatalf("Start in read-only mode: %v", err)
	}
	if application.DB() != nil {
		t.Error("DB() is non-nil for an uninitialized repository")
	}
	if application.Subject().DBPresent {
		t.Error("Subject reports DBPresent for an uninitialized repository")
	}

	assertNoRuntimeState(t, repo)
}

// TestShutdownClosesDBAndIsIdempotent covers the §88 drain: the pool is
// genuinely released, and a command that both defers a shutdown and calls one
// explicitly must not fail on the second.
func TestShutdownClosesDBAndIsIdempotent(t *testing.T) {
	repo := newGitRepo(t)

	application := bootstrap.New(options(t, repo, bootstrap.ModeInit, nil))
	if err := application.Start(t.Context()); err != nil {
		t.Fatalf("Start: %v", err)
	}

	db := application.DB()
	if db == nil {
		t.Fatal("DB() is nil after a successful init")
	}
	if err := db.PingContext(t.Context()); err != nil {
		t.Fatalf("Ping before shutdown: %v", err)
	}

	// A connection checked out before the drain is the only handle whose
	// closed-ness database/sql exposes as an exported value: a closed *sql.DB
	// reports an unexported "sql: database is closed", while a finished
	// *sql.Conn reports sql.ErrConnDone.
	conn, err := db.Conn(t.Context())
	if err != nil {
		t.Fatalf("check out a connection: %v", err)
	}

	if err := application.Shutdown(t.Context()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}

	if err := db.PingContext(t.Context()); err == nil {
		t.Error("Ping succeeded after shutdown; the pool was not released")
	}
	if err := conn.Close(); err != nil {
		t.Errorf("close the checked-out connection: %v", err)
	}
	if err := conn.PingContext(t.Context()); !errors.Is(err, sql.ErrConnDone) {
		t.Errorf("Ping on the returned connection = %v, want sql.ErrConnDone", err)
	}
	if open := db.Stats().OpenConnections; open != 0 {
		t.Errorf("%d connections still open after shutdown, want 0", open)
	}

	if err := application.Shutdown(t.Context()); err != nil {
		t.Errorf("second Shutdown = %v, want nil", err)
	}
}

// TestLazyManagersNotConstructedForStatus proves step 9 constructs nothing on
// its own, and that the guard around construction is a real one.
func TestLazyManagersNotConstructedForStatus(t *testing.T) {
	repo := newGitRepo(t)

	application := bootstrap.New(options(t, repo, bootstrap.ModeReadOnly, nil))
	t.Cleanup(func() { _ = application.Shutdown(context.Background()) })

	if err := application.Start(t.Context()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if got := application.ManagerInitCount(); got != 0 {
		t.Errorf("ManagerInitCount after start = %d, want 0", got)
	}

	const accessors = 16
	runners := make([]any, accessors)
	var wg sync.WaitGroup
	for i := range accessors {
		wg.Add(1)
		go func() {
			defer wg.Done()
			runners[i] = application.Doctor()
		}()
	}
	wg.Wait()

	if got := application.ManagerInitCount(); got != 1 {
		t.Errorf("ManagerInitCount after %d concurrent accessors = %d, want 1", accessors, got)
	}
	for i, runner := range runners {
		if runner != runners[0] {
			t.Fatalf("accessor %d got a different runner", i)
		}
	}
}

// TestNoTransactionOpenDuringGitOrFilesystemWork enforces tech-stack §21: a git
// subprocess or a filesystem walk must never run while a SQLite write
// transaction holds the write lock.
func TestNoTransactionOpenDuringGitOrFilesystemWork(t *testing.T) {
	repo := newGitRepo(t)

	probe := &txProbe{inner: git.NewExecRunner()}
	application := bootstrap.New(options(t, repo, bootstrap.ModeInit, func(o *bootstrap.Options) {
		o.Runner = probe
	}))
	t.Cleanup(func() { _ = application.Shutdown(context.Background()) })

	err := application.Run(t.Context(), func(ctx context.Context, a *bootstrap.App) error {
		if storage.TxActive(ctx) {
			t.Error("the command body runs inside a write transaction")
		}

		// A git call on the same context after the migrations have run: the
		// startup sequence opens write transactions in step 5, and this is what
		// fails if one of them is still on the context afterwards.
		if _, versionErr := git.NewAdapter(probe).Version(ctx); versionErr != nil {
			t.Errorf("git version after migration: %v", versionErr)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if probe.calls.Load() == 0 {
		t.Fatal("the git probe was never invoked, so it proved nothing")
	}
	if probe.insideTx.Load() {
		t.Error("a git invocation ran while a write transaction was open")
	}

	// Negative control: without this, a probe that always reported "no
	// transaction" would pass whatever the code did.
	db := application.DB()
	if db == nil {
		t.Fatal("DB() is nil after init")
	}
	detected := false
	if err := storage.InTx(t.Context(), db, func(txCtx context.Context, _ *sql.Tx) error {
		detected = storage.TxActive(txCtx)
		return nil
	}); err != nil {
		t.Fatalf("InTx: %v", err)
	}
	if !detected {
		t.Error("storage.TxActive did not detect an open transaction; the probe above is vacuous")
	}
}

// TestStartUsesInjectableRoots covers tech-stack §131. The suite must be able to
// run on a developer's machine without leaving anything in their home
// directory, which is only checkable by looking.
func TestStartUsesInjectableRoots(t *testing.T) {
	repo := newGitRepo(t)
	runtimeDir := filepath.Join(t.TempDir(), "runtime")
	cacheDir := filepath.Join(t.TempDir(), "cache")

	userCache := productDirUnder(t, os.UserCacheDir)
	userConfig := productDirUnder(t, os.UserConfigDir)
	cacheBefore := exists(userCache)
	configBefore := exists(userConfig)

	application := bootstrap.New(options(t, repo, bootstrap.ModeInit, func(o *bootstrap.Options) {
		o.RuntimeDirOverride = runtimeDir
		o.CacheDirOverride = cacheDir
	}))
	t.Cleanup(func() { _ = application.Shutdown(context.Background()) })

	if err := application.Start(t.Context()); err != nil {
		t.Fatalf("Start: %v", err)
	}

	paths := application.Paths()
	if paths.RuntimeRoot != runtimeDir {
		t.Errorf("RuntimeRoot = %q, want %q", paths.RuntimeRoot, runtimeDir)
	}
	if paths.CacheDir != cacheDir {
		t.Errorf("CacheDir = %q, want %q", paths.CacheDir, cacheDir)
	}
	if !strings.HasPrefix(paths.DBPath, runtimeDir+string(os.PathSeparator)) {
		t.Errorf("DBPath = %q, want it under %q", paths.DBPath, runtimeDir)
	}
	if _, err := os.Stat(paths.DBPath); err != nil {
		t.Errorf("stat injected database: %v", err)
	}
	assertNoRuntimeState(t, repo)

	if exists(userCache) != cacheBefore {
		t.Errorf("%s appeared or vanished; the run touched the user cache directory", userCache)
	}
	if exists(userConfig) != configBefore {
		t.Errorf("%s appeared or vanished; the run touched the user config directory", userConfig)
	}
}

// TestNoNetworkDuringStart backs acceptance criterion 1. A recording resolver
// and a transport that refuses every request turn "we do not think it dials"
// into a failure if it ever does.
func TestNoNetworkDuringStart(t *testing.T) {
	repo := newGitRepo(t)

	var dials atomic.Int64
	originalResolver := net.DefaultResolver
	net.DefaultResolver = &net.Resolver{
		PreferGo: true,
		Dial: func(context.Context, string, string) (net.Conn, error) {
			dials.Add(1)
			return nil, errors.New("network access is not allowed during startup")
		},
	}
	originalTransport := http.DefaultTransport
	http.DefaultTransport = refusingTransport{}
	t.Cleanup(func() {
		net.DefaultResolver = originalResolver
		http.DefaultTransport = originalTransport
	})

	application := bootstrap.New(options(t, repo, bootstrap.ModeInit, nil))
	t.Cleanup(func() { _ = application.Shutdown(context.Background()) })

	if err := application.Start(t.Context()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if got := dials.Load(); got != 0 {
		t.Errorf("startup performed %d DNS dials, want 0", got)
	}
}

// TestDiagnoseReattachesTheStartupCause covers the seam where the cause was
// being dropped.
//
// Commands derive their verdict from the doctor checks so that init, status and
// doctor can never report different codes for one broken installation. The check
// saw the failure as a Subject field and a code, though, not as the error the
// failing library returned, and the error rebuilt from that reading arrived at
// the user with no cause at all. Diagnose is what puts it back.
func TestDiagnoseReattachesTheStartupCause(t *testing.T) {
	repo := newGitRepo(t)

	// A database that is initialised and then damaged: startup fails at
	// open_sqlite with a cause from the storage layer, and every command reports
	// it through a check reading that never saw that cause.
	initialize(t, repo)
	corruptDatabase(t, repo)

	application := bootstrap.New(options(t, repo, bootstrap.ModeReadOnly, nil))
	t.Cleanup(func() { _ = application.Shutdown(context.Background()) })

	startErr := application.Start(t.Context())
	if startErr == nil {
		t.Fatal("startup succeeded against a damaged runtime database")
	}
	if app.CauseOf(startErr) == nil {
		t.Fatal("the startup error itself carries no cause; there is nothing to re-attach")
	}
	if application.StartError() != startErr {
		t.Errorf("StartError() = %v, want the error Start returned", application.StartError())
	}

	verdict := application.Doctor().Run(t.Context()).Err()
	if verdict == nil {
		t.Fatal("doctor reported no error for a startup that failed")
	}
	if app.CauseOf(verdict) != nil {
		t.Fatal("the doctor verdict already carries a cause; this test no longer covers the seam")
	}

	diagnosed := application.Diagnose(verdict)
	cause := app.CauseOf(diagnosed)
	if cause == nil {
		t.Fatalf("Diagnose left the verdict without a cause: %v", diagnosed)
	}
	if want := app.CauseOf(startErr).Error(); cause.Error() != want {
		t.Errorf("re-attached cause = %q, want %q", cause.Error(), want)
	}

	// The payload the user reads carries it too, which is the whole point:
	// before this, the string was reachable only by rebuilding the binary.
	payload, ok := app.PayloadOf(diagnosed)
	if !ok {
		t.Fatalf("diagnosed verdict carries no payload: %v", diagnosed)
	}
	if payload.Cause == "" {
		t.Errorf("payload carries no cause: %+v", payload)
	}
}

// TestDiagnoseRefusesAnUnrelatedCause is the guard that keeps the re-attachment
// from becoming a lie. Startup stops at the first hard failure while doctor
// reports all seven checks, so a verdict whose code differs from the startup
// error's is describing some other subsystem — and pasting an unrelated cause
// underneath it would be worse than having none.
func TestDiagnoseRefusesAnUnrelatedCause(t *testing.T) {
	repo := newGitRepo(t)

	application := bootstrap.New(options(t, repo, bootstrap.ModeReadOnly, nil))
	t.Cleanup(func() { _ = application.Shutdown(context.Background()) })

	if err := application.Start(t.Context()); err != nil {
		t.Fatalf("Start on a healthy repository: %v", err)
	}
	if application.StartError() != nil {
		t.Fatalf("StartError() = %v on a healthy repository", application.StartError())
	}

	unrelated := app.NewError(app.CodeMigrationFailed, app.KindFailed, "why", "impact", "next")
	if got := app.CauseOf(application.Diagnose(unrelated)); got != nil {
		t.Errorf("Diagnose invented a cause %v for an unrelated verdict", got)
	}
	if application.Diagnose(nil) != nil {
		t.Errorf("Diagnose(nil) = %v, want nil", application.Diagnose(nil))
	}
}

// initialize runs a real ModeInit start against repo, so a later read-only start
// meets a database that exists.
func initialize(t *testing.T, repo string) {
	t.Helper()

	application := bootstrap.New(options(t, repo, bootstrap.ModeInit, nil))
	if err := application.Start(t.Context()); err != nil {
		t.Fatalf("init %s: %v", repo, err)
	}
	if err := application.Shutdown(context.Background()); err != nil {
		t.Fatalf("shut down the init run: %v", err)
	}
}

// corruptDatabase replaces the runtime database with bytes that are not one. The
// write-ahead sidecars go too: leaving them behind would let SQLite recover the
// file and quietly disprove the scenario.
func corruptDatabase(t *testing.T, repo string) {
	t.Helper()

	dbPath := filepath.Join(repo, ".git", "mindrail", "mindrail.db")
	for _, suffix := range []string{"-wal", "-shm"} {
		if err := os.Remove(dbPath + suffix); err != nil && !os.IsNotExist(err) {
			t.Fatalf("remove %s%s: %v", dbPath, suffix, err)
		}
	}
	if err := os.WriteFile(dbPath, bytes.Repeat([]byte{0xde, 0xad, 0xbe, 0xef}, 1024), 0o600); err != nil {
		t.Fatalf("overwrite %s: %v", dbPath, err)
	}
}

// --- helpers ----------------------------------------------------------------

// options builds a hermetic Options: an explicit empty environment and an
// injected user config directory, so nothing the developer exported or wrote in
// their home directory can change the result.
func options(t *testing.T, startDir string, mode bootstrap.Mode, mutate func(*bootstrap.Options)) bootstrap.Options {
	t.Helper()

	opts := bootstrap.Options{
		StartDir:      startDir,
		Mode:          mode,
		Environ:       []string{},
		UserConfigDir: t.TempDir(),
	}
	if mutate != nil {
		mutate(&opts)
	}
	return opts
}

// stepRecorder captures the startup sequence. It is mutex-guarded because Start
// and the assertions can run on different goroutines under -race.
type stepRecorder struct {
	mu       sync.Mutex
	recorded []bootstrap.Step
}

func (r *stepRecorder) Step(s bootstrap.Step) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.recorded = append(r.recorded, s)
}

func (r *stepRecorder) steps() []bootstrap.Step {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]bootstrap.Step(nil), r.recorded...)
}

// txProbe wraps the real runner and records whether any git invocation ran
// while a write transaction was open on its context.
type txProbe struct {
	inner    git.CommandRunner
	calls    atomic.Int64
	insideTx atomic.Bool
}

func (p *txProbe) Run(ctx context.Context, dir string, args ...string) ([]byte, []byte, error) {
	p.calls.Add(1)
	if storage.TxActive(ctx) {
		p.insideTx.Store(true)
	}
	return p.inner.Run(ctx, dir, args...)
}

// refusingTransport fails every HTTP request rather than allowing one to
// escape, so a dial that slipped past the resolver hook still cannot succeed.
type refusingTransport struct{}

func (refusingTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("network access is not allowed during startup")
}

// newGitRepo creates a real temporary repository. Git-dependent behaviour is
// verified against git itself (tech-stack §92); faking rev-parse would only
// test the fake.
func newGitRepo(t *testing.T) string {
	t.Helper()
	requireGit(t)

	dir := t.TempDir()
	cmd := exec.Command("git", "init", "--quiet", dir)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
	return dir
}

func requireGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
}

// assertNoRuntimeState fails if anything Mindrail owns was created under root.
func assertNoRuntimeState(t *testing.T, root string) {
	t.Helper()

	forbidden := []string{"mindrail.db", "mindrail.db-wal", "mindrail.db-shm"}

	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name := entry.Name()
		if entry.IsDir() {
			if name == "mindrail" {
				t.Errorf("created runtime directory %s", path)
			}
			return nil
		}
		for _, bad := range forbidden {
			if name == bad {
				t.Errorf("created runtime file %s", path)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
}

func productDirUnder(t *testing.T, base func() (string, error)) string {
	t.Helper()

	dir, err := base()
	if err != nil {
		t.Skip("this host has no discoverable user directory")
	}
	return filepath.Join(dir, "mindrail")
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
