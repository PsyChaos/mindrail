package index_test

import (
	"testing"

	"github.com/PsyChaos/mindrail/internal/index"
)

// TestImpactReadsServeTraversal is MR-009 TASK-01. The traversal's only
// join keys are neutral row reads: resolved referrers per uid, uid per
// file-scoped key, uids per logical key, facts per uid. Unknowns resolve to
// absence, never errors.
func TestImpactReadsServeTraversal(t *testing.T) {
	store, db := indexStore(t)
	unit, err := store.UpsertUnit(t.Context(), t.TempDir(), index.UnitPython)
	if err != nil {
		t.Fatal(err)
	}
	exec := func(statement string, args ...any) {
		t.Helper()
		if _, err := db.ExecContext(t.Context(), statement, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`INSERT INTO symbol_identities
		(symbol_uid, project_id, unit_id, language, logical_key, previous_keys, created_at)
		VALUES ('SYM-R-A', 'PRJ', ?, 'python', 'a', '[]', '2026-09-23T10:00:00Z')`, unit.ID)
	exec(`INSERT INTO symbols
		(unit_id, path, logical_key, kind, name, start_line, start_col, end_line, end_col,
		signature_hash, body_hash, structure_hash, symbol_uid)
		VALUES (?, '/r/a.py', 'a', 'function', 'f', 1, 0, 2, 0, 's', 'b', 't', 'SYM-R-A')`, unit.ID)
	var id int64
	if err := db.QueryRowContext(t.Context(),
		`SELECT id FROM symbols WHERE symbol_uid = 'SYM-R-A'`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	exec(`INSERT INTO symbol_references
		(unit_id, path, referrer_key, target_text, label, confidence, resolved_symbol_id)
		VALUES (?, '/r/a.py', 'b', 'f', 'STRUCTURAL_NAME_MATCH', 0.5, ?)`, unit.ID, id)

	referrers, err := store.ReferrersOfUID(t.Context(), "SYM-R-A")
	if err != nil || len(referrers) != 1 {
		t.Fatalf("referrers = %+v, %v", referrers, err)
	}
	if referrers[0].Key != "b" || referrers[0].Path != "/r/a.py" || referrers[0].Confidence != 0.5 {
		t.Fatalf("referrer = %+v", referrers[0])
	}
	if empty, err := store.ReferrersOfUID(t.Context(), "SYM-NOPE"); err != nil || len(empty) != 0 {
		t.Fatalf("unknown = %+v, %v", empty, err)
	}
	uid, found, err := store.UIDForKey(t.Context(), unit.ID, "/r/a.py", "a")
	if err != nil || !found || uid != "SYM-R-A" {
		t.Fatalf("uid = %q, %v, %v", uid, found, err)
	}
	if _, found, err := store.UIDForKey(t.Context(), unit.ID, "/r/a.py", "nope"); err != nil || found {
		t.Fatalf("missing key = %v, %v", found, err)
	}
	uids, err := store.KeyToUIDs(t.Context(), "a")
	if err != nil || len(uids) != 1 || uids[0] != "SYM-R-A" {
		t.Fatalf("uids = %+v, %v", uids, err)
	}
	key, name, path, found, err := store.FactsForUID(t.Context(), "SYM-R-A")
	if err != nil || !found || key != "a" || name != "f" || path != "/r/a.py" {
		t.Fatalf("facts = %q %q %q, %v, %v", key, name, path, found, err)
	}
	if _, _, _, found, err := store.FactsForUID(t.Context(), "SYM-NOPE"); err != nil || found {
		t.Fatalf("unknown facts = %v, %v", found, err)
	}
}
