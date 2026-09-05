package cli

import (
	"context"
	"log/slog"
	"time"

	"github.com/spf13/cobra"

	"github.com/PsyChaos/mindrail/internal/bootstrap"
	"github.com/PsyChaos/mindrail/internal/status"
)

const statusLong = `Report whether this repository is ready for targeted work.

status never creates and never migrates anything. A repository that has not been
initialised is reported as BLOCKED with mindrail init as the next action, which
is a state rather than a failure, so the command still exits zero.`

// newStatusCommand builds `mindrail status`.
func newStatusCommand(o Options) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Report repository readiness",
		Long:  statusLong,
		Args:  cobra.NoArgs,
		RunE:  func(cmd *cobra.Command, _ []string) error { return runStatus(cmd, o) },
	}
}

func runStatus(cmd *cobra.Command, o Options) error {
	inv, err := newInvocation(cmd, "status", o)
	if err != nil {
		return inv.emit(nil, nil, nil, "", err)
	}

	started := time.Now()
	application := bootstrap.New(bootstrap.Options{
		StartDir: inv.startDir,
		Mode:     bootstrap.ModeReadOnly,
		Runner:   inv.runner(),
		Logger:   inv.logger,
		Environ:  inv.environ,
	})
	defer shutdown(cmd.Context(), application, inv.logger)

	return application.Run(cmd.Context(), func(ctx context.Context, a *bootstrap.App) error {
		_, verdict := a.Diagnosis(ctx)
		elapsed := time.Since(started)

		// A readiness report describes a repository. When startup never got as
		// far as deriving the repository's runtime locations there is no
		// repository to describe, and printing a report built from zero values
		// would name runtime_db as the blocker for what is really a missing
		// worktree or an unreadable config. The structured error says exactly
		// what went wrong; a fabricated report would not.
		//
		// That the error is *said* is the other half. status used to reach here
		// with a nil verdict whenever the startup failure was UNAVAILABLE-class
		// — a missing or hanging git — and then print nothing at all: no report,
		// no error block, no next action, one bare line on stderr and exit 4
		// (finding F12). Verdict is non-nil for a halted startup now, so emit
		// renders the §84 block whichever shape the result takes.
		var report *status.Report
		if a.Paths().RuntimeRoot != "" {
			built := status.Build(a.Subject(), elapsed)
			report = &built
		}

		inv.logger.Info("status finished",
			slog.String("readiness", readinessOf(report)),
			slog.Duration("duration", elapsed))

		if report == nil {
			return inv.emit(nil, nil, a.Warnings(), a.Config().Config.Output.Color, verdict)
		}
		return inv.emit(report, report.RenderHuman, a.Warnings(),
			a.Config().Config.Output.Color, verdict)
	})
}

func readinessOf(report *status.Report) string {
	if report == nil {
		return "unresolved"
	}
	return string(report.Readiness)
}
