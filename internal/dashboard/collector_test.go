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
	mustExec(t, db, `INSERT INTO jev_route_events VALUES ('RTE-1','PRJ-1','WS-1','TSK-1','SES-1','typesafe','jev-latest',1,'ok','advice_available','keyring','2026-09-27T11:40:00Z','2026-09-27T11:40:00.120Z',120,2,1,875,0,NULL,NULL,0,NULL,NULL,0,NULL,NULL)`)
	mustExec(t, db, `INSERT INTO agent_runtimes VALUES ('RUN-1','PRJ-1','WS-1','TSK-1','SES-1','claude-code','Claude Code','1.2.3','2026-09-27T11:00:00Z','2026-09-27T11:59:50Z','2026-09-27T11:59:45Z',7,NULL,NULL)`)

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
	if !snapshot.JEV.Used || snapshot.JEV.RouteCount != 1 || snapshot.JEV.AcceptedRouteCount != 1 || snapshot.JEV.LatestRouteAt == nil || len(snapshot.JEVRouteEvents) != 1 {
		t.Fatalf("JEV actual-use projection = %#v events=%#v", snapshot.JEV, snapshot.JEVRouteEvents)
	}
	route := snapshot.JEVRouteEvents[0]
	if route.Status != "ok" || route.Tools.CandidateCount != 2 || route.Tools.SelectedOrdinal == nil || *route.Tools.SelectedOrdinal != 1 || route.Tools.ConfidenceMilli == nil || *route.Tools.ConfidenceMilli != 875 {
		t.Fatalf("route event = %#v", route)
	}
	if len(snapshot.AgentRuntimes) != 1 || snapshot.AgentRuntimes[0].Status != "CONNECTED" || snapshot.AgentRuntimes[0].ClientFamily != "claude-code" {
		t.Fatalf("agent runtimes = %#v", snapshot.AgentRuntimes)
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

func TestCollectorProjectsLegacyClientMetadataBeforePublicJSON(t *testing.T) {
	db := dashboardDB(t)
	markers := []string{"legacy-name-keyring-marker", "legacy-title-typesafe-marker", "legacy-version-secret-marker"}
	t.Setenv("TYPESAFE_API_KEY", markers[0])
	mustExec(t, db, `INSERT INTO agent_runtimes VALUES ('RUN-LEGACY','PRJ-1','WS-1','TSK-1','SES-1',?,?,?,
		'2026-09-28T11:00:00Z','2026-09-28T11:00:00Z','2026-09-28T11:00:00Z',1,NULL,NULL)`, markers[0], markers[1], markers[2])
	collector, err := NewCollector(CollectorOptions{DB: db, ProjectID: "PRJ-1", Workspace: workspace.Workspace{ID: "WS-1", ProjectID: "PRJ-1"}, WorktreeRoot: "/repo"})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := collector.Snapshot(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if hits := markerCount(string(payload), markers); hits != 0 {
		t.Fatalf("public snapshot contains %d/3 legacy markers: %s", hits, payload)
	}
	if strings.Contains(string(payload), `"client_title"`) || strings.Contains(string(payload), `"client_version"`) {
		t.Fatalf("retired raw client keys remain in public JSON: %s", payload)
	}
	if len(snapshot.AgentRuntimes) != 1 || snapshot.AgentRuntimes[0].ID != "RUN-LEGACY" || snapshot.AgentRuntimes[0].ClientFamily != "unknown-client" {
		t.Fatalf("projected runtime = %#v", snapshot.AgentRuntimes)
	}
	var persisted string
	if err := db.QueryRow(`SELECT client_name || '|' || client_title || '|' || client_version FROM agent_runtimes WHERE runtime_id='RUN-LEGACY'`).Scan(&persisted); err != nil {
		t.Fatal(err)
	}
	if hits := markerCount(persisted, markers); hits != 3 {
		t.Fatalf("legacy row was rewritten: %d/3 markers", hits)
	}
}

func TestCollectorProjectsRuntimeTelemetryAndContinuityTruthfully(t *testing.T) {
	db := dashboardDB(t)
	mustExec(t, db, `INSERT INTO sessions VALUES ('SES-1','WS-1','worker','2026-09-28T11:00:00Z')`)
	mustExec(t, db, `INSERT INTO tasks VALUES ('TSK-1','PRJ-1','task','IN_PROGRESS',NULL,'SES-1','SES-1','2026-09-28T11:00:00Z','2026-09-28T11:00:00Z',1)`)
	mustExec(t, db, `INSERT INTO agent_runtimes VALUES ('RUN-1','PRJ-1','WS-1','TSK-1','SES-1','codex',NULL,NULL,
		'2026-09-28T11:00:00Z','2026-09-28T11:00:20Z','2026-09-28T11:00:20Z',2,NULL,NULL)`)
	mustExec(t, db, `INSERT INTO agent_runtime_observations VALUES
		('RUN-1','gpt-6-astra','high',600000,1000000,'host_adapter','structured_host','2026-09-28T11:00:21Z',3)`)
	mustExec(t, db, `INSERT INTO continuity_intents VALUES
		('CTI-1','PRJ-1','WS-1','TSK-1','SES-1',zeroblob(32),'SAME_TASK',NULL,'SPAWN_READY',4,3,6000,2,
		'CHK-1','HOST-1',zeroblob(32),NULL,NULL,NULL,'2026-09-28T11:00:00Z','2026-09-28T11:00:21Z','2026-09-28T12:00:00Z')`)
	collector, err := NewCollector(CollectorOptions{DB: db, ProjectID: "PRJ-1", Workspace: workspace.Workspace{ID: "WS-1", ProjectID: "PRJ-1"}, WorktreeRoot: "/repo", Now: func() time.Time { return time.Date(2026, 9, 28, 11, 0, 25, 0, time.UTC) }})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := collector.Snapshot(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	runtime := snapshot.AgentRuntimes[0]
	if runtime.Telemetry.State != "REPORTED" || runtime.Telemetry.Model != "gpt-6-astra" || runtime.Telemetry.UsedPercent == nil || *runtime.Telemetry.UsedPercent != 60 {
		t.Fatalf("telemetry = %#v", runtime.Telemetry)
	}
	if runtime.Continuity.State != "SPAWN_READY" || runtime.Continuity.IntentID != "CTI-1" || runtime.Continuity.CheckpointID != "CHK-1" {
		t.Fatalf("continuity = %#v", runtime.Continuity)
	}
}

func TestCollectorMarksTelemetryStaleIndependentlyOfConnection(t *testing.T) {
	db := dashboardDB(t)
	mustExec(t, db, `INSERT INTO agent_runtimes VALUES ('RUN-1','PRJ-1','WS-1','TSK-1','SES-1','codex',NULL,NULL,
		'2026-09-28T11:00:00Z','2026-09-28T11:01:00Z','2026-09-28T11:01:00Z',2,NULL,NULL)`)
	mustExec(t, db, `INSERT INTO agent_runtime_observations VALUES
		('RUN-1',NULL,NULL,NULL,NULL,'mcp_meta','self_reported','2026-09-28T11:00:00Z',1)`)
	collector, _ := NewCollector(CollectorOptions{DB: db, ProjectID: "PRJ-1", Workspace: workspace.Workspace{ID: "WS-1", ProjectID: "PRJ-1"}, WorktreeRoot: "/repo", Now: func() time.Time { return time.Date(2026, 9, 28, 11, 1, 1, 0, time.UTC) }})
	snapshot, err := collector.Snapshot(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.AgentRuntimes[0].Status != "CONNECTED" || snapshot.AgentRuntimes[0].Telemetry.State != "STALE" {
		t.Fatalf("runtime = %#v", snapshot.AgentRuntimes[0])
	}
}

func markerCount(value string, markers []string) int {
	hits := 0
	for _, marker := range markers {
		if strings.Contains(value, marker) {
			hits++
		}
	}
	return hits
}

func TestCollectorDerivesTruthfulRuntimePresenceAtExactBoundaries(t *testing.T) {
	db := dashboardDB(t)
	now := time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)
	mustExec(t, db, `INSERT INTO sessions VALUES ('SES-1','WS-1','worker','2026-09-28T10:00:00Z')`)
	for _, row := range []string{
		`('RUN-CONNECTED','PRJ-1','WS-1','TSK-1','SES-1','claude-code',NULL,NULL,'2026-09-28T11:00:00Z','2026-09-28T11:59:45Z','2026-09-28T11:59:30Z',1,NULL,NULL)`,
		`('RUN-IDLE','PRJ-1','WS-1','TSK-1','SES-1','codex',NULL,NULL,'2026-09-28T11:00:00Z','2026-09-28T11:59:45Z','2026-09-28T11:59:29.999Z',2,NULL,NULL)`,
		`('RUN-STALE','PRJ-1','WS-1','TSK-1','SES-1','other',NULL,NULL,'2026-09-28T11:00:00Z','2026-09-28T11:59:44.999Z','2026-09-28T11:59:59Z',3,NULL,NULL)`,
		`('RUN-ENDED','PRJ-1','WS-1','TSK-1','SES-1','other',NULL,NULL,'2026-09-28T11:00:00Z','2026-09-28T11:59:59Z','2026-09-28T11:59:59Z',4,'2026-09-28T11:20:00Z','disconnected')`,
	} {
		mustExec(t, db, `INSERT INTO agent_runtimes VALUES `+row)
	}
	collector, err := NewCollector(CollectorOptions{DB: db, ProjectID: "PRJ-1", Workspace: workspace.Workspace{ID: "WS-1", ProjectID: "PRJ-1"}, WorktreeRoot: "/repo", Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := collector.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]AgentRuntime{}
	for _, runtime := range snapshot.AgentRuntimes {
		got[runtime.ID] = runtime
	}
	for id, want := range map[string]string{"RUN-CONNECTED": "CONNECTED", "RUN-IDLE": "IDLE", "RUN-STALE": "STALE", "RUN-ENDED": "ENDED"} {
		if got[id].Status != want {
			t.Errorf("%s status = %q, want %q", id, got[id].Status, want)
		}
	}
}

func TestCollectorBoundsRouteAndRuntimeTelemetry(t *testing.T) {
	db := dashboardDB(t)
	for i := 0; i < maxJEVRoutes+3; i++ {
		mustExec(t, db, `INSERT INTO jev_route_events VALUES (?, 'PRJ-1','WS-1','TSK-1','SES-1','typesafe','jev-latest',1,'fallback','no_match','none','2026-09-28T11:00:00Z','2026-09-28T11:00:00.001Z',1,1,NULL,NULL,0,NULL,NULL,0,NULL,NULL,0,NULL,NULL)`, fmt.Sprintf("RTE-%03d", i))
	}
	for i := 0; i < maxAgentRuntimes+3; i++ {
		mustExec(t, db, `INSERT INTO agent_runtimes VALUES (?, 'PRJ-1','WS-1','TSK-1','SES-1','client',NULL,NULL,'2026-09-28T11:00:00Z','2026-09-28T11:00:00Z','2026-09-28T11:00:00Z',1,'2026-09-28T11:01:00Z','shutdown')`, fmt.Sprintf("RUN-%03d", i))
	}
	collector, err := NewCollector(CollectorOptions{DB: db, ProjectID: "PRJ-1", Workspace: workspace.Workspace{ID: "WS-1", ProjectID: "PRJ-1"}, WorktreeRoot: "/repo"})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := collector.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.JEVRouteEvents) != maxJEVRoutes || snapshot.JEV.RouteCount != maxJEVRoutes+3 || !snapshot.Truncated["jev_route_events"] {
		t.Fatalf("route bound: details=%d state=%#v truncated=%v", len(snapshot.JEVRouteEvents), snapshot.JEV, snapshot.Truncated)
	}
	if snapshot.JEV.Used || snapshot.JEV.AcceptedRouteCount != 0 || snapshot.JEV.LatestRouteAt != nil {
		t.Fatalf("fallback-only routes were presented as accepted advice: %#v", snapshot.JEV)
	}
	if len(snapshot.AgentRuntimes) != maxAgentRuntimes || !snapshot.Truncated["agent_runtimes"] {
		t.Fatalf("runtime bound: details=%d truncated=%v", len(snapshot.AgentRuntimes), snapshot.Truncated)
	}
}

func TestCollectorBoundsFreeTextWithoutLeakingSecretPrefixes(t *testing.T) {
	db := dashboardDB(t)
	secret := strings.Repeat("s", 80)
	prefix := strings.Repeat("x", maxTextRunes-20)
	mustExec(t, db, `INSERT INTO sessions VALUES ('SES-LONG','WS-1',?, '2026-09-27T10:00:00Z')`, prefix+secret)
	mustExec(t, db, `INSERT INTO tasks VALUES ('TSK-LONG','PRJ-1',?,'OPEN',?,'SES-LONG',NULL,'2026-09-27T10:00:00Z','2026-09-27T11:00:00Z',1)`, prefix+secret, prefix+secret)
	mustExec(t, db, `INSERT INTO leases VALUES ('LSE-LONG','PRJ-1','task','TSK-LONG','SES-LONG','2026-09-27T10:00:00Z','2026-09-27T10:00:00Z','2026-09-27T12:20:00Z','2026-09-27T12:00:00Z',?)`, prefix+secret)
	mustExec(t, db, `INSERT INTO agent_runtimes VALUES ('RUN-LONG','PRJ-1','WS-1','TSK-LONG','SES-LONG',?,NULL,NULL,'2026-09-27T10:00:00Z','2026-09-27T10:00:00Z','2026-09-27T10:00:00Z',1,NULL,NULL)`, strings.Repeat("x", maxIDRunes)+secret)
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
		`CREATE TABLE jev_route_events(route_id TEXT, project_id TEXT, workspace_id TEXT, task_id TEXT, session_id TEXT, provider TEXT, model TEXT, version INTEGER, status TEXT, reason TEXT, credential_source TEXT, started_at TEXT, ended_at TEXT, duration_ms INTEGER, tool_candidate_count INTEGER, tool_selected_ordinal INTEGER, tool_confidence_milli INTEGER, agent_candidate_count INTEGER, agent_selected_ordinal INTEGER, agent_confidence_milli INTEGER, model_candidate_count INTEGER, model_selected_ordinal INTEGER, model_confidence_milli INTEGER, effort_candidate_count INTEGER, effort_selected_ordinal INTEGER, effort_confidence_milli INTEGER)`,
		`CREATE TABLE agent_runtimes(runtime_id TEXT, project_id TEXT, workspace_id TEXT, task_id TEXT, session_id TEXT, client_name TEXT, client_title TEXT, client_version TEXT, started_at TEXT, last_heartbeat_at TEXT, last_activity_at TEXT, sequence INTEGER, ended_at TEXT, end_reason TEXT)`,
		`CREATE TABLE agent_runtime_observations(runtime_id TEXT, model_key TEXT, effort TEXT, context_used INTEGER, context_limit INTEGER, source TEXT, confidence TEXT, observed_at TEXT, revision INTEGER)`,
		`CREATE TABLE continuity_intents(intent_id TEXT, project_id TEXT, workspace_id TEXT, task_id TEXT, predecessor_session_id TEXT, predecessor_run_hash BLOB, kind TEXT, target_task_id TEXT, state TEXT, revision INTEGER, last_observation_sequence INTEGER, last_used_basis_points INTEGER, consecutive_handoff_observations INTEGER, checkpoint_id TEXT, host_operation_id TEXT, takeover_token_hash BLOB, successor_run_hash BLOB, successor_session_id TEXT, failure_code TEXT, created_at TEXT, updated_at TEXT, expires_at TEXT)`,
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
