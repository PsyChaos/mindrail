package dashboard

import (
	"context"
	"database/sql"
	"errors"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"github.com/PsyChaos/mindrail/internal/agent"
	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/config"
	"github.com/PsyChaos/mindrail/internal/status"
	"github.com/PsyChaos/mindrail/internal/workspace"
)

type CollectorOptions struct {
	DB           *sql.DB
	ProjectID    string
	Workspace    workspace.Workspace
	ProjectName  string
	WorktreeRoot string
	Profiles     map[string]config.ValidationProfile
	Readiness    status.Report
	JEV          JEVState
	StartedAt    time.Time
	Redact       func(string) string
	Now          func() time.Time
}

type Collector struct {
	opts CollectorOptions
	seq  atomic.Uint64
}

type queryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func NewCollector(opts CollectorOptions) (*Collector, error) {
	if opts.DB == nil || opts.ProjectID == "" || opts.Workspace.ID == "" || opts.WorktreeRoot == "" {
		return nil, errors.New("dashboard: database, project, workspace and worktree root are required")
	}
	if opts.Now == nil {
		opts.Now = func() time.Time { return time.Now().UTC() }
	}
	if opts.StartedAt.IsZero() {
		opts.StartedAt = opts.Now().UTC()
	} else {
		opts.StartedAt = opts.StartedAt.UTC()
	}
	opts.JEV = normalizeJEVState(opts.JEV)
	if opts.Redact == nil {
		opts.Redact = func(value string) string { return value }
	}
	return &Collector{opts: opts}, nil
}

func (c *Collector) Snapshot(ctx context.Context) (Snapshot, error) {
	now := c.opts.Now().UTC()
	tx, err := c.opts.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return Snapshot{}, err
	}
	defer tx.Rollback()
	tasks, taskCut, err := c.tasks(ctx, tx)
	if err != nil {
		return Snapshot{}, err
	}
	leases, leaseCut, err := c.leases(ctx, tx, now)
	if err != nil {
		return Snapshot{}, err
	}
	sessions, sessionCut, err := c.sessions(ctx, tx)
	if err != nil {
		return Snapshot{}, err
	}
	checkpoints, checkpointCut, err := c.checkpoints(ctx, tx)
	if err != nil {
		return Snapshot{}, err
	}
	evidence, evidenceCut, err := c.evidence(ctx, tx)
	if err != nil {
		return Snapshot{}, err
	}
	routes, routeCount, acceptedRouteCount, latestRouteAt, routeCut, err := c.jevRouteEvents(ctx, tx)
	if err != nil {
		return Snapshot{}, err
	}
	runtimes, runtimeCut, err := c.agentRuntimes(ctx, tx, now)
	if err != nil {
		return Snapshot{}, err
	}
	summary, engaged, taskCounts, leaseCounts, err := c.summary(ctx, tx, now)
	if err != nil {
		return Snapshot{}, err
	}
	activity, err := c.sessionActivity(ctx, tx, sessions, now)
	if err != nil {
		return Snapshot{}, err
	}
	for i := range sessions {
		sessions[i].Engaged = engaged[sessions[i].ID]
		sessions[i].TaskCount = taskCounts[sessions[i].ID]
		sessions[i].LeaseCount = leaseCounts[sessions[i].ID]
		applySessionActivity(&sessions[i], activity[sessions[i].ID])
	}

	latest, latestCI, err := c.latestEvidence(ctx, tx)
	if err != nil {
		return Snapshot{}, err
	}
	profiles := c.profiles(latest)
	ci := ProviderState{Status: "unavailable", Detail: "No persisted CI verification evidence is available.", Source: "evidence"}
	if latestCI != nil {
		ci = ProviderState{Status: "persisted_" + latestCI.Status + "_unverified", Detail: "Latest persisted CI verification record; snapshot freshness is not evaluated by the dashboard.", Source: latestCI.Profile}
	}
	if err := tx.Commit(); err != nil {
		return Snapshot{}, err
	}
	truncated := map[string]bool{}
	if taskCut {
		truncated["tasks"] = true
	}
	if leaseCut {
		truncated["leases"] = true
	}
	if sessionCut {
		truncated["sessions"] = true
	}
	if checkpointCut {
		truncated["checkpoints"] = true
	}
	if evidenceCut {
		truncated["evidence"] = true
	}
	if routeCut {
		truncated["jev_route_events"] = true
	}
	if runtimeCut {
		truncated["agent_runtimes"] = true
	}
	if len(profiles) < len(c.opts.Profiles) {
		truncated["profiles"] = true
	}

	jev := c.opts.JEV
	jev.Used = acceptedRouteCount > 0
	jev.RouteCount = routeCount
	jev.AcceptedRouteCount = acceptedRouteCount
	jev.LatestRouteAt = latestRouteAt
	return Snapshot{
		Sequence: c.seq.Add(1), GeneratedAt: now,
		Dashboard: DashboardState{StartedAt: c.opts.StartedAt, UptimeSeconds: nonNegativeSeconds(now.Sub(c.opts.StartedAt))},
		Project: Project{ID: boundedText(c.opts.ProjectID, maxIDRunes), Name: boundedText(c.opts.Redact(c.opts.ProjectName), maxTextRunes), WorkspaceID: boundedText(c.opts.Workspace.ID, maxIDRunes),
			LinkedWorktree: c.opts.Workspace.IsLinkedWorktree},
		Summary: summary, Tasks: tasks, Sessions: sessions, Leases: leases,
		Checkpoints: checkpoints, Evidence: evidence, Profiles: profiles, Readiness: readinessView(c.opts.Readiness, c.opts.Redact),
		CI:             ci,
		Merge:          ProviderState{Status: "unavailable", Detail: "No GitHub or merge provider is configured for this local read-only view."},
		JEV:            jev,
		JEVRouteEvents: routes,
		AgentRuntimes:  runtimes,
		Completion:     CompletionState{Status: "on_demand", Detail: "Completion and testguard findings are evaluated during completion and are not persisted by this schema.", Findings: 0},
		Capabilities: Capabilities{ReadOnly: true, LiveTransport: "sse", TaskRevisionHistory: "current_revision_only",
			CheckpointNotes: "metadata_only", SensitiveData: "checkpoint text, evidence output, argv, provenance and credentials omitted",
			ReadinessFreshness: "startup_snapshot"},
		Truncated: truncated,
	}, nil
}

func (c *Collector) jevRouteEvents(ctx context.Context, db queryer) ([]JEVRouteEvent, int, int, *time.Time, bool, error) {
	rows, err := db.QueryContext(ctx, `SELECT substr(route_id,1,?), substr(workspace_id,1,?), substr(task_id,1,?), substr(session_id,1,?),
		provider, model, version, status, reason, credential_source, started_at, ended_at, duration_ms,
		tool_candidate_count, tool_selected_ordinal, tool_confidence_milli,
		agent_candidate_count, agent_selected_ordinal, agent_confidence_milli,
		model_candidate_count, model_selected_ordinal, model_confidence_milli,
		effort_candidate_count, effort_selected_ordinal, effort_confidence_milli
		FROM jev_route_events WHERE project_id = ? ORDER BY route_id DESC LIMIT ?`,
		maxIDRunes+1, maxIDRunes+1, maxIDRunes+1, maxIDRunes+1, c.opts.ProjectID, maxJEVRoutes+1)
	if err != nil {
		return nil, 0, 0, nil, false, err
	}
	defer rows.Close()
	out := make([]JEVRouteEvent, 0)
	for rows.Next() {
		var item JEVRouteEvent
		var started, ended string
		var toolOrdinal, toolConfidence, agentOrdinal, agentConfidence sql.NullInt64
		var modelOrdinal, modelConfidence, effortOrdinal, effortConfidence sql.NullInt64
		if err := rows.Scan(&item.ID, &item.WorkspaceID, &item.TaskID, &item.SessionID,
			&item.Provider, &item.Model, &item.Version, &item.Status, &item.Reason, &item.CredentialSource,
			&started, &ended, &item.DurationMS,
			&item.Tools.CandidateCount, &toolOrdinal, &toolConfidence,
			&item.Agents.CandidateCount, &agentOrdinal, &agentConfidence,
			&item.Models.CandidateCount, &modelOrdinal, &modelConfidence,
			&item.Efforts.CandidateCount, &effortOrdinal, &effortConfidence); err != nil {
			return nil, 0, 0, nil, false, err
		}
		if item.StartedAt, err = app.ParseTime(started); err != nil {
			return nil, 0, 0, nil, false, err
		}
		if item.EndedAt, err = app.ParseTime(ended); err != nil {
			return nil, 0, 0, nil, false, err
		}
		item.ID = boundedText(item.ID, maxIDRunes)
		item.WorkspaceID = boundedText(item.WorkspaceID, maxIDRunes)
		item.TaskID = boundedText(item.TaskID, maxIDRunes)
		item.SessionID = boundedText(item.SessionID, maxIDRunes)
		item.Tools.SelectedOrdinal, item.Tools.ConfidenceMilli = optionalInt(toolOrdinal), optionalInt(toolConfidence)
		item.Agents.SelectedOrdinal, item.Agents.ConfidenceMilli = optionalInt(agentOrdinal), optionalInt(agentConfidence)
		item.Models.SelectedOrdinal, item.Models.ConfidenceMilli = optionalInt(modelOrdinal), optionalInt(modelConfidence)
		item.Efforts.SelectedOrdinal, item.Efforts.ConfidenceMilli = optionalInt(effortOrdinal), optionalInt(effortConfidence)
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, 0, nil, false, err
	}
	var count, accepted int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*), COALESCE(SUM(status = 'ok'),0)
		FROM jev_route_events WHERE project_id = ?`, c.opts.ProjectID).Scan(&count, &accepted); err != nil {
		return nil, 0, 0, nil, false, err
	}
	var latest *time.Time
	if accepted > 0 {
		var latestRaw string
		if err := db.QueryRowContext(ctx, `SELECT ended_at FROM jev_route_events
			WHERE project_id = ? AND status = 'ok' ORDER BY route_id DESC LIMIT 1`, c.opts.ProjectID).Scan(&latestRaw); err != nil {
			return nil, 0, 0, nil, false, err
		}
		value, parseErr := app.ParseTime(latestRaw)
		if parseErr != nil {
			return nil, 0, 0, nil, false, parseErr
		}
		latest = &value
	}
	return trim(out, maxJEVRoutes), count, accepted, latest, len(out) > maxJEVRoutes, nil
}

func (c *Collector) agentRuntimes(ctx context.Context, db queryer, now time.Time) ([]AgentRuntime, bool, error) {
	rows, err := db.QueryContext(ctx, `SELECT substr(r.runtime_id,1,?), substr(r.workspace_id,1,?), substr(r.task_id,1,?), substr(r.session_id,1,?),
		r.client_name, COALESCE(r.client_title,''), COALESCE(r.client_version,''),
		r.started_at, r.last_heartbeat_at, r.last_activity_at, r.sequence, r.ended_at, COALESCE(r.end_reason,''),
		o.model_key, o.effort, o.context_used, o.context_limit, o.source, o.confidence, o.observed_at, o.revision,
		i.intent_id, i.state, i.failure_code, i.checkpoint_id
		FROM agent_runtimes r
		LEFT JOIN agent_runtime_observations o ON o.runtime_id = r.runtime_id
		LEFT JOIN continuity_intents i ON i.intent_id = (
			SELECT ci.intent_id FROM continuity_intents ci
			WHERE ci.task_id = r.task_id AND ci.predecessor_session_id = r.session_id
			ORDER BY ci.intent_id DESC LIMIT 1
		)
		WHERE r.project_id = ? ORDER BY (r.ended_at IS NULL) DESC, r.runtime_id DESC LIMIT ?`,
		maxIDRunes+1, maxIDRunes+1, maxIDRunes+1, maxIDRunes+1,
		c.opts.ProjectID, maxAgentRuntimes+1)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	out := make([]AgentRuntime, 0)
	for rows.Next() {
		var item AgentRuntime
		var started, heartbeat, activity string
		var ended sql.NullString
		var rawName, rawTitle, rawVersion string
		var model, effort, source, confidence, observed sql.NullString
		var contextUsed, contextLimit, observationRevision sql.NullInt64
		var intentID, continuityState, failureCode, checkpointID sql.NullString
		if err := rows.Scan(&item.ID, &item.WorkspaceID, &item.TaskID, &item.SessionID, &rawName, &rawTitle, &rawVersion,
			&started, &heartbeat, &activity, &item.Sequence, &ended, &item.EndReason,
			&model, &effort, &contextUsed, &contextLimit, &source, &confidence, &observed, &observationRevision,
			&intentID, &continuityState, &failureCode, &checkpointID); err != nil {
			return nil, false, err
		}
		if item.StartedAt, err = app.ParseTime(started); err != nil {
			return nil, false, err
		}
		if item.LastHeartbeatAt, err = app.ParseTime(heartbeat); err != nil {
			return nil, false, err
		}
		if item.LastActivityAt, err = app.ParseTime(activity); err != nil {
			return nil, false, err
		}
		if ended.Valid {
			value, parseErr := app.ParseTime(ended.String)
			if parseErr != nil {
				return nil, false, parseErr
			}
			item.EndedAt = &value
		}
		item.ID, item.WorkspaceID = boundedText(item.ID, maxIDRunes), boundedText(item.WorkspaceID, maxIDRunes)
		item.TaskID, item.SessionID = boundedText(item.TaskID, maxIDRunes), boundedText(item.SessionID, maxIDRunes)
		item.ClientFamily = agent.CanonicalClientInfo(agent.ClientInfo{Name: rawName, Title: rawTitle, Version: rawVersion}).Name
		item.Status = runtimeStatus(item, now)
		item.Telemetry = runtimeTelemetry(model, effort, contextUsed, contextLimit, source, confidence, observed, observationRevision, item.LastActivityAt, now)
		item.Continuity = RuntimeContinuity{IntentID: boundedText(intentID.String, maxIDRunes), State: continuityState.String,
			FailureCode: boundedText(failureCode.String, 64), CheckpointID: boundedText(checkpointID.String, maxIDRunes)}
		if item.Continuity.State == "" {
			item.Continuity.State = "NONE"
		}
		out = append(out, item)
	}
	return trim(out, maxAgentRuntimes), len(out) > maxAgentRuntimes, rows.Err()
}

func runtimeTelemetry(model, effort sql.NullString, used, limit sql.NullInt64, source, confidence, observed sql.NullString,
	revision sql.NullInt64, lastActivity, now time.Time) RuntimeTelemetry {
	out := RuntimeTelemetry{State: "NOT_REPORTED"}
	if !observed.Valid {
		return out
	}
	value, err := app.ParseTime(observed.String)
	if err != nil {
		return out
	}
	out.State, out.Model, out.Effort = "REPORTED", model.String, effort.String
	out.Source, out.Confidence, out.ObservedAt, out.Revision = source.String, confidence.String, &value, revision.Int64
	if used.Valid && limit.Valid && limit.Int64 > 0 {
		u, l := used.Int64, limit.Int64
		percent := float64(u) * 100 / float64(l)
		out.ContextUsed, out.ContextLimit, out.UsedPercent = &u, &l, &percent
	}
	if now.Sub(value) > 30*time.Second || lastActivity.After(value) {
		out.State = "STALE"
	}
	return out
}

func runtimeStatus(runtime AgentRuntime, now time.Time) string {
	if runtime.EndedAt != nil {
		return "ENDED"
	}
	heartbeatAge := now.Sub(runtime.LastHeartbeatAt)
	if heartbeatAge > 15*time.Second {
		return "STALE"
	}
	if now.Sub(runtime.LastActivityAt) <= 30*time.Second {
		return "CONNECTED"
	}
	return "IDLE"
}

func optionalInt(value sql.NullInt64) *int {
	if !value.Valid {
		return nil
	}
	result := int(value.Int64)
	return &result
}

type sessionActivity struct {
	currentTaskCount      int
	currentTasks          []SessionTask
	currentTasksTruncated bool
	activeLeaseCount      int
	latestActivityAt      *time.Time
	latestLeaseRenewedAt  *time.Time
	nextLeaseExpiresAt    *time.Time
}

func applySessionActivity(session *Session, activity sessionActivity) {
	session.CurrentTaskCount = activity.currentTaskCount
	session.CurrentTasks = activity.currentTasks
	if session.CurrentTasks == nil {
		session.CurrentTasks = []SessionTask{}
	}
	session.CurrentTasksTruncated = activity.currentTasksTruncated
	session.LatestActivityAt = activity.latestActivityAt
	session.LatestLeaseRenewedAt = activity.latestLeaseRenewedAt
	session.NextLeaseExpiresAt = activity.nextLeaseExpiresAt
	switch {
	case activity.currentTaskCount > 0 && activity.activeLeaseCount > 0:
		session.ActivityStatus = "active_signal"
	case activity.currentTaskCount > 0:
		session.ActivityStatus = "claim_only"
	case activity.activeLeaseCount > 0:
		session.ActivityStatus = "lease_only"
	default:
		session.ActivityStatus = "history"
	}
}

func nonNegativeSeconds(duration time.Duration) int64 {
	if duration <= 0 {
		return 0
	}
	return int64(duration / time.Second)
}

func normalizeJEVState(state JEVState) JEVState {
	if state.Configured && (state.Source == "environment" || state.Source == "keyring") {
		return JEVState{Configured: true, Source: state.Source, Provider: "typesafe", Mode: "optional_advisory"}
	}
	if state.Source == "unavailable" {
		return JEVState{Configured: false, Source: "unavailable", Provider: "typesafe", Mode: "normal_routing"}
	}
	return JEVState{Configured: false, Source: "none", Provider: "typesafe", Mode: "normal_routing"}
}

func readinessView(report status.Report, redact func(string) string) ReadinessView {
	view := ReadinessView{Readiness: report.Readiness, Components: map[string]ReadinessComponent{}}
	for name, component := range report.Components {
		view.Components[string(name)] = ReadinessComponent{State: string(component.State), Summary: boundedText(redact(component.Summary), maxTextRunes),
			Pending: component.Pending, Failed: component.Failed, Units: component.Units}
	}
	return view
}

func (c *Collector) tasks(ctx context.Context, db queryer) ([]Task, bool, error) {
	rows, err := db.QueryContext(ctx, `SELECT substr(task_id,1,?),
		CASE WHEN length(title)>? THEN '[TRUNCATED]' ELSE title END, length(title)>?, state,
		CASE WHEN length(COALESCE(blocked_reason,''))>? THEN '[TRUNCATED]' ELSE COALESCE(blocked_reason,'') END,
		length(COALESCE(blocked_reason,''))>?, substr(opened_by,1,?),
		substr(COALESCE(claimed_by,''),1,?), revision, created_at, updated_at FROM tasks WHERE project_id = ?
		ORDER BY updated_at DESC, task_id DESC LIMIT ?`, maxIDRunes+1, maxTextRunes, maxTextRunes, maxTextRunes, maxTextRunes,
		maxIDRunes+1, maxIDRunes+1, c.opts.ProjectID, maxTasks+1)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	out := make([]Task, 0)
	fieldCut := false
	for rows.Next() {
		var item Task
		var created, updated string
		var titleCut, reasonCut bool
		if err := rows.Scan(&item.ID, &item.Title, &titleCut, &item.State, &item.BlockedReason, &reasonCut, &item.OpenedBy, &item.ClaimedBy, &item.Revision, &created, &updated); err != nil {
			return nil, false, err
		}
		if item.CreatedAt, err = app.ParseTime(created); err != nil {
			return nil, false, err
		}
		if item.UpdatedAt, err = app.ParseTime(updated); err != nil {
			return nil, false, err
		}
		item.ID = boundedText(item.ID, maxIDRunes)
		title, reason := c.opts.Redact(item.Title), c.opts.Redact(item.BlockedReason)
		fieldCut = fieldCut || titleCut || reasonCut || exceedsRunes(title, maxTextRunes) || exceedsRunes(reason, maxTextRunes)
		item.Title = boundedText(title, maxTextRunes)
		item.BlockedReason = boundedText(reason, maxTextRunes)
		item.OpenedBy = boundedText(item.OpenedBy, maxIDRunes)
		item.ClaimedBy = boundedText(item.ClaimedBy, maxIDRunes)
		out = append(out, item)
	}
	return trim(out, maxTasks), len(out) > maxTasks || fieldCut, rows.Err()
}

func (c *Collector) sessions(ctx context.Context, db queryer) ([]Session, bool, error) {
	rows, err := db.QueryContext(ctx, `SELECT substr(session_id,1,?), substr(workspace_id,1,?),
		CASE WHEN length(COALESCE(label,''))>? THEN '[TRUNCATED]' ELSE COALESCE(label,'') END,
		length(COALESCE(label,''))>?, started_at
		FROM sessions WHERE workspace_id IN (SELECT workspace_id FROM workspaces WHERE project_id = ?)
		ORDER BY started_at DESC, session_id DESC LIMIT ?`, maxIDRunes+1, maxIDRunes+1, maxTextRunes, maxTextRunes, c.opts.ProjectID, maxSessions+1)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	out := make([]Session, 0)
	fieldCut := false
	for rows.Next() {
		var item Session
		var started string
		var labelCut bool
		if err := rows.Scan(&item.ID, &item.WorkspaceID, &item.Label, &labelCut, &started); err != nil {
			return nil, false, err
		}
		if item.StartedAt, err = app.ParseTime(started); err != nil {
			return nil, false, err
		}
		item.ID = boundedText(item.ID, maxIDRunes)
		item.WorkspaceID = boundedText(item.WorkspaceID, maxIDRunes)
		label := c.opts.Redact(item.Label)
		fieldCut = fieldCut || labelCut || exceedsRunes(label, maxTextRunes)
		item.Label = boundedText(label, maxTextRunes)
		out = append(out, item)
	}
	return trim(out, maxSessions), len(out) > maxSessions || fieldCut, rows.Err()
}

func (c *Collector) leases(ctx context.Context, db queryer, now time.Time) ([]Lease, bool, error) {
	rows, err := db.QueryContext(ctx, `SELECT substr(lease_id,1,?), target_kind,
		CASE WHEN length(target_key)>? THEN '[TRUNCATED]' ELSE target_key END, length(target_key)>?, substr(holder,1,?), acquired_at, renewed_at,
		expires_at, released_at, CASE WHEN length(COALESCE(release_reason,''))>? THEN '[TRUNCATED]' ELSE COALESCE(release_reason,'') END,
		length(COALESCE(release_reason,''))>? FROM leases WHERE project_id = ?
		ORDER BY (released_at IS NULL) DESC, renewed_at DESC, acquired_at DESC, lease_id DESC LIMIT ?`, maxIDRunes+1, maxTextRunes, maxTextRunes,
		maxIDRunes+1, maxTextRunes, maxTextRunes, c.opts.ProjectID, maxLeases+1)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	out := make([]Lease, 0)
	fieldCut := false
	for rows.Next() {
		var item Lease
		var acquired, renewed, expires string
		var released sql.NullString
		var targetCut, reasonCut bool
		if err := rows.Scan(&item.ID, &item.TargetKind, &item.TargetKey, &targetCut, &item.Holder, &acquired, &renewed, &expires, &released, &item.ReleaseReason, &reasonCut); err != nil {
			return nil, false, err
		}
		if item.AcquiredAt, err = app.ParseTime(acquired); err != nil {
			return nil, false, err
		}
		if item.RenewedAt, err = app.ParseTime(renewed); err != nil {
			return nil, false, err
		}
		if item.ExpiresAt, err = app.ParseTime(expires); err != nil {
			return nil, false, err
		}
		if released.Valid {
			parsed, parseErr := app.ParseTime(released.String)
			if parseErr != nil {
				return nil, false, parseErr
			}
			item.ReleasedAt = &parsed
		}
		switch {
		case item.ReleasedAt != nil:
			item.Status = "released"
		case !now.Before(item.ExpiresAt):
			item.Status = "expired"
		default:
			item.Status = "active"
		}
		item.ID = boundedText(item.ID, maxIDRunes)
		targetKey, releaseReason := c.opts.Redact(item.TargetKey), c.opts.Redact(item.ReleaseReason)
		fieldCut = fieldCut || targetCut || reasonCut || exceedsRunes(targetKey, maxTextRunes) || exceedsRunes(releaseReason, maxTextRunes)
		item.TargetKey = boundedText(targetKey, maxTextRunes)
		item.Holder = boundedText(item.Holder, maxIDRunes)
		item.ReleaseReason = boundedText(releaseReason, maxTextRunes)
		out = append(out, item)
	}
	return trim(out, maxLeases), len(out) > maxLeases || fieldCut, rows.Err()
}

func (c *Collector) checkpoints(ctx context.Context, db queryer) ([]Checkpoint, bool, error) {
	rows, err := db.QueryContext(ctx, `SELECT substr(checkpoint_id,1,?), substr(task_id,1,?), substr(session_id,1,?), substr(workspace_id,1,?), handoff, created_at
		FROM checkpoints WHERE task_id IN (SELECT task_id FROM tasks WHERE project_id = ?)
		ORDER BY rowid DESC LIMIT ?`, maxIDRunes+1, maxIDRunes+1, maxIDRunes+1, maxIDRunes+1, c.opts.ProjectID, maxCheckpoints+1)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	out := make([]Checkpoint, 0)
	for rows.Next() {
		var item Checkpoint
		var created string
		var handoff int
		if err := rows.Scan(&item.ID, &item.TaskID, &item.SessionID, &item.WorkspaceID, &handoff, &created); err != nil {
			return nil, false, err
		}
		item.Handoff = handoff != 0
		if item.CreatedAt, err = app.ParseTime(created); err != nil {
			return nil, false, err
		}
		item.ID = boundedText(item.ID, maxIDRunes)
		item.TaskID = boundedText(item.TaskID, maxIDRunes)
		item.SessionID = boundedText(item.SessionID, maxIDRunes)
		item.WorkspaceID = boundedText(item.WorkspaceID, maxIDRunes)
		out = append(out, item)
	}
	return trim(out, maxCheckpoints), len(out) > maxCheckpoints, rows.Err()
}

func (c *Collector) evidence(ctx context.Context, db queryer) ([]Evidence, bool, error) {
	rows, err := db.QueryContext(ctx, `SELECT substr(evidence_id,1,?), substr(profile,1,?), type, status, exit_code, substr(snapshot_hash,1,13), created_at
		FROM evidence ORDER BY rowid DESC LIMIT ?`, maxIDRunes+1, maxIDRunes+1, maxEvidence+1)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	out := make([]Evidence, 0)
	for rows.Next() {
		var item Evidence
		var created string
		if err := rows.Scan(&item.ID, &item.Profile, &item.Type, &item.Status, &item.ExitCode, &item.SnapshotHash, &created); err != nil {
			return nil, false, err
		}
		item.ID = boundedText(item.ID, maxIDRunes)
		item.Profile = boundedText(item.Profile, maxIDRunes)
		item.SnapshotHash = boundedText(item.SnapshotHash, 12)
		if item.CreatedAt, err = app.ParseTime(created); err != nil {
			return nil, false, err
		}
		out = append(out, item)
	}
	return trim(out, maxEvidence), len(out) > maxEvidence, rows.Err()
}

// summary derives totals independently from the capped detail lists. Detail
// truncation must never turn an operational count into a deceptively green one.
func (c *Collector) summary(ctx context.Context, db queryer, now time.Time) (Summary, map[string]bool, map[string]int, map[string]int, error) {
	result := Summary{States: map[string]int{}}
	engaged := map[string]bool{}
	taskCounts := map[string]int{}
	leaseCounts := map[string]int{}

	rows, err := db.QueryContext(ctx, `SELECT state, COUNT(*) FROM tasks WHERE project_id = ? GROUP BY state`, c.opts.ProjectID)
	if err != nil {
		return result, nil, nil, nil, err
	}
	for rows.Next() {
		var state string
		var count int
		if err := rows.Scan(&state, &count); err != nil {
			rows.Close()
			return result, nil, nil, nil, err
		}
		result.States[state] = count
	}
	if err := rows.Close(); err != nil {
		return result, nil, nil, nil, err
	}

	rows, err = db.QueryContext(ctx, `SELECT session_id, COUNT(*) FROM (
		SELECT opened_by AS session_id, task_id FROM tasks WHERE project_id = ?
		UNION SELECT claimed_by AS session_id, task_id FROM tasks WHERE project_id = ? AND claimed_by IS NOT NULL
	) GROUP BY session_id`, c.opts.ProjectID, c.opts.ProjectID)
	if err != nil {
		return result, nil, nil, nil, err
	}
	for rows.Next() {
		var sessionID string
		var count int
		if err := rows.Scan(&sessionID, &count); err != nil {
			rows.Close()
			return result, nil, nil, nil, err
		}
		taskCounts[sessionID] = count
	}
	if err := rows.Close(); err != nil {
		return result, nil, nil, nil, err
	}

	rows, err = db.QueryContext(ctx, `SELECT claimed_by FROM tasks WHERE project_id = ?
		AND claimed_by IS NOT NULL AND state NOT IN ('COMPLETED','ABANDONED') GROUP BY claimed_by`, c.opts.ProjectID)
	if err != nil {
		return result, nil, nil, nil, err
	}
	for rows.Next() {
		var sessionID string
		if err := rows.Scan(&sessionID); err != nil {
			rows.Close()
			return result, nil, nil, nil, err
		}
		engaged[sessionID] = true
	}
	if err := rows.Close(); err != nil {
		return result, nil, nil, nil, err
	}

	rows, err = db.QueryContext(ctx, `SELECT holder, expires_at FROM leases WHERE project_id = ? AND released_at IS NULL`, c.opts.ProjectID)
	if err != nil {
		return result, nil, nil, nil, err
	}
	for rows.Next() {
		var holder, rawExpires string
		if err := rows.Scan(&holder, &rawExpires); err != nil {
			rows.Close()
			return result, nil, nil, nil, err
		}
		expiresAt, parseErr := app.ParseTime(rawExpires)
		if parseErr != nil {
			rows.Close()
			return result, nil, nil, nil, parseErr
		}
		if now.Before(expiresAt) {
			result.ActiveLeases++
			engaged[holder] = true
			leaseCounts[holder]++
		} else {
			result.ExpiredLeases++
		}
	}
	if err := rows.Close(); err != nil {
		return result, nil, nil, nil, err
	}
	result.AgentsEngaged = len(engaged)

	rows, err = db.QueryContext(ctx, `SELECT status, COUNT(*) FROM evidence GROUP BY status`)
	if err != nil {
		return result, nil, nil, nil, err
	}
	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			rows.Close()
			return result, nil, nil, nil, err
		}
		if status == "pass" {
			result.EvidencePass += count
		} else {
			result.EvidenceFail += count
		}
	}
	if err := rows.Close(); err != nil {
		return result, nil, nil, nil, err
	}
	return result, engaged, taskCounts, leaseCounts, nil
}

// sessionActivity derives the operator-facing coordination signals directly
// from the database rather than the capped top-level detail arrays. This keeps
// counts and status exact even when those arrays are truncated. The task names
// shown on a card are separately bounded and disclose their own truncation.
func (c *Collector) sessionActivity(ctx context.Context, db queryer, sessions []Session, now time.Time) (map[string]sessionActivity, error) {
	result := make(map[string]sessionActivity, len(sessions))
	for _, session := range sessions {
		started := session.StartedAt
		result[session.ID] = sessionActivity{latestActivityAt: &started, currentTasks: []SessionTask{}}
	}
	placeholders, sessionArgs := sessionQueryScope(sessions)
	if len(sessionArgs) == 0 {
		return result, nil
	}

	countArgs := []any{c.opts.ProjectID}
	countArgs = append(countArgs, sessionArgs...)
	rows, err := db.QueryContext(ctx, `SELECT claimed_by, COUNT(*) FROM tasks
		WHERE project_id = ? AND state NOT IN ('COMPLETED','ABANDONED')
		AND claimed_by IN (`+placeholders+`) GROUP BY claimed_by`, countArgs...)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var sessionID string
		var count int
		if err := rows.Scan(&sessionID, &count); err != nil {
			rows.Close()
			return nil, err
		}
		activity := result[sessionID]
		activity.currentTaskCount = count
		activity.currentTasksTruncated = count > maxSessionTasks
		result[sessionID] = activity
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}

	taskArgs := []any{maxTextRunes, c.opts.ProjectID}
	taskArgs = append(taskArgs, sessionArgs...)
	taskArgs = append(taskArgs, maxSessionTasks)
	rows, err = db.QueryContext(ctx, `WITH ranked AS (
		SELECT claimed_by, task_id,
		CASE WHEN length(title)>? THEN '[TRUNCATED]' ELSE title END AS bounded_title,
		state, updated_at,
		ROW_NUMBER() OVER (PARTITION BY claimed_by ORDER BY julianday(updated_at) DESC, task_id DESC) AS rank
		FROM tasks WHERE project_id = ? AND state NOT IN ('COMPLETED','ABANDONED')
		AND claimed_by IN (`+placeholders+`)
	) SELECT claimed_by, task_id, bounded_title, state, updated_at
	FROM ranked WHERE rank <= ? ORDER BY claimed_by, rank`, taskArgs...)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var sessionID, taskID, title, state, rawUpdated string
		if err := rows.Scan(&sessionID, &taskID, &title, &state, &rawUpdated); err != nil {
			rows.Close()
			return nil, err
		}
		activity := result[sessionID]
		updated, parseErr := app.ParseTime(rawUpdated)
		if parseErr != nil {
			rows.Close()
			return nil, parseErr
		}
		redacted := c.opts.Redact(title)
		activity.currentTasks = append(activity.currentTasks, SessionTask{
			ID: boundedText(taskID, maxIDRunes), Title: boundedText(redacted, maxTextRunes),
			State: state, UpdatedAt: updated,
		})
		result[sessionID] = activity
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}

	leaseArgs := []any{c.opts.ProjectID}
	leaseArgs = append(leaseArgs, sessionArgs...)
	rows, err = db.QueryContext(ctx, `SELECT holder, renewed_at, expires_at FROM leases
		WHERE project_id = ? AND released_at IS NULL AND holder IN (`+placeholders+`)`, leaseArgs...)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var sessionID, rawRenewed, rawExpires string
		if err := rows.Scan(&sessionID, &rawRenewed, &rawExpires); err != nil {
			rows.Close()
			return nil, err
		}
		activity, ok := result[sessionID]
		if !ok {
			continue
		}
		renewed, parseErr := app.ParseTime(rawRenewed)
		if parseErr != nil {
			rows.Close()
			return nil, parseErr
		}
		expires, parseErr := app.ParseTime(rawExpires)
		if parseErr != nil {
			rows.Close()
			return nil, parseErr
		}
		if !now.Before(expires) {
			continue
		}
		activity.activeLeaseCount++
		if activity.latestLeaseRenewedAt == nil || renewed.After(*activity.latestLeaseRenewedAt) {
			value := renewed
			activity.latestLeaseRenewedAt = &value
		}
		if activity.nextLeaseExpiresAt == nil || expires.Before(*activity.nextLeaseExpiresAt) {
			value := expires
			activity.nextLeaseExpiresAt = &value
		}
		result[sessionID] = activity
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}

	activityArgs := make([]any, 0, 5*(len(sessionArgs)+1))
	for range 5 {
		activityArgs = append(activityArgs, c.opts.ProjectID)
		activityArgs = append(activityArgs, sessionArgs...)
	}
	rows, err = db.QueryContext(ctx, `WITH activity AS (
		SELECT opened_by AS session_id, created_at AS activity_at FROM tasks WHERE project_id = ? AND opened_by IN (`+placeholders+`)
		UNION ALL SELECT claimed_by, updated_at FROM tasks WHERE project_id = ? AND claimed_by IN (`+placeholders+`)
		UNION ALL SELECT holder, renewed_at FROM leases WHERE project_id = ? AND holder IN (`+placeholders+`)
		UNION ALL SELECT holder, released_at FROM leases WHERE project_id = ? AND released_at IS NOT NULL AND holder IN (`+placeholders+`)
		UNION ALL SELECT checkpoints.session_id, checkpoints.created_at FROM checkpoints
		JOIN tasks ON tasks.task_id = checkpoints.task_id WHERE tasks.project_id = ? AND checkpoints.session_id IN (`+placeholders+`)
	), ranked AS (
		SELECT session_id, activity_at, ROW_NUMBER() OVER (
			PARTITION BY session_id ORDER BY julianday(activity_at) DESC,
			CASE WHEN instr(activity_at,'.') = 0 THEN 0 ELSE CAST(substr(
				substr(activity_at,instr(activity_at,'.')+1,length(activity_at)-instr(activity_at,'.')-1) || '000000000',1,9
			) AS INTEGER) END DESC
		) AS rank FROM activity
	)
	SELECT session_id, activity_at FROM ranked WHERE rank = 1`, activityArgs...)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var sessionID, rawActivity string
		if err := rows.Scan(&sessionID, &rawActivity); err != nil {
			rows.Close()
			return nil, err
		}
		activity, ok := result[sessionID]
		if !ok {
			continue
		}
		at, parseErr := app.ParseTime(rawActivity)
		if parseErr != nil {
			rows.Close()
			return nil, parseErr
		}
		if activity.latestActivityAt == nil || at.After(*activity.latestActivityAt) {
			value := at
			activity.latestActivityAt = &value
		}
		result[sessionID] = activity
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	return result, nil
}

func sessionQueryScope(sessions []Session) (string, []any) {
	if len(sessions) == 0 {
		return "", nil
	}
	args := make([]any, 0, len(sessions))
	for _, session := range sessions {
		args = append(args, session.ID)
	}
	return strings.TrimSuffix(strings.Repeat("?,", len(args)), ","), args
}

func (c *Collector) latestEvidence(ctx context.Context, db queryer) (map[string]Evidence, *Evidence, error) {
	latest := map[string]Evidence{}
	names := make([]string, 0, len(c.opts.Profiles))
	for name := range c.opts.Profiles {
		names = append(names, name)
	}
	sort.Strings(names)
	if len(names) > maxProfiles {
		names = names[:maxProfiles]
	}
	if len(names) > 0 {
		placeholders := strings.TrimSuffix(strings.Repeat("?,", len(names)), ",")
		args := make([]any, 0, len(names)+1)
		args = append(args, maxIDRunes+1)
		for _, name := range names {
			args = append(args, name)
		}
		rows, err := db.QueryContext(ctx, `SELECT substr(evidence_id,1,?), profile, type, status, exit_code,
			substr(snapshot_hash,1,13), created_at FROM evidence
			WHERE rowid IN (SELECT MAX(rowid) FROM evidence WHERE profile IN (`+placeholders+`) GROUP BY profile)`, args...)
		if err != nil {
			return nil, nil, err
		}
		for rows.Next() {
			item, rawProfile, scanErr := scanEvidenceWithRawProfile(rows)
			if scanErr != nil {
				rows.Close()
				return nil, nil, scanErr
			}
			latest[rawProfile] = item
		}
		if err := rows.Close(); err != nil {
			return nil, nil, err
		}
	}

	rows, err := db.QueryContext(ctx, `SELECT substr(evidence_id,1,?), substr(profile,1,?), type, status, exit_code,
		substr(snapshot_hash,1,13), created_at FROM evidence WHERE type = 'CI_VERIFICATION' ORDER BY rowid DESC LIMIT 1`, maxIDRunes+1, maxIDRunes+1)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	if !rows.Next() {
		return latest, nil, rows.Err()
	}
	item, err := scanEvidence(rows)
	if err != nil {
		return nil, nil, err
	}
	return latest, &item, rows.Err()
}

func scanEvidenceWithRawProfile(rows *sql.Rows) (Evidence, string, error) {
	var item Evidence
	var rawProfile, created string
	if err := rows.Scan(&item.ID, &rawProfile, &item.Type, &item.Status, &item.ExitCode, &item.SnapshotHash, &created); err != nil {
		return Evidence{}, "", err
	}
	item.ID = boundedText(item.ID, maxIDRunes)
	item.Profile = boundedText(rawProfile, maxIDRunes)
	item.SnapshotHash = boundedText(item.SnapshotHash, 12)
	parsed, err := app.ParseTime(created)
	if err != nil {
		return Evidence{}, "", err
	}
	item.CreatedAt = parsed
	return item, rawProfile, nil
}

func scanEvidence(rows *sql.Rows) (Evidence, error) {
	var item Evidence
	var created string
	if err := rows.Scan(&item.ID, &item.Profile, &item.Type, &item.Status, &item.ExitCode, &item.SnapshotHash, &created); err != nil {
		return Evidence{}, err
	}
	item.ID = boundedText(item.ID, maxIDRunes)
	item.Profile = boundedText(item.Profile, maxIDRunes)
	item.SnapshotHash = boundedText(item.SnapshotHash, 12)
	parsed, err := app.ParseTime(created)
	if err != nil {
		return Evidence{}, err
	}
	item.CreatedAt = parsed
	return item, nil
}

func (c *Collector) profiles(latest map[string]Evidence) []Profile {
	names := make([]string, 0, len(c.opts.Profiles))
	for name := range c.opts.Profiles {
		names = append(names, name)
	}
	sort.Strings(names)
	out := make([]Profile, 0, len(names))
	for _, name := range names {
		if len(out) == maxProfiles {
			break
		}
		configured := c.opts.Profiles[name]
		item := Profile{Name: boundedText(c.opts.Redact(name), maxIDRunes), Type: boundedText(configured.Type, maxIDRunes), ScopeCount: len(configured.Paths), CommandCount: len(configured.Commands), LatestStatus: "missing"}
		if row, ok := latest[name]; ok {
			item.LatestStatus = "persisted_" + row.Status + "_unverified"
			at := row.CreatedAt
			item.LatestAt = &at
		}
		out = append(out, item)
	}
	return out
}

func trim[T any](items []T, limit int) []T {
	if len(items) > limit {
		return items[:limit]
	}
	return items
}

func boundedText(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	if limit <= 1 {
		return "…"
	}
	return string(runes[:limit-1]) + "…"
}

func exceedsRunes(value string, limit int) bool { return len([]rune(value)) > limit }
