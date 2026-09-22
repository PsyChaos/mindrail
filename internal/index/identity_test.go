package index_test

import (
	"database/sql"
	"os"
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
