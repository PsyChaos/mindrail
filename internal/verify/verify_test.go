package verify_test

import (
	"context"
	"database/sql"
	"errors"
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
	changes *changes.Service
	store   *changes.Store
	indexes *index.Store
	indexer *index.Indexer
	unit    index.ProjectUnit
	db      *sql.DB
	root    string
	pkg     string
	runner  git.CommandRunner
}

type failNthStagedShow struct {
	inner git.CommandRunner
	want  int
	seen  int
}

func (r *failNthStagedShow) Run(ctx context.Context, dir string, args ...string) ([]byte, []byte, error) {
	if len(args) == 2 && args[0] == "show" && len(args[1]) > 0 && args[1][0] == ':' {
		r.seen++
		if r.seen == r.want {
			return nil, []byte("injected staged read failure"), errors.New("injected failure")
		}
	}
	return r.inner.Run(ctx, dir, args...)
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
	unit, err := indexes.UpsertUnit(t.Context(), pkg, index.UnitPython)
	if err != nil {
		t.Fatal(err)
	}
	seedVerifyTasks(t, db.DB, "TSK-A", "TSK-B")
	return stagedFixture{service: service, changes: changeService, store: changeStore, indexes: indexes, indexer: indexer,
		unit: unit, db: db.DB, root: root, pkg: pkg, runner: git.NewExecRunner()}
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

func containsCode(codes []string, want string) bool {
	for _, code := range codes {
		if code == want {
			return true
		}
	}
	return false
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

// A reused verify operation is a materialized view of the current index,
// not an append-only history. Unstaging the only path must therefore clear
// both the verdict and the durable projection from the preceding run.
func TestVerifyStagedUnstageReplacesProjectionWithEmpty(t *testing.T) {
	fx := newStagedFixture(t)
	path := filepath.Join(fx.pkg, "a.py")
	if err := os.WriteFile(path, []byte("def a():\n    return 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitStage(t, fx.root, "add", "pkg/a.py")
	gitStage(t, fx.root, "commit", "--quiet", "-m", "base")
	if _, err := fx.store.CaptureBaseline(t.Context(), "TSK-A", []string{path}, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := fx.store.EnsureOpenChange(t.Context(), "TSK-A", ""); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("def a():\n    return 2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitStage(t, fx.root, "add", "pkg/a.py")
	if _, err := fx.service.VerifyStaged(t.Context(), "PRJ-1", fx.root, fx.runner); err != nil {
		t.Fatal(err)
	}
	gitStage(t, fx.root, "reset", "--quiet", "HEAD", "--", "pkg/a.py")

	verdict, err := fx.service.VerifyStaged(t.Context(), "PRJ-1", fx.root, fx.runner)
	if err != nil {
		t.Fatal(err)
	}
	if !verdict.Allow || len(verdict.Denials) != 0 {
		t.Fatalf("verdict = %+v, want empty-index ALLOW", verdict)
	}
	var files, symbols int
	if err := fx.db.QueryRowContext(t.Context(), `SELECT count(*) FROM change_files f
		JOIN changes c ON c.change_id=f.change_id WHERE c.operation_id='verify-staged-v2'`).Scan(&files); err != nil {
		t.Fatal(err)
	}
	if err := fx.db.QueryRowContext(t.Context(), `SELECT count(*) FROM change_symbols s
		JOIN changes c ON c.change_id=s.change_id WHERE c.operation_id='verify-staged-v2'`).Scan(&symbols); err != nil {
		t.Fatal(err)
	}
	if files != 0 || symbols != 0 {
		t.Fatalf("stale projection: files=%d symbols=%d, want 0/0", files, symbols)
	}
}

// Replacing A in the index with B must replace the fixed operation's stored
// projection exactly; rows from the earlier verification cannot survive.
func TestVerifyStagedReplacementContainsOnlyCurrentIndex(t *testing.T) {
	fx := newStagedFixture(t)
	a := filepath.Join(fx.pkg, "a.py")
	b := filepath.Join(fx.pkg, "b.py")
	if err := os.WriteFile(a, []byte("def a():\n    return 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(b, []byte("def b():\n    return 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitStage(t, fx.root, "add", "pkg/a.py", "pkg/b.py")
	gitStage(t, fx.root, "commit", "--quiet", "-m", "base")
	if _, err := fx.store.CaptureBaseline(t.Context(), "TSK-A", []string{a, b}, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := fx.store.EnsureOpenChange(t.Context(), "TSK-A", ""); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(a, []byte("def a():\n    return 2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitStage(t, fx.root, "add", "pkg/a.py")
	if _, err := fx.service.VerifyStaged(t.Context(), "PRJ-1", fx.root, fx.runner); err != nil {
		t.Fatal(err)
	}
	gitStage(t, fx.root, "reset", "--quiet", "HEAD", "--", "pkg/a.py")
	if err := os.WriteFile(b, []byte("def b():\n    return 2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitStage(t, fx.root, "add", "pkg/b.py")
	if _, err := fx.service.VerifyStaged(t.Context(), "PRJ-1", fx.root, fx.runner); err != nil {
		t.Fatal(err)
	}

	rows, err := fx.db.QueryContext(t.Context(), `SELECT f.path FROM change_files f
		JOIN changes c ON c.change_id=f.change_id WHERE c.operation_id='verify-staged-v2' ORDER BY f.path`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var got []string
	for rows.Next() {
		var path string
		if err := rows.Scan(&path); err != nil {
			t.Fatal(err)
		}
		got = append(got, path)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != b {
		t.Fatalf("projection = %v, want only %s", got, b)
	}
	var aSymbols, bSymbols int
	if err := fx.db.QueryRowContext(t.Context(), `SELECT count(*) FROM change_symbols s
		JOIN changes c ON c.change_id=s.change_id
		JOIN symbol_identities i ON i.symbol_uid=s.symbol_uid
		WHERE c.operation_id='verify-staged-v2' AND i.logical_key LIKE '%a.py%'`).Scan(&aSymbols); err != nil {
		t.Fatal(err)
	}
	if err := fx.db.QueryRowContext(t.Context(), `SELECT count(*) FROM change_symbols s
		JOIN changes c ON c.change_id=s.change_id
		JOIN symbol_identities i ON i.symbol_uid=s.symbol_uid
		WHERE c.operation_id='verify-staged-v2' AND i.logical_key LIKE '%b.py%'`).Scan(&bSymbols); err != nil {
		t.Fatal(err)
	}
	if aSymbols != 0 || bSymbols == 0 {
		t.Fatalf("symbol projection: a=%d b=%d, want 0 and nonzero", aSymbols, bSymbols)
	}
}

// A failed construction of the next snapshot must leave the last complete
// fixed-operation projection untouched. Scratch rows are never published.
func TestVerifyStagedFailedReplacementPreservesPreviousProjection(t *testing.T) {
	fx := newStagedFixture(t)
	a := filepath.Join(fx.pkg, "a.py")
	b := filepath.Join(fx.pkg, "b.py")
	for _, path := range []string{a, b} {
		if err := os.WriteFile(path, []byte("value = 1\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	gitStage(t, fx.root, "add", "pkg/a.py", "pkg/b.py")
	gitStage(t, fx.root, "commit", "--quiet", "-m", "base")
	if _, err := fx.store.CaptureBaseline(t.Context(), "TSK-A", []string{a, b}, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := fx.store.EnsureOpenChange(t.Context(), "TSK-A", ""); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(a, []byte("value = 2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitStage(t, fx.root, "add", "pkg/a.py")
	if _, err := fx.service.VerifyStaged(t.Context(), "PRJ-1", fx.root, fx.runner); err != nil {
		t.Fatal(err)
	}
	gitStage(t, fx.root, "reset", "--quiet", "HEAD", "--", "pkg/a.py")
	if err := os.WriteFile(b, []byte("value = 2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitStage(t, fx.root, "add", "pkg/b.py")

	failing := &failNthStagedShow{inner: fx.runner, want: 2}
	if _, err := fx.service.VerifyStaged(t.Context(), "PRJ-1", fx.root, failing); err == nil {
		t.Fatal("verify succeeded, want injected staged read failure")
	}
	var path string
	if err := fx.db.QueryRowContext(t.Context(), `SELECT f.path FROM change_files f
		JOIN changes c ON c.change_id=f.change_id WHERE c.operation_id='verify-staged-v2'`).Scan(&path); err != nil {
		t.Fatal(err)
	}
	if path != a {
		t.Fatalf("published projection = %s, want previous %s", path, a)
	}
	var scratch int
	if err := fx.db.QueryRowContext(t.Context(), `SELECT count(*) FROM changes
		WHERE task_id IS NULL AND operation_id IS NULL`).Scan(&scratch); err != nil {
		t.Fatal(err)
	}
	if scratch != 0 {
		t.Fatalf("scratch changes left behind = %d", scratch)
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

func TestVerifyStagedUnsupportedFileAmbiguityDenies(t *testing.T) {
	fx := newStagedFixture(t)
	shared := filepath.Join(fx.root, "README.md")
	if err := os.WriteFile(shared, []byte("docs\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, task := range []string{"TSK-A", "TSK-B"} {
		if _, err := fx.store.CaptureBaseline(t.Context(), task, []string{shared}, ""); err != nil {
			t.Fatal(err)
		}
	}
	gitStage(t, fx.root, "add", "README.md")
	verdict, err := fx.service.VerifyStaged(t.Context(), "PRJ-1", fx.root, fx.runner)
	if err != nil {
		t.Fatal(err)
	}
	if verdict.Allow {
		t.Fatalf("verdict = %+v, want ambiguous denial", verdict)
	}
	found := false
	for _, denial := range verdict.Denials {
		if denial.Code == app.CodeReconcileAmbiguous {
			found = true
		}
	}
	if !found {
		t.Fatalf("denials = %+v, want ambiguity", verdict.Denials)
	}
}

// Ownership cardinality is decided from durable scopes, not from the subset
// of owners that already have Change rows. Otherwise source attribution could
// allow a file that unsupported-file attribution correctly rejects.
func TestVerifyStagedOwnerWithoutChangeStillAmbiguous(t *testing.T) {
	for _, name := range []string{"source", "unsupported"} {
		t.Run(name, func(t *testing.T) {
			fx := newStagedFixture(t)
			rel, before, after := "pkg/a.py", "def a():\n    return 1\n", "def a():\n    return 2\n"
			if name == "unsupported" {
				rel, before, after = "README.md", "before\n", "after\n"
			}
			path := filepath.Join(fx.root, filepath.FromSlash(rel))
			if err := os.WriteFile(path, []byte(before), 0o644); err != nil {
				t.Fatal(err)
			}
			gitStage(t, fx.root, "add", rel)
			gitStage(t, fx.root, "commit", "--quiet", "-m", "base")
			for _, task := range []string{"TSK-A", "TSK-B"} {
				if _, err := fx.store.CaptureBaseline(t.Context(), task, []string{path}, ""); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := fx.store.EnsureOpenChange(t.Context(), "TSK-A", ""); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(after), 0o644); err != nil {
				t.Fatal(err)
			}
			gitStage(t, fx.root, "add", rel)

			verdict, err := fx.service.VerifyStaged(t.Context(), "PRJ-1", fx.root, fx.runner)
			if err != nil {
				t.Fatal(err)
			}
			if verdict.Allow || !containsCode(denialCodes(t, verdict), string(app.CodeReconcileAmbiguous)) {
				t.Fatalf("missing-change owner verdict = %+v, want ambiguous denial", verdict)
			}
		})
	}
}

func TestVerifyStagedSoleOwnerWithoutChangeIsAllowed(t *testing.T) {
	for _, name := range []string{"source", "unsupported"} {
		t.Run(name, func(t *testing.T) {
			fx := newStagedFixture(t)
			rel, before, after := "pkg/a.py", "def a():\n    return 1\n", "def a():\n    return 2\n"
			if name == "unsupported" {
				rel, before, after = "README.md", "before\n", "after\n"
			}
			path := filepath.Join(fx.root, filepath.FromSlash(rel))
			if err := os.WriteFile(path, []byte(before), 0o644); err != nil {
				t.Fatal(err)
			}
			gitStage(t, fx.root, "add", rel)
			gitStage(t, fx.root, "commit", "--quiet", "-m", "base")
			if _, err := fx.store.CaptureBaseline(t.Context(), "TSK-A", []string{path}, ""); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(after), 0o644); err != nil {
				t.Fatal(err)
			}
			gitStage(t, fx.root, "add", rel)

			verdict, err := fx.service.VerifyStaged(t.Context(), "PRJ-1", fx.root, fx.runner)
			if err != nil {
				t.Fatal(err)
			}
			if !verdict.Allow || len(verdict.Denials) != 0 {
				t.Fatalf("owner without change verdict = %+v, want allow", verdict)
			}
		})
	}
}

// Persisted task states are untrusted input because older databases do not
// constrain the column. Both source and unsupported paths must fail closed.
func TestVerifyStagedInvalidOwnerStateFailsClosed(t *testing.T) {
	for _, name := range []string{"source", "unsupported"} {
		t.Run(name, func(t *testing.T) {
			fx := newStagedFixture(t)
			rel, before, after := "pkg/a.py", "def a():\n    return 1\n", "def a():\n    return 2\n"
			if name == "unsupported" {
				rel, before, after = "README.md", "before\n", "after\n"
			}
			path := filepath.Join(fx.root, filepath.FromSlash(rel))
			if err := os.WriteFile(path, []byte(before), 0o644); err != nil {
				t.Fatal(err)
			}
			gitStage(t, fx.root, "add", rel)
			gitStage(t, fx.root, "commit", "--quiet", "-m", "base")
			if _, err := fx.store.CaptureBaseline(t.Context(), "TSK-A", []string{path}, ""); err != nil {
				t.Fatal(err)
			}
			if _, err := fx.store.EnsureOpenChange(t.Context(), "TSK-A", ""); err != nil {
				t.Fatal(err)
			}
			if _, err := fx.db.ExecContext(t.Context(), `UPDATE tasks SET state='CORRUPT' WHERE task_id='TSK-A'`); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(after), 0o644); err != nil {
				t.Fatal(err)
			}
			gitStage(t, fx.root, "add", rel)

			if _, err := fx.service.VerifyStaged(t.Context(), "PRJ-1", fx.root, fx.runner); err == nil {
				t.Fatal("invalid persisted task state was accepted")
			} else if payload, ok := app.PayloadOf(err); !ok || payload.Code != app.CodeIndexStateCorrupt {
				t.Fatalf("error = %v, want %s", err, app.CodeIndexStateCorrupt)
			}
		})
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
		t.Fatalf("verdict = %+v; codes = %+v, want guard denial", second, codes)
	}
}

// Guard mappings must be captured from baseline facts before staged indexing
// deletes a weakened test's reference. This is the real one-pass flow: no
// synthetic reference is inserted after reconciliation.
func TestVerifyStagedGuardCapturesReferenceBeforeIndexReplacement(t *testing.T) {
	fx := newStagedFixture(t)
	test := filepath.Join(fx.pkg, "test_a.py")
	base := "def helper():\n    return 1\n\ndef test_helper():\n    assert helper()\n"
	if err := os.WriteFile(test, []byte(base), 0o644); err != nil {
		t.Fatal(err)
	}
	gitStage(t, fx.root, "add", "pkg/test_a.py")
	gitStage(t, fx.root, "commit", "--quiet", "-m", "base")
	if _, err := fx.indexer.IndexFile(t.Context(), "PRJ-1", fx.unit, test); err != nil {
		t.Fatal(err)
	}
	var helperUID string
	if err := fx.db.QueryRowContext(t.Context(),
		`SELECT symbol_uid FROM symbols WHERE path = ? AND name = 'helper'`, test).Scan(&helperUID); err != nil {
		t.Fatal(err)
	}
	if err := fx.indexes.UpsertBinding(t.Context(), "INV-G", helperUID, index.BindingBound, ""); err != nil {
		t.Fatal(err)
	}
	writeGuardInvariant(t, fx.root)
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

	verdict, err := fx.service.VerifyStaged(t.Context(), "PRJ-1", fx.root, fx.runner)
	if err != nil {
		t.Fatal(err)
	}
	if verdict.Allow || !containsCode(denialCodes(t, verdict), "TEST_GUARD_WEAKENED") {
		t.Fatalf("verdict = %+v, want guard denial from pre-index reference", verdict)
	}
	again, err := fx.service.VerifyStaged(t.Context(), "PRJ-1", fx.root, fx.runner)
	if err != nil {
		t.Fatal(err)
	}
	if again.Allow || !containsCode(denialCodes(t, again), "TEST_GUARD_WEAKENED") {
		t.Fatalf("second verdict = %+v, want retry-stable guard denial", again)
	}
}

func TestVerifyStagedRenamedTestUsesBaselinePath(t *testing.T) {
	fx := newStagedFixture(t)
	oldTest := filepath.Join(fx.pkg, "old_test.py")
	newTest := filepath.Join(fx.pkg, "new_test.py")
	base := "def helper():\n    return 1\n\ndef test_helper():\n    assert helper()\n"
	if err := os.WriteFile(oldTest, []byte(base), 0o644); err != nil {
		t.Fatal(err)
	}
	gitStage(t, fx.root, "add", "pkg/old_test.py")
	gitStage(t, fx.root, "commit", "--quiet", "-m", "base")
	if _, err := fx.indexer.IndexFile(t.Context(), "PRJ-1", fx.unit, oldTest); err != nil {
		t.Fatal(err)
	}
	var helperUID string
	if err := fx.db.QueryRowContext(t.Context(),
		`SELECT symbol_uid FROM symbols WHERE path = ? AND name = 'helper'`, oldTest).Scan(&helperUID); err != nil {
		t.Fatal(err)
	}
	if err := fx.indexes.UpsertBinding(t.Context(), "INV-G", helperUID, index.BindingBound, ""); err != nil {
		t.Fatal(err)
	}
	writeGuardInvariant(t, fx.root)
	if _, err := fx.store.CaptureBaseline(t.Context(), "TSK-A", []string{newTest}, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := fx.store.EnsureOpenChange(t.Context(), "TSK-A", ""); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(oldTest, newTest); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(newTest, []byte("def helper():\n    return 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitStage(t, fx.root, "add", "-A")
	verdict, err := fx.service.VerifyStaged(t.Context(), "PRJ-1", fx.root, fx.runner)
	if err != nil {
		t.Fatal(err)
	}
	if verdict.Allow || !containsCode(denialCodes(t, verdict), "TEST_GUARD_WEAKENED") {
		t.Fatalf("renamed test verdict = %+v, want guard denial", verdict)
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
	writeGuardInvariant(t, fx.root)
}

func writeGuardInvariant(t *testing.T, root string) {
	t.Helper()
	dir := filepath.Join(root, ".mindrail", "knowledge", "invariants")
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

// TestVerifyStagedDriftDenies pins fail-closed staged ownership: a file not
// named by any active task scope is unregistered.
func TestVerifyStagedDriftDenies(t *testing.T) {
	fx := newStagedFixture(t)
	other := filepath.Join(fx.pkg, "b.py")
	if err := os.WriteFile(other, []byte("x = 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitStage(t, fx.root, "add", "pkg/b.py")
	if _, err := fx.store.CaptureBaseline(t.Context(), "TSK-A", []string{filepath.Join(fx.pkg, "a.py")}, ""); err != nil {
		t.Fatal(err)
	}

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
		if code == string(app.CodeUnregisteredChange) {
			found = true
		}
	}
	if !found {
		t.Fatalf("codes = %+v, want unregistered", codes)
	}
}

// Disjoint active task scopes can jointly own one staged commit. Each file is
// judged against its unique owner, never against every unrelated baseline.
func TestVerifyStagedDisjointTaskScopesDoNotCrossDrift(t *testing.T) {
	fx := newStagedFixture(t)
	a := filepath.Join(fx.pkg, "a.py")
	b := filepath.Join(fx.pkg, "b.py")
	if err := os.WriteFile(a, []byte("def a():\n    return 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(b, []byte("def b():\n    return 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitStage(t, fx.root, "add", "pkg/a.py", "pkg/b.py")
	gitStage(t, fx.root, "commit", "--quiet", "-m", "base")
	for task, path := range map[string]string{"TSK-A": a, "TSK-B": b} {
		if _, err := fx.store.CaptureBaseline(t.Context(), task, []string{path}, ""); err != nil {
			t.Fatal(err)
		}
		if _, err := fx.store.EnsureOpenChange(t.Context(), task, ""); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(a, []byte("def a():\n    return 2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(b, []byte("def b():\n    return 2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitStage(t, fx.root, "add", "pkg/a.py", "pkg/b.py")

	verdict, err := fx.service.VerifyStaged(t.Context(), "PRJ-1", fx.root, fx.runner)
	if err != nil {
		t.Fatal(err)
	}
	if !verdict.Allow || len(verdict.Denials) != 0 {
		t.Fatalf("verdict = %+v, want disjoint scopes to allow", verdict)
	}
}

func TestVerifyStagedCompletedHistoricalScopeDoesNotCompete(t *testing.T) {
	fx := newStagedFixture(t)
	path := filepath.Join(fx.pkg, "a.py")
	if err := os.WriteFile(path, []byte("def a():\n    return 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitStage(t, fx.root, "add", "pkg/a.py")
	gitStage(t, fx.root, "commit", "--quiet", "-m", "base")
	for _, task := range []string{"TSK-A", "TSK-B"} {
		if _, err := fx.store.CaptureBaseline(t.Context(), task, []string{path}, ""); err != nil {
			t.Fatal(err)
		}
		if _, err := fx.store.EnsureOpenChange(t.Context(), task, ""); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := fx.db.ExecContext(t.Context(), `UPDATE tasks SET updated_at=CASE task_id
		WHEN 'TSK-A' THEN '2026-09-23T10:01:00Z'
		WHEN 'TSK-B' THEN '2026-09-23T10:03:00Z'
		END, state=CASE task_id WHEN 'TSK-B' THEN 'COMPLETED' ELSE state END
		WHERE task_id IN ('TSK-A', 'TSK-B')`); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("def a():\n    return 2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitStage(t, fx.root, "add", "pkg/a.py")

	verdict, err := fx.service.VerifyStaged(t.Context(), "PRJ-1", fx.root, fx.runner)
	if err != nil {
		t.Fatal(err)
	}
	if !verdict.Allow || len(verdict.Denials) != 0 {
		t.Fatalf("terminal baseline competed: %+v", verdict)
	}
}

// Sequential delegated tasks may all retain the same durable file scope after
// completion. The task completed most recently owns the staged file; older
// historical scopes must not make the release gate ambiguous.
func TestVerifyStagedLatestCompletedOwnerMayCommit(t *testing.T) {
	for _, name := range []string{"source", "unsupported"} {
		t.Run(name, func(t *testing.T) {
			fx := newStagedFixture(t)
			rel, before, after := "pkg/a.py", "def a():\n    return 1\n", "def a():\n    return 2\n"
			if name == "unsupported" {
				rel, before, after = "pkg/note.md", "before\n", "after\n"
			}
			path := filepath.Join(fx.root, filepath.FromSlash(rel))
			if err := os.WriteFile(path, []byte(before), 0o644); err != nil {
				t.Fatal(err)
			}
			gitStage(t, fx.root, "add", rel)
			gitStage(t, fx.root, "commit", "--quiet", "-m", "base")
			for _, task := range []string{"TSK-A", "TSK-B"} {
				if _, err := fx.store.CaptureBaseline(t.Context(), task, []string{path}, ""); err != nil {
					t.Fatal(err)
				}
				if _, err := fx.store.EnsureOpenChange(t.Context(), task, ""); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := fx.db.ExecContext(t.Context(), `UPDATE tasks SET state='COMPLETED', updated_at=CASE task_id
				WHEN 'TSK-A' THEN '2026-09-23T10:01:00Z'
				WHEN 'TSK-B' THEN '2026-09-23T10:02:00Z'
				END WHERE task_id IN ('TSK-A', 'TSK-B')`); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(after), 0o644); err != nil {
				t.Fatal(err)
			}
			gitStage(t, fx.root, "add", rel)

			verdict, err := fx.service.VerifyStaged(t.Context(), "PRJ-1", fx.root, fx.runner)
			if err != nil {
				t.Fatal(err)
			}
			if !verdict.Allow || len(verdict.Denials) != 0 {
				t.Fatalf("latest completed owner verdict = %+v, want ALLOW", verdict)
			}
		})
	}
}

func TestVerifyStagedLatestCompletedOwnerTieDenies(t *testing.T) {
	for _, name := range []string{"source", "unsupported"} {
		t.Run(name, func(t *testing.T) {
			fx := newStagedFixture(t)
			rel, before, after := "pkg/a.py", "def a():\n    return 1\n", "def a():\n    return 2\n"
			if name == "unsupported" {
				rel, before, after = "pkg/note.md", "before\n", "after\n"
			}
			path := filepath.Join(fx.root, filepath.FromSlash(rel))
			if err := os.WriteFile(path, []byte(before), 0o644); err != nil {
				t.Fatal(err)
			}
			gitStage(t, fx.root, "add", rel)
			gitStage(t, fx.root, "commit", "--quiet", "-m", "base")
			for _, task := range []string{"TSK-A", "TSK-B"} {
				if _, err := fx.store.CaptureBaseline(t.Context(), task, []string{path}, ""); err != nil {
					t.Fatal(err)
				}
				if _, err := fx.store.EnsureOpenChange(t.Context(), task, ""); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := fx.db.ExecContext(t.Context(), `UPDATE tasks
				SET state='COMPLETED', updated_at='2026-09-23T10:02:00Z'
				WHERE task_id IN ('TSK-A', 'TSK-B')`); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(after), 0o644); err != nil {
				t.Fatal(err)
			}
			gitStage(t, fx.root, "add", rel)

			verdict, err := fx.service.VerifyStaged(t.Context(), "PRJ-1", fx.root, fx.runner)
			if err != nil {
				t.Fatal(err)
			}
			if verdict.Allow || !containsCode(denialCodes(t, verdict), string(app.CodeReconcileAmbiguous)) {
				t.Fatalf("tied completed owners verdict = %+v, want ambiguous denial", verdict)
			}
		})
	}
}

// Completion normally precedes the Git commit. A sole completed owner must
// therefore retain both symbol-level and unsupported-file ownership.
func TestVerifyStagedCompletedSoleOwnerMayCommit(t *testing.T) {
	for _, name := range []string{"source", "unsupported"} {
		t.Run(name, func(t *testing.T) {
			fx := newStagedFixture(t)
			rel := "pkg/a.py"
			before := "def a():\n    return 1\n"
			after := "def a():\n    return 2\n"
			if name == "unsupported" {
				rel, before, after = "pkg/note.md", "before\n", "after\n"
			}
			path := filepath.Join(fx.root, filepath.FromSlash(rel))
			if err := os.WriteFile(path, []byte(before), 0o644); err != nil {
				t.Fatal(err)
			}
			gitStage(t, fx.root, "add", rel)
			gitStage(t, fx.root, "commit", "--quiet", "-m", "base")
			if _, err := fx.store.CaptureBaseline(t.Context(), "TSK-A", []string{path}, ""); err != nil {
				t.Fatal(err)
			}
			if _, err := fx.store.EnsureOpenChange(t.Context(), "TSK-A", ""); err != nil {
				t.Fatal(err)
			}
			if _, err := fx.db.ExecContext(t.Context(), `UPDATE tasks SET state='COMPLETED' WHERE task_id='TSK-A'`); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(after), 0o644); err != nil {
				t.Fatal(err)
			}
			gitStage(t, fx.root, "add", rel)
			verdict, err := fx.service.VerifyStaged(t.Context(), "PRJ-1", fx.root, fx.runner)
			if err != nil {
				t.Fatal(err)
			}
			if !verdict.Allow || len(verdict.Denials) != 0 {
				t.Fatalf("completed owner verdict = %+v", verdict)
			}
		})
	}
}

// Abandonment discards ownership. Its stale baseline must not authorize a
// later commit, for parsable source or file-only unsupported content.
func TestVerifyStagedAbandonedSoleOwnerDenies(t *testing.T) {
	for _, name := range []string{"source", "unsupported"} {
		t.Run(name, func(t *testing.T) {
			fx := newStagedFixture(t)
			rel, before, after := "pkg/a.py", "def a():\n    return 1\n", "def a():\n    return 2\n"
			if name == "unsupported" {
				rel, before, after = "pkg/note.md", "before\n", "after\n"
			}
			path := filepath.Join(fx.root, filepath.FromSlash(rel))
			if err := os.WriteFile(path, []byte(before), 0o644); err != nil {
				t.Fatal(err)
			}
			gitStage(t, fx.root, "add", rel)
			gitStage(t, fx.root, "commit", "--quiet", "-m", "base")
			if _, err := fx.store.CaptureBaseline(t.Context(), "TSK-A", []string{path}, ""); err != nil {
				t.Fatal(err)
			}
			if _, err := fx.store.EnsureOpenChange(t.Context(), "TSK-A", ""); err != nil {
				t.Fatal(err)
			}
			if _, err := fx.db.ExecContext(t.Context(), `UPDATE tasks SET state='ABANDONED' WHERE task_id='TSK-A'`); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(after), 0o644); err != nil {
				t.Fatal(err)
			}
			gitStage(t, fx.root, "add", rel)
			verdict, err := fx.service.VerifyStaged(t.Context(), "PRJ-1", fx.root, fx.runner)
			if err != nil {
				t.Fatal(err)
			}
			if verdict.Allow || !containsCode(denialCodes(t, verdict), string(app.CodeUnregisteredChange)) {
				t.Fatalf("abandoned owner verdict = %+v, want unregistered denial", verdict)
			}
		})
	}
}

// A rename qualifies delta keys at the destination, but removed identities
// still resolve through their original HEAD-qualified keys.
func TestVerifyStagedRenameRemovalPreservesUID(t *testing.T) {
	fx := newStagedFixture(t)
	oldPath := filepath.Join(fx.pkg, "old.py")
	newPath := filepath.Join(fx.pkg, "new.py")
	base := "def keep():\n    return 1\n\ndef removed():\n    return 2\n"
	if err := os.WriteFile(oldPath, []byte(base), 0o644); err != nil {
		t.Fatal(err)
	}
	gitStage(t, fx.root, "add", "pkg/old.py")
	gitStage(t, fx.root, "commit", "--quiet", "-m", "base")
	if _, err := fx.indexer.IndexFile(t.Context(), "PRJ-1", fx.unit, oldPath); err != nil {
		t.Fatal(err)
	}
	var removedUID string
	if err := fx.db.QueryRowContext(t.Context(),
		`SELECT symbol_uid FROM symbols WHERE path = ? AND name = 'removed'`, oldPath).Scan(&removedUID); err != nil {
		t.Fatal(err)
	}
	if _, err := fx.store.CaptureBaseline(t.Context(), "TSK-A", []string{newPath}, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := fx.store.EnsureOpenChange(t.Context(), "TSK-A", ""); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(oldPath, newPath); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(newPath, []byte("def keep():\n    return 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitStage(t, fx.root, "add", "-A")
	if _, err := fx.service.VerifyStaged(t.Context(), "PRJ-1", fx.root, fx.runner); err != nil {
		t.Fatal(err)
	}
	var preserved int
	if err := fx.db.QueryRowContext(t.Context(), `SELECT count(*) FROM change_symbols s
		JOIN changes c ON c.change_id=s.change_id
		WHERE c.operation_id='verify-staged-v2' AND s.kind='removed' AND s.symbol_uid=?`, removedUID).Scan(&preserved); err != nil {
		t.Fatal(err)
	}
	if preserved != 1 {
		t.Fatalf("removed uid rows = %d, want 1 for %s", preserved, removedUID)
	}
}

// A failed run may index its first file before a later parse fails. Retrying
// the same staged set must still publish both HEAD-to-index symbol deltas.
func TestVerifyStagedRetryAfterPartialIndexingKeepsEverySymbolDelta(t *testing.T) {
	fx := newStagedFixture(t)
	a := filepath.Join(fx.pkg, "a.py")
	b := filepath.Join(fx.pkg, "b.py")
	for path, body := range map[string]string{
		a: "def a():\n    return 1\n",
		b: "def b():\n    return 1\n",
	} {
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	gitStage(t, fx.root, "add", "pkg/a.py", "pkg/b.py")
	gitStage(t, fx.root, "commit", "--quiet", "-m", "base")
	if _, err := fx.store.CaptureBaseline(t.Context(), "TSK-A", []string{a, b}, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := fx.store.EnsureOpenChange(t.Context(), "TSK-A", ""); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(a, []byte("def a():\n    return 2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(b, []byte("def b(\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitStage(t, fx.root, "add", "pkg/a.py", "pkg/b.py")
	if _, err := fx.service.VerifyStaged(t.Context(), "PRJ-1", fx.root, fx.runner); err == nil {
		t.Fatal("invalid second file did not fail after the first file was indexed")
	}
	if err := os.WriteFile(b, []byte("def b():\n    return 2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitStage(t, fx.root, "add", "pkg/b.py")
	verdict, err := fx.service.VerifyStaged(t.Context(), "PRJ-1", fx.root, fx.runner)
	if err != nil {
		t.Fatal(err)
	}
	if !verdict.Allow {
		t.Fatalf("retry verdict = %+v", verdict)
	}
	var changed int
	if err := fx.db.QueryRowContext(t.Context(), `SELECT count(*) FROM change_symbols s
		JOIN changes c ON c.change_id=s.change_id
		WHERE c.operation_id='verify-staged-v2' AND s.kind='modified' AND s.body_changed=1`).Scan(&changed); err != nil {
		t.Fatal(err)
	}
	if changed != 2 {
		t.Fatalf("published modified body deltas = %d, want 2", changed)
	}
}

func TestReconcileStagedSnapshotRejectsPathSetChange(t *testing.T) {
	fx := newStagedFixture(t)
	a := filepath.Join(fx.pkg, "a.py")
	b := filepath.Join(fx.pkg, "b.py")
	if err := os.WriteFile(a, []byte("def a():\n    return 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitStage(t, fx.root, "add", "pkg/a.py")
	gitStage(t, fx.root, "commit", "--quiet", "-m", "base")
	if err := os.WriteFile(a, []byte("def a():\n    return 2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitStage(t, fx.root, "add", "pkg/a.py")
	entries, err := fx.store.DiscoverFilesStaged(t.Context(), fx.runner, fx.root)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(b, []byte("def b():\n    return 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitStage(t, fx.root, "add", "pkg/b.py")
	if _, err := fx.changes.ReconcileStagedSnapshot(t.Context(), "PRJ-1", fx.root, "", "verify-staged-v2", entries, fx.runner); err == nil {
		t.Fatal("snapshot reconcile accepted a newly staged path")
	}
	var published int
	if err := fx.db.QueryRowContext(t.Context(), `SELECT count(*) FROM change_files f
		JOIN changes c ON c.change_id=f.change_id WHERE c.operation_id='verify-staged-v2'`).Scan(&published); err != nil {
		t.Fatal(err)
	}
	if published != 0 {
		t.Fatalf("published rows = %d, want 0 after snapshot mismatch", published)
	}
}

// TestVerifyStagedUnbornHead pins the no-commit edge: without HEAD, staged
// files judge against empty before-bytes instead of git errors.
func TestVerifyStagedUnbornHead(t *testing.T) {
	fx := newStagedFixture(t)
	test := filepath.Join(fx.pkg, "test_a.py")
	if err := os.WriteFile(test, []byte("def test_a():\n    assert x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitStage(t, fx.root, "add", "pkg/test_a.py")

	verdict, err := fx.service.VerifyStaged(t.Context(), "PRJ-1", fx.root, fx.runner)
	if err != nil {
		t.Fatal(err)
	}
	_ = verdict
}
