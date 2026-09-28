package agent

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/identity"
)

const (
	RuntimeEndCompleted    = "completed"
	RuntimeEndDisconnected = "disconnected"
	RuntimeEndError        = "error"
	RuntimeEndReplaced     = "replaced"
	RuntimeEndShutdown     = "shutdown"
)

var ErrRuntimeEnded = errors.New("agent runtime has ended")

var canonicalClientFamilies = map[string]ClientInfo{
	"aider":              {Name: "aider", Title: "Aider"},
	"anthropic-claude":   {Name: "claude-code", Title: "Claude Code"},
	"claude":             {Name: "claude-code", Title: "Claude Code"},
	"claude-code":        {Name: "claude-code", Title: "Claude Code"},
	"claude-desktop":     {Name: "claude-code", Title: "Claude Code"},
	"cline":              {Name: "cline", Title: "Cline"},
	"codex":              {Name: "codex", Title: "Codex"},
	"codex-cli":          {Name: "codex", Title: "Codex"},
	"codex-desktop":      {Name: "codex", Title: "Codex"},
	"continue":           {Name: "continue", Title: "Continue"},
	"cursor":             {Name: "cursor", Title: "Cursor"},
	"gemini":             {Name: "gemini-cli", Title: "Gemini CLI"},
	"gemini-cli":         {Name: "gemini-cli", Title: "Gemini CLI"},
	"github-copilot":     {Name: "github-copilot", Title: "GitHub Copilot"},
	"mcp-inspector":      {Name: "mcp-inspector", Title: "MCP Inspector"},
	"openai-codex":       {Name: "codex", Title: "Codex"},
	"roo":                {Name: "roo-code", Title: "Roo Code"},
	"roo-code":           {Name: "roo-code", Title: "Roo Code"},
	"visual-studio-code": {Name: "vscode", Title: "Visual Studio Code"},
	"vscode":             {Name: "vscode", Title: "Visual Studio Code"},
	"windsurf":           {Name: "windsurf", Title: "Windsurf"},
	"zed":                {Name: "zed", Title: "Zed"},
}

type ClientInfo struct {
	Name, Title, Version string
}

type PresenceStart struct {
	ProjectID, WorkspaceID, TaskID, SessionID string
	Client                                    ClientInfo
}

type RuntimePresence struct {
	ID                                         string
	ProjectID, WorkspaceID, TaskID, SessionID  string
	Client                                     ClientInfo
	StartedAt, LastHeartbeatAt, LastActivityAt time.Time
	Sequence                                   int64
	EndedAt                                    *time.Time
	EndReason                                  string
}

type PresenceStore struct {
	db    *sql.DB
	clock app.Clock
}

func NewPresenceStore(db *sql.DB, clock app.Clock) (*PresenceStore, error) {
	if db == nil || clock == nil {
		return nil, errors.New("agent presence store: needs a database handle and clock")
	}
	return &PresenceStore{db: db, clock: clock}, nil
}

func (s *PresenceStore) Start(ctx context.Context, in PresenceStart) (RuntimePresence, error) {
	if err := requireAgentTelemetrySchema(ctx, s.db); err != nil {
		return RuntimePresence{}, err
	}
	if err := validatePresenceAttribution(in); err != nil {
		return RuntimePresence{}, err
	}
	safeClient := CanonicalClientInfo(in.Client)
	now := s.clock.Now().UTC()
	runtime := RuntimePresence{
		ID: identity.NewID("RUN"), ProjectID: in.ProjectID, WorkspaceID: in.WorkspaceID,
		TaskID: in.TaskID, SessionID: in.SessionID, Client: safeClient,
		StartedAt: now, LastHeartbeatAt: now, LastActivityAt: now, Sequence: 1,
	}
	result, err := s.db.ExecContext(ctx, `INSERT INTO agent_runtimes (
		runtime_id, project_id, workspace_id, task_id, session_id,
		client_name, client_title, client_version, started_at,
		last_heartbeat_at, last_activity_at, sequence, ended_at, end_reason)
	SELECT ?, ?, ?, ?, ?, ?, NULLIF(?, ''), NULLIF(?, ''), ?, ?, ?, 1, NULL, NULL
	FROM projects p
	JOIN workspaces w ON w.workspace_id = ? AND w.project_id = p.project_id
	JOIN sessions s ON s.session_id = ? AND s.workspace_id = w.workspace_id
	JOIN tasks t ON t.task_id = ? AND t.project_id = p.project_id
	WHERE p.project_id = ?`,
		runtime.ID, runtime.ProjectID, runtime.WorkspaceID, runtime.TaskID, runtime.SessionID,
		runtime.Client.Name, runtime.Client.Title, runtime.Client.Version,
		app.FormatTime(now), app.FormatTime(now), app.FormatTime(now),
		runtime.WorkspaceID, runtime.SessionID, runtime.TaskID, runtime.ProjectID)
	if err != nil {
		return RuntimePresence{}, fmt.Errorf("start agent runtime: %w", err)
	}
	written, err := result.RowsAffected()
	if err != nil {
		return RuntimePresence{}, fmt.Errorf("start agent runtime: %w", err)
	}
	if written != 1 {
		return RuntimePresence{}, errors.New("agent presence store: project, workspace, task and session attribution do not agree")
	}
	return runtime, nil
}

func (s *PresenceStore) Heartbeat(ctx context.Context, runtimeID string) (RuntimePresence, error) {
	return s.advance(ctx, runtimeID, false)
}

func (s *PresenceStore) Activity(ctx context.Context, runtimeID string) (RuntimePresence, error) {
	return s.advance(ctx, runtimeID, true)
}

func (s *PresenceStore) advance(ctx context.Context, runtimeID string, activity bool) (RuntimePresence, error) {
	if err := requireAgentTelemetrySchema(ctx, s.db); err != nil {
		return RuntimePresence{}, err
	}
	if runtimeID == "" {
		return RuntimePresence{}, errors.New("agent presence store: runtime id is required")
	}
	now := app.FormatTime(s.clock.Now())
	query := `UPDATE agent_runtimes SET last_heartbeat_at = ?, sequence = sequence + 1
		WHERE runtime_id = ? AND ended_at IS NULL`
	args := []any{now, runtimeID}
	if activity {
		query = `UPDATE agent_runtimes SET last_heartbeat_at = ?, last_activity_at = ?, sequence = sequence + 1
			WHERE runtime_id = ? AND ended_at IS NULL`
		args = []any{now, now, runtimeID}
	}
	result, err := s.db.ExecContext(ctx, query, args...)
	if err != nil {
		return RuntimePresence{}, fmt.Errorf("update agent runtime presence: %w", err)
	}
	written, err := result.RowsAffected()
	if err != nil {
		return RuntimePresence{}, fmt.Errorf("update agent runtime presence: %w", err)
	}
	runtime, readErr := s.Get(ctx, runtimeID)
	if readErr != nil {
		return RuntimePresence{}, readErr
	}
	if written == 0 {
		return runtime, ErrRuntimeEnded
	}
	return runtime, nil
}

func (s *PresenceStore) End(ctx context.Context, runtimeID, reason string) (RuntimePresence, error) {
	if err := requireAgentTelemetrySchema(ctx, s.db); err != nil {
		return RuntimePresence{}, err
	}
	if runtimeID == "" || !validEndReason(reason) {
		return RuntimePresence{}, errors.New("agent presence store: runtime id and valid end reason are required")
	}
	now := app.FormatTime(s.clock.Now())
	if _, err := s.db.ExecContext(ctx, `UPDATE agent_runtimes
		SET ended_at = ?, end_reason = ?, sequence = sequence + 1
		WHERE runtime_id = ? AND ended_at IS NULL`, now, reason, runtimeID); err != nil {
		return RuntimePresence{}, fmt.Errorf("end agent runtime: %w", err)
	}
	return s.Get(ctx, runtimeID)
}

func (s *PresenceStore) Get(ctx context.Context, runtimeID string) (RuntimePresence, error) {
	if err := requireAgentTelemetrySchema(ctx, s.db); err != nil {
		return RuntimePresence{}, err
	}
	if runtimeID == "" {
		return RuntimePresence{}, errors.New("agent presence store: runtime id is required")
	}
	runtime, err := scanRuntime(s.db.QueryRowContext(ctx, runtimeColumns+` WHERE runtime_id = ?`, runtimeID))
	if errors.Is(err, sql.ErrNoRows) {
		return RuntimePresence{}, errors.New("agent presence store: runtime not found")
	}
	return runtime, err
}

func (s *PresenceStore) ListProject(ctx context.Context, projectID string, limit int) ([]RuntimePresence, error) {
	if err := requireAgentTelemetrySchema(ctx, s.db); err != nil {
		return nil, err
	}
	if projectID == "" || limit < 1 || limit > 200 {
		return nil, errors.New("agent presence store: listing needs a project and a limit from 1 to 200")
	}
	rows, err := s.db.QueryContext(ctx, runtimeColumns+` WHERE project_id = ? ORDER BY runtime_id DESC LIMIT ?`, projectID, limit)
	if err != nil {
		return nil, fmt.Errorf("list agent runtimes: %w", err)
	}
	defer rows.Close()
	result := make([]RuntimePresence, 0)
	for rows.Next() {
		runtime, err := scanRuntime(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, runtime)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list agent runtimes: %w", err)
	}
	return result, nil
}

const runtimeColumns = `SELECT runtime_id, project_id, workspace_id, task_id, session_id,
	client_name, client_title, client_version, started_at, last_heartbeat_at,
	last_activity_at, sequence, ended_at, end_reason FROM agent_runtimes`

func scanRuntime(row rowScanner) (RuntimePresence, error) {
	var runtime RuntimePresence
	var title, version, ended, reason sql.NullString
	var started, heartbeat, activity string
	if err := row.Scan(&runtime.ID, &runtime.ProjectID, &runtime.WorkspaceID, &runtime.TaskID, &runtime.SessionID,
		&runtime.Client.Name, &title, &version, &started, &heartbeat, &activity,
		&runtime.Sequence, &ended, &reason); err != nil {
		return RuntimePresence{}, err
	}
	runtime.Client.Title, runtime.Client.Version = title.String, version.String
	var err error
	if runtime.StartedAt, err = app.ParseTime(started); err != nil {
		return RuntimePresence{}, err
	}
	if runtime.LastHeartbeatAt, err = app.ParseTime(heartbeat); err != nil {
		return RuntimePresence{}, err
	}
	if runtime.LastActivityAt, err = app.ParseTime(activity); err != nil {
		return RuntimePresence{}, err
	}
	if ended.Valid {
		parsed, err := app.ParseTime(ended.String)
		if err != nil {
			return RuntimePresence{}, err
		}
		runtime.EndedAt = &parsed
		runtime.EndReason = reason.String
	}
	runtime.Client = CanonicalClientInfo(runtime.Client)
	return runtime, nil
}

func validatePresenceAttribution(in PresenceStart) error {
	if in.ProjectID == "" || in.WorkspaceID == "" || in.TaskID == "" || in.SessionID == "" {
		return errors.New("agent presence store: runtime attribution is required")
	}
	return nil
}

// CanonicalClientInfo is the persistence and read boundary for self-reported MCP client
// metadata. Caller strings are never copied through. A known name becomes a
// canonical family and title; every other value collapses to controlled
// metadata. Title and version are never copied from the caller. This keeps
// presence useful without turning ClientInfo into an arbitrary durable text
// channel: shape checks cannot prove that apparently ordinary text is not a
// credential.
func CanonicalClientInfo(raw ClientInfo) ClientInfo {
	secret := strings.TrimSpace(os.Getenv("TYPESAFE_API_KEY"))
	family, known := canonicalClientFamily(raw.Name)
	if !known || containsConfiguredCredential(family.Name, secret) {
		family = ClientInfo{Name: safeUnknownClientName(secret)}
	} else if containsConfiguredCredential(family.Title, secret) {
		family.Title = ""
	}
	return family
}

func canonicalClientFamily(name string) (ClientInfo, bool) {
	if len(name) == 0 || len(name) > 128 || !utf8.ValidString(name) || strings.TrimSpace(name) != name {
		return ClientInfo{}, false
	}
	for _, character := range name {
		if unicode.IsControl(character) {
			return ClientInfo{}, false
		}
	}
	family, ok := canonicalClientFamilies[strings.ToLower(name)]
	return family, ok
}

func containsConfiguredCredential(value, secret string) bool {
	return value != "" && secret != "" && strings.Contains(value, secret)
}

func safeUnknownClientName(secret string) string {
	// "unknown" and "redacted" have disjoint character sets, so no non-empty
	// secret can be a substring of both. The first value remains the stable
	// ordinary placeholder; the others only handle a credential collision.
	for _, candidate := range []string{"unknown-client", "unknown", "redacted"} {
		if !containsConfiguredCredential(candidate, secret) {
			return candidate
		}
	}
	panic("unreachable: safe client placeholders have no shared character")
}

func validEndReason(reason string) bool {
	switch reason {
	case RuntimeEndCompleted, RuntimeEndDisconnected, RuntimeEndError, RuntimeEndReplaced, RuntimeEndShutdown:
		return true
	default:
		return false
	}
}
