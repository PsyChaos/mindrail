package inventory_test

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/index"
	"github.com/PsyChaos/mindrail/internal/index/inventory"
	"github.com/PsyChaos/mindrail/internal/migration"
	"github.com/PsyChaos/mindrail/internal/storage"
	"github.com/PsyChaos/mindrail/migrations"
)

func storeForInventory(t *testing.T) (*index.Store, *sql.DB) {
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

func marker(t *testing.T, root, relative string) string {
	t.Helper()
	path := filepath.Join(root, relative)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestDiscoverFindsDeterministicPythonTypeScriptAndJavaScriptUnits(t *testing.T) {
	root := t.TempDir()
	marker(t, root, "python/pyproject.toml")
	marker(t, root, "typescript/package.json")
	marker(t, root, "typescript/tsconfig.json")
	marker(t, root, "javascript/package.json")

	store, _ := storeForInventory(t)
	got, err := inventory.Discover(t.Context(), root, store)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}

	want := []struct {
		path string
		kind index.UnitKind
	}{
		{filepath.Join(root, "javascript"), index.UnitJavaScript},
		{filepath.Join(root, "python"), index.UnitPython},
		{filepath.Join(root, "typescript"), index.UnitTypeScript},
	}
	if len(got.Units) != len(want) {
		t.Fatalf("units = %+v, want %d", got.Units, len(want))
	}
	for i, unit := range got.Units {
		if unit.ID == "" || unit.Path != want[i].path || unit.Kind != want[i].kind {
			t.Errorf("unit %d = %+v, want path=%q kind=%q", i, unit, want[i].path, want[i].kind)
		}
	}

	persisted, err := store.ListUnits(t.Context(), root)
	if err != nil {
		t.Fatalf("ListUnits: %v", err)
	}
	if !reflect.DeepEqual(persisted, got.Units) {
		t.Errorf("persisted units = %+v, want %+v", persisted, got.Units)
	}
}

func TestDiscoverReturnsCanceledContextBeforeAnEmptyWalk(t *testing.T) {
	store, _ := storeForInventory(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if _, err := inventory.Discover(ctx, t.TempDir(), store); !errors.Is(err, context.Canceled) {
		t.Fatalf("Discover error = %v, want context.Canceled", err)
	}
}

func TestDiscoverPrioritizesCanceledContextOverAConfigurationError(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if _, err := inventory.Discover(ctx, t.TempDir(), nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("Discover error = %v, want context.Canceled before a nil-store error", err)
	}
}

func TestDiscoverIsIdempotentAndDoesNotTouchTheRepository(t *testing.T) {
	root := t.TempDir()
	markerPath := marker(t, root, "pkg/pyproject.toml")
	before, err := os.Stat(markerPath)
	if err != nil {
		t.Fatal(err)
	}

	store, db := storeForInventory(t)
	first, err := inventory.Discover(t.Context(), root, store)
	if err != nil {
		t.Fatalf("first Discover: %v", err)
	}
	second, err := inventory.Discover(t.Context(), root, store)
	if err != nil {
		t.Fatalf("second Discover: %v", err)
	}
	after, err := os.Stat(markerPath)
	if err != nil {
		t.Fatal(err)
	}
	if !before.ModTime().Equal(after.ModTime()) {
		t.Errorf("discovery changed marker modtime: before=%s after=%s", before.ModTime(), after.ModTime())
	}
	if !reflect.DeepEqual(first.Units, second.Units) {
		t.Errorf("rediscovery = %+v, want %+v", second.Units, first.Units)
	}
	var units, files int
	if err := db.QueryRowContext(context.Background(), "SELECT count(*) FROM project_units").Scan(&units); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(context.Background(), "SELECT count(*) FROM file_index_state").Scan(&files); err != nil {
		t.Fatal(err)
	}
	if units != 1 || files != 0 {
		t.Errorf("after repeated discovery: project_units=%d file_index_state=%d, want 1 and 0", units, files)
	}
}

func TestDiscoverDoesNotFollowSymlinksOutsideCanonicalRoot(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	marker(t, root, "inside/package.json")
	marker(t, outside, "escaped/pyproject.toml")
	if err := os.Symlink(filepath.Join(outside, "escaped"), filepath.Join(root, "linked-outside")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}

	store, _ := storeForInventory(t)
	got, err := inventory.Discover(t.Context(), root, store)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(got.Units) != 1 || got.Units[0].Path != filepath.Join(root, "inside") {
		t.Fatalf("units = %+v, want only the canonical in-root package", got.Units)
	}
}

func TestDiscoverDoesNotTreatASymlinkedMarkerFileAsInventory(t *testing.T) {
	root := t.TempDir()
	target := marker(t, t.TempDir(), "pyproject.toml")
	if err := os.Mkdir(filepath.Join(root, "linked"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(root, "linked", "pyproject.toml")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}

	store, _ := storeForInventory(t)
	got, err := inventory.Discover(t.Context(), root, store)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(got.Units) != 0 {
		t.Fatalf("symlinked marker produced units: %+v", got.Units)
	}
}

func TestDiscoverReconcilesRemovedRootWithoutTouchingSiblingRoot(t *testing.T) {
	parent := t.TempDir()
	firstRoot := filepath.Join(parent, "web")
	secondRoot := filepath.Join(parent, "website")
	firstMarker := marker(t, firstRoot, "package.json")
	marker(t, secondRoot, "pyproject.toml")
	store, db := storeForInventory(t)

	first, err := inventory.Discover(t.Context(), firstRoot, store)
	if err != nil {
		t.Fatalf("first discovery: %v", err)
	}
	if _, err := inventory.Discover(t.Context(), secondRoot, store); err != nil {
		t.Fatalf("second discovery: %v", err)
	}
	if err := store.UpsertFileState(t.Context(), index.FileIndexState{
		UnitID:   first.Units[0].ID,
		Path:     filepath.Join(firstRoot, "index.js"),
		Language: "javascript",
		State:    index.StatePending,
	}); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(firstMarker); err != nil {
		t.Fatal(err)
	}
	if _, err := inventory.Discover(t.Context(), firstRoot, store); err != nil {
		t.Fatalf("rediscovery after marker removal: %v", err)
	}

	firstUnits, err := store.ListUnits(t.Context(), firstRoot)
	if err != nil {
		t.Fatal(err)
	}
	secondUnits, err := store.ListUnits(t.Context(), secondRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(firstUnits) != 0 || len(secondUnits) != 1 || secondUnits[0].Path != secondRoot {
		t.Fatalf("reconciled units first=%+v second=%+v", firstUnits, secondUnits)
	}
	var files int
	if err := db.QueryRowContext(t.Context(), `SELECT count(*) FROM file_index_state WHERE path = ?`, filepath.Join(firstRoot, "index.js")).Scan(&files); err != nil {
		t.Fatal(err)
	}
	if files != 0 {
		t.Fatalf("stale root left %d file state rows", files)
	}
}

func TestDiscoverReconcileRollsBackWhenDeletingAStaleUnitFails(t *testing.T) {
	root := t.TempDir()
	markerPath := marker(t, root, "pkg/package.json")
	store, db := storeForInventory(t)
	first, err := inventory.Discover(t.Context(), root, store)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertFileState(t.Context(), index.FileIndexState{
		UnitID:   first.Units[0].ID,
		Path:     filepath.Join(root, "pkg", "index.js"),
		Language: "javascript",
		State:    index.StatePending,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(t.Context(), `CREATE TRIGGER fail_stale_unit_delete BEFORE DELETE ON project_units BEGIN SELECT RAISE(ABORT, 'injected stale-unit delete failure'); END`); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(markerPath); err != nil {
		t.Fatal(err)
	}
	if _, err := inventory.Discover(t.Context(), root, store); err == nil {
		t.Fatal("reconciliation succeeded despite injected delete failure")
	}
	units, err := store.ListUnits(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	if len(units) != 1 || units[0].ID != first.Units[0].ID {
		t.Fatalf("failed prune committed unit removal: %+v", units)
	}
	var files int
	if err := db.QueryRowContext(t.Context(), `SELECT count(*) FROM file_index_state`).Scan(&files); err != nil {
		t.Fatal(err)
	}
	if files != 1 {
		t.Fatalf("failed prune committed dependent deletion: files=%d", files)
	}
}

func TestStoreScopesAndReconcilesAtFilesystemRoot(t *testing.T) {
	store, _ := storeForInventory(t)
	unitPath := t.TempDir()
	if _, err := store.UpsertUnit(t.Context(), unitPath, index.UnitPython); err != nil {
		t.Fatal(err)
	}

	units, err := store.ListUnits(t.Context(), string(filepath.Separator))
	if err != nil {
		t.Fatal(err)
	}
	if len(units) != 1 || units[0].Path != unitPath {
		t.Fatalf("filesystem-root units = %+v, want %q", units, unitPath)
	}
	if err := store.ReconcileUnits(t.Context(), string(filepath.Separator), nil); err != nil {
		t.Fatal(err)
	}
	units, err = store.ListUnits(t.Context(), unitPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(units) != 0 {
		t.Fatalf("filesystem-root reconciliation left stale units: %+v", units)
	}
}

func TestOwnerUsesTheLongestContainingUnitRoot(t *testing.T) {
	root := t.TempDir()
	units := []index.ProjectUnit{
		{ID: "UNT-parent", Path: filepath.Join(root, "web"), Kind: index.UnitJavaScript},
		{ID: "UNT-child", Path: filepath.Join(root, "web", "app"), Kind: index.UnitTypeScript},
	}

	owner, ok := inventory.Owner(filepath.Join(root, "web", "app", "component.ts"), units)
	if !ok || owner.ID != "UNT-child" {
		t.Fatalf("nested owner = %+v, ok=%t, want UNT-child", owner, ok)
	}
	if _, ok := inventory.Owner(filepath.Join(root, "website", "main.js"), units); ok {
		t.Fatal("path sharing a string prefix with a unit was admitted")
	}
}
