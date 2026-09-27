package cli_test

import (
	"database/sql"
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/coordination"
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
	// Agent A's handoff released the task's lease (decision D-78), so agent
	// B's move took it over, and a takeover is a claim (decision D-68, which
	// amends D-58): the claimant is now B. Until MR-004 the claim stayed with
	// A, on the argument that a move is not a claim; the lease is what made
	// it one.
	if carried.Task.ClaimedBy != second {
		t.Errorf("claimed_by = %q, want the arriving agent %q, which took the task over", carried.Task.ClaimedBy, second)
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

	unavailable := run(t, repo, "task", "state", task, "--to", "IN_PROGRESS", "--session", session, "--json")
	if unavailable.code != app.ExitFailed {
		t.Errorf("an unavailable move exited %d, want %d", unavailable.code, app.ExitFailed)
	}
	refusal := unavailable.errorPayload(t)
	if refusal.Code != app.CodeTaskStateInvalid {
		t.Errorf("an unavailable move reported %q, want %q", refusal.Code, app.CodeTaskStateInvalid)
	}
	for _, expected := range []string{"OPEN", "IN_PROGRESS"} {
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

// TestRawTaskCompletionIsRefusedWithoutMutatingTaskOrLease is REQ-006's
// public boundary. A READY task is deliberately used: a refusal from OPEN
// would only prove the ordinary lifecycle table, not that the raw terminal
// transition is blocked before it can release the holder's lease.
func TestRawTaskCompletionIsRefusedWithoutMutatingTaskOrLease(t *testing.T) {
	repo := newInitializedRepo(t)
	session := sessionID(t, repo)
	task := openTask(t, repo, session, "complete through the gate")
	for _, state := range []string{"CLAIMED", "IN_PROGRESS", "READY_TO_COMPLETE"} {
		run(t, repo, "task", "state", task, "--to", state, "--session", session, "--json").requireExit(t, app.ExitSuccess)
	}

	before := run(t, repo, "task", "show", task, "--json")
	before.requireExit(t, app.ExitSuccess)
	var prior struct {
		Task struct {
			State    string `json:"state"`
			Revision int64  `json:"revision"`
		} `json:"task"`
	}
	decodeData(t, before.stdout, &prior)

	got := run(t, repo, "task", "state", task, "--to", "COMPLETED", "--session", session, "--json")
	got.requireExit(t, app.ExitFailed)
	refusal := got.errorPayload(t)
	if refusal.Code != app.CodeTaskStateInvalid {
		t.Fatalf("code = %q, want %q", refusal.Code, app.CodeTaskStateInvalid)
	}
	if len(refusal.NextAction) == 0 || !strings.Contains(refusal.NextAction[0], "task complete") {
		t.Errorf("next_action = %v, want evaluated completion as the READY action", refusal.NextAction)
	}
	for _, action := range refusal.NextAction {
		if strings.Contains(action, "move it to:") && strings.Contains(action, "COMPLETED") {
			t.Errorf("next_action = %v, must not offer raw COMPLETED as a lifecycle alternative", refusal.NextAction)
		}
	}

	after := run(t, repo, "task", "show", task, "--json")
	after.requireExit(t, app.ExitSuccess)
	var current struct {
		Task struct {
			State    string `json:"state"`
			Revision int64  `json:"revision"`
		} `json:"task"`
	}
	decodeData(t, after.stdout, &current)
	if current.Task.State != prior.Task.State || current.Task.Revision != prior.Task.Revision {
		t.Errorf("task after raw completion refusal = %+v, want %+v", current.Task, prior.Task)
	}

	leases := run(t, repo, "lease", "list", "--json")
	leases.requireExit(t, app.ExitSuccess)
	if !strings.Contains(leases.stdout, session) {
		t.Errorf("lease holder disappeared after raw completion refusal:\n%s", leases.stdout)
	}
}

// TestRawCompletionRefusalIsStateSpecific keeps a refused raw COMPLETED move
// actionable without offering an impossible command. Each row is a real task
// state, not a fabricated database value, so the task and lease snapshot also
// proves the refusal remains non-mutating.
func TestRawCompletionRefusalIsStateSpecific(t *testing.T) {
	for _, state := range coordination.States() {
		t.Run(state.String(), func(t *testing.T) {
			repo := newInitializedRepo(t)
			session := sessionID(t, repo)
			task := openTask(t, repo, session, "raw completion refusal "+state.String())
			driveTaskToState(t, repo, task, session, state)

			beforeTask := run(t, repo, "task", "show", task, "--json")
			beforeTask.requireExit(t, app.ExitSuccess)
			beforeLeases := run(t, repo, "lease", "list", "--json")
			beforeLeases.requireExit(t, app.ExitSuccess)

			refused := run(t, repo, "task", "state", task, "--to", coordination.StateCompleted.String(), "--session", session, "--json")
			refused.requireExit(t, app.ExitFailed)
			payload := refused.errorPayload(t)
			if payload.Code != app.CodeTaskStateInvalid || len(payload.NextAction) == 0 {
				t.Fatalf("refusal = %+v, want TASK_STATE_INVALID with actions", payload)
			}
			want := rawCompletionFirstAction(state)
			if !strings.Contains(payload.NextAction[0], want) {
				t.Fatalf("first action = %q, want executable %q", payload.NextAction[0], want)
			}
			for _, action := range payload.NextAction {
				if state != coordination.StateReadyToComplete && strings.Contains(action, "task complete") {
					t.Errorf("%s action wrongly offers completion: %q", state, action)
				}
				if !state.Terminal() && strings.Contains(action, "--to "+coordination.StateCompleted.String()) {
					t.Errorf("%s action wrongly offers raw completion: %q", state, action)
				}
			}

			afterTask := run(t, repo, "task", "show", task, "--json")
			afterTask.requireExit(t, app.ExitSuccess)
			afterLeases := run(t, repo, "lease", "list", "--json")
			afterLeases.requireExit(t, app.ExitSuccess)
			if afterTask.stdout != beforeTask.stdout || afterLeases.stdout != beforeLeases.stdout {
				t.Fatalf("raw completion refusal mutated %s\ntask before: %s\ntask after: %s\nleases before: %s\nleases after: %s",
					state, beforeTask.stdout, afterTask.stdout, beforeLeases.stdout, afterLeases.stdout)
			}
		})
	}
}

func rawCompletionFirstAction(state coordination.State) string {
	switch {
	case state.Terminal():
		return "task show <task-id>"
	case state == coordination.StateReadyToComplete:
		return "task complete <task-id> --session <session-id> --expect-revision <revision>"
	}
	for _, next := range coordination.TransitionsFrom(state) {
		if next != coordination.StateCompleted && next != coordination.StateBlocked {
			return "task state <task-id> --to " + next.String() + " --session <session-id>"
		}
	}
	for _, next := range coordination.TransitionsFrom(state) {
		if next == coordination.StateBlocked {
			return "task state <task-id> --to " + next.String() + " --session <session-id> --reason <reason>"
		}
	}
	panic("state has no executable non-terminal transition: " + state.String())
}

func driveTaskToState(t *testing.T, repo, task, session string, target coordination.State) {
	t.Helper()
	path := lifecyclePathTo(target)
	for _, next := range path[1:] {
		if next == coordination.StateCompleted {
			shown := run(t, repo, "task", "show", task, "--json")
			shown.requireExit(t, app.ExitSuccess)
			var current struct {
				Task struct {
					Revision int64 `json:"revision"`
				} `json:"task"`
			}
			decodeData(t, shown.stdout, &current)
			run(t, repo, "task", "complete", task, "--session", session,
				"--expect-revision", strconv.FormatInt(current.Task.Revision, 10), "--json").requireExit(t, app.ExitSuccess)
			continue
		}
		args := []string{"task", "state", task, "--to", next.String(), "--session", session, "--json"}
		if next == coordination.StateBlocked {
			args = append(args, "--reason", "waiting")
		}
		run(t, repo, args...).requireExit(t, app.ExitSuccess)
	}
}

func lifecyclePathTo(target coordination.State) []coordination.State {
	start := coordination.StateOpen
	queue := []coordination.State{start}
	previous := map[coordination.State]coordination.State{}
	seen := map[coordination.State]bool{start: true}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		if current == target {
			path := []coordination.State{current}
			for current != start {
				current = previous[current]
				path = append([]coordination.State{current}, path...)
			}
			return path
		}
		for _, next := range coordination.TransitionsFrom(current) {
			if !seen[next] {
				seen[next] = true
				previous[next] = current
				queue = append(queue, next)
			}
		}
	}
	panic("unreachable lifecycle state " + target.String())
}

// TestTaskCompleteCouplesTerminalStateToTheGate is REQ-006: a denied gate
// cannot release the READY holder or mutate its task, while a clean decision
// commits COMPLETED and the lease release through the existing atomic move.
func TestTaskCompleteCouplesTerminalStateToTheGate(t *testing.T) {
	for _, tc := range []struct {
		name     string
		required []string
		allow    bool
	}{
		{name: "denied evidence", required: []string{"missing-proof"}},
		{name: "clean allow", allow: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := newRepoWithCommit(t)
			if got := run(t, repo, "init", "--json"); got.code != app.ExitSuccess {
				t.Fatalf("init: %s", got.stdout)
			}
			gitCommitFile(t, repo, ".")
			session := sessionID(t, repo)
			task := openTask(t, repo, session, "evaluate terminal completion")
			for _, state := range []string{"CLAIMED", "IN_PROGRESS", "READY_TO_COMPLETE"} {
				run(t, repo, "task", "state", task, "--to", state, "--session", session, "--json").requireExit(t, app.ExitSuccess)
			}

			shown := run(t, repo, "task", "show", task, "--json")
			shown.requireExit(t, app.ExitSuccess)
			var before struct {
				Task struct {
					State    string `json:"state"`
					Revision int64  `json:"revision"`
				} `json:"task"`
			}
			decodeData(t, shown.stdout, &before)
			args := []string{"task", "complete", task, "--session", session, "--expect-revision", strconv.FormatInt(before.Task.Revision, 10), "--operation-id", "complete-1", "--json"}
			for _, profile := range tc.required {
				args = append(args, "--required", profile)
			}
			got := run(t, repo, args...)
			if tc.allow {
				got.requireExit(t, app.ExitSuccess)
				var completed struct {
					Allow bool `json:"allow"`
					Task  struct {
						State    string `json:"state"`
						Revision int64  `json:"revision"`
					} `json:"task"`
					Replayed bool `json:"replayed"`
				}
				decodeData(t, got.stdout, &completed)
				if !completed.Allow || completed.Task.State != "COMPLETED" || completed.Task.Revision != before.Task.Revision+1 {
					t.Fatalf("completion result = %+v, want allowed terminal revision", completed)
				}
				replay := run(t, repo, args...)
				replay.requireExit(t, app.ExitSuccess)
				decodeData(t, replay.stdout, &completed)
				if !completed.Replayed || completed.Task.Revision != before.Task.Revision+1 {
					t.Fatalf("completion replay = %+v, want original terminal result", completed)
				}
				changedRequired := append(append([]string{}, args...), "--required", "new-proof")
				changed := run(t, repo, changedRequired...)
				changed.requireExit(t, app.ExitFailed)
				if payload := changed.errorPayload(t); payload.Code != app.CodeOperationIDConflict {
					t.Fatalf("changed completion replay code = %q, want %q", payload.Code, app.CodeOperationIDConflict)
				}
			} else {
				got.requireExit(t, app.ExitDenied)
				payload := got.errorPayload(t)
				if payload.Code != app.CodeRequiredEvidenceNotCurrent {
					t.Fatalf("denial code = %q, want %q", payload.Code, app.CodeRequiredEvidenceNotCurrent)
				}
				current := run(t, repo, "task", "show", task, "--json")
				current.requireExit(t, app.ExitSuccess)
				decodeData(t, current.stdout, &before)
				if before.Task.State != "READY_TO_COMPLETE" {
					t.Errorf("denied task state = %s, want READY_TO_COMPLETE", before.Task.State)
				}
				leases := run(t, repo, "lease", "list", "--json")
				leases.requireExit(t, app.ExitSuccess)
				if !strings.Contains(leases.stdout, session) {
					t.Errorf("denial released the task lease:\n%s", leases.stdout)
				}
			}
		})
	}
}

func TestTaskCompleteRejectsExplicitBlankRequiredBeforeGate(t *testing.T) {
	repo := newInitializedRepo(t)
	session := sessionID(t, repo)
	task := openTask(t, repo, session, "blank required is invalid")
	for _, state := range []string{"CLAIMED", "IN_PROGRESS", "READY_TO_COMPLETE"} {
		run(t, repo, "task", "state", task, "--to", state, "--session", session, "--json").requireExit(t, app.ExitSuccess)
	}

	shown := run(t, repo, "task", "show", task, "--json")
	shown.requireExit(t, app.ExitSuccess)
	var before struct {
		Task struct {
			State    string `json:"state"`
			Revision int64  `json:"revision"`
		} `json:"task"`
	}
	decodeData(t, shown.stdout, &before)

	got := run(t, repo, "task", "complete", task, "--session", session,
		"--expect-revision", strconv.FormatInt(before.Task.Revision, 10), "--required", "", "--json")
	got.requireExit(t, app.ExitUsage)
	if payload := got.errorPayload(t); payload.Code != app.CodeCommandLineInvalid {
		t.Fatalf("blank required code = %q, want %q", payload.Code, app.CodeCommandLineInvalid)
	}

	after := run(t, repo, "task", "show", task, "--json")
	after.requireExit(t, app.ExitSuccess)
	decodeData(t, after.stdout, &before)
	if before.Task.State != "READY_TO_COMPLETE" {
		t.Fatalf("blank required moved task to %s", before.Task.State)
	}

	unicode := run(t, repo, "task", "complete", task, "--session", session,
		"--expect-revision", strconv.FormatInt(before.Task.Revision, 10), "--required", "İ", "--json")
	unicode.requireExit(t, app.ExitDenied)
	if payload := unicode.errorPayload(t); payload.Code != app.CodeRequiredEvidenceNotCurrent {
		t.Fatalf("unicode required code = %q, want %q", payload.Code, app.CodeRequiredEvidenceNotCurrent)
	}
}

// TestTaskCompleteFinalCASRejectsRevisionChangedDuringGate makes the final
// transition's optimistic lock observable. The trigger models an independent
// writer changing the task while completion captures its first guard baseline.
func TestTaskCompleteFinalCASRejectsRevisionChangedDuringGate(t *testing.T) {
	repo := newInitializedRepo(t)
	session := sessionID(t, repo)
	task := openTask(t, repo, session, "final completion CAS")
	for _, state := range []string{"CLAIMED", "IN_PROGRESS", "READY_TO_COMPLETE"} {
		run(t, repo, "task", "state", task, "--to", state, "--session", session, "--json").requireExit(t, app.ExitSuccess)
	}

	shown := run(t, repo, "task", "show", task, "--json")
	shown.requireExit(t, app.ExitSuccess)
	var before struct {
		Task struct {
			State    string `json:"state"`
			Revision int64  `json:"revision"`
		} `json:"task"`
	}
	decodeData(t, shown.stdout, &before)

	db, err := sql.Open("sqlite", "file:"+runtimeDBPath(t, repo))
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`CREATE TRIGGER completion_bumps_task_revision AFTER INSERT ON guard_baselines
BEGIN UPDATE tasks SET revision = revision + 1 WHERE task_id = '` + task + `'; END`)
	if closeErr := db.Close(); closeErr != nil && err == nil {
		err = closeErr
	}
	if err != nil {
		t.Fatalf("install revision trigger: %v", err)
	}

	got := run(t, repo, "task", "complete", task, "--session", session,
		"--expect-revision", strconv.FormatInt(before.Task.Revision, 10), "--json")
	got.requireExit(t, app.ExitFailed)
	if payload := got.errorPayload(t); payload.Code != app.CodeStateRevisionConflict {
		t.Fatalf("completion race code = %q, want %q", payload.Code, app.CodeStateRevisionConflict)
	}

	after := run(t, repo, "task", "show", task, "--json")
	after.requireExit(t, app.ExitSuccess)
	decodeData(t, after.stdout, &before)
	if before.Task.State != "READY_TO_COMPLETE" {
		t.Fatalf("revision race completed task: %s", before.Task.State)
	}
	if before.Task.Revision != 5 {
		t.Fatalf("revision after trigger = %d, want 5", before.Task.Revision)
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

// TestARefusedWriteMintsNoSession is finding F02, and it is the arm the test
// above was missing.
//
// That one counts sessions around the two reads and around a write that
// succeeds. Nothing counted them around a write that fails — and every one of
// these four refusals used to mint a session first, then refuse, then report
// `impact` as "Nothing was written". Thirty refusals in a loop left thirty
// rows, for thirty agent identities that never did anything, and no shipped
// command can list or remove one.
//
// The assertion has to be the row count. Two of these paths emit a
// byte-identical envelope whether the session was minted or not, so a test that
// read the payload would pass over the defect in either direction.
//
// The two blank-value cases also pin the envelope beyond the code. OpenTask and
// WriteCheckpoint each carry their own blank-value refusal with the identical
// code and why as the boundary's requireFlagText, so a missing
// requireFlagText call at the command line falls through to the store's guard
// and still reports CommandLineInvalid — only metadata.flag, the impact
// sentence and the second next_action disagree, and that deletion was
// invisible to all 19 packages until they were checked (audit round 2, §8 item
// 7).
func TestARefusedWriteMintsNoSession(t *testing.T) {
	for _, tc := range []struct {
		name string
		args func(task string) []string
		code app.Code

		// wantFlag is metadata.flag, and wantHelp is the command whose `--help`
		// requireFlagText's second next_action names. Both are empty for the
		// two cases that never reach requireFlagText, which skips the envelope
		// checks below for them.
		wantFlag string
		wantHelp string
	}{
		{
			name:     "task open with no title",
			args:     func(string) []string { return []string{"task", "open"} },
			code:     app.CodeCommandLineInvalid,
			wantFlag: "title",
			wantHelp: "mindrail task open --help",
		},
		{
			name:     "checkpoint write with no note",
			args:     func(task string) []string { return []string{"checkpoint", "write", task} },
			code:     app.CodeCommandLineInvalid,
			wantFlag: "note",
			wantHelp: "mindrail checkpoint write --help",
		},
		{
			name: "checkpoint write against a task that does not exist",
			args: func(string) []string {
				return []string{"checkpoint", "write", "TSK-0000000000000000000000000", "--note", "a note"}
			},
			code: app.CodeTaskNotFound,
		},
		{
			name: "task state moving somewhere the lifecycle does not go",
			args: func(task string) []string {
				return []string{"task", "state", task, "--to", "COMPLETED"}
			},
			code: app.CodeTaskStateInvalid,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := newInitializedRepo(t)
			session := sessionID(t, repo)
			task := openTask(t, repo, session, "a task in OPEN")

			before := sessionCount(t, repo)
			got := run(t, repo, append(tc.args(task), "--json")...)

			payload := got.errorPayload(t)
			if payload.Code != tc.code {
				t.Fatalf("code = %q, want %q\n%s", payload.Code, tc.code, got.stdout)
			}
			if after := sessionCount(t, repo); after != before {
				t.Errorf("a refusal that reports %q minted %d session(s); its impact says %q",
					payload.Code, after-before, payload.Impact)
			}

			if tc.wantFlag == "" {
				return
			}
			const wantImpact = "Mindrail did not run: nothing was read and nothing was written."
			if payload.Impact != wantImpact {
				t.Errorf("impact = %q, want %q", payload.Impact, wantImpact)
			}
			if got := payload.Metadata["flag"]; got != tc.wantFlag {
				t.Errorf("metadata.flag = %q, want %q", got, tc.wantFlag)
			}
			hasHelp := false
			for _, action := range payload.NextAction {
				if strings.Contains(action, tc.wantHelp) {
					hasHelp = true
					break
				}
			}
			if !hasHelp {
				t.Errorf("next_action = %v, want an entry naming `%s`", payload.NextAction, tc.wantHelp)
			}
		})
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
	return rowCount(t, repo, "sessions")
}

// rowCount reads one table's size straight from the runtime database.
//
// A refusal that emits a byte-identical error whether or not it wrote is
// invisible to any assertion over the envelope, so the tests that separate the
// two count rows instead.
func rowCount(t *testing.T, repo, table string) int {
	t.Helper()

	db, err := storage.Open(t.Context(), storage.Options{
		Path:     runtimeDBPath(t, repo),
		ReadOnly: true,
	})
	if err != nil {
		t.Fatalf("opening the runtime database: %v", err)
	}
	defer func() { _ = db.Close() }()

	// The table name is a constant at every call site; it cannot be a bound
	// parameter and there is no user input on this path.
	var count int
	if err := db.QueryRowContext(t.Context(), `SELECT count(*) FROM `+table).Scan(&count); err != nil {
		t.Fatalf("counting %s: %v", table, err)
	}
	return count
}

func remedyMentions(actions []string, needle string) bool {
	return strings.Contains(strings.Join(actions, " "), needle)
}

// TestInitAndStatusPublishTheSameCoordinationBlock is finding F10.
//
// `init` is idempotent and is the command a user runs on a repository that
// already exists. Its own report carries the same status block `status`
// publishes — and on the init path the summary was never read, so a re-init of
// a repository with work in flight printed "not observed — startup stopped
// before this subsystem was read" and three zeros, three lines above READY FOR
// TARGETED WORK, while `status` on the same bytes seconds later reported the
// task. The sequence had not stopped.
//
// The two blocks are compared as raw JSON rather than field by field, because
// the defect was in a field a field-by-field test would have had to think to
// include. Anything either command starts publishing is compared from here on.
func TestInitAndStatusPublishTheSameCoordinationBlock(t *testing.T) {
	repo := newInitializedRepo(t)
	session := sessionID(t, repo)
	task := openTask(t, repo, session, "work that was in flight when init ran again")
	run(t, repo, "checkpoint", "write", task, "--note", "half done", "--session", session, "--json").
		requireExit(t, app.ExitSuccess)

	reinit := run(t, repo, "init", "--json")
	reinit.requireExit(t, app.ExitSuccess)
	reported := run(t, repo, "status", "--json")
	reported.requireExit(t, app.ExitSuccess)

	var fromInit struct {
		Status struct {
			Readiness    string          `json:"readiness"`
			Coordination json.RawMessage `json:"coordination"`
		} `json:"status"`
	}
	var fromStatus struct {
		Readiness    string          `json:"readiness"`
		Coordination json.RawMessage `json:"coordination"`
	}
	decodeData(t, reinit.stdout, &fromInit)
	decodeData(t, reported.stdout, &fromStatus)

	if string(fromInit.Status.Coordination) != string(fromStatus.Coordination) {
		t.Errorf("the two commands describe one repository differently\n  init:   %s\n  status: %s",
			fromInit.Status.Coordination, fromStatus.Coordination)
	}
	if !strings.Contains(string(fromInit.Status.Coordination), `"observation":"observed"`) {
		t.Errorf("init says nobody looked at coordination on a run that completed: %s",
			fromInit.Status.Coordination)
	}

	// Decision D-62 in the other direction: reading the summary must not move
	// readiness, and a report that agreed by both saying "not observed" would
	// pass the comparison above.
	if fromInit.Status.Readiness != fromStatus.Readiness {
		t.Errorf("readiness = %q from init and %q from status", fromInit.Status.Readiness, fromStatus.Readiness)
	}
}

// TestStatusPublishesCoordinationWithoutMovingReadiness is AC-07.1 and AC-07.2,
// and decision D-62 in both directions.
//
// The block has to appear and the counts have to be right; readiness has to stay
// exactly where it was. A blocked task is a fact about work, not about the
// installation, and a tool that reported BLOCKED — the value reserved for "this
// repository cannot be verified" — because an agent parked a task would be
// unusable in the one situation the task was parked for.
func TestStatusPublishesCoordinationWithoutMovingReadiness(t *testing.T) {
	repo := newInitializedRepo(t)

	type statusData struct {
		Readiness    string `json:"readiness"`
		Coordination struct {
			Observation    string `json:"observation"`
			Open           int    `json:"tasks_open"`
			InProgress     int    `json:"tasks_in_progress"`
			Blocked        int    `json:"tasks_blocked"`
			LastCheckpoint *struct {
				TaskID    string `json:"task_id"`
				SessionID string `json:"session_id"`
			} `json:"last_checkpoint"`
		} `json:"coordination"`
	}

	read := func() statusData {
		t.Helper()
		got := run(t, repo, "status", "--json")
		got.requireExit(t, app.ExitSuccess)
		var data statusData
		decodeData(t, got.stdout, &data)
		return data
	}

	// Arm one: an initialised repository with no tasks. The counts are zero and
	// the block still says somebody looked, because "no work in flight" and
	// "nobody asked" are different answers.
	empty := read()
	before := empty.Readiness
	if empty.Coordination.Observation != "observed" {
		t.Errorf("observation = %q on an initialised repository, want observed", empty.Coordination.Observation)
	}
	if empty.Coordination.Open != 0 || empty.Coordination.LastCheckpoint != nil {
		t.Errorf("a repository with no tasks reports %+v", empty.Coordination)
	}

	// Arm two: one task in each of the three counted states, and a checkpoint.
	session := sessionID(t, repo)
	open := openTask(t, repo, session, "waiting to be picked up")
	_ = open

	working := openTask(t, repo, session, "being worked on")
	for _, to := range []string{"CLAIMED", "IN_PROGRESS"} {
		run(t, repo, "task", "state", working, "--to", to, "--session", session, "--json").
			requireExit(t, app.ExitSuccess)
	}

	stuck := openTask(t, repo, session, "waiting on a vendor patch")
	for _, to := range []string{"CLAIMED", "IN_PROGRESS"} {
		run(t, repo, "task", "state", stuck, "--to", to, "--session", session, "--json").
			requireExit(t, app.ExitSuccess)
	}
	run(t, repo, "task", "state", stuck, "--to", "BLOCKED",
		"--reason", "the vendor has not shipped it", "--session", session, "--json").
		requireExit(t, app.ExitSuccess)

	run(t, repo, "checkpoint", "write", stuck, "--note", "chased the vendor", "--session", session, "--json").
		requireExit(t, app.ExitSuccess)

	busy := read()
	if busy.Coordination.Open != 1 || busy.Coordination.InProgress != 1 || busy.Coordination.Blocked != 1 {
		t.Errorf("counts = open %d, in progress %d, blocked %d; want 1, 1, 1",
			busy.Coordination.Open, busy.Coordination.InProgress, busy.Coordination.Blocked)
	}
	if busy.Coordination.LastCheckpoint == nil {
		t.Fatal("status reports no checkpoint after one was written")
	}
	if busy.Coordination.LastCheckpoint.TaskID != stuck {
		t.Errorf("the newest checkpoint names %q, want %q", busy.Coordination.LastCheckpoint.TaskID, stuck)
	}

	// The whole point of the block: none of that moved readiness.
	if busy.Readiness != before {
		t.Errorf("readiness moved from %q to %q because tasks exist; coordination is not a component (D-62)",
			before, busy.Readiness)
	}
	if run(t, repo, "status", "--json").code != app.ExitSuccess {
		t.Error("status exited non-zero on a repository whose only unusual feature is a blocked task")
	}
}
