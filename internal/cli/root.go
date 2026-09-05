// Package cli builds the Mindrail command tree. Commands translate flags into
// application service calls and render results; they hold no domain logic.
package cli

import (
	"context"

	"github.com/spf13/cobra"
)

const rootLong = `Mindrail is a local engineering gate for AI coding agents.

It discovers the actual code change from Git, binds durable engineering
constraints to it, requires snapshot-bound evidence and denies completion
when the proof is insufficient.`

// NewRootCommand builds the root command. Subcommands are registered by their
// own packages so that the tree stays additive as milestones land.
func NewRootCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mindrail",
		Short: "Local engineering gate for AI coding agents",
		Long:  rootLong,

		// Errors are rendered and exit-coded by main, not by cobra.
		SilenceErrors: true,
		SilenceUsage:  true,
	}

	return cmd
}

// Execute runs the command tree against ctx and returns the command error
// unchanged so that main can map it to an exit code.
func Execute(ctx context.Context) error {
	return NewRootCommand().ExecuteContext(ctx)
}
