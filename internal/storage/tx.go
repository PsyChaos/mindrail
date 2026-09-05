package storage

import (
	"context"
	"database/sql"
	"fmt"
)

// txKeyType is unexported so no other package can plant or clear the flag; the
// only way into a transaction-marked context is through InTx.
type txKeyType struct{}

var txKey txKeyType

// InTx runs fn inside one short write transaction and commits it if fn returns
// nil. The connection is opened with _txlock=immediate, so BEGIN takes the
// write lock straight away: a writer that discovers contention at BEGIN can
// simply wait out busy_timeout, whereas one that discovers it at COMMIT has to
// discard work it already did (decision D-24).
//
// A panic rolls back and keeps travelling. Swallowing it would leave the
// caller believing a half-applied transaction succeeded.
func InTx(ctx context.Context, db *sql.DB, fn func(context.Context, *sql.Tx) error) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}

	txCtx := context.WithValue(ctx, txKey, struct{}{})

	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	if err := fn(txCtx, tx); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}
	committed = true

	return nil
}

// TxActive reports whether ctx was produced by InTx. It backs the tech-stack
// §21 rule that no long-running work -- a git subprocess, a filesystem walk --
// runs while a write transaction holds the SQLite write lock: the probes
// assert this is false before they start, which turns "we were careful" into
// something a test can fail on.
func TxActive(ctx context.Context) bool {
	return ctx.Value(txKey) != nil
}
