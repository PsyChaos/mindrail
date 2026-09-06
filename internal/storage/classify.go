package storage

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/filesystem"
)

// The write-path conditions storage can name. They are sentinels rather than
// bare booleans so a caller several layers up -- a command, a test -- can ask
// `errors.Is` about a failure it only ever sees wrapped.
var (
	// ErrBusy means another connection held the write lock for longer than the
	// busy timeout allowed. Nothing is wrong with the database.
	ErrBusy = errors.New("runtime database is locked by another process")

	// ErrReadOnly means the driver refused the write because the database
	// cannot be written to at all.
	ErrReadOnly = errors.New("runtime database is not writable")

	// ErrDiskFull means the write failed for want of space.
	ErrDiskFull = errors.New("no space left for the runtime database")

	// ErrSizeLimit means a size ceiling refused the write on a filesystem that
	// still has room: a per-file limit (RLIMIT_FSIZE) or an exhausted per-user
	// quota. It is a separate sentinel from ErrDiskFull because the remedies
	// have nothing in common — freeing space clears one and does nothing at all
	// for the other — and a caller that matched on ErrDiskFull would inherit the
	// wrong one (finding F03).
	ErrSizeLimit = errors.New("a size limit refused the runtime database its shared-memory index")

	// ErrCheckpointFailed means the write-ahead log could not be written back
	// into the database file for a reason none of the conditions above names.
	ErrCheckpointFailed = errors.New("the write-ahead log could not be checkpointed")
)

// IsBusy reports whether err is SQLite saying another connection holds the
// write lock right now.
//
// It is exported because the condition outlives the open: openWithinBusyBudget
// waits it out at the door, but a BEGIN IMMEDIATE on an already-open handle can
// meet the same lock, and there the caller is the one that has to tell "someone
// else is writing" apart from "this write is broken" (finding W1).
func IsBusy(err error) bool { return isBusyError(err) }

// IsReadOnly reports whether err is SQLite refusing to write to this database:
// the file's mode bits, the directory that must hold its WAL sidecars, or the
// mount underneath it.
func IsReadOnly(err error) bool { return isReadOnlyError(err) }

// IsDiskFull reports whether err is SQLite failing a write for want of space.
func IsDiskFull(err error) bool { return isDiskFullError(err) }

// WriteFailure names a failed write whose cause is the database rather than the
// statement, and returns nil for anything else.
//
// Returning nil for the unrecognised case is the point. A caller keeps its own
// diagnosis -- "migration 000001 could not be applied", "the workspace could not
// be registered" -- for the failures that really are about what it was writing,
// and defers to this one only for the three conditions that are about the
// database itself. Before this existed every one of those arrived as
// `app.KindFailed`, so a database another `mindrail init` held for a few
// milliseconds exited 1 ("the operation is broken") where decision D-03 puts
// contention at 4 ("try again"), under a remedy that named the wrong cause.
//
// q is used only to name the file in a remedy, and only for the two conditions
// whose remedy needs a path; it may be nil. The lookup is a read, so it works on
// exactly the handles these conditions leave behind -- a read-only database
// answers it, and a contended one is never asked.
//
// what is a verb phrase naming the write, read as "... so Mindrail could not
// <what>".
//
// The result is the concrete *app.DomainError rather than an error interface so
// that a caller's `if named := WriteFailure(...); named != nil` cannot be fooled
// by a typed nil, and so that a caller can still attach its own metadata to a
// diagnosis this function wrote.
func WriteFailure(ctx context.Context, q Querier, what string, cause error) *app.DomainError {
	switch {
	case IsBusy(cause):
		return busyFailure(what, cause)
	case IsReadOnly(cause):
		return readOnlyWriteFailure(ctx, q, what, cause)
	case IsDiskFull(cause):
		return diskFullFailure(ctx, q, what, cause)
	default:
		return nil
	}
}

// busyFailure reports contention: the one condition here that is expected to
// clear on its own, which is why decision D-03 rates it unavailable rather than
// failed. The remedy is to run the command again, and it can succeed -- the
// other process finishes and the lock is released.
func busyFailure(what string, cause error) *app.DomainError {
	return app.NewError(
		app.CodeRuntimeDBUnavailable,
		app.KindUnavailable,
		"another process holds the runtime database's write lock, so Mindrail could not "+what,
		"Nothing was written and nothing is damaged; the database is in use by another Mindrail command.",
		"Wait for the other Mindrail command to finish, then run this one again.",
	).WithMetadata("condition", "locked").WithCause(fmt.Errorf("%w: %w", ErrBusy, cause))
}

// readOnlyWriteFailure reports a database that can be read and not written.
//
// The remedy names the files because it has to be able to succeed: the remedy
// this replaces sent the user to `mindrail doctor`, which reported the same
// database healthy -- it opens read-only and never attempts a write -- and then
// back to the `mindrail init` that had just failed. The list comes from the
// filesystem rather than from a guess, so it names the `-shm` the failed run
// itself left behind at the same wrong mode, which chmodding the database file
// alone would leave in the way of the very next attempt.
func readOnlyWriteFailure(ctx context.Context, q Querier, what string, cause error) *app.DomainError {
	path := mainDatabaseFile(ctx, q)

	// SQLITE_READONLY is the same primary code whether the mode bits refuse the
	// write or the mount does, and the two remedies have nothing in common. The
	// probe is what can tell them apart, so it is asked before the sentence is
	// written: `doctor` inspecting this database and this failure describing it
	// must not name one condition two ways.
	if path != "" {
		if refusal := ProbeWriteAccess(path); refusal.Blocker == BlockerReadOnlyMedia {
			return readOnlyMediaWriteFailure(path, refusal.Blocked, what, cause)
		}
	}

	return app.NewError(
		app.CodeRuntimePathUnwritable,
		app.KindUnavailable,
		"the runtime database is not writable, so Mindrail could not "+what,
		"Mindrail can read the state already recorded here but cannot record anything new, so every command that writes will fail the same way.",
		"restore write permission on "+describeRefusedPaths(path),
	).WithMetadata("condition", "read_only").
		WithMetadata("path", path).
		WithCause(fmt.Errorf("%w: %w", ErrReadOnly, cause))
}

// readOnlyMediaWriteFailure is the read-only case whose remedy is not a chmod.
//
// It is kept apart from readOnlyWriteFailure for the same reason diskFullFailure
// is: the sentence has to change, because a mount that refuses writes cannot be
// argued with by changing a mode. Its wording matches
// readOnlyMediaDatabaseError, so the `mindrail init` that fails on a read-only
// checkout and the `mindrail doctor` that inspects the same checkout without
// opening anything print one sentence rather than two.
func readOnlyMediaWriteFailure(path, blocked, what string, cause error) *app.DomainError {
	return app.NewError(
		app.CodeRuntimePathUnwritable,
		app.KindUnavailable,
		"the runtime database is on a read-only filesystem, so Mindrail could not "+what,
		"Mindrail can read the state already recorded here but cannot record anything new, and no permission change or free space will alter that while the mount refuses writes.",
		"remount the filesystem holding "+describeDatabaseFile(blocked)+" read-write, or point MINDRAIL_RUNTIME_DIR at a writable location",
	).WithMetadata("condition", "read_only_filesystem").
		WithMetadata("path", path).
		WithCause(fmt.Errorf("%w: %w", ErrReadOnly, cause))
}

// describeRefusedPaths renders the remedy's subject: every file in the
// database's footprint that will not take a write, or a name for the database
// when the path could not be resolved at all.
func describeRefusedPaths(path string) string {
	if path == "" {
		return "the runtime database file and the directory holding it"
	}
	return strings.Join(RefusedWritePaths(path), ", ")
}

// diskFullFailure is kept apart from the read-only case because no permission
// change clears it: telling someone whose disk is full to chmod a file is the
// kind of remedy that cannot succeed.
func diskFullFailure(ctx context.Context, q Querier, what string, cause error) *app.DomainError {
	path := mainDatabaseFile(ctx, q)

	// The same corroboration the open path makes, for the same reason: this
	// branch is reached for SQLITE_IOERR_SHMSIZE as well as SQLITE_FULL, and only
	// the second has already asked the kernel about space (finding F03). A lookup
	// that could not name the file cannot be corroborated either way, and keeps
	// the sentence it had.
	if path != "" {
		if free, corroborated := spaceCorroboratesFullDisk(path); !corroborated {
			return sizeLimitWriteFailure(path, free, what, cause)
		}
	}

	return app.NewError(
		app.CodeRuntimePathUnwritable,
		app.KindUnavailable,
		"the disk holding the runtime database is full, so Mindrail could not "+what,
		"Nothing was written; Mindrail cannot record any new state until there is space for it.",
		"free space on the filesystem holding "+describeDatabaseFile(path),
	).WithMetadata("condition", "disk_full").
		WithMetadata("path", path).
		WithCause(fmt.Errorf("%w: %w", ErrDiskFull, cause))
}

// sizeLimitWriteFailure is diskFullFailure's counterpart for a write refused by
// a size ceiling rather than by an empty filesystem. Its wording matches
// sizeLimitOpenFailure, so the `init` that meets the limit while migrating and
// the `status` that meets it at the door print one condition rather than two.
func sizeLimitWriteFailure(path string, free filesystem.FreeSpace, what string, cause error) *app.DomainError {
	return app.NewError(
		app.CodeRuntimePathUnwritable,
		app.KindUnavailable,
		"a size limit refused the runtime database at "+path+" the room it asked for, so Mindrail could not "+what+
			"; the filesystem holding it reports "+strconv.FormatInt(free.AvailableBytes, 10)+" bytes available",
		"Nothing was written, and freeing space will not change that: the filesystem has room, and a limit above it is what refused the write.",
		"Check for a file-size limit on this process (`ulimit -f`, or a systemd LimitFSIZE= setting).",
		"Check whether this user's disk quota is exhausted on the filesystem holding "+path+".",
		"Check the database's own page ceiling with `PRAGMA max_page_count` on "+path+".",
		"Or point MINDRAIL_RUNTIME_DIR at a location none of those limits reach.",
	).WithMetadata("condition", "size_limit").
		WithMetadata("path", path).
		WithMetadata("available_bytes", strconv.FormatInt(free.AvailableBytes, 10)).
		WithCause(fmt.Errorf("%w: %w", ErrSizeLimit, cause))
}

// describeDatabaseFile renders the path for a remedy, falling back to a name
// rather than to an empty pair of quotes when the lookup could not answer.
func describeDatabaseFile(path string) string {
	if path == "" {
		return "the runtime database file"
	}
	return path
}

// mainDatabaseFile asks the connection which file it is attached to, so a remedy
// can name the path without the packages that write rows having to carry it.
//
// The alternative was to thread the path through Migrator and Store, which take
// a *sql.DB precisely so they cannot grow a dependency on this package's handle
// type. An empty answer is a normal outcome, not a failure: it only costs the
// remedy its path, and a remedy is being built because something has already
// gone wrong.
func mainDatabaseFile(ctx context.Context, q Querier) string {
	if q == nil {
		return ""
	}

	var file string
	if err := q.QueryRowContext(ctx,
		`SELECT file FROM pragma_database_list WHERE name = 'main'`).Scan(&file); err != nil {
		return ""
	}
	return file
}
