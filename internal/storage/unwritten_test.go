package storage_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PsyChaos/mindrail/internal/storage"
)

// TestIsUnwrittenSeparatesAnUnfinishedFileFromADatabase is finding H14's unit.
//
// A zero-length mindrail.db is what an interrupted first run leaves: the file
// exists and holds no database. Decision D-03 puts "not initialised" at exit 0,
// and the other half of that same interrupted run — a database created but not
// yet migrated — is already reported that way; this is the question that puts
// the two on the same footing.
//
// The false rows are the over-fire guards, and they are the point. Every one of
// them is a condition with a different remedy, and calling any of them
// "unwritten" would report a repository that needs attention as one that merely
// needs `mindrail init`.
func TestIsUnwrittenSeparatesAnUnfinishedFileFromADatabase(t *testing.T) {
	dir := t.TempDir()

	database := filepath.Join(dir, "real.db")
	db := openTemp(t, storage.Options{Path: database})
	if _, err := db.ExecContext(t.Context(), `CREATE TABLE probe (id TEXT PRIMARY KEY) STRICT`); err != nil {
		t.Fatalf("seed a database: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close the seed database: %v", err)
	}

	occupied := filepath.Join(dir, "occupied.db")
	if err := os.Mkdir(occupied, 0o700); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}

	notADatabase := filepath.Join(dir, "junk.db")
	if err := os.WriteFile(notADatabase, []byte(strings.Repeat("x", 64)), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	empty := filepath.Join(dir, "empty.db")
	if err := os.WriteFile(empty, nil, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	tests := map[string]struct {
		path string
		want bool
		why  string
	}{
		"nothing is there": {
			path: filepath.Join(dir, "absent.db"), want: true,
			why: "a repository that was never initialised",
		},
		"a zero-length file an interrupted first run left": {
			path: empty, want: true,
			why: "the file was created and no database was ever written to it",
		},
		"a database mindrail init produced": {
			path: database, want: false,
			why: "an initialised repository must never be reported as needing init",
		},
		"a directory occupying the path": {
			path: occupied, want: false,
			why: "an obstruction init cannot clear, with its own remedy",
		},
		"a non-empty file that is not a database": {
			path: notADatabase, want: false,
			why: "a corrupt file has to be moved aside first, which init will not do",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			if got := storage.IsUnwritten(tc.path); got != tc.want {
				t.Errorf("IsUnwritten(%s) = %v, want %v: %s", name, got, tc.want, tc.why)
			}
		})
	}
}
