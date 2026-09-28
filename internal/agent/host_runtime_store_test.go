package agent_test

import (
	"crypto/sha256"
	"testing"
	"time"

	"github.com/PsyChaos/mindrail/internal/agent"
	"github.com/PsyChaos/mindrail/internal/app"
)

func TestHostRuntimeStoreHandlesSubagentFirstAndBackfillsParent(t *testing.T) {
	fx := newAgentStoreFixture(t)
	now := time.Date(2026, 9, 28, 8, 5, 0, 0, time.UTC)
	store, err := agent.NewHostRuntimeStore(fx.db.DB, app.FixedClock{Instant: now})
	if err != nil {
		t.Fatal(err)
	}
	attr := agent.HostRuntimeAttribution{ProjectID: "PRJ-1", WorkspaceID: "WSP-1", TaskID: "TSK-1", SessionID: "SES-1"}
	child, err := store.Observe(t.Context(), attr, agent.HostRuntimeEventInput{
		Host: agent.HostClaudeCode, HostSessionID: "host-session", AgentID: "child-1", Generation: "generation-1",
		AgentType: "Explore", Event: agent.HostEventStart, Sequence: 1, ObservedAt: now,
	})
	if err != nil || !child.Accepted || child.IsMain || child.ParentRuntimeID != "" {
		t.Fatalf("child=%#v err=%v", child, err)
	}
	parent, err := store.Observe(t.Context(), attr, agent.HostRuntimeEventInput{
		Host: agent.HostClaudeCode, HostSessionID: "host-session", Generation: "generation-1",
		Event: agent.HostEventStart, Sequence: 1, ObservedAt: now,
	})
	if err != nil || !parent.IsMain {
		t.Fatalf("parent=%#v err=%v", parent, err)
	}
	var parentID string
	if err := fx.db.QueryRowContext(t.Context(), `SELECT parent_runtime_id FROM host_runtime_bindings WHERE runtime_id=?`, child.RuntimeID).Scan(&parentID); err != nil {
		t.Fatal(err)
	}
	if parentID != parent.RuntimeID {
		t.Fatalf("child parent=%q want %q", parentID, parent.RuntimeID)
	}
}

func TestHostRuntimeStoreRejectsReplayRevivalAndChildContext(t *testing.T) {
	fx := newAgentStoreFixture(t)
	now := time.Date(2026, 9, 28, 8, 5, 0, 0, time.UTC)
	store, _ := agent.NewHostRuntimeStore(fx.db.DB, app.FixedClock{Instant: now})
	attr := agent.HostRuntimeAttribution{ProjectID: "PRJ-1", WorkspaceID: "WSP-1", TaskID: "TSK-1", SessionID: "SES-1"}
	base := agent.HostRuntimeEventInput{Host: agent.HostCodex, HostSessionID: "thread-1", Generation: "turn-1",
		Event: agent.HostEventStart, Sequence: 1, ObservedAt: now}
	started, err := store.Observe(t.Context(), attr, base)
	if err != nil {
		t.Fatal(err)
	}
	replay, err := store.Observe(t.Context(), attr, base)
	if err != nil || replay.Accepted || replay.RuntimeID != started.RuntimeID {
		t.Fatalf("replay=%#v err=%v", replay, err)
	}
	stop := base
	stop.Event, stop.Sequence = agent.HostEventStop, 2
	if _, err := store.Observe(t.Context(), attr, stop); err != nil {
		t.Fatal(err)
	}
	stop.Event, stop.Sequence = agent.HostEventActivity, 3
	if _, err := store.Observe(t.Context(), attr, stop); err == nil {
		t.Fatal("ended generation was revived")
	}
	used, limit := int64(1), int64(10)
	child := base
	child.AgentID, child.Generation, child.ContextUsed, child.ContextLimit = "child", "turn-2", &used, &limit
	if _, err := store.Observe(t.Context(), attr, child); err == nil {
		t.Fatal("child context telemetry was accepted")
	}
}

func TestPendingHostRuntimeStoresOnlyDigestsAndCanBeClaimed(t *testing.T) {
	fx := newAgentStoreFixture(t)
	now := time.Date(2026, 9, 28, 8, 5, 0, 0, time.UTC)
	store, _ := agent.NewHostRuntimeStore(fx.db.DB, app.FixedClock{Instant: now})
	in := agent.HostRuntimeEventInput{Host: agent.HostClaudeCode, HostSessionID: "private-session", Generation: "generation-1",
		Event: agent.HostEventStart, Sequence: 1, ObservedAt: now, ModelKey: "claude-sonnet-4-6", Effort: "not_applicable"}
	if _, err := store.ObservePending(t.Context(), in); err != nil {
		t.Fatal(err)
	}
	wantHash := sha256.Sum256([]byte(in.HostSessionID))
	var gotHash []byte
	if err := fx.db.QueryRowContext(t.Context(), `SELECT host_session_hash FROM pending_host_runtime_events`).Scan(&gotHash); err != nil {
		t.Fatal(err)
	}
	if string(gotHash) != string(wantHash[:]) {
		t.Fatal("pending identity was not hashed")
	}
	attr := agent.HostRuntimeAttribution{ProjectID: "PRJ-1", WorkspaceID: "WSP-1", TaskID: "TSK-1", SessionID: "SES-1"}
	bound, err := store.BindPending(t.Context(), attr, agent.HostRuntimeIdentity{Host: in.Host, HostSessionID: in.HostSessionID, Generation: in.Generation})
	if err != nil || bound.RuntimeID == "" || !bound.IsMain {
		t.Fatalf("bound=%#v err=%v", bound, err)
	}
	var pending int
	if err := fx.db.QueryRowContext(t.Context(), `SELECT count(*) FROM pending_host_runtime_events`).Scan(&pending); err != nil || pending != 0 {
		t.Fatalf("pending=%d err=%v", pending, err)
	}
	for _, column := range []string{"host_session_id", "host_agent_id", "generation"} {
		var count int
		if err := fx.db.QueryRowContext(t.Context(), `SELECT count(*) FROM pragma_table_info('host_runtime_bindings') WHERE name=?`, column).Scan(&count); err != nil || count != 0 {
			t.Fatalf("raw identity column %q exists", column)
		}
	}
}

func TestDocumentedLifecycleAssignsSequenceAndClaimsByRunKey(t *testing.T) {
	fx := newAgentStoreFixture(t)
	now := time.Date(2026, 9, 28, 8, 5, 0, 0, time.UTC)
	store, _ := agent.NewHostRuntimeStore(fx.db.DB, app.FixedClock{Instant: now})
	lifecycle := agent.HostLifecycleInput{Host: agent.HostCodex, HostSessionID: "thread-42", Event: agent.HostEventStart,
		Source: "startup", ModelKey: "gpt-6-astra", Effort: "high"}
	if err := store.ObservePendingLifecycle(t.Context(), lifecycle); err != nil {
		t.Fatal(err)
	}
	lifecycle.Event = agent.HostEventActivity
	if err := store.ObservePendingLifecycle(t.Context(), lifecycle); err != nil {
		t.Fatal(err)
	}
	if err := store.AssociatePendingRun(t.Context(), lifecycle.Host, lifecycle.HostSessionID, "", "stable-run-key"); err != nil {
		t.Fatal(err)
	}
	attr := agent.HostRuntimeAttribution{ProjectID: "PRJ-1", WorkspaceID: "WSP-1", TaskID: "TSK-1", SessionID: "SES-1"}
	presence, _ := agent.NewPresenceStore(fx.db.DB, app.FixedClock{Instant: now})
	runtime, err := presence.Start(t.Context(), agent.PresenceStart{ProjectID: attr.ProjectID, WorkspaceID: attr.WorkspaceID,
		TaskID: attr.TaskID, SessionID: attr.SessionID, Client: agent.ClientInfo{Name: "codex"}})
	if err != nil {
		t.Fatal(err)
	}
	claimed, err := store.ClaimPendingRun(t.Context(), attr, "stable-run-key", runtime.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !claimed.Accepted || !claimed.IsMain || claimed.ProducerSequence != 2 || claimed.RuntimeID == "" {
		t.Fatalf("claimed=%#v", claimed)
	}
	if claimed.RuntimeID != runtime.ID {
		t.Fatalf("claim created duplicate runtime %q, want existing %q", claimed.RuntimeID, runtime.ID)
	}
	var model, effort string
	if err := fx.db.QueryRowContext(t.Context(), `SELECT model_key,effort FROM agent_runtime_observations WHERE runtime_id=?`, claimed.RuntimeID).Scan(&model, &effort); err != nil {
		t.Fatal(err)
	}
	if model != "gpt-6-astra" || effort != "high" {
		t.Fatalf("telemetry=%q/%q", model, effort)
	}
}

func TestRunClaimBindsExistingMainAndAllSessionChildren(t *testing.T) {
	fx := newAgentStoreFixture(t)
	now := time.Date(2026, 9, 28, 8, 5, 0, 0, time.UTC)
	clock := app.FixedClock{Instant: now}
	store, _ := agent.NewHostRuntimeStore(fx.db.DB, clock)
	main := agent.HostLifecycleInput{Host: agent.HostClaudeCode, HostSessionID: "session-all", Event: agent.HostEventStart}
	child := agent.HostLifecycleInput{Host: main.Host, HostSessionID: main.HostSessionID, AgentID: "child-1",
		AgentType: "Explore", Event: agent.HostEventStart}
	if err := store.ObservePendingLifecycle(t.Context(), main); err != nil {
		t.Fatal(err)
	}
	if err := store.ObservePendingLifecycle(t.Context(), child); err != nil {
		t.Fatal(err)
	}
	if err := store.AssociatePendingRun(t.Context(), main.Host, main.HostSessionID, "", "claim-all"); err != nil {
		t.Fatal(err)
	}
	attr := agent.HostRuntimeAttribution{ProjectID: "PRJ-1", WorkspaceID: "WSP-1", TaskID: "TSK-1", SessionID: "SES-1"}
	presence, _ := agent.NewPresenceStore(fx.db.DB, clock)
	runtime, err := presence.Start(t.Context(), agent.PresenceStart{ProjectID: attr.ProjectID, WorkspaceID: attr.WorkspaceID,
		TaskID: attr.TaskID, SessionID: attr.SessionID, Client: agent.ClientInfo{Name: "claude-code"}})
	if err != nil {
		t.Fatal(err)
	}
	claimed, err := store.ClaimPendingRun(t.Context(), attr, "claim-all", runtime.ID)
	if err != nil || claimed.RuntimeID != runtime.ID {
		t.Fatalf("claimed=%#v err=%v", claimed, err)
	}
	var bindings, pending, runtimes, linked int
	for query, target := range map[string]*int{
		`SELECT count(*) FROM host_runtime_bindings`:                                         &bindings,
		`SELECT count(*) FROM pending_host_runtime_events`:                                   &pending,
		`SELECT count(*) FROM agent_runtimes`:                                                &runtimes,
		`SELECT count(*) FROM host_runtime_bindings WHERE is_main=0 AND parent_runtime_id=?`: &linked,
	} {
		var err error
		if target == &linked {
			err = fx.db.QueryRowContext(t.Context(), query, runtime.ID).Scan(target)
		} else {
			err = fx.db.QueryRowContext(t.Context(), query).Scan(target)
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	if bindings != 2 || pending != 0 || runtimes != 2 || linked != 1 {
		t.Fatalf("bindings=%d pending=%d runtimes=%d linked=%d", bindings, pending, runtimes, linked)
	}
	child.Event = agent.HostEventStop
	if err := store.ObservePendingLifecycle(t.Context(), child); err != nil {
		t.Fatalf("post-claim child event did not use durable binding: %v", err)
	}
}

func TestExistingHostBindingRejectsDifferentAttribution(t *testing.T) {
	fx := newAgentStoreFixture(t)
	now := time.Date(2026, 9, 28, 8, 5, 0, 0, time.UTC)
	store, _ := agent.NewHostRuntimeStore(fx.db.DB, app.FixedClock{Instant: now})
	attr := agent.HostRuntimeAttribution{ProjectID: "PRJ-1", WorkspaceID: "WSP-1", TaskID: "TSK-1", SessionID: "SES-1"}
	event := agent.HostRuntimeEventInput{Host: agent.HostCodex, HostSessionID: "thread-bound", Generation: "turn-bound",
		Event: agent.HostEventStart, Sequence: 1, ObservedAt: now}
	if _, err := store.Observe(t.Context(), attr, event); err != nil {
		t.Fatal(err)
	}
	event.Event, event.Sequence = agent.HostEventActivity, 2
	wrong := attr
	wrong.TaskID = "TSK-other"
	if _, err := store.Observe(t.Context(), wrong, event); err == nil {
		t.Fatal("existing binding accepted different task attribution")
	}
}

func TestPostClaimStartReplacesBindingAndLaterActivityUsesReplacement(t *testing.T) {
	fx := newAgentStoreFixture(t)
	now := time.Date(2026, 9, 28, 8, 5, 0, 0, time.UTC)
	store, _ := agent.NewHostRuntimeStore(fx.db.DB, app.FixedClock{Instant: now})
	attr := agent.HostRuntimeAttribution{ProjectID: "PRJ-1", WorkspaceID: "WSP-1", TaskID: "TSK-1", SessionID: "SES-1"}
	first, err := store.ObserveLifecycle(t.Context(), attr, agent.HostLifecycleInput{
		Host: agent.HostClaudeCode, HostSessionID: "session-restart", Event: agent.HostEventStart,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.ObservePendingLifecycle(t.Context(), agent.HostLifecycleInput{
		Host: agent.HostClaudeCode, HostSessionID: "session-restart", Event: agent.HostEventStart, Source: "clear",
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.ObservePendingLifecycle(t.Context(), agent.HostLifecycleInput{
		Host: agent.HostClaudeCode, HostSessionID: "session-restart", Event: agent.HostEventActivity,
	}); err != nil {
		t.Fatal(err)
	}
	var activeID string
	if err := fx.db.QueryRowContext(t.Context(), `SELECT runtime_id FROM host_runtime_bindings WHERE lifecycle_state='ACTIVE'`).Scan(&activeID); err != nil {
		t.Fatal(err)
	}
	if activeID == first.RuntimeID {
		t.Fatal("restart activity remained attached to the replaced runtime")
	}
	var oldState string
	if err := fx.db.QueryRowContext(t.Context(), `SELECT lifecycle_state FROM host_runtime_bindings WHERE runtime_id=?`, first.RuntimeID).Scan(&oldState); err != nil || oldState != "ENDED" {
		t.Fatalf("old state=%q err=%v", oldState, err)
	}
}
