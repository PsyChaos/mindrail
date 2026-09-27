package index_test

import (
	"testing"

	"github.com/PsyChaos/mindrail/internal/index"
)

// TestPathForUIDResolvesLiveFile is MR-008 TASK-02. Attribution keys symbol
// rows to baseline scopes by file, and the change row carries no path — the
// uid is the only join key this method serves.
func TestPathForUIDResolvesLiveFile(t *testing.T) {
	store, db := indexStore(t)
	unit, err := store.UpsertUnit(t.Context(), t.TempDir(), index.UnitPython)
	if err != nil {
		t.Fatal(err)
	}
	seed := `INSERT INTO symbols
		(unit_id, path, logical_key, kind, name, start_line, start_col, end_line, end_col,
		signature_hash, body_hash, structure_hash, symbol_uid)
		VALUES (?, '/r/a.py', 'k', 'function', 'f', 1, 0, 2, 0, 's', 'b', 't', 'SYM-UID-1')`
	if _, err := db.ExecContext(t.Context(), `INSERT INTO symbol_identities
		(symbol_uid, project_id, unit_id, language, logical_key, previous_keys, created_at)
		VALUES ('SYM-UID-1', 'PRJ-TEST', ?, 'python', 'k', '[]', '2026-09-23T10:00:00Z')`,
		unit.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(t.Context(), seed, unit.ID); err != nil {
		t.Fatal(err)
	}
	path, found, err := store.PathForUID(t.Context(), "SYM-UID-1")
	if err != nil || !found || path != "/r/a.py" {
		t.Fatalf("PathForUID = %q, %v, %v", path, found, err)
	}
	if _, found, err := store.PathForUID(t.Context(), "SYM-NOPE"); err != nil || found {
		t.Fatalf("unknown uid = %v, %v", found, err)
	}
	if _, found, err := store.PathForUID(t.Context(), ""); err != nil || found {
		t.Fatalf("empty uid = %v, %v", found, err)
	}
}
