package changes_test

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/changes"
	"github.com/PsyChaos/mindrail/internal/migration"
	"github.com/PsyChaos/mindrail/internal/storage"
	"github.com/PsyChaos/mindrail/migrations"
)

func changesFixture(t *testing.T) *changes.Store {
	t.Helper()
	store, _, _ := changesFixtureDB(t)
	return store
}

func changesFixtureDB(t *testing.T) (*changes.Store, *storage.DB, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "mindrail.db")
	db, err := storage.Open(t.Context(), storage.Options{Path: path})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	set, err := migration.Load(migrations.FS)
	if err != nil {
		t.Fatal(err)
	}
	clock := app.FixedClock{Instant: time.Date(2026, 9, 23, 10, 0, 0, 0, time.UTC)}
	if _, err := migration.New(db.DB, set, clock).Up(t.Context()); err != nil {
		t.Fatal(err)
	}
	store, err := changes.NewStore(db.DB, clock)
	if err != nil {
		t.Fatal(err)
	}
	return store, db, path
}

// seedTaskChain writes the coordination rows a task-scoped change joins
// against: changes.task_id is a real foreign key, so tests name real tasks.
func seedTaskChain(t *testing.T, db *storage.DB, taskID string) {
	t.Helper()
	statements := []string{
		`INSERT OR IGNORE INTO projects (project_id, common_dir, registered_at)
			VALUES ('PRJ-1', '/repo/.git', '2026-09-23T10:00:00Z')`,
		`INSERT OR IGNORE INTO workspaces (workspace_id, project_id, root_path, git_dir, is_linked_worktree, registered_at, last_seen_at)
			VALUES ('WS-1', 'PRJ-1', '/repo', '/repo/.git', 0, '2026-09-23T10:00:00Z', '2026-09-23T10:00:00Z')`,
		`INSERT OR IGNORE INTO sessions (session_id, workspace_id, label, started_at)
			VALUES ('SES-1', 'WS-1', 's', '2026-09-23T10:00:00Z')`,
		`INSERT OR IGNORE INTO tasks (task_id, project_id, title, state, opened_by, created_at, updated_at)
			VALUES ('` + taskID + `', 'PRJ-1', 't', 'OPEN', 'SES-1', '2026-09-23T10:00:00Z', '2026-09-23T10:00:00Z')`,
	}
	for _, statement := range statements {
		if _, err := db.DB.ExecContext(t.Context(), statement); err != nil {
			t.Fatal(err)
		}
	}
}

func writeScopeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func shaOf(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

// TestCaptureBaselineReplacesWithoutStacking is AC-02.1: the latest capture
// is the whole truth — replaced, never stacked.
func TestCaptureBaselineReplacesWithoutStacking(t *testing.T) {
	store, db, _ := changesFixtureDB(t)
	seedTaskChain(t, db, "TSK-1")
	dir := t.TempDir()
	a := writeScopeFile(t, dir, "a.py", "a = 1\n")
	b := writeScopeFile(t, dir, "b.py", "b = 2\n")
	c := writeScopeFile(t, dir, "c.py", "c = 3\n")
	first, err := store.CaptureBaseline(t.Context(), "TSK-1", []string{a, b}, "")
	if err != nil || first.Files != 2 {
		t.Fatalf("capture = %+v, %v", first, err)
	}
	second, err := store.CaptureBaseline(t.Context(), "TSK-1", []string{b, c}, "")
	if err != nil || second.Files != 2 {
		t.Fatalf("recapture = %+v, %v", second, err)
	}
	baseline, err := store.ReadBaseline(t.Context(), "TSK-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(baseline) != 2 || baseline[b] != shaOf("b = 2\n") || baseline[c] != shaOf("c = 3\n") {
		t.Fatalf("baseline = %+v, want replaced scope with real hashes", baseline)
	}
	if _, ok := baseline[a]; ok {
		t.Fatalf("baseline keeps unscopes path %s", a)
	}
}

// TestCaptureBaselineEmptyHashForMissingAndNonRegular pins the best-effort
// half of AC-02.1: unhashable scope still records, never fails the capture.
func TestCaptureBaselineEmptyHashForMissingAndNonRegular(t *testing.T) {
	store, db, _ := changesFixtureDB(t)
	seedTaskChain(t, db, "TSK-1")
	dir := t.TempDir()
	missing := filepath.Join(dir, "gone.py")
	summary, err := store.CaptureBaseline(t.Context(), "TSK-1", []string{missing, dir}, "")
	if err != nil || summary.Files != 2 {
		t.Fatalf("capture = %+v, %v", summary, err)
	}
	baseline, err := store.ReadBaseline(t.Context(), "TSK-1")
	if err != nil {
		t.Fatal(err)
	}
	if baseline[missing] != "" || baseline[dir] != "" {
		t.Fatalf("baseline = %+v, want empty hashes for missing and directory", baseline)
	}
	if _, err := store.CaptureBaseline(t.Context(), "", []string{missing}, ""); err == nil {
		t.Fatal("capture accepted an empty task")
	}
	if _, err := store.CaptureBaseline(t.Context(), "TSK-1", []string{"relative.py"}, ""); err == nil {
		t.Fatal("capture accepted a relative path")
	}
	if _, err := store.CaptureBaseline(t.Context(), "TSK-1", []string{missing}, "bad id!"); err == nil {
		t.Fatal("capture accepted an unrepresentable operation id")
	}
}

// TestCaptureBaselineRefusesUnknownTask pins the fail-closed half of the
// task scope: baselines for tasks that do not exist are refused rather than
// orphaned.
func TestCaptureBaselineRefusesUnknownTask(t *testing.T) {
	store, _, _ := changesFixtureDB(t)
	dir := t.TempDir()
	a := writeScopeFile(t, dir, "a.py", "a = 1\n")
	if _, err := store.CaptureBaseline(t.Context(), "TSK-NOPE", []string{a}, ""); err == nil {
		t.Fatal("baseline accepted a missing task")
	}
}

// TestEnsureOpenChangeStable is AC-02.2's tasked half: one task, one row,
// every call.
func TestEnsureOpenChangeStable(t *testing.T) {
	store, db, _ := changesFixtureDB(t)
	seedTaskChain(t, db, "TSK-1")
	first, err := store.EnsureOpenChange(t.Context(), "TSK-1", "")
	if err != nil || first.ID == "" || first.TaskID != "TSK-1" {
		t.Fatalf("ensure = %+v, %v", first, err)
	}
	second, err := store.EnsureOpenChange(t.Context(), "TSK-1", "")
	if err != nil || second.ID != first.ID {
		t.Fatalf("second = %+v, want %s", second, first.ID)
	}
	third, err := store.EnsureOpenChange(t.Context(), "TSK-1", "OP-9")
	if err != nil || third.ID != first.ID {
		t.Fatalf("third = %+v, want the tasked row regardless of operation", third.ID)
	}
}

// TestEnsureOpenChangeRetroactive is AC-02.2's NULL-task half: no id mints
// fresh rows, an id replays its row.
func TestEnsureOpenChangeRetroactive(t *testing.T) {
	store := changesFixture(t)
	a, err := store.EnsureOpenChange(t.Context(), "", "")
	if err != nil || a.ID == "" || a.TaskID != "" {
		t.Fatalf("first = %+v, %v", a, err)
	}
	b, err := store.EnsureOpenChange(t.Context(), "", "")
	if err != nil || b.ID == a.ID {
		t.Fatalf("second = %+v, want a fresh retroactive row", b)
	}
	c, err := store.EnsureOpenChange(t.Context(), "", "OP-R")
	if err != nil {
		t.Fatal(err)
	}
	d, err := store.EnsureOpenChange(t.Context(), "", "OP-R")
	if err != nil || d.ID != c.ID {
		t.Fatalf("replay = %+v, want %s", d, c.ID)
	}
}

// TestOperationReplayAndConflict is AC-02.3: same id plus same content
// replays without new rows; same id plus other content conflicts.
func TestOperationReplayAndConflict(t *testing.T) {
	store, db, _ := changesFixtureDB(t)
	seedTaskChain(t, db, "TSK-1")
	seedTaskChain(t, db, "TSK-2")
	seedTaskChain(t, db, "TSK-9")
	first, err := store.EnsureOpenChange(t.Context(), "TSK-1", "OP-1")
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.EnsureOpenChange(t.Context(), "TSK-1", "OP-1")
	if err != nil || second.ID != first.ID {
		t.Fatalf("replay = %+v, want %s", second, first.ID)
	}
	if _, err := store.EnsureOpenChange(t.Context(), "TSK-2", "OP-1"); err == nil {
		t.Fatal("conflicting operation accepted")
	} else if payload, ok := app.PayloadOf(err); !ok || payload.Code != app.CodeOperationIDConflict {
		t.Fatalf("conflict = %v, want OPERATION_ID_CONFLICT", err)
	}
	dir := t.TempDir()
	a := writeScopeFile(t, dir, "a.py", "a = 1\n")
	before, err := store.CaptureBaseline(t.Context(), "TSK-9", []string{a}, "OP-B")
	if err != nil {
		t.Fatal(err)
	}
	after, err := store.CaptureBaseline(t.Context(), "TSK-9", []string{a}, "OP-B")
	if err != nil || after != before {
		t.Fatalf("baseline replay = %+v, want %+v", after, before)
	}
	var opRows int
	if err := db.DB.QueryRowContext(t.Context(), `SELECT count(*) FROM change_operations WHERE operation_id = 'OP-B'`).Scan(&opRows); err != nil || opRows != 1 {
		t.Fatalf("op rows = %d, %v — replay must not rewrite the log", opRows, err)
	}
	var captured string
	if err := db.DB.QueryRowContext(t.Context(), `SELECT captured_at FROM change_baselines WHERE task_id = 'TSK-9'`).Scan(&captured); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CaptureBaseline(t.Context(), "TSK-9", []string{a}, "OP-B"); err != nil {
		t.Fatal(err)
	}
	var recaptured string
	if err := db.DB.QueryRowContext(t.Context(), `SELECT captured_at FROM change_baselines WHERE task_id = 'TSK-9'`).Scan(&recaptured); err != nil || recaptured != captured {
		t.Fatalf("replay rewrote captured_at %q -> %q", captured, recaptured)
	}
	b := writeScopeFile(t, dir, "b.py", "b = 2\n")
	if _, err := store.CaptureBaseline(t.Context(), "TSK-9", []string{a, b}, "OP-B"); err == nil {
		t.Fatal("baseline conflict accepted")
	} else if payload, ok := app.PayloadOf(err); !ok || payload.Code != app.CodeOperationIDConflict {
		t.Fatalf("conflict = %v, want OPERATION_ID_CONFLICT", err)
	}
}

// TestBaselineClearingDiscipline is AC-02.4: baselines vanish on replace or
// explicit clear — never as a side effect of other store calls.
func TestBaselineClearingDiscipline(t *testing.T) {
	store, db, _ := changesFixtureDB(t)
	seedTaskChain(t, db, "TSK-1")
	dir := t.TempDir()
	a := writeScopeFile(t, dir, "a.py", "a = 1\n")
	if _, err := store.CaptureBaseline(t.Context(), "TSK-1", []string{a}, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := store.EnsureOpenChange(t.Context(), "TSK-1", ""); err != nil {
		t.Fatal(err)
	}
	baseline, err := store.ReadBaseline(t.Context(), "TSK-1")
	if err != nil || len(baseline) != 1 {
		t.Fatalf("baseline after other calls = %+v, %v", baseline, err)
	}
	if err := store.ClearBaseline(t.Context(), "TSK-1"); err != nil {
		t.Fatal(err)
	}
	empty, err := store.ReadBaseline(t.Context(), "TSK-1")
	if err != nil || len(empty) != 0 {
		t.Fatalf("cleared baseline = %+v, %v", empty, err)
	}
	if _, err := store.CaptureBaseline(t.Context(), "TSK-1", []string{a}, ""); err != nil {
		t.Fatalf("recapture after clear: %v", err)
	}
}

// TestConcurrentSameOperationConverges races one operation id across
// sixteen goroutines: every racer answers the winner's change id with no
// raw constraint error, and the log holds exactly one row.
func TestConcurrentSameOperationConverges(t *testing.T) {
	store, db, _ := changesFixtureDB(t)
	seedTaskChain(t, db, "TSK-R")
	const racers = 16
	ids := make([]string, racers)
	errs := make([]error, racers)
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := range racers {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			change, err := store.EnsureOpenChange(t.Context(), "TSK-R", "OP-RACE")
			if err != nil {
				errs[i] = err
				return
			}
			ids[i] = change.ID
		}(i)
	}
	close(start)
	wg.Wait()
	for i := range racers {
		if errs[i] != nil || ids[i] != ids[0] {
			t.Fatalf("racer %d = %q, %v", i, ids[i], errs[i])
		}
	}
	var opRows int
	if err := db.DB.QueryRowContext(t.Context(), `SELECT count(*) FROM change_operations WHERE operation_id = 'OP-RACE'`).Scan(&opRows); err != nil || opRows != 1 {
		t.Fatalf("op rows = %d, %v", opRows, err)
	}
}

// TestConcurrentEnsureAgreesOnOneRow races open-change creation: the partial
// unique index arbitrates, every racer reads back the winner.
func TestConcurrentEnsureAgreesOnOneRow(t *testing.T) {
	store, db, _ := changesFixtureDB(t)
	seedTaskChain(t, db, "TSK-R")
	const racers = 8
	ids := make([]string, racers)
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := range racers {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			change, err := store.EnsureOpenChange(t.Context(), "TSK-R", "")
			if err != nil {
				t.Errorf("racer %d: %v", i, err)
				return
			}
			ids[i] = change.ID
		}(i)
	}
	close(start)
	wg.Wait()
	for i := range racers {
		if ids[i] == "" || ids[i] != ids[0] {
			t.Fatalf("racer %d = %q, want %q", i, ids[i], ids[0])
		}
	}
}
