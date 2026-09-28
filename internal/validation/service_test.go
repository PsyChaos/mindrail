package validation_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/config"
	"github.com/PsyChaos/mindrail/internal/migration"
	"github.com/PsyChaos/mindrail/internal/storage"
	"github.com/PsyChaos/mindrail/internal/validation"
	"github.com/PsyChaos/mindrail/migrations"
)

// TestRunProfileFlowEndToEnd is TASK-02 AC-02.5: profile → run → redact →
// snapshot → store in one flow. A leaked secret arrives redacted in the
// stored row, the snapshot hash reproduces from the tree, and the same
// operation id replays instead of duplicating.
func TestRunProfileFlowEndToEnd(t *testing.T) {
	t.Setenv("MR010_FLOW_SECRET", "flow-secret-1")
	fx := newEvidenceFixture(t)
	runner, err := validation.NewRunner(t.TempDir(), 10*time.Second, 0)
	if err != nil {
		t.Fatal(err)
	}
	store := fx.store
	service, err := validation.NewService(runner, store)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "tests"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "tests", "a.py"), []byte("print(1)\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	profile := config.ValidationProfile{
		Type:  "AUTOMATED_TEST",
		Paths: []string{"tests"},
		Commands: [][]string{
			{"echo", "leaked flow-secret-1"},
			{"echo", "clean"},
		},
	}

	first, err := service.RunProfile(t.Context(), "test", profile, root,
		[]string{"MR010_FLOW_SECRET"}, "OP-FLOW")
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 2 {
		t.Fatalf("rows = %d, want 2", len(first))
	}
	if first[0].Status != validation.StatusPass || first[0].Output != "leaked [REDACTED]\n" {
		t.Fatalf("row = %+v", first[0])
	}
	if first[1].Output != "clean\n" {
		t.Fatalf("row = %+v", first[1])
	}
	want, err := validation.SnapshotScope(root, []string{"tests"})
	if err != nil {
		t.Fatal(err)
	}
	if first[0].SnapshotHash != want.Hash {
		t.Fatalf("snapshot = %q, want %q", first[0].SnapshotHash, want.Hash)
	}
	second, err := service.RunProfile(t.Context(), "test", profile, root,
		[]string{"MR010_FLOW_SECRET"}, "OP-FLOW")
	if err != nil {
		t.Fatal(err)
	}
	if second[0].ID != first[0].ID || second[1].ID != first[1].ID {
		t.Fatalf("replay duplicated rows: %+v vs %+v", second, first)
	}
	if second[0].Provenance != first[0].Provenance || second[1].Provenance != first[1].Provenance {
		t.Fatalf("replay changed run provenance: %+v vs %+v", second, first)
	}
	if _, coverage, err := validation.Check(root, second, []string{"test"}); err != nil {
		t.Fatal(err)
	} else if !coverage.Satisfied["test"] {
		t.Fatalf("idempotent replay lost complete-run coverage: %+v", coverage)
	}
	if _, err := service.RunProfile(t.Context(), "", profile, root, nil, ""); err == nil {
		t.Fatal("nameless profile accepted")
	}
}

func TestRunProfileEvidenceWriteInsideGitMetadataStaysCurrent(t *testing.T) {
	root := t.TempDir()
	initSnapshotGitRepository(t, root)
	writeScopeFile(t, root, "source.go", "package source\n")
	addSnapshotGitPaths(t, root, "source.go")

	databasePath := filepath.Join(root, ".git", "mindrail", "mindrail.db")
	if err := os.MkdirAll(filepath.Dir(databasePath), 0o755); err != nil {
		t.Fatal(err)
	}
	db, err := storage.Open(t.Context(), storage.Options{Path: databasePath})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	set, err := migration.Load(migrations.FS)
	if err != nil {
		t.Fatal(err)
	}
	clock := app.FixedClock{Instant: time.Date(2026, 9, 28, 10, 0, 0, 0, time.UTC)}
	if _, err := migration.New(db.DB, set, clock).Up(t.Context()); err != nil {
		t.Fatal(err)
	}
	store, err := validation.NewStore(db.DB, clock)
	if err != nil {
		t.Fatal(err)
	}
	runner, err := validation.NewRunner(root, 10*time.Second, 0)
	if err != nil {
		t.Fatal(err)
	}
	service, err := validation.NewService(runner, store)
	if err != nil {
		t.Fatal(err)
	}
	profile := config.ValidationProfile{
		Type: "AUTOMATED_TEST", Paths: []string{"."}, Commands: [][]string{{"true"}},
	}

	rows, err := service.RunProfile(t.Context(), "check", profile, root, nil, "OP-GIT-METADATA")
	if err != nil {
		t.Fatal(err)
	}
	verdicts, coverage, err := validation.Check(root, rows, []string{"check"})
	if err != nil {
		t.Fatal(err)
	}
	if len(verdicts) != 1 || verdicts[0].Status != validation.FreshCurrent || !coverage.Satisfied["check"] {
		t.Fatalf("evidence write made its own snapshot stale: verdicts=%+v coverage=%+v", verdicts, coverage)
	}
}

func TestBreakerPartialOperationReplay(t *testing.T) {
	fx := newEvidenceFixture(t)
	runner, err := validation.NewRunner(t.TempDir(), 10*time.Second, 0)
	if err != nil {
		t.Fatal(err)
	}
	service, err := validation.NewService(runner, fx.store)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	writeScopeFile(t, root, "tests/a.py", "print(1)\n")
	profile := config.ValidationProfile{
		Type: "AUTOMATED_TEST", Paths: []string{"tests"},
		Commands: [][]string{{"true"}, {"true"}},
	}

	first, err := service.RunProfile(t.Context(), "test", profile, root, nil, "resume")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fx.db.DB.ExecContext(t.Context(), `DELETE FROM evidence WHERE operation_id = ?`, "resume#1"); err != nil {
		t.Fatal(err)
	}
	replayed, err := service.RunProfile(t.Context(), "test", profile, root, nil, "resume")
	if err != nil {
		t.Fatal(err)
	}
	if replayed[0].ID != first[0].ID {
		t.Fatalf("first command was not replayed: first=%s replay=%s", first[0].ID, replayed[0].ID)
	}
	runIDs := map[string]bool{}
	for _, row := range replayed {
		var provenance struct {
			RunID string `json:"run_id"`
		}
		if err := json.Unmarshal([]byte(row.Provenance), &provenance); err != nil {
			t.Fatal(err)
		}
		runIDs[provenance.RunID] = true
	}
	if len(runIDs) != 1 {
		t.Fatalf("partial replay split one logical run across run IDs: %#v", runIDs)
	}
	if _, coverage, err := validation.Check(root, replayed, []string{"test"}); err != nil {
		t.Fatal(err)
	} else if !coverage.Satisfied["test"] {
		t.Fatalf("partial replay lost complete-run coverage: %#v", coverage)
	}
}

func TestRunProfileRejectsReplayedProvenanceMismatch(t *testing.T) {
	mutations := map[string]func(map[string]any){
		"profile":           func(provenance map[string]any) { provenance["profile"] = "other" },
		"command count":     func(provenance map[string]any) { provenance["command_count"] = 3 },
		"command index":     func(provenance map[string]any) { provenance["command_index"] = 1 },
		"scope declaration": func(provenance map[string]any) { provenance["scope_paths"] = []string{"other"} },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			fx := newEvidenceFixture(t)
			runner, err := validation.NewRunner(t.TempDir(), 10*time.Second, 0)
			if err != nil {
				t.Fatal(err)
			}
			service, err := validation.NewService(runner, fx.store)
			if err != nil {
				t.Fatal(err)
			}
			root := t.TempDir()
			writeScopeFile(t, root, "tests/a.py", "print(1)\n")
			profile := config.ValidationProfile{
				Type: "AUTOMATED_TEST", Paths: []string{"tests"},
				Commands: [][]string{{"true"}, {"true"}},
			}
			rows, err := service.RunProfile(t.Context(), "test", profile, root, nil, "mismatch")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := fx.db.DB.ExecContext(t.Context(), `DELETE FROM evidence WHERE operation_id = ?`, "mismatch#1"); err != nil {
				t.Fatal(err)
			}
			var provenance map[string]any
			if err := json.Unmarshal([]byte(rows[0].Provenance), &provenance); err != nil {
				t.Fatal(err)
			}
			mutate(provenance)
			tampered, err := json.Marshal(provenance)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := fx.db.DB.ExecContext(t.Context(), `UPDATE evidence SET provenance = ? WHERE operation_id = ?`, string(tampered), "mismatch#0"); err != nil {
				t.Fatal(err)
			}

			if _, err := service.RunProfile(t.Context(), "test", profile, root, nil, "mismatch"); err == nil {
				t.Fatal("replayed provenance mismatch accepted")
			} else if payload, ok := app.PayloadOf(err); !ok || payload.Code != app.CodeOperationIDConflict {
				t.Fatalf("error = %v, want %s", err, app.CodeOperationIDConflict)
			}
			var count int
			if err := fx.db.DB.QueryRowContext(t.Context(), `SELECT count(*) FROM evidence`).Scan(&count); err != nil {
				t.Fatal(err)
			}
			if count != 1 {
				t.Fatalf("mismatch wrote remaining command row: count=%d", count)
			}
		})
	}
}
