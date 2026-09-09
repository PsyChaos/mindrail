package migration_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/migration"
)

// TestLoadReadsTheColumnsOfTheEmbeddedSchema is the first over-fire guard for
// finding F9: if the parser invented a column, every healthy repository would
// fail, so what it reads off the real migration is pinned exactly.
func TestLoadReadsTheColumnsOfTheEmbeddedSchema(t *testing.T) {
	want := map[string][]string{
		"projects": {"project_id", "common_dir", "registered_at"},
		"workspaces": {
			"workspace_id", "project_id", "root_path", "git_dir",
			"is_linked_worktree", "registered_at", "last_seen_at",
		},
		"sessions": {"session_id", "workspace_id", "label", "started_at"},
		"tasks": {
			"task_id", "project_id", "title", "state", "blocked_reason",
			"opened_by", "claimed_by", "created_at", "updated_at",
		},
		"checkpoints": {
			"checkpoint_id", "task_id", "session_id", "workspace_id",
			"note", "handoff", "created_at",
		},
	}

	got := map[string][]string{}
	for _, m := range embeddedSet(t) {
		for table, columns := range m.Columns {
			got[table] = columns
		}
	}

	if len(got) != len(want) {
		t.Fatalf("Columns = %v, want one entry per table in %v", got, want)
	}
	for table, columns := range want {
		if strings.Join(got[table], ",") != strings.Join(columns, ",") {
			t.Errorf("Columns[%q] = %v, want %v", table, got[table], columns)
		}
	}
}

// TestLoadDoesNotInventColumnsFromConstraintsOrComments is the parser's
// over-fire guard proper. Every item here is something a column list can
// legally contain that is not a column, and reading any of them as one would
// make the shape check demand a column no database has.
func TestLoadDoesNotInventColumnsFromConstraintsOrComments(t *testing.T) {
	set, err := migration.Load(sqlFS(map[string]string{
		"000001_initial.sql": "CREATE TABLE parent (id TEXT PRIMARY KEY) STRICT;\n" +
			`CREATE TABLE awkward (
    id            TEXT PRIMARY KEY,
    -- a note, with a comma in it
    "quoted name" TEXT NOT NULL,
    [bracketed]   TEXT NOT NULL,
    labelled      TEXT NOT NULL DEFAULT 'a,b',   /* and, a, block, comment */
    ranged        INTEGER NOT NULL,
    parent_id     TEXT NOT NULL,
    CONSTRAINT ck_named CHECK (ranged > 0 AND ranged < 10),
    CHECK (length(labelled) > 0),
    PRIMARY KEY (id, ranged),
    UNIQUE (labelled, ranged),
    FOREIGN KEY (parent_id) REFERENCES parent(id)
);
`,
	}))
	if err != nil {
		t.Fatalf("Load = %v, want no error", err)
	}

	want := []string{"id", "quoted name", "bracketed", "labelled", "ranged", "parent_id"}
	got := set[0].Columns["awkward"]
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("Columns[\"awkward\"] = %v, want %v", got, want)
	}
}

// TestStatusRefusesATableRecreatedWithADifferentShape is finding F9.
//
// Existence alone let a table that had been dropped and rebuilt by hand pass,
// so `doctor` printed "Schema up to date", the next write failed on a missing
// column, and the remedy for *that* was "run `mindrail doctor`" -- back through
// the check that had just called the schema healthy.
func TestStatusRefusesATableRecreatedWithADifferentShape(t *testing.T) {
	db := newDB(t)
	migrator := migration.New(db.DB, embeddedSet(t), fixedClock())

	if _, err := migrator.Up(t.Context()); err != nil {
		t.Fatalf("Up = %v, want no error", err)
	}
	for _, stmt := range []string{
		`DROP TABLE workspaces`,
		`CREATE TABLE workspaces (workspace_id TEXT PRIMARY KEY, project_id TEXT NOT NULL) STRICT`,
		`CREATE INDEX idx_workspaces_project ON workspaces(project_id)`,
	} {
		if _, err := db.ExecContext(t.Context(), stmt); err != nil {
			t.Fatalf("%s = %v, want no error", stmt, err)
		}
	}

	_, err := migrator.Status(t.Context())
	if !errors.Is(err, migration.ErrSchemaShapeChanged) {
		t.Fatalf("Status = %v, want errors.Is(err, ErrSchemaShapeChanged)", err)
	}

	payload, ok := app.PayloadOf(err)
	if !ok {
		t.Fatalf("PayloadOf(%v) = _, false, want a domain payload", err)
	}
	if payload.Code != app.CodeMigrationFailed {
		t.Errorf("payload.Code = %q, want %q", payload.Code, app.CodeMigrationFailed)
	}
	for _, column := range []string{"root_path", "git_dir", "last_seen_at"} {
		if !strings.Contains(payload.Why, column) {
			t.Errorf("payload.Why = %q, want it to name the missing %q", payload.Why, column)
		}
	}
	if len(payload.NextAction) == 0 || strings.Contains(strings.Join(payload.NextAction, " "), "doctor") {
		t.Errorf("next actions %v send the user back through the check that just reported this", payload.NextAction)
	}
}

// TestStatusAcceptsAHealthyDatabase is the over-fire guard the whole shape
// check hangs on: the schema `mindrail init` produces must pass, repeatedly.
func TestStatusAcceptsAHealthyDatabase(t *testing.T) {
	db := newDB(t)
	migrator := migration.New(db.DB, embeddedSet(t), fixedClock())

	if _, err := migrator.Up(t.Context()); err != nil {
		t.Fatalf("Up = %v, want no error", err)
	}
	for i := range 3 {
		if _, err := migrator.Status(t.Context()); err != nil {
			t.Fatalf("Status #%d = %v, want no error on a freshly initialised database", i, err)
		}
	}
	if _, err := migrator.Up(t.Context()); err != nil {
		t.Fatalf("second Up = %v, want no error", err)
	}
}

// TestStatusToleratesAnExtraColumn keeps the check a containment assertion. A
// column the migration did not declare is not damage -- a later migration is
// allowed to add one -- and demanding equality would turn the check into the
// schema diff this package refuses to be.
func TestStatusToleratesAnExtraColumn(t *testing.T) {
	db := newDB(t)
	migrator := migration.New(db.DB, embeddedSet(t), fixedClock())

	if _, err := migrator.Up(t.Context()); err != nil {
		t.Fatalf("Up = %v, want no error", err)
	}
	if _, err := db.ExecContext(t.Context(), `ALTER TABLE projects ADD COLUMN note TEXT`); err != nil {
		t.Fatalf("ALTER TABLE projects ADD COLUMN = %v, want no error", err)
	}

	if _, err := migrator.Status(t.Context()); err != nil {
		t.Errorf("Status = %v, want no error; every declared column is still there", err)
	}
}

// TestStatusStopsCheckingTheShapeOfATableAMigrationAlters is the F4 trap
// avoided one level down.
//
// ALTER TABLE can add, drop or rename a column. A checker that kept demanding
// the columns of the original CREATE would brick every command the day a
// migration dropped one, and the printed remedy could not recover -- a rebuild
// replays the same migrations. Rather than grow a column-level replay, the
// check gives up on any table a recorded migration has altered.
func TestStatusStopsCheckingTheShapeOfATableAMigrationAlters(t *testing.T) {
	db := newDB(t)
	set, err := migration.Load(sqlFS(map[string]string{
		"000001_initial.sql":   "CREATE TABLE thing (id TEXT PRIMARY KEY, doomed TEXT NOT NULL) STRICT;\n",
		"000002_slim.sql":      "ALTER TABLE thing DROP COLUMN doomed;\n",
		"000003_unrelated.sql": "CREATE TABLE other (id TEXT PRIMARY KEY) STRICT;\n",
	}))
	if err != nil {
		t.Fatalf("Load = %v, want no error", err)
	}
	if got := set[1].Altered; len(got) != 1 || got[0] != "thing" {
		t.Fatalf("Altered = %v, want [thing]", got)
	}

	migrator := migration.New(db.DB, set, fixedClock())
	if _, err := migrator.Up(t.Context()); err != nil {
		t.Fatalf("Up = %v, want no error", err)
	}
	if _, err := migrator.Status(t.Context()); err != nil {
		t.Fatalf("Status = %v, want no error: version 2 dropped `doomed` on purpose", err)
	}
	// The tables nobody altered are still checked.
	if _, err := db.ExecContext(t.Context(), `DROP TABLE other`); err != nil {
		t.Fatalf("DROP TABLE other = %v, want no error", err)
	}
	if _, err := migrator.Status(t.Context()); !errors.Is(err, migration.ErrSchemaObjectMissing) {
		t.Fatalf("Status = %v, want the unaltered table still verified", err)
	}
}

// TestAnEditedMigrationIsReportedAsAChecksumMismatch is the ordering guard.
//
// A migration file edited after it ran no longer describes the schema, so
// deriving expectations from it reports the *database* as damaged and offers a
// rebuild -- when the fix is to revert the edit. The condition has its own
// diagnosis and it has to be the one that reaches the user.
func TestAnEditedMigrationIsReportedAsAChecksumMismatch(t *testing.T) {
	db := newDB(t)

	original, err := migration.Load(sqlFS(map[string]string{
		"000001_initial.sql": "CREATE TABLE alpha (id TEXT PRIMARY KEY) STRICT;\n",
	}))
	if err != nil {
		t.Fatalf("Load = %v, want no error", err)
	}
	if _, err := migration.New(db.DB, original, fixedClock()).Up(t.Context()); err != nil {
		t.Fatalf("Up = %v, want no error", err)
	}

	edited, err := migration.Load(sqlFS(map[string]string{
		"000001_initial.sql": "CREATE TABLE alpha (id TEXT PRIMARY KEY, extra TEXT) STRICT;\n",
	}))
	if err != nil {
		t.Fatalf("Load(edited) = %v, want no error", err)
	}

	_, err = migration.New(db.DB, edited, fixedClock()).Up(t.Context())
	if !errors.Is(err, migration.ErrChecksumMismatch) {
		t.Fatalf("Up = %v, want errors.Is(err, ErrChecksumMismatch)", err)
	}
	if errors.Is(err, migration.ErrSchemaShapeChanged) {
		t.Errorf("Up = %v, which blames the database for an edited file", err)
	}

	// Status stays reportable so doctor can describe what it found.
	if _, err := migration.New(db.DB, edited, fixedClock()).Status(t.Context()); err != nil {
		t.Errorf("Status = %v, want no error: doctor has to be able to report this ledger", err)
	}
}
