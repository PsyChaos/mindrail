package agent_test

import (
	"database/sql"
	"testing"
	"time"

	"github.com/PsyChaos/mindrail/internal/agent"
	"github.com/PsyChaos/mindrail/internal/app"
)

func TestRuntimeObservationStoreRecordsBoundedLatestTelemetry(t *testing.T) {
	fx := newAgentStoreFixture(t)
	clock := app.FixedClock{Instant: time.Date(2026, 9, 28, 8, 5, 0, 0, time.UTC)}
	presence, _ := agent.NewPresenceStore(fx.db.DB, clock)
	runtime, err := presence.Start(t.Context(), agent.PresenceStart{
		ProjectID: "PRJ-1", WorkspaceID: "WSP-1", TaskID: "TSK-1", SessionID: "SES-1",
		Client: agent.ClientInfo{Name: "codex"},
	})
	if err != nil {
		t.Fatal(err)
	}
	store, err := agent.NewRuntimeObservationStore(fx.db.DB, clock)
	if err != nil {
		t.Fatal(err)
	}
	used, limit := int64(600_000), int64(1_000_000)
	observation, err := store.Observe(t.Context(), runtime.ID, agent.RuntimeObservationInput{
		ModelKey: "GPT-6-ASTRA", Effort: "high", ContextUsed: &used, ContextLimit: &limit,
		Source: agent.ObservationSourceHost, Confidence: agent.ObservationStructuredHost,
	})
	if err != nil {
		t.Fatal(err)
	}
	if observation.ModelKey != "gpt-6-astra" || observation.Effort != "high" || observation.Revision != 1 || *observation.ContextUsed != used {
		t.Fatalf("observation = %#v", observation)
	}
	used = 500_000 // compaction may legitimately reduce usage
	observation, err = store.Observe(t.Context(), runtime.ID, agent.RuntimeObservationInput{
		ModelKey: "untrusted-arbitrary-model-value", ContextUsed: &used, ContextLimit: &limit,
		Source: agent.ObservationSourceMCPMeta, Confidence: agent.ObservationSelfReported,
	})
	if err != nil {
		t.Fatal(err)
	}
	if observation.ModelKey != "" || observation.Revision != 2 || *observation.ContextUsed != used {
		t.Fatalf("updated observation = %#v", observation)
	}
}

func TestRuntimeObservationStoreRejectsPartialCountersAndEndedRuntime(t *testing.T) {
	fx := newAgentStoreFixture(t)
	clock := app.FixedClock{Instant: time.Date(2026, 9, 28, 8, 5, 0, 0, time.UTC)}
	presence, _ := agent.NewPresenceStore(fx.db.DB, clock)
	runtime, err := presence.Start(t.Context(), agent.PresenceStart{
		ProjectID: "PRJ-1", WorkspaceID: "WSP-1", TaskID: "TSK-1", SessionID: "SES-1",
		Client: agent.ClientInfo{Name: "claude-code"},
	})
	if err != nil {
		t.Fatal(err)
	}
	store, _ := agent.NewRuntimeObservationStore(fx.db.DB, clock)
	used := int64(10)
	if _, err := store.Observe(t.Context(), runtime.ID, agent.RuntimeObservationInput{
		ContextUsed: &used, Source: agent.ObservationSourceHost, Confidence: agent.ObservationStructuredHost,
	}); err == nil {
		t.Fatal("partial context counters were accepted")
	}
	if _, err := presence.End(t.Context(), runtime.ID, agent.RuntimeEndCompleted); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Observe(t.Context(), runtime.ID, agent.RuntimeObservationInput{
		Source: agent.ObservationSourceHost, Confidence: agent.ObservationStructuredHost,
	}); err == nil {
		t.Fatal("ended runtime accepted telemetry")
	}
	if _, err := store.Get(t.Context(), "RUN-MISSING"); err == nil || !errorsIsNoRows(err) {
		t.Fatalf("missing runtime error = %v", err)
	}
}

func TestRuntimeObservationStoreAcceptsCurrentCodexEfforts(t *testing.T) {
	fx := newAgentStoreFixture(t)
	clock := app.FixedClock{Instant: time.Date(2026, 9, 28, 8, 5, 0, 0, time.UTC)}
	presence, _ := agent.NewPresenceStore(fx.db.DB, clock)
	runtime, err := presence.Start(t.Context(), agent.PresenceStart{ProjectID: "PRJ-1", WorkspaceID: "WSP-1", TaskID: "TSK-1", SessionID: "SES-1", Client: agent.ClientInfo{Name: "codex"}})
	if err != nil {
		t.Fatal(err)
	}
	store, _ := agent.NewRuntimeObservationStore(fx.db.DB, clock)
	for _, effort := range []string{"xhigh", "max", "ultra"} {
		observation, err := store.Observe(t.Context(), runtime.ID, agent.RuntimeObservationInput{ModelKey: "gpt-5.6-sol", Effort: effort, Source: agent.ObservationSourceHost, Confidence: agent.ObservationStructuredHost})
		if err != nil || observation.Effort != effort || observation.ModelKey != "gpt-5.6-sol" {
			t.Fatalf("effort %s: %#v %v", effort, observation, err)
		}
	}
}

func errorsIsNoRows(err error) bool { return err == sql.ErrNoRows }
