// Package completion discovers repository facts and evaluates the shared
// completion gate independently of CLI and MCP transports.
package completion

import (
	"context"
	"fmt"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/changes"
	"github.com/PsyChaos/mindrail/internal/gate"
	"github.com/PsyChaos/mindrail/internal/git"
	"github.com/PsyChaos/mindrail/internal/index"
	"github.com/PsyChaos/mindrail/internal/testguard"
	"github.com/PsyChaos/mindrail/internal/validation"
)

// Service shares the caller's stores and parsers. It does not own their
// lifecycle and never changes a task's coordination state or leases.
type Service struct {
	root     string
	changes  *changes.Service
	store    *changes.Store
	indexes  *index.Store
	evidence *validation.Store
	guard    *testguard.Service
}

// New composes existing services; the caller closes the resources it supplies.
func New(root string, changeService *changes.Service, indexes *index.Store, evidence *validation.Store, guard *testguard.Service) (*Service, error) {
	if root == "" || changeService == nil || indexes == nil || evidence == nil || guard == nil {
		return nil, fmt.Errorf("completion: repository root, change service, index store, evidence store and guard are required")
	}
	return &Service{root: root, changes: changeService, store: changeService.Store(), indexes: indexes, evidence: evidence, guard: guard}, nil
}

// Evaluate loads current knowledge, then reconciles Git truth before judging
// any task rows. Callers resolve taskID/projectID through coordination first.
// Reconciliation may persist discovery facts, but DENY or error never makes a
// terminal transition. A caller completing an allowed task must separately
// perform its revision-guarded task/lease transaction; this is not a filesystem
// lock and cannot make concurrent external edits atomic with that transaction.
func (s *Service) Evaluate(ctx context.Context, projectID, taskID string, required []string) (gate.Decision, error) {
	if err := ctx.Err(); err != nil {
		return gate.Decision{}, err
	}
	if projectID == "" || taskID == "" {
		return gate.Decision{}, app.NewError(app.CodeCommandLineInvalid, app.KindUsage,
			"completion needs a project and task", "Completion was not evaluated.", "Resolve the task and its project before evaluating completion.")
	}
	knowledge, err := s.loadKnowledge(ctx)
	if err != nil {
		return gate.Decision{}, err
	}
	runner := git.NewExecRunner()
	baseline, err := s.store.EnsureGuardBaseline(ctx, projectID, s.root, runner)
	if err != nil {
		return gate.Decision{}, err
	}
	if _, err := s.changes.Reconcile(ctx, projectID, s.root, taskID, "", runner); err != nil {
		return gate.Decision{}, err
	}
	if err := baseline.CheckHead(ctx, s.root, runner); err != nil {
		return gate.Decision{}, err
	}
	prior := rehydrateMappings(baseline.Mappings, knowledge)
	composed, err := s.compose(ctx, taskID, required, knowledge, prior)
	if err != nil {
		return gate.Decision{}, err
	}
	if err := baseline.CheckHead(ctx, s.root, runner); err != nil {
		return gate.Decision{}, err
	}
	return gate.New().Evaluate(composed)
}
