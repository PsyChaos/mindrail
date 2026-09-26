package symbol_test

// TEMPORARY MR-020 demonstration driver — deleted before milestone close
// (D-241). Verdicts come from production service calls
// (Indexer.IndexFile, Service.RefreshBindings) over a scratch repository;
// t.Log carries the verbatim outcomes that findings paste.
//
// Two-agent shape (see findings §D-239 note): sequential agents on ONE
// knowledge lifecycle — agent A protects, agent B renames later. A
// two-checkout staging was attempted and refuted by experiment: a second
// checkout mints an independent uid for byte-identical content
// (SYM-…RR3SPQ vs SYM-…VP65Z3), because uid durability is per lifecycle
// (one database), not across databases. Demonstrating "carry" across
// checkouts would misrepresent the product; the product's two-agent
// model here is MR-003 sequential handoff on one checkout.

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/filesystem"
	"github.com/PsyChaos/mindrail/internal/index"
	"github.com/PsyChaos/mindrail/internal/index/parser"
	"github.com/PsyChaos/mindrail/internal/index/snapshot"
	"github.com/PsyChaos/mindrail/internal/index/symbol"
	"github.com/PsyChaos/mindrail/internal/knowledge/record"
	"github.com/PsyChaos/mindrail/internal/migration"
	"github.com/PsyChaos/mindrail/internal/storage"
	"github.com/PsyChaos/mindrail/migrations"
)

// TestMR020DemoRename is MR-020 AC-02.2: agent A protects Box.run
// (CRITICAL INV-1000); agent B renames it later. A reliable rename
// carries the same symbol_uid; a twinned rename blocks with explicit
// ambiguity (no silent orphan); a deletion blocks as orphaned.
func TestMR020DemoRename(t *testing.T) {
	f := newRefreshFixture(t)
	path := f.indexFile(t, "a.py", "class Box:\n    def run(self):\n        return 1\n")
	inv := activeInvariant("INV-1000", record.SeverityCritical, record.ScopeSymbol, "pkg/a.py:Box.run")

	report, err := f.service.RefreshBindings(t.Context(), refreshProject, f.root, []index.ProjectUnit{f.unit}, []record.Invariant{inv})
	if err != nil {
		t.Fatal(err)
	}
	carried := report.Outcomes[0].UIDs[0]
	t.Logf("agent-A protect: status=%s uid=%s blocking=%v finding=%v",
		report.Outcomes[0].Status, carried, report.Outcomes[0].Blocking, report.Outcomes[0].Finding)

	// Agent B renames run into drive (reliable rename).
	if err := os.WriteFile(path, []byte("class Box:\n    def drive(self):\n        return 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := f.indexer.IndexFile(t.Context(), refreshProject, f.unit, path); err != nil {
		t.Fatal(err)
	}
	report, err = f.service.RefreshBindings(t.Context(), refreshProject, f.root, []index.ProjectUnit{f.unit}, []record.Invariant{inv})
	if err != nil {
		t.Fatal(err)
	}
	one := report.Outcomes[0]
	t.Logf("agent-B reliable rename: status=%s uids=%v blocking=%v finding=%v", one.Status, one.UIDs, one.Blocking, one.Finding)
	if one.Status != index.BindingBound || len(one.UIDs) != 1 || one.UIDs[0] != carried {
		t.Fatalf("identity not carried: %+v, want bound %s", one, carried)
	}

	// Agent B twins the body into alpha and beta (ambiguous rename).
	if err := os.WriteFile(path, []byte("class Box:\n    def alpha(self):\n        return 1\n    def beta(self):\n        return 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := f.indexer.IndexFile(t.Context(), refreshProject, f.unit, path); err != nil {
		t.Fatal(err)
	}
	report, err = f.service.RefreshBindings(t.Context(), refreshProject, f.root, []index.ProjectUnit{f.unit}, []record.Invariant{inv})
	if err != nil {
		t.Fatal(err)
	}
	two := report.Outcomes[0]
	t.Logf("agent-B ambiguous rename: status=%s uids=%v blocking=%v finding=%v", two.Status, two.UIDs, two.Blocking, two.Finding)
	if two.Status != index.BindingAmbiguous || !two.Blocking {
		t.Fatalf("ambiguity did not block: %+v", two)
	}
	if payload, ok := app.PayloadOf(two.Finding); !ok || payload.Code != app.CodeSymbolIdentityAmbiguous {
		t.Fatalf("finding = %v, want SYMBOL_IDENTITY_AMBIGUOUS", two.Finding)
	}
	rows, err := f.store.ListBindingsForInvariant(t.Context(), "INV-1000")
	if err != nil || len(rows) != 1 || rows[0].UID != carried {
		t.Fatalf("stored rows = %+v, %v: invariant silently orphaned", rows, err)
	}
	t.Logf("stored row after ambiguity: uid=%s status=%s (carried, not orphaned)", rows[0].UID, rows[0].Status)

	// Agent B deletes the symbols outright.
	if err := os.WriteFile(path, []byte("x = 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := f.indexer.IndexFile(t.Context(), refreshProject, f.unit, path); err != nil {
		t.Fatal(err)
	}
	report, err = f.service.RefreshBindings(t.Context(), refreshProject, f.root, []index.ProjectUnit{f.unit}, []record.Invariant{inv})
	if err != nil {
		t.Fatal(err)
	}
	three := report.Outcomes[0]
	t.Logf("agent-B deletion: status=%s uids=%v blocking=%v finding=%v", three.Status, three.UIDs, three.Blocking, three.Finding)
	if three.Status != index.BindingOrphaned || !three.Blocking {
		t.Fatalf("deletion did not block: %+v", three)
	}
	if payload, ok := app.PayloadOf(three.Finding); !ok || payload.Code != app.CodeOrphanedProtectedSymbol {
		t.Fatalf("finding = %v, want ORPHANED_PROTECTED_SYMBOL", three.Finding)
	}
}

// copySvc is one agent's service bundle over a working copy.
type copySvc struct {
	db       *storage.DB
	store    *index.Store
	indexer  *index.Indexer
	service  *symbol.Service
	registry *parser.Registry
	unit     index.ProjectUnit
}

func writeCopyFile(t *testing.T, svc copySvc, content string) string {
	t.Helper()
	path := filepath.Join(svc.unit.Path, "a.py")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.indexer.IndexFile(t.Context(), refreshProject, svc.unit, path); err != nil {
		t.Fatal(err)
	}
	return path
}

func refreshCopy(t *testing.T, svc copySvc, root string, inv record.Invariant) (string, []string, bool, error) {
	t.Helper()
	report, err := svc.service.RefreshBindings(t.Context(), refreshProject, root, []index.ProjectUnit{svc.unit}, []record.Invariant{inv})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Outcomes) != 1 {
		t.Fatalf("outcomes = %+v, want exactly one", report.Outcomes)
	}
	o := report.Outcomes[0]
	return o.Status, o.UIDs, o.Blocking, o.Finding
}

// TestMR020DemoRenameTwoWorktrees is the logged refuting experiment behind
// the findings D-239 note: two working copies (wt-a, wt-b) share ONE
// lifecycle database while each agent acts only in its own copy. Agent A
// protects Box.run in wt-a; agent B checks out the identical bytes in wt-b.
func TestMR020DemoRenameTwoWorktrees(t *testing.T) {
	wtA, wtB := t.TempDir(), t.TempDir()
	dbPath := filepath.Join(t.TempDir(), "shared.db")
	openShared := func(root string) (copySvc, func()) {
		db, err := storage.Open(t.Context(), storage.Options{Path: dbPath})
		if err != nil {
			t.Fatal(err)
		}
		set, err := migration.Load(migrations.FS)
		if err != nil {
			t.Fatal(err)
		}
		clock := app.FixedClock{Instant: time.Date(2026, 9, 23, 10, 0, 0, 0, time.UTC)}
		if _, err := migration.New(db.DB, set, clock).Up(t.Context()); err != nil {
			t.Fatal(err)
		}
		store := index.NewStore(db.DB, clock)
		pkg := filepath.Join(root, "pkg")
		if err := os.MkdirAll(pkg, 0o755); err != nil {
			t.Fatal(err)
		}
		unit, err := store.UpsertUnit(t.Context(), pkg, index.UnitPython)
		if err != nil {
			t.Fatal(err)
		}
		registry, err := parser.NewRegistry()
		if err != nil {
			t.Fatal(err)
		}
		indexer := index.NewIndexer(store, registry, snapshot.New(filesystem.RuntimePaths{CacheDir: filepath.Join(root, "cache")}))
		service, err := symbol.New(store)
		if err != nil {
			t.Fatal(err)
		}
		return copySvc{db: db, store: store, indexer: indexer, service: service, registry: registry, unit: unit},
			func() {
				registry.Close()
				_ = db.Close()
			}
	}
	inv := activeInvariant("INV-1000", record.SeverityCritical, record.ScopeSymbol, "pkg/a.py:Box.run")
	const body = "class Box:\n    def run(self):\n        return 1\n"

	svcA, closeA := openShared(wtA)
	writeCopyFile(t, svcA, body)
	statusA, uidsA, _, _ := refreshCopy(t, svcA, wtA, inv)
	closeA()
	if statusA != index.BindingBound || len(uidsA) != 1 {
		t.Fatalf("wt-a protect = %s %v", statusA, uidsA)
	}
	t.Logf("agent-A protect (wt-a): status=%s uid=%s", statusA, uidsA[0])

	svcB, closeB := openShared(wtB)
	defer closeB()
	writeCopyFile(t, svcB, body)
	statusB, uidsB, _, _ := refreshCopy(t, svcB, wtB, inv)
	t.Logf("agent-B checkout rebind (wt-b, identical bytes): status=%s uids=%v", statusB, uidsB)
	if statusB != index.BindingBound || len(uidsB) != 1 {
		t.Fatalf("wt-b rebind = %s %v", statusB, uidsB)
	}
	if uidsB[0] == uidsA[0] {
		t.Fatalf("copies share uid %s; independence refuted", uidsB[0])
	}
	t.Logf("uid independence: wt-a=%s wt-b=%s (per-lifecycle durability, not cross-checkout)", uidsA[0], uidsB[0])

	// The mechanism still works inside the second copy: rename carries wt-b's uid.
	writeCopyFile(t, svcB, "class Box:\n    def drive(self):\n        return 1\n")
	statusR, uidsR, _, _ := refreshCopy(t, svcB, wtB, inv)
	t.Logf("agent-B reliable rename (wt-b): status=%s uids=%v", statusR, uidsR)
	if statusR != index.BindingBound || len(uidsR) != 1 || uidsR[0] != uidsB[0] {
		t.Fatalf("wt-b carry = %s %v, want %s", statusR, uidsR, uidsB[0])
	}
}
