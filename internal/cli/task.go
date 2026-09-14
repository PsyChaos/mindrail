package cli

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/coordination"
)

// newTaskCommand builds `mindrail task`.
func newTaskCommand(o Options) *cobra.Command {
	group := &cobra.Command{
		Use:   "task",
		Short: "Open, move and read the tasks two sequential agents share",
		Long: `Manage tasks.

A task is the unit of work an agent claims and a later agent continues. It
belongs to the project — every worktree of this repository sees the same tasks —
and it moves through the lifecycle spec §58 defines. Every move goes through
` + "`task state --to`" + `, so the lifecycle is stated in one place.`,
	}
	group.AddCommand(
		newTaskOpenCommand(o),
		newTaskStateCommand(o),
		newTaskShowCommand(o),
		newTaskListCommand(o),
	)
	return group
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

			return runCoordination(cmd, "task open", o,
				func(ctx context.Context, s scope) (any, humanRenderer, error) {
					task, write, err := s.store.OpenTask(ctx, s.projectID(), s.attribution(handle), title)
					if err != nil {
						return nil, nil, err
					}
					result := taskResult{Task: task, Session: write.Session, SessionMinted: write.Minted}
					return result, result.RenderHuman, nil
				})
		},
	}
	cmd.Flags().String(flagTitle, "", "what the task is, in one line")
	cmd.Flags().String(flagSession, "", "the session id this run is attributed to; one is minted if omitted")
	return cmd
}

func newTaskStateCommand(o Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "state <task-id>",
		Short: "Move a task through its lifecycle",
		Long: `Move a task to another state.

This is the only command that changes a task's state, so spec §58's lifecycle is
consulted in exactly one place. A move the lifecycle does not have is refused by
name, and the refusal lists the moves that are available from where the task is.`,
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

			return runCoordination(cmd, "task state", o,
				func(ctx context.Context, s scope) (any, humanRenderer, error) {
					move, write, err := s.store.Transition(ctx, args[0], s.attribution(handle), to, reason)
					if err != nil {
						return nil, nil, err
					}
					result := taskResult{Task: move.Task, Session: write.Session, SessionMinted: write.Minted}
					return result, result.RenderHuman, nil
				})
		},
	}
	cmd.Flags().String(flagTo, "", "the state to move to: "+stateList())
	cmd.Flags().String(flagReason, "", "why, required when moving to BLOCKED")
	cmd.Flags().String(flagSession, "", "the session id this run is attributed to; one is minted if omitted")
	return cmd
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
// on under a fresh identity in silence.
type taskResult struct {
	Task          coordination.Task    `json:"task"`
	Session       coordination.Session `json:"session"`
	SessionMinted bool                 `json:"session_minted"`
}

func (r taskResult) RenderHuman(w io.Writer, _ bool) error {
	if _, err := fmt.Fprintf(w, "Task %s is %s.\n  %s\n", r.Task.ID, r.Task.State, r.Task.Title); err != nil {
		return err
	}
	if r.Task.BlockedReason != "" {
		if _, err := fmt.Fprintf(w, "  Blocked on: %s\n", r.Task.BlockedReason); err != nil {
			return err
		}
	}
	if r.SessionMinted {
		if _, err := fmt.Fprintf(w,
			"\nNo --session was given, so session %s was opened for this run.\nPass --session %s to keep later commands in it.\n",
			r.Session.ID, r.Session.ID); err != nil {
			return err
		}
	}
	return nil
}

// handoverResult is what `task show` publishes.
type handoverResult struct {
	coordination.Handover
}

func (r handoverResult) RenderHuman(w io.Writer, _ bool) error {
	task := r.Task
	if _, err := fmt.Fprintf(w, "Task %s is %s.\n  %s\n", task.ID, task.State, task.Title); err != nil {
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
