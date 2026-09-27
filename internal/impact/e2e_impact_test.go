package impact_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PsyChaos/mindrail/internal/filesystem"
	"github.com/PsyChaos/mindrail/internal/impact"
	"github.com/PsyChaos/mindrail/internal/index"
	"github.com/PsyChaos/mindrail/internal/index/parser"
	"github.com/PsyChaos/mindrail/internal/index/snapshot"
)

// TestAnalyzeRealRepositoryEndToEnd is TASK-03 AC-03.1. One real indexed
// repository: a changed symbol with a same-file caller, a cross-file name
// user and a bound invariant yields direct + name-match + file + invariant
// entries in one default analysis — every entry self-explaining, breadth at
// most MODULE, depth 1.
func TestAnalyzeRealRepositoryEndToEnd(t *testing.T) {
	fx := newImpactFixture(t)
	registry, err := parser.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(registry.Close)
	root := t.TempDir()
	pkg := filepath.Join(root, "pkg")
	if err := os.MkdirAll(pkg, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pkg, "pyproject.toml"), []byte("[project]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	unit, err := fx.indexes.UpsertUnit(t.Context(), pkg, index.UnitPython)
	if err != nil {
		t.Fatal(err)
	}
	indexer := index.NewIndexer(fx.indexes, registry, snapshot.New(filesystem.RuntimePaths{CacheDir: filepath.Join(root, "cache")}))
	shared := filepath.Join(pkg, "a.py")
	if err := os.WriteFile(shared, []byte("def helper():\n    return 1\n\ndef handler():\n    return helper()\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(pkg, "c.py")
	if err := os.WriteFile(other, []byte("def worker():\n    return helper()\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := indexer.IndexFile(t.Context(), "PRJ-E2E", unit, shared); err != nil {
		t.Fatal(err)
	}
	if _, err := indexer.IndexFile(t.Context(), "PRJ-E2E", unit, other); err != nil {
		t.Fatal(err)
	}
	helpers, err := fx.indexes.KeyToUIDs(t.Context(), helperKey(t, fx, shared))
	if err != nil || len(helpers) != 1 {
		t.Fatalf("helper uids = %+v, %v", helpers, err)
	}
	helper := helpers[0]
	if _, err := fx.db.ExecContext(t.Context(), `INSERT INTO invariant_symbol_bindings
		(invariant_id, symbol_uid, status, updated_at)
		VALUES ('INV-E2E', ?, 'bound', '2026-09-23T10:00:00Z')`, helper); err != nil {
		t.Fatal(err)
	}

	result, err := fx.service.Analyze(t.Context(), impact.Request{
		Symbols: []impact.Input{{UID: helper}},
	})
	if err != nil {
		t.Fatal(err)
	}
	kinds := map[string]int{}
	for _, entry := range result.Entries {
		kinds[string(entry.Via.Kind)]++
		if entry.Via.Kind == "" || entry.Depth != 1 {
			t.Fatalf("entry unexplained: %+v", entry)
		}
		if entry.Confidence <= 0 || entry.Confidence > 1 {
			t.Fatalf("confidence out of range: %+v", entry)
		}
		if entry.Via.Kind == impact.DirectEdge && entry.FallbackReason != "" {
			t.Fatalf("direct entry carries a reason: %+v", entry)
		}
		if entry.Via.Kind != impact.DirectEdge && entry.FallbackReason == "" {
			t.Fatalf("fallback entry names no reason: %+v", entry)
		}
		if !strings.Contains(strings.Join(entry.Invariants, ","), "INV-E2E") {
			t.Fatalf("entry misses the bound invariant: %+v", entry)
		}
	}
	if kinds[impact.DirectEdge] != 1 {
		t.Fatalf("kinds = %+v, want one direct caller", kinds)
	}
	if kinds[impact.FileEdge] != 1 {
		t.Fatalf("kinds = %+v, want the file floor", kinds)
	}
	if kinds[impact.NameMatchEdge] != 1 {
		t.Fatalf("kinds = %+v, want one cross-file name user (got %d ambiguous)", kinds, kinds[impact.AmbiguousEdge])
	}
	if result.StructuralBreadth != impact.ModuleBreadth || result.Breadth != impact.ModuleBreadth {
		t.Fatalf("breadth = %q / %q, want MODULE", result.StructuralBreadth, result.Breadth)
	}
}

func helperKey(t *testing.T, fx impactFixture, path string) string {
	t.Helper()
	var key string
	if err := fx.db.QueryRowContext(t.Context(),
		`SELECT logical_key FROM symbols WHERE path = ? AND name = 'helper'`, path).Scan(&key); err != nil {
		t.Fatal(err)
	}
	return key
}
