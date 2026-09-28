package gate_test

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/changes"
	"github.com/PsyChaos/mindrail/internal/config"
	"github.com/PsyChaos/mindrail/internal/filesystem"
	"github.com/PsyChaos/mindrail/internal/gate"
	"github.com/PsyChaos/mindrail/internal/git"
	"github.com/PsyChaos/mindrail/internal/index"
	"github.com/PsyChaos/mindrail/internal/index/parser"
	"github.com/PsyChaos/mindrail/internal/index/snapshot"
	"github.com/PsyChaos/mindrail/internal/migration"
	"github.com/PsyChaos/mindrail/internal/storage"
	"github.com/PsyChaos/mindrail/internal/testguard"
	"github.com/PsyChaos/mindrail/internal/validation"
	"github.com/PsyChaos/mindrail/migrations"
)

type kernelFixture struct {
	changes *changes.Service
	store   *changes.Store
	indexes *index.Store
	valid   *validation.Service
	guard   *testguard.Service
	gate    *gate.Service
	db      *sql.DB
	root    string
	pkg     string
	unit    index.ProjectUnit
}

func newKernelFixture(t *testing.T) kernelFixture {
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
	changeStore, err := changes.NewStore(db.DB, clock)
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
	changeService, err := changes.New(changeStore, indexes, indexer)
	if err != nil {
		t.Fatal(err)
	}
	runner, err := validation.NewRunner(root, 10*time.Second, 0)
	if err != nil {
		t.Fatal(err)
	}
	evidenceStore, err := validation.NewStore(db.DB, clock)
	if err != nil {
		t.Fatal(err)
	}
	valid, err := validation.NewService(runner, evidenceStore)
	if err != nil {
		t.Fatal(err)
	}
	guard, err := testguard.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(guard.Close)
	pkg := filepath.Join(root, "pkg")
	if err := os.MkdirAll(pkg, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pkg, "pyproject.toml"), []byte("[project]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	unit, err := indexes.UpsertUnit(t.Context(), pkg, index.UnitPython)
	if err != nil {
		t.Fatal(err)
	}
	seedKernelTasks(t, db.DB, "TSK-A", "TSK-B")
	return kernelFixture{changes: changeService, store: changeStore, indexes: indexes,
		valid: valid, guard: guard, gate: gate.New(), db: db.DB, root: root, pkg: pkg, unit: unit}
}

func seedKernelTasks(t *testing.T, db *sql.DB, tasks ...string) {
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

func fakeStatusRunner(stdout string) *git.FakeRunner {
	return &git.FakeRunner{
		Responses: map[string]git.FakeResponse{
			"status --porcelain=v1 -z --untracked-files=all -- .": {Stdout: stdout},
			"diff --no-color --name-status -z -M HEAD -- .":       {Stdout: ""},
		},
	}
}

// TestCompletionKernelEndToEnd is TASK-02 AC-02.1. The kernel scenario with
// real composition and no declarations: an undeclared edit reconciles into
// existence, overlapping baselines ambiguate it, and the gate DENYs naming
// both claimants. Override, fresh evidence and a settled binding flip the
// same flow to ALLOW.
func TestCompletionKernelEndToEnd(t *testing.T) {
	fx := newKernelFixture(t)
	shared := filepath.Join(fx.pkg, "a.py")
	if err := os.WriteFile(shared, []byte("def helper():\n    return 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// No before_change anywhere: reconcile discovers the edit cold.
	reconciled, err := fx.changes.Reconcile(t.Context(), "PRJ-KERNEL", fx.root, "TSK-A", "",
		fakeStatusRunner(" M pkg/a.py\x00"))
	if err != nil {
		t.Fatal(err)
	}
	aSymbols, err := fx.store.ReadChangeSymbols(t.Context(), reconciled.Change.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(aSymbols) == 0 {
		t.Fatal("reconcile discovered no symbols")
	}
	// Overlapping baselines after discovery: candidacy is path membership.
	if _, err := fx.store.CaptureBaseline(t.Context(), "TSK-A", []string{shared}, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := fx.store.CaptureBaseline(t.Context(), "TSK-B", []string{shared}, ""); err != nil {
		t.Fatal(err)
	}
	changeB, err := fx.store.EnsureOpenChange(t.Context(), "TSK-B", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := fx.store.UpsertFileRows(t.Context(), changeB.ID, []changes.FileChange{
		{Path: shared, Kind: changes.FileModified, Via: changes.ViaReconcile},
	}); err != nil {
		t.Fatal(err)
	}
	if err := fx.store.UpsertSymbolRows(t.Context(), changeB.ID, aSymbols); err != nil {
		t.Fatal(err)
	}

	blocked, err := fx.changes.EvaluateTask(t.Context(), "TSK-A", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(blocked) == 0 {
		t.Fatal("overlapping baselines produced no findings")
	}
	deny, err := fx.gate.Evaluate(gate.Input{Attribution: blocked})
	if err != nil {
		t.Fatal(err)
	}
	if deny.Allow || len(deny.Denials) == 0 {
		t.Fatalf("decision = %+v, want DENY", deny)
	}
	namesBoth := false
	for _, denial := range deny.Denials {
		if denial.Code != app.CodeReconcileAmbiguous {
			t.Fatalf("code = %q, want ambiguity", denial.Code)
		}
		if denial.Reason == "" || len(denial.NextAction) == 0 {
			t.Fatalf("denial unexplained: %+v", denial)
		}
		if containsBoth(denial.Reason, reconciled.Change.ID, changeB.ID) {
			namesBoth = true
		}
	}
	if !namesBoth {
		t.Fatalf("no denial names both changes: %+v", deny.Denials)
	}

	// Resolve: assign every symbol, run fresh evidence, settle the binding.
	for _, symbol := range aSymbols {
		if err := fx.store.RecordAttribution(t.Context(), symbol.Key,
			reconciled.Change.ID, "SES-1", "A owns it"); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(fx.root, "tests"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fx.root, "tests", "t.py"), []byte("print(1)\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	profile := config.ValidationProfile{Type: "AUTOMATED_TEST", Paths: []string{"tests"},
		Commands: [][]string{{"echo", "hi"}}}
	rows, err := fx.valid.RunProfile(t.Context(), "test", profile, fx.root, nil, "OP-KERNEL")
	if err != nil {
		t.Fatal(err)
	}
	verdicts, coverage, err := validation.Check(fx.root, rows, []string{"test"})
	if err != nil {
		t.Fatal(err)
	}
	if verdicts[0].Status != validation.FreshCurrent {
		t.Fatalf("fresh evidence = %+v", verdicts)
	}
	// The evidence leg is load-bearing: editing the scope after the run
	// must DENY on evidence before any fresh run restores ALLOW.
	if err := os.WriteFile(filepath.Join(fx.root, "tests", "t.py"), []byte("print(2)\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, staleCoverage, err := validation.Check(fx.root, rows, []string{"test"})
	if err != nil {
		t.Fatal(err)
	}
	staleDeny, err := fx.gate.Evaluate(gate.Input{Coverage: staleCoverage})
	if err != nil {
		t.Fatal(err)
	}
	if staleDeny.Allow || len(staleDeny.Denials) != 1 ||
		staleDeny.Denials[0].Code != app.CodeRequiredEvidenceNotCurrent {
		t.Fatalf("stale evidence = %+v, want evidence DENY", staleDeny)
	}
	rows, err = fx.valid.RunProfile(t.Context(), "test", profile, fx.root, nil, "OP-KERNEL-2")
	if err != nil {
		t.Fatal(err)
	}
	_, coverage, err = validation.Check(fx.root, rows, []string{"test"})
	if err != nil {
		t.Fatal(err)
	}
	clear, err := fx.changes.EvaluateTask(t.Context(), "TSK-A", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(clear) != 0 {
		t.Fatalf("attribution still blocked: %+v", clear)
	}
	allow, err := fx.gate.Evaluate(gate.Input{
		Coverage: coverage,
		Bindings: []gate.InvariantBlock{{
			InvariantID: "INV-WARN", UID: "SYM-W", Status: "orphaned",
			Severity: "MEDIUM", Active: true,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !allow.Allow || len(allow.Denials) != 0 {
		t.Fatalf("resolved decision = %+v", allow)
	}
}

func containsBoth(haystack, a, b string) bool {
	return strings.Contains(haystack, a) && strings.Contains(haystack, b)
}

// TestGateFamilyBeatsEndToEnd fires every remaining DENY family through
// real composition: stale evidence via run→edit→check, a blocking guard
// via the real testguard service, and an orphaned binding by value.
func TestGateFamilyBeatsEndToEnd(t *testing.T) {
	fx := newKernelFixture(t)
	if err := os.MkdirAll(filepath.Join(fx.root, "tests"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fx.root, "tests", "t.py"), []byte("print(1)\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	profile := config.ValidationProfile{Type: "AUTOMATED_TEST", Paths: []string{"tests"},
		Commands: [][]string{{"echo", "hi"}}}
	rows, err := fx.valid.RunProfile(t.Context(), "test", profile, fx.root, nil, "OP-BEATS")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fx.root, "tests", "t.py"), []byte("print(2)\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, coverage, err := validation.Check(fx.root, rows, []string{"test"})
	if err != nil {
		t.Fatal(err)
	}
	stale, err := fx.gate.Evaluate(gate.Input{Coverage: coverage})
	if err != nil {
		t.Fatal(err)
	}
	if stale.Allow || len(stale.Denials) != 1 ||
		stale.Denials[0].Code != app.CodeRequiredEvidenceNotCurrent {
		t.Fatalf("stale = %+v", stale)
	}

	guarded, err := fx.guard.Evaluate(t.Context(), testguard.Request{
		Files: []testguard.FileDelta{{
			Path: "tests/test_token.py", Language: testguard.LanguagePython,
			Before: []byte("def test_token():\n    assert issue()\n"),
			After:  []byte("def test_other():\n    assert x\n"),
		}},
		Mappings: []testguard.TestMapping{{
			Path: "tests/test_token.py", Test: "test_token",
			ProductionUID: "SYM-TOKEN", InvariantID: "INV-CRIT",
			Severity: "CRITICAL", Active: true,
		}},
		Trigger: testguard.TriggerCI,
	})
	if err != nil {
		t.Fatal(err)
	}
	weakened, err := fx.gate.Evaluate(gate.Input{Guard: guarded.Findings})
	if err != nil {
		t.Fatal(err)
	}
	if weakened.Allow || len(weakened.Denials) != 1 ||
		weakened.Denials[0].Code != app.CodeTestGuardWeakened {
		t.Fatalf("weakened = %+v", weakened)
	}

	orphaned, err := fx.gate.Evaluate(gate.Input{
		Bindings: []gate.InvariantBlock{{
			InvariantID: "INV-CRIT", UID: "SYM-GONE", Status: "orphaned",
			Severity: "CRITICAL", Active: true,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if orphaned.Allow || len(orphaned.Denials) != 1 ||
		orphaned.Denials[0].Code != app.CodeOrphanedProtectedSymbol {
		t.Fatalf("orphaned = %+v", orphaned)
	}
}
