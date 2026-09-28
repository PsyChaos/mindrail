package agent

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/identity"
)

const AgentTelemetrySchemaVersion = 14

const (
	RouteProviderTypeSafe = "typesafe"
	RouteModelJEVLatest   = "jev-latest"

	RouteStatusOK       = "ok"
	RouteStatusFallback = "fallback"
	RouteStatusDisabled = "disabled"

	RouteReasonAdviceAvailable = "advice_available"

	RouteCredentialKeyring     = "keyring"
	RouteCredentialEnvironment = "environment"
	RouteCredentialNone        = "none"
)

var routeReasons = map[string]struct{}{
	"adapter_failure": {}, "advice_available": {}, "api_key_missing": {}, "atomic_fallback": {},
	"credential_unavailable": {}, "deadline_unavailable": {}, "duplicate": {}, "input_not_json": {},
	"input_too_large": {}, "invalid_api_key": {}, "invalid_candidates": {},
	"invalid_goal": {}, "invalid_input": {}, "invalid_response": {},
	"low_confidence": {}, "no_candidates": {}, "no_match": {},
	"provider_http_error": {}, "provider_timeout": {}, "provider_unavailable": {}, "python_unavailable": {},
	"rate_limited": {}, "response_too_large": {}, "secret_in_input": {},
	"telemetry_failed": {}, "unknown_input_field": {},
}

// RouteDimension is the safe, ordinal-only summary of one closed candidate set.
// ConfidenceMilli is confidence multiplied by 1000. Candidate identifiers and
// descriptions intentionally have no representation in this API.
type RouteDimension struct {
	CandidateCount  int
	SelectedOrdinal *int
	ConfidenceMilli *int
}

type RouteEventInput struct {
	ProjectID, WorkspaceID, TaskID, SessionID string
	Provider, Model                           string
	Version                                   int
	Status, Reason, CredentialSource          string
	StartedAt, EndedAt                        time.Time
	Tool, Agent, ModelChoice, Effort          RouteDimension
}

type RouteEvent struct {
	ID                                        string
	ProjectID, WorkspaceID, TaskID, SessionID string
	Provider, Model                           string
	Version                                   int
	Status, Reason, CredentialSource          string
	StartedAt, EndedAt                        time.Time
	DurationMS                                int64
	Tool, Agent, ModelChoice, Effort          RouteDimension
}

type RouteStore struct{ db *sql.DB }

func NewRouteStore(db *sql.DB) (*RouteStore, error) {
	if db == nil {
		return nil, errors.New("agent route store: needs a database handle")
	}
	return &RouteStore{db: db}, nil
}

func (s *RouteStore) Record(ctx context.Context, in RouteEventInput) (RouteEvent, error) {
	if err := requireAgentTelemetrySchema(ctx, s.db); err != nil {
		return RouteEvent{}, err
	}
	if err := validateRouteInput(in); err != nil {
		return RouteEvent{}, err
	}
	duration := in.EndedAt.Sub(in.StartedAt)
	event := RouteEvent{
		ID: identity.NewID("JRV"), ProjectID: in.ProjectID, WorkspaceID: in.WorkspaceID,
		TaskID: in.TaskID, SessionID: in.SessionID, Provider: in.Provider, Model: in.Model,
		Version: in.Version, Status: in.Status, Reason: in.Reason,
		CredentialSource: in.CredentialSource, StartedAt: in.StartedAt.UTC(), EndedAt: in.EndedAt.UTC(),
		DurationMS: duration.Milliseconds(), Tool: in.Tool, Agent: in.Agent,
		ModelChoice: in.ModelChoice, Effort: in.Effort,
	}
	result, err := s.db.ExecContext(ctx, `INSERT INTO jev_route_events (
		route_id, project_id, workspace_id, task_id, session_id, provider, model, version,
		status, reason, credential_source, started_at, ended_at, duration_ms,
		tool_candidate_count, tool_selected_ordinal, tool_confidence_milli,
		agent_candidate_count, agent_selected_ordinal, agent_confidence_milli,
		model_candidate_count, model_selected_ordinal, model_confidence_milli,
		effort_candidate_count, effort_selected_ordinal, effort_confidence_milli)
	SELECT ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?
	FROM projects p
	JOIN workspaces w ON w.workspace_id = ? AND w.project_id = p.project_id
	JOIN sessions s ON s.session_id = ? AND s.workspace_id = w.workspace_id
	JOIN tasks t ON t.task_id = ? AND t.project_id = p.project_id
	WHERE p.project_id = ?`,
		event.ID, event.ProjectID, event.WorkspaceID, event.TaskID, event.SessionID,
		event.Provider, event.Model, event.Version, event.Status, event.Reason, event.CredentialSource,
		app.FormatTime(event.StartedAt), app.FormatTime(event.EndedAt), event.DurationMS,
		event.Tool.CandidateCount, event.Tool.SelectedOrdinal, event.Tool.ConfidenceMilli,
		event.Agent.CandidateCount, event.Agent.SelectedOrdinal, event.Agent.ConfidenceMilli,
		event.ModelChoice.CandidateCount, event.ModelChoice.SelectedOrdinal, event.ModelChoice.ConfidenceMilli,
		event.Effort.CandidateCount, event.Effort.SelectedOrdinal, event.Effort.ConfidenceMilli,
		event.WorkspaceID, event.SessionID, event.TaskID, event.ProjectID)
	if err != nil {
		return RouteEvent{}, fmt.Errorf("record JEV route metadata: %w", err)
	}
	written, err := result.RowsAffected()
	if err != nil {
		return RouteEvent{}, fmt.Errorf("record JEV route metadata: %w", err)
	}
	if written != 1 {
		return RouteEvent{}, errors.New("agent route store: project, workspace, task and session attribution do not agree")
	}
	return event, nil
}

func (s *RouteStore) ListProject(ctx context.Context, projectID string, limit int) ([]RouteEvent, error) {
	if err := requireAgentTelemetrySchema(ctx, s.db); err != nil {
		return nil, err
	}
	if projectID == "" || limit < 1 || limit > 200 {
		return nil, errors.New("agent route store: listing needs a project and a limit from 1 to 200")
	}
	rows, err := s.db.QueryContext(ctx, routeColumns+` WHERE project_id = ? ORDER BY route_id DESC LIMIT ?`, projectID, limit)
	if err != nil {
		return nil, fmt.Errorf("list JEV route metadata: %w", err)
	}
	defer rows.Close()
	events := make([]RouteEvent, 0)
	for rows.Next() {
		event, err := scanRoute(rows)
		if err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list JEV route metadata: %w", err)
	}
	return events, nil
}

const routeColumns = `SELECT route_id, project_id, workspace_id, task_id, session_id,
	provider, model, version, status, reason, credential_source, started_at, ended_at, duration_ms,
	tool_candidate_count, tool_selected_ordinal, tool_confidence_milli,
	agent_candidate_count, agent_selected_ordinal, agent_confidence_milli,
	model_candidate_count, model_selected_ordinal, model_confidence_milli,
	effort_candidate_count, effort_selected_ordinal, effort_confidence_milli FROM jev_route_events`

type rowScanner interface{ Scan(...any) error }

func scanRoute(row rowScanner) (RouteEvent, error) {
	var event RouteEvent
	var started, ended string
	var toolOrdinal, toolConfidence, agentOrdinal, agentConfidence sql.NullInt64
	var modelOrdinal, modelConfidence, effortOrdinal, effortConfidence sql.NullInt64
	err := row.Scan(&event.ID, &event.ProjectID, &event.WorkspaceID, &event.TaskID, &event.SessionID,
		&event.Provider, &event.Model, &event.Version, &event.Status, &event.Reason, &event.CredentialSource,
		&started, &ended, &event.DurationMS,
		&event.Tool.CandidateCount, &toolOrdinal, &toolConfidence,
		&event.Agent.CandidateCount, &agentOrdinal, &agentConfidence,
		&event.ModelChoice.CandidateCount, &modelOrdinal, &modelConfidence,
		&event.Effort.CandidateCount, &effortOrdinal, &effortConfidence)
	if err != nil {
		return RouteEvent{}, fmt.Errorf("read JEV route metadata: %w", err)
	}
	if event.StartedAt, err = app.ParseTime(started); err != nil {
		return RouteEvent{}, err
	}
	if event.EndedAt, err = app.ParseTime(ended); err != nil {
		return RouteEvent{}, err
	}
	event.Tool.SelectedOrdinal, event.Tool.ConfidenceMilli = nullableInt(toolOrdinal), nullableInt(toolConfidence)
	event.Agent.SelectedOrdinal, event.Agent.ConfidenceMilli = nullableInt(agentOrdinal), nullableInt(agentConfidence)
	event.ModelChoice.SelectedOrdinal, event.ModelChoice.ConfidenceMilli = nullableInt(modelOrdinal), nullableInt(modelConfidence)
	event.Effort.SelectedOrdinal, event.Effort.ConfidenceMilli = nullableInt(effortOrdinal), nullableInt(effortConfidence)
	return event, nil
}

func nullableInt(value sql.NullInt64) *int {
	if !value.Valid {
		return nil
	}
	converted := int(value.Int64)
	return &converted
}

func validateRouteInput(in RouteEventInput) error {
	if in.ProjectID == "" || in.WorkspaceID == "" || in.TaskID == "" || in.SessionID == "" {
		return errors.New("agent route store: route attribution is required")
	}
	if in.Provider != RouteProviderTypeSafe || in.Model != RouteModelJEVLatest || in.Version != 1 {
		return errors.New("agent route store: unsupported provider, model or version")
	}
	if in.Status != RouteStatusOK && in.Status != RouteStatusFallback && in.Status != RouteStatusDisabled {
		return errors.New("agent route store: unsupported status")
	}
	if _, ok := routeReasons[in.Reason]; !ok {
		return errors.New("agent route store: unsupported reason")
	}
	if in.CredentialSource != RouteCredentialKeyring && in.CredentialSource != RouteCredentialEnvironment && in.CredentialSource != RouteCredentialNone {
		return errors.New("agent route store: unsupported credential source")
	}
	if in.StartedAt.IsZero() || in.EndedAt.IsZero() || in.EndedAt.Before(in.StartedAt) || in.EndedAt.Sub(in.StartedAt) > time.Minute {
		return errors.New("agent route store: invalid bounded route duration")
	}
	for _, dimension := range []RouteDimension{in.Tool, in.Agent, in.ModelChoice, in.Effort} {
		if err := validateRouteDimension(dimension); err != nil {
			return err
		}
	}
	return nil
}

func validateRouteDimension(d RouteDimension) error {
	if d.CandidateCount < 0 || d.CandidateCount > 32 {
		return errors.New("agent route store: candidate count must be from 0 to 32")
	}
	if d.SelectedOrdinal != nil && d.ConfidenceMilli == nil {
		return errors.New("agent route store: a selection ordinal needs confidence")
	}
	if d.SelectedOrdinal != nil && (*d.SelectedOrdinal < 0 || *d.SelectedOrdinal >= d.CandidateCount) {
		return errors.New("agent route store: selection metadata is outside the candidate set")
	}
	if d.ConfidenceMilli != nil && (*d.ConfidenceMilli < 0 || *d.ConfidenceMilli > 1000) {
		return errors.New("agent route store: confidence must be from 0 to 1000")
	}
	return nil
}

func requireAgentTelemetrySchema(ctx context.Context, db *sql.DB) error {
	var version sql.NullInt64
	if err := db.QueryRowContext(ctx, `SELECT max(version) FROM schema_migrations`).Scan(&version); err != nil {
		return fmt.Errorf("agent telemetry schema: %w", err)
	}
	if version.Int64 < AgentTelemetrySchemaVersion {
		return fmt.Errorf("agent telemetry schema is behind: got %d, need %d", version.Int64, AgentTelemetrySchemaVersion)
	}
	return nil
}
