package changes

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/coordination"
	"github.com/PsyChaos/mindrail/internal/identity"
	"github.com/PsyChaos/mindrail/internal/storage"
)

// contentHash reads one scope file: its SHA-256, or empty when there is
// nothing to hash. A missing file baselines empty (it may appear); an
// unreadable present file fails the capture (fail-closed on real I/O
// problems, best-effort only on absence).
func contentHash(path string) (string, error) {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", nil
	}
	content, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:]), nil
}

// CaptureBaseline replaces one task's baseline with its scope's current
// hashes (decision D-116): no stacking, so the latest call is the truth
// about what the agent declared. Scope paths must be clean and absolute;
// containment against a worktree root is the caller's business (future
// drivers resolve it; tests pass temp roots directly).
func (s *Store) CaptureBaseline(ctx context.Context, taskID string, paths []string, operationID string) (BaselineSummary, error) {
	if taskID == "" {
		return BaselineSummary{}, invalidInput("baseline capture needs a task")
	}
	for _, path := range paths {
		if !filepath.IsAbs(path) || filepath.Clean(path) != path {
			return BaselineSummary{}, invalidInput("baseline scope needs clean absolute paths")
		}
	}
	if operationID != "" && !coordination.ValidOperationID(operationID) {
		return BaselineSummary{}, invalidInput("operation id is not representable")
	}
	hashes := make(map[string]string, len(paths))
	for _, path := range paths {
		hash, err := contentHash(path)
		if err != nil {
			return BaselineSummary{}, err
		}
		hashes[path] = hash
	}
	summary := BaselineSummary{TaskID: taskID, Files: len(hashes)}
	params := map[string]any{"task": taskID, "paths": paths}
	err := s.withOperation(ctx, operationID, taskID, "baseline.capture", params, &summary,
		func(ctx context.Context, tx *sql.Tx) error {
			if _, err := tx.ExecContext(ctx, `DELETE FROM change_baselines WHERE task_id = ?`, taskID); err != nil {
				return err
			}
			for path, hash := range hashes {
				if _, err := tx.ExecContext(ctx, `INSERT INTO change_baselines
					(task_id, path, content_hash, captured_at) VALUES (?, ?, ?, ?)`,
					taskID, path, hash, app.FormatTime(s.clock.Now())); err != nil {
					return err
				}
			}
			return nil
		})
	if err != nil {
		return BaselineSummary{}, err
	}
	return summary, nil
}

// BaselineSummary is what a capture recorded: the task and how many scope
// files it hashed. Empty hashes (missing/non-regular files) count — they are
// declared scope, not skipped work.
type BaselineSummary struct {
	TaskID string
	Files  int
}

// ReadBaseline returns one task's baseline as path→hash. An empty map means
// no baseline was ever captured, which discovery treats as "no declared
// truth", never as "nothing changed".
func (s *Store) ReadBaseline(ctx context.Context, taskID string) (map[string]string, error) {
	if taskID == "" {
		return nil, invalidInput("baseline read needs a task")
	}
	if err := s.requireSchema(ctx); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT path, content_hash FROM change_baselines
		WHERE task_id = ? ORDER BY path`, taskID)
	if err != nil {
		return nil, corruptState(err)
	}
	defer rows.Close()
	baseline := map[string]string{}
	for rows.Next() {
		var path, hash string
		if err := rows.Scan(&path, &hash); err != nil {
			return nil, corruptState(err)
		}
		baseline[path] = hash
	}
	if err := rows.Err(); err != nil {
		return nil, corruptState(err)
	}
	return baseline, nil
}

// ClearBaseline drops one task's baseline rows. It is the only remover
// besides the next capture: discovery never clears (AC-02.4), so a baseline
// survives every read path until replaced or explicitly cleared.
func (s *Store) ClearBaseline(ctx context.Context, taskID string) error {
	if taskID == "" {
		return invalidInput("baseline clear needs a task")
	}
	if err := s.requireSchema(ctx); err != nil {
		return err
	}
	err := storage.InTx(ctx, s.db, func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `DELETE FROM change_baselines WHERE task_id = ?`, taskID)
		return err
	})
	if err != nil {
		return writeFailure(ctx, s.db, "baseline clear", err)
	}
	return nil
}

// Change is one task's open answer to "what actually changed".
type Change struct {
	ID          string
	TaskID      string
	OperationID string
	UpdatedAt   string
}

// EnsureOpenChange returns the task's open Change, creating it when absent.
// A task holds at most one (D-115): repeated calls converge, never
// duplicate — the insert carries ON CONFLICT DO NOTHING so two racers agree
// on the first row instead of one failing. An empty task mints a fresh
// retroactive row per call unless an operation id replays one (AC-15's
// letter for the NULL-task flow).
func (s *Store) EnsureOpenChange(ctx context.Context, taskID string, operationID string) (Change, error) {
	if operationID != "" && !coordination.ValidOperationID(operationID) {
		return Change{}, invalidInput("operation id is not representable")
	}
	if err := s.requireSchema(ctx); err != nil {
		return Change{}, err
	}
	if taskID != "" {
		var change Change
		var operation sql.NullString
		err := s.db.QueryRowContext(ctx, `SELECT change_id, task_id, operation_id, updated_at
			FROM changes WHERE task_id = ?`, taskID).
			Scan(&change.ID, &change.TaskID, &operation, &change.UpdatedAt)
		change.OperationID = operation.String
		if err == nil {
			return change, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return Change{}, corruptState(err)
		}
	}
	var change Change
	params := map[string]any{"task": taskID}
	err := s.withOperation(ctx, operationID, taskID, "change.ensure", params, &change,
		func(ctx context.Context, tx *sql.Tx) error {
			now := app.FormatTime(s.clock.Now())
			change = Change{ID: identity.NewID("CHG"), TaskID: taskID, OperationID: operationID, UpdatedAt: now}
			if _, err := tx.ExecContext(ctx, `INSERT INTO changes
				(change_id, task_id, operation_id, created_at, updated_at)
				VALUES (?, NULLIF(?, ''), NULLIF(?, ''), ?, ?)
				ON CONFLICT(task_id) WHERE task_id IS NOT NULL DO NOTHING`,
				change.ID, taskID, operationID, now, now); err != nil {
				return err
			}
			return nil
		})
	if err != nil {
		return Change{}, err
	}
	if taskID != "" {
		// Read back whoever owns the task: the winner on a race, the
		// recorded row on a replay, the minted row otherwise. Never trust
		// the minted id past this point.
		return s.readTaskChange(ctx, taskID)
	}
	if change.ID == "" {
		return Change{}, corruptState(errors.New("changes: operation replayed without a change id"))
	}
	return s.readChange(ctx, change.ID)
}

// readTaskChange loads one task's open Change.
func (s *Store) readTaskChange(ctx context.Context, taskID string) (Change, error) {
	var change Change
	var operation sql.NullString
	err := s.db.QueryRowContext(ctx, `SELECT change_id, task_id, operation_id, updated_at
		FROM changes WHERE task_id = ?`, taskID).
		Scan(&change.ID, &change.TaskID, &operation, &change.UpdatedAt)
	change.OperationID = operation.String
	if err != nil {
		return Change{}, corruptState(err)
	}
	return change, nil
}

// readChange loads one Change row by id.
func (s *Store) readChange(ctx context.Context, changeID string) (Change, error) {
	var change Change
	var task, operation sql.NullString
	err := s.db.QueryRowContext(ctx, `SELECT change_id, task_id, operation_id, updated_at
		FROM changes WHERE change_id = ?`, changeID).
		Scan(&change.ID, &task, &operation, &change.UpdatedAt)
	if err != nil {
		return Change{}, corruptState(err)
	}
	change.TaskID, change.OperationID = task.String, operation.String
	return change, nil
}
