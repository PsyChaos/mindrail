package index

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"net/url"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/migration"
	"github.com/PsyChaos/mindrail/internal/storage"
	"github.com/PsyChaos/mindrail/migrations"
)

// errorFault wraps only the database driver boundary. Every successful query,
// transaction and rollback still executes against the migrated SQLite database.
// A fault is armed after fixture setup and disarmed after its first match.
type errorFault struct {
	query string
	mode  string
	cause error
	hits  int
}

type errorConnector struct {
	driver driver.Driver
	dsn    string
	fault  *errorFault
}

func (c errorConnector) Connect(context.Context) (driver.Conn, error) {
	conn, err := c.driver.Open(c.dsn)
	if err != nil {
		return nil, err
	}
	return &errorConn{Conn: conn, fault: c.fault}, nil
}

func (c errorConnector) Driver() driver.Driver { return c.driver }

type errorConn struct {
	driver.Conn
	fault *errorFault
}

func (c *errorConn) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	return c.Conn.(driver.ConnBeginTx).BeginTx(ctx, opts)
}

func (c *errorConn) match(query string) bool {
	if c.fault.query == "" || c.fault.hits != 0 || !strings.Contains(query, c.fault.query) {
		return false
	}
	c.fault.hits++
	return true
}

func (c *errorConn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	if c.match(query) {
		return nil, c.fault.cause
	}
	return c.Conn.(driver.ExecerContext).ExecContext(ctx, query, args)
}

func (c *errorConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	matched := c.match(query)
	if matched && c.fault.mode == "query" {
		return nil, c.fault.cause
	}
	rows, err := c.Conn.(driver.QueryerContext).QueryContext(ctx, query, args)
	if err != nil || !matched {
		return rows, err
	}
	return &errorRows{Rows: rows, fault: c.fault}, nil
}

type errorRows struct {
	driver.Rows
	fault *errorFault
	seen  bool
}

func (r *errorRows) Next(dest []driver.Value) error {
	if r.fault.mode == "rows" && r.seen {
		return r.fault.cause
	}
	err := r.Rows.Next(dest)
	if err == nil && !r.seen && r.fault.mode == "scan" {
		// NULL cannot scan into a mandatory Go string/int field. This models
		// a damaged persisted shape without changing the production schema.
		dest[0] = nil
	}
	r.seen = true
	return err
}

func errorStore(t *testing.T) (*Store, *sql.DB, *errorFault) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "mindrail.db")
	db, err := storage.Open(t.Context(), storage.Options{Path: path})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	set, err := migration.Load(migrations.FS)
	if err != nil {
		t.Fatal(err)
	}
	clock := app.FixedClock{Instant: time.Date(2026, 9, 18, 10, 0, 0, 0, time.UTC)}
	if _, err := migration.New(db.DB, set, clock).Up(t.Context()); err != nil {
		t.Fatal(err)
	}
	uri := url.URL{Scheme: "file", Path: path, RawQuery: "_pragma=foreign_keys(1)&_txlock=immediate"}
	fault := &errorFault{}
	injected := sql.OpenDB(errorConnector{driver: db.Driver(), dsn: uri.String(), fault: fault})
	injected.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = injected.Close() })
	return NewStore(injected, clock), db.DB, fault
}

// errorSnapshot compares every persisted cell, including IDs, pointers, hash,
// attempt count and timestamps, so an error cannot conceal a partial commit.
func errorSnapshot(t *testing.T, db *sql.DB) map[string][][]any {
	t.Helper()
	result := make(map[string][][]any)
	for _, table := range []string{"project_units", "file_index_state", "symbols", "symbol_imports", "symbol_references"} {
		rows, err := db.QueryContext(t.Context(), "SELECT * FROM "+table+" ORDER BY rowid")
		if err != nil {
			t.Fatal(err)
		}
		cols, err := rows.Columns()
		if err != nil {
			t.Fatal(err)
		}
		for rows.Next() {
			values := make([]any, len(cols))
			pointers := make([]any, len(cols))
			for i := range values {
				pointers[i] = &values[i]
			}
			if err := rows.Scan(pointers...); err != nil {
				t.Fatal(err)
			}
			result[table] = append(result[table], values)
		}
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
		if err := rows.Close(); err != nil {
			t.Fatal(err)
		}
	}
	return result
}

func errorFixture(t *testing.T, store *Store) (ProjectUnit, FileFacts, int64) {
	t.Helper()
	unit, err := store.UpsertUnit(t.Context(), filepath.Join(t.TempDir(), "package"), UnitPython)
	if err != nil {
		t.Fatal(err)
	}
	facts := FileFacts{UnitID: unit.ID, Path: filepath.Join(unit.Path, "a.py"), Language: "python", ContentHash: "old", State: StateFailed, LastError: "old parse error",
		Symbols: []Symbol{{LogicalKey: "f", Kind: "function", Name: "f", StartLine: 1, EndLine: 2, SignatureHash: "sig", BodyHash: "body", StructureHash: "shape"}},
		Imports: []Import{{Module: "os", Names: []string{"path"}}}, References: []Reference{{TargetText: "f", TargetLogicalKey: "f", Confidence: .5}}}
	if err := store.UpsertFileState(t.Context(), FileIndexState{UnitID: unit.ID, Path: facts.Path, Language: "python", State: StatePending}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ReplaceFileFacts(t.Context(), facts); err != nil {
		t.Fatal(err)
	}
	// A sibling declaration survives replacing a.py and supplies a valid
	// explicit-ID target for the resolution error paths.
	sibling := facts
	sibling.Path = filepath.Join(unit.Path, "b.py")
	sibling.Symbols = []Symbol{{LogicalKey: "g", Kind: "function", Name: "g", StartLine: 1, EndLine: 2}}
	sibling.References = nil
	if err := store.UpsertFileState(t.Context(), FileIndexState{UnitID: unit.ID, Path: sibling.Path, Language: "python", State: StatePending}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ReplaceFileFacts(t.Context(), sibling); err != nil {
		t.Fatal(err)
	}
	var id int64
	if err := store.db.QueryRowContext(t.Context(), "SELECT id FROM symbols WHERE logical_key = 'g'").Scan(&id); err != nil {
		t.Fatal(err)
	}
	return unit, facts, id
}

func TestStoreWriteFaultPreservesCauseAndRollsBack(t *testing.T) {
	cases := []struct {
		name, operation, query string
	}{
		{"unit_insert", "unit", "INSERT INTO project_units"},
		{"unit_lookup", "unit", "SELECT id, path, kind, discovered_at"},
		{"registration_unit_lookup", "register", "SELECT path FROM project_units"},
		{"registration_owner_lookup", "register", "SELECT unit_id FROM file_index_state"},
		{"registration_write", "register", "INSERT INTO file_index_state"},
		{"replacement_unit_lookup", "replace", "SELECT path FROM project_units"},
		{"replacement_owner_lookup", "replace", "SELECT unit_id FROM file_index_state"},
		{"delete_references", "replace", "DELETE FROM symbol_references"},
		{"delete_imports", "replace", "DELETE FROM symbol_imports"},
		{"delete_symbols", "replace", "DELETE FROM symbols"},
		{"insert_symbol", "replace", "INSERT INTO symbols"},
		{"insert_import", "replace", "INSERT INTO symbol_imports"},
		{"resolve_key_query", "replace", "SELECT id FROM symbols WHERE unit_id"},
		{"explicit_target_lookup", "explicit", "SELECT logical_key FROM symbols"},
		{"explicit_target_unique_query", "explicit", "SELECT id FROM symbols WHERE unit_id"},
		{"insert_reference", "replace", "INSERT INTO symbol_references"},
		{"state_write", "replace", "UPDATE file_index_state SET"},
		{"owner_transfer_delete_references", "transfer", "DELETE FROM symbol_references"},
		{"owner_transfer_delete_imports", "transfer", "DELETE FROM symbol_imports"},
		{"owner_transfer_delete_symbols", "transfer", "DELETE FROM symbols"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store, db, fault := errorStore(t)
			unit, facts, id := errorFixture(t, store)
			facts.ContentHash, facts.State, facts.LastError = "new", StateIndexed, ""
			registration := FileIndexState{UnitID: unit.ID, Path: facts.Path, Language: "python", ContentHash: "new", State: StatePending}
			if tc.operation == "explicit" {
				facts.References = []Reference{{TargetText: "g", ResolvedSymbolID: &id, Confidence: .5}}
			}
			if tc.operation == "transfer" {
				parent, err := store.UpsertUnit(t.Context(), filepath.Dir(unit.Path), UnitPython)
				if err != nil {
					t.Fatal(err)
				}
				registration.UnitID = parent.ID
			}
			before := errorSnapshot(t, db)
			cause := errors.New("injected " + tc.name)
			*fault = errorFault{query: tc.query, mode: "query", cause: cause}
			var err error
			switch tc.operation {
			case "unit":
				_, err = store.UpsertUnit(t.Context(), unit.Path, UnitTypeScript)
			case "register", "transfer":
				err = store.UpsertFileState(t.Context(), registration)
			default:
				var stats storage.TxStats
				stats, err = store.ReplaceFileFacts(t.Context(), facts)
				if stats != (storage.TxStats{}) {
					t.Fatalf("failed transaction reports committed timings: %+v", stats)
				}
			}
			if fault.hits != 1 || !errors.Is(err, cause) {
				t.Fatalf("fault hits=%d, error=%v; want original cause %v", fault.hits, err, cause)
			}
			if payload, ok := app.PayloadOf(err); ok {
				t.Fatalf("statement fault misclassified as domain failure: %+v", payload)
			}
			if after := errorSnapshot(t, db); !reflect.DeepEqual(after, before) {
				t.Fatalf("failed operation partially committed: before=%v after=%v", before, after)
			}
		})
	}
}

func TestStoreReadFaultDoesNotPublishPartialResults(t *testing.T) {
	for _, operation := range []string{"pending", "counts", "resolve", "explicit"} {
		for _, mode := range []string{"query", "scan", "rows"} {
			t.Run(operation+"_"+mode, func(t *testing.T) {
				store, db, fault := errorStore(t)
				unit, facts, id := errorFixture(t, store)
				before := errorSnapshot(t, db)
				cause := errors.New("injected " + operation + " " + mode)
				query := "SELECT id FROM symbols WHERE unit_id"
				switch operation {
				case "pending":
					query = "SELECT unit_id, path, language, content_hash"
				case "counts":
					query = "SELECT state, count(*)"
				case "explicit":
					facts.References = []Reference{{TargetText: "g", ResolvedSymbolID: &id, Confidence: .5}}
				}
				*fault = errorFault{query: query, mode: mode, cause: cause}
				var err error
				switch operation {
				case "pending":
					var rows []FileIndexState
					rows, err = store.ListPending(t.Context(), unit.ID)
					if rows != nil {
						t.Fatalf("read failure published partial queue: %+v", rows)
					}
				case "counts":
					var counts map[FileState]int
					counts, err = store.CountByState(t.Context(), unit.ID)
					if counts != nil {
						t.Fatalf("read failure published partial readiness counts: %+v", counts)
					}
				default:
					facts.ContentHash = "new"
					var stats storage.TxStats
					stats, err = store.ReplaceFileFacts(t.Context(), facts)
					if stats != (storage.TxStats{}) {
						t.Fatalf("resolution failure published committed timings: %+v", stats)
					}
				}
				if fault.hits != 1 || err == nil {
					t.Fatalf("fault hits=%d, error=%v", fault.hits, err)
				}
				if mode == "scan" {
					conversion := "converting NULL to string is unsupported"
					if operation == "resolve" || operation == "explicit" {
						conversion = "converting NULL to int64 is unsupported"
					}
					if !strings.Contains(err.Error(), conversion) {
						payload, ok := app.PayloadOf(err)
						if !ok || !strings.Contains(payload.Cause, conversion) {
							t.Fatalf("missing exact scan-conversion cause %q: %v", conversion, err)
						}
					}
				} else if !errors.Is(err, cause) {
					t.Fatalf("lost original driver cause: %v", err)
				}
				payload, classified := app.PayloadOf(err)
				if operation == "pending" || operation == "counts" {
					if !classified || payload.Code != app.CodeIndexStateCorrupt {
						t.Fatalf("read error classification = %+v, present=%v", payload, classified)
					}
				} else if classified {
					t.Fatalf("resolution statement fault unexpectedly classified: %+v", payload)
				}
				if after := errorSnapshot(t, db); !reflect.DeepEqual(after, before) {
					t.Fatalf("read/resolution failure changed persisted data: before=%v after=%v", before, after)
				}
			})
		}
	}
}

func TestStoreCancellationRetainsClassification(t *testing.T) {
	for _, cause := range []error{context.Canceled, context.DeadlineExceeded} {
		for _, operation := range []string{"schema", "read", "write"} {
			t.Run(operation+"_"+cause.Error(), func(t *testing.T) {
				store, db, fault := errorStore(t)
				unit, facts, _ := errorFixture(t, store)
				before := errorSnapshot(t, db)
				query := "SELECT max(version) FROM schema_migrations"
				if operation == "read" {
					query = "SELECT unit_id, path, language, content_hash"
				} else if operation == "write" {
					query = "INSERT INTO symbol_imports"
				}
				*fault = errorFault{query: query, mode: "query", cause: cause}
				var err error
				if operation == "write" {
					_, err = store.ReplaceFileFacts(t.Context(), facts)
				} else {
					_, err = store.ListPending(t.Context(), unit.ID)
				}
				if fault.hits != 1 || !errors.Is(err, cause) {
					t.Fatalf("hits=%d, error=%v; want %v", fault.hits, err, cause)
				}
				if err.Error() != cause.Error() {
					t.Fatalf("cancellation diagnosis was rewritten: got %q want %q", err.Error(), cause.Error())
				}
				if payload, ok := app.PayloadOf(err); ok {
					t.Fatalf("cancellation became repair/retry diagnosis: %+v", payload)
				}
				if after := errorSnapshot(t, db); !reflect.DeepEqual(after, before) {
					t.Fatalf("cancellation changed facts: before=%v after=%v", before, after)
				}
			})
		}
	}
}

func TestStoreCancellationOnUninitializedDatabaseRetainsClassification(t *testing.T) {
	for _, cause := range []error{context.Canceled, context.DeadlineExceeded} {
		t.Run(cause.Error(), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "mindrail.db")
			db, err := storage.Open(t.Context(), storage.Options{Path: path})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = db.Close() })
			var tables int
			if err := db.QueryRowContext(t.Context(), "SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name NOT GLOB 'sqlite_*'").Scan(&tables); err != nil || tables != 0 {
				t.Fatalf("fixture must be an uninitialized database: tables=%d err=%v", tables, err)
			}
			fault := &errorFault{query: "SELECT max(version) FROM schema_migrations", mode: "query", cause: cause}
			uri := url.URL{Scheme: "file", Path: path}
			injected := sql.OpenDB(errorConnector{driver: db.Driver(), dsn: uri.String(), fault: fault})
			injected.SetMaxOpenConns(1)
			t.Cleanup(func() { _ = injected.Close() })
			store := NewStore(injected, app.FixedClock{})
			rows, err := store.ListPending(t.Context(), "")
			if t.Context().Err() != nil {
				t.Fatalf("outer context must remain active: %v", t.Context().Err())
			}
			if fault.hits != 1 {
				t.Fatalf("schema query injection hits=%d, want 1", fault.hits)
			}
			if payload, ok := app.PayloadOf(err); ok {
				t.Fatalf("schema cancellation became repair/migration diagnosis: %+v", payload)
			}
			if !errors.Is(err, cause) || err.Error() != cause.Error() {
				t.Fatalf("schema cancellation lost its original cause: got %v want %v", err, cause)
			}
			if rows != nil {
				t.Fatalf("schema cancellation published a queue: %+v", rows)
			}
		})
	}
}

func TestStoreDomainFailurePreservesDiagnosis(t *testing.T) {
	store, db, fault := errorStore(t)
	_, facts, _ := errorFixture(t, store)
	before := errorSnapshot(t, db)
	cause := invalidInput("a protected fact cannot be replaced")
	*fault = errorFault{query: "INSERT INTO symbols", mode: "query", cause: cause}
	_, err := store.ReplaceFileFacts(t.Context(), facts)
	if fault.hits != 1 || !errors.Is(err, cause) {
		t.Fatalf("hits=%d, error=%v; want original domain cause", fault.hits, err)
	}
	if err.Error() != cause.Error() {
		t.Fatalf("domain diagnosis was rewritten: got %q want %q", err.Error(), cause.Error())
	}
	want, _ := app.PayloadOf(cause)
	got, ok := app.PayloadOf(err)
	if !ok || !reflect.DeepEqual(got, want) {
		t.Fatalf("domain diagnosis changed: got=%+v want=%+v", got, want)
	}
	if after := errorSnapshot(t, db); !reflect.DeepEqual(after, before) {
		t.Fatalf("domain failure changed facts: before=%v after=%v", before, after)
	}
}
