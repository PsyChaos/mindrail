package index_test

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/index"
	"github.com/PsyChaos/mindrail/internal/migration"
	"github.com/PsyChaos/mindrail/internal/storage"
	"github.com/PsyChaos/mindrail/migrations"
)

const testProjectID = "PRJ-TEST-01"

func indexStore(t *testing.T) (*index.Store, *sql.DB) {
	t.Helper()
	db, err := storage.Open(t.Context(), storage.Options{Path: filepath.Join(t.TempDir(), "mindrail.db")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	set, err := migration.Load(migrations.FS)
	if err != nil {
		t.Fatal(err)
	}
	clock := app.FixedClock{Instant: time.Date(2026, 9, 18, 10, 0, 0, 0, time.UTC)}
	if _, err := migration.New(db.DB, set, clock).Up(t.Context()); err != nil {
		t.Fatal(err)
	}
	return index.NewStore(db.DB, clock), db.DB
}

func rowCount(t *testing.T, db *sql.DB, table, path string) int {
	t.Helper()
	var n int
	if err := db.QueryRowContext(t.Context(), "SELECT count(*) FROM "+table+" WHERE path = ?", path).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestUpsertUnitKeepsIdentityAndDiscoveryTime(t *testing.T) {
	store, db := indexStore(t)
	root := filepath.Join(t.TempDir(), "package")
	first, err := store.UpsertUnit(t.Context(), root, index.UnitJavaScript)
	if err != nil {
		t.Fatal(err)
	}
	if first.ID == "" || first.Path != root || first.Kind != index.UnitJavaScript {
		t.Fatalf("first unit = %+v", first)
	}
	second, err := store.UpsertUnit(t.Context(), root, index.UnitTypeScript)
	if err != nil {
		t.Fatal(err)
	}
	if second.ID != first.ID || !second.DiscoveredAt.Equal(first.DiscoveredAt) || second.Kind != index.UnitTypeScript {
		t.Fatalf("rediscovery changed identity/time or missed kind update: first=%+v second=%+v", first, second)
	}
	var n int
	if err := db.QueryRowContext(t.Context(), "SELECT count(*) FROM project_units WHERE path = ?", root).Scan(&n); err != nil || n != 1 {
		t.Fatalf("project unit rows = %d, err = %v", n, err)
	}
}

func TestFileStateResumeAndUnsupportedAreDistinct(t *testing.T) {
	store, _ := indexStore(t)
	unit, err := store.UpsertUnit(t.Context(), filepath.Join(t.TempDir(), "package"), index.UnitPython)
	if err != nil {
		t.Fatal(err)
	}
	pending := index.FileIndexState{UnitID: unit.ID, Path: filepath.Join(unit.Path, "a.py"), Language: "python", State: index.StatePending}
	unsupported := index.FileIndexState{UnitID: unit.ID, Path: filepath.Join(unit.Path, "notes.md"), Language: "", State: index.StateUnsupported}
	for _, state := range []index.FileIndexState{pending, unsupported} {
		if err := store.UpsertFileState(t.Context(), state); err != nil {
			t.Fatal(err)
		}
	}
	got, err := store.ListPending(t.Context(), unit.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Path != pending.Path {
		t.Fatalf("resume set = %+v, want only pending", got)
	}
	counts, err := store.CountByState(t.Context(), unit.ID)
	if err != nil {
		t.Fatal(err)
	}
	if counts[index.StatePending] != 1 || counts[index.StateUnsupported] != 1 {
		t.Fatalf("state counts = %+v", counts)
	}
	if err := store.UpsertFileState(t.Context(), index.FileIndexState{UnitID: unit.ID, Path: pending.Path, Language: "python", State: index.StateIndexed}); err == nil {
		t.Fatal("registration must not claim an unparsed file is indexed")
	}
}

func TestReplaceFileFactsIsAtomicAndScopedToOneFile(t *testing.T) {
	store, db := indexStore(t)
	unit, err := store.UpsertUnit(t.Context(), filepath.Join(t.TempDir(), "package"), index.UnitPython)
	if err != nil {
		t.Fatal(err)
	}
	a := filepath.Join(unit.Path, "a.py")
	b := filepath.Join(unit.Path, "b.py")
	for _, path := range []string{a, b} {
		if err := store.UpsertFileState(t.Context(), index.FileIndexState{UnitID: unit.ID, Path: path, Language: "python", State: index.StatePending}); err != nil {
			t.Fatal(err)
		}
	}
	sym := index.Symbol{LogicalKey: "f", Kind: "function", Name: "f", StartLine: 1, EndLine: 2, SignatureHash: "sig", BodyHash: "body", StructureHash: "shape"}
	for _, path := range []string{a, b} {
		_, err := store.ReplaceFileFacts(t.Context(), index.FileFacts{ProjectID: testProjectID, UnitID: unit.ID, Path: path, Language: "python", ContentHash: "old", State: index.StateIndexed, Symbols: []index.Symbol{sym}})
		if err != nil {
			t.Fatal(err)
		}
	}
	// D-83: two same-key declarations are legal; logical_key is an index, not identity.
	_, err = store.ReplaceFileFacts(t.Context(), index.FileFacts{ProjectID: testProjectID, UnitID: unit.ID, Path: a, Language: "python", ContentHash: "new", State: index.StateFailed, LastError: "partial tree", Symbols: []index.Symbol{sym, sym}, Imports: []index.Import{{Module: "os", Names: []string{"path"}}}, References: []index.Reference{{TargetText: "f", Confidence: 0.5}}})
	if err != nil {
		t.Fatal(err)
	}
	if rowCount(t, db, "symbols", a) != 2 || rowCount(t, db, "symbols", b) != 1 || rowCount(t, db, "symbol_imports", a) != 1 || rowCount(t, db, "symbol_references", a) != 1 {
		t.Fatal("replacement dropped a sibling file or failed to persist partial facts")
	}
	resume, err := store.ListPending(t.Context(), unit.ID)
	if err != nil || len(resume) != 1 || resume[0].Path != a || resume[0].State != index.StateFailed || resume[0].LastError != "partial tree" {
		t.Fatalf("failed file resume = %+v, err = %v", resume, err)
	}
	// A failed insert must roll back the deletions and state change with it.
	badID := int64(999999)
	_, err = store.ReplaceFileFacts(t.Context(), index.FileFacts{ProjectID: testProjectID, UnitID: unit.ID, Path: a, Language: "python", ContentHash: "bad", State: index.StateIndexed, References: []index.Reference{{TargetText: "missing", Confidence: 0.5, ResolvedSymbolID: &badID}}})
	if err == nil {
		t.Fatal("invalid reference FK committed")
	}
	if rowCount(t, db, "symbols", a) != 2 || rowCount(t, db, "symbol_imports", a) != 1 {
		t.Fatal("failed replacement deleted previously committed facts")
	}
	resume, err = store.ListPending(t.Context(), unit.ID)
	if err != nil || len(resume) != 1 || resume[0].ContentHash != "new" {
		t.Fatalf("failed replacement changed state: %+v, err = %v", resume, err)
	}
}

func TestCancelledReplacementChangesNothing(t *testing.T) {
	store, db := indexStore(t)
	unit, err := store.UpsertUnit(t.Context(), filepath.Join(t.TempDir(), "package"), index.UnitPython)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(unit.Path, "a.py")
	if err := store.UpsertFileState(t.Context(), index.FileIndexState{UnitID: unit.ID, Path: path, Language: "python", State: index.StatePending}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := store.ReplaceFileFacts(ctx, index.FileFacts{ProjectID: testProjectID, UnitID: unit.ID, Path: path, Language: "python", ContentHash: "sha", State: index.StateIndexed}); err == nil {
		t.Fatal("cancelled replacement committed")
	}
	if rowCount(t, db, "symbols", path) != 0 {
		t.Fatal("cancelled replacement wrote facts")
	}
	resume, err := store.ListPending(t.Context(), unit.ID)
	if err != nil || len(resume) != 1 || resume[0].State != index.StatePending {
		t.Fatalf("cancelled replacement changed state: %+v, err=%v", resume, err)
	}
}

func TestIndexFailureModesHaveDistinctActions(t *testing.T) {
	errs := []error{
		index.UnsupportedLanguage("/repo/README.md"),
		index.ParseFailed("/repo/broken.py", errors.New("bad syntax")),
		index.IndexStateCorrupt(errors.New("missing index row")),
	}
	wants := []app.Code{app.CodeSyntaxLanguageUnsupported, app.CodeSyntaxParseFailed, app.CodeIndexStateCorrupt}
	remedies := map[string]bool{}
	for i, err := range errs {
		payload, ok := app.PayloadOf(err)
		if !ok || payload.Code != wants[i] || payload.Why == "" || payload.Impact == "" || len(payload.NextAction) == 0 {
			t.Fatalf("failure %d payload = %+v, ok = %t", i, payload, ok)
		}
		remedy := payload.NextAction[0]
		if remedies[remedy] {
			t.Fatalf("two index failures share remedy %q", remedy)
		}
		remedies[remedy] = true
	}
}

func TestReplacingASymbolClearsOtherFilesStaleResolvedPointer(t *testing.T) {
	store, db := indexStore(t)
	unit, err := store.UpsertUnit(t.Context(), filepath.Join(t.TempDir(), "package"), index.UnitPython)
	if err != nil {
		t.Fatal(err)
	}
	declaration := filepath.Join(unit.Path, "definition.py")
	caller := filepath.Join(unit.Path, "caller.py")
	for _, path := range []string{declaration, caller} {
		if err := store.UpsertFileState(t.Context(), index.FileIndexState{UnitID: unit.ID, Path: path, Language: "python", State: index.StatePending}); err != nil {
			t.Fatal(err)
		}
	}
	sym := index.Symbol{LogicalKey: "definition.py:function:f", Kind: "function", Name: "f", SignatureHash: "sig", BodyHash: "old", StructureHash: "shape"}
	if _, err := store.ReplaceFileFacts(t.Context(), index.FileFacts{ProjectID: testProjectID, UnitID: unit.ID, Path: declaration, Language: "python", ContentHash: "old", State: index.StateIndexed, Symbols: []index.Symbol{sym}}); err != nil {
		t.Fatal(err)
	}
	var symbolID int64
	if err := db.QueryRowContext(t.Context(), `SELECT id FROM symbols WHERE path = ?`, declaration).Scan(&symbolID); err != nil {
		t.Fatal(err)
	}
	callerSym := sym
	callerSym.LogicalKey = "caller.py:function:caller"
	callerSym.Name = "caller"
	if _, err := store.ReplaceFileFacts(t.Context(), index.FileFacts{ProjectID: testProjectID, UnitID: unit.ID, Path: caller, Language: "python", ContentHash: "caller", State: index.StateIndexed, Symbols: []index.Symbol{callerSym}, References: []index.Reference{{TargetText: "f", Confidence: 0.5, ResolvedSymbolID: &symbolID}}}); err != nil {
		t.Fatal(err)
	}
	// A same-file reference to the old symbol exercises deletion order as well.
	if _, err = store.ReplaceFileFacts(t.Context(), index.FileFacts{ProjectID: testProjectID, UnitID: unit.ID, Path: declaration, Language: "python", ContentHash: "new", State: index.StateIndexed, Symbols: []index.Symbol{sym}, References: []index.Reference{{TargetText: "f", Confidence: 0.5}}}); err != nil {
		t.Fatalf("replace referenced symbol: %v", err)
	}
	var pointer sql.NullInt64
	if err := db.QueryRowContext(t.Context(), `SELECT resolved_symbol_id FROM symbol_references WHERE path = ?`, caller).Scan(&pointer); err != nil {
		t.Fatal(err)
	}
	if pointer.Valid || rowCount(t, db, "symbols", caller) != 1 || rowCount(t, db, "symbol_references", caller) != 1 {
		t.Fatalf("caller facts changed beyond clearing stale pointer: pointer=%v", pointer)
	}
}

func TestRediscoveryPreservesIndexedHashAndUnitScopedCounts(t *testing.T) {
	store, _ := indexStore(t)
	root := t.TempDir()
	first, err := store.UpsertUnit(t.Context(), filepath.Join(root, "first"), index.UnitPython)
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.UpsertUnit(t.Context(), filepath.Join(root, "second"), index.UnitPython)
	if err != nil {
		t.Fatal(err)
	}
	a := filepath.Join(first.Path, "a.py")
	b := filepath.Join(second.Path, "b.py")
	for _, tc := range []struct{ unit, path string }{{first.ID, a}, {second.ID, b}} {
		if err := store.UpsertFileState(t.Context(), index.FileIndexState{UnitID: tc.unit, Path: tc.path, Language: "python", State: index.StatePending}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := store.ReplaceFileFacts(t.Context(), index.FileFacts{ProjectID: testProjectID, UnitID: first.ID, Path: a, Language: "python", ContentHash: "original", State: index.StateIndexed}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertFileState(t.Context(), index.FileIndexState{UnitID: first.ID, Path: a, Language: "python", ContentHash: "original", State: index.StatePending}); err != nil {
		t.Fatal(err)
	}
	counts, err := store.CountByState(t.Context(), first.ID)
	if err != nil || counts[index.StateIndexed] != 1 || counts[index.StatePending] != 0 {
		t.Fatalf("unchanged hash became pending: %v, %v", counts, err)
	}
	all, err := store.CountByState(t.Context(), "")
	if err != nil || all[index.StateIndexed] != 1 || all[index.StatePending] != 1 {
		t.Fatalf("all-unit counts = %v, %v", all, err)
	}
	if err := store.UpsertFileState(t.Context(), index.FileIndexState{UnitID: first.ID, Path: a, Language: "python", ContentHash: "changed", State: index.StatePending}); err != nil {
		t.Fatal(err)
	}
	resume, err := store.ListPending(t.Context(), "")
	if err != nil || len(resume) != 2 || resume[0].Path != a || resume[1].Path != b || resume[0].ContentHash != "changed" {
		t.Fatalf("changed hash resume set = %+v, %v", resume, err)
	}
}

func TestStoreRejectsInvalidUnitAndFileInputs(t *testing.T) {
	store, _ := indexStore(t)
	if _, err := store.UpsertUnit(t.Context(), "relative", index.UnitPython); err == nil {
		t.Fatal("relative unit path accepted")
	}
	if _, err := store.UpsertUnit(t.Context(), t.TempDir(), index.UnitKind("rust")); err == nil {
		t.Fatal("unsupported unit kind accepted")
	}
	unit, err := store.UpsertUnit(t.Context(), t.TempDir(), index.UnitPython)
	if err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside.py")
	for _, state := range []index.FileIndexState{
		{UnitID: unit.ID, Path: outside, State: index.StatePending},
		{UnitID: "missing", Path: filepath.Join(unit.Path, "a.py"), State: index.StatePending},
		{UnitID: unit.ID, Path: filepath.Join(unit.Path, "a.py"), State: index.StateFailed},
	} {
		if err := store.UpsertFileState(t.Context(), state); err == nil {
			t.Fatalf("invalid registration accepted: %+v", state)
		}
	}
	for _, facts := range []index.FileFacts{
		{ProjectID: testProjectID, UnitID: unit.ID, Path: outside, Language: "python", ContentHash: "hash", State: index.StateIndexed},
		{ProjectID: testProjectID, UnitID: unit.ID, Path: filepath.Join(unit.Path, "a.py"), Language: "python", ContentHash: "hash", State: index.StatePending},
		{ProjectID: testProjectID, UnitID: unit.ID, Path: filepath.Join(unit.Path, "a.py"), Language: "python", ContentHash: "hash", State: index.StateFailed},
	} {
		if _, err := store.ReplaceFileFacts(t.Context(), facts); err == nil {
			t.Fatalf("invalid replacement accepted: %+v", facts)
		}
	}
}

func TestDamagedIndexTimestampIsNamedAsCorrupt(t *testing.T) {
	store, db := indexStore(t)
	unit, err := store.UpsertUnit(t.Context(), t.TempDir(), index.UnitPython)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(unit.Path, "a.py")
	if err := store.UpsertFileState(t.Context(), index.FileIndexState{UnitID: unit.ID, Path: path, State: index.StatePending}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(t.Context(), `UPDATE file_index_state SET indexed_at = 'yesterday' WHERE path = ?`, path); err != nil {
		t.Fatal(err)
	}
	_, err = store.ListPending(t.Context(), unit.ID)
	payload, ok := app.PayloadOf(err)
	if !ok || payload.Code != app.CodeIndexStateCorrupt {
		t.Fatalf("damaged timestamp error = %v, payload = %+v", err, payload)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := store.CountByState(ctx, unit.ID); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled read = %v, want context.Canceled", err)
	}
}

func TestReplaceFileFactsResolvesOnlyUniqueSameUnitLogicalKey(t *testing.T) {
	store, db := indexStore(t)
	unit, err := store.UpsertUnit(t.Context(), t.TempDir(), index.UnitPython)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(unit.Path, "a.py")
	if err := store.UpsertFileState(t.Context(), index.FileIndexState{UnitID: unit.ID, Path: path, Language: "python", State: index.StatePending}); err != nil {
		t.Fatal(err)
	}
	sym := index.Symbol{LogicalKey: "a.py:function:f", Kind: "function", Name: "f", SignatureHash: "s", BodyHash: "b", StructureHash: "t"}
	write := func(hash string, symbols []index.Symbol) {
		t.Helper()
		if _, err := store.ReplaceFileFacts(t.Context(), index.FileFacts{ProjectID: testProjectID, UnitID: unit.ID, Path: path, Language: "python", ContentHash: hash, State: index.StateIndexed, Symbols: symbols, References: []index.Reference{{TargetText: "f", TargetLogicalKey: sym.LogicalKey, Confidence: 0.5}}}); err != nil {
			t.Fatal(err)
		}
	}
	write("one", []index.Symbol{sym})
	var pointer sql.NullInt64
	if err := db.QueryRowContext(t.Context(), `SELECT resolved_symbol_id FROM symbol_references WHERE path = ?`, path).Scan(&pointer); err != nil || !pointer.Valid {
		t.Fatalf("unique same-key reference pointer = %v, err=%v", pointer, err)
	}
	var symbolID int64
	if err := db.QueryRowContext(t.Context(), `SELECT id FROM symbols WHERE path = ?`, path).Scan(&symbolID); err != nil || pointer.Int64 != symbolID {
		t.Fatalf("reference points to %v, symbol id=%d, err=%v", pointer, symbolID, err)
	}
	write("two", []index.Symbol{sym, sym})
	if err := db.QueryRowContext(t.Context(), `SELECT resolved_symbol_id FROM symbol_references WHERE path = ?`, path).Scan(&pointer); err != nil || pointer.Valid {
		t.Fatalf("ambiguous same-key reference pointer = %v, err=%v", pointer, err)
	}
}

func TestMovingFileToNestedUnitDropsOldFacts(t *testing.T) {
	store, db := indexStore(t)
	root := t.TempDir()
	parent, err := store.UpsertUnit(t.Context(), root, index.UnitPython)
	if err != nil {
		t.Fatal(err)
	}
	child, err := store.UpsertUnit(t.Context(), filepath.Join(root, "nested"), index.UnitPython)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(child.Path, "a.py")
	if err := store.UpsertFileState(t.Context(), index.FileIndexState{UnitID: parent.ID, Path: path, Language: "python", State: index.StatePending}); err != nil {
		t.Fatal(err)
	}
	sym := index.Symbol{LogicalKey: "nested/a.py:function:f", Kind: "function", Name: "f", SignatureHash: "s", BodyHash: "b", StructureHash: "t"}
	if _, err := store.ReplaceFileFacts(t.Context(), index.FileFacts{ProjectID: testProjectID, UnitID: parent.ID, Path: path, Language: "python", ContentHash: "old", State: index.StateIndexed, Symbols: []index.Symbol{sym}, Imports: []index.Import{{Module: "os"}}, References: []index.Reference{{TargetText: "f", Confidence: 0.5}}}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertFileState(t.Context(), index.FileIndexState{UnitID: child.ID, Path: path, Language: "python", State: index.StatePending}); err != nil {
		t.Fatal(err)
	}
	if rowCount(t, db, "symbols", path) != 0 || rowCount(t, db, "symbol_imports", path) != 0 || rowCount(t, db, "symbol_references", path) != 0 {
		t.Fatal("unit reassignment retained old-unit facts")
	}
	resume, err := store.ListPending(t.Context(), child.ID)
	if err != nil || len(resume) != 1 || resume[0].UnitID != child.ID || resume[0].State != index.StatePending || resume[0].ContentHash != "" || resume[0].Attempts != 2 {
		t.Fatalf("new unit resume state = %+v, err=%v", resume, err)
	}
}

func TestUnclassifiedWriteFailureKeepsCauseAndCancellation(t *testing.T) {
	store, db := indexStore(t)
	unit, err := store.UpsertUnit(t.Context(), t.TempDir(), index.UnitPython)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(unit.Path, "a.py")
	if err := store.UpsertFileState(t.Context(), index.FileIndexState{UnitID: unit.ID, Path: path, Language: "python", State: index.StatePending}); err != nil {
		t.Fatal(err)
	}
	good := index.FileFacts{ProjectID: testProjectID, UnitID: unit.ID, Path: path, Language: "python", ContentHash: "good", State: index.StateIndexed, References: []index.Reference{{TargetText: "f", Confidence: 0.5}}}
	if _, err := store.ReplaceFileFacts(t.Context(), good); err != nil {
		t.Fatalf("valid confidence rejected: %v", err)
	}
	bad := good
	bad.ContentHash = "bad"
	bad.References = []index.Reference{{TargetText: "f", Confidence: 0.6}}
	if _, err := store.ReplaceFileFacts(t.Context(), bad); err == nil || errors.Unwrap(err) == nil {
		t.Fatalf("invalid confidence error lost underlying SQL cause: %v", err)
	} else {
		// This was a SQL CHECK failure, not evidence that persisted state was
		// corrupt. The chain must be safe for errors.Is and errors.As.
		_ = errors.Is(err, context.Canceled)
		if payload, ok := app.PayloadOf(err); ok && payload.Code == app.CodeIndexStateCorrupt {
			t.Fatalf("CHECK violation mislabeled persisted corruption: %+v", payload)
		}
	}
	if rowCount(t, db, "symbol_references", path) != 1 {
		t.Fatal("failed replacement changed committed references")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := store.ReplaceFileFacts(ctx, good); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled replacement = %v, want context.Canceled", err)
	}
}

func TestStoreGatesHealthyOlderSchemaBeforeAnyIndexOperation(t *testing.T) {
	db, err := storage.Open(t.Context(), storage.Options{Path: filepath.Join(t.TempDir(), "mindrail.db")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	set, err := migration.Load(migrations.FS)
	if err != nil {
		t.Fatal(err)
	}
	clock := app.FixedClock{Instant: time.Date(2026, 9, 18, 10, 0, 0, 0, time.UTC)}
	if _, err := migration.New(db.DB, set[:3], clock).Up(t.Context()); err != nil {
		t.Fatal(err)
	}
	store := index.NewStore(db.DB, clock)
	root := t.TempDir()
	path := filepath.Join(root, "a.py")
	checks := []struct {
		name string
		call func() error
	}{
		{"upsert unit", func() error { _, err := store.UpsertUnit(t.Context(), root, index.UnitPython); return err }},
		{"upsert file", func() error {
			return store.UpsertFileState(t.Context(), index.FileIndexState{UnitID: "UNT-1", Path: path, Language: "python", State: index.StatePending})
		}},
		{"replace facts", func() error {
			_, err := store.ReplaceFileFacts(t.Context(), index.FileFacts{ProjectID: testProjectID, UnitID: "UNT-1", Path: path, Language: "python", ContentHash: "hash", State: index.StateIndexed})
			return err
		}},
		{"list pending", func() error { _, err := store.ListPending(t.Context(), ""); return err }},
		{"count", func() error { _, err := store.CountByState(t.Context(), ""); return err }},
	}
	for _, tc := range checks {
		t.Run(tc.name, func(t *testing.T) {
			payload, ok := app.PayloadOf(tc.call())
			required := "5"
			if tc.name == "upsert file" {
				required = "10" // Registration must preserve retired generations.
			}
			if !ok || payload.Code != app.CodeMigrationFailed || payload.Metadata["applied_version"] != "3" || payload.Metadata["required_version"] != required || len(payload.NextAction) == 0 || payload.NextAction[0] != "Run `mindrail init` to apply the pending migrations, then re-run the command." {
				t.Fatalf("v3 schema gate = %+v, ok=%t", payload, ok)
			}
		})
	}
	if _, err := migration.New(db.DB, set, clock).Up(t.Context()); err != nil {
		t.Fatal(err)
	}
	unit, err := store.UpsertUnit(t.Context(), root, index.UnitPython)
	if err != nil {
		t.Fatalf("v5 upsert unit: %v", err)
	}
	if err := store.UpsertFileState(t.Context(), index.FileIndexState{UnitID: unit.ID, Path: path, Language: "python", State: index.StatePending}); err != nil {
		t.Fatalf("v5 upsert file: %v", err)
	}
	if _, err := store.ReplaceFileFacts(t.Context(), index.FileFacts{ProjectID: testProjectID, UnitID: unit.ID, Path: path, Language: "python", ContentHash: "hash", State: index.StateIndexed}); err != nil {
		t.Fatalf("v5 replacement: %v", err)
	}
	if _, err := store.ListPending(t.Context(), ""); err != nil {
		t.Fatalf("v5 pending: %v", err)
	}
	if _, err := store.CountByState(t.Context(), ""); err != nil {
		t.Fatalf("v5 counts: %v", err)
	}
	if _, err := db.ExecContext(t.Context(), `DROP TABLE file_index_state`); err != nil {
		t.Fatal(err)
	}
	for _, tc := range checks[3:] {
		payload, ok := app.PayloadOf(tc.call())
		if !ok || payload.Code != app.CodeIndexStateCorrupt {
			t.Fatalf("damaged v4 %s = %+v, ok=%t", tc.name, payload, ok)
		}
	}
}

func TestStoreDistinguishesNeverInitializedFromMissingLedger(t *testing.T) {
	db, err := storage.Open(t.Context(), storage.Options{Path: filepath.Join(t.TempDir(), "mindrail.db")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	clock := app.FixedClock{Instant: time.Date(2026, 9, 18, 10, 0, 0, 0, time.UTC)}
	store := index.NewStore(db.DB, clock)
	_, err = store.CountByState(t.Context(), "")
	payload, ok := app.PayloadOf(err)
	if !ok || payload.Code != app.CodeMigrationFailed || payload.Metadata["applied_version"] != "0" || len(payload.NextAction) == 0 {
		t.Fatalf("fresh database gate = %+v, ok=%t, err=%v", payload, ok, err)
	}
	set, err := migration.Load(migrations.FS)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := migration.New(db.DB, set, clock).Up(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(t.Context(), `DROP TABLE schema_migrations`); err != nil {
		t.Fatal(err)
	}
	_, err = store.CountByState(t.Context(), "")
	payload, ok = app.PayloadOf(err)
	if !ok || payload.Code != app.CodeIndexStateCorrupt {
		t.Fatalf("missing ledger over existing app tables = %+v, ok=%t, err=%v", payload, ok, err)
	}
}

func TestExplicitSymbolIDHonorsUniqueKeyAndUnit(t *testing.T) {
	store, db := indexStore(t)
	root := t.TempDir()
	unit, err := store.UpsertUnit(t.Context(), filepath.Join(root, "one"), index.UnitPython)
	if err != nil {
		t.Fatal(err)
	}
	other, err := store.UpsertUnit(t.Context(), filepath.Join(root, "two"), index.UnitPython)
	if err != nil {
		t.Fatal(err)
	}
	decl := filepath.Join(unit.Path, "decl.py")
	call := filepath.Join(unit.Path, "call.py")
	otherPath := filepath.Join(other.Path, "decl.py")
	for _, tc := range []struct{ id, path string }{{unit.ID, decl}, {unit.ID, call}, {other.ID, otherPath}} {
		if err := store.UpsertFileState(t.Context(), index.FileIndexState{UnitID: tc.id, Path: tc.path, Language: "python", State: index.StatePending}); err != nil {
			t.Fatal(err)
		}
	}
	sym := index.Symbol{LogicalKey: "shared", Kind: "function", Name: "f", SignatureHash: "s", BodyHash: "b", StructureHash: "t"}
	for _, tc := range []struct{ id, path string }{{unit.ID, decl}, {other.ID, otherPath}} {
		if _, err := store.ReplaceFileFacts(t.Context(), index.FileFacts{ProjectID: testProjectID, UnitID: tc.id, Path: tc.path, Language: "python", ContentHash: "one", State: index.StateIndexed, Symbols: []index.Symbol{sym}}); err != nil {
			t.Fatal(err)
		}
	}
	var id, otherID int64
	if err := db.QueryRowContext(t.Context(), `SELECT id FROM symbols WHERE path = ?`, decl).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(t.Context(), `SELECT id FROM symbols WHERE path = ?`, otherPath).Scan(&otherID); err != nil {
		t.Fatal(err)
	}
	write := func(target int64, hash string) error {
		t.Helper()
		_, err := store.ReplaceFileFacts(t.Context(), index.FileFacts{ProjectID: testProjectID, UnitID: unit.ID, Path: call, Language: "python", ContentHash: hash, State: index.StateIndexed, References: []index.Reference{{TargetText: "f", Confidence: 0.5, ResolvedSymbolID: &target}}})
		return err
	}
	if err := write(id, "valid"); err != nil {
		t.Fatal(err)
	}
	var pointer sql.NullInt64
	if err := db.QueryRowContext(t.Context(), `SELECT resolved_symbol_id FROM symbol_references WHERE path = ?`, call).Scan(&pointer); err != nil || !pointer.Valid || pointer.Int64 != id {
		t.Fatalf("unique explicit pointer = %v, err=%v", pointer, err)
	}
	if err := write(otherID, "cross-unit"); err == nil {
		t.Fatal("cross-unit pointer accepted")
	} else if payload, ok := app.PayloadOf(err); !ok || payload.Code != app.CodeCommandLineInvalid {
		t.Fatalf("cross-unit pointer error = %+v, ok=%t, want COMMAND_LINE_INVALID", payload, ok)
	}
	var savedHash string
	if err := db.QueryRowContext(t.Context(), `SELECT content_hash FROM file_index_state WHERE path = ?`, call).Scan(&savedHash); err != nil || savedHash != "valid" {
		t.Fatalf("cross-unit refusal changed committed hash to %q, err=%v", savedHash, err)
	}
	if err := db.QueryRowContext(t.Context(), `SELECT resolved_symbol_id FROM symbol_references WHERE path = ?`, call).Scan(&pointer); err != nil || !pointer.Valid || pointer.Int64 != id {
		t.Fatalf("cross-unit refusal changed committed reference to %v, err=%v", pointer, err)
	}
	if _, err := store.ReplaceFileFacts(t.Context(), index.FileFacts{ProjectID: testProjectID, UnitID: unit.ID, Path: decl, Language: "python", ContentHash: "ambiguous", State: index.StateIndexed, Symbols: []index.Symbol{sym, sym}}); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(t.Context(), `SELECT id FROM symbols WHERE path = ? ORDER BY id LIMIT 1`, decl).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if err := write(id, "ambiguous"); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(t.Context(), `SELECT resolved_symbol_id FROM symbol_references WHERE path = ?`, call).Scan(&pointer); err != nil || pointer.Valid {
		t.Fatalf("ambiguous explicit pointer = %v, err=%v", pointer, err)
	}
}

func TestReplacementGuardsUseRegisteredValidFile(t *testing.T) {
	store, db := indexStore(t)
	root := t.TempDir()
	parent, err := store.UpsertUnit(t.Context(), root, index.UnitPython)
	if err != nil {
		t.Fatal(err)
	}
	child, err := store.UpsertUnit(t.Context(), filepath.Join(root, "nested"), index.UnitPython)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(child.Path, "a.py")
	if err := store.UpsertFileState(t.Context(), index.FileIndexState{UnitID: parent.ID, Path: path, Language: "python", State: index.StatePending}); err != nil {
		t.Fatal(err)
	}
	valid := index.FileFacts{ProjectID: testProjectID, UnitID: parent.ID, Path: path, Language: "python", ContentHash: "hash", State: index.StateIndexed}
	for _, tc := range []struct {
		name string
		edit func(*index.FileFacts)
	}{
		{"required content hash", func(f *index.FileFacts) { f.ContentHash = "" }},
		{"replacement state", func(f *index.FileFacts) { f.State = index.StatePending }},
		{"failed last error", func(f *index.FileFacts) { f.State = index.StateFailed; f.LastError = "" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			facts := valid
			tc.edit(&facts)
			if _, err := store.ReplaceFileFacts(t.Context(), facts); err == nil {
				t.Fatalf("invalid registered-file replacement accepted: %+v", facts)
			}
		})
	}
	if _, err := store.ReplaceFileFacts(t.Context(), valid); err != nil {
		t.Fatalf("valid control rejected: %v", err)
	}
	if err := store.UpsertFileState(t.Context(), index.FileIndexState{UnitID: child.ID, Path: path, Language: "python", State: index.StatePending}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ReplaceFileFacts(t.Context(), valid); err == nil {
		t.Fatal("stale owner write accepted")
	}
	var count int
	if err := db.QueryRowContext(t.Context(), `SELECT count(*) FROM file_index_state WHERE path = ? AND unit_id = ?`, path, child.ID).Scan(&count); err != nil || count != 1 {
		t.Fatalf("child ownership lost: count=%d err=%v", count, err)
	}
}

func TestRegisterFileRejectsUncleanAbsolutePath(t *testing.T) {
	store, db := indexStore(t)
	unit, err := store.UpsertUnit(t.Context(), t.TempDir(), index.UnitPython)
	if err != nil {
		t.Fatal(err)
	}
	clean := filepath.Join(unit.Path, "clean.py")
	if err := store.UpsertFileState(t.Context(), index.FileIndexState{UnitID: unit.ID, Path: clean, Language: "python", State: index.StatePending}); err != nil {
		t.Fatalf("clean control rejected: %v", err)
	}
	unclean := unit.Path + "/./unclean.py"
	if err := store.UpsertFileState(t.Context(), index.FileIndexState{UnitID: unit.ID, Path: unclean, Language: "python", State: index.StatePending}); err == nil {
		t.Fatal("unclean absolute path accepted")
	} else if payload, ok := app.PayloadOf(err); !ok || payload.Code != app.CodeCommandLineInvalid {
		t.Fatalf("unclean path error = %+v, ok=%t", payload, ok)
	}
	if rowCount(t, db, "file_index_state", unclean) != 0 {
		t.Fatal("unclean path was persisted despite refusal")
	}
}

func TestEmptyReferenceLabelUsesStructuralNameMatch(t *testing.T) {
	store, db := indexStore(t)
	unit, err := store.UpsertUnit(t.Context(), t.TempDir(), index.UnitPython)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(unit.Path, "a.py")
	if err := store.UpsertFileState(t.Context(), index.FileIndexState{UnitID: unit.ID, Path: path, State: index.StatePending}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ReplaceFileFacts(t.Context(), index.FileFacts{ProjectID: testProjectID, UnitID: unit.ID, Path: path, Language: "python", ContentHash: "hash", State: index.StateIndexed, References: []index.Reference{{TargetText: "f", Confidence: 0.5}}}); err != nil {
		t.Fatal(err)
	}
	var label string
	if err := db.QueryRowContext(t.Context(), `SELECT label FROM symbol_references WHERE path = ?`, path).Scan(&label); err != nil || label != "STRUCTURAL_NAME_MATCH" {
		t.Fatalf("default reference label = %q, err=%v", label, err)
	}
}

func TestListPendingFiltersRequestedUnit(t *testing.T) {
	store, _ := indexStore(t)
	var firstID, firstPath string
	for i := range 2 {
		unit, err := store.UpsertUnit(t.Context(), t.TempDir(), index.UnitPython)
		if err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(unit.Path, "a.py")
		if i == 0 {
			firstID, firstPath = unit.ID, path
		}
		if err := store.UpsertFileState(t.Context(), index.FileIndexState{UnitID: unit.ID, Path: path, State: index.StatePending}); err != nil {
			t.Fatal(err)
		}
	}
	all, err := store.ListPending(t.Context(), "")
	if err != nil || len(all) != 2 {
		t.Fatalf("all-unit control = %+v, err=%v", all, err)
	}
	got, err := store.ListPending(t.Context(), firstID)
	if err != nil || len(got) != 1 || got[0].UnitID != firstID || got[0].Path != firstPath {
		t.Fatalf("unit-scoped pending = %+v, err=%v", got, err)
	}
}

func TestNilImportNamesPersistAsJSONArray(t *testing.T) {
	store, db := indexStore(t)
	unit, err := store.UpsertUnit(t.Context(), t.TempDir(), index.UnitPython)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(unit.Path, "a.py")
	if err := store.UpsertFileState(t.Context(), index.FileIndexState{UnitID: unit.ID, Path: path, State: index.StatePending}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ReplaceFileFacts(t.Context(), index.FileFacts{ProjectID: testProjectID, UnitID: unit.ID, Path: path, Language: "python", ContentHash: "hash", State: index.StateIndexed, Imports: []index.Import{{Module: "os"}}}); err != nil {
		t.Fatal(err)
	}
	var names string
	if err := db.QueryRowContext(t.Context(), `SELECT names FROM symbol_imports WHERE path = ?`, path).Scan(&names); err != nil || names != "[]" {
		t.Fatalf("nil import names = %q, err=%v; want JSON []", names, err)
	}
}

func TestBusyIndexWriteKeepsRetryableCode(t *testing.T) {
	_, first := indexStore(t)
	var dbPath string
	if err := first.QueryRowContext(t.Context(), `SELECT file FROM pragma_database_list WHERE name = 'main'`).Scan(&dbPath); err != nil {
		t.Fatal(err)
	}
	second, err := storage.Open(t.Context(), storage.Options{Path: dbPath, BusyTimeout: 20 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = second.Close() })
	store := index.NewStore(second.DB, app.SystemClock{})
	root := t.TempDir()
	if _, err := store.UpsertUnit(t.Context(), root, index.UnitPython); err != nil {
		t.Fatalf("unlocked control rejected: %v", err)
	}
	tx, err := first.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	locked := true
	t.Cleanup(func() {
		if locked {
			_ = tx.Rollback()
		}
	})
	_, err = store.UpsertUnit(t.Context(), root, index.UnitTypeScript)
	payload, ok := app.PayloadOf(err)
	if !ok || payload.Code != app.CodeBusyRetryable {
		t.Fatalf("busy write error = %+v, ok=%t, err=%v", payload, ok, err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	locked = false
	if _, err := store.UpsertUnit(t.Context(), root, index.UnitTypeScript); err != nil {
		t.Fatalf("write after lock release rejected: %v", err)
	}
}
