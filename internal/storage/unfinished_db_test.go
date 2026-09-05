package storage_test

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/storage"
)

// seedNonWAL writes a real SQLite database in the default rollback-journal
// mode, which is what any tool that is not Mindrail leaves behind.
//
// It goes through database/sql against the driver storage already registered,
// rather than through storage.Open, because storage.Open's whole job is to put
// the file into the state this helper has to avoid.
func seedNonWAL(t *testing.T, path string) {
	t.Helper()

	db, err := sql.Open(registeredDriverName, "file:"+path)
	if err != nil {
		t.Fatalf("open %q with database/sql: %v", path, err)
	}
	defer func() { _ = db.Close() }()

	db.SetMaxOpenConns(1)
	if _, err := db.ExecContext(t.Context(), `CREATE TABLE seeded (id TEXT PRIMARY KEY)`); err != nil {
		t.Fatalf("seed a rollback-journal database: %v", err)
	}

	var mode string
	if err := db.QueryRowContext(t.Context(), `PRAGMA journal_mode`).Scan(&mode); err != nil {
		t.Fatalf("read back journal_mode: %v", err)
	}
	if mode == "wal" {
		t.Fatalf("the seed is already in WAL; this fixture proves nothing")
	}
}

// registeredDriverName is the name internal/storage registers with
// database/sql. It is spelled out rather than imported because the arch tests
// confine the driver import to driver.go, this package's test files included.
const registeredDriverName = "sqlite"

// TestOpenReadOnlyBlamesTheFileNotTheDriverForAnUnfinishedDatabase is finding
// F5.
//
// A zero-length mindrail.db is the ordinary result of an interrupted first run,
// and a rollback-journal database is what another tool leaves behind. Both were
// reported as "a required SQLite pragma is not in effect on a pooled
// connection" whose only next action was "Report this with the mindrail
// version" -- an unactionable remedy for a condition `mindrail init` clears in
// one command.
func TestOpenReadOnlyBlamesTheFileNotTheDriverForAnUnfinishedDatabase(t *testing.T) {
	cases := []struct {
		name     string
		seed     func(t *testing.T, path string)
		wantWhy  string
		wantNext string
	}{
		{
			name: "a zero-length file left by an interrupted first run",
			seed: func(t *testing.T, path string) {
				t.Helper()
				if err := os.WriteFile(path, nil, 0o600); err != nil {
					t.Fatalf("seed a zero-length database: %v", err)
				}
			},
			wantWhy:  "empty",
			wantNext: "mindrail init",
		},
		{
			name:     "a database that was never converted to WAL",
			seed:     seedNonWAL,
			wantWhy:  "not in WAL mode",
			wantNext: "mindrail init",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "mindrail.db")
			tc.seed(t, path)

			_, err := storage.Open(t.Context(), storage.Options{Path: path, ReadOnly: true})
			if err == nil {
				t.Fatalf("Open = nil, want an error: %q is not a database Mindrail can read", path)
			}
			if !errors.Is(err, storage.ErrNotWAL) {
				t.Fatalf("Open = %v, want it to unwrap to ErrNotWAL", err)
			}

			payload, ok := app.PayloadOf(err)
			if !ok {
				t.Fatalf("Open error carries no domain payload: %v", err)
			}
			if !strings.Contains(payload.Why, tc.wantWhy) {
				t.Errorf("payload.Why = %q, want it to contain %q", payload.Why, tc.wantWhy)
			}
			if !strings.Contains(payload.Why, path) {
				t.Errorf("payload.Why = %q, want it to name %q", payload.Why, path)
			}

			joined := strings.Join(payload.NextAction, " | ")
			if !strings.Contains(joined, tc.wantNext) {
				t.Errorf("next actions %v do not offer the command that fixes this", payload.NextAction)
			}
			if strings.Contains(joined, "Report this") {
				t.Errorf("next actions %v ask for a bug report for a recoverable condition", payload.NextAction)
			}
			if payload.Metadata["path"] != path {
				t.Errorf("metadata path = %q, want %q", payload.Metadata["path"], path)
			}
		})
	}
}

// TestInitModeFinishesAnUnfinishedDatabase proves the remedy the report now
// prints is the one that works: opening the same two files for writing, which
// is what `mindrail init` does, succeeds and leaves them in WAL.
func TestInitModeFinishesAnUnfinishedDatabase(t *testing.T) {
	cases := map[string]func(t *testing.T, path string){
		"zero-length": func(t *testing.T, path string) {
			t.Helper()
			if err := os.WriteFile(path, nil, 0o600); err != nil {
				t.Fatalf("seed a zero-length database: %v", err)
			}
		},
		"non-WAL": seedNonWAL,
	}

	for name, seed := range cases {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "mindrail.db")
			seed(t, path)

			db := openTemp(t, storage.Options{Path: path})
			pragmas, err := storage.ReadPragmas(t.Context(), db)
			if err != nil {
				t.Fatalf("ReadPragmas = %v, want no error", err)
			}
			if pragmas != storage.ExpectedPragmas(0) {
				t.Fatalf("pragmas = %+v, want %+v", pragmas, storage.ExpectedPragmas(0))
			}

			// And the read-only path the report came from now works too.
			ro, err := storage.Open(t.Context(), storage.Options{Path: path, ReadOnly: true})
			if err != nil {
				t.Fatalf("read-only Open after the remedy = %v, want no error", err)
			}
			if err := ro.Close(); err != nil {
				t.Errorf("Close = %v, want no error", err)
			}
		})
	}
}

// TestOpenReadOnlyAcceptsAHealthyDatabase is the over-fire guard for finding
// F5: the new classification must not fire on the database `mindrail init`
// actually produces, opened the way `status` and `doctor` open it.
func TestOpenReadOnlyAcceptsAHealthyDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mindrail.db")

	writer := openTemp(t, storage.Options{Path: path})
	if _, err := writer.ExecContext(t.Context(), `CREATE TABLE probe (id TEXT PRIMARY KEY) STRICT`); err != nil {
		t.Fatalf("seed a table: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	db, err := storage.Open(t.Context(), storage.Options{Path: path, ReadOnly: true})
	if err != nil {
		t.Fatalf("read-only Open of a healthy WAL database = %v, want no error", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	pragmas, err := storage.ReadPragmas(t.Context(), db)
	if err != nil {
		t.Fatalf("ReadPragmas = %v, want no error", err)
	}
	if pragmas != storage.ExpectedPragmas(0) {
		t.Fatalf("pragmas = %+v, want %+v", pragmas, storage.ExpectedPragmas(0))
	}
}

// TestAHealthyDatabaseInAReadOnlyDirectoryIsNotCalledUnfinished is the second
// over-fire guard for finding F5, and the adjacent condition the new
// classification must not swallow.
//
// SQLite cannot open a WAL database read-only without creating the -shm
// sidecar, so a healthy database in a directory nothing can write fails at
// open. That is an environment problem with its own diagnosis, and reporting it
// as "the file was never initialised, run `mindrail init`" would send the user
// to a command that fails the same way.
func TestAHealthyDatabaseInAReadOnlyDirectoryIsNotCalledUnfinished(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not enforce Unix directory permissions this way")
	}
	if os.Geteuid() == 0 {
		t.Skip("root bypasses directory permissions, so there is nothing to observe")
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "mindrail.db")

	writer := openTemp(t, storage.Options{Path: path})
	if _, err := writer.ExecContext(t.Context(), `CREATE TABLE probe (id TEXT PRIMARY KEY) STRICT`); err != nil {
		t.Fatalf("seed a table: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatalf("make the directory read-only: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	_, err := storage.Open(t.Context(), storage.Options{Path: path, ReadOnly: true})
	if err == nil {
		t.Skip("this platform opened a WAL database read-only without a writable directory")
	}
	if errors.Is(err, storage.ErrNotWAL) {
		t.Fatalf("Open = %v, which calls a healthy WAL database unfinished", err)
	}
	if !errors.Is(err, storage.ErrOpenFailed) {
		t.Fatalf("Open = %v, want the ordinary open failure", err)
	}
}

// TestCorruptDatabaseRemedyNamesTheFile is finding F10. The path lived only in
// the JSON metadata, so the human output said "Move the file aside" without
// ever saying which file -- and the runtime database sits under .git, where a
// user has no reason to be looking.
func TestCorruptDatabaseRemedyNamesTheFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mindrail.db")
	if err := os.WriteFile(path, []byte(strings.Repeat("not a database at all\n", 64)), 0o600); err != nil {
		t.Fatalf("seed a corrupt database: %v", err)
	}

	_, err := storage.Open(t.Context(), storage.Options{Path: path})
	if err == nil {
		t.Fatalf("Open = nil, want an error for a file that is not a database")
	}
	if !errors.Is(err, storage.ErrCorrupt) {
		t.Fatalf("Open = %v, want it to unwrap to ErrCorrupt", err)
	}

	payload, ok := app.PayloadOf(err)
	if !ok {
		t.Fatalf("Open error carries no domain payload: %v", err)
	}
	if !strings.Contains(payload.Why, path) {
		t.Errorf("payload.Why = %q, want it to name %q", payload.Why, path)
	}
	if !strings.Contains(strings.Join(payload.NextAction, " | "), path) {
		t.Errorf("next actions %v tell the user to move a file they are never shown", payload.NextAction)
	}
}
