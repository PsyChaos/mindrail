package agent

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/identity"
)

const (
	HostClaudeCode = "claude-code"
	HostCodex      = "codex"

	HostEventStart     = "start"
	HostEventActivity  = "activity"
	HostEventTelemetry = "telemetry"
	HostEventStop      = "stop"

	hostEventMaxAge     = 30 * time.Second
	hostEventFutureSkew = 5 * time.Second
	pendingHostTTL      = 10 * time.Minute
	pendingHostCap      = 256
)

var hostOpaqueID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:@-]{0,127}$`)
var hostAgentType = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:@/-]{0,63}$`)

var ErrPendingHostGeneration = errors.New("pending host generation must start before later events")

type HostRuntimeAttribution struct {
	ProjectID, WorkspaceID, TaskID, SessionID string
}

type HostRuntimeEventInput struct {
	Host, HostSessionID, AgentID, Generation, AgentType string
	Event                                               string
	Sequence                                            int64
	ObservedAt                                          time.Time
	ModelKey, Effort                                    string
	ContextUsed, ContextLimit                           *int64
}

type HostRuntimeIdentity struct {
	Host, HostSessionID, AgentID, Generation string
}

// HostLifecycleInput is the documented-hook boundary. Sequence, timestamp and
// generation are deliberately absent: Mindrail assigns them transactionally.
type HostLifecycleInput struct {
	Host, HostSessionID, AgentID, AgentType string
	Event, Source                           string
	ModelKey, Effort                        string
	ContextUsed, ContextLimit               *int64
}

type HostRuntime struct {
	RuntimeID, Host, AgentType                 string
	ParentRuntimeID, LifecycleState, LastEvent string
	IsMain                                     bool
	ProducerSequence                           int64
	ObservedAt                                 time.Time
	Accepted                                   bool
}

type HostRuntimeStore struct {
	db    *sql.DB
	clock app.Clock
}

func NewHostRuntimeStore(db *sql.DB, clock app.Clock) (*HostRuntimeStore, error) {
	if db == nil || clock == nil {
		return nil, errors.New("host runtime store needs a database handle and clock")
	}
	return &HostRuntimeStore{db: db, clock: clock}, nil
}

func (s *HostRuntimeStore) ObserveLifecycle(ctx context.Context, attr HostRuntimeAttribution, in HostLifecycleInput) (HostRuntime, error) {
	now := s.clock.Now().UTC()
	if in.Event == HostEventStart {
		return s.Observe(ctx, attr, HostRuntimeEventInput{Host: in.Host, HostSessionID: in.HostSessionID,
			AgentID: in.AgentID, Generation: identity.NewID("GEN"), AgentType: in.AgentType,
			Event: in.Event, Sequence: 1, ObservedAt: now, ModelKey: in.ModelKey, Effort: in.Effort,
			ContextUsed: in.ContextUsed, ContextLimit: in.ContextLimit})
	}
	if in.Host != HostClaudeCode && in.Host != HostCodex || !hostOpaqueID.MatchString(in.HostSessionID) ||
		(in.AgentID != "" && !hostOpaqueID.MatchString(in.AgentID)) {
		return HostRuntime{}, errors.New("host lifecycle identity is invalid")
	}
	if in.AgentID != "" && in.ContextUsed != nil {
		return HostRuntime{}, errors.New("subagent context telemetry is unsupported")
	}
	sessionHash, agentHash := sha256.Sum256([]byte(in.HostSessionID)), sha256.Sum256([]byte(in.AgentID))
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return HostRuntime{}, err
	}
	defer tx.Rollback()
	current, err := getLatestActiveHostRuntime(ctx, tx, in.Host, sessionHash, agentHash)
	if err != nil {
		return HostRuntime{}, errors.New("host runtime generation must start before later events")
	}
	if err := requireRuntimeAttribution(ctx, tx, current.RuntimeID, attr); err != nil {
		return HostRuntime{}, err
	}
	event := HostRuntimeEventInput{Host: in.Host, HostSessionID: in.HostSessionID, AgentID: in.AgentID,
		Generation: "managed-generation", AgentType: in.AgentType, Event: in.Event,
		Sequence: current.ProducerSequence + 1, ObservedAt: now, ModelKey: in.ModelKey, Effort: in.Effort,
		ContextUsed: in.ContextUsed, ContextLimit: in.ContextLimit}
	if err := s.validate(ctx, event); err != nil {
		return HostRuntime{}, err
	}
	out, err := s.advanceTx(ctx, tx, current, event)
	if err != nil {
		return HostRuntime{}, err
	}
	if err := tx.Commit(); err != nil {
		return HostRuntime{}, err
	}
	return out, nil
}

func (s *HostRuntimeStore) ObservePendingLifecycle(ctx context.Context, in HostLifecycleInput) error {
	now := s.clock.Now().UTC()
	if in.Host != HostClaudeCode && in.Host != HostCodex || !hostOpaqueID.MatchString(in.HostSessionID) ||
		(in.AgentID != "" && !hostOpaqueID.MatchString(in.AgentID)) || (in.AgentType != "" && !hostAgentType.MatchString(in.AgentType)) {
		return errors.New("pending host lifecycle identity is invalid")
	}
	if in.AgentID != "" && in.ContextUsed != nil {
		return errors.New("subagent context telemetry is unsupported")
	}
	sessionHash, agentHash := sha256.Sum256([]byte(in.HostSessionID)), sha256.Sum256([]byte(in.AgentID))
	if in.Event == HostEventStart {
		var attr HostRuntimeAttribution
		err := s.db.QueryRowContext(ctx, `SELECT r.project_id,r.workspace_id,r.task_id,r.session_id
			FROM host_runtime_bindings h JOIN agent_runtimes r ON r.runtime_id=h.runtime_id
			WHERE h.host=? AND h.host_session_hash=? AND h.is_main=1 AND h.lifecycle_state='ACTIVE'
			ORDER BY h.observed_at DESC LIMIT 1`, in.Host, sessionHash[:]).Scan(
			&attr.ProjectID, &attr.WorkspaceID, &attr.TaskID, &attr.SessionID)
		if err == nil {
			_, err = s.ObserveLifecycle(ctx, attr, in)
			return err
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
	}
	// Once an official hook identity has been claimed, all later lifecycle
	// events are attributed through that durable binding. They must not fall
	// back into the unaffiliated pending bridge.
	if in.Event != HostEventStart {
		var attr HostRuntimeAttribution
		err := s.db.QueryRowContext(ctx, `SELECT r.project_id,r.workspace_id,r.task_id,r.session_id
			FROM host_runtime_bindings h JOIN agent_runtimes r ON r.runtime_id=h.runtime_id
			WHERE h.host=? AND h.host_session_hash=? AND h.host_agent_hash=? AND h.lifecycle_state='ACTIVE'
			ORDER BY h.observed_at DESC LIMIT 1`, in.Host, sessionHash[:], agentHash[:]).Scan(
			&attr.ProjectID, &attr.WorkspaceID, &attr.TaskID, &attr.SessionID)
		if err == nil {
			_, err = s.ObserveLifecycle(ctx, attr, in)
			return err
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
	}
	if err := s.cleanupPending(ctx, now); err != nil {
		return err
	}
	if in.Event == HostEventStart {
		generation := identity.NewID("GEN")
		generationHash := sha256.Sum256([]byte(generation))
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()
		if _, err := tx.ExecContext(ctx, `UPDATE pending_host_runtime_events SET lifecycle_state='ENDED',last_event='stop',ended_at=?,observed_at=?
			WHERE host=? AND host_session_hash=? AND host_agent_hash=? AND lifecycle_state='ACTIVE'`,
			app.FormatTime(now), app.FormatTime(now), in.Host, sessionHash[:], agentHash[:]); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO pending_host_runtime_events
			(host,host_session_hash,host_agent_hash,generation_hash,agent_type,lifecycle_state,last_event,producer_sequence,
			 model_key,effort,context_used,context_limit,started_at,last_activity_at,observed_at,ended_at,run_key_hash)
			VALUES (?,?,?,?,NULLIF(?,''),'ACTIVE','start',1,NULLIF(?,''),NULLIF(?,''),?,?, ?,?,?,NULL,NULL)`,
			in.Host, sessionHash[:], agentHash[:], generationHash[:], in.AgentType, canonicalModelKey(in.ModelKey),
			canonicalEffort(in.Effort), nullableInt64Pointer(in.ContextUsed), nullableInt64Pointer(in.ContextLimit),
			app.FormatTime(now), app.FormatTime(now), app.FormatTime(now))
		if err != nil {
			return err
		}
		return tx.Commit()
	}
	state, endedAt := "ACTIVE", any(nil)
	if in.Event == HostEventStop {
		state, endedAt = "ENDED", app.FormatTime(now)
	}
	result, err := s.db.ExecContext(ctx, `UPDATE pending_host_runtime_events SET
		agent_type=COALESCE(NULLIF(?,''),agent_type),lifecycle_state=?,last_event=?,producer_sequence=producer_sequence+1,
		model_key=COALESCE(NULLIF(?,''),model_key),effort=COALESCE(NULLIF(?,''),effort),
		context_used=COALESCE(?,context_used),context_limit=COALESCE(?,context_limit),last_activity_at=?,observed_at=?,ended_at=?
		WHERE host=? AND host_session_hash=? AND host_agent_hash=? AND lifecycle_state='ACTIVE'`, in.AgentType, state, in.Event,
		canonicalModelKey(in.ModelKey), canonicalEffort(in.Effort), nullableInt64Pointer(in.ContextUsed), nullableInt64Pointer(in.ContextLimit),
		app.FormatTime(now), app.FormatTime(now), endedAt, in.Host, sessionHash[:], agentHash[:])
	if err != nil {
		return err
	}
	if n, _ := result.RowsAffected(); n == 0 {
		return ErrPendingHostGeneration
	}
	return nil
}

func (s *HostRuntimeStore) AssociatePendingRun(ctx context.Context, host, hostSessionID, agentID, runKey string) error {
	if runKey == "" || len(runKey) > 256 || agentID != "" || !hostOpaqueID.MatchString(hostSessionID) {
		return errors.New("pending host run binding is invalid")
	}
	sessionHash, runHash := sha256.Sum256([]byte(hostSessionID)), sha256.Sum256([]byte(runKey))
	result, err := s.db.ExecContext(ctx, `UPDATE pending_host_runtime_events SET run_key_hash=?
		WHERE host=? AND host_session_hash=? AND lifecycle_state='ACTIVE'`, runHash[:], host, sessionHash[:])
	if err != nil {
		return err
	}
	if n, _ := result.RowsAffected(); n == 0 {
		return errors.New("pending host run has no active generation")
	}
	return nil
}

func (s *HostRuntimeStore) ClaimPendingRun(ctx context.Context, attr HostRuntimeAttribution, runKey, mainRuntimeID string) (HostRuntime, error) {
	if runKey == "" || mainRuntimeID == "" {
		return HostRuntime{}, errors.New("pending host claim needs a run key")
	}
	runHash := sha256.Sum256([]byte(runKey))
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return HostRuntime{}, err
	}
	defer tx.Rollback()
	var matches int
	emptyAgent := sha256.Sum256(nil)
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM pending_host_runtime_events
		WHERE run_key_hash=? AND host_agent_hash=?`, runHash[:], emptyAgent[:]).Scan(&matches); err != nil {
		return HostRuntime{}, err
	}
	if matches != 1 {
		return HostRuntime{}, errors.New("pending host claim needs exactly one main generation")
	}
	var host, agentType, state, lastEvent, model, effort, started, activity, observed string
	var sessionHash, agentHash, generationHash []byte
	var sequence int64
	var used, limit sql.NullInt64
	err = tx.QueryRowContext(ctx, `SELECT host,host_session_hash,host_agent_hash,generation_hash,COALESCE(agent_type,''),
		lifecycle_state,last_event,producer_sequence,COALESCE(model_key,''),COALESCE(effort,''),context_used,context_limit,
		started_at,last_activity_at,observed_at FROM pending_host_runtime_events WHERE run_key_hash=? AND host_agent_hash=?`,
		runHash[:], emptyAgent[:]).Scan(&host, &sessionHash, &agentHash, &generationHash, &agentType,
		&state, &lastEvent, &sequence, &model, &effort, &used, &limit, &started, &activity, &observed)
	if err != nil {
		return HostRuntime{}, err
	}
	runtimeID := mainRuntimeID
	result, err := tx.ExecContext(ctx, `UPDATE agent_runtimes SET client_name=?,last_heartbeat_at=?,
		last_activity_at=?,sequence=sequence+1 WHERE runtime_id=? AND project_id=? AND workspace_id=?
		AND task_id=? AND session_id=? AND ended_at IS NULL`, host, observed, activity, runtimeID,
		attr.ProjectID, attr.WorkspaceID, attr.TaskID, attr.SessionID)
	if err != nil {
		return HostRuntime{}, err
	}
	if n, _ := result.RowsAffected(); n != 1 {
		return HostRuntime{}, errors.New("pending host claim attribution does not agree")
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO host_runtime_bindings
		(runtime_id,host,host_session_hash,host_agent_hash,is_main,parent_runtime_id,generation_hash,agent_type,
		 lifecycle_state,last_event,producer_sequence,observed_at) VALUES (?,?,?,?,?,?,?,NULLIF(?,''),?,?,?,?)`,
		runtimeID, host, sessionHash, agentHash, 1, nil, generationHash, agentType, state, lastEvent, sequence, observed); err != nil {
		return HostRuntime{}, err
	}
	if model != "" || effort != "" || used.Valid {
		_, err = tx.ExecContext(ctx, `INSERT INTO agent_runtime_observations
			(runtime_id,model_key,effort,context_used,context_limit,source,confidence,observed_at,revision,producer_sequence)
			VALUES (?,NULLIF(?,''),NULLIF(?,''),?,?,'host_adapter','self_reported',?,1,?)`,
			runtimeID, model, effort, nullableNullInt64(used), nullableNullInt64(limit), observed, sequence)
		if err != nil {
			return HostRuntime{}, err
		}
	}
	if err := s.claimPendingChildrenTx(ctx, tx, attr, host, sessionHash, runtimeID, runHash); err != nil {
		return HostRuntime{}, err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM pending_host_runtime_events
		WHERE host=? AND host_session_hash=? AND run_key_hash=?`, host, sessionHash, runHash[:]); err != nil {
		return HostRuntime{}, err
	}
	if err := tx.Commit(); err != nil {
		return HostRuntime{}, err
	}
	observedAt, _ := app.ParseTime(observed)
	out := HostRuntime{RuntimeID: runtimeID, Host: host, AgentType: agentType, IsMain: true, LifecycleState: state,
		LastEvent: lastEvent, ProducerSequence: sequence, ObservedAt: observedAt, Accepted: true}
	return out, nil
}

func (s *HostRuntimeStore) claimPendingChildrenTx(ctx context.Context, tx *sql.Tx, attr HostRuntimeAttribution, host string, sessionHash []byte, parentRuntimeID string, runHash [sha256.Size]byte) error {
	emptyAgent := sha256.Sum256(nil)
	rows, err := tx.QueryContext(ctx, `SELECT host_agent_hash,generation_hash,COALESCE(agent_type,''),lifecycle_state,
		last_event,producer_sequence,COALESCE(model_key,''),COALESCE(effort,''),context_used,context_limit,
		started_at,last_activity_at,observed_at,ended_at FROM pending_host_runtime_events
		WHERE host=? AND host_session_hash=? AND run_key_hash=? AND host_agent_hash<>?`,
		host, sessionHash, runHash[:], emptyAgent[:])
	if err != nil {
		return err
	}
	type childRow struct {
		agentHash, generationHash                                               []byte
		agentType, state, lastEvent, model, effort, started, activity, observed string
		sequence                                                                int64
		used, limit                                                             sql.NullInt64
		ended                                                                   sql.NullString
	}
	var children []childRow
	for rows.Next() {
		var child childRow
		if err := rows.Scan(&child.agentHash, &child.generationHash, &child.agentType, &child.state, &child.lastEvent,
			&child.sequence, &child.model, &child.effort, &child.used, &child.limit, &child.started, &child.activity,
			&child.observed, &child.ended); err != nil {
			rows.Close()
			return err
		}
		children = append(children, child)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, child := range children {
		runtimeID := identity.NewID("RUN")
		var endedAt, endReason any
		if child.ended.Valid {
			endedAt, endReason = child.ended.String, RuntimeEndCompleted
		}
		result, err := tx.ExecContext(ctx, `INSERT INTO agent_runtimes
			(runtime_id,project_id,workspace_id,task_id,session_id,client_name,client_title,client_version,
			 started_at,last_heartbeat_at,last_activity_at,sequence,ended_at,end_reason)
			SELECT ?,?,?,?,?,?,NULL,NULL,?,?,?,1,?,? FROM projects p
			JOIN workspaces w ON w.workspace_id=? AND w.project_id=p.project_id
			JOIN sessions s ON s.session_id=? AND s.workspace_id=w.workspace_id
			JOIN tasks t ON t.task_id=? AND t.project_id=p.project_id WHERE p.project_id=?`, runtimeID,
			attr.ProjectID, attr.WorkspaceID, attr.TaskID, attr.SessionID, host, child.started, child.observed,
			child.activity, endedAt, endReason, attr.WorkspaceID, attr.SessionID, attr.TaskID, attr.ProjectID)
		if err != nil {
			return err
		}
		if n, _ := result.RowsAffected(); n != 1 {
			return errors.New("pending host child attribution does not agree")
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO host_runtime_bindings
			(runtime_id,host,host_session_hash,host_agent_hash,is_main,parent_runtime_id,generation_hash,agent_type,
			 lifecycle_state,last_event,producer_sequence,observed_at) VALUES (?,?,?,?,0,?,?,NULLIF(?,''),?,?,?,?)`,
			runtimeID, host, sessionHash, child.agentHash, parentRuntimeID, child.generationHash, child.agentType,
			child.state, child.lastEvent, child.sequence, child.observed); err != nil {
			return err
		}
		if child.model != "" || child.effort != "" || child.used.Valid {
			if _, err := tx.ExecContext(ctx, `INSERT INTO agent_runtime_observations
				(runtime_id,model_key,effort,context_used,context_limit,source,confidence,observed_at,revision,producer_sequence)
				VALUES (?,NULLIF(?,''),NULLIF(?,''),?,?,'host_adapter','self_reported',?,1,?)`, runtimeID,
				child.model, child.effort, nullableNullInt64(child.used), nullableNullInt64(child.limit), child.observed,
				child.sequence); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *HostRuntimeStore) cleanupPending(ctx context.Context, now time.Time) error {
	cutoff := app.FormatTime(now.Add(-pendingHostTTL))
	if _, err := s.db.ExecContext(ctx, `DELETE FROM pending_host_runtime_events WHERE observed_at<?`, cutoff); err != nil {
		return fmt.Errorf("expire pending host events: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM pending_host_runtime_events
		WHERE (host,host_session_hash,host_agent_hash,generation_hash) IN (
			SELECT host,host_session_hash,host_agent_hash,generation_hash FROM pending_host_runtime_events
			ORDER BY observed_at DESC LIMIT -1 OFFSET ?)`, pendingHostCap); err != nil {
		return fmt.Errorf("cap pending host events: %w", err)
	}
	return nil
}

// ObservePending records a command-hook event before an MCP connection has
// supplied task attribution. Only identity digests and bounded telemetry are
// retained. Once claimed, later command-hook events advance the bound runtime.
func (s *HostRuntimeStore) ObservePending(ctx context.Context, in HostRuntimeEventInput) (HostRuntime, error) {
	if err := s.validate(ctx, in); err != nil {
		return HostRuntime{}, err
	}
	sessionHash, agentHash, generationHash := hostIdentityHashes(in)
	var attr HostRuntimeAttribution
	err := s.db.QueryRowContext(ctx, `SELECT r.project_id,r.workspace_id,r.task_id,r.session_id
		FROM host_runtime_bindings h JOIN agent_runtimes r ON r.runtime_id=h.runtime_id
		WHERE h.host=? AND h.host_session_hash=? AND h.host_agent_hash=? AND h.generation_hash=?`,
		in.Host, sessionHash[:], agentHash[:], generationHash[:]).Scan(&attr.ProjectID, &attr.WorkspaceID, &attr.TaskID, &attr.SessionID)
	if err == nil {
		return s.Observe(ctx, attr, in)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return HostRuntime{}, fmt.Errorf("find pending host binding: %w", err)
	}
	var pendingCount int
	if err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM pending_host_runtime_events
		WHERE host=? AND host_session_hash=? AND host_agent_hash=? AND generation_hash=?`,
		in.Host, sessionHash[:], agentHash[:], generationHash[:]).Scan(&pendingCount); err != nil {
		return HostRuntime{}, fmt.Errorf("inspect pending host generation: %w", err)
	}
	if pendingCount == 0 && in.Event != HostEventStart {
		return HostRuntime{}, ErrPendingHostGeneration
	}
	state, endedAt := "ACTIVE", any(nil)
	if in.Event == HostEventStop {
		state, endedAt = "ENDED", app.FormatTime(in.ObservedAt)
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO pending_host_runtime_events
		(host,host_session_hash,host_agent_hash,generation_hash,agent_type,lifecycle_state,last_event,producer_sequence,
		 model_key,effort,context_used,context_limit,started_at,last_activity_at,observed_at,ended_at)
		VALUES (?,?,?,?,NULLIF(?,''),?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(host,host_session_hash,host_agent_hash,generation_hash) DO UPDATE SET
		agent_type=COALESCE(excluded.agent_type,pending_host_runtime_events.agent_type),
		lifecycle_state=excluded.lifecycle_state,last_event=excluded.last_event,producer_sequence=excluded.producer_sequence,
		model_key=COALESCE(excluded.model_key,pending_host_runtime_events.model_key),
		effort=COALESCE(excluded.effort,pending_host_runtime_events.effort),
		context_used=COALESCE(excluded.context_used,pending_host_runtime_events.context_used),
		context_limit=COALESCE(excluded.context_limit,pending_host_runtime_events.context_limit),
		last_activity_at=excluded.last_activity_at,observed_at=excluded.observed_at,ended_at=excluded.ended_at
		WHERE pending_host_runtime_events.lifecycle_state='ACTIVE' AND excluded.producer_sequence>pending_host_runtime_events.producer_sequence`,
		in.Host, sessionHash[:], agentHash[:], generationHash[:], in.AgentType, state, in.Event, in.Sequence,
		nullableString(canonicalModelKey(in.ModelKey)), nullableString(canonicalEffort(in.Effort)), nullableInt64Pointer(in.ContextUsed),
		nullableInt64Pointer(in.ContextLimit), app.FormatTime(in.ObservedAt), app.FormatTime(in.ObservedAt), app.FormatTime(in.ObservedAt), endedAt)
	if err != nil {
		return HostRuntime{}, fmt.Errorf("record pending host event: %w", err)
	}
	return HostRuntime{Host: in.Host, IsMain: in.AgentID == "", AgentType: in.AgentType, LifecycleState: state,
		LastEvent: in.Event, ProducerSequence: in.Sequence, ObservedAt: in.ObservedAt.UTC(), Accepted: true}, nil
}

// BindPending claims one exact digested host generation for an attributed MCP
// run. It cannot select an arbitrary active task or session.
func (s *HostRuntimeStore) BindPending(ctx context.Context, attr HostRuntimeAttribution, id HostRuntimeIdentity) (HostRuntime, error) {
	probe := HostRuntimeEventInput{Host: id.Host, HostSessionID: id.HostSessionID, AgentID: id.AgentID, Generation: id.Generation}
	if !hostOpaqueID.MatchString(id.HostSessionID) || !hostOpaqueID.MatchString(id.Generation) ||
		(id.AgentID != "" && !hostOpaqueID.MatchString(id.AgentID)) || (id.Host != HostClaudeCode && id.Host != HostCodex) {
		return HostRuntime{}, errors.New("pending host identity is invalid")
	}
	sessionHash, agentHash, generationHash := hostIdentityHashes(probe)
	var agentType, state, lastEvent, model, effort string
	var sequence int64
	var used, limit sql.NullInt64
	var started, observed string
	err := s.db.QueryRowContext(ctx, `SELECT COALESCE(agent_type,''),lifecycle_state,last_event,producer_sequence,
		COALESCE(model_key,''),COALESCE(effort,''),context_used,context_limit,started_at,observed_at
		FROM pending_host_runtime_events WHERE host=? AND host_session_hash=? AND host_agent_hash=? AND generation_hash=?`,
		id.Host, sessionHash[:], agentHash[:], generationHash[:]).Scan(&agentType, &state, &lastEvent, &sequence, &model, &effort, &used, &limit, &started, &observed)
	if err != nil {
		return HostRuntime{}, fmt.Errorf("read pending host generation: %w", err)
	}
	_ = lastEvent
	observedAt, err := app.ParseTime(observed)
	if err != nil {
		return HostRuntime{}, err
	}
	startAt, err := app.ParseTime(started)
	if err != nil {
		return HostRuntime{}, err
	}
	// Claiming is itself current even if the command hook was queued briefly.
	now := s.clock.Now().UTC()
	if now.Sub(observedAt) > hostEventMaxAge {
		return HostRuntime{}, errors.New("pending host generation is stale")
	}
	start := HostRuntimeEventInput{Host: id.Host, HostSessionID: id.HostSessionID, AgentID: id.AgentID,
		Generation: id.Generation, AgentType: agentType, Event: HostEventStart, Sequence: sequence, ObservedAt: startAt,
		ModelKey: model, Effort: effort}
	if used.Valid {
		start.ContextUsed, start.ContextLimit = &used.Int64, &limit.Int64
	}
	// Validate freshness against claim time while preserving the actual start in
	// the pending table; bound presence begins at the safe claim boundary.
	start.ObservedAt = now
	out, err := s.Observe(ctx, attr, start)
	if err != nil {
		return HostRuntime{}, err
	}
	if state == "ENDED" {
		stop := start
		stop.Event, stop.Sequence, stop.ObservedAt = HostEventStop, sequence+1, now
		out, err = s.Observe(ctx, attr, stop)
		if err != nil {
			return HostRuntime{}, err
		}
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM pending_host_runtime_events
		WHERE host=? AND host_session_hash=? AND host_agent_hash=? AND generation_hash=?`,
		id.Host, sessionHash[:], agentHash[:], generationHash[:]); err != nil {
		return HostRuntime{}, fmt.Errorf("delete claimed pending host generation: %w", err)
	}
	return out, nil
}

// Observe applies an event already bound to one exact Mindrail run. It never
// accepts attribution from a host payload; adapters obtain it from their MCP
// connection or another verified binding boundary.
func (s *HostRuntimeStore) Observe(ctx context.Context, attr HostRuntimeAttribution, in HostRuntimeEventInput) (HostRuntime, error) {
	if err := s.validate(ctx, in); err != nil {
		return HostRuntime{}, err
	}
	if attr.ProjectID == "" || attr.WorkspaceID == "" || attr.TaskID == "" || attr.SessionID == "" {
		return HostRuntime{}, errors.New("host runtime event needs explicit Mindrail attribution")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return HostRuntime{}, fmt.Errorf("begin host runtime event: %w", err)
	}
	defer tx.Rollback()
	out, err := s.observeTx(ctx, tx, attr, in)
	if err != nil {
		return HostRuntime{}, err
	}
	if err := tx.Commit(); err != nil {
		return HostRuntime{}, fmt.Errorf("commit host runtime event: %w", err)
	}
	return out, nil
}

func (s *HostRuntimeStore) observeTx(ctx context.Context, tx *sql.Tx, attr HostRuntimeAttribution, in HostRuntimeEventInput) (HostRuntime, error) {
	sessionHash, agentHash, generationHash := hostIdentityHashes(in)
	current, err := getHostRuntime(ctx, tx, in.Host, sessionHash, agentHash, generationHash)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return HostRuntime{}, err
	}
	if err == nil {
		if err := requireRuntimeAttribution(ctx, tx, current.RuntimeID, attr); err != nil {
			return HostRuntime{}, err
		}
		if in.Sequence <= current.ProducerSequence {
			current.Accepted = false
			return current, nil
		}
		if current.LifecycleState == "ENDED" {
			return HostRuntime{}, errors.New("ended host runtime generation cannot be revived")
		}
		return s.advanceTx(ctx, tx, current, in)
	}
	if in.Event != HostEventStart {
		return HostRuntime{}, errors.New("host runtime generation must start before later events")
	}
	if active, activeErr := getLatestActiveHostRuntime(ctx, tx, in.Host, sessionHash, agentHash); activeErr == nil {
		if err := requireRuntimeAttribution(ctx, tx, active.RuntimeID, attr); err != nil {
			return HostRuntime{}, err
		}
	} else if !errors.Is(activeErr, sql.ErrNoRows) {
		return HostRuntime{}, activeErr
	}
	now := app.FormatTime(in.ObservedAt)
	if _, err := tx.ExecContext(ctx, `UPDATE agent_runtimes SET ended_at=?, end_reason='replaced', sequence=sequence+1
		WHERE runtime_id IN (SELECT runtime_id FROM host_runtime_bindings
		 WHERE host=? AND host_session_hash=? AND host_agent_hash=? AND lifecycle_state='ACTIVE')`, now, in.Host, sessionHash[:], agentHash[:]); err != nil {
		return HostRuntime{}, fmt.Errorf("replace prior host generation presence: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE host_runtime_bindings SET lifecycle_state='ENDED', last_event='stop', observed_at=?
		WHERE host=? AND host_session_hash=? AND host_agent_hash=? AND lifecycle_state='ACTIVE'`, now, in.Host, sessionHash[:], agentHash[:]); err != nil {
		return HostRuntime{}, fmt.Errorf("replace prior host generation: %w", err)
	}
	runtimeID := identity.NewID("RUN")
	result, err := tx.ExecContext(ctx, `INSERT INTO agent_runtimes
		(runtime_id,project_id,workspace_id,task_id,session_id,client_name,client_title,client_version,
		 started_at,last_heartbeat_at,last_activity_at,sequence,ended_at,end_reason)
	SELECT ?,?,?,?,? ,?,NULL,NULL, ?,?,?,1,NULL,NULL
	FROM projects p JOIN workspaces w ON w.workspace_id=? AND w.project_id=p.project_id
	JOIN sessions s ON s.session_id=? AND s.workspace_id=w.workspace_id
	JOIN tasks t ON t.task_id=? AND t.project_id=p.project_id WHERE p.project_id=?`,
		runtimeID, attr.ProjectID, attr.WorkspaceID, attr.TaskID, attr.SessionID, in.Host,
		now, now, now, attr.WorkspaceID, attr.SessionID, attr.TaskID, attr.ProjectID)
	if err != nil {
		return HostRuntime{}, fmt.Errorf("create host runtime presence: %w", err)
	}
	if n, _ := result.RowsAffected(); n != 1 {
		return HostRuntime{}, errors.New("host runtime attribution does not agree")
	}
	var parent any
	if in.AgentID != "" {
		var parentID string
		err := tx.QueryRowContext(ctx, `SELECT h.runtime_id FROM host_runtime_bindings h
			JOIN agent_runtimes r ON r.runtime_id=h.runtime_id
			WHERE h.host=? AND h.host_session_hash=? AND h.is_main=1 AND h.lifecycle_state='ACTIVE'
			AND r.project_id=? AND r.workspace_id=? AND r.task_id=? AND r.session_id=?
			ORDER BY h.observed_at DESC LIMIT 1`, in.Host, sessionHash[:], attr.ProjectID, attr.WorkspaceID,
			attr.TaskID, attr.SessionID).Scan(&parentID)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return HostRuntime{}, fmt.Errorf("find host parent: %w", err)
		}
		if err == nil {
			parent = parentID
		}
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO host_runtime_bindings
		(runtime_id,host,host_session_hash,host_agent_hash,is_main,parent_runtime_id,
		 generation_hash,agent_type,lifecycle_state,last_event,producer_sequence,observed_at)
		VALUES (?,?,?,?,?,?,?,NULLIF(?,''),'ACTIVE','start',?,?)`, runtimeID, in.Host,
		sessionHash[:], agentHash[:], boolInt(in.AgentID == ""), parent, generationHash[:], in.AgentType, in.Sequence, now); err != nil {
		return HostRuntime{}, fmt.Errorf("bind host runtime: %w", err)
	}
	if in.AgentID == "" {
		if _, err := tx.ExecContext(ctx, `UPDATE host_runtime_bindings SET parent_runtime_id=?
			WHERE host=? AND host_session_hash=? AND is_main=0 AND parent_runtime_id IS NULL
			AND runtime_id IN (SELECT runtime_id FROM agent_runtimes WHERE project_id=? AND workspace_id=? AND task_id=? AND session_id=?)`,
			runtimeID, in.Host, sessionHash[:], attr.ProjectID, attr.WorkspaceID, attr.TaskID, attr.SessionID); err != nil {
			return HostRuntime{}, fmt.Errorf("attach host children: %w", err)
		}
	}
	out := HostRuntime{RuntimeID: runtimeID, Host: in.Host, IsMain: in.AgentID == "",
		AgentType: in.AgentType, LifecycleState: "ACTIVE", LastEvent: HostEventStart,
		ProducerSequence: in.Sequence, ObservedAt: in.ObservedAt.UTC(), Accepted: true}
	if parent != nil {
		out.ParentRuntimeID = parent.(string)
	}
	if err := writeHostObservation(ctx, tx, runtimeID, in); err != nil {
		return HostRuntime{}, err
	}
	return out, nil
}

func (s *HostRuntimeStore) advanceTx(ctx context.Context, tx *sql.Tx, current HostRuntime, in HostRuntimeEventInput) (HostRuntime, error) {
	state, endReason := "ACTIVE", any(nil)
	endedAt := any(nil)
	if in.Event == HostEventStop {
		state, endReason, endedAt = "ENDED", RuntimeEndCompleted, app.FormatTime(in.ObservedAt)
	}
	activityAt := any(nil)
	if in.Event == HostEventActivity || in.Event == HostEventTelemetry {
		activityAt = app.FormatTime(in.ObservedAt)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE agent_runtimes SET last_heartbeat_at=?,
		last_activity_at=COALESCE(?,last_activity_at), sequence=sequence+1, ended_at=?, end_reason=?
		WHERE runtime_id=? AND ended_at IS NULL`, app.FormatTime(in.ObservedAt), activityAt, endedAt, endReason, current.RuntimeID); err != nil {
		return HostRuntime{}, fmt.Errorf("advance host runtime presence: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE host_runtime_bindings SET lifecycle_state=?,last_event=?,producer_sequence=?,observed_at=?
		WHERE runtime_id=?`, state, in.Event, in.Sequence, app.FormatTime(in.ObservedAt), current.RuntimeID); err != nil {
		return HostRuntime{}, fmt.Errorf("advance host runtime: %w", err)
	}
	if err := writeHostObservation(ctx, tx, current.RuntimeID, in); err != nil {
		return HostRuntime{}, err
	}
	current.LifecycleState, current.LastEvent, current.ProducerSequence = state, in.Event, in.Sequence
	current.ObservedAt, current.Accepted = in.ObservedAt.UTC(), true
	return current, nil
}

func writeHostObservation(ctx context.Context, tx *sql.Tx, runtimeID string, in HostRuntimeEventInput) error {
	if in.ModelKey == "" && in.Effort == "" && in.ContextUsed == nil {
		return nil
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO agent_runtime_observations
		(runtime_id,model_key,effort,context_used,context_limit,source,confidence,observed_at,revision,producer_sequence)
		VALUES (?,NULLIF(?,''),NULLIF(?,''),?,?, 'host_adapter','self_reported',?,1,?)
		ON CONFLICT(runtime_id) DO UPDATE SET model_key=excluded.model_key,effort=excluded.effort,
		context_used=excluded.context_used,context_limit=excluded.context_limit,observed_at=excluded.observed_at,
		revision=agent_runtime_observations.revision+1,producer_sequence=excluded.producer_sequence
		WHERE excluded.producer_sequence>agent_runtime_observations.producer_sequence`, runtimeID, canonicalModelKey(in.ModelKey),
		canonicalEffort(in.Effort), nullableInt64Pointer(in.ContextUsed), nullableInt64Pointer(in.ContextLimit),
		app.FormatTime(in.ObservedAt), in.Sequence)
	if err != nil {
		return fmt.Errorf("record host runtime telemetry: %w", err)
	}
	return nil
}

func (s *HostRuntimeStore) validate(ctx context.Context, in HostRuntimeEventInput) error {
	if err := requireAgentTelemetrySchema(ctx, s.db); err != nil {
		return err
	}
	if in.Host != HostClaudeCode && in.Host != HostCodex {
		return errors.New("host runtime host is invalid")
	}
	if !hostOpaqueID.MatchString(in.HostSessionID) || !hostOpaqueID.MatchString(in.Generation) ||
		(in.AgentID != "" && !hostOpaqueID.MatchString(in.AgentID)) || (in.AgentType != "" && !hostAgentType.MatchString(in.AgentType)) {
		return errors.New("host runtime identifiers must be bounded opaque identifiers")
	}
	if in.Event != HostEventStart && in.Event != HostEventActivity && in.Event != HostEventTelemetry && in.Event != HostEventStop {
		return errors.New("host runtime event is invalid")
	}
	if in.Sequence < 1 || in.ObservedAt.IsZero() {
		return errors.New("host runtime event needs a positive sequence and observed time")
	}
	age := s.clock.Now().UTC().Sub(in.ObservedAt.UTC())
	if age < -hostEventFutureSkew || age > hostEventMaxAge {
		return errors.New("host runtime event timestamp is stale or from the future")
	}
	if in.AgentID != "" && in.ContextUsed != nil {
		return errors.New("subagent context telemetry is unsupported")
	}
	obs := RuntimeObservationInput{ModelKey: in.ModelKey, Effort: in.Effort, ContextUsed: in.ContextUsed,
		ContextLimit: in.ContextLimit, Source: ObservationSourceHost, Confidence: ObservationStructuredHost,
		ProducerSequence: in.Sequence, ObservedAt: in.ObservedAt}
	if err := validateObservation(obs); err != nil {
		return err
	}
	return nil
}

func getHostRuntime(ctx context.Context, q rowQueryer, host string, sessionHash, agentHash, generationHash [sha256.Size]byte) (HostRuntime, error) {
	var out HostRuntime
	var parent, agentType sql.NullString
	var observed string
	var isMain int
	err := q.QueryRowContext(ctx, `SELECT runtime_id,host,is_main,agent_type,
		parent_runtime_id,lifecycle_state,last_event,producer_sequence,observed_at FROM host_runtime_bindings
		WHERE host=? AND host_session_hash=? AND host_agent_hash=? AND generation_hash=?`,
		host, sessionHash[:], agentHash[:], generationHash[:]).Scan(&out.RuntimeID, &out.Host, &isMain,
		&agentType, &parent, &out.LifecycleState, &out.LastEvent, &out.ProducerSequence, &observed)
	if err != nil {
		return HostRuntime{}, err
	}
	out.AgentType, out.ParentRuntimeID = agentType.String, parent.String
	out.IsMain = isMain == 1
	out.ObservedAt, err = app.ParseTime(observed)
	return out, err
}

func getLatestActiveHostRuntime(ctx context.Context, q rowQueryer, host string, sessionHash, agentHash [sha256.Size]byte) (HostRuntime, error) {
	var out HostRuntime
	var parent, agentType sql.NullString
	var observed string
	var isMain int
	err := q.QueryRowContext(ctx, `SELECT runtime_id,host,is_main,agent_type,parent_runtime_id,lifecycle_state,
		last_event,producer_sequence,observed_at FROM host_runtime_bindings
		WHERE host=? AND host_session_hash=? AND host_agent_hash=? AND lifecycle_state='ACTIVE'
		ORDER BY observed_at DESC LIMIT 1`, host, sessionHash[:], agentHash[:]).Scan(&out.RuntimeID, &out.Host, &isMain,
		&agentType, &parent, &out.LifecycleState, &out.LastEvent, &out.ProducerSequence, &observed)
	if err != nil {
		return HostRuntime{}, err
	}
	out.IsMain, out.AgentType, out.ParentRuntimeID = isMain == 1, agentType.String, parent.String
	out.ObservedAt, err = app.ParseTime(observed)
	return out, err
}

type rowQueryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func requireRuntimeAttribution(ctx context.Context, q rowQueryer, runtimeID string, attr HostRuntimeAttribution) error {
	var projectID, workspaceID, taskID, sessionID string
	if err := q.QueryRowContext(ctx, `SELECT project_id,workspace_id,task_id,session_id FROM agent_runtimes WHERE runtime_id=?`,
		runtimeID).Scan(&projectID, &workspaceID, &taskID, &sessionID); err != nil {
		return err
	}
	if projectID != attr.ProjectID || workspaceID != attr.WorkspaceID || taskID != attr.TaskID || sessionID != attr.SessionID {
		return errors.New("host runtime binding attribution does not agree")
	}
	return nil
}

func hostIdentityHashes(in HostRuntimeEventInput) ([sha256.Size]byte, [sha256.Size]byte, [sha256.Size]byte) {
	return sha256.Sum256([]byte(in.HostSessionID)), sha256.Sum256([]byte(in.AgentID)), sha256.Sum256([]byte(in.Generation))
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func nullableNullInt64(value sql.NullInt64) any {
	if !value.Valid {
		return nil
	}
	return value.Int64
}
