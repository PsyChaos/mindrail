package cli

import (
	"context"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/bootstrap"
	"github.com/PsyChaos/mindrail/internal/coordination"
	"github.com/PsyChaos/mindrail/internal/workspace"
)

// flagSession is the agent session handle spec §98 makes an explicit
// application handle rather than a property of the connection. It is what lets
// an agent change tools mid-task and still be recognised.
const (
	flagSession = "session"
	flagTitle   = "title"
	flagState   = "state"
	flagTo      = "to"
	flagReason  = "reason"
	flagNote    = "note"
	flagHandoff = "handoff"
	flagLabel   = "label"
)

// scope is a started application with a coordination store and the workspace it
// belongs to. Every coordination command needs exactly these three things, and
// none of them is worth resolving twice.
type scope struct {
	app   *bootstrap.App
	store *coordination.Store
	space workspace.Workspace
}

// projectID is the owner of every task this repository has (decision D-60).
func (s scope) projectID() string { return s.space.ProjectID }

// runCoordination is the body every coordination command shares: start the
// application, resolve the store, and hand both to the command.
//
// ModeWrite, not ModeReadOnly, and the difference was a defect before it was a
// decision: the first draft of these commands ran read-only, on the argument
// that decision D-01's authority is about *creating* the database. It is — but
// ModeReadOnly also opens the SQLite handle read-only, so every one of these
// commands failed with RUNTIME_PATH_UNWRITABLE against a database whose
// permissions were perfectly fine. ModeWrite is the mode that separates the two:
// it opens for writing and creates, migrates and registers nothing.
//
// The read commands use it too. `task show` could run read-only, but the mode is
// also what decides whether a missing database is reported or created, and
// giving the six commands two different answers to that would make the group's
// behaviour depend on which one you happened to run.
func runCoordination(cmd *cobra.Command, name string, o Options,
	body func(context.Context, scope) (any, humanRenderer, error)) error {

	inv, err := newInvocation(cmd, name, o)
	if err != nil {
		return inv.emit(nil, nil, nil, "", err)
	}

	application := bootstrap.New(bootstrap.Options{
		StartDir: inv.startDir,
		Mode:     bootstrap.ModeWrite,
		Runner:   inv.runner(),
		Logger:   inv.logger,
		Environ:  inv.environ,
	})
	defer shutdown(cmd.Context(), application, inv.logger)

	return application.Run(cmd.Context(), func(ctx context.Context, a *bootstrap.App) error {
		resolved, err := coordinationScope(a)
		if err != nil {
			return inv.emit(nil, nil, a.Warnings(), a.Config().Config.Output.Color, err)
		}

		data, human, verdict := body(ctx, resolved)
		return inv.emit(data, human, a.Warnings(), a.Config().Config.Output.Color, verdict)
	})
}

// coordinationScope refuses a repository that has no coordination state.
//
// The two conditions are separate readings and both mean "run init", but they
// are not the same fact: one is "there is no runtime database", the other is
// "there is one and this worktree is not in it". Naming them separately in the
// diagnostic is what tells a reader whether their `init` failed or was never
// run here.
func coordinationScope(a *bootstrap.App) (scope, error) {
	store := a.Coordination()
	if store == nil {
		return scope{}, coordinationUnavailable(
			"this repository has no runtime database, so it holds no sessions, tasks or checkpoints")
	}

	space := a.Subject().Workspace
	if space.ID == "" || space.ProjectID == "" {
		return scope{}, coordinationUnavailable(
			"this worktree is not registered in the runtime database, so it has no project to hold tasks")
	}

	return scope{app: a, store: store, space: space}, nil
}

func coordinationUnavailable(why string) error {
	return app.NewError(
		app.CodeCoordinationUnavailable,
		app.KindFailed,
		why,
		"Nothing was read and nothing was written.",
		"Run `mindrail init` in this worktree, then re-run the command.",
	)
}

// resolveSession turns the --session flag into a session, minting one when the
// caller supplied none (decision D-61).
//
// The minted flag is returned rather than inferred by the caller, because "you
// are working under a session you did not name" is something the result has to
// say out loud: an agent that wanted continuity and forgot the flag would
// otherwise carry on under a fresh identity without noticing.
func (s scope) resolveSession(ctx context.Context, handle string) (coordination.Session, bool, error) {
	if handle == "" {
		session, err := s.store.OpenSession(ctx, s.space.ID, "")
		return session, true, err
	}

	session, err := s.store.FindSession(ctx, handle)
	return session, false, err
}

// newSessionCommand builds `mindrail session`.
func newSessionCommand(o Options) *cobra.Command {
	group := &cobra.Command{
		Use:   "session",
		Short: "Mint the agent session handle coordination commands are attributed to",
		Long: `Manage the agent session handle.

A session is one agent's run against this worktree. It is minted and never
resumed: an agent taking over from another gets a session of its own and finds
the work through the task and its checkpoints, which is what makes a handover
survive a process exit.`,
	}
	group.AddCommand(newSessionOpenCommand(o))
	return group
}

func newSessionOpenCommand(o Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "open",
		Short: "Mint a session and print its id",
		Long: `Mint a session and print its id.

Pass the id back as --session on later commands so that the tasks you open and
the checkpoints you write are attributed to one run.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			label, _ := cmd.Flags().GetString(flagLabel)

			return runCoordination(cmd, "session open", o,
				func(ctx context.Context, s scope) (any, humanRenderer, error) {
					session, err := s.store.OpenSession(ctx, s.space.ID, label)
					if err != nil {
						return nil, nil, err
					}
					result := sessionResult{Session: session}
					return result, result.RenderHuman, nil
				})
		},
	}
	cmd.Flags().String(flagLabel, "", "a name you will recognise this run by")
	return cmd
}

// sessionResult is what `session open` publishes.
type sessionResult struct {
	Session coordination.Session `json:"session"`
}

func (r sessionResult) RenderHuman(w io.Writer, _ bool) error {
	_, err := fmt.Fprintf(w, "Session %s opened.\nPass --session %s to the commands that belong to this run.\n",
		r.Session.ID, r.Session.ID)
	return err
}
