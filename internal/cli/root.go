// Package cli builds the Mindrail command tree. Commands translate flags into
// application service calls and render results; they hold no domain logic.
package cli

import (
	"context"
	"fmt"
	"os"
	"slices"

	"github.com/spf13/cobra"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/git"
)

// Options are the process seams the command tree cannot discover for itself.
//
// There is one, and it exists because the git-failure branch was otherwise
// unreachable from a test: every command wired git.NewExecRunner() directly, so
// "git is missing" and "git hangs" could only be exercised by mutating PATH for
// the whole process, which a parallel test suite cannot do. That is why the
// contradiction between the envelope and the exit code survived a green suite
// (findings F11, F19). internal/git ships FakeRunner as a non-test file for
// exactly this.
//
// The zero value is production: the real git binary, found on PATH.
type Options struct {
	// Runner executes git subprocesses. nil means git.NewExecRunner().
	Runner git.CommandRunner
}

const rootLong = `Mindrail is a local engineering gate for AI coding agents.

It discovers the actual code change from Git, binds durable engineering
constraints to it, requires snapshot-bound evidence and denies completion
when the proof is insufficient.`

// Root is the command tree together with the command line it was handed.
//
// The extra field exists for one reason. When cobra rejects a command line — an
// unknown subcommand, an unknown flag, a surplus argument — no command body ever
// runs, so nothing emits the single JSON object decision D-15 promises on
// stdout. Answering "did the caller ask for --json?" needs the raw line, and
// neither cobra nor pflag will give it back: cobra keeps its argument slice
// private, and pflag discards the input at the first flag it does not
// recognise. Reading the flag set instead would make the answer depend on the
// order the user happened to type, which is a worse contract than none
// (findings F04, F08).
type Root struct {
	*cobra.Command
	commandLine []string
}

// NewRoot builds the production command tree.
func NewRoot() *Root {
	return NewRootWith(Options{})
}

// NewRootWith builds the root command against explicit seams. Subcommands are
// registered here so that the tree stays additive as milestones land.
func NewRootWith(o Options) *Root {
	root := &Root{}

	cmd := &cobra.Command{
		Use:   "mindrail",
		Short: "Local engineering gate for AI coding agents",
		Long:  rootLong,

		// Errors are rendered and exit-coded by main, not by cobra.
		SilenceErrors: true,
		SilenceUsage:  true,

		// The root has to be runnable for cobra to reach ValidateArgs at all —
		// a non-runnable command returns ErrHelp first — and running it with no
		// arguments has always printed help, so that is what it does.
		RunE: func(c *cobra.Command, _ []string) error { return c.Help() },

		// Replaces cobra's legacyArgs, which produced the same sentence but as a
		// bare error that app.ExitCode could only read as "operation failed".
		Args: func(c *cobra.Command, args []string) error {
			if len(args) == 0 {
				return nil
			}
			return root.usageError(c,
				fmt.Errorf("unknown command %q for %q", args[0], c.CommandPath()),
				suggestionRemedies(c, args[0])...)
		},
	}

	cmd.SetFlagErrorFunc(func(c *cobra.Command, err error) error {
		return root.usageError(c, err)
	})

	// Persistent, because every command answers the same four questions: which
	// shape the answer takes, how much of the startup sequence is worth
	// logging, whether colour is welcome, and which directory the question is
	// about.
	flags := cmd.PersistentFlags()
	flags.Bool(flagJSON, false, "emit the result as a single JSON object on stdout")
	flags.Bool(flagVerbose, false, "log the startup sequence to stderr")
	flags.Bool(flagNoColor, false, "never colour human output")
	flags.StringP(flagChdir, "C", "", "run as if mindrail was started in this directory")

	cmd.AddCommand(
		newInitCommand(o),
		newStatusCommand(o),
		newDoctorCommand(o),
		newVersionCommand(),
	)

	// Each subcommand's own argument check is classified the same way, so that a
	// surplus argument to `init` and a typo'd subcommand at the root produce one
	// verdict rather than two. Doing it here rather than in the four
	// constructors keeps the classification in the place that owns the exit-code
	// contract; a command added by a later milestone is covered by construction.
	for _, sub := range cmd.Commands() {
		declared := sub.Args
		sub.Args = func(c *cobra.Command, args []string) error {
			if declared == nil {
				return nil
			}
			if err := declared(c, args); err != nil {
				return root.usageError(c, err)
			}
			return nil
		}
	}

	root.Command = cmd
	return root
}

// SetArgs records the command line before handing it to cobra, so that a
// failure during parsing can still tell whether --json was asked for.
func (r *Root) SetArgs(args []string) {
	r.commandLine = slices.Clone(args)
	r.Command.SetArgs(args)
}

// usageError classifies a command line this binary could not understand.
//
// Decision D-03 and tech-stack §13 reserve exit 2 for invalid usage, and every
// other usage mistake this binary knows — a directory that is not a repository,
// a bare repository, a malformed config.toml — already returns it. A typo'd
// subcommand is the same class of mistake; returning cobra's error unwrapped
// left it exiting 1, the code reserved for a gate that ran and failed, so a
// wrapper branching on "was my invocation wrong?" got the wrong answer
// (findings F04, F08).
func (r *Root) usageError(cmd *cobra.Command, err error, extra ...string) error {
	next := append([]string{
		"Run `" + cmd.CommandPath() + " --help` to see the accepted commands and flags.",
	}, extra...)

	verdict := app.NewError(
		app.CodeCommandLineInvalid,
		app.KindUsage,
		err.Error(),
		"Mindrail did not run: no repository was inspected and nothing was written.",
		next...,
	)

	r.emitUsageEnvelope(cmd, verdict)
	return verdict
}

// suggestionRemedies turns cobra's near-miss suggestions into remedies. They
// belong in next_action rather than glued onto the message, because that is the
// field spec §84 reserves for "what to do next" and the only one a machine
// reads for it.
func suggestionRemedies(cmd *cobra.Command, typed string) []string {
	suggestions := cmd.SuggestionsFor(typed)
	remedies := make([]string, 0, len(suggestions))
	for _, suggestion := range suggestions {
		remedies = append(remedies, "Did you mean `"+cmd.CommandPath()+" "+suggestion+"`?")
	}
	return remedies
}

// emitUsageEnvelope gives a rejected command line the same single JSON object
// every other failure puts on stdout.
//
// It is best-effort: a write failure here has nowhere left to be reported, and
// the exit code already carries the verdict. Nothing else has written to stdout
// at this point — the command body never ran — so the envelope is still the only
// value on it.
func (r *Root) emitUsageEnvelope(cmd *cobra.Command, verdict error) {
	if !r.jsonRequested() {
		return
	}
	_ = app.WriteJSON(cmd.OutOrStdout(), cmd.Name(), nil, nil, verdict)
}

// jsonRequested reads --json off the raw command line. Everything after a bare
// `--` is an argument rather than a flag, so the scan stops there.
func (r *Root) jsonRequested() bool {
	for _, arg := range r.commandLine {
		switch arg {
		case "--":
			return false
		case "--" + flagJSON, "--" + flagJSON + "=true":
			return true
		}
	}
	return false
}

// Execute runs the command tree against ctx and returns the command error
// unchanged so that main can map it to an exit code.
func Execute(ctx context.Context) error {
	root := NewRoot()
	root.SetArgs(os.Args[1:])
	return root.ExecuteContext(ctx)
}
