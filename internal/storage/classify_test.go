package storage_test

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/storage"
)

// shortBusy keeps the contention cases quick. The production budget is five
// seconds (decision D-08); a test that waited it out would spend that long
// proving a classification that is decided the moment BEGIN comes back.
const shortBusy = 50 * time.Millisecond

// contendedWriteError produces the error a second writer really gets: a genuine
// SQLITE_BUSY off the driver, not a hand-built one.
//
// Two handles in one process are enough. SQLite emulates its file locking
// between connections that share an inode, so the second BEGIN IMMEDIATE is
// refused exactly as it would be across processes -- and unlike the open-time
// contention in open_contention_test.go, this needs no re-exec because the lock
// is taken by a statement rather than during connection setup.
func contendedWriteError(t *testing.T) error {
	t.Helper()

	path := filepath.Join(t.TempDir(), "mindrail.db")
	holder := openForTest(t, path)
	if _, err := holder.ExecContext(t.Context(), `CREATE TABLE t (x INTEGER)`); err != nil {
		t.Fatalf("CREATE TABLE = %v, want no error", err)
	}

	tx, err := holder.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatalf("BeginTx = %v, want no error", err)
	}
	t.Cleanup(func() { _ = tx.Rollback() })
	if _, err := tx.ExecContext(t.Context(), `INSERT INTO t VALUES (1)`); err != nil {
		t.Fatalf("INSERT = %v, want no error", err)
	}

	loser := openForTest(t, path)
	err = storage.InTx(t.Context(), loser.DB, func(context.Context, *sql.Tx) error { return nil })
	if err == nil {
		t.Fatal("InTx against a held write lock = nil, want SQLITE_BUSY")
	}
	return err
}

func openForTest(t *testing.T, path string) *storage.DB {
	t.Helper()

	db, err := storage.Open(t.Context(), storage.Options{Path: path, BusyTimeout: shortBusy, MaxOpenConns: 1})
	if err != nil {
		t.Fatalf("storage.Open(%q) = %v, want no error", path, err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// readOnlyWriteError produces the error `mindrail init` got against a database
// whose mode bits refuse writes: SQLITE_READONLY, from a handle that opened
// perfectly and read back every pragma correctly.
func readOnlyWriteError(t *testing.T) (error, string) {
	t.Helper()
	requireModeBitsAreEnforced(t)

	path := initialisedDatabase(t)
	if err := os.Chmod(path, 0o444); err != nil {
		t.Fatalf("Chmod = %v, want no error", err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0o644) })

	db := openForTest(t, path)
	_, err := db.ExecContext(t.Context(), `INSERT INTO probe (x) VALUES (1)`)
	if err == nil {
		t.Fatal("INSERT into a read-only database = nil, want SQLITE_READONLY")
	}
	return err, path
}

// TestWriteFailureRatesContentionUnavailable is finding W1. Contention was
// indistinguishable from a broken write once a handle existed: isBusyError was
// consulted only at open, so a BEGIN IMMEDIATE that lost the race arrived at the
// caller as app.KindFailed and exit 1 -- "this operation is broken" -- for a
// condition decision D-03 rates exit 4 and a caller retries on.
func TestWriteFailureRatesContentionUnavailable(t *testing.T) {
	cause := contendedWriteError(t)

	if !storage.IsBusy(cause) {
		t.Fatalf("IsBusy(%v) = false, want true", cause)
	}

	err := storage.WriteFailure(t.Context(), nil, "do the thing", cause)
	if err == nil {
		t.Fatal("WriteFailure(SQLITE_BUSY) = nil, want a named condition")
	}
	if !errors.Is(err, storage.ErrBusy) {
		t.Errorf("WriteFailure = %v, want errors.Is(err, ErrBusy)", err)
	}

	payload := assertRetryablePayload(t, err, app.CodeRuntimeDBUnavailable)
	if strings.Contains(payload.Why, "database is locked (5)") {
		t.Errorf("payload.Why = %q, which hands the user the driver's words", payload.Why)
	}
}

// TestWriteFailureRatesAnUnwritableDatabaseUnavailable is the other half of the
// same gap, and the one whose remedy has to be able to succeed: the diagnosis it
// replaces sent the user to `mindrail doctor`, which reported the same database
// healthy, and then back to the `mindrail init` that had just failed.
func TestWriteFailureRatesAnUnwritableDatabaseUnavailable(t *testing.T) {
	cause, path := readOnlyWriteError(t)

	if !storage.IsReadOnly(cause) {
		t.Fatalf("IsReadOnly(%v) = false, want true", cause)
	}

	db := openForTest(t, path)
	err := storage.WriteFailure(t.Context(), db, "record this worktree", cause)
	if err == nil {
		t.Fatal("WriteFailure(SQLITE_READONLY) = nil, want a named condition")
	}
	if !errors.Is(err, storage.ErrReadOnly) {
		t.Errorf("WriteFailure = %v, want errors.Is(err, ErrReadOnly)", err)
	}

	payload := assertRetryablePayload(t, err, app.CodeRuntimePathUnwritable)
	remedy := strings.Join(payload.NextAction, " ")
	if !strings.Contains(remedy, path) {
		t.Errorf("NextAction = %q, want it to name %q; a remedy the user cannot locate is not one", payload.NextAction, path)
	}
	if strings.Contains(remedy, "mindrail doctor") {
		t.Errorf("NextAction = %q, which sends the user to the report that called this database healthy", payload.NextAction)
	}
}

// TestWriteFailureNamesNothingItCannotIdentify is the over-fire guard. The whole
// value of classifying on the result code is that everything else keeps its
// caller's own diagnosis: a migration whose SQL is wrong, a missing table, a
// cancelled context. A classifier that claimed those would replace a precise
// message with a vague one and move a real failure off exit 1.
func TestWriteFailureNamesNothingItCannotIdentify(t *testing.T) {
	db := openForTest(t, filepath.Join(t.TempDir(), "mindrail.db"))

	_, sqlErr := db.ExecContext(t.Context(), `INSERT INTO absent_table VALUES (1)`)
	if sqlErr == nil {
		t.Fatal("INSERT into a missing table = nil, want an error")
	}

	tests := []struct {
		name  string
		cause error
	}{
		{name: "a statement against a missing table", cause: sqlErr},
		{name: "a cancelled context", cause: context.Canceled},
		{name: "an expired deadline", cause: context.DeadlineExceeded},
		{name: "a plain error", cause: errors.New("something else went wrong")},
		{name: "no error at all", cause: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if storage.IsBusy(tt.cause) || storage.IsReadOnly(tt.cause) || storage.IsDiskFull(tt.cause) {
				t.Errorf("%v was classified as a database-level condition", tt.cause)
			}
			if got := storage.WriteFailure(t.Context(), db, "do the thing", tt.cause); got != nil {
				t.Errorf("WriteFailure(%v) = %v, want nil so the caller keeps its own diagnosis", tt.cause, got)
			}
		})
	}
}

// assertRetryablePayload states what a database-level write failure owes a
// reader: the stable code, the exit decision D-03 gives it, and a remedy.
func assertRetryablePayload(t *testing.T, err error, want app.Code) app.ErrorPayload {
	t.Helper()

	payload, ok := app.PayloadOf(err)
	if !ok {
		t.Fatalf("PayloadOf(%v) = _, false, want a domain payload", err)
	}
	if payload.Code != want {
		t.Errorf("payload.Code = %q, want %q", payload.Code, want)
	}
	if got := app.ExitCode(err); got != app.ExitUnavailable {
		t.Errorf("ExitCode = %d, want %d; decision D-03 rates this unavailable, not failed", got, app.ExitUnavailable)
	}
	if payload.Impact == "" {
		t.Error("payload.Impact is empty; a refusal owes the reader one")
	}
	if len(payload.NextAction) == 0 {
		t.Fatal("payload.NextAction is empty; there is nothing for the user to do")
	}
	return payload
}
