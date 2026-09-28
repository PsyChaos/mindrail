package agent_test

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/PsyChaos/mindrail/internal/agent"
	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/migration"
	"github.com/PsyChaos/mindrail/internal/storage"
	"github.com/PsyChaos/mindrail/migrations"
)

type agentStoreFixture struct {
	db *storage.DB
}

func newAgentStoreFixture(t *testing.T) agentStoreFixture {
	t.Helper()
	db, err := storage.Open(t.Context(), storage.Options{Path: filepath.Join(t.TempDir(), "mindrail.db")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	set, err := migration.Load(migrations.FS)
	if err != nil {
		t.Fatal(err)
	}
	clock := app.FixedClock{Instant: time.Date(2026, 9, 28, 8, 0, 0, 0, time.UTC)}
	if _, err := migration.New(db.DB, set, clock).Up(t.Context()); err != nil {
		t.Fatal(err)
	}
	for _, stmt := range []string{
		`INSERT INTO projects (project_id, common_dir, registered_at) VALUES ('PRJ-1', '/repo/.git', '2026-09-28T08:00:00Z')`,
		`INSERT INTO workspaces (workspace_id, project_id, root_path, git_dir, is_linked_worktree, registered_at, last_seen_at)
		 VALUES ('WSP-1', 'PRJ-1', '/repo', '/repo/.git', 0, '2026-09-28T08:00:00Z', '2026-09-28T08:00:00Z')`,
		`INSERT INTO sessions (session_id, workspace_id, label, started_at) VALUES ('SES-1', 'WSP-1', NULL, '2026-09-28T08:00:00Z')`,
		`INSERT INTO tasks (task_id, project_id, title, state, blocked_reason, opened_by, claimed_by, created_at, updated_at, revision)
		 VALUES ('TSK-1', 'PRJ-1', 'task', 'IN_PROGRESS', NULL, 'SES-1', 'SES-1', '2026-09-28T08:00:00Z', '2026-09-28T08:00:00Z', 1)`,
	} {
		if _, err := db.ExecContext(t.Context(), stmt); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	return agentStoreFixture{db: db}
}

func intptr(value int) *int { return &value }

func TestRouteStoreRecordsAndListsOnlyBoundedMetadata(t *testing.T) {
	fx := newAgentStoreFixture(t)
	store, err := agent.NewRouteStore(fx.db.DB)
	if err != nil {
		t.Fatal(err)
	}
	started := time.Date(2026, 9, 28, 8, 1, 2, 300_000_000, time.UTC)
	event, err := store.Record(t.Context(), agent.RouteEventInput{
		ProjectID: "PRJ-1", WorkspaceID: "WSP-1", TaskID: "TSK-1", SessionID: "SES-1",
		Provider: agent.RouteProviderTypeSafe, Model: agent.RouteModelJEVLatest, Version: 1,
		Status: agent.RouteStatusOK, Reason: agent.RouteReasonAdviceAvailable,
		CredentialSource: agent.RouteCredentialKeyring,
		StartedAt:        started, EndedAt: started.Add(125 * time.Millisecond),
		Tool:  agent.RouteDimension{CandidateCount: 3, SelectedOrdinal: intptr(1), ConfidenceMilli: intptr(875)},
		Agent: agent.RouteDimension{CandidateCount: 2, SelectedOrdinal: intptr(0), ConfidenceMilli: intptr(910)},
	})
	if err != nil {
		t.Fatal(err)
	}
	if event.ID == "" || event.DurationMS != 125 || event.Tool.CandidateCount != 3 || *event.Tool.SelectedOrdinal != 1 {
		t.Fatalf("recorded event = %+v", event)
	}
	listed, err := store.ListProject(t.Context(), "PRJ-1", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 || listed[0].ID != event.ID {
		t.Fatalf("listed = %+v", listed)
	}

	columns, err := fx.db.QueryContext(t.Context(), `PRAGMA table_info(jev_route_events)`)
	if err != nil {
		t.Fatal(err)
	}
	defer columns.Close()
	for columns.Next() {
		var cid, notNull, primaryKey int
		var name, typ string
		var defaultValue any
		if err := columns.Scan(&cid, &name, &typ, &notNull, &defaultValue, &primaryKey); err != nil {
			t.Fatal(err)
		}
		lower := strings.ToLower(name)
		for _, prohibited := range []string{"goal", "context", "description", "candidate_id", "provider_body", "credential_value", "api_key", "prompt", "raw_source", "raw_path", "raw_log"} {
			if strings.Contains(lower, prohibited) {
				t.Fatalf("jev_route_events has prohibited column %q", name)
			}
		}
	}
}

func TestRouteStoreRejectsUnsafeShapeAndMismatchedAttribution(t *testing.T) {
	fx := newAgentStoreFixture(t)
	store, err := agent.NewRouteStore(fx.db.DB)
	if err != nil {
		t.Fatal(err)
	}
	started := time.Date(2026, 9, 28, 8, 1, 0, 0, time.UTC)
	base := agent.RouteEventInput{
		ProjectID: "PRJ-1", WorkspaceID: "WSP-1", TaskID: "TSK-1", SessionID: "SES-1",
		Provider: agent.RouteProviderTypeSafe, Model: agent.RouteModelJEVLatest, Version: 1,
		Status: agent.RouteStatusFallback, Reason: "no_match",
		CredentialSource: agent.RouteCredentialKeyring,
		StartedAt:        started, EndedAt: started.Add(time.Millisecond),
		Tool: agent.RouteDimension{CandidateCount: 2, ConfidenceMilli: intptr(770)},
	}
	event, err := store.Record(t.Context(), base)
	if err != nil {
		t.Fatalf("confidence-only no-match telemetry: %v", err)
	}
	if event.Tool.SelectedOrdinal != nil || event.Tool.ConfidenceMilli == nil || *event.Tool.ConfidenceMilli != 770 {
		t.Fatalf("no-match dimension = %+v", event.Tool)
	}

	bad := base
	bad.Tool = agent.RouteDimension{CandidateCount: 33}
	if _, err := store.Record(t.Context(), bad); err == nil {
		t.Fatal("33 candidates were accepted")
	}
	bad = base
	bad.WorkspaceID = "WSP-MISSING"
	if _, err := store.Record(t.Context(), bad); err == nil {
		t.Fatal("mismatched attribution was accepted")
	}
	if _, err := fx.db.ExecContext(t.Context(), `INSERT INTO jev_route_events (
		route_id, project_id, workspace_id, task_id, session_id, provider, model, version,
		status, reason, credential_source, started_at, ended_at, duration_ms,
		tool_candidate_count, agent_candidate_count, model_candidate_count, effort_candidate_count)
		VALUES ('JRV-RAW', 'PRJ-1', 'WSP-1', 'TSK-1', 'SES-1', 'typesafe', 'jev-latest', 1,
		'ok', 'advice_available', 'keyring', '2026-09-28T08:00:00Z', '2026-09-28T08:00:00Z', 0,
		33, 0, 0, 0)`); err == nil {
		t.Fatal("schema accepted an out-of-bounds candidate count")
	}
}
