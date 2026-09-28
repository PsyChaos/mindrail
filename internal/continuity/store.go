package continuity

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/identity"
)

const SchemaVersion = 13

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
	ClaimTakeover(context.Context, string, []byte, []byte) (Intent, error)
	ResumeTakeover(context.Context, string, []byte, string) (Intent, error)
	CompleteTakeover(context.Context, string, []byte, string) (Intent, error)
	ListRecoverable(context.Context, string) ([]Intent, error)
	MarkActivated(context.Context, string) (Intent, error)
}

func (s *SQLStore) MarkActivated(ctx context.Context, intentID string) (Intent, error) {
	current, err := s.Get(ctx, intentID)
	if err != nil {
		return Intent{}, err
	}
	if current.ActivatedAt != nil {
		return current, nil
	}
	now := s.clock.Now().UTC()
	result, err := s.db.ExecContext(ctx, `UPDATE continuity_intents SET activated_at = ?, revision = revision + 1,
		updated_at = ? WHERE intent_id = ? AND revision = ? AND activated_at IS NULL`,
		app.FormatTime(now), app.FormatTime(now), intentID, current.Revision)
	if err != nil {
		return Intent{}, err
	}
	if written, rowsErr := result.RowsAffected(); rowsErr != nil || written != 1 {
		if latest, getErr := s.Get(ctx, intentID); getErr == nil && latest.ActivatedAt != nil {
			return latest, nil
		}
		return Intent{}, ErrConflict
	}
	return s.Get(ctx, intentID)
}

func (s *SQLStore) ListRecoverable(ctx context.Context, workspaceID string) ([]Intent, error) {
	if err := requireSchema(ctx, s.db); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, intentColumns+` WHERE workspace_id = ? AND state IN
		('WARNED','CHECKPOINTED','SPAWN_REQUESTED','SPAWN_READY','HANDED_OFF') AND expires_at > ? ORDER BY intent_id`, workspaceID, app.FormatTime(s.clock.Now().UTC()))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Intent
	for rows.Next() {
		intent, err := scanIntent(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, intent)
	}
	return out, rows.Err()
}

func (s *SQLStore) ClaimTakeover(ctx context.Context, intentID string, tokenHash, runHash []byte) (Intent, error) {
	if len(tokenHash) != sha256.Size || len(runHash) != sha256.Size {
		return Intent{}, ErrInvalidToken
	}
	current, err := s.Get(ctx, intentID)
	if err != nil {
		return Intent{}, err
	}
	if (current.State == StateClaimed || current.State == StateResumed || current.State == StateCompleted) &&
		bytes.Equal(current.SuccessorRunHash, runHash) && bytes.Equal(current.TakeoverTokenHash, tokenHash) {
		return current, nil
	}
	if current.State != StateHandedOff || !bytes.Equal(current.TakeoverTokenHash, tokenHash) || !current.ExpiresAt.After(s.clock.Now()) {
		return Intent{}, ErrInvalidToken
	}
	result, err := s.db.ExecContext(ctx, `UPDATE continuity_intents SET state = 'CLAIMED', revision = revision + 1,
		successor_run_hash = ?, updated_at = ? WHERE intent_id = ? AND revision = ? AND state = 'HANDED_OFF' AND takeover_token_hash = ?`,
		runHash, app.FormatTime(s.clock.Now().UTC()), intentID, current.Revision, tokenHash)
	if err != nil {
		return Intent{}, fmt.Errorf("claim continuity takeover: %w", err)
	}
	written, err := result.RowsAffected()
	if err != nil {
		return Intent{}, fmt.Errorf("claim continuity takeover: %w", err)
	}
	if written != 1 {
		return Intent{}, ErrTokenConsumed
	}
	return s.Get(ctx, intentID)
}

func (s *SQLStore) ResumeTakeover(ctx context.Context, intentID string, runHash []byte, sessionID string) (Intent, error) {
	if len(runHash) != sha256.Size || sessionID == "" {
		return Intent{}, ErrReservationLost
	}
	current, err := s.Get(ctx, intentID)
	if err != nil {
		return Intent{}, err
	}
	if (current.State == StateResumed || current.State == StateCompleted) && bytes.Equal(current.SuccessorRunHash, runHash) && current.SuccessorSessionID == sessionID {
		return current, nil
	}
	if current.State != StateClaimed || !bytes.Equal(current.SuccessorRunHash, runHash) {
		return Intent{}, ErrReservationLost
	}
	current.State = StateResumed
	current.SuccessorSessionID = sessionID
	return s.Save(ctx, current, current.Revision)
}

func (s *SQLStore) CompleteTakeover(ctx context.Context, intentID string, runHash []byte, sessionID string) (Intent, error) {
	current, err := s.Get(ctx, intentID)
	if err != nil {
		return Intent{}, err
	}
	if current.State == StateCompleted && bytes.Equal(current.SuccessorRunHash, runHash) && current.SuccessorSessionID == sessionID {
		return current, nil
	}
	if current.State != StateResumed || !bytes.Equal(current.SuccessorRunHash, runHash) || current.SuccessorSessionID != sessionID {
		return Intent{}, ErrReservationLost
	}
	current.State = StateCompleted
	return s.Save(ctx, current, current.Revision)
}

func (s *SQLStore) AuthorizeResume(ctx context.Context, taskID, runKey string) error {
	runHash := sha256.Sum256([]byte(runKey))
	var state string
	var reserved []byte
	err := s.db.QueryRowContext(ctx, `SELECT state, successor_run_hash FROM continuity_intents
		WHERE (CASE WHEN kind = 'NEXT_TASK' THEN target_task_id ELSE task_id END) = ? AND state IN ('SPAWN_READY','HANDED_OFF','CLAIMED','RESUMED')
		ORDER BY intent_id DESC LIMIT 1`, taskID).Scan(&state, &reserved)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if (State(state) == StateClaimed || State(state) == StateResumed) && bytes.Equal(reserved, runHash[:]) {
		return nil
	}
	return ErrReservationLost
}

func (s *SQLStore) BindResume(ctx context.Context, taskID, runKey, sessionID string) error {
	runHash := sha256.Sum256([]byte(runKey))
	var intentID, state string
	err := s.db.QueryRowContext(ctx, `SELECT intent_id, state FROM continuity_intents
		WHERE (CASE WHEN kind = 'NEXT_TASK' THEN target_task_id ELSE task_id END) = ?
		  AND state IN ('CLAIMED','RESUMED') ORDER BY intent_id DESC LIMIT 1`, taskID).Scan(&intentID, &state)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	_, err = s.ResumeTakeover(ctx, intentID, runHash[:], sessionID)
	return err
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
	if err := validateIntentPhase(next); err != nil {
		return Intent{}, err
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
		failure_code = NULLIF(?, ''), updated_at = ?, expires_at = ?, activated_at = ?
	WHERE intent_id = ? AND revision = ?`, string(next.State), next.LastObservationSequence,
		next.LastUsedBasisPoints, next.ConsecutiveHandoffObservations, next.CheckpointID,
		next.HostOperationID, nullableBytes(next.TakeoverTokenHash), nullableBytes(next.SuccessorRunHash),
		next.SuccessorSessionID, next.FailureCode, app.FormatTime(now), app.FormatTime(next.ExpiresAt), nullableTime(next.ActivatedAt),
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
	successor_session_id, failure_code, created_at, updated_at, expires_at, activated_at
	FROM continuity_intents`

type rowScanner interface{ Scan(...any) error }

func scanIntent(row rowScanner) (Intent, error) {
	var out Intent
	var kind, state, created, updated, expires string
	var target, checkpoint, operation, successorSession, failure, activated sql.NullString
	var tokenHash, successorHash []byte
	err := row.Scan(&out.ID, &out.ProjectID, &out.WorkspaceID, &out.TaskID,
		&out.PredecessorSessionID, &out.PredecessorRunHash, &kind, &target, &state, &out.Revision,
		&out.LastObservationSequence, &out.LastUsedBasisPoints, &out.ConsecutiveHandoffObservations,
		&checkpoint, &operation, &tokenHash, &successorHash, &successorSession, &failure,
		&created, &updated, &expires, &activated)
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
	if activated.Valid {
		value, err := app.ParseTime(activated.String)
		if err != nil {
			return Intent{}, fmt.Errorf("read continuity activated_at: %w", err)
		}
		out.ActivatedAt = &value
	}
	return out, nil
}

func nullableTime(value *time.Time) any {
	if value == nil {
		return nil
	}
	return app.FormatTime(value.UTC())
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

func validateIntentPhase(intent Intent) error {
	requireCheckpoint := intent.State == StateCheckpointed || intent.State == StateSpawnRequested ||
		intent.State == StateSpawnReady || intent.State == StateHandedOff || intent.State == StateClaimed ||
		intent.State == StateResumed || intent.State == StateCompleted
	requireToken := intent.State == StateSpawnRequested || intent.State == StateSpawnReady || intent.State == StateHandedOff ||
		intent.State == StateClaimed || intent.State == StateResumed || intent.State == StateCompleted
	requirePrepared := intent.State == StateSpawnReady || intent.State == StateHandedOff ||
		intent.State == StateClaimed || intent.State == StateResumed || intent.State == StateCompleted
	requireClaim := intent.State == StateClaimed || intent.State == StateResumed || intent.State == StateCompleted
	requireSession := intent.State == StateResumed || intent.State == StateCompleted
	if requireCheckpoint && intent.CheckpointID == "" {
		return errors.New("continuity phase requires a checkpoint")
	}
	if requireToken && len(intent.TakeoverTokenHash) != sha256.Size {
		return errors.New("continuity phase requires a takeover proposal")
	}
	if requirePrepared && intent.HostOperationID == "" {
		return errors.New("continuity phase requires a prepared successor")
	}
	if requireClaim && len(intent.SuccessorRunHash) != sha256.Size {
		return errors.New("continuity phase requires a claimed successor run")
	}
	if requireSession && intent.SuccessorSessionID == "" {
		return errors.New("continuity phase requires a successor session")
	}
	return nil
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
