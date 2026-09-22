package changes_test

import (
	"testing"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/changes"
	"github.com/PsyChaos/mindrail/internal/migration"
	"github.com/PsyChaos/mindrail/internal/storage"
	"github.com/PsyChaos/mindrail/migrations"
)

// TestChangesSchemaVersionNamesItsCreatingMigration pins the store gate to
// the migration that first creates these tables.
func TestChangesSchemaVersionNamesItsCreatingMigration(t *testing.T) {
	db, err := storage.Open(t.Context(), storage.Options{Path: t.TempDir() + "/mindrail.db"})
	if err != nil {
		t.Fatalf("opening the runtime database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	set, err := migration.Load(migrations.FS)
	if err != nil {
		t.Fatal(err)
	}
	if len(set) < 6 || set[5].Version != changes.TableSchemaVersion || set[5].Name != "changes" {
		t.Fatalf("changes schema gate %d does not name migration 000006_changes in the embedded set", changes.TableSchemaVersion)
	}
	migrator := migration.New(db.DB, set, app.FixedClock{})
	if _, err := migrator.Up(t.Context()); err != nil {
		t.Fatalf("applying the migration set: %v", err)
	}
}
