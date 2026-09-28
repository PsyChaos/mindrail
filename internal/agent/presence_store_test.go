package agent_test

import (
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/PsyChaos/mindrail/internal/agent"
)

func TestPresenceStoreProjectsLegacyClientMetadataOnEveryReadWithoutRewriting(t *testing.T) {
	fx := newAgentStoreFixture(t)
	markers := []string{"legacy-name-keyring-marker", "legacy-title-typesafe-marker", "legacy-version-secret-marker"}
	t.Setenv("TYPESAFE_API_KEY", markers[0])
	if _, err := fx.db.ExecContext(t.Context(), `INSERT INTO agent_runtimes (
		runtime_id, project_id, workspace_id, task_id, session_id, client_name, client_title, client_version,
		started_at, last_heartbeat_at, last_activity_at, sequence, ended_at, end_reason)
		VALUES ('RUN-LEGACY', 'PRJ-1', 'WSP-1', 'TSK-1', 'SES-1', ?, ?, ?,
		'2026-09-28T08:00:00Z', '2026-09-28T08:00:00Z', '2026-09-28T08:00:00Z', 1, NULL, NULL)`,
		markers[0], markers[1], markers[2]); err != nil {
		t.Fatal(err)
	}
	store, err := agent.NewPresenceStore(fx.db.DB, &mutableClock{now: time.Date(2026, 9, 28, 8, 1, 0, 0, time.UTC)})
	if err != nil {
		t.Fatal(err)
	}
	got, err := store.Get(t.Context(), "RUN-LEGACY")
	if err != nil {
		t.Fatal(err)
	}
	listed, err := store.ListProject(t.Context(), "PRJ-1", 10)
	if err != nil {
		t.Fatal(err)
	}
	for label, value := range map[string]any{"get": got, "list": listed} {
		payload, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if hits := markerHits(string(payload), markers); hits != 0 {
			t.Fatalf("%s projected payload contains %d/3 legacy markers: %s", label, hits, payload)
		}
	}
	var persisted string
	if err := fx.db.QueryRowContext(t.Context(), `SELECT client_name || '|' || client_title || '|' || client_version FROM agent_runtimes WHERE runtime_id='RUN-LEGACY'`).Scan(&persisted); err != nil {
		t.Fatal(err)
	}
	if hits := markerHits(persisted, markers); hits != 3 {
		t.Fatalf("legacy row was rewritten: got %d/3 markers in %q", hits, persisted)
	}
}

func markerHits(value string, markers []string) int {
	hits := 0
	for _, marker := range markers {
		if strings.Contains(value, marker) {
			hits++
		}
	}
	return hits
}

type mutableClock struct {
	mu  sync.Mutex
	now time.Time
}

func TestPresenceSequenceIsAtomicAndEndedRuntimeCannotReturnToLife(t *testing.T) {
	fx := newAgentStoreFixture(t)
	started := time.Date(2026, 9, 28, 8, 3, 0, 0, time.UTC)
	clock := &mutableClock{now: started}
	store, err := agent.NewPresenceStore(fx.db.DB, clock)
	if err != nil {
		t.Fatal(err)
	}
	runtime, err := store.Start(t.Context(), agent.PresenceStart{
		ProjectID: "PRJ-1", WorkspaceID: "WSP-1", TaskID: "TSK-1", SessionID: "SES-1",
		Client: agent.ClientInfo{Name: "codex", Version: "1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	clock.set(started.Add(5 * time.Second))
	const updates = 16
	errs := make(chan error, updates)
	var wg sync.WaitGroup
	for range updates {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := store.Heartbeat(t.Context(), runtime.ID)
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent heartbeat: %v", err)
		}
	}
	runtime, err = store.Get(t.Context(), runtime.ID)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.Sequence != 1+updates {
		t.Fatalf("sequence = %d, want %d", runtime.Sequence, 1+updates)
	}
	clock.set(started.Add(6 * time.Second))
	if _, err := store.End(t.Context(), runtime.ID, agent.RuntimeEndDisconnected); err != nil {
		t.Fatal(err)
	}
	clock.set(started.Add(7 * time.Second))
	if _, err := store.Heartbeat(t.Context(), runtime.ID); !errors.Is(err, agent.ErrRuntimeEnded) {
		t.Fatalf("heartbeat after end = %v, want ErrRuntimeEnded", err)
	}
}

func TestPresenceStoreProjectsUnsafeClientMetadataAndRejectsMismatchedAttribution(t *testing.T) {
	fx := newAgentStoreFixture(t)
	clock := &mutableClock{now: time.Date(2026, 9, 28, 8, 4, 0, 0, time.UTC)}
	store, err := agent.NewPresenceStore(fx.db.DB, clock)
	if err != nil {
		t.Fatal(err)
	}
	base := agent.PresenceStart{
		ProjectID: "PRJ-1", WorkspaceID: "WSP-1", TaskID: "TSK-1", SessionID: "SES-1",
		Client: agent.ClientInfo{Name: "codex", Title: "Codex", Version: "1"},
	}
	bad := base
	bad.Client.Title = "unsafe\nheader"
	runtime, err := store.Start(t.Context(), bad)
	if err != nil {
		t.Fatalf("unsafe optional client metadata should not reject presence: %v", err)
	}
	if runtime.Client != (agent.ClientInfo{Name: "codex", Title: "Codex"}) {
		t.Fatalf("projected client = %+v", runtime.Client)
	}
	bad = base
	bad.SessionID = "SES-MISSING"
	if _, err := store.Start(t.Context(), bad); err == nil {
		t.Fatal("mismatched attribution was accepted")
	}
	if _, err := fx.db.ExecContext(t.Context(), `INSERT INTO agent_runtimes (
		runtime_id, project_id, workspace_id, task_id, session_id, client_name,
		started_at, last_heartbeat_at, last_activity_at, sequence, ended_at, end_reason)
		VALUES ('RUN-RAW', 'PRJ-1', 'WSP-1', 'TSK-1', 'SES-1', 'codex',
		'2026-09-28T08:00:00Z', '2026-09-28T08:00:00Z', '2026-09-28T08:00:00Z', 1,
		NULL, 'completed')`); err == nil {
		t.Fatal("schema accepted an end reason without an end timestamp")
	}
}

func TestPresenceStoreNeverPersistsCredentialsOrCallerControlledFreeText(t *testing.T) {
	tests := []struct {
		name       string
		credential string
		client     agent.ClientInfo
		want       agent.ClientInfo
		forbidden  []string
	}{
		{
			name: "configured credential supplied as name", credential: "codex",
			client: agent.ClientInfo{Name: "codex", Title: "Codex", Version: "1.2.3"},
			want:   agent.ClientInfo{Name: "unknown-client"}, forbidden: []string{"codex", "1.2.3"},
		},
		{
			name: "configured credential supplied as title", credential: "jev-live-secret-value",
			client: agent.ClientInfo{Name: "claude-code", Title: "jev-live-secret-value", Version: "2.1.0"},
			want:   agent.ClientInfo{Name: "claude-code", Title: "Claude Code"}, forbidden: []string{"jev-live-secret-value", "2.1.0"},
		},
		{
			name: "configured credential supplied as version", credential: "1.2.3",
			client: agent.ClientInfo{Name: "cursor", Title: "Cursor", Version: "1.2.3"},
			want:   agent.ClientInfo{Name: "cursor", Title: "Cursor"}, forbidden: []string{"1.2.3"},
		},
		{
			name: "common credential shapes", client: agent.ClientInfo{
				Name: "sk-proj-abcdefghijklmnopqrstuvwxyz", Title: "ghp_abcdefghijklmnopqrstuvwxyz", Version: "xoxb-123-secret",
			},
			want:      agent.ClientInfo{Name: "unknown-client"},
			forbidden: []string{"sk-proj-abcdefghijklmnopqrstuvwxyz", "ghp_abcdefghijklmnopqrstuvwxyz", "xoxb-123-secret"},
		},
		{
			name: "high entropy unknown client", client: agent.ClientInfo{
				Name: "QWxhZGRpbjpvcGVuIHNlc2FtZV9yYW5kb21fNjRfYnl0ZXM", Title: "private build at /home/person/project", Version: "dev-secret-9f0a8b7c6d5e4f3a",
			},
			want:      agent.ClientInfo{Name: "unknown-client"},
			forbidden: []string{"QWxhZGRpbjpvcGVuIHNlc2FtZV9yYW5kb21fNjRfYnl0ZXM", "/home/person/project", "dev-secret"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("TYPESAFE_API_KEY", tt.credential)
			fx := newAgentStoreFixture(t)
			clock := &mutableClock{now: time.Date(2026, 9, 28, 8, 5, 0, 0, time.UTC)}
			store, err := agent.NewPresenceStore(fx.db.DB, clock)
			if err != nil {
				t.Fatal(err)
			}
			runtime, err := store.Start(t.Context(), agent.PresenceStart{
				ProjectID: "PRJ-1", WorkspaceID: "WSP-1", TaskID: "TSK-1", SessionID: "SES-1", Client: tt.client,
			})
			if err != nil {
				t.Fatal(err)
			}
			if runtime.Client != tt.want {
				t.Fatalf("projected client = %+v, want %+v", runtime.Client, tt.want)
			}
			var name string
			var title, version *string
			if err := fx.db.QueryRowContext(t.Context(), `SELECT client_name, client_title, client_version
				FROM agent_runtimes WHERE runtime_id = ?`, runtime.ID).Scan(&name, &title, &version); err != nil {
				t.Fatal(err)
			}
			persisted := name
			if title != nil {
				persisted += " " + *title
			}
			if version != nil {
				persisted += " " + *version
			}
			for _, forbidden := range tt.forbidden {
				if strings.Contains(persisted, forbidden) {
					t.Fatalf("persisted client metadata %q contains raw sensitive input %q", persisted, forbidden)
				}
			}
		})
	}
}

func TestPresenceStorePreservesOnlyKnownCanonicalClientIdentity(t *testing.T) {
	tests := []struct {
		in   agent.ClientInfo
		want agent.ClientInfo
	}{
		{agent.ClientInfo{Name: "openai-codex", Title: "arbitrary", Version: "v1.2.3-beta.1"}, agent.ClientInfo{Name: "codex", Title: "Codex"}},
		{agent.ClientInfo{Name: "claude", Title: "arbitrary", Version: "2.0"}, agent.ClientInfo{Name: "claude-code", Title: "Claude Code"}},
		{agent.ClientInfo{Name: "mcp-inspector", Title: "arbitrary", Version: "0.16.0"}, agent.ClientInfo{Name: "mcp-inspector", Title: "MCP Inspector"}},
		{agent.ClientInfo{Name: "my-private-agent", Title: "arbitrary", Version: "2026.9"}, agent.ClientInfo{Name: "unknown-client"}},
	}
	for _, tt := range tests {
		t.Run(tt.in.Name, func(t *testing.T) {
			t.Setenv("TYPESAFE_API_KEY", "")
			fx := newAgentStoreFixture(t)
			store, err := agent.NewPresenceStore(fx.db.DB, &mutableClock{now: time.Date(2026, 9, 28, 8, 6, 0, 0, time.UTC)})
			if err != nil {
				t.Fatal(err)
			}
			runtime, err := store.Start(t.Context(), agent.PresenceStart{
				ProjectID: "PRJ-1", WorkspaceID: "WSP-1", TaskID: "TSK-1", SessionID: "SES-1", Client: tt.in,
			})
			if err != nil {
				t.Fatal(err)
			}
			if runtime.Client != tt.want {
				t.Fatalf("projected client = %+v, want %+v", runtime.Client, tt.want)
			}
		})
	}
}

func (c *mutableClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *mutableClock) set(now time.Time) {
	c.mu.Lock()
	c.now = now
	c.mu.Unlock()
}

func TestPresenceStoreTracksConnectionActivityAndFrozenEndDuration(t *testing.T) {
	fx := newAgentStoreFixture(t)
	started := time.Date(2026, 9, 28, 8, 2, 0, 0, time.UTC)
	clock := &mutableClock{now: started}
	store, err := agent.NewPresenceStore(fx.db.DB, clock)
	if err != nil {
		t.Fatal(err)
	}
	runtime, err := store.Start(t.Context(), agent.PresenceStart{
		ProjectID: "PRJ-1", WorkspaceID: "WSP-1", TaskID: "TSK-1", SessionID: "SES-1",
		Client: agent.ClientInfo{Name: "claude-code", Title: "Claude Code", Version: "1.2.3"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if runtime.ID == "" || runtime.Sequence != 1 || !runtime.StartedAt.Equal(started) || runtime.EndedAt != nil {
		t.Fatalf("started runtime = %+v", runtime)
	}

	clock.set(started.Add(5 * time.Second))
	runtime, err = store.Heartbeat(t.Context(), runtime.ID)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.Sequence != 2 || !runtime.LastHeartbeatAt.Equal(clock.Now()) || !runtime.LastActivityAt.Equal(started) {
		t.Fatalf("heartbeat runtime = %+v", runtime)
	}

	clock.set(started.Add(7 * time.Second))
	runtime, err = store.Activity(t.Context(), runtime.ID)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.Sequence != 3 || !runtime.LastActivityAt.Equal(clock.Now()) || !runtime.LastHeartbeatAt.Equal(clock.Now()) {
		t.Fatalf("activity runtime = %+v", runtime)
	}

	clock.set(started.Add(11 * time.Second))
	runtime, err = store.End(t.Context(), runtime.ID, agent.RuntimeEndCompleted)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.Sequence != 4 || runtime.EndedAt == nil || !runtime.EndedAt.Equal(clock.Now()) || runtime.EndReason != agent.RuntimeEndCompleted {
		t.Fatalf("ended runtime = %+v", runtime)
	}

	clock.set(started.Add(time.Hour))
	again, err := store.End(t.Context(), runtime.ID, agent.RuntimeEndDisconnected)
	if err != nil {
		t.Fatal(err)
	}
	if again.Sequence != 4 || again.EndReason != agent.RuntimeEndCompleted || !again.EndedAt.Equal(started.Add(11*time.Second)) {
		t.Fatalf("second end changed terminal state: %+v", again)
	}
	listed, err := store.ListProject(t.Context(), "PRJ-1", 10)
	if err != nil || len(listed) != 1 || listed[0].ID != runtime.ID {
		t.Fatalf("listed = %+v, err = %v", listed, err)
	}
}
