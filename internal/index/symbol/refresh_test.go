package symbol_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
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

const refreshProject = "PRJ-TEST-REFRESH"

const packageSource = `def f():
    return 1


def tool():
    return 4


class f:
    pass


class Box:
    def run(self):
        return f()

    def helper(self):
        return 2


class Crate:
    def run(self):
        return 3
`

type refreshFixture struct {
	store    *index.Store
	db       *storage.DB
	registry *parser.Registry
	indexer  *index.Indexer
	service  *symbol.Service
	root     string
	unit     index.ProjectUnit
}

func newRefreshFixture(t *testing.T) refreshFixture {
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
	store := index.NewStore(db.DB, clock)
	pkg := filepath.Join(root, "pkg")
	if err := os.MkdirAll(pkg, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pkg, "pyproject.toml"), []byte("[project]\nname = 'pkg'\n"), 0o644); err != nil {
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
	t.Cleanup(registry.Close)
	indexer := index.NewIndexer(store, registry, snapshot.New(filesystem.RuntimePaths{CacheDir: filepath.Join(root, "cache")}))
	service, err := symbol.New(store)
	if err != nil {
		t.Fatal(err)
	}
	return refreshFixture{store: store, db: db, registry: registry, indexer: indexer, service: service, root: root, unit: unit}
}

func (f refreshFixture) indexFile(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(f.unit.Path, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := f.indexer.IndexFile(t.Context(), refreshProject, f.unit, path); err != nil {
		t.Fatal(err)
	}
	return path
}

func activeInvariant(id string, severity record.Severity, level record.ScopeLevel, target string) record.Invariant {
	return record.Invariant{
		SchemaVersion: 1, Kind: "invariant", ID: id, Status: record.StatusActive,
		Severity: severity, Scope: record.Scope{Level: level, Target: target},
	}
}

// TestResolveTargetScopeForms is AC-03.1: every scope form resolves to the
// expected uid or the expected non-match, through the production index path.
func TestResolveTargetScopeForms(t *testing.T) {
	f := newRefreshFixture(t)
	f.indexFile(t, "a.py", packageSource)
	rel := "pkg/a.py"

	cases := []struct {
		name    string
		target  string
		verdict symbol.Verdict
		check   func(t *testing.T, r symbol.Resolution)
	}{
		{"module function", rel + ":tool", symbol.VerdictResolved, func(t *testing.T, r symbol.Resolution) {
			if r.UID == "" {
				t.Fatal("no uid")
			}
		}},
		{"method", rel + ":Box.run", symbol.VerdictResolved, func(t *testing.T, r symbol.Resolution) {
			if r.UID == "" {
				t.Fatal("no uid")
			}
		}},
		{"same name across kinds is ambiguous", rel + ":f", symbol.VerdictAmbiguous, func(t *testing.T, r symbol.Resolution) {
			if len(r.Candidates) != 2 {
				t.Fatalf("candidates = %d, want the function and the class", len(r.Candidates))
			}
		}},
		{"bare method name is unresolvable", rel + ":run", symbol.VerdictUnresolvable, nil},
		{"unknown name", rel + ":nope", symbol.VerdictUnresolvable, nil},
		{"unknown container", rel + ":Missing.run", symbol.VerdictUnresolvable, nil},
		{"whole file fans out", rel, symbol.VerdictAmbiguous, func(t *testing.T, r symbol.Resolution) {
			if len(r.Candidates) != 8 {
				t.Fatalf("candidates = %d, want all eight lineages", len(r.Candidates))
			}
		}},
		{"outside units", "elsewhere/b.py:f", symbol.VerdictUnresolvable, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := f.service.ResolveTarget(t.Context(), refreshProject, f.root, []index.ProjectUnit{f.unit}, tc.target)
			if err != nil {
				t.Fatal(err)
			}
			if got.Verdict != tc.verdict {
				t.Fatalf("verdict = %q (%s), want %q", got.Verdict, got.Detail, tc.verdict)
			}
			if tc.check != nil {
				tc.check(t, got)
			}
		})
	}
}

// TestResolveContainerCollisionIsAmbiguous pins the container-level walk:
// a class and a function sharing one name under one container diverge, and
// descending into either child by guess is refused.
func TestResolveContainerCollisionIsAmbiguous(t *testing.T) {
	f := newRefreshFixture(t)
	f.indexFile(t, "a.py", packageSource)
	f.indexFile(t, "b.py", "def Dup():\n    pass\n\n\nclass Dup:\n    def go(self):\n        pass\n")
	got, err := f.service.ResolveTarget(t.Context(), refreshProject, f.root, []index.ProjectUnit{f.unit}, "pkg/b.py:Dup.go")
	if err != nil {
		t.Fatal(err)
	}
	if got.Verdict != symbol.VerdictAmbiguous || len(got.Candidates) != 2 {
		t.Fatalf("verdict = %q candidates %d, want ambiguous over both Dup lineages", got.Verdict, len(got.Candidates))
	}
}

// TestRefreshBindingsWritesBoundRows is AC-03.2's resolvable half: active
// invariants bind, re-refresh is idempotent, and bindings name live uids.
func TestRefreshBindingsWritesBoundRows(t *testing.T) {
	f := newRefreshFixture(t)
	f.indexFile(t, "a.py", packageSource)
	invariants := []record.Invariant{
		activeInvariant("INV-0001", record.SeverityCritical, record.ScopeSymbol, "pkg/a.py:Box.run"),
		activeInvariant("INV-0002", record.SeverityMedium, record.ScopeFile, "pkg/a.py"),
		activeInvariant("INV-0007", record.SeverityLow, record.ScopeProject, ""),
	}
	report, err := f.service.RefreshBindings(t.Context(), refreshProject, f.root, []index.ProjectUnit{f.unit}, invariants)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Outcomes) != 3 {
		t.Fatalf("outcomes = %d, want 3", len(report.Outcomes))
	}
	if project := report.Outcomes[2]; project.Status != index.BindingBound || project.Finding != nil || len(project.UIDs) != 0 {
		t.Fatalf("project outcome = %+v, want quiet bound with no uids", project)
	}
	projectRows, err := f.store.ListBindingsForInvariant(t.Context(), "INV-0007")
	if err != nil {
		t.Fatal(err)
	}
	if len(projectRows) != 0 {
		t.Fatalf("project scope left %d rows", len(projectRows))
	}
	for _, outcome := range report.Outcomes {
		if outcome.Status != index.BindingBound || outcome.Finding != nil || outcome.Blocking {
			t.Fatalf("outcome = %+v, want quiet bound", outcome)
		}
	}
	bound, err := f.store.ListBindingsForInvariant(t.Context(), "INV-0001")
	if err != nil || len(bound) != 1 || bound[0].Status != index.BindingBound {
		t.Fatalf("bindings = %+v, %v", bound, err)
	}
	fileBound, err := f.store.ListBindingsForInvariant(t.Context(), "INV-0002")
	if err != nil || len(fileBound) != 8 {
		t.Fatalf("file bindings = %d, want one per lineage", len(fileBound))
	}
	again, err := f.service.RefreshBindings(t.Context(), refreshProject, f.root, []index.ProjectUnit{f.unit}, invariants)
	if err != nil {
		t.Fatal(err)
	}
	still, err := f.store.ListBindingsForInvariant(t.Context(), "INV-0001")
	if err != nil || len(still) != 1 || still[0].UID != bound[0].UID {
		t.Fatalf("re-refresh moved the binding: %+v", still)
	}
	_ = again
}

// TestStickyBindingSurvivesStaleTargetText pins decision D-110: a bound
// lineage with live rows keeps its binding even when the target text no
// longer walks to it. The rename-carried case this enables arrives in
// TASK-04; here the mechanism is pinned with a hand-staled target.
func TestStickyBindingSurvivesStaleTargetText(t *testing.T) {
	f := newRefreshFixture(t)
	path := f.indexFile(t, "a.py", packageSource)
	rows, err := f.store.ListSymbolsInFile(t.Context(), f.unit.ID, path)
	if err != nil {
		t.Fatal(err)
	}
	var uid string
	for _, row := range rows {
		if row.Name == "run" {
			uid = row.UID
			break
		}
	}
	if uid == "" {
		t.Fatal("no run uid")
	}
	if err := f.store.UpsertBinding(t.Context(), "INV-0009", uid, index.BindingBound, ""); err != nil {
		t.Fatal(err)
	}
	// The target text now names nothing, but the lineage is alive.
	report, err := f.service.RefreshBindings(t.Context(), refreshProject, f.root, []index.ProjectUnit{f.unit},
		[]record.Invariant{activeInvariant("INV-0009", record.SeverityCritical, record.ScopeSymbol, "pkg/a.py:renamed")})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Outcomes) != 1 || report.Outcomes[0].Status != index.BindingBound || len(report.Outcomes[0].UIDs) != 1 || report.Outcomes[0].UIDs[0] != uid {
		t.Fatalf("outcome = %+v, want sticky bound to %s", report.Outcomes, uid)
	}
	if report.Outcomes[0].Finding != nil || report.Outcomes[0].Blocking {
		t.Fatalf("sticky outcome carries a finding: %+v", report.Outcomes[0])
	}
}

// TestRefreshDefersColdFiles is AC-03.3: pending files produce no rows, no
// findings and no blocks.
func TestRefreshDefersColdFiles(t *testing.T) {
	f := newRefreshFixture(t)
	path := filepath.Join(f.unit.Path, "cold.py")
	if err := os.WriteFile(path, []byte("def f():\n    pass\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := f.store.UpsertFileState(t.Context(), index.FileIndexState{
		UnitID: f.unit.ID, Path: path, Language: "python", State: index.StatePending,
	}); err != nil {
		t.Fatal(err)
	}
	report, err := f.service.RefreshBindings(t.Context(), refreshProject, f.root, []index.ProjectUnit{f.unit},
		[]record.Invariant{activeInvariant("INV-0003", record.SeverityCritical, record.ScopeSymbol, "pkg/cold.py:f")})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Outcomes) != 1 || report.Outcomes[0].Status != "deferred" {
		t.Fatalf("outcome = %+v, want deferred", report.Outcomes)
	}
	if report.Outcomes[0].Finding != nil || report.Outcomes[0].Blocking {
		t.Fatalf("deferred outcome blocks: %+v", report.Outcomes[0])
	}
	bound, err := f.store.ListBindingsForInvariant(t.Context(), "INV-0003")
	if err != nil {
		t.Fatal(err)
	}
	if len(bound) != 0 {
		t.Fatalf("deferred invariant left %d rows", len(bound))
	}
}

// TestOrphanBlocksOnCritical is AC-03.4: a removed symbol over an indexed
// file records orphaned bindings, blocking iff HIGH/CRITICAL. The CRITICAL
// case pre-binds first, so the test pins the stored row flipping to orphaned
// (not just the outcome); the LOW case never bound, pinning the finding-only
// half of decision D-111.
func TestOrphanBlocksOnCritical(t *testing.T) {
	f := newRefreshFixture(t)
	path := f.indexFile(t, "a.py", packageSource)
	pre, err := f.service.RefreshBindings(t.Context(), refreshProject, f.root, []index.ProjectUnit{f.unit},
		[]record.Invariant{activeInvariant("INV-0004", record.SeverityCritical, record.ScopeSymbol, "pkg/a.py:tool")})
	if err != nil || len(pre.Outcomes) != 1 || pre.Outcomes[0].Status != index.BindingBound {
		t.Fatalf("pre-bind = %+v, %v", pre.Outcomes, err)
	}
	// The symbol leaves the file; the file stays indexed.
	if err := os.WriteFile(path, []byte("def other():\n    pass\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := f.indexer.IndexFile(t.Context(), refreshProject, f.unit, path); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		id       string
		severity record.Severity
		blocking bool
	}{
		{"INV-0004", record.SeverityCritical, true},
		{"INV-0005", record.SeverityLow, false},
	} {
		report, err := f.service.RefreshBindings(t.Context(), refreshProject, f.root, []index.ProjectUnit{f.unit},
			[]record.Invariant{activeInvariant(tc.id, tc.severity, record.ScopeSymbol, "pkg/a.py:f")})
		if err != nil {
			t.Fatal(err)
		}
		if len(report.Outcomes) != 1 {
			t.Fatalf("outcomes = %d", len(report.Outcomes))
		}
		outcome := report.Outcomes[0]
		if outcome.Status != index.BindingOrphaned || outcome.Blocking != tc.blocking {
			t.Fatalf("%s outcome = %+v, want orphaned blocking=%t", tc.id, outcome, tc.blocking)
		}
		rows, err := f.store.ListBindingsForInvariant(t.Context(), tc.id)
		if err != nil {
			t.Fatal(err)
		}
		if tc.id == "INV-0004" {
			if len(rows) != 1 || rows[0].Status != index.BindingOrphaned {
				t.Fatalf("INV-0004 stored rows = %+v, want one orphaned flip", rows)
			}
		} else if len(rows) != 0 {
			t.Fatalf("never-bound INV-0005 left %d rows, want finding-only", len(rows))
		}
		if outcome.Finding == nil {
			t.Fatalf("%s outcome carries no finding", tc.id)
		}
		if payload, ok := app.PayloadOf(outcome.Finding); !ok || payload.Code != app.CodeOrphanedProtectedSymbol {
			t.Fatalf("%s finding = %v, want ORPHANED_PROTECTED_SYMBOL", tc.id, outcome.Finding)
		}
	}
}

// TestRefreshDropsInactiveBindings keeps the table describing active
// protection: a superseded invariant's rows are deleted, with no outcome.
func TestRefreshDropsInactiveBindings(t *testing.T) {
	f := newRefreshFixture(t)
	path := f.indexFile(t, "a.py", packageSource)
	rows, err := f.store.ListSymbolsInFile(t.Context(), f.unit.ID, path)
	if err != nil || len(rows) == 0 || rows[0].UID == "" {
		t.Fatalf("rows = %+v, %v", rows, err)
	}
	if err := f.store.UpsertBinding(t.Context(), "INV-0006", rows[0].UID, index.BindingBound, ""); err != nil {
		t.Fatal(err)
	}
	stale := activeInvariant("INV-0006", record.SeverityCritical, record.ScopeSymbol, "pkg/a.py:f")
	stale.Status = record.StatusSuperseded
	report, err := f.service.RefreshBindings(t.Context(), refreshProject, f.root, []index.ProjectUnit{f.unit}, []record.Invariant{stale})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Outcomes) != 0 {
		t.Fatalf("outcomes = %+v, want none for inactive", report.Outcomes)
	}
	kept, err := f.store.ListBindingsForInvariant(t.Context(), "INV-0006")
	if err != nil {
		t.Fatal(err)
	}
	if len(kept) != 0 {
		t.Fatalf("inactive invariant kept %d rows", len(kept))
	}
}

// TestRefreshBindsPrefixScopes is AC-03.1's MODULE/PACKAGE half: every uid
// under the prefix binds, and the separator guard keeps "pkg" from claiming
// sibling roots.
func TestRefreshBindsPrefixScopes(t *testing.T) {
	f := newRefreshFixture(t)
	f.indexFile(t, "a.py", packageSource)
	// A sibling root "pkg2" shares the "pkg" prefix byte-for-byte: the
	// separator guard must keep its lineages out of every "pkg" binding.
	sibling := filepath.Join(f.root, "pkg2")
	if err := os.MkdirAll(sibling, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sibling, "pyproject.toml"), []byte("[project]\nname = 'pkg2'\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	siblingUnit, err := f.store.UpsertUnit(t.Context(), sibling, index.UnitPython)
	if err != nil {
		t.Fatal(err)
	}
	siblingFile := filepath.Join(sibling, "a.py")
	if err := os.WriteFile(siblingFile, []byte("def f():\n    pass\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := f.indexer.IndexFile(t.Context(), refreshProject, siblingUnit, siblingFile); err != nil {
		t.Fatal(err)
	}
	units := []index.ProjectUnit{f.unit, siblingUnit}
	for _, tc := range []struct {
		id    string
		level record.ScopeLevel
		want  int
	}{
		{"INV-0010", record.ScopePackage, 8},
		{"INV-0011", record.ScopeModule, 8},
	} {
		report, err := f.service.RefreshBindings(t.Context(), refreshProject, f.root, units,
			[]record.Invariant{activeInvariant(tc.id, record.SeverityMedium, tc.level, "pkg")})
		if err != nil {
			t.Fatal(err)
		}
		if len(report.Outcomes) != 1 || report.Outcomes[0].Status != index.BindingBound || len(report.Outcomes[0].UIDs) != tc.want {
			t.Fatalf("%s outcome = %+v, want bound %d", tc.id, report.Outcomes, tc.want)
		}
	}
	module, err := f.service.RefreshBindings(t.Context(), refreshProject, f.root, units,
		[]record.Invariant{activeInvariant("INV-0012", record.SeverityMedium, record.ScopeModule, "pkg/a.py")})
	if err != nil {
		t.Fatal(err)
	}
	if len(module.Outcomes) != 1 || len(module.Outcomes[0].UIDs) != 8 {
		t.Fatalf("module outcome = %+v, want the file's eight", module.Outcomes)
	}
}

// TestRefreshRefusesMalformedScopes pins the fail-closed input validation:
// unknown levels and missing targets are errors, never quiet bindings.
func TestRefreshRefusesMalformedScopes(t *testing.T) {
	f := newRefreshFixture(t)
	f.indexFile(t, "a.py", packageSource)
	badLevel := activeInvariant("INV-0020", record.SeverityLow, "REGION", "pkg/a.py")
	if _, err := f.service.RefreshBindings(t.Context(), refreshProject, f.root, []index.ProjectUnit{f.unit}, []record.Invariant{badLevel}); err == nil {
		t.Fatal("unknown scope level accepted")
	}
	noTarget := activeInvariant("INV-0021", record.SeverityLow, record.ScopeSymbol, "")
	if _, err := f.service.RefreshBindings(t.Context(), refreshProject, f.root, []index.ProjectUnit{f.unit}, []record.Invariant{noTarget}); err == nil {
		t.Fatal("targetless symbol scope accepted")
	}
	if _, err := f.service.RefreshBindings(t.Context(), "", f.root, []index.ProjectUnit{f.unit}, nil); err == nil {
		t.Fatal("empty project accepted")
	}
}

// TestOutsideUnitTargetIsOrphaned pins decision D-110's second half: a path
// no unit owns can never produce rows, so absence is proven and the track is
// orphan, blocking iff HIGH/CRITICAL.
func TestOutsideUnitTargetIsOrphaned(t *testing.T) {
	f := newRefreshFixture(t)
	f.indexFile(t, "a.py", packageSource)
	report, err := f.service.RefreshBindings(t.Context(), refreshProject, f.root, []index.ProjectUnit{f.unit},
		[]record.Invariant{activeInvariant("INV-0030", record.SeverityHigh, record.ScopeSymbol, "elsewhere/b.py:f")})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Outcomes) != 1 {
		t.Fatalf("outcomes = %+v", report.Outcomes)
	}
	outcome := report.Outcomes[0]
	if outcome.Status != index.BindingOrphaned || !outcome.Blocking || outcome.Finding == nil {
		t.Fatalf("outcome = %+v, want blocking orphan", outcome)
	}
	if payload, ok := app.PayloadOf(outcome.Finding); !ok || payload.Code != app.CodeOrphanedProtectedSymbol {
		t.Fatalf("finding = %v", outcome.Finding)
	}
	kept, err := f.store.ListBindingsForInvariant(t.Context(), "INV-0030")
	if err != nil {
		t.Fatal(err)
	}
	if len(kept) != 0 {
		t.Fatalf("never-bound orphan left %d rows", len(kept))
	}
}

// TestAmbiguousTwinsBlockBySeverity pins blockOnSeverity at refresh level:
// the function/class twins share one target text, bind ambiguous rows each,
// and block iff HIGH/CRITICAL.
func TestAmbiguousTwinsBlockBySeverity(t *testing.T) {
	f := newRefreshFixture(t)
	f.indexFile(t, "a.py", packageSource)
	for _, tc := range []struct {
		id       string
		severity record.Severity
		blocking bool
	}{
		{"INV-0040", record.SeverityCritical, true},
		{"INV-0041", record.SeverityLow, false},
	} {
		report, err := f.service.RefreshBindings(t.Context(), refreshProject, f.root, []index.ProjectUnit{f.unit},
			[]record.Invariant{activeInvariant(tc.id, tc.severity, record.ScopeSymbol, "pkg/a.py:f")})
		if err != nil {
			t.Fatal(err)
		}
		if len(report.Outcomes) != 1 {
			t.Fatalf("outcomes = %+v", report.Outcomes)
		}
		outcome := report.Outcomes[0]
		if outcome.Status != index.BindingAmbiguous || outcome.Blocking != tc.blocking || len(outcome.UIDs) != 2 {
			t.Fatalf("%s outcome = %+v, want ambiguous 2-uid blocking=%t", tc.id, outcome, tc.blocking)
		}
		if outcome.Finding == nil {
			t.Fatalf("%s outcome carries no finding", tc.id)
		}
		if payload, ok := app.PayloadOf(outcome.Finding); !ok || payload.Code != app.CodeSymbolIdentityAmbiguous {
			t.Fatalf("%s finding = %v", tc.id, outcome.Finding)
		}
		payload, _ := app.PayloadOf(outcome.Finding)
		remedy := strings.Join(payload.NextAction, " ")
		if !strings.Contains(remedy, "Candidates:") || !strings.Contains(remedy, "several live lineages") {
			t.Fatalf("%s remedy = %q, want candidates and divergence detail", tc.id, remedy)
		}
		rows, err := f.store.ListBindingsForInvariant(t.Context(), tc.id)
		if err != nil || len(rows) != 2 {
			t.Fatalf("%s stored rows = %+v, %v", tc.id, rows, err)
		}
		for _, row := range rows {
			if row.Status != index.BindingAmbiguous {
				t.Fatalf("%s row = %+v, want ambiguous", tc.id, row)
			}
		}
	}
}

// TestResolvedRefreshPrunesStaleBindings pins the resolved-path prune: a
// binding the target no longer names is deleted, not left beside the new one.
func TestResolvedRefreshPrunesStaleBindings(t *testing.T) {
	f := newRefreshFixture(t)
	f.indexFile(t, "a.py", packageSource)
	rows, err := f.store.ListSymbolsInFile(t.Context(), f.unit.ID, filepath.Join(f.unit.Path, "a.py"))
	if err != nil || len(rows) == 0 {
		t.Fatalf("rows = %+v, %v", rows, err)
	}
	var toolUID string
	for _, row := range rows {
		if row.Name == "tool" {
			toolUID = row.UID
		}
	}
	if toolUID == "" {
		t.Fatal("no tool uid")
	}
	if err := f.store.UpsertBinding(t.Context(), "INV-0042", toolUID, index.BindingBound, ""); err != nil {
		t.Fatal(err)
	}
	report, err := f.service.RefreshBindings(t.Context(), refreshProject, f.root, []index.ProjectUnit{f.unit},
		[]record.Invariant{activeInvariant("INV-0042", record.SeverityMedium, record.ScopeSymbol, "pkg/a.py:Box.run")})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Outcomes) != 1 || report.Outcomes[0].Status != index.BindingBound {
		t.Fatalf("outcome = %+v, want bound", report.Outcomes)
	}
	kept, err := f.store.ListBindingsForInvariant(t.Context(), "INV-0042")
	if err != nil || len(kept) != 1 || kept[0].UID == toolUID {
		t.Fatalf("rows = %+v, want exactly the run uid", kept)
	}
}

// TestEndToEndProtectRenameAmbiguousDelete is AC-05.1 in three acts on
// stored rows: a CRITICAL invariant protects Box.run; a rename carries the
// binding on the same uid; twin heirs record ambiguity and the refresh
// reports it blocking with the ambiguous code (not an orphan: the lineage
// did not vanish, the decision is missing); removing the symbols orphans
// and blocks. Both blocking codes appear end to end.
func TestEndToEndProtectRenameAmbiguousDelete(t *testing.T) {
	f := newRefreshFixture(t)
	path := f.indexFile(t, "a.py", "class Box:\n    def run(self):\n        return 1\n")
	inv := activeInvariant("INV-1000", record.SeverityCritical, record.ScopeSymbol, "pkg/a.py:Box.run")

	// Act one: protect, then rename run into drive.
	report, err := f.service.RefreshBindings(t.Context(), refreshProject, f.root, []index.ProjectUnit{f.unit}, []record.Invariant{inv})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Outcomes) != 1 || report.Outcomes[0].Status != index.BindingBound {
		t.Fatalf("act one bind = %+v", report.Outcomes)
	}
	carried := report.Outcomes[0].UIDs[0]
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
	if len(report.Outcomes) != 1 || report.Outcomes[0].Status != index.BindingBound || len(report.Outcomes[0].UIDs) != 1 || report.Outcomes[0].UIDs[0] != carried {
		t.Fatalf("act one carried = %+v, want migrated-not-orphaned %s", report.Outcomes, carried)
	}
	rows, err := f.store.ListBindingsForInvariant(t.Context(), "INV-1000")
	if err != nil || len(rows) != 1 || rows[0].UID != carried || rows[0].Status != index.BindingBound {
		t.Fatalf("act one rows = %+v, %v", rows, err)
	}

	// Act two: the drive body twins into alpha and beta. The store records
	// ambiguity; the stale target orphans — both findings block.
	if err := os.WriteFile(path, []byte("class Box:\n    def alpha(self):\n        return 1\n    def beta(self):\n        return 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := f.indexer.IndexFile(t.Context(), refreshProject, f.unit, path); err != nil {
		t.Fatal(err)
	}
	var ambiguities int
	if err := f.db.DB.QueryRowContext(t.Context(), `SELECT count(*) FROM symbol_identity_ambiguities`).Scan(&ambiguities); err != nil || ambiguities != 1 {
		t.Fatalf("act two ambiguities = %d, %v", ambiguities, err)
	}
	var removedUID, candidateKeys string
	if err := f.db.DB.QueryRowContext(t.Context(), `SELECT removed_uid, candidate_keys FROM symbol_identity_ambiguities`).Scan(&removedUID, &candidateKeys); err != nil {
		t.Fatal(err)
	}
	var candidates []string
	// Keys are opaque hashes; the row names both heirs by count and identity.
	if err := json.Unmarshal([]byte(candidateKeys), &candidates); err != nil || len(candidates) != 2 || removedUID != carried {
		t.Fatalf("ambiguity = (%s, %s), want carried uid with two heir keys", removedUID, candidateKeys)
	}
	report, err = f.service.RefreshBindings(t.Context(), refreshProject, f.root, []index.ProjectUnit{f.unit}, []record.Invariant{inv})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Outcomes) != 1 {
		t.Fatalf("act two outcomes = %+v", report.Outcomes)
	}
	two := report.Outcomes[0]
	if two.Status != index.BindingAmbiguous || !two.Blocking || two.Finding == nil {
		t.Fatalf("act two outcome = %+v, want ambiguous blocking", two)
	}
	if payload, ok := app.PayloadOf(two.Finding); !ok || payload.Code != app.CodeSymbolIdentityAmbiguous {
		t.Fatalf("act two finding = %v, want SYMBOL_IDENTITY_AMBIGUOUS", two.Finding)
	}
	twoRows, err := f.store.ListBindingsForInvariant(t.Context(), "INV-1000")
	if err != nil || len(twoRows) != 1 || twoRows[0].Status != index.BindingAmbiguous || twoRows[0].UID != carried {
		t.Fatalf("act two rows = %+v, %v", twoRows, err)
	}

	// Act three: the symbols leave the file; the orphan blocks.
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
	if len(report.Outcomes) != 1 || report.Outcomes[0].Status != index.BindingOrphaned || !report.Outcomes[0].Blocking {
		t.Fatalf("act three outcome = %+v, want orphaned blocking", report.Outcomes)
	}
	if payload, ok := app.PayloadOf(report.Outcomes[0].Finding); !ok || payload.Code != app.CodeOrphanedProtectedSymbol {
		t.Fatalf("act three finding = %v", report.Outcomes[0].Finding)
	}
	threeRows, err := f.store.ListBindingsForInvariant(t.Context(), "INV-1000")
	if err != nil || len(threeRows) != 0 {
		t.Fatalf("act three rows = %+v, %v, want finding-only orphan", threeRows, err)
	}
	if len(report.Outcomes[0].UIDs) != 0 {
		t.Fatalf("act three uids = %+v, want none (finding-only)", report.Outcomes[0].UIDs)
	}
}

// TestRefreshTouchesNoKnowledgeFiles is AC-03.5's structural half: the
// symbol package performs no filesystem writes, so knowledge records cannot
// move under a refresh. The assertion scans package sources for write calls
// rather than trusting the import list.
func TestRefreshTouchesNoKnowledgeFiles(t *testing.T) {
	dir := symbolPackageDir(t)
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	banned := []string{"os.WriteFile", "os.Create", "os.Remove", "os.Rename", "os.MkdirAll", "os.OpenFile", "os.Write(", "ioutil.WriteFile"}
	for _, entry := range entries {
		if entry.IsDir() || len(entry.Name()) < 4 || entry.Name()[len(entry.Name())-3:] != ".go" {
			continue
		}
		if strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		body, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		for _, call := range banned {
			if strings.Contains(string(body), call) {
				t.Errorf("%s performs filesystem writes (%s)", entry.Name(), call)
			}
		}
		for _, line := range strings.Split(string(body), "\n") {
			line = strings.TrimSpace(line)
			if !strings.HasPrefix(line, "\"") || !strings.Contains(line, "internal/knowledge/") {
				continue
			}
			if !strings.Contains(line, "internal/knowledge/record\"") {
				t.Errorf("%s imports %s: only knowledge/record is readable here", entry.Name(), line)
			}
		}
	}
}

func symbolPackageDir(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("caller unavailable")
	}
	return filepath.Dir(file)
}
