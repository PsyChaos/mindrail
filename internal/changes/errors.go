package changes

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/storage"
)

func invalidInput(why string) error {
	return app.NewError(app.CodeCommandLineInvalid, app.KindUsage, why,
		"No change facts were changed.", "Pass a valid task, scope and file state.")
}

func corruptState(err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	return app.NewError(app.CodeIndexStateCorrupt, app.KindFailed,
		"the persisted change facts could not be read consistently",
		"Change discovery answers cannot be trusted until the state is repaired.",
		"Preserve the runtime database and restore its change state from a known-good backup before retrying.").WithCause(err)
}

func schemaBehind(applied int64) error {
	return app.NewError(app.CodeMigrationFailed, app.KindFailed,
		fmt.Sprintf("Schema is behind this binary: the runtime database has applied migrations up to %d and the changes store needs %d", applied, TableSchemaVersion),
		"Change commands cannot run; nothing was read and nothing was written.",
		"Run `mindrail init` to apply the pending migrations, then re-run the command.").
		WithMetadata("applied_version", strconv.FormatInt(applied, 10)).
		WithMetadata("required_version", strconv.FormatInt(TableSchemaVersion, 10))
}

func writeFailure(ctx context.Context, db *sql.DB, what string, err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	if _, ok := app.PayloadOf(err); ok {
		return err
	}
	wrapped := fmt.Errorf("write %s: %w", what, err)
	if classified := storage.WriteFailure(ctx, db, what, wrapped); classified != nil {
		return classified
	}
	return wrapped
}
