package dashboard

import (
	"context"
	"database/sql"
	"errors"
	"sort"
	"strings"
	"sync/atomic"
	"time"

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
	Environ      []string
	Redact       func(string) string
	Now          func() time.Time
}

type Collector struct {
	opts CollectorOptions
	seq  atomic.Uint64
}

type queryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func NewCollector(opts CollectorOptions) (*Collector, error) {
	if opts.DB == nil || opts.ProjectID == "" || opts.Workspace.ID == "" || opts.WorktreeRoot == "" {
		return nil, errors.New("dashboard: database, project, workspace and worktree root are required")
	}
	if opts.Now == nil {
		opts.Now = func() time.Time { return time.Now().UTC() }
	}
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
	summary, engaged, taskCounts, leaseCounts, err := c.summary(ctx, tx, now)
	if err != nil {
		return Snapshot{}, err
	}
	for i := range sessions {
		sessions[i].Engaged = engaged[sessions[i].ID]
		sessions[i].TaskCount = taskCounts[sessions[i].ID]
		sessions[i].LeaseCount = leaseCounts[sessions[i].ID]
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
	jev := c.jev()
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
	if len(profiles) < len(c.opts.Profiles) {
		truncated["profiles"] = true
	}

	return Snapshot{
		Sequence: c.seq.Add(1), GeneratedAt: now,
		Project: Project{ID: boundedText(c.opts.ProjectID, maxIDRunes), Name: boundedText(c.opts.Redact(c.opts.ProjectName), maxTextRunes), WorkspaceID: boundedText(c.opts.Workspace.ID, maxIDRunes),
			LinkedWorktree: c.opts.Workspace.IsLinkedWorktree},
		Summary: summary, Tasks: tasks, Sessions: sessions, Leases: leases,
		Checkpoints: checkpoints, Evidence: evidence, Profiles: profiles, Readiness: readinessView(c.opts.Readiness, c.opts.Redact),
		CI:         ci,
		Merge:      ProviderState{Status: "unavailable", Detail: "No GitHub or merge provider is configured for this local read-only view."},
		JEV:        jev,
		Completion: CompletionState{Status: "on_demand", Detail: "Completion and testguard findings are evaluated during completion and are not persisted by this schema.", Findings: 0},
		Capabilities: Capabilities{ReadOnly: true, LiveTransport: "sse", TaskRevisionHistory: "current_revision_only",
			CheckpointNotes: "metadata_only", SensitiveData: "checkpoint text, evidence output, argv, provenance and credentials omitted",
			ReadinessFreshness: "startup_snapshot"},
		Truncated: truncated,
	}, nil
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

func (c *Collector) jev() JEVState {
	for _, entry := range c.opts.Environ {
		if strings.HasPrefix(entry, "TYPESAFE_API_KEY=") && strings.TrimSpace(strings.TrimPrefix(entry, "TYPESAFE_API_KEY=")) != "" {
			return JEVState{Configured: true, Source: "environment", Provider: "typesafe", Mode: "optional_advisory"}
		}
	}
	return JEVState{Configured: false, Source: "none", Provider: "typesafe", Mode: "normal_routing"}
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
