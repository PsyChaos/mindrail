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
// true of the first two since they shipped. 000003 is pinned from the day it
// was written (MR-004, TASK-02); until MR-004 ships, a change to its row is a
// change to a file no user's repository has applied, which is allowed and is
// recorded in mr-004-findings.md when it happens.
func TestAnAppliedMigrationFileIsNeverEdited(t *testing.T) {
	shipped := map[string]string{
		"000001_initial.sql":                       "2dd55c6233abde9d1e1b6c7a5eec951569f64bdd55090037a238cd2b155fae43",
		"000002_coordination.sql":                  "1de055444435e8a355e5f13821e2a5de786a63fcbe3c06da9e70e3fc38ae03e0",
		"000003_lease_idempotency.sql":             "3ca7dbee3971cf6ea165c82738b5f40b792595afbfea58f10304d0f0c38ef488",
		"000004_index.sql":                         "b16fcbb34d59914b5a970b5f4910f3571c230deae67abd34dde287530f24feb9",
		"000005_symbol_identity.sql":               "780f256b695a383ffb689699dadd753d859bd8c455d58943957cac116f9a3b9a",
		"000006_changes.sql":                       "5ec101203eaa3438818c4733b706e5db71dee640d0b7ffb1e2a2f5f55fd4b903",
		"000007_scope_attribution.sql":             "01aca3385623b07f08a3f533334c5a6844e39fbf6dc08fd1dba7684f69e7c9e6",
		"000008_evidence.sql":                      "a0e1064d18e0ea36e86adf2ace66fdb914fb90d7eb796336089bbde329390a7d",
		"000009_guard_baselines.sql":               "40d69bd447c4dc15873aef30a32010c01d1ceb73ac972050200a97d83163a162",
		"000010_file_index_generations.sql":        "e857ef5fffc24f50d3c0ce1e9c5b8ecf5077c8370052e41a520c363deb6af923",
		"000011_jev_routes_and_agent_presence.sql": "a3de59eb0be9e37b9a9570a598b4e304edb26022674ecd5125ae287f237baeea",
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
