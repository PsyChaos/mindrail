package changes_test

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/changes"
	"github.com/PsyChaos/mindrail/internal/filesystem"
	"github.com/PsyChaos/mindrail/internal/git"
	"github.com/PsyChaos/mindrail/internal/index"
	"github.com/PsyChaos/mindrail/internal/index/parser"
	"github.com/PsyChaos/mindrail/internal/index/snapshot"
	"github.com/PsyChaos/mindrail/internal/migration"
	"github.com/PsyChaos/mindrail/internal/storage"
	"github.com/PsyChaos/mindrail/migrations"
)

const changeProject = "PRJ-TEST-CHANGE"

type serviceFixture struct {
	store   *changes.Store
	indexes *index.Store
	service *changes.Service
	indexer *index.Indexer
	db      *sql.DB
	root    string
	units   map[string]index.ProjectUnit
}

func newServiceFixture(t *testing.T) serviceFixture {
	t.Helper()
	root := t.TempDir()
	db, err := storage.Open(t.Context(), storage.Options{Path: filepath.Join(root, "mindrail.db")})
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
	store, err := changes.NewStore(db.DB, clock)
	if err != nil {
		t.Fatal(err)
	}
	indexes := index.NewStore(db.DB, clock)
	registry, err := parser.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(registry.Close)
	indexer := index.NewIndexer(indexes, registry, snapshot.New(filesystem.RuntimePaths{CacheDir: filepath.Join(root, "cache")}))
	service, err := changes.New(store, indexes, indexer)
	if err != nil {
		t.Fatal(err)
	}
	_ = indexer
	units := map[string]index.ProjectUnit{}
	for dir, kind := range map[string]index.UnitKind{"py": index.UnitPython, "ts": index.UnitTypeScript, "js": index.UnitJavaScript} {
		pkg := filepath.Join(root, dir)
		marker := "pyproject.toml"
		if kind != index.UnitPython {
			marker = "package.json"
		}
		if err := os.MkdirAll(pkg, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(pkg, marker), []byte("{}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if kind == index.UnitTypeScript {
			if err := os.WriteFile(filepath.Join(pkg, "tsconfig.json"), []byte("{}\n"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		unit, err := indexes.UpsertUnit(t.Context(), pkg, kind)
		if err != nil {
			t.Fatal(err)
		}
		units[dir] = unit
	}
	seedTasks(t, db.DB, "TSK-1", "TSK-ts", "TSK-js")
	return serviceFixture{store: store, indexes: indexes, service: service, indexer: indexer, db: db.DB, root: root, units: units}
}

func seedTasks(t *testing.T, db *sql.DB, tasks ...string) {
	t.Helper()
	seed := func(statement string, args ...any) {
		t.Helper()
		if _, err := db.ExecContext(t.Context(), statement, args...); err != nil {
			t.Fatal(err)
		}
	}
	seed(`INSERT OR IGNORE INTO projects (project_id, common_dir, registered_at)
		VALUES ('PRJ-1', '/repo/.git', '2026-09-23T10:00:00Z')`)
	seed(`INSERT OR IGNORE INTO workspaces (workspace_id, project_id, root_path, git_dir, is_linked_worktree, registered_at, last_seen_at)
		VALUES ('WS-1', 'PRJ-1', '/repo', '/repo/.git', 0, '2026-09-23T10:00:00Z', '2026-09-23T10:00:00Z')`)
	seed(`INSERT OR IGNORE INTO sessions (session_id, workspace_id, label, started_at)
		VALUES ('SES-1', 'WS-1', 's', '2026-09-23T10:00:00Z')`)
	for _, task := range tasks {
		seed(`INSERT OR IGNORE INTO tasks (task_id, project_id, title, state, opened_by, created_at, updated_at)
			VALUES (?, 'PRJ-1', 't', 'OPEN', 'SES-1', '2026-09-23T10:00:00Z', '2026-09-23T10:00:00Z')`, task)
	}
}

// afterChange runs the baseline path: capture scope, edit, AfterChange.
func afterChange(t *testing.T, f serviceFixture, task string, edits map[string]string) changes.Change {
	t.Helper()
	paths := make([]string, 0, len(edits))
	for rel := range edits {
		paths = append(paths, filepath.Join(f.root, filepath.FromSlash(rel)))
	}
	if _, err := f.store.CaptureBaseline(t.Context(), task, paths, ""); err != nil {
		t.Fatal(err)
	}
	for rel, content := range edits {
		path := filepath.Join(f.root, filepath.FromSlash(rel))
		if content == "" {
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	change, err := f.service.AfterChange(t.Context(), changeProject, f.root, task, "")
	if err != nil {
		t.Fatal(err)
	}
	return change
}

// TestSymbolFlagsPerLanguage is AC-04.1: body edits flag body, signature
// edits flag signature and structure, added rows flag added, vanished rows
// flag removed — asserted on stored rows with current hashes.
func TestSymbolFlagsPerLanguage(t *testing.T) {
	f := newServiceFixture(t)
	py := "py/a.py"
	if _, err := f.store.CaptureBaseline(t.Context(), "TSK-1", []string{filepath.Join(f.root, py)}, ""); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(f.root, py), []byte("def f(x):\n    return x + 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	change, err := f.service.AfterChange(t.Context(), changeProject, f.root, "TSK-1", "")
	if err != nil {
		t.Fatal(err)
	}
	rows, err := f.store.ReadChangeSymbols(t.Context(), change.ID)
	if err != nil || len(rows) != 1 {
		t.Fatalf("rows = %+v, %v", rows, err)
	}
	if rows[0].Kind != changes.SymbolAdded || rows[0].UID == "" {
		t.Fatalf("row = %+v, want added with uid", rows[0])
	}
	// Body-only edit: body flips, signature and structure hold.
	if err := os.WriteFile(filepath.Join(f.root, py), []byte("def f(x):\n    return x * 2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.AfterChange(t.Context(), changeProject, f.root, "TSK-1", ""); err != nil {
		t.Fatal(err)
	}
	rows, err = f.store.ReadChangeSymbols(t.Context(), change.ID)
	if err != nil || len(rows) != 1 {
		t.Fatalf("rows = %+v, %v", rows, err)
	}
	if rows[0].Kind != changes.SymbolModified || !rows[0].Body || rows[0].Signature || rows[0].Structure {
		t.Fatalf("row = %+v, want body-only modification", rows[0])
	}
	bodyUID := rows[0].UID
	// Signature edit: signature and structure flip, body holds, uid follows.
	if err := os.WriteFile(filepath.Join(f.root, py), []byte("def f(x, y):\n    return x * 2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.AfterChange(t.Context(), changeProject, f.root, "TSK-1", ""); err != nil {
		t.Fatal(err)
	}
	rows, err = f.store.ReadChangeSymbols(t.Context(), change.ID)
	if err != nil || len(rows) != 1 {
		t.Fatalf("rows = %+v, %v", rows, err)
	}
	if rows[0].Kind != changes.SymbolModified || rows[0].Body || !rows[0].Signature || !rows[0].Structure {
		t.Fatalf("row = %+v, want signature modification", rows[0])
	}
	if rows[0].UID != bodyUID || rows[0].UID == "" {
		t.Fatalf("uid moved %+v across edits", rows[0])
	}
	// Removal: the stored row flags removed with last-known hashes.
	if err := os.WriteFile(filepath.Join(f.root, py), []byte("x = 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.AfterChange(t.Context(), changeProject, f.root, "TSK-1", ""); err != nil {
		t.Fatal(err)
	}
	rows, err = f.store.ReadChangeSymbols(t.Context(), change.ID)
	if err != nil || len(rows) != 1 || rows[0].Kind != changes.SymbolRemoved {
		t.Fatalf("rows = %+v, %v", rows, err)
	}
}

// TestSymbolFlagsTypeScriptJavaScript runs the representative flag flow on
// the other two grammars: extraction, flags and uid following behave the
// same through the shared core.
func TestSymbolFlagsTypeScriptJavaScript(t *testing.T) {
	f := newServiceFixture(t)
	cases := []struct {
		dir     string
		file    string
		first   string
		second  string
		wantSig bool
	}{
		{"ts", "a.ts", "function f(x: number): number { return x + 1; }\n", "function f(x: number, y: number): number { return x + 1; }\n", true},
		{"js", "a.js", "function f(x) { return x + 1; }\n", "function f(x) { return x * 2; }\n", false},
	}
	for _, tc := range cases {
		t.Run(tc.dir, func(t *testing.T) {
			task := "TSK-" + tc.dir
			rel := tc.dir + "/" + tc.file
			path := filepath.Join(f.root, filepath.FromSlash(rel))
			if err := os.WriteFile(path, []byte(tc.first), 0o644); err != nil {
				t.Fatal(err)
			}
			if _, err := f.store.CaptureBaseline(t.Context(), task, []string{path}, ""); err != nil {
				t.Fatal(err)
			}
			// First contact always adds (no stored facts yet); the flag
			// assertion needs a second round against established facts, so
			// the first round edits before it runs.
			if err := os.WriteFile(path, []byte(tc.first+"\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			if _, err := f.service.AfterChange(t.Context(), changeProject, f.root, task, ""); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(tc.second), 0o644); err != nil {
				t.Fatal(err)
			}
			change, err := f.service.AfterChange(t.Context(), changeProject, f.root, task, "")
			if err != nil {
				t.Fatal(err)
			}
			rows, err := f.store.ReadChangeSymbols(t.Context(), change.ID)
			if err != nil || len(rows) != 1 {
				t.Fatalf("rows = %+v, %v", rows, err)
			}
			if tc.dir == "js" {
				if err := os.WriteFile(path, []byte("var x = 1\n"), 0o644); err != nil {
					t.Fatal(err)
				}
				if _, err := f.service.AfterChange(t.Context(), changeProject, f.root, task, ""); err != nil {
					t.Fatal(err)
				}
				gone, err := f.store.ReadChangeSymbols(t.Context(), change.ID)
				if err != nil || len(gone) != 1 || gone[0].Kind != changes.SymbolRemoved {
					t.Fatalf("removed = %+v, %v", gone, err)
				}
			}
			if rows[0].Kind != changes.SymbolModified || rows[0].UID == "" {
				t.Fatalf("row = %+v, want modified with uid", rows[0])
			}
			if rows[0].Signature != tc.wantSig || rows[0].Body == tc.wantSig || rows[0].Structure != tc.wantSig {
				t.Fatalf("row = %+v, want body=%t signature=%t structure=%t", rows[0], !tc.wantSig, tc.wantSig, tc.wantSig)
			}
		})
	}
}

// TestDiscoveryIndexesWhatItReads is AC-04.2: after discovery the stored
// facts equal a fresh parse, and a rename carries its uid through migration.
func TestDiscoveryIndexesWhatItReads(t *testing.T) {
	f := newServiceFixture(t)
	change := afterChange(t, f, "TSK-1", map[string]string{"py/a.py": "def old():\n    return 1\n"})
	path := filepath.Join(f.root, "py", "a.py")
	stored, err := f.indexes.ListSymbolsInFile(t.Context(), f.units["py"].ID, path)
	if err != nil || len(stored) != 1 {
		t.Fatalf("stored = %+v, %v", stored, err)
	}
	fresh := freshExtract(t, f, "py", path)
	if len(fresh) != len(stored) {
		t.Fatalf("fresh = %+v, stored = %+v", fresh, stored)
	}
	for i := range fresh {
		if fresh[i].LogicalKey != stored[i].LogicalKey || fresh[i].BodyHash != stored[i].BodyHash ||
			fresh[i].SignatureHash != stored[i].SignatureHash || fresh[i].StructureHash != stored[i].StructureHash {
			t.Fatalf("fresh %+v vs stored %+v", fresh[i], stored[i])
		}
	}
	changeRows, err := f.store.ReadChangeSymbols(t.Context(), change.ID)
	if err != nil || len(changeRows) != len(stored) {
		t.Fatalf("change rows = %+v, %v", changeRows, err)
	}
	for _, row := range changeRows {
		if row.SigHash == "" || row.BodyHash == "" || row.StructHash == "" {
			t.Fatalf("change row carries empty hashes: %+v", row)
		}
	}
	oldUID := stored[0].UID
	if oldUID == "" {
		t.Fatal("stored row has no uid")
	}
	// Rename through the same path: the change row follows the uid.
	if err := os.WriteFile(path, []byte("def new():\n    return 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	renamed, err := f.service.AfterChange(t.Context(), changeProject, f.root, "TSK-1", "")
	if err != nil {
		t.Fatal(err)
	}
	if renamed.ID != change.ID {
		t.Fatalf("after_change opened %s beside %s", renamed.ID, change.ID)
	}
	rows, err := f.store.ReadChangeSymbols(t.Context(), change.ID)
	if err != nil {
		t.Fatal(err)
	}
	var carried string
	for _, row := range rows {
		if row.Kind != changes.SymbolRemoved {
			carried = row.UID
		}
	}
	if carried != oldUID {
		t.Fatalf("renamed uid = %q, want carried %q (rows %+v)", carried, oldUID, rows)
	}
}

// freshExtract re-parses a file through the indexer's seam for the
// facts-equality half of AC-04.2: same adapters, no database writes.
func freshExtract(t *testing.T, f serviceFixture, dir, path string) []index.Symbol {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	symbols, err := f.indexer.ExtractCurrent(t.Context(), f.units[dir], path, content)
	if err != nil {
		t.Fatal(err)
	}
	return symbols
}

// TestAfterChangeEmptyDeltaReturnsOpenChange pins the no-change shape: a
// quiet run still answers the open change id with zero rows, so callers can
// never read a zero id as "no change".
func TestAfterChangeEmptyDeltaReturnsOpenChange(t *testing.T) {
	f := newServiceFixture(t)
	path := filepath.Join(f.root, "py", "a.py")
	if err := os.WriteFile(path, []byte("def f():\n    pass\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.CaptureBaseline(t.Context(), "TSK-1", []string{path}, ""); err != nil {
		t.Fatal(err)
	}
	change, err := f.service.AfterChange(t.Context(), changeProject, f.root, "TSK-1", "")
	if err != nil || change.ID == "" {
		t.Fatalf("quiet run = %+v, %v", change, err)
	}
	files, err := f.store.ReadChangeFiles(t.Context(), change.ID)
	if err != nil || len(files) != 0 {
		t.Fatalf("files = %+v, %v", files, err)
	}
}

// TestAfterChangeConverges is AC-04.3: calling twice converges on one change
// id and one row set — natural-key upserts make redelivery idempotent.
func TestAfterChangeConverges(t *testing.T) {
	f := newServiceFixture(t)
	first := afterChange(t, f, "TSK-1", map[string]string{"py/a.py": "def f():\n    pass\n"})
	second, err := f.service.AfterChange(t.Context(), changeProject, f.root, "TSK-1", "")
	if err != nil {
		t.Fatal(err)
	}
	if second.ID != first.ID {
		t.Fatalf("second run opened %s beside %s", second.ID, first.ID)
	}
	files, err := f.store.ReadChangeFiles(t.Context(), first.ID)
	if err != nil || len(files) != 1 {
		t.Fatalf("files = %+v, %v", files, err)
	}
	symbols, err := f.store.ReadChangeSymbols(t.Context(), first.ID)
	if err != nil || len(symbols) != 1 || symbols[0].Kind != changes.SymbolAdded {
		t.Fatalf("symbols = %+v, %v", symbols, err)
	}
	// Latest wins: a signature edit flips the stored flags in place.
	path := filepath.Join(f.root, "py", "a.py")
	if err := os.WriteFile(path, []byte("def f(x, y):\n    pass\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	third, err := f.service.AfterChange(t.Context(), changeProject, f.root, "TSK-1", "")
	if err != nil || third.ID != first.ID {
		t.Fatalf("third = %+v, %v", third, err)
	}
	flipped, err := f.store.ReadChangeSymbols(t.Context(), first.ID)
	if err != nil || len(flipped) != 1 || !flipped[0].Signature || flipped[0].Body {
		t.Fatalf("flipped = %+v, %v", flipped, err)
	}
}

// TestStagedMoveHintIsLive is AC-04.4: the same content moved with a
// Git-corroborated hint migrates, while the identical move without hints
// mints — proving the hint path reaches migration instead of dangling.
func TestStagedMoveHintIsLive(t *testing.T) {
	f := newServiceFixture(t)
	write := func(rel, content string) string {
		t.Helper()
		path := filepath.Join(f.root, filepath.FromSlash(rel))
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		return path
	}
	// With hints: a.py -> b.py migrates.
	a := filepath.Join(f.root, "py", "a.py")
	if _, err := f.store.CaptureBaseline(t.Context(), "TSK-1", []string{a}, ""); err != nil {
		t.Fatal(err)
	}
	write("py/a.py", "def f():\n    return 1\n")
	change, err := f.service.AfterChange(t.Context(), changeProject, f.root, "TSK-1", "")
	if err != nil {
		t.Fatal(err)
	}
	units := []index.ProjectUnit{f.units["py"]}
	b := write("py/b.py", "def f():\n    return 1\n")
	// Hints arrive through git.DiffRenames (staged rename), converted from
	// repo-relative to absolute: this pins the live path, not just the struct.
	runner := &git.FakeRunner{Responses: map[string]git.FakeResponse{
		"diff --no-color --name-status -z -M HEAD -- .": {Stdout: "R100\x00py/a.py\x00py/b.py\x00"},
	}}
	entries, err := git.DiffRenames(t.Context(), runner, f.root)
	if err != nil || len(entries) != 1 {
		t.Fatalf("entries = %+v, %v", entries, err)
	}
	var hints []index.RenameHint
	for _, entry := range entries {
		hints = append(hints, index.RenameHint{
			OldPath: filepath.Join(f.root, filepath.FromSlash(entry.OldPath)),
			NewPath: filepath.Join(f.root, filepath.FromSlash(entry.NewPath)),
		})
	}
	if err := f.service.SyncFileSymbols(t.Context(), changeProject, f.root, change.ID, b, "def f():\n    return 1\n", false, hints, changes.ViaReconcile, units); err != nil {
		t.Fatal(err)
	}
	uidsOf := func(path string) map[string]string {
		t.Helper()
		out := map[string]string{}
		rows, err := f.store.ReadChangeSymbols(t.Context(), change.ID)
		if err != nil {
			t.Fatal(err)
		}
		_ = rows
		symbols, err := f.indexes.ListSymbolsInFile(t.Context(), f.units["py"].ID, path)
		if err != nil {
			t.Fatal(err)
		}
		for _, row := range symbols {
			out[row.Name] = row.UID
		}
		return out
	}
	if uidsOf(b)["f"] != uidsOf(a)["f"] || uidsOf(a)["f"] == "" {
		t.Fatalf("hinted move lost the uid: %+v vs %+v", uidsOf(b), uidsOf(a))
	}
	// D-129 at row level: the moved file's delta row names the migrated uid,
	// not just the index store behind it.
	deltaRows, err := f.store.ReadChangeSymbols(t.Context(), change.ID)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, row := range deltaRows {
		if row.UID == uidsOf(a)["f"] && row.UID != "" {
			found = true
		}
	}
	if !found {
		t.Fatalf("no delta row names the migrated uid %+v", deltaRows)
	}
	// Without hints: identical content at c.py mints anew.
	c := write("py/c.py", "def f():\n    return 1\n")
	if err := f.service.SyncFileSymbols(t.Context(), changeProject, f.root, change.ID, c, "def f():\n    return 1\n", false, nil, changes.ViaReconcile, units); err != nil {
		t.Fatal(err)
	}
	if uidsOf(c)["f"] == uidsOf(a)["f"] {
		t.Fatalf("hintless move reused the uid")
	}
}

// TestAfterChangeAbortsOnBrokenFile pins fail-closed discovery: a file the
// parser disowns aborts the run instead of fabricating a delta around it.
func TestAfterChangeAbortsOnBrokenFile(t *testing.T) {
	f := newServiceFixture(t)
	path := filepath.Join(f.root, "py", "a.py")
	if err := os.WriteFile(path, []byte("def f():\n    pass\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.CaptureBaseline(t.Context(), "TSK-1", []string{path}, ""); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("def broken(:\n    pass\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.AfterChange(t.Context(), changeProject, f.root, "TSK-1", ""); err == nil {
		t.Fatal("broken file accepted")
	}
}

// TestAfterChangeMarksDeletedFileSymbolsRemoved pins D-128: deleting a file
// records every stored symbol removed with last-known hashes.
func TestAfterChangeMarksDeletedFileSymbolsRemoved(t *testing.T) {
	f := newServiceFixture(t)
	path := filepath.Join(f.root, "py", "a.py")
	if err := os.WriteFile(path, []byte("def f():\n    pass\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.CaptureBaseline(t.Context(), "TSK-1", []string{path}, ""); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("def f():\n    return 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	change, err := f.service.AfterChange(t.Context(), changeProject, f.root, "TSK-1", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.AfterChange(t.Context(), changeProject, f.root, "TSK-1", ""); err != nil {
		t.Fatal(err)
	}
	rows, err := f.store.ReadChangeSymbols(t.Context(), change.ID)
	if err != nil || len(rows) != 1 {
		t.Fatalf("rows = %+v, %v", rows, err)
	}
	if rows[0].Kind != changes.SymbolRemoved || rows[0].UID == "" || rows[0].BodyHash == "" {
		t.Fatalf("row = %+v, want removed with last-known identity and hashes", rows[0])
	}
	files, err := f.store.ReadChangeFiles(t.Context(), change.ID)
	if err != nil || len(files) != 1 || files[0].Kind != changes.FileDeleted {
		t.Fatalf("files = %+v, %v", files, err)
	}
}

// TestAfterChangeSkipsOutOfUnitScope pins the file-row-only contract: scope
// files no unit owns (prose, configs) succeed with file rows and no symbol
// delta, never an error.
func TestAfterChangeSkipsOutOfUnitScope(t *testing.T) {
	f := newServiceFixture(t)
	path := filepath.Join(f.root, "notes.md")
	if _, err := f.store.CaptureBaseline(t.Context(), "TSK-1", []string{path}, ""); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("# notes\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	change, err := f.service.AfterChange(t.Context(), changeProject, f.root, "TSK-1", "")
	if err != nil {
		t.Fatal(err)
	}
	files, err := f.store.ReadChangeFiles(t.Context(), change.ID)
	if err != nil || len(files) != 1 {
		t.Fatalf("files = %+v, %v", files, err)
	}
	symbols, err := f.store.ReadChangeSymbols(t.Context(), change.ID)
	if err != nil || len(symbols) != 0 {
		t.Fatalf("symbols = %+v, %v", symbols, err)
	}
}
