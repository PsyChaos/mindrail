package cli

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/bootstrap"
	"github.com/PsyChaos/mindrail/internal/config"
	"github.com/PsyChaos/mindrail/internal/credential"
	"github.com/PsyChaos/mindrail/internal/status"
)

const flagPort = "port"

// DashboardStart is the read-only, already-resolved state handed to the
// network-capable dashboard runner injected by cmd/mindrail. It deliberately
// carries no secret values and no log/evidence payloads.
type DashboardStart struct {
	DB             *sql.DB
	ProjectID      string
	ProjectName    string
	WorkspaceID    string
	WorktreeRoot   string
	LinkedWorktree bool
	Profiles       map[string]config.ValidationProfile
	Continuity     config.ContinuityConfig
	Readiness      status.Report
	JEV            DashboardJEVState
	SecretNames    []string
	Port           int
	Output         io.Writer
}

// DashboardJEVState is the credential-free startup projection passed across
// the optional dashboard transport seam. No secret value or process
// environment is retained by the long-running server.
type DashboardJEVState struct {
	Configured bool
	Source     string
}

func newDashboardCommand(o Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "dashboard",
		Short: "Open the live local operations dashboard",
		Long: `Serve a read-only control tower for this repository on loopback.

The dashboard uses a random URL token, serves embedded assets, streams bounded
snapshots over SSE and never exposes evidence output, command arguments,
credentials or unbounded checkpoint text. It runs until interrupted.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return runDashboard(cmd, o) },
	}
	cmd.Flags().Int(flagPort, 0, "loopback port (0 chooses an available port)")
	return cmd
}

func runDashboard(cmd *cobra.Command, o Options) error {
	inv, err := newInvocation(cmd, "dashboard", o)
	if err != nil {
		return inv.emit(nil, nil, nil, "", err)
	}
	if inv.flags.json {
		return inv.emit(nil, nil, nil, "", app.NewError(app.CodeCommandLineInvalid, app.KindUsage,
			"dashboard is a long-running browser view and does not support --json",
			"No server was started.", "Run `mindrail dashboard` without --json."))
	}
	port, err := cmd.Flags().GetInt(flagPort)
	if err != nil {
		return err
	}
	application := bootstrap.New(bootstrap.Options{StartDir: inv.startDir, Mode: bootstrap.ModeReadOnly,
		Runner: inv.runner(), BusyTimeout: inv.opts.BusyTimeout, Logger: inv.logger, Environ: inv.environ})
	defer shutdown(cmd.Context(), application, inv.logger)
	return application.Run(cmd.Context(), func(ctx context.Context, a *bootstrap.App) error {
		if _, verdict := a.Diagnosis(ctx); verdict != nil {
			return inv.emit(nil, nil, a.Warnings(), a.Config().Config.Output.Color, verdict)
		}
		resolved, err := coordinationScope(a)
		if err != nil {
			return inv.emit(nil, nil, a.Warnings(), a.Config().Config.Output.Color, err)
		}
		readiness := status.Build(a.Subject(), 0)
		jev := resolveDashboardJEV(ctx, inv.environ, credential.NewOSStore())
		if o.RunDashboard == nil {
			return dashboardFailure(fmt.Errorf("dashboard runner is unavailable"))
		}
		return o.RunDashboard(ctx, DashboardStart{
			DB: a.DB(), ProjectID: resolved.space.ProjectID, ProjectName: a.Config().Config.Project.Name,
			WorkspaceID: resolved.space.ID, WorktreeRoot: a.Repo().WorktreeRoot,
			LinkedWorktree: resolved.space.IsLinkedWorktree, Profiles: a.Config().Config.Validation,
			Continuity: a.Config().Config.Continuity,
			Readiness:  readiness, JEV: jev, SecretNames: a.Config().Config.Secrets.Env, Port: port, Output: inv.stdout,
		})
	})
}

func resolveDashboardJEV(ctx context.Context, environ []string, store credential.Store) DashboardJEVState {
	for _, entry := range environ {
		name, value, ok := strings.Cut(entry, "=")
		if ok && name == "TYPESAFE_API_KEY" && strings.TrimSpace(value) != "" {
			return DashboardJEVState{Configured: true, Source: "environment"}
		}
	}
	_, state := readJEVKey(ctx, store)
	switch state {
	case "available":
		return DashboardJEVState{Configured: true, Source: "keyring"}
	case "unavailable":
		return DashboardJEVState{Configured: false, Source: "unavailable"}
	default:
		return DashboardJEVState{Configured: false, Source: "none"}
	}
}

func dashboardFailure(err error) error {
	return app.NewError(app.CodeRuntimeDBUnavailable, app.KindUnavailable,
		"the local dashboard could not start", "No dashboard server is running.",
		"Check that the repository is initialized and the requested loopback port is available.").WithCause(err)
}
