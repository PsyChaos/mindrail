package impact_test

// Deferral proofs (MR-019 TASK-02): budget exhaustion returns partial
// entries plus an exact pending frontier — never a block, never a silent
// skip — and the frontier re-queues at high priority through the existing
// scheduler. Zero and item budgets make every assertion deterministic;
// no wall-clock appears.

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/filesystem"
	"github.com/PsyChaos/mindrail/internal/impact"
	"github.com/PsyChaos/mindrail/internal/index"
	"github.com/PsyChaos/mindrail/internal/index/parser"
	"github.com/PsyChaos/mindrail/internal/index/scheduler"
	"github.com/PsyChaos/mindrail/internal/index/snapshot"
	"github.com/PsyChaos/mindrail/internal/migration"
	"github.com/PsyChaos/mindrail/internal/perf"
	"github.com/PsyChaos/mindrail/internal/storage"
	"github.com/PsyChaos/mindrail/migrations"
)

// seedReferenceAt is seedReference with an explicit path: the deferral
// chain lives in a real unit directory, and the shared helper hardcodes
// /r/a.py.
func seedReferenceAt(t *testing.T, fx impactFixture, path, referrerKey, targetText string, targetID int64) {
	t.Helper()
	if _, err := fx.db.ExecContext(t.Context(), `INSERT INTO symbol_references
		(unit_id, path, referrer_key, target_text, label, confidence, resolved_symbol_id)
		VALUES (?, ?, ?, ?, 'STRUCTURAL_NAME_MATCH', 0.5, ?)`,
		fx.unit.ID, path, referrerKey, targetText, targetID); err != nil {
		t.Fatal(err)
	}
}

// TestDeferralZeroBudgetPartial pins the deterministic partial: a spent
// budget processes nothing, names the whole input frontier pending, and
// fails nothing.
func TestDeferralZeroBudgetPartial(t *testing.T) {
	fx := newImpactFixture(t)
	seedChain(t, fx)
	partial, err := fx.service.Analyze(perf.WithBudget(t.Context(), perf.NewBudget(0)), impact.Request{
		Symbols:    []impact.Input{{UID: "SYM-I-A"}},
		Depth:      2,
		DirectOnly: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if partial.Complete {
		t.Fatal("zero-budget analysis reports complete")
	}
	if len(partial.Entries) != 0 {
		t.Fatalf("entries = %+v, want none under zero budget", partial.Entries)
	}
	if len(partial.Pending) != 1 || partial.Pending[0] != "SYM-I-A" {
		t.Fatalf("pending = %v, want exactly the input frontier", partial.Pending)
	}
}

// TestDeferralItemBudgetFrontier pins the exact frontier: one work item
// expands A (one entry), and B — the unexpanded remainder — names
// itself.
func TestDeferralItemBudgetFrontier(t *testing.T) {
	fx := newImpactFixture(t)
	seedChain(t, fx)
	partial, err := fx.service.Analyze(perf.WithBudget(t.Context(), perf.NewItemBudget(1)), impact.Request{
		Symbols:    []impact.Input{{UID: "SYM-I-A"}},
		Depth:      2,
		DirectOnly: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if partial.Complete {
		t.Fatal("exhausted analysis reports complete")
	}
	if len(partial.Entries) != 1 || partial.Entries[0].Via.ReferrerKey != "b" {
		t.Fatalf("entries = %+v, want exactly the B edge", partial.Entries)
	}
	if len(partial.Pending) != 1 || partial.Pending[0] != "SYM-I-B" {
		t.Fatalf("pending = %v, want exactly SYM-I-B", partial.Pending)
	}
}

// TestDeferralGenerousBudgetCompletes pins the other end: without
// exhaustion the result is complete with an empty frontier.
func TestDeferralGenerousBudgetCompletes(t *testing.T) {
	fx := newImpactFixture(t)
	seedChain(t, fx)
	full, err := fx.service.Analyze(perf.WithBudget(t.Context(), perf.NewBudget(time.Hour)), impact.Request{
		Symbols:    []impact.Input{{UID: "SYM-I-A"}},
		Depth:      2,
		DirectOnly: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !full.Complete || len(full.Pending) != 0 {
		t.Fatalf("complete = %v, pending = %v, want complete with empty frontier", full.Complete, full.Pending)
	}
	if len(full.Entries) != 2 {
		t.Fatalf("entries = %+v, want both depths", full.Entries)
	}
}

// TestDeferralFrontierRequeuesPrioritized pins the reschedule half
// (AC-02.2): the pending frontier resolves to its unit, that unit's
// files move to the queue front through Scheduler.Prioritize, and a
// generous re-run completes over the full entries.
func TestDeferralFrontierRequeuesPrioritized(t *testing.T) {
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
	indexes := index.NewStore(db.DB, clock)
	unit, err := indexes.UpsertUnit(t.Context(), filepath.Join(root, "chain"), index.UnitPython)
	if err != nil {
		t.Fatal(err)
	}
	registry, err := parser.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(registry.Close)
	sched, err := scheduler.New(indexes, index.NewIndexer(indexes, registry,
		snapshot.New(filesystem.RuntimePaths{CacheDir: filepath.Join(root, "cache")})), 0)
	if err != nil {
		t.Fatal(err)
	}
	service, err := impact.New(indexes)
	if err != nil {
		t.Fatal(err)
	}
	fx := impactFixture{service: service, indexes: indexes, db: db.DB, unit: unit}
	// The chain lives in a real unit path: pending file states must sit
	// inside their unit, or Prioritize rightly refuses them. References
	// are seeded with the same path (seedReference hardcodes /r/a.py,
	// which would orphan the referrer lookup here).
	chainPath := filepath.Join(unit.Path, "a.py")
	a := seedSymbol(t, fx, "SYM-I-A", "a", "fa", chainPath)
	b := seedSymbol(t, fx, "SYM-I-B", "b", "fb", chainPath)
	seedSymbol(t, fx, "SYM-I-C", "c", "fc", chainPath)
	d := seedSymbol(t, fx, "SYM-I-D", "d", "fd", chainPath)
	seedReferenceAt(t, fx, chainPath, "b", "fa", a)
	seedReferenceAt(t, fx, chainPath, "c", "fb", b)
	seedReferenceAt(t, fx, chainPath, "b", "fd", d)
	if err := indexes.UpsertFileState(t.Context(), index.FileIndexState{
		UnitID: unit.ID, Path: chainPath, Language: "python", State: index.StatePending,
	}); err != nil {
		t.Fatal(err)
	}

	partial, err := service.Analyze(perf.WithBudget(t.Context(), perf.NewItemBudget(1)), impact.Request{
		Symbols:    []impact.Input{{UID: "SYM-I-A"}},
		Depth:      2,
		DirectOnly: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if partial.Complete || len(partial.Pending) != 1 {
		t.Fatalf("complete = %v, pending = %v, want one deferred uid", partial.Complete, partial.Pending)
	}
	// The caller shape: frontier uids resolve to units, units re-queue.
	seenUnits := map[string]bool{}
	for _, uid := range partial.Pending {
		unitID, _, found, err := indexes.UnitForUID(t.Context(), uid)
		if err != nil {
			t.Fatal(err)
		}
		if !found {
			t.Fatalf("pending %s resolves to no unit", uid)
		}
		seenUnits[unitID] = true
	}
	if len(seenUnits) != 1 || !seenUnits[unit.ID] {
		t.Fatalf("units = %v, want exactly the chain unit", seenUnits)
	}
	moved, err := sched.Prioritize(t.Context(), "PRJ-DEFER", []index.ProjectUnit{unit}, unit.ID)
	if err != nil {
		t.Fatal(err)
	}
	if moved != 1 {
		t.Fatalf("prioritize moved = %d, want the pending file", moved)
	}
	if got := sched.Paths(); len(got) != 1 || got[0] != chainPath {
		t.Fatalf("queue = %v, want the deferred unit's file first", got)
	}
	full, err := service.Analyze(t.Context(), impact.Request{
		Symbols:    []impact.Input{{UID: "SYM-I-A"}},
		Depth:      2,
		DirectOnly: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !full.Complete || len(full.Entries) != 2 {
		t.Fatalf("complete = %v, entries = %+v, want the full closure", full.Complete, full.Entries)
	}
}

// TestDeferralBranchingFrontier pins the exact remainder shape the
// single-chain tests cannot see: with inputs [B, D] and one work item,
// level 1 expands B (queuing C for next level) and exhausts on D, so the
// pending frontier must name BOTH the unexpanded input AND the already
// queued next-level uid, in that order. Dropping `next` silently skips
// queued work; per-level yield expands too much — both mutants die here.
func TestDeferralBranchingFrontier(t *testing.T) {
	fx := newImpactFixture(t)
	seedChain(t, fx)
	partial, err := fx.service.Analyze(perf.WithBudget(t.Context(), perf.NewItemBudget(1)), impact.Request{
		Symbols:    []impact.Input{{UID: "SYM-I-B"}, {UID: "SYM-I-D"}},
		Depth:      2,
		DirectOnly: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if partial.Complete {
		t.Fatal("exhausted analysis reports complete")
	}
	if len(partial.Entries) != 1 || partial.Entries[0].Via.ReferrerKey != "c" {
		t.Fatalf("entries = %+v, want exactly the C edge", partial.Entries)
	}
	want := []string{"SYM-I-D", "SYM-I-C"}
	if len(partial.Pending) != len(want) {
		t.Fatalf("pending = %v, want %v", partial.Pending, want)
	}
	for i := range want {
		if partial.Pending[i] != want[i] {
			t.Fatalf("pending = %v, want %v", partial.Pending, want)
		}
	}
}
