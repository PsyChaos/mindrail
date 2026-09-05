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
// a sub-reason most of the classifiers here do not need.
const sqlitePrimaryCodeMask = 0xff

// Extended SQLite result codes, for the one question the primary code cannot
// answer.
//
// SQLITE_IOERR on its own is "some I/O call failed", which is not a condition
// anybody can act on; the sub-reason in the high bits is the whole diagnosis.
// SQLITE_IOERR_SHMSIZE is returned when the `-shm` file beside a WAL database
// cannot be grown to the size SQLite needs, and on a Unix VFS the way that
// happens is that the filesystem will not give it the bytes. It is what a full
// disk looks like at *open* time rather than at write time: the database is
// opened, its `-wal` and `-shm` are created costing no data blocks, and the
// first attempt to size the shared-memory index fails.
//
// It was measured, not guessed. `mindrail status` against a cleanly closed,
// fully migrated database on a tmpfs with zero bytes available comes back with
// `disk I/O error (4874)` -- 4874 is SQLITE_IOERR | (19<<8) -- which classified
// as "everything else" and produced the fallback remedy "check that the Git
// common directory exists and is writable", over a directory that existed and
// was writable.
const sqliteIOErrShmSize = 4874 // SQLITE_IOERR_SHMSIZE

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

// isDiskFullError reports a failure that came from want of space rather than
// want of permission. It is the one condition in this file that no change to the
// database or its path can clear, so collapsing it into "unwritable" would hand
// the user a chmod for a full disk.
func isDiskFullError(err error) bool {
	code, ok := driverResultCode(err)
	return ok && diskFullCode(code)
}

// diskFullCode is the policy half of isDiskFullError, kept apart from the
// error-unwrapping half so it can be exercised over the result codes SQLite can
// actually return rather than only over the one a test managed to provoke.
//
// Two codes mean "there was no room", and the audit behind that short list is
// worth writing down, because the surrounding SQLITE_IOERR_* family looks like
// it belongs and does not:
//
//   - SQLITE_FULL (13, in any extended form) is the write-time answer. The Unix
//     VFS converts an ENOSPC out of write(2) into SQLITE_FULL itself, which is
//     precisely why SQLITE_IOERR_WRITE (778) is *not* here: by the time it is
//     returned, ENOSPC has already been ruled out and what is left is a real
//     I/O error, whose remedy is not "free space".
//   - SQLITE_IOERR_SHMSIZE (4874) is the open-time answer, measured above.
//
// Deliberately excluded, each for its own reason: SQLITE_IOERR_FSYNC (1034) and
// SQLITE_IOERR_DIR_FSYNC (1290) are a failed flush, which on a delayed-allocation
// filesystem can be caused by a full disk but is far more often hardware, and
// claiming the disk is full when a drive is failing sends the user to the wrong
// place entirely; SQLITE_IOERR_TRUNCATE (1546) shrinks a file, which does not
// need space; SQLITE_IOERR_SHMOPEN (4618) and SQLITE_IOERR_SHMMAP (5386) are a
// failed open and a failed mmap of the shared-memory index, which are a
// permission problem and a memory problem respectively; SQLITE_IOERR_NOMEM
// (3082) is memory, not disk. SQLITE_CANTOPEN (14) is excluded too: creating the
// empty `-wal` and `-shm` costs no data blocks and normally succeeds even at zero
// bytes free, so on a full disk it is rarely the code -- while a missing path or
// a refused permission produces it constantly, and a remedy telling those users
// to free space would be a dead end of exactly the kind this classification
// exists to remove.
func diskFullCode(code int) bool {
	return code&sqlitePrimaryCodeMask == sqliteFull || code == sqliteIOErrShmSize
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
	raw, ok := driverResultCode(err)
	if !ok {
		return false
	}

	code := raw & sqlitePrimaryCodeMask
	for _, candidate := range want {
		if code == candidate {
			return true
		}
	}
	return false
}

// driverResultCode extracts SQLite's result code from a failure, extended bits
// and all. It is the only place in the repository that knows what a driver error
// looks like, so a classifier that needs the sub-reason does not have to reach
// for the driver type to get it.
func driverResultCode(err error) (int, bool) {
	var sqliteErr *sqlitedriver.Error
	if !errors.As(err, &sqliteErr) {
		return 0, false
	}
	return sqliteErr.Code(), true
}
