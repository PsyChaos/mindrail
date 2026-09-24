package cli

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/bootstrap"
)

// newKnowledgeCommand builds `mindrail knowledge`.
func newKnowledgeCommand(o Options) *cobra.Command {
	group := &cobra.Command{
		Use:   "knowledge",
		Short: "Inspect the repository-owned knowledge records",
		Long: `Read the decisions and invariants this repository carries.
Validation fails closed: corrupt records and newer-than-binary schemas
refuse the whole report before any source check runs.`,
	}
	group.AddCommand(newKnowledgeValidateCommand(o))
	return group
}

func newKnowledgeValidateCommand(o Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "validate",
		Short: "Fail closed on unreadable knowledge",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runKnowledgeValidate(cmd, o)
		},
	}
	return cmd
}

func runKnowledgeValidate(cmd *cobra.Command, o Options) error {
	inv, err := newInvocation(cmd, "knowledge validate", o)
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
		store := a.Subject().Knowledge
		type problem struct {
			Path    string `json:"path"`
			Code    string `json:"code"`
			Message string `json:"message"`
			Fatal   bool   `json:"fatal"`
		}
		type report struct {
			Present    bool      `json:"present"`
			Decisions  int       `json:"decisions"`
			Invariants int       `json:"invariants"`
			Problems   []problem `json:"problems"`
		}
		out := report{
			Present:    store.Present,
			Decisions:  len(store.Decisions),
			Invariants: len(store.Invariants),
		}
		for _, issue := range store.Problems {
			out.Problems = append(out.Problems, problem{
				Path:    issue.Path,
				Code:    string(issue.Code),
				Message: issue.Message,
				Fatal:   issue.Fatal,
			})
			if issue.Fatal {
				return inv.emit(out, nil, a.Warnings(), a.Config().Config.Output.Color, app.NewError(
					issue.Code,
					app.KindFailed,
					"knowledge validation fails closed: "+issue.Message,
					"Source checks cannot run against unreadable knowledge.",
					"Fix or remove "+issue.Path+", then re-run.",
				))
			}
		}
		if out.Problems == nil {
			out.Problems = []problem{}
		}
		return inv.emit(out, nil, a.Warnings(), a.Config().Config.Output.Color, nil)
	})
}
