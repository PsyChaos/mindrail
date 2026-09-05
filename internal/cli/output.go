package cli

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/bootstrap"
	"github.com/PsyChaos/mindrail/internal/config"
	"github.com/PsyChaos/mindrail/internal/git"
)

// globalFlags are the persistent flags every command shares. They are read off
// the executing command rather than closed over, so the four command
// constructors keep the zero-argument signatures the package contract names.
type globalFlags struct {
	json    bool
	verbose bool
	noColor bool
	chdir   string
}

// Flag names, spelled once so the registration and the lookup cannot drift.
const (
	flagJSON    = "json"
	flagVerbose = "verbose"
	flagNoColor = "no-color"
	flagChdir   = "chdir"
)

// globalFlagsOf reads the persistent flags of the running command. Lookup
// errors are impossible unless the flag was never registered, which is a wiring
// defect rather than a runtime condition, so the zero value is the safe answer:
// no JSON, no colour, no verbosity.
func globalFlagsOf(cmd *cobra.Command) globalFlags {
	flags := cmd.Flags()

	jsonMode, _ := flags.GetBool(flagJSON)
	verbose, _ := flags.GetBool(flagVerbose)
	noColor, _ := flags.GetBool(flagNoColor)
	chdir, _ := flags.GetString(flagChdir)

	return globalFlags{json: jsonMode, verbose: verbose, noColor: noColor, chdir: chdir}
}

// invocation is everything a command needs that is not domain state: where the
// result goes, where the log goes, and where the command was run from.
//
// stdout and stderr come from the cobra command rather than from os, so the
// contract tests drive the real command bodies instead of a copy of them.
type invocation struct {
	command  string
	flags    globalFlags
	opts     Options
	stdout   io.Writer
	logger   *slog.Logger
	environ  []string
	startDir string
}

// newInvocation resolves the ambient inputs once. The start directory is
// resolved to an absolute path here rather than by changing the process
// directory: -C has to work identically in a shell, in a Git hook and inside a
// test running commands in parallel, and os.Chdir is process-global.
//
// The invocation is usable even when the resolution failed. A command that
// cannot name its own working directory still owes the caller the one envelope
// every other failure produces, and returning a zero invocation left `--json`
// printing nothing at all on stdout while the process exited 2 — the same
// envelope-versus-exit-code split as finding F11, one layer up.
func newInvocation(cmd *cobra.Command, command string, o Options) (invocation, error) {
	flags := globalFlagsOf(cmd)

	// Decision D-15: stdout carries the result and nothing else, so every slog
	// record goes to stderr. WARN by default keeps a routine run quiet;
	// --verbose opens it up to the startup sequence itself.
	level := slog.LevelWarn
	if flags.verbose {
		level = slog.LevelDebug
	}
	logger := slog.New(slog.NewTextHandler(cmd.ErrOrStderr(), &slog.HandlerOptions{Level: level}))

	inv := invocation{
		command: command,
		flags:   flags,
		opts:    o,
		stdout:  cmd.OutOrStdout(),
		logger:  logger.With(slog.String("command", command)),
		environ: os.Environ(),
	}

	startDir, err := resolveStartDir(flags.chdir)
	if err != nil {
		return inv, err
	}
	inv.startDir = startDir

	return inv, nil
}

// runner returns the git runner this invocation drives. nil means bootstrap
// picks the real one, which is what production does.
func (inv invocation) runner() git.CommandRunner { return inv.opts.Runner }

// resolveStartDir turns -C into an absolute directory, or falls back to the
// process working directory.
func resolveStartDir(chdir string) (string, error) {
	if chdir == "" {
		dir, err := os.Getwd()
		if err != nil {
			return "", startDirError("the working directory could not be determined", err)
		}
		return dir, nil
	}

	abs, err := filepath.Abs(chdir)
	if err != nil {
		return "", startDirError("the -C directory "+chdir+" could not be resolved", err)
	}
	return abs, nil
}

// startDirError codes the one failure that happens before any subsystem exists.
// It carries a payload like every other failure so that the envelope, the human
// block and the exit code are derived from the same value here too.
func startDirError(why string, cause error) error {
	return app.NewError(
		app.CodeConfigInvalid,
		app.KindUsage,
		why,
		"Mindrail scopes everything it does to a directory and has none to work from.",
		"Run the command from a directory that exists.",
		"Or pass -C with a path that does.",
	).WithCause(cause)
}

// humanRenderer is the human form of a command result. It takes the colour
// decision rather than making it, because the decision is the same for every
// command and belongs in one place (tech-stack §12).
type humanRenderer func(w io.Writer, color bool) error

// emit writes exactly one command result to stdout and returns the command's
// verdict.
//
// verdict is the single input from which all three of the caller's answers
// follow: `ok` is false exactly when it is non-nil, the `error` object is its
// payload, and main derives the exit code from the same value emit returns.
// Nothing downstream re-derives any of them, which is what keeps them from
// contradicting each other (finding F11).
//
// In JSON mode that is one envelope and nothing else, which is what makes
// `--json` parseable with no filtering (decision D-15). In human mode the
// structured error block comes first and the report second, so that init's
// terminal line — the sentence a human reads last and a CI job greps for — is
// still the last thing printed.
func (inv invocation) emit(data any, human humanRenderer, warnings []app.Warning, colorPref string, verdict error) error {
	inv.logCause(verdict)

	if inv.flags.json {
		// A document JSON cannot carry unchanged is refused rather than mangled
		// (finding W9). The swap happens here, before the envelope is built, so
		// that `ok`, the error object and the exit code all still follow from one
		// value: doing it inside the serializer would have left emit returning the
		// verdict of a report that was never published, which is finding F11's
		// split wearing a new cause.
		if refusal := refuseUnrepresentableJSON(data, verdict); refusal != nil {
			data, verdict = nil, refusal
		}

		if err := app.WriteJSON(inv.stdout, inv.command, data, warnings, verdict); err != nil {
			return app.Failed(fmt.Errorf("write %s output: %w", inv.command, err))
		}
		return verdict
	}

	color := app.ColorEnabled(inv.stdout, inv.environ, inv.colorPreference(colorPref), false)

	if verdict != nil {
		if err := app.RenderError(inv.stdout, verdict); err != nil {
			return app.Failed(fmt.Errorf("write %s output: %w", inv.command, err))
		}
		if human != nil {
			if _, err := io.WriteString(inv.stdout, "\n"); err != nil {
				return app.Failed(fmt.Errorf("write %s output: %w", inv.command, err))
			}
		}
	}

	if human != nil {
		if err := human(inv.stdout, color); err != nil {
			return app.Failed(fmt.Errorf("write %s output: %w", inv.command, err))
		}
	}

	return verdict
}

// logCause puts the failing layer's own message on stderr.
//
// It is a DEBUG record rather than part of the result: on a routine failure the
// user's business is the domain sentence and the remedy, and a driver string
// underneath them is noise. --verbose is the request for the whole story, and
// before this record existed the answer to "the runtime database could not be
// opened — but why?" was reachable only by rebuilding the binary.
func (inv invocation) logCause(verdict error) {
	cause := app.CauseOf(verdict)
	if cause == nil {
		return
	}
	inv.logger.Debug("failure cause", slog.String("cause", cause.Error()))
}

// colorPreference folds --no-color into the configured preference. The flag
// wins because it is the most explicit statement the user can make, and it has
// to work when the configuration itself is what failed to load.
func (inv invocation) colorPreference(configured string) string {
	if inv.flags.noColor {
		return config.ColorNever
	}
	if configured == "" {
		return config.ColorAuto
	}
	return configured
}

// blockedReason renders the one-line explanation that follows init's BLOCKED
// verdict (spec §82). It prefers the error's own why, because that sentence was
// written by the layer that detected the failure.
func blockedReason(verdict error, fallback string) string {
	if verdict != nil {
		if payload, ok := app.PayloadOf(verdict); ok && payload.Why != "" {
			return payload.Why
		}
		if verdict.Error() != "" {
			return verdict.Error()
		}
	}
	if fallback != "" {
		return fallback
	}
	return "the repository is not ready for targeted work"
}

// shutdown drains the application on every path, successful or not.
//
// The command context may already be cancelled — that is what a Ctrl-C looks
// like — so the drain deliberately does not inherit its cancellation: closing
// SQLite is the work that matters most precisely when the process was
// interrupted (tech-stack §88).
func shutdown(ctx context.Context, a *bootstrap.App, logger *slog.Logger) {
	if err := a.Shutdown(context.WithoutCancel(ctx)); err != nil {
		logger.Warn("shutdown failed", slog.String("error", err.Error()))
	}
}
