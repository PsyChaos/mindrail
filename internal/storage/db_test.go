package storage_test

import (
	"crypto/rand"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/storage"
)

// openTemp opens a fresh runtime database under t.TempDir and closes it when
// the test ends. Nothing here may touch the developer's real runtime dir.
func openTemp(t *testing.T, opts storage.Options) *storage.DB {
	t.Helper()

	if opts.Path == "" {
		opts.Path = filepath.Join(t.TempDir(), "mindrail.db")
	}

	db, err := storage.Open(t.Context(), opts)
	if err != nil {
		t.Fatalf("Open(%q) = %v, want no error", opts.Path, err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("Close() = %v, want no error", err)
		}
	})
	return db
}

// TestOpenAppliesPragmasOnEveryConnection is the observable half of decision
// D-22. A DSN parameter that reaches only the first pooled connection would
// leave later queries running with foreign_keys off, so the assertion is made
// against several connections held open at the same time.
func TestOpenAppliesPragmasOnEveryConnection(t *testing.T) {
	const conns = 4

	db := openTemp(t, storage.Options{MaxOpenConns: conns})
	want := storage.ExpectedPragmas(0)

	// Held simultaneously: releasing each conn before taking the next would let
	// the pool hand back the same underlying connection every time.
	held := make([]*sql.Conn, 0, conns)
	for i := range conns {
		conn, err := db.Conn(t.Context())
		if err != nil {
			t.Fatalf("Conn(%d) = %v, want no error", i, err)
		}
		t.Cleanup(func() { _ = conn.Close() })
		held = append(held, conn)
	}

	for i, conn := range held {
		got, err := storage.ReadPragmas(t.Context(), conn)
		if err != nil {
			t.Fatalf("ReadPragmas(conn %d) = %v, want no error", i, err)
		}
		if got != want {
			t.Errorf("conn %d pragmas = %+v, want %+v", i, got, want)
		}
	}
}

// TestOpenAppliesPragmasOnConcurrentConnections exercises the same guarantee
// from several goroutines, which is how the pool actually grows in production.
func TestOpenAppliesPragmasOnConcurrentConnections(t *testing.T) {
	const conns = 4

	db := openTemp(t, storage.Options{MaxOpenConns: conns})
	want := storage.ExpectedPragmas(0)

	var (
		wg    sync.WaitGroup
		mu    sync.Mutex
		start = make(chan struct{})
		got   = make([]storage.Pragmas, conns)
		errs  = make([]error, conns)
	)
	for i := range conns {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start

			conn, err := db.Conn(t.Context())
			if err != nil {
				mu.Lock()
				errs[i] = err
				mu.Unlock()
				return
			}
			defer func() { _ = conn.Close() }()

			pragmas, err := storage.ReadPragmas(t.Context(), conn)
			mu.Lock()
			got[i], errs[i] = pragmas, err
			mu.Unlock()
		}()
	}
	close(start)
	wg.Wait()

	for i := range conns {
		if errs[i] != nil {
			t.Fatalf("goroutine %d: %v, want no error", i, errs[i])
		}
		if got[i] != want {
			t.Errorf("goroutine %d pragmas = %+v, want %+v", i, got[i], want)
		}
	}
}

// TestReadPragmasObservesTheConnectionNotTheDSN is what gives decision D-22's
// read-back its value. If ReadPragmas merely echoed what Open asked for, the
// verification in Open would confirm nothing; changing a setting behind its
// back must therefore change what it reports.
func TestReadPragmasObservesTheConnectionNotTheDSN(t *testing.T) {
	db := openTemp(t, storage.Options{})

	conn, err := db.Conn(t.Context())
	if err != nil {
		t.Fatalf("Conn = %v, want no error", err)
	}
	defer func() { _ = conn.Close() }()

	before, err := storage.ReadPragmas(t.Context(), conn)
	if err != nil {
		t.Fatalf("ReadPragmas = %v, want no error", err)
	}
	if before != storage.ExpectedPragmas(0) {
		t.Fatalf("ReadPragmas = %+v, want %+v", before, storage.ExpectedPragmas(0))
	}

	if _, err := conn.ExecContext(t.Context(), `PRAGMA foreign_keys = OFF`); err != nil {
		t.Fatalf("PRAGMA foreign_keys = OFF: %v, want no error", err)
	}
	if _, err := conn.ExecContext(t.Context(), `PRAGMA busy_timeout = 17`); err != nil {
		t.Fatalf("PRAGMA busy_timeout = 17: %v, want no error", err)
	}

	after, err := storage.ReadPragmas(t.Context(), conn)
	if err != nil {
		t.Fatalf("ReadPragmas = %v, want no error", err)
	}
	if after.ForeignKeys != 0 {
		t.Errorf("ForeignKeys = %d after turning it off, want 0; ReadPragmas is not reading the connection",
			after.ForeignKeys)
	}
	if after.BusyTimeout != 17 {
		t.Errorf("BusyTimeout = %d after setting 17, want 17", after.BusyTimeout)
	}
	if after == storage.ExpectedPragmas(0) {
		t.Error("ReadPragmas still matches the expected set after the settings changed")
	}
}

func TestExpectedPragmasHonoursBusyTimeout(t *testing.T) {
	tests := []struct {
		name    string
		timeout time.Duration
		want    int
	}{
		{name: "zero falls back to the default", timeout: 0, want: 5000},
		{name: "explicit timeout in milliseconds", timeout: 250 * time.Millisecond, want: 250},
		{name: "sub-millisecond rounds down to the default", timeout: time.Microsecond, want: 5000},
		{name: "negative falls back to the default", timeout: -time.Second, want: 5000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := storage.ExpectedPragmas(tt.timeout)
			if got.BusyTimeout != tt.want {
				t.Errorf("ExpectedPragmas(%v).BusyTimeout = %d, want %d", tt.timeout, got.BusyTimeout, tt.want)
			}
			if got.JournalMode != "wal" || got.ForeignKeys != 1 || got.Synchronous != 1 {
				t.Errorf("ExpectedPragmas(%v) = %+v, want wal/1/1 for the other three", tt.timeout, got)
			}
		})
	}
}

// TestOpenCreatesWALSidecar proves journal_mode actually converted the file
// rather than merely being reported back by the pragma query.
func TestOpenCreatesWALSidecar(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mindrail.db")
	db := openTemp(t, storage.Options{Path: path})

	if _, err := db.ExecContext(t.Context(), `CREATE TABLE probe (id INTEGER PRIMARY KEY) STRICT`); err != nil {
		t.Fatalf("CREATE TABLE = %v, want no error", err)
	}

	for _, suffix := range []string{"", "-wal"} {
		if _, err := os.Stat(path + suffix); err != nil {
			t.Errorf("Stat(%q) = %v, want the file to exist", path+suffix, err)
		}
	}
}

func TestOpenRejectsCorruptDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mindrail.db")
	garbage := make([]byte, 8192)
	if _, err := rand.Read(garbage); err != nil {
		t.Fatalf("rand.Read = %v, want no error", err)
	}
	if err := os.WriteFile(path, garbage, 0o600); err != nil {
		t.Fatalf("WriteFile = %v, want no error", err)
	}

	db, err := storage.Open(t.Context(), storage.Options{Path: path})
	if err == nil {
		_ = db.Close()
		t.Fatal("Open(garbage) = nil error, want ErrCorrupt")
	}
	if !errors.Is(err, storage.ErrCorrupt) {
		t.Errorf("Open(garbage) = %v, want errors.Is(err, ErrCorrupt)", err)
	}

	payload, ok := app.PayloadOf(err)
	if !ok {
		t.Fatalf("PayloadOf(%v) = _, false, want a domain payload", err)
	}
	if payload.Code != app.CodeRuntimeDBCorrupt {
		t.Errorf("payload.Code = %q, want %q", payload.Code, app.CodeRuntimeDBCorrupt)
	}
	if app.ExitCode(err) != app.ExitUnavailable {
		t.Errorf("ExitCode = %d, want %d (decision D-03)", app.ExitCode(err), app.ExitUnavailable)
	}
}

// TestOpenReadOnlyDoesNotCreateFile guards decision D-01: status and doctor use
// the read-only mode and must report a missing setup, never manufacture one.
func TestOpenReadOnlyDoesNotCreateFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mindrail.db")

	db, err := storage.Open(t.Context(), storage.Options{Path: path, ReadOnly: true})
	if err == nil {
		_ = db.Close()
		t.Fatal("Open(read-only, absent) = nil error, want ErrOpenFailed")
	}
	if !errors.Is(err, storage.ErrOpenFailed) {
		t.Errorf("Open(read-only, absent) = %v, want errors.Is(err, ErrOpenFailed)", err)
	}
	if payload, ok := app.PayloadOf(err); !ok || payload.Code != app.CodeRuntimeDBUnavailable {
		t.Errorf("payload = %+v (ok=%v), want %q", payload, ok, app.CodeRuntimeDBUnavailable)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir = %v, want no error", err)
	}
	if len(entries) != 0 {
		t.Errorf("directory contains %d entries after a failed read-only open, want 0", len(entries))
	}
}

// TestOpenReadOnlyRejectsWrites confirms the read-only mode is enforced by the
// connection rather than by convention.
func TestOpenReadOnlyRejectsWrites(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mindrail.db")

	writable := openTemp(t, storage.Options{Path: path})
	if _, err := writable.ExecContext(t.Context(), `CREATE TABLE probe (id INTEGER PRIMARY KEY) STRICT`); err != nil {
		t.Fatalf("CREATE TABLE = %v, want no error", err)
	}

	reader := openTemp(t, storage.Options{Path: path, ReadOnly: true})
	if _, err := reader.ExecContext(t.Context(), `INSERT INTO probe (id) VALUES (1)`); err == nil {
		t.Error("INSERT on a read-only handle = nil error, want a write rejection")
	}

	var count int
	if err := reader.QueryRowContext(t.Context(), `SELECT count(*) FROM probe`).Scan(&count); err != nil {
		t.Fatalf("SELECT on a read-only handle = %v, want no error", err)
	}
}

// TestOpenReadOnlyAfterWriterClosed is the shape doctor and status actually
// see: init created a WAL database and exited, so the -shm sidecar is gone by
// the time the read-only handle arrives.
func TestOpenReadOnlyAfterWriterClosed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mindrail.db")

	writer, err := storage.Open(t.Context(), storage.Options{Path: path})
	if err != nil {
		t.Fatalf("Open(writable) = %v, want no error", err)
	}
	if _, err := writer.ExecContext(t.Context(),
		`CREATE TABLE probe (id INTEGER PRIMARY KEY, v TEXT NOT NULL) STRICT`); err != nil {
		t.Fatalf("CREATE TABLE = %v, want no error", err)
	}
	if _, err := writer.ExecContext(t.Context(), `INSERT INTO probe (id, v) VALUES (1, 'kept')`); err != nil {
		t.Fatalf("INSERT = %v, want no error", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("Close(writable) = %v, want no error", err)
	}

	reader := openTemp(t, storage.Options{Path: path, ReadOnly: true})

	var v string
	if err := reader.QueryRowContext(t.Context(), `SELECT v FROM probe WHERE id = 1`).Scan(&v); err != nil {
		t.Fatalf("SELECT = %v, want no error", err)
	}
	if v != "kept" {
		t.Errorf("v = %q, want %q", v, "kept")
	}
}

func TestOpenRejectsRelativePath(t *testing.T) {
	db, err := storage.Open(t.Context(), storage.Options{Path: "mindrail.db"})
	if err == nil {
		_ = db.Close()
		t.Fatal("Open(relative) = nil error, want ErrOpenFailed")
	}
	if !errors.Is(err, storage.ErrOpenFailed) {
		t.Errorf("Open(relative) = %v, want errors.Is(err, ErrOpenFailed)", err)
	}
}

func TestPathReturnsTheOpenedFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mindrail.db")
	db := openTemp(t, storage.Options{Path: path})

	if got := db.Path(); got != path {
		t.Errorf("Path() = %q, want %q", got, path)
	}
}

// TestReopenAfterCloseIsClean proves Close leaves a consistent file behind:
// a WAL database abandoned mid-checkpoint would fail integrity_check here.
func TestReopenAfterCloseIsClean(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mindrail.db")

	first, err := storage.Open(t.Context(), storage.Options{Path: path})
	if err != nil {
		t.Fatalf("first Open = %v, want no error", err)
	}
	if _, err := first.ExecContext(t.Context(), `CREATE TABLE probe (id INTEGER PRIMARY KEY, v TEXT NOT NULL) STRICT`); err != nil {
		t.Fatalf("CREATE TABLE = %v, want no error", err)
	}
	if _, err := first.ExecContext(t.Context(), `INSERT INTO probe (id, v) VALUES (1, 'kept')`); err != nil {
		t.Fatalf("INSERT = %v, want no error", err)
	}
	if err := first.Close(); err != nil {
		t.Fatalf("Close = %v, want no error", err)
	}

	second := openTemp(t, storage.Options{Path: path})
	result, err := storage.IntegrityCheck(t.Context(), second)
	if err != nil {
		t.Fatalf("IntegrityCheck = %v, want no error", err)
	}
	if result != "ok" {
		t.Errorf("IntegrityCheck = %q, want %q", result, "ok")
	}

	var v string
	if err := second.QueryRowContext(t.Context(), `SELECT v FROM probe WHERE id = 1`).Scan(&v); err != nil {
		t.Fatalf("SELECT after reopen = %v, want no error", err)
	}
	if v != "kept" {
		t.Errorf("v = %q, want %q", v, "kept")
	}
}

func TestCloseIsIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mindrail.db")
	db, err := storage.Open(t.Context(), storage.Options{Path: path})
	if err != nil {
		t.Fatalf("Open = %v, want no error", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("first Close = %v, want no error", err)
	}
	if err := db.Close(); err != nil {
		t.Errorf("second Close = %v, want no error", err)
	}
}

func TestForeignKeysAreEnforced(t *testing.T) {
	db := openTemp(t, storage.Options{})

	stmts := []string{
		`CREATE TABLE parent (id TEXT PRIMARY KEY) STRICT`,
		`CREATE TABLE child (id TEXT PRIMARY KEY, parent_id TEXT NOT NULL REFERENCES parent(id)) STRICT`,
	}
	for _, stmt := range stmts {
		if _, err := db.ExecContext(t.Context(), stmt); err != nil {
			t.Fatalf("%s = %v, want no error", stmt, err)
		}
	}

	// foreign_keys=OFF would silently accept this row; that is the exact
	// failure decision D-22 promotes the pragma to a MUST to prevent.
	if _, err := db.ExecContext(t.Context(), `INSERT INTO child (id, parent_id) VALUES ('c', 'missing')`); err == nil {
		t.Error("orphan INSERT = nil error, want a foreign key violation")
	}
}
