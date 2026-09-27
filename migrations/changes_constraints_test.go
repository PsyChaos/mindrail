package migrations_test

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/migration"
	"github.com/PsyChaos/mindrail/internal/storage"
	"github.com/PsyChaos/mindrail/migrations"
)

func migratedChangesSchema(t *testing.T) *sql.DB {
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
	if _, err := migration.New(db.DB, set, app.SystemClock{}).Up(t.Context()); err != nil {
		t.Fatal(err)
	}
	return db.DB
}

func mustChangesSQL(t *testing.T, db *sql.DB, statement string, args ...any) {
	t.Helper()
	if _, err := db.ExecContext(t.Context(), statement, args...); err != nil {
		t.Fatalf("%s: %v", statement, err)
	}
}

// TestOneOpenChangePerTask pins the D-115 grain: a second row for the same
// task fails, while NULL-task retroactive rows stay unlimited.
func seedTaskChain(t *testing.T, db *sql.DB) {
	t.Helper()
	mustChangesSQL(t, db, `INSERT INTO projects (project_id, common_dir, registered_at)
		VALUES ('PRJ-1', '/repo/.git', '2026-09-23T10:00:00Z')`)
	mustChangesSQL(t, db, `INSERT INTO workspaces (workspace_id, project_id, root_path, git_dir, is_linked_worktree, registered_at, last_seen_at)
		VALUES ('WS-1', 'PRJ-1', '/repo', '/repo/.git', 0, '2026-09-23T10:00:00Z', '2026-09-23T10:00:00Z')`)
	mustChangesSQL(t, db, `INSERT INTO sessions (session_id, workspace_id, label, started_at)
		VALUES ('SES-1', 'WS-1', 's', '2026-09-23T10:00:00Z')`)
	mustChangesSQL(t, db, `INSERT INTO tasks (task_id, project_id, title, state, opened_by, created_at, updated_at)
		VALUES ('TSK-1', 'PRJ-1', 't', 'OPEN', 'SES-1', '2026-09-23T10:00:00Z', '2026-09-23T10:00:00Z')`)
}

func TestOneOpenChangePerTask(t *testing.T) {
	db := migratedChangesSchema(t)
	seedTaskChain(t, db)
	mustChangesSQL(t, db, `INSERT INTO changes (change_id, task_id, created_at, updated_at)
		VALUES ('CHG-AAAAAAAAAAAAAAAAAAAAAAAAAA', 'TSK-1', '2026-09-23T10:00:00Z', '2026-09-23T10:00:00Z')`)
	if _, err := db.ExecContext(t.Context(), `INSERT INTO changes (change_id, task_id, created_at, updated_at)
		VALUES ('CHG-BBBBBBBBBBBBBBBBBBBBBBBBBB', 'TSK-1', '2026-09-23T10:00:00Z', '2026-09-23T10:00:00Z')`); err == nil {
		t.Fatal("second change row for one task accepted")
	}
	mustChangesSQL(t, db, `INSERT INTO changes (change_id, created_at, updated_at)
		VALUES ('CHG-CCCCCCCCCCCCCCCCCCCCCCCCCC', '2026-09-23T10:00:00Z', '2026-09-23T10:00:00Z')`)
	mustChangesSQL(t, db, `INSERT INTO changes (change_id, created_at, updated_at)
		VALUES ('CHG-DDDDDDDDDDDDDDDDDDDDDDDDDD', '2026-09-23T10:00:00Z', '2026-09-23T10:00:00Z')`)
}

// TestChangeRowsEnforceForeignKeys pins the join posture: changes name
// real tasks, files and symbols name real changes and identities.
func TestChangeRowsEnforceForeignKeys(t *testing.T) {
	db := migratedChangesSchema(t)
	seedTaskChain(t, db)
	if _, err := db.ExecContext(t.Context(), `INSERT INTO changes (change_id, task_id, created_at, updated_at)
		VALUES ('CHG-X', 'TSK-NOPE', '2026-09-23T10:00:00Z', '2026-09-23T10:00:00Z')`); err == nil {
		t.Error("change with missing task accepted")
	}
	if _, err := db.ExecContext(t.Context(), `INSERT INTO change_files (change_id, path, kind, discovered_via)
		VALUES ('CHG-NOPE', '/r/a.py', 'modified', 'reconcile')`); err == nil {
		t.Error("file row with missing change accepted")
	}
	if _, err := db.ExecContext(t.Context(), `INSERT INTO change_symbols (change_id, logical_key, kind, discovered_via, symbol_uid)
		VALUES ('CHG-NOPE', 'k', 'added', 'reconcile', 'SYM-NOPE')`); err == nil {
		t.Error("symbol row with missing change accepted")
	}
	if _, err := db.ExecContext(t.Context(), `INSERT INTO scope_attributions (logical_key, change_id, decided_by, decided_at)
		VALUES ('k', 'CHG-NOPE', 'SES-1', '2026-09-23T10:00:00Z')`); err == nil {
		t.Error("attribution with missing change accepted")
	}
}

// TestChangeSymbolGrainIsPerKey pins the (change_id, logical_key) natural
// key: one row per symbol per change, so redelivery converges instead of
// duplicating.
func TestChangeSymbolGrainIsPerKey(t *testing.T) {
	db := migratedChangesSchema(t)
	seedTaskChain(t, db)
	mustChangesSQL(t, db, `INSERT INTO changes (change_id, created_at, updated_at)
		VALUES ('CHG-AAAAAAAAAAAAAAAAAAAAAAAAAA', '2026-09-23T10:00:00Z', '2026-09-23T10:00:00Z')`)
	mustChangesSQL(t, db, `INSERT INTO change_symbols (change_id, logical_key, kind, discovered_via)
		VALUES ('CHG-AAAAAAAAAAAAAAAAAAAAAAAAAA', 'k', 'added', 'reconcile')`)
	// A different key under the same change is a different row: the grain is
	// per key, not per change.
	mustChangesSQL(t, db, `INSERT INTO change_symbols (change_id, logical_key, kind, discovered_via)
		VALUES ('CHG-AAAAAAAAAAAAAAAAAAAAAAAAAA', 'k2', 'added', 'reconcile')`)
	if _, err := db.ExecContext(t.Context(), `INSERT INTO change_symbols (change_id, logical_key, kind, discovered_via)
		VALUES ('CHG-AAAAAAAAAAAAAAAAAAAAAAAAAA', 'k', 'modified', 'baseline')`); err == nil {
		t.Fatal("duplicate symbol row for one change accepted")
	}
}

// TestBaselineGrainIsPerTaskPath pins the (task_id, path) baseline grain:
// one hash per scope file, replaced wholesale, never stacked.
func TestBaselineGrainIsPerTaskPath(t *testing.T) {
	db := migratedChangesSchema(t)
	mustChangesSQL(t, db, `INSERT INTO change_baselines (task_id, path, content_hash, captured_at)
		VALUES ('TSK-1', '/r/a.py', 'h1', '2026-09-23T10:00:00Z')`)
	mustChangesSQL(t, db, `INSERT INTO change_baselines (task_id, path, content_hash, captured_at)
		VALUES ('TSK-1', '/r/b.py', 'h2', '2026-09-23T10:00:00Z')`)
	if _, err := db.ExecContext(t.Context(), `INSERT INTO change_baselines (task_id, path, content_hash, captured_at)
		VALUES ('TSK-1', '/r/a.py', 'h3', '2026-09-23T10:00:00Z')`); err == nil {
		t.Fatal("duplicate baseline row for one task path accepted")
	}
}

// TestOperationLogColumnsAreMandatory pins the replay contract columns:
// no anonymous or hash-less operation rows.
func TestOperationLogColumnsAreMandatory(t *testing.T) {
	db := migratedChangesSchema(t)
	if _, err := db.ExecContext(t.Context(), `INSERT INTO change_operations (operation_id, task_id, result, recorded_at)
		VALUES ('OP-1', 'TSK-1', '{}', '2026-09-23T10:00:00Z')`); err == nil {
		t.Fatal("operation row without request hash accepted")
	}
}

// TestChangeRowVocabularies pins the kind and via CHECKs: file kinds,
// symbol kinds and discovered_via accept only their documented values.
func TestChangeRowVocabularies(t *testing.T) {
	db := migratedChangesSchema(t)
	mustChangesSQL(t, db, `INSERT INTO changes (change_id, created_at, updated_at)
		VALUES ('CHG-AAAAAAAAAAAAAAAAAAAAAAAAAA', '2026-09-23T10:00:00Z', '2026-09-23T10:00:00Z')`)
	for _, tc := range []struct{ table, columns, values, want string }{
		{"change_files", "(change_id, path, kind, discovered_via)", "('CHG-AAAAAAAAAAAAAAAAAAAAAAAAAA', '/r/a.py', 'vaporized', 'baseline')", "file kind"},
		{"change_files", "(change_id, path, kind, discovered_via)", "('CHG-AAAAAAAAAAAAAAAAAAAAAAAAAA', '/r/a.py', 'modified', 'telemetry')", "discovered_via"},
		{"change_symbols", "(change_id, logical_key, kind, discovered_via)", "('CHG-AAAAAAAAAAAAAAAAAAAAAAAAAA', 'k', 'edited', 'reconcile')", "symbol kind"},
	} {
		t.Run(tc.want, func(t *testing.T) {
			if _, err := db.ExecContext(t.Context(), `INSERT INTO `+tc.table+` `+tc.columns+` VALUES `+tc.values); err == nil {
				t.Fatalf("change row with bad %s accepted", tc.want)
			}
		})
	}
	mustChangesSQL(t, db, `INSERT INTO change_operations (operation_id, request_hash, result, recorded_at)
		VALUES ('OP-1', 'h', '{}', '2026-09-23T10:00:00Z')`)
	if _, err := db.ExecContext(t.Context(), `INSERT INTO change_operations (operation_id, request_hash, result, recorded_at)
		VALUES ('OP-1', 'h', '{}', '2026-09-23T10:00:00Z')`); err == nil {
		t.Fatal("duplicate operation id accepted")
	}
}
