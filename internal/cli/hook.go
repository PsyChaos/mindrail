package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/bootstrap"
)

// Hook block markers. Exact-match: owned lines only, foreign edits between
// them are preserved byte-for-byte (decision D-218).
const (
	hookBeginMarker = "# mindrail:begin verify-staged"
	hookEndMarker   = "# mindrail:end verify-staged"
	hookBlockBody   = "mindrail verify --staged"
)

// newHookCommand builds `mindrail hook`.
func newHookCommand(o Options) *cobra.Command {
	group := &cobra.Command{
		Use:   "hook",
		Short: "Manage repository git hooks without touching foreign content",
		Long: `Install Mindrail's pre-commit guard by appending a marker-delimited
block. Existing hook content is never rewritten; a second install changes
nothing.`,
	}
	group.AddCommand(newHookInstallCommand(o))
	return group
}

func newHookInstallCommand(o Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "install",
		Short: "Append the verify-staged block to the pre-commit hook",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runHookInstall(cmd, o)
		},
	}
	return cmd
}

func runHookInstall(cmd *cobra.Command, o Options) error {
	inv, err := newInvocation(cmd, "hook install", o)
	if err != nil {
		return inv.emit(nil, nil, nil, "", err)
	}
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
		commonDir := a.Subject().Repo.CommonDir
		if commonDir == "" {
			return inv.emit(nil, nil, a.Warnings(), a.Config().Config.Output.Color, app.NewError(
				app.CodeNotAGitRepository,
				app.KindUsage,
				"hook install needs a git repository",
				"There is no common directory to hold the hook.",
				"Run inside a git worktree, then re-run install.",
			))
		}
		result, err := installHookBlock(filepath.Join(commonDir, "hooks", "pre-commit"))
		if err != nil {
			return inv.emit(nil, nil, a.Warnings(), a.Config().Config.Output.Color, err)
		}
		return inv.emit(result, nil, a.Warnings(), a.Config().Config.Output.Color, nil)
	})
}

type hookInstallResult struct {
	Path    string `json:"path"`
	Changed bool   `json:"changed"`
}

// installHookBlock appends the guarded block when absent and reports
// byte-identical no-op when present. Foreign content is never rewritten:
// the file is read fully, matched exactly, and only appended to.
func installHookBlock(path string) (hookInstallResult, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return hookInstallResult{}, err
		}
		block := "#!/bin/sh\n" + hookBlock()
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return hookInstallResult{}, err
		}
		if err := os.WriteFile(path, []byte(block), 0o755); err != nil {
			return hookInstallResult{}, err
		}
		return hookInstallResult{Path: path, Changed: true}, nil
	}
	if strings.Contains(string(content), hookBeginMarker) && strings.Contains(string(content), hookEndMarker) {
		return hookInstallResult{Path: path, Changed: false}, nil
	}
	joined := string(content)
	if !strings.HasSuffix(joined, "\n") {
		joined += "\n"
	}
	joined += hookBlock()
	if err := os.WriteFile(path, []byte(joined), 0o755); err != nil {
		return hookInstallResult{}, err
	}
	return hookInstallResult{Path: path, Changed: true}, nil
}

func hookBlock() string {
	return hookBeginMarker + "\n" + hookBlockBody + "\n" + hookEndMarker + "\n"
}
