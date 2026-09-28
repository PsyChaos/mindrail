package continuity_test

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/continuity"
	"github.com/PsyChaos/mindrail/internal/migration"
	"github.com/PsyChaos/mindrail/internal/storage"
	"github.com/PsyChaos/mindrail/migrations"
)

func TestSQLStoreCreateAndCompareAndSwap(t *testing.T) {
	db, clock := newStoreFixture(t)
	store, err := continuity.NewSQLStore(db.DB, clock)
	if err != nil {
		t.Fatal(err)
	}
	expires := clock.Now().Add(time.Hour)
	created, err := store.Create(t.Context(), continuity.CreateIntent{
		ProjectID: "PRJ-1", WorkspaceID: "WSP-1", TaskID: "TSK-1", PredecessorSessionID: "SES-1",
		PredecessorRunHash: make([]byte, 32), Kind: continuity.IntentSameTask, ExpiresAt: expires,
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.ID == "" || created.State != continuity.StateObserving || created.Revision != 1 || created.LastUsedBasisPoints != -1 {
		t.Fatalf("created = %#v", created)
	}

	next := created
	next.State = continuity.StateWarned
	next.LastObservationSequence = 3
	next.LastUsedBasisPoints = 5600
	saved, err := store.Save(t.Context(), next, created.Revision)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Revision != 2 || saved.State != continuity.StateWarned || saved.LastObservationSequence != 3 {
		t.Fatalf("saved = %#v", saved)
	}
	if _, err := store.Save(t.Context(), next, created.Revision); !errors.Is(err, continuity.ErrConflict) {
		t.Fatalf("stale save error = %v, want conflict", err)
	}
	reverse := saved
	reverse.State = continuity.StateObserving
	if _, err := store.Save(t.Context(), reverse, saved.Revision); err == nil {
		t.Fatal("state reversal was accepted")
	}
}

func TestSQLStoreEnforcesSingleActiveIntentAndAttribution(t *testing.T) {
	db, clock := newStoreFixture(t)
	store, _ := continuity.NewSQLStore(db.DB, clock)
	input := continuity.CreateIntent{
		ProjectID: "PRJ-1", WorkspaceID: "WSP-1", TaskID: "TSK-1", PredecessorSessionID: "SES-1",
		PredecessorRunHash: make([]byte, 32), Kind: continuity.IntentSameTask, ExpiresAt: clock.Now().Add(time.Hour),
	}
	if _, err := store.Create(t.Context(), input); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Create(t.Context(), input); err == nil {
		t.Fatal("second active intent was accepted")
	}
	input.WorkspaceID = "WSP-MISSING"
	input.TaskID = "TSK-2"
	if _, err := store.Create(t.Context(), input); err == nil {
		t.Fatal("mismatched attribution was accepted")
	}
}

func TestSQLStoreRequiresExplicitSameProjectNextTask(t *testing.T) {
	db, clock := newStoreFixture(t)
	if _, err := db.ExecContext(t.Context(), `INSERT INTO tasks
		(task_id, project_id, title, state, blocked_reason, opened_by, claimed_by, created_at, updated_at, revision)
		VALUES ('TSK-2','PRJ-1','next','OPEN',NULL,'SES-1',NULL,'2026-09-28T08:00:00Z','2026-09-28T08:00:00Z',1)`); err != nil {
		t.Fatal(err)
	}
	store, _ := continuity.NewSQLStore(db.DB, clock)
	_, err := store.Create(t.Context(), continuity.CreateIntent{
		ProjectID: "PRJ-1", WorkspaceID: "WSP-1", TaskID: "TSK-1", PredecessorSessionID: "SES-1",
		PredecessorRunHash: make([]byte, 32), Kind: continuity.IntentNextTask, TargetTaskID: "TSK-2",
		ExpiresAt: clock.Now().Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestSQLStoreRecoversActiveIntentAfterProcessRestart(t *testing.T) {
	db, clock := newStoreFixture(t)
	store, _ := continuity.NewSQLStore(db.DB, clock)
	created, err := store.Create(t.Context(), continuity.CreateIntent{ProjectID: "PRJ-1", WorkspaceID: "WSP-1", TaskID: "TSK-1",
		PredecessorSessionID: "SES-1", PredecessorRunHash: make([]byte, 32), Kind: continuity.IntentSameTask, ExpiresAt: clock.Now().Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	reopened, _ := continuity.NewSQLStore(db.DB, clock)
	found, err := reopened.FindActive(t.Context(), "TSK-1", "SES-1")
	if err != nil || found.ID != created.ID {
		t.Fatalf("found=%#v err=%v", found, err)
	}
}

func newStoreFixture(t *testing.T) (*storage.DB, app.FixedClock) {
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
	for _, statement := range []string{
		`INSERT INTO projects (project_id, common_dir, registered_at) VALUES ('PRJ-1','/repo/.git','2026-09-28T08:00:00Z')`,
		`INSERT INTO workspaces (workspace_id, project_id, root_path, git_dir, is_linked_worktree, registered_at, last_seen_at)
		 VALUES ('WSP-1','PRJ-1','/repo','/repo/.git',0,'2026-09-28T08:00:00Z','2026-09-28T08:00:00Z')`,
		`INSERT INTO sessions (session_id, workspace_id, label, started_at) VALUES ('SES-1','WSP-1',NULL,'2026-09-28T08:00:00Z')`,
		`INSERT INTO tasks (task_id, project_id, title, state, blocked_reason, opened_by, claimed_by, created_at, updated_at, revision)
		 VALUES ('TSK-1','PRJ-1','task','IN_PROGRESS',NULL,'SES-1','SES-1','2026-09-28T08:00:00Z','2026-09-28T08:00:00Z',1)`,
	} {
		if _, err := db.ExecContext(t.Context(), statement); err != nil {
			t.Fatal(err)
		}
	}
	return db, clock
}
