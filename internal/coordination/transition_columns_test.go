package coordination_test

import (
	"testing"

	"github.com/PsyChaos/mindrail/internal/coordination"
)

// TestAnAcceptedMoveAdvancesUpdatedAt is finding F21's one that matters.
//
// updated_at is published on every task the CLI marshals, and the only
// assertion the column had was the refusal arm: a refused move must not rewrite
// it. A store that never wrote the column at all satisfied that identically, so
// `updated_at = updated_at` in the UPDATE left the suite green while the real
// binary reported the same timestamp after `task open`, after `--to CLAIMED`
// and after `--to IN_PROGRESS`.
//
// stepClock advances a fixed step per read, so "the column moved" is observable
// without sleeping and without depending on the resolution of a real clock.
func TestAnAcceptedMoveAdvancesUpdatedAt(t *testing.T) {
	f := newFixture(t)
	session := f.session(t)
	task := f.task(t, session.ID, "a task that will be moved twice")

	previous := task.UpdatedAt
	if previous.IsZero() {
		t.Fatal("the task was opened with no updated_at, so there is nothing to advance")
	}

	for _, to := range []coordination.State{coordination.StateClaimed, coordination.StateInProgress} {
		moved := f.move(t, task.ID, session.ID, to, "")
		if !moved.UpdatedAt.After(previous) {
			t.Errorf("moving to %s left updated_at at %s; an accepted move rewrites the row",
				to, moved.UpdatedAt)
		}

		// Read it back rather than trusting the returned value: the struct is
		// built in Go and the column is what a second process sees.
		stored, err := f.store.FindTask(t.Context(), task.ID)
		if err != nil {
			t.Fatalf("FindTask: %v", err)
		}
		if !stored.UpdatedAt.Equal(moved.UpdatedAt) {
			t.Errorf("the row says updated_at is %s and the answer said %s",
				stored.UpdatedAt, moved.UpdatedAt)
		}
		previous = stored.UpdatedAt
	}
}

// TestAbandoningATaskKeepsItsClaimant is the other survivor.
//
// The claim is recorded on the way in and released on the way out, and OPEN is
// the release path (decision D-55). ABANDONED is not: a task an agent claimed
// and gave up on still records which agent had it, which is the only thing left
// to answer "who was doing this" once the work has stopped. Adding ABANDONED to
// the clearing branch left the suite green.
func TestAbandoningATaskKeepsItsClaimant(t *testing.T) {
	f := newFixture(t)
	session := f.session(t)
	task := f.task(t, session.ID, "a task that will be claimed and given up")

	claimed := f.move(t, task.ID, session.ID, coordination.StateClaimed, "")
	if claimed.ClaimedBy != session.ID {
		t.Fatalf("claimed_by = %q after a claim, want %q", claimed.ClaimedBy, session.ID)
	}

	abandoned := f.move(t, task.ID, session.ID, coordination.StateAbandoned, "")
	if abandoned.ClaimedBy != session.ID {
		t.Errorf("claimed_by = %q after abandoning, want the session that claimed it, %q; "+
			"ABANDONED is not the release path, OPEN is", abandoned.ClaimedBy, session.ID)
	}

	stored, err := f.store.FindTask(t.Context(), task.ID)
	if err != nil {
		t.Fatalf("FindTask: %v", err)
	}
	if stored.ClaimedBy != session.ID {
		t.Errorf("the row says claimed_by is %q, want %q", stored.ClaimedBy, session.ID)
	}

	// The over-fire arm: the release path really does clear it, or the
	// assertion above would pass for a store that never cleared the column.
	released := f.task(t, session.ID, "a task that will be claimed and released")
	f.move(t, released.ID, session.ID, coordination.StateClaimed, "")
	back := f.move(t, released.ID, session.ID, coordination.StateOpen, "")
	if back.ClaimedBy != "" {
		t.Errorf("claimed_by = %q after a release, want it cleared", back.ClaimedBy)
	}
}
