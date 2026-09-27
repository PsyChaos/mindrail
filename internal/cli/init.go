package cli

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/bootstrap"
	"github.com/PsyChaos/mindrail/internal/config"
	"github.com/PsyChaos/mindrail/internal/doctor"
	"github.com/PsyChaos/mindrail/internal/filesystem"
	"github.com/PsyChaos/mindrail/internal/git"
	"github.com/PsyChaos/mindrail/internal/setup"
	"github.com/PsyChaos/mindrail/internal/status"
)

const initLong = `Prepare this repository for Mindrail.

init creates .mindrail/config.toml and the
knowledge directories if they are absent, creates the runtime database under the
Git common directory, applies the embedded migrations and registers this
worktree. It updates Mindrail's managed AGENTS.md instructions and installs a
pre-commit guard that chains existing hooks. User content and configuration are
preserved; repeated setup is safe.`

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
	return runRepositorySetup(cmd, o, "init", false)
}

// runRepositorySetup is the one repository migration/setup pipeline shared by
// init and update. update differs only in its read-only eligibility preflight
// and in the command identity carried by its output.
func runRepositorySetup(cmd *cobra.Command, o Options, command string, requireInitialized bool) error {
	inv, err := newInvocation(cmd, command, o)
	if err != nil {
		return inv.emit(nil, nil, nil, "", err)
	}
	if requireInitialized {
		if err := requireInitializedRepository(cmd.Context(), inv); err != nil {
			return inv.emit(nil, nil, nil, "", err)
		}
	}

	// Asked before anything is written; see the function for why the alternative
	// is not an alternative.
	if refusal := inv.refuseUnrepresentableRepository(cmd.Context()); refusal != nil {
		return inv.emit(nil, nil, nil, "", refusal)
	}

	started := time.Now()
	application := bootstrap.New(bootstrap.Options{
		StartDir:    inv.startDir,
		Mode:        bootstrap.ModeInit,
		Runner:      inv.runner(),
		BusyTimeout: inv.opts.BusyTimeout,
		Logger:      inv.logger,
		Environ:     inv.environ,
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
		_, checked := a.Diagnosis(ctx)

		verdict := initVerdict(checked, flushErr)
		var installed setup.Result
		if verdict == nil {
			installed, verdict = installAgentSetup(ctx, a, inv)
		}
		elapsed := time.Since(started)

		report := setupReportOf(command, a, verdict, elapsed)
		inv.logger.Info(command+" finished",
			slog.String("terminal_state", string(report.TerminalState)),
			slog.Duration("duration", elapsed))

		render := func(w io.Writer, color bool) error {
			if verdict == nil {
				if err := installed.RenderHuman(w); err != nil {
					return err
				}
			}
			return report.RenderHuman(w, color)
		}
		return inv.emit(report, render, a.Warnings(),
			a.Config().Config.Output.Color, verdict)
	})
}

// requireInitializedRepository proves update's prerequisite without creating
// a config, runtime directory, database, workspace row, or managed file. A
// partially completed first init is deliberately sent back through init: only
// the runtime database plus this worktree's registered identity establish that
// the repository has completed the original setup pipeline.
func requireInitializedRepository(ctx context.Context, inv invocation) error {
	probe := bootstrap.New(bootstrap.Options{
		StartDir:    inv.startDir,
		Mode:        bootstrap.ModeReadOnly,
		Runner:      inv.runner(),
		BusyTimeout: inv.opts.BusyTimeout,
		Logger:      inv.logger,
		Environ:     inv.environ,
	})
	startErr := probe.Start(ctx)
	initialized := probe.DB() != nil && probe.Subject().Workspace.ID != ""
	shutdown(ctx, probe, inv.logger)

	if startErr != nil {
		return startErr
	}
	if initialized {
		return nil
	}
	return app.NewError(
		app.CodeCoordinationUnavailable,
		app.KindFailed,
		"this worktree has not completed Mindrail initialization",
		"The repository was inspected, but update made no changes.",
		"Run `mindrail init` in this worktree, then run `mindrail update`.",
	)
}

func installAgentSetup(ctx context.Context, a *bootstrap.App, inv invocation) (setup.Result, error) {
	runner := inv.runner()
	if runner == nil {
		runner = git.NewExecRunner()
	}
	root := a.Paths().WorktreeRoot
	output, _, err := runner.Run(ctx, root, "rev-parse", "--git-path", "hooks/pre-commit")
	if err != nil {
		return setup.Result{}, err
	}
	hookPath := strings.TrimSuffix(string(output), "\n")
	if hookPath == "" || strings.ContainsAny(hookPath, "\x00\r\n") {
		return setup.Result{}, fmt.Errorf("Git returned an invalid pre-commit hook path")
	}
	if !filepath.IsAbs(hookPath) {
		hookPath = filepath.Join(root, hookPath)
	}
	return setup.Install(root, a.Subject().Repo.CommonDir, filepath.Clean(hookPath))
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
		StartDir:    inv.startDir,
		Mode:        bootstrap.ModeReadOnly,
		Runner:      inv.runner(),
		BusyTimeout: inv.opts.BusyTimeout,
		Logger:      inv.logger,
		Environ:     inv.environ,
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

// initVerdict folds the flush's outcome into the checks'.
//
// The checks run after the flush, so a failure the flush caused is normally
// already in their verdict, said in the vocabulary `status` and `doctor` use —
// and that one wins, because a reader who runs all three has to hear one
// sentence. What is left is the case the checks cannot see: a write that failed
// for a reason no probe of the finished disk reveals, which would otherwise be
// discarded and leave `init` reporting a repository it could not finish writing
// as ready.
//
// It is a fail-safe rather than a live branch, and saying so is the honest
// record: no repository state MR-001 can reach produces a flush failure the
// checks cannot also see, so passing `nil` for the second argument at the call
// site changes no observable behaviour. That is a fact about this milestone's
// small set of write failures, not a property to rely on — a later milestone
// that writes more will reach it — and the cost of keeping the fold is one unit
// test rather than a repository reporting as ready something it never finished
// writing.
//
// It is a function of two errors rather than three lines inside the callback
// because that is what makes both cases reachable from a value at all: no
// command produced the second, so dropping the fold left the suite green.
func initVerdict(checked, flush error) error {
	if checked != nil {
		return checked
	}
	return flush
}

// terminalStateOf decides spec §82's terminal line from the two answers that
// can each mean "targeted work cannot start here".
//
// The two coincide today. Every condition MR-001 can reach that produces a
// verdict also leaves a component blocking, and the reverse: readiness is
// BLOCKED for a component that is ERROR or UNAVAILABLE, and both of those reach
// doctor.Verdict as well. An audit hunted for a divergence and found none, and
// the justification the comment here used to give for the disjunction — "an
// unreadable knowledge record blocks work while every component that did resolve
// is healthy" — is simply not true of this binary: that record leaves the
// repository DEGRADED and exit 0 (finding F14).
//
// The disjunction stays anyway, and it is a fail-safe rather than a live
// branch. Each clause alone is one future change away from printing READY over
// a repository the next command refuses, which is the shape of finding F01, and
// the cost of keeping both is a test rather than a risk. This function exists so
// that both clauses can be exercised from a value: the two survivors an audit
// found here were survivors because neither could be reached separately through
// a whole running command.
func terminalStateOf(verdict error, readiness status.Readiness) status.TerminalState {
	if verdict != nil || readiness == status.ReadinessBlocked {
		return status.TerminalBlocked
	}
	return status.TerminalReady
}

// initReportOf assembles what init did and how the repository ended up.
func initReportOf(a *bootstrap.App, verdict error, elapsed time.Duration) status.InitReport {
	return setupReportOf("init", a, verdict, elapsed)
}

func setupReportOf(command string, a *bootstrap.App, verdict error, elapsed time.Duration) status.InitReport {
	result := a.InitResult()
	current := status.Build(a.Subject(), elapsed)

	report := status.InitReport{
		Command:              command,
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

	report.TerminalState = terminalStateOf(verdict, current.Readiness)
	if report.TerminalState == status.TerminalBlocked {
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
