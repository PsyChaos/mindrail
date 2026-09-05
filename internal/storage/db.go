// Package storage owns the database/sql handle for the runtime database and
// nothing else. It opens the file, guarantees the spec §10 pragmas are really
// in force, and hands out a *sql.DB; the schema itself belongs to
// internal/migration and the rows to the packages that write them.
package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/PsyChaos/mindrail/internal/app"
)

// Options configures a runtime database handle. The zero value of every field
// except Path is a usable default, so callers state only what they mean.
type Options struct {
	Path         string        // absolute mindrail.db path
	BusyTimeout  time.Duration // 0 => 5s
	ReadOnly     bool          // status/doctor
	MaxOpenConns int           // 0 => 4
}

// DB is the runtime database handle. It embeds *sql.DB so callers use the
// standard API directly; the wrapper exists to remember the path for
// diagnostics and to make Close safe to call twice during shutdown.
type DB struct {
	*sql.DB

	path      string
	closeOnce sync.Once
	closeErr  error
}

var (
	// ErrOpenFailed covers every reason the file could not be opened: absent
	// in read-only mode, locked, unwritable, or a bad path.
	ErrOpenFailed = errors.New("runtime database could not be opened")

	// ErrCorrupt means the file exists but is not a usable SQLite database.
	ErrCorrupt = errors.New("runtime database is corrupt")

	// ErrPragma means a connection came back configured differently from what
	// was asked for -- the failure decision D-22's read-back exists to catch.
	ErrPragma = errors.New("runtime database pragma not in effect")

	// ErrNotWAL means the file on disk is not in WAL mode. It is a property of
	// the file rather than of the connection, so it is a separate value from
	// ErrPragma: one is fixed by `mindrail init`, the other by a bug report.
	ErrNotWAL = errors.New("runtime database is not in WAL mode")
)

// Open opens the runtime database and verifies it before returning it.
//
// The verification is not ceremony. The four pragmas ride in on DSN parameters
// and every one of them is applied by the driver per connection; a driver
// upgrade that renamed a parameter, or a pooled connection created before the
// setting existed, would leave foreign keys unenforced with no symptom until a
// later MR's schema silently accepted an orphan row. Open therefore reads the
// settings back from every connection the pool is allowed to hold before it
// hands the handle out (decision D-22).
//
// Open can block for up to the busy timeout: a database another process is
// still creating is a wait, not a failure. See openWithinBusyBudget.
func Open(ctx context.Context, opts Options) (*DB, error) {
	if err := validatePath(opts.Path); err != nil {
		return nil, err
	}

	// Probed before the driver is asked for anything. The driver reports a
	// directory on the database path and a database that was never created
	// with the same generic "unable to open database file", and a caller that
	// cannot tell those apart ends up recommending `mindrail init` for a
	// condition init cannot fix.
	presence, err := Probe(opts.Path)
	if err != nil {
		return nil, err
	}
	if opts.ReadOnly && presence == PresenceAbsent {
		// Decision D-01 makes this a reportable state, not a reason to create
		// the file.
		return nil, absentFailure(opts.Path)
	}

	maxConns := opts.MaxOpenConns
	if maxConns <= 0 {
		maxConns = DefaultMaxOpenConns
	}

	return openWithinBusyBudget(ctx, opts, maxConns)
}

// openPollInterval is how long a blocked Open waits before looking again. It is
// a flat interval on purpose: the bounded exponential ladder and
// MINDRAIL_BUSY_RETRYABLE belong to MR-004, and shipping half of that ladder
// here would make the real one harder to introduce, not easier.
const openPollInterval = 20 * time.Millisecond

// openWithinBusyBudget is decision D-24's "the loser blocks on
// busy_timeout=5000 and then observes the applied set", applied at the point
// where it actually breaks.
//
// busy_timeout governs statements on a connection that already exists; it
// cannot govern the act of creating one. Converting a brand-new database to WAL
// happens during connection setup, so a second process that arrives mid
// conversion is told SQLITE_BUSY at once -- in practice after single-digit
// milliseconds, not after the five seconds the pragma promises. Without this
// wait, two `mindrail init` runs that overlap by a few milliseconds leave one of
// them dead with exit 4 against a repository that is healthy an instant later.
//
// The budget is the same busy_timeout the connection itself would have honoured,
// so the total time a caller can spend inside Open stays inside the D-08
// shutdown budget.
func openWithinBusyBudget(ctx context.Context, opts Options, maxConns int) (*DB, error) {
	deadline := time.Now().Add(busyBudget(opts.BusyTimeout))

	for {
		db, err := openOnce(ctx, opts, maxConns)
		if err == nil {
			return db, nil
		}
		if !isBusyError(err) || ctx.Err() != nil {
			return nil, err
		}

		wait := min(openPollInterval, time.Until(deadline))
		if wait <= 0 {
			return nil, err
		}

		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, err
		case <-timer.C:
		}
	}
}

// busyBudget is the wall-clock the caller has agreed to wait for a lock, which
// is by construction the same number the connection carries as busy_timeout.
func busyBudget(busyTimeout time.Duration) time.Duration {
	return time.Duration(busyTimeoutMillis(busyTimeout)) * time.Millisecond
}

// openOnce is one attempt: open, prove the file is reachable, prove the pragmas
// took. It owns the handle it creates, so a failed attempt leaves nothing for
// the retry loop to clean up.
func openOnce(ctx context.Context, opts Options, maxConns int) (*DB, error) {
	sqlDB, err := sql.Open(driverName, dsn(opts))
	if err != nil {
		return nil, classifyOpenError(opts.Path, err)
	}
	sqlDB.SetMaxOpenConns(maxConns)
	sqlDB.SetMaxIdleConns(maxConns)

	db := &DB{DB: sqlDB, path: opts.Path}

	// sql.Open is lazy; nothing has touched the file yet.
	if err := sqlDB.PingContext(ctx); err != nil {
		return nil, errors.Join(classifyOpenError(opts.Path, err), db.Close())
	}
	if err := verifyPragmas(ctx, sqlDB, opts, maxConns); err != nil {
		return nil, errors.Join(err, db.Close())
	}

	return db, nil
}

// Path reports the file this handle was opened against. Diagnostics quote it;
// nothing derives behaviour from it.
func (db *DB) Path() string { return db.path }

// Close releases the pool. It is idempotent because the shutdown sequence and
// a caller's defer can both reach it (tech-stack §88).
func (db *DB) Close() error {
	db.closeOnce.Do(func() { db.closeErr = db.DB.Close() })
	return db.closeErr
}

// validatePath rejects anything that is not an absolute file path. A relative
// path would resolve against the process working directory, which for a CLI
// invoked from a subdirectory is not the repository root.
func validatePath(path string) error {
	switch {
	case path == "":
		return openFailure(path, errors.New("no database path given"))
	case !filepath.IsAbs(path):
		return openFailure(path, fmt.Errorf("path %q is not absolute", path))
	default:
		return nil
	}
}

// verifyPragmas checks every connection the pool may hand out, not just one.
// Holding them all open at once is what forces the pool to create them:
// checking one connection four times would prove nothing about the fourth.
func verifyPragmas(ctx context.Context, sqlDB *sql.DB, opts Options, maxConns int) error {
	want := ExpectedPragmas(opts.BusyTimeout)

	held := make([]*sql.Conn, 0, maxConns)
	defer func() {
		for _, conn := range held {
			_ = conn.Close()
		}
	}()

	for range maxConns {
		conn, err := sqlDB.Conn(ctx)
		if err != nil {
			return openFailure(opts.Path, err)
		}
		held = append(held, conn)

		got, err := ReadPragmas(ctx, conn)
		if err != nil {
			return classifyOpenError(opts.Path, err)
		}
		if got != want {
			return classifyPragmaMismatch(opts, want, got)
		}
	}

	return nil
}

// classifyPragmaMismatch splits the read-back failure into the two conditions
// with genuinely different remedies (finding F5).
//
// journal_mode is not like the other three. They are per-connection settings
// the DSN sets on every connection, so one coming back wrong really does mean
// the driver ignored what it was told, and the only remedy is a bug report. WAL
// is a persistent property of the *file*, and a read-only handle deliberately
// does not ask for it (see dsn): opening one against a file that was never
// converted -- a zero-length `mindrail.db` left by an interrupted first run, or
// a database some other tool created in rollback-journal mode -- reports
// journal_mode=delete through a driver that behaved perfectly.
//
// Reporting that as a driver defect handed the user "Report this with the
// mindrail version" for a condition `mindrail init` clears in one command. This
// keeps the driver diagnosis for the case that is actually about the driver.
func classifyPragmaMismatch(opts Options, want, got Pragmas) error {
	journalOnly := got.JournalMode != want.JournalMode &&
		got.ForeignKeys == want.ForeignKeys &&
		got.BusyTimeout == want.BusyTimeout &&
		got.Synchronous == want.Synchronous

	if opts.ReadOnly && journalOnly {
		return notWALFailure(opts.Path, got.JournalMode)
	}
	return pragmaFailure(opts.Path, want, got)
}

// IsUnwritten reports a runtime database path that no database has ever been
// written to: nothing is there, or a regular file of zero length is.
//
// A zero-length mindrail.db is what an interrupted first run leaves behind. The
// file exists, and it holds no header, no schema, no workspace row and no
// journal — which is the same condition as a repository that was never
// initialised at all, cleared by the same `mindrail init`. Decision D-03 puts
// "not initialised" at exit 0, and the other half of the same interrupted first
// run — a database created but not yet migrated — is already reported that way.
// Treating the zero-length file as a database and opening it produced
// "not in WAL mode": a fatal RUNTIME_DB_UNAVAILABLE at exit 4 for the identical
// condition, two spellings of one state with two exit codes (finding H14).
//
// It is a separate question from Probe's rather than a fourth Presence value.
// Probe answers "can a database live at this path?", and for a zero-length file
// the answer is yes; this answers "does one live there yet?", which is what
// decides between reporting a state and reporting a failure.
//
// A path that cannot be stat'ed for any other reason is not unwritten: that is
// an obstruction, and Probe is the function that describes it.
func IsUnwritten(path string) bool {
	info, err := os.Stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return true
	}
	return err == nil && info.Mode().IsRegular() && info.Size() == 0
}

// notWALFailure reports a runtime database file that is not in WAL mode, with
// the remedy that converts it. The two spellings differ only in what the user
// is looking at: an empty file is a first run that did not finish, and a
// populated one is a database somebody else made.
//
// The empty branch is the last resort, not the reported one: a caller that asks
// IsUnwritten first — every MR-001 command does, through bootstrap — never gets
// here with a zero-length file, because that condition is a reportable state
// rather than an open failure (finding H14). It stays because storage.Open owes
// a usable diagnosis to a caller that did not ask.
func notWALFailure(path, observed string) error {
	why := "the runtime database at " + path + " is not in WAL mode"
	impact := "Mindrail requires WAL journalling before it will read or write this database."
	next := "Run `mindrail init` to convert it."

	if info, err := os.Stat(path); err == nil && info.Size() == 0 {
		why = "the runtime database at " + path + " is empty: it was created but never initialised"
		impact = "A previous `mindrail init` did not finish, so this file holds no schema and no state."
		next = "Run `mindrail init` to finish creating it."
	}

	return app.NewError(
		app.CodeRuntimeDBUnavailable,
		app.KindUnavailable,
		why,
		impact,
		next,
	).
		WithMetadata("path", path).
		WithMetadata("journal_mode", observed).
		WithCause(fmt.Errorf("%w: journal_mode is %q, want %q", ErrNotWAL, observed, expectedJournalMode))
}

// classifyOpenError splits a driver failure into the conditions with different
// remedies: a corrupt file must be removed and rebuilt, a full filesystem needs
// space and nothing else, and everything left is an environment problem that may
// clear on its own.
//
// The disk-full branch is not a refinement of the fallback, it is a repair of
// it. On a genuinely full filesystem holding an already-initialised database,
// every command failed here with the generic diagnosis below, whose first line
// ("check that the Git common directory exists and is writable") is a dead end
// -- the directory existed and was writable -- and whose second sends the user
// to `mindrail init`, which fails identically. The user loops.
func classifyOpenError(path string, cause error) error {
	switch {
	case isCorruptError(cause):
		return corruptFailure(path, cause)
	case isDiskFullError(cause):
		return diskFullOpenFailure(path, cause)
	default:
		return openFailure(path, cause)
	}
}

// diskFullOpenFailure reports a database that could not be opened because the
// filesystem under it has no space left.
//
// It is CodeRuntimePathUnwritable rather than CodeRuntimeDBUnavailable for the
// same reason the probe in writable.go is: nothing is wrong with the database.
// It is intact, it is readable the moment there is room for its shared-memory
// index, and describing it as unavailable is what produced a remedy about the
// Git common directory. Decision D-03 puts both codes at exit 4, so the exit
// status a caller sees does not move.
//
// The remedy is word for word the one `mindrail init` prints when a migration
// runs out of space, because it is the same condition seen a few milliseconds
// earlier and a reader who meets it twice should not have to work out that it is.
func diskFullOpenFailure(path string, cause error) error {
	return app.NewError(
		app.CodeRuntimePathUnwritable,
		app.KindUnavailable,
		"the filesystem holding the runtime database at "+path+" is full, so the database could not be opened",
		"SQLite cannot size the shared-memory index a WAL database needs, so no command can read or write runtime state until there is space for it.",
		"free space on the filesystem holding "+path,
	).
		WithMetadata("path", path).
		WithMetadata("condition", "disk_full").
		WithCause(fmt.Errorf("%w: %w", ErrDiskFull, cause))
}

func openFailure(path string, cause error) error {
	return app.NewError(
		app.CodeRuntimeDBUnavailable,
		app.KindUnavailable,
		"the runtime database could not be opened",
		"Mindrail cannot record or read workspace state until the database is reachable.",
		"Check that the Git common directory exists and is writable.",
		"Run `mindrail init` if this repository has not been initialised yet.",
	).WithMetadata("path", path).WithCause(fmt.Errorf("%w: %w", ErrOpenFailed, cause))
}

// corruptFailure reports a file that is not a usable SQLite database.
//
// The remedy names the file (finding F10). "Move the file aside" is only a
// remedy for a user who knows which file, and the runtime database lives under
// .git in a directory nothing else puts them in; leaving the path in the JSON
// metadata made the human output the one place the answer was missing.
func corruptFailure(path string, cause error) error {
	return app.NewError(
		app.CodeRuntimeDBCorrupt,
		app.KindUnavailable,
		"the runtime database file at "+path+" is not a valid SQLite database",
		"All recorded workspace state is unreadable; Mindrail cannot start against this file.",
		"Move "+path+" aside and run `mindrail init` to rebuild it.",
	).WithMetadata("path", path).WithCause(fmt.Errorf("%w: %w", ErrCorrupt, cause))
}

func pragmaFailure(path string, want, got Pragmas) error {
	return app.NewError(
		app.CodeRuntimeDBUnavailable,
		app.KindUnavailable,
		"a required SQLite pragma is not in effect on a pooled connection",
		"Foreign keys, durability or lock waiting may be silently disabled, so writes could corrupt state.",
		"Report this with the mindrail version; the SQLite driver is not honouring its connection settings.",
	).
		WithMetadata("path", path).
		WithMetadata("want", fmt.Sprintf("%+v", want)).
		WithMetadata("got", fmt.Sprintf("%+v", got)).
		WithCause(ErrPragma)
}
