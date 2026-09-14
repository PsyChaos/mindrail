package coordination

import (
	"errors"
	"fmt"
	"strconv"
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
func transitionNotAvailable(taskID string, from, to State, claimedBy string) error {
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

	// Decision D-58 promises that a second claim on a claimed task says which
	// session holds it, and nothing said so: the refusal named the two states
	// and stopped there, which tells an arriving agent that the task is taken
	// and not who to hand over to — on the one command group whose whole
	// purpose is handover (finding F22). The clause is about the claim, so the
	// holder is named where there is one; an unclaimed task in a state that
	// refuses the move has no holder to name and inventing one would be worse
	// than saying nothing.
	why := fmt.Sprintf("task %s is %s and cannot move to %s", taskID, from, to)
	if claimedBy != "" {
		why += fmt.Sprintf("; it is claimed by session %s", claimedBy)
	}

	failure := app.NewError(
		app.CodeTaskStateInvalid,
		app.KindFailed,
		why,
		"The task was left exactly as it was; nothing was written.",
		remedy,
	).
		WithMetadata("task_id", taskID).
		WithMetadata("from_state", string(from)).
		WithMetadata("to_state", string(to)).
		WithCause(ErrTransitionNotAvailable)

	if claimedBy != "" {
		failure = failure.WithMetadata("claimed_by", claimedBy)
	}
	return failure
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

// noWorkspace reports a session asked for without a worktree to attribute it to.
//
// It is not reachable from today's CLI — coordinationScope refuses an
// unregistered worktree before either call site is reached — and it was a bare
// fmt.Errorf for exactly that reason: nothing could see it, so nothing made it
// answerable (finding F26). AC-11.4 makes this package the surface MR-014's MCP
// tools call directly, with no coordinationScope in front of it, so the guard is
// coded rather than deleted.
func noWorkspace(what string) error {
	return app.NewError(
		app.CodeCoordinationUnavailable,
		app.KindFailed,
		what+" needs a worktree to belong to, and none was given",
		"Nothing was read and nothing was written.",
		"Pass the id of a worktree registered in this repository's runtime database.",
		"Run `mindrail init` in the worktree if it has never been registered.",
	)
}

// readFailed reports a row the runtime database holds and could not answer for.
//
// The read paths had no error of their own: every write was wrapped and every
// read returned a bare fmt.Errorf, which `emit` published verbatim, so one
// unparseable timestamp reached the user as a Go parser message with a format
// string in it and an empty code, impact and next_action (finding F44). The
// contrast that decides it is internal/workspace, which has the same bare read
// errors and still reaches the user coded, because its caller wraps them —
// MR-003's commands have no such caller.
func readFailed(what, id string, cause error) error {
	return app.NewError(
		app.CodeCoordinationReadFailed,
		app.KindFailed,
		fmt.Sprintf("%s could not be read from the runtime database", what),
		"Nothing was written. What the repository holds could not be reported, "+
			"so this answer says nothing about the work in flight.",
		"Run `mindrail doctor` to check the runtime database, then re-run the command.",
	).WithMetadata("subject_id", id).WithCause(cause)
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

// The lease conditions (MR-004, decision D-76). Three codes rather than one
// with a flag because a caller's next action differs: on a conflict it waits
// or asks the holder; on a lease it no longer holds it acquires again; on an
// unknown id it looks the id up.
var (
	// ErrLeaseConflict means the target's active lease is held by another
	// session. It is the milestone's scenario refused by name: the second
	// agent is told who holds the target and until when, never to force.
	ErrLeaseConflict = errors.New("lease held by another session")

	// ErrLeaseNotHeld means the lease the caller named has expired or been
	// released, so there is nothing to renew or give up.
	ErrLeaseNotHeld = errors.New("lease no longer held")

	// ErrLeaseNotFound means no lease carries that id.
	ErrLeaseNotFound = errors.New("lease not found")
)

// leaseConflict reports a target another session holds. The expiry is named
// because it is the one fact that tells the reader how long "wait" is, and
// the lease id because `lease release` takes it.
func leaseConflict(lease Lease) error {
	return app.NewError(
		app.CodeLeaseConflict,
		app.KindFailed,
		fmt.Sprintf("%s is held by session %s until %s", lease.Target(), lease.Holder, app.FormatTime(lease.ExpiresAt)),
		"Nothing was written; the lease stays with its holder.",
		fmt.Sprintf("Wait until %s, when the lease expires, then try again.", app.FormatTime(lease.ExpiresAt)),
		"Or ask the holder to release it: `mindrail lease release "+lease.ID+"`.",
	).
		WithMetadata("lease_id", lease.ID).
		WithMetadata("holder", lease.Holder).
		WithMetadata("expires_at", app.FormatTime(lease.ExpiresAt)).
		WithMetadata("target_kind", string(lease.TargetKind)).
		WithMetadata("target_key", lease.TargetKey).
		WithCause(ErrLeaseConflict)
}

// leaseNotHeld reports a renew or a release of a lease that has already
// ended. The remedy depends on the kind, because a file is taken again with
// `lease acquire` and a task by moving it (decision D-66).
func leaseNotHeld(lease Lease, verb string) error {
	var why string
	switch lease.Status {
	case LeaseReleased:
		when := ""
		if lease.ReleasedAt != nil {
			when = " at " + app.FormatTime(*lease.ReleasedAt)
		}
		why = fmt.Sprintf("lease %s on %s was released%s (%s), so there is nothing to %s",
			lease.ID, lease.Target(), when, lease.ReleaseReason, verb)
	default:
		why = fmt.Sprintf("lease %s on %s expired at %s, so there is nothing to %s",
			lease.ID, lease.Target(), app.FormatTime(lease.ExpiresAt), verb)
	}

	var remedy string
	switch lease.TargetKind {
	case TargetTask:
		remedy = "Move the task with `mindrail task state " + lease.TargetKey + " --to <STATE>`; the move takes the lease again."
	case TargetFile:
		remedy = "Run `mindrail lease acquire --file=" + ShellArgument(lease.TargetKey) + "` to take it again."
	default:
		// A kind this binary does not acquire — a row planted by hand, or one
		// a later binary wrote — gets the sentence that is true of every kind
		// rather than a command that would be wrong for it.
		remedy = "Acquire the target again with the command that takes a " + string(lease.TargetKind) + " lease."
	}

	return app.NewError(
		app.CodeLeaseNotHeld,
		app.KindFailed,
		why,
		"Nothing was written; the lease's history is unchanged.",
		remedy,
	).
		WithMetadata("lease_id", lease.ID).
		WithMetadata("status", string(lease.Status)).
		WithMetadata("target_kind", string(lease.TargetKind)).
		WithMetadata("target_key", lease.TargetKey).
		WithCause(ErrLeaseNotHeld)
}

// leaseNotFound reports a lease id that names nothing.
func leaseNotFound(id string) error {
	return app.NewError(
		app.CodeLeaseNotFound,
		app.KindFailed,
		fmt.Sprintf("no lease in this repository carries the id %q", id),
		"There is no lease to read, renew or release, so nothing was written.",
		"Run `mindrail lease list` to see the leases this project holds.",
	).WithMetadata("lease_id", id).WithCause(ErrLeaseNotFound)
}

// noProject is noWorkspace's sibling for the writers that belong to a project
// rather than a worktree: a lease acquired for no project would be a row the
// unique index scopes to nothing.
func noProject(what string) error {
	return app.NewError(
		app.CodeCoordinationUnavailable,
		app.KindFailed,
		what+" needs a project to belong to, and none was given",
		"Nothing was read and nothing was written.",
		"Pass the id of a project registered in this repository's runtime database.",
		"Run `mindrail init` in the worktree if it has never been registered.",
	)
}

// ErrRevisionConflict means the task is not at the revision the caller
// expected (decision D-72): the move was decided on a reading that is no
// longer current, and applying it would be the silent last-write-wins spec
// §11 forbids.
var ErrRevisionConflict = errors.New("task revision conflict")

// revisionConflict reports a stale expectation. It names the revision and the
// state the task actually has, because the caller's next step is to re-read
// and decide again, and the remedy hands it the command that re-reads.
func revisionConflict(task Task, expected int64) error {
	return app.NewError(
		app.CodeStateRevisionConflict,
		app.KindFailed,
		fmt.Sprintf("task %s is at revision %d and %s, not at revision %d", task.ID, task.Revision, task.State, expected),
		"The task was left exactly as it was; nothing was written. The move was decided on a reading of the task that is no longer current.",
		"Run `mindrail task show "+task.ID+"`, then re-run the move with --expect-revision "+
			strconv.FormatInt(task.Revision, 10)+" if it still applies.",
	).
		WithMetadata("task_id", task.ID).
		WithMetadata("expected_revision", strconv.FormatInt(expected, 10)).
		WithMetadata("current_revision", strconv.FormatInt(task.Revision, 10)).
		WithMetadata("state", string(task.State)).
		WithCause(ErrRevisionConflict)
}

// taskNotClaimableWhereItStands reports a `lease acquire --task` on a task
// that is not in a working state: an OPEN task is claimed by moving it to
// CLAIMED, and a finished one is not claimed at all (decision D-66 as
// amended). It is TASK_STATE_INVALID, the code for a move the lifecycle does
// not have, because that is what it is.
func taskNotClaimableWhereItStands(task Task) error {
	var remedy string
	switch task.State {
	case StateOpen:
		remedy = "Run `mindrail task state " + task.ID + " --to CLAIMED`; the claim is the move, and the move takes the lease."
	default:
		remedy = fmt.Sprintf("Task %s is %s, which is final; open a new task instead.", task.ID, task.State)
	}
	return app.NewError(
		app.CodeTaskStateInvalid,
		app.KindFailed,
		fmt.Sprintf("task %s is %s, and a task is taken where it stands only in a working state", task.ID, task.State),
		"The task was left exactly as it was; nothing was written.",
		remedy,
	).
		WithMetadata("task_id", task.ID).
		WithMetadata("from_state", string(task.State)).
		WithCause(ErrTransitionNotAvailable)
}
