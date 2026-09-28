package continuity

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/identity"
)

const SchemaVersion = 12

type CreateIntent struct {
	ProjectID, WorkspaceID, TaskID, PredecessorSessionID string
	PredecessorRunHash                                   []byte
	Kind                                                 IntentKind
	TargetTaskID                                         string
	ExpiresAt                                            time.Time
}

type Store interface {
	Create(context.Context, CreateIntent) (Intent, error)
	Get(context.Context, string) (Intent, error)
	Save(context.Context, Intent, int64) (Intent, error)
	FindActive(context.Context, string, string) (Intent, error)
}

func (s *SQLStore) FindActive(ctx context.Context, taskID, predecessorSessionID string) (Intent, error) {
	if err := requireSchema(ctx, s.db); err != nil {
		return Intent{}, err
	}
	intent, err := scanIntent(s.db.QueryRowContext(ctx, intentColumns+` WHERE task_id = ? AND predecessor_session_id = ?
		AND state NOT IN ('COMPLETED','MANUAL_REQUIRED','CANCELLED','EXPIRED') ORDER BY intent_id DESC LIMIT 1`, taskID, predecessorSessionID))
	if errors.Is(err, sql.ErrNoRows) {
		return Intent{}, ErrIntentNotFound
	}
	if err != nil {
		return Intent{}, err
	}
	if !intent.ExpiresAt.After(s.clock.Now()) {
		intent.State = StateExpired
		if _, saveErr := s.Save(ctx, intent, intent.Revision); saveErr != nil {
			return Intent{}, saveErr
		}
		return Intent{}, ErrIntentNotFound
	}
	return intent, nil
}

type SQLStore struct {
	db    *sql.DB
	clock app.Clock
}

func NewSQLStore(db *sql.DB, clock app.Clock) (*SQLStore, error) {
	if db == nil || clock == nil {
		return nil, errors.New("continuity store needs a database handle and clock")
	}
	return &SQLStore{db: db, clock: clock}, nil
}

func (s *SQLStore) Create(ctx context.Context, in CreateIntent) (Intent, error) {
	if err := requireSchema(ctx, s.db); err != nil {
		return Intent{}, err
	}
	if err := validateCreate(in, s.clock.Now()); err != nil {
		return Intent{}, err
	}
	now := s.clock.Now().UTC()
	intent := Intent{
		ID: identity.NewID("CTI"), ProjectID: in.ProjectID, WorkspaceID: in.WorkspaceID,
		TaskID: in.TaskID, PredecessorSessionID: in.PredecessorSessionID,
		PredecessorRunHash: append([]byte(nil), in.PredecessorRunHash...), Kind: in.Kind,
		TargetTaskID: in.TargetTaskID, State: StateObserving, Revision: 1,
		LastUsedBasisPoints: -1, CreatedAt: now, UpdatedAt: now, ExpiresAt: in.ExpiresAt.UTC(),
	}
	var target any
	if in.TargetTaskID != "" {
		target = in.TargetTaskID
	}
	result, err := s.db.ExecContext(ctx, `INSERT INTO continuity_intents (
		intent_id, project_id, workspace_id, task_id, predecessor_session_id,
		predecessor_run_hash, kind, target_task_id, state, revision,
		last_observation_sequence, last_used_basis_points, consecutive_handoff_observations,
		created_at, updated_at, expires_at)
	SELECT ?, ?, ?, ?, ?, ?, ?, ?, 'OBSERVING', 1, 0, -1, 0, ?, ?, ?
	FROM projects p
	JOIN workspaces w ON w.workspace_id = ? AND w.project_id = p.project_id
	JOIN sessions s ON s.session_id = ? AND s.workspace_id = w.workspace_id
	JOIN tasks t ON t.task_id = ? AND t.project_id = p.project_id
	WHERE p.project_id = ?
	AND (? IS NULL OR EXISTS (
		SELECT 1 FROM tasks target WHERE target.task_id = ? AND target.project_id = p.project_id
	))`, intent.ID, in.ProjectID, in.WorkspaceID, in.TaskID, in.PredecessorSessionID,
		in.PredecessorRunHash, string(in.Kind), target,
		app.FormatTime(now), app.FormatTime(now), app.FormatTime(in.ExpiresAt),
		in.WorkspaceID, in.PredecessorSessionID, in.TaskID, in.ProjectID, target, target)
	if err != nil {
		return Intent{}, fmt.Errorf("create continuity intent: %w", err)
	}
	written, err := result.RowsAffected()
	if err != nil {
		return Intent{}, fmt.Errorf("create continuity intent: %w", err)
	}
	if written != 1 {
		return Intent{}, errors.New("continuity intent attribution does not agree")
	}
	return intent, nil
}

func (s *SQLStore) Get(ctx context.Context, intentID string) (Intent, error) {
	if err := requireSchema(ctx, s.db); err != nil {
		return Intent{}, err
	}
	if intentID == "" {
		return Intent{}, errors.New("continuity intent id is required")
	}
	intent, err := scanIntent(s.db.QueryRowContext(ctx, intentColumns+` WHERE intent_id = ?`, intentID))
	if errors.Is(err, sql.ErrNoRows) {
		return Intent{}, ErrIntentNotFound
	}
	return intent, err
}

func (s *SQLStore) Save(ctx context.Context, next Intent, expectedRevision int64) (Intent, error) {
	if err := requireSchema(ctx, s.db); err != nil {
		return Intent{}, err
	}
	if next.ID == "" || expectedRevision < 1 || next.Revision != expectedRevision {
		return Intent{}, errors.New("continuity save needs an intent and its current revision")
	}
	if !validState(next.State) || next.LastObservationSequence < 0 || next.LastUsedBasisPoints < -1 || next.LastUsedBasisPoints > 10_000 {
		return Intent{}, errors.New("continuity save contains invalid state or observation metadata")
	}
	current, err := s.Get(ctx, next.ID)
	if err != nil {
		return Intent{}, err
	}
	if current.Revision != expectedRevision {
		return Intent{}, ErrConflict
	}
	if !validTransition(current.State, next.State) {
		return Intent{}, fmt.Errorf("continuity state cannot move from %s to %s", current.State, next.State)
	}
	now := s.clock.Now().UTC()
	result, err := s.db.ExecContext(ctx, `UPDATE continuity_intents SET
		state = ?, revision = revision + 1, last_observation_sequence = ?,
		last_used_basis_points = ?, consecutive_handoff_observations = ?,
		checkpoint_id = NULLIF(?, ''), host_operation_id = NULLIF(?, ''),
		takeover_token_hash = ?, successor_run_hash = ?, successor_session_id = NULLIF(?, ''),
		failure_code = NULLIF(?, ''), updated_at = ?, expires_at = ?
	WHERE intent_id = ? AND revision = ?`, string(next.State), next.LastObservationSequence,
		next.LastUsedBasisPoints, next.ConsecutiveHandoffObservations, next.CheckpointID,
		next.HostOperationID, nullableBytes(next.TakeoverTokenHash), nullableBytes(next.SuccessorRunHash),
		next.SuccessorSessionID, next.FailureCode, app.FormatTime(now), app.FormatTime(next.ExpiresAt),
		next.ID, expectedRevision)
	if err != nil {
		return Intent{}, fmt.Errorf("save continuity intent: %w", err)
	}
	written, err := result.RowsAffected()
	if err != nil {
		return Intent{}, fmt.Errorf("save continuity intent: %w", err)
	}
	if written != 1 {
		if _, getErr := s.Get(ctx, next.ID); errors.Is(getErr, ErrIntentNotFound) {
			return Intent{}, ErrIntentNotFound
		}
		return Intent{}, ErrConflict
	}
	return s.Get(ctx, next.ID)
}

func validTransition(from, to State) bool {
	if from == to {
		return true
	}
	if to == StateCancelled || to == StateExpired {
		return !terminal(from)
	}
	if to == StateManualRequired {
		return !terminal(from)
	}
	switch from {
	case StateObserving:
		return to == StateWarned
	case StateWarned:
		return to == StateCheckpointed
	case StateCheckpointed:
		return to == StateSpawnRequested
	case StateSpawnRequested:
		return to == StateSpawnReady
	case StateSpawnReady:
		return to == StateHandedOff
	case StateHandedOff:
		return to == StateClaimed
	case StateClaimed:
		return to == StateResumed
	case StateResumed:
		return to == StateCompleted
	default:
		return false
	}
}

const intentColumns = `SELECT intent_id, project_id, workspace_id, task_id,
	predecessor_session_id, predecessor_run_hash, kind, target_task_id, state, revision,
	last_observation_sequence, last_used_basis_points, consecutive_handoff_observations,
	checkpoint_id, host_operation_id, takeover_token_hash, successor_run_hash,
	successor_session_id, failure_code, created_at, updated_at, expires_at
	FROM continuity_intents`

type rowScanner interface{ Scan(...any) error }

func scanIntent(row rowScanner) (Intent, error) {
	var out Intent
	var kind, state, created, updated, expires string
	var target, checkpoint, operation, successorSession, failure sql.NullString
	var tokenHash, successorHash []byte
	err := row.Scan(&out.ID, &out.ProjectID, &out.WorkspaceID, &out.TaskID,
		&out.PredecessorSessionID, &out.PredecessorRunHash, &kind, &target, &state, &out.Revision,
		&out.LastObservationSequence, &out.LastUsedBasisPoints, &out.ConsecutiveHandoffObservations,
		&checkpoint, &operation, &tokenHash, &successorHash, &successorSession, &failure,
		&created, &updated, &expires)
	if err != nil {
		return Intent{}, err
	}
	out.Kind, out.State = IntentKind(kind), State(state)
	out.TargetTaskID, out.CheckpointID, out.HostOperationID = target.String, checkpoint.String, operation.String
	out.TakeoverTokenHash, out.SuccessorRunHash = tokenHash, successorHash
	out.SuccessorSessionID, out.FailureCode = successorSession.String, failure.String
	if !validState(out.State) {
		return Intent{}, fmt.Errorf("read continuity intent: invalid state %q", out.State)
	}
	var parseErr error
	if out.CreatedAt, parseErr = app.ParseTime(created); parseErr != nil {
		return Intent{}, fmt.Errorf("read continuity created_at: %w", parseErr)
	}
	if out.UpdatedAt, parseErr = app.ParseTime(updated); parseErr != nil {
		return Intent{}, fmt.Errorf("read continuity updated_at: %w", parseErr)
	}
	if out.ExpiresAt, parseErr = app.ParseTime(expires); parseErr != nil {
		return Intent{}, fmt.Errorf("read continuity expires_at: %w", parseErr)
	}
	return out, nil
}

func validateCreate(in CreateIntent, now time.Time) error {
	if in.ProjectID == "" || in.WorkspaceID == "" || in.TaskID == "" || in.PredecessorSessionID == "" || len(in.PredecessorRunHash) != 32 {
		return errors.New("continuity intent needs attributed ids and a 32-byte run hash")
	}
	if in.Kind != IntentSameTask && in.Kind != IntentNextTask {
		return errors.New("continuity intent kind is invalid")
	}
	if in.Kind == IntentNextTask && in.TargetTaskID == "" {
		return errors.New("next-task continuity needs an explicit target")
	}
	if in.Kind == IntentSameTask && in.TargetTaskID != "" && in.TargetTaskID != in.TaskID {
		return errors.New("same-task continuity target must be the current task")
	}
	if !in.ExpiresAt.After(now) {
		return errors.New("continuity intent expiry must be in the future")
	}
	return nil
}

func validState(state State) bool {
	switch state {
	case StateObserving, StateWarned, StateCheckpointed, StateSpawnRequested, StateSpawnReady,
		StateHandedOff, StateClaimed, StateResumed, StateCompleted, StateManualRequired, StateCancelled, StateExpired:
		return true
	default:
		return false
	}
}

func nullableBytes(value []byte) any {
	if len(value) == 0 {
		return nil
	}
	return value
}

func requireSchema(ctx context.Context, db *sql.DB) error {
	var version sql.NullInt64
	if err := db.QueryRowContext(ctx, `SELECT max(version) FROM schema_migrations`).Scan(&version); err != nil {
		return fmt.Errorf("continuity schema check: %w", err)
	}
	if version.Int64 < SchemaVersion {
		return fmt.Errorf("continuity schema is behind: got %d, need %d", version.Int64, SchemaVersion)
	}
	return nil
}
