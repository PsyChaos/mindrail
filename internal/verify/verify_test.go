package verify_test

import (
	"database/sql"
	"os"
	"os/exec"
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
	"github.com/PsyChaos/mindrail/internal/testguard"
	"github.com/PsyChaos/mindrail/internal/verify"
	"github.com/PsyChaos/mindrail/migrations"
)

type stagedFixture struct {
	service *verify.Service
	store   *changes.Store
	db      *sql.DB
	root    string
	pkg     string
	runner  git.CommandRunner
}

func newStagedFixture(t *testing.T) stagedFixture {
	t.Helper()
	root := t.TempDir()
	if out, err := exec.Command("git", "init", "--quiet", root).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
	for _, args := range [][]string{
		{"-C", root, "config", "user.email", "t@t"},
		{"-C", root, "config", "user.name", "t"},
	} {
		if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
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
	guard, err := testguard.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(guard.Close)
	service, err := verify.New(changeService, indexes, guard)
	if err != nil {
		t.Fatal(err)
	}
	pkg := filepath.Join(root, "pkg")
	if err := os.MkdirAll(pkg, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pkg, "pyproject.toml"), []byte("[project]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := indexes.UpsertUnit(t.Context(), pkg, index.UnitPython); err != nil {
		t.Fatal(err)
	}
	seedVerifyTasks(t, db.DB, "TSK-A", "TSK-B")
	return stagedFixture{service: service, store: changeStore, db: db.DB,
		root: root, pkg: pkg, runner: git.NewExecRunner()}
}

func seedVerifyTasks(t *testing.T, db *sql.DB, tasks ...string) {
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

func gitStage(t *testing.T, root string, args ...string) {
	t.Helper()
	full := append([]string{"-C", root}, args...)
	if out, err := exec.Command("git", full...).CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", full, err, out)
	}
}

func denialCodes(t *testing.T, verdict verify.Verdict) []string {
	t.Helper()
	var codes []string
	for _, denial := range verdict.Denials {
		if denial.Code == "" || denial.Reason == "" || len(denial.NextAction) == 0 {
			t.Fatalf("denial unexplained: %+v", denial)
		}
		codes = append(codes, string(denial.Code))
	}
	return codes
}

// TestVerifyStagedCleanGreen is TASK-01 AC-01.4: an empty index verifies
// green with no denials.
func TestVerifyStagedCleanGreen(t *testing.T) {
	fx := newStagedFixture(t)
	verdict, err := fx.service.VerifyStaged(t.Context(), "PRJ-1", fx.root, fx.runner)
	if err != nil {
		t.Fatal(err)
	}
	if !verdict.Allow || len(verdict.Denials) != 0 {
		t.Fatalf("verdict = %+v", verdict)
	}
}

// TestVerifyStagedAmbiguityDenies is TASK-01 AC-01.1: a staged shared file
// under overlapping baselines denies with the ambiguity code through the
// shared attribution.
func TestVerifyStagedAmbiguityDenies(t *testing.T) {
	fx := newStagedFixture(t)
	shared := filepath.Join(fx.pkg, "a.py")
	if err := os.WriteFile(shared, []byte("def helper():\n    return 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitStage(t, fx.root, "add", "pkg/a.py")
	gitStage(t, fx.root, "commit", "--quiet", "-m", "base")
	for _, task := range []string{"TSK-A", "TSK-B"} {
		if _, err := fx.store.CaptureBaseline(t.Context(), task, []string{shared}, ""); err != nil {
			t.Fatal(err)
		}
		if _, err := fx.store.EnsureOpenChange(t.Context(), task, ""); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(shared, []byte("def helper():\n    return 2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitStage(t, fx.root, "add", "pkg/a.py")

	verdict, err := fx.service.VerifyStaged(t.Context(), "PRJ-1", fx.root, fx.runner)
	if err != nil {
		t.Fatal(err)
	}
	if verdict.Allow {
		t.Fatalf("verdict = %+v, want DENY", verdict)
	}
	codes := denialCodes(t, verdict)
	found := false
	for _, code := range codes {
		if code == "RECONCILE_AMBIGUOUS" {
			found = true
		}
	}
	if !found {
		t.Fatalf("codes = %+v, want ambiguity", codes)
	}
}

// TestVerifyStagedGuardBlocks is TASK-01 AC-01.2: a staged weakened test
// with a bound CRITICAL mapping denies blocking through the shared guard.
// Two passes: the first populates index and change rows, the second judges
// with seeded binding + resolved reference standing in for resolution.
func TestVerifyStagedGuardBlocks(t *testing.T) {
	fx := newStagedFixture(t)
	test := filepath.Join(fx.pkg, "test_a.py")
	base := "def helper():\n    return 1\n\ndef test_helper():\n    assert helper()\n"
	if err := os.WriteFile(test, []byte(base), 0o644); err != nil {
		t.Fatal(err)
	}
	gitStage(t, fx.root, "add", "pkg/test_a.py")
	gitStage(t, fx.root, "commit", "--quiet", "-m", "base")
	if _, err := fx.store.CaptureBaseline(t.Context(), "TSK-A", []string{test}, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := fx.store.EnsureOpenChange(t.Context(), "TSK-A", ""); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(test, []byte("def helper():\n    return 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitStage(t, fx.root, "add", "pkg/test_a.py")

	first, err := fx.service.VerifyStaged(t.Context(), "PRJ-1", fx.root, fx.runner)
	if err != nil {
		t.Fatal(err)
	}
	_ = first
	var helperUID string
	if err := fx.db.QueryRowContext(t.Context(),
		`SELECT symbol_uid FROM symbols WHERE path = ? AND name = 'helper'`, test).Scan(&helperUID); err != nil {
		t.Fatal(err)
	}
	seedGuardRows(t, fx, test, helperUID)
	second, err := fx.service.VerifyStaged(t.Context(), "PRJ-1", fx.root, fx.runner)
	if err != nil {
		t.Fatal(err)
	}
	if second.Allow {
		t.Fatalf("verdict = %+v, want DENY", second)
	}
	codes := denialCodes(t, second)
	found := false
	for _, code := range codes {
		if code == "TEST_GUARD_WEAKENED" {
			found = true
		}
	}
	if !found {
		t.Fatalf("codes = %+v, want guard denial", codes)
	}
}

func seedGuardRows(t *testing.T, fx stagedFixture, test, helperUID string) {
	t.Helper()
	if _, err := fx.db.ExecContext(t.Context(), `INSERT OR IGNORE INTO symbol_identities
		(symbol_uid, project_id, unit_id, language, logical_key, previous_keys, created_at)
		SELECT 'SYM-T-SEED', project_id, unit_id, language, 'test-key', '[]', '2026-09-23T10:00:00Z'
		FROM symbol_identities WHERE symbol_uid = ?`, helperUID); err != nil {
		t.Fatal(err)
	}
	if _, err := fx.db.ExecContext(t.Context(), `INSERT INTO symbols
		(unit_id, path, logical_key, kind, name, start_line, start_col, end_line, end_col,
		signature_hash, body_hash, structure_hash, symbol_uid)
		SELECT unit_id, ?, 'test-key', 'function', 'test_helper', 1, 0, 2, 0, 's', 'b', 't', 'SYM-T-SEED'
		FROM symbols WHERE symbol_uid = ?`, test, helperUID); err != nil {
		t.Fatal(err)
	}
	var helperID int64
	if err := fx.db.QueryRowContext(t.Context(), `SELECT id FROM symbols WHERE symbol_uid = ?`,
		helperUID).Scan(&helperID); err != nil {
		t.Fatal(err)
	}
	if _, err := fx.db.ExecContext(t.Context(), `INSERT INTO symbol_references
		(unit_id, path, referrer_key, target_text, label, confidence, resolved_symbol_id)
		SELECT unit_id, ?, 'test-key', 'helper', 'STRUCTURAL_NAME_MATCH', 0.5, ?
		FROM symbols WHERE symbol_uid = ?`, test, helperID, helperUID); err != nil {
		t.Fatal(err)
	}
	if _, err := fx.db.ExecContext(t.Context(), `INSERT INTO invariant_symbol_bindings
		(invariant_id, symbol_uid, status, updated_at)
		VALUES ('INV-G', ?, 'bound', '2026-09-23T10:00:00Z')`, helperUID); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(fx.root, ".mindrail", "knowledge", "invariants")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	inv := `{"schema_version":1,"kind":"invariant","id":"INV-G","status":"active",` +
		`"created_at":"2026-09-23T10:00:00Z","statement":"Helpers hold.",` +
		`"severity":"CRITICAL","scope":{"level":"PROJECT"}}`
	if err := os.WriteFile(filepath.Join(dir, "INV-G.json"), []byte(inv), 0o644); err != nil {
		t.Fatal(err)
	}
}
