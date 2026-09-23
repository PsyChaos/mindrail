package validation_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/migration"
	"github.com/PsyChaos/mindrail/internal/storage"
	"github.com/PsyChaos/mindrail/internal/validation"
	"github.com/PsyChaos/mindrail/migrations"
)

type evidenceFixture struct {
	store *validation.Store
	db    *storage.DB
}

func newEvidenceFixture(t *testing.T) evidenceFixture {
	t.Helper()
	db, err := storage.Open(t.Context(), storage.Options{Path: filepath.Join(t.TempDir(), "mindrail.db")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	set, err := migration.Load(migrations.FS)
	if err != nil {
		t.Fatal(err)
	}
	clock := app.FixedClock{Instant: time.Date(2026, 9, 23, 10, 0, 0, 0, time.UTC)}
	if _, err := migration.New(db.DB, set, clock).Up(t.Context()); err != nil {
		t.Fatal(err)
	}
	store, err := validation.NewStore(db.DB, clock)
	if err != nil {
		t.Fatal(err)
	}
	return evidenceFixture{store: store, db: db}
}

// TestEvidenceSchemaVersionGate is TASK-02 AC-02.4 at the gate: the store
// answers to migration 000008 and nothing earlier.
func TestEvidenceSchemaVersionGate(t *testing.T) {
	if validation.TableSchemaVersion != 8 {
		t.Fatalf("version = %d, want 8", validation.TableSchemaVersion)
	}
	fx := newEvidenceFixture(t)
	redactor, err := validation.NewRedactor(nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fx.store.Record(t.Context(), "p", "BUILD", []string{"go", "build"},
		validation.Result{Status: validation.StatusPass}, "hash", "{}", "", redactor); err != nil {
		t.Fatalf("record on migrated schema: %v", err)
	}
}

func TestRecordBindsEvidenceRow(t *testing.T) {
	// The row binds profile, type, command, result, redacted output,
	// snapshot, provenance, timestamp and operation id together.
	fx := newEvidenceFixture(t)
	redactor, err := validation.NewRedactor(nil)
	if err != nil {
		t.Fatal(err)
	}
	got, err := fx.store.Record(t.Context(), "test", "AUTOMATED_TEST", []string{"echo", "hi"},
		validation.Result{Status: validation.StatusPass, ExitCode: 0, Stdout: "hi\n"},
		"snaphash", `{"profile":"test"}`, "OP-1", redactor)
	if err != nil {
		t.Fatal(err)
	}
	if got.Profile != "test" || got.Type != "AUTOMATED_TEST" || got.Status != validation.StatusPass ||
		got.SnapshotHash != "snaphash" || got.Provenance != `{"profile":"test"}` || got.OperationID != "OP-1" {
		t.Fatalf("record = %+v", got)
	}
	if len(got.Argv) != 2 || got.Argv[0] != "echo" {
		t.Fatalf("argv = %+v", got.Argv)
	}
	if got.Output != "hi\n" {
		t.Fatalf("output = %q", got.Output)
	}
	if got.ID == "" || got.CreatedAt == "" {
		t.Fatalf("identity/timestamp missing: %+v", got)
	}
}

// TestRecordRedactsArgvAndOutput pins the AC-24 boundary at storage time:
// secrets in argv and output both become [REDACTED], and the refusal table
// keeps malformed records out before any write.
func TestRecordRedactsArgvAndOutput(t *testing.T) {
	t.Setenv("MR010_STORE_SECRET", "leaked-secret-value")
	fx := newEvidenceFixture(t)
	redactor, err := validation.NewRedactor([]string{"MR010_STORE_SECRET"})
	if err != nil {
		t.Fatal(err)
	}
	got, err := fx.store.Record(t.Context(), "test", "AUTOMATED_TEST",
		[]string{"deploy", "leaked-secret-value"},
		validation.Result{Status: validation.StatusFail, ExitCode: 1, Stdout: "key leaked-secret-value\n"},
		"snap", "{}", "OP-RED", redactor)
	if err != nil {
		t.Fatal(err)
	}
	if got.Output != "key [REDACTED]\n" {
		t.Fatalf("output = %q", got.Output)
	}
	if len(got.Argv) != 2 || got.Argv[1] != validation.Redacted {
		t.Fatalf("argv = %+v", got.Argv)
	}
	for _, tc := range []struct {
		name     string
		profile  string
		typ      string
		argv     []string
		snap     string
		redactor *validation.Redactor
	}{
		{"no profile", "", "AUTOMATED_TEST", []string{"x"}, "s", redactor},
		{"no type", "p", "", []string{"x"}, "s", redactor},
		{"no argv", "p", "AUTOMATED_TEST", nil, "s", redactor},
		{"no snapshot", "p", "AUTOMATED_TEST", []string{"x"}, "", redactor},
		{"no redactor", "p", "AUTOMATED_TEST", []string{"x"}, "s", nil},
	} {
		if _, err := fx.store.Record(t.Context(), tc.profile, tc.typ, tc.argv,
			validation.Result{Status: validation.StatusPass}, tc.snap, "{}", "OP-"+tc.name, tc.redactor); err == nil {
			t.Fatalf("%s accepted", tc.name)
		}
	}
}

// TestOperationIDReplay is TASK-02 AC-02.4's idempotency: same id + same
// hash replays without a new row; same id + different hash conflicts.
func TestOperationIDReplay(t *testing.T) {
	fx := newEvidenceFixture(t)
	redactor, err := validation.NewRedactor(nil)
	if err != nil {
		t.Fatal(err)
	}
	first, err := fx.store.Record(t.Context(), "test", "BUILD", []string{"go", "build"},
		validation.Result{Status: validation.StatusPass}, "snap", "{}", "OP-9", redactor)
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := fx.store.Record(t.Context(), "test", "BUILD", []string{"go", "build"},
		validation.Result{Status: validation.StatusFail, ExitCode: 1}, "snap", "{}", "OP-9", redactor)
	if err != nil {
		t.Fatal(err)
	}
	if replayed.ID != first.ID || replayed.Status != validation.StatusPass {
		t.Fatalf("replay = %+v, want %+v", replayed, first)
	}
	if _, err := fx.store.Record(t.Context(), "test", "BUILD", []string{"go", "vet"},
		validation.Result{Status: validation.StatusPass}, "snap", "{}", "OP-9", redactor); err == nil {
		t.Fatal("conflicting retry accepted")
	}
}
