package cli

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
	"unicode/utf8"

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

	if refusal := inv.refuseUnrepresentableFlags(cmd); refusal != nil {
		return inv.emit(nil, nil, nil, "", refusal)
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
		// The startup verdict comes first, and it comes from the doctor checks
		// for the same reason `init` takes it that way: `init`, `status`,
		// `doctor` and the six coordination commands must never report
		// different codes for one broken installation.
		//
		// Run calls this body even when Start failed, so without this line the
		// only two readings below — is the store nil, is the workspace row
		// empty — answered for every cause there is. A repository whose
		// config.toml cannot be parsed, whose database is corrupt or whose
		// schema is newer than this binary was told, on stdout, that it "has no
		// runtime database, so it holds no sessions, tasks or checkpoints",
		// with `mindrail init` as the remedy that clears none of them, while
		// the true code went to stderr and was dropped (finding F09). An agent
		// reading that has been told its handover state does not exist.
		//
		// COORDINATION_UNAVAILABLE is reachable only when the startup sequence
		// completed, which is the one condition it describes truthfully.
		if _, verdict := a.Diagnosis(ctx); verdict != nil {
			return inv.emit(nil, nil, a.Warnings(), a.Config().Config.Output.Color, verdict)
		}

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

// coordinationTextFlags are the flags whose value is free text the caller wrote,
// as opposed to an identifier this binary minted or a word from a fixed
// vocabulary. They are the values that reach a stored column unchanged.
var coordinationTextFlags = []string{flagTitle, flagNote, flagReason, flagLabel}

// refuseUnrepresentableFlags stops a `--json` write whose own input JSON cannot
// carry, before the application is started and therefore before anything is
// written.
//
// The refusal is not new; its position is. It used to live in emit, which runs
// after the body has committed, so `mindrail --json task open --title "fix
// parser \xff bug"` inserted the task, then reported PATH_NOT_REPRESENTABLE at
// exit 2 — and a caller that retries on failure opened a second task, and a
// third (finding F28). `init` learned the same lesson at finding W9 and answers
// it the same way: refuse before acting, because the alternative is doing the
// whole job and calling it a failure.
//
// A flag value can be refused where a repository path cannot. The path is where
// the user already is and the command has no other way to describe it; a flag
// is input at a system boundary, and the caller can pass different bytes.
//
// Human mode is untouched. It writes the bytes through unchanged, which is why
// the JSON refusal can send the reader there, and refusing a title in human mode
// would take away the one view that can still show it.
func (inv invocation) refuseUnrepresentableFlags(cmd *cobra.Command) error {
	if !inv.flags.json {
		return nil
	}

	for _, name := range coordinationTextFlags {
		flag := cmd.Flags().Lookup(name)
		if flag == nil || !flag.Changed {
			continue
		}

		value := flag.Value.String()
		if utf8.ValidString(value) {
			continue
		}

		// strconv.Quote escapes every invalid byte as \xNN, so the refusal
		// naming the value is itself valid UTF-8 and the envelope reporting the
		// problem does not reproduce it.
		return app.NewError(
			app.CodeCommandLineInvalid,
			app.KindUsage,
			"the value given for --"+name+" is not valid UTF-8: "+strconv.Quote(value),
			"Nothing was read and nothing was written: a value JSON cannot carry unchanged would be "+
				"stored as it stands and published as something else.",
			"Pass a --"+name+" whose every byte is valid UTF-8.",
			"Or run the command without --json, where the bytes are carried through unchanged.",
		).WithMetadata("flag", name).WithMetadata("unrepresentable_value", strconv.Quote(value))
	}

	return nil
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

// attribution turns the --session flag into the store's Attribution: the
// session the caller named, or an instruction to mint one for this worktree
// (decision D-61).
//
// It resolves nothing itself. The session used to be minted here, before the
// operation was judged, so a refused write left a row for an agent that never
// did anything while telling the caller nothing had been written (finding F02).
// The store mints inside the transaction that carries the write, and a refusal
// takes the mint back with it.
func (s scope) attribution(handle string) coordination.Attribution {
	if handle == "" {
		return coordination.MintFor(s.space.ID)
	}
	return coordination.NamedSession(handle)
}

// refuseBeforeStarting reports a mistake in the command line without starting
// the application, so that the answer can say so.
//
// It is the shape the `--to` parse already had. A judgment made after the
// database is open is reported as something that happened during the
// operation — "Nothing was written" — where a judgment made here can say
// Mindrail never ran, which is both truthful and a different instruction to a
// caller deciding whether to retry.
func refuseBeforeStarting(cmd *cobra.Command, name string, o Options, refusal error) error {
	inv, err := newInvocation(cmd, name, o)
	if err != nil {
		return inv.emit(nil, nil, nil, "", err)
	}
	return inv.emit(nil, nil, nil, "", refusal)
}

// requireFlagText refuses a command whose one mandatory free-text flag is empty
// or blank, at the boundary rather than inside the store.
func requireFlagText(cmd *cobra.Command, flag, value, why, remedy string) error {
	if strings.TrimSpace(value) != "" {
		return nil
	}

	return app.NewError(
		app.CodeCommandLineInvalid,
		app.KindUsage,
		why,
		"Mindrail did not run: nothing was read and nothing was written.",
		remedy,
		"Run `"+cmd.CommandPath()+" --help` to see the accepted flags.",
	).WithMetadata("flag", flag)
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
