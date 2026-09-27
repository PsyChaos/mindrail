package index_test

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/filesystem"
	"github.com/PsyChaos/mindrail/internal/index"
	"github.com/PsyChaos/mindrail/internal/index/parser"
	"github.com/PsyChaos/mindrail/internal/index/snapshot"
	"github.com/PsyChaos/mindrail/internal/index/symbol"
	"github.com/PsyChaos/mindrail/internal/migration"
	"github.com/PsyChaos/mindrail/internal/storage"
	"github.com/PsyChaos/mindrail/migrations"
)

const identityProject = "PRJ-TEST-IDENTITY"

const migrationProject = "PRJ-TEST-MIGRATION"

// sourceFile writes content to name under root for the migration fixtures.
func sourceFile(t *testing.T, root, name, content string) string {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func identityFixture(t *testing.T) (*index.Store, *sql.DB, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "mindrail.db")
	db, err := storage.Open(t.Context(), storage.Options{Path: path})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	set, err := migration.Load(migrations.FS)
	if err != nil {
		t.Fatal(err)
	}
	clock := app.FixedClock{Instant: time.Date(2026, 9, 23, 10, 0, 0, 0, time.UTC)}
	if _, err := migration.New(db.DB, set, clock).Up(t.Context()); err != nil {
		t.Fatal(err)
	}
	return index.NewStore(db.DB, clock), db.DB, path
}

func identityUnit(t *testing.T, store *index.Store) index.ProjectUnit {
	t.Helper()
	unit, err := store.UpsertUnit(t.Context(), t.TempDir(), index.UnitPython)
	if err != nil {
		t.Fatal(err)
	}
	return unit
}

// TestEnsureIdentityIsStableAcrossCalls is AC-02.1: the same key resolves to
// the same uid, and the identities table holds exactly one row for it.
func TestEnsureIdentityIsStableAcrossCalls(t *testing.T) {
	store, db, _ := identityFixture(t)
	unit := identityUnit(t, store)
	service, err := symbol.New(store)
	if err != nil {
		t.Fatal(err)
	}
	first, err := service.EnsureIdentity(t.Context(), identityProject, unit, "python", "key")
	if err != nil || !strings.HasPrefix(first, "SYM-") {
		t.Fatalf("uid = %q, %v", first, err)
	}
	second, err := service.EnsureIdentity(t.Context(), identityProject, unit, "python", "key")
	if err != nil || second != first {
		t.Fatalf("second = %q, want %q", second, first)
	}
	var rows int
	if err := db.QueryRowContext(t.Context(), `SELECT count(*) FROM symbol_identities`).Scan(&rows); err != nil || rows != 1 {
		t.Fatalf("identity rows = %d, %v", rows, err)
	}
}

// TestConcurrentFirstSightAgreesOnOneUID is AC-02.2: sixteen racers over two
// separate database handles agree on one uid and one row. Separate handles
// are separate SQLite clients; only the UNIQUE index serializes them.
func TestConcurrentFirstSightAgreesOnOneUID(t *testing.T) {
	store, _, path := identityFixture(t)
	unit := identityUnit(t, store)
	db2, err := storage.Open(t.Context(), storage.Options{Path: path})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db2.Close() })
	stores := []*index.Store{store, index.NewStore(db2.DB, app.FixedClock{})}
	services := []*symbol.Service{}
	for _, st := range stores {
		service, err := symbol.New(st)
		if err != nil {
			t.Fatal(err)
		}
		services = append(services, service)
	}
	const racers = 16
	uids := make([]string, racers)
	errs := make([]error, racers)
	var ready sync.WaitGroup
	ready.Add(racers)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := range racers {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			ready.Done()
			<-start
			uids[i], errs[i] = services[i%2].EnsureIdentity(t.Context(), identityProject, unit, "python", "race-key")
		}(i)
	}
	ready.Wait()
	close(start)
	wg.Wait()
	for i := range racers {
		if errs[i] != nil || uids[i] != uids[0] {
			t.Fatalf("racer %d = %q, %v", i, uids[i], errs[i])
		}
	}
	var rows int
	// Either handle reads the same committed truth.
	if err := db2.DB.QueryRowContext(t.Context(), `SELECT count(*) FROM symbol_identities`).Scan(&rows); err != nil || rows != 1 {
		t.Fatalf("identity rows = %d, %v", rows, err)
	}
}

// TestBackfillStampsWithoutReparse is AC-02.3: rows written in the pre-MR-006
// shape (no uid column value) gain uids from lookup-or-mint, keys share, and
// a second run stamps zero. No parser is constructed anywhere in this test.
func TestBackfillStampsWithoutReparse(t *testing.T) {
	store, db, _ := identityFixture(t)
	unit := identityUnit(t, store)
	file := filepath.Join(unit.Path, "a.py")
	if _, err := db.ExecContext(t.Context(), `INSERT INTO file_index_state
		(path, unit_id, language, content_hash, state) VALUES (?, ?, 'python', 'h', 'indexed')`,
		file, unit.ID); err != nil {
		t.Fatal(err)
	}
	seed := func(name, key string) {
		t.Helper()
		if _, err := db.ExecContext(t.Context(), `INSERT INTO symbols
			(unit_id, path, logical_key, kind, name, start_line, start_col, end_line, end_col, signature_hash, body_hash, structure_hash)
			VALUES (?, ?, ?, 'function', ?, 1, 0, 2, 0, 's', 'b', 't')`,
			unit.ID, file, key, name); err != nil {
			t.Fatal(err)
		}
	}
	seed("f", "k1")
	seed("g", "k1")
	seed("h", "k2")
	stamped, err := store.BackfillUnit(t.Context(), identityProject, unit.ID)
	if err != nil || stamped != 3 {
		t.Fatalf("backfill stamped %d, %v", stamped, err)
	}
	rows, err := db.QueryContext(t.Context(), `SELECT logical_key, symbol_uid FROM symbols ORDER BY name`)
	if err != nil {
		t.Fatal(err)
	}
	uids := map[string]string{}
	for rows.Next() {
		var key, uid string
		if err := rows.Scan(&key, &uid); err != nil {
			rows.Close()
			t.Fatal(err)
		}
		if uid == "" {
			rows.Close()
			t.Fatalf("key %s left uid-less", key)
		}
		if seen, ok := uids[key]; ok && seen != uid {
			t.Fatalf("key %s has two uids", key)
		}
		uids[key] = uid
	}
	rows.Close()
	if len(uids) != 2 {
		t.Fatalf("distinct keyed uids = %d, want 2", len(uids))
	}
	again, err := store.BackfillUnit(t.Context(), identityProject, unit.ID)
	if err != nil || again != 0 {
		t.Fatalf("second backfill stamped %d, %v", again, err)
	}
}

// TestBackfillRefusesOrphanSymbols pins the fail-closed half of the join:
// uid-less rows with no file row are damage, and backfill reports them
// instead of silently leaving them behind.
func TestBackfillRefusesOrphanSymbols(t *testing.T) {
	store, db, _ := identityFixture(t)
	unit := identityUnit(t, store)
	if _, err := db.ExecContext(t.Context(), `INSERT INTO symbols
		(unit_id, path, logical_key, kind, name, start_line, start_col, end_line, end_col, signature_hash, body_hash, structure_hash)
		VALUES (?, ?, 'orphan', 'function', 'o', 1, 0, 2, 0, 's', 'b', 't')`,
		unit.ID, filepath.Join(unit.Path, "gone.py")); err != nil {
		t.Fatal(err)
	}
	if _, err := store.BackfillUnit(t.Context(), identityProject, unit.ID); err == nil {
		t.Fatal("backfill accepted orphan symbols")
	}
}

// TestCompletionKeepsUIDsAcrossIdenticalRewrites is AC-02.4: replacing a
// file's facts twice resolves the same uids instead of minting anew.
func TestCompletionKeepsUIDsAcrossIdenticalRewrites(t *testing.T) {
	store, db, _ := identityFixture(t)
	unit := identityUnit(t, store)
	path := filepath.Join(unit.Path, "a.py")
	if err := store.UpsertFileState(t.Context(), index.FileIndexState{
		UnitID: unit.ID, Path: path, Language: "python", State: index.StatePending,
	}); err != nil {
		t.Fatal(err)
	}
	complete := func(hash string) map[string]string {
		t.Helper()
		_, err := store.ReplaceFileFacts(t.Context(), index.FileFacts{
			ProjectID: testProjectID, UnitID: unit.ID, Path: path, Language: "python",
			ContentHash: hash, State: index.StateIndexed,
			Symbols: []index.Symbol{{LogicalKey: "k", Kind: "function", Name: "f", SignatureHash: "s", BodyHash: "b", StructureHash: "t"}},
		})
		if err != nil {
			t.Fatal(err)
		}
		out := map[string]string{}
		rows, err := db.QueryContext(t.Context(), `SELECT logical_key, symbol_uid FROM symbols WHERE path = ?`, path)
		if err != nil {
			t.Fatal(err)
		}
		for rows.Next() {
			var key string
			var uid sql.NullString
			if err := rows.Scan(&key, &uid); err != nil {
				rows.Close()
				t.Fatal(err)
			}
			out[key] = uid.String
		}
		rows.Close()
		return out
	}
	first := complete("h1")
	second := complete("h2")
	if len(first) != 1 || first["k"] == "" || first["k"] != second["k"] {
		t.Fatalf("uids %v -> %v across rewrites", first, second)
	}
}

// TestOverloadsSharingAKeyShareOneUID is AC-02.5: one lineage, one uid, no
// discriminator column in 0.1.
func TestOverloadsSharingAKeyShareOneUID(t *testing.T) {
	store, db, _ := identityFixture(t)
	unit := identityUnit(t, store)
	path := filepath.Join(unit.Path, "a.py")
	if err := store.UpsertFileState(t.Context(), index.FileIndexState{
		UnitID: unit.ID, Path: path, Language: "python", State: index.StatePending,
	}); err != nil {
		t.Fatal(err)
	}
	_, err := store.ReplaceFileFacts(t.Context(), index.FileFacts{
		ProjectID: testProjectID, UnitID: unit.ID, Path: path, Language: "python",
		ContentHash: "h", State: index.StateIndexed,
		Symbols: []index.Symbol{
			{LogicalKey: "over", Kind: "function", Name: "f", StartLine: 1, SignatureHash: "s1", BodyHash: "b1", StructureHash: "t1"},
			{LogicalKey: "over", Kind: "function", Name: "f", StartLine: 5, SignatureHash: "s2", BodyHash: "b2", StructureHash: "t2"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	var uids int
	if err := db.QueryRowContext(t.Context(), `SELECT count(DISTINCT symbol_uid) FROM symbols WHERE path = ?`, path).Scan(&uids); err != nil || uids != 1 {
		t.Fatalf("distinct uids = %d, %v", uids, err)
	}
	var identities int
	if err := db.QueryRowContext(t.Context(), `SELECT count(*) FROM symbol_identities`).Scan(&identities); err != nil || identities != 1 {
		t.Fatalf("identity rows = %d, %v", identities, err)
	}
}

// TestIndexerRejectsEmptyProject pins the identity-scope validation at the
// indexing entry: no project means no minting namespace, so nothing is read.
func TestIndexerRejectsEmptyProject(t *testing.T) {
	store, _, _ := identityFixture(t)
	unit := identityUnit(t, store)
	path := filepath.Join(unit.Path, "a.py")
	if err := os.WriteFile(path, []byte("def f():\n    pass\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	registry, err := parser.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(registry.Close)
	idx := index.NewIndexer(store, registry, snapshot.New(filesystem.RuntimePaths{CacheDir: filepath.Join(t.TempDir(), "cache")}))
	if _, err := idx.IndexFile(t.Context(), "", unit, path); err == nil {
		t.Fatal("indexing accepted an empty project")
	}
	state, err := store.ReadFileState(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	if state.Exists {
		t.Fatalf("refused indexing left state: %+v", state.State)
	}
}

// TestIdentityGuardsRejectEmptyScope pins the fail-closed validation behind
// lookup, mint, backfill and the service seam.
func TestIdentityGuardsRejectEmptyScope(t *testing.T) {
	store, _, _ := identityFixture(t)
	unit := identityUnit(t, store)
	service, err := symbol.New(store)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.LookupIdentity(t.Context(), "", unit.ID, "python", "k"); err == nil {
		t.Fatal("lookup accepted an empty project")
	}
	if _, err := store.MintIdentity(t.Context(), identityProject, "", "python", "k"); err == nil {
		t.Fatal("mint accepted an empty unit")
	}
	if _, err := service.EnsureIdentity(t.Context(), identityProject, unit, "", "k"); err == nil {
		t.Fatal("service accepted an empty language")
	}
	if _, err := store.BackfillUnit(t.Context(), identityProject, ""); err == nil {
		t.Fatal("backfill accepted an empty unit")
	}
	missingProject := filepath.Join(unit.Path, "missing-project.py")
	if err := store.UpsertFileState(t.Context(), index.FileIndexState{
		UnitID: unit.ID, Path: missingProject, Language: "python", State: index.StatePending,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ReplaceFileFacts(t.Context(), index.FileFacts{
		UnitID: unit.ID, Path: missingProject, Language: "python",
		ContentHash: "h", State: index.StateIndexed,
	}); err == nil {
		t.Fatal("replacement accepted a missing project")
	} else if payload, ok := app.PayloadOf(err); !ok || payload.Code != app.CodeCommandLineInvalid {
		t.Fatalf("missing project error = %v, want usage refusal", err)
	}
	if _, err := symbol.New(nil); err == nil {
		t.Fatal("service accepted a nil store")
	}
}

// migrationFixture indexes one file and returns its symbol uids by name.

func migrationFixture(t *testing.T) (*index.Indexer, index.ProjectUnit, string, *sql.DB) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "mindrail.db")
	db, err := storage.Open(t.Context(), storage.Options{Path: path})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	set, err := migration.Load(migrations.FS)
	if err != nil {
		t.Fatal(err)
	}
	clock := app.FixedClock{Instant: time.Date(2026, 9, 23, 10, 0, 0, 0, time.UTC)}
	if _, err := migration.New(db.DB, set, clock).Up(t.Context()); err != nil {
		t.Fatal(err)
	}
	store := index.NewStore(db.DB, clock)
	root := t.TempDir()
	unit, err := store.UpsertUnit(t.Context(), root, index.UnitPython)
	if err != nil {
		t.Fatal(err)
	}
	registry, err := parser.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(registry.Close)
	cache := snapshot.New(filesystem.RuntimePaths{CacheDir: filepath.Join(t.TempDir(), "cache")})
	return index.NewIndexer(store, registry, cache), unit, root, db.DB
}

// allocDatabase opens a migratable database at a stable path for the
// subprocess agreement test: workers reopen the same file.
func allocDatabase(t *testing.T) (*index.Store, *sql.DB, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "alloc.db")
	db, err := storage.Open(t.Context(), storage.Options{Path: path})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	set, err := migration.Load(migrations.FS)
	if err != nil {
		t.Fatal(err)
	}
	clock := app.FixedClock{}
	if _, err := migration.New(db.DB, set, clock).Up(t.Context()); err != nil {
		t.Fatal(err)
	}
	return index.NewStore(db.DB, clock), db.DB, path
}

// allocWorker is the subprocess half of AC-04.7: it opens nobody else's
// memory, mints through SQLite alone, and prints the agreed uid.
func allocWorker(t *testing.T) {
	t.Helper()
	db, err := storage.Open(t.Context(), storage.Options{Path: os.Getenv("MINDRAIL_TEST_ALLOC_DB")})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	store := index.NewStore(db.DB, app.SystemClock{})
	identity, err := store.MintIdentity(t.Context(), migrationProject,
		os.Getenv("MINDRAIL_TEST_ALLOC_UNIT"), "python", "race-key")
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println(identity.UID)
}

func uidsByName(t *testing.T, db *sql.DB, path string) map[string]string {
	t.Helper()
	rows, err := db.QueryContext(t.Context(), `SELECT name, symbol_uid FROM symbols WHERE path = ?`, path)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var name string
		var uid sql.NullString
		if err := rows.Scan(&name, &uid); err != nil {
			t.Fatal(err)
		}
		out[name] = uid.String
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func identityCount(t *testing.T, db *sql.DB) int {
	t.Helper()
	var n int
	if err := db.QueryRowContext(t.Context(), `SELECT count(*) FROM symbol_identities`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func ambiguityRows(t *testing.T, db *sql.DB) []struct {
	removedUID string
	removedKey string
	candidates string
} {
	t.Helper()
	rows, err := db.QueryContext(t.Context(), `SELECT removed_uid, removed_key, candidate_keys FROM symbol_identity_ambiguities ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []struct {
		removedUID string
		removedKey string
		candidates string
	}
	for rows.Next() {
		var row struct {
			removedUID string
			removedKey string
			candidates string
		}
		if err := rows.Scan(&row.removedUID, &row.removedKey, &row.candidates); err != nil {
			t.Fatal(err)
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

// TestRenameMigratesUID is AC-04.1: the uid follows the new key, the
// abandoned key joins the lineage memory, and no new identity is minted.
func TestRenameMigratesUID(t *testing.T) {
	idx, unit, root, db := migrationFixture(t)
	path := sourceFile(t, root, "a.py", "def old():\n    return 1\n")
	if _, err := idx.IndexFile(t.Context(), migrationProject, unit, path); err != nil {
		t.Fatal(err)
	}
	before := uidsByName(t, db, path)
	oldUID := before["old"]
	if oldUID == "" {
		t.Fatalf("old has no uid: %+v", before)
	}
	if _, err := db.ExecContext(t.Context(), `INSERT INTO invariant_symbol_bindings
		(invariant_id, symbol_uid, status, updated_at) VALUES ('INV-0001', ?, 'bound', '2026-09-23T10:00:00Z')`, oldUID); err != nil {
		t.Fatal(err)
	}
	var oldKey string
	if err := db.QueryRowContext(t.Context(), `SELECT logical_key FROM symbols WHERE path = ? AND name = 'old'`, path).Scan(&oldKey); err != nil {
		t.Fatal(err)
	}
	sourceFile(t, root, "a.py", "def new():\n    return 1\n")
	if _, err := idx.IndexFile(t.Context(), migrationProject, unit, path); err != nil {
		t.Fatal(err)
	}
	after := uidsByName(t, db, path)
	if after["new"] != oldUID {
		t.Fatalf("renamed uid = %q, want carried %q (all: %+v)", after["new"], oldUID, after)
	}
	if identityCount(t, db) != 1 {
		t.Fatal("rename minted a second identity")
	}
	var previousRaw string
	if err := db.QueryRowContext(t.Context(), `SELECT previous_keys FROM symbol_identities WHERE symbol_uid = ?`, oldUID).Scan(&previousRaw); err != nil {
		t.Fatal(err)
	}
	var previous []string
	if err := json.Unmarshal([]byte(previousRaw), &previous); err != nil {
		t.Fatal(err)
	}
	if len(previous) != 1 || previous[0] != oldKey {
		t.Fatalf("previous_keys = %s, want exactly abandoned %s", previousRaw, oldKey)
	}
	if rows := ambiguityRows(t, db); len(rows) != 0 {
		t.Fatalf("clean rename recorded %d ambiguities", len(rows))
	}
	var boundUID, boundStatus string
	if err := db.QueryRowContext(t.Context(), `SELECT symbol_uid, status FROM invariant_symbol_bindings WHERE invariant_id = 'INV-0001'`).Scan(&boundUID, &boundStatus); err != nil {
		t.Fatal(err)
	}
	if boundUID != oldUID || boundStatus != "bound" {
		t.Fatalf("binding = (%s, %s), want the carried uid still bound", boundUID, boundStatus)
	}
}

// TestStableTwinsReindexCleanly is the H1 regression: reindexing an
// unchanged file whose siblings share bodies must not ambiguate stable
// lineages against themselves. Ancestors exclude staged keys.
func TestStableTwinsReindexCleanly(t *testing.T) {
	idx, unit, root, db := migrationFixture(t)
	path := sourceFile(t, root, "a.py", "def f1():\n    return 1\ndef f2():\n    return 1\n")
	if _, err := idx.IndexFile(t.Context(), migrationProject, unit, path); err != nil {
		t.Fatal(err)
	}
	before := uidsByName(t, db, path)
	sourceFile(t, root, "a.py", "def f1():\n    return 1\ndef f2():\n    return 1\n\n")
	if _, err := idx.IndexFile(t.Context(), migrationProject, unit, path); err != nil {
		t.Fatal(err)
	}
	after := uidsByName(t, db, path)
	if after["f1"] != before["f1"] || after["f2"] != before["f2"] || after["f1"] == "" {
		t.Fatalf("stable uids moved %+v -> %+v", before, after)
	}
	if rows := ambiguityRows(t, db); len(rows) != 0 {
		t.Fatalf("clean reindex recorded %d ambiguities", len(rows))
	}
}

// TestOntoOwnedKeyRecordsAmbiguity pins the no-steal trail: a stable key
// whose content changes into a disappeared ancestor's shape keeps its uid,
// and the absorbed lineage is recorded instead of orphaned silently.
func TestOntoOwnedKeyRecordsAmbiguity(t *testing.T) {
	idx, unit, root, db := migrationFixture(t)
	path := sourceFile(t, root, "a.py", "def f():\n    return 1\ndef g():\n    return 2\n")
	if _, err := idx.IndexFile(t.Context(), migrationProject, unit, path); err != nil {
		t.Fatal(err)
	}
	before := uidsByName(t, db, path)
	sourceFile(t, root, "a.py", "def g():\n    return 1\n")
	if _, err := idx.IndexFile(t.Context(), migrationProject, unit, path); err != nil {
		t.Fatal(err)
	}
	after := uidsByName(t, db, path)
	if after["g"] != before["g"] {
		t.Fatalf("survivor uid moved %q -> %q: takeover must not steal", before["g"], after["g"])
	}
	rows := ambiguityRows(t, db)
	if len(rows) != 1 || rows[0].removedUID != before["f"] {
		t.Fatalf("ambiguity rows = %+v, want the absorbed lineage named", rows)
	}
}

// TestCascadeTwinsAmbiguate pins phase-two counting under a renamed parent:
// two heirs meeting two removed ancestors ambiguate instead of swapping.
func TestCascadeTwinsAmbiguate(t *testing.T) {
	idx, unit, root, db := migrationFixture(t)
	path := sourceFile(t, root, "a.py", "class Box:\n    def p(self):\n        return 1\n    def q(self):\n        return 1\n")
	if _, err := idx.IndexFile(t.Context(), migrationProject, unit, path); err != nil {
		t.Fatal(err)
	}
	sourceFile(t, root, "a.py", "class Crate:\n    def p(self):\n        return 1\n    def q(self):\n        return 1\n")
	if _, err := idx.IndexFile(t.Context(), migrationProject, unit, path); err != nil {
		t.Fatal(err)
	}
	after := uidsByName(t, db, path)
	if after["p"] != "" || after["q"] != "" {
		t.Fatalf("twin heirs got uids %+v; counting must ambiguate", after)
	}
	if rows := ambiguityRows(t, db); len(rows) != 2 {
		t.Fatalf("ambiguity rows = %d, want one per heir", len(rows))
	}
}

// TestRemapTwinsMintAnew characterizes the documented residual: twins under
// a changed parent cannot meet the bar (no remap forms without a migrated
// parent), so the removed lineage orphans and both heirs mint fresh. Nothing
// is guessed and nothing is dropped silently.
func TestRemapTwinsMintAnew(t *testing.T) {
	idx, unit, root, db := migrationFixture(t)
	path := sourceFile(t, root, "a.py", "class Box:\n    def f(self):\n        return 1\n")
	if _, err := idx.IndexFile(t.Context(), migrationProject, unit, path); err != nil {
		t.Fatal(err)
	}
	before := uidsByName(t, db, path)
	sourceFile(t, root, "a.py", "class Crate:\n    def g1(self):\n        return 1\n    def g2(self):\n        return 1\n")
	if _, err := idx.IndexFile(t.Context(), migrationProject, unit, path); err != nil {
		t.Fatal(err)
	}
	after := uidsByName(t, db, path)
	if after["g1"] == "" || after["g1"] == before["f"] || after["g2"] == "" || after["g2"] == before["f"] || after["g1"] == after["g2"] {
		t.Fatalf("heirs = %+v, want two fresh distinct uids", after)
	}
	if rows := ambiguityRows(t, db); len(rows) != 0 {
		t.Fatalf("unmatched twins recorded %d ambiguities", len(rows))
	}
	var live int
	if err := db.QueryRowContext(t.Context(), `SELECT count(*) FROM symbols WHERE symbol_uid = ?`, before["f"]).Scan(&live); err != nil || live != 0 {
		t.Fatalf("removed lineage rows = %d, %v", live, err)
	}
}

// TestMoveWithGitHintMigrates is AC-04.2's first half: identical content at
// a new path migrates under a Git-corroborated move.
func TestMoveWithGitHintMigrates(t *testing.T) {
	idx, unit, root, db := migrationFixture(t)
	a := sourceFile(t, root, "a.py", "def f():\n    return 1\n")
	if _, err := idx.IndexFile(t.Context(), migrationProject, unit, a); err != nil {
		t.Fatal(err)
	}
	before := uidsByName(t, db, a)
	b := sourceFile(t, root, "b.py", "def f():\n    return 1\n")
	hints := []index.RenameHint{{OldPath: a, NewPath: b}}
	if _, err := idx.IndexFile(t.Context(), migrationProject, unit, b, hints...); err != nil {
		t.Fatal(err)
	}
	after := uidsByName(t, db, b)
	if after["f"] != before["f"] || after["f"] == "" {
		t.Fatalf("moved uid = %q, want carried %q", after["f"], before["f"])
	}
}

// TestMoveWithoutGitHintMintsAnew is AC-04.2's documented limitation: a
// changed path with no corroboration mints instead of guessing.
func TestMoveWithoutGitHintMintsAnew(t *testing.T) {
	idx, unit, root, db := migrationFixture(t)
	a := sourceFile(t, root, "a.py", "def f():\n    return 1\n")
	if _, err := idx.IndexFile(t.Context(), migrationProject, unit, a); err != nil {
		t.Fatal(err)
	}
	before := uidsByName(t, db, a)
	c := sourceFile(t, root, "c.py", "def f():\n    return 1\n")
	if _, err := idx.IndexFile(t.Context(), migrationProject, unit, c); err != nil {
		t.Fatal(err)
	}
	after := uidsByName(t, db, c)
	if after["f"] == "" || after["f"] == before["f"] {
		t.Fatalf("uncorroborated move uid = %q, want a fresh uid distinct from %q", after["f"], before["f"])
	}
	if rows := ambiguityRows(t, db); len(rows) != 0 {
		t.Fatalf("mint recorded %d ambiguities", len(rows))
	}
}

// TestClassRenameCascades is AC-04.3: the class migrates first and its
// methods follow through the container remap, with the pointer set.
func TestClassRenameCascades(t *testing.T) {
	idx, unit, root, db := migrationFixture(t)
	path := sourceFile(t, root, "a.py", "class Box:\n    def run(self):\n        return 1\n")
	if _, err := idx.IndexFile(t.Context(), migrationProject, unit, path); err != nil {
		t.Fatal(err)
	}
	before := uidsByName(t, db, path)
	sourceFile(t, root, "a.py", "class Crate:\n    def run(self):\n        return 1\n")
	if _, err := idx.IndexFile(t.Context(), migrationProject, unit, path); err != nil {
		t.Fatal(err)
	}
	after := uidsByName(t, db, path)
	if after["Crate"] != before["Box"] || after["run"] != before["run"] {
		t.Fatalf("cascaded uids = %+v, want carried %+v", after, before)
	}
	if identityCount(t, db) != 2 {
		t.Fatalf("cascade minted: %d identities", identityCount(t, db))
	}
	var container sql.NullString
	if err := db.QueryRowContext(t.Context(), `SELECT container_uid FROM symbol_identities WHERE symbol_uid = ?`, after["run"]).Scan(&container); err != nil {
		t.Fatal(err)
	}
	if !container.Valid || container.String != after["Crate"] {
		t.Fatalf("method container_uid = %+v, want the class uid", container)
	}
}

// TestTwinsRecordAmbiguity is AC-04.4: one removed key meeting two added keys
// records ambiguity, mints nothing, and leaves both heirs uid-less.
func TestTwinsRecordAmbiguity(t *testing.T) {
	idx, unit, root, db := migrationFixture(t)
	path := sourceFile(t, root, "a.py", "def f():\n    return 1\n")
	if _, err := idx.IndexFile(t.Context(), migrationProject, unit, path); err != nil {
		t.Fatal(err)
	}
	before := uidsByName(t, db, path)
	sourceFile(t, root, "a.py", "def g1():\n    return 1\ndef g2():\n    return 1\n")
	if _, err := idx.IndexFile(t.Context(), migrationProject, unit, path); err != nil {
		t.Fatal(err)
	}
	after := uidsByName(t, db, path)
	if after["g1"] != "" || after["g2"] != "" {
		t.Fatalf("twins got uids %+v; ambiguity mints nothing", after)
	}
	if identityCount(t, db) != 1 {
		t.Fatalf("twins minted: %d identities", identityCount(t, db))
	}
	rows := ambiguityRows(t, db)
	if len(rows) != 1 {
		t.Fatalf("ambiguity rows = %d, want exactly one", len(rows))
	}
	if rows[0].removedUID != before["f"] {
		t.Fatalf("removed uid = %s, want %s", rows[0].removedUID, before["f"])
	}
	var candidates []string
	if err := json.Unmarshal([]byte(rows[0].candidates), &candidates); err != nil {
		t.Fatal(err)
	}
	stagedRows, err := db.QueryContext(t.Context(), `SELECT logical_key FROM symbols WHERE path = ?`, path)
	if err != nil {
		t.Fatal(err)
	}
	staged := map[string]bool{}
	for stagedRows.Next() {
		var key string
		if err := stagedRows.Scan(&key); err != nil {
			stagedRows.Close()
			t.Fatal(err)
		}
		staged[key] = true
	}
	stagedRows.Close()
	if len(candidates) != 2 || !staged[candidates[0]] || !staged[candidates[1]] || candidates[0] == candidates[1] {
		t.Fatalf("candidates = %s, want both staged heir keys", rows[0].candidates)
	}
}

// TestRenameOntoExistingKeyKeepsBothLineages pins the no-steal rule: a rename
// whose target key already has an identity keeps that identity, and the
// removed lineage orphans instead of merging.
func TestRenameOntoExistingKeyKeepsBothLineages(t *testing.T) {
	idx, unit, root, db := migrationFixture(t)
	path := sourceFile(t, root, "a.py", "def f():\n    return 1\ndef g():\n    return 2\n")
	if _, err := idx.IndexFile(t.Context(), migrationProject, unit, path); err != nil {
		t.Fatal(err)
	}
	before := uidsByName(t, db, path)
	sourceFile(t, root, "a.py", "def g():\n    return 2\n")
	if _, err := idx.IndexFile(t.Context(), migrationProject, unit, path); err != nil {
		t.Fatal(err)
	}
	after := uidsByName(t, db, path)
	if after["g"] != before["g"] {
		t.Fatalf("survivor uid moved %q -> %q", before["g"], after["g"])
	}
	if rows := ambiguityRows(t, db); len(rows) != 0 {
		t.Fatalf("merge recorded %d ambiguities", len(rows))
	}
	var live int
	if err := db.QueryRowContext(t.Context(), `SELECT count(*) FROM symbols WHERE symbol_uid = ?`, before["f"]).Scan(&live); err != nil || live != 0 {
		t.Fatalf("removed lineage rows = %d, %v", live, err)
	}
}

// TestMintLastAfterFailedMigration is AC-04.5: a rename that also changes the
// body meets no bar, mints exactly one new uid, and records nothing.
func TestMintLastAfterFailedMigration(t *testing.T) {
	idx, unit, root, db := migrationFixture(t)
	path := sourceFile(t, root, "a.py", "def f():\n    return 1\n")
	if _, err := idx.IndexFile(t.Context(), migrationProject, unit, path); err != nil {
		t.Fatal(err)
	}
	before := uidsByName(t, db, path)
	sourceFile(t, root, "a.py", "def g():\n    return 100\n")
	if _, err := idx.IndexFile(t.Context(), migrationProject, unit, path); err != nil {
		t.Fatal(err)
	}
	after := uidsByName(t, db, path)
	if after["g"] == "" || after["g"] == before["f"] {
		t.Fatalf("reworked uid = %q, want one fresh uid", after["g"])
	}
	if identityCount(t, db) != 2 {
		t.Fatalf("identities = %d, want old plus one mint", identityCount(t, db))
	}
	if rows := ambiguityRows(t, db); len(rows) != 0 {
		t.Fatalf("mint recorded %d ambiguities", len(rows))
	}
	var previous string
	if err := db.QueryRowContext(t.Context(), `SELECT previous_keys FROM symbol_identities WHERE symbol_uid = ?`, after["g"]).Scan(&previous); err != nil {
		t.Fatal(err)
	}
	if previous != "[]" && previous != "" {
		t.Fatalf("fresh mint carries lineage %s", previous)
	}
}

// TestMalformedHintsRefused pins the fail-closed hint validation.
func TestMalformedHintsRefused(t *testing.T) {
	idx, unit, root, _ := migrationFixture(t)
	path := sourceFile(t, root, "a.py", "def f():\n    pass\n")
	if _, err := idx.IndexFile(t.Context(), migrationProject, unit, path,
		index.RenameHint{OldPath: "relative/a.py", NewPath: path}); err == nil {
		t.Fatal("relative hint accepted")
	}
}

// TestConcurrentProcessesAgreeOnOneUID is AC-04.7 (decision D-109): two OS
// processes racing first sight agree through SQLite alone, where no Go
// memory is shared. The worker re-execs this test binary with an env flag.
func TestConcurrentProcessesAgreeOnOneUID(t *testing.T) {
	if os.Getenv("MINDRAIL_TEST_ALLOC_WORKER") == "1" {
		allocWorker(t)
		return
	}
	store, db, path := allocDatabase(t)
	unit := identityUnit(t, store)
	_ = db
	runWorker := func() string {
		t.Helper()
		cmd := exec.Command(os.Args[0], "-test.run=TestConcurrentProcessesAgreeOnOneUID")
		cmd.Env = append(os.Environ(),
			"MINDRAIL_TEST_ALLOC_WORKER=1",
			"MINDRAIL_TEST_ALLOC_DB="+path,
			"MINDRAIL_TEST_ALLOC_UNIT="+unit.ID,
		)
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("worker: %v\n%s", err, out)
		}
		// Timing lines vary with load; only the uid line is compared.
		for _, line := range strings.Split(string(out), "\n") {
			if uid := strings.TrimSpace(line); strings.HasPrefix(uid, "SYM-") {
				return uid
			}
		}
		t.Fatalf("worker printed no uid:\n%s", out)
		return ""
	}
	type result struct{ uid string }
	results := make(chan result, 2)
	go func() { results <- result{runWorker()} }()
	go func() { results <- result{runWorker()} }()
	first, second := <-results, <-results
	if first.uid == "" || first.uid != second.uid {
		t.Fatalf("process uids %q vs %q", first.uid, second.uid)
	}
	var rows int
	if err := db.QueryRowContext(t.Context(), `SELECT count(*) FROM symbol_identities`).Scan(&rows); err != nil || rows != 1 {
		t.Fatalf("identity rows = %d, %v", rows, err)
	}
}
