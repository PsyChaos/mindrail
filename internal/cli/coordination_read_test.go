package cli_test

import (
	"strings"
	"testing"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/storage"
)

// TestADamagedRowIsReportedWithACode is finding F44.
//
// Every write path in internal/coordination was wrapped in a domain error and
// every read path returned a bare fmt.Errorf, which emit published verbatim.
// One unparseable timestamp therefore reached the user as
// `{"code":"","why":"look up the last checkpoint of \"TSK-…\": parsing time
// \"yesterday\" as \"2006\"…","impact":"","next_action":[]}` — a Go parser
// message with a format string in it, at exit 1, with nothing to branch on and
// nothing to do about it. AC-04.6 requires a registered code, a diagnostic, an
// impact and a next action on every error.
//
// The damage is semantic rather than structural on purpose: PRAGMA
// integrity_check runs at startup and would intercept a corrupt file, so a
// column that parses as text and not as a timestamp is what actually reaches
// the read path. It is not exotic — FormatTime writes RFC3339Nano and ParseTime
// is strict, so a host clock outside years 1–9999 writes checkpoints this
// binary can never read back.
func TestADamagedRowIsReportedWithACode(t *testing.T) {
	repo := newInitializedRepo(t)
	session := sessionID(t, repo)
	task := openTask(t, repo, session, "a task with a note that will be damaged")
	run(t, repo, "checkpoint", "write", task, "--note", "the note", "--session", session, "--json").
		requireExit(t, app.ExitSuccess)

	execOnRuntimeDB(t, repo, `UPDATE checkpoints SET created_at = 'yesterday'`)

	got := run(t, repo, "task", "show", task, "--json")
	if got.code == app.ExitSuccess {
		t.Fatalf("task show succeeded over a row it cannot parse:\n%s", got.stdout)
	}

	payload := got.errorPayload(t)
	if !app.IsRegistered(payload.Code) {
		t.Errorf("code = %q, which this binary does not own; a caller has nothing to branch on\n%s",
			payload.Code, got.stdout)
	}
	if payload.Code != app.CodeCoordinationReadFailed {
		t.Errorf("code = %q, want %q", payload.Code, app.CodeCoordinationReadFailed)
	}
	assertFourErrorKeys(t, got.stdout)

	// The parser message is the cause, not the diagnosis. A `why` that is a Go
	// format string tells the reader what the code did, not what is wrong.
	if strings.Contains(payload.Why, "parsing time") {
		t.Errorf("why = %q is the parser's message rather than a diagnosis", payload.Why)
	}
}

// TestAReadThatRanAndFailedIsNotPublishedAsNobodyLooked is finding F46.
//
// The same damaged row, seen through `status`. The store was constructed, both
// queries ran and one failed — and the block said "not observed: startup
// stopped before this subsystem was read", about a startup that completed. The
// adjacent Runtime block has had `indeterminate` since MR-001 and means exactly
// what happened here.
func TestAReadThatRanAndFailedIsNotPublishedAsNobodyLooked(t *testing.T) {
	repo := newInitializedRepo(t)
	session := sessionID(t, repo)
	task := openTask(t, repo, session, "a task with a note that will be damaged")
	run(t, repo, "checkpoint", "write", task, "--note", "the note", "--session", session, "--json").
		requireExit(t, app.ExitSuccess)

	execOnRuntimeDB(t, repo, `UPDATE checkpoints SET created_at = 'yesterday'`)

	got := run(t, repo, "status", "--json")

	var data struct {
		Coordination struct {
			Observation string `json:"observation"`
		} `json:"coordination"`
	}
	decodeData(t, got.stdout, &data)

	if data.Coordination.Observation != "indeterminate" {
		t.Errorf("observation = %q after a read that ran and failed, want indeterminate:\n%s",
			data.Coordination.Observation, got.stdout)
	}
}

// TestNobodyLookedIsStillSaidWhereNobodyLooked is the over-fire guard for the
// test above, and the condition `not_observed` genuinely describes.
//
// With the database unreadable the startup sequence stops before step 6, so
// knowledge, the workspace and coordination are all things nothing was read
// about. All three have to say so together: a block that had quietly started
// answering `indeterminate` everywhere would pass the test above and mean
// nothing.
func TestNobodyLookedIsStillSaidWhereNobodyLooked(t *testing.T) {
	repo := newInitializedRepo(t)
	denyAccess(t, runtimeDBPath(t, repo))

	got := run(t, repo, "status", "--json")

	var data struct {
		Knowledge struct {
			Observation string `json:"observation"`
		} `json:"knowledge"`
		Workspace struct {
			Observation string `json:"observation"`
		} `json:"workspace"`
		Coordination struct {
			Observation string `json:"observation"`
		} `json:"coordination"`
	}
	decodeData(t, got.stdout, &data)

	for name, observation := range map[string]string{
		"knowledge":    data.Knowledge.Observation,
		"workspace":    data.Workspace.Observation,
		"coordination": data.Coordination.Observation,
	} {
		if observation != "not_observed" {
			t.Errorf("%s says %q where the sequence stopped before it, want not_observed:\n%s",
				name, observation, got.stdout)
		}
	}
}

// TestAReadThatFailsInsideAWriteNamesTheRowItCouldNotRead is audit round 2,
// §4.7, and the decision item 9 of its brief asked for.
//
// Every writing command reads before it writes — the session it is attributed
// to, the task it moves or annotates — and those reads run inside the write's
// own transaction. Their failures used to leave it bare, so adopt published
// them as COORDINATION_WRITE_FAILED, "the task could not be written to the
// runtime database", with a subject_id naming the row the command was about to
// insert and nothing had tried to; the session whose started_at could not be
// parsed appeared only in the free-text cause.
//
// The code is decided by what happened to a row, not by the verb of the
// command that met it: a row the database holds and cannot answer for is
// COORDINATION_READ_FAILED, which is what `task show` and `task list` already
// say about the same damage. Beyond the code, what is pinned here is the
// subject — the row that could not be read, not a phantom — and that the
// write the command was asked for did not happen, which the envelope alone
// cannot tell.
//
// The two rows are the two reads that decode a row. `checkpoint write`'s
// existence probe is the third precondition read and is not here: its only
// failure is a table the file does not hold, and the schema gate answers that
// with `mindrail init` before any command reaches the store. The store suite
// covers it for the direct caller that has no gate in front of it.
func TestAReadThatFailsInsideAWriteNamesTheRowItCouldNotRead(t *testing.T) {
	for _, tc := range []struct {
		name string
		// damage breaks the row the command will have to read, and returns the
		// id that row carries.
		damage func(t *testing.T, repo, session, task string) string
		args   func(session, task string) []string
		// unchanged reports whether the write the command was asked for is
		// still absent afterwards.
		unchanged func(t *testing.T, repo, task string) bool
	}{
		{
			name: "task open under a session whose started_at cannot be parsed",
			damage: func(t *testing.T, repo, session, _ string) string {
				execOnRuntimeDB(t, repo, `UPDATE sessions SET started_at = 'yesterday'`)
				return session
			},
			args: func(session, _ string) []string {
				return []string{"task", "open", "--title", "a task under a damaged session", "--session", session}
			},
			unchanged: func(t *testing.T, repo, _ string) bool { return rowCount(t, repo, "tasks") == 1 },
		},
		{
			name: "task state on a task whose created_at cannot be parsed",
			damage: func(t *testing.T, repo, _, task string) string {
				execOnRuntimeDB(t, repo, `UPDATE tasks SET created_at = 'yesterday'`)
				return task
			},
			args: func(session, task string) []string {
				return []string{"task", "state", task, "--to", "CLAIMED", "--session", session}
			},
			unchanged: func(t *testing.T, repo, task string) bool { return taskStateOf(t, repo, task) == "OPEN" },
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := newInitializedRepo(t)
			session := sessionID(t, repo)
			task := openTask(t, repo, session, "a task opened before the damage")
			subject := tc.damage(t, repo, session, task)

			got := run(t, repo, append(tc.args(session, task), "--json")...)
			if got.code != app.ExitFailed {
				t.Fatalf("exit = %d over a row it cannot read, want %d\n%s", got.code, app.ExitFailed, got.stdout)
			}

			payload := got.errorPayload(t)
			if payload.Code != app.CodeCoordinationReadFailed {
				t.Errorf("code = %q, want %q: a row was read and could not be decoded, and nothing was written",
					payload.Code, app.CodeCoordinationReadFailed)
			}
			if payload.Metadata["subject_id"] != subject {
				t.Errorf("metadata.subject_id = %q, want the row that could not be read, %q",
					payload.Metadata["subject_id"], subject)
			}
			if strings.Contains(payload.Why, "parsing time") {
				t.Errorf("why = %q is the parser's message rather than a diagnosis", payload.Why)
			}
			assertFourErrorKeys(t, got.stdout)

			if !tc.unchanged(t, repo, task) {
				t.Errorf("the write went through under a row that could not be read; impact says %q", payload.Impact)
			}
		})
	}
}

// taskStateOf reads one task's state straight from the runtime database. It is
// for the row above whose write is an UPDATE rather than an insert: a row count
// cannot tell a refused move from a made one.
func taskStateOf(t *testing.T, repo, task string) string {
	t.Helper()

	db, err := storage.Open(t.Context(), storage.Options{
		Path:     runtimeDBPath(t, repo),
		ReadOnly: true,
	})
	if err != nil {
		t.Fatalf("opening the runtime database: %v", err)
	}
	defer func() { _ = db.Close() }()

	var state string
	if err := db.QueryRowContext(t.Context(), `SELECT state FROM tasks WHERE task_id = ?`, task).Scan(&state); err != nil {
		t.Fatalf("reading the state of %s: %v", task, err)
	}
	return state
}
