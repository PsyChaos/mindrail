package dashboard

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/PsyChaos/mindrail/internal/config"
	"github.com/PsyChaos/mindrail/internal/status"
	"github.com/PsyChaos/mindrail/internal/storage"
	"github.com/PsyChaos/mindrail/internal/workspace"
)

func TestCollectorBuildsBoundedSanitizedOperationsSnapshot(t *testing.T) {
	db := dashboardDB(t)
	now := time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)
	note := strings.Repeat("checkpoint-detail-", 40)
	mustExec(t, db, `INSERT INTO sessions VALUES ('SES-1','WS-1','reviewer','2026-09-27T10:00:00Z')`)
	mustExec(t, db, `INSERT INTO tasks VALUES ('TSK-1','PRJ-1','Protect the gate','BLOCKED','waiting on evidence','SES-1','SES-1','2026-09-27T10:00:00Z','2026-09-27T11:00:00Z',7)`)
	mustExec(t, db, `INSERT INTO leases VALUES ('LSE-1','PRJ-1','task','TSK-1','SES-1','2026-09-27T10:00:00Z','2026-09-27T10:00:00Z','2026-09-27T12:20:00Z',NULL,NULL)`)
	if _, err := db.Exec(`INSERT INTO checkpoints VALUES ('CKP-1','TSK-1','SES-1','WS-1',?,1,'2026-09-27T11:30:00Z')`, note); err != nil {
		t.Fatal(err)
	}
	mustExec(t, db, `INSERT INTO evidence VALUES ('EVD-1','ci','CI_VERIFICATION','pass',0,'0123456789abcdef','2026-09-27T11:45:00Z')`)

	collector, err := NewCollector(CollectorOptions{DB: db, ProjectID: "PRJ-1", Workspace: workspace.Workspace{ID: "WS-1", ProjectID: "PRJ-1", IsLinkedWorktree: true},
		ProjectName: "Mindrail", WorktreeRoot: "/repo", Profiles: map[string]config.ValidationProfile{"ci": {Type: "CI_VERIFICATION", Paths: []string{"."}, Commands: [][]string{{"make", "gate"}}}},
		Readiness: status.Report{Readiness: status.ReadinessReady, Components: map[status.ComponentName]status.Component{status.ComponentRuntimeDB: {Summary: "runtime at super-secret"}}},
		JEV:       JEVState{Configured: true, Source: "environment", Provider: "typesafe", Mode: "optional_advisory"},
		StartedAt: time.Date(2026, 9, 27, 9, 30, 0, 0, time.UTC), Redact: func(value string) string {
			return strings.NewReplacer("evidence", "[REDACTED]", "super-secret", "[REDACTED]").Replace(value)
		}, Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := collector.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Summary.States["BLOCKED"] != 1 || snapshot.Summary.ActiveLeases != 1 || snapshot.Summary.AgentsEngaged != 1 {
		t.Fatalf("summary = %#v", snapshot.Summary)
	}
	if len(snapshot.Tasks) != 1 || snapshot.Tasks[0].Revision != 7 {
		t.Fatalf("tasks = %#v", snapshot.Tasks)
	}
	if strings.Contains(snapshot.Tasks[0].BlockedReason, "evidence") {
		t.Fatalf("task free text was not redacted: %#v", snapshot.Tasks[0])
	}
	if len(snapshot.Checkpoints) != 1 {
		t.Fatalf("checkpoint metadata missing: %#v", snapshot.Checkpoints)
	}
	if !snapshot.JEV.Configured || snapshot.JEV.Source != "environment" {
		t.Fatalf("JEV metadata = %#v", snapshot.JEV)
	}
	if snapshot.Dashboard.StartedAt != time.Date(2026, 9, 27, 9, 30, 0, 0, time.UTC) || snapshot.Dashboard.UptimeSeconds != 9_000 {
		t.Fatalf("dashboard runtime = %#v", snapshot.Dashboard)
	}
	if len(snapshot.Sessions) != 1 {
		t.Fatalf("sessions = %#v", snapshot.Sessions)
	}
	session := snapshot.Sessions[0]
	if session.ActivityStatus != "active_signal" || session.CurrentTaskCount != 1 || len(session.CurrentTasks) != 1 || session.CurrentTasks[0].ID != "TSK-1" {
		t.Fatalf("session activity = %#v", session)
	}
	if session.LatestActivityAt == nil || !session.LatestActivityAt.Equal(time.Date(2026, 9, 27, 11, 30, 0, 0, time.UTC)) {
		t.Fatalf("latest activity = %#v", session.LatestActivityAt)
	}
	if session.LatestLeaseRenewedAt == nil || session.NextLeaseExpiresAt == nil || !session.NextLeaseExpiresAt.Equal(time.Date(2026, 9, 27, 12, 20, 0, 0, time.UTC)) {
		t.Fatalf("lease projection = %#v", session)
	}
	if snapshot.CI.Status != "persisted_pass_unverified" || snapshot.Merge.Status != "unavailable" || snapshot.Completion.Status != "on_demand" {
		t.Fatalf("provider states = %#v %#v %#v", snapshot.CI, snapshot.Merge, snapshot.Completion)
	}
	payload, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"super-secret", "\"command_argv\":", "\"provenance\":", "checkpoint-detail-checkpoint-detail-checkpoint-detail-checkpoint-detail-checkpoint-detail-checkpoint-detail-checkpoint-detail-checkpoint-detail-checkpoint-detail-checkpoint-detail-checkpoint-detail-checkpoint-detail-checkpoint-detail-checkpoint-detail-checkpoint-detail-checkpoint-detail-checkpoint-detail-checkpoint-detail-checkpoint-detail-checkpoint-detail-checkpoint-detail-checkpoint-detail-checkpoint-detail-checkpoint-detail-checkpoint-detail-checkpoint-detail-checkpoint-detail-checkpoint-detail-checkpoint-detail-checkpoint-detail-checkpoint-detail-checkpoint-detail-checkpoint-detail-checkpoint-detail-checkpoint-detail-checkpoint-detail-checkpoint-detail-checkpoint-detail-checkpoint-detail-checkpoint-detail-"} {
		if strings.Contains(string(payload), forbidden) {
			t.Fatalf("payload leaked %q", forbidden)
		}
	}
}

func TestCollectorBoundsFreeTextWithoutLeakingSecretPrefixes(t *testing.T) {
	db := dashboardDB(t)
	secret := strings.Repeat("s", 80)
	prefix := strings.Repeat("x", maxTextRunes-20)
	mustExec(t, db, `INSERT INTO sessions VALUES ('SES-LONG','WS-1',?, '2026-09-27T10:00:00Z')`, prefix+secret)
	mustExec(t, db, `INSERT INTO tasks VALUES ('TSK-LONG','PRJ-1',?,'OPEN',?,'SES-LONG',NULL,'2026-09-27T10:00:00Z','2026-09-27T11:00:00Z',1)`, prefix+secret, prefix+secret)
	mustExec(t, db, `INSERT INTO leases VALUES ('LSE-LONG','PRJ-1','task','TSK-LONG','SES-LONG','2026-09-27T10:00:00Z','2026-09-27T10:00:00Z','2026-09-27T12:20:00Z','2026-09-27T12:00:00Z',?)`, prefix+secret)
	collector, err := NewCollector(CollectorOptions{DB: db, ProjectID: "PRJ-1", Workspace: workspace.Workspace{ID: "WS-1", ProjectID: "PRJ-1"}, WorktreeRoot: "/repo",
		Redact: func(value string) string { return strings.ReplaceAll(value, secret, "[REDACTED]") }})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := collector.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(payload), strings.Repeat("s", 20)) || !snapshot.Truncated["tasks"] || !snapshot.Truncated["sessions"] || !snapshot.Truncated["leases"] {
		t.Fatalf("unsafe or undisclosed truncation: truncated=%v payload=%s", snapshot.Truncated, payload)
	}
}

func TestCollectorSummaryIsExactWhenDetailIsTruncated(t *testing.T) {
	db := dashboardDB(t)
	mustExec(t, db, `INSERT INTO sessions VALUES ('SES-1','WS-1','worker','2026-09-27T10:00:00Z')`)
	for i := 0; i < maxTasks+5; i++ {
		mustExec(t, db, `INSERT INTO tasks VALUES (?, 'PRJ-1','task','OPEN',NULL,'SES-1',NULL,'2026-09-27T10:00:00Z','2026-09-27T11:00:00Z',1)`, fmt.Sprintf("TSK-%03d", i))
	}
	collector, err := NewCollector(CollectorOptions{DB: db, ProjectID: "PRJ-1", Workspace: workspace.Workspace{ID: "WS-1", ProjectID: "PRJ-1"}, WorktreeRoot: "/repo"})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := collector.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Tasks) != maxTasks || snapshot.Summary.States["OPEN"] != maxTasks+5 || !snapshot.Truncated["tasks"] {
		t.Fatalf("details=%d summary=%#v truncated=%v", len(snapshot.Tasks), snapshot.Summary, snapshot.Truncated)
	}
}

func TestCollectorSessionActivityIsIndependentOfTaskDetailTruncation(t *testing.T) {
	db := dashboardDB(t)
	now := time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)
	mustExec(t, db, `INSERT INTO sessions VALUES ('SES-1','WS-1','worker','2026-09-27T08:00:00Z')`)
	mustExec(t, db, `INSERT INTO tasks VALUES ('TSK-CURRENT','PRJ-1','current work','IN_PROGRESS',NULL,'SES-1','SES-1','2026-09-27T08:00:00Z','2026-09-27T08:30:00Z',1)`)
	mustExec(t, db, `INSERT INTO leases VALUES ('LSE-EXPIRED','PRJ-1','task','TSK-CURRENT','SES-1','2026-09-27T08:00:00Z','2026-09-27T08:10:00Z','2026-09-27T09:00:00Z',NULL,NULL)`)
	for i := 0; i < maxTasks+5; i++ {
		mustExec(t, db, `INSERT INTO tasks VALUES (?, 'PRJ-1','newer noise','OPEN',NULL,'SES-1',NULL,'2026-09-27T10:00:00Z','2026-09-27T11:00:00Z',1)`, fmt.Sprintf("TSK-NOISE-%03d", i))
	}
	collector, err := NewCollector(CollectorOptions{DB: db, ProjectID: "PRJ-1", Workspace: workspace.Workspace{ID: "WS-1", ProjectID: "PRJ-1"}, WorktreeRoot: "/repo", Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := collector.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !snapshot.Truncated["tasks"] {
		t.Fatalf("task details were not truncated: %v", snapshot.Truncated)
	}
	for _, task := range snapshot.Tasks {
		if task.ID == "TSK-CURRENT" {
			t.Fatal("test setup did not push current task out of bounded top-level details")
		}
	}
	session := snapshot.Sessions[0]
	if session.CurrentTaskCount != 1 || len(session.CurrentTasks) != 1 || session.CurrentTasks[0].ID != "TSK-CURRENT" {
		t.Fatalf("current task projection = %#v", session)
	}
	if session.LeaseCount != 0 || session.ActivityStatus != "claim_only" || session.LatestLeaseRenewedAt != nil || session.NextLeaseExpiresAt != nil {
		t.Fatalf("expired lease was presented as active: %#v", session)
	}
}

func TestCollectorSessionCurrentTaskProjectionDisclosesItsOwnBound(t *testing.T) {
	db := dashboardDB(t)
	mustExec(t, db, `INSERT INTO sessions VALUES ('SES-1','WS-1','worker','2026-09-27T08:00:00Z')`)
	for i := 0; i < maxSessionTasks+3; i++ {
		mustExec(t, db, `INSERT INTO tasks VALUES (?, 'PRJ-1','claimed task','IN_PROGRESS',NULL,'SES-1','SES-1','2026-09-27T10:00:00Z','2026-09-27T11:00:00Z',1)`, fmt.Sprintf("TSK-%03d", i))
	}
	collector, err := NewCollector(CollectorOptions{DB: db, ProjectID: "PRJ-1", Workspace: workspace.Workspace{ID: "WS-1", ProjectID: "PRJ-1"}, WorktreeRoot: "/repo"})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := collector.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	session := snapshot.Sessions[0]
	if session.CurrentTaskCount != maxSessionTasks+3 || len(session.CurrentTasks) != maxSessionTasks || !session.CurrentTasksTruncated {
		t.Fatalf("current task bound = %#v", session)
	}
}

func TestCollectorLongTaskTitleDoesNotClaimAnotherTaskWasOmitted(t *testing.T) {
	db := dashboardDB(t)
	mustExec(t, db, `INSERT INTO sessions VALUES ('SES-1','WS-1','worker','2026-09-27T08:00:00Z')`)
	mustExec(t, db, `INSERT INTO tasks VALUES ('TSK-1','PRJ-1',?,'IN_PROGRESS',NULL,'SES-1','SES-1','2026-09-27T10:00:00Z','2026-09-27T11:00:00Z',1)`, strings.Repeat("x", maxTextRunes+1))
	collector, err := NewCollector(CollectorOptions{DB: db, ProjectID: "PRJ-1", Workspace: workspace.Workspace{ID: "WS-1", ProjectID: "PRJ-1"}, WorktreeRoot: "/repo"})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := collector.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	session := snapshot.Sessions[0]
	if session.CurrentTaskCount != 1 || len(session.CurrentTasks) != 1 || session.CurrentTasksTruncated {
		t.Fatalf("long title changed list truncation semantics: %#v", session)
	}
	if session.CurrentTasks[0].Title != "[TRUNCATED]" {
		t.Fatalf("bounded title = %q", session.CurrentTasks[0].Title)
	}
}

func TestCollectorLatestActivityOrdersRFC3339NanoExactly(t *testing.T) {
	db := dashboardDB(t)
	mustExec(t, db, `INSERT INTO sessions VALUES ('SES-1','WS-1','worker','2026-09-27T08:00:00Z')`)
	mustExec(t, db, `INSERT INTO tasks VALUES ('TSK-1','PRJ-1','task','IN_PROGRESS',NULL,'SES-1','SES-1','2026-09-27T10:00:00Z','2026-09-27T11:00:00Z',1)`)
	mustExec(t, db, `INSERT INTO checkpoints VALUES ('CKP-1','TSK-1','SES-1','WS-1','',0,'2026-09-27T11:00:00.1Z')`)
	collector, err := NewCollector(CollectorOptions{DB: db, ProjectID: "PRJ-1", Workspace: workspace.Workspace{ID: "WS-1", ProjectID: "PRJ-1"}, WorktreeRoot: "/repo"})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := collector.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, 9, 27, 11, 0, 0, 100_000_000, time.UTC)
	if got := snapshot.Sessions[0].LatestActivityAt; got == nil || !got.Equal(want) {
		t.Fatalf("latest activity = %v, want %v", got, want)
	}
}

func TestCollectorTaskActivityDistinguishesOpenerFromClaimant(t *testing.T) {
	db := dashboardDB(t)
	mustExec(t, db, `INSERT INTO sessions VALUES ('SES-OPENER','WS-1','opener','2026-09-27T08:00:00Z')`)
	mustExec(t, db, `INSERT INTO sessions VALUES ('SES-CLAIMANT','WS-1','claimant','2026-09-27T09:00:00Z')`)
	mustExec(t, db, `INSERT INTO tasks VALUES ('TSK-1','PRJ-1','task','IN_PROGRESS',NULL,'SES-OPENER','SES-CLAIMANT','2026-09-27T10:00:00Z','2026-09-27T11:00:00Z',2)`)
	collector, err := NewCollector(CollectorOptions{DB: db, ProjectID: "PRJ-1", Workspace: workspace.Workspace{ID: "WS-1", ProjectID: "PRJ-1"}, WorktreeRoot: "/repo"})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := collector.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]Session{}
	for _, session := range snapshot.Sessions {
		byID[session.ID] = session
	}
	openerWant := time.Date(2026, 9, 27, 10, 0, 0, 0, time.UTC)
	if got := byID["SES-OPENER"].LatestActivityAt; got == nil || !got.Equal(openerWant) {
		t.Fatalf("opener latest activity = %v, want task creation %v", got, openerWant)
	}
	claimantWant := time.Date(2026, 9, 27, 11, 0, 0, 0, time.UTC)
	if got := byID["SES-CLAIMANT"].LatestActivityAt; got == nil || !got.Equal(claimantWant) {
		t.Fatalf("claimant latest activity = %v, want task update %v", got, claimantWant)
	}
}

func TestSessionQueryScopeBuildsOnlyDisplayedSessionArguments(t *testing.T) {
	placeholders, args := sessionQueryScope([]Session{{ID: "SES-A"}, {ID: "SES-B"}, {ID: "SES-C"}})
	if placeholders != "?,?,?" {
		t.Fatalf("placeholders = %q", placeholders)
	}
	want := []any{"SES-A", "SES-B", "SES-C"}
	if fmt.Sprint(args) != fmt.Sprint(want) {
		t.Fatalf("args = %#v, want %#v", args, want)
	}
	if placeholders, args := sessionQueryScope(nil); placeholders != "" || args != nil {
		t.Fatalf("empty scope = %q %#v", placeholders, args)
	}
}

func TestSessionActivityScopeIsStableWithLargeInvisibleHistory(t *testing.T) {
	db := dashboardDB(t)
	now := time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)
	visible := Session{ID: "SES-VISIBLE", WorkspaceID: "WS-1", StartedAt: time.Date(2026, 9, 27, 8, 0, 0, 0, time.UTC)}
	mustExec(t, db, `INSERT INTO sessions VALUES ('SES-VISIBLE','WS-1','visible','2026-09-27T08:00:00Z')`)
	mustExec(t, db, `INSERT INTO tasks VALUES ('TSK-VISIBLE','PRJ-1','visible task','IN_PROGRESS',NULL,'SES-VISIBLE','SES-VISIBLE','2026-09-27T09:00:00Z','2026-09-27T10:00:00Z',1)`)
	mustExec(t, db, `INSERT INTO leases VALUES ('LSE-VISIBLE','PRJ-1','task','TSK-VISIBLE','SES-VISIBLE','2026-09-27T09:00:00Z','2026-09-27T10:30:00Z','2026-09-27T12:30:00Z',NULL,NULL)`)
	for i := 0; i < maxSessions*3; i++ {
		sessionID := fmt.Sprintf("SES-HIST-%03d", i)
		taskID := fmt.Sprintf("TSK-HIST-%03d", i)
		mustExec(t, db, `INSERT INTO sessions VALUES (?,'WS-1','history','2026-01-01T00:00:00Z')`, sessionID)
		mustExec(t, db, `INSERT INTO tasks VALUES (?, 'PRJ-1','historical task','IN_PROGRESS',NULL,?,?, '2026-01-01T00:00:00Z','2026-09-27T11:59:59Z',1)`, taskID, sessionID, sessionID)
		mustExec(t, db, `INSERT INTO leases VALUES (?, 'PRJ-1','task',?,?,'2026-01-01T00:00:00Z','2026-09-27T11:59:59Z','2026-09-27T12:30:00Z',NULL,NULL)`, "LSE-HIST-"+taskID, taskID, sessionID)
	}
	collector, err := NewCollector(CollectorOptions{DB: db, ProjectID: "PRJ-1", Workspace: workspace.Workspace{ID: "WS-1", ProjectID: "PRJ-1"}, WorktreeRoot: "/repo"})
	if err != nil {
		t.Fatal(err)
	}
	tx, err := db.BeginTx(context.Background(), &sql.TxOptions{ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	activity, err := collector.sessionActivity(context.Background(), tx, []Session{visible}, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(activity) != 1 {
		t.Fatalf("activity sessions = %d, want 1", len(activity))
	}
	got := activity[visible.ID]
	if got.currentTaskCount != 1 || len(got.currentTasks) != 1 || got.activeLeaseCount != 1 {
		t.Fatalf("visible activity was expanded by invisible history: %#v", got)
	}
}

func TestCollectorOrdersCheckpointsByInsertionOrder(t *testing.T) {
	db := dashboardDB(t)
	mustExec(t, db, `INSERT INTO sessions VALUES ('SES-1','WS-1','worker','2026-09-27T10:00:00Z')`)
	mustExec(t, db, `INSERT INTO tasks VALUES ('TSK-1','PRJ-1','task','OPEN',NULL,'SES-1',NULL,'2026-09-27T10:00:00Z','2026-09-27T11:00:00Z',1)`)
	mustExec(t, db, `INSERT INTO checkpoints VALUES ('ZZZ','TSK-1','SES-1','WS-1','',0,'2026-09-27T11:00:00Z')`)
	mustExec(t, db, `INSERT INTO checkpoints VALUES ('AAA','TSK-1','SES-1','WS-1','',0,'2026-09-27T11:00:00Z')`)
	collector, err := NewCollector(CollectorOptions{DB: db, ProjectID: "PRJ-1", Workspace: workspace.Workspace{ID: "WS-1", ProjectID: "PRJ-1"}, WorktreeRoot: "/repo"})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := collector.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Checkpoints) != 2 || snapshot.Checkpoints[0].ID != "AAA" {
		t.Fatalf("checkpoints=%#v", snapshot.Checkpoints)
	}
}

func TestCollectorProviderStatesAreIndependentOfEvidenceDetailLimit(t *testing.T) {
	db := dashboardDB(t)
	profileName := "ci-" + strings.Repeat("p", maxIDRunes+20)
	mustExec(t, db, `INSERT INTO evidence VALUES ('CI-OLD',?,'CI_VERIFICATION','pass',0,'ci-hash','2026-09-27T09:00:00Z')`, profileName)
	for i := 0; i < maxEvidence+5; i++ {
		mustExec(t, db, `INSERT INTO evidence VALUES (?, 'noise','EXTERNAL_SYSTEM','pass',0,'noise-hash','2026-09-27T10:00:00Z')`, fmt.Sprintf("NOISE-%03d", i))
	}
	collector, err := NewCollector(CollectorOptions{DB: db, ProjectID: "PRJ-1", Workspace: workspace.Workspace{ID: "WS-1", ProjectID: "PRJ-1"}, WorktreeRoot: "/repo",
		Profiles: map[string]config.ValidationProfile{profileName: {Type: "CI_VERIFICATION", Paths: []string{"."}}}})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := collector.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.CI.Status != "persisted_pass_unverified" || len(snapshot.Profiles) != 1 || snapshot.Profiles[0].LatestStatus != "persisted_pass_unverified" {
		t.Fatalf("ci=%#v profiles=%#v", snapshot.CI, snapshot.Profiles)
	}
	if len(snapshot.Evidence) != maxEvidence || !snapshot.Truncated["evidence"] {
		t.Fatalf("evidence=%d truncated=%v", len(snapshot.Evidence), snapshot.Truncated)
	}
}

func dashboardDB(t *testing.T) *sql.DB {
	t.Helper()
	handle, err := storage.Open(t.Context(), storage.Options{Path: filepath.Join(t.TempDir(), "mindrail.db"), MaxOpenConns: 1})
	if err != nil {
		t.Fatal(err)
	}
	db := handle.DB
	t.Cleanup(func() { _ = handle.Close() })
	for _, schema := range []string{
		`CREATE TABLE workspaces(workspace_id TEXT, project_id TEXT)`,
		`CREATE TABLE sessions(session_id TEXT, workspace_id TEXT, label TEXT, started_at TEXT)`,
		`CREATE TABLE tasks(task_id TEXT, project_id TEXT, title TEXT, state TEXT, blocked_reason TEXT, opened_by TEXT, claimed_by TEXT, created_at TEXT, updated_at TEXT, revision INTEGER)`,
		`CREATE TABLE leases(lease_id TEXT, project_id TEXT, target_kind TEXT, target_key TEXT, holder TEXT, acquired_at TEXT, renewed_at TEXT, expires_at TEXT, released_at TEXT, release_reason TEXT)`,
		`CREATE TABLE checkpoints(checkpoint_id TEXT, task_id TEXT, session_id TEXT, workspace_id TEXT, note TEXT, handoff INTEGER, created_at TEXT)`,
		`CREATE TABLE evidence(evidence_id TEXT, profile TEXT, type TEXT, status TEXT, exit_code INTEGER, snapshot_hash TEXT, created_at TEXT)`,
		`INSERT INTO workspaces VALUES ('WS-1','PRJ-1')`,
	} {
		mustExec(t, db, schema)
	}
	return db
}

func mustExec(t *testing.T, db *sql.DB, query string, args ...any) {
	t.Helper()
	if _, err := db.Exec(query, args...); err != nil {
		t.Fatal(err)
	}
}
