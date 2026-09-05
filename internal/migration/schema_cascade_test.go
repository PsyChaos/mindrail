package migration_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/migration"
)

// This file is finding H1: SQLite removes a table's indexes and triggers along
// with the table, and a fold that subtracted only the table kept demanding them.
//
// Each test here runs Up, then Status, Pending and a second Up -- the three
// calls `init`, `status` and `doctor` make -- because the defect was not that
// one of them reported wrongly. It was that all of them failed permanently, and
// that the remedy they printed (rebuild the database) replayed the same
// migrations and landed in the identical state.

// TestStatusAcceptsAMigrationThatRetiresATableWithAnIndex is the defect itself,
// in its smallest form: a second migration that retires a table the first one
// created and indexed.
func TestStatusAcceptsAMigrationThatRetiresATableWithAnIndex(t *testing.T) {
	db := newDB(t)
	set, err := migration.Load(sqlFS(map[string]string{
		"000001_initial.sql": "CREATE TABLE legacy (id TEXT PRIMARY KEY, owner TEXT NOT NULL) STRICT;\n" +
			"CREATE INDEX idx_legacy_owner ON legacy(owner);\n" +
			"CREATE TABLE kept (id TEXT PRIMARY KEY) STRICT;\n",
		"000002_retire_legacy.sql": "DROP TABLE legacy;\n",
	}))
	if err != nil {
		t.Fatalf("Load = %v, want no error", err)
	}

	migrator := migration.New(db.DB, set, fixedClock())
	if _, err := migrator.Up(t.Context()); err != nil {
		t.Fatalf("Up = %v, want no error", err)
	}

	if _, err := migrator.Status(t.Context()); err != nil {
		t.Fatalf("Status = %v, want no error: SQLite dropped idx_legacy_owner with its table", err)
	}
	if _, err := migrator.Pending(t.Context()); err != nil {
		t.Fatalf("Pending = %v, want no error", err)
	}
	// The re-run is what made the printed remedy a loop.
	if _, err := migrator.Up(t.Context()); err != nil {
		t.Fatalf("second Up = %v, want no error", err)
	}
}

// TestStatusAcceptsThe12StepTableRebuild is the reason H1 was not a corner
// case. SQLite has no `ALTER TABLE ... ALTER COLUMN`, so the documented way to
// change a column is to rebuild the table, and a rebuild drops the original --
// taking its indexes with it. Both orderings of the recipe are exercised,
// because the migration set will eventually contain whichever one its author
// reached for.
func TestStatusAcceptsThe12StepTableRebuild(t *testing.T) {
	recipes := map[string]string{
		// The ordering in SQLite's own lang_altertable documentation.
		"new table first": `CREATE TABLE thing_new (
    id     TEXT PRIMARY KEY,
    tally  INTEGER NOT NULL DEFAULT 0
) STRICT;
INSERT INTO thing_new (id) SELECT id FROM thing;
DROP TABLE thing;
ALTER TABLE thing_new RENAME TO thing;
`,
		// The ordering most people write by hand. The rename carries
		// idx_thing_owner over to thing_old, and the drop then takes it.
		"rename first": `ALTER TABLE thing RENAME TO thing_old;
CREATE TABLE thing (
    id     TEXT PRIMARY KEY,
    tally  INTEGER NOT NULL DEFAULT 0
) STRICT;
INSERT INTO thing (id) SELECT id FROM thing_old;
DROP TABLE thing_old;
`,
	}

	for name, rebuild := range recipes {
		t.Run(name, func(t *testing.T) {
			db := newDB(t)
			set, err := migration.Load(sqlFS(map[string]string{
				"000001_initial.sql": "CREATE TABLE thing (id TEXT PRIMARY KEY, owner TEXT NOT NULL) STRICT;\n" +
					"CREATE INDEX idx_thing_owner ON thing(owner);\n",
				"000002_rebuild.sql": rebuild,
			}))
			if err != nil {
				t.Fatalf("Load = %v, want no error", err)
			}

			migrator := migration.New(db.DB, set, fixedClock())
			if _, err := migrator.Up(t.Context()); err != nil {
				t.Fatalf("Up = %v, want no error", err)
			}
			if _, err := migrator.Status(t.Context()); err != nil {
				t.Fatalf("Status = %v, want no error after a 12-step rebuild", err)
			}
			if _, err := migrator.Up(t.Context()); err != nil {
				t.Fatalf("second Up = %v, want no error", err)
			}
		})
	}
}

// TestStatusAcceptsAMigrationThatRetiresAView is the same cascade one kind
// over: dropping a view takes its INSTEAD OF triggers with it.
func TestStatusAcceptsAMigrationThatRetiresAView(t *testing.T) {
	db := newDB(t)
	set, err := migration.Load(sqlFS(map[string]string{
		"000001_initial.sql": "CREATE TABLE thing (id TEXT PRIMARY KEY) STRICT;\n" +
			"CREATE VIEW thing_view AS SELECT id FROM thing;\n" +
			"CREATE TRIGGER thing_view_insert INSTEAD OF INSERT ON thing_view\n" +
			"BEGIN INSERT INTO thing (id) VALUES (NEW.id); END;\n",
		"000002_retire_view.sql": "DROP VIEW thing_view;\n",
	}))
	if err != nil {
		t.Fatalf("Load = %v, want no error", err)
	}

	migrator := migration.New(db.DB, set, fixedClock())
	if _, err := migrator.Up(t.Context()); err != nil {
		t.Fatalf("Up = %v, want no error", err)
	}
	if _, err := migrator.Status(t.Context()); err != nil {
		t.Fatalf("Status = %v, want no error: SQLite dropped the view's trigger with it", err)
	}
}

// TestStatusStillDetectsAMissingTableAfterACascade is the over-fire guard in the
// direction that matters: narrowing the check to tables must not stop it seeing
// the damage it exists for. The migration below legitimately retires one table,
// and a table it left alone is then removed by hand.
func TestStatusStillDetectsAMissingTableAfterACascade(t *testing.T) {
	db := newDB(t)
	set, err := migration.Load(sqlFS(map[string]string{
		"000001_initial.sql": "CREATE TABLE legacy (id TEXT PRIMARY KEY, owner TEXT NOT NULL) STRICT;\n" +
			"CREATE INDEX idx_legacy_owner ON legacy(owner);\n" +
			"CREATE TABLE kept (id TEXT PRIMARY KEY) STRICT;\n",
		"000002_retire_legacy.sql": "DROP TABLE legacy;\n",
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

	_, err = migrator.Status(t.Context())
	if !errors.Is(err, migration.ErrSchemaObjectMissing) {
		t.Fatalf("Status = %v, want errors.Is(err, ErrSchemaObjectMissing) for the surviving table", err)
	}
	payload, ok := app.PayloadOf(err)
	if !ok {
		t.Fatalf("PayloadOf(%v) = _, false, want a domain payload", err)
	}
	if want := "table kept"; !strings.Contains(payload.Why, want) {
		t.Errorf("payload.Why = %q, want it to name %q", payload.Why, want)
	}
}

// TestStatusToleratesAnIndexDroppedByHand pins the guarantee this check
// deliberately gave up, so that giving it up stays a decision rather than an
// accident. A missing index costs query time; it never makes a command fail on
// an object that is not there, which is the failure the check exists for. The
// price of detecting it was a fold over objects SQLite removes implicitly, and
// that fold bricked every command the first time a migration retired a table.
func TestStatusToleratesAnIndexDroppedByHand(t *testing.T) {
	db := newDB(t)
	migrator := migration.New(db.DB, embeddedSet(t), fixedClock())

	if _, err := migrator.Up(t.Context()); err != nil {
		t.Fatalf("Up = %v, want no error", err)
	}
	if _, err := db.ExecContext(t.Context(), `DROP INDEX idx_workspaces_project`); err != nil {
		t.Fatalf("DROP INDEX = %v, want no error", err)
	}

	if _, err := migrator.Status(t.Context()); err != nil {
		t.Errorf("Status = %v, want no error; an absent index is not a failed command", err)
	}
}

// TestStatusRefusesATableReplacedByAnObjectOfAnotherKind keeps the narrowing
// honest at its edge. sqlite_master names are unique across kinds, so a view
// standing where a table should be is not the table, and the check has to say
// so rather than accept the name.
func TestStatusRefusesATableReplacedByAnObjectOfAnotherKind(t *testing.T) {
	db := newDB(t)
	migrator := migration.New(db.DB, embeddedSet(t), fixedClock())

	if _, err := migrator.Up(t.Context()); err != nil {
		t.Fatalf("Up = %v, want no error", err)
	}
	for _, stmt := range []string{
		`DROP TABLE workspaces`,
		`CREATE VIEW workspaces AS SELECT project_id FROM projects`,
	} {
		if _, err := db.ExecContext(t.Context(), stmt); err != nil {
			t.Fatalf("%s = %v, want no error", stmt, err)
		}
	}

	if _, err := migrator.Status(t.Context()); !errors.Is(err, migration.ErrSchemaObjectMissing) {
		t.Fatalf("Status = %v, want errors.Is(err, ErrSchemaObjectMissing): a view is not the table", err)
	}
}

// TestLoadRecordsIfNotExistsOnACreate pins the parse the "must be absent" pass
// depends on. Reading a tolerant create as an intolerant one would refuse an
// init the database would have accepted.
func TestLoadRecordsIfNotExistsOnACreate(t *testing.T) {
	set, err := migration.Load(sqlFS(map[string]string{
		"000001_initial.sql": "CREATE TABLE IF NOT EXISTS tolerant (id TEXT PRIMARY KEY) STRICT;\n" +
			"CREATE TABLE strict_one (id TEXT PRIMARY KEY) STRICT;\n" +
			"CREATE INDEX IF NOT EXISTS idx_tolerant ON tolerant(id);\n",
	}))
	if err != nil {
		t.Fatalf("Load = %v, want no error", err)
	}

	want := map[string]bool{"tolerant": true, "strict_one": false, "idx_tolerant": true}
	got := map[string]bool{}
	for _, effect := range set[0].Effects {
		got[effect.Object.Name] = effect.IfNotExists
	}

	if len(got) != len(want) {
		t.Fatalf("Effects = %+v, want one per object in %v", set[0].Effects, want)
	}
	for name, tolerant := range want {
		if got[name] != tolerant {
			t.Errorf("IfNotExists for %q = %v, want %v", name, got[name], tolerant)
		}
	}

	// The names must survive the extra capture group: an off-by-one in the
	// submatch indices would silently rename every object in every migration.
	if set[0].Objects[0] != (migration.SchemaObject{Kind: "table", Name: "tolerant"}) {
		t.Errorf("Objects[0] = %+v, want the table `tolerant`", set[0].Objects[0])
	}
}
