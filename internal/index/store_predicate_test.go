package index_test

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/PsyChaos/mindrail/internal/index"
)

func queryInt(t *testing.T, db *sql.DB, query string, args ...any) int {
	t.Helper()
	var value int
	if err := db.QueryRowContext(t.Context(), query, args...).Scan(&value); err != nil {
		t.Fatal(err)
	}
	return value
}

func TestUnitUpsertChangesOnlyRefinedKind(t *testing.T) {
	store, db := indexStore(t)
	path := t.TempDir()
	first, err := store.UpsertUnit(t.Context(), path, index.UnitJavaScript)
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		`CREATE TABLE audit_unit_updates (n INTEGER NOT NULL)`,
		`INSERT INTO audit_unit_updates VALUES (0)`,
		`CREATE TRIGGER audit_unit_update AFTER UPDATE ON project_units BEGIN UPDATE audit_unit_updates SET n = n + 1; END`,
	} {
		if _, err := db.ExecContext(t.Context(), statement); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := store.UpsertUnit(t.Context(), path, index.UnitJavaScript); err != nil {
		t.Fatal(err)
	}
	if n := queryInt(t, db, `SELECT n FROM audit_unit_updates`); n != 0 {
		t.Fatalf("unchanged discovery caused %d updates", n)
	}
	refined, err := store.UpsertUnit(t.Context(), path, index.UnitTypeScript)
	if err != nil {
		t.Fatal(err)
	}
	if n := queryInt(t, db, `SELECT n FROM audit_unit_updates`); n != 1 || refined.ID != first.ID || !refined.DiscoveredAt.Equal(first.DiscoveredAt) || refined.Kind != index.UnitTypeScript {
		t.Fatalf("kind refinement = %+v, updates=%d; first=%+v", refined, n, first)
	}
}

func TestFileConflictPredicatesPreserveUnchangedAndRequeueChangedHash(t *testing.T) {
	store, db := indexStore(t)
	unit, err := store.UpsertUnit(t.Context(), t.TempDir(), index.UnitPython)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(unit.Path, "a.py")
	register := func(hash string, state index.FileState) {
		t.Helper()
		if err := store.UpsertFileState(t.Context(), index.FileIndexState{UnitID: unit.ID, Path: path, Language: "python", ContentHash: hash, State: state}); err != nil {
			t.Fatal(err)
		}
	}
	register("", index.StatePending)
	for _, statement := range []string{
		`CREATE TABLE audit_file_updates (n INTEGER NOT NULL)`,
		`INSERT INTO audit_file_updates VALUES (0)`,
		`CREATE TRIGGER audit_file_update AFTER UPDATE ON file_index_state BEGIN UPDATE audit_file_updates SET n = n + 1; END`,
	} {
		if _, err := db.ExecContext(t.Context(), statement); err != nil {
			t.Fatal(err)
		}
	}
	register("", index.StatePending)
	if n := queryInt(t, db, `SELECT n FROM audit_file_updates`); n != 0 {
		t.Fatalf("unchanged pending registration caused %d updates", n)
	}
	if _, err := store.ReplaceFileFacts(t.Context(), index.FileFacts{UnitID: unit.ID, Path: path, Language: "python", ContentHash: "original", State: index.StateIndexed}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(t.Context(), `UPDATE audit_file_updates SET n = 0`); err != nil {
		t.Fatal(err)
	}
	register("original", index.StatePending)
	if n := queryInt(t, db, `SELECT n FROM audit_file_updates`); n != 0 {
		t.Fatalf("same-hash indexed registration caused %d updates", n)
	}
	register("changed", index.StatePending)
	if n := queryInt(t, db, `SELECT n FROM audit_file_updates`); n != 1 {
		t.Fatalf("changed hash caused %d updates, want one requeue", n)
	}
	var state index.FileState
	var hash sql.NullString
	if err := db.QueryRowContext(t.Context(), `SELECT state, content_hash FROM file_index_state WHERE path = ?`, path).Scan(&state, &hash); err != nil || state != index.StatePending || !hash.Valid || hash.String != "changed" {
		t.Fatalf("changed hash state=%q hash=%v err=%v, want pending target hash", state, hash, err)
	}
	register("", index.StateUnsupported)
	if n := queryInt(t, db, `SELECT n FROM audit_file_updates`); n != 2 {
		t.Fatalf("supported→unsupported caused %d updates, want two total", n)
	}
	register("", index.StateUnsupported)
	if n := queryInt(t, db, `SELECT n FROM audit_file_updates`); n != 2 {
		t.Fatalf("unchanged unsupported registration caused %d updates", n)
	}
}

func TestUniqueResolutionUsesUnitAndLogicalKey(t *testing.T) {
	store, db := indexStore(t)
	root := t.TempDir()
	first, err := store.UpsertUnit(t.Context(), filepath.Join(root, "first"), index.UnitPython)
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.UpsertUnit(t.Context(), filepath.Join(root, "second"), index.UnitPython)
	if err != nil {
		t.Fatal(err)
	}
	firstDecl := filepath.Join(first.Path, "decl.py")
	firstCall := filepath.Join(first.Path, "call.py")
	secondDecl := filepath.Join(second.Path, "decl.py")
	for _, tc := range []struct{ unitID, path string }{{first.ID, firstDecl}, {first.ID, firstCall}, {second.ID, secondDecl}} {
		if err := store.UpsertFileState(t.Context(), index.FileIndexState{UnitID: tc.unitID, Path: tc.path, Language: "python", State: index.StatePending}); err != nil {
			t.Fatal(err)
		}
	}
	target := index.Symbol{LogicalKey: "target", Kind: "function", Name: "target", SignatureHash: "s", BodyHash: "b", StructureHash: "t"}
	decoy := target
	decoy.LogicalKey, decoy.Name = "decoy", "decoy"
	for _, facts := range []index.FileFacts{
		{UnitID: first.ID, Path: firstDecl, Language: "python", ContentHash: "first", State: index.StateIndexed, Symbols: []index.Symbol{target, decoy}},
		{UnitID: second.ID, Path: secondDecl, Language: "python", ContentHash: "second", State: index.StateIndexed, Symbols: []index.Symbol{target}},
		{UnitID: first.ID, Path: firstCall, Language: "python", ContentHash: "call", State: index.StateIndexed, References: []index.Reference{{TargetText: "target", TargetLogicalKey: target.LogicalKey, Confidence: 0.5}}},
	} {
		if _, err := store.ReplaceFileFacts(t.Context(), facts); err != nil {
			t.Fatal(err)
		}
	}
	var targetID, resolvedID int64
	if err := db.QueryRowContext(t.Context(), `SELECT id FROM symbols WHERE unit_id = ? AND logical_key = ?`, first.ID, target.LogicalKey).Scan(&targetID); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(t.Context(), `SELECT resolved_symbol_id FROM symbol_references WHERE path = ?`, firstCall).Scan(&resolvedID); err != nil || resolvedID != targetID {
		t.Fatalf("resolved id=%d want first-unit target id=%d, err=%v", resolvedID, targetID, err)
	}
}
