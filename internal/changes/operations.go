package changes

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/storage"
)

// requestHash is SHA-256 over a canonical JSON encoding of the command name
// and the parameters the store received, hex-encoded — the MR-004 shape, so
// a "retry" with different parameters is a different request wearing the
// same id.
func requestHash(command string, params any) (string, error) {
	encoded, err := json.Marshal(struct {
		Command string `json:"command"`
		Params  any    `json:"params"`
	}{command, params})
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}

// operationConflict is AC-15's second half: the id is taken by another
// request, so answering from the log would launder one request's result as
// another's.
func operationConflict(operationID string) error {
	return app.NewError(app.CodeOperationIDConflict, app.KindFailed,
		"operation "+operationID+" was already recorded for different parameters",
		"Replaying it would answer this request with another request's change.",
		"Mint a new operation id and retry the command.").WithMetadata("operation_id", operationID)
}

// withOperation runs a mutating write under an operation id. The result
// travels by pointer: replay decodes the recorded JSON into it, and a fresh
// write leaves it for the write closure to fill. Same id plus same request
// hash replays without writing; same id plus another hash conflicts. An
// empty id runs the write directly with no log row.
//
// The log row commits in the SAME transaction as the write it describes
// (coordination's D-71 discipline): a crash between the two would leave a
// committed change no retry could replay. And when the commit loses a race
// on the operation id, the loser re-reads the log instead of surfacing a
// constraint error — same hash replays the winner's result, another hash
// conflicts — so racing retries converge instead of failing raw.
func (s *Store) withOperation(ctx context.Context, operationID, taskID, command string, params any, result any, write func(context.Context, *sql.Tx) error) error {
	if operationID == "" {
		if err := s.requireSchema(ctx); err != nil {
			return err
		}
		if err := storage.InTx(ctx, s.db, write); err != nil {
			return writeFailure(ctx, s.db, command, err)
		}
		return nil
	}
	hash, err := requestHash(command, params)
	if err != nil {
		return err
	}
	if err := s.requireSchema(ctx); err != nil {
		return err
	}
	if replayed, err := s.replayLogged(ctx, operationID, hash, result); err != nil || replayed {
		return err
	}
	err = storage.InTx(ctx, s.db, func(ctx context.Context, tx *sql.Tx) error {
		if err := write(ctx, tx); err != nil {
			return err
		}
		return s.recordOperationTx(ctx, tx, operationID, taskID, hash, result)
	})
	if err == nil {
		return nil
	}
	if replayed, rerr := s.replayLogged(ctx, operationID, hash, result); rerr != nil || replayed {
		return rerr
	}
	return writeFailure(ctx, s.db, command, err)
}

// replayLogged answers from the log when the id names this exact request
// hash, conflicts when it names another, and reports a miss otherwise.
func (s *Store) replayLogged(ctx context.Context, operationID, hash string, result any) (bool, error) {
	var recordedHash, resultJSON string
	err := s.db.QueryRowContext(ctx, `SELECT request_hash, result FROM change_operations
		WHERE operation_id = ?`, operationID).Scan(&recordedHash, &resultJSON)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, corruptState(err)
	}
	if recordedHash != hash {
		return false, operationConflict(operationID)
	}
	if err := json.Unmarshal([]byte(resultJSON), result); err != nil {
		return false, corruptState(err)
	}
	return true, nil
}

// recordOperationTx appends the log row inside the mutation's own
// transaction, keyed by the request hash the replay path recomputes.
func (s *Store) recordOperationTx(ctx context.Context, tx *sql.Tx, operationID, taskID, hash string, result any) error {
	encoded, err := json.Marshal(result)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO change_operations
		(operation_id, task_id, request_hash, result, recorded_at)
		VALUES (?, NULLIF(?, ''), ?, ?, ?)`,
		operationID, taskID, hash, string(encoded), app.FormatTime(s.clock.Now()))
	return err
}
