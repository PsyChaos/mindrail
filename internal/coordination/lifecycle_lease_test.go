package coordination_test

import (
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/coordination"
	"github.com/PsyChaos/mindrail/internal/storage"
)

// activeTaskLease reads the task's unreleased lease row through FindLease,
// judged at the fixture clock, or reports none.
func activeTaskLease(t *testing.T, f fixture, taskID string) (coordination.Lease, bool) {
	t.Helper()
	var id string
	err := f.db.QueryRowContext(t.Context(),
		`SELECT lease_id FROM leases WHERE target_kind = 'task' AND target_key = ? AND released_at IS NULL`, taskID).Scan(&id)
	if err != nil {
		return coordination.Lease{}, false
	}
	lease := readLeaseRow(t, f, id)
	return lease, lease.Status == coordination.LeaseActive
}

// assertInvariant is decision D-68: an OPEN task has no active lease, and a
// task with an active lease has claimed_by equal to its holder.
func assertInvariant(t *testing.T, f fixture, taskID, where string) {
	t.Helper()
	task, err := f.store.FindTask(t.Context(), taskID)
	if err != nil {
		t.Fatalf("%s: FindTask = %v", where, err)
	}
	lease, active := activeTaskLease(t, f, taskID)
	if task.State == coordination.StateOpen && active {
		t.Errorf("%s: an OPEN task holds an active lease %s", where, lease.ID)
	}
	if active && task.ClaimedBy != lease.Holder {
		t.Errorf("%s: claimed_by = %q while the active lease is held by %q", where, task.ClaimedBy, lease.Holder)
	}
}

// TestEveryDestinationHasItsLeaseEffect is design §7's destination table,
// row by row, each asserted on the lease row and the task row afterwards.
func TestEveryDestinationHasItsLeaseEffect(t *testing.T) {
	t.Run("CLAIMED acquires and makes the mover the claimant", func(t *testing.T) {
		f, clock := leaseFixture(t)
		me := f.session(t)
		task := f.task(t, me.ID, "to be claimed")

		move, _, err := f.store.Transition(t.Context(), task.ID, coordination.NamedSession(me.ID), coordination.StateClaimed, "")
		if err != nil {
			t.Fatalf("Transition = %v", err)
		}
		if move.Lease == nil || move.Lease.Holder != me.ID || move.Lease.Status != coordination.LeaseActive || move.Superseded != nil {
			t.Fatalf("move.Lease = %+v, superseded %v; want an active lease held by the mover and nothing superseded", move.Lease, move.Superseded)
		}
		if move.Task.ClaimedBy != me.ID || move.Task.Revision != 2 {
			t.Errorf("task = claimed_by %q revision %d, want the mover and revision 2", move.Task.ClaimedBy, move.Task.Revision)
		}
		stored := readLeaseRow(t, f, move.Lease.ID)
		if stored.TargetKind != coordination.TargetTask || stored.TargetKey != task.ID || !stored.ExpiresAt.Equal(clock.Now().Add(coordination.LeaseTTL)) {
			t.Errorf("stored lease = %+v, want a task lease on %s expiring at now + TTL", stored, task.ID)
		}
		assertInvariant(t, f, task.ID, "after CLAIMED")
	})

	t.Run("a working move by the holder renews under the same id", func(t *testing.T) {
		f, clock := leaseFixture(t)
		me := f.session(t)
		task := f.taskIn(t, me.ID, coordination.StateClaimed)
		before, _ := activeTaskLease(t, f, task.ID)

		clock.Advance(5 * time.Minute)
		move, _, err := f.store.Transition(t.Context(), task.ID, coordination.NamedSession(me.ID), coordination.StateInProgress, "")
		if err != nil {
			t.Fatalf("Transition = %v", err)
		}
		if move.Lease == nil || move.Lease.ID != before.ID || move.Superseded != nil {
			t.Fatalf("move.Lease = %+v, want the same tenure %s renewed", move.Lease, before.ID)
		}
		if !move.Lease.ExpiresAt.Equal(clock.Now().Add(coordination.LeaseTTL)) {
			t.Errorf("expires_at = %v, want moved to now + TTL", move.Lease.ExpiresAt)
		}
		assertInvariant(t, f, task.ID, "after IN_PROGRESS")
	})

	t.Run("a working move over an expired lease takes it over and says so", func(t *testing.T) {
		f, clock := leaseFixture(t)
		crashed, next := f.session(t), f.session(t)
		task := f.taskIn(t, crashed.ID, coordination.StateInProgress)
		old, _ := activeTaskLease(t, f, task.ID)

		clock.Advance(coordination.LeaseTTL)
		move, _, err := f.store.Transition(t.Context(), task.ID, coordination.NamedSession(next.ID), coordination.StateBlocked, "waiting on a review")
		if err != nil {
			t.Fatalf("Transition over an expired lease = %v, want the takeover", err)
		}
		if move.Superseded == nil || move.Superseded.ID != old.ID || move.Superseded.Holder != crashed.ID {
			t.Fatalf("superseded = %+v, want the crashed session's tenure %s", move.Superseded, old.ID)
		}
		if move.Lease == nil || move.Lease.Holder != next.ID || move.Lease.ID == old.ID {
			t.Fatalf("move.Lease = %+v, want a new tenure held by the taker", move.Lease)
		}
		if move.Task.ClaimedBy != next.ID {
			t.Errorf("claimed_by = %q, want the taker %s: a takeover is a claim (D-68)", move.Task.ClaimedBy, next.ID)
		}
		closed := readLeaseRow(t, f, old.ID)
		if closed.Status != coordination.LeaseReleased || closed.ReleaseReason != coordination.ReleaseReasonExpired {
			t.Errorf("the old tenure is %s (%q), want released (expired)", closed.Status, closed.ReleaseReason)
		}
		assertInvariant(t, f, task.ID, "after the takeover")
	})

	t.Run("a working move on a task nobody holds acquires", func(t *testing.T) {
		f, _ := leaseFixture(t)
		me, next := f.session(t), f.session(t)
		task := f.taskIn(t, me.ID, coordination.StateInProgress)
		held, _ := activeTaskLease(t, f, task.ID)
		if _, _, err := f.store.ReleaseLease(t.Context(), held.ID, coordination.NamedSession(me.ID)); err != nil {
			t.Fatalf("ReleaseLease = %v", err)
		}

		move, _, err := f.store.Transition(t.Context(), task.ID, coordination.NamedSession(next.ID), coordination.StateReadyToComplete, "")
		if err != nil {
			t.Fatalf("Transition on a released task = %v, want an acquisition", err)
		}
		if move.Lease == nil || move.Lease.Holder != next.ID || move.Superseded != nil {
			t.Fatalf("move = %+v, want a fresh lease for the mover and nothing superseded", move)
		}
		if move.Task.ClaimedBy != next.ID {
			t.Errorf("claimed_by = %q, want %s", move.Task.ClaimedBy, next.ID)
		}
		assertInvariant(t, f, task.ID, "after acquiring a released task")
	})

	t.Run("OPEN releases and clears the claimant", func(t *testing.T) {
		f, _ := leaseFixture(t)
		me := f.session(t)
		task := f.taskIn(t, me.ID, coordination.StateClaimed)
		held, _ := activeTaskLease(t, f, task.ID)

		move, _, err := f.store.Transition(t.Context(), task.ID, coordination.NamedSession(me.ID), coordination.StateOpen, "")
		if err != nil {
			t.Fatalf("Transition to OPEN = %v", err)
		}
		if move.Lease != nil || move.Task.ClaimedBy != "" {
			t.Errorf("move = lease %+v claimed_by %q; want no lease and no claimant", move.Lease, move.Task.ClaimedBy)
		}
		closed := readLeaseRow(t, f, held.ID)
		if closed.Status != coordination.LeaseReleased || closed.ReleaseReason != coordination.ReleaseReasonReleased {
			t.Errorf("the tenure is %s (%q), want released (released)", closed.Status, closed.ReleaseReason)
		}
		assertInvariant(t, f, task.ID, "after OPEN")
	})

	for _, terminal := range []coordination.State{coordination.StateCompleted, coordination.StateAbandoned} {
		t.Run("a terminal move releases with reason finished and keeps the claimant: "+string(terminal), func(t *testing.T) {
			f, _ := leaseFixture(t)
			me := f.session(t)
			from := coordination.StateReadyToComplete
			if terminal == coordination.StateAbandoned {
				from = coordination.StateInProgress
			}
			task := f.taskIn(t, me.ID, from)
			held, _ := activeTaskLease(t, f, task.ID)

			move, _, err := f.store.Transition(t.Context(), task.ID, coordination.NamedSession(me.ID), terminal, "")
			if err != nil {
				t.Fatalf("Transition to %s = %v", terminal, err)
			}
			if move.Lease != nil || move.Task.ClaimedBy != me.ID {
				t.Errorf("move = lease %+v claimed_by %q; want no lease and the claimant kept as attribution", move.Lease, move.Task.ClaimedBy)
			}
			closed := readLeaseRow(t, f, held.ID)
			if closed.Status != coordination.LeaseReleased || closed.ReleaseReason != coordination.ReleaseReasonFinished {
				t.Errorf("the tenure is %s (%q), want released (finished)", closed.Status, closed.ReleaseReason)
			}
			assertInvariant(t, f, task.ID, "after "+string(terminal))
		})
	}
}

// TestAMoveOnAnotherSessionsHeldTaskIsRefusedForEveryDestination is AC-05.2:
// LEASE_CONFLICT before the state table, naming the holder, the lease and the
// expiry, for every destination including the ones the table would refuse
// anyway — and the row is unchanged, by re-reading it.
func TestAMoveOnAnotherSessionsHeldTaskIsRefusedForEveryDestination(t *testing.T) {
	f, _ := leaseFixture(t)
	holder, intruder := f.session(t), f.session(t)
	task := f.taskIn(t, holder.ID, coordination.StateInProgress)
	held, _ := activeTaskLease(t, f, task.ID)
	before, err := f.store.FindTask(t.Context(), task.ID)
	if err != nil {
		t.Fatal(err)
	}

	for _, to := range coordination.States() {
		_, _, err := f.store.Transition(t.Context(), task.ID, coordination.NamedSession(intruder.ID), to, "a reason, in case")
		payload := requireCode(t, err, app.CodeLeaseConflict)
		if !errors.Is(err, coordination.ErrLeaseConflict) {
			t.Errorf("-> %s: errors.Is(err, ErrLeaseConflict) = false", to)
		}
		if payload.Metadata["holder"] != holder.ID || payload.Metadata["lease_id"] != held.ID ||
			payload.Metadata["expires_at"] != app.FormatTime(held.ExpiresAt) {
			t.Errorf("-> %s: metadata = %v, want holder %s, lease %s, expiry %s", to, payload.Metadata, holder.ID, held.ID, app.FormatTime(held.ExpiresAt))
		}
		after, err := f.store.FindTask(t.Context(), task.ID)
		if err != nil {
			t.Fatal(err)
		}
		if after != before {
			t.Errorf("-> %s: a refused move changed the row:\n before %+v\n after  %+v", to, before, after)
		}
	}
}

// TestAStaleRevisionIsRefusedBeforeAnythingElse is AC-05.4 and AC-05.5
// (decision D-72): the expectation is judged first, the row is unchanged, and
// the revision rises by exactly one on every successful move and on nothing
// else.
func TestAStaleRevisionIsRefusedBeforeAnythingElse(t *testing.T) {
	f, _ := leaseFixture(t)
	me, other := f.session(t), f.session(t)
	task := f.task(t, me.ID, "a task read at revision 1")
	if task.Revision != 1 {
		t.Fatalf("OpenTask writes revision %d, want 1", task.Revision)
	}

	// Both agents read the task at revision 1. The first moves it.
	first, _, err := f.store.TransitionExpecting(t.Context(), task.ID, coordination.NamedSession(me.ID), coordination.StateClaimed, "", 1)
	if err != nil {
		t.Fatalf("TransitionExpecting(1) = %v, want the move", err)
	}
	if first.Task.Revision != 2 {
		t.Errorf("after one move revision = %d, want 2", first.Task.Revision)
	}

	// The second acts on its stale reading. It would also be LEASE_CONFLICT —
	// the first holds the lease — and the revision is judged first.
	_, _, err = f.store.TransitionExpecting(t.Context(), task.ID, coordination.NamedSession(other.ID), coordination.StateClaimed, "", 1)
	payload := requireCode(t, err, app.CodeStateRevisionConflict)
	if !errors.Is(err, coordination.ErrRevisionConflict) {
		t.Error("errors.Is(err, ErrRevisionConflict) = false")
	}
	if payload.Metadata["current_revision"] != "2" || payload.Metadata["expected_revision"] != "1" || payload.Metadata["state"] != string(coordination.StateClaimed) {
		t.Errorf("metadata = %v, want current 2, expected 1, state CLAIMED", payload.Metadata)
	}
	if !strings.Contains(strings.Join(payload.NextAction, " "), "task show "+task.ID) {
		t.Errorf("remedy %v does not send the caller to re-read the task", payload.NextAction)
	}
	stored, err := f.store.FindTask(t.Context(), task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Revision != 2 || stored.State != coordination.StateClaimed {
		t.Errorf("after the refusal the row is revision %d in %s, want 2 in CLAIMED", stored.Revision, stored.State)
	}

	// The same stale caller with the current revision, by the holder, moves.
	second, _, err := f.store.TransitionExpecting(t.Context(), task.ID, coordination.NamedSession(me.ID), coordination.StateInProgress, "", 2)
	if err != nil {
		t.Fatalf("TransitionExpecting(2) = %v, want the move", err)
	}
	if second.Task.Revision != 3 {
		t.Errorf("revision = %d, want 3", second.Task.Revision)
	}

	// Nothing else moves it: a checkpoint, a lease renewal, a refused move.
	if _, _, err := f.store.WriteCheckpoint(t.Context(), task.ID, coordination.NamedSession(me.ID), f.spaceID, "a note", false); err != nil {
		t.Fatal(err)
	}
	if _, _, err := f.store.Transition(t.Context(), task.ID, coordination.NamedSession(me.ID), coordination.StateCompleted, ""); err == nil {
		t.Fatal("IN_PROGRESS -> COMPLETED = nil error, want TASK_STATE_INVALID")
	}
	stored, err = f.store.FindTask(t.Context(), task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Revision != 3 {
		t.Errorf("revision = %d after a checkpoint and a refused move, want still 3", stored.Revision)
	}

	// Zero expects nothing.
	if _, _, err := f.store.TransitionExpecting(t.Context(), task.ID, coordination.NamedSession(me.ID), coordination.StateBlocked, "a reason", 0); err != nil {
		t.Errorf("TransitionExpecting(0) = %v, want no expectation judged", err)
	}
}

// TestTheInvariantHoldsAfterEveryLegalTransition is AC-05.1: D-55's thirteen
// legal pairs, each walked from a task driven to its starting state, under two
// starting conditions — the mover holds the lease, and the lease has expired
// and a second session moves — with decision D-68's invariant checked after
// each.
func TestTheInvariantHoldsAfterEveryLegalTransition(t *testing.T) {
	moves := 0
	for _, from := range coordination.States() {
		for _, to := range coordination.TransitionsFrom(from) {
			moves++
			t.Run("held: "+string(from)+" -> "+string(to), func(t *testing.T) {
				f, _ := leaseFixture(t)
				me := f.session(t)
				task := f.taskIn(t, me.ID, from)
				assertInvariant(t, f, task.ID, "before")
				if _, _, err := f.store.Transition(t.Context(), task.ID, coordination.NamedSession(me.ID), to, "a reason, in case"); err != nil {
					t.Fatalf("Transition = %v", err)
				}
				assertInvariant(t, f, task.ID, "after")
			})
			t.Run("expired: "+string(from)+" -> "+string(to), func(t *testing.T) {
				f, clock := leaseFixture(t)
				first, second := f.session(t), f.session(t)
				task := f.taskIn(t, first.ID, from)
				old, _ := activeTaskLease(t, f, task.ID) // the first session's tenure, about to expire
				clock.Advance(coordination.LeaseTTL + time.Minute)
				assertInvariant(t, f, task.ID, "before")
				move, _, err := f.store.Transition(t.Context(), task.ID, coordination.NamedSession(second.ID), to, "a reason, in case")
				if err != nil {
					t.Fatalf("Transition by a second session over an expired lease = %v", err)
				}
				if from != coordination.StateOpen && move.Lease != nil && move.Superseded == nil {
					t.Errorf("a move over an expired tenure into %s acquired without naming the tenure it superseded", to)
				}
				// Whichever way the expired tenure was closed — superseded by an
				// acquisition, or released on the way to OPEN or a terminal state
				// — its history says it expired.
				if from != coordination.StateOpen {
					if closed := readLeaseRow(t, f, old.ID); closed.Status != coordination.LeaseReleased || closed.ReleaseReason != coordination.ReleaseReasonExpired {
						t.Errorf("the expired tenure is %s (%q) after the move to %s, want released (expired)", closed.Status, closed.ReleaseReason, to)
					}
				}
				assertInvariant(t, f, task.ID, "after")
			})
		}
	}
	if moves != 13 {
		t.Errorf("walked %d legal pairs, want 13", moves)
	}
}

// TestTheTableIsUnchangedByTheLease is AC-05.7: every illegal pair is still
// TASK_STATE_INVALID when the mover holds the lease, so the lease changed no
// transition. The legal arm is TestEveryTransitionTheTableAllowsIsWritten.
func TestTheTableIsUnchangedByTheLease(t *testing.T) {
	f, _ := leaseFixture(t)
	me := f.session(t)
	refusals := 0
	for _, from := range coordination.States() {
		for _, to := range coordination.States() {
			if coordination.CanTransition(from, to) {
				continue
			}
			refusals++
			task := f.taskIn(t, me.ID, from)
			_, _, err := f.store.Transition(t.Context(), task.ID, coordination.NamedSession(me.ID), to, "a reason, in case")
			requireCode(t, err, app.CodeTaskStateInvalid)
		}
	}
	if refusals != 36 {
		t.Errorf("exercised %d refusals, want 36", refusals)
	}
}

// TestAHoldersCheckpointRenewsAndAHandoffReleases is AC-05.6 and decision
// D-78, in three arms: the holder's note renews, the holder's handoff closes
// with reason `handoff`, and a stranger's note leaves the lease row byte for
// byte as it was.
func TestAHoldersCheckpointRenewsAndAHandoffReleases(t *testing.T) {
	f, clock := leaseFixture(t)
	holder, stranger := f.session(t), f.session(t)
	task := f.taskIn(t, holder.ID, coordination.StateInProgress)
	held, _ := activeTaskLease(t, f, task.ID)

	clock.Advance(3 * time.Minute)
	noted, _, err := f.store.WriteCheckpoint(t.Context(), task.ID, coordination.NamedSession(holder.ID), f.spaceID, "halfway", false)
	if err != nil {
		t.Fatalf("WriteCheckpoint = %v", err)
	}
	if noted.Lease == nil || noted.Lease.ID != held.ID || !noted.Lease.ExpiresAt.Equal(clock.Now().Add(coordination.LeaseTTL)) {
		t.Fatalf("noted.Lease = %+v, want %s renewed to now + TTL", noted.Lease, held.ID)
	}

	renewed := readLeaseRow(t, f, held.ID)
	byStranger, _, err := f.store.WriteCheckpoint(t.Context(), task.ID, coordination.NamedSession(stranger.ID), f.spaceID, "picking this up soon", false)
	if err != nil {
		t.Fatalf("a stranger's WriteCheckpoint = %v, want it written", err)
	}
	if byStranger.Lease != nil {
		t.Errorf("a stranger's checkpoint reports a lease %+v, want none touched", byStranger.Lease)
	}
	if untouched := readLeaseRow(t, f, held.ID); !sameRow(renewed, untouched) {
		t.Errorf("a stranger's checkpoint changed the lease row:\n before %+v\n after  %+v", renewed, untouched)
	}

	clock.Advance(time.Minute)
	handoff, _, err := f.store.WriteCheckpoint(t.Context(), task.ID, coordination.NamedSession(holder.ID), f.spaceID, "leaving; take it", true)
	if err != nil {
		t.Fatalf("handoff WriteCheckpoint = %v", err)
	}
	if handoff.Lease == nil || handoff.Lease.Status != coordination.LeaseReleased || handoff.Lease.ReleaseReason != coordination.ReleaseReasonHandoff {
		t.Fatalf("handoff.Lease = %+v, want released with reason handoff", handoff.Lease)
	}
	if closed := readLeaseRow(t, f, held.ID); closed.ReleasedAt == nil || !closed.ReleasedAt.Equal(clock.Now()) {
		t.Errorf("released_at = %v, want the handoff's instant %v", closed.ReleasedAt, clock.Now())
	}

	// The handover, replayed: the task is IN_PROGRESS and has no move that
	// keeps it there, so the next session takes it where it stands (D-66 as
	// amended); nothing is superseded because the tenure was released, not
	// expired.
	next := f.session(t)
	taken, _, err := f.store.AcquireLease(t.Context(), f.projectID, coordination.NamedSession(next.ID), coordination.TaskTarget(task.ID))
	if err != nil {
		t.Fatalf("the next session's claim after a handoff = %v, want an acquisition", err)
	}
	if taken.Lease.Holder != next.ID || taken.Superseded != nil {
		t.Errorf("acquisition = %+v, want a fresh tenure for the next session and nothing superseded", taken)
	}
	assertInvariant(t, f, task.ID, "after the handover")

	// And its next move renews what it now holds.
	move, _, err := f.store.Transition(t.Context(), task.ID, coordination.NamedSession(next.ID), coordination.StateReadyToComplete, "")
	if err != nil {
		t.Fatalf("the next session's move after taking the task = %v", err)
	}
	if move.Lease == nil || move.Lease.ID != taken.Lease.ID {
		t.Errorf("move.Lease = %+v, want the tenure %s renewed", move.Lease, taken.Lease.ID)
	}
	assertInvariant(t, f, task.ID, "after the next session's move")
}

// TestACheckpointOnADamagedTaskRowIsRefused closes MR-003 §7's second LOW
// finding: the existence probe read only task_id, so a row `task show`
// refused as undecodable still took a note. The task is now read whole.
func TestACheckpointOnADamagedTaskRowIsRefused(t *testing.T) {
	f, _ := leaseFixture(t)
	me := f.session(t)
	task := f.task(t, me.ID, "about to be damaged")
	if _, err := f.db.ExecContext(t.Context(), `UPDATE tasks SET created_at = 'yesterday' WHERE task_id = ?`, task.ID); err != nil {
		t.Fatal(err)
	}

	_, _, err := f.store.WriteCheckpoint(t.Context(), task.ID, coordination.NamedSession(me.ID), f.spaceID, "a note", false)
	payload := requireCode(t, err, app.CodeCoordinationReadFailed)
	if payload.Metadata["subject_id"] != task.ID {
		t.Errorf("subject_id = %q, want the damaged task %s", payload.Metadata["subject_id"], task.ID)
	}
	var notes int
	if err := f.db.QueryRowContext(t.Context(), `SELECT count(*) FROM checkpoints WHERE task_id = ?`, task.ID).Scan(&notes); err != nil {
		t.Fatal(err)
	}
	if notes != 0 {
		t.Errorf("%d checkpoint(s) written on a task row that cannot be read", notes)
	}
}

// TestHandoverReportsTheNewestTenure is the read `task show` will render: the
// task's newest lease row in whatever status it is, so a claimant whose lease
// expired is shown with the tenure's end rather than as if the claim bound.
func TestHandoverReportsTheNewestTenure(t *testing.T) {
	f, clock := leaseFixture(t)
	me := f.session(t)
	fresh := f.task(t, me.ID, "never claimed")
	if handover, err := f.store.Handover(t.Context(), fresh.ID); err != nil || handover.Lease != nil {
		t.Fatalf("Handover(fresh) = %+v, %v; want no lease", handover, err)
	}

	task := f.taskIn(t, me.ID, coordination.StateInProgress)
	handover, err := f.store.Handover(t.Context(), task.ID)
	if err != nil || handover.Lease == nil || handover.Lease.Status != coordination.LeaseActive || handover.Lease.Holder != me.ID {
		t.Fatalf("Handover(held) = %+v, %v; want the active tenure", handover.Lease, err)
	}

	clock.Advance(coordination.LeaseTTL)
	handover, err = f.store.Handover(t.Context(), task.ID)
	if err != nil || handover.Lease == nil || handover.Lease.Status != coordination.LeaseExpired {
		t.Fatalf("Handover(expired) = %+v, %v; want the tenure reported expired", handover.Lease, err)
	}
	if handover.Task.ClaimedBy != me.ID {
		t.Errorf("claimed_by = %q after expiry, want the attribution kept", handover.Task.ClaimedBy)
	}
}

// TestTheClaimWithoutAMoveReportsTheTaskItClaimed is TASK-04's Breaker
// finding: the claim raised the revision and reported nothing, so the
// claimant's own next move under decision D-72, with the revision it had read
// before claiming, was refused. The acquisition now carries the task as the
// claim left it.
func TestTheClaimWithoutAMoveReportsTheTaskItClaimed(t *testing.T) {
	f, _ := leaseFixture(t)
	me, next := f.session(t), f.session(t)
	task := f.taskIn(t, me.ID, coordination.StateInProgress)
	if _, _, err := f.store.WriteCheckpoint(t.Context(), task.ID, coordination.NamedSession(me.ID), f.spaceID, "leaving", true); err != nil {
		t.Fatal(err)
	}
	read, err := f.store.FindTask(t.Context(), task.ID)
	if err != nil {
		t.Fatal(err)
	}

	taken, _, err := f.store.AcquireLease(t.Context(), f.projectID, coordination.NamedSession(next.ID), coordination.TaskTarget(task.ID))
	if err != nil {
		t.Fatalf("AcquireLease(task) = %v", err)
	}
	if taken.Task == nil {
		t.Fatal("the claim without a move reports no task; the claimant cannot know the revision it raised")
	}
	if taken.Task.Revision != read.Revision+1 || taken.Task.ClaimedBy != next.ID || taken.Task.State != read.State {
		t.Errorf("acquisition.Task = %+v, want revision %d, claimant %s, state unchanged", taken.Task, read.Revision+1, next.ID)
	}

	// The claimant's next move with the revision the claim reported succeeds;
	// with the one it read before claiming it is refused, which is D-72 doing
	// its job on a genuinely stale reading.
	if _, _, err := f.store.TransitionExpecting(t.Context(), task.ID, coordination.NamedSession(next.ID), coordination.StateReadyToComplete, "", read.Revision); err == nil {
		t.Error("a move expecting the pre-claim revision = nil error, want STATE_REVISION_CONFLICT")
	} else {
		requireCode(t, err, app.CodeStateRevisionConflict)
	}
	if _, _, err := f.store.TransitionExpecting(t.Context(), task.ID, coordination.NamedSession(next.ID), coordination.StateReadyToComplete, "", taken.Task.Revision); err != nil {
		t.Errorf("a move expecting the revision the claim reported = %v, want the move", err)
	}

	// A file acquisition carries no task.
	file, _, err := f.store.AcquireLease(t.Context(), f.projectID, coordination.NamedSession(next.ID), mustFile(t, "src/x.go"))
	if err != nil || file.Task != nil {
		t.Errorf("a file acquisition = %+v, %v; want no task", file, err)
	}
}

// TestACheckpointIsStampedUnderTheWriteLock is the same Breaker's second
// finding, in the shape TestAMoveIsStampedUnderTheWriteLock has: the row's
// stamp and the lease judgment must be about the instant the write holds the
// lock, not about an instant before the wait — a holder's note that waited on
// the lock renewed a tenure the in-lock clock had already expired.
func TestACheckpointIsStampedUnderTheWriteLock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mindrail.db")
	f := openFixture(t, path, app.FixedClock{Instant: baseInstant})

	probe, err := storage.Open(t.Context(), storage.Options{Path: path, BusyTimeout: time.Millisecond})
	if err != nil {
		t.Fatalf("opening the probe handle: %v", err)
	}
	t.Cleanup(func() { _ = probe.Close() })
	if tx, err := probe.BeginTx(t.Context(), nil); err != nil {
		t.Fatalf("the probe cannot begin a transaction on an idle database: %v", err)
	} else {
		_ = tx.Rollback()
	}

	clock := &lockProbingClock{probe: probe, next: baseInstant}
	store := coordination.NewStore(f.db.DB, clock)
	session := f.session(t)
	task := f.task(t, session.ID, "a task whose notes are stamped under the lock")

	if _, _, err := store.WriteCheckpoint(t.Context(), task.ID, coordination.NamedSession(session.ID), f.spaceID, "a note", false); err != nil {
		t.Fatalf("WriteCheckpoint = %v", err)
	}
	if len(clock.held) != 1 {
		t.Fatalf("WriteCheckpoint read the clock %d time(s), want exactly once", len(clock.held))
	}
	if !clock.held[0] {
		t.Errorf("WriteCheckpoint read the clock without holding the write lock; a note that waited on the lock would judge the lease at an instant before the wait")
	}
}

// TestANegativeExpectationIsRefused is the same Breaker's grammar finding:
// zero means no expectation, and below zero is a mistake the store refuses
// for the callers with no command line in front of them.
func TestANegativeExpectationIsRefused(t *testing.T) {
	f, _ := leaseFixture(t)
	me := f.session(t)
	task := f.task(t, me.ID, "a task")
	_, _, err := f.store.TransitionExpecting(t.Context(), task.ID, coordination.NamedSession(me.ID), coordination.StateClaimed, "", -1)
	requireCode(t, err, app.CodeCommandLineInvalid)
	if after, _ := f.store.FindTask(t.Context(), task.ID); after.Revision != 1 || after.State != coordination.StateOpen {
		t.Errorf("a refused expectation moved the task: %+v", after)
	}
}

// TestHandoverReadsTheTaskAndItsLeaseAsOneSnapshot is the same Breaker's
// third finding: read as two statements, a takeover committing between them
// showed a claimant beside another session's active lease — a D-68 violation
// the rows never had. The pair is one statement now; this drives takeovers
// against readers and asserts the pair never disagrees.
func TestHandoverReadsTheTaskAndItsLeaseAsOneSnapshot(t *testing.T) {
	f, clock := leaseFixture(t)
	first := f.session(t)
	task := f.taskIn(t, first.ID, coordination.StateInProgress)

	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		// Takeovers in a loop: expire the tenure, let a new session take the
		// task where it stands, repeat.
		for i := 0; ; i++ {
			select {
			case <-stop:
				return
			default:
			}
			clock.Advance(coordination.LeaseTTL + time.Second)
			next := f.session(t)
			if _, _, err := f.store.AcquireLease(t.Context(), f.projectID, coordination.NamedSession(next.ID), coordination.TaskTarget(task.ID)); err != nil {
				t.Errorf("takeover %d = %v", i, err)
				return
			}
		}
	}()

	mismatches := 0
	for range 4000 {
		handover, err := f.store.Handover(t.Context(), task.ID)
		if err != nil {
			t.Fatalf("Handover = %v", err)
		}
		if handover.Lease != nil && handover.Lease.Status == coordination.LeaseActive && handover.Lease.Holder != handover.Task.ClaimedBy {
			mismatches++
		}
	}
	close(stop)
	wg.Wait()
	if mismatches != 0 {
		t.Errorf("%d of 4000 handovers showed a claimant beside another session's active lease", mismatches)
	}
}
