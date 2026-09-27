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
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/bootstrap"
	"github.com/PsyChaos/mindrail/internal/doctor"
	"github.com/PsyChaos/mindrail/internal/filesystem"
	"github.com/PsyChaos/mindrail/internal/git"
	"github.com/PsyChaos/mindrail/internal/index"
	"github.com/PsyChaos/mindrail/internal/index/inventory"
	"github.com/PsyChaos/mindrail/internal/index/parser"
	"github.com/PsyChaos/mindrail/internal/index/scheduler"
	"github.com/PsyChaos/mindrail/internal/index/snapshot"
	"github.com/PsyChaos/mindrail/internal/status"
	"github.com/PsyChaos/mindrail/internal/storage"
)

const testProjectID = "PRJ-TEST-01"

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

func TestStartupDiscoversInventoryAtTheExistingIndexStateStep(t *testing.T) {
	repo := newGitRepo(t)
	if err := os.MkdirAll(filepath.Join(repo, "pkg"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "pkg", "pyproject.toml"), []byte("[project]\nname = 'pkg'\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	recorder := &stepRecorder{}
	application := bootstrap.New(options(t, repo, bootstrap.ModeInit, func(o *bootstrap.Options) {
		o.Recorder = recorder
	}))
	t.Cleanup(func() { _ = application.Shutdown(context.Background()) })

	if err := application.Start(t.Context()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	subject := application.Subject()
	if !subject.InventoryObserved || len(subject.Inventory) != 1 {
		t.Fatalf("startup inventory = observed:%t units:%d, want observed one unit", subject.InventoryObserved, len(subject.Inventory))
	}
	if got := recorder.steps(); len(got) < 8 || got[7] != bootstrap.StepLoadIndexState {
		t.Fatalf("startup steps = %v, want discovery at %q", got, bootstrap.StepLoadIndexState)
	}
}

func TestReadOnlyStartupReportsPersistedInventoryWithoutWalkingSource(t *testing.T) {
	repo := newGitRepo(t)
	pkg := filepath.Join(repo, "pkg")
	if err := os.MkdirAll(pkg, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pkg, "package.json"), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	initialized := bootstrap.New(options(t, repo, bootstrap.ModeInit, nil))
	if err := initialized.Start(t.Context()); err != nil {
		t.Fatalf("init startup: %v", err)
	}
	if err := initialized.Shutdown(t.Context()); err != nil {
		t.Fatalf("close init database: %v", err)
	}
	if err := os.RemoveAll(pkg); err != nil {
		t.Fatal(err)
	}

	reader := bootstrap.New(options(t, repo, bootstrap.ModeReadOnly, nil))
	t.Cleanup(func() { _ = reader.Shutdown(context.Background()) })
	if err := reader.Start(t.Context()); err != nil {
		t.Fatalf("read-only startup: %v", err)
	}
	subject := reader.Subject()
	if !subject.InventoryObserved || len(subject.Inventory) != 1 || subject.Inventory[0].Kind != "javascript" {
		t.Fatalf("read-only inventory = %+v, observed=%t; want persisted JavaScript unit", subject.Inventory, subject.InventoryObserved)
	}
	report := status.Build(subject, time.Millisecond)
	if got := report.Components[status.ComponentInventory].Summary; got != "1 project units discovered" {
		t.Errorf("inventory summary = %q, want persisted unit count", got)
	}
	if got := report.Components[status.ComponentSyntax].Phase; got != "INVENTORY" {
		t.Errorf("syntax phase = %q, want INVENTORY", got)
	}
}

// TestReadOnlyStartupReportsIndexCensusWithoutWalkingSource is AC-06.4's
// structural proof: the census comes from persisted rows through one SQL
// aggregate, so files that do not exist on disk are still counted, and a
// read-only status writes nothing — no hashing, no walking, no row changes.
func TestReadOnlyStartupReportsIndexCensusWithoutWalkingSource(t *testing.T) {
	repo := newGitRepo(t)
	pkg := filepath.Join(repo, "pkg")
	if err := os.MkdirAll(pkg, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pkg, "pyproject.toml"), []byte("[project]\nname = 'pkg'\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	initialized := bootstrap.New(options(t, repo, bootstrap.ModeInit, nil))
	if err := initialized.Start(t.Context()); err != nil {
		t.Fatalf("init startup: %v", err)
	}
	units := initialized.Subject().Inventory
	if len(units) != 1 {
		t.Fatalf("startup inventory = %+v, want one unit", units)
	}
	store := index.NewStore(initialized.DB(), app.SystemClock{})
	ghostPending := filepath.Join(pkg, "ghost_pending.py")
	ghostFailed := filepath.Join(pkg, "ghost_failed.py")
	for _, seed := range []index.FileIndexState{
		{UnitID: units[0].ID, Path: ghostPending, Language: "python", State: index.StatePending},
		{UnitID: units[0].ID, Path: ghostFailed, Language: "python", State: index.StatePending},
	} {
		if err := store.UpsertFileState(t.Context(), seed); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := store.ReplaceFileFacts(t.Context(), index.FileFacts{
		ProjectID: testProjectID,
		UnitID:    units[0].ID, Path: ghostFailed, Language: "python",
		ContentHash: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		State:       index.StateFailed, LastError: "seeded failure",
	}); err != nil {
		t.Fatal(err)
	}
	var rowsBefore int
	if err := initialized.DB().QueryRowContext(t.Context(), `SELECT count(*) FROM file_index_state`).Scan(&rowsBefore); err != nil {
		t.Fatal(err)
	}
	if err := initialized.Shutdown(t.Context()); err != nil {
		t.Fatalf("close init database: %v", err)
	}
	// The sources never existed on disk; the census must still report them.
	if err := os.RemoveAll(pkg); err != nil {
		t.Fatal(err)
	}

	reader := bootstrap.New(options(t, repo, bootstrap.ModeReadOnly, nil))
	t.Cleanup(func() { _ = reader.Shutdown(context.Background()) })
	if err := reader.Start(t.Context()); err != nil {
		t.Fatalf("read-only startup: %v", err)
	}
	subject := reader.Subject()
	if !subject.IndexObserved {
		t.Fatal("read-only census not observed")
	}
	if got := subject.IndexCounts[index.StatePending]; got != 1 {
		t.Errorf("census pending = %d, want 1 ghost row", got)
	}
	if got := subject.IndexCounts[index.StateFailed]; got != 1 {
		t.Errorf("census failed = %d, want 1 ghost row", got)
	}
	var rowsAfter int
	if err := reader.DB().QueryRowContext(t.Context(), `SELECT count(*) FROM file_index_state`).Scan(&rowsAfter); err != nil {
		t.Fatal(err)
	}
	if rowsAfter != rowsBefore {
		t.Errorf("read-only startup changed %d rows to %d; the read path writes nothing", rowsBefore, rowsAfter)
	}
	report := status.Build(subject, time.Millisecond)
	if report.Readiness != status.ReadinessDegraded {
		t.Errorf("readiness = %q, want DEGRADED over the failed ghost row", report.Readiness)
	}
	syntax := report.Components[status.ComponentSyntax]
	if syntax.State != doctor.StateDegraded || syntax.Phase != "INDEXING" {
		t.Errorf("syntax component = %+v, want DEGRADED INDEXING", syntax)
	}
}

func TestLinkedWorktreeStatusDoesNotReportAnotherWorktreesInventory(t *testing.T) {
	repo := newGitRepo(t)
	if out, err := exec.Command("git", "-C", repo, "-c", "user.name=Mindrail", "-c", "user.email=test@example.invalid", "commit", "--quiet", "--allow-empty", "-m", "fixture").CombinedOutput(); err != nil {
		t.Fatalf("commit fixture: %v: %s", err, out)
	}
	linked := filepath.Join(t.TempDir(), "linked")
	if out, err := exec.Command("git", "-C", repo, "worktree", "add", "--quiet", "-b", "inventory-linked", linked).CombinedOutput(); err != nil {
		t.Fatalf("add linked worktree: %v: %s", err, out)
	}
	if err := os.WriteFile(filepath.Join(repo, "package.json"), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, root := range []string{repo, linked} {
		initialized := bootstrap.New(options(t, root, bootstrap.ModeInit, nil))
		if err := initialized.Start(t.Context()); err != nil {
			t.Fatalf("init %s: %v", root, err)
		}
		if err := initialized.Shutdown(t.Context()); err != nil {
			t.Fatalf("close init %s: %v", root, err)
		}
	}

	reader := bootstrap.New(options(t, linked, bootstrap.ModeReadOnly, nil))
	t.Cleanup(func() { _ = reader.Shutdown(context.Background()) })
	if err := reader.Start(t.Context()); err != nil {
		t.Fatalf("read linked worktree: %v", err)
	}
	if got := reader.Subject().Inventory; len(got) != 0 {
		t.Fatalf("linked worktree inventory includes foreign unit: %+v", got)
	}
}

func TestNestedRegisteredWorktreeIsExcludedFromParentInventoryAndPruning(t *testing.T) {
	repo := newGitRepo(t)
	if out, err := exec.Command("git", "-C", repo, "-c", "user.name=Mindrail", "-c", "user.email=test@example.invalid", "commit", "--quiet", "--allow-empty", "-m", "fixture").CombinedOutput(); err != nil {
		t.Fatalf("commit fixture: %v: %s", err, out)
	}
	nested := filepath.Join(repo, "nested")
	if out, err := exec.Command("git", "-C", repo, "worktree", "add", "--quiet", "-b", "inventory-nested", nested).CombinedOutput(); err != nil {
		t.Skipf("nested linked worktree unavailable: %v: %s", err, out)
	}
	if err := os.WriteFile(filepath.Join(nested, "package.json"), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	nestedInit := bootstrap.New(options(t, nested, bootstrap.ModeInit, nil))
	if err := nestedInit.Start(t.Context()); err != nil {
		t.Fatalf("init nested worktree: %v", err)
	}
	if err := nestedInit.Shutdown(t.Context()); err != nil {
		t.Fatalf("close nested init: %v", err)
	}

	parentInit := bootstrap.New(options(t, repo, bootstrap.ModeInit, nil))
	if err := parentInit.Start(t.Context()); err != nil {
		t.Fatalf("init parent worktree: %v", err)
	}
	if got := parentInit.Subject().Inventory; len(got) != 0 {
		t.Fatalf("parent inventory includes nested worktree unit: %+v", got)
	}
	if err := parentInit.Shutdown(t.Context()); err != nil {
		t.Fatalf("close parent init: %v", err)
	}

	nestedReader := bootstrap.New(options(t, nested, bootstrap.ModeReadOnly, nil))
	t.Cleanup(func() { _ = nestedReader.Shutdown(context.Background()) })
	if err := nestedReader.Start(t.Context()); err != nil {
		t.Fatalf("read nested worktree: %v", err)
	}
	if got := nestedReader.Subject().Inventory; len(got) != 1 || got[0].Path != nested {
		t.Fatalf("nested inventory was pruned or hidden: %+v", got)
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

// TestFlushReportsAFailedCheckpointRatherThanSwallowingIt is the propagation
// nothing asserted.
//
// Flush exists so that init's last write happens before the reading its verdict
// is taken from, which is only worth anything if a write that failed reaches the
// caller. Making Flush return nil on a checkpoint error passed the entire suite:
// the storage-level tests cover the checkpoint, and the command-level tests
// never produce a failing one, so the error's whole journey was unobserved.
//
// A closed database is the cheapest way to make the checkpoint fail for a reason
// that is neither contention nor cancellation, which are the two outcomes Flush
// is supposed to stay quiet about.
func TestFlushReportsAFailedCheckpointRatherThanSwallowingIt(t *testing.T) {
	repo := newGitRepo(t)

	application := bootstrap.New(options(t, repo, bootstrap.ModeInit, nil))
	t.Cleanup(func() { _ = application.Shutdown(context.Background()) })

	if err := application.Start(t.Context()); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// The healthy case first: a repository nothing is wrong with flushes clean,
	// or the assertion below would pass for a Flush that always failed.
	if err := application.Flush(t.Context()); err != nil {
		t.Fatalf("Flush on a healthy repository = %v, want no error", err)
	}

	db := application.DB()
	if db == nil {
		t.Fatal("DB() is nil after a successful init")
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close the database under the application: %v", err)
	}

	err := application.Flush(t.Context())
	if err == nil {
		t.Fatal("Flush = nil over a database it could not reach; init would report a repository it never finished writing as ready")
	}
	if _, ok := app.PayloadOf(err); !ok {
		t.Errorf("Flush = %v, which carries no domain payload for a command to render", err)
	}
}

// TestFlushIsQuietWhenThereIsNothingToWrite is the over-fire guard: the two
// outcomes that are not failures have to stay silent, or every read-only command
// and every interrupted one would start reporting a fault.
func TestFlushIsQuietWhenThereIsNothingToWrite(t *testing.T) {
	repo := newGitRepo(t)

	t.Run("a command that never opened a database", func(t *testing.T) {
		application := bootstrap.New(options(t, repo, bootstrap.ModeReadOnly, nil))
		t.Cleanup(func() { _ = application.Shutdown(context.Background()) })

		if err := application.Start(t.Context()); err != nil {
			t.Fatalf("Start in read-only mode: %v", err)
		}
		if application.DB() != nil {
			t.Fatal("the fixture opened a database, so it proves nothing")
		}
		if err := application.Flush(t.Context()); err != nil {
			t.Errorf("Flush with no database = %v, want no error", err)
		}
	})

	t.Run("a caller that was interrupted", func(t *testing.T) {
		application := bootstrap.New(options(t, newGitRepo(t), bootstrap.ModeInit, nil))
		t.Cleanup(func() { _ = application.Shutdown(context.Background()) })

		if err := application.Start(t.Context()); err != nil {
			t.Fatalf("Start: %v", err)
		}

		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		if err := application.Flush(ctx); err != nil {
			t.Errorf("Flush on a cancelled context = %v, want no error; nothing was found wrong", err)
		}
	})
}

// TestLargeInventoryColdIndexProof is AC-07.1 end to end: a synthetic
// repository of thousands of files across the three languages shows
// PARTIAL_READY with an explicit pending count while the cold index runs,
// init returning without waiting for it, the pending count draining to READY,
// a targeted unit moving ahead of the remainder, and every completed hash
// processed exactly once. Durations are logged for the record and asserted
// against the kernel-scope §5 STRUCTURAL budgets where the proof can afford
// the wall clock.
func TestLargeInventoryColdIndexProof(t *testing.T) {
	const filesPerUnit = 900
	repo := newGitRepo(t)
	units := map[string]struct {
		markers map[string]string
		ext     string
		body    func(i int) string
	}{
		"py": {
			markers: map[string]string{"pyproject.toml": "[project]\nname = 'pkg'\n"},
			ext:     ".py",
			body:    func(i int) string { return "def f" + itoa(i) + "():\n    return " + itoa(i) + "\n" },
		},
		"ts": {
			markers: map[string]string{"package.json": "{}\n", "tsconfig.json": "{}\n"},
			ext:     ".ts",
			body: func(i int) string {
				return "function f" + itoa(i) + "(x: number): number { return x + " + itoa(i) + "; }\n"
			},
		},
		"js": {
			markers: map[string]string{"package.json": "{}\n"},
			ext:     ".js",
			body:    func(i int) string { return "function f" + itoa(i) + "(x) { return x + " + itoa(i) + "; }\n" },
		},
	}
	for dir, unit := range units {
		pkg := filepath.Join(repo, dir)
		if err := os.MkdirAll(pkg, 0o755); err != nil {
			t.Fatal(err)
		}
		for name, content := range unit.markers {
			if err := os.WriteFile(filepath.Join(pkg, name), []byte(content), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		for i := range filesPerUnit {
			name := "f" + itoa(i) + unit.ext
			if err := os.WriteFile(filepath.Join(pkg, name), []byte(unit.body(i)), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	const total = 3 * filesPerUnit

	// Init must return without waiting for the cold index: thousands of
	// files are pending afterwards and none is indexed.
	initialized := bootstrap.New(options(t, repo, bootstrap.ModeInit, nil))
	started := time.Now()
	if err := initialized.Start(t.Context()); err != nil {
		t.Fatalf("init startup: %v", err)
	}
	initElapsed := time.Since(started)
	discovered := initialized.Subject().Inventory
	if len(discovered) != 3 {
		t.Fatalf("discovered %d units, want 3", len(discovered))
	}
	// Init registers nothing: a synchronously-blocking init would leave file
	// rows behind, so an empty file_index_state is the non-waiting proof —
	// stronger than any wall-clock bound.
	var fileRows int
	if err := initialized.DB().QueryRowContext(t.Context(), `SELECT count(*) FROM file_index_state`).Scan(&fileRows); err != nil {
		t.Fatal(err)
	}
	if fileRows != 0 {
		t.Fatalf("init left %d file rows; it must not wait for the cold index", fileRows)
	}
	if err := initialized.Shutdown(t.Context()); err != nil {
		t.Fatalf("close init database: %v", err)
	}
	t.Logf("init over %d files: %v", total, initElapsed)
	if initElapsed > 5*time.Second {
		t.Fatalf("init took %v over %d unindexed files; it must not wait for the cold index", initElapsed, total)
	}

	// PARTIAL_READY with the explicit pending count while the cold runs.
	// Registration below is scheduler work, so seed it here through the
	// write-mode database before the read-only status.
	driver := bootstrap.New(options(t, repo, bootstrap.ModeWrite, nil))
	t.Cleanup(func() { _ = driver.Shutdown(context.Background()) })
	if err := driver.Start(t.Context()); err != nil {
		t.Fatalf("driver startup: %v", err)
	}
	store := index.NewStore(driver.DB(), app.SystemClock{})
	registry, err := parser.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(registry.Close)
	sched, err := scheduler.New(store, index.NewIndexer(store, registry, snapshot.New(filesystem.RuntimePaths{CacheDir: filepath.Join(t.TempDir(), "cache")})), 0)
	if err != nil {
		t.Fatal(err)
	}
	walked, err := inventory.Files(t.Context(), repo, discovered)
	if err != nil {
		t.Fatal(err)
	}
	registered, unsupported, err := sched.Register(t.Context(), registry, walked)
	if err != nil {
		t.Fatal(err)
	}
	if registered != total || unsupported != 0 || len(walked) != total {
		t.Fatalf("walked %d registered %d unsupported %d, want %d/0", len(walked), registered, unsupported, total)
	}
	if err := driver.Shutdown(t.Context()); err != nil {
		t.Fatal(err)
	}

	reader := bootstrap.New(options(t, repo, bootstrap.ModeReadOnly, nil))
	t.Cleanup(func() { _ = reader.Shutdown(context.Background()) })
	if err := reader.Start(t.Context()); err != nil {
		t.Fatalf("read-only startup: %v", err)
	}
	subject := reader.Subject()
	if !subject.IndexObserved || subject.IndexCounts[index.StatePending] != total {
		t.Fatalf("census = %+v, want %d pending", subject.IndexCounts, total)
	}
	report := status.Build(subject, time.Millisecond)
	if report.Readiness != status.ReadinessPartialReady {
		t.Fatalf("readiness = %q, want PARTIAL_READY with %d files pending", report.Readiness, total)
	}
	if got := report.Components[status.ComponentSyntax].Pending; got == nil || *got != total {
		t.Fatalf("syntax pending = %v, want %d", got, total)
	}

	// Preemption moves the targeted unit ahead of the cold remainder.
	worker := bootstrap.New(options(t, repo, bootstrap.ModeWrite, nil))
	t.Cleanup(func() { _ = worker.Shutdown(context.Background()) })
	if err := worker.Start(t.Context()); err != nil {
		t.Fatalf("worker startup: %v", err)
	}
	wstore := index.NewStore(worker.DB(), app.SystemClock{})
	wsched, err := scheduler.New(wstore, index.NewIndexer(wstore, registry, snapshot.New(filesystem.RuntimePaths{CacheDir: filepath.Join(t.TempDir(), "cache2")})), 0)
	if err != nil {
		t.Fatal(err)
	}
	enqueued, more, err := wsched.FillCold(t.Context(), testProjectID, discovered)
	if err != nil {
		t.Fatal(err)
	}
	if enqueued != scheduler.MaxQueueJobs || !more {
		t.Fatalf("cold fill = %d/%t, want %d/true", enqueued, more, scheduler.MaxQueueJobs)
	}
	var tsID string
	for _, unit := range discovered {
		if unit.Kind == index.UnitTypeScript {
			tsID = unit.ID
		}
	}
	if tsID == "" {
		t.Fatal("no TypeScript unit discovered")
	}
	moved, err := wsched.Prioritize(t.Context(), testProjectID, discovered, tsID)
	if err != nil {
		t.Fatal(err)
	}
	if moved != filesPerUnit {
		t.Fatalf("prioritized %d files, want %d", moved, filesPerUnit)
	}
	tsRoot := filepath.Join(repo, "ts") + string(filepath.Separator)
	for i, path := range wsched.Paths()[:filesPerUnit] {
		if len(path) < len(tsRoot) || path[:len(tsRoot)] != tsRoot {
			t.Fatalf("queue[%d] = %s, want the targeted unit first", i, path)
		}
	}
	t.Logf("cold window %d (more=%t), prioritized %d TypeScript files first", enqueued, more, moved)

	// Drain to READY through refill windows; every completed hash is
	// processed exactly once (attempts stays 1).
	completed := 0
	for {
		n, more, err := wsched.FillCold(t.Context(), testProjectID, discovered)
		if err != nil {
			t.Fatal(err)
		}
		if n == 0 && !more {
			break
		}
		done, err := wsched.Run(t.Context())
		if err != nil {
			t.Fatalf("drain run: %v", err)
		}
		completed += done
	}
	if completed != total {
		t.Fatalf("drained %d files, want %d", completed, total)
	}
	var attemptRows, maxAttempts int
	if err := worker.DB().QueryRowContext(t.Context(), `SELECT count(*), max(attempts) FROM file_index_state`).Scan(&attemptRows, &maxAttempts); err != nil {
		t.Fatal(err)
	}
	if attemptRows != total || maxAttempts != 1 {
		t.Fatalf("rows %d max attempts %d, want %d rows at 1 attempt each (no reprocessing)", attemptRows, maxAttempts, total)
	}
	// Parse reality: state transitions alone could be faked by flipping rows
	// to indexed with empty facts. Every file defines exactly one function,
	// so the drain must leave exactly one symbol row per file.
	var symbols int
	if err := worker.DB().QueryRowContext(t.Context(), `SELECT count(*) FROM symbols`).Scan(&symbols); err != nil {
		t.Fatal(err)
	}
	if symbols != total {
		t.Fatalf("symbols = %d, want %d (one extracted function per file)", symbols, total)
	}
	if err := worker.Shutdown(t.Context()); err != nil {
		t.Fatal(err)
	}

	final := bootstrap.New(options(t, repo, bootstrap.ModeReadOnly, nil))
	t.Cleanup(func() { _ = final.Shutdown(context.Background()) })
	if err := final.Start(t.Context()); err != nil {
		t.Fatalf("final startup: %v", err)
	}
	if got := status.Build(final.Subject(), time.Millisecond).Readiness; got != status.ReadinessReady {
		t.Fatalf("readiness = %q after the drain, want READY", got)
	}

	// Warm-path SLO reading on the drained index: status answers from
	// persisted rows, so every sample must clear the §5 STRUCTURAL budget.
	subject = final.Subject()
	var worst time.Duration
	for range 20 {
		started := time.Now()
		status.Build(subject, time.Millisecond)
		if elapsed := time.Since(started); elapsed > worst {
			worst = elapsed
		}
	}
	t.Logf("status over %d indexed files: worst of 20 builds %v (budget 150ms)", total, worst)
	if worst > 150*time.Millisecond {
		t.Fatalf("status worst %v exceeds the 150ms STRUCTURAL budget", worst)
	}
}

func itoa(i int) string {
	return strconv.Itoa(i)
}
