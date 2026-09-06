package storage_test

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/PsyChaos/mindrail/internal/app"
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
		// An absolute bound, not one derived from the constant under test.
		// `20 * storage.CheckpointBudget` was an equation: raising the budget
		// back to the five seconds it used to inherit raised the bound with it,
		// and the test passed in 5.02 seconds while reporting success. Two
		// seconds is generous against a loaded machine and still less than half
		// the regression it exists to catch.
		const bound = 2 * time.Second
		if elapsed > bound {
			t.Errorf("Checkpoint waited %v for a held database, want under %v; "+
				"one held read lock must not cost `mindrail init` the database's whole busy timeout",
				elapsed, bound)
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

// TestAFailedCheckpointIsReportedAsOne covers the statement's own failure,
// which nothing reached.
//
// The Flush test one package up observes an error, but the error it observes is
// the pool's: a closed database fails at `db.Conn` and never runs the pragma.
// Making the scan's error return nil therefore passed everywhere, so the
// diagnosis built for a checkpoint that ran and was refused — the one condition
// this function exists to name — was unreachable.
//
// A write-ahead log overwritten with something that is not one is the cheapest
// way to make the statement itself fail, and it is a real shape: a log is an
// ordinary file beside the database and nothing stops a backup tool, an editor
// or another program from writing to it.
func TestAFailedCheckpointIsReportedAsOne(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mindrail.db")
	db, err := storage.Open(t.Context(), storage.Options{Path: path})
	if err != nil {
		t.Fatalf("storage.Open = %v, want no error", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if _, err := db.ExecContext(t.Context(), `CREATE TABLE probe (x INTEGER)`); err != nil {
		t.Fatalf("CREATE TABLE = %v, want no error", err)
	}
	for range 8 {
		if _, err := db.ExecContext(t.Context(), `INSERT INTO probe (x) VALUES (1)`); err != nil {
			t.Fatalf("INSERT = %v, want no error", err)
		}
	}

	if err := os.WriteFile(path+"-wal", []byte("not a write-ahead log at all"), 0o600); err != nil {
		t.Fatalf("overwrite the log: %v", err)
	}

	result, err := storage.Checkpoint(t.Context(), db.DB)
	if err == nil {
		t.Fatalf("Checkpoint = nil over a log it could not read; result %+v", result)
	}
	if result.Busy {
		t.Error("the failure was reported as contention, which clears on its own and this does not")
	}
	if !errors.Is(err, storage.ErrCheckpointFailed) {
		t.Errorf("Checkpoint = %v, want errors.Is(err, ErrCheckpointFailed)", err)
	}

	payload, ok := app.PayloadOf(err)
	if !ok {
		t.Fatalf("Checkpoint = %v, which carries no domain payload for a command to render", err)
	}
	if len(payload.NextAction) == 0 {
		t.Error("the failure carries no remedy")
	}
	if payload.Cause == "" {
		t.Error("the failure drops the driver's own message, which is the only thing that says why")
	}
}

// TestCheckpointHandsTheConnectionBackAsItFoundIt is the restore, which nothing
// could fail.
//
// The budget is a property of a connection, and the connection goes back to a
// pool the next writer draws from. Stubbing the restore out left every pooled
// connection carrying 250 ms where the database's own timeout is five seconds,
// so the next caller's wait was twenty times short — and the whole suite stayed
// green while the comment on the restore asserted exactly this.
func TestCheckpointHandsTheConnectionBackAsItFoundIt(t *testing.T) {
	const connections = 4

	path := filepath.Join(t.TempDir(), "mindrail.db")
	db, err := storage.Open(t.Context(), storage.Options{
		Path:         path,
		BusyTimeout:  5 * time.Second,
		MaxOpenConns: connections,
	})
	if err != nil {
		t.Fatalf("storage.Open = %v, want no error", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if _, err := db.ExecContext(t.Context(), `CREATE TABLE probe (x INTEGER)`); err != nil {
		t.Fatalf("CREATE TABLE = %v, want no error", err)
	}
	if _, err := storage.Checkpoint(t.Context(), db.DB); err != nil {
		t.Fatalf("Checkpoint = %v, want no error", err)
	}

	// Every connection in the pool, because the checkpoint borrows one of them
	// and which one is the pool's business. Holding them all at once is what
	// makes the answer about the pool rather than about whichever connection
	// happened to be handed back first.
	held := make([]*sql.Conn, 0, connections)
	defer func() {
		for _, conn := range held {
			_ = conn.Close()
		}
	}()

	for i := range connections {
		conn, err := db.Conn(t.Context())
		if err != nil {
			t.Fatalf("check out connection %d: %v", i, err)
		}
		held = append(held, conn)

		var timeout int64
		if err := conn.QueryRowContext(t.Context(), `PRAGMA busy_timeout`).Scan(&timeout); err != nil {
			t.Fatalf("read busy_timeout on connection %d: %v", i, err)
		}
		if want := (5 * time.Second).Milliseconds(); timeout != want {
			t.Errorf("connection %d came out of the pool with busy_timeout = %d, want %d; "+
				"the checkpoint's budget escaped onto a connection the next writer will use",
				i, timeout, want)
		}
	}
}

func fileSize(t *testing.T, path string) int64 {
	t.Helper()

	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return info.Size()
}
