package coordination

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/identity"
	"github.com/PsyChaos/mindrail/internal/storage"
)

// The lease methods of Store. They sit in their own file for length, not for
// package: the task lease is written in the transaction that moves the task
// (decision D-64), so these and Transition share a *sql.Tx and a package.

// AcquireLease takes a target for the session the write is attributed to, or
// says who holds it.
//
// Design §6's table, row by row: no lease — insert; active and held by this
// session — renew it under the same id; active and held by another —
// LEASE_CONFLICT naming the holder and the expiry; expired but unreleased —
// close it with reason `expired` and insert, naming the tenure superseded;
// released — insert. "Active" is judged against the clock read inside the
// transaction, under the write lock, so the judgment and the write are about
// the same instant (decision D-65).
//
// A task target is not acquired here. A task lease is taken by moving the task
// (decision D-66), and the one hook the lease has into the lifecycle is
// Transition; this method refuses a task target so that a second entry point
// cannot grow.
func (s *Store) AcquireLease(ctx context.Context, projectID string, by Attribution, target Target) (Acquisition, Write, error) {
	if target.Kind != TargetFile {
		return Acquisition{}, Write{}, app.NewError(
			app.CodeCommandLineInvalid,
			app.KindUsage,
			fmt.Sprintf("a %s lease is not acquired directly", target.Kind),
			"Nothing was read and nothing was written.",
			"Move the task with `mindrail task state <task-id> --to <STATE>`; the move takes the lease.",
		).WithMetadata("target_kind", string(target.Kind)).WithMetadata("target_key", target.Key)
	}
	if projectID == "" {
		return Acquisition{}, Write{}, noProject("a lease")
	}

	var (
		result Acquisition
		write  Write
	)
	stats, err := storage.InTxMeasured(ctx, s.db, func(ctx context.Context, tx *sql.Tx) error {
		now := s.clock.Now().UTC()

		resolved, err := s.attribute(ctx, tx, by, now)
		if err != nil {
			return err
		}
		write = resolved

		acquired, err := s.acquireIn(ctx, tx, projectID, resolved.Session.ID, target, now)
		if err != nil {
			return err
		}
		result = acquired
		return nil
	})
	if err != nil {
		return Acquisition{}, Write{}, s.adopt(ctx, "the lease", target.String(), err)
	}
	write.Timing = stats
	return result, write, nil
}

// acquireIn is the acquisition itself, inside a transaction the caller owns.
// Transition calls it for a task target with its own transaction and clock
// reading, which is what keeps the lease rule in one place.
func (s *Store) acquireIn(ctx context.Context, tx *sql.Tx, projectID, holder string, target Target, now time.Time) (Acquisition, error) {
	current, found, err := unreleasedLeaseOn(ctx, tx, projectID, target, now)
	if err != nil {
		return Acquisition{}, err
	}

	var superseded *Lease
	if found {
		switch current.Status {
		case LeaseActive:
			if current.Holder != holder {
				return Acquisition{}, leaseConflict(current)
			}
			renewed, err := renewIn(ctx, tx, current, now)
			if err != nil {
				return Acquisition{}, err
			}
			return Acquisition{Lease: renewed, Renewed: true}, nil
		case LeaseExpired:
			closed, err := closeIn(ctx, tx, current, now, ReleaseReasonExpired)
			if err != nil {
				return Acquisition{}, err
			}
			superseded = &closed
		}
	}

	lease := Lease{
		ID:         identity.NewID(leaseIDPrefix),
		ProjectID:  projectID,
		TargetKind: target.Kind,
		TargetKey:  target.Key,
		Holder:     holder,
		AcquiredAt: now,
		RenewedAt:  now,
		ExpiresAt:  now.Add(LeaseTTL),
		Status:     LeaseActive,
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO leases (lease_id, project_id, target_kind, target_key, holder,
			acquired_at, renewed_at, expires_at, released_at, release_reason)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, NULL, NULL)`,
		lease.ID, lease.ProjectID, string(lease.TargetKind), lease.TargetKey, lease.Holder,
		app.FormatTime(lease.AcquiredAt), app.FormatTime(lease.RenewedAt), app.FormatTime(lease.ExpiresAt)); err != nil {
		return Acquisition{}, err
	}
	return Acquisition{Lease: lease, Superseded: superseded}, nil
}

// RenewLease moves a held lease's expiry forward by LeaseTTL from now. Only
// the holder may; anyone else meets the same LEASE_CONFLICT an acquisition
// would, and a lease that has expired or been released is LEASE_NOT_HELD —
// the caller's next action is to acquire again, not to wait.
func (s *Store) RenewLease(ctx context.Context, leaseID string, by Attribution) (Lease, Write, error) {
	return s.holderWrite(ctx, leaseID, by, "renew", func(ctx context.Context, tx *sql.Tx, lease Lease, now time.Time) (Lease, error) {
		return renewIn(ctx, tx, lease, now)
	})
}

// ReleaseLease closes a held lease with reason `released`. Holder only, as
// RenewLease; an expired or released lease is LEASE_NOT_HELD rather than
// silently re-released, because the history would then say the holder gave up
// a tenure that had already ended without it.
func (s *Store) ReleaseLease(ctx context.Context, leaseID string, by Attribution) (Lease, Write, error) {
	return s.holderWrite(ctx, leaseID, by, "release", func(ctx context.Context, tx *sql.Tx, lease Lease, now time.Time) (Lease, error) {
		return closeIn(ctx, tx, lease, now, ReleaseReasonReleased)
	})
}

// holderWrite is what RenewLease and ReleaseLease share: one transaction, the
// clock read under the lock, the session resolved, the lease found and judged,
// and the write only for the session that holds it.
func (s *Store) holderWrite(ctx context.Context, leaseID string, by Attribution, verb string,
	apply func(context.Context, *sql.Tx, Lease, time.Time) (Lease, error)) (Lease, Write, error) {

	var (
		result Lease
		write  Write
	)
	stats, err := storage.InTxMeasured(ctx, s.db, func(ctx context.Context, tx *sql.Tx) error {
		now := s.clock.Now().UTC()

		resolved, err := s.attribute(ctx, tx, by, now)
		if err != nil {
			return err
		}
		write = resolved

		lease, err := findLeaseIn(ctx, tx, leaseID, now)
		if err != nil {
			return err
		}
		switch lease.Status {
		case LeaseExpired, LeaseReleased:
			return leaseNotHeld(lease, verb)
		}
		if lease.Holder != resolved.Session.ID {
			return leaseConflict(lease)
		}

		applied, err := apply(ctx, tx, lease, now)
		if err != nil {
			return err
		}
		result = applied
		return nil
	})
	if err != nil {
		return Lease{}, Write{}, s.adopt(ctx, "the lease", leaseID, err)
	}
	write.Timing = stats
	return result, write, nil
}

// renewIn moves the expiry forward on an active row and returns it as written.
//
// Forward only. A renewal computed from a clock that stepped back would
// shorten the tenure and open an early takeover (TASK-03's Breaker); a tenure
// that already runs later than now + TTL keeps its expiry, and the renewal is
// recorded in renewed_at alone.
func renewIn(ctx context.Context, tx *sql.Tx, lease Lease, now time.Time) (Lease, error) {
	lease.RenewedAt = now
	if until := now.Add(LeaseTTL); until.After(lease.ExpiresAt) {
		lease.ExpiresAt = until
	}
	lease.Status = LeaseActive
	_, err := tx.ExecContext(ctx,
		`UPDATE leases SET renewed_at = ?, expires_at = ? WHERE lease_id = ? AND released_at IS NULL`,
		app.FormatTime(lease.RenewedAt), app.FormatTime(lease.ExpiresAt), lease.ID)
	return lease, err
}

// closeIn ends a tenure with the reason given and returns the row as written.
func closeIn(ctx context.Context, tx *sql.Tx, lease Lease, now time.Time, reason string) (Lease, error) {
	released := now
	lease.ReleasedAt = &released
	lease.ReleaseReason = reason
	lease.Status = LeaseReleased
	_, err := tx.ExecContext(ctx,
		`UPDATE leases SET released_at = ?, release_reason = ? WHERE lease_id = ? AND released_at IS NULL`,
		app.FormatTime(released), reason, lease.ID)
	return lease, err
}

// FindLease returns a lease by id, in whatever status it is at now, or
// ErrLeaseNotFound.
func (s *Store) FindLease(ctx context.Context, leaseID string) (Lease, error) {
	lease, err := scanLease(s.db.QueryRowContext(ctx, selectLease+` WHERE lease_id = ?`, leaseID), s.clock.Now().UTC())
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return Lease{}, leaseNotFound(leaseID)
	case err != nil:
		return Lease{}, readFailed("the lease", leaseID, err)
	}
	return lease, nil
}

// ListLeases returns a project's active leases, oldest acquisition first.
//
// Active is judged against the clock at read time and nothing is marked
// (decision D-79): an expired row is left for its next acquirer to close, and
// is simply not listed. The order is the row order, which is the acquisition
// order — the id is minted per process and two processes' ids do not order
// (decision D-59's lesson).
func (s *Store) ListLeases(ctx context.Context, projectID string) ([]Lease, error) {
	now := s.clock.Now().UTC()
	rows, err := s.db.QueryContext(ctx,
		selectLease+` WHERE project_id = ? AND released_at IS NULL ORDER BY rowid`, projectID)
	if err != nil {
		return nil, readFailed("this project's leases", projectID, err)
	}
	defer func() { _ = rows.Close() }()

	leases := make([]Lease, 0, 4)
	for rows.Next() {
		lease, scanErr := scanLease(rows, now)
		if scanErr != nil {
			return nil, readFailed("this project's leases", projectID, scanErr)
		}
		if lease.Status == LeaseActive {
			leases = append(leases, lease)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, readFailed("this project's leases", projectID, err)
	}
	return leases, nil
}

// unreleasedLeaseOn reads the target's unreleased row, if there is one, judged
// at now. The partial unique index guarantees there is at most one.
func unreleasedLeaseOn(ctx context.Context, tx *sql.Tx, projectID string, target Target, now time.Time) (Lease, bool, error) {
	lease, err := scanLease(tx.QueryRowContext(ctx,
		selectLease+` WHERE project_id = ? AND target_kind = ? AND target_key = ? AND released_at IS NULL`,
		projectID, string(target.Kind), target.Key), now)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return Lease{}, false, nil
	case err != nil:
		return Lease{}, false, readFailed("the lease on "+target.String(), target.Key, err)
	}
	return lease, true, nil
}

// findLeaseIn reads a lease by id inside a write transaction, or refuses by
// name. A row it finds and cannot decode is a read failure naming the lease,
// by the rule of audit round 2 §4.7.
func findLeaseIn(ctx context.Context, tx *sql.Tx, leaseID string, now time.Time) (Lease, error) {
	lease, err := scanLease(tx.QueryRowContext(ctx, selectLease+` WHERE lease_id = ?`, leaseID), now)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return Lease{}, leaseNotFound(leaseID)
	case err != nil:
		return Lease{}, readFailed("the lease", leaseID, err)
	}
	return lease, nil
}

const selectLease = `SELECT lease_id, project_id, target_kind, target_key, holder,
	acquired_at, renewed_at, expires_at, released_at, release_reason FROM leases`

// scanLease decodes a row and judges its status at now.
func scanLease(row rowScanner, now time.Time) (Lease, error) {
	var (
		lease      Lease
		kind       string
		acquiredAt string
		renewedAt  string
		expiresAt  string
		releasedAt sql.NullString
		reason     sql.NullString
	)
	if err := row.Scan(&lease.ID, &lease.ProjectID, &kind, &lease.TargetKey, &lease.Holder,
		&acquiredAt, &renewedAt, &expiresAt, &releasedAt, &reason); err != nil {
		return Lease{}, err
	}
	lease.TargetKind = TargetKind(kind)
	lease.ReleaseReason = reason.String

	var err error
	if lease.AcquiredAt, err = app.ParseTime(acquiredAt); err != nil {
		return Lease{}, fmt.Errorf("lease %s acquired_at: %w", lease.ID, err)
	}
	if lease.RenewedAt, err = app.ParseTime(renewedAt); err != nil {
		return Lease{}, fmt.Errorf("lease %s renewed_at: %w", lease.ID, err)
	}
	if lease.ExpiresAt, err = app.ParseTime(expiresAt); err != nil {
		return Lease{}, fmt.Errorf("lease %s expires_at: %w", lease.ID, err)
	}
	if releasedAt.Valid {
		released, err := app.ParseTime(releasedAt.String)
		if err != nil {
			return Lease{}, fmt.Errorf("lease %s released_at: %w", lease.ID, err)
		}
		lease.ReleasedAt = &released
	}
	lease.Status = lease.statusAt(now)
	return lease, nil
}
