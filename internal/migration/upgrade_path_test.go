package migration_test

import (
	"errors"
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

func TestSchemaTenUpgradesAddAgentTelemetryWithoutTouchingExistingRows(t *testing.T) {
	full, err := migration.Load(migrations.FS)
	if err != nil {
		t.Fatal(err)
	}
	if len(full) < 11 || full[10].Version != 11 {
		t.Fatalf("embedded migrations = %d; migration 11 is not in its pinned position", len(full))
	}
	db := newDB(t)
	if _, err := migration.New(db.DB, full[:10], fixedClock()).Up(t.Context()); err != nil {
		t.Fatal(err)
	}
	for _, stmt := range []string{
		`INSERT INTO projects (project_id, common_dir, registered_at) VALUES ('PRJ-OLD', '/old/.git', '2026-01-01T00:00:00Z')`,
		`INSERT INTO workspaces (workspace_id, project_id, root_path, git_dir, is_linked_worktree, registered_at, last_seen_at)
		 VALUES ('WSP-OLD', 'PRJ-OLD', '/old', '/old/.git', 0, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`,
		`INSERT INTO sessions (session_id, workspace_id, label, started_at) VALUES ('SES-OLD', 'WSP-OLD', NULL, '2026-01-01T00:00:00Z')`,
		`INSERT INTO tasks (task_id, project_id, title, state, blocked_reason, opened_by, claimed_by, created_at, updated_at, revision)
		 VALUES ('TSK-OLD', 'PRJ-OLD', 'existing', 'OPEN', NULL, 'SES-OLD', NULL, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z', 1)`,
	} {
		if _, err := db.ExecContext(t.Context(), stmt); err != nil {
			t.Fatal(err)
		}
	}
	result, err := migration.New(db.DB, full[:11], fixedClock()).Up(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Applied) != 1 || result.Applied[0].Version != 11 {
		t.Fatalf("applied = %+v, want only migration 11", result.Applied)
	}
	var title string
	if err := db.QueryRowContext(t.Context(), `SELECT title FROM tasks WHERE task_id = 'TSK-OLD'`).Scan(&title); err != nil || title != "existing" {
		t.Fatalf("existing task title = %q, err = %v", title, err)
	}
	for _, table := range []string{"jev_route_events", "agent_runtimes"} {
		var found string
		if err := db.QueryRowContext(t.Context(), `SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&found); err != nil {
			t.Fatalf("new table %s: %v", table, err)
		}
	}
}

func TestSchemaTwelveUpgradesContinuityHardeningInPlace(t *testing.T) {
	full, err := migration.Load(migrations.FS)
	if err != nil {
		t.Fatal(err)
	}
	if len(full) != 13 || full[11].Version != 12 || full[12].Version != 13 {
		t.Fatalf("embedded migrations=%d; want schema 12 followed by 13", len(full))
	}
	db := newDB(t)
	if _, err := migration.New(db.DB, full[:12], fixedClock()).Up(t.Context()); err != nil {
		t.Fatal(err)
	}
	result, err := migration.New(db.DB, full, fixedClock()).Up(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Applied) != 1 || result.Applied[0].Version != 13 {
		t.Fatalf("applied=%+v, want only migration 13", result.Applied)
	}
	for table, column := range map[string]string{
		"agent_runtime_observations": "producer_sequence",
		"continuity_intents":         "activated_at",
	} {
		rows, err := db.QueryContext(t.Context(), `SELECT name FROM pragma_table_info(?) WHERE name = ?`, table, column)
		if err != nil {
			t.Fatal(err)
		}
		if !rows.Next() {
			rows.Close()
			t.Fatalf("%s.%s missing after schema 13", table, column)
		}
		rows.Close()
	}
	for _, trigger := range []string{"continuity_phase_insert_guard", "continuity_phase_update_guard"} {
		var found string
		if err := db.QueryRowContext(t.Context(), `SELECT name FROM sqlite_master WHERE type='trigger' AND name=?`, trigger).Scan(&found); err != nil {
			t.Fatalf("trigger %s missing: %v", trigger, err)
		}
	}
}

func TestSchemaThirteenRejectsMalformedAdvancedContinuityRows(t *testing.T) {
	full, err := migration.Load(migrations.FS)
	if err != nil {
		t.Fatal(err)
	}
	db := newDB(t)
	if _, err := migration.New(db.DB, full[:12], fixedClock()).Up(t.Context()); err != nil {
		t.Fatal(err)
	}
	for _, stmt := range []string{
		`INSERT INTO projects VALUES ('PRJ-1','/repo/.git','2026-09-28T08:00:00Z')`,
		`INSERT INTO workspaces VALUES ('WSP-1','PRJ-1','/repo','/repo/.git',0,'2026-09-28T08:00:00Z','2026-09-28T08:00:00Z')`,
		`INSERT INTO sessions VALUES ('SES-1','WSP-1',NULL,'2026-09-28T08:00:00Z')`,
		`INSERT INTO tasks VALUES ('TSK-1','PRJ-1','task','IN_PROGRESS',NULL,'SES-1','SES-1','2026-09-28T08:00:00Z','2026-09-28T08:00:00Z',1)`,
		`INSERT INTO continuity_intents
		 (intent_id,project_id,workspace_id,task_id,predecessor_session_id,predecessor_run_hash,kind,state,revision,
		  last_observation_sequence,last_used_basis_points,consecutive_handoff_observations,created_at,updated_at,expires_at)
		 VALUES ('CTI-1','PRJ-1','WSP-1','TSK-1','SES-1',zeroblob(32),'SAME_TASK','HANDED_OFF',1,2,6000,2,
		 '2026-09-28T08:00:00Z','2026-09-28T08:00:00Z','2026-09-29T08:00:00Z')`,
	} {
		if _, err := db.ExecContext(t.Context(), stmt); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := migration.New(db.DB, full, fixedClock()).Up(t.Context()); err == nil {
		t.Fatal("schema 13 accepted a malformed schema-12 HANDED_OFF row")
	}
}

// TestATaskWrittenBeforeTheRevisionColumnStartsAtOne is MR-004's AC-02.3 on
// the rows a user already has: a task written by the MR-003 binary, at schema
// 2, comes through migration 3 with revision 1 — the value every row written
// after it also starts at — and the shape check accepts the upgraded table.
func TestATaskWrittenBeforeTheRevisionColumnStartsAtOne(t *testing.T) {
	full, err := migration.Load(migrations.FS)
	if err != nil {
		t.Fatalf("Load(embedded) = %v, want no error", err)
	}
	if len(full) < 3 || full[2].Version != 3 {
		t.Fatalf("the embedded set is %d migrations long; this test is about the third", len(full))
	}

	db := newDB(t)
	if _, err := migration.New(db.DB, full[:2], fixedClock()).Up(t.Context()); err != nil {
		t.Fatalf("Up(schema 2) = %v, want no error", err)
	}

	// The rows the foreign keys need, then a task, all as the MR-003 binary
	// would have written them: no revision column exists to write.
	for _, stmt := range []string{
		`INSERT INTO projects (project_id, common_dir, registered_at) VALUES ('PRJ-1', '/repo/.git', '2026-01-01T00:00:00Z')`,
		`INSERT INTO workspaces (workspace_id, project_id, root_path, git_dir, is_linked_worktree, registered_at, last_seen_at)
		 VALUES ('WSP-1', 'PRJ-1', '/repo', '/repo/.git', 0, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`,
		`INSERT INTO sessions (session_id, workspace_id, label, started_at) VALUES ('SES-1', 'WSP-1', NULL, '2026-01-01T00:00:00Z')`,
		`INSERT INTO tasks (task_id, project_id, title, state, blocked_reason, opened_by, claimed_by, created_at, updated_at)
		 VALUES ('TSK-1', 'PRJ-1', 'written at schema 2', 'OPEN', NULL, 'SES-1', NULL, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`,
	} {
		if _, err := db.ExecContext(t.Context(), stmt); err != nil {
			t.Fatalf("%s = %v, want no error", stmt, err)
		}
	}

	result, err := migration.New(db.DB, full[:3], fixedClock()).Up(t.Context())
	if err != nil {
		t.Fatalf("Up(full) over schema 2 = %v, want no error", err)
	}
	if len(result.Applied) != 1 || result.Applied[0].Version != 3 {
		t.Fatalf("the upgrade applied %v, want exactly migration 3", result.Applied)
	}

	var revision int64
	if err := db.QueryRowContext(t.Context(), `SELECT revision FROM tasks WHERE task_id = 'TSK-1'`).Scan(&revision); err != nil {
		t.Fatalf("SELECT revision = %v, want the column the upgrade added", err)
	}
	if revision != 1 {
		t.Errorf("revision of a task written before the column = %d, want 1", revision)
	}
	if _, err := migration.New(db.DB, full[:3], fixedClock()).Status(t.Context()); err != nil {
		t.Errorf("Status after the upgrade = %v, want no error: tasks with its added column is the expected shape", err)
	}
}

// TestTheRealTasksTableIsCheckedForItsAddedColumn is AC-02.4 on the table
// that ships rather than on a fixture: `revision` dropped from `tasks` by hand
// — which SQLite allows under foreign_keys = 1, as TASK-02's Breaker showed
// and its first record denied — is reported as damage, not as health.
func TestTheRealTasksTableIsCheckedForItsAddedColumn(t *testing.T) {
	full, err := migration.Load(migrations.FS)
	if err != nil {
		t.Fatalf("Load(embedded) = %v, want no error", err)
	}
	db := newDB(t)
	migrator := migration.New(db.DB, full, fixedClock())
	if _, err := migrator.Up(t.Context()); err != nil {
		t.Fatalf("Up = %v, want no error", err)
	}
	if _, err := migrator.Status(t.Context()); err != nil {
		t.Fatalf("Status on the healthy schema = %v, want no error", err)
	}

	if _, err := db.ExecContext(t.Context(), `ALTER TABLE tasks DROP COLUMN revision`); err != nil {
		t.Fatalf("DROP COLUMN revision = %v, want no error: SQLite drops a non-key column under foreign_keys = 1", err)
	}
	if _, err := migrator.Status(t.Context()); !errors.Is(err, migration.ErrSchemaShapeChanged) {
		t.Fatalf("Status without revision = %v, want ErrSchemaShapeChanged", err)
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
