package cli

import (
	"context"
	"io"

	"github.com/spf13/cobra"

	agentrouter "github.com/PsyChaos/mindrail/internal/agent"
	"github.com/PsyChaos/mindrail/internal/credential"
)

const (
	maxAgentRouteInput  = agentrouter.MaxRouteInputBytes
	maxAgentRouteOutput = agentrouter.MaxRouteOutputBytes
)

type agentCommandDeps struct {
	findPython func() (string, error)
	runPython  func(context.Context, string, []byte, string) ([]byte, error)
	store      credential.Store
}

func newAgentCommand() *cobra.Command {
	return newAgentCommandWith(agentCommandDeps{store: credential.NewOSStore()})
}

func newAgentCommandWith(deps agentCommandDeps) *cobra.Command {
	agent := &cobra.Command{
		Use:    "agent",
		Short:  "Internal coding-agent integrations",
		Args:   cobra.NoArgs,
		Hidden: true,
	}
	route := &cobra.Command{
		Use:    "route",
		Short:  "Return optional JEV routing advice",
		Args:   cobra.NoArgs,
		Hidden: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runAgentRoute(cmd, deps)
		},
	}
	agent.AddCommand(route)
	return agent
}

func runAgentRoute(cmd *cobra.Command, deps agentCommandDeps) error {
	router := agentrouter.NewRouterWithDependencies(agentrouter.RouterDependencies{
		FindPython: deps.findPython,
		RunPython:  deps.runPython,
		Store:      deps.store,
	})
	result := router.Route(cmd.Context(), cmd.InOrStdin())
	return writeRouteJSON(cmd.OutOrStdout(), result.JSON)
}

func writeRouteJSON(writer io.Writer, raw []byte) error {
	if len(raw) == 0 || raw[len(raw)-1] == '\n' {
		_, err := writer.Write(raw)
		return err
	}
	_, err := writer.Write(append(append([]byte(nil), raw...), '\n'))
	return err
}

func findTrustedPython() (string, error) { return agentrouter.FindTrustedPython() }

func readJEVKey(ctx context.Context, store credential.Store) (string, string) {
	return agentrouter.ReadCredential(ctx, store)
}
