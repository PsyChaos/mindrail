package cli

import (
	"context"
	"log/slog"
	"time"

	"github.com/spf13/cobra"

	"github.com/PsyChaos/mindrail/internal/bootstrap"
	"github.com/PsyChaos/mindrail/internal/doctor"
	"github.com/PsyChaos/mindrail/internal/status"
)

const initLong = `Prepare this repository for Mindrail.

init is the only command that writes. It creates .mindrail/config.toml and the
knowledge directories if they are absent, creates the runtime database under the
Git common directory, applies the embedded migrations and registers this
worktree. An existing configuration is never overwritten.`

// newInitCommand builds `mindrail init`.
func newInitCommand(o Options) *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Prepare this repository for Mindrail",
		Long:  initLong,
		Args:  cobra.NoArgs,
		RunE:  func(cmd *cobra.Command, _ []string) error { return runInit(cmd, o) },
	}
}

func runInit(cmd *cobra.Command, o Options) error {
	inv, err := newInvocation(cmd, "init", o)
	if err != nil {
		return inv.emit(nil, nil, nil, "", err)
	}

	started := time.Now()
	application := bootstrap.New(bootstrap.Options{
		StartDir: inv.startDir,
		Mode:     bootstrap.ModeInit,
		Runner:   inv.runner(),
		Logger:   inv.logger,
		Environ:  inv.environ,
	})
	defer shutdown(cmd.Context(), application, inv.logger)

	return application.Run(cmd.Context(), func(ctx context.Context, a *bootstrap.App) error {
		// The verdict comes from the doctor checks rather than from the start
		// error directly, so `init`, `status` and `doctor` can never report
		// different codes for the same broken installation.
		_, verdict := a.Diagnosis(ctx)
		elapsed := time.Since(started)

		report := initReportOf(a, verdict, elapsed)
		inv.logger.Info("init finished",
			slog.String("terminal_state", string(report.TerminalState)),
			slog.Duration("duration", elapsed))

		return inv.emit(report, report.RenderHuman, a.Warnings(),
			a.Config().Config.Output.Color, verdict)
	})
}

// initReportOf assembles what init did and how the repository ended up.
//
// TerminalState is derived from both the verdict and the readiness, not from
// the verdict alone: a repository can end up unusable without any single check
// failing outright — an unreadable knowledge record blocks work while every
// component that did resolve is healthy — and spec §82 promises the terminal
// line reflects whether targeted work can start, not whether init crashed.
func initReportOf(a *bootstrap.App, verdict error, elapsed time.Duration) status.InitReport {
	result := a.InitResult()
	current := status.Build(a.Subject(), elapsed)

	report := status.InitReport{
		Command:              "init",
		TerminalState:        status.TerminalReady,
		ConfigPath:           result.ConfigPath,
		ConfigCreated:        result.ConfigCreated,
		KnowledgeDirsCreated: result.KnowledgeDirsCreated,
		MigrationsApplied:    result.MigrationsApplied,
		SchemaCurrent:        schemaIsCurrent(a.Subject()),
		Status:               current,
		DurationMS:           elapsed.Milliseconds(),
	}

	if verdict != nil || current.Readiness == status.ReadinessBlocked {
		report.TerminalState = status.TerminalBlocked
		report.Reason = blockedReason(verdict, blockingSummary(current))
	}

	return report
}

// schemaIsCurrent reports that the runtime schema was actually established:
// a database is present, the ledger was read without error, and nothing this
// binary knows is still pending.
//
// All three conditions are needed. A run that never opened a database, a run
// whose ledger could not be read and a run that left migrations pending all
// applied nothing, and none of them may claim the schema is current
// (finding F14).
func schemaIsCurrent(s doctor.Subject) bool {
	return s.DBPresent && s.MigrateErr == nil && s.PendingCount == 0
}

// blockingSummary names the component that stopped the run, for the case where
// readiness is BLOCKED but no check reported an outright error.
func blockingSummary(report status.Report) string {
	if report.BlockingComponent == "" {
		return ""
	}
	return string(report.BlockingComponent) + ": " + report.Components[report.BlockingComponent].Summary
}
