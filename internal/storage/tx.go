package storage

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// txKeyType is unexported so no other package can plant or clear the flag; the
// only way into a transaction-marked context is through InTx.
type txKeyType struct{}

var txKey txKeyType

// TxStats is what one write transaction cost: how long the caller waited for
// the write lock, and how long the transaction held it from BEGIN to COMMIT.
//
// Spec §11 names "p95 write wait" and "write transaction duration" as the two
// numbers that decide whether a repository-local write broker is ever needed,
// and kernel-scope §3 asks for SQLite wait to be collected from the first
// implementation. The store carries them on every Write; the wire shape they
// are published in is MR-019's (decision D-75).
//
// Waited is measured from the call to the lock, so it includes any time
// database/sql spent handing out a pooled connection — which is nothing for
// the one-goroutine command line, and is the wait an interactive writer
// actually experiences when it is not. A test that reads it under eight
// goroutines and a pool of four is reading pool queueing as well as SQLite's
// (TASK-01's Breaker measured 300 ms of it against a 50 ms busy budget).
type TxStats struct {
	Waited time.Duration
	Held   time.Duration
}

// beginBusy is the error InTx returns when BEGIN IMMEDIATE was refused as busy
// after waiting for the write lock. It carries how long, so that busyFailure
// can publish the wait rather than a guess at the budget; the driver's error
// stays in the chain so IsBusy still answers.
type beginBusy struct {
	waited time.Duration
	err    error
}

func (e *beginBusy) Error() string {
	return fmt.Sprintf("begin transaction: waited %s for the write lock: %v", e.waited.Round(time.Millisecond), e.err)
}

func (e *beginBusy) Unwrap() error { return e.err }

// InTx runs fn inside one short write transaction and commits it if fn returns
// nil. The connection is opened with _txlock=immediate, so BEGIN takes the
// write lock straight away: a writer that discovers contention at BEGIN can
// simply wait out busy_timeout, whereas one that discovers it at COMMIT has to
// discard work it already did (decision D-24).
//
// There is no retry of BEGIN here. SQLite's busy handler runs for the write
// lock from a fresh BEGIN IMMEDIATE, so a SQLITE_BUSY that reaches this
// function has already waited the whole busy_timeout, and a ladder on top of
// it would wait the budget again per step — past the shutdown budget D-08
// fixed at the same five seconds (decision D-74). What the refusal gains is the
// wait it carries.
//
// A panic rolls back and keeps travelling. Swallowing it would leave the
// caller believing a half-applied transaction succeeded.
func InTx(ctx context.Context, db *sql.DB, fn func(context.Context, *sql.Tx) error) error {
	_, err := InTxMeasured(ctx, db, fn)
	return err
}

// InTxMeasured is InTx with the transaction's cost returned beside its
// outcome. The stats are a value, not a pointer planted in the context: the
// first shape of this seam shared one *TxStats through a context, and two
// goroutines running InTx under one context raced on it (TASK-01's Breaker,
// under -race). A value per call cannot be shared by accident.
//
// A transaction that did not commit reports zero stats. There is no Write to
// carry them on, and a wait figure for a write that did not land would be the
// write-wait figure of nothing.
func InTxMeasured(ctx context.Context, db *sql.DB, fn func(context.Context, *sql.Tx) error) (TxStats, error) {
	began := time.Now()
	tx, err := db.BeginTx(ctx, nil)
	waited := time.Since(began)
	if err != nil {
		if isBusyError(err) {
			return TxStats{}, &beginBusy{waited: waited, err: err}
		}
		return TxStats{}, fmt.Errorf("begin transaction: %w", err)
	}

	txCtx := context.WithValue(ctx, txKey, struct{}{})

	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	if err := fn(txCtx, tx); err != nil {
		return TxStats{}, err
	}

	if err := tx.Commit(); err != nil {
		return TxStats{}, fmt.Errorf("commit transaction: %w", err)
	}
	committed = true

	return TxStats{Waited: waited, Held: time.Since(began) - waited}, nil
}

// TxActive reports whether ctx was produced by InTx. It backs the tech-stack
// §21 rule that no long-running work -- a git subprocess, a filesystem walk --
// runs while a write transaction holds the SQLite write lock: the probes
// assert this is false before they start, which turns "we were careful" into
// something a test can fail on.
func TxActive(ctx context.Context) bool {
	return ctx.Value(txKey) != nil
}
