package cli_test

import (
	"strings"
	"testing"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/storage"
)

// badBytes is a title a filesystem, a terminal and SQLite all carry happily and
// JSON cannot. `git log -1 --format=%s` in a repository configured with
// i18n.commitEncoding = ISO-8859-1 produces exactly this shape, so an agent
// reaching it is not doing anything unusual.
const badBytes = "fix parser \xff bug"

// TestARefusedWriteWritesNothing is finding F28.
//
// The refusal existed before this test; it fired in emit, which runs after the
// body has committed. `mindrail --json task open --title "…\xff…"` therefore
// inserted the task and the session, and *then* said ok:false at exit 2 — and
// the correct response to a failed command, retrying it, opened a second task
// and a third. On `checkpoint write` the duplicates are permanent: decision
// D-59 makes checkpoints append-only and there is no delete verb.
//
// So the assertion is a row count and not the envelope. The envelope was always
// right about the refusal; what was wrong was that the write had already
// happened, and a test that only read the payload could not see it.
func TestARefusedWriteWritesNothing(t *testing.T) {
	for _, tc := range []struct {
		name  string
		flag  string
		table string
		// reach moves the task to the state the row's command needs, so that the
		// refusal is the only thing standing between the command and a write.
		// Without it the `task state` row would be refused by the lifecycle
		// instead and would prove nothing about where the UTF-8 check runs.
		reach []string
		args  func(task string) []string
	}{
		{
			name:  "session open",
			flag:  "label",
			table: "sessions",
			args:  func(string) []string { return []string{"session", "open", "--label", badBytes} },
		},
		{
			name:  "task open",
			flag:  "title",
			table: "tasks",
			args:  func(string) []string { return []string{"task", "open", "--title", badBytes} },
		},
		{
			name:  "task state",
			flag:  "reason",
			table: "sessions",
			reach: []string{"CLAIMED", "IN_PROGRESS"},
			args: func(task string) []string {
				return []string{"task", "state", task, "--to", "BLOCKED", "--reason", badBytes}
			},
		},
		{
			name:  "checkpoint write",
			flag:  "note",
			table: "checkpoints",
			args:  func(task string) []string { return []string{"checkpoint", "write", task, "--note", badBytes} },
		},
		{
			// MR-004: the file key is free text the caller wrote, and it reaches
			// the leases table unchanged apart from cleaning.
			name:  "lease acquire",
			flag:  "file",
			table: "leases",
			args:  func(string) []string { return []string{"lease", "acquire", "--file", badBytes} },
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := newInitializedRepo(t)
			session := sessionID(t, repo)
			task := openTask(t, repo, session, "a task to write against")
			for _, to := range tc.reach {
				run(t, repo, "task", "state", task, "--to", to, "--session", session, "--json").
					requireExit(t, app.ExitSuccess)
			}

			before := rowCount(t, repo, tc.table)
			state := taskState(t, repo, task)

			got := run(t, repo, append(tc.args(task), "--json")...)

			if got.code != app.ExitUsage {
				t.Fatalf("exit = %d, want %d\n%s", got.code, app.ExitUsage, got.stdout)
			}
			payload := got.errorPayload(t)
			if payload.Code != app.CodeCommandLineInvalid {
				t.Errorf("code = %q, want %q", payload.Code, app.CodeCommandLineInvalid)
			}
			if !strings.Contains(payload.Why, "--"+tc.flag) {
				t.Errorf("why = %q does not name the flag that was refused", payload.Why)
			}
			// The diagnosis must survive the delivery it is complaining about.
			if strings.ContainsRune(payload.Why, '�') {
				t.Errorf("the refusal's own message was mangled: %q", payload.Why)
			}

			if after := rowCount(t, repo, tc.table); after != before {
				t.Errorf("a refused `%s` wrote %d row(s) into %s, want 0", tc.name, after-before, tc.table)
			}
			if after := taskState(t, repo, task); after != state {
				t.Errorf("a refused `%s` moved the task from %s to %s", tc.name, state, after)
			}
		})
	}
}

// TestAValidTitleIsStillWritten is the over-fire guard. Every row above would
// also pass if the four commands had simply stopped writing.
func TestAValidTitleIsStillWritten(t *testing.T) {
	repo := newInitializedRepo(t)

	before := rowCount(t, repo, "tasks")
	got := run(t, repo, "task", "open", "--title", "an ordinary title", "--json")
	got.requireExit(t, app.ExitSuccess)

	if after := rowCount(t, repo, "tasks"); after != before+1 {
		t.Errorf("a valid `task open` wrote %d rows, want exactly 1", after-before)
	}
}

// TestHumanModeStillCarriesTheBytesThrough is the other half of decision W9's
// bargain, and the reason the refusal above is gated on --json.
//
// Refusing everywhere would take away the one view that can still show such a
// value. The human report writes the bytes out untouched, which is what makes
// "run this command without --json" a remedy a reader can actually carry out.
func TestHumanModeStillCarriesTheBytesThrough(t *testing.T) {
	repo := newInitializedRepo(t)

	got := run(t, repo, "task", "open", "--title", badBytes, "--no-color")
	got.requireExit(t, app.ExitSuccess)

	if !strings.Contains(got.stdout, badBytes) {
		t.Errorf("the human report did not carry the title through unchanged:\n%s", got.stdout)
	}
}

// TestAStoredValueIsNotReportedAsAPath is finding F30.
//
// A row written before the boundary refusal existed — or by an older binary —
// still reaches the serialiser, and the emit-time refusal is right to fire on
// it. What was wrong was what it said: `tasks.title` was reported as "a path
// that does not exist on disk", remedied with "rename the offending path", over
// a value no rename reaches and that no command can edit or delete, because
// AC-06.1 fixes the surface at six verbs.
//
// The row is written straight into SQLite because there is deliberately no
// longer a command that can produce one.
func TestAStoredValueIsNotReportedAsAPath(t *testing.T) {
	repo := newInitializedRepo(t)
	session := sessionID(t, repo)
	task := openTask(t, repo, session, "a title that was fine when it was written")

	storeUnrepresentableTitle(t, repo, task)

	got := run(t, repo, "task", "list", "--json")
	if got.code != app.ExitUsage {
		t.Fatalf("exit = %d, want %d\n%s", got.code, app.ExitUsage, got.stdout)
	}

	payload := got.errorPayload(t)
	if payload.Code != app.CodePathNotRepresentable {
		t.Fatalf("code = %q, want %q", payload.Code, app.CodePathNotRepresentable)
	}
	if !strings.Contains(payload.Why, "title") {
		t.Errorf("why = %q does not say which value it is", payload.Why)
	}

	remedies := strings.Join(payload.NextAction, " ")
	if strings.Contains(remedies, "rename") {
		t.Errorf("the remedy tells the reader to rename a path for a value stored in a column: %v",
			payload.NextAction)
	}
	if !remedyMentions(payload.NextAction, "without --json") {
		t.Errorf("the remedy does not offer the view that can still show the value: %v", payload.NextAction)
	}
}

// storeUnrepresentableTitle puts bytes into tasks.title that no command will
// write. SQLite does not validate the encoding of a TEXT column, which is why
// the condition is reachable at all.
func storeUnrepresentableTitle(t *testing.T, repo, task string) {
	t.Helper()

	db, err := storage.Open(t.Context(), storage.Options{Path: runtimeDBPath(t, repo)})
	if err != nil {
		t.Fatalf("opening the runtime database: %v", err)
	}
	defer func() { _ = db.Close() }()

	result, err := db.ExecContext(t.Context(),
		`UPDATE tasks SET title = ? WHERE task_id = ?`, badBytes, task)
	if err != nil {
		t.Fatalf("storing an unrepresentable title: %v", err)
	}
	if affected, err := result.RowsAffected(); err != nil || affected != 1 {
		t.Fatalf("the fixture updated %d rows (%v), want 1", affected, err)
	}
}

// taskState reads one task's state straight from the database, so that a
// refusal cannot be checked against the surface that refused it.
func taskState(t *testing.T, repo, task string) string {
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
	if err := db.QueryRowContext(t.Context(),
		`SELECT state FROM tasks WHERE task_id = ?`, task).Scan(&state); err != nil {
		t.Fatalf("reading the state of %s: %v", task, err)
	}
	return state
}
