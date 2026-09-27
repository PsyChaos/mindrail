package cli

import (
	"context"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/changes"
	completionservice "github.com/PsyChaos/mindrail/internal/completion"
	"github.com/PsyChaos/mindrail/internal/gate"
	"github.com/PsyChaos/mindrail/internal/index"
	"github.com/PsyChaos/mindrail/internal/index/parser"
	"github.com/PsyChaos/mindrail/internal/index/snapshot"
	"github.com/PsyChaos/mindrail/internal/testguard"
	"github.com/PsyChaos/mindrail/internal/validation"
)

// evaluateTaskCompletion composes the SDK-neutral completion service from the
// same runtime database the coordination command already opened. The service
// may persist canonical reconcile facts; it deliberately cannot change task
// state or leases, which remains the caller's revision-guarded transition.
func evaluateTaskCompletion(ctx context.Context, s scope, taskID string, required []string) (gate.Decision, error) {
	registry, err := parser.NewRegistry()
	if err != nil {
		return gate.Decision{}, err
	}
	defer registry.Close()

	indexes := index.NewStore(s.app.DB(), app.SystemClock{})
	indexer := index.NewIndexer(indexes, registry, snapshot.New(s.app.Paths()))
	changeStore, err := changes.NewStore(s.app.DB(), app.SystemClock{})
	if err != nil {
		return gate.Decision{}, err
	}
	changeService, err := changes.New(changeStore, indexes, indexer)
	if err != nil {
		return gate.Decision{}, err
	}
	evidence, err := validation.NewStore(s.app.DB(), app.SystemClock{})
	if err != nil {
		return gate.Decision{}, err
	}
	guard, err := testguard.New()
	if err != nil {
		return gate.Decision{}, err
	}
	defer guard.Close()

	service, err := completionservice.New(s.app.Paths().WorktreeRoot, changeService, indexes, evidence, guard)
	if err != nil {
		return gate.Decision{}, err
	}
	return service.Evaluate(ctx, s.projectID(), taskID, required)
}
