package index_test

import (
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
