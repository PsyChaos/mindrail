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

func migratedIndexSchema(t *testing.T) *sql.DB {
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

func mustIndexSQL(t *testing.T, db *sql.DB, statement string, args ...any) {
	t.Helper()
	if _, err := db.ExecContext(t.Context(), statement, args...); err != nil {
		t.Fatalf("%s: %v", statement, err)
	}
}

func seedIndexConstraintRows(t *testing.T, db *sql.DB) {
	t.Helper()
	mustIndexSQL(t, db, `INSERT INTO project_units (id, path, kind, discovered_at) VALUES ('UNT-1', '/repo/unit', 'python', '2026-09-18T10:00:00Z')`)
	mustIndexSQL(t, db, `INSERT INTO file_index_state (path, unit_id, language, state) VALUES ('/repo/unit/a.py', 'UNT-1', 'python', 'pending')`)
	mustIndexSQL(t, db, `INSERT INTO symbols (id, unit_id, path, logical_key, kind, name, start_line, start_col, end_line, end_col, signature_hash, body_hash, structure_hash)
		VALUES (10, 'UNT-1', '/repo/unit/a.py', 'f', 'function', 'f', 1, 0, 2, 0, 's', 'b', 't')`)
	mustIndexSQL(t, db, `INSERT INTO symbol_imports (id, unit_id, path, module, names, is_relative)
		VALUES (20, 'UNT-1', '/repo/unit/a.py', 'os', '[]', 0)`)
	mustIndexSQL(t, db, `INSERT INTO symbol_references (id, unit_id, path, target_text, confidence, resolved_symbol_id)
		VALUES (30, 'UNT-1', '/repo/unit/a.py', 'f', 0.5, 10)`)
}

func TestIdentityAllocationKeyIsUniquePerProjectUnitLanguageKey(t *testing.T) {
	db := migratedIndexSchema(t)
	mustIndexSQL(t, db, `INSERT INTO project_units (id, path, kind, discovered_at) VALUES ('UNT-1', '/repo/unit', 'python', '2026-09-18T10:00:00Z')`)
	mustIndexSQL(t, db, `INSERT INTO symbol_identities (symbol_uid, project_id, unit_id, language, logical_key, created_at)
		VALUES ('SYM-AAAAAAAAAAAAAAAAAAAAAAAAAA', 'PRJ-1', 'UNT-1', 'python', 'k', '2026-09-18T10:00:00Z')`)
	// The allocation key is the arbiter: a second insert for the same key
	// fails, and the loser reads back the winner (spec §24, D-94).
	if _, err := db.ExecContext(t.Context(), `INSERT INTO symbol_identities (symbol_uid, project_id, unit_id, language, logical_key, created_at)
		VALUES ('SYM-BBBBBBBBBBBBBBBBBBBBBBBBBB', 'PRJ-1', 'UNT-1', 'python', 'k', '2026-09-18T10:00:00Z')`); err == nil {
		t.Fatal("duplicate allocation key accepted a second uid")
	}
	// A different project may hold the same key: identity is per project.
	mustIndexSQL(t, db, `INSERT INTO symbol_identities (symbol_uid, project_id, unit_id, language, logical_key, created_at)
		VALUES ('SYM-BBBBBBBBBBBBBBBBBBBBBBBBBB', 'PRJ-2', 'UNT-1', 'python', 'k', '2026-09-18T10:00:00Z')`)
}

func TestIdentityContainerUidRejectsDanglingParent(t *testing.T) {
	db := migratedIndexSchema(t)
	mustIndexSQL(t, db, `INSERT INTO project_units (id, path, kind, discovered_at) VALUES ('UNT-1', '/repo/unit', 'python', '2026-09-18T10:00:00Z')`)
	if _, err := db.ExecContext(t.Context(), `INSERT INTO symbol_identities (symbol_uid, project_id, unit_id, language, logical_key, container_uid, created_at)
		VALUES ('SYM-AAAAAAAAAAAAAAAAAAAAAAAAAA', 'PRJ-1', 'UNT-1', 'python', 'k', 'SYM-NOOOOOOOOOOOOOOOOOOOOOOOOO', '2026-09-18T10:00:00Z')`); err == nil {
		t.Fatal("container_uid pointing at no identity accepted")
	}
	mustIndexSQL(t, db, `INSERT INTO symbol_identities (symbol_uid, project_id, unit_id, language, logical_key, created_at)
		VALUES ('SYM-PAAAAAAAAAAAAAAAAAAAAAAAAA', 'PRJ-1', 'UNT-1', 'python', 'parent', '2026-09-18T10:00:00Z')`)
	mustIndexSQL(t, db, `INSERT INTO symbol_identities (symbol_uid, project_id, unit_id, language, logical_key, container_uid, created_at)
		VALUES ('SYM-AAAAAAAAAAAAAAAAAAAAAAAAAA', 'PRJ-1', 'UNT-1', 'python', 'k', 'SYM-PAAAAAAAAAAAAAAAAAAAAAAAAA', '2026-09-18T10:00:00Z')`)
}

func TestIdentityBindingStatusAndSymbolUidAreConstrained(t *testing.T) {
	db := migratedIndexSchema(t)
	mustIndexSQL(t, db, `INSERT INTO project_units (id, path, kind, discovered_at) VALUES ('UNT-1', '/repo/unit', 'python', '2026-09-18T10:00:00Z')`)
	mustIndexSQL(t, db, `INSERT INTO symbol_identities (symbol_uid, project_id, unit_id, language, logical_key, created_at)
		VALUES ('SYM-AAAAAAAAAAAAAAAAAAAAAAAAAA', 'PRJ-1', 'UNT-1', 'python', 'k', '2026-09-18T10:00:00Z')`)
	if _, err := db.ExecContext(t.Context(), `INSERT INTO invariant_symbol_bindings (invariant_id, symbol_uid, status, updated_at)
		VALUES ('INV-0001', 'SYM-AAAAAAAAAAAAAAAAAAAAAAAAAA', 'maybe', '2026-09-18T10:00:00Z')`); err == nil {
		t.Fatal("binding accepted a status outside bound/ambiguous/orphaned")
	}
	mustIndexSQL(t, db, `INSERT INTO invariant_symbol_bindings (invariant_id, symbol_uid, status, updated_at)
		VALUES ('INV-0001', 'SYM-AAAAAAAAAAAAAAAAAAAAAAAAAA', 'bound', '2026-09-18T10:00:00Z')`)
	// The pair is the grain: the same invariant may bind another uid, but not
	// the same uid twice.
	if _, err := db.ExecContext(t.Context(), `INSERT INTO invariant_symbol_bindings (invariant_id, symbol_uid, status, updated_at)
		VALUES ('INV-0001', 'SYM-AAAAAAAAAAAAAAAAAAAAAAAAAA', 'bound', '2026-09-18T10:00:00Z')`); err == nil {
		t.Fatal("duplicate invariant/uid binding accepted")
	}
	// symbol_uid stays nullable for pre-MR-006 rows and points at identities.
	mustIndexSQL(t, db, `INSERT INTO symbols (unit_id, path, logical_key, kind, name, start_line, start_col, end_line, end_col, signature_hash, body_hash, structure_hash, symbol_uid)
		VALUES ('UNT-1', '/repo/unit/a.py', 'k', 'function', 'f', 1, 0, 2, 0, 's', 'b', 't', 'SYM-AAAAAAAAAAAAAAAAAAAAAAAAAA')`)
	if _, err := db.ExecContext(t.Context(), `INSERT INTO symbols (unit_id, path, logical_key, kind, name, start_line, start_col, end_line, end_col, signature_hash, body_hash, structure_hash, symbol_uid)
		VALUES ('UNT-1', '/repo/unit/a.py', 'k', 'function', 'g', 3, 0, 4, 0, 's', 'b', 't', 'SYM-NOOOOOOOOOOOOOOOOOOOOOOOOO')`); err == nil {
		t.Fatal("symbol_uid pointing at no identity accepted")
	}
}

func TestIndexStateCheckRejectsUnknownValue(t *testing.T) {
	db := migratedIndexSchema(t)
	seedIndexConstraintRows(t, db)
	if _, err := db.ExecContext(t.Context(), `UPDATE file_index_state SET state = 'typo' WHERE path = '/repo/unit/a.py'`); err == nil {
		t.Fatal("raw SQL accepted an unknown durable index state")
	}
	var state string
	if err := db.QueryRowContext(t.Context(), `SELECT state FROM file_index_state WHERE path = '/repo/unit/a.py'`).Scan(&state); err != nil || state != "pending" {
		t.Fatalf("rejected state write changed committed value to %q, err=%v", state, err)
	}
	for _, valid := range []string{"indexed", "failed", "unsupported", "pending"} {
		if _, err := db.ExecContext(t.Context(), `UPDATE file_index_state SET state = ? WHERE path = '/repo/unit/a.py'`, valid); err != nil {
			t.Fatalf("valid state %q rejected: %v", valid, err)
		}
	}
}

func TestIndexTablesDeclarePrimaryKeys(t *testing.T) {
	db := migratedIndexSchema(t)
	for _, tc := range []struct{ table, column string }{
		{"project_units", "id"},
		{"file_index_state", "path"},
		{"symbols", "id"},
		{"symbol_imports", "id"},
		{"symbol_references", "id"},
	} {
		t.Run(tc.table, func(t *testing.T) {
			rows, err := db.QueryContext(t.Context(), `PRAGMA table_info(`+tc.table+`)`)
			if err != nil {
				t.Fatal(err)
			}
			defer rows.Close()
			var primary string
			for rows.Next() {
				var cid, notNull, pk int
				var name, typeName string
				var defaultValue sql.NullString
				if err := rows.Scan(&cid, &name, &typeName, &notNull, &defaultValue, &pk); err != nil {
					t.Fatal(err)
				}
				if pk != 0 {
					if primary != "" {
						t.Fatalf("multiple primary-key columns: %s, %s", primary, name)
					}
					primary = name
				}
			}
			if err := rows.Err(); err != nil {
				t.Fatal(err)
			}
			if primary != tc.column {
				t.Fatalf("%s primary key = %q, want %q", tc.table, primary, tc.column)
			}
		})
	}
}

func TestIndexTablesEnforceKeysNullabilityTypesAndReferences(t *testing.T) {
	db := migratedIndexSchema(t)
	seedIndexConstraintRows(t, db)
	// logical_key is deliberately non-unique (D-83), even though row ID is.
	mustIndexSQL(t, db, `INSERT INTO symbols (unit_id, path, logical_key, kind, name, start_line, start_col, end_line, end_col, signature_hash, body_hash, structure_hash)
		VALUES ('UNT-1', '/repo/unit/a.py', 'f', 'function', 'f', 3, 0, 4, 0, 's', 'b', 't')`)
	var sameKey int
	if err := db.QueryRowContext(t.Context(), `SELECT count(*) FROM symbols WHERE unit_id = 'UNT-1' AND logical_key = 'f'`).Scan(&sameKey); err != nil || sameKey != 2 {
		t.Fatalf("plain logical-key index held %d same-key rows, err=%v", sameKey, err)
	}

	invalid := []struct {
		name string
		sql  string
		args []any
	}{
		{"unit primary key", `INSERT INTO project_units (id, path, kind, discovered_at) VALUES ('UNT-1', '/other', 'python', '2026-09-18T10:00:00Z')`, nil},
		{"unit unique path", `INSERT INTO project_units (id, path, kind, discovered_at) VALUES ('UNT-2', '/repo/unit', 'python', '2026-09-18T10:00:00Z')`, nil},
		{"unit required timestamp", `INSERT INTO project_units (id, path, kind, discovered_at) VALUES ('UNT-2', '/other', 'python', NULL)`, nil},
		{"unit strict text", `INSERT INTO project_units (id, path, kind, discovered_at) VALUES ('UNT-2', '/other', ?, '2026-09-18T10:00:00Z')`, []any{[]byte{0xff}}},
		{"file primary key", `INSERT INTO file_index_state (path, unit_id, language, state) VALUES ('/repo/unit/a.py', 'UNT-1', 'python', 'pending')`, nil},
		{"file unit foreign key", `INSERT INTO file_index_state (path, unit_id, language, state) VALUES ('/repo/unit/b.py', 'UNT-missing', 'python', 'pending')`, nil},
		{"file required language", `INSERT INTO file_index_state (path, unit_id, language, state) VALUES ('/repo/unit/b.py', 'UNT-1', NULL, 'pending')`, nil},
		{"file strict attempts", `INSERT INTO file_index_state (path, unit_id, language, state, attempts) VALUES ('/repo/unit/b.py', 'UNT-1', 'python', 'pending', 'not-a-number')`, nil},
		{"symbol primary key", `INSERT INTO symbols (id, unit_id, path, logical_key, kind, name, start_line, start_col, end_line, end_col, signature_hash, body_hash, structure_hash) VALUES (10, 'UNT-1', '/repo/unit/a.py', 'g', 'function', 'g', 1, 0, 2, 0, 's', 'b', 't')`, nil},
		{"symbol unit foreign key", `INSERT INTO symbols (unit_id, path, logical_key, kind, name, start_line, start_col, end_line, end_col, signature_hash, body_hash, structure_hash) VALUES ('UNT-missing', '/repo/unit/a.py', 'g', 'function', 'g', 1, 0, 2, 0, 's', 'b', 't')`, nil},
		{"symbol required kind", `INSERT INTO symbols (unit_id, path, logical_key, kind, name, start_line, start_col, end_line, end_col, signature_hash, body_hash, structure_hash) VALUES ('UNT-1', '/repo/unit/a.py', 'g', NULL, 'g', 1, 0, 2, 0, 's', 'b', 't')`, nil},
		{"symbol strict line", `INSERT INTO symbols (unit_id, path, logical_key, kind, name, start_line, start_col, end_line, end_col, signature_hash, body_hash, structure_hash) VALUES ('UNT-1', '/repo/unit/a.py', 'g', 'function', 'g', 'not-a-line', 0, 2, 0, 's', 'b', 't')`, nil},
		{"import primary key", `INSERT INTO symbol_imports (id, unit_id, path, module, names, is_relative) VALUES (20, 'UNT-1', '/repo/unit/a.py', 'json', '[]', 0)`, nil},
		{"import unit foreign key", `INSERT INTO symbol_imports (unit_id, path, module, names, is_relative) VALUES ('UNT-missing', '/repo/unit/a.py', 'json', '[]', 0)`, nil},
		{"import required names", `INSERT INTO symbol_imports (unit_id, path, module, names, is_relative) VALUES ('UNT-1', '/repo/unit/a.py', 'json', NULL, 0)`, nil},
		{"import strict relative", `INSERT INTO symbol_imports (unit_id, path, module, names, is_relative) VALUES ('UNT-1', '/repo/unit/a.py', 'json', '[]', 'not-a-flag')`, nil},
		{"reference primary key", `INSERT INTO symbol_references (id, unit_id, path, target_text, confidence) VALUES (30, 'UNT-1', '/repo/unit/a.py', 'f', 0.5)`, nil},
		{"reference unit foreign key", `INSERT INTO symbol_references (unit_id, path, target_text, confidence) VALUES ('UNT-missing', '/repo/unit/a.py', 'f', 0.5)`, nil},
		{"reference resolved-symbol foreign key", `INSERT INTO symbol_references (unit_id, path, target_text, confidence, resolved_symbol_id) VALUES ('UNT-1', '/repo/unit/a.py', 'f', 0.5, 99999)`, nil},
		{"reference required target", `INSERT INTO symbol_references (unit_id, path, target_text, confidence) VALUES ('UNT-1', '/repo/unit/a.py', NULL, 0.5)`, nil},
		{"reference strict scope", `INSERT INTO symbol_references (unit_id, path, target_text, scope_text, confidence) VALUES ('UNT-1', '/repo/unit/a.py', 'f', ?, 0.5)`, []any{[]byte{0xff}}},
		{"reference confidence bound", `INSERT INTO symbol_references (unit_id, path, target_text, confidence) VALUES ('UNT-1', '/repo/unit/a.py', 'f', 0.6)`, nil},
	}
	for _, tc := range invalid {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := db.ExecContext(t.Context(), tc.sql, tc.args...); err == nil {
				t.Fatalf("schema accepted %s violation", tc.name)
			}
		})
	}
}
