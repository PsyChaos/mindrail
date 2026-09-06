package storage

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"time"

	"github.com/PsyChaos/mindrail/internal/app"
)

// CheckpointResult is what one checkpoint did, in the driver's own terms.
//
// Busy is the outcome that is not a failure: another connection held a read
// lock, the log was left where it is, and the next writer will move it. Nothing
// was lost and nothing needs remedying, so it is a separate field rather than an
// error a caller would have to classify back out.
type CheckpointResult struct {
	Busy bool

	// LogFrames is how many frames the write-ahead log held, and
	// CheckpointedFrames how many were moved into the database file. They are
	// carried so a caller can say what was left behind rather than only that
	// something was.
	LogFrames          int
	CheckpointedFrames int
}

// Checkpoint moves the write-ahead log back into the database file and truncates
// it, so that what is on disk when a command finishes is what that command left
// there.
//
// It exists because a command's verdict used to be taken while its own last
// write was still ahead of it. `mindrail init` opens a WAL database, migrates
// it, registers the workspace, reads the disk, prints READY FOR TARGETED WORK —
// and only then closes the handle. Closing is a write: SQLite checkpoints the
// log into the main file, and on a filesystem with room for the log but not for
// the checkpoint that is the write that fails. sqlite3_close discards the
// failure, the log and the shared-memory index stay on disk at their full size,
// and the repository `init` has just called ready is one every later command
// exits 4 on. Nothing changed on disk between the two answers; the reading was
// simply taken too early (finding F01).
//
// TRUNCATE rather than PASSIVE, because the point is to end with the space
// accounted for. A passive checkpoint leaves the log at its full length, which is
// the 74 KiB that decided the reproduction, and a reading taken over it would
// understate the free space by exactly the amount the close was about to return.

// CheckpointBudget bounds the wait for the database.
//
// TRUNCATE waits for every reader to leave, and the wait it inherits is the
// database's own five-second busy timeout — paid by `init`, on a repository
// where any other process happens to be reading, for a step that is an
// optimisation of the close. One held read lock took `init` from about ten
// milliseconds to five seconds while `status` and `doctor` stayed under forty.
//
// A budget is the right answer rather than a passive checkpoint, because the
// write is the point: it is the write the close would have attempted, and
// attempting it here is what lets its failure reach the report. Not getting the
// database is a different outcome from the write failing, and it is not a
// failure of anything.
const CheckpointBudget = 250 * time.Millisecond

func Checkpoint(ctx context.Context, db *sql.DB) (CheckpointResult, error) {
	if db == nil {
		return CheckpointResult{}, nil
	}

	// One connection for both statements, because the budget below is a
	// property of a connection and a pool would happily run the pragma on one
	// and the checkpoint on another.
	conn, err := db.Conn(ctx)
	if err != nil {
		return checkpointOutcome(ctx, db, err)
	}
	defer func() { _ = conn.Close() }()

	restore, err := boundCheckpointWait(ctx, conn)
	if err != nil {
		return checkpointOutcome(ctx, db, err)
	}
	defer restore()

	var busy int
	var result CheckpointResult
	row := conn.QueryRowContext(ctx, `PRAGMA wal_checkpoint(TRUNCATE)`)
	if err := row.Scan(&busy, &result.LogFrames, &result.CheckpointedFrames); err != nil {
		return checkpointOutcome(ctx, db, err)
	}

	result.Busy = busy != 0
	return result, nil
}

// boundCheckpointWait caps how long this one connection waits for the database,
// and returns the undo so the connection goes back to the pool as it came.
//
// The cap has to be the busy timeout rather than a context deadline. The wait
// happens inside SQLite's own busy handler, which sleeps without consulting
// anything Go can cancel: a 250 ms context around the query left `init` taking
// the full five seconds anyway, measured.
func boundCheckpointWait(ctx context.Context, conn *sql.Conn) (func(), error) {
	var previous int64
	if err := conn.QueryRowContext(ctx, `PRAGMA busy_timeout`).Scan(&previous); err != nil {
		return nil, err
	}

	budget := strconv.FormatInt(CheckpointBudget.Milliseconds(), 10)
	if _, err := conn.ExecContext(ctx, `PRAGMA busy_timeout = `+budget); err != nil {
		return nil, err
	}

	return func() {
		// Without cancellation: a connection handed back carrying a budget meant
		// for one statement would shorten the next caller's wait, and the path
		// this runs on is frequently one where ctx is already cancelled.
		_, _ = conn.ExecContext(context.WithoutCancel(ctx),
			`PRAGMA busy_timeout = `+strconv.FormatInt(previous, 10))
	}, nil
}

// checkpointOutcome decides what a failed checkpoint means.
//
// A caller that pressed Ctrl-C did not find anything wrong: the log stays where
// it is and the next writer moves it. Reporting that as a database failure had
// the command fabricate a fault — RUNTIME_DB_UNAVAILABLE at exit 4, over a
// repository whose own readiness line in the same output said READY and whose
// printed remedy, `mindrail doctor`, then exited 0 and found nothing.
//
// The context is asked rather than errors.Is on the driver's error, because an
// interrupted query comes back as whatever the driver chose to call an
// interrupt, and the context is the thing that actually knows.
func checkpointOutcome(ctx context.Context, db *sql.DB, err error) (CheckpointResult, error) {
	if ctx.Err() != nil {
		return CheckpointResult{Busy: true}, nil
	}
	return CheckpointResult{}, checkpointFailure(ctx, db, err)
}

// checkpointFailure names the condition behind a refused checkpoint.
//
// It goes through WriteFailure because a checkpoint is a write and has no
// conditions of its own: a full filesystem, a size limit, a read-only mount and
// a held lock are the same four things they are for every other write, and
// naming them a second time here is how two remedies for one condition start.
// Only the fallback is local, for a driver error none of them recognise.
func checkpointFailure(ctx context.Context, db *sql.DB, cause error) error {
	if named := WriteFailure(ctx, db, "finish writing the runtime database", cause); named != nil {
		return named
	}

	return app.NewError(
		app.CodeRuntimeDBUnavailable,
		app.KindUnavailable,
		"the write-ahead log could not be written back into the runtime database",
		"The state this command recorded is in the log rather than in the database file, so a later command may find work left over from this one.",
		"Run `mindrail doctor` to see the runtime database's current condition.",
	).WithCause(fmt.Errorf("%w: %w", ErrCheckpointFailed, cause))
}
