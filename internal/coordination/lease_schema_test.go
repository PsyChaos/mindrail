package coordination_test

import (
	"strings"
	"testing"

	"github.com/PsyChaos/mindrail/internal/storage"
)

// TestTheDatabaseRefusesASecondActiveLeaseOnOneTarget is MR-004's AC-02.1: the
// partial unique index of migration 000003 is the guard, and it holds with no
// Go in between — a second unreleased row for one target in one project is
// refused by SQLite itself, and the same row inserts once the first is
// released. The store's own refusal by name (LEASE_CONFLICT) sits in front of
// this; the index is what holds for a writer that never ran the read.
func TestTheDatabaseRefusesASecondActiveLeaseOnOneTarget(t *testing.T) {
	f := newFixture(t)
	session := f.session(t)

	const insert = `INSERT INTO leases (lease_id, project_id, target_kind, target_key, holder,
		acquired_at, renewed_at, expires_at, released_at, release_reason)
		VALUES (?, ?, 'file', 'src/auth.go', ?, 't0', 't0', 't1', NULL, NULL)`

	if _, err := f.db.ExecContext(t.Context(), insert, "LSE-1", f.projectID, session.ID); err != nil {
		t.Fatalf("first active lease = %v, want no error", err)
	}

	_, err := f.db.ExecContext(t.Context(), insert, "LSE-2", f.projectID, session.ID)
	if err == nil {
		t.Fatal("second active lease on the same target = nil, want the unique index to refuse it")
	}
	if !strings.Contains(err.Error(), "UNIQUE constraint failed") {
		t.Fatalf("second active lease = %v, want a UNIQUE constraint failure from idx_leases_active", err)
	}
	// A refusal that is not contention or a permission: nothing the storage
	// layer names, so a caller's own diagnosis is the one that would reach the
	// wire.
	if storage.IsBusy(err) || storage.IsReadOnly(err) || storage.IsDiskFull(err) {
		t.Errorf("the constraint failure %v is classified as a storage condition", err)
	}

	// Released rows do not count, and an expired-but-unreleased row still does:
	// the index is on released_at alone (decision D-65).
	if _, err := f.db.ExecContext(t.Context(),
		`UPDATE leases SET released_at = 't2', release_reason = 'released' WHERE lease_id = 'LSE-1'`); err != nil {
		t.Fatalf("release = %v, want no error", err)
	}
	if _, err := f.db.ExecContext(t.Context(), insert, "LSE-2", f.projectID, session.ID); err != nil {
		t.Fatalf("active lease after the first was released = %v, want no error", err)
	}

	var unreleased int
	if err := f.db.QueryRowContext(t.Context(),
		`SELECT count(*) FROM leases WHERE target_kind = 'file' AND target_key = 'src/auth.go' AND released_at IS NULL`).
		Scan(&unreleased); err != nil {
		t.Fatalf("count = %v, want no error", err)
	}
	if unreleased != 1 {
		t.Errorf("unreleased rows for the target = %d, want exactly 1", unreleased)
	}

	// The same key in another project is another target: the index is
	// project-scoped, as leases are (spec §61).
	if _, err := f.db.ExecContext(t.Context(),
		`INSERT INTO projects (project_id, common_dir, registered_at) VALUES ('PRJ-other', '/elsewhere/.git', 't0')`); err != nil {
		t.Fatalf("second project = %v, want no error", err)
	}
	if _, err := f.db.ExecContext(t.Context(), insert, "LSE-3", "PRJ-other", session.ID); err != nil {
		t.Errorf("the same path in another project = %v, want no error: the index is per project", err)
	}
}

// TestMigrationThreeLeavesTheObjectsTheDesignNames pins design §5's objects
// by name in sqlite_master, and the added column by name in the live table —
// the shape the ledger verifies on every start, read here directly so the two
// cannot drift.
func TestMigrationThreeLeavesTheObjectsTheDesignNames(t *testing.T) {
	f := newFixture(t)

	for _, object := range []struct{ kind, name string }{
		{"table", "leases"},
		{"table", "operations"},
		{"index", "idx_leases_active"},
		{"index", "idx_leases_holder"},
	} {
		var found string
		err := f.db.QueryRowContext(t.Context(),
			`SELECT name FROM sqlite_master WHERE type = ? AND name = ?`, object.kind, object.name).Scan(&found)
		if err != nil {
			t.Errorf("%s %s is not in the migrated database: %v", object.kind, object.name, err)
		}
	}

	var strict int
	if err := f.db.QueryRowContext(t.Context(),
		`SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name IN ('leases', 'operations') AND sql LIKE '%) STRICT'`).
		Scan(&strict); err != nil {
		t.Fatalf("STRICT count = %v, want no error", err)
	}
	if strict != 2 {
		t.Errorf("%d of the two new tables are STRICT, want both", strict)
	}

	var revision int
	if err := f.db.QueryRowContext(t.Context(),
		`SELECT count(*) FROM pragma_table_xinfo('tasks') WHERE name = 'revision'`).Scan(&revision); err != nil {
		t.Fatalf("pragma_table_xinfo = %v, want no error", err)
	}
	if revision != 1 {
		t.Errorf("tasks has %d columns named revision, want 1", revision)
	}

	// A task written through the store starts at revision 1 (AC-05.5's first
	// half, checkable before Transition learns to move it).
	task := f.task(t, f.session(t).ID, "a task at the first revision")
	var stored int64
	if err := f.db.QueryRowContext(t.Context(), `SELECT revision FROM tasks WHERE task_id = ?`, task.ID).Scan(&stored); err != nil {
		t.Fatalf("SELECT revision = %v, want no error", err)
	}
	if stored != 1 {
		t.Errorf("a newly opened task is at revision %d, want 1", stored)
	}
}
