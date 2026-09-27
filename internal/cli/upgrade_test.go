package cli_test

import (
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/coordination"
	"github.com/PsyChaos/mindrail/internal/storage"
)

// TestAWorktreeRegisteredByAnOlderBinaryIsStillRegistered is finding F01.
//
// MR-003 added the second migration, so on the first run after an upgrade every
// existing database is exactly one behind. The workspace lookup was skipped
// whenever anything was pending — a test that meant "the workspaces table does
// not exist yet" while there was one migration and stopped meaning it when
// there were two — so `status` reported `registered: false` about a table that
// was there and held the row, and `doctor` explained it with "there is no
// workspace table to query".
//
// The repository this builds is that repository: everything migration 2 created
// removed, and its ledger row with it, which is byte for byte what the previous
// binary left behind.
func TestAWorktreeRegisteredByAnOlderBinaryIsStillRegistered(t *testing.T) {
	repo := newInitializedRepo(t)
	downgradeToSchemaOne(t, repo)

	got := run(t, repo, "status", "--json")

	var data struct {
		Workspace struct {
			Observation string `json:"observation"`
			Registered  bool   `json:"registered"`
			ID          string `json:"workspace_id"`
		} `json:"workspace"`
	}
	decodeData(t, got.stdout, &data)

	if !data.Workspace.Registered {
		t.Errorf("registered = false for a worktree whose row is in the database:\n%s", got.stdout)
	}
	if data.Workspace.Observation != "observed" {
		t.Errorf("observation = %q for a lookup that ran and found the row", data.Workspace.Observation)
	}
	if data.Workspace.ID == "" {
		t.Error("the report carries no workspace id, so the row it found is not the one it reports")
	}
	if strings.Contains(got.stdout, string(app.CodeWorkspaceNotInitialized)) {
		t.Errorf("the report calls an upgraded repository uninitialised:\n%s", got.stdout)
	}

	// The condition that is genuinely present has to be the one reported. Only
	// `doctor` prints it: worst() ranks the workspace reading above the
	// migration one, so while the workspace check was failing this sentence was
	// never reached.
	diagnosed := run(t, repo, "doctor", "--no-color")
	if !strings.Contains(diagnosed.stdout, "Schema is behind this binary") {
		t.Errorf("doctor does not report the pending migration:\n%s", diagnosed.stdout)
	}

	// And the remedy works: init applies the one missing migration and the
	// repository comes back.
	if applied := run(t, repo, "init", "--json"); applied.code != app.ExitSuccess {
		t.Fatalf("init exited %d on an upgraded repository: %s", applied.code, applied.stdout)
	}
	after := run(t, repo, "status", "--json")
	after.requireExit(t, app.ExitSuccess)
	var upgraded struct {
		Workspace struct {
			ID string `json:"workspace_id"`
		} `json:"workspace"`
	}
	decodeData(t, after.stdout, &upgraded)
	if upgraded.Workspace.ID != data.Workspace.ID {
		t.Errorf("init changed the workspace id from %s to %s; the upgrade is supposed to preserve identity",
			data.Workspace.ID, upgraded.Workspace.ID)
	}
}

// TestAWorkspaceNobodyLookedUpIsNotReportedAsUnregistered is finding F41.
//
// The other side of the same reading. Where the table genuinely does not exist
// no lookup can be made, and the report used to publish `observation:
// "observed"` beside `registered: false` — "observed" being its own word for
// "the check ran and the values below it are findings", about a query that
// could not have run.
func TestAWorkspaceNobodyLookedUpIsNotReportedAsUnregistered(t *testing.T) {
	repo := newRepo(t)
	createUnmigratedDatabase(t, repo)

	got := run(t, repo, "status", "--json")

	var data struct {
		Workspace struct {
			Observation string `json:"observation"`
			Registered  bool   `json:"registered"`
		} `json:"workspace"`
	}
	decodeData(t, got.stdout, &data)

	if data.Workspace.Observation != "not_observed" {
		t.Errorf("observation = %q where there is no workspaces table to query, want not_observed:\n%s",
			data.Workspace.Observation, got.stdout)
	}
	if data.Workspace.Registered {
		t.Error("registered = true without a lookup")
	}
}

// TestCoordinationCommandsSendASchemaBehindDatabaseToInit is audit round 2, §4.1.
//
// The repository the fixture above builds — migration 1 applied, migration 2
// pending — is the state every existing database is in on its first run after
// an upgrade, and the workspace lookup rightly succeeds in it. The six
// coordination commands then held a store over a database with no sessions,
// tasks or checkpoints table, and answered `task list --json` with
// COORDINATION_READ_FAILED sending the reader to `mindrail doctor`, which
// exits 0 and clears nothing, while `status` on the same bytes called the
// condition MIGRATION_FAILED and printed the remedy that works. One
// condition, two codes, two remedies; this pins the one whose remedy clears.
//
// Two downgrades since MR-004 (requirement AC-02.5): a database the MR-002
// binary wrote, at schema 1, and one the MR-003 binary wrote, at schema 2 —
// which has every table the commands query except the leases and operations
// tables and the revision column, and is the state every existing repository
// is in on its first run after the MR-004 upgrade.
func TestCoordinationCommandsSendASchemaBehindDatabaseToInit(t *testing.T) {
	downgrades := []struct {
		name    string
		applied string // what the ledger holds after the downgrade
		to      func(*testing.T, string)
	}{
		{"schema 1", "1", downgradeToSchemaOne},
		{"schema 2", "2", downgradeToSchemaTwo},
	}
	for _, downgrade := range downgrades {
		// The commands that succeed on a fresh repository, so the retry after
		// `init` can be asserted to: the two MR-003 rows and, since MR-004's
		// TASK-06 gate, the two lease commands that need no id.
		for _, args := range [][]string{{"task", "list"}, {"session", "open"}, {"lease", "list"}, {"lease", "acquire", "--file", "src/behind.go"}} {
			name := commandName(args) + " at " + downgrade.name
			repo := newInitializedRepo(t)
			downgrade.to(t, repo)

			got := run(t, repo, append(slices.Clone(args), "--json")...)
			if got.code != app.ExitFailed {
				t.Fatalf("%s exited %d on a schema-behind database, want %d\n%s",
					name, got.code, app.ExitFailed, got.stdout)
			}
			payload := got.errorPayload(t)
			if payload.Code != app.CodeMigrationFailed {
				t.Errorf("`status` calls this condition %q and `%s` calls it %q; one condition has one code\n%s",
					app.CodeMigrationFailed, name, payload.Code, got.stdout)
			}
			// The sentence names the gap by number: it used to name the tables
			// migration 2 created, which was false on a database that had them
			// (TASK-02's Breaker).
			if payload.Metadata["applied_version"] != downgrade.applied ||
				payload.Metadata["required_version"] != strconv.FormatInt(coordination.TableSchemaVersion, 10) {
				t.Errorf("%s: metadata = %v, want applied_version %s and required_version %d",
					name, payload.Metadata, downgrade.applied, coordination.TableSchemaVersion)
			}
			if !strings.Contains(payload.Why, "up to "+downgrade.applied) {
				t.Errorf("%s: why = %q, want it to say which migration the database is at", name, payload.Why)
			}
			if !strings.Contains(strings.Join(payload.NextAction, " "), "mindrail init") {
				t.Errorf("%s does not send the reader to the command that clears the condition: %v",
					name, payload.NextAction)
			}
			if strings.Contains(payload.Why, "no such table") || strings.Contains(payload.Why, "no such column") {
				t.Errorf("%s publishes the raw SQL error as the whole cause:\n%s", name, got.stdout)
			}

			// The remedy works on this disk, which is the part the doctor remedy
			// could not do: init applies the missing migration and the same
			// command then succeeds on the same repository.
			if applied := run(t, repo, "init", "--json"); applied.code != app.ExitSuccess {
				t.Fatalf("%s: init exited %d on an upgraded repository: %s",
					name, applied.code, applied.stdout)
			}
			after := run(t, repo, append(slices.Clone(args), "--json")...)
			after.requireExit(t, app.ExitSuccess)
		}
	}
}

// TestAnUpgradedDatabaseGainsTheIndexSchemaWithoutLosingCoordination is
// MR-005 AC-01.5.
//
// The upgrade this milestone asks of every existing database is one migration:
// 000004 adds five tables and touches nothing MR-001 through MR-004 wrote. The
// fixture is that database — sessions, tasks and checkpoints with rows in
// them, the index tables and their ledger row removed — and the assertions are
// that init re-applies the migration without complaint and that the
// coordination rows it did not touch are still there.
func TestAnUpgradedDatabaseGainsTheIndexSchemaWithoutLosingCoordination(t *testing.T) {
	repo := newInitializedRepo(t)
	run(t, repo, "session", "open", "--json")
	opened := run(t, repo, "task", "open", "--title", "survives the upgrade", "--json")
	opened.requireExit(t, app.ExitSuccess)

	downgradeToSchemaThree(t, repo)

	applied := run(t, repo, "init", "--json")
	if applied.code != app.ExitSuccess {
		t.Fatalf("init exited %d over a schema-3 database: %s", applied.code, applied.stdout)
	}

	got := run(t, repo, "status", "--json")
	got.requireExit(t, app.ExitSuccess)
	var data struct {
		Runtime struct {
			SchemaVersion int64 `json:"schema_version"`
		} `json:"runtime"`
	}
	decodeData(t, got.stdout, &data)
	if data.Runtime.SchemaVersion != 10 {
		t.Errorf("schema_version = %d after init re-applied migrations 000004 through 000010, want 10", data.Runtime.SchemaVersion)
	}

	listed := run(t, repo, "task", "list", "--json")
	listed.requireExit(t, app.ExitSuccess)
	if !strings.Contains(listed.stdout, "survives the upgrade") {
		t.Errorf("the task written before the upgrade did not survive it:\n%s", listed.stdout)
	}
}

// TestAnUpgradedDatabaseGainsSymbolIdentityWithoutLosingSymbols is MR-006
// AC-01.2, still proving the 4→5 step under newer binaries: the fixture is
// a schema-4 database with indexed symbol rows, and init must bring it past
// 5 (now to 6) with those rows intact and the new column present but empty.
func TestAnUpgradedDatabaseGainsSymbolIdentityWithoutLosingSymbols(t *testing.T) {
	repo := newInitializedRepo(t)
	run(t, repo, "session", "open", "--json")
	pkg := filepath.Join(repo, "pkg")
	if err := os.MkdirAll(pkg, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pkg, "pyproject.toml"), []byte("[project]\nname = 'pkg'\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	execOnRuntimeDB(t, repo,
		`INSERT INTO project_units (id, path, kind, discovered_at) VALUES ('UNT-1', '`+repo+`/pkg', 'python', '2026-09-18T10:00:00Z')`,
		`INSERT INTO file_index_state (path, unit_id, language, state) VALUES ('`+repo+`/pkg/a.py', 'UNT-1', 'python', 'indexed')`,
		`INSERT INTO symbols (unit_id, path, logical_key, kind, name, start_line, start_col, end_line, end_col, signature_hash, body_hash, structure_hash)
			VALUES ('UNT-1', '`+repo+`/pkg/a.py', 'k', 'function', 'f', 1, 0, 2, 0, 's', 'b', 't')`)
	downgradeToSchemaFour(t, repo)

	applied := run(t, repo, "init", "--json")
	if applied.code != app.ExitSuccess {
		t.Fatalf("init exited %d over a schema-4 database: %s", applied.code, applied.stdout)
	}

	got := run(t, repo, "status", "--json")
	got.requireExit(t, app.ExitSuccess)
	var data struct {
		Runtime struct {
			SchemaVersion int64 `json:"schema_version"`
		} `json:"runtime"`
	}
	decodeData(t, got.stdout, &data)
	if data.Runtime.SchemaVersion != 10 {
		t.Errorf("schema_version = %d after init re-applied migrations 000005 through 000010, want 10", data.Runtime.SchemaVersion)
	}

	db, err := storage.Open(t.Context(), storage.Options{Path: runtimeDBPath(t, repo)})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	var symbols, uidNulls int
	if err := db.DB.QueryRowContext(t.Context(), `SELECT count(*) FROM symbols`).Scan(&symbols); err != nil {
		t.Fatal(err)
	}
	if err := db.DB.QueryRowContext(t.Context(), `SELECT count(*) FROM symbols WHERE symbol_uid IS NULL`).Scan(&uidNulls); err != nil {
		t.Fatal(err)
	}
	if symbols != 1 || uidNulls != 1 {
		t.Errorf("symbols carried %d rows (%d uid-less), want the MR-005 row intact and uid-less", symbols, uidNulls)
	}
}

// downgradeToSchemaFour removes what MR-006's through MR-008's migrations
// created, ledger rows included: the database a repository initialised by the
// MR-005 binary holds, which never saw migration 5, 6 or 7.
func downgradeToSchemaFour(t *testing.T, repo string) {
	t.Helper()

	dropGuardBaselines(t, repo)
	execOnRuntimeDB(t, repo,
		`DROP TABLE IF EXISTS evidence`,
		`DELETE FROM schema_migrations WHERE version = 8`,
		`DROP TABLE IF EXISTS scope_attributions`,
		`DELETE FROM schema_migrations WHERE version = 7`,
		`DROP INDEX IF EXISTS idx_changes_task_unique`,
		`DROP INDEX IF EXISTS idx_changes_operation`,
		`DROP TABLE IF EXISTS change_operations`,
		`DROP TABLE IF EXISTS change_baselines`,
		`DROP TABLE IF EXISTS change_symbols`,
		`DROP TABLE IF EXISTS change_files`,
		`DROP TABLE IF EXISTS changes`,
		`DELETE FROM schema_migrations WHERE version = 6`,
		`DROP INDEX IF EXISTS idx_identity_alloc`,
		`DROP INDEX IF EXISTS idx_identity_unit_key`,
		`DROP INDEX IF EXISTS idx_binding_pair`,
		`DROP TABLE IF EXISTS symbol_identity_ambiguities`,
		`DROP TABLE IF EXISTS invariant_symbol_bindings`,
		`DROP TABLE IF EXISTS symbol_identities`,
		`ALTER TABLE symbols DROP COLUMN symbol_uid`,
		`DELETE FROM schema_migrations WHERE version = 5`)
}

// TestAnUpgradedDatabaseGainsChangesWithoutLosingIdentities is MR-007
// AC-01.2. Migration 000006 adds five tables and touches nothing else: the
// fixture is a schema-5 database with an identity and a bound symbol, and
// init must bring it to 6 with both intact.
func TestAnUpgradedDatabaseGainsChangesWithoutLosingIdentities(t *testing.T) {
	repo := newInitializedRepo(t)
	run(t, repo, "session", "open", "--json")
	pkg := filepath.Join(repo, "pkg")
	if err := os.MkdirAll(pkg, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pkg, "pyproject.toml"), []byte("[project]\nname = 'pkg'\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	execOnRuntimeDB(t, repo,
		`INSERT INTO project_units (id, path, kind, discovered_at) VALUES ('UNT-1', '`+pkg+`', 'python', '2026-09-18T10:00:00Z')`,
		`INSERT INTO symbol_identities (symbol_uid, project_id, unit_id, language, logical_key, created_at)
			VALUES ('SYM-AAAAAAAAAAAAAAAAAAAAAAAAAA', 'PRJ-1', 'UNT-1', 'python', 'k', '2026-09-18T10:00:00Z')`,
		`INSERT INTO symbols (unit_id, path, logical_key, kind, name, start_line, start_col, end_line, end_col, signature_hash, body_hash, structure_hash, symbol_uid)
			VALUES ('UNT-1', '`+pkg+`/a.py', 'k', 'function', 'f', 1, 0, 2, 0, 's', 'b', 't', 'SYM-AAAAAAAAAAAAAAAAAAAAAAAAAA')`)
	downgradeToSchemaFive(t, repo)

	applied := run(t, repo, "init", "--json")
	if applied.code != app.ExitSuccess {
		t.Fatalf("init exited %d over a schema-5 database: %s", applied.code, applied.stdout)
	}

	got := run(t, repo, "status", "--json")
	got.requireExit(t, app.ExitSuccess)
	var data struct {
		Runtime struct {
			SchemaVersion int64 `json:"schema_version"`
		} `json:"runtime"`
	}
	decodeData(t, got.stdout, &data)
	if data.Runtime.SchemaVersion != 10 {
		t.Errorf("schema_version = %d after init re-applied migrations 000006 through 000010, want 10", data.Runtime.SchemaVersion)
	}

	db, err := storage.Open(t.Context(), storage.Options{Path: runtimeDBPath(t, repo)})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	var identities, bound int
	if err := db.DB.QueryRowContext(t.Context(), `SELECT count(*) FROM symbol_identities`).Scan(&identities); err != nil {
		t.Fatal(err)
	}
	if err := db.DB.QueryRowContext(t.Context(), `SELECT count(*) FROM symbols WHERE symbol_uid IS NOT NULL`).Scan(&bound); err != nil {
		t.Fatal(err)
	}
	if identities != 1 || bound != 1 {
		t.Errorf("identities %d bound symbols %d, want the MR-006 rows intact", identities, bound)
	}
}

// downgradeToSchemaFive removes what MR-007's and MR-008's migrations
// created, ledger rows included: the database a repository initialised by the
// MR-006 binary holds, which never saw migration 6 or 7.
func downgradeToSchemaFive(t *testing.T, repo string) {
	t.Helper()

	dropGuardBaselines(t, repo)
	execOnRuntimeDB(t, repo,
		`DROP INDEX IF EXISTS idx_changes_task_unique`,
		`DROP INDEX IF EXISTS idx_changes_operation`,
		`DROP TABLE IF EXISTS change_operations`,
		`DROP TABLE IF EXISTS change_baselines`,
		`DROP TABLE IF EXISTS change_symbols`,
		`DROP TABLE IF EXISTS change_files`,
		`DROP TABLE IF EXISTS changes`,
		`DELETE FROM schema_migrations WHERE version = 6`)
}

// TestAnUpgradedDatabaseGainsAttributionWithoutLosingChanges is MR-008
// AC-01.2. Migration 000007 adds one table and touches nothing else: the
// fixture is a schema-6 database with change rows, and init must bring it
// to 7 with those rows intact.
func TestAnUpgradedDatabaseGainsAttributionWithoutLosingChanges(t *testing.T) {
	repo := newInitializedRepo(t)
	run(t, repo, "session", "open", "--json")
	pkg := filepath.Join(repo, "pkg")
	if err := os.MkdirAll(pkg, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pkg, "pyproject.toml"), []byte("[project]\nname = 'pkg'\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	execOnRuntimeDB(t, repo,
		`INSERT INTO changes (change_id, created_at, updated_at) VALUES ('CHG-1', '2026-09-23T10:00:00Z', '2026-09-23T10:00:00Z')`,
		`INSERT INTO change_files (change_id, path, kind, discovered_via) VALUES ('CHG-1', '`+pkg+`/a.py', 'modified', 'reconcile')`)
	downgradeToSchemaSix(t, repo)

	applied := run(t, repo, "init", "--json")
	if applied.code != app.ExitSuccess {
		t.Fatalf("init exited %d over a schema-6 database: %s", applied.code, applied.stdout)
	}

	got := run(t, repo, "status", "--json")
	got.requireExit(t, app.ExitSuccess)
	var data struct {
		Runtime struct {
			SchemaVersion int64 `json:"schema_version"`
		} `json:"runtime"`
	}
	decodeData(t, got.stdout, &data)
	if data.Runtime.SchemaVersion != 10 {
		t.Errorf("schema_version = %d after init re-applied migrations 000007 through 000010, want 10", data.Runtime.SchemaVersion)
	}

	db, err := storage.Open(t.Context(), storage.Options{Path: runtimeDBPath(t, repo)})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	var files int
	if err := db.DB.QueryRowContext(t.Context(), `SELECT count(*) FROM change_files`).Scan(&files); err != nil || files != 1 {
		t.Fatalf("change files = %d, %v", files, err)
	}
}

// TestAnUpgradedDatabaseGainsEvidenceWithoutLosingChanges is MR-010
// AC-02.4. Migration 000008 adds one table and touches nothing else: the
// fixture is a schema-7 database with change rows, and init must bring it
// to 8 with those rows intact.
func TestAnUpgradedDatabaseGainsEvidenceWithoutLosingChanges(t *testing.T) {
	repo := newInitializedRepo(t)
	run(t, repo, "session", "open", "--json")
	execOnRuntimeDB(t, repo,
		`INSERT INTO changes (change_id, created_at, updated_at) VALUES ('CHG-1', '2026-09-23T10:00:00Z', '2026-09-23T10:00:00Z')`)
	downgradeToSchemaSeven(t, repo)

	applied := run(t, repo, "init", "--json")
	if applied.code != app.ExitSuccess {
		t.Fatalf("init exited %d over a schema-7 database: %s", applied.code, applied.stdout)
	}

	got := run(t, repo, "status", "--json")
	got.requireExit(t, app.ExitSuccess)
	var data struct {
		Runtime struct {
			SchemaVersion int64 `json:"schema_version"`
		} `json:"runtime"`
	}
	decodeData(t, got.stdout, &data)
	if data.Runtime.SchemaVersion != 10 {
		t.Errorf("schema_version = %d after init re-applied migrations 000008 through 000010, want 10", data.Runtime.SchemaVersion)
	}

	db, err := storage.Open(t.Context(), storage.Options{Path: runtimeDBPath(t, repo)})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	var changes int
	if err := db.DB.QueryRowContext(t.Context(), `SELECT count(*) FROM changes`).Scan(&changes); err != nil || changes != 1 {
		t.Fatalf("changes = %d, %v", changes, err)
	}
}

// downgradeToSchemaSeven removes what MR-010's migration created, ledger
// row included: the database a repository initialised by the MR-008 binary
// holds, which never saw migration 8.
func downgradeToSchemaSeven(t *testing.T, repo string) {
	t.Helper()

	dropGuardBaselines(t, repo)
	execOnRuntimeDB(t, repo,
		`DROP TABLE IF EXISTS evidence`,
		`DELETE FROM schema_migrations WHERE version = 8`)
}

// downgradeToSchemaSix removes what MR-008's migration created, ledger row
// included: the database a repository initialised by the MR-007 binary holds.
func downgradeToSchemaSix(t *testing.T, repo string) {
	t.Helper()

	dropGuardBaselines(t, repo)
	execOnRuntimeDB(t, repo,
		`DROP TABLE IF EXISTS evidence`,
		`DELETE FROM schema_migrations WHERE version = 8`,
		`DROP TABLE IF EXISTS scope_attributions`,
		`DELETE FROM schema_migrations WHERE version = 7`)
}

// downgradeToSchemaThree removes what MR-005's through MR-008's migrations
// created, ledger rows included: the database a repository initialised by the
// MR-004 binary holds, which never saw migration 4, 5, 6 or 7.
func downgradeToSchemaThree(t *testing.T, repo string) {
	t.Helper()

	dropGuardBaselines(t, repo)
	execOnRuntimeDB(t, repo,
		`DROP TABLE IF EXISTS evidence`,
		`DELETE FROM schema_migrations WHERE version = 8`,
		`DROP TABLE IF EXISTS scope_attributions`,
		`DELETE FROM schema_migrations WHERE version = 7`,
		`DROP INDEX IF EXISTS idx_changes_task_unique`,
		`DROP INDEX IF EXISTS idx_changes_operation`,
		`DROP TABLE IF EXISTS change_operations`,
		`DROP TABLE IF EXISTS change_baselines`,
		`DROP TABLE IF EXISTS change_symbols`,
		`DROP TABLE IF EXISTS change_files`,
		`DROP TABLE IF EXISTS changes`,
		`DELETE FROM schema_migrations WHERE version = 6`,
		`DROP INDEX IF EXISTS idx_identity_alloc`,
		`DROP INDEX IF EXISTS idx_identity_unit_key`,
		`DROP INDEX IF EXISTS idx_binding_pair`,
		`DROP TABLE IF EXISTS symbol_identity_ambiguities`,
		`DROP TABLE IF EXISTS invariant_symbol_bindings`,
		`DROP TABLE IF EXISTS symbol_identities`,
		`DROP INDEX IF EXISTS idx_refs_resolved`,
		`DROP INDEX IF EXISTS idx_refs_path`,
		`DROP INDEX IF EXISTS idx_imports_path`,
		`DROP INDEX IF EXISTS idx_symbols_path`,
		`DROP INDEX IF EXISTS idx_symbols_key`,
		`DROP INDEX IF EXISTS idx_file_state_unit`,
		`DROP TABLE IF EXISTS symbol_references`,
		`DROP TABLE IF EXISTS symbol_imports`,
		`DROP TABLE IF EXISTS symbols`,
		`DROP TABLE IF EXISTS file_index_state`,
		`DROP TABLE IF EXISTS project_units`,
		`DELETE FROM schema_migrations WHERE version = 5`,
		`DELETE FROM schema_migrations WHERE version = 4`)
}

// dropGuardBaselines restores a fixture to the schema before 000009. Every
// older-schema fixture calls it before removing its own later migrations, so
// the migration ledger and the actual object set stay coherent.
func dropGuardBaselines(t *testing.T, repo string) {
	t.Helper()
	execOnRuntimeDB(t, repo,
		`DROP TABLE IF EXISTS file_index_generations`,
		`DELETE FROM schema_migrations WHERE version = 10`,
		`DROP TABLE IF EXISTS guard_baselines`,
		`DELETE FROM schema_migrations WHERE version = 9`)
}

// downgradeToSchemaOne removes everything MR-003's and MR-004's migrations
// created, ledger rows included, which is the state a database written by the
// MR-002 binary is in.
func downgradeToSchemaOne(t *testing.T, repo string) {
	t.Helper()

	downgradeToSchemaTwo(t, repo)
	execOnRuntimeDB(t, repo,
		`DROP INDEX IF EXISTS idx_checkpoints_task`,
		`DROP INDEX IF EXISTS idx_tasks_project`,
		`DROP TABLE IF EXISTS checkpoints`,
		`DROP TABLE IF EXISTS tasks`,
		`DROP TABLE IF EXISTS sessions`,
		`DELETE FROM schema_migrations WHERE version = 2`)
}

// downgradeToSchemaTwo removes what MR-004's migration created and added,
// ledger row included: the database a repository initialised by the MR-003
// binary holds.
func downgradeToSchemaTwo(t *testing.T, repo string) {
	t.Helper()
	downgradeToSchemaThree(t, repo)

	execOnRuntimeDB(t, repo,
		`DROP INDEX IF EXISTS idx_leases_holder`,
		`DROP INDEX IF EXISTS idx_leases_active`,
		`DROP TABLE IF EXISTS leases`,
		`DROP TABLE IF EXISTS operations`,
		`ALTER TABLE tasks DROP COLUMN revision`,
		`DELETE FROM schema_migrations WHERE version = 3`)
}
