package migration_test

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"testing/fstest"
	"time"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/migration"
	"github.com/PsyChaos/mindrail/internal/storage"
	"github.com/PsyChaos/mindrail/migrations"
)

// fixedInstant is deliberately in a non-UTC zone: a stored timestamp that came
// out right anyway is proof the conversion happened rather than that the test
// machine happened to be on UTC.
var fixedInstant = time.Date(2026, time.September, 5, 9, 30, 15, 0, time.FixedZone("UTC+3", 3*60*60))

func fixedClock() app.Clock { return app.FixedClock{Instant: fixedInstant} }

func openDB(t *testing.T, path string) *storage.DB {
	t.Helper()

	db, err := storage.Open(t.Context(), storage.Options{Path: path})
	if err != nil {
		t.Fatalf("storage.Open(%q) = %v, want no error", path, err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func newDB(t *testing.T) *storage.DB {
	t.Helper()
	return openDB(t, filepath.Join(t.TempDir(), "mindrail.db"))
}

func embeddedSet(t *testing.T) []migration.Migration {
	t.Helper()

	set, err := migration.Load(migrations.FS)
	if err != nil {
		t.Fatalf("Load(migrations.FS) = %v, want no error", err)
	}
	return set
}

func tableExists(t *testing.T, db *storage.DB, name string) bool {
	t.Helper()

	var count int
	err := db.QueryRowContext(t.Context(),
		`SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, name).Scan(&count)
	if err != nil {
		t.Fatalf("sqlite_master lookup for %q = %v, want no error", name, err)
	}
	return count > 0
}

func TestUpAppliesAllOnFreshDatabase(t *testing.T) {
	db := newDB(t)
	set := embeddedSet(t)

	result, err := migration.New(db.DB, set, fixedClock()).Up(t.Context())
	if err != nil {
		t.Fatalf("Up = %v, want no error", err)
	}

	if len(result.Applied) != len(set) {
		t.Errorf("Up applied %d migrations, want %d", len(result.Applied), len(set))
	}
	if result.Total != len(set) {
		t.Errorf("Total = %d, want %d", result.Total, len(set))
	}
	if want := set[len(set)-1].Version; result.CurrentVersion != want {
		t.Errorf("CurrentVersion = %d, want %d", result.CurrentVersion, want)
	}

	var rows int
	if err := db.QueryRowContext(t.Context(), `SELECT count(*) FROM schema_migrations`).Scan(&rows); err != nil {
		t.Fatalf("count schema_migrations = %v, want no error", err)
	}
	if rows != len(set) {
		t.Errorf("schema_migrations has %d rows, want one per embedded file (%d)", rows, len(set))
	}

	for _, table := range []string{"projects", "workspaces"} {
		if !tableExists(t, db, table) {
			t.Errorf("table %q missing after Up", table)
		}
	}
}

// TestUpIsIdempotentOnSecondRun is the acceptance criterion that `init` may be
// run twice: the second run must apply nothing and must not rewrite the rows
// the first one left, applied_at included.
func TestUpIsIdempotentOnSecondRun(t *testing.T) {
	db := newDB(t)
	set := embeddedSet(t)

	first := migration.New(db.DB, set, fixedClock())
	if _, err := first.Up(t.Context()); err != nil {
		t.Fatalf("first Up = %v, want no error", err)
	}

	// A row nothing in the migration set would ever produce. If the second run
	// re-applied 000001, the CREATE TABLE would fail; if it dropped and
	// recreated, this row would vanish.
	if _, err := db.ExecContext(t.Context(),
		`INSERT INTO projects (project_id, common_dir, registered_at) VALUES (?, ?, ?)`,
		"PRJ-SENTINEL", "/sentinel/.git", app.FormatTime(fixedInstant)); err != nil {
		t.Fatalf("sentinel INSERT = %v, want no error", err)
	}

	before, err := first.Status(t.Context())
	if err != nil {
		t.Fatalf("Status = %v, want no error", err)
	}

	// A different clock: a re-applied row would carry the new instant.
	later := app.FixedClock{Instant: fixedInstant.Add(48 * time.Hour)}
	second := migration.New(db.DB, set, later)

	result, err := second.Up(t.Context())
	if err != nil {
		t.Fatalf("second Up = %v, want no error", err)
	}
	if len(result.Applied) != 0 {
		t.Errorf("second Up applied %d migrations, want 0", len(result.Applied))
	}
	if want := set[len(set)-1].Version; result.CurrentVersion != want {
		t.Errorf("CurrentVersion = %d, want %d", result.CurrentVersion, want)
	}

	after, err := second.Status(t.Context())
	if err != nil {
		t.Fatalf("Status = %v, want no error", err)
	}
	if len(before) != len(after) {
		t.Fatalf("row count changed from %d to %d", len(before), len(after))
	}
	for i := range before {
		if before[i].Version != after[i].Version ||
			before[i].Name != after[i].Name ||
			before[i].Checksum != after[i].Checksum ||
			!before[i].AppliedAt.Equal(after[i].AppliedAt) {
			t.Errorf("row %d changed: %+v -> %+v", i, before[i], after[i])
		}
	}

	var sentinels int
	if err := db.QueryRowContext(t.Context(),
		`SELECT count(*) FROM projects WHERE project_id = ?`, "PRJ-SENTINEL").Scan(&sentinels); err != nil {
		t.Fatalf("sentinel lookup = %v, want no error", err)
	}
	if sentinels != 1 {
		t.Errorf("sentinel row count = %d, want 1; the second run rebuilt the schema", sentinels)
	}
}

// TestUpFailsFastAndRecordsNothing proves the per-migration transaction of
// decision D-24 really wraps both the DDL and its bookkeeping row: a partially
// applied migration that still recorded itself would be undetectable later.
func TestUpFailsFastAndRecordsNothing(t *testing.T) {
	db := newDB(t)
	set, err := migration.Load(fstest.MapFS{
		"000001_first.sql":  &fstest.MapFile{Data: []byte(`CREATE TABLE alpha (id TEXT PRIMARY KEY) STRICT;`)},
		"000002_broken.sql": &fstest.MapFile{Data: []byte("CREATE TABLE beta (id TEXT PRIMARY KEY) STRICT;\nTHIS IS NOT SQL;\n")},
	})
	if err != nil {
		t.Fatalf("Load = %v, want no error", err)
	}

	result, err := migration.New(db.DB, set, fixedClock()).Up(t.Context())
	if err == nil {
		t.Fatal("Up(broken set) = nil error, want ErrApplyFailed")
	}
	if !errors.Is(err, migration.ErrApplyFailed) {
		t.Errorf("Up = %v, want errors.Is(err, ErrApplyFailed)", err)
	}

	payload, ok := app.PayloadOf(err)
	if !ok || payload.Code != app.CodeMigrationFailed {
		t.Errorf("payload = %+v (ok=%v), want %q", payload, ok, app.CodeMigrationFailed)
	}
	if app.ExitCode(err) != app.ExitFailed {
		t.Errorf("ExitCode = %d, want %d (decision D-03)", app.ExitCode(err), app.ExitFailed)
	}

	// The migration that did succeed stays applied: fail-fast stops, it does
	// not unwind.
	if len(result.Applied) != 1 || result.Applied[0].Version != 1 {
		t.Errorf("Applied = %+v, want exactly version 1", result.Applied)
	}
	if !tableExists(t, db, "alpha") {
		t.Error("table alpha missing; the first migration should have committed")
	}
	if tableExists(t, db, "beta") {
		t.Error("table beta exists; the failed migration was not rolled back")
	}

	var recorded int
	if err := db.QueryRowContext(t.Context(),
		`SELECT count(*) FROM schema_migrations WHERE version = 2`).Scan(&recorded); err != nil {
		t.Fatalf("schema_migrations lookup = %v, want no error", err)
	}
	if recorded != 0 {
		t.Errorf("schema_migrations records version 2 %d times, want 0", recorded)
	}
}

// TestChecksumMismatchIsRejected covers the realistic MR-002..MR-020 mistake:
// editing 000001 instead of adding 000002. Without the checksum the edited
// statements would simply never run and the divergence would be invisible.
func TestChecksumMismatchIsRejected(t *testing.T) {
	db := newDB(t)

	original, err := migration.Load(fstest.MapFS{
		"000001_initial.sql": &fstest.MapFile{Data: []byte(`CREATE TABLE alpha (id TEXT PRIMARY KEY) STRICT;`)},
	})
	if err != nil {
		t.Fatalf("Load = %v, want no error", err)
	}
	if _, err := migration.New(db.DB, original, fixedClock()).Up(t.Context()); err != nil {
		t.Fatalf("Up = %v, want no error", err)
	}

	edited, err := migration.Load(fstest.MapFS{
		"000001_initial.sql": &fstest.MapFile{
			Data: []byte("CREATE TABLE alpha (id TEXT PRIMARY KEY, extra TEXT) STRICT;"),
		},
	})
	if err != nil {
		t.Fatalf("Load(edited) = %v, want no error", err)
	}

	_, err = migration.New(db.DB, edited, fixedClock()).Up(t.Context())
	if err == nil {
		t.Fatal("Up(edited set) = nil error, want ErrChecksumMismatch")
	}
	if !errors.Is(err, migration.ErrChecksumMismatch) {
		t.Errorf("Up = %v, want errors.Is(err, ErrChecksumMismatch)", err)
	}
	if payload, ok := app.PayloadOf(err); !ok || payload.Code != app.CodeMigrationChecksumMismatch {
		t.Errorf("payload = %+v (ok=%v), want %q", payload, ok, app.CodeMigrationChecksumMismatch)
	}
	if app.ExitCode(err) != app.ExitFailed {
		t.Errorf("ExitCode = %d, want %d (decision D-03)", app.ExitCode(err), app.ExitFailed)
	}
}

// TestSchemaAheadFailsClosed is decision D-25: an older binary meeting a newer
// database stops instead of guessing, because guessing means writing rows a
// schema it cannot see has constraints on.
func TestSchemaAheadFailsClosed(t *testing.T) {
	db := newDB(t)
	set := embeddedSet(t)

	if _, err := migration.New(db.DB, set, fixedClock()).Up(t.Context()); err != nil {
		t.Fatalf("Up = %v, want no error", err)
	}

	// A version written by a future release of the binary.
	if _, err := db.ExecContext(t.Context(),
		`INSERT INTO schema_migrations (version, name, checksum, applied_at) VALUES (?, ?, ?, ?)`,
		999, "from_the_future", "deadbeef", app.FormatTime(fixedInstant)); err != nil {
		t.Fatalf("future row INSERT = %v, want no error", err)
	}

	_, err := migration.New(db.DB, set, fixedClock()).Up(t.Context())
	if err == nil {
		t.Fatal("Up(schema ahead) = nil error, want ErrSchemaAhead")
	}
	if !errors.Is(err, migration.ErrSchemaAhead) {
		t.Errorf("Up = %v, want errors.Is(err, ErrSchemaAhead)", err)
	}
	if payload, ok := app.PayloadOf(err); !ok || payload.Code != app.CodeRuntimeDBSchemaTooNew {
		t.Errorf("payload = %+v (ok=%v), want %q", payload, ok, app.CodeRuntimeDBSchemaTooNew)
	}
	if app.ExitCode(err) != app.ExitFailed {
		t.Errorf("ExitCode = %d, want %d (decision D-03)", app.ExitCode(err), app.ExitFailed)
	}

	// Status still reports, so doctor can explain what it found.
	applied, err := migration.New(db.DB, set, fixedClock()).Status(t.Context())
	if err != nil {
		t.Fatalf("Status = %v, want no error", err)
	}
	if applied[len(applied)-1].Version != 999 {
		t.Errorf("Status highest version = %d, want 999", applied[len(applied)-1].Version)
	}
}

func TestAppliedAtIsUTCRFC3339(t *testing.T) {
	db := newDB(t)
	set := embeddedSet(t)

	if _, err := migration.New(db.DB, set, fixedClock()).Up(t.Context()); err != nil {
		t.Fatalf("Up = %v, want no error", err)
	}

	var stored string
	if err := db.QueryRowContext(t.Context(),
		`SELECT applied_at FROM schema_migrations WHERE version = 1`).Scan(&stored); err != nil {
		t.Fatalf("read applied_at = %v, want no error", err)
	}

	want := app.FormatTime(fixedInstant)
	if stored != want {
		t.Errorf("stored applied_at = %q, want %q", stored, want)
	}
	if stored[len(stored)-1] != 'Z' {
		t.Errorf("stored applied_at = %q, want a UTC (Z) offset", stored)
	}

	applied, err := migration.New(db.DB, set, fixedClock()).Status(t.Context())
	if err != nil {
		t.Fatalf("Status = %v, want no error", err)
	}
	if !applied[0].AppliedAt.Equal(fixedInstant) {
		t.Errorf("AppliedAt = %v, want %v", applied[0].AppliedAt, fixedInstant.UTC())
	}
	if applied[0].AppliedAt.Location() != time.UTC {
		t.Errorf("AppliedAt location = %v, want UTC", applied[0].AppliedAt.Location())
	}
}

func TestPendingReportsUnappliedMigrations(t *testing.T) {
	db := newDB(t)
	set := embeddedSet(t)
	migrator := migration.New(db.DB, set, fixedClock())

	pending, err := migrator.Pending(t.Context())
	if err != nil {
		t.Fatalf("Pending(before Up) = %v, want no error", err)
	}
	if len(pending) != len(set) {
		t.Errorf("Pending(before Up) = %d migrations, want %d", len(pending), len(set))
	}

	if _, err := migrator.Up(t.Context()); err != nil {
		t.Fatalf("Up = %v, want no error", err)
	}

	pending, err = migrator.Pending(t.Context())
	if err != nil {
		t.Fatalf("Pending(after Up) = %v, want no error", err)
	}
	if len(pending) != 0 {
		t.Errorf("Pending(after Up) = %d migrations, want 0", len(pending))
	}
}

func TestStatusOnUnmigratedDatabaseIsEmpty(t *testing.T) {
	db := newDB(t)

	applied, err := migration.New(db.DB, embeddedSet(t), fixedClock()).Status(t.Context())
	if err != nil {
		t.Fatalf("Status = %v, want no error; an absent table is not a failure", err)
	}
	if len(applied) != 0 {
		t.Errorf("Status = %d rows, want 0", len(applied))
	}
}

// TestUpRunsOutsideACallerTransaction documents that Up owns its own
// transactions; handing it one would break the BEGIN IMMEDIATE guarantee.
func TestUpRunsOutsideACallerTransaction(t *testing.T) {
	db := newDB(t)

	err := storage.InTx(t.Context(), db.DB, func(ctx context.Context, _ *sql.Tx) error {
		if !storage.TxActive(ctx) {
			t.Error("TxActive inside InTx = false, want true")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("InTx = %v, want no error", err)
	}

	if _, err := migration.New(db.DB, embeddedSet(t), fixedClock()).Up(t.Context()); err != nil {
		t.Fatalf("Up = %v, want no error", err)
	}
	if storage.TxActive(t.Context()) {
		t.Error("TxActive after Up = true, want false")
	}
}
