package cli_test

import (
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/coordination"
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

	execOnRuntimeDB(t, repo,
		`DROP INDEX IF EXISTS idx_leases_holder`,
		`DROP INDEX IF EXISTS idx_leases_active`,
		`DROP TABLE IF EXISTS leases`,
		`DROP TABLE IF EXISTS operations`,
		`ALTER TABLE tasks DROP COLUMN revision`,
		`DELETE FROM schema_migrations WHERE version = 3`)
}
