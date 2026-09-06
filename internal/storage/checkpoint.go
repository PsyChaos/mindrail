package storage

import (
	"context"
	"database/sql"
	"fmt"

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
func Checkpoint(ctx context.Context, db *sql.DB) (CheckpointResult, error) {
	if db == nil {
		return CheckpointResult{}, nil
	}

	var busy int
	var result CheckpointResult
	row := db.QueryRowContext(ctx, `PRAGMA wal_checkpoint(TRUNCATE)`)
	if err := row.Scan(&busy, &result.LogFrames, &result.CheckpointedFrames); err != nil {
		return CheckpointResult{}, checkpointFailure(ctx, db, err)
	}

	result.Busy = busy != 0
	return result, nil
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
