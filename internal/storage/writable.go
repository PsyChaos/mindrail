package storage

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/PsyChaos/mindrail/internal/app"
)

// The two files SQLite keeps beside a WAL database.
//
// They are not interchangeable, and the difference was measured rather than
// assumed. A `-wal` whose mode bits refuse writes costs nothing: SQLite brings
// its mode into line with the database file's and writes through it, even from a
// directory that accepts no new entries. A `-shm` that refuses writes is fatal:
// it is mapped read-write and there is no recovery, so every write fails with
// SQLITE_READONLY while the database file's own mode is perfectly correct.
// TestProbeWriteAccessAgreesWithSQLite walks the whole matrix and would fail if
// either half of that stopped being true.
const (
	walSuffix = "-wal"
	shmSuffix = "-shm"
)

// walSidecarSuffixes is the pair, for the one question that is about both: a
// directory that accepts no new entries still leaves the database writable when
// SQLite has nothing left to create in it.
var walSidecarSuffixes = []string{walSuffix, shmSuffix}

// WriteBlocker names which part of a runtime database's on-disk footprint
// refuses a write.
//
// The three are separate because their remedies are: one is a chmod on the
// database file, one a chmod on the directory that has to hold its sidecars, and
// one is not a permission problem at all. Collapsing them is how a report ends up
// telling a user to fix the mode of a path whose mode was already correct.
type WriteBlocker string

const (
	// BlockerNone means nothing refused the write.
	BlockerNone WriteBlocker = ""

	// BlockerDatabaseFile means the mindrail.db file itself is not writable.
	BlockerDatabaseFile WriteBlocker = "database_file"

	// BlockerSidecar means the -shm shared-memory index beside the database is
	// not writable, which refuses every write while the database file's own mode
	// looks correct.
	BlockerSidecar WriteBlocker = "wal_sidecar"

	// BlockerDirectory means the directory holding the database will not accept
	// the WAL sidecars SQLite has to create there.
	BlockerDirectory WriteBlocker = "runtime_directory"

	// BlockerObstruction means the path is occupied by something that is not a
	// database file, which Probe describes in full.
	BlockerObstruction WriteBlocker = "obstruction"
)

// WriteAccess is the answer to "could Mindrail write to this runtime database?"
// asked without writing to it.
//
// It exists because every probe in MR-001 stopped at "can the file be opened".
// A mindrail.db whose mode bits refuse writes opens perfectly: `doctor` read the
// pragmas back, found WAL and foreign keys in force, printed `Overall: OK` and
// exited 0, while `mindrail init` in the same directory failed on
// `attempt to write a readonly database (8)` -- and sent the user to that same
// doctor. Openability and writability are separate facts and this type keeps
// them separate (finding W2).
type WriteAccess struct {
	// Path is the database file the answer is about.
	Path string

	// Writable reports whether a write would be allowed to reach the disk. A
	// database that does not exist yet is writable when its directory is: the
	// question is about the path, not about the file's existence.
	Writable bool

	// Blocker names what refused, and is BlockerNone exactly when Writable.
	Blocker WriteBlocker

	// Blocked is the path that actually carries the refusal, which for a
	// directory or a sidecar is not Path.
	Blocked string

	// Err is the structured error, non-nil exactly when Writable is false.
	Err error
}

// ProbeWriteAccess reports whether the runtime database at path could be
// written, without writing to it.
//
// Decision D-01 forbids doctor and status from mutating anything, and that rules
// out the two probes that would otherwise be obvious. Writing a row and deleting
// it is a mutation by any reading. So is `BEGIN IMMEDIATE; ROLLBACK`: in WAL mode
// taking the write lock creates `mindrail.db-shm` and `mindrail.db-wal` where a
// cleanly closed database has neither, so a doctor run would leave two files
// behind and, for the seconds it held the lock, block the `mindrail init` it was
// diagnosing. The question is therefore asked of the filesystem, which can answer
// it with no side effect at all.
//
// Three things have to be true, and only three. The main file, when it exists,
// has to accept writes. The `-shm` index, when it exists, has to accept them
// too. And SQLite has to be able to make whichever sidecars are missing, which
// means either the directory accepts new entries or nothing is missing.
//
// The `-wal`'s own mode is deliberately not part of the answer; see the suffix
// constants for the measurement behind that. Getting it wrong in either
// direction is a finding: claiming the `-wal` matters fails a working
// repository, and ignoring the `-shm` is the original gap wearing a new
// spelling.
//
// It answers about this database and nothing else. The cache directory, the
// repository config directory and every other path Mindrail touches are
// somebody else's question: a pass that let one of those decide the runtime
// database's verdict bricked a working repository once already.
func ProbeWriteAccess(path string) WriteAccess {
	result := WriteAccess{Path: path}

	presence, err := Probe(path)
	if err != nil {
		result.Blocker = BlockerObstruction
		result.Blocked = path
		result.Err = err
		return result
	}

	if presence == PresenceFile {
		if accessErr := accessWritableFile(path); accessErr != nil {
			return result.blockedBy(BlockerDatabaseFile, path, accessErr)
		}
	}

	shm := path + shmSuffix
	if _, statErr := os.Lstat(shm); statErr == nil {
		if accessErr := accessWritableFile(shm); accessErr != nil {
			return result.blockedBy(BlockerSidecar, shm, accessErr)
		}
	}

	dir := filepath.Dir(path)
	dirErr := accessWritableDir(dir)
	if dirErr == nil {
		result.Writable = true
		return result
	}

	return result.withoutADirectoryToWriteIn(dir, dirErr)
}

// withoutADirectoryToWriteIn decides the case where the directory holding the
// database accepts no new entries.
//
// It is not automatically fatal. SQLite creates nothing when the WAL and its
// shared-memory index are already beside the database, which is the state of
// every database another Mindrail process has open at this instant. Refusing
// that outright would report a busy, perfectly healthy repository as unwritable
// -- and would do it from doctor, whose whole job is to be believed.
func (a WriteAccess) withoutADirectoryToWriteIn(dir string, dirErr error) WriteAccess {
	for _, suffix := range walSidecarSuffixes {
		if _, statErr := os.Lstat(a.Path + suffix); statErr != nil {
			// SQLite would have to create this one, and it cannot.
			return a.blockedBy(BlockerDirectory, dir, dirErr)
		}
	}

	a.Writable = true
	return a
}

// blockedBy fills in the refusal half of the answer, so the four call sites
// above cannot describe the same shape four different ways.
func (a WriteAccess) blockedBy(blocker WriteBlocker, blocked string, cause error) WriteAccess {
	a.Writable = false
	a.Blocker = blocker
	a.Blocked = blocked
	a.Err = unwritableDatabaseError(a.Path, blocker, blocked, cause)
	return a
}

// unwritableDatabaseError reports a runtime database that can be read and not
// written.
//
// It is CodeRuntimePathUnwritable rather than CodeRuntimeDBUnavailable because
// the database is not unavailable: every read of it works, which is precisely
// why the condition survived four audits as `Overall: OK`. Decision D-03 puts an
// unwritable runtime path at exit 4.
//
// The remedy names every path in the footprint that currently refuses a write,
// not only the one this probe stopped at, because a remedy that clears half the
// condition is a slower spelling of the loop. A `mindrail init` that fails
// against a read-only `mindrail.db` leaves a `-shm` behind carrying the same
// mode -- SQLite gives a sidecar it creates the database file's permissions --
// so "restore write permission on mindrail.db" alone is followed by a re-run
// that fails on the file the first run created. That was measured, not
// imagined: it is what the first draft of this remedy did.
func unwritableDatabaseError(path string, blocker WriteBlocker, blocked string, cause error) error {
	why := "the runtime database at " + path + " is not writable"
	next := "restore write permission on " + strings.Join(RefusedWritePaths(path), ", ")

	switch blocker {
	case BlockerSidecar:
		why = "the shared-memory index " + blocked + " beside the runtime database is not writable"
	case BlockerDirectory:
		why = "the directory " + blocked + " will not accept the write-ahead log files the runtime database needs"
		next = "restore write and search permission on " + blocked
	}

	return app.NewError(
		app.CodeRuntimePathUnwritable,
		app.KindUnavailable,
		why,
		"Mindrail can read the state already recorded here but cannot record anything new, so `mindrail init` and every other command that writes will fail.",
		next,
	).
		WithMetadata("path", path).
		WithMetadata("blocked_path", blocked).
		WithMetadata("blocker", string(blocker)).
		WithMetadata("detail", cause.Error()).
		WithCause(errors.Join(ErrReadOnly, cause))
}

// RefusedWritePaths lists every existing file in a runtime database's footprint
// that will not accept a write, database file first.
//
// It exists so the remedy is complete rather than merely correct. The `-wal` is
// included even though its mode never decides ProbeWriteAccess's verdict:
// leaving a file the same failed run created at the same wrong mode is how a
// user ends up chmodding one path per attempt. Detection stays narrow so it
// cannot over-fire; the remedy is allowed to be generous, because widening it
// costs a user nothing and narrowing it costs them another failed run.
//
// The database path itself is the fallback when nothing can be inspected, so the
// remedy always names something.
func RefusedWritePaths(path string) []string {
	refused := make([]string, 0, 3)
	for _, candidate := range []string{path, path + walSuffix, path + shmSuffix} {
		if _, err := os.Lstat(candidate); err != nil {
			continue
		}
		if accessWritableFile(candidate) != nil {
			refused = append(refused, candidate)
		}
	}

	if len(refused) == 0 {
		return []string{path}
	}
	return refused
}
