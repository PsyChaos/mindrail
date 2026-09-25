package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/bootstrap"
	"github.com/PsyChaos/mindrail/internal/changes"
	"github.com/PsyChaos/mindrail/internal/filesystem"
	"github.com/PsyChaos/mindrail/internal/git"
	"github.com/PsyChaos/mindrail/internal/index"
	"github.com/PsyChaos/mindrail/internal/index/parser"
	"github.com/PsyChaos/mindrail/internal/index/snapshot"
	"github.com/PsyChaos/mindrail/internal/testguard"
	"github.com/PsyChaos/mindrail/internal/verify"
)

// newVerifyCommand builds `mindrail verify`.
func newVerifyCommand(o Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "verify",
		Short: "Evaluate staged changes through the shared gates",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runVerify(cmd, o)
		},
	}
	cmd.Flags().Bool("staged", false, "evaluate the index instead of the worktree")
	cmd.Flags().Bool("ci", false, "evaluate a committed merge-base range instead of the index")
	cmd.Flags().String("base", "", "range base revision for --ci (default: origin/main, main, master)")
	cmd.Flags().String("head", "", "range head revision for --ci (default: HEAD)")
	return cmd
}

func runVerify(cmd *cobra.Command, o Options) error {
	inv, err := newInvocation(cmd, "verify", o)
	if err != nil {
		return inv.emit(nil, nil, nil, "", err)
	}
	staged, err := cmd.Flags().GetBool("staged")
	if err != nil {
		return inv.emit(nil, nil, nil, "", err)
	}
	ci, err := cmd.Flags().GetBool("ci")
	if err != nil {
		return inv.emit(nil, nil, nil, "", err)
	}
	base, err := cmd.Flags().GetString("base")
	if err != nil {
		return inv.emit(nil, nil, nil, "", err)
	}
	head, err := cmd.Flags().GetString("head")
	if err != nil {
		return inv.emit(nil, nil, nil, "", err)
	}
	if staged && ci {
		return inv.emit(nil, nil, nil, "", app.NewError(
			app.CodeCommandLineInvalid,
			app.KindUsage,
			"verify takes one mode: --staged or --ci, never both",
			"The index and a committed range are different things to judge.",
			"Re-run with exactly one of `mindrail verify --staged` or `mindrail verify --ci`.",
		))
	}
	if (base != "" || head != "") && !ci {
		return inv.emit(nil, nil, nil, "", app.NewError(
			app.CodeCommandLineInvalid,
			app.KindUsage,
			"verify --base and --head need --ci",
			"Revisions name a committed range, which only the CI mode judges.",
			"Re-run as `mindrail verify --ci --base <rev> --head <rev>`.",
		))
	}
	if !staged && !ci {
		return inv.emit(nil, nil, nil, "", app.NewError(
			app.CodeCommandLineInvalid,
			app.KindUsage,
			"verify needs a mode: --staged or --ci",
			"Without a mode the command cannot know whether the index or a committed range is judged.",
			"Re-run as `mindrail verify --staged` or `mindrail verify --ci`.",
		))
	}
	application := bootstrap.New(bootstrap.Options{
		StartDir:    inv.startDir,
		Mode:        bootstrap.ModeWrite,
		Runner:      inv.runner(),
		BusyTimeout: inv.opts.BusyTimeout,
		Logger:      inv.logger,
		Environ:     inv.environ,
	})
	defer shutdown(cmd.Context(), application, inv.logger)

	return application.Run(cmd.Context(), func(ctx context.Context, a *bootstrap.App) error {
		verdict, err := verifyMode(ctx, a, ci, base, head)
		if err != nil {
			return inv.emit(nil, nil, a.Warnings(), a.Config().Config.Output.Color, err)
		}
		mode := "verify --staged"
		judged := "Staged changes fail the shared gates; the commit would not complete."
		if ci {
			mode = "verify --ci"
			judged = "The committed range fails the shared gates; the push would not complete."
		}
		if verdict.FatalKnown {
			fatal := verdict.Knowledge[0]
			for _, problem := range verdict.Knowledge {
				if problem.Fatal {
					fatal = problem
					break
				}
			}
			return inv.emit(verdictReport(verdict), nil, a.Warnings(), a.Config().Config.Output.Color, app.NewError(
				app.Code(fatal.Code),
				app.KindFailed,
				mode+" refuses: "+fatal.Message,
				"Knowledge fails closed before any source check runs.",
				"Fix or remove "+fatal.Path+", then re-run verify.",
			))
		}
		if !verdict.Allow {
			denied := app.NewError(
				verdict.Denials[0].Code,
				app.KindFailed,
				fmt.Sprintf("%s denies: %d blocking findings", mode, len(verdict.Denials)),
				judged,
				"Resolve the denials listed in the report, then re-run verify.",
			)
			return inv.emit(verdictReport(verdict), nil, a.Warnings(), a.Config().Config.Output.Color, denied)
		}
		return inv.emit(verdictReport(verdict), nil, a.Warnings(), a.Config().Config.Output.Color, nil)
	})
}

// verifyMode dispatches the judged source: the index for --staged, the
// committed merge-base range for --ci. Both compose the same services,
// built once here so the two modes cannot drift apart.
func verifyMode(ctx context.Context, a *bootstrap.App, ci bool, base, head string) (verify.Verdict, error) {
	registry, err := parser.NewRegistry()
	if err != nil {
		return verify.Verdict{}, err
	}
	defer registry.Close()
	indexes := index.NewStore(a.DB(), app.SystemClock{})
	indexer := index.NewIndexer(indexes, registry, snapshot.New(filesystem.RuntimePaths{
		CacheDir: a.Paths().CacheDir,
	}))
	changeStore, err := changes.NewStore(a.DB(), app.SystemClock{})
	if err != nil {
		return verify.Verdict{}, err
	}
	changeService, err := changes.New(changeStore, indexes, indexer)
	if err != nil {
		return verify.Verdict{}, err
	}
	guard, err := testguard.New()
	if err != nil {
		return verify.Verdict{}, err
	}
	defer guard.Close()
	service, err := verify.New(changeService, indexes, guard)
	if err != nil {
		return verify.Verdict{}, err
	}
	projectID, err := verifyProject(a)
	if err != nil {
		return verify.Verdict{}, err
	}
	root := a.Paths().WorktreeRoot
	runner := git.NewExecRunner()
	if ci {
		return service.VerifyCI(ctx, projectID, root, runner, base, head)
	}
	return service.VerifyStaged(ctx, projectID, root, runner)
}

func verifyProject(a *bootstrap.App) (string, error) {
	workspace := a.Subject().Workspace
	if workspace.ProjectID == "" {
		return "", app.NewError(
			app.CodeCommandLineInvalid,
			app.KindUsage,
			"verify needs a registered worktree",
			"Staged evaluation scopes indexing to a project it cannot name.",
			"Run `mindrail init` in the worktree, then re-run verify.",
		)
	}
	return workspace.ProjectID, nil
}

func verdictReport(verdict verify.Verdict) any {
	type wiredDenial struct {
		Code       string   `json:"code"`
		Reason     string   `json:"reason"`
		Key        string   `json:"key"`
		Provenance string   `json:"provenance"`
		NextAction []string `json:"next_action"`
	}
	type wiredProblem struct {
		Path    string `json:"path"`
		Code    string `json:"code"`
		Message string `json:"message"`
		Fatal   bool   `json:"fatal"`
	}
	type report struct {
		Allow     bool           `json:"allow"`
		Denials   []wiredDenial  `json:"denials"`
		Knowledge []wiredProblem `json:"knowledge_problems"`
	}
	out := report{Allow: verdict.Allow}
	for _, denial := range verdict.Denials {
		out.Denials = append(out.Denials, wiredDenial{
			Code:       string(denial.Code),
			Reason:     denial.Reason,
			Key:        denial.Key,
			Provenance: denial.Provenance,
			NextAction: denial.NextAction,
		})
	}
	for _, problem := range verdict.Knowledge {
		out.Knowledge = append(out.Knowledge, wiredProblem{
			Path:    problem.Path,
			Code:    problem.Code,
			Message: problem.Message,
			Fatal:   problem.Fatal,
		})
	}
	if out.Denials == nil {
		out.Denials = []wiredDenial{}
	}
	if out.Knowledge == nil {
		out.Knowledge = []wiredProblem{}
	}
	return out
}
