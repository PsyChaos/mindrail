package cli

import (
	"context"
	"log/slog"
	"time"

	"github.com/spf13/cobra"

	"github.com/PsyChaos/mindrail/internal/bootstrap"
	"github.com/PsyChaos/mindrail/internal/config"
	"github.com/PsyChaos/mindrail/internal/doctor"
	"github.com/PsyChaos/mindrail/internal/filesystem"
	"github.com/PsyChaos/mindrail/internal/git"
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

	// Asked before anything is written; see the function for why the alternative
	// is not an alternative.
	if refusal := inv.refuseUnrepresentableRepository(cmd.Context()); refusal != nil {
		return inv.emit(nil, nil, nil, "", refusal)
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
		// init's own last write, made before the reading rather than after it.
		//
		// Everything below reads the disk to decide what `init` achieved, and
		// until this call the answer was taken while a write-ahead log this
		// command had produced was still waiting to be folded back in by the
		// close. The reading described a filesystem the process was about to
		// change, and on a nearly-full one it described it wrongly: READY at
		// exit 0, and `status` on the identical bytes at exit 4 (finding F01).
		flushErr := a.Flush(ctx)

		// The verdict comes from the doctor checks rather than from the start
		// error directly, so `init`, `status` and `doctor` can never report
		// different codes for the same broken installation.
		_, verdict := a.Diagnosis(ctx)

		// The checks run after the flush, so a failure the flush caused is
		// normally already in the verdict, said in the vocabulary status and
		// doctor use. This is the case they cannot see: a write that failed for
		// a reason no probe of the finished disk reveals.
		if verdict == nil {
			verdict = flushErr
		}
		elapsed := time.Since(started)

		report := initReportOf(a, verdict, elapsed)
		inv.logger.Info("init finished",
			slog.String("terminal_state", string(report.TerminalState)),
			slog.Duration("duration", elapsed))

		return inv.emit(report, report.RenderHuman, a.Warnings(),
			a.Config().Config.Output.Color, verdict)
	})
}

// refuseUnrepresentableRepository stops `mindrail init --json` before it writes
// anything, in a repository whose own paths JSON cannot carry unchanged.
//
// The refusal itself is not new: a repository path holding bytes that are not
// valid UTF-8 has been refused since finding W9, because JSON encoding replaces
// them with U+FFFD and would publish a location that does not exist on disk.
// What was new was where it happened. `init` ran the whole §87 sequence first —
// creating .mindrail/config.toml, both knowledge directories, the runtime
// database, the schema and the workspace row — and only then, at the moment the
// envelope was serialised, refused and exited 2. The command did every part of
// its job and reported that it had failed, which is the worst of the two
// answers available: a caller retrying on failure re-runs a completed
// initialisation, and a caller trusting the exit code believes a repository is
// uninitialised when it is not.
//
// Of the two honest fixes — refuse before acting, or act and report success
// honestly — only the first is available. Reporting success would mean emitting
// the paths, and emitting the paths is exactly what cannot be done; a report
// that omitted them would answer "where does this repository's state live?" with
// silence, in the one command whose whole output is that answer.
//
// The check costs a second read-only startup, and only on `init --json`. It is
// the real startup rather than a hand-rolled prefix of it because the strings at
// risk come from three different layers — git's answers, the configuration's
// runtime overrides and the paths derived from both — and a second
// implementation that resolved two of the three would refuse exactly the cases
// it happened to know about. Read-only mode creates nothing (decision D-01), so
// the probe leaves the disk as it found it.
//
// A start that failed produces no refusal. Whatever went wrong is the real run's
// to report, and a repository nobody could resolve has no paths to be
// unrepresentable.
func (inv invocation) refuseUnrepresentableRepository(ctx context.Context) error {
	if !inv.flags.json {
		return nil
	}

	probe := bootstrap.New(bootstrap.Options{
		StartDir: inv.startDir,
		Mode:     bootstrap.ModeReadOnly,
		Runner:   inv.runner(),
		Logger:   inv.logger,
		Environ:  inv.environ,
	})
	defer shutdown(ctx, probe, inv.logger)

	if err := probe.Start(ctx); err != nil {
		return nil
	}

	subject := probe.Subject()
	return refuseUnrepresentableJSON(struct {
		Repo   git.Repository
		Paths  filesystem.RuntimePaths
		Config config.Config
	}{
		Repo:   subject.Repo,
		Paths:  subject.Paths,
		Config: subject.Config.Config,
	}, nil)
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
		ConfigPresent:        result.ConfigPresent,
		KnowledgeDirsCreated: result.KnowledgeDirsCreated,
		KnowledgeDirsPresent: result.KnowledgeDirsPresent,
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
