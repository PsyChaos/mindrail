package coordination_test

import (
	"path/filepath"
	"testing"

	"github.com/PsyChaos/mindrail/internal/app"
)

// TestTheNewestCheckpointIsTheOneWrittenLast is finding F37.
//
// "The newest checkpoint" was a lexical comparison of minted ids. The id's
// 48-bit millisecond prefix is monotonic within one process, because the bits
// under the prefix are a per-process counter — but between two processes
// writing in the same millisecond they are random, so the two ids order by 80
// random bits. Two agents handing over in one repository is the case this
// milestone exists for, and MR-003's own concurrency test proved the writes
// serialise correctly. What was undecided was the read that says which of them
// was last: a coin flip, half the time returning the note the departing agent
// had already superseded.
//
// The fixture is the shape two processes in one millisecond produce and not a
// story about them: two checkpoints inserted in a known order, whose ids sort
// the other way. Nothing here is timing-dependent, and the clock is fixed so
// created_at cannot be standing in for the answer.
//
// All three readers are asserted. LastCheckpoint is the query, Handover is what
// an arriving agent calls, and Summarize is what `status` publishes — and the
// project-scoped query in Summarize is a second copy of the same ORDER BY,
// which is exactly the kind of pair that gets fixed in one place.
func TestTheNewestCheckpointIsTheOneWrittenLast(t *testing.T) {
	f := openFixture(t, filepath.Join(t.TempDir(), "mindrail.db"), app.FixedClock{Instant: baseInstant})

	// Two sessions, because the two rows have to be distinguishable through
	// every reader. Summarize publishes a narrow view with no checkpoint id in
	// it, so with one session it could not tell the two apart and would pass
	// whichever row the query returned.
	departing := f.session(t)
	arriving := f.session(t)
	if departing.ID == arriving.ID {
		t.Fatal("the fixture minted one session twice")
	}
	task := f.task(t, departing.ID, "a task two agents both wrote on")

	// The first id sorts after the second. Both are the shape identity.NewID
	// mints; only their order is arranged.
	const (
		firstID  = "CKP-01M2337ZZZZZZZZZZZZZZZZZZZZZ"
		secondID = "CKP-01M23370000000000000000000000"
	)
	if firstID <= secondID {
		t.Fatalf("the fixture does not reproduce the condition: %q sorts before %q", firstID, secondID)
	}

	f.insertCheckpoint(t, firstID, task.ID, departing.ID, "the departing agent's note")
	f.insertCheckpoint(t, secondID, task.ID, arriving.ID, "the note that superseded it")

	last, err := f.store.LastCheckpoint(t.Context(), task.ID)
	if err != nil {
		t.Fatalf("LastCheckpoint = %v, want no error", err)
	}
	if last.ID != secondID {
		t.Errorf("LastCheckpoint returned %s (%q), want the row inserted second, %s",
			last.ID, last.Note, secondID)
	}

	handover, err := f.store.Handover(t.Context(), task.ID)
	if err != nil {
		t.Fatalf("Handover = %v, want no error", err)
	}
	if handover.Checkpoint == nil {
		t.Fatal("Handover carries no checkpoint, and the task has two")
	}
	if handover.Checkpoint.ID != secondID {
		t.Errorf("Handover returned %s (%q), want the row inserted second, %s",
			handover.Checkpoint.ID, handover.Checkpoint.Note, secondID)
	}

	summary, err := f.store.Summarize(t.Context(), f.projectID)
	if err != nil {
		t.Fatalf("Summarize = %v, want no error", err)
	}
	if summary.LastCheckpoint == nil {
		t.Fatal("Summarize carries no last checkpoint, and the project has two")
	}
	// Summarize publishes a narrow view with no id in it, so the assertion is
	// on the session the row belongs to.
	if summary.LastCheckpoint.SessionID != arriving.ID {
		t.Errorf("Summarize points at session %s, want the one that wrote second, %s",
			summary.LastCheckpoint.SessionID, arriving.ID)
	}
	if summary.LastCheckpoint.TaskID != task.ID {
		t.Errorf("Summarize points at task %s, want %s", summary.LastCheckpoint.TaskID, task.ID)
	}
}

// insertCheckpoint writes a checkpoint row with an id of the caller's choosing.
//
// WriteCheckpoint mints its own id, so the only way to arrange two rows whose
// ids run against their insertion order is to write them directly. Every other
// column is what WriteCheckpoint would have written.
func (f fixture) insertCheckpoint(t *testing.T, id, taskID, sessionID, note string) {
	t.Helper()

	_, err := f.db.ExecContext(t.Context(),
		`INSERT INTO checkpoints (checkpoint_id, task_id, session_id, workspace_id,
			note, handoff, created_at)
		 VALUES (?, ?, ?, ?, ?, 0, ?)`,
		id, taskID, sessionID, f.spaceID, note, app.FormatTime(baseInstant))
	if err != nil {
		t.Fatalf("inserting checkpoint %s: %v", id, err)
	}
}
