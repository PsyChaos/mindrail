package cli

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/coordination"
	"github.com/PsyChaos/mindrail/internal/gate"
)

// newTaskCommand builds `mindrail task`.
func newTaskCommand(o Options) *cobra.Command {
	group := &cobra.Command{
		Use:   "task",
		Short: "Open, move and read the tasks two sequential agents share",
		Long: `Manage tasks.

A task is the unit of work an agent claims and a later agent continues. It
belongs to the project — every worktree of this repository sees the same tasks —
and it moves through the lifecycle spec §58 defines. ` + "`task state --to`" + ` owns
non-terminal moves; ` + "`task complete`" + ` owns the evaluated COMPLETED edge.`,
	}
	group.AddCommand(
		newTaskOpenCommand(o),
		newTaskStateCommand(o),
		newTaskCompleteCommand(o),
		newTaskShowCommand(o),
		newTaskListCommand(o),
	)
	return group
}

// newTaskCompleteCommand evaluates the canonical completion gate before the
// one terminal lifecycle transition. Raw task state mutation cannot complete a
// task; on DENY this command returns without changing the task or its lease.
func newTaskCompleteCommand(o Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "complete <task-id>",
		Short: "Evaluate completion, then atomically complete a ready task",
		Long: `Evaluate the completion gate for a READY_TO_COMPLETE task.

The task is marked COMPLETED and its task lease released only after the gate
allows it. A denial leaves the task and lease unchanged.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			handle, _ := cmd.Flags().GetString(flagSession)
			if refusal := requireFlagText(cmd, flagSession, handle,
				"completion needs the session that holds the task lease",
				"Re-run with --session set to the holder shown by `mindrail task show <task-id>`."); refusal != nil {
				return refuseBeforeStarting(cmd, "task complete", o, refusal)
			}
			expect, refusal := expectRevisionFlag(cmd)
			if refusal != nil {
				return refuseBeforeStarting(cmd, "task complete", o, refusal)
			}
			if expect == 0 {
				return refuseBeforeStarting(cmd, "task complete", o, app.NewError(
					app.CodeCommandLineInvalid, app.KindUsage,
					"completion needs --expect-revision",
					"The final task transition must reject a task that changed while completion was evaluated.",
					"Pass --expect-revision as the revision `mindrail task show <task-id>` printed, then retry.",
				).WithMetadata("flag", flagExpectRevision))
			}
			operation, refusal := operationIDFlag(cmd)
			if refusal != nil {
				return refuseBeforeStarting(cmd, "task complete", o, refusal)
			}
			rawRequired, _ := cmd.Flags().GetStringSlice("required")
			if cmd.Flags().Changed("required") && len(rawRequired) == 0 {
				return refuseBeforeStarting(cmd, "task complete", o, app.NewError(
					app.CodeCommandLineInvalid, app.KindUsage,
					"a required validation profile cannot be empty or blank",
					"Completion was not evaluated and the task and lease were left unchanged.",
					"Pass a non-blank profile name after --required, or omit --required when no profile is required."))
			}
			required, refusal := coordination.CanonicalRequiredProfiles(rawRequired)
			if refusal != nil {
				return refuseBeforeStarting(cmd, "task complete", o, refusal)
			}

			return runCoordination(cmd, "task complete", o,
				func(ctx context.Context, s scope) (any, humanRenderer, error) {
					task, err := s.store.FindTask(ctx, args[0])
					if err != nil {
						return nil, nil, err
					}
					if task.ProjectID != s.projectID() {
						return nil, nil, app.NewError(app.CodeTaskNotFound, app.KindFailed,
							"the task does not belong to this repository's project",
							"Completion cannot cross project boundaries.",
							"Run `mindrail task list` in the project that owns the task and use its task id.",
						).WithMetadata("task_id", args[0])
					}
					// TransitionExpecting checks its idempotency record before the
					// terminal-state and revision guards. Reaching it here preserves a
					// lost-response retry of an already allowed completion without
					// re-evaluating a task that is now terminal.
					if task.State == coordination.StateCompleted && operation != "" {
						move, write, err := s.store.Idempotent(operation).CompleteExpecting(ctx, task.ID, s.attribution(handle), expect, required)
						if err != nil {
							return nil, nil, err
						}
						result := completionResult{
							Allow: true, Denials: []gate.Denial{}, Task: move.Task, Lease: move.Lease, Superseded: move.Superseded,
							attributed: attributed{Session: write.Session, SessionMinted: write.Minted, Replayed: write.Replayed, OperationID: write.OperationID},
						}
						return result, result.RenderHuman, nil
					}
					if task.State != coordination.StateReadyToComplete {
						return nil, nil, app.NewError(app.CodeTaskStateInvalid, app.KindFailed,
							"task is "+task.State.String()+"; completion needs READY_TO_COMPLETE",
							"Completion did not run and the task lease was left unchanged.",
							"Move the task to READY_TO_COMPLETE through its working lifecycle, then retry completion.",
						).WithMetadata("task_id", task.ID).WithMetadata("state", task.State.String())
					}
					if task.Revision != expect {
						return nil, nil, revisionChanged(task, expect)
					}

					decision, err := evaluateTaskCompletion(ctx, s, task.ID, required)
					if err != nil {
						return nil, nil, err
					}
					if !decision.Allow {
						result := completionResult{Allow: false, Denials: decision.Denials, Task: task}
						return result, result.RenderHuman, completionDenied(decision)
					}

					move, write, err := s.store.Idempotent(operation).CompleteExpecting(ctx, task.ID, s.attribution(handle), expect, required)
					if err != nil {
						return nil, nil, err
					}
					result := completionResult{
						Allow: true, Denials: []gate.Denial{}, Task: move.Task, Lease: move.Lease, Superseded: move.Superseded,
						attributed: attributed{Session: write.Session, SessionMinted: write.Minted, Replayed: write.Replayed, OperationID: write.OperationID},
					}
					return result, result.RenderHuman, nil
				})
		},
	}
	cmd.Flags().String(flagSession, "", "the session currently holding the task lease")
	cmd.Flags().String(flagExpectRevision, "", "the revision `task show` printed; required to protect the final transition")
	cmd.Flags().String(flagOperationID, "", "an id this request carries, so a retry can replay the terminal transition")
	cmd.Flags().StringSlice("required", nil, "validation profile that must have current successful evidence; repeat for more than one")
	return cmd
}

func revisionChanged(task coordination.Task, expected int64) error {
	return app.NewError(app.CodeStateRevisionConflict, app.KindFailed,
		fmt.Sprintf("task revision is %d, not the expected %d", task.Revision, expected),
		"Completion was not evaluated against a task state that can still be completed.",
		"Run `mindrail task show "+task.ID+"` for the current revision, then re-run completion.",
	).WithMetadata("task_id", task.ID).
		WithMetadata("expected_revision", fmt.Sprint(expected)).
		WithMetadata("current_revision", fmt.Sprint(task.Revision))
}

func completionDenied(decision gate.Decision) error {
	if len(decision.Denials) == 0 {
		return app.NewError(app.CodeRequiredEvidenceNotCurrent, app.KindDenied,
			"completion was denied without a reported gate reason",
			"The task and lease were left unchanged.",
			"Inspect repository state and retry completion once the gate can produce a decision.")
	}
	first := decision.Denials[0]
	return app.NewError(first.Code, app.KindDenied,
		"completion denied: "+first.Reason,
		"The task and lease were left unchanged.",
		append(first.NextAction, "Resolve every listed denial, then retry `mindrail task complete`.")...,
	)
}

type completionResult struct {
	Allow      bool                `json:"allow"`
	Denials    []gate.Denial       `json:"denials"`
	Task       coordination.Task   `json:"task"`
	Lease      *coordination.Lease `json:"lease"`
	Superseded *coordination.Lease `json:"superseded,omitempty"`
	attributed
}

func (r completionResult) RenderHuman(w io.Writer, color bool) error {
	if !r.Allow {
		_, err := fmt.Fprintf(w, "Completion for task %s was denied (%d finding(s)).\n", r.Task.ID, len(r.Denials))
		return err
	}
	return taskResult{Task: r.Task, Lease: r.Lease, Superseded: r.Superseded, attributed: r.attributed}.RenderHuman(w, color)
}

func newTaskOpenCommand(o Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "open",
		Short: "Open a task in OPEN",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			title, _ := cmd.Flags().GetString(flagTitle)
			handle, _ := cmd.Flags().GetString(flagSession)

			// The title is judged before the application starts, beside the
			// `--to` parse below. A missing title is a mistake in the command
			// line, and nothing has to be opened to know it.
			if refusal := requireFlagText(cmd, flagTitle, title,
				"a task needs a title",
				"Re-run with --title describing the work in one line."); refusal != nil {
				return refuseBeforeStarting(cmd, "task open", o, refusal)
			}

			operation, refusal := operationIDFlag(cmd)
			if refusal != nil {
				return refuseBeforeStarting(cmd, "task open", o, refusal)
			}

			return runCoordination(cmd, "task open", o,
				func(ctx context.Context, s scope) (any, humanRenderer, error) {
					task, write, err := s.store.Idempotent(operation).OpenTask(ctx, s.projectID(), s.attribution(handle), title)
					if err != nil {
						return nil, nil, err
					}
					result := taskResult{Task: task, attributed: attributed{Session: write.Session, SessionMinted: write.Minted, Replayed: write.Replayed, OperationID: write.OperationID}}
					return result, result.RenderHuman, nil
				})
		},
	}
	cmd.Flags().String(flagTitle, "", "what the task is, in one line")
	cmd.Flags().String(flagOperationID, "", "an id this request carries, so a retry of it is answered from the first delivery")
	cmd.Flags().String(flagSession, "", "the session id this run is attributed to; one is minted if omitted")
	return cmd
}

func newTaskStateCommand(o Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "state <task-id>",
		Short: "Move a task through its lifecycle",
		Long: `Move a task to another state.

This command owns non-terminal task lifecycle transitions. ` + "`task complete`" + ` owns
the evaluated COMPLETED edge; passing --to COMPLETED here is always refused.
A move the lifecycle does not have is refused by name, and the refusal lists the
moves that are available from where the task is.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			raw, _ := cmd.Flags().GetString(flagTo)
			reason, _ := cmd.Flags().GetString(flagReason)
			handle, _ := cmd.Flags().GetString(flagSession)

			// The state is parsed before the application starts. A word this
			// binary does not have is a mistake in the command line, and
			// discovering that after opening a database would report it as
			// something that happened during the operation.
			to, err := parseStateFlag(cmd, raw)
			if err != nil {
				return refuseBeforeStarting(cmd, "task state", o, err)
			}
			operation, refusal := operationIDFlag(cmd)
			if refusal != nil {
				return refuseBeforeStarting(cmd, "task state", o, refusal)
			}
			expect, refusal := expectRevisionFlag(cmd)
			if refusal != nil {
				return refuseBeforeStarting(cmd, "task state", o, refusal)
			}

			return runCoordination(cmd, "task state", o,
				func(ctx context.Context, s scope) (any, humanRenderer, error) {
					if to == coordination.StateCompleted {
						task, err := s.store.FindTask(ctx, args[0])
						if err != nil {
							return nil, nil, err
						}
						return nil, nil, rawCompletionRefusal(task.State)
					}
					move, write, err := s.store.Idempotent(operation).TransitionExpecting(ctx, args[0], s.attribution(handle), to, reason, expect)
					if err != nil {
						return nil, nil, err
					}
					result := taskResult{
						Task:       move.Task,
						Lease:      move.Lease,
						Superseded: move.Superseded,
						attributed: attributed{Session: write.Session, SessionMinted: write.Minted, Replayed: write.Replayed, OperationID: write.OperationID},
					}
					return result, result.RenderHuman, nil
				})
		},
	}
	cmd.Flags().String(flagTo, "", "the state to move to: "+stateList())
	cmd.Flags().String(flagReason, "", "why, required when moving to BLOCKED")
	cmd.Flags().String(flagOperationID, "", "an id this request carries, so a retry of it is answered from the first delivery")
	cmd.Flags().String(flagExpectRevision, "", "the revision `task show` printed; the move is refused if the task has moved on")
	cmd.Flags().String(flagSession, "", "the session id this run is attributed to; one is minted if omitted")
	return cmd
}

// rawCompletionRefusal keeps terminal state persistence coupled to the
// evaluated completion gate. `task state` still owns every non-terminal
// lifecycle edge; `task complete` owns the one terminal edge.
func rawCompletionRefusal(from coordination.State) error {
	var next []string
	switch {
	case from == coordination.StateReadyToComplete:
		next = []string{
			"Run `mindrail task complete <task-id> --session <session-id> --expect-revision <revision>` to evaluate the completion gate.",
			"From READY_TO_COMPLETE this task can move to: IN_PROGRESS, ABANDONED.",
		}
	case from.Terminal():
		next = []string{
			"Run `mindrail task show <task-id>` to inspect this terminal task.",
			"Open a new task with `mindrail task open --title <title>` for new work.",
		}
	default:
		names := nonTerminalTransitions(from)
		next = []string{
			"Run `mindrail task state <task-id> --to " + names[0] + " --session <session-id>` to continue this task.",
			"From " + from.String() + " this task can move to: " + strings.Join(names, ", ") + ".",
		}
	}

	return app.NewError(
		app.CodeTaskStateInvalid,
		app.KindFailed,
		"COMPLETED can only be written by the evaluated completion command",
		"Completion evidence has not been evaluated; the task state and its lease were left unchanged.",
		next...,
	)
}

func nonTerminalTransitions(from coordination.State) []string {
	names := make([]string, 0, len(coordination.TransitionsFrom(from)))
	for _, state := range coordination.TransitionsFrom(from) {
		if state != coordination.StateCompleted && state != coordination.StateBlocked {
			names = append(names, state.String())
		}
	}
	return names
}

func newTaskShowCommand(o Options) *cobra.Command {
	return &cobra.Command{
		Use:   "show <task-id>",
		Short: "Report a task and the newest checkpoint on it",
		Long: `Report a task and the newest checkpoint on it.

This is the arriving agent's whole question in one call: what was being done, how
far it got, and what the last agent said about it. It writes nothing, and it
mints no session.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCoordination(cmd, "task show", o,
				func(ctx context.Context, s scope) (any, humanRenderer, error) {
					handover, err := s.store.Handover(ctx, args[0])
					if err != nil {
						return nil, nil, err
					}
					result := handoverResult{Handover: handover}
					return result, result.RenderHuman, nil
				})
		},
	}
}

func newTaskListCommand(o Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List this project's tasks, newest first",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			raw, _ := cmd.Flags().GetString(flagState)

			var filter coordination.State
			if raw != "" {
				parsed, err := parseStateFlag(cmd, raw)
				if err != nil {
					inv, invErr := newInvocation(cmd, "task list", o)
					if invErr != nil {
						return inv.emit(nil, nil, nil, "", invErr)
					}
					return inv.emit(nil, nil, nil, "", err)
				}
				filter = parsed
			}

			return runCoordination(cmd, "task list", o,
				func(ctx context.Context, s scope) (any, humanRenderer, error) {
					tasks, err := s.store.ListTasks(ctx, s.projectID(), filter)
					if err != nil {
						return nil, nil, err
					}
					result := taskListResult{Tasks: tasks}
					return result, result.RenderHuman, nil
				})
		},
	}
	cmd.Flags().String(flagState, "", "show only tasks in this state: "+stateList())
	return cmd
}

// parseStateFlag turns a --to or --state value into a State, or into the usage
// error a misspelled command line deserves.
//
// The domain parser returns a plain error on purpose — whether a bad value is a
// usage mistake is a question about the caller, not about the value — and this
// is where this binary answers it. Exit 2 is decision D-03's class for a command
// line it could not understand, and every other spelling mistake in this tree
// already takes it.
func parseStateFlag(cmd *cobra.Command, raw string) (coordination.State, error) {
	state, err := coordination.ParseState(raw)
	if err == nil {
		return state, nil
	}

	return "", app.NewError(
		app.CodeCommandLineInvalid,
		app.KindUsage,
		err.Error(),
		"Mindrail did not run: no task was read and nothing was written.",
		"Re-run with one of: "+stateList()+".",
		"Run `"+cmd.CommandPath()+" --help` to see the accepted flags.",
	)
}

// stateList renders the vocabulary for help text and for a refusal. It reads
// coordination.States() rather than spelling the seven out, because a second
// copy of the lifecycle is what decision D-55 keeps out of this package.
func stateList() string {
	states := coordination.States()
	names := make([]string, 0, len(states))
	for _, state := range states {
		names = append(names, string(state))
	}
	return strings.Join(names, ", ")
}

// taskResult is what the two writing task commands publish.
//
// SessionMinted is on the wire rather than implied, because "you are working
// under a session you did not name" is something an agent has to be able to
// notice: one that wanted continuity and forgot --session would otherwise carry
// on under a fresh identity in silence. Lease is the task's lease as the write
// left it — null after a release or for a fresh task — and Superseded the
// expired tenure a move took over, when it did (decision D-67: reported, never
// silent).
type taskResult struct {
	Task       coordination.Task   `json:"task"`
	Lease      *coordination.Lease `json:"lease"`
	Superseded *coordination.Lease `json:"superseded,omitempty"`
	attributed
}

func (r taskResult) RenderHuman(w io.Writer, _ bool) error {
	if _, err := fmt.Fprintf(w, "Task %s is %s (revision %d).\n  %s\n", r.Task.ID, r.Task.State, r.Task.Revision, r.Task.Title); err != nil {
		return err
	}
	if r.Task.BlockedReason != "" {
		if _, err := fmt.Fprintf(w, "  Blocked on: %s\n", r.Task.BlockedReason); err != nil {
			return err
		}
	}
	if r.Lease != nil {
		if _, err := fmt.Fprintf(w, "  Lease %s: held by session %s until %s.\n",
			r.Lease.ID, r.Lease.Holder, app.FormatTime(r.Lease.ExpiresAt)); err != nil {
			return err
		}
	}
	if r.Superseded != nil {
		if _, err := fmt.Fprintf(w, "  Took over from session %s, whose lease expired at %s.\n",
			r.Superseded.Holder, app.FormatTime(r.Superseded.ExpiresAt)); err != nil {
			return err
		}
	}
	return r.renderAttribution(w)
}

// handoverResult is what `task show` publishes.
type handoverResult struct {
	coordination.Handover
}

func (r handoverResult) RenderHuman(w io.Writer, _ bool) error {
	task := r.Task
	if _, err := fmt.Fprintf(w, "Task %s is %s (revision %d).\n  %s\n", task.ID, task.State, task.Revision, task.Title); err != nil {
		return err
	}
	if task.BlockedReason != "" {
		if _, err := fmt.Fprintf(w, "  Blocked on: %s\n", task.BlockedReason); err != nil {
			return err
		}
	}
	if task.ClaimedBy != "" {
		if _, err := fmt.Fprintf(w, "  Claimed by: %s\n", task.ClaimedBy); err != nil {
			return err
		}
	}
	// The lease beside the claimant, in whatever status it is, so a reader is
	// never shown a claimant as if the claim still bound (decision D-68).
	if r.Lease != nil {
		if _, err := fmt.Fprintf(w, "  Lease %s:\n", r.Lease.ID); err != nil {
			return err
		}
		if err := renderLeaseLine(w, *r.Lease); err != nil {
			return err
		}
	}

	if r.Checkpoint == nil {
		_, err := io.WriteString(w, "\nNo checkpoint has been written on this task yet.\n")
		return err
	}

	marker := ""
	if r.Checkpoint.Handoff {
		marker = " (handoff)"
	}
	_, err := fmt.Fprintf(w, "\nLast checkpoint%s, written by session %s:\n  %s\n",
		marker, r.Checkpoint.SessionID, r.Checkpoint.Note)
	return err
}

// taskListResult is what `task list` publishes. Tasks is never nil so the
// marshalled report says [] rather than null for a project with no tasks.
type taskListResult struct {
	Tasks []coordination.Task `json:"tasks"`
}

func (r taskListResult) RenderHuman(w io.Writer, _ bool) error {
	if len(r.Tasks) == 0 {
		_, err := io.WriteString(w, "This project has no tasks.\n")
		return err
	}
	for _, task := range r.Tasks {
		if _, err := fmt.Fprintf(w, "%s  %-17s  %s\n", task.ID, task.State, task.Title); err != nil {
			return err
		}
	}
	return nil
}
