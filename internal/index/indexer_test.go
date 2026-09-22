package index

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/filesystem"
	"github.com/PsyChaos/mindrail/internal/index/parser"
	"github.com/PsyChaos/mindrail/internal/index/snapshot"
	"github.com/PsyChaos/mindrail/internal/storage"
)

func indexerFixture(t *testing.T) (*Indexer, ProjectUnit, string, *sql.DB) {
	t.Helper()
	store, db, _ := errorStore(t)
	root := t.TempDir()
	unit, err := store.UpsertUnit(t.Context(), root, UnitPython)
	if err != nil {
		t.Fatal(err)
	}
	registry, err := parser.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(registry.Close)
	cache := snapshot.New(filesystem.RuntimePaths{CacheDir: filepath.Join(t.TempDir(), "cache")})
	return NewIndexer(store, registry, cache), unit, root, db
}

func sourceFile(t *testing.T, root, name, content string) string {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestIndexFileUnchangedIndexedSkipsParseAndAllSQLWrites(t *testing.T) {
	idx, unit, root, db := indexerFixture(t)
	path := sourceFile(t, root, "a.py", "def f():\n    return 1\n")
	first, err := idx.IndexFile(t.Context(), unit, path)
	if err != nil || first.State.State != StateIndexed {
		t.Fatalf("first=%+v err=%v", first, err)
	}
	if _, err := db.ExecContext(t.Context(), `CREATE TABLE index_write_audit (n INTEGER NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"file_index_state", "symbols", "symbol_imports", "symbol_references"} {
		for _, operation := range []string{"INSERT", "UPDATE", "DELETE"} {
			query := "CREATE TRIGGER audit_" + table + "_" + operation + " AFTER " + operation + " ON " + table + " BEGIN INSERT INTO index_write_audit VALUES (1); END"
			if _, err := db.ExecContext(t.Context(), query); err != nil {
				t.Fatal(err)
			}
		}
	}
	var calls int
	idx.extract = func(context.Context, parser.SyntaxAdapter, parser.SourceFile) (parser.Facts, error) {
		calls++
		return parser.Facts{}, nil
	}
	second, err := idx.IndexFile(t.Context(), unit, path)
	if err != nil || !second.Skipped || calls != 0 {
		t.Fatalf("second=%+v err=%v parse calls=%d", second, err, calls)
	}
	var writes int
	if err := db.QueryRowContext(t.Context(), `SELECT count(*) FROM index_write_audit`).Scan(&writes); err != nil || writes != 0 {
		t.Fatalf("writes=%d err=%v", writes, err)
	}
}

func TestIndexFileCancellationLeavesTargetPendingAndRetries(t *testing.T) {
	idx, unit, root, _ := indexerFixture(t)
	path := sourceFile(t, root, "a.py", "def f():\n    return 1\n")
	ctx, cancel := context.WithCancel(t.Context())
	idx.extract = func(context.Context, parser.SyntaxAdapter, parser.SourceFile) (parser.Facts, error) {
		cancel()
		return parser.Facts{}, context.Canceled
	}
	if _, err := idx.IndexFile(ctx, unit, path); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel error=%v", err)
	}
	state, err := idx.store.ReadFileState(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256([]byte("def f():\n    return 1\n"))
	if state.State.State != StatePending || state.State.ContentHash != hex.EncodeToString(hash[:]) {
		t.Fatalf("pending state=%+v", state.State)
	}
	idx.extract = parser.Extract
	result, err := idx.IndexFile(t.Context(), unit, path)
	if err != nil || result.State.State != StateIndexed {
		t.Fatalf("resume=%+v err=%v", result, err)
	}
}

func TestIndexFileRejectsUnsupportedSymlinkAndOutsideUnit(t *testing.T) {
	idx, unit, root, _ := indexerFixture(t)
	unsupported := sourceFile(t, root, "a.md", "hello")
	if _, err := idx.IndexFile(t.Context(), unit, unsupported); err == nil {
		t.Fatal("unsupported accepted")
	} else if payload, ok := app.PayloadOf(err); !ok || payload.Code != app.CodeSyntaxLanguageUnsupported {
		t.Fatalf("unsupported=%v", err)
	}
	outside := sourceFile(t, t.TempDir(), "a.py", "pass\n")
	if _, err := idx.IndexFile(t.Context(), unit, outside); err == nil {
		t.Fatal("outside unit accepted")
	}
	link := filepath.Join(root, "linked.py")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if _, err := idx.IndexFile(t.Context(), unit, link); err == nil {
		t.Fatal("symlink accepted")
	}
}

func TestIndexFileChangedReplacesOnlyOwnFacts(t *testing.T) {
	idx, unit, root, db := indexerFixture(t)
	a := sourceFile(t, root, "a.py", "def old():\n    return 1\n")
	b := sourceFile(t, root, "b.py", "def sibling():\n    return 2\n")
	for _, path := range []string{a, b} {
		if _, err := idx.IndexFile(t.Context(), unit, path); err != nil {
			t.Fatal(err)
		}
	}
	var siblingID int64
	if err := db.QueryRowContext(t.Context(), `SELECT id FROM symbols WHERE path = ?`, b).Scan(&siblingID); err != nil {
		t.Fatal(err)
	}
	sourceFile(t, root, "a.py", "def newer():\n    return 3\n")
	if _, err := idx.IndexFile(t.Context(), unit, a); err != nil {
		t.Fatal(err)
	}
	var ownNames string
	if err := db.QueryRowContext(t.Context(), `SELECT group_concat(name) FROM symbols WHERE path = ?`, a).Scan(&ownNames); err != nil {
		t.Fatal(err)
	}
	var afterID int64
	if err := db.QueryRowContext(t.Context(), `SELECT id FROM symbols WHERE path = ?`, b).Scan(&afterID); err != nil {
		t.Fatal(err)
	}
	if ownNames != "newer" || afterID != siblingID {
		t.Fatalf("own names=%q sibling ID %d -> %d", ownNames, siblingID, afterID)
	}
}

func TestIndexFileFailedSameHashRetriesAndClassifiesParseFailure(t *testing.T) {
	idx, unit, root, _ := indexerFixture(t)
	path := sourceFile(t, root, "bad.py", "def broken(:\n    pass\n")
	first, err := idx.IndexFile(t.Context(), unit, path)
	payload, ok := app.PayloadOf(err)
	if !ok || payload.Code != app.CodeSyntaxParseFailed || first.State.State != StateFailed || first.State.Attempts != 1 {
		t.Fatalf("first=%+v err=%v", first, err)
	}
	second, err := idx.IndexFile(t.Context(), unit, path)
	payload, ok = app.PayloadOf(err)
	if !ok || payload.Code != app.CodeSyntaxParseFailed || second.Skipped || second.State.State != StateFailed || second.State.Attempts != 2 {
		t.Fatalf("retry=%+v err=%v", second, err)
	}
}

func TestIndexFileSourceChangedDuringParseDoesNotCommitOldFacts(t *testing.T) {
	idx, unit, root, db := indexerFixture(t)
	path := sourceFile(t, root, "a.py", "def old():\n    return 1\n")
	idx.extract = func(ctx context.Context, adapter parser.SyntaxAdapter, source parser.SourceFile) (parser.Facts, error) {
		if err := os.WriteFile(path, []byte("def newer():\n    return 2\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		return parser.Extract(ctx, adapter, source)
	}
	first, err := idx.IndexFile(t.Context(), unit, path)
	if err != nil || !first.Stale || first.State.State != StatePending {
		t.Fatalf("stale=%+v err=%v", first, err)
	}
	var rows int
	if err := db.QueryRowContext(t.Context(), `SELECT count(*) FROM symbols WHERE path = ?`, path).Scan(&rows); err != nil || rows != 0 {
		t.Fatalf("old rows=%d err=%v", rows, err)
	}
	idx.extract = parser.Extract
	second, err := idx.IndexFile(t.Context(), unit, path)
	if err != nil || second.State.State != StateIndexed {
		t.Fatalf("retry=%+v err=%v", second, err)
	}
	var name string
	if err := db.QueryRowContext(t.Context(), `SELECT name FROM symbols WHERE path = ?`, path).Scan(&name); err != nil || name != "newer" {
		t.Fatalf("name=%q err=%v", name, err)
	}
}

func TestIndexFilePostCommitChangeRequeuesNewTargetWithoutClaimingItParsed(t *testing.T) {
	idx, unit, root, db := indexerFixture(t)
	path := sourceFile(t, root, "a.py", "def old():\n    return 1\n")
	newContent := "def newer():\n    return 2\n"
	checks := 0
	idx.recheck = func(path string, prior sourceVersion) (sourceVersion, bool, error) {
		checks++
		if checks == 3 {
			if err := os.WriteFile(path, []byte(newContent), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		return sourceStillMatches(path, prior)
	}
	result, err := idx.IndexFile(t.Context(), unit, path)
	if err != nil || !result.Stale || result.State.State != StatePending || result.State.ContentHash != contentHash([]byte(newContent)) {
		t.Fatalf("result=%+v err=%v checks=%d", result, err, checks)
	}
	var oldName string
	if err := db.QueryRowContext(t.Context(), `SELECT name FROM symbols WHERE path = ?`, path).Scan(&oldName); err != nil || oldName != "old" {
		t.Fatalf("completed old facts=%q err=%v", oldName, err)
	}
	idx.recheck = sourceStillMatches
	second, err := idx.IndexFile(t.Context(), unit, path)
	if err != nil || second.State.State != StateIndexed || second.State.ContentHash != contentHash([]byte(newContent)) {
		t.Fatalf("retry=%+v err=%v", second, err)
	}
}

func TestIndexFilePostCommitInvalidationCannotOverwriteNewerRegistration(t *testing.T) {
	idx, unit, root, _ := indexerFixture(t)
	path := sourceFile(t, root, "a.py", "def old():\n    return 1\n")
	newContent := "def newer():\n    return 2\n"
	checks := 0
	idx.recheck = func(path string, prior sourceVersion) (sourceVersion, bool, error) {
		checks++
		if checks == 3 {
			if err := os.WriteFile(path, []byte(newContent), 0o644); err != nil {
				t.Fatal(err)
			}
			completed, err := idx.store.ReadFileState(t.Context(), path)
			if err != nil {
				t.Fatal(err)
			}
			if _, applied, err := idx.store.RegisterFileCAS(t.Context(), completed, unit.ID, path, "python", contentHash([]byte(newContent))); err != nil || !applied {
				t.Fatalf("H2 register=%t err=%v", applied, err)
			}
		}
		return sourceStillMatches(path, prior)
	}
	result, err := idx.IndexFile(t.Context(), unit, path)
	if err != nil || !result.Stale {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	state, err := idx.store.ReadFileState(t.Context(), path)
	if err != nil || state.State.State != StatePending || state.State.ContentHash != contentHash([]byte(newContent)) || state.State.Attempts != 2 {
		t.Fatalf("H2 state=%+v err=%v", state.State, err)
	}
	if result.State.State != StatePending || result.State.ContentHash != state.State.ContentHash {
		t.Fatalf("result did not reflect H2: result=%+v durable=%+v", result.State, state.State)
	}
}

func TestIndexFilePostCommitDeletionRemovesOnlyItsCompletedFacts(t *testing.T) {
	idx, unit, root, db := indexerFixture(t)
	path := sourceFile(t, root, "a.py", "def old():\n    return 1\n")
	checks := 0
	idx.recheck = func(path string, prior sourceVersion) (sourceVersion, bool, error) {
		checks++
		if checks == 3 {
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
		}
		return sourceStillMatches(path, prior)
	}
	result, err := idx.IndexFile(t.Context(), unit, path)
	if err != nil || !result.Stale || checks != 3 {
		t.Fatalf("result=%+v err=%v checks=%d", result, err, checks)
	}
	state, err := idx.store.ReadFileState(t.Context(), path)
	if err != nil || state.Exists {
		t.Fatalf("deleted source retained state=%+v err=%v", state, err)
	}
	for _, table := range []string{"symbols", "symbol_imports", "symbol_references"} {
		var rows int
		if err := db.QueryRowContext(t.Context(), "SELECT count(*) FROM "+table+" WHERE path = ?", path).Scan(&rows); err != nil || rows != 0 {
			t.Fatalf("%s rows=%d err=%v", table, rows, err)
		}
	}
}

func TestIndexFilePostCommitUnreadableSourceInvalidatesCompletion(t *testing.T) {
	idx, unit, root, _ := indexerFixture(t)
	path := sourceFile(t, root, "a.py", "def old():\n    return 1\n")
	checks := 0
	idx.recheck = func(path string, prior sourceVersion) (sourceVersion, bool, error) {
		checks++
		if checks == 3 {
			return sourceVersion{}, false, os.ErrPermission
		}
		return sourceStillMatches(path, prior)
	}
	result, err := idx.IndexFile(t.Context(), unit, path)
	if !errors.Is(err, os.ErrPermission) || !result.Stale {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	state, err := idx.store.ReadFileState(t.Context(), path)
	if err != nil || state.Exists {
		t.Fatalf("unreadable source retained state=%+v err=%v", state, err)
	}
}

func TestIndexFilePostCommitDeletionCannotRemoveNewerRegistration(t *testing.T) {
	idx, unit, root, db := indexerFixture(t)
	path := sourceFile(t, root, "a.py", "def old():\n    return 1\n")
	h2 := contentHash([]byte("def new():\n    pass\n"))
	checks := 0
	idx.recheck = func(path string, prior sourceVersion) (sourceVersion, bool, error) {
		checks++
		if checks == 3 {
			completed, err := idx.store.ReadFileState(t.Context(), path)
			if err != nil {
				t.Fatal(err)
			}
			if _, applied, err := idx.store.RegisterFileCAS(t.Context(), completed, unit.ID, path, "python", h2); err != nil || !applied {
				t.Fatalf("H2 register=%t err=%v", applied, err)
			}
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
		}
		return sourceStillMatches(path, prior)
	}
	result, err := idx.IndexFile(t.Context(), unit, path)
	if err != nil || !result.Stale {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	state, err := idx.store.ReadFileState(t.Context(), path)
	if err != nil || !state.Exists || state.State.State != StatePending || state.State.ContentHash != h2 {
		t.Fatalf("H2 lost=%+v err=%v", state, err)
	}
	var rows int
	if err := db.QueryRowContext(t.Context(), `SELECT count(*) FROM symbols WHERE path = ?`, path).Scan(&rows); err != nil || rows == 0 {
		t.Fatalf("newer registration's retained facts=%d err=%v", rows, err)
	}
}

func TestIndexFileCannotStealChildOwnedIndexedFile(t *testing.T) {
	idx, parent, root, db := indexerFixture(t)
	nested := filepath.Join(root, "nested")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	child, err := idx.store.UpsertUnit(t.Context(), nested, UnitPython)
	if err != nil {
		t.Fatal(err)
	}
	path := sourceFile(t, nested, "a.py", "def child():\n    pass\n")
	if _, err := idx.IndexFile(t.Context(), child, path); err != nil {
		t.Fatal(err)
	}
	var originalID int64
	if err := db.QueryRowContext(t.Context(), `SELECT id FROM symbols WHERE path = ?`, path).Scan(&originalID); err != nil {
		t.Fatal(err)
	}
	if _, err := idx.IndexFile(t.Context(), parent, path); err == nil {
		t.Fatal("stale parent stole child-owned indexed file")
	}
	state, err := idx.store.ReadFileState(t.Context(), path)
	if err != nil || state.State.UnitID != child.ID || state.State.State != StateIndexed {
		t.Fatalf("child state=%+v err=%v", state.State, err)
	}
	var persistedID int64
	if err := db.QueryRowContext(t.Context(), `SELECT id FROM symbols WHERE path = ?`, path).Scan(&persistedID); err != nil || persistedID != originalID {
		t.Fatalf("child symbol ID %d -> %d err=%v", originalID, persistedID, err)
	}
}

func TestIndexFileUniqueAndAmbiguousCalls(t *testing.T) {
	idx, unit, root, db := indexerFixture(t)
	path := sourceFile(t, root, "a.py", "def target():\n    pass\ntarget()\n")
	if _, err := idx.IndexFile(t.Context(), unit, path); err != nil {
		t.Fatal(err)
	}
	var resolved sql.NullInt64
	var label string
	var confidence float64
	if err := db.QueryRowContext(t.Context(), `SELECT resolved_symbol_id, label, confidence FROM symbol_references WHERE path = ? AND target_text = 'target' LIMIT 1`, path).Scan(&resolved, &label, &confidence); err != nil {
		t.Fatal(err)
	}
	if !resolved.Valid || label != "STRUCTURAL_NAME_MATCH" || confidence > .5 {
		t.Fatalf("unique ref=%v %q %g", resolved, label, confidence)
	}
	sourceFile(t, root, "a.py", "def target():\n    pass\ndef target():\n    pass\ntarget()\n")
	if _, err := idx.IndexFile(t.Context(), unit, path); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(t.Context(), `SELECT resolved_symbol_id, label, confidence FROM symbol_references WHERE path = ? AND target_text = 'target' LIMIT 1`, path).Scan(&resolved, &label, &confidence); err != nil {
		t.Fatal(err)
	}
	if resolved.Valid || label != "STRUCTURAL_NAME_MATCH" || confidence > .5 {
		t.Fatalf("ambiguous ref=%v %q %g", resolved, label, confidence)
	}
}

func TestIndexFileResumeSkipsCompletedAndProcessesOnlyRemainder(t *testing.T) {
	idx, unit, root, _ := indexerFixture(t)
	paths := []string{
		sourceFile(t, root, "a.py", "def a():\n    pass\n"),
		sourceFile(t, root, "b.py", "def b():\n    pass\n"),
		sourceFile(t, root, "c.py", "def c():\n    pass\n"),
	}
	if _, err := idx.IndexFile(t.Context(), unit, paths[0]); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	idx.extract = func(context.Context, parser.SyntaxAdapter, parser.SourceFile) (parser.Facts, error) {
		cancel()
		return parser.Facts{}, context.Canceled
	}
	if _, err := idx.IndexFile(ctx, unit, paths[1]); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel=%v", err)
	}
	var calls int
	idx.extract = func(ctx context.Context, a parser.SyntaxAdapter, s parser.SourceFile) (parser.Facts, error) {
		calls++
		return parser.Extract(ctx, a, s)
	}
	for _, path := range paths {
		if _, err := idx.IndexFile(t.Context(), unit, path); err != nil {
			t.Fatalf("resume %s: %v", path, err)
		}
	}
	if calls != 2 {
		t.Fatalf("resume parsed %d files, want pending and unrecorded only", calls)
	}
	for _, path := range paths {
		state, err := idx.store.ReadFileState(t.Context(), path)
		if err != nil || state.State.State != StateIndexed {
			t.Fatalf("state %s = %+v, %v", path, state.State, err)
		}
	}
}

func TestIndexFileWarmDeletedAndCorruptCacheProduceSameDurableFacts(t *testing.T) {
	idx, unit, root, db := indexerFixture(t)
	cacheDir := filepath.Join(t.TempDir(), "cache")
	idx.cache = snapshot.New(filesystem.RuntimePaths{CacheDir: cacheDir})
	first := "from .lib import tool as renamed\ndef helper():\n    return 1\nclass Box:\n    def run(self):\n        import os\n        return helper()\nBox().run()\n"
	other := "from .other import item as alternate\ndef other():\n    return 2\nother()\n"
	path := sourceFile(t, root, "a.py", first)
	var calls int
	idx.extract = func(ctx context.Context, a parser.SyntaxAdapter, s parser.SourceFile) (parser.Facts, error) {
		calls++
		return parser.Extract(ctx, a, s)
	}
	indexContent := func(content string) {
		t.Helper()
		sourceFile(t, root, "a.py", content)
		if _, err := idx.IndexFile(t.Context(), unit, path); err != nil {
			t.Fatal(err)
		}
	}
	readFacts := func() string { return semanticFileFacts(t, db, path) }
	indexContent(first)
	baseline := readFacts()
	var nested, aliased, scopedImport, resolved int
	if err := db.QueryRowContext(t.Context(), `SELECT count(*) FROM symbols WHERE path = ? AND container <> ''`, path).Scan(&nested); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(t.Context(), `SELECT count(*) FROM symbol_imports WHERE path = ? AND alias = 'renamed' AND is_relative = 1`, path).Scan(&aliased); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(t.Context(), `SELECT count(*) FROM symbol_imports WHERE path = ? AND importer_key <> ''`, path).Scan(&scopedImport); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(t.Context(), `SELECT count(*) FROM symbol_references WHERE path = ? AND target_text = 'helper' AND resolved_symbol_id IS NOT NULL`, path).Scan(&resolved); err != nil {
		t.Fatal(err)
	}
	if nested == 0 || aliased == 0 || scopedImport == 0 || resolved == 0 {
		t.Fatalf("fixture lacked nested symbol, aliased/scoped import or resolved call: %d/%d/%d/%d", nested, aliased, scopedImport, resolved)
	}
	if calls != 1 {
		t.Fatalf("cold parse calls=%d", calls)
	}
	indexContent(other)
	before := calls
	indexContent(first)
	if calls != before || readFacts() != baseline {
		t.Fatalf("warm cache calls=%d before=%d facts=%q baseline=%q", calls, before, readFacts(), baseline)
	}
	if err := os.RemoveAll(cacheDir); err != nil {
		t.Fatal(err)
	}
	indexContent(other)
	before = calls
	indexContent(first)
	if calls != before+1 || readFacts() != baseline {
		t.Fatalf("deleted cache calls=%d before=%d facts=%q", calls, before, readFacts())
	}
	err := filepath.WalkDir(cacheDir, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type().IsRegular() {
			return os.WriteFile(path, []byte("corrupt"), 0o600)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	indexContent(other)
	before = calls
	indexContent(first)
	if calls != before+1 || readFacts() != baseline {
		t.Fatalf("corrupt cache calls=%d before=%d facts=%q", calls, before, readFacts())
	}
}

// semanticFileFacts compares every persisted semantic column and the identity
// of each resolved target. Auto IDs, timestamps and attempt generations are
// intentionally excluded because re-extraction allocates new rows.
func semanticFileFacts(t *testing.T, db *sql.DB, path string) string {
	t.Helper()
	queries := map[string]string{
		"state": `SELECT unit_id, language, content_hash, state, last_error FROM file_index_state WHERE path = ?`,
		"symbols": `SELECT logical_key, kind, name, container, start_line, start_col, end_line, end_col,
			signature_hash, body_hash, structure_hash FROM symbols WHERE path = ? ORDER BY logical_key, start_line, start_col, id`,
		"imports": `SELECT importer_key, module, names, alias, is_relative FROM symbol_imports WHERE path = ? ORDER BY importer_key, module, names, alias, id`,
		"references": `SELECT r.referrer_key, r.target_text, r.scope_text, r.label, r.confidence,
			CASE WHEN r.resolved_symbol_id IS NULL THEN 0 ELSE 1 END,
			COALESCE(s.path, ''), COALESCE(s.logical_key, ''), COALESCE(s.kind, ''), COALESCE(s.name, ''),
			COALESCE(s.container, ''), COALESCE(s.start_line, -1), COALESCE(s.start_col, -1),
			COALESCE(s.end_line, -1), COALESCE(s.end_col, -1), COALESCE(s.signature_hash, ''),
			COALESCE(s.body_hash, ''), COALESCE(s.structure_hash, '')
			FROM symbol_references r LEFT JOIN symbols s ON s.id = r.resolved_symbol_id
			WHERE r.path = ? ORDER BY r.referrer_key, r.target_text, r.scope_text, r.id`,
	}
	result := make(map[string][][]any, len(queries))
	for name, query := range queries {
		rows, err := db.QueryContext(t.Context(), query, path)
		if err != nil {
			t.Fatal(err)
		}
		cols, err := rows.Columns()
		if err != nil {
			rows.Close()
			t.Fatal(err)
		}
		for rows.Next() {
			values := make([]any, len(cols))
			pointers := make([]any, len(cols))
			for i := range values {
				pointers[i] = &values[i]
			}
			if err := rows.Scan(pointers...); err != nil {
				rows.Close()
				t.Fatal(err)
			}
			for i, value := range values {
				if bytes, ok := value.([]byte); ok {
					values[i] = string(bytes)
				}
			}
			result[name] = append(result[name], values)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			t.Fatal(err)
		}
		if err := rows.Close(); err != nil {
			t.Fatal(err)
		}
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}

func TestSemanticFileFactsDetectsWrongNonNullResolvedTarget(t *testing.T) {
	idx, unit, root, db := indexerFixture(t)
	path := sourceFile(t, root, "a.py", "def helper(): pass\ndef other(): pass\nhelper()\n")
	if _, err := idx.IndexFile(t.Context(), unit, path); err != nil {
		t.Fatal(err)
	}
	baseline := semanticFileFacts(t, db, path)
	var otherID int64
	if err := db.QueryRowContext(t.Context(), `SELECT id FROM symbols WHERE path = ? AND name = 'other'`, path).Scan(&otherID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(t.Context(), `UPDATE symbol_references SET resolved_symbol_id = ? WHERE path = ? AND target_text = 'helper'`, otherID, path); err != nil {
		t.Fatal(err)
	}
	if changed := semanticFileFacts(t, db, path); changed == baseline {
		t.Fatal("semantic comparison treated wrong non-NULL target as equal")
	}
}

func TestIndexFilePersistsPartialFactsWithFailedState(t *testing.T) {
	idx, unit, root, db := indexerFixture(t)
	path := sourceFile(t, root, "partial.py", "def intact(): pass\ndef broken(:\n")
	result, err := idx.IndexFile(t.Context(), unit, path)
	payload, ok := app.PayloadOf(err)
	if !ok || payload.Code != app.CodeSyntaxParseFailed || result.State.State != StateFailed {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	var name string
	if err := db.QueryRowContext(t.Context(), `SELECT name FROM symbols WHERE path = ?`, path).Scan(&name); err != nil || name != "intact" {
		t.Fatalf("partial symbol=%q err=%v", name, err)
	}
}

func TestIndexFilePersistsFourLanguagePackageFacts(t *testing.T) {
	fixtures := []struct{ name, source string }{
		{"python.py", "import os\nimport sys as system\nfrom .lib import tool as renamed\ndef helper(value):\n    return value\nclass Box:\n    def run(self):\n        return helper(1)\n"},
		{"javascript.js", "import main, {tool as renamed} from './lib';\nimport * as ns from 'pkg';\nfunction helper(value) { return value; }\nclass Box { run() { return helper(1); } }\n"},
		{"typescript.ts", "import main, {tool as renamed} from './lib';\nimport * as ns from 'pkg';\nfunction helper(value: number): number { return value; }\nclass Box { run(): number { return helper(1); } }\n"},
		{"tsx.tsx", "import main, {tool as renamed} from './lib';\nimport * as ns from 'pkg';\nfunction helper(value: number): number { return value; }\nclass Box { run() { return <div>{helper(1)}</div>; } }\n"},
	}
	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			idx, unit, root, db := indexerFixture(t)
			path := sourceFile(t, root, fixture.name, fixture.source)
			result, err := idx.IndexFile(t.Context(), unit, path)
			if err != nil || result.State.State != StateIndexed {
				t.Fatalf("result=%+v err=%v", result, err)
			}
			var functions, classes, methods, imports, aliased, relative, calls, completeHashes int
			for query, dest := range map[string]*int{
				`SELECT count(*) FROM symbols WHERE path = ? AND name = 'helper'`:                                                                        &functions,
				`SELECT count(*) FROM symbols WHERE path = ? AND name = 'Box'`:                                                                           &classes,
				`SELECT count(*) FROM symbols WHERE path = ? AND name = 'run' AND kind = 'method'`:                                                       &methods,
				`SELECT count(*) FROM symbol_imports WHERE path = ?`:                                                                                     &imports,
				`SELECT count(*) FROM symbol_imports WHERE path = ? AND alias = 'renamed' AND names LIKE '%tool%'`:                                       &aliased,
				`SELECT count(*) FROM symbol_imports WHERE path = ? AND is_relative = 1`:                                                                 &relative,
				`SELECT count(*) FROM symbol_references WHERE path = ? AND target_text = 'helper' AND resolved_symbol_id IS NOT NULL`:                    &calls,
				`SELECT count(*) FROM symbols WHERE path = ? AND length(signature_hash) = 64 AND length(body_hash) = 64 AND length(structure_hash) = 64`: &completeHashes,
			} {
				if err := db.QueryRowContext(t.Context(), query, path).Scan(dest); err != nil {
					t.Fatal(err)
				}
			}
			if functions != 1 || classes != 1 || methods != 1 || imports != 3 || aliased != 1 || relative == 0 || calls == 0 || completeHashes < 3 {
				t.Fatalf("rows: functions=%d classes=%d methods=%d imports=%d aliased=%d relative=%d calls=%d hashes=%d", functions, classes, methods, imports, aliased, relative, calls, completeHashes)
			}
		})
	}
}

func TestIndexFileStoredFingerprintsSeparateBodyAndSignature(t *testing.T) {
	idx, unit, root, db := indexerFixture(t)
	path := sourceFile(t, root, "a.py", "def f(x):\n    return x + 1\n")
	read := func() (string, string, string) {
		t.Helper()
		if _, err := idx.IndexFile(t.Context(), unit, path); err != nil {
			t.Fatal(err)
		}
		var signature, body, structure string
		if err := db.QueryRowContext(t.Context(), `SELECT signature_hash, body_hash, structure_hash FROM symbols WHERE path = ? AND name = 'f'`, path).Scan(&signature, &body, &structure); err != nil {
			t.Fatal(err)
		}
		return signature, body, structure
	}
	sig1, body1, shape1 := read()
	sourceFile(t, root, "a.py", "def f(x):\n    return x * 2\n")
	sig2, body2, shape2 := read()
	if sig1 != sig2 || body1 == body2 || shape1 != shape2 {
		t.Fatalf("body edit hashes: %q/%q/%q -> %q/%q/%q", sig1, body1, shape1, sig2, body2, shape2)
	}
	sourceFile(t, root, "a.py", "def f(x, y):\n    return x * 2\n")
	sig3, body3, shape3 := read()
	if sig2 == sig3 || body2 != body3 || shape2 == shape3 {
		t.Fatalf("signature edit hashes: %q/%q/%q -> %q/%q/%q", sig2, body2, shape2, sig3, body3, shape3)
	}
}

func TestIndexFileNeverParsesInsideWriteTransaction(t *testing.T) {
	idx, unit, root, _ := indexerFixture(t)
	path := sourceFile(t, root, "a.py", "def f():\n    pass\n")
	called := false
	idx.extract = func(ctx context.Context, a parser.SyntaxAdapter, s parser.SourceFile) (parser.Facts, error) {
		called = true
		if storage.TxActive(ctx) {
			t.Fatal("parser ran inside SQL write transaction")
		}
		return parser.Extract(ctx, a, s)
	}
	if _, err := idx.IndexFile(t.Context(), unit, path); err != nil || !called {
		t.Fatalf("parse called=%t err=%v", called, err)
	}
}
