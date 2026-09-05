package storage

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// The values spec §10 recommends and decision D-22 promotes to a requirement.
// foreign_keys is the load-bearing one: with it off, every REFERENCES clause a
// later MR writes is decoration, and the damage is silent.
const (
	expectedJournalMode = "wal"
	expectedForeignKeys = 1
	expectedSynchronous = 1 // NORMAL: safe under WAL, without fsync per commit

	// DefaultBusyTimeout matches app.ShutdownTimeout (decision D-08) so a
	// writer that waits out the full contention window still fits inside the
	// shutdown budget.
	DefaultBusyTimeout = 5 * time.Second

	// DefaultMaxOpenConns bounds the pool. SQLite serializes writers anyway;
	// the connections exist so concurrent readers do not queue behind them.
	DefaultMaxOpenConns = 4
)

// Pragmas is the observed connection configuration. It is a value type so a
// test can compare a whole connection's settings in one assertion instead of
// four, and it serializes into the doctor report unchanged.
type Pragmas struct {
	JournalMode string `json:"journal_mode"` // "wal"
	ForeignKeys int    `json:"foreign_keys"` // 1
	BusyTimeout int    `json:"busy_timeout"` // 5000
	Synchronous int    `json:"synchronous"`  // 1
}

// Querier is the subset of database/sql shared by *sql.DB, *sql.Conn and
// *sql.Tx. Helpers take it so the same code can inspect a pooled connection,
// the pool itself, or a transaction.
type Querier interface {
	ExecContext(ctx context.Context, q string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, q string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, q string, args ...any) *sql.Row
}

var (
	_ Querier = (*sql.DB)(nil)
	_ Querier = (*sql.Conn)(nil)
	_ Querier = (*sql.Tx)(nil)
)

// ExpectedPragmas returns what a correctly configured connection must report.
// A zero or sub-millisecond busy timeout means "unset", not "do not wait".
func ExpectedPragmas(busyTimeout time.Duration) Pragmas {
	return Pragmas{
		JournalMode: expectedJournalMode,
		ForeignKeys: expectedForeignKeys,
		BusyTimeout: busyTimeoutMillis(busyTimeout),
		Synchronous: expectedSynchronous,
	}
}

// busyTimeoutMillis converts a duration to SQLite's millisecond unit, falling
// back to the default when the caller left it zero or asked for less than one
// millisecond, which SQLite would round to "do not wait at all".
func busyTimeoutMillis(busyTimeout time.Duration) int {
	millis := busyTimeout.Milliseconds()
	if millis <= 0 {
		millis = DefaultBusyTimeout.Milliseconds()
	}
	return int(millis)
}

// ReadPragmas reads the four settings back from whichever connection q maps
// to. Reading is the whole point: a DSN parameter that the driver silently
// dropped, or a pooled connection opened before the setting existed, is
// indistinguishable from a correct one until someone asks.
func ReadPragmas(ctx context.Context, q Querier) (Pragmas, error) {
	var pragmas Pragmas

	if err := q.QueryRowContext(ctx, `PRAGMA journal_mode`).Scan(&pragmas.JournalMode); err != nil {
		return Pragmas{}, fmt.Errorf("read journal_mode: %w", err)
	}
	if err := q.QueryRowContext(ctx, `PRAGMA foreign_keys`).Scan(&pragmas.ForeignKeys); err != nil {
		return Pragmas{}, fmt.Errorf("read foreign_keys: %w", err)
	}
	if err := q.QueryRowContext(ctx, `PRAGMA busy_timeout`).Scan(&pragmas.BusyTimeout); err != nil {
		return Pragmas{}, fmt.Errorf("read busy_timeout: %w", err)
	}
	if err := q.QueryRowContext(ctx, `PRAGMA synchronous`).Scan(&pragmas.Synchronous); err != nil {
		return Pragmas{}, fmt.Errorf("read synchronous: %w", err)
	}

	return pragmas, nil
}

// IntegrityCheck runs SQLite's own consistency check and returns its first
// line; a healthy database reports exactly "ok". Callers compare against that
// string rather than treating a nil error as health, because the check reports
// damage through its result set, not through an error.
func IntegrityCheck(ctx context.Context, q Querier) (string, error) {
	var result string
	if err := q.QueryRowContext(ctx, `PRAGMA integrity_check(1)`).Scan(&result); err != nil {
		return "", fmt.Errorf("run integrity_check: %w", err)
	}
	return result, nil
}
