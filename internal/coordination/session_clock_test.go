package coordination_test

import (
	"testing"

	"github.com/PsyChaos/mindrail/internal/coordination"
)

// TestAMintedSessionStartsNoLaterThanTheRowItAttributes is audit round 2,
// §4.12.
//
// Every writer stamps its own row from a clock reading taken before the
// transaction, and attribute used to take a second reading inside it for the
// session it minted — so a session started after the task it opened, by
// however long the two readings were apart. Under stepClock that is a full
// minute, which is what makes the inversion assertable without sleeping: the
// clock advances on every read, so a store that reads twice cannot produce two
// equal timestamps.
//
// The third writer is asserted with the other two although it never inverted:
// its clock reading came after the mint, so the session started before the
// move. It is here so the rule reads the same on all three, and so a reorder
// that put the reading first cannot pass.
func TestAMintedSessionStartsNoLaterThanTheRowItAttributes(t *testing.T) {
	f := newFixture(t)
	mint := coordination.MintFor(f.spaceID)

	task, opened, err := f.store.OpenTask(t.Context(), f.projectID, mint, "a task opened under a minted session")
	if err != nil {
		t.Fatalf("OpenTask = %v, want no error", err)
	}
	if !opened.Minted {
		t.Fatal("OpenTask under MintFor reports no mint; the assertion below would be about a session nothing minted")
	}
	if opened.Session.StartedAt.After(task.CreatedAt) {
		t.Errorf("the session started at %s, after the task it opened was created at %s",
			opened.Session.StartedAt, task.CreatedAt)
	}

	written, noted, err := f.store.WriteCheckpoint(t.Context(), task.ID, mint, f.spaceID, "a note under a minted session", false)
	if err != nil {
		t.Fatalf("WriteCheckpoint = %v, want no error", err)
	}
	if noted.Session.StartedAt.After(written.Checkpoint.CreatedAt) {
		t.Errorf("the session started at %s, after the checkpoint it wrote was created at %s",
			noted.Session.StartedAt, written.Checkpoint.CreatedAt)
	}

	moved, mover, err := f.store.Transition(t.Context(), task.ID, mint, coordination.StateClaimed, "")
	if err != nil {
		t.Fatalf("Transition = %v, want no error", err)
	}
	if mover.Session.StartedAt.After(moved.Task.UpdatedAt) {
		t.Errorf("the session started at %s, after the move it made was stamped at %s",
			mover.Session.StartedAt, moved.Task.UpdatedAt)
	}

	// The same question put to the rows rather than to the returned values,
	// in the words the audit used. The comparison is textual; it is sound here
	// because stepClock's instants are whole minutes and FormatTime renders
	// every one of them at the same length.
	for _, query := range []string{
		`SELECT count(*) FROM tasks t JOIN sessions s ON s.session_id = t.opened_by
		  WHERE s.started_at > t.created_at`,
		`SELECT count(*) FROM checkpoints c JOIN sessions s ON s.session_id = c.session_id
		  WHERE s.started_at > c.created_at`,
	} {
		var inverted int
		if err := f.db.QueryRowContext(t.Context(), query).Scan(&inverted); err != nil {
			t.Fatalf("%s = %v, want no error", query, err)
		}
		if inverted != 0 {
			t.Errorf("%d row(s) are attributed to a session that started after them", inverted)
		}
	}
}
