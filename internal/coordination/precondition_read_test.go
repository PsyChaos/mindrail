package coordination_test

import (
	"testing"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/coordination"
)

// TestAPreconditionReadThatFailsIsReportedAsAReadOfThatRow is audit round 2,
// §4.7, at the layer AC-11.4 names as the surface MR-014's tools call directly.
//
// Each writer reads before it writes, inside its own transaction, and a bare
// error leaving that transaction is adopted as COORDINATION_WRITE_FAILED with
// the writer's own subject — so a session whose started_at does not parse was
// published as "the task could not be written", naming a task id no statement
// ever attempted. The code follows what happened to a row, not the verb of the
// method: a row the database holds and cannot answer for is
// COORDINATION_READ_FAILED naming that row, and the write is absent.
//
// The third arm is the one the command layer cannot reach. WriteCheckpoint's
// existence probe reads only the key column, which the STRICT schema keeps as
// text, so its one failure is a table the file no longer holds — and the CLI's
// schema gate answers that with `mindrail init` before the store is asked. A
// direct caller has no such gate, which is why the rule is asserted here.
func TestAPreconditionReadThatFailsIsReportedAsAReadOfThatRow(t *testing.T) {
	for _, tc := range []struct {
		name   string
		damage string
		// subject picks which of the two ids the refusal must name.
		subject func(session coordination.Session, task coordination.Task) string
		write   func(t *testing.T, f fixture, session coordination.Session, task coordination.Task) error
	}{
		{
			name:    "OpenTask under a session whose started_at cannot be parsed",
			damage:  `UPDATE sessions SET started_at = 'yesterday'`,
			subject: func(session coordination.Session, _ coordination.Task) string { return session.ID },
			write: func(t *testing.T, f fixture, session coordination.Session, _ coordination.Task) error {
				_, _, err := f.store.OpenTask(t.Context(), f.projectID, coordination.NamedSession(session.ID), "a task under a damaged session")
				return err
			},
		},
		{
			name:    "Transition on a task whose created_at cannot be parsed",
			damage:  `UPDATE tasks SET created_at = 'yesterday'`,
			subject: func(_ coordination.Session, task coordination.Task) string { return task.ID },
			write: func(t *testing.T, f fixture, session coordination.Session, task coordination.Task) error {
				_, _, err := f.store.Transition(t.Context(), task.ID, coordination.NamedSession(session.ID), coordination.StateClaimed, "")
				return err
			},
		},
		{
			name:    "WriteCheckpoint on a task the database can no longer answer for",
			damage:  `DROP TABLE tasks`,
			subject: func(_ coordination.Session, task coordination.Task) string { return task.ID },
			write: func(t *testing.T, f fixture, session coordination.Session, task coordination.Task) error {
				_, _, err := f.store.WriteCheckpoint(t.Context(), task.ID, coordination.NamedSession(session.ID), f.spaceID, "a note on an unreadable task", false)
				return err
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			session := f.session(t)
			task := f.task(t, session.ID, "a task opened before the damage")
			if _, err := f.db.DB.ExecContext(t.Context(), tc.damage); err != nil {
				t.Fatalf("damaging the fixture: %v", err)
			}

			err := tc.write(t, f, session, task)
			if err == nil {
				t.Fatal("the write succeeded over a row it could not have read")
			}

			payload, ok := app.PayloadOf(err)
			if !ok {
				t.Fatalf("the failure %v carries no payload a caller can branch on", err)
			}
			if payload.Code != app.CodeCoordinationReadFailed {
				t.Errorf("code = %q, want %q: a row was read and could not be decoded, and nothing was written",
					payload.Code, app.CodeCoordinationReadFailed)
			}
			if want := tc.subject(session, task); payload.Metadata["subject_id"] != want {
				t.Errorf("metadata.subject_id = %q, want the row that could not be read, %q",
					payload.Metadata["subject_id"], want)
			}
		})
	}
}
