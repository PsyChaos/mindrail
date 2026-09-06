package storage_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/PsyChaos/mindrail/internal/storage"
)

// TestCheckpointTruncatesTheLogItWroteBack is the assertion the F01 fix rests on
// and did not have.
//
// storage.Checkpoint's own comment calls TRUNCATE load-bearing — "a passive
// checkpoint leaves the log at its full length, which is the 74 KiB that decided
// the reproduction" — and swapping TRUNCATE for PASSIVE passed the entire suite.
// The space accounting is the whole point of the call: init's verdict is taken
// after it, over a filesystem whose free space has to be what init will actually
// leave behind.
func TestCheckpointTruncatesTheLogItWroteBack(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mindrail.db")
	db, err := storage.Open(t.Context(), storage.Options{Path: path})
	if err != nil {
		t.Fatalf("storage.Open = %v, want no error", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if _, err := db.ExecContext(t.Context(), `CREATE TABLE probe (x BLOB)`); err != nil {
		t.Fatalf("CREATE TABLE = %v, want no error", err)
	}
	for range 64 {
		if _, err := db.ExecContext(t.Context(), `INSERT INTO probe (x) VALUES (zeroblob(4096))`); err != nil {
			t.Fatalf("INSERT = %v, want no error", err)
		}
	}

	before := fileSize(t, path+"-wal")
	if before == 0 {
		t.Fatal("the write-ahead log is empty, so this proves nothing about truncating it")
	}

	result, err := storage.Checkpoint(t.Context(), db.DB)
	if err != nil {
		t.Fatalf("Checkpoint = %v, want no error", err)
	}
	if result.Busy {
		t.Fatal("Checkpoint reported busy with no other connection reading")
	}

	// The frame counters are not asserted: a successful TRUNCATE reports the
	// state it left behind, which is an empty log, so both come back zero. The
	// file on disk is the evidence, and it is the thing the F01 fix is about.
	if after := fileSize(t, path+"-wal"); after != 0 {
		t.Errorf("the write-ahead log is %d bytes after the checkpoint, want 0; "+
			"the space init reports as free is the space this call returns", after)
	}
}

// TestCheckpointGivesUpOnAHeldDatabaseRatherThanWaitingForIt covers the two
// outcomes that are not failures, both of which were unreachable from any test.
//
// A checkpoint cannot have a database another connection is reading, and that is
// nobody's fault: the log stays where it is and the next writer moves it. Two
// things were wrong before. The wait was the database's own five-second busy
// timeout, so one held read lock took `mindrail init` from about ten
// milliseconds to five seconds. And a checkpoint whose caller pressed Ctrl-C was
// reported as a database failure, so the command fabricated a fault its own
// doctor checks then reported as healthy.
func TestCheckpointGivesUpOnAHeldDatabaseRatherThanWaitingForIt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mindrail.db")
	db, err := storage.Open(t.Context(), storage.Options{Path: path})
	if err != nil {
		t.Fatalf("storage.Open = %v, want no error", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if _, err := db.ExecContext(t.Context(), `CREATE TABLE probe (x INTEGER)`); err != nil {
		t.Fatalf("CREATE TABLE = %v, want no error", err)
	}

	t.Run("a reader holding the database is reported, not waited out", func(t *testing.T) {
		// Written first, so the log has frames a checkpoint would want to move
		// and the contention below is over something real.
		for range 8 {
			if _, err := db.ExecContext(t.Context(), `INSERT INTO probe (x) VALUES (1)`); err != nil {
				t.Fatalf("INSERT = %v, want no error", err)
			}
		}

		reader, err := storage.Open(t.Context(), storage.Options{Path: path})
		if err != nil {
			t.Fatalf("open a second connection = %v, want no error", err)
		}
		defer func() { _ = reader.Close() }()

		// An open cursor rather than a transaction: it is the shape that holds a
		// WAL read mark without taking anything a writer needs, which is what
		// the checkpoint has to give up on. A read transaction opened through
		// database/sql escalates far enough to block the writes above.
		rows, err := reader.QueryContext(t.Context(), `SELECT x FROM probe`)
		if err != nil {
			t.Fatalf("hold a read = %v, want no error", err)
		}
		defer func() { _ = rows.Close() }()
		if !rows.Next() {
			t.Fatalf("the reader found no rows: %v", rows.Err())
		}

		started := time.Now()
		result, err := storage.Checkpoint(t.Context(), db.DB)
		elapsed := time.Since(started)

		if err != nil {
			t.Fatalf("Checkpoint = %v, want no error; a held database is not a failure", err)
		}
		if !result.Busy {
			t.Error("Checkpoint reported success while another connection held a read transaction")
		}
		// Generous against a loaded machine, and still an order of magnitude
		// below the five-second timeout the wait used to inherit.
		if budget := 20 * storage.CheckpointBudget; elapsed > budget {
			t.Errorf("Checkpoint waited %v for a held database, want well under %v", elapsed, budget)
		}
	})

	t.Run("a cancelled caller is not a database failure", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		result, err := storage.Checkpoint(ctx, db.DB)
		if err != nil {
			t.Fatalf("Checkpoint on a cancelled context = %v, want no error; "+
				"nothing was found wrong and reporting one makes the command invent a fault", err)
		}
		if !result.Busy {
			t.Error("a cancelled checkpoint reported success; the log is still where it was")
		}
	})
}

func fileSize(t *testing.T, path string) int64 {
	t.Helper()

	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return info.Size()
}
