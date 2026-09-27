package index_test

import (
	"path/filepath"
	"testing"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/index"
	"github.com/PsyChaos/mindrail/internal/migration"
	"github.com/PsyChaos/mindrail/internal/storage"
	"github.com/PsyChaos/mindrail/migrations"
)

// TestIndexSchemaVersionNamesItsCreatingMigration pins the store gate to the
// migration that first creates these tables. A later migration can ship
// without making the store falsely claim its tables require that version.
func TestIndexSchemaVersionNamesItsCreatingMigration(t *testing.T) {
	set, err := migration.Load(migrations.FS)
	if err != nil {
		t.Fatalf("loading the embedded migration set: %v", err)
	}

	db, err := storage.Open(t.Context(), storage.Options{Path: t.TempDir() + "/mindrail.db"})
	if err != nil {
		t.Fatalf("opening the runtime database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	migrator := migration.New(db.DB, set, app.FixedClock{})
	if len(set) < 5 || set[4].Version != index.TableSchemaVersion || set[4].Name != "symbol_identity" {
		t.Fatalf("index schema gate %d does not name migration 000005_symbol_identity in the embedded set", index.TableSchemaVersion)
	}
	if _, err := migrator.Up(t.Context()); err != nil {
		t.Fatalf("applying the migration set: %v", err)
	}

	applied, err := migrator.Status(t.Context())
	if err != nil {
		t.Fatalf("reading the ledger: %v", err)
	}
	found := false
	for _, row := range applied {
		if row.Version == index.TableSchemaVersion {
			found = true
		}
	}
	if !found {
		t.Errorf("the ledger does not contain index.TableSchemaVersion %d", index.TableSchemaVersion)
	}
}

func TestFileGenerationMutationRequiresUpgradeFromVersionNine(t *testing.T) {
	set, err := migration.Load(migrations.FS)
	if err != nil {
		t.Fatal(err)
	}
	db, err := storage.Open(t.Context(), storage.Options{Path: filepath.Join(t.TempDir(), "old.db")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	clock := app.SystemClock{}
	if _, err := migration.New(db.DB, set[:9], clock).Up(t.Context()); err != nil {
		t.Fatal(err)
	}
	store := index.NewStore(db.DB, clock)
	unit, err := store.UpsertUnit(t.Context(), t.TempDir(), index.UnitPython)
	if err != nil {
		t.Fatal(err)
	}
	legacyPath := filepath.Join(unit.Path, "legacy.py")
	if _, err := db.ExecContext(t.Context(), `INSERT INTO file_index_state(path,unit_id,language,content_hash,state,attempts) VALUES(?,?,'python',?,'indexed',7)`, legacyPath, unit.ID, casHash("legacy")); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(t.Context(), `INSERT INTO symbols(unit_id,path,logical_key,kind,name,start_line,start_col,end_line,end_col,signature_hash,body_hash,structure_hash) VALUES(?,?,'legacy','function','legacy',1,0,2,0,'s','b','t')`, unit.ID, legacyPath); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(unit.Path, "file.py")
	missing, err := store.ReadFileState(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = store.RegisterFileCAS(t.Context(), missing, unit.ID, path, "python", casHash("body"))
	if payload, ok := app.PayloadOf(err); !ok || payload.Code != app.CodeMigrationFailed || payload.Metadata["required_version"] != "10" {
		t.Fatalf("v9 mutation should require migration10: %v", err)
	}
	if _, err := migration.New(db.DB, set, clock).Up(t.Context()); err != nil {
		t.Fatal(err)
	}
	legacy, err := store.ReadFileState(t.Context(), legacyPath)
	if err != nil || legacy.State.Attempts != 7 || legacy.State.ContentHash != casHash("legacy") {
		t.Fatalf("upgrade changed legacy live state: %+v %v", legacy, err)
	}
	if got := rowCount(t, db.DB, "symbols", legacyPath); got != 1 {
		t.Fatalf("upgrade removed legacy symbols: %d", got)
	}
	pending, applied, err := store.RegisterFileCAS(t.Context(), missing, unit.ID, path, "python", casHash("body"))
	if err != nil || !applied {
		t.Fatalf("upgraded registration: %v %v", applied, err)
	}
	if _, applied, err := store.RemoveFileCAS(t.Context(), pending); err != nil || !applied {
		t.Fatalf("upgraded removal: %v %v", applied, err)
	}
	if _, err := db.ExecContext(t.Context(), `INSERT INTO file_index_generations(path,generation) VALUES('invalid',-1)`); err == nil {
		t.Fatal("negative generation accepted")
	}
}
