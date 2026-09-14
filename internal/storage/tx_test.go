package storage_test

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/PsyChaos/mindrail/internal/storage"
)

func TestInTxCommitsAndRollsBack(t *testing.T) {
	db := openTemp(t, storage.Options{})
	if _, err := db.ExecContext(t.Context(), `CREATE TABLE probe (id INTEGER PRIMARY KEY, v TEXT NOT NULL) STRICT`); err != nil {
		t.Fatalf("CREATE TABLE = %v, want no error", err)
	}

	sentinel := errors.New("caller decided to abort")

	tests := []struct {
		name      string
		body      func(context.Context, *sql.Tx) error
		wantErr   error
		wantRows  int
		wantPanic bool
	}{
		{
			name: "commit on success",
			body: func(ctx context.Context, tx *sql.Tx) error {
				_, err := tx.ExecContext(ctx, `INSERT INTO probe (id, v) VALUES (1, 'committed')`)
				return err
			},
			wantRows: 1,
		},
		{
			name: "rollback on error",
			body: func(ctx context.Context, tx *sql.Tx) error {
				if _, err := tx.ExecContext(ctx, `INSERT INTO probe (id, v) VALUES (2, 'rolled back')`); err != nil {
					return err
				}
				return sentinel
			},
			wantErr:  sentinel,
			wantRows: 1,
		},
		{
			name: "rollback on panic",
			body: func(ctx context.Context, tx *sql.Tx) error {
				if _, err := tx.ExecContext(ctx, `INSERT INTO probe (id, v) VALUES (3, 'panicked')`); err != nil {
					return err
				}
				panic(sentinel)
			},
			wantRows:  1,
			wantPanic: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := func() (err error) {
				if tt.wantPanic {
					defer func() {
						if recovered := recover(); recovered == nil {
							t.Error("InTx swallowed the panic, want it to propagate")
						}
					}()
				}
				return storage.InTx(t.Context(), db.DB, tt.body)
			}()

			if tt.wantErr != nil && !errors.Is(err, tt.wantErr) {
				t.Errorf("InTx = %v, want errors.Is(err, %v)", err, tt.wantErr)
			}
			if tt.wantErr == nil && !tt.wantPanic && err != nil {
				t.Errorf("InTx = %v, want no error", err)
			}

			var rows int
			if err := db.QueryRowContext(t.Context(), `SELECT count(*) FROM probe`).Scan(&rows); err != nil {
				t.Fatalf("count = %v, want no error", err)
			}
			if rows != tt.wantRows {
				t.Errorf("row count = %d, want %d", rows, tt.wantRows)
			}
		})
	}
}

// TestTxActiveTracksTheWriteWindow backs the tech-stack §21 rule that no long
// running work happens inside a write transaction: the git and filesystem
// probes assert TxActive is false before they start.
func TestTxActiveTracksTheWriteWindow(t *testing.T) {
	db := openTemp(t, storage.Options{})

	if storage.TxActive(t.Context()) {
		t.Error("TxActive(fresh context) = true, want false")
	}

	var insideOuter, insideNested bool
	err := storage.InTx(t.Context(), db.DB, func(ctx context.Context, tx *sql.Tx) error {
		insideOuter = storage.TxActive(ctx)
		// The flag rides on the context, so a helper handed the same context
		// sees it; a helper handed a pristine one does not.
		insideNested = storage.TxActive(context.Background())
		return nil
	})
	if err != nil {
		t.Fatalf("InTx = %v, want no error", err)
	}
	if !insideOuter {
		t.Error("TxActive(tx context) = false, want true")
	}
	if insideNested {
		t.Error("TxActive(unrelated context) = true, want false")
	}
	if storage.TxActive(t.Context()) {
		t.Error("TxActive after InTx = true, want false")
	}
}

// TestInTxTakesTheWriteLockAtBegin proves the _txlock=immediate half of
// decision D-24. A deferred BEGIN takes no lock until the first write, so a
// second writer would sail past this holder and only discover the conflict at
// COMMIT -- too late to do anything but discard its work.
func TestInTxTakesTheWriteLockAtBegin(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mindrail.db")

	holder := openTemp(t, storage.Options{Path: path})
	if _, err := holder.ExecContext(t.Context(),
		`CREATE TABLE probe (id INTEGER PRIMARY KEY) STRICT`); err != nil {
		t.Fatalf("CREATE TABLE = %v, want no error", err)
	}

	// A short timeout keeps the contended case fast; production uses 5s.
	contender := openTemp(t, storage.Options{Path: path, BusyTimeout: 50 * time.Millisecond})

	var (
		begun    = make(chan struct{})
		release  = make(chan struct{})
		holderWG sync.WaitGroup
	)
	holderWG.Add(1)
	go func() {
		defer holderWG.Done()
		err := storage.InTx(context.Background(), holder.DB, func(context.Context, *sql.Tx) error {
			// Deliberately no statements: the lock must come from BEGIN.
			close(begun)
			<-release
			return nil
		})
		if err != nil {
			t.Errorf("holder InTx = %v, want no error", err)
		}
	}()

	<-begun
	err := storage.InTx(t.Context(), contender.DB, func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO probe (id) VALUES (1)`)
		return err
	})
	if err == nil {
		t.Error("contender InTx = nil error while another transaction was open, want a busy failure")
	}

	close(release)
	holderWG.Wait()

	// With the lock released the same work succeeds, so the failure above was
	// contention and not a broken statement.
	if err := storage.InTx(t.Context(), contender.DB, func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO probe (id) VALUES (1)`)
		return err
	}); err != nil {
		t.Errorf("contender InTx after release = %v, want no error", err)
	}
}

// TestInTxReportsItsWaitAndHoldWhenAsked is AC-01.4 (decision D-75): a caller
// that asks gets the two numbers spec §11 wants measured, a committed
// transaction fills them, and a refused one leaves them alone — there is no
// Write to carry them on, and a number from a transaction that did not land
// would be the write-wait figure of nothing.
func TestInTxReportsItsWaitAndHoldWhenAsked(t *testing.T) {
	db := openTemp(t, storage.Options{})

	ctx, stats := storage.WithTxStats(t.Context())
	const hold = 20 * time.Millisecond
	err := storage.InTx(ctx, db.DB, func(context.Context, *sql.Tx) error {
		time.Sleep(hold)
		return nil
	})
	if err != nil {
		t.Fatalf("InTx = %v, want no error", err)
	}
	if stats.Held < hold {
		t.Errorf("stats.Held = %v, want at least the %v the body held the lock", stats.Held, hold)
	}
	if stats.Waited < 0 || stats.Waited > time.Second {
		t.Errorf("stats.Waited = %v on an idle database, want a small non-negative wait", stats.Waited)
	}

	// A second transaction under the same stats replaces the numbers rather
	// than adding to them: each Write reports its own transaction.
	if err := storage.InTx(ctx, db.DB, func(context.Context, *sql.Tx) error { return nil }); err != nil {
		t.Fatalf("second InTx = %v, want no error", err)
	}
	if stats.Held >= hold {
		t.Errorf("stats.Held = %v after an empty transaction, want it replaced rather than accumulated", stats.Held)
	}

	// A refused transaction writes nothing into them.
	refused := errors.New("caller decided to abort")
	before := *stats
	err = storage.InTx(ctx, db.DB, func(context.Context, *sql.Tx) error {
		time.Sleep(hold)
		return refused
	})
	if !errors.Is(err, refused) {
		t.Fatalf("InTx = %v, want the caller's refusal", err)
	}
	if *stats != before {
		t.Errorf("stats = %+v after a rolled-back transaction, want %+v untouched", *stats, before)
	}

	// And a context that never asked is served without one.
	if err := storage.InTx(t.Context(), db.DB, func(context.Context, *sql.Tx) error { return nil }); err != nil {
		t.Fatalf("InTx without stats = %v, want no error", err)
	}
}

// TestInTxDoesNotRetryBegin is AC-01.5 and the negative half of decision D-74:
// the busy budget is the connection's, spent inside SQLite's own handler, and a
// refusal that reaches Go is final. A ladder here would multiply the budget.
func TestInTxDoesNotRetryBegin(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mindrail.db")
	holder := openTemp(t, storage.Options{Path: path})
	tx, err := holder.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatalf("BeginTx = %v, want no error", err)
	}

	// The lock is held for six budgets and then released. One attempt is
	// refused at the first budget, well before the release; a loop that tried
	// again would meet the released lock and succeed, which is the shape this
	// test is written to turn red on — by outcome rather than by a stopwatch,
	// so a slow machine cannot fake either answer.
	const (
		budget = 50 * time.Millisecond
		hold   = 6 * budget
	)
	release := time.AfterFunc(hold, func() { _ = tx.Rollback() })
	t.Cleanup(func() { release.Stop(); _ = tx.Rollback() })

	contender := openTemp(t, storage.Options{Path: path, BusyTimeout: budget})

	started := time.Now()
	err = storage.InTx(t.Context(), contender.DB, func(context.Context, *sql.Tx) error { return nil })
	elapsed := time.Since(started)
	if !storage.IsBusy(err) {
		t.Fatalf("InTx = %v after %v, want SQLITE_BUSY: BEGIN was tried again until the lock was released", err, elapsed)
	}
	if elapsed < budget {
		t.Errorf("InTx refused after %v, before the %v budget the driver was told to wait", elapsed, budget)
	}
}

// TestParameterizedSQLSurvivesInjectionLiteral is the spec §113 assertion: the
// value below is data on every hop, never fragment of a statement.
func TestParameterizedSQLSurvivesInjectionLiteral(t *testing.T) {
	const injection = `'; DROP TABLE schema_migrations; --`

	db := openTemp(t, storage.Options{Path: filepath.Join(t.TempDir(), "mindrail.db")})
	if _, err := db.ExecContext(t.Context(), `CREATE TABLE schema_migrations (
		version INTEGER PRIMARY KEY,
		name TEXT NOT NULL
	)`); err != nil {
		t.Fatalf("CREATE TABLE = %v, want no error", err)
	}

	err := storage.InTx(t.Context(), db.DB, func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations (version, name) VALUES (?, ?)`, 1, injection)
		return err
	})
	if err != nil {
		t.Fatalf("InTx = %v, want no error", err)
	}

	var got string
	if err := db.QueryRowContext(t.Context(),
		`SELECT name FROM schema_migrations WHERE name = ?`, injection).Scan(&got); err != nil {
		t.Fatalf("SELECT = %v, want the literal to round-trip", err)
	}
	if got != injection {
		t.Errorf("name = %q, want %q", got, injection)
	}

	var tables int
	if err := db.QueryRowContext(t.Context(),
		`SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = 'schema_migrations'`).Scan(&tables); err != nil {
		t.Fatalf("sqlite_master = %v, want no error", err)
	}
	if tables != 1 {
		t.Errorf("schema_migrations table count = %d, want 1 (the injection executed)", tables)
	}
}
