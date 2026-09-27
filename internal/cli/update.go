package cli

import "github.com/spf13/cobra"

const updateLong = `Refresh an existing Mindrail repository.

update applies the same embedded migrations and managed agent setup as init,
while preserving the existing configuration, knowledge, runtime identity and
foreign hooks. It only runs after a read-only check proves that this worktree
has already completed mindrail init; use mindrail init for first-time setup.`

func newUpdateCommand(o Options) *cobra.Command {
	return &cobra.Command{
		Use:   "update",
		Short: "Refresh an initialized Mindrail repository",
		Long:  updateLong,
		Args:  cobra.NoArgs,
		RunE:  func(cmd *cobra.Command, _ []string) error { return runUpdate(cmd, o) },
	}
}

func runUpdate(cmd *cobra.Command, o Options) error {
	return runRepositorySetup(cmd, o, "update", true)
}
