package impact_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/impact"
	"github.com/PsyChaos/mindrail/internal/index"
	"github.com/PsyChaos/mindrail/internal/migration"
	"github.com/PsyChaos/mindrail/internal/storage"
	"github.com/PsyChaos/mindrail/migrations"
)

type impactFixture struct {
	service *impact.Service
	indexes *index.Store
	db      *sql.DB
	unit    index.ProjectUnit
}

func newImpactFixture(t testing.TB) impactFixture {
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
	clock := app.FixedClock{Instant: time.Date(2026, 9, 23, 10, 0, 0, 0, time.UTC)}
	if _, err := migration.New(db.DB, set, clock).Up(t.Context()); err != nil {
		t.Fatal(err)
	}
	indexes := index.NewStore(db.DB, clock)
	unit, err := indexes.UpsertUnit(t.Context(), t.TempDir(), index.UnitPython)
	if err != nil {
		t.Fatal(err)
	}
	service, err := impact.New(indexes)
	if err != nil {
		t.Fatal(err)
	}
	return impactFixture{service: service, indexes: indexes, db: db.DB, unit: unit}
}

func seedSymbol(t testing.TB, fx impactFixture, uid, key, name, path string) int64 {
	t.Helper()
	return seedSymbolIn(t, fx, fx.unit, uid, key, name, path)
}

func seedSymbolIn(t testing.TB, fx impactFixture, unit index.ProjectUnit, uid, key, name, path string) int64 {
	t.Helper()
	exec := func(statement string, args ...any) {
		t.Helper()
		if _, err := fx.db.ExecContext(t.Context(), statement, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`INSERT INTO symbol_identities
		(symbol_uid, project_id, unit_id, language, logical_key, previous_keys, created_at)
		VALUES (?, 'PRJ', ?, 'python', ?, '[]', '2026-09-23T10:00:00Z')`, uid, unit.ID, key)
	exec(`INSERT INTO symbols
		(unit_id, path, logical_key, kind, name, start_line, start_col, end_line, end_col,
		signature_hash, body_hash, structure_hash, symbol_uid)
		VALUES (?, ?, ?, 'function', ?, 1, 0, 2, 0, 's', 'b', 't', ?)`,
		unit.ID, path, key, name, uid)
	var id int64
	if err := fx.db.QueryRowContext(t.Context(),
		`SELECT id FROM symbols WHERE symbol_uid = ?`, uid).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func seedReference(t testing.TB, fx impactFixture, referrerKey, targetText string, targetID int64) {
	t.Helper()
	if _, err := fx.db.ExecContext(t.Context(), `INSERT INTO symbol_references
		(unit_id, path, referrer_key, target_text, label, confidence, resolved_symbol_id)
		VALUES (?, '/r/a.py', ?, ?, 'STRUCTURAL_NAME_MATCH', 0.5, ?)`,
		fx.unit.ID, referrerKey, targetText, targetID); err != nil {
		t.Fatal(err)
	}
}

// seedChain builds C → B → A plus B → D: C refers to B, B refers to A and
// to D, all same file. D shares A's referrer, which pins per-edge dedupe.
func seedChain(t testing.TB, fx impactFixture) {
	t.Helper()
	a := seedSymbol(t, fx, "SYM-I-A", "a", "fa", "/r/a.py")
	b := seedSymbol(t, fx, "SYM-I-B", "b", "fb", "/r/a.py")
	seedSymbol(t, fx, "SYM-I-C", "c", "fc", "/r/a.py")
	d := seedSymbol(t, fx, "SYM-I-D", "d", "fd", "/r/a.py")
	seedReference(t, fx, "b", "fa", a)
	seedReference(t, fx, "c", "fb", b)
	seedReference(t, fx, "b", "fd", d)
}

// TestAnalyzeFollowsResolvedEdges is TASK-01 AC-01.2. Reverse traversal from
// a changed symbol follows resolved edges to direct referrers, and every
// entry carries its edge, confidence, depth and empty fallback reason
// (AC-01.1).
func TestAnalyzeFollowsResolvedEdges(t *testing.T) {
	fx := newImpactFixture(t)
	seedChain(t, fx)

	result, err := fx.service.Analyze(t.Context(), impact.Request{
		Symbols:    []impact.Input{{UID: "SYM-I-A"}},
		DirectOnly: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Entries) != 1 {
		t.Fatalf("entries = %+v", result.Entries)
	}
	entry := result.Entries[0]
	if entry.TargetUID != "SYM-I-A" || entry.TargetKey != "a" || entry.TargetName != "fa" ||
		entry.TargetPath != "/r/a.py" {
		t.Fatalf("target = %+v", entry)
	}
	if entry.Via.Kind != impact.DirectEdge || entry.Via.ReferrerKey != "b" ||
		entry.Via.ReferrerPath != "/r/a.py" || entry.Via.TargetText != "fa" {
		t.Fatalf("via = %+v", entry.Via)
	}
	if entry.Confidence != 0.5 {
		t.Fatalf("confidence = %v, want the row value", entry.Confidence)
	}
	if entry.Depth != 1 {
		t.Fatalf("depth = %d", entry.Depth)
	}
	if entry.FallbackReason != "" {
		t.Fatalf("fallback reason = %q, want empty on direct", entry.FallbackReason)
	}
}

// TestAnalyzeDepthDefaultsToOne is TASK-01 AC-01.3. A zero-value request
// analyzes depth 1; an explicit depth 2 reaches referrers of referrers,
// each entry carrying its own depth.
func TestAnalyzeDepthDefaultsToOne(t *testing.T) {
	fx := newImpactFixture(t)
	seedChain(t, fx)

	shallow, err := fx.service.Analyze(t.Context(), impact.Request{
		Symbols:    []impact.Input{{UID: "SYM-I-A"}},
		DirectOnly: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(shallow.Entries) != 1 || shallow.Entries[0].Depth != 1 {
		t.Fatalf("shallow = %+v", shallow.Entries)
	}
	deep, err := fx.service.Analyze(t.Context(), impact.Request{
		Symbols:    []impact.Input{{UID: "SYM-I-A"}},
		Depth:      2,
		DirectOnly: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(deep.Entries) != 2 {
		t.Fatalf("deep = %+v", deep.Entries)
	}
	depths := map[string]int{}
	for _, entry := range deep.Entries {
		depths[entry.Via.ReferrerKey] = entry.Depth
	}
	if depths["b"] != 1 || depths["c"] != 2 {
		t.Fatalf("depths = %+v", depths)
	}
}

// TestAnalyzeKeepsSharedReferrersDistinct pins per-edge dedupe: one
// referrer reaching two analyzed symbols yields two entries, not one.
func TestAnalyzeKeepsSharedReferrersDistinct(t *testing.T) {
	fx := newImpactFixture(t)
	seedChain(t, fx)

	result, err := fx.service.Analyze(t.Context(), impact.Request{
		Symbols:    []impact.Input{{UID: "SYM-I-A"}, {UID: "SYM-I-D"}},
		DirectOnly: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Entries) != 2 {
		t.Fatalf("entries = %+v", result.Entries)
	}
	targets := map[string]bool{}
	for _, entry := range result.Entries {
		targets[entry.TargetUID] = true
	}
	if !targets["SYM-I-A"] || !targets["SYM-I-D"] {
		t.Fatalf("targets = %+v", targets)
	}
}

// TestAnalyzeBreadthCapAndOverride is TASK-01 AC-01.4. Structural evidence
// alone stays TARGETED here and never exceeds MODULE; an explicit caller
// justification records a wider breadth verbatim without moving the
// structural one.
func TestAnalyzeBreadthCapAndOverride(t *testing.T) {
	fx := newImpactFixture(t)
	seedChain(t, fx)

	plain, err := fx.service.Analyze(t.Context(), impact.Request{
		Symbols:    []impact.Input{{UID: "SYM-I-A"}},
		Depth:      2,
		DirectOnly: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if plain.StructuralBreadth != impact.TargetedBreadth || plain.Breadth != impact.TargetedBreadth {
		t.Fatalf("breadth = %q / %q", plain.StructuralBreadth, plain.Breadth)
	}
	lifted, err := fx.service.Analyze(context.Background(), impact.Request{
		Symbols:         []impact.Input{{Key: "a"}},
		DirectOnly:      true,
		BreadthOverride: impact.PackageBreadth,
		Justification:   "public API rule: exported symbol",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(lifted.Entries) != 1 {
		t.Fatalf("key input resolved nowhere: %+v", lifted.Entries)
	}
	if lifted.StructuralBreadth != impact.TargetedBreadth {
		t.Fatalf("structural = %q", lifted.StructuralBreadth)
	}
	if lifted.Breadth != impact.PackageBreadth ||
		lifted.Justification != "public API rule: exported symbol" {
		t.Fatalf("override = %q / %q", lifted.Breadth, lifted.Justification)
	}
	bare, err := fx.service.Analyze(t.Context(), impact.Request{
		Symbols:         []impact.Input{{UID: "SYM-I-A"}},
		DirectOnly:      true,
		BreadthOverride: impact.PackageBreadth,
	})
	if err != nil {
		t.Fatal(err)
	}
	if bare.Breadth != impact.TargetedBreadth {
		t.Fatalf("unjustified override lifted to %q", bare.Breadth)
	}
}

// TestAnalyzeDropsUnknownInputs pins the no-invention rule: inputs that
// resolve nowhere contribute no entries and no error.
func TestAnalyzeDropsUnknownInputs(t *testing.T) {
	fx := newImpactFixture(t)
	seedChain(t, fx)

	result, err := fx.service.Analyze(t.Context(), impact.Request{
		Symbols: []impact.Input{{UID: "SYM-NOPE"}, {Key: "nope"}, {}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Entries) != 0 {
		t.Fatalf("entries = %+v", result.Entries)
	}
}
