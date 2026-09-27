package snapshot_test

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/filesystem"
	"github.com/PsyChaos/mindrail/internal/git"
	"github.com/PsyChaos/mindrail/internal/index"
	"github.com/PsyChaos/mindrail/internal/index/parser"
	"github.com/PsyChaos/mindrail/internal/index/snapshot"
	"github.com/PsyChaos/mindrail/internal/migration"
	"github.com/PsyChaos/mindrail/internal/storage"
	"github.com/PsyChaos/mindrail/migrations"
)

const testProjectID = "PRJ-TEST-01"

func gitCommand(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), "git", append([]string{"-C", dir}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
}

func realWorktreePaths(t *testing.T) (filesystem.RuntimePaths, filesystem.RuntimePaths) {
	t.Helper()
	base := t.TempDir()
	main := filepath.Join(base, "main")
	linked := filepath.Join(base, "linked")
	if err := os.Mkdir(main, 0o700); err != nil {
		t.Fatal(err)
	}
	gitCommand(t, main, "init", "--quiet")
	gitCommand(t, main, "-c", "user.name=Mindrail", "-c", "user.email=test@example.invalid", "commit", "--quiet", "--allow-empty", "-m", "fixture")
	gitCommand(t, main, "worktree", "add", "--quiet", "--detach", linked)
	resolver := git.NewAdapter(git.NewExecRunner())
	var paths [2]filesystem.RuntimePaths
	for i, root := range []string{main, linked} {
		repo, err := resolver.Resolve(t.Context(), root)
		if err != nil {
			t.Fatalf("resolve %s: %v", root, err)
		}
		paths[i], err = filesystem.ResolveRuntimePaths(filesystem.PathOptions{CommonDir: repo.CommonDir, WorktreeRoot: repo.WorktreeRoot})
		if err != nil {
			t.Fatal(err)
		}
	}
	if paths[0].CacheDir != paths[1].CacheDir || paths[0].WorktreeRoot == paths[1].WorktreeRoot {
		t.Fatalf("worktree runtime paths do not share cache: %+v %+v", paths[0], paths[1])
	}
	return paths[0], paths[1]
}

func pythonAdapter(t *testing.T) parser.SyntaxAdapter {
	t.Helper()
	registry, err := parser.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(registry.Close)
	adapter, ok := registry.Lookup("file.py")
	if !ok {
		t.Fatal("Python adapter missing")
	}
	return adapter
}

func parseFacts(adapter parser.SyntaxAdapter, calls *int) snapshot.Computer {
	return func(ctx context.Context, content []byte) (snapshot.Facts, error) {
		*calls++
		tree, parseErr := adapter.Parse(ctx, parser.SourceFile{Content: content})
		if tree == nil {
			return snapshot.Facts{}, parseErr
		}
		defer tree.Close()
		symbols, err := adapter.Symbols(tree)
		if err != nil {
			return snapshot.Facts{}, err
		}
		references, err := adapter.References(tree)
		if err != nil {
			return snapshot.Facts{}, err
		}
		return snapshot.Facts{Symbols: symbols, References: references, HasSyntaxErrors: tree.HasErrors()}, parseErr
	}
}

func TestRealLinkedWorktreesShareOneParseSnapshot(t *testing.T) {
	main, linked := realWorktreePaths(t)
	adapter := pythonAdapter(t)
	source := []byte("def alpha():\n    return helper()\n")
	calls := 0
	compute := parseFacts(adapter, &calls)
	want, err := snapshot.New(main).GetOrCompute(t.Context(), adapter.Info(), source, compute)
	if err != nil {
		t.Fatal(err)
	}
	got, err := snapshot.New(linked).GetOrCompute(t.Context(), adapter.Info(), source, compute)
	if err != nil || !reflect.DeepEqual(got, want) || calls != 1 {
		t.Fatalf("linked worktree facts = %+v, err=%v, parses=%d", got, err, calls)
	}
}

func openIndexStore(t *testing.T, paths filesystem.RuntimePaths) (*index.Store, *sql.DB) {
	t.Helper()
	if err := os.MkdirAll(paths.RuntimeRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	db, err := storage.Open(t.Context(), storage.Options{Path: paths.DBPath})
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

func storedFacts(t *testing.T, db *sql.DB, path string) []string {
	t.Helper()
	var result []string
	for _, statement := range []string{
		`SELECT 'S:' || name || ':' || kind FROM symbols WHERE path = ? ORDER BY name`,
		`SELECT 'R:' || target_text || ':' || label FROM symbol_references WHERE path = ? ORDER BY target_text`,
	} {
		rows, err := db.QueryContext(t.Context(), statement, path)
		if err != nil {
			t.Fatal(err)
		}
		for rows.Next() {
			var item string
			if err := rows.Scan(&item); err != nil {
				rows.Close()
				t.Fatal(err)
			}
			result = append(result, item)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			t.Fatal(err)
		}
		rows.Close()
	}
	return result
}

// TASK-04 bridge fixture: the production IndexFile path belongs to TASK-05.
// This proves cache loss/recovery over real parser facts committed by the real
// Store, without claiming that production indexing already uses the cache.
func TestCacheLossKeepsStoreFactsStableThroughBridgeFixture(t *testing.T) {
	paths, _ := realWorktreePaths(t)
	adapter := pythonAdapter(t)
	store, db := openIndexStore(t, paths)
	unit, err := store.UpsertUnit(t.Context(), paths.WorktreeRoot, index.UnitPython)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(unit.Path, "file.py")
	if err := store.UpsertFileState(t.Context(), index.FileIndexState{UnitID: unit.ID, Path: path, Language: "python", State: index.StatePending}); err != nil {
		t.Fatal(err)
	}
	source := []byte("def alpha():\n    return helper()\n")
	sum := sha256.Sum256(source)
	contentHash := hex.EncodeToString(sum[:])
	cache := snapshot.New(paths)
	calls := 0
	compute := parseFacts(adapter, &calls)
	indexOnce := func() []string {
		t.Helper()
		facts, err := cache.GetOrCompute(t.Context(), adapter.Info(), source, compute)
		if err != nil {
			t.Fatal(err)
		}
		replacement := index.FileFacts{ProjectID: testProjectID, UnitID: unit.ID, Path: path, Language: "python", ContentHash: contentHash, State: index.StateIndexed}
		for _, sym := range facts.Symbols {
			replacement.Symbols = append(replacement.Symbols, index.Symbol{
				LogicalKey: path + ":" + sym.Name, Kind: sym.Kind, Name: sym.Name,
				StartLine: int(sym.Range.StartRow), StartCol: int(sym.Range.StartColumn),
				EndLine: int(sym.Range.EndRow), EndCol: int(sym.Range.EndColumn),
				SignatureHash: "fixture-sig", BodyHash: "fixture-body", StructureHash: "fixture-shape",
			})
		}
		for _, ref := range facts.References {
			replacement.References = append(replacement.References, index.Reference{
				TargetText: ref.Name, Label: "STRUCTURAL_NAME_MATCH", Confidence: 0.5,
			})
		}
		if _, err := store.ReplaceFileFacts(t.Context(), replacement); err != nil {
			t.Fatal(err)
		}
		return storedFacts(t, db, path)
	}
	warm := indexOnce()
	if got := indexOnce(); !reflect.DeepEqual(got, warm) || calls != 1 {
		t.Fatalf("warm facts=%v, want=%v, parses=%d", got, warm, calls)
	}
	if err := os.RemoveAll(paths.CacheDir); err != nil {
		t.Fatal(err)
	}
	if got := indexOnce(); !reflect.DeepEqual(got, warm) || calls != 2 {
		t.Fatalf("deleted-cache facts=%v, want=%v, parses=%d", got, warm, calls)
	}
	entries, err := os.ReadDir(paths.CacheDir)
	if err != nil || len(entries) != 1 {
		t.Fatalf("cache entries=%v, err=%v", entries, err)
	}
	if err := os.WriteFile(filepath.Join(paths.CacheDir, entries[0].Name()), []byte("corrupt"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := indexOnce(); !reflect.DeepEqual(got, warm) || calls != 3 {
		t.Fatalf("corrupt-cache facts=%v, want=%v, parses=%d", got, warm, calls)
	}
	if len(warm) != 2 {
		t.Fatalf("bridge fixture stored %v, want one symbol and one reference", warm)
	}
}
