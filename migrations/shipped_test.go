package migrations_test

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"github.com/PsyChaos/mindrail/migrations"
)

// TestAnAppliedMigrationFileIsNeverEdited pins the bytes of every migration
// that has shipped.
//
// The migrator records a file's sha256 when it applies it and compares the two
// on every later start, so editing an applied file — a typo, a comment, a
// reworded sentence — reports MIGRATION_CHECKSUM_MISMATCH to every database that
// already applied it, and nothing else in the suite notices: load_test.go
// re-derives the hash from the same bytes it loaded, and the two mismatch tests
// use synthetic fixtures. Audit round 2 (§4.17) mutated the comment in
// 000002_coordination.sql and ran `go test ./...` fully green. This is the test
// that goes red instead, so that the correction lands in migrations/README.md
// where the file itself says it must.
//
// A new migration is a new file with a new row here. A checksum in this table
// changes only if the file was never applied anywhere, and that has not been
// true of either of these since they shipped.
func TestAnAppliedMigrationFileIsNeverEdited(t *testing.T) {
	shipped := map[string]string{
		"000001_initial.sql":      "2dd55c6233abde9d1e1b6c7a5eec951569f64bdd55090037a238cd2b155fae43",
		"000002_coordination.sql": "1de055444435e8a355e5f13821e2a5de786a63fcbe3c06da9e70e3fc38ae03e0",
	}

	entries, err := migrations.FS.ReadDir(".")
	if err != nil {
		t.Fatalf("reading the embedded migrations: %v", err)
	}
	seen := make(map[string]bool, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		want, ok := shipped[name]
		if !ok {
			// A migration that has not shipped yet has no row to hold it to;
			// it gains one when it ships.
			continue
		}
		body, err := migrations.FS.ReadFile(name)
		if err != nil {
			t.Fatalf("reading %s: %v", name, err)
		}
		sum := sha256.Sum256(body)
		if got := hex.EncodeToString(sum[:]); got != want {
			t.Errorf("%s has been edited after it was applied: sha256 %s, shipped as %s\n"+
				"Every database that applied it will report MIGRATION_CHECKSUM_MISMATCH. "+
				"Revert the edit and record the correction in migrations/README.md.", name, got, want)
		}
		seen[name] = true
	}
	for name := range shipped {
		if !seen[name] {
			t.Errorf("%s is pinned here but is no longer embedded", name)
		}
	}
}
