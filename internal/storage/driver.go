package storage

import (
	"errors"
	"net/url"
	"strconv"

	// This is the only import of a concrete SQLite driver in the whole
	// repository (tech-stack §20, decision D-33's sibling constraint). Keeping
	// it here means a driver swap touches one file and no query anywhere else
	// can grow a dependency on driver-specific behaviour.
	// TestDriverImportConfinedToStorage fails the build if it spreads.
	sqlitedriver "modernc.org/sqlite"
)

// driverName is the name modernc.org/sqlite registers with database/sql.
const driverName = "sqlite"

// Primary SQLite result codes. They are spelled out rather than imported from
// modernc.org/sqlite/lib so that the extended-code masking below stays
// readable; the values are fixed by the SQLite C API and cannot change.
const (
	sqliteBusy     = 5  // SQLITE_BUSY: another connection holds the lock
	sqliteLocked   = 6  // SQLITE_LOCKED: a table in this database is locked
	sqliteReadOnly = 8  // SQLITE_READONLY: the database cannot be written to
	sqliteCorrupt  = 11 // SQLITE_CORRUPT: the database disk image is malformed
	sqliteFull     = 13 // SQLITE_FULL: the disk holding the database is full
	sqliteNotADB   = 26 // SQLITE_NOTADB: the file is not a database
)

// sqlitePrimaryCodeMask strips the extended result code, whose high bits carry
// a sub-reason we do not classify on.
const sqlitePrimaryCodeMask = 0xff

// dsn renders the connection string. The four pragmas of spec §10 travel as
// DSN parameters so the driver applies them to every connection it opens,
// including the ones database/sql adds to the pool later; decision D-22 then
// has Open read them back before the handle is handed out.
//
// A read-only handle omits journal_mode: it is a persistent property of the
// file, and attempting to set it without write access fails the connection.
// The remaining three are per-connection settings and apply either way.
func dsn(opts Options) string {
	query := url.Values{}
	query.Add("_pragma", "busy_timeout("+strconv.Itoa(busyTimeoutMillis(opts.BusyTimeout))+")")
	query.Add("_pragma", "foreign_keys(1)")
	query.Add("_pragma", "synchronous("+strconv.Itoa(expectedSynchronous)+")")

	if opts.ReadOnly {
		query.Set("mode", "ro")
	} else {
		query.Add("_pragma", "journal_mode("+expectedJournalMode+")")
		// Every write in Mindrail goes through InTx, and every one of them
		// writes. Taking the write lock at BEGIN turns a mid-transaction
		// SQLITE_BUSY -- which cannot be retried without redoing the reads --
		// into a wait at the door, which busy_timeout handles (decision D-24).
		query.Set("_txlock", "immediate")
	}

	uri := url.URL{Scheme: "file", Path: opts.Path, RawQuery: query.Encode()}
	return uri.String()
}

// isBusyError reports whether a failure is "someone else holds the lock right
// now", which is the one open failure that is worth waiting out rather than
// reporting. It is decided on the driver's result code because the condition is
// indistinguishable from a permanent failure by message text, and treating a
// permanent failure as retryable would turn an immediate diagnosis into a
// five-second stall followed by the same diagnosis.
func isBusyError(err error) bool {
	return hasPrimaryCode(err, sqliteBusy, sqliteLocked)
}

// isReadOnlyError reports a database the driver will not write to. The primary
// code is the same 8 whether the main file's mode bits refuse the write, the
// directory refuses the WAL sidecars (SQLITE_READONLY_DIRECTORY, 1544), or the
// filesystem itself is mounted read-only; the extended code names which, and the
// mask deliberately drops that distinction because the remedy is "make the
// runtime path writable" in every case and the probe in writable.go is what
// tells the user which path to reach for.
func isReadOnlyError(err error) bool {
	return hasPrimaryCode(err, sqliteReadOnly)
}

// isDiskFullError reports a write that failed for want of space rather than for
// want of permission. It is the one condition in this file that no change to the
// database or its path can clear, so collapsing it into "unwritable" would hand
// the user a chmod for a full disk.
func isDiskFullError(err error) bool {
	return hasPrimaryCode(err, sqliteFull)
}

// isCorruptError distinguishes "this file is not a usable database" from every
// other reason an open can fail. The distinction drives two different error
// codes and two different remedies, so it is made on the driver's result code
// rather than on message text (tech-stack §72).
func isCorruptError(err error) bool {
	return hasPrimaryCode(err, sqliteCorrupt, sqliteNotADB)
}

// hasPrimaryCode reports whether err is a driver error whose primary result code
// is one of want. Every classifier in this file is the same three lines, and
// writing them once keeps the extended-code masking in a single place: a
// classifier that forgot the mask would silently stop matching the moment SQLite
// returned the extended form of the code it was looking for.
func hasPrimaryCode(err error, want ...int) bool {
	var sqliteErr *sqlitedriver.Error
	if !errors.As(err, &sqliteErr) {
		return false
	}

	code := sqliteErr.Code() & sqlitePrimaryCodeMask
	for _, candidate := range want {
		if code == candidate {
			return true
		}
	}
	return false
}
