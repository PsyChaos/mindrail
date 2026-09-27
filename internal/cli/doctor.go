package cli

import (
	"context"
	"log/slog"
	"time"

	"github.com/spf13/cobra"

	"github.com/PsyChaos/mindrail/internal/bootstrap"
)

const doctorLong = `Report the health of every Mindrail subsystem in this repository.

doctor changes nothing. It reports what the startup sequence found, including
what it failed to find, so a broken installation is described rather than merely
refused. Absent optional capabilities are not errors, so doctor exits zero
unless a check reports an outright failure.`

// newDoctorCommand builds `mindrail doctor`.
func newDoctorCommand(o Options) *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Diagnose this repository's Mindrail installation",
		Long:  doctorLong,
		Args:  cobra.NoArgs,
		RunE:  func(cmd *cobra.Command, _ []string) error { return runDoctor(cmd, o) },
	}
}

func runDoctor(cmd *cobra.Command, o Options) error {
	inv, err := newInvocation(cmd, "doctor", o)
	if err != nil {
		return inv.emit(nil, nil, nil, "", err)
	}

	started := time.Now()
	application := bootstrap.New(bootstrap.Options{
		StartDir:    inv.startDir,
		Mode:        bootstrap.ModeReadOnly,
		Runner:      inv.runner(),
		BusyTimeout: inv.opts.BusyTimeout,
		Logger:      inv.logger,
		Environ:     inv.environ,
	})
	defer shutdown(cmd.Context(), application, inv.logger)

	return application.Run(cmd.Context(), func(ctx context.Context, a *bootstrap.App) error {
		// Unlike status, doctor always has something to say: a report of seven
		// failed checks is exactly the output a user in a broken repository
		// needs, so the report is rendered whatever the startup sequence did.
		report, verdict := a.Diagnosis(ctx)

		inv.logger.Info("doctor finished",
			slog.String("worst_state", string(report.WorstState)),
			slog.Duration("duration", time.Since(started)))

		return inv.emit(report, report.RenderHuman, a.Warnings(),
			a.Config().Config.Output.Color, verdict)
	})
}
