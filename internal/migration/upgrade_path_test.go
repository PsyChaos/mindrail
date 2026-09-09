package migration_test

import (
	"slices"
	"testing"

	"github.com/PsyChaos/mindrail/internal/migration"
	"github.com/PsyChaos/mindrail/migrations"
)

// TestADatabaseAtTheOlderSchemaTakesOnlyTheNewMigration is MR-003's AC-02.3, and
// it is the one property a synthetic migration set cannot establish.
//
// Every other test in this package builds its own two- or three-file fixture, so
// they grade the migrator's logic and say nothing about the files that ship. This
// one drives the **embedded** set twice: once truncated to what MR-001 shipped,
// which is the database a user upgrading from the previous release actually has,
// and then in full.
//
// The assertion that matters is `Applied` on the second run. `Result.Applied`
// holds this run only, so "exactly one migration ran" is a statement that
// migration 1 was recognised as already applied rather than replayed — and a
// replay would either fail on the existing tables or, worse, succeed against a
// `CREATE TABLE IF NOT EXISTS` and quietly leave the ledger describing a schema
// nobody applied.
func TestADatabaseAtTheOlderSchemaTakesOnlyTheNewMigration(t *testing.T) {
	full, err := migration.Load(migrations.FS)
	if err != nil {
		t.Fatalf("Load(embedded) = %v, want no error", err)
	}
	if len(full) < 2 {
		t.Fatalf("the embedded set holds %d migrations; this test needs the upgrade path", len(full))
	}

	previousRelease := slices.Clone(full[:len(full)-1])
	newest := full[len(full)-1]

	db := newDB(t)

	first, err := migration.New(db.DB, previousRelease, fixedClock()).Up(t.Context())
	if err != nil {
		t.Fatalf("Up(previous release) = %v, want no error", err)
	}
	if len(first.Applied) != len(previousRelease) {
		t.Fatalf("the first run applied %d migrations, want %d", len(first.Applied), len(previousRelease))
	}
	if first.CurrentVersion != previousRelease[len(previousRelease)-1].Version {
		t.Fatalf("current version after the first run = %d, want %d",
			first.CurrentVersion, previousRelease[len(previousRelease)-1].Version)
	}

	second, err := migration.New(db.DB, full, fixedClock()).Up(t.Context())
	if err != nil {
		t.Fatalf("Up(full set) over the older schema = %v, want no error", err)
	}
	if len(second.Applied) != 1 {
		t.Fatalf("the upgrade applied %d migrations, want exactly the newest one", len(second.Applied))
	}
	if second.Applied[0].Version != newest.Version {
		t.Errorf("the upgrade applied migration %d, want %d", second.Applied[0].Version, newest.Version)
	}
	if second.CurrentVersion != newest.Version {
		t.Errorf("current version = %d, want %d", second.CurrentVersion, newest.Version)
	}

	// The other arm, and the one that would catch an upgrade that "succeeded" by
	// doing nothing: the tables the newest migration declares have to be there,
	// and the shape check has to accept the result. Status is what doctor calls,
	// so a schema this run left half-built is reported here rather than at the
	// first write.
	for _, object := range newest.Objects {
		if object.Kind != "table" {
			continue
		}
		var name string
		err := db.QueryRowContext(t.Context(),
			`SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?`, object.Name).Scan(&name)
		if err != nil {
			t.Errorf("table %q declared by migration %d is not in the upgraded database: %v",
				object.Name, newest.Version, err)
		}
	}
	if _, err := migration.New(db.DB, full, fixedClock()).Status(t.Context()); err != nil {
		t.Errorf("Status after the upgrade = %v, want no error", err)
	}

	// And running the full set again changes nothing, which is the idempotence
	// `mindrail init` relies on.
	third, err := migration.New(db.DB, full, fixedClock()).Up(t.Context())
	if err != nil {
		t.Fatalf("second Up(full set) = %v, want no error", err)
	}
	if len(third.Applied) != 0 {
		t.Errorf("re-running the full set applied %d migrations, want none", len(third.Applied))
	}
}

// TestAFreshDatabaseTakesEveryEmbeddedMigration is the over-fire guard for the
// test above: the upgrade path is only interesting if the direct path works, and
// a set that failed on a fresh database would make "exactly one applied" true
// for the wrong reason.
func TestAFreshDatabaseTakesEveryEmbeddedMigration(t *testing.T) {
	full, err := migration.Load(migrations.FS)
	if err != nil {
		t.Fatalf("Load(embedded) = %v, want no error", err)
	}

	db := newDB(t)
	result, err := migration.New(db.DB, full, fixedClock()).Up(t.Context())
	if err != nil {
		t.Fatalf("Up(embedded) = %v, want no error", err)
	}
	if len(result.Applied) != len(full) {
		t.Errorf("a fresh database took %d of %d migrations", len(result.Applied), len(full))
	}
	if result.CurrentVersion != full[len(full)-1].Version {
		t.Errorf("current version = %d, want %d", result.CurrentVersion, full[len(full)-1].Version)
	}
}
