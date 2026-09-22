// Package changes owns actual change discovery and attribution: the Change
// rows that answer "what actually changed" for a task, the baselines one
// before_change call replaces wholesale, and the operation log that replays
// retried creations (decision D-114).
//
// The facts live in the runtime database under migration 000006. Discovery
// funnels through one symbol-delta core whether the files came from a
// baseline or from Git, so per-row discovered_via may differ while file and
// symbol content cannot (decision D-118 and the AC-05.1 convergence rule).
package changes

import (
	"context"
	"database/sql"
	"errors"

	"github.com/PsyChaos/mindrail/internal/app"
)

// TableSchemaVersion is the migration that creates the changes,
// change_files, change_symbols, change_baselines and change_operations
// tables.
//
// It exists for the same reason every other store's version does: a reader
// of these tables must be able to tell "the schema does not hold them yet"
// apart from "they hold nothing", from the migration ledger, which is what
// the migrator itself is answerable for.
const TableSchemaVersion = 6

// Store owns only the change facts; Git spelling lives in internal/git and
// symbol facts in internal/index.
type Store struct {
	db    *sql.DB
	clock app.Clock
}

// NewStore builds a Store over an open runtime database handle.
func NewStore(db *sql.DB, clock app.Clock) (*Store, error) {
	if db == nil {
		return nil, errors.New("changes: needs a database handle")
	}
	return &Store{db: db, clock: clock}, nil
}

// requireSchema asks the migration ledger once per store operation, never per
// row. The store may survive `mindrail init`, so this cannot be cached at
// construction: a v5 database can become v6 while this Store is live.
func (s *Store) requireSchema(ctx context.Context) error {
	var applied sql.NullInt64
	if err := s.db.QueryRowContext(ctx, `SELECT max(version) FROM schema_migrations`).Scan(&applied); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return err
		}
		var tables int
		if probeErr := s.db.QueryRowContext(ctx, `SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name NOT GLOB 'sqlite_*'`).Scan(&tables); probeErr == nil && tables == 0 {
			return schemaBehind(0)
		}
		return corruptState(err)
	}
	if applied.Int64 < TableSchemaVersion {
		return schemaBehind(applied.Int64)
	}
	return nil
}
