package cli_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/storage"
)

// TestTwoSequentialAgentsContinueOneTaskAcrossAProcessBoundary is MR-003's
// acceptance criterion, driven through the real command tree.
//
// It is the milestone's own sentence executed: two different AgentSessions using
// the same workspace one after the other continue the same Task. Nothing is
// carried between the two halves in Go — no shared Store, no cached row, no open
// handle. Every step is a command invocation that starts the application, does
// one thing and shuts the database down again, which is what a second agent in a
// second process actually gets.
//
// The assertion that matters is the last one: the arriving session reads the
// task, its state and the departing session's note, and the note is attributed to
// the session that wrote it rather than to the one reading it.
func TestTwoSequentialAgentsContinueOneTaskAcrossAProcessBoundary(t *testing.T) {
	repo := newInitializedRepo(t)

	// --- agent A -----------------------------------------------------------
	first := sessionID(t, repo)
	task := openTask(t, repo, first, "Wire the reconcile path")

	for _, to := range []string{"CLAIMED", "IN_PROGRESS"} {
		got := run(t, repo, "task", "state", task, "--to", to, "--session", first, "--json")
		got.requireExit(t, app.ExitSuccess)
	}

	const note = "Parser is done; the resolver still returns nil for aliases."
	written := run(t, repo, "checkpoint", "write", task,
		"--note", note, "--handoff", "--session", first, "--json")
	written.requireExit(t, app.ExitSuccess)

	// --- agent B -----------------------------------------------------------
	second := sessionID(t, repo)
	if second == first {
		t.Fatal("the second agent was given the first agent's session; a session is minted, never resumed")
	}

	shown := run(t, repo, "task", "show", task, "--json")
	shown.requireExit(t, app.ExitSuccess)

	var handover struct {
		Task struct {
			ID            string `json:"task_id"`
			Title         string `json:"title"`
			State         string `json:"state"`
			ClaimedBy     string `json:"claimed_by"`
			BlockedReason string `json:"blocked_reason"`
		} `json:"task"`
		Checkpoint *struct {
			Note      string `json:"note"`
			SessionID string `json:"session_id"`
			Handoff   bool   `json:"handoff"`
		} `json:"checkpoint"`
	}
	decodeData(t, shown.stdout, &handover)

	switch {
	case handover.Task.ID != task:
		t.Errorf("task id = %q, want %q", handover.Task.ID, task)
	case handover.Task.Title != "Wire the reconcile path":
		t.Errorf("title = %q, want the one agent A opened it with", handover.Task.Title)
	case handover.Task.State != "IN_PROGRESS":
		t.Errorf("state = %q, want IN_PROGRESS, where agent A left it", handover.Task.State)
	case handover.Task.ClaimedBy != first:
		t.Errorf("claimed_by = %q, want agent A's session %q", handover.Task.ClaimedBy, first)
	}

	if handover.Checkpoint == nil {
		t.Fatalf("the arriving agent sees no checkpoint; the note is what it came for\n%s", shown.stdout)
	}
	if handover.Checkpoint.Note != note {
		t.Errorf("note = %q, want %q", handover.Checkpoint.Note, note)
	}
	if handover.Checkpoint.SessionID != first {
		t.Errorf("the note is attributed to %q, want the session that wrote it, %q",
			handover.Checkpoint.SessionID, first)
	}
	if !handover.Checkpoint.Handoff {
		t.Error("handoff = false, want the flag agent A set")
	}

	// And the arriving agent can carry the task on, under its own session.
	moved := run(t, repo, "task", "state", task, "--to", "READY_TO_COMPLETE", "--session", second, "--json")
	moved.requireExit(t, app.ExitSuccess)

	var carried struct {
		Task struct {
			State     string `json:"state"`
			ClaimedBy string `json:"claimed_by"`
		} `json:"task"`
	}
	decodeData(t, moved.stdout, &carried)
	if carried.Task.State != "READY_TO_COMPLETE" {
		t.Errorf("state = %q after the arriving agent moved it", carried.Task.State)
	}
	// The claim still names agent A. A move is not a claim, and rewriting it
	// would lose which session took the task on in the first place.
	if carried.Task.ClaimedBy != first {
		t.Errorf("claimed_by = %q, want the original claimant %q", carried.Task.ClaimedBy, first)
	}
}

// TestEveryCoordinationCommandRefusesAnUninitialisedRepository is AC-06.7.
//
// All six are asserted, not one. The refusal lives in a shared helper, and a
// command that forgot to call it would reach a nil store and panic — which is
// the failure a test over one command reliably misses.
func TestEveryCoordinationCommandRefusesAnUninitialisedRepository(t *testing.T) {
	repo := newRepo(t)

	for _, args := range [][]string{
		{"session", "open"},
		{"task", "open", "--title", "something"},
		{"task", "state", "TSK-0000000000000000000000000", "--to", "CLAIMED"},
		{"task", "show", "TSK-0000000000000000000000000"},
		{"task", "list"},
		{"checkpoint", "write", "TSK-0000000000000000000000000", "--note", "something"},
	} {
		name := strings.Join(args[:2], " ")
		got := run(t, repo, append(args, "--json")...)

		if got.code != app.ExitFailed {
			t.Errorf("%s exited %d in an uninitialised repository, want %d", name, got.code, app.ExitFailed)
		}
		payload := got.errorPayload(t)
		if payload.Code != app.CodeCoordinationUnavailable {
			t.Errorf("%s reported %q, want %q", name, payload.Code, app.CodeCoordinationUnavailable)
		}
		if !remedyMentions(payload.NextAction, "mindrail init") {
			t.Errorf("%s does not tell the reader to run init: %v", name, payload.NextAction)
		}
	}

	// The over-fire guard. Every row above would also pass if the commands
	// refused an initialised repository too.
	ready := newInitializedRepo(t)
	if got := run(t, ready, "task", "list", "--json"); got.code != app.ExitSuccess {
		t.Errorf("task list exited %d in an initialised repository: %s", got.code, got.stdout)
	}
}

// TestAMisspelledStateIsUsageAndAnUnavailableMoveIsNot is AC-06.4, and the
// distinction is the point.
//
// A word this binary does not have is a mistake in the command line and exits 2,
// like every other one in this tree. A word it does have, applied where the
// lifecycle does not allow it, is an operation that ran and could not be done:
// exit 1, with a message naming both states and what is available instead.
func TestAMisspelledStateIsUsageAndAnUnavailableMoveIsNot(t *testing.T) {
	repo := newInitializedRepo(t)
	session := sessionID(t, repo)
	task := openTask(t, repo, session, "a task to misdirect")

	misspelled := run(t, repo, "task", "state", task, "--to", "DONE", "--session", session, "--json")
	if misspelled.code != app.ExitUsage {
		t.Errorf("a state this binary does not have exited %d, want %d", misspelled.code, app.ExitUsage)
	}
	usage := misspelled.errorPayload(t)
	if usage.Code != app.CodeCommandLineInvalid {
		t.Errorf("a misspelled state reported %q, want %q", usage.Code, app.CodeCommandLineInvalid)
	}
	// The reader has to learn the vocabulary from the refusal.
	for _, state := range []string{"OPEN", "CLAIMED", "IN_PROGRESS", "READY_TO_COMPLETE"} {
		if !strings.Contains(strings.Join(usage.NextAction, " "), state) {
			t.Errorf("the refusal does not name %s: %v", state, usage.NextAction)
		}
	}

	unavailable := run(t, repo, "task", "state", task, "--to", "COMPLETED", "--session", session, "--json")
	if unavailable.code != app.ExitFailed {
		t.Errorf("an unavailable move exited %d, want %d", unavailable.code, app.ExitFailed)
	}
	refusal := unavailable.errorPayload(t)
	if refusal.Code != app.CodeTaskStateInvalid {
		t.Errorf("an unavailable move reported %q, want %q", refusal.Code, app.CodeTaskStateInvalid)
	}
	for _, expected := range []string{"OPEN", "COMPLETED"} {
		if !strings.Contains(refusal.Why, expected) {
			t.Errorf("the refusal does not name %s: %q", expected, refusal.Why)
		}
	}
	if !remedyMentions(refusal.NextAction, "CLAIMED") {
		t.Errorf("the refusal does not say what is available instead: %v", refusal.NextAction)
	}

	// Neither attempt moved the task.
	shown := run(t, repo, "task", "show", task, "--json")
	shown.requireExit(t, app.ExitSuccess)
	var handover struct {
		Task struct {
			State string `json:"state"`
		} `json:"task"`
	}
	decodeData(t, shown.stdout, &handover)
	if handover.Task.State != "OPEN" {
		t.Errorf("the task is %s after two refused moves, want OPEN", handover.Task.State)
	}
}

// TestAWriteWithNoSessionMintsOneAndSaysSo is AC-06.5 and decision D-61.
//
// The minted id has to reach the caller in both renderings. An agent that meant
// to pass --session and forgot would otherwise carry on under an identity it
// never learned, and its next command would open a second one.
func TestAWriteWithNoSessionMintsOneAndSaysSo(t *testing.T) {
	repo := newInitializedRepo(t)

	got := run(t, repo, "task", "open", "--title", "a task with no session named", "--json")
	got.requireExit(t, app.ExitSuccess)

	var result struct {
		Session struct {
			ID string `json:"session_id"`
		} `json:"session"`
		SessionMinted bool `json:"session_minted"`
	}
	decodeData(t, got.stdout, &result)

	if !result.SessionMinted {
		t.Error("session_minted = false, want true: no --session was given")
	}
	if !strings.HasPrefix(result.Session.ID, "SES-") {
		t.Errorf("session id = %q, want a minted one", result.Session.ID)
	}

	human := run(t, repo, "task", "open", "--title", "another one")
	human.requireExit(t, app.ExitSuccess)
	if !strings.Contains(human.stdout, "--session") {
		t.Errorf("the human rendering does not offer the minted session back:\n%s", human.stdout)
	}

	// The other arm: a session that *was* named is not reported as minted.
	named := run(t, repo, "task", "open", "--title", "a third", "--session", result.Session.ID, "--json")
	named.requireExit(t, app.ExitSuccess)
	decodeData(t, named.stdout, &result)
	if result.SessionMinted {
		t.Error("session_minted = true for a command that named its session")
	}
}

// TestAnUnknownSessionIsRefusedRatherThanMinted is decision D-61's other half.
//
// Minting under a typo would turn one agent into two and file the next
// checkpoint under an identity nothing else refers to.
func TestAnUnknownSessionIsRefusedRatherThanMinted(t *testing.T) {
	repo := newInitializedRepo(t)

	got := run(t, repo, "task", "open", "--title", "a task under a typo",
		"--session", "SES-0000000000000000000000000", "--json")
	if got.code != app.ExitFailed {
		t.Fatalf("an unknown session exited %d, want %d\n%s", got.code, app.ExitFailed, got.stdout)
	}
	if payload := got.errorPayload(t); payload.Code != app.CodeSessionNotFound {
		t.Errorf("an unknown session reported %q, want %q", payload.Code, app.CodeSessionNotFound)
	}

	listed := run(t, repo, "task", "list", "--json")
	listed.requireExit(t, app.ExitSuccess)
	var tasks struct {
		Tasks []struct {
			ID string `json:"task_id"`
		} `json:"tasks"`
	}
	decodeData(t, listed.stdout, &tasks)
	if len(tasks.Tasks) != 0 {
		t.Errorf("a task was opened under a session that does not exist: %+v", tasks.Tasks)
	}
}

// TestTheReadCommandsMintNoSession is AC-06.6.
//
// A command that created a row just by asking a question would make the session
// table a log of curiosity rather than of work — and would grow it once per
// `task list` in any script that polls.
func TestTheReadCommandsMintNoSession(t *testing.T) {
	repo := newInitializedRepo(t)
	session := sessionID(t, repo)
	task := openTask(t, repo, session, "a task to read repeatedly")

	before := sessionCount(t, repo)
	for range 3 {
		run(t, repo, "task", "list", "--json").requireExit(t, app.ExitSuccess)
		run(t, repo, "task", "show", task, "--json").requireExit(t, app.ExitSuccess)
	}
	if after := sessionCount(t, repo); after != before {
		t.Errorf("six read commands minted %d sessions, want none", after-before)
	}

	// The over-fire guard: a write still mints one, or the count above would be
	// stable for the wrong reason.
	run(t, repo, "task", "open", "--title", "a write with no session").requireExit(t, app.ExitSuccess)
	if after := sessionCount(t, repo); after != before+1 {
		t.Errorf("a session-less write minted %d sessions, want exactly 1", after-before)
	}
}

// TestEveryCoordinationCommandEmitsExactlyOneEnvelope is AC-06.2 over the whole
// group, on both the success and the failure path.
//
// Decision D-15's promise is that `--json` puts one object on stdout and nothing
// else, so a caller can parse it with no filtering. A command that printed a
// human line beside it, or two objects, breaks every consumer at once.
func TestEveryCoordinationCommandEmitsExactlyOneEnvelope(t *testing.T) {
	repo := newInitializedRepo(t)
	session := sessionID(t, repo)
	task := openTask(t, repo, session, "a task to exercise every command")

	for _, tc := range []struct {
		name string
		args []string
	}{
		{name: "session open", args: []string{"session", "open"}},
		{name: "task open", args: []string{"task", "open", "--title", "one more", "--session", session}},
		{name: "task state", args: []string{"task", "state", task, "--to", "CLAIMED", "--session", session}},
		{name: "task show", args: []string{"task", "show", task}},
		{name: "task list", args: []string{"task", "list"}},
		{name: "checkpoint write", args: []string{"checkpoint", "write", task, "--note", "a note", "--session", session}},
		{name: "task state refused", args: []string{"task", "state", task, "--to", "COMPLETED", "--session", session}},
		{name: "task show unknown", args: []string{"task", "show", "TSK-0000000000000000000000000"}},
	} {
		got := run(t, repo, append(tc.args, "--json")...)

		decoder := json.NewDecoder(strings.NewReader(got.stdout))
		var envelope map[string]any
		if err := decoder.Decode(&envelope); err != nil {
			t.Errorf("%s: stdout is not one JSON object: %v\n%s", tc.name, err, got.stdout)
			continue
		}
		if decoder.More() {
			t.Errorf("%s: stdout carries more than one JSON value:\n%s", tc.name, got.stdout)
		}

		ok, isBool := envelope["ok"].(bool)
		if !isBool {
			t.Errorf("%s: the envelope has no boolean ok member:\n%s", tc.name, got.stdout)
			continue
		}
		if ok != (got.code == app.ExitSuccess) {
			t.Errorf("%s: ok=%v with exit %d", tc.name, ok, got.code)
		}
		if _, carriesError := envelope["error"]; carriesError == ok {
			t.Errorf("%s: ok=%v and the error member's presence is %v", tc.name, ok, carriesError)
		}
	}
}

// --- helpers ----------------------------------------------------------------

// sessionID mints a session through the command tree and returns its id.
func sessionID(t *testing.T, repo string) string {
	t.Helper()

	got := run(t, repo, "session", "open", "--json")
	got.requireExit(t, app.ExitSuccess)

	var result struct {
		Session struct {
			ID string `json:"session_id"`
		} `json:"session"`
	}
	decodeData(t, got.stdout, &result)
	if result.Session.ID == "" {
		t.Fatalf("session open printed no id:\n%s", got.stdout)
	}
	return result.Session.ID
}

// openTask opens a task through the command tree and returns its id.
func openTask(t *testing.T, repo, session, title string) string {
	t.Helper()

	got := run(t, repo, "task", "open", "--title", title, "--session", session, "--json")
	got.requireExit(t, app.ExitSuccess)

	var result struct {
		Task struct {
			ID string `json:"task_id"`
		} `json:"task"`
	}
	decodeData(t, got.stdout, &result)
	if result.Task.ID == "" {
		t.Fatalf("task open printed no id:\n%s", got.stdout)
	}
	return result.Task.ID
}

// sessionCount reads the sessions table directly. There is no command that
// lists sessions — nothing needs one — so counting them means opening the
// database, and read-only is what a test that must not disturb the subject uses.
func sessionCount(t *testing.T, repo string) int {
	t.Helper()

	db, err := storage.Open(t.Context(), storage.Options{
		Path:     runtimeDBPath(t, repo),
		ReadOnly: true,
	})
	if err != nil {
		t.Fatalf("opening the runtime database: %v", err)
	}
	defer func() { _ = db.Close() }()

	var count int
	if err := db.QueryRowContext(t.Context(), `SELECT count(*) FROM sessions`).Scan(&count); err != nil {
		t.Fatalf("counting sessions: %v", err)
	}
	return count
}

func remedyMentions(actions []string, needle string) bool {
	return strings.Contains(strings.Join(actions, " "), needle)
}
