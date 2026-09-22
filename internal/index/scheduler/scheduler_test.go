package scheduler_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/filesystem"
	"github.com/PsyChaos/mindrail/internal/index"
	"github.com/PsyChaos/mindrail/internal/index/inventory"
	"github.com/PsyChaos/mindrail/internal/index/parser"
	"github.com/PsyChaos/mindrail/internal/index/scheduler"
	"github.com/PsyChaos/mindrail/internal/index/snapshot"
	"github.com/PsyChaos/mindrail/internal/migration"
	"github.com/PsyChaos/mindrail/internal/storage"
	"github.com/PsyChaos/mindrail/migrations"
)

type fixture struct {
	store    *index.Store
	registry *parser.Registry
	sched    *scheduler.Scheduler
	root     string
}

func newFixture(t *testing.T, maxJobs int) fixture {
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
	clock := app.FixedClock{Instant: time.Date(2026, 9, 22, 10, 0, 0, 0, time.UTC)}
	if _, err := migration.New(db.DB, set, clock).Up(t.Context()); err != nil {
		t.Fatal(err)
	}
	store := index.NewStore(db.DB, clock)
	registry, err := parser.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(registry.Close)
	cache := snapshot.New(filesystem.RuntimePaths{CacheDir: filepath.Join(t.TempDir(), "cache")})
	sched, err := scheduler.New(store, index.NewIndexer(store, registry, cache), maxJobs)
	if err != nil {
		t.Fatal(err)
	}
	return fixture{store: store, registry: registry, sched: sched, root: t.TempDir()}
}

func mkunit(t *testing.T, f fixture, name string, kind index.UnitKind) index.ProjectUnit {
	t.Helper()
	dir := filepath.Join(f.root, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	unit, err := f.store.UpsertUnit(t.Context(), dir, kind)
	if err != nil {
		t.Fatal(err)
	}
	return unit
}

func seedPending(t *testing.T, f fixture, unit index.ProjectUnit, names ...string) []string {
	t.Helper()
	var paths []string
	for _, name := range names {
		path := filepath.Join(unit.Path, name)
		if err := f.store.UpsertFileState(t.Context(), index.FileIndexState{
			UnitID: unit.ID, Path: path, Language: "python", State: index.StatePending,
		}); err != nil {
			t.Fatal(err)
		}
		paths = append(paths, path)
	}
	return paths
}

func writeSource(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestQueueCoalescesDuplicatePaths(t *testing.T) {
	f := newFixture(t, 0)
	unit := mkunit(t, f, "pkg", index.UnitPython)
	path := filepath.Join(unit.Path, "a.py")
	for range 20 {
		if _, err := f.sched.Enqueue(unit, path, scheduler.P4ColdRemainder); err != nil {
			t.Fatal(err)
		}
	}
	if got := f.sched.Len(); got != 1 {
		t.Fatalf("queue holds %d jobs for one path, want 1", got)
	}
	if added, err := f.sched.Enqueue(unit, path, scheduler.P0Targeted); err != nil || added {
		t.Fatalf("P0 re-enqueue added=%t err=%v, want coalesced", added, err)
	}
	if got := f.sched.Len(); got != 1 {
		t.Fatalf("P0 re-enqueue duplicated the path: %d jobs", got)
	}
}

func TestEnqueueP0PromotesQueuedPath(t *testing.T) {
	f := newFixture(t, 0)
	unit := mkunit(t, f, "pkg", index.UnitPython)
	paths := []string{"a.py", "b.py", "c.py"}
	for _, name := range paths {
		if _, err := f.sched.Enqueue(unit, filepath.Join(unit.Path, name), scheduler.P4ColdRemainder); err != nil {
			t.Fatal(err)
		}
	}
	target := filepath.Join(unit.Path, "b.py")
	if added, err := f.sched.Enqueue(unit, target, scheduler.P0Targeted); err != nil || added {
		t.Fatalf("P0 re-enqueue added=%t err=%v, want coalesced promotion", added, err)
	}
	got := f.sched.Paths()
	if len(got) != 3 || got[0] != target {
		t.Fatalf("queue = %v, want the P0 path first without duplication", got)
	}
}

func TestQueueBoundRejectsManualOverflow(t *testing.T) {
	f := newFixture(t, 3)
	unit := mkunit(t, f, "pkg", index.UnitPython)
	for _, name := range []string{"a.py", "b.py", "c.py"} {
		if _, err := f.sched.Enqueue(unit, filepath.Join(unit.Path, name), scheduler.P4ColdRemainder); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := f.sched.Enqueue(unit, filepath.Join(unit.Path, "d.py"), scheduler.P4ColdRemainder); !errors.Is(err, scheduler.ErrQueueFull) {
		t.Fatalf("overflow error = %v, want ErrQueueFull", err)
	}
	if got := f.sched.Len(); got != 3 {
		t.Fatalf("queue holds %d jobs past the bound", got)
	}
}

func TestFillColdOrdersByPathAndReportsMore(t *testing.T) {
	f := newFixture(t, 3)
	xa := mkunit(t, f, "xa", index.UnitPython)
	ab := mkunit(t, f, "ab", index.UnitPython)
	seedPending(t, f, xa, "z1.py", "z2.py")
	seedPending(t, f, ab, "a1.py", "a2.py", "a3.py")
	enqueued, more, err := f.sched.FillCold(t.Context(), []index.ProjectUnit{xa, ab})
	if err != nil || enqueued != 3 || !more {
		t.Fatalf("fill = %d/%t/%v, want 3/true/nil", enqueued, more, err)
	}
	got := f.sched.Paths()
	want := []string{filepath.Join(ab.Path, "a1.py"), filepath.Join(ab.Path, "a2.py"), filepath.Join(ab.Path, "a3.py")}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("queue = %v, want cold path order %v", got, want)
		}
	}
}

func TestPrioritizeMovesUnitAheadOfColdRemainder(t *testing.T) {
	f := newFixture(t, 0)
	a := mkunit(t, f, "a", index.UnitPython)
	b := mkunit(t, f, "b", index.UnitPython)
	seedPending(t, f, a, "z1.py", "z2.py")
	seedPending(t, f, b, "a1.py", "a2.py")
	if _, _, err := f.sched.FillCold(t.Context(), []index.ProjectUnit{a, b}); err != nil {
		t.Fatal(err)
	}
	moved, err := f.sched.Prioritize(t.Context(), []index.ProjectUnit{a, b}, a.ID)
	if err != nil || moved != 2 {
		t.Fatalf("prioritize = %d/%v, want 2/nil", moved, err)
	}
	got := f.sched.Paths()
	want := []string{filepath.Join(a.Path, "z1.py"), filepath.Join(a.Path, "z2.py"), filepath.Join(b.Path, "a1.py"), filepath.Join(b.Path, "a2.py")}
	if len(got) != len(want) {
		t.Fatalf("queue = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("queue = %v, want targeted unit first %v", got, want)
		}
	}
}

func TestPrioritizeEnqueuesUnqueuedUnitFiles(t *testing.T) {
	f := newFixture(t, 0)
	a := mkunit(t, f, "a", index.UnitPython)
	b := mkunit(t, f, "b", index.UnitPython)
	seedPending(t, f, a, "z1.py")
	seedPending(t, f, b, "a1.py")
	moved, err := f.sched.Prioritize(t.Context(), []index.ProjectUnit{a, b}, b.ID)
	if err != nil || moved != 1 {
		t.Fatalf("prioritize = %d/%v, want 1/nil", moved, err)
	}
	got := f.sched.Paths()
	if len(got) != 1 || got[0] != filepath.Join(b.Path, "a1.py") {
		t.Fatalf("queue = %v, want the targeted file alone", got)
	}
}

func TestRunDrainsThroughIndexer(t *testing.T) {
	f := newFixture(t, 0)
	unit := mkunit(t, f, "pkg", index.UnitPython)
	for _, name := range []string{"a.py", "b.py"} {
		path := filepath.Join(unit.Path, name)
		writeSource(t, path, "def f():\n    pass\n")
		if err := f.store.UpsertFileState(t.Context(), index.FileIndexState{
			UnitID: unit.ID, Path: path, Language: "python", State: index.StatePending,
		}); err != nil {
			t.Fatal(err)
		}
	}
	if _, _, err := f.sched.FillCold(t.Context(), []index.ProjectUnit{unit}); err != nil {
		t.Fatal(err)
	}
	completed, err := f.sched.Run(t.Context())
	if err != nil || completed != 2 || f.sched.Len() != 0 {
		t.Fatalf("run = %d/%v/queue %d, want 2/nil/0", completed, err, f.sched.Len())
	}
	for _, name := range []string{"a.py", "b.py"} {
		state, err := f.store.ReadFileState(t.Context(), filepath.Join(unit.Path, name))
		if err != nil || state.State.State != index.StateIndexed {
			t.Fatalf("%s state = %+v, %v", name, state.State, err)
		}
	}
}

func TestRunStopsOnFirstErrorWithoutSkipping(t *testing.T) {
	f := newFixture(t, 0)
	unit := mkunit(t, f, "pkg", index.UnitPython)
	writeSource(t, filepath.Join(unit.Path, "a.py"), "def f():\n    pass\n")
	writeSource(t, filepath.Join(unit.Path, "b.py"), "def broken(:\n    pass\n")
	for _, name := range []string{"a.py", "b.py"} {
		if err := f.store.UpsertFileState(t.Context(), index.FileIndexState{
			UnitID: unit.ID, Path: filepath.Join(unit.Path, name), Language: "python", State: index.StatePending,
		}); err != nil {
			t.Fatal(err)
		}
	}
	if _, _, err := f.sched.FillCold(t.Context(), []index.ProjectUnit{unit}); err != nil {
		t.Fatal(err)
	}
	completed, err := f.sched.Run(t.Context())
	if err == nil || completed != 1 {
		t.Fatalf("run = %d/%v, want 1/failure on the broken file", completed, err)
	}
	failed, err := f.store.ReadFileState(t.Context(), filepath.Join(unit.Path, "b.py"))
	if err != nil || failed.State.State != index.StateFailed {
		t.Fatalf("broken file state = %+v, %v — the failure must stay durable, not skipped", failed.State, err)
	}
	if got := f.sched.Len(); got != 0 {
		t.Fatalf("queue holds %d jobs after the failed run", got)
	}
}

func TestRunHonorsCancelBetweenFiles(t *testing.T) {
	f := newFixture(t, 0)
	unit := mkunit(t, f, "pkg", index.UnitPython)
	// A large valid module keeps the single worker in-flight while the test
	// cancels: the cancellation must take effect at the next between-files
	// checkpoint, aborting the in-flight file back to pending without
	// committing it and without touching the queued remainder.
	writeSource(t, filepath.Join(unit.Path, "a.py"), strings.Repeat("x = 1\n", 50000))
	for _, name := range []string{"b.py", "c.py"} {
		path := filepath.Join(unit.Path, name)
		writeSource(t, path, "def f():\n    pass\n")
	}
	for _, name := range []string{"a.py", "b.py", "c.py"} {
		if err := f.store.UpsertFileState(t.Context(), index.FileIndexState{
			UnitID: unit.ID, Path: filepath.Join(unit.Path, name), Language: "python", State: index.StatePending,
		}); err != nil {
			t.Fatal(err)
		}
	}
	if _, _, err := f.sched.FillCold(t.Context(), []index.ProjectUnit{unit}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	var completed int
	var runErr error
	go func() {
		defer close(done)
		completed, runErr = f.sched.Run(ctx)
	}()
	deadline := time.Now().Add(30 * time.Second)
	for f.sched.Len() == 3 {
		if time.Now().After(deadline) {
			cancel()
			t.Fatal("first file never popped; cannot cancel between files")
		}
	}
	cancel()
	<-done
	if !errors.Is(runErr, context.Canceled) {
		t.Fatalf("run error = %v, want context.Canceled", runErr)
	}
	if completed != 0 {
		t.Fatalf("completed = %d, want the aborted in-flight file uncounted", completed)
	}
	if got := f.sched.Len(); got != 2 {
		t.Fatalf("queue holds %d jobs, want the untouched remainder", got)
	}
	a, err := f.store.ReadFileState(t.Context(), filepath.Join(unit.Path, "a.py"))
	if err != nil {
		t.Fatal(err)
	}
	if a.State.State != index.StatePending {
		t.Fatalf("in-flight file state = %q, want pending for resume", a.State.State)
	}
	// The aborted job left the in-memory window; its durable pending row is
	// the resume path. Refill recovers exactly the uncompleted file; the two
	// untouched files coalesce instead of duplicating.
	refilled, more, err := f.sched.FillCold(t.Context(), []index.ProjectUnit{unit})
	if err != nil || refilled != 1 || more {
		t.Fatalf("refill = %d/%t/%v, want 1/false/nil", refilled, more, err)
	}
	if got := f.sched.Len(); got != 3 {
		t.Fatalf("queue holds %d jobs after refill, want 3 without duplicates", got)
	}
	resumed, err := f.sched.Run(t.Context())
	if err != nil || resumed != 3 || f.sched.Len() != 0 {
		t.Fatalf("resume = %d/%v/queue %d, want 3/nil/0", resumed, err, f.sched.Len())
	}
	for _, name := range []string{"a.py", "b.py", "c.py"} {
		state, err := f.store.ReadFileState(t.Context(), filepath.Join(unit.Path, name))
		if err != nil || state.State.State != index.StateIndexed {
			t.Fatalf("%s state = %+v, %v", name, state.State, err)
		}
	}
}

func TestRunRejectsCancelledContextWithQueueIntact(t *testing.T) {
	f := newFixture(t, 0)
	unit := mkunit(t, f, "pkg", index.UnitPython)
	seedPending(t, f, unit, "a.py")
	if _, _, err := f.sched.FillCold(t.Context(), []index.ProjectUnit{unit}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := f.sched.Run(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("run error = %v, want context.Canceled", err)
	}
	if got := f.sched.Len(); got != 1 {
		t.Fatalf("cancelled run dropped the queue: %d jobs", got)
	}
}

func TestRegisterMarksUnknownLanguageUnsupported(t *testing.T) {
	f := newFixture(t, 0)
	unit := mkunit(t, f, "pkg", index.UnitPython)
	good := filepath.Join(unit.Path, "a.py")
	strange := filepath.Join(unit.Path, "notes.bogus")
	registered, unsupported, err := f.sched.Register(t.Context(), f.registry, []inventory.File{
		{Unit: unit, Path: good},
		{Unit: unit, Path: strange},
	})
	if err != nil || registered != 1 || unsupported != 1 {
		t.Fatalf("register = %d/%d/%v, want 1/1/nil", registered, unsupported, err)
	}
	pending, err := f.store.ListPending(t.Context(), unit.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 || pending[0].Path != good {
		t.Fatalf("resume set = %+v, want only the supported file", pending)
	}
	state, err := f.store.ReadFileState(t.Context(), strange)
	if err != nil || !state.Exists || state.State.State != index.StateUnsupported {
		t.Fatalf("unknown language state = %+v, %v — terminal unsupported, never pending", state.State, err)
	}
}

func TestPriorityLadderNamesSection47(t *testing.T) {
	if scheduler.P0Targeted.String() != "P0-targeted" || scheduler.P4ColdRemainder.String() != "P4-cold-remainder" {
		t.Fatal("priority names drifted from the ladder")
	}
	if !(scheduler.P0Targeted < scheduler.P1DependencyClosure &&
		scheduler.P1DependencyClosure < scheduler.P2ActiveTaskUnit &&
		scheduler.P2ActiveTaskUnit < scheduler.P3ActiveWorkspace &&
		scheduler.P3ActiveWorkspace < scheduler.P4ColdRemainder) {
		t.Fatal("priority order drifted from P0-highest to P4-lowest")
	}
}
