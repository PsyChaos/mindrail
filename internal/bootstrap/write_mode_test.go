package bootstrap_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/PsyChaos/mindrail/internal/bootstrap"
	"github.com/PsyChaos/mindrail/internal/storage"
)

// TestWriteModeCreatesNothing is finding F40.
//
// ModeWrite exists to separate two things decision D-01 had folded together:
// opening the database for writing, which the coordination commands need, and
// bringing a database into existence, which only `mindrail init` may do. The
// mode's own comment states the second half — "it opens for writing and
// creates, migrates and registers nothing" — and until this test nothing
// asserted it. `creates()` returning `m == ModeInit || m == ModeWrite` left the
// whole suite green, and would have given six commands the authority D-01
// reserves for one.
//
// The three promises are asserted separately because they are three different
// mechanisms: a gate on opening, a gate on the migrator and a branch in step 7.
func TestWriteModeCreatesNothing(t *testing.T) {
	repo := newGitRepo(t)

	// The runtime directory exists and the database does not, which is the one
	// arrangement where "creates" and "does not create" are distinguishable:
	// with no directory the open would fail either way.
	runtimeRoot := filepath.Join(repo, ".git", "mindrail")
	if err := os.MkdirAll(runtimeRoot, 0o700); err != nil {
		t.Fatalf("create %s: %v", runtimeRoot, err)
	}

	application := bootstrap.New(options(t, repo, bootstrap.ModeWrite, nil))
	t.Cleanup(func() { _ = application.Shutdown(context.Background()) })

	// A halted start is not a failure of this test: the point is what is on
	// disk afterwards, and the mode is allowed to refuse a repository with no
	// database.
	_ = application.Start(t.Context())

	paths := application.Paths()
	if paths.DBPath == "" {
		t.Fatal("startup derived no database path, so the assertion below would be vacuous")
	}
	// Not assertNoRuntimeState: that helper also fails on the runtime directory,
	// which this fixture created on purpose. The database file is the whole
	// claim here.
	for _, suffix := range []string{"", "-wal", "-shm"} {
		if _, err := os.Stat(paths.DBPath + suffix); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("os.Stat(%s) = %v after a write-mode start; ModeWrite creates nothing",
				paths.DBPath+suffix, err)
		}
	}
}

// TestWriteModeMigratesNothingAndRegistersNothing is the other two promises, on
// a repository that does have a database.
//
// A write-mode start against a database one migration behind must leave the
// ledger exactly where it found it, and must not register a worktree that is
// not registered. Both are what `mindrail init` is for, and a coordination
// command that quietly did either would upgrade a user's database as a side
// effect of reading a task list.
func TestWriteModeMigratesNothingAndRegistersNothing(t *testing.T) {
	repo := newGitRepo(t)

	// Initialise properly first, then take the database back one migration and
	// remove the registration. A mode that migrates would re-apply the missing
	// migration; a mode that registers would put the row back.
	initialise := bootstrap.New(options(t, repo, bootstrap.ModeInit, nil))
	if err := initialise.Start(t.Context()); err != nil {
		t.Fatalf("init start: %v", err)
	}
	dbPath := initialise.Paths().DBPath
	if initialise.Subject().Workspace.ID == "" {
		t.Fatal("init registered no workspace, so the fixture is not what this test needs")
	}
	if err := initialise.Shutdown(context.Background()); err != nil {
		t.Fatalf("init shutdown: %v", err)
	}

	before := countRows(t, dbPath, "schema_migrations")
	if before < 2 {
		t.Fatalf("init applied %d migrations; this fixture needs at least two", before)
	}
	takeTheSchemaBackOneMigration(t, dbPath)
	unregisterEveryWorktree(t, dbPath)

	application := bootstrap.New(options(t, repo, bootstrap.ModeWrite, nil))
	t.Cleanup(func() { _ = application.Shutdown(context.Background()) })
	_ = application.Start(t.Context())

	if applied := countRows(t, dbPath, "schema_migrations"); applied != before-1 {
		t.Errorf("the ledger holds %d migrations after a write-mode start, want %d; ModeWrite migrates nothing",
			applied, before-1)
	}
	if rows := countRows(t, dbPath, "workspaces"); rows != 0 {
		t.Errorf("a write-mode start registered %d worktree(s); ModeWrite registers nothing", rows)
	}
	if id := application.Subject().Workspace.ID; id != "" {
		t.Errorf("the subject carries workspace %s after a start that registered nothing", id)
	}
}

// takeTheSchemaBackOneMigration removes everything the last migration created,
// its ledger row included.
func takeTheSchemaBackOneMigration(t *testing.T, dbPath string) {
	t.Helper()

	execOnDB(t, dbPath,
		`DROP INDEX IF EXISTS idx_checkpoints_task`,
		`DROP INDEX IF EXISTS idx_tasks_project`,
		`DROP TABLE IF EXISTS checkpoints`,
		`DROP TABLE IF EXISTS tasks`,
		`DROP TABLE IF EXISTS sessions`,
		`DELETE FROM schema_migrations WHERE version = (SELECT max(version) FROM schema_migrations)`)
}

// unregisterEveryWorktree empties the workspaces table without dropping it, so
// that a mode which registers would be visible as a row rather than as an error.
func unregisterEveryWorktree(t *testing.T, dbPath string) {
	t.Helper()
	execOnDB(t, dbPath, `DELETE FROM workspaces`)
}

func execOnDB(t *testing.T, dbPath string, statements ...string) {
	t.Helper()

	db, err := storage.Open(t.Context(), storage.Options{Path: dbPath})
	if err != nil {
		t.Fatalf("open %s: %v", dbPath, err)
	}
	defer func() { _ = db.Close() }()

	for _, statement := range statements {
		if _, err := db.ExecContext(t.Context(), statement); err != nil {
			t.Fatalf("%s: %v", statement, err)
		}
	}
}

func countRows(t *testing.T, dbPath, table string) int {
	t.Helper()

	db, err := storage.Open(t.Context(), storage.Options{Path: dbPath, ReadOnly: true})
	if err != nil {
		t.Fatalf("open %s: %v", dbPath, err)
	}
	defer func() { _ = db.Close() }()

	// The table name is a constant at every call site and there is no user
	// input on this path.
	var count int
	if err := db.QueryRowContext(t.Context(), `SELECT count(*) FROM `+table).Scan(&count); err != nil {
		t.Fatalf("counting %s: %v", table, err)
	}
	return count
}
