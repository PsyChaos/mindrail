package mcp

import (
	"context"

	"github.com/PsyChaos/mindrail/internal/cli"
	"github.com/PsyChaos/mindrail/internal/dashboard"
)

// RunDashboard adapts the CLI's network-free launch contract to the embedded
// loopback server. Package main already admits this package as its optional
// transport launcher, keeping HTTP dependencies out of internal/cli.
func RunDashboard(ctx context.Context, start cli.DashboardStart) error {
	return dashboard.Run(ctx, dashboard.RunOptions{
		DB: start.DB, ProjectID: start.ProjectID, ProjectName: start.ProjectName,
		WorkspaceID: start.WorkspaceID, WorktreeRoot: start.WorktreeRoot,
		LinkedWorktree: start.LinkedWorktree, Profiles: start.Profiles,
		Readiness: start.Readiness, Environ: start.Environ, SecretNames: start.SecretNames, Port: start.Port, Output: start.Output,
	})
}
