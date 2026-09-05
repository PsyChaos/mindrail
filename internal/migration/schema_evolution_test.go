package migration_test

import (
	"errors"
	"testing"

	"github.com/PsyChaos/mindrail/internal/migration"
)

// TestLoadRecordsTheObjectsAMigrationRemoves is the parse half of finding F4. A
// migration that drops an object has to be able to say so, or the verification
// below cannot subtract it.
func TestLoadRecordsTheObjectsAMigrationRemoves(t *testing.T) {
	set, err := migration.Load(sqlFS(map[string]string{
		"000001_initial.sql": "CREATE TABLE legacy (id TEXT PRIMARY KEY) STRICT;\n",
		"000002_evolve.sql": `-- DROP TABLE commented_out;
DROP TABLE legacy;
DROP INDEX IF EXISTS idx_gone;
`,
	}))
	if err != nil {
		t.Fatalf("Load = %v, want no error", err)
	}

	want := []migration.SchemaObject{
		{Kind: "table", Name: "legacy"},
		{Kind: "index", Name: "idx_gone"},
	}
	if len(set[1].Removes()) != len(want) {
		t.Fatalf("Removes = %+v, want %+v", set[1].Removes(), want)
	}
	for i, object := range set[1].Removes() {
		if object != want[i] {
			t.Errorf("Removes[%d] = %+v, want %+v", i, object, want[i])
		}
	}
}

// TestLoadRecordsARenameAsBothSides pins the other legitimate way an earlier
// object stops existing: it was renamed, so the old name must be subtracted and
// the new one expected in its place.
func TestLoadRecordsARenameAsBothSides(t *testing.T) {
	set, err := migration.Load(sqlFS(map[string]string{
		"000001_initial.sql": "CREATE TABLE old_name (id TEXT PRIMARY KEY) STRICT;\n",
		"000002_rename.sql":  "ALTER TABLE old_name RENAME TO new_name;\n",
	}))
	if err != nil {
		t.Fatalf("Load = %v, want no error", err)
	}

	if got, want := set[1].Removes(), []migration.SchemaObject{{Kind: "table", Name: "old_name"}}; !equalObjects(got, want) {
		t.Errorf("Removes = %+v, want %+v", got, want)
	}
	if got, want := set[1].Objects, []migration.SchemaObject{{Kind: "table", Name: "new_name"}}; !equalObjects(got, want) {
		t.Errorf("Objects = %+v, want %+v", got, want)
	}
}

func equalObjects(got, want []migration.SchemaObject) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

// TestStatusAcceptsAMigrationThatDropsAnEarlierTable is finding F4.
//
// verifySchemaObjects demanded the union of everything every recorded migration
// ever created, so the first migration that legitimately removed an earlier
// object made every command -- `mindrail init` included -- fail permanently, and
// the printed remedy could not recover: a rebuilt database reapplies both
// migrations and lands in the identical state.
func TestStatusAcceptsAMigrationThatDropsAnEarlierTable(t *testing.T) {
	db := newDB(t)
	set, err := migration.Load(sqlFS(map[string]string{
		"000001_initial.sql": "CREATE TABLE legacy (id TEXT PRIMARY KEY) STRICT;\n" +
			"CREATE TABLE kept (id TEXT PRIMARY KEY) STRICT;\n",
		"000002_drop_legacy.sql": "DROP TABLE legacy;\n",
	}))
	if err != nil {
		t.Fatalf("Load = %v, want no error", err)
	}

	migrator := migration.New(db.DB, set, fixedClock())
	if _, err := migrator.Up(t.Context()); err != nil {
		t.Fatalf("Up = %v, want no error", err)
	}

	if _, err := migrator.Status(t.Context()); err != nil {
		t.Fatalf("Status = %v, want no error: version 2 removed `legacy` on purpose", err)
	}
	if _, err := migrator.Pending(t.Context()); err != nil {
		t.Fatalf("Pending = %v, want no error", err)
	}
	// The re-run is the part the old remedy could not recover from.
	if _, err := migrator.Up(t.Context()); err != nil {
		t.Fatalf("second Up = %v, want no error", err)
	}
}

// TestStatusAcceptsAMigrationThatRenamesAnEarlierTable is the same guarantee for
// the other shape of legitimate removal.
func TestStatusAcceptsAMigrationThatRenamesAnEarlierTable(t *testing.T) {
	db := newDB(t)
	set, err := migration.Load(sqlFS(map[string]string{
		"000001_initial.sql": "CREATE TABLE old_name (id TEXT PRIMARY KEY) STRICT;\n",
		"000002_rename.sql":  "ALTER TABLE old_name RENAME TO new_name;\n",
	}))
	if err != nil {
		t.Fatalf("Load = %v, want no error", err)
	}

	migrator := migration.New(db.DB, set, fixedClock())
	if _, err := migrator.Up(t.Context()); err != nil {
		t.Fatalf("Up = %v, want no error", err)
	}
	if _, err := migrator.Status(t.Context()); err != nil {
		t.Fatalf("Status = %v, want no error: version 2 renamed `old_name` on purpose", err)
	}
}

// TestStatusStillReportsTheSurvivingObjectsAsMissing is the over-fire guard in
// the other direction: subtracting what a later migration removed must not
// weaken the check on what it left behind.
func TestStatusStillReportsTheSurvivingObjectsAsMissing(t *testing.T) {
	db := newDB(t)
	set, err := migration.Load(sqlFS(map[string]string{
		"000001_initial.sql": "CREATE TABLE legacy (id TEXT PRIMARY KEY) STRICT;\n" +
			"CREATE TABLE kept (id TEXT PRIMARY KEY) STRICT;\n",
		"000002_drop_legacy.sql": "DROP TABLE legacy;\n",
	}))
	if err != nil {
		t.Fatalf("Load = %v, want no error", err)
	}

	migrator := migration.New(db.DB, set, fixedClock())
	if _, err := migrator.Up(t.Context()); err != nil {
		t.Fatalf("Up = %v, want no error", err)
	}
	if _, err := db.ExecContext(t.Context(), `DROP TABLE kept`); err != nil {
		t.Fatalf("DROP TABLE kept = %v, want no error", err)
	}

	if _, err := migrator.Status(t.Context()); !errors.Is(err, migration.ErrSchemaObjectMissing) {
		t.Fatalf("Status = %v, want errors.Is(err, ErrSchemaObjectMissing) for the surviving table", err)
	}
}

// TestStatusStillExpectsAnObjectAPendingMigrationWouldDrop keeps the fold
// honest about versions: a database that stopped at version 1 must still have
// version 1's objects, even though version 2 would remove one of them.
func TestStatusStillExpectsAnObjectAPendingMigrationWouldDrop(t *testing.T) {
	db := newDB(t)
	set, err := migration.Load(sqlFS(map[string]string{
		"000001_initial.sql":     "CREATE TABLE legacy (id TEXT PRIMARY KEY) STRICT;\n",
		"000002_drop_legacy.sql": "DROP TABLE legacy;\n",
	}))
	if err != nil {
		t.Fatalf("Load = %v, want no error", err)
	}

	if _, err := migration.New(db.DB, set[:1], fixedClock()).Up(t.Context()); err != nil {
		t.Fatalf("Up(first only) = %v, want no error", err)
	}
	if _, err := db.ExecContext(t.Context(), `DROP TABLE legacy`); err != nil {
		t.Fatalf("DROP TABLE legacy = %v, want no error", err)
	}

	behind := migration.New(db.DB, set, fixedClock())
	if _, err := behind.Status(t.Context()); !errors.Is(err, migration.ErrSchemaObjectMissing) {
		t.Fatalf("Status = %v, want errors.Is(err, ErrSchemaObjectMissing): version 2 has not run yet", err)
	}
}
