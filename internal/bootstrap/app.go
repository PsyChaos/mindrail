// Package bootstrap runs the tech-stack §87 startup sequence once per process
// and hands the result to whichever command asked for it.
//
// It is the only package that knows every subsystem, which is deliberate: the
// alternative is each command wiring its own half of the tree and drifting from
// the others about what "started" means. Two rules keep that concentration
// honest.
//
// Nothing here decides policy. Every seam — the clock, the git runner, the
// runtime roots, the migration and schema filesystems — is a constructor
// parameter, so there is no ambient state for a test to fight (decision D-34).
//
// A failed start is still a described start. Each step records what it found
// and the error it hit into the doctor.Subject before returning, so `doctor`
// and `status` can report a half-broken installation instead of dying with it
// (acceptance criterion 4).
package bootstrap

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/config"
	"github.com/PsyChaos/mindrail/internal/coordination"
	"github.com/PsyChaos/mindrail/internal/doctor"
	"github.com/PsyChaos/mindrail/internal/filesystem"
	"github.com/PsyChaos/mindrail/internal/git"
	"github.com/PsyChaos/mindrail/internal/knowledge/loader"
	"github.com/PsyChaos/mindrail/internal/knowledge/schema"
	"github.com/PsyChaos/mindrail/internal/knowledge/validate"
	"github.com/PsyChaos/mindrail/internal/migration"
	"github.com/PsyChaos/mindrail/internal/storage"
	"github.com/PsyChaos/mindrail/internal/workspace"
	"github.com/PsyChaos/mindrail/migrations"
	"github.com/PsyChaos/mindrail/schemas"
)

// Mode decides whether this process is allowed to change anything.
//
// Decision D-01 gives exactly one command write authority. `status` and
// `doctor` exist to report a missing or broken setup, and a read path that
// quietly created the database would make the very condition they are asked
// about unobservable.
type Mode int

const (
	// ModeReadOnly never creates, never migrates and opens SQLite read-only:
	// status, doctor, version.
	ModeReadOnly Mode = iota
	// ModeInit is the only mode that creates the database, applies migrations
	// and registers the worktree.
	ModeInit
	// ModeWrite opens an existing database for writing without creating or
	// migrating it: the coordination commands MR-003 adds, and every later
	// milestone that records something into a repository `init` has already set
	// up.
	//
	// It exists because the two original modes did not divide the authority the
	// way decision D-01 does. D-01 gives *setup* authority to `mindrail init`
	// alone — creating the file, applying migrations, registering the worktree —
	// and a command that appends a row to a table init already made is not
	// exercising any of it. Folding those two into one mode meant a coordination
	// command either opened the database read-only, and could not write at all,
	// or ran as init and would create a database out from under a user who had
	// never run it.
	//
	// So this mode is read-only in everything except the SQLite handle: a
	// repository with no database is reported as such, pending migrations are
	// read and not applied, and an unregistered worktree stays unregistered.
	ModeWrite
)

// String renders the mode for logs and diagnostics.
func (m Mode) String() string {
	switch m {
	case ModeInit:
		return "init"
	case ModeWrite:
		return "write"
	default:
		return "read-only"
	}
}

// creates reports whether this mode may bring a runtime database into existence.
// Only init may, which is decision D-01 as a predicate rather than as a
// comparison repeated at four call sites.
func (m Mode) creates() bool { return m == ModeInit }

// Options carries every input the startup sequence has. A zero value is usable:
// it starts read-only in the process working directory against the real git,
// the real clock and the embedded assets.
type Options struct {
	StartDir           string
	Mode               Mode
	Runner             git.CommandRunner // nil => git.NewExecRunner()
	Clock              app.Clock         // nil => app.SystemClock{}
	Logger             *slog.Logger      // nil => slog.New(discard)
	Environ            []string          // nil => os.Environ()
	Flags              map[string]string
	UserConfigDir      string
	RuntimeDirOverride string
	CacheDirOverride   string
	MigrationFS        fs.FS // nil => migrations.FS
	SchemaFS           fs.FS // nil => schemas.KnowledgeFS
	Recorder           Recorder
}

// InitResult carries what only ModeInit produces. It is separate from
// doctor.Subject because a subject describes what was found, while these four
// fields describe what was changed — and spec §82's "existing config is never
// silently overwritten" is only verifiable if the two are told apart.
type InitResult struct {
	ConfigPath           string
	ConfigCreated        bool
	KnowledgeDirsCreated []string
	MigrationsApplied    []migration.Applied

	// ConfigPresent and KnowledgeDirsPresent record that the scaffold step ran
	// to completion, so the file and the directories are on disk whether or not
	// this run is the one that put them there. They are separate from the two
	// "created" fields because an empty create list has two meanings — nothing
	// needed doing, or nothing was attempted — and only the first of them
	// licenses `init` to report the scaffold as present (finding H13).
	ConfigPresent        bool
	KnowledgeDirsPresent bool
}

// App is one started (or half-started) Mindrail process.
type App struct {
	opts    Options
	clock   app.Clock
	logger  *slog.Logger
	runner  git.CommandRunner
	environ []string

	startOnce sync.Once
	startErr  error

	subject    doctor.Subject
	db         *storage.DB
	warnings   []app.Warning
	initResult InitResult

	doctorOnce    sync.Once
	doctorRunner  *doctor.Runner
	doctorSubject doctor.Subject
	managerInits  atomic.Int64

	shutdownOnce sync.Once
	shutdownErr  error
}

// New captures the options and resolves the defaults. It performs no I/O, so a
// command can construct an App, register its shutdown, and only then decide
// whether to start it.
func New(o Options) *App {
	a := &App{
		opts:    o,
		clock:   o.Clock,
		logger:  o.Logger,
		runner:  o.Runner,
		environ: o.Environ,
	}

	if a.clock == nil {
		a.clock = app.SystemClock{}
	}
	if a.logger == nil {
		a.logger = slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))
	}
	if a.runner == nil {
		a.runner = git.NewExecRunner()
	}
	if a.environ == nil {
		a.environ = os.Environ()
	}
	a.initResult.KnowledgeDirsCreated = []string{}
	a.initResult.MigrationsApplied = []migration.Applied{}

	return a
}

// Start executes steps 1-9 of tech-stack §87.
//
// It stops at the first hard failure and returns an *app.DomainError; partial
// state stays reachable through Subject(), which is what lets doctor describe
// the failure instead of merely inheriting it. Start is idempotent: the second
// call returns the first call's verdict rather than re-running the sequence.
func (a *App) Start(ctx context.Context) error {
	a.startOnce.Do(func() { a.startErr = a.start(ctx) })
	return a.startErr
}

func (a *App) start(ctx context.Context) error {
	started := time.Now()

	for _, step := range []func(context.Context) error{
		a.resolveRepository,
		a.loadConfig,
		a.resolveRuntimePaths,
		a.openSQLite,
		a.migrateDB,
		a.validateKnowledge,
		a.registerWorkspace,
	} {
		if err := step(ctx); err != nil {
			// The cause goes on the record beside the domain sentence, not
			// instead of it. The sentence is what the user acts on; the cause is
			// what tells a maintainer whether the sentence was even the right
			// one, and it is unreachable from every other output surface once a
			// check has re-derived the verdict from the Subject.
			attrs := []any{
				slog.String("error", err.Error()),
				slog.Duration("duration", time.Since(started)),
			}
			if cause := app.CauseOf(err); cause != nil {
				attrs = append(attrs, slog.String("cause", cause.Error()))
			}
			a.logger.Warn("startup stopped", attrs...)
			return err
		}
	}

	// Index state belongs to MR-005. The step is still announced because §87's
	// sequence is the contract every later milestone slots into, and a stage
	// that silently disappeared while it had no work would have to be argued
	// back in later.
	a.record(StepLoadIndexState)

	// Managers are constructed on first use, not here (see lazy.go). The step
	// marks the point in the sequence where that becomes legal.
	a.record(StepInitManagers)

	a.logger.Debug("startup complete", slog.Duration("duration", time.Since(started)))
	return nil
}

// Run starts the application, announces the execute_command step and calls fn.
//
// fn runs even when the start failed. Every MR-001 command has something to say
// about a broken installation — that is what acceptance criterion 4 asks for —
// so the decision about whether a partial start is fatal belongs to the command,
// which can see the whole Subject, rather than to this function, which cannot.
//
// fn's verdict is the only error Run returns. It used to fall back to the start
// error when fn reported none, which is what made the process exit code and the
// JSON envelope disagree: the envelope was written from fn's verdict and the
// exit code from the start error, so a missing git produced {"ok": true} with no
// data and exit 4 (finding F11). A command that wants the start error in its
// verdict asks Diagnosis, which cannot miss it — doctor.Verdict derives it from
// the same Subject the report is built from.
func (a *App) Run(ctx context.Context, fn func(context.Context, *App) error) error {
	startErr := a.Start(ctx)

	a.record(StepExecuteCommand)
	if fn == nil {
		return startErr
	}
	return fn(ctx, a)
}

// Diagnosis runs the health checks and returns both the report and the one
// error every output surface is derived from.
//
// It is the single decider finding F11 asks for. `ok` is false exactly when the
// second result is non-nil, the envelope's `error` object is that error's
// payload, and the process exit code is app.ExitCode of it — three answers from
// one value, so they cannot contradict each other whatever the failure is.
func (a *App) Diagnosis(ctx context.Context) (doctor.Report, error) {
	report := a.Doctor().Run(ctx)
	return report, a.Diagnose(doctor.Verdict(a.doctorSubject, report))
}

// Flush makes this command's last write happen before its verdict is taken,
// rather than after.
//
// It is here rather than folded into Shutdown because the ordering is the whole
// point. Shutdown runs after the report has been rendered, and closing a WAL
// database is itself a write — SQLite checkpoints the log into the main file —
// so a command whose only remaining write was its own teardown published a
// reading of a disk it was about to change. On a filesystem with room for the
// log and not for the checkpoint, `mindrail init` printed READY FOR TARGETED
// WORK and exit 0 over a repository that `status`, run a moment later on bytes
// nothing else had touched, refused at exit 4 (finding F01).
//
// A command that never opened a database has nothing to flush and gets nil. A
// checkpoint another connection is holding up is reported as a warning rather
// than a verdict: the log stays where it is, the next writer moves it, and
// nothing about the repository is wrong.
func (a *App) Flush(ctx context.Context) error {
	db := a.DB()
	if db == nil {
		return nil
	}

	result, err := storage.Checkpoint(ctx, db)
	if err != nil {
		return err
	}
	if result.Busy {
		a.logger.Debug("write-ahead log left in place",
			slog.Int("log_frames", result.LogFrames),
			slog.Int("checkpointed_frames", result.CheckpointedFrames))
	}
	return nil
}

// errShutdownTimeout reports a shutdown that outlived its budget. It is a
// distinct value so a caller can tell "the database refused to close" from "we
// stopped waiting for it".
var errShutdownTimeout = errors.New("shutdown did not complete within the bounded time")

// Shutdown releases everything Start acquired, within app.ShutdownTimeout
// (tech-stack §88, decision D-08). It is idempotent, and it deliberately does
// not inherit ctx's cancellation: a process shutting down because it was
// interrupted still has to close SQLite cleanly, and a context that is already
// cancelled would abort exactly the work that matters most.
func (a *App) Shutdown(ctx context.Context) error {
	a.shutdownOnce.Do(func() {
		if a.db == nil {
			return
		}

		done := make(chan error, 1)
		go func() { done <- a.db.Close() }()

		timer := time.NewTimer(app.ShutdownTimeout)
		defer timer.Stop()

		select {
		case err := <-done:
			a.shutdownErr = err
		case <-timer.C:
			a.shutdownErr = errShutdownTimeout
		}

		if a.shutdownErr != nil {
			a.logger.Warn("shutdown incomplete", slog.String("error", a.shutdownErr.Error()))
		}
	})
	return a.shutdownErr
}

// Subject returns the resolved state, errors included. The copy is deliberate:
// checks are pure functions of a subject, and handing out the live one would
// let a check mutate what a later check reads.
func (a *App) Subject() doctor.Subject { return a.subject }

// StartError returns the hard failure that stopped the §87 sequence, or nil.
//
// It is the only value in the process that still holds the cause chain: a health
// check reports a *reading*, and the error rebuilt from that reading carries the
// code and the remedy but not the driver or syscall message underneath.
func (a *App) StartError() error { return a.startErr }

// Diagnose returns verdict with the cause of the matching original failure
// re-attached.
//
// Commands derive their verdict from the doctor checks rather than from the
// start error, so that init, status and doctor can never report different codes
// for one broken installation. The cost of that indirection is the cause: the
// check saw the failure as a Subject field and a code, not as the error the
// failing library returned. Re-attaching it here is what makes `--verbose` able
// to answer "the runtime database could not be opened — but why?" with
// `unable to open database file (14)` instead of nothing.
//
// The codes must agree. Startup stops at the first hard failure while doctor
// reports all seven checks, so a mismatch means the verdict is describing some
// other subsystem, and pasting an unrelated cause underneath it would be worse
// than having none.
func (a *App) Diagnose(verdict error) error {
	verdictPayload, ok := app.PayloadOf(verdict)
	if !ok {
		return verdict
	}

	for _, source := range a.diagnosedFailures() {
		payload, ok := app.PayloadOf(source)
		if !ok || payload.Code != verdictPayload.Code {
			continue
		}
		if cause := app.CauseOf(source); cause != nil {
			return app.AdoptCause(verdict, cause)
		}
	}
	return verdict
}

// diagnosedFailures lists the errors this process still holds with their cause
// chains intact, most authoritative first.
//
// The start error is one of them, but not the only one: a runtime directory
// nothing can write, a database path occupied by a directory and a worktree that
// will not accept the repository scaffolding are all invisible to startup — a
// read-only start treats an unopenable database as a repository that was never
// initialised — and are established instead by doctor's read-only probe.
//
// Order is authority, and the list holds only probes whose failure the report
// actually claims. The write-access probe is deliberately absent: it reports a
// runtime directory that does not exist yet as "not writable", which is true of
// every uninitialised repository and is not a condition any check publishes, so
// listing it handed an unrelated RUNTIME_PATH_UNWRITABLE verdict a cause about a
// database path that was perfectly fine. Where that probe's answer *is* claimed,
// the check carries its payload directly.
//
// The knowledge-subtree probe is on the list for the same reason the repository
// config directory is: its answer became the one the report publishes for an
// unusable `.mindrail/knowledge` (finding E10), and a claimed verdict with no
// cause chain is one nobody can tell apart from any other unwritable path.
func (a *App) diagnosedFailures() []error {
	probes := a.doctorSubject.Probes

	failures := make([]error, 0, 5)
	if a.startErr != nil {
		failures = append(failures, a.startErr)
	}
	if probes.DBPathErr != nil {
		failures = append(failures, probes.DBPathErr)
	}
	for _, err := range []error{probes.RuntimeDir.Err, probes.RepoConfigDir.Err, probes.KnowledgeDir.Err} {
		if err != nil {
			failures = append(failures, err)
		}
	}
	return failures
}

// DB returns the runtime handle, or nil when there is none — which in
// ModeReadOnly means the repository has never been initialised (decision D-01).
func (a *App) DB() *sql.DB {
	if a.db == nil {
		return nil
	}
	return a.db.DB
}

// Coordination returns the session/task/checkpoint store, or nil when there is
// no runtime database to build it over.
//
// It is constructed here rather than by each command (requirement AC-08.1) for
// the reason every other seam in this file is: a command that built its own
// would be free to hand it a different clock, and two commands disagreeing about
// what time it is would write rows whose order does not match the order they
// happened in.
//
// nil rather than an error, because "there is no database" is not a failure of
// this call. In ModeReadOnly it means the repository has never been initialised
// (decision D-01), which is a state the caller reports with `mindrail init` as
// the remedy — the same shape DB() above already has.
func (a *App) Coordination() *coordination.Store {
	if a.db == nil {
		return nil
	}
	return coordination.NewStore(a.db.DB, a.clock)
}

// Repo returns the resolved Git layout, zero when discovery failed.
func (a *App) Repo() git.Repository { return a.subject.Repo }

// Paths returns the derived runtime locations, zero when they were never
// derived.
func (a *App) Paths() filesystem.RuntimePaths { return a.subject.Paths }

// Config returns the merged configuration and its provenance.
func (a *App) Config() config.Loaded { return a.subject.Config }

// Warnings returns the non-fatal conditions seen during startup. They never
// change the exit code; that is what makes them warnings.
func (a *App) Warnings() []app.Warning { return a.warnings }

// InitResult returns what ModeInit changed. In ModeReadOnly the slices are
// empty rather than nil, so a JSON payload built from it never carries a null
// where a consumer expects an array.
func (a *App) InitResult() InitResult { return a.initResult }

// --- steps -----------------------------------------------------------------

// resolveRepository is step 1 and the only unconditional prerequisite: every
// later step needs a common-dir or a worktree root, so failing here has to stop
// the sequence rather than let six steps report the same missing answer.
func (a *App) resolveRepository(ctx context.Context) error {
	a.record(StepResolveRepository)

	a.subject.StartDir = a.startDir()

	adapter := git.NewAdapter(a.runner)
	repo, err := adapter.Resolve(ctx, a.subject.StartDir)
	if err != nil {
		a.subject.RepoErr = asRepositoryError(err)
		return a.subject.RepoErr
	}
	a.subject.Repo = repo

	// A git that resolved the layout but cannot report its own version is a
	// curiosity, not a failure: the version is reported to a human and nothing
	// branches on it.
	if version, versionErr := adapter.Version(ctx); versionErr == nil {
		a.subject.GitVersion = version
	} else {
		a.logger.Debug("git version unavailable", slog.String("error", versionErr.Error()))
	}

	return nil
}

// loadConfig is step 2. In ModeInit it also lays down the repository
// scaffolding first, so that the configuration this process runs on is the one
// the repository will keep — loading before writing would run init against a
// file that does not exist yet and report a provenance nobody can reproduce.
func (a *App) loadConfig(context.Context) error {
	a.record(StepLoadConfig)

	worktreeRoot := a.subject.Repo.WorktreeRoot

	if a.opts.Mode == ModeInit {
		// Asked before anything is written, and asked with the same function the
		// loader asks below, because the two used to answer differently about one
		// disk. A dangling `.mindrail` symlink pointing inside the worktree is
		// refused by the read path — `status` and `doctor` exit 4 with "remove or
		// repoint the link" — while the write path resolved through it and
		// silently created the link's target, laid the whole scaffold down there
		// and reported success at exit 0. One condition, two opposite verdicts,
		// decided by which command the reader happened to run.
		//
		// Refusing before acting rather than after is the half that matters:
		// materialising a broken link is not something a later error message can
		// take back.
		if worktreeRoot != "" {
			repoDir := filepath.Join(worktreeRoot, config.RepoDir)
			if obstruction := filesystem.ObstructedDir(filesystem.RootRepository, repoDir); obstruction != nil {
				a.initResult.ConfigPath = filepath.Join(repoDir, config.ConfigFileName)
				a.subject.ConfigErr = obstruction
				return obstruction
			}
		}

		path, created, err := config.WriteIfAbsent(worktreeRoot)
		a.initResult.ConfigPath = path
		a.initResult.ConfigCreated = created
		if err != nil {
			a.subject.ConfigErr = err
			return err
		}
		// WriteIfAbsent returned, so the file is there: this run wrote it, or it
		// was already there and was left alone (spec §82).
		a.initResult.ConfigPresent = true

		dirs, err := config.EnsureKnowledgeDirs(worktreeRoot)
		if err != nil {
			a.subject.ConfigErr = err
			return err
		}
		a.initResult.KnowledgeDirsCreated = dirs
		a.initResult.KnowledgeDirsPresent = true
	} else if worktreeRoot != "" {
		a.initResult.ConfigPath = filepath.Join(worktreeRoot, config.RepoDir, config.ConfigFileName)
	}

	loaded, err := config.NewLoader(config.LoaderOptions{
		WorktreeRoot:  worktreeRoot,
		UserConfigDir: a.opts.UserConfigDir,
		Environ:       a.environ,
		Flags:         a.opts.Flags,
	}).Load()
	if err != nil {
		a.subject.ConfigErr = err
		return err
	}

	a.subject.Config = loaded
	a.warnings = loaded.Warnings
	for _, warning := range loaded.Warnings {
		a.logger.Warn(warning.Message, slog.String("code", string(warning.Code)))
	}

	return nil
}

// resolveRuntimePaths is step 3. Deriving the locations is a pure computation
// and happens in both modes; creating them is a write and happens only in
// ModeInit (decision D-01).
func (a *App) resolveRuntimePaths(context.Context) error {
	a.record(StepResolveRuntimePaths)

	paths, err := filesystem.ResolveRuntimePaths(filesystem.PathOptions{
		CommonDir:          a.subject.Repo.CommonDir,
		WorktreeRoot:       a.subject.Repo.WorktreeRoot,
		RuntimeDirOverride: a.runtimeDirOverride(),
		CacheDirOverride:   a.cacheDirOverride(),
	})
	if err != nil {
		a.subject.PathsErr = asRuntimePathError(err)
		return a.subject.PathsErr
	}
	a.subject.Paths = paths

	if a.opts.Mode == ModeInit {
		if err := paths.EnsureDirs(); err != nil {
			// Which root failed decides whether the run can continue. The
			// runtime root holds the database every command reads, so failing to
			// create it stops the sequence. The cache directory holds derived
			// data MR-001 never writes, so failing to create it is recorded,
			// reported as DEGRADED, and nothing more: treating the two alike
			// aborted `init` at step 3 on a repository that was otherwise
			// perfectly able to finish (finding F13).
			if kind, known := filesystem.RootKindOf(err); known && kind == filesystem.RootCache {
				a.subject.CacheErr = err
				a.logger.Warn("cache directory unusable",
					slog.String("dir", paths.CacheDir),
					slog.String("error", err.Error()))
				return nil
			}
			a.subject.PathsErr = asRuntimePathError(err)
			return a.subject.PathsErr
		}
	}

	return nil
}

// openSQLite is step 4.
//
// A read-only command against a repository that was never initialised is not a
// failure: DBPresent stays false, the sequence continues, and the report says
// so with a next action. That is decision D-01 and it is what keeps
// `mindrail status` usable as the thing you run to find out you need to run
// `mindrail init`.
//
// "Never initialised" includes the zero-length file an interrupted first run
// leaves behind. RuntimePaths.Exists() is a stat, and a stat cannot tell a
// database from a file that was created and never written; opening the second
// one read-only reported "not in WAL mode" and exited 4 for the same state the
// absent case reports at exit 0 (finding H14). storage.IsUnwritten is the one
// question that separates them, and it is asked in read-only mode only: init
// opens the same file for writing and finishes creating it, which is exactly
// the remedy the report recommends.
func (a *App) openSQLite(ctx context.Context) error {
	a.record(StepOpenSQLite)

	if !a.opts.Mode.creates() && !a.runtimeDatabaseExists() {
		a.subject.DBPresent = false
		return nil
	}

	db, err := storage.Open(ctx, storage.Options{
		Path:     a.subject.Paths.DBPath,
		ReadOnly: a.opts.Mode == ModeReadOnly,
	})
	if err != nil {
		a.subject.DBErr = err
		return err
	}
	a.db = db
	a.subject.DBPresent = true

	pragmas, err := storage.ReadPragmas(ctx, db)
	if err != nil {
		a.subject.DBErr = err
		return err
	}
	a.subject.Pragmas = pragmas

	// The integrity check runs on every start rather than only on demand: a
	// corrupt runtime database that is reported as healthy costs a later
	// milestone its evidence, and the MR-001 schema is two tables. The cost of
	// this on a large database is MR-019's to measure.
	result, err := storage.IntegrityCheck(ctx, db)
	if err != nil {
		a.subject.IntegrityErr = err
		return err
	}
	if result != integrityOK {
		a.subject.IntegrityErr = integrityFailure(a.subject.Paths.DBPath, result)
		return a.subject.IntegrityErr
	}

	return nil
}

// runtimeDatabaseExists reports whether there is a database at the runtime path
// for a read-only command to open. A path that is occupied by something a
// database can never be still counts: that is an obstruction Open has to
// describe, not an absence, and swallowing it here would report a directory on
// the database path as a repository nobody has initialised.
func (a *App) runtimeDatabaseExists() bool {
	return a.subject.Paths.Exists() && !storage.IsUnwritten(a.subject.Paths.DBPath)
}

// integrityOK is what PRAGMA integrity_check reports for a healthy file.
const integrityOK = "ok"

func integrityFailure(path, detail string) error {
	return app.NewError(
		app.CodeRuntimeDBCorrupt,
		app.KindUnavailable,
		"the runtime database failed its integrity check: "+detail,
		"Recorded workspace state is damaged, so nothing this database reports can be trusted.",
		"Move "+path+" aside and run `mindrail init` to rebuild it.",
	).WithMetadata("path", path).WithMetadata("detail", detail)
}

// migrateDB is step 5. ModeInit applies; every other mode only reads the ledger
// and verifies it, because a schema that drifted has to fail closed whichever
// command noticed it (decisions D-01, D-23, D-25).
func (a *App) migrateDB(ctx context.Context) error {
	a.record(StepMigrateDB)

	if a.db == nil {
		return nil
	}

	set, err := migration.Load(a.migrationFS())
	if err != nil {
		a.subject.MigrateErr = asMigrationSetError(err)
		return a.subject.MigrateErr
	}

	migrator := migration.New(a.db.DB, set, a.clock)

	if a.opts.Mode == ModeInit {
		result, upErr := migrator.Up(ctx)
		a.initResult.MigrationsApplied = result.Applied
		if upErr != nil {
			a.subject.MigrateErr = asMigrationError(upErr)
			return a.subject.MigrateErr
		}
	}

	ledger, err := migrator.Status(ctx)
	if err != nil {
		a.subject.MigrateErr = asMigrationError(err)
		return a.subject.MigrateErr
	}
	if err := verifyLedger(set, ledger); err != nil {
		a.subject.MigrateErr = err
		return err
	}
	a.subject.Migrations = ledger

	pending, err := migrator.Pending(ctx)
	if err != nil {
		a.subject.MigrateErr = asMigrationError(err)
		return a.subject.MigrateErr
	}
	a.subject.PendingCount = len(pending)

	return nil
}

// verifyLedger is the read-only half of the migrator's own fail-closed gate.
//
// Migrator.Up runs this check before it writes, but a read-only command never
// calls Up, and a database written by a newer Mindrail is exactly the condition
// a read-only command most needs to refuse (decision D-25). The comparison is
// repeated here rather than exported from internal/migration because the
// migrator's copy guards a write and this one guards a report; tying them
// together would mean a change to one silently changing the other.
func verifyLedger(set []migration.Migration, ledger []migration.Applied) error {
	known := make(map[int64]migration.Migration, len(set))
	for _, candidate := range set {
		known[candidate.Version] = candidate
	}

	for _, row := range ledger {
		expected, ok := known[row.Version]
		if !ok {
			return app.NewError(
				app.CodeRuntimeDBSchemaTooNew,
				app.KindFailed,
				"the runtime database records a migration this binary does not know",
				"A newer Mindrail wrote this database; reading it with this binary would report a schema it cannot interpret.",
				"Upgrade Mindrail to a version that includes this migration.",
			).WithMetadata("database_version", strconv.FormatInt(row.Version, 10)).
				WithCause(migration.ErrSchemaAhead)
		}
		if expected.Checksum != row.Checksum {
			return app.NewError(
				app.CodeMigrationChecksumMismatch,
				app.KindFailed,
				"an applied migration has changed since it was applied",
				"The database schema no longer matches the migration files, so nothing derived from it can be trusted.",
				"Revert the edit to the applied migration file and add a new numbered migration instead.",
			).WithMetadata("version", strconv.FormatInt(row.Version, 10)).
				WithCause(migration.ErrChecksumMismatch)
		}
	}

	return nil
}

// validateKnowledge is step 6. The loader performs spec §95 steps 1-4; steps
// 5-11 run here, once, immediately afterwards (decision D-42).
//
// This is the only place in the process that calls validate.Check. Not inside a
// doctor check body: doctor checks are pure functions of an already-resolved
// Subject that open nothing and create nothing, and schema.NewValidator returns
// an error a doctor.Result has no honest way to express. Not in status.Build
// either, for the same reason plus decision D-41 — two layers computing the same
// verdict is two layers that can disagree about one store.
func (a *App) validateKnowledge(ctx context.Context) error {
	a.record(StepValidateKnowledge)

	root, err := filesystem.NewRoot(a.subject.Repo.WorktreeRoot)
	if err != nil {
		a.subject.KnowledgeErr = asKnowledgeError(err)
		return a.subject.KnowledgeErr
	}

	registry, err := schema.NewRegistry(a.schemaFS())
	if err != nil {
		a.subject.KnowledgeErr = asKnowledgeError(err)
		return a.subject.KnowledgeErr
	}

	store, err := loader.New(root, registry).Load(ctx)
	if err != nil {
		a.subject.KnowledgeErr = asKnowledgeError(err)
		return a.subject.KnowledgeErr
	}
	a.subject.Knowledge = store

	// The findings are data, never an error (decision D-42). A store full of
	// invalid records leaves KnowledgeErr nil and the sequence running: the
	// records the repository owns are wrong and the binary is fine, and halting
	// here would make every later step report itself as never taken — publishing
	// a fabricated absence in place of a finding this step actually made.
	a.subject.KnowledgeFindings = validate.Check(store, mustCompileSchemas(registry))

	return nil
}

// mustCompileSchemas builds the validator steps 5-11 evaluate against, and
// refuses to continue when the documents this binary embeds do not compile.
//
// A failure here is a defect in the binary, not a condition of the repository
// (AC-08.2), and the two must not be confused. Folding it into KnowledgeErr
// would publish it as KNOWLEDGE_UNREADABLE with "Make .mindrail/knowledge
// readable." beside it — a diagnosis of a repository that is fine and a remedy
// that cannot work. There is no registered app.Code for "this binary cannot
// build its own validator", and REQ-05 freezes the code vocabulary MR-002 adds
// at two, so there is no honest envelope to put it in either.
//
// Returning a nil validator is worse still: validate.Check panics on one
// precisely because reporting unvalidated records as clean is the fabricated
// pass this whole milestone exists to prevent. So the failure is raised here,
// in the binary's own voice, naming the compile error it came from — the same
// reasoning, and the same shape, as validate.Check's own refusal.
//
// It is reachable only through Options.SchemaFS. The embedded documents compile,
// which internal/knowledge/schema's own tests assert; an injected filesystem
// that ships something else is how this path is exercised.
func mustCompileSchemas(registry *schema.Registry) *schema.Validator {
	validator, err := schema.NewValidator(registry)
	if err != nil {
		panic(fmt.Errorf("mindrail: the knowledge schema documents this binary ships do not compile, so no record can be validated: %w", err))
	}
	return validator
}

// registerWorkspace is step 7. Registration is a write, so ModeReadOnly only
// looks the worktree up; not finding it is the reportable state that drives
// readiness to BLOCKED with `mindrail init` as the remedy.
func (a *App) registerWorkspace(ctx context.Context) error {
	a.record(StepRegisterWorkspace)

	if a.db == nil {
		return nil
	}

	store := workspace.NewStore(a.db.DB, a.clock)

	if a.opts.Mode == ModeInit {
		_, ws, err := store.Register(ctx, workspace.Registration{
			CommonDir:        a.subject.Repo.CommonDir,
			WorktreeRoot:     a.subject.Repo.WorktreeRoot,
			GitDir:           a.subject.Repo.GitDir,
			IsLinkedWorktree: a.subject.Repo.IsLinkedWorktree,
		})
		if err != nil {
			a.subject.WorkspaceErr = asWorkspaceError(err)
			return a.subject.WorkspaceErr
		}
		a.subject.Workspace = ws
		return nil
	}

	// A read-only command can open a database another process created moments
	// ago and has not finished migrating: the file is there, the ledger is
	// empty, and the workspaces table does not exist yet. Querying it produces
	// `no such table: workspaces`, which reached the user verbatim as the `why`
	// line of a WORKSPACE_REGISTRATION_FAILED error (finding F16). The condition
	// is "not initialised yet", and the migration ledger already says so, so the
	// lookup is simply not made.
	if a.subject.PendingCount > 0 {
		a.logger.Debug("workspace lookup skipped: runtime schema not established",
			slog.Int("pending_migrations", a.subject.PendingCount))
		return nil
	}

	ws, err := store.FindByRoot(ctx, a.subject.Repo.WorktreeRoot)
	if err != nil {
		// Both branches are soft: an unregistered worktree is a state, and a
		// lookup that failed for another reason is something WorkspaceCheck
		// reports rather than something that should hide the rest of the
		// report behind it.
		a.subject.WorkspaceErr = err
		return nil
	}
	a.subject.Workspace = ws

	a.readCoordination(ctx)
	return nil
}

// readCoordination fills the summary `status` publishes.
//
// It sits at the end of step 7 rather than in a step of its own because it has
// exactly step 7's prerequisite — a registered workspace, and therefore a
// project to count tasks in — and because tech-stack §87's sequence is the
// contract every later milestone slots into: a step added for a read that
// nothing gates on would have to be argued back out again when MR-004 wants the
// slot.
//
// Every failure here is swallowed into "nobody looked". Coordination cannot move
// readiness (decision D-62), so a summary that could not be read has nothing to
// block, and turning it into a startup failure would let a task count take down
// a report about the repository.
func (a *App) readCoordination(ctx context.Context) {
	summary, err := coordination.NewStore(a.db.DB, a.clock).Summarize(ctx, a.subject.Workspace.ProjectID)
	if err != nil {
		a.logger.Debug("coordination summary unavailable", slog.String("error", err.Error()))
		return
	}
	a.subject.Coordination = summary
	a.subject.CoordinationObserved = true
}

// --- seams ------------------------------------------------------------------

func (a *App) startDir() string {
	if a.opts.StartDir != "" {
		return a.opts.StartDir
	}
	dir, err := os.Getwd()
	if err != nil {
		return "."
	}
	return dir
}

// runtimeDirOverride prefers the constructor parameter over the environment.
// The parameter is what an in-process test uses and the environment variable is
// what the compiled binary has (decision D-07); when both are present the
// explicit one is the one the caller can see.
func (a *App) runtimeDirOverride() string {
	if a.opts.RuntimeDirOverride != "" {
		return a.opts.RuntimeDirOverride
	}
	return a.subject.Config.Config.Runtime.Dir
}

func (a *App) cacheDirOverride() string {
	if a.opts.CacheDirOverride != "" {
		return a.opts.CacheDirOverride
	}
	return a.subject.Config.Config.Runtime.CacheDir
}

func (a *App) migrationFS() fs.FS {
	if a.opts.MigrationFS != nil {
		return a.opts.MigrationFS
	}
	return migrations.FS
}

func (a *App) schemaFS() fs.FS {
	if a.opts.SchemaFS != nil {
		return a.opts.SchemaFS
	}
	return schemas.KnowledgeFS
}

// --- error shaping ----------------------------------------------------------

// domainize guarantees Start's contract that every hard failure carries a code,
// a why, an impact and a remedy. An error that already has a payload is
// returned untouched: the layer that detected the failure knows more about it
// than this one does.
func domainize(err error, code app.Code, kind app.Kind, why, impact string, next ...string) error {
	if _, ok := app.PayloadOf(err); ok {
		return err
	}
	return app.NewError(code, kind, why+": "+err.Error(), impact, next...).WithCause(err)
}

// asRepositoryError codes a discovery failure the git adapter did not code
// itself. The only such failure is a cancelled or expired context: everything
// else already arrives as a domain error. It matters because the uncoded case
// would otherwise fall through to the check's "not a repository" fallback, and
// telling a user who pressed Ctrl-C that they are in the wrong directory sends
// them looking for a mistake they did not make.
func asRepositoryError(err error) error {
	if _, ok := app.PayloadOf(err); ok {
		return err
	}
	return app.NewError(
		app.CodeGitUnavailable,
		app.KindUnavailable,
		"repository discovery did not finish: "+err.Error(),
		"Mindrail never learned which repository it was in, so nothing else could run.",
		"Run the command again.",
	).WithCause(errors.Join(git.ErrGitUnavailable, err))
}

func asRuntimePathError(err error) error {
	return domainize(err, app.CodeRuntimePathUnwritable, app.KindUnavailable,
		"the runtime locations for this repository could not be established",
		"Mindrail cannot store runtime state for this repository, so no command that needs the database can run.",
		"Check the permissions on the Git common directory.",
		"Or point MINDRAIL_RUNTIME_DIR at a writable directory.")
}

func asMigrationError(err error) error {
	return domainize(err, app.CodeMigrationFailed, app.KindFailed,
		"the runtime schema could not be established",
		"The runtime database is not on the schema this binary expects, so it will not be written to.",
		"Run `mindrail init` to apply the pending migrations.")
}

// asMigrationSetError codes a migration set this binary could not even read.
//
// It is separate from asMigrationError because the two have nothing in common
// but a code. Everything asMigrationError describes is a state of the user's
// database, and "run `mindrail init` to apply the pending migrations" is the
// remedy for it. This one is a state of the *binary*: two files claiming the
// same version, a name the loader cannot parse, an embed that did not embed.
// No repository is at fault, `mindrail init` re-runs the command that just
// failed and reproduces it verbatim forever, and the reader is left carrying out
// a remedy that cannot clear the condition it was printed for (finding F12).
//
// It is developer-facing by nature — only a defective build reaches it — so the
// remedy names the build rather than the repository.
func asMigrationSetError(err error) error {
	return domainize(err, app.CodeMigrationFailed, app.KindFailed,
		"this Mindrail build ships a migration set it cannot read",
		"No repository is at fault: the migrations are compiled into the binary, so every command will fail this way in every repository until the binary is replaced.",
		"Reinstall or rebuild Mindrail; `mindrail version` names the build that is failing.",
		"Report the version and the message above, which names the migration files that disagree.")
}

func asKnowledgeError(err error) error {
	return domainize(err, app.CodeKnowledgeUnreadable, app.KindUnavailable,
		"the knowledge store could not be read",
		"Mindrail cannot tell which decisions and invariants apply, so it would work from an unknown subset of the project's knowledge.",
		"Check that .mindrail/knowledge is a readable directory.")
}

func asWorkspaceError(err error) error {
	return domainize(err, app.CodeWorkspaceRegistrationFailed, app.KindFailed,
		"this worktree could not be recorded",
		"Runtime state cannot be attributed to this worktree.",
		"Run `mindrail init` again once the runtime database is usable.")
}
