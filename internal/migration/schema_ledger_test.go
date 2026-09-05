package migration_test

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/migration"
	"github.com/PsyChaos/mindrail/internal/storage"
)

// This file covers the two ways the ledger stops describing the schema without
// the schema being damaged: the rows go missing (finding H2), and the table
// that holds them stops being readable (finding H3).
//
// Both used to end in a loop rather than a diagnosis. A ledger with no rows read
// as "not migrated yet", so `doctor` exited 0 calling the schema DEGRADED and
// prescribing `mindrail init`, while `init` failed on `table projects already
// exists` every time it ran and prescribed itself. An unreadable ledger reached
// the user in the driver's own words, under the same recommendation.

// driverText is the vocabulary of the layers underneath this package. None of it
// belongs in a sentence a user is asked to act on; all of it belongs in the
// cause, where --verbose prints it.
var driverText = []string{
	"SQL logic error",
	"driver.Value",
	"sql: Scan error",
	"parsing time",
	"invalid syntax",
	"no such column",
}

func assertUserFacing(t *testing.T, err error) app.ErrorPayload {
	t.Helper()
	return assertUserFacingCode(t, err, app.CodeMigrationFailed)
}

// assertUserFacingCode is the same contract with the code named by the caller.
// Most refusals in this package really are MIGRATION_FAILED, but not all of them
// are: a write the driver refused because the database cannot be written is a
// runtime-path condition, and asserting one code for every failure is what let
// that one be filed as a broken migration.
func assertUserFacingCode(t *testing.T, err error, want app.Code) app.ErrorPayload {
	t.Helper()

	payload, ok := app.PayloadOf(err)
	if !ok {
		t.Fatalf("PayloadOf(%v) = _, false, want a domain payload", err)
	}
	if payload.Code != want {
		t.Errorf("payload.Code = %q, want %q", payload.Code, want)
	}
	for _, fragment := range driverText {
		if strings.Contains(payload.Why, fragment) {
			t.Errorf("payload.Why = %q, which hands the user the driver's words (%q)", payload.Why, fragment)
		}
	}
	if payload.Impact == "" {
		t.Errorf("payload.Impact is empty; a non-OK reading owes the reader one")
	}
	if len(payload.NextAction) == 0 {
		t.Fatalf("payload.NextAction is empty; there is nothing for the user to do")
	}
	return payload
}

// assertRemedyIsNotTheFailingCommand rejects the loop both findings ended in: a
// remedy of "run `mindrail init`" for a condition in which `mindrail init` fails
// identically. Rebuilding the database is a different action and is allowed to
// mention init, because a rebuilt database is not the one that failed.
func assertRemedyIsNotTheFailingCommand(t *testing.T, payload app.ErrorPayload) {
	t.Helper()

	for _, action := range payload.NextAction {
		if strings.Contains(action, "mindrail init") && !strings.Contains(action, "aside") {
			t.Errorf("next action %q sends the user back to the command that just failed", action)
		}
	}
}

// TestStatusRefusesALedgerThatLostItsRows is finding H2.
func TestStatusRefusesALedgerThatLostItsRows(t *testing.T) {
	db := newDB(t)
	migrator := migration.New(db.DB, embeddedSet(t), fixedClock())

	if _, err := migrator.Up(t.Context()); err != nil {
		t.Fatalf("Up = %v, want no error", err)
	}
	if _, err := db.ExecContext(t.Context(), `DELETE FROM schema_migrations`); err != nil {
		t.Fatalf("DELETE FROM schema_migrations = %v, want no error", err)
	}

	_, err := migrator.Status(t.Context())
	if !errors.Is(err, migration.ErrSchemaBehindLedger) {
		t.Fatalf("Status = %v, want errors.Is(err, ErrSchemaBehindLedger)", err)
	}

	payload := assertUserFacing(t, err)
	assertRemedyIsNotTheFailingCommand(t, payload)
	for _, table := range []string{"projects", "workspaces"} {
		if !strings.Contains(payload.Why, table) {
			t.Errorf("payload.Why = %q, want it to name the standing table %q", payload.Why, table)
		}
	}
	if !strings.Contains(payload.Why, "000001_initial") {
		t.Errorf("payload.Why = %q, want it to name the migration the ledger lost", payload.Why)
	}

	// The point of the finding: every entry point has to agree, and `init` in
	// particular must stop before it writes rather than fail on a CREATE TABLE.
	if _, err := migrator.Pending(t.Context()); !errors.Is(err, migration.ErrSchemaBehindLedger) {
		t.Errorf("Pending = %v, want errors.Is(err, ErrSchemaBehindLedger)", err)
	}
	upErr := mustFailUp(t, migrator)
	if !errors.Is(upErr, migration.ErrSchemaBehindLedger) {
		t.Errorf("Up = %v, want errors.Is(err, ErrSchemaBehindLedger)", upErr)
	}
	if errors.Is(upErr, migration.ErrApplyFailed) {
		t.Errorf("Up = %v, which blames the migration for a ledger that lost its rows", upErr)
	}
}

// TestTheRemedyForALostLedgerRecovers is the half of H2 that a detection alone
// would not have fixed. The old remedy re-entered the failure; this one has to
// leave a working repository behind.
func TestTheRemedyForALostLedgerRecovers(t *testing.T) {
	dir := t.TempDir()
	broken := openDB(t, filepath.Join(dir, "mindrail.db"))
	set := embeddedSet(t)

	if _, err := migration.New(broken.DB, set, fixedClock()).Up(t.Context()); err != nil {
		t.Fatalf("Up = %v, want no error", err)
	}
	if _, err := broken.ExecContext(t.Context(), `DELETE FROM schema_migrations`); err != nil {
		t.Fatalf("DELETE FROM schema_migrations = %v, want no error", err)
	}

	err := mustFailUp(t, migration.New(broken.DB, set, fixedClock()))
	payload := assertUserFacing(t, err)
	if !strings.Contains(strings.Join(payload.NextAction, " "), "aside") {
		t.Fatalf("next actions %v, want the remedy to be moving the database aside", payload.NextAction)
	}

	// Carry the remedy out: the database is moved aside and init runs again.
	rebuilt := openDB(t, filepath.Join(dir, "rebuilt.db"))
	result, err := migration.New(rebuilt.DB, set, fixedClock()).Up(t.Context())
	if err != nil {
		t.Fatalf("Up after the remedy = %v, want no error; the remedy has to end the loop", err)
	}
	if len(result.Applied) != len(set) {
		t.Errorf("Up after the remedy applied %d migrations, want %d", len(result.Applied), len(set))
	}
	if _, err := migration.New(rebuilt.DB, set, fixedClock()).Status(t.Context()); err != nil {
		t.Errorf("Status after the remedy = %v, want a healthy repository", err)
	}
}

// TestStatusRefusesALedgerMissingOneRowOfSeveral keeps the detection from being
// a special case of "the ledger is empty".
func TestStatusRefusesALedgerMissingOneRowOfSeveral(t *testing.T) {
	db := newDB(t)
	set, err := migration.Load(sqlFS(map[string]string{
		"000001_initial.sql": "CREATE TABLE one (id TEXT PRIMARY KEY) STRICT;\n",
		"000002_second.sql":  "CREATE TABLE two (id TEXT PRIMARY KEY) STRICT;\n",
	}))
	if err != nil {
		t.Fatalf("Load = %v, want no error", err)
	}

	migrator := migration.New(db.DB, set, fixedClock())
	if _, err := migrator.Up(t.Context()); err != nil {
		t.Fatalf("Up = %v, want no error", err)
	}
	if _, err := db.ExecContext(t.Context(), `DELETE FROM schema_migrations WHERE version = 2`); err != nil {
		t.Fatalf("DELETE = %v, want no error", err)
	}

	_, err = migrator.Status(t.Context())
	if !errors.Is(err, migration.ErrSchemaBehindLedger) {
		t.Fatalf("Status = %v, want errors.Is(err, ErrSchemaBehindLedger)", err)
	}
	payload := assertUserFacing(t, err)
	if !strings.Contains(payload.Why, "000002_second") {
		t.Errorf("payload.Why = %q, want it to name migration 000002_second", payload.Why)
	}
	if strings.Contains(payload.Why, "one") && !strings.Contains(payload.Why, "two") {
		t.Errorf("payload.Why = %q, want it to name `two`, the table that actually collides", payload.Why)
	}
}

// --- over-fire guards -------------------------------------------------------

// TestStatusAcceptsAHealthyInitialisedRepository is the guard every detection in
// this package has to pass first. The schema `mindrail init` produces, read
// through every entry point, repeatedly, must be silent.
func TestStatusAcceptsAHealthyInitialisedRepository(t *testing.T) {
	db := newDB(t)
	migrator := migration.New(db.DB, embeddedSet(t), fixedClock())

	if _, err := migrator.Up(t.Context()); err != nil {
		t.Fatalf("Up = %v, want no error", err)
	}
	for i := range 3 {
		if _, err := migrator.Status(t.Context()); err != nil {
			t.Fatalf("Status #%d = %v, want no error on a freshly initialised database", i, err)
		}
		if _, err := migrator.Pending(t.Context()); err != nil {
			t.Fatalf("Pending #%d = %v, want no error", i, err)
		}
		if _, err := migrator.Up(t.Context()); err != nil {
			t.Fatalf("Up #%d = %v, want no error", i, err)
		}
	}
}

// TestStatusAcceptsADatabaseThatWasNeverInitialised is the other end of the same
// guard: a database with no ledger and no tables is a state, not damage.
func TestStatusAcceptsADatabaseThatWasNeverInitialised(t *testing.T) {
	db := newDB(t)
	migrator := migration.New(db.DB, embeddedSet(t), fixedClock())

	ledger, err := migrator.Status(t.Context())
	if err != nil {
		t.Fatalf("Status = %v, want no error on an empty database", err)
	}
	if len(ledger) != 0 {
		t.Errorf("Status = %v, want an empty ledger", ledger)
	}
	if _, err := migrator.Up(t.Context()); err != nil {
		t.Errorf("Up = %v, want the first init to work", err)
	}
}

// TestStatusAcceptsAnInterruptedFirstInit is the adjacent condition that looks
// most like the damage and is not: a process that created the ledger table and
// was killed before it applied anything. The ledger is empty for the same reason
// H2's is, and the tables its migrations create are simply not there yet.
func TestStatusAcceptsAnInterruptedFirstInit(t *testing.T) {
	db := newDB(t)
	if _, err := db.ExecContext(t.Context(), `CREATE TABLE schema_migrations (
    version    INTEGER PRIMARY KEY,
    name       TEXT NOT NULL,
    checksum   TEXT NOT NULL,
    applied_at TEXT NOT NULL
)`); err != nil {
		t.Fatalf("CREATE TABLE schema_migrations = %v, want no error", err)
	}

	migrator := migration.New(db.DB, embeddedSet(t), fixedClock())
	if _, err := migrator.Status(t.Context()); err != nil {
		t.Fatalf("Status = %v, want no error: nothing has been applied yet", err)
	}
	if _, err := migrator.Up(t.Context()); err != nil {
		t.Fatalf("Up = %v, want the interrupted init to finish", err)
	}
	if _, err := migrator.Status(t.Context()); err != nil {
		t.Errorf("Status after Up = %v, want a healthy repository", err)
	}
}

// TestStatusAcceptsAPendingRebuildOfAnExistingTable is the adjacent condition
// this detection must not trip on, and the one that would have made it the third
// bricking bug in a row.
//
// The 12-step rebuild recreates a table under the name it just dropped. Reading
// that create as "this table must not already exist" would refuse the upgrade
// over the very table the pending migration is about to drop itself -- turning a
// routine `mindrail init` after a binary upgrade into a permanent failure.
func TestStatusAcceptsAPendingRebuildOfAnExistingTable(t *testing.T) {
	rebuilds := map[string]string{
		"rename first": `ALTER TABLE thing RENAME TO thing_old;
CREATE TABLE thing (id TEXT PRIMARY KEY, tally INTEGER NOT NULL DEFAULT 0) STRICT;
INSERT INTO thing (id) SELECT id FROM thing_old;
DROP TABLE thing_old;
`,
		"new table first": `CREATE TABLE thing_new (id TEXT PRIMARY KEY, tally INTEGER NOT NULL DEFAULT 0) STRICT;
INSERT INTO thing_new (id) SELECT id FROM thing;
DROP TABLE thing;
ALTER TABLE thing_new RENAME TO thing;
`,
	}

	for name, rebuild := range rebuilds {
		t.Run(name, func(t *testing.T) {
			db := newDB(t)
			set, err := migration.Load(sqlFS(map[string]string{
				"000001_initial.sql": "CREATE TABLE thing (id TEXT PRIMARY KEY) STRICT;\n",
				"000002_rebuild.sql": rebuild,
			}))
			if err != nil {
				t.Fatalf("Load = %v, want no error", err)
			}

			if _, err := migration.New(db.DB, set[:1], fixedClock()).Up(t.Context()); err != nil {
				t.Fatalf("Up(first only) = %v, want no error", err)
			}

			upgraded := migration.New(db.DB, set, fixedClock())
			if _, err := upgraded.Status(t.Context()); err != nil {
				t.Fatalf("Status = %v, want no error: version 2 drops `thing` before recreating it", err)
			}
			pending, err := upgraded.Pending(t.Context())
			if err != nil {
				t.Fatalf("Pending = %v, want no error", err)
			}
			if len(pending) != 1 {
				t.Fatalf("Pending = %d migrations, want 1", len(pending))
			}
			if _, err := upgraded.Up(t.Context()); err != nil {
				t.Fatalf("Up = %v, want the upgrade to apply", err)
			}
		})
	}
}

// TestStatusAcceptsAPendingTolerantCreateOverAnExistingTable is the second
// adjacent condition. `CREATE TABLE IF NOT EXISTS` says an existing table is
// acceptable, so refusing one would block an init that would have succeeded.
func TestStatusAcceptsAPendingTolerantCreateOverAnExistingTable(t *testing.T) {
	db := newDB(t)
	set, err := migration.Load(sqlFS(map[string]string{
		"000001_initial.sql": "CREATE TABLE one (id TEXT PRIMARY KEY) STRICT;\n",
		"000002_tolerant.sql": "CREATE TABLE IF NOT EXISTS two (id TEXT PRIMARY KEY) STRICT;\n" +
			"CREATE INDEX IF NOT EXISTS idx_two ON two(id);\n",
	}))
	if err != nil {
		t.Fatalf("Load = %v, want no error", err)
	}

	if _, err := migration.New(db.DB, set[:1], fixedClock()).Up(t.Context()); err != nil {
		t.Fatalf("Up(first only) = %v, want no error", err)
	}
	if _, err := db.ExecContext(t.Context(), `CREATE TABLE two (id TEXT PRIMARY KEY) STRICT`); err != nil {
		t.Fatalf("CREATE TABLE two = %v, want no error", err)
	}

	upgraded := migration.New(db.DB, set, fixedClock())
	if _, err := upgraded.Status(t.Context()); err != nil {
		t.Fatalf("Status = %v, want no error: version 2 tolerates an existing `two`", err)
	}
	if _, err := upgraded.Up(t.Context()); err != nil {
		t.Fatalf("Up = %v, want the tolerant migration to apply", err)
	}
}

// TestStatusStaysReportableForANewerDatabase is the third, and the one that
// protects decision D-25.
//
// A database written by a newer Mindrail holds tables from migrations this
// binary cannot see, so its ledger legitimately records versions this binary
// does not know. Answering "your schema is ahead of its ledger, move the
// database aside" there would bury RUNTIME_DB_SCHEMA_TOO_NEW under a remedy that
// destroys a database a newer binary could still read.
func TestStatusStaysReportableForANewerDatabase(t *testing.T) {
	db := newDB(t)
	full, err := migration.Load(sqlFS(map[string]string{
		"000001_initial.sql": "CREATE TABLE one (id TEXT PRIMARY KEY) STRICT;\n",
		"000002_newer.sql":   "CREATE TABLE two (id TEXT PRIMARY KEY) STRICT;\n",
	}))
	if err != nil {
		t.Fatalf("Load = %v, want no error", err)
	}
	if _, err := migration.New(db.DB, full, fixedClock()).Up(t.Context()); err != nil {
		t.Fatalf("Up(newer binary) = %v, want no error", err)
	}

	older := migration.New(db.DB, full[:1], fixedClock())
	ledger, err := older.Status(t.Context())
	if err != nil {
		t.Fatalf("Status = %v, want no error: doctor has to be able to report a newer database", err)
	}
	if len(ledger) != 2 {
		t.Errorf("Status returned %d rows, want both of the newer database's rows", len(ledger))
	}

	// Up is where fail-closed belongs, and it must name the real condition.
	_, err = older.Up(t.Context())
	if !errors.Is(err, migration.ErrSchemaAhead) {
		t.Fatalf("Up = %v, want errors.Is(err, ErrSchemaAhead)", err)
	}
	if errors.Is(err, migration.ErrSchemaBehindLedger) {
		t.Errorf("Up = %v, which reads a newer database as a lost ledger", err)
	}
}

// TestStatusStaysReportableForAnEditedMigration is the same guard for the other
// unintelligible ledger. A migration file edited after it ran is a condition with
// its own diagnosis and its own remedy -- revert the edit -- and the tables it
// created are still standing, which must not be read as a lost ledger row.
func TestStatusStaysReportableForAnEditedMigration(t *testing.T) {
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

	if _, err := migration.New(db.DB, edited, fixedClock()).Status(t.Context()); err != nil {
		t.Fatalf("Status = %v, want no error: doctor has to be able to report this ledger", err)
	}
	_, err = migration.New(db.DB, edited, fixedClock()).Up(t.Context())
	if !errors.Is(err, migration.ErrChecksumMismatch) {
		t.Fatalf("Up = %v, want errors.Is(err, ErrChecksumMismatch)", err)
	}
	if errors.Is(err, migration.ErrSchemaBehindLedger) {
		t.Errorf("Up = %v, which reads an edited file as a lost ledger row", err)
	}
}

// --- finding H3: the driver's words ------------------------------------------

// TestAnUnreadableLedgerIsNotReportedInTheDriversWords covers the three ways
// reading the ledger can fail. Each of them reached the user verbatim, under a
// next_action recommending the command that had just failed the same way.
func TestAnUnreadableLedgerIsNotReportedInTheDriversWords(t *testing.T) {
	damage := map[string][]string{
		"a column is gone": {`ALTER TABLE schema_migrations DROP COLUMN checksum`},
		// The ledger's own shape is gone, so `version` no longer holds the
		// integer database/sql is asked to scan into.
		"a version is not a number": {
			`DROP TABLE schema_migrations`,
			`CREATE TABLE schema_migrations (version TEXT, name TEXT, checksum TEXT, applied_at TEXT)`,
			`INSERT INTO schema_migrations VALUES ('one', 'initial', 'deadbeef', '2026-01-01T00:00:00Z')`,
		},
		"a timestamp is not a time": {`UPDATE schema_migrations SET applied_at = 'yesterday'`},
	}

	for name, statements := range damage {
		t.Run(name, func(t *testing.T) {
			db := newDB(t)
			migrator := migration.New(db.DB, embeddedSet(t), fixedClock())
			if _, err := migrator.Up(t.Context()); err != nil {
				t.Fatalf("Up = %v, want no error", err)
			}
			for _, stmt := range statements {
				if _, err := db.ExecContext(t.Context(), stmt); err != nil {
					t.Fatalf("%s = %v, want no error", stmt, err)
				}
			}

			_, err := migrator.Status(t.Context())
			if err == nil {
				t.Fatalf("Status = nil, want an error: the ledger cannot be read")
			}

			payload := assertUserFacing(t, err)
			assertRemedyIsNotTheFailingCommand(t, payload)

			// The driver's text is not lost, only moved to where --verbose
			// prints it and a user is not asked to act on it.
			cause := app.CauseOf(err)
			if cause == nil {
				t.Fatalf("CauseOf(%v) = nil, want the driver error kept for --verbose", err)
			}
			if !strings.Contains(cause.Error(), "schema_migrations") {
				t.Errorf("cause = %q, want it to name the table it failed on", cause)
			}
		})
	}
}

// TestInitRefusesADatabaseWhereAnotherObjectOwnsTheLedgersName is the same loop
// one layer down: `CREATE TABLE IF NOT EXISTS schema_migrations` still fails when
// a view holds that name, and "check that the database is writable" sent the
// user to inspect permissions that were fine while init kept failing.
func TestInitRefusesADatabaseWhereAnotherObjectOwnsTheLedgersName(t *testing.T) {
	db := newDB(t)
	if _, err := db.ExecContext(t.Context(),
		`CREATE VIEW schema_migrations AS SELECT 1 AS version`); err != nil {
		t.Fatalf("CREATE VIEW = %v, want no error", err)
	}

	_, err := migration.New(db.DB, embeddedSet(t), fixedClock()).Up(t.Context())
	if !errors.Is(err, migration.ErrBookkeepingOccupied) {
		t.Fatalf("Up = %v, want errors.Is(err, ErrBookkeepingOccupied)", err)
	}

	payload := assertUserFacing(t, err)
	assertRemedyIsNotTheFailingCommand(t, payload)
	if !strings.Contains(payload.Why, "view") {
		t.Errorf("payload.Why = %q, want it to name what is standing in the way", payload.Why)
	}
}

// TestAnUnwritableDatabaseIsNotBlamedOnANameCollision is the over-fire guard for
// the diagnosis above: a create that failed for a reason other than the name
// being taken must not inherit that reading, because "move the database aside"
// is destructive advice for a database that is merely held open or read-only.
//
// It used to assert MIGRATION_FAILED, through a shared helper that assumed every
// refusal in this package was one. That was the finding, not the contract: a
// database the driver will not write to is an unwritable runtime path, which
// decision D-03 rates exit 4 -- the code a caller retries on -- and not the exit
// 1 that says the operation itself is broken. The assertion is now the corrected
// code plus the kind that produces that exit, which is strictly more than it
// pinned before.
func TestAnUnwritableDatabaseIsNotBlamedOnANameCollision(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mindrail.db")
	writable := openDB(t, path)
	if _, err := migration.New(writable.DB, embeddedSet(t), fixedClock()).Up(t.Context()); err != nil {
		t.Fatalf("Up = %v, want no error", err)
	}
	if _, err := writable.ExecContext(t.Context(), `DROP TABLE schema_migrations`); err != nil {
		t.Fatalf("DROP TABLE schema_migrations = %v, want no error", err)
	}

	readOnly, err := storage.Open(t.Context(), storage.Options{Path: path, ReadOnly: true})
	if err != nil {
		t.Fatalf("storage.Open(read-only) = %v, want no error", err)
	}
	t.Cleanup(func() { _ = readOnly.Close() })

	_, err = migration.New(readOnly.DB, embeddedSet(t), fixedClock()).Up(t.Context())
	if err == nil {
		t.Fatalf("Up on a read-only database = nil, want a failure")
	}
	if errors.Is(err, migration.ErrBookkeepingOccupied) {
		t.Fatalf("Up = %v, which blames a name collision for a database it merely cannot write", err)
	}
	payload := assertUserFacingCode(t, err, app.CodeRuntimePathUnwritable)
	if strings.Contains(strings.Join(payload.NextAction, " "), "aside") {
		t.Errorf("next actions %v tell the user to discard a database that is only unwritable", payload.NextAction)
	}
	if got := app.ExitCode(err); got != app.ExitUnavailable {
		t.Errorf("ExitCode = %d, want %d; decision D-03 rates an unwritable runtime path unavailable, not failed",
			got, app.ExitUnavailable)
	}
	if !errors.Is(err, storage.ErrReadOnly) {
		t.Errorf("Up = %v, want errors.Is(err, storage.ErrReadOnly)", err)
	}
}

// mustFailUp runs Up and requires it to refuse, so a call site that asserts on
// a refusal cannot silently assert on a success instead.
func mustFailUp(t *testing.T, migrator *migration.Migrator) error {
	t.Helper()

	_, err := migrator.Up(t.Context())
	if err == nil {
		t.Fatalf("Up = nil, want a refusal")
	}
	return err
}

// TestContentionOnTheWritePathIsRatedUnavailable is finding W1 at the layer that
// reports it. Every write in this package went through app.KindFailed
// unconditionally, so a database another `mindrail init` held for a few
// milliseconds came back as MIGRATION_FAILED at exit 1 -- "this operation is
// broken", with a remedy telling the user to inspect migration SQL that is
// perfectly good -- while decision D-03 puts a locked database at exit 4, the
// code a caller retries on.
//
// Both write points are covered: creating the ledger table, which is where the
// very first `init` takes the lock, and applying a migration, which is where
// every later one does.
func TestContentionOnTheWritePathIsRatedUnavailable(t *testing.T) {
	tests := []struct {
		name          string
		prepareLedger bool
	}{
		{name: "creating the bookkeeping table"},
		{name: "applying a migration", prepareLedger: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "mindrail.db")
			loser := openContendedDB(t, path)
			if tt.prepareLedger {
				if _, err := loser.ExecContext(t.Context(), `CREATE TABLE schema_migrations (
					version INTEGER PRIMARY KEY, name TEXT NOT NULL,
					checksum TEXT NOT NULL, applied_at TEXT NOT NULL)`); err != nil {
					t.Fatalf("CREATE TABLE = %v, want no error", err)
				}
			}

			holdWriteLock(t, path)

			_, err := migration.New(loser.DB, embeddedSet(t), fixedClock()).Up(t.Context())
			if err == nil {
				t.Fatal("Up against a held write lock = nil, want a refusal")
			}
			if !errors.Is(err, storage.ErrBusy) {
				t.Fatalf("Up = %v, want errors.Is(err, storage.ErrBusy)", err)
			}

			payload := assertUserFacingCode(t, err, app.CodeRuntimeDBUnavailable)
			if got := app.ExitCode(err); got != app.ExitUnavailable {
				t.Errorf("ExitCode = %d, want %d; a contended database is a wait, not a broken operation",
					got, app.ExitUnavailable)
			}
			if strings.Contains(strings.Join(payload.NextAction, " "), "migration SQL") {
				t.Errorf("NextAction = %q, which blames the migration for a lock somebody else holds",
					payload.NextAction)
			}
		})
	}
}

// TestAGenuineMigrationFailureIsStillAMigrationFailure is the over-fire guard
// for the classification above. Only the driver's own busy, read-only and
// disk-full codes may be re-rated; a migration whose SQL cannot run is a real
// operation failure and must keep MIGRATION_FAILED at exit 1, or the one exit
// code that means "your input is wrong" stops being reachable.
func TestAGenuineMigrationFailureIsStillAMigrationFailure(t *testing.T) {
	db := newDB(t)
	broken := []migration.Migration{{
		Version:  1,
		Name:     "broken",
		Checksum: "deadbeef",
		SQL:      `CREATE TABLE ( this is not sql`,
	}}

	_, err := migration.New(db.DB, broken, fixedClock()).Up(t.Context())
	if err == nil {
		t.Fatal("Up with unparseable SQL = nil, want a refusal")
	}
	if errors.Is(err, storage.ErrBusy) || errors.Is(err, storage.ErrReadOnly) {
		t.Fatalf("Up = %v, which blames the database for a migration that cannot parse", err)
	}

	assertUserFacingCode(t, err, app.CodeMigrationFailed)
	if got := app.ExitCode(err); got != app.ExitFailed {
		t.Errorf("ExitCode = %d, want %d; decision D-03 puts a migration failure at exit 1", got, app.ExitFailed)
	}
}

// openContendedDB opens a handle with a short busy timeout. The production
// budget is five seconds (decision D-08); a test that waited it out twice would
// spend ten seconds proving a classification decided at BEGIN.
func openContendedDB(t *testing.T, path string) *storage.DB {
	t.Helper()

	db, err := storage.Open(t.Context(), storage.Options{
		Path: path, BusyTimeout: 50 * time.Millisecond, MaxOpenConns: 1,
	})
	if err != nil {
		t.Fatalf("storage.Open(%q) = %v, want no error", path, err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// holdWriteLock takes the database's write lock on a second handle and keeps it
// for the rest of the test, which is what a concurrent `mindrail init` looks
// like from the loser's side.
func holdWriteLock(t *testing.T, path string) {
	t.Helper()

	holder := openContendedDB(t, path)
	tx, err := holder.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatalf("BeginTx = %v, want no error", err)
	}
	t.Cleanup(func() { _ = tx.Rollback() })

	if _, err := tx.ExecContext(t.Context(), `CREATE TABLE lock_holder (x INTEGER)`); err != nil {
		t.Fatalf("write inside the holding transaction = %v, want no error", err)
	}
}
