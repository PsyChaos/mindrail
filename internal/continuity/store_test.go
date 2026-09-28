package continuity_test

import (
	"crypto/sha256"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/continuity"
	"github.com/PsyChaos/mindrail/internal/coordination"
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

func TestSQLStoreTakeoverTokenIsSingleRunAndReplaySafe(t *testing.T) {
	db, clock := newStoreFixture(t)
	store, _ := continuity.NewSQLStore(db.DB, clock)
	created, err := store.Create(t.Context(), continuity.CreateIntent{ProjectID: "PRJ-1", WorkspaceID: "WSP-1", TaskID: "TSK-1",
		PredecessorSessionID: "SES-1", PredecessorRunHash: make([]byte, 32), Kind: continuity.IntentSameTask, ExpiresAt: clock.Now().Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	tokenHash := sha256.Sum256([]byte("01234567890123456789012345678901"))
	if _, err := db.ExecContext(t.Context(), `INSERT INTO checkpoints
		(checkpoint_id, task_id, session_id, workspace_id, note, handoff, created_at)
		VALUES ('CHK-1','TSK-1','SES-1','WSP-1','handoff',1,'2026-09-28T08:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(t.Context(), `UPDATE continuity_intents SET state='SPAWN_READY', revision=5,
		checkpoint_id='CHK-1', takeover_token_hash=?, host_operation_id='HOST-1' WHERE intent_id=?`, tokenHash[:], created.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(t.Context(), `INSERT INTO sessions (session_id, workspace_id, label, started_at)
		VALUES ('SES-2','WSP-1',NULL,'2026-09-28T08:01:00Z')`); err != nil {
		t.Fatal(err)
	}
	coord := coordination.NewStore(db.DB, clock)
	if _, _, err := coord.AcquireLease(t.Context(), "PRJ-1", coordination.NamedSession("SES-2"), coordination.TaskTarget("TSK-1")); err == nil {
		t.Fatal("unbound session stole a prepared task")
	}
	if _, err := db.ExecContext(t.Context(), `UPDATE continuity_intents SET state='HANDED_OFF', revision=6 WHERE intent_id=?`, created.ID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := coord.AcquireLease(t.Context(), "PRJ-1", coordination.NamedSession("SES-2"), coordination.TaskTarget("TSK-1")); err == nil {
		t.Fatal("unbound session stole a handed-off task")
	}
	runHash := sha256.Sum256([]byte("successor-run"))
	claimed, err := store.ClaimTakeover(t.Context(), created.ID, tokenHash[:], runHash[:])
	if err != nil || claimed.State != continuity.StateClaimed {
		t.Fatalf("claim=%#v err=%v", claimed, err)
	}
	if _, err := store.ClaimTakeover(t.Context(), created.ID, tokenHash[:], runHash[:]); err != nil {
		t.Fatalf("same-run replay failed: %v", err)
	}
	bogusToken := sha256.Sum256([]byte("bogus-token-012345678901234567890"))
	if _, err := store.ClaimTakeover(t.Context(), created.ID, bogusToken[:], runHash[:]); !errors.Is(err, continuity.ErrInvalidToken) {
		t.Fatalf("bogus-token same-run replay error=%v", err)
	}
	otherRun := sha256.Sum256([]byte("other-run"))
	if _, err := store.ClaimTakeover(t.Context(), created.ID, tokenHash[:], otherRun[:]); !errors.Is(err, continuity.ErrInvalidToken) {
		t.Fatalf("different-run replay error=%v", err)
	}
	if err := store.AuthorizeResume(t.Context(), "TSK-1", "other-run"); !errors.Is(err, continuity.ErrReservationLost) {
		t.Fatalf("unreserved run authorization=%v", err)
	}
	if err := store.AuthorizeResume(t.Context(), "TSK-1", "successor-run"); err != nil {
		t.Fatalf("reserved run authorization=%v", err)
	}
	resumed, err := store.ResumeTakeover(t.Context(), created.ID, runHash[:], "SES-2")
	if err != nil || resumed.State != continuity.StateResumed || resumed.SuccessorSessionID != "SES-2" {
		t.Fatalf("resume=%#v err=%v", resumed, err)
	}
	acquired, _, err := coord.AcquireLease(t.Context(), "PRJ-1", coordination.NamedSession("SES-2"), coordination.TaskTarget("TSK-1"))
	if err != nil {
		t.Fatalf("bound successor could not acquire task: %v", err)
	}
	if _, err := store.ClaimTakeover(t.Context(), created.ID, bogusToken[:], runHash[:]); !errors.Is(err, continuity.ErrInvalidToken) {
		t.Fatalf("bogus-token resumed replay error=%v", err)
	}
	completed, err := store.CompleteTakeover(t.Context(), created.ID, runHash[:], "SES-2")
	if err != nil || completed.State != continuity.StateCompleted {
		t.Fatalf("complete=%#v err=%v", completed, err)
	}
	if _, _, err := coord.ReleaseLease(t.Context(), acquired.Lease.ID, coordination.NamedSession("SES-2")); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(t.Context(), `INSERT INTO sessions (session_id, workspace_id, label, started_at)
		VALUES ('SES-3','WSP-1',NULL,'2026-09-28T08:02:00Z')`); err != nil {
		t.Fatal(err)
	}
	if _, _, err := coord.AcquireLease(t.Context(), "PRJ-1", coordination.NamedSession("SES-3"), coordination.TaskTarget("TSK-1")); err == nil {
		t.Fatal("unrelated session stole a nonterminal task after continuity takeover completed")
	}
	if _, err := store.ClaimTakeover(t.Context(), created.ID, tokenHash[:], runHash[:]); err != nil {
		t.Fatalf("completed same-run replay failed: %v", err)
	}
	if _, err := store.ClaimTakeover(t.Context(), created.ID, bogusToken[:], runHash[:]); !errors.Is(err, continuity.ErrInvalidToken) {
		t.Fatalf("bogus-token completed replay error=%v", err)
	}
}

func TestSQLStoreConcurrentTakeoverHasOneWinner(t *testing.T) {
	db, clock := newStoreFixture(t)
	store, _ := continuity.NewSQLStore(db.DB, clock)
	created, err := store.Create(t.Context(), continuity.CreateIntent{ProjectID: "PRJ-1", WorkspaceID: "WSP-1", TaskID: "TSK-1",
		PredecessorSessionID: "SES-1", PredecessorRunHash: make([]byte, 32), Kind: continuity.IntentSameTask, ExpiresAt: clock.Now().Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	tokenHash := sha256.Sum256([]byte("01234567890123456789012345678901"))
	if _, err := db.ExecContext(t.Context(), `INSERT INTO checkpoints
		(checkpoint_id, task_id, session_id, workspace_id, note, handoff, created_at)
		VALUES ('CHK-1','TSK-1','SES-1','WSP-1','handoff',1,'2026-09-28T08:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(t.Context(), `UPDATE continuity_intents SET state='HANDED_OFF', revision=5,
		checkpoint_id='CHK-1', takeover_token_hash=?, host_operation_id='HOST-1' WHERE intent_id=?`, tokenHash[:], created.ID); err != nil {
		t.Fatal(err)
	}
	runs := [2][32]byte{sha256.Sum256([]byte("successor-a")), sha256.Sum256([]byte("successor-b"))}
	errs := make([]error, len(runs))
	var wg sync.WaitGroup
	for i := range runs {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			_, errs[index] = store.ClaimTakeover(t.Context(), created.ID, tokenHash[:], runs[index][:])
		}(i)
	}
	wg.Wait()
	winners := 0
	for _, err := range errs {
		if err == nil {
			winners++
		} else if !errors.Is(err, continuity.ErrTokenConsumed) && !errors.Is(err, continuity.ErrInvalidToken) {
			t.Fatalf("unexpected claim error: %v", err)
		}
	}
	if winners != 1 {
		t.Fatalf("concurrent claim winners=%d errors=%v", winners, errs)
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
