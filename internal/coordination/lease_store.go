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
// A task target is the claim without a move (decision D-66 as amended in
// TASK-04): a task already in a working state — CLAIMED, IN_PROGRESS,
// BLOCKED or READY_TO_COMPLETE — is taken where it stands, its claimant
// becomes the holder and its revision rises, and its state does not change.
// The handover the design describes needs it: an agent that handed off an
// IN_PROGRESS task released the lease, and the next agent has no move that
// keeps the task IN_PROGRESS to take it with. An OPEN task is refused — its
// claim is the move to CLAIMED, and OPEN has no active lease by decision
// D-68 — and so is a task in a terminal state. Both paths run acquireIn and
// write claimed_by in the same transaction, which is what keeps D-68 true.
func (s *Store) AcquireLease(ctx context.Context, projectID string, by Attribution, target Target) (Acquisition, Write, error) {
	if projectID == "" {
		return Acquisition{}, Write{}, noProject("a lease")
	}
	if _, err := ParseTargetKind(string(target.Kind)); err != nil {
		return Acquisition{}, Write{}, app.NewError(
			app.CodeCommandLineInvalid,
			app.KindUsage,
			err.Error(),
			"Nothing was read and nothing was written.",
			"Pass --file with a repository-relative path, or --task with a task id.",
		).WithMetadata("target_kind", string(target.Kind)).WithMetadata("target_key", target.Key)
	}

	if err := s.refuseInvalidOperation(); err != nil {
		return Acquisition{}, Write{}, err
	}
	hash, err := requestHash("lease acquire", struct {
		Project string     `json:"project"`
		By      string     `json:"by"`
		Kind    TargetKind `json:"kind"`
		Key     string     `json:"key"`
	}{projectID, attributionKey(by), target.Kind, target.Key})
	if err != nil {
		return Acquisition{}, Write{}, err
	}

	var (
		result Acquisition
		write  Write
	)
	stats, err := storage.InTxMeasured(ctx, s.db, func(ctx context.Context, tx *sql.Tx) error {
		if replayed, found, err := s.replay(ctx, tx, "lease acquire", hash, &result); err != nil {
			return err
		} else if found {
			write = replayed
			return nil
		}

		now := s.clock.Now().UTC()

		resolved, err := s.attribute(ctx, tx, by, now)
		if err != nil {
			return err
		}
		write = resolved
		write.OperationID = s.op.ID

		if target.Kind == TargetTask {
			acquired, err := s.claimWhereItStands(ctx, tx, projectID, resolved.Session.ID, target.Key, now)
			if err != nil {
				return err
			}
			result = acquired
		} else {
			acquired, err := s.acquireIn(ctx, tx, projectID, resolved.Session.ID, target, now)
			if err != nil {
				return err
			}
			result = acquired
		}
		return s.record(ctx, tx, "lease acquire", hash, write, result, now)
	})
	if err != nil {
		return Acquisition{}, Write{}, s.adopt(ctx, "the lease", target.String(), err)
	}
	write.Timing = stats
	return result, write, nil
}

// claimWhereItStands takes a task's lease without moving the task, for a task
// that is in a working state, and makes the holder the claimant. The task row
// is updated — claimed_by, updated_at, and the revision, which rises on every
// update of the row (decision D-72).
func (s *Store) claimWhereItStands(ctx context.Context, tx *sql.Tx, projectID, holder, taskID string, now time.Time) (Acquisition, error) {
	task, err := scanTask(tx.QueryRowContext(ctx, selectTask+` WHERE task_id = ? AND project_id = ?`, taskID, projectID))
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return Acquisition{}, taskNotFound(taskID)
	case err != nil:
		return Acquisition{}, readFailed("the task", taskID, err)
	}
	if task.State == StateOpen || len(TransitionsFrom(task.State)) == 0 {
		return Acquisition{}, taskNotClaimableWhereItStands(task)
	}

	acquired, err := s.acquireIn(ctx, tx, projectID, holder, TaskTarget(taskID), now)
	if err != nil {
		return Acquisition{}, err
	}
	if acquired.Renewed {
		// The holder already held it; the row's attribution is already right,
		// and the task is reported as it stands.
		acquired.Task = &task
		return acquired, nil
	}

	result, err := tx.ExecContext(ctx,
		`UPDATE tasks SET claimed_by = ?, updated_at = ?, revision = revision + 1
		 WHERE task_id = ? AND revision = ?`,
		holder, app.FormatTime(now), taskID, task.Revision)
	if err != nil {
		return Acquisition{}, err
	}
	if affected, err := result.RowsAffected(); err != nil {
		return Acquisition{}, err
	} else if affected != 1 {
		return Acquisition{}, fmt.Errorf("the task's revision moved under the write lock: %d rows updated at revision %d", affected, task.Revision)
	}

	claimed := task
	claimed.ClaimedBy = holder
	claimed.UpdatedAt = now
	claimed.Revision = task.Revision + 1
	acquired.Task = &claimed
	return acquired, nil
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

	if err := s.refuseInvalidOperation(); err != nil {
		return Lease{}, Write{}, err
	}
	command := "lease " + verb
	hash, err := requestHash(command, struct {
		Lease string `json:"lease"`
		By    string `json:"by"`
	}{leaseID, attributionKey(by)})
	if err != nil {
		return Lease{}, Write{}, err
	}

	var (
		result Lease
		write  Write
	)
	stats, err := storage.InTxMeasured(ctx, s.db, func(ctx context.Context, tx *sql.Tx) error {
		if replayed, found, err := s.replay(ctx, tx, command, hash, &result); err != nil {
			return err
		} else if found {
			write = replayed
			return nil
		}

		now := s.clock.Now().UTC()

		resolved, err := s.attribute(ctx, tx, by, now)
		if err != nil {
			return err
		}
		write = resolved
		write.OperationID = s.op.ID

		lease, err := findLeaseIn(ctx, tx, leaseID, now)
		if err != nil {
			return err
		}
		switch lease.Status {
		case LeaseExpired, LeaseReleased:
			return leaseNotHeld(lease, verb, resolved.Session.ID)
		}
		if lease.Holder != resolved.Session.ID {
			return leaseConflict(lease)
		}

		applied, err := apply(ctx, tx, lease, now)
		if err != nil {
			return err
		}
		result = applied
		return s.record(ctx, tx, command, hash, write, result, now)
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
