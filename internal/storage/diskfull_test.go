package storage_test

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/storage"
)

// fullDatabaseWriteError produces a genuine SQLITE_FULL off the driver, on a
// filesystem with plenty of room.
//
// `PRAGMA max_page_count` is SQLite's own ceiling on how many pages a database
// may occupy, and a write that would cross it fails with exactly the result code
// a write that ran out of disk fails with -- SQLITE_FULL, 13, "database or disk
// is full". That is the point: the classification under test is made on the
// result code, so a test that can produce the real code without a real full disk
// tests the real thing, and can do it in the ordinary suite on any machine.
//
// It does not replace the full-filesystem test beside it. That one proves the
// driver still emits this code for the condition users actually hit; this one
// proves the classification survives being deleted (finding D3, a mutation that
// left the whole suite green).
func fullDatabaseWriteError(t *testing.T) (error, string) {
	t.Helper()

	path := filepath.Join(t.TempDir(), "mindrail.db")
	db, err := storage.Open(t.Context(), storage.Options{Path: path, MaxOpenConns: 1})
	if err != nil {
		t.Fatalf("storage.Open = %v, want no error", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if _, err := db.ExecContext(t.Context(), `CREATE TABLE probe (x BLOB)`); err != nil {
		t.Fatalf("CREATE TABLE = %v, want no error", err)
	}
	// One connection, so the pragma is in force for the write below.
	if _, err := db.ExecContext(t.Context(), `PRAGMA max_page_count = 4`); err != nil {
		t.Fatalf("PRAGMA max_page_count = %v, want no error", err)
	}

	for range 64 {
		if _, err := db.ExecContext(t.Context(),
			`INSERT INTO probe (x) VALUES (zeroblob(65536))`); err != nil {
			return err, path
		}
	}
	t.Fatal("64 inserts past max_page_count = no error, want SQLITE_FULL")
	return nil, path
}

// TestIsDiskFullRecognisesTheDriversOwnFullError is the test finding D3 says was
// missing: change isDiskFullError to `return false` and this fails.
//
// It is deliberately the plainest possible assertion on the plainest possible
// input. A classification whose only coverage is a scenario that needs a mounted
// filesystem to reproduce is a classification that can be deleted in silence,
// which is what happened.
func TestIsDiskFullRecognisesTheDriversOwnFullError(t *testing.T) {
	cause, _ := fullDatabaseWriteError(t)

	if !storage.IsDiskFull(cause) {
		t.Fatalf("IsDiskFull(%v) = false, want true; this is SQLITE_FULL off the driver", cause)
	}
	if storage.IsReadOnly(cause) || storage.IsBusy(cause) {
		t.Errorf("%v was also classified as read-only or busy; the three conditions have different remedies", cause)
	}
}

// TestWriteFailureRatesAFullDiskUnavailableWithASpaceRemedy is the remedy half.
// Rating the condition correctly is worth nothing if the sentence the user reads
// still tells them to change a permission that is already correct.
func TestWriteFailureRatesAFullDiskUnavailableWithASpaceRemedy(t *testing.T) {
	cause, path := fullDatabaseWriteError(t)

	db, err := storage.Open(t.Context(), storage.Options{Path: path, MaxOpenConns: 1})
	if err != nil {
		t.Fatalf("storage.Open = %v, want no error", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	failure := storage.WriteFailure(t.Context(), db, "record this worktree", cause)
	if failure == nil {
		t.Fatal("WriteFailure(SQLITE_FULL) = nil, want a named condition")
	}
	if !errors.Is(failure, storage.ErrDiskFull) {
		t.Errorf("WriteFailure = %v, want errors.Is(err, ErrDiskFull)", failure)
	}

	payload := assertRetryablePayload(t, failure, app.CodeRuntimePathUnwritable)
	remedy := strings.Join(payload.NextAction, " ")
	if !strings.Contains(remedy, "free space") {
		t.Errorf("NextAction = %q, want it to ask for space", payload.NextAction)
	}
	if !strings.Contains(remedy, path) {
		t.Errorf("NextAction = %q, want it to name %q so the user knows which filesystem", payload.NextAction, path)
	}
	if strings.Contains(remedy, "permission") || strings.Contains(remedy, "chmod") {
		t.Errorf("NextAction = %q, which prescribes a permission change for a full disk", payload.NextAction)
	}
}

// TestIsDiskFullIgnoresTheConditionsBesideIt is the over-fire guard for the
// classification. The two failures that arrive at the same call site and mean
// something else must keep their own remedies: contention clears on its own, and
// a read-only database is fixed by a chmod that would do nothing for a full
// disk.
func TestIsDiskFullIgnoresTheConditionsBesideIt(t *testing.T) {
	t.Run("a database whose mode bits refuse writes", func(t *testing.T) {
		cause, _ := readOnlyWriteError(t)
		if storage.IsDiskFull(cause) {
			t.Errorf("IsDiskFull(SQLITE_READONLY) = true; a chmod would be prescribed as free space")
		}
	})

	t.Run("another process holding the write lock", func(t *testing.T) {
		cause := contendedWriteError(t)
		if storage.IsDiskFull(cause) {
			t.Errorf("IsDiskFull(SQLITE_BUSY) = true; a retryable condition would be reported as a full disk")
		}
	})
}

// TestProbeWriteAccessPassesADatabaseAtItsOwnPageLimit is the over-fire guard
// for the *probe*, against the closest thing to a full disk that is not one.
//
// This database cannot take another byte: max_page_count refuses the write with
// the same SQLITE_FULL a full filesystem produces. The filesystem underneath it
// has room, though, and ProbeWriteAccess answers about the filesystem -- so it
// must still say writable. A probe that reported this one unwritable would fail
// every repository whose database happens to be at a ceiling somebody set.
func TestProbeWriteAccessPassesADatabaseAtItsOwnPageLimit(t *testing.T) {
	_, path := fullDatabaseWriteError(t)

	if got := storage.ProbeWriteAccess(path); !got.Writable {
		t.Errorf("ProbeWriteAccess = %+v, want Writable; the disk under this database is not full", got)
	}
}
