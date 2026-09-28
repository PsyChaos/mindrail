package coordination

import (
	"context"
	"database/sql"
	"errors"

	"github.com/PsyChaos/mindrail/internal/app"
)

// authorizeContinuityTaskMutation keeps every coordination entry point behind
// the same durable handoff reservation. A RESUMED successor is the only session
// allowed to mutate the reserved task; before that, even legacy claim paths are
// closed.
func authorizeContinuityTaskMutation(ctx context.Context, tx *sql.Tx, taskID, sessionID string) error {
	var state, predecessor, successor string
	err := tx.QueryRowContext(ctx, `SELECT state, predecessor_session_id, COALESCE(successor_session_id, '')
		FROM continuity_intents
		WHERE (CASE WHEN kind = 'NEXT_TASK' THEN target_task_id ELSE task_id END) = ?
		  AND state IN ('SPAWN_REQUESTED','SPAWN_READY','HANDED_OFF','CLAIMED','RESUMED','COMPLETED')
		ORDER BY updated_at DESC, intent_id DESC LIMIT 1`, taskID).Scan(&state, &predecessor, &successor)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if (state == "RESUMED" || state == "COMPLETED") && successor == sessionID {
		return nil
	}
	if (state == "SPAWN_REQUESTED" || state == "SPAWN_READY") && predecessor == sessionID {
		return nil
	}
	return app.NewError(app.CodeLeaseConflict, app.KindFailed,
		"task is reserved for a continuity successor",
		"This session cannot claim, lease, move, or complete the reserved task.",
		"Use the prepared successor's takeover token and run key to resume the task.",
	).WithMetadata("task_id", taskID).WithMetadata("continuity_state", state)
}
