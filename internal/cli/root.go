// Package cli builds the Mindrail command tree. Commands translate flags into
// application service calls and render results; they hold no domain logic.
package cli

import (
	"context"

	"github.com/spf13/cobra"

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

// NewRootCommand builds the production command tree.
func NewRootCommand() *cobra.Command {
	return NewRootCommandWith(Options{})
}

// NewRootCommandWith builds the root command against explicit seams.
// Subcommands are registered here so that the tree stays additive as milestones
// land.
func NewRootCommandWith(o Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mindrail",
		Short: "Local engineering gate for AI coding agents",
		Long:  rootLong,

		// Errors are rendered and exit-coded by main, not by cobra.
		SilenceErrors: true,
		SilenceUsage:  true,
	}

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

	return cmd
}

// Execute runs the command tree against ctx and returns the command error
// unchanged so that main can map it to an exit code.
func Execute(ctx context.Context) error {
	return NewRootCommand().ExecuteContext(ctx)
}
