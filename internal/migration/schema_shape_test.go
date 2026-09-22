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
		"leases": {
			"lease_id", "project_id", "target_kind", "target_key", "holder",
			"acquired_at", "renewed_at", "expires_at", "released_at", "release_reason",
		},
		"operations":       {"operation_id", "command", "request_hash", "result", "recorded_at"},
		"project_units":    {"id", "path", "kind", "discovered_at"},
		"file_index_state": {"path", "unit_id", "language", "content_hash", "state", "attempts", "last_error", "indexed_at"},
		"symbols": {
			"id", "unit_id", "path", "logical_key", "kind", "name", "container", "start_line", "start_col",
			"end_line", "end_col", "signature_hash", "body_hash", "structure_hash",
		},
		"symbol_imports":    {"id", "unit_id", "path", "importer_key", "module", "names", "alias", "is_relative"},
		"symbol_references": {"id", "unit_id", "path", "referrer_key", "target_text", "scope_text", "label", "confidence", "resolved_symbol_id"},
		"symbol_identities": {"symbol_uid", "project_id", "unit_id", "language", "logical_key", "container_uid", "previous_keys", "created_at"},
		"invariant_symbol_bindings": {
			"invariant_id", "symbol_uid", "status", "reason", "updated_at",
		},
		"symbol_identity_ambiguities": {"id", "unit_id", "removed_uid", "removed_key", "candidate_keys", "created_at"},
		"changes":                     {"change_id", "task_id", "operation_id", "created_at", "updated_at"},
		"change_files":                {"change_id", "path", "kind", "old_path", "content_hash", "discovered_via"},
		"change_symbols":              {"change_id", "logical_key", "symbol_uid", "kind", "body_changed", "signature_changed", "structure_changed", "signature_hash", "body_hash", "structure_hash", "discovered_via"},
		"change_baselines":            {"task_id", "path", "content_hash", "captured_at"},
		"change_operations":           {"operation_id", "task_id", "request_hash", "result", "recorded_at"},
	}

	got := map[string][]string{}
	added := map[string][]string{}
	for _, m := range embeddedSet(t) {
		for table, columns := range m.Columns {
			got[table] = columns
		}
		for table, columns := range m.Added {
			added[table] = append(added[table], columns...)
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

	// The two ALTER TABLE ... ADD COLUMN forms the embedded set carries (000003
	// on tasks, decision D-73; 000005 on symbols, decision D-103) are read as
	// additions and as nothing else: a parser that read either as a forgotten
	// table would silently drop that table from the shape check, and one that
	// invented a third column would fail every healthy repository.
	if strings.Join(added["tasks"], ",") != "revision" || strings.Join(added["symbols"], ",") != "symbol_uid" || len(added) != 2 {
		t.Errorf("Added = %v, want exactly {tasks: [revision], symbols: [symbol_uid]}", added)
	}
	for _, m := range embeddedSet(t) {
		if len(m.Altered) != 0 {
			t.Errorf("migration %d forgets %v; the embedded set has no ALTER the checker cannot follow", m.Version, m.Altered)
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

// TestStatusExpectsAColumnAMigrationAdded is decision D-73's other half and
// requirement AC-02.4: ADD COLUMN is the one form of ALTER TABLE the checker
// follows, so a table that lost an added column is reported as damaged rather
// than silently dropped from the check. Both arms: the healthy upgraded
// schema passes, and a table rebuilt by hand without the added column fails.
func TestStatusExpectsAColumnAMigrationAdded(t *testing.T) {
	db := newDB(t)
	set, err := migration.Load(sqlFS(map[string]string{
		"000001_initial.sql": "CREATE TABLE thing (id TEXT PRIMARY KEY) STRICT;\n",
		"000002_revision.sql": "ALTER TABLE thing ADD COLUMN revision INTEGER NOT NULL DEFAULT 1;\n" +
			"ALTER TABLE thing ADD note TEXT;\n",
	}))
	if err != nil {
		t.Fatalf("Load = %v, want no error", err)
	}
	if got := set[1].Added["thing"]; strings.Join(got, ",") != "revision,note" {
		t.Fatalf("Added[thing] = %v, want [revision note]: both spellings of ADD COLUMN are read", got)
	}
	if len(set[1].Altered) != 0 {
		t.Fatalf("Altered = %v, want nothing forgotten for an ADD COLUMN", set[1].Altered)
	}

	migrator := migration.New(db.DB, set, fixedClock())
	if _, err := migrator.Up(t.Context()); err != nil {
		t.Fatalf("Up = %v, want no error", err)
	}
	if _, err := migrator.Status(t.Context()); err != nil {
		t.Fatalf("Status = %v, want no error on the schema the migrations built", err)
	}

	// The F9 shape: the table is rebuilt by hand with its original columns
	// only, so "the object exists" is satisfied and only the added column is
	// missing.
	for _, stmt := range []string{
		`DROP TABLE thing`,
		`CREATE TABLE thing (id TEXT PRIMARY KEY) STRICT`,
	} {
		if _, err := db.ExecContext(t.Context(), stmt); err != nil {
			t.Fatalf("%s = %v, want no error", stmt, err)
		}
	}
	if _, err := migrator.Status(t.Context()); !errors.Is(err, migration.ErrSchemaShapeChanged) {
		t.Fatalf("Status = %v, want ErrSchemaShapeChanged: the added column is gone and the checker must say so", err)
	}
}

// TestAddColumnIsReadTheWayTheColumnListIs is TASK-02's Breaker findings on
// the ADD COLUMN reader, which read the name with an ASCII pattern of its own
// where the CREATE side reads it with a tokenizer that knows SQLite's four
// quotings and every script. Each row is a shape the Breaker built; the
// healthy-schema arm runs the file and checks Status accepts what it built,
// because every wrong reading here is a healthy repository refused.
func TestAddColumnIsReadTheWayTheColumnListIs(t *testing.T) {
	for _, tc := range []struct {
		name      string
		sql       string
		wantAdded []string // nil: nothing added
		wantAlter bool     // the table is forgotten
	}{
		{"bare", "ALTER TABLE t ADD COLUMN c TEXT;\n", []string{"c"}, false},
		{"no COLUMN keyword", "ALTER TABLE t ADD c TEXT;\n", []string{"c"}, false},
		{"double-quoted", `ALTER TABLE t ADD COLUMN "c" TEXT;` + "\n", []string{"c"}, false},
		{"bracketed", "ALTER TABLE t ADD COLUMN [c] TEXT;\n", []string{"c"}, false},
		{"backticked", "ALTER TABLE t ADD COLUMN `c` TEXT;\n", []string{"c"}, false},
		{"a name in another script", "ALTER TABLE t ADD COLUMN sütun TEXT;\n", []string{"sütun"}, false},
		{"a column named column, quoted", `ALTER TABLE t ADD "column" TEXT;` + "\n", []string{"column"}, false},
		{"upper-cased name is stored lower", "ALTER TABLE t ADD COLUMN Revision INTEGER NOT NULL DEFAULT 1;\n", []string{"revision"}, false},
		{"split over two lines", "ALTER TABLE t\n    ADD COLUMN c TEXT;\n", []string{"c"}, false},
		{"lower-case keywords", "alter table t add column c text;\n", []string{"c"}, false},
		{"inside a block comment", "/*\nALTER TABLE t ADD COLUMN ghost TEXT;\n*/\n", nil, false},
		{"inside a line comment", "-- ALTER TABLE t ADD COLUMN ghost TEXT;\n", nil, false},
		{"drop column is forgotten", "ALTER TABLE t DROP COLUMN b;\n", nil, true},
		{"rename column is forgotten", "ALTER TABLE t RENAME COLUMN b TO z;\n", nil, true},
		{"add then drop forgets", "ALTER TABLE t ADD COLUMN c TEXT;\nALTER TABLE t DROP COLUMN c;\n", []string{"c"}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			set, err := migration.Load(sqlFS(map[string]string{
				"000001_initial.sql": "CREATE TABLE t (a TEXT, b TEXT) STRICT;\n",
				"000002_alter.sql":   tc.sql,
			}))
			if err != nil {
				t.Fatalf("Load = %v, want no error", err)
			}
			if got := strings.Join(set[1].Added["t"], ","); got != strings.Join(tc.wantAdded, ",") {
				t.Errorf("Added[t] = %v, want %v", set[1].Added["t"], tc.wantAdded)
			}
			if got := len(set[1].Altered) == 1 && set[1].Altered[0] == "t"; got != tc.wantAlter {
				t.Errorf("Altered = %v, want forgotten = %v", set[1].Altered, tc.wantAlter)
			}

			// The schema the file builds must satisfy the check derived from it.
			db := newDB(t)
			migrator := migration.New(db.DB, set, fixedClock())
			if _, err := migrator.Up(t.Context()); err != nil {
				t.Fatalf("Up = %v, want no error", err)
			}
			if _, err := migrator.Status(t.Context()); err != nil {
				t.Errorf("Status = %v on the schema the migration built; the reader expects a column no database has", err)
			}
		})
	}
}

// TestACreateTableAsSelectIsNotReadAsAColumnList is the predating case the
// same Breaker found beside the new one: the parenthesis in `count(*)` was
// taken for the start of a column list, and every healthy database was then
// missing a column named `*`.
func TestACreateTableAsSelectIsNotReadAsAColumnList(t *testing.T) {
	set, err := migration.Load(sqlFS(map[string]string{
		"000001_initial.sql": "CREATE TABLE src (a TEXT) STRICT;\nCREATE TABLE cnt AS SELECT count(*) AS n FROM src;\n",
	}))
	if err != nil {
		t.Fatalf("Load = %v, want no error", err)
	}
	if _, read := set[0].Columns["cnt"]; read {
		t.Errorf("Columns[cnt] = %v, want the AS SELECT table skipped rather than read", set[0].Columns["cnt"])
	}

	db := newDB(t)
	migrator := migration.New(db.DB, set, fixedClock())
	if _, err := migrator.Up(t.Context()); err != nil {
		t.Fatalf("Up = %v, want no error", err)
	}
	if _, err := migrator.Status(t.Context()); err != nil {
		t.Errorf("Status = %v on the schema the migration built", err)
	}
}

// TestStatusStillForgetsATableAnotherAlterTouched keeps the bail-out where
// D-73 leaves it: a RENAME COLUMN in the same file as an ADD COLUMN is a form
// the checker cannot follow, and the table is forgotten rather than half
// modelled.
func TestStatusStillForgetsATableAnotherAlterTouched(t *testing.T) {
	db := newDB(t)
	set, err := migration.Load(sqlFS(map[string]string{
		"000001_initial.sql": "CREATE TABLE thing (id TEXT PRIMARY KEY, old TEXT) STRICT;\n",
		"000002_reshape.sql": "ALTER TABLE thing ADD COLUMN extra TEXT;\n" +
			"ALTER TABLE thing RENAME COLUMN old TO renamed;\n",
	}))
	if err != nil {
		t.Fatalf("Load = %v, want no error", err)
	}
	if got := set[1].Altered; len(got) != 1 || got[0] != "thing" {
		t.Fatalf("Altered = %v, want [thing]: the rename is the half the checker cannot follow", got)
	}

	migrator := migration.New(db.DB, set, fixedClock())
	if _, err := migrator.Up(t.Context()); err != nil {
		t.Fatalf("Up = %v, want no error", err)
	}
	// Rebuilt without either the added or the renamed column: forgotten means
	// forgotten, and the shape check has nothing to say.
	for _, stmt := range []string{
		`DROP TABLE thing`,
		`CREATE TABLE thing (id TEXT PRIMARY KEY) STRICT`,
	} {
		if _, err := db.ExecContext(t.Context(), stmt); err != nil {
			t.Fatalf("%s = %v, want no error", stmt, err)
		}
	}
	if _, err := migrator.Status(t.Context()); err != nil {
		t.Fatalf("Status = %v, want no error: a table a RENAME COLUMN touched is not shape-checked", err)
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
