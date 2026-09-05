package migration_test

import (
	"errors"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/migration"
)

// sqlFS builds a migration set from literal SQL, so a test can state the schema
// it needs instead of depending on whatever the embedded set happens to create.
func sqlFS(files map[string]string) fstest.MapFS {
	fsys := make(fstest.MapFS, len(files))
	for name, body := range files {
		fsys[name] = &fstest.MapFile{Data: []byte(body)}
	}
	return fsys
}

// TestLoadRecordsTheObjectsAMigrationCreates pins the parse the verification
// depends on. A migration whose objects came back empty would make every
// existence check below vacuously pass.
func TestLoadRecordsTheObjectsAMigrationCreates(t *testing.T) {
	found := map[migration.SchemaObject]bool{}
	for _, m := range embeddedSet(t) {
		for _, object := range m.Objects {
			found[object] = true
		}
	}

	for _, want := range []migration.SchemaObject{
		{Kind: "table", Name: "projects"},
		{Kind: "table", Name: "workspaces"},
		{Kind: "index", Name: "idx_workspaces_project"},
	} {
		if !found[want] {
			t.Errorf("embedded migrations declare %+v nowhere; declared set is %v", want, found)
		}
	}
}

func TestLoadIgnoresNonCreatingStatements(t *testing.T) {
	set, err := migration.Load(sqlFS(map[string]string{
		"000001_initial.sql": `-- CREATE TABLE commented_out (id TEXT);
CREATE TABLE kept (id TEXT PRIMARY KEY) STRICT;
CREATE TEMP TABLE scratch (id TEXT);
CREATE UNIQUE INDEX IF NOT EXISTS idx_kept ON kept(id);
INSERT INTO kept (id) VALUES ('seed');
`,
	}))
	if err != nil {
		t.Fatalf("Load = %v, want no error", err)
	}

	want := []migration.SchemaObject{
		{Kind: "table", Name: "kept"},
		{Kind: "index", Name: "idx_kept"},
	}
	if len(set[0].Objects) != len(want) {
		t.Fatalf("Objects = %+v, want %+v", set[0].Objects, want)
	}
	for i, object := range set[0].Objects {
		if object != want[i] {
			t.Errorf("Objects[%d] = %+v, want %+v", i, object, want[i])
		}
	}
}

// TestStatusRefusesALedgerTheSchemaDoesNotSupport is finding S4: the ledger is
// a claim, and reporting the claim as the schema is how `doctor` came to print
// "Schema up to date (version 1)" over a database with no tables in it.
func TestStatusRefusesALedgerTheSchemaDoesNotSupport(t *testing.T) {
	db := newDB(t)
	migrator := migration.New(db.DB, embeddedSet(t), fixedClock())

	if _, err := migrator.Up(t.Context()); err != nil {
		t.Fatalf("Up = %v, want no error", err)
	}
	for _, table := range []string{"workspaces", "projects"} {
		if _, err := db.ExecContext(t.Context(), `DROP TABLE `+table); err != nil {
			t.Fatalf("DROP TABLE %s = %v, want no error", table, err)
		}
	}

	ledger, err := migrator.Status(t.Context())
	if err == nil {
		t.Fatalf("Status = %v, nil; want an error naming the missing objects", ledger)
	}
	if !errors.Is(err, migration.ErrSchemaObjectMissing) {
		t.Errorf("Status = %v, want errors.Is(err, ErrSchemaObjectMissing)", err)
	}

	payload, ok := app.PayloadOf(err)
	if !ok {
		t.Fatalf("PayloadOf(%v) = _, false, want a domain payload", err)
	}
	if payload.Code != app.CodeMigrationFailed {
		t.Errorf("payload.Code = %q, want %q", payload.Code, app.CodeMigrationFailed)
	}
	for _, name := range []string{"projects", "workspaces"} {
		if !strings.Contains(payload.Why, name) {
			t.Errorf("payload.Why = %q, want it to name the missing %q", payload.Why, name)
		}
	}

	// Pending and Up read through Status, so neither may report health either.
	if _, err := migrator.Pending(t.Context()); !errors.Is(err, migration.ErrSchemaObjectMissing) {
		t.Errorf("Pending = %v, want errors.Is(err, ErrSchemaObjectMissing)", err)
	}
	if _, err := migrator.Up(t.Context()); !errors.Is(err, migration.ErrSchemaObjectMissing) {
		t.Errorf("Up = %v, want errors.Is(err, ErrSchemaObjectMissing)", err)
	}
}

// TestStatusToleratesAnIndexRecreatedByHand guards the check against becoming a
// schema diff: the objects have to exist and nothing more is asserted about
// them, so a legitimate rebuild of one is not reported as damage. That bound is
// what keeps the check cheap enough for the warm path.
func TestStatusToleratesAnIndexRecreatedByHand(t *testing.T) {
	db := newDB(t)
	migrator := migration.New(db.DB, embeddedSet(t), fixedClock())

	if _, err := migrator.Up(t.Context()); err != nil {
		t.Fatalf("Up = %v, want no error", err)
	}
	for _, stmt := range []string{
		`DROP INDEX idx_workspaces_project`,
		`CREATE INDEX idx_workspaces_project ON workspaces(project_id, root_path)`,
	} {
		if _, err := db.ExecContext(t.Context(), stmt); err != nil {
			t.Fatalf("%s = %v, want no error", stmt, err)
		}
	}

	if _, err := migrator.Status(t.Context()); err != nil {
		t.Errorf("Status = %v, want no error; the object exists", err)
	}
}

// TestStatusIgnoresObjectsOfUnappliedMigrations keeps the check from punishing a
// database that is merely behind: version 2's table is legitimately absent
// until version 2 runs.
func TestStatusIgnoresObjectsOfUnappliedMigrations(t *testing.T) {
	db := newDB(t)

	full, err := migration.Load(sqlFS(map[string]string{
		"000001_initial.sql": "CREATE TABLE one (id TEXT PRIMARY KEY) STRICT;\n",
		"000002_second.sql":  "CREATE TABLE two (id TEXT PRIMARY KEY) STRICT;\n",
	}))
	if err != nil {
		t.Fatalf("Load = %v, want no error", err)
	}

	if _, err := migration.New(db.DB, full[:1], fixedClock()).Up(t.Context()); err != nil {
		t.Fatalf("Up(first only) = %v, want no error", err)
	}

	behind := migration.New(db.DB, full, fixedClock())
	if _, err := behind.Status(t.Context()); err != nil {
		t.Errorf("Status = %v, want no error; migration 2 has not been applied", err)
	}
	pending, err := behind.Pending(t.Context())
	if err != nil {
		t.Fatalf("Pending = %v, want no error", err)
	}
	if len(pending) != 1 {
		t.Errorf("Pending = %d migrations, want 1", len(pending))
	}
}
