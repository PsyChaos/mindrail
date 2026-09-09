package coordination

import (
	"errors"
	"fmt"
	"strings"

	"github.com/PsyChaos/mindrail/internal/app"
)

// The conditions this package refuses on. They are sentinels so a caller can ask
// `errors.Is` without unwrapping a payload, and every published error below
// carries one as its cause so both routes agree.
var (
	// ErrSessionNotFound means a session handle names no session this repository
	// has minted. It is a refusal rather than a mint (decision D-61): minting
	// under a typo turns one agent into two.
	ErrSessionNotFound = errors.New("session not found")

	// ErrTaskNotFound means no task carries that id.
	ErrTaskNotFound = errors.New("task not found")

	// ErrCheckpointNotFound means a task has no checkpoint where one was
	// required. It is not the answer to `task show` on a fresh task: a task
	// nobody has checkpointed yet is normal, and Handover reports it as an
	// absent checkpoint rather than as a failure.
	ErrCheckpointNotFound = errors.New("checkpoint not found")

	// ErrTransitionNotAvailable means the move is not in decision D-55's table.
	ErrTransitionNotAvailable = errors.New("task state transition not available")

	// ErrBlockedReasonMissing means a task was sent to BLOCKED with nothing said
	// about why (decision D-63).
	ErrBlockedReasonMissing = errors.New("a blocked task needs a reason")
)

// sessionNotFound reports a session handle that names nothing.
func sessionNotFound(id string) error {
	return app.NewError(
		app.CodeSessionNotFound,
		app.KindFailed,
		fmt.Sprintf("no session in this repository carries the id %q", id),
		"The command was not attributed to an agent session, so nothing was written.",
		"Run `mindrail session open` and pass the id it prints as --session.",
		"Check the id for a transcription error; session ids are 26 characters after the SES- prefix.",
	).WithMetadata("session_id", id).WithCause(ErrSessionNotFound)
}

// taskNotFound reports a task id that names nothing.
func taskNotFound(id string) error {
	return app.NewError(
		app.CodeTaskNotFound,
		app.KindFailed,
		fmt.Sprintf("no task in this repository carries the id %q", id),
		"There is no task to read or move, so nothing was written.",
		"Run `mindrail task list` to see the tasks this repository has.",
	).WithMetadata("task_id", id).WithCause(ErrTaskNotFound)
}

// checkpointNotFound reports a task with no checkpoint where one was required.
func checkpointNotFound(taskID string) error {
	return app.NewError(
		app.CodeCheckpointNotFound,
		app.KindFailed,
		fmt.Sprintf("task %s has no checkpoint", taskID),
		"There is no handover note to read.",
		"Run `mindrail checkpoint write "+taskID+" --note \"...\"` to leave one.",
	).WithMetadata("task_id", taskID).WithCause(ErrCheckpointNotFound)
}

// transitionNotAvailable reports a move decision D-55's table does not have.
//
// The message names both states *and* what is available instead. A refusal that
// says only "you cannot do that" leaves the reader to guess, and the guess is
// usually a second wrong command; the alternatives come from TransitionsFrom, so
// this message cannot drift from the table it is describing.
func transitionNotAvailable(taskID string, from, to State) error {
	available := TransitionsFrom(from)

	var remedy string
	switch {
	case len(available) == 0:
		remedy = fmt.Sprintf("Task %s is %s, which is final; open a new task instead.", taskID, from)
	default:
		names := make([]string, 0, len(available))
		for _, state := range available {
			names = append(names, string(state))
		}
		remedy = fmt.Sprintf("From %s this task can move to: %s.", from, strings.Join(names, ", "))
	}

	return app.NewError(
		app.CodeTaskStateInvalid,
		app.KindFailed,
		fmt.Sprintf("task %s is %s and cannot move to %s", taskID, from, to),
		"The task was left exactly as it was; nothing was written.",
		remedy,
	).
		WithMetadata("task_id", taskID).
		WithMetadata("from_state", string(from)).
		WithMetadata("to_state", string(to)).
		WithCause(ErrTransitionNotAvailable)
}

// blockedReasonMissing reports a block with nothing said about why.
func blockedReasonMissing(taskID string) error {
	return app.NewError(
		app.CodeTaskStateInvalid,
		app.KindFailed,
		fmt.Sprintf("task %s cannot be blocked without a reason", taskID),
		"The task was left exactly as it was; nothing was written.",
		"Re-run with --reason describing what the task is waiting for.",
	).
		WithMetadata("task_id", taskID).
		WithMetadata("to_state", string(StateBlocked)).
		WithCause(ErrBlockedReasonMissing)
}

// writeFailed reports a coordination row that did not reach the database, after
// the storage layer has had its say.
//
// The named storage conditions come first for the reason MR-001's finding W2
// records: `init` against a database whose mode bits refuse writes used to exit
// 1 with the remedy "run doctor", and doctor — which only ever reads — called
// that same database healthy. A generic remedy is only correct for the failures
// that really are about the rows.
func writeFailed(what, id string, cause error) error {
	return app.NewError(
		app.CodeCoordinationWriteFailed,
		app.KindFailed,
		fmt.Sprintf("%s could not be written to the runtime database", what),
		"The coordination state is unchanged, so the next agent will not see this step.",
		"Run `mindrail doctor` to check the runtime database, then re-run the command.",
	).WithMetadata("subject_id", id).WithCause(cause)
}
